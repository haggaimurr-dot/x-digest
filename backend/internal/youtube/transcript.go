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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
