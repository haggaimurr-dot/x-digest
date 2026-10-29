package main

// GitHub repo AI summary (2026-10-29), same flow as the post summary:
//
//	GET  /api/v1/repos/{owner}/{name}/summary?lang=zh-CN|en   → 200 {"summary": ...} (status "none" = never generated)
//	POST /api/v1/repos/{owner}/{name}/summary?lang=…[&force=1]
//	     → 200 with the stored summary when done (and no force), else 202
//	       pending while a background job runs; the page polls GET.
//	GET  /api/v1/repos/{owner}/{name}/summary/stream?lang=…   (summary_stream.go)
//
// The job reads the README from GitHub (raw markdown) and adds the stored
// description, language, stars, tags and latest release. A README that
// cannot be read (none, rate limit) does not fail the job: the model gets
// the short facts only and the summary is marked as based on them. Jobs
// share the video / post summary pool (model, concurrency cap).

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"xdigest/internal/model"
	"xdigest/internal/translate"
	"xdigest/internal/ytsummary"
)

func repoJobKey(fullName, lang string) string { return "repo:" + fullName + "|" + lang }

func (s *server) handleRepoSummary(w http.ResponseWriter, r *http.Request, fullName string, stream bool) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = translate.TargetZh
	}
	if !translate.ValidTarget(lang) {
		writeErr(w, 422, "VALIDATION", "lang must be zh-CN or en")
		return
	}
	if !s.st.RepoExists(fullName) {
		writeErr(w, 404, "NOT_FOUND", "repo not found")
		return
	}
	if stream {
		if r.Method != http.MethodGet {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
			return
		}
		serveSummaryStream(w, r, s.sum, repoJobKey(fullName, lang), func() (model.YTSummary, error) { return s.currentRepoSummary(fullName, lang) })
		return
	}
	switch r.Method {
	case http.MethodGet:
		sum, err := s.currentRepoSummary(fullName, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"summary": sum})
	case http.MethodPost:
		s.startRepoSummary(w, fullName, lang, r.URL.Query().Get("force") == "1")
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

// currentRepoSummary reads the stored row; a "pending" row with no job
// running here (the API restarted mid-job) reads as failed.
func (s *server) currentRepoSummary(fullName, lang string) (model.YTSummary, error) {
	sum, ok, err := s.st.GetRepoSummary(fullName, lang)
	if err != nil {
		return sum, err
	}
	if !ok {
		return model.YTSummary{VideoID: fullName, Lang: lang, Status: "none", KeyPoints: []string{},
			Chapters: []model.YTSummaryChapter{}, Resources: []model.YTSummaryResource{}}, nil
	}
	if sum.Status == "pending" && !s.sum.running(repoJobKey(fullName, lang)) {
		sum.Status, sum.Error = "failed", "上次生成被中断（服务重启），请重试"
	}
	return sum, nil
}

func (s *server) startRepoSummary(w http.ResponseWriter, fullName, lang string, force bool) {
	if !s.sum.llm.Configured() {
		writeErr(w, 503, "AI_NOT_CONFIGURED", "AI 未配置：服务器需要设置 TRANSLATE_API_KEY")
		return
	}
	reply := func(code int) {
		cur, err := s.currentRepoSummary(fullName, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if cur.Status == "done" && code == 202 {
			code = 200
		}
		writeJSON(w, code, map[string]any{"summary": cur})
	}
	if !force {
		cur, err := s.currentRepoSummary(fullName, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if cur.Status == "done" || cur.Status == "pending" {
			reply(202)
			return
		}
	}
	key := repoJobKey(fullName, lang)
	if !s.sum.claim(key) {
		reply(202)
		return
	}
	if err := s.st.StartRepoSummary(fullName, lang); err != nil {
		s.sum.release(key)
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	go s.runRepoSummary(fullName, lang, key)
	writeJSON(w, 202, map[string]any{"summary": model.YTSummary{VideoID: fullName, Lang: lang, Status: "pending",
		KeyPoints: []string{}, Chapters: []model.YTSummaryChapter{}, Resources: []model.YTSummaryResource{}}})
}

// repoSummaryInput builds the model input from the stored repo, its latest
// release and the README text (may be "").
func repoSummaryInput(rep *model.Repo, readme string) ytsummary.RepoInput {
	in := ytsummary.RepoInput{FullName: rep.FullName, Description: rep.Description, Language: rep.Language,
		Stars: rep.Stars, Tags: rep.Tags, Readme: readme}
	if rel := rep.LatestRelease; rel != nil {
		in.Release = strings.TrimSpace(rel.TagName + " " + rel.Name + "\n" + rel.Body)
	}
	if strings.TrimSpace(in.Readme) == "" && rep.ReadmeIntro != "" {
		// The README intro saved by the last sync is better than nothing,
		// but it is not the README: keep Readme empty so the source says so.
		in.Description = strings.TrimSpace(in.Description + "\n" + rep.ReadmeIntro)
	}
	return in
}

func (s *server) runRepoSummary(fullName, lang, key string) {
	defer s.sum.release(key)
	ctx, cancel := context.WithTimeout(context.Background(), summaryJobTimeout)
	defer cancel()
	fail := func(msg string) {
		if r := []rune(msg); len(r) > maxSummaryErrChars {
			msg = string(r[:maxSummaryErrChars]) + "…"
		}
		log.Printf("repo summary %s (%s) failed: %s", fullName, lang, msg)
		_ = s.st.FailRepoSummary(fullName, lang, msg)
	}
	select {
	case s.sum.sem <- struct{}{}:
		defer func() { <-s.sum.sem }()
	case <-ctx.Done():
		fail("排队超时，请稍后重试")
		return
	}
	rep, err := s.st.GetRepo(fullName)
	if errors.Is(err, sql.ErrNoRows) {
		fail("仓库已被删除")
		return
	}
	if err != nil {
		fail("读取仓库失败：" + err.Error())
		return
	}
	readme := ""
	if s.gh != nil {
		if text, err := s.gh.FetchReadme(ctx, rep.Owner, rep.Name); err == nil {
			readme = text
		} else {
			log.Printf("repo summary %s: README: %v", fullName, err)
		}
	}
	res, err := ytsummary.SummarizeRepo(ctx, s.sum.llm, repoSummaryInput(rep, readme), lang)
	if err != nil {
		fail("AI 摘要失败：" + err.Error())
		return
	}
	keyPoints := res.KeyPoints
	if keyPoints == nil {
		keyPoints = []string{}
	}
	resources := make([]model.YTSummaryResource, len(res.Resources))
	for i, x := range res.Resources {
		resources[i] = model.YTSummaryResource{Name: x.Name, URL: x.URL, Note: x.Note}
	}
	if err := s.st.FinishRepoSummary(model.YTSummary{VideoID: fullName, Lang: lang, Summary: res.Summary,
		KeyPoints: keyPoints, Resources: resources, Source: res.Source, Model: s.sum.llm.Model}); err != nil {
		log.Printf("repo summary %s: store result: %v", fullName, err)
	}
}
