// Package store implements all MariaDB access for the x-digest API.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"
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
