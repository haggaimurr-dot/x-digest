<template>
  <div class="attachment-preview">
    <div v-if="loading">加载预览…</div>
    <div v-else-if="error" style="color:var(--danger-text)">{{ error }}</div>
    <template v-else>
      <img v-if="kind === 'image'" :src="previewUrl" :alt="attachment.name" />
      <div v-else-if="kind === 'pdf'">
        <p style="color:var(--text-muted);font-size:13px">PDF 预览使用浏览器原生 embed（见 README 已知差异，非原站 pdf.js 画布）。</p>
        <embed :src="previewUrl" type="application/pdf" width="100%" height="480" />
      </div>
      <div v-else-if="kind === 'md'" class="rich-content" v-html="htmlContent" />
      <pre v-else-if="kind === 'text'">{{ textContent }}</pre>
      <div v-else-if="kind === 'docx'" class="rich-content" v-html="htmlContent" />
      <div v-else-if="kind === 'pptx'">
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <button class="btn btn-sm" type="button" :disabled="slideIdx <= 0" @click="slideIdx--">上一页</button>
          <span>第 {{ slideIdx + 1 }} / {{ slides.length }} 页</span>
          <button class="btn btn-sm" type="button" :disabled="slideIdx >= slides.length - 1" @click="slideIdx++">下一页</button>
        </div>
        <div v-if="slides[slideIdx]" class="rich-content" style="white-space:pre-wrap">{{ slides[slideIdx].text }}</div>
        <div v-if="slides[slideIdx]?.images?.length" style="display:flex;gap:8px;flex-wrap:wrap;margin-top:8px">
          <img v-for="(im, i) in slides[slideIdx].images" :key="i" :src="im" style="max-width:220px" />
        </div>
      </div>
      <div v-else-if="kind === 'zip'">
        <p style="color:var(--text-muted);font-size:13px">压缩包条目（点击可下载单个文件）：</p>
        <ul>
          <li v-for="e in zipEntries" :key="e.name">
            <a v-if="e.url" :href="e.url" :download="e.name">{{ e.name }}</a>
            <span v-else>{{ e.name }}（目录）</span>
            <span v-if="e.size"> — {{ e.size }} 字节</span>
          </li>
        </ul>
      </div>
      <div v-else-if="kind === 'legacy'">
        <p style="color:var(--danger-text);font-size:13px">旧版二进制格式（doc/ppt），仅尽力提取可读文本，请以<a :href="downloadUrl" download>下载原文件</a>核对。</p>
        <pre>{{ textContent }}</pre>
      </div>
      <div v-else>
        <p>该类型暂不支持浏览器内预览，请<a :href="downloadUrl" download>下载文件</a>查看。</p>
      </div>
    </template>
    <p><a :href="downloadUrl" download>下载原文件</a></p>
  </div>
</template>

<script setup lang="ts">
import { marked } from 'marked'
import mammoth from 'mammoth'
import JSZip from 'jszip'
import type { Attachment } from '~/composables/types'
import { IMAGE_EXTS } from '~/composables/useStore'

const props = defineProps<{ attachment: Attachment }>()
const loading = ref(true)
const error = ref('')
const kind = ref('')
const textContent = ref('')
const htmlContent = ref('')
const slideIdx = ref(0)
const slides = ref<Array<{ text: string, images: string[] }>>([])
const zipEntries = ref<Array<{ name: string, size?: number, url?: string }>>([])

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
async function fetchBytes (): Promise<ArrayBuffer> {
  const a: any = props.attachment
  if (a.__blob) return await a.__blob.arrayBuffer()
  const res = await fetch(previewUrl.value)
  if (!res.ok) throw new Error(`下载失败 (${res.status})`)
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

onMounted(async () => {
  try {
    const e = ext.value
    if (IMAGE_EXTS.includes(e)) { kind.value = 'image'; return }
    if (e === 'pdf') { kind.value = 'pdf'; return }
    if (['rar', '7z', 'tar', 'gz', 'tgz', 'xls', 'xlsx', 'csv'].includes(e) && e !== 'csv') { kind.value = 'download'; return }
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
      slides.value = list.length ? list : [{ text: '(未提取到幻灯片文本)', images: [] }]
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
    if (['doc', 'ppt'].includes(e)) {
      const buf = await fetchBytes()
      textContent.value = extractStrings(buf)
      kind.value = 'legacy'; return
    }
    kind.value = 'download'
  } catch (err: any) {
    error.value = err?.message || '预览加载失败'
  } finally { loading.value = false }
})
</script>
