// localStorage keys — kept identical to the original site (SPEC) so old
// browser data stays readable. Theme/font/webhook stay local even in API mode.
import { t } from '~/utils/i18n'
export const LS = {
  watchlist: 'x-digest-watchlist',
  postCategories: 'x-digest-post-categories',
  selectedAuthors: 'x-digest-selected-authors',
  selectedCats: 'x-digest-selected-cats',
  theme: 'x-digest-theme',
  fontFamily: 'x-digest-font-family',
  fontSize: 'x-digest-font-size',
  lang: 'xd-lang',
  webhookUrl: 'x-digest-webhook-url',
  webhookKey: 'x-digest-webhook-key',
  chipsAuthorsExpanded: 'x-digest-chips-authors-expanded',
  chipsTagsExpanded: 'x-digest-chips-tags-expanded',
  chipsCatsExpanded: 'x-digest-chips-cats-expanded',
  filtersCollapsed: 'x-digest-filters-collapsed',
  replicas: 'xdigest.replicas', // mock-mode only; API mode uses the server
  apiToken: 'xdigest.apiToken' // API_TOKEN for write requests (see useApi)
}

function read (key: string): string | null {
  if (typeof localStorage === 'undefined') return null
  try { return localStorage.getItem(key) } catch { return null }
}
function write (key: string, val: string) {
  try { localStorage.setItem(key, val) } catch { /* private mode */ }
}

export function useSettings () {
  const theme = useState<'dark' | 'light'>('xd-theme', () => 'dark')
  const fontSize = useState<string>('xd-font-size', () => '15px')
  const fontFamily = useState<string>('xd-font-family', () => '')
  const webhookUrl = useState<string>('xd-webhook-url', () => '')
  const webhookKey = useState<string>('xd-webhook-key', () => '')

  function applyTheme () {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.theme = theme.value
    document.documentElement.style.setProperty('--font-size-base', fontSize.value)
    if (fontFamily.value) document.documentElement.style.setProperty('--font', fontFamily.value)
  }
  function load () {
    if (typeof localStorage === 'undefined') return
    theme.value = read(LS.theme) === 'light' ? 'light' : 'dark'
    fontSize.value = read(LS.fontSize) || '15px'
    fontFamily.value = read(LS.fontFamily) || ''
    webhookUrl.value = read(LS.webhookUrl) || ''
    webhookKey.value = read(LS.webhookKey) || ''
    applyTheme()
  }
  function setTheme (t: 'dark' | 'light') { theme.value = t; write(LS.theme, t); applyTheme() }
  function setFontSize (v: string) { fontSize.value = v; write(LS.fontSize, v); applyTheme() }
  function setFontFamily (v: string) { fontFamily.value = v; write(LS.fontFamily, v); applyTheme() }
  function saveWebhook (url: string, key: string) {
    webhookUrl.value = url; webhookKey.value = key
    write(LS.webhookUrl, url); write(LS.webhookKey, key)
  }
  function clearWebhook () { saveWebhook('', '') }

  // Webhook trigger: SPEC behaviour — POST {} , Bearer key (no double Bearer),
  // cors -> no-cors -> sendBeacon fallback. Webhook lives ONLY in localStorage.
  function authHeader (key: string): string {
    return /^Bearer\s/i.test(key) ? key : `Bearer ${key}`
  }
  async function postWebhook (): Promise<{ ok: boolean, via: string, status?: number }> {
    const url = webhookUrl.value
    const key = webhookKey.value
    if (!url || !key) throw new Error(t('settings.webhookNotConfigured'))
    const headers: Record<string, string> = { Authorization: authHeader(key), 'Content-Type': 'application/json' }
    try {
      const res = await fetch(url, { method: 'POST', headers, body: '{}', mode: 'cors', credentials: 'omit' })
      return { ok: true, via: 'cors', status: res.status }
    } catch (e) {
      try {
        await fetch(url, { method: 'POST', headers, body: '{}', mode: 'no-cors', credentials: 'omit' })
        return { ok: true, via: 'no-cors' }
      } catch { /* fall through */ }
      try {
        if (navigator.sendBeacon) {
          const sent = navigator.sendBeacon(url, new Blob(['{}'], { type: 'text/plain' }))
          if (sent) return { ok: true, via: 'beacon' }
        }
      } catch { /* fall through */ }
      throw e
    }
  }
  return { theme, fontSize, fontFamily, webhookUrl, webhookKey, load, applyTheme, setTheme, setFontSize, setFontFamily, saveWebhook, clearWebhook, postWebhook }
}
