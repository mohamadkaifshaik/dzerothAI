// Package bookmark implements the bookmark domain: owner-only finite-paginated
// bookmark lists per the Dzeroth product constitution.
//
// Per CLAUDE.md §2.3, public DTOs must never include social-validation metrics.
// BookmarkDTO is NEVER nested inside post.PostDTO — bookmarks are a private
// user interaction, not a public post property.
package bookmark

import (
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// BookmarkDTO is the owner-only DTO returned in the bookmark list.
// It wraps post.PostDTO but is NEVER nested inside post.PostDTO (CLAUDE.md §2.3).
// No bookmark count field is added to post.PostDTO anywhere.
type BookmarkDTO struct {
	PostID    string       `json:"post_id"`
	CreatedAt string       `json:"created_at"` // ISO 8601 UTC
	Post      post.PostDTO `json:"post"`
}

// BookmarkPage is the finite cursor-paginated bookmark list.
// Terminated=true is a hard stop — client must not fetch further pages (CLAUDE.md §2.1).
type BookmarkPage struct {
	Items      []BookmarkDTO `json:"items"`
	NextCursor string        `json:"next_cursor"`
	Terminated bool          `json:"terminated"`
}

// bookmarkWithPost is an internal type used by the repository to return a
// bookmark row joined with its post. Not exported in DTO form.
type bookmarkWithPost struct {
	UserID            uuid.UUID
	PostID            uuid.UUID
	BookmarkCreatedAt time.Time
	Post              post.Post
}
