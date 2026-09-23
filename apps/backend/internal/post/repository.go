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
			u.id, u.handle, u.display_name, u.avatar_url,
			td.slug, td.display_name
		FROM posts p
		JOIN users u ON u.id = p.author_id
		LEFT JOIN user_titles ut ON ut.id = u.primary_title_id
		LEFT JOIN title_definitions td ON td.id = ut.title_definition_id
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

// cursorArgs returns the (timestamp, id) SQL arguments for an optional cursor.
// Both are untyped nil (SQL NULL) when cursor is nil, which the window queries
// treat as "start at the top of the window".
func cursorArgs(cursor *FeedCursor) (any, any) {
	if cursor == nil {
		return nil, nil
	}
	return cursor.Timestamp, cursor.AfterID
}

// ListByAuthor returns one page of the author's non-deleted posts, ordered by
// created_at DESC, id DESC (most recent first).
//
// Hard depth (CLAUDE.md §2.1, ADR 0006): the window CTE selects the author's
// current top-maxDepth eligible posts first; the cursor then pages inside that
// window only, pageSize rows at a time. No cursor — valid, forged, or stale —
// can reach a post ranked below maxDepth. A cursor that lies past the end of
// the current window yields an empty, terminated page.
// Returns (posts, nextCursor, terminated, error).
func (r *Repository) ListByAuthor(ctx context.Context, authorID uuid.UUID, cursor *FeedCursor, pageSize, maxDepth int) ([]Post, string, bool, error) {
	const q = `
		WITH win AS (
			SELECT p.id, p.created_at
			FROM posts p
			WHERE p.author_id = $1
			  AND p.is_deleted = FALSE
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $2
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
		WHERE $3::timestamptz IS NULL OR (w.created_at, w.id) < ($3::timestamptz, $4::uuid)
		ORDER BY w.created_at DESC, w.id DESC
		LIMIT $5`

	cursorTS, cursorID := cursorArgs(cursor)
	// Fetch one extra row (within the window) to detect whether more rows exist.
	rows, err := r.pool.Query(ctx, q, authorID, maxDepth, cursorTS, cursorID, pageSize+1)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by author query: %w", err)
	}
	defer rows.Close()

	posts, err := collectRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by author scan: %w", err)
	}

	return buildPage(posts, pageSize)
}

// ListThreadReplies returns one page of non-deleted replies in a thread,
// ordered by created_at ASC, id ASC (chronological).
//
// Hard depth: the window CTE selects the thread's first maxDepth eligible
// replies (in chronological order); the cursor pages inside that window only,
// pageSize rows at a time. A cursor past the end of the window yields an empty,
// terminated page. Returns (posts, nextCursor, terminated, error).
func (r *Repository) ListThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursor *FeedCursor, pageSize, maxDepth int) ([]Post, string, bool, error) {
	const q = `
		WITH win AS (
			SELECT p.id, p.created_at
			FROM posts p
			WHERE p.thread_root_id = $1
			  AND p.is_deleted = FALSE
			ORDER BY p.created_at ASC, p.id ASC
			LIMIT $2
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
		WHERE $3::timestamptz IS NULL OR (w.created_at, w.id) > ($3::timestamptz, $4::uuid)
		ORDER BY w.created_at ASC, w.id ASC
		LIMIT $5`

	cursorTS, cursorID := cursorArgs(cursor)
	rows, err := r.pool.Query(ctx, q, threadRootID, maxDepth, cursorTS, cursorID, pageSize+1)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list thread replies query: %w", err)
	}
	defer rows.Close()

	posts, err := collectRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list thread replies scan: %w", err)
	}

	return buildPage(posts, pageSize)
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

// ListByHashtag returns one page of non-deleted posts tagged with the given
// normalized tag (lowercase, without '#'). Posts authored by users in
// blockedIDs are excluded when the slice is non-empty. Results are ordered by
// created_at DESC, id DESC (most recent first, deterministic).
//
// Hard depth: the window CTE applies the tag, soft-delete, and block filters
// first and keeps the current top-maxDepth eligible posts, so filtered rows
// never consume the window. The cursor pages inside the window only, pageSize
// rows at a time; a cursor past the end of the window yields an empty,
// terminated page. Returns (posts, nextCursor, terminated, error).
func (r *Repository) ListByHashtag(ctx context.Context, tag string, cursor *FeedCursor, pageSize, maxDepth int, blockedIDs []uuid.UUID) ([]Post, string, bool, error) {
	const q = `
		WITH win AS (
			SELECT p.id, p.created_at
			FROM posts p
			JOIN post_hashtags ph ON ph.post_id = p.id
			WHERE ph.tag = $1
			  AND p.is_deleted = FALSE
			  AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR p.author_id::text != ALL($2::text[]))
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $3
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
		WHERE $4::timestamptz IS NULL OR (w.created_at, w.id) < ($4::timestamptz, $5::uuid)
		ORDER BY w.created_at DESC, w.id DESC
		LIMIT $6`

	blockedArr := uuidSliceToStringSlice(blockedIDs)
	cursorTS, cursorID := cursorArgs(cursor)
	rows, err := r.pool.Query(ctx, q, tag, blockedArr, maxDepth, cursorTS, cursorID, pageSize+1)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by hashtag query: %w", err)
	}
	defer rows.Close()

	posts, err := collectRows(rows)
	if err != nil {
		return nil, "", false, fmt.Errorf("post: list by hashtag scan: %w", err)
	}

	return buildPage(posts, pageSize)
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

// scanPost scans a single row from a posts JOIN users LEFT JOIN title_definitions query.
// Column order (16 total):
//  1. p.id
//  2. p.author_id
//  3. p.post_type
//  4. p.content
//  5. p.parent_id
//  6. p.thread_root_id
//  7. p.quoted_post_id
//  8. p.is_deleted
//  9. p.created_at
//  10. p.updated_at
//  11. u.id
//  12. u.handle
//  13. u.display_name
//  14. u.avatar_url
//  15. td.slug        (nullable — NULL when no primary title)
//  16. td.display_name (nullable — NULL when no primary title)
func scanPost(row pgx.Row) (Post, error) {
	var p Post
	var authorID string
	var titleSlug *string
	var titleDisplayName *string
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
		&titleSlug,
		&titleDisplayName,
	)
	if err != nil {
		return Post{}, err
	}
	p.Author.ID = authorID
	if titleSlug != nil && titleDisplayName != nil {
		p.Author.PrimaryTitle = &PostAuthorTitle{
			Slug:        *titleSlug,
			DisplayName: *titleDisplayName,
		}
	}
	return p, nil
}

// collectRows scans all rows from a posts JOIN users LEFT JOIN title_definitions query.
func collectRows(rows pgx.Rows) ([]Post, error) {
	var posts []Post
	for rows.Next() {
		var p Post
		var authorID string
		var titleSlug *string
		var titleDisplayName *string
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
			&titleSlug,
			&titleDisplayName,
		)
		if err != nil {
			return nil, err
		}
		p.Author.ID = authorID
		if titleSlug != nil && titleDisplayName != nil {
			p.Author.PrimaryTitle = &PostAuthorTitle{
				Slug:        *titleSlug,
				DisplayName: *titleDisplayName,
			}
		}
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
//
// posts must be at most limit+1 rows taken from inside the endpoint's
// top-maxDepth window, so "no extra row" means the window is exhausted:
// terminated=true and an empty cursor.
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
