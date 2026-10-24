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

CREATE TABLE IF NOT EXISTS user_tag_vocabulary (
  tag VARCHAR(128) NOT NULL PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_tag_overrides (
  username VARCHAR(64)  NOT NULL,
  tag      VARCHAR(128) NOT NULL,
  PRIMARY KEY (username, tag),
  CONSTRAINT fk_overrides_author FOREIGN KEY (username)
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
  INDEX idx_posts_published (published_at DESC, id ASC),
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
