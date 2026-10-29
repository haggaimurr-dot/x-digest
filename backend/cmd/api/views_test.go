package main

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xdigest/internal/translate"
)

func TestValidView(t *testing.T) {
	cases := []struct {
		name, filter string
		ok           bool
	}{
		{"AI 工具", `{"tags":["AI"],"keyword":""}`, true},
		{"", `{}`, false},
		{"   ", `{}`, false},
		{strings.Repeat("长", 65), `{}`, false},
		{"x", `[]`, false},
		{"x", `"str"`, false},
		{"x", `{bad`, false},
		{"x", ``, false},
		{"x", `{"k":"` + strings.Repeat("a", 9000) + `"}`, false},
	}
	for _, c := range cases {
		if got := validView(c.name, json.RawMessage(c.filter)) == ""; got != c.ok {
			t.Errorf("validView(%q, %.20q) ok=%v, want %v", c.name, c.filter, got, c.ok)
		}
	}
}

// Bad bodies are rejected before the store is touched.
func TestViewsHandlerValidation(t *testing.T) {
	s := &server{}
	rec := httptest.NewRecorder()
	s.handleViews(rec, httptest.NewRequest("POST", "/api/v1/views", strings.NewReader(`{"name":"","filter":{}}`)))
	if rec.Code != 422 {
		t.Fatalf("empty name: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleView(rec, httptest.NewRequest("GET", "/api/v1/views/v1", nil))
	if rec.Code != 405 {
		t.Fatalf("GET one view: %d", rec.Code)
	}
}

func TestCategoryBatchValidation(t *testing.T) {
	s := &server{}
	for body, want := range map[string]int{
		`{"post_ids":[],"category_id":"c","action":"add"}`:     422,
		`{"post_ids":["p1"],"category_id":"","action":"add"}`:  422,
		`{"post_ids":["p1"],"category_id":"c","action":"zap"}`: 422,
		`nope`: 400,
	} {
		rec := httptest.NewRecorder()
		s.handleCategory(rec, httptest.NewRequest("POST", "/api/v1/categories/assignments/batch", strings.NewReader(body)))
		if rec.Code != want {
			t.Errorf("%s → %d, want %d", body, rec.Code, want)
		}
	}
}

func TestPostSummaryBatchValidation(t *testing.T) {
	s := &server{sum: &summaryJobs{llm: &translate.Client{}}}
	ids31 := make([]string, 31)
	for i := range ids31 {
		ids31[i] = `"p` + strings.Repeat("x", i+1) + `"`
	}
	for body, want := range map[string]int{
		`{"ids":[]}`:                422,
		`{"ids":["a"],"lang":"fr"}`: 422,
		`{"ids":[` + strings.Join(ids31, ",") + `]}`: 422,
		`{"ids":["a"],"lang":"zh-CN"}`:               503, // AI not configured
	} {
		rec := httptest.NewRecorder()
		s.handlePost(rec, httptest.NewRequest("POST", "/api/v1/posts/summary/batch", strings.NewReader(body)))
		if rec.Code != want {
			t.Errorf("%.40s -> %d, want %d", body, rec.Code, want)
		}
	}
}

func TestParseHandles(t *testing.T) {
	valid, invalid := parseHandles(strings.FieldsFunc("@alice, https://x.com/Bob\nALICE twitter.com/carol?s=1 bad-name！ way_too_long_handle_x 、dave", isHandleSep))
	if strings.Join(valid, ",") != "alice,Bob,carol,dave" {
		t.Fatalf("valid = %v", valid)
	}
	if len(invalid) != 2 {
		t.Fatalf("invalid = %v", invalid)
	}
}

func TestRepoSyncInterval(t *testing.T) {
	for v, want := range map[string]time.Duration{"": 24 * time.Hour, "12h": 12 * time.Hour, "0": 0, "30m": 0, "bad": 0} {
		t.Setenv("REPO_SYNC_INTERVAL", v)
		if got := repoSyncInterval(); got != want {
			t.Errorf("REPO_SYNC_INTERVAL=%q → %v, want %v", v, got, want)
		}
	}
}

type fakeVideoState struct {
	platform, id   string
	watched, later *bool
}

func (f *fakeVideoState) SetVideoState(platform, id string, watched, later *bool) error {
	if id == "missing" {
		return sql.ErrNoRows
	}
	f.platform, f.id, f.watched, f.later = platform, id, watched, later
	return nil
}

func TestPatchVideoState(t *testing.T) {
	f := &fakeVideoState{}
	do := func(method, body, id string) int {
		rec := httptest.NewRecorder()
		patchVideoState(rec, httptest.NewRequest(method, "/x", strings.NewReader(body)), f, "bili", id)
		return rec.Code
	}
	if c := do("PATCH", `{"watched":true}`, "BV1"); c != 200 || f.platform != "bili" || f.id != "BV1" || f.watched == nil || !*f.watched || f.later != nil {
		t.Fatalf("watched: %d %+v", c, f)
	}
	if c := do("PATCH", `{"watch_later":false}`, "BV1"); c != 200 || f.later == nil || *f.later || f.watched != nil {
		t.Fatalf("watch_later: %d %+v", c, f)
	}
	if c := do("PATCH", `{}`, "BV1"); c != 422 {
		t.Fatalf("empty: %d", c)
	}
	if c := do("PATCH", `{"watched":true}`, "missing"); c != 404 {
		t.Fatalf("missing: %d", c)
	}
	if c := do("GET", ``, "BV1"); c != 405 {
		t.Fatalf("GET: %d", c)
	}
}
