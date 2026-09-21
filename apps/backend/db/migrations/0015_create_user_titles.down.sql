-- 0015_create_user_titles.down.sql
--
-- Reverses 0015_create_user_titles.up.sql.
-- Migration 0016 (users.primary_title_id FK) must be rolled back before this
-- migration because that FK references user_titles (id).

DROP TABLE user_titles;
