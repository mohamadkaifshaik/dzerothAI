package feed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/ctxlog"
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// maxHomeFeedDepth is the server-enforced hard limit on home feed depth.
// The client must not request more than this and must stop when terminated=true
// is received (CLAUDE.md §2.1 — no infinite scrolling).
const maxHomeFeedDepth = 200

// FeedService is the interface for home timeline feed retrieval.
type FeedService interface {
	GetHomeFeed(ctx context.Context, callerID uuid.UUID, cursorStr string) (post.PostPage, error)
}

// BlockProvider abstracts the block.Service methods required by feed.Service.
// Using an interface prevents a circular import between feed and block packages.
type BlockProvider interface {
	GetBlockedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	GetMutedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// FollowProvider abstracts the follow.Service / follow.Repository methods
// required by feed.Service. Using an interface prevents a circular import.
type FollowProvider interface {
	GetFollowedIDs(ctx context.Context, followerID uuid.UUID) ([]uuid.UUID, error)
}

// Service implements home timeline feed business logic.
type Service struct {
	repo           *Repository
	blockProvider  BlockProvider
	followProvider FollowProvider
	log            *zap.Logger
	events         *platformMetrics.Events
}

// NewService constructs a feed Service.
func NewService(repo *Repository, bp BlockProvider, fp FollowProvider, log *zap.Logger) *Service {
	return &Service{
		repo:           repo,
		blockProvider:  bp,
		followProvider: fp,
		log:            log,
	}
}

// SetEvents injects the Prometheus event counters into the Service.
// Passing nil disables counter instrumentation (no-op, safe for unit tests).
func (s *Service) SetEvents(e *platformMetrics.Events) {
	s.events = e
}

// GetHomeFeed returns the home timeline PostPage for the authenticated caller.
//
// Algorithm:
//  1. Decode cursor (CodeValidation on bad cursor string).
//  2. Fetch IDs blocked by the caller.
//  3. Fetch IDs muted by the caller.
//  4. NOTE: Bidirectional block exclusion is naturally satisfied at the SQL
//     level because block.Repository.Block removes follows in both directions.
//     Users who have blocked the caller therefore never appear in the follows
//     subquery that populates the home feed.
//  5. Query the repository for the paginated home timeline.
//  6. Convert []post.Post to []post.PostDTO (no public metrics — CLAUDE.md §2.3).
//  7. Return PostPage. When callerID follows no one the result is
//     PostPage{Items:[], Terminated:true}, which is a valid finite terminal state.
func (s *Service) GetHomeFeed(ctx context.Context, callerID uuid.UUID, cursorStr string) (post.PostPage, error) {
	// Step 1: decode optional cursor.
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return post.PostPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	// Step 2: fetch IDs that the caller has blocked.
	blockedIDs, err := s.blockProvider.GetBlockedIDs(ctx, callerID)
	if err != nil {
		s.log.Error("feed: get blocked ids", ctxlog.RequestIDField(ctx), zap.Stringer("caller_id", callerID), zap.Error(err))
		return post.PostPage{}, fmt.Errorf("feed: get blocked ids: %w", err)
	}

	// Step 3: fetch IDs that the caller has muted.
	mutedIDs, err := s.blockProvider.GetMutedIDs(ctx, callerID)
	if err != nil {
		s.log.Error("feed: get muted ids", ctxlog.RequestIDField(ctx), zap.Stringer("caller_id", callerID), zap.Error(err))
		return post.PostPage{}, fmt.Errorf("feed: get muted ids: %w", err)
	}

	// Step 5: query the repository.
	posts, nextCursor, terminated, err := s.repo.ListHomeTimeline(
		ctx, callerID, blockedIDs, mutedIDs, cursor, maxHomeFeedDepth,
	)
	if err != nil {
		s.log.Error("feed: list home timeline", ctxlog.RequestIDField(ctx), zap.Stringer("caller_id", callerID), zap.Error(err))
		return post.PostPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	// Step 6: map to public DTOs. post.ToDTO never includes social-validation
	// metrics (CLAUDE.md §2.3).
	dtos := make([]post.PostDTO, len(posts))
	for i, p := range posts {
		dtos[i] = post.ToDTO(p)
	}

	// Guarantee a non-nil slice in the JSON response for empty feeds.
	if dtos == nil {
		dtos = []post.PostDTO{}
	}

	if terminated {
		s.events.RecordFeedTermination(platformMetrics.FeedTypeHome)
	}

	return post.PostPage{Items: dtos, NextCursor: nextCursor, Terminated: terminated}, nil
}
