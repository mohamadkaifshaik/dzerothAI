-- 0016_add_users_primary_title_id.down.sql
--
-- Reverses 0016_add_users_primary_title_id.up.sql.
-- The index must be dropped before the constraint, and the constraint must be
-- dropped before the column.

DROP INDEX users_primary_title_id_idx;
ALTER TABLE users DROP CONSTRAINT users_primary_title_id_fkey;
ALTER TABLE users DROP COLUMN primary_title_id;
