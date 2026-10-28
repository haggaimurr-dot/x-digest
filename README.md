# X 博主推文日报（新版：Nuxt + Go + MariaDB）

原纯静态站（`legacy-static/`，index.html + app.js + fuke.html）已于 2026-10-24 全量迁移为：

- **前端** `frontend/`：Nuxt 4（Vue 3），页面 `/` 摘要页、`/fuke` 复刻页，设置做成弹窗。
  视觉贴近原站 styles.css（`frontend/assets/css/original.css`），深色默认、浅色可切换，移动端可用。
- **后端** `backend/`：Go（标准库 net/http + database/sql + go-sql-driver/mysql），
  接口全部在 `/api/v1/` 下，另有 `/healthz`。无登录（个人工具，见下文安全说明）。
- **数据库**：MariaDB 新库 `xdigest`（与 chiji / bailiankai / bailiankai_web 隔离），
  表结构见 `backend/schema.sql`。附件只存元数据 + 磁盘路径（默认 `/var/lib/xdigest/attachments`），不存 BLOB。
- **旧数据**：根目录 `data/`、`config/`、`avatars/` 是迁移源（190 条推文、13 作者、75 带步骤），
  由 `backend/cmd/import`（服务器本地 CLI）导入；`frontend/assets/mock/` 只是开发 fallback（默认关闭）。

## 本地开发

```bash
# 后端（需 MariaDB，DSN 按需改）
cd backend && go run ./cmd/api
# 前端（开发用 mock 或指向本地 API）
cd frontend && npm install && npm run dev   # dev 端口 18091，/api 代理到 Go（见 nuxt.config.ts）
```

生产启动（服务器，见 `deploy/`）：Go 监听 `127.0.0.1:18093`（18090 已被同期上线的 chiji-dev 占用，部署时核实后改定），Nuxt 监听 `127.0.0.1:18091`
（注意：Nitro 认 `PORT`，`NUXT_PORT` 不生效），nginx 18089 代理 `/` → Nuxt（18091）、`/api/` → Go（18093）。

## 接口（摘要）

- `GET /api/v1/posts?author=&tag=&category=&user_tag=&q=&date_from=&date_to=&page=&page_size=`
  （page_size 默认 200、上限 1000；`author=` 空值 = 空列表，与原站空选中语义一致；
  `q=标签:xxx` / `tag:xxx` 只匹配用户标签）
- `GET /api/v1/posts/{id}`、`GET /api/v1/watchlist`（分组+账号 CRUD）、`GET /api/v1/tags`、
  `GET/PUT /api/v1/user-tags`、`GET/POST/DELETE /api/v1/categories`（+归属）、
  `GET/POST/PATCH/DELETE /api/v1/replicas`（步骤状态 idle/active/done 三态循环，同一帖子不重复建）、
  `POST/GET/DELETE /api/v1/replicas/{id}/attachments`、`/api/v1/attachments/{id}/download`。
- 附件白名单：pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif webp bmp avif svg ico
  zip rar 7z tar gz tgz；单个 ≤10MB（服务端校验，超大/非法扩展名返回 422）。

## 仓库版块（GitHub，与 X 日报分开展示、顶栏切换）

- 页面 `/repos`：仓库卡片（描述、语言、星标、Fork、开放 Issue/PR 数、最后更新、链接）+ 最近提交（10 条）+ 最新 Release；添加仓库（`owner/repo`）、分组、标签、备注、删除；单仓刷新与全部同步。X 推文和仓库绝不混在同一个列表里。
- 接口：`GET/POST /api/v1/repos`、`GET/PATCH/DELETE /api/v1/repos/{owner}/{repo}`、`GET .../commits`、`GET .../releases`、`POST .../sync`、`POST /api/v1/repos/sync-all`、`GET/POST/DELETE /api/v1/repos/groups`。新表：`repo_groups`、`repos`、`repo_tags`、`repo_commits`、`repo_releases`（见 `backend/schema.sql`，幂等追加）。
- 同步：后端直接调 GitHub REST API（仓库信息、提交、Release、search API 分别计 Issue/PR 数，不用含 PR 的 `open_issues_count`）。数据存 DB，页面只读 DB；同步只在添加仓库和点“刷新”时触发，记 `last_synced_at`/`sync_status`。未登录每小时 60 次，限流时保留旧数据并明确提示；私有仓库未登录会报“可能是不存在或私有仓库需要 token”。
- Token：后端读环境变量 `GITHUB_TOKEN`（有则用，无则走未登录模式）。多仓库频繁刷新需要一个 GitHub 细粒度 PAT（只读公开仓库即可），通过 Secure Vault 提供后配进服务器 `/etc/xdigest/api.env`，绝不写进仓库。

## 油管 AI 摘要（章节 + 全文）

- 油管页每个视频（播放列表除外）有「✨ AI 摘要」按钮：首次点击在后台生成，之后直接读库；「重新生成」会覆盖旧结果。
- 流程：yt-dlp 读元数据（标题、简介、作者章节、可用字幕）→ 只下载一条字幕（优先人工字幕，其次视频原语言的自动字幕）→ 调 `TRANSLATE_*` 配置的同一个 AI 中转生成全文摘要、要点和带时间点的章节。视频自带章节时保留作者的时间点；没有字幕时只根据标题和简介生成，并在页面上注明。
- 接口：`GET/POST /api/v1/yt/videos/{id}/summary?lang=zh-CN|en[&force=1]`（POST 返回 202 后页面每 4 秒轮询）；表 `yt_video_summaries`（见 `backend/schema.sql`，幂等追加）。
- 依赖：服务器上 `TRANSLATE_API_KEY` 必须已配置，否则返回 503；yt-dlp 走 `YT_HTTP_PROXY`，与同步相同。无登录，任何能访问页面的人都能触发 AI 调用（已生成的摘要不会重复调用）。

## X 日报 AI 摘要（与油管同一逻辑）

- 卡片下方「✨ AI 摘要」：首次点击时后台调 `TRANSLATE_*` 中转站生成（全文摘要、要点、提及的资源），存入表 `post_summaries`，之后直接读库；「重新生成」覆盖。
- 接口 `GET/POST /api/v1/posts/{id}/summary?lang=zh-CN|en[&force=1]`；原文不在 posts 表里（页面从 `x-content.json` 合并），所以 POST 体里由页面带上 `{text, links}`。没有原文的帖子只用标题、旧摘要和步骤生成，并在页面注明。

## X 日报：导入单帖 / 更新帖子

- 「导入单帖」：贴 x.com 帖子链接 → 用 fxtwitter 公开接口拉原文（长文、X 文章、引用、回复上下文、图片、头像）→ 中转站 AI 解析（与 `cmd/digest` 同一提示词）→ 直接写库；作者不在关注列表时自动加入「默认」分组。接口 `POST /api/v1/posts/import {url, force}`。
- 「更新帖子」：选账号（不选 = 全部），后台任务用中转站 Grok 的 `x_search` 找每个账号已入库最新帖之后的新帖 id（没有帖子的账号找最近 `X_REFRESH_DAYS`=30 天），再逐条拉原文、AI 解析、入库；15 字以内短回复跳过。Grok 只提供 id，帖子内容和作者都以 fxtwitter 为准，编造的 id 不会入库。接口 `POST/GET /api/v1/x/refresh`，页面轮询进度。
- AI 解析默认用 `DIGEST_MODEL`（未设则 `TRANSLATE_MODEL`）；主模型拒绝（中转站对个别话题返回 403）时改用 Grok 重试。
- 服务器环境变量（`/etc/xdigest/api.env`）：`GROK_API_KEY`（中转站 Grok 分组的 key，必填才能「更新帖子」），可选 `GROK_MODEL`（找帖，默认 grok-4.3）、`GROK_DIGEST_MODEL`（兜底解析，默认 grok-4.20-0309-non-reasoning）、`X_HTTP_PROXY`（服务器直连不了 fxtwitter 时的代理）。
- 这两条路径直接写数据库，不写 `data/raw_posts.json` / `posts.json`；之后再跑 `cmd/import` 不会删除它们。

## 视频卡片（油管 / B站共用）

- 视频按账号分组显示，每个账号一块，可折叠；左栏缩略图 + 简介，右栏标题、时间、标签，下方整行是「提及的资源」和 AI 摘要。
- 标签：按标题 + 简介关键词自动识别（Claude Code、Codex、Gemini、ChatGPT、Cursor、MCP、n8n 等，规则在 `frontend/utils/videoTags.ts`），检索条件里可按标签筛选。
- 提及的资源：简介里的链接（去掉社交账号）+ AI 摘要识别出的工具 / 仓库 / 网站（摘要字段 `resources`）。
- 热门评论已从页面移除（`/api/v1/<yt|bili>/comments` 接口仍在，页面不再调用）。

## 数据导入（服务器本地，不要暴露成 HTTP 接口）

```bash
# 先建库建用户：backend/schema.sql、backend/db-user.sql（密码只进 /etc/xdigest/api.env）
/opt/xdigest/backend/xdigest-import -data-dir /opt/xdigest/data -dsn 'xdigest:PASSWORD@tcp(127.0.0.1:3306)/xdigest'
# 期望计数：posts 190、post_steps 323、post_tags 843、作者 13、带步骤帖子 75；重复跑幂等不翻倍。
```

每日更新：原站的“立即更新”按钮仍调用户自己配置的外部更新器（Webhook URL/key 只存浏览器
localStorage，不进服务端）。外部更新器产出新 posts.json 后，在服务器上重跑一次导入 CLI 即可；
如需改成直连 Go 的导入接口，必须先加鉴权再做，不要直接暴露无鉴权导入。

## 部署与回滚（上海服务器，公网 http://42.192.114.201:18089/）

- 服务：`xdigest-api`（Go，18093）、`xdigest-web`（Nuxt，18091），均为内网监听 + systemd；
  nginx vhost `deploy/xdigest-18089.nginx`。
- 旧站备份：切换前 `/var/www/x-digest` 备份为 `/var/www/x-digest.bak-YYYYMMDD`，
  旧 vhost 备份为 `/etc/nginx/sites-available/x-digest-18089.static.bak`。
- 回滚：停掉 xdigest-api / xdigest-web，把备份目录拷回 `/var/www/x-digest`，
  把 static.bak 拷回 vhost 并 `nginx -t && systemctl reload nginx`，即回到旧静态站；
  新库 `xdigest` 不影响旧站（旧站只读静态文件）。

## 安全说明（必须读）

本项目无登录：任何能访问公网 18089 的人都能读写复刻数据（帖子本身是公开摘要，风险主要在
复刻/附件被他人改动或上传垃圾文件）。缓解办法（后续二选一，不要直接上生产就当没这回事）：
nginx 层加 basic auth 或 IP 白名单，或给 Go API 加 Bearer token 并让前端在请求头里带上。
附件目录与数据库密码只放服务器 root-only 文件（`/etc/xdigest/api.env`，600），绝不进仓库。

## 已知差异（相对原静态站，如实记录）

- 备注编辑器为自研 textarea + 工具栏 + marked 预览（未用 EasyMDE）；PDF 预览用浏览器原生
  embed（未用 pdf.js 画布）；PPTX 图片未按 slide 关系精确映射；xls 仅下载。
- 首屏为 Nuxt SSR 外壳 + 客户端拉取帖子（SSR 直出帖子列表未接通）；筛选目前在前端执行
  （拉 page_size=1000 后按原语义过滤），服务端过滤参数已实现但未作粗筛。
- 分类归属与用户标签覆盖在旧文件里不存在（只存在旧浏览器 localStorage），导入后为空，
  需在新版里重新归属；撤销删除、筛选区折叠、类别改名等次要交互未复刻。
- 标签大小写：为与原站 JS 精确匹配一致，精确匹配与 `/tags` 去重用 `utf8mb4_bin`，
  故 `/tags` 返回 408 个（含 `workbuddy` / `Workbuddy` 两个变体）。

## 原文暂存数据

### raw_posts.json 字段

`data/raw_posts.json` 是单个 JSON 文件，当前为空数组 `[]`。每条原文记录都应是完整对象，作者信息随记录保存，不另建作者表。字段约定如下：

```json
{
  "id": "tweet-id",
  "url": "https://x.com/handle/status/tweet-id",
  "author_id": "x-user-id",
  "author_username": "handle",
  "author_name": "显示名",
  "created_at": "2026-10-18T12:00:00Z",
  "lang": "zh",
  "text": "完整原文，不截断",
  "note_tweet": null,
  "long_text": null,
  "conversation_id": "conversation-id",
  "in_reply_to_id": null,
  "referenced_tweets": [
    {
      "type": "retweeted",
      "id": "referenced-tweet-id",
      "author_id": "original-author-id",
      "author_username": "original_handle",
      "author_name": "原作者显示名"
    }
  ],
  "media": [
    { "type": "photo", "url": "https://example.com/image.jpg" }
  ],
  "public_metrics": {
    "like": 0,
    "reply": 0,
    "repost": 0,
    "quote": 0,
    "impression": 0
  },
  "fetched_at": "2026-10-27T12:00:00Z",
  "source": "builtin-x"
}
```

`author_id`、`author_username`、`author_name` 必填且不得为空。

**写入去重（硬规则）**：向 `data/raw_posts.json` 追加前必须先读出已有全部 `id`；新帖 `id` 已存在则跳过，不得再 append。保证零重复。入库请用 `scripts/append_raw_posts.py`（或同等逻辑）。验证：`python3 tests/test_raw_posts_dedup.py`。
`note_tweet` 或 `long_text` 有值时保存完整长文；无长文时可为 `null`。`referenced_tweets` 保存引用或转帖的原作者信息，至少包含 `author_id` 和 `author_username`，有名称时也保存 `author_name`。`media` 中每项包含 `type`、`url`；`public_metrics` 使用 `like`、`reply`、`repost`、`quote`、`impression` 五项。`source` 为 `builtin-x` 或 `plugin`。


## 原文采集与摘要流程

默认流程：**X 拉取完整原文及作者头像 → 写入 `data/raw_posts.json` → 导入原文/元数据**。禁止采集、解析或 import 自动生成/写入 AI 摘要；模型生成仅由用户手动触发。X 卡片不默认渲染已有 summary。import 新帖子 summary 为空，更新帖子保留用户已有摘要。

头像读取 raw/API 的 avatar / profile_image 字段，并回退到作者映射与关注列表；import 补齐 posts.author_avatar 和 authors.avatar，API 也会回退到 authors.avatar。前端构建时同步原文和头像映射，运行时可回退关注列表头像。

「复刻」只对原文中的教程/技术分享显示，不能因为模型生成了 steps、summary 或遗留 showFlowchart=true 就显示。普通赞美、感谢、效果感叹帖不显示复刻；已有复刻仍可在复刻页查看。

本次上线：重新构建并发布 frontend；重新构建/重启 API 以启用作者头像回退。建议使用新 importer 重跑一次以补齐历史头像（无需改表），不会导入历史自动摘要或覆盖已存摘要。

当前先准备好 `config/watchlist.json` 关注列表；关注列表到位后，再按最近半年窗口从 X 拉取原文并写入 `data/raw_posts.json`。**现阶段先只采集和保存原文，不做分析，也不生成或更新摘要。**

### 正文完整性与中文（硬规则）

- **禁止用 `…` / `...` 截断** title / summary / steps 正文。短链 API 不够时，改用完整 `note_tweet`，或 `https://api.fxtwitter.com/{user}/status/{id}` 取全文后再写库。
- **面向用户的 title / summary / steps 必须是中文**：英文原文先完整理解再译写，不要把英文半截句子留在 steps 里。
- **长文可以很长**：完整写入 JSON；站点 UI 用「显示更多 / 收起」折叠，不要在数据层截断。

### 流程图 / 实现步骤（硬规则）

仅对 **技术 / 产品 / 工具介绍或教程** 生成 `steps` 并设 `showFlowchart: true`。观点、新闻、闲聊、吐槽、纯涨粉复盘、合集、盈利模式盘点等 **不要** 生成流程图。

须 **同时** 满足（拿不准则不生成）：

1. 正文点名具体产品 / 工具 / 技术；
2. 含使用步骤、原理拆解或搭建/配置流程。

字段：

- `showFlowchart`（boolean）：前端是否展示「实现步骤·流程图」折叠。旧数据若缺省，前端会用同一启发式推导。
- `steps`：仅在 `showFlowchart: true` 时填写；否则写 `[]`。
- `isLong`：仍可表示长文；**不再单独决定是否显示流程图**。

无需后端。Grok Bot 日常调度 routine 的 folder id 为 `x`（由统一调度维护；改规则时用 UpdateRoutine 更新该 routine，不要在本仓库另建一份）。

## GitHub Pages

本仓库根目录即为站点根（`index.html`）。Pages 源：`main` 分支 `/`（root）。
