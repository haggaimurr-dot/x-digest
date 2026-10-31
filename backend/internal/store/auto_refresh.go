package store

import "strings"

// AutoRefreshUsernames lists the accounts the scheduled refresh
// (REFRESH_INTERVAL) covers, see schema.sql authors.auto_refresh.
func (s *Store) AutoRefreshUsernames() ([]string, error) {
	rows, err := s.DB.Query("SELECT username FROM authors WHERE auto_refresh=1 ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetAutoRefresh makes usernames the whole auto-refresh set (an empty
// list clears it). Unknown usernames are ignored; the caller validates.
func (s *Store) SetAutoRefresh(usernames []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE authors SET auto_refresh=0 WHERE auto_refresh=1"); err != nil {
		return err
	}
	if len(usernames) > 0 {
		args := make([]any, len(usernames))
		for i, u := range usernames {
			args[i] = u
		}
		ph := strings.TrimSuffix(strings.Repeat("?,", len(usernames)), ",")
		if _, err := tx.Exec("UPDATE authors SET auto_refresh=1 WHERE username IN ("+ph+")", args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
