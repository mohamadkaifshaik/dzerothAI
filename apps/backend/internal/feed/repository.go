// Package feed implements the home timeline feed for authenticated users.
//
// Per CLAUDE.md §2.1, the feed is finite — no infinite scrolling is permitted.
// The server-enforced depth limit is maxHomeFeedDepth (200).
//
// Per CLAUDE.md §2.3, public DTOs must never include social-validation metrics.
// This package reuses post.PostDTO, which already excludes those fields.
package feed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// Repository provides data-access methods for the home timeline feed.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a feed Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListHomeTimeline returns a cursor-paginated slice of posts for the caller's home feed.
//
// Filters applied at the SQL level (never in Go post-processing):
//   - post.is_deleted = FALSE
//   - post.author_id IN (SELECT followed_id FROM follows WHERE follower_id = callerID)
//   - post.author_id NOT IN blockedIDs (bidirectional block exclusion is naturally
//     satisfied because block.Repository.Block removes follows in both directions,
//     so a user who has blocked the caller will not appear in the follows subquery)
//   - post.author_id NOT IN mutedIDs
//   - For private accounts: only included when callerID follows them, which the
//     follows JOIN already guarantees
//
// Parameters:
//   - callerID: authenticated user requesting the feed
//   - blockedIDs: IDs that callerID has blocked (outgoing blocks)
//   - mutedIDs:   IDs that callerID has muted
//   - cursor:     optional pagination cursor (nil = first page)
//   - max:        hard upper bound; must not exceed maxHomeFeedDepth
//
// Returns: (posts []post.Post, nextCursor string, terminated bool, err error)
// terminated=true means no further pages exist (client must stop — CLAUDE.md §2.1).
func (r *Repository) ListHomeTimeline(
	ctx context.Context,
	callerID uuid.UUID,
	blockedIDs []uuid.UUID,
	mutedIDs []uuid.UUID,
	cursor *post.FeedCursor,
	max int,
) ([]post.Post, string, bool, error) {
	// Fetch one extra row to detect whether more rows exist beyond the limit.
	fetchLimit := max + 1

	// Convert []uuid.UUID to []string for pgx array passing.
	// pgx v5 can scan uuid.UUID slices but needs them as []any for ANY($n::uuid[]).
	// We use the same approach as other repositories: pass nil as an empty array
	// sentinel, which pgx maps to a NULL and the CARDINALITY check handles.
	blockedArr := uuidSliceToStringSlice(blockedIDs)
	mutedArr := uuidSliceToStringSlice(mutedIDs)

	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		const q = `
			SELECT
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM posts p
			JOIN users u ON u.id = p.author_id
			WHERE p.author_id IN (
				SELECT followed_id FROM follows WHERE follower_id = $1
			)
			AND p.is_deleted = FALSE
			AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR p.author_id::text != ALL($2::text[]))
			AND ($3::text[] IS NULL OR array_length($3::text[], 1) IS NULL OR p.author_id::text != ALL($3::text[]))
			AND (
				u.is_private = FALSE
				OR EXISTS (
					SELECT 1 FROM follows
					WHERE follower_id = $1 AND followed_id = p.author_id
				)
			)
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, callerID, blockedArr, mutedArr, fetchLimit)
	} else {
		const q = `
			SELECT
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM posts p
			JOIN users u ON u.id = p.author_id
			WHERE p.author_id IN (
				SELECT followed_id FROM follows WHERE follower_id = $1
			)
			AND p.is_deleted = FALSE
			AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR p.author_id::text != ALL($2::text[]))
			AND ($3::text[] IS NULL OR array_length($3::text[], 1) IS NULL OR p.author_id::text != ALL($3::text[]))
			AND (
				u.is_private = FALSE
				OR EXISTS (
					SELECT 1 FROM follows
					WHERE follower_id = $1 AND followed_id = p.author_id
				)
			)
			AND (p.created_at < $5 OR (p.created_at = $5 AND p.id < $6))
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, callerID, blockedArr, mutedArr, fetchLimit, cursor.Timestamp, cursor.AfterID)
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("feed: list home timeline query: %w", err)
	}
	defer rows.Close()

	// Reuse the exported scan helper from the post package (OPEN-5).
	posts, err := post.ScanPostRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("feed: list home timeline scan: %w", err)
	}

	return buildFeedPage(posts, max)
}

// buildFeedPage applies the max+1 has-more detection, builds the opaque cursor,
// and sets the terminated flag. Reuses post.FeedCursor.Encode() for consistency.
func buildFeedPage(posts []post.Post, max int) ([]post.Post, string, bool, error) {
	hasMore := len(posts) > max
	if hasMore {
		posts = posts[:max]
	}

	terminated := !hasMore

	var nextCursor string
	if !terminated && len(posts) > 0 {
		last := posts[len(posts)-1]
		c := post.FeedCursor{AfterID: last.ID, Timestamp: last.CreatedAt}
		nextCursor = c.Encode()
	}

	return posts, nextCursor, terminated, nil
}

// uuidSliceToStringSlice converts []uuid.UUID to []string for pgx text[] array binding.
// Returns nil when the input is empty so that the SQL NULL/length check fires correctly.
func uuidSliceToStringSlice(ids []uuid.UUID) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}
