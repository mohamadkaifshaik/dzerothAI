-- 0012_add_search_indexes.up.sql
--
-- Adds pg_trgm extension and trigram GIN indexes to support fast
-- full-text ILIKE/similarity search on posts.content, users.handle,
-- and users.display_name.
--
-- pg_trgm: PostgreSQL extension that adds trigram-based text similarity
-- operators and index support. Required for GIN indexes using gin_trgm_ops.
-- CREATE EXTENSION IF NOT EXISTS is idempotent and safe to re-run.
--
-- posts_content_trgm_idx: partial index (WHERE is_deleted = FALSE) to
-- exclude soft-deleted posts from search, matching the authoritative
-- soft-delete convention established in migration 0004. content is nullable
-- on the posts table (reposts carry no text); NULL rows are excluded
-- automatically by GIN indexes.
--
-- users_handle_trgm_idx / users_display_name_trgm_idx: functional GIN
-- indexes on lower(handle) and lower(display_name) to enable
-- case-insensitive trigram search. lower() is consistent with the
-- existing users_handle_lower_idx unique index from migration 0001.
--
-- No -- migrate: notransaction annotation: these are plain CREATE INDEX
-- (not CONCURRENTLY) and are fully compatible with golang-migrate's
-- default transaction wrapping.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX posts_content_trgm_idx ON posts USING GIN (content gin_trgm_ops)
    WHERE is_deleted = FALSE;

CREATE INDEX users_handle_trgm_idx ON users USING GIN (lower(handle) gin_trgm_ops);

CREATE INDEX users_display_name_trgm_idx ON users USING GIN (lower(display_name) gin_trgm_ops);
