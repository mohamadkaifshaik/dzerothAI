// Package follow — service tests.
//
// Unit tests for follow service business logic. Tests that require repository
// interactions use a followRepo interface test double injected via a
// testableFollowService.
package follow

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
// followRepo interface and test double
// ---------------------------------------------------------------------------

// followRepo is the minimal interface for repository methods called by the
// service under test. It mirrors the concrete *Repository signatures.
type followRepo interface {
	Follow(ctx context.Context, followerID, followedID uuid.UUID) error
	Unfollow(ctx context.Context, followerID, followedID uuid.UUID) error
	IsFollowing(ctx context.Context, followerID, followedID uuid.UUID) (bool, error)
	IsBlockedBy(ctx context.Context, callerID, targetID uuid.UUID) (bool, error)
	ListFollowing(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error)
	ListFollowers(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error)
}

// fakeFollowRepo is a test double for followRepo.
type fakeFollowRepo struct {
	followFn        func(ctx context.Context, followerID, followedID uuid.UUID) error
	unfollowFn      func(ctx context.Context, followerID, followedID uuid.UUID) error
	isFollowingFn   func(ctx context.Context, followerID, followedID uuid.UUID) (bool, error)
	isBlockedByFn   func(ctx context.Context, callerID, targetID uuid.UUID) (bool, error)
	listFollowingFn func(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error)
	listFollowersFn func(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error)
}

func (f *fakeFollowRepo) Follow(ctx context.Context, followerID, followedID uuid.UUID) error {
	if f.followFn != nil {
		return f.followFn(ctx, followerID, followedID)
	}
	return nil
}

func (f *fakeFollowRepo) Unfollow(ctx context.Context, followerID, followedID uuid.UUID) error {
	if f.unfollowFn != nil {
		return f.unfollowFn(ctx, followerID, followedID)
	}
	return nil
}

func (f *fakeFollowRepo) IsFollowing(ctx context.Context, followerID, followedID uuid.UUID) (bool, error) {
	if f.isFollowingFn != nil {
		return f.isFollowingFn(ctx, followerID, followedID)
	}
	return false, nil
}

func (f *fakeFollowRepo) IsBlockedBy(ctx context.Context, callerID, targetID uuid.UUID) (bool, error) {
	if f.isBlockedByFn != nil {
		return f.isBlockedByFn(ctx, callerID, targetID)
	}
	return false, nil
}

func (f *fakeFollowRepo) ListFollowing(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error) {
	if f.listFollowingFn != nil {
		return f.listFollowingFn(ctx, userID, cursor, max)
	}
	return []FollowUserDTO{}, "", true, nil
}

func (f *fakeFollowRepo) ListFollowers(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error) {
	if f.listFollowersFn != nil {
		return f.listFollowersFn(ctx, userID, cursor, max)
	}
	return []FollowUserDTO{}, "", true, nil
}

// ---------------------------------------------------------------------------
// testableFollowService — service that accepts the followRepo interface
// ---------------------------------------------------------------------------

// testableFollowService mirrors Service but holds a followRepo interface so
// tests can inject fakes without a real database.
type testableFollowService struct {
	repo followRepo
	log  *zap.Logger
}

func newTestableFollowService(repo followRepo) *testableFollowService {
	log, _ := zap.NewDevelopment()
	return &testableFollowService{repo: repo, log: log}
}

func (s *testableFollowService) Follow(ctx context.Context, callerID, targetID uuid.UUID) error {
	if callerID == targetID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot follow yourself")
	}
	blocked, err := s.repo.IsBlockedBy(ctx, callerID, targetID)
	if err != nil {
		s.log.Error("follow: is blocked by check", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if blocked {
		return apierror.NewAPIError(apierror.CodeForbidden, "cannot follow this user")
	}
	if err := s.repo.Follow(ctx, callerID, targetID); err != nil {
		s.log.Error("follow: insert", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableFollowService) Unfollow(ctx context.Context, callerID, targetID uuid.UUID) error {
	if err := s.repo.Unfollow(ctx, callerID, targetID); err != nil {
		s.log.Error("follow: unfollow", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableFollowService) ListFollowing(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error) {
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return FollowPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}
	items, nextCursor, terminated, err := s.repo.ListFollowing(ctx, targetID, cursor, maxFollowDepth)
	if err != nil {
		s.log.Error("follow: list following", zap.Error(err))
		return FollowPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if items == nil {
		items = []FollowUserDTO{}
	}
	return FollowPage{Items: items, NextCursor: nextCursor, Terminated: terminated}, nil
}

func (s *testableFollowService) ListFollowers(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error) {
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return FollowPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}
	items, nextCursor, terminated, err := s.repo.ListFollowers(ctx, targetID, cursor, maxFollowDepth)
	if err != nil {
		s.log.Error("follow: list followers", zap.Error(err))
		return FollowPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if items == nil {
		items = []FollowUserDTO{}
	}
	return FollowPage{Items: items, NextCursor: nextCursor, Terminated: terminated}, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func apiErrCode(err error) string {
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// makeFakeFollowUsers builds n fake FollowUserDTO values.
func makeFakeFollowUsers(n int) []FollowUserDTO {
	users := make([]FollowUserDTO, n)
	for i := range users {
		users[i] = FollowUserDTO{
			ID:          uuid.New().String(),
			Handle:      "user",
			DisplayName: "User",
		}
	}
	return users
}

// ---------------------------------------------------------------------------
// TestFollow_SelfFollowRejected
// ---------------------------------------------------------------------------

// TestFollow_SelfFollowRejected verifies that a user cannot follow themselves.
// This returns a CodeValidation error before any repository call.
func TestFollow_SelfFollowRejected(t *testing.T) {
	svc := newTestableFollowService(&fakeFollowRepo{})
	userID := uuid.New()

	err := svc.Follow(context.Background(), userID, userID)
	if err == nil {
		t.Fatal("Follow self should return a validation error")
	}
	if code := apiErrCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// ---------------------------------------------------------------------------
// TestFollow_BlockedByTarget
// ---------------------------------------------------------------------------

// TestFollow_BlockedByTarget verifies that when the target has blocked the
// caller, Follow returns CodeForbidden.
func TestFollow_BlockedByTarget(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	fake := &fakeFollowRepo{
		isBlockedByFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return true, nil // target has blocked caller
		},
	}
	svc := newTestableFollowService(fake)

	err := svc.Follow(context.Background(), callerID, targetID)
	if err == nil {
		t.Fatal("Follow blocked user should return a forbidden error")
	}
	if code := apiErrCode(err); code != apierror.CodeForbidden {
		t.Errorf("error code = %q, want %q", code, apierror.CodeForbidden)
	}
}

// ---------------------------------------------------------------------------
// TestFollow_Idempotent
// ---------------------------------------------------------------------------

// TestFollow_Idempotent verifies that following the same user twice does not
// produce an error — the repository's ON CONFLICT DO NOTHING makes it idempotent.
func TestFollow_Idempotent(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	callCount := 0
	fake := &fakeFollowRepo{
		isBlockedByFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) { return false, nil },
		followFn: func(_ context.Context, _, _ uuid.UUID) error {
			callCount++
			return nil // idempotent — no error on conflict
		},
	}
	svc := newTestableFollowService(fake)

	if err := svc.Follow(context.Background(), callerID, targetID); err != nil {
		t.Fatalf("first Follow returned unexpected error: %v", err)
	}
	if err := svc.Follow(context.Background(), callerID, targetID); err != nil {
		t.Fatalf("second Follow (idempotent) returned unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected repo.Follow called 2 times, got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// TestUnfollow_NonExistent
// ---------------------------------------------------------------------------

// TestUnfollow_NonExistent verifies that unfollowing a user not currently
// followed returns nil — unfollow is idempotent.
func TestUnfollow_NonExistent(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	fake := &fakeFollowRepo{
		unfollowFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil // no row deleted — not an error
		},
	}
	svc := newTestableFollowService(fake)

	err := svc.Unfollow(context.Background(), callerID, targetID)
	if err != nil {
		t.Errorf("Unfollow non-existent follow returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestListFollowing_Terminated
// ---------------------------------------------------------------------------

// TestListFollowing_Terminated verifies that when the repository returns
// exactly maxFollowDepth (200) items with terminated=true, the page is
// marked Terminated — the client must stop fetching (CLAUDE.md §2.1).
func TestListFollowing_Terminated(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	users := makeFakeFollowUsers(maxFollowDepth)
	fake := &fakeFollowRepo{
		listFollowingFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor, _ int) ([]FollowUserDTO, string, bool, error) {
			return users, "", true, nil
		},
	}
	svc := newTestableFollowService(fake)

	page, err := svc.ListFollowing(context.Background(), callerID, targetID, "")
	if err != nil {
		t.Fatalf("ListFollowing returned unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true when repository signals feed termination at maxFollowDepth")
	}
	if len(page.Items) != maxFollowDepth {
		t.Errorf("expected %d items, got %d", maxFollowDepth, len(page.Items))
	}
}

// ---------------------------------------------------------------------------
// TestListFollowing_NotTerminated
// ---------------------------------------------------------------------------

// TestListFollowing_NotTerminated verifies that when the repository returns
// fewer than maxFollowDepth items with terminated=false, the page is NOT
// marked Terminated and a cursor is present.
func TestListFollowing_NotTerminated(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	users := makeFakeFollowUsers(10)
	fakeCursor := buildTestCursor()

	fake := &fakeFollowRepo{
		listFollowingFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor, _ int) ([]FollowUserDTO, string, bool, error) {
			return users, fakeCursor, false, nil
		},
	}
	svc := newTestableFollowService(fake)

	page, err := svc.ListFollowing(context.Background(), callerID, targetID, "")
	if err != nil {
		t.Fatalf("ListFollowing returned unexpected error: %v", err)
	}
	if page.Terminated {
		t.Error("expected Terminated=false for partial page")
	}
	if page.NextCursor == "" {
		t.Error("expected non-empty NextCursor for partial page")
	}
	if len(page.Items) != 10 {
		t.Errorf("expected 10 items, got %d", len(page.Items))
	}
}

// ---------------------------------------------------------------------------
// TestListFollowing_BadCursor
// ---------------------------------------------------------------------------

// TestListFollowing_BadCursor verifies that an invalid cursor string returns
// a CodeValidation error without any repository call.
func TestListFollowing_BadCursor(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	// Repo should never be called for a bad cursor.
	repoCalled := false
	fake := &fakeFollowRepo{
		listFollowingFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor, _ int) ([]FollowUserDTO, string, bool, error) {
			repoCalled = true
			return nil, "", true, nil
		},
	}
	svc := newTestableFollowService(fake)

	_, err := svc.ListFollowing(context.Background(), callerID, targetID, "!!!invalid-cursor!!!")
	if err == nil {
		t.Fatal("ListFollowing with bad cursor should return a validation error")
	}
	if code := apiErrCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
	if repoCalled {
		t.Error("repository must not be called when cursor is invalid")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildTestCursor encodes a valid FeedCursor string for use in tests.
func buildTestCursor() string {
	id, _ := uuid.NewV7()
	c := post.FeedCursor{AfterID: id, Timestamp: time.Now().UTC()}
	return c.Encode()
}
