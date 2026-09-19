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
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/block"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/bookmark"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/config"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/feed"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/follow"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/notification"
	platformDB "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/db"
	platformRedis "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/redis"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/reaction"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/report"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/search"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/studio"
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
	postRepo := post.NewRepository(pool)
	postSvc := post.NewService(postRepo, log)
	followRepo := follow.NewRepository(pool)
	followSvc := follow.NewService(followRepo, log)
	blockRepo := block.NewRepository(pool)
	blockSvc := block.NewService(blockRepo, log)

	// Phase-3 dependency injection: wire cross-package interfaces to avoid
	// circular imports. Setters are called in the startup phase before
	// the HTTP server starts listening.
	postSvc.SetFollowChecker(followSvc)
	postSvc.SetBlockProvider(blockSvc)

	// Phase-4: feed package. followRepo satisfies feed.FollowProvider directly
	// because GetFollowedIDs lives on follow.Repository, not follow.Service.
	feedRepo := feed.NewRepository(pool)
	feedSvc := feed.NewService(feedRepo, blockSvc, followRepo, log)
	feedHandler := feed.NewHandler(feedSvc, log)

	bookmarkRepo := bookmark.NewRepository(pool)
	bookmarkSvc := bookmark.NewService(bookmarkRepo, log)
	bookmarkHandler := bookmark.NewHandler(bookmarkSvc, log)

	// Phase-4 Wave 2c: notification, reaction, search packages.
	notifRepo := notification.NewRepository(pool)
	notifSvc := notification.NewService(notifRepo, log)
	notifHandler := notification.NewHandler(notifSvc, log)

	reactionRepo := reaction.NewRepository(pool)
	reactionSvc := reaction.NewService(reactionRepo, notifSvc, redisClient, log)
	reactionHandler := reaction.NewHandler(reactionSvc, postSvc, log)

	searchRepo := search.NewRepository(pool)
	searchSvc := search.NewService(searchRepo, blockSvc, log)
	searchHandler := search.NewHandler(searchSvc, log)

	// Wire notification publisher into existing services so that follow, reply,
	// and mention events trigger best-effort notifications. Setters are called
	// in the startup phase before the HTTP server starts listening.
	followSvc.SetNotificationPublisher(notifSvc)
	// postSvc requires an adapter because internal/notification imports internal/post
	// (for FeedCursor), so internal/post cannot directly import internal/notification.
	postSvc.SetNotificationPublisher(&postNotificationAdapter{svc: notifSvc})

	// Phase 5 — Report
	reportRepo := report.NewRepository(pool)
	reportSvc := report.NewService(reportRepo, postSvc, userSvc, redisClient, log)
	reportHandler := report.NewHandler(reportSvc, log)

	// Phase 5 — Studio (Private Creator Studio analytics)
	studioRepo := studio.NewRepository(pool)
	studioSvc := studio.NewService(studioRepo, redisClient, log)
	studioHandler := studio.NewHandler(studioSvc, log)

	// Phase 5 — Self-suspension session revoker: auth.Service satisfies user.SessionRevoker.
	userSvc.SetSessionRevoker(authSvc)

	authHandler := auth.NewHandler(authSvc, log)
	userHandler := user.NewHandler(userSvc, log)
	userHandler.SetBlockChecker(blockSvc)
	postHandler := post.NewHandler(postSvc, log)
	postHandler.SetReactionChecker(reactionSvc)
	followHandler := follow.NewHandler(followSvc, log)
	blockHandler := block.NewHandler(blockSvc, log)

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
		if len(cfg.CORSAllowedOrigins) == 0 {
			log.Warn("CORS_ALLOWED_ORIGINS is not set; all cross-origin requests will be blocked")
		} else {
			log.Info("cors allowlist configured", zap.Int("origin_count", len(cfg.CORSAllowedOrigins)))
		}
		r.Use(corsAllowList(cfg.CORSAllowedOrigins, log))
	}

	// ── 8. Register routes ────────────────────────────────────────────────────
	r.Get("/health", buildHealthHandler(pool, redisClient, log))

	r.Route("/api/v1", func(r chi.Router) {
		authHandler.RegisterRoutes(r, redisClient, cfg.JWTSecret)
		userHandler.RegisterRoutes(r, cfg.JWTSecret)
		postHandler.RegisterRoutes(r, redisClient, cfg.JWTSecret)
		followHandler.RegisterRoutes(r, redisClient, cfg.JWTSecret)
		blockHandler.RegisterRoutes(r, redisClient, cfg.JWTSecret)
		feedHandler.RegisterRoutes(r, cfg.JWTSecret)
		bookmarkHandler.RegisterRoutes(r, redisClient, cfg.JWTSecret)
		notifHandler.RegisterRoutes(r, cfg.JWTSecret)
		reactionHandler.RegisterRoutes(r, cfg.JWTSecret)
		searchHandler.RegisterRoutes(r, redisClient, cfg.JWTSecret)
		reportHandler.RegisterRoutes(r, cfg.JWTSecret)
		studioHandler.RegisterRoutes(r, cfg.JWTSecret)
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

// postNotificationAdapter adapts notification.Service to the post.PostNotificationPublisher
// interface. This adapter is required because internal/notification imports internal/post
// (for FeedCursor), preventing internal/post from directly importing internal/notification.
// The adapter lives in cmd/api/main.go — the only layer that may depend on both packages.
type postNotificationAdapter struct {
	svc *notification.Service
}

func (a *postNotificationAdapter) PublishPostEvent(ctx context.Context, event post.PostNotificationEvent) error {
	postIDCopy := event.PostID
	return a.svc.Publish(ctx, notification.PublishEvent{
		RecipientID: event.RecipientID,
		ActorID:     event.ActorID,
		Event:       notification.NotificationEvent(event.Event),
		PostID:      postIDCopy,
	})
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

// corsAllowList returns a middleware that enforces an explicit origin allowlist.
// Only origins in the provided slice receive CORS response headers.
// Requests without an Origin header, or with a disallowed origin, proceed without
// any Access-Control-Allow-Origin header — the browser blocks the response.
// Preflight OPTIONS requests on disallowed origins return 204 with no CORS headers.
// Access-Control-Allow-Origin: * is never emitted by this middleware.
// If origins is empty all cross-origin requests are effectively blocked.
func corsAllowList(origins []string, _ *zap.Logger) func(http.Handler) http.Handler {
	// Build a set for O(1) lookup.
	allowedSet := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowedSet[o] = struct{}{}
	}

	const (
		allowMethods = "GET, POST, PUT, DELETE, OPTIONS"
		allowHeaders = "Authorization, Content-Type, X-Request-ID"
		maxAge       = "600"
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			_, allowed := allowedSet[origin]
			if origin != "" && allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				// Vary: Origin is required so intermediary caches do not serve a
				// cached CORS response to a different origin.
				w.Header().Set("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
