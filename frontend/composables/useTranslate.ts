// On-demand content translation (DeepSeek relay via POST /api/v1/translate).
//
// Shared module-level queue + reactive cache: every page/component that calls
// useTranslate() sees the same translations. Pages enqueue the texts they
// currently display (filtered list / expanded detail / visible nodes);
// the queue flushes in chunks of 20, sequentially, in the background.
// Failures fall back to the original text silently — never a toast, never
// blank. Switching back to zh-CN needs no API call at all.
//
// Texts longer than MAX_TEXT_CHARS are left untranslated (a single huge
// release body is not worth a giant chat call). Filtering always uses the
// original text; only display goes through tr().
const MAX_TEXT_CHARS = 12000

const cache = reactive(new Map<string, string>())
const queue: string[] = []
let flushing = false
const pendingCount = ref(0)

function cacheKey (source: string): string { return `en\x00${source}` }

export function useTranslate () {
  const store = useStore()
  const { isEn } = useLang()
  const translating = computed(() => pendingCount.value > 0)

  // Translated display text, or the original when zh-CN / not yet translated
  // / translation failed. Reads the reactive cache, so the component
  // re-renders when a translation lands.
  function tr (text: string): string {
    if (!text || !isEn.value) return text
    return cache.get(cacheKey(text)) ?? text
  }

  function enqueue (texts: string[]) {
    if (!import.meta.client || !isEn.value) return
    let added = false
    for (const s of texts) {
      if (!s || !s.trim()) continue
      if (s.length > MAX_TEXT_CHARS) continue
      if (cache.has(cacheKey(s)) || queue.includes(s)) continue
      queue.push(s)
      added = true
    }
    if (added) void flush()
  }

  async function flush () {
    if (flushing) return
    flushing = true
    try {
      while (queue.length) {
        const chunk = [...new Set(queue.splice(0, 20))].filter(s => !cache.has(cacheKey(s)))
        if (!chunk.length) continue
        pendingCount.value += chunk.length
        try {
          const out = await store.translateTexts(chunk, 'en')
          chunk.forEach((s, i) => { if (out[i]) cache.set(cacheKey(s), out[i]) })
        } catch {
          // Leave uncached: tr() keeps showing the original. A later
          // enqueue (filter change / lang toggle) retries.
        } finally {
          pendingCount.value -= chunk.length
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
