package store

// YouTube store tests run against a scratch MariaDB database on the
// local machine (never the Shanghai server). Set XDIGEST_TEST_DSN to a
// server DSN with multiStatements enabled, e.g.:
//
//	XDIGEST_TEST_DSN='root@unix(/run/mysqld/mysqld.sock)/?parseTime=true&multiStatements=true'
//
// Without it the tests skip, so `go test ./...` still passes on machines
// without a local database.

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/model"
)

func testYTStore(t *testing.T) *Store {
	t.Helper()
	serverDSN := os.Getenv("XDIGEST_TEST_DSN")
	if serverDSN == "" {
		t.Skip("XDIGEST_TEST_DSN not set; skipping DB-backed YouTube store tests")
	}
	srv, err := sql.Open("mysql", serverDSN)
	if err != nil {
		t.Fatalf("open server DSN: %v", err)
	}
	defer srv.Close()
	// Own scratch DB per package: go test runs packages in parallel.
	if _, err := srv.Exec("DROP DATABASE IF EXISTS xdigest_test_store"); err != nil {
		t.Fatalf("drop scratch DB: %v", err)
	}
	if _, err := srv.Exec("CREATE DATABASE xdigest_test_store CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create scratch DB: %v", err)
	}
	dbDSN := strings.Replace(serverDSN, "/?", "/xdigest_test_store?", 1)
	st, err := New(dbDSN)
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
	return st
}

const testChannelID = "UC_x5XG1OV2P6uZZ5FSM9Ttw"

func testYTChannel() model.YTChannel {
	return model.YTChannel{
		ID: "ytc_test1", ChannelID: testChannelID, Handle: "@testchannel",
		Name: "Test Channel", GroupID: "", CreatedAt: Now(), SyncStatus: "never",
	}
}

func testYTVideos() []model.YTVideo {
	return []model.YTVideo{
		{ID: "dQw4w9WgXcQ", ChannelID: testChannelID, Title: "First video", Description: "about cats", PublishedAt: "2026-10-24 16:30:00", ThumbnailURL: "https://i4.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", CreatedAt: Now()},
		{ID: "abc123DEF45", ChannelID: testChannelID, Title: "Second video", Description: "about dogs", PublishedAt: "2026-10-22 20:00:00", ThumbnailURL: "", URL: "https://www.youtube.com/watch?v=abc123DEF45", CreatedAt: Now()},
	}
}

// Upserting the same videos twice must never duplicate rows; a re-fetch
// updates the title in place.
func TestYTUpsertNoDuplicate(t *testing.T) {
	st := testYTStore(t)
	if err := st.CreateYTChannel(testYTChannel()); err != nil {
		t.Fatalf("CreateYTChannel: %v", err)
	}
	if err := st.UpsertYTVideos(testChannelID, testYTVideos()); err != nil {
		t.Fatalf("upsert #1: %v", err)
	}
	videos, err := st.ListYTVideos(YTVideoFilter{})
	if err != nil {
		t.Fatalf("ListYTVideos: %v", err)
	}
	if len(videos) != 2 {
		t.Fatalf("after upsert #1: got %d videos, want 2", len(videos))
	}
	// Newest first.
	if videos[0].ID != "dQw4w9WgXcQ" || videos[1].ID != "abc123DEF45" {
		t.Fatalf("order wrong: %+v", videos)
	}
	if videos[0].ChannelName != "Test Channel" {
		t.Fatalf("channel name not joined: %q", videos[0].ChannelName)
	}
	// Second upsert: same IDs, one title changed, one brand-new video.
	again := testYTVideos()
	again[0].Title = "First video (updated title)"
	again = append(again, model.YTVideo{ID: "ZZZ999YYY88", ChannelID: testChannelID, Title: "Third video", Description: "about birds", PublishedAt: "2026-10-21 14:00:00", URL: "https://www.youtube.com/watch?v=ZZZ999YYY88", CreatedAt: Now()})
	if err := st.UpsertYTVideos(testChannelID, again); err != nil {
		t.Fatalf("upsert #2: %v", err)
	}
	videos, err = st.ListYTVideos(YTVideoFilter{})
	if err != nil {
		t.Fatalf("ListYTVideos: %v", err)
	}
	if len(videos) != 3 {
		t.Fatalf("after upsert #2: got %d videos, want 3 (no duplicates)", len(videos))
	}
	if videos[0].Title != "First video (updated title)" {
		t.Fatalf("title not updated on re-upsert: %q", videos[0].Title)
	}
}

func TestYTVideoFilters(t *testing.T) {
	st := testYTStore(t)
	if err := st.CreateYTChannel(testYTChannel()); err != nil {
		t.Fatalf("CreateYTChannel: %v", err)
	}
	if err := st.UpsertYTVideos(testChannelID, testYTVideos()); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Keyword matches title/description.
	got, err := st.ListYTVideos(YTVideoFilter{Q: "cats"})
	if err != nil || len(got) != 1 || got[0].ID != "dQw4w9WgXcQ" {
		t.Fatalf("Q=cats: got %v err %v", got, err)
	}
	got, err = st.ListYTVideos(YTVideoFilter{Q: "dogs"})
	if err != nil || len(got) != 1 || got[0].ID != "abc123DEF45" {
		t.Fatalf("Q=dogs: got %v err %v", got, err)
	}
	// Date range: only the 2026-10-24 video falls in 10-23..10-25.
	got, err = st.ListYTVideos(YTVideoFilter{DateFrom: "2026-10-23", DateTo: "2026-10-25"})
	if err != nil || len(got) != 1 || got[0].ID != "dQw4w9WgXcQ" {
		t.Fatalf("date range: got %v err %v", got, err)
	}
	// Channel filter + limit.
	got, err = st.ListYTVideos(YTVideoFilter{ChannelID: testChannelID, Limit: 1})
	if err != nil || len(got) != 1 {
		t.Fatalf("channel+limit: got %v err %v", got, err)
	}
	got, err = st.ListYTVideos(YTVideoFilter{ChannelID: "UC_doesnotexist"})
	if err != nil || len(got) != 0 {
		t.Fatalf("unknown channel: got %v err %v", got, err)
	}
}

func TestYTChannelDeleteCascadesVideos(t *testing.T) {
	st := testYTStore(t)
	if err := st.CreateYTChannel(testYTChannel()); err != nil {
		t.Fatalf("CreateYTChannel: %v", err)
	}
	if err := st.UpsertYTVideos(testChannelID, testYTVideos()); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := st.DeleteYTChannel("ytc_test1"); err != nil {
		t.Fatalf("DeleteYTChannel: %v", err)
	}
	if _, err := st.GetYTChannel("ytc_test1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("channel should be gone, err=%v", err)
	}
	videos, err := st.ListYTVideos(YTVideoFilter{})
	if err != nil {
		t.Fatalf("ListYTVideos: %v", err)
	}
	if len(videos) != 0 {
		t.Fatalf("videos should be gone with the channel, got %d", len(videos))
	}
}

func TestYTGroupDeleteMovesChannels(t *testing.T) {
	st := testYTStore(t)
	g, err := st.CreateYTGroup("测试分组")
	if err != nil {
		t.Fatalf("CreateYTGroup: %v", err)
	}
	ch := testYTChannel()
	ch.GroupID = g.ID
	if err := st.CreateYTChannel(ch); err != nil {
		t.Fatalf("CreateYTChannel: %v", err)
	}
	if code, err := st.DeleteYTGroup(g.ID); code != "" || err != nil {
		t.Fatalf("DeleteYTGroup: code=%q err=%v", code, err)
	}
	after, err := st.GetYTChannel("ytc_test1")
	if err != nil {
		t.Fatalf("GetYTChannel: %v", err)
	}
	if after.GroupID != "other" {
		t.Fatalf("channel group after delete = %q, want 'other'", after.GroupID)
	}
	// The fixed default group cannot be deleted.
	if code, _ := st.DeleteYTGroup("other"); code != "GROUP_FIXED" {
		t.Fatalf("deleting fixed group: code=%q, want GROUP_FIXED", code)
	}
}
