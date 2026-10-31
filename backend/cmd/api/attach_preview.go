package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Server-side preview of legacy Office files (2026-10-29):
//
//	GET /api/v1/attachments/{id}/preview.pdf
//
// .doc / .xls / .ppt have no light browser parser, so the server converts
// them to PDF with LibreOffice (`soffice --headless --convert-to pdf`) on
// the first preview and caches the result in <attachment dir>/previews/;
// the page then renders it with the same pdf.js viewer as uploaded PDFs.
// Conversions run one at a time (LibreOffice is heavy and its profile is
// not safe for parallel use). Without LibreOffice the endpoint answers 501
// and the page falls back to its text extraction.
//
//	SOFFICE_BIN   LibreOffice binary (default "soffice", looked up in PATH)

var legacyOfficeExts = map[string]bool{"doc": true, "xls": true, "ppt": true}

const officeConvertTimeout = 90 * time.Second

type officeConverter struct {
	mu  sync.Mutex // one conversion at a time
	bin string
	// run converts src into a PDF inside outDir (swapped in tests).
	run func(ctx context.Context, bin, src, outDir string) error
}

func newOfficeConverter() *officeConverter {
	return &officeConverter{bin: envOr("SOFFICE_BIN", "soffice"), run: runSoffice}
}

// available reports whether the LibreOffice binary can be found.
func (c *officeConverter) available() bool {
	if c.run == nil {
		return false
	}
	_, err := exec.LookPath(c.bin)
	return err == nil
}

func runSoffice(ctx context.Context, bin, src, outDir string) error {
	// A private profile dir: a desktop LibreOffice of the same user (or a
	// stale lock) would otherwise make the headless run exit silently.
	profile := filepath.ToSlash(filepath.Join(outDir, "profile"))
	if !strings.HasPrefix(profile, "/") { // Windows: C:/...
		profile = "/" + profile
	}
	profile = "file://" + profile
	cmd := exec.CommandContext(ctx, bin, "-env:UserInstallation="+profile, "--headless", "--norestore",
		"--convert-to", "pdf", "--outdir", outDir, src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("soffice: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// pdfFor returns the cached PDF of attachment id (stored at path, with
// extension ext), converting it first when needed.
func (c *officeConverter) pdfFor(attachDir, id, path, ext string) (string, error) {
	dir := filepath.Join(attachDir, "previews")
	dst := filepath.Join(dir, id+".pdf")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := os.Stat(dst); err == nil { // converted while we waited
		return dst, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	work, err := os.MkdirTemp(dir, "conv-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	// Stored files have no extension; LibreOffice picks the filter by it.
	src := filepath.Join(work, "in."+ext)
	if err := copyFile(path, src); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), officeConvertTimeout)
	defer cancel()
	if err := c.run(ctx, c.bin, src, work); err != nil {
		return "", err
	}
	out := filepath.Join(work, "in.pdf")
	if _, err := os.Stat(out); err != nil {
		return "", errors.New("soffice produced no PDF")
	}
	if err := os.Rename(out, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// removePreview deletes the cached PDF of a deleted attachment.
func removePreview(attachDir, id string) {
	_ = os.Remove(filepath.Join(attachDir, "previews", id+".pdf"))
}

// cleanOrphanPreviews removes cached PDFs whose attachment is gone (e.g.
// deleted with its replica), and conversion dirs left by a crash.
func cleanOrphanPreviews(attachDir string, known map[string]bool) {
	dir := filepath.Join(attachDir, "previews")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if strings.HasPrefix(name, "conv-") {
				if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
					_ = os.RemoveAll(filepath.Join(dir, name))
				}
			}
			continue
		}
		if !known[strings.TrimSuffix(name, ".pdf")] {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}

func (s *server) handleAttachmentPreview(w http.ResponseWriter, r *http.Request, id string) {
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
	if !legacyOfficeExts[att.Ext] {
		writeErr(w, 422, "VALIDATION", "preview.pdf is only for .doc/.xls/.ppt attachments")
		return
	}
	if !s.office.available() {
		writeErr(w, 501, "PREVIEW_UNAVAILABLE", "服务器未安装 LibreOffice，旧版 Office 文件暂不能在线预览")
		return
	}
	pdf, err := s.office.pdfFor(s.attachDir, id, path, att.Ext)
	if err != nil {
		log.Printf("attachment preview %s: %v", id, err)
		writeErr(w, 500, "CONVERT_FAILED", "文件转换失败，请下载原文件查看")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, pdf)
}
