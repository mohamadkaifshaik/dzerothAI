// Package post implements the post domain: internal models, public DTOs,
// feed cursor encoding, and mapping functions.
//
// Per CLAUDE.md §2.3, public DTOs must never include social-validation metrics.
// Per CLAUDE.md §2.1, feeds are finite — no infinite scrolling is permitted.
package post

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PostType represents the post_type ENUM values defined in migration 0004.
type PostType string

const (
	PostTypeOriginal PostType = "original"
	PostTypeReply    PostType = "reply"
	PostTypeQuote    PostType = "quote"
	PostTypeRepost   PostType = "repost"
)

// Post is the internal representation of a posts row.
// It must never be serialized directly into an API response.
// Use ToDTO to produce a client-safe PostDTO.
type Post struct {
	ID           uuid.UUID
	AuthorID     uuid.UUID
	PostType     PostType
	Content      *string
	ParentID     *uuid.UUID
	ThreadRootID *uuid.UUID
	QuotedPostID *uuid.UUID
	IsDeleted    bool
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// Author is populated by repository methods that JOIN the users table.
	Author PostAuthor
}

// PostAuthorTitle is the minimal title badge included in a PostAuthor block.
// It carries only the display fields required to render a title badge in post
// author blocks. This type is local to the post package to avoid an import
// cycle between post ↔ title (title → notification → post). The fields are
// identical to title.TitleSummaryDTO.
type PostAuthorTitle struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}

// PostAuthor holds the minimal author fields joined from the users table.
// PrimaryTitle is nil when the author has no primary title set.
type PostAuthor struct {
	ID           string
	Handle       string
	DisplayName  string
	AvatarURL    *string
	PrimaryTitle *PostAuthorTitle
}

// PostDTO is the public response struct for a single post.
//
// PostDTO intentionally contains zero social-validation metric fields — CLAUDE.md §2.3.
// Do not add like_count, impression_count, bookmark_count, reply_count,
// repost_count, share_count, view_count, follower_count, or any equivalent field.
type PostDTO struct {
	ID           string     `json:"id"`
	AuthorID     string     `json:"author_id"`
	Author       PostAuthor `json:"author"`
	PostType     PostType   `json:"post_type"`
	Content      *string    `json:"content"`
	ParentID     *string    `json:"parent_id,omitempty"`
	ThreadRootID *string    `json:"thread_root_id,omitempty"`
	QuotedPostID *string    `json:"quoted_post_id,omitempty"`
	IsDeleted    bool       `json:"is_deleted"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
}

// PostPage is the feed response envelope.
// Terminated MUST be set to true when the server-enforced depth limit is hit
// or there are no more items — the client must stop fetching (CLAUDE.md §2.1).
type PostPage struct {
	Items      []PostDTO `json:"items"`
	NextCursor string    `json:"next_cursor"`
	Terminated bool      `json:"terminated"`
}

// CreatePostRequest holds the fields required to create a new post.
// Validation is applied in the service layer before any database write.
type CreatePostRequest struct {
	Content      string   `json:"content"`
	PostType     PostType `json:"post_type"`
	ParentID     *string  `json:"parent_id,omitempty"`
	QuotedPostID *string  `json:"quoted_post_id,omitempty"`

	// ShareInitiatedAt is the UTC timestamp at which the client initiated the
	// share/quote action (i.e. when the mandatory 5-second countdown began).
	// Required for post_type "repost" and "quote"; must be absent or nil for
	// "original" and "reply".
	//
	// The backend measures elapsed time as time.Since(ShareInitiatedAt) against
	// its own clock. The client-supplied value is validated but never trusted for
	// business-logic purposes beyond "did the client start the countdown at least
	// 5 seconds ago?".
	//
	// Clock-skew tolerance: values up to 30 seconds in the future are accepted to
	// defend against minor NTP drift between client and server. Values more than
	// 30 seconds in the future are rejected as implausible.
	ShareInitiatedAt *time.Time `json:"share_initiated_at,omitempty"`
}

// ToDTO maps an internal Post to a public PostDTO.
// The caller is responsible for authorization before calling this function.
func ToDTO(p Post) PostDTO {
	dto := PostDTO{
		ID:        p.ID.String(),
		AuthorID:  p.AuthorID.String(),
		Author:    p.Author,
		PostType:  p.PostType,
		Content:   p.Content,
		IsDeleted: p.IsDeleted,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if p.ParentID != nil {
		s := p.ParentID.String()
		dto.ParentID = &s
	}
	if p.ThreadRootID != nil {
		s := p.ThreadRootID.String()
		dto.ThreadRootID = &s
	}
	if p.QuotedPostID != nil {
		s := p.QuotedPostID.String()
		dto.QuotedPostID = &s
	}
	return dto
}

// ReactionChecker allows post.Handler to query whether a given user has reacted
// to a post without importing the reaction package directly, avoiding a
// potential circular dependency. reaction.Service satisfies this interface via
// its HasReacted method.
type ReactionChecker interface {
	HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error)
}

// PostDetailResponse is the response struct for GET /posts/{postID}.
// It wraps PostDTO with viewer-only enrichment that must never appear on feed,
// thread, bookmark, or search responses (CLAUDE.md §2.3).
//
// ViewerHasReacted uses *bool with omitempty so the field is fully absent from
// the JSON response when the caller is unauthenticated (nil pointer).
// Authenticated callers receive true or false explicitly.
type PostDetailResponse struct {
	PostDTO
	ViewerHasReacted *bool `json:"viewer_has_reacted,omitempty"`
}

// FeedCursor is the decoded form of the opaque pagination cursor.
// Cursor format: base64url-encoded JSON {"after_id":"<uuid-v7>","ts":"<ISO8601Z>"}.
type FeedCursor struct {
	AfterID   uuid.UUID `json:"after_id"`
	Timestamp time.Time `json:"ts"`
}

// Encode serializes the cursor to the opaque base64url string sent to clients.
func (c FeedCursor) Encode() string {
	type wire struct {
		AfterID string `json:"after_id"`
		TS      string `json:"ts"`
	}
	w := wire{
		AfterID: c.AfterID.String(),
		TS:      c.Timestamp.UTC().Format(time.RFC3339Nano),
	}
	b, _ := json.Marshal(w)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses the opaque cursor string into a FeedCursor.
// Returns an error if the string is malformed or contains invalid values.
func DecodeCursor(s string) (*FeedCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("post: decode cursor base64: %w", err)
	}
	type wire struct {
		AfterID string `json:"after_id"`
		TS      string `json:"ts"`
	}
	var w wire
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("post: decode cursor json: %w", err)
	}
	id, err := uuid.Parse(w.AfterID)
	if err != nil {
		return nil, fmt.Errorf("post: decode cursor after_id: %w", err)
	}
	ts, err := time.Parse(time.RFC3339Nano, w.TS)
	if err != nil {
		return nil, fmt.Errorf("post: decode cursor ts: %w", err)
	}
	return &FeedCursor{AfterID: id, Timestamp: ts}, nil
}
