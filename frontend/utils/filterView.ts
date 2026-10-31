// Serialised filter state of the X daily page, as stored in saved views
// (backend /api/v1/views, filter_json). Unknown / missing fields read as
// "no filter", so views saved by an older page still load; fields the page
// no longer has (date_from / date_to / user_tags) are dropped.
export interface FilterState {
  keyword: string, kind: string, min_chars: number,
  authors: string[], tags: string[], categories: string[]
}

const strs = (v: unknown): string[] => Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
const str = (v: unknown): string => typeof v === 'string' ? v : ''
const num = (v: unknown): number => typeof v === 'number' && Number.isFinite(v) && v > 0 ? Math.floor(v) : 0

export function normalizeFilter (raw: unknown): FilterState {
  const o = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    keyword: str(o.keyword), kind: str(o.kind), min_chars: num(o.min_chars),
    authors: strs(o.authors), tags: strs(o.tags), categories: strs(o.categories)
  }
}

export function sameFilter (a: FilterState, b: FilterState): boolean {
  return JSON.stringify(normalizeFilter(a)) === JSON.stringify(normalizeFilter(b))
}
