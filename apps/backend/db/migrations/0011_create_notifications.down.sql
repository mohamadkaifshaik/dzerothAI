-- 0011_create_notifications.down.sql
--
-- Reverses 0011_create_notifications.up.sql.
-- Indexes are dropped before the table for explicit ordering clarity.
-- The table must be dropped before the notification_event ENUM because
-- the ENUM is referenced by the table's event column.

DROP INDEX IF EXISTS notifications_dedup_idx;
DROP INDEX IF EXISTS notifications_recipient_created_idx;

DROP TABLE IF EXISTS notifications;

DROP TYPE IF EXISTS notification_event;
