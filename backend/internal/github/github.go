// Package github syncs public (or, with a token, private) GitHub repository
// metadata into the x-digest repos tables. Standard library only.
//
// Sync flow per repository — about 6 requests, in order:
//
//	GET /repos/{owner}/{repo}
//	GET /repos/{owner}/{repo}/commits?per_page=10
//	GET /repos/{owner}/{repo}/releases?per_page=100&page=1.. (up to 100 releases)
//	GET /search/issues?q=repo:{o}/{r}+type:issue+state:open (total_count)
//	GET /search/issues?q=repo:{o}/{r}+type:pr+state:open   (total_count)
//	GET /repos/{owner}/{repo}/readme (Accept: application/vnd.github.raw)
//
// The search calls are needed because the repo payload's open_issues_count
// counts issues AND pull requests together. If a search call fails or is
// rate-limited, OpenIssues/OpenPRs stay nil and the result is "partial"
// with the reason in Error — counts are never invented.
//
// Rate limiting: X-RateLimit-Remaining/Reset are read on every response.
// A 403 with 0 remaining means the caller must keep the old row untouched
// (Result.RateLimited == true) and, for sync-all, stop immediately.
//
// The token comes only from the GITHUB_TOKEN environment variable. It is
// never written to disk, never logged, and never included in errors.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"xdigest/internal/model"
)

const (
	defaultBaseURL = "https://api.github.com"
	userAgent      = "x-digest"
	requestTimeout = 15 * time.Second
)

var shanghai = time.FixedZone("CST", 8*3600)

var segmentRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ParseFullName validates "owner/repo" (each segment: letters, digits,
// '.', '_' or '-'). It returns the segments as given; callers compare
// case-insensitively (MySQL's default collation already does) but store
// the original spelling.
func ParseFullName(s string) (owner, repo string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("full_name must be owner/repo")
	}
	if !segmentRe.MatchString(parts[0]) || !segmentRe.MatchString(parts[1]) {
		return "", "", fmt.Errorf("full_name segments may only contain letters, digits, '.', '_' or '-'")
	}
	return parts[0], parts[1], nil
}

// Client is a minimal GitHub API client.
type Client struct {
	HTTP    *http.Client
	Token   string // empty = unauthenticated
	BaseURL string
}

// NewFromEnv builds a client reading GITHUB_TOKEN from the environment.
func NewFromEnv() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: requestTimeout},
		Token:   os.Getenv("GITHUB_TOKEN"),
		BaseURL: defaultBaseURL,
	}
}

// SyncResult is one repository sync. Status is one of
// "synced", "partial", "rate_limited", "failed".
type SyncResult struct {
	Repo        model.Repo
	Commits     []model.RepoCommit
	Releases    []model.RepoRelease
	Status      string
	Error       string
	RateLimited bool
}

// rateLimit tracks the headers of the last response.
type rateLimit struct {
	remaining int
	reset     time.Time
	hasHeader bool
}

func parseRateLimit(h http.Header) rateLimit {
	rl := rateLimit{remaining: -1}
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.remaining = n
			rl.hasHeader = true
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			rl.reset = time.Unix(n, 0)
		}
	}
	return rl
}

func (rl rateLimit) exhausted() bool { return rl.hasHeader && rl.remaining == 0 }

// do performs one authenticated GET and returns the body bytes. A non-2xx
// status is returned as an error carrying the status code; rate-limit
// exhaustion is reported separately so callers can preserve old data.
func (c *Client) do(ctx context.Context, path string, query url.Values) ([]byte, rateLimit, error) {
	return c.doWithAccept(ctx, path, query, "application/vnd.github+json")
}

// doWithAccept is do with an overridable Accept header. The README fetch
// uses "application/vnd.github.raw" to get the markdown text instead of
// the JSON metadata envelope.
func (c *Client) doWithAccept(ctx context.Context, path string, query url.Values, accept string) ([]byte, rateLimit, error) {
	u := strings.TrimSuffix(c.BaseURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, rateLimit{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, rateLimit{}, err
	}
	defer resp.Body.Close()
	rl := parseRateLimit(resp.Header)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, rl, err
	}
	if resp.StatusCode == http.StatusForbidden && rl.exhausted() {
		return nil, rl, errRateLimited{reset: rl.reset}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, rl, statusError{code: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	return body, rl, nil
}

// doDecode is do for large JSON payloads: it decodes the response stream
// instead of buffering the whole body. A /releases page of 100 items can
// exceed 30MB (mostly release assets we never read), far past the 4MB cap
// in doWithAccept, so buffering it truncates the JSON and breaks parsing.
// Streaming decode only materialises the fields we declare.
func (c *Client) doDecode(ctx context.Context, path string, query url.Values, target any) (rateLimit, error) {
	u := strings.TrimSuffix(c.BaseURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return rateLimit{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return rateLimit{}, err
	}
	defer resp.Body.Close()
	rl := parseRateLimit(resp.Header)
	if resp.StatusCode == http.StatusForbidden && rl.exhausted() {
		return rl, errRateLimited{reset: rl.reset}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return rl, statusError{code: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return rl, err
	}
	return rl, nil
}

// errRateLimited signals a 403 with X-RateLimit-Remaining: 0.
type errRateLimited struct{ reset time.Time }

func (e errRateLimited) Error() string {
	if e.reset.IsZero() {
		return "GitHub API rate limit exhausted"
	}
	return fmt.Sprintf("GitHub API rate limit exhausted, resets at %s", e.reset.Format(time.RFC3339))
}

// statusError is a non-2xx GitHub response (not rate limiting).
type statusError struct {
	code int
	body string
}

func (e statusError) Error() string {
	msg := e.body
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Sprintf("GitHub API returned status %d: %s", e.code, msg)
}

// formatGitHubTime converts an RFC3339 timestamp (or nil/empty) to a
// Shanghai wall-time string for DATETIME storage; nil stays nil.
func formatGitHubTime(s *string) (*string, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, err
	}
	v := t.In(shanghai).Format("2006-01-02 15:04:05")
	return &v, nil
}

// parseSearchTotal extracts total_count from a /search/issues response.
func parseSearchTotal(body []byte) (int, error) {
	var v struct {
		TotalCount int `json:"total_count"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return 0, err
	}
	return v.TotalCount, nil
}

// Sync fetches one repository's metadata, recent commits, releases and
// open issue/PR counts. It never mutates the database; the caller decides
// what to persist based on Status/RateLimited.
func (c *Client) Sync(ctx context.Context, owner, repo string) (*SyncResult, error) {
	res := &SyncResult{Status: "synced"}
	fullName := owner + "/" + repo

	// 1. Repository metadata.
	body, _, err := c.do(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo), nil)
	if err != nil {
		if _, ok := err.(errRateLimited); ok {
			res.Status = "rate_limited"
			res.RateLimited = true
			res.Error = err.Error()
			return res, nil
		}
		res.Status = "failed"
		if se, ok := err.(statusError); ok && se.code == http.StatusNotFound {
			if c.Token == "" {
				res.Error = "仓库可能是不存在或私有仓库需要 token：未登录时 GitHub 对不存在和私有仓库都返回 404，请配置 GITHUB_TOKEN 后重试"
			} else {
				res.Error = "仓库不存在（GitHub 返回 404）"
			}
		} else {
			res.Error = err.Error()
		}
		return res, nil
	}
	var gh struct {
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name          string  `json:"name"`
		Description   *string `json:"description"`
		Language      *string `json:"language"`
		Stars         int     `json:"stargazers_count"`
		Forks         int     `json:"forks_count"`
		HTMLURL       string  `json:"html_url"`
		Homepage      *string `json:"homepage"`
		PushedAt      *string `json:"pushed_at"`
		RepoUpdatedAt *string `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &gh); err != nil {
		res.Status = "failed"
		res.Error = "cannot parse repository response: " + err.Error()
		return res, nil
	}
	pushedAt, err := formatGitHubTime(gh.PushedAt)
	if err != nil {
		res.Status = "failed"
		res.Error = "cannot parse pushed_at: " + err.Error()
		return res, nil
	}
	repoUpdatedAt, err := formatGitHubTime(gh.RepoUpdatedAt)
	if err != nil {
		res.Status = "failed"
		res.Error = "cannot parse updated_at: " + err.Error()
		return res, nil
	}
	r := model.Repo{
		FullName:      gh.FullName,
		Owner:         gh.Owner.Login,
		Name:          gh.Name,
		Stars:         gh.Stars,
		Forks:         gh.Forks,
		HTMLURL:       gh.HTMLURL,
		PushedAt:      pushedAt,
		RepoUpdatedAt: repoUpdatedAt,
	}
	if r.FullName == "" {
		r.FullName = fullName
	}
	if gh.Description != nil {
		r.Description = *gh.Description
	}
	if gh.Language != nil {
		r.Language = *gh.Language
	}
	if gh.Homepage != nil {
		r.Homepage = *gh.Homepage
	}
	res.Repo = r

	// 2. Recent commits (10).
	body, _, err = c.do(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/commits",
		url.Values{"per_page": {"10"}})
	if err != nil {
		if _, ok := err.(errRateLimited); ok {
			res.Status = "rate_limited"
			res.RateLimited = true
			res.Error = err.Error()
			return res, nil
		}
		// Commits failing does not fail the whole sync; metadata still applies.
		res.Status = "partial"
		res.Error = "commits fetch failed: " + err.Error()
	} else {
		commits, perr := parseCommits(body)
		if perr != nil {
			res.Status = "partial"
			res.Error = "cannot parse commits response: " + perr.Error()
		} else {
			res.Commits = commits
		}
	}

	// 3. Releases, paginated (per_page=100, up to 100 total).
	releases, err := c.fetchAllReleases(ctx, owner, repo)
	if err != nil {
		if _, ok := err.(errRateLimited); ok {
			res.Status = "rate_limited"
			res.RateLimited = true
			res.Error = err.Error()
			return res, nil
		}
		markPartial(res, "releases fetch failed: "+err.Error())
	} else {
		res.Releases = releases
	}

	// 4+5. Open issue / PR counts via search (total_count only).
	issues, err := c.searchCount(ctx, owner, repo, "issue")
	if err != nil {
		if _, ok := err.(errRateLimited); ok {
			res.Status = "rate_limited"
			res.RateLimited = true
			res.Error = err.Error()
			return res, nil
		}
		markPartial(res, "open-issue search failed, open_issues/open_prs left unknown: "+err.Error())
	} else {
		res.Repo.OpenIssues = &issues
	}
	prs, err := c.searchCount(ctx, owner, repo, "pr")
	if err != nil {
		if _, ok := err.(errRateLimited); ok {
			res.Status = "rate_limited"
			res.RateLimited = true
			res.Error = err.Error()
			return res, nil
		}
		// If the issue search succeeded but the PR search failed, keep the
		// issue count and null only the PR count.
		res.Repo.OpenPRs = nil
		markPartial(res, "open-PR search failed, open_prs left unknown: "+err.Error())
	} else {
		res.Repo.OpenPRs = &prs
	}

	// 6. README (raw markdown). Never fails the whole sync: a missing
	// README (404) is silently skipped, any other failure only marks the
	// result partial and keeps everything fetched above.
	intro, steps, rerr := c.fetchReadme(ctx, owner, repo)
	if rerr != nil {
		if _, ok := rerr.(errRateLimited); ok {
			res.Status = "rate_limited"
			res.RateLimited = true
			res.Error = rerr.Error()
			return res, nil
		}
		if se, ok := rerr.(statusError); ok && se.code == http.StatusNotFound {
			// No README — nothing to parse.
		} else {
			markPartial(res, "readme fetch failed: "+rerr.Error())
		}
	} else {
		res.Repo.ReadmeIntro = intro
		res.Repo.TutorialSteps = steps
	}
	return res, nil
}

// fetchAllReleases pages through /releases (per_page=100) until a page
// returns fewer than 100 items, capping the total at 100 releases.
func (c *Client) fetchAllReleases(ctx context.Context, owner, repo string) ([]model.RepoRelease, error) {
	const perPage = 100
	const maxReleases = 100
	out := []model.RepoRelease{}
	for page := 1; ; page++ {
		var raw []rawRelease
		_, err := c.doDecode(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/releases",
			url.Values{"per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}}, &raw)
		if err != nil {
			return nil, err
		}
		pageReleases, perr := toRepoReleases(raw)
		if perr != nil {
			return nil, fmt.Errorf("cannot parse releases response: %w", perr)
		}
		out = append(out, pageReleases...)
		if len(out) >= maxReleases {
			return out[:maxReleases], nil
		}
		if len(pageReleases) < perPage {
			return out, nil
		}
	}
}

// fetchReadme fetches the repository README as raw markdown and parses it
// into an intro paragraph and tutorial steps. A 404 surfaces as a
// statusError so the caller can silently skip a repo without a README.
func (c *Client) fetchReadme(ctx context.Context, owner, repo string) (string, []model.TutorialStep, error) {
	body, _, err := c.doWithAccept(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/readme",
		nil, "application/vnd.github.raw")
	if err != nil {
		return "", nil, err
	}
	intro, steps := ParseReadme(string(body))
	return intro, steps, nil
}

func markPartial(res *SyncResult, msg string) {
	if res.Status != "partial" {
		res.Status = "partial"
		res.Error = msg
	} else {
		res.Error = res.Error + "; " + msg
	}
}

func (c *Client) searchCount(ctx context.Context, owner, repo, typ string) (int, error) {
	// Spaces separate search qualifiers; url.Values.Encode turns them
	// into '+' in the query string, which GitHub decodes back to spaces.
	// (A literal '+' inside q is NOT a separator and makes GitHub 422.)
	q := fmt.Sprintf("repo:%s/%s type:%s state:open", owner, repo, typ)
	body, _, err := c.do(ctx, "/search/issues", url.Values{"q": {q}, "per_page": {"1"}})
	if err != nil {
		return 0, err
	}
	return parseSearchTotal(body)
}

func parseCommits(body []byte) ([]model.RepoCommit, error) {
	var raw []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := []model.RepoCommit{}
	for _, c := range raw {
		committed := c.Commit.Author.Date
		if t, err := time.Parse(time.RFC3339, committed); err == nil {
			committed = t.In(shanghai).Format("2006-01-02 15:04:05")
		}
		login := ""
		if c.Author != nil {
			login = c.Author.Login
		}
		out = append(out, model.RepoCommit{
			SHA:         c.SHA,
			Message:     c.Commit.Message,
			AuthorName:  c.Commit.Author.Name,
			AuthorLogin: login,
			CommittedAt: committed,
			HTMLURL:     c.HTMLURL,
		})
	}
	return out, nil
}

// rawRelease is the GitHub release payload shape we read. Only these
// fields are decoded; assets and other large sub-objects are skipped.
type rawRelease struct {
	TagName     string  `json:"tag_name"`
	Name        *string `json:"name"`
	PublishedAt *string `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	Body        string  `json:"body"`
}

func parseReleases(body []byte) ([]model.RepoRelease, error) {
	var raw []rawRelease
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return toRepoReleases(raw)
}

func toRepoReleases(raw []rawRelease) ([]model.RepoRelease, error) {
	out := []model.RepoRelease{}
	for _, r := range raw {
		pub, err := formatGitHubTime(r.PublishedAt)
		if err != nil {
			return nil, err
		}
		name := ""
		if r.Name != nil {
			name = *r.Name
		}
		out = append(out, model.RepoRelease{
			TagName:     r.TagName,
			Name:        name,
			PublishedAt: pub,
			HTMLURL:     r.HTMLURL,
			Draft:       r.Draft,
			Prerelease:  r.Prerelease,
			Body:        r.Body,
		})
	}
	return out, nil
}
