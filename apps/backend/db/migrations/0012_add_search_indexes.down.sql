-- 0012_add_search_indexes.down.sql
--
-- Removes the trigram GIN indexes added in the up migration.
-- Drops are ordered from most-recently-created to least-recently-created
-- to mirror the reverse of the up migration.
--
-- pg_trgm extension is intentionally NOT dropped: other indexes or
-- extensions in the database may depend on it, and dropping it could
-- cause unrelated failures. The extension is lightweight and safe to
-- leave installed.

DROP INDEX IF EXISTS users_display_name_trgm_idx;
DROP INDEX IF EXISTS users_handle_trgm_idx;
DROP INDEX IF EXISTS posts_content_trgm_idx;
