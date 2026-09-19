-- 0011_create_notifications.up.sql
--
-- Creates the notification_event ENUM and notifications table for
-- Dzeroth Phase 4 (Notifications, Wave 1C).
--
-- ID: Application-generated UUID v7 (ADR 0004). No DEFAULT gen_random_uuid().
-- Timestamps: TIMESTAMPTZ stored in UTC (IDENTIFIERS_AND_TIME.md).
--
-- recipient_id: the user who receives the notification.
-- actor_id: the user whose action triggered the notification.
-- event: the type of interaction that produced the notification.
-- post_id: nullable — follow events carry no associated post.
-- is_read: starts FALSE; set TRUE only by an explicit "Mark all as read"
--   action. No auto-mark behaviour is implemented at the schema layer.
--
-- Deduplication index rationale: PostgreSQL treats multiple NULL values as
-- distinct in a standard unique index, which would allow unlimited duplicate
-- follow notifications (where post_id IS NULL). The COALESCE expression
-- substitutes the nil UUID sentinel for NULL so that uniqueness is evaluated
-- on a concrete value, preventing duplicate follow notifications while still
-- allowing the same actor to trigger different event types on the same post.
--
-- Public social-validation metrics are permanently excluded per CLAUDE.md
-- section 2.3. No notification-count column is added to any table here.

-- -------------------------------------------------------------------------
-- notification_event ENUM
-- -------------------------------------------------------------------------

CREATE TYPE notification_event AS ENUM ('follow', 'mention', 'reply', 'reaction');

-- -------------------------------------------------------------------------
-- notifications
-- -------------------------------------------------------------------------

CREATE TABLE notifications (
    id            UUID               NOT NULL,
    recipient_id  UUID               NOT NULL,
    actor_id      UUID               NOT NULL,
    event         notification_event NOT NULL,
    post_id       UUID,
    is_read       BOOLEAN            NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ        NOT NULL DEFAULT now(),

    CONSTRAINT notifications_pkey
        PRIMARY KEY (id),

    CONSTRAINT notifications_recipient_id_fkey
        FOREIGN KEY (recipient_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT notifications_actor_id_fkey
        FOREIGN KEY (actor_id) REFERENCES users (id) ON DELETE CASCADE,

    CONSTRAINT notifications_post_id_fkey
        FOREIGN KEY (post_id) REFERENCES posts (id) ON DELETE CASCADE
);

-- Primary read path: "fetch all notifications for recipient, newest first."
-- Supports paginated notification inbox queries efficiently.
CREATE INDEX notifications_recipient_created_idx
    ON notifications (recipient_id, created_at DESC);

-- Deduplication: prevents a (recipient, actor, event, post) tuple from being
-- inserted more than once. COALESCE substitutes the nil UUID sentinel for a
-- NULL post_id so that follow notifications (where post_id IS NULL) are
-- correctly deduplicated rather than treated as distinct by the unique index.
CREATE UNIQUE INDEX notifications_dedup_idx ON notifications (
    recipient_id,
    actor_id,
    event,
    COALESCE(post_id, '00000000-0000-0000-0000-000000000000'::uuid)
);
