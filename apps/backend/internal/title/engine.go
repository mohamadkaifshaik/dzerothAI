package title

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ErrNotQualifiable is returned when the caller requests qualification for
// a title that the engine does not evaluate (e.g. top_1pct_creator with
// is_active=FALSE, or an unrecognised slug).
var ErrNotQualifiable = errors.New("title: not qualifiable")

// Product constants — authoritative for Title System v1.
const (
	platformLaunchDateStr    = "2027-04-20"
	foundingMemberWindowDays = 30
	centurionPostThreshold   = int64(100)
	trendsetterLikeThreshold = int64(10_000)
	nichePostThreshold       = int64(15)
	nicheEngagementThreshold = 5.0 // F1 percentage
)

// Niche Guru tag sets (stored lowercase without #).
var (
	nicheGuruTechTags    = []string{"tech", "gadgets"}
	nicheGuruFitnessTags = []string{"fitness", "workout"}
)

// qualificationRepo is the subset of Repository methods used by the engine.
// Unexported so only *Repository (same package) satisfies it, and fakes can be
// used in tests.
type qualificationRepo interface {
	GetDefinitionBySlug(ctx context.Context, slug string) (*TitleDefinition, error)
	CreateUserTitle(ctx context.Context, userID, definitionID uuid.UUID) (*UserTitle, error)
	GetUserCreatedAt(ctx context.Context, userID uuid.UUID) (time.Time, error)
	CountCenturionPosts(ctx context.Context, userID uuid.UUID) (int64, error)
	GetTrendsetterLikes(ctx context.Context, userID uuid.UUID) (int64, error)
	GetNicheMetrics(ctx context.Context, userID uuid.UUID, tags []string) (int64, int64, error)
}

// Engine evaluates title qualification rules and unlocks titles for users.
type Engine struct {
	repo qualificationRepo
	// logger is used to record qualification errors that do not abort the full
	// EvaluateAndUnlock run so that a single failing title does not block others.
	logger *zap.Logger
	// foundingMemberDeadline is the inclusive UTC calendar-day boundary.
	foundingMemberDeadline time.Time
}

// NewEngine constructs a qualification Engine.
func NewEngine(repo qualificationRepo, logger *zap.Logger) *Engine {
	// Parse the platform launch date and compute the founding member deadline.
	launch, err := time.Parse("2006-01-02", platformLaunchDateStr)
	if err != nil {
		panic(fmt.Sprintf("title: invalid platformLaunchDateStr: %v", err))
	}
	deadline := launch.UTC().AddDate(0, 0, foundingMemberWindowDays)
	return &Engine{
		repo:                   repo,
		logger:                 logger,
		foundingMemberDeadline: deadline,
	}
}

// QualificationResult is returned by Qualify for a single title.
type QualificationResult struct {
	DefinitionID uuid.UUID
	Slug         string
	Qualified    bool
	// UserTitle is non-nil when Qualified is true and the title was created or
	// already existed. Nil when Qualified is false.
	UserTitle *UserTitle
}

// Qualify evaluates whether userID qualifies for the title identified by slug,
// and if so, creates (or returns the existing) user_titles row.
//
// Returns ErrNotQualifiable for top_1pct_creator (is_active=FALSE) or unknown
// slugs. Returns ErrNotFound if the definition does not exist.
func (e *Engine) Qualify(ctx context.Context, userID uuid.UUID, slug string) (QualificationResult, error) {
	// Guard: top_1pct_creator is never evaluated.
	if slug == "top_1pct_creator" {
		return QualificationResult{Slug: slug}, ErrNotQualifiable
	}

	var qualified bool
	var evalErr error

	switch slug {
	case "founding_member":
		qualified, evalErr = e.qualifyFoundingMember(ctx, userID)
	case "centurion":
		qualified, evalErr = e.qualifyCenturion(ctx, userID)
	case "trendsetter":
		qualified, evalErr = e.qualifyTrendsetter(ctx, userID)
	case "niche_guru_tech":
		qualified, evalErr = e.qualifyNicheGuru(ctx, userID, nicheGuruTechTags)
	case "niche_guru_fitness":
		qualified, evalErr = e.qualifyNicheGuru(ctx, userID, nicheGuruFitnessTags)
	default:
		return QualificationResult{Slug: slug}, ErrNotQualifiable
	}

	if evalErr != nil {
		return QualificationResult{Slug: slug}, evalErr
	}

	// Fetch definition to resolve UUID for CreateUserTitle.
	def, err := e.repo.GetDefinitionBySlug(ctx, slug)
	if err != nil {
		return QualificationResult{Slug: slug}, fmt.Errorf("title: qualify fetch definition %q: %w", slug, err)
	}

	result := QualificationResult{
		DefinitionID: def.ID,
		Slug:         slug,
		Qualified:    qualified,
	}

	if !qualified {
		return result, nil
	}

	ut, err := e.repo.CreateUserTitle(ctx, userID, def.ID)
	if err != nil {
		return QualificationResult{DefinitionID: def.ID, Slug: slug}, fmt.Errorf("title: qualify create user title %q: %w", slug, err)
	}
	result.UserTitle = ut
	return result, nil
}

// EvaluateAndUnlock evaluates all active qualifiable titles for userID and
// unlocks each one the user qualifies for. It is idempotent (CreateUserTitle
// uses ON CONFLICT DO NOTHING).
//
// Returns a slice of QualificationResult — one per evaluated title (whether
// qualified or not). The method continues past individual qualification errors,
// logging them, so a single failing title does not block others.
func (e *Engine) EvaluateAndUnlock(ctx context.Context, userID uuid.UUID) ([]QualificationResult, error) {
	slugs := []string{
		"founding_member",
		"centurion",
		"trendsetter",
		"niche_guru_tech",
		"niche_guru_fitness",
		// top_1pct_creator is intentionally excluded — ErrNotQualifiable.
	}

	results := make([]QualificationResult, 0, len(slugs))
	for _, slug := range slugs {
		res, err := e.Qualify(ctx, userID, slug)
		if err != nil {
			e.logger.Warn("title: EvaluateAndUnlock qualification error",
				zap.String("slug", slug),
				zap.Stringer("user_id", userID),
				zap.Error(err),
			)
			// Continue — do not abort other titles because one failed.
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// per-title qualification methods
// ---------------------------------------------------------------------------

// qualifyFoundingMember calls GetUserCreatedAt and applies isFoundingMemberEligible.
func (e *Engine) qualifyFoundingMember(ctx context.Context, userID uuid.UUID) (bool, error) {
	createdAt, err := e.repo.GetUserCreatedAt(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("title: qualify founding_member get created_at: %w", err)
	}
	return isFoundingMemberEligible(createdAt, e.foundingMemberDeadline), nil
}

// qualifyCenturion calls CountCenturionPosts and applies isCenturionQualified.
func (e *Engine) qualifyCenturion(ctx context.Context, userID uuid.UUID) (bool, error) {
	n, err := e.repo.CountCenturionPosts(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("title: qualify centurion count posts: %w", err)
	}
	return isCenturionQualified(n), nil
}

// qualifyTrendsetter calls GetTrendsetterLikes and applies isTrendsetterQualified.
func (e *Engine) qualifyTrendsetter(ctx context.Context, userID uuid.UUID) (bool, error) {
	n, err := e.repo.GetTrendsetterLikes(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("title: qualify trendsetter get likes: %w", err)
	}
	return isTrendsetterQualified(n), nil
}

// qualifyNicheGuru calls GetNicheMetrics and applies isNicheGuruQualified.
func (e *Engine) qualifyNicheGuru(ctx context.Context, userID uuid.UUID, tags []string) (bool, error) {
	posts, likes, err := e.repo.GetNicheMetrics(ctx, userID, tags)
	if err != nil {
		return false, fmt.Errorf("title: qualify niche_guru get metrics: %w", err)
	}
	return isNicheGuruQualified(posts, likes), nil
}

// ---------------------------------------------------------------------------
// pure threshold functions (testable without database)
// ---------------------------------------------------------------------------

// isFoundingMemberEligible returns true if createdAt (UTC calendar day) is on
// or before the deadline (UTC calendar day).
func isFoundingMemberEligible(createdAt, deadline time.Time) bool {
	day := createdAt.UTC().Truncate(24 * time.Hour)
	return !day.After(deadline.UTC().Truncate(24 * time.Hour))
}

// isCenturionQualified returns true if postCount >= centurionPostThreshold.
func isCenturionQualified(postCount int64) bool {
	return postCount >= centurionPostThreshold
}

// isTrendsetterQualified returns true if likeCount > trendsetterLikeThreshold.
func isTrendsetterQualified(likeCount int64) bool {
	return likeCount > trendsetterLikeThreshold
}

// isNicheGuruQualified returns true if posts > nichePostThreshold AND
// F1 engagement > nicheEngagementThreshold. Avoids division by zero.
func isNicheGuruQualified(posts, likes int64) bool {
	if posts <= nichePostThreshold {
		return false
	}
	f1 := computeNicheEngagement(posts, likes)
	return f1 > nicheEngagementThreshold
}

// computeNicheEngagement returns (likes/posts)*100. Returns 0 if posts == 0.
func computeNicheEngagement(posts, likes int64) float64 {
	if posts == 0 {
		return 0
	}
	return (float64(likes) / float64(posts)) * 100
}
