-- 0009_add_repost_uniqueness.down.sql
--
-- Reverses 0009_add_repost_uniqueness.up.sql.

DROP INDEX IF EXISTS posts_repost_unique_per_author;
