package report

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

const (
	reportRateMax    int64         = 10
	reportRateWindow time.Duration = 15 * time.Minute
)

// PostChecker is satisfied by *post.Service or *post.Repository.
// Avoids a direct import of internal/post from internal/report, which
// would require internal/post to be aware of internal/report for injection.
type PostChecker interface {
	PostExistsAndNotDeleted(ctx context.Context, postID uuid.UUID) (bool, error)
}

// UserChecker is satisfied by *user.Service or *user.Repository.
type UserChecker interface {
	UserExists(ctx context.Context, userID uuid.UUID) (bool, error)
}

// Service implements report business logic.
type Service struct {
	repo        *Repository
	postChecker PostChecker
	userChecker UserChecker
	rdb         *redis.Client // MUST NOT be nil — rate limiting is fail-closed
	logger      *zap.Logger
	events      *platformMetrics.Events
}

// SetEvents injects the Prometheus event counters into the Service.
// Passing nil disables counter instrumentation (no-op, safe for unit tests).
func (s *Service) SetEvents(e *platformMetrics.Events) {
	s.events = e
}

// NewService constructs a report Service.
// rdb must not be nil in production: report submission is an abuse-sensitive
// operation and rate limiting is fail-closed (CodeServiceUnavailable when rdb
// is nil or returns an error).
func NewService(
	repo *Repository,
	postChecker PostChecker,
	userChecker UserChecker,
	rdb *redis.Client,
	logger *zap.Logger,
) *Service {
	return &Service{
		repo:        repo,
		postChecker: postChecker,
		userChecker: userChecker,
		rdb:         rdb,
		logger:      logger,
	}
}

// SubmitPostReport records a report against a post.
//
// Validation order (all server-side):
//  1. Rate limit — fail-closed (nil rdb or Redis error → CodeServiceUnavailable).
//  2. reason must be in validReasons.
//  3. detail, if provided, must be ≤ 500 code points.
//  4. Post must exist and not be soft-deleted.
//  5. Insert — ON CONFLICT DO NOTHING; duplicate pending → nil (idempotent 204).
func (s *Service) SubmitPostReport(ctx context.Context, reporterID, postID uuid.UUID, req CreateReportRequest) error {
	// Step 1: rate limit — fail-closed.
	if err := s.enforceRateLimit(ctx, reporterID); err != nil {
		return err
	}

	// Step 2: validate reason.
	if _, ok := validReasons[req.Reason]; !ok {
		return apierror.NewAPIError(apierror.CodeValidation, fmt.Sprintf("invalid reason %q", req.Reason))
	}

	// Step 3: validate detail length.
	if err := validateDetail(req.Detail); err != nil {
		return err
	}

	// Step 4: verify post exists and is not deleted.
	exists, err := s.postChecker.PostExistsAndNotDeleted(ctx, postID)
	if err != nil {
		s.logger.Error("report: post existence check", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if !exists {
		return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
	}

	// Step 5: insert — idempotent via ON CONFLICT DO NOTHING.
	postIDCopy := postID
	_, insertErr := s.repo.SubmitReport(ctx, reporterID, ReportTargetPost, &postIDCopy, nil, req.Reason, req.Detail)
	if insertErr != nil {
		s.logger.Error("report: submit post report", zap.Error(insertErr))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	// created=false (duplicate pending) is treated as idempotent success.
	return nil
}

// SubmitUserReport records a report against a user account.
//
// Validation order (all server-side):
//  1. Rate limit — fail-closed.
//  2. reporterID must not equal targetUserID (no self-reports).
//  3. reason must be in validReasons.
//  4. detail, if provided, must be ≤ 500 code points.
//  5. Target user must exist.
//  6. Insert — ON CONFLICT DO NOTHING; duplicate pending → nil (idempotent 204).
//
// Block relationships do NOT prevent safety reporting (safety design).
func (s *Service) SubmitUserReport(ctx context.Context, reporterID, targetUserID uuid.UUID, req CreateReportRequest) error {
	// Step 1: rate limit — fail-closed.
	if err := s.enforceRateLimit(ctx, reporterID); err != nil {
		return err
	}

	// Step 2: self-report guard.
	if reporterID == targetUserID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot report your own account")
	}

	// Step 3: validate reason.
	if _, ok := validReasons[req.Reason]; !ok {
		return apierror.NewAPIError(apierror.CodeValidation, fmt.Sprintf("invalid reason %q", req.Reason))
	}

	// Step 4: validate detail length.
	if err := validateDetail(req.Detail); err != nil {
		return err
	}

	// Step 5: verify target user exists.
	exists, err := s.userChecker.UserExists(ctx, targetUserID)
	if err != nil {
		s.logger.Error("report: user existence check", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if !exists {
		return apierror.NewAPIError(apierror.CodeNotFound, "user not found")
	}

	// Step 6: insert — idempotent via ON CONFLICT DO NOTHING.
	userIDCopy := targetUserID
	_, insertErr := s.repo.SubmitReport(ctx, reporterID, ReportTargetUser, nil, &userIDCopy, req.Reason, req.Detail)
	if insertErr != nil {
		s.logger.Error("report: submit user report", zap.Error(insertErr))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// enforceRateLimit checks the sliding-window rate limit for the reporter.
// Key: rl:report:{reporterID}, limit: 10 per 15 min.
// Fail-CLOSED: nil rdb or any Redis error → CodeServiceUnavailable.
// This is intentionally stricter than the reaction package (fail-open) because
// report submission is an abuse-sensitive operation.
func (s *Service) enforceRateLimit(ctx context.Context, reporterID uuid.UUID) error {
	if s.rdb == nil {
		s.logger.Error("report: rate limit: Redis client is nil — failing closed",
			zap.String("reporter_id", reporterID.String()),
		)
		return apierror.NewAPIError(apierror.CodeServiceUnavailable, "Service temporarily unavailable.")
	}

	key := fmt.Sprintf("rl:report:%s", reporterID.String())

	// Initialize key with TTL only when it does not exist yet, then increment.
	// Mirrors the pattern in auth.RateLimitMiddleware and bookmark rate limit.
	if err := s.rdb.SetNX(ctx, key, 0, reportRateWindow).Err(); err != nil {
		s.logger.Error("report: rate limit: Redis SetNX failed — failing closed",
			zap.String("reporter_id", reporterID.String()),
			zap.Error(err),
		)
		return apierror.NewAPIError(apierror.CodeServiceUnavailable, "Service temporarily unavailable.")
	}

	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		s.logger.Error("report: rate limit: Redis Incr failed — failing closed",
			zap.String("reporter_id", reporterID.String()),
			zap.Error(err),
		)
		return apierror.NewAPIError(apierror.CodeServiceUnavailable, "Service temporarily unavailable.")
	}

	// Belt-and-suspenders: ensure TTL is always set.
	if count == 1 {
		if expErr := s.rdb.Expire(ctx, key, reportRateWindow).Err(); expErr != nil {
			s.logger.Error("report: rate limit: Redis Expire failed after count==1",
				zap.String("key", key),
				zap.Error(expErr),
			)
		}
	}

	if count > reportRateMax {
		s.events.RecordRateLimit(platformMetrics.RateLimitCategoryReport, platformMetrics.RateLimitResultRejected)
		return apierror.NewAPIError(apierror.CodeRateLimit, "Too many report requests. Please try again later.")
	}

	s.events.RecordRateLimit(platformMetrics.RateLimitCategoryReport, platformMetrics.RateLimitResultAllowed)
	return nil
}

// validateDetail returns CodeValidation if the detail string exceeds maxDetailCodePoints.
func validateDetail(detail *string) error {
	if detail == nil {
		return nil
	}
	if utf8.RuneCountInString(*detail) > maxDetailCodePoints {
		return apierror.NewAPIError(apierror.CodeValidation,
			fmt.Sprintf("detail must not exceed %d characters", maxDetailCodePoints))
	}
	return nil
}
