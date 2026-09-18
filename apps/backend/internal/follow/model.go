// Package follow implements the follow domain: directed follow relationships
// between users, cursor-paginated follower/following lists, and block checks.
//
// Per CLAUDE.md §2.3, public DTOs must never include social-validation metrics.
// Per CLAUDE.md §2.1, follow lists are finite — no infinite scrolling is permitted.
package follow

import (
	"time"

	"github.com/google/uuid"
)

// Follow represents a directed follow relationship stored in the follows table.
type Follow struct {
	FollowerID uuid.UUID
	FollowedID uuid.UUID
	CreatedAt  time.Time
}

// FollowUserDTO is the minimal public user shape returned in follower/following lists.
//
// Per CLAUDE.md §2.3, this struct must contain ZERO aggregate count fields.
// Do not add follower_count, following_count, like_count, impression_count,
// bookmark_count, or any equivalent social-validation metric.
type FollowUserDTO struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// FollowPage is the finite, cursor-paginated envelope for follow lists.
//
// Terminated MUST be set to true when the server-enforced depth limit is hit
// or there are no more items — the client must stop fetching (CLAUDE.md §2.1).
type FollowPage struct {
	Items      []FollowUserDTO `json:"items"`
	NextCursor string          `json:"next_cursor"`
	Terminated bool            `json:"terminated"`
}
