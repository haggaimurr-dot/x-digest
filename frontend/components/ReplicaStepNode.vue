<template>
  <!-- 子节点：图片样式的清单行（红色复选框 + 文字，点击文字展开详情）。 -->
  <div v-if="isChild">
    <div class="check-row">
      <button type="button" class="check-box" :class="{ checked: step.status === 'done' }"
        :title="t('node.childAriaStatus', { status: statusNames[step.status] })"
        :aria-label="t('node.childAriaStatus', { status: statusNames[step.status] })"
        @click="toggleChildStatus"><UiIcon v-if="step.status === 'done'" name="check" :size="13" :stroke="3" /></button>
      <span class="check-text" :class="{ 'is-done': step.status === 'done' }" :title="t('node.toggleChildDetail')" @click="childDetailOpen = !childDetailOpen">{{ tr(step.text) }}</span>
      <IconBtn class="btn-sm btn-quiet" icon="paperclip" :size="16" :badge="attCount || undefined" :label="t('resource.title')" :title="t('resource.title')" @click="openResource" />
      <div v-if="step.is_custom" class="node-menu-wrap">
        <IconBtn class="btn-sm btn-quiet node-menu-btn" icon="more" :size="18" :label="t('node.menu')" aria-haspopup="menu" :aria-expanded="openMenu === 'actions'" @click="toggleMenu('actions')" />
        <div v-if="openMenu === 'actions'" class="node-menu" role="menu">
          <button type="button" role="menuitem" class="menu-item-icon" @click="menuEdit"><UiIcon name="pencil" />{{ t('common.edit') }}</button>
          <button type="button" role="menuitem" class="danger menu-item-icon" @click="menuDelete"><UiIcon name="trash" />{{ t('common.delete') }}</button>
        </div>
      </div>
    </div>
    <div v-if="childDetailOpen" class="check-detail">
      <div v-if="editingText !== null" style="margin:6px 0">
        <input class="field" style="width:100%" type="text" v-model="editingText" :aria-label="t('node.editTextAria')" />
        <div style="margin-top:6px;display:flex;gap:6px">
          <IconBtn class="btn-sm btn-primary" icon="check" :label="t('node.saveText')" @click="saveStepText" />
          <IconBtn class="btn-sm" icon="x" :label="t('common.cancel')" @click="editingText = null" />
        </div>
      </div>
      <p v-if="(step.note || '').trim() && tr(step.note || '') !== (step.note || '')" class="note-translated"><span class="note-translated-label">{{ t('node.noteTranslation') }}</span>{{ tr(step.note || '') }}</p>
      <NoteEditor :model-value="step.note || ''" :upload-image="uploadPastedImage" @update:model-value="step.note = $event" @save="saveNote($event)" />
    </div>
    <ResourceManager v-model:open="resourceOpen" :replica-id="replica.id" :step-idx="step.idx" :items="resourceItems" />
  </div>

  <!-- 顶层节点：时间线卡片（圆标 + 卡片 + 状态徽标）。 -->
  <div v-else class="flow-step" :class="{ 'is-last': isLast, 'is-done': step.status === 'done', ['st-' + step.status]: true }">
    <div class="flow-rail">
      <button type="button" class="status-dot" :class="`is-${step.status}`"
        :title="t('node.statusClick', { status: statusNames[step.status] })" :aria-label="t('node.ariaStatus', { num, status: statusNames[step.status] })"
        @click="cycleStatus"><UiIcon v-if="step.status === 'done'" name="check" :size="15" :stroke="2.8" /><template v-else>{{ num.padStart(2, '0') }}</template></button>
      <span class="flow-line" v-if="!isLast" />
    </div>
    <div class="flow-card replica-node-card" style="flex:1;min-width:0">
      <div class="node-head">
        <div class="node-head-main">
          <div class="replica-step-text"><strong>{{ tr(step.text) }}</strong></div>
          <div v-if="subSummary" class="node-sub">{{ subSummary }}</div>
        </div>
        <div class="node-head-side">
          <span class="node-pill" :class="`pill-${step.status}`">{{ statusNames[step.status] }}</span>
          <div v-if="step.is_custom" class="node-menu-wrap">
            <IconBtn class="btn-sm btn-quiet node-menu-btn" icon="more" :size="18" :label="t('node.menu')" aria-haspopup="menu" :aria-expanded="openMenu === 'actions'" @click="toggleMenu('actions')" />
            <div v-if="openMenu === 'actions'" class="node-menu" role="menu">
              <button type="button" role="menuitem" class="menu-item-icon" :title="t('node.badgeTitleToggle')" @click="menuBadge"><UiIcon name="star" />{{ t('node.badgeCustom') }}</button>
              <button type="button" role="menuitem" class="menu-item-icon" @click="menuEdit"><UiIcon name="pencil" />{{ t('common.edit') }}</button>
              <button type="button" role="menuitem" class="danger menu-item-icon" @click="menuDelete"><UiIcon name="trash" />{{ t('common.delete') }}</button>
            </div>
          </div>
        </div>
      </div>
      <div v-if="editingText !== null" style="margin:6px 0">
        <input class="field" style="width:100%" type="text" v-model="editingText" :aria-label="t('node.editTextAria')" />
        <div style="margin-top:6px;display:flex;gap:6px">
          <IconBtn class="btn-sm btn-primary" icon="check" :label="t('node.saveText')" @click="saveStepText" />
          <IconBtn class="btn-sm" icon="x" :label="t('common.cancel')" @click="editingText = null" />
        </div>
      </div>
      <div v-if="step.tools || step.tech || step.prep" style="color:var(--text-muted);font-size:13px">
        <span v-if="step.tools">{{ t('node.tool') }}{{ step.tools }} </span>
        <span v-if="step.tech">{{ t('node.tech') }}{{ step.tech }} </span>
        <span v-if="step.prep">{{ t('node.prep') }}{{ step.prep }}</span>
      </div>
      <!-- 一直显示：没有任何详情的节点也要能展开，才能写备注、加子节点、上传附件。 -->
      <!-- 父节点卡片头直接给附件入口（📎），不用先展开详情；徽标显示附件数。 -->
      <div style="margin-top:8px;display:flex;gap:6px;align-items:center">
        <IconBtn class="btn-sm" :icon="detailsOpen ? 'chevron-up' : 'list'" :class="{ 'btn-on': detailsOpen }" :badge="!detailsOpen && subCount ? subCount : undefined" :label="detailsToggleLabel" :aria-expanded="detailsOpen" @click="detailsOpen = !detailsOpen" />
        <IconBtn class="btn-sm" icon="paperclip" :badge="attCount || undefined" :label="t('resource.title')" :title="t('resource.title')" @click="openResource" />
      </div>
        <div v-if="detailsOpen">
          <div v-if="children.length" class="check-list">
            <ReplicaStepNode v-for="(child, ci) in children" :key="child.idx" :replica="replica" :step="child"
              :num="`${num}.${ci + 1}`" :is-last="true" :is-child="true" />
          </div>
          <div v-if="(step.substeps || []).length" style="margin-top:10px">
            <div v-for="(sub, j) in step.substeps!.slice(0, 5)" :key="j" class="flow-text" style="font-size:13px;color:var(--flow-soft)">
              {{ `${num}.${j + 1} ` }}{{ trunc40(tr(sub.text)) }}
            </div>
          </div>
          <div style="margin-top:8px">
            <input class="field" style="width:100%" type="text" v-model="newChildText" :placeholder="t('node.childPh')" :aria-label="t('node.childAria')" />
            <div style="margin-top:6px">
              <IconBtn class="btn-sm btn-primary" icon="plus" :label="t('node.addChildBtn')" :disabled="!newChildText.trim()" @click="addChild" />
            </div>
          </div>
          <p v-if="(step.note || '').trim() && tr(step.note || '') !== (step.note || '')" class="note-translated"><span class="note-translated-label">{{ t('node.noteTranslation') }}</span>{{ tr(step.note || '') }}</p>
          <NoteEditor :model-value="step.note || ''" :upload-image="uploadPastedImage" @update:model-value="step.note = $event" @save="saveNote($event)" />
        </div>
    </div>
    <ResourceManager v-model:open="resourceOpen" :replica-id="replica.id" :step-idx="step.idx" :items="resourceItems" />
  </div>
  <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
</template>

<script setup lang="ts">
import type { Replica, ReplicaStep, Attachment } from '~/composables/types'
import { trunc40 } from '~/composables/useFormat'

const props = defineProps<{ replica: Replica, step: ReplicaStep, num: string, isLast: boolean, isChild?: boolean }>()
const store = useStore()
const { t, lang } = useLang()
const { tr, enqueue } = useTranslate()
const toast = ref('')
const statusNames = computed<Record<string, string>>(() => ({ idle: t('node.statusIdle'), active: t('node.statusActive'), done: t('node.statusDone') }))
const NEXT: Record<string, 'idle' | 'active' | 'done'> = { idle: 'active', active: 'done', done: 'idle' }
const editingText = ref<string | null>(null)
const newChildText = ref('')
// 资源管理弹窗：父子节点共用，增删改查都在里面。
const resourceOpen = ref(false)
const resourceItems = computed<Attachment[]>(() => props.step.attachments || (props.step.attachments = []))
const attCount = computed(() => resourceItems.value.length)
function openResource () { resourceOpen.value = true }
// 详情区：进行中的节点默认展开，状态变化时跟着走（完成变收起、开始变展开）。
const detailsOpen = ref(props.step.status === 'active')
watch(() => props.step.status, (s) => { detailsOpen.value = s === 'active' })
const childDetailOpen = ref(false)
// 汉堡菜单：节点操作（自建节点 / 编辑 / 删除），同一时间只开一个。
const openMenu = ref<'actions' | null>(null)
const menuId = useId()
function toggleMenu (which: 'actions') {
  if (openMenu.value === which) { openMenu.value = null; return }
  openMenu.value = which
  window.dispatchEvent(new CustomEvent('xd:close-node-menus', { detail: menuId }))
}
function closeMenu () { openMenu.value = null }
function menuBadge () { closeMenu(); onBadgeClick() }
function menuEdit () { closeMenu(); startEditText() }
function menuDelete () { closeMenu(); removeStep() }
function onDocClick (e: MouseEvent) {
  if (!openMenu.value) return
  if (!(e.target as HTMLElement).closest('.node-menu-wrap')) closeMenu()
}
function onKeyDown (e: KeyboardEvent) { if (e.key === 'Escape') closeMenu() }
function onCloseOthers (e: Event) { if ((e as CustomEvent).detail !== menuId) closeMenu() }
onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeyDown)
  window.addEventListener('xd:close-node-menus', onCloseOthers)
})
onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('xd:close-node-menus', onCloseOthers)
})

const children = computed(() => props.replica.steps.filter(s => s.parent_idx === props.step.idx))
const subCount = computed(() => children.value.length + (props.step.substeps || []).length)
const childDone = computed(() => children.value.filter(c => c.status === 'done').length)
// 节点副标题：有子节点时显示已完成 / 总数，没有时显示内联子步骤数。
const subSummary = computed(() => {
  if (children.value.length) return `${childDone.value}/${children.value.length} 项`
  const n = (props.step.substeps || []).length
  return n ? `${n} 项` : ''
})
const detailsToggleLabel = computed(() => {
  if (detailsOpen.value) return subCount.value ? t('node.collapseSub') : t('node.collapseDetail')
  return subCount.value ? t('node.expandSub', { n: subCount.value }) : t('node.expandDetail')
})
// Enqueue this node's display texts (text, note, substeps) for translation
// when EN is active. The editor below always shows/edits the original.
watch([() => props.step.text, () => props.step.note, () => props.step.substeps, lang], () => {
  enqueue([props.step.text, props.step.note || '', ...(props.step.substeps || []).map(s => s.text)])
}, { immediate: true })

function showToast (m: string) { toast.value = m; setTimeout(() => { toast.value = '' }, 3000) }
async function cycleStatus () {
  const next = NEXT[props.step.status] || 'idle'
  props.step.status = next
  try { await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, status: next }]) } catch (e: any) { showToast(e?.message || t('node.statusSaveFailed')) }
}
// 清单里的子节点：复选框只在已完成 / 未开始之间切换。
async function toggleChildStatus () {
  const next = props.step.status === 'done' ? 'idle' : 'done'
  props.step.status = next
  try { await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, status: next }]) } catch (e: any) { showToast(e?.message || t('node.statusSaveFailed')) }
}
// A pasted screenshot becomes an attachment of this step; the note links
// it inline (?inline=1 so the preview renders the image).
async function uploadPastedImage (file: File): Promise<string> {
  const made = await store.uploadAttachments(props.replica.id, props.step.idx, [file])
  props.step.attachments = [...(props.step.attachments || []), ...made]
  const att = made[0]
  if (!att) throw new Error('上传失败')
  return `${att.download_url}${att.download_url.includes('?') ? '&' : '?'}inline=1`
}
async function saveNote (note: string) {
  try { await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, note }]) } catch (e: any) { showToast(e?.message || t('node.noteSaveFailed')) }
}
function startEditText () { editingText.value = props.step.text }
// 点徽标要有反应：外层节点展开 / 收起详情，子节点没有子流程，点徽标改为编辑文字。
function onBadgeClick () {
  if (props.isChild) startEditText()
  else detailsOpen.value = !detailsOpen.value
}
async function saveStepText () {
  if (editingText.value === null) return
  const text = editingText.value.trim()
  if (!text) { showToast(t('node.textEmpty')); return }
  try {
    await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, text }])
    props.step.text = text
    editingText.value = null
    showToast(t('node.textSaved'))
  } catch (e: any) { showToast(e?.message || t('common.saveFailed')) }
}
async function removeStep () {
  if (!confirm(t('node.deleteConfirm', { child: props.isChild ? t('node.childWord') : '', text: props.step.text.slice(0, 20) }))) return
  try {
    await store.deleteReplicaStep(props.replica.id, props.step.idx)
    props.replica.steps = props.replica.steps.filter(s => s.idx !== props.step.idx && s.parent_idx !== props.step.idx)
    showToast(t('node.deleted'))
  } catch (e: any) { showToast(e?.message || t('common.deleteFailed')) }
}
async function addChild () {
  const text = newChildText.value.trim()
  if (!text) return
  try {
    const step = await store.addReplicaStep(props.replica.id, text, props.step.idx)
    props.replica.steps = [...props.replica.steps, step]
    newChildText.value = ''
    showToast(t('node.childAdded'))
  } catch (e: any) { showToast(e?.message || t('node.addChildFailed')) }
}
</script>
