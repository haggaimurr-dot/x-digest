import { rawAvatar } from '~/utils/x-post.mjs'
import xContent from '~/assets/mock/x-content.json'
// Data layer: real /api/v1 by default, mock fallback when NUXT_PUBLIC_USE_MOCK=1.
// Pages only ever talk to this layer, never to raw fetch or raw mock data.
import type { Post, Watchlist, Categories, Replica, ReplicaStep, Attachment, Repo, RepoGroup, RepoDetail, RepoRelease, RepoSyncJob, SavedView, YTChannel, YTVideo, YTSummary, XRefreshJob, XRefreshAccounts } from './types'
import { ApiError } from './types'
// Video platforms sharing the /api/v1/<p>/ routes and the VideoHub page.
export type VideoPlatform = 'yt' | 'bili'
import { t } from '~/utils/i18n'
import { MOCK_POSTS, MOCK_UPDATED_AT, mockRepoCommits, mockRepoRelease, mockRepoReleases } from './useMock'

export const ATTACHMENT_EXTS = 'pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif webp bmp avif svg ico'.split(' ')
export const IMAGE_EXTS = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'avif', 'svg', 'ico']
export const MAX_ATTACHMENT_SIZE = 5 * 1024 * 1024

export function validateAttachmentFiles (files: File[]): string | null {
  for (const f of files) {
    const ext = f.name.split('.').pop()?.toLowerCase() || ''
    if (!ATTACHMENT_EXTS.includes(ext)) return t('store.unsupportedFile', { name: f.name })
    if (f.size > MAX_ATTACHMENT_SIZE) return t('store.fileTooBig', { name: f.name })
  }
  return null
}

function withXContent (post: Post): Post {
  const raw = (xContent as Record<string, { text: string, links: string[], avatar?: string }>)[post.id]
  const text = post.text || raw?.text || ''
  // Rows imported with posts.text carry their own expanded links; the
  // bundled x-content.json copy (t.co links) only fills in for older rows.
  const links = [...new Set([...(post.links || []), ...(post.text ? [] : [...(raw?.links || []), ...(text.match(/https?:\/\/[^\s<>"“”]+/gu) || [])])])]
    .filter(url => { try { return ['https:', 'http:'].includes(new URL(url).protocol) } catch { return false } })
  return { ...post, text, links, author_avatar: rawAvatar(post) || raw?.avatar || '' }
}

export function useStore () {
  const { mock, apiFetch, apiUpload, apiBase, authHeaders, askToken } = useApi()
  const mockStore = useMockStore()
  const loading = useState<boolean>('xd-loading', () => false)
  const error = useState<string>('xd-error', () => '')
  const mockDeleted = useState<string[]>('xd-mock-deleted-posts', () => [])
  const mockAutoRefresh = useState<string[]>('xd-mock-auto-refresh', () => [])

  async function loadPosts (params: {
    authors?: string[], tags?: string[], categories?: string[],
    keyword?: string, dateFrom?: string, dateTo?: string, kind?: string, minChars?: number,
    page?: number, pageSize?: number, sort?: string
  } = {}): Promise<{ updated_at: string, total: number, posts: Post[] }> {
    if (mock.value) {
      const posts = MOCK_POSTS.filter(p => !mockDeleted.value.includes(p.id)).sort((a, b) => {
        const d = Date.parse(b.published_at) - Date.parse(a.published_at)
        return d !== 0 ? d : a.id.localeCompare(b.id)
      })
      return { updated_at: MOCK_UPDATED_AT, total: posts.length, posts: posts.map(withXContent) }
    }
    // Server-side filtering + pagination: the API returns only the requested
    // page, so the client never downloads the whole library at once.
    const q = new URLSearchParams()
    if (params.authors?.length) q.set('author', params.authors.join(','))
    if (params.tags?.length) q.set('tag', params.tags.join(','))
    if (params.categories?.length) q.set('category', params.categories.join(','))
    if (params.keyword?.trim()) q.set('q', params.keyword.trim())
    if (params.dateFrom) q.set('date_from', params.dateFrom)
    if (params.dateTo) q.set('date_to', params.dateTo)
    if (params.kind) q.set('kind', params.kind)
    if (params.minChars) q.set('min_chars', String(params.minChars))
    if (params.sort) q.set('sort', params.sort)
    q.set('page', String(params.page || 1))
    q.set('page_size', String(params.pageSize || 20))
    const data = await apiFetch<{ updated_at: string, total: number, posts: Post[] }>(`posts?${q.toString()}`)
    data.posts = data.posts.map(withXContent)
    return data
  }
  async function loadWatchlist (): Promise<Watchlist> {
    if (mock.value) return mockStore.watchlist.value
    return apiFetch<Watchlist>('watchlist')
  }
  // Posts per account in the last 30 days (GET /watchlist?with_stats=1).
  async function loadWatchlistActivity (): Promise<Record<string, number>> {
    if (mock.value) return {}
    return (await apiFetch<{ posts_30d?: Record<string, number> }>('watchlist?with_stats=1')).posts_30d || {}
  }
  // Paste a following list: handles / @handles / profile links → default group.
  async function bulkAddAccounts (text: string): Promise<{ created: string[], existing: string[], invalid: string[] }> {
    return apiFetch('watchlist/accounts/bulk', { method: 'POST', body: JSON.stringify({ text }) })
  }
  async function loadTags (): Promise<string[]> {
    if (mock.value) {
      const counts = new Map<string, number>()
      for (const p of MOCK_POSTS) for (const t of p.tags || []) counts.set(t, (counts.get(t) || 0) + 1)
      return [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([t]) => t)
    }
    const data = await apiFetch<{ tags: string[] }>('tags')
    return data.tags || []
  }
  async function loadCategories (): Promise<Categories> {
    if (mock.value) return mockStore.categories.value
    return apiFetch<Categories>('categories')
  }
  async function loadReplicas (sort: 'time' | 'time_asc' | 'edited' | 'progress' = 'time'): Promise<Replica[]> {
    if (mock.value) {
      const list = [...mockStore.replicas.value]
      if (sort === 'progress') {
        const prog = (r: Replica) => r.steps.length ? r.steps.filter(s => s.status === 'done').length / r.steps.length : 0
        list.sort((a, b) => prog(b) - prog(a))
      } else if (sort === 'edited') {
        list.sort((a, b) => Date.parse(b.updated_at || b.created_at) - Date.parse(a.updated_at || a.created_at))
      } else if (sort === 'time_asc') {
        list.sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at))
      } else list.sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
      return list
    }
    const data = await apiFetch<Replica[] | { replicas: Replica[] }>(`replicas?sort=${sort}`)
    return Array.isArray(data) ? data : (data as any).replicas || []
  }
  async function createReplica (post: Post): Promise<{ replica: Replica, existing: boolean }> {
    if (mock.value) return mockStore.createReplica(post)
    try {
      const data = await apiFetch<any>('replicas', { method: 'POST', body: JSON.stringify({ post_id: post.id }) })
      const replica: Replica = data.replica || data
      return { replica, existing: !!data.existing }
    } catch (e) {
      // Some servers return the existing object directly (SPEC allows either).
      if (e instanceof ApiError && e.status === 200) throw e
      throw e
    }
  }
  async function createStandaloneReplica (title: string): Promise<Replica> {
    if (mock.value) return mockStore.createMockStandaloneReplica(title).replica
    return await apiFetch<Replica>('replicas', { method: 'POST', body: JSON.stringify({ title }) })
  }
  async function patchReplicaSteps (id: string, steps: Array<{ idx: number, status?: string, note?: string, text?: string }>) {
    if (mock.value) { mockStore.patchReplicaSteps(id, steps); return }
    await apiFetch(`replicas/${id}`, { method: 'PATCH', body: JSON.stringify({ steps }) })
  }
  async function addReplicaStep (id: string, text: string, parentIdx?: number | null): Promise<ReplicaStep> {
    if (mock.value) return mockStore.addMockReplicaStep(id, text, parentIdx ?? null)
    return await apiFetch<ReplicaStep>(`replicas/${id}/steps`, { method: 'POST', body: JSON.stringify({ text, parent_idx: parentIdx ?? null }) })
  }
  async function deleteReplicaStep (id: string, idx: number) {
    if (mock.value) { mockStore.deleteMockReplicaStep(id, idx); return }
    await apiFetch(`replicas/${id}/steps/${idx}`, { method: 'DELETE' })
  }
  async function deleteReplica (id: string) {
    if (mock.value) { mockStore.deleteReplica(id); return }
    await apiFetch(`replicas/${id}`, { method: 'DELETE' })
  }
  async function uploadAttachments (replicaId: string, stepIdx: number, files: File[]): Promise<Attachment[]> {
    const err = validateAttachmentFiles(files)
    if (err) throw new Error(err)
    if (mock.value) return mockStore.addMockAttachments(replicaId, stepIdx, files)
    const data = await apiUpload<any>(`replicas/${replicaId}/attachments?step_idx=${stepIdx}`, files)
    return Array.isArray(data) ? data : (data.attachments || data || [])
  }
  async function deleteAttachment (id: string) {
    if (mock.value) { mockStore.deleteMockAttachment(id); return }
    await apiFetch(`attachments/${id}`, { method: 'DELETE' })
  }
  async function renameAttachment (id: string, name: string): Promise<Attachment> {
    if (mock.value) { return mockStore.renameMockAttachment(id, name) }
    return await apiFetch<Attachment>(`attachments/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) })
  }
  // 录音转文字：把录音 blob 发给后端的 /api/v1/stt（本地 Whisper），返回识别文本。
  // mock 模式下直接提示不可用。
  async function transcribeAudio (blob: Blob): Promise<string> {
    if (mock.value) throw new Error('preview mode: transcription unavailable')
    const base = apiBase()
    const url = `${base.replace(/\/$/, '')}/stt`
    const fd = new FormData()
    fd.append('audio', blob, 'recording.webm')
    let res: Response
    try { res = await fetch(url, { method: 'POST', body: fd, headers: authHeaders() }) } catch (e: any) {
      throw new Error(e?.message || 'network failed')
    }
    if (res.status === 401 && askToken()) return transcribeAudio(blob)
    const data = await res.json().catch(() => null)
    if (!res.ok) throw new Error(data?.message || `transcription failed (HTTP ${res.status})`)
    return (data?.text || '').trim()
  }
  async function replicaAttachments (replicaId: string): Promise<Attachment[]> {
    if (mock.value) {
      const r = mockStore.replicas.value.find(x => x.id === replicaId)
      return r ? r.steps.flatMap(s => s.attachments || []) : []
    }
    const data = await apiFetch<any>(`replicas/${replicaId}/attachments`)
    return Array.isArray(data) ? data : (data.attachments || [])
  }
  // Watchlist / category mutations (SPEC endpoints; mock = local state).
  async function addGroup (name: string) {
    if (mock.value) {
      const id = `g-${Date.now().toString(36)}`
      mockStore.watchlist.value = { ...mockStore.watchlist.value, groups: [...mockStore.watchlist.value.groups, { id, name, fixed: false }] }
      return
    }
    await apiFetch('watchlist/groups', { method: 'POST', body: JSON.stringify({ name }) })
  }
  async function deleteGroup (id: string) {
    if (mock.value) {
      mockStore.watchlist.value = {
        groups: mockStore.watchlist.value.groups.filter(g => g.id !== id),
        accounts: mockStore.watchlist.value.accounts.map(a => a.group === id ? { ...a, group: 'other' } : a)
      }
      return
    }
    await apiFetch(`watchlist/groups/${id}`, { method: 'DELETE' })
  }
  async function addAccount (a: any) {
    if (mock.value) { mockStore.watchlist.value = { ...mockStore.watchlist.value, accounts: [...mockStore.watchlist.value.accounts, a] }; return }
    await apiFetch('watchlist/accounts', { method: 'POST', body: JSON.stringify(a) })
  }
  async function patchAccount (username: string, patch: any) {
    if (mock.value) {
      mockStore.watchlist.value = { ...mockStore.watchlist.value, accounts: mockStore.watchlist.value.accounts.map(a => a.username === username ? { ...a, ...patch } : a) }
      return
    }
    await apiFetch(`watchlist/accounts/${username}`, { method: 'PATCH', body: JSON.stringify(patch) })
  }
  async function deleteAccount (username: string) {
    if (mock.value) { mockStore.watchlist.value = { ...mockStore.watchlist.value, accounts: mockStore.watchlist.value.accounts.filter(a => a.username !== username) }; return }
    await apiFetch(`watchlist/accounts/${username}`, { method: 'DELETE' })
  }
  async function addCategory (name: string) {
    if (mock.value) {
      const id = `c-${Date.now().toString(36)}`
      mockStore.categories.value = { ...mockStore.categories.value, categories: [...mockStore.categories.value.categories, { id, name }] }
      mockStore.persist()
      return
    }
    await apiFetch('categories', { method: 'POST', body: JSON.stringify({ name }) })
  }
  async function deleteCategory (id: string) {
    if (mock.value) {
      const assignments: Record<string, string[]> = {}
      for (const [k, v] of Object.entries(mockStore.categories.value.assignments)) assignments[k] = v.filter(x => x !== id)
      mockStore.categories.value = { categories: mockStore.categories.value.categories.filter(c => c.id !== id), assignments }
      mockStore.persist()
      return
    }
    await apiFetch(`categories/${id}`, { method: 'DELETE' })
  }
  // Top-bar notifications (new releases of tracked repos).
  async function loadNotifications (): Promise<{ unread: number, notifications: any[] }> {
    return apiFetch('notifications')
  }
  async function markNotificationsRead (ids: string[]) {
    await apiFetch('notifications/read', { method: 'POST', body: JSON.stringify({ ids }) })
  }
  // Replica from a repo README's install / quick-start steps.
  async function createReplicaFromRepo (fullName: string): Promise<Replica> {
    return apiFetch<Replica>(`repos/${fullName}/replica`, { method: 'POST' })
  }
  // Copy a flow's steps into a new standalone flow (POST /replicas/{id}/clone).
  async function cloneReplica (id: string, copyNotes = false): Promise<Replica> {
    return apiFetch<Replica>(`replicas/${id}/clone`, { method: 'POST', body: JSON.stringify({ copy_notes: copyNotes }) })
  }
  // Multi-select batch actions on the X daily page.
  async function batchCategory (postIds: string[], categoryId: string, action: 'add' | 'remove'): Promise<{ changed: number }> {
    return apiFetch('categories/assignments/batch', { method: 'POST', body: JSON.stringify({ post_ids: postIds, category_id: categoryId, action }) })
  }
  async function batchPostSummaries (ids: string[], lang: string, force = false): Promise<{ queued: number, skipped: number, missing: number, results: Record<string, string> }> {
    return apiFetch('posts/summary/batch', { method: 'POST', body: JSON.stringify({ ids, lang, force }) })
  }
  // Saved filter views (backend views.go).
  async function loadViews (): Promise<SavedView[]> {
    if (mock.value) return []
    return (await apiFetch<{ views: SavedView[] }>('views')).views
  }
  async function createView (name: string, filter: unknown): Promise<SavedView> {
    return apiFetch<SavedView>('views', { method: 'POST', body: JSON.stringify({ name, filter }) })
  }
  async function deleteView (id: string) {
    await apiFetch(`views/${id}`, { method: 'DELETE' })
  }
  // Delete posts (backend records them so imports do not bring them back).
  async function deletePosts (ids: string[]): Promise<{ deleted: number }> {
    if (mock.value) {
      const hit = ids.filter(id => MOCK_POSTS.some(p => p.id === id) && !mockDeleted.value.includes(id))
      mockDeleted.value = [...mockDeleted.value, ...hit]
      return { deleted: hit.length }
    }
    if (ids.length === 1) return apiFetch(`posts/${encodeURIComponent(ids[0]!)}`, { method: 'DELETE' })
    return apiFetch('posts/batch-delete', { method: 'POST', body: JSON.stringify({ ids }) })
  }
  async function setPostReplicaEnabled (id: string, v: boolean | null) {
    if (mock.value) return
    await apiFetch(`posts/${id}`, { method: 'PATCH', body: JSON.stringify({ replica_enabled: v }) })
  }
  async function renameCategory (id: string, name: string) {
    if (mock.value) {
      mockStore.categories.value = { ...mockStore.categories.value, categories: mockStore.categories.value.categories.map(c => c.id === id ? { ...c, name } : c) }
      mockStore.persist()
      return
    }
    await apiFetch(`categories/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) })
  }
  async function restoreCategory (id: string) {
    if (mock.value) return // mock delete is permanent
    await apiFetch(`categories/${id}/restore`, { method: 'POST' })
  }
  async function setPostCategories (postId: string, categoryIds: string[]) {
    if (mock.value) {
      mockStore.categories.value = { ...mockStore.categories.value, assignments: { ...mockStore.categories.value.assignments, [postId]: categoryIds } }
      mockStore.persist()
      return
    }
    await apiFetch('categories/assignments', { method: 'PUT', body: JSON.stringify({ post_id: postId, category_ids: categoryIds }) })
  }
  // ---------- Content translation (POST /api/v1/translate) ----------
  // Mock mode has no backend: return the originals (UI toggle still works).
  async function translateTexts (texts: string[], target: 'en' | 'zh-CN'): Promise<string[]> {
    if (mock.value) return texts
    if (!texts.length) return []
    const data = await apiFetch<{ translations: string[] }>('translate', { method: 'POST', body: JSON.stringify({ texts, target }) })
    return Array.isArray(data.translations) && data.translations.length === texts.length ? data.translations : texts
  }
  // ---------- GitHub repos (contract 2026-10-25; all data via /api/v1/repos...) ----------
  const REPO_FULL_NAME_RE = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/
  function validateRepoFullName (fullName: string): string | null {
    return REPO_FULL_NAME_RE.test(fullName.trim()) ? null : t('repos.invalidFullName')
  }
  async function loadRepos (): Promise<Repo[]> {
    if (mock.value) return [...mockStore.repos.value]
    const data = await apiFetch<{ repos: Repo[] } | Repo[]>('repos')
    return Array.isArray(data) ? data : (data.repos || [])
  }
  async function loadRepoGroups (): Promise<RepoGroup[]> {
    if (mock.value) return [...mockStore.repoGroups.value]
    const data = await apiFetch<{ groups: RepoGroup[] } | RepoGroup[]>('repos/groups')
    return Array.isArray(data) ? data : (data.groups || [])
  }
  async function addRepo (fullName: string, groupId?: string | null, tags?: string[], note?: string): Promise<Repo> {
    const err = validateRepoFullName(fullName)
    if (err) throw new ApiError(422, 'INVALID_FULL_NAME', err)
    const body = { full_name: fullName.trim(), group_id: groupId || undefined, tags: tags || [], note: note || '' }
    if (mock.value) return mockStore.addMockRepo(body.full_name, body.group_id ?? null, body.tags, body.note)
    const data = await apiFetch<any>('repos', { method: 'POST', body: JSON.stringify(body) })
    return data.repo || data
  }
  async function loadRepoDetail (fullName: string): Promise<RepoDetail> {
    if (mock.value) {
      const repo = mockStore.repos.value.find(r => r.full_name.toLowerCase() === fullName.toLowerCase())
      if (!repo) throw new ApiError(404, 'NOT_FOUND', t('repos.notFound'))
      return { repo, commits: mockRepoCommits(repo.full_name), latest_release: mockRepoRelease(repo.full_name), releases: mockRepoReleases(repo.full_name) }
    }
    const data = await apiFetch<any>(`repos/${fullName}`)
    const repo: Repo = data.repo || data
    // Full release list: prefer it if the detail endpoint already embeds it,
    // otherwise fetch the releases endpoint. A releases failure must not
    // break the whole detail load — fall back to an empty list.
    let releases: RepoRelease[] | undefined = Array.isArray(data.releases) ? data.releases : undefined
    if (releases === undefined) {
      try { releases = await loadRepoReleases(fullName) } catch { releases = [] }
    }
    return { repo, commits: data.commits || [], latest_release: data.latest_release ?? null, releases: releases || [] }
  }
  async function loadRepoCommits (fullName: string) {
    if (mock.value) return mockRepoCommits(fullName)
    const data = await apiFetch<any>(`repos/${fullName}/commits`)
    return Array.isArray(data) ? data : (data.commits || [])
  }
  async function loadRepoReleases (fullName: string) {
    if (mock.value) return mockRepoReleases(fullName)
    const data = await apiFetch<any>(`repos/${fullName}/releases`)
    return Array.isArray(data) ? data : (data.releases || [])
  }
  async function patchRepo (fullName: string, patch: { group_id?: string | null, tags?: string[], note?: string }): Promise<Repo | null> {
    if (mock.value) return mockStore.patchMockRepo(fullName, patch)
    const data = await apiFetch<any>(`repos/${fullName}`, { method: 'PATCH', body: JSON.stringify(patch) })
    return data.repo || data
  }
  async function deleteRepo (fullName: string) {
    if (mock.value) { mockStore.deleteMockRepo(fullName); return }
    await apiFetch(`repos/${fullName}`, { method: 'DELETE' })
  }
  async function syncRepo (fullName: string): Promise<Repo | null> {
    if (mock.value) return mockStore.syncMockRepo(fullName)
    const data = await apiFetch<any>(`repos/${fullName}/sync`, { method: 'POST' })
    return data.repo || data || null
  }
  // Background job (backend repos_sync.go): POST starts it (202), GET polls.
  async function syncAllRepos (): Promise<RepoSyncJob> {
    if (mock.value) {
      const total = mockStore.repos.value.length
      for (const r of mockStore.repos.value) mockStore.syncMockRepo(r.full_name)
      return { running: false, done: total, synced: total, failed: 0, rate_limited: 0, total }
    }
    return apiFetch<RepoSyncJob>('repos/sync-all', { method: 'POST' })
  }
  async function getRepoSyncStatus (): Promise<RepoSyncJob> {
    return apiFetch<RepoSyncJob>('repos/sync-all')
  }
  async function addRepoGroup (name: string): Promise<RepoGroup> {
    if (mock.value) return mockStore.addMockRepoGroup(name)
    const data = await apiFetch<any>('repos/groups', { method: 'POST', body: JSON.stringify({ name }) })
    return data.group || data
  }
  async function deleteRepoGroup (id: string) {
    if (mock.value) { mockStore.deleteMockRepoGroup(id); return }
    await apiFetch(`repos/groups/${id}`, { method: 'DELETE' })
  }
  // ---------- YouTube channels (prototype 2026-10-26; all data via /api/v1/yt/...) ----------
  async function loadYTChannels (p: VideoPlatform = 'yt'): Promise<YTChannel[]> {
    if (mock.value) return [...mockStore.ytChannels.value]
    const data = await apiFetch<{ channels: YTChannel[] } | YTChannel[]>(`${p}/channels`)
    return Array.isArray(data) ? data : (data.channels || [])
  }
  async function loadYTVideos (p: VideoPlatform = 'yt'): Promise<YTVideo[]> {
    if (mock.value) return [...mockStore.ytVideos.value]
    // Prototype volume is small: fetch up to 500 newest and filter
    // client-side (same instant-filter UX as the other pages). The
    // server also supports q/channel_id/date_from/date_to/limit.
    const data = await apiFetch<{ videos: YTVideo[] } | YTVideo[]>(`${p}/videos?limit=500`)
    return Array.isArray(data) ? data : (data.videos || [])
  }
  async function setVideoState (id: string, state: { watched?: boolean, watch_later?: boolean }, p: VideoPlatform = 'yt') {
    if (mock.value) return
    await apiFetch(`${p}/videos/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(state) })
  }
  async function loadYTGroups (p: VideoPlatform = 'yt'): Promise<RepoGroup[]> {
    if (mock.value) return [...mockStore.ytGroups.value]
    const data = await apiFetch<{ groups: RepoGroup[] } | RepoGroup[]>(`${p}/groups`)
    return Array.isArray(data) ? data : (data.groups || [])
  }
  async function addYTChannel (input: string, groupId?: string | null, dateFrom?: string | null, dateTo?: string | null, p: VideoPlatform = 'yt'): Promise<YTChannel> {
    if (!input.trim()) throw new ApiError(422, 'VALIDATION', t('yt.inputLabel'))
    const body: Record<string, unknown> = { channel_id: input.trim(), group_id: groupId || undefined }
    if (dateFrom) body.date_from = dateFrom
    if (dateTo) body.date_to = dateTo
    if (mock.value) return mockStore.addMockYTChannel(body.channel_id as string, body.group_id as string | null ?? null, dateFrom || null, dateTo || null)
    const data = await apiFetch<any>(`${p}/channels`, { method: 'POST', body: JSON.stringify(body) })
    return data.channel || data
  }
  async function updateYTChannel (id: string, patch: { group_id?: string | null, date_from?: string | null, date_to?: string | null }, p: VideoPlatform = 'yt'): Promise<YTChannel> {
    if (mock.value) return mockStore.patchMockYTChannel(id, patch)
    const data = await apiFetch<any>(`${p}/channels/${id}`, { method: 'PATCH', body: JSON.stringify(patch) })
    return data.channel || data
  }
  async function deleteYTChannel (id: string, p: VideoPlatform = 'yt') {
    if (mock.value) { mockStore.deleteMockYTChannel(id); return }
    await apiFetch(`${p}/channels/${id}`, { method: 'DELETE' })
  }
  // full=true on a single-video channel: switch it to full mode and sync all its videos.
  async function syncYTChannel (id: string, p: VideoPlatform = 'yt', full = false): Promise<YTChannel | null> {
    if (mock.value) return mockStore.syncMockYTChannel(id)
    const data = await apiFetch<any>(`${p}/channels/${id}/sync${full ? '?full=1' : ''}`, { method: 'POST' })
    return data.channel || data || null
  }
  // Add one YouTube video by URL; its channel is registered (single mode) when not followed yet.
  async function previewYTVideo (url: string, p: VideoPlatform = 'yt'): Promise<{ video: YTVideo, channel: { channel_id: string, name: string, handle?: string }, existing_channel?: YTChannel }> {
    if (mock.value) throw new ApiError(501, 'MOCK', '演示模式不支持预览')
    return apiFetch(`${p}/videos/preview`, { method: 'POST', body: JSON.stringify({ url: url.trim() }) })
  }
  async function addYTVideo (url: string, groupId?: string | null, p: VideoPlatform = 'yt', tags: string[] = []): Promise<{ video: YTVideo, channel: YTChannel, channel_created: boolean }> {
    if (!url.trim()) throw new ApiError(422, 'VALIDATION', t('yt.videoInputLabel'))
    return apiFetch(`${p}/videos`, { method: 'POST', body: JSON.stringify({ url: url.trim(), group_id: groupId || undefined, tags }) })
  }
  async function syncAllYTChannels (p: VideoPlatform = 'yt'): Promise<{ synced: number, failed: number, total: number, skipped?: number }> {
    if (mock.value) {
      const total = mockStore.ytChannels.value.length
      for (const c of mockStore.ytChannels.value) mockStore.syncMockYTChannel(c.id)
      return { synced: total, failed: 0, total }
    }
    return apiFetch<{ synced: number, failed: number, total: number }>(`${p}/sync`, { method: 'POST' })
  }
  // AI summary: GET reads the stored one (status 'none' when never
  // generated); POST starts a background job (or returns the finished
  // one unless force) — the page polls GET while status is 'pending'.
  // X post AI summary: same flow as the video one; POST sends the original
  // text/links (they live in x-content.json on the page, not in the DB).
  // X daily: single-post import / account refresh (backend x_import.go).
  async function importXPost (url: string, force = false, tags: string[] = [], groupId?: string, postGroupId?: string): Promise<{ post: Post, existed?: boolean, account_created?: boolean }> {
    if (mock.value) throw new ApiError(501, 'MOCK', '演示模式不支持导入')
    const data = await apiFetch<{ post: Post, existed?: boolean, account_created?: boolean }>('posts/import', { method: 'POST', body: JSON.stringify({ url: url.trim(), force, tags, group_id: groupId || undefined, post_group_id: postGroupId || undefined }) })
    data.post = withXContent(data.post)
    return data
  }
  async function loadTags (): Promise<string[]> {
    if (mock.value) return []
    const data = await apiFetch<{ tags: string[] }>('tags')
    return data.tags || []
  }
  async function loadPostGroups (): Promise<{ id: string, name: string, fixed?: boolean }[]> {
    if (mock.value) return []
    const data = await apiFetch<{ groups: { id: string, name: string, fixed?: boolean }[] }>('post-groups')
    return data.groups || []
  }
  async function ensurePostGroup (name: string): Promise<{ id: string, groups: { id: string, name: string }[] }> {
    if (mock.value) throw new ApiError(501, 'MOCK', '演示模式不支持')
    return apiFetch('post-groups', { method: 'POST', body: JSON.stringify({ name: name.trim() }) })
  }
  async function getXRefresh (): Promise<XRefreshJob> {
    if (mock.value) return { running: false, accounts: [], configured: false }
    return apiFetch<XRefreshJob>('x/refresh')
  }
  async function startXRefresh (usernames: string[]): Promise<XRefreshJob> {
    if (mock.value) throw new ApiError(501, 'MOCK', '演示模式不支持更新')
    return apiFetch<XRefreshJob>('x/refresh', { method: 'POST', body: JSON.stringify({ usernames }) })
  }
  // Accounts the scheduled refresh (REFRESH_INTERVAL) covers; interval is
  // '' when the schedule is off on the server. Mock mode keeps them in memory.
  async function getXRefreshAccounts (): Promise<XRefreshAccounts> {
    if (mock.value) return { usernames: [...mockAutoRefresh.value], interval: '' }
    return apiFetch<XRefreshAccounts>('x/refresh/accounts')
  }
  async function setXRefreshAccounts (usernames: string[]): Promise<XRefreshAccounts> {
    if (mock.value) { mockAutoRefresh.value = [...new Set(usernames)].sort(); return { usernames: [...mockAutoRefresh.value], interval: '' } }
    return apiFetch<XRefreshAccounts>('x/refresh/accounts', { method: 'PUT', body: JSON.stringify({ usernames }) })
  }
  // Re-import only the posts that failed in the last refresh job.
  async function retryXRefresh (): Promise<XRefreshJob> {
    return apiFetch<XRefreshJob>('x/refresh/retry', { method: 'POST' })
  }
  async function getPostSummary (id: string, lang: string): Promise<YTSummary> {
    if (mock.value) return { ...mockStore.mockYTSummary(id, lang), chapters: [] }
    const data = await apiFetch<{ summary: YTSummary }>(`posts/${encodeURIComponent(id)}/summary?lang=${lang}`)
    return data.summary
  }
  async function startPostSummary (id: string, lang: string, body: { text: string, links: string[] }, force = false): Promise<YTSummary> {
    if (mock.value) return { ...mockStore.mockYTSummary(id, lang), chapters: [] }
    const data = await apiFetch<{ summary: YTSummary }>(`posts/${encodeURIComponent(id)}/summary?lang=${lang}${force ? '&force=1' : ''}`, { method: 'POST', body: JSON.stringify(body) })
    return data.summary
  }
  // GitHub repo AI summary (GET/POST /api/v1/repos/{owner}/{name}/summary).
  async function getRepoSummary (fullName: string, lang: string): Promise<YTSummary> {
    if (mock.value) return { ...mockStore.mockYTSummary(fullName, lang), chapters: [] }
    const data = await apiFetch<{ summary: YTSummary }>(`repos/${fullName}/summary?lang=${lang}`)
    return data.summary
  }
  async function startRepoSummary (fullName: string, lang: string, force = false): Promise<YTSummary> {
    if (mock.value) return { ...mockStore.mockYTSummary(fullName, lang), chapters: [] }
    const data = await apiFetch<{ summary: YTSummary }>(`repos/${fullName}/summary?lang=${lang}${force ? '&force=1' : ''}`, { method: 'POST' })
    return data.summary
  }
  async function getYTSummary (id: string, lang: string, p: VideoPlatform = 'yt'): Promise<YTSummary> {
    if (mock.value) return mockStore.mockYTSummary(id, lang)
    const data = await apiFetch<{ summary: YTSummary }>(`${p}/videos/${encodeURIComponent(id)}/summary?lang=${lang}`)
    return data.summary
  }
  async function startYTSummary (id: string, lang: string, force = false, p: VideoPlatform = 'yt'): Promise<YTSummary> {
    if (mock.value) return mockStore.mockYTSummary(id, lang)
    const data = await apiFetch<{ summary: YTSummary }>(`${p}/videos/${encodeURIComponent(id)}/summary?lang=${lang}${force ? '&force=1' : ''}`, { method: 'POST' })
    return data.summary
  }
  async function addYTGroup (name: string, p: VideoPlatform = 'yt'): Promise<RepoGroup> {
    if (mock.value) return mockStore.addMockYTGroup(name)
    const data = await apiFetch<any>(`${p}/groups`, { method: 'POST', body: JSON.stringify({ name }) })
    return data.group || data
  }
  async function deleteYTGroup (id: string, p: VideoPlatform = 'yt') {
    if (mock.value) { mockStore.deleteMockYTGroup(id); return }
    await apiFetch(`${p}/groups/${id}`, { method: 'DELETE' })
  }
  return {
    mock, loading, error, loadPosts, loadWatchlist, loadWatchlistActivity, bulkAddAccounts, loadTags, loadCategories,
    loadReplicas, createReplica, createStandaloneReplica, patchReplicaSteps, addReplicaStep, deleteReplicaStep, deleteReplica, uploadAttachments,
    deleteAttachment, renameAttachment, transcribeAudio, replicaAttachments, addGroup, deleteGroup, addAccount, patchAccount,
    deleteAccount, addCategory, deleteCategory, renameCategory, restoreCategory, deletePosts, setPostReplicaEnabled, loadViews, createView, deleteView, batchCategory, batchPostSummaries, cloneReplica, loadNotifications, markNotificationsRead, createReplicaFromRepo, setPostCategories, translateTexts,
    loadRepos, loadRepoGroups, addRepo, loadRepoDetail, loadRepoCommits, loadRepoReleases,
    patchRepo, deleteRepo, syncRepo, syncAllRepos, getRepoSyncStatus, addRepoGroup, deleteRepoGroup, validateRepoFullName,
    loadYTChannels, loadYTVideos, addYTChannel, updateYTChannel, deleteYTChannel, syncYTChannel, syncAllYTChannels, getYTSummary, startYTSummary, addYTVideo, getPostSummary, startPostSummary, getRepoSummary, startRepoSummary, importXPost, loadTags, loadPostGroups, ensurePostGroup, previewYTVideo, getXRefresh, startXRefresh, retryXRefresh, getXRefreshAccounts, setXRefreshAccounts,
    loadYTGroups, addYTGroup, deleteYTGroup, setVideoState
  }
}
