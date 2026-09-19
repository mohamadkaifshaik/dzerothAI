// Package search implements the search domain: full-text search over posts and
// users using the pg_trgm GIN indexes added in migration 0012.
//
// Per CLAUDE.md §2.3, public DTOs must never include social-validation metrics.
// Per CLAUDE.md §2.1, search result pages are finite — no infinite scrolling.
package search

import (
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// UserSearchResult is the minimal public user shape returned in user search results.
//
// Per CLAUDE.md §2.3, this struct must contain ZERO aggregate count fields.
// Do not add follower_count, following_count, like_count, impression_count,
// bookmark_count, or any equivalent social-validation metric.
type UserSearchResult struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// PostSearchPage is the finite, cursor-paginated envelope for post search results.
//
// Terminated MUST be set to true when the server-enforced depth limit is hit
// or there are no more items — the client must stop fetching (CLAUDE.md §2.1).
type PostSearchPage struct {
	Items      []post.PostDTO `json:"items"`
	NextCursor string         `json:"next_cursor"`
	Terminated bool           `json:"terminated"`
}

// UserSearchPage is the finite, cursor-paginated envelope for user search results.
//
// Terminated MUST be set to true when the server-enforced depth limit is hit
// or there are no more items — the client must stop fetching (CLAUDE.md §2.1).
type UserSearchPage struct {
	Items      []UserSearchResult `json:"items"`
	NextCursor string             `json:"next_cursor"`
	Terminated bool               `json:"terminated"`
}
