// Package youtube syncs YouTube channel videos into the x-digest yt_*
// tables, driven by the yt-dlp binary (no API key, no paid service).
//
// Why yt-dlp, why RSS was dropped: the prototype (2026-10-26) read each
// channel's public RSS feed, but a feed only carries the ~15 latest
// videos — no tab split (videos / Shorts / lives / playlists), no
// playlists at all, and no community posts. The v2 page needs four
// separate tabs plus a per-channel time range plus a description per
// item, none of which RSS can supply, so the feed code was deleted and
// sync now shells out to yt-dlp.
//
// How a sync works (Client.SyncTabs, one channel):
//
//  1. Tab listing. For each tab in {videos, shorts, streams, playlists}
//     run `yt-dlp --flat-playlist --dump-json --no-warnings
//     --playlist-end 30 https://www.youtube.com/channel/<UC ID>/<tab>`
//     and parse stdout as one JSON object per line. Kind mapping:
//     videos→video, shorts→short, streams→live, playlists→playlist.
//     Cap 30 items per tab. Flat entries carry id, title, duration,
//     live_status, webpage_url (canonical URL — watch?v=, /shorts/ or
//     playlist?list= — used as the row URL), thumbnails (the FIRST
//     entry's url is used) and playlist_title (e.g. "Google for
//     Developers - Videos"; the prefix before " - <Tab>" is the channel
//     name, used when the stored name is empty). Flat entries carry NO
//     upload_date and NO description — that is what step 2 is for.
//     --dateafter/--datebefore are NOT passed: verified, in flat mode
//     yt-dlp ignores them (flat entries have no upload_date), so they
//     would silently do nothing. The range is enforced in step 3.
//
//  2. Description + date backfill. For items that are NEW (not in the
//     knownIDs set the caller passes) or are missing a description /
//     publish date, run one metadata fetch per item:
//     `yt-dlp --dump-json --no-playlist --no-warnings --skip-download
//     <item webpage_url>`, which returns description + upload_date
//     (YYYYMMDD) + duration + live_status. Cap 10 items per tab per
//     sync, newest first (the flat listing is newest-first), 800ms
//     between calls, 30s timeout each. Backfill failure is tolerated
//     completely: a 429, a timeout or a bad response just leaves that
//     item unfilled and the sync continues; it is retried on the next
//     sync. A sync never fails because of the backfill.
//
//  3. Date-range enforcement. A video/short/live item is only stored
//     once its backfill has supplied an upload_date. If the channel has
//     date_from/date_to set and the date falls outside [from, to]
//     (date_to inclusive of the whole day) the item is SKIPPED, never
//     stored. If the backfill failed (no upload_date) the item is NOT
//     stored yet — it stays a candidate for the next sync. Playlists
//     are always stored (no date to check; the range does not apply).
//     Consequence: a newly added channel fills in gradually, up to 10
//     backfilled items per tab per sync.
//
// Failure style mirrors the github client: a failed tab fetch never
// deletes existing videos. Status is "failed" only if ALL FOUR tabs
// failed; if at least one tab succeeded the sync is "synced" and the
// failed tab names are listed in FailedTabs (Error stays empty on
// partial success). A non-zero yt-dlp exit with partial stdout is OK —
// the lines that parsed are kept.
//
// Proxy: when YT_HTTP_PROXY is set, every yt-dlp call gets
// `--proxy <url>` and the channel-page fetches below go through the
// same proxy. The Shanghai box needs it: YouTube is unreachable from
// mainland China (the TLS SNI is killed by the GFW), so the server runs
// a local xray client (xray-yt) whose VLESS tunnel to the LA node hides
// the SNI; YT_HTTP_PROXY=http://127.0.0.1:18099 points at it. On a
// machine with direct YouTube access the variable is simply unset.
//
// Channel resolve: the add endpoint accepts a UC... ID, a channel URL,
// or an @handle. A non-ID input is resolved by GET-ing the URL (or
// https://www.youtube.com/@handle) with a browser User-Agent and
// extracting "channelId":"(UC...)" from the HTML (first match).
package youtube

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"xdigest/internal/model"
)

const (
	defaultBaseURL = "https://www.youtube.com"
	// The channel-page resolve sometimes serves a consent/redirect page
	// to bare clients; a browser User-Agent keeps the real page.
	browserUA      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	requestTimeout = 15 * time.Second

	// tabListCap is the --playlist-end for each tab listing.
	tabListCap = 30
	// backfillCap is the max metadata fetches per tab per sync.
	backfillCap = 10
	// backfillDelay is the pause between metadata fetches, to stay
	// gentle with YouTube (it 429s aggressive clients).
	backfillDelay = 800 * time.Millisecond
	// tabListTimeout bounds one tab listing; metaTimeout bounds one
	// single-item metadata fetch.
	tabListTimeout = 120 * time.Second
	metaTimeout    = 30 * time.Second
)

var shanghai = time.FixedZone("CST", 8*3600)

// Channel IDs are "UC" + 22 base64url-ish chars (24 total).
var channelIDRe = regexp.MustCompile(`^UC[0-9A-Za-z_-]{22}$`)

// channelIDInPage finds the first "channelId":"UC..." in a channel page's HTML.
var channelIDInPageRe = regexp.MustCompile(`"channelId":"(UC[0-9A-Za-z_-]{22})"`)

// channelIDCanonical finds the channel ID in the page's canonical link
// (https://www.youtube.com/channel/UC...). Some channel pages (e.g.
// @claudecourse) carry the ID only in URLs — canonical, og:url, the RSS
// feed link — and never as a "channelId" JSON field, so the lookup above
// misses them. Fall back to this before giving up.
var channelIDCanonicalRe = regexp.MustCompile(`/channel/(UC[0-9A-Za-z_-]{22})`)

// ytTab is one channel tab: its URL slug and the Kind stored for its items.
type ytTab struct {
	slug string
	kind string
}

var ytTabs = []ytTab{
	{slug: "videos", kind: "video"},
	{slug: "shorts", kind: "short"},
	{slug: "streams", kind: "live"},
	{slug: "playlists", kind: "playlist"},
}

// tabTitleSuffix is stripped from a flat entry's playlist_title to get
// the channel name, e.g. "Google for Developers - Videos" → "Google
// for Developers".
var tabTitleSuffix = map[string]string{
	"videos":    " - Videos",
	"shorts":    " - Shorts",
	"streams":   " - Live",
	"playlists": " - Playlists",
}

// Client runs yt-dlp and fetches channel pages. BaseURL is overridable
// so tests can point resolve at httptest servers; production always
// uses https://www.youtube.com.
//
// Bin / Proxy default to the YT_DLP_BIN / YT_HTTP_PROXY environment
// when left empty, so tests can point the runner at a fake binary with
// t.Setenv without constructing the client through New. Runner, when
// set, replaces the real exec call entirely (unit tests inject canned
// yt-dlp output through it). BackfillDelay overrides the 800ms pause
// between metadata fetches (tests set it to 0).
type Client struct {
	HTTP          *http.Client
	BaseURL       string
	Bin           string
	Proxy         string
	CookieFile    string
	Runner        func(ctx context.Context, args []string) ([]byte, error)
	BackfillDelay time.Duration
}

// New returns a client with a 15s HTTP timeout, pointed at youtube.com.
// The yt-dlp binary path comes from YT_DLP_BIN (default "yt-dlp") and
// the proxy from YT_HTTP_PROXY (see the package comment).
func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := strings.TrimSpace(os.Getenv("YT_HTTP_PROXY")); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &Client{
		HTTP:    &http.Client{Timeout: requestTimeout, Transport: transport},
		BaseURL: defaultBaseURL,
		Bin:     strings.TrimSpace(os.Getenv("YT_DLP_BIN")),
		Proxy:   strings.TrimSpace(os.Getenv("YT_HTTP_PROXY")),
	}
}

// IsChannelID reports whether s is already a UC... channel ID.
func IsChannelID(s string) bool { return channelIDRe.MatchString(strings.TrimSpace(s)) }

func (c *Client) binPath() string {
	if strings.TrimSpace(c.Bin) != "" {
		return strings.TrimSpace(c.Bin)
	}
	if v := strings.TrimSpace(os.Getenv("YT_DLP_BIN")); v != "" {
		return v
	}
	return "yt-dlp"
}

func (c *Client) proxyURL() string {
	if strings.TrimSpace(c.Proxy) != "" {
		return strings.TrimSpace(c.Proxy)
	}
	return strings.TrimSpace(os.Getenv("YT_HTTP_PROXY"))
}

// cookieFile returns the path to the YouTube cookies file (Netscape format),
// used to bypass "Sign in to confirm you're not a bot". Empty if not configured.
func (c *Client) cookieFile() string {
	if strings.TrimSpace(c.CookieFile) != "" {
		return strings.TrimSpace(c.CookieFile)
	}
	return strings.TrimSpace(os.Getenv("YT_COOKIE_FILE"))
}

func (c *Client) backfillDelay() time.Duration {
	if c.BackfillDelay != 0 {
		return c.BackfillDelay
	}
	return backfillDelay
}

// runYtDlp executes the yt-dlp binary with args (plus --proxy when a
// proxy is configured) and returns its stdout. A non-zero exit with
// partial stdout is NOT an error here — the caller parses whatever
// lines survived; runErr is returned alongside so the caller can tell
// a clean run from a partial one. With no stdout at all, a non-zero
// exit is an error.
func (c *Client) runYtDlp(ctx context.Context, args ...string) ([]byte, error) {
	if c.Runner != nil {
		return c.Runner(ctx, args)
	}
	full := []string{}
	if proxy := c.proxyURL(); proxy != "" {
		full = append(full, "--proxy", proxy)
	}
	if ck := c.cookieFile(); ck != "" {
		full = append(full, "--cookies", ck)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, c.binPath(), full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("yt-dlp: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), err
}

// SyncResult is one channel sync. Videos is nil when every tab failed;
// Status is "synced" or "failed" and Error carries the reason (empty on
// partial success — see FailedTabs). SyncedTabs counts the tabs that
// listed OK; FailedTabs names the ones that did not.
type SyncResult struct {
	ChannelName string
	Videos      []model.YTVideo
	Status      string
	Error       string
	SyncedTabs  int
	FailedTabs  []string
}

// SyncTabs syncs one channel's four tabs into model rows, newest first
// within each tab. dateFrom/dateTo are "2006-01-02" ("" = no limit);
// knownIDs holds the video/playlist IDs already stored with a complete
// row (description, and for non-playlists a publish date) — those are
// skipped, everything else gets a metadata backfill attempt (cap 10 per
// tab). See the package comment for the full design.
func (c *Client) SyncTabs(ctx context.Context, channelID, dateFrom, dateTo string, knownIDs map[string]bool) (*SyncResult, error) {
	if strings.TrimSpace(channelID) == "" {
		return nil, fmt.Errorf("channel ID 为空")
	}
	res := &SyncResult{Status: "synced", Videos: []model.YTVideo{}}
	seen := map[string]bool{}
	for _, tab := range ytTabs {
		items, name, err := c.listTab(ctx, channelID, tab)
		if err != nil {
			res.FailedTabs = append(res.FailedTabs, tab.slug)
			continue
		}
		res.SyncedTabs++
		if res.ChannelName == "" && name != "" {
			res.ChannelName = name
		}
		backfilled := 0
		for _, it := range items {
			if seen[it.id] {
				continue
			}
			seen[it.id] = true
			if knownIDs != nil && knownIDs[it.id] {
				continue
			}
			// Playlists have no date to wait for: store them from the
			// flat listing even when the backfill fails; a successful
			// backfill just adds the description.
			if tab.kind == "playlist" {
				v := it.toVideo(channelID, tab.kind)
				if backfilled < backfillCap {
					c.sleepBeforeBackfill(ctx, backfilled)
					if meta, err := c.fetchPlaylistMeta(ctx, it.url); err == nil {
						// Only the description: a playlist has no
						// meaningful duration / live_status of its own.
						v.Description = meta.Description
					}
					backfilled++
				}
				res.Videos = append(res.Videos, v)
				continue
			}
			// Video / short / live: only stored once the backfill has
			// supplied an upload_date. No date yet → not stored, the
			// next sync retries it.
			if backfilled >= backfillCap {
				continue
			}
			c.sleepBeforeBackfill(ctx, backfilled)
			meta, err := c.fetchMeta(ctx, it.url)
			backfilled++
			if err != nil {
				continue
			}
			if !inDateRange(meta.uploadDate(), dateFrom, dateTo) {
				continue
			}
			v := it.toVideo(channelID, tab.kind)
			v.Description = meta.Description
			v.PublishedAt = meta.publishedAt()
			if d := meta.durationSecs(); d > 0 {
				v.Duration = d
			}
			if meta.LiveStatus != "" {
				v.LiveStatus = meta.LiveStatus
			}
			res.Videos = append(res.Videos, v)
		}
	}
	if res.SyncedTabs == 0 {
		res.Status = "failed"
		res.Error = "全部 4 个标签页同步失败（" + strings.Join(res.FailedTabs, "、") + "）"
		res.Videos = nil
	}
	return res, nil
}

// sleepBeforeBackfill pauses before a metadata fetch when it is not
// the first one of this tab, to stay gentle with YouTube (it 429s
// aggressive clients). It respects ctx cancellation.
func (c *Client) sleepBeforeBackfill(ctx context.Context, alreadyFetched int) {
	if alreadyFetched <= 0 {
		return
	}
	d := c.backfillDelay()
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// listTab runs one tab listing and parses its flat entries. The channel
// name comes from the first entry's playlist_title prefix.
func (c *Client) listTab(ctx context.Context, channelID string, tab ytTab) ([]flatEntry, string, error) {
	ctx, cancel := context.WithTimeout(ctx, tabListTimeout)
	defer cancel()
	tabURL := strings.TrimSuffix(c.BaseURL, "/") + "/channel/" + channelID + "/" + tab.slug
	out, runErr := c.runYtDlp(ctx, "--flat-playlist", "--dump-json", "--no-warnings",
		"--playlist-end", strconv.Itoa(tabListCap), tabURL)
	entries, name := parseFlatOutput(out, tab.slug)
	if len(entries) == 0 && runErr != nil {
		return nil, "", fmt.Errorf("标签页 %s 同步失败：%v", tab.slug, runErr)
	}
	return entries, name, nil
}

// fetchMeta runs one single-item metadata fetch. Any failure (non-zero
// exit, timeout, unparsable output) is an error the caller tolerates.
// fetchPlaylistMeta returns a playlist's own description. A playlist URL
// fetched with fetchMeta (--dump-json --no-playlist) resolves to the FIRST
// VIDEO in the playlist, not the playlist — so the description it returns
// is the video's, and for many playlists the call fails outright, leaving
// the playlist with no description. --dump-single-json --flat-playlist
// returns the playlist object itself (with its real description) plus
// lightweight flat entries.
func (c *Client) fetchPlaylistMeta(ctx context.Context, playlistURL string) (*metaEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()
	out, err := c.runYtDlp(ctx, "--dump-single-json", "--flat-playlist", "--no-warnings",
		"--skip-download", playlistURL)
	if err != nil {
		return nil, fmt.Errorf("yt-dlp 播放列表元数据失败：%v", err)
	}
	return parseMeta(out)
}

func (c *Client) fetchMeta(ctx context.Context, itemURL string) (*metaEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()
	out, err := c.runYtDlp(ctx, "--dump-json", "--no-playlist", "--no-warnings",
		"--skip-download", itemURL)
	if err != nil {
		return nil, fmt.Errorf("yt-dlp 元数据失败：%v", err)
	}
	return parseMeta(out)
}

// ---- yt-dlp JSON parsing ----

// flatEntry is one --flat-playlist --dump-json line. Only the fields
// the sync uses are decoded; upload_date and description are absent in
// flat mode by design (see the package comment).
type flatEntry struct {
	id            string
	title         string
	duration      int
	liveStatus    string
	url           string
	thumbnailURL  string
	playlistTitle string
}

type flatEntryJSON struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Duration      *float64 `json:"duration"`
	LiveStatus    string   `json:"live_status"`
	WebpageURL    string   `json:"webpage_url"`
	PlaylistTitle string   `json:"playlist_title"`
	Thumbnails    []struct {
		URL string `json:"url"`
	} `json:"thumbnails"`
}

func parseFlatEntry(line []byte) (*flatEntry, error) {
	var raw flatEntryJSON
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw.ID) == "" {
		return nil, fmt.Errorf("flat entry 缺少 id")
	}
	e := &flatEntry{
		id:            strings.TrimSpace(raw.ID),
		title:         strings.TrimSpace(raw.Title),
		liveStatus:    strings.TrimSpace(raw.LiveStatus),
		url:           strings.TrimSpace(raw.WebpageURL),
		playlistTitle: strings.TrimSpace(raw.PlaylistTitle),
	}
	if raw.Duration != nil {
		e.duration = int(*raw.Duration)
	}
	// First thumbnail wins (usually the smallest/default; good enough
	// for a list card and stable across runs).
	if len(raw.Thumbnails) > 0 {
		e.thumbnailURL = strings.TrimSpace(raw.Thumbnails[0].URL)
	}
	if e.url == "" {
		e.url = "https://www.youtube.com/watch?v=" + e.id
	}
	return e, nil
}

// parseFlatOutput decodes one JSON object per line, skipping lines that
// do not parse (a partial yt-dlp run still yields its good lines). It
// also extracts the channel name from the first playlist_title.
func parseFlatOutput(out []byte, tabSlug string) ([]flatEntry, string) {
	entries := []flatEntry{}
	name := ""
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		e, err := parseFlatEntry(line)
		if err != nil {
			continue
		}
		if name == "" {
			name = channelNameFromPlaylistTitle(e.playlistTitle, tabSlug)
		}
		entries = append(entries, *e)
	}
	return entries, name
}

// channelNameFromPlaylistTitle strips the " - <Tab>" suffix from a flat
// entry's playlist_title, e.g. "Google for Developers - Videos".
func channelNameFromPlaylistTitle(playlistTitle, tabSlug string) string {
	if playlistTitle == "" {
		return ""
	}
	if suffix, ok := tabTitleSuffix[tabSlug]; ok {
		return strings.TrimSpace(strings.TrimSuffix(playlistTitle, suffix))
	}
	return playlistTitle
}

// toVideo converts a flat entry to a model row. PublishedAt stays ""
// until the backfill supplies it (playlists keep "").
func (e *flatEntry) toVideo(channelID, kind string) model.YTVideo {
	return model.YTVideo{
		ID:           e.id,
		ChannelID:    channelID,
		Kind:         kind,
		Title:        e.title,
		Duration:     e.duration,
		LiveStatus:   e.liveStatus,
		ThumbnailURL: e.thumbnailURL,
		URL:          e.url,
		CreatedAt:    time.Now().In(shanghai).Format("2006-01-02 15:04:05"),
	}
}

// metaEntry is one --dump-json single-item result.
type metaEntry struct {
	Description string   `json:"description"`
	UploadDate  string   `json:"upload_date"`
	Duration    *float64 `json:"duration"`
	LiveStatus  string   `json:"live_status"`
}

// parseMeta decodes a single-item metadata JSON object. yt-dlp prints
// exactly one object; a trailing newline or extra blank lines are
// tolerated by decoding the first object in the stream.
func parseMeta(out []byte) (*metaEntry, error) {
	dec := json.NewDecoder(bytes.NewReader(out))
	var m metaEntry
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("解析 yt-dlp 元数据失败：%v", err)
	}
	m.Description = strings.TrimSpace(m.Description)
	m.UploadDate = strings.TrimSpace(m.UploadDate)
	m.LiveStatus = strings.TrimSpace(m.LiveStatus)
	return &m, nil
}

// durationSecs returns the metadata duration in whole seconds (0 when
// absent).
func (m *metaEntry) durationSecs() int {
	if m.Duration == nil {
		return 0
	}
	return int(*m.Duration)
}

// uploadDate returns the metadata upload_date as "2006-01-02"
// (upload_date arrives as YYYYMMDD). "" when absent or malformed.
func (m *metaEntry) uploadDate() string {
	if len(m.UploadDate) != 8 {
		return ""
	}
	t, err := time.Parse("20060102", m.UploadDate)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// publishedAt converts the upload_date to Shanghai wall time at
// midnight ("2006-01-02 15:04:05"). "" when the date is unknown.
func (m *metaEntry) publishedAt() string {
	d := m.uploadDate()
	if d == "" {
		return ""
	}
	return d + " 00:00:00"
}

// inDateRange reports whether date ("2006-01-02") falls inside
// [from, to]; an empty bound means no limit, and to is inclusive of
// the whole day. An empty date (backfill supplied none) is never in
// range — the item is not stored yet.
func inDateRange(date, from, to string) bool {
	if date == "" {
		return false
	}
	if from != "" && date < from {
		return false
	}
	if to != "" && date > to {
		return false
	}
	return true
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
		// Some pages never carry a "channelId" JSON field; the ID is
		// only in URLs (canonical / og:url / RSS feed link).
		m = channelIDCanonicalRe.FindSubmatch(body)
	}
	if m == nil {
		return "", fmt.Errorf("页面中未找到 channelId")
	}
	return string(m[1]), nil
}
