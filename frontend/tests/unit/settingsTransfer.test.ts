import { describe, expect, it } from 'vitest'
import { exportSettings, importSettings } from '~/utils/settingsTransfer'
import { LS, LEGACY_LS, clearLegacySettings } from '~/composables/useSettings'

function mem (init: Record<string, string> = {}) {
  const m = new Map(Object.entries(init))
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => { m.set(k, v) }, m }
}

describe('settings export/import', () => {
  it('round-trips preferences between devices, never secrets', () => {
    const a = mem({ [LS.theme]: 'light', [LS.fontSize]: '17px', [LS.lang]: 'en', [LS.apiToken]: 'secret' })
    const json = exportSettings(a)
    expect(json).not.toContain('secret')
    const b = mem()
    expect(importSettings(json, b)).toBe(3)
    expect(b.m.get(LS.theme)).toBe('light')
    expect(b.m.get(LS.fontSize)).toBe('17px')
    expect(b.m.get(LS.lang)).toBe('en')
  })
  it('drops the retired webhook keys', () => {
    const a = mem({ [LEGACY_LS[0]]: 'https://hook', [LEGACY_LS[1]]: 'k', [LS.theme]: 'light' })
    clearLegacySettings({ removeItem: (k: string) => { a.m.delete(k) } })
    expect([...a.m.keys()]).toEqual([LS.theme])
  })
  it('rejects other files', () => {
    expect(() => importSettings('{"a":1}', mem())).toThrow('不是 x-digest 设置导出文件')
    expect(() => importSettings('nope', mem())).toThrow('不是有效的 JSON 文件')
  })
})
