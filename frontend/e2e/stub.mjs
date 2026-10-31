// E2E stand-ins for everything outside x-digest (2026-10-29):
//
//  :18099  fake upstreams — fxtwitter (GET /status/{id}) and the AI relay
//          (POST /v1/chat/completions, OpenAI shape). The API points at it
//          through XPOST_API_BASE / TRANSLATE_API_BASE.
//  :18080  the front door, like nginx in production: /api/ → Go API on
//          :18090, everything else → Nuxt on :18091. Playwright opens this.
//
// Run: node e2e/stub.mjs
import http from 'node:http'

export const POST_ID = '1900000000000000001'
export const POST_TITLE = 'E2E 教程：三步部署日报服务'
const STEPS = [
  { text: '准备一台 Linux 服务器并安装 MariaDB', tools: 'MariaDB', tech: '数据库', prep: '' },
  { text: '编译 Go 后端并配置 systemd 服务', tools: 'Go、systemd', tech: '服务部署', prep: '' },
  { text: '构建 Nuxt 前端并用 nginx 反向代理', tools: 'Nuxt、nginx', tech: '前端部署', prep: '' }
]
const TEXT = '手把手教你部署一个自己的内容日报站：第一步准备一台 Linux 服务器并安装 MariaDB 数据库；' +
  '第二步编译 Go 后端，写好 systemd 服务单元让它开机自启；第三步构建 Nuxt 前端，用 nginx 把页面和接口反向代理到同一个端口。' +
  '整个过程大约半小时，适合想把信息流整理成日报的同学照着做。'

function json (res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify(body))
}

function readBody (req) {
  return new Promise((resolve) => {
    let b = ''
    req.on('data', (c) => { b += c })
    req.on('end', () => resolve(b))
  })
}

const upstream = http.createServer(async (req, res) => {
  if (req.method === 'GET' && req.url.startsWith('/status/')) {
    const id = req.url.slice('/status/'.length)
    if (id !== POST_ID) return json(res, 404, { code: 404, message: 'NOT_FOUND' })
    return json(res, 200, {
      code: 200,
      tweet: {
        id: POST_ID,
        url: `https://x.com/e2e_author/status/${POST_ID}`,
        text: TEXT,
        author: { id: '42', screen_name: 'e2e_author', name: 'E2E 作者', avatar_url: '', description: '' },
        created_timestamp: 1761700000,
        lang: 'zh'
      }
    })
  }
  if (req.method === 'POST' && req.url === '/v1/chat/completions') {
    const body = JSON.parse(await readBody(req) || '{}')
    const system = body.messages?.find(m => m.role === 'system')?.content || ''
    const user = body.messages?.filter(m => m.role === 'user').pop()?.content || ''
    // The digest prompt gets the post fields; anything else (translation)
    // gets its input back unchanged.
    const content = system.includes('X 博主推文日报')
      ? JSON.stringify({
        title: POST_TITLE, industry: '软件开发', tech: 'Go、Nuxt、MariaDB', tags: ['部署', '教程', '日报'],
        summary: '作者分三步讲解如何部署内容日报站：安装 MariaDB、部署 Go 后端、用 nginx 代理 Nuxt 前端。',
        steps: STEPS, showFlowchart: true, kind: '教程实操', resources: []
      })
      : user
    return json(res, 200, { choices: [{ message: { role: 'assistant', content } }] })
  }
  json(res, 404, { error: 'stub: no route' })
})

function proxyTo (port, req, res) {
  const p = http.request({ host: '127.0.0.1', port, path: req.url, method: req.method, headers: req.headers }, (r) => {
    res.writeHead(r.statusCode || 502, r.headers)
    r.pipe(res)
  })
  p.on('error', (e) => { res.writeHead(502); res.end(`proxy: ${e.message}`) })
  req.pipe(p)
}

const front = http.createServer((req, res) => {
  proxyTo(req.url.startsWith('/api/') ? 18090 : 18091, req, res)
})

upstream.listen(18099, '127.0.0.1')
front.listen(18080, '127.0.0.1', () => console.log('e2e stub: upstreams :18099, front door :18080'))
