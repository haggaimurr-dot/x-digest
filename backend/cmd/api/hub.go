package main

// videoHub serves one video platform under /api/v1/<prefix>/ with the
// same routes and JSON as the YouTube endpoints (2026-10-27), so the
// frontend page is shared. Bilibili is the first platform on it:
//
//	GET/POST            /api/v1/bili/channels
//	GET/PATCH/DELETE    /api/v1/bili/channels/{id}     POST .../sync
//	GET                 /api/v1/bili/videos
//	POST                /api/v1/bili/sync
//	GET/POST            /api/v1/bili/groups            DELETE .../{id}
//	GET/POST            /api/v1/bili/videos/{id}/summary
//	GET                 /api/v1/bili/comments?ids=
//
// Platform-specific work (resolving an uploader, syncing, reading a
// video's subtitles and comments) goes through hubPlatform; storage is
// store.HubStore on the <prefix>_* tables. AI summaries share the
// YouTube job pool (model, concurrency cap); comments get their own
// queue and worker. The YouTube handlers predate this and are separate.

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xdigest/internal/model"
	"xdigest/internal/store"
	"xdigest/internal/translate"
	"xdigest/internal/youtube"
	"xdigest/internal/ytsummary"
)

type hubPlatform interface {
	// Resolve turns user input into a channel ID and display name.
	Resolve(ctx context.Context, input string) (channelID, name string, err error)
	Sync(ctx context.Context, channelID, dateFrom, dateTo string, known map[string]bool) (*youtube.SyncResult, error)
	FetchDetail(ctx context.Context, videoID string) (*youtube.VideoDetail, error)
	FetchTopComments(ctx context.Context, videoID string, n int) ([]youtube.Comment, error)
}

type videoHub struct {
	prefix   string // URL segment and table prefix, e.g. "bili"
	idPrefix string // channel row id prefix, e.g. "bic_"
	kinds    map[string]bool
	st       *store.HubStore
	pf       hubPlatform
	sum      *summaryJobs
	comments *commentQueue
}

func (h *videoHub) register(mux *http.ServeMux) {
	base := "/api/v1/" + h.prefix
	mux.HandleFunc(base+"/channels", h.handleChannels)
	mux.HandleFunc(base+"/channels/", h.handleChannelSub)
	mux.HandleFunc(base+"/videos", h.handleVideos)
	mux.HandleFunc(base+"/videos/", h.handleVideoSub)
	mux.HandleFunc(base+"/sync", h.handleSyncAll)
	mux.HandleFunc(base+"/groups", h.handleGroups)
	mux.HandleFunc(base+"/groups/", h.handleGroup)
	mux.HandleFunc(base+"/comments", h.handleComments)
	go h.runCommentWorker()
}

// ---- channels ----

func (h *videoHub) syncChannel(id string) (*model.YTChannel, error) {
	ch, err := h.st.GetChannel(id)
	if err != nil {
		return nil, err
	}
	dateFrom, dateTo := "", ""
	if ch.DateFrom != nil {
		dateFrom = *ch.DateFrom
	}
	if ch.DateTo != nil {
		dateTo = *ch.DateTo
	}
	res, err := h.pf.Sync(context.Background(), ch.ChannelID, dateFrom, dateTo, nil)
	if err == nil && res.Status == "failed" {
		err = errors.New(res.Error)
	}
	if err != nil {
		_ = h.st.SetChannelSyncStatus(id, "failed", err.Error())
		ch, err = h.st.GetChannel(id)
		return &ch, err
	}
	name := res.ChannelName
	if name == "" {
		name = ch.Name
	}
	// A partial sync (one tab failed) is still "synced"; the failed tab
	// is reported in sync_error so the page shows it.
	if err := h.st.UpdateChannelMeta(id, name, "synced", strings.Join(res.FailedTabs, "；")); err != nil {
		return nil, err
	}
	if err := h.st.UpsertVideos(ch.ChannelID, res.Videos); err != nil {
		return nil, err
	}
	if err := h.st.PruneVideosOutsideRange(ch.ChannelID, dateFrom, dateTo); err != nil {
		return nil, err
	}
	ch, err = h.st.GetChannel(id)
	return &ch, err
}

func (h *videoHub) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels, err := h.st.ListChannels()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"channels": channels})
	case http.MethodPost:
		var body struct {
			ChannelID string `json:"channel_id"`
			URL       string `json:"url"`
			GroupID   string `json:"group_id"`
			DateFrom  string `json:"date_from"`
			DateTo    string `json:"date_to"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		dateFrom, dateTo := strings.TrimSpace(body.DateFrom), strings.TrimSpace(body.DateTo)
		if (dateFrom != "" && !validYTDate(dateFrom)) || (dateTo != "" && !validYTDate(dateTo)) {
			writeErr(w, 422, "VALIDATION", "dates must be YYYY-MM-DD")
			return
		}
		if dateFrom != "" && dateTo != "" && dateFrom > dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		if body.GroupID != "" && !h.st.GroupExists(body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
			return
		}
		input := strings.TrimSpace(body.ChannelID)
		if input == "" {
			input = strings.TrimSpace(body.URL)
		}
		channelID, name, err := h.pf.Resolve(r.Context(), input)
		if err != nil {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		if _, err := h.st.GetChannelByChannelID(channelID); err == nil {
			writeErr(w, 409, "CONFLICT", "channel already exists: "+channelID)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		ch := model.YTChannel{ID: store.NewID(h.idPrefix), ChannelID: channelID, Handle: "UID " + channelID,
			Name: name, GroupID: body.GroupID, CreatedAt: store.Now(), SyncStatus: "never"}
		if dateFrom != "" {
			ch.DateFrom = &dateFrom
		}
		if dateTo != "" {
			ch.DateTo = &dateTo
		}
		if err := h.st.CreateChannel(ch); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		synced, err := h.syncChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, synced)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (h *videoHub) handleChannelSub(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"+h.prefix+"/channels/"), "/")
	id := parts[0]
	if id == "" || len(parts) > 2 || (len(parts) == 2 && parts[1] != "sync") {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	if !h.st.ChannelExists(id) {
		writeErr(w, 404, "NOT_FOUND", "channel not found")
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		ch, err := h.syncChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
		return
	}
	switch r.Method {
	case http.MethodGet:
		ch, err := h.st.GetChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodPatch:
		ch, err := h.st.GetChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		var body struct {
			GroupID  optString `json:"group_id"`
			DateFrom optString `json:"date_from"`
			DateTo   optString `json:"date_to"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		groupID := ch.GroupID
		if body.GroupID.Set {
			groupID = ""
			if body.GroupID.Val != nil {
				groupID = strings.TrimSpace(*body.GroupID.Val)
			}
			if groupID != "" && !h.st.GroupExists(groupID) {
				writeErr(w, 422, "VALIDATION", "group does not exist: "+groupID)
				return
			}
		}
		dateFrom, dateTo := ch.DateFrom, ch.DateTo
		for _, f := range []struct {
			in  optString
			out **string
		}{{body.DateFrom, &dateFrom}, {body.DateTo, &dateTo}} {
			if !f.in.Set {
				continue
			}
			if f.in.Val == nil || strings.TrimSpace(*f.in.Val) == "" {
				*f.out = nil
				continue
			}
			v := strings.TrimSpace(*f.in.Val)
			if !validYTDate(v) {
				writeErr(w, 422, "VALIDATION", "dates must be YYYY-MM-DD")
				return
			}
			*f.out = &v
		}
		if dateFrom != nil && dateTo != nil && *dateFrom > *dateTo {
			writeErr(w, 422, "VALIDATION", "date_from must not be after date_to")
			return
		}
		if err := h.st.UpdateChannel(id, groupID, dateFrom, dateTo); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		ch, err = h.st.GetChannel(id)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, ch)
	case http.MethodDelete:
		if err := h.st.DeleteChannel(id); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH, DELETE")
	}
}

func (h *videoHub) handleVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	q := r.URL.Query()
	limit := 100
	if n, err := strconv.Atoi(q.Get("limit")); err == nil {
		limit = n
	}
	kind := q.Get("kind")
	if kind != "" && !h.kinds[kind] {
		writeErr(w, 422, "VALIDATION", "unknown kind: "+kind)
		return
	}
	videos, err := h.st.ListVideos(store.YTVideoFilter{ChannelID: q.Get("channel_id"), Q: q.Get("q"),
		DateFrom: q.Get("date_from"), DateTo: q.Get("date_to"), Kind: kind, Limit: limit})
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"videos": videos})
}

func (h *videoHub) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	channels, err := h.st.ListChannels()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	synced, failed := 0, 0
	for _, ch := range channels {
		updated, err := h.syncChannel(ch.ID)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if updated.SyncStatus == "synced" {
			synced++
		} else {
			failed++
		}
	}
	writeJSON(w, 200, map[string]any{"synced": synced, "failed": failed, "total": len(channels)})
}

func (h *videoHub) handleGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := h.st.ListGroups()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"groups": groups})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			writeErr(w, 422, "VALIDATION", "name is required")
			return
		}
		g, err := h.st.CreateGroup(strings.TrimSpace(body.Name))
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, g)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (h *videoHub) handleGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/"+h.prefix+"/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := h.st.DeleteGroup(id)
	switch {
	case code == "NOT_FOUND":
		writeErr(w, 404, "NOT_FOUND", "group not found")
	case code == "GROUP_FIXED":
		writeErr(w, 422, "GROUP_FIXED", "fixed groups cannot be deleted")
	case err != nil:
		writeErr(w, 500, "DB_ERROR", err.Error())
	default:
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

// ---- AI summary (same flow as yt_summary.go) ----

func (h *videoHub) handleVideoSub(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"+h.prefix+"/videos/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "summary" {
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
		writeErr(w, 422, "NOT_SUPPORTED", "合集不支持 AI 摘要，请对其中的单个视频生成")
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

// ---- top comments (same flow as yt_comments.go) ----

func (h *videoHub) runCommentWorker() {
	for id := range h.comments.ch {
		h.comments.done(id, !h.fetchComments(id))
		time.Sleep(commentsPause)
	}
}

func (h *videoHub) fetchComments(id string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), commentsFetchTO)
	defer cancel()
	raw, err := h.pf.FetchTopComments(ctx, id, commentsShown)
	if err != nil {
		log.Printf("%s comments %s: %v", h.prefix, id, err)
		return false
	}
	out := make([]model.YTComment, len(raw))
	for i, c := range raw {
		out[i] = model.YTComment{Author: c.Author, Text: c.Text, Likes: c.Likes}
		if c.Timestamp > 0 {
			out[i].PublishedAt = time.Unix(c.Timestamp, 0).In(shanghaiTZ).Format("2006-01-02 15:04:05")
		}
	}
	if err := h.st.ReplaceComments(id, out); err != nil {
		log.Printf("%s comments %s: store: %v", h.prefix, id, err)
		return false
	}
	return true
}

func (h *videoHub) handleComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	var ids []string
	seen := map[string]bool{}
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id = strings.TrimSpace(id); id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > maxCommentIDs {
		writeErr(w, 422, "VALIDATION", "at most 60 ids per request")
		return
	}
	states, err := h.st.GetComments(ids)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	comments := map[string][]model.YTComment{}
	pending := []string{}
	for _, id := range ids {
		st := states[id]
		if st == nil {
			continue
		}
		comments[id] = st.Comments
		stale := st.FetchedAt == ""
		if !stale {
			t, err := time.ParseInLocation("2006-01-02 15:04:05", st.FetchedAt, shanghaiTZ)
			stale = err != nil || time.Since(t) > commentsTTL
		}
		if stale && h.comments.enqueue(id) {
			pending = append(pending, id)
		}
	}
	writeJSON(w, 200, map[string]any{"comments": comments, "pending": pending})
}
