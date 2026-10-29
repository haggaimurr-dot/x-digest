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

      <!-- 搜索区：所有条件放在一个大框里，改条件不会立即检索，点「搜索」（或在关键词框回车）才生效；「重置」清空条件并检索全部。 -->
      <section class="filter-panel search-panel" v-if="!loading" role="search" :aria-label="t('index.searchPanel')">
        <div class="filter-row" style="justify-content:space-between">
          <button class="btn btn-sm" type="button" @click="filtersCollapsed = !filtersCollapsed" :aria-expanded="!filtersCollapsed">{{ filtersCollapsed ? t('index.expandFilters') : t('index.collapseFilters') }}</button>
          <span class="active-count" v-if="filtersCollapsed && activeFilterCount">{{ t('index.selectedFilters', { n: activeFilterCount }) }}</span>
        </div>
        <form v-show="!filtersCollapsed" class="search-box" @submit.prevent="runSearch">
          <div class="unified-bar">
            <div class="cond cond-wide">
              <label class="cond-label" for="x-keyword">{{ t('index.keyword') }}</label>
              <input id="x-keyword" class="field" style="width:100%" type="search" v-model="keyword" :placeholder="t('index.keywordPh')" :aria-label="t('common.keywordSearch')" />
            </div>
            <FilterCombo :label="t('index.author')" :placeholder="t('index.authorPh')" :options="authorOptions" v-model="selectedAuthorsArr" />
            <FilterCombo :label="t('index.postTags')" :placeholder="t('index.postTagsPh')" :options="tagOptions" v-model="selectedTagsArr" />
            <FilterCombo :label="t('index.postCats')" :placeholder="t('index.postCatsPh')" :options="catOptions" v-model="selectedCatsArr" />
            <div class="cond">
              <label class="cond-label" for="x-min-chars">{{ t('index.minChars') }}</label>
              <select id="x-min-chars" class="field" style="width:100%" v-model.number="minChars">
                <option :value="0">{{ t('index.minCharsAny') }}</option>
                <option v-for="n in [50, 100, 200, 500, 1000]" :key="n" :value="n">{{ t('index.minCharsAbove', { n }) }}</option>
              </select>
            </div>
          </div>
          <div class="cond" style="margin-top:8px">
            <div class="cond-label">{{ t('index.kind') }}</div>
            <div class="chips">
              <button v-for="kc in kindCounts" :key="kc.kind" type="button" class="chip" :class="{ active: kindFilter === kc.kind }" :aria-pressed="kindFilter === kc.kind" @click="kindFilter = kindFilter === kc.kind ? '' : kc.kind">{{ kc.kind }}</button>
            </div>
          </div>
          <div class="filter-row search-actions">
            <button class="btn btn-primary" type="submit">{{ t('index.search') }}</button>
            <button class="btn" type="button" @click="resetFilters">{{ t('index.reset') }}</button>
            <span v-if="searchDirty" class="active-count">{{ t('index.pendingSearch') }}</span>
            <span style="flex:1" />
            <select v-if="savedViews.length" class="field" style="width:auto" :aria-label="t('index.views')" :value="currentViewId" @change="applyView(($event.target as HTMLSelectElement).value)">
              <option value="">{{ t('index.viewsPick') }}</option>
              <option v-for="v in savedViews" :key="v.id" :value="v.id">{{ v.name }}</option>
            </select>
            <button class="btn btn-sm" type="button" @click="saveCurrentView">{{ t('index.viewSave') }}</button>
            <button v-if="currentViewId" class="btn btn-sm btn-danger" type="button" @click="deleteCurrentView">{{ t('index.viewDelete') }}</button>
          </div>
          <div class="filter-row" style="margin-top:4px">
            <span class="result-count">{{ t('index.resultCount', { n: totalPosts }) }}</span>
            <span v-if="translating" class="active-count">{{ t('common.translating') }}</span>
            <span class="active-count" v-if="activeFilterCount">{{ t('index.selectedFilters', { n: activeFilterCount }) }}</span>
          </div>
          <p v-if="applied.keyword && firstSnippet" class="hint" v-html="t('index.firstHit') + firstSnippet" />
        </form>
      </section>

      <!-- 与油管页同一卡片样式：按作者分组（可折叠），左栏原文、右栏标题/时间/标签/操作，
           下方整行为资源与可折叠的 AI 摘要（含流程图）。 -->
      <!-- 多选：批量归类 / 批量生成 AI 摘要 / 批量删除 -->
      <div class="filter-row" style="margin:8px 0">
        <button class="btn btn-sm" :class="{ 'btn-primary': selectMode }" type="button" @click="toggleSelectMode">{{ selectMode ? '退出多选' : '多选' }}</button>
        <template v-if="selectMode">
          <span class="active-count">已选 {{ selectedPosts.size }} 帖</span>
          <button class="btn btn-sm" type="button" @click="selectPage">全选本页</button>
          <select v-model="batchCat" class="field" style="width:auto" aria-label="批量归类的类别">
            <option value="">选择类别…</option>
            <option v-for="c in categories.categories" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
          <button class="btn btn-sm" type="button" :disabled="!batchCat || !selectedPosts.size || batchBusy" @click="batchCategory('add')">归入</button>
          <button class="btn btn-sm" type="button" :disabled="!batchCat || !selectedPosts.size || batchBusy" @click="batchCategory('remove')">移出</button>
          <button class="btn btn-sm btn-primary" type="button" :disabled="!selectedPosts.size || batchBusy" @click="batchSummaries">批量生成 AI 摘要</button>
          <button class="btn btn-sm btn-danger" type="button" :disabled="!selectedPosts.size || batchBusy" @click="deleteSelected">删除所选</button>
          <span v-if="batchMsg" class="active-count">{{ batchMsg }}</span>
        </template>
      </div>
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
          <article v-for="post in grp.posts" :key="post.id" class="post-card yt-video-card" :class="{ 'is-selected': selectedPosts.has(post.id) }">
            <label v-if="selectMode" class="select-box"><input type="checkbox" :checked="selectedPosts.has(post.id)" @change="togglePostSelected(post.id)" /> 选择</label>
            <div class="yt-left">
              <div class="x-orig" :class="{ 'is-missing': !post.text }">
                <div class="x-orig-label"><span>𝕏 原文</span><a class="x-orig-link" :href="post.url" target="_blank" rel="noopener">查看原帖 ↗</a></div>
                <p v-if="post.text" class="x-orig-text" :class="{ 'yt-desc-open': expandedOriginal.has(post.id) }" :id="`original-${post.id}`"><template v-for="(part, pi) in linkParts(post.text)" :key="pi"><a v-if="part.href" class="yt-desc-link" :href="part.href" :title="part.href" target="_blank" rel="noopener noreferrer">{{ part.text.replace(/^https?:\/\/(www\.)?/, '') }}</a><span v-else v-html="highlightHtml(part.text, plainKeyword)" /></template></p>
                <p v-else class="x-orig-text">原文暂未收录，<a :href="post.url" target="_blank" rel="noopener" style="color:var(--accent)">点击查看原推文</a>。</p>
              </div>
              <!-- 原文/译文对照：按段落交替显示，译文走翻译缓存 -->
              <button v-if="post.text" class="yt-desc-more" type="button" :aria-pressed="bilingual.has(post.id)" @click="toggleBilingual(post)">{{ bilingual.has(post.id) ? '关闭对照' : '对照' }}</button>
              <div v-if="bilingual.has(post.id)" class="bilingual">
                <template v-for="(para, i) in paragraphsOf(post.text || '')" :key="i">
                  <p class="bi-orig">{{ para }}</p>
                  <p class="bi-tr">{{ tr(para) === para ? (translating ? '翻译中…' : '（与当前语言相同，无需翻译）') : tr(para) }}</p>
                </template>
              </div>
              <button v-if="isLongText(post.text)" class="yt-desc-more" type="button" :aria-expanded="expandedOriginal.has(post.id)" :aria-controls="`original-${post.id}`" @click="toggleOriginal(post.id)">{{ expandedOriginal.has(post.id) ? '收起' : '展开' }}</button>
            </div>
            <div class="yt-info">
              <p v-if="post.title" class="x-brief"><span class="x-brief-label">内容简介：</span><span v-html="highlightHtml(tr(post.title), plainKeyword)" /></p>
              <p class="yt-meta"><a class="yt-meta-link" :href="post.url" target="_blank" rel="noopener">{{ shanghaiTime(post.published_at) }}（{{ relativeTime(post.published_at, lang.value) }}）</a><span v-if="post.industry"> · {{ post.industry }}</span><span v-if="post.tech"> · {{ post.tech }}</span></p>
              <div class="yt-tags">
                <button v-if="post.kind" type="button" class="chip active yt-tag" :class="{ 'x-kind-on': applied.kind === post.kind }" @click="searchKind(post.kind)">{{ post.kind }}</button>
                <button v-for="tg in (post.tags || []).filter(tag => tag !== '复刻' || isReplicable(post)).slice(0, 8)" :key="tg" type="button" class="chip yt-tag yt-topic" :class="{ active: applied.tags.includes(tg) }" @click="selectTagOnly(tg)">#{{ tg }}</button>
                <span v-for="cid in catsOf(post.id)" :key="cid" class="chip active yt-tag">{{ categoryName(cid) }}</span>
              </div>
              <div v-if="post.note" style="margin-top:6px"><span class="note-pill">{{ post.note }}</span></div>
              <div class="yt-actions">
                <button class="btn btn-sm cat-toggle" type="button" :aria-expanded="catMenuPost === post.id" @click="catMenuPost = catMenuPost === post.id ? null : post.id">{{ t('index.category') }}</button>
                <button v-if="isReplicable(post)" class="btn btn-sm" :class="{ 'btn-primary': replicatedIds.has(post.id) }" type="button" @click="replicate(post)">
                  {{ replicatedIds.has(post.id) ? t('index.replicated') : t('index.replicate') }}
                </button>
                <NuxtLink v-if="replicatedIds.has(post.id)" to="/fuke" class="btn btn-sm">{{ t('index.goFuke') }}</NuxtLink>
                <button class="btn btn-sm btn-danger" type="button" :disabled="deleting.has(post.id)" @click="deletePost(post)">删除</button>
              </div>
              <div v-if="catMenuPost === post.id" class="cat-menu" style="margin-top:8px">
                <div v-for="c in categories.categories" :key="c.id" class="cat-menu-item" style="display:flex;gap:6px;align-items:center">
                  <label style="display:flex;gap:6px;flex:1;min-width:0">
                    <input type="checkbox" :checked="catsOf(post.id).includes(c.id)" @change="togglePostCat(post.id, c.id, ($event.target as HTMLInputElement).checked)" />
                    <span>{{ c.name }}</span>
                  </label>
                  <button class="btn btn-sm" type="button" :title="t('index.catRename')" :aria-label="t('index.catRename')" @click="renameCat(c.id, c.name)">✎</button>
                  <button class="btn btn-sm" type="button" :title="t('index.catDelete')" :aria-label="t('index.catDelete')" @click="removeCat(c.id, c.name)">✕</button>
                </div>
                <div style="display:flex;gap:6px;margin-top:6px">
                  <input class="field" v-model="newCatName" :placeholder="t('index.newCatPh')" maxlength="20" />
                  <button class="btn btn-sm btn-primary" type="button" @click="createCat(post.id)">{{ t('index.newCat') }}</button>
                </div>
                <label v-if="isLongPost(post)" style="display:flex;gap:6px;align-items:center;margin-top:6px;font-size:13px">
                  复刻入口
                  <select class="field" style="width:auto" :value="post.replica_enabled === true ? 'on' : post.replica_enabled === false ? 'off' : 'auto'" @change="setReplicaMode(post, ($event.target as HTMLSelectElement).value)">
                    <option value="auto">自动（{{ autoReplicable(post) ? '显示' : '隐藏' }}）</option>
                    <option value="on">启用复刻</option>
                    <option value="off">隐藏复刻</option>
                  </select>
                </label>
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
            <!-- AI 摘要：只对长文（字数 > 100）提供。点击时才调中转站 API 生成并入库（同油管），之后直接读库；可复刻帖子的流程图也在这里 -->
            <button v-if="isLongPost(post)" class="yt-summary-toggle" type="button" :aria-expanded="openSummary.has(post.id)" @click="toggleSummary(post)">
              <span>✨ {{ openSummary.has(post.id) ? '收起 AI 摘要' : 'AI 摘要' }}<template v-if="hasFlow(post)"> · {{ t('index.stepsSummary', { n: post.steps!.length }) }}</template></span>
              <svg class="yt-chevron" :class="{ 'is-collapsed': !openSummary.has(post.id) }" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            </button>
            <section v-if="isLongPost(post) && openSummary.has(post.id)" class="yt-summary" aria-label="AI 摘要">
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
        <p v-if="!loading && !totalPosts" class="empty">{{ t('index.noPosts') }}</p>
        <nav v-if="totalPosts" class="x-pager" aria-label="分页">
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
          <span class="x-pager-info">共 {{ totalPosts }} 条</span>
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
          <!-- 一键导入：粘贴关注列表（用户名 / @用户名 / 主页链接），批量加入「默认」分组 -->
          <details class="wl-bulk">
            <summary>批量导入关注列表</summary>
            <textarea class="field" v-model="bulkHandles" rows="3" placeholder="每行或用逗号分隔：alice, @bob, https://x.com/carol" />
            <div style="display:flex;gap:8px;align-items:center;margin-top:6px">
              <button class="btn btn-sm btn-primary" type="button" :disabled="!bulkHandles.trim() || bulkBusy" @click="doBulkAdd">导入到「默认」分组</button>
              <span v-if="bulkMsg" class="active-count">{{ bulkMsg }}</span>
            </div>
          </details>
          <div class="wl-list-head">
            <span>已关注 {{ watchlist.accounts.length }} 位作者</span>
            <label style="font-size:13px;display:flex;gap:4px;align-items:center"><input type="checkbox" v-model="sortByActivity" /> 按近 30 天发帖数排序</label>
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
                  <span class="chip-count" :title="'近 30 天发帖数'">30 天 {{ posts30d[a.username] ?? 0 }} 帖</span>
                  <span style="flex:1" />
                  <button class="btn btn-sm" type="button" :aria-expanded="editingAuthor === a.username" @click="editingAuthor = editingAuthor === a.username ? '' : a.username">{{ editingAuthor === a.username ? '收起' : '编辑' }}</button>
                </div>
                <div v-if="editingAuthor === a.username" class="wl-author-edit">
                  <label>{{ t('settings.displayNamePh') }}<input class="field" :value="a.name" @change="doPatchAccount(a.username, { name: ($event.target as HTMLInputElement).value })" /></label>
                  <label>分组<select class="field" :value="a.group" @change="doPatchAccount(a.username, { group: ($event.target as HTMLSelectElement).value })">
                    <option v-for="g in watchlist.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
                  </select></label>
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
          <h3>导出 / 导入设置</h3>
          <p style="color:var(--text-muted);font-size:13px;margin:0">把主题、字体、语言、筛选记忆导出成 JSON，在另一台设备导入（不含 API Token 等密钥）。</p>
          <div class="modal-actions" style="justify-content:flex-start">
            <button class="btn btn-sm" type="button" @click="doExportSettings">导出设置</button>
            <label class="btn btn-sm" style="cursor:pointer">导入设置<input type="file" accept="application/json,.json" style="display:none" @change="doImportSettings" /></label>
          </div>
        </div>

        <div class="modal-actions">
          <button class="btn btn-primary" type="button" @click="showSettings = false">{{ t('settings.done') }}</button>
        </div>
      </div>
    </div>
    <XImportDialog v-if="xDialog" :mode="xDialog" :accounts="watchlist.accounts" @close="xDialog = null" @changed="reloadPosts" />
    <p v-if="toast" style="position:fixed;bottom:16px;left:50%;transform:translateX(-50%);background:var(--bg-elevated);border:1px solid var(--border);padding:10px 16px;border-radius:9999px;z-index:200;display:flex;gap:10px;align-items:center">{{ toast }}<button v-if="toastAction" class="btn btn-sm btn-primary" type="button" @click="runToastAction">{{ toastAction.label }}</button></p>
  </div>
</template>

<script setup lang="ts">
import type { Post, Watchlist, WatchlistAccount, Categories, YTSummary, SavedView } from '~/composables/types'
import { LS, useSettings } from '~/composables/useSettings'
import { canReplicatePost } from '~/utils/x-post.mjs'
import { splitParagraphs } from '~/utils/paragraphs'
import { followSummary } from '~/utils/summaryStream'
import { exportSettings, importSettings } from '~/utils/settingsTransfer'
import { normalizeFilter, sameFilter, type FilterState } from '~/utils/filterView'
import { apiBase } from '~/composables/useApi'
import { avatarUrl, avatarColor, avatarInitial, relativeTime, shanghaiTime, highlightHtml, firstHitSnippet, shouldShowFlowchart, trunc40, isLongPost } from '~/composables/useFormat'
import { useScrollLock } from '~/composables/useScrollLock'
import { linkParts, descResources } from '~/utils/videoTags'

const store = useStore()
const settings = useSettings()
const { t, lang } = useLang()
const { tr, enqueue, translating } = useTranslate()

const loading = ref(true)
const loadError = ref('')
const posts = ref<Post[]>([])
const totalPosts = ref(0)
const tagNames = ref<string[]>([])
const updatedAt = ref('')
const watchlist = ref<Watchlist>({ groups: [], accounts: [] })
const categories = ref<Categories>({ categories: [], assignments: {} })
const replicaCount = ref(0)
const replicatedIds = ref<Set<string>>(new Set())

// 搜索区的条件（草稿）：改动不会立即检索，点「搜索」时复制到 applied 再加载。
const keyword = ref('')
const selectedAuthors = ref<Set<string>>(new Set())
const selectedTags = ref<Set<string>>(new Set())
const selectedCats = ref<Set<string>>(new Set())
const minChars = ref(0)
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
// Server-side load: fetch the current page with the applied (searched) filters.
async function loadPostsPage () {
  const f = applied.value
  const p = await store.loadPosts({
    authors: f.authors,
    tags: f.tags,
    categories: f.categories,
    keyword: f.keyword,
    kind: f.kind,
    minChars: f.min_chars,
    page: page.value,
    pageSize: pageSize.value,
  })
  posts.value = p.posts; totalPosts.value = p.total; updatedAt.value = p.updated_at
}
// After an import / refresh: posts and the watchlist (new authors) reload.
async function reloadPosts () {
  try {
    await loadPostsPage()
    await refreshWatchlist()
  } catch { /* the next action retries */ }
}
const toast = ref('')
const newGroupName = ref('')
const newAccount = ref({ username: '', name: '', group: 'other' })

let authorsInitialised = false
// Restore the stored author/category selections (once). Runs at setup, before
// any watcher exists, so the first server load already uses them.
function restoreSelection () {
  if (authorsInitialised) return
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
restoreSelection()
let toastTimer: any = null
// toastAction: optional button on the toast (e.g. 撤销), shown for `ms`.
const toastAction = ref<{ label: string, run: () => void } | null>(null)
function showToast (msg: string, action?: { label: string, run: () => void }, ms = 3000) {
  clearTimeout(toastTimer)
  toast.value = msg
  toastAction.value = action || null
  toastTimer = setTimeout(() => { toast.value = ''; toastAction.value = null }, ms)
}
function runToastAction () {
  const a = toastAction.value
  toast.value = ''; toastAction.value = null
  a?.run()
}
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
const catsOf = (postId: string) => categories.value.assignments[postId] || []
const categoryName = (id: string) => categories.value.categories.find(c => c.id === id)?.name || id
// Keyword highlighting only (filtering is server-side): the searched keyword.
const plainKeyword = computed(() => applied.value.keyword)
// ---- 帖子类型（cmd/digest 的 AI 分类；没分类的旧帖为「未分类」） ----
const POST_KINDS = ['教程实操', '技术分享', '工具资源推荐', '观点讨论', '日常闲聊']
const kindFilter = ref('')
function kindOf (p: Post): string { return p.kind || '未分类' }
const kindCounts = computed(() => [...POST_KINDS, '未分类'].map(kind => ({ kind })))
// 复刻：AI 判为「教程实操」才可复刻；没分类的旧帖沿用关键词规则 canReplicatePost。
function autoReplicable (p: Post): boolean { return p.kind ? p.kind === '教程实操' : canReplicatePost(p) }
// 只有长文（字数 > 100）才可复刻；其中手动开关（posts.replica_enabled）优先，没设置时按自动规则。
function isReplicable (p: Post): boolean {
  if (!isLongPost(p)) return false
  return typeof p.replica_enabled === 'boolean' ? p.replica_enabled : autoReplicable(p)
}
async function setReplicaMode (p: Post, mode: string) {
  const v = mode === 'on' ? true : mode === 'off' ? false : null
  try {
    await store.setPostReplicaEnabled(p.id, v)
    p.replica_enabled = v
  } catch (e: any) { showToast(e?.message || '保存失败') }
}
// ---- 分页：每页 10 / 20 / 50 条（记在本机），筛选变化回到第 1 页 ----
const PAGE_SIZES = [10, 20, 50]
const PAGE_SIZE_KEY = 'x-digest-x-page-size'
const pageSize = ref(20)
const page = ref(1)
const pageCount = computed(() => Math.max(1, Math.ceil(totalPosts.value / pageSize.value)))
const pagedPosts = computed(() => posts.value)
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
// ---- 搜索：条件改动只改草稿，点「搜索」/回车才检索；「重置」清空后检索全部 ----
// 当前草稿条件（同保存视图的格式）。
function currentFilter (): FilterState {
  return normalizeFilter({
    keyword: keyword.value.trim(), kind: kindFilter.value, min_chars: minChars.value,
    authors: [...selectedAuthors.value], tags: [...selectedTags.value], categories: [...selectedCats.value]
  })
}
// 已生效（最近一次搜索）的条件：列表、分页、高亮都按它。
const applied = ref<FilterState>(currentFilter())
const searchDirty = computed(() => !sameFilter(currentFilter(), applied.value))
function runSearch () {
  applied.value = currentFilter()
  persistSelection()
  if (page.value !== 1) page.value = 1 // the page watcher reloads
  else reloadPosts()
}
function resetFilters () {
  keyword.value = ''; kindFilter.value = ''; minChars.value = 0
  selectedAuthors.value = new Set(); selectedTags.value = new Set(); selectedCats.value = new Set()
  runSearch()
}
watch([page, pageSize], () => reloadPosts())
watch(totalPosts, () => { if (page.value > pageCount.value) page.value = 1 })
const activeFilterCount = computed(() => {
  const f = applied.value
  return f.authors.length + f.tags.length + f.categories.length + (f.keyword ? 1 : 0) + (f.kind ? 1 : 0) + (f.min_chars ? 1 : 0)
})
const firstSnippet = computed(() => {
  if (!applied.value.keyword || !posts.value.length) return ''
  return firstHitSnippet(posts.value[0], applied.value.keyword)
})
// 卡片上点标签 / 类型：直接按它检索（明确的点击动作，不必再点搜索）。
function selectTagOnly (t: string) { selectedTags.value = new Set([t]); runSearch() }
function searchKind (kind: string) { kindFilter.value = applied.value.kind === kind ? '' : kind; runSearch() }
// 下拉联想框用数组 v-model，内部筛选状态仍是 Set。
function setBridge (refSet: Ref<Set<string>>) {
  return computed<string[]>({
    get: () => [...refSet.value],
    set: (v) => { refSet.value = new Set(v) }
  })
}
// ---- 保存的筛选视图（/api/v1/views）：一键切换常用筛选组合 ----
const savedViews = ref<SavedView[]>([])
const currentViewId = computed(() => savedViews.value.find(v => sameFilter(normalizeFilter(v.filter), currentFilter()))?.id || '')
function applyView (id: string) {
  const v = savedViews.value.find(x => x.id === id)
  if (!v) return
  const f = normalizeFilter(v.filter)
  keyword.value = f.keyword; kindFilter.value = f.kind; minChars.value = f.min_chars
  selectedAuthors.value = new Set(f.authors); selectedTags.value = new Set(f.tags); selectedCats.value = new Set(f.categories)
  runSearch()
}
async function refreshViews () { try { savedViews.value = await store.loadViews() } catch { /* views are optional */ } }
async function saveCurrentView () {
  const name = window.prompt(t('index.viewNamePrompt'))?.trim()
  if (!name) return
  try { await store.createView(name, currentFilter()); await refreshViews(); showToast(t('index.viewSaved', { name })) } catch (e: any) {
    showToast(e?.status === 409 ? t('index.viewNameTaken') : (e?.message || t('index.viewSaveFailed')))
  }
}
async function deleteCurrentView () {
  const v = savedViews.value.find(x => x.id === currentViewId.value)
  if (!v || !window.confirm(t('index.viewDeleteConfirm', { name: v.name }))) return
  try { await store.deleteView(v.id); await refreshViews() } catch (e: any) { showToast(e?.message || t('index.viewSaveFailed')) }
}
const selectedAuthorsArr = setBridge(selectedAuthors)
const selectedTagsArr = setBridge(selectedTags)
const selectedCatsArr = setBridge(selectedCats)
// 各条件框的下拉选项（值多的字段靠输入联想过滤，见 FilterCombo）。
const authorOptions = computed(() => {
  const listed = watchlist.value.accounts.map(a => ({ value: a.username, label: `${a.name} @${a.username}` }))
  const unlisted = unlistedAuthors.value.map(u => ({ value: u, label: `${authorName(u)} @${u}` }))
  return [...listed, ...unlisted]
})
const tagOptions = computed(() => tagNames.value.map(t => ({ value: t, label: t })))
const catOptions = computed(() => [
  ...categories.value.categories.map(c => ({ value: c.id, label: c.name })),
  { value: '__uncat', label: t('index.uncategorized') }
])
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
// Enqueue the current page's display texts for translation
// (debounced; the queue flushes in chunks of 20). Translation runs toward
// the current UI language; filtering always uses the original text —
// only display goes through tr().
let enqueueTimer: any = null
const bilingual = ref<Set<string>>(new Set())
function toggleBilingual (p: Post) {
  const s = new Set(bilingual.value)
  if (s.has(p.id)) s.delete(p.id)
  else { s.add(p.id); enqueue(splitParagraphs(p.text || '')) }
  bilingual.value = s
}
function paragraphsOf (text: string) { return splitParagraphs(text) }
function enqueueVisible () {
  const texts: string[] = []
  for (const p of posts.value) {
    texts.push(p.title)
    for (const s of p.steps || []) {
      texts.push(s.text)
      for (const sub of s.substeps || []) texts.push(sub.text)
    }
  }
  enqueue(texts)
}
watch([posts, lang], () => {
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
// 生成中：优先用 SSE 等服务端推送结果，连接失败时退回每 4 秒轮询。
const streaming = new Set<string>()
function schedulePoll (p: Post) {
  const key = sumKey(p.id)
  if (streaming.has(key) || pollTimers.has(key)) return
  const l = lang.value
  if (!store.mock.value && followSummary(`${apiBase()}/posts/${encodeURIComponent(p.id)}/summary/stream?lang=${l}`, (sum) => { setSummary(sum); if (sum.status !== 'pending') streaming.delete(key) }, () => { streaming.delete(key); pollSummary(p) })) {
    streaming.add(key)
    return
  }
  pollSummary(p)
}
function pollSummary (p: Post) {
  const key = sumKey(p.id)
  if (pollTimers.has(key)) return
  const l = lang.value
  pollTimers.set(key, setTimeout(async () => {
    pollTimers.delete(key)
    try {
      const sum = await store.getPostSummary(p.id, l)
      setSummary(sum)
      if (sum.status === 'pending' && l === lang.value) pollSummary(p)
    } catch { if (l === lang.value) pollSummary(p) }
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
// ---- 多选：批量归类、批量生成摘要 ----
const selectMode = ref(false)
const selectedPosts = ref<Set<string>>(new Set())
const batchCat = ref('')
const batchBusy = ref(false)
const batchMsg = ref('')
function toggleSelectMode () { selectMode.value = !selectMode.value; selectedPosts.value = new Set(); batchMsg.value = '' }
function togglePostSelected (id: string) {
  const s = new Set(selectedPosts.value)
  s.has(id) ? s.delete(id) : s.add(id)
  selectedPosts.value = s
}
function selectPage () { selectedPosts.value = new Set([...selectedPosts.value, ...pagedPosts.value.map(p => p.id)]) }
async function batchCategory (action: 'add' | 'remove') {
  batchBusy.value = true
  try {
    const r = await store.batchCategory([...selectedPosts.value], batchCat.value, action)
    await refreshCategories()
    batchMsg.value = `${action === 'add' ? '归入' : '移出'}完成：${r.changed} 条变更`
  } catch (e: any) { batchMsg.value = e?.message || '批量归类失败' } finally { batchBusy.value = false }
}
async function batchSummaries () {
  // AI 摘要只对长文提供：短帖不送去生成。
  const ids = posts.value.filter(p => selectedPosts.value.has(p.id) && isLongPost(p)).map(p => p.id)
  const short = selectedPosts.value.size - ids.length
  if (!ids.length) { batchMsg.value = '所选都是短帖（≤ 100 字），没有 AI 摘要'; return }
  if (ids.length > 30) { batchMsg.value = '一次最多 30 帖'; return }
  batchBusy.value = true
  try {
    const r = await store.batchPostSummaries(ids, lang.value)
    batchMsg.value = `已排队 ${r.queued} 帖，跳过 ${r.skipped} 帖（已有摘要）` + (short ? `，${short} 帖为短帖未生成` : '')
    // Open the queued ones so their panels follow progress (SSE / poll).
    const open = new Set(openSummary.value)
    for (const p of posts.value) {
      if (r.results[p.id] === 'started') { open.add(p.id); setSummary({ video_id: p.id, lang: lang.value, status: 'pending', summary: '', key_points: [], chapters: [], resources: [], source: '', transcript_lang: '', model: '', error: '' }); schedulePoll(p) }
    }
    openSummary.value = open
  } catch (e: any) { batchMsg.value = e?.message || '批量生成失败' } finally { batchBusy.value = false }
}
// ---- 删除帖子：单个（卡片「删除」）与批量（多选「删除所选」）。删除后不会被自动更新/导入找回，
// 需要时可用「导入单帖」重新导入。
const deleting = ref<Set<string>>(new Set())
async function removePosts (ids: string[]) {
  const r = await store.deletePosts(ids)
  const sel = new Set(selectedPosts.value)
  for (const id of ids) sel.delete(id)
  selectedPosts.value = sel
  await loadPostsPage()
  return r.deleted
}
async function deletePost (p: Post) {
  const title = (p.title || p.text || p.id).slice(0, 40)
  if (!window.confirm(`删除这条帖子？\n「${title}」\n删除后不会再被自动更新拉回，如需恢复可用「导入单帖」重新导入。`)) return
  deleting.value = new Set([...deleting.value, p.id])
  try {
    await removePosts([p.id])
    showToast('已删除 1 条帖子')
  } catch (e: any) { showToast(e?.message || '删除失败') } finally {
    const s = new Set(deleting.value); s.delete(p.id); deleting.value = s
  }
}
async function deleteSelected () {
  const ids = [...selectedPosts.value]
  if (!ids.length || !window.confirm(`删除选中的 ${ids.length} 条帖子？删除后不会再被自动更新拉回。`)) return
  batchBusy.value = true
  try {
    const n = await removePosts(ids)
    batchMsg.value = `已删除 ${n} 条帖子`
  } catch (e: any) { batchMsg.value = e?.message || '批量删除失败' } finally { batchBusy.value = false }
}
async function renameCat (id: string, old: string) {
  const name = window.prompt(t('index.catRenamePrompt'), old)?.trim()
  if (!name || name === old) return
  try { await store.renameCategory(id, name); await refreshCategories() } catch (e: any) {
    showToast(e?.status === 409 ? t('index.catNameTaken') : (e?.message || t('index.catRenameFailed')))
  }
}
// Soft delete with a 10-second undo (backend keeps it 7 days).
async function removeCat (id: string, name: string) {
  try { await store.deleteCategory(id); await refreshCategories() } catch (e: any) { showToast(e?.message || t('index.catDeleteFailed')); return }
  showToast(t('index.catDeleted', { name }), {
    label: t('index.undo'),
    run: async () => {
      try { await store.restoreCategory(id); await refreshCategories() } catch (e: any) { showToast(e?.message || t('index.catRestoreFailed')) }
    }
  }, 10000)
}
async function refreshWatchlist () {
  watchlist.value = await store.loadWatchlist()
  restoreSelection()
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
  const byActivity = (x: WatchlistAccount, y: WatchlistAccount) => (posts30d.value[y.username] ?? 0) - (posts30d.value[x.username] ?? 0)
  return groups.map(g => {
    const accounts = watchlist.value.accounts.filter(a => (g.id === '__none' ? !known.has(a.group) : a.group === g.id) && match(a))
    return { id: g.id, name: g.name, accounts: sortByActivity.value ? [...accounts].sort(byActivity) : accounts }
  }).filter(g => g.accounts.length)
})
// 活跃度：近 30 天发帖数（GET /watchlist?with_stats=1），可按它排序。
const posts30d = ref<Record<string, number>>({})
const sortByActivity = ref(false)
async function refreshActivity () { try { posts30d.value = await store.loadWatchlistActivity() } catch { /* optional */ } }
watch(showSettings, (v) => { if (v) refreshActivity() })
const bulkHandles = ref('')
const bulkBusy = ref(false)
const bulkMsg = ref('')
async function doBulkAdd () {
  bulkBusy.value = true; bulkMsg.value = ''
  try {
    const r = await store.bulkAddAccounts(bulkHandles.value)
    bulkMsg.value = `新增 ${r.created.length}，已存在 ${r.existing.length}` + (r.invalid.length ? `，无效 ${r.invalid.length}（${r.invalid.slice(0, 3).join('、')}）` : '')
    if (r.created.length) { bulkHandles.value = ''; await refreshWatchlist() }
  } catch (e: any) { bulkMsg.value = e?.message || '导入失败' } finally { bulkBusy.value = false }
}
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
function doExportSettings () {
  const blob = new Blob([exportSettings(localStorage)], { type: 'application/json' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `xdigest-settings-${new Date().toISOString().slice(0, 10)}.json`
  a.click()
  setTimeout(() => URL.revokeObjectURL(a.href), 1000)
}
async function doImportSettings (e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const n = importSettings(await file.text(), localStorage)
    showToast(`已导入 ${n} 项设置，正在刷新…`)
    setTimeout(() => location.reload(), 800)
  } catch (err: any) { showToast(err?.message || '导入失败') }
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
  try {
    // Selections were already restored at setup; the posts load (first element)
    // runs in parallel with the other loads and uses the restored filters.
    const [, w, c, reps, tags] = await Promise.all([
      loadPostsPage(), store.loadWatchlist(), store.loadCategories(), store.loadReplicas('time').catch(() => [] as any[]), store.loadTags()
    ])
    watchlist.value = w; categories.value = c; tagNames.value = tags
    replicatedIds.value = new Set((reps as any[]).map(r => r.post_id))
    refreshViews()
    replicaCount.value = (reps as any[]).length
    await refreshWatchlist()
  } catch (err: any) { loadError.value = err?.message || t('common.loadError') }
  finally { loading.value = false }
})
</script>

<style scoped>
.search-box { border: 1px solid var(--border); border-radius: 12px; padding: 12px; margin-top: 8px; background: var(--bg-elevated); }
.search-box .cond-wide { flex: 1 1 100%; }
.search-actions { margin-top: 10px; gap: 8px; }
.bilingual { margin: 6px 0; border-left: 3px solid var(--border); padding-left: 10px; }
.bilingual .bi-orig { margin: 6px 0 2px; white-space: pre-wrap; }
.bilingual .bi-tr { margin: 0 0 8px; color: var(--text-muted); white-space: pre-wrap; }
.select-box { display: inline-flex; gap: 6px; align-items: center; font-size: 13px; color: var(--text-muted); margin-bottom: 6px; }
.post-card.is-selected { outline: 2px solid var(--accent, #1d9bf0); }
</style>
