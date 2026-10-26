// Package youtube syncs YouTube channel videos into the x-digest yt_*
// tables (prototype 2026-10-26). No API key, no paid service: each
// channel's public RSS feed is the only data source.
//
// Per channel, one request:
//
//	GET https://www.youtube.com/feeds/videos.xml?channel_id=UC...
//
// The Atom feed gives, per entry: yt:videoId, title, link href,
// published, media:thumbnail url, media:description (may be absent).
// Only what the feed gives is kept; video pages are never scraped.
//
// Channel resolve: the add endpoint accepts a UC... ID, a channel URL,
// or an @handle. A non-ID input is resolved by GET-ing the URL (or
// https://www.youtube.com/@handle) with a browser User-Agent and
// extracting "channelId":"(UC...)" from the HTML (first match).
//
// Failure style mirrors the github client: a failed feed fetch never
// deletes existing videos — Sync folds the failure into Status and the
// caller keeps the old rows untouched.
package youtube

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"xdigest/internal/model"
)

const (
	defaultBaseURL = "https://www.youtube.com"
	// RSS feeds sometimes serve a consent/redirect page to bare clients;
	// a browser User-Agent keeps the real feed. Same reason as the
	// channel-page resolve below.
	browserUA      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	requestTimeout = 15 * time.Second
)

var shanghai = time.FixedZone("CST", 8*3600)

// Channel IDs are "UC" + 22 base64url-ish chars (24 total).
var channelIDRe = regexp.MustCompile(`^UC[0-9A-Za-z_-]{22}$`)

// channelIDInPage finds the first "channelId":"UC..." in a channel page's HTML.
var channelIDInPageRe = regexp.MustCompile(`"channelId":"(UC[0-9A-Za-z_-]{22})"`)

// Client fetches YouTube RSS feeds and channel pages. BaseURL is
// overridable so tests can point it at httptest servers; production
// always uses https://www.youtube.com.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// New returns a client with a 15s timeout, pointed at youtube.com.
func New() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: requestTimeout},
		BaseURL: defaultBaseURL,
	}
}

// IsChannelID reports whether s is already a UC... channel ID.
func IsChannelID(s string) bool { return channelIDRe.MatchString(strings.TrimSpace(s)) }

// FeedURL returns the public RSS feed URL for a channel ID.
func (c *Client) FeedURL(channelID string) string {
	return strings.TrimSuffix(c.BaseURL, "/") + "/feeds/videos.xml?channel_id=" + url.QueryEscape(channelID)
}

// SyncResult is one channel sync. Videos is nil when the fetch failed;
// Status is "synced" or "failed" and Error carries the reason.
type SyncResult struct {
	ChannelName string
	Videos      []model.YTVideo
	Status      string
	Error       string
}

// Sync fetches one channel's RSS feed and returns its videos, newest
// first (the feed already lists them that way). A feed with zero entries
// is OK — a brand-new channel — and syncs with an empty list.
func (c *Client) Sync(ctx context.Context, channelID string) (*SyncResult, error) {
	res := &SyncResult{Status: "synced"}
	feed, err := c.FetchFeed(ctx, channelID)
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		return res, nil
	}
	res.ChannelName = feed.Author.Name
	res.Videos = feed.Videos(channelID)
	return res, nil
}

// ResolveChannelID turns user input into a UC... channel ID. It accepts:
//   - a UC... ID as-is
//   - a channel URL (any youtube.com/@handle, /channel/UC..., /c/..., /user/...)
//   - an @handle (with or without the leading @)
//
// Non-ID inputs are resolved by GET-ing the channel page and extracting
// "channelId":"(UC...)" from the HTML. Failure returns an error asking
// for the UC... ID directly.
func (c *Client) ResolveChannelID(ctx context.Context, input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("请输入频道 ID、频道链接或 @handle")
	}
	if IsChannelID(s) {
		return s, nil
	}
	// A /channel/UC... URL already contains the ID.
	if m := regexp.MustCompile(`/(?:channel/)(UC[0-9A-Za-z_-]{22})\b`).FindStringSubmatch(s); m != nil {
		return m[1], nil
	}
	pageURL := s
	switch {
	case strings.HasPrefix(s, "@"):
		pageURL = strings.TrimSuffix(c.BaseURL, "/") + "/" + s
	case !strings.Contains(s, "://") && !strings.Contains(s, "/"):
		pageURL = strings.TrimSuffix(c.BaseURL, "/") + "/@" + s
	}
	id, err := c.resolveFromPage(ctx, pageURL)
	if err != nil {
		return "", fmt.Errorf("无法从 %s 解析频道 ID，请直接提供 UC 开头的频道 ID：%v", pageURL, err)
	}
	return id, nil
}

func (c *Client) resolveFromPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("YouTube 返回 %d", resp.StatusCode)
	}
	// Channel pages are large; the channelId usually appears early, but
	// cap the read so a pathological page cannot balloon memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	m := channelIDInPageRe.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("页面中未找到 channelId")
	}
	return string(m[1]), nil
}

// FetchFeed GETs the channel's public RSS feed and parses it.
func (c *Client) FetchFeed(ctx context.Context, channelID string) (*Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.FeedURL(channelID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YouTube RSS 返回 %d", resp.StatusCode)
	}
	return ParseFeed(resp.Body)
}

// ---- Atom feed parsing (encoding/xml, streaming) ----

const (
	atomNS  = "http://www.w3.org/2005/Atom"
	ytNS    = "http://www.youtube.com/xml/schemas/2015"
	mediaNS = "http://search.yahoo.com/mrss/"
)

// Feed is a parsed YouTube channel RSS feed.
type Feed struct {
	Title   string      `xml:"http://www.w3.org/2005/Atom title"`
	Author  feedAuthor  `xml:"http://www.w3.org/2005/Atom author"`
	Entries []feedEntry `xml:"http://www.w3.org/2005/Atom entry"`
}

type feedAuthor struct {
	Name string `xml:"http://www.w3.org/2005/Atom name"`
	URI  string `xml:"http://www.w3.org/2005/Atom uri"`
}

type feedEntry struct {
	VideoID   string     `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	ChannelID string     `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
	Title     string     `xml:"http://www.w3.org/2005/Atom title"`
	Link      feedLink   `xml:"http://www.w3.org/2005/Atom link"`
	Published string     `xml:"http://www.w3.org/2005/Atom published"`
	Group     mediaGroup `xml:"http://search.yahoo.com/mrss/ group"`
}

type feedLink struct {
	Href string `xml:"href,attr"`
}

type mediaGroup struct {
	Title       string         `xml:"http://search.yahoo.com/mrss/ title"`
	Description string         `xml:"http://search.yahoo.com/mrss/ description"`
	Thumbnail   mediaThumbnail `xml:"http://search.yahoo.com/mrss/ thumbnail"`
}

type mediaThumbnail struct {
	URL string `xml:"url,attr"`
}

// ParseFeed decodes an Atom feed from r (streaming; no full-body buffer).
func ParseFeed(r io.Reader) (*Feed, error) {
	var f Feed
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("解析 RSS 失败：%v", err)
	}
	return &f, nil
}

// Videos converts feed entries to model rows. Entries without a video ID
// are skipped; a missing yt:videoId falls back to the v= query param of
// the entry link. Missing description/thumbnail stay empty — the feed
// simply does not provide them.
func (f *Feed) Videos(channelID string) []model.YTVideo {
	out := []model.YTVideo{}
	now := time.Now().In(shanghai).Format("2006-01-02 15:04:05")
	for _, e := range f.Entries {
		id := strings.TrimSpace(e.VideoID)
		if id == "" {
			if u, err := url.Parse(e.Link.Href); err == nil {
				id = u.Query().Get("v")
			}
		}
		if id == "" {
			continue
		}
		vid := model.YTVideo{
			ID:           id,
			ChannelID:    channelID,
			Title:        strings.TrimSpace(e.Title),
			Description:  strings.TrimSpace(e.Group.Description),
			PublishedAt:  parsePublished(e.Published),
			ThumbnailURL: strings.TrimSpace(e.Group.Thumbnail.URL),
			URL:          "https://www.youtube.com/watch?v=" + id,
			CreatedAt:    now,
		}
		out = append(out, vid)
	}
	return out
}

// parsePublished converts the feed's RFC3339 timestamp to Shanghai wall
// time ("2006-01-02 15:04:05"). Unparsable values are kept as-is so the
// row still upserts instead of failing the whole sync.
func parsePublished(s string) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	return t.In(shanghai).Format("2006-01-02 15:04:05")
}
