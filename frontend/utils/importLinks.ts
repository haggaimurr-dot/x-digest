// Batch "导入单帖": split pasted text into post links (one per line or
// separated by spaces/commas), drop duplicates (same status id), and flag
// what is not an x.com / twitter.com status link or a bare post id. Mirrors
// backend xpost.ParseStatusURL, so a link accepted here is accepted there.
const STATUS_RE = /(?:x|twitter|fxtwitter|vxtwitter|fixupx)\.com\/(?:[A-Za-z0-9_]{1,15}|i(?:\/web)?)\/status(?:es)?\/(\d+)/i
const BARE_ID_RE = /^\d{5,25}$/

export interface ParsedLinks {
  posts: Array<{ url: string, id: string }>
  duplicates: string[]
  invalid: string[]
}

export function parseImportLinks (text: string): ParsedLinks {
  const out: ParsedLinks = { posts: [], duplicates: [], invalid: [] }
  const seen = new Set<string>()
  for (const raw of text.split(/[\s,，]+/)) {
    const s = raw.trim()
    if (!s) continue
    const id = s.match(STATUS_RE)?.[1] || (BARE_ID_RE.test(s) ? s : '')
    if (!id) { out.invalid.push(s); continue }
    if (seen.has(id)) { out.duplicates.push(s); continue }
    seen.add(id)
    out.posts.push({ url: s, id })
  }
  return out
}
