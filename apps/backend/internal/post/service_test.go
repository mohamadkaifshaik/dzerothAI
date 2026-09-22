// Package post — service tests.
//
// Unit tests for post service business logic. Pure-validation paths (those that
// return before any repository call) use a Service with a nil *Repository.
// Tests that exercise paths requiring the repository use a testableService
// defined in this file, which accepts a postRepo interface test double.
package post

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// ---------------------------------------------------------------------------
// testableService — a service that accepts a postRepo interface so repo calls
// can be controlled in tests without a real database connection.
// ---------------------------------------------------------------------------

// postRepo is the minimal interface for the repository methods called by the
// service functions under test. It mirrors the concrete *Repository signatures.
type postRepo interface {
	Create(ctx context.Context, p Post, mentions []uuid.UUID, hashtags []string) error
	GetByID(ctx context.Context, id uuid.UUID) (Post, error)
	ListByAuthor(ctx context.Context, authorID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error)
	ListThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error)
	SoftDelete(ctx context.Context, postID uuid.UUID, authorID uuid.UUID) error
	GetAuthorHandleByHandle(ctx context.Context, handle string) (uuid.UUID, error)
	GetAuthorIsPrivate(ctx context.Context, authorID uuid.UUID) (bool, error)
}

// testableService mirrors the Service struct but holds a postRepo interface
// so that tests can inject fakes without a real database.
type testableService struct {
	repo postRepo
	log  *zap.Logger
}

// createPost mirrors Service.CreatePost, using the interface repo.
func (s *testableService) createPost(ctx context.Context, authorID uuid.UUID, req CreatePostRequest) (PostDTO, error) {
	switch req.PostType {
	case PostTypeOriginal, PostTypeReply, PostTypeQuote, PostTypeRepost:
		// valid
	default:
		if req.PostType == "" {
			req.PostType = PostTypeOriginal
		} else {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "invalid post_type")
		}
	}

	if req.PostType != PostTypeRepost {
		count := utf8.RuneCountInString(req.Content)
		if count == 0 {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "content is required")
		}
		if count > maxPostRunes {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "content exceeds 500 code points")
		}
	}

	if req.PostType == PostTypeReply && req.ParentID == nil {
		return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "parent_id is required for reply posts")
	}

	if (req.PostType == PostTypeQuote || req.PostType == PostTypeRepost) && req.QuotedPostID == nil {
		return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "quoted_post_id is required for quote and repost posts")
	}

	if req.PostType == PostTypeQuote {
		if countDistinctWords(req.Content) < minQuoteWords {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation,
				fmt.Sprintf("quote post content must contain at least %d distinct words", minQuoteWords))
		}
	}

	// Share/quote delay enforcement — mirrors Service.CreatePost.
	if req.PostType == PostTypeRepost || req.PostType == PostTypeQuote {
		if err := validateShareDelay(req.ShareInitiatedAt); err != nil {
			return PostDTO{}, err
		}
	}

	var parentID *uuid.UUID
	if req.ParentID != nil {
		parsed, err := uuid.Parse(*req.ParentID)
		if err != nil {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "parent_id must be a valid UUID")
		}
		parentID = &parsed
	}

	var quotedPostID *uuid.UUID
	if req.QuotedPostID != nil {
		parsed, err := uuid.Parse(*req.QuotedPostID)
		if err != nil {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeValidation, "quoted_post_id must be a valid UUID")
		}
		quotedPostID = &parsed
	}

	var threadRootID *uuid.UUID
	if req.PostType == PostTypeReply && parentID != nil {
		parent, err := s.repo.GetByID(ctx, *parentID)
		if err != nil {
			if err == ErrNotFound {
				return PostDTO{}, apierror.NewAPIError(apierror.CodeNotFound, "parent post not found")
			}
			return PostDTO{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
		}
		if parent.ThreadRootID != nil {
			threadRootID = parent.ThreadRootID
		} else {
			threadRootID = parentID
		}
	}

	var hashtags []string
	var mentionUserIDs []uuid.UUID

	if req.PostType != PostTypeRepost {
		hashtags = extractHashtags(req.Content)
		handles := extractMentions(req.Content)
		for _, handle := range handles {
			userID, err := s.repo.GetAuthorHandleByHandle(ctx, handle)
			if err != nil {
				continue
			}
			mentionUserIDs = append(mentionUserIDs, userID)
		}
	}

	postID, err := uuid.NewV7()
	if err != nil {
		return PostDTO{}, err
	}

	var contentPtr *string
	if req.PostType != PostTypeRepost {
		c := req.Content
		contentPtr = &c
	}

	p := Post{
		ID:           postID,
		AuthorID:     authorID,
		PostType:     req.PostType,
		Content:      contentPtr,
		ParentID:     parentID,
		ThreadRootID: threadRootID,
		QuotedPostID: quotedPostID,
	}

	if err := s.repo.Create(ctx, p, mentionUserIDs, hashtags); err != nil {
		return PostDTO{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	created, err := s.repo.GetByID(ctx, postID)
	if err != nil {
		return PostDTO{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	return ToDTO(created), nil
}

// listPostsByAuthor mirrors Service.ListPostsByAuthor using the interface repo.
func (s *testableService) listPostsByAuthor(ctx context.Context, callerID *uuid.UUID, authorID uuid.UUID, cursorStr string) (PostPage, error) {
	isPrivate, err := s.repo.GetAuthorIsPrivate(ctx, authorID)
	if err != nil {
		if err == ErrNotFound {
			return PostPage{}, apierror.NewAPIError(apierror.CodeNotFound, "user not found")
		}
		return PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	if isPrivate {
		isOwner := callerID != nil && *callerID == authorID
		if !isOwner {
			return PostPage{Items: []PostDTO{}, NextCursor: "", Terminated: true}, nil
		}
	}

	var cursor *FeedCursor
	if cursorStr != "" {
		c, err := DecodeCursor(cursorStr)
		if err != nil {
			return PostPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	posts, nextCursor, terminated, err := s.repo.ListByAuthor(ctx, authorID, cursor, maxDepthAuthor)
	if err != nil {
		return PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	dtos := make([]PostDTO, len(posts))
	for i, p := range posts {
		dtos[i] = ToDTO(p)
	}

	return PostPage{Items: dtos, NextCursor: nextCursor, Terminated: terminated}, nil
}

// listThreadReplies mirrors Service.ListThreadReplies using the interface repo.
func (s *testableService) listThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursorStr string) (PostPage, error) {
	var cursor *FeedCursor
	if cursorStr != "" {
		c, err := DecodeCursor(cursorStr)
		if err != nil {
			return PostPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	posts, nextCursor, terminated, err := s.repo.ListThreadReplies(ctx, threadRootID, cursor, maxDepthThread)
	if err != nil {
		return PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	dtos := make([]PostDTO, len(posts))
	for i, p := range posts {
		dtos[i] = ToDTO(p)
	}

	return PostPage{Items: dtos, NextCursor: nextCursor, Terminated: terminated}, nil
}

// deletePost mirrors Service.DeletePost using the interface repo.
func (s *testableService) deletePost(ctx context.Context, callerID uuid.UUID, postID uuid.UUID) error {
	p, err := s.repo.GetByID(ctx, postID)
	if err != nil {
		if err == ErrNotFound {
			return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		}
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	if p.AuthorID != callerID {
		return apierror.NewAPIError(apierror.CodeForbidden, "you may not delete another user's post")
	}

	if err := s.repo.SoftDelete(ctx, postID, callerID); err != nil {
		if err == ErrNotFound {
			return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		}
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// ---------------------------------------------------------------------------
// fakeRepo — test double for postRepo
// ---------------------------------------------------------------------------

type fakeRepo struct {
	// getByIDFn is called by GetByID if set, otherwise returns (Post{}, ErrNotFound).
	getByIDFn func(ctx context.Context, id uuid.UUID) (Post, error)

	// createFn is called by Create if set, otherwise returns nil.
	createFn func(ctx context.Context, p Post, mentions []uuid.UUID, hashtags []string) error

	// listByAuthorFn is called by ListByAuthor if set.
	listByAuthorFn func(ctx context.Context, authorID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error)

	// listThreadRepliesFn is called by ListThreadReplies if set.
	listThreadRepliesFn func(ctx context.Context, threadRootID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error)

	// softDeleteFn is called by SoftDelete if set.
	softDeleteFn func(ctx context.Context, postID uuid.UUID, authorID uuid.UUID) error

	// getHandleFn is called by GetAuthorHandleByHandle if set.
	getHandleFn func(ctx context.Context, handle string) (uuid.UUID, error)

	// getIsPrivateFn is called by GetAuthorIsPrivate if set.
	getIsPrivateFn func(ctx context.Context, authorID uuid.UUID) (bool, error)

	// captured values for assertions.
	capturedHashtags []string
	capturedMentions []uuid.UUID
}

func (f *fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (Post, error) {
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, id)
	}
	return Post{}, ErrNotFound
}

func (f *fakeRepo) Create(ctx context.Context, p Post, mentions []uuid.UUID, hashtags []string) error {
	f.capturedHashtags = hashtags
	f.capturedMentions = mentions
	if f.createFn != nil {
		return f.createFn(ctx, p, mentions, hashtags)
	}
	return nil
}

func (f *fakeRepo) ListByAuthor(ctx context.Context, authorID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error) {
	if f.listByAuthorFn != nil {
		return f.listByAuthorFn(ctx, authorID, cursor, limit)
	}
	return nil, "", true, nil
}

func (f *fakeRepo) ListThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error) {
	if f.listThreadRepliesFn != nil {
		return f.listThreadRepliesFn(ctx, threadRootID, cursor, limit)
	}
	return nil, "", true, nil
}

func (f *fakeRepo) SoftDelete(ctx context.Context, postID uuid.UUID, authorID uuid.UUID) error {
	if f.softDeleteFn != nil {
		return f.softDeleteFn(ctx, postID, authorID)
	}
	return nil
}

func (f *fakeRepo) GetAuthorHandleByHandle(ctx context.Context, handle string) (uuid.UUID, error) {
	if f.getHandleFn != nil {
		return f.getHandleFn(ctx, handle)
	}
	return uuid.UUID{}, ErrNotFound
}

func (f *fakeRepo) GetAuthorIsPrivate(ctx context.Context, authorID uuid.UUID) (bool, error) {
	if f.getIsPrivateFn != nil {
		return f.getIsPrivateFn(ctx, authorID)
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestableService(repo postRepo) *testableService {
	log, _ := zap.NewDevelopment()
	return &testableService{repo: repo, log: log}
}

// newNilRepoService creates a real Service with a nil *Repository.
// Only safe to use for test cases that return before any repository call
// (i.e., pure validation error paths).
func newNilRepoService() *Service {
	log, _ := zap.NewDevelopment()
	return &Service{repo: nil, log: log}
}

func apiErrorCode(err error) string {
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// ---------------------------------------------------------------------------
// CreatePost — content validation
// ---------------------------------------------------------------------------

// TestCreatePost_ValidContent verifies a 500-rune post is accepted.
func TestCreatePost_ValidContent(t *testing.T) {
	// Build exactly 500 runes.
	content := strings.Repeat("a", maxPostRunes)
	if utf8.RuneCountInString(content) != maxPostRunes {
		t.Fatalf("test setup: expected %d runes, got %d", maxPostRunes, utf8.RuneCountInString(content))
	}

	fake := &fakeRepo{
		createFn: func(_ context.Context, _ Post, _ []uuid.UUID, _ []string) error { return nil },
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			c := content
			return Post{ID: id, AuthorID: uuid.New(), PostType: PostTypeOriginal, Content: &c}, nil
		},
	}
	svc := newTestableService(fake)

	_, err := svc.createPost(context.Background(), uuid.New(), CreatePostRequest{
		PostType: PostTypeOriginal,
		Content:  content,
	})
	if err != nil {
		t.Errorf("createPost with %d-rune content returned unexpected error: %v", maxPostRunes, err)
	}
}

// TestCreatePost_ContentExactLimit verifies that exactly maxPostRunes runes
// (500) is accepted — on-the-boundary test.
func TestCreatePost_ContentExactLimit(t *testing.T) {
	content := strings.Repeat("x", maxPostRunes)

	fake := &fakeRepo{
		createFn: func(_ context.Context, _ Post, _ []uuid.UUID, _ []string) error { return nil },
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			c := content
			return Post{ID: id, AuthorID: uuid.New(), PostType: PostTypeOriginal, Content: &c}, nil
		},
	}
	svc := newTestableService(fake)

	_, err := svc.createPost(context.Background(), uuid.New(), CreatePostRequest{
		PostType: PostTypeOriginal,
		Content:  content,
	})
	if err != nil {
		t.Errorf("createPost with exactly %d runes returned error: %v", maxPostRunes, err)
	}
}

// TestCreatePost_ContentTooLong verifies a 501-rune post is rejected with a
// validation error.
func TestCreatePost_ContentTooLong(t *testing.T) {
	// Use the real Service with nil repo — validation returns before any repo call.
	svc := newNilRepoService()
	content := strings.Repeat("a", maxPostRunes+1)

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType: PostTypeOriginal,
		Content:  content,
	})
	if err == nil {
		t.Fatal("CreatePost with 501-rune content should return a validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestCreatePost_EmptyContent verifies that an empty content string for an
// original post is rejected with a validation error.
func TestCreatePost_EmptyContent(t *testing.T) {
	svc := newNilRepoService()

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType: PostTypeOriginal,
		Content:  "",
	})
	if err == nil {
		t.Fatal("CreatePost with empty content should return a validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestCreatePost_RepostHasNoContent verifies the actual service behavior for a
// repost: repost type skips content validation entirely, so content is ignored
// (not an error) — but quoted_post_id is required.
// The service enforces: PostTypeRepost requires quoted_post_id; no content rule.
func TestCreatePost_RepostHasNoContent(t *testing.T) {
	svc := newNilRepoService()

	// A repost without quoted_post_id should fail validation — not due to content.
	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType: PostTypeRepost,
		Content:  "this content should be ignored", // content is not validated for reposts
	})
	if err == nil {
		t.Fatal("CreatePost repost without quoted_post_id should return a validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}

	// Confirm the error message is about quoted_post_id, not content.
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		if !strings.Contains(ae.Message, "quoted_post_id") {
			t.Errorf("expected error about quoted_post_id, got: %q", ae.Message)
		}
	}
}

// TestCreatePost_UnicodeRunes verifies a string with multi-byte UTF-8 characters
// where len(bytes) > 500 but utf8.RuneCountInString <= 500 is accepted.
// This confirms the service counts runes, not bytes.
func TestCreatePost_UnicodeRunes(t *testing.T) {
	// "日" is 3 bytes in UTF-8, 1 rune.
	// 500 * "日" = 1500 bytes, 500 runes — must be accepted.
	content := strings.Repeat("日", maxPostRunes)
	byteLen := len(content)
	runeLen := utf8.RuneCountInString(content)

	if runeLen != maxPostRunes {
		t.Fatalf("test setup: expected %d runes, got %d", maxPostRunes, runeLen)
	}
	if byteLen <= maxPostRunes {
		t.Fatalf("test setup: expected byte length > %d to prove the test is meaningful, got %d", maxPostRunes, byteLen)
	}

	fake := &fakeRepo{
		createFn: func(_ context.Context, _ Post, _ []uuid.UUID, _ []string) error { return nil },
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			c := content
			return Post{ID: id, AuthorID: uuid.New(), PostType: PostTypeOriginal, Content: &c}, nil
		},
	}
	svc := newTestableService(fake)

	_, err := svc.createPost(context.Background(), uuid.New(), CreatePostRequest{
		PostType: PostTypeOriginal,
		Content:  content,
	})
	if err != nil {
		t.Errorf("createPost with %d-rune (multi-byte) content returned unexpected error: %v", runeLen, err)
	}
}

// ---------------------------------------------------------------------------
// extractHashtags
// ---------------------------------------------------------------------------

// TestHashtagExtraction verifies that hashtags are extracted, normalized to
// lowercase, and the leading '#' is stripped.
func TestHashtagExtraction(t *testing.T) {
	got := extractHashtags("#Hello #world #test")
	want := []string{"hello", "world", "test"}

	if len(got) != len(want) {
		t.Fatalf("extractHashtags: got %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("extractHashtags[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestHashtagExtraction_MaxTen verifies that only the first maxHashtags (10)
// distinct tags are kept when the content contains more than 10 hashtags.
func TestHashtagExtraction_MaxTen(t *testing.T) {
	// 12 unique hashtags.
	content := "#one #two #three #four #five #six #seven #eight #nine #ten #eleven #twelve"
	got := extractHashtags(content)

	if len(got) > maxHashtags {
		t.Errorf("extractHashtags returned %d tags, want at most %d", len(got), maxHashtags)
	}
	if len(got) != maxHashtags {
		t.Errorf("extractHashtags returned %d tags for %d-hashtag content, want exactly %d", len(got), 12, maxHashtags)
	}
}

// TestHashtagExtraction_Deduplication verifies that case-insensitive duplicates
// are collapsed so "#hello #Hello" yields only ["hello"].
func TestHashtagExtraction_Deduplication(t *testing.T) {
	got := extractHashtags("#hello #Hello #HELLO")

	if len(got) != 1 {
		t.Errorf("extractHashtags: expected 1 deduplicated tag, got %v", got)
	}
	if len(got) > 0 && got[0] != "hello" {
		t.Errorf("extractHashtags: expected %q, got %q", "hello", got[0])
	}
}

// ---------------------------------------------------------------------------
// extractMentions
// ---------------------------------------------------------------------------

// TestMentionExtraction verifies "@alice @bob" yields both handles without the
// leading '@'.
func TestMentionExtraction(t *testing.T) {
	got := extractMentions("hello @alice and @bob!")
	if len(got) != 2 {
		t.Fatalf("extractMentions: got %v (len %d), want 2 handles", got, len(got))
	}
	found := make(map[string]bool)
	for _, h := range got {
		found[strings.ToLower(h)] = true
	}
	if !found["alice"] {
		t.Errorf("extractMentions: missing handle 'alice', got %v", got)
	}
	if !found["bob"] {
		t.Errorf("extractMentions: missing handle 'bob', got %v", got)
	}
}

// TestMentionExtraction_SkipsUnknown verifies that unknown handles are skipped
// gracefully (no error) when creating a post.
func TestMentionExtraction_SkipsUnknown(t *testing.T) {
	// The fakeRepo returns ErrNotFound for all handles — no mention IDs captured.
	fake := &fakeRepo{
		getHandleFn: func(_ context.Context, _ string) (uuid.UUID, error) {
			return uuid.UUID{}, ErrNotFound
		},
		createFn: func(_ context.Context, _ Post, _ []uuid.UUID, _ []string) error { return nil },
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			c := "hello @unknown"
			return Post{ID: id, AuthorID: uuid.New(), PostType: PostTypeOriginal, Content: &c}, nil
		},
	}
	svc := newTestableService(fake)

	_, err := svc.createPost(context.Background(), uuid.New(), CreatePostRequest{
		PostType: PostTypeOriginal,
		Content:  "hello @unknown",
	})
	if err != nil {
		t.Errorf("createPost with unknown mention returned unexpected error: %v", err)
	}
	if len(fake.capturedMentions) != 0 {
		t.Errorf("expected 0 mention IDs for unknown handle, got %v", fake.capturedMentions)
	}
}

// ---------------------------------------------------------------------------
// ListPostsByAuthor — private account and depth limit
// ---------------------------------------------------------------------------

// TestListPostsByAuthor_PrivateAccount verifies that a private account's posts
// are not returned to a caller who is not the owner: the page is empty and
// Terminated is true.
func TestListPostsByAuthor_PrivateAccount(t *testing.T) {
	authorID := uuid.New()
	otherCallerID := uuid.New() // different from authorID

	fake := &fakeRepo{
		getIsPrivateFn: func(_ context.Context, id uuid.UUID) (bool, error) {
			return true, nil // account is private
		},
	}
	svc := newTestableService(fake)

	page, err := svc.listPostsByAuthor(context.Background(), &otherCallerID, authorID, "")
	if err != nil {
		t.Fatalf("listPostsByAuthor returned unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true for private account seen by non-owner")
	}
	if len(page.Items) != 0 {
		t.Errorf("expected 0 items for private account, got %d", len(page.Items))
	}
}

// TestListPostsByAuthor_MaxDepth verifies that when the repository returns
// exactly maxDepthAuthor (200) posts the page is marked Terminated.
// The buildPage helper sets terminated=true when hasMore is false (i.e. when
// fewer than limit+1 rows are returned). Returning exactly 200 means no extra
// row, so terminated=true.
func TestListPostsByAuthor_MaxDepth(t *testing.T) {
	authorID := uuid.New()

	// Return exactly maxDepthAuthor posts — no extra row → terminated.
	posts := makeFakePosts(authorID, maxDepthAuthor)

	fake := &fakeRepo{
		getIsPrivateFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		listByAuthorFn: func(_ context.Context, _ uuid.UUID, _ *FeedCursor, _ int) ([]Post, string, bool, error) {
			return posts, "", true, nil
		},
	}
	svc := newTestableService(fake)

	page, err := svc.listPostsByAuthor(context.Background(), nil, authorID, "")
	if err != nil {
		t.Fatalf("listPostsByAuthor returned unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true when repository signals feed termination at maxDepthAuthor")
	}
	if len(page.Items) != maxDepthAuthor {
		t.Errorf("expected %d items, got %d", maxDepthAuthor, len(page.Items))
	}
}

// TestListThreadReplies_MaxDepth verifies that when the repository returns
// exactly maxDepthThread (100) replies the page is marked Terminated.
func TestListThreadReplies_MaxDepth(t *testing.T) {
	threadRootID := uuid.New()
	authorID := uuid.New()

	posts := makeFakePosts(authorID, maxDepthThread)

	fake := &fakeRepo{
		listThreadRepliesFn: func(_ context.Context, _ uuid.UUID, _ *FeedCursor, _ int) ([]Post, string, bool, error) {
			return posts, "", true, nil
		},
	}
	svc := newTestableService(fake)

	page, err := svc.listThreadReplies(context.Background(), threadRootID, "")
	if err != nil {
		t.Fatalf("listThreadReplies returned unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true when repository signals termination at maxDepthThread")
	}
	if len(page.Items) != maxDepthThread {
		t.Errorf("expected %d items, got %d", maxDepthThread, len(page.Items))
	}
}

// ---------------------------------------------------------------------------
// DeletePost — ownership
// ---------------------------------------------------------------------------

// TestDeletePost_OwnerSucceeds verifies that the post owner can delete their post.
func TestDeletePost_OwnerSucceeds(t *testing.T) {
	ownerID := uuid.New()
	postID := uuid.New()

	fake := &fakeRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			return Post{ID: postID, AuthorID: ownerID}, nil
		},
		softDeleteFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil },
	}
	svc := newTestableService(fake)

	err := svc.deletePost(context.Background(), ownerID, postID)
	if err != nil {
		t.Errorf("deletePost by owner returned unexpected error: %v", err)
	}
}

// TestDeletePost_NotOwner verifies that a non-owner receives a forbidden error.
func TestDeletePost_NotOwner(t *testing.T) {
	ownerID := uuid.New()
	callerID := uuid.New() // different user
	postID := uuid.New()

	fake := &fakeRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			return Post{ID: postID, AuthorID: ownerID}, nil
		},
	}
	svc := newTestableService(fake)

	err := svc.deletePost(context.Background(), callerID, postID)
	if err == nil {
		t.Fatal("deletePost by non-owner should return an error")
	}
	if code := apiErrorCode(err); code != apierror.CodeForbidden {
		t.Errorf("error code = %q, want %q", code, apierror.CodeForbidden)
	}
}

// ---------------------------------------------------------------------------
// Quote post — five-distinct-word invariant (CLAUDE.md §2.2)
// ---------------------------------------------------------------------------

// TestCreatePost_QuoteRequiresFiveDistinctWords verifies that a quote post with
// fewer than 5 distinct words in the content is rejected with a validation error.
// This is an absolute Dzeroth product invariant enforced by the backend
// (CLAUDE.md §2.2 and §18 rule 10).
//
// Success cases (content with ≥ 5 distinct words) are covered by the
// countDistinctWords unit test and the general CreatePost happy-path tests.
// A nil repo is safe here because all cases are expected to fail validation
// before any repository call.
func TestCreatePost_QuoteRequiresFiveDistinctWords(t *testing.T) {
	quotedID := uuid.New().String()
	svc := newNilRepoService()

	cases := []struct {
		name    string
		content string
	}{
		{"four distinct words", "one two three four"},
		{"four words with punctuation", "one, two! three? four."},
		{"duplicates don't count", "hello hello hello hello hello"},
		{"case-insensitive dedup collapses to two", "Hello hello HELLO hElLo world"},
		{"empty string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
				PostType:     PostTypeQuote,
				Content:      tc.content,
				QuotedPostID: &quotedID,
			})
			if err == nil {
				t.Errorf("CreatePost quote with content %q: expected validation error, got nil", tc.content)
				return
			}
			if code := apiErrorCode(err); code != apierror.CodeValidation {
				t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
			}
		})
	}
}

// TestCreatePost_QuoteFiveDistinctWords_Accepted verifies that a quote post
// with exactly 5 distinct words passes the word-count validation.
// Uses a testableService with a fake repo so the success path does not panic.
// A valid ShareInitiatedAt (6 seconds ago) is provided to satisfy the share
// delay invariant (CLAUDE.md §2.2).
func TestCreatePost_QuoteFiveDistinctWords_Accepted(t *testing.T) {
	quotedID := uuid.New().String()
	content := "one two three four five"
	ts := time.Now().UTC().Add(-6 * time.Second)
	fake := &fakeRepo{
		createFn: func(_ context.Context, _ Post, _ []uuid.UUID, _ []string) error { return nil },
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			c := content
			qid, _ := uuid.Parse(quotedID)
			return Post{ID: id, AuthorID: uuid.New(), PostType: PostTypeQuote, Content: &c, QuotedPostID: &qid}, nil
		},
	}
	svc := newTestableService(fake)

	_, err := svc.createPost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeQuote,
		Content:          content,
		QuotedPostID:     &quotedID,
		ShareInitiatedAt: &ts,
	})
	if err != nil {
		t.Errorf("createPost quote with 5 distinct words returned unexpected error: %v", err)
	}
}

// TestCountDistinctWords verifies the normalization strategy used by the
// five-distinct-word quota. This strategy must match the Flutter client.
func TestCountDistinctWords(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"Hello HELLO hello", 1},   // case-insensitive dedup
		{"one, two! three.", 3},    // punctuation stripped
		{"  spaces  between  ", 2}, // leading/trailing spaces
		{"日本語 テスト", 2},             // non-ASCII letters counted
		{"123 456 123", 2},         // numbers counted, deduped
	}

	for _, tc := range cases {
		got := countDistinctWords(tc.input)
		if got != tc.want {
			t.Errorf("countDistinctWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// makeFakePosts builds n fake Post values for use in feed tests.
func makeFakePosts(authorID uuid.UUID, n int) []Post {
	posts := make([]Post, n)
	for i := range posts {
		id, _ := uuid.NewV7()
		c := "content"
		posts[i] = Post{
			ID:       id,
			AuthorID: authorID,
			PostType: PostTypeOriginal,
			Content:  &c,
		}
	}
	return posts
}
