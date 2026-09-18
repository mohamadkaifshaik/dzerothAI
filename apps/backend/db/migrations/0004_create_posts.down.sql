-- 0004_create_posts.down.sql
--
-- Reverses 0004_create_posts.up.sql.
-- Indexes are dropped before the table for explicit ordering clarity.
-- The table must be dropped before the post_type ENUM because the ENUM is
-- referenced by the table's post_type column.

DROP INDEX IF EXISTS posts_created_at_idx;
DROP INDEX IF EXISTS posts_parent_id_idx;
DROP INDEX IF EXISTS posts_thread_root_id_created_at_idx;
DROP INDEX IF EXISTS posts_author_id_created_at_idx;

DROP TABLE IF EXISTS posts;

DROP TYPE IF EXISTS post_type;
