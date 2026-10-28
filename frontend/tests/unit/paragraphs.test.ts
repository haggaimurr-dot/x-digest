import { describe, expect, it } from 'vitest'
import { splitParagraphs } from '~/utils/paragraphs'

describe('splitParagraphs', () => {
  it('splits on blank lines first', () => {
    expect(splitParagraphs('First line\nstill first\n\n  Second  \r\n\r\nThird')).toEqual(['First line\nstill first', 'Second', 'Third'])
  })
  it('falls back to single line breaks', () => {
    expect(splitParagraphs('a\nb\n\n')).toEqual(['a', 'b'])
    expect(splitParagraphs('')).toEqual([])
  })
  it('cuts very long paragraphs at sentence ends', () => {
    const long = 'This is a sentence. '.repeat(60)
    const parts = splitParagraphs(long)
    expect(parts.length).toBeGreaterThan(1)
    expect(parts.every(p => p.length <= 600)).toBe(true)
    expect(parts.join(' ').replace(/\s+/g, ' ')).toBe(long.trim().replace(/\s+/g, ' '))
  })
})
