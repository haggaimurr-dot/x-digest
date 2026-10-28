package main

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"xdigest/internal/model"
)

func TestSplitList(t *testing.T) {
	if got := splitList(" a, b,,c "); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("splitList = %v", got)
	}
	if splitList("") != nil {
		t.Fatal("empty → nil")
	}
}

func TestFirstHitSnippet(t *testing.T) {
	p := model.Post{Title: "标题", Text: "前面有很多很多很多很多很多很多很多很多很多很多很多很多很多很多很多很多很多很多字，然后提到 DeepSeek 新模型，后面还有一些内容", Tags: []string{"AI"}}
	got := firstHitSnippet(p, "deepseek")
	if got == "" || !contains(got, "DeepSeek") || got[:3] != "…" {
		t.Fatalf("snippet = %q", got)
	}
	if firstHitSnippet(p, "标签:AI") != "" {
		t.Fatal("user-tag query has no snippet")
	}
	if firstHitSnippet(p, "nothing") != "" {
		t.Fatal("no hit → empty")
	}
}

func contains(s, sub string) bool { return runeIndex([]rune(s), []rune(sub)) >= 0 }

// PATCH /posts/{id} rejects a bad body before touching the store.
func TestPatchPostValidation(t *testing.T) {
	s := &server{}
	for body, want := range map[string]int{
		`{}`:                        422,
		`{"replica_enabled":"yes"}`: 422,
		`not json`:                  400,
	} {
		rec := httptest.NewRecorder()
		s.handlePost(rec, httptest.NewRequest("PATCH", "/api/v1/posts/p1", strings.NewReader(body)))
		if rec.Code != want {
			t.Errorf("body %s: got %d want %d", body, rec.Code, want)
		}
	}
	rec := httptest.NewRecorder()
	s.handlePost(rec, httptest.NewRequest("PUT", "/api/v1/posts/p1", nil))
	if rec.Code != 405 {
		t.Errorf("PUT: got %d", rec.Code)
	}
}
