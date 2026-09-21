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

// GetUserCreatedAt returns the created_at timestamp for the given user.
// Returns ErrNotFound if no users row matches the given id.
func (r *Repository) GetUserCreatedAt(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	const q = `SELECT created_at FROM users WHERE id = $1`
	var t time.Time
	err := r.pool.QueryRow(ctx, q, userID).Scan(&t)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, ErrNotFound
		}
		return time.Time{}, fmt.Errorf("title: get user created_at: %w", err)
	}
	return t, nil
}

// CountCenturionPosts returns the lifetime count of qualifying posts
// (post_type IN ('original','reply','quote') AND is_deleted=FALSE) authored
// by userID. Reposts are excluded per the Centurion qualification rule.
func (r *Repository) CountCenturionPosts(ctx context.Context, userID uuid.UUID) (int64, error) {
	const q = `
		SELECT COUNT(*)
		FROM posts
		WHERE author_id = $1
		  AND is_deleted = FALSE
		  AND post_type IN ('original', 'reply', 'quote')`
	var n int64
	if err := r.pool.QueryRow(ctx, q, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("title: count centurion posts: %w", err)
	}
	return n, nil
}

// GetTrendsetterLikes returns the count of 'like' reactions received on the
// user's non-deleted posts within the rolling 30-day window.
func (r *Repository) GetTrendsetterLikes(ctx context.Context, userID uuid.UUID) (int64, error) {
	const q = `
		SELECT COUNT(*)
		FROM reactions r
		JOIN posts p ON p.id = r.post_id
		WHERE p.author_id = $1
		  AND p.is_deleted = FALSE
		  AND r.reaction_type = 'like'
		  AND r.created_at >= now() - interval '30 days'`
	var n int64
	if err := r.pool.QueryRow(ctx, q, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("title: get trendsetter likes: %w", err)
	}
	return n, nil
}

// GetNicheMetrics returns the count of distinct qualifying posts and their
// associated like count for the given user and tag set, over the rolling
// 30-day window.
//
// The CTE ensures a post tagged with multiple qualifying tags (e.g. both
// #tech and #gadgets) is counted exactly once (DISTINCT post id) rather than
// once per matching tag, avoiding a multiplication bug.
func (r *Repository) GetNicheMetrics(ctx context.Context, userID uuid.UUID, tags []string) (posts int64, likes int64, err error) {
	const q = `
		WITH qualifying_posts AS (
		    SELECT DISTINCT p.id
		    FROM posts p
		    JOIN post_hashtags ph ON ph.post_id = p.id
		    WHERE p.author_id = $1
		      AND p.is_deleted = FALSE
		      AND p.post_type IN ('original', 'reply', 'quote')
		      AND ph.tag = ANY($2::text[])
		      AND p.created_at >= now() - interval '30 days'
		)
		SELECT
		    (SELECT COUNT(*) FROM qualifying_posts) AS niche_posts_30d,
		    (
		        SELECT COUNT(*)
		        FROM reactions r
		        WHERE r.post_id IN (SELECT id FROM qualifying_posts)
		          AND r.reaction_type = 'like'
		          AND r.created_at >= now() - interval '30 days'
		    ) AS niche_likes_30d`
	if err = r.pool.QueryRow(ctx, q, userID, tags).Scan(&posts, &likes); err != nil {
		return 0, 0, fmt.Errorf("title: get niche metrics: %w", err)
	}
	return posts, likes, nil
}

// GetUserIDsBatch returns up to limit user IDs with id > afterID, ordered by
// id ASC. Pass uuid.Nil as afterID to start from the beginning. Used by the
// TitleQualificationWorker to iterate all users in deterministic cursor batches
// without loading all user IDs into memory at once.
func (r *Repository) GetUserIDsBatch(ctx context.Context, afterID uuid.UUID, limit int) ([]uuid.UUID, error) {
	const q = `SELECT id FROM users WHERE id > $1 ORDER BY id ASC LIMIT $2`
	rows, err := r.pool.Query(ctx, q, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("title: get user ids batch query: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("title: get user ids batch scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("title: get user ids batch rows: %w", err)
	}
	return ids, nil
}

// EnterGracePeriod transitions a user_titles row from active → grace_period.
// Returns ErrNotFound if no active row with the given ID exists (the WHERE
// status='active' guard prevents accidental double-application).
func (r *Repository) EnterGracePeriod(ctx context.Context, userTitleID uuid.UUID, endsAt time.Time) error {
	const q = `
		UPDATE user_titles
		SET status = 'grace_period', grace_period_ends_at = $2
		WHERE id = $1 AND status = 'active'`

	tag, err := r.pool.Exec(ctx, q, userTitleID, endsAt)
	if err != nil {
		return fmt.Errorf("title: enter grace period: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RestoreGracePeriodTitle transitions a user_titles row from grace_period → active.
// Returns ErrNotFound if no grace_period row with the given ID exists.
func (r *Repository) RestoreGracePeriodTitle(ctx context.Context, userTitleID uuid.UUID) error {
	const q = `
		UPDATE user_titles
		SET status = 'active', grace_period_ends_at = NULL
		WHERE id = $1 AND status = 'grace_period'`

	tag, err := r.pool.Exec(ctx, q, userTitleID)
	if err != nil {
		return fmt.Errorf("title: restore grace period title: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeUserTitleTx atomically revokes a user_titles row and conditionally
// clears users.primary_title_id if it currently references that exact row.
// The second UPDATE is conditional — it only sets primary_title_id=NULL when
// the user's current primary matches userTitleID, leaving other primary titles
// untouched.
func (r *Repository) RevokeUserTitleTx(ctx context.Context, userTitleID, userID uuid.UUID, revokedAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("title: revoke user title begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const revokeQ = `UPDATE user_titles SET status = 'revoked', revoked_at = $2 WHERE id = $1`
	tag, err := tx.Exec(ctx, revokeQ, userTitleID, revokedAt)
	if err != nil {
		return fmt.Errorf("title: revoke user title update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	// Conditionally clear primary_title_id only when it points at the revoked row.
	const clearPrimaryQ = `
		UPDATE users
		SET primary_title_id = NULL
		WHERE id = $1 AND primary_title_id = $2`
	if _, err := tx.Exec(ctx, clearPrimaryQ, userID, userTitleID); err != nil {
		return fmt.Errorf("title: revoke user title clear primary: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("title: revoke user title commit: %w", err)
	}
	return nil
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
