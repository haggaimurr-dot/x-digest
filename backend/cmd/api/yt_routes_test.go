package main

// Baseline for moving YouTube onto videoHub (2026-10-29): the behaviour of
// /api/v1/yt/* that yt_test.go did not pin yet — add a single video by URL,
// single-mode channels, groups, watch state and the summary guards. Same
// scratch database and fake yt-dlp as yt_test.go.

import (
	"net/http"
	"testing"
)

// Adding a video by URL creates its channel in single mode; 同步全部 skips
// that channel; its own sync re-reads only the added videos; sync?full=1
// turns it into a normal channel, and the added video survives the
// channel's date range.
func TestYTSingleVideoFlow(t *testing.T) {
	s := testYTServerWithFakeBin(t)
	h := ytMux(s)

	code, body := ytCall(t, h, http.MethodPost, "/api/v1/yt/videos", map[string]any{"url": "https://www.youtube.com/watch?v=single00001"})
	if code != 201 || body["channel_created"] != true {
		t.Fatalf("add video: code=%d body=%v, want 201 channel_created", code, body)
	}
	ch, _ := body["channel"].(map[string]any)
	video, _ := body["video"].(map[string]any)
	if ch["sync_mode"] != "single" || ch["handle"] != "@singlechan" || ch["name"] != "Single Chan" || ch["sync_status"] != "synced" {
		t.Fatalf("channel = %v", ch)
	}
	if video["id"] != "single00001" || video["channel_name"] != "Single Chan" || video["published_at"] != "2026-10-20 00:00:00" {
		t.Fatalf("video = %v", video)
	}
	id, _ := ch["id"].(string)

	// The same video again: 200, no new channel.
	code, body = ytCall(t, h, http.MethodPost, "/api/v1/yt/videos", map[string]any{"url": "single00001"})
	if code != 200 || body["channel_created"] != false {
		t.Fatalf("re-add: code=%d body=%v, want 200", code, body)
	}
	// Bad link / unknown group: 422.
	if code, _ = ytCall(t, h, http.MethodPost, "/api/v1/yt/videos", map[string]any{"url": "https://www.youtube.com/watch?v=missing0000"}); code != 422 {
		t.Fatalf("unreadable video: code=%d, want 422", code)
	}
	if code, _ = ytCall(t, h, http.MethodPost, "/api/v1/yt/videos", map[string]any{"url": "single00001", "group_id": "nope"}); code != 422 {
		t.Fatalf("unknown group: code=%d, want 422", code)
	}

	// 同步全部 skips the single-mode channel.
	code, body = ytCall(t, h, http.MethodPost, "/api/v1/yt/sync", nil)
	if code != 200 || body["skipped"] != float64(1) || body["synced"] != float64(0) || body["total"] != float64(1) {
		t.Fatalf("sync-all: code=%d body=%v, want skipped=1 synced=0 total=1", code, body)
	}
	// Its own sync re-reads the added video only.
	code, body = ytCall(t, h, http.MethodPost, "/api/v1/yt/channels/"+id+"/sync", nil)
	if code != 200 || body["sync_mode"] != "single" || body["sync_status"] != "synced" || body["video_count"] != float64(1) {
		t.Fatalf("single sync: code=%d body=%v", code, body)
	}

	// A range that excludes the added video, then a full sync: the channel
	// switches to full mode and the video added by URL is kept.
	if code, body = ytCall(t, h, http.MethodPatch, "/api/v1/yt/channels/"+id, map[string]any{"date_from": "2026-10-22"}); code != 200 {
		t.Fatalf("patch range: code=%d body=%v", code, body)
	}
	code, body = ytCall(t, h, http.MethodPost, "/api/v1/yt/channels/"+id+"/sync?full=1", nil)
	if code != 200 || body["sync_mode"] != "full" {
		t.Fatalf("full sync: code=%d body=%v, want sync_mode full", code, body)
	}
	code, body = ytCall(t, h, http.MethodGet, "/api/v1/yt/videos?channel_id=UCsinglesinglesinglesi00", nil)
	found := false
	for _, v := range body["videos"].([]any) {
		if v.(map[string]any)["id"] == "single00001" {
			found = true
		}
	}
	if code != 200 || !found {
		t.Fatalf("video added by URL was pruned by the date range: %v", body)
	}
}

func TestYTGroupsAndWatchState(t *testing.T) {
	s := testYTServerWithFakeBin(t)
	h := ytMux(s)
	if _, body := ytCall(t, h, http.MethodPost, "/api/v1/yt/channels", map[string]any{"channel_id": "UCtesttesttesttesttest00"}); body["id"] == nil {
		t.Fatalf("add channel failed: %v", body)
	}

	code, g := ytCall(t, h, http.MethodPost, "/api/v1/yt/groups", map[string]any{"name": "学习"})
	if code != 201 || g["name"] != "学习" {
		t.Fatalf("create group: code=%d body=%v", code, g)
	}
	if code, _ = ytCall(t, h, http.MethodPost, "/api/v1/yt/groups", map[string]any{"name": " "}); code != 422 {
		t.Fatalf("empty group name: code=%d, want 422", code)
	}
	code, body := ytCall(t, h, http.MethodGet, "/api/v1/yt/groups", nil)
	if groups, _ := body["groups"].([]any); code != 200 || len(groups) != 2 {
		t.Fatalf("groups: code=%d body=%v, want default + new", code, body)
	}
	if code, body = ytCall(t, h, http.MethodDelete, "/api/v1/yt/groups/other", nil); code != 422 || body["error"] != "GROUP_FIXED" {
		t.Fatalf("delete default group: code=%d body=%v", code, body)
	}
	if code, _ = ytCall(t, h, http.MethodDelete, "/api/v1/yt/groups/"+g["id"].(string), nil); code != 200 {
		t.Fatalf("delete group: code=%d", code)
	}
	if code, _ = ytCall(t, h, http.MethodDelete, "/api/v1/yt/groups/nope", nil); code != 404 {
		t.Fatalf("delete missing group: code=%d, want 404", code)
	}

	if code, _ = ytCall(t, h, http.MethodPatch, "/api/v1/yt/videos/vid000000001", map[string]any{"watch_later": true, "watched": true}); code != 200 {
		t.Fatalf("patch state: code=%d", code)
	}
	code, body = ytCall(t, h, http.MethodGet, "/api/v1/yt/videos?watch_later=1", nil)
	videos, _ := body["videos"].([]any)
	if code != 200 || len(videos) != 1 || videos[0].(map[string]any)["watched_at"] == "" {
		t.Fatalf("watch later list: code=%d body=%v", code, body)
	}
	if code, _ = ytCall(t, h, http.MethodPatch, "/api/v1/yt/videos/nope", map[string]any{"watched": true}); code != 404 {
		t.Fatalf("patch unknown video: code=%d, want 404", code)
	}
	if code, _ = ytCall(t, h, http.MethodPatch, "/api/v1/yt/videos/vid000000001", map[string]any{}); code != 422 {
		t.Fatalf("patch empty body: code=%d, want 422", code)
	}
}

func TestYTSummaryGuards(t *testing.T) {
	s := testYTServerWithFakeBin(t)
	h := ytMux(s)
	if _, body := ytCall(t, h, http.MethodPost, "/api/v1/yt/channels", map[string]any{"channel_id": "UCtesttesttesttesttest00"}); body["id"] == nil {
		t.Fatalf("add channel failed: %v", body)
	}
	for _, q := range []string{
		"INSERT INTO yt_videos(id,channel_id,kind,title,created_at) VALUES('PLlist000001','UCtesttesttesttesttest00','playlist','A playlist',NOW())",
		"INSERT INTO yt_videos(id,channel_id,kind,title,live_status,created_at) VALUES('live00000001','UCtesttesttesttesttest00','live','Live now','is_live',NOW())",
	} {
		if _, err := s.st.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	code, body := ytCall(t, h, http.MethodGet, "/api/v1/yt/videos/vid000000001/summary", nil)
	if sum, _ := body["summary"].(map[string]any); code != 200 || sum["status"] != "none" || sum["lang"] != "zh-CN" {
		t.Fatalf("summary none: code=%d body=%v", code, body)
	}
	if code, _ = ytCall(t, h, http.MethodGet, "/api/v1/yt/videos/vid000000001/summary?lang=fr", nil); code != 422 {
		t.Fatalf("bad lang: code=%d, want 422", code)
	}
	if code, _ = ytCall(t, h, http.MethodGet, "/api/v1/yt/videos/nope/summary", nil); code != 404 {
		t.Fatalf("unknown video: code=%d, want 404", code)
	}
	if code, body = ytCall(t, h, http.MethodPost, "/api/v1/yt/videos/PLlist000001/summary", nil); code != 422 || body["error"] != "NOT_SUPPORTED" {
		t.Fatalf("playlist: code=%d body=%v, want 422 NOT_SUPPORTED", code, body)
	}
	if code, body = ytCall(t, h, http.MethodPost, "/api/v1/yt/videos/live00000001/summary", nil); code != 422 || body["error"] != "NOT_SUPPORTED" {
		t.Fatalf("live: code=%d body=%v, want 422 NOT_SUPPORTED", code, body)
	}
	if code, body = ytCall(t, h, http.MethodPost, "/api/v1/yt/videos/vid000000001/summary", nil); code != 503 || body["error"] != "AI_NOT_CONFIGURED" {
		t.Fatalf("no AI: code=%d body=%v, want 503", code, body)
	}
	if code, _ = ytCall(t, h, http.MethodPost, "/api/v1/yt/channels/ytc_nope/sync", nil); code != 404 {
		t.Fatalf("sync missing channel: code=%d, want 404", code)
	}
}
