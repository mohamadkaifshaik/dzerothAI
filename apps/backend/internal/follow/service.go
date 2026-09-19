package follow

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/notification"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/ctxlog"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// maxFollowDepth is the server-enforced hard limit on follower/following list
// depth per page (CLAUDE.md §2.1 — no infinite scrolling).
const maxFollowDepth = 200

// Service implements follow domain business logic.
type Service struct {
	repo     *Repository
	log      *zap.Logger
	notifier notification.NotificationPublisher
}

// NewService constructs a follow Service.
func NewService(repo *Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// SetNotificationPublisher injects a NotificationPublisher dependency. Must be
// called after NewService and before the first request is served. Concurrency-safe
// if called during the single-threaded startup phase before the HTTP server starts.
func (s *Service) SetNotificationPublisher(np notification.NotificationPublisher) {
	s.notifier = np
}

// Follow creates a directed follow from callerID → targetID.
//
// Rules enforced:
//   - callerID must not equal targetID (CodeValidation)
//   - targetID must not have blocked callerID (CodeForbidden)
//   - duplicate follows are idempotent (repo uses ON CONFLICT DO NOTHING)
func (s *Service) Follow(ctx context.Context, callerID, targetID uuid.UUID) error {
	if callerID == targetID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot follow yourself")
	}

	blocked, err := s.repo.IsBlockedBy(ctx, callerID, targetID)
	if err != nil {
		s.log.Error("follow: is blocked by check", ctxlog.RequestIDField(ctx), zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if blocked {
		return apierror.NewAPIError(apierror.CodeForbidden, "cannot follow this user")
	}

	if err := s.repo.Follow(ctx, callerID, targetID); err != nil {
		s.log.Error("follow: insert", ctxlog.RequestIDField(ctx), zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	// Publish best-effort follow notification. Failure must not fail the primary
	// follow operation (CLAUDE.md reliability rules).
	if s.notifier != nil {
		if pubErr := s.notifier.Publish(ctx, notification.PublishEvent{
			RecipientID: targetID,
			ActorID:     callerID,
			Event:       notification.EventFollow,
			PostID:      nil,
		}); pubErr != nil {
			s.log.Warn("follow: publish notification failed",
				ctxlog.RequestIDField(ctx),
				zap.String("caller_id", callerID.String()),
				zap.String("target_id", targetID.String()),
				zap.Error(pubErr),
			)
		}
	}

	return nil
}

// Unfollow removes the directed follow from callerID → targetID.
// Returns nil if the follow row did not exist — unfollow is idempotent.
func (s *Service) Unfollow(ctx context.Context, callerID, targetID uuid.UUID) error {
	if err := s.repo.Unfollow(ctx, callerID, targetID); err != nil {
		s.log.Error("follow: unfollow", ctxlog.RequestIDField(ctx), zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// ListFollowing returns a finite, cursor-paginated FollowPage of users that
// targetID follows. The server-enforced maximum depth is maxFollowDepth (200).
func (s *Service) ListFollowing(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error) {
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

// ListFollowers returns a finite, cursor-paginated FollowPage of users that
// follow targetID. The server-enforced maximum depth is maxFollowDepth (200).
func (s *Service) ListFollowers(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error) {
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

// IsFollowing returns whether callerID currently follows targetID.
// Used by profile and feed logic.
func (s *Service) IsFollowing(ctx context.Context, callerID, targetID uuid.UUID) (bool, error) {
	following, err := s.repo.IsFollowing(ctx, callerID, targetID)
	if err != nil {
		s.log.Error("follow: is following", zap.Error(err))
		return false, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return following, nil
}
