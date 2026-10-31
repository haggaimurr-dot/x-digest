package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Observability (2026-10-28).
//
// Logs: slog JSON on stderr (journald). slog.SetDefault also routes the
// existing log.Printf calls through the JSON handler, so every line is JSON.
// Each request logs method, path, status and duration.
//
// Metrics: GET /api/v1/metrics in the Prometheus text format (a small
// built-in registry, no client library):
//
//	xdigest_http_requests_total{method,code}
//	xdigest_http_request_duration_seconds (histogram)
//	xdigest_ai_calls_total{model}, xdigest_ai_failures_total{model}
//
// nginx denies /api/v1/metrics from outside; scrape it on 127.0.0.1.

func setupLogging() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
}

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

type metrics struct {
	mu       sync.Mutex
	requests map[[2]string]int64 // {method, code}
	buckets  []int64             // cumulative per durationBuckets, +Inf last
	sum      float64
	count    int64
	aiCalls  map[string]int64
	aiFails  map[string]int64
}

func newMetrics() *metrics {
	return &metrics{requests: map[[2]string]int64{}, buckets: make([]int64, len(durationBuckets)+1),
		aiCalls: map[string]int64{}, aiFails: map[string]int64{}}
}

func (m *metrics) observe(method string, code int, d time.Duration) {
	sec := d.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[[2]string{method, strconv.Itoa(code)}]++
	for i, b := range durationBuckets {
		if sec <= b {
			m.buckets[i]++
		}
	}
	m.buckets[len(durationBuckets)]++
	m.sum += sec
	m.count++
}

// aiCall is translate.CallHook.
func (m *metrics) aiCall(model string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aiCalls[model]++
	if err != nil {
		m.aiFails[model]++
	}
}

func (m *metrics) write(w *strings.Builder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.WriteString("# HELP xdigest_http_requests_total HTTP requests by method and status code.\n# TYPE xdigest_http_requests_total counter\n")
	keys := make([][2]string, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
	for _, k := range keys {
		fmt.Fprintf(w, "xdigest_http_requests_total{method=%q,code=%q} %d\n", k[0], k[1], m.requests[k])
	}
	w.WriteString("# HELP xdigest_http_request_duration_seconds HTTP request duration.\n# TYPE xdigest_http_request_duration_seconds histogram\n")
	for i, b := range durationBuckets {
		fmt.Fprintf(w, "xdigest_http_request_duration_seconds_bucket{le=%q} %d\n", strconv.FormatFloat(b, 'f', -1, 64), m.buckets[i])
	}
	fmt.Fprintf(w, "xdigest_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", m.buckets[len(durationBuckets)])
	fmt.Fprintf(w, "xdigest_http_request_duration_seconds_sum %g\nxdigest_http_request_duration_seconds_count %d\n", m.sum, m.count)
	for _, c := range []struct {
		name, help string
		data       map[string]int64
	}{{"xdigest_ai_calls_total", "Model calls by model.", m.aiCalls}, {"xdigest_ai_failures_total", "Failed model calls by model.", m.aiFails}} {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
		models := make([]string, 0, len(c.data))
		for k := range c.data {
			models = append(models, k)
		}
		sort.Strings(models)
		for _, k := range models {
			fmt.Fprintf(w, "%s{model=%q} %d\n", c.name, k, c.data[k])
		}
	}
}

func (m *metrics) handler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	m.write(&b)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) { s.code = code; s.ResponseWriter.WriteHeader(code) }

// Flush keeps SSE streaming working through the recorder.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// observeRequests logs and counts every request.
func (m *metrics) observeRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: 200}
		next.ServeHTTP(rec, r)
		d := time.Since(start)
		m.observe(r.Method, rec.code, d)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.code, "duration_ms", d.Milliseconds())
	})
}
