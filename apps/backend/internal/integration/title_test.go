//go:build integration

// Title repository integration tests.
//
// Validates the Phase 2 title repository against real PostgreSQL with all
// 0001–0017 migrations applied. Every test exercises the full chain:
//
//	application code → title.Repository → pgx → PostgreSQL → real schema
//
// No database mocking is used.
// Run with: go test -tags integration ./internal/integration/
package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/title"
)

// ---------------------------------------------------------------------------
// Well-known seed UUIDs — migration 0014
// ---------------------------------------------------------------------------

var (
	defFoundingMember   = uuid.MustParse("00000000-0000-7000-8000-000000000001")
	defCenturion        = uuid.MustParse("00000000-0000-7000-8000-000000000002")
	defTrendsetter      = uuid.MustParse("00000000-0000-7000-8000-000000000003")
	defNicheGuruTech    = uuid.MustParse("00000000-0000-7000-8000-000000000004")
	defNicheGuruFitness = uuid.MustParse("00000000-0000-7000-8000-000000000005")
	defTop1Pct          = uuid.MustParse("00000000-0000-7000-8000-000000000006")
)

// ---------------------------------------------------------------------------
// Helper: newTitleTestUser
// ---------------------------------------------------------------------------

// newTitleTestUser creates a unique test user via auth.CreateUser and returns
// the user's UUID. Follows the existing integration-test convention: unique
// data derived from uuid.New() so tests can run concurrently against the same
// shared database without cleanup.
func newTitleTestUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := auth.CreateUser(ctx, pool, &auth.UserCreateInput{
		ID:           id,
		Handle:       "ttl_" + id.String()[:8],
		DisplayName:  "Title Test User",
		Email:        "ttl_" + id.String()[:8] + "@example.com",
		PasswordHash: "$2a$12$placeholderhashforintegrationtestonly0000000000000000",
	})
	if err != nil {
		t.Fatalf("newTitleTestUser: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------------
// 1. GetDefinitions — seeded data verification
// ---------------------------------------------------------------------------

// TestTitleRepo_GetDefinitions_SeededData verifies that the five active title
// definitions seeded in migration 0014 are returned with correct field values,
// and that Top 1% Creator (is_active=FALSE) is excluded.
func TestTitleRepo_GetDefinitions_SeededData(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	defs, err := repo.GetDefinitions(ctx)
	if err != nil {
		t.Fatalf("GetDefinitions: %v", err)
	}

	// Index by slug for deterministic lookup (ORDER BY created_at may tie because
	// all seed rows share the same transaction timestamp).
	bySlug := make(map[string]title.TitleDefinition, len(defs))
	for _, d := range defs {
		bySlug[d.Slug] = d
	}

	// Top 1% Creator must be absent (is_active=FALSE).
	if _, ok := bySlug["top_1pct_creator"]; ok {
		t.Error("GetDefinitions must not return top_1pct_creator (is_active=FALSE)")
	}

	type wantDef struct {
		id          uuid.UUID
		displayName string
		category    title.TitleCategory
		isRevocable bool
	}
	wants := map[string]wantDef{
		"founding_member":    {defFoundingMember, "Founding Member", title.CategoryMilestone, false},
		"centurion":          {defCenturion, "Centurion", title.CategoryMilestone, false},
		"trendsetter":        {defTrendsetter, "Trendsetter", title.CategoryPerformance, true},
		"niche_guru_tech":    {defNicheGuruTech, "Niche Guru: Tech", title.CategoryNiche, true},
		"niche_guru_fitness": {defNicheGuruFitness, "Niche Guru: Fitness", title.CategoryNiche, true},
	}

	for slug, want := range wants {
		d, ok := bySlug[slug]
		if !ok {
			t.Errorf("expected definition %q not found in GetDefinitions", slug)
			continue
		}
		if d.ID != want.id {
			t.Errorf("%s: ID = %v, want %v", slug, d.ID, want.id)
		}
		if d.DisplayName != want.displayName {
			t.Errorf("%s: DisplayName = %q, want %q", slug, d.DisplayName, want.displayName)
		}
		if d.Category != want.category {
			t.Errorf("%s: Category = %v, want %v", slug, d.Category, want.category)
		}
		if d.IsRevocable != want.isRevocable {
			t.Errorf("%s: IsRevocable = %v, want %v", slug, d.IsRevocable, want.isRevocable)
		}
		if !d.IsActive {
			t.Errorf("%s: IsActive should be TRUE", slug)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. GetDefinitionBySlug
// ---------------------------------------------------------------------------

// TestTitleRepo_GetDefinitionBySlug_Found verifies that an existing slug
// returns the correct definition. Uses top_1pct_creator (is_active=FALSE) to
// confirm GetDefinitionBySlug has no is_active filter.
func TestTitleRepo_GetDefinitionBySlug_Found(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	d, err := repo.GetDefinitionBySlug(ctx, "top_1pct_creator")
	if err != nil {
		t.Fatalf("GetDefinitionBySlug: %v", err)
	}
	if d.ID != defTop1Pct {
		t.Errorf("ID = %v, want %v", d.ID, defTop1Pct)
	}
	if d.IsActive {
		t.Error("top_1pct_creator IsActive must be FALSE")
	}
	if d.Category != title.CategoryPerformance {
		t.Errorf("Category = %v, want performance", d.Category)
	}
	if !d.IsRevocable {
		t.Error("top_1pct_creator IsRevocable should be TRUE")
	}
	if d.Slug != "top_1pct_creator" {
		t.Errorf("Slug = %q, want top_1pct_creator", d.Slug)
	}
	if d.DisplayName != "Top 1% Creator" {
		t.Errorf("DisplayName = %q, want Top 1%% Creator", d.DisplayName)
	}
}

// TestTitleRepo_GetDefinitionBySlug_NotFound verifies that a nonexistent slug
// returns ErrNotFound (errors.Is, not string comparison).
func TestTitleRepo_GetDefinitionBySlug_NotFound(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	_, err := repo.GetDefinitionBySlug(ctx, "nonexistent_"+uuid.New().String()[:8])
	if !errors.Is(err, title.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3. GetUserTitles
// ---------------------------------------------------------------------------

// TestTitleRepo_GetUserTitles_ActiveAndGraceReturned_RevokedExcluded verifies
// that active and grace_period titles are returned, revoked titles are excluded,
// and joined definition fields are correctly scanned.
func TestTitleRepo_GetUserTitles_ActiveAndGraceReturned_RevokedExcluded(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	// Create active title (founding_member).
	active, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle active: %v", err)
	}

	// Create centurion and transition to grace_period.
	grace, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("CreateUserTitle grace: %v", err)
	}
	graceEnds := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Second)
	if err := repo.UpdateUserTitleStatus(ctx, grace.ID, title.TitleStatusUpdate{
		Status:            title.StatusGracePeriod,
		GracePeriodEndsAt: &graceEnds,
	}); err != nil {
		t.Fatalf("UpdateUserTitleStatus → grace_period: %v", err)
	}

	// Create trendsetter and revoke it.
	revoked, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle revoked: %v", err)
	}
	revokedAt := time.Now().UTC()
	if err := repo.UpdateUserTitleStatus(ctx, revoked.ID, title.TitleStatusUpdate{
		Status:    title.StatusRevoked,
		RevokedAt: &revokedAt,
	}); err != nil {
		t.Fatalf("UpdateUserTitleStatus → revoked: %v", err)
	}

	entries, err := repo.GetUserTitles(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserTitles: %v", err)
	}

	// Expect exactly 2 (active + grace_period); revoked is excluded.
	if len(entries) != 2 {
		t.Errorf("GetUserTitles returned %d entries, want 2", len(entries))
	}

	byID := make(map[uuid.UUID]title.UserTitleEntry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	// Revoked must be absent.
	if _, ok := byID[revoked.ID]; ok {
		t.Error("revoked title must not appear in GetUserTitles")
	}

	// Active title — joined fields.
	ae, ok := byID[active.ID]
	if !ok {
		t.Fatal("active title not found in GetUserTitles")
	}
	if ae.Status != title.StatusActive {
		t.Errorf("active Status = %v, want active", ae.Status)
	}
	if ae.Slug != "founding_member" {
		t.Errorf("active Slug = %q, want founding_member", ae.Slug)
	}
	if ae.DisplayName != "Founding Member" {
		t.Errorf("active DisplayName = %q, want Founding Member", ae.DisplayName)
	}
	if ae.Category != title.CategoryMilestone {
		t.Errorf("active Category = %v, want milestone", ae.Category)
	}
	if ae.IsRevocable {
		t.Error("founding_member IsRevocable should be FALSE")
	}
	if ae.UserID != userID {
		t.Errorf("active UserID = %v, want %v", ae.UserID, userID)
	}

	// Grace_period title present.
	ge, ok := byID[grace.ID]
	if !ok {
		t.Fatal("grace_period title not found in GetUserTitles")
	}
	if ge.Status != title.StatusGracePeriod {
		t.Errorf("grace Status = %v, want grace_period", ge.Status)
	}
	if ge.Slug != "centurion" {
		t.Errorf("grace Slug = %q, want centurion", ge.Slug)
	}
}

// TestTitleRepo_GetUserTitles_OtherUserIsolated verifies that GetUserTitles
// returns only the requesting user's titles, not another user's.
func TestTitleRepo_GetUserTitles_OtherUserIsolated(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userA := newTitleTestUser(ctx, t, pool)
	userB := newTitleTestUser(ctx, t, pool)

	if _, err := repo.CreateUserTitle(ctx, userA, defFoundingMember); err != nil {
		t.Fatalf("CreateUserTitle userA: %v", err)
	}

	entries, err := repo.GetUserTitles(ctx, userB)
	if err != nil {
		t.Fatalf("GetUserTitles userB: %v", err)
	}
	for _, e := range entries {
		if e.UserID == userA {
			t.Error("GetUserTitles returned a title belonging to a different user")
		}
	}
}

// ---------------------------------------------------------------------------
// 4. GetPrimaryTitle
// ---------------------------------------------------------------------------

// TestTitleRepo_GetPrimaryTitle_NilWhenNotSet verifies that a user with no
// primary_title_id set returns nil, nil.
func TestTitleRepo_GetPrimaryTitle_NilWhenNotSet(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	s, err := repo.GetPrimaryTitle(ctx, userID)
	if err != nil {
		t.Fatalf("GetPrimaryTitle: %v", err)
	}
	if s != nil {
		t.Errorf("expected nil for user with no primary title, got %+v", s)
	}
}

// TestTitleRepo_GetPrimaryTitle_ReturnsCorrectSummary verifies that after
// SetPrimaryTitle, GetPrimaryTitle returns the correct TitleSummary with Slug,
// DisplayName, and ID joined from the definition.
func TestTitleRepo_GetPrimaryTitle_ReturnsCorrectSummary(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	if err := repo.SetPrimaryTitle(ctx, userID, ut.ID); err != nil {
		t.Fatalf("SetPrimaryTitle: %v", err)
	}

	s, err := repo.GetPrimaryTitle(ctx, userID)
	if err != nil {
		t.Fatalf("GetPrimaryTitle: %v", err)
	}
	if s == nil {
		t.Fatal("GetPrimaryTitle returned nil after SetPrimaryTitle")
	}
	if s.ID != ut.ID {
		t.Errorf("TitleSummary.ID = %v, want %v", s.ID, ut.ID)
	}
	if s.Slug != "centurion" {
		t.Errorf("TitleSummary.Slug = %q, want centurion", s.Slug)
	}
	if s.DisplayName != "Centurion" {
		t.Errorf("TitleSummary.DisplayName = %q, want Centurion", s.DisplayName)
	}
}

// ---------------------------------------------------------------------------
// 5. CreateUserTitle
// ---------------------------------------------------------------------------

// TestTitleRepo_CreateUserTitle_RowPersisted verifies that a new user_titles
// row is inserted with the correct field values and is retrievable via the
// ownership-check method.
func TestTitleRepo_CreateUserTitle_RowPersisted(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	if ut == nil {
		t.Fatal("CreateUserTitle returned nil")
	}
	if ut.UserID != userID {
		t.Errorf("UserID = %v, want %v", ut.UserID, userID)
	}
	if ut.TitleDefinitionID != defFoundingMember {
		t.Errorf("TitleDefinitionID = %v, want %v", ut.TitleDefinitionID, defFoundingMember)
	}
	if ut.Status != title.StatusActive {
		t.Errorf("Status = %v, want active", ut.Status)
	}
	if ut.GracePeriodEndsAt != nil {
		t.Errorf("GracePeriodEndsAt should be nil on creation, got %v", ut.GracePeriodEndsAt)
	}
	if ut.RevokedAt != nil {
		t.Errorf("RevokedAt should be nil on creation, got %v", ut.RevokedAt)
	}
	if ut.GraceNotificationSent {
		t.Error("GraceNotificationSent should be FALSE on creation")
	}
	if ut.UnlockNotificationSent {
		t.Error("UnlockNotificationSent should be FALSE on creation")
	}
}

// TestTitleRepo_CreateUserTitle_UUIDv7 verifies that the generated ID is a
// UUID v7 (byte[6] high nibble == 7), per Dzeroth ADR 0004.
func TestTitleRepo_CreateUserTitle_UUIDv7(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// UUID v7: version nibble is byte[6] >> 4.
	if ut.ID[6]>>4 != 7 {
		t.Errorf("CreateUserTitle ID %v is not UUID v7 (byte[6]>>4=%d, want 7)", ut.ID, ut.ID[6]>>4)
	}
}

// TestTitleRepo_CreateUserTitle_Idempotent verifies ON CONFLICT DO NOTHING:
// calling CreateUserTitle for the same (user, definition) when an active row
// already exists returns the existing row and does not create a duplicate.
func TestTitleRepo_CreateUserTitle_Idempotent(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	first, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("first CreateUserTitle: %v", err)
	}

	second, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("second CreateUserTitle: %v", err)
	}
	if second == nil {
		t.Fatal("second CreateUserTitle returned nil")
	}
	if second.ID != first.ID {
		t.Errorf("idempotent call returned different ID: first=%v second=%v", first.ID, second.ID)
	}
}

// ---------------------------------------------------------------------------
// 6. UpdateUserTitleStatus
// ---------------------------------------------------------------------------

// TestTitleRepo_UpdateUserTitleStatus_ActiveToGracePeriod verifies the
// active → grace_period transition sets status and grace_period_ends_at.
func TestTitleRepo_UpdateUserTitleStatus_ActiveToGracePeriod(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	endsAt := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Second)
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:            title.StatusGracePeriod,
		GracePeriodEndsAt: &endsAt,
	}); err != nil {
		t.Fatalf("UpdateUserTitleStatus → grace_period: %v", err)
	}

	updated, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if updated.Status != title.StatusGracePeriod {
		t.Errorf("Status = %v, want grace_period", updated.Status)
	}
	if updated.GracePeriodEndsAt == nil {
		t.Fatal("GracePeriodEndsAt must be set after transition")
	}
	if !updated.GracePeriodEndsAt.UTC().Truncate(time.Second).Equal(endsAt) {
		t.Errorf("GracePeriodEndsAt = %v, want %v", updated.GracePeriodEndsAt.UTC(), endsAt)
	}
}

// TestTitleRepo_UpdateUserTitleStatus_GracePeriodToRevoked verifies the
// grace_period → revoked transition sets status and revoked_at.
func TestTitleRepo_UpdateUserTitleStatus_GracePeriodToRevoked(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	endsAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:            title.StatusGracePeriod,
		GracePeriodEndsAt: &endsAt,
	}); err != nil {
		t.Fatalf("transition to grace_period: %v", err)
	}

	revokedAt := time.Now().UTC()
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:    title.StatusRevoked,
		RevokedAt: &revokedAt,
	}); err != nil {
		t.Fatalf("transition to revoked: %v", err)
	}

	updated, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if updated.Status != title.StatusRevoked {
		t.Errorf("Status = %v, want revoked", updated.Status)
	}
	if updated.RevokedAt == nil {
		t.Fatal("RevokedAt must be set after revocation")
	}
}

// TestTitleRepo_UpdateUserTitleStatus_ActiveToRevoked verifies the direct
// active → revoked transition (no grace_period intermediate step required).
func TestTitleRepo_UpdateUserTitleStatus_ActiveToRevoked(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defNicheGuruTech)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	revokedAt := time.Now().UTC()
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:    title.StatusRevoked,
		RevokedAt: &revokedAt,
	}); err != nil {
		t.Fatalf("transition to revoked: %v", err)
	}

	updated, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if updated.Status != title.StatusRevoked {
		t.Errorf("Status = %v, want revoked", updated.Status)
	}
}

// TestTitleRepo_UpdateUserTitleStatus_UnlockNotificationSent verifies that
// setting UnlockNotificationSent=true persists correctly.
func TestTitleRepo_UpdateUserTitleStatus_UnlockNotificationSent(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	tr := true
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:                 title.StatusActive,
		UnlockNotificationSent: &tr,
	}); err != nil {
		t.Fatalf("UpdateUserTitleStatus unlock_notification_sent: %v", err)
	}

	updated, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if !updated.UnlockNotificationSent {
		t.Error("UnlockNotificationSent should be TRUE after update")
	}
}

// TestTitleRepo_UpdateUserTitleStatus_GraceNotificationSent verifies that
// setting GraceNotificationSent=true persists correctly.
func TestTitleRepo_UpdateUserTitleStatus_GraceNotificationSent(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	endsAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:            title.StatusGracePeriod,
		GracePeriodEndsAt: &endsAt,
	}); err != nil {
		t.Fatalf("transition to grace_period: %v", err)
	}

	tr := true
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:                title.StatusGracePeriod,
		GraceNotificationSent: &tr,
	}); err != nil {
		t.Fatalf("UpdateUserTitleStatus grace_notification_sent: %v", err)
	}

	updated, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if !updated.GraceNotificationSent {
		t.Error("GraceNotificationSent should be TRUE after update")
	}
}

// TestTitleRepo_UpdateUserTitleStatus_NilFieldsPreserveExisting verifies the
// COALESCE nil-preservation contract: passing nil GracePeriodEndsAt must not
// overwrite the existing column value.
func TestTitleRepo_UpdateUserTitleStatus_NilFieldsPreserveExisting(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// Transition to grace_period with a specific end timestamp.
	originalEndsAt := time.Now().UTC().Add(14 * 24 * time.Hour).Truncate(time.Second)
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:            title.StatusGracePeriod,
		GracePeriodEndsAt: &originalEndsAt,
	}); err != nil {
		t.Fatalf("set grace_period: %v", err)
	}

	// Call again with nil GracePeriodEndsAt — COALESCE must preserve the existing value.
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:            title.StatusGracePeriod,
		GracePeriodEndsAt: nil,
	}); err != nil {
		t.Fatalf("nil GracePeriodEndsAt update: %v", err)
	}

	updated, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if updated.GracePeriodEndsAt == nil {
		t.Fatal("GracePeriodEndsAt was cleared despite nil COALESCE — expected preservation")
	}
	if !updated.GracePeriodEndsAt.UTC().Truncate(time.Second).Equal(originalEndsAt) {
		t.Errorf("GracePeriodEndsAt = %v, want %v (nil COALESCE must preserve existing value)",
			updated.GracePeriodEndsAt.UTC(), originalEndsAt)
	}
}

// TestTitleRepo_UpdateUserTitleStatus_NotFound verifies ErrNotFound for a
// nonexistent user_titles ID.
func TestTitleRepo_UpdateUserTitleStatus_NotFound(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	err := repo.UpdateUserTitleStatus(ctx, uuid.New(), title.TitleStatusUpdate{
		Status: title.StatusActive,
	})
	if !errors.Is(err, title.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 7. SetPrimaryTitle
// ---------------------------------------------------------------------------

// TestTitleRepo_SetPrimaryTitle_ActiveTitle_Succeeds verifies that an active
// owned title can be set as primary and the users table column is updated.
func TestTitleRepo_SetPrimaryTitle_ActiveTitle_Succeeds(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	if err := repo.SetPrimaryTitle(ctx, userID, ut.ID); err != nil {
		t.Fatalf("SetPrimaryTitle: %v", err)
	}

	// Verify via GetPrimaryTitle.
	s, err := repo.GetPrimaryTitle(ctx, userID)
	if err != nil {
		t.Fatalf("GetPrimaryTitle: %v", err)
	}
	if s == nil {
		t.Fatal("GetPrimaryTitle returned nil after SetPrimaryTitle")
	}
	if s.ID != ut.ID {
		t.Errorf("primary title ID = %v, want %v", s.ID, ut.ID)
	}

	// Verify database column directly.
	var colVal *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT primary_title_id FROM users WHERE id = $1`, userID,
	).Scan(&colVal); err != nil {
		t.Fatalf("query users.primary_title_id: %v", err)
	}
	if colVal == nil || *colVal != ut.ID {
		t.Errorf("users.primary_title_id = %v, want %v", colVal, ut.ID)
	}
}

// TestTitleRepo_SetPrimaryTitle_GracePeriodTitle_Succeeds verifies that a
// grace_period title can be set as primary (both active and grace_period are
// eligible per the Phase 2 contract).
func TestTitleRepo_SetPrimaryTitle_GracePeriodTitle_Succeeds(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defCenturion)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	endsAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:            title.StatusGracePeriod,
		GracePeriodEndsAt: &endsAt,
	}); err != nil {
		t.Fatalf("transition to grace_period: %v", err)
	}

	if err := repo.SetPrimaryTitle(ctx, userID, ut.ID); err != nil {
		t.Fatalf("SetPrimaryTitle grace_period: %v", err)
	}

	s, err := repo.GetPrimaryTitle(ctx, userID)
	if err != nil {
		t.Fatalf("GetPrimaryTitle: %v", err)
	}
	if s == nil || s.ID != ut.ID {
		t.Errorf("expected primary %v, got %v", ut.ID, s)
	}
}

// TestTitleRepo_SetPrimaryTitle_OtherUserTitle_ReturnsForbidden verifies that
// a user cannot set another user's title as their primary.
func TestTitleRepo_SetPrimaryTitle_OtherUserTitle_ReturnsForbidden(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userA := newTitleTestUser(ctx, t, pool)
	userB := newTitleTestUser(ctx, t, pool)

	utA, err := repo.CreateUserTitle(ctx, userA, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle userA: %v", err)
	}

	err = repo.SetPrimaryTitle(ctx, userB, utA.ID)
	if !errors.Is(err, title.ErrForbidden) {
		t.Errorf("expected ErrForbidden for cross-user SetPrimaryTitle, got %v", err)
	}
}

// TestTitleRepo_SetPrimaryTitle_RevokedTitle_ReturnsForbidden verifies that a
// revoked title cannot become primary.
func TestTitleRepo_SetPrimaryTitle_RevokedTitle_ReturnsForbidden(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defTrendsetter)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	revokedAt := time.Now().UTC()
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:    title.StatusRevoked,
		RevokedAt: &revokedAt,
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if err := repo.SetPrimaryTitle(ctx, userID, ut.ID); !errors.Is(err, title.ErrForbidden) {
		t.Errorf("expected ErrForbidden for revoked title, got %v", err)
	}
}

// TestTitleRepo_SetPrimaryTitle_NonexistentTitle_ReturnsForbidden verifies
// that a nonexistent user_title ID returns ErrForbidden (the EXISTS subquery
// returns false → 0 rows updated → ErrForbidden).
func TestTitleRepo_SetPrimaryTitle_NonexistentTitle_ReturnsForbidden(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	if err := repo.SetPrimaryTitle(ctx, userID, uuid.New()); !errors.Is(err, title.ErrForbidden) {
		t.Errorf("expected ErrForbidden for nonexistent title, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 8. ClearPrimaryTitle
// ---------------------------------------------------------------------------

// TestTitleRepo_ClearPrimaryTitle_SetsNull verifies that ClearPrimaryTitle
// sets users.primary_title_id to NULL and GetPrimaryTitle returns nil.
func TestTitleRepo_ClearPrimaryTitle_SetsNull(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	if err := repo.SetPrimaryTitle(ctx, userID, ut.ID); err != nil {
		t.Fatalf("SetPrimaryTitle: %v", err)
	}

	if err := repo.ClearPrimaryTitle(ctx, userID); err != nil {
		t.Fatalf("ClearPrimaryTitle: %v", err)
	}

	s, err := repo.GetPrimaryTitle(ctx, userID)
	if err != nil {
		t.Fatalf("GetPrimaryTitle after clear: %v", err)
	}
	if s != nil {
		t.Errorf("GetPrimaryTitle after ClearPrimaryTitle should return nil, got %+v", s)
	}

	var colVal *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT primary_title_id FROM users WHERE id = $1`, userID,
	).Scan(&colVal); err != nil {
		t.Fatalf("query users.primary_title_id: %v", err)
	}
	if colVal != nil {
		t.Errorf("users.primary_title_id = %v after ClearPrimaryTitle, want NULL", *colVal)
	}
}

// TestTitleRepo_ClearPrimaryTitle_Idempotent verifies that ClearPrimaryTitle
// when already NULL returns no error (idempotent).
func TestTitleRepo_ClearPrimaryTitle_Idempotent(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	if err := repo.ClearPrimaryTitle(ctx, userID); err != nil {
		t.Fatalf("ClearPrimaryTitle first call: %v", err)
	}
	if err := repo.ClearPrimaryTitle(ctx, userID); err != nil {
		t.Fatalf("ClearPrimaryTitle second call: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 9. GetUserTitleForOwnershipCheck
// ---------------------------------------------------------------------------

// TestTitleRepo_GetOwnershipCheck_Owned_Succeeds verifies that a user can
// fetch their own title and receives the full UserTitle.
func TestTitleRepo_GetOwnershipCheck_Owned_Succeeds(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	created, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	fetched, err := repo.GetUserTitleForOwnershipCheck(ctx, created.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID = %v, want %v", fetched.ID, created.ID)
	}
	if fetched.UserID != userID {
		t.Errorf("UserID = %v, want %v", fetched.UserID, userID)
	}
	if fetched.Status != title.StatusActive {
		t.Errorf("Status = %v, want active", fetched.Status)
	}
}

// TestTitleRepo_GetOwnershipCheck_OtherUser_Forbidden verifies ErrForbidden
// when the title exists but belongs to a different user.
func TestTitleRepo_GetOwnershipCheck_OtherUser_Forbidden(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userA := newTitleTestUser(ctx, t, pool)
	userB := newTitleTestUser(ctx, t, pool)

	utA, err := repo.CreateUserTitle(ctx, userA, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	_, err = repo.GetUserTitleForOwnershipCheck(ctx, utA.ID, userB)
	if !errors.Is(err, title.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// TestTitleRepo_GetOwnershipCheck_Nonexistent_NotFound verifies ErrNotFound
// for a nonexistent user_title ID.
func TestTitleRepo_GetOwnershipCheck_Nonexistent_NotFound(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	_, err := repo.GetUserTitleForOwnershipCheck(ctx, uuid.New(), userID)
	if !errors.Is(err, title.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestTitleRepo_GetOwnershipCheck_RevokedTitle_Succeeds verifies that a
// revoked title is returned without error (status is not filtered by this
// method — ownership is the only check).
func TestTitleRepo_GetOwnershipCheck_RevokedTitle_Succeeds(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defNicheGuruFitness)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	revokedAt := time.Now().UTC()
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:    title.StatusRevoked,
		RevokedAt: &revokedAt,
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	fetched, err := repo.GetUserTitleForOwnershipCheck(ctx, ut.ID, userID)
	if err != nil {
		t.Fatalf("GetUserTitleForOwnershipCheck on revoked: %v", err)
	}
	if fetched.Status != title.StatusRevoked {
		t.Errorf("Status = %v, want revoked", fetched.Status)
	}
}

// ---------------------------------------------------------------------------
// Database constraint tests
// ---------------------------------------------------------------------------

// TestTitleConstraint_TitleTables_Exist verifies that all tables and columns
// created by migrations 0014–0016 exist in the schema.
func TestTitleConstraint_TitleTables_Exist(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	for _, tbl := range []string{"title_definitions", "user_titles"} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, tbl,
		).Scan(&exists); err != nil {
			t.Fatalf("checking table %q: %v", tbl, err)
		}
		if !exists {
			t.Errorf("table %q must exist after migrations", tbl)
		}
	}

	var colExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = 'users'
			  AND column_name  = 'primary_title_id'
		)`,
	).Scan(&colExists); err != nil {
		t.Fatalf("checking users.primary_title_id: %v", err)
	}
	if !colExists {
		t.Error("users.primary_title_id must exist after migration 0016")
	}
}

// TestTitleConstraint_NotificationEvents_Extended verifies that migration 0017
// added title_unlocked and title_grace_period to the notification_event ENUM.
func TestTitleConstraint_NotificationEvents_Extended(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	for _, v := range []string{"title_unlocked", "title_grace_period"} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_enum e
				JOIN pg_type t ON t.oid = e.enumtypid
				WHERE t.typname = 'notification_event'
				  AND e.enumlabel = $1
			)`, v,
		).Scan(&exists); err != nil {
			t.Fatalf("checking notification_event value %q: %v", v, err)
		}
		if !exists {
			t.Errorf("notification_event ENUM missing %q (migration 0017)", v)
		}
	}
}

// TestTitleConstraint_SlugUnique verifies that inserting two title_definitions
// with the same slug produces a unique constraint violation.
func TestTitleConstraint_SlugUnique(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	slug := "dup_slug_" + uuid.New().String()[:8]

	if _, err := pool.Exec(ctx, `
		INSERT INTO title_definitions (id, slug, display_name, category)
		VALUES ($1, $2, 'Test Title', 'milestone')`,
		uuid.New(), slug,
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO title_definitions (id, slug, display_name, category)
		VALUES ($1, $2, 'Test Title 2', 'milestone')`,
		uuid.New(), slug,
	)
	if err == nil {
		t.Error("expected unique constraint violation on duplicate slug, got nil")
	}
}

// TestTitleConstraint_UserTitles_UserFK verifies that inserting a user_titles
// row with a nonexistent user_id fails with a foreign-key violation.
func TestTitleConstraint_UserTitles_UserFK(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO user_titles (id, user_id, title_definition_id, status, unlocked_at, created_at)
		VALUES ($1, $2, $3, 'active', now(), now())`,
		uuid.New(), uuid.New(), defFoundingMember,
	)
	if err == nil {
		t.Error("expected FK violation for nonexistent user_id, got nil")
	}
}

// TestTitleConstraint_UserTitles_DefinitionFK verifies that inserting a
// user_titles row with a nonexistent title_definition_id fails.
func TestTitleConstraint_UserTitles_DefinitionFK(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()

	userID := newTitleTestUser(ctx, t, pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO user_titles (id, user_id, title_definition_id, status, unlocked_at, created_at)
		VALUES ($1, $2, $3, 'active', now(), now())`,
		uuid.New(), userID, uuid.New(),
	)
	if err == nil {
		t.Error("expected FK violation for nonexistent title_definition_id, got nil")
	}
}

// TestTitleConstraint_PartialUniqueIndex_PreventsDuplicateActive verifies that
// the user_titles_one_active_per_def_idx partial unique index prevents a user
// from holding two active rows for the same definition simultaneously.
func TestTitleConstraint_PartialUniqueIndex_PreventsDuplicateActive(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	if _, err := repo.CreateUserTitle(ctx, userID, defFoundingMember); err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}

	// Bypass ON CONFLICT DO NOTHING to trigger the raw constraint.
	_, err := pool.Exec(ctx, `
		INSERT INTO user_titles (id, user_id, title_definition_id, status, unlocked_at, created_at)
		VALUES ($1, $2, $3, 'active', now(), now())`,
		uuid.New(), userID, defFoundingMember,
	)
	if err == nil {
		t.Error("expected partial unique constraint violation for duplicate active row, got nil")
	}
}

// TestTitleConstraint_RevokedAllowsReearn verifies that a revoked row does not
// prevent a new active row for the same (user_id, title_definition_id) pair,
// because revoked rows are excluded from the partial unique index.
func TestTitleConstraint_RevokedAllowsReearn(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)

	ut, err := repo.CreateUserTitle(ctx, userID, defNicheGuruTech)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	revokedAt := time.Now().UTC()
	if err := repo.UpdateUserTitleStatus(ctx, ut.ID, title.TitleStatusUpdate{
		Status:    title.StatusRevoked,
		RevokedAt: &revokedAt,
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Re-earn: direct INSERT with status='active' must not violate the partial index.
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_titles (id, user_id, title_definition_id, status, unlocked_at, created_at)
		VALUES ($1, $2, $3, 'active', now(), now())`,
		uuid.New(), userID, defNicheGuruTech,
	); err != nil {
		t.Errorf("re-earning after revocation must succeed, got: %v", err)
	}
}

// TestTitleConstraint_OnDeleteSetNull verifies the users_primary_title_id_fkey
// ON DELETE SET NULL behavior: deleting a user_titles row that is currently set
// as the user's primary title automatically sets users.primary_title_id = NULL.
func TestTitleConstraint_OnDeleteSetNull(t *testing.T) {
	pool := connectTestDB(t)
	ctx := context.Background()
	repo := title.NewRepository(pool)

	userID := newTitleTestUser(ctx, t, pool)
	ut, err := repo.CreateUserTitle(ctx, userID, defFoundingMember)
	if err != nil {
		t.Fatalf("CreateUserTitle: %v", err)
	}
	if err := repo.SetPrimaryTitle(ctx, userID, ut.ID); err != nil {
		t.Fatalf("SetPrimaryTitle: %v", err)
	}

	// Delete the user_title row directly to trigger ON DELETE SET NULL.
	if _, err := pool.Exec(ctx, `DELETE FROM user_titles WHERE id = $1`, ut.ID); err != nil {
		t.Fatalf("DELETE user_title: %v", err)
	}

	// Column must be NULL.
	var colVal *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT primary_title_id FROM users WHERE id = $1`, userID,
	).Scan(&colVal); err != nil {
		t.Fatalf("query users.primary_title_id: %v", err)
	}
	if colVal != nil {
		t.Errorf("users.primary_title_id = %v after user_title deletion; want NULL (ON DELETE SET NULL)", *colVal)
	}

	// GetPrimaryTitle must confirm nil.
	s, err := repo.GetPrimaryTitle(ctx, userID)
	if err != nil {
		t.Fatalf("GetPrimaryTitle after ON DELETE SET NULL: %v", err)
	}
	if s != nil {
		t.Errorf("GetPrimaryTitle should return nil after ON DELETE SET NULL, got %+v", s)
	}
}
