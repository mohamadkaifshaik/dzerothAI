package reaction

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/notification"
)

const (
	reactRateMax    int64         = 120
	reactRateWindow time.Duration = 15 * time.Minute
)

// Service implements reaction business logic.
//
// *Service satisfies the ReactionChecker interface.
type Service struct {
	repo     *Repository
	notifier notification.NotificationPublisher
	rdb      *redis.Client // nullable — rate limiting fails open when nil
	logger   *zap.Logger
}

// NewService constructs a reaction Service.
// rdb may be nil; when nil, rate limiting is skipped (fail-open per OPEN-P4-3).
func NewService(
	repo *Repository,
	notifier notification.NotificationPublisher,
	rdb *redis.Client,
	logger *zap.Logger,
) *Service {
	return &Service{
		repo:     repo,
		notifier: notifier,
		rdb:      rdb,
		logger:   logger,
	}
}

// React records a like reaction from callerID on postID.
//
// Rate limit: 120 requests per 15-minute window, keyed on rl:react:{callerID}.
// Fail-open: if rdb is nil OR any Redis command returns an error the request
// is allowed through (OPEN-P4-3 approved behavior). Only an explicit
// limit-exceeded result returns CodeRateLimit.
//
// Idempotent: if the caller has already reacted, returns nil without re-notifying.
//
// Notification: if a new reaction was created AND callerID != postAuthorID, a
// notification.EventReaction event is published.
func (s *Service) React(ctx context.Context, callerID, postID uuid.UUID, postAuthorID uuid.UUID) error {
	// Rate limit — fail open.
	limited, err := s.checkRateLimit(ctx, callerID)
	if err != nil {
		// Redis error — allow the request (fail open).
		s.logger.Warn("reaction: rate limit check failed — allowing request (fail open)",
			zap.String("caller_id", callerID.String()),
			zap.Error(err),
		)
	}
	if limited {
		return apierror.NewAPIError(apierror.CodeRateLimit, "Too many reaction requests. Please try again later.")
	}

	created, err := s.repo.React(ctx, callerID, postID)
	if err != nil {
		s.logger.Error("reaction: react", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	// Idempotent: already reacted — no notification.
	if !created {
		return nil
	}

	// Publish notification only for a new reaction and only when the reactor
	// is not the post author (no self-notification).
	if callerID != postAuthorID && s.notifier != nil {
		postIDCopy := postID
		pubErr := s.notifier.Publish(ctx, notification.PublishEvent{
			RecipientID: postAuthorID,
			ActorID:     callerID,
			Event:       notification.EventReaction,
			PostID:      &postIDCopy,
		})
		if pubErr != nil {
			// Notification failure is non-fatal: log and continue.
			s.logger.Warn("reaction: publish notification failed",
				zap.String("caller_id", callerID.String()),
				zap.String("post_id", postID.String()),
				zap.Error(pubErr),
			)
		}
	}

	return nil
}

// Unreact removes callerID's reaction from postID.
//
// Rate limit: same key and window as React (fail-open).
// Idempotent: returns nil if the reaction did not exist.
// No notification is published on unreact.
func (s *Service) Unreact(ctx context.Context, callerID, postID uuid.UUID) error {
	// Rate limit — fail open.
	limited, err := s.checkRateLimit(ctx, callerID)
	if err != nil {
		s.logger.Warn("reaction: rate limit check failed — allowing request (fail open)",
			zap.String("caller_id", callerID.String()),
			zap.Error(err),
		)
	}
	if limited {
		return apierror.NewAPIError(apierror.CodeRateLimit, "Too many reaction requests. Please try again later.")
	}

	if err := s.repo.Unreact(ctx, callerID, postID); err != nil {
		s.logger.Error("reaction: unreact", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	return nil
}

// HasReacted returns true if userID has reacted to postID.
// Implements ReactionChecker.
func (s *Service) HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error) {
	reacted, err := s.repo.HasReacted(ctx, userID, postID)
	if err != nil {
		s.logger.Error("reaction: has reacted", zap.Error(err))
		return false, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return reacted, nil
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

	key := fmt.Sprintf("rl:react:%s", callerID.String())

	// Initialize key with TTL only when it does not exist yet, then increment.
	// This mirrors the pattern used by auth.RateLimitMiddleware (see middleware.go).
	if setErr := s.rdb.SetNX(ctx, key, 0, reactRateWindow).Err(); setErr != nil {
		return false, fmt.Errorf("reaction: rate limit SetNX: %w", setErr)
	}

	count, incrErr := s.rdb.Incr(ctx, key).Result()
	if incrErr != nil {
		return false, fmt.Errorf("reaction: rate limit Incr: %w", incrErr)
	}

	// Belt-and-suspenders: ensure TTL is always set when count reaches 1.
	if count == 1 {
		if expErr := s.rdb.Expire(ctx, key, reactRateWindow).Err(); expErr != nil {
			s.logger.Error("reaction: rate limit Expire failed after count==1",
				zap.String("key", key),
				zap.Error(expErr),
			)
		}
	}

	return count > reactRateMax, nil
}
