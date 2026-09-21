//go:build !integration

package title

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// fakeQualificationRepo
// ---------------------------------------------------------------------------

type fakeQualificationRepo struct {
	createdAt        time.Time
	centurionPosts   int64
	trendsetterLikes int64
	nichePosts       int64
	nicheLikes       int64
	definitions      map[string]*TitleDefinition
	createdTitles    []*UserTitle
	// err is returned by all methods when non-nil.
	err error
}

func (f *fakeQualificationRepo) GetDefinitionBySlug(_ context.Context, slug string) (*TitleDefinition, error) {
	if f.err != nil {
		return nil, f.err
	}
	if d, ok := f.definitions[slug]; ok {
		return d, nil
	}
	return nil, ErrNotFound
}

func (f *fakeQualificationRepo) CreateUserTitle(_ context.Context, userID, definitionID uuid.UUID) (*UserTitle, error) {
	if f.err != nil {
		return nil, f.err
	}
	ut := &UserTitle{
		ID:                uuid.New(),
		UserID:            userID,
		TitleDefinitionID: definitionID,
		Status:            StatusActive,
		UnlockedAt:        time.Now().UTC(),
		CreatedAt:         time.Now().UTC(),
	}
	f.createdTitles = append(f.createdTitles, ut)
	return ut, nil
}

func (f *fakeQualificationRepo) GetUserCreatedAt(_ context.Context, _ uuid.UUID) (time.Time, error) {
	if f.err != nil {
		return time.Time{}, f.err
	}
	return f.createdAt, nil
}

func (f *fakeQualificationRepo) CountCenturionPosts(_ context.Context, _ uuid.UUID) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.centurionPosts, nil
}

func (f *fakeQualificationRepo) GetTrendsetterLikes(_ context.Context, _ uuid.UUID) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.trendsetterLikes, nil
}

func (f *fakeQualificationRepo) GetNicheMetrics(_ context.Context, _ uuid.UUID, _ []string) (int64, int64, error) {
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.nichePosts, f.nicheLikes, nil
}

// newFakeRepo builds a fakeQualificationRepo with well-known seed definition UUIDs.
func newFakeRepo() *fakeQualificationRepo {
	defs := map[string]*TitleDefinition{
		"founding_member": {
			ID:          uuid.MustParse("00000000-0000-7000-8000-000000000001"),
			Slug:        "founding_member",
			DisplayName: "Founding Member",
			Category:    CategoryMilestone,
			IsRevocable: false,
			IsActive:    true,
		},
		"centurion": {
			ID:          uuid.MustParse("00000000-0000-7000-8000-000000000002"),
			Slug:        "centurion",
			DisplayName: "Centurion",
			Category:    CategoryMilestone,
			IsRevocable: false,
			IsActive:    true,
		},
		"trendsetter": {
			ID:          uuid.MustParse("00000000-0000-7000-8000-000000000003"),
			Slug:        "trendsetter",
			DisplayName: "Trendsetter",
			Category:    CategoryPerformance,
			IsRevocable: true,
			IsActive:    true,
		},
		"niche_guru_tech": {
			ID:          uuid.MustParse("00000000-0000-7000-8000-000000000004"),
			Slug:        "niche_guru_tech",
			DisplayName: "Niche Guru: Tech",
			Category:    CategoryNiche,
			IsRevocable: true,
			IsActive:    true,
		},
		"niche_guru_fitness": {
			ID:          uuid.MustParse("00000000-0000-7000-8000-000000000005"),
			Slug:        "niche_guru_fitness",
			DisplayName: "Niche Guru: Fitness",
			Category:    CategoryNiche,
			IsRevocable: true,
			IsActive:    true,
		},
		"top_1pct_creator": {
			ID:          uuid.MustParse("00000000-0000-7000-8000-000000000006"),
			Slug:        "top_1pct_creator",
			DisplayName: "Top 1% Creator",
			Category:    CategoryPerformance,
			IsRevocable: true,
			IsActive:    false,
		},
	}
	return &fakeQualificationRepo{definitions: defs}
}

// ---------------------------------------------------------------------------
// TestNewEngine_FoundingMemberDeadline
// ---------------------------------------------------------------------------

func TestNewEngine_FoundingMemberDeadline(t *testing.T) {
	t.Parallel()

	eng := NewEngine(newFakeRepo(), zap.NewNop())

	// platformLaunchDate is 2027-04-20; deadline = launch + 30 days = 2027-05-20
	want := time.Date(2027, 5, 20, 0, 0, 0, 0, time.UTC)
	got := eng.foundingMemberDeadline.UTC()
	if !got.Equal(want) {
		t.Errorf("foundingMemberDeadline = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestIsFoundingMemberEligible
// ---------------------------------------------------------------------------

func TestIsFoundingMemberEligible(t *testing.T) {
	t.Parallel()

	deadline := time.Date(2027, 5, 20, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		createdAt time.Time
		want      bool
	}{
		{
			name:      "well before deadline",
			createdAt: time.Date(2027, 4, 20, 0, 0, 0, 0, time.UTC),
			want:      true,
		},
		{
			name:      "one day before deadline",
			createdAt: time.Date(2027, 5, 19, 23, 59, 59, 0, time.UTC),
			want:      true,
		},
		{
			name:      "exactly on deadline calendar day",
			createdAt: time.Date(2027, 5, 20, 0, 0, 0, 0, time.UTC),
			want:      true,
		},
		{
			name:      "end of deadline calendar day",
			createdAt: time.Date(2027, 5, 20, 23, 59, 59, 999999999, time.UTC),
			want:      true,
		},
		{
			name:      "one day after deadline",
			createdAt: time.Date(2027, 5, 21, 0, 0, 0, 0, time.UTC),
			want:      false,
		},
		{
			name:      "well after deadline",
			createdAt: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC),
			want:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isFoundingMemberEligible(tc.createdAt, deadline)
			if got != tc.want {
				t.Errorf("isFoundingMemberEligible(%v) = %v, want %v", tc.createdAt, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestIsCenturionQualified
// ---------------------------------------------------------------------------

func TestIsCenturionQualified(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		posts int64
		want  bool
	}{
		{"zero", 0, false},
		{"99", 99, false},
		{"100 exactly", 100, true},
		{"101", 101, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isCenturionQualified(tc.posts); got != tc.want {
				t.Errorf("isCenturionQualified(%d) = %v, want %v", tc.posts, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestIsTrendsetterQualified
// ---------------------------------------------------------------------------

func TestIsTrendsetterQualified(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		likes int64
		want  bool
	}{
		{"zero", 0, false},
		{"10000 at threshold", 10_000, false},
		{"10001 strictly greater", 10_001, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isTrendsetterQualified(tc.likes); got != tc.want {
				t.Errorf("isTrendsetterQualified(%d) = %v, want %v", tc.likes, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestComputeNicheEngagement
// ---------------------------------------------------------------------------

func TestComputeNicheEngagement(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		posts int64
		likes int64
		want  float64
	}{
		{"zero posts zero likes", 0, 0, 0},
		{"100 posts 6 likes", 100, 6, 6.0},
		{"100 posts 5 likes", 100, 5, 5.0},
		{"16 posts 1 like", 16, 1, 6.25},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeNicheEngagement(tc.posts, tc.likes)
			if got != tc.want {
				t.Errorf("computeNicheEngagement(%d, %d) = %v, want %v", tc.posts, tc.likes, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestIsNicheGuruQualified
// ---------------------------------------------------------------------------

func TestIsNicheGuruQualified(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		posts int64
		likes int64
		want  bool
	}{
		{"posts at threshold 15", 15, 100, false},
		{"posts=16 likes=0 F1=0%", 16, 0, false},
		{"posts=16 likes=1 F1=6.25% above threshold", 16, 1, true},
		{"posts=100 likes=6 F1=6%", 100, 6, true},
		{"posts=100 likes=5 F1=5% at threshold", 100, 5, false},
		{"zero posts", 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isNicheGuruQualified(tc.posts, tc.likes)
			if got != tc.want {
				t.Errorf("isNicheGuruQualified(%d, %d) = %v, want %v", tc.posts, tc.likes, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestQualify_NotQualifiable
// ---------------------------------------------------------------------------

func TestQualify_Top1Pct_NotQualifiable(t *testing.T) {
	t.Parallel()

	eng := NewEngine(newFakeRepo(), zap.NewNop())
	_, err := eng.Qualify(context.Background(), uuid.New(), "top_1pct_creator")
	if !errors.Is(err, ErrNotQualifiable) {
		t.Errorf("expected ErrNotQualifiable for top_1pct_creator, got %v", err)
	}
}

func TestQualify_UnknownSlug_NotQualifiable(t *testing.T) {
	t.Parallel()

	eng := NewEngine(newFakeRepo(), zap.NewNop())
	_, err := eng.Qualify(context.Background(), uuid.New(), "nonexistent_slug")
	if !errors.Is(err, ErrNotQualifiable) {
		t.Errorf("expected ErrNotQualifiable for unknown slug, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestQualify_FoundingMember
// ---------------------------------------------------------------------------

func TestQualify_FoundingMember_Eligible(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	// User created during the window.
	repo.createdAt = time.Date(2027, 4, 25, 10, 0, 0, 0, time.UTC)
	eng := NewEngine(repo, zap.NewNop())

	userID := uuid.New()
	res, err := eng.Qualify(context.Background(), userID, "founding_member")
	if err != nil {
		t.Fatalf("Qualify founding_member: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for user within founding window")
	}
	if res.UserTitle == nil {
		t.Error("expected UserTitle to be non-nil when Qualified=true")
	}
	if len(repo.createdTitles) != 1 {
		t.Errorf("expected 1 created title, got %d", len(repo.createdTitles))
	}
}

func TestQualify_FoundingMember_Expired(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	// User created after the window.
	repo.createdAt = time.Date(2027, 5, 21, 0, 0, 0, 0, time.UTC)
	eng := NewEngine(repo, zap.NewNop())

	res, err := eng.Qualify(context.Background(), uuid.New(), "founding_member")
	if err != nil {
		t.Fatalf("Qualify founding_member: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false for user outside founding window")
	}
	if res.UserTitle != nil {
		t.Error("expected UserTitle to be nil when Qualified=false")
	}
	if len(repo.createdTitles) != 0 {
		t.Errorf("expected no created titles when not qualified, got %d", len(repo.createdTitles))
	}
}

// ---------------------------------------------------------------------------
// TestQualify_Centurion
// ---------------------------------------------------------------------------

func TestQualify_Centurion_Qualified(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.centurionPosts = 100
	eng := NewEngine(repo, zap.NewNop())

	res, err := eng.Qualify(context.Background(), uuid.New(), "centurion")
	if err != nil {
		t.Fatalf("Qualify centurion: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for 100 posts")
	}
}

func TestQualify_Centurion_NotQualified(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.centurionPosts = 99
	eng := NewEngine(repo, zap.NewNop())

	res, err := eng.Qualify(context.Background(), uuid.New(), "centurion")
	if err != nil {
		t.Fatalf("Qualify centurion: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false for 99 posts")
	}
}

// ---------------------------------------------------------------------------
// TestQualify_Trendsetter
// ---------------------------------------------------------------------------

func TestQualify_Trendsetter_Qualified(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.trendsetterLikes = 10_001
	eng := NewEngine(repo, zap.NewNop())

	res, err := eng.Qualify(context.Background(), uuid.New(), "trendsetter")
	if err != nil {
		t.Fatalf("Qualify trendsetter: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for 10001 likes")
	}
}

func TestQualify_Trendsetter_AtThreshold(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.trendsetterLikes = 10_000
	eng := NewEngine(repo, zap.NewNop())

	res, err := eng.Qualify(context.Background(), uuid.New(), "trendsetter")
	if err != nil {
		t.Fatalf("Qualify trendsetter: %v", err)
	}
	if res.Qualified {
		t.Error("expected Qualified=false for exactly 10000 likes (strictly greater required)")
	}
}

// ---------------------------------------------------------------------------
// TestQualify_NicheGuru
// ---------------------------------------------------------------------------

func TestQualify_NicheGuruTech_Qualified(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	// 16 posts, 2 likes each → F1 = 12.5%
	repo.nichePosts = 16
	repo.nicheLikes = 32
	eng := NewEngine(repo, zap.NewNop())

	res, err := eng.Qualify(context.Background(), uuid.New(), "niche_guru_tech")
	if err != nil {
		t.Fatalf("Qualify niche_guru_tech: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true for 16 posts F1=200%")
	}
}

func TestQualify_NicheGuruFitness_Qualified(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.nichePosts = 20
	repo.nicheLikes = 4 // F1 = 20%
	eng := NewEngine(repo, zap.NewNop())

	res, err := eng.Qualify(context.Background(), uuid.New(), "niche_guru_fitness")
	if err != nil {
		t.Fatalf("Qualify niche_guru_fitness: %v", err)
	}
	if !res.Qualified {
		t.Error("expected Qualified=true")
	}
}

// ---------------------------------------------------------------------------
// TestEvaluateAndUnlock_Idempotent
// ---------------------------------------------------------------------------

func TestEvaluateAndUnlock_AllTitlesEvaluated(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	// User qualifies for founding_member and centurion.
	repo.createdAt = time.Date(2027, 4, 20, 0, 0, 0, 0, time.UTC)
	repo.centurionPosts = 100
	repo.trendsetterLikes = 0
	repo.nichePosts = 0
	repo.nicheLikes = 0
	eng := NewEngine(repo, zap.NewNop())

	results, err := eng.EvaluateAndUnlock(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("EvaluateAndUnlock: %v", err)
	}

	// 5 qualifiable slugs (top_1pct_creator is skipped).
	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}

	bySlug := make(map[string]QualificationResult, len(results))
	for _, r := range results {
		bySlug[r.Slug] = r
	}

	if !bySlug["founding_member"].Qualified {
		t.Error("founding_member should be Qualified=true")
	}
	if !bySlug["centurion"].Qualified {
		t.Error("centurion should be Qualified=true")
	}
	if bySlug["trendsetter"].Qualified {
		t.Error("trendsetter should be Qualified=false")
	}
	if bySlug["niche_guru_tech"].Qualified {
		t.Error("niche_guru_tech should be Qualified=false")
	}
	if bySlug["niche_guru_fitness"].Qualified {
		t.Error("niche_guru_fitness should be Qualified=false")
	}
	if _, ok := bySlug["top_1pct_creator"]; ok {
		t.Error("top_1pct_creator must not appear in EvaluateAndUnlock results")
	}

	// 2 titles created.
	if len(repo.createdTitles) != 2 {
		t.Errorf("expected 2 created titles, got %d", len(repo.createdTitles))
	}
}

func TestEvaluateAndUnlock_ContinuesPastError(t *testing.T) {
	t.Parallel()

	// Simulate an error on the repo — all methods fail.
	repo := newFakeRepo()
	repo.err = errors.New("simulated db failure")
	eng := NewEngine(repo, zap.NewNop())

	// Should not panic; should return empty results (all titles logged as errors).
	results, err := eng.EvaluateAndUnlock(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("EvaluateAndUnlock returned unexpected error: %v", err)
	}
	// All titles skipped due to errors.
	if len(results) != 0 {
		t.Errorf("expected 0 results when all titles fail, got %d", len(results))
	}
}
