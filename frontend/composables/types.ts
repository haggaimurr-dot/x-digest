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
  summary?: string
  note?: string
  published_at: string
  is_long?: boolean
  show_flowchart?: boolean | null
  tags?: string[]
  steps?: Step[]
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
  idx: number, status: 'idle' | 'active' | 'done', note?: string, attachments?: Attachment[]
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
