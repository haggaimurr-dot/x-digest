import { afterEach, describe, expect, it, vi } from 'vitest'
import { followSummary } from '~/utils/summaryStream'

class FakeES {
  static last: FakeES
  listeners: Record<string, (ev: any) => void> = {}
  onerror: (() => void) | null = null
  closed = false
  constructor (public url: string) { FakeES.last = this }
  addEventListener (type: string, fn: (ev: any) => void) { this.listeners[type] = fn }
  close () { this.closed = true }
  emit (summary: any) { this.listeners.summary?.({ data: JSON.stringify({ summary }) }) }
}

afterEach(() => { vi.unstubAllGlobals() })

describe('followSummary', () => {
  it('delivers pending then final, and closes on the final one', () => {
    vi.stubGlobal('EventSource', FakeES)
    const got: string[] = []
    const fail = vi.fn()
    expect(followSummary('/api/v1/posts/1/summary/stream?lang=zh-CN', s => got.push(s.status), fail)).toBe(true)
    FakeES.last.emit({ status: 'pending' })
    expect(FakeES.last.closed).toBe(false)
    FakeES.last.emit({ status: 'done' })
    expect(got).toEqual(['pending', 'done'])
    expect(FakeES.last.closed).toBe(true)
    FakeES.last.onerror?.() // the server closing afterwards is not a failure
    expect(fail).not.toHaveBeenCalled()
  })
  it('falls back once when the stream fails early', () => {
    vi.stubGlobal('EventSource', FakeES)
    const fail = vi.fn()
    followSummary('/x', () => {}, fail)
    FakeES.last.onerror?.()
    FakeES.last.onerror?.()
    expect(fail).toHaveBeenCalledTimes(1)
  })
  it('returns false without EventSource', () => {
    vi.stubGlobal('EventSource', undefined)
    expect(followSummary('/x', () => {}, () => {})).toBe(false)
  })
})
