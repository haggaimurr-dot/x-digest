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
      if (status(s) === "done") done += 1;
      for (const sub of s.substeps || []) {
        total += 1;
        if (status(sub) === "done") done += 1;
      }
    }
    return { done, total };
  }

  function newReplicaId() {
    return "r" + Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
  }

  const status = step => ["idle", "active", "done"].includes(step.status) ? step.status : step.done ? "done" : "idle";
  const statusNames = { idle: "未开始", active: "进行中", done: "已完成" };
  function statusButton(r, step, idx, sub = "") {
    const state = status(step);
    return `<button type="button" class="status-dot is-${state}" data-status="${escapeHtml(r.id)}" data-step-idx="${idx}" data-sub-idx="${sub}" aria-label="${sub === "" ? "步骤" : "子步骤"} ${idx + 1}${sub === "" ? "" : "." + (sub + 1)}：${statusNames[state]}，点击切换状态" title="${statusNames[state]}，点击切换状态"></button>`;
  }
  async function deleteReplica(id) {
    await attachmentDB("readwrite", store => {
      const request = store.openCursor();
      request.onsuccess = () => { const cursor = request.result; if (cursor) { if (cursor.value.replicaId === id) cursor.delete(); cursor.continue(); } };
    });
    replicas = replicas.filter(r => r.id !== id);
    saveReplicas();
  }
  // Android uses accept/capture to choose the picker; validation remains extension-based.
  const attachmentExtensions = "pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif webp bmp avif svg ico zip rar 7z tar gz tgz".split(" ");
  const imageAttachmentAccept = "image/*";
  const maxAttachmentSize = 10 * 1024 * 1024;
  function noteEditor(r, step, idx) {
    const tools = [["bold", "加粗"], ["italic", "斜体"], ["heading", "标题"], ["list", "列表"], ["link", "链接"], ["code", "代码"], ["quote", "引用"]];
    return `<div class="note-editor"><div class="note-controls"><div class="note-modes" role="group" aria-label="备注模式"><button type="button" data-note-mode="edit" aria-pressed="true">编辑</button><button type="button" data-note-mode="preview" aria-pressed="false">预览</button></div><span class="note-save" role="status">Markdown · 自动保存</span></div><div class="note-toolbar" role="toolbar" aria-label="Markdown 工具栏">${tools.map(([action, label]) => `<button type="button" data-note-tool="${action}" title="${label}" aria-label="${label}">${label}</button>`).join("")}</div><textarea class="replica-note" rows="4" data-replica-note="${escapeHtml(r.id)}" data-step-idx="${idx}" aria-label="步骤 ${idx + 1} 备注" placeholder="写点备注… 支持 Markdown">${escapeHtml(step.note || "")}</textarea><div class="note-preview rich-content" aria-label="备注预览" hidden></div></div>`;
  }
  function replicaStepHtml(r, step, idx) {
    const subs = (step.substeps || []).map((sub, j) => `<li class="replica-substep-row ${status(sub) === "done" ? "is-done" : ""}">${statusButton(r, sub, idx, j)}<span class="replica-substep-text">${escapeHtml(sub.text)}</span></li>`).join("");
    return window.xdigestFlowNode(`
      <div class="replica-step-row">${statusButton(r, step, idx)}<span class="replica-step-text">${idx + 1}. ${escapeHtml(step.text)}</span></div>
      ${subs ? `<ol class="replica-substeps">${subs}</ol>` : ""}
      ${noteEditor(r, step, idx)}
      <section class="step-attachments" data-attachment-section="${escapeHtml(r.id)}" data-step-idx="${idx}">
        <div class="attachment-heading"><strong>阶段附件</strong><div class="attachment-upload-actions"><label class="attachment-upload">＋ 上传图片<input type="file" multiple accept="${imageAttachmentAccept}" data-upload="${escapeHtml(r.id)}" data-step-idx="${idx}" aria-label="步骤 ${idx + 1} 上传图片" /></label><label class="attachment-upload">＋ 上传文档/压缩包<input type="file" multiple data-upload="${escapeHtml(r.id)}" data-step-idx="${idx}" aria-label="步骤 ${idx + 1} 上传文档或压缩包" /></label></div></div>
        <p class="attachment-help">文档、图片、压缩包 · 单个不超过 10MB · 仅保存在当前浏览器</p>
        <div class="attachment-list">${(step.attachments || []).map(id => `<div class="attachment-item" data-attachment-id="${escapeHtml(id)}">加载附件…</div>`).join("")}</div>
      </section>
    `, idx, idx === r.steps.length - 1, `replica-flow-step ${status(step) === "done" ? "is-done" : ""}`);
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
    hydrateAttachments();
  }

  if (els.replicaSort) {
    els.replicaSort.addEventListener("change", (e) => {
      replicaSortMode = e.target.value === "progress" ? "progress" : "time";
      renderReplicasPage();
    });
  }

  if (els.replicaList) {
    els.replicaList.addEventListener("change", async e => {
      const input = e.target.closest("[data-upload]");
      if (!input) return;
      const r = replicas.find(r => r.id === input.dataset.upload), step = r?.steps[input.dataset.stepIdx];
      if (!step) return;
      for (const file of input.files) {
        if (!isAllowedAttachment(file)) { alert(`${file.name} 类型不支持，请选择文档、图片或压缩包。`); continue; }
        if (file.size > maxAttachmentSize) { alert(`${file.name} 超过 10MB，已拒绝上传。`); continue; }
        const id = newReplicaId();
        try {
          await attachmentDB("readwrite", store => store.put({id, replicaId: r.id, name: file.name, type: file.type, blob: file}));
          (step.attachments ||= []).push(id);
          try { saveReplicas(); } catch (error) { step.attachments.pop(); await attachmentDB("readwrite", store => store.delete(id)); throw error; }
        } catch (error) { alert(`附件保存失败：${error.message}`); }
      }
      input.value = "";
      renderReplicasPage();
    });

    els.replicaList.addEventListener("input", (e) => {
      const ta = e.target.closest("[data-replica-note]");
      if (!ta) return;
      const r = replicas.find((x) => x.id === ta.getAttribute("data-replica-note"));
      const idx = Number(ta.getAttribute("data-step-idx"));
      if (!r || !r.steps[idx]) return;
      r.steps[idx].note = ta.value;
      const saved = ta.closest(".note-editor").querySelector(".note-save");
      try { saveReplicas(); saved.textContent = "已自动保存"; }
      catch (_) { saved.textContent = "保存失败，请复制备注备份（存储空间不足）"; }
    });

    function applyNoteTool(editor, tool) {
      const ta = editor.querySelector("textarea");
      const start = ta.selectionStart, end = ta.selectionEnd;
      const selected = ta.value.slice(start, end);
      const pairs = {bold: ["**", "**", "加粗文字"], italic: ["*", "*", "斜体文字"], link: ["[", "](https://example.com)", "链接文字"], code: ["`", "`", "代码"]};
      let replacement, selectionStart, selectionEnd;
      if (pairs[tool]) {
        const [before, after, fallback] = pairs[tool];
        replacement = before + (selected || fallback) + after;
        selectionStart = start + before.length; selectionEnd = selectionStart + (selected || fallback).length;
      } else {
        const prefix = {heading: "## ", list: "- ", quote: "> "}[tool];
        const lineStart = ta.value.lastIndexOf("\n", start - 1) + 1;
        const chunk = ta.value.slice(lineStart, end) || "文字";
        replacement = chunk.split("\n").map(line => prefix + line).join("\n");
        ta.setSelectionRange(lineStart, end);
        selectionStart = lineStart; selectionEnd = lineStart + replacement.length;
      }
      ta.setRangeText(replacement, ta.selectionStart, ta.selectionEnd, "end");
      ta.focus(); ta.setSelectionRange(selectionStart, selectionEnd);
      ta.dispatchEvent(new Event("input", {bubbles: true}));
    }
    els.replicaList.addEventListener("keydown", e => {
      if (e.target.matches("[data-replica-note]") && (e.ctrlKey || e.metaKey) && ["b", "i"].includes(e.key.toLowerCase())) {
        e.preventDefault(); applyNoteTool(e.target.closest(".note-editor"), e.key.toLowerCase() === "b" ? "bold" : "italic");
      }
    });
    els.replicaList.addEventListener("click", async (e) => {
      try {
        const tool = e.target.closest("[data-note-tool]");
        if (tool) { applyNoteTool(tool.closest(".note-editor"), tool.dataset.noteTool); return; }
        const mode = e.target.closest("[data-note-mode]");
        if (mode) {
          const editor = mode.closest(".note-editor"), show = mode.dataset.noteMode === "preview";
          editor.querySelectorAll("[data-note-mode]").forEach(b => b.setAttribute("aria-pressed", String(b === mode)));
          editor.querySelector("textarea").hidden = show;
          editor.querySelector(".note-toolbar").hidden = show;
          const target = editor.querySelector(".note-preview"); target.hidden = !show;
          const token = {}; editor.previewToken = token;
          if (show) {
            target.textContent = "正在加载预览…";
            try { const html = await window.fukePreview.markdown(editor.querySelector("textarea").value); if (editor.previewToken === token) target.innerHTML = html || "暂无备注"; }
            catch (_) { if (editor.previewToken === token) target.textContent = "预览加载失败，请切回编辑后重试。"; }
          }
          return;
        }
        const dot = e.target.closest("[data-status]");
        if (dot) {
          const r = replicas.find(r => r.id === dot.dataset.status);
          const step = r.steps[dot.dataset.stepIdx];
          const target = dot.dataset.subIdx === "" ? step : step.substeps[dot.dataset.subIdx];
          target.status = {idle: "active", active: "done", done: "idle"}[status(target)];
          target.done = target.status === "done";
          saveReplicas(); renderReplicasPage(); return;
        }
        const action = e.target.closest("[data-attachment-action]");
        if (action) { await attachmentAction(action); return; }
      } catch (error) { alert(`操作失败：${error.message}`); return; }
      const delBtn = e.target.closest("[data-replica-delete]");
      if (!delBtn) return;
      e.preventDefault();
      const id = delBtn.getAttribute("data-replica-delete");
      const r = replicas.find((x) => x.id === id);
      if (!r) return;
      confirmDelete("删除这条复刻？", `「${r.postTitle || "该流程"}」的状态、备注和全部附件都会被删除`).then(async (ok) => {
        if (!ok) return;
        await deleteReplica(id);
        renderReplicasPage();

      }).catch(error => alert(`删除失败：${error.message}`));
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
  let dbPromise;
  function attachmentDB(mode, action) {
    dbPromise ||= new Promise((resolve, reject) => {
      const request = indexedDB.open("xdigest.attachments", 1);
      request.onupgradeneeded = () => request.result.createObjectStore("files", {keyPath: "id"});
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => { dbPromise = null; reject(request.error); };
    });
    return dbPromise.then(db => new Promise((resolve, reject) => {
      const tx = db.transaction("files", mode);
      const request = action(tx.objectStore("files"));
      tx.oncomplete = () => resolve(request?.result);
      tx.onerror = tx.onabort = () => reject(tx.error || new Error("附件存储失败"));
    }));
  }
  const objectURLs = new Set();
  function objectURL(blob) { const url = URL.createObjectURL(blob); objectURLs.add(url); return url; }
  function clearURLs() { objectURLs.forEach(url => URL.revokeObjectURL(url)); objectURLs.clear(); }
  function isAllowedAttachment(file) {
    const ext = file.name.includes(".") ? file.name.split(".").pop().toLowerCase() : "";
    return attachmentExtensions.includes(ext);
  }
  function fileKind(file) {
    const ext = file.name.split(".").pop().toLowerCase();
    if (/^(png|jpg|jpeg|gif|webp|bmp|avif|svg|ico)$/.test(ext) || (file.type || "").startsWith("image/")) return "image";
    if (ext === "pdf") return "pdf";
    if (/^(txt|md|csv)$/.test(ext)) return "text";
    if (ext === "zip") return "zip";
    if (/^(rar|7z|tar|gz|tgz)$/.test(ext)) return "archive";
    return "document";
  }
  async function hydrateAttachments() {
    clearURLs();
    await Promise.all([...els.replicaList.querySelectorAll("[data-attachment-id]")].map(async node => {
      try {
        const file = await attachmentDB("readonly", store => store.get(node.dataset.attachmentId));
        if (!node.isConnected) return;
        if (!file) { node.textContent = "附件不存在（可能已清除浏览器数据）"; return; }
        const kind = fileKind(file);
        node.innerHTML = `${kind === "image" ? `<img class="attachment-thumb" alt="${escapeHtml(file.name)}" src="${objectURL(file.blob)}" />` : `<span class="attachment-icon" aria-hidden="true">${kind === "archive" || kind === "zip" ? "🗜" : "📄"}</span>`}<span class="attachment-name">${escapeHtml(file.name)}<small>${(file.blob.size / 1024).toFixed(1)} KB</small></span><div class="attachment-actions"><button type="button" data-attachment-action="preview">预览</button><button type="button" data-attachment-action="download">下载</button><button type="button" data-attachment-action="delete">删除</button></div>`;
        if (kind === "image") node.querySelector("img").onclick = () => node.querySelector("button").click();
      } catch (error) { if (node.isConnected) node.textContent = `附件读取失败：${error.message}`; }
    }));
  }
  const preview = document.getElementById("attachmentPreview");
  let previewToken = 0;
  let previewURLs = [];
  let previewCleanup;
  function resetPreview() {
    if (previewCleanup) { previewCleanup(); previewCleanup = null; }
    previewToken++;
    document.getElementById("attachmentPreviewBody").replaceChildren();
    previewURLs.forEach(url => URL.revokeObjectURL(url)); previewURLs = [];
  }
  preview.addEventListener("close", resetPreview);
  document.getElementById("attachmentPreviewClose").onclick = () => preview.close();
  function zipEntries(buffer) {
    const view = new DataView(buffer), decoder = new TextDecoder();
    let end = buffer.byteLength - 22;
    for (; end >= Math.max(0, buffer.byteLength - 65557); end--) {
      if (view.getUint32(end, true) === 0x06054b50 && end + 22 + view.getUint16(end + 20, true) === buffer.byteLength) break;
    }
    if (end < Math.max(0, buffer.byteLength - 65557)) throw new Error("ZIP 目录损坏或不支持此格式");
    const count = view.getUint16(end + 10, true);
    let offset = view.getUint32(end + 16, true);
    const entries = [];
    for (let i = 0; i < count; i++) {
      if (offset + 46 > end || view.getUint32(offset, true) !== 0x02014b50) throw new Error("ZIP 目录不完整（不支持 ZIP64 或分卷）");
      const length = view.getUint16(offset + 28, true), extra = view.getUint16(offset + 30, true), comment = view.getUint16(offset + 32, true);
      if (offset + 46 + length + extra + comment > end) throw new Error("ZIP 目录损坏");
      entries.push(`${decoder.decode(new Uint8Array(buffer, offset + 46, length))}  (${view.getUint32(offset + 24, true)} 字节)`);
      offset += 46 + length + extra + comment;
    }
    return entries;
  }
  async function attachmentAction(button) {
    const node = button.closest("[data-attachment-id]"), id = node.dataset.attachmentId;
    const file = await attachmentDB("readonly", store => store.get(id));
    if (!file) throw new Error("附件不存在");
    if (button.dataset.attachmentAction === "delete") {
      if (!await confirmDelete("删除附件？", `「${file.name}」将从浏览器中永久删除。`)) return;
      await attachmentDB("readwrite", store => store.delete(id));
      replicas.forEach(r => r.steps?.forEach(step => { step.attachments = (step.attachments || []).filter(ref => ref !== id); }));
      saveReplicas(); renderReplicasPage(); return;
    }
    if (button.dataset.attachmentAction === "download") {
      const url = URL.createObjectURL(file.blob), a = document.createElement("a");
      a.href = url; a.download = file.name; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); return;
    }
    const body = document.getElementById("attachmentPreviewBody");
    document.getElementById("attachmentPreviewTitle").textContent = file.name;
    resetPreview();
    const token = previewToken;
    const current = () => token === previewToken && preview.open;
    const makeURL = blob => { if (!current()) throw new Error("预览已关闭"); const url = URL.createObjectURL(blob); previewURLs.push(url); return url; };
    const download = document.createElement("button"); download.type = "button"; download.textContent = "下载原文件";
    download.onclick = () => attachmentAction(node.querySelector('[data-attachment-action="download"]')).catch(error => alert(error.message));
    body.append(download);
    const contentHost = document.createElement("div"); body.append(contentHost);
    preview.showModal();
    const kind = fileKind(file);
    if (kind === "image") {
      const media = document.createElement("img");
      const previewURL = makeURL(new Blob([file.blob], {type: file.type || "image/" + file.name.split(".").pop()}));
      media.src = previewURL; media.setAttribute("alt", file.name); contentHost.append(media);
    } else if (/\.(pdf|docx|pptx|md|doc|ppt)$/i.test(file.name)) {
      contentHost.textContent = "正在解析附件…";
      try { await window.fukePreview.render(file, contentHost, makeURL, current, cleanup => { if (current()) previewCleanup = cleanup; else cleanup(); }); }
      catch (error) { if (current()) contentHost.textContent = `无法预览：${error.message}。请下载原文件查看。`; }
    } else {
      const content = document.createElement("pre");
      if (kind === "text") content.textContent = await file.blob.text();
      else if (kind === "zip") { try { const entries = zipEntries(await file.blob.arrayBuffer()); content.textContent = entries.join("\n") || "空压缩包"; } catch (error) { content.textContent = error.message; } }
      else content.textContent = `${kind === "archive" ? "🗜" : "📄"} ${file.name}\n${kind === "archive" ? "此压缩格式暂不支持列出目录，请下载后查看。" : "此文档格式请下载后打开。"}\n文件大小：${file.blob.size} 字节`;
      if (current()) contentHost.append(content);
    }
  }
  window.addEventListener("pagehide", clearURLs);
  document.documentElement.dataset.theme = localStorage.getItem("x-digest-theme") || "dark";
  loadReplicas();
  renderReplicasPage();
})();
