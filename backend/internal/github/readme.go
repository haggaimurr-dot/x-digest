// ParseReadme extracts a short intro paragraph and tutorial steps from a
// repository README (raw markdown). Standard library only.
//
// Intro: the first substantive paragraph — not a heading, badge/image-only
// line, HTML comment or code block — with markdown markup stripped,
// capped at 500 runes.
//
// Steps: list items and fenced code blocks from the first section whose
// heading matches install/installation/quick start/usage/getting started
// （安装/使用/快速开始）, falling back to the first ordered list in the
// whole document when no such section exists. At most 12 steps.
package github

import (
	"regexp"
	"strings"

	"xdigest/internal/model"
)

const (
	maxIntroRunes    = 500
	maxTutorialSteps = 12
)

var (
	// tutorialHeadingRe matches section headings that hold install/usage
	// instructions, in English or Chinese.
	tutorialHeadingRe = regexp.MustCompile(`(?i)install|installation|quick.?start|usage|getting.?started|使用|安装|快速开始`)
	atxHeadingRe      = regexp.MustCompile(`^(#{1,6})(\s+(.*?))?\s*$`)
	setextUnderlineRe = regexp.MustCompile(`^(=+|-+)\s*$`)
	hrRe              = regexp.MustCompile(`^(-{3,}|_{3,}|\*{3,}|(-\s+){2,}-|(\*\s+){2,}\*|(_\s+){2,}_)\s*$`)
	listItemRe        = regexp.MustCompile(`^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$`)
	orderedItemRe     = regexp.MustCompile(`^(\s*)\d{1,9}[.)]\s+(.*)$`)
	imageRe           = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	linkRe            = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	refLinkRe         = regexp.MustCompile(`\[([^\]]*)\]\[[^\]]*\]`)
	codeSpanRe        = regexp.MustCompile("`([^`]*)`")
	strikeRe          = regexp.MustCompile(`~~([^~]+)~~`)
	boldRe            = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	italicRe          = regexp.MustCompile(`\*([^*]+)\*`)
	htmlTagRe         = regexp.MustCompile(`<[^>]*>`)
)

// ParseReadme returns the README's intro paragraph and tutorial steps.
func ParseReadme(md string) (intro string, steps []model.TutorialStep) {
	lines := strings.Split(md, "\n")
	return parseIntro(lines), parseSteps(lines)
}

// ---- intro ----

func parseIntro(lines []string) string {
	var para []string
	inFence := false
	inComment := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		line, inComment = stripHTMLComment(line, inComment)
		if inComment {
			continue
		}
		if line == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		// Headings (ATX, or any line starting with '#') end/skip the intro.
		if strings.HasPrefix(line, "#") {
			if len(para) > 0 {
				break
			}
			continue
		}
		if setextUnderlineRe.MatchString(line) {
			// The previous line was a Setext heading; drop it. Any
			// lines before it were a real paragraph, which is complete.
			if len(para) > 0 {
				para = para[:len(para)-1]
				if len(para) > 0 {
					break
				}
			}
			continue
		}
		if hrRe.MatchString(line) {
			if len(para) > 0 {
				break
			}
			continue
		}
		if listItemRe.MatchString(line) {
			// Lists are not intro text.
			if len(para) > 0 {
				break
			}
			continue
		}
		cleaned := cleanLine(line)
		if cleaned == "" {
			// Badge/image-only or tag-only line.
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, cleaned)
	}
	if len(para) == 0 {
		return ""
	}
	intro := strings.Join(strings.Fields(strings.Join(para, " ")), " ")
	return truncateRunes(intro, maxIntroRunes)
}

// stripHTMLComment removes <!-- ... --> comments from a line. A comment
// that starts on this line but does not close here sets inComment; text
// after a closing --> on a later line is returned. It reports the
// remaining line content and whether we are still inside a comment.
func stripHTMLComment(line string, inComment bool) (string, bool) {
	for {
		if inComment {
			if idx := strings.Index(line, "-->"); idx >= 0 {
				inComment = false
				line = strings.TrimSpace(line[idx+3:])
				continue
			}
			return "", true
		}
		idx := strings.Index(line, "<!--")
		if idx < 0 {
			return line, false
		}
		rest := line[idx:]
		if end := strings.Index(rest, "-->"); end >= 0 {
			line = strings.TrimSpace(line[:idx] + " " + rest[end+3:])
			continue
		}
		inComment = true
		return strings.TrimSpace(line[:idx]), true
	}
}

// ---- steps ----

func parseSteps(lines []string) []model.TutorialStep {
	inFence := false
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if lvl, text, ok := parseATXHeading(trimmed); ok && tutorialHeadingRe.MatchString(text) {
			return stepsInSection(lines, i, lvl)
		}
	}
	// No install/usage section: fall back to the first ordered list.
	return stepsFromFirstOrderedList(lines)
}

// stepsInSection collects top-level list items and fenced code blocks from
// the section starting after line start, until the next heading of the
// same or a higher level.
func stepsInSection(lines []string, start, level int) []model.TutorialStep {
	type item struct {
		text   string
		indent int
		isCode bool
	}
	var items []item
	inFence := false
	inComment := false
	var codeFirst string
	codeHasContent := false
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if inFence {
				inFence = false
				if codeHasContent {
					items = append(items, item{text: codeFirst, isCode: true})
				}
				codeFirst = ""
				codeHasContent = false
			} else {
				inFence = true
			}
			continue
		}
		if inFence {
			if !codeHasContent && trimmed != "" {
				codeFirst = trimmed
				codeHasContent = true
			}
			continue
		}
		_, inComment = stripHTMLComment(trimmed, inComment)
		if inComment {
			continue
		}
		if lvl, _, ok := parseATXHeading(trimmed); ok && lvl <= level {
			break
		}
		if setextUnderlineRe.MatchString(trimmed) {
			sl := 2
			if strings.HasPrefix(trimmed, "=") {
				sl = 1
			}
			if sl <= level {
				break
			}
			continue
		}
		if indent, text, ok := parseListItem(line); ok {
			items = append(items, item{text: text, indent: indent})
		}
	}
	if inFence && codeHasContent {
		items = append(items, item{text: codeFirst, isCode: true})
	}
	// Only top-level list items: the smallest indent among list items in
	// this section is the top level; deeper items are nested sub-lists.
	minIndent := -1
	for _, it := range items {
		if !it.isCode && (minIndent < 0 || it.indent < minIndent) {
			minIndent = it.indent
		}
	}
	steps := []model.TutorialStep{}
	for _, it := range items {
		if it.isCode {
			if t := strings.TrimSpace(it.text); t != "" {
				steps = append(steps, model.TutorialStep{Text: t})
			}
			continue
		}
		if it.indent != minIndent {
			continue
		}
		if t := cleanStepText(it.text); t != "" {
			steps = append(steps, model.TutorialStep{Text: t})
		}
	}
	return capSteps(steps)
}

// stepsFromFirstOrderedList collects the items of the first ordered list
// in the whole document (top-level items only).
func stepsFromFirstOrderedList(lines []string) []model.TutorialStep {
	steps := []model.TutorialStep{}
	inFence := false
	started := false
	baseIndent := 0
	for _, raw := range lines {
		line := raw
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if !started {
			indent, text, ok := parseOrderedItem(line)
			if !ok {
				continue
			}
			started = true
			baseIndent = indent
			if t := cleanStepText(text); t != "" {
				steps = append(steps, model.TutorialStep{Text: t})
			}
			continue
		}
		if _, _, ok := parseATXHeading(trimmed); ok {
			break
		}
		if trimmed == "" {
			continue
		}
		if indent, text, ok := parseOrderedItem(line); ok {
			if indent == baseIndent {
				if t := cleanStepText(text); t != "" {
					steps = append(steps, model.TutorialStep{Text: t})
				}
			}
			// Deeper items are nested sub-steps; the list continues.
			continue
		}
		// A non-list line at the base indent or shallower ends the list;
		// deeper lines are continuations of an item.
		if indentOf(line) <= baseIndent {
			break
		}
	}
	return capSteps(steps)
}

func capSteps(steps []model.TutorialStep) []model.TutorialStep {
	if len(steps) > maxTutorialSteps {
		steps = steps[:maxTutorialSteps]
	}
	if steps == nil {
		steps = []model.TutorialStep{}
	}
	return steps
}

// ---- helpers ----

// parseATXHeading returns the level (1-6) and text of an ATX heading
// ("# Title"), or ok=false.
func parseATXHeading(line string) (level int, text string, ok bool) {
	m := atxHeadingRe.FindStringSubmatch(line)
	if m == nil || m[2] == "" {
		return 0, "", false
	}
	text = strings.TrimRight(strings.TrimSpace(m[3]), "#")
	text = strings.TrimSpace(text)
	return len(m[1]), text, true
}

// parseListItem matches an unordered or ordered list item, returning its
// indent (tabs count as 4 spaces) and item text.
func parseListItem(line string) (indent int, text string, ok bool) {
	m := listItemRe.FindStringSubmatch(line)
	if m == nil {
		return 0, "", false
	}
	return indentOf(m[1]), m[3], true
}

// parseOrderedItem matches an ordered list item only.
func parseOrderedItem(line string) (indent int, text string, ok bool) {
	m := orderedItemRe.FindStringSubmatch(line)
	if m == nil {
		return 0, "", false
	}
	return indentOf(m[1]), m[2], true
}

// indentOf counts leading whitespace, tabs as 4 spaces.
func indentOf(s string) int {
	n := 0
	for _, r := range s {
		if r == '\t' {
			n += 4
		} else if r == ' ' {
			n++
		} else {
			break
		}
	}
	return n
}

// cleanLine strips blockquote markers and markdown markup from one line.
func cleanLine(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, ">") {
		s = strings.TrimSpace(strings.TrimPrefix(s, ">"))
	}
	return strings.TrimSpace(stripMarkdown(s))
}

// cleanStepText strips markdown markup and a task-list checkbox from a
// list item's text.
func cleanStepText(s string) string {
	s = strings.TrimSpace(stripMarkdown(s))
	for _, box := range []string{"[ ]", "[x]", "[X]"} {
		if strings.HasPrefix(s, box) {
			s = strings.TrimSpace(s[len(box):])
			break
		}
	}
	return s
}

// stripMarkdown removes images (entirely), turns links into their text,
// and drops emphasis/code/strikethrough markers and HTML tags.
func stripMarkdown(s string) string {
	s = imageRe.ReplaceAllString(s, "")
	s = linkRe.ReplaceAllString(s, "$1")
	s = refLinkRe.ReplaceAllString(s, "$1")
	s = codeSpanRe.ReplaceAllString(s, "$1")
	s = strikeRe.ReplaceAllString(s, "$1")
	s = boldRe.ReplaceAllString(s, "$1$2")
	s = italicRe.ReplaceAllString(s, "$1")
	s = htmlTagRe.ReplaceAllString(s, "")
	return s
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
