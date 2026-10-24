// Data layer: real /api/v1 by default, mock fallback when NUXT_PUBLIC_USE_MOCK=1.
// Pages only ever talk to this layer, never to raw fetch or raw mock data.
import type { Post, Watchlist, Categories, Replica, Attachment } from './types'
import { ApiError } from './types'
import { MOCK_POSTS, MOCK_UPDATED_AT } from './useMock'

export const ATTACHMENT_EXTS = 'pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif webp bmp avif svg ico zip rar 7z tar gz tgz'.split(' ')
export const IMAGE_EXTS = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'avif', 'svg', 'ico']
export const MAX_ATTACHMENT_SIZE = 10 * 1024 * 1024

export function validateAttachmentFiles (files: File[]): string | null {
  for (const f of files) {
    const ext = f.name.split('.').pop()?.toLowerCase() || ''
    if (!ATTACHMENT_EXTS.includes(ext)) return `不支持的文件类型：${f.name}`
    if (f.size > MAX_ATTACHMENT_SIZE) return `文件超过 10MB：${f.name}`
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
  async function patchReplicaSteps (id: string, steps: Array<{ idx: number, status?: string, note?: string }>) {
    if (mock.value) { mockStore.patchReplicaSteps(id, steps); return }
    await apiFetch(`replicas/${id}`, { method: 'PATCH', body: JSON.stringify({ steps }) })
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
  return {
    loading, error, loadPosts, loadWatchlist, loadTags, loadUserTags, loadCategories,
    loadReplicas, createReplica, patchReplicaSteps, deleteReplica, uploadAttachments,
    deleteAttachment, replicaAttachments, addGroup, deleteGroup, addAccount, patchAccount,
    deleteAccount, addCategory, deleteCategory, setPostCategories, setUserTags
  }
}
