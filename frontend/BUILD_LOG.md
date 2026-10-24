# 构建日志

## 环境

- 本机 Node：v24.22.1（`node --version` 实测）
- npm：10.9.4
- Nuxt：4.6.2（`npm view nuxt version` 当日 latest；Nuxt 4 要求 Node ≥ 20.19，Node 24 满足）
- Vue 3.5.44、marked 17.0.6、mammoth 1.13.0、jszip 3.10.2（`package-lock.json` 实测）

## 命令与结果

### 1. `npm install`

- 结果：成功。`added 634 packages`，耗时约 3 分钟。
- 唯一警告：`npm warn deprecated glob@10.5.0`（Nuxt 依赖链带的旧 glob，不影响构建）。

### 2. `npm run build`（第一次）

- 结果：**成功**。`✨ Build complete in 16.0s`，Client + Server + Nitro 均完成，
  `.output/server/index.mjs` 已生成，总大小 3.68 MB（957 kB gzip）。
- 唯一警告：`[INEFFECTIVE_DYNAMIC_IMPORT] composables/useMock.ts is dynamically imported
  by composables/useStore.ts but also statically imported`——已修复：把 mock 的动态 import
  改为静态 import，见第 3 步。

### 3. `npm run build`（修复警告后，第二次）

- 结果：**成功**。`✨ Build complete in 13.6s`，上述警告消失，无 ERROR。

## 冒烟验证（mock 模式，生产产物）

- 首次尝试：`NUXT_PORT=18091 node .output/server/index.mjs` → 服务监听在默认 3000，
  18091 无响应。**结论：NUXT_PORT 对内置 Nitro 服务器不生效**（见 README 的 PORT 说明）。
- 改用 `PORT=18091 NUXT_PUBLIC_USE_MOCK=1 node .output/server/index.mjs`：
  - `GET /` → HTTP 200，7707 字节，含 `x-digest` 标识；
  - `GET /fuke` → HTTP 200，7809 字节，含“复刻”；
  - 日志：`Listening on http://[::]:18091`。
- 注意：SSR 返回的是页面外壳，帖子数据由客户端经 `useApi` 加载（mock 模式下读本地
  归一化数据；API 模式下浏览器请求相对路径 `/api/v1/...`）。该行为已在 README 差异 5 中如实说明。

## 失败 / workaround

- 本次构建无失败。唯一 workaround 是生产端口变量：用 `PORT=18091` 代替任务书原写的
  `NUXT_PORT=18091`。
