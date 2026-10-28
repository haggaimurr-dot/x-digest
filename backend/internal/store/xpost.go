package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// XPostStep is one tutorial step of an analysed post.
type XPostStep struct {
	Text, Tools, Tech, Prep string
}

// XPostResource explains one link of an analysed post.
type XPostResource struct {
	URL  string `json:"url"`
	Name string `json:"name"`
	Note string `json:"note"`
}

// XPostRow is one analysed post written by the single-post import and the
// account refresh (the same columns cmd/import fills from posts.json).
type XPostRow struct {
	ID, URL, Author, AuthorName, AuthorAvatar string
	PublishedAt                               string // RFC3339
	Title, Industry, Tech, Kind               string
	Text                                      string
	Links                                     []string
	Tags                                      []string
	Steps                                     []XPostStep
	Resources                                 []XPostResource
	IsLong, ShowFlowchart                     bool
}

// EnsureAuthor makes sure the post's author is on the watchlist (default
// group for a new one) and fills an empty name/avatar/bio. created reports
// a new account.
func (s *Store) EnsureAuthor(username, name, avatar, bio string) (created bool, err error) {
	res, err := s.DB.Exec(`INSERT IGNORE INTO authors(username,name,avatar,group_id,bio) VALUES(?,?,?,'other',?)`,
		username, name, avatar, bio)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	_, err = s.DB.Exec(`UPDATE authors SET name=IF(name='',?,name), avatar=IF(avatar='',?,avatar), bio=IF(bio='',?,bio)
		WHERE username=?`, name, avatar, bio, username)
	return false, err
}

// PostExists reports whether a post id is already stored.
func (s *Store) PostExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM posts WHERE id=?", id).Scan(&n)
	return n > 0
}

// LatestPostTime is the newest stored post of an author (UTC), ok=false
// when it has none.
func (s *Store) LatestPostTime(username string) (t time.Time, ok bool, err error) {
	var v sql.NullString
	if err := s.DB.QueryRow("SELECT DATE_FORMAT(MAX(published_at),'%Y-%m-%d %H:%i:%s') FROM posts WHERE author=?", username).Scan(&v); err != nil {
		return t, false, err
	}
	if !v.Valid || v.String == "" {
		return t, false, nil
	}
	t, err = time.ParseInLocation("2006-01-02 15:04:05", v.String, shanghai)
	return t.UTC(), err == nil, err
}

// WatchUsernames lists every account on the watchlist.
func (s *Store) WatchUsernames() ([]string, error) {
	rows, err := s.DB.Query("SELECT username FROM authors ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpsertXPost writes one analysed post with its tags and steps. Like the
// importer it never stores an AI summary (that stays user-triggered) and
// never overwrites text/links/kind/resources with empty values.
func (s *Store) UpsertXPost(p XPostRow) error {
	published, err := ParsePublished(p.PublishedAt)
	if err != nil {
		return err
	}
	linksJSON, resJSON := "", ""
	if len(p.Links) > 0 {
		b, _ := json.Marshal(p.Links)
		linksJSON = string(b)
	}
	if len(p.Resources) > 0 {
		b, _ := json.Marshal(p.Resources)
		resJSON = string(b)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Importing a deleted post again (导入单帖) restores it. Error ignored so
	// imports keep working before schema.sql adds deleted_posts (MariaDB does
	// not abort the transaction on a failed statement).
	_, _ = tx.Exec("DELETE FROM deleted_posts WHERE id=?", p.ID)
	if _, err := tx.Exec(`INSERT INTO authors(username,name,avatar,group_id,bio) VALUES(?,?,?,'other','')
		ON DUPLICATE KEY UPDATE avatar=IF(avatar='',VALUES(avatar),avatar)`, p.Author, p.AuthorName, p.AuthorAvatar); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO posts(id,url,author,author_name,author_avatar,industry,tech,title,summary,note,
			published_at,is_long,show_flowchart,updated_at_meta,text,links,kind,resources) VALUES(?,?,?,?,?,?,?,?,'','',?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE url=VALUES(url), author=VALUES(author), author_name=VALUES(author_name),
			text=IF(VALUES(text)<>'',VALUES(text),text), links=IF(VALUES(links)<>'',VALUES(links),links),
			kind=IF(VALUES(kind)<>'',VALUES(kind),kind), resources=IF(VALUES(resources)<>'',VALUES(resources),resources),
			author_avatar=IF(VALUES(author_avatar)<>'',VALUES(author_avatar),author_avatar), industry=VALUES(industry), tech=VALUES(tech),
			title=VALUES(title), published_at=VALUES(published_at), is_long=VALUES(is_long),
			show_flowchart=VALUES(show_flowchart), updated_at_meta=VALUES(updated_at_meta)`,
		p.ID, p.URL, p.Author, p.AuthorName, p.AuthorAvatar, p.Industry, p.Tech, p.Title,
		published, boolInt(p.IsLong), p.ShowFlowchart, time.Now().In(shanghai).Format("2006-01-02 15:04:05"),
		p.Text, linksJSON, p.Kind, resJSON); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM post_steps WHERE post_id=?", p.ID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM post_tags WHERE post_id=?", p.ID); err != nil {
		return err
	}
	for i, st := range p.Steps {
		if _, err := tx.Exec(`INSERT INTO post_steps(post_id,idx,text,tools,tech,prep,substeps_json) VALUES(?,?,?,?,?,?,'null')`,
			p.ID, i, st.Text, st.Tools, st.Tech, st.Prep); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, t := range p.Tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		if _, err := tx.Exec("INSERT INTO post_tags(post_id,tag) VALUES(?,?)", p.ID, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// AuthorAvatar is the stored avatar of an account ("" when none), so a
// curated local avatar wins over the fetched X one.
func (s *Store) AuthorAvatar(username string) string {
	var v sql.NullString
	_ = s.DB.QueryRow("SELECT avatar FROM authors WHERE username=?", username).Scan(&v)
	return v.String
}
