package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a queried row does not exist.
var ErrNotFound = errors.New("auth: not found")

// ErrDuplicateEmail is returned when the email is already registered (case-insensitive).
var ErrDuplicateEmail = errors.New("auth: email already registered")

// ErrDuplicateHandle is returned when the handle is already taken (case-insensitive).
var ErrDuplicateHandle = errors.New("auth: handle already taken")

// UserRow is the internal representation of a users row used only within the auth package.
// It must not be serialized directly into API responses.
type UserRow struct {
	ID           uuid.UUID
	Handle       string
	DisplayName  string
	Email        string
	PasswordHash string
	IsSuspended  bool
}

// SessionRow is the internal representation of a sessions row.
type SessionRow struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	ExpiresAt  time.Time
	LastUsedAt time.Time
}

// UserCreateInput holds validated fields required to insert a new user row.
type UserCreateInput struct {
	ID           uuid.UUID
	Handle       string
	DisplayName  string
	Email        string
	PasswordHash string
}

// SessionCreateInput holds validated fields required to insert a new session row.
type SessionCreateInput struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	IPAddress *string
	UserAgent *string
}

// CreateUser inserts a new user row. Returns ErrDuplicateEmail or ErrDuplicateHandle on
// unique-constraint violations; wraps other database errors.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, input *UserCreateInput) (*UserRow, error) {
	const q = `
		INSERT INTO users (id, handle, display_name, email, password_hash)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, handle, display_name, email, password_hash, is_suspended`

	row := pool.QueryRow(ctx, q,
		input.ID,
		input.Handle,
		input.DisplayName,
		input.Email,
		input.PasswordHash,
	)

	var u UserRow
	if err := row.Scan(&u.ID, &u.Handle, &u.DisplayName, &u.Email, &u.PasswordHash, &u.IsSuspended); err != nil {
		if isUniqueViolation(err, "users_email_lower_idx") {
			return nil, ErrDuplicateEmail
		}
		if isUniqueViolation(err, "users_handle_lower_idx") {
			return nil, ErrDuplicateHandle
		}
		return nil, fmt.Errorf("auth: create user: %w", err)
	}
	return &u, nil
}

// GetUserByEmail fetches a user row by email (case-insensitive). Returns ErrNotFound if no row matches.
func GetUserByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*UserRow, error) {
	const q = `
		SELECT id, handle, display_name, email, password_hash, is_suspended
		FROM users
		WHERE lower(email) = lower($1)`

	var u UserRow
	err := pool.QueryRow(ctx, q, email).
		Scan(&u.ID, &u.Handle, &u.DisplayName, &u.Email, &u.PasswordHash, &u.IsSuspended)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("auth: get user by email: %w", err)
	}
	return &u, nil
}

// GetUserByHandle fetches a user row by handle (case-insensitive). Returns ErrNotFound if no row matches.
func GetUserByHandle(ctx context.Context, pool *pgxpool.Pool, handle string) (*UserRow, error) {
	const q = `
		SELECT id, handle, display_name, email, password_hash, is_suspended
		FROM users
		WHERE lower(handle) = lower($1)`

	var u UserRow
	err := pool.QueryRow(ctx, q, handle).
		Scan(&u.ID, &u.Handle, &u.DisplayName, &u.Email, &u.PasswordHash, &u.IsSuspended)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("auth: get user by handle: %w", err)
	}
	return &u, nil
}

// CreateSession inserts a new session row.
func CreateSession(ctx context.Context, pool *pgxpool.Pool, input *SessionCreateInput) error {
	const q = `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := pool.Exec(ctx, q,
		input.ID,
		input.UserID,
		input.TokenHash,
		input.ExpiresAt,
		input.IPAddress,
		input.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("auth: create session: %w", err)
	}
	return nil
}

// GetSessionByHash looks up a session by the SHA-256 hash of the refresh token.
// Returns ErrNotFound if no matching session exists.
func GetSessionByHash(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (*SessionRow, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, last_used_at
		FROM sessions
		WHERE token_hash = $1`

	var s SessionRow
	err := pool.QueryRow(ctx, q, tokenHash).
		Scan(&s.ID, &s.UserID, &s.TokenHash, &s.ExpiresAt, &s.LastUsedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("auth: get session by hash: %w", err)
	}
	return &s, nil
}

// RotateSession atomically replaces the token_hash and expires_at on an existing session
// row. The operation runs inside a single transaction to prevent partial state.
func RotateSession(ctx context.Context, pool *pgxpool.Pool, oldID uuid.UUID, newHash string, newExpiresAt time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: rotate session begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		UPDATE sessions
		SET token_hash = $1, expires_at = $2, last_used_at = now()
		WHERE id = $3`

	tag, err := tx.Exec(ctx, q, newHash, newExpiresAt, oldID)
	if err != nil {
		return fmt.Errorf("auth: rotate session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("auth: rotate session commit: %w", err)
	}
	return nil
}

// DeleteSession removes a session row, effectively revoking the refresh token.
func DeleteSession(ctx context.Context, pool *pgxpool.Pool, sessionID uuid.UUID) error {
	const q = `DELETE FROM sessions WHERE id = $1`
	_, err := pool.Exec(ctx, q, sessionID)
	if err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

// DeleteSessionByID removes the session row identified by sessionID only when it
// belongs to userID. The user_id predicate is the server-side authorization check
// that prevents a user from revoking another user's session.
// Returns ErrNotFound when no matching row exists (session already deleted or
// belongs to a different user).
func DeleteSessionByID(ctx context.Context, pool *pgxpool.Pool, sessionID uuid.UUID, userID uuid.UUID) error {
	const q = `DELETE FROM sessions WHERE id = $1 AND user_id = $2`
	tag, err := pool.Exec(ctx, q, sessionID, userID)
	if err != nil {
		return fmt.Errorf("auth: delete session by id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateSessionLastUsed sets last_used_at = now() for the given session.
func UpdateSessionLastUsed(ctx context.Context, pool *pgxpool.Pool, sessionID uuid.UUID) error {
	const q = `UPDATE sessions SET last_used_at = now() WHERE id = $1`
	_, err := pool.Exec(ctx, q, sessionID)
	if err != nil {
		return fmt.Errorf("auth: update session last_used_at: %w", err)
	}
	return nil
}

// DeleteAllUserSessions removes all session rows for a user, revoking every active
// refresh token for that account. Used by the logout handler when a jti→session_id
// mapping is not yet persisted (Phase 1 conservative logout behavior).
func DeleteAllUserSessions(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) error {
	const q = `DELETE FROM sessions WHERE user_id = $1`
	_, err := pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("auth: delete all user sessions: %w", err)
	}
	return nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint violation
// whose error text references the given index name substring.
// PostgreSQL error code 23505 = unique_violation.
func isUniqueViolation(err error, indexNameSubstr string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") && strings.Contains(msg, indexNameSubstr)
}
