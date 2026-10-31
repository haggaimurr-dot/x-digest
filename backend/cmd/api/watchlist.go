package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"xdigest/internal/model"
)

// ---- watchlist ----

func (s *server) handleWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only GET")
		return
	}
	groups, accounts, err := s.st.ListWatchlist()
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	out := map[string]any{"groups": groups, "accounts": accounts}
	// ?with_stats=1 adds posts_30d: {username: posts in the last 30 days}.
	if r.URL.Query().Get("with_stats") == "1" {
		counts, err := s.st.PostCounts30d()
		if err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		stats := map[string]int{}
		for _, a := range accounts {
			stats[a.Username] = counts[a.Username]
		}
		out["posts_30d"] = stats
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/watchlist/accounts/bulk {usernames: [...] | text: "..."}
//
//	→ 200 {created: [...], existing: [...], invalid: [...]}
//
// One-shot import of a pasted following list (handles, @handles or
// x.com/handle links, separated by spaces, commas or new lines) into the
// default group. Nothing is fetched from X.
func (s *server) handleAccountsBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var body struct {
		Usernames []string `json:"usernames"`
		Text      string   `json:"text"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	valid, invalid := parseHandles(append(body.Usernames, strings.FieldsFunc(body.Text, isHandleSep)...))
	if len(valid)+len(invalid) == 0 || len(valid) > 500 {
		writeErr(w, 422, "VALIDATION", "1 to 500 usernames")
		return
	}
	created, existing := []string{}, []string{}
	for _, u := range valid {
		if s.st.AccountExists(u) {
			existing = append(existing, u)
			continue
		}
		if err := s.st.CreateAccount(model.Account{Username: u, Group: "other", Tags: []string{}}); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		created = append(created, u)
	}
	writeJSON(w, 200, map[string]any{"created": created, "existing": existing, "invalid": invalid})
}

var handleRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
var handleURLRe = regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.|mobile\.)?(?:x|twitter)\.com/([A-Za-z0-9_]{1,15})/?(?:[?#].*)?$`)

func isHandleSep(r rune) bool {
	return r == ',' || r == '，' || r == '、' || r == ';' || unicode.IsSpace(r)
}

// parseHandles normalises @handles / profile links, drops case-insensitive
// duplicates and returns what is not a valid X handle separately.
func parseHandles(in []string) (valid, invalid []string) {
	seen := map[string]bool{}
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if m := handleURLRe.FindStringSubmatch(s); m != nil {
			s = m[1]
		}
		s = strings.TrimPrefix(s, "@")
		if !handleRe.MatchString(s) {
			invalid = append(invalid, raw)
			continue
		}
		if k := strings.ToLower(s); !seen[k] {
			seen[k] = true
			valid = append(valid, s)
		}
	}
	return valid, invalid
}

func (s *server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
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
	g, err := s.st.CreateGroup(body.Name)
	if err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 201, g)
}

func (s *server) handleGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only DELETE")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/watchlist/groups/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, 404, "NOT_FOUND", "group not found")
		return
	}
	code, err := s.st.DeleteGroup(id)
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

func (s *server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only POST")
		return
	}
	var a model.Account
	if !decodeBody(w, r, &a) {
		return
	}
	if strings.TrimSpace(a.Username) == "" {
		writeErr(w, 422, "VALIDATION", "username is required")
		return
	}
	if a.Group == "" {
		a.Group = "other"
	}
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if s.st.AccountExists(a.Username) {
		writeErr(w, 409, "CONFLICT", "account already exists")
		return
	}
	if !s.st.GroupExists(a.Group) {
		writeErr(w, 422, "VALIDATION", "group does not exist: "+a.Group)
		return
	}
	if err := s.st.CreateAccount(a); err != nil {
		writeErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, 201, a)
}

func (s *server) handleAccount(w http.ResponseWriter, r *http.Request) {
	username, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/v1/watchlist/accounts/"))
	if username == "" || strings.Contains(username, "/") {
		writeErr(w, 404, "NOT_FOUND", "account not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		if !s.st.AccountExists(username) {
			writeErr(w, 404, "NOT_FOUND", "account not found")
			return
		}
		var body struct {
			Name   *string   `json:"name"`
			Avatar *string   `json:"avatar"`
			Group  *string   `json:"group"`
			Tags   *[]string `json:"tags"`
			Bio    *string   `json:"bio"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := s.st.PatchAccount(username, body.Name, body.Avatar, body.Group, body.Bio, body.Tags); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case http.MethodDelete:
		if !s.st.AccountExists(username) {
			writeErr(w, 404, "NOT_FOUND", "account not found")
			return
		}
		if err := s.st.DeleteAccount(username); err != nil {
			writeErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 405, "METHOD_NOT_ALLOWED", "only PATCH, DELETE")
	}
}
