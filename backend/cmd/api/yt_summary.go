package main

// YouTube AI summary endpoints (2026-10-27):
//
//	GET  /api/v1/yt/videos/{id}/summary?lang=zh-CN|en
//	     → 200 {"summary": YTSummary}; status "none" when never generated.
//	POST /api/v1/yt/videos/{id}/summary?lang=zh-CN|en[&force=1]
//	     → 200 with the stored summary when one is done (and no force),
//	       otherwise 202 with status "pending" while a background job
//	       runs; the page polls GET until done / failed.
//
// The job runs in the background because a long video takes well over
// a minute (two yt-dlp calls plus one or more model calls), longer than
// a proxied request should stay open. At most summaryConcurrency jobs
// run at once; one (video, lang) never runs twice at the same time.
// There is no login (see README "Security note"): anyone who can reach
// the API can spend model calls, which is why a finished summary is
// served from the database and only force=1 regenerates it.

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"xdigest/internal/model"
	"xdigest/internal/translate"
	"xdigest/internal/youtube"
	"xdigest/internal/ytsummary"
)

const (
	summaryConcurrency = 2
	summaryJobTimeout  = 10 * time.Minute
	// summaryHTTPTimeout is per model call; a long final call over a
	// big transcript can take a couple of minutes.
	summaryHTTPTimeout = 3 * time.Minute
	maxSummaryErrChars = 500
)

type summaryJobs struct {
	llm      *translate.Client
	yt       *youtube.Client
	sem      chan struct{}
	mu       sync.Mutex
	inflight map[string]bool
}

func newSummaryJobs(yt *youtube.Client) *summaryJobs {
	llm := translate.NewFromEnv()
	llm.HTTP = &http.Client{Timeout: summaryHTTPTimeout}
	// Video summaries use deepseek-v4-pro (user's choice, 2026-10-27);
	// translation stays on TRANSLATE_MODEL (deepseek-flash).
	if m := os.Getenv("YT_SUMMARY_MODEL"); m != "" {
		llm.Model = m
	} else {
		llm.Model = "deepseek-v4-pro"
	}
	return &summaryJobs{llm: llm, yt: yt, sem: make(chan struct{}, summaryConcurrency), inflight: map[string]bool{}}
}

func (j *summaryJobs) running(key string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.inflight[key]
}

// claim marks key in flight; false when it already is.
func (j *summaryJobs) claim(key string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inflight[key] {
		return false
	}
	j.inflight[key] = true
	return true
}

func (j *summaryJobs) release(key string) {
	j.mu.Lock()
	delete(j.inflight, key)
	j.mu.Unlock()
}

func (s *server) handleYTVideoSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/yt/videos/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "summary" {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	id := parts[0]
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = translate.TargetZh
	}
	if !translate.ValidTarget(lang) {
		writeErr(w, 422, "VALIDATION", "lang must be zh-CN or en")
		return
	}
	video, err := s.st.GetYTVideo(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "NOT_FOUND", "video not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		sum, err := s.currentSummary(id, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"summary": sum})
	case http.MethodPost:
		s.startSummary(w, video, lang, r.URL.Query().Get("force") == "1")
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

// currentSummary reads the stored row. A row left "pending" with no job
// running in this process (the API restarted mid-job) reads as failed,
// so the page offers a retry instead of polling forever.
func (s *server) currentSummary(id, lang string) (model.YTSummary, error) {
	sum, ok, err := s.st.GetYTSummary(id, lang)
	if err != nil {
		return sum, err
	}
	if !ok {
		return model.YTSummary{VideoID: id, Lang: lang, Status: "none", KeyPoints: []string{}, Chapters: []model.YTSummaryChapter{}}, nil
	}
	if sum.Status == "pending" && !s.sum.running(id+"|"+lang) {
		sum.Status = "failed"
		sum.Error = "上次生成被中断（服务重启），请重试"
	}
	return sum, nil
}

func (s *server) startSummary(w http.ResponseWriter, video model.YTVideo, lang string, force bool) {
	if video.Kind == "playlist" {
		writeErr(w, 422, "NOT_SUPPORTED", "播放列表不支持 AI 摘要，请对其中的单个视频生成")
		return
	}
	if video.LiveStatus == "is_live" || video.LiveStatus == "is_upcoming" {
		writeErr(w, 422, "NOT_SUPPORTED", "直播进行中或未开始，结束后再生成摘要")
		return
	}
	if !s.sum.llm.Configured() {
		writeErr(w, 503, "AI_NOT_CONFIGURED", "AI 未配置：服务器需要设置 TRANSLATE_API_KEY")
		return
	}
	key := video.ID + "|" + lang
	if !force {
		cur, err := s.currentSummary(video.ID, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if cur.Status == "done" || cur.Status == "pending" {
			status := 200
			if cur.Status == "pending" {
				status = 202
			}
			writeJSON(w, status, map[string]any{"summary": cur})
			return
		}
	}
	if !s.sum.claim(key) {
		cur, err := s.currentSummary(video.ID, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 202, map[string]any{"summary": cur})
		return
	}
	if err := s.st.StartYTSummary(video.ID, lang); err != nil {
		s.sum.release(key)
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	go s.runSummary(video, lang, key)
	cur, err := s.currentSummary(video.ID, lang)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 202, map[string]any{"summary": cur})
}

func (s *server) runSummary(video model.YTVideo, lang, key string) {
	defer s.sum.release(key)
	ctx, cancel := context.WithTimeout(context.Background(), summaryJobTimeout)
	defer cancel()
	fail := func(msg string) {
		if len([]rune(msg)) > maxSummaryErrChars {
			msg = string([]rune(msg)[:maxSummaryErrChars]) + "…"
		}
		log.Printf("yt summary %s (%s) failed: %s", video.ID, lang, msg)
		if err := s.st.FailYTSummary(video.ID, lang, msg); err != nil {
			log.Printf("yt summary %s: store failure: %v", video.ID, err)
		}
	}
	select {
	case s.sum.sem <- struct{}{}:
		defer func() { <-s.sum.sem }()
	case <-ctx.Done():
		fail("排队超时，请稍后重试")
		return
	}
	detail, err := s.sum.yt.FetchDetail(ctx, video.URL)
	if err != nil {
		fail("读取视频信息失败：" + err.Error())
		return
	}
	if detail.Title == "" {
		detail.Title = video.Title
	}
	if detail.Description == "" {
		detail.Description = video.Description
	}
	if detail.Duration == 0 {
		detail.Duration = video.Duration
	}
	res, err := ytsummary.Summarize(ctx, s.sum.llm, detail, lang)
	if err != nil {
		fail("AI 摘要失败：" + err.Error())
		return
	}
	chapters := make([]model.YTSummaryChapter, len(res.Chapters))
	for i, c := range res.Chapters {
		chapters[i] = model.YTSummaryChapter{Start: c.Start, Title: c.Title, Summary: c.Summary}
	}
	resources := make([]model.YTSummaryResource, len(res.Resources))
	for i, x := range res.Resources {
		resources[i] = model.YTSummaryResource{Name: x.Name, URL: x.URL, Note: x.Note}
	}
	keyPoints := res.KeyPoints
	if keyPoints == nil {
		keyPoints = []string{}
	}
	if err := s.st.FinishYTSummary(model.YTSummary{
		VideoID: video.ID, Lang: lang, Summary: res.Summary, KeyPoints: keyPoints, Chapters: chapters, Resources: resources,
		Source: res.Source, TranscriptLang: detail.TranscriptLang, Model: s.sum.llm.Model,
	}); err != nil {
		log.Printf("yt summary %s: store result: %v", video.ID, err)
	}
}
