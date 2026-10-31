// On-demand content translation (DeepSeek relay via POST /api/v1/translate).
//
// Shared module-level queue + reactive cache: every page/component that calls
// useTranslate() sees the same translations. Pages enqueue the texts they
// currently display (filtered list / expanded detail / visible nodes);
// the queue flushes in chunks of 20, sequentially, in the background.
// Failures fall back to the original text silently — never a toast, never
// blank.
//
// Translation runs TOWARD the current UI language, in both directions:
// Chinese UI translates English content to Chinese, English UI translates
// Chinese content to English. That matters because the content itself is
// mixed-language: digest posts are Chinese, GitHub repo details are English.
// Texts already in the target language are skipped without any API call
// (cheap CJK check up front; the backend short-circuits them too).
// Translations are cached per (source, target), so toggling back and forth
// never re-translates. Filtering always uses the original text; only
// display goes through tr().
//
// Texts longer than MAX_TEXT_CHARS are left untranslated (a single huge
// release body is not worth a giant chat call).
import type { Lang } from '~/utils/i18n'

const MAX_TEXT_CHARS = 12000

const cache = reactive(new Map<string, string>())
const queue: Array<{ s: string, target: Lang }> = []
let flushing = false
const pendingCount = ref(0)

function cacheKey (target: Lang, source: string): string { return `${target}\x00${source}` }
function hasCJK (s: string): boolean { return /[\u4e00-\u9fff]/.test(s) }
function hasKana (s: string): boolean { return /[\u3040-\u309f\u30a0-\u30ff]/.test(s) }
function hasHangul (s: string): boolean { return /[\uac00-\ud7af]/.test(s) }
// A text needs translation only when it is (mostly) in the other language.
// Japanese (kana) and Korean (hangul) also use CJK kanji, so they must be
// detected separately when the target is Chinese.
function needsTranslation (s: string, target: Lang): boolean {
  if (target === 'en') return hasCJK(s) || hasKana(s) || hasHangul(s)
  if (hasKana(s) || hasHangul(s)) return true
  return !hasCJK(s)
}

export function useTranslate () {
  const store = useStore()
  const { lang } = useLang()
  const translating = computed(() => pendingCount.value > 0)

  // Translated display text, or the original when not yet translated /
  // translation failed / already in the target language. Reads the reactive
  // cache, so the component re-renders when a translation lands.
  function tr (text: string): string {
    if (!text) return text
    return cache.get(cacheKey(lang.value, text)) ?? text
  }

  function enqueue (texts: string[]) {
    if (!import.meta.client) return
    const target = lang.value
    let added = false
    for (const s of texts) {
      if (!s || !s.trim()) continue
      if (s.length > MAX_TEXT_CHARS) continue
      if (!needsTranslation(s, target)) continue
      if (cache.has(cacheKey(target, s))) continue
      if (queue.some(q => q.s === s && q.target === target)) continue
      queue.push({ s, target })
      added = true
    }
    if (added) void flush()
  }

  async function flush () {
    if (flushing) return
    flushing = true
    try {
      while (queue.length) {
        // One chunk = up to 20 texts sharing the target language (the user
        // may have toggled mid-queue; each chunk uses its own target).
        const target = queue[0].target
        const idxs: number[] = []
        for (let i = 0; i < queue.length && idxs.length < 20; i++) {
          if (queue[i].target === target) idxs.push(i)
        }
        const chunk = idxs.map(i => queue[i])
        for (let i = idxs.length - 1; i >= 0; i--) queue.splice(idxs[i], 1)
        const texts = chunk.map(c => c.s).filter(s => !cache.has(cacheKey(target, s)))
        if (!texts.length) continue
        pendingCount.value += texts.length
        try {
          const out = await store.translateTexts(texts, target)
          texts.forEach((s, i) => { if (out[i]) cache.set(cacheKey(target, s), out[i]) })
        } catch {
          // Leave uncached: tr() keeps showing the original. A later
          // enqueue (filter change / lang toggle) retries.
        } finally {
          pendingCount.value -= texts.length
        }
      }
    } finally {
      flushing = false
      // Items enqueued while the last chunk was in flight.
      if (queue.length) void flush()
    }
  }

  return { tr, enqueue, translating }
}
