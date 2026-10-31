# 协作约定

## 提交

- **一个提交只做一件事**：按领域拆分（例如「后端 · 视频评论下线」「前端 · Webhook 下线」各一个提交），不要把多个领域的改动合在一个提交里，方便评审和回滚。2026-10-29 起执行（此前 `500c4fb` 把多个领域的拆分合在了一个提交里，历史不再改写）。
- 提交信息用中文，第一行写清「模块：做了什么」，正文列出要点和需要人工执行的事项（如执行 `schema.sql`、删表脚本）。
- 改了表结构就同步改 `backend/schema.sql`（只用 `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` 这类可重复执行的语句）；删表、删列写成 `backend/scripts/` 下单独的脚本，由人工备份后执行。

## 提交前自测

```bash
cd backend && go vet ./... && gofmt -l cmd internal && go test ./...
cd frontend && npm test && npm run build
```

- 需要数据库的用例在没有 `XDIGEST_TEST_DSN` 时会跳过；CI 带 MariaDB，并设置了 `XDIGEST_REQUIRE_DB=1`，DB 用例在 CI 中不允许跳过。
- 单个 Go 源文件尽量不超过 600 行，CI 会对超出的文件给出警告。

## 分支与合并

- 日常开发在 `muse-dev`，通过 PR 合入 `main`；PR 上的 CI（后端、前端、E2E）全部通过后再合并。
