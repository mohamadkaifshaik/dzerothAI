-- 0006_create_post_hashtags.down.sql
--
-- Reverses 0006_create_post_hashtags.up.sql.
-- Index dropped before the table for explicit ordering clarity.

DROP INDEX IF EXISTS post_hashtags_tag_created_at_idx;

DROP TABLE IF EXISTS post_hashtags;
