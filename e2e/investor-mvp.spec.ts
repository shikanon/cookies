import { expect, test, type Page } from '@playwright/test'

const apiBaseURL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8787'

test.beforeEach(async ({ page }) => {
  const response = await page.request.post(`${apiBaseURL}/api/session`, {
    data: { email: 'demo@cookies.local', password: 'cookies-demo' },
  })
  expect(response.ok()).toBeTruthy()
})

async function createProjectWithBrief(page: Page, name: string) {
  const projectResponse = await page.request.post(`${apiBaseURL}/api/projects`, {
    data: { name, brand: 'E2E 隔离品牌', objective: '独立验证当前项目服务端事实来源' },
  })
  expect(projectResponse.ok()).toBeTruthy()
  const project = await projectResponse.json() as { id: string }
  const briefResponse = await page.request.post(`${apiBaseURL}/api/artifacts`, {
    data: { projectId: project.id, kind: 'brief', status: 'ready', content: `${name} 已确认 Brief` },
  })
  expect(briefResponse.ok()).toBeTruthy()
  return project.id
}

async function getReadyBriefId(page: Page, projectId: string) {
  const response = await page.request.get(`${apiBaseURL}/api/artifacts?projectId=${projectId}`)
  expect(response.ok()).toBeTruthy()
  const artifacts = await response.json() as Array<{ id: string, kind: string, status: string }>
  const brief = artifacts.find(artifact => artifact.kind === 'brief' && artifact.status === 'ready')
  expect(brief).toBeTruthy()
  return brief!.id
}

async function openProject(page: Page, name: string) {
  const projectId = await createProjectWithBrief(page, name)
  await page.goto(`/projects/${projectId}/home`)
  await expect(page.getByRole('heading', { name })).toBeVisible()
  return projectId
}

async function setProviderMode(page: Page, mode: 'success' | 'create_failure' | 'task_failure') {
  const response = await page.request.post('http://127.0.0.1:8791/test/mode', { data: { mode } })
  expect(response.ok()).toBeTruthy()
}

async function selectShortDramaCandidate(page: Page) {
  await page.getByLabel('短剧标题').fill('雨夜归来的继承人')
  await page.getByLabel('故事梗概').fill('被逐出家门的女主在雨夜带着证据归来，发现继母正在转移家产，并必须在家族晚宴前揭开真相。')
  await page.getByLabel('已审核卖点').fill('豪门继承权争夺')
  await page.getByLabel('正片首句（可选）').fill('你以为我今晚回来，是为了求你吗？')
  await page.getByRole('button', { name: '生成 AI 候选' }).click()
  const candidatePanel = page.getByRole('complementary', { name: '短剧前贴 AI 候选' })
  await expect(candidatePanel.getByText('需人工选择')).toBeVisible()
  await expect(candidatePanel.getByText('评分仅表示钩子机制相关性，不代表转化效果预测。')).toBeVisible()
  await expect(candidatePanel.getByRole('button')).toHaveCount(4)
  await candidatePanel.getByRole('button').first().click()
  await expect(page.getByRole('status')).toContainText('已人工选择')
  await expect(page.getByLabel('当前镜头预览')).toContainText('已选候选')
}

async function syncPrerollJob(
  page: Page,
  projectId: string,
  prerollType: 'short_drama' | 'game' | 'commerce',
  expectedStatus: 'succeeded' | 'failed',
) {
  const query = `projectId=${projectId}&purpose=preroll&prerollType=${prerollType}`
  await expect.poll(async () => {
    const jobsResponse = await page.request.get(`${apiBaseURL}/api/generation-jobs?${query}`)
    const jobs = await jobsResponse.json() as Array<{ id: string }>
    const job = jobs.at(-1)
    if (!job) return false
    const response = await page.request.get(`${apiBaseURL}/api/generation-jobs/${job.id}?${query}`)
    const synced = await response.json() as { status: string }
    return synced.status === expectedStatus
  }, { timeout: 10_000 }).toBe(true)
}

async function expectInitialViewportControl(page: Page, locator: ReturnType<Page['locator']>, width: number) {
  await expect(locator).toBeVisible()
  await expect(locator).toBeEnabled()
  const box = await locator.boundingBox()
  expect(box).not.toBeNull()
  expect(box!.x).toBeGreaterThanOrEqual(0)
  expect(box!.x + box!.width).toBeLessThanOrEqual(width)
  expect(box!.y).toBeGreaterThanOrEqual(0)
  expect(box!.y + box!.height).toBeLessThanOrEqual(await page.evaluate(() => window.innerHeight))
  await locator.click({ trial: true })
}

async function expectNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => {
    const pageContainer = document.querySelector<HTMLElement>('.page-frame')
    return document.documentElement.scrollWidth <= window.innerWidth
      && document.body.scrollWidth <= window.innerWidth
      && (pageContainer?.scrollWidth ?? 0) <= (pageContainer?.clientWidth ?? 0)
  })).toBeTruthy()
}

async function expectInitialViewportElement(page: Page, locator: ReturnType<Page['locator']>, width: number) {
  await expect(locator).toBeVisible()
  const box = await locator.boundingBox()
  expect(box).not.toBeNull()
  expect(box!.x).toBeGreaterThanOrEqual(0)
  expect(box!.x + box!.width).toBeLessThanOrEqual(width)
  expect(box!.y).toBeGreaterThanOrEqual(0)
  expect(box!.y + box!.height).toBeLessThanOrEqual(await page.evaluate(() => window.innerHeight))
}

test('项目主路径仅使用本用例创建的 Project 和 Brief', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 独立主路径')

  await expect(page).toHaveURL(`/projects/${projectId}/home`)
  await expect(page.getByRole('region', { name: '项目八阶段业务流程' })).toBeVisible()
  await page.getByRole('button', { name: '投后承接阶段 06 至 08 投放数据形成复盘，再沉淀为下一轮经验' }).click()
  await expect(page).toHaveURL(`/projects/${projectId}/insight/performance`)
  await expect(page.getByText('没有可用的投后运营数据')).toBeVisible()
})

test('创意镜头支持键盘切换并同步当前预览', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 键盘镜头')
  await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
  await page.getByRole('tab', { name: /游戏前贴/ }).click()

  const firstShot = page.getByRole('button', { name: /展示公差挑战目标/ })
  await firstShot.focus()
  await page.keyboard.press('ArrowDown')

  await expect(page.getByRole('button', { name: /失败反馈与进度掉落/ })).toBeFocused()
  await expect(page.getByRole('status')).toContainText('当前镜头：02 · 失败反馈与进度掉落。')
  await expect(page.getByLabel('当前镜头预览')).toContainText('02 / 03')
})

test('爆款视频复刻支持五维提示词拆解并生成复刻视频', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 爆款视频复刻')
  await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
  await setProviderMode(page, 'success')
  await page.getByRole('tab', { name: /爆款复刻/ }).click()

  await page.getByLabel('源视频 Asset ID').fill('viral-source-e2e')
  await page.getByLabel('视频标题').fill('E2E 爆款源视频')
  await page.getByLabel('上传参考图片').setInputFiles({
    name: 'e2e-reference-product.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=', 'base64'),
  })
  await expect(page.locator('.viral-image-preview').getByText('e2e-reference-product.png')).toBeVisible()
  await page.getByLabel('文本指令').fill('更年轻化，突出夏季户外场景，参考图片主体必须保持清晰。')
  await page.getByLabel('目标产品').fill('E2E 智能投放助手')
  await page.getByLabel('卖点 1').fill('3 秒提炼爆款结构')
  await page.getByLabel('CTA').fill('立即生成复刻脚本')
  await page.getByRole('button', { name: '视觉理解拆解五维提示词' }).click()

  await expect(page.getByRole('status')).toContainText('视觉理解拆解完成')
  await expect(page.locator('.viral-dimension-card').filter({ hasText: '任务目标类型' }).getByRole('textbox')).toHaveValue(/生成一条 30s 左右的爆款复刻广告视频/)
  await expect(page.locator('.viral-dimension-card').filter({ hasText: '画质&风格&光影规范' }).getByRole('textbox')).toHaveValue(/商业广告画质/)
  await expect(page.locator('.viral-dimension-card').filter({ hasText: '环境氛围' }).getByRole('textbox')).toHaveValue(/问题压力/)
  await expect(page.locator('.viral-dimension-card').filter({ hasText: '镜头画面内容' }).getByRole('textbox')).toHaveValue(/按源视频镜头功能复刻/)
  await expect(page.locator('.viral-dimension-card').filter({ hasText: '音乐&音效' }).getByRole('textbox')).toHaveValue(/短视频平台高留存节奏/)
  await expect(page.getByLabel('复刻视频总提示词')).toHaveValue(/【音乐&音效】/)
  await expect(page.getByLabel('复刻视频总提示词')).toHaveValue(/文本指令优先约束内容改写：更年轻化/)
  await expect(page.getByLabel('复刻视频总提示词')).toHaveValue(/参考图片用于约束主体外观.*e2e-reference-product\.png/)

  await page.getByRole('button', { name: '生成复刻视频' }).click()
  await expect(page.getByText(/复刻视频生成任务已创建|复刻视频生成完成/)).toBeVisible()
  await expect.poll(async () => {
    const jobsResponse = await page.request.get(`${apiBaseURL}/api/generation-jobs?projectId=${projectId}`)
    const jobs = await jobsResponse.json() as Array<{ id: string, artifactKind: string, status: string }>
    const job = jobs.filter(item => item.artifactKind === 'video').at(-1)
    if (!job) return ''
    const syncedResponse = await page.request.get(`${apiBaseURL}/api/generation-jobs/${job.id}?projectId=${projectId}`)
    const synced = await syncedResponse.json() as { status: string }
    return synced.status
  }, { timeout: 10_000 }).toBe('succeeded')
})

test('前贴分镜成功后出现持久化资产，并在刷新后按当前前贴类型恢复', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 前贴成功恢复')
  await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
  await setProviderMode(page, 'success')

  const generate = page.getByRole('button', { name: '生成前贴分镜' })
  const addToLibrary = page.getByRole('button', { name: '加入混剪素材箱' })
  await expect(generate).toBeDisabled()
  await expect(addToLibrary).toBeDisabled()
  await selectShortDramaCandidate(page)
  await expect(generate).toBeEnabled()
  await generate.click()
  await syncPrerollJob(page, projectId, 'short_drama', 'succeeded')
  await page.reload()
  await expect(addToLibrary).toBeEnabled()
  await expect(page.getByLabel('正片首句（可选）')).toHaveValue('')

  await expect.poll(async () => {
    const response = await page.request.get(`${apiBaseURL}/api/artifacts?projectId=${projectId}&purpose=preroll&prerollType=short_drama`)
    const items = await response.json() as Array<{ status: string, sourceJobId?: string }>
    return items.some(item => item.status === 'ready' && Boolean(item.sourceJobId))
  }).toBe(true)
  await page.reload()

  await expect(addToLibrary).toBeEnabled()
  await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('素材剪辑')}`)
  await expect(page.getByText('短剧前贴视频').first()).toBeVisible()
  await expect(page.getByText('当前 Project 暂无可用于混剪的已持久化视频资产。')).toHaveCount(0)
})

test('短剧规划和生成拒绝跨 Project Brief，游戏与电商前贴保持原有媒体链路', async ({ page }) => {
  const shortDramaProjectId = await createProjectWithBrief(page, 'E2E 短剧跨项目拒绝')
  const otherProjectId = await createProjectWithBrief(page, 'E2E 其他 Brief 项目')
  const shortDramaBriefId = await getReadyBriefId(page, shortDramaProjectId)
  const otherBriefId = await getReadyBriefId(page, otherProjectId)
  const storyContext = {
    title: '雨夜归来的继承人',
    synopsis: '被逐出家门的女主在雨夜带着证据归来，发现继母正在转移家产，并必须在家族晚宴前揭开真相。',
    reviewedSellingPoints: ['豪门继承权争夺'],
    openingLine: '你以为我今晚回来，是为了求你吗？',
  }

  const rejectedPlan = await page.request.post(`${apiBaseURL}/api/short-drama-preroll-plans`, {
    data: { projectId: shortDramaProjectId, briefId: otherBriefId, storyContext },
  })
  expect(rejectedPlan.status()).toBe(400)
  expect((await rejectedPlan.json()).error.code).toBe('BRIEF_NOT_CONFIRMED')

  const planResponse = await page.request.post(`${apiBaseURL}/api/short-drama-preroll-plans`, {
    data: { projectId: shortDramaProjectId, briefId: shortDramaBriefId, storyContext },
  })
  expect(planResponse.ok()).toBeTruthy()
  const plan = await planResponse.json() as { version: string, candidates: Array<{ id: string }> }
  expect(plan.candidates.length).toBeGreaterThanOrEqual(3)
  expect(plan.candidates.length).toBeLessThanOrEqual(5)

  const rejectedGeneration = await page.request.post(`${apiBaseURL}/api/generation/media`, {
    data: {
      projectId: shortDramaProjectId,
      kind: 'video',
      purpose: 'preroll',
      prerollType: 'short_drama',
      briefId: otherBriefId,
      shortDramaPlanVersion: plan.version,
      shortDramaCandidateId: plan.candidates[0].id,
      storyContext,
    },
  })
  expect(rejectedGeneration.status()).toBe(400)
  expect((await rejectedGeneration.json()).error.code).toBe('BRIEF_NOT_CONFIRMED')

  await setProviderMode(page, 'success')
  const gameJob = await page.request.post(`${apiBaseURL}/api/generation/media`, {
    data: {
      projectId: shortDramaProjectId,
      kind: 'video',
      purpose: 'preroll',
      prerollType: 'game',
      prompt: '游戏前贴回归：展示目标、失败反馈和成功过关。',
      briefId: shortDramaBriefId,
    },
  })
  const commerceJob = await page.request.post(`${apiBaseURL}/api/generation/media`, {
    data: {
      projectId: shortDramaProjectId,
      kind: 'video',
      purpose: 'preroll',
      prerollType: 'commerce',
      prompt: '电商前贴回归：商品保真、动作变化和稳定定格。',
      briefId: shortDramaBriefId,
    },
  })
  expect(gameJob.status()).toBe(202)
  expect(commerceJob.status()).toBe(202)
  await syncPrerollJob(page, shortDramaProjectId, 'game', 'succeeded')
  await syncPrerollJob(page, shortDramaProjectId, 'commerce', 'succeeded')

  const [gameArtifacts, commerceArtifacts] = await Promise.all([
    page.request.get(`${apiBaseURL}/api/artifacts?projectId=${shortDramaProjectId}&purpose=preroll&prerollType=game`),
    page.request.get(`${apiBaseURL}/api/artifacts?projectId=${shortDramaProjectId}&purpose=preroll&prerollType=commerce`),
  ])
  expect(await gameArtifacts.json()).toEqual(expect.arrayContaining([
    expect.objectContaining({ status: 'ready', prerollType: 'game' }),
  ]))
  expect(await commerceArtifacts.json()).toEqual(expect.arrayContaining([
    expect.objectContaining({ status: 'ready', prerollType: 'commerce' }),
  ]))
})

test('前贴任务按项目和类型隔离，重试或失败不会保留旧成功可用态', async ({ page }) => {
  const firstProjectId = await openProject(page, 'E2E 前贴重试失败')
  await page.goto(`/projects/${firstProjectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
  await setProviderMode(page, 'success')
  await selectShortDramaCandidate(page)
  await page.getByRole('button', { name: '生成前贴分镜' }).click()
  await syncPrerollJob(page, firstProjectId, 'short_drama', 'succeeded')
  await page.reload()
  await expect(page.getByRole('button', { name: '加入混剪素材箱' })).toBeEnabled()

  await setProviderMode(page, 'task_failure')
  await page.getByRole('button', { name: '重新生成前贴' }).click()
  await expect(page.getByRole('button', { name: '加入混剪素材箱' })).toBeDisabled()
  await syncPrerollJob(page, firstProjectId, 'short_drama', 'failed')
  await page.reload()
  await expect(page.getByRole('button', { name: '加入混剪素材箱' })).toBeDisabled()

  const secondProjectId = await createProjectWithBrief(page, 'E2E 前贴类型隔离')

  await setProviderMode(page, 'success')
  await page.goto(`/projects/${secondProjectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
  await page.getByRole('tab', { name: /游戏前贴/ }).click()
  await page.getByRole('button', { name: '生成前贴分镜' }).click()
  await syncPrerollJob(page, secondProjectId, 'game', 'succeeded')
  await page.reload()
  await page.getByRole('tab', { name: /游戏前贴/ }).click()
  await expect(page.getByRole('button', { name: '加入混剪素材箱' })).toBeEnabled()

  const [firstGameJobs, secondShortJobs, secondGameArtifacts] = await Promise.all([
    page.request.get(`${apiBaseURL}/api/generation-jobs?projectId=${firstProjectId}&purpose=preroll&prerollType=game`),
    page.request.get(`${apiBaseURL}/api/generation-jobs?projectId=${secondProjectId}&purpose=preroll&prerollType=short_drama`),
    page.request.get(`${apiBaseURL}/api/artifacts?projectId=${secondProjectId}&purpose=preroll&prerollType=game`),
  ])
  expect(await firstGameJobs.json()).toEqual([])
  expect(await secondShortJobs.json()).toEqual([])
  expect(await secondGameArtifacts.json()).toEqual(expect.arrayContaining([
    expect.objectContaining({ status: 'ready', prerollType: 'game' }),
  ]))
})

test('前贴创建被 Provider 拒绝时展示可重试错误，且不伪造资产', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 前贴创建拒绝')
  await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
  await page.getByRole('tab', { name: /游戏前贴/ }).click()
  await setProviderMode(page, 'create_failure')

  await page.getByRole('button', { name: '生成前贴分镜' }).click()
  await expect(page.getByRole('status')).toContainText('Model provider could not complete the request')
  await expect(page.getByRole('button', { name: '加入混剪素材箱' })).toBeDisabled()
  await setProviderMode(page, 'success')
})

test('主创意聚合和 ChangeSet 不接受前贴资产', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 主创意边界')
  const artifactsResponse = await page.request.get(`${apiBaseURL}/api/artifacts?projectId=${projectId}`)
  const artifacts = await artifactsResponse.json() as Array<{ id: string, kind: string }>
  const briefId = artifacts.find(artifact => artifact.kind === 'brief')?.id
  expect(briefId).toBeTruthy()
  const mainCreative = await page.request.post(`${apiBaseURL}/api/artifacts`, {
    data: { projectId, kind: 'image', status: 'ready', content: '当前项目主创意' },
  })
  const preroll = await page.request.post(`${apiBaseURL}/api/artifacts`, {
    data: {
      projectId,
      kind: 'video',
      purpose: 'preroll',
      prerollType: 'short_drama',
      status: 'ready',
      content: '不得作为主创意的前贴',
    },
  })
  expect(mainCreative.ok()).toBeTruthy()
  expect(preroll.ok()).toBeTruthy()
  const mainArtifact = await mainCreative.json() as { id: string }
  const prerollArtifact = await preroll.json() as { id: string }

  await page.goto(`/projects/${projectId}/delivery/plans`)
  await expect(page.getByRole('button', { name: '素材组合' })).toBeVisible()
  await expect(page.getByText('不得作为主创意的前贴')).toHaveCount(0)

  const rejected = await page.request.post(`${apiBaseURL}/api/change-sets`, {
    data: { projectId, name: '拒绝前贴', artifactIds: [briefId, prerollArtifact.id], budgetLimit: 100 },
  })
  expect(rejected.status()).toBe(400)
  const accepted = await page.request.post(`${apiBaseURL}/api/change-sets`, {
    data: { projectId, name: '接受主创意', artifactIds: [briefId, mainArtifact.id], budgetLimit: 100 },
  })
  expect(accepted.ok()).toBeTruthy()
})

test('素材体验页只展示本用例当前 Project 的持久化 Artifact', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 资产体验')
  const currentAsset = await page.request.post(`${apiBaseURL}/api/artifacts`, {
    data: { projectId, kind: 'image', status: 'ready', content: '本项目持久化主创意' },
  })
  expect(currentAsset.ok()).toBeTruthy()
  const otherProjectId = await createProjectWithBrief(page, 'E2E 其他资产项目')
  const otherAsset = await page.request.post(`${apiBaseURL}/api/artifacts`, {
    data: { projectId: otherProjectId, kind: 'video', status: 'ready', content: '其他项目不可见资产' },
  })
  expect(otherAsset.ok()).toBeTruthy()

  await page.goto(`/projects/${projectId}/insight/assets`)
  await expect(page.getByRole('heading', { name: '本项目持久化主创意' })).toBeVisible()
  await expect(page.getByText('其他项目不可见资产')).toHaveCount(0)
})

test('内容分析页展示 data 目录导入的公开短视频洞察样本', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 公开洞察样本')

  await page.goto(`/projects/${projectId}/insight/content`)
  await expect(page.getByText('公开短视频洞察样本')).toBeVisible()
  await expect(page.getByText('6 条样本 · 1 个文件')).toBeVisible()
  await expect(page.getByText('部署后自动导入 data 目录作为示例展示。')).toBeVisible()

  await page.getByLabel('公开短视频洞察行业').selectOption('美妆护肤')
  await expect(page.getByRole('button', { name: /美妆新品用半脸对比展示上妆速度/ })).toBeVisible()
  await expect(page.getByRole('button', { name: /insight-006/ })).toContainText('美妆护肤')
  await expect(page.getByText('当前样本拆解')).toBeVisible()
  await expect(page.getByText('早八通勤只想快但妆面不能糊')).toBeVisible()

  await page.getByLabel('公开短视频洞察行业').selectOption('')
  await page.getByLabel('搜索公开短视频洞察').fill('护眼学习灯')
  await expect(page.getByRole('button', { name: /护眼学习灯用场景证明减少家长焦虑/ })).toBeVisible()
  await expect(page.getByText('别只看亮不亮关键是孩子愿不愿意久坐')).toBeVisible()
})

test('前贴任务取消后刷新会恢复服务端取消态，且不暴露旧预览或素材箱入口', async ({ page }) => {
  const projectId = await openProject(page, 'E2E 前贴取消恢复')
  await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
  await setProviderMode(page, 'success')

  await selectShortDramaCandidate(page)
  await page.getByRole('button', { name: '生成前贴分镜' }).click()
  const cancel = page.getByRole('button', { name: '取消生成' })
  await expect(cancel).toBeVisible()
  await cancel.click()
  await expect(page.getByRole('status')).toContainText('前贴分镜任务已取消')
  await page.reload()

  const query = `projectId=${projectId}&purpose=preroll&prerollType=short_drama`
  await expect.poll(async () => {
    const response = await page.request.get(`${apiBaseURL}/api/generation-jobs?${query}`)
    const jobs = await response.json() as Array<{ status: string }>
    return jobs.at(-1)?.status
  }).toBe('cancelled')
  await expect(page.getByText('已取消').first()).toBeVisible()
  await expect(page.getByRole('button', { name: '播放短剧前贴预览' })).toBeDisabled()
  await expect(page.getByRole('button', { name: '加入混剪素材箱' })).toBeDisabled()
  await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('素材剪辑')}`)
  await expect(page.getByText('当前 Project 暂无可用于混剪的已持久化视频资产。')).toBeVisible()
})

for (const width of [1280, 1440, 1680]) {
  test(`桌面 ${width}px 核心页面主任务可见且无横向溢出`, async ({ page }) => {
    await page.setViewportSize({ width, height: 960 })
    const projectId = await openProject(page, `E2E Task28 核心页面 ${width}`)

    await page.goto('/')
    await expect(page.getByRole('heading', { name: '代理商客户组合工作台' })).toBeVisible()
    await expectInitialViewportControl(page, page.getByRole('button', { name: /进入创意队列/ }), width)
    await expectNoHorizontalOverflow(page)

    const routes: Array<{
      path: string
      heading: string | RegExp
      headingLevel?: number
      primary: () => ReturnType<Page['locator']>
      elementOnly?: boolean
    }> = [
      {
        path: `/projects/${projectId}/home`,
        heading: `E2E Task28 核心页面 ${width}`,
        headingLevel: 1,
        primary: () => page.getByRole('button', { name: /进入需求与策略/ }),
      },
      {
        path: `/projects/${projectId}/strategy/tasks`,
        heading: '策略任务',
        headingLevel: 1,
        primary: () => page.getByRole('button', { name: '新建策略任务' }),
      },
      {
        path: `/projects/${projectId}/creative/tasks`,
        heading: '创意任务',
        headingLevel: 1,
        primary: () => page.getByRole('button', { name: '新建创意任务' }),
      },
      {
        path: `/projects/${projectId}/insight/assets`,
        heading: '当前 Project 素材',
        headingLevel: 2,
        primary: () => page.getByLabel('搜索素材经验'),
      },
      {
        path: `/projects/${projectId}/insight/performance`,
        heading: '投后结论',
        headingLevel: 2,
        primary: () => page.getByRole('button', { name: '重新拉取' }),
      },
      {
        path: `/projects/${projectId}/delivery/evidence`,
        heading: '证据与审计',
        headingLevel: 1,
        primary: () => page.getByRole('heading', { name: '服务端审计轨迹' }),
        elementOnly: true,
      },
      {
        path: `/projects/${projectId}/manage`,
        heading: `E2E Task28 核心页面 ${width}`,
        headingLevel: 1,
        primary: () => page.getByRole('button', { name: /进入项目工作台/ }),
      },
    ]

    for (const route of routes) {
      await page.goto(route.path)
      await expect(page.getByRole('heading', { name: route.heading, level: route.headingLevel })).toBeVisible()
      if (route.elementOnly) {
        await expectInitialViewportElement(page, route.primary(), width)
      } else {
        await expectInitialViewportControl(page, route.primary(), width)
      }
      await expectNoHorizontalOverflow(page)
    }
  })

  test(`桌面 ${width}px 初始视口中前贴工作区和素材剪辑控件可操作且无横向溢出`, async ({ page }) => {
    await page.setViewportSize({ width, height: 960 })
    const projectId = await openProject(page, `E2E 初始桌面视口 ${width}`)
    await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('效果广告')}`)
    await page.getByRole('tab', { name: /游戏前贴/ }).click()

    const workspace = page.locator('.preroll-workspace')
    const generate = page.getByRole('button', { name: '生成前贴分镜' })
    const addToLibrary = page.getByRole('button', { name: '加入混剪素材箱' })
    await expect(workspace).toBeVisible()
    await expectInitialViewportControl(page, generate, width)
    await expect(addToLibrary).toBeVisible()
    await expect(page.getByRole('button', { name: '播放游戏前贴预览' })).toBeVisible()
    await expectNoHorizontalOverflow(page)

    await setProviderMode(page, 'success')
    await generate.click()
    const cancel = page.getByRole('button', { name: '取消生成' })
    await expectInitialViewportControl(page, cancel, width)
    await cancel.click()

    await page.getByRole('button', { name: '生成前贴分镜' }).click()
    await syncPrerollJob(page, projectId, 'game', 'succeeded')
    await page.reload()
    await page.getByRole('tab', { name: /游戏前贴/ }).click()
    await expectInitialViewportControl(page, addToLibrary, width)
    await expectNoHorizontalOverflow(page)

    const assetResponse = await page.request.post(`${apiBaseURL}/api/artifacts`, {
      data: {
        projectId,
        kind: 'video',
        purpose: 'preroll',
        prerollType: 'game',
        status: 'ready',
        content: '素材剪辑初始视口资产',
      },
    })
    expect(assetResponse.ok()).toBeTruthy()
    await page.goto(`/projects/${projectId}/creative/video?view=${encodeURIComponent('素材剪辑')}`)

    const assetBin = page.locator('.editing-assets')
    const sourceAsset = assetBin.getByRole('button').first()
    const addToTimeline = page.getByRole('button', { name: '加入混剪时间线' })
    await expect(assetBin).toBeVisible()
    await expectInitialViewportControl(page, sourceAsset, width)
    await expectNoHorizontalOverflow(page)

    await sourceAsset.click()
    await expectInitialViewportControl(page, addToTimeline, width)
    await expectInitialViewportControl(page, page.getByRole('button', { name: '生成混剪版本' }), width)
    await expectInitialViewportControl(page, page.getByRole('button', { name: '保存为 EditTask' }), width)
    await expectNoHorizontalOverflow(page)
  })
}
