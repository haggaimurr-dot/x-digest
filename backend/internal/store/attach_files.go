package store

import (
	"os"
	"path/filepath"
	"time"
)

// CleanOrphanFiles removes files in the attachment directory that no
// attachments row points at (left behind when a delete failed half-way) and
// returns their names. Files younger than minAge are kept: an upload writes
// the file before its row.
func CleanOrphanFiles(dir string, known map[string]bool, minAge time.Duration, now time.Time) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		if e.IsDir() || known[e.Name()] {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < minAge {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			removed = append(removed, e.Name())
		}
	}
	return removed, nil
}
