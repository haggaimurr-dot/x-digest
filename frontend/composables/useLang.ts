// Language toggle: 中文 / EN, default Simplified Chinese. The choice is
// persisted in localStorage (LS.lang) and applied without reload; templates
// re-render because t() reads the reactive lang ref. The module-level
// language in utils/i18n (used by plain modules via t()) is kept in sync.
import { setCurrentLang, translate, type Lang } from '~/utils/i18n'
import { LS } from './useSettings'

export function useLang () {
  const lang = useState<Lang>('xd-lang', () => 'zh-CN')
  const isEn = computed(() => lang.value === 'en')

  function applyLang (l: Lang) {
    if (typeof document !== 'undefined') document.documentElement.lang = l
  }
  function setLang (l: Lang) {
    lang.value = l
    setCurrentLang(l)
    if (import.meta.client) {
      try { localStorage.setItem(LS.lang, l) } catch { /* private mode */ }
    }
    applyLang(l)
  }
  // Read the saved choice on mount (useState is seeded zh-CN from the
  // server payload, same as the theme in useSettings).
  function loadLang () {
    if (!import.meta.client) return
    try {
      const saved = localStorage.getItem(LS.lang)
      if (saved === 'en' || saved === 'zh-CN') setLang(saved)
      else applyLang(lang.value)
    } catch { applyLang(lang.value) }
  }
  function toggleLang () { setLang(lang.value === 'en' ? 'zh-CN' : 'en') }
  function t (key: string, params?: Record<string, string | number>): string {
    return translate(key, lang.value, params)
  }
  return { lang, isEn, setLang, loadLang, toggleLang, t }
}
