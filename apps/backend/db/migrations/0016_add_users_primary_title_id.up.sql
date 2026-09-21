-- 0016_add_users_primary_title_id.up.sql
--
-- Adds primary_title_id to the users table, linking a user to their currently
-- displayed title badge (one of their active user_titles rows).
--
-- Design decisions:
--   - Nullable: users who have no title or have not selected one will have NULL.
--   - ON DELETE SET NULL: if the referenced user_title row is deleted (e.g.
--     the user account is wiped and cascaded), this column is nulled rather
--     than blocking the delete or leaving a dangling reference.
--   - DEFERRABLE INITIALLY DEFERRED: the mutual reference between users and
--     user_titles (users.primary_title_id → user_titles.id, and
--     user_titles.user_id → users.id) creates a potential circular FK issue
--     during inserts. Deferring allows a single transaction to insert a user
--     and a user_title row and then set primary_title_id without violating
--     referential integrity mid-transaction.
--   - Sparse index: only rows where primary_title_id IS NOT NULL are indexed,
--     keeping the index small since most users will not have a title set.

ALTER TABLE users ADD COLUMN primary_title_id UUID;

ALTER TABLE users
    ADD CONSTRAINT users_primary_title_id_fkey
    FOREIGN KEY (primary_title_id) REFERENCES user_titles (id)
    ON DELETE SET NULL
    DEFERRABLE INITIALLY DEFERRED;

-- Sparse index: only rows where a primary title is set are indexed.
CREATE INDEX users_primary_title_id_idx
    ON users (primary_title_id)
    WHERE primary_title_id IS NOT NULL;
