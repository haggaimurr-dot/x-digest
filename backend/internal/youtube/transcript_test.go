package youtube

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rollingVTT = `WEBVTT
Kind: captions
Language: en

00:00:00.320 --> 00:00:02.990 align:start position:0%
` + " " + `
hello<00:00:00.640><c> everyone</c><00:00:01.120><c> and</c><00:00:01.360><c> welcome</c>

00:00:02.990 --> 00:00:03.000 align:start position:0%
hello everyone and welcome
` + " " + `

00:00:03.000 --> 00:00:06.150 align:start position:0%
hello everyone and welcome
today<00:00:03.400><c> we</c><00:00:03.600><c> build</c><00:00:04.000><c> a</c><00:00:04.200><c> Claude</c><00:00:04.600><c> agent</c>

01:02:03.500 --> 01:02:05.000
Tom &amp; Jerry
`

func TestParseVTTDedupesRollingCaptions(t *testing.T) {
	segs := ParseVTT([]byte(rollingVTT))
	want := []Segment{
		{Start: 0.32, Text: "hello everyone and welcome"},
		{Start: 3.0, Text: "today we build a Claude agent"},
		{Start: 3723.5, Text: "Tom & Jerry"},
	}
	if len(segs) != len(want) {
		t.Fatalf("got %d segments %+v, want %d", len(segs), segs, len(want))
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Errorf("segment %d = %+v, want %+v", i, segs[i], want[i])
		}
	}
}

func TestPickTrack(t *testing.T) {
	cases := []struct {
		name         string
		lang         string
		manual, auto []string
		want         string
		wantAuto     bool
		wantOK       bool
	}{
		{"manual in video language wins", "ja", []string{"en", "ja"}, []string{"ja-orig", "en"}, "ja", false, true},
		{"manual prefix match", "en", []string{"en-US"}, []string{"en-orig"}, "en-US", false, true},
		{"manual chinese over auto", "fr", []string{"zh-Hans"}, []string{"fr-orig"}, "zh-Hans", false, true},
		{"auto orig track", "en", nil, []string{"de", "en", "en-orig"}, "en-orig", true, true},
		{"auto plain language", "es", nil, []string{"en", "es"}, "es", true, true},
		{"unknown language falls back to auto english", "", nil, []string{"de", "en"}, "en", true, true},
		{"only an odd manual track", "", []string{"pt-BR"}, nil, "pt-BR", false, true},
		{"nothing", "en", nil, nil, "", false, false},
	}
	for _, c := range cases {
		got, auto, ok := pickTrack(c.lang, c.manual, c.auto)
		if got != c.want || auto != c.wantAuto || ok != c.wantOK {
			t.Errorf("%s: got (%q,%v,%v), want (%q,%v,%v)", c.name, got, auto, ok, c.want, c.wantAuto, c.wantOK)
		}
	}
}

func TestFetchDetailDownloadsOneTrack(t *testing.T) {
	var calls [][]string
	c := &Client{Runner: func(ctx context.Context, args []string) ([]byte, error) {
		calls = append(calls, args)
		if len(calls) == 1 {
			return []byte(`{"title":"T","description":"D","duration":125.4,"language":"en",
				"chapters":[{"start_time":0,"end_time":60,"title":"Intro"},{"start_time":60,"end_time":125,"title":"Build"}],
				"subtitles":{"live_chat":[]},"automatic_captions":{"en":[],"en-orig":[],"fr":[]}}`), nil
		}
		for i, a := range args {
			if a == "-o" {
				out := strings.Replace(args[i+1], "%(ext)s", "en-orig.vtt", 1)
				if err := os.WriteFile(out, []byte(rollingVTT), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		return nil, nil
	}}
	d, err := c.FetchDetail(context.Background(), "https://www.youtube.com/watch?v=x")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "T" || d.Duration != 125 || len(d.Chapters) != 2 || d.Chapters[1].Start != 60 {
		t.Fatalf("unexpected detail %+v", d)
	}
	if d.TranscriptLang != "en-orig" || !d.TranscriptAuto || len(d.Segments) != 3 {
		t.Fatalf("transcript = %q auto=%v segs=%d", d.TranscriptLang, d.TranscriptAuto, len(d.Segments))
	}
	if len(calls) != 2 {
		t.Fatalf("yt-dlp calls = %d, want 2", len(calls))
	}
	second := strings.Join(calls[1], " ")
	if !strings.Contains(second, "--write-auto-subs") || !strings.Contains(second, "--sub-langs en-orig ") {
		t.Errorf("second call should fetch only the en-orig auto track: %s", second)
	}
	if dir := filepath.Dir(calls[1][len(calls[1])-2]); dir != "" {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("temp dir %s should be removed", dir)
		}
	}
}

func TestFetchDetailWithoutSubtitles(t *testing.T) {
	calls := 0
	c := &Client{Runner: func(ctx context.Context, args []string) ([]byte, error) {
		calls++
		return []byte(`{"title":"T","description":"D","duration":30}`), nil
	}}
	d, err := c.FetchDetail(context.Background(), "u")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(d.Segments) != 0 || d.TranscriptLang != "" {
		t.Fatalf("calls=%d detail=%+v", calls, d)
	}
}

func TestFetchTopCommentsSortsByLikes(t *testing.T) {
	var args []string
	c := &Client{Runner: func(ctx context.Context, a []string) ([]byte, error) {
		args = a
		return []byte(`{"id":"x","comments":[
			{"author":"@a","text":"first","like_count":3,"parent":"root","timestamp":1761500000},
			{"author":"@b","text":"reply","like_count":999,"parent":"Ugx1"},
			{"author":"@c","text":"  ","like_count":500,"parent":"root"},
			{"author":"@d","text":"best","like_count":120,"parent":"root"},
			{"author":"@e","text":"no likes field","parent":"root"},
			{"author":"@f","text":"mid","like_count":40,"parent":"root"}]}`), nil
	}}
	got, err := c.FetchTopComments(context.Background(), "u", 3)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, cm := range got {
		texts = append(texts, cm.Text)
	}
	if strings.Join(texts, ",") != "best,mid,first" {
		t.Fatalf("got %v", texts)
	}
	if got[2].Timestamp != 1761500000 || got[0].Author != "@d" {
		t.Errorf("fields not kept: %+v", got)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--write-comments") || !strings.Contains(joined, "comment_sort=top") {
		t.Errorf("unexpected args: %s", joined)
	}
}

func TestFetchTopCommentsDisabled(t *testing.T) {
	c := &Client{Runner: func(ctx context.Context, a []string) ([]byte, error) { return []byte(`{"id":"x"}`), nil }}
	got, err := c.FetchTopComments(context.Background(), "u", 3)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
