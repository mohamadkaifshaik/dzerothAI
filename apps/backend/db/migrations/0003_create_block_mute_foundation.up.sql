-- 0003_create_block_mute_foundation.up.sql
--
-- Creates the blocks and mutes tables as schema foundations for F-022.
-- Per FEATURE_REGISTRY.md: Phase 1 establishes the schema; full UI and
-- enforcement is Phase 5.
--
-- No surrogate primary key: the relationship itself is the entity. The
-- composite PK (blocker_id, blocked_id) and (muter_id, muted_id) is the
-- natural key and prevents duplicate rows at the database layer.
--
-- Self-relationship CHECK constraints prevent a user from blocking or muting
-- themselves. This is enforced at the database layer independent of
-- application validation (database.md rules).
--
-- ON DELETE CASCADE: when a user account is deleted, all block/mute
-- relationships in which they participate are automatically removed.

-- -------------------------------------------------------------------------
-- blocks
-- -------------------------------------------------------------------------

CREATE TABLE blocks (
    blocker_id  UUID        NOT NULL,
    blocked_id  UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT blocks_pkey
        PRIMARY KEY (blocker_id, blocked_id),

    CONSTRAINT blocks_blocker_id_fkey
        FOREIGN KEY (blocker_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT blocks_blocked_id_fkey
        FOREIGN KEY (blocked_id) REFERENCES users (id) ON DELETE CASCADE,

    -- Prevents self-blocking. Enforced at the database layer.
    CONSTRAINT blocks_no_self_block
        CHECK (blocker_id <> blocked_id)
);

-- The composite PK index covers (blocker_id, blocked_id) lookups
-- ("has user A blocked user B?"). A separate index on blocked_id covers
-- the reverse direction: "which users has user B been blocked by?".
-- This is needed for feed filtering and profile visibility checks.
CREATE INDEX blocks_blocked_id_idx ON blocks (blocked_id);

-- -------------------------------------------------------------------------
-- mutes
-- -------------------------------------------------------------------------

CREATE TABLE mutes (
    muter_id    UUID        NOT NULL,
    muted_id    UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT mutes_pkey
        PRIMARY KEY (muter_id, muted_id),

    CONSTRAINT mutes_muter_id_fkey
        FOREIGN KEY (muter_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT mutes_muted_id_fkey
        FOREIGN KEY (muted_id) REFERENCES users (id) ON DELETE CASCADE,

    -- Prevents self-muting. Enforced at the database layer.
    CONSTRAINT mutes_no_self_mute
        CHECK (muter_id <> muted_id)
);

-- Same rationale as blocks_blocked_id_idx: covers the reverse lookup
-- "which users has user B been muted by?".
CREATE INDEX mutes_muted_id_idx ON mutes (muted_id);
