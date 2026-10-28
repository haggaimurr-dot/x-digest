# x-digest 项目进度 · 可测试内容清单

- 生成日期：2026-10-28
- 分支：`muse-dev`（HEAD `5066e4e`）
- 实际结果来源说明：
  - **【本次实测】**：2026-10-28 在本机（Windows，Go 1.26.1，Node v24.20.0）实际运行得到；
  - **【联调记录】**：引自 `backend/TEST_LOG.md`（2026-10-24 本地 VM + MariaDB 10.11 联调）或 `frontend/BUILD_LOG.md`；
  - **【待测】**：功能已实现，但目前没有自动化用例，也没有联调记录，需要人工在页面上验证。
- 达成率口径：已通过的验证点 ÷ 该功能点的验证点总数。待测项按代码完成度预估，并标注“（预估）”。

## 一、总体结果

| 项目 | 结果 |
|---|---|
| 后端 `go vet ./...` | 通过，无输出【本次实测】 |
| 后端 `go test ./...` | 10 个包全部 ok；**86 PASS / 14 SKIP / 0 FAIL**【本次实测】 |
| SKIP 原因 | 14 个油管相关的 DB 用例需要环境变量 `XDIGEST_TEST_DSN`（MariaDB），本机未配置 |
| 前端 `node --test utils/x-post.test.mjs` | 3/3 通过【本次实测】 |
| 前端 `npm run build` | 本机未安装 node_modules，本次未执行；BUILD_LOG 记录构建成功【联调记录】 |
| 线上 `http://42.192.114.201:18089/` | 返回 **401 Authorization Required**（nginx 已加 basic auth，README 里“无登录”的风险已在 nginx 层缓解）【本次实测】 |

## 二、可测试内容明细

| 模块 | 功能点 | 测试步骤 | 预期结果 | 实际结果 | 达成率 | 备注 |
|---|---|---|---|---|---|---|
| 基础/构建 | 后端静态检查与编译 | `cd backend && go vet ./... && go build ./cmd/api ./cmd/import` | 无报错 | vet 无输出，构建通过【本次实测】 | 100% | |
| 基础/构建 | 前端生产构建 | `cd frontend && npm install && npm run build` | Build complete，生成 `.output/server/index.mjs` | 构建成功，13.6s，无 ERROR【联调记录】 | 100% | 生产端口要用 `PORT`，`NUXT_PORT` 无效 |
| 基础/构建 | 健康检查 | `curl /healthz`、`curl /api/v1/healthz` | 200 `{"ok":true}` | 均为 200，DB ping 成功【联调记录】 | 100% | |
| 数据导入 | 旧数据导入 CLI（幂等） | `xdigest-import -data-dir <dir> -dsn <DSN>` 连跑两次，再用 SQL 计数 | posts 190、post_steps 323、post_tags 843、作者 13、带步骤帖 75；两次结果相同 | 两次输出一致，计数全部命中【联调记录】 | 100% | |
| 数据导入 | 原文字段解析（expanded_url、引用正文、头像） | `go test ./internal/importer` | 测试通过 | TestAvatarFromRaw、TestRawExpandedURLsAndReferencedText 通过【本次实测】 | 100% | |
| X 日报 | 帖子列表与筛选接口 | `GET /api/v1/posts?page_size=1000`；分别带 `author=`、`q=DeepSeek`、`q=标签:AI 工具`、`user_tag=`、`date_from/date_to` | total 分别为 190 / 0 / 22 / 176 / 115 / 1 | 全部与预期一致【联调记录】 | 100% | 页面上的筛选仍在前端执行 |
| X 日报 | 帖子详情 | `GET /api/v1/posts/{id}`，以及不存在的 id | 存在返回帖子，不存在返回 404 `NOT_FOUND` | 符合预期【联调记录】 | 100% | |
| X 日报 | 页面卡片展示（按作者分组、原文折叠、头像回退） | 打开 `/`，查看卡片；找一条 >120 字的原文点“展开/收起”；把某作者的头像地址改坏 | 按作者分组；长原文默认折叠为 3 行；头像按 库值→关注列表→/avatars→首字母 依次回退 | 【待测】 | 90%（预估） | 需人工在浏览器验证 |
| X 日报 | 帖子类型判定（教程可复刻 / 感谢类不可复刻） | `node --test frontend/utils/x-post.test.mjs` | 3 个用例通过 | 3/3 通过【本次实测】 | 100% | |
| X 日报 | AI 摘要（生成/读库/重新生成） | 卡片点“✨ AI 摘要”；再点一次；点“重新生成” | 首次 202 后轮询出结果；第二次直接读库；重新生成会覆盖 | 单测 TestSummarizePost 通过【本次实测】；页面流程【待测】 | 60% | 需服务器配置 `TRANSLATE_API_KEY`，否则 503 |
| X 日报 | 导入单帖 | 点“导入单帖”，粘贴一个 x.com 帖子链接；作者不在关注列表时再试一次 | 帖子入库并显示；作者自动加入“默认”分组 | URL 解析、资源过滤单测通过（TestParseStatusURL、TestKeepResourcesOnlyRealLinks）【本次实测】；端到端【待测】 | 50% | 依赖 fxtwitter 与中转站 AI |
| X 日报 | 更新帖子（Grok 找新帖） | 选账号或不选，点“更新帖子”，观察进度 | 后台任务逐条入库；15 字以内的短回复跳过；Grok 编造的 id 不入库 | TestPlanFiltersShortRepliesAndResumes、TestParseIDList、TestParseModel 通过【本次实测】；端到端【待测】 | 50% | 需 `GROK_API_KEY` |
| X 日报 | 关注列表管理 | 新增分组 → 删除固定分组 → 删除新分组；新增账号 → 改 tags → 删除；页面上测试搜索和折叠 | 201 / 422 `GROUP_FIXED` / 200；账号 CRUD 正常；页面新增的账号显示在最上面 | 接口全部符合预期【联调记录】；页面搜索/折叠【待测】 | 80% | |
| X 日报 | 用户标签覆盖 | `PUT /api/v1/user-tags`，再 GET；在页面“标签管理”里改常用标签并刷新 | 覆盖已保存；刷新后常用标签还在 | 接口符合预期【联调记录】；页面【待测】 | 70% | c72e6fb 修复过“常用标签不保存” |
| X 日报 | 分类与归属 | 新建分类 → PUT 归属 → `GET posts?category=` → 删除分类 | 201 / 200 / total=1 / 200 | 符合预期【联调记录】 | 100% | 旧数据里没有分类归属，需要重新归属 |
| X 日报 | 翻译接口 | `go test ./internal/translate ./cmd/api -run Translate` | 缓存命中、中文直接短路、未配置时 503、参数校验 | 13 个相关用例全部通过【本次实测】 | 100% | |
| 复刻 (/fuke) | 创建复刻（不重复建） | 对同一 post_id 连续 `POST /api/v1/replicas` 两次 | 第二次返回同一个 id，`existing:true` | 符合预期【联调记录】 | 100% | |
| 复刻 (/fuke) | 步骤状态与备注 | PATCH 步骤为 active+备注 → done → 非法状态 bogus | 进度 0/3 → 1/3，备注保留；非法状态返回 422 | 符合预期【联调记录】 | 100% | 页面上三态循环点击【待测】 |
| 复刻 (/fuke) | 附件上传/下载/删除 | 上传 txt；下载；上传 .exe、11MB 文件、step_idx=99；删除附件；删除复刻 | 201；内容一致；三种异常都返回 422；删除后磁盘文件同步删除 | 全部符合预期【联调记录】 | 100% | |
| 复刻 (/fuke) | 附件预览（PDF/DOCX/PPTX/图片） | 页面上分别上传各类型文件并点预览 | 能预览；xls 只能下载 | 【待测】 | 80%（预估） | 已知差异：PPTX 图片没有按 slide 精确对应 |
| 复刻 (/fuke) | 备注编辑器（Markdown） | 在步骤备注里输入 Markdown，用工具栏插入格式，切换预览 | 预览渲染正确，保存后刷新还在 | 【待测】 | 85%（预估） | 自研 textarea + marked，没有用 EasyMDE |
| 仓库 (/repos) | 同步 GitHub 仓库 | `go test ./internal/github` | 成功同步；限流时保留旧数据；404/私有仓库给出提示；Release 分页 | 15 个用例全部通过【本次实测】 | 100% | |
| 仓库 (/repos) | README 解析（简介、安装步骤） | 同上 | 徽章、简介截断、安装步骤、嵌套列表等解析正确 | 全部通过【本次实测】 | 100% | |
| 仓库 (/repos) | 页面：添加/分组/标签/备注/刷新/删除 | 打开 `/repos`，添加 `owner/repo`，改分组和标签，点刷新，最后删除 | 卡片展示星标、Issue/PR 数、最近提交、Release；操作后马上生效 | 【待测】 | 85%（预估） | 多仓库频繁刷新需要 `GITHUB_TOKEN` |
| 油管 (/youtube) | 频道解析与视频同步（yt-dlp） | `go test ./internal/youtube` | 各种频道 URL 能解析；多 tab 同步；部分失败/429 能容错；日期范围过滤 | 全部通过【本次实测】 | 100% | |
| 油管 (/youtube) | 频道/视频接口（DB） | 设置 `XDIGEST_TEST_DSN` 后运行 `go test ./cmd/api ./internal/store` | 添加频道 422/409、日期范围、分组删除、级联删除等 | **14 个用例 SKIP**（本机没有 MariaDB）【本次实测】 | 0%（本次未覆盖） | 需要连测试库重新跑 |
| 油管 (/youtube) | 添加单个视频并登记频道 | 页面粘贴单个视频链接 → 点“同步该账号全部视频” | 视频入库，频道自动登记，再同步出全部视频 | TestFetchVideo 通过【本次实测】；页面【待测】 | 50% | a9d8d4b 新增 |
| 油管 (/youtube) | 字幕 + AI 章节摘要 | `go test ./internal/ytsummary`；页面点“✨ AI 摘要” | 保留作者章节；没有字幕时只根据标题和简介生成；长字幕会压缩 | 7 个用例通过【本次实测】；页面【待测】 | 70% | 需要 `TRANSLATE_API_KEY`、`YT_HTTP_PROXY` |
| 油管/B站 | 视频卡片（分组折叠、自动标签、资源区） | 打开 `/youtube`、`/bilibili`，折叠账号，按标签筛选 | 能按账号折叠；Claude Code、Cursor 等标签识别正确；资源里去掉了社交账号链接 | 【待测】 | 85%（预估） | 规则在 `frontend/utils/videoTags.ts`，没有单测 |
| B站 (/bilibili) | UP 主解析与视频同步 | `go test ./internal/bilibili` | WBI 签名、mid 解析、视频与合集同步、优先中文人工字幕、风控报错可读 | 8 个用例通过【本次实测】 | 100% | |
| 评论 | 热门评论接口 | `go test ./cmd/api -run Comment` 以及 youtube/bilibili 评论用例 | 去重、退避、按点赞排序、评论关闭时能处理 | 通过【本次实测】 | 100% | 页面已不再调用 |
| 全站 | 外观设置（深/浅色、Aa 外观）、中英切换、移动端 | 顶栏切换主题和语言；用手机宽度（375px）浏览各页面 | 切换立即生效，刷新后保留；移动端没有横向滚动 | 【待测】 | 85%（预估） | |
| 部署/安全 | 线上访问控制 | `curl -I http://42.192.114.201:18089/` | 未带凭证时返回 401 | 401【本次实测】 | 100% | 带凭证的线上回归需要用户提供账号后人工测试 |

## 三、各模块达成率汇总

| 模块 | 功能点数 | 平均达成率 | 主要缺口 |
|---|---|---|---|
| 基础/构建/导入 | 5 | 100% | — |
| X 日报 | 11 | 82% | AI 摘要、导入单帖、更新帖子缺少端到端验证 |
| 复刻 | 5 | 93% | 页面预览和编辑器靠人工验证 |
| 仓库 | 3 | 95% | 页面操作待测 |
| 油管 / B站 / 评论 | 7 | 72% | 14 个 DB 用例需要 MariaDB 才能跑；页面待测 |
| 全站 / 部署 | 2 | 93% | 主题和移动端待人工验证 |
| **合计** | **33** | **约 86%** | |

## 四、建议的下一步测试

1. 配置 `XDIGEST_TEST_DSN` 指向一个隔离的 MariaDB 测试库，补跑 14 个 SKIP 用例。
2. 在本地 `npm install && npm run build` 重新确认前端在当前代码下能构建（BUILD_LOG 是 10-24 的结果，之后改动很多）。
3. 用带凭证的浏览器对线上做一轮手工回归，把本表里的【待测】项逐条更新为实际结果。
4. `videoTags.ts` 和前端筛选逻辑目前没有单测，可以参照 `x-post.test.mjs` 补上。
