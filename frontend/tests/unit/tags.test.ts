import { describe, expect, it } from 'vitest'
import { cloudSizes, duplicateGroups, normalizeTag } from '~/utils/tags'

describe('tags', () => {
  it('normalizes', () => {
    expect(normalizeTag('  ＡＩ　工具 ')).toBe('AI 工具')
  })
  it('groups case / width variants, most used first', () => {
    const g = duplicateGroups([{ tag: 'Workbuddy', n: 2 }, { tag: 'workbuddy', n: 7 }, { tag: 'ｗｏｒｋｂｕｄｄｙ', n: 0 }, { tag: 'AI', n: 3 }])
    expect(g).toEqual([[{ tag: 'workbuddy', n: 7 }, { tag: 'Workbuddy', n: 2 }, { tag: 'ｗｏｒｋｂｕｄｄｙ', n: 0 }]])
  })
  it('sizes the tag cloud by usage', () => {
    const c = cloudSizes([{ tag: 'a', n: 1 }, { tag: 'b', n: 10 }, { tag: 'c', n: 100 }])
    expect(c.map(x => x.size)).toEqual([12, 19, 26])
    expect(cloudSizes([{ tag: 'x', n: 3 }])[0]?.size).toBe(19)
    expect(cloudSizes([])).toEqual([])
  })
})
