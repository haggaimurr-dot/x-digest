package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

// PATCH /api/v1/{yt|bili}/videos/{id} {"watched": bool, "watch_later": bool,
// "tags": [string]} (every field optional; tags replaces the video's user
// tags, a "-name" entry hides an auto-extracted tag) → 200 {ok}; 404 unknown
// video; 422 empty body.
// GET /api/v1/{yt|bili}/videos?watch_later=1 lists the 稍后观看 videos.

type videoStateStore interface {
	SetVideoState(platform, id string, watched, later *bool) error
	SetVideoTags(videoID string, tags []string) error
}

func patchVideoState(w http.ResponseWriter, r *http.Request, st videoStateStore, platform, id string) {
	if r.Method != http.MethodPatch {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only PATCH")
		return
	}
	var body struct {
		Watched    *bool     `json:"watched"`
		WatchLater *bool     `json:"watch_later"`
		Tags       *[]string `json:"tags"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Watched == nil && body.WatchLater == nil && body.Tags == nil {
		writeErr(w, 422, "VALIDATION", "watched, watch_later or tags is required")
		return
	}
	var tags []string
	if body.Tags != nil {
		if tags = cleanVideoTags(*body.Tags); tags == nil {
			writeErr(w, 422, "VALIDATION", "too many tags or a tag is too long")
			return
		}
	}
	var err error
	if body.Watched != nil || body.WatchLater != nil {
		err = st.SetVideoState(platform, id, body.Watched, body.WatchLater)
	}
	if err == nil && body.Tags != nil {
		err = st.SetVideoTags(id, tags)
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "NOT_FOUND", "video not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "watched": body.Watched, "watch_later": body.WatchLater, "tags": body.Tags})
}

// cleanVideoTags trims and de-duplicates; nil (never an empty slice) when
// over 50 tags or a tag over 64 characters.
func cleanVideoTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || t == "-" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > 64 {
			return nil
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > 50 {
		return nil
	}
	return out
}
