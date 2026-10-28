import { describe, expect, it } from 'vitest'
import { embedUrl, seekMessage } from '~/utils/embedPlayer'

describe('embedPlayer', () => {
  it('YouTube starts at the chapter and allows JS seeking', () => {
    expect(embedUrl('yt', 'dQw4w9WgXcQ', 330.7)).toBe('https://www.youtube.com/embed/dQw4w9WgXcQ?enablejsapi=1&autoplay=1&start=330')
  })
  it('Bilibili reloads with t=', () => {
    expect(embedUrl('bili', 'BV1xx411c7mD', 330)).toBe('https://player.bilibili.com/player.html?bvid=BV1xx411c7mD&t=330&autoplay=1&high_quality=1')
    expect(embedUrl('bili', 'av170001', 0)).toContain('aid=170001&t=0')
  })
  it('seek command for 05:30', () => {
    expect(JSON.parse(seekMessage(330))).toEqual({ event: 'command', func: 'seekTo', args: [330, true] })
  })
})
