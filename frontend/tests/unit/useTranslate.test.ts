import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

// useTranslate keeps a module-level cache and queue, so every test loads a
// fresh copy of the module with its own useStore / useLang stubs.
const g = globalThis as any
let translateTexts: ReturnType<typeof vi.fn>
const lang = ref<'zh' | 'en'>('zh')
const origUseLang = g.useLang

async function load () {
  vi.resetModules()
  const { useTranslate } = await import('~/composables/useTranslate')
  return useTranslate()
}
const settle = () => new Promise(r => setTimeout(r, 0))

beforeEach(() => {
  lang.value = 'zh'
  translateTexts = vi.fn(async (texts: string[], target: string) => texts.map(s => `[${target}]${s}`))
  g.useStore = () => ({ translateTexts })
  g.useLang = () => ({ lang })
})
afterEach(() => { delete g.useStore; g.useLang = origUseLang })

describe('useTranslate', () => {
  it('translates only texts not already in the UI language', async () => {
    const { tr, enqueue } = await load()
    enqueue(['hello', '你好', '', '  '])
    await settle()
    expect(translateTexts).toHaveBeenCalledTimes(1)
    expect(translateTexts).toHaveBeenCalledWith(['hello'], 'zh')
    expect(tr('hello')).toBe('[zh]hello')
    expect(tr('你好')).toBe('你好')
  })

  it('serves repeats from the cache, per target language', async () => {
    const { tr, enqueue } = await load()
    enqueue(['hello'])
    await settle()
    enqueue(['hello'])
    await settle()
    expect(translateTexts).toHaveBeenCalledTimes(1)

    lang.value = 'en'
    expect(tr('hello')).toBe('hello') // zh translation is not shown in the en UI
    enqueue(['你好'])
    await settle()
    expect(translateTexts).toHaveBeenLastCalledWith(['你好'], 'en')
    expect(tr('你好')).toBe('[en]你好')

    lang.value = 'zh'
    expect(tr('hello')).toBe('[zh]hello') // toggling back needs no new call
    expect(translateTexts).toHaveBeenCalledTimes(2)
  })

  it('falls back to the original text when the request fails, and retries later', async () => {
    translateTexts.mockRejectedValueOnce(new Error('relay down'))
    const { tr, enqueue, translating } = await load()
    enqueue(['hello'])
    expect(translating.value).toBe(true)
    await settle()
    expect(translating.value).toBe(false)
    expect(tr('hello')).toBe('hello')
    enqueue(['hello'])
    await settle()
    expect(tr('hello')).toBe('[zh]hello')
    expect(translateTexts).toHaveBeenCalledTimes(2)
  })

  it('sends chunks of at most 20 and skips oversized texts', async () => {
    const { enqueue } = await load()
    const texts = Array.from({ length: 45 }, (_, i) => `text ${i}`)
    enqueue([...texts, 'x'.repeat(12001)])
    await settle(); await settle(); await settle()
    expect(translateTexts.mock.calls.map(c => c[0].length)).toEqual([20, 20, 5])
  })
})
