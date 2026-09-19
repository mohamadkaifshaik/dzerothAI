package search

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// maxSearchResults is the server-enforced hard limit on search result pages.
// The client must stop fetching when terminated=true is received (CLAUDE.md §2.1).
const maxSearchResults = 50

// Repository provides data-access methods for the search domain.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository constructs a search Repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// SearchPosts returns a cursor-paginated list of non-deleted posts whose content
// matches the query string (case-insensitive substring match).
//
// Posts authored by users in blockedIDs are excluded when the slice is non-empty.
// Results are ordered by created_at DESC, id DESC (deterministic).
func (r *Repository) SearchPosts(ctx context.Context, query string, cursor string, blockedIDs []uuid.UUID) (PostSearchPage, error) {
	var fc *post.FeedCursor
	if cursor != "" {
		c, err := post.DecodeCursor(cursor)
		if err != nil {
			return PostSearchPage{}, fmt.Errorf("search: decode post cursor: %w", err)
		}
		fc = c
	}

	blockedArr := uuidSliceToStringSlice(blockedIDs)
	fetchLimit := maxSearchResults + 1

	var (
		pgxRows pgx.Rows
		err     error
	)

	if fc == nil {
		const q = `
			SELECT
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM posts p
			JOIN users u ON u.id = p.author_id
			WHERE p.content ILIKE '%' || $1 || '%'
			  AND p.is_deleted = FALSE
			  AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR p.author_id::text != ALL($2::text[]))
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $3`
		pgxRows, err = r.db.Query(ctx, q, query, blockedArr, fetchLimit)
	} else {
		const q = `
			SELECT
				p.id, p.author_id, p.post_type, p.content,
				p.parent_id, p.thread_root_id, p.quoted_post_id,
				p.is_deleted, p.created_at, p.updated_at,
				u.id, u.handle, u.display_name, u.avatar_url
			FROM posts p
			JOIN users u ON u.id = p.author_id
			WHERE p.content ILIKE '%' || $1 || '%'
			  AND p.is_deleted = FALSE
			  AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR p.author_id::text != ALL($2::text[]))
			  AND (p.created_at < $4 OR (p.created_at = $4 AND p.id < $5))
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT $3`
		pgxRows, err = r.db.Query(ctx, q, query, blockedArr, fetchLimit, fc.Timestamp, fc.AfterID)
	}
	if err != nil {
		return PostSearchPage{}, fmt.Errorf("search: search posts query: %w", err)
	}
	defer pgxRows.Close()

	rawPosts, err := post.ScanPostRows(pgxRows)
	if err != nil {
		return PostSearchPage{}, fmt.Errorf("search: search posts scan: %w", err)
	}

	trimmed, nextCursor, terminated := buildPostPage(rawPosts, maxSearchResults)

	dtos := make([]post.PostDTO, len(trimmed))
	for i, p := range trimmed {
		dtos[i] = post.ToDTO(p)
	}
	if dtos == nil {
		dtos = []post.PostDTO{}
	}

	return PostSearchPage{
		Items:      dtos,
		NextCursor: nextCursor,
		Terminated: terminated,
	}, nil
}

// SearchUsers returns a cursor-paginated list of users whose handle or
// display_name matches the query string (case-insensitive substring match).
//
// Users in blockedIDs are excluded when the slice is non-empty.
// Results are ordered by handle ASC, id ASC (deterministic).
func (r *Repository) SearchUsers(ctx context.Context, query string, cursor string, blockedIDs []uuid.UUID) (UserSearchPage, error) {
	var uc *userCursor
	if cursor != "" {
		c, err := decodeUserCursor(cursor)
		if err != nil {
			return UserSearchPage{}, fmt.Errorf("search: decode user cursor: %w", err)
		}
		uc = c
	}

	blockedArr := uuidSliceToStringSlice(blockedIDs)
	fetchLimit := maxSearchResults + 1

	var (
		pgxRows pgx.Rows
		err     error
	)

	if uc == nil {
		const q = `
			SELECT u.id, u.handle, u.display_name, u.avatar_url
			FROM users u
			WHERE (
				lower(u.handle) LIKE '%' || lower($1) || '%'
				OR lower(u.display_name) LIKE '%' || lower($1) || '%'
			)
			AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR u.id::text != ALL($2::text[]))
			ORDER BY u.handle ASC, u.id ASC
			LIMIT $3`
		pgxRows, err = r.db.Query(ctx, q, query, blockedArr, fetchLimit)
	} else {
		const q = `
			SELECT u.id, u.handle, u.display_name, u.avatar_url
			FROM users u
			WHERE (
				lower(u.handle) LIKE '%' || lower($1) || '%'
				OR lower(u.display_name) LIKE '%' || lower($1) || '%'
			)
			AND ($2::text[] IS NULL OR array_length($2::text[], 1) IS NULL OR u.id::text != ALL($2::text[]))
			AND (u.handle > $4 OR (u.handle = $4 AND u.id::text > $5))
			ORDER BY u.handle ASC, u.id ASC
			LIMIT $3`
		pgxRows, err = r.db.Query(ctx, q, query, blockedArr, fetchLimit, uc.AfterHandle, uc.AfterID)
	}
	if err != nil {
		return UserSearchPage{}, fmt.Errorf("search: search users query: %w", err)
	}
	defer pgxRows.Close()

	users, err := collectUserRows(pgxRows)
	if err != nil {
		return UserSearchPage{}, fmt.Errorf("search: search users scan: %w", err)
	}

	items, nextCursor, terminated := buildUserPage(users, maxSearchResults)

	if items == nil {
		items = []UserSearchResult{}
	}

	return UserSearchPage{
		Items:      items,
		NextCursor: nextCursor,
		Terminated: terminated,
	}, nil
}

// collectUserRows scans all rows from a users search query into UserSearchResult values.
func collectUserRows(rows pgx.Rows) ([]UserSearchResult, error) {
	var users []UserSearchResult
	for rows.Next() {
		var u UserSearchResult
		if err := rows.Scan(&u.ID, &u.Handle, &u.DisplayName, &u.AvatarURL); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// buildPostPage applies max+1 has-more detection, builds the cursor, and sets
// the terminated flag using post.FeedCursor.
func buildPostPage(posts []post.Post, max int) ([]post.Post, string, bool) {
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

	return posts, nextCursor, terminated
}

// buildUserPage applies max+1 has-more detection, builds the cursor, and sets
// the terminated flag.
func buildUserPage(users []UserSearchResult, max int) ([]UserSearchResult, string, bool) {
	hasMore := len(users) > max
	if hasMore {
		users = users[:max]
	}

	terminated := !hasMore

	var nextCursor string
	if !terminated && len(users) > 0 {
		last := users[len(users)-1]
		c := userCursor{AfterHandle: last.Handle, AfterID: last.ID}
		nextCursor = c.encode()
	}

	return users, nextCursor, terminated
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
