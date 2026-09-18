package block

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// Service implements block and mute business logic.
type Service struct {
	repo *Repository
	log  *zap.Logger
}

// NewService constructs a block Service.
func NewService(repo *Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Block creates a block from callerID → targetID.
//
// Rules enforced:
//   - callerID must not equal targetID (CodeValidation)
//   - The block atomically removes follows in both directions (repository
//     handles this in a single transaction).
//   - Blocking the same user twice is idempotent (ON CONFLICT DO NOTHING).
func (s *Service) Block(ctx context.Context, callerID, targetID uuid.UUID) error {
	if callerID == targetID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot block yourself")
	}

	if err := s.repo.Block(ctx, callerID, targetID); err != nil {
		s.log.Error("block: insert block", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// Unblock removes the block from callerID → targetID.
// Returns nil if the block row did not exist — idempotent.
func (s *Service) Unblock(ctx context.Context, callerID, targetID uuid.UUID) error {
	if err := s.repo.Unblock(ctx, callerID, targetID); err != nil {
		s.log.Error("block: unblock", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// Mute creates a mute from callerID → targetID.
//
// Rules enforced:
//   - callerID must not equal targetID (CodeValidation)
//   - Muting the same user twice is idempotent (ON CONFLICT DO NOTHING).
func (s *Service) Mute(ctx context.Context, callerID, targetID uuid.UUID) error {
	if callerID == targetID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot mute yourself")
	}

	if err := s.repo.Mute(ctx, callerID, targetID); err != nil {
		s.log.Error("block: insert mute", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// Unmute removes the mute from callerID → targetID.
// Returns nil if the mute row did not exist — idempotent.
func (s *Service) Unmute(ctx context.Context, callerID, targetID uuid.UUID) error {
	if err := s.repo.Unmute(ctx, callerID, targetID); err != nil {
		s.log.Error("block: unmute", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// IsBlockedBidirectional returns true if EITHER userA has blocked userB OR
// userB has blocked userA.
//
// This is a thin wrapper used by the user handler for profile protection. A 404
// (not 403) must be returned when this is true so that the block state is not
// revealed to the blocked party (CLAUDE.md §12).
func (s *Service) IsBlockedBidirectional(ctx context.Context, userA, userB uuid.UUID) (bool, error) {
	blocked, err := s.repo.IsBlockedBidirectional(ctx, userA, userB)
	if err != nil {
		s.log.Error("block: is blocked bidirectional", zap.Error(err))
		return false, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return blocked, nil
}

// GetBlockedIDs returns all user IDs that userID has blocked.
// Intended for home timeline feed filtering (Phase 4).
func (s *Service) GetBlockedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.repo.GetBlockedIDs(ctx, userID)
	if err != nil {
		s.log.Error("block: get blocked ids", zap.Error(err))
		return nil, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return ids, nil
}

// GetMutedIDs returns all user IDs that userID has muted.
// Intended for home timeline feed filtering (Phase 4).
func (s *Service) GetMutedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.repo.GetMutedIDs(ctx, userID)
	if err != nil {
		s.log.Error("block: get muted ids", zap.Error(err))
		return nil, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return ids, nil
}
