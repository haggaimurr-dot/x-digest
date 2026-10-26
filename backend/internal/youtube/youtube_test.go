package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A realistic channel feed: two full entries, one missing description /
// thumbnail, one missing yt:videoId (falls back to the v= link param).
const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
  <link rel="self" href="https://www.youtube.com/feeds/videos.xml?channel_id=UC_x5XG1OV2P6uZZ5FSM9Ttw"/>
  <id>yt:channel:UC_x5XG1OV2P6uZZ5FSM9Ttw</id>
  <yt:channelId>UC_x5XG1OV2P6uZZ5FSM9Ttw</yt:channelId>
  <title>Google for Developers</title>
  <author><name>Google for Developers</name><uri>https://www.youtube.com/channel/UC_x5XG1OV2P6uZZ5FSM9Ttw</uri></author>
  <published>2026-10-20T10:00:00+00:00</published>
  <entry>
    <id>yt:video:dQw4w9WgXcQ</id>
    <yt:videoId>dQw4w9WgXcQ</yt:videoId>
    <yt:channelId>UC_x5XG1OV2P6uZZ5FSM9Ttw</yt:channelId>
    <title>First video title</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=dQw4w9WgXcQ"/>
    <author><name>Google for Developers</name></author>
    <published>2026-10-24T08:30:00+00:00</published>
    <updated>2026-10-24T09:00:00+00:00</updated>
    <media:group>
      <media:title>First video title</media:title>
      <media:content url="https://www.youtube.com/v/dQw4w9WgXcQ?version=3&amp;autohide=1" type="application/x-shockwave-flash" width="640" height="390"/>
      <media:thumbnail url="https://i4.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" width="480" height="360"/>
      <media:description>First video description here.</media:description>
    </media:group>
  </entry>
  <entry>
    <id>yt:video:abc123DEF45</id>
    <yt:videoId>abc123DEF45</yt:videoId>
    <yt:channelId>UC_x5XG1OV2P6uZZ5FSM9Ttw</yt:channelId>
    <title>Second video, no description</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=abc123DEF45"/>
    <author><name>Google for Developers</name></author>
    <published>2026-10-22T12:00:00+00:00</published>
    <media:group>
      <media:title>Second video, no description</media:title>
    </media:group>
  </entry>
  <entry>
    <id>yt:video:ZZZ999YYY88</id>
    <yt:channelId>UC_x5XG1OV2P6uZZ5FSM9Ttw</yt:channelId>
    <title>Third video, no videoId element</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=ZZZ999YYY88"/>
    <author><name>Google for Developers</name></author>
    <published>2026-10-21T06:00:00+00:00</published>
    <media:group>
      <media:title>Third video, no videoId element</media:title>
      <media:thumbnail url="https://i4.ytimg.com/vi/ZZZ999YYY88/hqdefault.jpg" width="480" height="360"/>
      <media:description>Recovered via the v= link param.</media:description>
    </media:group>
  </entry>
</feed>
`

func TestParseFeed(t *testing.T) {
	f, err := ParseFeed(strings.NewReader(sampleFeed))
	if err != nil {
		t.Fatalf("ParseFeed: %v", err)
	}
	if f.Author.Name != "Google for Developers" {
		t.Fatalf("author = %q", f.Author.Name)
	}
	videos := f.Videos("UC_x5XG1OV2P6uZZ5FSM9Ttw")
	if len(videos) != 3 {
		t.Fatalf("got %d videos, want 3", len(videos))
	}
	v := videos[0]
	if v.ID != "dQw4w9WgXcQ" || v.Title != "First video title" {
		t.Fatalf("video[0] = %+v", v)
	}
	if v.Description != "First video description here." {
		t.Fatalf("video[0] description = %q", v.Description)
	}
	if v.ThumbnailURL != "https://i4.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" {
		t.Fatalf("video[0] thumbnail = %q", v.ThumbnailURL)
	}
	if v.URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("video[0] url = %q", v.URL)
	}
	// Feed timestamps are UTC; stored as Shanghai wall time (+08:00).
	if v.PublishedAt != "2026-10-24 16:30:00" {
		t.Fatalf("video[0] published_at = %q, want 2026-10-24 16:30:00", v.PublishedAt)
	}
	// Missing description/thumbnail stay empty, row still parses.
	if v2 := videos[1]; v2.ID != "abc123DEF45" || v2.Description != "" || v2.ThumbnailURL != "" {
		t.Fatalf("video[1] = %+v", v2)
	}
	// Missing yt:videoId falls back to the v= link param.
	if v3 := videos[2]; v3.ID != "ZZZ999YYY88" || v3.Description != "Recovered via the v= link param." {
		t.Fatalf("video[2] = %+v", v3)
	}
}

func TestParseFeedInvalid(t *testing.T) {
	if _, err := ParseFeed(strings.NewReader("<feed><entry>")); err == nil {
		t.Fatal("expected error for malformed XML")
	}
	if _, err := ParseFeed(strings.NewReader("not xml at all")); err == nil {
		t.Fatal("expected error for non-XML input")
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

// feedTestServer serves the sample feed at the real feed path.
func feedTestServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feeds/videos.xml" || r.URL.Query().Get("channel_id") == "" {
			t.Errorf("unexpected feed request: %s", r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestFetchFeedAndSync(t *testing.T) {
	srv := feedTestServer(t, http.StatusOK, sampleFeed)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	res, err := c.Sync(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Status != "synced" || res.Error != "" {
		t.Fatalf("res = %+v", res)
	}
	if res.ChannelName != "Google for Developers" {
		t.Fatalf("channel name = %q", res.ChannelName)
	}
	if len(res.Videos) != 3 {
		t.Fatalf("got %d videos, want 3", len(res.Videos))
	}
	if res.Videos[0].ChannelID != "UC_x5XG1OV2P6uZZ5FSM9Ttw" {
		t.Fatalf("video channel_id = %q", res.Videos[0].ChannelID)
	}
}

func TestSyncEmptyFeedOK(t *testing.T) {
	empty := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <title>Brand New Channel</title>
  <author><name>Brand New Channel</name></author>
</feed>`
	srv := feedTestServer(t, http.StatusOK, empty)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	res, err := c.Sync(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Status != "synced" || len(res.Videos) != 0 {
		t.Fatalf("empty feed should sync with zero videos, got %+v", res)
	}
}

func TestSyncFailedFeedKeepsStatus(t *testing.T) {
	srv := feedTestServer(t, http.StatusNotFound, "not found")
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	res, err := c.Sync(context.Background(), "UC_x5XG1OV2P6uZZ5FSM9Ttw")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Status != "failed" || res.Error == "" || res.Videos != nil {
		t.Fatalf("failed feed should fold into Status, got %+v", res)
	}
}
