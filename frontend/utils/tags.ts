// User-tag helpers (mirror backend internal/store/user_tags.go).

// normalizeTag: trim, full-width ASCII → half-width, collapse whitespace.
export function normalizeTag (t: string): string {
  return t.replace(/[！-～]/g, ch => String.fromCharCode(ch.charCodeAt(0) - 0xFEE0))
    .replace(/　/g, ' ').split(/\s+/).filter(Boolean).join(' ')
}

// duplicateGroups: tags whose normalized, lower-cased form collides (疑似重复),
// each group sorted by use count desc; the first entry is the merge target.
export function duplicateGroups (rows: Array<{ tag: string, n: number }>): Array<Array<{ tag: string, n: number }>> {
  const by = new Map<string, Array<{ tag: string, n: number }>>()
  for (const r of rows) {
    const k = normalizeTag(r.tag).toLowerCase()
    by.set(k, [...(by.get(k) || []), r])
  }
  return [...by.values()].filter(g => g.length > 1).map(g => [...g].sort((a, b) => b.n - a.n || a.tag.localeCompare(b.tag)))
}

// cloudSizes: font size per tag for the tag cloud, 12px (least used) to
// 26px (most used), on a log scale so one huge tag does not flatten the rest.
export function cloudSizes (rows: Array<{ tag: string, n: number }>, min = 12, max = 26): Array<{ tag: string, n: number, size: number }> {
  if (!rows.length) return []
  const logs = rows.map(r => Math.log(Math.max(1, r.n)))
  const lo = Math.min(...logs)
  const hi = Math.max(...logs)
  return rows.map((r, i) => ({ ...r, size: Math.round(hi === lo ? (min + max) / 2 : min + (max - min) * (logs[i]! - lo) / (hi - lo)) }))
}
