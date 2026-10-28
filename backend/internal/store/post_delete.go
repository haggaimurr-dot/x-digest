package store

import "strings"

// DeletePosts removes posts (steps, tags and category links cascade; AI
// summaries have no foreign key and are deleted here) and records each id in
// deleted_posts, so the bot import and「更新帖子」do not bring them back.
// An explicit single import (UpsertXPost) clears the record again. Ids that
// do not exist are skipped; the result counts the posts actually removed.
func (s *Store) DeletePosts(ids []string) (int, error) {
	ids = clean(ids, "")
	if len(ids) == 0 {
		return 0, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Only ids that are really posts get a tombstone.
	if _, err := tx.Exec("INSERT IGNORE INTO deleted_posts(id, deleted_at) SELECT id, NOW() FROM posts WHERE id IN ("+ph+")", args...); err != nil {
		return 0, err
	}
	if _, err := tx.Exec("DELETE FROM post_summaries WHERE post_id IN ("+ph+")", args...); err != nil {
		return 0, err
	}
	res, err := tx.Exec("DELETE FROM posts WHERE id IN ("+ph+")", args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), tx.Commit()
}

// PostDeleted reports whether the post was deleted on the page.
func (s *Store) PostDeleted(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM deleted_posts WHERE id=?", id).Scan(&n)
	return n > 0
}
