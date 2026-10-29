package main

// Adding one video by URL (2026-10-27, on the hub since 2026-10-29):
//
//	POST /api/v1/<p>/videos {"url": "...", "group_id": "..."}
//	     → 201 {"video": YTVideo, "channel": YTChannel, "channel_created": true}
//	       (200 with channel_created false when the channel is followed)
//
// One metadata call reads the video and the channel it belongs to. A
// channel not followed yet is created in "single" mode: it holds only the
// videos added by URL, 同步全部 skips it, and its own 同步该账号全部视频
// (POST .../channels/{id}/sync?full=1) switches it to a normal channel.
// Videos added this way are flagged added_manually so a channel's date
// range never prunes them. Only platforms with FetchVideo (YouTube) serve it.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"xdigest/internal/model"
	"xdigest/internal/store"
)

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

func (h *videoHub) addVideo(w http.ResponseWriter, r *http.Request, pf singleVideoPlatform) {
	var body struct {
		URL     string   `json:"url"`
		GroupID string   `json:"group_id"`
		Tags    []string `json:"tags"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.GroupID != "" && !h.st.GroupExists(body.GroupID) {
		writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
		return
	}
	sv, err := pf.FetchVideo(r.Context(), body.URL)
	if err != nil {
		writeErr(w, 422, "VALIDATION", err.Error())
		return
	}
	created := false
	ch, err := h.st.GetChannelByChannelID(sv.ChannelID)
	if errors.Is(err, sql.ErrNoRows) {
		ch = model.YTChannel{
			ID: store.NewID(h.idPrefix), ChannelID: sv.ChannelID, Handle: sv.Handle, Name: sv.ChannelName,
			GroupID: body.GroupID, CreatedAt: store.Now(), SyncStatus: "synced", SyncMode: "single",
		}
		if err := h.st.CreateChannel(ch); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		created = true
	} else if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if err := h.st.UpsertVideos(sv.ChannelID, []model.YTVideo{sv.Video}); err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if err := h.st.MarkVideoManual(sv.Video.ID); err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if len(body.Tags) > 0 {
		if err := h.st.SetVideoTags(sv.Video.ID, body.Tags); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
	}
	if created || ch.SyncMode == "single" {
		name := ch.Name
		if name == "" {
			name = sv.ChannelName
		}
		_ = h.st.UpdateChannelMeta(ch.ID, name, "synced", "")
	}
	ch, err = h.st.GetChannel(ch.ID)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	video := sv.Video
	video.ChannelName = ch.Name
	status := 200
	if created {
		status = 201
	}
	writeJSON(w, status, map[string]any{"video": video, "channel": ch, "channel_created": created})
}

// refreshSingle re-reads the videos of a single-mode channel (titles,
// descriptions and live status change over time). A video that can no
// longer be read keeps its stored row.
func (h *videoHub) refreshSingle(ch model.YTChannel) (*model.YTChannel, error) {
	pf, ok := h.pf.(singleVideoPlatform)
	if !ok {
		return &ch, nil
	}
	urls, err := h.st.ManualVideoURLs(ch.ChannelID)
	if err != nil {
		return nil, err
	}
	failed := 0
	var videos []model.YTVideo
	for _, u := range urls {
		sv, err := pf.FetchVideo(context.Background(), u)
		if err != nil || sv.ChannelID != ch.ChannelID {
			failed++
			continue
		}
		videos = append(videos, sv.Video)
	}
	if err := h.st.UpsertVideos(ch.ChannelID, videos); err != nil {
		return nil, err
	}
	status, msg := "synced", ""
	if failed > 0 && len(videos) == 0 {
		status, msg = "failed", "视频信息读取失败"
	} else if failed > 0 {
		msg = "部分视频读取失败"
	}
	if err := h.st.SetChannelSyncStatus(ch.ID, status, msg); err != nil {
		return nil, err
	}
	out, err := h.st.GetChannel(ch.ID)
	return &out, err
}
