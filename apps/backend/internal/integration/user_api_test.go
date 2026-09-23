//go:build integration

// User profile and follow-list HTTP API integration tests.
//
// Validates GET /api/v1/users/{id}, GET /api/v1/users/{id}/following and
// GET /api/v1/users/{id}/followers against real PostgreSQL and Redis, using
// the shared postTestServer (block-aware profile lookups, follow and block
// routes wired as in cmd/api/main.go).
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// forbiddenPublicMetricFields are the social-validation metric keys that must
// never appear in public responses (CLAUDE.md §2.3). Same list as the feed
// and hashtag API tests.
var forbiddenPublicMetricFields = []string{
	"like_count",
	"impression_count",
	"bookmark_count",
	"retweet_count",
	"follower_count",
	"view_count",
	"share_count",
	"repost_count",
}

// assertNoForbiddenMetricFields fails the test if obj contains any forbidden
// public metric key.
func assertNoForbiddenMetricFields(t *testing.T, where string, obj map[string]json.RawMessage) {
	t.Helper()
	for _, field := range forbiddenPublicMetricFields {
		if _, found := obj[field]; found {
			t.Errorf("%s contains forbidden public metric field %q (CLAUDE.md §2.3)", where, field)
		}
	}
}

// registerUserWithID registers a unique user via registerAndGetToken and
// resolves its id and handle through GET /api/v1/me.
func registerUserWithID(t *testing.T, srv *postTestServer) (id uuid.UUID, handle, token string) {
	t.Helper()

	token = registerAndGetToken(t, srv)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/me"), nil, bearerHeader(token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /me: want 200, got %d: %s", resp.StatusCode, b)
	}

	var me struct {
		Data struct {
			ID     string `json:"id"`
			Handle string `json:"handle"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatalf("decode /me: %v", err)
	}
	parsed, err := uuid.Parse(me.Data.ID)
	if err != nil {
		t.Fatalf("parse /me id %q: %v", me.Data.ID, err)
	}
	return parsed, me.Data.Handle, token
}

// doAuthedExpectNoContent issues an authenticated request and requires 204.
func doAuthedExpectNoContent(t *testing.T, srv *postTestServer, method, path, token string) {
	t.Helper()
	resp := doJSON(t, method, srv.url(path), nil, bearerHeader(token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: want 204, got %d: %s", method, path, resp.StatusCode, b)
	}
}

// getUserProfile issues GET /api/v1/users/{id} as the given caller.
func getUserProfile(t *testing.T, srv *postTestServer, token string, id uuid.UUID) *http.Response {
	t.Helper()
	return doJSON(t, http.MethodGet, srv.url("/api/v1/users/"+id.String()), nil, bearerHeader(token))
}

// requireUserNotFound asserts a 404 with the standard NOT_FOUND envelope and
// the same message used for a genuinely unknown user, so block state is not
// distinguishable from non-existence.
func requireUserNotFound(t *testing.T, resp *http.Response, context string) {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: want 404, got %d: %s", context, resp.StatusCode, b)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("%s: decode error body: %v", context, err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Errorf("%s: error code = %q, want NOT_FOUND", context, body.Error.Code)
	}
	if body.Error.Message != "User not found." {
		t.Errorf("%s: error message = %q, want %q", context, body.Error.Message, "User not found.")
	}
}

// ---------------------------------------------------------------------------
// GET /users/{id}
// ---------------------------------------------------------------------------

// TestUserAPI_GetUserByID_Success verifies that an authenticated caller can
// fetch another user's public profile and that it carries no public metrics.
func TestUserAPI_GetUserByID_Success(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	_, _, viewerToken := registerUserWithID(t, srv)
	targetID, targetHandle, _ := registerUserWithID(t, srv)

	resp := getUserProfile(t, srv, viewerToken, targetID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /users/{id}: want 200, got %d: %s", resp.StatusCode, b)
	}

	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if envelope.Data == nil {
		t.Fatal("profile response: data object is absent")
	}

	var id, handle string
	if err := json.Unmarshal(envelope.Data["id"], &id); err != nil {
		t.Fatalf("decode data.id: %v", err)
	}
	if err := json.Unmarshal(envelope.Data["handle"], &handle); err != nil {
		t.Fatalf("decode data.handle: %v", err)
	}
	if id != targetID.String() {
		t.Errorf("data.id = %q, want %q", id, targetID)
	}
	if handle != targetHandle {
		t.Errorf("data.handle = %q, want %q", handle, targetHandle)
	}
	assertNoForbiddenMetricFields(t, "public profile", envelope.Data)
}

// TestUserAPI_GetUserByID_Unknown_404 verifies that a well-formed but unknown
// user id returns 404 NOT_FOUND.
func TestUserAPI_GetUserByID_Unknown_404(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	_, _, token := registerUserWithID(t, srv)

	requireUserNotFound(t, getUserProfile(t, srv, token, uuid.New()), "unknown user")
}

// TestUserAPI_GetUserByID_ViewerBlockedByTarget_404 verifies that when the
// target has blocked the viewer, the viewer receives the same 404 as for an
// unknown user (block state is never revealed).
func TestUserAPI_GetUserByID_ViewerBlockedByTarget_404(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	targetID, _, targetToken := registerUserWithID(t, srv)
	viewerID, _, viewerToken := registerUserWithID(t, srv)

	doAuthedExpectNoContent(t, srv, http.MethodPost, "/api/v1/users/"+viewerID.String()+"/block", targetToken)

	requireUserNotFound(t, getUserProfile(t, srv, viewerToken, targetID), "viewer blocked by target")
}

// TestUserAPI_GetUserByID_TargetBlockedByViewer_404 verifies that when the
// viewer has blocked the target, the viewer also receives the unknown-user 404.
func TestUserAPI_GetUserByID_TargetBlockedByViewer_404(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	targetID, _, _ := registerUserWithID(t, srv)
	_, _, viewerToken := registerUserWithID(t, srv)

	doAuthedExpectNoContent(t, srv, http.MethodPost, "/api/v1/users/"+targetID.String()+"/block", viewerToken)

	requireUserNotFound(t, getUserProfile(t, srv, viewerToken, targetID), "target blocked by viewer")
}

// ---------------------------------------------------------------------------
// GET /users/{id}/following and /users/{id}/followers
// ---------------------------------------------------------------------------

// followPageResponse mirrors the follow.FollowPage JSON envelope.
type followPageResponse struct {
	Items      []map[string]json.RawMessage `json:"items"`
	NextCursor string                       `json:"next_cursor"`
	Terminated *bool                        `json:"terminated"`
}

// getFollowPage fetches a following/followers page and requires 200.
func getFollowPage(t *testing.T, srv *postTestServer, token, path string) followPageResponse {
	t.Helper()
	resp := doJSON(t, http.MethodGet, srv.url(path), nil, bearerHeader(token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: want 200, got %d: %s", path, resp.StatusCode, b)
	}
	var page followPageResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return page
}

// requireSingleFollowUser asserts a terminated single-item follow page whose
// item is the expected user, with the documented fields and no metrics.
func requireSingleFollowUser(t *testing.T, where string, page followPageResponse, wantID uuid.UUID, wantHandle string) {
	t.Helper()
	if len(page.Items) != 1 {
		t.Fatalf("%s: want exactly 1 item, got %d", where, len(page.Items))
	}
	item := page.Items[0]
	var id, handle string
	if err := json.Unmarshal(item["id"], &id); err != nil {
		t.Fatalf("%s: decode item id: %v", where, err)
	}
	if err := json.Unmarshal(item["handle"], &handle); err != nil {
		t.Fatalf("%s: decode item handle: %v", where, err)
	}
	if id != wantID.String() {
		t.Errorf("%s: item id = %q, want %q", where, id, wantID)
	}
	if handle != wantHandle {
		t.Errorf("%s: item handle = %q, want %q", where, handle, wantHandle)
	}
	for _, key := range []string{"display_name", "avatar_url"} {
		if _, ok := item[key]; !ok {
			t.Errorf("%s: item missing documented field %q", where, key)
		}
	}
	assertNoForbiddenMetricFields(t, where+" item", item)

	// Below the server-enforced depth the list is complete: terminated with
	// no continuation cursor (CLAUDE.md §2.1).
	if page.Terminated == nil || !*page.Terminated {
		t.Errorf("%s: want terminated=true, got %v", where, page.Terminated)
	}
	if page.NextCursor != "" {
		t.Errorf("%s: want empty next_cursor, got %q", where, page.NextCursor)
	}
}

// TestFollowAPI_Lists_RequireAuth verifies that both follow-list endpoints
// reject unauthenticated callers with 401.
func TestFollowAPI_Lists_RequireAuth(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	id := uuid.New().String()

	for _, path := range []string{
		"/api/v1/users/" + id + "/following",
		"/api/v1/users/" + id + "/followers",
	} {
		resp := doJSON(t, http.MethodGet, srv.url(path), nil, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without auth: want 401, got %d", path, resp.StatusCode)
		}
	}
}

// TestFollowAPI_FollowingAndFollowers_Shape verifies that after A follows B,
// A's following list contains B and B's followers list contains A, using the
// documented FollowPage envelope with no public metric fields.
func TestFollowAPI_FollowingAndFollowers_Shape(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	aID, aHandle, aToken := registerUserWithID(t, srv)
	bID, bHandle, bToken := registerUserWithID(t, srv)

	doAuthedExpectNoContent(t, srv, http.MethodPost, "/api/v1/users/"+bID.String()+"/follow", aToken)

	following := getFollowPage(t, srv, aToken, "/api/v1/users/"+aID.String()+"/following")
	requireSingleFollowUser(t, "following", following, bID, bHandle)

	followers := getFollowPage(t, srv, bToken, "/api/v1/users/"+bID.String()+"/followers")
	requireSingleFollowUser(t, "followers", followers, aID, aHandle)
}

// TestFollowAPI_Lists_InvalidCursor_422 verifies that a malformed cursor on
// either follow-list endpoint is rejected with 422 VALIDATION_ERROR (the follow
// handler's existing CodeValidation mapping, follow/handler.go).
func TestFollowAPI_Lists_InvalidCursor_422(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	id, _, token := registerUserWithID(t, srv)

	for _, path := range []string{
		"/api/v1/users/" + id.String() + "/following?cursor=not-a-valid-cursor",
		"/api/v1/users/" + id.String() + "/followers?cursor=not-a-valid-cursor",
	} {
		resp := doJSON(t, http.MethodGet, srv.url(path), nil, bearerHeader(token))
		if resp.StatusCode != http.StatusUnprocessableEntity {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("GET %s: want 422, got %d: %s", path, resp.StatusCode, b)
			continue
		}
		if code := apiErrorCode(t, resp.Body); code != "VALIDATION_ERROR" {
			t.Errorf("GET %s: error code = %q, want VALIDATION_ERROR", path, code)
		}
		resp.Body.Close()
	}
}
