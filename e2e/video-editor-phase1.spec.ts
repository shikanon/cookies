import { expect, test, type Page } from '@playwright/test'

const projectId = 'project_investor_precision_evidence'
const editorHub = `/projects/${projectId}/creative/video/editing`

test('independent entry creates an EditTask and stable URL restores its frozen timeline', async ({ page }) => {
  await page.goto(editorHub)

  await expect(page.getByRole('heading', { name: '素材剪辑任务' })).toBeVisible()
  await page.getByRole('button', { name: '新建剪辑' }).click()
  await expect(page).toHaveURL(new RegExp(`/creative/video/editing/edit_[^/?]+$`))
  await expect(page.locator('.video-editing-workspace.c3-editor-workspace')).toBeVisible()
  await page.getByRole('button', { name: '视频', exact: true }).click()
  await expect(page.locator('.c3-asset-list article')).toHaveCount(1)

  await page.locator('.c3-asset-list article').first().getByRole('button', { name: '加入所选轨道' }).click()
  await expect(page.locator('.c3-track-clip.kind-video')).toHaveCount(1)
  await expect(page.getByText(/时间线 v1 · 已保存/)).toBeVisible({ timeout: 10_000 })

  const stableURL = page.url()
  await page.reload()
  await expect(page).toHaveURL(stableURL)
  await expect(page.locator('.c3-track-clip.kind-video')).toHaveCount(1)
  await expect(page.getByText(/时间线 v1 · 已保存/)).toBeVisible()
  await expect(page.getByText(/素材剪辑|短剧前贴|创意成片/).first()).toBeVisible()
})

test('two browser contexts expose a 409 conflict and recovery choices instead of overwriting', async ({ browser, request }) => {
  const response = await request.post(`/api/creative/v1/projects/${projectId}/edit-tasks`, {
    data: { display_name: `C7 冲突验收 ${Date.now()}` },
  })
  expect(response.ok()).toBeTruthy()
  const task = await response.json() as { id: string }
  const taskURL = `${editorHub}/${task.id}`

  const left = await browser.newContext()
  const right = await browser.newContext()
  try {
    const [leftPage, rightPage] = await Promise.all([left.newPage(), right.newPage()])
    await Promise.all([openEmptyTask(leftPage, taskURL), openEmptyTask(rightPage, taskURL)])

    await Promise.all([
      leftPage.locator('.c3-asset-list article').first().getByRole('button', { name: '加入所选轨道' }).click(),
      rightPage.locator('.c3-asset-list article').first().getByRole('button', { name: '加入所选轨道' }).click(),
    ])

    const leftConflict = leftPage.getByRole('alert')
    const rightConflict = rightPage.getByRole('alert')
    await expect.poll(async () => Number(await leftConflict.isVisible()) + Number(await rightConflict.isVisible()), { timeout: 12_000 }).toBe(1)
    const conflictPage = await leftConflict.isVisible() ? leftPage : rightPage
    const savedPage = conflictPage === leftPage ? rightPage : leftPage

    await expect(savedPage.getByText(/时间线 v1 · 已保存/)).toBeVisible()
    await expect(conflictPage.getByRole('button', { name: '载入服务端版本' })).toBeVisible()
    await expect(conflictPage.getByRole('button', { name: '另存为新 EditTask' })).toBeVisible()
    await conflictPage.getByRole('button', { name: '载入服务端版本' }).click()
    await expect(conflictPage.getByText(/时间线 v1 · 已保存/)).toBeVisible()
    await expect(conflictPage.locator('.c3-track-clip.kind-video')).toHaveCount(1)
  } finally {
    await Promise.all([left.close(), right.close()])
  }
})

test('editor remains reachable and actionable at C7 target desktop widths', async ({ page, request }) => {
  const response = await request.post(`/api/creative/v1/projects/${projectId}/edit-tasks`, {
    data: { display_name: `C7 视口验收 ${Date.now()}` },
  })
  const task = await response.json() as { id: string }

  for (const width of [1280, 1366, 1440, 1920]) {
    await page.setViewportSize({ width, height: 900 })
    await page.goto(`${editorHub}/${task.id}`)
    await expect(page.locator('.c3-assets')).toBeVisible()
    await expect(page.locator('.c3-center')).toBeVisible()
    await expect(page.locator('.c3-inspector')).toBeVisible()
    await expect(page.getByRole('button', { name: '正式导出' }).first()).toBeVisible()
    const viewport = await page.evaluate(() => ({ documentWidth: document.documentElement.scrollWidth, viewportWidth: document.documentElement.clientWidth }))
    expect(viewport.documentWidth).toBeLessThanOrEqual(viewport.viewportWidth)
  }
})

async function openEmptyTask(page: Page, url: string) {
  await page.goto(url)
  await expect(page.locator('.video-editing-workspace.c3-editor-workspace')).toBeVisible()
  await page.getByRole('button', { name: '视频', exact: true }).click()
  await expect(page.locator('.c3-asset-list article')).toHaveCount(1)
  await expect(page.getByText(/时间线 v0 · 已保存/)).toBeVisible()
}
