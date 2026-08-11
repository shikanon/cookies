import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const primaryProjectId = 'project_investor_precision_evidence'
const otherProjectId = 'project_local'

test('A03 审批绑定 Plan/ChangeSet/hash，计划变化后旧审批保留但失效', async ({ page, request }) => {
  const suffix = Date.now().toString(36)
  const planName = `A03 黄金计划 ${suffix}`

  await page.goto(`/projects/${primaryProjectId}/delivery/plans`)
  await expect(page.getByRole('tablist', { name: '投放计划视图' })).toHaveCount(0)
  await page.getByRole('button', { name: '新建投放计划' }).click()
  await page.getByLabel('计划名称').fill(planName)
  await page.getByLabel('业务目标').fill('验证内容哈希绑定、审批有效期和执行范围')
  await page.getByLabel('账户边界').selectOption('mock-advertiser-001')
  await page.getByLabel('策略来源').selectOption('task_demo_precision_strategy')
  await page.getByRole('button', { name: '预算与排期', exact: true }).click()
  await page.getByLabel('总预算').fill('3000')
  await page.getByRole('button', { name: '素材引用', exact: true }).click()
  await page.getByLabel('已确认素材').selectOption('asset_demo_investor_creative_video')

  const createPlanPromise = page.waitForResponse(response =>
    response.request().method() === 'POST' &&
    new URL(response.url()).pathname === `/api/delivery/v1/projects/${primaryProjectId}/plans`,
  )
  await page.getByRole('button', { name: '保存草稿', exact: true }).click()
  const createPlanResponse = await createPlanPromise
  expect(createPlanResponse.status()).toBe(201)
  const planV1 = await createPlanResponse.json() as {
    id: string
    current_version_number: number
    current_version: { canonical_hash: string }
  }
  expect(planV1.current_version_number).toBe(1)
  expect(planV1.current_version.canonical_hash).toMatch(/^[0-9a-f]{64}$/)

  await createAndPreflightChangeSet(page, planV1.id)
  const v1ChangeSet = await latestChangeSetForPlan(request, planV1.id)
  expect(v1ChangeSet).toMatchObject({
    plan_name: planName,
    plan_version: 1,
    status: 'preflight_passed',
    plan_canonical_hash: planV1.current_version.canonical_hash,
    source: 'mock',
    scenario: 'platform_configuration',
  })

  await page.goto(`/projects/${primaryProjectId}/delivery/approvals/${v1ChangeSet.id}`)
  await expect(page.getByRole('tablist', { name: '审批中心视图' })).toHaveCount(0)
  await expect(page.getByText('服务端对象详情', { exact: true })).toHaveCount(0)
  await expect(page.locator('.approval-queue .surface-toolbar .section-label')).toHaveCount(0)
  await expect(page.locator('.approval-queue')).toHaveCSS('overflow-y', 'auto')
  expect(await page.locator('.approval-queue').evaluate(element => element.clientHeight)).toBeLessThanOrEqual(720)
  await expect(page.getByRole('heading', { name: planName, exact: true })).toBeVisible()
  await expect(page.getByText('Plan V1 · 批准内容由计划快照', { exact: false })).toBeVisible()
  await expect(page.getByText(planV1.current_version.canonical_hash.slice(0, 12), { exact: true })).toBeVisible()
  await expect(page.getByText('首次上线操作演练', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('批准后 24 小时', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: '批准当前快照，或说明原因后打回' })).toBeVisible()
  const rejectionReason = page.getByLabel('打回修改说明 打回时必填')
  await expect(rejectionReason).toHaveCSS('min-height', '96px')
  await expect(page.getByRole('button', { name: '打回修改' })).toBeDisabled()

  const approveV1Promise = page.waitForResponse(response =>
    response.request().method() === 'POST' &&
    new URL(response.url()).pathname.endsWith(`/change-sets/${v1ChangeSet.id}:approve`),
  )
  await page.getByRole('button', { name: '批准平台操作演练' }).click()
  const approveV1Response = await approveV1Promise
  expect(approveV1Response.status()).toBe(200)
  const approvedV1 = await approveV1Response.json() as ChangeSetWire
  expect(approvedV1.approval).toMatchObject({
    valid: true,
    plan_version: 1,
    change_set_version: approvedV1.version,
    plan_canonical_hash: planV1.current_version.canonical_hash,
    action: 'execute',
    scope: 'execute_mock',
    budget_limit_minor: 300000,
    currency: 'CNY',
    source: 'mock',
    scenario: 'platform_configuration',
  })
  expect(new Date(approvedV1.approval!.expires_at).getTime() - new Date(approvedV1.approval!.approved_at).getTime()).toBe(24 * 60 * 60 * 1000)
  await expect(page.locator('.gate-row.passed')).toHaveCount(3)
  await expect(page.locator('.gate-row.failed')).toHaveCount(0)
  await expect(page.locator('.gate-row.passed > svg').first()).toHaveCSS('color', 'oklch(0.57 0.14 155)')
  await expect(page.getByRole('button', { name: '启动平台操作演练' })).toBeEnabled()

  await page.goto(`/projects/${primaryProjectId}/delivery/plans`)
  await page.getByRole('button').filter({ hasText: planName }).first().click()
  await page.getByRole('button', { name: '预算与排期', exact: true }).click()
  await page.getByLabel('总预算').fill('4200')
  const updatePlanPromise = page.waitForResponse(response =>
    response.request().method() === 'PATCH' &&
    new URL(response.url()).pathname.endsWith(`/plans/${planV1.id}`),
  )
  await page.getByRole('button', { name: '保存新版本', exact: true }).click()
  const updatePlanResponse = await updatePlanPromise
  expect(updatePlanResponse.status()).toBe(200)
  const planV2 = await updatePlanResponse.json() as {
    current_version_number: number
    current_version: { canonical_hash: string }
  }
  expect(planV2.current_version_number).toBe(2)
  expect(planV2.current_version.canonical_hash).not.toBe(planV1.current_version.canonical_hash)

  await page.goto(`/projects/${primaryProjectId}/delivery/approvals`)
  await expect(page.getByRole('tablist', { name: '审批中心视图' })).toHaveCount(0)
  await page.getByRole('button', { name: new RegExp(`^${v1ChangeSet.id} `) }).click()
  await expect(page.getByText('旧审批已失效', { exact: true })).toBeVisible()
  await expect(page.getByText('计划已产生新版本，旧版本审批永久失效。', { exact: true })).toBeVisible()
  const invalidNotice = page.locator('.approval-invalid-notice')
  await expect(invalidNotice).toHaveCSS('display', 'flex')
  const invalidIconBox = await invalidNotice.locator('svg').boundingBox()
  const invalidCopyBox = await invalidNotice.locator('span').boundingBox()
  expect(invalidIconBox).not.toBeNull()
  expect(invalidCopyBox).not.toBeNull()
  expect(invalidIconBox!.x + invalidIconBox!.width).toBeLessThanOrEqual(invalidCopyBox!.x)
  await expect(page.locator('.approval-audit-heading')).toHaveCSS('display', 'flex')
  await expect(page.getByText(/请求体只接受 expected_version/)).toHaveCount(0)
  await expect(page.locator('.gate-row.passed')).toHaveCount(2)
  await expect(page.locator('.gate-row.failed')).toHaveCount(1)
  await expect(page.locator('.gate-row.passed > svg').first()).toHaveCSS('color', 'oklch(0.57 0.14 155)')
  await expect(page.locator('.gate-row.failed > svg')).toHaveCSS('color', 'oklch(0.55 0.205 25)')
  await expect(page.getByRole('button', { name: '启动平台操作演练' })).toBeDisabled()

  const staleExecute = await request.post(
    `/api/delivery/v1/projects/${primaryProjectId}/change-sets/${v1ChangeSet.id}:execute`,
    {
      headers: { 'Idempotency-Key': `a03-stale-${suffix}` },
      data: { expected_version: approvedV1.version, scenario: 'success' },
    },
  )
  expect(staleExecute.status()).toBe(409)
  expect(await staleExecute.json()).toMatchObject({
    error: { code: 'STALE_PLAN_VERSION' },
    source: 'mock',
    scenario: 'stale_plan_version',
  })
  const oldAudit = await request.get(`/api/delivery/v1/projects/${primaryProjectId}/change-sets/${v1ChangeSet.id}`)
  expect(oldAudit.status()).toBe(200)
  expect(await oldAudit.json()).toMatchObject({
    approval: {
      approval_id: approvedV1.approval!.approval_id,
      valid: false,
      invalid_reason: 'STALE_PLAN_VERSION',
    },
  })

  await page.goto(`/projects/${primaryProjectId}/delivery/plans`)
  await page.getByRole('button').filter({ hasText: planName }).first().click()
  await createAndPreflightChangeSet(page, planV1.id)
  const v2ChangeSet = await latestChangeSetForPlan(request, planV1.id)
  expect(v2ChangeSet.plan_version).toBe(2)
  expect(v2ChangeSet.id).not.toBe(v1ChangeSet.id)

  await page.goto(`/projects/${primaryProjectId}/delivery/approvals`)
  await page.getByRole('button', { name: new RegExp(`^${v2ChangeSet.id} `) }).click()
  await expect(page.getByRole('heading', { name: planName, exact: true })).toBeVisible()
  await expect(page.getByText('Plan V2 · 批准内容由计划快照', { exact: false })).toBeVisible()
  const approveV2Promise = page.waitForResponse(response =>
    response.request().method() === 'POST' &&
    new URL(response.url()).pathname.endsWith(`/change-sets/${v2ChangeSet.id}:approve`),
  )
  await page.getByRole('button', { name: '批准平台操作演练' }).click()
  const approvedV2Response = await approveV2Promise
  expect(approvedV2Response.status()).toBe(200)
  const approvedV2 = await approvedV2Response.json() as ChangeSetWire
  expect(approvedV2).toMatchObject({
    approval: {
      valid: true,
      plan_version: 2,
      plan_canonical_hash: planV2.current_version.canonical_hash,
      source: 'mock',
      scenario: 'platform_configuration',
    },
  })
  await expect(page.getByRole('button', { name: '启动平台操作演练' })).toBeEnabled()

  const executeV2Promise = page.waitForResponse(response =>
    response.request().method() === 'POST' &&
    new URL(response.url()).pathname.endsWith(`/change-sets/${v2ChangeSet.id}:execute`),
  )
  await page.getByRole('button', { name: '启动平台操作演练' }).click()
  const executeV2Response = await executeV2Promise
  expect(executeV2Response.status()).toBe(201)
  expect(await executeV2Response.json()).toMatchObject({
    change_set: {
      id: v2ChangeSet.id,
      status: 'executed',
      version: approvedV2.version + 1,
      approval: {
        approval_id: approvedV2.approval!.approval_id,
        change_set_version: approvedV2.version,
        valid: true,
      },
    },
  })
  await expect(page.getByText('旧审批已失效', { exact: true })).toHaveCount(0)
  await expect(page.locator('.gate-row.passed')).toHaveCount(3)
  await expect(page.getByRole('button', { name: '启动平台操作演练' })).toBeDisabled()

  const executedAudit = await request.get(`/api/delivery/v1/projects/${primaryProjectId}/change-sets/${v2ChangeSet.id}`)
  expect(executedAudit.status()).toBe(200)
  expect(await executedAudit.json()).toMatchObject({
    status: 'executed',
    version: approvedV2.version + 1,
    approval: {
      approval_id: approvedV2.approval!.approval_id,
      change_set_version: approvedV2.version,
      valid: true,
    },
  })

  const crossProjectRead = await request.get(`/api/delivery/v1/projects/${otherProjectId}/change-sets/${v2ChangeSet.id}`)
  expect(crossProjectRead.status()).toBe(404)
  expect(await crossProjectRead.json()).toMatchObject({
    error: { code: 'RESOURCE_NOT_FOUND' },
    source: 'mock',
  })
})

async function createAndPreflightChangeSet(page: Page, planId: string) {
  const planPreflightPromise = page.waitForResponse(response =>
    response.request().method() === 'POST' &&
    new URL(response.url()).pathname.endsWith(`/plans/${planId}/preflight`),
  )
  await page.getByRole('button', { name: '检查当前草稿', exact: true }).click()
  expect((await planPreflightPromise).status()).toBe(200)

  const createChangeSetPromise = page.waitForResponse(response =>
    response.request().method() === 'POST' &&
    new URL(response.url()).pathname.endsWith(`/plans/${planId}:create-change-set`),
  )
  const preflightChangeSetPromise = page.waitForResponse(response =>
    response.request().method() === 'POST' &&
    /\/change-sets\/[^/]+:preflight$/.test(new URL(response.url()).pathname),
  )
  await page.getByRole('button', { name: '提交变更申请', exact: true }).click()
  expect((await createChangeSetPromise).status()).toBe(201)
  expect((await preflightChangeSetPromise).status()).toBe(200)
}

async function latestChangeSetForPlan(request: APIRequestContext, planId: string): Promise<ChangeSetWire> {
  const response = await request.get(`/api/delivery/v1/projects/${primaryProjectId}/change-sets`)
  expect(response.status()).toBe(200)
  const body = await response.json() as { items: ChangeSetWire[]; source: string; scenario: string }
  expect(body).toMatchObject({ source: 'mock', scenario: 'approval_queue' })
  const value = body.items.find(item => item.plan_id === planId)
  if (!value) throw new Error(`No ChangeSet found for ${planId}`)
  return value
}

type ChangeSetWire = {
  id: string
  plan_id: string
  plan_name: string
  plan_version: number
  plan_canonical_hash: string
  status: string
  version: number
  source: string
  scenario: string
  approval?: {
    approval_id: string
    valid: boolean
    plan_version: number
    change_set_version: number
    plan_canonical_hash: string
    action: string
    scope: string
    budget_limit_minor: number
    currency: string
    approved_at: string
    expires_at: string
    source: string
    scenario: string
  }
}
