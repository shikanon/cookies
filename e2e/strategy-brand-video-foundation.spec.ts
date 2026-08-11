import { expect, test } from '@playwright/test'

const projectId = 'project_guerlain_abeille_royale_acceptance'

test('isolated Guerlain Project starts a persistent Strategy workspace', async ({ page }) => {
  await page.goto(`/projects/${projectId}/strategy/workspaces`)

  await expect(page.getByRole('heading', { name: '策略工作区', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '娇兰第三代黄金复原蜜品牌广告验收' })).toBeVisible()
  await expect(page.getByText('尚未连接到服务端')).toHaveCount(0)

  const createWorkspace = page.getByRole('button', { name: '创建主策略工作区' })
  const startWorkspace = page.getByRole('button', { name: '开始策略梳理' })
  const currentChain = page.getByText('当前工作链', { exact: true })
  await expect(createWorkspace.or(startWorkspace).or(currentChain)).toBeVisible()
  if (await createWorkspace.isVisible().catch(() => false)) {
    await createWorkspace.click()
    await expect(startWorkspace).toBeVisible()
  }
  await expect(startWorkspace.or(currentChain)).toBeVisible()
  if (await startWorkspace.isVisible().catch(() => false)) {
    await startWorkspace.click()
    await expect(page.getByRole('button', { name: '开始策略对话' })).toBeVisible()
  }

  await page.getByRole('tab', { name: '对话' }).click()
  await expect(page.getByRole('heading', { name: '先说清楚要解决什么。' })).toBeVisible()
  await expect(page.getByLabel('当前工作链状态')).toContainText('需求收敛中')
  await expect(page.locator('#kanon-strategy-message')).toBeEnabled()
  await expect(page.getByRole('button', { name: '发送需求消息' })).toBeDisabled()
})
