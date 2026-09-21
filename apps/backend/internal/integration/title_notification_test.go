//go:build integration

// Title notification worker integration tests.
//
// Validates Phase 6 notification delivery against real PostgreSQL with all
// 0001–0017 migrations applied. Every test exercises the full chain:
//
//	TitleNotificationWorker → title.Repository → pgx → PostgreSQL
//	                        → notification.Service → notification.Repository → PostgreSQL
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/notification"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/title"
)

// newNotificationWorker builds a real TitleNotificationWorker backed by the
// test database, wiring the real notification.Repository and notification.Service.
func newNotificationWorker(titleRepo *title.Repository, notifRepo *notification.Repository) *title.TitleNotificationWorker {
	notifSvc := notification.NewService(notifRepo, zap.NewNop())
	cfg := title.NotificationWorkerConfig{
		Interval:  time.Minute, // ticker not used in RunOnce
		BatchSize: 100,
	}
	return title.NewTitleNotificationWorker(titleRepo, notifSvc, cfg, zap.NewNop())
}

// ---------------------------------------------------------------------------
// 1. TestTitleNotification_UnlockPending
// ---------------------------------------------------------------------------

// TestTitleNotification_UnlockPending verifies that an active user_title with
// unlock_notification_sent=FALSE is processed by the worker, which publishes
// the notification and sets the flag to TRUE.
func TestTitleNotification_UnlockPending(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	titleRepo := title.NewRepository(pool)
	notifRepo := notification.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := titleRepo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	// Verify flag starts false.
	if ut.UnlockNotificationSent {
		t.Fatal("unlock_notification_sent should be FALSE on creation")
	}

	w := newNotificationWorker(titleRepo, notifRepo)
	w.RunOnce(ctx)

	// Verify flag is now TRUE.
	updated, err := titleRepo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if !updated.UnlockNotificationSent {
		t.Error("unlock_notification_sent must be TRUE after worker run")
	}
}

// ---------------------------------------------------------------------------
// 2. TestTitleNotification_GracePending
// ---------------------------------------------------------------------------

// TestTitleNotification_GracePending verifies that a grace_period user_title
// with grace_notification_sent=FALSE is processed by the worker.
func TestTitleNotification_GracePending(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	titleRepo := title.NewRepository(pool)
	notifRepo := notification.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := titleRepo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// Force into grace_period.
	endsAt := time.Now().UTC().Add(24 * time.Hour)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1 WHERE id=$2`,
		endsAt, ut.ID,
	); err != nil {
		t.Fatalf("force grace_period: %v", err)
	}

	w := newNotificationWorker(titleRepo, notifRepo)
	w.RunOnce(ctx)

	// Verify grace flag is now TRUE.
	updated, err := titleRepo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if !updated.GraceNotificationSent {
		t.Error("grace_notification_sent must be TRUE after worker run on grace_period title")
	}
}

// ---------------------------------------------------------------------------
// 3. TestTitleNotification_AlreadySent_Unlock
// ---------------------------------------------------------------------------

// TestTitleNotification_AlreadySent_Unlock verifies that a user_title with
// unlock_notification_sent=TRUE is not processed again by the worker.
func TestTitleNotification_AlreadySent_Unlock(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	titleRepo := title.NewRepository(pool)
	notifRepo := notification.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := titleRepo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// Pre-mark as sent.
	if err := titleRepo.MarkUnlockNotificationSent(ctx, ut.ID); err != nil {
		t.Fatalf("MarkUnlockNotificationSent: %v", err)
	}

	// Count notifications before worker run.
	var before int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND event = 'title_unlocked'`,
		userID,
	).Scan(&before); err != nil {
		t.Fatalf("count notifications before: %v", err)
	}

	w := newNotificationWorker(titleRepo, notifRepo)
	w.RunOnce(ctx)

	// Count after — must not have increased.
	var after int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND event = 'title_unlocked'`,
		userID,
	).Scan(&after); err != nil {
		t.Fatalf("count notifications after: %v", err)
	}
	if after != before {
		t.Errorf("notification count changed from %d to %d; already-sent row must not be processed again", before, after)
	}
}

// ---------------------------------------------------------------------------
// 4. TestTitleNotification_AlreadySent_Grace
// ---------------------------------------------------------------------------

// TestTitleNotification_AlreadySent_Grace verifies that a grace_period user_title
// with grace_notification_sent=TRUE is not processed again.
func TestTitleNotification_AlreadySent_Grace(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	titleRepo := title.NewRepository(pool)
	notifRepo := notification.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := titleRepo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// Force into grace_period with grace_notification_sent=TRUE.
	endsAt := time.Now().UTC().Add(24 * time.Hour)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1, grace_notification_sent=TRUE WHERE id=$2`,
		endsAt, ut.ID,
	); err != nil {
		t.Fatalf("force grace_period with sent=TRUE: %v", err)
	}

	var before int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND event = 'title_grace_period'`,
		userID,
	).Scan(&before); err != nil {
		t.Fatalf("count notifications before: %v", err)
	}

	w := newNotificationWorker(titleRepo, notifRepo)
	w.RunOnce(ctx)

	var after int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND event = 'title_grace_period'`,
		userID,
	).Scan(&after); err != nil {
		t.Fatalf("count notifications after: %v", err)
	}
	if after != before {
		t.Errorf("notification count changed from %d to %d; already-sent row must not be processed again", before, after)
	}
}

// ---------------------------------------------------------------------------
// 5. TestTitleNotification_Idempotent
// ---------------------------------------------------------------------------

// TestTitleNotification_Idempotent verifies that running the worker twice on
// the same pending row results in the flag being TRUE after the first run and
// still TRUE after the second run (idempotent).
func TestTitleNotification_Idempotent(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	titleRepo := title.NewRepository(pool)
	notifRepo := notification.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := titleRepo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	w := newNotificationWorker(titleRepo, notifRepo)

	// First run.
	w.RunOnce(ctx)

	first, err := titleRepo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck after first run: %v", err)
	}
	if !first.UnlockNotificationSent {
		t.Fatal("unlock_notification_sent must be TRUE after first worker run")
	}

	// Second run — must not error, flag must remain TRUE.
	w.RunOnce(ctx)

	second, err := titleRepo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck after second run: %v", err)
	}
	if !second.UnlockNotificationSent {
		t.Error("unlock_notification_sent must still be TRUE after second worker run")
	}
}

// ---------------------------------------------------------------------------
// 6. TestTitleNotification_RevokedNotEligible
// ---------------------------------------------------------------------------

// TestTitleNotification_RevokedNotEligible verifies that a revoked user_title
// with unlock_notification_sent=FALSE is NOT processed by the worker. Revoked
// titles are not eligible for unlock notifications.
func TestTitleNotification_RevokedNotEligible(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	titleRepo := title.NewRepository(pool)
	notifRepo := notification.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := titleRepo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// Force directly to revoked, keeping unlock_notification_sent=FALSE.
	revokedAt := time.Now().UTC()
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='revoked', revoked_at=$1, unlock_notification_sent=FALSE WHERE id=$2`,
		revokedAt, ut.ID,
	); err != nil {
		t.Fatalf("force revoked: %v", err)
	}

	var before int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND event = 'title_unlocked'`,
		userID,
	).Scan(&before); err != nil {
		t.Fatalf("count notifications before: %v", err)
	}

	w := newNotificationWorker(titleRepo, notifRepo)
	w.RunOnce(ctx)

	// Flag must still be FALSE — revoked titles are excluded from the query.
	updated, err := titleRepo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if updated.UnlockNotificationSent {
		t.Error("unlock_notification_sent must remain FALSE for revoked title")
	}

	var after int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND event = 'title_unlocked'`,
		userID,
	).Scan(&after); err != nil {
		t.Fatalf("count notifications after: %v", err)
	}
	if after != before {
		t.Errorf("notification count changed from %d to %d; revoked titles must not trigger unlock notifications", before, after)
	}
}
