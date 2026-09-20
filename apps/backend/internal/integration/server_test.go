//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
	platformMW "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/middleware"
	platformRedis "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/redis"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/user"
)

// testJWTSecret is the JWT signing key used by all integration test servers.
// It satisfies the 32-byte minimum enforced by config.Load.
var testJWTSecret = []byte("test-jwt-secret-for-integration-tests-32b!")

// testAPIServer bundles the running httptest.Server with its logger observer
// so tests can assert on log output as well as HTTP responses.
type testAPIServer struct {
	server    *httptest.Server
	logs      *observer.ObservedLogs
	jwtSecret []byte
}

// url returns the full URL for a given path on the test server.
func (s *testAPIServer) url(path string) string {
	return s.server.URL + path
}

// buildTestAPIServer builds a real chi router with the auth and user handlers,
// real middleware (RequestID, AccessLog, RealIP), and a real httptest.Server.
//
// The test server uses:
//   - A fresh Prometheus registry per server (avoids DefaultRegisterer conflicts).
//   - An observer-based zap logger so tests can assert on log entries.
//   - corsAllowAll behavior when corsOrigins is nil/empty (test environment).
//   - corsAllowList behavior when corsOrigins is non-empty.
//
// Only auth and user routes are mounted; other feature routes are not needed
// for Phase 8D-2 API integration tests.
func buildTestAPIServer(t *testing.T, pool *pgxpool.Pool, redisClient *rdb.Client, corsOrigins []string) *testAPIServer {
	t.Helper()

	// Observer-based logger so tests can inspect log entries.
	core, logs := observer.New(zapcore.DebugLevel)
	log := zap.New(core)

	// Fresh Prometheus registry per test server — avoids DefaultRegisterer conflicts
	// when multiple test servers are created in the same process.
	reg := prometheus.NewRegistry()
	httpMetrics := platformMetrics.New(reg)
	eventMetrics := platformMetrics.NewEvents(reg)
	infraMetrics := platformMetrics.NewInfraMetrics(reg, log)
	platformMetrics.RegisterBuildInfo(reg, "test", "test-commit", "test-build-time")

	// ── Services ──────────────────────────────────────────────────────────────
	authSvc := auth.NewService(pool, testJWTSecret, log)
	authSvc.SetEvents(eventMetrics)

	userSvc := user.NewService(pool, log)
	// sessionRevoker allows DELETE /me/account to revoke sessions.
	userSvc.SetSessionRevoker(authSvc)

	// ── Handlers ──────────────────────────────────────────────────────────────
	authHandler := auth.NewHandler(authSvc, log)
	authHandler.SetEvents(eventMetrics)

	userHandler := user.NewHandler(userSvc, log)
	// blockChecker is not injected; GET /me does not require it.

	// ── Router ────────────────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Standard middleware chain matching the production order in cmd/api/main.go.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(platformMW.AccessLog(log))
	r.Use(chimw.Recoverer)
	r.Use(platformMW.SecurityHeaders)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(httpMetrics.Middleware)

	// CORS: allow-all for test environment (empty origins); allowlist otherwise.
	if len(corsOrigins) == 0 {
		r.Use(corsAllowAllMW)
	} else {
		r.Use(corsAllowListMW(corsOrigins))
	}

	// Public health endpoint.
	r.Get("/health", buildTestHealthHandler(pool, redisClient, log, infraMetrics))

	// Admin-equivalent endpoints — served on the main test server for simplicity.
	r.Get("/livez", buildTestLivezHandler())
	r.Get("/readyz", buildTestReadyzHandler(pool, redisClient, log, infraMetrics))

	// API v1 routes.
	r.Route("/api/v1", func(r chi.Router) {
		authHandler.RegisterRoutes(r, redisClient, testJWTSecret)
		userHandler.RegisterRoutes(r, testJWTSecret)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() { srv.Close() })

	return &testAPIServer{
		server:    srv,
		logs:      logs,
		jwtSecret: testJWTSecret,
	}
}

// ---------------------------------------------------------------------------
// CORS middleware helpers
// ---------------------------------------------------------------------------
//
// These inline implementations mirror the production CORS behavior in
// cmd/api/main.go. The production CORS middleware is already thoroughly
// tested at unit level in cmd/api/cors_test.go; these helpers exist only
// to allow integration tests to exercise CORS behavior through the real server.
// They are intentionally not exported (test package only).

// corsAllowAllMW permits all origins. Mirrors cmd/api.corsAllowAll.
func corsAllowAllMW(next http.Handler) http.Handler {
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

// corsAllowListMW returns a middleware that enforces an explicit origin allowlist.
// Mirrors cmd/api.corsAllowList.
func corsAllowListMW(origins []string) func(http.Handler) http.Handler {
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
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowedSet[origin]; !ok {
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", allowMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Health handler helpers
// ---------------------------------------------------------------------------
//
// These inline handlers mirror the production health handlers in
// cmd/api/main.go. The production handlers are in package main and not
// exported; these are minimal equivalent implementations for integration testing.

type testHealthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
	Redis  string `json:"redis"`
}

type testLivezResponse struct {
	Status string `json:"status"`
}

type testReadyzResponse struct {
	Status string `json:"status"`
	DB     string `json:"db,omitempty"`
	Redis  string `json:"redis,omitempty"`
}

func buildTestLivezHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(testLivezResponse{Status: "ok"})
	}
}

func buildTestReadyzHandler(pool *pgxpool.Pool, redisClient *rdb.Client, log *zap.Logger, infra *platformMetrics.InfraMetrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unavailable"
			log.Warn("readyz: database ping failed", zap.Error(err))
		}

		redisOK := platformRedis.IsAvailable(ctx, redisClient)
		infra.SetRedisUp(redisOK)

		redisStatus := "ok"
		if !redisOK {
			redisStatus = "unavailable"
		}

		if dbStatus == "ok" && redisStatus == "ok" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(testReadyzResponse{Status: "ready"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(testReadyzResponse{Status: "not_ready"})
	}
}

func buildTestHealthHandler(pool *pgxpool.Pool, redisClient *rdb.Client, log *zap.Logger, infra *platformMetrics.InfraMetrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unavailable"
			log.Warn("health: database ping failed", zap.Error(err))
		}

		redisOK := platformRedis.IsAvailable(ctx, redisClient)
		infra.SetRedisUp(redisOK)

		redisStatus := "ok"
		if !redisOK {
			redisStatus = "unavailable"
		}

		status := "ok"
		httpStatus := http.StatusOK
		if dbStatus != "ok" || redisStatus != "ok" {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(testHealthResponse{
			Status: status,
			DB:     dbStatus,
			Redis:  redisStatus,
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP client helpers
// ---------------------------------------------------------------------------

// apiTokenPair holds the tokens returned by register/login/refresh endpoints.
// Fields are kept in memory only — never printed or logged.
type apiTokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// parseTokenPair decodes an auth endpoint response body into apiTokenPair.
func parseTokenPair(t *testing.T, body io.Reader) *apiTokenPair {
	t.Helper()
	var pair apiTokenPair
	if err := json.NewDecoder(body).Decode(&pair); err != nil {
		t.Fatalf("parseTokenPair: decode: %v", err)
	}
	if pair.AccessToken == "" {
		t.Fatal("parseTokenPair: access_token is empty")
	}
	if pair.RefreshToken == "" {
		t.Fatal("parseTokenPair: refresh_token is empty")
	}
	return &pair
}

// apiErrorCode parses an apierror envelope and returns the "code" field.
func apiErrorCode(t *testing.T, body io.Reader) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("apiErrorCode: decode: %v", err)
	}
	return resp.Error.Code
}

// doJSON issues an HTTP request with a JSON body (if provided) and returns the response.
// The caller is responsible for closing resp.Body.
func doJSON(t *testing.T, method, url string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("doJSON: new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("doJSON: do: %v", err)
	}
	return resp
}

// registerTestUser registers a new unique user on the test server and returns
// the token pair. Credentials are derived from a UUID to guarantee uniqueness
// across all tests sharing the same database, regardless of test name prefixes.
func registerTestUser(t *testing.T, srv *testAPIServer) *apiTokenPair {
	t.Helper()

	id := uuid.New().String()[:8]
	// Ensure handle is 3–50 chars, starts/ends with alphanumeric.
	handle := "u" + id
	email := "apitest_" + id + "@example.com"
	password := "Password123!"

	body := fmt.Sprintf(
		`{"handle":%q,"display_name":"API Test User","email":%q,"password":%q}`,
		handle, email, password,
	)

	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/register"),
		strings.NewReader(body), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("register: want 201, got %d: %s", resp.StatusCode, bodyBytes)
	}

	return parseTokenPair(t, resp.Body)
}
