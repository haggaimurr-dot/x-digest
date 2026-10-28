package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"xdigest/internal/model"
)

func TestSummaryConcurrencyEnv(t *testing.T) {
	for v, want := range map[string]int{"": 2, "1": 1, "8": 8, "0": 2, "99": 2, "x": 2} {
		os.Setenv("SUMMARY_CONCURRENCY", v)
		if got := summaryConcurrency(); got != want {
			t.Errorf("SUMMARY_CONCURRENCY=%q → %d, want %d", v, got, want)
		}
	}
	os.Unsetenv("SUMMARY_CONCURRENCY")
}

// A pending job: the stream sends "pending" at once, then the finished
// summary as soon as the job ends — one connection, no polling.
func TestSummaryStreamPushesOnDone(t *testing.T) {
	jobs := &summaryJobs{inflight: map[string]bool{}, waiters: map[string][]chan struct{}{}}
	jobs.claim("k")
	var finished atomic.Bool
	current := func() (model.YTSummary, error) {
		if finished.Load() {
			return model.YTSummary{Status: "done", Summary: "全文摘要"}, nil
		}
		return model.YTSummary{Status: "pending"}, nil
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSummaryStream(w, r, jobs, "k", current)
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	rd := bufio.NewReader(resp.Body)
	readEvent := func() string {
		var data string
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return data
			}
			if strings.HasPrefix(line, "data: ") {
				data = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			}
			if line == "\n" && data != "" {
				return data
			}
		}
	}
	if ev := readEvent(); !strings.Contains(ev, `"status":"pending"`) {
		t.Fatalf("first event %q", ev)
	}
	start := time.Now()
	go func() {
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
		jobs.release("k")
	}()
	if ev := readEvent(); !strings.Contains(ev, `"status":"done"`) || !strings.Contains(ev, "全文摘要") {
		t.Fatalf("second event %q", ev)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("push was not immediate")
	}
}

// Nothing running: one event and the stream ends.
func TestSummaryStreamNotPending(t *testing.T) {
	jobs := &summaryJobs{inflight: map[string]bool{}, waiters: map[string][]chan struct{}{}}
	rec := httptest.NewRecorder()
	serveSummaryStream(rec, httptest.NewRequest("GET", "/", nil), jobs, "k", func() (model.YTSummary, error) {
		return model.YTSummary{Status: "none"}, nil
	})
	if n := strings.Count(rec.Body.String(), "event: summary"); n != 1 {
		t.Fatalf("events = %d, body %q", n, rec.Body.String())
	}
}
