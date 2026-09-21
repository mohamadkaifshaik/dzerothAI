-- 0015_create_user_titles.up.sql
--
-- Creates the user_titles table for the Dzeroth Title System (Phase 1, v1).
--
-- ID: Application-generated UUID v7 (ADR 0004). No DEFAULT gen_random_uuid().
-- Timestamps: TIMESTAMPTZ stored in UTC (IDENTIFIERS_AND_TIME.md).
--
-- status lifecycle: active → grace_period → revoked
--   'active'       — title is currently held and publicly displayable.
--   'grace_period' — title criteria no longer met; user has a grace window
--                    before the title is formally revoked. grace_period_ends_at
--                    must be set when status is 'grace_period'.
--   'revoked'      — title was removed. revoked_at must be set.
--
-- ON DELETE CASCADE on user_id: removing a user removes all their title rows.
-- ON DELETE RESTRICT on title_definition_id: title definitions cannot be
--   deleted while any user_title references them (hard requirement — use
--   is_active = FALSE on the definition to retire a title type instead).
--
-- Deduplication index (user_titles_one_active_per_def_idx): prevents a user
-- from holding two active/grace-period instances of the same title definition
-- simultaneously. Revoked rows are excluded from the partial unique index so
-- that a user can re-earn a title after revocation without a constraint error.

CREATE TABLE user_titles (
    id                       UUID        NOT NULL,
    user_id                  UUID        NOT NULL,
    title_definition_id      UUID        NOT NULL,
    status                   VARCHAR(20) NOT NULL DEFAULT 'active',
    grace_period_ends_at     TIMESTAMPTZ,
    grace_notification_sent  BOOLEAN     NOT NULL DEFAULT FALSE,
    unlock_notification_sent BOOLEAN     NOT NULL DEFAULT FALSE,
    revoked_at               TIMESTAMPTZ,
    unlocked_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT user_titles_pkey
        PRIMARY KEY (id),

    CONSTRAINT user_titles_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE CASCADE,

    CONSTRAINT user_titles_title_definition_id_fkey
        FOREIGN KEY (title_definition_id) REFERENCES title_definitions (id)
        ON DELETE RESTRICT,

    CONSTRAINT user_titles_status_values
        CHECK (status IN ('active', 'grace_period', 'revoked')),

    CONSTRAINT user_titles_revoked_requires_timestamp
        CHECK (status != 'revoked' OR revoked_at IS NOT NULL),

    CONSTRAINT user_titles_grace_requires_end_timestamp
        CHECK (status != 'grace_period' OR grace_period_ends_at IS NOT NULL)
);

CREATE INDEX user_titles_user_id_idx
    ON user_titles (user_id);

-- Prevents duplicate active or grace-period instances for the same user and
-- title definition. Revoked rows are excluded from this index, which allows
-- re-earning a title after revocation.
CREATE UNIQUE INDEX user_titles_one_active_per_def_idx
    ON user_titles (user_id, title_definition_id)
    WHERE status IN ('active', 'grace_period');
