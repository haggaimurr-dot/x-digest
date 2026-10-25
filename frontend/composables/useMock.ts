// Dev-only mock/fallback data layer (default OFF, NUXT_PUBLIC_USE_MOCK=1).
// Source: original static data (data/posts.json, config/*.json) normalised to
// the SPEC API shapes. Mutations in mock mode are kept in memory + the same
// localStorage keys the original site used; they never touch a real backend.
import rawPosts from '~/assets/mock/posts.json'
import rawWatchlist from '~/assets/mock/watchlist.json'
import rawUserTags from '~/assets/mock/user-tags.json'
import type { Post, Watchlist, Categories, Replica, ReplicaStep, Attachment, Repo, RepoGroup, RepoCommit, RepoRelease } from './types'
import { LS } from './useSettings'

function normPost (p: any): Post {
  return {
    id: p.id, url: p.url, author: p.author, author_name: p.authorName,
    author_avatar: p.authorAvatar, industry: p.industry, tech: p.tech,
    title: p.title, summary: p.summary, note: p.note, published_at: p.publishedAt,
    is_long: p.isLong, show_flowchart: p.showFlowchart, tags: p.tags || [],
    steps: (p.steps || []).map((s: any) => ({
      text: s.text, tools: s.tools, tech: s.tech, prep: s.prep,
      substeps: (s.substeps || []).map((x: any) => typeof x === 'string' ? { text: x } : x)
    }))
  }
}
export const MOCK_POSTS: Post[] = (rawPosts as any).posts.map(normPost)
export const MOCK_UPDATED_AT: string = (rawPosts as any).updatedAt
export const MOCK_WATCHLIST: Watchlist = rawWatchlist as unknown as Watchlist
export const MOCK_USER_TAG_VOCAB: string[] = (rawUserTags as any).tags || []

// ---------- GitHub repos mock (dev-only; shapes follow the 2026-10-25 contract) ----------
export const MOCK_REPO_GROUPS: RepoGroup[] = [
  { id: 'default', name: '默认', fixed: true },
  { id: 'ai-tools', name: 'AI 工具' }
]
const MOCK_REPO_COMMITS: Record<string, RepoCommit[]> = {
  'nuxt/nuxt': [
    { sha: 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2', message: 'fix(nuxt): resolve route rules merging', author_name: 'Evan You', author_login: 'yyx990803', date: '2026-10-24T09:12:00Z', html_url: 'https://github.com/nuxt/nuxt/commit/a1b2c3d' },
    { sha: 'b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4', message: 'feat: add experimental view transitions support', author_name: 'Daniel Roe', author_login: 'danielroe', date: '2026-10-23T16:40:00Z', html_url: 'https://github.com/nuxt/nuxt/commit/b2c3d4e' },
    { sha: 'c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6', message: 'docs: update deployment guide', author_name: 'Pooya Parsa', author_login: 'pi0', date: '2026-10-22T11:05:00Z', html_url: 'https://github.com/nuxt/nuxt/commit/c3d4e5f' }
  ],
  'vuejs/core': [
    { sha: 'd4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1', message: 'fix(compiler): handle edge case in v-model', author_name: 'Evan You', author_login: 'yyx990803', date: '2026-10-24T07:30:00Z', html_url: 'https://github.com/vuejs/core/commit/d4e5f6a' },
    { sha: 'e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2', message: 'chore: bump deps', author_name: 'Evan You', author_login: 'yyx990803', date: '2026-10-23T02:15:00Z', html_url: 'https://github.com/vuejs/core/commit/e5f6a1b' }
  ]
}
const MOCK_REPO_RELEASES: Record<string, RepoRelease | null> = {
  'nuxt/nuxt': { tag_name: 'v4.6.2', name: 'v4.6.2', published_at: '2026-10-20T10:00:00Z', html_url: 'https://github.com/nuxt/nuxt/releases/tag/v4.6.2' },
  'vuejs/core': null
}
export const MOCK_REPOS: Repo[] = [
  {
    full_name: 'nuxt/nuxt', owner: 'nuxt', name: 'nuxt',
    description: 'The Intuitive Vue Framework',
    language: 'TypeScript', stars: 57800, forks: 6200, open_issues: 412, open_prs: 86,
    pushed_at: '2026-10-24T09:12:00Z', updated_at: '2026-10-24T09:12:00Z',
    html_url: 'https://github.com/nuxt/nuxt', homepage: 'https://nuxt.com',
    group_id: 'ai-tools', tags: ['框架', 'Vue'], note: '本站前端用的框架，关注大版本更新。',
    last_synced_at: '2026-10-25T02:00:00Z', sync_status: 'synced', sync_error: null
  },
  {
    full_name: 'vuejs/core', owner: 'vuejs', name: 'core',
    description: 'Vue.js is a progressive, incrementally-adoptable JavaScript framework for building UI on the web.',
    language: 'TypeScript', stars: 49200, forks: 8100, open_issues: null, open_prs: null,
    pushed_at: '2026-10-24T07:30:00Z', updated_at: '2026-10-24T07:30:00Z',
    html_url: 'https://github.com/vuejs/core', homepage: 'https://vuejs.org',
    group_id: 'default', tags: ['框架'], note: '',
    last_synced_at: '2026-10-24T02:00:00Z', sync_status: 'rate_limited', sync_error: 'GitHub API rate limit exceeded'
  }
]
export function mockRepoCommits (fullName: string): RepoCommit[] { return MOCK_REPO_COMMITS[fullName] || [] }
export function mockRepoRelease (fullName: string): RepoRelease | null { return MOCK_REPO_RELEASES[fullName] ?? null }

let seq = 1
export function useMockStore () {
  const watchlist = useState<Watchlist>('mock-watchlist', () => JSON.parse(JSON.stringify(MOCK_WATCHLIST)))
  const categories = useState<Categories>('mock-categories', () => {
    // Local assignments from the original key, if any; else just 收藏.
    let assignments: Record<string, string[]> = {}
    if (import.meta.client) {
      try { assignments = JSON.parse(localStorage.getItem(LS.postCategories) || '{}').assignments || {} } catch { assignments = {} }
    }
    return { categories: [{ id: 'fav', name: '收藏' }], assignments }
  })
  const userTagOverrides = useState<Record<string, string[]>>('mock-user-tag-overrides', () => {
    const o: Record<string, string[]> = {}
    for (const a of MOCK_WATCHLIST.accounts) o[a.username] = [...(a.tags || [])]
    return o
  })
  const vocabulary = useState<string[]>('mock-user-tag-vocab', () => [...MOCK_USER_TAG_VOCAB])
  const replicas = useState<Replica[]>('mock-replicas', () => {
    if (import.meta.client) {
      try { return JSON.parse(localStorage.getItem(LS.replicas) || '[]') } catch { return [] }
    }
    return []
  })
  function persist () {
    if (!import.meta.client) return
    try {
      localStorage.setItem(LS.replicas, JSON.stringify(replicas.value))
      localStorage.setItem(LS.postCategories, JSON.stringify(categories.value))
    } catch { /* ignore */ }
  }
  function createReplica (post: Post): { replica: Replica, existing: boolean } {
    const found = replicas.value.find(r => r.post_id === post.id)
    if (found) return { replica: found, existing: true }
    const replica: Replica = {
      id: `r-mock-${Date.now().toString(36)}${(seq++).toString(36)}`,
      post_id: post.id, post_url: post.url, post_author: post.author,
      post_author_name: post.author_name, post_title: post.title,
      created_at: new Date().toISOString(),
      steps: (post.steps || []).map((s, idx) => ({
        ...s, idx, status: 'idle' as const, note: '', attachments: []
      }))
    }
    replicas.value = [replica, ...replicas.value]
    persist()
    return { replica, existing: false }
  }
  function patchReplicaSteps (id: string, steps: Array<{ idx: number, status?: string, note?: string, text?: string }>) {
    const r = replicas.value.find(x => x.id === id)
    if (!r) return
    for (const patch of steps) {
      const s = r.steps.find(x => x.idx === patch.idx)
      if (!s) continue
      if (patch.status && ['idle', 'active', 'done'].includes(patch.status)) s.status = patch.status as any
      if (patch.note !== undefined) s.note = patch.note
      if (patch.text !== undefined && s.is_custom) s.text = patch.text
    }
    persist()
  }
  function addMockReplicaStep (id: string, text: string): ReplicaStep {
    const r = replicas.value.find(x => x.id === id)
    if (!r) throw new Error('复刻不存在')
    const idx = r.steps.reduce((m, s) => Math.max(m, s.idx), -1) + 1
    const step: ReplicaStep = { text: text.trim(), idx, status: 'idle', note: '', is_custom: true, attachments: [] }
    r.steps = [...r.steps, step]
    persist()
    return step
  }
  function deleteMockReplicaStep (id: string, idx: number) {
    const r = replicas.value.find(x => x.id === id)
    if (!r) return
    r.steps = r.steps.filter(s => !(s.idx === idx && s.is_custom))
    persist()
  }
  function deleteReplica (id: string) {
    replicas.value = replicas.value.filter(r => r.id !== id)
    persist()
  }
  function addMockAttachments (replicaId: string, stepIdx: number, files: File[]): Attachment[] {
    const r = replicas.value.find(x => x.id === replicaId)
    if (!r) return []
    const step = r.steps.find(s => s.idx === stepIdx)
    if (!step) return []
    const made: Attachment[] = files.map((f) => {
      const ext = f.name.split('.').pop()?.toLowerCase() || ''
      const att: Attachment = {
        id: `att-mock-${Date.now().toString(36)}${(seq++).toString(36)}`,
        replica_id: replicaId, step_idx: stepIdx, name: f.name, size: f.size,
        content_type: f.type, ext, created_at: new Date().toISOString(),
        download_url: URL.createObjectURL(f)
      }
      // Keep the blob reachable for preview: stash on the object (mock only).
      ;(att as any).__blob = f
      return att
    })
    step.attachments = [...(step.attachments || []), ...made]
    persist()
    return made
  }
  function deleteMockAttachment (id: string) {
    for (const r of replicas.value) for (const s of r.steps) s.attachments = (s.attachments || []).filter(a => a.id !== id)
    persist()
  }
  // ---------- GitHub repos mock state ----------
  const repoGroups = useState<RepoGroup[]>('mock-repo-groups', () => JSON.parse(JSON.stringify(MOCK_REPO_GROUPS)))
  const repos = useState<Repo[]>('mock-repos', () => JSON.parse(JSON.stringify(MOCK_REPOS)))
  function persistRepos () {
    if (!import.meta.client) return
    try {
      localStorage.setItem('xd-mock-repos', JSON.stringify(repos.value))
      localStorage.setItem('xd-mock-repo-groups', JSON.stringify(repoGroups.value))
    } catch { /* ignore */ }
  }
  function addMockRepo (fullName: string, groupId?: string | null, tags?: string[], note?: string): Repo {
    const [owner, name] = fullName.split('/')
    if (repos.value.some(r => r.full_name.toLowerCase() === fullName.toLowerCase())) {
      const e = new Error('该仓库已存在') as any
      e.status = 409; throw e
    }
    const repo: Repo = {
      full_name: fullName, owner, name, description: '（Mock 数据）示例仓库描述。',
      language: 'TypeScript', stars: 0, forks: 0, open_issues: null, open_prs: null,
      pushed_at: null, updated_at: new Date().toISOString(), html_url: `https://github.com/${fullName}`,
      homepage: null, group_id: groupId || 'default', tags: tags || [], note: note || '',
      last_synced_at: new Date().toISOString(), sync_status: 'synced', sync_error: null
    }
    repos.value = [repo, ...repos.value]
    persistRepos()
    return repo
  }
  function patchMockRepo (fullName: string, patch: { group_id?: string | null, tags?: string[], note?: string }): Repo | null {
    const r = repos.value.find(x => x.full_name.toLowerCase() === fullName.toLowerCase())
    if (!r) return null
    Object.assign(r, patch)
    repos.value = [...repos.value]
    persistRepos()
    return r
  }
  function deleteMockRepo (fullName: string) {
    repos.value = repos.value.filter(r => r.full_name.toLowerCase() !== fullName.toLowerCase())
    persistRepos()
  }
  function syncMockRepo (fullName: string): Repo | null {
    const r = repos.value.find(x => x.full_name.toLowerCase() === fullName.toLowerCase())
    if (!r) return null
    r.last_synced_at = new Date().toISOString()
    r.sync_status = 'synced'; r.sync_error = null
    repos.value = [...repos.value]
    persistRepos()
    return r
  }
  function addMockRepoGroup (name: string): RepoGroup {
    const g: RepoGroup = { id: `rg-${Date.now().toString(36)}`, name }
    repoGroups.value = [...repoGroups.value, g]
    persistRepos()
    return g
  }
  function deleteMockRepoGroup (id: string) {
    repoGroups.value = repoGroups.value.filter(g => g.id !== id)
    repos.value = repos.value.map(r => r.group_id === id ? { ...r, group_id: 'default' } : r)
    persistRepos()
  }
  return { watchlist, categories, userTagOverrides, vocabulary, replicas, persist, createReplica, patchReplicaSteps, deleteReplica, addMockReplicaStep, deleteMockReplicaStep, addMockAttachments, deleteMockAttachment, repoGroups, repos, addMockRepo, patchMockRepo, deleteMockRepo, syncMockRepo, addMockRepoGroup, deleteMockRepoGroup }
}
