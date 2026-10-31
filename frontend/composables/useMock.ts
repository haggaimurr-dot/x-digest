// Dev-only mock/fallback data layer (default OFF, NUXT_PUBLIC_USE_MOCK=1).
// Source: original static data (data/posts.json, config/*.json) normalised to
// the SPEC API shapes. Mutations in mock mode are kept in memory + the same
// localStorage keys the original site used; they never touch a real backend.
import rawPosts from '~/assets/mock/posts.json'
import rawWatchlist from '~/assets/mock/watchlist.json'
import rawUserTags from '~/assets/mock/user-tags.json'
import type { Post, Watchlist, Categories, Replica, ReplicaStep, Attachment, Repo, RepoGroup, RepoCommit, RepoRelease, YTChannel, YTVideo, YTSummary } from './types'
import { LS } from './useSettings'
import { t } from '~/utils/i18n'

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
const MOCK_REPO_RELEASES: Record<string, RepoRelease[]> = {
  'nuxt/nuxt': [
    {
      tag_name: 'v4.6.0', name: 'v4.6.0', published_at: '2026-09-15T10:00:00Z',
      html_url: 'https://github.com/nuxt/nuxt/releases/tag/v4.6.0',
      body: '## 新功能\n- 新增实验性视图过渡支持\n- 路由规则合并逻辑优化\n\n## 修复\n- 修复开发服务器偶发热更新失效的问题'
    },
    {
      tag_name: 'v4.6.1', name: 'v4.6.1', published_at: '2026-10-05T14:30:00Z',
      html_url: 'https://github.com/nuxt/nuxt/releases/tag/v4.6.1',
      body: '## 修复\n- 修复 NuxtLink 在特定路由下的预取异常\n- 修复 useFetch 缓存键冲突\n\n完整变更见提交记录。'
    },
    {
      tag_name: 'v4.6.2', name: 'v4.6.2', published_at: '2026-10-20T10:00:00Z',
      html_url: 'https://github.com/nuxt/nuxt/releases/tag/v4.6.2',
      body: '## 修复\n- fix(nuxt): resolve route rules merging\n- 修复构建产物中多余的 sourcemap\n\n## 文档\n- 更新部署指南，增加容器部署示例'
    },
    {
      tag_name: 'v4.7.0-beta.1', name: 'v4.7.0-beta.1', published_at: '2026-10-24T09:00:00Z',
      html_url: 'https://github.com/nuxt/nuxt/releases/tag/v4.7.0-beta.1', prerelease: true,
      body: '## 预发布说明\n这是一个测试版本，请勿用于生产环境。\n\n- 试验新的 Islands 渲染管线\n- 已知问题：与旧版模块的兼容性待验证'
    },
    {
      tag_name: 'v4.7.0', name: 'v4.7.0',
      html_url: 'https://github.com/nuxt/nuxt/releases/tag/v4.7.0', draft: true,
      body: '（草稿）\n\n- 计划中的大版本更新说明，尚未定稿。'
    }
  ],
  'vuejs/core': []
}
export function mockRepoReleases (fullName: string): RepoRelease[] { return MOCK_REPO_RELEASES[fullName] || [] }
export function mockRepoRelease (fullName: string): RepoRelease | null {
  const published = mockRepoReleases(fullName).filter(r => !r.draft && r.published_at)
  if (!published.length) return null
  return [...published].sort((a, b) => Date.parse(b.published_at!) - Date.parse(a.published_at!))[0]
}
export const MOCK_REPOS: Repo[] = [
  {
    full_name: 'nuxt/nuxt', owner: 'nuxt', name: 'nuxt',
    description: 'The Intuitive Vue Framework',
    readme_intro: 'Nuxt 是基于 Vue 的直观框架，提供开箱即用的服务端渲染、自动路由与强大的模块生态。',
    tutorial_steps: [
      { text: '安装 Node.js 18 或更高版本，然后执行 npx nuxi@latest init my-app 创建项目' },
      { text: '进入项目目录并安装依赖：cd my-app && npm install' },
      { text: '启动开发服务器：npm run dev，在浏览器打开 http://localhost:3000 预览' },
      { text: '编辑 app.vue 开始开发，完成后执行 npm run build 构建生产版本' }
    ],
    language: 'TypeScript', stars: 57800, forks: 6200, open_issues: 412, open_prs: 86,
    pushed_at: '2026-10-24T09:12:00Z', updated_at: '2026-10-24T09:12:00Z',
    html_url: 'https://github.com/nuxt/nuxt', homepage: 'https://nuxt.com',
    group_id: 'ai-tools', tags: ['框架', 'Vue'], note: '本站前端用的框架，关注大版本更新。',
    last_synced_at: '2026-10-25T02:00:00Z', sync_status: 'synced', sync_error: null,
    created_at: '2026-10-20 10:00:00', released_at: '2026-10-22 18:30:00'
  },
  {
    full_name: 'vuejs/core', owner: 'vuejs', name: 'core',
    description: 'Vue.js is a progressive, incrementally-adoptable JavaScript framework for building UI on the web.',
    language: 'TypeScript', stars: 49200, forks: 8100, open_issues: null, open_prs: null,
    pushed_at: '2026-10-24T07:30:00Z', updated_at: '2026-10-24T07:30:00Z',
    html_url: 'https://github.com/vuejs/core', homepage: 'https://vuejs.org',
    group_id: 'default', tags: ['框架'], note: '',
    last_synced_at: '2026-10-24T02:00:00Z', sync_status: 'rate_limited', sync_error: 'GitHub API rate limit exceeded',
    created_at: '2026-10-22 09:00:00', released_at: null
  }
]
export function mockRepoCommits (fullName: string): RepoCommit[] { return MOCK_REPO_COMMITS[fullName] || [] }

// ---------- YouTube channels mock (dev-only; shapes follow the 2026-10-26 contract) ----------
export const MOCK_YT_GROUPS: RepoGroup[] = [
  { id: 'default', name: '默认', fixed: true },
  { id: 'starred', name: '特别关注', fixed: true },
  { id: 'dev', name: '开发' }
]
export const MOCK_YT_CHANNELS: YTChannel[] = [
  {
    id: 'ytc-mock-1', channel_id: 'UC_x5XG1OV2P6uZZ5FSM9Ttw', handle: '@GoogleDevelopers',
    name: 'Google for Developers', group_id: 'dev', created_at: '2026-10-25T02:00:00Z',
    last_synced_at: '2026-10-26T02:00:00Z', sync_status: 'synced', sync_error: null, video_count: 3,
    date_from: '2026-10-01', date_to: '2026-10-26'
  },
  {
    id: 'ytc-mock-2', channel_id: 'UCsBjURrPoezykLs9EqgamOA', handle: '@Fireship',
    name: 'Fireship', group_id: 'default', created_at: '2026-10-25T02:00:00Z',
    last_synced_at: '2026-10-26T02:00:00Z', sync_status: 'synced', sync_error: null, video_count: 2,
    date_from: null, date_to: null
  }
]
export const MOCK_YT_VIDEOS: YTVideo[] = [
  {
    id: 'dQw4w9WgXcQ', channel_id: 'UC_x5XG1OV2P6uZZ5FSM9Ttw', channel_name: 'Google for Developers',
    title: 'First video title', description: 'First video description here.',
    published_at: '2026-10-24 16:30:00', thumbnail_url: 'https://i4.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg',
    url: 'https://www.youtube.com/watch?v=dQw4w9WgXcQ', created_at: '2026-10-24T16:30:00Z',
    kind: 'video', duration: 612, live_status: ''
  },
  {
    id: 'abc123DEF45', channel_id: 'UC_x5XG1OV2P6uZZ5FSM9Ttw', channel_name: 'Google for Developers',
    title: 'Second video, no description', description: '',
    published_at: '2026-10-22 20:00:00', thumbnail_url: '',
    url: 'https://www.youtube.com/watch?v=abc123DEF45', created_at: '2026-10-22T20:00:00Z',
    kind: 'short', duration: 45, live_status: ''
  },
  {
    id: 'PLmocklist01', channel_id: 'UC_x5XG1OV2P6uZZ5FSM9Ttw', channel_name: 'Google for Developers',
    title: 'Android Development Playlist', description: 'A curated playlist of Android development talks and tutorials.',
    published_at: '', thumbnail_url: '',
    url: 'https://www.youtube.com/playlist?list=PLmocklist01', created_at: '2026-10-24T16:30:00Z',
    kind: 'playlist', duration: 0, live_status: ''
  },
  {
    id: 'mockfire001', channel_id: 'UCsBjURrPoezykLs9EqgamOA', channel_name: 'Fireship',
    title: '100 JavaScript concepts in 10 minutes', description: 'A fast tour of the JavaScript language, from variables to async iterators.',
    published_at: '2026-10-23 10:00:00', thumbnail_url: 'https://i4.ytimg.com/vi/mockfire001/hqdefault.jpg',
    url: 'https://www.youtube.com/watch?v=mockfire001', created_at: '2026-10-23T10:00:00Z',
    kind: 'video', duration: 600, live_status: ''
  },
  {
    id: 'mockfire002', channel_id: 'UCsBjURrPoezykLs9EqgamOA', channel_name: 'Fireship',
    title: 'CSS in 100 seconds', description: 'Everything you need to know about CSS, in 100 seconds.',
    published_at: '2026-10-21 09:00:00', thumbnail_url: 'https://i4.ytimg.com/vi/mockfire002/hqdefault.jpg',
    url: 'https://www.youtube.com/watch?v=mockfire002', created_at: '2026-10-21T09:00:00Z',
    kind: 'video', duration: 100, live_status: ''
  },
  {
    id: 'mockfirelive1', channel_id: 'UCsBjURrPoezykLs9EqgamOA', channel_name: 'Fireship',
    title: 'Live Q&A: ask us anything', description: 'Monthly live Q&A stream recording.',
    published_at: '', thumbnail_url: 'https://i4.ytimg.com/vi/mockfirelive1/hqdefault.jpg',
    url: 'https://www.youtube.com/watch?v=mockfirelive1', created_at: '2026-10-25T09:00:00Z',
    kind: 'live', duration: 0, live_status: 'was_live'
  }
]

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
  function addMockReplicaStep (id: string, text: string, parentIdx?: number | null): ReplicaStep {
    const r = replicas.value.find(x => x.id === id)
    if (!r) throw new Error(t('mock.replicaNotFound'))
    if (parentIdx != null) {
      const parent = r.steps.find(s => s.idx === parentIdx)
      if (!parent) throw new Error(t('mock.parentNotFound'))
      if (parent.parent_idx != null) throw new Error(t('mock.maxTwoLevels'))
    }
    const idx = r.steps.reduce((m, s) => Math.max(m, s.idx), -1) + 1
    const step: ReplicaStep = { text: text.trim(), idx, status: 'idle', note: '', is_custom: true, parent_idx: parentIdx ?? null, attachments: [] }
    r.steps = [...r.steps, step]
    persist()
    return step
  }
  function deleteMockReplicaStep (id: string, idx: number) {
    const r = replicas.value.find(x => x.id === id)
    if (!r) return
    const target = r.steps.find(s => s.idx === idx)
    if (!target || !target.is_custom) return
    r.steps = r.steps.filter(s => !(s.idx === idx || s.parent_idx === idx))
    persist()
  }
  function deleteReplica (id: string) {
    replicas.value = replicas.value.filter(r => r.id !== id)
    persist()
  }
  function createMockStandaloneReplica (title: string): { replica: Replica, existing: boolean } {
    const replica: Replica = {
      id: `r-mock-${Date.now().toString(36)}${(seq++).toString(36)}`,
      post_id: '', post_url: '', post_author: '', post_author_name: '自建',
      post_title: title.trim(), created_at: new Date().toISOString(), steps: []
    }
    replicas.value = [replica, ...replicas.value]
    persist()
    return { replica, existing: false }
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
  function renameMockAttachment (id: string, name: string) {
    for (const r of replicas.value) for (const s of r.steps) for (const a of (s.attachments || [])) if (a.id === id) { a.name = name; persist(); return a }
    throw new Error('attachment not found')
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
      const e = new Error(t('mock.repoExists')) as any
      e.status = 409; throw e
    }
    const repo: Repo = {
      full_name: fullName, owner, name, description: '（Mock 数据）示例仓库描述。',
      language: 'TypeScript', stars: 0, forks: 0, open_issues: null, open_prs: null,
      pushed_at: null, updated_at: new Date().toISOString(), html_url: `https://github.com/${fullName}`,
      homepage: null, group_id: groupId || 'default', tags: tags || [], note: note || '',
      last_synced_at: new Date().toISOString(), sync_status: 'synced', sync_error: null,
      created_at: new Date().toISOString(), released_at: null
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
  // ---------- YouTube channels mock state ----------
  const ytGroups = useState<RepoGroup[]>('mock-yt-groups', () => JSON.parse(JSON.stringify(MOCK_YT_GROUPS)))
  const ytChannels = useState<YTChannel[]>('mock-yt-channels', () => JSON.parse(JSON.stringify(MOCK_YT_CHANNELS)))
  const ytVideos = useState<YTVideo[]>('mock-yt-videos', () => JSON.parse(JSON.stringify(MOCK_YT_VIDEOS)))
  function persistYT () {
    if (!import.meta.client) return
    try {
      localStorage.setItem('xd-mock-yt-channels', JSON.stringify(ytChannels.value))
      localStorage.setItem('xd-mock-yt-groups', JSON.stringify(ytGroups.value))
    } catch { /* ignore */ }
  }
  function addMockYTChannel (input: string, groupId?: string | null, dateFrom?: string | null, dateTo?: string | null): YTChannel {
    const channelId = input.trim()
    if (ytChannels.value.some(c => c.channel_id === channelId)) {
      const e = new Error(t('mock.channelExists')) as any
      e.status = 409; throw e
    }
    const ch: YTChannel = {
      id: `ytc-mock-${Date.now().toString(36)}${(seq++).toString(36)}`,
      channel_id: channelId, handle: channelId.startsWith('@') ? channelId : '',
      name: channelId, group_id: groupId || 'default',
      created_at: new Date().toISOString(), last_synced_at: new Date().toISOString(),
      sync_status: 'synced', sync_error: null, video_count: 0,
      date_from: dateFrom || null, date_to: dateTo || null
    }
    ytChannels.value = [ch, ...ytChannels.value]
    persistYT()
    return ch
  }
  function patchMockYTChannel (id: string, patch: { group_id?: string | null, date_from?: string | null, date_to?: string | null }): YTChannel {
    const ch = ytChannels.value.find(c => c.id === id)
    if (!ch) throw new Error(t('mock.channelNotFound'))
    const updated: YTChannel = {
      ...ch,
      group_id: patch.group_id !== undefined ? patch.group_id : ch.group_id,
      date_from: patch.date_from !== undefined ? (patch.date_from || null) : ch.date_from,
      date_to: patch.date_to !== undefined ? (patch.date_to || null) : ch.date_to
    }
    ytChannels.value = ytChannels.value.map(c => c.id === id ? updated : c)
    persistYT()
    return updated
  }
  function deleteMockYTChannel (id: string) {
    const ch = ytChannels.value.find(c => c.id === id)
    ytChannels.value = ytChannels.value.filter(c => c.id !== id)
    if (ch) ytVideos.value = ytVideos.value.filter(v => v.channel_id !== ch.channel_id)
    persistYT()
  }
  function syncMockYTChannel (id: string): YTChannel | null {
    const ch = ytChannels.value.find(c => c.id === id)
    if (!ch) return null
    ch.last_synced_at = new Date().toISOString()
    ch.sync_status = 'synced'; ch.sync_error = null
    ytChannels.value = [...ytChannels.value]
    persistYT()
    return ch
  }
  function addMockYTGroup (name: string): RepoGroup {
    const g: RepoGroup = { id: `grp-${Date.now().toString(36)}`, name }
    ytGroups.value = [...ytGroups.value, g]
    persistYT()
    return g
  }
  function deleteMockYTGroup (id: string) {
    ytGroups.value = ytGroups.value.filter(g => g.id !== id)
    ytChannels.value = ytChannels.value.map(c => c.group_id === id ? { ...c, group_id: 'default' } : c)
    persistYT()
  }
  // Mock mode never calls the model: a canned summary shows the layout.
  function mockYTSummary (id: string, lang: string): YTSummary {
    const v = ytVideos.value.find(x => x.id === id)
    const zh = lang !== 'en'
    return {
      video_id: id, lang, status: 'done', source: 'transcript', transcript_lang: 'en', model: 'mock', error: '',
      summary: zh
        ? `（演示数据）「${v?.title || id}」的全文摘要会显示在这里。\n\n第二段：主要论点、步骤与结论。`
        : `(Demo data) The full summary of "${v?.title || id}" appears here.\n\nSecond paragraph: main points, steps and conclusion.`,
      key_points: zh ? ['要点一', '要点二', '要点三'] : ['Point one', 'Point two', 'Point three'],
      chapters: [
        { start: 0, title: zh ? '开场' : 'Intro', summary: zh ? '介绍本期内容。' : 'What this video covers.' },
        { start: 95, title: zh ? '正文' : 'Main part', summary: zh ? '核心演示。' : 'The core demo.' },
        { start: 300, title: zh ? '总结' : 'Wrap-up', summary: zh ? '结论与下一步。' : 'Conclusion and next steps.' }
      ]
    }
  }
  return { watchlist, categories, userTagOverrides, vocabulary, replicas, persist, mockYTSummary, createReplica, createMockStandaloneReplica, patchReplicaSteps, deleteReplica, addMockReplicaStep, deleteMockReplicaStep, addMockAttachments, deleteMockAttachment, renameMockAttachment, repoGroups, repos, addMockRepo, patchMockRepo, deleteMockRepo, syncMockRepo, addMockRepoGroup, deleteMockRepoGroup, ytGroups, ytChannels, ytVideos, addMockYTChannel, patchMockYTChannel, deleteMockYTChannel, syncMockYTChannel, addMockYTGroup, deleteMockYTGroup }
}
