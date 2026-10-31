<template>
  <div class="note-editor">
    <div class="note-toolbar" role="group" :aria-label="t('note.groupAria')">
      <span role="status" style="color:var(--text-muted);font-size:13px;align-self:center">{{ t('note.markdown') }}</span>
      <IconBtn class="btn-sm" :icon="preview ? 'pencil' : 'eye'" :label="preview ? t('note.edit') : t('note.preview')" :aria-pressed="preview" @click="preview = !preview" />
      <IconBtn class="btn-sm" :class="{ 'btn-danger': recording }" icon="mic"
        :label="recording ? t('note.recording') : transcribing ? t('note.transcribing') : t('note.record')"
        :aria-pressed="recording" :disabled="transcribing" @click="toggleRecord" />
      <span v-if="recording" class="rec-dot" aria-hidden="true">{{ recSecs }}s</span>
    </div>
    <!-- Markdown preview: marked output always goes through DOMPurify. -->
    <div v-if="preview" class="rich-content note-preview" v-html="previewHtml" />
    <textarea v-else ref="ta" :value="modelValue" rows="3" :placeholder="t('note.ph')"
      :aria-label="t('note.aria')" @input="onInput" @paste="onPaste" />
    <p v-if="pasteMsg" style="color:var(--text-muted);font-size:12px;margin:4px 0 0">{{ pasteMsg }}</p>
  </div>
</template>

<script setup lang="ts">
import IconBtn from '~/components/IconBtn.vue'
import { marked } from 'marked'
import { safeHtml } from '~/utils/sanitize'
import { insertAt } from '~/utils/textInsert'

// uploadImage (optional): uploads a pasted image as an attachment of this
// step and resolves to its URL; the editor then inserts ![name](url).
const props = defineProps<{ modelValue: string, uploadImage?: (file: File) => Promise<string> }>()
const ta = ref<HTMLTextAreaElement | null>(null)
const pasteMsg = ref('')
const { t } = useLang()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void, (e: 'save', v: string): void }>()
const preview = ref(false)
const previewHtml = computed(() => safeHtml(marked.parse(props.modelValue || '') as string))
let saveTimer: any = null
// 录音转文字：MediaRecorder 录音 -> 后端 /api/v1/stt（本地 Whisper）-> 文本插入光标处，可继续编辑。
const store = useStore()
const recording = ref(false)
const transcribing = ref(false)
const recSecs = ref(0)
let mediaRecorder: MediaRecorder | null = null
let recChunks: Blob[] = []
let recTimer: any = null
let recStream: MediaStream | null = null
async function toggleRecord () {
  if (transcribing.value) return
  if (recording.value) { stopRecord(); return }
  if (!window.MediaRecorder) { pasteMsg.value = t('note.recordFailed'); return }
  try {
    recStream = await navigator.mediaDevices.getUserMedia({ audio: true })
  } catch {
    pasteMsg.value = t('note.micDenied')
    return
  }
  recChunks = []
  const mime = MediaRecorder.isTypeSupported('audio/webm') ? 'audio/webm' : ''
  try {
    mediaRecorder = mime ? new MediaRecorder(recStream, { mimeType: mime }) : new MediaRecorder(recStream)
  } catch {
    pasteMsg.value = t('note.recordFailed')
    recStream.getTracks().forEach(tr => tr.stop())
    return
  }
  mediaRecorder.ondataavailable = (e) => { if (e.data.size) recChunks.push(e.data) }
  mediaRecorder.onstop = onRecordStop
  mediaRecorder.start()
  recording.value = true
  recSecs.value = 0
  pasteMsg.value = ''
  recTimer = setInterval(() => {
    recSecs.value++
    if (recSecs.value >= 120) stopRecord() // 最长 2 分钟
  }, 1000)
}
function stopRecord () {
  clearInterval(recTimer)
  recording.value = false
  try { mediaRecorder?.stop() } catch { /* noop */ }
  recStream?.getTracks().forEach(tr => tr.stop())
  recStream = null
}
async function onRecordStop () {
  const blob = new Blob(recChunks, { type: mediaRecorder?.mimeType || 'audio/webm' })
  recChunks = []
  if (!blob.size) { pasteMsg.value = t('note.recordFailed'); return }
  transcribing.value = true
  pasteMsg.value = t('note.transcribing')
  try {
    const text = await store.transcribeAudio(blob)
    if (!text) {
      pasteMsg.value = t('note.transcribeEmpty')
    } else {
      const el = ta.value
      const at = el ? el.selectionStart : props.modelValue.length
      const v = insertAt(props.modelValue, at, text)
      emit('update:modelValue', v)
      emit('save', v)
      pasteMsg.value = ''
      nextTick(() => { el?.focus() })
    }
  } catch (err: any) {
    pasteMsg.value = err?.message || t('note.transcribeFailed')
  } finally {
    transcribing.value = false
  }
}
onUnmounted(() => { clearInterval(recTimer); try { mediaRecorder?.state === 'recording' && mediaRecorder.stop() } catch { /* noop */ } recStream?.getTracks().forEach(tr => tr.stop()) })
async function onPaste (e: ClipboardEvent) {
  const files = [...(e.clipboardData?.files || [])].filter(f => f.type.startsWith('image/'))
  if (!files.length || !props.uploadImage) return
  e.preventDefault()
  const el = ta.value
  const at = el ? el.selectionStart : props.modelValue.length
  pasteMsg.value = '图片上传中…'
  try {
    const parts: string[] = []
    for (const f of files) {
      const name = f.name && f.name !== 'image.png' ? f.name : `粘贴图片-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '')}.png`
      const url = await props.uploadImage(new File([f], name, { type: f.type }))
      parts.push(`![${name}](${url})`)
    }
    const v = insertAt(props.modelValue, at, parts.join('\n'))
    emit('update:modelValue', v)
    emit('save', v)
    pasteMsg.value = ''
  } catch (err: any) { pasteMsg.value = err?.message || '图片上传失败' }
}
function onInput (e: Event) {
  const v = (e.target as HTMLTextAreaElement).value
  emit('update:modelValue', v)
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => emit('save', v), 600) // 自动保存（去抖 600ms）
}
</script>

<style scoped>
.note-preview { min-height: 60px; padding: 8px; border: 1px solid var(--border); border-radius: 8px; }
.rec-dot { color: var(--danger-text, #f87171); font-size: 13px; font-weight: 600; animation: rec-blink 1s infinite; }
@keyframes rec-blink { 50% { opacity: 0.35; } }
</style>
