package studio

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// Repository provides data-access methods for the Creator Studio domain.
// All queries are scoped to the authenticated owner's posts.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository constructs a studio Repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// studioPostRow holds the minimal columns fetched in the first query.
type studioPostRow struct {
	ID        uuid.UUID
	Content   *string
	PostType  string
	CreatedAt time.Time
}

// ListPostAnalytics returns a finite, cursor-paginated list of PostAnalytics
// for posts owned by ownerID. Counts are fetched in four separate batch queries
// (one per metric) to avoid cross-join cardinality issues and to maximise
// index use.
//
// Returns (items, nextCursor, terminated, error).
// terminated is true when there are no further rows, or when the hard cap
// maxStudioDepth is reached.
func (r *Repository) ListPostAnalytics(ctx context.Context, ownerID uuid.UUID, cursor *post.FeedCursor, limit int) ([]PostAnalytics, string, bool, error) {
	fetchLimit := limit + 1

	// ── Step 1: fetch posts with cursor pagination ──────────────────────────
	var postRows []studioPostRow
	var err error

	if cursor == nil {
		const q = `
			SELECT p.id, p.content, p.post_type, p.created_at
			FROM posts p
			WHERE p.author_id = $1 AND p.is_deleted = FALSE
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $2`
		rows, qErr := r.db.Query(ctx, q, ownerID, fetchLimit)
		if qErr != nil {
			return nil, "", false, fmt.Errorf("studio: list analytics query: %w", qErr)
		}
		postRows, err = collectStudioPostRows(rows)
		rows.Close()
	} else {
		const q = `
			SELECT p.id, p.content, p.post_type, p.created_at
			FROM posts p
			WHERE p.author_id = $1 AND p.is_deleted = FALSE
			  AND (p.created_at, p.id) < ($2, $3)
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $4`
		rows, qErr := r.db.Query(ctx, q, ownerID, cursor.Timestamp, cursor.AfterID, fetchLimit)
		if qErr != nil {
			return nil, "", false, fmt.Errorf("studio: list analytics cursor query: %w", qErr)
		}
		postRows, err = collectStudioPostRows(rows)
		rows.Close()
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("studio: list analytics scan: %w", err)
	}

	// ── Step 2: empty result → terminated ───────────────────────────────────
	if len(postRows) == 0 {
		return nil, "", true, nil
	}

	// ── Step 3: has-more detection and truncation ────────────────────────────
	hasMore := len(postRows) > limit
	if hasMore {
		postRows = postRows[:limit]
	}

	// Build a slice of UUIDs for the batch aggregate queries.
	postIDs := make([]uuid.UUID, len(postRows))
	for i, pr := range postRows {
		postIDs[i] = pr.ID
	}

	// ── Step 4: batch aggregate queries ─────────────────────────────────────
	reactionCounts, err := r.batchReactionCounts(ctx, postIDs)
	if err != nil {
		return nil, "", false, fmt.Errorf("studio: batch reaction counts: %w", err)
	}

	bookmarkCounts, err := r.batchBookmarkCounts(ctx, postIDs)
	if err != nil {
		return nil, "", false, fmt.Errorf("studio: batch bookmark counts: %w", err)
	}

	replyCounts, err := r.batchReplyCounts(ctx, postIDs)
	if err != nil {
		return nil, "", false, fmt.Errorf("studio: batch reply counts: %w", err)
	}

	quoteCounts, err := r.batchQuoteCounts(ctx, postIDs)
	if err != nil {
		return nil, "", false, fmt.Errorf("studio: batch quote counts: %w", err)
	}

	// ── Step 5: merge ────────────────────────────────────────────────────────
	items := make([]PostAnalytics, len(postRows))
	for i, pr := range postRows {
		content := ""
		if pr.Content != nil {
			content = *pr.Content
		}
		items[i] = PostAnalytics{
			PostID:        pr.ID.String(),
			Content:       content,
			PostType:      pr.PostType,
			CreatedAt:     pr.CreatedAt.UTC().Format(time.RFC3339),
			ReactionCount: reactionCounts[pr.ID],
			BookmarkCount: bookmarkCounts[pr.ID],
			ReplyCount:    replyCounts[pr.ID],
			QuoteCount:    quoteCounts[pr.ID],
		}
	}

	// ── Step 6: encode next cursor ───────────────────────────────────────────
	terminated := !hasMore
	var nextCursor string
	if !terminated {
		last := postRows[len(postRows)-1]
		c := post.FeedCursor{AfterID: last.ID, Timestamp: last.CreatedAt}
		nextCursor = c.Encode()
	}

	return items, nextCursor, terminated, nil
}

// collectStudioPostRows scans rows from the posts query into studioPostRow values.
func collectStudioPostRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]studioPostRow, error) {
	var result []studioPostRow
	for rows.Next() {
		var pr studioPostRow
		if err := rows.Scan(&pr.ID, &pr.Content, &pr.PostType, &pr.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, pr)
	}
	return result, rows.Err()
}

// batchReactionCounts queries reaction counts for the given post IDs.
// Uses reactions_post_id_idx (migration 0010).
func (r *Repository) batchReactionCounts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	const q = `
		SELECT post_id, COUNT(*) AS cnt
		FROM reactions
		WHERE post_id = ANY($1)
		GROUP BY post_id`
	return r.batchIntCounts(ctx, q, postIDs)
}

// batchBookmarkCounts queries bookmark counts for the given post IDs.
// Uses bookmarks_post_id_idx (migration 0013).
func (r *Repository) batchBookmarkCounts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	const q = `
		SELECT post_id, COUNT(*) AS cnt
		FROM bookmarks
		WHERE post_id = ANY($1)
		GROUP BY post_id`
	return r.batchIntCounts(ctx, q, postIDs)
}

// batchReplyCounts queries direct-reply counts for the given post IDs.
// Uses posts_parent_id_idx (migration 0004).
func (r *Repository) batchReplyCounts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	const q = `
		SELECT parent_id, COUNT(*) AS cnt
		FROM posts
		WHERE parent_id = ANY($1)
		  AND post_type = 'reply'
		  AND is_deleted = FALSE
		GROUP BY parent_id`
	return r.batchIntCounts(ctx, q, postIDs)
}

// batchQuoteCounts queries quote-post counts for the given post IDs.
// Uses posts_quoted_post_id_idx (migration 0013, partial index on quote+non-deleted).
// Reposts (post_type='repost') are explicitly excluded.
func (r *Repository) batchQuoteCounts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	const q = `
		SELECT quoted_post_id, COUNT(*) AS cnt
		FROM posts
		WHERE quoted_post_id = ANY($1)
		  AND post_type = 'quote'
		  AND is_deleted = FALSE
		GROUP BY quoted_post_id`
	return r.batchIntCounts(ctx, q, postIDs)
}

// batchIntCounts is a helper that executes a query of the shape
// "SELECT id_col, COUNT(*) FROM ... WHERE id_col = ANY($1) GROUP BY id_col"
// and returns a map from UUID to count. IDs absent from the result are zero.
func (r *Repository) batchIntCounts(ctx context.Context, q string, postIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	result := make(map[uuid.UUID]int, len(postIDs))

	rows, err := r.db.Query(ctx, q, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, err
		}
		result[id] = cnt
	}
	return result, rows.Err()
}
