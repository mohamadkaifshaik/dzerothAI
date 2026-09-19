package post

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when the requested post does not exist.
var ErrNotFound = errors.New("post: not found")

// Repository provides data-access methods for the post domain.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a post Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a post, its mentions, and its hashtags in a single transaction.
// The caller must supply a fully populated Post (including the application-generated ID).
func (r *Repository) Create(ctx context.Context, p Post, mentions []uuid.UUID, hashtags []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("post: create begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insertPost = `
		INSERT INTO posts (id, author_id, post_type, content, parent_id, thread_root_id, quoted_post_id, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, $8, $9)`

	_, err = tx.Exec(ctx, insertPost,
		p.ID,
		p.AuthorID,
		string(p.PostType),
		p.Content,
		p.ParentID,
		p.ThreadRootID,
		p.QuotedPostID,
		p.CreatedAt,
		p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("post: create insert post: %w", err)
	}

	if len(mentions) > 0 {
		const insertMention = `
			INSERT INTO post_mentions (post_id, user_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`
		for _, userID := range mentions {
			if _, err := tx.Exec(ctx, insertMention, p.ID, userID); err != nil {
				return fmt.Errorf("post: create insert mention: %w", err)
			}
		}
	}

	if len(hashtags) > 0 {
		const insertHashtag = `
			INSERT INTO post_hashtags (post_id, tag)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`
		for _, tag := range hashtags {
			if _, err := tx.Exec(ctx, insertHashtag, p.ID, tag); err != nil {
				return fmt.Errorf("post: create insert hashtag: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("post: create commit: %w", err)
	}
	return nil
}

// GetByID fetches a single post joined with its author. Returns ErrNotFound if no row matches.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Post, error) {
	const q = `
		SELECT
			p.id, p.author_id, p.post_type, p.content,
			p.parent_id, p.thread_root_id, p.quoted_post_id,
			p.is_deleted, p.created_at, p.updated_at,
			u.id, u.handle, u.display_name, u.avatar_url
		FROM posts p
		JOIN users u ON u.id = p.author_id
		WHERE p.id = $1`

	row := r.pool.QueryRow(ctx, q, id)
	p, err := scanPost(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Post{}, ErrNotFound
		}
		return Post{}, fmt.Errorf("post: get by id: %w", err)
	}
	return p, nil
}

// ListByAuthor returns a paginated list of non-deleted posts by the given author.
// Results are ordered by created_at DESC (most recent first).
// Returns (posts, nextCursor, terminated, error).
// terminated is true when the limit is reached or no further rows exist.
func (r *Repository) ListByAuthor(ctx context.Context, authorID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error) {
	// Fetch one extra row to detect whether more rows exist beyond the limit.
	fetchLimit := limit + 1

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
			WHERE p.author_id = $1
			  AND p.is_deleted = FALSE
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $2`
		rows, err = r.pool.Query(ctx, q, authorID, fetchLimit)
	} else {
		const q = `
			SELECT
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM posts p
			JOIN users u ON u.id = p.author_id
			WHERE p.author_id = $1
			  AND p.is_deleted = FALSE
			  AND (p.created_at, p.id) < ($2, $3)
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, authorID, cursor.Timestamp, cursor.AfterID, fetchLimit)
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by author query: %w", err)
	}
	defer rows.Close()

	posts, err := collectRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by author scan: %w", err)
	}

	return buildPage(posts, limit)
}

// ListThreadReplies returns a paginated list of non-deleted replies in a thread.
// Results are ordered by created_at ASC (chronological).
// Returns (posts, nextCursor, terminated, error).
func (r *Repository) ListThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, string, bool, error) {
	fetchLimit := limit + 1

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
			WHERE p.thread_root_id = $1
			  AND p.is_deleted = FALSE
			ORDER BY p.created_at ASC, p.id ASC
			LIMIT $2`
		rows, err = r.pool.Query(ctx, q, threadRootID, fetchLimit)
	} else {
		const q = `
			SELECT
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM posts p
			JOIN users u ON u.id = p.author_id
			WHERE p.thread_root_id = $1
			  AND p.is_deleted = FALSE
			  AND (p.created_at, p.id) > ($2, $3)
			ORDER BY p.created_at ASC, p.id ASC
			LIMIT $4`
		rows, err = r.pool.Query(ctx, q, threadRootID, cursor.Timestamp, cursor.AfterID, fetchLimit)
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list thread replies query: %w", err)
	}
	defer rows.Close()

	posts, err := collectRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list thread replies scan: %w", err)
	}

	return buildPage(posts, limit)
}

// SoftDelete sets is_deleted = TRUE for the post identified by postID where
// the author_id matches. Returns ErrNotFound if no row was updated (post
// does not exist or caller is not the owner).
func (r *Repository) SoftDelete(ctx context.Context, postID uuid.UUID, authorID uuid.UUID) error {
	const q = `UPDATE posts SET is_deleted = TRUE, updated_at = now() WHERE id = $1 AND author_id = $2`
	tag, err := r.pool.Exec(ctx, q, postID, authorID)
	if err != nil {
		return fmt.Errorf("post: soft delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAuthorHandleByHandle looks up the user_id for the given handle (case-insensitive).
// Returns ErrNotFound if no user matches.
func (r *Repository) GetAuthorHandleByHandle(ctx context.Context, handle string) (uuid.UUID, error) {
	const q = `SELECT id FROM users WHERE lower(handle) = lower($1)`
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, q, handle).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, ErrNotFound
		}
		return uuid.UUID{}, fmt.Errorf("post: get user id by handle: %w", err)
	}
	return id, nil
}

// GetAuthorIsPrivate returns the is_private flag for the user with the given ID.
// Returns ErrNotFound if the user does not exist.
func (r *Repository) GetAuthorIsPrivate(ctx context.Context, authorID uuid.UUID) (bool, error) {
	const q = `SELECT is_private FROM users WHERE id = $1`
	var isPrivate bool
	err := r.pool.QueryRow(ctx, q, authorID).Scan(&isPrivate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("post: get author is_private: %w", err)
	}
	return isPrivate, nil
}

// ListByHashtag returns a cursor-paginated list of non-deleted posts tagged
// with the given normalized tag (lowercase, without '#').
// Posts authored by users in blockedIDs are excluded when the slice is non-empty.
// Results are ordered by created_at DESC, id DESC (most recent first, deterministic).
// Returns (posts, nextCursor, terminated, error).
func (r *Repository) ListByHashtag(ctx context.Context, tag string, cursor *FeedCursor, limit int, blockedIDs []uuid.UUID) ([]Post, string, bool, error) {
	fetchLimit := limit + 1
	blockedArr := uuidSliceToStringSlice(blockedIDs)

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
			JOIN post_hashtags ph ON ph.post_id = p.id
			WHERE ph.tag = $1
			  AND p.is_deleted = FALSE
			  AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR p.author_id::text != ALL($2::text[]))
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $3`
		rows, err = r.pool.Query(ctx, q, tag, blockedArr, fetchLimit)
	} else {
		const q = `
			SELECT
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM posts p
			JOIN users u ON u.id = p.author_id
			JOIN post_hashtags ph ON ph.post_id = p.id
			WHERE ph.tag = $1
			  AND p.is_deleted = FALSE
			  AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR p.author_id::text != ALL($2::text[]))
			  AND (p.created_at, p.id) < ($3, $4)
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $5`
		rows, err = r.pool.Query(ctx, q, tag, blockedArr, cursor.Timestamp, cursor.AfterID, fetchLimit)
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by hashtag query: %w", err)
	}
	defer rows.Close()

	posts, err := collectRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by hashtag scan: %w", err)
	}

	return buildPage(posts, limit)
}

// uuidSliceToStringSlice converts []uuid.UUID to []string for use in pgx array
// parameters. Returns nil when the input is empty (preserving the "no filter"
// SQL behavior).
func uuidSliceToStringSlice(ids []uuid.UUID) []string {
	if len(ids) == 0 {
		return nil
	}
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = id.String()
	}
	return s
}

// ScanPostRow scans a single row from a posts JOIN users query.
// Exported so that other packages (e.g. internal/feed) can reuse the scan
// logic without duplicating it (OPEN-5).
func ScanPostRow(row pgx.Row) (Post, error) {
	return scanPost(row)
}

// ScanPostRows scans all rows from a posts JOIN users query.
// Exported so that other packages (e.g. internal/feed) can reuse the scan
// logic without duplicating it (OPEN-5).
func ScanPostRows(rows pgx.Rows) ([]Post, error) {
	return collectRows(rows)
}

// scanPost scans a single row from a posts JOIN users query.
func scanPost(row pgx.Row) (Post, error) {
	var p Post
	var authorID string
	err := row.Scan(
		&p.ID,
		&p.AuthorID,
		&p.PostType,
		&p.Content,
		&p.ParentID,
		&p.ThreadRootID,
		&p.QuotedPostID,
		&p.IsDeleted,
		&p.CreatedAt,
		&p.UpdatedAt,
		&authorID,
		&p.Author.Handle,
		&p.Author.DisplayName,
		&p.Author.AvatarURL,
	)
	if err != nil {
		return Post{}, err
	}
	p.Author.ID = authorID
	return p, nil
}

// collectRows scans all rows from a posts JOIN users query.
func collectRows(rows pgx.Rows) ([]Post, error) {
	var posts []Post
	for rows.Next() {
		var p Post
		var authorID string
		err := rows.Scan(
			&p.ID,
			&p.AuthorID,
			&p.PostType,
			&p.Content,
			&p.ParentID,
			&p.ThreadRootID,
			&p.QuotedPostID,
			&p.IsDeleted,
			&p.CreatedAt,
			&p.UpdatedAt,
			&authorID,
			&p.Author.Handle,
			&p.Author.DisplayName,
			&p.Author.AvatarURL,
		)
		if err != nil {
			return nil, err
		}
		p.Author.ID = authorID
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return posts, nil
}

// buildPage trims the extra row fetched for has-more detection and builds
// the cursor and terminated flag. It does not mutate the input slice beyond
// the capacity it already has.
func buildPage(posts []Post, limit int) ([]Post, string, bool, error) {
	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit]
	}

	terminated := !hasMore

	var nextCursor string
	if !terminated && len(posts) > 0 {
		last := posts[len(posts)-1]
		c := FeedCursor{AfterID: last.ID, Timestamp: last.CreatedAt}
		nextCursor = c.Encode()
	}

	return posts, nextCursor, terminated, nil
}
