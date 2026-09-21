// Package notification implements the notification domain: owner-only
// finite-paginated notification inbox per the Dzeroth product constitution.
//
// Per CLAUDE.md §2.3, no notification counts are exposed in any public DTO.
// Notifications are owner-only: only the recipient can list or mark their own
// notifications.
package notification

import (
	"time"

	"github.com/google/uuid"
)

// NotificationEvent is the notification_event ENUM type defined in migration 0011.
type NotificationEvent string

const (
	EventFollow           NotificationEvent = "follow"
	EventMention          NotificationEvent = "mention"
	EventReply            NotificationEvent = "reply"
	EventReaction         NotificationEvent = "reaction"
	EventTitleUnlocked    NotificationEvent = "title_unlocked"
	EventTitleGracePeriod NotificationEvent = "title_grace_period"
)

// ActorSummary holds the minimal actor fields joined from the users table.
// Defined in this package so that consumers import internal/notification
// rather than a shared dto package (which does not exist in this codebase).
type ActorSummary struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
}

// NotificationDTO is the owner-only DTO for a single notification row.
// is_read is surfaced per-notification; no aggregate counts are present.
// Per CLAUDE.md §2.3, no social-validation metric fields are included.
type NotificationDTO struct {
	ID          string            `json:"id"`
	RecipientID string            `json:"recipient_id"`
	ActorID     string            `json:"actor_id"`
	Actor       ActorSummary      `json:"actor"`
	Event       NotificationEvent `json:"event"`
	PostID      *string           `json:"post_id,omitempty"`
	IsRead      bool              `json:"is_read"`
	CreatedAt   string            `json:"created_at"` // ISO 8601 UTC
}

// NotificationPage is the finite cursor-paginated notification inbox.
// Terminated=true is a hard stop — client must not fetch further pages
// (CLAUDE.md §2.1, no infinite scrolling).
type NotificationPage struct {
	Items      []NotificationDTO `json:"items"`
	NextCursor string            `json:"next_cursor"`
	Terminated bool              `json:"terminated"`
}

// PublishEvent is the event struct passed to the NotificationPublisher interface.
// It carries all fields needed to insert a single notification row.
// PostID is nil for follow events (which carry no associated post).
type PublishEvent struct {
	RecipientID uuid.UUID
	ActorID     uuid.UUID
	Event       NotificationEvent
	PostID      *uuid.UUID
}

// notificationRow is an internal type used by the repository to hold a
// scanned notification row joined with its actor. Not exported.
type notificationRow struct {
	ID          uuid.UUID
	RecipientID uuid.UUID
	ActorID     uuid.UUID
	Event       NotificationEvent
	PostID      *uuid.UUID
	IsRead      bool
	CreatedAt   time.Time
	Actor       ActorSummary
}
