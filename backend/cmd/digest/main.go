// Command digest turns data/raw_posts.json into data/posts.json entries
// (the X daily digest) by asking the TRANSLATE_* chat relay for a headline,
// industry, tech, tags, a short summary and — for tutorials — steps
// (2026-10-27).
//
// Usage (repo root as -data-dir; the relay key comes from the environment,
// e.g. /etc/xdigest/api.env):
//
//	set -a; . /etc/xdigest/api.env; set +a
//	go run ./cmd/digest -data-dir /opt/xdigest -dry-run      # counts only
//	go run ./cmd/digest -data-dir /opt/xdigest               # generate
//	xdigest-import -data-dir /opt/xdigest -dsn '...'         # then import
//
// Rules:
//   - short replies are skipped: under -min-chars characters left after
//     removing links and @mentions (default 15);
//   - posts already in posts.json are skipped (re-runs resume where the
//     last run stopped); -force regenerates them;
//   - progress is written to posts.json every -save-every results, so an
//     interrupted run loses at most that many;
//   - a post whose call fails is left out and retried on the next run.
//
// The original text sent to the model is importer.RawContent: t.co links
// expanded and quoted / replied-to text appended when the raw item has them.
// DIGEST_MODEL overrides the model (default: TRANSLATE_MODEL).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"xdigest/internal/importer"
	"xdigest/internal/translate"
)

func main() {
	dataDir := flag.String("data-dir", ".", "repo root containing data/raw_posts.json and data/posts.json")
	minChars := flag.Int("min-chars", 15, "skip posts with fewer characters after removing links and @mentions")
	limit := flag.Int("limit", 0, "generate at most N posts this run (0 = all)")
	conc := flag.Int("concurrency", 3, "parallel relay calls")
	saveEvery := flag.Int("save-every", 10, "write posts.json after this many new results")
	force := flag.Bool("force", false, "regenerate posts that are already in posts.json")
	dryRun := flag.Bool("dry-run", false, "only count what would be generated")
	flag.Parse()

	rawPath := filepath.Join(*dataDir, "data", "raw_posts.json")
	postsPath := filepath.Join(*dataDir, "data", "posts.json")
	var raw []map[string]any
	if err := readJSON(rawPath, &raw); err != nil {
		log.Fatalf("raw_posts.json: %v", err)
	}
	var file postsFile
	if err := readJSON(postsPath, &file); err != nil && !os.IsNotExist(err) {
		log.Fatalf("posts.json: %v", err)
	}
	todo, skippedShort, skippedDone := plan(raw, file.Posts, *minChars, *force)
	if *limit > 0 && len(todo) > *limit {
		todo = todo[:*limit]
	}
	log.Printf("raw %d | short replies skipped %d | already in posts.json %d | to generate %d",
		len(raw), skippedShort, skippedDone, len(todo))
	if *dryRun || len(todo) == 0 {
		return
	}

	llm := translate.NewFromEnv()
	if !llm.Configured() {
		log.Fatal("TRANSLATE_API_KEY is not set (load /etc/xdigest/api.env first)")
	}
	llm.HTTP = &http.Client{Timeout: 2 * time.Minute}
	if m := os.Getenv("DIGEST_MODEL"); m != "" {
		llm.Model = m
	}

	g := &generator{llm: llm, path: postsPath, file: file, saveEvery: *saveEvery}
	g.index()
	jobs := make(chan map[string]any)
	var wg sync.WaitGroup
	for i := 0; i < *conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				g.one(p)
			}
		}()
	}
	for _, p := range todo {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	if err := g.save(); err != nil {
		log.Fatalf("write posts.json: %v", err)
	}
	log.Printf("done: generated %d, failed %d (failed ones are retried on the next run)", g.ok, g.failed)
}

// ---- posts.json shape (same as the existing file / the importer) ----

type step struct {
	Text  string `json:"text"`
	Tools string `json:"tools"`
	Tech  string `json:"tech"`
	Prep  string `json:"prep"`
}

type post struct {
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Author        string   `json:"author"`
	AuthorName    string   `json:"authorName"`
	PublishedAt   string   `json:"publishedAt"`
	Title         string   `json:"title"`
	Industry      string   `json:"industry"`
	Tech          string   `json:"tech"`
	Summary       string   `json:"summary"`
	Steps         []step   `json:"steps"`
	Tags          []string `json:"tags"`
	IsLong        bool     `json:"isLong"`
	ShowFlowchart bool     `json:"showFlowchart"`
	Kind          string   `json:"kind"`
	Resources     []res    `json:"resources"`
}

// res explains one link of the post (only links that are in the post).
type res struct {
	URL  string `json:"url"`
	Name string `json:"name"`
	Note string `json:"note"`
}

// postKinds are the allowed post types; anything else is stored as "".
var postKinds = map[string]bool{"教程实操": true, "技术分享": true, "工具资源推荐": true, "观点讨论": true, "日常闲聊": true}

type postsFile struct {
	UpdatedAt string `json:"updatedAt"`
	Posts     []post `json:"posts"`
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func str(p map[string]any, key string) string {
	switch v := p[key].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	}
	return ""
}

// plan picks the raw items to generate, newest first.
func plan(raw []map[string]any, existing []post, minChars int, force bool) (todo []map[string]any, short, done int) {
	have := map[string]bool{}
	for _, p := range existing {
		have[p.ID] = true
	}
	for _, p := range raw {
		id := str(p, "id")
		if id == "" {
			continue
		}
		if importer.SubstantiveChars(p) < minChars {
			short++
			continue
		}
		if have[id] && !force {
			done++
			continue
		}
		todo = append(todo, p)
	}
	sort.SliceStable(todo, func(i, j int) bool { return str(todo[i], "created_at") > str(todo[j], "created_at") })
	return todo, short, done
}

// ---- generation ----

type generator struct {
	llm       *translate.Client
	path      string
	saveEvery int

	mu      sync.Mutex
	file    postsFile
	byID    map[string]int
	ok      int
	failed  int
	pending int
}

func (g *generator) index() {
	g.byID = map[string]int{}
	for i, p := range g.file.Posts {
		g.byID[p.ID] = i
	}
}

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
	Steps         []step   `json:"steps"`
	ShowFlowchart bool     `json:"showFlowchart"`
	Kind          string   `json:"kind"`
	Resources     []res    `json:"resources"`
}

// keepResources keeps the model's explanations only for links that are
// really in the post (never a URL the model made up), one per link.
func keepResources(in []res, links []string) []res {
	allowed := map[string]bool{}
	for _, l := range links {
		allowed[l] = true
	}
	out := []res{}
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

func (g *generator) one(raw map[string]any) {
	id := str(raw, "id")
	text, links := importer.RawContent(raw)
	user := fmt.Sprintf("作者：%s（@%s）\n发布时间：%s\n\n原文：\n%s", str(raw, "author_name"), str(raw, "author_username"),
		str(raw, "created_at"), text)
	if len(links) > 0 {
		user += "\n\n原文中的链接：\n" + strings.Join(links, "\n")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := g.llm.Complete(ctx, systemPrompt, user)
	var m modelOut
	if err == nil {
		err = parseModel(out, &m)
	}
	if err != nil {
		log.Printf("post %s failed: %v", id, err)
		g.mu.Lock()
		g.failed++
		g.mu.Unlock()
		return
	}
	p := post{
		ID: id, URL: str(raw, "url"), Author: str(raw, "author_username"), AuthorName: str(raw, "author_name"),
		PublishedAt: str(raw, "created_at"), Title: strings.TrimSpace(m.Title), Industry: strings.TrimSpace(m.Industry),
		Tech: strings.TrimSpace(m.Tech), Summary: strings.TrimSpace(m.Summary), Steps: m.Steps, Tags: cleanTags(m.Tags),
		ShowFlowchart: m.ShowFlowchart && len(m.Steps) >= 3,
		Resources:     keepResources(m.Resources, links),
	}
	if k := strings.TrimSpace(m.Kind); postKinds[k] {
		p.Kind = k
	}
	if p.Steps == nil {
		p.Steps = []step{}
	}
	p.IsLong = len([]rune(p.Summary)) > 200
	g.mu.Lock()
	defer g.mu.Unlock()
	if i, ok := g.byID[id]; ok {
		g.file.Posts[i] = p
	} else {
		g.byID[id] = len(g.file.Posts)
		g.file.Posts = append(g.file.Posts, p)
	}
	g.ok++
	g.pending++
	if g.pending >= g.saveEvery {
		if err := g.saveLocked(); err != nil {
			log.Printf("write posts.json: %v", err)
		} else {
			log.Printf("saved: %d generated so far", g.ok)
		}
	}
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

func cleanTags(in []string) []string {
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

func (g *generator) save() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.saveLocked()
}

// saveLocked writes posts.json atomically (temp file + rename), newest
// first.
func (g *generator) saveLocked() error {
	sort.SliceStable(g.file.Posts, func(i, j int) bool { return g.file.Posts[i].PublishedAt > g.file.Posts[j].PublishedAt })
	g.index()
	g.file.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(g.file, "", "  ")
	if err != nil {
		return err
	}
	tmp := g.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, g.path); err != nil {
		return err
	}
	g.pending = 0
	return nil
}
