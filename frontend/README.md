# x-digest 前端（Nuxt）

全量重构的前端部分。契约见 `../SPEC.md`，字段名、筛选语义、localStorage 键名均按 SPEC，不自创。

## 版本（已核实兼容性）

- Node **v24.22.1**（本机实测）
- Nuxt **4.6.2**（当前 stable `latest`，2026-10-24 核实；Nuxt 4 要求 Node ≥ 20.19，Node 24 满足）
- Vue 3.5.44、marked 17.0.6、mammoth 1.13.0、jszip 3.10.2
- 无 CDN：所有库均为 npm 本地打包（`node_modules` → 构建产物）。

## 启动方式

```bash
cd ~/workspace/xdigest-new/frontend
npm install
npm run dev        # 开发：http://127.0.0.1:18091（nuxt dev --port 18091）
npm run build      # 生产构建
```

生产启动（实测结论，见下方注意）：

```bash
PORT=18091 node .output/server/index.mjs
```

> ⚠️ 注意：任务书原写 `NUXT_PORT=18091 node .output/server/index.mjs`，**实测不生效**（服务仍监听默认 3000）。
> 内置 Nitro 服务器认的是 `PORT`（或 `NITRO_PORT`），已用 `PORT=18091` 验证通过并返回 200。
> 最终部署时 nginx 代理到 18091 即可，`PORT` 只决定 Nuxt 自身监听端口。

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `NUXT_SERVER_API_BASE` | `http://127.0.0.1:18090/api/v1` | 仅 SSR 侧使用（`composables/useApi.ts` 的 `apiBase()`）。浏览器侧一律用相对路径 `/api/v1/...`，由 nginx 代理到 Go API。 |
| `NUXT_PUBLIC_USE_MOCK` | （空 = 关闭） | 设为 `1` 开启本地 mock/静态数据开发 fallback。默认关闭。 |
| `PORT` / `NITRO_PORT` | `3000` | 生产服务器监听端口（见上）。 |

Mock 数据来源：原站 `data/posts.json`（190 条）、`config/watchlist.json`、`config/user-tags.json`，
已归一化为 SPEC 的 snake_case API 形状，放在 `assets/mock/`。Mock 下的复刻/分类改动只存浏览器
localStorage（复刻键沿用原站 `xdigest.replicas`），不写任何后端。

## 页面清单

| 路由 | 内容 |
|---|---|
| `/` 摘要页 | 帖子卡片时间线（published_at 倒序；头像/无头像首字母+颜色、显示名、@handle 链 x.com、相对时间+上海时间、行业/技术、标题、简介、标签最多 8 个可点、备注 pill、分类徽章、复刻/已复刻按钮、长文展开、实现步骤流程图竖向时间轴：步骤工具/技术/前提 chips、子步骤最多 5 条、每条截断 40 字、仅 `show_flowchart` 为 true 时显示、缺失时按原站启发式 `deriveShowFlowchart` 推导）；作者分组 chips（默认选中特别关注作者；**空选中 = 空列表**，与原站一致）；帖子标签 chips；自定义标签 chips；分类 chips（含“未分类”）；关键词搜索（含高亮、首命中片段，支持 `标签:xxx` / `tag:xxx` 只搜用户标签，标签字段去开头 `#` 再匹配）；日期范围（Asia/Shanghai 当天起止）；结果计数、已选筛选计数、清除全部；帖子卡片上的分类勾选/新建；设置弹窗；Webhook 触发按钮。 |
| `/fuke` 复刻页 | 从摘要页一键生成复刻（同一帖子不重复建，已存在返回已有）；步骤状态圆点三态循环 未开始→进行中→已完成（进度只按步骤计，不含子步骤）；Markdown 备注编辑+预览+工具栏（自研 textarea+工具栏+marked 预览，见差异）；附件上传（图片与文档两入口，白名单见下，单个 ≤5MB，前端校验、服务端须再校验）；附件在线预览；排序（时间/进度）；删除确认弹窗；复刻计数。 |

设置弹窗内容：主题深/浅（默认深色）、字号（13/15/17/19px）、字体、关注列表管理
（分组增删、作者增删改、作者归属分组）、用户标签词汇与按作者覆盖、Webhook 设置与触发。

Webhook 行为（按 SPEC，仅存 localStorage，键 `x-digest-webhook-url` / `x-digest-webhook-key`）：
触发时 POST `{}` 到该 URL，`Authorization: Bearer <key>`（key 已带 Bearer 时不重复加）；
失败依次 cors → no-cors → sendBeacon 兜底。无服务端 webhook 密钥。

localStorage 键（与原站一致）：`x-digest-watchlist`、`x-digest-post-categories`、
`x-digest-selected-authors`、`x-digest-selected-cats`、`x-digest-theme`、`x-digest-font-family`、
`x-digest-font-size`、`x-digest-user-tags`、`x-digest-webhook-url`、`x-digest-webhook-key`、
`x-digest-chips-*-expanded`、`x-digest-filters-collapsed`。

附件白名单（与原站 `fuke.js` 一致）：
`pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif webp bmp avif svg ico`。

头像：API 字段如 `avatars/ai_xiaomu.jpg`，前端按 `/avatars/ai_xiaomu.jpg` 解析。
`public/avatars/` 现为从原站拷来的 13 张占位图（联调占位，不计入交付问题）；
集成时协调人把正式头像拷到 `public/avatars` 即可。

## 与原站的已知差异（据实列出）

1. **备注编辑器不用 EasyMDE**：自研 textarea + 工具栏（粗体/斜体/标题/列表/链接/代码/引用）
   + marked 预览，600ms 去抖自动保存（PATCH `/api/v1/replicas/{id}`）。无 CodeMirror、无快捷键。
2. **PDF 预览用浏览器原生 embed/iframe**，不用原站 pdf.js 画布分页+缩放。embed 自带浏览器
   阅读器的分页/缩放，功能等价但渲染实现不同。
3. **PPTX 预览**：JSZip 解 `ppt/slides/slide*.xml` 提取 `<a:t>` 文本并翻页；图片为尽力而为
   （当前实现把 `ppt/media/*` 全部图片附在每页，未按 slide 关系精确映射到页）。
4. **旧版 doc/ppt**：二进制字符串提取（可打印 ASCII 连续串），仅尽力提文本并提示下载核对；
   rar/7z/tar/gz、xls/xlsx 仅下载（原站亦无浏览器内渲染；SPEC 未要求 xls 在线渲染）。
5. **首屏数据在客户端加载**：页面初始数据在 `onMounted` 里经 `useApi` 拉取，SSR 返回的是
   外壳（已验证 200）。`useApi` 已实现双 base（浏览器相对路径 / SSR `NUXT_SERVER_API_BASE`），
   但 SSR 直出帖子列表尚未接通——待后端联调时可把初始加载改为 `useAsyncData` 走 SSR，
   届时 SSR base 即生效。本项列为未完成项，不谎称已做。
6. **筛选在前端执行**：前端拉取 `GET /api/v1/posts?page_size=1000` 全量后，按 SPEC 语义在
   浏览器内做 AND/OR 过滤（作者组内 OR、分类组内 OR、空作者选中=空列表）。这样 mock 与 API
   两种模式行为完全一致；服务端过滤参数（author/tag/category/user_tag/q/date）目前未用作
   粗筛，后端实现后可再接。
7. **筛选计数口径简化**：分类 chips 计数按“通过作者外其他筛选”的帖子计（与原站 ignoreCats
   口径一致）；作者/标签 chips 计数为全量计数，未做原站那种“忽略本组”的交叉计数。
8. **原站的撤销删除（undo toast）、更新冷却、筛选区折叠、chips 展开记忆等次要交互未复刻**，
   只保留了 `x-digest-chips-*-expanded` / `x-digest-filters-collapsed` 键名占位，未读写。
9. **原站帖子卡片的“分类改名”输入框未复刻**：设置弹窗可增删类别，卡片可勾选/新建类别，
   但类别改名需走后端 DELETE+POST（前端未提供改名输入框）。

## 目录结构

```
nuxt.config.ts            # 双 base、端口、CSS（原站 styles.css 原样复用为 original.css + app.css 增补）
app.vue                   # 壳 + 主题应用
pages/index.vue           # / 摘要页（含设置弹窗、Webhook）
pages/fuke.vue            # /fuke 复刻页
components/NoteEditor.vue       # Markdown 备注编辑+预览+工具栏
components/AttachmentPreview.vue# 附件在线预览（图片/pdf/md/txt/docx/pptx/zip/旧版/仅下载）
composables/types.ts      # SPEC API 形状（不自创字段）
composables/useApi.ts     # 统一 API 封装：浏览器 /api/v1，SSR NUXT_SERVER_API_BASE
composables/useStore.ts   # 数据层：API 与 mock 的统一出口
composables/useMock.ts    # mock 数据层（默认关闭，NUXT_PUBLIC_USE_MOCK=1 开启）
composables/useFormat.ts  # 头像/时间/高亮/首命中片段/流程图启发式（原站 app.js 移植）
composables/useSettings.ts# 主题/字号/字体/Webhook（localStorage，键名与原站一致）
assets/css/original.css   # 原站 styles.css 原样复用（视觉贴近原站）
assets/css/app.css        # 增补样式 + ≤620px 不横向溢出
assets/mock/              # mock 数据（原站 posts/watchlist/user-tags 归一化后）
public/avatars/           # 联调占位头像 13 张（集成时由协调人替换/补齐）
```

## 未完成项（据实）

- SSR 直出帖子列表（见差异 5）；当前 SSR 返回外壳、数据客户端加载，页面功能不受影响。
- 与真实 Go 后端联调未做（开发时后端不存在）：`POST /api/v1/replicas` 返回已有对象时的
  `existing:true` 与“直接返回已有对象”两种形态前端都已兼容，但未对真实后端验证过；
  附件上传的 422 逐文件说明依赖后端返回 `message`，前端原样展示。
- 类别改名输入框、撤销删除、筛选区折叠等次要交互未复刻（见差异 8、9）。
- 未做任何 git 操作、未访问上海服务器、未改 `~/workspace/x-digest` 与其他项目（按任务要求）。
