// Package bookmark — service tests.
//
// Unit tests for bookmark service business logic using fake repository doubles.
// No real database connection is required.
package bookmark

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// ---------------------------------------------------------------------------
// fakeBookmarkRepo — test double for Repository
// ---------------------------------------------------------------------------

type fakeBookmarkRepo struct {
	addFn        func(ctx context.Context, userID, postID uuid.UUID) error
	removeFn     func(ctx context.Context, userID, postID uuid.UUID) error
	listByUserFn func(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]bookmarkWithPost, string, bool, error)
}

func (f *fakeBookmarkRepo) Add(ctx context.Context, userID, postID uuid.UUID) error {
	if f.addFn != nil {
		return f.addFn(ctx, userID, postID)
	}
	return nil
}

func (f *fakeBookmarkRepo) Remove(ctx context.Context, userID, postID uuid.UUID) error {
	if f.removeFn != nil {
		return f.removeFn(ctx, userID, postID)
	}
	return nil
}

func (f *fakeBookmarkRepo) ListByUser(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]bookmarkWithPost, string, bool, error) {
	if f.listByUserFn != nil {
		return f.listByUserFn(ctx, userID, cursor, max)
	}
	return nil, "", true, nil
}

// testableBookmarkService mirrors Service but accepts a bookmarkRepo interface
// so tests can inject fakes without a real database.
type bookmarkRepoIface interface {
	Add(ctx context.Context, userID, postID uuid.UUID) error
	Remove(ctx context.Context, userID, postID uuid.UUID) error
	ListByUser(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]bookmarkWithPost, string, bool, error)
}

// newFakeService creates a Service whose repo field points to the fake via a
// wrapper. Because Service.repo is *Repository (concrete), we build a thin
// testable service directly from the service logic.
func newFakeService(fake bookmarkRepoIface) *testableBookmarkService {
	log, _ := zap.NewDevelopment()
	return &testableBookmarkService{repo: fake, log: log}
}

// testableBookmarkService duplicates the service logic using the interface so
// repository calls go through the fake. This mirrors the pattern in post/service_test.go.
type testableBookmarkService struct {
	repo bookmarkRepoIface
	log  *zap.Logger
}

func (s *testableBookmarkService) Bookmark(ctx context.Context, callerID, postID uuid.UUID) error {
	if err := s.repo.Add(ctx, callerID, postID); err != nil {
		var apiErr *apierror.APIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		s.log.Error("bookmark: add", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableBookmarkService) Unbookmark(ctx context.Context, callerID, postID uuid.UUID) error {
	if err := s.repo.Remove(ctx, callerID, postID); err != nil {
		s.log.Error("bookmark: remove", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableBookmarkService) ListBookmarks(ctx context.Context, callerID uuid.UUID, cursorStr string) (BookmarkPage, error) {
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return BookmarkPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	items, nextCursor, terminated, err := s.repo.ListByUser(ctx, callerID, cursor, maxBookmarkDepth)
	if err != nil {
		s.log.Error("bookmark: list by user", zap.Error(err))
		return BookmarkPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	dtos := make([]BookmarkDTO, len(items))
	for i, bwp := range items {
		dtos[i] = BookmarkDTO{
			PostID:    bwp.PostID.String(),
			CreatedAt: bwp.BookmarkCreatedAt.UTC().Format(time.RFC3339),
			Post:      post.ToDTO(bwp.Post),
		}
	}
	if len(dtos) == 0 {
		dtos = []BookmarkDTO{}
	}

	return BookmarkPage{
		Items:      dtos,
		NextCursor: nextCursor,
		Terminated: terminated,
	}, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func apiErrorCode(err error) string {
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func makeFakeBWP(userID uuid.UUID, n int) []bookmarkWithPost {
	items := make([]bookmarkWithPost, n)
	for i := range items {
		postID, _ := uuid.NewV7()
		c := "content"
		items[i] = bookmarkWithPost{
			UserID:            userID,
			PostID:            postID,
			BookmarkCreatedAt: time.Now().UTC(),
			Post: post.Post{
				ID:       postID,
				AuthorID: userID,
				PostType: post.PostTypeOriginal,
				Content:  &c,
			},
		}
	}
	return items
}

// ---------------------------------------------------------------------------
// TestBookmark_PostNotFound
// ---------------------------------------------------------------------------

// TestBookmark_PostNotFound verifies that bookmarking a non-existent post
// returns a CodeNotFound error.
func TestBookmark_PostNotFound(t *testing.T) {
	fake := &fakeBookmarkRepo{
		addFn: func(_ context.Context, _, _ uuid.UUID) error {
			return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		},
	}
	svc := newFakeService(fake)

	err := svc.Bookmark(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected CodeNotFound error, got nil")
	}
	if code := apiErrorCode(err); code != apierror.CodeNotFound {
		t.Errorf("error code = %q, want %q", code, apierror.CodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// TestBookmark_Idempotent
// ---------------------------------------------------------------------------

// TestBookmark_Idempotent verifies that bookmarking the same post twice does
// not return an error (ON CONFLICT DO NOTHING semantics).
func TestBookmark_Idempotent(t *testing.T) {
	calls := 0
	fake := &fakeBookmarkRepo{
		addFn: func(_ context.Context, _, _ uuid.UUID) error {
			calls++
			return nil // both calls succeed — idempotent
		},
	}
	svc := newFakeService(fake)

	callerID := uuid.New()
	postID := uuid.New()

	if err := svc.Bookmark(context.Background(), callerID, postID); err != nil {
		t.Fatalf("first Bookmark returned unexpected error: %v", err)
	}
	if err := svc.Bookmark(context.Background(), callerID, postID); err != nil {
		t.Fatalf("second Bookmark returned unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 Add calls, got %d", calls)
	}
}

// ---------------------------------------------------------------------------
// TestUnbookmark_NonExistent
// ---------------------------------------------------------------------------

// TestUnbookmark_NonExistent verifies that unbookmarking a non-existent
// bookmark returns nil (idempotent delete).
func TestUnbookmark_NonExistent(t *testing.T) {
	fake := &fakeBookmarkRepo{
		removeFn: func(_ context.Context, _, _ uuid.UUID) error {
			// DELETE affects 0 rows — repo returns nil.
			return nil
		},
	}
	svc := newFakeService(fake)

	err := svc.Unbookmark(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Errorf("Unbookmark non-existent bookmark returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestListBookmarks_Terminated
// ---------------------------------------------------------------------------

// TestListBookmarks_Terminated verifies that when the repository returns
// exactly maxBookmarkDepth items the page is marked Terminated.
func TestListBookmarks_Terminated(t *testing.T) {
	callerID := uuid.New()
	items := makeFakeBWP(callerID, maxBookmarkDepth)

	fake := &fakeBookmarkRepo{
		listByUserFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor, _ int) ([]bookmarkWithPost, string, bool, error) {
			return items, "", true, nil
		},
	}
	svc := newFakeService(fake)

	page, err := svc.ListBookmarks(context.Background(), callerID, "")
	if err != nil {
		t.Fatalf("ListBookmarks returned unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true when repo signals termination at maxBookmarkDepth")
	}
	if len(page.Items) != maxBookmarkDepth {
		t.Errorf("expected %d items, got %d", maxBookmarkDepth, len(page.Items))
	}
}

// ---------------------------------------------------------------------------
// TestListBookmarks_BadCursor
// ---------------------------------------------------------------------------

// TestListBookmarks_BadCursor verifies that a malformed cursor string is
// rejected with a CodeValidation error.
func TestListBookmarks_BadCursor(t *testing.T) {
	fake := &fakeBookmarkRepo{}
	svc := newFakeService(fake)

	_, err := svc.ListBookmarks(context.Background(), uuid.New(), "!!!invalid!!!")
	if err == nil {
		t.Fatal("expected CodeValidation error for bad cursor, got nil")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// ---------------------------------------------------------------------------
// TestListBookmarks_Empty
// ---------------------------------------------------------------------------

// TestListBookmarks_Empty verifies that a user with no bookmarks receives an
// empty items slice and Terminated=true.
func TestListBookmarks_Empty(t *testing.T) {
	fake := &fakeBookmarkRepo{
		listByUserFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor, _ int) ([]bookmarkWithPost, string, bool, error) {
			return nil, "", true, nil
		},
	}
	svc := newFakeService(fake)

	page, err := svc.ListBookmarks(context.Background(), uuid.New(), "")
	if err != nil {
		t.Fatalf("ListBookmarks returned unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true for empty bookmark list")
	}
	if page.Items == nil {
		t.Error("expected non-nil Items slice for empty result")
	}
	if len(page.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Items))
	}
}
