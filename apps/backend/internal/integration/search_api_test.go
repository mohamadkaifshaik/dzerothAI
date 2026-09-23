//go:build integration

// Search HTTP API integration tests.
//
// Validates GET /api/v1/search/posts and GET /api/v1/search/users against real
// PostgreSQL and Redis using the shared postTestServer (search routes and the
// block provider wired as in cmd/api/main.go).
//
// Each test searches for a UUID-derived token so results cannot collide with
// data written by other tests.
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uniqueSearchToken returns a lowercase token that is unique per call.
func uniqueSearchToken() string {
	return "srch" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
}

// searchPageResponse mirrors the search page envelopes (PostSearchPage and
// UserSearchPage share the same items/next_cursor/terminated shape).
type searchPageResponse struct {
	Items      []map[string]json.RawMessage `json:"items"`
	NextCursor string                       `json:"next_cursor"`
	Terminated *bool                        `json:"terminated"`
}

// doSearch issues GET /api/v1/search/{kind}?q=<q>. token may be empty for an
// anonymous request (search is auth-optional).
func doSearch(t *testing.T, srv *postTestServer, kind, q, token string) *http.Response {
	t.Helper()
	var headers map[string]string
	if token != "" {
		headers = bearerHeader(token)
	}
	return doJSON(t, http.MethodGet,
		srv.url("/api/v1/search/"+kind+"?q="+url.QueryEscape(q)), nil, headers)
}

// searchOK performs a search, requires 200, and decodes the page.
func searchOK(t *testing.T, srv *postTestServer, kind, q, token string) searchPageResponse {
	t.Helper()
	resp := doSearch(t, srv, kind, q, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("search %s: want 200, got %d: %s", kind, resp.StatusCode, b)
	}
	var page searchPageResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode search %s: %v", kind, err)
	}
	return page
}

// itemIDs returns the "id" of every item in a search page.
func itemIDs(t *testing.T, page searchPageResponse) []string {
	t.Helper()
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		var id string
		if err := json.Unmarshal(item["id"], &id); err != nil {
			t.Fatalf("decode item id: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

// requireTerminatedPage asserts a complete page: terminated=true, no cursor.
func requireTerminatedPage(t *testing.T, where string, page searchPageResponse) {
	t.Helper()
	if page.Terminated == nil || !*page.Terminated {
		t.Errorf("%s: want terminated=true, got %v", where, page.Terminated)
	}
	if page.NextCursor != "" {
		t.Errorf("%s: want empty next_cursor, got %q", where, page.NextCursor)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestSearchAPI_Posts_ReturnsMatchingPost verifies that an anonymous caller can
// search posts and receives the documented envelope containing the matching
// post, with no public metric fields.
func TestSearchAPI_Posts_ReturnsMatchingPost(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	q := uniqueSearchToken()
	postID := createHashtagPost(t, srv, token, "Search contract probe "+q)

	page := searchOK(t, srv, "posts", q, "")

	ids := itemIDs(t, page)
	if len(ids) != 1 || ids[0] != postID {
		t.Fatalf("search posts: want exactly [%s], got %v", postID, ids)
	}
	requireTerminatedPage(t, "search posts", page)
	assertNoForbiddenMetricFields(t, "search posts item", page.Items[0])
}

// TestSearchAPI_Users_ReturnsMatchingUser verifies that an anonymous caller can
// search users by handle and receives the documented user result fields with
// no public metric fields.
func TestSearchAPI_Users_ReturnsMatchingUser(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	userID, handle, _ := registerUserWithID(t, srv)

	page := searchOK(t, srv, "users", handle, "")

	ids := itemIDs(t, page)
	if len(ids) != 1 || ids[0] != userID.String() {
		t.Fatalf("search users: want exactly [%s], got %v", userID, ids)
	}
	item := page.Items[0]
	for _, key := range []string{"id", "handle", "display_name", "avatar_url"} {
		if _, ok := item[key]; !ok {
			t.Errorf("search users item missing documented field %q", key)
		}
	}
	requireTerminatedPage(t, "search users", page)
	assertNoForbiddenMetricFields(t, "search users item", item)
}

// TestSearchAPI_EmptyQuery_400 verifies that an empty q is rejected with 400
// VALIDATION_ERROR on both search endpoints.
func TestSearchAPI_EmptyQuery_400(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)

	for _, kind := range []string{"posts", "users"} {
		resp := doSearch(t, srv, kind, "", "")
		if resp.StatusCode != http.StatusBadRequest {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Errorf("search %s with empty q: want 400, got %d: %s", kind, resp.StatusCode, b)
			continue
		}
		if code := apiErrorCode(t, resp.Body); code != "VALIDATION_ERROR" {
			t.Errorf("search %s with empty q: error code = %q, want VALIDATION_ERROR", kind, code)
		}
		resp.Body.Close()
	}
}

// TestSearchAPI_Posts_BlockedAuthorExcludedForAuthenticatedCaller verifies that
// posts by a user the caller has blocked are excluded from the caller's search
// results, while an anonymous search still returns them.
func TestSearchAPI_Posts_BlockedAuthorExcludedForAuthenticatedCaller(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	authorID, _, authorToken := registerUserWithID(t, srv)
	_, _, viewerToken := registerUserWithID(t, srv)

	q := uniqueSearchToken()
	postID := createHashtagPost(t, srv, authorToken, "Search block probe "+q)
	doAuthedExpectNoContent(t, srv, http.MethodPost, "/api/v1/users/"+authorID.String()+"/block", viewerToken)

	viewerPage := searchOK(t, srv, "posts", q, viewerToken)
	if ids := itemIDs(t, viewerPage); len(ids) != 0 {
		t.Errorf("search posts as blocking viewer: want 0 items, got %v", ids)
	}

	anonPage := searchOK(t, srv, "posts", q, "")
	if ids := itemIDs(t, anonPage); len(ids) != 1 || ids[0] != postID {
		t.Errorf("anonymous search posts: want exactly [%s], got %v", postID, ids)
	}
}

// TestSearchAPI_Users_BlockedUserExcludedForAuthenticatedCaller verifies that
// a user the caller has blocked is excluded from the caller's user search,
// while an anonymous search still returns them.
func TestSearchAPI_Users_BlockedUserExcludedForAuthenticatedCaller(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	targetID, targetHandle, _ := registerUserWithID(t, srv)
	_, _, viewerToken := registerUserWithID(t, srv)

	doAuthedExpectNoContent(t, srv, http.MethodPost, "/api/v1/users/"+targetID.String()+"/block", viewerToken)

	viewerPage := searchOK(t, srv, "users", targetHandle, viewerToken)
	if ids := itemIDs(t, viewerPage); len(ids) != 0 {
		t.Errorf("search users as blocking viewer: want 0 items, got %v", ids)
	}

	anonPage := searchOK(t, srv, "users", targetHandle, "")
	if ids := itemIDs(t, anonPage); len(ids) != 1 || ids[0] != targetID.String() {
		t.Errorf("anonymous search users: want exactly [%s], got %v", targetID, ids)
	}
}
