import { ApiError } from './types'
import { t } from '~/utils/i18n'
import { LS } from './useSettings'

// Write requests need the API token (backend auth.go). It goes in X-API-Token,
// not Authorization, so nginx basic auth keeps working in the browser.
// The token lives in localStorage; on a 401 the user is asked for it once and
// the request is retried.
export function apiToken (): string {
  if (typeof localStorage === 'undefined') return ''
  try { return localStorage.getItem(LS.apiToken) || '' } catch { return '' }
}
export function setApiToken (v: string) {
  try { v ? localStorage.setItem(LS.apiToken, v) : localStorage.removeItem(LS.apiToken) } catch { /* private mode */ }
}
function authHeaders (): Record<string, string> {
  const tok = apiToken()
  return tok ? { 'X-API-Token': tok } : {}
}
// Asks for a token after a 401; true when the user entered one (retry).
function askToken (): boolean {
  if (import.meta.server || typeof window === 'undefined') return false
  const v = window.prompt(t('api.tokenPrompt'), apiToken())
  if (!v || !v.trim()) return false
  setApiToken(v.trim())
  return true
}

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

  async function apiFetch<T> (path: string, opts: any = {}, retried = false): Promise<T> {
    const base = apiBase()
    const url = path.startsWith('http') ? path : `${base.replace(/\/$/, '')}/${path.replace(/^\//, '')}`
    let res: Response
    try {
      res = await fetch(url, { ...opts, headers: { 'Content-Type': 'application/json', ...authHeaders(), ...(opts.headers || {}) } })
    } catch (e: any) {
      throw new ApiError(0, 'NETWORK', e?.message || t('api.networkFailed'))
    }
    if (res.status === 401 && !retried && askToken()) return apiFetch<T>(path, opts, true)
    const text = await res.text()
    let data: any = null
    try { data = text ? JSON.parse(text) : null } catch { data = text }
    if (!res.ok) {
      const code = data?.error || `HTTP_${res.status}`
      throw new ApiError(res.status, code, data?.message || t('api.requestFailed', { status: res.status }))
    }
    return data as T
  }
  async function apiUpload<T> (path: string, files: File[], retried = false): Promise<T> {
    const base = apiBase()
    const url = `${base.replace(/\/$/, '')}/${path.replace(/^\//, '')}`
    const fd = new FormData()
    for (const f of files) fd.append('file', f)
    let res: Response
    try { res = await fetch(url, { method: 'POST', body: fd, headers: authHeaders() }) } catch (e: any) {
      throw new ApiError(0, 'NETWORK', e?.message || t('api.uploadFailed'))
    }
    if (res.status === 401 && !retried && askToken()) return apiUpload<T>(path, files, true)
    const data = await res.json().catch(() => null)
    if (!res.ok) throw new ApiError(res.status, data?.error || `HTTP_${res.status}`, data?.message || t('api.uploadFailed'))
    return data as T
  }
  return { mock, apiFetch, apiUpload }
}
