package title

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/notification"
)

// systemActorID is used as the actor_id for system-generated title
// notifications. The nil UUID signals a non-human actor and cannot collide
// with any real user ID. Using uuid.Nil avoids creating a phantom user row
// and is consistent with the deduplication index in the notifications table.
// Note: notification.Service suppresses self-notifications (actor == recipient)
// so the actor must differ from the recipient — uuid.Nil is always safe here.
var systemActorID = uuid.Nil

// titleNotificationRepo is the subset of Repository methods used by
// TitleNotificationWorker. Unexported so fakes can be injected in unit tests.
type titleNotificationRepo interface {
	GetPendingUnlockNotifications(ctx context.Context, limit int) ([]PendingTitleNotification, error)
	GetPendingGraceNotifications(ctx context.Context, limit int) ([]PendingTitleNotification, error)
	MarkUnlockNotificationSent(ctx context.Context, userTitleID uuid.UUID) error
	MarkGraceNotificationSent(ctx context.Context, userTitleID uuid.UUID) error
}

// titleNotificationPublisher is the subset of notification.NotificationPublisher
// used by TitleNotificationWorker. Unexported for the same reason as
// titleNotificationRepo.
type titleNotificationPublisher interface {
	Publish(ctx context.Context, event notification.PublishEvent) error
}

// NotificationWorkerConfig holds configuration for TitleNotificationWorker.
type NotificationWorkerConfig struct {
	// Interval controls how often the worker dispatches pending notifications.
	// Default: 1 minute.
	Interval time.Duration
	// BatchSize caps the number of rows fetched per pass.
	// Default: 100.
	BatchSize int
}

// TitleNotificationWorker periodically queries pending title notifications and
// publishes them via NotificationPublisher.
//
// Failure contract:
//   - Publish first, mark-sent second.
//   - If publish succeeds and mark-sent fails (process crash): the notification
//     may be re-published on the next run. The notifications table deduplication
//     index (notifications_dedup_idx) prevents duplicate rows from reaching the
//     user.
//   - If publish fails: the flag stays false and the row is retried on the next
//     run.
//
// This worker does NOT change grace/active/revoked status, does NOT touch
// primary_title_id, and does NOT duplicate title qualification logic.
type TitleNotificationWorker struct {
	repo      titleNotificationRepo
	publisher titleNotificationPublisher
	cfg       NotificationWorkerConfig
	logger    *zap.Logger
}

// NewTitleNotificationWorker constructs a TitleNotificationWorker using the
// concrete *Repository and notification.NotificationPublisher implementations.
func NewTitleNotificationWorker(
	repo titleNotificationRepo,
	publisher titleNotificationPublisher,
	cfg NotificationWorkerConfig,
	logger *zap.Logger,
) *TitleNotificationWorker {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	return &TitleNotificationWorker{
		repo:      repo,
		publisher: publisher,
		cfg:       cfg,
		logger:    logger,
	}
}

// Run starts the worker loop. Blocks until ctx is cancelled.
func (w *TitleNotificationWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()
	w.logger.Info("title notification worker started", zap.Duration("interval", w.cfg.Interval))
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("title notification worker stopped")
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce performs one full notification dispatch pass synchronously.
// Exported so integration tests and callers that need synchronous execution
// can invoke it directly without waiting for the ticker.
func (w *TitleNotificationWorker) RunOnce(ctx context.Context) {
	w.processUnlockBatch(ctx)
	w.processGraceBatch(ctx)
}

// processUnlockBatch fetches pending unlock notifications and dispatches them.
func (w *TitleNotificationWorker) processUnlockBatch(ctx context.Context) {
	pending, err := w.repo.GetPendingUnlockNotifications(ctx, w.cfg.BatchSize)
	if err != nil {
		w.logger.Warn("title notification worker: get pending unlock notifications failed",
			zap.Error(err),
		)
		return
	}

	for _, p := range pending {
		evt := notification.PublishEvent{
			RecipientID: p.UserID,
			ActorID:     systemActorID,
			Event:       notification.EventTitleUnlocked,
			PostID:      nil,
		}
		if err := w.publisher.Publish(ctx, evt); err != nil {
			w.logger.Warn("title notification worker: publish unlock notification failed",
				zap.String("user_title_id", p.UserTitleID.String()),
				zap.String("user_id", p.UserID.String()),
				zap.String("slug", p.Slug),
				zap.Error(err),
			)
			// Leave flag unset — retry on next run.
			continue
		}
		if err := w.repo.MarkUnlockNotificationSent(ctx, p.UserTitleID); err != nil {
			w.logger.Warn("title notification worker: mark unlock notification sent failed",
				zap.String("user_title_id", p.UserTitleID.String()),
				zap.Error(err),
			)
			// Notification was published; the dedup index prevents duplicates on retry.
		}
	}
}

// processGraceBatch fetches pending grace period notifications and dispatches them.
func (w *TitleNotificationWorker) processGraceBatch(ctx context.Context) {
	pending, err := w.repo.GetPendingGraceNotifications(ctx, w.cfg.BatchSize)
	if err != nil {
		w.logger.Warn("title notification worker: get pending grace notifications failed",
			zap.Error(err),
		)
		return
	}

	for _, p := range pending {
		evt := notification.PublishEvent{
			RecipientID: p.UserID,
			ActorID:     systemActorID,
			Event:       notification.EventTitleGracePeriod,
			PostID:      nil,
		}
		if err := w.publisher.Publish(ctx, evt); err != nil {
			w.logger.Warn("title notification worker: publish grace notification failed",
				zap.String("user_title_id", p.UserTitleID.String()),
				zap.String("user_id", p.UserID.String()),
				zap.String("slug", p.Slug),
				zap.Error(err),
			)
			// Leave flag unset — retry on next run.
			continue
		}
		if err := w.repo.MarkGraceNotificationSent(ctx, p.UserTitleID); err != nil {
			w.logger.Warn("title notification worker: mark grace notification sent failed",
				zap.String("user_title_id", p.UserTitleID.String()),
				zap.Error(err),
			)
			// Notification was published; the dedup index prevents duplicates on retry.
		}
	}
}
