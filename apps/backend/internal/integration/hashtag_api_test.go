//go:build integration

// Hashtag feed HTTP API integration tests.
//
// Validates GET /api/v1/hashtags/{tag}/posts against real PostgreSQL and Redis.
// Every test exercises the full chain:
//
//	HTTP request → chi router → post.Handler → post.Service → PostgreSQL
//
// Tags written by one test are unique per test (UUID-derived) so tests sharing
// the same database cannot observe each other's posts.
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uniqueMixedCaseHashtag returns a hashtag body (without '#') that is unique
// per call and contains both upper- and lower-case letters, e.g.
// "Tag_0a1b2c3d4e5f". It matches the extraction pattern
// [a-zA-Z][a-zA-Z0-9_]* so it is stored as its lowercase form.
func uniqueMixedCaseHashtag() string {
	return "Tag_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
}

// createHashtagPost creates an original post with the given content through
// POST /api/v1/posts and returns the created post ID.
func createHashtagPost(t *testing.T, srv *postTestServer, token, content string) string {
	t.Helper()

	body := fmt.Sprintf(`{"post_type":"original","content":%q}`, content)
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
			ID string `json:"id"`
		} `json:"post"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode create post response: %v", err)
	}
	if result.Post.ID == "" {
		t.Fatal("create post: post.id is empty")
	}
	return result.Post.ID
}

// hashtagFeedPage mirrors the post.PostPage JSON envelope. next_cursor has no
// omitempty on the server side, so a terminated page carries "".
type hashtagFeedPage struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
	NextCursor string `json:"next_cursor"`
	Terminated *bool  `json:"terminated"`
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestHashtagAPI_CreateThenFetch_Normalized verifies the create → extract →
// persist → fetch path: a post containing a mixed-case hashtag is returned by
// the hashtag feed when requested with the same mixed-case tag. The write path
// lowercases on extraction and the read path lowercases the path parameter,
// so both must agree for the post to be found.
func TestHashtagAPI_CreateThenFetch_Normalized(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	tag := uniqueMixedCaseHashtag()
	postID := createHashtagPost(t, srv, token, "Hashtag integration test #"+tag)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/hashtags/"+tag+"/posts"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("hashtag feed: want 200, got %d: %s", resp.StatusCode, b)
	}

	var page hashtagFeedPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode hashtag feed: %v", err)
	}

	if len(page.Items) != 1 {
		t.Fatalf("hashtag feed: want exactly 1 item for unique tag %q, got %d", tag, len(page.Items))
	}
	if page.Items[0].ID != postID {
		t.Errorf("hashtag feed item id = %q, want created post %q", page.Items[0].ID, postID)
	}
	// A single-item result is below the server-enforced depth, so the feed is
	// finite and complete: terminated with no continuation cursor (CLAUDE.md §2.1).
	if page.Terminated == nil || !*page.Terminated {
		t.Errorf("hashtag feed: want terminated=true for a single-item result, got %v", page.Terminated)
	}
	if page.NextCursor != "" {
		t.Errorf("hashtag feed: want empty next_cursor on terminated page, got %q", page.NextCursor)
	}
}

// TestHashtagAPI_UnknownTag_EmptyTerminated verifies that a well-formed tag
// with no posts returns 200 with an empty, terminated page.
func TestHashtagAPI_UnknownTag_EmptyTerminated(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)

	tag := uniqueMixedCaseHashtag() // never written to the database

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/hashtags/"+tag+"/posts"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("unknown hashtag feed: want 200, got %d: %s", resp.StatusCode, b)
	}

	var page hashtagFeedPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode unknown hashtag feed: %v", err)
	}

	if len(page.Items) != 0 {
		t.Errorf("unknown hashtag feed: want 0 items, got %d", len(page.Items))
	}
	if page.Terminated == nil || !*page.Terminated {
		t.Errorf("unknown hashtag feed: want terminated=true, got %v", page.Terminated)
	}
	if page.NextCursor != "" {
		t.Errorf("unknown hashtag feed: want empty next_cursor, got %q", page.NextCursor)
	}
}

// TestHashtagAPI_InvalidTag_400 verifies that a malformed tag (must start with
// a letter) is rejected with 400 and the standard VALIDATION_ERROR envelope.
func TestHashtagAPI_InvalidTag_400(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/hashtags/123abc/posts"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("invalid hashtag: want 400, got %d: %s", resp.StatusCode, b)
	}
	code := apiErrorCode(t, resp.Body)
	if code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", code)
	}
}

// TestHashtagAPI_NoPublicMetricFields verifies that a non-empty hashtag feed
// response contains no social-validation metric fields (CLAUDE.md §2.3).
// A post is created first so the item-level check is not vacuous.
func TestHashtagAPI_NoPublicMetricFields(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	token := registerAndGetToken(t, srv)

	tag := uniqueMixedCaseHashtag()
	createHashtagPost(t, srv, token, "Hashtag metric lockdown test #"+tag)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/hashtags/"+tag+"/posts"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("hashtag feed: want 200, got %d: %s", resp.StatusCode, b)
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
			t.Fatalf("decode items: %v", err)
		}
	}
	if len(items) == 0 {
		t.Fatal("hashtag feed returned no items — metric check would be vacuous")
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
				t.Errorf("hashtag feed item contains forbidden public metric field %q (CLAUDE.md §2.3)", field)
			}
		}
	}
}
