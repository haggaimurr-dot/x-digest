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
	// Original post text and its links (from raw_posts.json at import).
	Text  string   `json:"text"`
	Links []string `json:"links"`
	// Kind is the AI post type (教程实操 / 技术分享 / 工具资源推荐 / 观点讨论 /
	// 日常闲聊; "" = not classified). Resources explain each link.
	Kind      string         `json:"kind"`
	Resources []PostResource `json:"resources"`
}

// PostResource is one link of a post with what it is (from cmd/digest).
type PostResource struct {
	URL  string `json:"url"`
	Name string `json:"name"`
	Note string `json:"note"`
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
	IsCustom    bool         `json:"is_custom,omitempty"`
	ParentIdx   *int         `json:"parent_idx,omitempty"`
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
	Body        string  `json:"body"`
}

// TutorialStep is one step extracted from a repository README's
// install / quick-start section.
type TutorialStep struct {
	Text string `json:"text"`
}

// Repo mirrors the repos table. Nullable columns surface as nil pointers so
// callers can tell "never synced / unknown" (null) apart from zero.
// PushedAt / RepoUpdatedAt / LastSyncedAt are Shanghai wall time
// ("2006-01-02 15:04:05"), matching the DATETIME columns.
type Repo struct {
	FullName      string         `json:"full_name"`
	Owner         string         `json:"owner"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	ReadmeIntro   string         `json:"readme_intro"`
	TutorialSteps []TutorialStep `json:"tutorial_steps,omitempty"`
	Language      string         `json:"language"`
	Stars         int            `json:"stars"`
	Forks         int            `json:"forks"`
	OpenIssues    *int           `json:"open_issues"`
	OpenPRs       *int           `json:"open_prs"`
	HTMLURL       string         `json:"html_url"`
	Homepage      string         `json:"homepage"`
	PushedAt      *string        `json:"pushed_at"`
	RepoUpdatedAt *string        `json:"repo_updated_at"`
	GroupID       string         `json:"group_id"`
	Note          string         `json:"note"`
	LastSyncedAt  *string        `json:"last_synced_at"`
	SyncStatus    string         `json:"sync_status"`
	SyncError     string         `json:"sync_error"`
	Tags          []string       `json:"tags"`
	Commits       []RepoCommit   `json:"commits,omitempty"`
	LatestRelease *RepoRelease   `json:"latest_release,omitempty"`
}

// YTChannel mirrors the yt_channels table (YouTube prototype 2026-10-26).
// ChannelID is the UC... ID (unique); Handle is the @handle when known.
// PublishedAt / LastSyncedAt are Shanghai wall time ("2006-01-02 15:04:05").
// DateFrom/DateTo are the per-channel pull range ("2006-01-02", null =
// no limit); the sync only stores videos/shorts/lives whose upload date
// falls inside it — playlists are never range-filtered.
// VideoCount is computed at read time, not stored.
type YTChannel struct {
	ID           string  `json:"id"`
	ChannelID    string  `json:"channel_id"`
	Handle       string  `json:"handle"`
	Name         string  `json:"name"`
	GroupID      string  `json:"group_id"`
	DateFrom     *string `json:"date_from"`
	DateTo       *string `json:"date_to"`
	CreatedAt    string  `json:"created_at"`
	LastSyncedAt *string `json:"last_synced_at"`
	SyncStatus   string  `json:"sync_status"`
	SyncError    string  `json:"sync_error"`
	VideoCount   int     `json:"video_count"`
}

// YTVideo mirrors the yt_videos table. ID is the YouTube video_id
// (e.g. dQw4w9WgXcQ) or, for playlists, the playlist ID (PL...).
// ChannelName is joined at read time for display. Kind is one of
// video/short/live/playlist. Duration is seconds (0 = unknown).
// PublishedAt is "" when unknown (playlists never have one; a video
// only gets one once the yt-dlp metadata backfill supplies it).
type YTVideo struct {
	ID           string `json:"id"`
	ChannelID    string `json:"channel_id"`
	ChannelName  string `json:"channel_name"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	PublishedAt  string `json:"published_at"`
	Duration     int    `json:"duration"`
	LiveStatus   string `json:"live_status"`
	ThumbnailURL string `json:"thumbnail_url"`
	URL          string `json:"url"`
	CreatedAt    string `json:"created_at"`
}

// YTSummaryChapter is one AI summary chapter (Start in whole seconds).
type YTSummaryChapter struct {
	Start   int    `json:"start"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// YTSummaryResource is something the video mentions (URL may be "").
type YTSummaryResource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Note string `json:"note"`
}

// YTSummary mirrors yt_video_summaries: the AI summary of one video in
// one language. Status is none (no row yet; never stored), pending, done
// or failed. Source is transcript or description (no subtitles found).
type YTSummary struct {
	VideoID        string              `json:"video_id"`
	Lang           string              `json:"lang"`
	Status         string              `json:"status"`
	Summary        string              `json:"summary"`
	KeyPoints      []string            `json:"key_points"`
	Chapters       []YTSummaryChapter  `json:"chapters"`
	Resources      []YTSummaryResource `json:"resources"`
	Source         string              `json:"source"`
	TranscriptLang string              `json:"transcript_lang"`
	Model          string              `json:"model"`
	Error          string              `json:"error"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

// YTComment is one of a video's most-liked top-level comments.
// PublishedAt is Shanghai wall time, "" when unknown.
type YTComment struct {
	Author      string `json:"author"`
	Text        string `json:"text"`
	Likes       int    `json:"likes"`
	PublishedAt string `json:"published_at"`
}
