import { describe, expect, it } from 'vitest'
import { parseImportLinks } from '~/utils/importLinks'

describe('parseImportLinks', () => {
  it('3 valid + 1 duplicate + 1 invalid', () => {
    const r = parseImportLinks(`https://x.com/alice/status/1850000000000000001
twitter.com/bob/status/1850000000000000002?s=20
1850000000000000003
https://x.com/alice/status/1850000000000000001/photo/1
https://example.com/not-a-post`)
    expect(r.posts.map(p => p.id)).toEqual(['1850000000000000001', '1850000000000000002', '1850000000000000003'])
    expect(r.duplicates).toEqual(['https://x.com/alice/status/1850000000000000001/photo/1'])
    expect(r.invalid).toEqual(['https://example.com/not-a-post'])
  })
  it('accepts the same hosts as the backend', () => {
    expect(parseImportLinks('https://fxtwitter.com/a/status/123456 https://x.com/i/web/status/654321').posts.map(p => p.id)).toEqual(['123456', '654321'])
  })
  it('accepts commas and spaces as separators', () => {
    expect(parseImportLinks('1850000000000000001，1850000000000000002 , x').posts).toHaveLength(2)
  })
})
