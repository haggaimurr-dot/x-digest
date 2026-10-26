package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- Canned yt-dlp output (copied from real runs, IDs/titles kept) ----
//
// Flat entries (--flat-playlist --dump-json): one JSON object per line,
// no upload_date, no description. Metadata (--dump-json --no-playlist):
// a single JSON object with description + upload_date (YYYYMMDD).

const flatVideo1 = `{"_type": "url", "ie_key": "Youtube", "id": "vid000000001", "url": "https://www.youtube.com/watch?v=vid000000001", "title": "First video title", "description": null, "duration": 372, "live_status": "not_live", "channel_id": "UC_x5XG1OV2P6uZZ5FSM9Ttw", "channel": "Google for Developers", "playlist_title": "Google for Developers - Videos", "webpage_url": "https://www.youtube.com/watch?v=vid000000001", "thumbnails": [{"url": "https://i.ytimg.com/vi/vid000000001/hqdefault.jpg", "width": 480, "height": 360}, {"url": "https://i.ytimg.com/vi/vid000000001/maxresdefault.jpg", "width": 1280, "height": 720}]}`
const flatVideo2 = `{"_type": "url", "ie_key": "Youtube", "id": "vid000000002", "url": "https://www.youtube.com/watch?v=vid000000002", "title": "Second video, bare entry", "channel_id": "UC_x5XG1OV2P6uZZ5FSM9Ttw", "playlist_title": "Google for Developers - Videos", "webpage_url": "https://www.youtube.com/watch?v=vid000000002"}`
const flatShort1 = `{"_type": "url", "ie_key": "Youtube", "id": "short00000001", "url": "https://www.youtube.com/shorts/short00000001", "title": "A short", "channel_id": "UC_x5XG1OV2P6uZZ5FSM9Ttw", "playlist_title": "Google for Developers - Shorts", "webpage_url": "https://www.youtube.com/shorts/short00000001", "thumbnails": [{"url": "https://i.ytimg.com/vi/short00000001/hqdefault.jpg"}]}`
const flatLive1 = `{"_type": "url", "ie_key": "Youtube", "id": "live000000001", "url": "https://www.youtube.com/watch?v=live000000001", "title": "Past live stream", "duration": 5400, "live_status": "was_live", "channel_id": "UC_x5XG1OV2P6uZZ5FSM9Ttw", "playlist_title": "Google for Developers - Live", "webpage_url": "https://www.youtube.com/watch?v=live000000001", "thumbnails": [{"url": "https://i.ytimg.com/vi/live000000001/hqdefault.jpg"}]}`
const flatPlaylist1 = `{"_type": "url", "ie_key": "Youtube", "id": "PL000000000001", "url": "https://www.youtube.com/playlist?list=PL000000000001", "title": "Getting started", "channel_id": "UC_x5XG1OV2P6uZZ5FSM9Ttw", "playlist_title": "Google for Developers - Playlists", "webpage_url": "https://www.youtube.com/playlist?list=PL000000000001", "thumbnails": [{"url": "https://i.ytimg.com/vi/PL000000000001/hqdefault.jpg"}]}`

func metaJSON(id, uploadDate, description string) string {
	return fmt.Sprintf(`{"id": %q, "title": "title for %s", "description": %q, "upload_date": %q, "duration": 372, "live_status": "not_live", "channel": "Google for Developers", "webpage_url": "https://www.youtube.com/watch?v=%s"}`,
		id, id, description, uploadDate, id)
}

// fakeYtDlp is an injectable Runner: canned flat listings per tab slug,
// canned metadata per item ID. A missing/empty metadata entry fails the
// call (simulating a 429); a slug in flatFails fails that tab listing.
type fakeYtDlp struct {
	flat      map[string]string
	meta      map[string]string
	flatFails map[string]bool
	metaCalls []string
	flatCalls []string
}

func (f *fakeYtDlp) run(_ context.Context, args []string) ([]byte, error) {
	last := args[len(args)-1]
	isFlat := false
	for _, a := range args {
		if a == "--flat-playlist" {
			isFlat = true
		}
	}
	if isFlat {
		slug := last[strings.LastIndex(last, "/")+1:]
		f.flatCalls = append(f.flatCalls, slug)
		if f.flatFails[slug] {
			return nil, errors.New("exit status 1")
		}
		return []byte(f.flat[slug]), nil
	}
	id := itemIDFromURL(last)
	f.metaCalls = append(f.metaCalls, id)
	if j, ok := f.meta[id]; ok && j != "" {
		return []byte(j), nil
	}
	return nil, errors.New("ERROR: HTTP Error 429: Too Many Requests")
}

func itemIDFromURL(u string) string {
	if i := strings.Index(u, "v="); i >= 0 {
		rest := u[i+2:]
		if j := strings.Index(rest, "&"); j >= 0 {
			rest = rest[:j]
		}
		return rest
	}
	if i := strings.Index(u, "/shorts/"); i >= 0 {
		return u[i+len("/shorts/"):]
	}
	if i := strings.Index(u, "list="); i >= 0 {
		rest := u[i+len("list="):]
		if j := strings.Index(rest, "&"); j >= 0 {
			rest = rest[:j]
		}
		return rest
	}
	return u
}

func allTabsFake() *fakeYtDlp {
	return &fakeYtDlp{
		flat: map[string]string{
			"videos":    flatVideo1 + "\n" + flatVideo2 + "\n",
			"shorts":    flatShort1 + "\n",
			"streams":   flatLive1 + "\n",
			"playlists": flatPlaylist1 + "\n",
		},
		meta: map[string]string{
			"vid000000001":   metaJSON("vid000000001", "20261024", "First video description here."),
			"vid000000002":   metaJSON("vid000000002", "20261022", "Second video description."),
			"short00000001":  metaJSON("short00000001", "20261023", "Short description."),
			"live000000001":  metaJSON("live000000001", "20261021", "Live description."),
			"PL000000000001": `{"id": "PL000000000001", "title": "Getting started", "description": "Playlist description here.", "channel": "Google for Developers", "webpage_url": "https://www.youtube.com/playlist?list=PL000000000001"}`,
		},
	}
}

func testClient(f *fakeYtDlp) *Client {
	return &Client{
		HTTP:          http.DefaultClient,
		BaseURL:       defaultBaseURL,
		Runner:        f.run,
		BackfillDelay: 0,
	}
}

func TestParseFlatEntry(t *testing.T) {
	e, err := parseFlatEntry([]byte(flatVideo1))
	if err != nil {
		t.Fatalf("parseFlatEntry: %v", err)
	}
	if e.id != "vid000000001" || e.title != "First video title" {
		t.Fatalf("entry = %+v", e)
	}
	if e.duration != 372 || e.liveStatus != "not_live" {
		t.Fatalf("duration/live_status = %d/%q", e.duration, e.liveStatus)
	}
	// First thumbnail wins.
	if e.thumbnailURL != "https://i.ytimg.com/vi/vid000000001/hqdefault.jpg" {
		t.Fatalf("thumbnail = %q", e.thumbnailURL)
	}
	// Canonical URL comes from webpage_url.
	if e.url != "https://www.youtube.com/watch?v=vid000000001" {
		t.Fatalf("url = %q", e.url)
	}
	if e.playlistTitle != "Google for Developers - Videos" {
		t.Fatalf("playlist_title = %q", e.playlistTitle)
	}
	// Bare entry: missing duration/live_status/thumbnails stay zero.
	e2, err := parseFlatEntry([]byte(flatVideo2))
	if err != nil {
		t.Fatalf("parseFlatEntry bare: %v", err)
	}
	if e2.duration != 0 || e2.liveStatus != "" || e2.thumbnailURL != "" {
		t.Fatalf("bare entry = %+v", e2)
	}
	// Short URL is a /shorts/ URL, not watch?v=.
	e3, err := parseFlatEntry([]byte(flatShort1))
	if err != nil {
		t.Fatalf("parseFlatEntry short: %v", err)
	}
	if e3.url != "https://www.youtube.com/shorts/short00000001" {
		t.Fatalf("short url = %q", e3.url)
	}
	// Missing id / bad JSON are errors.
	if _, err := parseFlatEntry([]byte(`{"title":"no id"}`)); err == nil {
		t.Fatal("expected error for entry without id")
	}
	if _, err := parseFlatEntry([]byte(`not json`)); err == nil {
		t.Fatal("expected error for non-JSON line")
	}
}

func TestChannelNameFromPlaylistTitle(t *testing.T) {
	cases := []struct{ title, tab, want string }{
		{"Google for Developers - Videos", "videos", "Google for Developers"},
		{"Google for Developers - Shorts", "shorts", "Google for Developers"},
		{"Google for Developers - Live", "streams", "Google for Developers"},
		{"Google for Developers - Playlists", "playlists", "Google for Developers"},
		{"No Suffix Here", "videos", "No Suffix Here"},
		{"", "videos", ""},
		// A channel name containing " - " itself keeps its own dashes.
		{"A - B Channel - Videos", "videos", "A - B Channel"},
	}
	for _, tc := range cases {
		if got := channelNameFromPlaylistTitle(tc.title, tc.tab); got != tc.want {
			t.Errorf("channelNameFromPlaylistTitle(%q, %q) = %q, want %q", tc.title, tc.tab, got, tc.want)
		}
	}
}

func TestParseMeta(t *testing.T) {
	m, err := parseMeta([]byte(metaJSON("vid000000001", "20261024", "Desc here.")))
	if err != nil {
		t.Fatalf("parseMeta: %v", err)
	}
	if m.Description != "Desc here." || m.uploadDate() != "2026-10-24" {
		t.Fatalf("meta = %+v", m)
	}
	if m.publishedAt() != "2026-10-24 00:00:00" {
		t.Fatalf("publishedAt = %q", m.publishedAt())
	}
	if m.durationSecs() != 372 {
		t.Fatalf("duration = %d", m.durationSecs())
	}
	// No upload_date (e.g. a playlist) → no date, not an error.
	m2, err := parseMeta([]byte(`{"id":"PL000000000001","description":"P desc."}`))
	if err != nil {
		t.Fatalf("parseMeta playlist: %v", err)
	}
	if m2.uploadDate() != "" || m2.publishedAt() != "" || m2.durationSecs() != 0 {
		t.Fatalf("playlist meta = %+v", m2)
	}
	if _, err := parseMeta([]byte(`not json`)); err == nil {
		t.Fatal("expected error for non-JSON metadata")
	}
}

func TestInDateRange(t *testing.T) {
	cases := []struct {
		date, from, to string
		want           bool
	}{
		{"2026-10-24", "", "", true},
		{"2026-10-24", "2026-10-01", "2026-10-31", true},
		{"2026-10-24", "2026-10-24", "2026-10-24", true}, // to is inclusive
		{"2026-10-23", "2026-10-24", "", false},
		{"2026-11-01", "", "2026-10-31", false},
		{"", "2026-10-01", "2026-10-31", false}, // no date → not stored yet
	}
	for _, tc := range cases {
		if got := inDateRange(tc.date, tc.from, tc.to); got != tc.want {
			t.Errorf("inDateRange(%q, %q, %q) = %v, want %v", tc.date, tc.from, tc.to, got, tc.want)
		}
	}
}

func TestSyncTabsAllTabs(t *testing.T) {
	f := allTabsFake()
	c := testClient(f)
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "", "", nil)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if res.Status != "synced" || res.Error != "" {
		t.Fatalf("res status = %q err = %q", res.Status, res.Error)
	}
	if res.SyncedTabs != 4 || len(res.FailedTabs) != 0 {
		t.Fatalf("tabs: synced=%d failed=%v", res.SyncedTabs, res.FailedTabs)
	}
	if res.ChannelName != "Google for Developers" {
		t.Fatalf("channel name = %q", res.ChannelName)
	}
	if len(res.Videos) != 5 {
		t.Fatalf("got %d videos, want 5 (2 videos + short + live + playlist)", len(res.Videos))
	}
	kinds := map[string]string{}
	for _, v := range res.Videos {
		kinds[v.ID] = v.Kind
		if v.ID == "vid000000001" {
			if v.Description != "First video description here." || v.PublishedAt != "2026-10-24 00:00:00" || v.Duration != 372 || v.LiveStatus != "not_live" {
				t.Fatalf("video1 = %+v", v)
			}
			if v.ThumbnailURL != "https://i.ytimg.com/vi/vid000000001/hqdefault.jpg" {
				t.Fatalf("video1 thumbnail = %q", v.ThumbnailURL)
			}
		}
		if v.ID == "PL000000000001" {
			if v.Description != "Playlist description here." || v.PublishedAt != "" {
				t.Fatalf("playlist = %+v", v)
			}
			if v.URL != "https://www.youtube.com/playlist?list=PL000000000001" {
				t.Fatalf("playlist url = %q", v.URL)
			}
		}
	}
	wantKinds := map[string]string{
		"vid000000001": "video", "vid000000002": "video",
		"short00000001": "short", "live000000001": "live",
		"PL000000000001": "playlist",
	}
	for id, want := range wantKinds {
		if kinds[id] != want {
			t.Errorf("kind[%s] = %q, want %q", id, kinds[id], want)
		}
	}
}

// A 429 (or any metadata failure) never fails the sync: the affected
// video/short/live items are simply not stored yet, and playlists are
// still stored from the flat listing.
func TestSyncTabsBackfillFailureTolerated(t *testing.T) {
	f := allTabsFake()
	f.meta = map[string]string{} // every metadata call fails with 429
	c := testClient(f)
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "", "", nil)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if res.Status != "synced" {
		t.Fatalf("status = %q, want synced (backfill must not fail the sync)", res.Status)
	}
	if len(res.Videos) != 1 || res.Videos[0].ID != "PL000000000001" || res.Videos[0].Kind != "playlist" {
		t.Fatalf("videos = %+v, want only the playlist", res.Videos)
	}
	if res.Videos[0].Description != "" {
		t.Fatalf("playlist description should stay empty when its backfill fails, got %q", res.Videos[0].Description)
	}
}

func TestSyncTabsDateRangeSkip(t *testing.T) {
	f := allTabsFake()
	// vid1 2026-10-24 (in range), vid2 2026-10-22 (before), short
	// 2026-10-23 (before), live 2026-10-21 (before).
	c := testClient(f)
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "2026-10-24", "2026-10-24", nil)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if res.Status != "synced" {
		t.Fatalf("status = %q", res.Status)
	}
	got := map[string]bool{}
	for _, v := range res.Videos {
		got[v.ID] = true
	}
	// Only the in-range video and the (dateless, never filtered) playlist.
	if len(res.Videos) != 2 || !got["vid000000001"] || !got["PL000000000001"] {
		t.Fatalf("videos = %+v, want only vid000000001 + playlist", res.Videos)
	}
}

func TestSyncTabsPartialTabFailure(t *testing.T) {
	f := allTabsFake()
	f.flatFails = map[string]bool{"shorts": true}
	c := testClient(f)
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "", "", nil)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if res.Status != "synced" || res.SyncedTabs != 3 {
		t.Fatalf("status = %q syncedTabs = %d, want synced/3", res.Status, res.SyncedTabs)
	}
	if len(res.FailedTabs) != 1 || res.FailedTabs[0] != "shorts" {
		t.Fatalf("failedTabs = %v, want [shorts]", res.FailedTabs)
	}
	if res.Error != "" {
		t.Fatalf("Error should stay empty on partial success, got %q", res.Error)
	}
	for _, v := range res.Videos {
		if v.Kind == "short" {
			t.Fatalf("short tab failed, but a short was returned: %+v", v)
		}
	}
	if len(res.Videos) != 4 {
		t.Fatalf("got %d videos, want 4 (all but the short)", len(res.Videos))
	}
}

func TestSyncTabsAllTabsFail(t *testing.T) {
	f := allTabsFake()
	f.flatFails = map[string]bool{"videos": true, "shorts": true, "streams": true, "playlists": true}
	c := testClient(f)
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "", "", nil)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if res.Status != "failed" || res.Error == "" || res.Videos != nil {
		t.Fatalf("all tabs failed should fold into Status, got %+v", res)
	}
	if res.SyncedTabs != 0 || len(res.FailedTabs) != 4 {
		t.Fatalf("syncedTabs=%d failedTabs=%v", res.SyncedTabs, res.FailedTabs)
	}
}

// Known (already stored, complete) IDs are skipped entirely: no
// metadata call, no row in the result.
func TestSyncTabsKnownIDsSkipped(t *testing.T) {
	f := allTabsFake()
	c := testClient(f)
	known := map[string]bool{"vid000000001": true, "PL000000000001": true}
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "", "", known)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if len(res.Videos) != 3 {
		t.Fatalf("got %d videos, want 3 (known 2 skipped)", len(res.Videos))
	}
	for _, id := range f.metaCalls {
		if id == "vid000000001" || id == "PL000000000001" {
			t.Fatalf("metadata fetched for known id %s: %v", id, f.metaCalls)
		}
	}
}

// Backfill is capped at 10 items per tab: a 12-video flat listing only
// fetches metadata for the first 10.
func TestSyncTabsBackfillCap(t *testing.T) {
	var lines []string
	meta := map[string]string{}
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("capvideo%06d", i)
		lines = append(lines, fmt.Sprintf(`{"id": %q, "title": "cap %d", "playlist_title": "Cap Channel - Videos", "webpage_url": "https://www.youtube.com/watch?v=%s"}`, id, i, id))
		meta[id] = metaJSON(id, "20261024", "d")
	}
	f := &fakeYtDlp{
		flat: map[string]string{
			"videos":    strings.Join(lines, "\n") + "\n",
			"shorts":    "",
			"streams":   "",
			"playlists": "",
		},
		meta: meta,
	}
	c := testClient(f)
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "", "", nil)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if len(f.metaCalls) != 10 {
		t.Fatalf("metadata calls = %d, want 10 (cap)", len(f.metaCalls))
	}
	if len(res.Videos) != 10 {
		t.Fatalf("videos = %d, want 10", len(res.Videos))
	}
}

// A non-zero yt-dlp exit with partial stdout keeps the lines that
// parsed: listTab returns the one good entry instead of failing.
func TestListTabPartialOutputKept(t *testing.T) {
	f := &fakeYtDlp{flat: map[string]string{}}
	c := testClient(f)
	c.Runner = func(_ context.Context, args []string) ([]byte, error) {
		return []byte(flatVideo1 + "\n"), errors.New("exit status 1")
	}
	entries, name, err := c.listTab(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", ytTab{slug: "videos", kind: "video"})
	if err != nil {
		t.Fatalf("listTab with partial output: %v", err)
	}
	if len(entries) != 1 || entries[0].id != "vid000000001" {
		t.Fatalf("entries = %+v", entries)
	}
	if name != "Google for Developers" {
		t.Fatalf("name = %q", name)
	}
	// Zero good lines + non-zero exit is a real tab failure.
	c.Runner = func(_ context.Context, args []string) ([]byte, error) {
		return []byte("garbage line\n"), errors.New("exit status 1")
	}
	if _, _, err := c.listTab(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", ytTab{slug: "videos", kind: "video"}); err == nil {
		t.Fatal("expected error when no lines parse and yt-dlp failed")
	}
}

// A fake yt-dlp binary (via YT_DLP_BIN) that always 429s: every tab
// fails and the sync folds into Status=failed.
func TestSyncTabsFakeBinary429(t *testing.T) {
	script := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'ERROR: HTTP Error 429: Too Many Requests' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	t.Setenv("YT_DLP_BIN", script)
	c := &Client{HTTP: http.DefaultClient, BaseURL: defaultBaseURL, BackfillDelay: 0}
	res, err := c.SyncTabs(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw", "", "", nil)
	if err != nil {
		t.Fatalf("SyncTabs: %v", err)
	}
	if res.Status != "failed" || res.Videos != nil {
		t.Fatalf("res = %+v, want failed with nil videos", res)
	}
}

func TestIsChannelID(t *testing.T) {
	valid := []string{"UC_x5XG1OV2P6uZZ5FSM9Ttw", "UC" + strings.Repeat("a", 22)}
	for _, s := range valid {
		if !IsChannelID(s) {
			t.Errorf("IsChannelID(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "UC_x5XG1OV2P6uZZ5FSM9Tt", "UC_x5XG1OV2P6uZZ5FSM9TtwX", "@handle", "https://www.youtube.com/@handle", "UC_x5XG1OV2P6uZZ5FSM9Tt!"}
	for _, s := range invalid {
		if IsChannelID(s) {
			t.Errorf("IsChannelID(%q) = true, want false", s)
		}
	}
}

func TestResolveChannelIDDirectAndChannelURL(t *testing.T) {
	c := New()
	ctx := context.Background()
	// Direct ID passes through untouched (no HTTP).
	id, err := c.ResolveChannelID(ctx, "UC_x5XG1OV2P6uZZ5FSM9Ttw")
	if err != nil || id != "UC_x5XG1OV2P6uZZ5FSM9Ttw" {
		t.Fatalf("direct: id=%q err=%v", id, err)
	}
	// /channel/UC... URL extracts the ID without a fetch.
	id, err = c.ResolveChannelID(ctx, "https://www.youtube.com/channel/UC_x5XG1OV2P6uZZ5FSM9Ttw")
	if err != nil || id != "UC_x5XG1OV2P6uZZ5FSM9Ttw" {
		t.Fatalf("channel URL: id=%q err=%v", id, err)
	}
	// Empty input is a clean error (the handler maps it to 422).
	if _, err := c.ResolveChannelID(ctx, "  "); err == nil {
		t.Fatal("expected error for empty input")
	}
}

// resolveTestServer serves a canned channel page containing channelId.
func resolveTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "Mozilla") {
			t.Errorf("expected browser User-Agent, got %q", ua)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
}

func TestResolveChannelIDFromHandlePage(t *testing.T) {
	srv := resolveTestServer(t, `<html><body>{"channelId":"UC_x5XG1OV2P6uZZ5FSM9Ttw","other":"x"}</body></html>`)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	id, err := c.ResolveChannelID(context.Background(), "@somehandle")
	if err != nil {
		t.Fatalf("resolve @handle: %v", err)
	}
	if id != "UC_x5XG1OV2P6uZZ5FSM9Ttw" {
		t.Fatalf("id = %q", id)
	}
}

func TestResolveChannelIDFromURLPage(t *testing.T) {
	srv := resolveTestServer(t, `<html><head><link rel="canonical" href="https://www.youtube.com/@somehandle"></head><body>... "channelId":"UC_x5XG1OV2P6uZZ5FSM9Ttw" ...</body></html>`)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	id, err := c.ResolveChannelID(context.Background(), srv.URL+"/@somehandle")
	if err != nil {
		t.Fatalf("resolve URL: %v", err)
	}
	if id != "UC_x5XG1OV2P6uZZ5FSM9Ttw" {
		t.Fatalf("id = %q", id)
	}
}

func TestResolveChannelIDNoMatch(t *testing.T) {
	srv := resolveTestServer(t, `<html><body>no channel id here</body></html>`)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	if _, err := c.ResolveChannelID(context.Background(), "@nobody"); err == nil {
		t.Fatal("expected error when the page has no channelId")
	} else if !strings.Contains(err.Error(), "UC") {
		t.Fatalf("error should ask for the UC ID, got: %v", err)
	}
}

func TestResolveChannelIDNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	if _, err := c.ResolveChannelID(context.Background(), "@gone"); err == nil {
		t.Fatal("expected error on non-200 channel page")
	}
}

// New() honours YT_HTTP_PROXY: the channel-page fetch must travel
// through the proxy (the Shanghai box reaches YouTube only via its
// local xray client). The test uses an http:// BaseURL so the fake
// proxy sees a plain GET with the absolute page URI; production uses
// https:// (CONNECT), same wiring.
func TestNewUsesProxyEnv(t *testing.T) {
	var gotURI string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>{"channelId":"UC_x5XG1OV2P6uZZ5FSM9Ttw"}</body></html>`))
	}))
	defer proxy.Close()
	t.Setenv("YT_HTTP_PROXY", proxy.URL)
	c := New()
	c.BaseURL = "http://www.youtube.com"
	id, err := c.ResolveChannelID(context.Background(), "@somehandle")
	if err != nil {
		t.Fatalf("resolve via proxy: %v", err)
	}
	if id != "UC_x5XG1OV2P6uZZ5FSM9Ttw" {
		t.Fatalf("id = %q", id)
	}
	if !strings.HasPrefix(gotURI, "http://www.youtube.com/@somehandle") {
		t.Fatalf("proxy saw RequestURI %q, want the absolute channel page URL", gotURI)
	}
}
