// Data layer: real /api/v1 by default, mock fallback when NUXT_PUBLIC_USE_MOCK=1.
// Pages only ever talk to this layer, never to raw fetch or raw mock data.
import type { Post, Watchlist, Categories, Replica, ReplicaStep, Attachment, Repo, RepoGroup, RepoDetail, RepoRelease, YTChannel, YTVideo, YTSummary, YTComment } from './types'
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

export function useStore () {
  const { mock, apiFetch, apiUpload } = useApi()
  const mockStore = useMockStore()
  const loading = useState<boolean>('xd-loading', () => false)
  const error = useState<string>('xd-error', () => '')

  async function loadPosts (): Promise<{ updated_at: string, total: number, posts: Post[] }> {
    if (mock.value) {
      const posts = [...MOCK_POSTS].sort((a, b) => {
        const d = Date.parse(b.published_at) - Date.parse(a.published_at)
        return d !== 0 ? d : a.id.localeCompare(b.id)
      })
      return { updated_at: MOCK_UPDATED_AT, total: posts.length, posts }
    }
    const data = await apiFetch<{ updated_at: string, total: number, posts: Post[] }>('posts?page=1&page_size=1000')
    // Client-side filtering (SPEC semantics) is applied in the page; the server
    // list is already published_at DESC, id ASC per SPEC.
    return data
  }
  async function loadWatchlist (): Promise<Watchlist> {
    if (mock.value) return mockStore.watchlist.value
    return apiFetch<Watchlist>('watchlist')
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
  async function loadUserTags (): Promise<{ vocabulary: string[], overrides: Record<string, string[]> }> {
    if (mock.value) return { vocabulary: mockStore.vocabulary.value, overrides: mockStore.userTagOverrides.value }
    return apiFetch<{ vocabulary: string[], overrides: Record<string, string[]> }>('user-tags')
  }
  async function loadCategories (): Promise<Categories> {
    if (mock.value) return mockStore.categories.value
    return apiFetch<Categories>('categories')
  }
  async function loadReplicas (sort: 'time' | 'progress' = 'time'): Promise<Replica[]> {
    if (mock.value) {
      const list = [...mockStore.replicas.value]
      if (sort === 'progress') {
        const prog = (r: Replica) => r.steps.length ? r.steps.filter(s => s.status === 'done').length / r.steps.length : 0
        list.sort((a, b) => prog(b) - prog(a))
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
  async function replicaAttachments (replicaId: string): Promise<Attachment[]> {
    if (mock.value) {
      const r = mockStore.replicas.value.find(x => x.id === replicaId)
      return r ? r.steps.flatMap(s => s.attachments || []) : []
    }
    const data = await apiFetch<any>(`replicas/${replicaId}/attachments`)
    return Array.isArray(data) ? data : (data.attachments || [])
  }
  // Watchlist / category / user-tag mutations (SPEC endpoints; mock = local state).
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
  async function setPostCategories (postId: string, categoryIds: string[]) {
    if (mock.value) {
      mockStore.categories.value = { ...mockStore.categories.value, assignments: { ...mockStore.categories.value.assignments, [postId]: categoryIds } }
      mockStore.persist()
      return
    }
    await apiFetch('categories/assignments', { method: 'PUT', body: JSON.stringify({ post_id: postId, category_ids: categoryIds }) })
  }
  async function setUserTags (username: string, tags: string[]) {
    if (mock.value) { mockStore.userTagOverrides.value = { ...mockStore.userTagOverrides.value, [username]: tags }; return }
    await apiFetch('user-tags', { method: 'PUT', body: JSON.stringify({ username, tags }) })
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
  async function syncAllRepos (): Promise<{ synced: number, failed: number, rate_limited: number, total: number }> {
    if (mock.value) {
      const total = mockStore.repos.value.length
      for (const r of mockStore.repos.value) mockStore.syncMockRepo(r.full_name)
      return { synced: total, failed: 0, rate_limited: 0, total }
    }
    return apiFetch<{ synced: number, failed: number, rate_limited: number, total: number }>('repos/sync-all', { method: 'POST' })
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
  async function syncYTChannel (id: string, p: VideoPlatform = 'yt'): Promise<YTChannel | null> {
    if (mock.value) return mockStore.syncMockYTChannel(id)
    const data = await apiFetch<any>(`${p}/channels/${id}/sync`, { method: 'POST' })
    return data.channel || data || null
  }
  async function syncAllYTChannels (p: VideoPlatform = 'yt'): Promise<{ synced: number, failed: number, total: number }> {
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
  // Top comments for up to 60 videos; "pending" IDs are being fetched in
  // the background, the page asks again until none are left.
  async function loadYTComments (ids: string[], p: VideoPlatform = 'yt'): Promise<{ comments: Record<string, YTComment[]>, pending: string[] }> {
    if (mock.value) return { comments: mockStore.mockYTComments(ids), pending: [] }
    return apiFetch(`${p}/comments?ids=${ids.map(encodeURIComponent).join(',')}`)
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
    loading, error, loadPosts, loadWatchlist, loadTags, loadUserTags, loadCategories,
    loadReplicas, createReplica, createStandaloneReplica, patchReplicaSteps, addReplicaStep, deleteReplicaStep, deleteReplica, uploadAttachments,
    deleteAttachment, replicaAttachments, addGroup, deleteGroup, addAccount, patchAccount,
    deleteAccount, addCategory, deleteCategory, setPostCategories, setUserTags, translateTexts,
    loadRepos, loadRepoGroups, addRepo, loadRepoDetail, loadRepoCommits, loadRepoReleases,
    patchRepo, deleteRepo, syncRepo, syncAllRepos, addRepoGroup, deleteRepoGroup, validateRepoFullName,
    loadYTChannels, loadYTVideos, addYTChannel, updateYTChannel, deleteYTChannel, syncYTChannel, syncAllYTChannels, getYTSummary, startYTSummary, loadYTComments,
    loadYTGroups, addYTGroup, deleteYTGroup
  }
}
