<template>
  <div>
    <main class="feed-col" style="max-width:600px;margin:0 auto;padding:16px;min-width:0">
      <div class="filter-row" style="margin-bottom:12px">
        <span class="updated" style="color:var(--text-muted)">{{ updatedAt ? `更新于 ${updatedAt}` : '' }}</span>
        <span style="flex:1" />
        <button class="btn btn-sm" type="button" @click="triggerWebhook" :disabled="webhookBusy">{{ webhookBusy ? '发送中…' : '立即更新' }}</button>
        <button class="btn btn-sm" type="button" @click="showSettings = true">设置</button>
      </div>
      <p v-if="loadError" style="color:var(--danger-text)">{{ loadError }}</p>
      <p v-if="loading">加载中…</p>

      <section class="filter-panel" v-if="!loading">
        <div class="filter-row" style="justify-content:space-between">
          <button class="btn btn-sm" type="button" @click="filtersCollapsed = !filtersCollapsed" :aria-expanded="!filtersCollapsed">{{ filtersCollapsed ? '展开搜索筛选 ▸' : '收起搜索筛选 ▾' }}</button>
          <span class="result-count">共 {{ filtered.length }} 条</span>
          <span class="active-count" v-if="filtersCollapsed && keyword.trim()">搜索：{{ keyword.trim() }}</span>
        </div>
        <div v-show="!filtersCollapsed">
          <div class="filter-row" style="margin-top:8px">
            <input class="field grow" style="flex:1" type="search" v-model="keyword" placeholder="联合搜索：标题、作者、标签、类别、日期，例如：AI、@黄小木、10-25" aria-label="联合搜索" />
            <button class="btn btn-sm" type="button" @click="clearAll">清除</button>
          </div>
          <p v-if="keyword.trim()" class="hint">正在全库搜索：匹配标题、作者、标签、类别与日期（支持 标签:xxx / tag:xxx 只搜用户标签；清空后回到关注列表）</p>
          <p v-if="keyword && firstSnippet" class="hint" v-html="`首命中：${firstSnippet}`" />
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
              <div style="color:var(--text-muted);font-size:13px">{{ relativeTime(post.published_at) }} · {{ shanghaiTime(post.published_at) }}<span v-if="post.industry"> · {{ post.industry }}</span><span v-if="post.tech"> · {{ post.tech }}</span></div>
            </div>
          </div>
          <h2 class="post-title"><a :href="post.url" target="_blank" rel="noopener" v-html="highlightHtml(post.title, plainKeyword)" /></h2>
          <p class="post-summary" v-html="highlightHtml(summaryShown(post), plainKeyword)" />
          <button v-if="post.is_long" class="btn btn-sm" type="button" @click="toggleLong(post.id)">{{ expandedLong.has(post.id) ? '收起' : '展开长文' }}</button>
          <div v-if="post.note" style="margin-top:6px"><span class="note-pill">{{ post.note }}</span></div>
          <div class="chips" style="margin-top:8px">
            <button v-for="t in (post.tags || []).slice(0, 8)" :key="t" type="button" class="chip" @click="searchTag(t)">{{ t }}</button>
          </div>
          <div style="margin-top:8px;display:flex;gap:6px;flex-wrap:wrap;align-items:center">
            <span v-for="cid in catsOf(post.id)" :key="cid" class="chip active">{{ categoryName(cid) }}</span>
            <button class="btn btn-sm" type="button" @click="catMenuPost = catMenuPost === post.id ? null : post.id">分类</button>
            <button class="btn btn-sm" :class="{ 'btn-primary': replicatedIds.has(post.id) }" type="button" @click="replicate(post)">
              {{ replicatedIds.has(post.id) ? '已复刻' : '复刻' }}
            </button>
            <NuxtLink v-if="replicatedIds.has(post.id)" to="/fuke" class="btn btn-sm">去复刻页</NuxtLink>
          </div>
          <div v-if="catMenuPost === post.id" class="cat-menu" style="margin-top:8px">
            <label v-for="c in categories.categories" :key="c.id" class="cat-menu-item" style="display:flex;gap:6px">
              <input type="checkbox" :checked="catsOf(post.id).includes(c.id)" @change="togglePostCat(post.id, c.id, ($event.target as HTMLInputElement).checked)" />
              <span>{{ c.name }}</span>
            </label>
            <div style="display:flex;gap:6px;margin-top:6px">
              <input class="field" v-model="newCatName" placeholder="新建类别…" maxlength="20" />
              <button class="btn btn-sm btn-primary" type="button" @click="createCat(post.id)">新建</button>
            </div>
          </div>
          <details v-if="shouldShowFlowchart(post) && (post.steps || []).length" class="steps" style="margin-top:10px">
            <summary>实现步骤（{{ post.steps!.length }}）· 流程图</summary>
            <div class="flow-chart">
              <div v-for="(step, i) in post.steps" :key="i" class="flow-step" :class="{ 'is-last': i === post.steps!.length - 1 }">
                <div class="flow-rail" aria-hidden="true"><span class="flow-node" /><span v-if="i !== post.steps!.length - 1" class="flow-line" /></div>
                <div class="flow-card">
                  <div class="flow-text" v-html="highlightHtml(`${i + 1}. ${step.text}`, plainKeyword)" />
                  <div class="flow-meta" v-if="step.tools || step.tech || step.prep">
                    <span v-if="step.tools" class="flow-chip"><span class="flow-chip-label">工具</span>{{ step.tools }}</span>
                    <span v-if="step.tech" class="flow-chip"><span class="flow-chip-label">技术</span>{{ step.tech }}</span>
                    <span v-if="step.prep" class="flow-chip"><span class="flow-chip-label">前提</span>{{ step.prep }}</span>
                  </div>
                  <div v-if="(step.substeps || []).length" style="margin-top:6px">
                    <button class="btn btn-sm" type="button" @click="toggleSubsteps(post.id, i)">{{ substepsOpen.has(`${post.id}:${i}`) ? '收起子步骤' : `展开子步骤（${step.substeps!.length}）` }}</button>
                    <ol v-if="substepsOpen.has(`${post.id}:${i}`)">
                      <li v-for="(sub, j) in step.substeps!.slice(0, 5)" :key="j" :title="sub.text">{{ `${i + 1}.${j + 1} ` }}{{ trunc40(sub.text) }}<span v-if="sub.source" style="color:var(--text-muted)"> — {{ sub.source }}</span></li>
                    </ol>
                  </div>
                </div>
              </div>
            </div>
          </details>
          </div>
        </article>
        <p v-if="!loading && !filtered.length" class="empty">没有符合条件的帖子。试试清除筛选。</p>
      </section>
    </main>

    <!-- Settings modal -->
    <div v-if="showSettings" class="modal-backdrop" @click.self="showSettings = false">
      <div class="modal-card" role="dialog" aria-modal="true" aria-label="设置">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px">
          <h2 style="margin:0">设置</h2>
          <button class="btn btn-sm" type="button" @click="showSettings = false" aria-label="关闭设置">✕</button>
        </div>
        <label>主题</label>
        <div style="display:flex;gap:8px">
          <button class="btn" :class="{ 'btn-primary': theme === 'dark' }" type="button" @click="setTheme('dark')">深色（默认）</button>
          <button class="btn" :class="{ 'btn-primary': theme === 'light' }" type="button" @click="setTheme('light')">浅色</button>
        </div>
        <label>字号</label>
        <div style="display:flex;gap:6px;flex-wrap:wrap">
          <button v-for="s in ['13px', '15px', '17px', '19px']" :key="s" class="btn btn-sm" :class="{ 'btn-primary': fontSize === s }" type="button" @click="setFontSize(s)">{{ s }}</button>
        </div>
        <label>字体</label>
        <select class="field" :value="fontFamily" @change="setFontFamily(($event.target as HTMLSelectElement).value)">
          <option value="">系统默认</option>
          <option value="'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif">苹方 / 微软雅黑</option>
          <option value="Georgia,serif">衬线</option>
          <option value="ui-monospace,Menlo,monospace">等宽</option>
        </select>

        <div class="settings-section">
          <h3>关注列表管理</h3>
          <div style="display:flex;gap:6px;flex-wrap:wrap">
            <input class="field" v-model="newGroupName" placeholder="新分组名称" style="flex:1;min-width:140px" />
            <button class="btn btn-sm btn-primary" type="button" @click="doAddGroup">新增分组</button>
          </div>
          <div v-for="g in watchlist.groups" :key="g.id" class="watch-row">
            <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">({{ g.id }})</span>
            <button v-if="!g.fixed" class="btn btn-sm btn-danger" type="button" @click="doDeleteGroup(g.id)">删除分组（作者归入默认）</button>
          </div>
          <div v-for="a in watchlist.accounts" :key="a.username" class="watch-row">
            <input :value="a.name" placeholder="显示名" @change="doPatchAccount(a.username, { name: ($event.target as HTMLInputElement).value })" />
            <span>@{{ a.username }}</span>
            <select :value="a.group" @change="doPatchAccount(a.username, { group: ($event.target as HTMLSelectElement).value })">
              <option v-for="g in watchlist.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
            <input :value="(a.tags || []).join('，')" placeholder="用户标签（中文逗号分隔）" @change="doPatchAccount(a.username, { tags: ($event.target as HTMLInputElement).value.split(/[,，]/).map(s => s.trim()).filter(Boolean) })" />
            <button class="btn btn-sm btn-danger" type="button" @click="doDeleteAccount(a.username)">删除作者</button>
          </div>
          <div class="watch-row">
            <input class="field" v-model="newAccount.username" placeholder="username（不含 @）" />
            <input class="field" v-model="newAccount.name" placeholder="显示名" />
            <select v-model="newAccount.group">
              <option v-for="g in watchlist.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
            <button class="btn btn-sm btn-primary" type="button" @click="doAddAccount">新增作者</button>
          </div>
        </div>

        <div class="settings-section">
          <h3>用户标签</h3>
          <p style="color:var(--text-muted)">词汇表（用于自定义标签 chips 与 <code>标签:xxx</code> 搜索）。</p>
          <div class="chips">
            <button v-for="t in userTagVocab" :key="t" type="button" class="chip" @click="removeVocab(t)">{{ t }} ×</button>
          </div>
          <div style="display:flex;gap:6px;margin-top:8px">
            <input class="field" v-model="newVocab" placeholder="新增用户标签" style="flex:1" />
            <button class="btn btn-sm btn-primary" type="button" @click="addVocab">添加</button>
          </div>
          <label>按作者覆盖自定义标签（保存后即用于该作者的用户标签过滤）</label>
          <div v-for="a in watchlist.accounts" :key="'ov-' + a.username" class="watch-row">
            <span style="min-width:120px">{{ a.name }} @{{ a.username }}</span>
            <input class="field" :value="(userTagOverrides[a.username] || []).join('，')" style="flex:1"
              @change="doSetUserTags(a.username, ($event.target as HTMLInputElement).value.split(/[,，]/).map(s => s.trim()).filter(Boolean))" />
          </div>
        </div>

        <div class="settings-section">
          <h3>Webhook（仅存浏览器 localStorage，不发往服务端）</h3>
          <label>Webhook URL</label>
          <input class="field" v-model="webhookUrlDraft" placeholder="https://…" />
          <label>Webhook Key</label>
          <input class="field" v-model="webhookKeyDraft" placeholder="key（已带 Bearer 时不重复加）" type="password" />
          <p v-if="webhookMsg" style="color:var(--text-muted)">{{ webhookMsg }}</p>
          <div class="modal-actions" style="justify-content:flex-start">
            <button class="btn btn-sm btn-primary" type="button" @click="saveWebhookDraft">保存</button>
            <button class="btn btn-sm" type="button" @click="clearWebhookDraft">清除</button>
            <button class="btn btn-sm" type="button" @click="triggerWebhook">触发一次</button>
          </div>
        </div>

        <div class="modal-actions">
          <button class="btn btn-primary" type="button" @click="showSettings = false">完成</button>
        </div>
      </div>
    </div>
    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { Post, Watchlist, Categories } from '~/composables/types'
import { LS, useSettings } from '~/composables/useSettings'
import { avatarUrl, avatarColor, avatarInitial, relativeTime, shanghaiTime, highlightHtml, firstHitSnippet, shouldShowFlowchart, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'

const store = useStore()
const settings = useSettings()
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
// 作者范围只作默认视图（关注列表）用，不再有单独的筛选输入；一旦输入查询即全库联合搜索。
const selectedAuthors = ref<Set<string>>(new Set())
const filtersCollapsed = ref(false)
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
  } catch { /* ignore */ }
}
function accountOf (username: string) { return watchlist.value.accounts.find(a => a.username === username) }
function userTagsOf (username: string): string[] {
  // SPEC: user tags = watchlist account tags, with per-author overrides winning.
  if (userTagOverrides.value[username]) return userTagOverrides.value[username]
  return accountOf(username)?.tags || []
}
const catsOf = (postId: string) => categories.value.assignments[postId] || []
const categoryName = (id: string) => categories.value.categories.find(c => c.id === id)?.name || id
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
  // 联合搜索：标题、摘要、行业、技术、作者（名 / @handle / 简介）、用户标签、帖子标签、
  // 所属类别名、步骤文本、发布日期（上海时间，形如 2026-10-25 16:28，搜 10-25 也能命中）。
  const hay = [p.title, p.summary || '', p.industry || '', p.tech || '', p.author, '@' + p.author, p.author_name, accountOf(p.author)?.bio || '', ...userTagsOf(p.author), ...(p.tags || []), ...catsOf(p.id).map(categoryName), shanghaiTime(p.published_at), ...stepTexts].join('\n').toLowerCase()
  // Tag field: strip leading # before matching (SPEC).
  return hay.includes(q.replace(/^#+/, ''))
}
const filtered = computed(() => {
  const q = keyword.value.trim()
  // 空查询时按关注列表作者范围看（SPEC: 空作者选择 = 空列表，不是不过滤）；
  // 一旦输入查询，全库联合搜索，不受作者范围限制。
  return posts.value.filter(p => (q ? true : selectedAuthors.value.has(p.author)) && matchesKeyword(p))
})
const firstSnippet = computed(() => {
  if (!keyword.value.trim() || !filtered.value.length) return ''
  return firstHitSnippet(filtered.value[0], plainKeyword.value || keyword.value.trim())
})
// 点帖子上的标签，把标签填进联合搜索框（替代原来的“只看该标签”筛选）。
function searchTag (t: string) { keyword.value = t }
function clearAll () { keyword.value = '' }
function toggleLong (id: string) { expandedLong.value.has(id) ? expandedLong.value.delete(id) : expandedLong.value.add(id); expandedLong.value = new Set(expandedLong.value) }
function summaryShown (p: Post) {
  if (!p.is_long || expandedLong.value.has(p.id)) return p.summary || ''
  return (p.summary || '').slice(0, 200) + ((p.summary || '').length > 200 ? '…' : '')
}
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
    showToast(replica ? '已生成复刻，去复刻页查看' : '已复刻')
  } catch (err: any) { showToast(err?.message || '生成复刻失败') }
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
  }
}
async function doAddGroup () {
  if (!newGroupName.value.trim()) { showToast('请先输入分组名称'); return }
  try {
    await store.addGroup(newGroupName.value.trim()); newGroupName.value = ''; await refreshWatchlist()
    showToast('分组已新增')
  } catch (e: any) {
    showToast(e?.message || '新增分组失败，请重试')
  }
}
async function doDeleteGroup (id: string) {
  if (!confirm('删除该分组？组内作者将归入“默认”。')) return
  await store.deleteGroup(id); await refreshWatchlist()
}
async function doAddAccount () {
  if (!newAccount.value.username.trim() || !newAccount.value.name.trim()) { showToast('请填写 username 与显示名'); return }
  await store.addAccount({ ...newAccount.value, avatar: '', tags: [], bio: '' })
  newAccount.value = { username: '', name: '', group: 'other' }; await refreshWatchlist()
}
async function doPatchAccount (username: string, patch: any) { await store.patchAccount(username, patch); await refreshWatchlist() }
async function doDeleteAccount (username: string) {
  if (!confirm(`删除作者 @${username}？`)) return
  await store.deleteAccount(username); await refreshWatchlist()
}
async function doSetUserTags (username: string, tags: string[]) { await store.setUserTags(username, tags); await refreshWatchlist() }
function addVocab () {
  if (!newVocab.value.trim() || userTagVocab.value.includes(newVocab.value.trim())) return
  userTagVocab.value = [...userTagVocab.value, newVocab.value.trim()]; newVocab.value = ''
}
function removeVocab (t: string) { userTagVocab.value = userTagVocab.value.filter(x => x !== t) }
function saveWebhookDraft () { saveWebhook(webhookUrlDraft.value.trim(), webhookKeyDraft.value); webhookMsg.value = 'Webhook 已保存到本浏览器 localStorage。' }
function clearWebhookDraft () { clearWebhook(); webhookUrlDraft.value = ''; webhookKeyDraft.value = ''; webhookMsg.value = 'Webhook 已清除。' }
async function triggerWebhook () {
  if (webhookBusy.value) return
  webhookBusy.value = true
  try {
    const r = await postWebhook()
    showToast(`Webhook 已发送（${r.via}${r.status ? ` · ${r.status}` : ''}）`)
    webhookMsg.value = `上次发送：${r.via}${r.status ? ` · HTTP ${r.status}` : ''}`
  } catch (err: any) { showToast(err?.message || 'Webhook 发送失败'); webhookMsg.value = err?.message || '发送失败' }
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
  } catch (err: any) { loadError.value = err?.message || '数据加载失败' }
  finally { loading.value = false }
})
</script>
