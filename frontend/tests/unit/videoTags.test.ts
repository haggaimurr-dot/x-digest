import { describe, expect, it } from 'vitest'
import { descResources, linkParts, videoTags } from '~/utils/videoTags'

describe('videoTags', () => {
  it('specific names win over broad ones', () => {
    expect(videoTags('Claude Code 实战', '')).toEqual(['Claude Code'])
    expect(videoTags('Anthropic Claude 新模型', '')).toEqual(['Claude'])
    expect(videoTags('Cursor vs Copilot', 'with MCP and n8n')).toEqual(['Cursor', 'Copilot', 'MCP', 'n8n'])
    expect(videoTags('今天聊聊做饭', '')).toEqual([])
  })
  it('splits links without trailing punctuation', () => {
    expect(linkParts('看 https://a.com/x）。完')).toEqual([{ text: '看 ' }, { text: 'https://a.com/x', href: 'https://a.com/x' }, { text: '）。完' }])
    expect(linkParts('end https://b.io.')).toEqual([{ text: 'end ' }, { text: 'https://b.io', href: 'https://b.io' }, { text: '.' }])
  })
  it('lists description resources, skipping social profiles', () => {
    const desc = '✅ 免费模组下载：https://mods.example.com/dl\n关注我 https://x.com/me\nhttps://github.com/o/r\nhttps://github.com/o/r'
    expect(descResources(desc)).toEqual([
      { label: '免费模组下载', url: 'https://mods.example.com/dl' },
      { label: 'github.com', url: 'https://github.com/o/r' }
    ])
    expect(descResources('https://a.com https://b.com https://c.com', 2)).toHaveLength(2)
  })
})
