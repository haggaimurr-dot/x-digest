import { describe, expect, it } from 'vitest'
import { normalizeFilter, sameFilter } from '~/utils/filterView'

describe('filterView', () => {
  it('fills missing fields and drops junk', () => {
    expect(normalizeFilter({ tags: ['AI', 3], keyword: 7, min_chars: -5, extra: true })).toEqual({
      keyword: '', kind: '', min_chars: 0, authors: [], tags: ['AI'], categories: []
    })
    expect(normalizeFilter(null).authors).toEqual([])
  })
  it('drops removed fields of old views (date range, custom tags)', () => {
    const f = normalizeFilter({ keyword: 'a', date_from: '2026-10-01', date_to: '2026-10-02', user_tags: ['x'], min_chars: 100 })
    expect(f).toEqual({ keyword: 'a', kind: '', min_chars: 100, authors: [], tags: [], categories: [] })
  })
  it('round-trips through JSON', () => {
    const f = normalizeFilter({ keyword: 'claude', authors: ['a'], categories: ['__uncat'], min_chars: 200 })
    expect(sameFilter(f, normalizeFilter(JSON.parse(JSON.stringify(f))))).toBe(true)
    expect(sameFilter(f, { ...f, keyword: 'x' })).toBe(false)
    expect(sameFilter(f, { ...f, min_chars: 0 })).toBe(false)
  })
})
