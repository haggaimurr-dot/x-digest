# x-digest · AI/技术博主内容日报

个人用的信息流整理站：把关注的 X 博主、GitHub 仓库、YouTube / B 站频道的内容集中起来，
整理成可检索、可打标签、可按步骤「复刻」的中文日报。AI 摘要只在用户点击时生成，采集流程不自动生成。

技术栈：**Nuxt 4（Vue 3）前端 + Go 后端 + MariaDB**。原纯静态站已于 2026-10-24 迁移，旧代码保留在 `legacy-static/`。

## 功能一览

顶栏五个版块（中文 / EN 切换；「Aa 外观」全站设置，深色默认、可切浅色；移动端可用）：

| 页面 | 功能 |
|---|---|
| `/` X 日报 | 推文按作者分组展示：左栏原文（超过 120 字默认折叠）、右栏标题 / 标签 / 内容简介、提及的资源、查看原帖；按作者、标签、分类、用户标签、关键词、日期筛选并分页；点击生成 AI 摘要；「导入单帖」「更新帖子」；关注列表（分组、折叠、搜索）与标签管理 |
| `/fuke` 复刻 | 把教程类帖子的实现步骤建成复刻清单：每步 idle / active / done 三态、Markdown 备注、附件上传与浏览器内预览 |
| `/repos` 仓库 | 追踪 GitHub 仓库：描述、语言、星标、Fork、开放 Issue / PR、最近 10 条提交、最新 Release；分组、标签、备注；单仓刷新和全部同步 |
| `/youtube` 油管 | 按频道同步视频 / 播放列表；可添加单个视频（同时登记所属频道）；按账号分组的视频卡片、自动标签、提及的资源；点击生成 AI 摘要（章节 + 全文） |
| `/bilibili` B站 | 与油管同一套页面和接口 |

## 目录结构

```
frontend/         Nuxt 前端（pages / components / composables）
backend/          Go 后端
  cmd/api/          HTTP API 服务（xdigest-api）
  cmd/import/       导入 CLI：data/ + config/ → MariaDB（幂等）
  cmd/digest/       批量 AI 解析：raw_posts.json → posts.json
  internal/         store / importer / xpost / youtube / bilibili / github / translate / ytsummary
  schema.sql        建表语句（幂等，可重复执行）
data/             raw_posts.json（原文暂存）、posts.json（解析结果）、avatars.json、采集队列进度
config/           watchlist.json（关注列表）、user-tags.json（用户标签词表）
avatars/          作者头像（前端 public/avatars 有同一份副本）
scripts/          append_raw_posts.py：按 id 去重追加原文
tests/            test_raw_posts_dedup.py
deploy/           部署脚本、systemd 单元、nginx vhost
legacy-static/    迁移前的静态站（只作参考 / 回滚）
```

## 本地开发

```bash
# 后端（需 MariaDB；先执行 backend/schema.sql）
cd backend && go run ./cmd/api              # 默认监听 127.0.0.1:18090
# 前端
cd frontend && npm install && npm run dev   # http://127.0.0.1:18091，/api 代理到 Go
```

- 前端 `predev` / `prebuild` 会运行 `scripts/sync-x-content.mjs`，把 `data/raw_posts.json` 的原文和头像合并成 `frontend/assets/mock/x-content.json`（帖子原文不存在 posts 表里，由页面合并）。
- `NUXT_PUBLIC_USE_MOCK=1` 时使用 `frontend/assets/mock/` 下的数据，不调用 API。
- `NUXT_SERVER_API_BASE` 指定 SSR / 开发代理使用的 API 地址（默认 `http://127.0.0.1:18090/api/v1`）。

## 环境变量（后端）

服务器上统一放在 `/etc/xdigest/api.env`（root-only，600），模板见 `backend/api.env.example`。真实密钥和密码不得提交到仓库。

| 变量 | 说明 |
|---|---|
| `XDIGEST_ADDR` | 监听地址，代码默认 `127.0.0.1:18090`，生产配为 `127.0.0.1:18093` |
| `XDIGEST_DSN` | MariaDB DSN |
| `XDIGEST_ATTACHMENT_DIR` | 附件目录，默认 `/var/lib/xdigest/attachments` |
| `TRANSLATE_API_BASE` / `TRANSLATE_API_KEY` / `TRANSLATE_MODEL` | OpenAI 兼容的 AI 中转站；翻译、各类 AI 摘要、帖子解析共用。未配 key 时相关接口返回 503 |
| `DIGEST_MODEL` | 帖子解析用的模型（未设则用 `TRANSLATE_MODEL`） |
| `YT_SUMMARY_MODEL` | 视频摘要用的模型（可选） |
| `GROK_API_KEY` | 「更新帖子」必填，用 Grok `x_search` 找新帖 id |
| `GROK_API_BASE` / `GROK_MODEL` / `GROK_DIGEST_MODEL` | Grok 地址（默认同 `TRANSLATE_API_BASE`）、找帖模型（默认 grok-4.3）、兜底解析模型（默认 grok-4.20-0309-non-reasoning） |
| `X_REFRESH_DAYS` | 账号没有已入库帖子时向前找的天数，默认 30 |
| `XPOST_API_BASE` / `X_HTTP_PROXY` | fxtwitter 接口地址 / 代理 |
| `GITHUB_TOKEN` | GitHub 细粒度只读 PAT（可选；未配时每小时 60 次） |
| `YT_DLP_BIN` / `YT_HTTP_PROXY` | yt-dlp 路径 / 代理 |
| `BILI_API_BASE` / `BILI_HTTP_PROXY` / `BILIBILI_SESSDATA` | B 站接口地址 / 代理 / 登录 Cookie（可选） |

## X 日报数据流

```
config/watchlist.json
  → 采集 X 原文与头像 → data/raw_posts.json（按 id 去重追加）
  → backend/cmd/digest（AI 解析标题、行业、技术、标签、简介，教程类加步骤）→ data/posts.json
  → backend/cmd/import（幂等 upsert）→ MariaDB → Go API → Nuxt 页面
```

- 原文采集由外部 Grok Bot routine 完成（folder id `x`，由统一调度维护，改规则时更新该 routine，不在本仓库另建一份）。`data/following_queue.txt` / `queue_progress.json` / `fetched_accounts.txt` 记录批量采集进度。
- `posts.json` 的增量由 bot 自动提交（提交信息「posts.json：解析 bot 新拉取的帖子」）。
- 网页上的「导入单帖」和「更新帖子」**直接写数据库**，不写 `raw_posts.json` / `posts.json`；之后再跑 `cmd/import` 不会删除这些帖子。

### 导入单帖 / 更新帖子

- **导入单帖** `POST /api/v1/posts/import {url, force}`：用 fxtwitter 拉原文（长文、X 文章、引用、回复上下文、图片、头像）→ AI 解析（与 `cmd/digest` 同一提示词）→ 写库；作者不在关注列表时自动加入「默认」分组。
- **更新帖子** `POST/GET /api/v1/x/refresh`：选账号（不选 = 全部），后台任务用 Grok `x_search` 找每个账号最新已入库帖之后的新帖 id，再逐条拉原文、解析、入库；15 字以内的短回复跳过。Grok 只提供 id，正文和作者以 fxtwitter 为准，模型编造的 id 不会入库。页面轮询进度。
- 主模型拒绝（中转站对个别话题返回 403）时改用 Grok 重试。

### 批量解析与导入（服务器本地 CLI，不暴露成 HTTP 接口）

```bash
set -a; . /etc/xdigest/api.env; set +a
go run ./cmd/digest -data-dir /opt/xdigest -dry-run     # 只统计
go run ./cmd/digest -data-dir /opt/xdigest              # 生成，已有的跳过、可断点续跑；-force 重新生成
/opt/xdigest/backend/xdigest-import -data-dir /opt/xdigest -dsn 'xdigest:PASSWORD@tcp(127.0.0.1:3306)/xdigest'
```

导入幂等：帖子按主键 upsert，steps / tags 每次重建；新帖子摘要为空，已有的用户摘要保留；同时补齐 `posts.author_avatar` 和 `authors.avatar`。

## AI 摘要

X 帖子、油管、B 站共用同一套逻辑：首次点击时后台生成并入库，之后直接读库；「重新生成」会覆盖。接口 POST 返回 202，页面每 4 秒轮询。

- **X 帖子** `GET/POST /api/v1/posts/{id}/summary?lang=zh-CN|en[&force=1]`，表 `post_summaries`。输出全文摘要、要点、提及的资源。原文由页面在 POST 体里带上 `{text, links}`；没有原文时只用标题、旧摘要和步骤生成，并在页面注明。
- **视频** `GET/POST /api/v1/<yt|bili>/videos/{id}/summary?lang=…[&force=1]`，表 `yt_video_summaries` / `bili_video_summaries`。yt-dlp 读元数据，只下载一条字幕（优先人工字幕，其次原语言自动字幕），生成全文摘要、要点和带时间点的章节；视频自带章节时保留作者的时间点；没有字幕时只根据标题和简介生成。播放列表不提供摘要。

## 内容规则（硬规则）

- **原文完整**：title / summary / steps 不得用 `…` / `...` 截断；短链 API 不够时用 `note_tweet` 或 fxtwitter 取全文。长文完整入库，由 UI 折叠。
- **中文输出**：面向用户的 title / summary / steps 必须是中文，英文原文完整理解后再译写。
- **流程图 / 实现步骤**：只给技术 / 产品 / 工具介绍或教程生成 `steps` 并设 `showFlowchart: true`，须同时满足：① 正文点名具体产品 / 工具 / 技术；② 含使用步骤、原理拆解或搭建 / 配置流程。拿不准就不生成。观点、新闻、闲聊、吐槽、涨粉复盘、合集、盈利盘点等一律不生成，此时 `steps` 写 `[]`。
- **复刻入口**：只对原文是教程 / 技术分享的帖子显示，不能因为模型生成了 steps / summary 或遗留了 `showFlowchart=true` 就显示。已有复刻仍可在复刻页查看。
- **原文去重**：向 `data/raw_posts.json` 追加前必须先读出已有全部 `id`，已存在的跳过。请用 `scripts/append_raw_posts.py`；验证：`python3 tests/test_raw_posts_dedup.py`。

### raw_posts.json 字段

单个 JSON 数组，作者信息随每条记录保存：

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
    { "type": "quoted", "id": "referenced-tweet-id", "author_id": "...", "author_username": "...", "author_name": "...", "text": "被引用推文正文" }
  ],
  "entities": { "urls": [{ "url": "https://t.co/...", "expanded_url": "https://..." }] },
  "media": [{ "type": "photo", "url": "https://example.com/image.jpg" }],
  "public_metrics": { "like": 0, "reply": 0, "repost": 0, "quote": 0, "impression": 0 },
  "fetched_at": "2026-10-27T12:00:00Z",
  "source": "builtin-x"
}
```

- `author_id`、`author_username`、`author_name` 必填且不得为空。
- `note_tweet` / `long_text` 有值时保存完整长文。
- `entities.urls[].expanded_url` 用于还原 t.co 短链；引用推文的展开链接不算资源（其正文已附在原文后）。
- `source` 为 `builtin-x` 或 `plugin`。
- 头像读 raw / API 的 avatar / profile_image 字段，依次回退：库值 → 关注列表 → `/avatars/<用户名>.jpg` → 首字母。

## 接口（摘要）

全部为 JSON，前缀 `/api/v1/`，错误格式 `{"error":"CODE","message":"..."}`；另有 `/healthz`。详细说明见 `backend/README.md`。

- **帖子** `GET /posts?author=&tag=&category=&user_tag=&q=&date_from=&date_to=&page=&page_size=`（page_size 默认 200、上限 1000；`author=` 为空时返回空列表；`q=标签:xxx` / `tag:xxx` 只匹配用户标签）、`GET /posts/{id}`、`/posts/{id}/summary`、`POST /posts/import`、`/x/refresh`
- **关注列表与标签** `/watchlist`（分组 + 账号 CRUD）、`GET /tags`、`GET/PUT /user-tags`、`/categories`（含 `PUT /categories/assignments`）
- **复刻** `/replicas`（同一帖子不重复建）、`/replicas/{id}/attachments`、`/attachments/{id}[/download]`
- **仓库** `/repos`、`/repos/{owner}/{repo}[/commits|/releases|/sync]`、`POST /repos/sync-all`、`/repos/groups`
- **视频** `/yt/*` 与 `/bili/*` 同构：`channels`、`videos`、`videos/{id}/summary`、`sync`、`groups`、`comments`（评论接口仍在，页面已不调用）
- **翻译** `POST /translate`（结果缓存在 `translations` 表）

附件白名单：pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif webp bmp avif svg ico；单个 ≤ 5MB，先全部校验再落盘，不合规返回 422。磁盘文件名用内部 id，数据库只存元数据和路径。

### GitHub 同步说明

后端直接调 GitHub REST API；Issue / PR 数用 search API 分别统计（不用含 PR 的 `open_issues_count`）。数据存库，页面只读库；只在添加仓库和点「刷新」时同步，记录 `last_synced_at` / `sync_status`。被限流时保留旧数据并提示；未配 token 时，私有仓库会报「可能是不存在或私有仓库需要 token」。

## 部署与回滚（上海服务器，公网 http://42.192.114.201:18089/）

| 组件 | 监听 | 说明 |
|---|---|---|
| nginx | `:18089` | `deploy/xdigest-18089.nginx`：`/` → Nuxt，`/api/` → Go |
| `xdigest-web`（Nuxt） | `127.0.0.1:18091` | `deploy/xdigest-web.service`（Nitro 认 `PORT`，`NUXT_PORT` 不生效） |
| `xdigest-api`（Go） | `127.0.0.1:18093` | `backend/xdigest-api.service`，`EnvironmentFile=/etc/xdigest/api.env`（18090 已被 chiji-dev 占用） |

部署步骤：
1. 执行 `backend/schema.sql` 建库建表，按 `backend/db-user.sql` 建专用用户，密码只写进 `/etc/xdigest/api.env`。
2. 把二进制和 Nuxt `.output` 拷到 `/opt/xdigest`，执行 `sudo bash deploy/deploy-xdigest.sh` 安装并启动服务（脚本不改 nginx）。
3. 切换 nginx vhost 是单独的一步。

回滚：停掉 `xdigest-api` / `xdigest-web`，把 `/var/www/x-digest.bak-YYYYMMDD` 拷回 `/var/www/x-digest`，把 `/etc/nginx/sites-available/x-digest-18089.static.bak` 拷回 vhost，然后 `nginx -t && systemctl reload nginx`。`xdigest` 库不影响旧站。

## 安全说明（必须读）

本项目**没有登录和鉴权**：任何能访问公网 18089 的人都能读写关注列表、复刻和附件，也能触发 AI 调用（会产生中转站费用；已生成的摘要不会重复调用）。上线前应补一层鉴权，二选一：

1. nginx 层 basic auth 或 IP 白名单；
2. Go 层加 Bearer token 中间件（token 放 `api.env` 的 `XDIGEST_API_TOKEN`），前端在请求头里带上。

数据导入只通过服务器本地 CLI 完成；如要改成 HTTP 导入接口，必须先加鉴权。

## 已知差异（相对原静态站）

- 备注编辑器是自研 textarea + 工具栏 + marked 预览（未用 EasyMDE）；PDF 预览用浏览器原生 embed；PPTX 图片未按 slide 关系精确映射；xls 只能下载。
- 首屏是 Nuxt SSR 外壳，帖子由客户端拉取；筛选在前端执行（拉取 page_size=1000 后按原语义过滤），服务端过滤参数已实现但未使用。
- 旧站的分类归属和用户标签覆盖只存在浏览器 localStorage，导入后为空，需要在新版里重新设置。
- 标签精确匹配与 `/tags` 去重用 `utf8mb4_bin`（区分大小写，如 `workbuddy` / `Workbuddy`）；关键词 `q` 的搜索不区分大小写。
