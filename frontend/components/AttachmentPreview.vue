<template>
  <Teleport to="body">
    <div class="preview-overlay" @click.self="emit('close')">
      <div ref="panelRef" class="preview-panel" role="dialog" aria-modal="true" :aria-label="attachment.name">
        <div class="preview-header">
          <span class="preview-title">{{ attachment.name }}</span>
          <button class="btn btn-sm" type="button" @click="toggleFullscreen">{{ isFullscreen ? t('preview.exitFullscreen') : t('preview.fullscreen') }}</button>
          <button class="btn btn-sm" type="button" :aria-label="t('preview.close')" @click="emit('close')">✕</button>
        </div>
        <div class="preview-body">
          <div v-if="loading">{{ t('preview.loading') }}</div>
          <div v-else-if="error" style="color:var(--danger-text)">{{ error }}</div>
          <template v-else>
            <img v-if="kind === 'image'" class="preview-image" :src="previewUrl" :alt="attachment.name" />
            <div v-else-if="kind === 'pdf'">
              <p style="color:var(--text-muted);font-size:13px">{{ t('preview.pdfNote') }}</p>
              <embed :src="previewUrl" type="application/pdf" style="width:100%;height:72vh" />
            </div>
            <div v-else-if="kind === 'md'" class="rich-content" v-html="htmlContent" />
            <pre v-else-if="kind === 'text'">{{ textContent }}</pre>
            <div v-else-if="kind === 'docx'" class="rich-content" v-html="htmlContent" />
            <div v-else-if="kind === 'xlsx'">
              <p v-if="xlsxTruncated" style="color:var(--text-muted);font-size:13px">{{ t('preview.xlsxTruncated') }}</p>
              <table class="preview-table">
                <tr v-for="(row, ri) in xlsxRows" :key="ri">
                  <template v-for="(c, ci) in row" :key="ci">
                    <th v-if="ri === 0">{{ c }}</th>
                    <td v-else>{{ c }}</td>
                  </template>
                </tr>
              </table>
            </div>
            <div v-else-if="kind === 'pptx'">
              <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
                <button class="btn btn-sm" type="button" :disabled="slideIdx <= 0" @click="slideIdx--">{{ t('preview.prevPage') }}</button>
                <span>{{ t('preview.pageOf', { cur: slideIdx + 1, total: slides.length }) }}</span>
                <button class="btn btn-sm" type="button" :disabled="slideIdx >= slides.length - 1" @click="slideIdx++">{{ t('preview.nextPage') }}</button>
              </div>
              <div v-if="slides[slideIdx]" class="rich-content" style="white-space:pre-wrap">{{ slides[slideIdx].text }}</div>
              <div v-if="slides[slideIdx]?.images?.length" style="display:flex;gap:8px;flex-wrap:wrap;margin-top:8px">
                <img v-for="(im, i) in slides[slideIdx].images" :key="i" :src="im" style="max-width:220px" />
              </div>
            </div>
            <div v-else-if="kind === 'zip'">
              <p style="color:var(--text-muted);font-size:13px">{{ t('preview.zipEntries') }}</p>
              <ul>
                <li v-for="e in zipEntries" :key="e.name">
                  <a v-if="e.url" :href="e.url" :download="e.name">{{ e.name }}</a>
                  <span v-else>{{ e.name }}{{ t('preview.dir') }}</span>
                </li>
              </ul>
            </div>
            <div v-else-if="kind === 'legacy'">
              <p style="color:var(--danger-text);font-size:13px">{{ t('preview.legacyNote') }}<a :href="downloadUrl" download>{{ t('preview.downloadOriginal') }}</a>{{ t('preview.legacySuffix') }}</p>
              <pre>{{ textContent }}</pre>
            </div>
            <div v-else>
              <p>{{ t('preview.unsupportedPrefix') }}<a :href="downloadUrl" download>{{ t('common.download') }}</a>{{ t('preview.unsupportedSuffix') }}</p>
            </div>
          </template>
        </div>
        <div class="preview-footer"><a :href="downloadUrl" download>{{ t('preview.downloadOriginal') }}</a></div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { marked } from 'marked'
import mammoth from 'mammoth'
import JSZip from 'jszip'
import type { Attachment } from '~/composables/types'
import { IMAGE_EXTS } from '~/composables/useStore'
import { useScrollLock } from '~/composables/useScrollLock'

const props = defineProps<{ attachment: Attachment }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t } = useLang()
// 预览以居中遮罩打开期间锁住页面滚动（复用 ref 计数锁，关闭即释放）。
useScrollLock(ref(true))
const loading = ref(true)
const error = ref('')
const kind = ref('')
const textContent = ref('')
const htmlContent = ref('')
const slideIdx = ref(0)
const slides = ref<Array<{ text: string, images: string[] }>>([])
const zipEntries = ref<Array<{ name: string, size?: number, url?: string }>>([])
const xlsxRows = ref<string[][]>([])
const xlsxTruncated = ref(false)
const panelRef = ref<HTMLElement | null>(null)
const isFullscreen = ref(false)

const ext = computed(() => (props.attachment.ext || props.attachment.name.split('.').pop() || '').toLowerCase())
// Mock attachments carry a blob; real ones use the API download URL (inline=1 for preview).
const downloadUrl = computed(() => {
  const a: any = props.attachment
  return a.__blob ? (a.__blobUrl || (a.__blobUrl = URL.createObjectURL(a.__blob))) : props.attachment.download_url
})
const previewUrl = computed(() => {
  const a: any = props.attachment
  if (a.__blob) return downloadUrl.value
  const u = props.attachment.download_url
  return u.includes('?') ? `${u}&inline=1` : `${u}?inline=1`
})
async function toggleFullscreen () {
  if (document.fullscreenElement) {
    await document.exitFullscreen()
    return
  }
  try { await panelRef.value?.requestFullscreen() } catch { /* 浏览器不支持或被拒绝时留在窗口模式 */ }
}
function onFullscreenChange () { isFullscreen.value = !!document.fullscreenElement }
function onKeydown (e: KeyboardEvent) {
  // 全屏时 Esc 由浏览器先用来退出全屏，不应同时关掉预览。
  if (e.key === 'Escape' && !document.fullscreenElement) emit('close')
}
async function fetchBytes (): Promise<ArrayBuffer> {
  const a: any = props.attachment
  if (a.__blob) return await a.__blob.arrayBuffer()
  const res = await fetch(previewUrl.value)
  if (!res.ok) throw new Error(t('preview.downloadFailed', { status: res.status }))
  return await res.arrayBuffer()
}
function extractStrings (buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  let out: string[] = []
  let cur = ''
  for (const b of bytes) {
    const ch = String.fromCharCode(b)
    if (b >= 32 && b < 127) cur += ch
    else {
      if (cur.length >= 4) out.push(cur)
      cur = ''
    }
  }
  if (cur.length >= 4) out.push(cur)
  return out.join('\n').slice(0, 20000)
}
function slideTexts (xml: string): string {
  const matches = [...xml.matchAll(/<a:t>([^<]*)<\/a:t>/g)].map(m => m[1])
  return matches.join(' ')
}
// 列标（A/B/AA）转 0 起下标，用于按单元格位置补空缺。
function colIndex (ref: string): number {
  const letters = ref.replace(/[^A-Za-z]/g, '').toUpperCase()
  let n = 0
  for (const ch of letters) n = n * 26 + (ch.charCodeAt(0) - 64)
  return n - 1
}
// xlsx 是 zip：sharedStrings + 第一个工作表的行列，用现成的 JSZip 解析成表格。
async function parseXlsx (buf: ArrayBuffer): Promise<{ rows: string[][], truncated: boolean }> {
  const zip = await JSZip.loadAsync(buf)
  const shared: string[] = []
  const ssFile = zip.file('xl/sharedStrings.xml')
  if (ssFile) {
    const xml = await ssFile.async('string')
    for (const m of xml.matchAll(/<si>([\s\S]*?)<\/si>/g)) {
      shared.push([...m[1].matchAll(/<t[^>]*>([^<]*)<\/t>/g)].map(x => x[1]).join(''))
    }
  }
  const sheetFile = zip.file('xl/worksheets/sheet1.xml')
  if (!sheetFile) throw new Error(t('preview.noSheet'))
  let xml = await sheetFile.async('string')
  xml = xml.replace(/<c([^>]*)\/>/g, '<c$1></c>') // 空单元格补成成对标签，统一处理
  const rows: string[][] = []
  let truncated = false
  for (const rm of xml.matchAll(/<row[^>]*>([\s\S]*?)<\/row>/g)) {
    const cells: string[] = []
    for (const cm of rm[1].matchAll(/<c([^>]*)>([\s\S]*?)<\/c>/g)) {
      const attrs = cm[1]
      const inner = cm[2]
      const ref = attrs.match(/\br="([A-Za-z]+\d+)"/)?.[1] || ''
      const t = attrs.match(/\bt="(\w+)"/)?.[1]
      let v = inner.match(/<v>([^<]*)<\/v>/)?.[1] ?? ''
      if (!v && inner.includes('<is>')) v = [...inner.matchAll(/<t[^>]*>([^<]*)<\/t>/g)].map(x => x[1]).join('')
      const idx = ref ? colIndex(ref) : cells.length
      while (cells.length <= idx) cells.push('')
      cells[idx] = t === 's' ? (shared[Number(v)] ?? v) : v
    }
    rows.push(cells)
    if (rows.length >= 200) { truncated = true; break }
  }
  return { rows, truncated }
}

onMounted(async () => {
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('keydown', onKeydown)
  try {
    const e = ext.value
    if (IMAGE_EXTS.includes(e)) { kind.value = 'image'; return }
    if (e === 'pdf') { kind.value = 'pdf'; return }
    if (['rar', '7z', 'tar', 'gz', 'tgz'].includes(e)) { kind.value = 'download'; return }
    if (e === 'md') {
      const buf = await fetchBytes()
      htmlContent.value = marked.parse(new TextDecoder().decode(buf)) as string
      kind.value = 'md'; return
    }
    if (['txt', 'csv'].includes(e)) {
      const buf = await fetchBytes()
      textContent.value = new TextDecoder().decode(buf)
      kind.value = 'text'; return
    }
    if (e === 'docx') {
      const buf = await fetchBytes()
      const r = await mammoth.convertToHtml({ arrayBuffer: buf })
      htmlContent.value = r.value
      kind.value = 'docx'; return
    }
    if (e === 'xlsx') {
      const buf = await fetchBytes()
      const parsed = await parseXlsx(buf)
      xlsxRows.value = parsed.rows
      xlsxTruncated.value = parsed.truncated
      kind.value = 'xlsx'; return
    }
    if (e === 'pptx') {
      const buf = await fetchBytes()
      const zip = await JSZip.loadAsync(buf)
      const slideNames = Object.keys(zip.files).filter(n => /^ppt\/slides\/slide\d+\.xml$/.test(n)).sort()
      const list: Array<{ text: string, images: string[] }> = []
      for (const name of slideNames) {
        const xml = await zip.files[name].async('string')
        const images: string[] = []
        // Best-effort: embed all media images on every slide (SPEC: text + images paging).
        const media = Object.keys(zip.files).filter(n => /^ppt\/media\//.test(n))
        for (const m of media) {
          const blob = await zip.files[m].async('blob')
          images.push(URL.createObjectURL(blob))
        }
        list.push({ text: slideTexts(xml), images })
      }
      slides.value = list.length ? list : [{ text: t('preview.noSlideText'), images: [] }]
      kind.value = 'pptx'; return
    }
    if (e === 'zip') {
      const buf = await fetchBytes()
      const zip = await JSZip.loadAsync(buf)
      const entries: Array<{ name: string, size?: number, url?: string }> = []
      for (const [name, file] of Object.entries(zip.files)) {
        if (file.dir) { entries.push({ name }); continue }
        const blob = await file.async('blob')
        entries.push({ name, size: blob.size, url: URL.createObjectURL(blob) })
      }
      zipEntries.value = entries
      kind.value = 'zip'; return
    }
    if (['doc', 'ppt', 'xls'].includes(e)) {
      const buf = await fetchBytes()
      textContent.value = extractStrings(buf)
      kind.value = 'legacy'; return
    }
    kind.value = 'download'
  } catch (err: any) {
    error.value = err?.message || t('preview.loadFailed')
  } finally { loading.value = false }
})
onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<style scoped>
/* 预览一律居中遮罩打开：遮罩盖住整页，面板在页面正中心；全屏时面板铺满屏幕。 */
.preview-overlay { position: fixed; inset: 0; z-index: 300; background: rgba(0, 0, 0, .78); display: flex; align-items: center; justify-content: center; padding: 16px; }
.preview-panel { background: var(--bg-elevated); border: 1px solid var(--border); border-radius: 16px; width: min(960px, 94vw); max-height: 92vh; display: flex; flex-direction: column; overflow: hidden; }
.preview-panel:fullscreen { width: 100%; height: 100%; max-height: none; border-radius: 0; }
.preview-header { display: flex; gap: 8px; align-items: center; padding: 12px 14px; border-bottom: 1px solid var(--border); }
.preview-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 700; }
.preview-body { overflow: auto; padding: 14px; }
.preview-image { display: block; max-width: 100%; max-height: 76vh; margin: 0 auto; object-fit: contain; }
.preview-panel:fullscreen .preview-image { max-height: calc(100vh - 130px); }
.preview-footer { padding: 10px 14px; border-top: 1px solid var(--border); }
.preview-table { border-collapse: collapse; width: 100%; }
.preview-table th, .preview-table td { border: 1px solid var(--border); padding: 4px 8px; font-size: 13px; text-align: left; }
.preview-table th { background: var(--bg); }
</style>
