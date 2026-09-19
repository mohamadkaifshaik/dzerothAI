// Package studio implements the private Creator Studio analytics domain.
//
// Per CLAUDE.md §2.3, social-validation metrics must never appear in public
// API responses. All types in this package are owner-only, JWT-callerID-scoped,
// and must never be imported by or embedded in public-facing packages such as
// internal/post, internal/feed, or internal/search.
//
// The studio package has a one-way dependency on internal/post for FeedCursor
// reuse only. internal/post must NOT depend on internal/studio.
package studio

// PostAnalytics holds PRIVATE analytics for a single post owned by the authenticated caller.
//
// THIS TYPE MUST NEVER BE RETURNED IN ANY PUBLIC ENDPOINT RESPONSE.
// It is owner-only, JWT-callerID-scoped.
//
// These counts are PRIVATE — CLAUDE.md §2.3 and Dzeroth product constitution.
// They are NOT public social-validation metrics.
// They are current-state aggregates only — no historical/time-series data.
//
// Semantics:
//
//	reaction_count  — number of distinct users who currently have a reaction on this post (reactions table)
//	bookmark_count  — number of distinct users who currently have this post bookmarked (bookmarks table)
//	reply_count     — number of current non-deleted direct replies (posts WHERE parent_id = this AND post_type='reply' AND is_deleted=FALSE)
//	quote_count     — number of current non-deleted quote posts (posts WHERE quoted_post_id = this AND post_type='quote' AND is_deleted=FALSE)
//
// All counts reflect live database state at query time. Deleted replies/quotes do not contribute.
// Reposts (post_type='repost') are NOT counted as quote posts.
type PostAnalytics struct {
	PostID        string `json:"post_id"`
	Content       string `json:"content"` // post content (may be empty for reposts)
	PostType      string `json:"post_type"`
	CreatedAt     string `json:"created_at"` // ISO 8601 with Z suffix
	ReactionCount int    `json:"reaction_count"`
	BookmarkCount int    `json:"bookmark_count"`
	ReplyCount    int    `json:"reply_count"`
	QuoteCount    int    `json:"quote_count"`
}

// StudioPage is the finite cursor-paginated response for Creator Studio.
// Same envelope convention as all other paginated responses (FeedCursor-based).
// Terminated MUST be set to true when the server-enforced depth limit (maxStudioDepth)
// is reached or there are no more items — the client must stop fetching (CLAUDE.md §2.1).
type StudioPage struct {
	Items      []PostAnalytics `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
	Terminated bool            `json:"terminated"`
}

// maxStudioDepth is the hard upper-bound on items returned in a single Studio
// analytics session. The client must stop fetching when Terminated is true.
const maxStudioDepth = 50
