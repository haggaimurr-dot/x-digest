// Package bilibili syncs Bilibili uploaders (UP主) into the same row
// shapes the YouTube page uses (2026-10-27). It talks to Bilibili's web
// JSON API directly with the standard library — no yt-dlp: the Shanghai
// server reaches bilibili.com without a proxy, and the space listing
// already carries each video's description and publish time, so there
// is no per-video metadata backfill.
//
// Anti-crawler measures handled here, as the web page does:
//   - a buvid3 device cookie from /x/frontend/finger/spi, held in a
//     cookie jar for the client's lifetime;
//   - WBI signing (wts + w_rid) for the /wbi/ endpoints, with the
//     img/sub keys read from /x/web-interface/nav and cached for an hour;
//   - the dm_img_* fingerprint parameters the space listing checks.
//
// BILI_API_BASE overrides https://api.bilibili.com (local testing only).
// Optional environment: BILIBILI_SESSDATA (a logged-in SESSDATA cookie:
// needed for AI subtitles, which feed the AI summary, and it makes
// risk-control blocks rarer) and BILI_HTTP_PROXY. Neither is ever logged
// or returned in errors.
//
// Tabs: "video" (the space upload list, newest first, paged until the
// channel's date range is covered, at most maxPages pages) and
// "playlist" (合集 seasons and 系列 series). A tab that fails does not
// remove anything already stored.
package bilibili

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"xdigest/internal/model"
	"xdigest/internal/youtube"
)

const (
	defaultAPIBase = "https://api.bilibili.com"
	browserUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	requestTimeout = 20 * time.Second
	pageSize       = 30
	maxPages       = 5
	wbiKeyTTL      = time.Hour
	maxBodyBytes   = 8 << 20
)

// mixinKeyEncTab reorders img_key+sub_key into the WBI mixin key.
var mixinKeyEncTab = []int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
	33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40, 61,
	26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11, 36,
	20, 34, 44, 52,
}

// Client calls the Bilibili web API. APIBase is overridable for tests.
type Client struct {
	HTTP     *http.Client
	APIBase  string
	SESSDATA string

	mu        sync.Mutex
	mixinKey  string
	keyAt     time.Time
	buvidDone bool
	now       func() time.Time
}

// New builds a client from BILIBILI_SESSDATA / BILI_HTTP_PROXY.
func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if p := strings.TrimSpace(os.Getenv("BILI_HTTP_PROXY")); p != "" {
		if u, err := url.Parse(p); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	base := defaultAPIBase
	if v := strings.TrimSpace(os.Getenv("BILI_API_BASE")); v != "" {
		base = v // local testing against a fake API only
	}
	return NewWith(&http.Client{Timeout: requestTimeout, Transport: transport}, base,
		strings.TrimSpace(os.Getenv("BILIBILI_SESSDATA")))
}

// NewWith builds a client around hc (a cookie jar is added when hc has
// none).
func NewWith(hc *http.Client, apiBase, sessdata string) *Client {
	if hc.Jar == nil {
		jar, _ := cookiejar.New(nil)
		hc.Jar = jar
	}
	return &Client{HTTP: hc, APIBase: strings.TrimSuffix(apiBase, "/"), SESSDATA: sessdata, now: time.Now}
}

// apiError is a non-zero Bilibili "code".
type apiError struct {
	Code int
	Msg  string
}

func (e *apiError) Error() string {
	switch e.Code {
	case -352, -412, -509, -799:
		return fmt.Sprintf("B站风控拦截（%d %s），稍后重试；频繁出现可在服务器配置 BILIBILI_SESSDATA", e.Code, e.Msg)
	case -404, 62002, 62004, 62012:
		return fmt.Sprintf("内容不存在或不可见（%d %s）", e.Code, e.Msg)
	}
	return fmt.Sprintf("B站接口错误（%d %s）", e.Code, e.Msg)
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// get fetches path?params (signed when wbi) and decodes data into out.
func (c *Client) get(ctx context.Context, path string, params url.Values, wbi bool, out any) error {
	if err := c.ensureBuvid(ctx); err != nil {
		return err
	}
	if params == nil {
		params = url.Values{}
	}
	if wbi {
		key, err := c.wbiKey(ctx)
		if err != nil {
			return err
		}
		params = signWBI(params, key, c.now().Unix())
	}
	u := c.APIBase + path
	if len(params) > 0 {
		u += "?" + encodeQuery(params)
	}
	var env envelope
	if err := c.getJSON(ctx, u, &env); err != nil {
		return err
	}
	if env.Code != 0 {
		return &apiError{Code: env.Code, Msg: env.Message}
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

func (c *Client) getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Origin", "https://www.bilibili.com")
	if c.SESSDATA != "" {
		req.AddCookie(&http.Cookie{Name: "SESSDATA", Value: c.SESSDATA})
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("请求 B站失败：%w", scrubErr(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusPreconditionFailed {
			return &apiError{Code: -412, Msg: "request blocked"}
		}
		return fmt.Errorf("B站 HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("B站返回的不是 JSON（可能被风控拦截）")
	}
	return nil
}

// scrubErr drops the URL from transport errors (it can carry signed
// query strings; nothing secret, but noisy).
func scrubErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// ensureBuvid fetches the buvid3/buvid4 device cookies once.
func (c *Client) ensureBuvid(ctx context.Context) error {
	c.mu.Lock()
	done := c.buvidDone
	c.mu.Unlock()
	if done {
		return nil
	}
	var env envelope
	if err := c.getJSON(ctx, c.APIBase+"/x/frontend/finger/spi", &env); err != nil {
		return err
	}
	var d struct {
		B3 string `json:"b_3"`
		B4 string `json:"b_4"`
	}
	_ = json.Unmarshal(env.Data, &d)
	if d.B3 != "" {
		if u, err := url.Parse(c.APIBase); err == nil {
			cookies := []*http.Cookie{{Name: "buvid3", Value: d.B3, Path: "/"}}
			if d.B4 != "" {
				cookies = append(cookies, &http.Cookie{Name: "buvid4", Value: d.B4, Path: "/"})
			}
			c.HTTP.Jar.SetCookies(u, cookies)
		}
	}
	c.mu.Lock()
	c.buvidDone = true
	c.mu.Unlock()
	return nil
}

// wbiKey returns the cached mixin key, refreshing it from nav hourly.
// nav answers code -101 (not logged in) but still carries wbi_img.
func (c *Client) wbiKey(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.mixinKey != "" && c.now().Sub(c.keyAt) < wbiKeyTTL {
		k := c.mixinKey
		c.mu.Unlock()
		return k, nil
	}
	c.mu.Unlock()
	var env struct {
		Data struct {
			WbiImg struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, c.APIBase+"/x/web-interface/nav", &env); err != nil {
		return "", err
	}
	img, sub := keyFromURL(env.Data.WbiImg.ImgURL), keyFromURL(env.Data.WbiImg.SubURL)
	if img == "" || sub == "" {
		return "", errors.New("读取 B站 WBI 签名密钥失败")
	}
	key := MixinKey(img, sub)
	c.mu.Lock()
	c.mixinKey, c.keyAt = key, c.now()
	c.mu.Unlock()
	return key, nil
}

func keyFromURL(u string) string {
	i := strings.LastIndex(u, "/")
	name := u[i+1:]
	if j := strings.Index(name, "."); j >= 0 {
		name = name[:j]
	}
	return name
}

// MixinKey derives the 32-char WBI mixin key.
func MixinKey(imgKey, subKey string) string {
	raw := imgKey + subKey
	var b strings.Builder
	for _, i := range mixinKeyEncTab {
		if i < len(raw) {
			b.WriteByte(raw[i])
		}
	}
	s := b.String()
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

var wbiStrip = strings.NewReplacer("!", "", "'", "", "(", "", ")", "", "*", "")

// signWBI adds wts and w_rid to params (a copy).
func signWBI(params url.Values, mixinKey string, wts int64) url.Values {
	out := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			out.Add(k, wbiStrip.Replace(v))
		}
	}
	out.Set("wts", strconv.FormatInt(wts, 10))
	sum := md5.Sum([]byte(encodeQuery(out) + mixinKey))
	out.Set("w_rid", hex.EncodeToString(sum[:]))
	return out
}

// encodeQuery is url.Values.Encode (sorted keys) with %20 for spaces,
// matching the browser's encodeURIComponent that WBI hashes over.
func encodeQuery(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		for _, val := range v[k] {
			parts = append(parts, strings.ReplaceAll(url.QueryEscape(k), "+", "%20")+"="+strings.ReplaceAll(url.QueryEscape(val), "+", "%20"))
		}
	}
	return strings.Join(parts, "&")
}

// ---- resolve ----

var (
	midRe      = regexp.MustCompile(`^\d{1,20}$`)
	spaceURLRe = regexp.MustCompile(`(?:space\.bilibili\.com/|bilibili\.com/space/)(\d{1,20})`)
	uidRe      = regexp.MustCompile(`(?i)^uid[:：\s]*(\d{1,20})$`)
	// b23.tv short links, e.g. https://b23.tv/Jmkrb3h (usually a shared
	// space link). Resolved via a single no-follow request in Resolve.
	b23Re = regexp.MustCompile(`(?i)(?:https?://)?b23\.tv/([A-Za-z0-9]+)`)
)

// ParseMid extracts a numeric mid from "123", "UID:123" or a
// space.bilibili.com URL.
func ParseMid(input string) (string, bool) {
	s := strings.TrimSpace(input)
	if midRe.MatchString(s) {
		return s, true
	}
	if m := uidRe.FindStringSubmatch(s); m != nil {
		return m[1], true
	}
	if m := spaceURLRe.FindStringSubmatch(s); m != nil {
		return m[1], true
	}
	return "", false
}

// resolveB23 follows a b23.tv short link with a single request that does
// NOT follow redirects, and returns the redirect target (normally a
// https://space.bilibili.com/<UID> or m.bilibili.com/space/<UID> URL).
// Returns ("", nil) when the input is not a b23.tv link.
func resolveB23(ctx context.Context, input string) (string, error) {
	m := b23Re.FindStringSubmatch(strings.TrimSpace(input))
	if m == nil {
		return "", nil
	}
	target := m[0]
	if !strings.HasPrefix(strings.ToLower(target), "http") {
		target = "https://" + target
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10) AppleWebKit/537.36")
	noFollow := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 15 * time.Second,
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "" {
		return loc, nil
	}
	return "", fmt.Errorf("b23.tv 短链没有返回跳转目标")
}

// Resolve checks the uploader exists and returns its mid and name.
func (c *Client) Resolve(ctx context.Context, input string) (mid, name string, err error) {
	s := strings.TrimSpace(input)
	if b23Re.MatchString(s) {
		resolved, rerr := resolveB23(ctx, s)
		if rerr != nil {
			return "", "", fmt.Errorf("b23.tv 短链解析失败：%v", rerr)
		}
		s = resolved
	}
	mid, ok := ParseMid(s)
	if !ok {
		return "", "", errors.New("请输入 B站 UID（数字）、个人空间链接 https://space.bilibili.com/<UID> 或 b23.tv 短链")
	}
	var d struct {
		Card struct {
			Mid  string `json:"mid"`
			Name string `json:"name"`
		} `json:"card"`
	}
	if err := c.get(ctx, "/x/web-interface/card", url.Values{"mid": {mid}}, false, &d); err != nil {
		return "", "", err
	}
	if d.Card.Name == "" {
		return "", "", fmt.Errorf("找不到 UID %s 对应的 UP 主", mid)
	}
	return mid, d.Card.Name, nil
}

// ---- sync ----

// space listing fingerprint parameters (what the web page sends).
var spaceFingerprint = url.Values{
	"dm_img_list":      {"[]"},
	"dm_img_str":       {"V2ViR0wgMS4wIChPcGVuR0wgRVMgMi4wIENocm9taXVtKQ"},
	"dm_cover_img_str": {"QU5HTEUgKEludGVsLCBJbnRlbChSKSBVSEQgR3JhcGhpY3MgRGlyZWN0M0QxMSB2c181XzAgcHNfNV8wKSwgb3IgZXF1aXZhbGVudClHb29nbGUgSW5jLiAoSW50ZWwp"},
	"dm_img_inter":     {`{"ds":[],"wh":[0,0,0],"of":[0,0,0]}`},
}

type arcItem struct {
	Aid         int64  `json:"aid"`
	Bvid        string `json:"bvid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Created     int64  `json:"created"`
	Length      string `json:"length"`
	Pic         string `json:"pic"`
	Author      string `json:"author"`
}

var shanghai = time.FixedZone("CST", 8*3600)

// Sync lists one uploader's videos and collections. known holds IDs
// already stored complete; they are still returned (cheap: no extra
// request per video) so titles/covers stay fresh.
func (c *Client) Sync(ctx context.Context, mid, dateFrom, dateTo string, _ map[string]bool) (*youtube.SyncResult, error) {
	res := &youtube.SyncResult{Status: "synced", Videos: []model.YTVideo{}}
	vids, name, err := c.listVideos(ctx, mid, dateFrom, dateTo)
	if err != nil {
		res.FailedTabs = append(res.FailedTabs, "video: "+err.Error())
	} else {
		res.SyncedTabs++
		res.ChannelName = name
		res.Videos = append(res.Videos, vids...)
	}
	lists, err := c.listCollections(ctx, mid)
	if err != nil {
		res.FailedTabs = append(res.FailedTabs, "playlist: "+err.Error())
	} else {
		res.SyncedTabs++
		res.Videos = append(res.Videos, lists...)
	}
	if res.SyncedTabs == 0 {
		res.Status = "failed"
		res.Error = strings.Join(res.FailedTabs, "；")
		res.Videos = nil
	}
	return res, nil
}

func (c *Client) listVideos(ctx context.Context, mid, dateFrom, dateTo string) ([]model.YTVideo, string, error) {
	var out []model.YTVideo
	name := ""
	for pn := 1; pn <= maxPages; pn++ {
		params := url.Values{"mid": {mid}, "ps": {strconv.Itoa(pageSize)}, "pn": {strconv.Itoa(pn)},
			"order": {"pubdate"}, "platform": {"web"}, "web_location": {"1550101"}}
		for k, v := range spaceFingerprint {
			params[k] = v
		}
		var d struct {
			List struct {
				Vlist []arcItem `json:"vlist"`
			} `json:"list"`
			Page struct {
				Count int `json:"count"`
			} `json:"page"`
		}
		if err := c.get(ctx, "/x/space/wbi/arc/search", params, true, &d); err != nil {
			if pn > 1 {
				break // keep what earlier pages gave
			}
			return nil, "", err
		}
		olderThanRange := false
		for _, it := range d.List.Vlist {
			if name == "" {
				name = it.Author
			}
			published := time.Unix(it.Created, 0).In(shanghai)
			day := published.Format("2006-01-02")
			if dateFrom != "" && day < dateFrom {
				olderThanRange = true
				continue
			}
			if dateTo != "" && day > dateTo {
				continue
			}
			out = append(out, model.YTVideo{
				ID: it.Bvid, ChannelID: mid, Kind: "video", Title: it.Title,
				Description: strings.TrimSpace(it.Description), PublishedAt: published.Format("2006-01-02 15:04:05"),
				Duration: parseLength(it.Length), ThumbnailURL: httpsURL(it.Pic),
				URL: "https://www.bilibili.com/video/" + it.Bvid, CreatedAt: nowString(),
			})
		}
		// Stop when the list is exhausted, when the range start is
		// reached, or (no range) after the first page — 30 newest
		// videos, the same depth the YouTube tabs use.
		if len(d.List.Vlist) < pageSize || olderThanRange || dateFrom == "" || pn*pageSize >= d.Page.Count {
			break
		}
	}
	return out, name, nil
}

func (c *Client) listCollections(ctx context.Context, mid string) ([]model.YTVideo, error) {
	type meta struct {
		SeasonID    int64  `json:"season_id"`
		SeriesID    int64  `json:"series_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Cover       string `json:"cover"`
		Total       int    `json:"total"`
	}
	var d struct {
		ItemsLists struct {
			Seasons []struct {
				Meta meta `json:"meta"`
			} `json:"seasons_list"`
			Series []struct {
				Meta meta `json:"meta"`
			} `json:"series_list"`
		} `json:"items_lists"`
	}
	params := url.Values{"mid": {mid}, "page_num": {"1"}, "page_size": {"20"}, "web_location": {"333.999"}}
	if err := c.get(ctx, "/x/polymer/web-space/seasons_series_list", params, true, &d); err != nil {
		return nil, err
	}
	var out []model.YTVideo
	add := func(id, typ string, m meta) {
		desc := strings.TrimSpace(m.Description)
		if m.Total > 0 {
			count := fmt.Sprintf("共 %d 个视频", m.Total)
			if desc == "" {
				desc = count
			} else {
				desc = count + "\n" + desc
			}
		}
		out = append(out, model.YTVideo{
			ID: typ + "-" + id, ChannelID: mid, Kind: "playlist", Title: m.Name, Description: desc,
			ThumbnailURL: httpsURL(m.Cover), URL: fmt.Sprintf("https://space.bilibili.com/%s/lists/%s?type=%s", mid, id, typ),
			CreatedAt: nowString(),
		})
	}
	for _, s := range d.ItemsLists.Seasons {
		add(strconv.FormatInt(s.Meta.SeasonID, 10), "season", s.Meta)
	}
	for _, s := range d.ItemsLists.Series {
		add(strconv.FormatInt(s.Meta.SeriesID, 10), "series", s.Meta)
	}
	return out, nil
}

func nowString() string { return time.Now().In(shanghai).Format("2006-01-02 15:04:05") }

// parseLength turns "12:34" / "1:02:03" into seconds.
func parseLength(s string) int {
	total := 0
	for _, p := range strings.Split(strings.TrimSpace(s), ":") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}

func httpsURL(u string) string {
	u = strings.TrimSpace(u)
	switch {
	case strings.HasPrefix(u, "//"):
		return "https:" + u
	case strings.HasPrefix(u, "http://"):
		return "https://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

// ---- detail (AI summary input) ----

type viewData struct {
	Aid      int64  `json:"aid"`
	Bvid     string `json:"bvid"`
	Cid      int64  `json:"cid"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Duration int    `json:"duration"`
}

func (c *Client) view(ctx context.Context, bvid string) (*viewData, error) {
	var v viewData
	if err := c.get(ctx, "/x/web-interface/view", url.Values{"bvid": {bvid}}, false, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// FetchDetail returns title, description, chapters (高能进度条 / 分段
// view_points) and the subtitle of a video's first part. Human subtitles
// are preferred over AI ones, Chinese over other languages; AI subtitles
// are only listed to logged-in requests (BILIBILI_SESSDATA). No subtitle
// is not an error.
func (c *Client) FetchDetail(ctx context.Context, bvid string) (*youtube.VideoDetail, error) {
	v, err := c.view(ctx, bvid)
	if err != nil {
		return nil, err
	}
	d := &youtube.VideoDetail{Title: v.Title, Description: strings.TrimSpace(v.Desc), Duration: v.Duration, Language: "zh-CN"}
	var p struct {
		Subtitle struct {
			Subtitles []struct {
				Lan         string `json:"lan"`
				SubtitleURL string `json:"subtitle_url"`
				AIType      int    `json:"ai_type"`
				Type        int    `json:"type"`
			} `json:"subtitles"`
		} `json:"subtitle"`
		ViewPoints []struct {
			Content string  `json:"content"`
			From    float64 `json:"from"`
			To      float64 `json:"to"`
		} `json:"view_points"`
	}
	params := url.Values{"aid": {strconv.FormatInt(v.Aid, 10)}, "cid": {strconv.FormatInt(v.Cid, 10)}}
	if err := c.get(ctx, "/x/player/wbi/v2", params, true, &p); err != nil {
		return d, nil // detail without subtitles/chapters still summarizes
	}
	for _, vp := range p.ViewPoints {
		if strings.TrimSpace(vp.Content) != "" {
			d.Chapters = append(d.Chapters, youtube.Chapter{Start: vp.From, End: vp.To, Title: strings.TrimSpace(vp.Content)})
		}
	}
	best, bestScore := -1, -1
	for i, s := range p.Subtitle.Subtitles {
		if s.SubtitleURL == "" {
			continue
		}
		score := 0
		if strings.HasPrefix(s.Lan, "zh") || strings.HasPrefix(s.Lan, "ai-zh") {
			score += 2
		}
		if !strings.HasPrefix(s.Lan, "ai-") && s.AIType == 0 {
			score += 1
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return d, nil
	}
	sub := p.Subtitle.Subtitles[best]
	var body struct {
		Body []struct {
			From    float64 `json:"from"`
			Content string  `json:"content"`
		} `json:"body"`
	}
	if err := c.getJSON(ctx, httpsURL(sub.SubtitleURL), &body); err != nil {
		return d, nil
	}
	for _, line := range body.Body {
		if t := strings.TrimSpace(line.Content); t != "" {
			d.Segments = append(d.Segments, youtube.Segment{Start: line.From, Text: t})
		}
	}
	if len(d.Segments) > 0 {
		d.TranscriptLang = sub.Lan
		d.TranscriptAuto = strings.HasPrefix(sub.Lan, "ai-") || sub.AIType != 0
	}
	return d, nil
}
