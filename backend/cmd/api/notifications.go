package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Notifications and repo automation (2026-10-28).
//
//	GET  /api/v1/notifications              → {unread, notifications: [...]} (newest 50)
//	POST /api/v1/notifications/read {ids?}  → mark read (all when ids empty)
//	POST /api/v1/repos/{owner}/{name}/replica → 201/200 replica from the README steps
//
//	REPO_SYNC_INTERVAL  scheduled "sync all repos" (Go duration, default 24h;
//	                    "0" = off). Uses the same background job and rate
//	                    limiter as the button; a new release found by any
//	                    sync becomes a notification (repos.go syncRepoCtx).

func (s *server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/api/v1/notifications") {
	case "", "/":
		if r.Method != http.MethodGet {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
			return
		}
		list, unread, err := s.st.ListNotifications(50)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"unread": unread, "notifications": list})
	case "/read":
		if r.Method != http.MethodPost {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
			return
		}
		var body struct {
			IDs []string `json:"ids"`
		}
		if r.ContentLength != 0 && !decodeBody(w, r, &body) {
			return
		}
		if err := s.st.MarkNotificationsRead(splitList(strings.Join(body.IDs, ","))); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 404, "NOT_FOUND", "not found")
	}
}

// repoSyncInterval: REPO_SYNC_INTERVAL, default 24h; 0 or < 1h = off.
func repoSyncInterval() time.Duration {
	v := os.Getenv("REPO_SYNC_INTERVAL")
	if v == "" {
		return 24 * time.Hour
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < time.Hour {
		return 0
	}
	return d
}

func (s *server) scheduleRepoSync(tick <-chan time.Time) {
	for range tick {
		if ok, err := s.repoSync.start(); err != nil {
			log.Printf("scheduled repo sync: %v", err)
		} else if ok {
			log.Printf("scheduled repo sync started")
		}
	}
}

// handleRepoReplica: POST /api/v1/repos/{owner}/{name}/replica.
func (s *server) handleRepoReplica(w http.ResponseWriter, r *http.Request, fullName string) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	rep, existing, err := s.st.CreateReplicaFromRepo(fullName)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "NOT_FOUND", "repo not found")
	case err != nil && err.Error() == "no tutorial steps":
		writeErr(w, 422, "NO_TUTORIAL", "README 中没有解析出安装/上手步骤，无法建复刻")
	case err != nil:
		writeErr(w, 500, "DB_ERROR", err.Error())
	case existing:
		writeJSON(w, 200, rep)
	default:
		writeJSON(w, 201, rep)
	}
}
