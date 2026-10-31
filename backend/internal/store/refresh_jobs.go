package store

// SaveRefreshJob upserts one X refresh job (see schema.sql refresh_jobs).
func (s *Store) SaveRefreshJob(id string, running bool, state []byte) error {
	_, err := s.DB.Exec(`INSERT INTO refresh_jobs(id,running,state) VALUES(?,?,?)
		ON DUPLICATE KEY UPDATE running=VALUES(running), state=VALUES(state)`, id, running, string(state))
	return err
}

// LatestRefreshJob returns the newest job's state; sql.ErrNoRows when none.
func (s *Store) LatestRefreshJob() ([]byte, error) {
	var state string
	err := s.DB.QueryRow(`SELECT state FROM refresh_jobs ORDER BY started_at DESC, id DESC LIMIT 1`).Scan(&state)
	if err != nil {
		return nil, err
	}
	return []byte(state), nil
}
