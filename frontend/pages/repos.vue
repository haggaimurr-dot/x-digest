<template>
  <div>
    <main class="feed-col" style="max-width:760px;margin:0 auto;padding:16px;min-width:0">
      <h1 style="margin:4px 0 12px;font-size:20px">{{ t('repos.title') }}</h1>
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">{{ t('common.loading') }}</p>

      <template v-if="!loading && !loadError">
        <!-- 工具条 -->
        <div class="filter-row">
          <span style="color:var(--text-muted)">{{ t('repos.total', { n: filtered.length }) }}</span>
          <span style="flex:1" />
          <button class="btn btn-sm" type="button" @click="syncAll" :disabled="syncAllBusy">{{ syncAllBusy ? t('repos.syncing') : t('repos.syncAll') }}</button>
          <button class="btn btn-sm btn-primary" type="button" @click="openAdd">{{ t('repos.addRepo') }}</button>
        </div>
        <div class="filter-row" style="margin-top:8px">
          <input class="field grow" style="flex:1" type="search" v-model="keyword" :placeholder="t('repos.keywordPh')" :aria-label="t('common.keywordSearch')" />
          <label>{{ t('repos.group') }}
            <select class="field" v-model="groupFilter">
              <option value="">{{ t('repos.allGroups') }}</option>
              <option value="__ungrouped">{{ t('repos.ungrouped') }}</option>
              <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
          </label>
          <label>{{ t('repos.sort') }}
            <select class="field" v-model="sort">
              <option value="stars">{{ t('repos.sortStars') }}</option>
              <option value="updated">{{ t('repos.sortUpdated') }}</option>
            </select>
          </label>
        </div>
        <div v-if="allTags.length" style="margin-top:8px">
          <div class="chip-group-title">{{ t('repos.tagFilter') }}</div>
          <div class="chips">
            <button v-for="t in allTags" :key="t" type="button" class="chip"
              :class="{ active: tagFilter === t }" @click="tagFilter = tagFilter === t ? '' : t">{{ t }}</button>
          </div>
        </div>
        <p v-if="syncAllMsg" style="color:var(--text-muted)">{{ syncAllMsg }}</p>
        <p v-if="translating" style="color:var(--text-muted)">{{ t('common.translating') }}</p>

        <!-- 分组管理 -->
        <section class="settings-section">
          <h3 style="margin:8px 0">{{ t('repos.groupManage') }}</h3>
          <div style="display:flex;gap:6px;flex-wrap:wrap">
            <input class="field" v-model="newGroupName" :placeholder="t('repos.newGroupPh')" style="flex:1;min-width:140px" />
            <button class="btn btn-sm btn-primary" type="button" @click="doAddGroup">{{ t('repos.addGroup') }}</button>
          </div>
          <div v-for="g in groups" :key="g.id" class="watch-row">
            <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">{{ t('repos.reposInGroup', { n: repoCountIn(g.id) }) }}</span>
            <button v-if="!g.fixed" class="btn btn-sm btn-danger" type="button" @click="doDeleteGroup(g)">{{ t('repos.deleteGroup') }}</button>
          </div>
        </section>

        <!-- 空状态 -->
        <p v-if="!repos.length" class="empty" style="margin-top:16px">{{ t('repos.empty') }}</p>
        <p v-else-if="!filtered.length" class="empty" style="margin-top:16px">{{ t('repos.noMatch') }}</p>

        <!-- 仓库卡片 -->
        <section class="repo-list" style="margin-top:16px">
          <article v-for="r in filtered" :key="r.full_name" class="post-card repo-card">
            <div class="repo-head">
              <div style="min-width:0">
                <h2 class="post-title" style="margin:0">
                  <a :href="r.html_url" target="_blank" rel="noopener">{{ r.full_name }}</a>
                </h2>
                <p v-if="repoIntro(r)" class="post-summary" style="margin:6px 0 0">{{ repoIntro(r) }}</p>
                <p v-else style="margin:6px 0 0;color:var(--text-muted)">{{ t('repos.noIntro') }}</p>
              </div>
              <span v-if="isStale(r)" class="stale-badge" :title="t('repos.staleTitle')">{{ t('repos.stale') }}</span>
            </div>

            <div class="repo-stats">
              <span v-if="r.language" class="repo-lang"><span class="lang-dot" />{{ r.language }}</span>
              <span :title="t('repos.starsTitle')">★ {{ fmtNum(r.stars) }}</span>
              <span :title="t('repos.forksTitle')">⑂ {{ fmtNum(r.forks) }}</span>
              <span :title="t('repos.issuesTitle')">Issue {{ r.open_issues ?? '—' }}</span>
              <span :title="t('repos.prsTitle')">PR {{ r.open_prs ?? '—' }}</span>
              <span :title="t('repos.updatedTitle')">{{ r.pushed_at ? t('repos.updatedAt', { time: relativeTime(r.pushed_at, lang.value) }) : t('repos.updatedUnknown') }}</span>
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
                {{ r.last_synced_at ? t('repos.lastSynced', { time: shanghaiTime(r.last_synced_at) }) : t('repos.neverSyncedShort') }}
              </span>
              <span v-if="r.sync_error" style="color:var(--danger-text)" :title="r.sync_error">{{ trunc40(r.sync_error) }}</span>
            </div>

            <div style="margin-top:10px;display:flex;gap:6px;flex-wrap:wrap">
              <button class="btn btn-sm" type="button" @click="toggleDetail(r)" :disabled="detailLoading.has(r.full_name)">
                {{ expanded.has(r.full_name) ? t('repos.collapseDetail') : t('repos.expandDetail') }}
              </button>
              <button class="btn btn-sm" type="button" @click="doSync(r)" :disabled="syncing.has(r.full_name)">
                {{ syncing.has(r.full_name) ? t('repos.refreshing') : t('repos.refresh') }}
              </button>
              <button class="btn btn-sm" type="button" @click="openEdit(r)">{{ t('common.edit') }}</button>
              <button class="btn btn-sm btn-danger" type="button" @click="doDelete(r)">{{ t('common.delete') }}</button>
            </div>

            <!-- 详情区 -->
            <div v-if="expanded.has(r.full_name)" class="repo-detail">
              <p v-if="detailLoading.has(r.full_name)">{{ t('repos.detailLoading') }}</p>
              <p v-else-if="detailError[r.full_name]" style="color:var(--danger-text)">{{ detailError[r.full_name] }}</p>
              <template v-else-if="details[r.full_name]">
                <h3>{{ t('repos.intro') }}</h3>
                <p v-if="repoIntro(details[r.full_name].repo)">{{ repoIntro(details[r.full_name].repo) }}</p>
                <p v-else style="color:var(--text-muted)">{{ t('repos.noIntro') }}</p>

                <h3>{{ t('repos.tutorial') }}</h3>
                <div v-if="(details[r.full_name].repo.tutorial_steps || []).length" class="flow-list">
                  <div v-for="(s, i) in details[r.full_name].repo.tutorial_steps!" :key="i" class="flow-step" :class="{ 'is-last': i === details[r.full_name].repo.tutorial_steps!.length - 1 }">
                    <div class="flow-rail">
                      <span class="flow-node" />
                      <span class="flow-line" v-if="i < details[r.full_name].repo.tutorial_steps!.length - 1" />
                    </div>
                    <div class="flow-card">
                      <p class="flow-text"><strong>{{ i + 1 }}. </strong>{{ tr(s.text) }}</p>
                    </div>
                  </div>
                </div>
                <p v-else style="color:var(--text-muted)">{{ t('repos.noTutorial') }}</p>

                <h3>{{ t('repos.iterationLog') }} <span style="color:var(--text-muted);font-size:13px;font-weight:400">{{ t('repos.versions', { n: detailReleases(r).length }) }}</span></h3>
                <div v-if="detailReleases(r).length">
                  <div v-for="rel in detailReleases(r)" :key="rel.tag_name" class="release-item">
                    <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
                      <button class="btn btn-sm" type="button" @click="toggleRelease(r, rel)">{{ isReleaseOpen(r, rel) ? t('repos.collapseRelease') : t('repos.expandRelease') }}</button>
                      <a :href="rel.html_url" target="_blank" rel="noopener"><strong>{{ rel.tag_name }}</strong></a>
                      <span v-if="rel.name && rel.name !== rel.tag_name" style="color:var(--text-muted)">{{ rel.name }}</span>
                      <span v-if="rel.draft" class="chip active">{{ t('repos.draft') }}</span>
                      <span v-if="rel.prerelease" class="chip active">{{ t('repos.prerelease') }}</span>
                      <span style="color:var(--text-muted)">{{ rel.published_at ? shanghaiTime(rel.published_at) : t('repos.notPublished') }}</span>
                    </div>
                    <pre v-if="isReleaseOpen(r, rel)" class="release-body">{{ tr((rel.body || '').trim()) || t('repos.noReleaseBody') }}</pre>
                  </div>
                </div>
                <p v-else style="color:var(--text-muted)">{{ t('repos.noReleases') }}</p>

                <h3>{{ t('repos.recentCommits') }}</h3>
                <ul v-if="details[r.full_name].commits.length" class="commit-list">
                  <li v-for="c in details[r.full_name].commits" :key="c.sha">
                    <a :href="c.html_url" target="_blank" rel="noopener">{{ trunc40(c.message) }}</a>
                    <span style="color:var(--text-muted)"> · {{ c.author_name }} · {{ relativeTime(c.date, lang.value) }}</span>
                  </li>
                </ul>
                <p v-else style="color:var(--text-muted)">{{ t('repos.noCommits') }}</p>
              </template>
            </div>
          </article>
        </section>
      </template>
    </main>

    <!-- 添加仓库弹窗 -->
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
      <div class="modal-card" role="dialog" aria-modal="true" :aria-label="t('repos.addTitle')">
        <h2>{{ t('repos.addTitle') }}</h2>
        <label>{{ t('repos.fullNameLabel') }}</label>
        <input class="field" v-model="addForm.full_name" :placeholder="t('repos.fullNamePh')" />
        <label>{{ t('repos.groupLabel') }}</label>
        <select class="field" v-model="addForm.group_id">
          <option value="">{{ t('repos.ungrouped') }}</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <label>{{ t('repos.tagsLabel') }}</label>
        <input class="field" v-model="addForm.tagsText" :placeholder="t('repos.tagsPh')" />
        <label>{{ t('repos.noteLabel') }}</label>
        <textarea v-model="addForm.note" rows="2" :placeholder="t('repos.notePh')" />
        <p v-if="addError" style="color:var(--danger-text)">{{ addError }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="showAdd = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="button" @click="doAdd" :disabled="addBusy">{{ addBusy ? t('repos.adding') : t('repos.add') }}</button>
        </div>
      </div>
    </div>

    <!-- 编辑仓库弹窗 -->
    <div v-if="editTarget" class="modal-backdrop" @click.self="editTarget = null">
      <div class="modal-card" role="dialog" aria-modal="true" :aria-label="t('repos.editTitle', { name: editTarget.full_name })">
        <h2>{{ t('repos.editTitle', { name: editTarget.full_name }) }}</h2>
        <label>{{ t('repos.groupLabel') }}</label>
        <select class="field" v-model="editForm.group_id">
          <option value="">{{ t('repos.ungrouped') }}</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <label>{{ t('repos.tagsLabel') }}</label>
        <input class="field" v-model="editForm.tagsText" :placeholder="t('repos.tagsPh')" />
        <label>{{ t('repos.noteLabel') }}</label>
        <textarea v-model="editForm.note" rows="2" :placeholder="t('repos.notePh')" />
        <p v-if="editError" style="color:var(--danger-text)">{{ editError }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="editTarget = null">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="button" @click="doEdit" :disabled="editBusy">{{ editBusy ? t('repos.saving') : t('common.save') }}</button>
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
const { t, lang, isEn } = useLang()
const { tr, enqueue, translating } = useTranslate()

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
    case 'synced': return t('repos.synced')
    case 'never': return t('repos.neverSynced')
    case 'rate_limited': return t('repos.rateLimited')
    case 'failed': return t('repos.syncFailed')
    case 'partial': return t('repos.partial')
    default: return status
  }
}
function repoIntroSource (r: Repo): string {
  // GitHub description 优先，空时用 README 提取的简介兜底。
  return (r.description || '').trim() || (r.readme_intro || '').trim()
}
// Display intro: translated when EN is active (filtering uses the original).
function repoIntro (r: Repo): string { return tr(repoIntroSource(r)) }
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
  // A release body is only translated once its release is expanded.
  if (s.has(k) && isEn.value && rel.body) enqueue([rel.body])
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
    const timeOf = (r: Repo) => Date.parse(r.pushed_at || r.updated_at || '') || 0
    list = [...list].sort((a, b) => timeOf(b) - timeOf(a))
  }
  return list
})

// Enqueue the listed repos' intros when EN is active (deduped by the queue).
watch([filtered, isEn], () => {
  if (!isEn.value) return
  enqueue(filtered.value.map(r => repoIntroSource(r)).filter(Boolean))
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
    // Enqueue the detail's translatable texts when EN is active.
    if (isEn.value) enqueue([repoIntroSource(d.repo), ...(d.repo.tutorial_steps || []).map(s => s.text)])
  } catch (err: any) {
    detailError.value = { ...detailError.value, [r.full_name]: errText(err, t('repos.detailFailed')) }
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
    showToast(t('repos.added', { name: repo.full_name }))
  } catch (err: any) {
    addError.value = errText(err, t('repos.addFailed'))
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
    showToast(t('repos.saved'))
    editTarget.value = null
  } catch (err: any) {
    editError.value = errText(err, t('repos.saveFailed'))
  } finally { editBusy.value = false }
}
async function doDelete (r: Repo) {
  if (!confirm(t('repos.deleteConfirm', { name: r.full_name }))) return
  try {
    await store.deleteRepo(r.full_name)
    repos.value = repos.value.filter(x => x.full_name !== r.full_name)
    const s = new Set(expanded.value); s.delete(r.full_name); expanded.value = s
    showToast(t('repos.deleted', { name: r.full_name }))
  } catch (err: any) { showToast(errText(err, t('repos.deleteFailed'))) }
}
async function doSync (r: Repo) {
  syncing.value = new Set([...syncing.value, r.full_name])
  try {
    const updated = await store.syncRepo(r.full_name)
    if (updated) {
      repos.value = repos.value.map(x => x.full_name === r.full_name ? updated : x)
      showToast(t('repos.refreshed', { name: r.full_name }))
    } else {
      await reload()
      showToast(t('repos.refreshed', { name: r.full_name }))
    }
  } catch (err: any) {
    // Rate limiting must not break the page: show the error, keep old data.
    showToast(errText(err, t('common.refreshFailedKeepOld')))
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
    syncAllMsg.value = t('repos.syncAllDone', { ok: res.synced, fail: res.failed, limited: res.rate_limited, total: res.total })
    await reload()
  } catch (err: any) {
    syncAllMsg.value = errText(err, t('repos.syncAllFailed'))
  } finally { syncAllBusy.value = false }
}
async function doAddGroup () {
  if (!newGroupName.value.trim()) return
  try {
    const g = await store.addRepoGroup(newGroupName.value.trim())
    groups.value = [...groups.value, g]
    newGroupName.value = ''
  } catch (err: any) { showToast(errText(err, t('repos.addGroupFailed'))) }
}
async function doDeleteGroup (g: RepoGroup) {
  if (!confirm(t('repos.deleteGroupConfirm', { name: g.name }))) return
  try {
    await store.deleteRepoGroup(g.id)
    await reload()
    showToast(t('repos.groupDeleted', { name: g.name }))
  } catch (err: any) { showToast(errText(err, t('repos.deleteGroupFailed'))) }
}

onMounted(async () => {
  try {
    await reload()
  } catch (err: any) { loadError.value = errText(err, t('common.loadError')) }
  finally { loading.value = false }
})
</script>
