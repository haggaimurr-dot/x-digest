package main

import (
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// Daily AI quota (2026-10-28). Per-IP rate limits live in nginx
// (deploy/xdigest-18089.nginx: 10 r/m on AI paths); this is the global
// backstop on model spend:
//
//	AI_DAILY_LIMIT   max AI-spending requests per Shanghai calendar day,
//	                 all clients together. 0 / unset = unlimited.
//
// Over the limit: 429 AI_QUOTA_EXCEEDED (logged). Only POSTs that can start
// a model call count; reads of stored summaries are free.

var aiPathRe = regexp.MustCompile(`^/api/v1/(translate|posts/import|x/refresh(/retry)?|posts/summary/batch|posts/[^/]+/summary|(yt|bili)/videos/[^/]+/summary)$`)

type aiQuota struct {
	limit int
	now   func() time.Time

	mu    sync.Mutex
	day   string
	count int
}

func newAIQuota(limit int) *aiQuota { return &aiQuota{limit: limit, now: time.Now} }

// take counts one AI request; false when today's quota is used up.
func (q *aiQuota) take() (ok bool, used int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	day := q.now().In(shanghaiTZ).Format("2006-01-02")
	if day != q.day {
		q.day, q.count = day, 0
	}
	if q.count >= q.limit {
		return false, q.count
	}
	q.count++
	return true, q.count
}

func (q *aiQuota) wrap(next http.Handler) http.Handler {
	if q.limit <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && aiPathRe.MatchString(r.URL.Path) {
			if ok, used := q.take(); !ok {
				log.Printf("AI quota exceeded: %s %s (%d/%d today)", r.Method, r.URL.Path, used, q.limit)
				writeErr(w, 429, "AI_QUOTA_EXCEEDED", "今日 AI 调用次数已达上限（AI_DAILY_LIMIT）")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
