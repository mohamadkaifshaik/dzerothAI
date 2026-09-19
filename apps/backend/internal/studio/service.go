package studio

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

const (
	studioRateMax    int64         = 60
	studioRateWindow time.Duration = 15 * time.Minute
)

// Service implements Creator Studio analytics business logic.
type Service struct {
	repo   *Repository
	rdb    *redis.Client // nullable — fail-open when nil
	logger *zap.Logger
}

// NewService constructs a studio Service.
// rdb may be nil; when nil rate limiting is skipped (fail-open).
func NewService(repo *Repository, rdb *redis.Client, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		rdb:    rdb,
		logger: logger,
	}
}

// GetStudioAnalytics returns a finite paginated page of private analytics for
// the caller's own posts.
//
// Rate limit: 60 requests per 15-minute window, keyed on rl:studio:{callerID}.
// Fail-open: nil rdb or any Redis error allows the request through.
// The callerID is extracted from the JWT context by the handler — it is never
// supplied by the client directly.
func (s *Service) GetStudioAnalytics(ctx context.Context, callerID uuid.UUID, cursorStr string) (StudioPage, error) {
	// Rate limit — fail open.
	limited, err := s.checkRateLimit(ctx, callerID)
	if err != nil {
		// Redis error — allow the request (fail open).
		s.logger.Warn("studio: rate limit check failed — allowing request (fail open)",
			zap.String("caller_id", callerID.String()),
			zap.Error(err),
		)
	}
	if limited {
		return StudioPage{}, apierror.NewAPIError(apierror.CodeRateLimit, "Too many studio requests. Please try again later.")
	}

	// Decode cursor if provided.
	var cursor *post.FeedCursor
	if cursorStr != "" {
		cursor, err = post.DecodeCursor(cursorStr)
		if err != nil {
			return StudioPage{}, apierror.NewAPIError(apierror.CodeValidation, "Invalid cursor value.")
		}
	}

	items, nextCursor, terminated, err := s.repo.ListPostAnalytics(ctx, callerID, cursor, maxStudioDepth)
	if err != nil {
		s.logger.Error("studio: list post analytics", zap.Error(err))
		return StudioPage{}, apierror.NewAPIError(apierror.CodeInternal, "An unexpected error occurred.")
	}

	return StudioPage{
		Items:      items,
		NextCursor: nextCursor,
		Terminated: terminated,
	}, nil
}

// checkRateLimit checks the sliding-window rate limit for the given callerID.
// Returns (true, nil)  when Redis explicitly signals the limit is exceeded.
// Returns (false, err) when Redis is unavailable or returns any other error.
// Returns (false, nil) when the request is within the allowed window.
//
// The fail-open contract: callers must treat a non-nil error as "allow".
func (s *Service) checkRateLimit(ctx context.Context, callerID uuid.UUID) (limited bool, err error) {
	if s.rdb == nil {
		// No Redis client — skip rate limiting (fail open).
		return false, nil
	}

	key := fmt.Sprintf("rl:studio:%s", callerID.String())

	// Initialize key with TTL only when it does not exist yet, then increment.
	// This mirrors the pattern used by reaction.Service (see reaction/service.go).
	if setErr := s.rdb.SetNX(ctx, key, 0, studioRateWindow).Err(); setErr != nil {
		return false, fmt.Errorf("studio: rate limit SetNX: %w", setErr)
	}

	count, incrErr := s.rdb.Incr(ctx, key).Result()
	if incrErr != nil {
		return false, fmt.Errorf("studio: rate limit Incr: %w", incrErr)
	}

	// Belt-and-suspenders: ensure TTL is always set when count reaches 1.
	if count == 1 {
		if expErr := s.rdb.Expire(ctx, key, studioRateWindow).Err(); expErr != nil {
			s.logger.Error("studio: rate limit Expire failed after count==1",
				zap.String("key", key),
				zap.Error(expErr),
			)
		}
	}

	return count > studioRateMax, nil
}
