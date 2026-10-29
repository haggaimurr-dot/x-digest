package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"xdigest/internal/github"
	"xdigest/internal/model"
	"xdigest/internal/store"
)

// ---- GitHub repos ----

// syncRepo runs one GitHub sync for an existing repo row and returns the
// fresh row. A failed or rate-limited sync never rolls back the row and
// never wipes old synced data: only sync_status/sync_error (and
// last_synced_at) change. A partial sync still applies the metadata and
// whatever sub-fetches succeeded; nil commit/release slices are skipped
// so previously synced rows are kept.
func (s *server) syncRepo(fullName string) (*model.Repo, error) {
	return s.syncRepoCtx(context.Background(), fullName)
}

// syncRepoCtx is syncRepo with a caller context (the background sync-all
// passes one that may wait for the GitHub rate-limit reset).
func (s *server) syncRepoCtx(ctx context.Context, fullName string) (*model.Repo, error) {
	owner, repo, err := github.ParseFullName(fullName)
	if err != nil {
		return nil, err
	}
	var prevRelease *model.RepoRelease
	if prev, err := s.st.GetRepo(fullName); err == nil {
		prevRelease = prev.LatestRelease
	}
	res, err := s.gh.Sync(ctx, owner, repo)
	if err != nil {
		// Defensive: Sync currently folds transport errors into Status.
		_ = s.st.SetRepoSyncStatus(fullName, "failed", err.Error())
		return s.st.GetRepo(fullName)
	}
	switch {
	case res.RateLimited:
		_ = s.st.SetRepoSyncStatus(fullName, "rate_limited", res.Error)
	case res.Status == "failed":
		_ = s.st.SetRepoSyncStatus(fullName, "failed", res.Error)
	default: // "synced" or "partial"
		if err := s.st.UpdateRepoMeta(fullName, res.Repo, res.Status, res.Error); err != nil {
			return nil, err
		}
		if res.Commits != nil {
			if err := s.st.UpsertRepoCommits(fullName, res.Commits); err != nil {
				return nil, err
			}
		}
		if res.Releases != nil {
			if err := s.st.UpsertRepoReleases(fullName, res.Releases); err != nil {
				return nil, err
			}
		}
	}
	cur, err := s.st.GetRepo(fullName)
	if err == nil {
		if title, url, ok := store.ReleaseNotice(fullName, prevRelease, cur.LatestRelease); ok {
			if err := s.st.AddNotification("release", title, url); err != nil {
				log.Printf("release notification %s: %v", fullName, err)
			}
		}
	}
	return cur, err
}

func (s *server) handleRepos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		sort := q.Get("sort")
		if sort != "stars" && sort != "pushed_at" {
			sort = "pushed_at"
		}
		repos, err := s.st.ListRepos(store.RepoFilter{Group: q.Get("group"), Tag: q.Get("tag"), Sort: sort})
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"repos": repos})
	case http.MethodPost:
		var body struct {
			FullName string   `json:"full_name"`
			GroupID  string   `json:"group_id"`
			Tags     []string `json:"tags"`
			Note     string   `json:"note"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		owner, name, err := github.ParseFullName(strings.TrimSpace(body.FullName))
		if err != nil {
			writeErr(w, 422, "VALIDATION", err.Error())
			return
		}
		if body.GroupID == "" {
			body.GroupID = "other"
		}
		if body.Tags == nil {
			body.Tags = []string{}
		}
		if s.st.RepoExists(owner + "/" + name) {
			writeErr(w, 409, "CONFLICT", "repo already exists")
			return
		}
		if !s.st.RepoGroupExists(body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+body.GroupID)
			return
		}
		created := model.Repo{
			FullName: owner + "/" + name, Owner: owner, Name: name,
			GroupID: body.GroupID, Tags: body.Tags, Note: body.Note,
			SyncStatus: "never",
		}
		if err := s.st.CreateRepo(created); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		// Sync immediately; a sync failure does not roll back the create —
		// it is recorded as sync_status rate_limited/failed on the row.
		rep, err := s.syncRepo(created.FullName)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, rep)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleRepoSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		writeErr(w, 404, "NOT_FOUND", "repo not found")
		return
	}
	owner, repo, err := github.ParseFullName(parts[0] + "/" + parts[1])
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "repo not found")
		return
	}
	fullName := owner + "/" + repo
	if len(parts) >= 3 && parts[2] == "summary" && (len(parts) == 3 || (len(parts) == 4 && parts[3] == "stream")) {
		s.handleRepoSummary(w, r, fullName, len(parts) == 4) // see repo_summary.go
		return
	}
	if len(parts) == 3 {
		switch parts[2] {
		case "replica":
			s.handleRepoReplica(w, r, fullName) // see notifications.go
			return
		case "commits":
			if r.Method != http.MethodGet {
				writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
				return
			}
			if !s.st.RepoExists(fullName) {
				writeErr(w, 404, "NOT_FOUND", "repo not found")
				return
			}
			commits, err := s.st.ListRepoCommits(fullName, 10)
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"commits": commits})
			return
		case "releases":
			if r.Method != http.MethodGet {
				writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
				return
			}
			if !s.st.RepoExists(fullName) {
				writeErr(w, 404, "NOT_FOUND", "repo not found")
				return
			}
			releases, err := s.st.ListRepoReleases(fullName, 100)
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"releases": releases})
			return
		case "sync":
			if r.Method != http.MethodPost {
				writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
				return
			}
			if !s.st.RepoExists(fullName) {
				writeErr(w, 404, "NOT_FOUND", "repo not found")
				return
			}
			rep, err := s.syncRepo(fullName)
			if err != nil {
				writeErr(w, 500, "DB_ERROR", err.Error())
				return
			}
			writeJSON(w, 200, rep)
			return
		default:
			writeErr(w, 404, "NOT_FOUND", "not found")
			return
		}
	}
	if len(parts) != 2 {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rep, err := s.st.GetRepo(fullName)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "NOT_FOUND", "repo not found")
			return
		}
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, rep)
	case http.MethodPatch:
		if !s.st.RepoExists(fullName) {
			writeErr(w, 404, "NOT_FOUND", "repo not found")
			return
		}
		var body struct {
			GroupID *string   `json:"group_id"`
			Tags    *[]string `json:"tags"`
			Note    *string   `json:"note"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.GroupID != nil && *body.GroupID != "" && !s.st.RepoGroupExists(*body.GroupID) {
			writeErr(w, 422, "VALIDATION", "group does not exist: "+*body.GroupID)
			return
		}
		if err := s.st.PatchRepo(fullName, body.GroupID, body.Tags, body.Note); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		rep, err := s.st.GetRepo(fullName)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, rep)
	case http.MethodDelete:
		if !s.st.RepoExists(fullName) {
			writeErr(w, 404, "NOT_FOUND", "repo not found")
			return
		}
		if err := s.st.DeleteRepo(fullName); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, PATCH, DELETE")
	}
}

func (s *server) handleRepoGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		groups, err := s.st.ListRepoGroups()
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
		g, err := s.st.CreateRepoGroup(body.Name)
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 201, g)
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET, POST")
	}
}

func (s *server) handleRepoGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := s.st.DeleteRepoGroup(id)
	if code == "NOT_FOUND" {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	if code == "GROUP_FIXED" {
		writeErr(w, 422, "GROUP_FIXED", "fixed groups cannot be deleted")
		return
	}
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
