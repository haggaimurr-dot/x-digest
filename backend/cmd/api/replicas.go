package main

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"xdigest/internal/store"
)

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
	if len(parts) == 2 && parts[1] == "clone" {
		// POST {copy_notes?: bool} → 201 new standalone flow, steps reset to
		// idle, attachments not copied.
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		var body struct {
			CopyNotes bool `json:"copy_notes"`
		}
		if r.ContentLength != 0 && !decodeBody(w, r, &body) {
			return
		}
		rep, err := s.st.CloneReplica(id, body.CopyNotes)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "replica not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, rep)
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
