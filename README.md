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

生产启动（服务器，见 `deploy/`）：Go 监听 `127.0.0.1:18090`，Nuxt 监听 `127.0.0.1:18091`
（注意：Nitro 认 `PORT`，`NUXT_PORT` 不生效），nginx 18089 代理 `/` → Nuxt、`/api/` → Go。

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

- 服务：`xdigest-api`（Go，18090）、`xdigest-web`（Nuxt，18091），均为内网监听 + systemd；
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
