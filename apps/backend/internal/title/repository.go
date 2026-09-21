package title

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when the requested title row does not exist.
var ErrNotFound = errors.New("title: not found")

// ErrForbidden is returned when an ownership or status precondition is not met.
// Callers (handlers) must map this to HTTP 403, not 404, so that the client
// cannot enumerate other users' title IDs.
var ErrForbidden = errors.New("title: forbidden")

// Repository provides data-access methods for the title domain.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a title Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetDefinitions returns all active title definitions ordered by created_at ASC.
// Definitions with is_active=false (e.g. top_1pct_creator) are excluded.
func (r *Repository) GetDefinitions(ctx context.Context) ([]TitleDefinition, error) {
	const q = `
		SELECT id, slug, display_name, description, category,
		       is_revocable, is_active, created_at, updated_at
		FROM title_definitions
		WHERE is_active = TRUE
		ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("title: get definitions query: %w", err)
	}
	defer rows.Close()

	var defs []TitleDefinition
	for rows.Next() {
		d, err := scanDefinition(rows)
		if err != nil {
			return nil, fmt.Errorf("title: get definitions scan: %w", err)
		}
		defs = append(defs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("title: get definitions rows: %w", err)
	}
	return defs, nil
}

// GetDefinitionBySlug returns a single title definition by its slug.
// Returns ErrNotFound if no row matches.
func (r *Repository) GetDefinitionBySlug(ctx context.Context, slug string) (*TitleDefinition, error) {
	const q = `
		SELECT id, slug, display_name, description, category,
		       is_revocable, is_active, created_at, updated_at
		FROM title_definitions
		WHERE slug = $1`

	row := r.pool.QueryRow(ctx, q, slug)
	d, err := scanDefinition(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("title: get definition by slug: %w", err)
	}
	return &d, nil
}

// GetUserTitles returns the active and grace_period user_titles rows for the
// given user, joined with their title definition for display fields.
// Revoked titles are excluded. Ordered by unlocked_at ASC (oldest first).
func (r *Repository) GetUserTitles(ctx context.Context, userID uuid.UUID) ([]UserTitleEntry, error) {
	const q = `
		SELECT
			ut.id, ut.user_id, ut.title_definition_id, ut.status,
			ut.grace_period_ends_at, ut.grace_notification_sent,
			ut.unlock_notification_sent, ut.revoked_at, ut.unlocked_at, ut.created_at,
			td.slug, td.display_name, td.category, td.is_revocable
		FROM user_titles ut
		JOIN title_definitions td ON td.id = ut.title_definition_id
		WHERE ut.user_id = $1
		  AND ut.status IN ('active', 'grace_period')
		ORDER BY ut.unlocked_at ASC`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("title: get user titles query: %w", err)
	}
	defer rows.Close()

	var result []UserTitleEntry
	for rows.Next() {
		row, err := scanUserTitleRow(rows)
		if err != nil {
			return nil, fmt.Errorf("title: get user titles scan: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("title: get user titles rows: %w", err)
	}
	return result, nil
}

// GetPrimaryTitle returns the TitleSummary for the user's currently selected
// primary title. Returns nil, nil when no primary title is set (primary_title_id IS NULL).
func (r *Repository) GetPrimaryTitle(ctx context.Context, userID uuid.UUID) (*TitleSummary, error) {
	const q = `
		SELECT ut.id, td.slug, td.display_name
		FROM users u
		JOIN user_titles ut ON ut.id = u.primary_title_id
		JOIN title_definitions td ON td.id = ut.title_definition_id
		WHERE u.id = $1`

	var s TitleSummary
	err := r.pool.QueryRow(ctx, q, userID).Scan(&s.ID, &s.Slug, &s.DisplayName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("title: get primary title: %w", err)
	}
	return &s, nil
}

// CreateUserTitle inserts a new user_titles row with status='active'.
// Idempotent: ON CONFLICT DO NOTHING on the partial unique index
// (user_id, title_definition_id) WHERE status IN ('active', 'grace_period').
// If the user already holds an active or grace_period instance of the definition,
// the existing row is returned without error.
func (r *Repository) CreateUserTitle(ctx context.Context, userID, definitionID uuid.UUID) (*UserTitle, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("title: create user title generate id: %w", err)
	}

	now := time.Now().UTC()
	const ins = `
		INSERT INTO user_titles
			(id, user_id, title_definition_id, status, unlocked_at, created_at)
		VALUES ($1, $2, $3, 'active', $4, $4)
		ON CONFLICT DO NOTHING`

	tag, err := r.pool.Exec(ctx, ins, id, userID, definitionID, now)
	if err != nil {
		return nil, fmt.Errorf("title: create user title insert: %w", err)
	}

	if tag.RowsAffected() == 1 {
		return r.getUserTitleByID(ctx, id)
	}

	// Conflict — return the existing active or grace_period row.
	return r.getActiveUserTitleByDefinition(ctx, userID, definitionID)
}

// UpdateUserTitleStatus applies a status transition or notification-flag update
// to the user_titles row identified by userTitleID. Status is always required;
// nil pointer fields in the update are preserved via SQL COALESCE (i.e. the
// existing column value is retained). Returns ErrNotFound if no row matches.
func (r *Repository) UpdateUserTitleStatus(ctx context.Context, userTitleID uuid.UUID, u TitleStatusUpdate) error {
	const q = `
		UPDATE user_titles
		SET
			status                   = $2,
			grace_period_ends_at     = COALESCE($3, grace_period_ends_at),
			revoked_at               = COALESCE($4, revoked_at),
			grace_notification_sent  = COALESCE($5, grace_notification_sent),
			unlock_notification_sent = COALESCE($6, unlock_notification_sent)
		WHERE id = $1`

	tag, err := r.pool.Exec(ctx, q,
		userTitleID,
		string(u.Status),
		u.GracePeriodEndsAt,
		u.RevokedAt,
		u.GraceNotificationSent,
		u.UnlockNotificationSent,
	)
	if err != nil {
		return fmt.Errorf("title: update user title status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPrimaryTitle sets users.primary_title_id = userTitleID for the given user.
// The update is conditional: the user_titles row must belong to the user and
// have status 'active' or 'grace_period'. Returns ErrForbidden if the row does
// not exist, is not owned by the user, or has an ineligible status.
// Uses DEFERRABLE INITIALLY DEFERRED FK — safe within a single statement.
func (r *Repository) SetPrimaryTitle(ctx context.Context, userID, userTitleID uuid.UUID) error {
	const q = `
		UPDATE users
		SET primary_title_id = $1
		WHERE id = $2
		  AND EXISTS (
		      SELECT 1
		      FROM user_titles
		      WHERE id = $1
		        AND user_id = $2
		        AND status IN ('active', 'grace_period')
		  )`

	tag, err := r.pool.Exec(ctx, q, userTitleID, userID)
	if err != nil {
		return fmt.Errorf("title: set primary title: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// ClearPrimaryTitle sets users.primary_title_id = NULL for the given user.
// Idempotent: no error if already NULL.
func (r *Repository) ClearPrimaryTitle(ctx context.Context, userID uuid.UUID) error {
	const q = `UPDATE users SET primary_title_id = NULL WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("title: clear primary title: %w", err)
	}
	return nil
}

// GetUserTitleForOwnershipCheck fetches a UserTitle row, verifying that it
// belongs to the given userID. Returns ErrNotFound if the row does not exist,
// ErrForbidden if the row exists but belongs to a different user. Used by
// handlers to validate ownership before delegating to service operations.
func (r *Repository) GetUserTitleForOwnershipCheck(ctx context.Context, userTitleID, userID uuid.UUID) (*UserTitle, error) {
	const q = `
		SELECT id, user_id, title_definition_id, status,
		       grace_period_ends_at, grace_notification_sent,
		       unlock_notification_sent, revoked_at, unlocked_at, created_at
		FROM user_titles
		WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, userTitleID)
	ut, err := scanUserTitle(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("title: ownership check: %w", err)
	}
	if ut.UserID != userID {
		return nil, ErrForbidden
	}
	return &ut, nil
}

// ---------------------------------------------------------------------------
// unexported helpers
// ---------------------------------------------------------------------------

// getUserTitleByID fetches a single user_titles row by primary key.
func (r *Repository) getUserTitleByID(ctx context.Context, id uuid.UUID) (*UserTitle, error) {
	const q = `
		SELECT id, user_id, title_definition_id, status,
		       grace_period_ends_at, grace_notification_sent,
		       unlock_notification_sent, revoked_at, unlocked_at, created_at
		FROM user_titles
		WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, id)
	ut, err := scanUserTitle(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("title: get user title by id: %w", err)
	}
	return &ut, nil
}

// getActiveUserTitleByDefinition fetches the active or grace_period user_titles
// row for the given (userID, definitionID) pair. Returns ErrNotFound if no
// such row exists.
func (r *Repository) getActiveUserTitleByDefinition(ctx context.Context, userID, definitionID uuid.UUID) (*UserTitle, error) {
	const q = `
		SELECT id, user_id, title_definition_id, status,
		       grace_period_ends_at, grace_notification_sent,
		       unlock_notification_sent, revoked_at, unlocked_at, created_at
		FROM user_titles
		WHERE user_id = $1
		  AND title_definition_id = $2
		  AND status IN ('active', 'grace_period')
		LIMIT 1`

	row := r.pool.QueryRow(ctx, q, userID, definitionID)
	ut, err := scanUserTitle(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("title: get active user title by definition: %w", err)
	}
	return &ut, nil
}

// scanDefinition scans a single title_definitions row.
func scanDefinition(row pgx.Row) (TitleDefinition, error) {
	var d TitleDefinition
	err := row.Scan(
		&d.ID,
		&d.Slug,
		&d.DisplayName,
		&d.Description,
		&d.Category,
		&d.IsRevocable,
		&d.IsActive,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	return d, err
}

// scanUserTitle scans a single user_titles row (no joined fields).
func scanUserTitle(row pgx.Row) (UserTitle, error) {
	var ut UserTitle
	err := row.Scan(
		&ut.ID,
		&ut.UserID,
		&ut.TitleDefinitionID,
		&ut.Status,
		&ut.GracePeriodEndsAt,
		&ut.GraceNotificationSent,
		&ut.UnlockNotificationSent,
		&ut.RevokedAt,
		&ut.UnlockedAt,
		&ut.CreatedAt,
	)
	return ut, err
}

// scanUserTitleRow scans a user_titles JOIN title_definitions row as returned
// by GetUserTitles.
func scanUserTitleRow(rows pgx.Rows) (UserTitleEntry, error) {
	var r UserTitleEntry
	err := rows.Scan(
		&r.ID,
		&r.UserID,
		&r.TitleDefinitionID,
		&r.Status,
		&r.GracePeriodEndsAt,
		&r.GraceNotificationSent,
		&r.UnlockNotificationSent,
		&r.RevokedAt,
		&r.UnlockedAt,
		&r.CreatedAt,
		&r.Slug,
		&r.DisplayName,
		&r.Category,
		&r.IsRevocable,
	)
	return r, err
}
