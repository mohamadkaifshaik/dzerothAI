package bookmark

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// Repository provides data-access methods for the bookmark domain.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a bookmark Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Add inserts a bookmark. Idempotent: ON CONFLICT DO NOTHING.
// Returns CodeNotFound if the post does not exist or is soft-deleted.
func (r *Repository) Add(ctx context.Context, userID, postID uuid.UUID) error {
	// Verify the post exists and is not deleted before inserting.
	const checkPost = `SELECT id FROM posts WHERE id = $1 AND is_deleted = FALSE`
	var dummy uuid.UUID
	err := r.pool.QueryRow(ctx, checkPost, postID).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		}
		return fmt.Errorf("bookmark: add check post: %w", err)
	}

	const insertBookmark = `
		INSERT INTO bookmarks (user_id, post_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`
	_, err = r.pool.Exec(ctx, insertBookmark, userID, postID)
	if err != nil {
		return fmt.Errorf("bookmark: add insert: %w", err)
	}
	return nil
}

// Remove deletes a bookmark. Returns nil if it didn't exist (idempotent).
func (r *Repository) Remove(ctx context.Context, userID, postID uuid.UUID) error {
	const q = `DELETE FROM bookmarks WHERE user_id = $1 AND post_id = $2`
	_, err := r.pool.Exec(ctx, q, userID, postID)
	if err != nil {
		return fmt.Errorf("bookmark: remove: %w", err)
	}
	return nil
}

// ListByUser returns a cursor-paginated list of bookmarks with their posts.
// Filters out soft-deleted posts (is_deleted = FALSE).
// max is the hard upper bound (200).
// Returns (items []bookmarkWithPost, nextCursor string, terminated bool, err error).
func (r *Repository) ListByUser(ctx context.Context, userID uuid.UUID, cursor *post.FeedCursor, max int) ([]bookmarkWithPost, string, bool, error) {
	fetchLimit := max + 1

	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		const q = `
			SELECT
				b.user_id, b.post_id, b.created_at,
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM bookmarks b
			JOIN posts p ON p.id = b.post_id
			JOIN users u ON u.id = p.author_id
			WHERE b.user_id = $1
			  AND p.is_deleted = FALSE
			ORDER BY b.created_at DESC, b.post_id DESC
			LIMIT $2`
		rows, err = r.pool.Query(ctx, q, userID, fetchLimit)
	} else {
		const q = `
			SELECT
				b.user_id, b.post_id, b.created_at,
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM bookmarks b
			JOIN posts p ON p.id = b.post_id
			JOIN users u ON u.id = p.author_id
			WHERE b.user_id = $1
			  AND p.is_deleted = FALSE
			  AND (b.created_at < $2 OR (b.created_at = $2 AND b.post_id < $3))
			ORDER BY b.created_at DESC, b.post_id DESC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, userID, cursor.Timestamp, cursor.AfterID, fetchLimit)
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("bookmark: list by user query: %w", err)
	}
	defer rows.Close()

	items, err := collectBookmarkRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("bookmark: list by user scan: %w", err)
	}

	return buildBookmarkPage(items, max)
}

// collectBookmarkRows scans all rows from a bookmarks JOIN posts JOIN users query.
// The bookmark columns (user_id, post_id, b.created_at) are scanned first, then
// the post and author columns are scanned manually following the same column order
// as post.collectRows. We cannot call post.ScanPostRows directly because the
// bookmark query has additional leading columns that offset the scan positions.
func collectBookmarkRows(rows pgx.Rows) ([]bookmarkWithPost, error) {
	var items []bookmarkWithPost
	for rows.Next() {
		var bwp bookmarkWithPost
		var authorID string
		err := rows.Scan(
			// bookmark columns
			&bwp.UserID,
			&bwp.PostID,
			&bwp.BookmarkCreatedAt,
			// post columns
			&bwp.Post.ID,
			&bwp.Post.AuthorID,
			&bwp.Post.PostType,
			&bwp.Post.Content,
			&bwp.Post.ParentID,
			&bwp.Post.ThreadRootID,
			&bwp.Post.QuotedPostID,
			&bwp.Post.IsDeleted,
			&bwp.Post.CreatedAt,
			&bwp.Post.UpdatedAt,
			// user columns
			&authorID,
			&bwp.Post.Author.Handle,
			&bwp.Post.Author.DisplayName,
			&bwp.Post.Author.AvatarURL,
		)
		if err != nil {
			return nil, err
		}
		bwp.Post.Author.ID = authorID
		items = append(items, bwp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// buildBookmarkPage trims the extra row fetched for has-more detection and
// builds the cursor and terminated flag using post.FeedCursor on (b.created_at, b.post_id).
func buildBookmarkPage(items []bookmarkWithPost, max int) ([]bookmarkWithPost, string, bool, error) {
	hasMore := len(items) > max
	if hasMore {
		items = items[:max]
	}

	terminated := !hasMore

	var nextCursor string
	if !terminated && len(items) > 0 {
		last := items[len(items)-1]
		c := post.FeedCursor{
			AfterID:   last.PostID,
			Timestamp: last.BookmarkCreatedAt,
		}
		nextCursor = c.Encode()
	}

	return items, nextCursor, terminated, nil
}
