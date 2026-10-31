package youtube

// Transcript fetch for the AI summary (2026-10-27).
//
// Two yt-dlp calls per video, both through the same runner (and so the
// same YT_HTTP_PROXY) as the sync:
//
//  1. `--dump-json --skip-download` for the metadata: title, description,
//     duration, language, the creator's chapters, and which subtitle
//     tracks exist (manual "subtitles" and "automatic_captions").
//  2. `--skip-download --write-subs|--write-auto-subs --sub-langs <one>
//     --sub-format vtt` into a temp dir, for exactly ONE track picked
//     from step 1. Asking for a single language matters: every video
//     lists ~100 machine-translated auto-caption languages, and pulling
//     many of them in one run is what gets a box rate-limited (HTTP 429).
//
// Track preference: a manual track in the video's own language, then a
// manual Chinese/English track, then the auto track in the video's own
// language ("<lang>-orig" first: it is the raw speech recognition, not a
// translation), then auto English. No track at all is not an error: the
// summary then falls back to title + description and says so.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"xdigest/internal/model"
)

// Chapter is one creator-defined chapter (seconds from the start).
type Chapter struct {
	Start float64 `json:"start_time"`
	End   float64 `json:"end_time"`
	Title string  `json:"title"`
}

// Segment is one de-duplicated subtitle line with its start time.
type Segment struct {
	Start float64
	Text  string
}

// VideoDetail is what the summary needs about one video.
type VideoDetail struct {
	Title       string
	Description string
	Duration    int
	Language    string
	Chapters    []Chapter
	Segments    []Segment
	// TranscriptLang is the picked track's language code ("" = none);
	// TranscriptAuto is true for an automatic (speech recognition) track.
	TranscriptLang string
	TranscriptAuto bool
}

type detailMeta struct {
	Title             string                     `json:"title"`
	Description       string                     `json:"description"`
	Duration          *float64                   `json:"duration"`
	Language          string                     `json:"language"`
	Chapters          []Chapter                  `json:"chapters"`
	Subtitles         map[string]json.RawMessage `json:"subtitles"`
	AutomaticCaptions map[string]json.RawMessage `json:"automatic_captions"`
}

// FetchDetail returns metadata, chapters and (when a track exists) the
// transcript of one video. A failed subtitle download is not an error:
// the detail comes back without segments.
func (c *Client) FetchDetail(ctx context.Context, videoURL string) (*VideoDetail, error) {
	out, err := c.runYtDlp(ctx, "--dump-json", "--no-playlist", "--no-warnings", "--skip-download", videoURL)
	if len(out) == 0 {
		return nil, fmt.Errorf("yt-dlp 元数据失败：%v", err)
	}
	var m detailMeta
	if derr := json.NewDecoder(bytes.NewReader(out)).Decode(&m); derr != nil {
		return nil, fmt.Errorf("解析 yt-dlp 元数据失败：%v", derr)
	}
	d := &VideoDetail{
		Title:       strings.TrimSpace(m.Title),
		Description: strings.TrimSpace(m.Description),
		Language:    strings.TrimSpace(m.Language),
		Chapters:    m.Chapters,
	}
	if m.Duration != nil {
		d.Duration = int(*m.Duration)
	}
	lang, auto, ok := pickTrack(m.Language, keys(m.Subtitles), keys(m.AutomaticCaptions))
	if !ok {
		return d, nil
	}
	segs, err := c.downloadTrack(ctx, videoURL, lang, auto)
	if err != nil || len(segs) == 0 {
		return d, nil
	}
	d.Segments, d.TranscriptLang, d.TranscriptAuto = segs, lang, auto
	return d, nil
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "live_chat" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// pickTrack chooses one subtitle track (see the file comment for the
// order). It returns ok=false when nothing usable exists.
func pickTrack(videoLang string, manual, auto []string) (lang string, isAuto bool, ok bool) {
	has := func(list []string, l string) bool {
		for _, x := range list {
			if x == l {
				return true
			}
		}
		return false
	}
	// prefix match: "en" matches "en-US"; returns the first hit.
	withPrefix := func(list []string, p string) string {
		if p == "" {
			return ""
		}
		for _, x := range list {
			if x == p || strings.HasPrefix(x, p+"-") {
				return x
			}
		}
		return ""
	}
	base := strings.SplitN(videoLang, "-", 2)[0]
	if l := withPrefix(manual, videoLang); l != "" {
		return l, false, true
	}
	if l := withPrefix(manual, base); l != "" {
		return l, false, true
	}
	for _, p := range []string{"zh-Hans", "zh-CN", "zh", "en"} {
		if l := withPrefix(manual, p); l != "" {
			return l, false, true
		}
	}
	if len(manual) > 0 && len(auto) == 0 {
		return manual[0], false, true
	}
	for _, l := range []string{videoLang + "-orig", base + "-orig", videoLang, base, "en-orig", "en"} {
		if l != "" && l != "-orig" && has(auto, l) {
			return l, true, true
		}
	}
	if len(manual) > 0 {
		return manual[0], false, true
	}
	return "", false, false
}

// downloadTrack writes one VTT track to a temp dir and parses it.
func (c *Client) downloadTrack(ctx context.Context, videoURL, lang string, auto bool) ([]Segment, error) {
	dir, err := os.MkdirTemp("", "xdigest-subs-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	flag := "--write-subs"
	if auto {
		flag = "--write-auto-subs"
	}
	_, runErr := c.runYtDlp(ctx, "--skip-download", "--no-playlist", "--no-warnings", flag,
		"--sub-langs", lang, "--sub-format", "vtt", "-o", filepath.Join(dir, "sub.%(ext)s"), videoURL)
	matches, _ := filepath.Glob(filepath.Join(dir, "*.vtt"))
	if len(matches) == 0 {
		if runErr != nil {
			return nil, runErr
		}
		return nil, fmt.Errorf("yt-dlp 未写出字幕文件")
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return nil, err
	}
	return ParseVTT(raw), nil
}

var (
	vttTimeRe = regexp.MustCompile(`^(\d+:)?(\d{1,2}):(\d{2})[.,](\d{3})\s+-->`)
	vttTagRe  = regexp.MustCompile(`<[^>]*>`)
)

// ParseVTT turns a WebVTT file into de-duplicated lines. YouTube's auto
// captions "roll": each cue repeats the previous line above the new
// one, so a line already emitted recently is skipped.
func ParseVTT(raw []byte) []Segment {
	var out []Segment
	recent := make([]string, 0, 4)
	seen := func(s string) bool {
		for _, r := range recent {
			if r == s {
				return true
			}
		}
		return false
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var cueStart float64
	inCue := false
	for sc.Scan() {
		raw := strings.TrimRight(sc.Text(), "\r")
		line := strings.TrimSpace(raw)
		if m := vttTimeRe.FindStringSubmatch(line); m != nil {
			cueStart = vttSeconds(m)
			inCue = true
			continue
		}
		// Only a truly empty line ends a cue: YouTube's auto captions
		// put a line holding a single space right after the timing.
		if raw == "" {
			inCue = false
			continue
		}
		if !inCue {
			continue // header, NOTE/STYLE blocks, cue identifiers
		}
		text := strings.TrimSpace(htmlUnescape(vttTagRe.ReplaceAllString(line, "")))
		if text == "" || seen(text) {
			continue
		}
		out = append(out, Segment{Start: cueStart, Text: text})
		recent = append(recent, text)
		if len(recent) > 3 {
			recent = recent[1:]
		}
	}
	return out
}

func vttSeconds(m []string) float64 {
	h := 0
	if m[1] != "" {
		h, _ = strconv.Atoi(strings.TrimSuffix(m[1], ":"))
	}
	mi, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	ms, _ := strconv.Atoi(m[4])
	return float64(h*3600+mi*60+s) + float64(ms)/1000
}

var htmlEntities = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&nbsp;", " ")

func htmlUnescape(s string) string { return htmlEntities.Replace(s) }

// ---- Single video (2026-10-27) ----

// SingleVideo is one video looked up by URL, with the channel it belongs to.
type SingleVideo struct {
	Video       model.YTVideo
	ChannelID   string
	ChannelName string
	Handle      string
}

type singleMeta struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	UploadDate  string   `json:"upload_date"`
	Duration    *float64 `json:"duration"`
	Thumbnail   string   `json:"thumbnail"`
	ChannelID   string   `json:"channel_id"`
	Channel     string   `json:"channel"`
	UploaderID  string   `json:"uploader_id"`
	WebpageURL  string   `json:"webpage_url"`
	OriginalURL string   `json:"original_url"`
	LiveStatus  string   `json:"live_status"`
	Type        string   `json:"_type"`
}

// FetchVideo looks up one video (watch / youtu.be / shorts / live URL or a
// bare video id) with a single yt-dlp metadata call. Playlists are refused.
func (c *Client) FetchVideo(ctx context.Context, input string) (*SingleVideo, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return nil, fmt.Errorf("请输入视频链接")
	}
	if !strings.Contains(in, "/") && len(in) == 11 {
		in = "https://www.youtube.com/watch?v=" + in
	}
	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()
	out, err := c.runYtDlp(ctx, "--dump-json", "--no-playlist", "--no-warnings", "--skip-download", in)
	if len(out) == 0 {
		// YouTube bot detection ("Sign in to confirm you're not a bot") blocks
		// yt-dlp. Fall back to oEmbed for basic metadata (no auth needed).
		if err != nil && strings.Contains(err.Error(), "not a bot") {
			if sv, ferr := c.fetchVideoOEmbed(ctx, in); ferr == nil {
				return sv, nil
			}
		}
		return nil, fmt.Errorf("读取视频失败（链接无效或视频不可见）：%v", err)
	}
	var m singleMeta
	if derr := json.NewDecoder(bytes.NewReader(out)).Decode(&m); derr != nil {
		return nil, fmt.Errorf("解析 yt-dlp 元数据失败：%v", derr)
	}
	if m.Type == "playlist" || m.ID == "" {
		return nil, fmt.Errorf("这是播放列表或频道链接，请填写单个视频的链接")
	}
	if m.ChannelID == "" {
		return nil, fmt.Errorf("读不到视频所属的频道")
	}
	kind := "video"
	switch {
	case strings.Contains(m.OriginalURL+" "+m.WebpageURL+" "+input, "/shorts/"):
		kind = "short"
	case m.LiveStatus == "is_live" || m.LiveStatus == "was_live" || m.LiveStatus == "post_live" || m.LiveStatus == "is_upcoming":
		kind = "live"
	}
	meta := metaEntry{UploadDate: m.UploadDate}
	v := model.YTVideo{
		ID: m.ID, ChannelID: m.ChannelID, Kind: kind, Title: strings.TrimSpace(m.Title),
		Description: strings.TrimSpace(m.Description), PublishedAt: meta.publishedAt(),
		LiveStatus: strings.TrimSpace(m.LiveStatus), ThumbnailURL: m.Thumbnail,
		URL:       "https://www.youtube.com/watch?v=" + m.ID,
		CreatedAt: time.Now().In(shanghai).Format("2006-01-02 15:04:05"),
	}
	if kind == "short" {
		v.URL = "https://www.youtube.com/shorts/" + m.ID
	}
	if m.Duration != nil {
		v.Duration = int(*m.Duration)
	}
	handle := strings.TrimSpace(m.UploaderID)
	if handle != "" && !strings.HasPrefix(handle, "@") {
		handle = ""
	}
	return &SingleVideo{Video: v, ChannelID: m.ChannelID, ChannelName: strings.TrimSpace(m.Channel), Handle: handle}, nil
}

// oembedResponse is the subset of YouTube oEmbed we use for the bot-block fallback.
type oembedResponse struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	AuthorURL    string `json:"author_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// fetchVideoOEmbed fetches basic video metadata via YouTube's oEmbed endpoint.
// Used when yt-dlp is blocked by bot detection. Returns title, author name,
// thumbnail and the @handle; description/duration/publish date are unavailable.
func (c *Client) fetchVideoOEmbed(ctx context.Context, videoURL string) (*SingleVideo, error) {
	// Extract video ID from URL.
	vid := ""
	if m := regexp.MustCompile(`(?:v=|youtu\.be/|/shorts/|/embed/|/live/)([A-Za-z0-9_-]{11})`).FindStringSubmatch(videoURL); m != nil {
		vid = m[1]
	} else if len(videoURL) == 11 {
		vid = videoURL
	}
	if vid == "" {
		return nil, fmt.Errorf("无法从链接提取视频 ID")
	}
	apiURL := "https://www.youtube.com/oembed?url=" + url.QueryEscape("https://www.youtube.com/watch?v="+vid) + "&format=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; x-digest/1.0)")
	client := &http.Client{Timeout: 15 * time.Second}
	// Use proxy if configured (same as yt-dlp).
	if proxy := c.proxyURL(); proxy != "" {
		if pu, perr := url.Parse(proxy); perr == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(pu)}
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oEmbed 请求失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("oEmbed 返回 HTTP %d", resp.StatusCode)
	}
	var oe oembedResponse
	if err := json.NewDecoder(resp.Body).Decode(&oe); err != nil {
		return nil, fmt.Errorf("解析 oEmbed 失败：%v", err)
	}
	if oe.Title == "" {
		return nil, fmt.Errorf("oEmbed 未返回标题")
	}
	// Extract @handle from author_url (https://www.youtube.com/@handle).
	handle := ""
	channelID := ""
	if m := regexp.MustCompile(`youtube\.com/(@[^/]+)`).FindStringSubmatch(oe.AuthorURL); m != nil {
		handle = m[1]
		channelID = handle // use handle as the channel identifier
	}
	if channelID == "" {
		// Fall back to author name as identifier.
		channelID = "oembed_" + oe.AuthorName
	}
	v := model.YTVideo{
		ID:           vid,
		ChannelID:    channelID,
		Kind:         "video",
		Title:        strings.TrimSpace(oe.Title),
		ThumbnailURL: oe.ThumbnailURL,
		URL:          "https://www.youtube.com/watch?v=" + vid,
		CreatedAt:    time.Now().In(shanghai).Format("2006-01-02 15:04:05"),
	}
	// Detect shorts from the original URL.
	if strings.Contains(videoURL, "/shorts/") {
		v.Kind = "short"
		v.URL = "https://www.youtube.com/shorts/" + vid
	}
	return &SingleVideo{
		Video:       v,
		ChannelID:   channelID,
		ChannelName: strings.TrimSpace(oe.AuthorName),
		Handle:      handle,
	}, nil
}
