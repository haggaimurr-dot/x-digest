<template>
  <div>
    <main style="max-width:600px;margin:0 auto;padding:16px;min-width:0">
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">加载中…</p>
      <p v-if="!loading" style="color:var(--text-muted)">共 {{ replicas.length }} 个复刻</p>
      <div class="filter-row" style="margin-bottom:12px">
        <label>排序
          <select class="field" v-model="sort" @change="reload">
            <option value="time">按时间</option>
            <option value="progress">按进度</option>
          </select>
        </label>
        <NuxtLink to="/" class="btn btn-sm">回摘要页选帖复刻</NuxtLink>
      </div>
      <div style="margin-bottom:12px;border:1px solid var(--border);border-radius:12px;padding:12px">
        <div class="chip-group-title" style="margin-top:0">新建空白流程（不依赖 X 贴，从头开始）</div>
        <div class="filter-row">
          <input class="field grow" style="flex:1" type="text" v-model="newFlowTitle" placeholder="流程标题，例如：剪映出片流程" aria-label="新流程标题" />
          <button class="btn btn-sm btn-primary" type="button" :disabled="!newFlowTitle.trim()" @click="createFlow">新建流程</button>
        </div>
      </div>
      <p v-if="!loading && !replicas.length" class="empty">还没有复刻。去摘要页点帖子卡片上的「复刻」即可生成（同一帖子只生成一次），或在上方新建一个空白流程，从头开始加节点。</p>

      <article v-for="r in replicas" :key="r.id" class="post-card" style="margin-bottom:16px;display:block">
        <div style="display:flex;gap:8px;align-items:flex-start;justify-content:space-between;flex-wrap:wrap">
          <div style="min-width:0">
            <h2 class="post-title" style="margin:0">
              <a v-if="r.post_url" :href="r.post_url" target="_blank" rel="noopener">{{ r.post_title }}</a>
              <span v-else>{{ r.post_title }}</span>
            </h2>
            <div style="color:var(--text-muted);font-size:13px">
              <span v-if="r.post_id">{{ r.post_author_name }} @{{ r.post_author }} · </span>
              <span v-else>自建流程 · </span>{{ shanghaiTime(r.created_at) }}
            </div>
          </div>
          <button class="btn btn-sm btn-danger" type="button" @click="askDelete(r)">删除复刻</button>
        </div>
        <div class="flow-list" style="margin-top:12px">
          <p v-if="!topSteps(r).length" style="color:var(--text-muted)">还没有节点，在下面添加第一个节点吧。</p>
          <ReplicaStepNode v-for="(step, si) in topSteps(r)" :key="step.idx" :replica="r" :step="step"
            :num="String(si + 1)" :is-last="si === topSteps(r).length - 1" />
        </div>
        <div style="margin:12px 0 4px;display:flex;justify-content:space-between;gap:8px">
          <span>进度 {{ progress(r).done }} / {{ progress(r).total }}</span>
          <span>{{ progressPct(r) }}%</span>
        </div>
        <div class="progress-bar"><div :style="{ width: progressPct(r) + '%' }" /></div>
        <div style="margin-top:12px;border-top:1px dashed var(--border);padding-top:10px">
          <div class="chip-group-title" style="margin-top:0">自建流程节点</div>
          <input class="field" style="width:100%" type="text" v-model="newStepText[r.id]" placeholder="节点文字（纯文本），添加后可继续改状态、写备注、上传图片和附件" aria-label="新节点文本" />
          <div style="margin-top:6px">
            <button class="btn btn-sm btn-primary" type="button" :disabled="!((newStepText[r.id] || '').trim())" @click="addStep(r)">添加节点</button>
          </div>
        </div>
      </article>
    </main>

    <div v-if="confirmTarget" class="modal-backdrop" @click.self="confirmTarget = null">
      <div class="modal-card" role="alertdialog" aria-modal="true" aria-label="确认删除">
        <h2>删除复刻？</h2>
        <p>将删除「{{ confirmTarget.post_title }}」及其全部附件（服务端会连带删附件文件）。此操作不可撤销。</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="confirmTarget = null">取消</button>
          <button class="btn btn-danger" type="button" @click="doDelete">删除</button>
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
const loading = ref(true)
const loadError = ref('')
const replicas = ref<Replica[]>([])
const sort = ref<'time' | 'progress'>('time')
const confirmTarget = ref<Replica | null>(null)
useScrollLock(computed(() => confirmTarget.value !== null))
const toast = ref('')

function showToast (m: string) { toast.value = m; setTimeout(() => { toast.value = '' }, 3000) }
function topSteps (r: Replica): ReplicaStep[] { return r.steps.filter(s => s.parent_idx == null) }
function progress (r: Replica) {
  const total = r.steps.length
  const done = r.steps.filter(s => s.status === 'done').length
  return { done, total }
}
function progressPct (r: Replica) {
  const { done, total } = progress(r)
  return total ? Math.round(done / total * 100) : 0
}
async function reload () {
  loading.value = true
  try { replicas.value = await store.loadReplicas(sort.value) } catch (e: any) { loadError.value = e?.message || '加载失败' }
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
    showToast('空白流程已创建，去加节点吧')
  } catch (e: any) { showToast(e?.message || '创建流程失败') }
}
async function addStep (r: Replica) {
  const text = (newStepText.value[r.id] || '').trim()
  if (!text) return
  try {
    const step = await store.addReplicaStep(r.id, text)
    r.steps = [...r.steps, step]
    newStepText.value[r.id] = ''
    showToast('节点已添加')
  } catch (e: any) { showToast(e?.message || '添加节点失败') }
}
function askDelete (r: Replica) { confirmTarget.value = r }
async function doDelete () {
  if (!confirmTarget.value) return
  const id = confirmTarget.value.id
  try {
    await store.deleteReplica(id)
    replicas.value = replicas.value.filter(r => r.id !== id)
    showToast('复刻已删除')
  } catch (err: any) { showToast(err?.message || '删除失败') }
  confirmTarget.value = null
}
onMounted(async () => {
  useSettings().load()
  await reload()
})
</script>
