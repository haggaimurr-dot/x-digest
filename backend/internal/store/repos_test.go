package store

import (
	"testing"

	"xdigest/internal/model"
)

// DB-backed (XDIGEST_TEST_DSN, see store_yt_test.go): a new repo gets its
// 入库时间, released_at is the newest non-draft release, and a repo's
// summary rows go away with it.
func TestRepoCreatedAndReleasedAt(t *testing.T) {
	st := testYTStore(t)
	for _, name := range []string{"a/one", "a/two"} {
		if err := st.CreateRepo(model.Repo{FullName: name, Owner: "a", Name: name[2:], Tags: []string{}}); err != nil {
			t.Fatal(err)
		}
	}
	p := func(s string) *string { return &s }
	if err := st.UpsertRepoReleases("a/one", []model.RepoRelease{
		{TagName: "v1", PublishedAt: p("2026-10-01 08:00:00")},
		{TagName: "v2", PublishedAt: p("2026-10-20 08:00:00")},
		{TagName: "v3-draft", PublishedAt: p("2026-10-28 08:00:00"), Draft: true},
	}); err != nil {
		t.Fatal(err)
	}
	repos, err := st.ListRepos(RepoFilter{})
	if err != nil || len(repos) != 2 {
		t.Fatalf("ListRepos = %v, %v", repos, err)
	}
	got := map[string]model.Repo{}
	for _, r := range repos {
		got[r.FullName] = r
		if r.CreatedAt == nil || *r.CreatedAt == "" {
			t.Fatalf("%s: created_at not set", r.FullName)
		}
	}
	if r := got["a/one"]; r.ReleasedAt == nil || *r.ReleasedAt != "2026-10-20 08:00:00" {
		t.Fatalf("a/one released_at = %v, want the newest non-draft release", r.ReleasedAt)
	}
	if r := got["a/two"]; r.ReleasedAt != nil {
		t.Fatalf("a/two released_at = %v, want nil (no release)", *r.ReleasedAt)
	}

	if err := st.StartRepoSummary("a/one", "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishRepoSummary(model.YTSummary{VideoID: "a/one", Lang: "zh-CN", Summary: "s", KeyPoints: []string{"k"}}); err != nil {
		t.Fatal(err)
	}
	sum, ok, err := st.GetRepoSummary("a/one", "zh-CN")
	if err != nil || !ok || sum.Status != "done" || sum.Summary != "s" || len(sum.KeyPoints) != 1 {
		t.Fatalf("summary = %+v %v %v", sum, ok, err)
	}
	if err := st.DeleteRepo("a/one"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.GetRepoSummary("a/one", "zh-CN"); ok {
		t.Fatal("summary should be deleted with the repo")
	}
}
