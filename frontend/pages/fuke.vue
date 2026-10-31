<template>
  <div>
    <main style="max-width:600px;margin:0 auto;padding:16px;min-width:0">
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">{{ t('common.loading') }}</p>
      <p v-if="!loading" style="color:var(--text-muted)">{{ t('fuke.total', { n: replicas.length }) }}</p>
      <!-- 进度看板：总数 / 进行中 / 已完成 / 未开始 / 平均完成率（按顶层节点） -->
      <div v-if="!loading && replicas.length" class="fuke-board">
        <div><strong>{{ board.total }}</strong><span>全部</span></div>
        <div><strong>{{ board.inProgress }}</strong><span>进行中</span></div>
        <div><strong>{{ board.done }}</strong><span>已完成</span></div>
        <div><strong>{{ board.notStarted }}</strong><span>未开始</span></div>
        <div><strong>{{ board.avgRate }}%</strong><span>平均完成率</span></div>
      </div>
      <p v-if="translating" style="color:var(--text-muted)">{{ t('common.translating') }}</p>
      <div class="filter-row" style="margin-bottom:12px">
        <label>{{ t('fuke.sort') }}
          <select class="field" v-model="sort" @change="reload">
            <option value="time">{{ t('fuke.sortImportedDesc') }}</option>
            <option value="time_asc">{{ t('fuke.sortImportedAsc') }}</option>
            <option value="edited">{{ t('fuke.sortEdited') }}</option>
            <option value="progress">{{ t('fuke.sortProgress') }}</option>
          </select>
        </label>
      </div>
      <div style="margin-bottom:12px;border:1px solid var(--border);border-radius:12px;padding:12px">
        <div class="chip-group-title" style="margin-top:0">{{ t('fuke.newFlowTitle') }}</div>
        <div class="filter-row">
          <input class="field grow" style="flex:1" type="text" v-model="newFlowTitle" :placeholder="t('fuke.newFlowPh')" :aria-label="t('fuke.newFlowAria')" />
          <IconBtn class="btn-sm btn-primary" icon="plus" :label="t('fuke.createFlow')" :disabled="!newFlowTitle.trim()" @click="createFlow" />
        </div>
      </div>
      <p v-if="!loading && !replicas.length" class="empty">{{ t('fuke.empty') }}</p>

      <article v-for="r in replicas" :key="r.id" class="post-card flow-sheet" :class="{ 'is-collapsed': isCollapsed(r.id) }" style="margin-bottom:16px;display:block">
        <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:8px">
          <div class="flow-eyebrow">FLOW · {{ topSteps(r).length }} NODES</div>
          <div class="flow-head-side">
          <IconBtn class="btn-sm btn-quiet flow-collapse-btn" :icon="isCollapsed(r.id) ? 'chevron-right' : 'chevron-down'" :size="18" :label="isCollapsed(r.id) ? t('fuke.expandFlow') : t('fuke.collapseFlow')" :aria-expanded="!isCollapsed(r.id)" @click="toggleCollapsed(r.id)" />
          <div class="node-menu-wrap flow-menu-wrap">
            <IconBtn class="btn-sm btn-quiet node-menu-btn" icon="more" :size="18" :label="t('fuke.flowMenu')" aria-haspopup="menu" :aria-expanded="openFlowMenu === r.id" @click="toggleFlowMenu(r.id)" />
            <div v-if="openFlowMenu === r.id" class="node-menu" role="menu">
              <button type="button" role="menuitem" class="menu-item-icon" @click="menuExport(r)"><UiIcon name="file-down" />导出 Markdown</button>
              <button type="button" role="menuitem" class="menu-item-icon" @click="menuClone(r)"><UiIcon name="copy" />复制为新流程</button>
              <button type="button" role="menuitem" class="danger menu-item-icon" @click="menuDeleteFlow(r)"><UiIcon name="trash" />{{ t('fuke.deleteReplica') }}</button>
            </div>
          </div>
          </div>
        </div>
        <h2 class="flow-title">
          <a v-if="r.post_url" :href="r.post_url" target="_blank" rel="noopener" style="color:inherit;text-decoration:none">{{ tr(r.post_title) }}</a>
          <span v-else>{{ tr(r.post_title) }}</span>
        </h2>
        <div class="flow-meta">
          <span v-if="r.post_id">{{ r.post_author_name }} @{{ r.post_author }} · </span>
          <span v-else>{{ t('fuke.customFlow') }}</span>{{ shanghaiTime(r.created_at) }}
        </div>
        <div class="flow-progress-row">
          <span>{{ t('fuke.flowProgress', { done: progress(r).done, total: progress(r).total }) }}</span>
          <span>{{ progressPct(r) }}%</span>
        </div>
        <div class="progress-bar"><div :style="{ width: progressPct(r) + '%' }" /></div>
        <template v-if="!isCollapsed(r.id)">
        <div style="margin:12px 0 4px">
          <div class="chip-group-title" style="margin-top:0">{{ t('fuke.customNodes') }}</div>
          <input class="field" style="width:100%" type="text" v-model="newStepText[r.id]" :placeholder="t('fuke.newStepPh')" :aria-label="t('fuke.newStepAria')" />
          <div style="margin-top:6px">
            <IconBtn class="btn-sm btn-primary" icon="plus" :label="t('fuke.addStep')" :disabled="!((newStepText[r.id] || '').trim())" @click="addStep(r)" />
          </div>
        </div>
        <div class="flow-list" style="margin-top:12px">
          <p v-if="!topSteps(r).length" style="color:var(--text-muted)">{{ t('fuke.noSteps') }}</p>
          <ReplicaStepNode v-for="(step, si) in topSteps(r)" :key="step.idx" :replica="r" :step="step"
            :num="String(si + 1)" :is-last="si === topSteps(r).length - 1" />
        </div>
        <div v-if="topSteps(r).length" class="flow-action">
          <div class="flow-current">{{ currentNode(r) ? t('fuke.currentNode', { num: currentNum(r), title: tr(currentNode(r)!.text) }) : t('fuke.allDone') }}</div>
          <button v-if="currentNode(r)" class="flow-complete-btn with-icon" type="button" @click="completeCurrent(r)"><UiIcon name="circle-check" :size="18" />{{ currentActionLabel(r) }}</button>
          <button v-else class="flow-complete-btn with-icon" type="button" disabled><UiIcon name="check-check" :size="18" />{{ t('fuke.allDone') }}</button>
        </div>
        </template>
      </article>
    </main>

    <div v-if="confirmTarget" class="modal-backdrop" @click.self="confirmTarget = null">
      <div class="modal-card" role="alertdialog" aria-modal="true" :aria-label="t('fuke.confirmDeleteAria')">
        <h2>{{ t('fuke.deleteTitle') }}</h2>
        <p>{{ t('fuke.deleteBody', { title: tr(confirmTarget.post_title) }) }}</p>
        <div class="modal-actions">
          <IconBtn icon="x" :label="t('common.cancel')" @click="confirmTarget = null" />
          <IconBtn class="btn-danger" icon="trash" :label="t('common.delete')" @click="doDelete" />
        </div>
      </div>
    </div>
    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import { replicaBoard, replicaMarkdown } from '~/utils/replicaExport'
import type { Replica, ReplicaStep } from '~/composables/types'
import { shanghaiTime } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'
import { usePageActions } from '~/composables/usePageActions'

const store = useStore()
const { t, lang } = useLang()
const { tr, enqueue, translating } = useTranslate()
// 页面级操作放进侧边菜单（AppDrawer）
usePageActions(() => [
  { key: 'back', icon: 'arrow-left', label: t('fuke.backToDaily'), run: () => { navigateTo('/') } }
])
const loading = ref(true)
const loadError = ref('')
const replicas = ref<Replica[]>([])
const sort = ref<'time' | 'time_asc' | 'edited' | 'progress'>('time')
const confirmTarget = ref<Replica | null>(null)
useScrollLock(computed(() => confirmTarget.value !== null))
const toast = ref('')
const openFlowMenu = ref<string | null>(null)

function showToast (m: string) { toast.value = m; setTimeout(() => { toast.value = '' }, 3000) }
function topSteps (r: Replica): ReplicaStep[] { return r.steps.filter(s => s.parent_idx == null) }
// 进度按顶层节点算（图片里的 FLOW · N NODES / 已完成 x / y 个节点）。
function progress (r: Replica) {
  const tops = topSteps(r)
  const total = tops.length
  const done = tops.filter(s => s.status === 'done').length
  return { done, total }
}
function progressPct (r: Replica) {
  const { done, total } = progress(r)
  return total ? Math.round(done / total * 100) : 0
}
// 当前节点：第一个进行中的顶层节点，否则第一个未开始的；全完成时为 null。
function currentNode (r: Replica): ReplicaStep | null {
  const tops = topSteps(r)
  return tops.find(s => s.status === 'active') || tops.find(s => s.status === 'idle') || null
}
function currentNum (r: Replica): string {
  const cur = currentNode(r)
  if (!cur) return ''
  return String(topSteps(r).indexOf(cur) + 1).padStart(2, '0')
}
function currentActionLabel (r: Replica): string {
  const cur = currentNode(r)
  if (!cur) return t('fuke.allDone')
  return cur.status === 'active' ? t('fuke.completeNode', { title: tr(cur.text) }) : t('fuke.startNode', { title: tr(cur.text) })
}
// 底部大按钮：进行中的节点标为完成并激活下一个未开始的；未开始的则直接开始。
async function completeCurrent (r: Replica) {
  const cur = currentNode(r)
  if (!cur) return
  const tops = topSteps(r)
  const patches: Array<{ idx: number, status: 'idle' | 'active' | 'done' }> = []
  if (cur.status === 'active') {
    patches.push({ idx: cur.idx, status: 'done' })
    const next = tops.slice(tops.indexOf(cur) + 1).find(s => s.status === 'idle')
      || tops.find(s => s.status === 'idle' && s.idx !== cur.idx)
    if (next) patches.push({ idx: next.idx, status: 'active' })
  } else {
    patches.push({ idx: cur.idx, status: 'active' })
  }
  const wasActive = cur.status === 'active'
  try {
    await store.patchReplicaSteps(r.id, patches)
    for (const p of patches) {
      const s = r.steps.find(x => x.idx === p.idx)
      if (s) s.status = p.status
    }
    showToast(wasActive ? t('fuke.nodeCompleted', { title: tr(cur.text) }) : t('fuke.nodeStarted', { title: tr(cur.text) }))
  } catch (e: any) { showToast(e?.message || t('node.statusSaveFailed')) }
}
// 卡片折叠状态只存本机浏览器（与原站 localStorage 设置同类），折叠后只留标题和进度。
const COLLAPSED_KEY = 'x-digest-fuke-collapsed'
const collapsed = ref<string[]>([])
function isCollapsed (id: string) { return collapsed.value.includes(id) }
function toggleCollapsed (id: string) {
  collapsed.value = isCollapsed(id) ? collapsed.value.filter(x => x !== id) : [...collapsed.value, id]
  try { localStorage.setItem(COLLAPSED_KEY, JSON.stringify(collapsed.value)) } catch { /* private mode */ }
}
function loadCollapsed () {
  try {
    const v = JSON.parse(localStorage.getItem(COLLAPSED_KEY) || '[]')
    if (Array.isArray(v)) collapsed.value = v.filter(x => typeof x === 'string')
  } catch { /* ignore */ }
}
function toggleFlowMenu (id: string) { openFlowMenu.value = openFlowMenu.value === id ? null : id }
function menuDeleteFlow (r: Replica) { openFlowMenu.value = null; askDelete(r) }
const board = computed(() => replicaBoard(replicas.value))
// 导出：步骤转为 - [x]/- [ ] 清单（含备注、附件链接），浏览器端生成下载。
function menuExport (r: Replica) {
  openFlowMenu.value = null
  const blob = new Blob([replicaMarkdown(r, location.origin)], { type: 'text/markdown;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `${(r.post_title || 'replica').replace(/[\\/:*?"<>|]+/g, '_').slice(0, 60)}.md`
  a.click()
  setTimeout(() => URL.revokeObjectURL(a.href), 1000)
}
// 复制：同样的步骤结构建一个新流程，全部回到未开始，不带附件；可选复制备注。
async function menuClone (r: Replica) {
  openFlowMenu.value = null
  const copyNotes = window.confirm('复制为新流程：是否同时复制各步骤的备注？（确定 = 复制备注，取消 = 不复制）')
  try { await store.cloneReplica(r.id, copyNotes); await reload() } catch (e: any) { window.alert(e?.message || '复制失败') }
}
function onDocClick (e: MouseEvent) {
  if (!openFlowMenu.value) return
  if (!(e.target as HTMLElement).closest('.flow-menu-wrap')) openFlowMenu.value = null
}
function onKeyDown (e: KeyboardEvent) { if (e.key === 'Escape') openFlowMenu.value = null }
async function reload () {
  loading.value = true
  try { replicas.value = await store.loadReplicas(sort.value) } catch (e: any) { loadError.value = e?.message || t('fuke.loadFailed') }
  finally { loading.value = false }
}
const newStepText = ref<Record<string, string>>({})
const newFlowTitle = ref('')
async function createFlow () {
  const title = newFlowTitle.value.trim()
  if (!title) return
  try {
    const rep = await store.createStandaloneReplica(title)
    replicas.value = [rep, ...replicas.value]
    newFlowTitle.value = ''
    showToast(t('fuke.flowCreated'))
  } catch (e: any) { showToast(e?.message || t('fuke.createFlowFailed')) }
}
async function addStep (r: Replica) {
  const text = (newStepText.value[r.id] || '').trim()
  if (!text) return
  try {
    const step = await store.addReplicaStep(r.id, text)
    r.steps = [...r.steps, step]
    newStepText.value[r.id] = ''
    showToast(t('fuke.stepAdded'))
  } catch (e: any) { showToast(e?.message || t('fuke.addStepFailed')) }
}
function askDelete (r: Replica) { confirmTarget.value = r }
// Enqueue replica titles for translation (toward the current UI language).
watch([replicas, lang], () => {
  enqueue(replicas.value.map(r => r.post_title).filter(Boolean))
})
async function doDelete () {
  if (!confirmTarget.value) return
  const id = confirmTarget.value.id
  try {
    await store.deleteReplica(id)
    replicas.value = replicas.value.filter(r => r.id !== id)
    showToast(t('fuke.replicaDeleted'))
  } catch (err: any) { showToast(err?.message || t('common.deleteFailed')) }
  confirmTarget.value = null
}
onMounted(async () => {
  useSettings().load()
  loadCollapsed()
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeyDown)
  await reload()
})
onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeyDown)
})
</script>

<style scoped>
.fuke-board { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 6px; margin: 8px 0 12px; }
.fuke-board > div { border: 1px solid var(--border); border-radius: 10px; padding: 8px; text-align: center; }
.fuke-board strong { display: block; font-size: 18px; }
.fuke-board span { font-size: 12px; color: var(--text-muted); }
</style>
