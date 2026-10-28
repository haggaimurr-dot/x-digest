<template>
  <div>
    <main class="feed-col" style="max-width:760px;margin:0 auto;padding:16px;min-width:0">
      <div class="filter-row" style="margin-bottom:12px">
        <span class="updated" style="color:var(--text-muted)">{{ updatedAt ? t('index.updatedAt', { time: updatedAt }) : '' }}</span>
        <span style="flex:1" />
        <button class="btn btn-sm" type="button" @click="xDialog = 'post'">导入单帖</button>
        <button class="btn btn-sm" type="button" @click="xDialog = 'refresh'">更新帖子</button>
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
          <div v-if="kindCounts.length" style="margin-top:8px">
            <div class="chip-group-title">帖子类型</div>
            <div class="chips">
              <button v-for="kc in kindCounts" :key="kc.kind" type="button" class="chip" :class="{ active: kindFilter === kc.kind }" @click="kindFilter = kindFilter === kc.kind ? '' : kc.kind">{{ kc.kind }}（{{ kc.n }}）</button>
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
                <div class="x-orig-label"><span>𝕏 原文</span><a class="x-orig-link" :href="post.url" target="_blank" rel="noopener">查看原帖 ↗</a></div>
                <p v-if="post.text" class="x-orig-text" :class="{ 'yt-desc-open': expandedOriginal.has(post.id) }" :id="`original-${post.id}`"><template v-for="(part, pi) in linkParts(post.text)" :key="pi"><a v-if="part.href" class="yt-desc-link" :href="part.href" :title="part.href" target="_blank" rel="noopener noreferrer">{{ part.text.replace(/^https?:\/\/(www\.)?/, '') }}</a><span v-else v-html="highlightHtml(part.text, plainKeyword)" /></template></p>
                <p v-else class="x-orig-text">原文暂未收录，<a :href="post.url" target="_blank" rel="noopener" style="color:var(--accent)">点击查看原推文</a>。</p>
              </div>
              <button v-if="isLongText(post.text)" class="yt-desc-more" type="button" :aria-expanded="expandedOriginal.has(post.id)" :aria-controls="`original-${post.id}`" @click="toggleOriginal(post.id)">{{ expandedOriginal.has(post.id) ? '收起' : '展开' }}</button>
            </div>
            <div class="yt-info">
              <p v-if="post.title" class="x-brief"><span class="x-brief-label">内容简介：</span><span v-html="highlightHtml(tr(post.title), plainKeyword)" /></p>
              <p class="yt-meta"><a class="yt-meta-link" :href="post.url" target="_blank" rel="noopener">{{ shanghaiTime(post.published_at) }}（{{ relativeTime(post.published_at, lang.value) }}）</a><span v-if="post.industry"> · {{ post.industry }}</span><span v-if="post.tech"> · {{ post.tech }}</span></p>
              <div class="yt-tags">
                <button v-if="post.kind" type="button" class="chip active yt-tag" :class="{ 'x-kind-on': kindFilter === post.kind }" @click="kindFilter = kindFilter === post.kind ? '' : post.kind">{{ post.kind }}</button>
                <button v-for="tg in (post.tags || []).filter(tag => tag !== '复刻' || isReplicable(post)).slice(0, 8)" :key="tg" type="button" class="chip yt-tag yt-topic" :class="{ active: selectedTags.has(tg) }" @click="selectTagOnly(tg)">#{{ tg }}</button>
                <span v-for="cid in catsOf(post.id)" :key="cid" class="chip active yt-tag">{{ categoryName(cid) }}</span>
              </div>
              <div v-if="post.note" style="margin-top:6px"><span class="note-pill">{{ post.note }}</span></div>
              <div class="yt-actions">
                <button class="btn btn-sm cat-toggle" type="button" :aria-expanded="catMenuPost === post.id" @click="catMenuPost = catMenuPost === post.id ? null : post.id">{{ t('index.category') }}</button>
                <button v-if="isReplicable(post)" class="btn btn-sm" :class="{ 'btn-primary': replicatedIds.has(post.id) }" type="button" @click="replicate(post)">
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
                <div style="display:flex;justify-content:flex-end;margin-top:6px">
                  <button class="btn btn-sm" type="button" @click="catMenuPost = null">完成</button>
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
        <nav v-if="filtered.length" class="x-pager" aria-label="分页">
          <button class="btn btn-sm" type="button" :disabled="page <= 1" @click="goPage(page - 1)">上一页</button>
          <template v-for="(it, ii) in pageItems" :key="'pg' + ii">
            <span v-if="it === '…'" class="x-pager-gap">…</span>
            <button v-else class="btn btn-sm" :class="{ 'btn-primary': it === page }" type="button" :aria-current="it === page ? 'page' : undefined" @click="goPage(it)">{{ it }}</button>
          </template>
          <button class="btn btn-sm" type="button" :disabled="page >= pageCount" @click="goPage(page + 1)">下一页</button>
          <label class="x-pager-size">每页
            <select class="field" :value="pageSize" @change="setPageSize(Number(($event.target as HTMLSelectElement).value))">
              <option v-for="n in PAGE_SIZES" :key="n" :value="n">{{ n }} 条</option>
            </select>
          </label>
          <span class="x-pager-info">共 {{ filtered.length }} 条</span>
        </nav>
      </section>
    </main>

    <!-- Settings modal -->
    <div v-if="showSettings" class="modal-backdrop" @click.self="showSettings = false">
      <div class="modal-card" role="dialog" aria-modal="true" :aria-label="t('settings.title')">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px">
          <h2 style="margin:0">{{ t('settings.title') }}</h2>
          <button class="btn btn-sm" type="button" @click="showSettings = false" :aria-label="t('settings.close')">✕</button>
        </div>
        <p style="color:var(--text-muted);font-size:13px;margin:6px 0 0">主题、字号、字体在顶栏右上角「Aa 外观」里设置，对所有页面生效。</p>

        <div class="settings-section">
          <h3>{{ t('settings.watchlist') }}</h3>
          <!-- 新增作者放最上面：填 @用户名 或 x.com 链接即可，显示名可不填 -->
          <div class="wl-add">
            <div class="wl-add-title">新增作者</div>
            <div class="wl-add-row">
              <input class="field" v-model="newAccount.username" :placeholder="t('settings.usernamePh')" @keyup.enter="doAddAccount" />
              <input class="field" v-model="newAccount.name" :placeholder="t('settings.displayNamePh') + '（可选）'" @keyup.enter="doAddAccount" />
              <select class="field" v-model="newAccount.group">
                <option v-for="g in watchlist.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
              </select>
              <button class="btn btn-sm btn-primary" type="button" @click="doAddAccount">{{ t('settings.addAuthor') }}</button>
            </div>
          </div>
          <!-- 已关注作者：按分组折叠 + 搜索；点「编辑」才展开该作者的编辑项 -->
          <div class="wl-list-head">
            <span>已关注 {{ watchlist.accounts.length }} 位作者</span>
            <input class="field wl-search" type="search" v-model="authorSearch" placeholder="搜索名字或 @用户名" />
          </div>
          <div v-for="grp in watchGroupsView" :key="grp.id" class="wl-group">
            <button class="wl-group-head" type="button" :aria-expanded="isWatchGroupOpen(grp.id)" @click="toggleWatchGroup(grp.id)">
              <svg class="yt-chevron" :class="{ 'is-collapsed': !isWatchGroupOpen(grp.id) }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
              <span>{{ grp.name }}</span><span class="yt-section-count">（{{ grp.accounts.length }}）</span>
            </button>
            <template v-if="isWatchGroupOpen(grp.id)">
              <div v-for="a in grp.accounts" :key="a.username" class="wl-author">
                <div class="wl-author-line">
                  <span class="wl-author-name">{{ a.name || a.username }}</span>
                  <a class="wl-author-handle" :href="`https://x.com/${a.username}`" target="_blank" rel="noopener">@{{ a.username }}</a>
                  <span v-for="tg in (a.tags || [])" :key="tg" class="chip yt-tag">{{ tg }}</span>
                  <span style="flex:1" />
                  <button class="btn btn-sm" type="button" :aria-expanded="editingAuthor === a.username" @click="editingAuthor = editingAuthor === a.username ? '' : a.username">{{ editingAuthor === a.username ? '收起' : '编辑' }}</button>
                </div>
                <div v-if="editingAuthor === a.username" class="wl-author-edit">
                  <label>{{ t('settings.displayNamePh') }}<input class="field" :value="a.name" @change="doPatchAccount(a.username, { name: ($event.target as HTMLInputElement).value })" /></label>
                  <label>分组<select class="field" :value="a.group" @change="doPatchAccount(a.username, { group: ($event.target as HTMLSelectElement).value })">
                    <option v-for="g in watchlist.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
                  </select></label>
                  <div class="wl-field"><span>用户标签（用于「用户标签」筛选）</span>
                    <TagInput :model-value="userTagsOf(a.username)" :suggestions="userTagVocab" @update:model-value="saveAuthorTags(a.username, $event)" />
                  </div>
                  <div style="display:flex;justify-content:flex-end"><button class="btn btn-sm btn-danger" type="button" @click="doDeleteAccount(a.username)">{{ t('settings.deleteAuthor') }}</button></div>
                </div>
              </div>
            </template>
          </div>
          <p v-if="authorSearch && !watchGroupsView.length" style="color:var(--text-muted)">没有匹配的作者</p>
          <!-- 分组管理（折叠） -->
          <button class="wl-group-head" type="button" style="margin-top:10px" :aria-expanded="showGroupMgr" @click="showGroupMgr = !showGroupMgr">
            <svg class="yt-chevron" :class="{ 'is-collapsed': !showGroupMgr }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            <span>分组管理</span><span class="yt-section-count">（{{ watchlist.groups.length }}）</span>
          </button>
          <template v-if="showGroupMgr">
            <div style="display:flex;gap:6px;flex-wrap:wrap">
              <input class="field" v-model="newGroupName" :placeholder="t('settings.newGroupPh')" style="flex:1;min-width:140px" @keyup.enter="doAddGroup" />
              <button class="btn btn-sm btn-primary" type="button" @click="doAddGroup">{{ t('settings.addGroup') }}</button>
            </div>
            <div v-for="g in watchlist.groups" :key="g.id" class="watch-row">
              <strong>{{ g.name }}</strong><span style="color:var(--text-muted)">({{ g.id }})</span>
              <button v-if="!g.fixed" class="btn btn-sm btn-danger" type="button" @click="doDeleteGroup(g.id)">{{ t('settings.deleteGroup') }}</button>
            </div>
          </template>
        </div>

        <div class="settings-section">
          <h3>{{ t('settings.userTagSection') }}</h3>
          <p style="color:var(--text-muted);font-size:13px;margin:0">给作者打的标签，用于「用户标签」筛选和关键词 <code>标签:xxx</code> 搜索。这里维护常用标签，作者的标签在上方「编辑」里选。</p>
          <div class="wl-add-row" style="grid-template-columns:1fr auto;margin-top:8px">
            <input class="field" v-model="newVocab" :placeholder="t('settings.newUserTagPh') + '（回车添加，可用逗号一次加多个）'" @keyup.enter="addVocab" />
            <button class="btn btn-sm btn-primary" type="button" @click="addVocab">{{ t('settings.add') }}</button>
          </div>
          <div class="tagmgr-list">
            <div v-for="tg in tagManagerRows" :key="tg.tag" class="tagmgr-row">
              <span class="tagmgr-name">{{ tg.tag }}</span>
              <span class="tagmgr-count">{{ tg.n ? `${tg.n} 位作者` : '未使用' }}</span>
              <span v-if="!tg.inVocab" class="tagmgr-count">（作者自带，未在常用列表）</span>
              <span style="flex:1" />
              <button v-if="!tg.inVocab" class="btn btn-sm" type="button" @click="addVocabTag(tg.tag)">加入常用</button>
              <button v-else class="btn btn-sm btn-danger" type="button" @click="removeVocab(tg.tag)">移出</button>
            </div>
            <p v-if="!tagManagerRows.length" style="color:var(--text-muted);font-size:13px">还没有标签。</p>
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
    <XImportDialog v-if="xDialog" :mode="xDialog" :accounts="watchlist.accounts" @close="xDialog = null" @changed="reloadPosts" />
    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200">{{ toast }}</p>
  </div>
</template>

<script setup lang="ts">
import type { Post, Watchlist, WatchlistAccount, Categories, YTSummary } from '~/composables/types'
import { LS, useSettings } from '~/composables/useSettings'
import { canReplicatePost } from '~/utils/x-post.mjs'
import { avatarUrl, avatarColor, avatarInitial, relativeTime, shanghaiTime, shanghaiDayBound, highlightHtml, firstHitSnippet, shouldShowFlowchart, trunc40 } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'
import { linkParts, descResources } from '~/utils/videoTags'

const store = useStore()
const settings = useSettings()
const { t, lang } = useLang()
const { tr, enqueue, translating } = useTranslate()
const { saveWebhook, clearWebhook, postWebhook } = settings

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
const xDialog = ref<'post' | 'refresh' | null>(null)
useScrollLock(computed(() => !!xDialog.value))
// After an import / refresh: posts and the watchlist (new authors) reload.
async function reloadPosts () {
  try {
    const p = await store.loadPosts()
    posts.value = p.posts; updatedAt.value = p.updated_at
    await refreshWatchlist()
  } catch { /* the next action retries */ }
}
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
  // 没选任何作者 = 不按作者筛选（显示全部）；其他条件同理，没选就不限制。
  return posts.value.filter(p => authorOk(p) && passesNonAuthor(p) &&
    (!kindFilter.value || kindOf(p) === kindFilter.value))
})
function authorOk (p: Post): boolean { return !selectedAuthors.value.size || selectedAuthors.value.has(p.author) }
// ---- 帖子类型（cmd/digest 的 AI 分类；没分类的旧帖为「未分类」） ----
const POST_KINDS = ['教程实操', '技术分享', '工具资源推荐', '观点讨论', '日常闲聊']
const kindFilter = ref('')
function kindOf (p: Post): string { return p.kind || '未分类' }
const kindCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const p of posts.value) if (authorOk(p) && passesNonAuthor(p)) counts.set(kindOf(p), (counts.get(kindOf(p)) || 0) + 1)
  return [...POST_KINDS, '未分类'].filter(k => counts.has(k)).map(k => ({ kind: k, n: counts.get(k)! }))
})
// 复刻：AI 判为「教程实操」才可复刻；没分类的旧帖沿用关键词规则 canReplicatePost。
function isReplicable (p: Post): boolean { return p.kind ? p.kind === '教程实操' : canReplicatePost(p) }
// ---- 分页：每页 10 / 20 / 50 条（记在本机），筛选变化回到第 1 页 ----
const PAGE_SIZES = [10, 20, 50]
const PAGE_SIZE_KEY = 'x-digest-x-page-size'
const pageSize = ref(20)
const page = ref(1)
const pageCount = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize.value)))
const pagedPosts = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
function setPageSize (n: number) {
  pageSize.value = n; page.value = 1
  try { localStorage.setItem(PAGE_SIZE_KEY, String(n)) } catch { /* private mode */ }
}
function goPage (n: number) {
  page.value = Math.min(Math.max(1, n), pageCount.value)
  if (import.meta.client) document.querySelector('.timeline')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
// 页码条：首尾 + 当前页附近，其余用省略号。
const pageItems = computed<Array<number | '…'>>(() => {
  const n = pageCount.value; const c = page.value
  const keep = [...new Set([1, n, c - 1, c, c + 1].filter(x => x >= 1 && x <= n))].sort((a, b) => a - b)
  const out: Array<number | '…'> = []
  keep.forEach((x, i) => { if (i && x - keep[i - 1] > 1) out.push('…'); out.push(x) })
  return out
})
watch(() => filtered.value.length, () => { if (page.value > pageCount.value) page.value = 1 })
watch([keyword, dateFrom, dateTo, kindFilter, selectedAuthors, selectedTags, selectedUserTags, selectedCats], () => { page.value = 1 })
const activeFilterCount = computed(() =>
  selectedTags.value.size + selectedUserTags.value.size + selectedCats.value.size +
  (keyword.value.trim() ? 1 : 0) + (dateFrom.value || dateTo.value ? 1 : 0) + (kindFilter.value ? 1 : 0))
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
  keyword.value = ''; dateFrom.value = ''; dateTo.value = ''; kindFilter.value = ''
  selectedTags.value = new Set(); selectedUserTags.value = new Set(); selectedCats.value = new Set()
  selectedAuthors.value = new Set()
  persistSelection()
}
// 原文默认折叠为 3 行：字数多或换行多（短句多行同样会被截断）时显示展开按钮。
function isLongText (text?: string) {
  const s = text || ''
  return s.length > 120 || s.split('\n').filter(l => l.trim()).length > 3
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
// 头像按顺序尝试，加载失败就换下一个，全部失败才显示首字母：
// 库里的 posts.author_avatar → 关注列表 authors.avatar → 约定的本地静态图
// /avatars/<username>.jpg（frontend/public/avatars，库里该列为空时也能显示）。
function avatarCandidates (post: Post): string[] {
  const list = [post.author_avatar, accountOf(post.author)?.avatar, `avatars/${post.author}.jpg`]
    .map(x => avatarUrl(x || '')).filter(Boolean)
  return [...new Set(list)]
}
function postAvatar (post: Post) {
  return avatarCandidates(post).find(src => !failedAvatars.value.has(src)) || ''
}
function onAvatarError (e: Event, post: Post) {
  const src = (e.target as HTMLImageElement).getAttribute('src') || postAvatar(post)
  failedAvatars.value = new Set([...failedAvatars.value, src])
}
// ---- 油管样式：按作者分组（顺序同关注列表）、折叠状态存本机 ----
const groupedByAuthor = computed(() => {
  const byAuthor = new Map<string, Post[]>()
  for (const p of pagedPosts.value) {
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
function hasFlow (p: Post) { return isReplicable(p) && shouldShowFlowchart(p) && !!(p.steps || []).length }
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
// 分类弹窗：点弹窗外任意处或按 Esc 关闭（「分类」按钮本身负责开关）。
function onDocClick (e: MouseEvent) {
  if (!catMenuPost.value) return
  const el = e.target as HTMLElement
  if (!el.closest('.cat-menu') && !el.closest('.cat-toggle')) catMenuPost.value = null
}
function onDocKey (e: KeyboardEvent) { if (e.key === 'Escape') catMenuPost.value = null }
onMounted(() => { document.addEventListener('click', onDocClick); document.addEventListener('keydown', onDocKey) })
onUnmounted(() => { document.removeEventListener('click', onDocClick); document.removeEventListener('keydown', onDocKey) })
// 资源：每条都带「是什么」——优先用 cmd/digest 的 AI 说明（post.resources），
// 其次原文同一行的文字，再按网址认出类型（推文 / GitHub 仓库 / 视频…）；
// 生成过 AI 摘要后补上摘要识别出的资源。
function describeLink (url: string): string {
  let u: URL
  try { u = new URL(url) } catch { return '链接' }
  const host = u.hostname.replace(/^www\./, '')
  const seg = u.pathname.split('/').filter(Boolean)
  if (/^(x|twitter)\.com$/.test(host) && seg[1] === 'status') return `@${seg[0]} 的推文`
  if (host === 't.co') return '原文附带的短链接（未展开）'
  if (host === 'github.com' && seg.length >= 2) return `GitHub 仓库 ${seg[0]}/${seg[1]}`
  if (/youtube\.com$|youtu\.be$/.test(host)) return 'YouTube 视频'
  if (/bilibili\.com$|b23\.tv$/.test(host)) return 'B站视频'
  if (host === 'mp.weixin.qq.com') return '微信公众号文章'
  if (/xiaohongshu\.com$|xhslink\.com$/.test(host)) return '小红书笔记'
  return host
}
function resourcesOf (p: Post): Array<{ label: string, url: string, note?: string }> {
  const out: Array<{ label: string, url: string, note?: string }> = []
  const add = (url: string, label?: string, note?: string) => {
    const hit = out.find(x => x.url === url)
    if (hit) { if (note && !hit.note) hit.note = note; return }
    out.push({ url, label: label || describeLink(url), note })
  }
  for (const r of p.resources || []) add(r.url, r.name, r.note)
  const fromLine = new Map(descResources(p.text || '', 50).map(r => [r.url, r.label]))
  for (const url of p.links || []) {
    const lineLabel = fromLine.get(url)
    add(url, lineLabel && !lineLabel.includes('.') ? lineLabel : undefined)
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
  if (!isReplicable(post) || replicatedIds.value.has(post.id)) return
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
    // Default = no author filter (all authors); a stored selection wins.
    let stored: string[] | null = null
    if (import.meta.client) {
      try { const raw = localStorage.getItem(LS.selectedAuthors); stored = raw ? JSON.parse(raw) : null } catch { stored = null }
    }
    selectedAuthors.value = stored !== null
      ? new Set(stored)
      : new Set()
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
// 关注列表：搜索、分组折叠（默认收起，搜索时自动展开）、一次只编辑一位作者。
const authorSearch = ref('')
const openWatchGroups = ref<Set<string>>(new Set())
const editingAuthor = ref('')
const showGroupMgr = ref(false)
const watchGroupsView = computed(() => {
  const q = authorSearch.value.trim().toLowerCase().replace(/^@/, '')
  const match = (a: WatchlistAccount) => !q || a.username.toLowerCase().includes(q) || (a.name || '').toLowerCase().includes(q)
  const known = new Set(watchlist.value.groups.map(g => g.id))
  const groups = [...watchlist.value.groups, { id: '__none', name: '未分组' }]
  return groups.map(g => ({
    id: g.id, name: g.name,
    accounts: watchlist.value.accounts.filter(a => (g.id === '__none' ? !known.has(a.group) : a.group === g.id) && match(a))
  })).filter(g => g.accounts.length)
})
function isWatchGroupOpen (id: string) { return !!authorSearch.value.trim() || openWatchGroups.value.has(id) }
function toggleWatchGroup (id: string) {
  const s = new Set(openWatchGroups.value)
  if (s.has(id)) s.delete(id); else s.add(id)
  openWatchGroups.value = s
}
// 用户名可填 @xxx、x.com/xxx 或 twitter.com/xxx 链接；显示名不填就用用户名。
function normalizeUsername (input: string): string {
  const s = input.trim()
  const m = s.match(/(?:x|twitter)\.com\/(@?[A-Za-z0-9_]{1,15})/i)
  return (m ? m[1] : s).replace(/^@/, '').trim()
}
async function doAddAccount () {
  const username = normalizeUsername(newAccount.value.username)
  if (!/^[A-Za-z0-9_]{1,15}$/.test(username)) { showToast(t('settings.accountEmpty')); return }
  if (watchlist.value.accounts.some(a => a.username.toLowerCase() === username.toLowerCase())) { showToast(`@${username} 已在关注列表中`); return }
  const name = newAccount.value.name.trim() || username
  try {
    await store.addAccount({ username, name, group: newAccount.value.group, avatar: '', tags: [], bio: '' })
  } catch (e: any) { showToast(e?.message || t('settings.accountEmpty')); return }
  newAccount.value = { username: '', name: '', group: newAccount.value.group }
  openWatchGroups.value = new Set([...openWatchGroups.value, watchlist.value.groups.some(g => g.id === newAccount.value.group) ? newAccount.value.group : '__none'])
  await refreshWatchlist()
  showToast(`已添加 @${username}`)
}
async function doPatchAccount (username: string, patch: any) { await store.patchAccount(username, patch); await refreshWatchlist() }
async function doDeleteAccount (username: string) {
  if (!confirm(t('settings.deleteAuthorConfirm', { username }))) return
  await store.deleteAccount(username); await refreshWatchlist()
}
// 常用标签（词汇表）：增删都立即保存到服务端。
async function saveVocab (next: string[]) {
  const prev = userTagVocab.value
  userTagVocab.value = next
  try { await store.setUserTagVocabulary(next) } catch (e: any) { userTagVocab.value = prev; showToast(e?.message || '保存标签失败') }
}
function addVocab () {
  const add = newVocab.value.split(/[,，、]/).map(x => x.trim()).filter(x => x && !userTagVocab.value.includes(x))
  newVocab.value = ''
  if (add.length) saveVocab([...userTagVocab.value, ...add])
}
function addVocabTag (tg: string) { if (!userTagVocab.value.includes(tg)) saveVocab([...userTagVocab.value, tg]) }
function removeVocab (tg: string) {
  const n = tagUsage.value.get(tg) || 0
  if (n && !confirm(`「${tg}」还有 ${n} 位作者在用。移出常用列表不会从这些作者身上删掉它，确定移出？`)) return
  saveVocab(userTagVocab.value.filter(x => x !== tg))
}
// 每个标签被多少位作者使用（按生效的标签：有覆盖用覆盖，否则用作者自带标签）。
const tagUsage = computed(() => {
  const m = new Map<string, number>()
  for (const a of watchlist.value.accounts) for (const tg of userTagsOf(a.username)) m.set(tg, (m.get(tg) || 0) + 1)
  return m
})
const tagManagerRows = computed(() => {
  const all = [...new Set([...userTagVocab.value, ...tagUsage.value.keys()])]
  return all.map(tag => ({ tag, n: tagUsage.value.get(tag) || 0, inVocab: userTagVocab.value.includes(tag) }))
    .sort((a, b) => b.n - a.n || a.tag.localeCompare(b.tag, 'zh'))
})
// 作者标签只编辑一份：写到作者自带标签，并清掉旧的覆盖，避免两处不一致；
// 新输入的标签顺手加入常用列表。
async function saveAuthorTags (username: string, tags: string[]) {
  try {
    await store.patchAccount(username, { tags })
    if (userTagOverrides.value[username]) await store.setUserTags(username, [])
    const fresh = tags.filter(x => !userTagVocab.value.includes(x))
    if (fresh.length) await saveVocab([...userTagVocab.value, ...fresh])
    await refreshWatchlist()
  } catch (e: any) { showToast(e?.message || '保存标签失败') }
}
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
    const n = Number(localStorage.getItem(PAGE_SIZE_KEY))
    if (PAGE_SIZES.includes(n)) pageSize.value = n
  } catch { /* ignore */ }
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
