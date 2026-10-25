<template>
  <div class="flow-step" :class="{ 'is-last': isLast, 'is-done': step.status === 'done' }">
    <div class="flow-rail">
      <button type="button" class="status-dot" :class="`is-${step.status}`"
        :title="`${statusNames[step.status]}，点击切换状态`" :aria-label="`节点 ${num}：${statusNames[step.status]}，点击切换`"
        @click="cycleStatus" />
      <span class="flow-line" v-if="!isLast" />
    </div>
    <div class="flow-card" style="flex:1;min-width:0">
      <div class="replica-step-text"><strong>{{ num }}. {{ step.text }}</strong></div>
      <div v-if="step.is_custom" class="node-actions">
        <button class="chip active" type="button" :title="isChild ? '点击编辑节点文字' : '点击展开 / 收起子流程'" @click="onBadgeClick">{{ isChild ? '子节点' : '自建节点' }}</button>
        <span style="flex:1" />
        <button class="btn btn-sm btn-quiet" type="button" @click="startEditText">编辑</button>
        <button class="btn btn-sm btn-quiet-danger" type="button" @click="removeStep">删除</button>
      </div>
      <div v-if="editingText !== null" style="margin:6px 0">
        <input class="field" style="width:100%" type="text" v-model="editingText" aria-label="编辑节点文本" />
        <div style="margin-top:6px;display:flex;gap:6px">
          <button class="btn btn-sm btn-primary" type="button" @click="saveStepText">保存文本</button>
          <button class="btn btn-sm" type="button" @click="editingText = null">取消</button>
        </div>
      </div>
      <div v-if="step.tools || step.tech || step.prep" style="color:var(--text-muted);font-size:13px">
        <span v-if="step.tools">工具：{{ step.tools }} </span>
        <span v-if="step.tech">技术：{{ step.tech }} </span>
        <span v-if="step.prep">前提：{{ step.prep }}</span>
      </div>
      <div v-if="!isChild" style="margin-top:8px">
        <button class="btn btn-sm" type="button" @click="childOpen = !childOpen">
          {{ childOpen ? '收起子流程 ▾' : (subCount ? `展开子流程（${subCount}） ▸` : '＋ 添加子节点 ▸') }}
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
                <div class="flow-text" style="font-size:13px">{{ trunc40(sub.text) }}</div>
              </div>
            </div>
          </div>
          <div style="margin-top:8px">
            <input class="field" style="width:100%" type="text" v-model="newChildText" placeholder="子节点文字（纯文本）" aria-label="新子节点文本" />
            <div style="margin-top:6px">
              <button class="btn btn-sm btn-primary" type="button" :disabled="!newChildText.trim()" @click="addChild">添加子节点</button>
            </div>
          </div>
        </div>
      </div>
      <NoteEditor :model-value="step.note || ''" @update:model-value="step.note = $event" @save="saveNote($event)" />
      <div style="margin-top:8px">
        <label class="btn btn-sm" style="cursor:pointer">上传图片
          <input type="file" accept="image/*" multiple hidden @change="upload($event)" />
        </label>
        <label class="btn btn-sm" style="cursor:pointer;margin-left:6px">上传文档 / 压缩包
          <input type="file" multiple hidden :accept="acceptDocs" @change="upload($event)" />
        </label>
        <span style="color:var(--text-muted);font-size:13px;margin-left:6px">白名单见 README；单个 ≤10MB</span>
      </div>
      <div v-if="(step.attachments || []).length">
        <div v-for="att in step.attachments" :key="att.id" class="attachment-row">
          <span class="attachment-name">{{ att.name }}（{{ att.size }} 字节）</span>
          <button class="btn btn-sm" type="button" @click="previewAtt = previewAtt?.id === att.id ? null : att">{{ previewAtt?.id === att.id ? '收起预览' : '预览' }}</button>
          <a class="btn btn-sm" :href="att.download_url" download>下载</a>
          <button class="btn btn-sm btn-danger" type="button" @click="removeAttachment(att)">删除附件</button>
        </div>
        <AttachmentPreview v-if="previewAtt && step.attachments!.some(a => a.id === previewAtt!.id)" :key="previewAtt.id" :attachment="previewAtt" />
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
const toast = ref('')
const statusNames: Record<string, string> = { idle: '未开始', active: '进行中', done: '已完成' }
const NEXT: Record<string, 'idle' | 'active' | 'done'> = { idle: 'active', active: 'done', done: 'idle' }
const acceptDocs = '.' + ATTACHMENT_EXTS.filter(e => !IMAGE_EXTS.includes(e)).join(',.')
const editingText = ref<string | null>(null)
const childOpen = ref(false)
const newChildText = ref('')
const previewAtt = ref<Attachment | null>(null)

const children = computed(() => props.replica.steps.filter(s => s.parent_idx === props.step.idx))
const subCount = computed(() => children.value.length + (props.step.substeps || []).length)

function showToast (m: string) { toast.value = m; setTimeout(() => { toast.value = '' }, 3000) }
async function cycleStatus () {
  const next = NEXT[props.step.status] || 'idle'
  props.step.status = next
  try { await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, status: next }]) } catch (e: any) { showToast(e?.message || '状态保存失败') }
}
async function saveNote (note: string) {
  try { await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, note }]) } catch (e: any) { showToast(e?.message || '备注保存失败') }
}
async function upload (e: Event) {
  const input = e.target as HTMLInputElement
  const files = [...(input.files || [])]
  input.value = ''
  if (!files.length) return
  try {
    const made = await store.uploadAttachments(props.replica.id, props.step.idx, files)
    props.step.attachments = [...(props.step.attachments || []), ...made]
    showToast(`已上传 ${made.length} 个附件`)
  } catch (err: any) { showToast(err?.message || '上传失败') }
}
async function removeAttachment (att: Attachment) {
  if (!confirm(`删除附件 ${att.name}？`)) return
  try {
    await store.deleteAttachment(att.id)
    props.step.attachments = (props.step.attachments || []).filter(a => a.id !== att.id)
    if (previewAtt.value?.id === att.id) previewAtt.value = null
  } catch (err: any) { showToast(err?.message || '删除附件失败') }
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
  if (!text) { showToast('节点文本不能为空'); return }
  try {
    await store.patchReplicaSteps(props.replica.id, [{ idx: props.step.idx, text }])
    props.step.text = text
    editingText.value = null
    showToast('节点文本已保存')
  } catch (e: any) { showToast(e?.message || '保存失败') }
}
async function removeStep () {
  if (!confirm(`删除${props.isChild ? '子' : ''}节点「${props.step.text.slice(0, 20)}」？节点下的附件会一起删除。`)) return
  try {
    await store.deleteReplicaStep(props.replica.id, props.step.idx)
    props.replica.steps = props.replica.steps.filter(s => s.idx !== props.step.idx && s.parent_idx !== props.step.idx)
    showToast('节点已删除')
  } catch (e: any) { showToast(e?.message || '删除失败') }
}
async function addChild () {
  const text = newChildText.value.trim()
  if (!text) return
  try {
    const step = await store.addReplicaStep(props.replica.id, text, props.step.idx)
    props.replica.steps = [...props.replica.steps, step]
    newChildText.value = ''
    showToast('子节点已添加')
  } catch (e: any) { showToast(e?.message || '添加子节点失败') }
}
</script>
