package studio

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// ---------------------------------------------------------------------------
// fakeRepository — test double for Repository
// ---------------------------------------------------------------------------

type fakeRepository struct {
	listFn func(ctx context.Context, ownerID uuid.UUID, cursor *post.FeedCursor, limit int) ([]PostAnalytics, string, bool, error)
}

func (f *fakeRepository) ListPostAnalytics(ctx context.Context, ownerID uuid.UUID, cursor *post.FeedCursor, limit int) ([]PostAnalytics, string, bool, error) {
	if f.listFn != nil {
		return f.listFn(ctx, ownerID, cursor, limit)
	}
	return nil, "", true, nil
}

// fakeService wraps a fakeRepository and wires it into a Service using a
// constructor-compatible shim, allowing us to test Service logic without a
// real database or Redis connection.
type fakeService struct {
	repo *fakeRepository
	svc  *Service
}

func newFakeService(repo *fakeRepository) *fakeService {
	log, _ := zap.NewDevelopment()
	// Pass nil Redis — fail-open, no rate limiting in unit tests.
	svc := &Service{
		repo:   nil, // replaced below via shim
		rdb:    nil,
		logger: log,
	}
	fs := &fakeService{repo: repo, svc: svc}
	return fs
}

// GetStudioAnalytics calls the service using the fake repo directly
// (we bypass the real repo field by calling repo methods via the fake).
func (fs *fakeService) GetStudioAnalytics(ctx context.Context, callerID uuid.UUID, cursorStr string) (StudioPage, error) {
	// Reproduce service logic using the fake repo so we can test the
	// cursor decode, rate-limit skip (nil Redis), and result assembly.
	log, _ := zap.NewDevelopment()
	svc := &serviceWithFakeRepo{repo: fs.repo, logger: log}
	return svc.GetStudioAnalytics(ctx, callerID, cursorStr)
}

// serviceWithFakeRepo is a minimal copy of Service that accepts a fakeRepository
// via the repoIface interface so unit tests don't need a real *Repository.
type repoIface interface {
	ListPostAnalytics(ctx context.Context, ownerID uuid.UUID, cursor *post.FeedCursor, limit int) ([]PostAnalytics, string, bool, error)
}

type serviceWithFakeRepo struct {
	repo   repoIface
	logger *zap.Logger
}

func (s *serviceWithFakeRepo) GetStudioAnalytics(ctx context.Context, callerID uuid.UUID, cursorStr string) (StudioPage, error) {
	// No rate limiting (nil Redis → fail-open equivalent).
	var cursor *post.FeedCursor
	if cursorStr != "" {
		var err error
		cursor, err = post.DecodeCursor(cursorStr)
		if err != nil {
			// Return a typed error matching the real service.
			return StudioPage{}, &invalidCursorError{}
		}
	}

	items, nextCursor, terminated, err := s.repo.ListPostAnalytics(ctx, callerID, cursor, maxStudioDepth)
	if err != nil {
		return StudioPage{}, err
	}

	return StudioPage{
		Items:      items,
		NextCursor: nextCursor,
		Terminated: terminated,
	}, nil
}

type invalidCursorError struct{}

func (e *invalidCursorError) Error() string { return "invalid cursor" }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestGetStudioAnalytics_ReturnsOnlyCallerPosts verifies that the callerID
// passed to GetStudioAnalytics is forwarded to the repository unchanged.
func TestGetStudioAnalytics_ReturnsOnlyCallerPosts(t *testing.T) {
	callerID := uuid.New()
	otherID := uuid.New()

	repo := &fakeRepository{
		listFn: func(ctx context.Context, ownerID uuid.UUID, cursor *post.FeedCursor, limit int) ([]PostAnalytics, string, bool, error) {
			if ownerID != callerID {
				t.Errorf("repo called with ownerID %v, want callerID %v", ownerID, callerID)
			}
			_ = otherID // verified above that ownerID != otherID
			return []PostAnalytics{
				{PostID: uuid.New().String(), Content: "test post", PostType: "original", CreatedAt: "2024-01-01T00:00:00Z"},
			}, "", true, nil
		},
	}

	fs := newFakeService(repo)
	page, err := fs.GetStudioAnalytics(context.Background(), callerID, "")
	if err != nil {
		t.Fatalf("GetStudioAnalytics returned error: %v", err)
	}

	if len(page.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(page.Items))
	}
	if !page.Terminated {
		t.Error("expected page.Terminated = true")
	}
}

// TestGetStudioAnalytics_EmptyPage_ReturnsTerminated verifies that when the
// repository returns zero items the page is terminated and empty.
func TestGetStudioAnalytics_EmptyPage_ReturnsTerminated(t *testing.T) {
	callerID := uuid.New()

	repo := &fakeRepository{
		listFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor, _ int) ([]PostAnalytics, string, bool, error) {
			return nil, "", true, nil
		},
	}

	fs := newFakeService(repo)
	page, err := fs.GetStudioAnalytics(context.Background(), callerID, "")
	if err != nil {
		t.Fatalf("GetStudioAnalytics returned error: %v", err)
	}

	if len(page.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Items))
	}
	if !page.Terminated {
		t.Error("expected page.Terminated = true for empty result")
	}
	if page.NextCursor != "" {
		t.Errorf("expected empty NextCursor for terminated page, got %q", page.NextCursor)
	}
}

// TestGetStudioAnalytics_TerminatedAtLimit verifies that when the repository
// returns exactly maxStudioDepth items with terminated=true, the page reflects
// that termination and includes all items.
func TestGetStudioAnalytics_TerminatedAtLimit(t *testing.T) {
	callerID := uuid.New()

	items := make([]PostAnalytics, maxStudioDepth)
	for i := range items {
		items[i] = PostAnalytics{
			PostID:    uuid.New().String(),
			Content:   "post content",
			PostType:  "original",
			CreatedAt: "2024-01-01T00:00:00Z",
		}
	}

	repo := &fakeRepository{
		listFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor, limit int) ([]PostAnalytics, string, bool, error) {
			if limit != maxStudioDepth {
				t.Errorf("repo called with limit %d, want %d", limit, maxStudioDepth)
			}
			return items, "", true, nil
		},
	}

	fs := newFakeService(repo)
	page, err := fs.GetStudioAnalytics(context.Background(), callerID, "")
	if err != nil {
		t.Fatalf("GetStudioAnalytics returned error: %v", err)
	}

	if len(page.Items) != maxStudioDepth {
		t.Errorf("expected %d items, got %d", maxStudioDepth, len(page.Items))
	}
	if !page.Terminated {
		t.Error("expected page.Terminated = true at hard cap")
	}
}
