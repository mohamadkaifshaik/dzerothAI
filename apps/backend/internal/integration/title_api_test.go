//go:build integration

// Title HTTP API integration tests.
//
// Validates the Phase 9 title HTTP endpoints against real PostgreSQL with all
// migrations applied. Every test exercises the full chain:
//
//	HTTP request → chi router → JWT middleware → handler → service → title.Repository → PostgreSQL
//
// No database mocking is used.
// Run with: go test -tags integration ./internal/integration/
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
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/follow"
	platformMW "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/middleware"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/title"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/user"
)

// ---------------------------------------------------------------------------
// titleUserPrivacyAdapter — local copy matching cmd/api/main.go
// ---------------------------------------------------------------------------

// titlePrivacyAdapter adapts *user.Service to the title.userPrivacyChecker
// interface. This mirrors the titleUserPrivacyAdapter in cmd/api/main.go.
// Defined here to avoid importing cmd/api from the integration test package.
type titlePrivacyAdapter struct {
	svc *user.Service
}

func (a *titlePrivacyAdapter) IsPrivateAccount(ctx context.Context, userID uuid.UUID) (bool, error) {
	u, err := a.svc.GetProfile(ctx, userID)
	if err != nil {
		return false, err
	}
	return u.IsPrivate, nil
}

// ---------------------------------------------------------------------------
// titleAPIServer — test server with title routes mounted
// ---------------------------------------------------------------------------

// titleTestServer wraps httptest.Server with the title routes and a convenience
// method to issue authenticated requests.
type titleTestServer struct {
	server *httptest.Server
}

func (s *titleTestServer) url(path string) string {
	return s.server.URL + path
}

// buildTitleAPIServer builds a minimal chi router with real dependencies:
//   - title.Repository → real PostgreSQL pool
//   - title.Service → wired with real follow.Service (followChecker) and
//     real user.Service via titlePrivacyAdapter (privacyChecker)
//   - title.Handler → routes registered at /api/v1/titles/*
//
// The server also mounts auth routes so tests can register users and obtain
// real JWT access tokens. Redis is required for auth rate-limit middleware.
func buildTitleAPIServer(t *testing.T) *titleTestServer {
	t.Helper()

	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	log := zap.NewNop()

	// Real repositories / services.
	titleRepo := title.NewRepository(pool)
	titleSvc := title.NewService(titleRepo, log)

	followRepo := follow.NewRepository(pool)
	followSvc := follow.NewService(followRepo, log)
	titleSvc.SetFollowChecker(followSvc)

	userSvc := user.NewService(pool, log)
	titleSvc.SetPrivacyChecker(&titlePrivacyAdapter{svc: userSvc})

	// Handlers.
	titleHandler := title.NewHandler(titleSvc, log)
	authSvc := auth.NewService(pool, testJWTSecret, log)
	authHandler := auth.NewHandler(authSvc, log)

	// Router.
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(platformMW.AccessLog(log))
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(corsAllowAllMW)

	r.Route("/api/v1", func(r chi.Router) {
		// Auth routes — needed to register test users and obtain tokens.
		// Redis is required for the auth rate-limit middleware (fail-closed).
		authHandler.RegisterRoutes(r, redisClient, testJWTSecret)
		// Title routes.
		titleHandler.RegisterRoutes(r, testJWTSecret)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() { srv.Close() })

	return &titleTestServer{server: srv}
}

// registerTitleTestUser registers a new unique user on the title test server
// and returns the access token string.
func registerTitleTestUser(t *testing.T, srv *titleTestServer) (userID uuid.UUID, accessToken string) {
	t.Helper()

	id := uuid.New()
	suffix := id.String()[:8]
	handle := "ttlapi_" + suffix
	email := "ttlapi_" + suffix + "@example.com"
	password := "Password123!"

	body := fmt.Sprintf(
		`{"handle":%q,"display_name":"Title API Test","email":%q,"password":%q}`,
		handle, email, password,
	)
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/register"),
		strings.NewReader(body), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("registerTitleTestUser: want 201, got %d: %s", resp.StatusCode, raw)
	}

	var pair struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		t.Fatalf("registerTitleTestUser: decode: %v", err)
	}
	if pair.AccessToken == "" {
		t.Fatal("registerTitleTestUser: empty access_token")
	}

	// Resolve the user UUID from the token claims via GET /api/v1/me.
	meResp := doJSON(t, http.MethodGet, srv.url("/api/v1/me"),
		nil, map[string]string{"Authorization": "Bearer " + pair.AccessToken})
	defer meResp.Body.Close()

	if meResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(meResp.Body)
		t.Fatalf("registerTitleTestUser: GET /me want 200, got %d: %s", meResp.StatusCode, raw)
	}

	var meBody struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&meBody); err != nil {
		t.Fatalf("registerTitleTestUser: decode /me: %v", err)
	}

	parsed, err := uuid.Parse(meBody.Data.ID)
	if err != nil {
		t.Fatalf("registerTitleTestUser: parse user id %q: %v", meBody.Data.ID, err)
	}

	return parsed, pair.AccessToken
}

// bearerHeader returns an Authorization header map for use with doJSON.
func bearerHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// ---------------------------------------------------------------------------
// 1. TestTitleAPI_GetCatalog
// ---------------------------------------------------------------------------

// TestTitleAPI_GetCatalog verifies that GET /api/v1/titles/catalog returns the
// five active definitions and excludes the Top 1% Creator (is_active=false).
func TestTitleAPI_GetCatalog(t *testing.T) {
	srv := buildTitleAPIServer(t)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/titles/catalog"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetCatalog: want 200, got %d: %s", resp.StatusCode, raw)
	}

	var body struct {
		Items []struct {
			ID          string `json:"id"`
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Category    string `json:"category"`
			IsRevocable bool   `json:"is_revocable"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GetCatalog: decode: %v", err)
	}

	// Exactly 5 active definitions seeded in migration 0014.
	if len(body.Items) != 5 {
		t.Errorf("GetCatalog: len(items) = %d, want 5", len(body.Items))
	}

	// Top 1% Creator must not appear (is_active=false in migration 0014).
	for _, item := range body.Items {
		if item.Slug == "top_1pct_creator" {
			t.Errorf("GetCatalog: top_1pct_creator must not appear in catalog (is_active=false)")
		}
	}

	// All items must have non-empty required fields.
	for _, item := range body.Items {
		if item.ID == "" {
			t.Errorf("GetCatalog: item has empty id")
		}
		if item.Slug == "" {
			t.Errorf("GetCatalog: item has empty slug")
		}
		if item.DisplayName == "" {
			t.Errorf("GetCatalog: item has empty display_name")
		}
		if item.Category == "" {
			t.Errorf("GetCatalog: item has empty category")
		}
	}
}

// ---------------------------------------------------------------------------
// 2. TestTitleAPI_GetMyTitles_Empty
// ---------------------------------------------------------------------------

// TestTitleAPI_GetMyTitles_Empty verifies that GET /api/v1/titles/me returns
// an empty items array for a new user with no earned titles.
// PrimaryID has json tag "primary_id,omitempty" so it must be absent (not null)
// when nil.
func TestTitleAPI_GetMyTitles_Empty(t *testing.T) {
	srv := buildTitleAPIServer(t)
	_, token := registerTitleTestUser(t, srv)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/titles/me"),
		nil, bearerHeader(token))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetMyTitles/empty: want 200, got %d: %s", resp.StatusCode, raw)
	}

	// Decode into a generic map to check exact JSON structure.
	var rawBody map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawBody); err != nil {
		t.Fatalf("GetMyTitles/empty: decode: %v", err)
	}

	// items must be present and be an empty array.
	itemsRaw, ok := rawBody["items"]
	if !ok {
		t.Fatal("GetMyTitles/empty: missing 'items' key")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		t.Fatalf("GetMyTitles/empty: items not an array: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("GetMyTitles/empty: want 0 items, got %d", len(items))
	}

	// primary_id must be absent (omitempty + nil) — not present as null.
	if _, present := rawBody["primary_id"]; present {
		t.Errorf("GetMyTitles/empty: primary_id key must be absent when nil (omitempty), but it is present")
	}
}

// ---------------------------------------------------------------------------
// 3. TestTitleAPI_GetMyTitles_WithTitle
// ---------------------------------------------------------------------------

// TestTitleAPI_GetMyTitles_WithTitle verifies that GET /api/v1/titles/me
// includes a title that was inserted directly into the database.
func TestTitleAPI_GetMyTitles_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	userID, token := registerTitleTestUser(t, srv)

	repo := title.NewRepository(pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/titles/me"),
		nil, bearerHeader(token))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetMyTitles/withTitle: want 200, got %d: %s", resp.StatusCode, raw)
	}

	var body struct {
		Items []struct {
			ID     string `json:"id"`
			Slug   string `json:"slug"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GetMyTitles/withTitle: decode: %v", err)
	}

	if len(body.Items) == 0 {
		t.Fatal("GetMyTitles/withTitle: expected at least 1 item, got 0")
	}

	found := false
	for _, item := range body.Items {
		if item.ID == ut.ID.String() {
			found = true
			if item.Slug != "founding_member" {
				t.Errorf("GetMyTitles/withTitle: slug = %q, want founding_member", item.Slug)
			}
			if item.Status != "active" {
				t.Errorf("GetMyTitles/withTitle: status = %q, want active", item.Status)
			}
		}
	}
	if !found {
		t.Errorf("GetMyTitles/withTitle: user_title id %s not found in response", ut.ID)
	}
}

// ---------------------------------------------------------------------------
// 4. TestTitleAPI_GetMyPrimary_NoneSet
// ---------------------------------------------------------------------------

// TestTitleAPI_GetMyPrimary_NoneSet verifies that GET /api/v1/titles/me/primary
// returns {"primary_title":null} for a user with no primary title set.
func TestTitleAPI_GetMyPrimary_NoneSet(t *testing.T) {
	srv := buildTitleAPIServer(t)
	_, token := registerTitleTestUser(t, srv)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/titles/me/primary"),
		nil, bearerHeader(token))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetMyPrimary/noneSet: want 200, got %d: %s", resp.StatusCode, raw)
	}

	var body struct {
		PrimaryTitle *struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
		} `json:"primary_title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GetMyPrimary/noneSet: decode: %v", err)
	}

	if body.PrimaryTitle != nil {
		t.Errorf("GetMyPrimary/noneSet: primary_title = %+v, want null", body.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// 5. TestTitleAPI_SetPrimary_Success
// ---------------------------------------------------------------------------

// TestTitleAPI_SetPrimary_Success verifies that PUT /api/v1/titles/me/primary
// succeeds when the user owns an active title and returns the slug + display_name.
func TestTitleAPI_SetPrimary_Success(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	userID, token := registerTitleTestUser(t, srv)

	repo := title.NewRepository(pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	body := fmt.Sprintf(`{"user_title_id":%q}`, ut.ID.String())
	resp := doJSON(t, http.MethodPut, srv.url("/api/v1/titles/me/primary"),
		strings.NewReader(body), bearerHeader(token))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("SetPrimary/success: want 200, got %d: %s", resp.StatusCode, raw)
	}

	var respBody struct {
		PrimaryTitle *struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
		} `json:"primary_title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("SetPrimary/success: decode: %v", err)
	}

	if respBody.PrimaryTitle == nil {
		t.Fatal("SetPrimary/success: primary_title is null, want non-null")
	}
	if respBody.PrimaryTitle.Slug != "founding_member" {
		t.Errorf("SetPrimary/success: slug = %q, want founding_member", respBody.PrimaryTitle.Slug)
	}
	if respBody.PrimaryTitle.DisplayName == "" {
		t.Error("SetPrimary/success: display_name is empty")
	}
}

// ---------------------------------------------------------------------------
// 6. TestTitleAPI_SetPrimary_Forbidden
// ---------------------------------------------------------------------------

// TestTitleAPI_SetPrimary_Forbidden verifies that PUT /api/v1/titles/me/primary
// returns 403 when the caller provides a user_title_id that belongs to a
// different user.
func TestTitleAPI_SetPrimary_Forbidden(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	// Owner creates a title.
	ownerID, _ := registerTitleTestUser(t, srv)
	// Caller is a different user.
	_, callerToken := registerTitleTestUser(t, srv)

	repo := title.NewRepository(pool)
	ownerTitle, err := repo.CreateUserTitle(ctx, ownerID, defCenturion)
	if err != nil {
		t.Fatalf("CreateUserTitle for owner: %v", err)
	}

	// Caller attempts to set owner's title as their own primary.
	body := fmt.Sprintf(`{"user_title_id":%q}`, ownerTitle.ID.String())
	resp := doJSON(t, http.MethodPut, srv.url("/api/v1/titles/me/primary"),
		strings.NewReader(body), bearerHeader(callerToken))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("SetPrimary/forbidden: want 403, got %d: %s", resp.StatusCode, raw)
	}
}

// ---------------------------------------------------------------------------
// 7. TestTitleAPI_SetPrimary_RevokedTitle_Forbidden
// ---------------------------------------------------------------------------

// TestTitleAPI_SetPrimary_RevokedTitle_Forbidden verifies that PUT
// /api/v1/titles/me/primary returns 403 when the user_title has status=revoked.
func TestTitleAPI_SetPrimary_RevokedTitle_Forbidden(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	userID, token := registerTitleTestUser(t, srv)

	repo := title.NewRepository(pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// Transition to revoked status directly.
	now := time.Now().UTC()
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:    title.StatusRevoked,
		RevokedAt: &now,
	}); err != nil {
		t.Fatalf("UpdateUserTitleStatus→revoked: %v", err)
	}

	// Attempt to set the revoked title as primary.
	body := fmt.Sprintf(`{"user_title_id":%q}`, ut.ID.String())
	resp := doJSON(t, http.MethodPut, srv.url("/api/v1/titles/me/primary"),
		strings.NewReader(body), bearerHeader(token))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("SetPrimary/revokedForbidden: want 403, got %d: %s", resp.StatusCode, raw)
	}
}

// ---------------------------------------------------------------------------
// 8. TestTitleAPI_ClearPrimary
// ---------------------------------------------------------------------------

// TestTitleAPI_ClearPrimary verifies that DELETE /api/v1/titles/me/primary
// returns 204 and that a subsequent GET returns null.
func TestTitleAPI_ClearPrimary(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	userID, token := registerTitleTestUser(t, srv)

	// Insert and set a primary title.
	repo := title.NewRepository(pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	setBody := fmt.Sprintf(`{"user_title_id":%q}`, ut.ID.String())
	setResp := doJSON(t, http.MethodPut, srv.url("/api/v1/titles/me/primary"),
		strings.NewReader(setBody), bearerHeader(token))
	setResp.Body.Close()
	if setResp.StatusCode != http.StatusOK {
		t.Fatalf("ClearPrimary: set-up PUT want 200, got %d", setResp.StatusCode)
	}

	// Delete primary.
	delResp := doJSON(t, http.MethodDelete, srv.url("/api/v1/titles/me/primary"),
		nil, bearerHeader(token))
	defer delResp.Body.Close()

	if delResp.StatusCode != http.StatusNoContent {
		raw, _ := io.ReadAll(delResp.Body)
		t.Fatalf("ClearPrimary: want 204, got %d: %s", delResp.StatusCode, raw)
	}

	// Verify subsequent GET returns null.
	getResp := doJSON(t, http.MethodGet, srv.url("/api/v1/titles/me/primary"),
		nil, bearerHeader(token))
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(getResp.Body)
		t.Fatalf("ClearPrimary: GET after clear want 200, got %d: %s", getResp.StatusCode, raw)
	}

	var body struct {
		PrimaryTitle *struct{} `json:"primary_title"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&body); err != nil {
		t.Fatalf("ClearPrimary: decode: %v", err)
	}
	if body.PrimaryTitle != nil {
		t.Errorf("ClearPrimary: primary_title = %+v after clear, want null", body.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// 9. TestTitleAPI_ClearPrimary_Idempotent
// ---------------------------------------------------------------------------

// TestTitleAPI_ClearPrimary_Idempotent verifies that DELETE /api/v1/titles/me/primary
// is idempotent: calling it twice both return 204 with no error.
func TestTitleAPI_ClearPrimary_Idempotent(t *testing.T) {
	srv := buildTitleAPIServer(t)
	_, token := registerTitleTestUser(t, srv)

	for i := 0; i < 2; i++ {
		resp := doJSON(t, http.MethodDelete, srv.url("/api/v1/titles/me/primary"),
			nil, bearerHeader(token))
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("ClearPrimary/idempotent: call %d want 204, got %d: %s", i+1, resp.StatusCode, raw)
		}
	}
}

// ---------------------------------------------------------------------------
// 10. TestTitleAPI_GetUserPrimary_PublicAccount
// ---------------------------------------------------------------------------

// TestTitleAPI_GetUserPrimary_PublicAccount verifies that an unauthenticated
// viewer can see the primary title of a public account.
func TestTitleAPI_GetUserPrimary_PublicAccount(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	ownerID, ownerToken := registerTitleTestUser(t, srv)

	// Insert an active title and set it as primary.
	repo := title.NewRepository(pool)
	ut, err := repo.CreateUserTitle(ctx, ownerID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	setBody := fmt.Sprintf(`{"user_title_id":%q}`, ut.ID.String())
	setResp := doJSON(t, http.MethodPut, srv.url("/api/v1/titles/me/primary"),
		strings.NewReader(setBody), bearerHeader(ownerToken))
	setResp.Body.Close()
	if setResp.StatusCode != http.StatusOK {
		t.Fatalf("GetUserPrimary/public: set-up PUT want 200, got %d", setResp.StatusCode)
	}

	// Unauthenticated viewer.
	path := fmt.Sprintf("/api/v1/titles/%s/primary", ownerID)
	resp := doJSON(t, http.MethodGet, srv.url(path), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetUserPrimary/public: want 200, got %d: %s", resp.StatusCode, raw)
	}

	var body struct {
		PrimaryTitle *struct {
			Slug string `json:"slug"`
		} `json:"primary_title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GetUserPrimary/public: decode: %v", err)
	}

	if body.PrimaryTitle == nil {
		t.Fatal("GetUserPrimary/public: primary_title is null, want non-null for public account")
	}
	if body.PrimaryTitle.Slug != "founding_member" {
		t.Errorf("GetUserPrimary/public: slug = %q, want founding_member", body.PrimaryTitle.Slug)
	}
}

// ---------------------------------------------------------------------------
// 11. TestTitleAPI_GetUserPrimary_PrivateAccount_NonFollower
// ---------------------------------------------------------------------------

// TestTitleAPI_GetUserPrimary_PrivateAccount_NonFollower verifies that an
// authenticated non-follower receives HTTP 200 with primary_title:null (not
// 403 or 404) for a private account.
func TestTitleAPI_GetUserPrimary_PrivateAccount_NonFollower(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	ownerID, ownerToken := registerTitleTestUser(t, srv)
	_, viewerToken := registerTitleTestUser(t, srv)

	// Insert title and set primary for owner.
	repo := title.NewRepository(pool)
	ut, err := repo.CreateUserTitle(ctx, ownerID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	setBody := fmt.Sprintf(`{"user_title_id":%q}`, ut.ID.String())
	setResp := doJSON(t, http.MethodPut, srv.url("/api/v1/titles/me/primary"),
		strings.NewReader(setBody), bearerHeader(ownerToken))
	setResp.Body.Close()

	// Make owner's account private via SQL (mirrors cmd/api titlePrivacyAdapter).
	_, err = pool.Exec(ctx,
		`UPDATE users SET is_private = TRUE WHERE id = $1`, ownerID)
	if err != nil {
		t.Fatalf("set is_private: %v", err)
	}

	// Authenticated non-follower requests the owner's primary title.
	path := fmt.Sprintf("/api/v1/titles/%s/primary", ownerID)
	resp := doJSON(t, http.MethodGet, srv.url(path), nil, bearerHeader(viewerToken))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetUserPrimary/privateNonFollower: want 200, got %d: %s", resp.StatusCode, raw)
	}

	var body struct {
		PrimaryTitle *struct {
			Slug string `json:"slug"`
		} `json:"primary_title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GetUserPrimary/privateNonFollower: decode: %v", err)
	}

	// Non-follower must receive null (title is hidden for private accounts).
	if body.PrimaryTitle != nil {
		t.Errorf("GetUserPrimary/privateNonFollower: primary_title = %+v, want null for non-follower of private account",
			body.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// 12. TestTitleAPI_GetUserPrimary_PrivateAccount_Owner
// ---------------------------------------------------------------------------

// TestTitleAPI_GetUserPrimary_PrivateAccount_Owner verifies that the account
// owner always sees their own primary title, even on a private account.
func TestTitleAPI_GetUserPrimary_PrivateAccount_Owner(t *testing.T) {
	pool := connectTestDB(t)
	srv := buildTitleAPIServer(t)
	ctx := context.Background()

	ownerID, ownerToken := registerTitleTestUser(t, srv)

	// Insert and set primary.
	repo := title.NewRepository(pool)
	ut, err := repo.CreateUserTitle(ctx, ownerID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	setBody := fmt.Sprintf(`{"user_title_id":%q}`, ut.ID.String())
	setResp := doJSON(t, http.MethodPut, srv.url("/api/v1/titles/me/primary"),
		strings.NewReader(setBody), bearerHeader(ownerToken))
	setResp.Body.Close()

	// Make owner's account private.
	_, err = pool.Exec(ctx,
		`UPDATE users SET is_private = TRUE WHERE id = $1`, ownerID)
	if err != nil {
		t.Fatalf("set is_private: %v", err)
	}

	// Owner requests their own primary title via the /{userID}/primary endpoint.
	path := fmt.Sprintf("/api/v1/titles/%s/primary", ownerID)
	resp := doJSON(t, http.MethodGet, srv.url(path), nil, bearerHeader(ownerToken))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetUserPrimary/privateOwner: want 200, got %d: %s", resp.StatusCode, raw)
	}

	var body struct {
		PrimaryTitle *struct {
			Slug string `json:"slug"`
		} `json:"primary_title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GetUserPrimary/privateOwner: decode: %v", err)
	}

	if body.PrimaryTitle == nil {
		t.Fatal("GetUserPrimary/privateOwner: primary_title is null, want non-null for owner of private account")
	}
	if body.PrimaryTitle.Slug != "founding_member" {
		t.Errorf("GetUserPrimary/privateOwner: slug = %q, want founding_member", body.PrimaryTitle.Slug)
	}
}

// ---------------------------------------------------------------------------
// 13. TestTitleAPI_Unauthenticated_MeEndpoints
// ---------------------------------------------------------------------------

// TestTitleAPI_Unauthenticated_MeEndpoints verifies that all four /titles/me*
// routes return 401 when no Authorization header is provided.
func TestTitleAPI_Unauthenticated_MeEndpoints(t *testing.T) {
	srv := buildTitleAPIServer(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/titles/me"},
		{http.MethodGet, "/api/v1/titles/me/primary"},
		{http.MethodPut, "/api/v1/titles/me/primary"},
		{http.MethodDelete, "/api/v1/titles/me/primary"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := doJSON(t, tc.method, srv.url(tc.path), nil, nil)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				raw, _ := io.ReadAll(resp.Body)
				t.Errorf("%s %s: want 401, got %d: %s", tc.method, tc.path, resp.StatusCode, raw)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 14. TestTitleAPI_InvalidUUID_Returns400
// ---------------------------------------------------------------------------

// TestTitleAPI_InvalidUUID_Returns400 verifies that GET /titles/not-a-uuid/primary
// returns 400 Bad Request due to UUID parse failure.
func TestTitleAPI_InvalidUUID_Returns400(t *testing.T) {
	srv := buildTitleAPIServer(t)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/titles/not-a-uuid/primary"),
		nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("InvalidUUID: want 400, got %d: %s", resp.StatusCode, raw)
	}
}
