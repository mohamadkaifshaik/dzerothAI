-- 0017_extend_notification_events.up.sql
--
-- Extends the notification_event ENUM with title system events.
--
-- IRREVERSIBILITY WARNING:
-- PostgreSQL does not support removing ENUM values once added.
-- These values (title_unlocked, title_grace_period) cannot be removed without
-- DROP TYPE notification_event CASCADE, which would destroy the notifications
-- table. This migration must be treated as permanent in all environments.
-- The IF NOT EXISTS clause makes this statement safe to re-run.
ALTER TYPE notification_event ADD VALUE IF NOT EXISTS 'title_unlocked';
ALTER TYPE notification_event ADD VALUE IF NOT EXISTS 'title_grace_period';
