-- 0006_create_post_hashtags.up.sql
--
-- Creates the post_hashtags table for Dzeroth Phase 2
-- (Posts and Conversations).
--
-- Hashtags are extracted and normalized by the application layer at write time.
-- Normalization rules:
--   - lowercased before insertion
--   - the leading '#' character is NOT stored; only the tag body is stored
--   - the database enforces lowercase via CHECK so any application bug that
--     inserts non-lowercase values is rejected at the database layer
--
-- No surrogate primary key: (post_id, tag) is the natural key. The composite
-- PK prevents the same tag from appearing twice on the same post.
--
-- ON DELETE CASCADE on post_id: when a post is hard-deleted (outside normal
-- application paths), its hashtag associations are automatically removed.
--
-- 100 code-point tag length limit: char_length() counts Unicode code points.
-- This is enforced at the database layer independent of application validation.

CREATE TABLE post_hashtags (
    post_id     UUID        NOT NULL,
    tag         TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT post_hashtags_pkey
        PRIMARY KEY (post_id, tag),

    CONSTRAINT post_hashtags_post_id_fkey
        FOREIGN KEY (post_id) REFERENCES posts (id) ON DELETE CASCADE,

    -- Enforces lowercase normalization at the database layer. Any application
    -- path that inserts a non-lowercase tag value will be rejected.
    CONSTRAINT post_hashtags_tag_lowercase
        CHECK (tag = lower(tag)),

    -- Reasonable upper bound on tag length (code points).
    CONSTRAINT post_hashtags_tag_length
        CHECK (char_length(tag) <= 100)
);

-- Hashtag timeline queries: "fetch posts tagged with <tag> ordered by
-- recency." The composite index (tag, created_at DESC) satisfies this query
-- with a single index scan. The composite PK covers the forward direction
-- (post_id, tag); this index covers the reverse direction.
CREATE INDEX post_hashtags_tag_created_at_idx ON post_hashtags (tag, created_at DESC);
