-- ─── REPORTS TABLE ─────────────────────────────────────────────────────────

CREATE TABLE reports (
    id               UUID         NOT NULL,
    reporter_id      UUID         NOT NULL,
    target_type      VARCHAR(10)  NOT NULL,
    target_post_id   UUID,
    target_user_id   UUID,
    reason           VARCHAR(50)  NOT NULL,
    detail           TEXT,
    status           VARCHAR(20)  NOT NULL DEFAULT 'pending',
    reviewed_by      UUID,
    reviewed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT reports_pkey                PRIMARY KEY (id),
    CONSTRAINT reports_reporter_id_fkey    FOREIGN KEY (reporter_id)    REFERENCES users (id)  ON DELETE CASCADE,
    CONSTRAINT reports_target_post_id_fkey FOREIGN KEY (target_post_id) REFERENCES posts (id)  ON DELETE CASCADE,
    CONSTRAINT reports_target_user_id_fkey FOREIGN KEY (target_user_id) REFERENCES users (id)  ON DELETE CASCADE,
    CONSTRAINT reports_reviewed_by_fkey    FOREIGN KEY (reviewed_by)    REFERENCES users (id)  ON DELETE SET NULL,
    CONSTRAINT reports_target_type_check   CHECK (target_type IN ('post', 'user')),
    CONSTRAINT reports_status_check        CHECK (status IN ('pending', 'reviewed', 'dismissed')),
    CONSTRAINT reports_target_xor          CHECK (
        (target_post_id IS NOT NULL AND target_user_id IS NULL)
        OR
        (target_post_id IS NULL AND target_user_id IS NOT NULL)
    ),
    CONSTRAINT reports_reason_check        CHECK (reason IN (
        'spam', 'harassment', 'misinformation', 'hate_speech', 'violence', 'other'
    )),
    CONSTRAINT reports_detail_length       CHECK (char_length(detail) <= 500)
);

CREATE INDEX reports_reporter_id_idx       ON reports (reporter_id);
CREATE INDEX reports_target_post_id_idx    ON reports (target_post_id) WHERE target_post_id IS NOT NULL;
CREATE INDEX reports_target_user_id_idx    ON reports (target_user_id) WHERE target_user_id IS NOT NULL;
CREATE INDEX reports_status_created_at_idx ON reports (status, created_at DESC);

-- Phase 5 note: No Phase 5 endpoint sets status to 'reviewed' or 'dismissed'.
-- reviewed_by/reviewed_at/status columns are retained for Phase 7 moderation actions.
-- All Phase 5 inserts will have status = 'pending'.

-- Dedup: one pending report per (reporter, target).
-- Mirrors the COALESCE pattern from migration 0011.
CREATE UNIQUE INDEX reports_dedup_pending_idx ON reports (
    reporter_id,
    COALESCE(target_post_id, '00000000-0000-0000-0000-000000000000'::uuid),
    COALESCE(target_user_id, '00000000-0000-0000-0000-000000000000'::uuid)
) WHERE status = 'pending';

-- ─── INDEXES REQUIRED FOR CREATOR STUDIO ANALYTICS ────────────────────────

-- bookmarks_post_id_idx: the existing bookmarks index is on (user_id, created_at DESC).
-- Creator Studio aggregates bookmark counts per post_id; requires post_id as leading key.
CREATE INDEX bookmarks_post_id_idx ON bookmarks (post_id);

-- posts_quoted_post_id_idx: used by Studio quote_count aggregation.
-- Partial index on non-deleted quote-type posts only.
-- posts_parent_id_idx (for reply_count) already exists from migration 0004.
-- reactions_post_id_idx (for reaction_count) already exists from migration 0010.
CREATE INDEX posts_quoted_post_id_idx ON posts (quoted_post_id)
    WHERE post_type = 'quote' AND is_deleted = FALSE;
