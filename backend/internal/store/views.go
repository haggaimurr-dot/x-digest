package store

import (
	"encoding/json"
	"errors"

	"github.com/go-sql-driver/mysql"
)

// SavedView is one saved filter combination of the X daily page.
type SavedView struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Filter    json.RawMessage `json:"filter"`
	SortOrder int             `json:"sort_order"`
}

// ErrDuplicate: a unique key (e.g. the view name) is already taken.
var ErrDuplicate = errors.New("duplicate")

func (s *Store) ListViews() ([]SavedView, error) {
	rows, err := s.DB.Query("SELECT id,name,filter_json,sort_order FROM saved_views ORDER BY sort_order, created_at, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedView{}
	for rows.Next() {
		var v SavedView
		var f string
		if err := rows.Scan(&v.ID, &v.Name, &f, &v.SortOrder); err != nil {
			return nil, err
		}
		v.Filter = json.RawMessage(f)
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateView appends a view (sort_order after the last one); ErrDuplicate
// when the name exists.
func (s *Store) CreateView(name string, filter json.RawMessage) (SavedView, error) {
	v := SavedView{ID: NewID("view_"), Name: name, Filter: filter}
	err := s.DB.QueryRow("SELECT COALESCE(MAX(sort_order),0)+1 FROM saved_views").Scan(&v.SortOrder)
	if err != nil {
		return v, err
	}
	_, err = s.DB.Exec("INSERT INTO saved_views(id,name,filter_json,sort_order) VALUES(?,?,?,?)", v.ID, v.Name, string(filter), v.SortOrder)
	if isDupKey(err) {
		return v, ErrDuplicate
	}
	return v, err
}

// DeleteView: false when there was no such view.
func (s *Store) DeleteView(id string) (bool, error) {
	res, err := s.DB.Exec("DELETE FROM saved_views WHERE id=?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func isDupKey(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
