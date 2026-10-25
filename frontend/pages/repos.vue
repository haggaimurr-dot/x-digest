<template>
  <div>
    <main class="feed-col" style="max-width:760px;margin:0 auto;padding:16px;min-width:0">
      <h1 style="margin:4px 0 12px;font-size:20px">GitHub 仓库</h1>
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">加载中…</p>

      <template v-if="!loading && !loadError">
        <!-- 工具条 -->
        <div class="filter-row">
          <span style="color:var(--text-muted)">共 {{ filtered.length }} 个仓库</span>
          <span style="flex:1" />
          <button class="btn btn-sm" type="button" @click="syncAll" :disabled="syncAllBusy">{{ syncAllBusy ? '同步中…' : '同步全部' }}</button>
          <button class="btn btn-sm btn-primary" type="button" @click="openAdd">添加仓库</button>
        </div>
        <div class="filter-row" style="margin-top:8px">
          <input class="field grow" style="flex:1" type="search" v-model="keyword" placeholder="关键词搜索（仓库名 / 描述）" aria-label="关键词搜索" />
          <label>分组
            <select class="field" v-model="groupFilter">
              <option value="">全部分组</option>
              <option value="__ungrouped">未分组</option>
              <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
          </label>
          <label>排序
            <select class="field" v-model="sort">
              <option value="stars">按星标</option>
              <option value="updated">按最近更新</option>
            </select>
          </label>
        </div>
        <div v-if="allTags.length" style="margin-top:8px">
          <div class="chip-group-title">标签筛选</div>
          <div class="chips">
            <button v-for="t in allTags" :key="t" type="button" class="chip"
              :class="{ active: tagFilter === t }" @click="tagFilter = tagFilter === t ? '' : t">{{ t }}</button>
          </div>
        </div>
        <p v-if="syncAllMsg" style="color:var(--text-muted)">{{ syncAllMsg }}</p>

        <!-- 分组管理 -->
        <section class="settings-section">
          <h3 style="margin:8px 0">分组管理</h3>
          <div style="display:flex;gap:6px;flex-wrap:wrap">
            <input class="field" v-model="newGroupName" placeholder="新分组名称" style="flex:1;min-width:140px" />
            <button class="btn btn-sm btn-primary" type="button" @click="doAddGroup">新增分组</button>
          </div>
          <div v-for="g in groups" :key="g.id" class="watch-row">
            <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">（{{ repoCountIn(g.id) }} 个仓库）</span>
            <button v-if="!g.fixed" class="btn btn-sm btn-danger" type="button" @click="doDeleteGroup(g)">删除分组</button>
          </div>
        </section>

        <!-- 空状态 -->
        <p v-if="!repos.length" class="empty" style="margin-top:16px">还没有添加仓库，点右上角「添加仓库」把想关注的 GitHub 仓库加进来吧。</p>
        <p v-else-if="!filtered.length" class="empty" style="margin-top:16px">没有符合条件的仓库。试试清除筛选。</p>

        <!-- 仓库卡片 -->
        <section class="repo-list" style="margin-top:16px">
          <article v-for="r in filtered" :key="r.full_name" class="post-card repo-card">
            <div class="repo-head">
              <div style="min-width:0">
                <h2 class="post-title" style="margin:0">
                  <a :href="r.html_url" target="_blank" rel="noopener">{{ r.full_name }}</a>
                </h2>
                <p v-if="repoIntro(r)" class="post-summary" style="margin:6px 0 0">{{ repoIntro(r) }}</p>
                <p v-else style="margin:6px 0 0;color:var(--text-muted)">暂无简介</p>
              </div>
              <span v-if="isStale(r)" class="stale-badge" title="上次同步未成功或从未同步，展示的可能是旧数据">旧数据</span>
            </div>

            <div class="repo-stats">
              <span v-if="r.language" class="repo-lang"><span class="lang-dot" />{{ r.language }}</span>
              <span title="星标">★ {{ fmtNum(r.stars) }}</span>
              <span title="Fork">⑂ {{ fmtNum(r.forks) }}</span>
              <span title="开放 Issue">Issue {{ r.open_issues ?? '—' }}</span>
              <span title="开放 PR">PR {{ r.open_prs ?? '—' }}</span>
              <span title="最后更新">{{ r.pushed_at ? `更新于 ${relativeTime(r.pushed_at)}` : '更新时间未知' }}</span>
            </div>

            <div class="chips" style="margin-top:8px">
              <span v-if="groupName(r.group_id)" class="chip active">{{ groupName(r.group_id) }}</span>
              <button v-for="t in (r.tags || [])" :key="t" type="button" class="chip" @click="tagFilter = t">{{ t }}</button>
            </div>
            <p v-if="r.note" style="margin:8px 0 0"><span class="note-pill">{{ r.note }}</span></p>
            <p v-if="r.homepage" style="margin:8px 0 0"><a :href="r.homepage" target="_blank" rel="noopener">{{ r.homepage }}</a></p>

            <div class="repo-sync">
              <span :class="['sync-badge', `sync-${r.sync_status}`]">{{ syncLabel(r.sync_status) }}</span>
              <span style="color:var(--text-muted)">
                {{ r.last_synced_at ? `上次同步 ${shanghaiTime(r.last_synced_at)}` : '从未同步' }}
              </span>
              <span v-if="r.sync_error" style="color:var(--danger-text)" :title="r.sync_error">{{ trunc40(r.sync_error) }}</span>
            </div>

            <div style="margin-top:10px;display:flex;gap:6px;flex-wrap:wrap">
              <button class="btn btn-sm" type="button" @click="toggleDetail(r)" :disabled="detailLoading.has(r.full_name)">
                {{ expanded.has(r.full_name) ? '收起详情' : '展开详情' }}
              </button>
              <button class="btn btn-sm" type="button" @click="doSync(r)" :disabled="syncing.has(r.full_name)">
                {{ syncing.has(r.full_name) ? '刷新中…' : '刷新' }}
              </button>
              <button class="btn btn-sm" type="button" @click="openEdit(r)">编辑</button>
              <button class="btn btn-sm btn-danger" type="button" @click="doDelete(r)">删除</button>
            </div>

            <!-- 详情区 -->
            <div v-if="expanded.has(r.full_name)" class="repo-detail">
              <p v-if="detailLoading.has(r.full_name)">详情加载中…</p>
              <p v-else-if="detailError[r.full_name]" style="color:var(--danger-text)">{{ detailError[r.full_name] }}</p>
              <template v-else-if="details[r.full_name]">
                <h3>简介</h3>
                <p v-if="repoIntro(details[r.full_name].repo)">{{ repoIntro(details[r.full_name].repo) }}</p>
                <p v-else style="color:var(--text-muted)">暂无简介</p>

                <h3>使用教程</h3>
                <div v-if="(details[r.full_name].repo.tutorial_steps || []).length" class="flow-list">
                  <div v-for="(s, i) in details[r.full_name].repo.tutorial_steps!" :key="i" class="flow-step" :class="{ 'is-last': i === details[r.full_name].repo.tutorial_steps!.length - 1 }">
                    <div class="flow-rail">
                      <span class="flow-node" />
                      <span class="flow-line" v-if="i < details[r.full_name].repo.tutorial_steps!.length - 1" />
                    </div>
                    <div class="flow-card">
                      <p class="flow-text"><strong>{{ i + 1 }}. </strong>{{ s.text }}</p>
                    </div>
                  </div>
                </div>
                <p v-else style="color:var(--text-muted)">未从 README 中提取到使用步骤</p>

                <h3>迭代日志 <span style="color:var(--text-muted);font-size:13px;font-weight:400">共 {{ detailReleases(r).length }} 个版本</span></h3>
                <div v-if="detailReleases(r).length">
                  <div v-for="rel in detailReleases(r)" :key="rel.tag_name" class="release-item">
                    <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
                      <button class="btn btn-sm" type="button" @click="toggleRelease(r, rel)">{{ isReleaseOpen(r, rel) ? '收起 ▾' : '展开 ▸' }}</button>
                      <a :href="rel.html_url" target="_blank" rel="noopener"><strong>{{ rel.tag_name }}</strong></a>
                      <span v-if="rel.name && rel.name !== rel.tag_name" style="color:var(--text-muted)">{{ rel.name }}</span>
                      <span v-if="rel.draft" class="chip active">草稿</span>
                      <span v-if="rel.prerelease" class="chip active">预发布</span>
                      <span style="color:var(--text-muted)">{{ rel.published_at ? shanghaiTime(rel.published_at) : '未发布' }}</span>
                    </div>
                    <pre v-if="isReleaseOpen(r, rel)" class="release-body">{{ (rel.body || '').trim() || '无发布说明' }}</pre>
                  </div>
                </div>
                <p v-else style="color:var(--text-muted)">暂无 Release</p>

                <h3>最近提交</h3>
                <ul v-if="details[r.full_name].commits.length" class="commit-list">
                  <li v-for="c in details[r.full_name].commits" :key="c.sha">
                    <a :href="c.html_url" target="_blank" rel="noopener">{{ trunc40(c.message) }}</a>
                    <span style="color:var(--text-muted)"> · {{ c.author_name }} · {{ relativeTime(c.date) }}</span>
                  </li>
                </ul>
                <p v-else style="color:var(--text-muted)">暂无提交记录。</p>
              </template>
            </div>
          </article>
        </section>
      </template>
    </main>

    <!-- 添加仓库弹窗 -->
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
      <div class="modal-card" role="dialog" aria-modal="true" aria-label="添加仓库">
        <h2>添加仓库</h2>
        <label>仓库（owner/repo）</label>
        <input class="field" v-model="addForm.full_name" placeholder="例如 nuxt/nuxt" />
        <label>分组</label>
        <select class="field" v-model="addForm.group_id">
          <option value="">未分组</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <label>标签（逗号分隔）</label>
        <input class="field" v-model="addForm.tagsText" placeholder="例如 框架，Vue" />
        <label>备注</label>
        <textarea v-model="addForm.note" rows="2" placeholder="为什么关注这个仓库…" />
        <p v-if="addError" style="color:var(--danger-text)">{{ addError }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="showAdd = false">取消</button>
          <button class="btn btn-primary" type="button" @click="doAdd" :disabled="addBusy">{{ addBusy ? '添加中…' : '添加' }}</button>
        </div>
      </div>
    </div>

    <!-- 编辑仓库弹窗 -->
    <div v-if="editTarget" class="modal-backdrop" @click.self="editTarget = null">
      <div class="modal-card" role="dialog" aria-modal="true" aria-label="编辑仓库">
        <h2>编辑 {{ editTarget.full_name }}</h2>
        <label>分组</label>
        <select class="field" v-model="editForm.group_id">
          <option value="">未分组</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <label>标签（逗号分隔）</label>
        <input class="field" v-model="editForm.tagsText" placeholder="例如 框架，Vue" />
        <label>备注</label>
        <textarea v-model="editForm.note" rows="2" placeholder="为什么关注这个仓库…" />
        <p v-if="editError" style="color:var(--danger-text)">{{ editError }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="editTarget = null">取消</button>
          <button class="btn btn-primary" type="button" @click="doEdit" :disabled="editBusy">{{ editBusy ? '保存中…' : '保存' }}</button>
        </div>
      </div>
    </div>

    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { Repo, RepoGroup, RepoDetail, RepoRelease } from '~/composables/types'
import { ApiError } from '~/composables/types'
import { relativeTime, shanghaiTime, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'

const store = useStore()

const loading = ref(true)
const loadError = ref('')
const repos = ref<Repo[]>([])
const groups = ref<RepoGroup[]>([])
const keyword = ref('')
const groupFilter = ref('')
const tagFilter = ref('')
const sort = ref<'stars' | 'updated'>('stars')

const expanded = ref<Set<string>>(new Set())
const details = ref<Record<string, RepoDetail>>({})
const detailLoading = ref<Set<string>>(new Set())
const detailError = ref<Record<string, string>>({})
const openReleases = ref<Set<string>>(new Set())
const syncing = ref<Set<string>>(new Set())
const syncAllBusy = ref(false)
const syncAllMsg = ref('')

const showAdd = ref(false)
const addBusy = ref(false)
const addError = ref('')
const addForm = ref({ full_name: '', group_id: '', tagsText: '', note: '' })

const editTarget = ref<Repo | null>(null)
useScrollLock(computed(() => showAdd.value || editTarget.value !== null))
const editBusy = ref(false)
const editError = ref('')
const editForm = ref({ group_id: '', tagsText: '', note: '' })

const newGroupName = ref('')
const toast = ref('')

function showToast (msg: string) { toast.value = msg; setTimeout(() => { toast.value = '' }, 3000) }
function fmtNum (n?: number | null): string {
  if (n == null) return '—'
  return n.toLocaleString('en-US')
}
function groupName (id?: string | null): string {
  if (!id) return ''
  return groups.value.find(g => g.id === id)?.name || ''
}
function repoCountIn (groupId: string): number {
  return repos.value.filter(r => r.group_id === groupId).length
}
function isStale (r: Repo): boolean {
  return r.sync_status !== 'synced'
}
function syncLabel (status: Repo['sync_status']): string {
  switch (status) {
    case 'synced': return '已同步'
    case 'never': return '尚未同步'
    case 'rate_limited': return 'GitHub 限流 · 旧数据'
    case 'failed': return '同步失败 · 旧数据'
    case 'partial': return '部分同步 · 可能为旧数据'
    default: return status
  }
}
function repoIntro (r: Repo): string {
  // GitHub description 优先，空时用 README 提取的简介兜底。
  return (r.description || '').trim() || (r.readme_intro || '').trim()
}
function detailReleases (r: Repo): RepoRelease[] {
  const d = details.value[r.full_name]
  if (!d) return []
  // published_at 正序（最旧在上）；未发布的排在最后面。
  return [...(d.releases || [])].sort((a, b) => {
    const ta = a.published_at ? Date.parse(a.published_at) : Infinity
    const tb = b.published_at ? Date.parse(b.published_at) : Infinity
    return (ta - tb) || a.tag_name.localeCompare(b.tag_name)
  })
}
function releaseKey (r: Repo, rel: RepoRelease): string { return `${r.full_name}@${rel.tag_name}` }
function isReleaseOpen (r: Repo, rel: RepoRelease): boolean { return openReleases.value.has(releaseKey(r, rel)) }
function toggleRelease (r: Repo, rel: RepoRelease) {
  const s = new Set(openReleases.value)
  const k = releaseKey(r, rel)
  if (s.has(k)) s.delete(k); else s.add(k)
  openReleases.value = s
}
function parseTags (text: string): string[] {
  return text.split(/[,，]/).map(s => s.trim()).filter(Boolean)
}
function errText (err: any, fallback: string): string {
  if (err instanceof ApiError) {
    // Surface the backend message verbatim; keep the HTTP status for 409/422 etc.
    return err.status ? `(${err.status}) ${err.message}` : err.message
  }
  return err?.message || fallback
}

const allTags = computed(() => {
  const s = new Set<string>()
  for (const r of repos.value) for (const t of r.tags || []) s.add(t)
  return [...s].sort()
})
const filtered = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  let list = repos.value.filter((r) => {
    if (groupFilter.value === '__ungrouped' && r.group_id) return false
    if (groupFilter.value && groupFilter.value !== '__ungrouped' && r.group_id !== groupFilter.value) return false
    if (tagFilter.value && !(r.tags || []).includes(tagFilter.value)) return false
    if (q && !`${r.full_name}\n${r.description || ''}`.toLowerCase().includes(q)) return false
    return true
  })
  if (sort.value === 'stars') {
    list = [...list].sort((a, b) => (b.stars || 0) - (a.stars || 0))
  } else {
    const t = (r: Repo) => Date.parse(r.pushed_at || r.updated_at || '') || 0
    list = [...list].sort((a, b) => t(b) - t(a))
  }
  return list
})

async function reload () {
  const [rs, gs] = await Promise.all([store.loadRepos(), store.loadRepoGroups()])
  repos.value = rs
  groups.value = gs
}
async function loadDetail (r: Repo) {
  if (details.value[r.full_name] || detailLoading.value.has(r.full_name)) return
  detailLoading.value = new Set([...detailLoading.value, r.full_name])
  try {
    const d = await store.loadRepoDetail(r.full_name)
    // 详情没带 releases 时回退到 releases 专用接口；失败不影响详情展示。
    if (d.releases === undefined) {
      try { d.releases = await store.loadRepoReleases(r.full_name) } catch { d.releases = [] }
    }
    details.value = { ...details.value, [r.full_name]: d }
    // Keep the card's repo fields in step with the detail response.
    repos.value = repos.value.map(x => x.full_name === r.full_name ? d.repo : x)
    delete detailError.value[r.full_name]
  } catch (err: any) {
    detailError.value = { ...detailError.value, [r.full_name]: errText(err, '详情加载失败') }
  } finally {
    const s = new Set(detailLoading.value); s.delete(r.full_name); detailLoading.value = s
  }
}
function toggleDetail (r: Repo) {
  if (expanded.value.has(r.full_name)) {
    const s = new Set(expanded.value); s.delete(r.full_name); expanded.value = s
  } else {
    expanded.value = new Set([...expanded.value, r.full_name])
    loadDetail(r)
  }
}
function openAdd () {
  addForm.value = { full_name: '', group_id: '', tagsText: '', note: '' }
  addError.value = ''
  showAdd.value = true
}
async function doAdd () {
  const name = addForm.value.full_name.trim()
  const err = store.validateRepoFullName(name)
  if (err) { addError.value = err; return }
  addBusy.value = true
  addError.value = ''
  try {
    const repo = await store.addRepo(name, addForm.value.group_id || null, parseTags(addForm.value.tagsText), addForm.value.note.trim())
    repos.value = [repo, ...repos.value]
    showAdd.value = false
    showToast(`已添加 ${repo.full_name}`)
  } catch (err: any) {
    addError.value = errText(err, '添加失败')
  } finally { addBusy.value = false }
}
function openEdit (r: Repo) {
  editTarget.value = r
  editForm.value = { group_id: r.group_id || '', tagsText: (r.tags || []).join('，'), note: r.note || '' }
  editError.value = ''
}
async function doEdit () {
  if (!editTarget.value) return
  editBusy.value = true
  editError.value = ''
  try {
    const patch = { group_id: editForm.value.group_id || null, tags: parseTags(editForm.value.tagsText), note: editForm.value.note.trim() }
    const updated = await store.patchRepo(editTarget.value.full_name, patch)
    const merged: Repo = { ...editTarget.value, ...patch, ...(updated || {}) }
    repos.value = repos.value.map(x => x.full_name === editTarget.value!.full_name ? merged : x)
    if (details.value[editTarget.value.full_name]) {
      details.value = { ...details.value, [editTarget.value.full_name]: { ...details.value[editTarget.value.full_name], repo: merged } }
    }
    showToast('已保存')
    editTarget.value = null
  } catch (err: any) {
    editError.value = errText(err, '保存失败')
  } finally { editBusy.value = false }
}
async function doDelete (r: Repo) {
  if (!confirm(`删除仓库 ${r.full_name}？该操作不可恢复。`)) return
  try {
    await store.deleteRepo(r.full_name)
    repos.value = repos.value.filter(x => x.full_name !== r.full_name)
    const s = new Set(expanded.value); s.delete(r.full_name); expanded.value = s
    showToast(`已删除 ${r.full_name}`)
  } catch (err: any) { showToast(errText(err, '删除失败')) }
}
async function doSync (r: Repo) {
  syncing.value = new Set([...syncing.value, r.full_name])
  try {
    const updated = await store.syncRepo(r.full_name)
    if (updated) {
      repos.value = repos.value.map(x => x.full_name === r.full_name ? updated : x)
      showToast(`${r.full_name} 已刷新`)
    } else {
      await reload()
      showToast(`${r.full_name} 已刷新`)
    }
  } catch (err: any) {
    // Rate limiting must not break the page: show the error, keep old data.
    showToast(errText(err, '刷新失败，已保留旧数据'))
    await reload().catch(() => {})
  } finally {
    const s = new Set(syncing.value); s.delete(r.full_name); syncing.value = s
  }
}
async function syncAll () {
  if (syncAllBusy.value) return
  syncAllBusy.value = true
  syncAllMsg.value = ''
  try {
    const res = await store.syncAllRepos()
    syncAllMsg.value = `同步完成：成功 ${res.synced}，失败 ${res.failed}，限流 ${res.rate_limited}（共 ${res.total}）`
    await reload()
  } catch (err: any) {
    syncAllMsg.value = errText(err, '同步全部失败，已保留旧数据')
  } finally { syncAllBusy.value = false }
}
async function doAddGroup () {
  if (!newGroupName.value.trim()) return
  try {
    const g = await store.addRepoGroup(newGroupName.value.trim())
    groups.value = [...groups.value, g]
    newGroupName.value = ''
  } catch (err: any) { showToast(errText(err, '新增分组失败')) }
}
async function doDeleteGroup (g: RepoGroup) {
  if (!confirm(`删除分组「${g.name}」？组内仓库将归入未分组。`)) return
  try {
    await store.deleteRepoGroup(g.id)
    await reload()
    showToast(`已删除分组「${g.name}」`)
  } catch (err: any) { showToast(errText(err, '删除分组失败')) }
}

onMounted(async () => {
  try {
    await reload()
  } catch (err: any) { loadError.value = errText(err, '数据加载失败') }
  finally { loading.value = false }
})
</script>
