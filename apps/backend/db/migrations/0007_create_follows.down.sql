-- 0007_create_follows.down.sql
--
-- Reverses 0007_create_follows.up.sql.
-- Index is dropped before the table for explicit ordering clarity.

DROP INDEX IF EXISTS follows_followed_id_created_at_idx;

DROP TABLE IF EXISTS follows;
