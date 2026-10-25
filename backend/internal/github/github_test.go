package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseFullName(t *testing.T) {
	valid := []struct{ in, owner, repo string }{
		{"haggaimurr-dot/x-digest", "haggaimurr-dot", "x-digest"},
		{"Owner/Repo.Name_1", "Owner", "Repo.Name_1"},
		{"a/b", "a", "b"},
	}
	for _, v := range valid {
		o, r, err := ParseFullName(v.in)
		if err != nil {
			t.Errorf("ParseFullName(%q): unexpected error: %v", v.in, err)
		}
		if o != v.owner || r != v.repo {
			t.Errorf("ParseFullName(%q) = %q/%q, want %q/%q", v.in, o, r, v.owner, v.repo)
		}
	}
	invalid := []string{
		"", "onlyowner", "a/b/c", "/repo", "owner/", "owner/repo ",
		"own er/repo", "owner/repo!", "owner/repo?",
	}
	for _, in := range invalid {
		if _, _, err := ParseFullName(in); err == nil {
			t.Errorf("ParseFullName(%q): expected error, got nil", in)
		}
	}
}

func TestParseSearchTotal(t *testing.T) {
	n, err := parseSearchTotal([]byte(`{"total_count": 7, "incomplete_results": false, "items": []}`))
	if err != nil || n != 7 {
		t.Fatalf("parseSearchTotal = %d, %v; want 7, nil", n, err)
	}
	if _, err := parseSearchTotal([]byte(`not json`)); err == nil {
		t.Error("parseSearchTotal(invalid JSON): expected error")
	}
}

// fixtureServer serves canned GitHub responses for the 5 sync requests.
func fixtureServer(t *testing.T, searchIssuesStatus, searchPRsStatus int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		checkHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"full_name":"o/r","owner":{"login":"o"},"name":"r",
			"description":"desc","language":"Go","stargazers_count":42,"forks_count":3,
			"html_url":"https://github.com/o/r","homepage":"https://example.com",
			"pushed_at":"2026-10-24T10:00:00Z","updated_at":"2026-10-24T09:00:00Z"}`))
	})
	mux.HandleFunc("/repos/o/r/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "10" {
			t.Errorf("commits per_page = %q, want 10", r.URL.Query().Get("per_page"))
		}
		_, _ = w.Write([]byte(`[{"sha":"abc123","html_url":"https://github.com/o/r/commit/abc123",
			"commit":{"message":"first","author":{"name":"Ann","date":"2026-10-24T08:00:00Z"}},
			"author":{"login":"ann"}}]`))
	})
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "5" {
			t.Errorf("releases per_page = %q, want 5", r.URL.Query().Get("per_page"))
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0","name":"V1","published_at":"2026-10-20T00:00:00Z",
			"html_url":"https://github.com/o/r/releases/v1.0","draft":false,"prerelease":false}]`))
	})
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.Contains(q, "type:issue") {
			w.WriteHeader(searchIssuesStatus)
			_, _ = w.Write([]byte(`{"total_count": 7, "items": []}`))
			return
		}
		if strings.Contains(q, "type:pr") {
			w.WriteHeader(searchPRsStatus)
			_, _ = w.Write([]byte(`{"total_count": 2, "items": []}`))
			return
		}
		t.Errorf("unexpected search q=%q", q)
		w.WriteHeader(500)
	})
	return httptest.NewServer(mux)
}

func checkHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("User-Agent"); got != "x-digest" {
		t.Errorf("User-Agent = %q, want x-digest", got)
	}
	if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want application/vnd.github+json", got)
	}
}

func TestSyncSuccess(t *testing.T) {
	srv := fixtureServer(t, 200, 200)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	res, err := c.Sync(context.Background(), "o", "r")
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if res.Status != "synced" {
		t.Fatalf("Status = %q, want synced (error: %s)", res.Status, res.Error)
	}
	if res.Repo.Stars != 42 || res.Repo.Forks != 3 || res.Repo.Language != "Go" {
		t.Errorf("repo meta = %+v, want stars 42 forks 3 lang Go", res.Repo)
	}
	if res.Repo.OpenIssues == nil || *res.Repo.OpenIssues != 7 {
		t.Errorf("OpenIssues = %v, want 7", res.Repo.OpenIssues)
	}
	if res.Repo.OpenPRs == nil || *res.Repo.OpenPRs != 2 {
		t.Errorf("OpenPRs = %v, want 2", res.Repo.OpenPRs)
	}
	// GitHub UTC -> Shanghai wall time (+8h).
	if res.Repo.PushedAt == nil || *res.Repo.PushedAt != "2026-10-24 18:00:00" {
		t.Errorf("PushedAt = %v, want 2026-10-24 18:00:00", res.Repo.PushedAt)
	}
	if len(res.Commits) != 1 || res.Commits[0].SHA != "abc123" || res.Commits[0].AuthorLogin != "ann" {
		t.Errorf("commits = %+v, want 1 commit abc123/ann", res.Commits)
	}
	if len(res.Releases) != 1 || res.Releases[0].TagName != "v1.0" {
		t.Errorf("releases = %+v, want 1 release v1.0", res.Releases)
	}
}

// A failed search must not invent counts: the affected count stays nil
// and the result is partial, while the other count survives.
func TestSyncSearchFailureIsPartialNotInvented(t *testing.T) {
	srv := fixtureServer(t, 500, 200)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	res, err := c.Sync(context.Background(), "o", "r")
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if res.Status != "partial" {
		t.Fatalf("Status = %q, want partial", res.Status)
	}
	if res.Repo.OpenIssues != nil {
		t.Errorf("OpenIssues = %v, want nil (search failed)", res.Repo.OpenIssues)
	}
	if res.Repo.OpenPRs == nil || *res.Repo.OpenPRs != 2 {
		t.Errorf("OpenPRs = %v, want 2 (that search succeeded)", res.Repo.OpenPRs)
	}
	if res.Error == "" {
		t.Error("Error should explain the partial sync")
	}
	// Metadata must still be present on a partial sync.
	if res.Repo.Stars != 42 {
		t.Errorf("Stars = %d, want 42 on partial sync", res.Repo.Stars)
	}
}

// Rate limiting: no repo data is returned to overwrite the old row with,
// and the result is flagged so the caller preserves existing data.
func TestSyncRateLimitedPreservesOldData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1790000000")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	res, err := c.Sync(context.Background(), "o", "r")
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if !res.RateLimited || res.Status != "rate_limited" {
		t.Fatalf("RateLimited=%v Status=%q, want true/rate_limited", res.RateLimited, res.Status)
	}
	if res.Repo.FullName != "" || res.Repo.Stars != 0 || len(res.Commits) != 0 || len(res.Releases) != 0 {
		t.Errorf("rate-limited result must carry no repo data to overwrite old rows, got %+v", res.Repo)
	}
	if res.Error == "" {
		t.Error("Error should describe the rate limit")
	}
}

// Unauthenticated 404 must not claim the repo does not exist: it may be private.
func TestSyncNotFoundUnauthenticated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL} // no token
	res, err := c.Sync(context.Background(), "o", "r")
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("Status = %q, want failed", res.Status)
	}
	if !strings.Contains(res.Error, "私有") || !strings.Contains(res.Error, "token") {
		t.Errorf("Error = %q, must warn it may be nonexistent OR private needing a token", res.Error)
	}
}
