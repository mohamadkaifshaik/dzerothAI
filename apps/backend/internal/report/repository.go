package report

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data-access for the report domain.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository constructs a report Repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// SubmitReport inserts a report row.
//
// The INSERT uses ON CONFLICT ... DO NOTHING targeting the functional dedup
// index reports_dedup_pending_idx, which enforces one pending report per
// (reporter_id, COALESCE(target_post_id, zero_uuid), COALESCE(target_user_id, zero_uuid))
// WHERE status = 'pending'.
//
// Returns:
//   - created=true  when a new row was inserted
//   - created=false when a pending report from this reporter for the same target
//     already existed (duplicate — the caller should treat this as idempotent success)
func (r *Repository) SubmitReport(
	ctx context.Context,
	reporterID uuid.UUID,
	targetType ReportTargetType,
	targetPostID *uuid.UUID,
	targetUserID *uuid.UUID,
	reason ReportReason,
	detail *string,
) (created bool, err error) {
	newID := uuid.New()

	const q = `
		INSERT INTO reports (id, reporter_id, target_type, target_post_id, target_user_id, reason, detail, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		ON CONFLICT ON CONSTRAINT reports_dedup_pending_idx DO NOTHING`

	tag, execErr := r.db.Exec(ctx, q,
		newID,
		reporterID,
		string(targetType),
		targetPostID,
		targetUserID,
		string(reason),
		detail,
	)
	if execErr != nil {
		return false, fmt.Errorf("report: submit: %w", execErr)
	}

	return tag.RowsAffected() == 1, nil
}
