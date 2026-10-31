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
	"regexp"
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
	AvatarsSeen   int `json:"avatars_seen"`
	Categories    int `json:"categories"`
	PostsWithStep int `json:"posts_with_steps"`
	// Report (2026-10-28): posts newly inserted / already present and
	// updated / skipped (duplicate id in the file) / failed validation.
	// Failed posts are left out; the rest still go in.
	Inserted int          `json:"inserted"`
	Updated  int          `json:"updated"`
	Skipped  int          `json:"skipped"`
	Failed   []FailedPost `json:"failed"`
}

// FailedPost is one posts.json item that could not be imported.
type FailedPost struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// checkPost validates one posts.json item before it is written; "" = fine.
// It returns the parsed published time for the insert.
func checkPost(p legacyPost) (published string, reason string) {
	if strings.TrimSpace(p.ID) == "" {
		return "", "missing id"
	}
	if strings.TrimSpace(p.Author) == "" {
		return "", "missing author"
	}
	published, err := store.ParsePublished(p.PublishedAt)
	if err != nil {
		return "", "bad publishedAt: " + err.Error()
	}
	return published, ""
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
	// Original post text / links: optional in posts.json; when absent they
	// come from data/raw_posts.json (same rules as sync-x-content.mjs).
	Text  string   `json:"text"`
	Links []string `json:"links"`
	// From cmd/digest: post type and link explanations (optional).
	Kind      string `json:"kind"`
	Resources []struct {
		URL  string `json:"url"`
		Name string `json:"name"`
		Note string `json:"note"`
	} `json:"resources"`
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
// (containing data/posts.json, config/watchlist.json,
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

	// Raw/API fields (avatar, original text) are optional for legacy archives.
	raw, err := loadRaw(filepath.Join(dataDir, "data", "raw_posts.json"))
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
	existing := map[string]bool{}
	if rows, err := tx.Query("SELECT id FROM posts"); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				existing[id] = true
			}
		}
		rows.Close()
	}
	// Posts deleted on the page stay deleted (no table yet = nothing deleted).
	seenIDs := map[string]bool{}
	if rows, err := tx.Query("SELECT id FROM deleted_posts"); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				seenIDs[id] = true
			}
		}
		rows.Close()
	}
	c.Failed = []FailedPost{}
	for _, p := range postsFile.Posts {
		published, reason := checkPost(p)
		if reason != "" {
			c.Failed = append(c.Failed, FailedPost{ID: p.ID, Reason: reason})
			continue
		}
		if seenIDs[p.ID] {
			c.Skipped++
			continue
		}
		seenIDs[p.ID] = true
		if existing[p.ID] {
			c.Updated++
		} else {
			c.Inserted++
		}
		r := raw[p.ID]
		if p.AuthorAvatar == "" {
			p.AuthorAvatar = r.Avatar
		}
		if strings.TrimSpace(p.Text) == "" {
			p.Text = r.Text
		}
		if len(p.Links) == 0 {
			p.Links = r.Links
		}
		if len(p.Links) == 0 {
			p.Links = extractLinks(p.Text)
		}
		resourcesJSON := ""
		if len(p.Resources) > 0 {
			b, _ := json.Marshal(p.Resources)
			resourcesJSON = string(b)
		}
		linksJSON := ""
		if len(p.Links) > 0 {
			b, _ := json.Marshal(p.Links)
			linksJSON = string(b)
		}
		if p.AuthorAvatar == "" {
			p.AuthorAvatar = authorAvatars[strings.ToLower(p.Author)]
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
		// text/links never overwrite a stored value with an empty one.
		if _, err := tx.Exec(`INSERT INTO posts(id,url,author,author_name,author_avatar,industry,tech,title,summary,note,
				published_at,is_long,show_flowchart,updated_at_meta,text,links,kind,resources,imported_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NOW())
			ON DUPLICATE KEY UPDATE url=VALUES(url), author=VALUES(author), author_name=VALUES(author_name),
				text=IF(VALUES(text)<>'',VALUES(text),text), links=IF(VALUES(links)<>'',VALUES(links),links),
				kind=IF(VALUES(kind)<>'',VALUES(kind),kind), resources=IF(VALUES(resources)<>'',VALUES(resources),resources),
				author_avatar=VALUES(author_avatar), industry=VALUES(industry), tech=VALUES(tech),
				title=VALUES(title), note=VALUES(note),
				published_at=VALUES(published_at), is_long=VALUES(is_long),
				show_flowchart=VALUES(show_flowchart), updated_at_meta=VALUES(updated_at_meta)`,
			p.ID, p.URL, p.Author, p.AuthorName, p.AuthorAvatar, p.Industry, p.Tech, p.Title, "", p.Note,
			published, boolToInt(p.IsLong), show, updatedMeta, p.Text, linksJSON, p.Kind, resourcesJSON); err != nil {
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

// rawPost is what the importer takes from one data/raw_posts.json item.
type rawPost struct {
	Avatar string
	Text   string
	Links  []string
}

// textFromRaw picks the fullest text: long_text, note_tweet, then text
// (each may be a string or an object with a "text" field).
func textFromRaw(p map[string]any) string {
	for _, key := range []string{"long_text", "note_tweet", "text"} {
		switch v := p[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case map[string]any:
			if t, ok := v["text"].(string); ok && strings.TrimSpace(t) != "" {
				return t
			}
		}
	}
	return ""
}

var (
	linkRe      = regexp.MustCompile(`https?://[^\s<>"“”]+`)
	linkTrailRe = regexp.MustCompile(`[，。！？、；：）】》]+$`)
)

// extractLinks returns the distinct http(s) links in text, without
// trailing full-width punctuation.
func extractLinks(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range linkRe.FindAllString(text, -1) {
		m = linkTrailRe.ReplaceAllString(m, "")
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func loadRaw(path string) (map[string]rawPost, error) {
	result := map[string]rawPost{}
	var posts []map[string]any
	if err := readJSON(path, &posts); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	for _, p := range posts {
		if id, ok := p["id"].(string); ok {
			text, links := RawContent(p)
			result[id] = rawPost{Avatar: avatarFromRaw(p), Text: text, Links: links}
		}
	}
	return result, nil
}

// RawContent returns the display text of one raw_posts.json item (fullest
// text, t.co links expanded, quoted/replied-to text appended) and its
// resource links. The importer and cmd/digest share it.
func RawContent(p map[string]any) (text string, links []string) {
	text = expandShortLinks(textFromRaw(p), urlEntities(p)) + referencedText(p)
	links = resourceLinks(extractLinks(text))
	// A quote's expanded link is the quoted tweet itself (its text is
	// already appended), not a resource.
	refs, _ := p["referenced_tweets"].([]any)
	var kept []string
	for _, l := range links {
		isRef := false
		for _, r := range refs {
			if ref, ok := r.(map[string]any); ok {
				if id, _ := ref["id"].(string); id != "" && strings.Contains(l, "/status/"+id) {
					isRef = true
				}
			}
		}
		if !isRef {
			kept = append(kept, l)
		}
	}
	return text, kept
}

var substanceStripRe = regexp.MustCompile(`https?://\S+|@\w+`)

// SubstantiveChars counts what is left of the post's own text after
// removing links and @mentions — the short-reply filter (< 15 = skip).
func SubstantiveChars(p map[string]any) int {
	return len([]rune(strings.TrimSpace(substanceStripRe.ReplaceAllString(textFromRaw(p), ""))))
}

// urlEntities maps each t.co short link to its expanded_url, from the X
// API v2 shape entities.urls[] (or a flat urls[]). Optional: older raw
// files have neither and keep their t.co links.
func urlEntities(p map[string]any) map[string]string {
	out := map[string]string{}
	var lists []any
	if ent, ok := p["entities"].(map[string]any); ok {
		if l, ok := ent["urls"].([]any); ok {
			lists = append(lists, l...)
		}
	}
	if l, ok := p["urls"].([]any); ok {
		lists = append(lists, l...)
	}
	for _, it := range lists {
		u, ok := it.(map[string]any)
		if !ok {
			continue
		}
		short, _ := u["url"].(string)
		long, _ := u["expanded_url"].(string)
		if short != "" && strings.HasPrefix(long, "http") {
			out[short] = long
		}
	}
	return out
}

// expandShortLinks swaps t.co links in text for their expanded URLs.
func expandShortLinks(text string, m map[string]string) string {
	for short, long := range m {
		text = strings.ReplaceAll(text, short, long)
	}
	return text
}

// mediaLinkRe matches the expanded form of a photo/video attachment
// (x.com/<user>/status/<id>/photo/1): the tweet's own media, not a resource.
var mediaLinkRe = regexp.MustCompile(`^https?://(x|twitter)\.com/[^/]+/status/\d+/(photo|video)/\d+`)

// resourceLinks drops media-attachment links from the resource list.
func resourceLinks(links []string) []string {
	var out []string
	for _, l := range links {
		if !mediaLinkRe.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// referencedText appends the text of quoted / replied-to tweets when the
// raw item carries it (referenced_tweets[].text), so the page and the AI
// summary see what the post responds to. Missing text adds nothing.
func referencedText(p map[string]any) string {
	refs, _ := p["referenced_tweets"].([]any)
	var b strings.Builder
	for _, r := range refs {
		ref, ok := r.(map[string]any)
		if !ok {
			continue
		}
		text, _ := ref["text"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		label := "引用"
		if t, _ := ref["type"].(string); t == "replied_to" {
			label = "回复"
		}
		who, _ := ref["author_username"].(string)
		if who != "" {
			label += " @" + who
		}
		fmt.Fprintf(&b, "\n\n【%s】%s", label, expandShortLinks(strings.TrimSpace(text), urlEntities(ref)))
	}
	return b.String()
}
