# x-digest 后端联调记录

- 日期：2026-10-24（本地 VM 联调，未动上海服务器，未动 chiji / bailiankai /
  bailiankai_web，未重启任何其他服务，未碰 Hermes）
- Go：1.23.0（工具链装在 `~/workspace/tools/go`，本仓库外）
- 依赖：`github.com/go-sql-driver/mysql v1.8.1`（`filippo.io/edwards25519 v1.1.0` 间接）
- MariaDB：本机 VM 的 MariaDB 10.11.14（服务启动前为 inactive，联调时本地启动）
- 联调库：本机新建的隔离库 `xdigest`（`schema.sql` 建库建表；专用用户按
  `db-user.sql` 模板创建，密码为一次性随机值，只存在联调期间 `/tmp` 的
  600 文件中，联调结束后已删除该文件并删除该 DB 用户；密码未写入任何工作区文件与本记录）
- 构建：`go vet ./...` 通过（无输出），`go build ./cmd/api` 与 `./cmd/import` 均通过

## 导入（`./xdigest-import -data-dir ~/workspace/x-digest -dsn <DSN>`，跑两次）

两次输出完全一致（幂等，无翻倍）：

```json
{"posts": 190, "post_steps": 323, "post_tags": 843, "authors": 12,
 "groups": 2, "author_tags": 38, "vocabulary": 10, "avatars_seen": 12,
 "categories": 1, "posts_with_steps": 75}
```

导入后 SQL 核查（期望 190 / 13 / 75 全部命中）：

```sql
SELECT COUNT(*) FROM posts;                          -- 190
SELECT COUNT(DISTINCT author) FROM posts;            -- 13
SELECT COUNT(DISTINCT post_id) FROM post_steps;      -- 75
SELECT COUNT(*) FROM authors;                        -- 13（12 关注作者 + 1 仅发帖作者 weiyong_wang）
SELECT COUNT(*) FROM post_tags;                      -- 843
```

注：`authors: 12` 是导入计数里的关注账号数；库内作者行共 13 行。

## API 实测（服务监听 127.0.0.1:18090，附件目录为 /tmp 下的测试目录）

| 命令（摘要） | 结果 |
|---|---|
| `curl /healthz` | 200 `{"ok":true}` |
| `curl /api/v1/healthz` | 200 `{"ok":true}`（DB ping 成功） |
| `curl "/api/v1/posts?page_size=1000"` | 200，`total=190`，`page_size=1000`，`updated_at="2026-10-23 08:13:58"`，返回 190 条；首条为 2113408906479435820（蓝哥AI，Grok Bot） |
| `curl /api/v1/posts/000nonexistent` | 404 `{"error":"NOT_FOUND"}` |
| `curl /api/v1/watchlist` | 200，groups 2 个（special 特别关注、other 默认，均为 fixed），accounts 12 个 |
| `curl /api/v1/tags` | 200，去重标签 408 个（含 `workbuddy` 与 `Workbuddy` 两个大小写变体，见 README 差异说明） |
| `curl "/api/v1/posts?author=ai_xiaomu,miles_mazy"` | 200，`total=41`（22+19，符合源站计数） |
| `curl "/api/v1/posts?author="` | 200，`total=0`（空选中 = 空列表语义） |
| `curl "/api/v1/posts?q=DeepSeek"` | 200，`total=22`，首条为 DeepSeek 中文写作流程帖 |
| `curl "/api/v1/posts?q=标签:AI 工具"` | 200，`total=176` |
| `curl "/api/v1/posts?user_tag=创业商业"` | 200，`total=115` |
| `curl "/api/v1/posts?date_from=2026-10-23&date_to=2026-10-23"` | 200，`total=1` |
| `curl "/api/v1/posts?category=fav"`（导入后） | 200，`total=0`（归属为空，符合预期） |
| `curl /api/v1/categories` | 200，`categories=[{id:fav, name:收藏}]`，`assignments={}` |
| `POST /api/v1/replicas {"post_id":"2113280029421666606"}` | 200，复刻 `rep_1783a0e59daebe15`，steps 3 条，progress `{done:0,total:3}` |
| 再次 `POST` 同一 post_id | 200，同一 id，`existing:true`（未重复建） |
| `PATCH /api/v1/replicas/{id} {"steps":[{"idx":0,"status":"active","note":"测试备注"}]}` | 200，step0 为 active，备注已保存，progress `{done:0,total:3}` |
| `PATCH ... {"steps":[{"idx":0,"status":"done"}]}` | 200，step0 为 done，备注保留，progress `{done:1,total:3}` |
| `PATCH ... {"steps":[{"idx":0,"status":"bogus"}]}` | 422 `VALIDATION: invalid status: bogus` |
| `GET /api/v1/replicas?sort=progress` | 200，1 条，progress `{done:1,total:3}` |
| `POST /api/v1/watchlist/groups {"name":"测试组"}` | 201，新分组 id `grp_…` |
| `DELETE /api/v1/watchlist/groups/special` | 422 `GROUP_FIXED` |
| `DELETE` 新分组 | 200 |
| `POST /api/v1/watchlist/accounts`（test_user）→ `PATCH` 改 tags → `GET /watchlist` 核对 → `DELETE` | 201 / 200 / tags 已改为 `["编程开发"]` / 200 |
| `PUT /api/v1/user-tags {"username":"ai_xiaomu","tags":["AI 工具","自测标签"]}` → `GET /api/v1/user-tags` | 200；overrides 含该作者；联调后已 PUT 空数组清回，`overrides={}` |
| `POST /api/v1/categories {"name":"测试分类"}` → `PUT /api/v1/categories/assignments`（帖子 2113408906479435820）→ `GET /api/v1/posts?category={id}` → `DELETE` 该分类 | 201 / 200 / `total=1` / 200 |
| `POST /api/v1/replicas/{id}/attachments?step_idx=0 -F file=@小txt`（内容 `hello xdigest attachment test`） | 201，`id=att_…`，`size=30`，`ext=txt`，`content_type=text/plain`，含 download_url |
| `GET /api/v1/attachments/{id}/download` | 200，内容与上传一致；响应头 `Content-Disposition: attachment; filename="test-note.txt"` |
| `GET /api/v1/attachments/{id}` | 200，元数据与上传一致 |
| `GET /api/v1/replicas/{id}/attachments` | 200，两次上传后 `count=2` |
| 上传 `.exe` 文件 | 422 `INVALID_FILE: file "bad.exe": extension .exe is not allowed` |
| 上传 11MB txt | 422 `FILE_TOO_LARGE: size 11534336 exceeds 10MB limit` |
| 上传 `step_idx=99` | 422 `VALIDATION: step_idx … does not exist` |
| `DELETE /api/v1/attachments/{id}` 后再 `GET` | 200 删除；再 GET 为 404；磁盘文件同步删除（目录文件数 2→1） |
| `DELETE /api/v1/replicas/{id}` | 200；剩余附件磁盘文件同步删除（目录文件数 1→0）；再 GET 复刻为 404；库内 replicas / replica_steps / attachments 计数均为 0 |

## 联调后状态

- 测试用复刻、附件、测试分组/账号/分类、用户标签覆盖均已删除或清回；
  库内帖子/作者/标签数据保持导入状态（posts 190、authors 13、replicas 0、attachments 0）。
- 测试 API 进程已停止；本地测试 DB 用户与临时密码文件已删除。
- 未完成项见 README.md（无登录、附件预览属前端、分类归属与标签覆盖需用户在新版里重建）。
