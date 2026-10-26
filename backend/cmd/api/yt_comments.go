package main

// YouTube top comments (2026-10-27):
//
//	GET /api/v1/yt/comments?ids=a,b,c   (at most maxCommentIDs IDs)
//	    → 200 {"comments": {id: [YTComment...]}, "pending": [id...]}
//
// The page asks for the videos it is showing. Stored comments come back
// at once; a video whose comments were never fetched, or were fetched
// more than commentsTTL ago, is queued for ONE background worker that
// runs one yt-dlp call per video with a pause between calls (YouTube
// rate-limits busy clients). "pending" lists the IDs still queued or
// being fetched, so the page polls until it is empty. A failed fetch is
// not retried for commentsRetryAfter.

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"xdigest/internal/model"
)

const (
	maxCommentIDs      = 60
	commentsShown      = 5
	commentsTTL        = 7 * 24 * time.Hour
	commentsRetryAfter = 30 * time.Minute
	commentsFetchTO    = 90 * time.Second
	commentsPause      = 1500 * time.Millisecond
	commentsQueueCap   = 500
)

type commentQueue struct {
	mu       sync.Mutex
	queued   map[string]bool // queued or in progress
	failedAt map[string]time.Time
	ch       chan string
}

func newCommentQueue() *commentQueue {
	return &commentQueue{queued: map[string]bool{}, failedAt: map[string]time.Time{}, ch: make(chan string, commentsQueueCap)}
}

// enqueue adds id unless it is already queued, failed recently, or the
// queue is full. It reports whether id is now pending.
func (q *commentQueue) enqueue(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.queued[id] {
		return true
	}
	if t, ok := q.failedAt[id]; ok && time.Since(t) < commentsRetryAfter {
		return false
	}
	select {
	case q.ch <- id:
		q.queued[id] = true
		return true
	default:
		return false
	}
}

func (q *commentQueue) done(id string, failed bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.queued, id)
	if failed {
		q.failedAt[id] = time.Now()
	} else {
		delete(q.failedAt, id)
	}
}

// runCommentWorker fetches queued videos one at a time, forever.
func (s *server) runCommentWorker() {
	for id := range s.comments.ch {
		s.comments.done(id, !s.fetchComments(id))
		time.Sleep(commentsPause)
	}
}

func (s *server) fetchComments(id string) bool {
	video, err := s.st.GetYTVideo(id)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("yt comments %s: %v", id, err)
		}
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), commentsFetchTO)
	defer cancel()
	raw, err := s.yt.FetchTopComments(ctx, video.URL, commentsShown)
	if err != nil {
		log.Printf("yt comments %s: %v", id, err)
		return false
	}
	out := make([]model.YTComment, len(raw))
	for i, c := range raw {
		out[i] = model.YTComment{Author: c.Author, Text: c.Text, Likes: c.Likes}
		if c.Timestamp > 0 {
			out[i].PublishedAt = time.Unix(c.Timestamp, 0).In(shanghaiTZ).Format("2006-01-02 15:04:05")
		}
	}
	if err := s.st.ReplaceYTComments(id, out); err != nil {
		log.Printf("yt comments %s: store: %v", id, err)
		return false
	}
	return true
}

var shanghaiTZ = time.FixedZone("CST", 8*3600)

func (s *server) handleYTComments(w http.ResponseWriter, r *http.Request) {
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
	states, err := s.st.GetYTComments(ids)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	comments := map[string][]model.YTComment{}
	pending := []string{}
	for _, id := range ids {
		st := states[id]
		if st == nil {
			continue // unknown video
		}
		comments[id] = st.Comments
		stale := st.FetchedAt == ""
		if !stale {
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", st.FetchedAt, shanghaiTZ); err != nil || time.Since(t) > commentsTTL {
				stale = true
			}
		}
		if stale && s.comments.enqueue(id) {
			pending = append(pending, id)
		}
	}
	writeJSON(w, 200, map[string]any{"comments": comments, "pending": pending})
}
