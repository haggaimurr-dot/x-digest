package ytsummary

// GitHub repo AI summary (2026-10-29): same model call, JSON shape and
// parsing as the post summary, without chapters. The material is the
// README (raw markdown) with the description, language, stars, tags and
// the latest release; without a README only the short facts are sent
// (Source is then SourceDescription and the page says so).

import (
	"context"
	"fmt"
	"strings"
)

// maxReadmeChars caps the README sent to the model (huge docs READMEs).
const maxReadmeChars = 30000

// RepoInput is what the repos page knows about one repository.
type RepoInput struct {
	FullName    string
	Description string
	Language    string
	Stars       int
	Tags        []string
	Readme      string // raw markdown ("" = none or not readable)
	Release     string // latest release: "tag name\nnotes" ("" = none)
}

// SummarizeRepo builds the summary of one repository in the target
// language ("zh-CN" or "en"). Chapters are always empty.
func SummarizeRepo(ctx context.Context, llm Completer, in RepoInput, target string) (*Result, error) {
	lang := "Simplified Chinese"
	if target == "en" {
		lang = "English"
	}
	source := SourceTranscript
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\n", in.FullName)
	if s := strings.TrimSpace(in.Description); s != "" {
		fmt.Fprintf(&b, "Description: %s\n", s)
	}
	if in.Language != "" {
		fmt.Fprintf(&b, "Main language: %s\n", in.Language)
	}
	fmt.Fprintf(&b, "Stars: %d\n", in.Stars)
	if len(in.Tags) > 0 {
		fmt.Fprintf(&b, "Reader's tags: %s\n", strings.Join(in.Tags, ", "))
	}
	if s := strings.TrimSpace(in.Release); s != "" {
		fmt.Fprintf(&b, "\nLatest release:\n%s\n", truncateRunes(s, 3000))
	}
	if s := strings.TrimSpace(in.Readme); s != "" {
		fmt.Fprintf(&b, "\nREADME:\n%s\n", truncateRunes(s, maxReadmeChars))
	} else {
		source = SourceDescription
		b.WriteString("\n(The README could not be read. Use only the facts above and do not invent details.)\n")
	}
	system := "You summarize GitHub repositories for a reader deciding whether to use one. Write everything in " + lang + ". " +
		"Return ONLY a JSON object, no markdown fences, with this shape: " +
		`{"summary": string, "key_points": [string], "chapters": [], "resources": [{"name": string, "url": string, "note": string}]}. ` +
		"summary: what the project is, what problem it solves and for whom, and how it is used, 1-3 short paragraphs separated by \\n\\n. " +
		"key_points: 3-6 concrete points (main features, how to install or run it, requirements, limits), one sentence each. " +
		"resources: documentation, demos, related tools or models the material links or names; url only when the material states it (else \"\"), never guess one; " +
		"note: one short phrase on what it is. Skip badges and social links. chapters: always []. " +
		"Base everything on the material given; if it is too thin to say more, say so briefly instead of inventing."
	out, err := llm.Complete(ctx, system, b.String())
	if err != nil {
		return nil, err
	}
	res, err := parseResult(out)
	if err != nil {
		return nil, err
	}
	res.Chapters = nil
	res.Source = source
	return res, nil
}
