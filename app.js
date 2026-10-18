(() => {
  "use strict";

  const STORAGE_KEY = "x-digest-watchlist";
  const THEME_KEY = "x-digest-theme";
  const DEFAULT_WATCHLIST = [{ username: "ai_xiaomu", name: "黄小木" }];
  const AVATAR_COLORS = [
    "var(--avatar-1)",
    "var(--avatar-2)",
    "var(--avatar-3)",
    "var(--avatar-4)",
    "var(--avatar-5)",
  ];

  const els = {
    lastUpdated: document.getElementById("lastUpdated"),
    keyword: document.getElementById("keyword"),
    authorChips: document.getElementById("authorChips"),
    resultCount: document.getElementById("resultCount"),
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
    newUsername: document.getElementById("newUsername"),
    newDisplayName: document.getElementById("newDisplayName"),
    resetWatchlist: document.getElementById("resetWatchlist"),
  };

  let allPosts = [];
  let watchlist = [];
  let selectedAuthors = new Set();
  let repoWatchlist = null;

  function escapeHtml(str) {
    return String(str ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
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

  function initials(name, username) {
    const src = (name || username || "?").trim();
    if (!src) return "?";
    // CJK: take first char; Latin: first letter
    const first = Array.from(src)[0];
    return /[A-Za-z]/.test(first) ? first.toUpperCase() : first;
  }

  function avatarColor(username) {
    let h = 0;
    const s = String(username || "");
    for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
    return AVATAR_COLORS[h % AVATAR_COLORS.length];
  }

  function loadTheme() {
    const saved = localStorage.getItem(THEME_KEY);
    // Dark is primary default (X aesthetic)
    const theme = saved || "dark";
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

  async function loadData() {
    const [postsRes, watchRes] = await Promise.all([
      fetch("data/posts.json", { cache: "no-store" }),
      fetch("config/watchlist.json", { cache: "no-store" }).catch(() => null),
    ]);
    if (!postsRes.ok) throw new Error("无法加载 data/posts.json");
    const data = await postsRes.json();
    allPosts = Array.isArray(data.posts) ? data.posts.slice() : [];
    allPosts.sort((a, b) => String(b.publishedAt).localeCompare(String(a.publishedAt)));

    els.lastUpdated.textContent = `最近更新：${formatShanghai(data.updatedAt)}`;

    if (watchRes && watchRes.ok) {
      const w = await watchRes.json();
      repoWatchlist = Array.isArray(w.accounts) ? w.accounts : DEFAULT_WATCHLIST;
    } else {
      repoWatchlist = DEFAULT_WATCHLIST;
    }

    const stored = loadWatchlistFromStorage();
    watchlist = stored && stored.length ? stored : repoWatchlist.slice();
    selectedAuthors = new Set(watchlist.map((a) => a.username));
  }

  function authorDisplay(username) {
    const hit = watchlist.find((a) => a.username === username);
    if (hit) return hit.name || hit.username;
    const fromPost = allPosts.find((p) => p.author === username);
    return fromPost?.authorName || username;
  }

  function renderChips() {
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

  function stepsHtml(post) {
    const steps = Array.isArray(post.steps) ? post.steps.filter(Boolean) : [];
    if (!post.isLong || steps.length === 0) return "";
    const items = steps.map((s) => `<li>${escapeHtml(s)}</li>`).join("");
    return `<details class="steps"><summary>实现步骤（${steps.length}）</summary><ol>${items}</ol></details>`;
  }

  function filteredPosts() {
    const q = els.keyword.value.trim().toLowerCase();
    return allPosts.filter((p) => {
      if (selectedAuthors.size && !selectedAuthors.has(p.author)) return false;
      if (!q) return true;
      const hay = [
        p.title,
        p.summary,
        p.industry,
        p.tech,
        p.author,
        p.authorName,
        ...(Array.isArray(p.steps) ? p.steps : []),
      ]
        .join("\n")
        .toLowerCase();
      return hay.includes(q);
    });
  }

  function renderCard(p) {
    const name = p.authorName || authorDisplay(p.author);
    const note = p.note
      ? `<span class="note-pill">${escapeHtml(p.note)}</span>`
      : "";
    const tags = [
      p.industry ? `<span class="tag">${escapeHtml(p.industry)}</span>` : "",
      p.tech ? `<span class="tag">${escapeHtml(p.tech)}</span>` : "",
    ].join("");

    return `<article class="post-card">
      <div class="avatar" style="background:${avatarColor(p.author)}" aria-hidden="true">${escapeHtml(
        initials(name, p.author)
      )}</div>
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
        <p class="post-summary">${escapeHtml(p.summary || "")}</p>
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
    els.resultCount.textContent = `${posts.length} 条推文`;
    els.emptyState.hidden = posts.length > 0;
    els.timeline.innerHTML = posts.map(renderCard).join("");
  }

  function renderWatchListUI() {
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
    renderList();
  }

  function closeModal() {
    if (els.settingsModal.open) els.settingsModal.close();
  }

  els.authorChips.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-author]");
    if (!btn) return;
    const u = btn.getAttribute("data-author");
    if (selectedAuthors.has(u)) selectedAuthors.delete(u);
    else selectedAuthors.add(u);
    applyFiltersAndRender();
  });

  els.keyword.addEventListener("input", () => renderList());

  els.clearFilters.addEventListener("click", () => {
    els.keyword.value = "";
    selectedAuthors = new Set(watchlist.map((a) => a.username));
    applyFiltersAndRender();
  });

  els.themeToggle.addEventListener("click", toggleTheme);

  els.openSettings.addEventListener("click", () => {
    renderWatchListUI();
    els.settingsModal.showModal();
  });

  els.closeSettings.addEventListener("click", closeModal);
  els.doneSettings.addEventListener("click", closeModal);

  els.settingsModal.addEventListener("click", (e) => {
    if (e.target === els.settingsModal) closeModal();
  });

  els.watchList.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-remove]");
    if (!btn) return;
    const idx = Number(btn.getAttribute("data-remove"));
    const removed = watchlist[idx];
    watchlist = watchlist.filter((_, i) => i !== idx);
    saveWatchlist(watchlist);
    if (removed) selectedAuthors.delete(removed.username);
    renderWatchListUI();
    applyFiltersAndRender();
  });

  els.addAuthor.addEventListener("click", () => {
    const username = els.newUsername.value.trim().replace(/^@/, "");
    if (!/^[A-Za-z0-9_]{1,15}$/.test(username)) {
      els.newUsername.focus();
      return;
    }
    if (watchlist.some((a) => a.username.toLowerCase() === username.toLowerCase())) {
      els.newUsername.value = "";
      return;
    }
    const name = els.newDisplayName.value.trim() || username;
    watchlist.push({ username, name });
    saveWatchlist(watchlist);
    selectedAuthors.add(username);
    els.newUsername.value = "";
    els.newDisplayName.value = "";
    renderWatchListUI();
    applyFiltersAndRender();
  });

  els.resetWatchlist.addEventListener("click", () => {
    const base = (repoWatchlist || DEFAULT_WATCHLIST).slice();
    saveWatchlist(base);
    selectedAuthors = new Set(base.map((a) => a.username));
    renderWatchListUI();
    applyFiltersAndRender();
  });

  loadTheme();
  loadData()
    .then(() => applyFiltersAndRender())
    .catch((err) => {
      els.lastUpdated.textContent = "数据加载失败";
      els.emptyState.hidden = false;
      els.emptyState.textContent = String(err.message || err);
      console.error(err);
    });
})();
