-- 0018_insert_system_actor.up.sql
--
-- Inserts the sentinel system-actor user row required by the title
-- notification worker (Phase 6).
--
-- Background
-- ----------
-- TitleNotificationWorker uses uuid.Nil (00000000-0000-0000-0000-000000000000)
-- as the actor_id for system-generated title notifications so that the actor
-- is clearly distinguishable from real users and cannot accidentally match any
-- application-generated UUID v7. The notifications table has a FK constraint
-- (notifications_actor_id_fkey) that requires actor_id to reference a row in
-- users. Without this sentinel row every INSERT into notifications from the
-- title worker fails with a FK violation, which prevents the
-- unlock_notification_sent / grace_notification_sent flags from being set.
--
-- The sentinel row:
--   id           = 00000000-0000-0000-0000-000000000000 (uuid.Nil)
--   handle       = __system__ (reserved; cannot be registered by real users
--                  because the registration path rejects the __ prefix)
--   display_name = System
--   email        = system@dzeroth.internal (unreachable domain, not a real address)
--   password_hash = '' (empty — this account cannot be authenticated)
--
-- ON CONFLICT DO NOTHING makes this migration safe to re-run (idempotent).

INSERT INTO users (
    id,
    handle,
    display_name,
    email,
    password_hash,
    created_at,
    updated_at
)
VALUES (
    '00000000-0000-0000-0000-000000000000',
    '__system__',
    'System',
    'system@dzeroth.internal',
    '',
    '2000-01-01 00:00:00+00',
    '2000-01-01 00:00:00+00'
)
ON CONFLICT DO NOTHING;
