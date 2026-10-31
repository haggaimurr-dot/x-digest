package store

import (
	"encoding/json"
	"time"
)

// GetUserSettings returns one settings copy (sql.ErrNoRows when none).
// updated_at is stored as UTC.
func (s *Store) GetUserSettings(key string) (json.RawMessage, time.Time, error) {
	var raw, at string
	err := s.DB.QueryRow("SELECT json, DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%s.%fZ') FROM user_settings WHERE token_hash=?", key).Scan(&raw, &at)
	if err != nil {
		return nil, time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	return json.RawMessage(raw), t, err
}

func (s *Store) PutUserSettings(key string, settings json.RawMessage, updatedAt time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO user_settings(token_hash,json,updated_at) VALUES(?,?,?)
		ON DUPLICATE KEY UPDATE json=VALUES(json), updated_at=VALUES(updated_at)`,
		key, string(settings), updatedAt.UTC().Format("2006-01-02 15:04:05.000"))
	return err
}
