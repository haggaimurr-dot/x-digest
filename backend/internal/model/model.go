// Package model defines the JSON shapes of the x-digest API (see ../SPEC.md).
// Times are rendered as "2006-01-02 15:04:05" Asia/Shanghai wall time, matching
// the DATETIME columns in schema.sql. published_at in the legacy JSON carried a
// +08:00 offset and is stored converted to Shanghai wall time.
package model

type Substep struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

type PostStep struct {
	Text     string    `json:"text"`
	Tools    string    `json:"tools,omitempty"`
	Tech     string    `json:"tech,omitempty"`
	Prep     string    `json:"prep,omitempty"`
	Substeps []Substep `json:"substeps,omitempty"`
}

type Post struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	Author        string     `json:"author"`
	AuthorName    string     `json:"author_name"`
	AuthorAvatar  string     `json:"author_avatar"`
	Industry      string     `json:"industry"`
	Tech          string     `json:"tech"`
	Title         string     `json:"title"`
	Summary       string     `json:"summary"`
	Note          string     `json:"note"`
	PublishedAt   string     `json:"published_at"`
	IsLong        bool       `json:"is_long"`
	ShowFlowchart *bool      `json:"show_flowchart"`
	Tags          []string   `json:"tags"`
	Steps         []PostStep `json:"steps"`
}

type Group struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Fixed bool   `json:"fixed"`
}

type Account struct {
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Avatar   string   `json:"avatar"`
	Group    string   `json:"group"`
	Tags     []string `json:"tags"`
	Bio      string   `json:"bio"`
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Attachment struct {
	ID          string `json:"id"`
	ReplicaID   string `json:"replica_id"`
	StepIdx     int    `json:"step_idx"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Ext         string `json:"ext"`
	CreatedAt   string `json:"created_at"`
	DownloadURL string `json:"download_url"`
}

type ReplicaStep struct {
	Idx         int          `json:"idx"`
	Text        string       `json:"text"`
	Tools       string       `json:"tools,omitempty"`
	Tech        string       `json:"tech,omitempty"`
	Prep        string       `json:"prep,omitempty"`
	Substeps    []Substep    `json:"substeps,omitempty"`
	Status      string       `json:"status"`
	Note        string       `json:"note"`
	Attachments []Attachment `json:"attachments"`
}

type ReplicaProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type Replica struct {
	ID             string          `json:"id"`
	PostID         string          `json:"post_id"`
	PostURL        string          `json:"post_url"`
	PostAuthor     string          `json:"post_author"`
	PostAuthorName string          `json:"post_author_name"`
	PostTitle      string          `json:"post_title"`
	CreatedAt      string          `json:"created_at"`
	Steps          []ReplicaStep   `json:"steps"`
	Progress       ReplicaProgress `json:"progress"`
	Existing       bool            `json:"existing,omitempty"`
}

// ---- GitHub repos section ----

type RepoCommit struct {
	SHA         string `json:"sha"`
	Message     string `json:"message"`
	AuthorName  string `json:"author_name"`
	AuthorLogin string `json:"author_login"`
	CommittedAt string `json:"committed_at"`
	HTMLURL     string `json:"html_url"`
}

type RepoRelease struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	PublishedAt *string `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
}

// Repo mirrors the repos table. Nullable columns surface as nil pointers so
// callers can tell "never synced / unknown" (null) apart from zero.
// PushedAt / RepoUpdatedAt / LastSyncedAt are Shanghai wall time
// ("2006-01-02 15:04:05"), matching the DATETIME columns.
type Repo struct {
	FullName      string       `json:"full_name"`
	Owner         string       `json:"owner"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Language      string       `json:"language"`
	Stars         int          `json:"stars"`
	Forks         int          `json:"forks"`
	OpenIssues    *int         `json:"open_issues"`
	OpenPRs       *int         `json:"open_prs"`
	HTMLURL       string       `json:"html_url"`
	Homepage      string       `json:"homepage"`
	PushedAt      *string      `json:"pushed_at"`
	RepoUpdatedAt *string      `json:"repo_updated_at"`
	GroupID       string       `json:"group_id"`
	Note          string       `json:"note"`
	LastSyncedAt  *string      `json:"last_synced_at"`
	SyncStatus    string       `json:"sync_status"`
	SyncError     string       `json:"sync_error"`
	Tags          []string     `json:"tags"`
	Commits       []RepoCommit `json:"commits,omitempty"`
	LatestRelease *RepoRelease `json:"latest_release,omitempty"`
}
