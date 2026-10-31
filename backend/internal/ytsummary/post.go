package ytsummary

// X post AI summary (2026-10-27): same model call, JSON shape and parsing
// as the video summary, without chapters. The material is the post's
// original text and links; when the original text was never collected,
// the digest title, the earlier digest summary and the steps stand in
// (Source is then SourceDescription and the page says so).

import (
	"context"
	"fmt"
	"strings"
)

// maxPostChars caps the original text sent to the model (long threads).
const maxPostChars = 20000

// PostInput is what the X page knows about one post.
type PostInput struct {
	Title         string
	Author        string
	Text          string // original post text ("" = not collected)
	Links         []string
	DigestSummary string // earlier pipeline summary, used only without Text
	Steps         []string
}

// SummarizePost builds the summary of one X post in the target language
// ("zh-CN" or "en"). Chapters are always empty.
func SummarizePost(ctx context.Context, llm Completer, in PostInput, target string) (*Result, error) {
	lang := "Simplified Chinese"
	if target == "en" {
		lang = "English"
	}
	source := SourceTranscript
	var b strings.Builder
	fmt.Fprintf(&b, "Author: @%s\nHeadline: %s\n", in.Author, in.Title)
	if text := strings.TrimSpace(in.Text); text != "" {
		fmt.Fprintf(&b, "\nOriginal post:\n%s\n", truncateRunes(text, maxPostChars))
	} else {
		source = SourceDescription
		b.WriteString("\n(The original post text was not collected. Use only the material below and do not invent details.)\n")
		if s := strings.TrimSpace(in.DigestSummary); s != "" {
			fmt.Fprintf(&b, "\nEarlier digest of the post:\n%s\n", s)
		}
		if len(in.Steps) > 0 {
			b.WriteString("\nSteps extracted earlier:\n")
			for i, s := range in.Steps {
				fmt.Fprintf(&b, "%d. %s\n", i+1, s)
			}
		}
	}
	if len(in.Links) > 0 {
		b.WriteString("\nLinks in the post:\n")
		for _, l := range in.Links {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	system := "You summarize posts from X (Twitter) for a reader who has not read them. Write everything in " + lang + ". " +
		"Return ONLY a JSON object, no markdown fences, with this shape: " +
		`{"summary": string, "key_points": [string], "chapters": [], "resources": [{"name": string, "url": string, "note": string}]}. ` +
		"summary: what the post says and why it matters, 1-3 short paragraphs separated by \\n\\n (one short paragraph is enough for a short post or a reply). " +
		"key_points: 0-6 concrete takeaways (names, numbers, tools, steps), one sentence each; empty for chit-chat. " +
		"resources: tools, products, repos, websites or downloads the post mentions; url only when the material states it (else \"\"), never guess one; " +
		"note: one short phrase on what it is. Skip social accounts. chapters: always []. " +
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
