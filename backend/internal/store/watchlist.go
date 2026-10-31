package store

import (
	"database/sql"
	"fmt"
	"strings"

	"xdigest/internal/model"
)

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
	if _, err := tx.Exec("DELETE FROM authors WHERE username=?", username); err != nil {
		return err
	}
	return tx.Commit()
}

// PostCounts30d: posts per author published in the last 30 days (authors
// without posts are absent).
func (s *Store) PostCounts30d() (map[string]int, error) {
	rows, err := s.DB.Query("SELECT author, COUNT(*) FROM posts WHERE published_at >= NOW() - INTERVAL 30 DAY GROUP BY author")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var a string
		var n int
		if err := rows.Scan(&a, &n); err != nil {
			return nil, err
		}
		out[a] = n
	}
	return out, rows.Err()
}
