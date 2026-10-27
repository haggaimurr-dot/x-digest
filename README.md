# X 博主推文日报

静态站点：多作者筛选、关键词搜索、长文「实现步骤」展开。纯 HTML/CSS/JS，无构建步骤。

- 在线：https://haggaimurr-dot.github.io/x-digest/
- 仓库：https://github.com/haggaimurr-dot/x-digest

## 本地预览

```bash
cd x-digest   # 或本仓库根目录
python3 -m http.server 8080
# 打开 http://localhost:8080
```

或直接用任意静态服务器打开根目录（需能读 `data/posts.json`，勿用 `file://`）。

## 数据文件

| 路径 | 作用 |
|------|------|
| `data/posts.json` | 推文摘要列表（前端只读） |
| `data/raw_posts.json` | X 原文暂存列表；完整对象内保存作者信息，不单独建作者表 |
| `config/watchlist.json` | **权威关注列表**（可含 `avatar` 路径），供每日更新 Agent 使用 |
| `data/avatars.json` | 用户名 → 头像路径映射 |
| `avatars/*.jpg` | 本地缓存的 X 头像（优先于 twimg 热链） |

浏览器设置面板会把关注列表额外写入 `localStorage`（键名 `x-digest-watchlist`），仅影响本机筛选默认值；**改仓库关注列表请编辑 `config/watchlist.json` 并提交**。

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

`author_id`、`author_username`、`author_name` 必填且不得为空。`note_tweet` 或 `long_text` 有值时保存完整长文；无长文时可为 `null`。`referenced_tweets` 保存引用或转帖的原作者信息，至少包含 `author_id` 和 `author_username`，有名称时也保存 `author_name`。`media` 中每项包含 `type`、`url`；`public_metrics` 使用 `like`、`reply`、`repost`、`quote`、`impression` 五项。`source` 为 `builtin-x` 或 `plugin`。

### posts.json 字段

```json
{
  "updatedAt": "2026-10-19T08:00:00+08:00",
  "posts": [
    {
      "id": "tweet-id",
      "url": "https://x.com/...",
      "author": "ai_xiaomu",
      "authorName": "黄小木",
      "title": "短标题",
      "industry": "行业",
      "tech": "技术点",
      "summary": "简介",
      "publishedAt": "2026-10-18T12:00:00+08:00",
      "steps": [
        { "text": "步骤说明", "tools": "工具", "tech": "技术点", "prep": "前提条件" }
      ],
      "tags": ["标签1", "标签2"],
      "isLong": true,
      "showFlowchart": true,
      "note": "可选备注，如「黄小木转发」"
    }
  ]
}
```

## 如何添加作者

1. 编辑 `config/watchlist.json`，在 `accounts` 中增加：
   ```json
   { "username": "handle", "name": "显示名" }
   ```
2. 提交并 push 到 `main`。
3. （可选）在站点「设置」里同步添加到本机 localStorage。

## 原文采集与摘要流程

目标流程为：**X 拉取完整原文 → 写入 `data/raw_posts.json` → 中转站 API 读取原文并产出 `title` / `summary` / `steps` → 写入 `data/posts.json`**。分析由中转站 API 完成，不走 Grok Bot；生成结果仍沿用上面的 `posts.json` 结构，前端只读取摘要文件。

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
