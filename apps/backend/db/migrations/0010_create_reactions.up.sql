-- 0010_create_reactions.up.sql
--
-- Creates the reactions table for Dzeroth Phase 4 (Wave 1B).
--
-- No surrogate primary key: the (user_id, post_id) composite PK is the
-- natural idempotency key and enforces one reaction per user per post at
-- the database layer. Application-layer upsert paths must agree with this
-- constraint but do not replace it (CLAUDE.md §3, rule 6).
--
-- reaction_type: VARCHAR(20) with DEFAULT 'like'. No CHECK constraint is
-- applied here to preserve forward flexibility for additional reaction types.
-- This is intentional per migration specification.
--
-- ON DELETE CASCADE on both FKs: when a user account is deleted all of their
-- reactions are automatically removed; when a post is hard-deleted (outside
-- normal application soft-delete paths) its reaction associations are
-- automatically removed.
--
-- Reaction counts (like_count, etc.) are a private analytics metric and must
-- never be exposed on public DTOs per CLAUDE.md section 2.3. No aggregate
-- count column is added to any table.
--
-- ID strategy: no surrogate id column. UUIDs for user_id and post_id are
-- application-generated UUID v7 values (ADR 0004). No DEFAULT gen_random_uuid().
-- Timestamps: TIMESTAMPTZ stored in UTC (IDENTIFIERS_AND_TIME.md).

CREATE TABLE reactions (
    user_id       UUID        NOT NULL,
    post_id       UUID        NOT NULL,
    reaction_type VARCHAR(20) NOT NULL DEFAULT 'like',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT reactions_pkey
        PRIMARY KEY (user_id, post_id),

    CONSTRAINT reactions_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT reactions_post_id_fkey
        FOREIGN KEY (post_id) REFERENCES posts (id) ON DELETE CASCADE
);

-- Covers "fetch all reactions on a post" — the primary query shape for
-- private creator analytics and internal reaction resolution. The composite
-- PK index already covers the forward direction (user_id, post_id); this
-- index covers the reverse direction so the query planner can satisfy
-- post-centric lookups with a single index scan.
CREATE INDEX reactions_post_id_idx ON reactions (post_id);
