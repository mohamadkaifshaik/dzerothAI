-- 0001_create_users.down.sql
--
-- Reverses 0001_create_users.up.sql.
-- Drops indexes before the table (CASCADE would handle them, but explicit
-- ordering makes the intent clear and safe).

DROP INDEX IF EXISTS users_created_at_idx;
DROP INDEX IF EXISTS users_email_lower_idx;
DROP INDEX IF EXISTS users_handle_lower_idx;

DROP TABLE IF EXISTS users;
