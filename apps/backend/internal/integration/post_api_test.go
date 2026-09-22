//go:build integration

// Post HTTP API integration tests.
//
// Validates the POST /api/v1/posts endpoint and share delay enforcement
// against real PostgreSQL and Redis. Every test exercises the full chain:
//
//	HTTP request → chi router → JWT middleware → post.Handler → post.Service → PostgreSQL
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
	"github.com/prometheus/client_golang/prometheus"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/block"
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
	platformMW "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/middleware"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/user"
)

// ---------------------------------------------------------------------------
// postTestServer — minimal server with auth + post routes
// ---------------------------------------------------------------------------

type postTestServer struct {
	server    *httptest.Server
	jwtSecret []byte
}

func (s *postTestServer) url(path string) string {
	return s.server.URL + path
}

func buildPostAPIServer(t *testing.T, redisClient *rdb.Client) *postTestServer {
	t.Helper()

	pool := connectTestDB(t)
	log := zap.NewNop()

	reg := prometheus.NewRegistry()
	eventMetrics := platformMetrics.NewEvents(reg)

	authSvc := auth.NewService(pool, testJWTSecret, log)
	authSvc.SetEvents(eventMetrics)
	userSvc := user.NewService(pool, log)
	userSvc.SetSessionRevoker(authSvc)

	postRepo := post.NewRepository(pool)
	postSvc := post.NewService(postRepo, log)

	blockRepo := block.NewRepository(pool)
	blockSvc := block.NewService(blockRepo, log)
	postSvc.SetBlockProvider(blockSvc)

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
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() { srv.Close() })

	return &postTestServer{server: srv, jwtSecret: testJWTSecret}
}

// registerAndGetToken registers a unique test user and returns their access token.
func registerAndGetToken(t *testing.T, srv *postTestServer) string {
	t.Helper()

	id := uuid.New().String()[:8]
	handle := "p" + id
	email := "post_" + id + "@example.com"
	password := "Password123!"

	body := fmt.Sprintf(
		`{"handle":%q,"display_name":"Post Tester","email":%q,"password":%q}`,
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

// TestPostAPI_CreateOriginalPost verifies that an authenticated user can
// create an original post and receives 201 with a post containing the author.
func TestPostAPI_CreateOriginalPost(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	body := `{"post_type":"original","content":"Hello integration test world"}`
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(body),
		map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create post: want 201, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Post struct {
			ID       string `json:"id"`
			PostType string `json:"post_type"`
			Author   struct {
				ID string `json:"ID"`
			} `json:"author"`
		} `json:"post"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Post.PostType != "original" {
		t.Errorf("post_type = %q, want %q", result.Post.PostType, "original")
	}
	if result.Post.ID == "" {
		t.Error("post.id is empty")
	}
}

// TestPostAPI_UnauthenticatedCreate verifies that unauthenticated post
// creation returns 401.
func TestPostAPI_UnauthenticatedCreate(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)

	body := `{"post_type":"original","content":"No auth post"}`
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(body), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// TestPostAPI_CreateQuote_ValidTiming verifies that a quote post with valid
// share_initiated_at (6s ago) and 5+ distinct words returns 201.
func TestPostAPI_CreateQuote_ValidTiming(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	// First create an original post to quote.
	origBody := `{"post_type":"original","content":"This is the original post to quote"}`
	origResp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(origBody),
		map[string]string{"Authorization": "Bearer " + token})
	defer origResp.Body.Close()
	if origResp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(origResp.Body)
		t.Fatalf("create original: want 201, got %d: %s", origResp.StatusCode, b)
	}
	var origResult struct {
		Post struct {
			ID string `json:"id"`
		} `json:"post"`
	}
	if err := json.NewDecoder(origResp.Body).Decode(&origResult); err != nil {
		t.Fatalf("decode original post: %v", err)
	}

	// Now create a quote with valid timing.
	initiatedAt := time.Now().UTC().Add(-6 * time.Second).Format(time.RFC3339)
	quoteBody := fmt.Sprintf(
		`{"post_type":"quote","content":"one two three four five","quoted_post_id":%q,"share_initiated_at":%q}`,
		origResult.Post.ID, initiatedAt,
	)
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(quoteBody),
		map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create quote: want 201, got %d: %s", resp.StatusCode, b)
	}
}

// TestPostAPI_CreateQuote_TimingViolation verifies that a quote post with
// share_initiated_at 3 seconds ago returns 400 (timing violation).
func TestPostAPI_CreateQuote_TimingViolation(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	initiatedAt := time.Now().UTC().Add(-3 * time.Second).Format(time.RFC3339)
	quotedID := uuid.New().String()
	body := fmt.Sprintf(
		`{"post_type":"quote","content":"one two three four five","quoted_post_id":%q,"share_initiated_at":%q}`,
		quotedID, initiatedAt,
	)
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(body),
		map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400 (timing violation), got %d: %s", resp.StatusCode, b)
	}
	code := apiErrorCode(t, resp.Body)
	if code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", code)
	}
}

// TestPostAPI_CreateQuote_MissingShareInitiatedAt verifies that a quote post
// without share_initiated_at returns 400.
func TestPostAPI_CreateQuote_MissingShareInitiatedAt(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	quotedID := uuid.New().String()
	body := fmt.Sprintf(
		`{"post_type":"quote","content":"one two three four five","quoted_post_id":%q}`,
		quotedID,
	)
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(body),
		map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400 (missing share_initiated_at), got %d: %s", resp.StatusCode, b)
	}
	code := apiErrorCode(t, resp.Body)
	if code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", code)
	}
}

// TestPostAPI_CreateQuote_WordCountViolation verifies that a quote post with
// valid timing but fewer than 5 distinct words returns 400.
func TestPostAPI_CreateQuote_WordCountViolation(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	// Valid timing, but only 4 distinct words.
	initiatedAt := time.Now().UTC().Add(-6 * time.Second).Format(time.RFC3339)
	quotedID := uuid.New().String()
	body := fmt.Sprintf(
		`{"post_type":"quote","content":"one two three four","quoted_post_id":%q,"share_initiated_at":%q}`,
		quotedID, initiatedAt,
	)
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(body),
		map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400 (word count violation), got %d: %s", resp.StatusCode, b)
	}
	code := apiErrorCode(t, resp.Body)
	if code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", code)
	}
}

// TestPostAPI_CreateRepost_ValidTiming verifies that a repost with valid
// share_initiated_at (6s ago) returns 201.
func TestPostAPI_CreateRepost_ValidTiming(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	// Create an original post to repost.
	origBody := `{"post_type":"original","content":"Original post for repost test"}`
	origResp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(origBody),
		map[string]string{"Authorization": "Bearer " + token})
	defer origResp.Body.Close()
	if origResp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(origResp.Body)
		t.Fatalf("create original: want 201, got %d: %s", origResp.StatusCode, b)
	}
	var origResult struct {
		Post struct {
			ID string `json:"id"`
		} `json:"post"`
	}
	if err := json.NewDecoder(origResp.Body).Decode(&origResult); err != nil {
		t.Fatalf("decode original post: %v", err)
	}

	initiatedAt := time.Now().UTC().Add(-6 * time.Second).Format(time.RFC3339)
	repostBody := fmt.Sprintf(
		`{"post_type":"repost","quoted_post_id":%q,"share_initiated_at":%q}`,
		origResult.Post.ID, initiatedAt,
	)
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(repostBody),
		map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create repost: want 201, got %d: %s", resp.StatusCode, b)
	}
}

// TestPostAPI_CreateRepost_InvalidTiming verifies that a repost with
// share_initiated_at 3 seconds ago returns 400.
func TestPostAPI_CreateRepost_InvalidTiming(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	initiatedAt := time.Now().UTC().Add(-3 * time.Second).Format(time.RFC3339)
	quotedID := uuid.New().String()
	body := fmt.Sprintf(
		`{"post_type":"repost","quoted_post_id":%q,"share_initiated_at":%q}`,
		quotedID, initiatedAt,
	)
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/posts"),
		strings.NewReader(body),
		map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 400 (timing violation), got %d: %s", resp.StatusCode, b)
	}
	code := apiErrorCode(t, resp.Body)
	if code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", code)
	}
}
