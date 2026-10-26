import type { Post } from './types'
import { getLang, translate, type Lang } from '~/utils/i18n'

// ---------- avatars: SPEC: "avatars/ai_xiaomu.jpg" -> "/avatars/ai_xiaomu.jpg" ----------
export function avatarUrl (raw?: string): string {
  if (!raw) return ''
  if (/^https?:\/\//.test(raw)) return raw
  return '/' + raw.replace(/^\/+/, '')
}
const AVATAR_COLORS = ['#1d9bf0', '#7856ff', '#00ba7c', '#f91880', '#ff7a00']
export function avatarColor (name: string): string {
  let h = 0
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return AVATAR_COLORS[h % AVATAR_COLORS.length]
}
export function avatarInitial (name: string): string {
  return (name || '?').trim().slice(0, 1) || '?'
}

// ---------- time: relative + Asia/Shanghai ----------
export function shanghaiTime (iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hour12: false
  }).formatToParts(d)
  const get = (t: string) => parts.find(p => p.type === t)?.value ?? ''
  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}`
}
export function relativeTime (iso?: string, lang: Lang = getLang()): string {
  if (!iso) return ''
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return ''
  const diff = Date.now() - t
  const m = Math.floor(diff / 60000)
  if (m < 1) return translate('time.justNow', lang)
  if (m < 60) return translate('time.minAgo', lang, { m })
  const h = Math.floor(m / 60)
  if (h < 24) return translate('time.hourAgo', lang, { h })
  const d = Math.floor(h / 24)
  if (d < 30) return translate('time.dayAgo', lang, { d })
  return shanghaiTime(iso)
}
// Shanghai day bounds (SPEC: date range uses Asia/Shanghai day start/end).
export function shanghaiDayBound (dateStr: string, end: boolean): number | null {
  if (!dateStr) return null
  // Interpret the calendar date as a Shanghai wall-clock date.
  const base = new Date(`${dateStr}T00:00:00+08:00`)
  if (Number.isNaN(base.getTime())) return null
  return end ? base.getTime() + 24 * 3600 * 1000 - 1 : base.getTime()
}

// ---------- highlight + first-hit snippet (original app.js highlightHtml) ----------
export function escapeHtml (s: unknown): string {
  return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}
export function highlightHtml (text: string, q: string): string {
  const raw = String(text ?? '')
  if (!q) return escapeHtml(raw)
  const lower = raw.toLowerCase()
  const needle = q.toLowerCase()
  let out = ''
  let i = 0
  while (true) {
    const idx = lower.indexOf(needle, i)
    if (idx < 0) { out += escapeHtml(raw.slice(i)); break }
    out += escapeHtml(raw.slice(i, idx)) + `<mark class="search-hit">${escapeHtml(raw.slice(idx, idx + needle.length))}</mark>`
    i = idx + needle.length
  }
  return out
}
/** First-hit snippet across title/summary/tags/author/industry/tech. */
export function firstHitSnippet (p: Post, q: string): string {
  if (!q) return ''
  const needle = q.toLowerCase()
  const fields = [p.title, p.summary || '', (p.tags || []).join(' '), p.author_name, p.author, p.industry || '', p.tech || '']
  for (const f of fields) {
    const idx = f.toLowerCase().indexOf(needle)
    if (idx >= 0) {
      const start = Math.max(0, idx - 30)
      const end = Math.min(f.length, idx + needle.length + 50)
      return (start > 0 ? '…' : '') + highlightHtml(f.slice(start, end), q) + (end < f.length ? '…' : '')
    }
  }
  return ''
}

// ---------- flowchart eligibility: port of app.js deriveShowFlowchart ----------
const FLOW_PRODUCT_RE = /Claude(?:\s*Code|\s*Pro)?|Codex|ChatGPT|GPT-?\d|Grok\s*Bot|\bGrok\b|豆包(?:工作)?|Open-?Science|Clipper|Typeless|叭哥说|Tripo|Blender|Unity|Remotion|ESP32|RTX\s*Spark|Hugging\s*Face|Live\s*Link\s*Face|抖音直播伴侣|剪映|Cursor|Antigravity|Opus(?:\s*5\.5)?|Astra|DeepSeek|Mole|Glimpse|AIHOT|Monid|Fable|Unlimited-?OCR|Whoop|Apple\s*Health|UU\s*远程|网易\s*UU|Node\.?js|ffmpeg|StarryBlu|DMIT|Apple\s*Pay|Auto-?Rig|Image\s*2\.5|Google\s*Flow|Profound|REA|Next\.?js|Health\+GPT|Atypica|Sai|MCP|Skill|直播伴侣|面捕/i
const FLOW_ACTION_RE = /打开|点击|选择|运行|执行|输入|登录|注册|安装|下载|配置|设置|新建|创建|连接|授权|上传|录制|挂载|部署|接入|切换|填写|复制|启动|关闭|佩戴|获取|导入|复现|迁移|报白|推流|第一步|第二步|首先|然后|接着|保姆级|手把手|教程|SOP|从\s*0|按步骤/
const FLOW_HARD_NO_RE = /必读合集|神帖合集|观察报告|盈利模式|到底为了什么|情绪比时长|抱歉回复晚了|翻转法|代表作拿影响力|半年成长六点|房车长帖|线下跑客户|普通人收益路径|普通人 AI 赚钱老几样|别只盯硅谷模型|放出.{0,20}Flash|月下载\s*\d|涨粉复盘|可复制性的 X 涨粉|从小白到单篇|冷启动贴写错了|血泪报价法|GEO 公开课|主题式连载|脆弱性（GEO）|论文：从公开内容/
const FLOW_TUTORIAL_TITLE_RE = /教程|保姆|手把手|SOP|从\s*0|开通|安装|部署|上手|清理|开播|配置清单|迁移|实测长文|打通|平替|开源还原|扒了|验收|指标化|万能复刻/
export function deriveShowFlowchart (p: Post): boolean {
  const raw = Array.isArray(p.steps) ? p.steps : []
  if (raw.length < 2) return false
  const texts = raw.map(s => String(s.text || ''))
  const blob = [p.title || '', p.summary || '', p.tech || '', ...(p.tags || []), ...texts].join('\n')
  if (FLOW_HARD_NO_RE.test(p.title || '') || FLOW_HARD_NO_RE.test((p.summary || '').slice(0, 240))) return false
  if (!FLOW_PRODUCT_RE.test(blob)) return false
  const actionN = texts.filter(t => FLOW_ACTION_RE.test(t)).length
  return FLOW_TUTORIAL_TITLE_RE.test(p.title || '') || actionN >= 2 || (actionN >= 1 && FLOW_PRODUCT_RE.test(p.title || ''))
}
export function shouldShowFlowchart (p: Post): boolean {
  if (typeof p.show_flowchart === 'boolean') return p.show_flowchart
  return deriveShowFlowchart(p)
}
export function trunc40 (s: string): string {
  return s.length > 40 ? s.slice(0, 40) + '…' : s
}
