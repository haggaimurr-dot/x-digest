// API shapes — exactly as in ~/workspace/xdigest-new/SPEC.md. Do not invent fields.
export interface Step {
  text: string
  tools?: string
  tech?: string
  prep?: string
  substeps?: Array<{ text: string, source?: string }>
}
export interface Post {
  id: string
  url: string
  author: string
  author_name: string
  author_avatar?: string
  industry?: string
  tech?: string
  title: string
  text?: string
  links?: string[]
  summary?: string
  note?: string
  published_at: string
  is_long?: boolean
  show_flowchart?: boolean | null
  tags?: string[]
  steps?: Step[]
  // AI post type from cmd/digest ("" = not classified) and link explanations.
  kind?: string
  resources?: Array<{ url: string, name: string, note: string }>
}
export interface WatchlistGroup { id: string, name: string, fixed?: boolean }
export interface WatchlistAccount {
  username: string, name: string, avatar?: string, group: string, tags?: string[], bio?: string
}
export interface Watchlist { groups: WatchlistGroup[], accounts: WatchlistAccount[] }
export interface Category { id: string, name: string }
export interface Categories { categories: Category[], assignments: Record<string, string[]> }
export interface Attachment {
  id: string, replica_id: string, step_idx: number, name: string, size: number,
  content_type?: string, ext?: string, created_at?: string,
  download_url: string
}
export interface ReplicaStep extends Step {
  idx: number, status: 'idle' | 'active' | 'done', note?: string, is_custom?: boolean, parent_idx?: number | null, attachments?: Attachment[]
}
export interface Replica {
  id: string, post_id: string, post_url: string, post_author: string,
  post_author_name: string, post_title: string, created_at: string, steps: ReplicaStep[]
}
export class ApiError extends Error {
  code: string
  status: number
  constructor (status: number, code: string, message: string) {
    super(message); this.status = status; this.code = code
  }
}

// ---------- GitHub repos section (contract 2026-10-25; backend in parallel) ----------
export type RepoSyncStatus = 'never' | 'synced' | 'rate_limited' | 'failed' | 'partial'
export interface RepoGroup { id: string, name: string, fixed?: boolean }
export interface RepoCommit {
  sha: string, message: string, author_name: string, author_login?: string,
  date: string, html_url: string
}
export interface RepoRelease {
  tag_name: string, name?: string, published_at?: string, html_url: string,
  body?: string, draft?: boolean, prerelease?: boolean
}
export interface TutorialStep { text: string }
export interface Repo {
  full_name: string, owner: string, name: string, description?: string | null,
  language?: string | null, stars: number, forks: number,
  open_issues?: number | null, open_prs?: number | null,
  pushed_at?: string | null, updated_at?: string | null, html_url: string,
  homepage?: string | null, group_id?: string | null, tags?: string[], note?: string | null,
  readme_intro?: string | null, tutorial_steps?: TutorialStep[],
  last_synced_at?: string | null, sync_status: RepoSyncStatus, sync_error?: string | null
}
export interface RepoDetail {
  repo: Repo, commits: RepoCommit[], latest_release?: RepoRelease | null,
  releases?: RepoRelease[]
}
export interface YTChannel {
  id: string, channel_id: string, handle?: string | null, name: string,
  group_id?: string | null, created_at: string,
  last_synced_at?: string | null, sync_status: string, sync_error?: string | null,
  video_count: number,
  sync_mode?: 'full' | 'single',
  date_from?: string | null, date_to?: string | null
}
export interface YTVideo {
  id: string, channel_id: string, channel_name?: string | null, title: string,
  description?: string | null, published_at: string, thumbnail_url?: string | null,
  url: string, created_at: string,
  kind?: 'video' | 'short' | 'live' | 'playlist', duration?: number, live_status?: string
}
// AI summary of one video in one language (GET/POST /api/v1/yt/videos/{id}/summary).
export interface YTSummaryChapter { start: number, title: string, summary: string }
export interface YTSummaryResource { name: string, url: string, note: string }
export interface YTSummary {
  video_id: string, lang: string, status: 'none' | 'pending' | 'done' | 'failed',
  summary: string, key_points: string[], chapters: YTSummaryChapter[], resources?: YTSummaryResource[],
  source: '' | 'transcript' | 'description', transcript_lang: string, model: string,
  error: string, created_at?: string, updated_at?: string
}

export interface XRefreshAccount {
  username: string
  state: 'pending' | 'running' | 'done' | 'error'
  since?: string
  found: number
  added: number
  skipped: number
  failed: number
  error?: string
}
export interface XRefreshJob {
  running: boolean
  configured: boolean
  started_at?: string
  finished_at?: string
  total?: number
  done?: number
  added?: number
  accounts: XRefreshAccount[]
}
