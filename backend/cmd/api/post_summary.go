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
		text, links := post.Text, post.Links
		if strings.TrimSpace(text) == "" {
			text, links = body.Text, body.Links
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
		s.startPostSummary(w, id, lang, in, r.URL.Query().Get("force") == "1")
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func postJobKey(id, lang string) string { return "x:" + id + "|" + lang }

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
	reply := func(status int) {
		cur, err := s.currentPostSummary(id, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if cur.Status == "done" && status == 202 {
			status = 200
		}
		writeJSON(w, status, map[string]any{"summary": cur})
	}
	if !force {
		cur, err := s.currentPostSummary(id, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if cur.Status == "done" || cur.Status == "pending" {
			reply(202)
			return
		}
	}
	key := postJobKey(id, lang)
	if !s.sum.claim(key) {
		reply(202)
		return
	}
	if err := s.st.StartPostSummary(id, lang); err != nil {
		s.sum.release(key)
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	go s.runPostSummary(id, lang, key, in)
	reply(202)
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
