-- 0002_create_sessions.up.sql
--
-- Creates the sessions table for database-backed refresh tokens.
-- Per ADR 0005: JWT access tokens are stateless; refresh tokens are opaque
-- 32-byte values whose SHA-256 hash is stored here. The raw token value is
-- never persisted.
--
-- ID: Application-generated UUID v7. Separate from the JWT jti claim (ADR 0005).
-- Rotation: on every successful /auth/refresh the row is updated in-transaction.
-- Revocation: DELETE the row on logout or suspension detection.
-- Cleanup: expired rows are pruned by the application; expires_at index supports
--   efficient range scans for the cleanup query.

CREATE TABLE sessions (
    id            UUID        NOT NULL,
    user_id       UUID        NOT NULL,
    token_hash    TEXT        NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip_address    TEXT,
    user_agent    TEXT,

    CONSTRAINT sessions_pkey PRIMARY KEY (id),
    CONSTRAINT sessions_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Enforces one hash per row and is the lookup key for token verification.
-- Every /auth/refresh performs a lookup by token_hash; this index is the
-- primary hot path for authenticated refresh requests.
CREATE UNIQUE INDEX sessions_token_hash_idx ON sessions (token_hash);

-- Supports efficient lookup of all sessions belonging to a user (e.g., for
-- "log out all devices" or account suspension).
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- Supports efficient range scan when pruning expired sessions. Without this
-- index a cleanup query would require a full table scan as the sessions table
-- grows.
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
