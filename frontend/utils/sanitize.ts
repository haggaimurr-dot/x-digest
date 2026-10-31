import DOMPurify from 'dompurify'

// Every HTML string that ends up in v-html (marked output, mammoth output,
// keyword highlights) goes through this. On the server (SSR) there is no DOM,
// so tags are stripped instead; the client re-renders with the sanitized HTML.
export function safeHtml (html: string): string {
  if (typeof window === 'undefined') return html.replace(/<[^>]*>/g, '')
  return DOMPurify.sanitize(html, { USE_PROFILES: { html: true } })
}
