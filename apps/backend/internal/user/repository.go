package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when the requested user does not exist.
var ErrNotFound = errors.New("user: not found")

// UpdateInput holds the optional profile fields the owner may update.
// A nil pointer means "do not update this field".
type UpdateInput struct {
	DisplayName *string
	Bio         *string
	Location    *string
	WebsiteURL  *string
}

// GetByID fetches a user row by its UUID primary key.
// Returns ErrNotFound if no row matches.
func GetByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*User, error) {
	const q = `
		SELECT id, handle, display_name, email, email_verified,
		       bio, avatar_url, header_url, location, website_url,
		       is_private, is_suspended, created_at, updated_at
		FROM users
		WHERE id = $1`

	return scanUser(pool.QueryRow(ctx, q, id))
}

// GetByHandle fetches a user row by handle (case-insensitive).
// Returns ErrNotFound if no row matches.
func GetByHandle(ctx context.Context, pool *pgxpool.Pool, handle string) (*User, error) {
	const q = `
		SELECT id, handle, display_name, email, email_verified,
		       bio, avatar_url, header_url, location, website_url,
		       is_private, is_suspended, created_at, updated_at
		FROM users
		WHERE lower(handle) = lower($1)`

	return scanUser(pool.QueryRow(ctx, q, handle))
}

// Update applies the provided UpdateInput fields to the user row identified by id.
// Only non-nil fields are updated; nil fields are left unchanged.
// Returns the updated User on success.
func Update(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, input UpdateInput) (*User, error) {
	// Build a dynamic UPDATE statement to avoid overwriting fields that were not supplied.
	// We use positional parameters because pgx does not support named parameters in
	// Exec/QueryRow without additional libraries.
	args := []any{}
	setClauses := []string{}
	paramIdx := 1

	if input.DisplayName != nil {
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", paramIdx))
		args = append(args, *input.DisplayName)
		paramIdx++
	}
	if input.Bio != nil {
		setClauses = append(setClauses, fmt.Sprintf("bio = $%d", paramIdx))
		args = append(args, *input.Bio)
		paramIdx++
	}
	if input.Location != nil {
		setClauses = append(setClauses, fmt.Sprintf("location = $%d", paramIdx))
		args = append(args, *input.Location)
		paramIdx++
	}
	if input.WebsiteURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("website_url = $%d", paramIdx))
		args = append(args, *input.WebsiteURL)
		paramIdx++
	}

	if len(setClauses) == 0 {
		// Nothing to update; return the current row.
		return GetByID(ctx, pool, id)
	}

	// Always update updated_at.
	setClauses = append(setClauses, "updated_at = now()")

	args = append(args, id)
	idParam := fmt.Sprintf("$%d", paramIdx)

	q := fmt.Sprintf(`
		UPDATE users
		SET %s
		WHERE id = %s
		RETURNING id, handle, display_name, email, email_verified,
		          bio, avatar_url, header_url, location, website_url,
		          is_private, is_suspended, created_at, updated_at`,
		joinClauses(setClauses),
		idParam,
	)

	row := pool.QueryRow(ctx, q, args...)
	return scanUser(row)
}

// SuspendSelf sets is_suspended = TRUE for the given user.
// The operation is idempotent: if the account is already suspended the UPDATE
// affects zero rows but no error is returned.
func SuspendSelf(ctx context.Context, pool *pgxpool.Pool, callerID uuid.UUID) error {
	const q = `UPDATE users SET is_suspended = TRUE WHERE id = $1`
	_, err := pool.Exec(ctx, q, callerID)
	if err != nil {
		return fmt.Errorf("user: suspend self: %w", err)
	}
	return nil
}

// scanUser scans a single users row into a User struct.
func scanUser(row pgx.Row) (*User, error) {
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
		return nil, fmt.Errorf("user: scan: %w", err)
	}
	return &u, nil
}

// joinClauses joins SET clause fragments with ", ".
func joinClauses(clauses []string) string {
	result := ""
	for i, c := range clauses {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
}
