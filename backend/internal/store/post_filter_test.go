package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildPostWhere(t *testing.T) {
	if w, a := buildPostWhere(PostFilter{}); w != "" || a != nil {
		t.Fatalf("empty filter: %q %v", w, a)
	}
	w, a := buildPostWhere(PostFilter{
		Authors: []string{"alice", "bob"}, Tag: "#AI", Tags: []string{"AI", "Agent"},
		Categories: []string{"c1", "__uncat"}, Kind: "教程实操",
		DateFrom: "2026-10-01", DateTo: "2026-10-28",
	})
	for _, frag := range []string{
		"p.author IN (?,?)",
		"t.tag COLLATE utf8mb4_bin IN (?,?)", // "#AI" and "AI" fold into one
		"NOT EXISTS (SELECT 1 FROM post_categories pc JOIN categories c ON c.id=pc.category_id WHERE pc.post_id=p.id AND c.deleted_at IS NULL) OR EXISTS (SELECT 1 FROM post_categories pc WHERE pc.post_id=p.id AND pc.category_id IN (?))",
		"p.kind=?",
		"p.published_at >= ?",
		"p.published_at < DATE_ADD(?, INTERVAL 1 DAY)",
	} {
		if !strings.Contains(w, frag) {
			t.Errorf("WHERE lacks %q:\n%s", frag, w)
		}
	}
	want := []any{"alice", "bob", "AI", "Agent", "c1", "教程实操", "2026-10-01 00:00:00", "2026-10-28 00:00:00"}
	if !reflect.DeepEqual(a, want) {
		t.Errorf("args = %v\nwant   %v", a, want)
	}
	if strings.Count(w, "?") != len(a) {
		t.Errorf("placeholders %d != args %d", strings.Count(w, "?"), len(a))
	}
}

func TestBuildPostWhereQuery(t *testing.T) {
	// 标签：xxx is a plain keyword search now (the user-tag dimension is gone).
	w, a := buildPostWhere(PostFilter{Query: "标签：Claude"})
	if !strings.Contains(w, "p.title LIKE ?") || len(a) != 8 || a[0] != "%标签：Claude%" {
		t.Errorf("tag-prefix query: %s %v", w, a)
	}
	w, a = buildPostWhere(PostFilter{Query: "#DeepSeek", Kind: "未分类"})
	if !strings.Contains(w, "p.title LIKE ?") || !strings.Contains(w, "COALESCE(p.kind,'')=''") || len(a) != 8 || a[0] != "%DeepSeek%" {
		t.Errorf("plain query: %s %v", w, a)
	}
}

func TestPostOrder(t *testing.T) {
	if postOrder("oldest") != "p.published_at ASC, p.id ASC" || postOrder("") != "p.published_at DESC, p.id ASC" {
		t.Fatal("sort order")
	}
}

func TestBuildPostWhereMinChars(t *testing.T) {
	w, a := buildPostWhere(PostFilter{MinChars: 50})
	if !strings.Contains(w, "CHAR_LENGTH(TRIM(COALESCE(NULLIF(p.text, ''), p.summary, p.title))) >= ?") || len(a) != 1 || a[0] != 50 {
		t.Fatalf("min_chars: %s %v", w, a)
	}
	if w, _ := buildPostWhere(PostFilter{MinChars: 0}); w != "" {
		t.Fatalf("min_chars 0 must not filter: %s", w)
	}
}
