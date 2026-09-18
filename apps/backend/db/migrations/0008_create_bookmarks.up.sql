-- 0008_create_bookmarks.up.sql
--
-- Creates the bookmarks table for Dzeroth Phase 3.
--
-- No surrogate primary key: the (user_id, post_id) pair is the natural key
-- and prevents a user from bookmarking the same post more than once at the
-- database layer.
--
-- ON DELETE CASCADE on both FKs: when a user account is deleted all of their
-- bookmarks are automatically removed; when a post is hard-deleted (outside
-- normal application soft-delete paths) its bookmark associations are
-- automatically removed.
--
-- Bookmark counts are a private analytics metric and must never be exposed
-- on public DTOs per CLAUDE.md section 2.3. No bookmark_count column is
-- added to any table.

CREATE TABLE bookmarks (
    user_id     UUID        NOT NULL,
    post_id     UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT bookmarks_pkey
        PRIMARY KEY (user_id, post_id),

    CONSTRAINT bookmarks_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT bookmarks_post_id_fkey
        FOREIGN KEY (post_id) REFERENCES posts (id) ON DELETE CASCADE
);

-- Covers: "fetch all bookmarks for a user, newest first."
-- The composite PK index covers the existence check (user_id, post_id);
-- this index covers the paginated listing query shape.
CREATE INDEX bookmarks_user_id_created_at_idx ON bookmarks (user_id, created_at DESC);
