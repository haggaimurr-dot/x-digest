package store

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"xdigest/internal/model"
)

func TestCleanOrphanFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	for name, mtime := range map[string]time.Time{"att_known": old, "att_orphan": old, "att_fresh": now} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, mtime, mtime)
	}
	_ = os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
	removed, err := CleanOrphanFiles(dir, map[string]bool{"att_known": true}, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "att_orphan" {
		t.Fatalf("removed %v, want [att_orphan]", removed)
	}
	left, _ := os.ReadDir(dir)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) != 3 || names[0] != "att_fresh" || names[1] != "att_known" {
		t.Fatalf("left %v", names)
	}
}

func TestReleaseNotice(t *testing.T) {
	v1 := &model.RepoRelease{TagName: "v1.0", HTMLURL: "https://github.com/o/r/releases/v1.0"}
	v2 := &model.RepoRelease{TagName: "v2.0", HTMLURL: "https://github.com/o/r/releases/v2.0"}
	if _, _, ok := ReleaseNotice("o/r", nil, v1); ok {
		t.Fatal("first sync must not notify")
	}
	if _, _, ok := ReleaseNotice("o/r", v1, v1); ok {
		t.Fatal("same tag must not notify")
	}
	if _, _, ok := ReleaseNotice("o/r", v1, nil); ok {
		t.Fatal("no release now must not notify")
	}
	title, url, ok := ReleaseNotice("o/r", v1, v2)
	if !ok || title != "o/r 发布了新版本 v2.0" || url != v2.HTMLURL {
		t.Fatalf("notice = %q %q %v", title, url, ok)
	}
}
