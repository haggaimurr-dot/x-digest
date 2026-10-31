import { expect, test } from '@playwright/test'

// The three key flows (2026-10-29), in order on one fresh database:
// import a post → replicate it and track progress → attach a file to a step.
// Upstream data comes from e2e/stub.mjs.
const POST_URL = 'https://x.com/e2e_author/status/1900000000000000001'
const POST_TITLE = 'E2E 教程：三步部署日报服务'

test.describe.configure({ mode: 'serial' })

test('导入单帖：粘贴链接后帖子入库并出现在列表中', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: '菜单' }).click()
  await page.getByRole('button', { name: '导入单帖' }).click()
  const dialog = page.getByRole('dialog', { name: '导入单帖' })
  await dialog.locator('textarea').fill(POST_URL)
  await dialog.getByRole('button', { name: '导入', exact: true }).click()
  await expect(dialog.getByText(`已导入：${POST_TITLE}`)).toBeVisible()
  await dialog.getByRole('button', { name: '关闭' }).click()

  // A fresh load (server-rendered first page) shows the new post.
  await page.reload()
  await expect(page.locator('article.post-card', { hasText: POST_TITLE })).toBeVisible()

  // Importing it again reports it as already stored.
  await page.getByRole('button', { name: '菜单' }).click()
  await page.getByRole('button', { name: '导入单帖' }).click()
  await dialog.locator('textarea').fill(POST_URL)
  await dialog.getByRole('button', { name: '导入', exact: true }).click()
  await expect(dialog.getByText(`已在库中：${POST_TITLE}`)).toBeVisible()
})

test('复刻：从帖子建复刻，推进节点后刷新进度仍在', async ({ page }) => {
  await page.goto('/')
  const card = page.locator('article.post-card', { hasText: POST_TITLE })
  await card.getByRole('button', { name: '复刻', exact: true }).click()
  await expect(page.getByText('已生成复刻')).toBeVisible()

  await page.goto('/fuke')
  const flow = page.locator('article.flow-sheet', { hasText: POST_TITLE })
  await expect(flow.getByText('已完成 0 / 3 个节点')).toBeVisible()
  const action = flow.locator('.flow-complete-btn')
  await action.click() // start node 1
  await expect(action).toContainText('完成')
  await action.click() // finish node 1, start node 2
  await expect(flow.getByText('已完成 1 / 3 个节点')).toBeVisible()

  await page.reload()
  const reloaded = page.locator('article.flow-sheet', { hasText: POST_TITLE })
  await expect(reloaded.getByText('已完成 1 / 3 个节点')).toBeVisible()
  await expect(reloaded.locator('.flow-step').first()).toHaveClass(/is-done/)
})

test('附件上传：给节点上传文件、预览、删除', async ({ page }) => {
  await page.goto('/fuke')
  const flow = page.locator('article.flow-sheet', { hasText: POST_TITLE })
  // Node 2 is the active one after the previous test; its details are open.
  const node = flow.locator('.flow-step').nth(1)
  const toggle = node.getByRole('button', { name: /展开详情/ })
  if (await toggle.isVisible()) await toggle.click()

  await node.getByRole('button', { name: '上传', exact: true }).click()
  await node.locator('input[type=file]:not([accept="image/*"])').setInputFiles({
    name: 'e2e-notes.txt', mimeType: 'text/plain', buffer: Buffer.from('E2E 附件内容：部署检查清单')
  })
  const row = node.locator('.attachment-row', { hasText: 'e2e-notes.txt' })
  await expect(row).toBeVisible()

  await row.getByRole('button', { name: '更多操作' }).click()
  await row.getByRole('menuitem', { name: '预览' }).click()
  // The preview is a dialog teleported to <body>, named after the file.
  const preview = page.getByRole('dialog', { name: 'e2e-notes.txt' })
  await expect(preview.getByText('E2E 附件内容：部署检查清单')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(preview).toHaveCount(0)

  // Still there after a reload (stored on the server, not only in the page).
  await page.reload()
  const node2 = page.locator('article.flow-sheet', { hasText: POST_TITLE }).locator('.flow-step').nth(1)
  const toggle2 = node2.getByRole('button', { name: /展开详情/ })
  if (await toggle2.isVisible()) await toggle2.click()
  const row2 = node2.locator('.attachment-row', { hasText: 'e2e-notes.txt' })
  await expect(row2).toBeVisible()

  page.once('dialog', d => d.accept())
  await row2.getByRole('button', { name: '更多操作' }).click()
  await row2.getByRole('menuitem', { name: '删除附件' }).click()
  await expect(row2).toHaveCount(0)
})
