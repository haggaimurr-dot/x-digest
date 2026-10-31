package main

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

// ---- AI summary ----
//
// GET returns the stored summary (status "none" when never generated);
// POST starts a background job (202; the page polls GET or listens on
// .../stream) unless one is done or running, force=1 regenerates. A long
// video takes well over a minute, longer than a request should stay open.
// A finished summary is served from the database, so only force=1 spends
// model calls again.

func (h *videoHub) handleVideoSub(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"+h.prefix+"/videos/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		if r.Method == http.MethodDelete {
			if err := h.st.DeleteVideo(parts[0]); err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		patchVideoState(w, r, h.st, h.prefix, parts[0]) // see video_state.go
		return
	}
	stream := len(parts) == 3 && parts[2] == "stream"
	if (len(parts) != 2 && !stream) || parts[0] == "" || parts[1] != "summary" {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = translate.TargetZh
	}
	if !translate.ValidTarget(lang) {
		writeErr(w, 422, "VALIDATION", "lang must be zh-CN or en")
		return
	}
	video, err := h.st.GetVideo(parts[0])
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "NOT_FOUND", "video not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	if stream {
		serveSummaryStream(w, r, h.sum, h.jobKey(video.ID, lang), func() (model.YTSummary, error) { return h.currentSummary(video.ID, lang) })
		return
	}
	switch r.Method {
	case http.MethodGet:
		sum, err := h.currentSummary(video.ID, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"summary": sum})
	case http.MethodPost:
		h.startSummary(w, video, lang, r.URL.Query().Get("force") == "1")
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (h *videoHub) jobKey(id, lang string) string { return h.prefix + ":" + id + "|" + lang }

// currentSummary reads the stored row. A row left "pending" with no job
// running in this process (the API restarted mid-job) reads as failed, so
// the page offers a retry instead of polling forever.
func (h *videoHub) currentSummary(id, lang string) (model.YTSummary, error) {
	sum, ok, err := h.st.GetSummary(id, lang)
	if err != nil {
		return sum, err
	}
	if !ok {
		return model.YTSummary{VideoID: id, Lang: lang, Status: "none", KeyPoints: []string{}, Chapters: []model.YTSummaryChapter{}}, nil
	}
	if sum.Status == "pending" && !h.sum.running(h.jobKey(id, lang)) {
		sum.Status, sum.Error = "failed", "上次生成被中断（服务重启），请重试"
	}
	return sum, nil
}

func (h *videoHub) startSummary(w http.ResponseWriter, video model.YTVideo, lang string, force bool) {
	if video.Kind == "playlist" {
		writeErr(w, 422, "NOT_SUPPORTED", h.playlistName+"不支持 AI 摘要，请对其中的单个视频生成")
		return
	}
	if video.LiveStatus == "is_live" || video.LiveStatus == "is_upcoming" {
		writeErr(w, 422, "NOT_SUPPORTED", "直播进行中或未开始，结束后再生成摘要")
		return
	}
	if !h.sum.llm.Configured() {
		writeErr(w, 503, "AI_NOT_CONFIGURED", "AI 未配置：服务器需要设置 TRANSLATE_API_KEY")
		return
	}
	reply := func(status int) {
		cur, err := h.currentSummary(video.ID, lang)
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
		cur, err := h.currentSummary(video.ID, lang)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if cur.Status == "done" || cur.Status == "pending" {
			reply(202)
			return
		}
	}
	key := h.jobKey(video.ID, lang)
	if !h.sum.claim(key) {
		reply(202)
		return
	}
	if err := h.st.StartSummary(video.ID, lang); err != nil {
		h.sum.release(key)
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	go h.runSummary(video, lang, key)
	reply(202)
}

func (h *videoHub) runSummary(video model.YTVideo, lang, key string) {
	defer h.sum.release(key)
	ctx, cancel := context.WithTimeout(context.Background(), summaryJobTimeout)
	defer cancel()
	fail := func(msg string) {
		if r := []rune(msg); len(r) > maxSummaryErrChars {
			msg = string(r[:maxSummaryErrChars]) + "…"
		}
		log.Printf("%s summary %s (%s) failed: %s", h.prefix, video.ID, lang, msg)
		_ = h.st.FailSummary(video.ID, lang, msg)
	}
	select {
	case h.sum.sem <- struct{}{}:
		defer func() { <-h.sum.sem }()
	case <-ctx.Done():
		fail("排队超时，请稍后重试")
		return
	}
	detail, err := h.pf.FetchDetail(ctx, video.ID)
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
	res, err := ytsummary.Summarize(ctx, h.sum.llm, detail, lang)
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
	if err := h.st.FinishSummary(model.YTSummary{VideoID: video.ID, Lang: lang, Summary: res.Summary,
		KeyPoints: keyPoints, Chapters: chapters, Resources: resources, Source: res.Source, TranscriptLang: detail.TranscriptLang,
		Model: h.sum.llm.Model}); err != nil {
		log.Printf("%s summary %s: store result: %v", h.prefix, video.ID, err)
	}
}
