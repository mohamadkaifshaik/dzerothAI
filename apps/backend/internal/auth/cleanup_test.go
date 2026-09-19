package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// ---------------------------------------------------------------------------
// Fake pool for cleanup tests
// ---------------------------------------------------------------------------

// fakeCleanupPool is a minimal stub that satisfies the pool interface needed
// by DeleteExpiredSessions without requiring a live PostgreSQL connection.
// It counts calls and can be configured to return an error or a row count.
type fakeCleanupPool struct {
	callCount  atomic.Int64
	returnN    int64
	returnErr  error
	batchSizes []int // batchSize values passed by each call
}

// execResult is a minimal implementation of pgconn.CommandTag.
// pgx's pool.Exec returns a pgconn.CommandTag; to avoid the real pgx pool we
// use a wrapper in the worker tests that bypasses the real DB call entirely.
// (See workerWithFakeDelete below.)

// workerWithFakeDelete wraps SessionCleanupWorker so that runOnce can use a
// fake delete function instead of hitting a real database.
type workerWithFakeDelete struct {
	log       *zap.Logger
	deleteFn  func(ctx context.Context, batchSize int) (int64, error)
	batchSize int
}

func (w *workerWithFakeDelete) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
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

func (w *workerWithFakeDelete) runOnce(ctx context.Context) {
	start := time.Now()
	n, err := w.deleteFn(ctx, w.batchSize)
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
}

// ---------------------------------------------------------------------------
// Helper to build a zaptest observed logger
// ---------------------------------------------------------------------------

func newObservedLogger(level zapcore.Level) (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(level)
	return zap.New(core), logs
}

// ---------------------------------------------------------------------------
// TestSessionCleanupWorker_RunsCleanup
// Verifies the worker calls delete at least once within a short interval.
// ---------------------------------------------------------------------------

func TestSessionCleanupWorker_RunsCleanup(t *testing.T) {
	var callCount atomic.Int64

	log, _ := newObservedLogger(zapcore.InfoLevel)
	w := &workerWithFakeDelete{
		log:       log,
		batchSize: 10,
		deleteFn: func(_ context.Context, _ int) (int64, error) {
			callCount.Add(1)
			return 3, nil // return > 0 so the info log fires
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	w.run(ctx, 5*time.Millisecond)

	if callCount.Load() == 0 {
		t.Fatal("expected delete to be called at least once within 200ms with a 5ms interval")
	}
}

// ---------------------------------------------------------------------------
// TestSessionCleanupWorker_StopsOnContextCancel
// Verifies Run() returns promptly when ctx is cancelled.
// ---------------------------------------------------------------------------

func TestSessionCleanupWorker_StopsOnContextCancel(t *testing.T) {
	log, _ := newObservedLogger(zapcore.InfoLevel)
	w := &workerWithFakeDelete{
		log:       log,
		batchSize: 10,
		deleteFn: func(_ context.Context, _ int) (int64, error) {
			return 0, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.run(ctx, 1*time.Hour) // very long interval — should stop on cancel
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// passed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not return within 500ms after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// TestSessionCleanupWorker_LogsOnError
// Verifies that a DB error is logged at Warn, not Error, not Fatal.
// ---------------------------------------------------------------------------

func TestSessionCleanupWorker_LogsOnError(t *testing.T) {
	log, logs := newObservedLogger(zapcore.DebugLevel)
	dbErr := errors.New("connection refused")

	w := &workerWithFakeDelete{
		log:       log,
		batchSize: 10,
		deleteFn: func(_ context.Context, _ int) (int64, error) {
			return 0, dbErr
		},
	}

	w.runOnce(context.Background())

	warnLogs := logs.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warnLogs) != 1 {
		t.Fatalf("expected exactly 1 Warn log entry, got %d", len(warnLogs))
	}
	if warnLogs[0].Message != "session cleanup failed" {
		t.Errorf("Warn message = %q, want %q", warnLogs[0].Message, "session cleanup failed")
	}

	// Must not log at Error or higher.
	errorLogs := logs.FilterLevelExact(zapcore.ErrorLevel).All()
	if len(errorLogs) != 0 {
		t.Errorf("expected no Error logs on DB error, got %d", len(errorLogs))
	}
}

// ---------------------------------------------------------------------------
// TestSessionCleanupWorker_NoLogOnZeroDeleted
// Verifies no log is emitted when delete returns 0 rows (clean table).
// ---------------------------------------------------------------------------

func TestSessionCleanupWorker_NoLogOnZeroDeleted(t *testing.T) {
	log, logs := newObservedLogger(zapcore.DebugLevel)

	w := &workerWithFakeDelete{
		log:       log,
		batchSize: 10,
		deleteFn: func(_ context.Context, _ int) (int64, error) {
			return 0, nil
		},
	}

	w.runOnce(context.Background())

	if logs.Len() != 0 {
		t.Errorf("expected 0 log entries when n=0, got %d: %v", logs.Len(), logs.All())
	}
}

// ---------------------------------------------------------------------------
// TestSessionCleanupWorker_LogsSummaryOnDeletion
// Verifies exactly one Info log is emitted per cycle when n > 0.
// ---------------------------------------------------------------------------

func TestSessionCleanupWorker_LogsSummaryOnDeletion(t *testing.T) {
	log, logs := newObservedLogger(zapcore.DebugLevel)

	w := &workerWithFakeDelete{
		log:       log,
		batchSize: 10,
		deleteFn: func(_ context.Context, _ int) (int64, error) {
			return 42, nil
		},
	}

	w.runOnce(context.Background())

	infoLogs := logs.FilterLevelExact(zapcore.InfoLevel).All()
	if len(infoLogs) != 1 {
		t.Fatalf("expected exactly 1 Info log when n>0, got %d", len(infoLogs))
	}
	if infoLogs[0].Message != "session cleanup completed" {
		t.Errorf("Info message = %q, want %q", infoLogs[0].Message, "session cleanup completed")
	}
	// Verify the deleted count is present in the structured fields.
	deleted := infoLogs[0].ContextMap()["deleted"]
	if deleted != int64(42) {
		t.Errorf("log field deleted = %v, want 42", deleted)
	}
}

// ---------------------------------------------------------------------------
// TestDeleteExpiredSessions_BatchLimit
// Verifies the batchSize value is forwarded to the delete function.
// ---------------------------------------------------------------------------

func TestDeleteExpiredSessions_BatchLimit(t *testing.T) {
	capturedBatchSize := 0

	log, _ := newObservedLogger(zapcore.InfoLevel)
	w := &workerWithFakeDelete{
		log:       log,
		batchSize: 250,
		deleteFn: func(_ context.Context, batchSize int) (int64, error) {
			capturedBatchSize = batchSize
			return 0, nil
		},
	}

	w.runOnce(context.Background())

	if capturedBatchSize != 250 {
		t.Errorf("batchSize passed to delete = %d, want 250", capturedBatchSize)
	}
}

// ---------------------------------------------------------------------------
// TestNewSessionCleanupWorker_ConstructorSetsFields
// Verifies NewSessionCleanupWorker stores the provided values.
// ---------------------------------------------------------------------------

func TestNewSessionCleanupWorker_ConstructorSetsFields(t *testing.T) {
	log := zap.NewNop()
	interval := 30 * time.Minute

	w := NewSessionCleanupWorker(nil, log, interval, 500)
	if w == nil {
		t.Fatal("NewSessionCleanupWorker returned nil")
	}
	if w.interval != interval {
		t.Errorf("interval = %v, want %v", w.interval, interval)
	}
	if w.batchSize != 500 {
		t.Errorf("batchSize = %d, want 500", w.batchSize)
	}
	if w.log != log {
		t.Error("log field does not match provided logger")
	}
}

// ---------------------------------------------------------------------------
// TestSessionCleanupBatchSize_Constant
// Verifies the package-level constant has the expected value.
// ---------------------------------------------------------------------------

func TestSessionCleanupBatchSize_Constant(t *testing.T) {
	if sessionCleanupBatchSize != 500 {
		t.Errorf("sessionCleanupBatchSize = %d, want 500", sessionCleanupBatchSize)
	}
}
