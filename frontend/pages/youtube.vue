<template>
  <div>
    <main class="feed-col" style="max-width:760px;margin:0 auto;padding:16px;min-width:0">
      <h1 style="margin:4px 0 12px;font-size:20px">{{ t('yt.title') }}</h1>
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">{{ t('common.loading') }}</p>

      <template v-if="!loading && !loadError">
        <!-- 工具条 -->
        <div class="filter-row">
          <span style="color:var(--text-muted)">{{ t('yt.total', { n: channels.length, v: videos.length }) }}</span>
          <span style="flex:1" />
          <button class="btn btn-sm" type="button" @click="syncAll" :disabled="syncAllBusy">{{ syncAllBusy ? t('yt.syncing') : t('yt.syncAll') }}</button>
          <button class="btn btn-sm btn-primary" type="button" @click="openAdd">{{ t('yt.addChannel') }}</button>
        </div>
        <div class="filter-row" style="justify-content:space-between;margin-top:8px">
          <button class="btn btn-sm" type="button" @click="filtersCollapsed = !filtersCollapsed" :aria-expanded="!filtersCollapsed">{{ filtersCollapsed ? t('yt.expandFilters') : t('yt.collapseFilters') }}</button>
          <span class="active-count" v-if="filtersCollapsed && activeFilterCount">{{ t('index.selectedFilters', { n: activeFilterCount }) }}</span>
        </div>
        <div v-show="!filtersCollapsed">
          <div class="unified-bar">
            <div class="cond">
              <label class="cond-label">{{ t('common.keywordSearch') }}</label>
              <input class="field" style="width:100%" type="search" v-model="keyword" :placeholder="t('yt.keywordPh')" :aria-label="t('common.keywordSearch')" />
            </div>
            <div class="cond">
              <label class="cond-label">{{ t('yt.group') }}</label>
              <select class="field" style="width:100%" v-model="groupFilter">
                <option value="">{{ t('yt.allGroups') }}</option>
                <option value="__ungrouped">{{ t('yt.ungrouped') }}</option>
                <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
              </select>
            </div>
            <div class="cond-date">
              <label class="cond-label">{{ t('yt.date') }}</label>
              <div style="display:flex;gap:6px">
                <input type="date" class="field" style="flex:1;min-width:0" v-model="dateFrom" :aria-label="t('yt.dateFromAria')" />
                <input type="date" class="field" style="flex:1;min-width:0" v-model="dateTo" :aria-label="t('yt.dateToAria')" />
              </div>
            </div>
          </div>
          <div v-if="channels.length" style="margin-top:8px">
            <div class="chip-group-title">{{ t('yt.channel') }}</div>
            <div class="chips">
              <button v-for="c in channels" :key="c.id" type="button" class="chip"
                :class="{ active: channelFilter === c.channel_id }" @click="channelFilter = channelFilter === c.channel_id ? '' : c.channel_id">{{ c.name || c.channel_id }}</button>
            </div>
          </div>
          <div class="filter-row" style="margin-top:8px">
            <button class="btn btn-sm" type="button" @click="clearAll">{{ t('yt.clearAll') }}</button>
          </div>
        </div>
        <p v-if="syncAllMsg" style="color:var(--text-muted)">{{ syncAllMsg }}</p>
        <p v-if="translating" style="color:var(--text-muted)">{{ t('common.translating') }}</p>

        <!-- 频道管理 -->
        <section class="settings-section">
          <h3 style="margin:8px 0">{{ t('yt.channels') }}</h3>
          <div v-for="c in channels" :key="c.id" class="watch-row">
            <strong>{{ c.name || c.channel_id }}</strong>
            <span v-if="c.handle" style="color:var(--text-muted)">{{ c.handle }}</span>
            <span v-if="groupName(c.group_id)" class="chip active">{{ groupName(c.group_id) }}</span>
            <span style="color:var(--text-muted)">{{ t('yt.videosCount', { n: c.video_count }) }}</span>
            <span style="color:var(--text-muted)">{{ rangeText(c) }}</span>
            <span :class="['sync-badge', `sync-${c.sync_status}`]">{{ syncLabel(c.sync_status) }}</span>
            <span style="color:var(--text-muted)">
              {{ c.last_synced_at ? t('yt.lastSynced', { time: shanghaiTime(c.last_synced_at) }) : t('yt.neverSyncedShort') }}
            </span>
            <span v-if="c.sync_error" style="color:var(--danger-text)" :title="c.sync_error">{{ trunc40(c.sync_error) }}</span>
            <button class="btn btn-sm" type="button" @click="openEdit(c)">{{ t('yt.editRange') }}</button>
            <button class="btn btn-sm" type="button" @click="doSync(c)" :disabled="syncing.has(c.id)">
              {{ syncing.has(c.id) ? t('yt.refreshing') : t('yt.sync') }}
            </button>
            <button class="btn btn-sm btn-danger" type="button" @click="doDelete(c)">{{ t('common.delete') }}</button>
          </div>
          <h3 style="margin:14px 0 8px">{{ t('yt.groupManage') }}</h3>
          <div style="display:flex;gap:6px;flex-wrap:wrap">
            <input class="field" v-model="newGroupName" :placeholder="t('yt.newGroupPh')" style="flex:1;min-width:140px" />
            <button class="btn btn-sm btn-primary" type="button" @click="doAddGroup">{{ t('yt.addGroup') }}</button>
          </div>
          <div v-for="g in groups" :key="g.id" class="watch-row">
            <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">（{{ channelCountIn(g.id) }}）</span>
            <button v-if="!g.fixed" class="btn btn-sm btn-danger" type="button" @click="doDeleteGroup(g)">{{ t('yt.deleteGroup') }}</button>
          </div>
        </section>

        <!-- 四个分类页签 -->
        <div class="chips" style="margin-top:16px" role="tablist" :aria-label="t('yt.title')">
          <button v-for="tab in tabs" :key="tab.key" type="button" class="chip"
            :class="{ active: activeTab === tab.key }" role="tab" :aria-selected="activeTab === tab.key"
            @click="activeTab = tab.key">{{ t(tab.label) }}（{{ tabCounts[tab.key] }}）</button>
        </div>

        <!-- 空状态 -->
        <p v-if="!channels.length" class="empty" style="margin-top:16px">{{ t('yt.empty') }}</p>
        <p v-else-if="!filtered.length" class="empty" style="margin-top:16px">{{ t('yt.noMatch') }}</p>

        <!-- 视频卡片（最新在前） -->
        <section class="repo-list" style="margin-top:16px">
          <article v-for="v in filtered" :key="v.id" class="post-card yt-video-card">
            <a :href="v.url" target="_blank" rel="noopener" :title="t('yt.watchOnYouTube')" class="yt-thumb">
              <img v-if="v.thumbnail_url" :src="v.thumbnail_url" :alt="v.title" loading="lazy" />
              <span v-else class="yt-thumb-ph">▶</span>
            </a>
            <div class="yt-info">
              <h2 class="post-title yt-title">
                <a :href="v.url" target="_blank" rel="noopener">{{ tr(v.title) }}</a>
              </h2>
              <p class="yt-meta">
                {{ v.channel_name || v.channel_id }}<template v-if="v.published_at"> · {{ shanghaiTime(v.published_at) }}（{{ relativeTime(v.published_at, lang.value) }}）</template>
              </p>
              <div class="yt-tags">
                <span class="chip yt-tag">{{ kindLabel(v) }}</span>
                <span v-if="v.live_status === 'is_live'" class="chip active yt-tag">{{ t('yt.liveNow') }}</span>
                <span v-else-if="v.live_status === 'was_live'" class="chip yt-tag">{{ t('yt.liveReplay') }}</span>
              </div>
              <p v-if="v.description" class="yt-desc" :class="{ 'yt-desc-open': expandedDesc.has(v.id) }"><template v-for="(part, pi) in linkParts(tr(v.description))" :key="pi"><a v-if="part.href" class="yt-desc-link" :href="part.href" target="_blank" rel="noopener noreferrer">{{ part.text }}</a><template v-else>{{ part.text }}</template></template></p>
              <p v-else class="yt-desc-empty">{{ t('yt.noDesc') }}</p>
              <div class="yt-actions">
                <button v-if="(v.description || '').length > 140" class="btn btn-sm" type="button" @click="toggleDesc(v.id)">
                  {{ expandedDesc.has(v.id) ? t('yt.collapseDesc') : t('yt.expandDesc') }}
                </button>
                <button v-if="videoKind(v) !== 'playlist'" class="btn btn-sm yt-ai-btn" type="button" :aria-expanded="openSummary.has(v.id)" @click="toggleSummary(v)">
                  ✨ {{ openSummary.has(v.id) ? t('yt.aiSummaryHide') : t('yt.aiSummary') }}
                </button>
              </div>
            </div>
            <section v-if="openSummary.has(v.id)" class="yt-summary" :aria-label="t('yt.aiSummary')">
              <template v-for="sum in [summaryOf(v.id)]" :key="'s' + v.id">
                <p v-if="!sum" class="yt-sum-note">{{ t('common.loading') }}</p>
                <p v-else-if="sum.status === 'pending' || sum.status === 'none'" class="yt-sum-note"><span class="yt-spinner" aria-hidden="true" />{{ t('yt.aiPending') }}</p>
                <div v-else-if="sum.status === 'failed'" class="yt-sum-note">
                  <p class="yt-sum-error">{{ t('yt.aiFailed', { msg: sum.error || '' }) }}</p>
                  <button class="btn btn-sm" type="button" @click="generateSummary(v, true)">{{ t('yt.aiRetry') }}</button>
                </div>
                <template v-if="sum && sum.summary && sum.status !== 'pending'">
                  <h3 class="yt-sum-h">{{ t('yt.aiFullSummary') }}</h3>
                  <p v-for="(para, pi) in paragraphs(sum.summary)" :key="'p' + pi" class="yt-sum-para">{{ para }}</p>
                  <template v-if="sum.key_points.length">
                    <h3 class="yt-sum-h">{{ t('yt.aiKeyPoints') }}</h3>
                    <ul class="yt-sum-points"><li v-for="(k, ki) in sum.key_points" :key="'k' + ki">{{ k }}</li></ul>
                  </template>
                  <h3 class="yt-sum-h">{{ t('yt.aiChapters') }}</h3>
                  <ol v-if="sum.chapters.length" class="yt-chapters">
                    <li v-for="c in sum.chapters" :key="'c' + c.start" class="yt-chapter">
                      <a class="yt-ch-time" :href="watchAt(v, c.start)" target="_blank" rel="noopener" :title="t('yt.aiJumpTo', { time: clock(c.start) })">{{ clock(c.start) }}</a>
                      <div class="yt-ch-body">
                        <div class="yt-ch-title">{{ c.title }}</div>
                        <p v-if="c.summary" class="yt-ch-sum">{{ c.summary }}</p>
                      </div>
                    </li>
                  </ol>
                  <p v-else class="yt-sum-note">{{ t('yt.aiNoChapters') }}</p>
                  <div class="yt-sum-foot">
                    <span>{{ sum.source === 'description' ? t('yt.aiSourceDescription') : t('yt.aiSourceTranscript', { lang: sum.transcript_lang || '—' }) }}<template v-if="sum.updated_at"> · {{ t('yt.aiGeneratedAt', { time: sum.updated_at }) }}</template></span>
                    <button v-if="sum.status === 'done'" class="btn btn-sm" type="button" @click="generateSummary(v, true)">{{ t('yt.aiRegenerate') }}</button>
                  </div>
                </template>
              </template>
            </section>
          </article>
        </section>
      </template>
    </main>

    <!-- 添加频道 -->
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
      <div class="modal-card" role="dialog" aria-modal="true" :aria-label="t('yt.addTitle')">
        <h2>{{ t('yt.addTitle') }}</h2>
        <label>{{ t('yt.inputLabel') }}</label>
        <input class="field" v-model="addForm.input" :placeholder="t('yt.inputPh')" @keyup.enter="doAdd" />
        <p style="color:var(--text-muted);font-size:13px">{{ t('yt.inputHint') }}</p>
        <label>{{ t('yt.groupLabel') }}</label>
        <select class="field" v-model="addForm.group_id">
          <option value="">{{ t('yt.ungrouped') }}</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <label>{{ t('yt.rangeLabel') }}</label>
        <div style="display:flex;gap:6px">
          <input type="date" class="field" style="flex:1;min-width:0" v-model="addForm.date_from" :aria-label="t('yt.rangeFrom')" />
          <input type="date" class="field" style="flex:1;min-width:0" v-model="addForm.date_to" :aria-label="t('yt.rangeTo')" />
        </div>
        <p style="color:var(--text-muted);font-size:13px">{{ t('yt.rangeHint') }}</p>
        <p v-if="addError" style="color:var(--danger-text)">{{ addError }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="showAdd = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="button" @click="doAdd" :disabled="addBusy">{{ addBusy ? t('yt.adding') : t('yt.add') }}</button>
        </div>
      </div>
    </div>

    <!-- 编辑拉取范围 -->
    <div v-if="showEdit" class="modal-backdrop" @click.self="showEdit = false">
      <div class="modal-card" role="dialog" aria-modal="true" :aria-label="t('yt.editRangeTitle')">
        <h2>{{ t('yt.editRangeTitle') }} · {{ editChannel?.name || editChannel?.channel_id }}</h2>
        <label>{{ t('yt.groupLabel') }}</label>
        <select class="field" v-model="editForm.group_id">
          <option value="">{{ t('yt.ungrouped') }}</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <label>{{ t('yt.rangeLabel') }}</label>
        <div style="display:flex;gap:6px">
          <input type="date" class="field" style="flex:1;min-width:0" v-model="editForm.date_from" :aria-label="t('yt.rangeFrom')" />
          <input type="date" class="field" style="flex:1;min-width:0" v-model="editForm.date_to" :aria-label="t('yt.rangeTo')" />
        </div>
        <p style="color:var(--text-muted);font-size:13px">{{ t('yt.rangeHint') }}</p>
        <p v-if="editError" style="color:var(--danger-text)">{{ editError }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="showEdit = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="button" @click="doEdit" :disabled="editBusy">{{ t('common.save') }}</button>
        </div>
      </div>
    </div>

    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { YTChannel, YTVideo, YTSummary, RepoGroup } from '~/composables/types'
import { ApiError } from '~/composables/types'
import { relativeTime, shanghaiTime, shanghaiDayBound, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'

const store = useStore()
const { t, lang } = useLang()
const { tr, enqueue, translating } = useTranslate()

const loading = ref(true)
const loadError = ref('')
const channels = ref<YTChannel[]>([])
const videos = ref<YTVideo[]>([])
const groups = ref<RepoGroup[]>([])
const keyword = ref('')
const channelFilter = ref('')
const groupFilter = ref('')
const dateFrom = ref('')
const dateTo = ref('')
type YTKind = 'video' | 'short' | 'live' | 'playlist'
const tabs: Array<{ key: YTKind, label: string }> = [
  { key: 'video', label: 'yt.tabVideo' },
  { key: 'short', label: 'yt.tabShort' },
  { key: 'live', label: 'yt.tabLive' },
  { key: 'playlist', label: 'yt.tabPlaylist' }
]
const activeTab = ref<YTKind>('video')
const filtersCollapsed = ref(false)
const activeFilterCount = computed(() =>
  (keyword.value.trim() ? 1 : 0) + (channelFilter.value ? 1 : 0) + (groupFilter.value ? 1 : 0) +
  (dateFrom.value || dateTo.value ? 1 : 0))

const expandedDesc = ref<Set<string>>(new Set())
const syncing = ref<Set<string>>(new Set())
const syncAllBusy = ref(false)
const syncAllMsg = ref('')

const showAdd = ref(false)
const addBusy = ref(false)
const addError = ref('')
const addForm = ref({ input: '', group_id: '', date_from: '', date_to: '' })
const showEdit = ref(false)
const editBusy = ref(false)
const editError = ref('')
const editChannel = ref<YTChannel | null>(null)
const editForm = ref({ group_id: '', date_from: '', date_to: '' })
useScrollLock(computed(() => showAdd.value || showEdit.value))

const newGroupName = ref('')
const toast = ref('')

function showToast (msg: string) { toast.value = msg; setTimeout(() => { toast.value = '' }, 3000) }
function errText (err: any, fallback: string): string {
  if (err instanceof ApiError) {
    return err.status ? `(${err.status}) ${err.message}` : err.message
  }
  return err?.message || fallback
}
function groupName (id?: string | null): string {
  if (!id) return ''
  return groups.value.find(g => g.id === id)?.name || ''
}
function channelCountIn (groupId: string): number {
  return channels.value.filter(c => (c.group_id || '') === groupId).length
}
function syncLabel (status: string): string {
  switch (status) {
    case 'synced': return t('yt.synced')
    case 'failed': return t('yt.syncFailed')
    default: return t('yt.neverSynced')
  }
}
function videoKind (v: YTVideo): YTKind {
  return v.kind || 'video'
}
function kindLabel (v: YTVideo): string {
  switch (videoKind(v)) {
    case 'short': return t('yt.tabShort')
    case 'live': return t('yt.tabLive')
    case 'playlist': return t('yt.playlistBadge')
    default: return t('yt.tabVideo')
  }
}
// 简介按纯文本渲染，只把 http(s) 链接切出来做成 <a>（不用 v-html，避免注入）。
// 链接末尾的中英文标点、括号不算进链接。
const URL_RE = /https?:\/\/[^\s<>"'，。；！？、）】」]+/g
function linkParts (text: string): Array<{ text: string, href?: string }> {
  const parts: Array<{ text: string, href?: string }> = []
  let last = 0
  for (const m of text.matchAll(URL_RE)) {
    let url = m[0]
    const trail = url.match(/[.,;:!?)\]}'"]+$/)
    if (trail) url = url.slice(0, -trail[0].length)
    const start = m.index!
    if (start > last) parts.push({ text: text.slice(last, start) })
    parts.push({ text: url, href: url })
    last = start + url.length
  }
  if (last < text.length) parts.push({ text: text.slice(last) })
  return parts
}
function rangeText (c: YTChannel): string {
  if (!c.date_from && !c.date_to) return t('yt.allTime')
  return `${c.date_from || '…'} ~ ${c.date_to || '…'}`
}
function matchesDate (v: YTVideo): boolean {
  const from = shanghaiDayBound(dateFrom.value, false)
  const to = shanghaiDayBound(dateTo.value, true)
  if (from == null && to == null) return true
  // Unknown date (playlists, metadata not fetched yet) cannot be
  // excluded by a date filter — keep it visible.
  const ts = Date.parse(v.published_at || '')
  if (Number.isNaN(ts)) return true
  if (from != null && ts < from) return false
  if (to != null && ts > to) return false
  return true
}

const channelById = computed(() => {
  const m = new Map<string, YTChannel>()
  for (const c of channels.value) m.set(c.channel_id, c)
  return m
})
const filteredExceptKind = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return videos.value.filter((v) => {
    // Filtering always uses the original text, never the translation.
    if (q && !((v.title || '').toLowerCase().includes(q) || (v.description || '').toLowerCase().includes(q))) return false
    if (channelFilter.value && v.channel_id !== channelFilter.value) return false
    if (groupFilter.value) {
      const g = channelById.value.get(v.channel_id)?.group_id || ''
      if (groupFilter.value === '__ungrouped') { if (g) return false } else if (g !== groupFilter.value) return false
    }
    if (!matchesDate(v)) return false
    return true
  })
})
// Tab counts respect every filter except the tab itself.
const tabCounts = computed(() => {
  const counts: Record<YTKind, number> = { video: 0, short: 0, live: 0, playlist: 0 }
  for (const v of filteredExceptKind.value) counts[videoKind(v)]++
  return counts
})
const filtered = computed(() => filteredExceptKind.value.filter(v => videoKind(v) === activeTab.value))
watch([filtered, lang], () => {
  enqueue(filtered.value.flatMap(v => [v.title, v.description || '']))
})

function clearAll () {
  keyword.value = ''; channelFilter.value = ''; groupFilter.value = ''
  dateFrom.value = ''; dateTo.value = ''
  activeTab.value = 'video'
}
// ---- AI summary (per video, in the current UI language) ----
// Cached per "id|lang"; while a job is pending the panel polls GET every
// 4s. Generating starts on the first open when none exists yet.
const summaries = ref<Record<string, YTSummary>>({})
const openSummary = ref<Set<string>>(new Set())
const pollTimers = new Map<string, ReturnType<typeof setTimeout>>()
function sumKey (id: string) { return `${id}|${lang.value}` }
function summaryOf (id: string): YTSummary | undefined { return summaries.value[sumKey(id)] }
function setSummary (sum: YTSummary) { summaries.value = { ...summaries.value, [`${sum.video_id}|${sum.lang}`]: sum } }
function failSummary (v: YTVideo, msg: string) {
  setSummary({ video_id: v.id, lang: lang.value, status: 'failed', summary: summaryOf(v.id)?.summary || '', key_points: summaryOf(v.id)?.key_points || [], chapters: summaryOf(v.id)?.chapters || [], source: summaryOf(v.id)?.source || '', transcript_lang: summaryOf(v.id)?.transcript_lang || '', model: '', error: msg })
}
function schedulePoll (v: YTVideo) {
  const key = sumKey(v.id)
  if (pollTimers.has(key)) return
  const l = lang.value
  pollTimers.set(key, setTimeout(async () => {
    pollTimers.delete(key)
    try {
      const sum = await store.getYTSummary(v.id, l)
      setSummary(sum)
      if (sum.status === 'pending' && l === lang.value) schedulePoll(v)
    } catch (err: any) {
      if (l === lang.value) schedulePoll(v)
    }
  }, 4000))
}
async function generateSummary (v: YTVideo, force = false) {
  const prev = summaryOf(v.id)
  setSummary({ ...(prev || { video_id: v.id, lang: lang.value, summary: '', key_points: [], chapters: [], source: '', transcript_lang: '', model: '', error: '' }), status: 'pending', error: '' })
  try {
    const sum = await store.startYTSummary(v.id, lang.value, force)
    setSummary(sum)
    if (sum.status === 'pending') schedulePoll(v)
  } catch (err: any) { failSummary(v, errText(err, t('yt.aiLoadFailed'))) }
}
async function loadSummary (v: YTVideo) {
  try {
    const sum = await store.getYTSummary(v.id, lang.value)
    if (sum.status === 'none') return generateSummary(v)
    setSummary(sum)
    if (sum.status === 'pending') schedulePoll(v)
  } catch (err: any) { failSummary(v, errText(err, t('yt.aiLoadFailed'))) }
}
function toggleSummary (v: YTVideo) {
  const s = new Set(openSummary.value)
  if (s.has(v.id)) { s.delete(v.id); openSummary.value = s; return }
  s.add(v.id); openSummary.value = s
  const cur = summaryOf(v.id)
  if (!cur || cur.status === 'none') loadSummary(v)
}
// Switching the UI language shows (or generates) the summary in the new language.
watch(lang, () => {
  for (const v of videos.value) if (openSummary.value.has(v.id) && !summaryOf(v.id)) loadSummary(v)
})
onUnmounted(() => { for (const tm of pollTimers.values()) clearTimeout(tm); pollTimers.clear() })
function paragraphs (text: string): string[] { return text.split(/\n\s*\n/).map(p => p.trim()).filter(Boolean) }
function clock (secs: number): string {
  const h = Math.floor(secs / 3600); const m = Math.floor(secs % 3600 / 60); const sec = Math.floor(secs % 60)
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(sec)}` : `${pad(m)}:${pad(sec)}`
}
function watchAt (v: YTVideo, secs: number): string {
  return `https://www.youtube.com/watch?v=${encodeURIComponent(v.id)}&t=${Math.floor(secs)}s`
}
function toggleDesc (id: string) {
  const s = new Set(expandedDesc.value)
  if (s.has(id)) s.delete(id); else s.add(id)
  expandedDesc.value = s
}
async function reload () {
  const [cs, vs, gs] = await Promise.all([store.loadYTChannels(), store.loadYTVideos(), store.loadYTGroups()])
  channels.value = cs
  videos.value = vs
  groups.value = gs
}
function openAdd () {
  addForm.value = { input: '', group_id: '', date_from: '', date_to: '' }
  addError.value = ''
  showAdd.value = true
}
async function doAdd () {
  const input = addForm.value.input.trim()
  if (!input) { addError.value = t('yt.inputLabel'); return }
  addBusy.value = true
  addError.value = ''
  try {
    const ch = await store.addYTChannel(input, addForm.value.group_id || null, addForm.value.date_from || null, addForm.value.date_to || null)
    await reload()
    showAdd.value = false
    showToast(t('yt.added', { name: ch.name || ch.channel_id }))
  } catch (err: any) {
    addError.value = errText(err, t('yt.addFailed'))
  } finally { addBusy.value = false }
}
function openEdit (c: YTChannel) {
  editChannel.value = c
  editForm.value = { group_id: c.group_id || '', date_from: c.date_from || '', date_to: c.date_to || '' }
  editError.value = ''
  showEdit.value = true
}
async function doEdit () {
  if (!editChannel.value) return
  editBusy.value = true
  editError.value = ''
  try {
    const updated = await store.updateYTChannel(editChannel.value.id, {
      group_id: editForm.value.group_id || null,
      date_from: editForm.value.date_from || null,
      date_to: editForm.value.date_to || null
    })
    channels.value = channels.value.map(x => x.id === updated.id ? updated : x)
    showEdit.value = false
    showToast(t('yt.rangeUpdated'))
  } catch (err: any) {
    editError.value = errText(err, t('yt.rangeUpdateFailed'))
  } finally { editBusy.value = false }
}
async function doDelete (c: YTChannel) {
  if (!confirm(t('yt.deleteConfirm', { name: c.name || c.channel_id }))) return
  try {
    await store.deleteYTChannel(c.id)
    await reload()
    showToast(t('yt.deleted', { name: c.name || c.channel_id }))
  } catch (err: any) { showToast(errText(err, t('yt.deleteFailed'))) }
}
async function doSync (c: YTChannel) {
  syncing.value = new Set([...syncing.value, c.id])
  try {
    const updated = await store.syncYTChannel(c.id)
    if (updated) {
      channels.value = channels.value.map(x => x.id === c.id ? updated : x)
      showToast(t('yt.refreshed', { name: updated.name || updated.channel_id }))
    }
    await reload()
  } catch (err: any) {
    showToast(errText(err, t('common.refreshFailedKeepOld')))
    await reload().catch(() => {})
  } finally {
    const s = new Set(syncing.value); s.delete(c.id); syncing.value = s
  }
}
async function syncAll () {
  if (syncAllBusy.value) return
  syncAllBusy.value = true
  syncAllMsg.value = ''
  try {
    const res = await store.syncAllYTChannels()
    syncAllMsg.value = t('yt.syncAllDone', { ok: res.synced, fail: res.failed, total: res.total })
    await reload()
  } catch (err: any) {
    syncAllMsg.value = errText(err, t('yt.syncAllFailed'))
  } finally { syncAllBusy.value = false }
}
async function doAddGroup () {
  if (!newGroupName.value.trim()) return
  try {
    const g = await store.addYTGroup(newGroupName.value.trim())
    groups.value = [...groups.value, g]
    newGroupName.value = ''
  } catch (err: any) { showToast(errText(err, t('yt.addGroupFailed'))) }
}
async function doDeleteGroup (g: RepoGroup) {
  if (!confirm(t('yt.deleteGroupConfirm', { name: g.name }))) return
  try {
    await store.deleteYTGroup(g.id)
    await reload()
    showToast(t('yt.groupDeleted', { name: g.name }))
  } catch (err: any) { showToast(errText(err, t('yt.deleteGroupFailed'))) }
}

onMounted(async () => {
  try {
    await reload()
  } catch (err: any) { loadError.value = errText(err, t('common.loadError')) }
  finally { loading.value = false }
})
</script>
