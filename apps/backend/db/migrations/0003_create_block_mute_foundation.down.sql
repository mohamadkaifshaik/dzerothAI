-- 0003_create_block_mute_foundation.down.sql
--
-- Reverses 0003_create_block_mute_foundation.up.sql.
-- mutes dropped before blocks (reverse of creation order).

DROP INDEX IF EXISTS mutes_muted_id_idx;
DROP TABLE IF EXISTS mutes;

DROP INDEX IF EXISTS blocks_blocked_id_idx;
DROP TABLE IF EXISTS blocks;
