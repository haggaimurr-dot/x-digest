import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NoteEditor from '~/components/NoteEditor.vue'
import { insertAt } from '~/utils/textInsert'

describe('insertAt', () => {
  it('puts the insert on its own line', () => {
    expect(insertAt('ab', 1, 'X')).toBe('a\nX\nb')
    expect(insertAt('a\n', 2, 'X')).toBe('a\nX')
    expect(insertAt('', 0, 'X')).toBe('X')
    expect(insertAt('abc', 99, 'X')).toBe('abc\nX')
  })
})

describe('NoteEditor paste', () => {
  it('uploads a pasted image and inserts a markdown image link', async () => {
    const upload = vi.fn(async (f: File) => `/api/v1/attachments/att_1/download?inline=1#${f.type}`)
    const w = mount(NoteEditor, { props: { modelValue: '前文', uploadImage: upload } })
    const file = new File(['png'], 'shot.png', { type: 'image/png' })
    await w.find('textarea').trigger('paste', { clipboardData: { files: [file] } })
    await flushPromises()
    expect(upload).toHaveBeenCalledTimes(1)
    const saved = w.emitted('save')?.[0]?.[0] as string
    expect(saved).toMatch(/^前文\n!\[shot\.png\]\(\/api\/v1\/attachments\/att_1\/download\?inline=1#image\/png\)$/)
  })
  it('ignores pastes without images', async () => {
    const upload = vi.fn()
    const w = mount(NoteEditor, { props: { modelValue: '', uploadImage: upload } })
    await w.find('textarea').trigger('paste', { clipboardData: { files: [new File(['x'], 'a.txt', { type: 'text/plain' })] } })
    expect(upload).not.toHaveBeenCalled()
  })
  it('preview strips scripts', async () => {
    const w = mount(NoteEditor, { props: { modelValue: '<img src=x onerror=alert(1)> **b**' } })
    await w.find('button').trigger('click')
    expect(w.find('.note-preview').html()).not.toContain('onerror')
    expect(w.find('.note-preview').html()).toContain('<strong>b</strong>')
  })
})
