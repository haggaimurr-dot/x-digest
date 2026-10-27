package importer

import "testing"

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
