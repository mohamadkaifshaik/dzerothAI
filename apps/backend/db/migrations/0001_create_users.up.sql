-- 0001_create_users.up.sql
--
-- Creates the users table for Dzeroth Phase 1.
--
-- ID: Application-generated UUID v7 (ADR 0004). No DEFAULT gen_random_uuid().
-- Timestamps: TIMESTAMPTZ stored in UTC (IDENTIFIERS_AND_TIME.md).
-- updated_at: Maintained by application code, not a trigger (database.md rules).
--
-- Public social-validation metrics (follower_count, following_count, like_count,
-- post_count, etc.) are permanently excluded per CLAUDE.md section 2.3.

CREATE TABLE users (
    id              UUID        NOT NULL,
    handle          TEXT        NOT NULL,
    display_name    TEXT        NOT NULL,
    email           TEXT        NOT NULL,
    email_verified  BOOLEAN     NOT NULL DEFAULT FALSE,
    password_hash   TEXT        NOT NULL,
    bio             TEXT,
    avatar_url      TEXT,
    header_url      TEXT,
    location        TEXT,
    website_url     TEXT,
    is_private      BOOLEAN     NOT NULL DEFAULT FALSE,
    is_suspended    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_pkey PRIMARY KEY (id)
);

-- Case-insensitive handle uniqueness. Handles that differ only in case are
-- treated as the same handle. Using a functional index on lower(handle) so
-- that the uniqueness check is case-insensitive while the stored value
-- preserves the user's original casing.
CREATE UNIQUE INDEX users_handle_lower_idx ON users (lower(handle));

-- Case-insensitive email uniqueness. Same rationale as handle.
CREATE UNIQUE INDEX users_email_lower_idx ON users (lower(email));

-- Index on created_at to support ordered queries and pagination by insertion
-- order. UUID v7 provides time-ordering at the ID level, but an explicit index
-- on created_at ensures query plans can use it directly without decoding the
-- UUID timestamp prefix.
CREATE INDEX users_created_at_idx ON users (created_at);
