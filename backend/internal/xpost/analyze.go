// Package xpost holds the X daily-digest pipeline pieces shared by
// cmd/digest (batch, raw_posts.json → posts.json) and the API (single-post
// import, per-account refresh): the AI analysis of one raw post, fetching a
// post from X (fxtwitter) and finding an account's new post ids (Grok
// x_search through the relay).
package xpost

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"xdigest/internal/importer"
	"xdigest/internal/translate"
)

type Step struct {
	Text  string `json:"text"`
	Tools string `json:"tools"`
	Tech  string `json:"tech"`
	Prep  string `json:"prep"`
}

// Post is one posts.json entry (same shape as the existing file / the
// importer).
type Post struct {
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Author        string   `json:"author"`
	AuthorName    string   `json:"authorName"`
	PublishedAt   string   `json:"publishedAt"`
	Title         string   `json:"title"`
	Industry      string   `json:"industry"`
	Tech          string   `json:"tech"`
	Summary       string   `json:"summary"`
	Steps         []Step   `json:"steps"`
	Tags          []string `json:"tags"`
	IsLong        bool     `json:"isLong"`
	ShowFlowchart bool     `json:"showFlowchart"`
	Kind          string   `json:"kind"`
	Resources     []Res    `json:"resources"`
}

// Res explains one link of the post (only links that are in the post).
type Res struct {
	URL  string `json:"url"`
	Name string `json:"name"`
	Note string `json:"note"`
}

// PostKinds are the allowed post types; anything else is stored as "".
var PostKinds = map[string]bool{"教程实操": true, "技术分享": true, "工具资源推荐": true, "观点讨论": true, "日常闲聊": true}

// MinChars is the short-reply filter: posts with fewer substantive
// characters (links and @mentions removed) are not digested.
const MinChars = 15

const systemPrompt = `你在为「X 博主推文日报」整理单条推文。只根据给出的原文写，不要编造；原文没提到的就不写。
只返回一个 JSON 对象（不要 markdown 代码块），字段：
{"title": 一句话中文标题（20 字以内，说清这条推文讲了什么，不要写"作者称/作者表示"这类套话）,
 "industry": 所属行业（如 人工智能 / 跨境电商，没有就空字符串）,
 "tech": 涉及的技术或产品（顿号分隔，没有就空字符串）,
 "tags": 3-6 个中文短标签,
 "summary": 2-4 句中文摘要：讲了什么、关键信息（工具、数字、做法）、结论；纯互动寒暄就一句话说明,
 "steps": 只有教程/实操类推文才填，按顺序列出步骤 [{"text": 步骤, "tools": 用到的工具, "tech": 技术点, "prep": 前提}]，其他推文给 [],
 "showFlowchart": steps 有 3 步及以上时为 true，否则 false,
 "kind": 帖子类型，只能是以下之一：教程实操（一步步教怎么做）/ 技术分享（讲技术原理、经验、方案）/ 工具资源推荐（推荐工具、仓库、文章、课程，本身不讲做法）/ 观点讨论 / 日常闲聊,
 "resources": 对"原文中的链接"逐个说明 [{"url": 原样照抄给出的链接, "name": 是什么（如"Claude Code 官方文档""@某人的推文""GitHub 仓库 xx"）, "note": 一句话说明内容或用途}]；看不出是什么就写"原文附带的链接"，不要编造，没有链接给 []}
如果原文附有【引用 …】或【回复 …】，那是对方的内容，用来理解上下文，摘要以作者本人的观点为主。`

type modelOut struct {
	Title         string   `json:"title"`
	Industry      string   `json:"industry"`
	Tech          string   `json:"tech"`
	Tags          []string `json:"tags"`
	Summary       string   `json:"summary"`
	Steps         []Step   `json:"steps"`
	ShowFlowchart bool     `json:"showFlowchart"`
	Kind          string   `json:"kind"`
	Resources     []Res    `json:"resources"`
}

// Str reads a string (or a JSON number) field of a raw item.
func Str(p map[string]any, key string) string {
	switch v := p[key].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	}
	return ""
}

// Analyze asks the relay model for the digest fields of one raw_posts.json
// item.
func Analyze(ctx context.Context, llm *translate.Client, raw map[string]any) (Post, error) {
	text, links := importer.RawContent(raw)
	user := fmt.Sprintf("作者：%s（@%s）\n发布时间：%s\n\n原文：\n%s", Str(raw, "author_name"), Str(raw, "author_username"),
		Str(raw, "created_at"), text)
	if len(links) > 0 {
		user += "\n\n原文中的链接：\n" + strings.Join(links, "\n")
	}
	out, err := llm.Complete(ctx, systemPrompt, user)
	var m modelOut
	if err == nil {
		err = parseModel(out, &m)
	}
	if err != nil {
		return Post{}, err
	}
	p := Post{
		ID: Str(raw, "id"), URL: Str(raw, "url"), Author: Str(raw, "author_username"), AuthorName: Str(raw, "author_name"),
		PublishedAt: Str(raw, "created_at"), Title: strings.TrimSpace(m.Title), Industry: strings.TrimSpace(m.Industry),
		Tech: strings.TrimSpace(m.Tech), Summary: strings.TrimSpace(m.Summary), Steps: m.Steps, Tags: CleanTags(m.Tags),
		ShowFlowchart: m.ShowFlowchart && len(m.Steps) >= 3,
		Resources:     KeepResources(m.Resources, links),
	}
	if k := strings.TrimSpace(m.Kind); PostKinds[k] {
		p.Kind = k
	}
	if p.Steps == nil {
		p.Steps = []Step{}
	}
	p.IsLong = len([]rune(p.Summary)) > 200
	return p, nil
}

// AnalyzeWithFallback runs Analyze on primary and, when that fails, once
// more on fallback (nil = no fallback).
func AnalyzeWithFallback(ctx context.Context, primary, fallback *translate.Client, raw map[string]any) (Post, error) {
	p, err := Analyze(ctx, primary, raw)
	if err != nil && fallback != nil {
		if p2, err2 := Analyze(ctx, fallback, raw); err2 == nil {
			return p2, nil
		}
	}
	return p, err
}

// KeepResources keeps the model's explanations only for links that are
// really in the post (never a URL the model made up), one per link.
func KeepResources(in []Res, links []string) []Res {
	allowed := map[string]bool{}
	for _, l := range links {
		allowed[l] = true
	}
	out := []Res{}
	seen := map[string]bool{}
	for _, r := range in {
		r.URL, r.Name, r.Note = strings.TrimSpace(r.URL), strings.TrimSpace(r.Name), strings.TrimSpace(r.Note)
		if allowed[r.URL] && !seen[r.URL] && r.Name != "" {
			seen[r.URL] = true
			out = append(out, r)
		}
	}
	return out
}

// parseModel reads the first {...} object from the reply (tolerates
// fences or chatter) and checks the required fields.
func parseModel(s string, m *modelOut) error {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return fmt.Errorf("reply is not JSON")
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), m); err != nil {
		return fmt.Errorf("reply JSON: %w", err)
	}
	if strings.TrimSpace(m.Title) == "" || strings.TrimSpace(m.Summary) == "" {
		return fmt.Errorf("reply missing title or summary")
	}
	return nil
}

func CleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t != "" && !seen[t] && len(out) < 8 {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
