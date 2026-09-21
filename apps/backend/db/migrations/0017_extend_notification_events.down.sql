-- 0017_extend_notification_events.down.sql
--
-- PostgreSQL does not support removing values from an ENUM type.
-- Rolling back this migration requires DROP TYPE ... CASCADE, which is
-- destructive (it drops the notifications table). This down migration is
-- intentionally a no-op. Treat migration 0017 as permanent in production.
-- If removal is required, execute a separate incident procedure.
SELECT 'ENUM values title_unlocked and title_grace_period cannot be removed '
       'without recreating the notifications table. '
       'This down migration is a no-op by design.' AS notice;
