-- 0005_create_post_mentions.up.sql
--
-- Creates the post_mentions table for Dzeroth Phase 2
-- (Posts and Conversations).
--
-- Mentions are extracted by the application layer at write time.
-- The application resolves @handle → user_id before inserting.
-- Storing the resolved user_id (not the raw handle string) keeps the mention
-- record durable against handle renames and enables efficient notification
-- fan-out by user_id.
--
-- No surrogate primary key: the (post_id, user_id) pair is the natural key.
-- The composite PK prevents a post from mentioning the same user twice.
--
-- ON DELETE CASCADE on both FKs: when a post is hard-deleted (outside normal
-- application paths) or a user account is deleted, related mention rows are
-- automatically removed.

CREATE TABLE post_mentions (
    post_id     UUID        NOT NULL,
    user_id     UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT post_mentions_pkey
        PRIMARY KEY (post_id, user_id),

    CONSTRAINT post_mentions_post_id_fkey
        FOREIGN KEY (post_id) REFERENCES posts (id) ON DELETE CASCADE,

    CONSTRAINT post_mentions_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Covers "which posts mention this user?" — the primary query shape for
-- notification fan-out. The composite PK index already covers the forward
-- direction (post_id, user_id), so only the reverse direction needs a
-- dedicated index.
CREATE INDEX post_mentions_user_id_idx ON post_mentions (user_id);
