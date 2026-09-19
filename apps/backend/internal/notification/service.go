package notification

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// Service implements notification business logic.
// It satisfies the NotificationPublisher interface.
type Service struct {
	repo *Repository
	log  *zap.Logger
}

// NewService constructs a notification Service.
func NewService(repo *Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Publish inserts a notification for the given event.
// Self-notifications (actor == recipient) are silently suppressed and never
// written to the database — this is a Dzeroth product invariant.
// Delegates synchronously to the repository; no fan-out.
//
// Publish satisfies the NotificationPublisher interface.
func (s *Service) Publish(ctx context.Context, event PublishEvent) error {
	if event.ActorID == event.RecipientID {
		// Self-notifications are suppressed at the service boundary.
		return nil
	}

	if err := s.repo.Publish(ctx, event); err != nil {
		s.log.Error("notification: publish", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// ListNotifications returns the caller's paginated notification inbox.
// Decodes the cursor, queries the repository, and returns a NotificationPage.
// Notifications are owner-only: callerID is always the JWT-derived identity.
func (s *Service) ListNotifications(ctx context.Context, callerID uuid.UUID, cursorStr string) (NotificationPage, error) {
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return NotificationPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	page, err := s.repo.ListNotifications(ctx, callerID, cursor)
	if err != nil {
		s.log.Error("notification: list", zap.Error(err))
		return NotificationPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return page, nil
}

// MarkAllRead marks all unread notifications for the caller as read.
// Notifications are owner-only: callerID is always the JWT-derived identity.
func (s *Service) MarkAllRead(ctx context.Context, callerID uuid.UUID) error {
	if err := s.repo.MarkAllRead(ctx, callerID); err != nil {
		s.log.Error("notification: mark all read", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}
