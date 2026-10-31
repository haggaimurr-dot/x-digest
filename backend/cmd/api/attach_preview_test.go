package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A legacy file is converted once (with its extension, so LibreOffice picks
// the right filter) and served from the cache afterwards; a failed
// conversion leaves no cached file behind.
func TestOfficeConverterCachesPDF(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "att_1")
	if err := os.WriteFile(src, []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := &officeConverter{bin: "soffice", run: func(ctx context.Context, bin, in, outDir string) error {
		calls++
		if filepath.Ext(in) != ".xls" {
			t.Errorf("input %s has no .xls extension", in)
		}
		return os.WriteFile(filepath.Join(outDir, "in.pdf"), []byte("%PDF-1.4"), 0o644)
	}}
	for i := 0; i < 2; i++ {
		pdf, err := c.pdfFor(dir, "att_1", src, "xls")
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(pdf); string(b) != "%PDF-1.4" {
			t.Fatalf("pdf content %q", b)
		}
	}
	if calls != 1 {
		t.Fatalf("converted %d times; want 1", calls)
	}

	c.run = func(ctx context.Context, bin, in, outDir string) error { return errors.New("boom") }
	if _, err := c.pdfFor(dir, "att_2", src, "doc"); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(filepath.Join(dir, "previews", "att_2.pdf")); !os.IsNotExist(err) {
		t.Fatal("failed conversion left a cached pdf")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "previews")); len(entries) != 1 {
		t.Fatalf("previews dir = %v; want only att_1.pdf (work dirs removed)", entries)
	}

	cleanOrphanPreviews(dir, map[string]bool{"att_9": true})
	if _, err := os.Stat(filepath.Join(dir, "previews", "att_1.pdf")); !os.IsNotExist(err) {
		t.Fatal("orphan preview not removed")
	}
}

func TestOfficeConverterAvailable(t *testing.T) {
	if (&officeConverter{bin: "no-such-soffice-xdigest", run: runSoffice}).available() {
		t.Fatal("missing binary reported as available")
	}
}

// Real LibreOffice round trip, only where soffice is installed (the server
// after `apt install libreoffice-core`); skipped elsewhere.
func TestOfficeConverterRealSoffice(t *testing.T) {
	c := newOfficeConverter()
	if _, err := exec.LookPath(c.bin); err != nil {
		if os.Getenv("XDIGEST_REQUIRE_SOFFICE") != "" {
			t.Fatal("XDIGEST_REQUIRE_SOFFICE is set but soffice is not in PATH")
		}
		t.Skip("soffice not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "att_txt")
	// LibreOffice opens plain text through the .doc filter, enough to prove
	// the command line works.
	if err := os.WriteFile(src, []byte("hello x-digest"), 0o644); err != nil {
		t.Fatal(err)
	}
	pdf, err := c.pdfFor(dir, "att_txt", src, "doc")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(pdf); len(b) < 4 || string(b[:4]) != "%PDF" {
		t.Fatalf("not a PDF: %q", b[:min(len(b), 16)])
	}
}
