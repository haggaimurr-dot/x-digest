package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAIQuota(t *testing.T) {
	q := newAIQuota(3)
	now := time.Date(2026, 10, 28, 23, 0, 0, 0, shanghaiTZ)
	q.now = func() time.Time { return now }
	h := q.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	do := func(method, path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec.Code
	}
	for i := 0; i < 3; i++ {
		if c := do("POST", "/api/v1/posts/p1/summary"); c != 200 {
			t.Fatalf("call %d: %d", i+1, c)
		}
	}
	if c := do("POST", "/api/v1/yt/videos/v1/summary"); c != 429 {
		t.Fatalf("4th AI call: %d, want 429", c)
	}
	// Reads and non-AI writes are not counted.
	for _, c := range []struct{ m, p string }{{"GET", "/api/v1/posts/p1/summary"}, {"POST", "/api/v1/replicas"}, {"GET", "/api/v1/posts"}} {
		if code := do(c.m, c.p); code != 200 {
			t.Fatalf("%s %s: %d", c.m, c.p, code)
		}
	}
	// A new Shanghai day resets the count.
	now = now.Add(2 * time.Hour)
	if c := do("POST", "/api/v1/translate"); c != 200 {
		t.Fatalf("next day: %d", c)
	}
}

func TestAIQuotaUnlimited(t *testing.T) {
	h := newAIQuota(0).wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/translate", nil))
		if rec.Code != 204 {
			t.Fatal("unlimited quota blocked a request")
		}
	}
}
