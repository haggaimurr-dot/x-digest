package main

import (
	"net/http"
	"strings"
	"testing"

	"xdigest/internal/model"
)

func TestRepoSummaryInput(t *testing.T) {
	rep := &model.Repo{FullName: "a/b", Description: "d", Language: "Go", Stars: 7, Tags: []string{"cli"},
		ReadmeIntro: "intro", LatestRelease: &model.RepoRelease{TagName: "v1.2", Name: "Big", Body: "notes"}}
	in := repoSummaryInput(rep, "# README")
	if in.Readme != "# README" || in.Description != "d" || in.Release != "v1.2 Big\nnotes" || in.Stars != 7 {
		t.Fatalf("%+v", in)
	}
	// Without a README the saved intro joins the description; Readme stays
	// empty so the summary is marked as based on the short facts.
	in = repoSummaryInput(rep, "")
	if in.Readme != "" || !strings.Contains(in.Description, "intro") {
		t.Fatalf("%+v", in)
	}
}

// DB-backed: the summary routes validate before any model call.
func TestRepoSummaryRoutes(t *testing.T) {
	s := testYTServer(t, nil)
	if err := s.st.CreateRepo(model.Repo{FullName: "a/b", Owner: "a", Name: "b", Tags: []string{}}); err != nil {
		t.Fatal(err)
	}
	h := http.HandlerFunc(s.handleRepoSub)
	code, body := ytCall(t, h, http.MethodGet, "/api/v1/repos/a/b/summary", nil)
	if sum, _ := body["summary"].(map[string]any); code != 200 || sum["status"] != "none" || sum["video_id"] != "a/b" {
		t.Fatalf("GET: code=%d body=%v", code, body)
	}
	if code, body = ytCall(t, h, http.MethodPost, "/api/v1/repos/a/b/summary", nil); code != 503 || body["error"] != "AI_NOT_CONFIGURED" {
		t.Fatalf("POST without AI: code=%d body=%v", code, body)
	}
	if code, _ = ytCall(t, h, http.MethodGet, "/api/v1/repos/a/b/summary?lang=fr", nil); code != 422 {
		t.Fatalf("bad lang: code=%d", code)
	}
	if code, _ = ytCall(t, h, http.MethodGet, "/api/v1/repos/a/nope/summary", nil); code != 404 {
		t.Fatalf("unknown repo: code=%d", code)
	}
	if code, _ = ytCall(t, h, http.MethodDelete, "/api/v1/repos/a/b/summary", nil); code != 405 {
		t.Fatalf("DELETE: code=%d", code)
	}
}
