// Command api serves the x-digest Go backend (see ../SPEC.md contract).
//
// Environment:
//
//	XDIGEST_ADDR           listen address, default 127.0.0.1:18090
//	XDIGEST_DSN            MySQL DSN, default "xdigest:CHANGE_ME@tcp(127.0.0.1:3306)/xdigest"
//	XDIGEST_ATTACHMENT_DIR disk directory for attachment files,
//	                       default /var/lib/xdigest/attachments
//	GITHUB_TOKEN            optional GitHub personal access token for repo
//	                       sync (higher rate limits, private repos).
//	                       Unset = unauthenticated mode.
//	TRANSLATE_API_BASE      translation relay base URL,
//	                       default https://api.apikey.fan
//	TRANSLATE_API_KEY       translation relay API key. Unset = POST
//	                       /api/v1/translate answers 503; the UI language
//	                       toggle still works (UI strings are local).
//	BILIBILI_SESSDATA       optional logged-in Bilibili cookie (AI subtitles
//	                       for summaries, fewer risk-control blocks).
//	BILI_HTTP_PROXY         optional proxy for Bilibili requests.
//	TRANSLATE_MODEL         translation chat model, default deepseek-flash.
//	                       The same relay/key/model also write the
//	                       YouTube AI summaries (see yt_summary.go).
//	GROK_API_KEY            relay key of the Grok group: X account refresh
//	                       (x_import.go; also GROK_MODEL, X_HTTP_PROXY).
//
// No authentication is implemented (see README.md "Security note").
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/bilibili"
	"xdigest/internal/github"
	"xdigest/internal/model"
	"xdigest/internal/store"
	"xdigest/internal/translate"
	"xdigest/internal/youtube"
)

const maxAttachmentSize = 5 << 20 // 5 MiB per file

var allowedExts = map[string]bool{
	"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
	"ppt": true, "pptx": true, "txt": true, "md": true, "csv": true,
	"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true,
	"bmp": true, "avif": true, "svg": true, "ico": true,
}

type server struct {
	st        *store.Store
	attachDir string
	gh        *github.Client
	tr        *translate.Service
	yt        *youtube.Client
	sum       *summaryJobs
	comments  *commentQueue
}

func main() {
	addr := envOr("XDIGEST_ADDR", "127.0.0.1:18090")
	dsn := envOr("XDIGEST_DSN", "xdigest:CHANGE_ME@tcp(127.0.0.1:3306)/xdigest")
	attachDir := envOr("XDIGEST_ATTACHMENT_DIR", "/var/lib/xdigest/attachments")
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		log.Fatalf("attachment dir: %v", err)
	}
	st, err := store.New(dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	s := &server{st: st, attachDir: attachDir, gh: github.NewFromEnv(), tr: translate.NewService(translate.NewFromEnv(), st), yt: youtube.New()}
	s.sum = newSummaryJobs(s.yt)
	s.comments = newCommentQueue()
	go s.runCommentWorker()
	mux := http.NewServeMux()
	newXImporter(st).register(mux)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/v1/healthz", s.handleHealth)
	mux.HandleFunc("/api/v1/posts", s.handlePosts)
	mux.HandleFunc("/api/v1/posts/", s.handlePost)
	mux.HandleFunc("/api/v1/watchlist", s.handleWatchlist)
	mux.HandleFunc("/api/v1/watchlist/groups", s.handleGroups)
	mux.HandleFunc("/api/v1/watchlist/groups/", s.handleGroup)
	mux.HandleFunc("/api/v1/watchlist/accounts", s.handleAccounts)
	mux.HandleFunc("/api/v1/watchlist/accounts/", s.handleAccount)
	mux.HandleFunc("/api/v1/tags", s.handleTags)
	mux.HandleFunc("/api/v1/user-tags", s.handleUserTags)
	mux.HandleFunc("/api/v1/categories", s.handleCategories)
	mux.HandleFunc("/api/v1/categories/", s.handleCategory)
	mux.HandleFunc("/api/v1/categories/assignments", s.handleCategoryAssignments)
	mux.HandleFunc("/api/v1/replicas", s.handleReplicas)
	mux.HandleFunc("/api/v1/replicas/", s.handleReplicaSub)
	mux.HandleFunc("/api/v1/repos", s.handleRepos)
	mux.HandleFunc("/api/v1/repos/", s.handleRepoSub)
	mux.HandleFunc("/api/v1/repos/groups", s.handleRepoGroups)
	mux.HandleFunc("/api/v1/repos/groups/", s.handleRepoGroup)
	mux.HandleFunc("/api/v1/repos/sync-all", s.handleRepoSyncAll)
	mux.HandleFunc("/api/v1/yt/channels", s.handleYTChannels)
	mux.HandleFunc("/api/v1/yt/channels/", s.handleYTChannelSub)
	mux.HandleFunc("/api/v1/yt/videos", s.handleYTVideos)
	mux.HandleFunc("/api/v1/yt/videos/", s.handleYTVideoSub)
	mux.HandleFunc("/api/v1/yt/comments", s.handleYTComments)
	// Bilibili uploaders: same routes/JSON as /api/v1/yt (see hub.go).
	(&videoHub{prefix: "bili", idPrefix: "bic_", kinds: map[string]bool{"video": true, "playlist": true},
		st: st.Hub("bili"), pf: bilibili.New(), sum: s.sum, comments: newCommentQueue()}).register(mux)
	mux.HandleFunc("/api/v1/yt/sync", s.handleYTSyncAll)
	mux.HandleFunc("/api/v1/yt/groups", s.handleYTGroups)
	mux.HandleFunc("/api/v1/yt/groups/", s.handleYTGroup)
	mux.HandleFunc("/api/v1/translate", s.handleTranslate)
	mux.HandleFunc("/api/v1/attachments/", s.handleAttachment)
	log.Printf("xdigest-api listening on %s (attachments: %s)", addr, attachDir)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, 400, "BAD_JSON", "request body is not valid JSON: "+err.Error())
		return false
	}
	return true
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	if err := s.st.Ping(); err != nil {
		writeErr(w, 503, "DB_UNAVAILABLE", "database ping failed: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- translation ----
// POST /api/v1/translate {texts: string[], target: "en"|"zh-CN"} ->
// {translations: string[]} in the same order. Untranslated items come back
// as the original text; the endpoint never fails the whole request because
// of one item. Without TRANSLATE_API_KEY it answers 503.

func (s *server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var req struct {
		Texts  []string `json:"texts"`
		Target string   `json:"target"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if !translate.ValidTarget(req.Target) {
		writeErr(w, 400, "INVALID_TARGET", "target must be \"en\" or \"zh-CN\"")
		return
	}
	if len(req.Texts) > 20 {
		writeErr(w, 400, "TOO_MANY_TEXTS", "at most 20 texts per request")
		return
	}
	out, err := s.tr.Translate(r.Context(), req.Texts, req.Target)
	if errors.Is(err, translate.ErrNotConfigured) {
		writeErr(w, 503, "TRANSLATE_UNAVAILABLE", "translation is not configured on this server")
		return
	}
	if err != nil {
		writeErr(w, 500, "TRANSLATE_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"translations": out})
}

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
	f.Category = q.Get("category")
	f.UserTag = q.Get("user_tag")
	f.Query = q.Get("q")
	f.DateFrom = q.Get("date_from")
	f.DateTo = q.Get("date_to")
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
	writeJSON(w, 200, map[string]any{
		"updated_at": updatedAt, "total": total,
		"page": f.Page, "page_size": f.PageSize, "posts": posts,
	})
}

func (s *server) handlePost(w http.ResponseWriter, r *http.Request) {
	if id, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/posts/"), "/summary"); ok && id != "" && !strings.Contains(id, "/") {
		s.handlePostSummary(w, r, id) // see post_summary.go
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "post not found")
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

// ---- watchlist ----

func (s *server) handleWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	groups, accounts, err := s.st.ListWatchlist()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"groups": groups, "accounts": accounts})
}

func (s *server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeErr(w, 422, "VALIDATION", "name is required")
		return
	}
	g, err := s.st.CreateGroup(body.Name)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 201, g)
}

func (s *server) handleGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/watchlist/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := s.st.DeleteGroup(id)
	if code == "NOT_FOUND" {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	if code == "GROUP_FIXED" {
		writeErr(w, 422, "GROUP_FIXED", "fixed groups cannot be deleted")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var a model.Account
	if !decodeBody(w, r, &a) {
		return
	}
	if strings.TrimSpace(a.Username) == "" {
		writeErr(w, 422, "VALIDATION", "username is required")
		return
	}
	if a.Group == "" {
		a.Group = "other"
	}
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if s.st.AccountExists(a.Username) {
		writeErr(w, 409, "CONFLICT", "account already exists")
		return
	}
	if !s.st.GroupExists(a.Group) {
		writeErr(w, 422, "VALIDATION", "group does not exist: "+a.Group)
		return
	}
	if err := s.st.CreateAccount(a); err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 201, a)
}

func (s *server) handleAccount(w http.ResponseWriter, r *http.Request) {
	username, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/v1/watchlist/accounts/"))
	if username == "" || strings.Contains(username, "/") {
		writeErr(w, 404, "NOT_FOUND", "account not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		if !s.st.AccountExists(username) {
			writeErr(w, 404, "NOT_FOUND", "account not found")
			return
		}
		var body struct {
			Name   *string   `json:"name"`
			Avatar *string   `json:"avatar"`
			Group  *string   `json:"group"`
			Tags   *[]string `json:"tags"`
			Bio    *string   `json:"bio"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := s.st.PatchAccount(username, body.Name, body.Avatar, body.Group, body.Bio, body.Tags); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case http.MethodDelete:
		if !s.st.AccountExists(username) {
			writeErr(w, 404, "NOT_FOUND", "account not found")
			return
		}
		if err := s.st.DeleteAccount(username); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only PATCH, DELETE")
	}
}

// ---- tags / user-tags / categories ----

func (s *server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	tags, err := s.st.PostTagsByFreq()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"tags": tags})
}

func (s *server) handleUserTags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		vocab, overrides, err := s.st.UserTags()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"vocabulary": vocab, "overrides": overrides})
	case http.MethodPut:
		// {username, tags} sets one author's override; {vocabulary: [...]}
		// replaces the user-tag vocabulary (the tag list in settings).
		var body struct {
			Username   string    `json:"username"`
			Tags       []string  `json:"tags"`
			Vocabulary *[]string `json:"vocabulary"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.Vocabulary != nil {
			if err := s.st.SetUserTagVocabulary(*body.Vocabulary); err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if body.Username == "" {
			writeErr(w, 422, "VALIDATION", "username or vocabulary is required")
			return
		}
		if !s.st.AccountExists(body.Username) {
			writeErr(w, 404, "NOT_FOUND", "author not found")
			return
		}
		if body.Tags == nil {
			body.Tags = []string{}
		}
		if err := s.st.SetUserTagOverride(body.Username, body.Tags); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PUT")
	}
}

func (s *server) handleCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cats, assignments, err := s.st.ListCategories()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"categories": cats, "assignments": assignments})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			writeErr(w, 422, "VALIDATION", "name is required")
			return
		}
		c, err := s.st.CreateCategory(body.Name)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, c)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleCategory(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/categories/")
	if rest == "assignments" {
		s.handleCategoryAssignments(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := rest
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "category not found")
		return
	}
	if err := s.st.DeleteCategory(id); err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) handleCategoryAssignments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only PUT")
		return
	}
	var body struct {
		PostID      string   `json:"post_id"`
		CategoryIDs []string `json:"category_ids"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.CategoryIDs == nil {
		body.CategoryIDs = []string{}
	}
	if err := s.st.SetPostCategories(body.PostID, body.CategoryIDs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "post not found")
			return
		}
		if strings.HasPrefix(err.Error(), "unknown category") {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- replicas ----

func (s *server) handleReplicas(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.st.ListReplicas(r.URL.Query().Get("sort"))
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"replicas": list})
	case http.MethodPost:
		var body struct {
			PostID string `json:"post_id"`
			Title  string `json:"title"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.PostID == "" && body.Title != "" {
			rep, err := s.st.CreateStandaloneReplica(body.Title)
			if err != nil {
				if err.Error() == "empty replica title" {
					writeErr(w, 422, "VALIDATION", "title is required")
					return
				}
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 201, rep)
			return
		}
		if body.PostID == "" {
			writeErr(w, 422, "VALIDATION", "post_id is required")
			return
		}
		rep, _, err := s.st.CreateReplicaFromPost(body.PostID)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "post not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, rep)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleReplicaSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/replicas/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		writeErr(w, 404, "NOT_FOUND", "replica not found")
		return
	}
	if len(parts) == 2 && parts[1] == "attachments" {
		// GET list, or POST upload (?step_idx=N, multipart field "file")
		if r.Method == http.MethodPost {
			s.handleUpload(w, r, id)
			return
		}
		if r.Method != http.MethodGet {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
			return
		}
		if !s.st.ReplicaExists(id) {
			writeErr(w, 404, "NOT_FOUND", "replica not found")
			return
		}
		atts, err := s.st.ListReplicaAttachments(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"attachments": atts})
		return
	}
	if len(parts) == 2 && parts[1] == "steps" {
		// POST /api/v1/replicas/{id}/steps — add a user-created flow node.
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		var body struct {
			Text      string `json:"text"`
			ParentIdx *int   `json:"parent_idx"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		st, err := s.st.AddReplicaStep(id, body.Text, body.ParentIdx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeErr(w, 404, "NOT_FOUND", "replica not found")
				return
			}
			if err.Error() == "empty step text" {
				writeErr(w, 422, "VALIDATION", "step text is required")
				return
			}
			if strings.HasPrefix(err.Error(), "unknown parent step") || strings.HasPrefix(err.Error(), "sub-nodes support") {
				writeErr(w, 422, "VALIDATION", err.Error())
				return
			}
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, st)
		return
	}
	if len(parts) == 3 && parts[1] == "steps" {
		// DELETE /api/v1/replicas/{id}/steps/{idx} — delete a custom node only.
		if r.Method != http.MethodDelete {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
			return
		}
		idx, err := strconv.Atoi(parts[2])
		if err != nil || idx < 0 {
			writeErr(w, 422, "VALIDATION", "invalid step idx")
			return
		}
		paths, err := s.st.DeleteReplicaStep(id, idx)
		if err != nil {
			if strings.HasPrefix(err.Error(), "unknown step") {
				writeErr(w, 422, "VALIDATION", err.Error())
				return
			}
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		for _, p := range paths {
			_ = os.Remove(p) // best effort; DB rows are already gone
		}
		writeJSON(w, 200, map[string]any{"deleted": true})
		return
	}
	if len(parts) != 1 {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rep, err := s.st.GetReplica(id, "")
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "replica not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, rep)
	case http.MethodPatch:
		var body struct {
			Steps []struct {
				Idx    int     `json:"idx"`
				Status *string `json:"status"`
				Note   *string `json:"note"`
				Text   *string `json:"text"`
			} `json:"steps"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		var patches []store.StepPatch
		for _, st := range body.Steps {
			patches = append(patches, store.StepPatch{Idx: st.Idx, Status: st.Status, Note: st.Note, Text: st.Text})
		}
		if err := s.st.PatchReplicaSteps(id, patches); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeErr(w, 404, "NOT_FOUND", "replica not found")
				return
			}
			if strings.HasPrefix(err.Error(), "invalid status") || strings.HasPrefix(err.Error(), "unknown step") {
				writeErr(w, 422, "VALIDATION", err.Error())
				return
			}
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		rep, err := s.st.GetReplica(id, "")
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, rep)
	case http.MethodDelete:
		if !s.st.ReplicaExists(id) {
			writeErr(w, 404, "NOT_FOUND", "replica not found")
			return
		}
		paths, err := s.st.DeleteReplica(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		for _, p := range paths {
			_ = os.Remove(p) // best effort; DB row is already gone
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH, DELETE")
	}
}

// ---- attachments ----

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request, replicaID string) {
	if !s.st.ReplicaExists(replicaID) {
		writeErr(w, 404, "NOT_FOUND", "replica not found")
		return
	}
	stepIdx, err := strconv.Atoi(r.URL.Query().Get("step_idx"))
	if err != nil || stepIdx < 0 || !s.st.ReplicaStepExists(replicaID, stepIdx) {
		writeErr(w, 422, "VALIDATION", "step_idx is missing or does not exist in this replica")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, 400, "BAD_MULTIPART", "cannot parse multipart form: "+err.Error())
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeErr(w, 422, "VALIDATION", "no file field in multipart form")
		return
	}
	// Validate ALL files first so one bad file rejects the whole request
	// before anything is written to disk.
	for _, fh := range files {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fh.Filename), "."))
		if !allowedExts[ext] {
			writeErr(w, 422, "INVALID_FILE", fmt.Sprintf("file %q: extension .%s is not allowed", fh.Filename, ext))
			return
		}
		if fh.Size > maxAttachmentSize {
			writeErr(w, 422, "FILE_TOO_LARGE", fmt.Sprintf("file %q: size %d exceeds 5MB limit", fh.Filename, fh.Size))
			return
		}
	}
	var saved []model.Attachment
	var savedPaths []string
	for _, fh := range files {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fh.Filename), "."))
		src, err := fh.Open()
		if err != nil {
			writeErr(w, 500, "IO_ERROR", err.Error())
			return
		}
		id := store.NewID("att_")
		dstPath := filepath.Join(s.attachDir, id)
		dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			src.Close()
			writeErr(w, 500, "IO_ERROR", err.Error())
			return
		}
		written, err := io.Copy(dst, io.LimitReader(src, maxAttachmentSize+1))
		src.Close()
		dst.Close()
		if err != nil || written > maxAttachmentSize {
			_ = os.Remove(dstPath)
			for _, p := range savedPaths {
				_ = os.Remove(p)
			}
			writeErr(w, 422, "FILE_TOO_LARGE", fmt.Sprintf("file %q: size exceeds 5MB limit", fh.Filename))
			return
		}
		ct := fh.Header.Get("Content-Type")
		if ct == "" || ct == "application/octet-stream" {
			if sniffed := sniffContentType(dstPath); sniffed != "" {
				ct = sniffed
			}
		}
		if ct == "" {
			ct = mime.TypeByExtension("." + ext)
		}
		att := model.Attachment{
			ID: id, ReplicaID: replicaID, StepIdx: stepIdx,
			Name: filepath.Base(fh.Filename), Size: written,
			ContentType: ct, Ext: ext, CreatedAt: store.Now(),
			DownloadURL: "/api/v1/attachments/" + id + "/download",
		}
		if err := s.st.CreateAttachment(att, dstPath); err != nil {
			_ = os.Remove(dstPath)
			for _, p := range savedPaths {
				_ = os.Remove(p)
			}
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		saved = append(saved, att)
		savedPaths = append(savedPaths, dstPath)
	}
	writeJSON(w, 201, map[string]any{"attachments": saved})
}

func sniffContentType(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return http.DetectContentType(buf[:n])
}

func (s *server) handleAttachment(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/attachments/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		writeErr(w, 404, "NOT_FOUND", "attachment not found")
		return
	}
	if len(parts) == 2 && parts[1] == "download" {
		if r.Method != http.MethodGet {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
			return
		}
		att, path, err := s.st.GetAttachment(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "attachment not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		disposition := "attachment"
		if r.URL.Query().Get("inline") == "1" {
			disposition = "inline"
		}
		safeName := strings.NewReplacer(`"`, "", "\n", "", "\r", "").Replace(att.Name)
		w.Header().Set("Content-Type", att.ContentType)
		w.Header().Set("Content-Disposition", disposition+`; filename="`+safeName+`"; filename*=UTF-8''`+url.PathEscape(att.Name))
		w.Header().Set("Content-Length", strconv.FormatInt(att.Size, 10))
		http.ServeFile(w, r, path)
		return
	}
	if len(parts) != 1 {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		att, _, err := s.st.GetAttachment(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "attachment not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, att)
	case http.MethodDelete:
		path, err := s.st.DeleteAttachment(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "attachment not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		_ = os.Remove(path)
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, DELETE")
	}
}

// ---- GitHub repos ----

// syncRepo runs one GitHub sync for an existing repo row and returns the
// fresh row. A failed or rate-limited sync never rolls back the row and
// never wipes old synced data: only sync_status/sync_error (and
// last_synced_at) change. A partial sync still applies the metadata and
// whatever sub-fetches succeeded; nil commit/release slices are skipped
// so previously synced rows are kept.
func (s *server) syncRepo(fullName string) (*model.Repo, error) {
	owner, repo, err := github.ParseFullName(fullName)
	if err != nil {
		return nil, err
	}
	res, err := s.gh.Sync(context.Background(), owner, repo)
	if err != nil {
		// Defensive: Sync currently folds transport errors into Status.
		_ = s.st.SetRepoSyncStatus(fullName, "failed", err.Error())
		return s.st.GetRepo(fullName)
	}
	switch {
	case res.RateLimited:
		_ = s.st.SetRepoSyncStatus(fullName, "rate_limited", res.Error)
	case res.Status == "failed":
		_ = s.st.SetRepoSyncStatus(fullName, "failed", res.Error)
	default: // "synced" or "partial"
		if err := s.st.UpdateRepoMeta(fullName, res.Repo, res.Status, res.Error); err != nil {
			return nil, err
		}
		if res.Commits != nil {
			if err := s.st.UpsertRepoCommits(fullName, res.Commits); err != nil {
				return nil, err
			}
		}
		if res.Releases != nil {
			if err := s.st.UpsertRepoReleases(fullName, res.Releases); err != nil {
				return nil, err
			}
		}
	}
	return s.st.GetRepo(fullName)
}

func (s *server) handleRepos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		sort := q.Get("sort")
		if sort != "stars" && sort != "pushed_at" {
			sort = "pushed_at"
		}
		repos, err := s.st.ListRepos(store.RepoFilter{Group: q.Get("group"), Tag: q.Get("tag"), Sort: sort})
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"repos": repos})
	case http.MethodPost:
		var body struct {
			FullName string   `json:"full_name"`
			GroupID  string   `json:"group_id"`
			Tags     []string `json:"tags"`
			Note     string   `json:"note"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		owner, name, err := github.ParseFullName(strings.TrimSpace(body.FullName))
		if err != nil {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		if body.GroupID == "" {
			body.GroupID = "other"
		}
		if body.Tags == nil {
			body.Tags = []string{}
		}
		if s.st.RepoExists(owner + "/" + name) {
			writeErr(w, 409, "CONFLICT", "repo already exists")
			return
		}
		if !s.st.RepoGroupExists(body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
			return
		}
		created := model.Repo{
			FullName: owner + "/" + name, Owner: owner, Name: name,
			GroupID: body.GroupID, Tags: body.Tags, Note: body.Note,
			SyncStatus: "never",
		}
		if err := s.st.CreateRepo(created); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		// Sync immediately; a sync failure does not roll back the create —
		// it is recorded as sync_status rate_limited/failed on the row.
		rep, err := s.syncRepo(created.FullName)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, rep)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleRepoSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		writeErr(w, 404, "NOT_FOUND", "repo not found")
		return
	}
	owner, repo, err := github.ParseFullName(parts[0] + "/" + parts[1])
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "repo not found")
		return
	}
	fullName := owner + "/" + repo
	if len(parts) == 3 {
		switch parts[2] {
		case "commits":
			if r.Method != http.MethodGet {
				writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
				return
			}
			if !s.st.RepoExists(fullName) {
				writeErr(w, 404, "NOT_FOUND", "repo not found")
				return
			}
			commits, err := s.st.ListRepoCommits(fullName, 10)
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"commits": commits})
			return
		case "releases":
			if r.Method != http.MethodGet {
				writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
				return
			}
			if !s.st.RepoExists(fullName) {
				writeErr(w, 404, "NOT_FOUND", "repo not found")
				return
			}
			releases, err := s.st.ListRepoReleases(fullName, 100)
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"releases": releases})
			return
		case "sync":
			if r.Method != http.MethodPost {
				writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
				return
			}
			if !s.st.RepoExists(fullName) {
				writeErr(w, 404, "NOT_FOUND", "repo not found")
				return
			}
			rep, err := s.syncRepo(fullName)
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, rep)
			return
		default:
			writeErr(w, 404, "NOT_FOUND", "not found")
			return
		}
	}
	if len(parts) != 2 {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rep, err := s.st.GetRepo(fullName)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "repo not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, rep)
	case http.MethodPatch:
		if !s.st.RepoExists(fullName) {
			writeErr(w, 404, "NOT_FOUND", "repo not found")
			return
		}
		var body struct {
			GroupID *string   `json:"group_id"`
			Tags    *[]string `json:"tags"`
			Note    *string   `json:"note"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.GroupID != nil && *body.GroupID != "" && !s.st.RepoGroupExists(*body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+*body.GroupID)
			return
		}
		if err := s.st.PatchRepo(fullName, body.GroupID, body.Tags, body.Note); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		rep, err := s.st.GetRepo(fullName)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, rep)
	case http.MethodDelete:
		if !s.st.RepoExists(fullName) {
			writeErr(w, 404, "NOT_FOUND", "repo not found")
			return
		}
		if err := s.st.DeleteRepo(fullName); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH, DELETE")
	}
}

func (s *server) handleRepoGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := s.st.ListRepoGroups()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"groups": groups})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			writeErr(w, 422, "VALIDATION", "name is required")
			return
		}
		g, err := s.st.CreateRepoGroup(body.Name)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, g)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleRepoGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := s.st.DeleteRepoGroup(id)
	if code == "NOT_FOUND" {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	if code == "GROUP_FIXED" {
		writeErr(w, 422, "GROUP_FIXED", "fixed groups cannot be deleted")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) handleRepoSyncAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	repos, err := s.st.ListRepos(store.RepoFilter{})
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	total := len(repos)
	synced, failed, rateLimited := 0, 0, 0
	for _, repo := range repos {
		rep, err := s.syncRepo(repo.FullName)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		switch rep.SyncStatus {
		case "synced", "partial":
			synced++
		case "rate_limited":
			rateLimited++
			// Stop at once: further requests would only burn quota.
			writeJSON(w, 200, map[string]any{
				"synced": synced, "failed": failed,
				"rate_limited": rateLimited, "total": total,
			})
			return
		default: // "failed", "never"
			failed++
		}
	}
	writeJSON(w, 200, map[string]any{
		"synced": synced, "failed": failed,
		"rate_limited": rateLimited, "total": total,
	})
}

// ---- YouTube channels (yt-dlp sync, no API key) ----

// validYTDate reports whether s is a "2006-01-02" calendar date.
func validYTDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// optString is a JSON request field that distinguishes "absent" from
// "present as null" from "present as a string": absent → Set=false;
// null → Set=true, Val=nil; "x" → Set=true, Val=&"x". PATCH uses it so
// a date sent as null or "" clears the field while an absent field
// leaves it unchanged.
type optString struct {
	Set bool
	Val *string
}

func (o *optString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Val = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Val = &s
	return nil
}

// syncYTChannel syncs one channel's four tabs into yt_videos. A failed
// sync keeps the old videos and only updates the channel's sync status
// (mirrors syncRepo). After a successful sync, rows outside the
// channel's date range are pruned.
func (s *server) syncYTChannel(id string) (*model.YTChannel, error) {
	ch, err := s.st.GetYTChannel(id)
	if err != nil {
		return nil, err
	}
	if ch.SyncMode == "single" {
		return s.refreshSingleChannel(ch) // only the videos added by URL
	}
	// IDs already stored with a complete row (description, and for
	// non-playlists a publish date) skip the backfill this sync; the
	// sync re-checks everything else. Playlists count as complete
	// once they have a description — they never get a publish date.
	knownIDs := map[string]bool{}
	existing, err := s.st.ListYTVideos(store.YTVideoFilter{ChannelID: ch.ChannelID, Limit: 1000})
	if err != nil {
		return nil, err
	}
	for _, v := range existing {
		if v.Description != "" && (v.Kind == "playlist" || v.PublishedAt != "") {
			knownIDs[v.ID] = true
		}
	}
	dateFrom, dateTo := "", ""
	if ch.DateFrom != nil {
		dateFrom = *ch.DateFrom
	}
	if ch.DateTo != nil {
		dateTo = *ch.DateTo
	}
	res, err := s.yt.SyncTabs(context.Background(), ch.ChannelID, dateFrom, dateTo, knownIDs)
	if err != nil {
		// Defensive: SyncTabs currently folds tab failures into Status.
		_ = s.st.SetYTChannelSyncStatus(id, "failed", err.Error())
		ch, err = s.st.GetYTChannel(id)
		return &ch, err
	}
	if res.Status == "failed" {
		_ = s.st.SetYTChannelSyncStatus(id, "failed", res.Error)
		ch, err = s.st.GetYTChannel(id)
		return &ch, err
	}
	name := res.ChannelName
	if name == "" {
		name = ch.Name
	}
	if err := s.st.UpdateYTChannelMeta(id, name, "synced", ""); err != nil {
		return nil, err
	}
	if res.Videos != nil {
		if err := s.st.UpsertYTVideos(ch.ChannelID, res.Videos); err != nil {
			return nil, err
		}
	}
	// A narrowed range (or a re-sync) can leave rows outside it behind.
	if dateFrom != "" || dateTo != "" {
		if err := s.st.PruneYTVideosOutsideRange(ch.ChannelID, dateFrom, dateTo); err != nil {
			return nil, err
		}
	}
	ch, err = s.st.GetYTChannel(id)
	return &ch, err
}

func (s *server) handleYTChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels, err := s.st.ListYTChannels()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"channels": channels})
	case http.MethodPost:
		var body struct {
			ChannelID string `json:"channel_id"`
			URL       string `json:"url"`
			Handle    string `json:"handle"`
			GroupID   string `json:"group_id"`
			DateFrom  string `json:"date_from"`
			DateTo    string `json:"date_to"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		dateFrom := strings.TrimSpace(body.DateFrom)
		dateTo := strings.TrimSpace(body.DateTo)
		if dateFrom != "" && !validYTDate(dateFrom) {
			writeErr(w, 422, "VALIDATION", "date_from must be YYYY-MM-DD")
			return
		}
		if dateTo != "" && !validYTDate(dateTo) {
			writeErr(w, 422, "VALIDATION", "date_to must be YYYY-MM-DD")
			return
		}
		if dateFrom != "" && dateTo != "" && dateFrom > dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		input := strings.TrimSpace(body.ChannelID)
		if input == "" {
			input = strings.TrimSpace(body.URL)
		}
		if input == "" {
			input = strings.TrimSpace(body.Handle)
		}
		channelID, err := s.yt.ResolveChannelID(r.Context(), input)
		if err != nil {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		if body.GroupID != "" && !s.st.YTGroupExists(body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
			return
		}
		if _, err := s.st.GetYTChannelByChannelID(channelID); err == nil {
			writeErr(w, 409, "CONFLICT", "channel already exists: "+channelID)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		// Keep the @handle when the user gave one; the channel name
		// comes from the tab listing's playlist_title on the first sync.
		handle := strings.TrimSpace(body.Handle)
		if handle == "" && strings.HasPrefix(input, "@") {
			handle = input
		}
		ch := model.YTChannel{
			ID: store.NewID("ytc_"), ChannelID: channelID, Handle: handle,
			Name: "", GroupID: body.GroupID, CreatedAt: store.Now(),
			SyncStatus: "never",
		}
		if dateFrom != "" {
			ch.DateFrom = &dateFrom
		}
		if dateTo != "" {
			ch.DateTo = &dateTo
		}
		if err := s.st.CreateYTChannel(ch); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		// Sync once immediately so the page shows videos right away; a
		// sync failure keeps the channel with its error status.
		synced, err := s.syncYTChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, synced)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleYTChannelSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/yt/channels/")
	parts := strings.Split(rest, "/")
	if len(parts) < 1 || parts[0] == "" {
		writeErr(w, 404, "NOT_FOUND", "channel not found")
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "sync" {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		if !s.st.YTChannelExists(id) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		// ?full=1 on a single-video channel = 同步该账号全部视频: switch it
		// to full mode, then run the normal tab sync.
		if r.URL.Query().Get("full") == "1" {
			if err := s.st.SetYTChannelSyncMode(id, "full"); err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
		}
		ch, err := s.syncYTChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
		return
	}
	if len(parts) != 1 {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		ch, err := s.st.GetYTChannel(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodPatch:
		// PATCH {group_id?, date_from?, date_to?}: a field present as
		// null or "" clears it; an absent field leaves it unchanged.
		// Returns the updated channel.
		ch, err := s.st.GetYTChannel(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		var body struct {
			GroupID  optString `json:"group_id"`
			DateFrom optString `json:"date_from"`
			DateTo   optString `json:"date_to"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		groupID := ch.GroupID
		if body.GroupID.Set {
			if body.GroupID.Val == nil || strings.TrimSpace(*body.GroupID.Val) == "" {
				groupID = ""
			} else {
				groupID = strings.TrimSpace(*body.GroupID.Val)
			}
			if groupID != "" && !s.st.YTGroupExists(groupID) {
				writeErr(w, 422, "VALIDATION", "group does not exist: "+groupID)
				return
			}
		}
		dateFrom, dateTo := ch.DateFrom, ch.DateTo
		if body.DateFrom.Set {
			if body.DateFrom.Val == nil || strings.TrimSpace(*body.DateFrom.Val) == "" {
				dateFrom = nil
			} else {
				v := strings.TrimSpace(*body.DateFrom.Val)
				if !validYTDate(v) {
					writeErr(w, 422, "VALIDATION", "date_from must be YYYY-MM-DD")
					return
				}
				dateFrom = &v
			}
		}
		if body.DateTo.Set {
			if body.DateTo.Val == nil || strings.TrimSpace(*body.DateTo.Val) == "" {
				dateTo = nil
			} else {
				v := strings.TrimSpace(*body.DateTo.Val)
				if !validYTDate(v) {
					writeErr(w, 422, "VALIDATION", "date_to must be YYYY-MM-DD")
					return
				}
				dateTo = &v
			}
		}
		if dateFrom != nil && dateTo != nil && *dateFrom > *dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		if err := s.st.UpdateYTChannel(id, groupID, dateFrom, dateTo); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		ch, err = s.st.GetYTChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodDelete:
		if !s.st.YTChannelExists(id) {
			writeErr(w, 404, "NOT_FOUND", "channel not found")
			return
		}
		if err := s.st.DeleteYTChannel(id); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH, DELETE")
	}
}

func (s *server) handleYTVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.addYTVideo(w, r) // single-video configuration, see yt_single.go
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
		return
	}
	q := r.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	kind := q.Get("kind")
	switch kind {
	case "", "video", "short", "live", "playlist":
	default:
		writeErr(w, 422, "VALIDATION", "kind must be video, short, live or playlist")
		return
	}
	videos, err := s.st.ListYTVideos(store.YTVideoFilter{
		ChannelID: q.Get("channel_id"),
		Q:         q.Get("q"),
		DateFrom:  q.Get("date_from"),
		DateTo:    q.Get("date_to"),
		Kind:      kind,
		Limit:     limit,
	})
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"videos": videos})
}

func (s *server) handleYTSyncAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	channels, err := s.st.ListYTChannels()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	total := len(channels)
	synced, failed, skipped := 0, 0, 0
	for _, ch := range channels {
		// Single-video channels are not pulled in full by 同步全部; their
		// own 同步该账号全部视频 button does that.
		if ch.SyncMode == "single" {
			skipped++
			continue
		}
		updated, err := s.syncYTChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		switch updated.SyncStatus {
		case "synced":
			synced++
		default: // "failed", "never"
			failed++
		}
	}
	writeJSON(w, 200, map[string]any{
		"synced": synced, "failed": failed, "total": total, "skipped": skipped,
	})
}

func (s *server) handleYTGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := s.st.ListYTGroups()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"groups": groups})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			writeErr(w, 422, "VALIDATION", "name is required")
			return
		}
		g, err := s.st.CreateYTGroup(body.Name)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, g)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleYTGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/yt/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := s.st.DeleteYTGroup(id)
	if code == "NOT_FOUND" {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	if code == "GROUP_FIXED" {
		writeErr(w, 422, "GROUP_FIXED", "fixed groups cannot be deleted")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
