import { ApiError } from './types'

// Unified API layer (SPEC):
//  - browser: relative /api/v1/... (nginx proxies /api/ to Go on :18090)
//  - Nuxt SSR: NUXT_SERVER_API_BASE (default http://127.0.0.1:18090/api/v1)
export function apiBase (): string {
  if (import.meta.server) {
    const cfg = useRuntimeConfig()
    return (cfg.serverApiBase as string) || 'http://127.0.0.1:18090/api/v1'
  }
  return '/api/v1'
}
export function useApi () {
  const cfg = useRuntimeConfig()
  // NOTE: Nuxt applies NUXT_PUBLIC_USE_MOCK at runtime via destr(), so the
  // env value '1' arrives as the NUMBER 1 (not boolean true / string '1').
  // Accept all three, otherwise mock mode silently never engages.
  const mock = computed(() => {
    const v = (cfg.public as any).useMock
    return v === true || v === '1' || v === 1
  })

  async function apiFetch<T> (path: string, opts: any = {}): Promise<T> {
    const base = apiBase()
    const url = path.startsWith('http') ? path : `${base.replace(/\/$/, '')}/${path.replace(/^\//, '')}`
    let res: Response
    try {
      res = await fetch(url, { headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) }, ...opts })
    } catch (e: any) {
      throw new ApiError(0, 'NETWORK', e?.message || '网络请求失败')
    }
    const text = await res.text()
    let data: any = null
    try { data = text ? JSON.parse(text) : null } catch { data = text }
    if (!res.ok) {
      const code = data?.error || `HTTP_${res.status}`
      throw new ApiError(res.status, code, data?.message || `请求失败 (${res.status})`)
    }
    return data as T
  }
  async function apiUpload<T> (path: string, files: File[]): Promise<T> {
    const base = apiBase()
    const url = `${base.replace(/\/$/, '')}/${path.replace(/^\//, '')}`
    const fd = new FormData()
    for (const f of files) fd.append('file', f)
    let res: Response
    try { res = await fetch(url, { method: 'POST', body: fd }) } catch (e: any) {
      throw new ApiError(0, 'NETWORK', e?.message || '上传失败')
    }
    const data = await res.json().catch(() => null)
    if (!res.ok) throw new ApiError(res.status, data?.error || `HTTP_${res.status}`, data?.message || '上传失败')
    return data as T
  }
  return { mock, apiFetch, apiUpload }
}
