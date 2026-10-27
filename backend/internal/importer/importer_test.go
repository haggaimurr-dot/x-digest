package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAvatarFromRaw(t *testing.T) {
	for _, p := range []map[string]any{
		{"avatar": "https://example.com/a.jpg"},
		{"author": map[string]any{"profile_image_url_https": "https://example.com/a.jpg"}},
		{"user": map[string]any{"legacy": map[string]any{"profile_image_url": "https://example.com/a.jpg"}}},
	} {
		if got := avatarFromRaw(p); got != "https://example.com/a.jpg" {
			t.Fatalf("avatar lost: %q", got)
		}
	}
	if avatarFromRaw(map[string]any{"author": "miles_mazy"}) != "" {
		t.Fatal("handle is not an avatar")
	}
}

func TestRawExpandedURLsAndReferencedText(t *testing.T) {
	dir := t.TempDir()
	raw := `[{"id":"1","text":"看这个 https://t.co/aaa 和图 https://t.co/pic",
	  "entities":{"urls":[{"url":"https://t.co/aaa","expanded_url":"https://github.com/anthropics/claude-code"},
	                      {"url":"https://t.co/pic","expanded_url":"https://x.com/miles_mazy/status/1/photo/1"}]},
	  "referenced_tweets":[{"type":"quoted","id":"9","author_username":"johnzhou123","text":"我做了个插件 https://t.co/q",
	                        "entities":{"urls":[{"url":"https://t.co/q","expanded_url":"https://example.com/plugin"}]}}]},
	  {"id":"2","text":"老格式 https://t.co/old","referenced_tweets":[{"type":"quoted","id":"8"}]}]`
	path := filepath.Join(dir, "raw.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	p := got["1"]
	if !strings.Contains(p.Text, "https://github.com/anthropics/claude-code") || strings.Contains(p.Text, "t.co/aaa") {
		t.Errorf("short link not expanded: %q", p.Text)
	}
	if !strings.Contains(p.Text, "【引用 @johnzhou123】我做了个插件 https://example.com/plugin") {
		t.Errorf("quoted text missing: %q", p.Text)
	}
	want := "https://github.com/anthropics/claude-code,https://example.com/plugin"
	if strings.Join(p.Links, ",") != want {
		t.Errorf("links = %v, want %s (media link dropped)", p.Links, want)
	}
	old := got["2"]
	if old.Text != "老格式 https://t.co/old" || strings.Join(old.Links, ",") != "https://t.co/old" {
		t.Errorf("old raw format should import unchanged: %+v", old)
	}
}
