<template>
  <Teleport to="body">
    <div v-if="open" class="modal-backdrop" @click.self="emit('update:open', false)">
      <div class="modal-card resource-manager" role="dialog" aria-modal="true" :aria-label="t('resource.title')">
        <div class="resource-head">
          <h2 style="margin:0">{{ t('resource.title') }} · {{ items.length }}</h2>
          <IconBtn class="btn-sm btn-quiet" icon="x" :size="18" :label="t('common.close')" @click="emit('update:open', false)" />
        </div>
        <p class="resource-hint">{{ t('node.uploadHint') }}</p>
        <div v-if="!items.length" class="empty" style="margin:12px 0">{{ t('resource.empty') }}</div>
        <div v-else class="resource-list">
          <div v-for="att in items" :key="att.id" class="resource-item">
            <img v-if="isImage(att)" class="resource-thumb" :src="thumbUrl(att)" :alt="att.name" loading="lazy" @click="previewAtt = att" />
            <div v-else class="resource-thumb resource-file" @click="previewAtt = att"><UiIcon name="file" :size="22" /></div>
            <div class="resource-main">
              <div v-if="editingId === att.id" class="resource-rename">
                <input ref="renameInput" class="field" v-model="editingName" :aria-label="t('resource.renameAria')" @keyup.enter="saveRename(att)" @keyup.escape="editingId = null" />
                <IconBtn class="btn-sm btn-primary" icon="check" :label="t('common.save')" @click="saveRename(att)" />
                <IconBtn class="btn-sm" icon="x" :label="t('common.cancel')" @click="editingId = null" />
              </div>
              <div v-else class="resource-name" :title="att.name" @click="previewAtt = att">{{ att.name }}</div>
              <div class="resource-meta">{{ fmtSize(att.size) }} · {{ fmtTime(att.created_at) }}</div>
            </div>
            <div class="node-menu-wrap">
              <IconBtn class="btn-sm btn-quiet node-menu-btn" icon="more" :size="18" :label="t('node.menu')" :aria-expanded="menuId === att.id" aria-haspopup="menu" @click="menuId = menuId === att.id ? null : att.id" />
              <div v-if="menuId === att.id" class="node-menu" role="menu">
                <button type="button" role="menuitem" class="menu-item-icon" @click="doPreview(att)"><UiIcon name="eye" />{{ t('common.preview') }}</button>
                <button type="button" role="menuitem" class="menu-item-icon" @click="startRename(att)"><UiIcon name="pencil" />{{ t('resource.rename') }}</button>
                <a role="menuitem" class="menu-item-icon" :href="att.download_url" download @click="menuId = null"><UiIcon name="download" />{{ t('common.download') }}</a>
                <button type="button" role="menuitem" class="danger menu-item-icon" @click="askDelete(att)"><UiIcon name="trash" />{{ t('common.delete') }}</button>
              </div>
            </div>
          </div>
        </div>
        <div class="resource-upload-row">
          <label class="btn-sm btn-primary with-icon" role="button" tabindex="0"><UiIcon name="image" :size="16" />{{ t('node.uploadImage') }}
            <input type="file" accept="image/*" multiple hidden @change="upload($event)" />
          </label>
          <label class="btn-sm with-icon" role="button" tabindex="0"><UiIcon name="file" :size="16" />{{ t('node.uploadDocs') }}
            <input type="file" multiple hidden :accept="acceptDocs" @change="upload($event)" />
          </label>
        </div>
        <p v-if="toast" class="resource-toast">{{ toast }}</p>
      </div>
    </div>
    <AttachmentPreview v-if="previewAtt" :key="previewAtt.id" :attachment="previewAtt" @close="previewAtt = null" />
    <div v-if="deleteTarget" class="modal-backdrop" @click.self="deleteTarget = null">
      <div class="modal-card" role="alertdialog" aria-modal="true">
        <h2>{{ t('resource.deleteTitle') }}</h2>
        <p>{{ t('resource.deleteBody', { name: deleteTarget.name }) }}</p>
        <div class="modal-actions">
          <IconBtn icon="x" :label="t('common.cancel')" @click="deleteTarget = null" />
          <IconBtn class="btn-danger" icon="trash" :label="t('common.delete')" @click="doDelete" />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import type { Attachment } from '~/composables/types'
import { IMAGE_EXTS, ATTACHMENT_EXTS } from '~/composables/useStore'

const props = defineProps<{ open: boolean, replicaId: string, stepIdx: number, items: Attachment[] }>()
const emit = defineEmits<{ 'update:open': [value: boolean], change: [] }>()
const store = useStore()
const { t } = useLang()

const acceptDocs = '.' + ATTACHMENT_EXTS.filter(e => !IMAGE_EXTS.includes(e)).join(',.')
const menuId = ref<string | null>(null)
const previewAtt = ref<Attachment | null>(null)
const deleteTarget = ref<Attachment | null>(null)
const editingId = ref<string | null>(null)
const editingName = ref('')
const renameInput = ref<HTMLInputElement | null>(null)
const toast = ref('')

function showToast (m: string) { toast.value = m; setTimeout(() => { toast.value = '' }, 2500) }
function isImage (a: Attachment) { return IMAGE_EXTS.includes((a.ext || '').toLowerCase()) }
function thumbUrl (a: Attachment) { return `${a.download_url}${a.download_url.includes('?') ? '&' : '?'}inline=1` }
function fmtSize (n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1048576) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1048576).toFixed(1)} MB`
}
function fmtTime (s: string) { return (s || '').slice(0, 16).replace('T', ' ') }
function doPreview (a: Attachment) { menuId.value = null; previewAtt.value = a }

function startRename (a: Attachment) {
  menuId.value = null
  editingId.value = a.id
  editingName.value = a.name
  nextTick(() => { renameInput.value?.focus(); renameInput.value?.select() })
}
async function saveRename (a: Attachment) {
  const name = editingName.value.trim()
  editingId.value = null
  if (!name || name === a.name) return
  try {
    const updated = await store.renameAttachment(a.id, name)
    a.name = updated.name
    emit('change')
  } catch (e: any) { showToast(e?.message || t('resource.renameFailed')) }
}
function askDelete (a: Attachment) { menuId.value = null; deleteTarget.value = a }
async function doDelete () {
  const a = deleteTarget.value
  deleteTarget.value = null
  if (!a) return
  try {
    await store.deleteAttachment(a.id)
    const i = props.items.findIndex(x => x.id === a.id)
    if (i >= 0) props.items.splice(i, 1)
    if (previewAtt.value?.id === a.id) previewAtt.value = null
    emit('change')
  } catch (e: any) { showToast(e?.message || t('node.deleteAttFailed')) }
}
async function upload (e: Event) {
  const input = e.target as HTMLInputElement
  const files = [...(input.files || [])]
  input.value = ''
  if (!files.length) return
  try {
    const made = await store.uploadAttachments(props.replicaId, props.stepIdx, files)
    props.items.push(...made)
    emit('change')
    showToast(t('node.uploaded', { n: made.length }))
  } catch (e: any) { showToast(e?.message || t('node.uploadFailed')) }
}
watch(() => props.open, (v) => { if (!v) { menuId.value = null; previewAtt.value = null; editingId.value = null } })
</script>

<style scoped>
.resource-manager { max-width: 520px; width: calc(100vw - 32px); max-height: 80vh; display: flex; flex-direction: column; }
.resource-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.resource-hint { color: var(--text-muted); font-size: 13px; margin: 6px 0 0; }
.resource-list { overflow-y: auto; margin: 10px 0; display: flex; flex-direction: column; gap: 8px; }
.resource-item { display: flex; gap: 10px; align-items: center; border: 1px solid var(--border); border-radius: 10px; padding: 8px; }
.resource-thumb { width: 48px; height: 48px; border-radius: 8px; object-fit: cover; cursor: pointer; flex-shrink: 0; background: var(--bg-muted); }
.resource-file { display: flex; align-items: center; justify-content: center; color: var(--text-muted); }
.resource-main { flex: 1; min-width: 0; }
.resource-name { font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.resource-meta { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
.resource-rename { display: flex; gap: 6px; align-items: center; }
.resource-rename .field { flex: 1; min-width: 0; }
.resource-upload-row { display: flex; gap: 8px; margin-top: 4px; }
.resource-upload-row label { cursor: pointer; display: inline-flex; align-items: center; gap: 6px; }
.resource-toast { text-align: center; color: var(--text-muted); font-size: 13px; margin: 8px 0 0; }
.with-icon { display: inline-flex; align-items: center; gap: 6px; }
</style>
