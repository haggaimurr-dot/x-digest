package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xdigest/internal/model"
	"xdigest/internal/store"
)

// ---- YouTube channels (yt-dlp sync, no API key) ----

// validYTDate reports whether s is a "2006-01-02" calendar date.
func validYTDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// optString is a JSON request field that distinguishes "absent" from
// "present as null" from "present as a string": absent → Set=false;
// null → Set=true, Val=nil; "x" → Set=true, Val=&"x". PATCH uses it so
// a date sent as null or "" clears the field while an absent field
// leaves it unchanged.
type optString struct {
	Set bool
	Val *string
}

func (o *optString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Val = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Val = &s
	return nil
}

// syncYTChannel syncs one channel's four tabs into yt_videos. A failed
// sync keeps the old videos and only updates the channel's sync status
// (mirrors syncRepo). After a successful sync, rows outside the
// channel's date range are pruned.
func (s *server) syncYTChannel(id string) (*model.YTChannel, error) {
	ch, err := s.st.GetYTChannel(id)
	if err != nil {
		return nil, err
	}
	if ch.SyncMode == "single" {
		return s.refreshSingleChannel(ch) // only the videos added by URL
	}
	// IDs already stored with a complete row (description, and for
	// non-playlists a publish date) skip the backfill this sync; the
	// sync re-checks everything else. Playlists count as complete
	// once they have a description — they never get a publish date.
	knownIDs := map[string]bool{}
	existing, err := s.st.ListYTVideos(store.YTVideoFilter{ChannelID: ch.ChannelID, Limit: 1000})
	if err != nil {
		return nil, err
	}
	for _, v := range existing {
		if v.Description != "" && (v.Kind == "playlist" || v.PublishedAt != "") {
			knownIDs[v.ID] = true
		}
	}
	dateFrom, dateTo := "", ""
	if ch.DateFrom != nil {
		dateFrom = *ch.DateFrom
	}
	if ch.DateTo != nil {
		dateTo = *ch.DateTo
	}
	res, err := s.yt.SyncTabs(context.Background(), ch.ChannelID, dateFrom, dateTo, knownIDs)
	if err != nil {
		// Defensive: SyncTabs currently folds tab failures into Status.
		_ = s.st.SetYTChannelSyncStatus(id, "failed", err.Error())
		ch, err = s.st.GetYTChannel(id)
		return &ch, err
	}
	if res.Status == "failed" {
		_ = s.st.SetYTChannelSyncStatus(id, "failed", res.Error)
		ch, err = s.st.GetYTChannel(id)
		return &ch, err
	}
	name := res.ChannelName
	if name == "" {
		name = ch.Name
	}
	if err := s.st.UpdateYTChannelMeta(id, name, "synced", ""); err != nil {
		return nil, err
	}
	if res.Videos != nil {
		if err := s.st.UpsertYTVideos(ch.ChannelID, res.Videos); err != nil {
			return nil, err
		}
	}
	// A narrowed range (or a re-sync) can leave rows outside it behind.
	if dateFrom != "" || dateTo != "" {
		if err := s.st.PruneYTVideosOutsideRange(ch.ChannelID, dateFrom, dateTo); err != nil {
			return nil, err
		}
	}
	ch, err = s.st.GetYTChannel(id)
	return &ch, err
}

func (s *server) handleYTChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels, err := s.st.ListYTChannels()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"channels": channels})
	case http.MethodPost:
		var body struct {
			ChannelID string `json:"channel_id"`
			URL       string `json:"url"`
			Handle    string `json:"handle"`
			GroupID   string `json:"group_id"`
			DateFrom  string `json:"date_from"`
			DateTo    string `json:"date_to"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		dateFrom := strings.TrimSpace(body.DateFrom)
		dateTo := strings.TrimSpace(body.DateTo)
		if dateFrom != "" && !validYTDate(dateFrom) {
			writeErr(w, 422, "VALIDATION", "date_from must be YYYY-MM-DD")
			return
		}
		if dateTo != "" && !validYTDate(dateTo) {
			writeErr(w, 422, "VALIDATION", "date_to must be YYYY-MM-DD")
			return
		}
		if dateFrom != "" && dateTo != "" && dateFrom > dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		input := strings.TrimSpace(body.ChannelID)
		if input == "" {
			input = strings.TrimSpace(body.URL)
		}
		if input == "" {
			input = strings.TrimSpace(body.Handle)
		}
		channelID, err := s.yt.ResolveChannelID(r.Context(), input)
		if err != nil {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		if body.GroupID != "" && !s.st.YTGroupExists(body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
			return
		}
		if _, err := s.st.GetYTChannelByChannelID(channelID); err == nil {
			writeErr(w, 409, "CONFLICT", "channel already exists: "+channelID)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		// Keep the @handle when the user gave one; the channel name
		// comes from the tab listing's playlist_title on the first sync.
		handle := strings.TrimSpace(body.Handle)
		if handle == "" && strings.HasPrefix(input, "@") {
			handle = input
		}
		ch := model.YTChannel{
			ID: store.NewID("ytc_"), ChannelID: channelID, Handle: handle,
			Name: "", GroupID: body.GroupID, CreatedAt: store.Now(),
			SyncStatus: "never",
		}
		if dateFrom != "" {
			ch.DateFrom = &dateFrom
		}
		if dateTo != "" {
			ch.DateTo = &dateTo
		}
		if err := s.st.CreateYTChannel(ch); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		// Sync once immediately so the page shows videos right away; a
		// sync failure keeps the channel with its error status.
		synced, err := s.syncYTChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, synced)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleYTChannelSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/yt/channels/")
	parts := strings.Split(rest, "/")
	if len(parts) < 1 || parts[0] == "" {
		writeErr(w, 404, "NOT_FOUND", "channel not found")
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "sync" {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		if !s.st.YTChannelExists(id) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		// ?full=1 on a single-video channel = 同步该账号全部视频: switch it
		// to full mode, then run the normal tab sync.
		if r.URL.Query().Get("full") == "1" {
			if err := s.st.SetYTChannelSyncMode(id, "full"); err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
		}
		ch, err := s.syncYTChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
		return
	}
	if len(parts) != 1 {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		ch, err := s.st.GetYTChannel(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodPatch:
		// PATCH {group_id?, date_from?, date_to?}: a field present as
		// null or "" clears it; an absent field leaves it unchanged.
		// Returns the updated channel.
		ch, err := s.st.GetYTChannel(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		var body struct {
			GroupID  optString `json:"group_id"`
			DateFrom optString `json:"date_from"`
			DateTo   optString `json:"date_to"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		groupID := ch.GroupID
		if body.GroupID.Set {
			if body.GroupID.Val == nil || strings.TrimSpace(*body.GroupID.Val) == "" {
				groupID = ""
			} else {
				groupID = strings.TrimSpace(*body.GroupID.Val)
			}
			if groupID != "" && !s.st.YTGroupExists(groupID) {
				writeErr(w, 422, "VALIDATION", "group does not exist: "+groupID)
				return
			}
		}
		dateFrom, dateTo := ch.DateFrom, ch.DateTo
		if body.DateFrom.Set {
			if body.DateFrom.Val == nil || strings.TrimSpace(*body.DateFrom.Val) == "" {
				dateFrom = nil
			} else {
				v := strings.TrimSpace(*body.DateFrom.Val)
				if !validYTDate(v) {
					writeErr(w, 422, "VALIDATION", "date_from must be YYYY-MM-DD")
					return
				}
				dateFrom = &v
			}
		}
		if body.DateTo.Set {
			if body.DateTo.Val == nil || strings.TrimSpace(*body.DateTo.Val) == "" {
				dateTo = nil
			} else {
				v := strings.TrimSpace(*body.DateTo.Val)
				if !validYTDate(v) {
					writeErr(w, 422, "VALIDATION", "date_to must be YYYY-MM-DD")
					return
				}
				dateTo = &v
			}
		}
		if dateFrom != nil && dateTo != nil && *dateFrom > *dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		if err := s.st.UpdateYTChannel(id, groupID, dateFrom, dateTo); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		ch, err = s.st.GetYTChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodDelete:
		if !s.st.YTChannelExists(id) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		if err := s.st.DeleteYTChannel(id); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH, DELETE")
	}
}

func (s *server) handleYTVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.addYTVideo(w, r) // single-video configuration, see yt_single.go
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
		return
	}
	q := r.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	kind := q.Get("kind")
	switch kind {
	case "", "video", "short", "live", "playlist":
	default:
		writeErr(w, 422, "VALIDATION", "kind must be video, short, live or playlist")
		return
	}
	videos, err := s.st.ListYTVideos(store.YTVideoFilter{
		WatchLater: q.Get("watch_later") == "1",
		ChannelID:  q.Get("channel_id"),
		Q:          q.Get("q"),
		DateFrom:   q.Get("date_from"),
		DateTo:     q.Get("date_to"),
		Kind:       kind,
		Limit:      limit,
	})
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"videos": videos})
}

func (s *server) handleYTSyncAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	channels, err := s.st.ListYTChannels()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	total := len(channels)
	synced, failed, skipped := 0, 0, 0
	for _, ch := range channels {
		// Single-video channels are not pulled in full by 同步全部; their
		// own 同步该账号全部视频 button does that.
		if ch.SyncMode == "single" {
			skipped++
			continue
		}
		updated, err := s.syncYTChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		switch updated.SyncStatus {
		case "synced":
			synced++
		default: // "failed", "never"
			failed++
		}
	}
	writeJSON(w, 200, map[string]any{
		"synced": synced, "failed": failed, "total": total, "skipped": skipped,
	})
}

func (s *server) handleYTGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := s.st.ListYTGroups()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"groups": groups})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			writeErr(w, 422, "VALIDATION", "name is required")
			return
		}
		g, err := s.st.CreateYTGroup(body.Name)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, g)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleYTGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/yt/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := s.st.DeleteYTGroup(id)
	if code == "NOT_FOUND" {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	if code == "GROUP_FIXED" {
		writeErr(w, 422, "GROUP_FIXED", "fixed groups cannot be deleted")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
