-- 0005_create_post_mentions.down.sql
--
-- Reverses 0005_create_post_mentions.up.sql.
-- Index dropped before the table for explicit ordering clarity.

DROP INDEX IF EXISTS post_mentions_user_id_idx;

DROP TABLE IF EXISTS post_mentions;
