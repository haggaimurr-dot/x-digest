package main

import (
	"context"
	"net/http"
	"strings"

	"xdigest/internal/youtube"
)

// YouTube on the video hub (2026-10-29): /api/v1/yt/* is served by
// videoHub with this adapter; the routes and JSON are the ones the
// YouTube-only handlers had (pinned by yt_test.go / yt_routes_test.go).
// Syncs run yt-dlp (no API key); see internal/youtube.

type ytPlatform struct{ c *youtube.Client }

// Resolve turns a UC… ID, channel URL or @handle into the channel ID. The
// name comes from the tab listing on the first sync.
func (p ytPlatform) Resolve(ctx context.Context, input string) (string, string, error) {
	id, err := p.c.ResolveChannelID(ctx, input)
	return id, "", err
}

// Sync lists the four channel tabs. A tab that failed while others listed
// is not reported as a sync error (channels without shorts or lives fail
// those tabs on every sync); all tabs failing is.
func (p ytPlatform) Sync(ctx context.Context, channelID, dateFrom, dateTo string, known map[string]bool) (*youtube.SyncResult, error) {
	res, err := p.c.SyncTabs(ctx, channelID, dateFrom, dateTo, known)
	if res != nil {
		res.FailedTabs = nil
	}
	return res, err
}

// FetchDetail reads subtitles and metadata of one video for its summary.
func (p ytPlatform) FetchDetail(ctx context.Context, videoID string) (*youtube.VideoDetail, error) {
	return p.c.FetchDetail(ctx, "https://www.youtube.com/watch?v="+videoID)
}

// FetchVideo reads one video by URL (add-by-URL, single-mode channels).
func (p ytPlatform) FetchVideo(ctx context.Context, input string) (*youtube.SingleVideo, error) {
	return p.c.FetchVideo(ctx, input)
}

// ytHandle keeps the @handle the user gave (the "handle" field, or an
// @name typed as the input); otherwise the handle stays empty.
func ytHandle(input, given, _ string) string {
	if given != "" {
		return given
	}
	if strings.HasPrefix(input, "@") {
		return input
	}
	return ""
}

// registerYT serves /api/v1/yt/* (the handler tests go through it too).
func (s *server) registerYT(mux *http.ServeMux) {
	(&videoHub{prefix: "yt", idPrefix: "ytc_",
		kinds: map[string]bool{"video": true, "short": true, "live": true, "playlist": true},
		st:    s.st.Hub("yt"), pf: ytPlatform{s.yt}, sum: s.sum,
		playlistName: "播放列表", handle: ytHandle}).register(mux)
}
