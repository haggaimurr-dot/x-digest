import type { YTSummary } from '~/composables/types'

// Follows one pending AI summary over server-sent events
// (GET …/summary/stream, backend summary_stream.go): the server pushes the
// summary when the job ends, so there is one connection instead of a GET
// every 4 seconds. Returns false when EventSource is unavailable; when the
// stream errors before a final summary arrives, onFail() runs once (the
// caller falls back to polling).
export function followSummary (url: string, onSummary: (s: YTSummary) => void, onFail: () => void): boolean {
  if (typeof window === 'undefined' || typeof EventSource === 'undefined') return false
  const es = new EventSource(url)
  let finished = false
  es.addEventListener('summary', (ev) => {
    try {
      const s = JSON.parse((ev as MessageEvent).data).summary as YTSummary
      onSummary(s)
      if (s.status !== 'pending') { finished = true; es.close() }
    } catch { /* ignore a malformed event */ }
  })
  es.onerror = () => {
    es.close()
    if (!finished) { finished = true; onFail() }
  }
  return true
}
