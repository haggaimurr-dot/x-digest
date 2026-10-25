package github

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseReadmeBadgesAndIntro(t *testing.T) {
	md := `# My Project

[![Build](https://example.com/build.svg)](https://example.com) [![Go Report](https://example.com/report.svg)](https://example.com)
![](https://example.com/banner.png)

A **fast** little library for *parsing* things. See [the docs](https://example.com/docs) or run ` + "`mytool --help`" + `.

More text in a second paragraph.
`
	intro, steps := ParseReadme(md)
	want := "A fast little library for parsing things. See the docs or run mytool --help."
	if intro != want {
		t.Errorf("intro = %q, want %q", intro, want)
	}
	if strings.Contains(intro, "Build") || strings.Contains(intro, "Go Report") || strings.Contains(intro, "banner") {
		t.Errorf("intro must not contain badge text: %q", intro)
	}
	if len(steps) != 0 {
		t.Errorf("steps = %+v, want empty (no lists)", steps)
	}
}

func TestParseReadmeInstallSectionListAndCode(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("# Demo\n\nIntro paragraph.\n\n## Installation\n\n")
	// 13 list items + 1 code block: must cap at 12 steps.
	for i := 1; i <= 13; i++ {
		fmt.Fprintf(&sb, "- step %d\n", i)
	}
	sb.WriteString("\n```bash\ngo install ./...\n```\n\n## Usage\n\n- not in this section\n")
	intro, steps := ParseReadme(sb.String())
	if intro != "Intro paragraph." {
		t.Errorf("intro = %q, want %q", intro, "Intro paragraph.")
	}
	if len(steps) != 12 {
		t.Fatalf("len(steps) = %d, want 12 (capped)", len(steps))
	}
	if steps[0].Text != "step 1" || steps[11].Text != "step 12" {
		t.Errorf("steps[0] = %q, steps[11] = %q, want step 1 / step 12", steps[0].Text, steps[11].Text)
	}
	for _, s := range steps {
		if s.Text == "go install ./..." {
			t.Error("code block step must be cut off by the 12-step cap")
		}
		if s.Text == "not in this section" {
			t.Error("steps must stop at the next same-level heading")
		}
	}
}

func TestParseReadmeInstallSectionCodeBlockStep(t *testing.T) {
	md := `# Demo

Intro.

## Quick Start

Run the installer:

` + "```sh" + `
./install.sh --yes
echo done
` + "```" + `

- then configure it
`
	_, steps := ParseReadme(md)
	want := []string{"./install.sh --yes", "then configure it"}
	if len(steps) != len(want) {
		t.Fatalf("steps = %+v, want %v", steps, want)
	}
	for i, w := range want {
		if steps[i].Text != w {
			t.Errorf("steps[%d] = %q, want %q", i, steps[i].Text, w)
		}
	}
}

func TestParseReadmeNestedSublistOnlyTopLevel(t *testing.T) {
	md := `# Demo

Intro.

## 安装

- first
  - nested one
  - nested two
- second
  1. nested ordered
- third
`
	_, steps := ParseReadme(md)
	want := []string{"first", "second", "third"}
	if len(steps) != len(want) {
		t.Fatalf("steps = %+v, want %v", steps, want)
	}
	for i, w := range want {
		if steps[i].Text != w {
			t.Errorf("steps[%d] = %q, want %q", i, steps[i].Text, w)
		}
	}
}

func TestParseReadmeFallbackToFirstOrderedList(t *testing.T) {
	md := `# Demo

Intro paragraph.

## Features

- fast
- small

## Changelog

1. download the binary
2. run it
3. enjoy

## Other

1. not this list
`
	_, steps := ParseReadme(md)
	want := []string{"download the binary", "run it", "enjoy"}
	if len(steps) != len(want) {
		t.Fatalf("steps = %+v, want %v", steps, want)
	}
	for i, w := range want {
		if steps[i].Text != w {
			t.Errorf("steps[%d] = %q, want %q", i, steps[i].Text, w)
		}
	}
}

func TestParseReadmeNoListsMeansNoSteps(t *testing.T) {
	md := `# Demo

Just a paragraph, no lists at all.

## About

More prose here.
`
	intro, steps := ParseReadme(md)
	if intro != "Just a paragraph, no lists at all." {
		t.Errorf("intro = %q", intro)
	}
	if len(steps) != 0 {
		t.Errorf("steps = %+v, want empty", steps)
	}
}

func TestParseReadmeIntroTruncatedAt500Runes(t *testing.T) {
	long := strings.Repeat("字", 600)
	md := "# Demo\n\n" + long + "\n"
	intro, _ := ParseReadme(md)
	if len([]rune(intro)) != 501 { // 500 runes + "…"
		t.Errorf("len(intro runes) = %d, want 501", len([]rune(intro)))
	}
	if !strings.HasSuffix(intro, "…") {
		t.Errorf("truncated intro must end with …, got %q", intro)
	}
	if !strings.HasPrefix(intro, "字") {
		t.Errorf("intro = %q", intro)
	}
}

func TestParseReadmeSkipsCommentsCodeAndSetextTitle(t *testing.T) {
	md := `<!-- a comment
spanning lines -->
` + "```" + `
# not a heading, inside code
` + "```" + `

Project Title
=============

The real intro paragraph.
`
	intro, _ := ParseReadme(md)
	if intro != "The real intro paragraph." {
		t.Errorf("intro = %q, want %q", intro, "The real intro paragraph.")
	}
}

func TestParseReadmeEmpty(t *testing.T) {
	intro, steps := ParseReadme("")
	if intro != "" || len(steps) != 0 {
		t.Errorf("empty README: intro=%q steps=%+v, want both empty", intro, steps)
	}
	intro, steps = ParseReadme("# Only a title\n\n[![badge](https://example.com/b.svg)](https://example.com)\n")
	if intro != "" || len(steps) != 0 {
		t.Errorf("title+badges only: intro=%q steps=%+v, want both empty", intro, steps)
	}
}
