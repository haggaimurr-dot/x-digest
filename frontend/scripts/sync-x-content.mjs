// Build a compact ID join for both API and demo posts without shipping the raw archive.
import { readFile, writeFile } from 'node:fs/promises'
const read = async path => JSON.parse(await readFile(new URL(path, import.meta.url), 'utf8'))
const raw = await read('../../data/raw_posts.json')
const live = await read('../../data/posts.json')
const demo = await read('../assets/mock/posts.json')
const ids = new Set([...live.posts, ...demo.posts].map(p => String(p.id)))
const content = {}
for (const p of raw) {
  if (!ids.has(String(p.id))) continue
  const text = [p.long_text, p.note_tweet, p.text].map(v => typeof v === 'string' ? v : v?.text).find(v => v?.trim()) || ''
  const links = [...new Set((text.match(/https?:\/\/[^\s<>"“”]+/gu) || []).map(u => u.replace(/[，。！？、；：）】》]+$/u, '')))]
  content[p.id] = { text, links }
}
await writeFile(new URL('../assets/mock/x-content.json', import.meta.url), JSON.stringify(content, null, 2) + '\n')
console.log(`Joined original text/resources for ${Object.keys(content).length} posts`)
