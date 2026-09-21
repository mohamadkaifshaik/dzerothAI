-- 0014_create_title_definitions.down.sql
--
-- Reverses 0014_create_title_definitions.up.sql.
-- Drops the title_definitions table including its seed data.
-- user_titles (0015) must be rolled back before this migration because
-- user_titles has a FOREIGN KEY ON DELETE RESTRICT referencing title_definitions.

DROP TABLE title_definitions;
