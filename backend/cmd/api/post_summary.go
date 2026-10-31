package main

// X post AI summary (2026-10-27), same flow as the video summaries:
//
//	GET  /api/v1/posts/{id}/summary?lang=zh-CN|en   → 200 {"summary": ...} (status "none" = never generated)
//	POST /api/v1/posts/{id}/summary?lang=…[&force=1] body {"text": "...", "links": [...]}
//	     → 200 with the stored summary when done (and no force), else 202
//	       pending while a background job runs; the page polls GET.
//
// The original text comes from posts.text (imported from raw_posts.json);
// for rows imported before that column existed the page sends its copy
// (x-content.json) in the POST body. Without either, the model gets the
// title, the earlier digest and the steps, and the summary says so. Jobs
// share the video summary pool (model, concurrency cap, in-flight set).

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

const (
	maxPostBodyText  = 50000
	maxPostBodyLinks = 30
)

func (s *server) handlePostSummary(w http.ResponseWriter, r *http.Request, id string) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = translate.TargetZh
	}
	if !translate.ValidTarget(lang) {
		writeErr(w, 422, "VALIDATION", "lang must be zh-CN or en")
		return
	}
	post, err := s.st.GetPost(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "NOT_FOUND", "post not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		sum, err := s.currentPostSummary(id, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"summary": sum})
	case http.MethodPost:
		var body struct {
			Text  string   `json:"text"`
			Links []string `json:"links"`
		}
		if r.ContentLength != 0 && !decodeBody(w, r, &body) {
			return
		}
		if len([]rune(body.Text)) > maxPostBodyText {
			body.Text = string([]rune(body.Text)[:maxPostBodyText])
		}
		if len(body.Links) > maxPostBodyLinks {
			body.Links = body.Links[:maxPostBodyLinks]
		}
		// The stored original text wins; the body (page-side x-content.json)
		// only fills in for posts imported before posts.text existed.
		s.startPostSummary(w, id, lang, postSummaryInput(post, body.Text, body.Links), r.URL.Query().Get("force") == "1")
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func postJobKey(id, lang string) string { return "x:" + id + "|" + lang }

// GET /api/v1/posts/{id}/summary/stream?lang=… (summary_stream.go).
func (s *server) handlePostSummaryStream(w http.ResponseWriter, r *http.Request, id string) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = translate.TargetZh
	}
	if !translate.ValidTarget(lang) {
		writeErr(w, 422, "VALIDATION", "lang must be zh-CN or en")
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	serveSummaryStream(w, r, s.sum, postJobKey(id, lang), func() (model.YTSummary, error) { return s.currentPostSummary(id, lang) })
}

func (s *server) currentPostSummary(id, lang string) (model.YTSummary, error) {
	sum, ok, err := s.st.GetPostSummary(id, lang)
	if err != nil {
		return sum, err
	}
	if !ok {
		return model.YTSummary{VideoID: id, Lang: lang, Status: "none", KeyPoints: []string{},
			Chapters: []model.YTSummaryChapter{}, Resources: []model.YTSummaryResource{}}, nil
	}
	if sum.Status == "pending" && !s.sum.running(postJobKey(id, lang)) {
		sum.Status, sum.Error = "failed", "上次生成被中断（服务重启），请重试"
	}
	return sum, nil
}

func (s *server) startPostSummary(w http.ResponseWriter, id, lang string, in ytsummary.PostInput, force bool) {
	if !s.sum.llm.Configured() {
		writeErr(w, 503, "AI_NOT_CONFIGURED", "AI 未配置：服务器需要设置 TRANSLATE_API_KEY")
		return
	}
	status, err := s.queuePostSummary(id, lang, in, force)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	cur, err := s.currentPostSummary(id, lang)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	code := 202
	if cur.Status == "done" && status != "started" {
		code = 200
	}
	writeJSON(w, code, map[string]any{"summary": cur})
}

// queuePostSummary starts a background job unless none is needed: returns
// "exists" (done or pending, without force), "running" (another request
// holds the key) or "started".
func (s *server) queuePostSummary(id, lang string, in ytsummary.PostInput, force bool) (string, error) {
	if !force {
		cur, err := s.currentPostSummary(id, lang)
		if err != nil {
			return "", err
		}
		if cur.Status == "done" || cur.Status == "pending" {
			return "exists", nil
		}
	}
	key := postJobKey(id, lang)
	if !s.sum.claim(key) {
		return "running", nil
	}
	if err := s.st.StartPostSummary(id, lang); err != nil {
		s.sum.release(key)
		return "", err
	}
	go s.runPostSummary(id, lang, key, in)
	return "started", nil
}

// postSummaryInput builds the model input from the stored post: the text
// from posts.text wins; the request body only fills in when it is empty.
func postSummaryInput(post *model.Post, bodyText string, bodyLinks []string) ytsummary.PostInput {
	text, links := post.Text, post.Links
	if strings.TrimSpace(text) == "" {
		text, links = bodyText, bodyLinks
	}
	in := ytsummary.PostInput{Title: post.Title, Author: post.Author, Text: text, DigestSummary: post.Summary}
	for _, l := range links {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
			in.Links = append(in.Links, l)
		}
	}
	for _, st := range post.Steps {
		in.Steps = append(in.Steps, st.Text)
	}
	return in
}

// POST /api/v1/posts/summary/batch {ids: [...], lang, force}
//
//	→ 202 {queued, skipped, missing, results: {id: "started"|"exists"|"running"|"missing"}}
//
// Queues one summary job per post on the shared pool (SUMMARY_CONCURRENCY);
// posts that already have a summary are skipped unless force. AI_DAILY_LIMIT
// counts requests, so one batch is capped at maxBatchSummaries posts.
const maxBatchSummaries = 30

func (s *server) handlePostSummaryBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var body struct {
		IDs   []string `json:"ids"`
		Lang  string   `json:"lang"`
		Force bool     `json:"force"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	ids := splitList(strings.Join(body.IDs, ","))
	if body.Lang == "" {
		body.Lang = translate.TargetZh
	}
	switch {
	case len(ids) == 0 || len(ids) > maxBatchSummaries:
		writeErr(w, 422, "VALIDATION", "ids: 1 to 30 post ids")
		return
	case !translate.ValidTarget(body.Lang):
		writeErr(w, 422, "VALIDATION", "lang must be zh-CN or en")
		return
	case !s.sum.llm.Configured():
		writeErr(w, 503, "AI_NOT_CONFIGURED", "AI 未配置：服务器需要设置 TRANSLATE_API_KEY")
		return
	}
	results := map[string]string{}
	queued, skipped, missing := 0, 0, 0
	for _, id := range ids {
		post, err := s.st.GetPost(id)
		if errors.Is(err, sql.ErrNoRows) {
			results[id] = "missing"
			missing++
			continue
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		st, err := s.queuePostSummary(id, body.Lang, postSummaryInput(post, "", nil), body.Force)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		results[id] = st
		if st == "started" {
			queued++
		} else {
			skipped++
		}
	}
	writeJSON(w, 202, map[string]any{"queued": queued, "skipped": skipped, "missing": missing, "results": results})
}

func (s *server) runPostSummary(id, lang, key string, in ytsummary.PostInput) {
	defer s.sum.release(key)
	ctx, cancel := context.WithTimeout(context.Background(), summaryJobTimeout)
	defer cancel()
	fail := func(msg string) {
		if r := []rune(msg); len(r) > maxSummaryErrChars {
			msg = string(r[:maxSummaryErrChars]) + "…"
		}
		log.Printf("post summary %s (%s) failed: %s", id, lang, msg)
		_ = s.st.FailPostSummary(id, lang, msg)
	}
	select {
	case s.sum.sem <- struct{}{}:
		defer func() { <-s.sum.sem }()
	case <-ctx.Done():
		fail("排队超时，请稍后重试")
		return
	}
	res, err := ytsummary.SummarizePost(ctx, s.sum.llm, in, lang)
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
	if err := s.st.FinishPostSummary(model.YTSummary{VideoID: id, Lang: lang, Summary: res.Summary,
		KeyPoints: keyPoints, Resources: resources, Source: res.Source, Model: s.sum.llm.Model}); err != nil {
		log.Printf("post summary %s: store result: %v", id, err)
	}
}
