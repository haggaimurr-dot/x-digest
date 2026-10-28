# x-digest 后端（Go + MariaDB）

实现 `~/workspace/xdigest-new/SPEC.md` 的全部服务端契约。只含 Go 后端、
MariaDB schema 与本地导入 CLI；不含 Nuxt 前端，不改前端目录，不推 git。

## 结构

| 文件 | 说明 |
|---|---|
| `cmd/api/main.go` | HTTP API 入口与路由（标准库 net/http）；各领域 handler 按文件拆分：`posts.go`、`watchlist.go`、`categories.go`、`replicas.go`、`attachments.go`、`repos.go`（+`repos_sync.go`）、`yt.go`、`hub.go`、`translate.go`、`x_import.go`、`auth.go` |
| `cmd/maint/main.go` | 运维 CLI `xdigest-maint`（清理翻译缓存、清理软删类别） |
| `cmd/import/main.go` | 本地导入 CLI `xdigest-import` |
| `internal/model/model.go` | API 的 JSON 结构 |
| `internal/store/*.go` | MariaDB 访问，按领域拆分：`posts.go`、`post_filter.go`、`categories.go`、`user_tags.go`、`replicas.go`、`repos.go`、`yt.go`、`hub.go`、`translations.go`、`refresh_jobs.go` |
| `internal/importer/importer.go` | 旧站 JSON → DB 的幂等导入逻辑 |
| `schema.sql` | 新库 `xdigest` 的建表语句（utf8mb4 / utf8mb4_unicode_ci） |
| `db-user.sql` | 专用 DB 用户创建模板（密码为占位符，见下） |
| `xdigest-api.service` | systemd 单元模板（`xdigest-api`；监听地址取自 api.env，生产为 127.0.0.1:18093） |
| `api.env.example` | 环境变量模板（只写占位符） |

## 运行方式

```bash
cd backend
go build -o xdigest-api ./cmd/api
XDIGEST_ADDR=127.0.0.1:18090 \
XDIGEST_DSN='xdigest:<密码>@tcp(127.0.0.1:3306)/xdigest' \
XDIGEST_ATTACHMENT_DIR=/var/lib/xdigest/attachments \
./xdigest-api
```

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `XDIGEST_ADDR` | `127.0.0.1:18090` | 内网监听地址（代码默认值；生产 api.env 配为 `127.0.0.1:18093`，18090 在服务器上被 chiji-dev 占用） |
| `API_TOKEN` | 空 | 写接口令牌，见下方「安全说明」 |
| `SUMMARY_CONCURRENCY` | `2` | AI 摘要并发上限（1–16） |
| `COMMENTS_ENABLED` | `false` | 视频热门评论 worker 与 `/comments` 接口；关闭时接口返回 410 |
| `XDIGEST_DSN` | `xdigest:CHANGE_ME@tcp(127.0.0.1:3306)/xdigest` | MySQL DSN（密码只放服务器 root-only 的 `/etc/xdigest/api.env`） |
| `XDIGEST_ATTACHMENT_DIR` | `/var/lib/xdigest/attachments` | 附件磁盘目录；DB 只存元数据 + 路径，绝不存 BLOB |

时间：DB 的 DATETIME 列存 Asia/Shanghai 墙时间；旧站 JSON 的 `+08:00`
时间戳导入时转换后存入。`date_from`/`date_to` 按上海当天起止过滤。

## 接口清单（全部 JSON，错误形如 `{"error":"CODE","message":"..."}`）

- `GET /healthz`、`GET /api/v1/healthz` → `{"ok":true}`，并 ping DB，失败 503。
- `GET /api/v1/posts?author=a,b&tag=x&category=cid&user_tag=y&q=kw&date_from=&date_to=&page=&page_size=`
  → `{"updated_at","total","page","page_size","posts":[...]}`；默认 page_size 200，上限 1000；
  排序 `published_at DESC, id ASC`（`sort=oldest` 反向）。`q` 支持 `标签:`/`tag:` 前缀只匹配用户标签（子串、不分大小写）。
  多选：`tags=a,b`、`categories=c1,__uncat`、`user_tags=x,y`（同一维度内为「或」），`kind=教程实操|未分类`。
  响应另含 `has_more` 与 `snippet`（关键词在第一条结果中的命中片段）。
- `GET /api/v1/posts/{id}`（不存在 404）；`PATCH /api/v1/posts/{id} {"replica_enabled": true|false|null}`（手动复刻开关）。
- `GET /api/v1/posts/{id}/summary/stream?lang=`、`GET /api/v1/{yt|bili}/videos/{id}/summary/stream?lang=`：
  AI 摘要的 SSE 推送（`event: summary`），生成结束即推送最终结果。
- `GET /api/v1/watchlist`；`POST /api/v1/watchlist/groups`；`DELETE /api/v1/watchlist/groups/{id}`
  （fixed 分组删不得，422；删后该组作者归入 `other`）。
- `POST /api/v1/watchlist/accounts`（已存在 409）；`PATCH` / `DELETE /api/v1/watchlist/accounts/{username}`。
- `GET /api/v1/tags`（去重，按出现频次倒序）；`GET` / `PUT /api/v1/user-tags`（写入时归一化：全角转半角、大小写不同沿用已有写法）；
  `GET /api/v1/user-tags/duplicates`（疑似重复分组）；`POST /api/v1/user-tags/merge {from[], to}`。
- `GET` / `POST /api/v1/categories`（重名 409）；`PATCH /api/v1/categories/{id} {name}`（重名 409）；
  `DELETE /api/v1/categories/{id}`（软删，7 天后清理）；`POST /api/v1/categories/{id}/restore`（撤销删除）；
  `PUT /api/v1/categories/assignments {post_id, category_ids[]}`。
- `GET /api/v1/replicas?sort=time|progress`；`POST /api/v1/replicas {post_id}`
  （按帖子当前 steps 快照建复刻；同一帖子已存在则 200 返回已有对象并带 `existing:true`，不重复建）。
- `GET` / `PATCH /api/v1/replicas/{id}`（`steps:[{idx,status,note}]`，只更新传来的字段；
  status 只能是 idle/active/done，非法 422）；`DELETE /api/v1/replicas/{id}`（连带删附件文件与行）。
- `POST /api/v1/replicas/{id}/attachments?step_idx=N`（multipart，字段 `file`，可多文件）；
  `GET /api/v1/replicas/{id}/attachments`；`GET` / `DELETE /api/v1/attachments/{id}`；
  `GET /api/v1/attachments/{id}/download`（`?inline=1` 时 inline，否则 attachment）。
- `POST /api/v1/repos/sync-all` 启动后台同步（202，运行中 409）；`GET /api/v1/repos/sync-all` 查进度
  （`{running,total,done,synced,failed,rate_limited,waiting_until}`）。

附件规则：扩展名白名单 pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif
webp bmp avif svg ico；单个 ≤5MB；先全量校验再落盘，
非法/超大返回 422 并说明是哪个文件；文件头与扩展名不符（如改名的 .exe）返回 400 `INVALID_FILE_CONTENT`；磁盘文件名用内部 id（防路径穿越），
原文件名只存元数据。

## 导入（服务器本地 CLI，绝不暴露 HTTP 导入接口）

```bash
go build -o xdigest-import ./cmd/import
./xdigest-import -data-dir <旧站根目录> -dsn 'xdigest:<密码>@tcp(127.0.0.1:3306)/xdigest'
```

读取旧站 `data/posts.json`、`config/watchlist.json`、`config/user-tags.json`、
`data/avatars.json` 全量导入，完成后打印逐项计数。幂等：帖子按主键 upsert，
其 steps/tags 每次重建；分组/作者/作者标签/词汇表按主键 upsert；重复跑不翻倍
（已实测两次结果完全一致）。

## 部署步骤草稿（由协调人/部署方在服务器上执行，本子代理未部署）

1. 服务器上 `mysql < schema.sql` 建库建表；按 `db-user.sql` 模板创建专用用户，
   真实密码只写进 root-only（600）的 `/etc/xdigest/api.env`。
2. 拷 `xdigest-api` 二进制到 `/opt/xdigest/backend/`，拷 `xdigest-api.service`
   到 systemd，`mkdir -p /var/lib/xdigest/attachments` 并 `chown ubuntu`。
3. 把旧站目录拷到服务器本地，跑 `xdigest-import`（只在服务器本地跑）。
4. `systemctl enable --now xdigest-api`；nginx 18089 最终代理 `/` 到 Nuxt、
   `/api/` 到 Go（本仓库不管 nginx 配置）。
5. 头像：协调人把旧站 `avatars/` 拷到 Nuxt `public/avatars`（帖子里的
   `author_avatar` 如 `avatars/ai_xiaomu.jpg` 按 `/avatars/...` 解析）。

## 安全说明

- **写接口鉴权**（`cmd/api/auth.go`）：设置 `API_TOKEN` 后，除 GET/HEAD/OPTIONS 外的所有请求都要带
  `X-API-Token: <token>`（或 `Authorization: Bearer <token>`），否则 401；读接口保持公开，`/healthz` 豁免。
  前端把 token 存在浏览器 localStorage（`xdigest.apiToken`），遇到 401 弹框输入。未设置 `API_TOKEN` 时不校验（仅限本地开发）。
- **nginx 外层**（`deploy/xdigest-18089.nginx`）：写方法 basic auth（`/etc/nginx/xdigest.htpasswd`）+ 按 IP 限流
  （AI / 导入 / 同步 10 次/分，其他 API 10 次/秒，超出 429）。浏览器用 `X-API-Token` 而不用 `Authorization`，正是为了不覆盖 basic auth。
- 附件上传校验文件头魔数（改扩展名的 .exe 返回 400）；下载响应带 `nosniff` 与沙箱 CSP。
- API 只监听内网地址，公网访问必须经 nginx。

## 已知差异 / 未完成项

- **筛选语义的取舍**：`author` 参数出现但为空时返回空列表（原站“空选中 = 空列表”
  语义）；`user_tag` 匹配作者的 watchlist 标签 **或** 自定义覆盖标签（两者并集），
  而原站前端的覆盖语义未在契约中明确，集成时以前端约定为准。
- **标签大小写**：源站同时存在 `workbuddy` 与 `Workbuddy` 两个标签；为与原站
  JS 的精确匹配一致，帖子标签与用户标签的精确匹配用了 `utf8mb4_bin` 校对，
  `/tags` 去重同样按二进制区分，因此返回 408 个（与源站去重后一致），
  而不是 `utf8mb4_unicode_ci` 下合并的 407 个。关键词 `q` 的 LIKE 搜索仍为
  大小写不敏感。
- **帖子分类归属**：旧站的分类归属只存在浏览器 localStorage，不在仓库文件里，
  因此导入后 `assignments` 为空，只有默认分类 `fav`（收藏）；用户需在新版里重新归属。
- **用户标签覆盖**：旧站文件里只有词汇表，没有按作者的覆盖数据，因此导入后
  `overrides` 为空。
- **仅在 watchlist 外的作者**：帖子作者 `weiyong_wang` 不在关注列表中，导入后
  其 `authors` 行 `group_id` 为 NULL，帖子本身仍带完整的作者名/头像字段；
  `/watchlist` 只返回有分组的 12 个作者，而帖子作者总数为 13 个。
- **附件预览**：后端只负责原文件下载与元数据；图片/pdf/md/docx/pptx/zip 的
  浏览器内预览由前端实现（见 SPEC），不在本后端范围内。
- **未做**：Nuxt 前端、nginx 配置、生产部署、鉴权（见上）、Webhook 服务端
  （按 SPEC，webhook 只存在浏览器 localStorage，本后端不实现）。
