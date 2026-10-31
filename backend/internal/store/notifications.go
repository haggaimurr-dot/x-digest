package store

import (
	"database/sql"
	"fmt"
	"strings"

	"xdigest/internal/model"
)

// Notifications (2026-10-28): the bell in the top bar. Written by the
// repo sync when a tracked repository publishes a new release.

type Notification struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	CreatedAt string  `json:"created_at"`
	ReadAt    *string `json:"read_at"`
}

func (s *Store) AddNotification(kind, title, url string) error {
	_, err := s.DB.Exec("INSERT INTO notifications(id,kind,title,url,created_at) VALUES(?,?,?,?,?)",
		NewID("ntf_"), kind, title, url, Now())
	return err
}

// ListNotifications returns the newest limit notifications and the unread
// count.
func (s *Store) ListNotifications(limit int) ([]Notification, int, error) {
	var unread int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM notifications WHERE read_at IS NULL").Scan(&unread); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query("SELECT id,kind,title,url,created_at,read_at FROM notifications ORDER BY created_at DESC, id DESC LIMIT ?", limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var read sql.NullString
		if err := rows.Scan(&n.ID, &n.Kind, &n.Title, &n.URL, &n.CreatedAt, &read); err != nil {
			return nil, 0, err
		}
		if read.Valid {
			n.ReadAt = &read.String
		}
		out = append(out, n)
	}
	return out, unread, rows.Err()
}

// MarkNotificationsRead marks the given ids (all unread ones when ids is
// empty) as read.
func (s *Store) MarkNotificationsRead(ids []string) error {
	if len(ids) == 0 {
		_, err := s.DB.Exec("UPDATE notifications SET read_at=? WHERE read_at IS NULL", Now())
		return err
	}
	args := []any{Now()}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.DB.Exec("UPDATE notifications SET read_at=? WHERE read_at IS NULL AND id IN ("+
		strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+")", args...)
	return err
}

// ReleaseNotice decides whether a sync that moved a repo's latest release
// from prev to cur deserves a notification: only a different, non-empty tag
// after a known one (the first sync of a repo never notifies).
func ReleaseNotice(fullName string, prev, cur *model.RepoRelease) (title, url string, ok bool) {
	if prev == nil || cur == nil || cur.TagName == "" || cur.TagName == prev.TagName {
		return "", "", false
	}
	return fmt.Sprintf("%s 发布了新版本 %s", fullName, cur.TagName), cur.HTMLURL, true
}

// CreateReplicaFromRepo builds a replica from a repository README's
// install / quick-start steps (source "repo"). Like a post replica, one repo
// gets one replica: an existing one is returned with existing=true.
func (s *Store) CreateReplicaFromRepo(fullName string) (*model.Replica, bool, error) {
	r, err := s.GetRepo(fullName)
	if err != nil {
		return nil, false, err
	}
	if len(r.TutorialSteps) == 0 {
		return nil, false, fmt.Errorf("no tutorial steps")
	}
	key := "repo:" + r.FullName
	var existingID string
	switch err := s.DB.QueryRow("SELECT id FROM replicas WHERE post_id=? LIMIT 1", key).Scan(&existingID); err {
	case nil:
		rep, err := s.GetReplica(existingID, "")
		if err != nil {
			return nil, false, err
		}
		rep.Existing = true
		return rep, true, nil
	case sql.ErrNoRows:
	default:
		return nil, false, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	id := NewID("rep_")
	// post_id carries "repo:owner/name" so the one-replica-per-source rule
	// and the flow card's link work without a separate key column.
	if _, err := tx.Exec(`INSERT INTO replicas(id,post_id,post_url,post_author,post_author_name,post_title,created_at,source)
		VALUES(?,?,?,?,?,?,?, 'repo')`, id, key, r.HTMLURL, r.Owner, r.Owner, r.FullName+" 上手步骤", Now()); err != nil {
		return nil, false, err
	}
	for i, st := range r.TutorialSteps {
		if _, err := tx.Exec("INSERT INTO replica_steps(replica_id,idx,text,status) VALUES(?,?,?, 'idle')", id, i, st.Text); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	rep, err := s.GetReplica(id, "")
	return rep, false, err
}
