-- 0004_create_posts.up.sql
--
-- Creates the post_type ENUM and posts table for Dzeroth Phase 2
-- (Posts and Conversations).
--
-- post_type ENUM: created before the table; must be dropped after in down.
--
-- ID: Application-generated UUID v7 (ADR 0004). No DEFAULT gen_random_uuid().
-- Timestamps: TIMESTAMPTZ stored in UTC (IDENTIFIERS_AND_TIME.md).
-- updated_at: Maintained by application code, not a trigger (database.md rules).
--
-- Soft delete: rows are never physically removed. is_deleted = TRUE hides a
-- post from feeds while preserving reply chain integrity and FK references.
--
-- content is nullable to accommodate reposts, which carry no authored text.
-- CHECK constraints enforce that each post_type has the correct nullable/
-- non-nullable content and reference columns. These are the authoritative
-- server-side enforcement points (CLAUDE.md §3, rule 6).
--
-- 500 Unicode code-point limit: char_length() counts Unicode code points, not
-- bytes. This is the single authoritative constraint; Go service validation and
-- API request validation must agree with this value.
--
-- thread_root_id denormalization: all replies in a thread carry the root post's
-- id so that bounded thread queries (CLAUDE.md §2.1) can be issued with a
-- single index scan rather than a recursive CTE walk.
--
-- Public social-validation metrics (like_count, impression_count,
-- bookmark_count, share_count, reply_count, etc.) are permanently excluded
-- per CLAUDE.md section 2.3.

-- -------------------------------------------------------------------------
-- post_type ENUM
-- -------------------------------------------------------------------------

CREATE TYPE post_type AS ENUM ('original', 'reply', 'quote', 'repost');

-- -------------------------------------------------------------------------
-- posts
-- -------------------------------------------------------------------------

CREATE TABLE posts (
    id              UUID        NOT NULL,
    author_id       UUID        NOT NULL,
    post_type       post_type   NOT NULL DEFAULT 'original',
    content         TEXT,
    parent_id       UUID,
    thread_root_id  UUID,
    quoted_post_id  UUID,
    is_deleted      BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT posts_pkey
        PRIMARY KEY (id),

    CONSTRAINT posts_author_id_fkey
        FOREIGN KEY (author_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT posts_parent_id_fkey
        FOREIGN KEY (parent_id) REFERENCES posts (id) ON DELETE SET NULL,

    CONSTRAINT posts_thread_root_id_fkey
        FOREIGN KEY (thread_root_id) REFERENCES posts (id) ON DELETE SET NULL,

    CONSTRAINT posts_quoted_post_id_fkey
        FOREIGN KEY (quoted_post_id) REFERENCES posts (id) ON DELETE SET NULL,

    -- Authoritative 500 Unicode code-point content limit.
    -- char_length() counts code points, not bytes.
    CONSTRAINT posts_content_length
        CHECK (char_length(content) <= 500),

    -- Reposts carry no authored text.
    CONSTRAINT posts_repost_no_content
        CHECK (post_type <> 'repost' OR content IS NULL),

    -- Original posts must have content.
    CONSTRAINT posts_original_has_content
        CHECK (post_type <> 'original' OR content IS NOT NULL),

    -- Replies must have content.
    CONSTRAINT posts_reply_has_content
        CHECK (post_type NOT IN ('reply', 'quote') OR content IS NOT NULL),

    -- Replies must reference a parent post.
    CONSTRAINT posts_reply_has_parent
        CHECK (post_type <> 'reply' OR parent_id IS NOT NULL),

    -- Quote posts must reference the quoted post.
    CONSTRAINT posts_quote_has_quoted_post
        CHECK (post_type <> 'quote' OR quoted_post_id IS NOT NULL),

    -- Reposts must reference the original post via quoted_post_id.
    CONSTRAINT posts_repost_has_quoted_post
        CHECK (post_type <> 'repost' OR quoted_post_id IS NOT NULL),

    -- Prevents self-reply.
    CONSTRAINT posts_no_self_reply
        CHECK (parent_id <> id),

    -- Prevents self-quote.
    CONSTRAINT posts_no_self_quote
        CHECK (quoted_post_id <> id)
);

-- Author timeline pagination: fetch all posts by a given author ordered by
-- recency. The DESC ordering on created_at matches the expected query shape
-- for a chronologically reversed profile timeline.
CREATE INDEX posts_author_id_created_at_idx ON posts (author_id, created_at DESC);

-- Thread reply listing: fetch all replies belonging to a thread root in
-- chronological (ASC) order. Required for bounded thread display (CLAUDE.md
-- §2.1 — no infinite scrolling; queries use LIMIT against this index).
CREATE INDEX posts_thread_root_id_created_at_idx ON posts (thread_root_id, created_at ASC);

-- Direct reply lookup: find all immediate children of a given post. Used when
-- rendering the direct-reply layer beneath a post.
CREATE INDEX posts_parent_id_idx ON posts (parent_id);

-- General feed ordering: supports home-timeline and discovery feed queries
-- that sort across authors by recency. Partial queries and range scans against
-- created_at benefit from this index.
CREATE INDEX posts_created_at_idx ON posts (created_at DESC);
