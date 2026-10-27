package main

import (
	"testing"
)

func TestPlanFiltersShortRepliesAndResumes(t *testing.T) {
	raw := []map[string]any{
		{"id": "1", "created_at": "2026-10-20T00:00:00Z", "text": "@a 谢谢 https://t.co/x"},
		{"id": "2", "created_at": "2026-10-21T00:00:00Z", "text": "今天分享一个用 Claude Code 自动跑测试的完整配置方法"},
		{"id": "3", "created_at": "2026-10-22T00:00:00Z", "text": "这条已经生成过了，但内容足够长不会被过滤掉的"},
		{"id": "4", "created_at": "2026-10-23T00:00:00Z", "long_text": "长文版本：这里有足够多的文字来通过过滤规则", "text": "短"},
	}
	existing := []post{{ID: "3"}}
	todo, short, done := plan(raw, existing, 15, false)
	if short != 1 || done != 1 || len(todo) != 2 || str(todo[0], "id") != "4" || str(todo[1], "id") != "2" {
		t.Fatalf("todo=%v short=%d done=%d", todo, short, done)
	}
	todo, _, done = plan(raw, existing, 15, true)
	if done != 0 || len(todo) != 3 {
		t.Fatalf("force should include existing: %d %d", len(todo), done)
	}
}

func TestParseModel(t *testing.T) {
	var m modelOut
	if err := parseModel("```json\n{\"title\":\"t\",\"summary\":\"s\",\"tags\":[\"#a\",\"a\",\" b \"],\"steps\":[]}\n```", &m); err != nil {
		t.Fatal(err)
	}
	if got := cleanTags(m.Tags); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("tags %v", got)
	}
	if err := parseModel(`{"title":"","summary":"s"}`, &m); err == nil {
		t.Fatal("empty title should fail")
	}
}
