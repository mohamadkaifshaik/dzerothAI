package title

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/ctxlog"
)

// followChecker is satisfied by *follow.Service via SetFollowChecker in main.go.
// Defined locally to avoid an import cycle between internal/title and internal/follow.
type followChecker interface {
	IsFollowing(ctx context.Context, followerID, followedID uuid.UUID) (bool, error)
}

// userPrivacyChecker is satisfied by *titleUserPrivacyAdapter in cmd/api/main.go.
// Defined locally to avoid an import cycle between internal/title and internal/user.
type userPrivacyChecker interface {
	IsPrivateAccount(ctx context.Context, userID uuid.UUID) (bool, error)
}

// Service implements the title HTTP API business logic.
type Service struct {
	repo           *Repository
	followChecker  followChecker
	privacyChecker userPrivacyChecker
	log            *zap.Logger
}

// NewService constructs a title Service. SetFollowChecker and SetPrivacyChecker
// must be called before the HTTP server starts if privacy-aware endpoints are needed.
func NewService(repo *Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// SetFollowChecker injects a followChecker dependency. Must be called after
// NewService and before the first request.
func (s *Service) SetFollowChecker(fc followChecker) { s.followChecker = fc }

// SetPrivacyChecker injects a userPrivacyChecker dependency. Must be called after
// NewService and before the first request.
func (s *Service) SetPrivacyChecker(pc userPrivacyChecker) { s.privacyChecker = pc }

// GetCatalog returns all active title definitions as a TitleCatalogResponse.
// The Items slice is never nil — an empty slice is returned when no definitions exist.
func (s *Service) GetCatalog(ctx context.Context) (TitleCatalogResponse, error) {
	defs, err := s.repo.GetDefinitions(ctx)
	if err != nil {
		s.log.Error("title: get catalog", ctxlog.RequestIDField(ctx), zap.Error(err))
		return TitleCatalogResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	items := make([]TitleDefinitionDTO, 0, len(defs))
	for _, d := range defs {
		desc := ""
		if d.Description != nil {
			desc = *d.Description
		}
		items = append(items, TitleDefinitionDTO{
			ID:          d.ID.String(),
			Slug:        d.Slug,
			DisplayName: d.DisplayName,
			Description: desc,
			Category:    string(d.Category),
			IsRevocable: d.IsRevocable,
		})
	}
	return TitleCatalogResponse{Items: items}, nil
}

// GetMyTitles returns the caller's active+grace_period titles and primary title ID.
func (s *Service) GetMyTitles(ctx context.Context, callerID uuid.UUID) (UserTitlesResponse, error) {
	entries, err := s.repo.GetUserTitles(ctx, callerID)
	if err != nil {
		s.log.Error("title: get my titles", ctxlog.RequestIDField(ctx), zap.Error(err))
		return UserTitlesResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	items := make([]UserTitleDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, UserTitleDTO{
			ID:          e.ID.String(),
			Slug:        e.Slug,
			DisplayName: e.DisplayName,
			Category:    string(e.Category),
			IsRevocable: e.IsRevocable,
			Status:      string(e.Status),
			UnlockedAt:  e.UnlockedAt.UTC().Format(time.RFC3339),
		})
	}

	primary, err := s.repo.GetPrimaryTitle(ctx, callerID)
	if err != nil {
		s.log.Error("title: get my titles primary", ctxlog.RequestIDField(ctx), zap.Error(err))
		return UserTitlesResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	var primaryID *string
	if primary != nil {
		idStr := primary.ID.String()
		primaryID = &idStr
	}

	return UserTitlesResponse{Items: items, PrimaryID: primaryID}, nil
}

// GetMyPrimaryTitle returns the caller's primary title (nil PrimaryTitle if none set).
func (s *Service) GetMyPrimaryTitle(ctx context.Context, callerID uuid.UUID) (PrimaryTitleResponse, error) {
	primary, err := s.repo.GetPrimaryTitle(ctx, callerID)
	if err != nil {
		s.log.Error("title: get my primary title", ctxlog.RequestIDField(ctx), zap.Error(err))
		return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return PrimaryTitleResponse{PrimaryTitle: summaryToDTO(primary)}, nil
}

// SetMyPrimaryTitle sets the caller's primary title to the given userTitleID.
// Returns CodeForbidden if the title doesn't belong to the caller or isn't active/grace_period.
func (s *Service) SetMyPrimaryTitle(ctx context.Context, callerID uuid.UUID, userTitleID uuid.UUID) (PrimaryTitleResponse, error) {
	err := s.repo.SetPrimaryTitle(ctx, callerID, userTitleID)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeForbidden, "title not eligible or not owned by you")
		}
		s.log.Error("title: set my primary title", ctxlog.RequestIDField(ctx), zap.Error(err))
		return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}

	primary, err := s.repo.GetPrimaryTitle(ctx, callerID)
	if err != nil {
		s.log.Error("title: set my primary title get primary", ctxlog.RequestIDField(ctx), zap.Error(err))
		return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return PrimaryTitleResponse{PrimaryTitle: summaryToDTO(primary)}, nil
}

// ClearMyPrimaryTitle clears the caller's primary title. Idempotent.
func (s *Service) ClearMyPrimaryTitle(ctx context.Context, callerID uuid.UUID) error {
	if err := s.repo.ClearPrimaryTitle(ctx, callerID); err != nil {
		s.log.Error("title: clear my primary title", ctxlog.RequestIDField(ctx), zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// GetUserPrimaryTitle returns the primary title of the target user, respecting privacy.
//
// Privacy rules:
//   - Owner calling (callerID == targetID): always return title.
//   - Public account: return title.
//   - Private account + unauthenticated caller: return PrimaryTitleResponse{nil}.
//   - Private account + authenticated follower: return title.
//   - Private account + authenticated non-follower: return PrimaryTitleResponse{nil}.
//   - If privacyChecker is nil: treat all accounts as public (safe default).
func (s *Service) GetUserPrimaryTitle(ctx context.Context, callerID *uuid.UUID, targetID uuid.UUID) (PrimaryTitleResponse, error) {
	// Owner always sees their own primary title.
	if callerID != nil && *callerID == targetID {
		return s.fetchPrimary(ctx, targetID)
	}

	// Check privacy when a privacy checker is wired.
	if s.privacyChecker != nil {
		isPrivate, err := s.privacyChecker.IsPrivateAccount(ctx, targetID)
		if err != nil {
			// Fail open: treat as public on privacy-check errors to avoid silently
			// hiding public content. Log at warn so it is observable.
			s.log.Warn("title: get user primary title privacy check failed — treating as public",
				ctxlog.RequestIDField(ctx),
				zap.String("target_id", targetID.String()),
				zap.Error(err),
			)
			return s.fetchPrimary(ctx, targetID)
		}

		if isPrivate {
			// Unauthenticated: cannot see private account title.
			if callerID == nil {
				return PrimaryTitleResponse{PrimaryTitle: nil}, nil
			}

			// Authenticated: check follow relationship.
			if s.followChecker != nil {
				following, err := s.followChecker.IsFollowing(ctx, *callerID, targetID)
				if err != nil {
					s.log.Error("title: get user primary title follow check",
						ctxlog.RequestIDField(ctx),
						zap.Error(err),
					)
					return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
				}
				if !following {
					return PrimaryTitleResponse{PrimaryTitle: nil}, nil
				}
			} else {
				// No follow checker wired and account is private — deny.
				return PrimaryTitleResponse{PrimaryTitle: nil}, nil
			}
		}
	}

	return s.fetchPrimary(ctx, targetID)
}

// fetchPrimary retrieves and maps the primary title for targetID.
func (s *Service) fetchPrimary(ctx context.Context, targetID uuid.UUID) (PrimaryTitleResponse, error) {
	primary, err := s.repo.GetPrimaryTitle(ctx, targetID)
	if err != nil {
		s.log.Error("title: fetch primary", ctxlog.RequestIDField(ctx), zap.Error(err))
		return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return PrimaryTitleResponse{PrimaryTitle: summaryToDTO(primary)}, nil
}

// summaryToDTO maps a *TitleSummary to a *TitleSummaryDTO.
// Returns nil when s is nil.
func summaryToDTO(s *TitleSummary) *TitleSummaryDTO {
	if s == nil {
		return nil
	}
	return &TitleSummaryDTO{Slug: s.Slug, DisplayName: s.DisplayName}
}
