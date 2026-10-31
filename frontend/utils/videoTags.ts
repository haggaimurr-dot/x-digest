// Video topic tags and description resources for the video pages
// (YouTube / Bilibili, 2026-10-27). Both run on title + description in
// the browser: no model call, instant, and re-computed whenever the text
// changes. Tags drive the "标签" filter; resources are the links a
// description lists (with the text before each link as its label).

// Ordered: more specific names first. `skipIf` drops a broader tag when a
// more specific one already matched (Claude Code → no separate Claude).
const TAG_RULES: Array<{ tag: string, re: RegExp, skipIf?: string }> = [
  { tag: 'Claude Code', re: /claude[\s-]?code|claudecode/i },
  { tag: 'Claude', re: /\bclaude\b|anthropic/i, skipIf: 'Claude Code' },
  { tag: 'Codex', re: /\bcodex\b/i },
  { tag: 'Gemini', re: /\bgemini\b/i },
  { tag: 'ChatGPT', re: /chat\s?gpt|\bgpt-?[345o]|\bopenai\b/i },
  { tag: 'Cursor', re: /\bCursor\b/ },
  { tag: 'Copilot', re: /copilot/i },
  { tag: 'Windsurf', re: /windsurf/i },
  { tag: 'DeepSeek', re: /deep\s?seek/i },
  { tag: 'Qwen', re: /\bqwen|通义千问/i },
  { tag: 'Kimi', re: /\bkimi\b/i },
  { tag: 'Grok', re: /\bgrok\b/i },
  { tag: 'MCP', re: /\bMCP\b|model context protocol/i },
  { tag: 'n8n', re: /\bn8n\b/i },
  { tag: 'Agent', re: /\bai agents?\b|\bagentic\b|智能体/i },
  { tag: 'Midjourney', re: /midjourney/i },
  { tag: 'Sora', re: /\bsora\b/i },
  { tag: 'Veo', re: /\bveo\s?\d?\b/i },
  { tag: 'Lovable', re: /\blovable\b/i },
  { tag: 'Obsidian', re: /\bobsidian\b/i },
  { tag: 'Notion', re: /\bnotion\b/i }
]

export function videoTags (title: string, description: string): string[] {
  const text = `${title}\n${description || ''}`
  const out: string[] = []
  for (const r of TAG_RULES) {
    if (r.skipIf && out.includes(r.skipIf)) continue
    if (r.re.test(text)) out.push(r.tag)
  }
  return out
}

// ---- links in descriptions ----

// http(s) links; trailing punctuation (incl. full-width) is not part of
// the link.
const URL_RE = /https?:\/\/[^\s<>"'，。；！？、）】」]+/g
const TRAIL_RE = /[.,;:!?)\]}'"]+$/

export interface TextPart { text: string, href?: string }

// Splits text into plain parts and link parts (rendered without v-html).
export function linkParts (text: string): TextPart[] {
  const parts: TextPart[] = []
  let last = 0
  for (const m of text.matchAll(URL_RE)) {
    let url = m[0]
    const trail = url.match(TRAIL_RE)
    if (trail) url = url.slice(0, -trail[0].length)
    const start = m.index!
    if (start > last) parts.push({ text: text.slice(last, start) })
    parts.push({ text: url, href: url })
    last = start + url.length
  }
  if (last < text.length) parts.push({ text: text.slice(last) })
  return parts
}

export interface DescResource { label: string, url: string }

// Social profiles / sponsor plumbing are not "resources".
const SKIP_HOSTS = /(^|\.)(instagram\.com|tiktok\.com|facebook\.com|twitter\.com|x\.com|threads\.net|linkedin\.com|patreon\.com|weibo\.com|douyin\.com)$/i
const SKIP_PATHS = /youtube\.com\/(@|channel\/|c\/|user\/)|space\.bilibili\.com\//i

// The links a description lists, labelled by the text before the link on
// the same line ("✅ 免费模组下载：https://…" → "免费模组下载"), else by
// host. De-duplicated, at most `max`.
export function descResources (description: string, max = 8): DescResource[] {
  const out: DescResource[] = []
  const seen = new Set<string>()
  for (const line of (description || '').split('\n')) {
    for (const part of linkParts(line)) {
      if (!part.href || seen.has(part.href)) continue
      let host = ''
      try { host = new URL(part.href).hostname.replace(/^www\./, '') } catch { continue }
      if (SKIP_HOSTS.test(host) || SKIP_PATHS.test(part.href)) continue
      seen.add(part.href)
      const before = line.slice(0, line.indexOf(part.href))
        .replace(/https?:\/\/\S+/g, '')
        .replace(/[\p{Extended_Pictographic}️►▶→•·*|#>\-–—_]+/gu, ' ')
        .replace(/[：:]\s*$/, '').trim()
      out.push({ label: before.length >= 2 && before.length <= 40 ? before : host, url: part.href })
      if (out.length >= max) return out
    }
  }
  return out
}
