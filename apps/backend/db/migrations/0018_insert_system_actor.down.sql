-- 0018_insert_system_actor.down.sql
--
-- Removes the sentinel system-actor user row.
-- Any notifications rows with actor_id = uuid.Nil will be removed first via
-- the ON DELETE CASCADE on notifications.actor_id.

DELETE FROM users WHERE id = '00000000-0000-0000-0000-000000000000';
