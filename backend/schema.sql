-- x-digest MariaDB schema. Database: xdigest, isolated from
-- chiji / bailiankai / bailiankai_web. Character set utf8mb4,
-- collation utf8mb4_unicode_ci everywhere.
-- Apply as a privileged user:  mysql < schema.sql
-- The application user is created separately (see db-user.sql template).

CREATE DATABASE IF NOT EXISTS xdigest
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE xdigest;

CREATE TABLE IF NOT EXISTS watchlist_groups (
  id    VARCHAR(64)  NOT NULL PRIMARY KEY,
  name  VARCHAR(128) NOT NULL,
  fixed TINYINT(1)   NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The default group (2026-10-29): new authors (导入单帖 / 关注列表批量添加)
-- go here. cmd/import also creates it from config/watchlist.json, but a
-- fresh database that only ran this file had none, so the first import
-- failed on fk_authors_group (found by the first CI / E2E run).
INSERT IGNORE INTO watchlist_groups (id, name, fixed) VALUES ('other', '默认', 1);

CREATE TABLE IF NOT EXISTS authors (
  username VARCHAR(64)  NOT NULL PRIMARY KEY,
  name     VARCHAR(128) NOT NULL DEFAULT '',
  avatar   VARCHAR(512) NOT NULL DEFAULT '',
  group_id VARCHAR(64)  NULL,
  bio      TEXT,
  CONSTRAINT fk_authors_group FOREIGN KEY (group_id)
    REFERENCES watchlist_groups(id) ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS author_tags (
  username VARCHAR(64)  NOT NULL,
  tag      VARCHAR(128) NOT NULL,
  PRIMARY KEY (username, tag),
  CONSTRAINT fk_author_tags_author FOREIGN KEY (username)
    REFERENCES authors(username) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS posts (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  url             VARCHAR(1024) NOT NULL DEFAULT '',
  author          VARCHAR(64)  NOT NULL DEFAULT '',
  author_name     VARCHAR(128) NOT NULL DEFAULT '',
  author_avatar   VARCHAR(512) NOT NULL DEFAULT '',
  industry        VARCHAR(256) NOT NULL DEFAULT '',
  tech            VARCHAR(512) NOT NULL DEFAULT '',
  title           TEXT,
  summary         TEXT,
  note            TEXT,
  published_at    DATETIME     NOT NULL,
  is_long         TINYINT(1)   NOT NULL DEFAULT 0,
  show_flowchart  TINYINT(1)   NULL,
  updated_at_meta DATETIME     NULL,
  imported_at   DATETIME     NULL,
  group_id      VARCHAR(64)  NULL,
  INDEX idx_posts_published (published_at DESC, id ASC),
  INDEX idx_posts_imported (imported_at DESC, id ASC),
  INDEX idx_posts_author (author)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS post_steps (
  id            BIGINT AUTO_INCREMENT PRIMARY KEY,
  post_id       VARCHAR(64) NOT NULL,
  idx           INT NOT NULL,
  text          TEXT,
  tools         VARCHAR(512) NOT NULL DEFAULT '',
  tech          VARCHAR(512) NOT NULL DEFAULT '',
  prep          TEXT,
  substeps_json TEXT,
  CONSTRAINT fk_post_steps_post FOREIGN KEY (post_id)
    REFERENCES posts(id) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_post_steps_post (post_id, idx)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS post_groups (
  id    VARCHAR(64)  NOT NULL PRIMARY KEY,
  name  VARCHAR(128) NOT NULL,
  fixed TINYINT(1)   NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO post_groups (id, name, fixed) VALUES ('other', '默认', 1);

CREATE TABLE IF NOT EXISTS post_tags (
  post_id VARCHAR(64)  NOT NULL,
  tag     VARCHAR(256) NOT NULL,
  PRIMARY KEY (post_id, tag),
  CONSTRAINT fk_post_tags_post FOREIGN KEY (post_id)
    REFERENCES posts(id) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_post_tags_tag (tag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS categories (
  id   VARCHAR(64)  NOT NULL PRIMARY KEY,
  name VARCHAR(128) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Default category, mirroring the legacy app's fallback (app.js).
INSERT IGNORE INTO categories (id, name) VALUES ('fav', '收藏');

CREATE TABLE IF NOT EXISTS post_categories (
  post_id     VARCHAR(64) NOT NULL,
  category_id VARCHAR(64) NOT NULL,
  PRIMARY KEY (post_id, category_id),
  CONSTRAINT fk_pc_post FOREIGN KEY (post_id)
    REFERENCES posts(id) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT fk_pc_cat FOREIGN KEY (category_id)
    REFERENCES categories(id) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS replicas (
  id               VARCHAR(64)   NOT NULL PRIMARY KEY,
  post_id          VARCHAR(64)   NOT NULL,
  post_url         VARCHAR(1024) NOT NULL DEFAULT '',
  post_author      VARCHAR(64)   NOT NULL DEFAULT '',
  post_author_name VARCHAR(128)  NOT NULL DEFAULT '',
  post_title       TEXT,
  created_at       DATETIME      NOT NULL,
  INDEX idx_replicas_post (post_id),
  INDEX idx_replicas_created (created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Fuke edit-time sort (2026-10-30): updated_at tracks the last user edit
-- (step add/delete/patch, attachment add/delete). Old rows backfill from
-- created_at so the sort is total.
ALTER TABLE replicas ADD COLUMN IF NOT EXISTS updated_at DATETIME NULL;
UPDATE replicas SET updated_at=created_at WHERE updated_at IS NULL;

CREATE TABLE IF NOT EXISTS replica_steps (
  id            BIGINT AUTO_INCREMENT PRIMARY KEY,
  replica_id    VARCHAR(64) NOT NULL,
  idx           INT NOT NULL,
  text          TEXT,
  tools         VARCHAR(512) NOT NULL DEFAULT '',
  tech          VARCHAR(512) NOT NULL DEFAULT '',
  prep          TEXT,
  substeps_json TEXT,
  status        ENUM('idle','active','done') NOT NULL DEFAULT 'idle',
  note          TEXT,
  is_custom     TINYINT(1) NOT NULL DEFAULT 0,
  parent_idx    INT NULL DEFAULT NULL,
  CONSTRAINT fk_replica_steps_replica FOREIGN KEY (replica_id)
    REFERENCES replicas(id) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_replica_steps_replica (replica_id, idx)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Attachments store metadata + disk path only. NEVER a BLOB column.
CREATE TABLE IF NOT EXISTS attachments (
  id           VARCHAR(64)   NOT NULL PRIMARY KEY,
  replica_id   VARCHAR(64)   NOT NULL,
  step_idx     INT           NOT NULL,
  name         VARCHAR(512)  NOT NULL,
  size         BIGINT        NOT NULL,
  content_type VARCHAR(128)  NOT NULL DEFAULT '',
  ext          VARCHAR(16)   NOT NULL DEFAULT '',
  path         TEXT          NOT NULL,
  created_at   DATETIME      NOT NULL,
  CONSTRAINT fk_attachments_replica FOREIGN KEY (replica_id)
    REFERENCES replicas(id) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_attachments_replica (replica_id, step_idx, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- GitHub repos section (added 2026-10-25) ----
-- Only additive tables; no existing table is altered and no existing
-- data is touched. All CREATEs are IF NOT EXISTS and all ALTERs use
-- ADD COLUMN IF NOT EXISTS, so re-apply is safe.

CREATE TABLE IF NOT EXISTS repo_groups (
  id    VARCHAR(64)  NOT NULL PRIMARY KEY,
  name  VARCHAR(128) NOT NULL,
  fixed TINYINT(1)   NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO repo_groups (id, name, fixed) VALUES ('other', '默认', 1);

CREATE TABLE IF NOT EXISTS repos (
  full_name      VARCHAR(255) NOT NULL PRIMARY KEY,
  owner          VARCHAR(128) NOT NULL,
  name           VARCHAR(128) NOT NULL,
  description    TEXT,
  readme_intro   TEXT,
  tutorial_steps JSON,
  language       VARCHAR(64)  NOT NULL DEFAULT '',
  stars          INT          NOT NULL DEFAULT 0,
  forks          INT          NOT NULL DEFAULT 0,
  open_issues    INT          NULL,
  open_prs       INT          NULL,
  html_url       VARCHAR(1024) NOT NULL DEFAULT '',
  homepage       VARCHAR(1024) NOT NULL DEFAULT '',
  pushed_at      DATETIME     NULL,
  repo_updated_at DATETIME    NULL,
  group_id       VARCHAR(64)  NULL,
  note           TEXT,
  last_synced_at DATETIME     NULL,
  sync_status    VARCHAR(32)  NOT NULL DEFAULT 'never',
  sync_error     TEXT,
  CONSTRAINT fk_repos_group FOREIGN KEY (group_id)
    REFERENCES repo_groups(id) ON DELETE SET NULL ON UPDATE CASCADE,
  INDEX idx_repos_group (group_id),
  INDEX idx_repos_pushed (pushed_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- New columns for pre-existing installs (fresh CREATE above already
-- includes them); IF NOT EXISTS keeps re-apply safe.
ALTER TABLE repos ADD COLUMN IF NOT EXISTS readme_intro TEXT;
ALTER TABLE repos ADD COLUMN IF NOT EXISTS tutorial_steps JSON;

CREATE TABLE IF NOT EXISTS repo_tags (
  full_name VARCHAR(255) NOT NULL,
  tag       VARCHAR(128) NOT NULL,
  PRIMARY KEY (full_name, tag),
  CONSTRAINT fk_repo_tags_repo FOREIGN KEY (full_name)
    REFERENCES repos(full_name) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS repo_commits (
  id            BIGINT AUTO_INCREMENT PRIMARY KEY,
  full_name     VARCHAR(255) NOT NULL,
  sha           VARCHAR(64)  NOT NULL,
  message       TEXT,
  author_name   VARCHAR(128) NOT NULL DEFAULT '',
  author_login  VARCHAR(128) NOT NULL DEFAULT '',
  committed_at  DATETIME     NOT NULL,
  html_url      VARCHAR(1024) NOT NULL DEFAULT '',
  UNIQUE KEY uq_repo_commits_full_sha (full_name, sha),
  CONSTRAINT fk_repo_commits_repo FOREIGN KEY (full_name)
    REFERENCES repos(full_name) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_repo_commits_full_time (full_name, committed_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS repo_releases (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  full_name    VARCHAR(255) NOT NULL,
  tag_name     VARCHAR(128) NOT NULL,
  name         VARCHAR(256) NOT NULL DEFAULT '',
  body         MEDIUMTEXT,
  published_at DATETIME     NULL,
  html_url     VARCHAR(1024) NOT NULL DEFAULT '',
  draft        TINYINT(1)   NOT NULL DEFAULT 0,
  prerelease   TINYINT(1)   NOT NULL DEFAULT 0,
  UNIQUE KEY uq_repo_releases_full_tag (full_name, tag_name),
  CONSTRAINT fk_repo_releases_repo FOREIGN KEY (full_name)
    REFERENCES repos(full_name) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_repo_releases_full_time (full_name, published_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- New column for pre-existing installs (fresh CREATE above already
-- includes it); IF NOT EXISTS keeps re-apply safe.
ALTER TABLE repo_releases ADD COLUMN IF NOT EXISTS body MEDIUMTEXT;

-- ---- YouTube channels (prototype 2026-10-26; yt-dlp sync since 2026-10-26 v2) ----
-- Mirrors the repos pattern: yt_groups like repo_groups (default group
-- 'other'), yt_channels like repos, yt_videos like posts. Videos come
-- from yt-dlp tab listings (videos/shorts/streams/playlists) plus a
-- per-item metadata backfill for descriptions and upload dates — the
-- RSS feed prototype could only fetch ~15 latest videos with no tab
-- split, no playlists and no posts, so it was replaced (see
-- internal/youtube). yt_videos.channel_id is by value, no FK
-- constraint. Upsert on sync: same video_id = update, never duplicate.
-- A failed tab fetch never deletes existing videos.
CREATE TABLE IF NOT EXISTS yt_groups (
  id    VARCHAR(64)  NOT NULL PRIMARY KEY,
  name  VARCHAR(128) NOT NULL,
  fixed TINYINT(1)   NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO yt_groups (id, name, fixed) VALUES ('other', '默认', 1);

CREATE TABLE IF NOT EXISTS yt_channels (
  id             VARCHAR(64)  NOT NULL PRIMARY KEY,
  channel_id     VARCHAR(64)  NOT NULL,
  handle         VARCHAR(128) NOT NULL DEFAULT '',
  name           VARCHAR(256) NOT NULL DEFAULT '',
  group_id       VARCHAR(64)  NULL,
  date_from      DATE          NULL,
  date_to        DATE          NULL,
  created_at     DATETIME     NOT NULL,
  last_synced_at DATETIME     NULL,
  sync_status    VARCHAR(32)  NOT NULL DEFAULT 'never',
  sync_error     TEXT,
  UNIQUE KEY uq_yt_channels_channel_id (channel_id),
  CONSTRAINT fk_yt_channels_group FOREIGN KEY (group_id)
    REFERENCES yt_groups(id) ON DELETE SET NULL ON UPDATE CASCADE,
  INDEX idx_yt_channels_group (group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS yt_videos (
  id            VARCHAR(64)   NOT NULL PRIMARY KEY,
  channel_id    VARCHAR(64)   NOT NULL,
  kind          VARCHAR(16)   NOT NULL DEFAULT 'video',
  title         TEXT          NOT NULL,
  description   TEXT,
  published_at  DATETIME      NULL,
  duration      INT           NULL,
  live_status   VARCHAR(16)   NOT NULL DEFAULT '',
  thumbnail_url VARCHAR(1024) NOT NULL DEFAULT '',
  url           VARCHAR(1024) NOT NULL DEFAULT '',
  created_at    DATETIME      NOT NULL,
  INDEX idx_yt_videos_channel_time (channel_id, published_at DESC),
  INDEX idx_yt_videos_published (published_at DESC),
  INDEX idx_yt_videos_channel_kind (channel_id, kind, published_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- v2 columns for pre-existing installs (fresh CREATE above already
-- includes them); IF NOT EXISTS keeps re-apply safe. published_at is
-- nullable now: playlists have no publish date, and videos only get a
-- date once the yt-dlp metadata backfill supplies one.
ALTER TABLE yt_channels ADD COLUMN IF NOT EXISTS date_from DATE NULL;
ALTER TABLE yt_channels ADD COLUMN IF NOT EXISTS date_to DATE NULL;
ALTER TABLE yt_videos ADD COLUMN IF NOT EXISTS kind VARCHAR(16) NOT NULL DEFAULT 'video';
ALTER TABLE yt_videos ADD COLUMN IF NOT EXISTS duration INT NULL;
ALTER TABLE yt_videos ADD COLUMN IF NOT EXISTS live_status VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE yt_videos MODIFY published_at DATETIME NULL;
ALTER TABLE yt_videos ADD INDEX IF NOT EXISTS idx_yt_videos_channel_kind (channel_id, kind, published_at DESC);

-- ---- YouTube single-video configuration (added 2026-10-27) ----
-- sync_mode: full = the channel's tabs are synced (default); single = the
-- channel was added through one video and only holds the videos added by
-- URL until "同步该账号全部视频" switches it to full. "同步全部" skips
-- single-mode channels. added_manually videos survive date-range pruning.
ALTER TABLE yt_channels ADD COLUMN IF NOT EXISTS sync_mode VARCHAR(16) NOT NULL DEFAULT 'full';
ALTER TABLE yt_videos ADD COLUMN IF NOT EXISTS added_manually TINYINT(1) NOT NULL DEFAULT 0;

-- ---- YouTube AI summaries (added 2026-10-27) ----
-- One row per (video, summary language). Generated on demand (POST
-- /api/v1/yt/videos/{id}/summary) from the video's subtitles via yt-dlp
-- plus the TRANSLATE_* chat relay; status pending -> done | failed.
-- key_points / chapters are JSON arrays (chapters: start seconds, title,
-- summary). source: transcript | description (no subtitles found).
-- video_id is by value, no FK (same as yt_videos.channel_id); rows are
-- removed with their channel.
CREATE TABLE IF NOT EXISTS yt_video_tags (
  video_id VARCHAR(64) NOT NULL,
  tag      VARCHAR(128) NOT NULL,
  PRIMARY KEY (video_id, tag),
  CONSTRAINT fk_yt_video_tags_video FOREIGN KEY (video_id)
    REFERENCES yt_videos(id) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_yt_video_tags_tag (tag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bili_video_tags (
  video_id VARCHAR(64) NOT NULL,
  tag      VARCHAR(128) NOT NULL,
  PRIMARY KEY (video_id, tag),
  CONSTRAINT fk_bili_video_tags_video FOREIGN KEY (video_id)
    REFERENCES bili_videos(id) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX idx_bili_video_tags_tag (tag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS yt_video_summaries (
  video_id        VARCHAR(64)  NOT NULL,
  lang            VARCHAR(10)  NOT NULL,
  status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
  summary         MEDIUMTEXT,
  key_points      MEDIUMTEXT,
  chapters        MEDIUMTEXT,
  source          VARCHAR(16)  NOT NULL DEFAULT '',
  transcript_lang VARCHAR(16)  NOT NULL DEFAULT '',
  model           VARCHAR(64)  NOT NULL DEFAULT '',
  error           TEXT,
  created_at      DATETIME     NOT NULL,
  updated_at      DATETIME     NOT NULL,
  PRIMARY KEY (video_id, lang)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- Video top comments: removed 2026-10-29 ----
-- yt_video_comments / bili_video_comments and the *_videos.comments_fetched_at
-- columns belonged to the top-comments feature, which is gone. New installs
-- no longer create them; on an existing database back them up and drop
-- them with backend/scripts/drop_video_comments.sql.

-- ---- Bilibili uploaders (added 2026-10-27) ----
-- Same shape as the yt_* tables (the /bilibili page reuses the YouTube
-- page): channel_id is the uploader's numeric mid; video id is the BV
-- id, or season-<id> / series-<id> for 合集 / 系列 (kind playlist).
-- Synced from Bilibili's web API (internal/bilibili), not yt-dlp.
CREATE TABLE IF NOT EXISTS bili_groups (
  id    VARCHAR(64)  NOT NULL PRIMARY KEY,
  name  VARCHAR(128) NOT NULL,
  fixed TINYINT(1)   NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO bili_groups (id, name, fixed) VALUES ('other', '默认', 1);

CREATE TABLE IF NOT EXISTS bili_channels (
  id             VARCHAR(64)  NOT NULL PRIMARY KEY,
  channel_id     VARCHAR(64)  NOT NULL,
  handle         VARCHAR(128) NOT NULL DEFAULT '',
  name           VARCHAR(256) NOT NULL DEFAULT '',
  group_id       VARCHAR(64)  NULL,
  date_from      DATE         NULL,
  date_to        DATE         NULL,
  created_at     DATETIME     NOT NULL,
  last_synced_at DATETIME     NULL,
  sync_status    VARCHAR(32)  NOT NULL DEFAULT 'never',
  sync_error     TEXT,
  UNIQUE KEY uq_bili_channels_channel_id (channel_id),
  CONSTRAINT fk_bili_channels_group FOREIGN KEY (group_id)
    REFERENCES bili_groups(id) ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bili_videos (
  id                  VARCHAR(64)   NOT NULL PRIMARY KEY,
  channel_id          VARCHAR(64)   NOT NULL,
  kind                VARCHAR(16)   NOT NULL DEFAULT 'video',
  title               TEXT          NOT NULL,
  description         TEXT,
  published_at        DATETIME      NULL,
  duration            INT           NULL,
  live_status         VARCHAR(16)   NOT NULL DEFAULT '',
  thumbnail_url       VARCHAR(1024) NOT NULL DEFAULT '',
  url                 VARCHAR(1024) NOT NULL DEFAULT '',
  created_at          DATETIME      NOT NULL,
  INDEX idx_bili_videos_channel_kind (channel_id, kind, published_at DESC),
  INDEX idx_bili_videos_published (published_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bili_video_summaries (
  video_id        VARCHAR(64)  NOT NULL,
  lang            VARCHAR(10)  NOT NULL,
  status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
  summary         MEDIUMTEXT,
  key_points      MEDIUMTEXT,
  chapters        MEDIUMTEXT,
  source          VARCHAR(16)  NOT NULL DEFAULT '',
  transcript_lang VARCHAR(16)  NOT NULL DEFAULT '',
  model           VARCHAR(64)  NOT NULL DEFAULT '',
  error           TEXT,
  created_at      DATETIME     NOT NULL,
  updated_at      DATETIME     NOT NULL,
  PRIMARY KEY (video_id, lang)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- AI summary "resources" (tools / repos / sites the video mentions,
-- JSON array of {name,url,note}), added 2026-10-27.
ALTER TABLE yt_video_summaries ADD COLUMN IF NOT EXISTS resources MEDIUMTEXT NULL;
ALTER TABLE bili_video_summaries ADD COLUMN IF NOT EXISTS resources MEDIUMTEXT NULL;

-- Original post text + links (JSON array), filled by the importer from
-- data/raw_posts.json (added 2026-10-27). The page used to get them only
-- from the bundled frontend/assets/mock/x-content.json.
ALTER TABLE posts ADD COLUMN IF NOT EXISTS text MEDIUMTEXT NULL;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS links TEXT NULL;
-- AI post type and link explanations from cmd/digest (added 2026-10-27).
ALTER TABLE posts ADD COLUMN IF NOT EXISTS kind VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE posts ADD COLUMN IF NOT EXISTS resources TEXT NULL;

-- ---- X post AI summaries (added 2026-10-27) ----
-- Generated on demand (POST /api/v1/posts/{id}/summary) from the post's
-- original text via the TRANSLATE_* relay, same flow as the video
-- summaries. source: transcript = from the original text, description =
-- original not collected (title / earlier digest only).
CREATE TABLE IF NOT EXISTS post_summaries (
  post_id    VARCHAR(64)  NOT NULL,
  lang       VARCHAR(10)  NOT NULL,
  status     VARCHAR(16)  NOT NULL DEFAULT 'pending',
  summary    MEDIUMTEXT,
  key_points MEDIUMTEXT,
  resources  MEDIUMTEXT,
  source     VARCHAR(16)  NOT NULL DEFAULT '',
  model      VARCHAR(64)  NOT NULL DEFAULT '',
  error      TEXT,
  created_at DATETIME     NOT NULL,
  updated_at DATETIME     NOT NULL,
  PRIMARY KEY (post_id, lang)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- Content translation cache (added 2026-10-26) ----
-- One row per (source text, target language). source_hash is the SHA-256
-- of source text + target language (see internal/translate.Hash).
-- Only genuine translations are stored; short-circuited originals and
-- failed items are never cached.

CREATE TABLE IF NOT EXISTS translations (
  source_hash     CHAR(64)     NOT NULL,
  target_lang     VARCHAR(10)  NOT NULL,
  source_text     MEDIUMTEXT   NOT NULL,
  translated_text MEDIUMTEXT   NOT NULL,
  created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (source_hash, target_lang),
  INDEX idx_translations_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- X account refresh jobs (2026-10-28). The whole job state (accounts with
-- their pending/running/done/error status, counts and failed post ids) is
-- one JSON document, saved after every account. A job still marked running
-- at start-up was cut off by a restart and is resumed: accounts already done
-- are not sent to Grok again. Only the newest job matters to the page.
CREATE TABLE IF NOT EXISTS refresh_jobs (
  id          VARCHAR(40)  NOT NULL PRIMARY KEY,
  running     TINYINT(1)   NOT NULL DEFAULT 1,
  state       MEDIUMTEXT   NOT NULL,
  started_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_refresh_jobs_started (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Categories soft delete (2026-10-28): DELETE /categories/{id} sets
-- deleted_at; POST /categories/{id}/restore clears it (page undo toast);
-- rows deleted more than 7 days ago are purged at API start-up and daily.
ALTER TABLE categories ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL DEFAULT NULL;

-- Manual 复刻 switch per post (2026-10-28): NULL = the page decides from the
-- post type / keyword rules, 1 = always offer 复刻, 0 = never.
ALTER TABLE posts ADD COLUMN IF NOT EXISTS replica_enabled TINYINT(1) NULL DEFAULT NULL;

-- Saved filter combinations on the X daily page (2026-10-28). filter_json
-- is the page's filter state ({keyword, date_from, date_to, kind, authors,
-- tags, categories}); the API only checks it is a JSON object.
CREATE TABLE IF NOT EXISTS saved_views (
  id          VARCHAR(40)  NOT NULL PRIMARY KEY,
  name        VARCHAR(64)  NOT NULL,
  filter_json TEXT         NOT NULL,
  sort_order  INT          NOT NULL DEFAULT 0,
  created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_saved_views_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Notifications (2026-10-28): the top-bar bell; written when a repo sync
-- finds a new release.
CREATE TABLE IF NOT EXISTS notifications (
  id         VARCHAR(40)   NOT NULL PRIMARY KEY,
  kind       VARCHAR(32)   NOT NULL,
  title      VARCHAR(512)  NOT NULL,
  url        VARCHAR(1024) NOT NULL DEFAULT '',
  created_at DATETIME      NOT NULL,
  read_at    DATETIME      NULL DEFAULT NULL,
  INDEX idx_notifications_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Where a replica came from (2026-10-28): post (default) or repo (README
-- steps; post_id then holds "repo:owner/name").
ALTER TABLE replicas ADD COLUMN IF NOT EXISTS source VARCHAR(16) NOT NULL DEFAULT 'post';

-- Video watch state (2026-10-28): 已看 (watched_at) and 稍后观看
-- (watch_later), set from the video cards via PATCH /{yt|bili}/videos/{id}.
ALTER TABLE yt_videos ADD COLUMN IF NOT EXISTS watched_at DATETIME NULL DEFAULT NULL;
ALTER TABLE yt_videos ADD COLUMN IF NOT EXISTS watch_later TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE bili_videos ADD COLUMN IF NOT EXISTS watched_at DATETIME NULL DEFAULT NULL;
ALTER TABLE bili_videos ADD COLUMN IF NOT EXISTS watch_later TINYINT(1) NOT NULL DEFAULT 0;

-- YouTube moved onto the shared video hub (2026-10-29): the hub code reads
-- sync_mode / added_manually for every platform, so the bili tables get the
-- same two columns as yt (Bilibili has no add-by-URL yet; they stay at the
-- defaults).
ALTER TABLE bili_channels ADD COLUMN IF NOT EXISTS sync_mode VARCHAR(16) NOT NULL DEFAULT 'full';
ALTER TABLE bili_videos ADD COLUMN IF NOT EXISTS added_manually TINYINT(1) NOT NULL DEFAULT 0;

-- Settings cloud sync (2026-10-28): one row per API token (sha256 hex of
-- the token; "default" when the server has no API_TOKEN).
CREATE TABLE IF NOT EXISTS user_settings (
  token_hash  VARCHAR(64)  NOT NULL PRIMARY KEY,
  json        TEXT         NOT NULL,
  updated_at  DATETIME(3)  NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Posts deleted on the page (2026-10-29): DELETE /api/v1/posts/{id} and
-- POST /api/v1/posts/batch-delete record the id here so the bot import
-- (cmd/import) and 更新帖子 do not bring the post back; importing it again
-- with 导入单帖 removes the record.
CREATE TABLE IF NOT EXISTS deleted_posts (
  id         VARCHAR(64) NOT NULL PRIMARY KEY,
  deleted_at DATETIME    NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Scheduled 更新帖子 (2026-10-29): REFRESH_INTERVAL refreshes only the
-- accounts marked here (set on the page, PUT /api/v1/x/refresh/accounts),
-- not the whole watchlist.
ALTER TABLE authors ADD COLUMN IF NOT EXISTS auto_refresh TINYINT(1) NOT NULL DEFAULT 0;

-- Repos page (2026-10-29): 入库时间 for sorting (rows added before this
-- column get their last sync time, else now, as the closest known value),
-- and the repo AI summaries (same shape as post_summaries).
ALTER TABLE repos ADD COLUMN IF NOT EXISTS created_at DATETIME NULL;
UPDATE repos SET created_at=COALESCE(last_synced_at, NOW()) WHERE created_at IS NULL;

CREATE TABLE IF NOT EXISTS repo_summaries (
  full_name  VARCHAR(255) NOT NULL,
  lang       VARCHAR(10)  NOT NULL,
  status     VARCHAR(16)  NOT NULL DEFAULT 'pending',
  summary    MEDIUMTEXT,
  key_points MEDIUMTEXT,
  resources  MEDIUMTEXT,
  source     VARCHAR(16)  NOT NULL DEFAULT '',
  model      VARCHAR(64)  NOT NULL DEFAULT '',
  error      TEXT,
  created_at DATETIME     NOT NULL,
  updated_at DATETIME     NOT NULL,
  PRIMARY KEY (full_name, lang),
  CONSTRAINT fk_repo_summaries_repo FOREIGN KEY (full_name)
    REFERENCES repos(full_name) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2026-10-30: posts.imported_at (import time). Backfill with published_at as proxy.
ALTER TABLE posts ADD COLUMN IF NOT EXISTS imported_at DATETIME NULL;
UPDATE posts SET imported_at = published_at WHERE imported_at IS NULL;

-- 2026-10-30: posts.group_id (post-level groups, separate from author groups).
ALTER TABLE posts ADD COLUMN IF NOT EXISTS group_id VARCHAR(64) NULL;
