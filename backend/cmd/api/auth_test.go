package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireAuth(t *testing.T) {
	hits := 0
	h := requireAuth("s3cret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(200)
	}))
	cases := []struct {
		method, path, auth string
		want               int
	}{
		{"POST", "/api/v1/replicas", "", 401},
		{"POST", "/api/v1/replicas", "Bearer wrong", 401},
		{"POST", "/api/v1/replicas", "Bearer s3cret", 200},
		{"POST", "/api/v1/replicas", "Basic dXNlcjpwYXNz", 401},
		{"POST", "/api/v1/replicas", "X:s3cret", 200},
		{"POST", "/api/v1/posts/import", "", 401},
		{"POST", "/api/v1/x/refresh", "", 401},
		{"POST", "/api/v1/posts/1/summary", "", 401},
		{"POST", "/api/v1/translate", "", 401},
		{"POST", "/api/v1/replicas/r1/attachments", "", 401},
		{"DELETE", "/api/v1/attachments/a1", "", 401},
		{"PATCH", "/api/v1/categories/c1", "", 401},
		{"PUT", "/api/v1/user-tags", "", 401},
		{"GET", "/api/v1/posts", "", 200},
		{"GET", "/api/v1/attachments/a1", "", 200},
		{"HEAD", "/api/v1/posts", "", 200},
		{"OPTIONS", "/api/v1/posts", "", 200},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		if tok, ok := strings.CutPrefix(c.auth, "X:"); ok {
			req.Header.Set("X-API-Token", tok)
		} else if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s %s auth=%q: got %d want %d", c.method, c.path, c.auth, rec.Code, c.want)
		}
		if rec.Code == 401 && rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s %s: 401 without WWW-Authenticate", c.method, c.path)
		}
	}
	// 401s never reach the handler (no AI call, no DB write).
	if hits != 6 {
		t.Errorf("handler hits = %d, want 6", hits)
	}
}

func TestRequireAuthDisabled(t *testing.T) {
	h := requireAuth("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/replicas", nil))
	if rec.Code != 204 {
		t.Fatalf("no token configured: got %d want 204", rec.Code)
	}
}
