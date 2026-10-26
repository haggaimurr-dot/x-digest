package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// mapCache is an in-memory Cache for tests.
type mapCache struct {
	mu   sync.Mutex
	data map[string]string // hash + "\x00" + target -> translation
}

func newMapCache() *mapCache { return &mapCache{data: map[string]string{}} }

func (c *mapCache) GetTranslations(hashes []string, target string) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]string{}
	for _, h := range hashes {
		if v, ok := c.data[h+"\x00"+target]; ok {
			out[h] = v
		}
	}
	return out, nil
}

func (c *mapCache) PutTranslations(target string, items map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for src, tr := range items {
		c.data[Hash(src, target)+"\x00"+target] = tr
	}
	return nil
}

// mockRelay is a canned OpenAI-compatible chat endpoint. Batch requests
// (user message is a JSON array) get a JSON array of "TR:"+item; single
// requests get "TR:"+text. Every response includes reasoning_content with
// deliberately wrong text, so tests prove only content is used.
type mockRelay struct {
	mu       sync.Mutex
	calls    int
	batches  int
	singles  int
	failNext int // number of upcoming calls that return HTTP 500
	batchBad bool // if true, batch calls return non-JSON content
}

func (m *mockRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.calls++
	if m.failNext > 0 {
		m.failNext--
		m.mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
		return
	}
	m.mu.Unlock()

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)
	user := ""
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			user = msg.Content
		}
	}
	var content string
	trimmed := strings.TrimSpace(user)
	if strings.HasPrefix(trimmed, "[") {
		m.mu.Lock()
		m.batches++
		bad := m.batchBad
		m.mu.Unlock()
		if bad {
			content = "this is not a json array"
		} else {
			var items []string
			if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			trs := make([]string, len(items))
			for i, s := range items {
				trs[i] = "TR:" + s
			}
			raw, _ := json.Marshal(trs)
			content = string(raw)
		}
	} else {
		m.mu.Lock()
		m.singles++
		m.mu.Unlock()
		content = "TR:" + user
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":"mock","object":"chat.completion","model":"%s","choices":[{"index":0,"message":{"role":"assistant","content":%s,"reasoning_content":"WRONG REASONING, MUST BE IGNORED"},"finish_reason":"stop"}]}`,
		req.Model, mustJSON(content))
}

func mustJSON(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func (m *mockRelay) counts() (calls, batches, singles int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls, m.batches, m.singles
}

// newTestClientServer returns a client pointed at a mock relay plus the
// server (caller must Close it).
func newTestClientServer(relay *mockRelay) (*Client, *httptest.Server) {
	srv := httptest.NewServer(relay)
	return &Client{HTTP: srv.Client(), BaseURL: srv.URL, APIKey: "test-key", Model: "deepseek-flash"}, srv
}

func TestContainsCJK(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"", false},
		{"hello world", false},
		{"Hello, 世界", true},
		{"人工智能", true},
		{"123 ！", false}, // fullwidth punctuation is not CJK, but "！" is U+FF01
		{"，。", true},     // CJK punctuation counts
		{"Test 测试 test", true},
	}
	for _, tc := range cases {
		if got := ContainsCJK(tc.s); got != tc.want {
			t.Errorf("ContainsCJK(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestNeedsTranslation(t *testing.T) {
	cases := []struct {
		s, target string
		want      bool
	}{
		{"", "en", false},
		{"   ", "en", false},
		{"hello", "en", false},       // already English
		{"你好", "en", true},         // Chinese -> English
		{"你好 hello", "en", true},   // mixed -> English
		{"hello", "zh-CN", true},     // English -> Chinese
		{"你好", "zh-CN", false},     // already Chinese
		{"你好 hello", "zh-CN", false}, // mixed contains CJK -> as-is
	}
	for _, tc := range cases {
		if got := NeedsTranslation(tc.s, tc.target); got != tc.want {
			t.Errorf("NeedsTranslation(%q, %q) = %v, want %v", tc.s, tc.target, got, tc.want)
		}
	}
}

func TestHash(t *testing.T) {
	a := Hash("你好", "en")
	if a == Hash("你好", "zh-CN") {
		t.Error("hash must differ by target")
	}
	if a != Hash("你好", "en") {
		t.Error("hash must be deterministic")
	}
	if len(a) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA-256 hex)", len(a))
	}
}

func TestParseJSONArray(t *testing.T) {
	got, err := parseJSONArray("[\"a\", \"b\"]")
	if err != nil || len(got) != 2 || got[1] != "b" {
		t.Fatalf("plain array: got %v, err %v", got, err)
	}
	got, err = parseJSONArray("```json\n[\"x\"]\n```")
	if err != nil || len(got) != 1 || got[0] != "x" {
		t.Fatalf("fenced array: got %v, err %v", got, err)
	}
	if _, err := parseJSONArray("no json here"); err == nil {
		t.Error("garbage should error")
	}
}

func TestClientBatchUsesContentNotReasoning(t *testing.T) {
	relay := &mockRelay{}
	c, srv := newTestClientServer(relay)
	defer srv.Close()
	results := c.Translate(context.Background(), []string{"你好世界"}, "en")
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].Text != "TR:你好世界" || !results[0].Translated {
		t.Errorf("got %+v, want translated TR:你好世界", results[0])
	}
	if strings.Contains(results[0].Text, "WRONG") {
		t.Error("reasoning_content leaked into the translation")
	}
	calls, batches, singles := relay.counts()
	if calls != 1 || batches != 1 || singles != 0 {
		t.Errorf("calls=%d batches=%d singles=%d, want 1/1/0", calls, batches, singles)
	}
}

func TestClientBatchFallbackToSingles(t *testing.T) {
	relay := &mockRelay{batchBad: true}
	c, srv := newTestClientServer(relay)
	defer srv.Close()
	results := c.Translate(context.Background(), []string{"你好", "世界"}, "en")
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	for i, want := range []string{"TR:你好", "TR:世界"} {
		if results[i].Text != want || !results[i].Translated {
			t.Errorf("result[%d] = %+v, want translated %q", i, results[i], want)
		}
	}
	calls, batches, singles := relay.counts()
	if batches != 1 || singles != 2 {
		t.Errorf("calls=%d batches=%d singles=%d, want batch 1 + singles 2", calls, batches, singles)
	}
}

func TestClientUpstreamFailureReturnsOriginal(t *testing.T) {
	relay := &mockRelay{failNext: 100} // every call fails
	c, srv := newTestClientServer(relay)
	defer srv.Close()
	results := c.Translate(context.Background(), []string{"你好", "hello"}, "en")
	if results[0].Text != "你好" || results[0].Translated {
		t.Errorf("failed text should fall back to original untranslated: %+v", results[0])
	}
	if results[1].Text != "hello" || results[1].Translated {
		t.Errorf("already-English text should pass through: %+v", results[1])
	}
}

func TestServiceCacheHitAndMiss(t *testing.T) {
	relay := &mockRelay{}
	c, srv := newTestClientServer(relay)
	defer srv.Close()
	cache := newMapCache()
	// Pre-seed the cache with one translation.
	if err := cache.PutTranslations("en", map[string]string{"你好": "Hello"}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(c, cache)
	out, err := svc.Translate(context.Background(), []string{"你好", "世界"}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != "Hello" {
		t.Errorf("cached item: got %q, want Hello", out[0])
	}
	if out[1] != "TR:世界" {
		t.Errorf("missed item: got %q, want TR:世界", out[1])
	}
	calls, batches, singles := relay.counts()
	if calls != 1 || batches != 1 {
		t.Errorf("expected exactly 1 batch call for the miss, got calls=%d batches=%d singles=%d", calls, batches, singles)
	}
	// Second call: everything cached, no relay traffic at all.
	out2, err := svc.Translate(context.Background(), []string{"你好", "世界"}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if out2[0] != "Hello" || out2[1] != "TR:世界" {
		t.Errorf("second call: got %v", out2)
	}
	if calls2, _, _ := relay.counts(); calls2 != 1 {
		t.Errorf("second call must not hit the relay, total calls=%d", calls2)
	}
}

func TestServiceShortCircuitNoAPICall(t *testing.T) {
	relay := &mockRelay{}
	c, srv := newTestClientServer(relay)
	defer srv.Close()
	svc := NewService(c, newMapCache())
	out, err := svc.Translate(context.Background(), []string{"hello", "Hello world", ""}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != "hello" || out[1] != "Hello world" || out[2] != "" {
		t.Errorf("already-English/empty texts must pass through: %v", out)
	}
	if calls, _, _ := relay.counts(); calls != 0 {
		t.Errorf("no relay calls expected, got %d", calls)
	}
}

func TestServiceNotConfigured(t *testing.T) {
	svc := NewService(&Client{APIKey: ""}, newMapCache())
	_, err := svc.Translate(context.Background(), []string{"你好"}, "en")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestServiceInvalidTarget(t *testing.T) {
	relay := &mockRelay{}
	c, srv := newTestClientServer(relay)
	defer srv.Close()
	svc := NewService(c, newMapCache())
	_, err := svc.Translate(context.Background(), []string{"hello"}, "fr")
	if err == nil {
		t.Error("invalid target should error")
	}
}
