package main

import (
	"net/http"
	"strings"
)

// Accounts of the scheduled 更新帖子 (2026-10-29):
//
//	GET /api/v1/x/refresh/accounts  → {"usernames": [...], "interval": "6h0m0s" or ""}
//	PUT /api/v1/x/refresh/accounts  {"usernames": [...]} replaces the set
//	    (empty list = none); 422 lists usernames not on the watchlist.
//
// REFRESH_INTERVAL refreshes only these accounts; "interval" is "" when
// the schedule is off, so the page can say the setting has no effect yet.

type autoRefreshStore interface {
	AutoRefreshUsernames() ([]string, error)
	SetAutoRefresh(usernames []string) error
	AccountExists(username string) bool
}

const maxAutoRefreshAccounts = 500

func (x *xImporter) handleRefreshAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		var body struct {
			Usernames []string `json:"usernames"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		names := []string{}
		seen := map[string]bool{}
		var unknown []string
		for _, n := range body.Usernames {
			n = strings.TrimPrefix(strings.TrimSpace(n), "@")
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			if !x.auto.AccountExists(n) {
				unknown = append(unknown, n)
				continue
			}
			names = append(names, n)
		}
		if len(unknown) > 0 {
			writeErr(w, 422, "VALIDATION", "不在关注列表中的账号："+strings.Join(unknown, ", "))
			return
		}
		if len(names) > maxAutoRefreshAccounts {
			writeErr(w, 422, "VALIDATION", "at most 500 accounts")
			return
		}
		if err := x.auto.SetAutoRefresh(names); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PUT")
		return
	}
	names, err := x.auto.AutoRefreshUsernames()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	interval := ""
	if x.interval > 0 {
		interval = x.interval.String()
	}
	writeJSON(w, 200, map[string]any{"usernames": names, "interval": interval})
}
