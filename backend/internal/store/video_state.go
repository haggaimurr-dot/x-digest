package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// SetVideoState updates the watch state of one video of a platform ("yt" or
// "bili"); nil leaves a field alone. watched=true stamps watched_at now,
// false clears it. sql.ErrNoRows for an unknown video.
func (s *Store) SetVideoState(platform, id string, watched, later *bool) error {
	if platform != "yt" && platform != "bili" {
		return fmt.Errorf("unknown platform %q", platform)
	}
	var sets []string
	var args []any
	if watched != nil {
		if *watched {
			sets, args = append(sets, "watched_at=?"), append(args, Now())
		} else {
			sets = append(sets, "watched_at=NULL")
		}
	}
	if later != nil {
		sets, args = append(sets, "watch_later=?"), append(args, *later)
	}
	if len(sets) == 0 {
		return nil
	}
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM "+platform+"_videos WHERE id=?", id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	_, err := s.DB.Exec("UPDATE "+platform+"_videos SET "+strings.Join(sets, ",")+" WHERE id=?", append(args, id)...)
	return err
}

// SetVideoState for one platform's hub store (same tables, same rules).
func (h *HubStore) SetVideoState(platform, id string, watched, later *bool) error {
	return (&Store{DB: h.DB}).SetVideoState(platform, id, watched, later)
}
