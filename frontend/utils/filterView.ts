// Serialised filter state of the X daily page, as stored in saved views
// (backend /api/v1/views, filter_json). Unknown / missing fields read as
// "no filter", so views saved by an older page still load.
export interface FilterState {
  keyword: string, date_from: string, date_to: string, kind: string,
  authors: string[], tags: string[], user_tags: string[], categories: string[]
}

const strs = (v: unknown): string[] => Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
const str = (v: unknown): string => typeof v === 'string' ? v : ''

export function normalizeFilter (raw: unknown): FilterState {
  const o = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    keyword: str(o.keyword), date_from: str(o.date_from), date_to: str(o.date_to), kind: str(o.kind),
    authors: strs(o.authors), tags: strs(o.tags), user_tags: strs(o.user_tags), categories: strs(o.categories)
  }
}

export function sameFilter (a: FilterState, b: FilterState): boolean {
  return JSON.stringify(normalizeFilter(a)) === JSON.stringify(normalizeFilter(b))
}
