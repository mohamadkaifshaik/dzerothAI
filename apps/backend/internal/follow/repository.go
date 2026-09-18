package follow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// Repository provides data-access methods for the follow domain.
// It uses *pgxpool.Pool consistent with the post package convention.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a follow Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Follow inserts a follow row. Idempotent: ON CONFLICT DO NOTHING means a
// duplicate follow is silently ignored rather than returning an error.
func (r *Repository) Follow(ctx context.Context, followerID, followedID uuid.UUID) error {
	const q = `
		INSERT INTO follows (follower_id, followed_id, created_at)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING`

	if _, err := r.pool.Exec(ctx, q, followerID, followedID); err != nil {
		return fmt.Errorf("follow: insert: %w", err)
	}
	return nil
}

// Unfollow removes a follow row. Returns nil if the row did not exist — the
// caller treats a non-existent follow as a no-op.
func (r *Repository) Unfollow(ctx context.Context, followerID, followedID uuid.UUID) error {
	const q = `DELETE FROM follows WHERE follower_id = $1 AND followed_id = $2`

	if _, err := r.pool.Exec(ctx, q, followerID, followedID); err != nil {
		return fmt.Errorf("follow: delete: %w", err)
	}
	return nil
}

// IsFollowing returns true if followerID currently follows followedID.
func (r *Repository) IsFollowing(ctx context.Context, followerID, followedID uuid.UUID) (bool, error) {
	const q = `SELECT 1 FROM follows WHERE follower_id = $1 AND followed_id = $2`

	var dummy int
	err := r.pool.QueryRow(ctx, q, followerID, followedID).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("follow: is following: %w", err)
	}
	return true, nil
}

// IsBlockedBy returns true if targetID has blocked callerID — i.e., callerID
// cannot follow or view targetID's content. Checks blocks WHERE
// blocker_id = targetID AND blocked_id = callerID.
func (r *Repository) IsBlockedBy(ctx context.Context, callerID, targetID uuid.UUID) (bool, error) {
	const q = `SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`

	var dummy int
	err := r.pool.QueryRow(ctx, q, targetID, callerID).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("follow: is blocked by: %w", err)
	}
	return true, nil
}

// ListFollowing returns a cursor-paginated list of users that userID follows.
// max is the hard upper bound (200). Returns (items, nextCursor, terminated, err).
// terminated is true when there are no further pages.
func (r *Repository) ListFollowing(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error) {
	fetchLimit := max + 1

	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		const q = `
			SELECT u.id, u.handle, u.display_name, u.avatar_url, f.created_at, f.followed_id
			FROM follows f
			JOIN users u ON u.id = f.followed_id
			WHERE f.follower_id = $1
			ORDER BY f.created_at DESC, f.followed_id DESC
			LIMIT $2`
		rows, err = r.pool.Query(ctx, q, userID, fetchLimit)
	} else {
		const q = `
			SELECT u.id, u.handle, u.display_name, u.avatar_url, f.created_at, f.followed_id
			FROM follows f
			JOIN users u ON u.id = f.followed_id
			WHERE f.follower_id = $1
			  AND (f.created_at, f.followed_id) < ($2, $3)
			ORDER BY f.created_at DESC, f.followed_id DESC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, userID, cursor.Timestamp, cursor.AfterID, fetchLimit)
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("follow: list following query: %w", err)
	}
	defer rows.Close()

	items, err := collectFollowRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("follow: list following scan: %w", err)
	}

	return buildFollowPage(items, max)
}

// ListFollowers returns a cursor-paginated list of users that follow userID.
// max is the hard upper bound (200). Returns (items, nextCursor, terminated, err).
func (r *Repository) ListFollowers(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]FollowUserDTO, string, bool, error) {
	fetchLimit := max + 1

	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		const q = `
			SELECT u.id, u.handle, u.display_name, u.avatar_url, f.created_at, f.follower_id
			FROM follows f
			JOIN users u ON u.id = f.follower_id
			WHERE f.followed_id = $1
			ORDER BY f.created_at DESC, f.follower_id DESC
			LIMIT $2`
		rows, err = r.pool.Query(ctx, q, userID, fetchLimit)
	} else {
		const q = `
			SELECT u.id, u.handle, u.display_name, u.avatar_url, f.created_at, f.follower_id
			FROM follows f
			JOIN users u ON u.id = f.follower_id
			WHERE f.followed_id = $1
			  AND (f.created_at, f.follower_id) < ($2, $3)
			ORDER BY f.created_at DESC, f.follower_id DESC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, userID, cursor.Timestamp, cursor.AfterID, fetchLimit)
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("follow: list followers query: %w", err)
	}
	defer rows.Close()

	items, err := collectFollowRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("follow: list followers scan: %w", err)
	}

	return buildFollowPage(items, max)
}

// GetFollowedIDs returns all user IDs that followerID follows with no pagination.
// Used by the home timeline feed query (Phase 4+).
func (r *Repository) GetFollowedIDs(ctx context.Context, followerID uuid.UUID) ([]uuid.UUID, error) {
	const q = `SELECT followed_id FROM follows WHERE follower_id = $1`

	rows, err := r.pool.Query(ctx, q, followerID)
	if err != nil {
		return nil, fmt.Errorf("follow: get followed ids: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("follow: get followed ids scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("follow: get followed ids rows: %w", err)
	}
	return ids, nil
}

// followRow holds a FollowUserDTO alongside the pagination columns needed to
// build the next cursor. pgx v5 scans TIMESTAMPTZ as time.Time.
type followRow struct {
	dto       FollowUserDTO
	createdAt time.Time
	paginID   uuid.UUID
}

// collectFollowRows scans rows that SELECT:
// u.id, u.handle, u.display_name, u.avatar_url, f.created_at, f.<follow_side_id>
func collectFollowRows(rows pgx.Rows) ([]followRow, error) {
	var result []followRow
	for rows.Next() {
		var fr followRow
		if err := rows.Scan(
			&fr.dto.ID,
			&fr.dto.Handle,
			&fr.dto.DisplayName,
			&fr.dto.AvatarURL,
			&fr.createdAt,
			&fr.paginID,
		); err != nil {
			return nil, err
		}
		result = append(result, fr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// buildFollowPage applies the max+1 has-more detection, builds the opaque
// cursor using post.FeedCursor, and sets the terminated flag.
func buildFollowPage(rows []followRow, max int) ([]FollowUserDTO, string, bool, error) {
	hasMore := len(rows) > max
	if hasMore {
		rows = rows[:max]
	}
	terminated := !hasMore

	var nextCursor string
	if !terminated && len(rows) > 0 {
		last := rows[len(rows)-1]
		c := post.FeedCursor{AfterID: last.paginID, Timestamp: last.createdAt}
		nextCursor = c.Encode()
	}

	dtos := make([]FollowUserDTO, len(rows))
	for i, fr := range rows {
		dtos[i] = fr.dto
	}

	return dtos, nextCursor, terminated, nil
}
