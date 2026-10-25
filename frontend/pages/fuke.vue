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
          <p v-if="!r.steps.length" style="color:var(--text-muted)">还没有节点，在下面添加第一个节点吧。</p>
          <div v-for="(step, si) in r.steps" :key="step.idx" class="flow-step" :class="{ 'is-last': si === r.steps.length - 1, 'is-done': step.status === 'done' }">
            <div class="flow-rail">
              <button type="button" class="status-dot" :class="`is-${step.status}`"
                :title="`${statusNames[step.status]}，点击切换状态`" :aria-label="`步骤 ${si + 1}：${statusNames[step.status]}，点击切换`"
                @click="cycleStatus(r, step)" />
              <span class="flow-line" v-if="si < r.steps.length - 1" />
            </div>
            <div class="flow-card" style="flex:1;min-width:0">
              <div class="replica-step-text"><strong>{{ si + 1 }}. {{ step.text }}</strong></div>
            <div v-if="step.is_custom" style="margin:4px 0;display:flex;gap:6px;align-items:center;flex-wrap:wrap">
              <span class="chip active">自建节点</span>
              <button class="btn btn-sm" type="button" @click="startEditText(r, step)">编辑文本</button>
              <button class="btn btn-sm btn-danger" type="button" @click="removeStep(r, step)">删除节点</button>
            </div>
            <div v-if="editingStep && editingStep.rid === r.id && editingStep.idx === step.idx" style="margin:6px 0">
              <input class="field" style="width:100%" type="text" v-model="editingStep.text" aria-label="编辑节点文本" />
              <div style="margin-top:6px;display:flex;gap:6px">
                <button class="btn btn-sm btn-primary" type="button" @click="saveStepText(r, step)">保存文本</button>
                <button class="btn btn-sm" type="button" @click="editingStep = null">取消</button>
              </div>
            </div>
            <div v-if="step.tools || step.tech || step.prep" style="color:var(--text-muted);font-size:13px">
              <span v-if="step.tools">工具：{{ step.tools }} </span>
              <span v-if="step.tech">技术：{{ step.tech }} </span>
              <span v-if="step.prep">前提：{{ step.prep }}</span>
            </div>
            <div v-if="(step.substeps || []).length" style="margin-top:8px">
              <button class="btn btn-sm" type="button" @click="toggleSubsteps(r, step)">
                {{ substepsOpen.has(subKey(r, step)) ? '收起子流程 ▾' : `展开子流程（${step.substeps!.length}） ▸` }}
              </button>
              <div v-if="substepsOpen.has(subKey(r, step))" class="flow-list" style="margin-top:8px">
                <div v-for="(sub, j) in step.substeps!.slice(0, 5)" :key="j" class="flow-step" :class="{ 'is-last': j === Math.min(step.substeps!.length, 5) - 1 }">
                  <div class="flow-rail">
                    <span class="flow-node" />
                    <span class="flow-line" v-if="j < Math.min(step.substeps!.length, 5) - 1" />
                  </div>
                  <div class="flow-card" style="padding:8px 10px">
                    <div class="flow-text" style="font-size:13px">{{ trunc40(sub.text) }}</div>
                  </div>
                </div>
              </div>
            </div>
            <NoteEditor :model-value="step.note || ''" @update:model-value="step.note = $event" @save="saveNote(r, step, $event)" />
            <div style="margin-top:8px">
              <label class="btn btn-sm" style="cursor:pointer">上传图片
                <input type="file" accept="image/*" multiple hidden @change="upload(r, step, $event)" />
              </label>
              <label class="btn btn-sm" style="cursor:pointer;margin-left:6px">上传文档 / 压缩包
                <input type="file" multiple hidden :accept="acceptDocs" @change="upload(r, step, $event)" />
              </label>
              <span style="color:var(--text-muted);font-size:13px;margin-left:6px">白名单见 README；单个 ≤10MB</span>
            </div>
            <div v-if="(step.attachments || []).length">
              <div v-for="att in step.attachments" :key="att.id" class="attachment-row">
                <span class="attachment-name">{{ att.name }}（{{ att.size }} 字节）</span>
                <button class="btn btn-sm" type="button" @click="previewAtt = previewAtt?.id === att.id ? null : att">{{ previewAtt?.id === att.id ? '收起预览' : '预览' }}</button>
                <a class="btn btn-sm" :href="att.download_url" download>下载</a>
                <button class="btn btn-sm btn-danger" type="button" @click="removeAttachment(r, step, att)">删除附件</button>
              </div>
              <AttachmentPreview v-if="previewAtt && step.attachments!.some(a => a.id === previewAtt!.id)" :key="previewAtt.id" :attachment="previewAtt" />
            </div>
            </div>
          </div>
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
import type { Replica, ReplicaStep, Attachment } from '~/composables/types'
import { shanghaiTime, trunc40 } from '~/composables/useFormat'
import { ATTACHMENT_EXTS, IMAGE_EXTS } from '~/composables/useStore'
import { useScrollLock } from '~/composables/useScrollLock'

const store = useStore()
const loading = ref(true)
const loadError = ref('')
const replicas = ref<Replica[]>([])
const sort = ref<'time' | 'progress'>('time')
const confirmTarget = ref<Replica | null>(null)
useScrollLock(computed(() => confirmTarget.value !== null))
const previewAtt = ref<Attachment | null>(null)
const toast = ref('')
const statusNames: Record<string, string> = { idle: '未开始', active: '进行中', done: '已完成' }
const NEXT: Record<string, 'idle' | 'active' | 'done'> = { idle: 'active', active: 'done', done: 'idle' }
const acceptDocs = '.' + ATTACHMENT_EXTS.filter(e => !IMAGE_EXTS.includes(e)).join(',.')

function showToast (m: string) { toast.value = m; setTimeout(() => { toast.value = '' }, 3000) }
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
const substepsOpen = ref<Set<string>>(new Set())
function subKey (r: Replica, step: ReplicaStep) { return `${r.id}:${step.idx}` }
function toggleSubsteps (r: Replica, step: ReplicaStep) {
  const k = subKey(r, step)
  if (substepsOpen.value.has(k)) substepsOpen.value.delete(k)
  else substepsOpen.value.add(k)
  substepsOpen.value = new Set(substepsOpen.value)
}
async function cycleStatus (r: Replica, step: ReplicaStep) {
  const next = NEXT[step.status] || 'idle'
  step.status = next
  try { await store.patchReplicaSteps(r.id, [{ idx: step.idx, status: next }]) } catch (e: any) { showToast(e?.message || '状态保存失败') }
}
async function saveNote (r: Replica, step: ReplicaStep, note: string) {
  try { await store.patchReplicaSteps(r.id, [{ idx: step.idx, note }]) } catch (e: any) { showToast(e?.message || '备注保存失败') }
}
const newStepText = ref<Record<string, string>>({})
const editingStep = ref<{ rid: string, idx: number, text: string } | null>(null)
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
function startEditText (r: Replica, step: ReplicaStep) {
  editingStep.value = { rid: r.id, idx: step.idx, text: step.text }
}
async function saveStepText (r: Replica, step: ReplicaStep) {
  if (!editingStep.value) return
  const text = editingStep.value.text.trim()
  if (!text) { showToast('节点文本不能为空'); return }
  try {
    await store.patchReplicaSteps(r.id, [{ idx: step.idx, text }])
    step.text = text
    editingStep.value = null
    showToast('节点文本已保存')
  } catch (e: any) { showToast(e?.message || '保存失败') }
}
async function removeStep (r: Replica, step: ReplicaStep) {
  if (!confirm(`删除自建节点「${step.text.slice(0, 20)}」？节点下的附件会一起删除。`)) return
  try {
    await store.deleteReplicaStep(r.id, step.idx)
    r.steps = r.steps.filter(s => s.idx !== step.idx)
    showToast('节点已删除')
  } catch (e: any) { showToast(e?.message || '删除失败') }
}
async function upload (r: Replica, step: ReplicaStep, e: Event) {
  const input = e.target as HTMLInputElement
  const files = [...(input.files || [])]
  input.value = ''
  if (!files.length) return
  try {
    const made = await store.uploadAttachments(r.id, step.idx, files)
    step.attachments = [...(step.attachments || []), ...made]
    showToast(`已上传 ${made.length} 个附件`)
  } catch (err: any) { showToast(err?.message || '上传失败') }
}
async function removeAttachment (r: Replica, step: ReplicaStep, att: Attachment) {
  if (!confirm(`删除附件 ${att.name}？`)) return
  try {
    await store.deleteAttachment(att.id)
    step.attachments = (step.attachments || []).filter(a => a.id !== att.id)
    if (previewAtt.value?.id === att.id) previewAtt.value = null
  } catch (err: any) { showToast(err?.message || '删除附件失败') }
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
