-- 0010_create_reactions.down.sql
--
-- Drops the reactions table created in 0010_create_reactions.up.sql.
--
-- IF EXISTS is used defensively so that a partially applied migration or
-- out-of-order down execution does not produce a hard error.
-- The reactions_post_id_idx index is owned by the table and is dropped
-- automatically when the table is dropped.

DROP TABLE IF EXISTS reactions;
