package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"xdigest/internal/store"
)

// Saved filter views of the X daily page (2026-10-28).
//
//	GET    /api/v1/views                 → {views: [{id,name,filter,sort_order}]}
//	POST   /api/v1/views {name, filter}  → 201 view; 409 NAME_TAKEN; 422
//	DELETE /api/v1/views/{id}            → 200; 404

const maxViewFilterBytes = 8 << 10

// validView checks a POST body; "" when fine, else the 422 message.
func validView(name string, filter json.RawMessage) string {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "name is required (at most 64 characters)"
	}
	f := bytes.TrimSpace(filter)
	if len(f) == 0 || f[0] != '{' || !json.Valid(f) {
		return "filter must be a JSON object"
	}
	if len(f) > maxViewFilterBytes {
		return "filter is too large (8KB max)"
	}
	return ""
}

func (s *server) handleViews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		views, err := s.st.ListViews()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"views": views})
	case http.MethodPost:
		var body struct {
			Name   string          `json:"name"`
			Filter json.RawMessage `json:"filter"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if msg := validView(body.Name, body.Filter); msg != "" {
			writeErr(w, 422, "VALIDATION", msg)
			return
		}
		v, err := s.st.CreateView(strings.TrimSpace(body.Name), bytes.TrimSpace(body.Filter))
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, 409, "NAME_TAKEN", "视图名称已存在")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, v)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleView(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/views/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "view not found")
		return
	}
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	ok, err := s.st.DeleteView(id)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "view not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
