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
          <IconBtn class="btn-sm" icon="refresh" :busy="syncAllBusy" :label="syncAllBusy ? t('yt.syncing') : t('yt.syncAll')" :disabled="syncAllBusy" @click="syncAll" />
          <IconBtn v-if="P === 'yt'" class="btn-sm" icon="video" plus :label="t('yt.addVideo')" @click="openAdd('video')" />
          <IconBtn class="btn-sm btn-primary" icon="tv" plus :label="t('yt.addChannel')" @click="openAdd('channel')" />
        </div>
        <div class="filter-row" style="justify-content:space-between;margin-top:8px">
          <IconBtn class="btn-sm" icon="filter" :class="{ 'btn-on': !filtersCollapsed }" :label="filtersCollapsed ? t('yt.expandFilters') : t('yt.collapseFilters')" :aria-expanded="!filtersCollapsed" @click="filtersCollapsed = !filtersCollapsed" />
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
          <div v-if="tagCounts.length" style="margin-top:8px">
            <div class="chip-group-title">{{ t('yt.tags') }}</div>
            <div class="chips">
              <button v-for="tc in tagCounts" :key="tc.tag" type="button" class="chip"
                :class="{ active: tagFilter === tc.tag }" @click="tagFilter = tagFilter === tc.tag ? '' : tc.tag">{{ tc.tag }}（{{ tc.n }}）</button>
            </div>
          </div>
          <div class="filter-row" style="margin-top:8px">
            <IconBtn class="btn-sm" icon="eraser" :label="t('yt.clearAll')" @click="clearAll" />
          </div>
        </div>
        <p v-if="syncAllMsg" style="color:var(--text-muted)">{{ syncAllMsg }}</p>
        <p v-if="translating" style="color:var(--text-muted)">{{ t('common.translating') }}</p>

        <!-- 频道管理 -->
        <section class="settings-section">
          <button class="yt-section-toggle" type="button" :aria-expanded="!collapsedSections.channels" @click="toggleSection('channels')">
            <svg class="yt-chevron" :class="{ 'is-collapsed': collapsedSections.channels }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            <span>{{ t('yt.channels') }}</span><span class="yt-section-count">（{{ channels.length }}）</span>
          </button>
          <template v-if="!collapsedSections.channels">
          <div v-for="c in channels" :key="c.id" class="watch-row">
            <strong>{{ c.name || c.channel_id }}</strong>
            <span v-if="c.handle" style="color:var(--text-muted)">{{ c.handle }}</span>
            <span v-if="groupName(c.group_id)" class="chip active">{{ groupName(c.group_id) }}</span>
            <span style="color:var(--text-muted)">{{ t('yt.videosCount', { n: c.video_count }) }}</span>
            <span style="color:var(--text-muted)">{{ rangeText(c) }}</span>
            <span v-if="c.sync_mode === 'single'" class="chip yt-single-badge" :title="t('yt.singleModeHint')">{{ t('yt.singleMode') }}</span>
            <span :class="['sync-badge', `sync-${c.sync_status}`]">{{ syncLabel(c.sync_status) }}</span>
            <span style="color:var(--text-muted)">
              {{ c.last_synced_at ? t('yt.lastSynced', { time: shanghaiTime(c.last_synced_at) }) : t('yt.neverSyncedShort') }}
            </span>
            <span v-if="c.sync_error" style="color:var(--danger-text)" :title="c.sync_error">{{ trunc40(c.sync_error) }}</span>
            <span class="btn-group">
            <IconBtn class="btn-sm" icon="calendar" :label="t('yt.editRange')" @click="openEdit(c)" />
            <IconBtn class="btn-sm" icon="refresh" :busy="syncing.has(c.id)" :label="syncing.has(c.id) ? t('yt.refreshing') : t('yt.sync')" :disabled="syncing.has(c.id)" @click="doSync(c)" />
            <IconBtn v-if="c.sync_mode === 'single'" class="btn-sm btn-primary" icon="cloud-download" :label="t('yt.syncChannelAll')" :disabled="syncing.has(c.id)" @click="doSync(c, true)" />
            <IconBtn class="btn-sm btn-danger" icon="trash" :label="t('common.delete')" @click="doDelete(c)" />
            </span>
          </div>
          </template>
          <button class="yt-section-toggle" type="button" style="margin-top:10px" :aria-expanded="!collapsedSections.groups" @click="toggleSection('groups')">
            <svg class="yt-chevron" :class="{ 'is-collapsed': collapsedSections.groups }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            <span>{{ t('yt.groupManage') }}</span><span class="yt-section-count">（{{ groups.length }}）</span>
          </button>
          <template v-if="!collapsedSections.groups">
          <div style="display:flex;gap:6px;flex-wrap:wrap">
            <input class="field" v-model="newGroupName" :placeholder="t('yt.newGroupPh')" style="flex:1;min-width:140px" />
            <IconBtn class="btn-sm btn-primary" icon="folder-plus" :label="t('yt.addGroup')" @click="doAddGroup" />
          </div>
          <div v-for="g in groups" :key="g.id" class="watch-row">
            <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">（{{ channelCountIn(g.id) }}）</span>
            <IconBtn v-if="!g.fixed" class="btn-sm btn-danger" icon="trash" :label="t('yt.deleteGroup')" @click="doDeleteGroup(g)" />
          </div>
          </template>
        </section>

        <!-- 四个分类页签 -->
        <div class="chips" style="margin-top:16px" role="tablist" :aria-label="t('yt.title')">
          <button v-for="tab in tabs" :key="tab.key" type="button" class="chip"
            :class="{ active: activeTab === tab.key }" role="tab" :aria-selected="activeTab === tab.key"
            @click="activeTab = tab.key">{{ t(tab.label) }}（{{ tabCounts[tab.key] }}）</button>
          <button type="button" class="chip" :class="{ active: watchLaterOnly }" :aria-pressed="watchLaterOnly" @click="watchLaterOnly = !watchLaterOnly"><span class="with-icon"><UiIcon name="clock" :size="14" />稍后观看（{{ watchLaterCount }}）</span></button>
        </div>

        <!-- 空状态 -->
        <p v-if="!channels.length" class="empty" style="margin-top:16px">{{ t('yt.empty') }}</p>
        <p v-else-if="!filtered.length" class="empty" style="margin-top:16px">{{ t('yt.noMatch') }}</p>

        <!-- 视频卡片（最新在前） -->
        <section class="repo-list" style="margin-top:16px">
          <div v-for="grp in groupedByChannel" :key="grp.id" class="yt-channel-block">
          <button class="yt-channel-head" type="button" :aria-expanded="!collapsedChannels.has(grp.id)" @click="toggleChannelVideos(grp.id)">
            <svg class="yt-chevron" :class="{ 'is-collapsed': collapsedChannels.has(grp.id) }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            <span class="yt-channel-name">{{ grp.name }}</span><span class="yt-section-count">（{{ grp.videos.length }}）</span>
          </button>
          <template v-if="!collapsedChannels.has(grp.id)">
          <article v-for="v in grp.videos" :key="v.id" class="post-card yt-video-card" :class="{ 'is-watched': !!v.watched_at }">
            <!-- 左：缩略图 + 简介；右：标题、时间、标签 -->
            <div class="yt-left">
              <a :href="v.url" target="_blank" rel="noopener" :title="t('yt.watchOnYouTube')" class="yt-thumb">
                <img v-if="v.thumbnail_url" :src="v.thumbnail_url" :alt="v.title" loading="lazy" />
                <span v-else class="yt-thumb-ph">▶</span>
              </a>
              <p v-if="v.description" class="yt-desc" :class="{ 'yt-desc-open': expandedDesc.has(v.id) }"><template v-for="(part, pi) in linkParts(tr(v.description))" :key="pi"><a v-if="part.href" class="yt-desc-link" :href="part.href" :title="part.href" target="_blank" rel="noopener noreferrer">{{ part.text.replace(/^https?:\/\/(www\.)?/, '') }}</a><template v-else>{{ part.text }}</template></template></p>
              <p v-else class="yt-desc-empty">{{ t('yt.noDesc') }}</p>
              <IconBtn v-if="(v.description || '').length > 80" class="btn-sm x-text-tool" :icon="expandedDesc.has(v.id) ? 'chevron-up' : 'chevron-down'" :label="expandedDesc.has(v.id) ? t('yt.collapseDesc') : t('yt.expandDesc')" :aria-expanded="expandedDesc.has(v.id)" @click="toggleDesc(v.id)" />
            </div>
            <div class="yt-info">
              <h2 class="post-title yt-title">
                <a :href="v.url" target="_blank" rel="noopener">{{ tr(v.title) }}</a>
              </h2>
              <p class="yt-meta">
                <template v-if="v.published_at">{{ shanghaiTime(v.published_at) }}（{{ relativeTime(v.published_at, lang.value) }}）</template>
              </p>
              <div class="yt-tags">
                <span class="chip yt-tag">{{ kindLabel(v) }}</span>
                <span v-if="v.live_status === 'is_live'" class="chip active yt-tag">{{ t('yt.liveNow') }}</span>
                <span v-else-if="v.live_status === 'was_live'" class="chip yt-tag">{{ t('yt.liveReplay') }}</span>
                <button v-for="tag in tagsOf(v)" :key="tag" type="button" class="chip yt-tag yt-topic" :class="{ active: tagFilter === tag }" @click="tagFilter = tagFilter === tag ? '' : tag">#{{ tag }}</button>
              </div>
            </div>
            <!-- 视频提及的资源：简介里的链接 + AI 摘要识别出的工具/仓库/网站 -->
            <div v-if="resourcesOf(v).length" class="yt-resources">
              <div class="yt-res-title">{{ t('yt.resources') }}</div>
              <ul class="yt-res-list">
                <li v-for="(r, ri) in resourcesOf(v)" :key="'r' + ri">
                  <span class="yt-res-label">{{ r.label }}</span>
                  <a v-if="r.url" class="yt-desc-link" :href="r.url" target="_blank" rel="noopener noreferrer">{{ r.url.replace(/^https?:\/\/(www\.)?/, '') }}</a>
                  <span v-if="r.note" class="yt-res-note">{{ r.note }}</span>
                </li>
              </ul>
            </div>
            <div class="yt-watch-row">
              <IconBtn class="btn-sm" icon="circle-check" :class="{ 'btn-on': !!v.watched_at }" :label="v.watched_at ? '已看（点击取消）' : '标记已看'" :aria-pressed="!!v.watched_at" @click="toggleWatched(v)" />
              <IconBtn class="btn-sm" icon="clock" :class="{ 'btn-on': !!v.watch_later }" :label="v.watch_later ? '移出稍后观看' : '稍后观看'" :aria-pressed="!!v.watch_later" @click="toggleWatchLater(v)" />
            </div>
            <!-- AI 摘要：整行放在卡片模块下方（不在右侧栏内），点按展开/收起，默认收起。 -->
            <button v-if="videoKind(v) !== 'playlist'" class="yt-summary-toggle" type="button" :aria-expanded="openSummary.has(v.id)" @click="toggleSummary(v)">
              <span class="with-icon"><UiIcon name="sparkles" />{{ openSummary.has(v.id) ? t('yt.aiSummaryHide') : t('yt.aiSummary') }}</span>
              <svg class="yt-chevron" :class="{ 'is-collapsed': !openSummary.has(v.id) }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            </button>
            <section v-if="openSummary.has(v.id)" class="yt-summary" :aria-label="t('yt.aiSummary')">
              <template v-for="sum in [summaryOf(v.id)]" :key="'s' + v.id">
                <p v-if="!sum" class="yt-sum-note">{{ t('common.loading') }}</p>
                <p v-else-if="sum.status === 'pending' || sum.status === 'none'" class="yt-sum-note"><span class="yt-spinner" aria-hidden="true" />{{ t('yt.aiPending') }}</p>
                <div v-else-if="sum.status === 'failed'" class="yt-sum-note">
                  <p class="yt-sum-error">{{ t('yt.aiFailed', { msg: sum.error || '' }) }}</p>
                  <IconBtn class="btn-sm" icon="reset" :label="t('yt.aiRetry')" @click="generateSummary(v, true)" />
                </div>
                <template v-if="sum && sum.summary && sum.status !== 'pending'">
                  <h3 class="yt-sum-h">{{ t('yt.aiFullSummary') }}</h3>
                  <p v-for="(para, pi) in paragraphs(sum.summary)" :key="'p' + pi" class="yt-sum-para">{{ para }}</p>
                  <template v-if="sum.key_points.length">
                    <h3 class="yt-sum-h">{{ t('yt.aiKeyPoints') }}</h3>
                    <ul class="yt-sum-points"><li v-for="(k, ki) in sum.key_points" :key="'k' + ki">{{ k }}</li></ul>
                  </template>
                  <h3 class="yt-sum-h">{{ t('yt.aiChapters') }}</h3>
                  <!-- 点章节时间：在这里内嵌播放器并跳到该时间点 -->
                  <div v-if="playerSrc[v.id]" class="yt-embed">
                    <iframe :ref="el => setPlayerEl(v.id, el)" :src="playerSrc[v.id]" title="player" allow="autoplay; encrypted-media; picture-in-picture; fullscreen" allowfullscreen />
                    <IconBtn class="btn-sm" icon="x" label="关闭播放器" @click="closePlayer(v.id)" />
                  </div>
                  <ol v-if="sum.chapters.length" class="yt-chapters">
                    <li v-for="c in sum.chapters" :key="'c' + c.start" class="yt-chapter">
                      <a class="yt-ch-time" :href="watchAt(v, c.start)" target="_blank" rel="noopener" :title="t('yt.aiJumpTo', { time: clock(c.start) })" @click.prevent="playAt(v, c.start)">{{ clock(c.start) }}</a>
                      <div class="yt-ch-body">
                        <div class="yt-ch-title">{{ c.title }}</div>
                        <p v-if="c.summary" class="yt-ch-sum">{{ c.summary }}</p>
                      </div>
                    </li>
                  </ol>
                  <p v-else class="yt-sum-note">{{ t('yt.aiNoChapters') }}</p>
                  <div class="yt-sum-foot">
                    <span>{{ sum.source === 'description' ? t('yt.aiSourceDescription') : t('yt.aiSourceTranscript', { lang: sum.transcript_lang || '—' }) }}<template v-if="sum.updated_at"> · {{ t('yt.aiGeneratedAt', { time: sum.updated_at }) }}</template></span>
                    <IconBtn v-if="sum.status === 'done'" class="btn-sm" icon="refresh" :label="t('yt.aiRegenerate')" @click="generateSummary(v, true)" />
                  </div>
                </template>
              </template>
            </section>
          </article>
          </template>
          </div>
        </section>
      </template>
    </main>

    <!-- 添加频道 -->
    <div v-if="showAdd" class="modal-backdrop" @click.self="showAdd = false">
      <div class="modal-card" role="dialog" aria-modal="true" :aria-label="addMode === 'video' ? t('yt.addVideoTitle') : t('yt.addTitle')">
        <h2>{{ addMode === 'video' ? t('yt.addVideoTitle') : t('yt.addTitle') }}</h2>
        <label>{{ addMode === 'video' ? t('yt.videoInputLabel') : t('yt.inputLabel') }}</label>
        <input class="field" v-model="addForm.input" :placeholder="addMode === 'video' ? t('yt.videoInputPh') : t('yt.inputPh')" @keyup.enter="doAdd" />
        <p style="color:var(--text-muted);font-size:13px">{{ addMode === 'video' ? t('yt.videoInputHint') : t('yt.inputHint') }}</p>
        <label>{{ t('yt.groupLabel') }}</label>
        <select class="field" v-model="addForm.group_id">
          <option value="">{{ t('yt.ungrouped') }}</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
        </select>
        <template v-if="addMode === 'channel'">
        <label>{{ t('yt.rangeLabel') }}</label>
        <div style="display:flex;gap:6px">
          <input type="date" class="field" style="flex:1;min-width:0" v-model="addForm.date_from" :aria-label="t('yt.rangeFrom')" />
          <input type="date" class="field" style="flex:1;min-width:0" v-model="addForm.date_to" :aria-label="t('yt.rangeTo')" />
        </div>
        <p style="color:var(--text-muted);font-size:13px">{{ t('yt.rangeHint') }}</p>
        </template>
        <p v-if="addError" style="color:var(--danger-text)">{{ addError }}</p>
        <div class="modal-actions">
          <IconBtn icon="x" :label="t('common.cancel')" @click="showAdd = false" />
          <IconBtn class="btn-primary" icon="check" :label="addBusy ? t('yt.adding') : t('yt.add')" :disabled="addBusy" @click="doAdd" />
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
          <IconBtn icon="x" :label="t('common.cancel')" @click="showEdit = false" />
          <IconBtn class="btn-primary" icon="check" :label="t('common.save')" :disabled="editBusy" @click="doEdit" />
        </div>
      </div>
    </div>

    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { YTChannel, YTVideo, YTSummary, RepoGroup } from '~/composables/types'
import type { VideoPlatform } from '~/composables/useStore'
import { DICT } from '~/utils/i18n'
import { videoTags, descResources, linkParts } from '~/utils/videoTags'
import { ApiError } from '~/composables/types'
import { relativeTime, shanghaiTime, shanghaiDayBound, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'
import { followSummary } from '~/utils/summaryStream'
import { embedUrl, seekMessage } from '~/utils/embedPlayer'
import { apiBase } from '~/composables/useApi'

const store = useStore()
// One page for every video platform (YouTube, Bilibili): the API routes
// and JSON are identical under /api/v1/<platform>/. Text keys are the
// yt.* ones; a bili.* key with the same suffix overrides it on Bilibili.
const props = defineProps<{ platform: VideoPlatform }>()
const P = props.platform
const { t: baseT, lang } = useLang()
function t (key: string, params?: Record<string, string | number>): string {
  if (P !== 'yt' && key.startsWith('yt.')) {
    const k = `${P}.${key.slice(3)}`
    if (k in DICT) return baseT(k, params)
  }
  return baseT(key, params)
}
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
// Bilibili has no Shorts / live tabs: only uploads and 合集·系列.
const tabs: Array<{ key: YTKind, label: string }> = ([
  { key: 'video', label: 'yt.tabVideo' },
  { key: 'short', label: 'yt.tabShort' },
  { key: 'live', label: 'yt.tabLive' },
  { key: 'playlist', label: 'yt.tabPlaylist' }
] as Array<{ key: YTKind, label: string }>).filter(tab => P === 'yt' || tab.key === 'video' || tab.key === 'playlist')
const activeTab = ref<YTKind>('video')
const filtersCollapsed = ref(false)
const tagFilter = ref('')
const activeFilterCount = computed(() =>
  (keyword.value.trim() ? 1 : 0) + (channelFilter.value ? 1 : 0) + (groupFilter.value ? 1 : 0) +
  (dateFrom.value || dateTo.value ? 1 : 0) + (tagFilter.value ? 1 : 0))

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
    if (tagFilter.value && !tagsOf(v).includes(tagFilter.value)) return false
    return true
  })
})
// Tab counts respect every filter except the tab itself.
const tabCounts = computed(() => {
  const counts: Record<YTKind, number> = { video: 0, short: 0, live: 0, playlist: 0 }
  for (const v of filteredExceptKind.value) counts[videoKind(v)]++
  return counts
})
const filtered = computed(() => filteredExceptKind.value.filter(v => videoKind(v) === activeTab.value && (!watchLaterOnly.value || v.watch_later)))
// 章节跳转：内嵌播放器。油管已打开时用 postMessage seekTo（不重载），
// B 站 iframe 没有跳转 API，只能带 t= 重设 src。
const playerSrc = ref<Record<string, string>>({})
const playerEls = new Map<string, HTMLIFrameElement>()
function setPlayerEl (id: string, el: any) { if (el) playerEls.set(id, el as HTMLIFrameElement); else playerEls.delete(id) }
function playAt (v: YTVideo, start: number) {
  const el = playerEls.get(v.id)
  if (P === 'yt' && el?.contentWindow && playerSrc.value[v.id]) {
    el.contentWindow.postMessage(seekMessage(start), '*')
    return
  }
  playerSrc.value = { ...playerSrc.value, [v.id]: embedUrl(P, v.id, start) }
}
function closePlayer (id: string) {
  const next = { ...playerSrc.value }
  delete next[id]
  playerSrc.value = next
}
// 已看 / 稍后观看（PATCH /{yt|bili}/videos/{id}），乐观更新，失败回滚。
const watchLaterOnly = ref(false)
const watchLaterCount = computed(() => filteredExceptKind.value.filter(v => v.watch_later).length)
async function toggleWatched (v: YTVideo) {
  const prev = v.watched_at
  v.watched_at = prev ? '' : new Date().toISOString()
  try { await store.setVideoState(v.id, { watched: !prev }, P) } catch (e: any) { v.watched_at = prev; showToast(errText(e, '保存失败')) }
}
async function toggleWatchLater (v: YTVideo) {
  const prev = !!v.watch_later
  v.watch_later = !prev
  try { await store.setVideoState(v.id, { watch_later: !prev }, P) } catch (e: any) { v.watch_later = prev; showToast(errText(e, '保存失败')) }
}
// Topic tags from title + description (utils/videoTags.ts), cached per
// video text; the tag chips list every tag present, most used first.
const tagCache = new Map<string, string[]>()
function tagsOf (v: YTVideo): string[] {
  const key = `${v.id}\u0000${v.title}\u0000${v.description || ''}`
  let tags = tagCache.get(key)
  if (!tags) { tags = videoTags(v.title, v.description || ''); tagCache.set(key, tags) }
  return tags
}
const tagCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const v of videos.value) for (const tag of tagsOf(v)) counts.set(tag, (counts.get(tag) || 0) + 1)
  return [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([tag, n]) => ({ tag, n }))
})
// ① Videos are listed under their channel, in the channel list's order;
// each channel block folds (remembered per browser).
const groupedByChannel = computed(() => {
  const byChannel = new Map<string, YTVideo[]>()
  for (const v of filtered.value) {
    const list = byChannel.get(v.channel_id)
    if (list) list.push(v); else byChannel.set(v.channel_id, [v])
  }
  const order = channels.value.map(c => c.channel_id)
  for (const id of byChannel.keys()) if (!order.includes(id)) order.push(id)
  return order.filter(id => byChannel.has(id)).map(id => ({
    id, name: channelById.value.get(id)?.name || byChannel.get(id)![0].channel_name || id, videos: byChannel.get(id)!
  }))
})
const CH_COLLAPSE_KEY = `x-digest-${P}-collapsed-channel-videos`
const collapsedChannels = ref<Set<string>>(new Set())
function toggleChannelVideos (id: string) {
  const s = new Set(collapsedChannels.value)
  if (s.has(id)) s.delete(id); else s.add(id)
  collapsedChannels.value = s
  try { localStorage.setItem(CH_COLLAPSE_KEY, JSON.stringify([...s])) } catch { /* private mode */ }
}
// ⑥ Resources: the description's links, plus what the AI summary found
// (once a summary has been loaded for the video).
function resourcesOf (v: YTVideo): Array<{ label: string, url: string, note?: string }> {
  const out: Array<{ label: string, url: string, note?: string }> = descResources(v.description || '')
  const sum = summaryOf(v.id)
  if (sum?.status === 'done') {
    for (const r of sum.resources || []) {
      if (r.url && out.some(x => x.url === r.url)) continue
      out.push({ label: r.name, url: r.url, note: r.note })
    }
  }
  return out
}
watch([filtered, lang], () => {
  enqueue(filtered.value.flatMap(v => [v.title, v.description || '']))
})

function clearAll () {
  keyword.value = ''; channelFilter.value = ''; groupFilter.value = ''; tagFilter.value = ''
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
// Pending: wait for the server push over SSE; poll every 4s only when the
// stream fails.
const streaming = new Set<string>()
function schedulePoll (v: YTVideo) {
  const key = sumKey(v.id)
  if (streaming.has(key) || pollTimers.has(key)) return
  const l = lang.value
  if (!store.mock.value && followSummary(`${apiBase()}/${P}/videos/${encodeURIComponent(v.id)}/summary/stream?lang=${l}`, (sum) => { setSummary(sum); if (sum.status !== 'pending') streaming.delete(key) }, () => { streaming.delete(key); pollSummary(v) })) {
    streaming.add(key)
    return
  }
  pollSummary(v)
}
function pollSummary (v: YTVideo) {
  const key = sumKey(v.id)
  if (pollTimers.has(key)) return
  const l = lang.value
  pollTimers.set(key, setTimeout(async () => {
    pollTimers.delete(key)
    try {
      const sum = await store.getYTSummary(v.id, l, P)
      setSummary(sum)
      if (sum.status === 'pending' && l === lang.value) pollSummary(v)
    } catch (err: any) {
      if (l === lang.value) pollSummary(v)
    }
  }, 4000))
}
async function generateSummary (v: YTVideo, force = false) {
  const prev = summaryOf(v.id)
  setSummary({ ...(prev || { video_id: v.id, lang: lang.value, summary: '', key_points: [], chapters: [], source: '', transcript_lang: '', model: '', error: '' }), status: 'pending', error: '' })
  try {
    const sum = await store.startYTSummary(v.id, lang.value, force, P)
    setSummary(sum)
    if (sum.status === 'pending') schedulePoll(v)
  } catch (err: any) { failSummary(v, errText(err, t('yt.aiLoadFailed'))) }
}
async function loadSummary (v: YTVideo) {
  try {
    const sum = await store.getYTSummary(v.id, lang.value, P)
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
  if (P === 'bili') return `https://www.bilibili.com/video/${encodeURIComponent(v.id)}/?t=${Math.floor(secs)}`
  return `https://www.youtube.com/watch?v=${encodeURIComponent(v.id)}&t=${Math.floor(secs)}s`
}
// 频道 / 分组两个管理区块可折叠；折叠状态只存本机浏览器。
const SECTIONS_KEY = `x-digest-${P}-collapsed-sections`
const collapsedSections = ref<{ channels: boolean, groups: boolean }>({ channels: false, groups: false })
function toggleSection (k: 'channels' | 'groups') {
  collapsedSections.value = { ...collapsedSections.value, [k]: !collapsedSections.value[k] }
  try { localStorage.setItem(SECTIONS_KEY, JSON.stringify(collapsedSections.value)) } catch { /* private mode */ }
}
function loadSections () {
  try {
    const ch = JSON.parse(localStorage.getItem(CH_COLLAPSE_KEY) || '[]')
    if (Array.isArray(ch)) collapsedChannels.value = new Set(ch.filter((x: unknown) => typeof x === 'string'))
  } catch { /* ignore */ }
  try {
    const v = JSON.parse(localStorage.getItem(SECTIONS_KEY) || '{}')
    collapsedSections.value = { channels: v.channels === true, groups: v.groups === true }
  } catch { /* ignore */ }
}
function toggleDesc (id: string) {
  const s = new Set(expandedDesc.value)
  if (s.has(id)) s.delete(id); else s.add(id)
  expandedDesc.value = s
}
async function reload () {
  const [cs, vs, gs] = await Promise.all([store.loadYTChannels(P), store.loadYTVideos(P), store.loadYTGroups(P)])
  channels.value = cs
  videos.value = vs
  groups.value = gs
}
// 添加频道 / 添加单个视频（仅油管）：单个视频会连带登记它所属的频道（单视频模式）。
const addMode = ref<'channel' | 'video'>('channel')
function openAdd (mode: 'channel' | 'video' = 'channel') {
  addMode.value = mode
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
    if (addMode.value === 'video') {
      const res = await store.addYTVideo(input, addForm.value.group_id || null)
      await reload()
      showAdd.value = false
      const name = res.channel.name || res.channel.channel_id
      showToast(res.channel_created ? t('yt.videoAddedNewChannel', { name }) : t('yt.videoAdded', { name }))
      return
    }
    const ch = await store.addYTChannel(input, addForm.value.group_id || null, addForm.value.date_from || null, addForm.value.date_to || null, P)
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
    }, P)
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
    await store.deleteYTChannel(c.id, P)
    await reload()
    showToast(t('yt.deleted', { name: c.name || c.channel_id }))
  } catch (err: any) { showToast(errText(err, t('yt.deleteFailed'))) }
}
async function doSync (c: YTChannel, full = false) {
  if (full && !confirm(t('yt.syncChannelAllConfirm', { name: c.name || c.channel_id }))) return
  syncing.value = new Set([...syncing.value, c.id])
  try {
    const updated = await store.syncYTChannel(c.id, P, full)
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
    const res = await store.syncAllYTChannels(P)
    syncAllMsg.value = t('yt.syncAllDone', { ok: res.synced, fail: res.failed, total: res.total }) +
      (res.skipped ? t('yt.syncAllSkipped', { n: res.skipped }) : '')
    await reload()
  } catch (err: any) {
    syncAllMsg.value = errText(err, t('yt.syncAllFailed'))
  } finally { syncAllBusy.value = false }
}
async function doAddGroup () {
  if (!newGroupName.value.trim()) return
  try {
    const g = await store.addYTGroup(newGroupName.value.trim(), P)
    groups.value = [...groups.value, g]
    newGroupName.value = ''
  } catch (err: any) { showToast(errText(err, t('yt.addGroupFailed'))) }
}
async function doDeleteGroup (g: RepoGroup) {
  if (!confirm(t('yt.deleteGroupConfirm', { name: g.name }))) return
  try {
    await store.deleteYTGroup(g.id, P)
    await reload()
    showToast(t('yt.groupDeleted', { name: g.name }))
  } catch (err: any) { showToast(errText(err, t('yt.deleteGroupFailed'))) }
}

onMounted(async () => {
  loadSections()
  try {
    await reload()
  } catch (err: any) { loadError.value = errText(err, t('common.loadError')) }
  finally { loading.value = false }
})
</script>

<style scoped>
.yt-video-card.is-watched { opacity: .6; }
.yt-embed { margin: 6px 0 10px; }
.yt-embed iframe { display: block; width: 100%; aspect-ratio: 16 / 9; border: 0; border-radius: 10px; margin-bottom: 6px; background: #000; }
.yt-watch-row { display: flex; gap: 6px; margin: 8px 0; flex-wrap: wrap; }
</style>
