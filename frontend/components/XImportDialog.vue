<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal-card" role="dialog" aria-modal="true" :aria-label="mode === 'post' ? '导入单帖' : '更新账号帖子'">
      <!-- single post -->
      <template v-if="mode === 'post'">
        <h2>导入单帖</h2>
        <label>帖子链接（可一次粘贴多条，每行一条）</label>
        <textarea ref="urlEl" class="field" v-model="url" rows="4" placeholder="https://x.com/用户名/status/…" :disabled="importing" @keydown.ctrl.enter="doImport()" />
        <p style="color:var(--text-muted);font-size:13px;margin:6px 0 0">
          拉取原文后用中转站 AI 解析（标题、类型、标签、资源说明），帖子作者不在关注列表时会自动加入「默认」分组。约 5–30 秒。
        </p>
        <p v-if="importMsg" class="xi-msg" :class="{ 'xi-err': importErr }">{{ importMsg }}</p>
        <ul v-if="batchRows.length" class="xi-rows">
          <li v-for="(r, i) in batchRows" :key="i" :class="'xi-' + r.state">
            <span class="xi-name" :title="r.url">{{ r.url }}</span>
            <span class="xi-state">{{ r.text }}</span>
          </li>
        </ul>
        <div class="modal-actions">
          <button v-if="existedId" class="btn" type="button" :disabled="importing" @click="doImport(true)">重新解析</button>
          <button class="btn" type="button" @click="$emit('close')">关闭</button>
          <button class="btn btn-primary" type="button" :disabled="importing || !url.trim()" @click="doImport()">{{ importing ? '导入中…' : '导入' }}</button>
        </div>
      </template>

      <!-- account refresh -->
      <template v-else>
        <h2>更新到最新帖子</h2>
        <p style="color:var(--text-muted);font-size:13px;margin:0 0 8px">
          从每个账号已入库的最新帖子之后开始拉取（没有帖子的账号拉最近 30 天），新帖用中转站 AI 解析后入库；15 字以内的短回复跳过。不选账号 = 全部账号。
        </p>
        <template v-if="!job.running">
          <input class="field wl-search" v-model="search" type="search" placeholder="搜索账号" style="width:100%" />
          <div class="chips xi-accounts">
            <button type="button" class="chip" :class="{ active: !selected.size }" @click="selected = new Set()">全部账号（{{ accounts.length }}）</button>
            <button v-for="a in shownAccounts" :key="a.username" type="button" class="chip" :class="{ active: selected.has(a.username) }" @click="toggle(a.username)">
              {{ a.name || a.username }} <span class="chip-count">@{{ a.username }}</span>
            </button>
          </div>
        </template>
        <div v-if="job.accounts.length" class="xi-progress">
          <div class="xi-progress-head">
            <strong>{{ job.running ? '更新中' : '已完成' }}</strong>
            <span>{{ job.done || 0 }}/{{ job.total || 0 }} 个账号 · 新增 {{ job.added || 0 }} 条</span>
          </div>
          <div class="xi-bar"><div :style="{ width: pct + '%' }" /></div>
          <ul class="xi-rows">
            <li v-for="a in job.accounts" :key="a.username" :class="'xi-' + a.state">
              <span class="xi-name">@{{ a.username }}</span>
              <span class="xi-state">{{ stateText(a) }}</span>
              <span v-if="a.error" class="xi-error" :title="a.error">{{ a.error }}</span>
            </li>
          </ul>
        </div>
        <p v-if="refreshMsg" class="xi-msg" :class="{ 'xi-err': refreshErr }">{{ refreshMsg }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="$emit('close')">{{ job.running ? '后台继续' : '关闭' }}</button>
          <button v-if="!job.running && failedCount" class="btn" type="button" :disabled="starting" @click="doRetry">重试失败（{{ failedCount }} 帖）</button>
          <button v-if="!job.running" class="btn btn-primary" type="button" :disabled="starting" @click="doRefresh">
            {{ selected.size ? `更新所选 ${selected.size} 个账号` : '更新全部账号' }}
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { WatchlistAccount as Account, XRefreshAccount, XRefreshJob } from '~/composables/types'
import { parseImportLinks } from '~/utils/importLinks'

// Two dialogs of the X daily page: import one post by link, and refresh
// selected (or all) watchlist accounts up to their newest posts. Emits
// `changed` when posts/watchlist changed so the page reloads them.
const props = defineProps<{ mode: 'post' | 'refresh', accounts: Account[] }>()
const emit = defineEmits<{ (e: 'close'): void, (e: 'changed'): void }>()
const store = useStore()

// ---- single post (one link, or several pasted at once) ----
const url = ref('')
const urlEl = ref<HTMLTextAreaElement | null>(null)
// Per-link result of a batch import: 成功 / 已存在 / 格式错误 / 失败.
const batchRows = ref<Array<{ url: string, state: 'pending' | 'running' | 'done' | 'error', text: string }>>([])
const importing = ref(false)
const importMsg = ref('')
const importErr = ref(false)
const existedId = ref('')
async function doImport (force = false) {
  if (!url.value.trim() || importing.value) return
  const parsed = parseImportLinks(url.value)
  if (parsed.posts.length + parsed.duplicates.length + parsed.invalid.length > 1) return doBatchImport(parsed)
  batchRows.value = []
  importing.value = true; importErr.value = false; importMsg.value = ''; existedId.value = ''
  try {
    const r = await store.importXPost(url.value, force)
    if (r.existed) {
      existedId.value = r.post.id
      importMsg.value = `已在库中：${r.post.title || r.post.id}`
    } else {
      importMsg.value = `已导入：${r.post.title}` + (r.account_created ? `（新账号 @${r.post.author} 已加入关注列表）` : '')
      url.value = ''
      emit('changed')
    }
  } catch (err: any) {
    importErr.value = true
    importMsg.value = err?.message || '导入失败'
  } finally { importing.value = false }
}

// Several links: imported one after another (each is a fetch + AI call),
// each row shows its own outcome; duplicates and malformed lines are
// reported without a request.
async function doBatchImport (parsed: ReturnType<typeof parseImportLinks>) {
  importing.value = true; importErr.value = false; importMsg.value = ''; existedId.value = ''
  const rows = [
    ...parsed.posts.map(p => ({ url: p.url, state: 'pending' as const, text: '等待' })),
    ...parsed.duplicates.map(u => ({ url: u, state: 'error' as const, text: '重复链接，已跳过' })),
    ...parsed.invalid.map(u => ({ url: u, state: 'error' as const, text: '格式错误：不是 X 帖子链接' }))
  ]
  batchRows.value = rows
  let added = 0
  for (let i = 0; i < parsed.posts.length; i++) {
    const row = batchRows.value[i]!
    row.state = 'running'; row.text = '导入中…'
    try {
      const r = await store.importXPost(row.url, false)
      row.state = 'done'
      row.text = r.existed ? '已存在' : `成功：${r.post.title || r.post.id}`
      if (!r.existed) added++
    } catch (err: any) {
      row.state = 'error'; row.text = err?.message || '导入失败'
    }
  }
  importMsg.value = `完成：新增 ${added} 条，共 ${rows.length} 行`
  if (added) emit('changed')
  importing.value = false
}

// ---- account refresh ----
const search = ref('')
const selected = ref(new Set<string>())
const job = ref<XRefreshJob>({ running: false, configured: true, accounts: [] })
const starting = ref(false)
const refreshMsg = ref('')
const refreshErr = ref(false)
const shownAccounts = computed(() => {
  const q = search.value.trim().toLowerCase()
  return props.accounts.filter(a => !q || a.username.toLowerCase().includes(q) || (a.name || '').toLowerCase().includes(q))
})
const pct = computed(() => job.value.total ? Math.round(100 * (job.value.done || 0) / job.value.total) : 0)
function toggle (u: string) {
  const s = new Set(selected.value)
  s.has(u) ? s.delete(u) : s.add(u)
  selected.value = s
}
function stateText (a: XRefreshAccount) {
  if (a.state === 'pending') return '排队中'
  if (a.state === 'running') return a.found ? `处理中 · 找到 ${a.found} 条，新增 ${a.added}` : '查找新帖…'
  if (a.state === 'error') return '失败'
  return `找到 ${a.found} · 新增 ${a.added} · 跳过 ${a.skipped}` + (a.failed ? ` · 失败 ${a.failed}` : '')
}

let timer: ReturnType<typeof setTimeout> | null = null
let lastAdded = 0
async function poll () {
  try {
    job.value = await store.getXRefresh()
    if ((job.value.added || 0) !== lastAdded) { lastAdded = job.value.added || 0; emit('changed') }
  } catch { /* keep last state */ }
  if (job.value.running) timer = setTimeout(poll, 3000)
  else if (job.value.finished_at && lastAdded >= 0) refreshMsg.value = `更新完成，新增 ${job.value.added || 0} 条帖子。`
}
async function doRefresh () {
  starting.value = true; refreshErr.value = false; refreshMsg.value = ''
  try {
    job.value = await store.startXRefresh([...selected.value])
    lastAdded = 0
    if (timer) clearTimeout(timer)
    timer = setTimeout(poll, 2000)
  } catch (err: any) {
    refreshErr.value = true
    refreshMsg.value = err?.message || '启动失败'
  } finally { starting.value = false }
}

const failedCount = computed(() => job.value.accounts.reduce((n, a) => n + (a.failed_ids?.length || 0), 0))
async function doRetry () {
  starting.value = true; refreshErr.value = false; refreshMsg.value = ''
  try {
    job.value = await store.retryXRefresh()
    lastAdded = 0
    if (timer) clearTimeout(timer)
    timer = setTimeout(poll, 2000)
  } catch (err: any) {
    refreshErr.value = true
    refreshMsg.value = err?.message || '重试失败'
  } finally { starting.value = false }
}

onMounted(async () => {
  if (props.mode === 'post') { nextTick(() => urlEl.value?.focus()); return }
  try {
    job.value = await store.getXRefresh()
    lastAdded = job.value.added || 0
    if (job.value.running) timer = setTimeout(poll, 3000)
    else if (!job.value.configured) { refreshErr.value = true; refreshMsg.value = '服务器未配置 GROK_API_KEY，暂时不能更新账号。' }
  } catch { /* first open: no job yet */ }
})
onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>
