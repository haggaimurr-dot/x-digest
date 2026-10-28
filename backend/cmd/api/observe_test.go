package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAndRequestLog(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	m := newMetrics()
	h := m.observeRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(404)
		}
	}))
	for _, p := range []string{"/api/v1/posts", "/api/v1/posts", "/missing"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}
	m.aiCall("deepseek-flash", nil)
	m.aiCall("deepseek-flash", errors.New("upstream 500"))

	var out strings.Builder
	m.write(&out)
	text := out.String()
	for _, want := range []string{
		`xdigest_http_requests_total{method="GET",code="200"} 2`,
		`xdigest_http_requests_total{method="GET",code="404"} 1`,
		`xdigest_http_request_duration_seconds_bucket{le="+Inf"} 3`,
		`xdigest_http_request_duration_seconds_count 3`,
		`xdigest_ai_calls_total{model="deepseek-flash"} 2`,
		`xdigest_ai_failures_total{model="deepseek-flash"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics lack %q:\n%s", want, text)
		}
	}
	// Request log lines are JSON with method/path/status/duration.
	line := strings.SplitN(buf.String(), "\n", 2)[0]
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("log line is not JSON: %q", line)
	}
	for _, k := range []string{"method", "path", "status", "duration_ms"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("log line lacks %s: %v", k, rec)
		}
	}
	// The old log.Printf calls go through slog as JSON too.
	buf.Reset()
	log.Printf("legacy %d", 1)
	if !strings.Contains(buf.String(), `"msg":"legacy 1"`) {
		t.Errorf("log.Printf not routed to slog JSON: %q", buf.String())
	}
}
