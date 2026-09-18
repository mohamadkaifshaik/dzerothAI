// cmd/api is the Dzeroth core API process.
// It wires all internal packages, runs pending database migrations, starts the HTTP
// server, and shuts down gracefully on SIGTERM or SIGINT.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	appdb "github.com/mohamadkaifshaik/dzerothAI/apps/backend/db"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/config"
	platformDB "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/db"
	platformRedis "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/redis"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/user"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// ── 1. Load and validate configuration ───────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// ── 2. Initialize structured logger ──────────────────────────────────────
	log, err := buildLogger(cfg)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting dzeroth api",
		zap.String("environment", cfg.Environment),
		zap.String("port", cfg.APIPort),
	)

	// ── 3. Connect to PostgreSQL ──────────────────────────────────────────────
	bgCtx := context.Background()
	pool, err := platformDB.Connect(bgCtx, cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer platformDB.Close(pool)
	log.Info("database connected")

	// ── 4. Run pending migrations ─────────────────────────────────────────────
	if err := runMigrations(cfg.PostgresDSN, log); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// ── 5. Connect to Redis (non-fatal — fail closed at rate limiter) ─────────
	redisClient, redisErr := platformRedis.Connect(bgCtx, cfg.RedisAddr)
	if redisErr != nil {
		// Redis unavailability is not fatal at startup. Auth endpoints will fail
		// closed (503) when they cannot reach Redis.
		log.Error("redis unavailable at startup — rate limiting will fail closed",
			zap.String("addr", cfg.RedisAddr),
			zap.Error(redisErr),
		)
		// redisClient is nil when Connect fails; downstream code handles nil safely.
	} else {
		log.Info("redis connected", zap.String("addr", cfg.RedisAddr))
	}
	defer func() {
		if redisClient != nil {
			_ = platformRedis.Close(redisClient)
		}
	}()

	// ── 6. Wire services ──────────────────────────────────────────────────────
	authSvc := auth.NewService(pool, cfg.JWTSecret, log)
	userSvc := user.NewService(pool, log)

	authHandler := auth.NewHandler(authSvc, log)
	userHandler := user.NewHandler(userSvc, log)

	// ── 7. Build router ───────────────────────────────────────────────────────
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	if cfg.Environment == "local" || cfg.Environment == "test" {
		r.Use(corsAllowAll)
	} else {
		r.Use(corsRestrictive)
	}

	// ── 8. Register routes ────────────────────────────────────────────────────
	r.Get("/health", buildHealthHandler(pool, redisClient, log))

	r.Route("/api/v1", func(r chi.Router) {
		authHandler.RegisterRoutes(r, redisClient, cfg.JWTSecret)
		userHandler.RegisterRoutes(r, cfg.JWTSecret)
	})

	// ── 9. Start HTTP server ──────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.APIPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ── 10. Graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case sig := <-quit:
		log.Info("shutdown signal received", zap.String("signal", sig.String()))
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(bgCtx, 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	log.Info("server stopped cleanly")
	return nil
}

// buildLogger creates a zap logger tuned to the configured environment and level.
func buildLogger(cfg *config.Config) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = zapcore.InfoLevel
	}

	if cfg.Environment == "local" || cfg.Environment == "test" {
		devCfg := zap.NewDevelopmentConfig()
		devCfg.Level = zap.NewAtomicLevelAt(level)
		return devCfg.Build()
	}

	prodCfg := zap.NewProductionConfig()
	prodCfg.Level = zap.NewAtomicLevelAt(level)
	return prodCfg.Build()
}

// runMigrations applies all pending up migrations using the embedded SQL files.
// The migration files are embedded via apps/backend/db/migrations.go.
// The golang-migrate pgx/v5 driver requires the "pgx5://" URL scheme.
func runMigrations(dsn string, log *zap.Logger) error {
	src, err := iofs.New(appdb.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	// golang-migrate's pgx/v5 driver expects "pgx5://" instead of "postgres://".
	migrateDSN := "pgx5://" + strings.TrimPrefix(dsn, "postgres://")

	m, err := migrate.NewWithSourceInstance("iofs", src, migrateDSN)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up: %w", err)
	}

	log.Info("database migrations applied")
	return nil
}

// healthResponse is the JSON body for GET /health.
type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
	Redis  string `json:"redis"`
}

// buildHealthHandler returns the GET /health handler.
// If db is unreachable: status="degraded", HTTP 503.
// If redis is unreachable: status="degraded", HTTP 503 (API continues serving).
func buildHealthHandler(pool *pgxpool.Pool, redisClient *rdb.Client, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unavailable"
			log.Warn("health: database ping failed", zap.Error(err))
		}

		redisStatus := "ok"
		if !platformRedis.IsAvailable(ctx, redisClient) {
			redisStatus = "unavailable"
		}

		status := "ok"
		httpStatus := http.StatusOK
		if dbStatus != "ok" || redisStatus != "ok" {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		resp := healthResponse{
			Status: status,
			DB:     dbStatus,
			Redis:  redisStatus,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// corsAllowAll is a CORS middleware that permits all origins.
// Used in local and test environments only.
func corsAllowAll(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsRestrictive is a placeholder CORS middleware for production.
// TODO(production): read allowed origins from config and enforce them.
// Until explicitly configured, cross-origin requests are not permitted in production.
func corsRestrictive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
