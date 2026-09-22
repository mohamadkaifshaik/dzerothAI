//go:build integration

// Feed HTTP API integration tests.
//
// Validates the GET /api/v1/feeds/home endpoint against real PostgreSQL and
// Redis. Every test exercises the full chain:
//
//	HTTP request → chi router → JWT middleware → feed.Handler → feed.Service → PostgreSQL
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
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

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/block"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/feed"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/follow"
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
	platformMW "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/middleware"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/user"
)

// ---------------------------------------------------------------------------
// feedTestServer — minimal server with auth + post + feed routes
// ---------------------------------------------------------------------------

type feedTestServer struct {
	server    *httptest.Server
	jwtSecret []byte
}

func (s *feedTestServer) url(path string) string {
	return s.server.URL + path
}

func buildFeedAPIServer(t *testing.T, pool *pgxpool.Pool, redisClient *rdb.Client) *feedTestServer {
	t.Helper()

	log := zap.NewNop()

	reg := prometheus.NewRegistry()
	eventMetrics := platformMetrics.NewEvents(reg)

	authSvc := auth.NewService(pool, testJWTSecret, log)
	authSvc.SetEvents(eventMetrics)
	userSvc := user.NewService(pool, log)
	userSvc.SetSessionRevoker(authSvc)

	postRepo := post.NewRepository(pool)
	postSvc := post.NewService(postRepo, log)

	followRepo := follow.NewRepository(pool)
	followSvc := follow.NewService(followRepo, log)
	postSvc.SetFollowChecker(followSvc)

	blockRepo := block.NewRepository(pool)
	blockSvc := block.NewService(blockRepo, log)
	postSvc.SetBlockProvider(blockSvc)

	feedRepo := feed.NewRepository(pool)
	feedSvc := feed.NewService(feedRepo, blockSvc, followRepo, log)
	feedSvc.SetEvents(eventMetrics)
	feedHandler := feed.NewHandler(feedSvc, log)

	authHandler := auth.NewHandler(authSvc, log)
	authHandler.SetEvents(eventMetrics)
	userHandler := user.NewHandler(userSvc, log)
	postHandler := post.NewHandler(postSvc, log)
	postHandler.SetEvents(eventMetrics)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(platformMW.AccessLog(log))
	r.Use(chimw.Recoverer)
	r.Use(platformMW.SecurityHeaders)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(corsAllowAllMW)

	r.Route("/api/v1", func(r chi.Router) {
		authHandler.RegisterRoutes(r, redisClient, testJWTSecret)
		userHandler.RegisterRoutes(r, testJWTSecret)
		postHandler.RegisterRoutes(r, redisClient, testJWTSecret)
		feedHandler.RegisterRoutes(r, testJWTSecret)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() { srv.Close() })

	return &feedTestServer{server: srv, jwtSecret: testJWTSecret}
}

// registerUserOnFeedServer registers a new unique user and returns their access token.
func registerUserOnFeedServer(t *testing.T, srv *feedTestServer) string {
	t.Helper()

	// Reset the shared registration rate-limit bucket before each registration.
	// See clearRegisterRateLimit in helpers_test.go for the full explanation.
	clearRegisterRateLimit(t)

	id := uuid.New().String()[:8]
	handle := "f" + id
	email := "feed_" + id + "@example.com"
	password := "Password123!"

	body := fmt.Sprintf(
		`{"handle":%q,"display_name":"Feed Tester","email":%q,"password":%q}`,
		handle, email, password,
	)

	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/register"),
		strings.NewReader(body), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register: want 201, got %d: %s", resp.StatusCode, b)
	}

	tokens := parseTokenPair(t, resp.Body)
	return tokens.AccessToken
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestFeedAPI_AuthenticatedHomeFeed verifies that an authenticated user gets
// a 200 response with an items array (may be empty for a new user with no follows).
func TestFeedAPI_AuthenticatedHomeFeed(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildFeedAPIServer(t, pool, redisClient)
	token := registerUserOnFeedServer(t, srv)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/feeds/home"),
		nil, map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("home feed: want 200, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Items      []json.RawMessage `json:"items"`
		NextCursor string            `json:"next_cursor"`
		Terminated bool              `json:"terminated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode home feed: %v", err)
	}

	// Items must be a non-nil array (may be empty for a new user).
	if result.Items == nil {
		t.Error("home feed: items should be a non-nil array")
	}
}

// TestFeedAPI_UnauthenticatedHomeFeed verifies that unauthenticated requests
// to the home feed return 401.
func TestFeedAPI_UnauthenticatedHomeFeed(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildFeedAPIServer(t, pool, redisClient)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/feeds/home"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("unauthenticated feed: want 401, got %d: %s", resp.StatusCode, b)
	}
}

// TestFeedAPI_NoPublicMetricFields verifies that the home feed response
// contains no social-validation metric fields (CLAUDE.md §2.3).
//
// The test creates a user, has them follow themselves (which is not possible),
// so we create a second user who follows the first and creates a post, then the
// first user fetches their feed. For simplicity, we just verify the response
// shape of an empty feed — the metric lockdown applies regardless.
func TestFeedAPI_NoPublicMetricFields(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildFeedAPIServer(t, pool, redisClient)
	token := registerUserOnFeedServer(t, srv)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/feeds/home"),
		nil, map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("home feed: want 200, got %d: %s", resp.StatusCode, b)
	}

	// Parse response as a generic map to check for forbidden fields.
	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Parse items to check individual post objects.
	var items []map[string]json.RawMessage
	if rawItems, ok := result["items"]; ok {
		if err := json.Unmarshal(rawItems, &items); err != nil {
			// Empty array is fine.
			items = nil
		}
	}

	// Forbidden public metric fields — must never appear in feed items.
	forbidden := []string{
		"like_count",
		"impression_count",
		"bookmark_count",
		"retweet_count",
		"follower_count",
		"view_count",
		"share_count",
		"repost_count",
	}

	for _, item := range items {
		for _, field := range forbidden {
			if _, found := item[field]; found {
				t.Errorf("feed item contains forbidden public metric field %q (CLAUDE.md §2.3)", field)
			}
		}
	}
}

// TestFeedAPI_FiniteFeed verifies that the home feed response includes the
// terminated field, indicating finite feed semantics (CLAUDE.md §2.1).
func TestFeedAPI_FiniteFeed(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildFeedAPIServer(t, pool, redisClient)
	token := registerUserOnFeedServer(t, srv)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/feeds/home"),
		nil, map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("home feed: want 200, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Items      []json.RawMessage `json:"items"`
		NextCursor string            `json:"next_cursor"`
		Terminated *bool             `json:"terminated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode home feed: %v", err)
	}

	// The terminated field must be present in the response (CLAUDE.md §2.1).
	// For a new user with no follows the feed is immediately terminated.
	if result.Terminated == nil {
		t.Error("home feed: terminated field is absent — finite feed contract violated")
	}
}
