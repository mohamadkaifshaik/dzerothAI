DROP INDEX IF EXISTS posts_quoted_post_id_idx;
DROP INDEX IF EXISTS bookmarks_post_id_idx;
DROP INDEX IF EXISTS reports_dedup_pending_idx;
DROP INDEX IF EXISTS reports_status_created_at_idx;
DROP INDEX IF EXISTS reports_target_user_id_idx;
DROP INDEX IF EXISTS reports_target_post_id_idx;
DROP INDEX IF EXISTS reports_reporter_id_idx;
DROP TABLE IF EXISTS reports;
