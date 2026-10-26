<template>
  <div class="flow-step" :class="{ 'is-last': isLast, 'is-done': step.status === 'done', ['st-' + step.status]: true }">
    <div class="flow-rail">
      <button type="button" class="status-dot" :class="`is-${step.status}`"
        :title="t('node.statusClick', { status: statusNames[step.status] })" :aria-label="t('node.ariaStatus', { num, status: statusNames[step.status] })"
        @click="cycleStatus" />
      <span class="flow-line" v-if="!isLast" />
    </div>
    <div class="flow-card replica-node-card" style="flex:1;min-width:0">
      <div class="replica-step-text"><strong>{{ num }}. {{ tr(step.text) }}</strong></div>
      <div v-if="step.is_custom" class="node-actions">
        <span style="flex:1" />
        <div class="node-menu-wrap">
          <button class="btn btn-sm btn-quiet node-menu-btn" type="button" @click="toggleMenu('actions')" :aria-label="t('node.menu')" aria-haspopup="menu" :aria-expanded="openMenu === 'actions'">☰</button>
          <div v-if="openMenu === 'actions'" class="node-menu" role="menu">
            <button v-if="!isChild" type="button" role="menuitem" :title="t('node.badgeTitleToggle')" @click="menuBadge">{{ t('node.badgeCustom') }}</button>
            <button type="button" role="menuitem" @click="menuEdit">{{ t('common.edit') }}</button>
            <button type="button" role="menuitem" class="danger" @click="menuDelete">{{ t('common.delete') }}</button>
          </div>
        </div>
      </div>
      <div v-if="editingText !== null" style="margin:6px 0">
        <input class="field" style="width:100%" type="text" v-model="editingText" :aria-label="t('node.editTextAria')" />
        <div style="margin-top:6px;display:flex;gap:6px">
          <button class="btn btn-sm btn-primary" type="button" @click="saveStepText">{{ t('node.saveText') }}</button>
          <button class="btn btn-sm" type="button" @click="editingText = null">{{ t('common.cancel') }}</button>
        </div>
      </div>
      <div v-if="step.tools || step.tech || step.prep" style="color:var(--text-muted);font-size:13px">
        <span v-if="step.tools">{{ t('node.tool') }}{{ step.tools }} </span>
        <span v-if="step.tech">{{ t('node.tech') }}{{ step.tech }} </span>
        <span v-if="step.prep">{{ t('node.prep') }}{{ step.prep }}</span>
      </div>
      <div v-if="!isChild" style="margin-top:8px">
        <button class="btn btn-sm" type="button" @click="childOpen = !childOpen">
          {{ childOpen ? t('node.collapseSub') : (subCount ? t('node.expandSub', { n: subCount }) : t('node.addChild')) }}
        </button>
        <div v-if="childOpen">
          <div v-if="children.length" class="flow-list" style="margin-top:8px">
            <ReplicaStepNode v-for="(child, ci) in children" :key="child.idx" :replica="replica" :step="child"
              :num="`${num}.${ci + 1}`" :is-last="ci === children.length - 1 && !(step.substeps || []).length" :is-child="true" />
          </div>
          <div v-if="(step.substeps || []).length" class="flow-list" style="margin-top:8px">
            <div v-for="(sub, j) in step.substeps!.slice(0, 5)" :key="j" class="flow-step" :class="{ 'is-last': j === Math.min(step.substeps!.length, 5) - 1 }">
              <div class="flow-rail">
                <span class="flow-node" />
                <span class="flow-line" v-if="j < Math.min(step.substeps!.length, 5) - 1" />
              </div>
              <div class="flow-card" style="padding:8px 10px">
                <div class="flow-text" style="font-size:13px">{{ trunc40(tr(sub.text)) }}</div>
              </div>
            </div>
          </div>
          <div style="margin-top:8px">
            <input class="field" style="width:100%" type="text" v-model="newChildText" :placeholder="t('node.childPh')" :aria-label="t('node.childAria')" />
            <div style="margin-top:6px">
              <button class="btn btn-sm btn-primary" type="button" :disabled="!newChildText.trim()" @click="addChild">{{ t('node.addChildBtn') }}</button>
            </div>
          </div>
        </div>
      </div>
      <p v-if="isEn && (step.note || '').trim()" class="note-translated"><span class="note-translated-label">{{ t('node.noteTranslation') }}</span>{{ tr(step.note || '') }}</p>
      <NoteEditor :model-value="step.note || ''" @update:model-value="step.note = $event" @save="saveNote($event)" />
      <div style="margin-top:8px;display:flex;align-items:center;gap:6px">
        <div class="node-menu-wrap">
          <button class="btn btn-sm btn-quiet node-menu-btn" type="button" @click="toggleMenu('upload')" :aria-label="t('node.uploadMenu')" aria-haspopup="menu" :aria-expanded="openMenu === 'upload'">☰</button>
          <div v-if="openMenu === 'upload'" class="node-menu left" role="menu">
            <label role="menuitem">{{ t('node.uploadImage') }}
              <input type="file" accept="image/*" multiple hidden @change="upload($event)" />
            </label>
            <label role="menuitem">{{ t('node.uploadDocs') }}
              <input type="file" multiple hidden :accept="acceptDocs" @change="upload($event)" />
            </label>
          </div>
        </div>
        <span style="color:var(--text-muted);font-size:13px">{{ t('node.uploadHint') }}</span>
      </div>
      <div v-if="(step.attachments || []).length">
        <div v-for="att in step.attachments" :key="att.id" class="attachment-row">
          <span class="attachment-name">{{ att.name }}</span>
          <div class="node-menu-wrap">
            <button class="btn btn-sm btn-quiet node-menu-btn" type="button" @click="toggleAttMenu(att.id)" :aria-label="t('node.menu')" aria-haspopup="menu" :aria-expanded="attMenuId === att.id">☰</button>
            <div v-if="attMenuId === att.id" class="node-menu" role="menu">
              <button type="button" role="menuitem" @click="menuPreviewAtt(att)">{{ previewAtt?.id === att.id ? t('common.collapsePreview') : t('common.preview') }}</button>
              <a role="menuitem" :href="att.download_url" download @click="attMenuId = null">{{ t('common.download') }}</a>
              <button type="button" role="menuitem" class="danger" @click="menuDeleteAtt(att)">{{ t('node.deleteAtt') }}</button>
            </div>
          </div>
        </div>
        <AttachmentPreview v-if="previewAtt && step.attachments!.some(a => a.id === previewAtt!.id)" :key="previewAtt.id" :attachment="previewAtt" @close="previewAtt = null" />
      </div>
    </div>
  </div>
  <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
</template>

<script setup lang="ts">
import type { Replica, ReplicaStep, Attachment } from '~/composables/types'
import { trunc40 } from '~/composables/useFormat'
import { ATTACHMENT_EXTS, IMAGE_EXTS } from '~/composables/useStore'

const props = defineProps<{ replica: Replica, step: ReplicaStep, num: string, isLast: boolean, isChild?: boolean }>()
const store = useStore()
const { t, isEn } = useLang()
const { tr, enqueue } = useTranslate()
const toast = ref('')
const statusNames = computed<Record<string, string>>(() => ({ idle: t('node.statusIdle'), active: t('node.statusActive'), done: t('node.statusDone') }))
const NEXT: Record<string, 'idle' | 'active' | 'done'> = { idle: 'active', active: 'done', done: 'idle' }
const acceptDocs = '.' + ATTACHMENT_EXTS.filter(e => !IMAGE_EXTS.includes(e)).join(',.')
const editingText = ref<string | null>(null)
const childOpen = ref(false)
const newChildText = ref('')
const previewAtt = ref<Attachment | null>(null)
// 汉堡菜单：节点操作（自建节点 / 编辑 / 删除）与上传（图片 / 文档）
// 各收进一个 ☰ 下拉菜单，同一时间只开一个。
const openMenu = ref<'actions' | 'upload' | null>(null)
const attMenuId = ref<string | null>(null)
const menuId = useId()
function toggleMenu (which: 'actions' | 'upload') {
  if (openMenu.value === which) { openMenu.value = null; return }
  openMenu.value = which
  attMenuId.value = null
  window.dispatchEvent(new CustomEvent('xd:close-node-menus', { detail: menuId }))
}
function toggleAttMenu (id: string) {
  if (attMenuId.value === id) { attMenuId.value = null; return }
  attMenuId.value = id
  openMenu.value = null
  window.dispatchEvent(new CustomEvent('xd:close-node-menus', { detail: menuId }))
}
function closeMenu () { openMenu.value = null; attMenuId.value = null }
function menuBadge () { closeMenu(); onBadgeClick() }
function menuEdit () { closeMenu(); startEditText() }
function menuDelete () { closeMenu(); removeStep() }
function menuPreviewAtt (att: Attachment) {
  previewAtt.value = previewAtt.value?.id === att.id ? null : att
  attMenuId.value = null
}
function menuDeleteAtt (att: Attachment) {
  attMenuId.value = null
  removeAttachment(att)
}
function onDocClick (e: MouseEvent) {
  if (!openMenu.value && !attMenuId.value) return
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
// Enqueue this node's display texts (text, note, substeps) for translation
// when EN is active. The editor below always shows/edits the original.
watch([() => props.step.text, () => props.step.note, () => props.step.substeps, isEn], () => {
  if (!isEn.value) return
  enqueue([props.step.text, props.step.note || '', ...(props.step.substeps || []).map(s => s.text)])
}, { immediate: true })

function showToast (m: string) { toast.value = m; setTimeout(() => { toast.value = '' }, 3000) }
async function cycleStatus () {
  const next = NEXT[props.step.status] || 'idle'
  props.step.status = next
  try { await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, status: next }]) } catch (e: any) { showToast(e?.message || t('node.statusSaveFailed')) }
}
async function saveNote (note: string) {
  try { await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, note }]) } catch (e: any) { showToast(e?.message || t('node.noteSaveFailed')) }
}
async function upload (e: Event) {
  const input = e.target as HTMLInputElement
  const files = [...(input.files || [])]
  input.value = ''
  if (!files.length) return
  closeMenu()
  try {
    const made = await store.uploadAttachments(props.replica.id, props.step.idx, files)
    props.step.attachments = [...(props.step.attachments || []), ...made]
    showToast(t('node.uploaded', { n: made.length }))
  } catch (err: any) { showToast(err?.message || t('node.uploadFailed')) }
}
async function removeAttachment (att: Attachment) {
  if (!confirm(t('node.deleteAttConfirm', { name: att.name }))) return
  try {
    await store.deleteAttachment(att.id)
    props.step.attachments = (props.step.attachments || []).filter(a => a.id !== att.id)
    if (previewAtt.value?.id === att.id) previewAtt.value = null
  } catch (err: any) { showToast(err?.message || t('node.deleteAttFailed')) }
}
function startEditText () { editingText.value = props.step.text }
// 点徽标要有反应：外层节点展开 / 收起子流程，子节点没有子流程，点徽标改为编辑文字。
function onBadgeClick () {
  if (props.isChild) startEditText()
  else childOpen.value = !childOpen.value
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
