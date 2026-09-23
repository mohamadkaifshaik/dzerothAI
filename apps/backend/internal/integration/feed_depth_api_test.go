//go:build integration

// Hard feed-depth integration tests (CLAUDE.md §2.1, ADR 0006).
//
// Validates, over HTTP against real PostgreSQL and Redis, that the four feed
// endpoints serve pages of pageSize from the current top-maxDepth window only:
//
//	GET /api/v1/feeds/home                 pageSize 50, maxDepth 200
//	GET /api/v1/users/{userID}/posts       pageSize 50, maxDepth 200
//	GET /api/v1/hashtags/{tag}/posts       pageSize 50, maxDepth 200
//	GET /api/v1/posts/{postID}/thread      pageSize 25, maxDepth 100
//
// Test data is inserted through post.Repository (not the HTTP API) so that
// hundreds of posts do not hit the post-creation rate limiter. Every post gets
// an explicit, strictly increasing created_at so ordering is deterministic.
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// ---------------------------------------------------------------------------
// Data helpers
// ---------------------------------------------------------------------------

// postSeeder inserts posts with strictly increasing created_at values, one
// millisecond apart, so feed ordering never depends on clock resolution.
type postSeeder struct {
	t    *testing.T
	ctx  context.Context
	repo *post.Repository
	next time.Time
}

func newPostSeeder(t *testing.T, pool *pgxpool.Pool) *postSeeder {
	t.Helper()
	return &postSeeder{
		t:    t,
		ctx:  context.Background(),
		repo: post.NewRepository(pool),
		next: time.Now().UTC().Truncate(time.Millisecond),
	}
}

// insert creates one post and returns it. parentID/threadRootID make it a reply.
func (s *postSeeder) insert(authorID uuid.UUID, tags []string, parentID, threadRootID *uuid.UUID) post.Post {
	s.t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		s.t.Fatalf("uuid.NewV7: %v", err)
	}
	content := "Feed depth test post " + id.String()
	p := post.Post{
		ID:           id,
		AuthorID:     authorID,
		PostType:     post.PostTypeOriginal,
		Content:      &content,
		ParentID:     parentID,
		ThreadRootID: threadRootID,
		CreatedAt:    s.next,
		UpdatedAt:    s.next,
	}
	if parentID != nil {
		p.PostType = post.PostTypeReply
	}
	s.next = s.next.Add(time.Millisecond)
	if err := s.repo.Create(s.ctx, p, nil, tags); err != nil {
		s.t.Fatalf("seed post: %v", err)
	}
	return p
}

// insertMany creates n posts and returns their ids in creation order (oldest first).
func (s *postSeeder) insertMany(n int, authorID uuid.UUID, tags []string, parentID, threadRootID *uuid.UUID) []string {
	s.t.Helper()
	ids := make([]string, n)
	for i := range ids {
		ids[i] = s.insert(authorID, tags, parentID, threadRootID).ID.String()
	}
	return ids
}

// insertDeleted creates n posts and soft-deletes them.
func (s *postSeeder) insertDeleted(n int, authorID uuid.UUID, tags []string, parentID, threadRootID *uuid.UUID) {
	s.t.Helper()
	for i := 0; i < n; i++ {
		p := s.insert(authorID, tags, parentID, threadRootID)
		if err := s.repo.SoftDelete(s.ctx, p.ID, authorID); err != nil {
			s.t.Fatalf("soft delete seed post: %v", err)
		}
	}
}

// newestFirst returns a reversed copy (creation order → feed DESC order).
func newestFirst(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[len(ids)-1-i] = id
	}
	return out
}

// execSQL runs a setup statement and fails the test on error.
func execSQL(t *testing.T, pool *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("setup SQL %q: %v", q, err)
	}
}

// meID resolves the caller's user id via GET /api/v1/me at meURL.
func meID(t *testing.T, meURL, token string) uuid.UUID {
	t.Helper()
	resp := doJSON(t, http.MethodGet, meURL, nil, bearerHeader(token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /me: want 200, got %d: %s", resp.StatusCode, b)
	}
	var me struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatalf("decode /me: %v", err)
	}
	id, err := uuid.Parse(me.Data.ID)
	if err != nil {
		t.Fatalf("parse /me id %q: %v", me.Data.ID, err)
	}
	return id
}

// uniqueDepthTag returns a valid, unique, lowercase hashtag body.
func uniqueDepthTag() string {
	return "depth" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
}

// ---------------------------------------------------------------------------
// HTTP / chain helpers
// ---------------------------------------------------------------------------

type depthPage struct {
	IDs        []string
	NextCursor string
	Terminated bool
}

// feedFetcher returns a page for the given cursor ("" = first page).
type feedFetcher func(t *testing.T, cursor string) depthPage

// newFeedFetcher builds a fetcher for baseURL (which may already contain a
// query string) with an optional bearer token.
func newFeedFetcher(baseURL, token string) feedFetcher {
	return func(t *testing.T, cursor string) depthPage {
		t.Helper()
		u := baseURL
		if cursor != "" {
			sep := "?"
			if strings.Contains(u, "?") {
				sep = "&"
			}
			u += sep + "cursor=" + url.QueryEscape(cursor)
		}
		var headers map[string]string
		if token != "" {
			headers = bearerHeader(token)
		}
		resp := doJSON(t, http.MethodGet, u, nil, headers)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("GET %s: want 200, got %d: %s", u, resp.StatusCode, b)
		}
		var body struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			NextCursor *string `json:"next_cursor"`
			Terminated *bool   `json:"terminated"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s: %v", u, err)
		}
		if body.Items == nil {
			t.Fatalf("GET %s: items must be a JSON array, got null/absent", u)
		}
		if body.NextCursor == nil || body.Terminated == nil {
			t.Fatalf("GET %s: next_cursor and terminated must both be present", u)
		}
		p := depthPage{NextCursor: *body.NextCursor, Terminated: *body.Terminated}
		for _, it := range body.Items {
			p.IDs = append(p.IDs, it.ID)
		}
		return p
	}
}

// walkChain follows next_cursor from the first page until terminated. It fails
// if a non-terminated page has no cursor or the chain exceeds maxPages.
func walkChain(t *testing.T, fetch feedFetcher, maxPages int) []depthPage {
	t.Helper()
	var pages []depthPage
	cursor := ""
	for {
		p := fetch(t, cursor)
		pages = append(pages, p)
		if p.Terminated {
			return pages
		}
		if p.NextCursor == "" {
			t.Fatalf("page %d: terminated=false but next_cursor is empty", len(pages))
		}
		if len(pages) >= maxPages {
			t.Fatalf("chain did not terminate within %d pages", maxPages)
		}
		cursor = p.NextCursor
	}
}

// assertFullDepthChain asserts a complete chain over a feed holding at least
// maxDepth eligible items: exactly maxDepth/pageSize full pages, continuation
// cursors on every page but the last, a terminated final page with "", no
// overlap, and exactly want[:maxDepth] in feed order — nothing beyond maxDepth.
func assertFullDepthChain(t *testing.T, fetch feedFetcher, pageSize, maxDepth int, want []string) []depthPage {
	t.Helper()
	wantPages := maxDepth / pageSize
	pages := walkChain(t, fetch, wantPages+2)

	if len(pages) != wantPages {
		t.Fatalf("pages = %d, want %d (maxDepth %d / pageSize %d)", len(pages), wantPages, maxDepth, pageSize)
	}
	var got []string
	seen := make(map[string]bool)
	for i, p := range pages {
		if len(p.IDs) != pageSize {
			t.Errorf("page %d: %d items, want %d", i+1, len(p.IDs), pageSize)
		}
		last := i == len(pages)-1
		if !last && (p.Terminated || p.NextCursor == "") {
			t.Errorf("page %d: want terminated=false with a cursor, got terminated=%v cursor=%q", i+1, p.Terminated, p.NextCursor)
		}
		if last && (!p.Terminated || p.NextCursor != "") {
			t.Errorf("final page: want terminated=true and next_cursor=\"\", got terminated=%v cursor=%q", p.Terminated, p.NextCursor)
		}
		for _, id := range p.IDs {
			if seen[id] {
				t.Errorf("page %d: item %s already returned on an earlier page", i+1, id)
			}
			seen[id] = true
			got = append(got, id)
		}
	}

	if len(got) != maxDepth {
		t.Fatalf("total exposed = %d, want exactly maxDepth %d", len(got), maxDepth)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("item %d = %s, want %s (feed order)", i+1, got[i], want[i])
		}
	}
	for _, id := range want[maxDepth:] {
		if seen[id] {
			t.Errorf("item %s lies beyond maxDepth %d but was exposed", id, maxDepth)
		}
	}
	return pages
}

// assertRepeatedCursorStable asserts that the same cursor yields the same page.
func assertRepeatedCursorStable(t *testing.T, fetch feedFetcher, cursor string) {
	t.Helper()
	a := fetch(t, cursor)
	b := fetch(t, cursor)
	if strings.Join(a.IDs, ",") != strings.Join(b.IDs, ",") ||
		a.NextCursor != b.NextCursor || a.Terminated != b.Terminated {
		t.Errorf("repeated cursor returned different pages:\n first: %v %q %v\nsecond: %v %q %v",
			a.IDs, a.NextCursor, a.Terminated, b.IDs, b.NextCursor, b.Terminated)
	}
}

// assertEmptyTerminated asserts the out-of-window result: [], "", true.
func assertEmptyTerminated(t *testing.T, p depthPage, context string) {
	t.Helper()
	if len(p.IDs) != 0 || !p.Terminated || p.NextCursor != "" {
		t.Errorf("%s: want items=[] terminated=true next_cursor=\"\", got %d items terminated=%v cursor=%q",
			context, len(p.IDs), p.Terminated, p.NextCursor)
	}
}

// assertMalformedCursorRejected asserts that a syntactically invalid cursor is
// rejected with wantStatus and the VALIDATION_ERROR envelope. The status is the
// endpoint's existing validation mapping: post handlers (author, hashtag,
// thread) render 400; the feed handler renders 422 (feed/handler.go).
func assertMalformedCursorRejected(t *testing.T, baseURL, token string, wantStatus int) {
	t.Helper()
	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	var headers map[string]string
	if token != "" {
		headers = bearerHeader(token)
	}
	resp := doJSON(t, http.MethodGet, baseURL+sep+"cursor=not-a-valid-cursor", nil, headers)
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("malformed cursor: want %d, got %d: %s", wantStatus, resp.StatusCode, b)
	}
	if code := apiErrorCode(t, resp.Body); code != "VALIDATION_ERROR" {
		t.Errorf("malformed cursor: error code = %q, want VALIDATION_ERROR", code)
	}
}

// encodeCursorFor returns a syntactically valid cursor positioned at p.
func encodeCursorFor(p post.Post) string {
	return post.FeedCursor{AfterID: p.ID, Timestamp: p.CreatedAt}.Encode()
}

// ---------------------------------------------------------------------------
// Home feed — pageSize 50, maxDepth 200
// ---------------------------------------------------------------------------

// TestFeedDepth_Home_CrossingBoundary covers first/middle/final pages, the
// 201st eligible post never being exposed, ineligible rows (deleted, muted,
// blocked) not consuming the window, a repeated cursor, and a cursor pushed
// out of the window by newer posts.
func TestFeedDepth_Home_CrossingBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildFeedAPIServer(t, pool, redisClient)
	token := registerUserOnFeedServer(t, srv)
	viewer := meID(t, srv.url("/api/v1/me"), token)
	ctx := context.Background()

	author := newTitleTestUser(ctx, t, pool)
	muted := newTitleTestUser(ctx, t, pool)
	blocked := newTitleTestUser(ctx, t, pool)
	for _, followed := range []uuid.UUID{author, muted, blocked} {
		execSQL(t, pool, `INSERT INTO follows (follower_id, followed_id) VALUES ($1, $2)`, viewer, followed)
	}
	execSQL(t, pool, `INSERT INTO mutes (muter_id, muted_id) VALUES ($1, $2)`, viewer, muted)
	execSQL(t, pool, `INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, viewer, blocked)

	seed := newPostSeeder(t, pool)
	eligible := seed.insertMany(201, author, nil, nil, nil)
	// Newest rows are ineligible: if they consumed the window, fewer than 200
	// eligible posts would be exposed.
	seed.insertDeleted(10, author, nil, nil, nil)
	seed.insertMany(10, muted, nil, nil, nil)
	seed.insertMany(10, blocked, nil, nil, nil)

	fetch := newFeedFetcher(srv.url("/api/v1/feeds/home"), token)
	pages := assertFullDepthChain(t, fetch, 50, 200, newestFirst(eligible))

	assertRepeatedCursorStable(t, fetch, pages[0].NextCursor)

	// pages[2].NextCursor points at eligible rank 150. 51 newer eligible posts
	// push that item to rank 201 — outside the current top-200 window.
	stale := pages[2].NextCursor
	seed.insertMany(51, author, nil, nil, nil)
	assertEmptyTerminated(t, fetch(t, stale), "home cursor pushed out of window")
}

// TestFeedDepth_Home_ExactBoundary verifies exactly 200 eligible posts are
// served as four full pages ending terminated with an empty cursor.
func TestFeedDepth_Home_ExactBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildFeedAPIServer(t, pool, redisClient)
	token := registerUserOnFeedServer(t, srv)
	viewer := meID(t, srv.url("/api/v1/me"), token)

	author := newTitleTestUser(context.Background(), t, pool)
	execSQL(t, pool, `INSERT INTO follows (follower_id, followed_id) VALUES ($1, $2)`, viewer, author)

	eligible := newPostSeeder(t, pool).insertMany(200, author, nil, nil, nil)
	assertFullDepthChain(t, newFeedFetcher(srv.url("/api/v1/feeds/home"), token), 50, 200, newestFirst(eligible))
}

// TestFeedDepth_Home_MalformedCursor_422 verifies invalid cursor syntax → 422
// VALIDATION_ERROR (the feed handler's existing CodeValidation mapping).
func TestFeedDepth_Home_MalformedCursor_422(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildFeedAPIServer(t, pool, redisClient)
	token := registerUserOnFeedServer(t, srv)

	assertMalformedCursorRejected(t, srv.url("/api/v1/feeds/home"), token, http.StatusUnprocessableEntity)
}

// ---------------------------------------------------------------------------
// Author posts — pageSize 50, maxDepth 200
// ---------------------------------------------------------------------------

// TestFeedDepth_Author_CrossingBoundary covers the full chain, the 201st post
// never being exposed, deleted posts not consuming the window, a repeated
// cursor, and a cursor pushed out of the window by newer posts.
func TestFeedDepth_Author_CrossingBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	author := newTitleTestUser(context.Background(), t, pool)

	seed := newPostSeeder(t, pool)
	eligible := seed.insertMany(201, author, nil, nil, nil)
	seed.insertDeleted(10, author, nil, nil, nil)

	fetch := newFeedFetcher(srv.url("/api/v1/users/"+author.String()+"/posts"), "")
	pages := assertFullDepthChain(t, fetch, 50, 200, newestFirst(eligible))

	assertRepeatedCursorStable(t, fetch, pages[1].NextCursor)

	stale := pages[2].NextCursor
	seed.insertMany(51, author, nil, nil, nil)
	assertEmptyTerminated(t, fetch(t, stale), "author cursor pushed out of window")
}

// TestFeedDepth_Author_ExactBoundary verifies exactly 200 posts → four full
// pages ending terminated with an empty cursor.
func TestFeedDepth_Author_ExactBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	author := newTitleTestUser(context.Background(), t, pool)

	eligible := newPostSeeder(t, pool).insertMany(200, author, nil, nil, nil)
	assertFullDepthChain(t, newFeedFetcher(srv.url("/api/v1/users/"+author.String()+"/posts"), ""), 50, 200, newestFirst(eligible))
}

// TestFeedDepth_Author_MalformedCursor_400 verifies invalid cursor syntax → 400.
func TestFeedDepth_Author_MalformedCursor_400(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	author := newTitleTestUser(context.Background(), t, pool)

	assertMalformedCursorRejected(t, srv.url("/api/v1/users/"+author.String()+"/posts"), "", http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Hashtag feed — pageSize 50, maxDepth 200
// ---------------------------------------------------------------------------

// TestFeedDepth_Hashtag_CrossingBoundary covers the full chain for an
// authenticated caller, the 201st tagged post never being exposed, deleted and
// blocked-author posts not consuming the window, a repeated cursor, and a
// cursor pushed out of the window by newer posts.
func TestFeedDepth_Hashtag_CrossingBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	viewer, _, token := registerUserWithID(t, srv)
	ctx := context.Background()

	author := newTitleTestUser(ctx, t, pool)
	blocked := newTitleTestUser(ctx, t, pool)
	execSQL(t, pool, `INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, viewer, blocked)

	tag := uniqueDepthTag()
	tags := []string{tag}
	seed := newPostSeeder(t, pool)
	eligible := seed.insertMany(201, author, tags, nil, nil)
	seed.insertDeleted(10, author, tags, nil, nil)
	seed.insertMany(10, blocked, tags, nil, nil)

	fetch := newFeedFetcher(srv.url("/api/v1/hashtags/"+tag+"/posts"), token)
	pages := assertFullDepthChain(t, fetch, 50, 200, newestFirst(eligible))

	assertRepeatedCursorStable(t, fetch, pages[0].NextCursor)

	stale := pages[2].NextCursor
	seed.insertMany(51, author, tags, nil, nil)
	assertEmptyTerminated(t, fetch(t, stale), "hashtag cursor pushed out of window")
}

// TestFeedDepth_Hashtag_ExactBoundary verifies exactly 200 tagged posts →
// four full pages ending terminated with an empty cursor.
func TestFeedDepth_Hashtag_ExactBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	author := newTitleTestUser(context.Background(), t, pool)

	tag := uniqueDepthTag()
	eligible := newPostSeeder(t, pool).insertMany(200, author, []string{tag}, nil, nil)
	assertFullDepthChain(t, newFeedFetcher(srv.url("/api/v1/hashtags/"+tag+"/posts"), ""), 50, 200, newestFirst(eligible))
}

// TestFeedDepth_Hashtag_MalformedCursor_400 verifies invalid cursor syntax → 400.
func TestFeedDepth_Hashtag_MalformedCursor_400(t *testing.T) {
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)

	assertMalformedCursorRejected(t, srv.url("/api/v1/hashtags/"+uniqueDepthTag()+"/posts"), "", http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Thread replies — pageSize 25, maxDepth 100 (chronological order)
// ---------------------------------------------------------------------------

// TestFeedDepth_Thread_CrossingBoundary covers the full chain in chronological
// order, the 101st reply never being exposed, deleted replies (the oldest
// rows) not consuming the window, a repeated cursor, and a valid cursor that
// lies outside the current window.
func TestFeedDepth_Thread_CrossingBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	author := newTitleTestUser(context.Background(), t, pool)

	seed := newPostSeeder(t, pool)
	root := seed.insert(author, nil, nil, nil)
	rootID := root.ID
	// Oldest replies are deleted: in chronological order they come first, so
	// counting them would push eligible replies out of the window.
	seed.insertDeleted(5, author, nil, &rootID, &rootID)
	var eligible []string
	var replies []post.Post
	for i := 0; i < 101; i++ {
		r := seed.insert(author, nil, &rootID, &rootID)
		replies = append(replies, r)
		eligible = append(eligible, r.ID.String())
	}

	fetch := newFeedFetcher(srv.url("/api/v1/posts/"+rootID.String()+"/thread"), "")
	pages := assertFullDepthChain(t, fetch, 25, 100, eligible) // chronological, oldest first

	assertRepeatedCursorStable(t, fetch, pages[1].NextCursor)

	// A syntactically valid cursor positioned at the 101st reply lies past the
	// end of the current 100-reply window.
	assertEmptyTerminated(t, fetch(t, encodeCursorFor(replies[100])), "thread cursor outside window")
}

// TestFeedDepth_Thread_ExactBoundary verifies exactly 100 replies → four full
// pages ending terminated with an empty cursor.
func TestFeedDepth_Thread_ExactBoundary(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildPostAPIServer(t, redisClient)
	author := newTitleTestUser(context.Background(), t, pool)

	seed := newPostSeeder(t, pool)
	root := seed.insert(author, nil, nil, nil)
	rootID := root.ID
	eligible := seed.insertMany(100, author, nil, &rootID, &rootID)

	assertFullDepthChain(t, newFeedFetcher(srv.url("/api/v1/posts/"+rootID.String()+"/thread"), ""), 25, 100, eligible)
}
