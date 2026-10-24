# x-digest 后端（Go + MariaDB）

实现 `~/workspace/xdigest-new/SPEC.md` 的全部服务端契约。只含 Go 后端、
MariaDB schema 与本地导入 CLI；不含 Nuxt 前端，不改前端目录，不推 git。

## 结构

| 文件 | 说明 |
|---|---|
| `cmd/api/main.go` | HTTP API（标准库 net/http，全部接口） |
| `cmd/import/main.go` | 本地导入 CLI `xdigest-import` |
| `internal/model/model.go` | API 的 JSON 结构 |
| `internal/store/store.go` | 全部 MariaDB 访问 |
| `internal/importer/importer.go` | 旧站 JSON → DB 的幂等导入逻辑 |
| `schema.sql` | 新库 `xdigest` 的建表语句（utf8mb4 / utf8mb4_unicode_ci） |
| `db-user.sql` | 专用 DB 用户创建模板（密码为占位符，见下） |
| `xdigest-api.service` | systemd 单元模板（`xdigest-api`，127.0.0.1:18090） |
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
| `XDIGEST_ADDR` | `127.0.0.1:18090` | 内网监听地址 |
| `XDIGEST_DSN` | `xdigest:CHANGE_ME@tcp(127.0.0.1:3306)/xdigest` | MySQL DSN（密码只放服务器 root-only 的 `/etc/xdigest/api.env`） |
| `XDIGEST_ATTACHMENT_DIR` | `/var/lib/xdigest/attachments` | 附件磁盘目录；DB 只存元数据 + 路径，绝不存 BLOB |

时间：DB 的 DATETIME 列存 Asia/Shanghai 墙时间；旧站 JSON 的 `+08:00`
时间戳导入时转换后存入。`date_from`/`date_to` 按上海当天起止过滤。

## 接口清单（全部 JSON，错误形如 `{"error":"CODE","message":"..."}`）

- `GET /healthz`、`GET /api/v1/healthz` → `{"ok":true}`，并 ping DB，失败 503。
- `GET /api/v1/posts?author=a,b&tag=x&category=cid&user_tag=y&q=kw&date_from=&date_to=&page=&page_size=`
  → `{"updated_at","total","page","page_size","posts":[...]}`；默认 page_size 200，上限 1000；
  排序 `published_at DESC, id ASC`。`q` 支持 `标签:`/`tag:` 前缀只匹配用户标签。
- `GET /api/v1/posts/{id}`（不存在 404）。
- `GET /api/v1/watchlist`；`POST /api/v1/watchlist/groups`；`DELETE /api/v1/watchlist/groups/{id}`
  （fixed 分组删不得，422；删后该组作者归入 `other`）。
- `POST /api/v1/watchlist/accounts`（已存在 409）；`PATCH` / `DELETE /api/v1/watchlist/accounts/{username}`。
- `GET /api/v1/tags`（去重，按出现频次倒序）；`GET` / `PUT /api/v1/user-tags`。
- `GET` / `POST /api/v1/categories`；`DELETE /api/v1/categories/{id}`；
  `PUT /api/v1/categories/assignments {post_id, category_ids[]}`。
- `GET /api/v1/replicas?sort=time|progress`；`POST /api/v1/replicas {post_id}`
  （按帖子当前 steps 快照建复刻；同一帖子已存在则 200 返回已有对象并带 `existing:true`，不重复建）。
- `GET` / `PATCH /api/v1/replicas/{id}`（`steps:[{idx,status,note}]`，只更新传来的字段；
  status 只能是 idle/active/done，非法 422）；`DELETE /api/v1/replicas/{id}`（连带删附件文件与行）。
- `POST /api/v1/replicas/{id}/attachments?step_idx=N`（multipart，字段 `file`，可多文件）；
  `GET /api/v1/replicas/{id}/attachments`；`GET` / `DELETE /api/v1/attachments/{id}`；
  `GET /api/v1/attachments/{id}/download`（`?inline=1` 时 inline，否则 attachment）。

附件规则：扩展名白名单 pdf doc docx xls xlsx ppt pptx txt md csv png jpg jpeg gif
webp bmp avif svg ico zip rar 7z tar gz tgz；单个 ≤10MB；先全量校验再落盘，
非法/超大返回 422 并说明是哪个文件；磁盘文件名用内部 id（防路径穿越），
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

## 安全说明（无登录，如实写明）

本 API **没有任何鉴权**：任何能访问到它的人都能读写关注列表、分类、复刻与附件。
因此它只能监听内网（默认 127.0.0.1:18090），公网访问必须经 nginx 反代，
且在生产上线前应补一层鉴权。建议后续方案（二选一，不擅自实施）：

1. nginx 层 basic auth / IP 白名单（最快，适合单人调试）；
2. 在 Go 层加 Bearer token 中间件：写接口要求 `Authorization: Bearer <token>`，
   token 放 `api.env` 的 `XDIGEST_API_TOKEN`，读接口保持公开。

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
