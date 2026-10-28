import { describe, expect, it } from 'vitest'
import { replicaBoard, replicaMarkdown } from '~/utils/replicaExport'
import type { Replica } from '~/composables/types'

const step = (idx: number, status: 'idle' | 'active' | 'done', extra: object = {}) => ({ idx, text: `步骤${idx}`, status, parent_idx: null, ...extra }) as any

const r35: Replica = {
  id: 'r1', post_id: 'p1', post_url: 'https://x.com/a/status/1', post_author: 'a', post_author_name: 'Alice', post_title: '教程', created_at: '',
  steps: [step(0, 'done', { note: '装好了\n第二行' }), step(1, 'done'), step(2, 'done', { attachments: [{ name: 'shot.png', download_url: '/api/v1/attachments/att_1/download' }] }), step(3, 'active'), step(4, 'idle'), step(5, 'done', { parent_idx: 3 })]
}

describe('replica board & export', () => {
  it('board: 3/5 → 60%', () => {
    const fresh: Replica = { ...r35, id: 'r2', steps: [step(0, 'idle')] }
    const full: Replica = { ...r35, id: 'r3', steps: [step(0, 'done')] }
    expect(replicaBoard([r35])).toEqual({ total: 1, inProgress: 1, done: 0, notStarted: 0, avgRate: 60 })
    expect(replicaBoard([r35, fresh, full])).toEqual({ total: 3, inProgress: 1, done: 1, notStarted: 1, avgRate: 53 })
  })
  it('markdown checklist states, notes, attachments, sub-steps', () => {
    const md = replicaMarkdown(r35, 'https://site')
    expect(md).toContain('# 教程')
    expect(md).toContain('进度：60%')
    expect(md).toContain('- [x] 步骤0\n  > 装好了\n  > 第二行')
    expect(md).toContain('- [x] 步骤2\n  - 附件：[shot.png](https://site/api/v1/attachments/att_1/download)')
    expect(md).toContain('- [ ] 步骤3（进行中）\n  - [x] 步骤5')
    expect(md).toContain('- [ ] 步骤4')
  })
})
