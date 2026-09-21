//go:build integration

// Title qualification worker integration tests.
//
// Validates Phase 5 worker lifecycle reconciliation against real PostgreSQL
// with all 0001–0017 migrations applied. Every test exercises the full chain:
//
//	TitleQualificationWorker → title.Engine → title.Repository → pgx → PostgreSQL
//
// Direct SQL manipulation is used to set up preconditions (e.g. force a title
// into grace_period) without going through the full worker run cycle.
//
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/title"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newWorker builds a real TitleQualificationWorker backed by the test database.
func newWorker(repo *title.Repository) *title.TitleQualificationWorker {
	eng := title.NewEngine(repo, zap.NewNop())
	cfg := title.WorkerConfig{Interval: time.Minute} // ticker not used in RunOnce
	return title.NewTitleQualificationWorker(eng, repo, cfg, zap.NewNop())
}

// ---------------------------------------------------------------------------
// 1. TestWorker_ActiveToGrace
// ---------------------------------------------------------------------------

// TestWorker_ActiveToGrace verifies that a revocable active title for a user
// who no longer qualifies transitions to grace_period after worker reconciliation.
func TestWorker_ActiveToGrace(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	// Create a user with no posts (so trendsetter is unqualified).
	userID := newTitleTestUser(ctx, t, pool)

	// Directly insert an active trendsetter title to simulate a previously earned state.
	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle trendsetter: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// Verify the title transitioned to grace_period.
	entry, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if entry.Status != title.StatusGracePeriod {
		t.Errorf("status = %v, want grace_period", entry.Status)
	}
	if entry.GracePeriodEndsAt == nil {
		t.Fatal("grace_period_ends_at must be set")
	}
	expectedMin := time.Now().UTC().Add(47 * time.Hour)
	expectedMax := time.Now().UTC().Add(49 * time.Hour)
	if entry.GracePeriodEndsAt.Before(expectedMin) || entry.GracePeriodEndsAt.After(expectedMax) {
		t.Errorf("grace_period_ends_at %v not in expected ~48h range [%v, %v]",
			entry.GracePeriodEndsAt, expectedMin, expectedMax)
	}
}

// ---------------------------------------------------------------------------
// 2. TestWorker_GraceToActive
// ---------------------------------------------------------------------------

// TestWorker_GraceToActive verifies that a grace_period title is restored to
// active when the user re-qualifies during the grace window.
//
// Uses niche_guru_tech (is_revocable=true) so the worker lifecycle branch for
// grace_period → active is exercised. Centurion is a permanent (is_revocable=false)
// title; the worker intentionally skips lifecycle reconciliation for permanent
// titles. 16 posts with #tech hashtag and 1 like each satisfy the niche_guru_tech
// qualification threshold cheaply.
func TestWorker_GraceToActive(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	// Create a user who qualifies for niche_guru_tech:
	// >15 posts tagged #tech, each with >5% engagement (1 like per post = 6.25%).
	userID := newTitleTestUser(ctx, t, pool)
	for i := 0; i < 16; i++ {
		postID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			postID, userID, fmt.Sprintf("Tech post %d #tech", i),
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO post_hashtags (post_id, tag) VALUES ($1, 'tech')`,
			postID,
		); err != nil {
			t.Fatalf("insert hashtag %d: %v", i, err)
		}
		// 1 like per post from a unique reactor.
		reactorID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, handle, display_name, email, password_hash)
			 VALUES ($1, $2, 'Reactor', $3, $4)`,
			reactorID,
			fmt.Sprintf("gta_%s_%d", reactorID.String()[:6], i),
			fmt.Sprintf("gta_%s_%d@example.com", reactorID.String()[:6], i),
			"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		); err != nil {
			t.Fatalf("insert reactor %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
			 VALUES ($1, $2, 'like', now())`,
			reactorID, postID,
		); err != nil {
			t.Fatalf("insert reaction %d: %v", i, err)
		}
	}

	// Create an active niche_guru_tech title, then force it into grace_period via SQL.
	ut, err := repo.CreateUserTitle(ctx, userID, defNicheGuruTech)
	if err != nil {
		t.Fatalf("CreateUserTitle niche_guru_tech: %v", err)
	}

	endsAt := time.Now().UTC().Add(24 * time.Hour) // still within window
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1 WHERE id=$2`,
		endsAt, ut.ID,
	); err != nil {
		t.Fatalf("force grace_period: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// Should be restored to active since user qualifies for niche_guru_tech.
	entry, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if entry.Status != title.StatusActive {
		t.Errorf("status = %v, want active after re-qualification during grace", entry.Status)
	}
	if entry.GracePeriodEndsAt != nil {
		t.Errorf("grace_period_ends_at should be NULL after restore, got %v", entry.GracePeriodEndsAt)
	}
}

// ---------------------------------------------------------------------------
// 3. TestWorker_GraceToRevoked
// ---------------------------------------------------------------------------

// TestWorker_GraceToRevoked verifies that a grace_period title with an expired
// grace window is revoked when the user does not re-qualify.
func TestWorker_GraceToRevoked(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	// User has no qualifying activity for trendsetter.
	userID := newTitleTestUser(ctx, t, pool)

	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle trendsetter: %v", err)
	}

	// Force into grace_period with an expired end time.
	expiredAt := time.Now().UTC().Add(-1 * time.Second)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1 WHERE id=$2`,
		expiredAt, ut.ID,
	); err != nil {
		t.Fatalf("force expired grace_period: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// Should be revoked.
	entry, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if entry.Status != title.StatusRevoked {
		t.Errorf("status = %v, want revoked after expired grace period", entry.Status)
	}
	if entry.RevokedAt == nil {
		t.Error("revoked_at must be set after revocation")
	}
}

// ---------------------------------------------------------------------------
// 4. TestWorker_RevokedReearned
// ---------------------------------------------------------------------------

// TestWorker_RevokedReearned verifies that a user who previously had a revoked
// title and now re-qualifies gets a new active user_titles row (different row).
func TestWorker_RevokedReearned(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	// Create user with 100 posts so centurion qualifies.
	userID := newTitleTestUser(ctx, t, pool)
	for i := 0; i < 100; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			uuid.New(), userID, "Post content",
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
	}

	// Create centurion title and directly revoke it (simulating a prior revocation).
	ut, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("CreateUserTitle centurion: %v", err)
	}
	revokedAt := time.Now().UTC().Add(-time.Hour)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='revoked', revoked_at=$1 WHERE id=$2`,
		revokedAt, ut.ID,
	); err != nil {
		t.Fatalf("force revoked: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// Verify a new active row was created (different ID from the revoked row).
	entries, err := repo.GetUserTitles(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserTitles: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.TitleDefinitionID == defCenturion && e.Status == title.StatusActive && e.ID != ut.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected a new active centurion row after re-earning; none found")
	}
}

// ---------------------------------------------------------------------------
// 5. TestWorker_PrimaryTitleClearedOnRevoke
// ---------------------------------------------------------------------------

// TestWorker_PrimaryTitleClearedOnRevoke verifies that when a grace_period title
// that is the user's primary is finally revoked, users.primary_title_id is set to NULL.
func TestWorker_PrimaryTitleClearedOnRevoke(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	// Create trendsetter title and set it as primary.
	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle trendsetter: %v", err)
	}
	if err := repo.SetPrimaryTitle(ctx, userID, ut.ID); err != nil {
		t.Fatalf("SetPrimaryTitle: %v", err)
	}

	// Force into grace_period with expired end time (user does not qualify).
	expiredAt := time.Now().UTC().Add(-1 * time.Second)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1 WHERE id=$2`,
		expiredAt, ut.ID,
	); err != nil {
		t.Fatalf("force expired grace_period: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// primary_title_id must be cleared.
	var primaryID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT primary_title_id FROM users WHERE id = $1`, userID,
	).Scan(&primaryID); err != nil {
		t.Fatalf("query primary_title_id: %v", err)
	}
	if primaryID != nil {
		t.Errorf("primary_title_id = %v, want NULL after revocation of primary title", *primaryID)
	}
}

// ---------------------------------------------------------------------------
// 6. TestWorker_PrimaryTitleNotClearedForDifferent
// ---------------------------------------------------------------------------

// TestWorker_PrimaryTitleNotClearedForDifferent verifies that revoking title B
// does not clear primary_title_id when it points to title A (a different row).
func TestWorker_PrimaryTitleNotClearedForDifferent(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	// User with 100 posts qualifies for centurion.
	userID := newTitleTestUser(ctx, t, pool)
	for i := 0; i < 100; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			uuid.New(), userID, "Post content",
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
	}

	// Title A: centurion (permanent, stays active — is_revocable=false).
	utA, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("CreateUserTitle centurion: %v", err)
	}

	// Title B: trendsetter (revocable, will be revoked).
	utB, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle trendsetter: %v", err)
	}

	// Set primary to title A (centurion).
	if err := repo.SetPrimaryTitle(ctx, userID, utA.ID); err != nil {
		t.Fatalf("SetPrimaryTitle to centurion: %v", err)
	}

	// Force title B (trendsetter) into expired grace_period.
	expiredAt := time.Now().UTC().Add(-1 * time.Second)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1 WHERE id=$2`,
		expiredAt, utB.ID,
	); err != nil {
		t.Fatalf("force trendsetter into expired grace_period: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// primary_title_id must still point to title A.
	var primaryID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT primary_title_id FROM users WHERE id = $1`, userID,
	).Scan(&primaryID); err != nil {
		t.Fatalf("query primary_title_id: %v", err)
	}
	if primaryID == nil || *primaryID != utA.ID {
		t.Errorf("primary_title_id = %v, want %v (must not be cleared for a different title)",
			primaryID, utA.ID)
	}
}

// ---------------------------------------------------------------------------
// 7. TestWorker_PermanentTitleNotRevoked
// ---------------------------------------------------------------------------

// TestWorker_PermanentTitleNotRevoked verifies that an active Founding Member
// title (is_revocable=false) is never revoked by the worker, even if the user
// no longer qualifies (joined after the founding window).
func TestWorker_PermanentTitleNotRevoked(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	// Insert user with created_at far outside the founding member window.
	userID := uuid.New()
	lateCreatedAt := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, handle, display_name, email, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		userID,
		"perm_"+userID.String()[:8],
		"Permanent Test User",
		"perm_"+userID.String()[:8]+"@example.com",
		"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		lateCreatedAt,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Directly insert an active founding_member title (bypassing qualification).
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle founding_member: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// Title must remain active — permanent titles are never revoked.
	entry, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if entry.Status != title.StatusActive {
		t.Errorf("permanent title status = %v, want active (must never be revoked by worker)", entry.Status)
	}
}

// ---------------------------------------------------------------------------
// 8. TestWorker_GraceBefore48h
// ---------------------------------------------------------------------------

// TestWorker_GraceBefore48h verifies that a grace_period title with
// grace_period_ends_at = now + 1 hour remains in grace_period when unqualified.
func TestWorker_GraceBefore48h(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle trendsetter: %v", err)
	}

	// Set grace_period_ends_at to 1 hour from now (well within window).
	endsAt := time.Now().UTC().Add(1 * time.Hour)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1 WHERE id=$2`,
		endsAt, ut.ID,
	); err != nil {
		t.Fatalf("force grace_period: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	// Must still be grace_period — window has not expired.
	entry, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if entry.Status != title.StatusGracePeriod {
		t.Errorf("status = %v, want grace_period (window not yet expired)", entry.Status)
	}
}

// ---------------------------------------------------------------------------
// 9. TestWorker_GraceExactlyAt48h
// ---------------------------------------------------------------------------

// TestWorker_GraceExactlyAt48h verifies that a grace_period title with
// grace_period_ends_at just expired (now - 1 second) is revoked.
func TestWorker_GraceExactlyAt48h(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle trendsetter: %v", err)
	}

	// Set grace_period_ends_at to 1 second in the past (just expired).
	justExpired := time.Now().UTC().Add(-1 * time.Second)
	if _, err := pool.Exec(ctx,
		`UPDATE user_titles SET status='grace_period', grace_period_ends_at=$1 WHERE id=$2`,
		justExpired, ut.ID,
	); err != nil {
		t.Fatalf("force just-expired grace_period: %v", err)
	}

	w := newWorker(repo)
	w.RunOnce(ctx)

	entry, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if entry.Status != title.StatusRevoked {
		t.Errorf("status = %v, want revoked (grace window just expired)", entry.Status)
	}
	if entry.RevokedAt == nil {
		t.Error("revoked_at must be set")
	}
}
