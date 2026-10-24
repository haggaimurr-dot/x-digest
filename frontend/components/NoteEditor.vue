<template>
  <div class="note-editor">
    <div class="note-toolbar" role="group" aria-label="Markdown 格式工具">
      <button v-for="t in tools" :key="t.action" type="button" :title="t.label" @click="applyTool(t.action)">{{ t.label }}</button>
      <button type="button" @click="mode = mode === 'edit' ? 'preview' : 'edit'">{{ mode === 'edit' ? '预览' : '编辑' }}</button>
      <span role="status" style="color:var(--text-muted);font-size:13px;align-self:center">Markdown · 自动保存</span>
    </div>
    <textarea v-if="mode === 'edit'" ref="ta" :value="modelValue" rows="4" placeholder="写点备注… 支持 Markdown"
      aria-label="步骤备注" @input="onInput" />
    <div v-else class="rich-content" v-html="previewHtml" />
  </div>
</template>

<script setup lang="ts">
import { marked } from 'marked'
const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void, (e: 'save', v: string): void }>()
const mode = ref<'edit' | 'preview'>('edit')
const ta = ref<HTMLTextAreaElement | null>(null)
const tools = [
  { action: 'bold', label: '加粗' }, { action: 'italic', label: '斜体' }, { action: 'heading', label: '标题' },
  { action: 'list', label: '列表' }, { action: 'link', label: '链接' }, { action: 'code', label: '代码' }, { action: 'quote', label: '引用' }
]
let saveTimer: any = null
function onInput (e: Event) {
  const v = (e.target as HTMLTextAreaElement).value
  emit('update:modelValue', v)
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => emit('save', v), 600) // 自动保存（去抖 600ms）
}
const previewHtml = computed(() => marked.parse(props.modelValue || '') as string)
function wrap (before: string, after: string, placeholder = '文本') {
  const el = ta.value
  if (!el) return
  const s = el.selectionStart, e = el.selectionEnd
  const sel = props.modelValue.slice(s, e) || placeholder
  const next = props.modelValue.slice(0, s) + before + sel + after + props.modelValue.slice(e)
  emit('update:modelValue', next)
  emit('save', next)
  nextTick(() => { el.focus(); el.setSelectionRange(s + before.length, s + before.length + sel.length) })
}
function applyTool (action: string) {
  if (action === 'bold') wrap('**', '**')
  else if (action === 'italic') wrap('*', '*')
  else if (action === 'heading') wrap('## ', '', '标题')
  else if (action === 'list') wrap('- ', '', '列表项')
  else if (action === 'link') wrap('[', '](https://)', '链接文字')
  else if (action === 'code') wrap('`', '`', '代码')
  else if (action === 'quote') wrap('> ', '', '引用')
}
</script>
