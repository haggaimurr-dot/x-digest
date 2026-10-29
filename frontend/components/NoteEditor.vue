<template>
  <div class="note-editor">
    <div class="note-toolbar" role="group" :aria-label="t('note.groupAria')">
      <span role="status" style="color:var(--text-muted);font-size:13px;align-self:center">{{ t('note.markdown') }}</span>
      <IconBtn class="btn-sm" :icon="preview ? 'pencil' : 'eye'" :label="preview ? t('note.edit') : t('note.preview')" :aria-pressed="preview" @click="preview = !preview" />
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
</style>
