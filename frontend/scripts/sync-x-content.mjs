// Build a compact ID join for both API and demo posts without shipping the raw archive.
import { readFile, writeFile } from 'node:fs/promises'
const read = async path => JSON.parse(await readFile(new URL(path, import.meta.url), 'utf8'))
import { rawAvatar } from '../utils/x-post.mjs'
const avatars = await read('../../data/avatars.json')
const watchlist = await read('../../config/watchlist.json')
const raw = await read('../../data/raw_posts.json')
const live = await read('../../data/posts.json')
const demo = await read('../assets/mock/posts.json')
const ids = new Set([...live.posts, ...demo.posts].map(p => String(p.id)))
const content = {}
for (const p of raw) {
  if (!ids.has(String(p.id))) continue
  const text = [p.long_text, p.note_tweet, p.text].map(v => typeof v === 'string' ? v : v?.text).find(v => v?.trim()) || ''
  const links = [...new Set((text.match(/https?:\/\/[^\s<>"“”]+/gu) || []).map(u => u.replace(/[，。！？、；：）】》]+$/u, '')))]
  const username = String(p.author_username || p.author?.username || p.author || '').toLowerCase()
  const account = watchlist.accounts.find(a => a.username.toLowerCase() === username)
  const mapped = Object.entries(avatars.avatars).find(([u]) => u.toLowerCase() === username)?.[1]
  content[p.id] = { text, links, avatar: rawAvatar(p) || rawAvatar(account) || rawAvatar(mapped) }
}
await writeFile(new URL('../assets/mock/x-content.json', import.meta.url), JSON.stringify(content, null, 2) + '\n')
console.log(`Joined original text/resources for ${Object.keys(content).length} posts`)
