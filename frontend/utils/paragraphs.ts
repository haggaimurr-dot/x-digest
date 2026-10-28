// Paragraphs for the 原文/译文对照 view: split on blank lines (or single
// line breaks when there are no blank lines), trim, drop empties. Very long
// paragraphs are cut at sentence ends so each translation request stays
// small (the translate API takes ≤ 20 texts per call).
const MAX_PARA = 600

export function splitParagraphs (text: string): string[] {
  const t = (text || '').replace(/\r\n?/g, '\n').trim()
  if (!t) return []
  const blocks = /\n\s*\n/.test(t) ? t.split(/\n\s*\n/) : t.split('\n')
  const out: string[] = []
  for (const b of blocks.map(x => x.trim()).filter(Boolean)) {
    if (b.length <= MAX_PARA) { out.push(b); continue }
    let cur = ''
    for (const sentence of b.match(/[^。！？.!?]+[。！？.!?]*\s*/g) || [b]) {
      if (cur && cur.length + sentence.length > MAX_PARA) { out.push(cur.trim()); cur = '' }
      cur += sentence
    }
    if (cur.trim()) out.push(cur.trim())
  }
  return out
}
