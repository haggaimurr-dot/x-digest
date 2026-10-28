package github

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Request pacing (2026-10-28). Every API call goes through limitedTransport:
//
//   - at most MaxConcurrent requests in flight (default 2);
//   - when the last response said fewer than LowWater calls remain, a request
//     waits for X-RateLimit-Reset — but only when its context allows it
//     (WithRateLimitWait, used by the background sync-all job). Interactive
//     calls fail fast as before (the 403 is reported as rate_limited);
//   - a secondary-limit 403/429 (quota not exhausted) is retried with
//     exponential backoff 1s → 2s → 4s (Retry-After wins when given);
//   - GET responses with an ETag are cached in memory and revalidated with
//     If-None-Match; GitHub does not count a 304 against the quota, so an
//     unchanged repository costs nothing on the next sync.

const (
	defaultMaxConcurrent = 2
	lowWater             = 5
	maxRetries           = 3
	maxCachedBody        = 1 << 20 // larger bodies (big release pages) are not cached
)

type waitKey struct{}

// WithRateLimitWait lets requests made with ctx sleep until the rate-limit
// reset (at most max) instead of failing.
func WithRateLimitWait(ctx context.Context, max time.Duration) context.Context {
	return context.WithValue(ctx, waitKey{}, max)
}

type cachedResp struct {
	etag   string
	status int
	header http.Header
	body   []byte
}

type limitedTransport struct {
	next  http.RoundTripper
	sem   chan struct{}
	sleep func(context.Context, time.Duration) error // swapped in tests
	now   func() time.Time

	mu        sync.Mutex
	remaining int // -1 = unknown
	reset     time.Time
	cache     map[string]cachedResp
	// OnWait is told when a request starts waiting for the reset (progress).
	onWait func(until time.Time)
}

func newLimitedTransport(next http.RoundTripper) *limitedTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &limitedTransport{
		next: next, sem: make(chan struct{}, defaultMaxConcurrent), sleep: sleepCtx, now: time.Now,
		remaining: -1, cache: map[string]cachedResp{},
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	select {
	case t.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-t.sem }()

	if err := t.waitForQuota(ctx); err != nil {
		return nil, err
	}
	key := req.URL.String() + "\x00" + req.Header.Get("Accept")
	t.mu.Lock()
	cached, hasCache := t.cache[key]
	t.mu.Unlock()

	for attempt := 0; ; attempt++ {
		r := req.Clone(ctx)
		if hasCache && req.Method == http.MethodGet {
			r.Header.Set("If-None-Match", cached.etag)
		}
		resp, err := t.next.RoundTrip(r)
		if err != nil {
			return nil, err
		}
		t.note(resp.Header)
		if resp.StatusCode == http.StatusNotModified && hasCache {
			resp.Body.Close()
			return cachedResponse(req, cached, resp.Header), nil
		}
		if retryable(resp) && attempt < maxRetries {
			d := backoff(resp, attempt)
			resp.Body.Close()
			if err := t.sleep(ctx, d); err != nil {
				return nil, err
			}
			continue
		}
		if req.Method == http.MethodGet && resp.StatusCode == http.StatusOK && resp.Header.Get("ETag") != "" {
			return t.store(key, resp)
		}
		return resp, nil
	}
}

// waitForQuota sleeps until the reset when quota is (nearly) gone and the
// context allows waiting.
func (t *limitedTransport) waitForQuota(ctx context.Context) error {
	max, ok := ctx.Value(waitKey{}).(time.Duration)
	if !ok || max <= 0 {
		return nil
	}
	t.mu.Lock()
	low := t.remaining >= 0 && t.remaining < lowWater
	until := t.reset
	onWait := t.onWait
	t.mu.Unlock()
	if !low {
		return nil
	}
	d := until.Sub(t.now()) + time.Second
	if d <= 0 {
		return nil
	}
	if d > max {
		d = max
	}
	if onWait != nil {
		onWait(t.now().Add(d))
	}
	if err := t.sleep(ctx, d); err != nil {
		return err
	}
	t.mu.Lock()
	t.remaining = -1 // unknown until the next response
	t.mu.Unlock()
	return nil
}

func (t *limitedTransport) note(h http.Header) {
	rl := parseRateLimit(h)
	if !rl.hasHeader {
		return
	}
	t.mu.Lock()
	t.remaining, t.reset = rl.remaining, rl.reset
	t.mu.Unlock()
}

// retryable: a secondary rate limit (403 with quota left, or 429). A 403
// with remaining 0 is the primary limit and is reported, not retried.
func retryable(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden && !parseRateLimit(resp.Header).exhausted() &&
		(resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") != "")
}

func backoff(resp *http.Response, attempt int) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 120 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Second << attempt // 1s, 2s, 4s
}

func (t *limitedTransport) store(key string, resp *http.Response) (*http.Response, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCachedBody+1))
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	if len(body) > maxCachedBody {
		// Too big to keep: hand back the full stream untouched.
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), resp.Body), resp.Body}
		return resp, nil
	}
	resp.Body.Close()
	t.mu.Lock()
	t.cache[key] = cachedResp{etag: resp.Header.Get("ETag"), status: resp.StatusCode, header: resp.Header.Clone(), body: body}
	t.mu.Unlock()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

func cachedResponse(req *http.Request, c cachedResp, fresh http.Header) *http.Response {
	h := c.header.Clone()
	for _, k := range []string{"X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if v := fresh.Get(k); v != "" {
			h.Set(k, v)
		}
	}
	h.Set("X-From-Cache", "1")
	return &http.Response{
		Status: "200 OK", StatusCode: c.status, Header: h, Request: req,
		Body: io.NopCloser(bytes.NewReader(c.body)), ContentLength: int64(len(c.body)),
		Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
	}
}

// SetWaitHook registers fn to be told when a request waits for the rate
// limit reset (used for "等待限流至 hh:mm" progress).
func (c *Client) SetWaitHook(fn func(until time.Time)) {
	if lt, ok := c.HTTP.Transport.(*limitedTransport); ok {
		lt.mu.Lock()
		lt.onWait = fn
		lt.mu.Unlock()
	}
}
