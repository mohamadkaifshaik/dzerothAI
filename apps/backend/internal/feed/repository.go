// Package feed implements the home timeline feed for authenticated users.
//
// Per CLAUDE.md §2.1, the feed is finite — no infinite scrolling is permitted.
// The server-enforced depth limit is maxHomeFeedDepth (200), served in pages
// of homeFeedPageSize.
//
// Per CLAUDE.md §2.3, public DTOs must never include social-validation metrics.
// This package reuses post.PostDTO, which already excludes those fields.
package feed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
// Hard depth (CLAUDE.md §2.1, ADR 0006): all of the filters above are applied
// inside the window CTE, which keeps the caller's current top-maxDepth eligible
// posts — filtered rows never consume the window. The cursor pages inside that
// window only, pageSize rows at a time, so no cursor (valid, forged, or stale)
// can reach a post ranked below maxDepth. A cursor past the end of the current
// window yields an empty, terminated page.
//
// Parameters:
//   - callerID:   authenticated user requesting the feed
//   - blockedIDs: IDs that callerID has blocked (outgoing blocks)
//   - mutedIDs:   IDs that callerID has muted
//   - cursor:     optional pagination cursor (nil = first page)
//   - pageSize:   items per page
//   - maxDepth:   hard cumulative depth of the feed (maxHomeFeedDepth)
//
// Returns: (posts []post.Post, nextCursor string, terminated bool, err error)
// terminated=true means no further pages exist (client must stop — CLAUDE.md §2.1).
func (r *Repository) ListHomeTimeline(
	ctx context.Context,
	callerID uuid.UUID,
	blockedIDs []uuid.UUID,
	mutedIDs []uuid.UUID,
	cursor *post.FeedCursor,
	pageSize int,
	maxDepth int,
) ([]post.Post, string, bool, error) {
	// Convert []uuid.UUID to []string for pgx array passing.
	// pgx v5 can scan uuid.UUID slices but needs them as []any for ANY($n::uuid[]).
	// We use the same approach as other repositories: pass nil as an empty array
	// sentinel, which pgx maps to a NULL and the CARDINALITY check handles.
	blockedArr := uuidSliceToStringSlice(blockedIDs)
	mutedArr := uuidSliceToStringSlice(mutedIDs)

	// Nil cursor → SQL NULL → start at the top of the window.
	var cursorTS, cursorID any
	if cursor != nil {
		cursorTS, cursorID = cursor.Timestamp, cursor.AfterID
	}

	const q = `
		WITH win AS (
			SELECT p.id, p.created_at
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
			LIMIT $4
		)
		SELECT
			p.id, p.author_id, p.post_type, p.content,
			p.parent_id, p.thread_root_id, p.quoted_post_id,
			p.is_deleted, p.created_at, p.updated_at,
			u.id, u.handle, u.display_name, u.avatar_url,
			td.slug, td.display_name
		FROM win w
		JOIN posts p ON p.id = w.id
		JOIN users u ON u.id = p.author_id
		LEFT JOIN user_titles ut ON ut.id = u.primary_title_id
		LEFT JOIN title_definitions td ON td.id = ut.title_definition_id
		WHERE $5::timestamptz IS NULL OR (w.created_at, w.id) < ($5::timestamptz, $6::uuid)
		ORDER BY w.created_at DESC, w.id DESC
		LIMIT $7`

	// Fetch one extra row (within the window) to detect whether more rows exist.
	rows, err := r.pool.Query(ctx, q, callerID, blockedArr, mutedArr, maxDepth, cursorTS, cursorID, pageSize+1)
	if err != nil {
		return nil, "", false, fmt.Errorf("feed: list home timeline query: %w", err)
	}
	defer rows.Close()

	// Reuse the exported scan helper from the post package (OPEN-5).
	posts, err := post.ScanPostRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("feed: list home timeline scan: %w", err)
	}

	return buildFeedPage(posts, pageSize)
}

// buildFeedPage applies the max+1 has-more detection, builds the opaque cursor,
// and sets the terminated flag. Reuses post.FeedCursor.Encode() for consistency.
// posts must be at most max+1 rows taken from inside the top-maxDepth window, so
// "no extra row" means the window is exhausted: terminated=true, empty cursor.
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
