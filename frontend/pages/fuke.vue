<template>
  <div>
    <header class="topbar">
      <div class="topbar-inner">
        <div class="topnav">
          <span class="logo"><span class="logo-x">𝕏</span> x-digest</span>
          <NuxtLink to="/">摘要</NuxtLink>
          <NuxtLink to="/fuke">复刻</NuxtLink>
          <span style="margin-left:auto;color:var(--text-muted)">共 {{ replicas.length }} 个复刻</span>
        </div>
      </div>
    </header>
    <main style="max-width:600px;margin:0 auto;padding:16px;min-width:0">
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">加载中…</p>
      <div class="filter-row" style="margin-bottom:12px">
        <label>排序
          <select class="field" v-model="sort" @change="reload">
            <option value="time">按时间</option>
            <option value="progress">按进度</option>
          </select>
        </label>
        <NuxtLink to="/" class="btn btn-sm">回摘要页选帖复刻</NuxtLink>
      </div>
      <p v-if="!loading && !replicas.length" class="empty">还没有复刻。去摘要页点帖子卡片上的「复刻」即可生成（同一帖子只生成一次）。</p>

      <article v-for="r in replicas" :key="r.id" class="post-card" style="margin-bottom:16px">
        <div style="display:flex;gap:8px;align-items:flex-start;justify-content:space-between;flex-wrap:wrap">
          <div style="min-width:0">
            <h2 class="post-title" style="margin:0"><a :href="r.post_url" target="_blank" rel="noopener">{{ r.post_title }}</a></h2>
            <div style="color:var(--text-muted);font-size:13px">{{ r.post_author_name }} @{{ r.post_author }} · {{ shanghaiTime(r.created_at) }}</div>
          </div>
          <button class="btn btn-sm btn-danger" type="button" @click="askDelete(r)">删除复刻</button>
        </div>
        <div style="margin:10px 0 4px;display:flex;justify-content:space-between;gap:8px">
          <span>进度 {{ progress(r).done }} / {{ progress(r).total }}</span>
          <span>{{ progressPct(r) }}%</span>
        </div>
        <div class="progress-bar"><div :style="{ width: progressPct(r) + '%' }" /></div>

        <div v-for="step in r.steps" :key="step.idx" class="replica-step" :class="{ 'is-done': step.status === 'done' }">
          <button type="button" class="status-dot" :class="`is-${step.status}`"
            :title="`${statusNames[step.status]}，点击切换状态`" :aria-label="`步骤 ${step.idx + 1}：${statusNames[step.status]}，点击切换`"
            @click="cycleStatus(r, step)" />
          <div style="flex:1;min-width:0">
            <div class="replica-step-text"><strong>{{ step.idx + 1 }}. {{ step.text }}</strong></div>
            <div v-if="step.tools || step.tech || step.prep" style="color:var(--text-muted);font-size:13px">
              <span v-if="step.tools">工具：{{ step.tools }} </span>
              <span v-if="step.tech">技术：{{ step.tech }} </span>
              <span v-if="step.prep">前提：{{ step.prep }}</span>
            </div>
            <ol v-if="(step.substeps || []).length" style="color:var(--text-muted);font-size:13px">
              <li v-for="(sub, j) in step.substeps!.slice(0, 5)" :key="j">{{ trunc40(sub.text) }}</li>
            </ol>
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

const store = useStore()
const loading = ref(true)
const loadError = ref('')
const replicas = ref<Replica[]>([])
const sort = ref<'time' | 'progress'>('time')
const confirmTarget = ref<Replica | null>(null)
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
async function cycleStatus (r: Replica, step: ReplicaStep) {
  const next = NEXT[step.status] || 'idle'
  step.status = next
  try { await store.patchReplicaSteps(r.id, [{ idx: step.idx, status: next }]) } catch (e: any) { showToast(e?.message || '状态保存失败') }
}
async function saveNote (r: Replica, step: ReplicaStep, note: string) {
  try { await store.patchReplicaSteps(r.id, [{ idx: step.idx, note }]) } catch (e: any) { showToast(e?.message || '备注保存失败') }
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
