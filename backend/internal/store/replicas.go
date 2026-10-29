package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"xdigest/internal/model"
)

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
	rows, err := s.DB.Query(`SELECT idx,COALESCE(text,''),tools,tech,COALESCE(prep,''),COALESCE(substeps_json,''),status,COALESCE(note,''),is_custom,parent_idx
		FROM replica_steps WHERE replica_id=? ORDER BY idx`, replicaID)
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

// CloneReplica copies a replica's step structure into a new standalone flow
// (post_id ” so several copies of one post can exist): every step reset to
// idle, attachments not copied, notes copied only when copyNotes. The new
// title is the old one + "（副本）". sql.ErrNoRows for an unknown id.
func (s *Store) CloneReplica(id string, copyNotes bool) (*model.Replica, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var url, author, authorName string
	var title sql.NullString
	err = tx.QueryRow("SELECT post_url,post_author,post_author_name,post_title FROM replicas WHERE id=?", id).Scan(&url, &author, &authorName, &title)
	if err != nil {
		return nil, err
	}
	newID := NewID("rep_")
	if _, err := tx.Exec(`INSERT INTO replicas(id,post_id,post_url,post_author,post_author_name,post_title,created_at)
		VALUES(?, '', ?, ?, ?, ?, ?)`, newID, url, author, authorName, title.String+"（副本）", Now()); err != nil {
		return nil, err
	}
	note := "''"
	if copyNotes {
		note = "note"
	}
	if _, err := tx.Exec(`INSERT INTO replica_steps(replica_id,idx,text,tools,tech,prep,substeps_json,status,note,is_custom,parent_idx)
		SELECT ?, idx, text, tools, tech, prep, substeps_json, 'idle', `+note+`, is_custom, parent_idx
		FROM replica_steps WHERE replica_id=? ORDER BY idx`, newID, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetReplica(newID, "")
}

// AttachmentBytes is the total size of all stored attachments.
func (s *Store) AttachmentBytes() (int64, error) {
	var n int64
	err := s.DB.QueryRow("SELECT COALESCE(SUM(size),0) FROM attachments").Scan(&n)
	return n, err
}

// AttachmentFileNames: base names of every attachment file the DB knows.
func (s *Store) AttachmentFileNames() (map[string]bool, error) {
	rows, err := s.DB.Query("SELECT path FROM attachments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[filepath.Base(p)] = true
	}
	return out, rows.Err()
}
