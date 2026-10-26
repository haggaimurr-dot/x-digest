<template>
  <div>
    <main style="max-width:600px;margin:0 auto;padding:16px;min-width:0">
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">{{ t('common.loading') }}</p>
      <p v-if="!loading" style="color:var(--text-muted)">{{ t('fuke.total', { n: replicas.length }) }}</p>
      <p v-if="translating" style="color:var(--text-muted)">{{ t('common.translating') }}</p>
      <div class="filter-row" style="margin-bottom:12px">
        <label>{{ t('fuke.sort') }}
          <select class="field" v-model="sort" @change="reload">
            <option value="time">{{ t('fuke.sortTime') }}</option>
            <option value="progress">{{ t('fuke.sortProgress') }}</option>
          </select>
        </label>
        <NuxtLink to="/" class="btn btn-sm">{{ t('fuke.backToDaily') }}</NuxtLink>
      </div>
      <div style="margin-bottom:12px;border:1px solid var(--border);border-radius:12px;padding:12px">
        <div class="chip-group-title" style="margin-top:0">{{ t('fuke.newFlowTitle') }}</div>
        <div class="filter-row">
          <input class="field grow" style="flex:1" type="text" v-model="newFlowTitle" :placeholder="t('fuke.newFlowPh')" :aria-label="t('fuke.newFlowAria')" />
          <button class="btn btn-sm btn-primary" type="button" :disabled="!newFlowTitle.trim()" @click="createFlow">{{ t('fuke.createFlow') }}</button>
        </div>
      </div>
      <p v-if="!loading && !replicas.length" class="empty">{{ t('fuke.empty') }}</p>

      <article v-for="r in replicas" :key="r.id" class="post-card flow-sheet" data-theme="light" style="margin-bottom:16px;display:block">
        <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:8px">
          <div class="flow-eyebrow">FLOW · {{ topSteps(r).length }} NODES</div>
          <div class="node-menu-wrap flow-menu-wrap">
            <button class="btn btn-sm btn-quiet node-menu-btn" type="button" @click="toggleFlowMenu(r.id)" :aria-label="t('fuke.flowMenu')" aria-haspopup="menu" :aria-expanded="openFlowMenu === r.id">⋯</button>
            <div v-if="openFlowMenu === r.id" class="node-menu" role="menu">
              <button type="button" role="menuitem" class="danger" @click="menuDeleteFlow(r)">{{ t('fuke.deleteReplica') }}</button>
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
        <div style="margin:12px 0 4px">
          <div class="chip-group-title" style="margin-top:0">{{ t('fuke.customNodes') }}</div>
          <input class="field" style="width:100%" type="text" v-model="newStepText[r.id]" :placeholder="t('fuke.newStepPh')" :aria-label="t('fuke.newStepAria')" />
          <div style="margin-top:6px">
            <button class="btn btn-sm btn-primary" type="button" :disabled="!((newStepText[r.id] || '').trim())" @click="addStep(r)">{{ t('fuke.addStep') }}</button>
          </div>
        </div>
        <div class="flow-list" style="margin-top:12px">
          <p v-if="!topSteps(r).length" style="color:var(--text-muted)">{{ t('fuke.noSteps') }}</p>
          <ReplicaStepNode v-for="(step, si) in topSteps(r)" :key="step.idx" :replica="r" :step="step"
            :num="String(si + 1)" :is-last="si === topSteps(r).length - 1" />
        </div>
        <div v-if="topSteps(r).length" class="flow-action">
          <div class="flow-current">{{ currentNode(r) ? t('fuke.currentNode', { num: currentNum(r), title: tr(currentNode(r)!.text) }) : t('fuke.allDone') }}</div>
          <button v-if="currentNode(r)" class="flow-complete-btn" type="button" @click="completeCurrent(r)">{{ currentActionLabel(r) }}</button>
          <button v-else class="flow-complete-btn" type="button" disabled>{{ t('fuke.allDone') }}</button>
        </div>
      </article>
    </main>

    <div v-if="confirmTarget" class="modal-backdrop" @click.self="confirmTarget = null">
      <div class="modal-card" role="alertdialog" aria-modal="true" :aria-label="t('fuke.confirmDeleteAria')">
        <h2>{{ t('fuke.deleteTitle') }}</h2>
        <p>{{ t('fuke.deleteBody', { title: tr(confirmTarget.post_title) }) }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="confirmTarget = null">{{ t('common.cancel') }}</button>
          <button class="btn btn-danger" type="button" @click="doDelete">{{ t('common.delete') }}</button>
        </div>
      </div>
    </div>
    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { Replica, ReplicaStep } from '~/composables/types'
import { shanghaiTime } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'

const store = useStore()
const { t, lang } = useLang()
const { tr, enqueue, translating } = useTranslate()
const loading = ref(true)
const loadError = ref('')
const replicas = ref<Replica[]>([])
const sort = ref<'time' | 'progress'>('time')
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
function toggleFlowMenu (id: string) { openFlowMenu.value = openFlowMenu.value === id ? null : id }
function menuDeleteFlow (r: Replica) { openFlowMenu.value = null; askDelete(r) }
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
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeyDown)
  await reload()
})
onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeyDown)
})
</script>
