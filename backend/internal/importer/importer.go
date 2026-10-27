// Package importer reads a legacy x-digest static site directory and upserts
// everything into MariaDB. It is idempotent: re-running it never duplicates
// rows (posts/steps/tags are replaced per post, groups/accounts/tags are
// upserted by primary key).
package importer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/store"
)

type Counts struct {
	Posts         int `json:"posts"`
	PostSteps     int `json:"post_steps"`
	PostTags      int `json:"post_tags"`
	Authors       int `json:"authors"`
	Groups        int `json:"groups"`
	AuthorTags    int `json:"author_tags"`
	Vocabulary    int `json:"vocabulary"`
	AvatarsSeen   int `json:"avatars_seen"`
	Categories    int `json:"categories"`
	PostsWithStep int `json:"posts_with_steps"`
}

type legacySubstep struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

type legacyStep struct {
	Text     string          `json:"text"`
	Tools    string          `json:"tools"`
	Tech     string          `json:"tech"`
	Prep     string          `json:"prep"`
	Substeps []legacySubstep `json:"substeps"`
}

type legacyPost struct {
	ID           string       `json:"id"`
	URL          string       `json:"url"`
	Author       string       `json:"author"`
	AuthorName   string       `json:"authorName"`
	AuthorAvatar string       `json:"authorAvatar"`
	Title        string       `json:"title"`
	Industry     string       `json:"industry"`
	Tech         string       `json:"tech"`
	Tags         []string     `json:"tags"`
	Summary      string       `json:"summary"`
	Note         string       `json:"note"`
	PublishedAt  string       `json:"publishedAt"`
	IsLong       bool         `json:"isLong"`
	Steps        []legacyStep `json:"steps"`
	ShowFlow     *bool        `json:"showFlowchart"`
}

type legacyPostsFile struct {
	UpdatedAt string       `json:"updatedAt"`
	Posts     []legacyPost `json:"posts"`
}

type legacyWatchlist struct {
	Groups []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Fixed bool   `json:"fixed"`
	} `json:"groups"`
	Accounts []struct {
		Username string   `json:"username"`
		Name     string   `json:"name"`
		Avatar   string   `json:"avatar"`
		Group    string   `json:"group"`
		Tags     []string `json:"tags"`
		Bio      string   `json:"bio"`
	} `json:"accounts"`
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Import performs the full legacy import. dataDir is the old site root
// (containing data/posts.json, config/watchlist.json, config/user-tags.json,
// data/avatars.json).
func Import(db *sql.DB, dataDir string) (Counts, error) {
	var c Counts
	var postsFile legacyPostsFile
	if err := readJSON(filepath.Join(dataDir, "data", "posts.json"), &postsFile); err != nil {
		return c, fmt.Errorf("posts.json: %w", err)
	}
	var wl legacyWatchlist
	if err := readJSON(filepath.Join(dataDir, "config", "watchlist.json"), &wl); err != nil {
		return c, fmt.Errorf("watchlist.json: %w", err)
	}
	var userTags struct {
		Tags []string `json:"tags"`
	}
	if err := readJSON(filepath.Join(dataDir, "config", "user-tags.json"), &userTags); err != nil {
		return c, fmt.Errorf("user-tags.json: %w", err)
	}
	var avatars struct {
		Avatars map[string]struct {
			Username string `json:"username"`
			Name     string `json:"name"`
			Avatar   string `json:"avatar"`
		} `json:"avatars"`
	}
	if err := readJSON(filepath.Join(dataDir, "data", "avatars.json"), &avatars); err != nil {
		return c, fmt.Errorf("avatars.json: %w", err)
	}
	c.AvatarsSeen = len(avatars.Avatars)

	// Raw/API author fields are optional for legacy archives.
	rawAvatars, err := loadRawAvatars(filepath.Join(dataDir, "data", "raw_posts.json"))
	if err != nil {
		return c, fmt.Errorf("raw_posts.json: %w", err)
	}
	authorAvatars := map[string]string{}
	for username, av := range avatars.Avatars {
		authorAvatars[strings.ToLower(username)] = av.Avatar
	}
	for _, a := range wl.Accounts {
		if a.Avatar != "" {
			authorAvatars[strings.ToLower(a.Username)] = a.Avatar
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return c, err
	}
	defer tx.Rollback()

	// Default category from the legacy app (app.js fallback).
	if _, err := tx.Exec("INSERT IGNORE INTO categories(id,name) VALUES('fav','收藏')"); err != nil {
		return c, err
	}
	c.Categories = 1

	for _, g := range wl.Groups {
		if _, err := tx.Exec(`INSERT INTO watchlist_groups(id,name,fixed) VALUES(?,?,?)
			ON DUPLICATE KEY UPDATE name=VALUES(name), fixed=VALUES(fixed)`, g.ID, g.Name, boolToInt(g.Fixed)); err != nil {
			return c, err
		}
		c.Groups++
	}
	for _, a := range wl.Accounts {
		if _, err := tx.Exec(`INSERT INTO authors(username,name,avatar,group_id,bio) VALUES(?,?,?,?,?)
			ON DUPLICATE KEY UPDATE name=VALUES(name), avatar=VALUES(avatar), group_id=VALUES(group_id), bio=VALUES(bio)`,
			a.Username, a.Name, a.Avatar, a.Group, a.Bio); err != nil {
			return c, err
		}
		c.Authors++
		if _, err := tx.Exec("DELETE FROM author_tags WHERE username=?", a.Username); err != nil {
			return c, err
		}
		for _, t := range a.Tags {
			if _, err := tx.Exec("INSERT IGNORE INTO author_tags(username,tag) VALUES(?,?)", a.Username, t); err != nil {
				return c, err
			}
			c.AuthorTags++
		}
	}
	for _, t := range userTags.Tags {
		if _, err := tx.Exec("INSERT IGNORE INTO user_tag_vocabulary(tag) VALUES(?)", t); err != nil {
			return c, err
		}
		c.Vocabulary++
	}
	// Fallback authors from avatars.json (only if not already present).
	for username, av := range avatars.Avatars {
		if _, err := tx.Exec(`INSERT IGNORE INTO authors(username,name,avatar,group_id,bio) VALUES(?,?,?,NULL,'')`,
			username, av.Name, av.Avatar); err != nil {
			return c, err
		}
	}

	updatedMeta := postsFile.UpdatedAt
	if updatedMeta != "" {
		if conv, err := store.ParsePublished(updatedMeta); err == nil {
			updatedMeta = conv
		}
	}
	for _, p := range postsFile.Posts {
		if p.AuthorAvatar == "" {
			p.AuthorAvatar = rawAvatars[p.ID]
		}
		if p.AuthorAvatar == "" {
			p.AuthorAvatar = authorAvatars[strings.ToLower(p.Author)]
		}

		published, err := store.ParsePublished(p.PublishedAt)
		if err != nil {
			return c, fmt.Errorf("post %s publishedAt: %w", p.ID, err)
		}
		// Post-only authors (e.g. a poster not on the watchlist) keep a row
		// with NULL group so the watchlist listing stays curated, while the
		// post itself always carries denormalised author fields.
		if _, err := tx.Exec(`INSERT INTO authors(username,name,avatar,group_id,bio) VALUES(?,?,?,NULL,'')
            ON DUPLICATE KEY UPDATE avatar=IF(VALUES(avatar)<>'',VALUES(avatar),avatar)`,
			p.Author, p.AuthorName, p.AuthorAvatar); err != nil {
			return c, err
		}
		var show sql.NullBool
		if p.ShowFlow != nil {
			show = sql.NullBool{Bool: *p.ShowFlow, Valid: true}
		}
		if _, err := tx.Exec(`INSERT INTO posts(id,url,author,author_name,author_avatar,industry,tech,title,summary,note,
				published_at,is_long,show_flowchart,updated_at_meta) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE url=VALUES(url), author=VALUES(author), author_name=VALUES(author_name),
				author_avatar=VALUES(author_avatar), industry=VALUES(industry), tech=VALUES(tech),
				title=VALUES(title), note=VALUES(note),
				published_at=VALUES(published_at), is_long=VALUES(is_long),
				show_flowchart=VALUES(show_flowchart), updated_at_meta=VALUES(updated_at_meta)`,
			p.ID, p.URL, p.Author, p.AuthorName, p.AuthorAvatar, p.Industry, p.Tech, p.Title, "", p.Note,
			published, boolToInt(p.IsLong), show, updatedMeta); err != nil {
			return c, err
		}
		c.Posts++
		if _, err := tx.Exec("DELETE FROM post_steps WHERE post_id=?", p.ID); err != nil {
			return c, err
		}
		if _, err := tx.Exec("DELETE FROM post_tags WHERE post_id=?", p.ID); err != nil {
			return c, err
		}
		if len(p.Steps) > 0 {
			c.PostsWithStep++
		}
		for i, st := range p.Steps {
			sub, _ := json.Marshal(st.Substeps)
			if st.Substeps == nil {
				sub = []byte("null")
			}
			if _, err := tx.Exec(`INSERT INTO post_steps(post_id,idx,text,tools,tech,prep,substeps_json)
				VALUES(?,?,?,?,?,?,?)`, p.ID, i, st.Text, st.Tools, st.Tech, st.Prep, string(sub)); err != nil {
				return c, err
			}
			c.PostSteps++
		}
		seen := map[string]bool{}
		for _, t := range p.Tags {
			t = strings.TrimSpace(t)
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			if _, err := tx.Exec("INSERT INTO post_tags(post_id,tag) VALUES(?,?)", p.ID, t); err != nil {
				return c, err
			}
			c.PostTags++
		}
	}
	return c, tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// avatarFromRaw accepts both flattened collectors and nested API user objects.
func avatarFromRaw(p map[string]any) string {
	for _, key := range []string{"author_avatar", "authorAvatar", "avatar", "avatar_url", "profile_image", "profile_image_url_https", "profile_image_url"} {
		if value, ok := p[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, key := range []string{"author", "user", "legacy"} {
		if nested, ok := p[key].(map[string]any); ok {
			if value := avatarFromRaw(nested); value != "" {
				return value
			}
		}
	}
	return ""
}
func loadRawAvatars(path string) (map[string]string, error) {
	result := map[string]string{}
	var posts []map[string]any
	if err := readJSON(path, &posts); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	for _, p := range posts {
		if id, ok := p["id"].(string); ok {
			result[id] = avatarFromRaw(p)
		}
	}
	return result, nil
}
