package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"xdigest/internal/model"
	"xdigest/internal/store"
)

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

// /api/v1/user-tags/stats       GET  ?date_from&date_to → {tags: [{tag,authors,posts}]} (标签云)
// /api/v1/user-tags/aliases     GET → {aliases}; POST {alias, canonical}; DELETE ?alias=
// /api/v1/user-tags/duplicates  GET  → {groups: [[{tag,count}…]…]} (疑似重复)
// /api/v1/user-tags/merge       POST {from: [...], to} → {ok, changed}
func (s *server) handleUserTagSub(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/api/v1/user-tags/") {
	case "duplicates":
		if r.Method != http.MethodGet {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
			return
		}
		groups, err := s.st.UserTagDuplicates()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"groups": groups})
	case "stats":
		if r.Method != http.MethodGet {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
			return
		}
		from, to := r.URL.Query().Get("date_from"), r.URL.Query().Get("date_to")
		if (from != "" && !validYTDate(from)) || (to != "" && !validYTDate(to)) {
			writeErr(w, 422, "VALIDATION", "date_from/date_to must be YYYY-MM-DD")
			return
		}
		stats, err := s.st.UserTagStats(from, to)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"tags": stats})
	case "aliases":
		s.handleTagAliases(w, r)
	case "merge":
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		var body struct {
			From []string `json:"from"`
			To   string   `json:"to"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if store.NormalizeTag(body.To) == "" || len(body.From) == 0 {
			writeErr(w, 422, "VALIDATION", "from and to are required")
			return
		}
		n, err := s.st.MergeUserTags(body.From, body.To)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "changed": n})
	default:
		writeErr(w, 404, "NOT_FOUND", "not found")
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
		if errors.Is(err, store.ErrNameTaken) {
			writeErr(w, 409, "NAME_TAKEN", "类别名称已存在")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, c)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

// /api/v1/categories/{id}:
//
//	PATCH  {name}    rename → 200 category; 409 NAME_TAKEN; 404
//	DELETE           soft delete → 200 (undo within 7 days via restore)
//	POST   …/restore undo a delete → 200 category; 404 when purged
func (s *server) handleCategory(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/categories/")
	if rest == "assignments" {
		s.handleCategoryAssignments(w, r)
		return
	}
	if rest == "assignments/batch" {
		s.handleCategoryBatch(w, r)
		return
	}
	if id, ok := strings.CutSuffix(rest, "/restore"); ok && id != "" && !strings.Contains(id, "/") {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		c, err := s.st.RestoreCategory(id)
		s.writeCategoryResult(w, c, err)
		return
	}
	id := rest
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "category not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
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
		c, err := s.st.RenameCategory(id, body.Name)
		s.writeCategoryResult(w, c, err)
	case http.MethodDelete:
		if err := s.st.DeleteCategory(id); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only PATCH, DELETE")
	}
}

func (s *server) writeCategoryResult(w http.ResponseWriter, c model.Category, err error) {
	switch {
	case err == nil:
		writeJSON(w, 200, c)
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "NOT_FOUND", "category not found")
	case errors.Is(err, store.ErrNameTaken):
		writeErr(w, 409, "NAME_TAKEN", "类别名称已存在")
	default:
		writeErr(w, 500, "DB_ERROR", err.Error())
	}
}

// purgeCategoriesLoop drops categories soft-deleted over 7 days ago, at
// start-up and then daily.
func (s *server) purgeCategoriesLoop() {
	for {
		if n, err := s.st.PurgeDeletedCategories(7); err != nil {
			log.Printf("purge categories: %v", err)
		} else if n > 0 {
			log.Printf("purge categories: %d removed", n)
		}
		time.Sleep(24 * time.Hour)
	}
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

// POST /api/v1/categories/assignments/batch
//
//	{post_ids: [...], category_id, action: "add"|"remove"} → {ok, changed}
//
// Multi-select on the X daily page; one transaction.
func (s *server) handleCategoryBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var body struct {
		PostIDs    []string `json:"post_ids"`
		CategoryID string   `json:"category_id"`
		Action     string   `json:"action"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	ids := splitList(strings.Join(body.PostIDs, ","))
	switch {
	case len(ids) == 0 || len(ids) > 500:
		writeErr(w, 422, "VALIDATION", "post_ids: 1 to 500 ids")
		return
	case body.CategoryID == "":
		writeErr(w, 422, "VALIDATION", "category_id is required")
		return
	case body.Action != "add" && body.Action != "remove":
		writeErr(w, 422, "VALIDATION", `action must be "add" or "remove"`)
		return
	}
	n, err := s.st.BatchCategory(ids, body.CategoryID, body.Action == "add")
	if err != nil {
		if strings.HasPrefix(err.Error(), "unknown category") {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "changed": n})
}

func (s *server) handleTagAliases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		as, err := s.st.ListTagAliases()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"aliases": as})
	case http.MethodPost:
		var body store.TagAlias
		if !decodeBody(w, r, &body) {
			return
		}
		a, c := store.NormalizeTag(body.Alias), store.NormalizeTag(body.Canonical)
		if a == "" || c == "" || strings.EqualFold(a, c) {
			writeErr(w, 422, "VALIDATION", "alias and canonical are required and must differ")
			return
		}
		if err := s.st.SetTagAlias(a, c); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, store.TagAlias{Alias: a, Canonical: c})
	case http.MethodDelete:
		ok, err := s.st.DeleteTagAlias(r.URL.Query().Get("alias"))
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if !ok {
			writeErr(w, 404, "NOT_FOUND", "alias not found")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST, DELETE")
	}
}
