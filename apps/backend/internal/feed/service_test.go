package feed

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
	"go.uber.org/zap"
)

// ── fakes ───────────────────────────────────────────────────────────────────

type fakeRepository struct {
	posts      []post.Post
	nextCursor string
	terminated bool
	err        error
}

func (f *fakeRepository) ListHomeTimeline(
	_ context.Context,
	_ uuid.UUID,
	_ []uuid.UUID,
	_ []uuid.UUID,
	_ *post.FeedCursor,
	_ int,
) ([]post.Post, string, bool, error) {
	return f.posts, f.nextCursor, f.terminated, f.err
}

type fakeBlockProvider struct {
	blockedIDs []uuid.UUID
	mutedIDs   []uuid.UUID
	err        error
}

func (f *fakeBlockProvider) GetBlockedIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.blockedIDs, f.err
}

func (f *fakeBlockProvider) GetMutedIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.mutedIDs, f.err
}

type fakeFollowProvider struct {
	ids []uuid.UUID
	err error
}

func (f *fakeFollowProvider) GetFollowedIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.ids, f.err
}

// serviceUnderTest wraps the real Service but injects a fakeRepository so that
// we can test service logic without a live database.
//
// Because Service.repo is *Repository (a concrete struct) we need a thin shim.
// We define a repoIface interface internally to enable injection in tests.
type repoIface interface {
	ListHomeTimeline(
		ctx context.Context,
		callerID uuid.UUID,
		blockedIDs []uuid.UUID,
		mutedIDs []uuid.UUID,
		cursor *post.FeedCursor,
		max int,
	) ([]post.Post, string, bool, error)
}

// testService is a test-local variant of Service that accepts repoIface.
type testService struct {
	repo           repoIface
	blockProvider  BlockProvider
	followProvider FollowProvider
	log            *zap.Logger
}

func newTestService(repo repoIface, bp BlockProvider, fp FollowProvider) *testService {
	log, _ := zap.NewDevelopment()
	return &testService{repo: repo, blockProvider: bp, followProvider: fp, log: log}
}

func (s *testService) GetHomeFeed(ctx context.Context, callerID uuid.UUID, cursorStr string) (post.PostPage, error) {
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return post.PostPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	blockedIDs, err := s.blockProvider.GetBlockedIDs(ctx, callerID)
	if err != nil {
		return post.PostPage{}, err
	}

	mutedIDs, err := s.blockProvider.GetMutedIDs(ctx, callerID)
	if err != nil {
		return post.PostPage{}, err
	}

	posts, nextCursor, terminated, err := s.repo.ListHomeTimeline(
		ctx, callerID, blockedIDs, mutedIDs, cursor, maxHomeFeedDepth,
	)
	if err != nil {
		return post.PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	dtos := make([]post.PostDTO, len(posts))
	for i, p := range posts {
		dtos[i] = post.ToDTO(p)
	}
	if dtos == nil {
		dtos = []post.PostDTO{}
	}

	return post.PostPage{Items: dtos, NextCursor: nextCursor, Terminated: terminated}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makePosts(n int) []post.Post {
	posts := make([]post.Post, n)
	now := time.Now().UTC()
	for i := range posts {
		id, _ := uuid.NewV7()
		posts[i] = post.Post{
			ID:        id,
			AuthorID:  uuid.New(),
			PostType:  post.PostTypeOriginal,
			CreatedAt: now.Add(-time.Duration(i) * time.Second),
			UpdatedAt: now,
			Author: post.PostAuthor{
				ID:     uuid.New().String(),
				Handle: "user",
			},
		}
	}
	return posts
}

func noopFollowProvider() *fakeFollowProvider { return &fakeFollowProvider{} }
func noopBlockProvider() *fakeBlockProvider   { return &fakeBlockProvider{} }

// ── tests ─────────────────────────────────────────────────────────────────────

// TestGetHomeFeed_EmptyFollowList verifies that when the repository returns zero
// posts (caller follows nobody), the page is empty and terminated=true.
func TestGetHomeFeed_EmptyFollowList(t *testing.T) {
	repo := &fakeRepository{posts: nil, nextCursor: "", terminated: true}
	svc := newTestService(repo, noopBlockProvider(), noopFollowProvider())

	page, err := svc.GetHomeFeed(context.Background(), uuid.New(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true for empty follow list")
	}
	if len(page.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Items))
	}
	if page.Items == nil {
		t.Error("Items must be a non-nil empty slice, not nil")
	}
}

// TestGetHomeFeed_Terminated verifies that when the repository returns exactly
// maxHomeFeedDepth posts with terminated=true, the page is fully terminated.
func TestGetHomeFeed_Terminated(t *testing.T) {
	posts := makePosts(maxHomeFeedDepth)
	repo := &fakeRepository{posts: posts, nextCursor: "", terminated: true}
	svc := newTestService(repo, noopBlockProvider(), noopFollowProvider())

	page, err := svc.GetHomeFeed(context.Background(), uuid.New(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true")
	}
	if len(page.Items) != maxHomeFeedDepth {
		t.Errorf("expected %d items, got %d", maxHomeFeedDepth, len(page.Items))
	}
}

// TestGetHomeFeed_NotTerminated verifies that when the repository returns fewer
// than maxHomeFeedDepth posts with terminated=false, a cursor is present.
func TestGetHomeFeed_NotTerminated(t *testing.T) {
	posts := makePosts(10)
	// Build a valid cursor from the last post.
	last := posts[len(posts)-1]
	cur := post.FeedCursor{AfterID: last.ID, Timestamp: last.CreatedAt}
	encoded := cur.Encode()

	repo := &fakeRepository{posts: posts, nextCursor: encoded, terminated: false}
	svc := newTestService(repo, noopBlockProvider(), noopFollowProvider())

	page, err := svc.GetHomeFeed(context.Background(), uuid.New(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Terminated {
		t.Error("expected Terminated=false")
	}
	if page.NextCursor == "" {
		t.Error("expected non-empty NextCursor when not terminated")
	}
	if len(page.Items) != 10 {
		t.Errorf("expected 10 items, got %d", len(page.Items))
	}
}

// TestGetHomeFeed_BadCursor verifies that a malformed cursor string results in
// a CodeValidation error.
func TestGetHomeFeed_BadCursor(t *testing.T) {
	repo := &fakeRepository{}
	svc := newTestService(repo, noopBlockProvider(), noopFollowProvider())

	_, err := svc.GetHomeFeed(context.Background(), uuid.New(), "not-a-valid-cursor!!!")
	if err == nil {
		t.Fatal("expected error for bad cursor, got nil")
	}

	var apiErr *apierror.APIError
	ok := false
	// errors.As works for *apierror.APIError.
	switch e := err.(type) {
	case *apierror.APIError:
		apiErr = e
		ok = true
	}
	if !ok {
		t.Fatalf("expected *apierror.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != apierror.CodeValidation {
		t.Errorf("expected CodeValidation, got %q", apiErr.Code)
	}
}

// TestGetHomeFeed_FiltersBadCursorFormat is a second bad-cursor test confirming
// the exact error code on a different invalid format (base64 garbage).
func TestGetHomeFeed_FiltersBadCursorFormat(t *testing.T) {
	repo := &fakeRepository{}
	svc := newTestService(repo, noopBlockProvider(), noopFollowProvider())

	_, err := svc.GetHomeFeed(context.Background(), uuid.New(), "aGVsbG8=")
	if err == nil {
		t.Fatal("expected error for bad cursor, got nil")
	}

	var apiErr *apierror.APIError
	switch e := err.(type) {
	case *apierror.APIError:
		apiErr = e
	default:
		t.Fatalf("expected *apierror.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != apierror.CodeValidation {
		t.Errorf("expected CodeValidation, got %q", apiErr.Code)
	}
}
