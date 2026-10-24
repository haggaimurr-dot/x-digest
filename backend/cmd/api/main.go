// Command api serves the x-digest Go backend (see ../SPEC.md contract).
//
// Environment:
//
//	XDIGEST_ADDR           listen address, default 127.0.0.1:18090
//	XDIGEST_DSN            MySQL DSN, default "xdigest:CHANGE_ME@tcp(127.0.0.1:3306)/xdigest"
//	XDIGEST_ATTACHMENT_DIR disk directory for attachment files,
//	                       default /var/lib/xdigest/attachments
//
// No authentication is implemented (see README.md "Security note").
package main

import (
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

	_ "github.com/go-sql-driver/mysql"

	"xdigest/internal/model"
	"xdigest/internal/store"
)

const maxAttachmentSize = 10 << 20 // 10 MiB per file

var allowedExts = map[string]bool{
	"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
	"ppt": true, "pptx": true, "txt": true, "md": true, "csv": true,
	"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true,
	"bmp": true, "avif": true, "svg": true, "ico": true,
	"zip": true, "rar": true, "7z": true, "tar": true, "gz": true, "tgz": true,
}

type server struct {
	st        *store.Store
	attachDir string
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
	s := &server{st: st, attachDir: attachDir}
	mux := http.NewServeMux()
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
		var body struct {
			Username string   `json:"username"`
			Tags     []string `json:"tags"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.Username == "" {
			writeErr(w, 422, "VALIDATION", "username is required")
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
		}
		if !decodeBody(w, r, &body) {
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
			} `json:"steps"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		var patches []store.StepPatch
		for _, st := range body.Steps {
			patches = append(patches, store.StepPatch{Idx: st.Idx, Status: st.Status, Note: st.Note})
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
			writeErr(w, 422, "FILE_TOO_LARGE", fmt.Sprintf("file %q: size %d exceeds 10MB limit", fh.Filename, fh.Size))
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
			writeErr(w, 422, "FILE_TOO_LARGE", fmt.Sprintf("file %q: size exceeds 10MB limit", fh.Filename))
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
