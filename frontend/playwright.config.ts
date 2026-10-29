import { defineConfig, devices } from '@playwright/test'

// End-to-end flows against the real stack (Go API + MariaDB + built Nuxt),
// with the outside services replaced by e2e/stub.mjs. The CI e2e job starts
// the API and Nuxt; this config starts the stub (front door on :18080).
// Locally: start MariaDB + API (XPOST_API_BASE / TRANSLATE_API_BASE pointing
// at http://127.0.0.1:18099) and `node .output/server/index.mjs`, then
// `npx playwright test`.
export default defineConfig({
  testDir: 'e2e',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1, // the flows share one database
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: 'http://127.0.0.1:18080',
    locale: 'zh-CN',
    trace: 'retain-on-failure'
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'node e2e/stub.mjs',
    port: 18080,
    reuseExistingServer: !process.env.CI
  }
})
