package store

import (
	"database/sql"
	"encoding/json"

	"xdigest/internal/model"
)

// ---- posts ----

// PostFilter: see post_filter.go for the WHERE clause it builds.
type PostFilter struct {
	AuthorPresent bool
	Authors       []string
	Tag           string
	Tags          []string // any of (OR); Tag is folded in
	Category      string
	Categories    []string // any of (OR); "__uncat" = posts without a category
	Kind          string   // posts.kind; "未分类" = empty kind
	MinChars      int      // minimum length of text (fallback summary, then title)
	Query         string
	DateFrom      string // YYYY-MM-DD, Shanghai day start (inclusive)
	DateTo        string // YYYY-MM-DD, Shanghai day end (inclusive)
	Sort          string // "newest" (default) | "oldest"
	Page          int
	PageSize      int
}

// ListPosts implements the SPEC filter semantics (AND across dimensions,
// OR within one dimension's list). The plain query searches title,
// summary, tags, author name/handle, industry and tech.
func (s *Store) ListPosts(f PostFilter) (posts []model.Post, total int, updatedAt string, err error) {
	if f.AuthorPresent && len(f.Authors) == 0 {
		// Empty author selection = empty list (legacy semantics, not "no filter").
		return []model.Post{}, 0, "", nil
	}
	where, args := buildPostWhere(f)
	if err := s.DB.QueryRow("SELECT COALESCE(MAX(updated_at_meta), '') FROM posts").Scan(&updatedAt); err != nil {
		return nil, 0, "", err
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM posts p "+where, args...).Scan(&total); err != nil {
		return nil, 0, "", err
	}
	offset := (f.Page - 1) * f.PageSize
	qargs := append(append([]any{}, args...), f.PageSize, offset)
	rows, err := s.DB.Query(`SELECT p.id,p.url,p.author,p.author_name,COALESCE(NULLIF(p.author_avatar,''),(SELECT a.avatar FROM authors a WHERE a.username=p.author),''),p.industry,p.tech,
		p.title,p.summary,p.note,p.published_at,p.is_long,p.show_flowchart,COALESCE(p.text,''),COALESCE(p.links,''),p.kind,COALESCE(p.resources,''),p.replica_enabled
		FROM posts p `+where+` ORDER BY `+postOrder(f.Sort)+` LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()
	posts = []model.Post{}
	for rows.Next() {
		var p model.Post
		var show, replica sql.NullBool
		var note sql.NullString
		var links, resources string
		if err := rows.Scan(&p.ID, &p.URL, &p.Author, &p.AuthorName, &p.AuthorAvatar, &p.Industry, &p.Tech,
			&p.Title, &p.Summary, &note, &p.PublishedAt, &p.IsLong, &show, &p.Text, &links, &p.Kind, &resources, &replica); err != nil {
			return nil, 0, "", err
		}
		p.ReplicaEnabled = nullBoolPtr(replica)
		p.Note = note.String
		p.Links = decodeLinks(links)
		p.Resources = decodeResources(resources)
		if show.Valid {
			v := show.Bool
			p.ShowFlowchart = &v
		}
		p.Tags, err = s.postTags(p.ID)
		if err != nil {
			return nil, 0, "", err
		}
		p.Steps, err = s.postSteps(p.ID)
		if err != nil {
			return nil, 0, "", err
		}
		posts = append(posts, p)
	}
	return posts, total, updatedAt, rows.Err()
}

func nullBoolPtr(b sql.NullBool) *bool {
	if !b.Valid {
		return nil
	}
	v := b.Bool
	return &v
}

// SetPostReplicaEnabled sets the manual 复刻 switch (nil = back to the
// heuristic). sql.ErrNoRows for an unknown post.
func (s *Store) SetPostReplicaEnabled(id string, v *bool) error {
	var arg any
	if v != nil {
		arg = *v
	}
	res, err := s.DB.Exec("UPDATE posts SET replica_enabled=? WHERE id=?", arg, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 && !s.PostExists(id) {
		return sql.ErrNoRows
	}
	return nil
}

// decodeResources reads posts.resources (JSON array; "" = none).
func decodeResources(raw string) []model.PostResource {
	out := []model.PostResource{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// decodeLinks reads posts.links (a JSON array; "" = none).
func decodeLinks(raw string) []string {
	out := []string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func (s *Store) postTags(postID string) ([]string, error) {
	rows, err := s.DB.Query("SELECT tag FROM post_tags WHERE post_id=? ORDER BY tag", postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := emptyStrSlice()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) postSteps(postID string) ([]model.PostStep, error) {
	rows, err := s.DB.Query("SELECT text,tools,tech,prep,substeps_json FROM post_steps WHERE post_id=? ORDER BY idx", postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PostStep{}
	for rows.Next() {
		var st model.PostStep
		var sub string
		if err := rows.Scan(&st.Text, &st.Tools, &st.Tech, &st.Prep, &sub); err != nil {
			return nil, err
		}
		if sub != "" {
			if err := json.Unmarshal([]byte(sub), &st.Substeps); err != nil {
				return nil, err
			}
		}
		if st.Substeps == nil {
			st.Substeps = nil // keep omitted when empty
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) GetPost(id string) (*model.Post, error) {
	var p model.Post
	var show, replica sql.NullBool
	var note sql.NullString
	var links, resources string
	err := s.DB.QueryRow(`SELECT id,url,author,author_name,COALESCE(NULLIF(author_avatar,''),(SELECT a.avatar FROM authors a WHERE a.username=posts.author),''),industry,tech,title,summary,note,
		published_at,is_long,show_flowchart,COALESCE(text,''),COALESCE(links,''),kind,COALESCE(resources,''),replica_enabled FROM posts WHERE id=?`, id).Scan(
		&p.ID, &p.URL, &p.Author, &p.AuthorName, &p.AuthorAvatar, &p.Industry, &p.Tech,
		&p.Title, &p.Summary, &note, &p.PublishedAt, &p.IsLong, &show, &p.Text, &links, &p.Kind, &resources, &replica)
	if err != nil {
		return nil, err
	}
	p.Note = note.String
	p.Links = decodeLinks(links)
	p.Resources = decodeResources(resources)
	if show.Valid {
		v := show.Bool
		p.ShowFlowchart = &v
	}
	p.ReplicaEnabled = nullBoolPtr(replica)
	if p.Tags, err = s.postTags(id); err != nil {
		return nil, err
	}
	if p.Steps, err = s.postSteps(id); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- tags / categories ----

func (s *Store) PostTagsByFreq() ([]string, error) {
	rows, err := s.DB.Query("SELECT tag FROM post_tags GROUP BY tag COLLATE utf8mb4_bin ORDER BY COUNT(*) DESC, tag ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := emptyStrSlice()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
