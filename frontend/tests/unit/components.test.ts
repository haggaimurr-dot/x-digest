import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TagInput from '~/components/TagInput.vue'
import FilterCombo from '~/components/FilterCombo.vue'

describe('TagInput', () => {
  it('adds comma-separated tags without duplicates', async () => {
    const w = mount(TagInput, { props: { modelValue: ['AI'] } })
    const input = w.find('input')
    await input.setValue('AI，Agent, MCP')
    await input.trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:modelValue')?.[0]?.[0]).toEqual(['AI', 'Agent', 'MCP'])
  })
  it('removes with × and with Backspace on an empty draft', async () => {
    const w = mount(TagInput, { props: { modelValue: ['a', 'b'] } })
    await w.find('.tag-input-x').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]?.[0]).toEqual(['b'])
    await w.find('input').trigger('keydown', { key: 'Backspace' })
    expect(w.emitted('update:modelValue')?.[1]?.[0]).toEqual(['a'])
  })
  it('offers only unused suggestions', () => {
    const w = mount(TagInput, { props: { modelValue: ['a'], suggestions: ['a', 'b'] } })
    expect(w.findAll('.tag-input-suggest-chip').map(b => b.text())).toEqual(['+ b'])
  })
})

describe('FilterCombo', () => {
  const options = [{ value: 'c1', label: 'AI 工具', count: 3 }, { value: 'c2', label: '教程' }, 'raw']
  it('filters suggestions by query and picks the first on Enter', async () => {
    const w = mount(FilterCombo, { props: { label: '类别', options, modelValue: [] }, attachTo: document.body })
    const input = w.find('input')
    await input.trigger('focus')
    expect(w.findAll('.combo-list li')).toHaveLength(3)
    await input.setValue('教')
    expect(w.findAll('.combo-list li').map(l => l.text())).toEqual(['教程'])
    await input.trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:modelValue')?.[0]?.[0]).toEqual(['c2'])
    w.unmount()
  })
  it('shows chip labels and removes', async () => {
    const w = mount(FilterCombo, { props: { label: '类别', options, modelValue: ['c1'] } })
    expect(w.find('.combo-chip').text()).toContain('AI 工具')
    await w.find('.combo-chip button').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]?.[0]).toEqual([])
  })
  it('says no match', async () => {
    const w = mount(FilterCombo, { props: { label: 'x', options, modelValue: [] } })
    const input = w.find('input')
    await input.trigger('focus')
    await input.setValue('zzz')
    expect(w.find('.combo-empty').exists()).toBe(true)
  })
})
