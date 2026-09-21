//go:build integration

// Phase 7 integration tests: primary title hydration in PostAuthor blocks.
//
// These tests verify that every post-producing query (GetByID, ListByAuthor,
// ListThreadReplies, ListByHashtag, ListHomeTimeline, SearchPosts, bookmarks
// ListByUser) correctly populates PostAuthor.PrimaryTitle when a user has a
// primary title set, and leaves it nil when none is set.
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/bookmark"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/feed"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/search"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/title"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// createTestPost creates a post via post.Repository and returns the created Post.
func createTestPost(ctx context.Context, t *testing.T, postRepo *post.Repository, authorID uuid.UUID, content string) post.Post {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("createTestPost uuid: %v", err)
	}
	now := time.Now().UTC()
	p := post.Post{
		ID:        id,
		AuthorID:  authorID,
		PostType:  post.PostTypeOriginal,
		Content:   &content,
		IsDeleted: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := postRepo.Create(ctx, p, nil, nil); err != nil {
		t.Fatalf("createTestPost create: %v", err)
	}
	return p
}

// createTestPostWithHashtag creates a post with one hashtag row.
func createTestPostWithHashtag(ctx context.Context, t *testing.T, postRepo *post.Repository, authorID uuid.UUID, content, tag string) post.Post {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("createTestPostWithHashtag uuid: %v", err)
	}
	now := time.Now().UTC()
	p := post.Post{
		ID:        id,
		AuthorID:  authorID,
		PostType:  post.PostTypeOriginal,
		Content:   &content,
		IsDeleted: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := postRepo.Create(ctx, p, nil, []string{tag}); err != nil {
		t.Fatalf("createTestPostWithHashtag create: %v", err)
	}
	return p
}

// grantAndSetPrimaryTitle creates a user_title for the given definition and
// sets it as the user's primary title. Returns the user_titles.id.
func grantAndSetPrimaryTitle(ctx context.Context, t *testing.T, titleRepo *title.Repository, userID, defID uuid.UUID) uuid.UUID {
	t.Helper()
	ut, err := titleRepo.CreateUserTitle(ctx, userID, defID)
	if err != nil {
		t.Fatalf("grantAndSetPrimaryTitle CreateUserTitle: %v", err)
	}
	if err := titleRepo.SetPrimaryTitle(ctx, userID, ut.ID); err != nil {
		t.Fatalf("grantAndSetPrimaryTitle SetPrimaryTitle: %v", err)
	}
	return ut.ID
}

// ---------------------------------------------------------------------------
// 1. TestPostAuthor_GetByID_WithTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_GetByID_WithTitle verifies that GetByID returns a non-nil
// PrimaryTitle.Slug when the author has a primary title set.
func TestPostAuthor_GetByID_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userID, defFoundingMember)

	p := createTestPost(ctx, t, postRepo, userID, "hello from founding member")

	got, err := postRepo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Author.PrimaryTitle == nil {
		t.Fatal("GetByID: PrimaryTitle is nil, expected non-nil for user with primary title")
	}
	if got.Author.PrimaryTitle.Slug != "founding_member" {
		t.Errorf("GetByID: PrimaryTitle.Slug = %q, want founding_member", got.Author.PrimaryTitle.Slug)
	}
	if got.Author.PrimaryTitle.DisplayName != "Founding Member" {
		t.Errorf("GetByID: PrimaryTitle.DisplayName = %q, want Founding Member", got.Author.PrimaryTitle.DisplayName)
	}
}

// ---------------------------------------------------------------------------
// 2. TestPostAuthor_GetByID_NoTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_GetByID_NoTitle verifies that GetByID returns nil PrimaryTitle
// when the author has no primary title set.
func TestPostAuthor_GetByID_NoTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	p := createTestPost(ctx, t, postRepo, userID, "no title user post")

	got, err := postRepo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Author.PrimaryTitle != nil {
		t.Errorf("GetByID: PrimaryTitle should be nil for user with no primary title, got %+v", got.Author.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// 3. TestPostAuthor_ListByAuthor_WithTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_ListByAuthor_WithTitle verifies that ListByAuthor populates
// PrimaryTitle for all returned posts.
func TestPostAuthor_ListByAuthor_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userID, defCenturion)
	createTestPost(ctx, t, postRepo, userID, "centurion post")

	posts, _, _, err := postRepo.ListByAuthor(ctx, userID, nil, 10)
	if err != nil {
		t.Fatalf("ListByAuthor: %v", err)
	}
	if len(posts) == 0 {
		t.Fatal("ListByAuthor returned no posts")
	}
	for _, p := range posts {
		if p.Author.PrimaryTitle == nil {
			t.Errorf("ListByAuthor: PrimaryTitle is nil for post %v", p.ID)
		} else if p.Author.PrimaryTitle.Slug != "centurion" {
			t.Errorf("ListByAuthor: PrimaryTitle.Slug = %q, want centurion", p.Author.PrimaryTitle.Slug)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. TestPostAuthor_ListThreadReplies_WithTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_ListThreadReplies_WithTitle verifies that ListThreadReplies
// includes PrimaryTitle in the reply author's block.
func TestPostAuthor_ListThreadReplies_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)

	parentAuthor := newTitleTestUser(ctx, t, pool)
	replyAuthor := newTitleTestUser(ctx, t, pool)
	grantAndSetPrimaryTitle(ctx, t, titleRepo, replyAuthor, defFoundingMember)

	// Create parent post (serves as thread root).
	parentID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid parent: %v", err)
	}
	now := time.Now().UTC()
	parentContent := "parent post for thread test"
	parentPost := post.Post{
		ID:        parentID,
		AuthorID:  parentAuthor,
		PostType:  post.PostTypeOriginal,
		Content:   &parentContent,
		IsDeleted: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := postRepo.Create(ctx, parentPost, nil, nil); err != nil {
		t.Fatalf("create parent post: %v", err)
	}

	// Create reply with thread_root_id = parentID.
	replyID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid reply: %v", err)
	}
	replyContent := "reply in thread"
	replyPost := post.Post{
		ID:           replyID,
		AuthorID:     replyAuthor,
		PostType:     post.PostTypeReply,
		Content:      &replyContent,
		ParentID:     &parentID,
		ThreadRootID: &parentID,
		IsDeleted:    false,
		CreatedAt:    now.Add(time.Second),
		UpdatedAt:    now.Add(time.Second),
	}
	if err := postRepo.Create(ctx, replyPost, nil, nil); err != nil {
		t.Fatalf("create reply post: %v", err)
	}

	replies, _, _, err := postRepo.ListThreadReplies(ctx, parentID, nil, 10)
	if err != nil {
		t.Fatalf("ListThreadReplies: %v", err)
	}
	if len(replies) == 0 {
		t.Fatal("ListThreadReplies returned no replies")
	}
	found := false
	for _, r := range replies {
		if r.ID == replyID {
			found = true
			if r.Author.PrimaryTitle == nil {
				t.Errorf("ListThreadReplies: reply author PrimaryTitle is nil")
			} else if r.Author.PrimaryTitle.Slug != "founding_member" {
				t.Errorf("ListThreadReplies: PrimaryTitle.Slug = %q, want founding_member", r.Author.PrimaryTitle.Slug)
			}
		}
	}
	if !found {
		t.Error("ListThreadReplies: reply post not found in results")
	}
}

// ---------------------------------------------------------------------------
// 5. TestPostAuthor_ListByHashtag_WithTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_ListByHashtag_WithTitle verifies that ListByHashtag populates
// PrimaryTitle for all returned posts.
func TestPostAuthor_ListByHashtag_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userID, defTrendsetter)

	// Use a unique tag to avoid cross-test interference.
	tag := "ttltest_" + userID.String()[:8]
	createTestPostWithHashtag(ctx, t, postRepo, userID, "post with "+tag, tag)

	posts, _, _, err := postRepo.ListByHashtag(ctx, tag, nil, 10, nil)
	if err != nil {
		t.Fatalf("ListByHashtag: %v", err)
	}
	if len(posts) == 0 {
		t.Fatal("ListByHashtag returned no posts")
	}
	for _, p := range posts {
		if p.Author.PrimaryTitle == nil {
			t.Errorf("ListByHashtag: PrimaryTitle is nil for post %v", p.ID)
		} else if p.Author.PrimaryTitle.Slug != "trendsetter" {
			t.Errorf("ListByHashtag: PrimaryTitle.Slug = %q, want trendsetter", p.Author.PrimaryTitle.Slug)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. TestPostAuthor_Feed_WithTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_Feed_WithTitle verifies that ListHomeTimeline populates
// PrimaryTitle in the author block for posts from followed users.
func TestPostAuthor_Feed_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)
	feedRepo := feed.NewRepository(pool)

	authorID := newTitleTestUser(ctx, t, pool)
	followerID := newTitleTestUser(ctx, t, pool)

	grantAndSetPrimaryTitle(ctx, t, titleRepo, authorID, defFoundingMember)
	createTestPost(ctx, t, postRepo, authorID, fmt.Sprintf("feed title test post %s", uuid.New().String()[:8]))

	// followerID follows authorID.
	if _, err := pool.Exec(ctx,
		`INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
		followerID, authorID,
	); err != nil {
		t.Fatalf("insert follow: %v", err)
	}

	posts, _, _, err := feedRepo.ListHomeTimeline(ctx, followerID, nil, nil, nil, 20)
	if err != nil {
		t.Fatalf("ListHomeTimeline: %v", err)
	}

	found := false
	for _, p := range posts {
		if p.AuthorID == authorID {
			found = true
			if p.Author.PrimaryTitle == nil {
				t.Errorf("Feed: PrimaryTitle is nil for author with primary title")
			} else if p.Author.PrimaryTitle.Slug != "founding_member" {
				t.Errorf("Feed: PrimaryTitle.Slug = %q, want founding_member", p.Author.PrimaryTitle.Slug)
			}
		}
	}
	if !found {
		t.Error("Feed: no post from expected author found in home timeline")
	}
}

// ---------------------------------------------------------------------------
// 7. TestPostAuthor_Search_WithTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_Search_WithTitle verifies that SearchPosts populates
// PrimaryTitle in the DTO's author block.
func TestPostAuthor_Search_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)
	searchRepo := search.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userID, defCenturion)

	// Unique content token to avoid matching other tests' posts.
	uniqueToken := "srchttltoken_" + userID.String()[:8]
	createTestPost(ctx, t, postRepo, userID, "post for search title test "+uniqueToken)

	page, err := searchRepo.SearchPosts(ctx, uniqueToken, "", nil)
	if err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("SearchPosts returned no results for unique token")
	}
	for _, dto := range page.Items {
		if dto.Author.PrimaryTitle == nil {
			t.Errorf("SearchPosts: PrimaryTitle is nil for post %v", dto.ID)
		} else if dto.Author.PrimaryTitle.Slug != "centurion" {
			t.Errorf("SearchPosts: PrimaryTitle.Slug = %q, want centurion", dto.Author.PrimaryTitle.Slug)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. TestPostAuthor_Bookmark_WithTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_Bookmark_WithTitle verifies that bookmark.Service.ListBookmarks
// includes PrimaryTitle in the returned post's author block.
func TestPostAuthor_Bookmark_WithTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)
	bookmarkRepo := bookmark.NewRepository(pool)
	bookmarkSvc := bookmark.NewService(bookmarkRepo, zap.NewNop())

	authorID := newTitleTestUser(ctx, t, pool)
	viewerID := newTitleTestUser(ctx, t, pool)

	grantAndSetPrimaryTitle(ctx, t, titleRepo, authorID, defFoundingMember)
	p := createTestPost(ctx, t, postRepo, authorID, "bookmarked post with title")

	if err := bookmarkRepo.Add(ctx, viewerID, p.ID); err != nil {
		t.Fatalf("bookmark Add: %v", err)
	}

	page, err := bookmarkSvc.ListBookmarks(ctx, viewerID, "")
	if err != nil {
		t.Fatalf("bookmark ListBookmarks: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("bookmark ListBookmarks returned no items")
	}

	found := false
	for _, item := range page.Items {
		if item.PostID == p.ID.String() {
			found = true
			if item.Post.Author.PrimaryTitle == nil {
				t.Errorf("Bookmark: PrimaryTitle is nil for author with primary title")
			} else if item.Post.Author.PrimaryTitle.Slug != "founding_member" {
				t.Errorf("Bookmark: PrimaryTitle.Slug = %q, want founding_member", item.Post.Author.PrimaryTitle.Slug)
			}
		}
	}
	if !found {
		t.Error("Bookmark: expected bookmarked post not found in ListBookmarks results")
	}
}

// ---------------------------------------------------------------------------
// 9. TestPostAuthor_ChangePrimaryTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_ChangePrimaryTitle verifies that switching the primary title
// is reflected in the next GetByID call without a schema change.
func TestPostAuthor_ChangePrimaryTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	p := createTestPost(ctx, t, postRepo, userID, "change primary title test")

	// Set founding_member as primary.
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userID, defFoundingMember)

	got, err := postRepo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID after founding_member: %v", err)
	}
	if got.Author.PrimaryTitle == nil || got.Author.PrimaryTitle.Slug != "founding_member" {
		t.Errorf("after founding_member: PrimaryTitle = %v, want founding_member", got.Author.PrimaryTitle)
	}

	// Grant centurion and switch primary to it.
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userID, defCenturion)

	got2, err := postRepo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID after centurion: %v", err)
	}
	if got2.Author.PrimaryTitle == nil || got2.Author.PrimaryTitle.Slug != "centurion" {
		t.Errorf("after centurion: PrimaryTitle = %v, want centurion", got2.Author.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// 10. TestPostAuthor_ClearPrimaryTitle
// ---------------------------------------------------------------------------

// TestPostAuthor_ClearPrimaryTitle verifies that clearing the primary title
// causes PrimaryTitle to return nil on subsequent reads.
func TestPostAuthor_ClearPrimaryTitle(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userID, defFoundingMember)
	p := createTestPost(ctx, t, postRepo, userID, "clear primary title test")

	// Confirm title is present.
	got, err := postRepo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID before clear: %v", err)
	}
	if got.Author.PrimaryTitle == nil {
		t.Fatal("PrimaryTitle should be non-nil before clearing")
	}

	// Clear primary title.
	if err := titleRepo.ClearPrimaryTitle(ctx, userID); err != nil {
		t.Fatalf("ClearPrimaryTitle: %v", err)
	}

	// Confirm title is now nil.
	got2, err := postRepo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID after clear: %v", err)
	}
	if got2.Author.PrimaryTitle != nil {
		t.Errorf("PrimaryTitle should be nil after ClearPrimaryTitle, got %+v", got2.Author.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// 11. TestPostAuthor_MultipleAuthors
// ---------------------------------------------------------------------------

// TestPostAuthor_MultipleAuthors verifies that in a single feed query:
// - user A (has title) has non-nil PrimaryTitle
// - user B (no title) has nil PrimaryTitle
func TestPostAuthor_MultipleAuthors(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	postRepo := post.NewRepository(pool)
	titleRepo := title.NewRepository(pool)
	feedRepo := feed.NewRepository(pool)

	userA := newTitleTestUser(ctx, t, pool)
	grantAndSetPrimaryTitle(ctx, t, titleRepo, userA, defFoundingMember)

	userB := newTitleTestUser(ctx, t, pool)

	viewer := newTitleTestUser(ctx, t, pool)

	// viewer follows both A and B.
	for _, followedID := range []uuid.UUID{userA, userB} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
			viewer, followedID,
		); err != nil {
			t.Fatalf("insert follow for %v: %v", followedID, err)
		}
	}

	suffix := uuid.New().String()[:8]
	createTestPost(ctx, t, postRepo, userA, "multi-author A post "+suffix)
	createTestPost(ctx, t, postRepo, userB, "multi-author B post "+suffix)

	posts, _, _, err := feedRepo.ListHomeTimeline(ctx, viewer, nil, nil, nil, 20)
	if err != nil {
		t.Fatalf("ListHomeTimeline: %v", err)
	}

	var seenA, seenB bool
	for _, p := range posts {
		switch p.AuthorID {
		case userA:
			seenA = true
			if p.Author.PrimaryTitle == nil {
				t.Errorf("user A should have PrimaryTitle, got nil (post %v)", p.ID)
			} else if p.Author.PrimaryTitle.Slug != "founding_member" {
				t.Errorf("user A PrimaryTitle.Slug = %q, want founding_member", p.Author.PrimaryTitle.Slug)
			}
		case userB:
			seenB = true
			if p.Author.PrimaryTitle != nil {
				t.Errorf("user B should have nil PrimaryTitle, got %+v (post %v)", p.Author.PrimaryTitle, p.ID)
			}
		}
	}
	if !seenA {
		t.Error("user A's post not found in feed")
	}
	if !seenB {
		t.Error("user B's post not found in feed")
	}
}
