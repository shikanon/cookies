import { expect, test, type Page, type Route } from '@playwright/test'

const apiBaseURL = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8787'
let projectListDelayMs = 0

test.beforeEach(async ({ page }) => {
  projectListDelayMs = 0
  const response = await page.request.post(`${apiBaseURL}/api/session`, {
    data: { email: 'demo@cookies.local', password: 'cookies-demo' },
  })
  expect(response.ok()).toBeTruthy()
})

const agencyProjects = [
  {
    id: 'project-nova-home-launch',
    name: 'Nova Home 夏季清洁增长',
    brand: 'Nova Home',
    objective: '验证家庭清洁痛点素材，提升搜索与信息流线索效率。',
    runtime: {
      code: 'NOVA-HOME-2607',
      product: '全屋扫拖机器人 S8',
      stage: '素材人工确认',
      progress: 64,
      status: 'active',
      owner: 'Lin Wei',
      budget: 260000,
      currency: 'CNY',
      timezone: 'Asia/Shanghai',
    },
    version: 3,
    createdAt: '2026-07-18T02:00:00.000Z',
    updatedAt: '2026-07-27T07:50:00.000Z',
  },
  {
    id: 'project-nova-kids-presale',
    name: 'Nova Kids 开学季预售',
    brand: 'Nova Kids',
    objective: '围绕护眼与陪伴场景准备预售素材和账户测试计划。',
    runtime: {
      code: 'NOVA-KIDS-2608',
      product: 'AI 学习灯 L2',
      stage: '创意制作',
      progress: 38,
      status: 'active',
      owner: 'Sofia Chen',
      budget: 180000,
      currency: 'CNY',
      timezone: 'Asia/Shanghai',
    },
    version: 2,
    createdAt: '2026-07-20T03:10:00.000Z',
    updatedAt: '2026-07-27T06:20:00.000Z',
  },
  {
    id: 'project-orbit-care-sleep',
    name: 'Orbit Care 睡眠健康线索',
    brand: 'Orbit Care',
    objective: '验证睡眠监测教育素材，建立可扩量的获客计划。',
    runtime: {
      code: 'ORBIT-SLEEP-2607',
      product: '睡眠监测贴片 Pro',
      stage: '投放预检',
      progress: 76,
      status: 'active',
      owner: 'Noah Xu',
      budget: 320000,
      currency: 'CNY',
      timezone: 'Asia/Shanghai',
    },
    version: 4,
    createdAt: '2026-07-12T01:30:00.000Z',
    updatedAt: '2026-07-27T07:10:00.000Z',
  },
]

const approvedNovaHomeChangeSet = {
  id: 'cs-e2e-unconfirmed-material',
  projectId: 'project-nova-home-launch',
  name: 'E2E 未确认素材阻断',
  status: 'approved',
  artifactIds: ['asset-nova-home-hook', 'asset-nova-home-proof'],
  budgetLimit: 260000,
  preflight: {
    passed: true,
    checkedAt: '2026-07-27T08:10:00.000Z',
    checks: [
      { code: 'confirmed_brief', passed: true, message: 'Brief 已确认', repair: '无需处理' },
      { code: 'ready_creative', passed: true, message: '创意素材已就绪', repair: '无需处理' },
      { code: 'budget_boundary', passed: true, message: '预算在护栏内', repair: '无需处理' },
    ],
  },
  version: 2,
  createdAt: '2026-07-27T08:00:00.000Z',
  updatedAt: '2026-07-27T08:10:00.000Z',
}

test.beforeEach(async ({ page }) => {
  await mockAgencyProjectApi(page)
})

test('Project 快速切换后只展示当前 Project 数据', async ({ page }) => {
  await page.goto('/projects/project-nova-home-launch/creative/tasks')
  await expect(page.locator('.statusbar')).toContainText('Project：Nova Home 夏季清洁增长')

  await chooseProject(page, 'Orbit Care 睡眠健康线索')
  await expect(page).toHaveURL(/\/projects\/project-orbit-care-sleep\/creative\/tasks/)
  await expect(page.locator('.statusbar')).toContainText('Project：Orbit Care 睡眠健康线索')
  await expect(page.locator('.statusbar')).not.toContainText('Nova Home 夏季清洁增长')

  await chooseProject(page, 'Nova Kids 开学季预售')
  await expect(page).toHaveURL(/\/projects\/project-nova-kids-presale\/creative\/tasks/)
  await expect(page.locator('.statusbar')).toContainText('Project：Nova Kids 开学季预售')
  await expect(page.locator('.statusbar')).not.toContainText('Orbit Care 睡眠健康线索')
})

test('Project 路由目标尚未落地时显示加载态而不是错误态', async ({ page }) => {
  projectListDelayMs = 1500

  const navigation = page.goto('/projects/project-orbit-care-sleep/creative/tasks')
  await expect(page.getByText('正在加载路由目标 Project')).toBeVisible()
  await navigation
  await expect(page.getByRole('heading', { name: '数据暂时无法读取' })).toHaveCount(0)
  await expect(page.locator('.statusbar')).toContainText('Project：Orbit Care 睡眠健康线索')
})

test('素材检查支持质检通过后的人工确认和新版本需要修改路径', async ({ page }) => {
  await page.goto('/projects/project-nova-home-launch/creative/reviews/asset-nova-home-hook@v3')

  await expect(page.getByRole('complementary', { name: '质检结果与人工确认' }).getByText('暂无大模型质检')).toBeVisible()
  await expect(page.getByRole('button', { name: '确认素材', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: '需要修改', exact: true })).toBeDisabled()

  await page.getByRole('button', { name: /运行大模型质检/ }).click()
  await expect(page.getByText('已完成 v3 大模型质检，可进入人工确认。')).toBeVisible()
  await expect(page.getByRole('button', { name: '确认素材', exact: true })).toBeEnabled()

  await page.getByRole('button', { name: '确认素材', exact: true }).click()
  await expect(page.getByText('已确认素材')).toBeVisible()
  await expect(page.getByText('humanConfirmedVersion')).toBeVisible()
  await expect(page.locator('.material-version-ledger')).toContainText('v3')

  await page.getByRole('button', { name: /生成新版本/ }).click()
  await expect(page).toHaveURL(/asset-nova-home-hook@v4/)
  await expect(page.getByText('已生成 v4，旧质检和确认记录保留，新版本回到待质检流程。')).toBeVisible()
  await expect(page.getByRole('button', { name: '确认素材', exact: true })).toBeDisabled()

  await page.getByRole('button', { name: /运行大模型质检/ }).click()
  await page.getByLabel('修改问题说明').fill('画面中的 CTA 与新版 Brief 不一致')
  await page.getByRole('button', { name: '需要修改', exact: true }).click()
  await expect(page.getByText('已将 v4 标记为需要修改，并返回制作环节。')).toBeVisible()
  await expect(page.locator('p').filter({ hasText: '画面中的 CTA 与新版 Brief 不一致' })).toBeVisible()
})

test('广告账户页使用专用账户绑定视图而不是通用业务记录表', async ({ page }) => {
  await page.goto('/projects/project-orbit-care-sleep/delivery/accounts?view=广告账户')

  await expect(page.getByLabel('搜索广告账户绑定')).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '账户名称与 ID' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '权限' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '登录' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '追踪' })).toBeVisible()
  await expect(page.getByText('Orbit Care 腾讯演示账户')).toBeVisible()
  await expect(page.getByLabel('复制账户 ID TX-DEMO-ORBIT-026')).toBeVisible()
  await expect(page.getByText('追踪已失效')).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '编号' })).toHaveCount(0)
  await expect(page.getByText('没有服务端记录')).toHaveCount(0)
  await expect(page.getByText(/Token|登录凭据/)).toHaveCount(0)
})

async function chooseProject(page: Page, projectName: string) {
  await page.getByRole('button', { name: /夏季清洁增长|睡眠健康线索|开学季预售/ }).first().click()
  await page.getByPlaceholder('搜索客户、品牌、Project、代码或负责人').fill(projectName)
  await page.getByRole('menuitem', { name: new RegExp(projectName) }).click()
}

async function mockAgencyProjectApi(page: Page) {
  await page.route(`${apiBaseURL}/api/**`, async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (url.pathname === '/api/projects' && request.method() === 'GET') {
      if (projectListDelayMs > 0) {
        const delay = projectListDelayMs
        projectListDelayMs = 0
        await new Promise(resolve => setTimeout(resolve, delay))
      }
      await fulfillJson(route, agencyProjects)
      return
    }
    if (url.pathname === '/api/provider/capabilities') {
      await fulfillJson(route, {
        provider: 'ark',
        status: 'configured',
        checkedAt: '2026-07-27T08:00:00.000Z',
        capabilities: [{ capability: 'image', model: 'e2e-model' }],
      })
      return
    }
    if (url.pathname === '/api/change-sets' && request.method() === 'GET') {
      const projectId = url.searchParams.get('projectId')
      await fulfillJson(route, projectId === 'project-nova-home-launch' ? [approvedNovaHomeChangeSet] : [])
      return
    }
    if (url.pathname === '/api/asset-features') {
      await fulfillJson(route, { items: [] })
      return
    }
    if (['/api/artifacts', '/api/generation-jobs', '/api/tasks', '/api/operations'].includes(url.pathname)
      || /^\/api\/projects\/[^/]+\/operations$/.test(url.pathname)) {
      await fulfillJson(route, [])
      return
    }
    await route.fallback()
  })
}

async function fulfillJson(route: Route, json: unknown) {
  await route.fulfill({
    status: 200,
    headers: {
      'Access-Control-Allow-Origin': 'http://127.0.0.1:4173',
      'Access-Control-Allow-Credentials': 'true',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(json),
  })
}
