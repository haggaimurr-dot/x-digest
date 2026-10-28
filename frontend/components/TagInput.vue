<template>
  <div class="tag-input">
    <div class="tag-input-box" @click="inputEl?.focus()">
      <span v-for="tg in modelValue" :key="tg" class="tag-input-chip">
        {{ tg }}
        <button type="button" class="tag-input-x" :aria-label="`移除 ${tg}`" @click.stop="remove(tg)">×</button>
      </span>
      <input ref="inputEl" v-model="draft" class="tag-input-field" type="text" :placeholder="modelValue.length ? '' : placeholder"
        @keydown.enter.prevent="add(draft)" @keydown="onKey" @blur="add(draft)" />
    </div>
    <div v-if="restSuggestions.length" class="tag-input-suggest">
      <span class="tag-input-suggest-label">常用：</span>
      <button v-for="tg in restSuggestions" :key="tg" type="button" class="chip tag-input-suggest-chip" @click="add(tg)">+ {{ tg }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
// Chip-style tag editor: type and press Enter (or comma) to add, × to
// remove, click a suggestion to add it. Emits the full new list on every
// change; the parent saves it.
const props = withDefaults(defineProps<{ modelValue: string[], suggestions?: string[], placeholder?: string }>(), {
  suggestions: () => [], placeholder: '输入标签后回车'
})
const emit = defineEmits<{ (e: 'update:modelValue', v: string[]): void }>()
const draft = ref('')
const inputEl = ref<HTMLInputElement | null>(null)
const restSuggestions = computed(() => props.suggestions.filter(s => !props.modelValue.includes(s)))
function add (raw: string) {
  const parts = raw.split(/[,，、]/).map(s => s.trim()).filter(Boolean)
  draft.value = ''
  const next = [...props.modelValue]
  for (const p of parts) if (!next.includes(p)) next.push(p)
  if (next.length !== props.modelValue.length) emit('update:modelValue', next)
}
function remove (tg: string) { emit('update:modelValue', props.modelValue.filter(x => x !== tg)) }
function onKey (e: KeyboardEvent) {
  if (e.key === ',' || e.key === '，') { e.preventDefault(); add(draft.value) }
  if (e.key === 'Backspace' && !draft.value && props.modelValue.length) remove(props.modelValue[props.modelValue.length - 1])
}
</script>
