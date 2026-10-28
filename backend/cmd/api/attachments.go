package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"xdigest/internal/model"
	"xdigest/internal/store"
)

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
		if !fileHeadMatches(fh, ext) {
			writeErr(w, 400, "INVALID_FILE_CONTENT", fmt.Sprintf("file %q: content does not match extension .%s", fh.Filename, ext))
			return
		}
	}
	if quota := attachmentQuotaBytes(); quota > 0 {
		var incoming int64
		for _, fh := range files {
			incoming += fh.Size
		}
		used, err := s.st.AttachmentBytes()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if overQuota(used, incoming, quota) {
			writeErr(w, 413, "QUOTA_EXCEEDED", fmt.Sprintf("附件总容量已达上限（%d MB），请先删除不需要的附件", quota>>20))
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
		// Uploaded files are untrusted: no sniffing, and an inline SVG/HTML
		// can never run script on this origin.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; sandbox")
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
