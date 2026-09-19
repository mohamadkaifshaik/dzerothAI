package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost      = 12
	refreshTokenTTL = 30 * 24 * time.Hour
)

// handlePattern enforces: 3–50 chars, alphanumeric + underscore + hyphen,
// no leading or trailing special characters.
var handlePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{1,48}[a-zA-Z0-9]$|^[a-zA-Z0-9]{3}$`)

// RegisterInput holds the fields required to create a new account.
type RegisterInput struct {
	Handle      string
	DisplayName string
	Email       string
	Password    string
}

// LoginInput holds credentials for authentication.
type LoginInput struct {
	Email    string
	Password string
}

// TokenPair is the access + refresh token pair returned to clients after
// successful register, login, or refresh operations.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Service implements auth business logic.
type Service struct {
	pool      *pgxpool.Pool
	jwtSecret []byte
	log       *zap.Logger
}

// NewService constructs an auth Service.
func NewService(pool *pgxpool.Pool, jwtSecret []byte, log *zap.Logger) *Service {
	return &Service{pool: pool, jwtSecret: jwtSecret, log: log}
}

// Register validates the input, checks for duplicate email/handle, hashes the password,
// creates the user and an initial session inside a single database transaction, and
// returns a TokenPair on success.
func (s *Service) Register(ctx context.Context, input RegisterInput) (*TokenPair, error) {
	if err := validateRegisterInput(input); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("auth: hash password: %w", err)
	}

	userID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: generate user id: %w", err)
	}

	user, err := CreateUser(ctx, s.pool, &UserCreateInput{
		ID:           userID,
		Handle:       input.Handle,
		DisplayName:  input.DisplayName,
		Email:        input.Email,
		PasswordHash: string(hash),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateEmail) || errors.Is(err, ErrDuplicateHandle) {
			return nil, err
		}
		return nil, fmt.Errorf("auth: register: %w", err)
	}

	return s.issueTokenPair(ctx, user.ID, nil, nil)
}

// Login validates credentials against the stored bcrypt hash, checks account suspension,
// creates a new session, and returns a TokenPair.
func (s *Service) Login(ctx context.Context, input LoginInput) (*TokenPair, error) {
	if strings.TrimSpace(input.Email) == "" || strings.TrimSpace(input.Password) == "" {
		return nil, &ValidationError{Message: "email and password are required"}
	}

	user, err := GetUserByEmail(ctx, s.pool, input.Email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Return a generic error to avoid leaking whether the email exists.
			return nil, &ValidationError{Message: "invalid credentials"}
		}
		return nil, fmt.Errorf("auth: login lookup: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, &ValidationError{Message: "invalid credentials"}
	}

	if user.IsSuspended {
		return nil, &SuspendedError{}
	}

	return s.issueTokenPair(ctx, user.ID, nil, nil)
}

// Refresh validates the raw refresh token, looks up its session, checks expiry and
// suspension, then atomically rotates the session to a new token pair.
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	if strings.TrimSpace(rawRefreshToken) == "" {
		return nil, &ValidationError{Message: "refresh_token is required"}
	}

	tokenHash, err := HashRefreshToken(rawRefreshToken)
	if err != nil {
		return nil, &ValidationError{Message: "invalid refresh token format"}
	}

	session, err := GetSessionByHash(ctx, s.pool, tokenHash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, &UnauthorizedError{Message: "refresh token not found or already rotated"}
		}
		return nil, fmt.Errorf("auth: refresh lookup: %w", err)
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, &UnauthorizedError{Message: "refresh token expired"}
	}

	// Check whether the user is suspended before issuing new tokens.
	userSuspended, err := isUserSuspended(ctx, s.pool, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth: refresh suspension check: %w", err)
	}
	if userSuspended {
		return nil, &SuspendedError{}
	}

	newRaw, newHash, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("auth: refresh generate token: %w", err)
	}

	newExpiry := time.Now().UTC().Add(refreshTokenTTL)
	if err := RotateSession(ctx, s.pool, session.ID, newHash, newExpiry); err != nil {
		return nil, fmt.Errorf("auth: rotate session: %w", err)
	}

	accessToken, err := GenerateAccessToken(session.UserID, s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRaw,
	}, nil
}

// Logout deletes the session row, immediately invalidating the refresh token.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	if err := DeleteSession(ctx, s.pool, sessionID); err != nil {
		return fmt.Errorf("auth: logout: %w", err)
	}
	return nil
}

// RevokeAllSessions removes every session row for the given user, immediately
// invalidating all active refresh tokens. This method satisfies the
// user.SessionRevoker interface, allowing the user package to trigger session
// revocation without a circular import.
func (s *Service) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	if err := DeleteAllUserSessions(ctx, s.pool, userID); err != nil {
		return fmt.Errorf("auth: revoke all sessions: %w", err)
	}
	return nil
}

// issueTokenPair generates an access token and a new refresh token, creates a session
// row, and returns the pair. ipAddress and userAgent are optional audit fields.
func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID, ipAddress, userAgent *string) (*TokenPair, error) {
	accessToken, err := GenerateAccessToken(userID, s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	rawRefresh, hashRefresh, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("auth: generate refresh token: %w", err)
	}

	sessionID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: generate session id: %w", err)
	}

	if err := CreateSession(ctx, s.pool, &SessionCreateInput{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: hashRefresh,
		ExpiresAt: time.Now().UTC().Add(refreshTokenTTL),
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}); err != nil {
		return nil, fmt.Errorf("auth: create session: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
	}, nil
}

// validateRegisterInput returns a ValidationError if any field is invalid.
func validateRegisterInput(input RegisterInput) error {
	var details []ValidationDetail

	handle := strings.TrimSpace(input.Handle)
	handleLen := utf8.RuneCountInString(handle)
	if handleLen < 3 || handleLen > 50 {
		details = append(details, ValidationDetail{Field: "handle", Message: "must be 3–50 characters"})
	} else if !handlePattern.MatchString(handle) {
		details = append(details, ValidationDetail{Field: "handle", Message: "may only contain letters, numbers, underscores, and hyphens, with no leading or trailing special characters"})
	}

	displayName := strings.TrimSpace(input.DisplayName)
	dnLen := utf8.RuneCountInString(displayName)
	if dnLen < 1 || dnLen > 50 {
		details = append(details, ValidationDetail{Field: "display_name", Message: "must be 1–50 characters"})
	}

	email := strings.TrimSpace(input.Email)
	if email == "" || !strings.Contains(email, "@") {
		details = append(details, ValidationDetail{Field: "email", Message: "must be a valid email address"})
	}

	if len(input.Password) < 8 {
		details = append(details, ValidationDetail{Field: "password", Message: "must be at least 8 characters"})
	}

	if len(details) > 0 {
		return &ValidationError{Message: "Request validation failed.", Details: details}
	}
	return nil
}

// isUserSuspended queries the suspended status of a user by ID.
func isUserSuspended(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) (bool, error) {
	const q = `SELECT is_suspended FROM users WHERE id = $1`
	var suspended bool
	err := pool.QueryRow(ctx, q, userID).Scan(&suspended)
	if err != nil {
		return false, fmt.Errorf("auth: is_suspended lookup: %w", err)
	}
	return suspended, nil
}

// --- domain errors ---

// ValidationDetail is a single field-level validation message.
type ValidationDetail struct {
	Field   string
	Message string
}

// ValidationError is returned when input validation fails.
type ValidationError struct {
	Message string
	Details []ValidationDetail
}

func (e *ValidationError) Error() string { return e.Message }

// UnauthorizedError is returned for invalid or expired tokens.
type UnauthorizedError struct {
	Message string
}

func (e *UnauthorizedError) Error() string { return e.Message }

// SuspendedError is returned when a suspended user attempts authentication.
type SuspendedError struct{}

func (e *SuspendedError) Error() string { return "account is suspended" }
