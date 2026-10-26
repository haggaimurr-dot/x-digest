<template>
  <div class="note-editor">
    <div class="note-toolbar" role="group" :aria-label="t('note.groupAria')">
      <span role="status" style="color:var(--text-muted);font-size:13px;align-self:center">{{ t('note.plainText') }}</span>
    </div>
    <textarea ref="ta" :value="modelValue" rows="3" :placeholder="t('note.ph')"
      :aria-label="t('note.aria')" @input="onInput" />
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{ modelValue: string }>()
const { t } = useLang()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void, (e: 'save', v: string): void }>()
let saveTimer: any = null
function onInput (e: Event) {
  const v = (e.target as HTMLTextAreaElement).value
  emit('update:modelValue', v)
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => emit('save', v), 600) // 自动保存（去抖 600ms）
}
</script>
