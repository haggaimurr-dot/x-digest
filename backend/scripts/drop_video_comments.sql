-- 视频热门评论下线（2026-10-29）：删除评论表与 *_videos.comments_fetched_at 列。
-- 代码已不再读写这些表/列；本脚本不会自动执行，需人工在服务器上运行。
-- 1. 先备份（备份至少保留 90 天）：
--    mysqldump xdigest yt_video_comments bili_video_comments > video_comments-$(date +%Y%m%d).sql
-- 2. 再执行：mysql xdigest < backend/scripts/drop_video_comments.sql
-- 回滚：mysql xdigest < video_comments-YYYYMMDD.sql（列可按旧 schema.sql 重新 ADD COLUMN）。
DROP TABLE IF EXISTS yt_video_comments;
DROP TABLE IF EXISTS bili_video_comments;
ALTER TABLE yt_videos DROP COLUMN IF EXISTS comments_fetched_at;
ALTER TABLE bili_videos DROP COLUMN IF EXISTS comments_fetched_at;
