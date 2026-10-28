import { describe, expect, it } from 'vitest'
import { avatarColor, avatarInitial, avatarUrl, deriveShowFlowchart, escapeHtml, firstHitSnippet, highlightHtml, shanghaiDayBound, shanghaiTime, shouldShowFlowchart, trunc40 } from '~/composables/useFormat'
import type { Post } from '~/composables/types'

const post = (p: Partial<Post>): Post => ({ id: '1', url: '', author: 'alice', author_name: 'Alice', author_avatar: '', industry: '', tech: '', title: '', summary: '', note: '', published_at: '2026-10-01T00:00:00+08:00', is_long: false, tags: [], steps: [], ...p } as Post)

describe('useFormat', () => {
  it('avatars', () => {
    expect(avatarUrl('avatars/a.jpg')).toBe('/avatars/a.jpg')
    expect(avatarUrl('https://x/a.jpg')).toBe('https://x/a.jpg')
    expect(avatarUrl('')).toBe('')
    expect(avatarColor('alice')).toMatch(/^#[0-9a-f]{6}$/)
    expect(avatarColor('alice')).toBe(avatarColor('alice'))
    expect(avatarInitial(' 张三')).toBe('张')
    expect(avatarInitial('')).toBe('?')
  })
  it('Shanghai time and day bounds', () => {
    expect(shanghaiTime('2026-10-01T16:30:00Z')).toBe('2026-10-02 00:30')
    expect(shanghaiTime('')).toBe('—')
    expect(shanghaiTime('nope')).toBe('nope')
    expect(shanghaiDayBound('2026-10-02', false)).toBe(Date.parse('2026-10-01T16:00:00Z'))
    expect(shanghaiDayBound('2026-10-02', true)).toBe(Date.parse('2026-10-02T16:00:00Z') - 1)
    expect(shanghaiDayBound('', false)).toBeNull()
  })
  it('escapes before highlighting (no HTML injection)', () => {
    expect(escapeHtml('<b>"x"&')).toBe('&lt;b&gt;&quot;x&quot;&amp;')
    const h = highlightHtml('<script>alert(1)</script> Claude', 'claude')
    expect(h).not.toContain('<script>')
    expect(h).toContain('<mark class="search-hit">Claude</mark>')
    expect(highlightHtml('a<b', '')).toBe('a&lt;b')
  })
  it('first-hit snippet', () => {
    const p = post({ title: '无关', text: 'x'.repeat(60) + 'DeepSeek' + 'y'.repeat(80) })
    const s = firstHitSnippet(p, 'deepseek')
    expect(s.startsWith('…')).toBe(true)
    expect(s.endsWith('…')).toBe(true)
    expect(s).toContain('<mark class="search-hit">DeepSeek</mark>')
    expect(firstHitSnippet(p, '')).toBe('')
    expect(firstHitSnippet(p, 'nothing')).toBe('')
  })
  it('flowchart eligibility', () => {
    const tut = post({ title: 'Claude Code 安装教程', steps: [{ text: '安装 Claude Code' }, { text: '运行命令' }] as any })
    expect(deriveShowFlowchart(tut)).toBe(true)
    expect(deriveShowFlowchart(post({ title: '闲聊', steps: [{ text: 'a' }] as any }))).toBe(false)
    expect(shouldShowFlowchart({ ...tut, show_flowchart: false })).toBe(false)
    expect(shouldShowFlowchart(tut)).toBe(true)
  })
  it('trunc40', () => {
    expect(trunc40('a'.repeat(41))).toBe('a'.repeat(40) + '…')
    expect(trunc40('short')).toBe('short')
  })
})
