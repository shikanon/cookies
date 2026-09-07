import { expect, test, type Page } from '@playwright/test'

import { runtimeAccountId, seedRuntimeAccount } from './delivery-runtime-fixture'

const primaryProjectId = 'project_investor_precision_evidence'
const otherProjectId = 'project_local'

test('DeliveryPlan editor creates immutable v2 intent/configuration history with authoritative preflight', async ({ page, request }) => {
  seedRuntimeAccount(primaryProjectId)
  const suffix = Date.now().toString(36)
  const planName = `E2E 平台配置计划 ${suffix}`
  await page.goto(`/projects/${primaryProjectId}/delivery/plans`)
  await expect(page.getByRole('heading', { name: '计划草稿' })).toBeVisible()
  await startNewPlan(page, planName)
  await page.getByRole('button', { name: '预算与排期', exact: true }).click()
  await page.getByRole('spinbutton', { name: /日预算|总预算/ }).fill('3000')
  await page.getByRole('button', { name: '素材引用', exact: true }).click()
  await expect(page.getByRole('group', { name: '已确认素材多选' })).toBeVisible()

  const createPromise = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === `/api/delivery/v1/projects/${primaryProjectId}/plans`)
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const createResponse = await createPromise
  expect(createResponse.status()).toBe(201)
  const created = await createResponse.json() as any
  expect(created).toMatchObject({ source: 'mock', scenario: 'platform_configuration', current_version_number: 1 })
  expect(created.current_version).toMatchObject({
    schema_version: 'delivery-plan-version/v2', runtime_status: 'active',
    intent: { schema_version: 'delivery-intent/v1', payload: { marketing_objective: '获取高质量销售线索并验证投前门禁' } },
    platform_configuration: { schema_version: 'delivery-platform-configuration/v2', platform: 'ocean_engine' },
  })
  expect(created.current_version.canonical_hash).toBe(created.current_version.platform_configuration.canonical_hash)
  const planId = created.id

  await page.goto(`/projects/${primaryProjectId}/delivery/plans?plan_id=${planId}`)
  await expect(page.getByRole('heading', { name: planName })).toBeVisible()
  await page.getByRole('button', { name: '预算与排期', exact: true }).click()
  await page.getByRole('spinbutton', { name: /日预算|总预算/ }).fill('4200')
  const updatePromise = page.waitForResponse(response => response.request().method() === 'PATCH' && new URL(response.url()).pathname === `/api/delivery/v1/projects/${primaryProjectId}/plans/${planId}`)
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const updateResponse = await updatePromise
  expect(updateResponse.status()).toBe(200)
  const updated = await updateResponse.json() as any
  expect(updated.current_version_number).toBe(2)
  expect(updated.versions).toHaveLength(2)
  expect(updated.versions[0].intent.payload.budget_boundary.maximum_total_minor).toBe(300000)
  expect(updated.versions[1].intent.payload.budget_boundary.maximum_total_minor).toBe(420000)
  expect(updated.versions[0].canonical_hash).not.toBe(updated.versions[1].canonical_hash)
  await expect(page.getByRole('button', { name: '查看版本 V1' })).toBeVisible()
  await expect(page.getByRole('button', { name: '查看版本 V2' })).toBeVisible()

  const preflightResponse = await request.post(`/api/delivery/v1/projects/${primaryProjectId}/plans/${planId}/preflight`)
  expect(preflightResponse.status()).toBe(200)
  expect(await preflightResponse.json()).toMatchObject({
    passed: false,
    blocked: true,
    checks: expect.arrayContaining([
      expect.objectContaining({ code: 'delivery_intent_valid', passed: true }),
      expect.objectContaining({ code: 'platform_configuration_valid', passed: true }),
      expect.objectContaining({ code: 'marketing_product_outside_intent', passed: false }),
    ]),
  })

  const crossProject = await request.get(`/api/delivery/v1/projects/${otherProjectId}/plans/${planId}`)
  expect(crossProject.status()).toBe(404)
  await page.goto(`/projects/${otherProjectId}/delivery/plans`)
  await expect(page.getByText(planName)).toHaveCount(0)
})

async function startNewPlan(page: Page, name: string) {
  await page.getByRole('button', { name: '新建投放计划' }).click()
  await page.getByLabel('计划名称').fill(name)
  await page.getByLabel('业务目标').fill('获取高质量销售线索并验证投前门禁')
  await page.getByLabel('账户边界').selectOption(runtimeAccountId(primaryProjectId))
  await page.getByLabel('策略来源').selectOption('task_demo_precision_strategy')
  await page.getByLabel('巨量营销目的', { exact: true }).selectOption('lead_generation')
  await page.getByLabel('cookies 产品', { exact: true }).selectOption({ index: 1 })
  await page.getByRole('button', { name: '投放载体和监测', exact: true }).click()
  await page.getByLabel('投放载体', { exact: true }).selectOption('owned_landing_page')
  await page.getByLabel('自研落地页链接', { exact: true }).fill('https://example.test/e2e')
  await page.getByLabel('优化目标', { exact: true }).selectOption('click')
}
