//go:build integration

// Title qualification engine integration tests.
//
// Validates Phase 4 qualification rules against real PostgreSQL with all
// migrations applied. Every test exercises the full chain:
//
//	application code → title.Engine → title.Repository → pgx → PostgreSQL
//
// Data is inserted directly via SQL to control timestamps precisely.
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/title"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newQualificationEngine builds a title.Engine backed by a real title.Repository.
func newQualificationEngine(repo *title.Repository) *title.Engine {
	return title.NewEngine(repo, zap.NewNop())
}

// ---------------------------------------------------------------------------
// 1. TestQualification_FoundingMember_Eligible
// ---------------------------------------------------------------------------

// TestQualification_FoundingMember_Eligible verifies that a user created within
// the founding member window (created_at = 2027-04-15) qualifies.
func TestQualification_FoundingMember_Eligible(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := uuid.New()
	createdAt := time.Date(2027, 4, 15, 12, 0, 0, 0, time.UTC)

	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, handle, display_name, email, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		userID,
		"fm_elig_"+userID.String()[:8],
		"FM Eligible User",
		"fm_elig_"+userID.String()[:8]+"@example.com",
		"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		createdAt,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	res, err := eng.Qualify(ctx, userID, "founding_member")
	if err != nil {
		t.Fatalf("Qualify: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for user created 2027-04-15")
	}
	if res.UserTitle == nil {
		t.Error("expected UserTitle non-nil when Qualified=true")
	}
}

// ---------------------------------------------------------------------------
// 2. TestQualification_FoundingMember_Boundary
// ---------------------------------------------------------------------------

// TestQualification_FoundingMember_Boundary verifies the inclusive boundary:
// a user created exactly on 2027-05-20 qualifies.
func TestQualification_FoundingMember_Boundary(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := uuid.New()
	createdAt := time.Date(2027, 5, 20, 0, 0, 0, 0, time.UTC)

	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, handle, display_name, email, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		userID,
		"fm_bnd_"+userID.String()[:8],
		"FM Boundary User",
		"fm_bnd_"+userID.String()[:8]+"@example.com",
		"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		createdAt,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	res, err := eng.Qualify(ctx, userID, "founding_member")
	if err != nil {
		t.Fatalf("Qualify: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for user created exactly on 2027-05-20 (inclusive boundary)")
	}
}

// ---------------------------------------------------------------------------
// 3. TestQualification_FoundingMember_Expired
// ---------------------------------------------------------------------------

// TestQualification_FoundingMember_Expired verifies that a user created
// 2027-05-21 (one day after the deadline) does not qualify.
func TestQualification_FoundingMember_Expired(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := uuid.New()
	createdAt := time.Date(2027, 5, 21, 0, 0, 0, 0, time.UTC)

	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, handle, display_name, email, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		userID,
		"fm_exp_"+userID.String()[:8],
		"FM Expired User",
		"fm_exp_"+userID.String()[:8]+"@example.com",
		"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		createdAt,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	res, err := eng.Qualify(ctx, userID, "founding_member")
	if err != nil {
		t.Fatalf("Qualify: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false for user created 2027-05-21 (one day after deadline)")
	}
}

// ---------------------------------------------------------------------------
// 4. TestQualification_Centurion_Qualified
// ---------------------------------------------------------------------------

// TestQualification_Centurion_Qualified verifies that a user with 100 original
// non-deleted posts qualifies for the Centurion title.
func TestQualification_Centurion_Qualified(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	for i := range 100 {
		postID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			postID, userID, fmt.Sprintf("Post number %d", i),
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
	}

	res, err := eng.Qualify(ctx, userID, "centurion")
	if err != nil {
		t.Fatalf("Qualify centurion: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for 100 original posts")
	}
	if res.UserTitle == nil {
		t.Error("expected UserTitle non-nil when Qualified=true")
	}
}

// ---------------------------------------------------------------------------
// 5. TestQualification_Centurion_NotQualified
// ---------------------------------------------------------------------------

// TestQualification_Centurion_NotQualified verifies that 99 posts do not meet
// the 100-post threshold.
func TestQualification_Centurion_NotQualified(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	for i := range 99 {
		postID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			postID, userID, fmt.Sprintf("Post number %d", i),
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
	}

	res, err := eng.Qualify(ctx, userID, "centurion")
	if err != nil {
		t.Fatalf("Qualify centurion: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false for 99 posts")
	}
}

// ---------------------------------------------------------------------------
// 6. TestQualification_Centurion_RepostsExcluded
// ---------------------------------------------------------------------------

// TestQualification_Centurion_RepostsExcluded_Strict verifies repost exclusion
// with a cross-user seed so the qualifying post count stays at 99.
func TestQualification_Centurion_RepostsExcluded_Strict(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)
	seedAuthorID := newTitleTestUser(ctx, t, pool) // different user owns the seed

	// 99 original posts by the user under test.
	for i := range 99 {
		postID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			postID, userID, fmt.Sprintf("Original post %d", i),
		); err != nil {
			t.Fatalf("insert original post %d: %v", i, err)
		}
	}

	// Seed post owned by the OTHER user.
	seedPostID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
		 VALUES ($1, $2, 'original', 'seed for repost', FALSE, now(), now())`,
		seedPostID, seedAuthorID,
	); err != nil {
		t.Fatalf("insert seed post: %v", err)
	}

	// 5 reposts by the user under test — should NOT count.
	for i := range 5 {
		postID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, quoted_post_id, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'repost', NULL, $3, FALSE, now(), now())`,
			postID, userID, seedPostID,
		); err != nil {
			t.Fatalf("insert repost %d: %v", i, err)
		}
	}

	res, err := eng.Qualify(ctx, userID, "centurion")
	if err != nil {
		t.Fatalf("Qualify centurion: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false: 99 originals + 5 reposts; reposts must not count")
	}
}

// ---------------------------------------------------------------------------
// 7. TestQualification_Trendsetter_Qualified
// ---------------------------------------------------------------------------

// TestQualification_Trendsetter_Qualified verifies that 10,001 likes on the
// user's posts within 30 days qualifies for the Trendsetter title.
func TestQualification_Trendsetter_Qualified(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	// Insert 10,001 unique reactor users and one like each on a single post.
	// For performance: insert the post once, then bulk-insert reactions from
	// unique reactor users. Each reaction requires a unique user_id (PK constraint).
	postID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
		 VALUES ($1, $2, 'original', 'trendsetter test post', FALSE, now(), now())`,
		postID, userID,
	); err != nil {
		t.Fatalf("insert post: %v", err)
	}

	// Insert reactor users in bulk, then their reactions.
	// To avoid inserting 10001 user rows (slow), we insert reactions with
	// random UUIDs as user_id. The reactions.user_id FK references users(id)
	// ON DELETE CASCADE, so we must insert real user rows.
	// Strategy: batch-insert users without sessions, then insert reactions.
	const likeCount = 10_001
	reactorIDs := make([]uuid.UUID, likeCount)
	for i := range likeCount {
		reactorIDs[i] = uuid.New()
	}

	// Batch-insert reactor user rows.
	for i, rid := range reactorIDs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, handle, display_name, email, password_hash)
			 VALUES ($1, $2, $3, $4, $5)`,
			rid,
			fmt.Sprintf("r%d_%s", i, rid.String()[:6]),
			"Reactor",
			fmt.Sprintf("r%d_%s@example.com", i, rid.String()[:6]),
			"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		); err != nil {
			t.Fatalf("insert reactor user %d: %v", i, err)
		}
	}

	// Batch-insert reactions.
	for i, rid := range reactorIDs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
			 VALUES ($1, $2, 'like', now())`,
			rid, postID,
		); err != nil {
			t.Fatalf("insert reaction %d: %v", i, err)
		}
	}

	res, err := eng.Qualify(ctx, userID, "trendsetter")
	if err != nil {
		t.Fatalf("Qualify trendsetter: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for 10001 likes")
	}
}

// ---------------------------------------------------------------------------
// 8. TestQualification_Trendsetter_AtThreshold
// ---------------------------------------------------------------------------

// TestQualification_Trendsetter_AtThreshold verifies that exactly 10,000 likes
// does NOT qualify (strictly greater than required).
func TestQualification_Trendsetter_AtThreshold(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	postID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
		 VALUES ($1, $2, 'original', 'trendsetter threshold post', FALSE, now(), now())`,
		postID, userID,
	); err != nil {
		t.Fatalf("insert post: %v", err)
	}

	const likeCount = 10_000
	for i := range likeCount {
		rid := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, handle, display_name, email, password_hash)
			 VALUES ($1, $2, $3, $4, $5)`,
			rid,
			fmt.Sprintf("t%d_%s", i, rid.String()[:6]),
			"Reactor",
			fmt.Sprintf("t%d_%s@example.com", i, rid.String()[:6]),
			"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		); err != nil {
			t.Fatalf("insert reactor user %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
			 VALUES ($1, $2, 'like', now())`,
			rid, postID,
		); err != nil {
			t.Fatalf("insert reaction %d: %v", i, err)
		}
	}

	res, err := eng.Qualify(ctx, userID, "trendsetter")
	if err != nil {
		t.Fatalf("Qualify trendsetter: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false for exactly 10000 likes (strictly greater required)")
	}
}

// ---------------------------------------------------------------------------
// 9. TestQualification_NicheGuruTech_Qualified
// ---------------------------------------------------------------------------

// TestQualification_NicheGuruTech_Qualified verifies that 16 posts with #tech
// tags and 2 likes each (F1=12.5%) qualifies for Niche Guru: Tech.
func TestQualification_NicheGuruTech_Qualified(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	postIDs := make([]uuid.UUID, 16)
	for i := range 16 {
		postIDs[i] = uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			postIDs[i], userID, fmt.Sprintf("Tech post %d #tech", i),
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO post_hashtags (post_id, tag) VALUES ($1, 'tech')`,
			postIDs[i],
		); err != nil {
			t.Fatalf("insert hashtag %d: %v", i, err)
		}
	}

	// 2 likes per post → 32 total likes → F1 = 32/16 * 100 = 200%
	for _, pid := range postIDs {
		for j := 0; j < 2; j++ {
			rid := uuid.New()
			if _, err := pool.Exec(ctx,
				`INSERT INTO users (id, handle, display_name, email, password_hash)
				 VALUES ($1, $2, $3, $4, $5)`,
				rid,
				fmt.Sprintf("ng%s_%d", rid.String()[:6], j),
				"Reactor",
				fmt.Sprintf("ng%s_%d@example.com", rid.String()[:6], j),
				"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
			); err != nil {
				t.Fatalf("insert reactor: %v", err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
				 VALUES ($1, $2, 'like', now())`,
				rid, pid,
			); err != nil {
				t.Fatalf("insert reaction: %v", err)
			}
		}
	}

	res, err := eng.Qualify(ctx, userID, "niche_guru_tech")
	if err != nil {
		t.Fatalf("Qualify niche_guru_tech: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for 16 tech posts F1=200%")
	}
}

// ---------------------------------------------------------------------------
// 10. TestQualification_NicheGuruTech_MultiTagDedup
// ---------------------------------------------------------------------------

// TestQualification_NicheGuruTech_MultiTagDedup verifies that a post tagged
// with both #tech and #gadgets is counted as exactly 1 post, not 2.
func TestQualification_NicheGuruTech_MultiTagDedup(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	// One post with both #tech and #gadgets tags.
	postID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
		 VALUES ($1, $2, 'original', 'tech and gadgets post', FALSE, now(), now())`,
		postID, userID,
	); err != nil {
		t.Fatalf("insert post: %v", err)
	}
	for _, tag := range []string{"tech", "gadgets"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO post_hashtags (post_id, tag) VALUES ($1, $2)`,
			postID, tag,
		); err != nil {
			t.Fatalf("insert hashtag %q: %v", tag, err)
		}
	}

	// 1 like on the post.
	reactorID := newTitleTestUser(ctx, t, pool)
	if _, err := pool.Exec(ctx,
		`INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
		 VALUES ($1, $2, 'like', now())`,
		reactorID, postID,
	); err != nil {
		t.Fatalf("insert reaction: %v", err)
	}

	// Fetch niche metrics directly to verify dedup.
	posts, likes, err := repo.GetNicheMetrics(ctx, userID, []string{"tech", "gadgets"})
	if err != nil {
		t.Fatalf("GetNicheMetrics: %v", err)
	}
	if posts != 1 {
		t.Errorf("niche_posts_30d = %d, want 1 (multi-tag post must be deduped)", posts)
	}
	if likes != 1 {
		t.Errorf("niche_likes_30d = %d, want 1", likes)
	}
}

// ---------------------------------------------------------------------------
// 11. TestQualification_NicheGuruTech_InsufficientPosts
// ---------------------------------------------------------------------------

// TestQualification_NicheGuruTech_InsufficientPosts verifies that exactly 15
// posts (at the threshold, not strictly greater) does not qualify.
func TestQualification_NicheGuruTech_InsufficientPosts(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	for i := range 15 {
		postID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			postID, userID, fmt.Sprintf("Tech post %d", i),
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO post_hashtags (post_id, tag) VALUES ($1, 'tech')`,
			postID,
		); err != nil {
			t.Fatalf("insert hashtag %d: %v", i, err)
		}
		// Give each post many likes so F1 would be high IF post count qualified.
		for j := 0; j < 100; j++ {
			rid := uuid.New()
			if _, err := pool.Exec(ctx,
				`INSERT INTO users (id, handle, display_name, email, password_hash)
				 VALUES ($1, $2, $3, $4, $5)`,
				rid,
				fmt.Sprintf("ni%s_%d_%d", rid.String()[:5], i, j),
				"Reactor",
				fmt.Sprintf("ni%s_%d_%d@example.com", rid.String()[:5], i, j),
				"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
			); err != nil {
				t.Fatalf("insert reactor: %v", err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
				 VALUES ($1, $2, 'like', now())`,
				rid, postID,
			); err != nil {
				t.Fatalf("insert reaction: %v", err)
			}
		}
	}

	res, err := eng.Qualify(ctx, userID, "niche_guru_tech")
	if err != nil {
		t.Fatalf("Qualify niche_guru_tech: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false for exactly 15 posts (threshold is strictly greater)")
	}
}

// ---------------------------------------------------------------------------
// 12. TestQualification_NicheGuruFitness_Qualified
// ---------------------------------------------------------------------------

// TestQualification_NicheGuruFitness_Qualified verifies that 16 fitness posts
// with high engagement qualify for Niche Guru: Fitness.
func TestQualification_NicheGuruFitness_Qualified(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	for i := range 16 {
		postID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, post_type, content, is_deleted, created_at, updated_at)
			 VALUES ($1, $2, 'original', $3, FALSE, now(), now())`,
			postID, userID, fmt.Sprintf("Fitness post %d", i),
		); err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO post_hashtags (post_id, tag) VALUES ($1, 'fitness')`,
			postID,
		); err != nil {
			t.Fatalf("insert hashtag %d: %v", i, err)
		}
		// 1 like per post → F1 = 1/16 * 100 = 6.25% > 5%
		rid := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, handle, display_name, email, password_hash)
			 VALUES ($1, $2, $3, $4, $5)`,
			rid,
			fmt.Sprintf("fit%s_%d", rid.String()[:6], i),
			"Reactor",
			fmt.Sprintf("fit%s_%d@example.com", rid.String()[:6], i),
			"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		); err != nil {
			t.Fatalf("insert reactor: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
			 VALUES ($1, $2, 'like', now())`,
			rid, postID,
		); err != nil {
			t.Fatalf("insert reaction: %v", err)
		}
	}

	res, err := eng.Qualify(ctx, userID, "niche_guru_fitness")
	if err != nil {
		t.Fatalf("Qualify niche_guru_fitness: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for 16 fitness posts with F1=6.25%")
	}
}

// ---------------------------------------------------------------------------
// 13. TestQualification_Top1Pct_NotQualifiable
// ---------------------------------------------------------------------------

// TestQualification_Top1Pct_NotQualifiable verifies that Qualify returns
// ErrNotQualifiable for the top_1pct_creator slug.
func TestQualification_Top1Pct_NotQualifiable(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	userID := newTitleTestUser(ctx, t, pool)

	_, err := eng.Qualify(ctx, userID, "top_1pct_creator")
	if err == nil {
		t.Fatal("expected ErrNotQualifiable, got nil")
	}
	if !isNotQualifiable(err) {
		t.Errorf("expected ErrNotQualifiable, got %v", err)
	}
}

// isNotQualifiable checks for title.ErrNotQualifiable via errors.Is.
func isNotQualifiable(err error) bool {
	return errors.Is(err, title.ErrNotQualifiable)
}

// ---------------------------------------------------------------------------
// 14. TestQualification_EvaluateAndUnlock_Idempotent
// ---------------------------------------------------------------------------

// TestQualification_EvaluateAndUnlock_Idempotent verifies that calling
// EvaluateAndUnlock twice produces the same results and does not create
// duplicate user_titles rows.
func TestQualification_EvaluateAndUnlock_Idempotent(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)
	eng := newQualificationEngine(repo)

	// User created within founding window.
	userID := uuid.New()
	createdAt := time.Date(2027, 4, 21, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, handle, display_name, email, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		userID,
		"idem_"+userID.String()[:8],
		"Idempotent User",
		"idem_"+userID.String()[:8]+"@example.com",
		"$2a$12$placeholderhashforintegrationtestonly0000000000000000",
		createdAt,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	results1, err := eng.EvaluateAndUnlock(ctx, userID)
	if err != nil {
		t.Fatalf("first EvaluateAndUnlock: %v", err)
	}

	results2, err := eng.EvaluateAndUnlock(ctx, userID)
	if err != nil {
		t.Fatalf("second EvaluateAndUnlock: %v", err)
	}

	// Both runs must return the same number of results.
	if len(results1) != len(results2) {
		t.Errorf("result count mismatch: first=%d second=%d", len(results1), len(results2))
	}

	// Build slug→qualified maps for both runs.
	qualified1 := make(map[string]bool, len(results1))
	for _, r := range results1 {
		qualified1[r.Slug] = r.Qualified
	}
	qualified2 := make(map[string]bool, len(results2))
	for _, r := range results2 {
		qualified2[r.Slug] = r.Qualified
	}

	for slug, q1 := range qualified1 {
		if q2, ok := qualified2[slug]; !ok || q1 != q2 {
			t.Errorf("slug %q: first Qualified=%v, second Qualified=%v", slug, q1, q2)
		}
	}

	// Verify no duplicate active user_titles rows exist.
	var dupCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
		    SELECT title_definition_id
		    FROM user_titles
		    WHERE user_id = $1
		      AND status IN ('active', 'grace_period')
		    GROUP BY title_definition_id
		    HAVING COUNT(*) > 1
		) sub`,
		userID,
	).Scan(&dupCount); err != nil {
		t.Fatalf("check duplicate rows: %v", err)
	}
	if dupCount > 0 {
		t.Errorf("found %d title_definition_id(s) with duplicate active rows after idempotent run", dupCount)
	}
}
