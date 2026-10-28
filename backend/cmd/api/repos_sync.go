package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"xdigest/internal/github"
	"xdigest/internal/store"
)

// Background "sync all repos" (2026-10-28).
//
//	POST /api/v1/repos/sync-all → 202 job status; 409 when one is running
//	GET  /api/v1/repos/sync-all → job status (the page polls it)
//
// Status: {running, total, done, synced, failed, rate_limited, waiting_until,
// started_at, finished_at}. Requests go through the GitHub client's limiter
// (internal/github/ratelimit.go): when the hourly quota runs low the job
// waits for the reset (waiting_until is shown as 「等待限流至 hh:mm」)
// instead of stopping, and unchanged repos cost only free 304s.

type repoSyncStatus struct {
	Running      bool   `json:"running"`
	Total        int    `json:"total"`
	Done         int    `json:"done"`
	Synced       int    `json:"synced"`
	Failed       int    `json:"failed"`
	RateLimited  int    `json:"rate_limited"`
	WaitingUntil string `json:"waiting_until,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

type repoSyncer struct {
	list func() ([]string, error)
	// sync syncs one repo and returns its sync_status.
	sync func(ctx context.Context, fullName string) (string, error)

	mu  sync.Mutex
	job repoSyncStatus
}

// maxRateLimitWait caps one wait for the GitHub reset (the reset is at most
// an hour away).
const maxRateLimitWait = 65 * time.Minute

func (s *server) newRepoSyncer() *repoSyncer {
	rs := &repoSyncer{
		list: func() ([]string, error) {
			repos, err := s.st.ListRepos(store.RepoFilter{})
			names := make([]string, len(repos))
			for i, r := range repos {
				names[i] = r.FullName
			}
			return names, err
		},
		sync: func(ctx context.Context, name string) (string, error) {
			rep, err := s.syncRepoCtx(ctx, name)
			if err != nil {
				return "", err
			}
			return rep.SyncStatus, nil
		},
	}
	if s.gh != nil {
		s.gh.SetWaitHook(rs.waiting)
	}
	return rs
}

func (rs *repoSyncer) status() repoSyncStatus {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.job
}

func (rs *repoSyncer) waiting(until time.Time) {
	rs.mu.Lock()
	rs.job.WaitingUntil = until.UTC().Format(time.RFC3339)
	rs.mu.Unlock()
}

// start launches a job; false when one is already running.
func (rs *repoSyncer) start() (bool, error) {
	names, err := rs.list()
	if err != nil {
		return false, err
	}
	rs.mu.Lock()
	if rs.job.Running {
		rs.mu.Unlock()
		return false, nil
	}
	rs.job = repoSyncStatus{Running: true, Total: len(names), StartedAt: time.Now().UTC().Format(time.RFC3339)}
	rs.mu.Unlock()
	go rs.run(names)
	return true, nil
}

func (rs *repoSyncer) run(names []string) {
	ctx := github.WithRateLimitWait(context.Background(), maxRateLimitWait)
	for _, name := range names {
		st, err := rs.sync(ctx, name)
		rs.mu.Lock()
		rs.job.Done++
		rs.job.WaitingUntil = ""
		switch {
		case err != nil:
			rs.job.Failed++
			log.Printf("repo sync-all %s: %v", name, err)
		case st == "synced" || st == "partial":
			rs.job.Synced++
		case st == "rate_limited":
			rs.job.RateLimited++
		default:
			rs.job.Failed++
		}
		rs.mu.Unlock()
	}
	rs.mu.Lock()
	rs.job.Running = false
	rs.job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	j := rs.job
	rs.mu.Unlock()
	log.Printf("repo sync-all: %d synced, %d failed, %d rate-limited of %d", j.Synced, j.Failed, j.RateLimited, j.Total)
}

func (s *server) handleRepoSyncAll(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.repoSync.status())
	case http.MethodPost:
		ok, err := s.repoSync.start()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		if !ok {
			writeErr(w, 409, "CONFLICT", "已有同步任务在运行")
			return
		}
		writeJSON(w, 202, s.repoSync.status())
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}
