package bookmark

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// maxBookmarkDepth is the server-enforced maximum number of bookmarks returned
// in a single paginated response (CLAUDE.md §2.1, no infinite scrolling).
const maxBookmarkDepth = 200

// Service implements bookmark business logic.
type Service struct {
	repo *Repository
	log  *zap.Logger
}

// NewService constructs a bookmark Service.
func NewService(repo *Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Bookmark adds a bookmark for the caller on the given post.
// Returns CodeNotFound if the post does not exist or is soft-deleted.
func (s *Service) Bookmark(ctx context.Context, callerID, postID uuid.UUID) error {
	if err := s.repo.Add(ctx, callerID, postID); err != nil {
		var apiErr *apierror.APIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		s.log.Error("bookmark: add", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// Unbookmark removes a bookmark. Returns nil if the bookmark did not exist.
func (s *Service) Unbookmark(ctx context.Context, callerID, postID uuid.UUID) error {
	if err := s.repo.Remove(ctx, callerID, postID); err != nil {
		s.log.Error("bookmark: remove", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// ListBookmarks returns the caller's paginated bookmark list.
// Decodes the cursor, queries the repository, and maps rows to BookmarkDTO.
func (s *Service) ListBookmarks(ctx context.Context, callerID uuid.UUID, cursorStr string) (BookmarkPage, error) {
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return BookmarkPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	items, nextCursor, terminated, err := s.repo.ListByUser(ctx, callerID, cursor, maxBookmarkDepth)
	if err != nil {
		s.log.Error("bookmark: list by user", zap.Error(err))
		return BookmarkPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	dtos := make([]BookmarkDTO, len(items))
	for i, bwp := range items {
		dtos[i] = BookmarkDTO{
			PostID:    bwp.PostID.String(),
			CreatedAt: bwp.BookmarkCreatedAt.UTC().Format(time.RFC3339),
			Post:      post.ToDTO(bwp.Post),
		}
	}

	// Guarantee non-nil slice for consistent JSON serialization.
	if len(dtos) == 0 {
		dtos = []BookmarkDTO{}
	}

	return BookmarkPage{
		Items:      dtos,
		NextCursor: nextCursor,
		Terminated: terminated,
	}, nil
}
