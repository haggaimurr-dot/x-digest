// Package store implements all MariaDB access for the x-digest API.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"xdigest/internal/model"
	"xdigest/internal/translate"
)

var shanghai = time.FixedZone("CST", 8*3600)

// Now returns current Shanghai wall time formatted for DATETIME columns.
func Now() string { return time.Now().In(shanghai).Format("2006-01-02 15:04:05") }

// ParsePublished converts an RFC3339 timestamp (legacy JSON, usually +08:00)
// to Shanghai wall time for DATETIME storage.
func ParsePublished(s string) (string, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "", err
	}
	return t.In(shanghai).Format("2006-01-02 15:04:05"), nil
}

// NewID returns a random hex id with the given prefix (grp_, cat_, rep_, att_).
func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

type Store struct{ DB *sql.DB }

func New(dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Ping() error { return s.DB.Ping() }

func nullBool(b *bool) sql.NullBool {
	if b == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *b, Valid: true}
}

func emptyStrSlice() []string { return []string{} }

// ---- posts ----

type PostFilter struct {
	AuthorPresent bool
	Authors       []string
	Tag           string
	Category      string
	UserTag       string
	Query         string
	DateFrom      string // YYYY-MM-DD, Shanghai day start (inclusive)
	DateTo        string // YYYY-MM-DD, Shanghai day end (inclusive)
	Page          int
	PageSize      int
}

// ListPosts implements the SPEC filter semantics (AND across dimensions,
// OR within the author list). Query prefixes 标签:/tag: match user tags only
// (author_tags + user_tag_overrides). The plain query searches title,
// summary, tags, author name/handle, industry and tech.
func (s *Store) ListPosts(f PostFilter) (posts []model.Post, total int, updatedAt string, err error) {
	var conds []string
	var args []any
	if f.AuthorPresent && len(f.Authors) == 0 {
		// Empty author selection = empty list (legacy semantics, not "no filter").
		return []model.Post{}, 0, "", nil
	}
	if len(f.Authors) > 0 {
		ph := strings.Repeat("?,", len(f.Authors))
		conds = append(conds, "p.author IN ("+strings.TrimSuffix(ph, ",")+")")
		for _, a := range f.Authors {
			args = append(args, a)
		}
	}
	if f.Tag != "" {
		// BINARY collation: the legacy JS match is case-sensitive, and the
		// source has both "workbuddy" and "Workbuddy" as distinct tags.
		conds = append(conds, "EXISTS (SELECT 1 FROM post_tags t WHERE t.post_id=p.id AND t.tag COLLATE utf8mb4_bin = ?)")
		args = append(args, strings.TrimPrefix(f.Tag, "#"))
	}
	if f.Category != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM post_categories pc WHERE pc.post_id=p.id AND pc.category_id=?)")
		args = append(args, f.Category)
	}
	if f.UserTag != "" {
		conds = append(conds, `(EXISTS (SELECT 1 FROM author_tags at WHERE at.username=p.author AND at.tag COLLATE utf8mb4_bin = ?)
			OR EXISTS (SELECT 1 FROM user_tag_overrides uo WHERE uo.username=p.author AND uo.tag COLLATE utf8mb4_bin = ?))`)
		args = append(args, f.UserTag, f.UserTag)
	}
	if f.Query != "" {
		q := f.Query
		if strings.HasPrefix(q, "标签:") || strings.HasPrefix(q, "tag:") {
			ut := strings.TrimPrefix(strings.TrimPrefix(q, "标签:"), "tag:")
			ut = strings.TrimSpace(ut)
			conds = append(conds, `(EXISTS (SELECT 1 FROM author_tags at WHERE at.username=p.author AND at.tag COLLATE utf8mb4_bin = ?)
				OR EXISTS (SELECT 1 FROM user_tag_overrides uo WHERE uo.username=p.author AND uo.tag COLLATE utf8mb4_bin = ?))`)
			args = append(args, ut, ut)
		} else {
			like := "%" + strings.TrimPrefix(q, "#") + "%"
			conds = append(conds, `(p.title LIKE ? OR p.summary LIKE ? OR p.industry LIKE ? OR p.tech LIKE ?
				OR p.author_name LIKE ? OR p.author LIKE ? OR p.text LIKE ?
				OR EXISTS (SELECT 1 FROM post_tags t WHERE t.post_id=p.id AND t.tag LIKE ?))`)
			for i := 0; i < 8; i++ {
				args = append(args, like)
			}
		}
	}
	if f.DateFrom != "" {
		conds = append(conds, "p.published_at >= ?")
		args = append(args, f.DateFrom+" 00:00:00")
	}
	if f.DateTo != "" {
		conds = append(conds, "p.published_at < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.DateTo+" 00:00:00")
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	if err := s.DB.QueryRow("SELECT COALESCE(MAX(updated_at_meta), '') FROM posts").Scan(&updatedAt); err != nil {
		return nil, 0, "", err
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM posts p "+where, args...).Scan(&total); err != nil {
		return nil, 0, "", err
	}
	offset := (f.Page - 1) * f.PageSize
	qargs := append(append([]any{}, args...), f.PageSize, offset)
	rows, err := s.DB.Query(`SELECT p.id,p.url,p.author,p.author_name,COALESCE(NULLIF(p.author_avatar,''),(SELECT a.avatar FROM authors a WHERE a.username=p.author),''),p.industry,p.tech,
		p.title,p.summary,p.note,p.published_at,p.is_long,p.show_flowchart,COALESCE(p.text,''),COALESCE(p.links,''),p.kind,COALESCE(p.resources,'')
		FROM posts p `+where+` ORDER BY p.published_at DESC, p.id ASC LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()
	posts = []model.Post{}
	for rows.Next() {
		var p model.Post
		var show sql.NullBool
		var note sql.NullString
		var links, resources string
		if err := rows.Scan(&p.ID, &p.URL, &p.Author, &p.AuthorName, &p.AuthorAvatar, &p.Industry, &p.Tech,
			&p.Title, &p.Summary, &note, &p.PublishedAt, &p.IsLong, &show, &p.Text, &links, &p.Kind, &resources); err != nil {
			return nil, 0, "", err
		}
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
	var show sql.NullBool
	var note sql.NullString
	var links, resources string
	err := s.DB.QueryRow(`SELECT id,url,author,author_name,COALESCE(NULLIF(author_avatar,''),(SELECT a.avatar FROM authors a WHERE a.username=posts.author),''),industry,tech,title,summary,note,
		published_at,is_long,show_flowchart,COALESCE(text,''),COALESCE(links,''),kind,COALESCE(resources,'') FROM posts WHERE id=?`, id).Scan(
		&p.ID, &p.URL, &p.Author, &p.AuthorName, &p.AuthorAvatar, &p.Industry, &p.Tech,
		&p.Title, &p.Summary, &note, &p.PublishedAt, &p.IsLong, &show, &p.Text, &links, &p.Kind, &resources)
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
	if p.Tags, err = s.postTags(id); err != nil {
		return nil, err
	}
	if p.Steps, err = s.postSteps(id); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- watchlist ----

func (s *Store) ListWatchlist() (groups []model.Group, accounts []model.Account, err error) {
	grows, err := s.DB.Query("SELECT id,name,fixed FROM watchlist_groups ORDER BY id")
	if err != nil {
		return nil, nil, err
	}
	defer grows.Close()
	groups = []model.Group{}
	for grows.Next() {
		var g model.Group
		if err := grows.Scan(&g.ID, &g.Name, &g.Fixed); err != nil {
			return nil, nil, err
		}
		groups = append(groups, g)
	}
	if err := grows.Err(); err != nil {
		return nil, nil, err
	}
	arows, err := s.DB.Query("SELECT username,name,avatar,group_id,bio FROM authors ORDER BY username")
	if err != nil {
		return nil, nil, err
	}
	defer arows.Close()
	accounts = []model.Account{}
	for arows.Next() {
		var a model.Account
		var gid sql.NullString
		if err := arows.Scan(&a.Username, &a.Name, &a.Avatar, &gid, &a.Bio); err != nil {
			return nil, nil, err
		}
		// Authors with no group (NULL, e.g. imported from posts before any
		// watchlist assignment, or orphaned by an old group delete) belong
		// to the default group so they stay visible and manageable.
		if gid.Valid && gid.String != "" {
			a.Group = gid.String
		} else {
			a.Group = "other"
		}
		a.Tags, err = s.authorTags(a.Username)
		if err != nil {
			return nil, nil, err
		}
		accounts = append(accounts, a)
	}
	return groups, accounts, arows.Err()
}

func (s *Store) authorTags(username string) ([]string, error) {
	rows, err := s.DB.Query("SELECT tag FROM author_tags WHERE username=? ORDER BY tag", username)
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

func (s *Store) CreateGroup(name string) (model.Group, error) {
	g := model.Group{ID: NewID("grp_"), Name: name}
	_, err := s.DB.Exec("INSERT INTO watchlist_groups(id,name,fixed) VALUES(?,?,0)", g.ID, g.Name)
	return g, err
}

func (s *Store) DeleteGroup(id string) (code string, err error) {
	var fixed bool
	err = s.DB.QueryRow("SELECT fixed FROM watchlist_groups WHERE id=?", id).Scan(&fixed)
	if err != nil {
		return "NOT_FOUND", err
	}
	if fixed {
		return "GROUP_FIXED", fmt.Errorf("fixed group cannot be deleted")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Guarantee the default group exists before reassigning into it; the
	// reassign and the delete must land together or not at all, otherwise
	// authors are orphaned (NULL) and vanish from the watchlist response.
	if _, err := tx.Exec("INSERT IGNORE INTO watchlist_groups(id,name,fixed) VALUES('other','默认',1)"); err != nil {
		return "", err
	}
	if _, err := tx.Exec("UPDATE authors SET group_id='other' WHERE group_id=?", id); err != nil {
		return "", err
	}
	if _, err := tx.Exec("DELETE FROM watchlist_groups WHERE id=?", id); err != nil {
		return "", err
	}
	return "", tx.Commit()
}

func (s *Store) GroupExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM watchlist_groups WHERE id=?", id).Scan(&n)
	return n > 0
}

func (s *Store) AccountExists(username string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM authors WHERE username=?", username).Scan(&n)
	return n > 0
}

func (s *Store) CreateAccount(a model.Account) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO authors(username,name,avatar,group_id,bio) VALUES(?,?,?,?,?)",
		a.Username, a.Name, a.Avatar, a.Group, a.Bio); err != nil {
		return err
	}
	for _, t := range a.Tags {
		if _, err := tx.Exec("INSERT IGNORE INTO author_tags(username,tag) VALUES(?,?)", a.Username, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PatchAccount updates only non-nil fields; tags (when non-nil) replace the set.
func (s *Store) PatchAccount(username string, name, avatar, group, bio *string, tags *[]string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sets []string
	var args []any
	if name != nil {
		sets, args = append(sets, "name=?"), append(args, *name)
	}
	if avatar != nil {
		sets, args = append(sets, "avatar=?"), append(args, *avatar)
	}
	if group != nil {
		sets, args = append(sets, "group_id=?"), append(args, *group)
	}
	if bio != nil {
		sets, args = append(sets, "bio=?"), append(args, *bio)
	}
	if len(sets) > 0 {
		args = append(args, username)
		if _, err := tx.Exec("UPDATE authors SET "+strings.Join(sets, ",")+" WHERE username=?", args...); err != nil {
			return err
		}
	}
	if tags != nil {
		if _, err := tx.Exec("DELETE FROM author_tags WHERE username=?", username); err != nil {
			return err
		}
		for _, t := range *tags {
			if _, err := tx.Exec("INSERT IGNORE INTO author_tags(username,tag) VALUES(?,?)", username, t); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteAccount(username string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM author_tags WHERE username=?", username); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM user_tag_overrides WHERE username=?", username); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM authors WHERE username=?", username); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- tags / user-tags / categories ----

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

func (s *Store) UserTags() (vocab []string, overrides map[string][]string, err error) {
	rows, err := s.DB.Query("SELECT tag FROM user_tag_vocabulary ORDER BY tag")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	vocab = emptyStrSlice()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, nil, err
		}
		vocab = append(vocab, t)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	orows, err := s.DB.Query("SELECT username,tag FROM user_tag_overrides ORDER BY username,tag")
	if err != nil {
		return nil, nil, err
	}
	defer orows.Close()
	overrides = map[string][]string{}
	for orows.Next() {
		var u, t string
		if err := orows.Scan(&u, &t); err != nil {
			return nil, nil, err
		}
		overrides[u] = append(overrides[u], t)
	}
	return vocab, overrides, orows.Err()
}

func (s *Store) SetUserTagOverride(username string, tags []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM user_tag_overrides WHERE username=?", username); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := tx.Exec("INSERT INTO user_tag_overrides(username,tag) VALUES(?,?)", username, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListCategories() (cats []model.Category, assignments map[string][]string, err error) {
	rows, err := s.DB.Query("SELECT id,name FROM categories ORDER BY name")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cats = []model.Category{}
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, nil, err
		}
		cats = append(cats, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	arows, err := s.DB.Query("SELECT post_id,category_id FROM post_categories ORDER BY post_id,category_id")
	if err != nil {
		return nil, nil, err
	}
	defer arows.Close()
	assignments = map[string][]string{}
	for arows.Next() {
		var p, c string
		if err := arows.Scan(&p, &c); err != nil {
			return nil, nil, err
		}
		assignments[p] = append(assignments[p], c)
	}
	return cats, assignments, arows.Err()
}

func (s *Store) CreateCategory(name string) (model.Category, error) {
	c := model.Category{ID: NewID("cat_"), Name: name}
	_, err := s.DB.Exec("INSERT INTO categories(id,name) VALUES(?,?)", c.ID, c.Name)
	return c, err
}

func (s *Store) DeleteCategory(id string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM post_categories WHERE category_id=?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM categories WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetPostCategories(postID string, catIDs []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM posts WHERE id=?", postID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	for _, c := range catIDs {
		var m int
		if err := tx.QueryRow("SELECT COUNT(*) FROM categories WHERE id=?", c).Scan(&m); err != nil {
			return err
		}
		if m == 0 {
			return fmt.Errorf("unknown category: %s", c)
		}
	}
	if _, err := tx.Exec("DELETE FROM post_categories WHERE post_id=?", postID); err != nil {
		return err
	}
	for _, c := range catIDs {
		if _, err := tx.Exec("INSERT INTO post_categories(post_id,category_id) VALUES(?,?)", postID, c); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- replicas ----

func (s *Store) GetReplica(id string, attachDir string) (*model.Replica, error) {
	var r model.Replica
	err := s.DB.QueryRow("SELECT id,post_id,post_url,post_author,post_author_name,post_title,created_at FROM replicas WHERE id=?", id).
		Scan(&r.ID, &r.PostID, &r.PostURL, &r.PostAuthor, &r.PostAuthorName, &r.PostTitle, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	steps, err := s.replicaSteps(id)
	if err != nil {
		return nil, err
	}
	r.Steps = steps
	for _, st := range steps {
		if st.Status == "done" {
			r.Progress.Done++
		}
	}
	r.Progress.Total = len(steps)
	return &r, nil
}

func (s *Store) replicaSteps(replicaID string) ([]model.ReplicaStep, error) {
	rows, err := s.DB.Query(`SELECT idx,text,tools,tech,prep,substeps_json,status,note,is_custom,parent_idx FROM replica_steps
		WHERE replica_id=? ORDER BY idx`, replicaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := []model.ReplicaStep{}
	for rows.Next() {
		var st model.ReplicaStep
		var sub string
		var parent sql.NullInt64
		if err := rows.Scan(&st.Idx, &st.Text, &st.Tools, &st.Tech, &st.Prep, &sub, &st.Status, &st.Note, &st.IsCustom, &parent); err != nil {
			return nil, err
		}
		if parent.Valid {
			p := int(parent.Int64)
			st.ParentIdx = &p
		}
		if sub != "" {
			_ = json.Unmarshal([]byte(sub), &st.Substeps)
		}
		atts, err := s.replicaStepAttachments(replicaID, st.Idx)
		if err != nil {
			return nil, err
		}
		st.Attachments = atts
		all = append(all, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Display order: each top-level node followed immediately by its children
	// (both in idx order), so a child always sits under its parent.
	children := map[int][]model.ReplicaStep{}
	out := []model.ReplicaStep{}
	for _, st := range all {
		if st.ParentIdx != nil {
			children[*st.ParentIdx] = append(children[*st.ParentIdx], st)
		}
	}
	for _, st := range all {
		if st.ParentIdx != nil {
			continue
		}
		out = append(out, st)
		out = append(out, children[st.Idx]...)
	}
	return out, nil
}

func (s *Store) replicaStepAttachments(replicaID string, stepIdx int) ([]model.Attachment, error) {
	rows, err := s.DB.Query(`SELECT id,replica_id,step_idx,name,size,content_type,ext,created_at FROM attachments
		WHERE replica_id=? AND step_idx=? ORDER BY created_at`, replicaID, stepIdx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Attachment{}
	for rows.Next() {
		var a model.Attachment
		if err := rows.Scan(&a.ID, &a.ReplicaID, &a.StepIdx, &a.Name, &a.Size, &a.ContentType, &a.Ext, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.DownloadURL = "/api/v1/attachments/" + a.ID + "/download"
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListReplicas(sort string) ([]model.Replica, error) {
	rows, err := s.DB.Query("SELECT id FROM replicas ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []model.Replica{}
	for _, id := range ids {
		r, err := s.GetReplica(id, "")
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	if sort == "progress" {
		// Stable sort by done/total ratio desc, then created_at desc.
		for i := 1; i < len(out); i++ {
			for j := i; j > 0; j-- {
				if ratio(out[j]) > ratio(out[j-1]) {
					out[j], out[j-1] = out[j-1], out[j]
				} else {
					break
				}
			}
		}
	}
	return out, nil
}

func ratio(r model.Replica) float64 {
	if r.Progress.Total == 0 {
		return 0
	}
	return float64(r.Progress.Done) / float64(r.Progress.Total)
}

// CreateStandaloneReplica starts a blank flow from scratch: no X post, no
// snapshot steps. The user builds it entirely from custom nodes.
// post_id stays ” (no UNIQUE constraint on it, so many blank flows coexist).
func (s *Store) CreateStandaloneReplica(title string) (*model.Replica, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("empty replica title")
	}
	id := NewID("rep_")
	if _, err := s.DB.Exec(`INSERT INTO replicas(id,post_id,post_url,post_author,post_author_name,post_title,created_at)
		VALUES(?, '', '', '', '自建', ?, ?)`, id, title, Now()); err != nil {
		return nil, err
	}
	return s.GetReplica(id, "")
}

// CreateReplicaFromPost snapshots the post's current steps. If a replica for
// the post already exists it is returned with existing=true and no new row.
func (s *Store) CreateReplicaFromPost(postID string) (*model.Replica, bool, error) {
	var existingID string
	err := s.DB.QueryRow("SELECT id FROM replicas WHERE post_id=? LIMIT 1", postID).Scan(&existingID)
	if err == nil {
		r, err := s.GetReplica(existingID, "")
		if err != nil {
			return nil, false, err
		}
		r.Existing = true
		return r, true, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, err
	}
	p, err := s.GetPost(postID)
	if err != nil {
		return nil, false, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	id := NewID("rep_")
	if _, err := tx.Exec(`INSERT INTO replicas(id,post_id,post_url,post_author,post_author_name,post_title,created_at)
		VALUES(?,?,?,?,?,?,?)`, id, p.ID, p.URL, p.Author, p.AuthorName, p.Title, Now()); err != nil {
		return nil, false, err
	}
	for i, st := range p.Steps {
		sub, _ := json.Marshal(st.Substeps)
		if _, err := tx.Exec(`INSERT INTO replica_steps(replica_id,idx,text,tools,tech,prep,substeps_json,status,note)
			VALUES(?,?,?,?,?,?,?,?,?)`, id, i, st.Text, st.Tools, st.Tech, st.Prep, string(sub), "idle", ""); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	r, err := s.GetReplica(id, "")
	return r, false, err
}

type StepPatch struct {
	Idx    int
	Status *string
	Note   *string
	Text   *string
}

// AddReplicaStep appends a user-created (custom) flow node to a replica.
// Custom nodes have no tools/tech/prep/substeps; they support status, note,
// text edits and attachments exactly like post-derived steps.
// With parentIdx set, the node becomes a child of that top-level node;
// at most two levels are supported, so a child cannot have children.
func (s *Store) AddReplicaStep(id, text string, parentIdx *int) (model.ReplicaStep, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return model.ReplicaStep{}, fmt.Errorf("empty step text")
	}
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM replicas WHERE id=?", id).Scan(&n); err != nil {
		return model.ReplicaStep{}, err
	}
	if n == 0 {
		return model.ReplicaStep{}, sql.ErrNoRows
	}
	if parentIdx != nil {
		var parentParent sql.NullInt64
		err := s.DB.QueryRow("SELECT parent_idx FROM replica_steps WHERE replica_id=? AND idx=?", id, *parentIdx).Scan(&parentParent)
		if err != nil {
			return model.ReplicaStep{}, fmt.Errorf("unknown parent step: %d", *parentIdx)
		}
		if parentParent.Valid {
			return model.ReplicaStep{}, fmt.Errorf("sub-nodes support at most two levels")
		}
	}
	var maxIdx sql.NullInt64
	if err := s.DB.QueryRow("SELECT MAX(idx) FROM replica_steps WHERE replica_id=?", id).Scan(&maxIdx); err != nil {
		return model.ReplicaStep{}, err
	}
	idx := 0
	if maxIdx.Valid {
		idx = int(maxIdx.Int64) + 1
	}
	st := model.ReplicaStep{Idx: idx, Text: text, Status: "idle", Note: "", IsCustom: true, ParentIdx: parentIdx, Attachments: []model.Attachment{}}
	var parentArg any
	if parentIdx != nil {
		parentArg = *parentIdx
	}
	_, err := s.DB.Exec(`INSERT INTO replica_steps(replica_id,idx,text,tools,tech,prep,substeps_json,status,note,is_custom,parent_idx)
		VALUES(?,?,?,'','', '', '[]', 'idle', '', 1, ?)`, id, idx, text, parentArg)
	if err != nil {
		return model.ReplicaStep{}, err
	}
	return st, nil
}

// DeleteReplicaStep removes a custom node only; post-derived steps cannot be
// deleted through this path (they belong to the post snapshot). Deleting a
// top-level node also deletes its child nodes. Returns the disk paths of the
// deleted nodes' attachments so the caller can unlink the files.
func (s *Store) DeleteReplicaStep(id string, idx int) ([]string, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("DELETE FROM replica_steps WHERE replica_id=? AND idx=? AND is_custom=1", id, idx)
	if err != nil {
		return nil, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil, fmt.Errorf("unknown step idx: %d", idx)
	}
	var childIdxs []int
	rows, err := tx.Query("SELECT idx FROM replica_steps WHERE replica_id=? AND parent_idx=?", id, idx)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ci int
		if err := rows.Scan(&ci); err != nil {
			rows.Close()
			return nil, err
		}
		childIdxs = append(childIdxs, ci)
	}
	rows.Close()
	if _, err := tx.Exec("DELETE FROM replica_steps WHERE replica_id=? AND parent_idx=?", id, idx); err != nil {
		return nil, err
	}
	delIdxs := append([]int{idx}, childIdxs...)
	var paths []string
	for _, di := range delIdxs {
		prows, err := tx.Query("SELECT path FROM attachments WHERE replica_id=? AND step_idx=?", id, di)
		if err != nil {
			return nil, err
		}
		for prows.Next() {
			var p string
			if err := prows.Scan(&p); err != nil {
				prows.Close()
				return nil, err
			}
			paths = append(paths, p)
		}
		prows.Close()
		if _, err := tx.Exec("DELETE FROM attachments WHERE replica_id=? AND step_idx=?", id, di); err != nil {
			return nil, err
		}
	}
	return paths, tx.Commit()
}

func (s *Store) PatchReplicaSteps(id string, patches []StepPatch) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM replicas WHERE id=?", id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	for _, p := range patches {
		if p.Status != nil && *p.Status != "idle" && *p.Status != "active" && *p.Status != "done" {
			return fmt.Errorf("invalid status: %s", *p.Status)
		}
		if p.Text != nil {
			// Step text is editable only on user-created nodes; post-derived
			// steps keep the post snapshot.
			res, err := tx.Exec("UPDATE replica_steps SET text=? WHERE replica_id=? AND idx=? AND is_custom=1",
				strings.TrimSpace(*p.Text), id, p.Idx)
			if err != nil {
				return err
			}
			aff, _ := res.RowsAffected()
			if aff == 0 {
				return fmt.Errorf("step text is editable only on custom steps: %d", p.Idx)
			}
		}
		var sets []string
		var args []any
		if p.Status != nil {
			sets, args = append(sets, "status=?"), append(args, *p.Status)
		}
		if p.Note != nil {
			sets, args = append(sets, "note=?"), append(args, *p.Note)
		}
		if len(sets) == 0 {
			continue
		}
		args = append(args, id, p.Idx)
		res, err := tx.Exec("UPDATE replica_steps SET "+strings.Join(sets, ",")+" WHERE replica_id=? AND idx=?", args...)
		if err != nil {
			return err
		}
		aff, _ := res.RowsAffected()
		if aff == 0 {
			return fmt.Errorf("unknown step idx: %d", p.Idx)
		}
	}
	return tx.Commit()
}

// DeleteReplica returns the disk paths of its attachments (caller deletes files).
func (s *Store) DeleteReplica(id string) ([]string, error) {
	var paths []string
	rows, err := s.DB.Query("SELECT path FROM attachments WHERE replica_id=?", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, p)
	}
	rows.Close()
	if _, err := s.DB.Exec("DELETE FROM replicas WHERE id=?", id); err != nil {
		return nil, err
	}
	return paths, nil
}

func (s *Store) ReplicaStepExists(replicaID string, idx int) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM replica_steps WHERE replica_id=? AND idx=?", replicaID, idx).Scan(&n)
	return n > 0
}

func (s *Store) ReplicaExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM replicas WHERE id=?", id).Scan(&n)
	return n > 0
}

// ---- attachments ----

func (s *Store) CreateAttachment(a model.Attachment, path string) error {
	_, err := s.DB.Exec(`INSERT INTO attachments(id,replica_id,step_idx,name,size,content_type,ext,path,created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, a.ID, a.ReplicaID, a.StepIdx, a.Name, a.Size, a.ContentType, a.Ext, path, a.CreatedAt)
	return err
}

func (s *Store) GetAttachment(id string) (*model.Attachment, string, error) {
	var a model.Attachment
	var path string
	err := s.DB.QueryRow(`SELECT id,replica_id,step_idx,name,size,content_type,ext,path,created_at FROM attachments WHERE id=?`, id).
		Scan(&a.ID, &a.ReplicaID, &a.StepIdx, &a.Name, &a.Size, &a.ContentType, &a.Ext, &path, &a.CreatedAt)
	if err != nil {
		return nil, "", err
	}
	a.DownloadURL = "/api/v1/attachments/" + a.ID + "/download"
	return &a, path, nil
}

func (s *Store) DeleteAttachment(id string) (string, error) {
	var path string
	err := s.DB.QueryRow("SELECT path FROM attachments WHERE id=?", id).Scan(&path)
	if err != nil {
		return "", err
	}
	if _, err := s.DB.Exec("DELETE FROM attachments WHERE id=?", id); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) ListReplicaAttachments(replicaID string) ([]model.Attachment, error) {
	rows, err := s.DB.Query(`SELECT id,replica_id,step_idx,name,size,content_type,ext,created_at FROM attachments
		WHERE replica_id=? ORDER BY step_idx, created_at`, replicaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Attachment{}
	for rows.Next() {
		var a model.Attachment
		if err := rows.Scan(&a.ID, &a.ReplicaID, &a.StepIdx, &a.Name, &a.Size, &a.ContentType, &a.Ext, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.DownloadURL = "/api/v1/attachments/" + a.ID + "/download"
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- GitHub repos ----

type RepoFilter struct {
	Group string
	Tag   string
	Sort  string // "stars" or "pushed_at" (default)
}

func nullInt(p *int) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(*p), Valid: true}
}

func nullStr(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func (s *Store) repoTags(fullName string) ([]string, error) {
	rows, err := s.DB.Query("SELECT tag FROM repo_tags WHERE full_name=? ORDER BY tag", fullName)
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

func (s *Store) scanRepoRow(r *model.Repo, description, readmeIntro, tutorialSteps, language, htmlURL, homepage sql.NullString,
	openIssues, openPRs sql.NullInt32, pushedAt, repoUpdatedAt, lastSyncedAt sql.NullString,
	groupID, note, syncError sql.NullString) error {
	r.Description = description.String
	r.ReadmeIntro = readmeIntro.String
	if tutorialSteps.Valid && strings.TrimSpace(tutorialSteps.String) != "" {
		var steps []model.TutorialStep
		if err := json.Unmarshal([]byte(tutorialSteps.String), &steps); err == nil {
			r.TutorialSteps = steps
		}
		// A corrupt tutorial_steps value degrades to nil rather than
		// failing the whole row query.
	}
	r.Language = language.String
	r.HTMLURL = htmlURL.String
	r.Homepage = homepage.String
	if openIssues.Valid {
		v := int(openIssues.Int32)
		r.OpenIssues = &v
	}
	if openPRs.Valid {
		v := int(openPRs.Int32)
		r.OpenPRs = &v
	}
	if pushedAt.Valid {
		v := pushedAt.String
		r.PushedAt = &v
	}
	if repoUpdatedAt.Valid {
		v := repoUpdatedAt.String
		r.RepoUpdatedAt = &v
	}
	if lastSyncedAt.Valid {
		v := lastSyncedAt.String
		r.LastSyncedAt = &v
	}
	r.GroupID = groupID.String
	r.Note = note.String
	r.SyncError = syncError.String
	return nil
}

const repoColumns = `full_name,owner,name,description,readme_intro,tutorial_steps,language,stars,forks,open_issues,open_prs,
	html_url,homepage,pushed_at,repo_updated_at,group_id,note,last_synced_at,sync_status,sync_error`

func (s *Store) scanRepos(rows *sql.Rows) ([]model.Repo, error) {
	out := []model.Repo{}
	for rows.Next() {
		var r model.Repo
		var description, readmeIntro, tutorialSteps, language, htmlURL, homepage sql.NullString
		var openIssues, openPRs sql.NullInt32
		var pushedAt, repoUpdatedAt, lastSyncedAt sql.NullString
		var groupID, note, syncError sql.NullString
		if err := rows.Scan(&r.FullName, &r.Owner, &r.Name, &description, &readmeIntro, &tutorialSteps, &language,
			&r.Stars, &r.Forks, &openIssues, &openPRs, &htmlURL, &homepage,
			&pushedAt, &repoUpdatedAt, &groupID, &note, &lastSyncedAt,
			&r.SyncStatus, &syncError); err != nil {
			return nil, err
		}
		if err := s.scanRepoRow(&r, description, readmeIntro, tutorialSteps, language, htmlURL, homepage, openIssues, openPRs,
			pushedAt, repoUpdatedAt, lastSyncedAt, groupID, note, syncError); err != nil {
			return nil, err
		}
		tags, err := s.repoTags(r.FullName)
		if err != nil {
			return nil, err
		}
		r.Tags = tags
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListRepos(f RepoFilter) ([]model.Repo, error) {
	var conds []string
	var args []any
	if f.Group != "" {
		conds = append(conds, "r.group_id=?")
		args = append(args, f.Group)
	}
	if f.Tag != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM repo_tags t WHERE t.full_name=r.full_name AND t.tag=?)")
		args = append(args, f.Tag)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	order := "r.pushed_at DESC, r.full_name ASC"
	if f.Sort == "stars" {
		order = "r.stars DESC, r.full_name ASC"
	}
	rows, err := s.DB.Query("SELECT "+repoColumns+" FROM repos r "+where+" ORDER BY "+order, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRepos(rows)
}

func (s *Store) GetRepo(fullName string) (*model.Repo, error) {
	var r model.Repo
	var description, readmeIntro, tutorialSteps, language, htmlURL, homepage sql.NullString
	var openIssues, openPRs sql.NullInt32
	var pushedAt, repoUpdatedAt, lastSyncedAt sql.NullString
	var groupID, note, syncError sql.NullString
	err := s.DB.QueryRow("SELECT "+repoColumns+" FROM repos WHERE full_name=?", fullName).Scan(
		&r.FullName, &r.Owner, &r.Name, &description, &readmeIntro, &tutorialSteps, &language,
		&r.Stars, &r.Forks, &openIssues, &openPRs, &htmlURL, &homepage,
		&pushedAt, &repoUpdatedAt, &groupID, &note, &lastSyncedAt,
		&r.SyncStatus, &syncError)
	if err != nil {
		return nil, err
	}
	if err := s.scanRepoRow(&r, description, readmeIntro, tutorialSteps, language, htmlURL, homepage, openIssues, openPRs,
		pushedAt, repoUpdatedAt, lastSyncedAt, groupID, note, syncError); err != nil {
		return nil, err
	}
	if r.Tags, err = s.repoTags(fullName); err != nil {
		return nil, err
	}
	if r.Commits, err = s.ListRepoCommits(fullName, 10); err != nil {
		return nil, err
	}
	rels, err := s.ListRepoReleases(fullName, 1)
	if err != nil {
		return nil, err
	}
	if len(rels) > 0 {
		r.LatestRelease = &rels[0]
	}
	return &r, nil
}

func (s *Store) RepoExists(fullName string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM repos WHERE full_name=?", fullName).Scan(&n)
	return n > 0
}

func (s *Store) CreateRepo(r model.Repo) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO repos(full_name,owner,name,description,language,stars,forks,
		open_issues,open_prs,html_url,homepage,pushed_at,repo_updated_at,group_id,note,
		last_synced_at,sync_status,sync_error)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.FullName, r.Owner, r.Name, r.Description, r.Language, r.Stars, r.Forks,
		nullInt(r.OpenIssues), nullInt(r.OpenPRs), r.HTMLURL, r.Homepage,
		nullStr(r.PushedAt), nullStr(r.RepoUpdatedAt), groupIDArg(r.GroupID), r.Note,
		nullStr(r.LastSyncedAt), "never", ""); err != nil {
		return err
	}
	for _, t := range r.Tags {
		if _, err := tx.Exec("INSERT IGNORE INTO repo_tags(full_name,tag) VALUES(?,?)", r.FullName, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// groupIDArg maps "" to NULL so an empty group means "no group".
func groupIDArg(g string) any {
	if g == "" {
		return nil
	}
	return g
}

// PatchRepo updates only non-nil fields; tags (when non-nil) replace the set.
// A non-nil group_id of "" clears the group (NULL).
func (s *Store) PatchRepo(fullName string, groupID *string, tags *[]string, note *string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sets []string
	var args []any
	if groupID != nil {
		sets, args = append(sets, "group_id=?"), append(args, groupIDArg(*groupID))
	}
	if note != nil {
		sets, args = append(sets, "note=?"), append(args, *note)
	}
	if len(sets) > 0 {
		args = append(args, fullName)
		if _, err := tx.Exec("UPDATE repos SET "+strings.Join(sets, ",")+" WHERE full_name=?", args...); err != nil {
			return err
		}
	}
	if tags != nil {
		if _, err := tx.Exec("DELETE FROM repo_tags WHERE full_name=?", fullName); err != nil {
			return err
		}
		for _, t := range *tags {
			if _, err := tx.Exec("INSERT IGNORE INTO repo_tags(full_name,tag) VALUES(?,?)", fullName, t); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteRepo(fullName string) error {
	_, err := s.DB.Exec("DELETE FROM repos WHERE full_name=?", fullName)
	return err
}

// UpdateRepoMeta applies one sync's metadata to an existing row. Nil
// OpenIssues/OpenPRs write NULL (unknown, e.g. a failed search) rather
// than inventing a number. Commits/releases are upserted separately by
// the caller, only when that fetch succeeded (non-nil slices).
// ReadmeIntro is written as-is ("" when the README has no intro) and
// TutorialSteps as JSON, or NULL when there are none.
func (s *Store) UpdateRepoMeta(fullName string, r model.Repo, status, syncErr string) error {
	var tutorialSteps any
	if len(r.TutorialSteps) > 0 {
		b, err := json.Marshal(r.TutorialSteps)
		if err != nil {
			return err
		}
		tutorialSteps = string(b)
	}
	_, err := s.DB.Exec(`UPDATE repos SET description=?,readme_intro=?,tutorial_steps=?,language=?,stars=?,forks=?,
		open_issues=?,open_prs=?,html_url=?,homepage=?,pushed_at=?,repo_updated_at=?,
		last_synced_at=?,sync_status=?,sync_error=? WHERE full_name=?`,
		r.Description, r.ReadmeIntro, tutorialSteps, r.Language, r.Stars, r.Forks,
		nullInt(r.OpenIssues), nullInt(r.OpenPRs), r.HTMLURL, r.Homepage,
		nullStr(r.PushedAt), nullStr(r.RepoUpdatedAt), Now(), status, syncErr, fullName)
	return err
}

// SetRepoSyncStatus records a sync outcome without touching any synced
// data — used for rate_limited / failed so the old row is preserved.
func (s *Store) SetRepoSyncStatus(fullName, status, syncErr string) error {
	_, err := s.DB.Exec("UPDATE repos SET last_synced_at=?,sync_status=?,sync_error=? WHERE full_name=?",
		Now(), status, syncErr, fullName)
	return err
}

func (s *Store) UpsertRepoCommits(fullName string, commits []model.RepoCommit) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range commits {
		if _, err := tx.Exec(`INSERT INTO repo_commits(full_name,sha,message,author_name,author_login,committed_at,html_url)
			VALUES(?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE message=VALUES(message),author_name=VALUES(author_name),
			author_login=VALUES(author_login),committed_at=VALUES(committed_at),html_url=VALUES(html_url)`,
			fullName, c.SHA, c.Message, c.AuthorName, c.AuthorLogin, c.CommittedAt, c.HTMLURL); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertRepoReleases(fullName string, releases []model.RepoRelease) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, rel := range releases {
		if _, err := tx.Exec(`INSERT INTO repo_releases(full_name,tag_name,name,body,published_at,html_url,draft,prerelease)
			VALUES(?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE name=VALUES(name),body=VALUES(body),published_at=VALUES(published_at),
			html_url=VALUES(html_url),draft=VALUES(draft),prerelease=VALUES(prerelease)`,
			fullName, rel.TagName, rel.Name, rel.Body, nullStr(rel.PublishedAt), rel.HTMLURL, rel.Draft, rel.Prerelease); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListRepoCommits(fullName string, limit int) ([]model.RepoCommit, error) {
	rows, err := s.DB.Query(`SELECT sha,message,author_name,author_login,committed_at,html_url
		FROM repo_commits WHERE full_name=? ORDER BY committed_at DESC LIMIT ?`, fullName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RepoCommit{}
	for rows.Next() {
		var c model.RepoCommit
		if err := rows.Scan(&c.SHA, &c.Message, &c.AuthorName, &c.AuthorLogin, &c.CommittedAt, &c.HTMLURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListRepoReleases(fullName string, limit int) ([]model.RepoRelease, error) {
	rows, err := s.DB.Query(`SELECT tag_name,name,body,published_at,html_url,draft,prerelease
		FROM repo_releases WHERE full_name=? ORDER BY published_at DESC LIMIT ?`, fullName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RepoRelease{}
	for rows.Next() {
		var rel model.RepoRelease
		var body, pub sql.NullString
		if err := rows.Scan(&rel.TagName, &rel.Name, &body, &pub, &rel.HTMLURL, &rel.Draft, &rel.Prerelease); err != nil {
			return nil, err
		}
		rel.Body = body.String
		if pub.Valid {
			v := pub.String
			rel.PublishedAt = &v
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// ---- repo groups ----

func (s *Store) ListRepoGroups() ([]model.Group, error) {
	rows, err := s.DB.Query("SELECT id,name,fixed FROM repo_groups ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Group{}
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Fixed); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateRepoGroup(name string) (model.Group, error) {
	g := model.Group{ID: NewID("grp_"), Name: name}
	_, err := s.DB.Exec("INSERT INTO repo_groups(id,name,fixed) VALUES(?,?,0)", g.ID, g.Name)
	return g, err
}

func (s *Store) DeleteRepoGroup(id string) (code string, err error) {
	var fixed bool
	err = s.DB.QueryRow("SELECT fixed FROM repo_groups WHERE id=?", id).Scan(&fixed)
	if err != nil {
		return "NOT_FOUND", err
	}
	if fixed {
		return "GROUP_FIXED", fmt.Errorf("fixed group cannot be deleted")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT IGNORE INTO repo_groups(id,name,fixed) VALUES('other','默认',1)"); err != nil {
		return "", err
	}
	if _, err := tx.Exec("UPDATE repos SET group_id='other' WHERE group_id=?", id); err != nil {
		return "", err
	}
	if _, err := tx.Exec("DELETE FROM repo_groups WHERE id=?", id); err != nil {
		return "", err
	}
	return "", tx.Commit()
}

func (s *Store) RepoGroupExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM repo_groups WHERE id=?", id).Scan(&n)
	return n > 0
}

// ---- YouTube channels / videos (prototype 2026-10-26) ----

// YTVideoFilter filters ListYTVideos. DateFrom/DateTo are "2006-01-02"
// calendar dates (Shanghai day bounds, like the post date filter).
// Kind is video|short|live|playlist; "" means all kinds.
// Limit <= 0 means the default 100.
type YTVideoFilter struct {
	ChannelID string
	Q         string
	DateFrom  string
	DateTo    string
	Kind      string
	Limit     int
}

func scanYTChannel(c *model.YTChannel, groupID, lastSyncedAt, dateFrom, dateTo sql.NullString, videoCount int) {
	if groupID.Valid {
		c.GroupID = groupID.String
	}
	if lastSyncedAt.Valid {
		v := lastSyncedAt.String
		c.LastSyncedAt = &v
	}
	if dateFrom.Valid {
		v := dateFrom.String
		c.DateFrom = &v
	}
	if dateTo.Valid {
		v := dateTo.String
		c.DateTo = &v
	}
	c.VideoCount = videoCount
}

// ListYTChannels returns all followed channels, oldest first, each with
// its stored video count.
func (s *Store) ListYTChannels() ([]model.YTChannel, error) {
	rows, err := s.DB.Query(`SELECT c.id,c.channel_id,c.handle,c.name,c.group_id,
		DATE_FORMAT(c.date_from,'%Y-%m-%d'),DATE_FORMAT(c.date_to,'%Y-%m-%d'),
		c.created_at,c.last_synced_at,c.sync_status,c.sync_error,
		(SELECT COUNT(*) FROM yt_videos v WHERE v.channel_id=c.channel_id) AS video_count,c.sync_mode
		FROM yt_channels c ORDER BY c.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.YTChannel{}
	for rows.Next() {
		var c model.YTChannel
		var groupID, lastSyncedAt, dateFrom, dateTo sql.NullString
		var videoCount int
		if err := rows.Scan(&c.ID, &c.ChannelID, &c.Handle, &c.Name, &groupID,
			&dateFrom, &dateTo,
			&c.CreatedAt, &lastSyncedAt, &c.SyncStatus, &c.SyncError, &videoCount, &c.SyncMode); err != nil {
			return nil, err
		}
		scanYTChannel(&c, groupID, lastSyncedAt, dateFrom, dateTo, videoCount)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) scanYTChannelRow(row *sql.Row) (model.YTChannel, error) {
	var c model.YTChannel
	var groupID, lastSyncedAt, dateFrom, dateTo sql.NullString
	var videoCount int
	err := row.Scan(&c.ID, &c.ChannelID, &c.Handle, &c.Name, &groupID,
		&dateFrom, &dateTo,
		&c.CreatedAt, &lastSyncedAt, &c.SyncStatus, &c.SyncError, &videoCount, &c.SyncMode)
	if err != nil {
		return c, err
	}
	scanYTChannel(&c, groupID, lastSyncedAt, dateFrom, dateTo, videoCount)
	return c, nil
}

const ytChannelCols = `SELECT c.id,c.channel_id,c.handle,c.name,c.group_id,
	DATE_FORMAT(c.date_from,'%Y-%m-%d'),DATE_FORMAT(c.date_to,'%Y-%m-%d'),
	c.created_at,c.last_synced_at,c.sync_status,c.sync_error,
	(SELECT COUNT(*) FROM yt_videos v WHERE v.channel_id=c.channel_id) AS video_count,c.sync_mode
	FROM yt_channels c `

// GetYTChannel loads one channel by its row id.
func (s *Store) GetYTChannel(id string) (model.YTChannel, error) {
	return s.scanYTChannelRow(s.DB.QueryRow(ytChannelCols+"WHERE c.id=?", id))
}

// GetYTChannelByChannelID loads one channel by its UC... ID.
func (s *Store) GetYTChannelByChannelID(channelID string) (model.YTChannel, error) {
	return s.scanYTChannelRow(s.DB.QueryRow(ytChannelCols+"WHERE c.channel_id=?", channelID))
}

// YTChannelExists reports whether a channel row id exists.
func (s *Store) YTChannelExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM yt_channels WHERE id=?", id).Scan(&n)
	return n > 0
}

// CreateYTChannel inserts a new channel row (status "never" until the
// first sync runs). Callers check for duplicates first.
func (s *Store) CreateYTChannel(c model.YTChannel) error {
	mode := c.SyncMode
	if mode == "" {
		mode = "full"
	}
	_, err := s.DB.Exec(`INSERT INTO yt_channels(id,channel_id,handle,name,group_id,
		date_from,date_to,
		created_at,last_synced_at,sync_status,sync_error,sync_mode) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.ChannelID, c.Handle, c.Name, groupIDArg(c.GroupID),
		dateArg(c.DateFrom), dateArg(c.DateTo),
		c.CreatedAt, nil, c.SyncStatus, c.SyncError, mode)
	return err
}

// dateArg maps a nil *string to NULL so an unset date means "no limit".
func dateArg(d *string) any {
	if d == nil {
		return nil
	}
	return *d
}

// UpdateYTChannel updates a channel's group and pull range. A nil
// dateFrom/dateTo clears that bound (NULL = no limit); groupID "" clears
// the group. Callers validate the dates; nothing is validated here.
func (s *Store) UpdateYTChannel(id string, groupID string, dateFrom, dateTo *string) error {
	_, err := s.DB.Exec(`UPDATE yt_channels SET group_id=?,date_from=?,date_to=? WHERE id=?`,
		groupIDArg(groupID), dateArg(dateFrom), dateArg(dateTo), id)
	return err
}

// DeleteYTChannel deletes the channel AND its videos (videos are keyed
// by channel_id value, no FK, so both deletes run in one transaction).
func (s *Store) DeleteYTChannel(id string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var channelID string
	if err := tx.QueryRow("SELECT channel_id FROM yt_channels WHERE id=?", id).Scan(&channelID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE s FROM yt_video_summaries s JOIN yt_videos v ON v.id=s.video_id
		WHERE v.channel_id=?`, channelID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE c FROM yt_video_comments c JOIN yt_videos v ON v.id=c.video_id
		WHERE v.channel_id=?`, channelID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM yt_videos WHERE channel_id=?", channelID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM yt_channels WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetYTChannelSyncStatus records a sync outcome (and its time) without
// touching the channel name or any videos.
func (s *Store) SetYTChannelSyncStatus(id, status, syncErr string) error {
	_, err := s.DB.Exec("UPDATE yt_channels SET last_synced_at=?,sync_status=?,sync_error=? WHERE id=?",
		Now(), status, syncErr, id)
	return err
}

// UpdateYTChannelMeta applies one sync's channel name + outcome.
func (s *Store) UpdateYTChannelMeta(id, name, status, syncErr string) error {
	_, err := s.DB.Exec("UPDATE yt_channels SET name=?,last_synced_at=?,sync_status=?,sync_error=? WHERE id=?",
		name, Now(), status, syncErr, id)
	return err
}

// UpsertYTVideos inserts new videos and updates re-fetched ones. The
// PRIMARY KEY on yt_videos.id (the video_id / playlist ID) makes a
// re-sync of the same item an update, never a duplicate row.
//
// A re-fetch must never clobber what is already known with an empty
// value: description and published_at are only overwritten when the
// incoming value is non-empty / non-NULL, and the same guard applies to
// duration and live_status (0 / "" mean "unknown" on the way in). Title,
// thumbnail, URL and kind are always overwritten — they always come
// from the flat listing. An empty PublishedAt becomes NULL (never a
// zero date); an empty Kind defaults to "video".
func (s *Store) UpsertYTVideos(channelID string, videos []model.YTVideo) error {
	if len(videos) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO yt_videos(id,channel_id,kind,title,description,
		published_at,duration,live_status,thumbnail_url,url,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE kind=VALUES(kind),title=VALUES(title),
		description=IF(VALUES(description)<>'',VALUES(description),description),
		published_at=IF(VALUES(published_at) IS NOT NULL,VALUES(published_at),published_at),
		duration=IF(VALUES(duration) IS NOT NULL,VALUES(duration),duration),
		live_status=IF(VALUES(live_status)<>'',VALUES(live_status),live_status),
		thumbnail_url=VALUES(thumbnail_url),url=VALUES(url)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, v := range videos {
		kind := v.Kind
		if kind == "" {
			kind = "video"
		}
		var publishedAt, duration any
		if v.PublishedAt != "" {
			publishedAt = v.PublishedAt
		}
		if v.Duration > 0 {
			duration = v.Duration
		}
		if _, err := stmt.Exec(v.ID, channelID, kind, v.Title, v.Description,
			publishedAt, duration, v.LiveStatus, v.ThumbnailURL, v.URL, v.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PruneYTVideosOutsideRange deletes a channel's video/short/live rows
// whose published_at falls outside [from, to] (date_to is inclusive of
// the whole day). A bound that is "" is not applied; with neither bound
// set this is a no-op. Playlists are never pruned — they have no date
// and the range does not apply to them. Rows with a NULL published_at
// are kept: they are candidates for the next sync's backfill, not stale
// data.
func (s *Store) PruneYTVideosOutsideRange(channelID, from, to string) error {
	if from == "" && to == "" {
		return nil
	}
	// Delete the complement of the range: before from, or on/after the
	// day after to. Only one side is tested when only one bound is set.
	var outside []string
	var args []any
	args = append(args, channelID)
	if from != "" {
		outside = append(outside, "published_at<?")
		args = append(args, from)
	}
	if to != "" {
		outside = append(outside, "published_at>=DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, to)
	}
	q := `DELETE FROM yt_videos WHERE channel_id=? AND kind IN ('video','short','live')
		AND added_manually=0 AND published_at IS NOT NULL AND (` + strings.Join(outside, " OR ") + ")"
	_, err := s.DB.Exec(q, args...)
	return err
}

// ListYTVideos returns videos newest first, with the channel name joined
// for display. Keyword matches title/description (LIKE); the filter
// always runs on the original text.
func (s *Store) ListYTVideos(f YTVideoFilter) ([]model.YTVideo, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var conds []string
	var args []any
	if f.ChannelID != "" {
		conds = append(conds, "v.channel_id=?")
		args = append(args, f.ChannelID)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		conds = append(conds, "(v.title LIKE ? OR v.description LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if f.Kind != "" {
		conds = append(conds, "v.kind=?")
		args = append(args, f.Kind)
	}
	if f.DateFrom != "" {
		conds = append(conds, "v.published_at>=?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		conds = append(conds, "v.published_at<DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.DateTo)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	q := `SELECT v.id,v.channel_id,COALESCE(c.name,''),v.kind,v.title,v.description,
		DATE_FORMAT(v.published_at,'%Y-%m-%d %H:%i:%s'),v.duration,v.live_status,v.thumbnail_url,v.url,v.created_at
		FROM yt_videos v LEFT JOIN yt_channels c ON c.channel_id=v.channel_id ` +
		where + ` ORDER BY v.published_at DESC, v.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.YTVideo{}
	for rows.Next() {
		var v model.YTVideo
		var publishedAt sql.NullString
		var duration sql.NullInt64
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.ChannelName, &v.Kind, &v.Title,
			&v.Description, &publishedAt, &duration, &v.LiveStatus, &v.ThumbnailURL, &v.URL, &v.CreatedAt); err != nil {
			return nil, err
		}
		// NULL published_at (playlists, or a video whose backfill has
		// not supplied a date yet) reads back as "". DATE_FORMAT keeps
		// the "2006-01-02 15:04:05" wall-time format whether or not
		// the DSN sets parseTime=true (a plain column scan would
		// yield RFC3339 under parseTime).
		if publishedAt.Valid {
			v.PublishedAt = publishedAt.String
		}
		if duration.Valid {
			v.Duration = int(duration.Int64)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- yt groups (same pattern as repo_groups) ----

func (s *Store) ListYTGroups() ([]model.Group, error) {
	rows, err := s.DB.Query("SELECT id,name,fixed FROM yt_groups ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Group{}
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Fixed); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateYTGroup(name string) (model.Group, error) {
	g := model.Group{ID: NewID("grp_"), Name: name}
	_, err := s.DB.Exec("INSERT INTO yt_groups(id,name,fixed) VALUES(?,?,0)", g.ID, g.Name)
	return g, err
}

// DeleteYTGroup moves the group's channels to the default group first,
// then deletes the group (mirrors DeleteRepoGroup).
func (s *Store) DeleteYTGroup(id string) (code string, err error) {
	var fixed bool
	err = s.DB.QueryRow("SELECT fixed FROM yt_groups WHERE id=?", id).Scan(&fixed)
	if err != nil {
		return "NOT_FOUND", err
	}
	if fixed {
		return "GROUP_FIXED", fmt.Errorf("fixed group cannot be deleted")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT IGNORE INTO yt_groups(id,name,fixed) VALUES('other','默认',1)"); err != nil {
		return "", err
	}
	if _, err := tx.Exec("UPDATE yt_channels SET group_id='other' WHERE group_id=?", id); err != nil {
		return "", err
	}
	if _, err := tx.Exec("DELETE FROM yt_groups WHERE id=?", id); err != nil {
		return "", err
	}
	return "", tx.Commit()
}

func (s *Store) YTGroupExists(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM yt_groups WHERE id=?", id).Scan(&n)
	return n > 0
}

// ---- translation cache ----
// Store implements translate.Cache. The key is translate.Hash(source,
// target); only genuine translations are ever stored (short-circuited
// originals and failed items are never cached).

func (s *Store) GetTranslations(hashes []string, target string) (map[string]string, error) {
	out := map[string]string{}
	if len(hashes) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(hashes))
	q := "SELECT source_hash, translated_text FROM translations WHERE target_lang=? AND source_hash IN (" + strings.TrimSuffix(ph, ",") + ")"
	args := make([]any, 0, len(hashes)+1)
	args = append(args, target)
	for _, h := range hashes {
		args = append(args, h)
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h, tr string
		if err := rows.Scan(&h, &tr); err != nil {
			return nil, err
		}
		out[h] = tr
	}
	return out, rows.Err()
}

func (s *Store) PutTranslations(target string, items map[string]string) error {
	if len(items) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("INSERT INTO translations (source_hash, target_lang, source_text, translated_text) VALUES ")
	args := make([]any, 0, len(items)*4)
	first := true
	for src, tr := range items {
		if !first {
			sb.WriteString(",")
		}
		first = false
		sb.WriteString("(?,?,?,?)")
		args = append(args, translate.Hash(src, target), target, src, tr)
	}
	sb.WriteString(" ON DUPLICATE KEY UPDATE source_text=VALUES(source_text), translated_text=VALUES(translated_text)")
	_, err := s.DB.Exec(sb.String(), args...)
	return err
}

// GetYTVideo returns one video by its YouTube ID (sql.ErrNoRows when
// unknown).
func (s *Store) GetYTVideo(id string) (model.YTVideo, error) {
	var v model.YTVideo
	var desc, published sql.NullString
	var duration sql.NullInt64
	err := s.DB.QueryRow(`SELECT id,channel_id,kind,title,description,
		DATE_FORMAT(published_at,'%Y-%m-%d %H:%i:%s'),duration,
		live_status,thumbnail_url,url FROM yt_videos WHERE id=?`, id).Scan(&v.ID, &v.ChannelID, &v.Kind,
		&v.Title, &desc, &published, &duration, &v.LiveStatus, &v.ThumbnailURL, &v.URL)
	if err != nil {
		return v, err
	}
	v.Description = desc.String
	v.PublishedAt = published.String
	v.Duration = int(duration.Int64)
	return v, nil
}

// GetYTSummary returns the summary row of (video, lang); ok=false when
// there is none.
func (s *Store) GetYTSummary(videoID, lang string) (sum model.YTSummary, ok bool, err error) {
	var summary, keyPoints, chapters, errText sql.NullString
	var resources string
	err = s.DB.QueryRow(`SELECT video_id,lang,status,summary,key_points,chapters,COALESCE(resources,''),source,transcript_lang,
		model,error,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s') FROM yt_video_summaries WHERE video_id=? AND lang=?`, videoID, lang).
		Scan(&sum.VideoID, &sum.Lang, &sum.Status, &summary, &keyPoints, &chapters, &resources, &sum.Source,
			&sum.TranscriptLang, &sum.Model, &errText, &sum.CreatedAt, &sum.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sum, false, nil
	}
	if err != nil {
		return sum, false, err
	}
	sum.Summary, sum.Error = summary.String, errText.String
	sum.KeyPoints, sum.Chapters = []string{}, []model.YTSummaryChapter{}
	if keyPoints.String != "" {
		_ = json.Unmarshal([]byte(keyPoints.String), &sum.KeyPoints)
	}
	if chapters.String != "" {
		_ = json.Unmarshal([]byte(chapters.String), &sum.Chapters)
	}
	sum.Resources = []model.YTSummaryResource{}
	if resources != "" {
		_ = json.Unmarshal([]byte(resources), &sum.Resources)
	}
	return sum, true, nil
}

// StartYTSummary marks (video, lang) pending, clearing any earlier
// result or error (a regenerate starts from scratch).
func (s *Store) StartYTSummary(videoID, lang string) error {
	now := Now()
	_, err := s.DB.Exec(`INSERT INTO yt_video_summaries(video_id,lang,status,summary,key_points,chapters,
		source,transcript_lang,model,error,created_at,updated_at) VALUES(?,?,'pending','','[]','[]','','','','',?,?)
		ON DUPLICATE KEY UPDATE status='pending',error='',updated_at=VALUES(updated_at)`, videoID, lang, now, now)
	return err
}

// FinishYTSummary stores a finished summary.
func (s *Store) FinishYTSummary(sum model.YTSummary) error {
	kp, err := json.Marshal(sum.KeyPoints)
	if err != nil {
		return err
	}
	ch, err := json.Marshal(sum.Chapters)
	if err != nil {
		return err
	}
	resources := sum.Resources
	if resources == nil {
		resources = []model.YTSummaryResource{}
	}
	rs, err := json.Marshal(resources)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE yt_video_summaries SET status='done',summary=?,key_points=?,chapters=?,resources=?,
		source=?,transcript_lang=?,model=?,error='',updated_at=? WHERE video_id=? AND lang=?`,
		sum.Summary, string(kp), string(ch), string(rs), sum.Source, sum.TranscriptLang, sum.Model, Now(), sum.VideoID, sum.Lang)
	return err
}

// FailYTSummary records a failed run. An earlier finished summary (a
// failed regenerate) is kept; only the status and error change.
func (s *Store) FailYTSummary(videoID, lang, msg string) error {
	_, err := s.DB.Exec(`UPDATE yt_video_summaries SET status='failed',error=?,updated_at=?
		WHERE video_id=? AND lang=?`, msg, Now(), videoID, lang)
	return err
}

// YTCommentsState is the stored comments of one video plus when they were
// fetched ("" = never).
type YTCommentsState struct {
	Comments  []model.YTComment
	FetchedAt string
}

// GetYTComments returns the stored comments and fetch time of each known
// video in ids (unknown IDs are absent from the map).
func (s *Store) GetYTComments(ids []string) (map[string]*YTCommentsState, error) {
	out := map[string]*YTCommentsState{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(`SELECT id, DATE_FORMAT(comments_fetched_at,'%Y-%m-%d %H:%i:%s')
		FROM yt_videos WHERE id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var fetched sql.NullString
		if err := rows.Scan(&id, &fetched); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = &YTCommentsState{Comments: []model.YTComment{}, FetchedAt: fetched.String}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = s.DB.Query(`SELECT video_id,author,text,like_count,
		DATE_FORMAT(published_at,'%Y-%m-%d %H:%i:%s') FROM yt_video_comments
		WHERE video_id IN (`+ph+`) ORDER BY video_id, idx`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var c model.YTComment
		var published sql.NullString
		if err := rows.Scan(&id, &c.Author, &c.Text, &c.Likes, &published); err != nil {
			return nil, err
		}
		c.PublishedAt = published.String
		if st := out[id]; st != nil {
			st.Comments = append(st.Comments, c)
		}
	}
	return out, rows.Err()
}

// ReplaceYTComments stores a video's comments (replacing earlier ones)
// and stamps comments_fetched_at, in one transaction.
func (s *Store) ReplaceYTComments(videoID string, comments []model.YTComment) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM yt_video_comments WHERE video_id=?", videoID); err != nil {
		return err
	}
	for i, c := range comments {
		var published any
		if c.PublishedAt != "" {
			published = c.PublishedAt
		}
		if _, err := tx.Exec(`INSERT INTO yt_video_comments(video_id,idx,author,text,like_count,published_at)
			VALUES(?,?,?,?,?,?)`, videoID, i, c.Author, c.Text, c.Likes, published); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("UPDATE yt_videos SET comments_fetched_at=? WHERE id=?", Now(), videoID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetYTChannelSyncMode switches a channel between full and single mode.
func (s *Store) SetYTChannelSyncMode(id, mode string) error {
	_, err := s.DB.Exec("UPDATE yt_channels SET sync_mode=? WHERE id=?", mode, id)
	return err
}

// MarkYTVideoManual flags a video as added by URL (kept by date pruning).
func (s *Store) MarkYTVideoManual(id string) error {
	_, err := s.DB.Exec("UPDATE yt_videos SET added_manually=1 WHERE id=?", id)
	return err
}

// ManualYTVideoURLs returns the URLs of a channel's videos added by URL.
func (s *Store) ManualYTVideoURLs(channelID string) ([]string, error) {
	rows, err := s.DB.Query("SELECT url FROM yt_videos WHERE channel_id=? AND added_manually=1", channelID)
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
