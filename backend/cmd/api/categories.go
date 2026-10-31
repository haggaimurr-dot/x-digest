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

// ---- tags / categories ----

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
