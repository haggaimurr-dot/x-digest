package main

// The AI summary job pool shared by the video hub (hub_summary.go) and the
// post summaries (2026-10-27). Jobs run in the background because a long
// video takes well over a minute, longer than a proxied request should stay
// open. At most summaryConcurrency jobs run at once; one key (platform,
// video, lang) never runs twice at the same time. There is no login (see
// README "Security note"): anyone who can reach the API can spend model
// calls, which is why a finished summary is served from the database and
// only force=1 regenerates it.

import (
	"net/http"
	"os"
	"sync"
	"time"

	"xdigest/internal/translate"
)

const (
	defaultSummaryConcurrency = 2
	summaryJobTimeout         = 10 * time.Minute
	// summaryHTTPTimeout is per model call; a long final call over a
	// big transcript can take a couple of minutes.
	summaryHTTPTimeout = 3 * time.Minute
	maxSummaryErrChars = 500
)

type summaryJobs struct {
	llm      *translate.Client
	sem      chan struct{}
	mu       sync.Mutex
	inflight map[string]bool
	// waiters are closed when the key's job ends (SSE, summary_stream.go).
	waiters map[string][]chan struct{}
}

// summaryConcurrency: SUMMARY_CONCURRENCY (1–16), default 2. Shared by
// video and post summaries.
func summaryConcurrency() int {
	n := atoiDefault(os.Getenv("SUMMARY_CONCURRENCY"), defaultSummaryConcurrency)
	if n < 1 || n > 16 {
		return defaultSummaryConcurrency
	}
	return n
}

func newSummaryJobs() *summaryJobs {
	llm := translate.NewFromEnv()
	llm.HTTP = &http.Client{Timeout: summaryHTTPTimeout}
	// Video summaries use deepseek-v4-pro (user's choice, 2026-10-27);
	// translation stays on TRANSLATE_MODEL (deepseek-flash).
	if m := os.Getenv("YT_SUMMARY_MODEL"); m != "" {
		llm.Model = m
	} else {
		llm.Model = "deepseek-v4-pro"
	}
	return &summaryJobs{llm: llm, sem: make(chan struct{}, summaryConcurrency()), inflight: map[string]bool{}, waiters: map[string][]chan struct{}{}}
}

func (j *summaryJobs) running(key string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.inflight[key]
}

// claim marks key in flight; false when it already is.
func (j *summaryJobs) claim(key string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inflight[key] {
		return false
	}
	j.inflight[key] = true
	return true
}

func (j *summaryJobs) release(key string) {
	j.mu.Lock()
	delete(j.inflight, key)
	ws := j.waiters[key]
	delete(j.waiters, key)
	j.mu.Unlock()
	for _, w := range ws {
		close(w)
	}
}

// done returns a channel closed when key's job ends (already closed when
// none is running).
func (j *summaryJobs) done(key string) <-chan struct{} {
	ch := make(chan struct{})
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.inflight[key] {
		close(ch)
		return ch
	}
	j.waiters[key] = append(j.waiters[key], ch)
	return ch
}
