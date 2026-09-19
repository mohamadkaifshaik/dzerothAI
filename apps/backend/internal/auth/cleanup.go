package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// sessionCleanupBatchSize is the maximum number of expired session rows removed
// per cleanup cycle. Bounded batches prevent a single cleanup run from holding
// long-duration locks or consuming excessive I/O on tables with many expired rows.
const sessionCleanupBatchSize = 500

// DeleteExpiredSessions removes up to batchSize expired session rows.
// Expired sessions are those whose expires_at timestamp is in the past.
// Revoked sessions are never stored — logout deletes the row immediately — so
// expires_at < NOW() is the only cleanup criterion needed.
//
// The query uses the ctid-based sub-select pattern to apply LIMIT inside a DELETE,
// which is not directly supported by PostgreSQL's DELETE syntax. The
// sessions_expires_at_idx index (created in migration 0002) makes the sub-select
// range scan efficient.
//
// Safe to call concurrently across multiple API instances: two instances deleting
// the same expired row race harmlessly — one succeeds, the other deletes zero rows.
//
// Returns the number of rows deleted and any database error.
func DeleteExpiredSessions(ctx context.Context, pool *pgxpool.Pool, batchSize int) (int64, error) {
	const q = `
		DELETE FROM sessions
		WHERE ctid IN (
			SELECT ctid FROM sessions
			WHERE expires_at < NOW()
			ORDER BY expires_at
			LIMIT $1
		)`

	tag, err := pool.Exec(ctx, q, batchSize)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// SessionCleanupWorker runs periodic cleanup of expired sessions in the background.
// It must be started via Run after the application is ready and stopped by
// cancelling the context passed to Run.
type SessionCleanupWorker struct {
	pool      *pgxpool.Pool
	log       *zap.Logger
	interval  time.Duration
	batchSize int
}

// NewSessionCleanupWorker constructs a SessionCleanupWorker.
// interval controls how often cleanup runs; batchSize caps rows removed per cycle.
func NewSessionCleanupWorker(pool *pgxpool.Pool, log *zap.Logger, interval time.Duration, batchSize int) *SessionCleanupWorker {
	return &SessionCleanupWorker{
		pool:      pool,
		log:       log,
		interval:  interval,
		batchSize: batchSize,
	}
}

// Run starts the cleanup loop. It blocks until ctx is cancelled.
// Errors from individual cleanup cycles are logged at Warn level and do not
// terminate the loop — the worker continues at the next interval.
func (w *SessionCleanupWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce executes a single cleanup cycle. Database errors are logged at Warn
// (cleanup is non-critical; a failed cycle is retried at the next interval).
// When zero rows are deleted no log entry is emitted to avoid noise on clean tables.
func (w *SessionCleanupWorker) runOnce(ctx context.Context) {
	start := time.Now()
	n, err := DeleteExpiredSessions(ctx, w.pool, w.batchSize)
	if err != nil {
		w.log.Warn("session cleanup failed",
			zap.Error(err),
			zap.Duration("duration", time.Since(start)),
		)
		return
	}
	if n > 0 {
		w.log.Info("session cleanup completed",
			zap.Int64("deleted", n),
			zap.Duration("duration", time.Since(start)),
		)
	}
	// n == 0: table is already clean — do not log to avoid constant noise.
}
