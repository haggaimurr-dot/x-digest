// Package store implements all MariaDB access for the x-digest API.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"xdigest/internal/model"
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
				OR p.author_name LIKE ? OR p.author LIKE ?
				OR EXISTS (SELECT 1 FROM post_tags t WHERE t.post_id=p.id AND t.tag LIKE ?))`)
			for i := 0; i < 7; i++ {
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
	rows, err := s.DB.Query(`SELECT p.id,p.url,p.author,p.author_name,p.author_avatar,p.industry,p.tech,
		p.title,p.summary,p.note,p.published_at,p.is_long,p.show_flowchart
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
		if err := rows.Scan(&p.ID, &p.URL, &p.Author, &p.AuthorName, &p.AuthorAvatar, &p.Industry, &p.Tech,
			&p.Title, &p.Summary, &note, &p.PublishedAt, &p.IsLong, &show); err != nil {
			return nil, 0, "", err
		}
		p.Note = note.String
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
	err := s.DB.QueryRow(`SELECT id,url,author,author_name,author_avatar,industry,tech,title,summary,note,
		published_at,is_long,show_flowchart FROM posts WHERE id=?`, id).Scan(
		&p.ID, &p.URL, &p.Author, &p.AuthorName, &p.AuthorAvatar, &p.Industry, &p.Tech,
		&p.Title, &p.Summary, &note, &p.PublishedAt, &p.IsLong, &show)
	if err != nil {
		return nil, err
	}
	p.Note = note.String
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
	arows, err := s.DB.Query("SELECT username,name,avatar,group_id,bio FROM authors WHERE group_id IS NOT NULL ORDER BY username")
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
		a.Group = gid.String
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
	if _, err := s.DB.Exec("UPDATE authors SET group_id='other' WHERE group_id=?", id); err != nil {
		return "", err
	}
	_, err = s.DB.Exec("DELETE FROM watchlist_groups WHERE id=?", id)
	return "", err
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
	rows, err := s.DB.Query(`SELECT idx,text,tools,tech,prep,substeps_json,status,note FROM replica_steps
		WHERE replica_id=? ORDER BY idx`, replicaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ReplicaStep{}
	for rows.Next() {
		var st model.ReplicaStep
		var sub string
		if err := rows.Scan(&st.Idx, &st.Text, &st.Tools, &st.Tech, &st.Prep, &sub, &st.Status, &st.Note); err != nil {
			return nil, err
		}
		if sub != "" {
			_ = json.Unmarshal([]byte(sub), &st.Substeps)
		}
		atts, err := s.replicaStepAttachments(replicaID, st.Idx)
		if err != nil {
			return nil, err
		}
		st.Attachments = atts
		out = append(out, st)
	}
	return out, rows.Err()
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
