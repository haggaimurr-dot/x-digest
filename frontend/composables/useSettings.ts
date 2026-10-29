// localStorage keys — kept identical to the original site (SPEC) so old
// browser data stays readable. Theme/font stay local even in API mode.
export const LS = {
  watchlist: 'x-digest-watchlist',
  postCategories: 'x-digest-post-categories',
  selectedAuthors: 'x-digest-selected-authors',
  selectedCats: 'x-digest-selected-cats',
  theme: 'x-digest-theme',
  fontFamily: 'x-digest-font-family',
  fontSize: 'x-digest-font-size',
  lang: 'xd-lang',
  chipsAuthorsExpanded: 'x-digest-chips-authors-expanded',
  chipsTagsExpanded: 'x-digest-chips-tags-expanded',
  chipsCatsExpanded: 'x-digest-chips-cats-expanded',
  filtersCollapsed: 'x-digest-filters-collapsed',
  replicas: 'xdigest.replicas', // mock-mode only; API mode uses the server
  apiToken: 'xdigest.apiToken' // API_TOKEN for write requests (see useApi)
}

// Keys of features that are gone; load() deletes them so old browsers do
// not keep stale secrets. Webhook「立即更新」was retired 2026-10-29 (the
// page's 更新帖子 replaces it).
export const LEGACY_LS = ['x-digest-webhook-url', 'x-digest-webhook-key']

export function clearLegacySettings (storage: Pick<Storage, 'removeItem'>) {
  for (const k of LEGACY_LS) {
    try { storage.removeItem(k) } catch { /* private mode */ }
  }
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
    clearLegacySettings(localStorage)
    applyTheme()
  }
  function setTheme (t: 'dark' | 'light') { theme.value = t; write(LS.theme, t); applyTheme() }
  function setFontSize (v: string) { fontSize.value = v; write(LS.fontSize, v); applyTheme() }
  function setFontFamily (v: string) { fontFamily.value = v; write(LS.fontFamily, v); applyTheme() }
  return { theme, fontSize, fontFamily, load, applyTheme, setTheme, setFontSize, setFontFamily }
}
