<template>
  <div>
    <main class="feed-col" style="max-width:760px;margin:0 auto;padding:16px;min-width:0">
      <div class="filter-row" style="margin-bottom:12px">
        <span class="updated" style="color:var(--text-muted)">{{ updatedAt ? t('index.updatedAt', { time: updatedAt }) : '' }}</span>
        <span style="flex:1" />
        <button class="btn btn-sm" type="button" @click="triggerWebhook" :disabled="webhookBusy">{{ webhookBusy ? t('index.sending') : t('index.updateNow') }}</button>
        <button class="btn btn-sm" type="button" @click="showSettings = true">{{ t('index.settings') }}</button>
      </div>
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">{{ t('common.loading') }}</p>

      <section class="filter-panel" v-if="!loading">
        <div class="filter-row" style="justify-content:space-between">
          <button class="btn btn-sm" type="button" @click="filtersCollapsed = !filtersCollapsed" :aria-expanded="!filtersCollapsed">{{ filtersCollapsed ? t('index.expandFilters') : t('index.collapseFilters') }}</button>
          <span class="active-count" v-if="filtersCollapsed && activeFilterCount">{{ t('index.selectedFilters', { n: activeFilterCount }) }}</span>
        </div>
        <div v-show="!filtersCollapsed">
          <div class="unified-bar">
            <div class="cond">
              <label class="cond-label">{{ t('index.keyword') }}</label>
              <input class="field" style="width:100%" type="search" v-model="keyword" :placeholder="t('index.keywordPh')" :aria-label="t('common.keywordSearch')" />
            </div>
            <FilterCombo :label="t('index.author')" :placeholder="t('index.authorPh')" :options="authorOptions" v-model="selectedAuthorsArr" />
            <FilterCombo :label="t('index.postTags')" :placeholder="t('index.postTagsPh')" :options="tagOptions" v-model="selectedTagsArr" />
            <FilterCombo :label="t('index.userTags')" :placeholder="t('index.userTagsPh')" :options="userTagOptions" v-model="selectedUserTagsArr" />
            <FilterCombo :label="t('index.postCats')" :placeholder="t('index.postCatsPh')" :options="catOptions" v-model="selectedCatsArr" />
            <div class="cond cond-date">
              <label class="cond-label">{{ t('index.date') }}</label>
              <div style="display:flex;gap:6px">
                <input type="date" class="field" style="flex:1;min-width:0" v-model="dateFrom" :aria-label="t('index.dateFromAria')" />
                <input type="date" class="field" style="flex:1;min-width:0" v-model="dateTo" :aria-label="t('index.dateToAria')" />
              </div>
            </div>
          </div>
          <div class="filter-row" style="margin-top:8px">
            <button class="btn btn-sm" type="button" @click="clearAll">{{ t('index.clearAll') }}</button>
            <span class="result-count">{{ t('index.resultCount', { n: filtered.length }) }}</span>
            <span v-if="translating" class="active-count">{{ t('common.translating') }}</span>
            <span class="active-count" v-if="activeFilterCount">{{ t('index.selectedFilters', { n: activeFilterCount }) }}</span>
          </div>
          <p v-if="keyword && firstSnippet" class="hint" v-html="t('index.firstHit') + firstSnippet" />
        </div>
      </section>

      <!-- 与油管页同一卡片样式：按作者分组（可折叠），左栏原文、右栏标题/时间/标签/操作，
           下方整行为资源与可折叠的 AI 摘要（含流程图）。 -->
      <section class="timeline">
        <div v-for="grp in groupedByAuthor" :key="grp.author" class="yt-channel-block">
          <button class="yt-channel-head" type="button" :aria-expanded="!collapsedAuthors.has(grp.author)" @click="toggleAuthorPosts(grp.author)">
            <svg class="yt-chevron" :class="{ 'is-collapsed': collapsedAuthors.has(grp.author) }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            <img v-if="postAvatar(grp.posts[0])" :src="postAvatar(grp.posts[0])" :alt="grp.name" class="x-head-avatar" @error="onAvatarError($event, grp.posts[0])" />
            <span v-else class="x-head-avatar" :style="{ background: avatarColor(grp.name) }">{{ avatarInitial(grp.name) }}</span>
            <span class="yt-channel-name">{{ grp.name }}</span>
            <a class="x-head-handle" :href="`https://x.com/${grp.author}`" target="_blank" rel="noopener" @click.stop>@{{ grp.author }}</a>
            <span class="yt-section-count">（{{ grp.posts.length }}）</span>
          </button>
          <template v-if="!collapsedAuthors.has(grp.author)">
          <article v-for="post in grp.posts" :key="post.id" class="post-card yt-video-card">
            <div class="yt-left">
              <div class="x-orig" :class="{ 'is-missing': !post.text }">
                <div class="x-orig-label">𝕏 原文</div>
                <p v-if="post.text" class="x-orig-text" :class="{ 'yt-desc-open': expandedOriginal.has(post.id) }" :id="`original-${post.id}`"><template v-for="(part, pi) in linkParts(post.text)" :key="pi"><a v-if="part.href" class="yt-desc-link" :href="part.href" :title="part.href" target="_blank" rel="noopener noreferrer">{{ part.text.replace(/^https?:\/\/(www\.)?/, '') }}</a><span v-else v-html="highlightHtml(part.text, plainKeyword)" /></template></p>
                <p v-else class="x-orig-text">原文暂未收录，<a :href="post.url" target="_blank" rel="noopener" style="color:var(--accent)">点击查看原推文</a>。</p>
              </div>
              <button v-if="(post.text || '').length > 120" class="yt-desc-more" type="button" :aria-expanded="expandedOriginal.has(post.id)" :aria-controls="`original-${post.id}`" @click="toggleOriginal(post.id)">{{ expandedOriginal.has(post.id) ? '收起' : '展开' }}</button>
            </div>
            <div class="yt-info">
              <p class="yt-meta"><a class="yt-meta-link" :href="post.url" target="_blank" rel="noopener">{{ shanghaiTime(post.published_at) }}（{{ relativeTime(post.published_at, lang.value) }}）</a><span v-if="post.industry"> · {{ post.industry }}</span><span v-if="post.tech"> · {{ post.tech }}</span></p>
              <div class="yt-tags">
                <button v-for="tg in (post.tags || []).filter(tag => tag !== '复刻' || canReplicatePost(post)).slice(0, 8)" :key="tg" type="button" class="chip yt-tag yt-topic" :class="{ active: selectedTags.has(tg) }" @click="selectTagOnly(tg)">#{{ tg }}</button>
                <span v-for="cid in catsOf(post.id)" :key="cid" class="chip active yt-tag">{{ categoryName(cid) }}</span>
              </div>
              <div v-if="post.note" style="margin-top:6px"><span class="note-pill">{{ post.note }}</span></div>
              <div class="yt-actions">
                <button class="btn btn-sm" type="button" @click="catMenuPost = catMenuPost === post.id ? null : post.id">{{ t('index.category') }}</button>
                <button v-if="canReplicatePost(post)" class="btn btn-sm" :class="{ 'btn-primary': replicatedIds.has(post.id) }" type="button" @click="replicate(post)">
                  {{ replicatedIds.has(post.id) ? t('index.replicated') : t('index.replicate') }}
                </button>
                <NuxtLink v-if="replicatedIds.has(post.id)" to="/fuke" class="btn btn-sm">{{ t('index.goFuke') }}</NuxtLink>
              </div>
              <div v-if="catMenuPost === post.id" class="cat-menu" style="margin-top:8px">
                <label v-for="c in categories.categories" :key="c.id" class="cat-menu-item" style="display:flex;gap:6px">
                  <input type="checkbox" :checked="catsOf(post.id).includes(c.id)" @change="togglePostCat(post.id, c.id, ($event.target as HTMLInputElement).checked)" />
                  <span>{{ c.name }}</span>
                </label>
                <div style="display:flex;gap:6px;margin-top:6px">
                  <input class="field" v-model="newCatName" :placeholder="t('index.newCatPh')" maxlength="20" />
                  <button class="btn btn-sm btn-primary" type="button" @click="createCat(post.id)">{{ t('index.newCat') }}</button>
                </div>
              </div>
            </div>
            <div v-if="resourcesOf(post).length" class="yt-resources">
              <div class="yt-res-title">提及的资源</div>
              <ul class="yt-res-list">
                <li v-for="(r, ri) in resourcesOf(post)" :key="'r' + ri">
                  <span v-if="r.label" class="yt-res-label">{{ r.label }}</span>
                  <a v-if="r.url" class="yt-desc-link" :href="r.url" target="_blank" rel="noopener noreferrer">{{ r.url.replace(/^https?:\/\/(www\.)?/, '') }}</a>
                  <span v-if="r.note" class="yt-res-note">{{ r.note }}</span>
                </li>
              </ul>
            </div>
            <!-- AI 摘要：点击时才调中转站 API 生成并入库（同油管），之后直接读库；可复刻帖子的流程图也在这里 -->
            <button class="yt-summary-toggle" type="button" :aria-expanded="openSummary.has(post.id)" @click="toggleSummary(post)">
              <span>✨ {{ openSummary.has(post.id) ? '收起 AI 摘要' : 'AI 摘要' }}<template v-if="hasFlow(post)"> · {{ t('index.stepsSummary', { n: post.steps!.length }) }}</template></span>
              <svg class="yt-chevron" :class="{ 'is-collapsed': !openSummary.has(post.id) }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            </button>
            <section v-if="openSummary.has(post.id)" class="yt-summary" aria-label="AI 摘要">
              <template v-for="sum in [summaryOf(post.id)]" :key="'s' + post.id">
                <p v-if="!sum" class="yt-sum-note">{{ t('common.loading') }}</p>
                <p v-else-if="sum.status === 'pending' || sum.status === 'none'" class="yt-sum-note"><span class="yt-spinner" aria-hidden="true" />正在调用 AI 生成摘要，通常需要十几秒…</p>
                <div v-else-if="sum.status === 'failed'" class="yt-sum-note">
                  <p class="yt-sum-error">生成失败：{{ sum.error }}</p>
                  <button class="btn btn-sm" type="button" @click="generateSummary(post, true)">重试</button>
                </div>
                <template v-if="sum && sum.summary && sum.status !== 'pending'">
                  <h3 class="yt-sum-h">全文摘要</h3>
                  <p v-for="(para, pi) in sum.summary.split(/\n\s*\n/).filter(Boolean)" :key="'p' + pi" class="yt-sum-para">{{ para }}</p>
                  <template v-if="sum.key_points.length">
                    <h3 class="yt-sum-h">要点</h3>
                    <ul class="yt-sum-points"><li v-for="(k, ki) in sum.key_points" :key="'k' + ki">{{ k }}</li></ul>
                  </template>
                  <div class="yt-sum-foot">
                    <span>{{ sum.source === 'description' ? '原文未收录，只根据标题和已有信息生成，内容可能不完整' : '根据推文原文生成' }}<template v-if="sum.updated_at"> · 生成于 {{ sum.updated_at }}</template></span>
                    <button v-if="sum.status === 'done'" class="btn btn-sm" type="button" @click="generateSummary(post, true)">重新生成</button>
                  </div>
                </template>
              </template>
          <div v-if="hasFlow(post)" class="steps" style="margin-top:12px">
            <h3 class="yt-sum-h">{{ t('index.stepsSummary', { n: post.steps!.length }) }}</h3>
            <div class="flow-chart">
              <div v-for="(step, i) in post.steps" :key="i" class="flow-step" :class="{ 'is-last': i === post.steps!.length - 1 }">
                <div class="flow-rail" aria-hidden="true"><span class="flow-node" /><span v-if="i !== post.steps!.length - 1" class="flow-line" /></div>
                <div class="flow-card">
                  <div class="flow-text" v-html="highlightHtml(`${i + 1}. ${tr(step.text)}`, plainKeyword)" />
                  <div class="flow-meta" v-if="step.tools || step.tech || step.prep">
                    <span v-if="step.tools" class="flow-chip"><span class="flow-chip-label">{{ t('index.tool') }}</span>{{ step.tools }}</span>
                    <span v-if="step.tech" class="flow-chip"><span class="flow-chip-label">{{ t('index.tech') }}</span>{{ step.tech }}</span>
                    <span v-if="step.prep" class="flow-chip"><span class="flow-chip-label">{{ t('index.prep') }}</span>{{ step.prep }}</span>
                  </div>
                  <div v-if="(step.substeps || []).length" style="margin-top:6px">
                    <button class="btn btn-sm" type="button" @click="toggleSubsteps(post.id, i)">{{ substepsOpen.has(`${post.id}:${i}`) ? t('index.collapseSubsteps') : t('index.expandSubsteps', { n: step.substeps!.length }) }}</button>
                    <ol v-if="substepsOpen.has(`${post.id}:${i}`)">
                      <li v-for="(sub, j) in step.substeps!.slice(0, 5)" :key="j" :title="sub.text">{{ `${i + 1}.${j + 1} ` }}{{ trunc40(tr(sub.text)) }}<span v-if="sub.source" style="color:var(--text-muted)"> — {{ sub.source }}</span></li>
                    </ol>
                  </div>
                </div>
              </div>
            </div>
          </div>
            </section>
          </article>
          </template>
        </div>
        <p v-if="!loading && !filtered.length" class="empty">{{ t('index.noPosts') }}</p>
      </section>
    </main>

    <!-- Settings modal -->
    <div v-if="showSettings" class="modal-backdrop" @click.self="showSettings = false">
      <div class="modal-card" role="dialog" aria-modal="true" :aria-label="t('settings.title')">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px">
          <h2 style="margin:0">{{ t('settings.title') }}</h2>
          <button class="btn btn-sm" type="button" @click="showSettings = false" :aria-label="t('settings.close')">✕</button>
        </div>
        <label>{{ t('settings.theme') }}</label>
        <div style="display:flex;gap:8px">
          <button class="btn" :class="{ 'btn-primary': theme === 'dark' }" type="button" @click="setTheme('dark')">{{ t('settings.dark') }}</button>
          <button class="btn" :class="{ 'btn-primary': theme === 'light' }" type="button" @click="setTheme('light')">{{ t('settings.light') }}</button>
        </div>
        <label>{{ t('settings.fontSize') }}</label>
        <div style="display:flex;gap:6px;flex-wrap:wrap">
          <button v-for="s in ['13px', '15px', '17px', '19px']" :key="s" class="btn btn-sm" :class="{ 'btn-primary': fontSize === s }" type="button" @click="setFontSize(s)">{{ s }}</button>
        </div>
        <label>{{ t('settings.font') }}</label>
        <select class="field" :value="fontFamily" @change="setFontFamily(($event.target as HTMLSelectElement).value)">
          <option value="">{{ t('settings.fontSystem') }}</option>
          <option value="'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif">{{ t('settings.fontPingFang') }}</option>
          <option value="Georgia,serif">{{ t('settings.fontSerif') }}</option>
          <option value="ui-monospace,Menlo,monospace">{{ t('settings.fontMono') }}</option>
        </select>

        <div class="settings-section">
          <h3>{{ t('settings.watchlist') }}</h3>
          <div style="display:flex;gap:6px;flex-wrap:wrap">
            <input class="field" v-model="newGroupName" :placeholder="t('settings.newGroupPh')" style="flex:1;min-width:140px" />
            <button class="btn btn-sm btn-primary" type="button" @click="doAddGroup">{{ t('settings.addGroup') }}</button>
          </div>
          <div v-for="g in watchlist.groups" :key="g.id" class="watch-row">
            <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">({{ g.id }})</span>
            <button v-if="!g.fixed" class="btn btn-sm btn-danger" type="button" @click="doDeleteGroup(g.id)">{{ t('settings.deleteGroup') }}</button>
          </div>
          <div v-for="a in watchlist.accounts" :key="a.username" class="watch-row">
            <input :value="a.name" :placeholder="t('settings.displayNamePh')" @change="doPatchAccount(a.username, { name: ($event.target as HTMLInputElement).value })" />
            <span>@{{ a.username }}</span>
            <select :value="a.group" @change="doPatchAccount(a.username, { group: ($event.target as HTMLSelectElement).value })">
              <option v-for="g in watchlist.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
            <input :value="(a.tags || []).join('，')" :placeholder="t('settings.userTagsPh')" @change="doPatchAccount(a.username, { tags: ($event.target as HTMLInputElement).value.split(/[,，]/).map(s => s.trim()).filter(Boolean) })" />
            <button class="btn btn-sm btn-danger" type="button" @click="doDeleteAccount(a.username)">{{ t('settings.deleteAuthor') }}</button>
          </div>
          <div class="watch-row">
            <input class="field" v-model="newAccount.username" :placeholder="t('settings.usernamePh')" />
            <input class="field" v-model="newAccount.name" :placeholder="t('settings.displayNamePh')" />
            <select v-model="newAccount.group">
              <option v-for="g in watchlist.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
            <button class="btn btn-sm btn-primary" type="button" @click="doAddAccount">{{ t('settings.addAuthor') }}</button>
          </div>
        </div>

        <div class="settings-section">
          <h3>{{ t('settings.userTagSection') }}</h3>
          <p style="color:var(--text-muted)">{{ t('settings.vocabHintPrefix') }}<code>标签:xxx</code>{{ t('settings.vocabHintSuffix') }}</p>
          <div class="chips">
            <button v-for="t in userTagVocab" :key="t" type="button" class="chip" @click="removeVocab(t)">{{ t }} ×</button>
          </div>
          <div style="display:flex;gap:6px;margin-top:8px">
            <input class="field" v-model="newVocab" :placeholder="t('settings.newUserTagPh')" style="flex:1" />
            <button class="btn btn-sm btn-primary" type="button" @click="addVocab">{{ t('settings.add') }}</button>
          </div>
          <label>{{ t('settings.overrideByAuthor') }}</label>
          <div v-for="a in watchlist.accounts" :key="'ov-' + a.username" class="watch-row">
            <span style="min-width:120px">{{ a.name }} @{{ a.username }}</span>
            <input class="field" :value="(userTagOverrides[a.username] || []).join('，')" style="flex:1"
              @change="doSetUserTags(a.username, ($event.target as HTMLInputElement).value.split(/[,，]/).map(s => s.trim()).filter(Boolean))" />
          </div>
        </div>

        <div class="settings-section">
          <h3>{{ t('settings.webhook') }}</h3>
          <label>{{ t('settings.webhookUrl') }}</label>
          <input class="field" v-model="webhookUrlDraft" placeholder="https://…" />
          <label>{{ t('settings.webhookKey') }}</label>
          <input class="field" v-model="webhookKeyDraft" :placeholder="t('settings.webhookKeyPh')" type="password" />
          <p v-if="webhookMsg" style="color:var(--text-muted)">{{ webhookMsg }}</p>
          <div class="modal-actions" style="justify-content:flex-start">
            <button class="btn btn-sm btn-primary" type="button" @click="saveWebhookDraft">{{ t('settings.save') }}</button>
            <button class="btn btn-sm" type="button" @click="clearWebhookDraft">{{ t('settings.clear') }}</button>
            <button class="btn btn-sm" type="button" @click="triggerWebhook">{{ t('settings.triggerOnce') }}</button>
          </div>
        </div>

        <div class="modal-actions">
          <button class="btn btn-primary" type="button" @click="showSettings = false">{{ t('settings.done') }}</button>
        </div>
      </div>
    </div>
    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { Post, Watchlist, Categories, YTSummary } from '~/composables/types'
import { LS, useSettings } from '~/composables/useSettings'
import { canReplicatePost } from '~/utils/x-post.mjs'
import { avatarUrl, avatarColor, avatarInitial, relativeTime, shanghaiTime, shanghaiDayBound, highlightHtml, firstHitSnippet, shouldShowFlowchart, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'
import { linkParts, descResources } from '~/utils/videoTags'

const store = useStore()
const settings = useSettings()
const { t, lang } = useLang()
const { tr, enqueue, translating } = useTranslate()
const { theme, fontSize, fontFamily, setTheme, setFontSize, setFontFamily, saveWebhook, clearWebhook, postWebhook } = settings

const loading = ref(true)
const loadError = ref('')
const posts = ref<Post[]>([])
const updatedAt = ref('')
const watchlist = ref<Watchlist>({ groups: [], accounts: [] })
const categories = ref<Categories>({ categories: [], assignments: {} })
const userTagVocab = ref<string[]>([])
const userTagOverrides = ref<Record<string, string[]>>({})
const replicaCount = ref(0)
const replicatedIds = ref<Set<string>>(new Set())

const keyword = ref('')
const dateFrom = ref('')
const dateTo = ref('')
const selectedAuthors = ref<Set<string>>(new Set())
const selectedTags = ref<Set<string>>(new Set())
const selectedUserTags = ref<Set<string>>(new Set())
const selectedCats = ref<Set<string>>(new Set())
const filtersCollapsed = ref(false)
const expandedOriginal = ref<Set<string>>(new Set())
const failedAvatars = ref<Set<string>>(new Set())
const substepsOpen = ref<Set<string>>(new Set())
const catMenuPost = ref<string | null>(null)
const newCatName = ref('')
const showSettings = ref(false)
useScrollLock(showSettings)
const toast = ref('')
const webhookBusy = ref(false)
const webhookMsg = ref('')
const webhookUrlDraft = ref('')
const webhookKeyDraft = ref('')
const newGroupName = ref('')
const newVocab = ref('')
const newAccount = ref({ username: '', name: '', group: 'other' })

let authorsInitialised = false
function showToast (msg: string) { toast.value = msg; setTimeout(() => { toast.value = '' }, 3000) }
function persistSelection () {
  if (!import.meta.client) return
  try {
    localStorage.setItem(LS.selectedAuthors, JSON.stringify([...selectedAuthors.value]))
    localStorage.setItem(LS.selectedCats, JSON.stringify([...selectedCats.value]))
  } catch { /* ignore */ }
}
function accountOf (username: string) { return watchlist.value.accounts.find(a => a.username === username) }
function authorName (u: string) { return accountOf(u)?.name || u }
const unlistedAuthors = computed(() => {
  const known = new Set(watchlist.value.accounts.map(a => a.username))
  return [...new Set(posts.value.map(p => p.author))].filter(a => !known.has(a))
})
function userTagsOf (username: string): string[] {
  // SPEC: user tags = watchlist account tags, with per-author overrides winning.
  if (userTagOverrides.value[username]) return userTagOverrides.value[username]
  return accountOf(username)?.tags || []
}
const userTagOptions = computed(() => {
  const s = new Set<string>(userTagVocab.value)
  for (const a of watchlist.value.accounts) for (const t of userTagsOf(a.username)) s.add(t)
  return [...s]
})
const allTags = computed(() => {
  const counts = new Map<string, number>()
  for (const p of posts.value) for (const t of p.tags || []) counts.set(t, (counts.get(t) || 0) + 1)
  return [...counts.entries()].map(([tag, n]) => ({ tag, n })).sort((a, b) => b.n - a.n)
})
const catsOf = (postId: string) => categories.value.assignments[postId] || []
const categoryName = (id: string) => categories.value.categories.find(c => c.id === id)?.name || id
const catCounts = computed(() => {
  const counts: Record<string, number> = { __uncat: 0 }
  for (const p of baseFiltered.value) {
    const cats = catsOf(p.id)
    if (!cats.length) counts.__uncat++
    for (const c of cats) counts[c] = (counts[c] || 0) + 1
  }
  return counts
})
// Filters other than the author chips (used for author-chip consistency).
const baseFiltered = computed(() => posts.value.filter(p => passesNonAuthor(p)))
function passesNonAuthor (p: Post): boolean {
  if (selectedUserTags.value.size && !userTagsOf(p.author).some(t => selectedUserTags.value.has(t))) return false
  if (!matchesCats(p)) return false
  if (!matchesDate(p)) return false
  if (!matchesTags(p)) return false
  if (!matchesKeyword(p)) return false
  return true
}
function matchesCats (p: Post): boolean {
  if (!selectedCats.value.size) return true
  const cats = catsOf(p.id)
  if (selectedCats.value.has('__uncat') && !cats.length) return true
  return cats.some(c => selectedCats.value.has(c))
}
function matchesDate (p: Post): boolean {
  const from = shanghaiDayBound(dateFrom.value, false)
  const to = shanghaiDayBound(dateTo.value, true)
  if (from == null && to == null) return true
  const ts = Date.parse(p.published_at || '')
  if (Number.isNaN(ts)) return false
  if (from != null && ts < from) return false
  if (to != null && ts > to) return false
  return true
}
function matchesTags (p: Post): boolean {
  if (!selectedTags.value.size) return true
  return (p.tags || []).some(t => selectedTags.value.has(t))
}
const isUserTagQuery = computed(() => /^(?:标签[:：]|tag:)/i.test(keyword.value.trim()))
const plainKeyword = computed(() => isUserTagQuery.value ? keyword.value.trim().replace(/^(?:标签[:：]|tag:)/i, '').trim() : keyword.value.trim())
function matchesKeyword (p: Post): boolean {
  const q = keyword.value.trim().toLowerCase()
  if (!q) return true
  if (isUserTagQuery.value) {
    const needle = plainKeyword.value.toLowerCase()
    return userTagsOf(p.author).some(t => t.toLowerCase().includes(needle))
  }
  const stepTexts = (p.steps || []).flatMap(s => [s.text, s.tools, s.tech, s.prep, ...(s.substeps || []).map(x => x.text)].filter(Boolean) as string[])
  const hay = [p.title, p.text || '', ...(p.links || []), p.industry || '', p.tech || '', p.author, '@' + p.author, p.author_name, accountOf(p.author)?.bio || '', ...userTagsOf(p.author), ...(p.tags || []), ...catsOf(p.id).map(categoryName), shanghaiTime(p.published_at), ...stepTexts].join('\n').toLowerCase()
  // Tag field: strip leading # before matching (SPEC).
  return hay.includes(q.replace(/^#+/, ''))
}
const filtered = computed(() => {
  // SPEC: empty author selection = EMPTY list, not "no filter".
  return posts.value.filter(p => selectedAuthors.value.has(p.author) && passesNonAuthor(p))
})
const activeFilterCount = computed(() =>
  selectedTags.value.size + selectedUserTags.value.size + selectedCats.value.size +
  (keyword.value.trim() ? 1 : 0) + (dateFrom.value || dateTo.value ? 1 : 0))
const firstSnippet = computed(() => {
  if (!keyword.value.trim() || !filtered.value.length) return ''
  return firstHitSnippet(filtered.value[0], plainKeyword.value || keyword.value.trim())
})
function selectTagOnly (t: string) { selectedTags.value = new Set([t]) }
// 下拉联想框用数组 v-model，内部筛选状态仍是 Set；桥接读写并保持原有的持久化。
function setBridge (refSet: Ref<Set<string>>, persist = false) {
  return computed<string[]>({
    get: () => [...refSet.value],
    set: (v) => { refSet.value = new Set(v); if (persist) persistSelection() }
  })
}
const selectedAuthorsArr = setBridge(selectedAuthors, true)
const selectedTagsArr = setBridge(selectedTags)
const selectedUserTagsArr = setBridge(selectedUserTags)
const selectedCatsArr = setBridge(selectedCats, true)
// 各条件框的下拉选项（值多的字段靠输入联想过滤，见 FilterCombo）。
const authorOptions = computed(() => {
  const listed = watchlist.value.accounts.map(a => ({ value: a.username, label: `${a.name} @${a.username}` }))
  const unlisted = unlistedAuthors.value.map(u => ({ value: u, label: `${authorName(u)} @${u}` }))
  return [...listed, ...unlisted]
})
const tagOptions = computed(() => allTags.value.map(t => ({ value: t.tag, label: t.tag, count: t.n })))
const catOptions = computed(() => [
  ...categories.value.categories.map(c => ({ value: c.id, label: c.name, count: catCounts[c.id] || 0 })),
  { value: '__uncat', label: t('index.uncategorized'), count: catCounts.__uncat || 0 }
])
function clearAll () {
  keyword.value = ''; dateFrom.value = ''; dateTo.value = ''
  selectedTags.value = new Set(); selectedUserTags.value = new Set(); selectedCats.value = new Set()
  selectedAuthors.value = new Set(watchlist.value.accounts.filter(a => a.group === 'special').map(a => a.username))
  persistSelection()
}
function toggleOriginal (id: string) {
  const next = new Set(expandedOriginal.value)
  next.has(id) ? next.delete(id) : next.add(id)
  expandedOriginal.value = next
}
// Enqueue the currently filtered posts' display texts for translation
// (debounced; the queue flushes in chunks of 20). Translation runs toward
// the current UI language; filtering always uses the original text —
// only display goes through tr().
let enqueueTimer: any = null
function enqueueVisible () {
  const texts: string[] = []
  for (const p of filtered.value) {
    texts.push(p.title)
    for (const s of p.steps || []) {
      texts.push(s.text)
      for (const sub of s.substeps || []) texts.push(sub.text)
    }
  }
  enqueue(texts)
}
watch([filtered, lang], () => {
  clearTimeout(enqueueTimer)
  enqueueTimer = setTimeout(enqueueVisible, 300)
})
function toggleSubsteps (postId: string, i: number) {
  const k = `${postId}:${i}`
  substepsOpen.value.has(k) ? substepsOpen.value.delete(k) : substepsOpen.value.add(k)
  substepsOpen.value = new Set(substepsOpen.value)
}
function postAvatar (post: Post) {
  const src = avatarUrl(post.author_avatar || accountOf(post.author)?.avatar)
  return failedAvatars.value.has(src) ? '' : src
}
function onAvatarError (e: Event, post: Post) {
  failedAvatars.value = new Set([...failedAvatars.value, postAvatar(post)])
}
// ---- 油管样式：按作者分组（顺序同关注列表）、折叠状态存本机 ----
const groupedByAuthor = computed(() => {
  const byAuthor = new Map<string, Post[]>()
  for (const p of filtered.value) {
    const list = byAuthor.get(p.author)
    if (list) list.push(p); else byAuthor.set(p.author, [p])
  }
  const order = watchlist.value.accounts.map(a => a.username)
  for (const a of byAuthor.keys()) if (!order.includes(a)) order.push(a)
  return order.filter(a => byAuthor.has(a)).map((a) => {
    const list = byAuthor.get(a)!
    return { author: a, name: accountOf(a)?.name || list[0].author_name || a, avatar: list[0].author_avatar || accountOf(a)?.avatar || '', posts: list }
  })
})
const AUTHOR_COLLAPSE_KEY = 'x-digest-x-collapsed-authors'
const collapsedAuthors = ref<Set<string>>(new Set())
function toggleAuthorPosts (author: string) {
  const s = new Set(collapsedAuthors.value)
  if (s.has(author)) s.delete(author); else s.add(author)
  collapsedAuthors.value = s
  try { localStorage.setItem(AUTHOR_COLLAPSE_KEY, JSON.stringify([...s])) } catch { /* private mode */ }
}
function hasFlow (p: Post) { return canReplicatePost(p) && shouldShowFlowchart(p) && !!(p.steps || []).length }
// ---- AI 摘要（与油管同一逻辑）：首次展开时读库，没有就发起生成，生成中每 4 秒轮询 ----
const openSummary = ref<Set<string>>(new Set())
const summaries = ref<Record<string, YTSummary>>({})
const pollTimers = new Map<string, ReturnType<typeof setTimeout>>()
function sumKey (id: string) { return `${id}|${lang.value}` }
function summaryOf (id: string): YTSummary | undefined { return summaries.value[sumKey(id)] }
function setSummary (sum: YTSummary) { summaries.value = { ...summaries.value, [`${sum.video_id}|${sum.lang}`]: sum } }
function failSummary (p: Post, msg: string) {
  const prev = summaryOf(p.id)
  setSummary({ video_id: p.id, lang: lang.value, status: 'failed', summary: prev?.summary || '', key_points: prev?.key_points || [], chapters: [], resources: prev?.resources || [], source: prev?.source || '', transcript_lang: '', model: '', error: msg })
}
function schedulePoll (p: Post) {
  const key = sumKey(p.id)
  if (pollTimers.has(key)) return
  const l = lang.value
  pollTimers.set(key, setTimeout(async () => {
    pollTimers.delete(key)
    try {
      const sum = await store.getPostSummary(p.id, l)
      setSummary(sum)
      if (sum.status === 'pending' && l === lang.value) schedulePoll(p)
    } catch { if (l === lang.value) schedulePoll(p) }
  }, 4000))
}
async function generateSummary (p: Post, force = false) {
  const prev = summaryOf(p.id)
  setSummary({ ...(prev || { video_id: p.id, lang: lang.value, summary: '', key_points: [], chapters: [], resources: [], source: '', transcript_lang: '', model: '', error: '' }), status: 'pending', error: '' })
  try {
    const sum = await store.startPostSummary(p.id, lang.value, { text: p.text || '', links: p.links || [] }, force)
    setSummary(sum)
    if (sum.status === 'pending') schedulePoll(p)
  } catch (err: any) { failSummary(p, err?.message || '读取摘要失败') }
}
async function loadSummary (p: Post) {
  try {
    const sum = await store.getPostSummary(p.id, lang.value)
    if (sum.status === 'none') return generateSummary(p)
    setSummary(sum)
    if (sum.status === 'pending') schedulePoll(p)
  } catch (err: any) { failSummary(p, err?.message || '读取摘要失败') }
}
function toggleSummary (p: Post) {
  const s = new Set(openSummary.value)
  if (s.has(p.id)) { s.delete(p.id); openSummary.value = s; return }
  s.add(p.id); openSummary.value = s
  const cur = summaryOf(p.id)
  if (!cur || cur.status === 'none') loadSummary(p)
}
onUnmounted(() => { for (const tm of pollTimers.values()) clearTimeout(tm); pollTimers.clear() })
// 资源：解析出的 links 在前，原文里其余链接补在后；生成过 AI 摘要后补上它识别出的资源。
function resourcesOf (p: Post): Array<{ label: string, url: string, note?: string }> {
  const out: Array<{ label: string, url: string, note?: string }> = (p.links || []).map(url => ({ label: '', url }))
  for (const r of descResources(p.text || '')) {
    if (!out.some(x => x.url === r.url)) out.push(r)
  }
  const sum = summaryOf(p.id)
  if (sum?.status === 'done') {
    for (const r of sum.resources || []) {
      if (r.url && out.some(x => x.url === r.url)) continue
      out.push({ label: r.name, url: r.url, note: r.note })
    }
  }
  return out
}
async function replicate (post: Post) {
  if (!canReplicatePost(post) || replicatedIds.value.has(post.id)) return
  try {
    const { replica } = await store.createReplica(post)
    replicatedIds.value = new Set([...replicatedIds.value, post.id])
    replicaCount.value++
    showToast(replica ? t('index.replicateDone') : t('index.replicatedAlready'))
  } catch (err: any) { showToast(err?.message || t('index.replicateFailed')) }
}
async function togglePostCat (postId: string, catId: string, checked: boolean) {
  const cur = new Set(catsOf(postId))
  checked ? cur.add(catId) : cur.delete(catId)
  await store.setPostCategories(postId, [...cur])
  await refreshCategories()
}
async function createCat (postId: string) {
  if (!newCatName.value.trim()) return
  await store.addCategory(newCatName.value.trim())
  newCatName.value = ''
  await refreshCategories()
}
async function refreshCategories () { categories.value = await store.loadCategories() }
async function refreshWatchlist () {
  watchlist.value = await store.loadWatchlist()
  const ut = await store.loadUserTags()
  userTagVocab.value = ut.vocabulary || []
  userTagOverrides.value = ut.overrides || {}
  if (!authorsInitialised) {
    authorsInitialised = true
    // Default = special-group authors; a stored selection (even empty) wins.
    let stored: string[] | null = null
    if (import.meta.client) {
      try { const raw = localStorage.getItem(LS.selectedAuthors); stored = raw ? JSON.parse(raw) : null } catch { stored = null }
    }
    selectedAuthors.value = stored !== null
      ? new Set(stored)
      : new Set(watchlist.value.accounts.filter(a => a.group === 'special').map(a => a.username))
    let storedCats: string[] = []
    if (import.meta.client) {
      try { storedCats = JSON.parse(localStorage.getItem(LS.selectedCats) || '[]') } catch { storedCats = [] }
    }
    selectedCats.value = new Set(storedCats)
  }
}
async function doAddGroup () {
  if (!newGroupName.value.trim()) { showToast(t('settings.groupNameEmpty')); return }
  try {
    await store.addGroup(newGroupName.value.trim()); newGroupName.value = ''; await refreshWatchlist()
    showToast(t('settings.groupAdded'))
  } catch (e: any) {
    showToast(e?.message || t('settings.groupAddFailed'))
  }
}
async function doDeleteGroup (id: string) {
  if (!confirm(t('settings.deleteGroupConfirm'))) return
  await store.deleteGroup(id); await refreshWatchlist()
}
async function doAddAccount () {
  if (!newAccount.value.username.trim() || !newAccount.value.name.trim()) { showToast(t('settings.accountEmpty')); return }
  await store.addAccount({ ...newAccount.value, avatar: '', tags: [], bio: '' })
  newAccount.value = { username: '', name: '', group: 'other' }; await refreshWatchlist()
}
async function doPatchAccount (username: string, patch: any) { await store.patchAccount(username, patch); await refreshWatchlist() }
async function doDeleteAccount (username: string) {
  if (!confirm(t('settings.deleteAuthorConfirm', { username }))) return
  await store.deleteAccount(username); await refreshWatchlist()
}
async function doSetUserTags (username: string, tags: string[]) { await store.setUserTags(username, tags); await refreshWatchlist() }
function addVocab () {
  if (!newVocab.value.trim() || userTagVocab.value.includes(newVocab.value.trim())) return
  userTagVocab.value = [...userTagVocab.value, newVocab.value.trim()]; newVocab.value = ''
}
function removeVocab (t: string) { userTagVocab.value = userTagVocab.value.filter(x => x !== t) }
function saveWebhookDraft () { saveWebhook(webhookUrlDraft.value.trim(), webhookKeyDraft.value); webhookMsg.value = t('settings.webhookSaved') }
function clearWebhookDraft () { clearWebhook(); webhookUrlDraft.value = ''; webhookKeyDraft.value = ''; webhookMsg.value = t('settings.webhookCleared') }
async function triggerWebhook () {
  if (webhookBusy.value) return
  webhookBusy.value = true
  try {
    const r = await postWebhook()
    showToast(t('index.webhookSent', { detail: `${r.via}${r.status ? ` · ${r.status}` : ''}` }))
    webhookMsg.value = t('index.lastSent', { detail: `${r.via}${r.status ? ` · HTTP ${r.status}` : ''}` })
  } catch (err: any) { showToast(err?.message || t('index.webhookFailed')); webhookMsg.value = err?.message || t('index.sendFailed') }
  finally { webhookBusy.value = false }
}

onMounted(async () => {
  settings.load()
  try {
    const v = JSON.parse(localStorage.getItem(AUTHOR_COLLAPSE_KEY) || '[]')
    if (Array.isArray(v)) collapsedAuthors.value = new Set(v.filter((x: unknown) => typeof x === 'string'))
  } catch { /* ignore */ }
  webhookUrlDraft.value = settings.webhookUrl.value
  webhookKeyDraft.value = settings.webhookKey.value
  try {
    const [p, w, c, reps] = await Promise.all([
      store.loadPosts(), store.loadWatchlist(), store.loadCategories(), store.loadReplicas('time').catch(() => [] as any[])
    ])
    posts.value = p.posts; updatedAt.value = p.updated_at
    watchlist.value = w; categories.value = c
    const ut = await store.loadUserTags()
    userTagVocab.value = ut.vocabulary || []; userTagOverrides.value = ut.overrides || {}
    replicatedIds.value = new Set((reps as any[]).map(r => r.post_id))
    replicaCount.value = (reps as any[]).length
    await refreshWatchlist()
  } catch (err: any) { loadError.value = err?.message || t('common.loadError') }
  finally { loading.value = false }
})
</script>
