package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"xdigest/internal/model"
)

// Summary push (2026-10-28): GET …/summary/stream?lang=… is a
// text/event-stream for the X post, YouTube and Bilibili summaries. It sends
// the current summary as a "summary" event at once; while a job is running
// it waits for the job to end and sends the final one, then closes. The
// page uses EventSource and falls back to polling GET …/summary when the
// stream fails. nginx must not buffer these paths (proxy_buffering off,
// see deploy/xdigest-18089.nginx).

const (
	streamMaxWait   = summaryJobTimeout + time.Minute
	streamHeartbeat = 25 * time.Second
)

func serveSummaryStream(w http.ResponseWriter, r *http.Request, jobs *summaryJobs, key string, current func() (model.YTSummary, error)) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "NO_STREAMING", "streaming unsupported")
		return
	}
	sum, err := current()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	send := func(s model.YTSummary) {
		b, _ := json.Marshal(map[string]any{"summary": s})
		fmt.Fprintf(w, "event: summary\ndata: %s\n\n", b)
		fl.Flush()
	}
	send(sum)
	if sum.Status != "pending" {
		return
	}
	done := jobs.done(key)
	tick := time.NewTicker(streamHeartbeat)
	defer tick.Stop()
	timeout := time.NewTimer(streamMaxWait)
	defer timeout.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-timeout.C:
			return
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-done:
			if sum, err = current(); err == nil {
				send(sum)
			}
			return
		}
	}
}
