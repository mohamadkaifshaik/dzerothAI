package title

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// workerBatchSize caps the number of user IDs fetched per cursor page.
	// Bounded batches prevent loading all users into memory.
	workerBatchSize = 500

	// gracePeriodDuration is the 48-hour window granted before a revocable title
	// that no longer qualifies is finally revoked.
	gracePeriodDuration = 48 * time.Hour
)

// workerRepository is the subset of Repository methods used by the worker.
// Unexported so only *Repository (same package) satisfies it naturally, and
// fake implementations can be used in tests without hitting the database.
type workerRepository interface {
	GetUserIDsBatch(ctx context.Context, afterID uuid.UUID, limit int) ([]uuid.UUID, error)
	GetUserTitles(ctx context.Context, userID uuid.UUID) ([]UserTitleEntry, error)
	EnterGracePeriod(ctx context.Context, userTitleID uuid.UUID, endsAt time.Time) error
	RestoreGracePeriodTitle(ctx context.Context, userTitleID uuid.UUID) error
	RevokeUserTitleTx(ctx context.Context, userTitleID, userID uuid.UUID, revokedAt time.Time) error
}

// workerEngine is the subset of Engine methods used by the worker.
// Unexported for the same reasons as workerRepository.
type workerEngine interface {
	EvaluateAndUnlock(ctx context.Context, userID uuid.UUID) ([]QualificationResult, error)
	Qualify(ctx context.Context, userID uuid.UUID, slug string) (QualificationResult, error)
}

// WorkerConfig holds configuration for TitleQualificationWorker.
type WorkerConfig struct {
	// Interval controls how often the worker runs a full reconciliation pass.
	// Default: 5 minutes.
	Interval time.Duration
}

// TitleQualificationWorker periodically evaluates title qualification for all
// users and reconciles lifecycle state (active → grace_period → revoked and
// grace_period → active restores).
//
// The worker processes users in deterministic cursor batches of workerBatchSize
// to avoid loading all user IDs into memory. A full pass blocks the ticker so
// overlapping reconciliation runs are naturally prevented.
//
// Participation in the application workerWG follows the existing pattern:
//
//	workerWG.Add(1)
//	go func() { defer workerWG.Done(); worker.Run(workerCtx) }()
type TitleQualificationWorker struct {
	engine workerEngine
	repo   workerRepository
	cfg    WorkerConfig
	logger *zap.Logger
}

// NewTitleQualificationWorker constructs a TitleQualificationWorker.
func NewTitleQualificationWorker(engine *Engine, repo *Repository, cfg WorkerConfig, logger *zap.Logger) *TitleQualificationWorker {
	return &TitleQualificationWorker{
		engine: engine,
		repo:   repo,
		cfg:    cfg,
		logger: logger,
	}
}

// newTitleQualificationWorkerFromIfaces is the internal constructor used by
// unit tests to inject fake implementations.
func newTitleQualificationWorkerFromIfaces(engine workerEngine, repo workerRepository, cfg WorkerConfig, logger *zap.Logger) *TitleQualificationWorker {
	return &TitleQualificationWorker{
		engine: engine,
		repo:   repo,
		cfg:    cfg,
		logger: logger,
	}
}

// Run starts the worker loop. It blocks until ctx is cancelled.
func (w *TitleQualificationWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()
	w.logger.Info("title qualification worker started", zap.Duration("interval", w.cfg.Interval))
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("title qualification worker stopped")
			return
		case <-ticker.C:
			w.runReconciliation(ctx)
		}
	}
}

// RunOnce performs one full reconciliation pass synchronously. Exported so
// integration tests and callers that need synchronous execution can invoke it
// directly without waiting for the ticker.
func (w *TitleQualificationWorker) RunOnce(ctx context.Context) {
	w.runReconciliation(ctx)
}

// runReconciliation performs a single full pass over all users using cursor
// pagination. It logs aggregate results and continues past per-user errors.
func (w *TitleQualificationWorker) runReconciliation(ctx context.Context) {
	w.logger.Info("title reconciliation started")

	var (
		cursor    = uuid.Nil
		processed int
	)

	for {
		// Check cancellation between batches.
		select {
		case <-ctx.Done():
			w.logger.Info("title reconciliation interrupted by context cancellation",
				zap.Int("users_processed", processed),
			)
			return
		default:
		}

		ids, err := w.repo.GetUserIDsBatch(ctx, cursor, workerBatchSize)
		if err != nil {
			w.logger.Warn("title reconciliation: get user ids batch failed",
				zap.Stringer("cursor", cursor),
				zap.Error(err),
			)
			return
		}

		for _, userID := range ids {
			if err := w.reconcileUser(ctx, userID); err != nil {
				w.logger.Warn("title reconciliation: reconcile user error",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				// Continue — one user's failure must not stop others.
			}
			processed++
		}

		if len(ids) < workerBatchSize {
			// Fewer results than the batch limit — we've exhausted all users.
			break
		}

		// Advance the cursor to the last ID returned.
		cursor = ids[len(ids)-1]
	}

	w.logger.Info("title reconciliation completed", zap.Int("users_processed", processed))
}

// reconcileUser runs title qualification and lifecycle reconciliation for a
// single user. It first calls EvaluateAndUnlock to create any newly earned
// titles (idempotent), then iterates active/grace_period revocable titles to
// apply the appropriate lifecycle transition.
func (w *TitleQualificationWorker) reconcileUser(ctx context.Context, userID uuid.UUID) error {
	// Step 1: unlock any newly qualifying titles. This is idempotent — already
	// active rows are returned as-is via ON CONFLICT DO NOTHING.
	_, err := w.engine.EvaluateAndUnlock(ctx, userID)
	if err != nil {
		return err
	}

	// Step 2: fetch existing active/grace_period titles to reconcile lifecycle.
	entries, err := w.repo.GetUserTitles(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	for _, entry := range entries {
		// Permanent titles (is_revocable=false) are never subject to lifecycle
		// reconciliation — they can only be lost if the user loses eligibility
		// conditions that are checked at unlock time (founding_member, centurion).
		// The product spec does not revoke permanent titles after unlock.
		if !entry.IsRevocable {
			continue
		}

		// Re-evaluate qualification for this specific title.
		res, err := w.engine.Qualify(ctx, userID, entry.Slug)
		if err != nil {
			// ErrNotQualifiable means the engine does not evaluate this slug
			// (e.g. top_1pct_creator). Skip it.
			w.logger.Warn("title reconciliation: qualify error",
				zap.String("user_id", userID.String()),
				zap.String("slug", entry.Slug),
				zap.Error(err),
			)
			continue
		}

		switch entry.Status {
		case StatusActive:
			if res.Qualified {
				// Still qualified — no change.
				continue
			}
			// Lost qualification — enter grace period.
			endsAt := now.Add(gracePeriodDuration)
			if err := w.repo.EnterGracePeriod(ctx, entry.ID, endsAt); err != nil {
				w.logger.Warn("title reconciliation: enter grace period failed",
					zap.String("user_id", userID.String()),
					zap.String("slug", entry.Slug),
					zap.Error(err),
				)
				continue
			}
			w.logger.Info("title entered grace_period",
				zap.String("user_id", userID.String()),
				zap.String("slug", entry.Slug),
				zap.Time("grace_ends_at", endsAt),
			)

		case StatusGracePeriod:
			if res.Qualified {
				// Re-qualified during grace window — restore to active.
				if err := w.repo.RestoreGracePeriodTitle(ctx, entry.ID); err != nil {
					w.logger.Warn("title reconciliation: restore grace period title failed",
						zap.String("user_id", userID.String()),
						zap.String("slug", entry.Slug),
						zap.Error(err),
					)
					continue
				}
				w.logger.Info("title restored to active",
					zap.String("user_id", userID.String()),
					zap.String("slug", entry.Slug),
				)
				continue
			}

			// Still not qualified. Check whether the grace window has expired.
			if entry.GracePeriodEndsAt == nil || now.Before(*entry.GracePeriodEndsAt) {
				// Still within the grace window — no action yet.
				continue
			}

			// Grace window expired — revoke the title and conditionally clear primary.
			if err := w.repo.RevokeUserTitleTx(ctx, entry.ID, userID, now); err != nil {
				w.logger.Warn("title reconciliation: revoke user title failed",
					zap.String("user_id", userID.String()),
					zap.String("slug", entry.Slug),
					zap.Error(err),
				)
				continue
			}
			w.logger.Info("title revoked",
				zap.String("user_id", userID.String()),
				zap.String("slug", entry.Slug),
			)
		}
	}

	return nil
}
