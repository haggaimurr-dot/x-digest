/* Local, demand-loaded attachment readers. No server or CDN required. */
(() => {
  "use strict";
  const base = new URL("./vendor/", document.currentScript.src);
  const pending = new Map();
  function library(path) {
    if (!pending.has(path)) pending.set(path, new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = new URL(path, base).href;
      script.onload = resolve;
      script.onerror = () => { pending.delete(path); script.remove(); reject(new Error("本地预览库加载失败，请重试")); };
      document.head.append(script);
    }));
    return pending.get(path);
  }
  async function sanitize(html) {
    await library("dompurify/purify.min.js");
    return DOMPurify.sanitize(html, {USE_PROFILES: {html: true}, FORBID_TAGS: ["style", "form", "input", "button", "textarea", "video", "audio"], FORBID_ATTR: ["style", "srcset"]});
  }
  async function markdown(text) {
    await library("marked/marked.umd.js");
    return sanitize(marked.parse(text, {breaks: true, gfm: true}));
  }
  function xml(text) {
    const doc = new DOMParser().parseFromString(text, "application/xml");
    if (doc.querySelector("parsererror")) throw new Error("Office XML 文件损坏");
    return doc;
  }
  const tags = (root, name) => [...root.getElementsByTagNameNS("*", name)];
  const relationId = node => node.getAttributeNS("http://schemas.openxmlformats.org/officeDocument/2006/relationships", "id");
  function resolvePath(from, target) {
    const path = new URL(target, "https://office.invalid/" + from);
    if (path.origin !== "https://office.invalid") return null;
    return decodeURIComponent(path.pathname.slice(1));
  }
  async function openZip(buffer) {
    await library("jszip/jszip.min.js");
    const zip = await JSZip.loadAsync(buffer);
    const files = Object.values(zip.files);
    // Bound expanded data before reading XML or embedded images.
    if (files.length > 3000 || files.reduce((sum, f) => sum + (f._data?.uncompressedSize || 0), 0) > 60 * 1024 * 1024) throw new Error("解压内容过大（上限 60MB / 3000 项）");
    return zip;
  }
  async function readXML(zip, path) {
    const file = zip.file(path);
    if (!file) throw new Error("文档缺少 " + path);
    return xml(await file.async("string"));
  }
  async function relationships(zip, path) {
    const slash = path.lastIndexOf("/");
    const entry = zip.file(path.slice(0, slash + 1) + "_rels/" + path.slice(slash + 1) + ".rels");
    const map = new Map();
    if (entry) for (const rel of tags(xml(await entry.async("string")), "Relationship")) {
      if (rel.getAttribute("TargetMode") !== "External") map.set(rel.getAttribute("Id"), resolvePath(path, rel.getAttribute("Target")));
    }
    return map;
  }
  async function slides(buffer, host, makeURL, current) {
    const zip = await openZip(buffer);
    const presentation = await readXML(zip, "ppt/presentation.xml");
    const rels = await relationships(zip, "ppt/presentation.xml");
    const paths = tags(presentation, "sldId").map(node => rels.get(relationId(node)));
    if (!paths.length || paths.some(path => !path)) throw new Error("没有可读取的幻灯片");
    if (!current()) return;
    host.replaceChildren();
    const help = document.createElement("p"); help.className = "preview-hint"; help.textContent = "按幻灯片顺序展示文字和内嵌图片；不还原原版排版、主题、动画、图表或母版。";
    const controls = document.createElement("div"); controls.className = "slide-controls";
    const prev = document.createElement("button"), next = document.createElement("button"), label = document.createElement("span");
    prev.textContent = "上一页"; next.textContent = "下一页"; label.setAttribute("aria-live", "polite");
    controls.append(prev, label, next);
    const page = document.createElement("section"); page.className = "ppt-slide rich-content"; page.setAttribute("aria-label", "幻灯片内容");
    host.append(help, controls, page);
    let index = 0, turn = 0;
    async function show() {
      const thisTurn = ++turn, path = paths[index];
      label.textContent = `${index + 1} / ${paths.length}`; prev.disabled = index === 0; next.disabled = index === paths.length - 1;
      page.textContent = "正在加载本页…";
      try {
        const doc = await readXML(zip, path), rels = await relationships(zip, path);
        const fragment = document.createDocumentFragment();
        const tree = tags(doc, "spTree")[0];
        for (const shape of tree ? [...tree.children] : []) {
          // Paragraphs in text boxes and tables, including grouped shapes.
          for (const paragraph of tags(shape, "p")) {
            const text = tags(paragraph, "t").map(t => t.textContent).join("");
            if (text) { const p = document.createElement("p"); p.textContent = text; fragment.append(p); }
          }
          for (const blip of tags(shape, "blip")) {
            const id = blip.getAttributeNS("http://schemas.openxmlformats.org/officeDocument/2006/relationships", "embed");
            const target = rels.get(id), image = target && zip.file(target);
            if (!image) continue;
            const ext = target.split(".").pop().toLowerCase();
            if (!/^(png|jpg|jpeg|gif|webp|bmp)$/.test(ext)) { const p = document.createElement("p"); p.textContent = "此内嵌图片格式暂不支持，请下载查看。"; fragment.append(p); continue; }
            const bytes = await image.async("uint8array");
            if (!current() || turn !== thisTurn) return;
            const img = document.createElement("img"); img.alt = "幻灯片内嵌图片"; img.src = makeURL(new Blob([bytes], {type: "image/" + (ext === "jpg" ? "jpeg" : ext)})); fragment.append(img);
          }
        }
        if (current() && turn === thisTurn) { page.replaceChildren(fragment); if (!page.childNodes.length) page.textContent = "本页无可读取的文字或图片，请下载查看原稿。"; }
      } catch (error) { if (current() && turn === thisTurn) page.textContent = `本页解析失败：${error.message}。请下载查看。`; }
    }
    prev.onclick = () => { index--; show(); }; next.onclick = () => { index++; show(); };
    await show();
  }
  const stream = (cfb, name) => { const data = CFB.find(cfb, name)?.content; return data ? Uint8Array.from(data) : null; };
  function wordText(cfb) {
    const word = stream(cfb, "WordDocument");
    if (!word || word.length < 426) return "";
    const view = new DataView(word.buffer, word.byteOffset, word.byteLength);
    const table = stream(cfb, view.getUint16(10, true) & 0x200 ? "1Table" : "0Table");
    if (!table) return "";
    const tv = new DataView(table.buffer, table.byteOffset, table.byteLength);
    // Word 97–2003 FIB fcClx/lcbClx point to the piece table.
    let pos = view.getUint32(418, true), end = pos + view.getUint32(422, true);
    if (end > table.length) return "";
    while (pos < end && table[pos] === 1) { pos += 3 + tv.getUint16(pos + 1, true); }
    if (pos + 5 > end || table[pos] !== 2) return "";
    const size = tv.getUint32(pos + 1, true), count = (size - 4) / 12, start = pos + 5;
    if (!Number.isInteger(count) || count < 0 || count > 100000 || start + size > table.length) return "";
    const out = [];
    for (let i = 0; i < count; i++) {
      const chars = tv.getUint32(start + (i + 1) * 4, true) - tv.getUint32(start + i * 4, true);
      const fc = tv.getUint32(start + (count + 1) * 4 + i * 8 + 2, true);
      const compressed = Boolean(fc & 0x40000000), offset = (fc & 0x3fffffff) / (compressed ? 2 : 1), length = chars * (compressed ? 1 : 2);
      if (offset + length > word.length || chars > 2000000) continue;
      out.push(new TextDecoder(compressed ? "windows-1252" : "utf-16le").decode(word.subarray(offset, offset + length)));
    }
    return out.join("").replace(/[\x00-\x08\x0e-\x1f]/g, "").replace(/[\r\x0b\x0c\x07]/g, "\n").trim();
  }
  function pptText(cfb) {
    const bytes = stream(cfb, "PowerPoint Document");
    if (!bytes) return "";
    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength), out = [];
    function walk(start, end, depth) {
      if (depth > 30) return;
      for (let pos = start; pos + 8 <= end;) {
        const version = view.getUint16(pos, true) & 15, type = view.getUint16(pos + 2, true), size = view.getUint32(pos + 4, true), next = pos + 8 + size;
        if (next > end) break;
        if (type === 4000 || type === 4008) out.push(new TextDecoder(type === 4000 ? "utf-16le" : "windows-1252").decode(bytes.subarray(pos + 8, next)));
        else if (version === 15) walk(pos + 8, next, depth + 1);
        pos = next;
      }
    }
    walk(0, bytes.length, 0);
    return [...new Set(out)].join("\n").replace(/\r/g, "\n").trim();
  }
  let pdfLibrary;
  async function pdf(buffer, host, current, registerCleanup) {
    pdfLibrary ||= import(new URL("pdfjs/build/pdf.min.mjs", base).href).catch(error => { pdfLibrary = null; throw error; });
    const lib = await pdfLibrary;
    if (!current()) return;
    lib.GlobalWorkerOptions.workerSrc = new URL("pdfjs/build/pdf.worker.min.mjs", base).href;
    const task = lib.getDocument({data: buffer, cMapUrl: new URL("pdfjs/cmaps/", base).href, cMapPacked: true,
      standardFontDataUrl: new URL("pdfjs/standard_fonts/", base).href, wasmUrl: new URL("pdfjs/wasm/", base).href,
      isEvalSupported: false});
    let disposed = false, rendering, turn = 0, resizeTimer;
    const observer = new ResizeObserver(() => { clearTimeout(resizeTimer); resizeTimer = setTimeout(() => { if (doc && !disposed && current()) show(); }, 160); });
    let doc;
    registerCleanup(() => {
      disposed = true; turn++; clearTimeout(resizeTimer); observer.disconnect(); rendering?.cancel();
      task.destroy().catch(() => {});
    });
    doc = await task.promise;
    if (disposed || !current()) return;
    host.className = "pdf-preview";
    host.innerHTML = '<div class="pdf-controls"><button type="button" data-pdf="prev">上一页</button><span class="pdf-page-label" aria-live="polite"></span><button type="button" data-pdf="next">下一页</button><button type="button" data-pdf="out" aria-label="缩小 PDF">−</button><span class="pdf-zoom-label"></span><button type="button" data-pdf="in" aria-label="放大 PDF">＋</button><button type="button" data-pdf="fit">适合宽度</button></div><p class="preview-hint" role="status"></p><div class="pdf-stage" tabindex="0" aria-label="PDF 页面，可滚动查看放大内容"></div>';
    const stage = host.querySelector('.pdf-stage'), hint = host.querySelector('[role="status"]');
    const button = name => host.querySelector(`[data-pdf="${name}"]`);
    let index = 1, zoom = 1;
    async function show() {
      const ticket = ++turn;
      rendering?.cancel();
      host.querySelector('.pdf-page-label').textContent = `${index} / ${doc.numPages}`;
      host.querySelector('.pdf-zoom-label').textContent = `${Math.round(zoom * 100)}%`;
      button('prev').disabled = index === 1; button('next').disabled = index === doc.numPages;
      button('out').disabled = zoom <= .5; button('in').disabled = zoom >= 3;
      hint.textContent = '正在绘制本页…'; stage.replaceChildren();
      try {
        const page = await doc.getPage(index);
        if (disposed || !current() || ticket !== turn) return;
        const original = page.getViewport({scale: 1});
        const scale = Math.max(1, stage.clientWidth - 2) / original.width * zoom;
        const viewport = page.getViewport({scale});
        // Bound bitmap memory on high-density phones and unusually large pages.
        const ratio = Math.min(devicePixelRatio || 1, 2, Math.sqrt(8000000 / (viewport.width * viewport.height)));
        const canvas = document.createElement('canvas');
        canvas.setAttribute('aria-label', `PDF 第 ${index} 页`); canvas.setAttribute('role', 'img');
        canvas.width = Math.ceil(viewport.width * ratio); canvas.height = Math.ceil(viewport.height * ratio);
        canvas.style.width = `${viewport.width}px`; canvas.style.height = `${viewport.height}px`;
        stage.append(canvas);
        rendering = page.render({canvasContext: canvas.getContext('2d'), viewport, transform: [ratio, 0, 0, ratio, 0, 0]});
        await rendering.promise;
        if (disposed || !current() || ticket !== turn) return;
        canvas.dataset.rendered = 'true'; hint.textContent = `第 ${index} 页已绘制；可翻页或缩放查看。`;
        page.cleanup();
      } catch (error) {
        if (!disposed && current() && ticket === turn && error.name !== 'RenderingCancelledException') hint.textContent = `本页绘制失败：${error.message}。请重试或下载原文件。`;
      }
    }
    button('prev').onclick = () => { index--; show(); };
    button('next').onclick = () => { index++; show(); };
    button('out').onclick = () => { zoom = Math.max(.5, zoom - .25); show(); };
    button('in').onclick = () => { zoom = Math.min(3, zoom + .25); show(); };
    button('fit').onclick = () => { zoom = 1; show(); };
    observer.observe(stage);
    await show();
  }
  async function render(file, host, makeURL, current, registerCleanup) {
    const ext = file.name.split(".").pop().toLowerCase();
    if (ext === "md") { const html = await markdown(await file.blob.text()); if (current()) { host.className = "rich-content"; host.innerHTML = html; } return; }
    const buffer = await file.blob.arrayBuffer();
    if (ext === "pdf") return pdf(buffer, host, current, registerCleanup);
    if (ext === "pptx") return slides(buffer, host, makeURL, current);
    if (ext === "docx") {
      await openZip(buffer);
      await library("mammoth/mammoth.browser.min.js");
      const result = await mammoth.convertToHtml({arrayBuffer: buffer});
      const html = await sanitize(result.value);
      if (current()) {
        host.className = "rich-content"; host.innerHTML = html;
        const hint = document.createElement("p"); hint.className = "preview-hint"; hint.textContent = "文档内容预览：保留标题、列表、表格和内嵌图片，不保证分页与原稿排版一致。"; host.prepend(hint);
        if (!result.value) host.append("未提取到内容，请下载原文件查看。");
      }
      return;
    }
    await library("cfb/cfb.min.js");
    const cfb = CFB.read(new Uint8Array(buffer), {type: "array"});
    const text = ext === "doc" ? wordText(cfb) : pptText(cfb);
    if (!current()) return;
    host.replaceChildren();
    const hint = document.createElement("p"); hint.className = "preview-hint";
    hint.textContent = text ? "旧版二进制格式：仅尽力提取可读文本，可能缺失或乱序；不支持原版排版、表格、图片或分页。请下载原文件核对。" : "无法从此旧版二进制文件提取可读文本（可能加密、损坏或版本不支持）。请下载后用 Office / LibreOffice 打开，或转换为 docx / pptx 后重新上传。";
    host.append(hint);
    if (text) { const pre = document.createElement("pre"); pre.textContent = text; host.append(pre); }
  }
  window.fukePreview = {markdown, render};
})();
