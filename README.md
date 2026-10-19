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
| `config/watchlist.json` | **权威关注列表**（可含 `avatar` 路径），供每日更新 Agent 使用 |
| `data/avatars.json` | 用户名 → 头像路径映射 |
| `avatars/*.jpg` | 本地缓存的 X 头像（优先于 twimg 热链） |

浏览器设置面板会把关注列表额外写入 `localStorage`（键名 `x-digest-watchlist`），仅影响本机筛选默认值；**改仓库关注列表请编辑 `config/watchlist.json` 并提交**。

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

## 每日更新（Agent）

1. 读取 `config/watchlist.json` 得到要拉取的账号。
2. 用 X API / MCP 拉取各账号近期推文，摘要成上述 schema（长文提取编号步骤）。
3. **覆盖写入** `data/posts.json`（更新 `updatedAt`，时区建议 `Asia/Shanghai`）。
4. `git add data/posts.json && git commit -m "chore: daily posts refresh" && git push origin main`。
5. GitHub Pages 会自动发布；站点客户端下次打开即看到新数据。

### 正文完整性与中文（硬规则）

- **禁止用 `…` / `...` 截断** title / summary / steps 正文。短链 API 不够时，改用完整 `note_tweet`，或 `https://api.fxtwitter.com/{user}/status/{id}` 取全文后再写库。
- **面向用户的 title / summary / steps 必须是中文**：英文原文先完整理解再译写，不要把英文半截句子留在 steps 里。
- **长文可以很长**：完整写入 JSON；站点 UI 用「显示更多 / 收起」折叠，不要在数据层截断。

无需后端。Grok Bot 日常调度 routine 的 folder id 为 `x`（由统一调度维护；改规则时用 UpdateRoutine 更新该 routine，不要在本仓库另建一份）。

## GitHub Pages

本仓库根目录即为站点根（`index.html`）。Pages 源：`main` 分支 `/`（root）。
