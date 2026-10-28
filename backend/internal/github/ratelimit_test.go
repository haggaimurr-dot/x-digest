package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestTransport() (*limitedTransport, *[]time.Duration) {
	lt := newLimitedTransport(nil)
	var slept []time.Duration
	var mu sync.Mutex
	lt.sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		slept = append(slept, d)
		mu.Unlock()
		return nil
	}
	return lt, &slept
}

func get(t *testing.T, c *http.Client, ctx context.Context, url string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

// Unchanged resource: the second GET sends If-None-Match, gets 304 and the
// caller still sees 200 with the cached body.
func TestETagRevalidation(t *testing.T) {
	var conditional int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "50")
		if r.Header.Get("If-None-Match") == `"v1"` {
			atomic.AddInt32(&conditional, 1)
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, `{"name":"repo"}`)
	}))
	defer srv.Close()
	lt, _ := newTestTransport()
	c := &http.Client{Transport: lt}
	for i := 0; i < 2; i++ {
		resp, body := get(t, c, context.Background(), srv.URL+"/repos/a/b")
		if resp.StatusCode != 200 || body != `{"name":"repo"}` {
			t.Fatalf("round %d: %d %q", i, resp.StatusCode, body)
		}
		if i == 1 && resp.Header.Get("X-From-Cache") != "1" {
			t.Fatal("second response not from cache")
		}
	}
	if conditional != 1 {
		t.Fatalf("conditional requests = %d, want 1", conditional)
	}
}

// Secondary limit (403 with quota left / 429): retried with 1s, 2s backoff.
func TestSecondaryLimitBackoff(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch atomic.AddInt32(&n, 1) {
		case 1:
			w.Header().Set("X-RateLimit-Remaining", "30")
			w.WriteHeader(403)
		case 2:
			w.WriteHeader(429)
		default:
			fmt.Fprint(w, "ok")
		}
	}))
	defer srv.Close()
	lt, slept := newTestTransport()
	resp, body := get(t, &http.Client{Transport: lt}, context.Background(), srv.URL)
	if resp.StatusCode != 200 || body != "ok" {
		t.Fatalf("%d %q", resp.StatusCode, body)
	}
	if len(*slept) != 2 || (*slept)[0] != time.Second || (*slept)[1] != 2*time.Second {
		t.Fatalf("backoff = %v", *slept)
	}
}

// Primary limit exhausted: not retried (the caller reports rate_limited).
func TestPrimaryLimitNotRetried(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
		w.WriteHeader(403)
	}))
	defer srv.Close()
	lt, slept := newTestTransport()
	resp, _ := get(t, &http.Client{Transport: lt}, context.Background(), srv.URL)
	if resp.StatusCode != 403 || n != 1 || len(*slept) != 0 {
		t.Fatalf("status %d, requests %d, slept %v", resp.StatusCode, n, *slept)
	}
}

// Low quota: a waiting context sleeps until the reset; an interactive one
// does not wait.
func TestWaitForReset(t *testing.T) {
	reset := time.Now().Add(10 * time.Minute)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "2")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset.Unix()))
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	lt, slept := newTestTransport()
	var waitedUntil time.Time
	lt.onWait = func(u time.Time) { waitedUntil = u }
	c := &http.Client{Transport: lt}
	get(t, c, context.Background(), srv.URL) // learns remaining=2
	get(t, c, context.Background(), srv.URL) // interactive: no wait
	if len(*slept) != 0 {
		t.Fatalf("interactive request waited: %v", *slept)
	}
	get(t, c, WithRateLimitWait(context.Background(), time.Hour), srv.URL)
	if len(*slept) != 1 || (*slept)[0] < 9*time.Minute || (*slept)[0] > 11*time.Minute {
		t.Fatalf("background wait = %v", *slept)
	}
	if waitedUntil.IsZero() {
		t.Fatal("wait hook not called")
	}
}

// At most two requests are in flight at once.
func TestConcurrencyCap(t *testing.T) {
	var cur, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := atomic.AddInt32(&cur, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if v <= p || atomic.CompareAndSwapInt32(&peak, p, v) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	lt, _ := newTestTransport()
	c := &http.Client{Transport: lt}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); get(t, c, context.Background(), srv.URL) }()
	}
	wg.Wait()
	if peak > 2 {
		t.Fatalf("peak concurrency %d > 2", peak)
	}
}
