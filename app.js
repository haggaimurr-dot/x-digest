(() => {
  "use strict";

  const STORAGE_KEY = "x-digest-watchlist";
  const THEME_KEY = "x-digest-theme";
  const DEFAULT_WATCHLIST = [{ username: "ai_xiaomu", name: "黄小木" }];

  const els = {
    lastUpdated: document.getElementById("lastUpdated"),
    keyword: document.getElementById("keyword"),
    authorChips: document.getElementById("authorChips"),
    resultCount: document.getElementById("resultCount"),
    clearFilters: document.getElementById("clearFilters"),
    postsTableBody: document.getElementById("postsTableBody"),
    postsCards: document.getElementById("postsCards"),
    emptyState: document.getElementById("emptyState"),
    themeToggle: document.getElementById("themeToggle"),
    openSettings: document.getElementById("openSettings"),
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

  function formatDateShort(iso) {
    if (!iso) return "—";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return new Intl.DateTimeFormat("zh-CN", {
      timeZone: "Asia/Shanghai",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(d);
  }

  function loadTheme() {
    const saved = localStorage.getItem(THEME_KEY);
    const theme =
      saved ||
      (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    document.documentElement.setAttribute("data-theme", theme);
  }

  function toggleTheme() {
    const cur = document.documentElement.getAttribute("data-theme") || "light";
    const next = cur === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem(THEME_KEY, next);
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
        )}" aria-pressed="${selectedAuthors.has(u)}">@${escapeHtml(u)} · ${label}</button>`;
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

  function renderList() {
    const posts = filteredPosts();
    els.resultCount.textContent = `${posts.length} 条推文`;
    els.emptyState.hidden = posts.length > 0;

    els.postsTableBody.innerHTML = posts
      .map((p) => {
        const note = p.note
          ? `<span class="note-badge">${escapeHtml(p.note)}</span>`
          : "";
        return `<tr>
          <td class="title-cell">${escapeHtml(p.title)}${note}${stepsHtml(p)}</td>
          <td><span class="tag">${escapeHtml(p.industry || "—")}</span></td>
          <td>${escapeHtml(p.tech || "—")}</td>
          <td class="summary-cell">${escapeHtml(p.summary || "")}</td>
          <td>${escapeHtml(formatDateShort(p.publishedAt))}</td>
          <td><a class="author-link" href="https://x.com/${escapeHtml(
            p.author
          )}" target="_blank" rel="noopener">@${escapeHtml(p.author)}</a><br/><small>${escapeHtml(
          p.authorName || ""
        )}</small></td>
          <td><a class="ext-link" href="${escapeHtml(
            p.url
          )}" target="_blank" rel="noopener">打开 ↗</a></td>
        </tr>`;
      })
      .join("");

    els.postsCards.innerHTML = posts
      .map((p) => {
        const note = p.note
          ? `<span class="note-badge">${escapeHtml(p.note)}</span>`
          : "";
        return `<article class="card">
          <h3>${escapeHtml(p.title)}${note}</h3>
          <div class="card-meta">
            <span class="tag">${escapeHtml(p.industry || "—")}</span>
            <span>${escapeHtml(p.tech || "")}</span>
            <span>${escapeHtml(formatDateShort(p.publishedAt))}</span>
            <a class="author-link" href="https://x.com/${escapeHtml(
              p.author
            )}" target="_blank" rel="noopener">@${escapeHtml(p.author)}</a>
          </div>
          <p>${escapeHtml(p.summary || "")}</p>
          ${stepsHtml(p)}
          <p style="margin-top:10px"><a class="ext-link" href="${escapeHtml(
            p.url
          )}" target="_blank" rel="noopener">查看原文 ↗</a></p>
        </article>`;
      })
      .join("");
  }

  function renderWatchListUI() {
    els.watchList.innerHTML = watchlist
      .map(
        (a, i) => `<li>
        <div class="meta">
          <strong>${escapeHtml(a.name || a.username)}</strong>
          <span>@${escapeHtml(a.username)}</span>
        </div>
        <button type="button" class="btn btn-danger" data-remove="${i}">移除</button>
      </li>`
      )
      .join("");
  }

  function applyFiltersAndRender() {
    renderChips();
    renderList();
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
