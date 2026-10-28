import { describe, expect, it } from 'vitest'
import { marked } from 'marked'
import { safeHtml } from '~/utils/sanitize'

describe('safeHtml', () => {
  it('drops event handlers and scripts', () => {
    const out = safeHtml(marked.parse('<img src=x onerror=alert(1)> <script>alert(2)</script> **ok**') as string)
    expect(out).not.toMatch(/onerror|<script/i)
    expect(out).toContain('<strong>ok</strong>')
  })
  it('drops javascript: links', () => {
    expect(safeHtml('<a href="javascript:alert(1)">x</a>')).not.toContain('javascript:')
  })
  it('keeps normal markdown (tables, code)', () => {
    const out = safeHtml(marked.parse('| a | b |\n|---|---|\n| 1 | 2 |\n\n```js\nconst x = 1\n```') as string)
    expect(out).toContain('<table>')
    expect(out).toContain('<code class="language-js">')
  })
})
