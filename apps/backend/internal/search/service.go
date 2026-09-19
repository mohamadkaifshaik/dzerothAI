package search

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// BlockProvider abstracts the block.Service methods required by search.Service.
// Using an interface prevents a circular import between search and block packages.
type BlockProvider interface {
	GetBlockedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// Service implements search business logic.
type Service struct {
	repo          *Repository
	blockProvider BlockProvider
	log           *zap.Logger
}

// NewService constructs a search Service.
func NewService(repo *Repository, blockProvider BlockProvider, log *zap.Logger) *Service {
	return &Service{
		repo:          repo,
		blockProvider: blockProvider,
		log:           log,
	}
}

// SearchPosts returns a cursor-paginated PostSearchPage for posts matching query.
//
// Validation: query must not be empty (CodeValidation error).
// If callerID is non-nil, blocked users' posts are excluded from results.
// If callerID is nil (unauthenticated), no block filtering is applied.
func (s *Service) SearchPosts(ctx context.Context, callerID *uuid.UUID, query string, cursor string) (PostSearchPage, error) {
	if query == "" {
		return PostSearchPage{}, apierror.NewAPIError(apierror.CodeValidation, "search query must not be empty")
	}

	var blockedIDs []uuid.UUID
	if callerID != nil {
		ids, err := s.blockProvider.GetBlockedIDs(ctx, *callerID)
		if err != nil {
			s.log.Error("search: get blocked ids for post search",
				zap.Stringer("caller_id", *callerID),
				zap.Error(err),
			)
			return PostSearchPage{}, fmt.Errorf("search: get blocked ids: %w", err)
		}
		blockedIDs = ids
	}

	page, err := s.repo.SearchPosts(ctx, query, cursor, blockedIDs)
	if err != nil {
		s.log.Error("search: search posts repository error",
			zap.String("query", query),
			zap.Error(err),
		)
		return PostSearchPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	return page, nil
}

// SearchUsers returns a cursor-paginated UserSearchPage for users matching query.
//
// Validation: query must not be empty (CodeValidation error).
// If callerID is non-nil, blocked users are excluded from results.
// If callerID is nil (unauthenticated), no block filtering is applied.
func (s *Service) SearchUsers(ctx context.Context, callerID *uuid.UUID, query string, cursor string) (UserSearchPage, error) {
	if query == "" {
		return UserSearchPage{}, apierror.NewAPIError(apierror.CodeValidation, "search query must not be empty")
	}

	var blockedIDs []uuid.UUID
	if callerID != nil {
		ids, err := s.blockProvider.GetBlockedIDs(ctx, *callerID)
		if err != nil {
			s.log.Error("search: get blocked ids for user search",
				zap.Stringer("caller_id", *callerID),
				zap.Error(err),
			)
			return UserSearchPage{}, fmt.Errorf("search: get blocked ids: %w", err)
		}
		blockedIDs = ids
	}

	page, err := s.repo.SearchUsers(ctx, query, cursor, blockedIDs)
	if err != nil {
		s.log.Error("search: search users repository error",
			zap.String("query", query),
			zap.Error(err),
		)
		return UserSearchPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	return page, nil
}
