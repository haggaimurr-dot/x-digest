import { describe, expect, it } from 'vitest'
import { normalizeFilter, sameFilter } from '~/utils/filterView'

describe('filterView', () => {
  it('fills missing fields and drops junk', () => {
    expect(normalizeFilter({ tags: ['AI', 3], keyword: 7, extra: true })).toEqual({
      keyword: '', date_from: '', date_to: '', kind: '', authors: [], tags: ['AI'], user_tags: [], categories: []
    })
    expect(normalizeFilter(null).authors).toEqual([])
  })
  it('round-trips through JSON', () => {
    const f = normalizeFilter({ keyword: 'claude', authors: ['a'], categories: ['__uncat'], date_from: '2026-10-01' })
    expect(sameFilter(f, normalizeFilter(JSON.parse(JSON.stringify(f))))).toBe(true)
    expect(sameFilter(f, { ...f, keyword: 'x' })).toBe(false)
  })
})
