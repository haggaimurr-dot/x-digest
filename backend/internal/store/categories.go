package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"xdigest/internal/model"
)

// Categories (2026-10-28): rename, soft delete with undo, purge.
// A deleted category keeps its row and its post_categories rows with
// deleted_at set, so RestoreCategory brings both back; PurgeDeletedCategories
// removes the ones deleted more than the given days ago (FK cascade drops
// their assignments).

// ErrNameTaken: another live category already has this name.
var ErrNameTaken = errors.New("category name already exists")

func (s *Store) ListCategories() (cats []model.Category, assignments map[string][]string, err error) {
	rows, err := s.DB.Query("SELECT id,name FROM categories WHERE deleted_at IS NULL ORDER BY name")
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
	arows, err := s.DB.Query(`SELECT pc.post_id,pc.category_id FROM post_categories pc
		JOIN categories c ON c.id=pc.category_id WHERE c.deleted_at IS NULL ORDER BY pc.post_id,pc.category_id`)
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
	c := model.Category{ID: NewID("cat_"), Name: strings.TrimSpace(name)}
	if taken, err := s.categoryNameTaken(c.Name, ""); err != nil {
		return c, err
	} else if taken {
		return c, ErrNameTaken
	}
	_, err := s.DB.Exec("INSERT INTO categories(id,name) VALUES(?,?)", c.ID, c.Name)
	return c, err
}

// RenameCategory: sql.ErrNoRows for an unknown/deleted id, ErrNameTaken
// when another live category has the name (case-insensitive, ci collation).
func (s *Store) RenameCategory(id, name string) (model.Category, error) {
	name = strings.TrimSpace(name)
	c := model.Category{ID: id, Name: name}
	if taken, err := s.categoryNameTaken(name, id); err != nil {
		return c, err
	} else if taken {
		return c, ErrNameTaken
	}
	res, err := s.DB.Exec("UPDATE categories SET name=? WHERE id=? AND deleted_at IS NULL", name, id)
	if err != nil {
		return c, err
	}
	if n, _ := res.RowsAffected(); n == 0 && !s.liveCategory(id) {
		return c, sql.ErrNoRows
	}
	return c, nil
}

func (s *Store) categoryNameTaken(name, exceptID string) (bool, error) {
	var n int
	err := s.DB.QueryRow("SELECT COUNT(*) FROM categories WHERE name=? AND id<>? AND deleted_at IS NULL", name, exceptID).Scan(&n)
	return n > 0, err
}

func (s *Store) liveCategory(id string) bool {
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM categories WHERE id=? AND deleted_at IS NULL", id).Scan(&n)
	return n > 0
}

// DeleteCategory soft-deletes; the page offers undo (RestoreCategory).
func (s *Store) DeleteCategory(id string) error {
	_, err := s.DB.Exec("UPDATE categories SET deleted_at=NOW() WHERE id=? AND deleted_at IS NULL", id)
	return err
}

// RestoreCategory undoes DeleteCategory; sql.ErrNoRows when there is no
// deleted category with this id (purged, or never deleted). A live category
// that took the name meanwhile → ErrNameTaken.
func (s *Store) RestoreCategory(id string) (model.Category, error) {
	var c model.Category
	err := s.DB.QueryRow("SELECT id,name FROM categories WHERE id=? AND deleted_at IS NOT NULL", id).Scan(&c.ID, &c.Name)
	if err != nil {
		return c, err
	}
	if taken, err := s.categoryNameTaken(c.Name, id); err != nil {
		return c, err
	} else if taken {
		return c, ErrNameTaken
	}
	_, err = s.DB.Exec("UPDATE categories SET deleted_at=NULL WHERE id=?", id)
	return c, err
}

// PurgeDeletedCategories hard-deletes categories soft-deleted more than
// days ago and returns how many went.
func (s *Store) PurgeDeletedCategories(days int) (int64, error) {
	res, err := s.DB.Exec("DELETE FROM categories WHERE deleted_at IS NOT NULL AND deleted_at < NOW() - INTERVAL ? DAY", days)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetPostCategories replaces a post's live categories. Assignments to
// soft-deleted categories stay, so an undo restores them.
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
		if err := tx.QueryRow("SELECT COUNT(*) FROM categories WHERE id=? AND deleted_at IS NULL", c).Scan(&m); err != nil {
			return err
		}
		if m == 0 {
			return fmt.Errorf("unknown category: %s", c)
		}
	}
	if _, err := tx.Exec(`DELETE pc FROM post_categories pc JOIN categories c ON c.id=pc.category_id
		WHERE pc.post_id=? AND c.deleted_at IS NULL`, postID); err != nil {
		return err
	}
	for _, c := range catIDs {
		if _, err := tx.Exec("INSERT INTO post_categories(post_id,category_id) VALUES(?,?)", postID, c); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BatchCategory adds (add=true) or removes one live category for many posts
// in one transaction and returns the rows changed. Unknown post ids are
// skipped on add (FK) and ignored on remove; an unknown or deleted
// category → "unknown category" error.
func (s *Store) BatchCategory(postIDs []string, categoryID string, add bool) (int64, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM categories WHERE id=? AND deleted_at IS NULL", categoryID).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, fmt.Errorf("unknown category: %s", categoryID)
	}
	var changed int64
	for _, id := range postIDs {
		q := "DELETE FROM post_categories WHERE post_id=? AND category_id=?"
		if add {
			q = "INSERT IGNORE INTO post_categories(post_id,category_id) SELECT id, ? FROM posts WHERE id=?"
		}
		args := []any{id, categoryID}
		if add {
			args = []any{categoryID, id}
		}
		res, err := tx.Exec(q, args...)
		if err != nil {
			return 0, err
		}
		k, _ := res.RowsAffected()
		changed += k
	}
	return changed, tx.Commit()
}
