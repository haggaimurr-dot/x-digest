package main

// YouTube single-video configuration (2026-10-27):
//
//	POST /api/v1/yt/videos {"url": "...", "group_id": "..."}
//	     → 201 {"video": YTVideo, "channel": YTChannel, "channel_created": bool}
//
// One yt-dlp metadata call reads the video and the channel it belongs to.
// A channel not followed yet is created in "single" mode: it holds only
// the videos added by URL, 同步全部 skips it, and its own
// 同步该账号全部视频 (POST /yt/channels/{id}/sync?full=1) switches it to a
// normal full channel. Videos added this way are flagged added_manually
// so a channel's date range never prunes them.

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"xdigest/internal/model"
	"xdigest/internal/store"
)

func (s *server) addYTVideo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL     string `json:"url"`
		GroupID string `json:"group_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.GroupID != "" && !s.st.YTGroupExists(body.GroupID) {
		writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
		return
	}
	sv, err := s.yt.FetchVideo(r.Context(), body.URL)
	if err != nil {
		writeErr(w, 422, "VALIDATION", err.Error())
		return
	}
	created := false
	ch, err := s.st.GetYTChannelByChannelID(sv.ChannelID)
	if errors.Is(err, sql.ErrNoRows) {
		ch = model.YTChannel{
			ID: store.NewID("ytc_"), ChannelID: sv.ChannelID, Handle: sv.Handle, Name: sv.ChannelName,
			GroupID: body.GroupID, CreatedAt: store.Now(), SyncStatus: "synced", SyncMode: "single",
		}
		if err := s.st.CreateYTChannel(ch); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		created = true
	} else if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if err := s.st.UpsertYTVideos(sv.ChannelID, []model.YTVideo{sv.Video}); err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if err := s.st.MarkYTVideoManual(sv.Video.ID); err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if created || ch.SyncMode == "single" {
		name := ch.Name
		if name == "" {
			name = sv.ChannelName
		}
		_ = s.st.UpdateYTChannelMeta(ch.ID, name, "synced", "")
	}
	ch, err = s.st.GetYTChannel(ch.ID)
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

// refreshSingleChannel re-reads the metadata of a single-mode channel's
// videos (titles, descriptions, live status change over time). A video
// that can no longer be read keeps its stored row.
func (s *server) refreshSingleChannel(ch model.YTChannel) (*model.YTChannel, error) {
	urls, err := s.st.ManualYTVideoURLs(ch.ChannelID)
	if err != nil {
		return nil, err
	}
	failed := 0
	var videos []model.YTVideo
	for _, u := range urls {
		sv, err := s.yt.FetchVideo(context.Background(), u)
		if err != nil || sv.ChannelID != ch.ChannelID {
			failed++
			continue
		}
		videos = append(videos, sv.Video)
	}
	if err := s.st.UpsertYTVideos(ch.ChannelID, videos); err != nil {
		return nil, err
	}
	status, msg := "synced", ""
	if failed > 0 && len(videos) == 0 {
		status, msg = "failed", "视频信息读取失败"
	} else if failed > 0 {
		msg = "部分视频读取失败"
	}
	if err := s.st.SetYTChannelSyncStatus(ch.ID, status, msg); err != nil {
		return nil, err
	}
	out, err := s.st.GetYTChannel(ch.ID)
	return &out, err
}
