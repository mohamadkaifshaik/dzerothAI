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
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo"
	platformDB "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/db"
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
	platformMW "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/middleware"
	platformRedis "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/redis"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/reaction"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/report"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/search"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/studio"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/title"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/user"
)

// sessionCleanupBatchSize caps the number of expired session rows removed per
// cleanup cycle. Bounded batches prevent long-duration locks on large tables.
const sessionCleanupBatchSize = 500

// maxRequestBodyBytes is the application-level request body size limit (1 MiB).
// Applied as a global middleware so no single request body can grow unbounded
// regardless of Content-Length. Handlers that decode a body must check for
// *http.MaxBytesError and return 413 Request Entity Too Large.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

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
		zap.String("version", buildinfo.Version),
		zap.String("commit", buildinfo.Commit),
		zap.String("build_time", buildinfo.BuildTime),
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
	redisClient, redisErr := platformRedis.Connect(bgCtx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisTLS)
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

	// Phase 8 — Title HTTP API (wired here so titleHandler is available for route registration below).
	// titleRepo is constructed in the workers section below; we forward-declare it here
	// so the handler can be registered before the HTTP server starts.
	titleRepo := title.NewRepository(pool)
	titleSvc := title.NewService(titleRepo, log)
	titleSvc.SetFollowChecker(followSvc)
	titleSvc.SetPrivacyChecker(&titleUserPrivacyAdapter{svc: userSvc})
	titleHandler := title.NewHandler(titleSvc, log)

	// ── 7. Build HTTP metrics instruments ────────────────────────────────────
	httpMetrics := platformMetrics.New(prometheus.DefaultRegisterer)
	// Event counters for auth, rate-limit, and feed termination events.
	// Registered on the same DefaultRegisterer; exposed via /metrics on the admin listener.
	eventMetrics := platformMetrics.NewEvents(prometheus.DefaultRegisterer)
	// Infrastructure gauges for DB pool and Redis availability, plus the Redis error
	// counter and state-change logger. Updated by piggybacking on /health and /readyz
	// handler calls — no extra goroutine.
	infraMetrics := platformMetrics.NewInfraMetrics(prometheus.DefaultRegisterer, log)
	// Build identity gauge: dzeroth_build_info{version,commit,build_time}=1.
	// Populated at build time via -ldflags; defaults to dev/unknown/unknown for local builds.
	platformMetrics.RegisterBuildInfo(prometheus.DefaultRegisterer,
		buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime)
	authSvc.SetEvents(eventMetrics)
	authHandler.SetEvents(eventMetrics)
	feedSvc.SetEvents(eventMetrics)
	postHandler.SetEvents(eventMetrics)
	followHandler.SetEvents(eventMetrics)
	blockHandler.SetEvents(eventMetrics)
	bookmarkHandler.SetEvents(eventMetrics)
	searchHandler.SetEvents(eventMetrics)
	reactionSvc.SetEvents(eventMetrics)
	reportSvc.SetEvents(eventMetrics)
	studioSvc.SetEvents(eventMetrics)

	// ── 8. Build router ───────────────────────────────────────────────────────
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(platformMW.AccessLog(log))
	r.Use(chimw.Recoverer)
	// LimitRequestBody wraps every request body with http.MaxBytesReader so
	// that no handler can read more than maxRequestBodyBytes (1 MiB). Applied
	// after Recoverer so recovered panics are unaffected, and before
	// SecurityHeaders so the 413 response still carries security headers.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req.Body = http.MaxBytesReader(w, req.Body, maxRequestBodyBytes)
			next.ServeHTTP(w, req)
		})
	})
	// SecurityHeaders adds X-Content-Type-Options, X-Frame-Options, and Referrer-Policy
	// to every response. Applied after Recoverer so that security headers are present
	// even on recovered panics. HSTS is intentionally omitted — it belongs at the
	// reverse proxy (nginx/Caddy/ALB), not the application server.
	r.Use(platformMW.SecurityHeaders)
	r.Use(chimw.Timeout(30 * time.Second))
	// HTTP metrics middleware records http_requests_total and
	// http_request_duration_seconds using normalized chi route patterns as labels.
	r.Use(httpMetrics.Middleware)

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

	// ── 9. Register public API routes ─────────────────────────────────────────
	// /health remains on the public router for backward compatibility with existing
	// callers (load balancers, etc.) that already use this path.
	r.Get("/health", buildHealthHandler(pool, redisClient, log, infraMetrics))

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
		titleHandler.RegisterRoutes(r, cfg.JWTSecret)
	})

	// ── 10. Build admin router ────────────────────────────────────────────────
	// The admin router exposes /livez, /readyz, /health, and /metrics.
	// It must NOT be exposed on the public API port or through the public load balancer.
	adminRouter := chi.NewRouter()
	adminRouter.Get("/livez", buildLivezHandler())
	adminRouter.Get("/readyz", buildReadyzHandler(pool, redisClient, log, infraMetrics))
	adminRouter.Get("/health", buildHealthHandler(pool, redisClient, log, infraMetrics))
	adminRouter.Handle("/metrics", promhttp.Handler())

	// ── 11. Start HTTP servers ────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.APIPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	adminSrv := &http.Server{
		Addr:         cfg.AdminAddr,
		Handler:      adminRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── 12. Graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	// workerCtx is cancelled on shutdown signal so background workers stop
	// cleanly without waiting for the full HTTP shutdown timeout.
	workerCtx, workerCancel := context.WithCancel(bgCtx)
	defer workerCancel()
	var workerWG sync.WaitGroup

	// ── 13. Start background workers ──────────────────────────────────────────
	// Session cleanup: removes expired session rows periodically in bounded
	// batches. Non-critical — DB errors are logged at Warn and the worker
	// continues. Stops when workerCtx is cancelled (i.e. on shutdown signal).
	cleanupWorker := auth.NewSessionCleanupWorker(
		pool,
		log,
		cfg.SessionCleanupInterval,
		sessionCleanupBatchSize,
	)
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		cleanupWorker.Run(workerCtx)
		log.Info("session cleanup worker stopped")
	}()

	// Title qualification worker: periodically evaluates all users for title
	// qualification and reconciles lifecycle state (active → grace_period →
	// revoked, and grace_period → active restores). Non-critical — per-user
	// errors are logged at Warn and the pass continues.
	// Note: titleRepo is already constructed above (Phase 8 wiring).
	titleEngine := title.NewEngine(titleRepo, log)
	titleWorker := title.NewTitleQualificationWorker(
		titleEngine,
		titleRepo,
		title.WorkerConfig{Interval: cfg.TitleWorkerInterval},
		log,
	)
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		titleWorker.Run(workerCtx)
	}()

	// Title notification worker: periodically dispatches pending title unlock
	// and grace period notifications. Reuses the existing notifSvc publisher.
	// Publish-first, mark-sent-second failure contract: the notifications table
	// deduplication index prevents duplicate rows if the process crashes between
	// publish and mark-sent.
	titleNotifCfg := title.NotificationWorkerConfig{
		Interval:  cfg.TitleNotificationInterval,
		BatchSize: 100,
	}
	titleNotifWorker := title.NewTitleNotificationWorker(titleRepo, notifSvc, titleNotifCfg, log)
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		titleNotifWorker.Run(workerCtx)
	}()

	serverErr := make(chan error, 2)
	go func() {
		log.Info("http server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("api: %w", err)
		}
	}()
	go func() {
		log.Info("admin server listening", zap.String("addr", adminSrv.Addr))
		if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("admin: %w", err)
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case sig := <-quit:
		log.Info("shutdown signal received", zap.String("signal", sig.String()))
	}

	// Cancel worker context first so background goroutines stop immediately
	// while HTTP servers drain in-flight requests.
	workerCancel()
	workerWG.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(bgCtx, 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown api: %w", err)
	}
	if err := adminSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown admin: %w", err)
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

// titleUserPrivacyAdapter adapts *user.Service to the title.userPrivacyChecker
// interface. Defined in cmd/api/main.go — the only layer that may depend on
// both internal/title and internal/user — to avoid an import cycle.
type titleUserPrivacyAdapter struct {
	svc *user.Service
}

func (a *titleUserPrivacyAdapter) IsPrivateAccount(ctx context.Context, userID uuid.UUID) (bool, error) {
	u, err := a.svc.GetProfile(ctx, userID)
	if err != nil {
		return false, err
	}
	return u.IsPrivate, nil
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

// dbPinger abstracts PostgreSQL connectivity checks so that handlers can be tested
// without a live database. *pgxpool.Pool satisfies this interface.
type dbPinger interface {
	Ping(ctx context.Context) error
}

// dbStatter abstracts pool statistics retrieval. It returns a dbPoolStats value
// directly so that the health handler does not depend on *pgxpool.Stat internals,
// and test implementations do not need to construct a live pool.
type dbStatter interface {
	PoolStats() dbPoolStats
}

// dbChecker composes the two pool capability interfaces required by health handlers.
type dbChecker interface {
	dbPinger
	dbStatter
}

// redisChecker abstracts Redis availability checks so handlers can be tested without
// a live Redis server.
type redisChecker interface {
	// IsAvailable performs a bounded Redis ping and returns true if reachable.
	IsAvailable(ctx context.Context) bool
}

// redisClientChecker wraps *rdb.Client to satisfy the redisChecker interface using
// the existing platformRedis.IsAvailable helper.
type redisClientChecker struct {
	client *rdb.Client
}

func (rc *redisClientChecker) IsAvailable(ctx context.Context) bool {
	return platformRedis.IsAvailable(ctx, rc.client)
}

// livezResponse is the JSON body for GET /livez.
type livezResponse struct {
	Status string `json:"status"`
}

// buildLivezHandler returns the GET /livez handler.
// Liveness checks only that the HTTP server goroutine is alive — it never checks
// dependencies. If this endpoint responds, the process is alive.
func buildLivezHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(livezResponse{Status: "ok"})
	}
}

// readyzResponse is the JSON body for GET /readyz.
type readyzResponse struct {
	Status string `json:"status"`
	DB     string `json:"db,omitempty"`
	Redis  string `json:"redis,omitempty"`
}

// buildReadyzHandler returns the GET /readyz handler.
// Readiness requires both PostgreSQL and Redis to be reachable. A 503 response
// signals the load balancer to stop routing traffic to this instance until
// dependencies recover.
func buildReadyzHandler(pool *pgxpool.Pool, redisClient *rdb.Client, log *zap.Logger, infra *platformMetrics.InfraMetrics) http.HandlerFunc {
	return buildReadyzHandlerFromCheckers(pool, &redisClientChecker{client: redisClient}, log, infra)
}

// buildReadyzHandlerFromCheckers is the testable form of buildReadyzHandler that accepts
// interface types instead of concrete pgxpool and redis clients.
func buildReadyzHandlerFromCheckers(db dbPinger, rc redisChecker, log *zap.Logger, infra *platformMetrics.InfraMetrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := db.Ping(ctx); err != nil {
			dbStatus = "unavailable"
			log.Warn("readyz: database ping failed", zap.Error(err))
		}

		redisAvailable := rc.IsAvailable(ctx)
		// Update the Redis availability gauge on every readyz call so that the gauge
		// reflects the most recent health check result.
		infra.SetRedisUp(redisAvailable)

		redisStatus := "ok"
		if !redisAvailable {
			redisStatus = "unavailable"
			log.Warn("readyz: redis unavailable")
		}

		if dbStatus == "ok" && redisStatus == "ok" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(readyzResponse{Status: "ready"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(readyzResponse{
			Status: "not_ready",
			DB:     dbStatus,
			Redis:  redisStatus,
		})
	}
}

// dbPoolStats holds connection pool statistics for health reporting.
type dbPoolStats struct {
	Total int32 `json:"total"`
	Idle  int32 `json:"idle"`
	InUse int32 `json:"in_use"`
	Max   int32 `json:"max"`
}

// healthResponse is the JSON body for GET /health.
type healthResponse struct {
	Status string       `json:"status"`
	DB     string       `json:"db"`
	Redis  string       `json:"redis"`
	DBPool *dbPoolStats `json:"db_pool,omitempty"`
}

// pgxpoolChecker wraps *pgxpool.Pool to satisfy the dbChecker interface.
type pgxpoolChecker struct {
	pool *pgxpool.Pool
}

func (p *pgxpoolChecker) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

func (p *pgxpoolChecker) PoolStats() dbPoolStats {
	stat := p.pool.Stat()
	return dbPoolStats{
		Total: stat.TotalConns(),
		Idle:  stat.IdleConns(),
		InUse: stat.AcquiredConns(),
		Max:   stat.MaxConns(),
	}
}

// buildHealthHandler returns the GET /health handler.
// If db is unreachable: status="degraded", HTTP 503.
// If redis is unreachable: status="degraded", HTTP 503 (API continues serving).
// When the DB ping succeeds, db_pool statistics are included (non-blocking read) and
// the four DB pool gauges are updated. The Redis availability gauge is always updated.
func buildHealthHandler(pool *pgxpool.Pool, redisClient *rdb.Client, log *zap.Logger, infra *platformMetrics.InfraMetrics) http.HandlerFunc {
	return buildHealthHandlerFromCheckers(&pgxpoolChecker{pool: pool}, &redisClientChecker{client: redisClient}, log, infra)
}

// buildHealthHandlerFromCheckers is the testable form of buildHealthHandler that accepts
// interface types instead of concrete pgxpool and redis clients.
func buildHealthHandlerFromCheckers(db dbChecker, rc redisChecker, log *zap.Logger, infra *platformMetrics.InfraMetrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		var poolStats *dbPoolStats
		if err := db.Ping(ctx); err != nil {
			dbStatus = "unavailable"
			log.Warn("health: database ping failed", zap.Error(err))
		} else {
			// PoolStats() is a non-blocking in-memory read; safe to call in the hot path.
			stats := db.PoolStats()
			poolStats = &stats
			// Update DB pool gauges. pgxpoolChecker.PoolStats() wraps pool.Stat() which
			// is a non-blocking in-memory read — no I/O, safe in the hot path.
			infra.UpdateDBPool(&dbPoolStatAdapter{stats: stats})
		}

		redisAvailable := rc.IsAvailable(ctx)
		// Update the Redis availability gauge on every health call so the gauge reflects
		// the most recent check result.
		infra.SetRedisUp(redisAvailable)

		redisStatus := "ok"
		if !redisAvailable {
			redisStatus = "unavailable"
			log.Warn("health: redis unavailable")
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
			DBPool: poolStats,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// dbPoolStatAdapter adapts dbPoolStats (the internal health response struct) to
// satisfy the platformMetrics.DBPoolStat interface. This avoids duplicating the
// pool.Stat() call — poolStats is already populated from db.PoolStats() above.
type dbPoolStatAdapter struct {
	stats dbPoolStats
}

func (a *dbPoolStatAdapter) TotalConns() int32    { return a.stats.Total }
func (a *dbPoolStatAdapter) AcquiredConns() int32 { return a.stats.InUse }
func (a *dbPoolStatAdapter) IdleConns() int32     { return a.stats.Idle }
func (a *dbPoolStatAdapter) MaxConns() int32      { return a.stats.Max }

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
