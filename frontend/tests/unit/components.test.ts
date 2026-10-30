import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TagInput from '~/components/TagInput.vue'
import FilterCombo from '~/components/FilterCombo.vue'
import ListPager from '~/components/ListPager.vue'

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
  it('closes the dropdown with the confirm button', async () => {
    const w = mount(FilterCombo, { props: { label: '类别', options, modelValue: ['c1'] }, attachTo: document.body })
    await w.find('input').trigger('focus')
    expect(w.find('.combo-pop').exists()).toBe(true)
    expect(w.find('.combo-picked').text()).toContain('1')
    await w.find('.combo-foot button').trigger('click')
    expect(w.find('.combo-pop').exists()).toBe(false)
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

describe('ListPager', () => {
  const props = { page: 5, pageCount: 9, pageSize: 20, total: 170, sizes: [20, 50, 100] }
  it('shows first / last and the pages around the current one', () => {
    const w = mount(ListPager, { props })
    expect(w.findAll('.x-pager > button:not(.btn-icon), .x-pager-gap').map(e => e.text())).toEqual(['1', '…', '4', '5', '6', '…', '9'])
    expect(w.find('[aria-current="page"]').text()).toBe('5')
    expect(w.find('.x-pager-info').text()).toBe('共 170 条')
  })
  it('emits page and page-size changes', async () => {
    const w = mount(ListPager, { props })
    await w.find('[aria-label="下一页"]').trigger('click')
    await w.findAll('.x-pager > button').find(b => b.text() === '9')!.trigger('click')
    expect(w.emitted('update:page')).toEqual([[6], [9]])
    await w.find('select').setValue('100')
    expect(w.emitted('update:pageSize')).toEqual([[100]])
  })
  it('disables prev on the first page and next on the last', () => {
    const w = mount(ListPager, { props: { ...props, page: 1, pageCount: 1 } })
    expect(w.find('[aria-label="上一页"]').attributes('disabled')).toBeDefined()
    expect(w.find('[aria-label="下一页"]').attributes('disabled')).toBeDefined()
  })
})
