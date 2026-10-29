package main

// YouTube handler tests run against a scratch MariaDB database (same
// XDIGEST_TEST_DSN convention as internal/store). Channel-page resolves
// run against httptest servers and yt-dlp runs against a fake binary
// (YT_DLP_BIN), never real YouTube.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/store"
	"xdigest/internal/youtube"
)

// fakeYtDlpScript writes a shell script that emulates yt-dlp for the
// handler tests and returns its path. The videos tab lists one video
// (vid000000001, "Handler Test Channel - Videos"); the other tabs are
// empty. A metadata call for that video returns a description and an
// upload_date of 20261024. A metadata call for single00001 returns a video
// of another channel (UCsinglesinglesinglesi00, @singlechan), for the
// add-by-URL tests; any other metadata call fails (exit 1).
func fakeYtDlpScript(t *testing.T) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "yt-dlp")
	body := `#!/bin/sh
flat=0
url=""
for a in "$@"; do
  [ "$a" = "--flat-playlist" ] && flat=1
  url="$a"
done
if [ "$flat" = 1 ]; then
  case "$url" in
    */videos) printf '%s\n' '{"id":"vid000000001","title":"Handler test video","duration":60,"live_status":"not_live","playlist_title":"Handler Test Channel - Videos","webpage_url":"https://www.youtube.com/watch?v=vid000000001","thumbnails":[{"url":"https://i.ytimg.com/vi/vid000000001/hqdefault.jpg"}]}' ;;
    *) : ;;
  esac
  exit 0
fi
case "$url" in
  *vid000000001*) printf '%s\n' '{"id":"vid000000001","title":"Handler test video","description":"A video for handler tests.","upload_date":"20261024","duration":60,"live_status":"not_live"}' ;;
  *single00001*) printf '%s\n' '{"id":"single00001","title":"Single video","description":"Added by URL.","upload_date":"20261020","duration":30,"live_status":"not_live","channel_id":"UCsinglesinglesinglesi00","channel":"Single Chan","uploader_id":"@singlechan","webpage_url":"https://www.youtube.com/watch?v=single00001"}' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake yt-dlp: %v", err)
	}
	return script
}

func testYTServer(t *testing.T, yt *youtube.Client) *server {
	t.Helper()
	serverDSN := os.Getenv("XDIGEST_TEST_DSN")
	if serverDSN == "" {
		// CI sets XDIGEST_REQUIRE_DB so a lost DSN fails instead of silently skipping.
		if os.Getenv("XDIGEST_REQUIRE_DB") != "" {
			t.Fatal("XDIGEST_REQUIRE_DB is set but XDIGEST_TEST_DSN is empty")
		}
		t.Skip("XDIGEST_TEST_DSN not set; skipping DB-backed YouTube handler tests")
	}
	srv, err := sql.Open("mysql", serverDSN)
	if err != nil {
		t.Fatalf("open server DSN: %v", err)
	}
	defer srv.Close()
	// Own scratch DB per package: go test runs packages in parallel.
	if _, err := srv.Exec("DROP DATABASE IF EXISTS xdigest_test_api"); err != nil {
		t.Fatalf("drop scratch DB: %v", err)
	}
	if _, err := srv.Exec("CREATE DATABASE xdigest_test_api CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create scratch DB: %v", err)
	}
	dbDSN := strings.Replace(serverDSN, "/?", "/xdigest_test_api?", 1)
	st, err := store.New(dbDSN)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	schema, err := os.ReadFile("../../schema.sql")
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	// schema.sql starts with CREATE DATABASE / USE xdigest (for the
	// `mysql < schema.sql` deploy path). Strip those two statements so
	// the tables land in the scratch DB, not a literal `xdigest` DB.
	var kept []string
	for _, line := range strings.Split(string(schema), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CREATE DATABASE") ||
			trimmed == "CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" ||
			strings.HasPrefix(trimmed, "USE xdigest") {
			continue
		}
		kept = append(kept, line)
	}
	if _, err := st.DB.Exec(strings.Join(kept, "\n")); err != nil {
		t.Fatalf("apply schema.sql: %v", err)
	}
	t.Setenv("TRANSLATE_API_KEY", "") // summaries answer 503, never a real model call
	return &server{st: st, yt: yt, sum: newSummaryJobs(yt)}
}

// testYTServerWithFakeBin is testYTServer with syncs driven by the fake
// yt-dlp binary (YT_DLP_BIN). The client reads the binary path from the
// environment on every call, so t.Setenv is enough.
func testYTServerWithFakeBin(t *testing.T) *server {
	t.Helper()
	t.Setenv("YT_DLP_BIN", fakeYtDlpScript(t))
	return testYTServer(t, &youtube.Client{HTTP: http.DefaultClient, BaseURL: "https://www.youtube.com"})
}

// ytMux routes /api/v1/yt/* like the API does, so the tests pin the URLs
// and JSON rather than handler functions.
func ytMux(s *server) http.Handler {
	mux := http.NewServeMux()
	s.registerYT(mux)
	return mux
}

// ytCall sends one request through h and decodes the JSON body.
func ytCall(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var decoded map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}
	return rec.Code, decoded
}

func TestYTAddChannelValidation422(t *testing.T) {
	// Resolve runs against a page with no channelId, so no real YouTube.
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body>no channel id here</body></html>"))
	}))
	defer page.Close()
	s := testYTServer(t, &youtube.Client{HTTP: page.Client(), BaseURL: page.URL})

	// Empty input.
	code, body := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels", map[string]any{})
	if code != 422 {
		t.Fatalf("empty input: code=%d body=%v, want 422", code, body)
	}
	// Unresolvable handle: the error must ask for the UC... ID.
	code, body = ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels", map[string]any{"handle": "@nobody"})
	if code != 422 {
		t.Fatalf("@nobody: code=%d body=%v, want 422", code, body)
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "UC") {
		t.Fatalf("422 message should ask for the UC ID, got %q", msg)
	}
	// Unknown group is also a 422.
	code, body = ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels", map[string]any{"channel_id": "UCtesttesttesttesttest00", "group_id": "no-such-group"})
	if code != 422 {
		t.Fatalf("unknown group: code=%d body=%v, want 422", code, body)
	}
	// Bad date_from format is a 422.
	code, body = ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels", map[string]any{"channel_id": "UCtesttesttesttesttest00", "date_from": "24/10/2026"})
	if code != 422 {
		t.Fatalf("bad date_from: code=%d body=%v, want 422", code, body)
	}
	// date_from after date_to is a 422.
	code, body = ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels", map[string]any{"channel_id": "UCtesttesttesttesttest00", "date_from": "2026-10-25", "date_to": "2026-10-24"})
	if code != 422 {
		t.Fatalf("from>to: code=%d body=%v, want 422", code, body)
	}
}

func TestYTAddChannelSyncsOnceThenDuplicate409(t *testing.T) {
	s := testYTServerWithFakeBin(t)

	code, body := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00", "handle": "@handlertest"})
	if code != 201 {
		t.Fatalf("add: code=%d body=%v, want 201", code, body)
	}
	if body["name"] != "Handler Test Channel" {
		t.Fatalf("name = %v, want tab playlist_title channel name", body["name"])
	}
	if body["sync_status"] != "synced" {
		t.Fatalf("sync_status = %v, want synced", body["sync_status"])
	}
	if body["handle"] != "@handlertest" {
		t.Fatalf("handle = %v", body["handle"])
	}
	// The sync ran once at add time: the video is already listed, with
	// its backfilled description and date.
	code, body = ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/videos", nil)
	if code != 200 {
		t.Fatalf("videos: code=%d body=%v", code, body)
	}
	videos, _ := body["videos"].([]any)
	if len(videos) != 1 {
		t.Fatalf("videos = %v, want 1 video after add", body["videos"])
	}
	v0, _ := videos[0].(map[string]any)
	if v0["kind"] != "video" || v0["description"] != "A video for handler tests." || v0["published_at"] != "2026-10-24 00:00:00" {
		t.Fatalf("video = %v, want kind video + backfilled description/date", v0)
	}
	// Adding the same channel again is a 409.
	code, body = ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	if code != 409 {
		t.Fatalf("duplicate: code=%d body=%v, want 409", code, body)
	}
}

func TestYTAddChannelWithDateRange(t *testing.T) {
	s := testYTServerWithFakeBin(t)

	code, body := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00", "date_from": "2026-10-01", "date_to": "2026-10-31"})
	if code != 201 {
		t.Fatalf("add: code=%d body=%v, want 201", code, body)
	}
	if body["date_from"] != "2026-10-01" || body["date_to"] != "2026-10-31" {
		t.Fatalf("range = %v/%v, want 2026-10-01/2026-10-31", body["date_from"], body["date_to"])
	}
	// The fake video's upload date (2026-10-24) is inside the range.
	code, body = ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/videos", nil)
	if videos, _ := body["videos"].([]any); len(videos) != 1 {
		t.Fatalf("videos in range: code=%d body=%v, want 1", code, body)
	}
}

func TestYTChannelSubHandlers(t *testing.T) {
	s := testYTServerWithFakeBin(t)

	_, body := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("add did not return an id: %v", body)
	}
	// GET one channel.
	code, got := ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/channels/"+id, nil)
	if code != 200 || got["channel_id"] != "UCtesttesttesttesttest00" {
		t.Fatalf("get one: code=%d body=%v", code, got)
	}
	// GET missing channel is a 404.
	code, _ = ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/channels/ytc_nope", nil)
	if code != 404 {
		t.Fatalf("get missing: code=%d, want 404", code)
	}
	// DELETE removes the channel and its videos.
	code, _ = ytCall(t, ytMux(s), http.MethodDelete, "/api/v1/yt/channels/"+id, nil)
	if code != 200 {
		t.Fatalf("delete: code=%d, want 200", code)
	}
	code, body = ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/videos", nil)
	if code != 200 {
		t.Fatalf("videos after delete: code=%d", code)
	}
	if videos, _ := body["videos"].([]any); len(videos) != 0 {
		t.Fatalf("videos should be gone after channel delete, got %v", videos)
	}
	// DELETE again is a 404.
	code, _ = ytCall(t, ytMux(s), http.MethodDelete, "/api/v1/yt/channels/"+id, nil)
	if code != 404 {
		t.Fatalf("delete missing: code=%d, want 404", code)
	}
}

func TestYTPatchChannel(t *testing.T) {
	s := testYTServerWithFakeBin(t)

	_, body := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("add did not return an id: %v", body)
	}
	// Set a range.
	code, got := ytCall(t, ytMux(s), http.MethodPatch, "/api/v1/yt/channels/"+id,
		map[string]any{"date_from": "2026-10-01", "date_to": "2026-10-31"})
	if code != 200 {
		t.Fatalf("patch set range: code=%d body=%v, want 200", code, got)
	}
	if got["date_from"] != "2026-10-01" || got["date_to"] != "2026-10-31" {
		t.Fatalf("range after patch = %v/%v", got["date_from"], got["date_to"])
	}
	// Null clears one bound; the other is untouched.
	code, got = ytCall(t, ytMux(s), http.MethodPatch, "/api/v1/yt/channels/"+id,
		map[string]any{"date_from": nil})
	if code != 200 {
		t.Fatalf("patch clear from: code=%d body=%v, want 200", code, got)
	}
	if _, present := got["date_from"]; present && got["date_from"] != nil {
		t.Fatalf("date_from should be null after clear, got %v", got["date_from"])
	}
	if got["date_to"] != "2026-10-31" {
		t.Fatalf("date_to should be untouched, got %v", got["date_to"])
	}
	// Empty string also clears.
	code, got = ytCall(t, ytMux(s), http.MethodPatch, "/api/v1/yt/channels/"+id,
		map[string]any{"date_to": ""})
	if code != 200 {
		t.Fatalf("patch clear to: code=%d body=%v, want 200", code, got)
	}
	if _, present := got["date_to"]; present && got["date_to"] != nil {
		t.Fatalf("date_to should be null after clear, got %v", got["date_to"])
	}
	// Bad date / from>to / unknown group are 422s.
	code, _ = ytCall(t, ytMux(s), http.MethodPatch, "/api/v1/yt/channels/"+id,
		map[string]any{"date_from": "not-a-date"})
	if code != 422 {
		t.Fatalf("bad date: code=%d, want 422", code)
	}
	code, _ = ytCall(t, ytMux(s), http.MethodPatch, "/api/v1/yt/channels/"+id,
		map[string]any{"date_from": "2026-10-25", "date_to": "2026-10-24"})
	if code != 422 {
		t.Fatalf("from>to: code=%d, want 422", code)
	}
	code, _ = ytCall(t, ytMux(s), http.MethodPatch, "/api/v1/yt/channels/"+id,
		map[string]any{"group_id": "no-such-group"})
	if code != 422 {
		t.Fatalf("unknown group: code=%d, want 422", code)
	}
	// PATCH on a missing channel is a 404.
	code, _ = ytCall(t, ytMux(s), http.MethodPatch, "/api/v1/yt/channels/ytc_nope",
		map[string]any{"date_from": "2026-10-01"})
	if code != 404 {
		t.Fatalf("patch missing: code=%d, want 404", code)
	}
}

func TestYTVideosKindFilter(t *testing.T) {
	s := testYTServerWithFakeBin(t)

	_, body := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	if body["id"] == nil {
		t.Fatalf("add failed: %v", body)
	}
	code, body := ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/videos?kind=video", nil)
	if code != 200 {
		t.Fatalf("kind=video: code=%d body=%v", code, body)
	}
	if videos, _ := body["videos"].([]any); len(videos) != 1 {
		t.Fatalf("kind=video: got %v, want 1", body["videos"])
	}
	code, body = ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/videos?kind=short", nil)
	if code != 200 {
		t.Fatalf("kind=short: code=%d body=%v", code, body)
	}
	if videos, _ := body["videos"].([]any); len(videos) != 0 {
		t.Fatalf("kind=short: got %v, want 0", body["videos"])
	}
	code, body = ytCall(t, ytMux(s), http.MethodGet, "/api/v1/yt/videos?kind=bogus", nil)
	if code != 422 {
		t.Fatalf("kind=bogus: code=%d body=%v, want 422", code, body)
	}
}

func TestYTSyncAll(t *testing.T) {
	s := testYTServerWithFakeBin(t)

	_, body := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	if body["id"] == nil {
		t.Fatalf("add failed: %v", body)
	}
	code, res := ytCall(t, ytMux(s), http.MethodPost, "/api/v1/yt/sync", nil)
	if code != 200 {
		t.Fatalf("sync-all: code=%d body=%v", code, res)
	}
	if res["synced"] != float64(1) || res["total"] != float64(1) {
		t.Fatalf("sync-all result = %v, want synced=1 total=1", res)
	}
}
