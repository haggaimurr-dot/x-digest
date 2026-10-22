(() => {
  "use strict";

  const STORAGE_KEY = "x-digest-watchlist";
  const SELECTED_KEY = "x-digest-selected-authors";
  const SELECTED_CATS_KEY = "x-digest-selected-cats";
  const POST_CATS_KEY = "x-digest-post-categories";
  const CHIPS_CATS_EXPANDED_KEY = "x-digest-chips-cats-expanded";
  const UNCATEGORIZED = "__uncategorized__";
  const SPECIAL_GROUP_ID = "special";
  const SPECIAL_GROUP_NAME = "特别关注";
  const DEFAULT_GROUPS = [
    { id: SPECIAL_GROUP_ID, name: SPECIAL_GROUP_NAME, fixed: true },
    { id: "other", name: "默认", fixed: true },
  ];
  const DEFAULT_GROUP_ID = "other";
  const DEFAULT_GROUP_NAME = "默认";
  const LONG_PRESS_MS = 600;
  const THEME_KEY = "x-digest-theme";
  const FILTER_COLLAPSE_KEY = "x-digest-filters-collapsed";
  const CHIPS_AUTHORS_EXPANDED_KEY = "x-digest-chips-authors-expanded";
  const CHIPS_TAGS_EXPANDED_KEY = "x-digest-chips-tags-expanded";
  const CHIPS_COLLAPSED_MAX_PX = 84;
  const WEBHOOK_URL_KEY = "x-digest-webhook-url";
  const WEBHOOK_KEY_KEY = "x-digest-webhook-key";
  const REPLICAS_KEY = "xdigest.replicas";
  const UPDATE_DEBOUNCE_MS = 30000;
  const UPDATE_STATUS_MSG =
    "已发送更新请求，一两分钟后刷新页面看结果；完成后我会在聊天里通知你";
  const DEFAULT_WATCHLIST = [{ username: "ai_xiaomu", name: "黄小木" }];
  const AVATAR_COLORS = [
    "var(--avatar-1)",
    "var(--avatar-2)",
    "var(--avatar-3)",
    "var(--avatar-4)",
    "var(--avatar-5)",
  ];

  /**
   * Resolve data URLs against the site root (/x-digest/), not the current path.
   * Works with <base href="/x-digest/"> and as a fallback without it.
   */
  function siteBaseUrl() {
    // Prefer the directory that served app.js (immune to missing trailing slash).
    const scripts = document.getElementsByTagName("script");
    for (let i = scripts.length - 1; i >= 0; i--) {
      const src = scripts[i].src || "";
      if (src.includes("app.js")) {
        return new URL(".", src);
      }
    }
    const baseEl = document.querySelector("base[href]");
    if (baseEl) {
      try {
        return new URL(baseEl.getAttribute("href"), window.location.origin);
      } catch (_) {
        /* fall through */
      }
    }
    // Fallback: directory of current page
    let path = window.location.pathname || "/";
    if (!path.endsWith("/")) {
      path = path.replace(/\/[^/]*$/, "/");
      if (!path.endsWith("/")) path += "/";
    }
    return new URL(path, window.location.origin);
  }

  function asset(relPath) {
    return new URL(relPath.replace(/^\//, ""), siteBaseUrl()).href;
  }

  const els = {
    lastUpdated: document.getElementById("lastUpdated"),
    keyword: document.getElementById("keyword"),
    dateFrom: document.getElementById("dateFrom"),
    dateTo: document.getElementById("dateTo"),
    userFilter: document.getElementById("userFilter"),
    tagFilter: document.getElementById("tagFilter"),
    authorChips: document.getElementById("authorChips"),
    tagChips: document.getElementById("tagChips"),
    authorChipsToggle: document.getElementById("authorChipsToggle"),
    tagChipsToggle: document.getElementById("tagChipsToggle"),
    catChips: document.getElementById("catChips"),
    catChipsToggle: document.getElementById("catChipsToggle"),
    catList: document.getElementById("catList"),
    newCatName: document.getElementById("newCatName"),
    addCat: document.getElementById("addCat"),
    filterBar: document.getElementById("filterBar"),
    filterToggle: document.getElementById("filterToggle"),
    filterPanel: document.getElementById("filterPanel"),
    resultCount: document.getElementById("resultCount"),
    activeFilterCount: document.getElementById("activeFilterCount"),
    clearFilters: document.getElementById("clearFilters"),
    timeline: document.getElementById("timeline"),
    emptyState: document.getElementById("emptyState"),
    themeToggle: document.getElementById("themeToggle"),
    openSettings: document.getElementById("openSettings"),
    closeSettings: document.getElementById("closeSettings"),
    doneSettings: document.getElementById("doneSettings"),
    settingsModal: document.getElementById("settingsModal"),
    undoToast: document.getElementById("undoToast"),
    undoToastMsg: document.getElementById("undoToastMsg"),
    undoToastBtn: document.getElementById("undoToastBtn"),
    confirmModal: document.getElementById("confirmModal"),
    confirmTitle: document.getElementById("confirmTitle"),
    confirmBody: document.getElementById("confirmBody"),
    confirmCancel: document.getElementById("confirmCancel"),
    watchList: document.getElementById("watchList"),
    addAuthor: document.getElementById("addAuthor"),
    addStatus: document.getElementById("addStatus"),
    newUsername: document.getElementById("newUsername"),
    newDisplayName: document.getElementById("newDisplayName"),
    resetWatchlist: document.getElementById("resetWatchlist"),
    groupList: document.getElementById("groupList"),
    newGroupName: document.getElementById("newGroupName"),
    addGroup: document.getElementById("addGroup"),
    triggerUpdate: document.getElementById("triggerUpdate"),
    webhookUrl: document.getElementById("webhookUrl"),
    webhookKey: document.getElementById("webhookKey"),
    saveWebhook: document.getElementById("saveWebhook"),
    clearWebhook: document.getElementById("clearWebhook"),
    webhookStatus: document.getElementById("webhookStatus"),
    webhookUrlError: document.getElementById("webhookUrlError"),
    webhookKeyError: document.getElementById("webhookKeyError"),
    updateToast: document.getElementById("updateToast"),
    openReplicas: document.getElementById("openReplicas"),
    feedMain: document.querySelector("main.feed-col:not(.replicas-col)"),
    replicasPage: document.getElementById("replicasPage"),
    replicaList: document.getElementById("replicaList"),
    replicaEmpty: document.getElementById("replicaEmpty"),
    replicaCount: document.getElementById("replicaCount"),
    replicaSort: document.getElementById("replicaSort"),
  };

  const USER_TAGS_KEY = "x-digest-user-tags";
  let userTagVocabulary = [];
  let userTagOverrides = {};
  let selectedUserTags = new Set();
  let allPosts = [];
  let watchlist = [];
  // Post categories: { categories: [{id,name}], assignments: { [postId]: [catId] } }
  let postCats = { categories: [], assignments: {} };
  let selectedCats = new Set();
  let openCatMenuPost = null;
  let groups = DEFAULT_GROUPS.map((g) => ({ ...g }));
  let repoGroups = null;
  let selectedAuthors = new Set();
  let selectedTags = new Set();
  let repoWatchlist = null;
  /** @type {Record<string, string>} username -> avatar URL (local or remote) */
  let avatarMap = {};
  /** @type {Array<Object>} replicated flows, persisted to REPLICAS_KEY */
  let replicas = [];
  let replicaSortMode = "time";

  function escapeHtml(str) {
    return String(str ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function searchQuery() {
    return (els.keyword && els.keyword.value ? els.keyword.value : "").trim();
  }

  // Escape text and wrap every case-insensitive occurrence of `q` in <mark class="search-hit">.
  // Highlight keeps the surrounding text color (WCAG AA); only the background changes.
  function highlightHtml(text, q) {
    const raw = String(text ?? "");
    if (!q) return escapeHtml(raw);
    const lower = raw.toLowerCase();
    const needle = q.toLowerCase();
    let out = "";
    let i = 0;
    while (i < raw.length) {
      const j = lower.indexOf(needle, i);
      if (j < 0) {
        out += escapeHtml(raw.slice(i));
        break;
      }
      out += escapeHtml(raw.slice(i, j));
      out += `<mark class="search-hit">${escapeHtml(raw.slice(j, j + q.length))}</mark>`;
      i = j + (q.length || 1);
    }
    return out;
  }

  // One-line snippet: ~20 Unicode chars before & after the first hit in `text`.
  function matchSnippetHtml(text, q) {
    const raw = String(text ?? "");
    if (!q || !raw) return "";
    const j = raw.toLowerCase().indexOf(q.toLowerCase());
    if (j < 0) return "";
    const before = [...raw.slice(0, j)];
    const hit = raw.slice(j, j + q.length);
    const after = [...raw.slice(j + q.length)];
    const pre = before.slice(-20).join("");
    const post = after.slice(0, 20).join("");
    const lead = before.length > 20 ? "…" : "";
    const trail = after.length > 20 ? "…" : "";
    return `<p class="match-snippet">${lead}${highlightHtml(pre + hit + post, q)}${trail}</p>`;
  }

  // First hit across title → body → tags → steps (document order for the card).
  function firstHitSnippet(p, q) {
    if (!q) return "";
    const ql = q.toLowerCase();
    if (String(p.title || "").toLowerCase().includes(ql)) return matchSnippetHtml(p.title, q);
    if (String(p.summary || "").toLowerCase().includes(ql)) return matchSnippetHtml(p.summary, q);
    for (const t of postTags(p)) {
      if (String(t).toLowerCase().includes(ql)) return matchSnippetHtml(t, q);
    }
    for (const s of Array.isArray(p.steps) ? p.steps : []) {
      const texts =
        typeof s === "string"
          ? [s]
          : [s && s.text, s && s.tools, s && s.tech, s && s.prep]
              .concat((s && Array.isArray(s.substeps) ? s.substeps : []).map((x) => x && x.text))
              .filter(Boolean);
      for (const t of texts) {
        if (String(t).toLowerCase().includes(ql)) return matchSnippetHtml(t, q);
      }
    }
    return "";
  }

  // After render: expand the collapsed region that holds the first hit, then scroll it into view.
  function revealFirstSearchHit() {
    if (!searchQuery() || !els.timeline) return;
    // Prefer an in-content hit (title / body / tags / steps), not the match-snippet under the title.
    // Prefer a substep hit when present so nested panels open for step-body matches.
    const hit =
      els.timeline.querySelector(".substep-text .search-hit") ||
      els.timeline.querySelector(
        ".post-title .search-hit, .post-summary .search-hit, .tags .search-hit, .flow-text .search-hit"
      ) ||
      els.timeline.querySelector(".search-hit");
    if (!hit) return;
    let node = hit.parentElement;
    while (node && node !== els.timeline) {
      if (node.classList && node.classList.contains("collapsible") && !node.classList.contains("is-expanded")) {
        node.classList.add("is-expanded");
        node.dataset.collapsed = "false";
        const btn = node.querySelector(":scope > .collapse-toggle");
        if (btn) {
          btn.setAttribute("aria-expanded", "true");
          btn.textContent = "收起";
        }
      }
      if (node.classList && node.classList.contains("substeps") && !node.classList.contains("is-open")) {
        openSubsteps(node, true);
      }
      if (node.tagName === "DETAILS" && !node.open) node.open = true;
      node = node.parentElement;
    }
    // Also open every substeps block in this card that contains a hit (parent text may match too).
    const card = hit.closest(".post-card");
    if (card) {
      card.querySelectorAll(".substeps").forEach((wrap) => {
        if (wrap.querySelector(".search-hit")) openSubsteps(wrap, true);
      });
    }
    // Wait one frame so expansion has laid out, then scroll if the hit is off-screen.
    requestAnimationFrame(() => {
      const rect = hit.getBoundingClientRect();
      const pad = 48;
      const fullyVisible = rect.top >= pad && rect.bottom <= window.innerHeight - pad;
      if (!fullyVisible) {
        hit.scrollIntoView({ block: "center", behavior: "smooth" });
      }
    });
  }

  /** Collapse long text in UI without truncating the stored string. */
  const SUMMARY_COLLAPSE_CHARS = 120;
  const STEP_COLLAPSE_CHARS = 80;

  function collapsibleHtml(fullText, { className, threshold, query }) {
    const text = String(fullText ?? "");
    if (!text) return "";
    const body = query ? highlightHtml(text, query) : escapeHtml(text);
    const needs = [...text].length > threshold || text.split("\n").length > 4;
    if (!needs) {
      return `<p class="${className}">${body}</p>`;
    }
    return `<div class="collapsible ${className}-wrap" data-collapsed="true">
      <p class="${className} collapsible-text">${body}</p>
      <button type="button" class="collapse-toggle" aria-expanded="false">显示更多</button>
    </div>`;
  }

  function bindCollapseToggles(root) {
    if (!root) return;
    root.querySelectorAll(".collapse-toggle").forEach((btn) => {
      if (btn.dataset.bound === "1") return;
      btn.dataset.bound = "1";
      btn.addEventListener("click", (e) => {
        e.preventDefault();
        e.stopPropagation();
        const wrap = btn.closest(".collapsible");
        if (!wrap) return;
        const expanded = wrap.classList.toggle("is-expanded");
        wrap.dataset.collapsed = expanded ? "false" : "true";
        btn.setAttribute("aria-expanded", expanded ? "true" : "false");
        btn.textContent = expanded ? "收起" : "显示更多";
      });
    });
  }

  function openSubsteps(wrap, open) {
    if (!wrap) return;
    const btn = wrap.querySelector(":scope > .substeps-toggle");
    const list = wrap.querySelector(":scope > .substep-list");
    const n = list ? list.children.length : 0;
    wrap.classList.toggle("is-open", !!open);
    if (list) list.hidden = !open;
    if (btn) {
      btn.setAttribute("aria-expanded", open ? "true" : "false");
      btn.textContent = open ? "收起子步骤" : `展开子步骤（${n}）`;
    }
  }

  function bindSubstepsToggles(root) {
    if (!root) return;
    root.querySelectorAll(".substeps-toggle").forEach((btn) => {
      if (btn.dataset.bound === "1") return;
      btn.dataset.bound = "1";
      btn.addEventListener("click", (e) => {
        e.preventDefault();
        e.stopPropagation();
        const wrap = btn.closest(".substeps");
        if (!wrap) return;
        openSubsteps(wrap, !wrap.classList.contains("is-open"));
      });
    });
  }


  function formatShanghai(iso) {
    if (!iso) return "—";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    const parts = new Intl.DateTimeFormat("zh-CN", {
      timeZone: "Asia/Shanghai",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).formatToParts(d);
    const get = (type) => parts.find((p) => p.type === type)?.value ?? "";
    return `${get("year")}-${get("month")}-${get("day")} ${get("hour")}:${get("minute")}（Asia/Shanghai）`;
  }

  function formatRelativeOrDate(iso) {
    if (!iso) return "";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    const now = Date.now();
    const diffSec = Math.round((now - d.getTime()) / 1000);
    if (diffSec < 60) return "刚刚";
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)} 分钟`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} 小时`;
    if (diffSec < 86400 * 7) return `${Math.floor(diffSec / 86400)} 天`;
    return new Intl.DateTimeFormat("zh-CN", {
      timeZone: "Asia/Shanghai",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(d);
  }


  /** Shanghai calendar day YYYY-MM-DD for an instant */
  function shanghaiYmd(iso) {
    if (!iso) return "";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return "";
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone: "Asia/Shanghai",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).formatToParts(d);
    const get = (t) => parts.find((p) => p.type === t)?.value ?? "";
    return `${get("year")}-${get("month")}-${get("day")}`;
  }

  /** Start/end of Shanghai day as epoch ms */
  function shanghaiDayBound(ymd, end) {
    if (!ymd) return null;
    const suffix = end ? "T23:59:59.999+08:00" : "T00:00:00.000+08:00";
    const t = Date.parse(`${ymd}${suffix}`);
    return Number.isNaN(t) ? null : t;
  }

  function postTags(p) {
    const out = [];
    const seen = new Set();
    const push = (t) => {
      const s = String(t || "").trim();
      if (!s || seen.has(s)) return;
      seen.add(s);
      out.push(s);
    };
    if (Array.isArray(p.tags)) p.tags.forEach(push);
    String(p.industry || "")
      .split(/[/、，,·|]+/)
      .forEach(push);
    String(p.tech || "")
      .split(/[/、，,·|]+/)
      .forEach(push);
    return out;
  }

  /**
   * Tags for chip UI: only from posts by selectedAuthors (active author chips).
   * If username search (userFilter) is set, further narrow to matching authors.
   * selectedAuthors remains the source of truth for which author chips are on.
   */
  function collectAllTags() {
    const userQ = (els.userFilter && els.userFilter.value ? els.userFilter.value : "")
      .trim()
      .replace(/^@/, "")
      .toLowerCase();
    const set = new Set();
    for (const p of allPosts) {
      // Filter posts by selectedAuthors before collecting tags
      if (!selectedAuthors.has(p.author)) continue;
      if (userQ) {
        const name = String(p.authorName || "").toLowerCase();
        const handle = String(p.author || "").toLowerCase();
        if (!name.includes(userQ) && !handle.includes(userQ)) continue;
      }
      postTags(p).forEach((t) => set.add(t));
    }
    return [...set].sort((a, b) => a.localeCompare(b, "zh-CN"));
  }

  /** Drop selectedTags that no longer appear in the author-filtered tag set. */
  function pruneSelectedTags(available) {
    const keep = new Set(available);
    for (const t of [...selectedTags]) {
      if (!keep.has(t)) selectedTags.delete(t);
    }
  }

  function countActiveFilters() {
    let n = selectedUserTags.size ? 1 : 0;
    if (els.keyword && els.keyword.value.trim()) n += 1;
    if (els.dateFrom && els.dateFrom.value) n += 1;
    if (els.dateTo && els.dateTo.value) n += 1;
    if (els.userFilter && els.userFilter.value.trim()) n += 1;
    // Only-# is empty after strip — don't count as an active filter.
    if (els.tagFilter && els.tagFilter.value.trim().replace(/^#+/, "")) n += 1;
    n += selectedTags.size;
    if (selectedCats.size) n += 1;
    const defaultAuthors = specialAuthors();
    let authorChanged = selectedAuthors.size !== defaultAuthors.size;
    if (!authorChanged) {
      for (const u of defaultAuthors) {
        if (!selectedAuthors.has(u)) {
          authorChanged = true;
          break;
        }
      }
    }
    if (!authorChanged) {
      for (const u of selectedAuthors) {
        if (!defaultAuthors.has(u)) {
          authorChanged = true;
          break;
        }
      }
    }
    if (authorChanged) n += 1;
    return n;
  }

  function updateActiveFilterCount() {
    const n = countActiveFilters();
    if (!els.activeFilterCount) return;
    if (n > 0) {
      els.activeFilterCount.hidden = false;
      els.activeFilterCount.textContent = `${n} 个筛选条件`;
    } else {
      els.activeFilterCount.hidden = true;
    }
    if (els.clearFilters) els.clearFilters.hidden = n === 0;
  }

  function initials(name, username) {
    const src = String(name || username || "?").trim();
    if (!src) return "?";
    const first = Array.from(src)[0];
    return /[A-Za-z]/.test(first) ? first.toUpperCase() : first;
  }

  function avatarColor(username) {
    let h = 0;
    const s = String(username || "");
    for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
    return AVATAR_COLORS[h % AVATAR_COLORS.length];
  }

  function hiResTwimg(url) {
    if (!url) return "";
    return String(url).replace("_normal.", "_400x400.");
  }

  function resolveAvatar(username, postAvatar) {
    const u = String(username || "").toLowerCase();
    const candidates = [postAvatar, avatarMap[u]];
    const fromWatch = watchlist.find((a) => a.username.toLowerCase() === u);
    if (fromWatch && fromWatch.avatar) candidates.push(fromWatch.avatar);
    for (const raw of candidates) {
      if (!raw) continue;
      if (/^https?:\/\//i.test(raw)) return hiResTwimg(raw);
      return asset(raw);
    }
    return "";
  }

  function avatarHtml(username, displayName, postAvatar) {
    const src = resolveAvatar(username, postAvatar);
    const label = escapeHtml(displayName || username || "");
    const ini = escapeHtml(initials(displayName, username));
    const color = avatarColor(username);
    const fallback = `<div class="avatar avatar-fallback" style="background:${color}" aria-hidden="true">${ini}</div>`;
    if (!src) return `<button type="button" class="avatar-link avatar-trigger" data-profile="${escapeHtml(username)}" aria-label="查看 ${label} 简介">${fallback}</button>`;
    return `<a class="avatar-link" data-profile="${escapeHtml(username)}" aria-haspopup="dialog" href="https://x.com/${escapeHtml(
      username
    )}" target="_blank" rel="noopener" title="@${escapeHtml(username)}">
      <img class="avatar-img" src="${escapeHtml(src)}" alt="${label}" width="40" height="40" loading="lazy" decoding="async" data-fallback-initials="${ini}" data-fallback-color="${escapeHtml(color)}" />
    </a>`;
  }



  function isFiltersCollapsed() {
    const saved = localStorage.getItem(FILTER_COLLAPSE_KEY);
    // Default collapsed when unset
    if (saved === null || saved === undefined || saved === "") return true;
    return saved !== "0" && saved !== "false";
  }

  function setFiltersCollapsed(collapsed) {
    localStorage.setItem(FILTER_COLLAPSE_KEY, collapsed ? "1" : "0");
    applyFilterCollapseUI(collapsed);
  }

  function applyFilterCollapseUI(collapsed) {
    const bar = els.filterBar;
    const toggle = els.filterToggle;
    const panel = els.filterPanel;
    if (!bar || !toggle || !panel) return;
    bar.classList.toggle("is-collapsed", collapsed);
    panel.hidden = collapsed;
    toggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
    const chev = toggle.querySelector(".filter-chevron");
    if (chev) chev.textContent = collapsed ? "▸" : "▾";
    if (!collapsed) {
      // Panel just shown — measure chip overflow after layout
      requestAnimationFrame(() => syncAllChipSections());
    }
  }

  function initFilterCollapse() {
    applyFilterCollapseUI(isFiltersCollapsed());
    if (!els.filterToggle || !els.filterBar) {
      console.error("filter toggle/bar missing — cannot wire collapse UI");
      return;
    }
    els.filterToggle.addEventListener("click", (e) => {
      e.preventDefault();
      e.stopPropagation();
      const currentlyCollapsed = els.filterBar.classList.contains("is-collapsed");
      setFiltersCollapsed(!currentlyCollapsed);
    });
  }


  function isChipSectionExpanded(key) {
    return localStorage.getItem(key) === "1" || localStorage.getItem(key) === "true";
  }

  function setChipSectionExpanded(key, expanded) {
    localStorage.setItem(key, expanded ? "1" : "0");
  }

  function chipCount(container) {
    if (!container) return 0;
    return container.querySelectorAll(".chip").length;
  }

  function measureChipsOverflow(container) {
    if (!container) return false;
    if (container.classList.contains("is-collapsed")) {
      // overflow:hidden + max-height: scrollHeight is still the full content height
      return container.scrollHeight > container.clientHeight + 1;
    }
    // Expanded: compare natural height to the collapsed budget
    return container.scrollHeight > CHIPS_COLLAPSED_MAX_PX + 1;
  }

  function syncChipSection(container, toggle, storageKey) {
    if (!container || !toggle) return;
    const n = chipCount(container);
    const expanded = isChipSectionExpanded(storageKey);
    const overflows = measureChipsOverflow(container);

    container.classList.toggle("is-collapsed", !expanded);
    container.classList.toggle("has-overflow", !expanded && overflows);

    if (!overflows) {
      toggle.hidden = true;
      // Keep storage as-is; UI just hides the control when content fits
      container.classList.remove("is-collapsed");
      container.classList.remove("has-overflow");
      toggle.setAttribute("aria-expanded", "true");
      return;
    }

    toggle.hidden = false;
    toggle.setAttribute("aria-expanded", expanded ? "true" : "false");
    toggle.textContent = expanded ? "收起" : `展开全部 (${n})`;
  }

  function syncAllChipSections() {
    syncChipSection(els.authorChips, els.authorChipsToggle, CHIPS_AUTHORS_EXPANDED_KEY);
    syncChipSection(els.tagChips, els.tagChipsToggle, CHIPS_TAGS_EXPANDED_KEY);
    syncChipSection(els.catChips, els.catChipsToggle, CHIPS_CATS_EXPANDED_KEY);
  }

  function initChipSections() {
    const wire = (toggle, storageKey) => {
      if (!toggle) return;
      toggle.addEventListener("click", (e) => {
        e.preventDefault();
        e.stopPropagation();
        const next = !isChipSectionExpanded(storageKey);
        setChipSectionExpanded(storageKey, next);
        syncAllChipSections();
      });
    };
    wire(els.authorChipsToggle, CHIPS_AUTHORS_EXPANDED_KEY);
    wire(els.tagChipsToggle, CHIPS_TAGS_EXPANDED_KEY);
    wire(els.catChipsToggle, CHIPS_CATS_EXPANDED_KEY);

    let resizeTimer = null;
    window.addEventListener("resize", () => {
      if (resizeTimer) clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => syncAllChipSections(), 100);
    });
  }

  const FONT_SIZE_KEY = "x-digest-font-size";
  const FONT_FAMILY_KEY = "x-digest-font-family";
  const FONT_STACKS = {
    system: null,
    pingfang: '"PingFang SC", "PingFang TC", ',
    noto: '"Noto Sans SC", "Source Han Sans SC", ',
    yahei: '"Microsoft YaHei", "Microsoft YaHei UI", ',
  };
  const systemFontStack = getComputedStyle(document.documentElement).getPropertyValue("--font").trim();

  function applyFontPreferences() {
    const savedSize = localStorage.getItem(FONT_SIZE_KEY);
    const size = ["13", "15", "17", "19"].includes(savedSize) ? savedSize : "15";
    const savedFamily = localStorage.getItem(FONT_FAMILY_KEY);
    const family = Object.hasOwn(FONT_STACKS, savedFamily) ? savedFamily : "system";
    const root = document.documentElement;
    root.style.setProperty("--font-size-base", `${size}px`);
    if (family === "system") root.style.removeProperty("--font");
    else root.style.setProperty("--font", FONT_STACKS[family] + systemFontStack);
    document.querySelectorAll("[data-font-size]").forEach((button) => {
      button.setAttribute("aria-pressed", String(button.dataset.fontSize === size));
    });
    document.getElementById("fontFamily").value = family;
  }

  document.querySelectorAll("[data-font-size]").forEach((button) => {
    button.addEventListener("click", () => {
      localStorage.setItem(FONT_SIZE_KEY, button.dataset.fontSize);
      applyFontPreferences();
    });
  });
  document.getElementById("fontFamily").addEventListener("change", (event) => {
    localStorage.setItem(FONT_FAMILY_KEY, event.target.value);
    applyFontPreferences();
  });
  document.getElementById("resetFont").addEventListener("click", () => {
    localStorage.removeItem(FONT_SIZE_KEY);
    localStorage.removeItem(FONT_FAMILY_KEY);
    applyFontPreferences();
  });
  applyFontPreferences();

  function loadTheme() {
    // Dark is primary. Only honor explicit "light" from localStorage.
    const saved = localStorage.getItem(THEME_KEY);
    const theme = saved === "light" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", theme);
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = theme === "light" ? "#ffffff" : "#000000";
  }

  function toggleTheme() {
    const cur = document.documentElement.getAttribute("data-theme") || "dark";
    const next = cur === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem(THEME_KEY, next);
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = next === "light" ? "#ffffff" : "#000000";
  }

  function normalizeGroups(raw) {
    const out = [
      { id: SPECIAL_GROUP_ID, name: SPECIAL_GROUP_NAME, fixed: true },
      { id: DEFAULT_GROUP_ID, name: DEFAULT_GROUP_NAME, fixed: true },
    ];
    const seen = new Set([SPECIAL_GROUP_ID, DEFAULT_GROUP_ID]);
    if (Array.isArray(raw)) {
      for (const g of raw) {
        if (!g || typeof g !== "object") continue;
        const id = String(g.id || "").trim();
        if (!id || seen.has(id)) continue;
        seen.add(id);
        out.push({ id, name: String(g.name || "").trim() || "其他关注" });
      }
    }
    return out;
  }

  function firstOtherGroupId() {
    return DEFAULT_GROUP_ID;
  }

  function isFixedGroup(id) {
    return id === SPECIAL_GROUP_ID || id === DEFAULT_GROUP_ID;
  }

  function normalizeAccounts(accounts) {
    const valid = new Set(groups.map((g) => g.id));
    return accounts.map((a) => ({
      ...a,
      group: valid.has(a.group) ? a.group : a.group === undefined && !hasGroupInfo ? SPECIAL_GROUP_ID : firstOtherGroupId(),
    }));
  }
  let hasGroupInfo = true;

  function groupOfAuthor(username) {
    const hit = watchlist.find((a) => a.username === username);
    if (hit && groups.some((g) => g.id === hit.group)) return hit.group;
    return firstOtherGroupId();
  }

  function specialAuthors() {
    return new Set(watchlist.filter((a) => a.group === SPECIAL_GROUP_ID).map((a) => a.username));
  }

  function newGroupId() {
    let id;
    do {
      id = "g" + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
    } while (groups.some((g) => g.id === id));
    return id;
  }

  function loadWatchlistFromStorage() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed?.accounts)) return parsed;
      if (Array.isArray(parsed)) return { accounts: parsed };
    } catch (_) {
      /* ignore */
    }
    return null;
  }

  function saveWatchlist(accounts) {
    dropPendingUndo();
    watchlist = accounts.slice();
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ groups, accounts: watchlist }));
  }

  function loadPostCats() {
    try {
      const raw = localStorage.getItem(POST_CATS_KEY);
      if (!raw) {
        postCats = { categories: [{ id: "fav", name: "收藏" }], assignments: {} };
        savePostCats();
        return;
      }
      const p = JSON.parse(raw);
      const cats = Array.isArray(p?.categories)
        ? p.categories.filter((c) => c && c.id).map((c) => ({ id: String(c.id), name: String(c.name || "未命名") }))
        : [];
      const valid = new Set(cats.map((c) => c.id));
      const assignments = {};
      for (const [pid, arr] of Object.entries(p?.assignments || {})) {
        const list = Array.isArray(arr) ? [...new Set(arr.map(String))].filter((c) => valid.has(c)) : [];
        if (list.length) assignments[pid] = list;
      }
      postCats = { categories: cats, assignments };
    } catch (_) {
      postCats = { categories: [], assignments: {} };
    }
  }

  function savePostCats() {
    dropPendingUndo();
    localStorage.setItem(POST_CATS_KEY, JSON.stringify(postCats));
  }

  function postKey(p) {
    return String(p.id || p.url || "");
  }

  function catsOfPost(p) {
    return postCats.assignments[postKey(p)] || [];
  }

  function catName(id) {
    const c = postCats.categories.find((x) => x.id === id);
    return c ? c.name : "";
  }

  function createCategory(name) {
    const n = String(name || "").trim();
    if (!n) return null;
    const dup = postCats.categories.find((c) => c.name === n);
    if (dup) return dup;
    let id;
    do {
      id = "c" + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
    } while (postCats.categories.some((c) => c.id === id));
    const cat = { id, name: n };
    postCats.categories.push(cat);
    savePostCats();
    return cat;
  }

  function setPostCat(pid, cid, on) {
    const cur = new Set(postCats.assignments[pid] || []);
    if (on) cur.add(cid);
    else cur.delete(cid);
    if (cur.size) postCats.assignments[pid] = [...cur];
    else delete postCats.assignments[pid];
    savePostCats();
  }

  function deleteCategory(cid) {
    postCats.categories = postCats.categories.filter((c) => c.id !== cid);
    for (const pid of Object.keys(postCats.assignments)) {
      const list = postCats.assignments[pid].filter((c) => c !== cid);
      if (list.length) postCats.assignments[pid] = list;
      else delete postCats.assignments[pid];
    }
    selectedCats.delete(cid);
  }

  // ---- Deferred delete with undo ----
  // A delete snapshots all mutable settings state, applies the change in memory + UI,
  // and only persists when the undo window ends (6s), the modal closes, the page is
  // hidden, or another delete/save supersedes it. Undo restores the snapshot.
  const UNDO_MS = 6000;
  const TOAST_FADE_MS = 150;
  // { snapshot, timer, remaining, startedAt }; timer is null while paused.
  let pendingUndo = null;
  let toastHideTimer = null; // finishes the fade-out by setting [hidden]
  let toastHovered = false;
  let toastFocused = false;

  function snapshotState() {
    return {
      json: JSON.stringify({ groups, watchlist, postCats }),
      selectedAuthors: [...selectedAuthors],
      selectedCats: [...selectedCats],
    };
  }

  function showUndoToast(message) {
    const t = els.undoToast;
    if (!t) return;
    clearTimeout(toastHideTimer); // a fade from the previous toast must not hide this one
    toastHideTimer = null;
    els.undoToastMsg.textContent = message;
    t.hidden = false;
    void t.offsetWidth; // flush so the opacity transition runs from 0
    t.classList.add("is-visible");
  }

  // Fade out (opacity 1→0, 150ms), then [hidden] removes it from view and the a11y tree.
  function hideUndoToast() {
    const t = els.undoToast;
    if (!t || t.hidden) return;
    t.classList.remove("is-visible");
    clearTimeout(toastHideTimer);
    toastHideTimer = setTimeout(() => {
      toastHideTimer = null;
      t.hidden = true;
    }, TOAST_FADE_MS);
  }

  // Countdown runs only while the toast is neither hovered nor focused.
  function startUndoCountdown() {
    if (!pendingUndo || pendingUndo.timer) return;
    pendingUndo.startedAt = Date.now();
    pendingUndo.timer = setTimeout(commitPendingDelete, pendingUndo.remaining);
  }

  function pauseUndoCountdown() {
    if (!pendingUndo || !pendingUndo.timer) return;
    clearTimeout(pendingUndo.timer);
    pendingUndo.timer = null;
    pendingUndo.remaining = Math.max(0, pendingUndo.remaining - (Date.now() - pendingUndo.startedAt));
  }

  function syncUndoCountdown() {
    if (toastHovered || toastFocused) pauseUndoCountdown();
    else startUndoCountdown();
  }

  // Forget the pending undo without writing (caller is about to write current state).
  function dropPendingUndo() {
    if (!pendingUndo) return;
    clearTimeout(pendingUndo.timer);
    pendingUndo = null;
    toastHovered = toastFocused = false;
    hideUndoToast();
  }

  // Persist the pending delete (in-memory state already reflects it).
  function commitPendingDelete() {
    if (!pendingUndo) return;
    dropPendingUndo();
    saveWatchlist(watchlist);
    savePostCats();
    saveSelection();
  }

  function undoPendingDelete() {
    if (!pendingUndo) return;
    const { snapshot: snap, onUndo } = pendingUndo;
    dropPendingUndo();
    if (onUndo) onUndo(); // restores state that lives outside the snapshot (e.g. webhook storage)
    const st = JSON.parse(snap.json);
    groups = st.groups;
    watchlist = st.watchlist;
    postCats = st.postCats;
    selectedAuthors = new Set(snap.selectedAuthors);
    selectedCats = new Set(snap.selectedCats);
    if (els.settingsModal && els.settingsModal.open) {
      renderWatchListUI();
      renderCatsUI();
    }
    applyFiltersAndRender();
  }

  // Run `mutate` (in-memory only, no saves), re-render, and offer undo for 6s.
  // `onUndo` (optional) reverts side effects that `mutate` applied outside the snapshot.
  function deleteWithUndo(message, mutate, onUndo) {
    commitPendingDelete(); // only one pending delete at a time
    const snapshot = snapshotState();
    mutate();
    renderWatchListUI();
    renderCatsUI();
    applyFiltersAndRender();
    pendingUndo = { snapshot, onUndo, timer: null, remaining: UNDO_MS, startedAt: 0 };
    showUndoToast(message);
    startUndoCountdown();
  }

  if (els.undoToast) {
    const t = els.undoToast;
    t.addEventListener("mouseenter", () => {
      toastHovered = true;
      syncUndoCountdown();
    });
    t.addEventListener("mouseleave", () => {
      toastHovered = false;
      syncUndoCountdown();
    });
    t.addEventListener("focusin", () => {
      toastFocused = true;
      syncUndoCountdown();
    });
    t.addEventListener("focusout", (e) => {
      if (t.contains(e.relatedTarget)) return;
      toastFocused = false;
      syncUndoCountdown();
    });
  }

  // Background tabs throttle timers; when the page becomes visible again, commit an
  // undo window that has already run out instead of waiting for the delayed timer.
  document.addEventListener("visibilitychange", () => {
    if (document.hidden || !pendingUndo || !pendingUndo.timer) return;
    if (Date.now() - pendingUndo.startedAt >= pendingUndo.remaining) commitPendingDelete();
  });

  // Confirm dialog for deleting non-empty categories. Resolves true on 删除.
  function confirmDelete(title, body) {
    const dlg = els.confirmModal;
    if (!dlg) return Promise.resolve(true);
    els.confirmTitle.textContent = title;
    els.confirmBody.textContent = body;
    dlg.returnValue = "";
    dlg.showModal();
    els.confirmCancel.focus();
    return new Promise((resolve) => {
      dlg.addEventListener("close", () => resolve(dlg.returnValue === "ok"), { once: true });
    });
  }

  // The single definition of "未分类", shared by chip counts and the list filter.
  function isUncategorized(p) {
    return catsOfPost(p).length === 0;
  }

  // Post-category filter: OR across the selected chips (未分类 included).
  function matchesSelectedCats(p) {
    if (!selectedCats.size) return true;
    if (selectedCats.has(UNCATEGORIZED) && isUncategorized(p)) return true;
    return catsOfPost(p).some((c) => selectedCats.has(c));
  }

  function renderCatChips() {
    if (!els.catChips) return;
    // Counts are taken over the posts that pass every *other* filter (authors, search,
    // tags, dates), so a chip's number equals the list length when it alone is selected.
    const base = filteredPosts({ ignoreCats: true });
    const counts = {};
    let unc = 0;
    for (const p of base) {
      if (isUncategorized(p)) unc += 1;
      for (const c of catsOfPost(p)) counts[c] = (counts[c] || 0) + 1;
    }
    for (const c of [...selectedCats]) {
      if (c !== UNCATEGORIZED && !postCats.categories.some((x) => x.id === c)) selectedCats.delete(c);
    }
    const chip = (id, label, n, extra) =>
      `<button type="button" class="chip chip-cat ${extra || ""} ${selectedCats.has(id) ? "active" : ""}" data-cat="${escapeHtml(
        id
      )}" aria-pressed="${selectedCats.has(id)}">${escapeHtml(label)} <span class="chip-count">${n}</span></button>`;
    els.catChips.innerHTML =
      postCats.categories.map((c) => chip(c.id, c.name, counts[c.id] || 0)).join("") +
      chip(UNCATEGORIZED, "未分类", unc, "chip-uncat");
  }

  function renderCatsUI() {
    if (!els.catList) return;
    els.catList.innerHTML = postCats.categories.length
      ? postCats.categories
          .map((c) => {
            const n = Object.values(postCats.assignments).filter((l) => l.includes(c.id)).length;
            return `<li>
          <input type="text" class="field group-name-input" data-cat-rename="${escapeHtml(c.id)}" value="${escapeHtml(
              c.name
            )}" maxlength="20" aria-label="类别名称" />
          <span class="group-count">${n} 条</span>
          <button type="button" class="btn-remove" data-cat-delete="${escapeHtml(c.id)}">删除</button>
        </li>`;
          })
          .join("")
      : '<li class="empty-li"><span class="group-count">还没有帖子类别</span></li>';
  }

  function catMenuHtml(p) {
    const pid = postKey(p);
    const mine = new Set(catsOfPost(p));
    const items = postCats.categories
      .map(
        (c) => `<label class="cat-menu-item"><input type="checkbox" data-menu-cat="${escapeHtml(c.id)}" data-pid="${escapeHtml(
          pid
        )}" ${mine.has(c.id) ? "checked" : ""} /><span>${escapeHtml(c.name)}</span></label>`
      )
      .join("");
    return `<div class="cat-menu" role="menu" data-cat-menu="${escapeHtml(pid)}">
      <div class="cat-menu-title">加入帖子类别</div>
      ${items || '<div class="cat-menu-empty">还没有类别</div>'}
      <div class="cat-menu-new">
        <input type="text" class="field" data-menu-new="${escapeHtml(pid)}" placeholder="新建类别…" maxlength="20" />
        <button type="button" class="btn-accent btn-sm" data-menu-new-btn="${escapeHtml(pid)}">新建</button>
      </div>
    </div>`;
  }

  // Persists author and post-category chip selections (one state for filter + highlight).
  function saveSelection() {
    localStorage.setItem(SELECTED_KEY, JSON.stringify([...selectedAuthors]));
    localStorage.setItem(SELECTED_CATS_KEY, JSON.stringify([...selectedCats]));
  }

  function loadSelectedCats() {
    try {
      const arr = JSON.parse(localStorage.getItem(SELECTED_CATS_KEY) || "[]");
      const valid = new Set(postCats.categories.map((c) => c.id).concat(UNCATEGORIZED));
      return new Set((Array.isArray(arr) ? arr : []).map(String).filter((c) => valid.has(c)));
    } catch (_) {
      return new Set();
    }
  }

  function loadSelection() {
    try {
      const raw = localStorage.getItem(SELECTED_KEY);
      if (!raw) return null;
      const arr = JSON.parse(raw);
      return Array.isArray(arr) ? new Set(arr.map(String)) : null;
    } catch (_) {
      return null;
    }
  }

  async function fetchJson(relPath) {
    const url = asset(relPath);
    const res = await fetch(url, { cache: "no-store" });
    if (!res.ok) {
      throw new Error(`无法加载 ${relPath}（HTTP ${res.status}）来自 ${url}`);
    }
    return res.json();
  }

  async function loadData() {
    // Load posts first (required). Watchlist is optional.
    const data = await fetchJson("data/posts.json");
    allPosts = Array.isArray(data.posts) ? data.posts.slice() : [];
    allPosts.sort((a, b) => String(b.publishedAt).localeCompare(String(a.publishedAt)));

    if (els.lastUpdated) {
      els.lastUpdated.textContent = `最近更新：${formatShanghai(data.updatedAt)}`;
    }

    try {
      const w = await fetchJson("config/watchlist.json");
      repoWatchlist = Array.isArray(w.accounts) ? w.accounts : DEFAULT_WATCHLIST;
      repoGroups = Array.isArray(w.groups) ? w.groups : null;
    } catch (err) {
      console.warn("watchlist fallback:", err);
      repoWatchlist = DEFAULT_WATCHLIST;
    }

    const vocabulary = await fetchJson("config/user-tags.json");
    userTagVocabulary = [...new Set(vocabulary.tags.filter(t => typeof t === "string" && t.trim()))];
    try {
      const saved = JSON.parse(localStorage.getItem(USER_TAGS_KEY) || "{}");
      userTagOverrides = saved && typeof saved === "object" && !Array.isArray(saved) ? saved : {};
    } catch (_) { userTagOverrides = {}; }
    const stored = loadWatchlistFromStorage();
    if (stored && stored.accounts.length) {
      hasGroupInfo = Array.isArray(stored.groups);
      groups = normalizeGroups(stored.groups || repoGroups);
      watchlist = normalizeAccounts(stored.accounts);
      hasGroupInfo = true;
      saveWatchlist(watchlist);
    } else {
      applyRepoDefault();
    }
    const sel = loadSelection();
    selectedAuthors = sel || specialAuthors();

    // Build avatar map: avatars.json + watchlist + posts
    avatarMap = {};
    try {
      const av = await fetchJson("data/avatars.json");
      const entries = av.avatars || av;
      if (entries && typeof entries === "object") {
        for (const [k, v] of Object.entries(entries)) {
          const key = String(k).toLowerCase();
          if (typeof v === "string") avatarMap[key] = v;
          else if (v && (v.avatar || v.url)) avatarMap[key] = v.avatar || v.url;
        }
      }
    } catch (err) {
      console.warn("avatars.json optional:", err);
    }
    for (const a of watchlist) {
      if (a.avatar) avatarMap[String(a.username).toLowerCase()] = a.avatar;
    }
    for (const p of allPosts) {
      if (p.authorAvatar) avatarMap[String(p.author).toLowerCase()] = p.authorAvatar;
    }
  }

  function authorDisplay(username) {
    const hit = watchlist.find((a) => a.username === username);
    if (hit) return hit.name || hit.username;
    const fromPost = allPosts.find((p) => p.author === username);
    return (fromPost && fromPost.authorName) || username;
  }

  function applyRepoDefault() {
    groups = normalizeGroups(repoGroups);
    hasGroupInfo = !!repoGroups;
    const base = normalizeAccounts((repoWatchlist || DEFAULT_WATCHLIST).slice());
    hasGroupInfo = true;
    watchlist = base;
    // Only persist once the user changes something; keep repo as source of truth
  }

  function renderChips() {
    if (!els.authorChips) return;
    const authorsFromData = [...new Set(allPosts.map((p) => p.author))];
    const usernames = [
      ...new Set([...watchlist.map((a) => a.username), ...authorsFromData]),
    ];

    const chipHtml = (u) => {
      const active = selectedAuthors.has(u) ? "active" : "";
      const label = escapeHtml(authorDisplay(u));
      const lp = groupOfAuthor(u) !== DEFAULT_GROUP_ID;
      return `<button type="button" class="chip ${active} ${lp ? "lp-able" : ""}" data-author="${escapeHtml(
        u
      )}" aria-pressed="${selectedAuthors.has(u)}" ${lp ? 'title="长按移至默认"' : ""}>${label}</button>`;
    };
    els.authorChips.innerHTML = groups
      .map((g) => {
        const members = usernames.filter((u) => groupOfAuthor(u) === g.id);
        const nSel = members.filter((u) => selectedAuthors.has(u)).length;
        const all = members.length > 0 && nSel === members.length;
        const some = nSel > 0 && !all;
        return `<div class="author-group" data-group="${escapeHtml(g.id)}">
          <label class="author-group-header">
            <input type="checkbox" class="group-toggle" data-group-toggle="${escapeHtml(g.id)}" ${
              all ? "checked" : ""
            } ${some ? 'data-indeterminate="1"' : ""} ${members.length ? "" : "disabled"} />
            <span class="author-group-name">${escapeHtml(g.name)}</span>
            <span class="author-group-count">${nSel}/${members.length}</span>
          </label>
          <div class="author-group-chips">${
            members.length ? members.map(chipHtml).join("") : '<span class="author-group-empty">暂无作者</span>'
          }</div>
        </div>`;
      })
      .join("");
    els.authorChips.querySelectorAll("[data-indeterminate]").forEach((cb) => {
      cb.indeterminate = true;
    });
    syncAllChipSections();
  }

  function renderTagChips() {
    if (!els.tagChips) return;
    const tags = collectAllTags();
    pruneSelectedTags(tags);
    // Strip leading #; only-# is treated as blank (show full chip list).
    const q = (els.tagFilter && els.tagFilter.value ? els.tagFilter.value : "")
      .trim()
      .replace(/^#+/, "")
      .toLowerCase();
    // No max-cap / ellipsis: chip count shrinks with selected authors
    const list = q
      ? tags.filter((t) => t.toLowerCase().includes(q) || selectedTags.has(t))
      : tags;
    els.tagChips.innerHTML = list
      .map((t) => {
        const active = selectedTags.has(t) ? "active" : "";
        return `<button type="button" class="chip chip-tag ${active}" data-tag="${escapeHtml(
          t
        )}" aria-pressed="${selectedTags.has(t)}">#${escapeHtml(t)}</button>`;
      })
      .join("");
    syncAllChipSections();
  }

  function normalizeStep(s) {
    if (!s) return null;
    if (typeof s === "string") {
      const text = s.trim();
      return text ? { text, tools: "", tech: "", prep: "", substeps: [] } : null;
    }
    if (typeof s === "object" && s.text) {
      const substeps = Array.isArray(s.substeps)
        ? s.substeps
            .filter((x) => x && String(x.text || "").trim())
            .slice(0, 5)
            .map((x) => ({
              text: String(x.text).trim().slice(0, 40),
              source: x.source ? String(x.source) : "",
            }))
        : [];
      return {
        text: String(s.text),
        tools: s.tools ? String(s.tools) : "",
        tech: s.tech ? String(s.tech) : "",
        prep: s.prep ? String(s.prep) : "",
        substeps,
      };
    }
    return null;
  }

  function substepsHtml(step, stepNum, query) {
    const subs = Array.isArray(step.substeps) ? step.substeps : [];
    if (!subs.length) return "";
    const n = subs.length;
    const items = subs
      .map((sub, j) => {
        const label = `${stepNum}.${j + 1}`;
        const body = query ? highlightHtml(sub.text, query) : escapeHtml(sub.text);
        const src = sub.source
          ? `<div class="substep-source">${escapeHtml(sub.source)}</div>`
          : "";
        return `<li class="substep">
          <div class="substep-rail" aria-hidden="true">
            <span class="substep-node"></span>
            ${j < n - 1 ? '<span class="substep-line"></span>' : ""}
          </div>
          <div class="substep-body">
            <div class="substep-text"><span class="substep-num">${escapeHtml(label)}</span> ${body}</div>
            ${src}
          </div>
        </li>`;
      })
      .join("");
    // Default collapsed; search reveal opens via .is-open + aria.
    return `<div class="substeps" data-substeps>
      <button type="button" class="substeps-toggle" aria-expanded="false">展开子步骤（${n}）</button>
      <ol class="substep-list" hidden>${items}</ol>
    </div>`;
  }

  function stepMetaChips(step) {
    const chips = [];
    if (step.tools) {
      chips.push(
        `<span class="flow-chip"><span class="flow-chip-label">工具</span>${escapeHtml(
          step.tools
        )}</span>`
      );
    }
    if (step.tech) {
      chips.push(
        `<span class="flow-chip"><span class="flow-chip-label">技术</span>${escapeHtml(
          step.tech
        )}</span>`
      );
    }
    if (step.prep) {
      chips.push(
        `<span class="flow-chip"><span class="flow-chip-label">前提</span>${escapeHtml(
          step.prep
        )}</span>`
      );
    }
    return chips.length ? `<div class="flow-meta">${chips.join("")}</div>` : "";
  }

  // ---- Flowchart eligibility (技术/产品/工具介绍或教程 only) ----
  // Durable flag `showFlowchart` is set at digest generation. For older posts without
  // the flag, derive with the same heuristics: BOTH (a) concrete product/tool AND
  // (b) usage/setup flow; if unsure → do not show.
  const FLOW_PRODUCT_RE =
    /Claude(?:\s*Code|\s*Pro)?|Codex|ChatGPT|GPT-?\d|Grok\s*Bot|\bGrok\b|豆包(?:工作)?|Open-?Science|Clipper|Typeless|叭哥说|Tripo|Blender|Unity|Remotion|ESP32|RTX\s*Spark|Hugging\s*Face|Live\s*Link\s*Face|抖音直播伴侣|剪映|Cursor|Antigravity|Opus(?:\s*5\.5)?|Astra|DeepSeek|Mole|Glimpse|AIHOT|Monid|Fable|Unlimited-?OCR|Whoop|Apple\s*Health|UU\s*远程|网易\s*UU|Node\.?js|ffmpeg|StarryBlu|DMIT|Apple\s*Pay|Auto-?Rig|Image\s*2\.5|Google\s*Flow|Profound|REA|Next\.?js|Health\+GPT|Atypica|Sai|MCP|Skill|直播伴侣|面捕/i;
  const FLOW_ACTION_RE =
    /打开|点击|选择|运行|执行|输入|登录|注册|安装|下载|配置|设置|新建|创建|连接|授权|上传|录制|挂载|部署|接入|切换|填写|复制|启动|关闭|佩戴|获取|导入|复现|迁移|报白|推流|第一步|第二步|首先|然后|接着|保姆级|手把手|教程|SOP|从\s*0|按步骤/;
  const FLOW_HARD_NO_RE =
    /必读合集|神帖合集|观察报告|盈利模式|到底为了什么|情绪比时长|抱歉回复晚了|翻转法|代表作拿影响力|半年成长六点|房车长帖|线下跑客户|普通人收益路径|普通人 AI 赚钱老几样|别只盯硅谷模型|放出.{0,20}Flash|月下载\s*\d|涨粉复盘|可复制性的 X 涨粉|从小白到单篇|冷启动贴写错了|血泪报价法|GEO 公开课|主题式连载|脆弱性（GEO）|论文：从公开内容/;
  const FLOW_TUTORIAL_TITLE_RE =
    /教程|保姆|手把手|SOP|从\s*0|开通|安装|部署|上手|清理|开播|配置清单|迁移|实测长文|打通|平替|开源还原|扒了|验收|指标化|万能复刻/;

  function deriveShowFlowchart(p) {
    const raw = Array.isArray(p.steps) ? p.steps : [];
    if (raw.length < 2) return false;
    const texts = raw.map((s) => (s && typeof s === "object" ? String(s.text || "") : String(s || "")));
    const title = String(p.title || "");
    const summary = String(p.summary || "");
    const blob = [title, summary, String(p.tech || ""), ...(p.tags || []), ...texts].join("\n");
    if (FLOW_HARD_NO_RE.test(title) || FLOW_HARD_NO_RE.test(summary.slice(0, 240))) return false;
    if (!FLOW_PRODUCT_RE.test(blob)) return false;
    const actionN = texts.filter((t) => FLOW_ACTION_RE.test(t)).length;
    const tutorialish =
      FLOW_TUTORIAL_TITLE_RE.test(title) || actionN >= 2 || (actionN >= 1 && FLOW_PRODUCT_RE.test(title));
    return !!tutorialish;
  }

  function shouldShowFlowchart(p) {
    if (typeof p.showFlowchart === "boolean") return p.showFlowchart;
    return deriveShowFlowchart(p);
  }

  function stepsHtml(post) {
    if (!shouldShowFlowchart(post)) return "";
    const raw = Array.isArray(post.steps) ? post.steps : [];
    const steps = raw.map(normalizeStep).filter(Boolean);
    if (steps.length === 0) return "";

    const nodes = steps
      .map((step, i) => {
        const n = i + 1;
        const isLast = i === steps.length - 1;
        // Numbers live in the card title (「1. …」); track dots stay unlabeled.
        const labeled = `${n}. ${step.text || ""}`;
        return `<li class="flow-step${isLast ? " is-last" : ""}" style="--step-i: ${i}">
        <div class="flow-rail" aria-hidden="true">
          <span class="flow-node"></span>
          ${isLast ? "" : '<span class="flow-line"></span>'}
        </div>
        <div class="flow-card">
          ${collapsibleHtml(labeled, { className: "flow-text", threshold: STEP_COLLAPSE_CHARS, query: searchQuery() || undefined })}
          ${stepMetaChips(step)}
          ${substepsHtml(step, n, searchQuery() || undefined)}
        </div>
      </li>`;
      })
      .join("");

    const pid = postKey(post);
    const existing = findReplicaByPost(pid);
    const btnLabel = existing ? "已复刻" : "复刻";
    const btnClass = existing ? "replicate-btn is-done" : "replicate-btn";
    const icon = existing
      ? '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path d="M9 16.17 4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>'
      : '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path d="M7.5 3.75a.75.75 0 0 1 .75.75v3h7.5v-3a.75.75 0 0 1 1.5 0v3h.75A2.25 2.25 0 0 1 20.25 9.75v9A2.25 2.25 0 0 1 18 21H6a2.25 2.25 0 0 1-2.25-2.25v-9A2.25 2.25 0 0 1 6 5.5h.75v-3a.75.75 0 0 1 .75-.75zM5.25 10.5v8.25c0 .41.34.75.75.75h12a.75.75 0 0 0 .75-.75V10.5H5.25zm6 2.25a.75.75 0 0 1 .75.75v1.5h1.5a.75.75 0 0 1 0 1.5h-1.5v1.5a.75.75 0 0 1-1.5 0v-1.5h-1.5a.75.75 0 0 1 0-1.5h1.5v-1.5a.75.75 0 0 1 .75-.75z"/></svg>';

    return `<details class="steps">
      <summary>实现步骤（${steps.length}）· 流程图</summary>
      <ol class="flow-chart">${nodes}</ol>
      <div class="replicate-row">
        <button type="button" class="${btnClass}" data-replicate-post="${escapeHtml(pid)}" aria-pressed="${!!existing}">
          ${icon}<span>${btnLabel}</span>
        </button>
      </div>
    </details>`;
  }

  // ---- 复刻 (replicate a post's flow into a personal checklist) ----

  function loadReplicas() {
    try {
      const raw = JSON.parse(localStorage.getItem(REPLICAS_KEY) || "[]");
      replicas = Array.isArray(raw) ? raw.filter((r) => r && typeof r === "object") : [];
    } catch (_) {
      replicas = [];
    }
  }

  function saveReplicas() {
    localStorage.setItem(REPLICAS_KEY, JSON.stringify(replicas));
  }

  function findReplicaByPost(pid) {
    return replicas.find((r) => r.postId === pid) || null;
  }

  function replicaProgress(r) {
    let total = 0;
    let done = 0;
    for (const s of r.steps || []) {
      total += 1;
      if (s.done) done += 1;
      for (const sub of s.substeps || []) {
        total += 1;
        if (sub.done) done += 1;
      }
    }
    return { done, total };
  }

  function newReplicaId() {
    return "r" + Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
  }

  // Build a replica from a post's current steps (snapshot — later edits to the post
  // don't retroactively change an existing replica).
  function createReplicaFromPost(post) {
    const pid = postKey(post);
    const existing = findReplicaByPost(pid);
    if (existing) return existing;
    const raw = Array.isArray(post.steps) ? post.steps : [];
    const steps = raw.map(normalizeStep).filter(Boolean).map((step) => ({
      text: step.text,
      done: false,
      note: "",
      substeps: (step.substeps || []).map((sub) => ({ text: sub.text, done: false, note: "" })),
    }));
    const replica = {
      id: newReplicaId(),
      postId: pid,
      postUrl: post.url || "",
      postAuthor: post.author || "",
      postAuthorName: post.authorName || post.author || "",
      postTitle: post.title || "",
      createdAt: new Date().toISOString(),
      steps,
    };
    replicas = [replica, ...replicas];
    saveReplicas();
    return replica;
  }

  function deleteReplica(id) {
    replicas = replicas.filter((r) => r.id !== id);
    saveReplicas();
  }

  function showPage(page) {
    const isReplicas = page === "replicas";
    if (els.feedMain) els.feedMain.hidden = isReplicas;
    if (els.replicasPage) els.replicasPage.hidden = !isReplicas;
    if (els.openReplicas) els.openReplicas.setAttribute("aria-pressed", String(isReplicas));
    if (isReplicas) renderReplicasPage();
  }

  function goToReplicas() {
    if (location.hash !== "#/fuke") location.hash = "#/fuke";
    else showPage("replicas");
  }

  function routeFromHash() {
    showPage(location.hash === "#/fuke" ? "replicas" : "feed");
  }

  function replicaStepHtml(r, step, idx) {
    const subs = (step.substeps || [])
      .map((sub, j) => {
        return `<li class="replica-substep-row ${sub.done ? "is-done" : ""}">
          <input type="checkbox" class="replica-check" data-replica-sub-check="${escapeHtml(r.id)}" data-step-idx="${idx}" data-sub-idx="${j}" ${sub.done ? "checked" : ""} aria-label="子步骤 ${idx + 1}.${j + 1} 完成" />
          <span class="replica-substep-text">${escapeHtml(sub.text)}</span>
        </li>`;
      })
      .join("");
    return `<li class="replica-step ${step.done ? "is-done" : ""}">
      <div class="replica-step-row">
        <input type="checkbox" class="replica-check" data-replica-check="${escapeHtml(r.id)}" data-step-idx="${idx}" ${step.done ? "checked" : ""} aria-label="步骤 ${idx + 1} 完成" />
        <span class="replica-step-text">${idx + 1}. ${escapeHtml(step.text)}</span>
      </div>
      ${subs ? `<ol class="replica-substeps">${subs}</ol>` : ""}
      <textarea class="replica-note" rows="1" data-replica-note="${escapeHtml(r.id)}" data-step-idx="${idx}" placeholder="写点备注…">${escapeHtml(step.note || "")}</textarea>
    </li>`;
  }

  function replicaCardHtml(r) {
    const { done, total } = replicaProgress(r);
    const pct = total ? Math.round((done / total) * 100) : 0;
    const stepsListHtml = (r.steps || []).map((s, i) => replicaStepHtml(r, s, i)).join("");
    return `<article class="replica-card" data-replica-id="${escapeHtml(r.id)}">
      <div class="replica-head">
        <div class="replica-source">
          <h2 class="replica-source-title">${escapeHtml(r.postTitle || "（无标题）")}</h2>
          <div class="replica-source-meta">
            <span>${escapeHtml(r.postAuthorName || r.postAuthor || "")}</span>
            ${r.postAuthor ? `<span>@${escapeHtml(r.postAuthor)}</span>` : ""}
            ${r.postUrl ? `<a href="${escapeHtml(r.postUrl)}" target="_blank" rel="noopener">查看原帖</a>` : ""}
          </div>
          <div class="replica-time">复刻于 ${escapeHtml(formatShanghai(r.createdAt))}</div>
        </div>
        <button type="button" class="replica-delete" data-replica-delete="${escapeHtml(r.id)}" aria-label="删除复刻" title="删除复刻">
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path fill="currentColor" d="M9.75 3.75h4.5a.75.75 0 0 1 .75.75v.75h4.5a.75.75 0 0 1 0 1.5h-.564l-.853 12.013A2.25 2.25 0 0 1 15.84 21H8.16a2.25 2.25 0 0 1-2.244-2.237L5.064 6.75H4.5a.75.75 0 0 1 0-1.5H9v-.75a.75.75 0 0 1 .75-.75zm-3.182 3 .84 11.85a.75.75 0 0 0 .748.65h7.688a.75.75 0 0 0 .748-.65l.84-11.85H6.568zM10.5 9a.75.75 0 0 1 .75.75v6a.75.75 0 0 1-1.5 0v-6A.75.75 0 0 1 10.5 9zm3 0a.75.75 0 0 1 .75.75v6a.75.75 0 0 1-1.5 0v-6a.75.75 0 0 1 .75-.75z"/></svg>
        </button>
      </div>
      <div class="replica-progress-row">
        <div class="replica-progress-bar"><div class="replica-progress-fill ${pct >= 100 ? "is-complete" : ""}" style="width:${pct}%"></div></div>
        <span class="replica-progress-label">${done}/${total}</span>
      </div>
      <ol class="replica-steps">${stepsListHtml}</ol>
    </article>`;
  }

  function sortedReplicas() {
    const list = replicas.slice();
    if (replicaSortMode === "progress") {
      list.sort((a, b) => {
        const pa = replicaProgress(a);
        const pb = replicaProgress(b);
        const ra = pa.total ? pa.done / pa.total : 0;
        const rb = pb.total ? pb.done / pb.total : 0;
        if (rb !== ra) return rb - ra;
        return String(b.createdAt).localeCompare(String(a.createdAt));
      });
    } else {
      list.sort((a, b) => String(b.createdAt).localeCompare(String(a.createdAt)));
    }
    return list;
  }

  function renderReplicasPage() {
    if (!els.replicaList) return;
    const list = sortedReplicas();
    if (els.replicaCount) els.replicaCount.textContent = `${list.length} 条复刻`;
    els.replicaList.innerHTML = list.map(replicaCardHtml).join("");
    if (els.replicaEmpty) els.replicaEmpty.hidden = list.length > 0;
  }

  function refreshReplicateButtons() {
    if (!els.timeline) return;
    els.timeline.querySelectorAll("[data-replicate-post]").forEach((btn) => {
      const pid = btn.getAttribute("data-replicate-post");
      const exists = !!findReplicaByPost(pid);
      btn.classList.toggle("is-done", exists);
      btn.setAttribute("aria-pressed", String(exists));
      const label = btn.querySelector("span:last-child");
      if (label) label.textContent = exists ? "已复刻" : "复刻";
      const svg = btn.querySelector("svg");
      if (svg) {
        svg.innerHTML = exists
          ? '<path d="M9 16.17 4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/>'
          : '<path d="M7.5 3.75a.75.75 0 0 1 .75.75v3h7.5v-3a.75.75 0 0 1 1.5 0v3h.75A2.25 2.25 0 0 1 20.25 9.75v9A2.25 2.25 0 0 1 18 21H6a2.25 2.25 0 0 1-2.25-2.25v-9A2.25 2.25 0 0 1 6 5.5h.75v-3a.75.75 0 0 1 .75-.75zM5.25 10.5v8.25c0 .41.34.75.75.75h12a.75.75 0 0 0 .75-.75V10.5H5.25zm6 2.25a.75.75 0 0 1 .75.75v1.5h1.5a.75.75 0 0 1 0 1.5h-1.5v1.5a.75.75 0 0 1-1.5 0v-1.5h-1.5a.75.75 0 0 1 0-1.5h1.5v-1.5a.75.75 0 0 1 .75-.75z"/>';
      }
    });
  }

  // All feed filters. `ignoreCats` skips the post-category step (used for chip counts).
  function filteredPosts({ ignoreCats = false } = {}) {
    const q = (els.keyword && els.keyword.value ? els.keyword.value : "")
      .trim()
      .toLowerCase();
    const isUserTagQuery = /^(?:标签[:：]|tag:)/i.test(q);
    const userTagQuery = isUserTagQuery ? q.replace(/^(?:标签[:：]|tag:)/i, "").trim() : "";
    const userQ = (els.userFilter && els.userFilter.value ? els.userFilter.value : "")
      .trim()
      .replace(/^@/, "")
      .toLowerCase();
    // Strip leading # (like username search strips @). Only-# → empty query (no-op).
    const tagQ = (els.tagFilter && els.tagFilter.value ? els.tagFilter.value : "")
      .trim()
      .replace(/^#+/, "")
      .toLowerCase();
    const fromMs = shanghaiDayBound(els.dateFrom && els.dateFrom.value, false);
    const toMs = shanghaiDayBound(els.dateTo && els.dateTo.value, true);

    return allPosts.filter((p) => {
      // Author chips (OR within selected set). Empty selection = empty feed
      // (not "skip author filter"), so counts/tags/chips stay consistent.
      if (!selectedAuthors.has(p.author)) return false;
      if (selectedUserTags.size && !userInfo(p.author).tags.some(t => selectedUserTags.has(t))) return false;

      // Post categories (OR within selected set, AND with others)
      if (!ignoreCats && !matchesSelectedCats(p)) return false;

      // Date range (Asia/Shanghai day bounds)
      if (fromMs != null || toMs != null) {
        const t = Date.parse(p.publishedAt || "");
        if (Number.isNaN(t)) return false;
        if (fromMs != null && t < fromMs) return false;
        if (toMs != null && t > toMs) return false;
      }

      // User name text (authorName OR username)
      if (userQ) {
        const name = String(p.authorName || "").toLowerCase();
        const handle = String(p.author || "").toLowerCase();
        if (!name.includes(userQ) && !handle.includes(userQ)) return false;
      }

      // Tag chips (OR among selected) AND free-text tag query
      const tags = postTags(p);
      if (selectedTags.size) {
        let hit = false;
        for (const t of selectedTags) {
          if (tags.includes(t)) {
            hit = true;
            break;
          }
        }
        if (!hit) return false;
      }
      if (tagQ) {
        const blob = tags.join("\n").toLowerCase();
        if (!blob.includes(tagQ)) return false;
      }

      // Keyword search
      if (isUserTagQuery) {
        if (!userInfo(p.author).tags.some(t => t.toLowerCase().includes(userTagQuery))) return false;
      } else if (q) {
        const stepTexts = (Array.isArray(p.steps) ? p.steps : []).flatMap((s) => {
          if (!s) return [];
          if (typeof s === "string") return [s];
          const subs = Array.isArray(s.substeps) ? s.substeps.map((x) => x && x.text) : [];
          return [s.text, s.tools, s.tech, s.prep, ...subs].filter(Boolean);
        });
        const hay = [
          p.title,
          p.summary,
          p.industry,
          p.tech,
          p.author,
          p.authorName,
          userInfo(p.author).bio,
          ...userInfo(p.author).tags,
          ...tags,
          ...stepTexts,
        ]
          .join("\n")
          .toLowerCase();
        if (!hay.includes(q)) return false;
      }

      return true;
    });
  }

  function renderCard(p) {
    const q = searchQuery();
    const name = p.authorName || authorDisplay(p.author);
    const note = p.note
      ? `<span class="note-pill">${escapeHtml(p.note)}</span>`
      : "";
    const tagList = postTags(p);
    const tags = tagList
      .slice(0, 8)
      .map((t) => {
        const label = q ? highlightHtml(t, q) : escapeHtml(t);
        return `<button type="button" class="tag tag-click" data-tag="${escapeHtml(t)}">#${label}</button>`;
      })
      .join("");

    const myCats = catsOfPost(p);
    const catBadges = myCats.length
      ? `<div class="cat-badges">${myCats
          .map((c) => `<span class="cat-badge">${escapeHtml(catName(c))}</span>`)
          .join("")}</div>`
      : "";
    const pid = postKey(p);
    const menuOpen = openCatMenuPost === pid;

    return `<article class="post-card" data-post-id="${escapeHtml(pid)}">
      ${avatarHtml(p.author, name, p.authorAvatar)}
      <div class="post-body">
        <div class="post-meta">
          <a class="display-name" href="https://x.com/${escapeHtml(
            p.author
          )}" target="_blank" rel="noopener">${escapeHtml(name)}</a>
          <span class="handle">@${escapeHtml(p.author)}</span>
          <span class="meta-dot">·</span>
          <time class="meta-time" datetime="${escapeHtml(
            p.publishedAt || ""
          )}" title="${escapeHtml(formatShanghai(p.publishedAt))}">${escapeHtml(
      formatRelativeOrDate(p.publishedAt)
    )}</time>
          ${note}
        </div>
        <h2 class="post-title">${q ? highlightHtml(p.title || "", q) : escapeHtml(p.title || "")}</h2>
        ${q ? firstHitSnippet(p, q) : ""}
        ${collapsibleHtml(p.summary || "", { className: "post-summary", threshold: SUMMARY_COLLAPSE_CHARS, query: q || undefined })}
        ${tags ? `<div class="tags">${tags}</div>` : ""}
        ${catBadges}
        ${stepsHtml(p)}
        <div class="post-footer">
          <a class="link-original" href="${escapeHtml(
            p.url
          )}" target="_blank" rel="noopener">
            <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>
            查看原文
          </a>
          <div class="cat-control">
            <button type="button" class="cat-btn ${myCats.length ? "has-cats" : ""}" data-cat-toggle="${escapeHtml(
              pid
            )}" aria-haspopup="true" aria-expanded="${menuOpen}" title="帖子类别">
              <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M6 2.75A2.25 2.25 0 0 0 3.75 5v16.2a.75.75 0 0 0 1.18.62L12 16.97l7.07 4.85a.75.75 0 0 0 1.18-.62V5A2.25 2.25 0 0 0 18 2.75H6zm0 1.5h12c.41 0 .75.34.75.75v14.78l-6.32-4.33a.75.75 0 0 0-.86 0l-6.32 4.33V5c0-.41.34-.75.75-.75z"/></svg>
              <span>${myCats.length ? `类别 ${myCats.length}` : "分类"}</span>
            </button>
            ${menuOpen ? catMenuHtml(p) : ""}
          </div>
        </div>
      </div>
    </article>`;
  }

  function renderList() {
    const posts = filteredPosts();
    if (els.resultCount) els.resultCount.textContent = `${posts.length} 条推文`;
    updateActiveFilterCount();

    if (!els.timeline) {
      console.error("Missing #timeline element");
      if (els.emptyState) {
        els.emptyState.hidden = false;
        els.emptyState.classList.add("boot-error");
        els.emptyState.textContent = "页面结构异常：缺少时间线容器，请强制刷新（Ctrl+Shift+R）。";
      }
      return;
    }

    els.timeline.innerHTML = posts.map(renderCard).join("");
    bindCollapseToggles(els.timeline);
    bindSubstepsToggles(els.timeline);
    revealFirstSearchHit();
    els.timeline.querySelectorAll("img.avatar-img").forEach((img) => {
      img.addEventListener("error", () => {
        const ini = img.getAttribute("data-fallback-initials") || "?";
        const color = img.getAttribute("data-fallback-color") || "#2f3336";
        const div = document.createElement("div");
        div.className = "avatar avatar-fallback";
        div.style.background = color;
        div.setAttribute("aria-hidden", "true");
        div.textContent = ini;
        const parent = img.parentElement;
        if (parent && parent.classList.contains("avatar-link")) {
          parent.replaceWith(div);
        } else {
          img.replaceWith(div);
        }
      }, { once: true });
    });

    if (els.emptyState) {
      els.emptyState.hidden = posts.length > 0;
      if (posts.length === 0) {
        els.emptyState.classList.remove("boot-error");
        els.emptyState.textContent = "没有匹配的推文，试试调整筛选条件。";
      }
    }
  }


  /**
   * Parse pasted X identity into { username, nameHint }.
   * Accepts: handle | @handle | 显示名@handle | https://x.com/handle
   */
  function parseWatchlistInput(raw, displayNameField) {
    let s = String(raw || "").trim();
    let nameHint = String(displayNameField || "").trim();
    if (!s) return { error: "请输入用户名或 显示名@handle" };

    // URL forms
    const urlMatch = s.match(/(?:x\.com|twitter\.com)\/([A-Za-z0-9_]{1,15})(?:\/|$|\?)/i);
    if (urlMatch) {
      return { username: urlMatch[1], nameHint: nameHint || "" };
    }

    // If contains @, take handle after the last @; text before is display name
    if (s.includes("@")) {
      const at = s.lastIndexOf("@");
      const before = s.slice(0, at).trim();
      const after = s.slice(at + 1).trim();
      // after may still have path junk
      const handle = after.split(/[\s/?#]/)[0];
      if (before && !nameHint) nameHint = before;
      s = handle;
    } else {
      s = s.replace(/^@/, "").split(/[\s/?#]/)[0];
    }

    s = s.trim();
    if (!/^[A-Za-z0-9_]{1,15}$/.test(s)) {
      return {
        error: "无法识别 handle。请用 @Khazix0 或 显示名@Khazix0（仅字母数字下划线，最多 15 位）",
      };
    }
    return { username: s, nameHint };
  }

  function setAddStatus(msg, ok) {
    if (!els.addStatus) return;
    if (!msg) {
      els.addStatus.hidden = true;
      els.addStatus.textContent = "";
      return;
    }
    els.addStatus.hidden = false;
    els.addStatus.textContent = msg;
    els.addStatus.classList.toggle("is-ok", !!ok);
  }


  function userInfo(username) {
    const key = String(username).toLowerCase();
    const account = (repoWatchlist || []).find(a => a.username.toLowerCase() === key)
      || watchlist.find(a => a.username.toLowerCase() === key) || {};
    const tags = Object.hasOwn(userTagOverrides, key) ? userTagOverrides[key]
      : Object.hasOwn(account, "tagsManual") ? account.tagsManual : account.tags;
    return { bio: typeof account.bio === "string" ? account.bio : "", tags: Array.isArray(tags) ? [...new Set(tags.filter(t => userTagVocabulary.includes(t)))] : [] };
  }

  function renderUserTagChips() {
    const container = document.getElementById("userTagChips");
    container.innerHTML = userTagVocabulary.map(t => {
      const count = watchlist.filter(a => userInfo(a.username).tags.includes(t)).length;
      return `<button type="button" class="chip ${selectedUserTags.has(t) ? "active" : ""}" data-user-tag-filter="${escapeHtml(t)}" aria-pressed="${selectedUserTags.has(t)}">${escapeHtml(t)} <span>${count} 人</span></button>`;
    }).join("");
  }

  document.getElementById("userTagChips").addEventListener("click", e => {
    const button = e.target.closest("[data-user-tag-filter]");
    if (!button) return;
    const tag = button.dataset.userTagFilter;
    selectedUserTags.has(tag) ? selectedUserTags.delete(tag) : selectedUserTags.add(tag);
    applyFiltersAndRender();
  });

  function saveUserTags(row, tags) {
    userTagOverrides[row.dataset.username.toLowerCase()] = tags;
    localStorage.setItem(USER_TAGS_KEY, JSON.stringify(userTagOverrides));
    renderWatchListUI();
    applyFiltersAndRender();
  }
  els.watchList.addEventListener("click", e => {
    const row = e.target.closest("li[data-username]");
    if (!row) return;
    const remove = e.target.closest("[data-user-tag-remove]");
    if (remove) saveUserTags(row, userInfo(row.dataset.username).tags.filter(t => t !== remove.dataset.userTagRemove));
    if (e.target.closest("[data-user-tag-add]")) {
      const picker = row.querySelector(".user-tag-picker");
      picker.hidden = !picker.hidden;
      if (!picker.hidden) picker.focus();
    }
  });
  els.watchList.addEventListener("change", e => {
    if (!e.target.matches(".user-tag-picker") || !userTagVocabulary.includes(e.target.value)) return;
    const row = e.target.closest("li[data-username]");
    saveUserTags(row, [...userInfo(row.dataset.username).tags, e.target.value]);
  });

  const profileCard = document.createElement("div");
  profileCard.className = "user-profile-card";
  profileCard.hidden = true;
  profileCard.setAttribute("role", "dialog");
  profileCard.setAttribute("aria-label", "用户简介");
  document.body.append(profileCard);
  let profileTimer;
  function showProfile(trigger) {
    clearTimeout(profileTimer);
    const username = trigger.dataset.profile;
    const info = userInfo(username);
    profileCard.innerHTML = `<strong>${escapeHtml(authorDisplay(username))}</strong><a href="https://x.com/${encodeURIComponent(username)}" target="_blank" rel="noopener">@${escapeHtml(username)} ↗</a>${info.bio ? `<p>${escapeHtml(info.bio)}</p>` : ""}${info.tags.length ? `<div class="user-profile-tags">${info.tags.map(t => `<span class="chip">${escapeHtml(t)}</span>`).join("")}</div>` : ""}`;
    profileCard.hidden = false;
    const rect = trigger.getBoundingClientRect();
    profileCard.style.left = `${Math.max(8, Math.min(rect.left, innerWidth - profileCard.offsetWidth - 8))}px`;
    profileCard.style.top = `${rect.bottom + profileCard.offsetHeight + 8 > innerHeight ? Math.max(8, rect.top - profileCard.offsetHeight - 8) : rect.bottom + 8}px`;
  }
  els.timeline.addEventListener("pointerover", e => { const t = e.target.closest("[data-profile]"); if (t) showProfile(t); });
  els.timeline.addEventListener("focusin", e => { const t = e.target.closest("[data-profile]"); if (t) showProfile(t); });
  els.timeline.addEventListener("pointerout", e => { if (e.target.closest("[data-profile]")) profileTimer = setTimeout(() => { profileCard.hidden = true; }, 180); });
  profileCard.addEventListener("pointerenter", () => clearTimeout(profileTimer));
  profileCard.addEventListener("pointerleave", () => { profileCard.hidden = true; });
  document.addEventListener("click", e => {
    const trigger = e.target.closest("[data-profile]");
    if (trigger) { e.preventDefault(); showProfile(trigger); }
    else if (!profileCard.contains(e.target)) profileCard.hidden = true;
  });
  document.addEventListener("keydown", e => { if (e.key === "Escape") profileCard.hidden = true; });

  function renderWatchListUI() {
    if (!els.watchList) return;
    renderGroupsUI();
    const opts = (cur) =>
      groups
        .map(
          (g) =>
            `<option value="${escapeHtml(g.id)}" ${g.id === cur ? "selected" : ""}>${escapeHtml(g.name)}</option>`
        )
        .join("");
    els.watchList.innerHTML = watchlist
      .map(
        (a, i) => `<li data-username="${escapeHtml(a.username)}" class="${
          a.group !== DEFAULT_GROUP_ID ? "lp-able" : ""
        }" title="${a.group !== DEFAULT_GROUP_ID ? "长按移至默认" : ""}">
        <div class="meta">
          <strong>${escapeHtml(a.name || a.username)}</strong>
          <span>@${escapeHtml(a.username)}</span>
          <div class="user-tag-editor">${userInfo(a.username).tags.map(t => `<button type="button" class="chip" data-user-tag-remove="${escapeHtml(t)}" aria-label="移除标签 ${escapeHtml(t)}">${escapeHtml(t)} ×</button>`).join("")}
          <button type="button" class="chip" data-user-tag-add aria-label="添加用户标签">+</button>
          <select class="field user-tag-picker" aria-label="选择用户标签" hidden><option value="">选择标签…</option>${userTagVocabulary.filter(t => !userInfo(a.username).tags.includes(t)).map(t => `<option>${escapeHtml(t)}</option>`).join("")}</select></div>
        </div>
        <div class="watch-actions">
          <select class="field group-select" data-group-select="${i}" aria-label="关注类别">${opts(a.group)}</select>
          <button type="button" class="btn-remove" data-remove="${i}">移除</button>
        </div>
      </li>`
      )
      .join("");
  }

  function renderGroupsUI() {
    if (!els.groupList) return;
    els.groupList.innerHTML = groups
      .map((g) => {
        const n = watchlist.filter((a) => a.group === g.id).length;
        if (isFixedGroup(g.id)) {
          return `<li class="group-fixed"><div class="meta"><strong>${escapeHtml(g.name)}</strong><span>固定类别 · ${n} 人</span></div></li>`;
        }
        return `<li>
          <input type="text" class="field group-name-input" data-group-rename="${escapeHtml(
            g.id
          )}" value="${escapeHtml(g.name)}" maxlength="20" aria-label="类别名称" />
          <span class="group-count">${n} 人</span>
          <button type="button" class="btn-remove" data-group-delete="${escapeHtml(g.id)}">删除</button>
        </li>`;
      })
      .join("");
  }

  function applyFiltersAndRender() {
    renderUserTagChips();
    renderChips();
    renderCatChips();
    renderTagChips();
    renderList();
    updateActiveFilterCount();
  }


  let updateCooldownUntil = 0;
  let updateCooldownTimer = null;
  let updateInFlight = false;
  let toastTimer = null;

  function getWebhookConfig() {
    return {
      url: (localStorage.getItem(WEBHOOK_URL_KEY) || "").trim(),
      key: (localStorage.getItem(WEBHOOK_KEY_KEY) || "").trim(),
    };
  }

  function saveWebhookConfig(url, key) {
    localStorage.setItem(WEBHOOK_URL_KEY, String(url || "").trim());
    localStorage.setItem(WEBHOOK_KEY_KEY, String(key || "").trim());
  }

  // Removing the config disables 立即更新 push.
  function removeWebhookConfig() {
    localStorage.removeItem(WEBHOOK_URL_KEY);
    localStorage.removeItem(WEBHOOK_KEY_KEY);
  }

  // 「清除」 is offered only while a config is stored; also refreshes the 立即更新 button.
  function syncWebhookClear() {
    const cfg = getWebhookConfig();
    if (els.clearWebhook) els.clearWebhook.hidden = !(cfg.url || cfg.key);
    syncUpdateButton();
  }

  function authHeaderValue(key) {
    const k = String(key || "").trim();
    if (!k) return "";
    if (/^Bearer\s+/i.test(k)) return k;
    return `Bearer ${k}`;
  }

  // ---- Webhook 保存: inline button states (保存 → 保存中… → 已保存 → 保存) ----
  const WEBHOOK_LOADING_MS = 300; // long enough for the loading state to be perceivable
  const WEBHOOK_SAVED_MS = 2000;
  let webhookTimer = null; // one timer for whichever phase is in flight

  function setWebhookButton(state) {
    const btn = els.saveWebhook;
    if (!btn) return;
    const done = state === "saved" || state === "closed";
    btn.disabled = state === "loading";
    btn.classList.toggle("is-saved", done);
    // SVG elements have no .hidden property in browsers; toggle the attribute.
    btn.querySelector(".btn-icon").toggleAttribute("hidden", !done);
    btn.querySelector(".btn-label").textContent =
      { loading: "保存中…", saved: "已保存", closed: "已关闭" }[state] || "保存";
  }

  // Cancel any pending phase and return to the idle button (repeated clicks, modal close).
  function resetWebhookSave() {
    clearTimeout(webhookTimer);
    webhookTimer = null;
    setWebhookButton("idle");
    if (els.webhookStatus) els.webhookStatus.textContent = "";
  }

  function setFieldError(input, errEl, msg) {
    if (!input || !errEl) return;
    errEl.textContent = msg || "";
    errEl.hidden = !msg;
    if (msg) {
      input.setAttribute("aria-invalid", "true");
      input.setAttribute("aria-describedby", errEl.id);
    } else {
      input.removeAttribute("aria-invalid");
      input.removeAttribute("aria-describedby");
    }
  }

  function clearWebhookErrors() {
    setFieldError(els.webhookUrl, els.webhookUrlError, "");
    setFieldError(els.webhookKey, els.webhookKeyError, "");
  }

  // Returns the trimmed config, or null after showing inline errors.
  function validateWebhookFields() {
    const url = (els.webhookUrl ? els.webhookUrl.value : "").trim();
    const key = (els.webhookKey ? els.webhookKey.value : "").trim();
    let urlOk = false;
    try {
      urlOk = /^https?:$/.test(new URL(url).protocol) && /^https?:\/\//i.test(url);
    } catch (_) {
      urlOk = false;
    }
    setFieldError(els.webhookUrl, els.webhookUrlError, urlOk ? "" : "请输入以 http:// 或 https:// 开头的 URL");
    setFieldError(els.webhookKey, els.webhookKeyError, key ? "" : "请输入 key");
    return urlOk && key ? { url, key } : null;
  }

  function saveWebhookFromSettings() {
    resetWebhookSave();
    setWebhookButton("loading");
    webhookTimer = setTimeout(() => {
      // Both fields empty = turn push off: remove the stored config instead of validating.
      if (!els.webhookUrl.value.trim() && !els.webhookKey.value.trim()) {
        clearWebhookErrors();
        removeWebhookConfig();
        showWebhookResult("closed", "Webhook 已关闭");
        return;
      }
      const cfg = validateWebhookFields();
      if (!cfg) {
        resetWebhookSave();
        const bad = els.webhookUrl.getAttribute("aria-invalid") ? els.webhookUrl : els.webhookKey;
        bad.focus();
        return;
      }
      // Saving identical values is still a successful save and gets the same feedback.
      saveWebhookConfig(cfg.url, cfg.key);
      showWebhookResult("saved", "Webhook 已保存");
    }, WEBHOOK_LOADING_MS);
  }

  // Result phase: button shows the outcome for 2s, live region announces it.
  function showWebhookResult(state, announcement) {
    syncWebhookClear();
    setWebhookButton(state);
    if (els.webhookStatus) els.webhookStatus.textContent = announcement;
    webhookTimer = setTimeout(resetWebhookSave, WEBHOOK_SAVED_MS);
  }

  // 清除: remove the config right away; the shared undo toast can put it back.
  function clearWebhookFromSettings() {
    const prev = getWebhookConfig();
    resetWebhookSave();
    deleteWithUndo(
      "已清除 Webhook 配置",
      () => {
        els.webhookUrl.value = "";
        els.webhookKey.value = "";
        clearWebhookErrors();
        removeWebhookConfig();
        syncWebhookClear();
      },
      () => {
        saveWebhookConfig(prev.url, prev.key);
        els.webhookUrl.value = prev.url;
        els.webhookKey.value = prev.key;
        syncWebhookClear();
      }
    );
  }

  function showUpdateToast(msg) {
    if (!els.updateToast) return;
    els.updateToast.textContent = msg;
    els.updateToast.hidden = false;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      els.updateToast.hidden = true;
    }, 8000);
  }

  function loadWebhookFields() {
    const cfg = getWebhookConfig();
    if (els.webhookUrl) els.webhookUrl.value = cfg.url;
    if (els.webhookKey) els.webhookKey.value = cfg.key;
    syncWebhookClear();
  }

  function syncUpdateButton() {
    const btn = els.triggerUpdate;
    if (!btn) return;
    const cfg = getWebhookConfig();
    const configured = !!(cfg.url && cfg.key);
    const cooling = Date.now() < updateCooldownUntil;
    const busy = updateInFlight || cooling;
    // Unconfigured: keep clickable (opens settings) but dimmed.
    btn.disabled = busy;
    btn.classList.toggle("is-dimmed", !configured && !busy);
    btn.classList.toggle("is-spinning", busy);
    btn.setAttribute("aria-label", "立即更新");
    if (!configured) {
      btn.title = "先在设置里配置 Webhook";
    } else if (updateInFlight) {
      btn.title = "正在发送更新请求…";
    } else if (cooling) {
      const left = Math.max(1, Math.ceil((updateCooldownUntil - Date.now()) / 1000));
      btn.title = `请稍候 ${left}s`;
    } else {
      btn.title = "立即更新";
    }
  }

  function startUpdateCooldown() {
    updateCooldownUntil = Date.now() + UPDATE_DEBOUNCE_MS;
    syncUpdateButton();
    if (updateCooldownTimer) clearInterval(updateCooldownTimer);
    updateCooldownTimer = setInterval(() => {
      if (Date.now() >= updateCooldownUntil) {
        clearInterval(updateCooldownTimer);
        updateCooldownTimer = null;
        updateCooldownUntil = 0;
      }
      syncUpdateButton();
    }, 500);
  }

  async function postWebhook(url, key) {
    const auth = authHeaderValue(key);
    const headers = {
      Authorization: auth,
      "Content-Type": "application/json",
    };
    try {
      const res = await fetch(url, {
        method: "POST",
        headers,
        body: "{}",
        mode: "cors",
        credentials: "omit",
      });
      return { ok: true, status: res.status, cors: true };
    } catch (err) {
      // CORS or network: best-effort opaque send (custom headers may be stripped).
      try {
        await fetch(url, {
          method: "POST",
          headers,
          body: "{}",
          mode: "no-cors",
          credentials: "omit",
        });
        return { ok: true, opaque: true, cors: false };
      } catch (_) {
        /* continue */
      }
      try {
        if (typeof navigator !== "undefined" && navigator.sendBeacon) {
          const blob = new Blob(["{}"], { type: "text/plain" });
          const sent = navigator.sendBeacon(url, blob);
          if (sent) return { ok: true, beacon: true, cors: false };
        }
      } catch (_) {
        /* continue */
      }
      throw err;
    }
  }

  async function triggerImmediateUpdate() {
    const cfg = getWebhookConfig();
    if (!cfg.url || !cfg.key) {
      showUpdateToast("请先在设置中配置 Webhook URL 与 key");
      if (els.settingsModal) {
        loadWebhookFields();
        els.settingsModal.showModal();
      }
      return;
    }
    if (updateInFlight || Date.now() < updateCooldownUntil) return;

    updateInFlight = true;
    syncUpdateButton();

    try {
      await postWebhook(cfg.url, cfg.key);
      showUpdateToast(UPDATE_STATUS_MSG);
      updateInFlight = false;
      startUpdateCooldown();
    } catch (err) {
      console.error(err);
      updateInFlight = false;
      showUpdateToast("发送失败（可能是网络或 CORS）。请检查 URL/key，或稍后再试。");
      syncUpdateButton();
    }
  }

  function closeModal() {
    if (els.settingsModal && els.settingsModal.open) els.settingsModal.close();
  }

  function showFatal(err) {
    const msg = String((err && err.message) || err || "未知错误");
    console.error(err);
    if (els.lastUpdated) {
      els.lastUpdated.textContent = "数据加载失败";
      els.lastUpdated.classList.add("boot-error");
    }
    if (els.emptyState) {
      els.emptyState.hidden = false;
      els.emptyState.classList.add("boot-error");
      els.emptyState.textContent = msg;
    }
  }

  function toggleTag(tag) {
    if (!tag) return;
    if (selectedTags.has(tag)) selectedTags.delete(tag);
    else selectedTags.add(tag);
    applyFiltersAndRender();
  }

  if (els.authorChips) {
    els.authorChips.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-author]");
      if (!btn) return;
      if (suppressClickUntil > Date.now()) {
        e.preventDefault();
        return;
      }
      const u = btn.getAttribute("data-author");
      if (selectedAuthors.has(u)) selectedAuthors.delete(u);
      else selectedAuthors.add(u);
      saveSelection();
      applyFiltersAndRender();
    });
    els.authorChips.addEventListener("change", (e) => {
      const cb = e.target.closest("[data-group-toggle]");
      if (!cb) return;
      const gid = cb.getAttribute("data-group-toggle");
      const usernames = [
        ...new Set([...watchlist.map((a) => a.username), ...allPosts.map((p) => p.author)]),
      ].filter((u) => groupOfAuthor(u) === gid);
      for (const u of usernames) {
        if (cb.checked) selectedAuthors.add(u);
        else selectedAuthors.delete(u);
      }
      saveSelection();
      applyFiltersAndRender();
    });
  }

  if (els.tagChips) {
    els.tagChips.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-tag]");
      if (!btn) return;
      toggleTag(btn.getAttribute("data-tag"));
    });
  }

  if (els.timeline) {
    els.timeline.addEventListener("click", (e) => {
      const btn = e.target.closest(".tag-click[data-tag]");
      if (!btn) return;
      toggleTag(btn.getAttribute("data-tag"));
    });
  }

  if (els.timeline) {
    els.timeline.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-replicate-post]");
      if (!btn) return;
      e.preventDefault();
      e.stopPropagation();
      const pid = btn.getAttribute("data-replicate-post");
      const post = allPosts.find((p) => postKey(p) === pid);
      const already = findReplicaByPost(pid);
      if (!post && !already) return;
      if (!already) createReplicaFromPost(post);
      refreshReplicateButtons();
      goToReplicas();
    });
  }

  if (els.catChips) {
    els.catChips.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-cat]");
      if (!btn) return;
      const c = btn.getAttribute("data-cat");
      if (selectedCats.has(c)) selectedCats.delete(c);
      else selectedCats.add(c);
      saveSelection();
      applyFiltersAndRender();
    });
  }

  function rerenderPostsKeepScroll() {
    const y = window.scrollY;
    renderCatChips();
    renderList();
    updateActiveFilterCount();
    syncAllChipSections();
    window.scrollTo(0, y);
  }

  function focusMenuInput(pid) {
    const inp = document.querySelector(`[data-menu-new="${CSS.escape(pid)}"]`);
    if (inp) inp.focus();
  }

  function menuCreate(pid) {
    const inp = document.querySelector(`[data-menu-new="${CSS.escape(pid)}"]`);
    const cat = createCategory(inp && inp.value);
    if (!cat) {
      if (inp) inp.focus();
      return;
    }
    setPostCat(pid, cat.id, true);
    rerenderPostsKeepScroll();
    focusMenuInput(pid);
  }

  if (els.timeline) {
    els.timeline.addEventListener("click", (e) => {
      const t = e.target.closest("[data-cat-toggle]");
      if (t) {
        e.preventDefault();
        const pid = t.getAttribute("data-cat-toggle");
        openCatMenuPost = openCatMenuPost === pid ? null : pid;
        rerenderPostsKeepScroll();
        return;
      }
      const nb = e.target.closest("[data-menu-new-btn]");
      if (nb) {
        e.preventDefault();
        menuCreate(nb.getAttribute("data-menu-new-btn"));
      }
    });
    els.timeline.addEventListener("change", (e) => {
      const cb = e.target.closest("[data-menu-cat]");
      if (!cb) return;
      setPostCat(cb.getAttribute("data-pid"), cb.getAttribute("data-menu-cat"), cb.checked);
      rerenderPostsKeepScroll();
    });
    els.timeline.addEventListener("keydown", (e) => {
      const inp = e.target.closest("[data-menu-new]");
      if (inp && e.key === "Enter") {
        e.preventDefault();
        menuCreate(inp.getAttribute("data-menu-new"));
      }
    });
  }

  document.addEventListener("click", (e) => {
    if (!openCatMenuPost) return;
    if (e.target.closest(".cat-control")) return;
    openCatMenuPost = null;
    rerenderPostsKeepScroll();
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && openCatMenuPost) {
      openCatMenuPost = null;
      rerenderPostsKeepScroll();
    }
  });

  if (els.catList) {
    els.catList.addEventListener("change", (e) => {
      const inp = e.target.closest("[data-cat-rename]");
      if (!inp) return;
      const c = postCats.categories.find((x) => x.id === inp.getAttribute("data-cat-rename"));
      const name = inp.value.trim();
      if (!c || !name) {
        if (c) inp.value = c.name;
        return;
      }
      c.name = name;
      savePostCats();
      renderCatsUI();
      applyFiltersAndRender();
    });
    els.catList.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-cat-delete]");
      if (!btn) return;
      e.preventDefault();
      const cid = btn.getAttribute("data-cat-delete");
      const cat = postCats.categories.find((c) => c.id === cid);
      if (!cat) return;
      const n = Object.values(postCats.assignments).filter((l) => l.includes(cid)).length;
      const ask = n > 0 ? confirmDelete(`删除类别“${cat.name}”？`, `其中 ${n} 条帖子会取消该分类`) : Promise.resolve(true);
      ask.then((ok) => {
        if (ok) deleteWithUndo(`已删除类别 ${cat.name}`, () => deleteCategory(cid));
      });
    });
  }

  function addCategoryFromSettings() {
    if (!createCategory(els.newCatName && els.newCatName.value)) {
      if (els.newCatName) els.newCatName.focus();
      return;
    }
    els.newCatName.value = "";
    renderCatsUI();
    applyFiltersAndRender();
  }
  if (els.addCat) {
    els.addCat.addEventListener("click", (e) => {
      e.preventDefault();
      addCategoryFromSettings();
    });
  }
  if (els.newCatName) {
    els.newCatName.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        addCategoryFromSettings();
      }
    });
  }

  const refilter = () => {
    renderTagChips();
    renderList();
    updateActiveFilterCount();
  };

  if (els.keyword) els.keyword.addEventListener("input", refilter);
  if (els.userFilter) els.userFilter.addEventListener("input", refilter);
  if (els.tagFilter) els.tagFilter.addEventListener("input", refilter);
  if (els.dateFrom) els.dateFrom.addEventListener("change", refilter);
  if (els.dateTo) els.dateTo.addEventListener("change", refilter);

  if (els.clearFilters) {
    els.clearFilters.addEventListener("click", () => {
      if (els.keyword) els.keyword.value = "";
      if (els.userFilter) els.userFilter.value = "";
      if (els.tagFilter) els.tagFilter.value = "";
      if (els.dateFrom) els.dateFrom.value = "";
      if (els.dateTo) els.dateTo.value = "";
      selectedUserTags.clear();
      selectedTags = new Set();
      selectedCats = new Set();
      selectedAuthors = specialAuthors();
      saveSelection();
      applyFiltersAndRender();
    });
  }

  if (els.themeToggle) els.themeToggle.addEventListener("click", toggleTheme);

  if (els.openSettings) {
    els.openSettings.addEventListener("click", () => {
      setAddStatus("");
      resetWebhookSave();
      clearWebhookErrors();
      renderWatchListUI();
      renderCatsUI();
      loadWebhookFields();
      if (els.settingsModal) els.settingsModal.showModal();
    });
  }

  if (els.closeSettings) els.closeSettings.addEventListener("click", closeModal);
  if (els.doneSettings) els.doneSettings.addEventListener("click", closeModal);

  if (els.settingsModal) {
    els.settingsModal.addEventListener("click", (e) => {
      if (e.target === els.settingsModal) closeModal();
    });
    // Closing the modal (完成, ×, Esc, backdrop) ends the undo window.
    els.settingsModal.addEventListener("close", () => {
      commitPendingDelete();
      resetWebhookSave(); // drop in-flight 保存 timers
    });
  }
  if (els.undoToastBtn) els.undoToastBtn.addEventListener("click", undoPendingDelete);
  window.addEventListener("pagehide", commitPendingDelete);

  if (els.watchList) {
    els.watchList.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-remove]");
      if (!btn) return;
      e.preventDefault();
      e.stopPropagation();
      const idx = Number(btn.getAttribute("data-remove"));
      if (!Number.isInteger(idx) || idx < 0 || idx >= watchlist.length) return;
      const removed = watchlist[idx];
      deleteWithUndo(`已移除 ${removed.name || removed.username}`, () => {
        watchlist = watchlist.filter((_, i) => i !== idx);
        selectedAuthors.delete(removed.username);
      });
    });
  }

  // ---- Long press: move author to 默认 ----
  let suppressClickUntil = 0;

  function moveAuthorToDefault(username) {
    const idx = watchlist.findIndex((a) => a.username === username);
    const name = authorDisplay(username);
    if (idx < 0 || watchlist[idx].group === DEFAULT_GROUP_ID) {
      showUpdateToast(`${name} 已在「${DEFAULT_GROUP_NAME}」中`);
      return false;
    }
    watchlist[idx] = { ...watchlist[idx], group: DEFAULT_GROUP_ID };
    saveWatchlist(watchlist);
    showUpdateToast(`已将 ${name} 移至${DEFAULT_GROUP_NAME}`);
    if (els.settingsModal && els.settingsModal.open) renderWatchListUI();
    applyFiltersAndRender();
    return true;
  }

  function bindLongPress(root, selector, getUsername) {
    if (!root) return;
    let timer = null;
    let target = null;
    let startX = 0;
    let startY = 0;
    const cancel = () => {
      if (timer) clearTimeout(timer);
      timer = null;
      if (target) target.classList.remove("is-pressing");
      target = null;
    };
    root.addEventListener("pointerdown", (e) => {
      if (e.button !== undefined && e.button !== 0) return;
      const el = e.target.closest(selector);
      if (!el || e.target.closest("select, input, button.btn-remove")) return;
      const u = getUsername(el);
      if (!u) return;
      cancel();
      target = el;
      startX = e.clientX;
      startY = e.clientY;
      if (groupOfAuthor(u) !== DEFAULT_GROUP_ID) el.classList.add("is-pressing");
      timer = setTimeout(() => {
        timer = null;
        el.classList.remove("is-pressing");
        target = null;
        suppressClickUntil = Date.now() + 800;
        if (navigator.vibrate) {
          try { navigator.vibrate(20); } catch (_) { /* ignore */ }
        }
        moveAuthorToDefault(u);
      }, LONG_PRESS_MS);
    });
    root.addEventListener("pointermove", (e) => {
      if (!target) return;
      if (Math.abs(e.clientX - startX) > 10 || Math.abs(e.clientY - startY) > 10) cancel();
    });
    ["pointerup", "pointercancel", "pointerleave"].forEach((t) =>
      root.addEventListener(t, () => {
        cancel();
        // Swallow only the click that immediately follows a long press
        if (suppressClickUntil) setTimeout(() => (suppressClickUntil = 0), 0);
      })
    );
    root.addEventListener("contextmenu", (e) => {
      if (e.target.closest(selector)) e.preventDefault();
    });
  }

  bindLongPress(els.authorChips, "[data-author]", (el) => el.getAttribute("data-author"));
  bindLongPress(els.watchList, "li[data-username]", (el) => el.getAttribute("data-username"));

  function addWatchlistAccount() {
    const parsed = parseWatchlistInput(
      els.newUsername && els.newUsername.value,
      els.newDisplayName && els.newDisplayName.value
    );
    if (parsed.error) {
      setAddStatus(parsed.error, false);
      if (els.newUsername) els.newUsername.focus();
      return false;
    }
    const username = parsed.username;
    if (watchlist.some((a) => a.username.toLowerCase() === username.toLowerCase())) {
      setAddStatus(`@${username} 已在关注列表中`, false);
      return false;
    }
    const name = parsed.nameHint || username;
    watchlist = watchlist.concat([{ username, name, group: firstOtherGroupId() }]);
    saveWatchlist(watchlist);
    if (els.newUsername) els.newUsername.value = "";
    if (els.newDisplayName) els.newDisplayName.value = "";
    let msg = `已添加 @${username}`;
    const hasPosts = (allPosts || []).some(
      (p) => (p.author || "").toLowerCase() === username.toLowerCase()
    );
    if (!hasPosts) {
      msg = `已加入关注 @${username}；日报数据里还没有 TA 的帖，等每日更新`;
    }
    setAddStatus(msg, true);
    renderWatchListUI();
    applyFiltersAndRender();
    return true;
  }

  if (els.addAuthor) {
    els.addAuthor.addEventListener("click", (e) => {
      e.preventDefault();
      addWatchlistAccount();
    });
  }

  if (els.newUsername) {
    els.newUsername.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        addWatchlistAccount();
      }
    });
  }

  if (els.resetWatchlist) {
    els.resetWatchlist.addEventListener("click", () => {
      localStorage.removeItem(USER_TAGS_KEY);
      userTagOverrides = {};
      selectedUserTags.clear();
      applyRepoDefault();
      saveWatchlist(watchlist);
      selectedAuthors = specialAuthors();
      saveSelection();
      renderWatchListUI();
      applyFiltersAndRender();
    });
  }

  if (els.watchList) {
    els.watchList.addEventListener("change", (e) => {
      const sel = e.target.closest("[data-group-select]");
      if (!sel) return;
      const idx = Number(sel.getAttribute("data-group-select"));
      if (!watchlist[idx]) return;
      watchlist[idx] = { ...watchlist[idx], group: sel.value };
      saveWatchlist(watchlist);
      renderWatchListUI();
      applyFiltersAndRender();
    });
  }

  if (els.groupList) {
    els.groupList.addEventListener("change", (e) => {
      const inp = e.target.closest("[data-group-rename]");
      if (!inp) return;
      const g = groups.find((x) => x.id === inp.getAttribute("data-group-rename"));
      if (!g || isFixedGroup(g.id)) return;
      const name = inp.value.trim();
      if (!name) {
        inp.value = g.name;
        return;
      }
      g.name = name;
      saveWatchlist(watchlist);
      renderWatchListUI();
      applyFiltersAndRender();
    });
    els.groupList.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-group-delete]");
      if (!btn || btn.disabled) return;
      e.preventDefault();
      const gid = btn.getAttribute("data-group-delete");
      const group = groups.find((g) => g.id === gid);
      if (!group || isFixedGroup(gid)) return;
      const n = watchlist.filter((a) => a.group === gid).length;
      const ask = n > 0 ? confirmDelete(`删除类别“${group.name}”？`, `其中 ${n} 个用户会变为未分类`) : Promise.resolve(true);
      ask.then((ok) => {
        if (!ok) return;
        // Same as before: members move to the first remaining non-fixed-special group.
        deleteWithUndo(`已删除类别 ${group.name}`, () => {
          groups = groups.filter((g) => g.id !== gid);
          const target = firstOtherGroupId();
          watchlist = watchlist.map((a) => (a.group === gid ? { ...a, group: target } : a));
        });
      });
    });
  }

  if (els.addGroup) {
    els.addGroup.addEventListener("click", (e) => {
      e.preventDefault();
      const name = (els.newGroupName && els.newGroupName.value.trim()) || `新类别 ${groups.length - 1}`;
      groups.push({ id: newGroupId(), name });
      if (els.newGroupName) els.newGroupName.value = "";
      saveWatchlist(watchlist);
      renderWatchListUI();
      applyFiltersAndRender();
    });
  }

  if (els.saveWebhook) els.saveWebhook.addEventListener("click", saveWebhookFromSettings);
  if (els.clearWebhook) els.clearWebhook.addEventListener("click", clearWebhookFromSettings);
  // Editing a field clears its own error.
  if (els.webhookUrl) els.webhookUrl.addEventListener("input", () => setFieldError(els.webhookUrl, els.webhookUrlError, ""));
  if (els.webhookKey) els.webhookKey.addEventListener("input", () => setFieldError(els.webhookKey, els.webhookKeyError, ""));

  if (els.triggerUpdate) {
    els.triggerUpdate.addEventListener("click", (e) => {
      e.preventDefault();
      triggerImmediateUpdate();
    });
  }

  syncUpdateButton();

  // ---- 复刻 page wiring ----
  if (els.openReplicas) {
    els.openReplicas.addEventListener("click", (e) => {
      e.preventDefault();
      goToReplicas();
    });
  }

  window.addEventListener("hashchange", routeFromHash);

  if (els.replicaSort) {
    els.replicaSort.addEventListener("change", (e) => {
      replicaSortMode = e.target.value === "progress" ? "progress" : "time";
      renderReplicasPage();
    });
  }

  if (els.replicaList) {
    els.replicaList.addEventListener("change", (e) => {
      const stepCb = e.target.closest("[data-replica-check]");
      if (stepCb) {
        const r = replicas.find((x) => x.id === stepCb.getAttribute("data-replica-check"));
        const idx = Number(stepCb.getAttribute("data-step-idx"));
        if (r && r.steps[idx]) {
          r.steps[idx].done = stepCb.checked;
          saveReplicas();
          renderReplicasPage();
        }
        return;
      }
      const subCb = e.target.closest("[data-replica-sub-check]");
      if (subCb) {
        const r = replicas.find((x) => x.id === subCb.getAttribute("data-replica-sub-check"));
        const idx = Number(subCb.getAttribute("data-step-idx"));
        const sidx = Number(subCb.getAttribute("data-sub-idx"));
        if (r && r.steps[idx] && r.steps[idx].substeps[sidx]) {
          r.steps[idx].substeps[sidx].done = subCb.checked;
          saveReplicas();
          renderReplicasPage();
        }
      }
    });

    // Notes: debounce writes so every keystroke doesn't thrash localStorage or
    // re-render (which would steal focus from the textarea).
    let noteSaveTimer = null;
    els.replicaList.addEventListener("input", (e) => {
      const ta = e.target.closest("[data-replica-note]");
      if (!ta) return;
      const r = replicas.find((x) => x.id === ta.getAttribute("data-replica-note"));
      const idx = Number(ta.getAttribute("data-step-idx"));
      if (!r || !r.steps[idx]) return;
      r.steps[idx].note = ta.value;
      clearTimeout(noteSaveTimer);
      noteSaveTimer = setTimeout(saveReplicas, 400);
    });

    els.replicaList.addEventListener("click", (e) => {
      const delBtn = e.target.closest("[data-replica-delete]");
      if (!delBtn) return;
      e.preventDefault();
      const id = delBtn.getAttribute("data-replica-delete");
      const r = replicas.find((x) => x.id === id);
      if (!r) return;
      confirmDelete("删除这条复刻？", `「${r.postTitle || "该流程"}」的打卡记录和备注都会被删除`).then((ok) => {
        if (!ok) return;
        deleteReplica(id);
        renderReplicasPage();
        refreshReplicateButtons();
      });
    });
  }

  loadTheme();
  initFilterCollapse();
  initChipSections();
  loadPostCats();
  loadReplicas();
  selectedCats = loadSelectedCats();
  loadData()
    .then(() => {
      applyFiltersAndRender();
      routeFromHash();
    })
    .catch(showFatal);
})();
