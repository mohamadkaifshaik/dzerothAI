package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// UpdateProfileInput holds the updatable profile fields from a client request.
// All fields are optional. A nil pointer means "do not update".
type UpdateProfileInput struct {
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	Location    *string `json:"location"`
	WebsiteURL  *string `json:"website_url"`
}

// SessionRevoker is satisfied by the auth repository or service.
// Defined here to avoid a circular import between the user and auth packages.
type SessionRevoker interface {
	RevokeAllSessions(ctx context.Context, userID uuid.UUID) error
}

// Service implements user profile business logic.
type Service struct {
	pool           *pgxpool.Pool
	log            *zap.Logger
	sessionRevoker SessionRevoker
}

// NewService constructs a user Service.
func NewService(pool *pgxpool.Pool, log *zap.Logger) *Service {
	return &Service{pool: pool, log: log}
}

// SetSessionRevoker injects a SessionRevoker. Must be called after NewService
// and before the first request is served when session revocation on self-suspension
// is desired.
func (s *Service) SetSessionRevoker(sr SessionRevoker) {
	s.sessionRevoker = sr
}

// UserExists returns true when a user row with the given ID exists.
// Satisfies the report.UserChecker interface.
// Any repository error is treated as "not found" (returns false, nil).
func (s *Service) UserExists(ctx context.Context, userID uuid.UUID) (bool, error) {
	_, err := GetByID(ctx, s.pool, userID)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// GetProfile retrieves a user by ID. Returns ErrNotFound if the user does not exist.
func (s *Service) GetProfile(ctx context.Context, id uuid.UUID) (*User, error) {
	u, err := GetByID(ctx, s.pool, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user: get profile: %w", err)
	}
	return u, nil
}

// GetProfileByHandle retrieves a user by handle. Returns ErrNotFound if the user does not exist.
func (s *Service) GetProfileByHandle(ctx context.Context, handle string) (*User, error) {
	u, err := GetByHandle(ctx, s.pool, handle)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user: get profile by handle: %w", err)
	}
	return u, nil
}

// UpdateProfile validates the input, enforces that only the account owner may update,
// and applies the changes. Returns the updated User on success.
func (s *Service) UpdateProfile(ctx context.Context, callerID uuid.UUID, input UpdateProfileInput) (*User, error) {
	if err := validateUpdateProfileInput(input); err != nil {
		return nil, err
	}

	repoInput := UpdateInput{
		Bio:        input.Bio,
		Location:   input.Location,
		WebsiteURL: input.WebsiteURL,
	}
	if input.DisplayName != nil {
		repoInput.DisplayName = input.DisplayName
	}

	u, err := Update(ctx, s.pool, callerID, repoInput)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user: update profile: %w", err)
	}
	return u, nil
}

// UpdateSettings updates the is_private flag for the authenticated user.
func (s *Service) UpdateSettings(ctx context.Context, callerID uuid.UUID, isPrivate bool) (*User, error) {
	const q = `
		UPDATE users
		SET is_private = $1, updated_at = now()
		WHERE id = $2
		RETURNING id, handle, display_name, email, email_verified,
		          bio, avatar_url, header_url, location, website_url,
		          is_private, is_suspended, created_at, updated_at`

	row := s.pool.QueryRow(ctx, q, isPrivate, callerID)
	u, err := scanUserRow(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user: update settings: %w", err)
	}
	return u, nil
}

// SuspendSelf sets is_suspended = TRUE for the authenticated caller.
// The operation is idempotent: suspending an already-suspended account returns nil.
// After the database write succeeds, all sessions are revoked via the configured
// SessionRevoker. Session revocation is best-effort: a failure is logged at WARN
// but does NOT cause SuspendSelf to return an error, because the account is
// already suspended and new logins are blocked.
func (s *Service) SuspendSelf(ctx context.Context, callerID uuid.UUID) error {
	if err := SuspendSelf(ctx, s.pool, callerID); err != nil {
		return fmt.Errorf("user: suspend self: %w", err)
	}

	if s.sessionRevoker != nil {
		if err := s.sessionRevoker.RevokeAllSessions(ctx, callerID); err != nil {
			s.log.Warn("user: suspend self: session revocation failed (best-effort)",
				zap.String("user_id", callerID.String()),
				zap.Error(err),
			)
		}
	}

	return nil
}

// ProfileValidationError is returned when profile update input fails validation.
type ProfileValidationError struct {
	Details []ProfileValidationDetail
}

// ProfileValidationDetail holds a single field-level validation message.
type ProfileValidationDetail struct {
	Field   string
	Message string
}

func (e *ProfileValidationError) Error() string { return "profile validation failed" }

func validateUpdateProfileInput(input UpdateProfileInput) error {
	var details []ProfileValidationDetail

	if input.DisplayName != nil {
		dn := strings.TrimSpace(*input.DisplayName)
		dnLen := utf8.RuneCountInString(dn)
		if dnLen < 1 || dnLen > 50 {
			details = append(details, ProfileValidationDetail{
				Field:   "display_name",
				Message: "must be 1–50 characters",
			})
		}
	}

	if input.Bio != nil {
		bioLen := utf8.RuneCountInString(*input.Bio)
		if bioLen > 160 {
			details = append(details, ProfileValidationDetail{
				Field:   "bio",
				Message: "must be 160 characters or fewer",
			})
		}
	}

	if input.Location != nil {
		locLen := utf8.RuneCountInString(*input.Location)
		if locLen > 100 {
			details = append(details, ProfileValidationDetail{
				Field:   "location",
				Message: "must be 100 characters or fewer",
			})
		}
	}

	if input.WebsiteURL != nil {
		urlLen := utf8.RuneCountInString(*input.WebsiteURL)
		if urlLen > 2048 {
			details = append(details, ProfileValidationDetail{
				Field:   "website_url",
				Message: "must be 2048 characters or fewer",
			})
		}
	}

	if len(details) > 0 {
		return &ProfileValidationError{Details: details}
	}
	return nil
}

// scanUserRow scans a single users row. Used for inline UPDATE … RETURNING queries
// in the service layer that cannot go through the repository's unexported scanUser.
func scanUserRow(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID,
		&u.Handle,
		&u.DisplayName,
		&u.Email,
		&u.EmailVerified,
		&u.Bio,
		&u.AvatarURL,
		&u.HeaderURL,
		&u.Location,
		&u.WebsiteURL,
		&u.IsPrivate,
		&u.IsSuspended,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user: scan row: %w", err)
	}
	return &u, nil
}
