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
            <span :class="['sync-badge', `sync-${c.sync_status}`]">{{ syncLabel(c.sync_status) }}</span>
            <span style="color:var(--text-muted)">
              {{ c.last_synced_at ? t('yt.lastSynced', { time: shanghaiTime(c.last_synced_at) }) : t('yt.neverSyncedShort') }}
            </span>
            <span v-if="c.sync_error" style="color:var(--danger-text)" :title="c.sync_error">{{ trunc40(c.sync_error) }}</span>
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
            <div style="min-width:0">
              <h2 class="post-title" style="margin:0">
                <a :href="v.url" target="_blank" rel="noopener">{{ tr(v.title) }}</a>
              </h2>
              <p style="margin:6px 0 0;color:var(--text-muted);font-size:13px">
                {{ v.channel_name || v.channel_id }} · {{ shanghaiTime(v.published_at) }}（{{ relativeTime(v.published_at, lang.value) }}）
              </p>
              <p v-if="v.description" class="yt-desc" :class="{ 'yt-desc-open': expandedDesc.has(v.id) }" style="margin:6px 0 0">{{ tr(v.description) }}</p>
              <p v-else style="margin:6px 0 0;color:var(--text-muted)">{{ t('yt.noDesc') }}</p>
              <button v-if="(v.description || '').length > 140" class="btn btn-sm" type="button" style="margin-top:6px" @click="toggleDesc(v.id)">
                {{ expandedDesc.has(v.id) ? t('yt.collapseDesc') : t('yt.expandDesc') }}
              </button>
            </div>
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
        <p v-if="addError" style="color:var(--danger-text)">{{ addError }}</p>
        <div class="modal-actions">
          <button class="btn" type="button" @click="showAdd = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="button" @click="doAdd" :disabled="addBusy">{{ addBusy ? t('yt.adding') : t('yt.add') }}</button>
        </div>
      </div>
    </div>

    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { YTChannel, YTVideo, RepoGroup } from '~/composables/types'
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
const addForm = ref({ input: '', group_id: '' })
useScrollLock(computed(() => showAdd.value))

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
function matchesDate (v: YTVideo): boolean {
  const from = shanghaiDayBound(dateFrom.value, false)
  const to = shanghaiDayBound(dateTo.value, true)
  if (from == null && to == null) return true
  const ts = Date.parse(v.published_at || '')
  if (Number.isNaN(ts)) return false
  if (from != null && ts < from) return false
  if (to != null && ts > to) return false
  return true
}

const channelById = computed(() => {
  const m = new Map<string, YTChannel>()
  for (const c of channels.value) m.set(c.channel_id, c)
  return m
})
const filtered = computed(() => {
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
watch([filtered, lang], () => {
  enqueue(filtered.value.flatMap(v => [v.title, v.description || '']))
})

function clearAll () {
  keyword.value = ''; channelFilter.value = ''; groupFilter.value = ''
  dateFrom.value = ''; dateTo.value = ''
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
  addForm.value = { input: '', group_id: '' }
  addError.value = ''
  showAdd.value = true
}
async function doAdd () {
  const input = addForm.value.input.trim()
  if (!input) { addError.value = t('yt.inputLabel'); return }
  addBusy.value = true
  addError.value = ''
  try {
    const ch = await store.addYTChannel(input, addForm.value.group_id || null)
    await reload()
    showAdd.value = false
    showToast(t('yt.added', { name: ch.name || ch.channel_id }))
  } catch (err: any) {
    addError.value = errText(err, t('yt.addFailed'))
  } finally { addBusy.value = false }
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
