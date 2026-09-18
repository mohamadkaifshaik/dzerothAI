package block

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data-access methods for the block and mute domain.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a block Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Block inserts a block row and atomically removes follows in both directions
// within a single transaction.
//
// Idempotency: ON CONFLICT DO NOTHING ensures re-running this operation (e.g.
// after a transient failure) is safe. The DELETE on follows is also idempotent.
func (r *Repository) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("block: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insertBlock = `
		INSERT INTO blocks (blocker_id, blocked_id, created_at)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING`

	if _, err := tx.Exec(ctx, insertBlock, blockerID, blockedID); err != nil {
		return fmt.Errorf("block: insert block: %w", err)
	}

	// Remove follows in both directions atomically in the same transaction.
	// This is required so that a blocked user cannot silently remain a follower.
	const deleteFollows = `
		DELETE FROM follows
		WHERE (follower_id = $1 AND followed_id = $2)
		   OR (follower_id = $2 AND followed_id = $1)`

	if _, err := tx.Exec(ctx, deleteFollows, blockerID, blockedID); err != nil {
		return fmt.Errorf("block: delete follows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("block: commit tx: %w", err)
	}
	return nil
}

// Unblock removes a block row. Returns nil if the row did not exist — the
// operation is treated as a no-op in that case.
func (r *Repository) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	const q = `DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`
	if _, err := r.pool.Exec(ctx, q, blockerID, blockedID); err != nil {
		return fmt.Errorf("block: unblock: %w", err)
	}
	return nil
}

// Mute inserts a mute row. Idempotent: ON CONFLICT DO NOTHING.
func (r *Repository) Mute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	const q = `
		INSERT INTO mutes (muter_id, muted_id, created_at)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING`
	if _, err := r.pool.Exec(ctx, q, muterID, mutedID); err != nil {
		return fmt.Errorf("block: mute insert: %w", err)
	}
	return nil
}

// Unmute removes a mute row. Returns nil if the row did not exist — idempotent.
func (r *Repository) Unmute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	const q = `DELETE FROM mutes WHERE muter_id = $1 AND muted_id = $2`
	if _, err := r.pool.Exec(ctx, q, muterID, mutedID); err != nil {
		return fmt.Errorf("block: unmute: %w", err)
	}
	return nil
}

// IsBlocked returns true if blockerID has blocked blockedID.
func (r *Repository) IsBlocked(ctx context.Context, blockerID, blockedID uuid.UUID) (bool, error) {
	const q = `SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`
	var dummy int
	err := r.pool.QueryRow(ctx, q, blockerID, blockedID).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("block: is blocked: %w", err)
	}
	return true, nil
}

// IsBlockedBy returns true if targetID has blocked callerID — i.e., the caller
// has been blocked by the target. This is semantically equivalent to
// follow.Repository.IsBlockedBy and is provided here for package independence.
func (r *Repository) IsBlockedBy(ctx context.Context, callerID, targetID uuid.UUID) (bool, error) {
	// blocks WHERE blocker_id = targetID AND blocked_id = callerID
	return r.IsBlocked(ctx, targetID, callerID)
}

// IsBlockedBidirectional returns true if EITHER userA has blocked userB OR
// userB has blocked userA. Used by profile lookup to enforce mutual invisibility.
func (r *Repository) IsBlockedBidirectional(ctx context.Context, userA, userB uuid.UUID) (bool, error) {
	const q = `
		SELECT 1 FROM blocks
		WHERE (blocker_id = $1 AND blocked_id = $2)
		   OR (blocker_id = $2 AND blocked_id = $1)
		LIMIT 1`
	var dummy int
	err := r.pool.QueryRow(ctx, q, userA, userB).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("block: is blocked bidirectional: %w", err)
	}
	return true, nil
}

// GetBlockedIDs returns all user IDs that userID has blocked, with no
// pagination. Used by the home timeline feed filtering (Phase 4).
func (r *Repository) GetBlockedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	const q = `SELECT blocked_id FROM blocks WHERE blocker_id = $1`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("block: get blocked ids: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("block: get blocked ids scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("block: get blocked ids rows: %w", err)
	}
	return ids, nil
}

// GetMutedIDs returns all user IDs that userID has muted, with no pagination.
// Used by the home timeline feed filtering (Phase 4).
func (r *Repository) GetMutedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	const q = `SELECT muted_id FROM mutes WHERE muter_id = $1`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("block: get muted ids: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("block: get muted ids scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("block: get muted ids rows: %w", err)
	}
	return ids, nil
}
