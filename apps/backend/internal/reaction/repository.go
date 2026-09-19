package reaction

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data-access methods for the reaction domain.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a reaction Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// React inserts a reaction row with reaction_type='like'.
// Idempotent via ON CONFLICT DO NOTHING.
// Returns created=true if a new row was inserted, created=false if the
// reaction already existed (conflict on the (user_id, post_id) primary key).
func (r *Repository) React(ctx context.Context, userID, postID uuid.UUID) (created bool, err error) {
	const q = `
		INSERT INTO reactions (user_id, post_id, reaction_type, created_at)
		VALUES ($1, $2, 'like', now())
		ON CONFLICT (user_id, post_id) DO NOTHING`

	tag, err := r.pool.Exec(ctx, q, userID, postID)
	if err != nil {
		return false, fmt.Errorf("reaction: react insert: %w", err)
	}
	// RowsAffected == 1 means a row was inserted; 0 means conflict (already existed).
	return tag.RowsAffected() == 1, nil
}

// Unreact deletes the reaction row for (userID, postID).
// Returns nil if the row did not exist (idempotent).
func (r *Repository) Unreact(ctx context.Context, userID, postID uuid.UUID) error {
	const q = `DELETE FROM reactions WHERE user_id = $1 AND post_id = $2`
	_, err := r.pool.Exec(ctx, q, userID, postID)
	if err != nil {
		return fmt.Errorf("reaction: unreact delete: %w", err)
	}
	return nil
}

// HasReacted returns true if a reaction row exists for (userID, postID).
func (r *Repository) HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM reactions WHERE user_id = $1 AND post_id = $2)`
	var exists bool
	if err := r.pool.QueryRow(ctx, q, userID, postID).Scan(&exists); err != nil {
		return false, fmt.Errorf("reaction: has reacted: %w", err)
	}
	return exists, nil
}
