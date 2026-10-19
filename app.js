(() => {
  "use strict";

  const STORAGE_KEY = "x-digest-watchlist";
  const THEME_KEY = "x-digest-theme";
  const FILTER_COLLAPSE_KEY = "x-digest-filters-collapsed";
  const WEBHOOK_URL_KEY = "x-digest-webhook-url";
  const WEBHOOK_KEY_KEY = "x-digest-webhook-key";
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
    watchList: document.getElementById("watchList"),
    addAuthor: document.getElementById("addAuthor"),
    addStatus: document.getElementById("addStatus"),
    newUsername: document.getElementById("newUsername"),
    newDisplayName: document.getElementById("newDisplayName"),
    resetWatchlist: document.getElementById("resetWatchlist"),
    triggerUpdate: document.getElementById("triggerUpdate"),
    webhookUrl: document.getElementById("webhookUrl"),
    webhookKey: document.getElementById("webhookKey"),
    saveWebhook: document.getElementById("saveWebhook"),
    webhookStatus: document.getElementById("webhookStatus"),
    updateToast: document.getElementById("updateToast"),
  };

  let allPosts = [];
  let watchlist = [];
  let selectedAuthors = new Set();
  let selectedTags = new Set();
  let repoWatchlist = null;
  /** @type {Record<string, string>} username -> avatar URL (local or remote) */
  let avatarMap = {};

  function escapeHtml(str) {
    return String(str ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }


  /** Collapse long text in UI without truncating the stored string. */
  const SUMMARY_COLLAPSE_CHARS = 120;
  const STEP_COLLAPSE_CHARS = 80;

  function collapsibleHtml(fullText, { className, threshold }) {
    const text = String(fullText ?? "");
    if (!text) return "";
    const needs = [...text].length > threshold || text.split("\n").length > 4;
    if (!needs) {
      return `<p class="${className}">${escapeHtml(text)}</p>`;
    }
    return `<div class="collapsible ${className}-wrap" data-collapsed="true">
      <p class="${className} collapsible-text">${escapeHtml(text)}</p>
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
    let n = 0;
    if (els.keyword && els.keyword.value.trim()) n += 1;
    if (els.dateFrom && els.dateFrom.value) n += 1;
    if (els.dateTo && els.dateTo.value) n += 1;
    if (els.userFilter && els.userFilter.value.trim()) n += 1;
    if (els.tagFilter && els.tagFilter.value.trim()) n += 1;
    n += selectedTags.size;
    const defaultAuthors = new Set(watchlist.map((a) => a.username));
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
    if (!src) return fallback;
    return `<a class="avatar-link" href="https://x.com/${escapeHtml(
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

  function loadWatchlistFromStorage() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed?.accounts)) return parsed.accounts;
      if (Array.isArray(parsed)) return parsed;
    } catch (_) {
      /* ignore */
    }
    return null;
  }

  function saveWatchlist(accounts) {
    watchlist = accounts.slice();
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ accounts: watchlist }));
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
    } catch (err) {
      console.warn("watchlist fallback:", err);
      repoWatchlist = DEFAULT_WATCHLIST;
    }

    const stored = loadWatchlistFromStorage();
    watchlist = stored && stored.length ? stored : repoWatchlist.slice();
    selectedAuthors = new Set(watchlist.map((a) => a.username));

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

  function renderChips() {
    if (!els.authorChips) return;
    const authorsFromData = [...new Set(allPosts.map((p) => p.author))];
    const usernames = [
      ...new Set([...watchlist.map((a) => a.username), ...authorsFromData]),
    ];

    els.authorChips.innerHTML = usernames
      .map((u) => {
        const active = selectedAuthors.has(u) ? "active" : "";
        const label = escapeHtml(authorDisplay(u));
        return `<button type="button" class="chip ${active}" data-author="${escapeHtml(
          u
        )}" aria-pressed="${selectedAuthors.has(u)}">${label}</button>`;
      })
      .join("");
  }

  function renderTagChips() {
    if (!els.tagChips) return;
    const tags = collectAllTags();
    pruneSelectedTags(tags);
    const q = (els.tagFilter && els.tagFilter.value ? els.tagFilter.value : "")
      .trim()
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
  }

  function normalizeStep(s) {
    if (!s) return null;
    if (typeof s === "string") {
      const text = s.trim();
      return text ? { text, tools: "", tech: "", prep: "" } : null;
    }
    if (typeof s === "object" && s.text) {
      return {
        text: String(s.text),
        tools: s.tools ? String(s.tools) : "",
        tech: s.tech ? String(s.tech) : "",
        prep: s.prep ? String(s.prep) : "",
      };
    }
    return null;
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

  function stepsHtml(post) {
    const raw = Array.isArray(post.steps) ? post.steps : [];
    const steps = raw.map(normalizeStep).filter(Boolean);
    if (!post.isLong || steps.length === 0) return "";

    const nodes = steps
      .map((step, i) => {
        const n = i + 1;
        const isLast = i === steps.length - 1;
        return `<li class="flow-step${isLast ? " is-last" : ""}">
        <div class="flow-rail" aria-hidden="true">
          <span class="flow-node">${n}</span>
          ${isLast ? "" : '<span class="flow-line"></span>'}
        </div>
        <div class="flow-card">
          ${collapsibleHtml(step.text, { className: "flow-text", threshold: STEP_COLLAPSE_CHARS })}
          ${stepMetaChips(step)}
        </div>
      </li>`;
      })
      .join("");

    return `<details class="steps">
      <summary>实现步骤（${steps.length}）· 流程图</summary>
      <ol class="flow-chart">${nodes}</ol>
    </details>`;
  }

  function filteredPosts() {
    const q = (els.keyword && els.keyword.value ? els.keyword.value : "")
      .trim()
      .toLowerCase();
    const userQ = (els.userFilter && els.userFilter.value ? els.userFilter.value : "")
      .trim()
      .replace(/^@/, "")
      .toLowerCase();
    const tagQ = (els.tagFilter && els.tagFilter.value ? els.tagFilter.value : "")
      .trim()
      .toLowerCase();
    const fromMs = shanghaiDayBound(els.dateFrom && els.dateFrom.value, false);
    const toMs = shanghaiDayBound(els.dateTo && els.dateTo.value, true);

    return allPosts.filter((p) => {
      // Author chips (OR within selected set)
      if (selectedAuthors.size && !selectedAuthors.has(p.author)) return false;

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
      if (q) {
        const stepTexts = (Array.isArray(p.steps) ? p.steps : []).flatMap((s) => {
          if (!s) return [];
          if (typeof s === "string") return [s];
          return [s.text, s.tools, s.tech, s.prep].filter(Boolean);
        });
        const hay = [
          p.title,
          p.summary,
          p.industry,
          p.tech,
          p.author,
          p.authorName,
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
    const name = p.authorName || authorDisplay(p.author);
    const note = p.note
      ? `<span class="note-pill">${escapeHtml(p.note)}</span>`
      : "";
    const tagList = postTags(p);
    const tags = tagList
      .slice(0, 8)
      .map((t) => `<button type="button" class="tag tag-click" data-tag="${escapeHtml(t)}">#${escapeHtml(t)}</button>`)
      .join("");

    return `<article class="post-card">
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
        <h2 class="post-title">${escapeHtml(p.title || "")}</h2>
        ${collapsibleHtml(p.summary || "", { className: "post-summary", threshold: SUMMARY_COLLAPSE_CHARS })}
        ${tags ? `<div class="tags">${tags}</div>` : ""}
        ${stepsHtml(p)}
        <div class="post-footer">
          <a class="link-original" href="${escapeHtml(
            p.url
          )}" target="_blank" rel="noopener">
            <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>
            查看原文
          </a>
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

  function renderWatchListUI() {
    if (!els.watchList) return;
    els.watchList.innerHTML = watchlist
      .map(
        (a, i) => `<li>
        <div class="meta">
          <strong>${escapeHtml(a.name || a.username)}</strong>
          <span>@${escapeHtml(a.username)}</span>
        </div>
        <button type="button" class="btn-remove" data-remove="${i}">移除</button>
      </li>`
      )
      .join("");
  }

  function applyFiltersAndRender() {
    renderChips();
    renderTagChips();
    renderList();
    updateActiveFilterCount();
  }


  let updateCooldownUntil = 0;
  let updateCooldownTimer = null;
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

  function authHeaderValue(key) {
    const k = String(key || "").trim();
    if (!k) return "";
    if (/^Bearer\s+/i.test(k)) return k;
    return `Bearer ${k}`;
  }

  function setWebhookStatus(msg, ok) {
    if (!els.webhookStatus) return;
    if (!msg) {
      els.webhookStatus.hidden = true;
      els.webhookStatus.textContent = "";
      return;
    }
    els.webhookStatus.hidden = false;
    els.webhookStatus.textContent = msg;
    els.webhookStatus.classList.toggle("is-ok", !!ok);
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
  }

  function syncUpdateButton() {
    const btn = els.triggerUpdate;
    if (!btn) return;
    const cfg = getWebhookConfig();
    const configured = !!(cfg.url && cfg.key);
    const cooling = Date.now() < updateCooldownUntil;
    btn.disabled = !configured || cooling;
    if (!configured) {
      btn.title = "请先在设置中配置 Webhook";
      btn.textContent = "立即更新";
    } else if (cooling) {
      const left = Math.max(1, Math.ceil((updateCooldownUntil - Date.now()) / 1000));
      btn.title = `请稍候 ${left}s`;
      btn.textContent = `请稍候 ${left}s`;
    } else {
      btn.title = "触发站内立即更新";
      btn.textContent = "立即更新";
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
    if (Date.now() < updateCooldownUntil) return;

    const btn = els.triggerUpdate;
    if (btn) btn.disabled = true;

    try {
      await postWebhook(cfg.url, cfg.key);
      showUpdateToast(UPDATE_STATUS_MSG);
      startUpdateCooldown();
    } catch (err) {
      console.error(err);
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
      const u = btn.getAttribute("data-author");
      if (selectedAuthors.has(u)) selectedAuthors.delete(u);
      else selectedAuthors.add(u);
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
      selectedTags = new Set();
      selectedAuthors = new Set(watchlist.map((a) => a.username));
      applyFiltersAndRender();
    });
  }

  if (els.themeToggle) els.themeToggle.addEventListener("click", toggleTheme);

  if (els.openSettings) {
    els.openSettings.addEventListener("click", () => {
      setAddStatus("");
      setWebhookStatus("");
      renderWatchListUI();
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
  }

  if (els.watchList) {
    els.watchList.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-remove]");
      if (!btn) return;
      e.preventDefault();
      e.stopPropagation();
      const idx = Number(btn.getAttribute("data-remove"));
      if (!Number.isInteger(idx) || idx < 0 || idx >= watchlist.length) return;
      const removed = watchlist[idx];
      watchlist = watchlist.filter((_, i) => i !== idx);
      saveWatchlist(watchlist);
      if (removed) selectedAuthors.delete(removed.username);
      setAddStatus(removed ? `已移除 @${removed.username}` : "已移除", true);
      renderWatchListUI();
      applyFiltersAndRender();
    });
  }

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
    watchlist = watchlist.concat([{ username, name }]);
    saveWatchlist(watchlist);
    selectedAuthors.add(username);
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
      const base = (repoWatchlist || DEFAULT_WATCHLIST).slice();
      saveWatchlist(base);
      selectedAuthors = new Set(base.map((a) => a.username));
      renderWatchListUI();
      applyFiltersAndRender();
    });
  }

  if (els.saveWebhook) {
    els.saveWebhook.addEventListener("click", () => {
      const url = els.webhookUrl ? els.webhookUrl.value : "";
      const key = els.webhookKey ? els.webhookKey.value : "";
      if (!String(url || "").trim() || !String(key || "").trim()) {
        setWebhookStatus("请填写 Webhook URL 与 key", false);
        return;
      }
      try {
        // Validate URL shape without storing anything else.
        // eslint-disable-next-line no-new
        new URL(String(url).trim());
      } catch (_) {
        setWebhookStatus("Webhook URL 格式无效", false);
        return;
      }
      saveWebhookConfig(url, key);
      setWebhookStatus("已保存到本机（不会上传到仓库）", true);
      syncUpdateButton();
    });
  }

  if (els.triggerUpdate) {
    els.triggerUpdate.addEventListener("click", (e) => {
      e.preventDefault();
      triggerImmediateUpdate();
    });
  }

  syncUpdateButton();

  loadTheme();
  initFilterCollapse();
  loadData()
    .then(() => applyFiltersAndRender())
    .catch(showFatal);
})();
