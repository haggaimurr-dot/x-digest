package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"xdigest/internal/importer"
	"xdigest/internal/store"
	"xdigest/internal/translate"
	"xdigest/internal/xpost"
)

// X daily: single-post import and per-account refresh (2026-10-28).
//
//	POST /api/v1/posts/import   {"url": "<x.com post link or id>", "force": bool}
//	     fetch (fxtwitter) → AI fields (relay, same prompt as cmd/digest) →
//	     posts + the author on the watchlist. 201 {post, account_created}
//	     200 {post, existed:true} when already stored (unless force).
//	POST /api/v1/x/refresh      {"usernames": [...]}  (empty = whole watchlist)
//	     starts a background job: Grok x_search finds each account's posts
//	     since its newest stored one, then each new post is imported as above.
//	     202 job status, 409 when one is running.
//	GET  /api/v1/x/refresh      job status (poll).
//
// Short replies (< xpost.MinChars) are skipped by the refresh, like the
// batch digest; an explicit single import always goes in.
//
//	DIGEST_MODEL     model for the AI fields (default TRANSLATE_MODEL)
//	X_REFRESH_DAYS   look-back for an account with no stored post (default 30)

type xImporter struct {
	st  *store.Store
	fx  *xpost.Fetcher
	gk  *xpost.Finder
	llm *translate.Client
	fb  *translate.Client // Grok fallback when llm refuses a post

	mu  sync.Mutex
	job *refreshJob
}

func newXImporter(st *store.Store) *xImporter {
	llm := translate.NewFromEnv()
	llm.HTTP = &http.Client{Timeout: 3 * time.Minute}
	if m := os.Getenv("DIGEST_MODEL"); m != "" {
		llm.Model = m
	}
	return &xImporter{st: st, fx: xpost.NewFetcherFromEnv(), gk: xpost.NewFinderFromEnv(), llm: llm, fb: xpost.NewGrokChatFromEnv()}
}

func (x *xImporter) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/posts/import", x.handleImport)
	mux.HandleFunc("/api/v1/x/refresh", x.handleRefresh)
}

// importRaw analyses one fetched post and stores it with its author.
func (x *xImporter) importRaw(ctx context.Context, raw map[string]any) (accountCreated bool, err error) {
	p, err := xpost.AnalyzeWithFallback(ctx, x.llm, x.fb, raw)
	if err != nil {
		return false, fmt.Errorf("AI 解析失败：%w", err)
	}
	text, links := importer.RawContent(raw)
	row := store.XPostRow{
		ID: p.ID, URL: p.URL, Author: p.Author, AuthorName: p.AuthorName, AuthorAvatar: xpost.Str(raw, "author_avatar"),
		PublishedAt: p.PublishedAt, Title: p.Title, Industry: p.Industry, Tech: p.Tech, Kind: p.Kind,
		Text: text, Links: links, Tags: p.Tags, IsLong: p.IsLong, ShowFlowchart: p.ShowFlowchart,
	}
	for _, s := range p.Steps {
		row.Steps = append(row.Steps, store.XPostStep{Text: s.Text, Tools: s.Tools, Tech: s.Tech, Prep: s.Prep})
	}
	for _, r := range p.Resources {
		row.Resources = append(row.Resources, store.XPostResource{URL: r.URL, Name: r.Name, Note: r.Note})
	}
	created, err := x.st.EnsureAuthor(p.Author, p.AuthorName, row.AuthorAvatar, xpost.Str(raw, "author_bio"))
	if err != nil {
		return false, err
	}
	if av := x.st.AuthorAvatar(p.Author); av != "" {
		row.AuthorAvatar = av
	}
	return created, x.st.UpsertXPost(row)
}

func (x *xImporter) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var body struct {
		URL   string `json:"url"`
		Force bool   `json:"force"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	id, err := xpost.ParseStatusURL(body.URL)
	if err != nil {
		writeErr(w, 422, "VALIDATION", err.Error())
		return
	}
	if !body.Force && x.st.PostExists(id) {
		p, err := x.st.GetPost(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"post": p, "existed": true})
		return
	}
	if !x.llm.Configured() {
		writeErr(w, 503, "AI_NOT_CONFIGURED", "AI 未配置：服务器需要设置 TRANSLATE_API_KEY")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	raw, err := x.fx.Fetch(ctx, id)
	if err != nil {
		code := 502
		if errors.Is(err, xpost.ErrNotFound) {
			code = 404
		}
		writeErr(w, code, "FETCH_FAILED", err.Error())
		return
	}
	created, err := x.importRaw(ctx, raw)
	if err != nil {
		writeErr(w, 502, "IMPORT_FAILED", err.Error())
		return
	}
	p, err := x.st.GetPost(id)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"post": p, "account_created": created})
}

// ---- refresh job ----

type refreshAccount struct {
	Username string `json:"username"`
	State    string `json:"state"` // pending | running | done | error
	Since    string `json:"since,omitempty"`
	Found    int    `json:"found"`
	Added    int    `json:"added"`
	Skipped  int    `json:"skipped"`
	Failed   int    `json:"failed"`
	Error    string `json:"error,omitempty"`
}

type refreshJob struct {
	Running    bool              `json:"running"`
	StartedAt  string            `json:"started_at"`
	FinishedAt string            `json:"finished_at,omitempty"`
	Total      int               `json:"total"`
	Done       int               `json:"done"`
	Added      int               `json:"added"`
	Accounts   []*refreshAccount `json:"accounts"`
}

func (x *xImporter) snapshot() map[string]any {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.job == nil {
		return map[string]any{"running": false, "accounts": []any{}, "configured": x.gk.Configured()}
	}
	accs := make([]refreshAccount, len(x.job.Accounts))
	for i, a := range x.job.Accounts {
		accs[i] = *a
	}
	return map[string]any{"running": x.job.Running, "started_at": x.job.StartedAt, "finished_at": x.job.FinishedAt,
		"total": x.job.Total, "done": x.job.Done, "added": x.job.Added, "accounts": accs, "configured": x.gk.Configured()}
}

func (x *xImporter) handleRefresh(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, x.snapshot())
	case http.MethodPost:
		var body struct {
			Usernames []string `json:"usernames"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if !x.gk.Configured() {
			writeErr(w, 503, "GROK_NOT_CONFIGURED", "账号更新未配置：服务器需要设置 GROK_API_KEY（中转站 Grok 分组的 key）")
			return
		}
		if !x.llm.Configured() {
			writeErr(w, 503, "AI_NOT_CONFIGURED", "AI 未配置：服务器需要设置 TRANSLATE_API_KEY")
			return
		}
		names := body.Usernames
		if len(names) == 0 {
			all, err := x.st.WatchUsernames()
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			names = all
		}
		x.mu.Lock()
		if x.job != nil && x.job.Running {
			x.mu.Unlock()
			writeErr(w, 409, "CONFLICT", "已有更新任务在运行")
			return
		}
		job := &refreshJob{Running: true, StartedAt: time.Now().UTC().Format(time.RFC3339), Total: len(names)}
		seen := map[string]bool{}
		for _, n := range names {
			n = strings.TrimPrefix(strings.TrimSpace(n), "@")
			if n != "" && !seen[strings.ToLower(n)] {
				seen[strings.ToLower(n)] = true
				job.Accounts = append(job.Accounts, &refreshAccount{Username: n, State: "pending"})
			}
		}
		job.Total = len(job.Accounts)
		x.job = job
		x.mu.Unlock()
		go x.runRefresh(job)
		writeJSON(w, 202, x.snapshot())
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

const (
	refreshAccountWorkers = 3
	refreshPostWorkers    = 3
)

func (x *xImporter) runRefresh(job *refreshJob) {
	queue := make(chan *refreshAccount)
	var wg sync.WaitGroup
	for i := 0; i < refreshAccountWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for a := range queue {
				x.refreshAccount(a)
				x.mu.Lock()
				job.Done++
				job.Added += a.Added
				x.mu.Unlock()
			}
		}()
	}
	for _, a := range job.Accounts {
		queue <- a
	}
	close(queue)
	wg.Wait()
	x.mu.Lock()
	job.Running = false
	job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	x.mu.Unlock()
	log.Printf("x refresh: %d accounts, %d new posts", job.Total, job.Added)
}

func (x *xImporter) set(f func()) {
	x.mu.Lock()
	defer x.mu.Unlock()
	f()
}

func (x *xImporter) refreshAccount(a *refreshAccount) {
	x.set(func() { a.State = "running" })
	since, ok, err := x.st.LatestPostTime(a.Username)
	if err != nil {
		x.set(func() { a.State, a.Error = "error", err.Error() })
		return
	}
	if !ok {
		days := atoiDefault(os.Getenv("X_REFRESH_DAYS"), 30)
		since = time.Now().UTC().AddDate(0, 0, -days)
	}
	x.set(func() { a.Since = since.Format(time.RFC3339) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	ids, err := x.gk.RecentIDs(ctx, a.Username, since)
	if err != nil {
		x.set(func() { a.State, a.Error = "error", err.Error() })
		return
	}
	x.set(func() { a.Found = len(ids) })
	// Posts of one account go through a few at a time (fetch + AI ≈ 10 s each).
	sem := make(chan struct{}, refreshPostWorkers)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer func() { <-sem; wg.Done() }()
			if x.st.PostExists(id) {
				x.set(func() { a.Skipped++ })
				return
			}
			raw, err := x.fx.Fetch(ctx, id)
			// Grok supplies ids only: keep a post only when X confirms it is
			// this account's, published after the newest stored one.
			if err == nil && !strings.EqualFold(xpost.Str(raw, "author_username"), a.Username) {
				err = errSkip
			}
			if err == nil {
				if t, e := time.Parse(time.RFC3339, xpost.Str(raw, "created_at")); e != nil || t.Before(since) {
					err = errSkip
				} else if importer.SubstantiveChars(raw) < xpost.MinChars {
					err = errSkip
				}
			}
			if err == nil {
				_, err = x.importRaw(ctx, raw)
			}
			switch {
			case err == nil:
				x.set(func() { a.Added++ })
			case errors.Is(err, errSkip), errors.Is(err, xpost.ErrNotFound):
				x.set(func() { a.Skipped++ })
			default:
				log.Printf("x refresh %s %s: %v", a.Username, id, err)
				x.set(func() { a.Failed++; a.Error = err.Error() })
			}
		}(id)
	}
	wg.Wait()
	x.set(func() { a.State = "done" })
}

var errSkip = errors.New("skip")
