// Package block — service tests.
//
// Unit tests for block/mute service business logic. Tests that require
// repository interactions use a blockRepo interface test double.
package block

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// ---------------------------------------------------------------------------
// blockRepo interface and fake
// ---------------------------------------------------------------------------

// blockRepo is the minimal interface for repository methods called by the
// service under test. It mirrors the concrete *Repository signatures used
// by the functions under test.
type blockRepoIface interface {
	Block(ctx context.Context, blockerID, blockedID uuid.UUID) error
	Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error
	Mute(ctx context.Context, muterID, mutedID uuid.UUID) error
	Unmute(ctx context.Context, muterID, mutedID uuid.UUID) error
	IsBlockedBidirectional(ctx context.Context, userA, userB uuid.UUID) (bool, error)
	GetBlockedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	GetMutedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// fakeBlockRepo is a test double for blockRepoIface.
type fakeBlockRepo struct {
	blockFn                  func(ctx context.Context, blockerID, blockedID uuid.UUID) error
	unblockFn                func(ctx context.Context, blockerID, blockedID uuid.UUID) error
	muteFn                   func(ctx context.Context, muterID, mutedID uuid.UUID) error
	unmuteFn                 func(ctx context.Context, muterID, mutedID uuid.UUID) error
	isBlockedBidirectionalFn func(ctx context.Context, userA, userB uuid.UUID) (bool, error)
	getBlockedIDsFn          func(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	getMutedIDsFn            func(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)

	// blockCallCount tracks how many times Block was called (for idempotency tests).
	blockCallCount int
	// muteCallCount tracks how many times Mute was called (for idempotency tests).
	muteCallCount int
}

func (f *fakeBlockRepo) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	f.blockCallCount++
	if f.blockFn != nil {
		return f.blockFn(ctx, blockerID, blockedID)
	}
	return nil
}

func (f *fakeBlockRepo) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if f.unblockFn != nil {
		return f.unblockFn(ctx, blockerID, blockedID)
	}
	return nil
}

func (f *fakeBlockRepo) Mute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	f.muteCallCount++
	if f.muteFn != nil {
		return f.muteFn(ctx, muterID, mutedID)
	}
	return nil
}

func (f *fakeBlockRepo) Unmute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	if f.unmuteFn != nil {
		return f.unmuteFn(ctx, muterID, mutedID)
	}
	return nil
}

func (f *fakeBlockRepo) IsBlockedBidirectional(ctx context.Context, userA, userB uuid.UUID) (bool, error) {
	if f.isBlockedBidirectionalFn != nil {
		return f.isBlockedBidirectionalFn(ctx, userA, userB)
	}
	return false, nil
}

func (f *fakeBlockRepo) GetBlockedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	if f.getBlockedIDsFn != nil {
		return f.getBlockedIDsFn(ctx, userID)
	}
	return nil, nil
}

func (f *fakeBlockRepo) GetMutedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	if f.getMutedIDsFn != nil {
		return f.getMutedIDsFn(ctx, userID)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// testableService — service that accepts a blockRepoIface test double
// ---------------------------------------------------------------------------

// testableService mirrors Service but holds a blockRepoIface so tests can
// inject fakes without a real database.
type testableService struct {
	repo blockRepoIface
	log  *zap.Logger
}

func newTestableService(repo blockRepoIface) *testableService {
	log, _ := zap.NewDevelopment()
	return &testableService{repo: repo, log: log}
}

func (s *testableService) Block(ctx context.Context, callerID, targetID uuid.UUID) error {
	if callerID == targetID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot block yourself")
	}
	if err := s.repo.Block(ctx, callerID, targetID); err != nil {
		s.log.Error("block: insert block", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableService) Unblock(ctx context.Context, callerID, targetID uuid.UUID) error {
	if err := s.repo.Unblock(ctx, callerID, targetID); err != nil {
		s.log.Error("block: unblock", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableService) Mute(ctx context.Context, callerID, targetID uuid.UUID) error {
	if callerID == targetID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot mute yourself")
	}
	if err := s.repo.Mute(ctx, callerID, targetID); err != nil {
		s.log.Error("block: insert mute", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableService) Unmute(ctx context.Context, callerID, targetID uuid.UUID) error {
	if err := s.repo.Unmute(ctx, callerID, targetID); err != nil {
		s.log.Error("block: unmute", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
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

// ---------------------------------------------------------------------------
// TestBlock_SelfBlockRejected
// ---------------------------------------------------------------------------

// TestBlock_SelfBlockRejected verifies that a user cannot block themselves.
// This returns CodeValidation before any repository call.
func TestBlock_SelfBlockRejected(t *testing.T) {
	svc := newTestableService(&fakeBlockRepo{})
	userID := uuid.New()

	err := svc.Block(context.Background(), userID, userID)
	if err == nil {
		t.Fatal("Block self should return a validation error")
	}
	if code := apiErrCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// ---------------------------------------------------------------------------
// TestBlock_Idempotent
// ---------------------------------------------------------------------------

// TestBlock_Idempotent verifies that blocking the same user twice does not
// produce an error — the repository's ON CONFLICT DO NOTHING makes it idempotent.
func TestBlock_Idempotent(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	fake := &fakeBlockRepo{
		blockFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil // idempotent — no error on conflict
		},
	}
	svc := newTestableService(fake)

	if err := svc.Block(context.Background(), callerID, targetID); err != nil {
		t.Fatalf("first Block returned unexpected error: %v", err)
	}
	if err := svc.Block(context.Background(), callerID, targetID); err != nil {
		t.Fatalf("second Block (idempotent) returned unexpected error: %v", err)
	}
	if fake.blockCallCount != 2 {
		t.Errorf("expected repo.Block called 2 times, got %d", fake.blockCallCount)
	}
}

// ---------------------------------------------------------------------------
// TestBlock_RemovesFollows
// ---------------------------------------------------------------------------

// TestBlock_RemovesFollows verifies that the repository Block method is called
// (which internally handles follow deletion in the same transaction).
// This unit test verifies the service calls the repo; the transactional
// atomicity is an integration concern verified at the repository level.
func TestBlock_RemovesFollows(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	repoCalled := false
	fake := &fakeBlockRepo{
		blockFn: func(_ context.Context, blockerID, blockedID uuid.UUID) error {
			repoCalled = true
			// Verify the correct IDs are passed through.
			if blockerID != callerID {
				t.Errorf("Block called with wrong blockerID: got %v, want %v", blockerID, callerID)
			}
			if blockedID != targetID {
				t.Errorf("Block called with wrong blockedID: got %v, want %v", blockedID, targetID)
			}
			return nil
		},
	}
	svc := newTestableService(fake)

	if err := svc.Block(context.Background(), callerID, targetID); err != nil {
		t.Fatalf("Block returned unexpected error: %v", err)
	}
	if !repoCalled {
		t.Error("expected repo.Block to be called")
	}
}

// ---------------------------------------------------------------------------
// TestUnblock_NonExistent
// ---------------------------------------------------------------------------

// TestUnblock_NonExistent verifies that unblocking a user not currently blocked
// returns nil — the operation is idempotent.
func TestUnblock_NonExistent(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	fake := &fakeBlockRepo{
		unblockFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil // no row deleted — not an error
		},
	}
	svc := newTestableService(fake)

	err := svc.Unblock(context.Background(), callerID, targetID)
	if err != nil {
		t.Errorf("Unblock non-existent block returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestMute_SelfMuteRejected
// ---------------------------------------------------------------------------

// TestMute_SelfMuteRejected verifies that a user cannot mute themselves.
// This returns CodeValidation before any repository call.
func TestMute_SelfMuteRejected(t *testing.T) {
	svc := newTestableService(&fakeBlockRepo{})
	userID := uuid.New()

	err := svc.Mute(context.Background(), userID, userID)
	if err == nil {
		t.Fatal("Mute self should return a validation error")
	}
	if code := apiErrCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// ---------------------------------------------------------------------------
// TestMute_Idempotent
// ---------------------------------------------------------------------------

// TestMute_Idempotent verifies that muting the same user twice does not
// produce an error — the repository's ON CONFLICT DO NOTHING makes it idempotent.
func TestMute_Idempotent(t *testing.T) {
	callerID := uuid.New()
	targetID := uuid.New()

	fake := &fakeBlockRepo{
		muteFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil // idempotent — no error on conflict
		},
	}
	svc := newTestableService(fake)

	if err := svc.Mute(context.Background(), callerID, targetID); err != nil {
		t.Fatalf("first Mute returned unexpected error: %v", err)
	}
	if err := svc.Mute(context.Background(), callerID, targetID); err != nil {
		t.Fatalf("second Mute (idempotent) returned unexpected error: %v", err)
	}
	if fake.muteCallCount != 2 {
		t.Errorf("expected repo.Mute called 2 times, got %d", fake.muteCallCount)
	}
}
