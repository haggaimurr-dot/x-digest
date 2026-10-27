package ytsummary

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"xdigest/internal/youtube"
)

type fakeLLM struct {
	calls   []string // user messages
	systems []string
	reply   func(system, user string) (string, error)
}

func (f *fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	f.calls = append(f.calls, user)
	f.systems = append(f.systems, system)
	return f.reply(system, user)
}

func TestFormatTranscriptBuckets(t *testing.T) {
	got := FormatTranscript([]youtube.Segment{
		{Start: 1, Text: "a"}, {Start: 12, Text: "b"}, {Start: 31, Text: "c"}, {Start: 3700, Text: "d"},
	})
	want := "[00:01] a b\n[00:31] c\n[1:01:40] d"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSummarizeKeepsCreatorChapters(t *testing.T) {
	d := &youtube.VideoDetail{
		Title: "Build an agent", Duration: 600,
		Chapters: []youtube.Chapter{{Start: 0, Title: "Intro"}, {Start: 95, Title: "Setup"}, {Start: 300, Title: "Demo"}},
		Segments: []youtube.Segment{{Start: 0, Text: "hi"}, {Start: 100, Text: "install"}},
	}
	llm := &fakeLLM{reply: func(system, user string) (string, error) {
		// Fenced output, wrong start times: both must be handled.
		return "```json\n" + `{"summary":"全文","key_points":[" 要点一 ",""],"chapters":[
			{"start":0,"title":"开场","summary":"s1"},{"start":90,"title":"安装","summary":"s2"},{"start":290,"title":"演示","summary":"s3"}]}` + "\n```", nil
	}}
	res, err := Summarize(context.Background(), llm, d, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(llm.calls) != 1 || !strings.Contains(llm.calls[0], "start=95 [01:35] Setup") {
		t.Fatalf("prompt should list creator chapters: %v", llm.calls)
	}
	if !strings.Contains(llm.systems[0], "Simplified Chinese") {
		t.Errorf("target language missing from system prompt")
	}
	if res.Source != SourceTranscript || res.Summary != "全文" || len(res.KeyPoints) != 1 || res.KeyPoints[0] != "要点一" {
		t.Fatalf("unexpected result %+v", res)
	}
	starts := []int{res.Chapters[0].Start, res.Chapters[1].Start, res.Chapters[2].Start}
	if fmt.Sprint(starts) != "[0 95 300]" {
		t.Errorf("chapter starts = %v, want creator's [0 95 300]", starts)
	}
}

func TestSummarizeCleansModelChapters(t *testing.T) {
	d := &youtube.VideoDetail{Title: "t", Duration: 200, Segments: []youtube.Segment{{Start: 0, Text: "x"}}}
	llm := &fakeLLM{reply: func(_, _ string) (string, error) {
		return `{"summary":"s","key_points":[],"chapters":[
			{"start":120,"title":"B","summary":""},{"start":0,"title":"A","summary":""},
			{"start":120,"title":"dup","summary":""},{"start":500,"title":"past end","summary":""},
			{"start":-1,"title":"neg","summary":""},{"start":60,"title":" ","summary":""}]}`, nil
	}}
	res, err := Summarize(context.Background(), llm, d, "en")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range res.Chapters {
		got = append(got, fmt.Sprintf("%d:%s", c.Start, c.Title))
	}
	if strings.Join(got, ",") != "0:A,120:B" {
		t.Fatalf("chapters = %v", got)
	}
}

func TestSummarizeWithoutTranscript(t *testing.T) {
	d := &youtube.VideoDetail{Title: "t", Description: "about Go"}
	llm := &fakeLLM{reply: func(system, user string) (string, error) {
		if !strings.Contains(user, "No transcript is available") || !strings.Contains(system, "empty chapters array") {
			t.Errorf("prompt should say there is no transcript:\n%s\n%s", system, user)
		}
		return `{"summary":"s","key_points":["k"],"chapters":[]}`, nil
	}}
	res, err := Summarize(context.Background(), llm, d, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceDescription {
		t.Errorf("source = %q, want description", res.Source)
	}
}

func TestSummarizeCondensesLongTranscript(t *testing.T) {
	var segs []youtube.Segment
	for i := 0; i < 1500; i++ {
		segs = append(segs, youtube.Segment{Start: float64(i * 31), Text: strings.Repeat("词", 30)})
	}
	d := &youtube.VideoDetail{Title: "long", Duration: 1500 * 31, Segments: segs}
	llm := &fakeLLM{reply: func(system, user string) (string, error) {
		if strings.HasPrefix(user, "Part ") {
			return "[00:00] note", nil
		}
		if !strings.Contains(user, "Notes condensed") || strings.Contains(user, "词词词") {
			t.Errorf("final call should get notes, not the raw transcript")
		}
		return `{"summary":"s","key_points":[],"chapters":[{"start":0,"title":"A","summary":""}]}`, nil
	}}
	if _, err := Summarize(context.Background(), llm, d, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if len(llm.calls) < 3 {
		t.Fatalf("expected several slice calls plus the final call, got %d", len(llm.calls))
	}
}

func TestSummarizeErrors(t *testing.T) {
	d := &youtube.VideoDetail{Title: "t"}
	for name, reply := range map[string]string{
		"not json":      "sorry, I can't",
		"empty summary": `{"summary":"  ","key_points":[],"chapters":[]}`,
	} {
		llm := &fakeLLM{reply: func(_, _ string) (string, error) { return reply, nil }}
		if _, err := Summarize(context.Background(), llm, d, "zh-CN"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	boom := errors.New("upstream 500")
	llm := &fakeLLM{reply: func(_, _ string) (string, error) { return "", boom }}
	if _, err := Summarize(context.Background(), llm, d, "zh-CN"); !errors.Is(err, boom) {
		t.Errorf("upstream error should propagate, got %v", err)
	}
}

func TestSummarizeResources(t *testing.T) {
	d := &youtube.VideoDetail{Title: "t", Segments: []youtube.Segment{{Start: 0, Text: "x"}}}
	llm := &fakeLLM{reply: func(system, _ string) (string, error) {
		if !strings.Contains(system, `"resources"`) {
			t.Errorf("prompt should ask for resources")
		}
		return `{"summary":"s","key_points":[],"chapters":[],"resources":[
			{"name":" Claude Code ","url":"https://claude.com/claude-code","note":"CLI"},
			{"name":"n8n","url":"n8n.io","note":""},{"name":"","url":"https://x","note":"no name"}]}`, nil
	}}
	res, err := Summarize(context.Background(), llm, d, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Resources) != 2 || res.Resources[0].Name != "Claude Code" || res.Resources[1].URL != "" {
		t.Fatalf("resources = %+v", res.Resources)
	}
}
