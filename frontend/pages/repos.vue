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
        </div>
        <div class="filter-row" style="justify-content:space-between;margin-top:8px">
          <IconBtn class="btn-sm" icon="filter" :class="{ 'btn-on': !filtersCollapsed }" :label="filtersCollapsed ? t('repos.expandFilters') : t('repos.collapseFilters')" :aria-expanded="!filtersCollapsed" @click="filtersCollapsed = !filtersCollapsed" />
          <span class="active-count" v-if="filtersCollapsed && activeFilterCount">{{ t('index.selectedFilters', { n: activeFilterCount }) }}</span>
        </div>
        <div v-show="!filtersCollapsed">
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
              <option v-for="k in REPO_SORTS" :key="k" :value="k">{{ t('repos.sort_' + k) }}</option>
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
        </div>
        <p v-if="syncAllMsg" style="color:var(--text-muted)">{{ syncAllMsg }}</p>
        <p v-if="translating" style="color:var(--text-muted)">{{ t('common.translating') }}</p>

        <!-- 分组管理 -->
        <section class="settings-section">
          <h3 style="margin:8px 0">{{ t('repos.groupManage') }}</h3>
          <div style="display:flex;gap:6px;flex-wrap:wrap">
            <input class="field" v-model="newGroupName" :placeholder="t('repos.newGroupPh')" style="flex:1;min-width:140px" />
            <IconBtn class="btn-sm btn-primary" icon="folder-plus" :label="t('repos.addGroup')" @click="doAddGroup" />
          </div>
          <div v-for="g in groups" :key="g.id" class="watch-row">
            <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">{{ t('repos.reposInGroup', { n: repoCountIn(g.id) }) }}</span>
            <IconBtn v-if="!g.fixed" class="btn-sm btn-danger" icon="trash" :label="t('repos.deleteGroup')" @click="doDeleteGroup(g)" />
          </div>
        </section>

        <!-- 空状态 -->
        <p v-if="!repos.length" class="empty" style="margin-top:16px">{{ t('repos.empty') }}</p>
        <p v-else-if="!filtered.length" class="empty" style="margin-top:16px">{{ t('repos.noMatch') }}</p>

        <!-- 仓库卡片 -->
        <section class="repo-list" style="margin-top:16px">
          <article v-for="r in filtered" :key="r.full_name" class="post-card repo-card">
            <div class="repo-head">
              <a class="repo-avatar" :href="`https://github.com/${r.owner}`" target="_blank" rel="noopener" :title="r.owner">
                <img v-if="!failedAvatars.has(r.owner)" :src="ownerAvatarUrl(r.owner)" :alt="r.owner" loading="lazy" @error="onAvatarError(r.owner)" />
                <span v-else aria-hidden="true">{{ (r.owner || '?').slice(0, 1).toUpperCase() }}</span>
              </a>
              <div style="min-width:0;flex:1">
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
              <span v-if="r.released_at" :title="t('repos.releasedTitle')">{{ t('repos.releasedAt', { time: r.released_at.slice(0, 10) }) }}</span>
              <span v-if="r.created_at" :title="t('repos.createdTitle')">{{ t('repos.createdAt', { time: r.created_at.slice(0, 10) }) }}</span>
            </div>

            <!-- AI 智能摘要：README + 简介 + 最新 Release，点击时才生成 -->
            <button class="yt-summary-toggle repo-summary-toggle" type="button" :aria-expanded="openSummary.has(r.full_name)" @click="toggleSummary(r)">
              <span class="with-icon"><UiIcon name="sparkles" />{{ openSummary.has(r.full_name) ? t('repos.summaryHide') : t('repos.summaryShow') }}</span>
              <svg class="yt-chevron" :class="{ 'is-collapsed': !openSummary.has(r.full_name) }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            </button>
            <section v-if="openSummary.has(r.full_name)" class="yt-summary repo-summary" :aria-label="t('repos.summaryShow')">
              <template v-for="sum in [summaryOf(r.full_name)]" :key="'s' + r.full_name">
                <p v-if="!sum" class="yt-sum-note">{{ t('common.loading') }}</p>
                <p v-else-if="sum.status === 'pending' || sum.status === 'none'" class="yt-sum-note"><span class="yt-spinner" aria-hidden="true" />{{ t('repos.summaryPending') }}</p>
                <div v-else-if="sum.status === 'failed'" class="yt-sum-note">
                  <p class="yt-sum-error">{{ t('repos.summaryFailed', { error: sum.error }) }}</p>
                  <IconBtn class="btn-sm" icon="reset" :label="t('repos.summaryRetry')" @click="generateSummary(r, true)" />
                </div>
                <template v-if="sum && sum.summary && sum.status !== 'pending'">
                  <p v-for="(para, pi) in sum.summary.split(/\n\s*\n/).filter(Boolean)" :key="'p' + pi" class="yt-sum-para">{{ para }}</p>
                  <template v-if="(sum.key_points || []).length">
                    <h3 class="yt-sum-h">{{ t('repos.summaryPoints') }}</h3>
                    <ul class="yt-sum-points"><li v-for="(k, ki) in sum.key_points" :key="'k' + ki">{{ k }}</li></ul>
                  </template>
                  <template v-if="(sum.resources || []).length">
                    <h3 class="yt-sum-h">{{ t('repos.summaryResources') }}</h3>
                    <ul class="yt-sum-points">
                      <li v-for="(x, xi) in (sum.resources || [])" :key="'r' + xi">
                        <a v-if="x.url" :href="x.url" target="_blank" rel="noopener">{{ x.name }}</a><span v-else>{{ x.name }}</span><span v-if="x.note" style="color:var(--text-muted)"> — {{ x.note }}</span>
                      </li>
                    </ul>
                  </template>
                  <div class="yt-sum-foot">
                    <span>{{ sum.source === 'description' ? t('repos.summaryFromFacts') : t('repos.summaryFromReadme') }}<template v-if="sum.updated_at"> · {{ t('repos.summaryAt', { time: sum.updated_at }) }}</template></span>
                    <IconBtn v-if="sum.status === 'done'" class="btn-sm" icon="refresh" :label="t('repos.summaryRegenerate')" @click="generateSummary(r, true)" />
                  </div>
                </template>
              </template>
            </section>

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
              <IconBtn class="btn-sm" :icon="expanded.has(r.full_name) ? 'chevron-up' : 'info'" :class="{ 'btn-on': expanded.has(r.full_name) }" :busy="detailLoading.has(r.full_name)" :label="expanded.has(r.full_name) ? t('repos.collapseDetail') : t('repos.expandDetail')" :aria-expanded="expanded.has(r.full_name)" :disabled="detailLoading.has(r.full_name)" @click="toggleDetail(r)" />
              <IconBtn class="btn-sm" icon="refresh" :busy="syncing.has(r.full_name)" :label="syncing.has(r.full_name) ? t('repos.refreshing') : t('repos.refresh')" :disabled="syncing.has(r.full_name)" @click="doSync(r)" />
              <IconBtn class="btn-sm" icon="pencil" :label="t('common.edit')" @click="openEdit(r)" />
              <IconBtn class="btn-sm btn-danger" icon="trash" :label="t('common.delete')" @click="doDelete(r)" />
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
                <div v-if="(details[r.full_name]!.repo.tutorial_steps || []).length" style="margin-top:8px">
                  <IconBtn class="btn-sm btn-primary" icon="fork" label="按这些步骤建复刻" @click="replicateRepo(r.full_name)" />
                </div>
                <p v-else style="color:var(--text-muted)">{{ t('repos.noTutorial') }}</p>

                <h3>{{ t('repos.iterationLog') }} <span style="color:var(--text-muted);font-size:13px;font-weight:400">{{ t('repos.versions', { n: detailReleases(r).length }) }}</span></h3>
                <div v-if="detailReleases(r).length">
                  <div v-for="rel in detailReleases(r)" :key="rel.tag_name" class="release-item">
                    <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
                      <IconBtn class="btn-sm" :icon="isReleaseOpen(r, rel) ? 'chevron-up' : 'chevron-down'" :label="isReleaseOpen(r, rel) ? t('repos.collapseRelease') : t('repos.expandRelease')" :aria-expanded="isReleaseOpen(r, rel)" @click="toggleRelease(r, rel)" />
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
          <IconBtn icon="x" :label="t('common.cancel')" @click="showAdd = false" />
          <IconBtn class="btn-primary" icon="check" :label="addBusy ? t('repos.adding') : t('repos.add')" :disabled="addBusy" @click="doAdd" />
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
          <IconBtn icon="x" :label="t('common.cancel')" @click="editTarget = null" />
          <IconBtn class="btn-primary" icon="check" :label="editBusy ? t('repos.saving') : t('common.save')" :disabled="editBusy" @click="doEdit" />
        </div>
      </div>
    </div>

    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { Repo, RepoGroup, RepoDetail, RepoRelease, YTSummary } from '~/composables/types'
import { ApiError } from '~/composables/types'
import { relativeTime, shanghaiTime, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'
import { REPO_SORTS, isRepoSort, ownerAvatarUrl, sortRepos, type RepoSort } from '~/utils/repoSort'
import { followSummary } from '~/utils/summaryStream'
import { apiBase } from '~/composables/useApi'
import { usePageActions } from '~/composables/usePageActions'

const store = useStore()
const { t, lang } = useLang()
const { tr, enqueue, translating } = useTranslate()

const loading = ref(true)
const loadError = ref('')
const repos = ref<Repo[]>([])
const groups = ref<RepoGroup[]>([])
const keyword = ref('')
const groupFilter = ref('')
const tagFilter = ref('')
// 排序方式记在本机（x-digest-repos-sort）。
const SORT_KEY = 'x-digest-repos-sort'
const sort = ref<RepoSort>('created_desc')
watch(sort, (v) => { try { localStorage.setItem(SORT_KEY, v) } catch { /* private mode */ } })
const failedAvatars = ref<Set<string>>(new Set())
function onAvatarError (owner: string) { failedAvatars.value = new Set([...failedAvatars.value, owner]) }
const filtersCollapsed = ref(false)
const activeFilterCount = computed(() =>
  (keyword.value.trim() ? 1 : 0) + (groupFilter.value ? 1 : 0) + (tagFilter.value ? 1 : 0))

const expanded = ref<Set<string>>(new Set())
const details = ref<Record<string, RepoDetail>>({})
const detailLoading = ref<Set<string>>(new Set())
const detailError = ref<Record<string, string>>({})
const openReleases = ref<Set<string>>(new Set())
const syncing = ref<Set<string>>(new Set())
const syncAllBusy = ref(false)
// 页面级操作放进侧边菜单（AppDrawer）
usePageActions(() => [
  { key: 'add', icon: 'book', plus: true, primary: true, label: t('repos.addRepo'), run: openAdd },
  { key: 'sync', icon: 'refresh', busy: syncAllBusy.value, disabled: syncAllBusy.value, label: syncAllBusy.value ? t('repos.syncing') : t('repos.syncAll'), run: syncAll }
])
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
  if (s.has(k) && rel.body) enqueue([rel.body])
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
  return sortRepos(list, sort.value)
})

// Enqueue the listed repos' intros when EN is active (deduped by the queue).
watch([filtered, lang], () => {
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
    // Enqueue the detail's translatable texts.
    enqueue([repoIntroSource(d.repo), ...(d.repo.tutorial_steps || []).map(s => s.text)])
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
// README 里解析出的上手步骤 → 复刻清单（同一仓库只建一次，已有则直接跳转）。
async function replicateRepo (fullName: string) {
  try {
    await store.createReplicaFromRepo(fullName)
    await navigateTo('/fuke')
  } catch (err: any) { syncAllMsg.value = errText(err, '建复刻失败') }
}
// 同步全部是后台任务：POST 启动，之后每 3 秒查进度（已同步 n/N，限流时显示等待到几点）。
async function syncAll () {
  if (syncAllBusy.value) return
  syncAllBusy.value = true
  syncAllMsg.value = ''
  try {
    let st = await store.syncAllRepos().catch(async (err: any) => {
      if (err?.status === 409) return await store.getRepoSyncStatus() // already running: follow it
      throw err
    })
    while (st.running) {
      syncAllMsg.value = syncProgressText(st)
      await new Promise(r => setTimeout(r, 3000))
      st = await store.getRepoSyncStatus()
    }
    syncAllMsg.value = t('repos.syncAllDone', { ok: st.synced, fail: st.failed, limited: st.rate_limited, total: st.total })
    await reload()
  } catch (err: any) {
    syncAllMsg.value = errText(err, t('repos.syncAllFailed'))
  } finally { syncAllBusy.value = false }
}
function syncProgressText (st: { done: number, total: number, waiting_until?: string }): string {
  const base = t('repos.syncProgress', { n: st.done, total: st.total })
  if (!st.waiting_until) return base
  const hhmm = new Date(st.waiting_until).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Shanghai' })
  return `${base}${t('repos.syncWaiting', { time: hhmm })}`
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

// ---- AI 智能摘要（同帖子摘要：先读库，没有就生成；生成中优先 SSE，失败退回轮询） ----
const openSummary = ref<Set<string>>(new Set())
const summaries = ref<Record<string, YTSummary>>({})
const pollTimers = new Map<string, ReturnType<typeof setTimeout>>()
const streaming = new Set<string>()
function sumKey (fullName: string) { return `${fullName}|${lang.value}` }
function summaryOf (fullName: string): YTSummary | undefined { return summaries.value[sumKey(fullName)] }
function setSummary (sum: YTSummary) { summaries.value = { ...summaries.value, [`${sum.video_id}|${sum.lang}`]: sum } }
function failSummary (r: Repo, msg: string) {
  const prev = summaryOf(r.full_name)
  setSummary({ video_id: r.full_name, lang: lang.value, status: 'failed', summary: prev?.summary || '', key_points: prev?.key_points || [], chapters: [], resources: prev?.resources || [], source: prev?.source || '', transcript_lang: '', model: '', error: msg })
}
function pollSummary (r: Repo) {
  const key = sumKey(r.full_name)
  if (pollTimers.has(key)) return
  const l = lang.value
  pollTimers.set(key, setTimeout(async () => {
    pollTimers.delete(key)
    try {
      const sum = await store.getRepoSummary(r.full_name, l)
      setSummary(sum)
      if (sum.status === 'pending' && l === lang.value) pollSummary(r)
    } catch { if (l === lang.value) pollSummary(r) }
  }, 4000))
}
function schedulePoll (r: Repo) {
  const key = sumKey(r.full_name)
  if (streaming.has(key) || pollTimers.has(key)) return
  const l = lang.value
  if (!store.mock.value && followSummary(`${apiBase()}/repos/${r.full_name}/summary/stream?lang=${l}`, (sum) => { setSummary(sum); if (sum.status !== 'pending') streaming.delete(key) }, () => { streaming.delete(key); pollSummary(r) })) {
    streaming.add(key)
    return
  }
  pollSummary(r)
}
async function generateSummary (r: Repo, force = false) {
  const prev = summaryOf(r.full_name)
  setSummary({ ...(prev || { video_id: r.full_name, lang: lang.value, summary: '', key_points: [], chapters: [], resources: [], source: '', transcript_lang: '', model: '', error: '' }), status: 'pending', error: '' })
  try {
    const sum = await store.startRepoSummary(r.full_name, lang.value, force)
    setSummary(sum)
    if (sum.status === 'pending') schedulePoll(r)
  } catch (err: any) { failSummary(r, errText(err, t('repos.summaryLoadFailed'))) }
}
async function loadSummary (r: Repo) {
  try {
    const sum = await store.getRepoSummary(r.full_name, lang.value)
    if (sum.status === 'none') return generateSummary(r)
    setSummary(sum)
    if (sum.status === 'pending') schedulePoll(r)
  } catch (err: any) { failSummary(r, errText(err, t('repos.summaryLoadFailed'))) }
}
function toggleSummary (r: Repo) {
  const s = new Set(openSummary.value)
  if (s.has(r.full_name)) { s.delete(r.full_name); openSummary.value = s; return }
  s.add(r.full_name); openSummary.value = s
  const cur = summaryOf(r.full_name)
  if (!cur || cur.status === 'none') loadSummary(r)
}
onUnmounted(() => { for (const tm of pollTimers.values()) clearTimeout(tm); pollTimers.clear() })

onMounted(async () => {
  try {
    const v = localStorage.getItem(SORT_KEY)
    if (isRepoSort(v)) sort.value = v
  } catch { /* ignore */ }
  try {
    await reload()
  } catch (err: any) { loadError.value = errText(err, t('common.loadError')) }
  finally { loading.value = false }
})
</script>
