// Package ytsummary builds the AI summary of one YouTube video: a
// full-text summary, key points, and chapters with start times
// (2026-10-27).
//
// Input is the video's transcript (see youtube.FetchDetail) plus title,
// description and the creator's own chapters when it has them. The
// model is whatever chat relay translation uses (TRANSLATE_*), called
// through the Completer interface so tests can fake it.
//
// Long transcripts are summarized in two passes: each ~20k-character
// slice is first condensed into timestamped notes, then the notes (in
// place of the raw transcript) go into the final call. Short ones go
// straight into the final call.
//
// Chapters: when the creator defined chapters, the model must keep them
// (same count, order and start times; it only translates titles and
// writes a summary per chapter) and the start times are forced back to
// the creator's afterwards. Otherwise the model proposes chapters from
// the transcript's [mm:ss] marks. Every start time is validated (inside
// the video, ascending, no duplicates) before it is stored.
package ytsummary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"xdigest/internal/youtube"
)

// Completer is one chat completion (translate.Client implements it).
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

const (
	// directLimit: transcripts up to this many characters go straight
	// into the final call; longer ones are condensed slice by slice.
	directLimit = 24000
	sliceChars  = 20000
	// bucketSecs groups subtitle lines into one "[mm:ss] ..." line.
	bucketSecs = 30
	// maxDescChars caps the description sent to the model.
	maxDescChars = 3000
)

// Source values: what the summary was based on.
const (
	SourceTranscript  = "transcript"
	SourceDescription = "description"
)

// Chapter is one summary chapter. Start is whole seconds.
type Chapter struct {
	Start   int    `json:"start"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// Result is a finished summary.
type Result struct {
	Summary   string    `json:"summary"`
	KeyPoints []string  `json:"key_points"`
	Chapters  []Chapter `json:"chapters"`
	Source    string    `json:"source"`
}

// Summarize builds the summary of d in the target language ("zh-CN" or
// "en").
func Summarize(ctx context.Context, llm Completer, d *youtube.VideoDetail, target string) (*Result, error) {
	lang := "Simplified Chinese"
	if target == "en" {
		lang = "English"
	}
	transcript := FormatTranscript(d.Segments)
	source := SourceTranscript
	if strings.TrimSpace(transcript) == "" {
		source = SourceDescription
	}
	material := transcript
	materialName := "Transcript (each line starts with its [mm:ss] or [h:mm:ss] time)"
	if utf8.RuneCountInString(transcript) > directLimit {
		notes, err := condense(ctx, llm, transcript, lang)
		if err != nil {
			return nil, err
		}
		material = notes
		materialName = "Notes condensed from the full transcript (each line starts with its time)"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", d.Title)
	if d.Duration > 0 {
		fmt.Fprintf(&b, "Duration: %s\n", Clock(d.Duration))
	}
	if desc := truncateRunes(d.Description, maxDescChars); desc != "" {
		fmt.Fprintf(&b, "\nDescription:\n%s\n", desc)
	}
	if len(d.Chapters) > 0 {
		b.WriteString("\nCreator chapters (keep exactly these: same count, order and start seconds):\n")
		for _, c := range d.Chapters {
			fmt.Fprintf(&b, "- start=%d [%s] %s\n", int(c.Start), Clock(int(c.Start)), c.Title)
		}
	}
	if material != "" {
		fmt.Fprintf(&b, "\n%s:\n%s\n", materialName, material)
	} else {
		b.WriteString("\n(No transcript is available. Summarize from the title and description only, and do not invent details.)\n")
	}

	chapterRule := "Propose 4-12 chapters that follow the video's structure. Each start must be a time (in seconds) that appears in the transcript; the first chapter starts at 0."
	switch {
	case len(d.Chapters) > 0:
		chapterRule = "Return exactly the creator chapters given, in the same order with the same start seconds; translate each title and write its summary."
	case material == "":
		chapterRule = "Return an empty chapters array (there is no transcript to place chapters in)."
	}
	system := "You summarize YouTube videos for a reader who has not watched them. Write everything in " + lang + ". " +
		"Return ONLY a JSON object, no markdown fences, with this shape: " +
		`{"summary": string, "key_points": [string], "chapters": [{"start": integer seconds, "title": string, "summary": string}]}. ` +
		"summary: the whole video in 2-5 short paragraphs separated by \\n\\n — what it is about, the main arguments or steps, and the conclusion. " +
		"key_points: 3-8 concrete takeaways (names, numbers, tools, steps), one sentence each. " +
		"Each chapter summary: 1-3 sentences on what that part covers. " + chapterRule + " " +
		"Base everything on the material given; do not invent facts. Ignore sponsor reads and calls to subscribe."

	out, err := llm.Complete(ctx, system, b.String())
	if err != nil {
		return nil, err
	}
	res, err := parseResult(out)
	if err != nil {
		return nil, err
	}
	res.Source = source
	res.Chapters = cleanChapters(res.Chapters, d)
	return res, nil
}

// condense turns a long transcript into timestamped notes, one model
// call per slice (sequential: keeps the relay load to one call at a
// time per video).
func condense(ctx context.Context, llm Completer, transcript, lang string) (string, error) {
	slices := splitLines(transcript, sliceChars)
	system := "You condense one part of a video transcript into notes, in " + lang + ". " +
		"Output 10-30 lines, each starting with the [mm:ss] (or [h:mm:ss]) time where the point is made, " +
		"followed by the point: claims, steps, names, numbers. Keep the original times. No other text."
	var notes []string
	for i, s := range slices {
		user := fmt.Sprintf("Part %d of %d:\n%s", i+1, len(slices), s)
		out, err := llm.Complete(ctx, system, user)
		if err != nil {
			return "", fmt.Errorf("分段摘要 %d/%d 失败：%w", i+1, len(slices), err)
		}
		notes = append(notes, strings.TrimSpace(out))
	}
	return strings.Join(notes, "\n"), nil
}

// FormatTranscript groups segments into one line per bucketSecs window:
// "[mm:ss] text text ...".
func FormatTranscript(segs []youtube.Segment) string {
	var b strings.Builder
	bucket := -1
	for _, s := range segs {
		t := strings.TrimSpace(s.Text)
		if t == "" {
			continue
		}
		start := int(s.Start)
		if bk := start / bucketSecs; bk != bucket {
			if bucket >= 0 {
				b.WriteByte('\n')
			}
			bucket = bk
			fmt.Fprintf(&b, "[%s] %s", Clock(start), t)
			continue
		}
		b.WriteByte(' ')
		b.WriteString(t)
	}
	return b.String()
}

// Clock formats seconds as m:ss or h:mm:ss.
func Clock(secs int) string {
	if secs < 0 {
		secs = 0
	}
	h, m, s := secs/3600, secs%3600/60, secs%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// splitLines cuts text into pieces of at most max characters, only at
// line breaks (a single over-long line becomes its own piece).
func splitLines(text string, max int) []string {
	var out []string
	var cur strings.Builder
	n := 0
	for _, line := range strings.Split(text, "\n") {
		l := utf8.RuneCountInString(line) + 1
		if n > 0 && n+l > max {
			out = append(out, cur.String())
			cur.Reset()
			n = 0
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
		n += l
	}
	if n > 0 {
		out = append(out, cur.String())
	}
	return out
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "…"
}

// parseResult decodes the model's JSON object, tolerating markdown
// fences or chatter around it.
func parseResult(s string) (*Result, error) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, errors.New("AI 返回的不是 JSON")
	}
	var r Result
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return nil, fmt.Errorf("AI 返回的 JSON 无法解析：%v", err)
	}
	r.Summary = strings.TrimSpace(r.Summary)
	if r.Summary == "" {
		return nil, errors.New("AI 返回的摘要为空")
	}
	kp := r.KeyPoints[:0]
	for _, k := range r.KeyPoints {
		if k = strings.TrimSpace(k); k != "" {
			kp = append(kp, k)
		}
	}
	r.KeyPoints = kp
	return &r, nil
}

// cleanChapters enforces the creator's start times when the counts
// match, then drops chapters outside the video or with an empty title,
// sorts by start and removes duplicate starts.
func cleanChapters(in []Chapter, d *youtube.VideoDetail) []Chapter {
	if len(d.Chapters) > 0 && len(in) == len(d.Chapters) {
		for i := range in {
			in[i].Start = int(d.Chapters[i].Start)
		}
	}
	out := make([]Chapter, 0, len(in))
	for _, c := range in {
		c.Title = strings.TrimSpace(c.Title)
		c.Summary = strings.TrimSpace(c.Summary)
		if c.Title == "" || c.Start < 0 || (d.Duration > 0 && c.Start >= d.Duration) {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	dedup := out[:0]
	for i, c := range out {
		if i > 0 && c.Start == dedup[len(dedup)-1].Start {
			continue
		}
		dedup = append(dedup, c)
	}
	return dedup
}
