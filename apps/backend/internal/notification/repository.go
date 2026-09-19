package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// Repository provides data-access methods for the notification domain.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a notification Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Publish inserts a notification row.
// Idempotent: ON CONFLICT on the dedup index (recipient, actor, event,
// COALESCE(post_id, nil-uuid)) does nothing. Self-notifications (actor ==
// recipient) must be suppressed by the caller before reaching this method;
// the repository does not re-check this condition.
func (r *Repository) Publish(ctx context.Context, event PublishEvent) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("notification: generate id: %w", err)
	}

	// ON CONFLICT DO NOTHING (no target specified) matches any unique constraint
	// violation on the table, including the expression index notifications_dedup_idx.
	// PostgreSQL requires the omission of an explicit conflict target when the
	// conflicting index is defined on an expression (COALESCE) rather than a plain column.
	const q = `
		INSERT INTO notifications (id, recipient_id, actor_id, event, post_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING`

	_, err = r.pool.Exec(ctx, q,
		id,
		event.RecipientID,
		event.ActorID,
		string(event.Event),
		event.PostID,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("notification: publish insert: %w", err)
	}
	return nil
}

// ListNotifications returns a cursor-paginated notification inbox for the
// given recipient. maxDepth = 100. Excludes self-notifications (actor ==
// recipient). Orders by created_at DESC, id DESC. Returns Terminated=true
// when there are no further pages (CLAUDE.md §2.1).
func (r *Repository) ListNotifications(ctx context.Context, recipientID uuid.UUID, cursor *post.FeedCursor) (NotificationPage, error) {
	const maxDepth = 100
	fetchLimit := maxDepth + 1

	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		const q = `
			SELECT
				n.id, n.recipient_id, n.actor_id, n.event, n.post_id,
				n.is_read, n.created_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM notifications n
			JOIN users u ON u.id = n.actor_id
			WHERE n.recipient_id = $1
			  AND n.actor_id <> n.recipient_id
			ORDER BY n.created_at DESC, n.id DESC
			LIMIT $2`
		rows, err = r.pool.Query(ctx, q, recipientID, fetchLimit)
	} else {
		const q = `
			SELECT
				n.id, n.recipient_id, n.actor_id, n.event, n.post_id,
				n.is_read, n.created_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM notifications n
			JOIN users u ON u.id = n.actor_id
			WHERE n.recipient_id = $1
			  AND n.actor_id <> n.recipient_id
			  AND (n.created_at < $2 OR (n.created_at = $2 AND n.id < $3))
			ORDER BY n.created_at DESC, n.id DESC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, recipientID, cursor.Timestamp, cursor.AfterID, fetchLimit)
	}
	if err != nil {
		return NotificationPage{}, fmt.Errorf("notification: list query: %w", err)
	}
	defer rows.Close()

	items, err := collectNotificationRows(rows)
	if err != nil {
		return NotificationPage{}, fmt.Errorf("notification: list scan: %w", err)
	}

	return buildNotificationPage(items, maxDepth), nil
}

// MarkAllRead sets is_read = TRUE for all unread notifications owned by
// recipientID. Idempotent: no error if there are no unread rows.
func (r *Repository) MarkAllRead(ctx context.Context, recipientID uuid.UUID) error {
	const q = `
		UPDATE notifications
		SET is_read = TRUE
		WHERE recipient_id = $1
		  AND is_read = FALSE`

	_, err := r.pool.Exec(ctx, q, recipientID)
	if err != nil {
		return fmt.Errorf("notification: mark all read: %w", err)
	}
	return nil
}

// collectNotificationRows scans all rows from the notification JOIN users query.
func collectNotificationRows(rows pgx.Rows) ([]notificationRow, error) {
	var items []notificationRow
	for rows.Next() {
		var nr notificationRow
		var actorID string
		var event string
		if err := rows.Scan(
			&nr.ID,
			&nr.RecipientID,
			&nr.ActorID,
			&event,
			&nr.PostID,
			&nr.IsRead,
			&nr.CreatedAt,
			// actor (users join)
			&actorID,
			&nr.Actor.Handle,
			&nr.Actor.DisplayName,
			&nr.Actor.AvatarURL,
		); err != nil {
			return nil, err
		}
		nr.Actor.ID = actorID
		nr.Event = NotificationEvent(event)
		items = append(items, nr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// buildNotificationPage trims the extra row fetched for has-more detection,
// builds the cursor, and sets the terminated flag.
func buildNotificationPage(items []notificationRow, max int) NotificationPage {
	hasMore := len(items) > max
	if hasMore {
		items = items[:max]
	}
	terminated := !hasMore

	var nextCursor string
	if !terminated && len(items) > 0 {
		last := items[len(items)-1]
		c := post.FeedCursor{
			AfterID:   last.ID,
			Timestamp: last.CreatedAt,
		}
		nextCursor = c.Encode()
	}

	dtos := make([]NotificationDTO, len(items))
	for i, nr := range items {
		dto := NotificationDTO{
			ID:          nr.ID.String(),
			RecipientID: nr.RecipientID.String(),
			ActorID:     nr.ActorID.String(),
			Actor:       nr.Actor,
			Event:       nr.Event,
			IsRead:      nr.IsRead,
			CreatedAt:   nr.CreatedAt.UTC().Format(time.RFC3339),
		}
		if nr.PostID != nil {
			s := nr.PostID.String()
			dto.PostID = &s
		}
		dtos[i] = dto
	}

	// Guarantee non-nil slice for consistent JSON serialization.
	if len(dtos) == 0 {
		dtos = []NotificationDTO{}
	}

	return NotificationPage{
		Items:      dtos,
		NextCursor: nextCursor,
		Terminated: terminated,
	}
}
