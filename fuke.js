(() => {
  "use strict";
  const REPLICAS_KEY = "xdigest.replicas";
  let replicas = [], replicaSortMode = "time";
  const expanded = new Set();
  const els = Object.fromEntries(["replicaList", "replicaSort", "replicaEmpty", "replicaCount", "confirmModal", "confirmTitle", "confirmBody", "confirmCancel"].map(id => [id, document.getElementById(id)]));
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
  }  function confirmDelete(title, body) {
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
  }  function loadReplicas() {
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

  function deleteReplica(id) {
    replicas = replicas.filter((r) => r.id !== id);
    saveReplicas();
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
    return window.xdigestFlowNode(`
      <label class="replica-step-row">
        <input type="checkbox" class="replica-check" data-replica-check="${escapeHtml(r.id)}" data-step-idx="${idx}" ${step.done ? "checked" : ""} aria-label="步骤 ${idx + 1} 完成" />
        <span class="replica-step-text">${idx + 1}. ${escapeHtml(step.text)}</span>
      </label>
      ${subs ? `<ol class="replica-substeps">${subs}</ol>` : ""}
      <textarea class="replica-note" rows="1" data-replica-note="${escapeHtml(r.id)}" data-step-idx="${idx}" aria-label="步骤 ${idx + 1} 备注" placeholder="写点备注…">${escapeHtml(step.note || "")}</textarea>
    `, idx, idx === r.steps.length - 1, `replica-flow-step ${step.done ? "is-done" : ""}`);
  }

  function replicaCardHtml(r) {
    const { done, total } = replicaProgress(r);
    const pct = total ? Math.round((done / total) * 100) : 0;
    const stepsListHtml = (r.steps || []).map((s, i) => replicaStepHtml(r, s, i)).join("");
    return `<article class="replica-card" data-replica-id="${escapeHtml(r.id)}">
      <details class="replica-details" ${expanded.has(r.id) ? "open" : ""}><summary class="replica-summary">
      <div class="replica-head">
        <div class="replica-source">
          <span class="replica-source-title">${escapeHtml(r.postTitle || "（无标题）")}</span>
          <div class="replica-source-meta">
            <span>${escapeHtml(r.postAuthorName || r.postAuthor || "")}</span>
            ${r.postAuthor ? `<span>@${escapeHtml(r.postAuthor)}</span>` : ""}
            ${r.postUrl ? `<a href="${escapeHtml(r.postUrl)}" target="_blank" rel="noopener">查看原帖</a>` : ""}
          </div>
          <div class="replica-time">复刻于 ${escapeHtml(formatShanghai(r.createdAt))}</div>
        </div>

      </div>
      <div class="replica-progress-row">
        <div class="replica-progress-bar"><div class="replica-progress-fill ${pct >= 100 ? "is-complete" : ""}" style="width:${pct}%"></div></div>
        <span class="replica-progress-label">${done}/${total} · ${pct}%</span>
      </div>
      <span class="replica-expand-hint">展开 / 收起流程</span></summary>
      <ol class="flow-chart">${stepsListHtml}</ol></details>        <button type="button" class="replica-delete" data-replica-delete="${escapeHtml(r.id)}" aria-label="删除复刻" title="删除复刻">
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path fill="currentColor" d="M9.75 3.75h4.5a.75.75 0 0 1 .75.75v.75h4.5a.75.75 0 0 1 0 1.5h-.564l-.853 12.013A2.25 2.25 0 0 1 15.84 21H8.16a2.25 2.25 0 0 1-2.244-2.237L5.064 6.75H4.5a.75.75 0 0 1 0-1.5H9v-.75a.75.75 0 0 1 .75-.75zm-3.182 3 .84 11.85a.75.75 0 0 0 .748.65h7.688a.75.75 0 0 0 .748-.65l.84-11.85H6.568zM10.5 9a.75.75 0 0 1 .75.75v6a.75.75 0 0 1-1.5 0v-6A.75.75 0 0 1 10.5 9zm3 0a.75.75 0 0 1 .75.75v6a.75.75 0 0 1-1.5 0v-6a.75.75 0 0 1 .75-.75z"/></svg>
        </button>
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
    els.replicaList.querySelectorAll("[data-replica-id]").forEach(card => {
      if (card.querySelector("details").open) expanded.add(card.dataset.replicaId);
      else expanded.delete(card.dataset.replicaId);
    });
    if (!els.replicaList) return;
    const list = sortedReplicas();
    if (els.replicaCount) els.replicaCount.textContent = `${list.length} 条复刻`;
    els.replicaList.innerHTML = list.map(replicaCardHtml).join("");
    if (els.replicaEmpty) els.replicaEmpty.hidden = list.length > 0;
  }

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


    els.replicaList.addEventListener("input", (e) => {
      const ta = e.target.closest("[data-replica-note]");
      if (!ta) return;
      const r = replicas.find((x) => x.id === ta.getAttribute("data-replica-note"));
      const idx = Number(ta.getAttribute("data-step-idx"));
      if (!r || !r.steps[idx]) return;
      r.steps[idx].note = ta.value;
      saveReplicas();
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

      });
    });
  }

  els.replicaList.addEventListener("toggle", event => {
    const details = event.target.closest(".replica-details");
    if (!details) return;
    const id = details.closest("[data-replica-id]").dataset.replicaId;
    if (details.open) expanded.add(id); else if (details.isConnected) expanded.delete(id);
  }, true);
  window.addEventListener("storage", event => {
    if (event.key === REPLICAS_KEY || event.key === null) { loadReplicas(); renderReplicasPage(); }
  });
  document.documentElement.dataset.theme = localStorage.getItem("x-digest-theme") || "dark";
  loadReplicas();
  renderReplicasPage();
})();
