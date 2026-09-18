-- 0009_add_repost_uniqueness.up.sql
--
-- Adds a partial unique index on posts to enforce one active repost per
-- (author_id, quoted_post_id) pair.
--
-- Partial index rationale: the WHERE clause restricts uniqueness to rows
-- where post_type = 'repost' AND is_deleted = FALSE. Soft-deleted repost
-- rows are excluded so that a user can re-repost a post after deleting their
-- original repost without being blocked by the uniqueness constraint.
--
-- This is the authoritative server-side enforcement of the one-repost-per-
-- author rule. Application-layer validation must agree with this constraint
-- but does not replace it (CLAUDE.md §3, rule 6).
--
-- The posts table and post_type ENUM were created in migration 0004.
-- The columns author_id, quoted_post_id, post_type, and is_deleted are all
-- confirmed present in the posts table.

CREATE UNIQUE INDEX posts_repost_unique_per_author
    ON posts (author_id, quoted_post_id)
    WHERE post_type = 'repost' AND is_deleted = FALSE;
