import type { Replica, ReplicaStep } from '~/composables/types'

// 复刻看板与导出 (2026-10-28). Progress counts top-level nodes, like the
// flow card ("已完成 x / y 个节点").

function tops (r: Replica): ReplicaStep[] { return r.steps.filter(s => s.parent_idx == null) }

export function replicaRate (r: Replica): number {
  const t = tops(r)
  return t.length ? t.filter(s => s.status === 'done').length / t.length : 0
}

export interface ReplicaBoard { total: number, inProgress: number, done: number, notStarted: number, avgRate: number }

// done = every top node done; inProgress = some node active/done but not all;
// avgRate = mean completion rate (0–100, rounded) over replicas with steps.
export function replicaBoard (list: Replica[]): ReplicaBoard {
  const b: ReplicaBoard = { total: list.length, inProgress: 0, done: 0, notStarted: 0, avgRate: 0 }
  let sum = 0
  let counted = 0
  for (const r of list) {
    const t = tops(r)
    const rate = replicaRate(r)
    if (t.length) { sum += rate; counted++ }
    if (t.length && rate === 1) b.done++
    else if (t.some(s => s.status !== 'idle')) b.inProgress++
    else b.notStarted++
  }
  b.avgRate = counted ? Math.round(sum / counted * 100) : 0
  return b
}

// Markdown checklist: top nodes as "- [x]/- [ ]", sub-nodes indented, notes
// as quoted lines, attachments as links (absolute when origin is given).
export function replicaMarkdown (r: Replica, origin = ''): string {
  const lines: string[] = [`# ${r.post_title || '复刻'}`, '']
  if (r.post_url) lines.push(`来源：[${r.post_author_name || r.post_author || r.post_url}](${r.post_url})`, '')
  lines.push(`进度：${Math.round(replicaRate(r) * 100)}%`, '')
  const children = (idx: number) => r.steps.filter(s => s.parent_idx === idx)
  const emit = (s: ReplicaStep, depth: number) => {
    const pad = '  '.repeat(depth)
    lines.push(`${pad}- [${s.status === 'done' ? 'x' : ' '}] ${s.text}${s.status === 'active' ? '（进行中）' : ''}`)
    for (const l of (s.note || '').trim().split('\n').filter(Boolean)) lines.push(`${pad}  > ${l}`)
    for (const a of s.attachments || []) lines.push(`${pad}  - 附件：[${a.name}](${origin}${a.download_url})`)
    for (const c of children(s.idx)) emit(c, depth + 1)
  }
  for (const s of tops(r)) emit(s, 0)
  return lines.join('\n') + '\n'
}
