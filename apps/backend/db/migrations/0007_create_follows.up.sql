-- 0007_create_follows.up.sql
--
-- Creates the follows table for Dzeroth Phase 3 (Social Graph).
--
-- No surrogate primary key: the (follower_id, followed_id) pair is the
-- natural key and prevents duplicate follow relationships at the database
-- layer. The composite PK implicitly covers the forward direction lookup
-- "does user A follow user B?".
--
-- Self-follow CHECK constraint prevents a user from following themselves.
-- Enforced at the database layer independent of application validation
-- (database.md rules).
--
-- ON DELETE CASCADE on both FKs: when a user account is deleted, all follow
-- relationships in which they participate are automatically removed.
--
-- Public social-validation metrics (follower_count, following_count) are
-- permanently excluded per CLAUDE.md section 2.3.

CREATE TABLE follows (
    follower_id  UUID        NOT NULL,
    followed_id  UUID        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT follows_pkey
        PRIMARY KEY (follower_id, followed_id),

    CONSTRAINT follows_follower_id_fkey
        FOREIGN KEY (follower_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT follows_followed_id_fkey
        FOREIGN KEY (followed_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT follows_no_self_follow
        CHECK (follower_id <> followed_id)
);

-- Covers fan-in: "who follows user B?" (needed for home timeline query).
-- The composite PK index already covers the forward direction
-- (follower_id, followed_id); this index covers the reverse direction
-- with recency ordering to support paginated follower listings.
CREATE INDEX follows_followed_id_created_at_idx ON follows (followed_id, created_at DESC);
