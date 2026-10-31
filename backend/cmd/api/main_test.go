package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"xdigest/internal/translate"
)

// testCache is an in-memory translate.Cache for handler tests.
type testCache struct {
	mu   sync.Mutex
	data map[string]string
}

func newTestCache() *testCache { return &testCache{data: map[string]string{}} }

func (c *testCache) GetTranslations(hashes []string, target string) (map[string]string, error) {
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

func (c *testCache) PutTranslations(target string, items map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for src, tr := range items {
		c.data[translate.Hash(src, target)+"\x00"+target] = tr
	}
	return nil
}

// testRelay is a canned OpenAI-compatible chat endpoint: a JSON-array user
// message gets a JSON array of "TR:"+item back, anything else gets
// "TR:"+text. It also counts calls so tests can prove caching works.
type testRelay struct {
	mu    sync.Mutex
	calls int
}

func (m *testRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	var req struct {
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
	if strings.HasPrefix(strings.TrimSpace(user), "[") {
		var items []string
		if err := json.Unmarshal([]byte(strings.TrimSpace(user)), &items); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		trs := make([]string, len(items))
		for i, s := range items {
			trs[i] = "TR:" + s
		}
		raw, _ := json.Marshal(trs)
		content = string(raw)
	} else {
		content = "TR:" + user
	}
	raw, _ := json.Marshal(content)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + string(raw) + `}}]}`))
}

func (m *testRelay) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func newTestServer(t *testing.T, apiKey string) (*server, *testRelay, *httptest.Server) {
	t.Helper()
	relay := &testRelay{}
	srv := httptest.NewServer(relay)
	client := &translate.Client{HTTP: srv.Client(), BaseURL: srv.URL, APIKey: apiKey, Model: "deepseek-flash"}
	return &server{tr: translate.NewService(client, newTestCache())}, relay, srv
}

func postTranslate(t *testing.T, s *server, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/translate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleTranslate(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestHandleTranslateOK(t *testing.T) {
	s, relay, srv := newTestServer(t, "test-key")
	defer srv.Close()
	code, out := postTranslate(t, s, `{"texts":["你好","hello"],"target":"en"}`)
	if code != 200 {
		t.Fatalf("status = %d, body = %v", code, out)
	}
	trs, ok := out["translations"].([]any)
	if !ok || len(trs) != 2 {
		t.Fatalf("translations = %v", out["translations"])
	}
	if trs[0] != "TR:你好" {
		t.Errorf("translations[0] = %v, want TR:你好", trs[0])
	}
	if trs[1] != "hello" {
		t.Errorf("translations[1] = %v, want hello (already English, no API call)", trs[1])
	}
	if relay.count() != 1 {
		t.Errorf("relay calls = %d, want 1 batch call", relay.count())
	}
}

func TestHandleTranslateValidation(t *testing.T) {
	s, _, srv := newTestServer(t, "test-key")
	defer srv.Close()
	// Bad target.
	if code, _ := postTranslate(t, s, `{"texts":["hi"],"target":"fr"}`); code != 400 {
		t.Errorf("bad target: status = %d, want 400", code)
	}
	// Too many texts.
	var sb strings.Builder
	sb.WriteString(`{"texts":[`)
	for i := 0; i < 21; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"x"`)
	}
	sb.WriteString(`],"target":"en"}`)
	if code, _ := postTranslate(t, s, sb.String()); code != 400 {
		t.Errorf("21 texts: status = %d, want 400", code)
	}
	// Empty list is fine.
	if code, out := postTranslate(t, s, `{"texts":[],"target":"en"}`); code != 200 {
		t.Errorf("empty list: status = %d, want 200", code)
	} else if trs, ok := out["translations"].([]any); !ok || len(trs) != 0 {
		t.Errorf("empty list: translations = %v", out["translations"])
	}
}

func TestHandleTranslateMissingKey503(t *testing.T) {
	s, _, srv := newTestServer(t, "")
	defer srv.Close()
	code, out := postTranslate(t, s, `{"texts":["你好"],"target":"en"}`)
	if code != 503 {
		t.Fatalf("status = %d, want 503", code)
	}
	if out["error"] != "TRANSLATE_UNAVAILABLE" {
		t.Errorf("error = %v, want TRANSLATE_UNAVAILABLE", out["error"])
	}
}

func TestHandleTranslateMethodNotAllowed(t *testing.T) {
	s, _, srv := newTestServer(t, "test-key")
	defer srv.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/translate", nil)
	rec := httptest.NewRecorder()
	s.handleTranslate(rec, req)
	if rec.Code != 405 {
		t.Errorf("GET status = %d, want 405", rec.Code)
	}
}
