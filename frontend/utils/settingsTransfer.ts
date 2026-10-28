import { LS } from '~/composables/useSettings'

// Settings export / import (stopgap until settings sync to the server).
// Only display preferences and filter selections travel; secrets
// (API token, webhook key) and mock-mode data never do.
export const TRANSFER_KEYS: string[] = [
  LS.theme, LS.fontFamily, LS.fontSize, LS.lang, LS.selectedAuthors, LS.selectedCats,
  LS.chipsAuthorsExpanded, LS.chipsTagsExpanded, LS.chipsCatsExpanded, LS.filtersCollapsed,
  'x-digest-x-page-size'
]
const FORMAT = 'xdigest-settings'

export function exportSettings (storage: Pick<Storage, 'getItem'>): string {
  const values: Record<string, string> = {}
  for (const k of TRANSFER_KEYS) {
    const v = storage.getItem(k)
    if (v !== null) values[k] = v
  }
  return JSON.stringify({ format: FORMAT, version: 1, exported_at: new Date().toISOString(), values }, null, 2)
}

// Returns how many keys were applied; throws on a file that is not an export.
export function importSettings (json: string, storage: Pick<Storage, 'setItem'>): number {
  let data: any
  try { data = JSON.parse(json) } catch { throw new Error('不是有效的 JSON 文件') }
  if (data?.format !== FORMAT || typeof data.values !== 'object' || !data.values) throw new Error('不是 x-digest 设置导出文件')
  let n = 0
  for (const k of TRANSFER_KEYS) {
    const v = data.values[k]
    if (typeof v === 'string') { storage.setItem(k, v); n++ }
  }
  return n
}
