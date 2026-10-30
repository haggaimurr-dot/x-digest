package main

// videoHub serves one video platform under /api/v1/<prefix>/ with the same
// routes and JSON for every platform, so the frontend page is shared
// (2026-10-27). YouTube ("yt", hub_yt.go) and Bilibili ("bili") run on it;
// YouTube moved over from its own handlers on 2026-10-29.
//
//	GET/POST            /api/v1/<p>/channels
//	GET/PATCH/DELETE    /api/v1/<p>/channels/{id}     POST .../sync[?full=1]
//	GET                 /api/v1/<p>/videos            POST: add one video by URL (hub_single.go)
//	PATCH               /api/v1/<p>/videos/{id}       watch state (video_state.go)
//	POST                /api/v1/<p>/sync
//	GET/POST            /api/v1/<p>/groups            DELETE .../{id}
//	GET/POST            /api/v1/<p>/videos/{id}/summary   GET .../summary/stream
//
// Platform-specific work (resolving a channel, syncing, reading a video's
// subtitles, reading one video by URL) goes through hubPlatform; storage is
// store.HubStore on the <prefix>_* tables. AI summaries share one job pool
// (model, concurrency cap) with the post summaries.

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"xdigest/internal/model"
	"xdigest/internal/store"
	"xdigest/internal/youtube"
)

type hubPlatform interface {
	// Resolve turns user input into a channel ID and display name.
	Resolve(ctx context.Context, input string) (channelID, name string, err error)
	Sync(ctx context.Context, channelID, dateFrom, dateTo string, known map[string]bool) (*youtube.SyncResult, error)
	FetchDetail(ctx context.Context, videoID string) (*youtube.VideoDetail, error)
}

// singleVideoPlatform is a hubPlatform that can read one video by URL
// (YouTube). Without it POST /videos answers 405.
type singleVideoPlatform interface {
	FetchVideo(ctx context.Context, input string) (*youtube.SingleVideo, error)
}

type videoHub struct {
	prefix   string // URL segment and table prefix, e.g. "bili"
	idPrefix string // channel row id prefix, e.g. "bic_"
	kinds    map[string]bool
	st       *store.HubStore
	pf       hubPlatform
	sum      *summaryJobs
	// playlistName is the platform's word for a playlist row in messages
	// ("播放列表" / "合集").
	playlistName string
	// handle picks a new channel's display handle from the request (input,
	// the "handle" field) and the resolved channel ID; nil = "UID <id>".
	handle func(input, given, channelID string) string
}

func (h *videoHub) register(mux *http.ServeMux) {
	base := "/api/v1/" + h.prefix
	mux.HandleFunc(base+"/channels", h.handleChannels)
	mux.HandleFunc(base+"/channels/", h.handleChannelSub)
	mux.HandleFunc(base+"/videos", h.handleVideos)
	mux.HandleFunc(base+"/videos/", h.handleVideoSub)
	mux.HandleFunc(base+"/videos/preview", h.handleVideoPreview)
	mux.HandleFunc(base+"/sync", h.handleSyncAll)
	mux.HandleFunc(base+"/groups", h.handleGroups)
	mux.HandleFunc(base+"/groups/", h.handleGroup)
}

// ---- channels ----

// syncChannel syncs one channel into <p>_videos. A failed sync keeps the
// old videos and only records the error; after a successful one, rows
// outside the channel's date range are pruned. Videos already stored
// complete (a description, and a date unless a playlist) are passed as
// known so the platform can skip re-reading them.
func (h *videoHub) syncChannel(id string) (*model.YTChannel, error) {
	ch, err := h.st.GetChannel(id)
	if err != nil {
		return nil, err
	}
	if ch.SyncMode == "single" {
		return h.refreshSingle(ch) // only the videos added by URL
	}
	known := map[string]bool{}
	existing, err := h.st.ListVideos(store.YTVideoFilter{ChannelID: ch.ChannelID, Limit: 1000})
	if err != nil {
		return nil, err
	}
	for _, v := range existing {
		if v.Description != "" && (v.Kind == "playlist" || v.PublishedAt != "") {
			known[v.ID] = true
		}
	}
	dateFrom, dateTo := "", ""
	if ch.DateFrom != nil {
		dateFrom = *ch.DateFrom
	}
	if ch.DateTo != nil {
		dateTo = *ch.DateTo
	}
	res, err := h.pf.Sync(context.Background(), ch.ChannelID, dateFrom, dateTo, known)
	if err == nil && res.Status == "failed" {
		err = errors.New(res.Error)
	}
	if err != nil {
		_ = h.st.SetChannelSyncStatus(id, "failed", err.Error())
		ch, err = h.st.GetChannel(id)
		return &ch, err
	}
	name := res.ChannelName
	if name == "" {
		name = ch.Name
	}
	// A partial sync (one tab failed) is still "synced"; the failed tab
	// is reported in sync_error so the page shows it.
	if err := h.st.UpdateChannelMeta(id, name, "synced", strings.Join(res.FailedTabs, "；")); err != nil {
		return nil, err
	}
	if err := h.st.UpsertVideos(ch.ChannelID, res.Videos); err != nil {
		return nil, err
	}
	if err := h.st.PruneVideosOutsideRange(ch.ChannelID, dateFrom, dateTo); err != nil {
		return nil, err
	}
	ch, err = h.st.GetChannel(id)
	return &ch, err
}

func (h *videoHub) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels, err := h.st.ListChannels()
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
		dateFrom, dateTo := strings.TrimSpace(body.DateFrom), strings.TrimSpace(body.DateTo)
		if (dateFrom != "" && !validYTDate(dateFrom)) || (dateTo != "" && !validYTDate(dateTo)) {
			writeErr(w, 422, "VALIDATION", "dates must be YYYY-MM-DD")
			return
		}
		if dateFrom != "" && dateTo != "" && dateFrom > dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		if body.GroupID != "" && !h.st.GroupExists(body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
			return
		}
		input := strings.TrimSpace(body.ChannelID)
		if input == "" {
			input = strings.TrimSpace(body.URL)
		}
		if input == "" {
			input = strings.TrimSpace(body.Handle)
		}
		channelID, name, err := h.pf.Resolve(r.Context(), input)
		if err != nil {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		if _, err := h.st.GetChannelByChannelID(channelID); err == nil {
			writeErr(w, 409, "CONFLICT", "channel already exists: "+channelID)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		handle := "UID " + channelID
		if h.handle != nil {
			handle = h.handle(input, strings.TrimSpace(body.Handle), channelID)
		}
		ch := model.YTChannel{ID: store.NewID(h.idPrefix), ChannelID: channelID, Handle: handle,
			Name: name, GroupID: body.GroupID, CreatedAt: store.Now(), SyncStatus: "never"}
		if dateFrom != "" {
			ch.DateFrom = &dateFrom
		}
		if dateTo != "" {
			ch.DateTo = &dateTo
		}
		if err := h.st.CreateChannel(ch); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		synced, err := h.syncChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, synced)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (h *videoHub) handleChannelSub(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"+h.prefix+"/channels/"), "/")
	id := parts[0]
	if id == "" || len(parts) > 2 || (len(parts) == 2 && parts[1] != "sync") {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	if !h.st.ChannelExists(id) {
		writeErr(w, 404, "NOT_FOUND", "channel not found")
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		// ?full=1 on a single-mode channel = 同步该账号全部视频: switch it
		// to full mode, then run the normal sync.
		if r.URL.Query().Get("full") == "1" {
			if err := h.st.SetChannelSyncMode(id, "full"); err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
		}
		ch, err := h.syncChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
		return
	}
	switch r.Method {
	case http.MethodGet:
		ch, err := h.st.GetChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodPatch:
		ch, err := h.st.GetChannel(id)
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
			groupID = ""
			if body.GroupID.Val != nil {
				groupID = strings.TrimSpace(*body.GroupID.Val)
			}
			if groupID != "" && !h.st.GroupExists(groupID) {
				writeErr(w, 422, "VALIDATION", "group does not exist: "+groupID)
				return
			}
		}
		dateFrom, dateTo := ch.DateFrom, ch.DateTo
		for _, f := range []struct {
			in  optString
			out **string
		}{{body.DateFrom, &dateFrom}, {body.DateTo, &dateTo}} {
			if !f.in.Set {
				continue
			}
			if f.in.Val == nil || strings.TrimSpace(*f.in.Val) == "" {
				*f.out = nil
				continue
			}
			v := strings.TrimSpace(*f.in.Val)
			if !validYTDate(v) {
				writeErr(w, 422, "VALIDATION", "dates must be YYYY-MM-DD")
				return
			}
			*f.out = &v
		}
		if dateFrom != nil && dateTo != nil && *dateFrom > *dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		if err := h.st.UpdateChannel(id, groupID, dateFrom, dateTo); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		ch, err = h.st.GetChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodDelete:
		if err := h.st.DeleteChannel(id); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH, DELETE")
	}
}

func (h *videoHub) handleVideos(w http.ResponseWriter, r *http.Request) {
	if pf, ok := h.pf.(singleVideoPlatform); ok && r.Method == http.MethodPost {
		h.addVideo(w, r, pf) // hub_single.go
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	q := r.URL.Query()
	limit := 100
	if n, err := strconv.Atoi(q.Get("limit")); err == nil {
		limit = n
	}
	kind := q.Get("kind")
	if kind != "" && !h.kinds[kind] {
		writeErr(w, 422, "VALIDATION", "unknown kind: "+kind)
		return
	}
	videos, err := h.st.ListVideos(store.YTVideoFilter{ChannelID: q.Get("channel_id"), Q: q.Get("q"),
		DateFrom: q.Get("date_from"), DateTo: q.Get("date_to"), Kind: kind, Limit: limit, WatchLater: q.Get("watch_later") == "1",
		Sort: q.Get("sort")})
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"videos": videos})
}

func (h *videoHub) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	channels, err := h.st.ListChannels()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	synced, failed, skipped := 0, 0, 0
	for _, ch := range channels {
		// Single-mode channels are not pulled in full by 同步全部; their
		// own 同步该账号全部视频 does that.
		if ch.SyncMode == "single" {
			skipped++
			continue
		}
		updated, err := h.syncChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if updated.SyncStatus == "synced" {
			synced++
		} else {
			failed++
		}
	}
	writeJSON(w, 200, map[string]any{"synced": synced, "failed": failed, "total": len(channels), "skipped": skipped})
}

func (h *videoHub) handleGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := h.st.ListGroups()
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
		g, err := h.st.CreateGroup(strings.TrimSpace(body.Name))
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, g)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (h *videoHub) handleGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/"+h.prefix+"/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := h.st.DeleteGroup(id)
	switch {
	case code == "NOT_FOUND":
		writeErr(w, 404, "NOT_FOUND", "group not found")
	case code == "GROUP_FIXED":
		writeErr(w, 422, "GROUP_FIXED", "fixed groups cannot be deleted")
	case err != nil:
		writeErr(w, 500, "DB_ERROR", err.Error())
	default:
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}
