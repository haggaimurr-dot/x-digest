<template>
  <div>
    <main class="feed-col" style="max-width:600px;margin:0 auto;padding:16px;min-width:0">
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

      <section class="timeline">
        <article v-for="post in filtered" :key="post.id" class="post-card">
          <img v-if="avatarUrl(post.author_avatar)" :src="avatarUrl(post.author_avatar)" :alt="post.author_name" class="avatar"
            style="width:40px;height:40px;border-radius:50%;object-fit:cover" @error="onAvatarError($event, post)" />
          <span v-else class="avatar" :style="{ background: avatarColor(post.author_name), width: '40px', height: '40px', borderRadius: '50%', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', color: '#fff' }">{{ avatarInitial(post.author_name) }}</span>
          <div class="post-body">
          <div class="post-meta">
            <div style="min-width:0">
              <div><strong>{{ post.author_name }}</strong> <a :href="`https://x.com/${post.author}`" target="_blank" rel="noopener">@{{ post.author }}</a></div>
              <div style="color:var(--text-muted);font-size:13px">{{ relativeTime(post.published_at, lang.value) }} · {{ shanghaiTime(post.published_at) }}<span v-if="post.industry"> · {{ post.industry }}</span><span v-if="post.tech"> · {{ post.tech }}</span></div>
            </div>
          </div>
          <h2 class="post-title"><a :href="post.url" target="_blank" rel="noopener" v-html="highlightHtml(tr(post.title), plainKeyword)" /></h2>
          <div v-if="post.text" class="post-original">
            <p class="post-summary" :id="`original-${post.id}`" v-html="highlightHtml(originalShown(post), plainKeyword)" />
            <button v-if="post.text.length > 280" class="btn btn-sm" type="button" :aria-expanded="expandedOriginal.has(post.id)" :aria-controls="`original-${post.id}`" @click="toggleOriginal(post.id)">{{ expandedOriginal.has(post.id) ? '收起原文' : '展开原文' }}</button>
          </div>
          <p v-else class="original-missing">原文暂未收录，可点击标题查看原推文。</p>
          <div v-if="post.note" style="margin-top:6px"><span class="note-pill">{{ post.note }}</span></div>
          <div class="chips" style="margin-top:8px">
            <button v-for="t in (post.tags || []).slice(0, 8)" :key="t" type="button" class="chip" @click="selectTagOnly(t)">{{ t }}</button>
          </div>
          <div style="margin-top:8px;display:flex;gap:6px;flex-wrap:wrap;align-items:center">
            <span v-for="cid in catsOf(post.id)" :key="cid" class="chip active">{{ categoryName(cid) }}</span>
            <button class="btn btn-sm" type="button" @click="catMenuPost = catMenuPost === post.id ? null : post.id">{{ t('index.category') }}</button>
            <button class="btn btn-sm" :class="{ 'btn-primary': replicatedIds.has(post.id) }" type="button" @click="replicate(post)">
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
          <section v-if="post.links?.length" class="post-resources" aria-label="资源链接">
            <h3>资源</h3>
            <ul><li v-for="url in post.links" :key="url"><a :href="url" target="_blank" rel="noopener noreferrer">{{ url }}</a></li></ul>
          </section>
          <section v-if="post.summary" class="post-ai-summary" aria-label="AI 摘要">
            <h3>AI 摘要</h3>
          <p class="post-summary" v-html="highlightHtml(summaryShown(post), plainKeyword)" />
          <button v-if="post.is_long" class="btn btn-sm" type="button" @click="toggleLong(post.id)">{{ expandedLong.has(post.id) ? t('index.collapseLong') : t('index.expandLong') }}</button>
          </section>
          <details v-if="shouldShowFlowchart(post) && (post.steps || []).length" class="steps" style="margin-top:10px">
            <summary>{{ t('index.stepsSummary', { n: post.steps!.length }) }}</summary>
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
          </details>
          </div>
        </article>
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
import type { Post, Watchlist, Categories } from '~/composables/types'
import { LS, useSettings } from '~/composables/useSettings'
import { avatarUrl, avatarColor, avatarInitial, relativeTime, shanghaiTime, shanghaiDayBound, highlightHtml, firstHitSnippet, shouldShowFlowchart, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'

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
const expandedLong = ref<Set<string>>(new Set())
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
  const hay = [p.title, p.text || '', ...(p.links || []), p.summary || '', p.industry || '', p.tech || '', p.author, '@' + p.author, p.author_name, accountOf(p.author)?.bio || '', ...userTagsOf(p.author), ...(p.tags || []), ...catsOf(p.id).map(categoryName), shanghaiTime(p.published_at), ...stepTexts].join('\n').toLowerCase()
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
function originalShown (p: Post) {
  const text = p.text || ''
  return text.length > 280 && !expandedOriginal.value.has(p.id) ? text.slice(0, 280) + '…' : text
}
function toggleLong (id: string) { expandedLong.value.has(id) ? expandedLong.value.delete(id) : expandedLong.value.add(id); expandedLong.value = new Set(expandedLong.value) }
function summaryShown (p: Post) {
  // Translate the full summary first, then truncate the translation.
  const s = tr(p.summary || '')
  if (!p.is_long || expandedLong.value.has(p.id)) return s
  return s.slice(0, 200) + (s.length > 200 ? '…' : '')
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
    if (p.summary) texts.push(p.summary)
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
function onAvatarError (e: Event, post: Post) { (e.target as HTMLImageElement).style.display = 'none'; post.author_avatar = '' }
async function replicate (post: Post) {
  if (replicatedIds.value.has(post.id)) return
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
