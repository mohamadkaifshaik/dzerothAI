-- 0002_create_sessions.down.sql
--
-- Reverses 0002_create_sessions.up.sql.
-- Indexes are dropped before the table for explicit ordering clarity.

DROP INDEX IF EXISTS sessions_expires_at_idx;
DROP INDEX IF EXISTS sessions_user_id_idx;
DROP INDEX IF EXISTS sessions_token_hash_idx;

DROP TABLE IF EXISTS sessions;
