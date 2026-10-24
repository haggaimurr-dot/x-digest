// Dev-only mock/fallback data layer (default OFF, NUXT_PUBLIC_USE_MOCK=1).
// Source: original static data (data/posts.json, config/*.json) normalised to
// the SPEC API shapes. Mutations in mock mode are kept in memory + the same
// localStorage keys the original site used; they never touch a real backend.
import rawPosts from '~/assets/mock/posts.json'
import rawWatchlist from '~/assets/mock/watchlist.json'
import rawUserTags from '~/assets/mock/user-tags.json'
import type { Post, Watchlist, Categories, Replica, Attachment } from './types'
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
  function patchReplicaSteps (id: string, steps: Array<{ idx: number, status?: string, note?: string }>) {
    const r = replicas.value.find(x => x.id === id)
    if (!r) return
    for (const patch of steps) {
      const s = r.steps[patch.idx]
      if (!s) continue
      if (patch.status && ['idle', 'active', 'done'].includes(patch.status)) s.status = patch.status as any
      if (patch.note !== undefined) s.note = patch.note
    }
    persist()
  }
  function deleteReplica (id: string) {
    replicas.value = replicas.value.filter(r => r.id !== id)
    persist()
  }
  function addMockAttachments (replicaId: string, stepIdx: number, files: File[]): Attachment[] {
    const r = replicas.value.find(x => x.id === replicaId)
    if (!r) return []
    const step = r.steps[stepIdx]
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
  return { watchlist, categories, userTagOverrides, vocabulary, replicas, persist, createReplica, patchReplicaSteps, deleteReplica, addMockAttachments, deleteMockAttachment }
}
