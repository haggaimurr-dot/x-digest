package main

import (
	"database/sql"
	"errors"
	"net/http"
)

// PATCH /api/v1/{yt|bili}/videos/{id} {"watched": bool, "watch_later": bool}
// (either field optional) → 200 {ok}; 404 unknown video; 422 empty body.
// GET /api/v1/{yt|bili}/videos?watch_later=1 lists the 稍后观看 videos.

type videoStateStore interface {
	SetVideoState(platform, id string, watched, later *bool) error
}

func patchVideoState(w http.ResponseWriter, r *http.Request, st videoStateStore, platform, id string) {
	if r.Method != http.MethodPatch {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only PATCH")
		return
	}
	var body struct {
		Watched    *bool `json:"watched"`
		WatchLater *bool `json:"watch_later"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Watched == nil && body.WatchLater == nil {
		writeErr(w, 422, "VALIDATION", "watched or watch_later is required")
		return
	}
	err := st.SetVideoState(platform, id, body.Watched, body.WatchLater)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "NOT_FOUND", "video not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "watched": body.Watched, "watch_later": body.WatchLater})
}
