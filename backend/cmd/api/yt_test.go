package main

// YouTube handler tests run against a scratch MariaDB database (same
// XDIGEST_TEST_DSN convention as internal/store). Feed fetches and
// channel-page resolves run against httptest servers, never real YouTube.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/store"
	"xdigest/internal/youtube"
)

const ytTestFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
  <title>Handler Test Channel</title>
  <author><name>Handler Test Channel</name></author>
  <entry>
    <id>yt:video:vid000000001</id>
    <yt:videoId>vid000000001</yt:videoId>
    <yt:channelId>UCtesttesttesttesttest00</yt:channelId>
    <title>Handler test video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=vid000000001"/>
    <published>2026-10-24T08:30:00+00:00</published>
    <media:group>
      <media:title>Handler test video</media:title>
      <media:description>A video for handler tests.</media:description>
    </media:group>
  </entry>
</feed>
`

func testYTServer(t *testing.T, yt *youtube.Client) *server {
	t.Helper()
	serverDSN := os.Getenv("XDIGEST_TEST_DSN")
	if serverDSN == "" {
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
	return &server{st: st, yt: yt}
}

// call invokes one handler directly and decodes the JSON body.
func ytCall(t *testing.T, h http.HandlerFunc, method, path string, body any) (int, map[string]any) {
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
	h(rec, req)
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
	code, body := ytCall(t, s.handleYTChannels, http.MethodPost, "/api/v1/yt/channels", map[string]any{})
	if code != 422 {
		t.Fatalf("empty input: code=%d body=%v, want 422", code, body)
	}
	// Unresolvable handle: the error must ask for the UC... ID.
	code, body = ytCall(t, s.handleYTChannels, http.MethodPost, "/api/v1/yt/channels", map[string]any{"handle": "@nobody"})
	if code != 422 {
		t.Fatalf("@nobody: code=%d body=%v, want 422", code, body)
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "UC") {
		t.Fatalf("422 message should ask for the UC ID, got %q", msg)
	}
	// Unknown group is also a 422.
	code, body = ytCall(t, s.handleYTChannels, http.MethodPost, "/api/v1/yt/channels", map[string]any{"channel_id": "UCtesttesttesttesttest00", "group_id": "no-such-group"})
	if code != 422 {
		t.Fatalf("unknown group: code=%d body=%v, want 422", code, body)
	}
}

func TestYTAddChannelSyncsOnceThenDuplicate409(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feeds/videos.xml" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(ytTestFeed))
	}))
	defer feed.Close()
	s := testYTServer(t, &youtube.Client{HTTP: feed.Client(), BaseURL: feed.URL})

	code, body := ytCall(t, s.handleYTChannels, http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00", "handle": "@handlertest"})
	if code != 201 {
		t.Fatalf("add: code=%d body=%v, want 201", code, body)
	}
	if body["name"] != "Handler Test Channel" {
		t.Fatalf("name = %v, want feed author name", body["name"])
	}
	if body["sync_status"] != "synced" {
		t.Fatalf("sync_status = %v, want synced", body["sync_status"])
	}
	if body["handle"] != "@handlertest" {
		t.Fatalf("handle = %v", body["handle"])
	}
	// The sync ran once at add time: the video is already listed.
	code, body = ytCall(t, s.handleYTVideos, http.MethodGet, "/api/v1/yt/videos", nil)
	if code != 200 {
		t.Fatalf("videos: code=%d body=%v", code, body)
	}
	videos, _ := body["videos"].([]any)
	if len(videos) != 1 {
		t.Fatalf("videos = %v, want 1 video after add", body["videos"])
	}
	// Adding the same channel again is a 409.
	code, body = ytCall(t, s.handleYTChannels, http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	if code != 409 {
		t.Fatalf("duplicate: code=%d body=%v, want 409", code, body)
	}
}

func TestYTChannelSubHandlers(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(ytTestFeed))
	}))
	defer feed.Close()
	s := testYTServer(t, &youtube.Client{HTTP: feed.Client(), BaseURL: feed.URL})

	_, body := ytCall(t, s.handleYTChannels, http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("add did not return an id: %v", body)
	}
	// GET one channel.
	code, got := ytCall(t, s.handleYTChannelSub, http.MethodGet, "/api/v1/yt/channels/"+id, nil)
	if code != 200 || got["channel_id"] != "UCtesttesttesttesttest00" {
		t.Fatalf("get one: code=%d body=%v", code, got)
	}
	// GET missing channel is a 404.
	code, _ = ytCall(t, s.handleYTChannelSub, http.MethodGet, "/api/v1/yt/channels/ytc_nope", nil)
	if code != 404 {
		t.Fatalf("get missing: code=%d, want 404", code)
	}
	// DELETE removes the channel and its videos.
	code, _ = ytCall(t, s.handleYTChannelSub, http.MethodDelete, "/api/v1/yt/channels/"+id, nil)
	if code != 200 {
		t.Fatalf("delete: code=%d, want 200", code)
	}
	code, body = ytCall(t, s.handleYTVideos, http.MethodGet, "/api/v1/yt/videos", nil)
	if code != 200 {
		t.Fatalf("videos after delete: code=%d", code)
	}
	if videos, _ := body["videos"].([]any); len(videos) != 0 {
		t.Fatalf("videos should be gone after channel delete, got %v", videos)
	}
	// DELETE again is a 404.
	code, _ = ytCall(t, s.handleYTChannelSub, http.MethodDelete, "/api/v1/yt/channels/"+id, nil)
	if code != 404 {
		t.Fatalf("delete missing: code=%d, want 404", code)
	}
}

func TestYTSyncAll(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(ytTestFeed))
	}))
	defer feed.Close()
	s := testYTServer(t, &youtube.Client{HTTP: feed.Client(), BaseURL: feed.URL})

	_, body := ytCall(t, s.handleYTChannels, http.MethodPost, "/api/v1/yt/channels",
		map[string]any{"channel_id": "UCtesttesttesttesttest00"})
	if body["id"] == nil {
		t.Fatalf("add failed: %v", body)
	}
	code, res := ytCall(t, s.handleYTSyncAll, http.MethodPost, "/api/v1/yt/sync", nil)
	if code != 200 {
		t.Fatalf("sync-all: code=%d body=%v", code, res)
	}
	if res["synced"] != float64(1) || res["total"] != float64(1) {
		t.Fatalf("sync-all result = %v, want synced=1 total=1", res)
	}
}
