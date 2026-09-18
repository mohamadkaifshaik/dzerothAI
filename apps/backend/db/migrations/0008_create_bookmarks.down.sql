-- 0008_create_bookmarks.down.sql
--
-- Reverses 0008_create_bookmarks.up.sql.
-- Index is dropped before the table for explicit ordering clarity.

DROP INDEX IF EXISTS bookmarks_user_id_created_at_idx;

DROP TABLE IF EXISTS bookmarks;
