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
// DIGEST_MODEL overrides the model (default: TRANSLATE_MODEL); with
// GROK_API_KEY set, a post that model refuses is retried on Grok.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"xdigest/internal/importer"
	"xdigest/internal/translate"
	"xdigest/internal/xpost"
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

	g := &generator{llm: llm, fb: xpost.NewGrokChatFromEnv(), path: postsPath, file: file, saveEvery: *saveEvery}
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

type postsFile struct {
	UpdatedAt string       `json:"updatedAt"`
	Posts     []xpost.Post `json:"posts"`
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// plan picks the raw items to generate, newest first.
func plan(raw []map[string]any, existing []xpost.Post, minChars int, force bool) (todo []map[string]any, short, done int) {
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

var str = xpost.Str

// ---- generation ----

type generator struct {
	llm       *translate.Client
	fb        *translate.Client // Grok fallback (GROK_API_KEY), may be nil
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

func (g *generator) one(raw map[string]any) {
	id := str(raw, "id")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	p, err := xpost.AnalyzeWithFallback(ctx, g.llm, g.fb, raw)
	if err != nil {
		log.Printf("post %s failed: %v", id, err)
		g.mu.Lock()
		g.failed++
		g.mu.Unlock()
		return
	}
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
