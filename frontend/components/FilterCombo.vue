<template>
  <div class="combo" ref="rootRef">
    <label class="combo-label">{{ label }}</label>
    <div class="combo-box" @click="focusInput">
      <span v-for="v in modelValue" :key="v" class="combo-chip">
        {{ labelOf(v) }}
        <button type="button" @click.stop="remove(v)" :aria-label="`移除 ${labelOf(v)}`">✕</button>
      </span>
      <input ref="inputRef" v-model="query" type="text" :placeholder="modelValue.length ? '' : placeholder"
        :aria-label="label" @focus="open = true" @input="open = true"
        @keydown.escape="open = false" @keydown.enter.prevent="pickFirst" />
    </div>
    <ul v-if="open && suggestions.length" class="combo-list">
      <li v-for="opt in suggestions" :key="opt.value" @mousedown.prevent="pick(opt.value)">
        {{ opt.label }}<span v-if="opt.count != null" class="chip-count">{{ opt.count }}</span>
      </li>
    </ul>
    <p v-else-if="open && query.trim() && !suggestions.length" class="combo-empty">无匹配</p>
  </div>
</template>

<script setup lang="ts">
export interface ComboOption { value: string, label: string, count?: number }
const props = defineProps<{
  label: string
  placeholder?: string
  options: Array<string | ComboOption>
  modelValue: string[]
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string[]): void }>()
const query = ref('')
const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)

const normOptions = computed<ComboOption[]>(() => props.options.map(o =>
  typeof o === 'string' ? { value: o, label: o } : o))
function labelOf (v: string) { return normOptions.value.find(o => o.value === v)?.label || v }
const suggestions = computed(() => {
  const q = query.value.trim().toLowerCase()
  const rest = normOptions.value.filter(o => !props.modelValue.includes(o.value))
  const matched = q ? rest.filter(o => o.label.toLowerCase().includes(q)) : rest
  return matched.slice(0, 30)
})
function focusInput () { inputRef.value?.focus() }
function pick (v: string) {
  if (props.modelValue.includes(v)) return
  emit('update:modelValue', [...props.modelValue, v])
  query.value = ''
  open.value = true
  nextTick(() => inputRef.value?.focus())
}
function pickFirst () { if (suggestions.value.length) pick(suggestions.value[0].value) }
function remove (v: string) { emit('update:modelValue', props.modelValue.filter(x => x !== v)) }
function onDocClick (e: MouseEvent) {
  if (rootRef.value && !rootRef.value.contains(e.target as Node)) open.value = false
}
onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<style scoped>
.combo { position: relative; min-width: 180px; flex: 1 1 220px; }
.combo-label { display: block; font-size: 12px; color: var(--text-muted); margin-bottom: 4px; font-weight: 700; }
.combo-box { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; border: 1px solid var(--border); border-radius: 8px; padding: 4px 6px; background: var(--bg); min-height: 38px; cursor: text; }
.combo-box input { flex: 1; min-width: 90px; border: none; outline: none; background: transparent; color: var(--text); font: inherit; padding: 4px; }
.combo-chip { display: inline-flex; align-items: center; gap: 4px; background: var(--chip-bg); border: 1px solid var(--chip-border); border-radius: 9999px; padding: 2px 4px 2px 8px; font-size: 13px; white-space: nowrap; max-width: 100%; overflow: hidden; text-overflow: ellipsis; }
.combo-chip button { border: none; background: transparent; color: var(--text-muted); cursor: pointer; padding: 0 2px; font-size: 13px; flex-shrink: 0; }
.combo-list { position: absolute; z-index: 50; top: calc(100% + 4px); left: 0; right: 0; max-height: 240px; overflow-y: auto; background: var(--bg-elevated); border: 1px solid var(--border); border-radius: 10px; list-style: none; margin: 0; padding: 4px; box-shadow: 0 8px 24px rgba(0, 0, 0, .4); }
.combo-list li { padding: 8px 10px; border-radius: 6px; cursor: pointer; }
.combo-list li:hover { background: var(--bg); }
.combo-empty { position: absolute; z-index: 50; top: calc(100% + 4px); left: 0; right: 0; background: var(--bg-elevated); border: 1px solid var(--border); border-radius: 10px; margin: 0; padding: 8px 10px; color: var(--text-muted); font-size: 13px; }
</style>
