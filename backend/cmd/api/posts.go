package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"xdigest/internal/model"
	"xdigest/internal/store"
)

// ---- posts ----

func (s *server) handlePosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	q := r.URL.Query()
	_, authorPresent := q["author"]
	f := store.PostFilter{AuthorPresent: authorPresent}
	if v := q.Get("author"); v != "" {
		for _, a := range strings.Split(v, ",") {
			if a = strings.TrimSpace(a); a != "" {
				f.Authors = append(f.Authors, a)
			}
		}
	}
	f.Tag = q.Get("tag")
	f.Tags = splitList(q.Get("tags"))
	f.Category = q.Get("category")
	f.Categories = splitList(q.Get("categories"))
	f.UserTag = q.Get("user_tag")
	f.UserTags = splitList(q.Get("user_tags"))
	f.Kind = q.Get("kind")
	f.Query = q.Get("q")
	f.DateFrom = q.Get("date_from")
	f.DateTo = q.Get("date_to")
	f.Sort = q.Get("sort")
	f.Page = atoiDefault(q.Get("page"), 1)
	if f.Page < 1 {
		f.Page = 1
	}
	f.PageSize = atoiDefault(q.Get("page_size"), 200)
	if f.PageSize < 1 {
		f.PageSize = 200
	}
	if f.PageSize > 1000 {
		f.PageSize = 1000
	}
	posts, total, updatedAt, err := s.st.ListPosts(f)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	// snippet: first keyword hit in the first post (the page shows it as a
	// hint above the list), so the page does not need every post to find it.
	snippet := ""
	if len(posts) > 0 && f.Query != "" {
		snippet = firstHitSnippet(posts[0], f.Query)
	}
	writeJSON(w, 200, map[string]any{
		"updated_at": updatedAt, "total": total,
		"page": f.Page, "page_size": f.PageSize, "posts": posts,
		"has_more": f.Page*f.PageSize < total, "snippet": snippet,
	})
}

// splitList: "a,b, c" → [a b c]; "" → nil.
func splitList(v string) []string {
	var out []string
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// firstHitSnippet: ~30 chars before and ~50 after the first case-insensitive
// hit across title / text / tags / author / industry / tech (plain text; the
// page escapes and highlights it). Mirrors useFormat.firstHitSnippet.
func firstHitSnippet(p model.Post, q string) string {
	q = strings.TrimLeft(strings.TrimSpace(q), "#")
	for _, pre := range []string{"标签:", "标签：", "tag:"} {
		if strings.HasPrefix(q, pre) {
			return ""
		}
	}
	if q == "" {
		return ""
	}
	needle := []rune(strings.ToLower(q))
	for _, f := range []string{p.Title, p.Text, strings.Join(p.Tags, " "), p.AuthorName, p.Author, p.Industry, p.Tech} {
		r := []rune(f)
		lower := []rune(strings.ToLower(f))
		if len(lower) != len(r) {
			lower = r // odd case mappings: fall back to exact match
		}
		idx := runeIndex(lower, needle)
		if idx < 0 {
			continue
		}
		start, end := max(0, idx-30), min(len(r), idx+len(needle)+50)
		out := string(r[start:end])
		if start > 0 {
			out = "…" + out
		}
		if end < len(r) {
			out += "…"
		}
		return out
	}
	return ""
}

func runeIndex(h, n []rune) int {
	for i := 0; i+len(n) <= len(h); i++ {
		match := true
		for j := range n {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func (s *server) handlePost(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/posts/summary/batch" {
		s.handlePostSummaryBatch(w, r) // see post_summary.go
		return
	}
	if id, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/posts/"), "/summary/stream"); ok && id != "" && !strings.Contains(id, "/") {
		s.handlePostSummaryStream(w, r, id) // see post_summary.go
		return
	}
	if id, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/posts/"), "/summary"); ok && id != "" && !strings.Contains(id, "/") {
		s.handlePostSummary(w, r, id) // see post_summary.go
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "post not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPatch:
		s.patchPost(w, r, id)
		return
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH")
		return
	}
	p, err := s.st.GetPost(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "NOT_FOUND", "post not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, p)
}

// PATCH /api/v1/posts/{id} {"replica_enabled": true|false|null}
// sets the manual 复刻 switch (null = back to automatic).
func (s *server) patchPost(w http.ResponseWriter, r *http.Request, id string) {
	var body map[string]json.RawMessage
	if !decodeBody(w, r, &body) {
		return
	}
	raw, ok := body["replica_enabled"]
	if !ok {
		writeErr(w, 422, "VALIDATION", "replica_enabled is required (true, false or null)")
		return
	}
	var v *bool
	if err := json.Unmarshal(raw, &v); err != nil {
		writeErr(w, 422, "VALIDATION", "replica_enabled must be true, false or null")
		return
	}
	if err := s.st.SetPostReplicaEnabled(id, v); errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "NOT_FOUND", "post not found")
		return
	} else if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	p, err := s.st.GetPost(id)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, p)
}

func atoiDefault(v string, def int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
