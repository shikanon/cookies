import { expect, test, type Page } from '@playwright/test'

import { runtimeAccountId, seedRuntimeAccount } from './delivery-runtime-fixture'

const primaryProjectId = 'project_investor_precision_evidence'
const otherProjectId = 'project_local'

test('DeliveryPlan editor creates immutable v2 intent/configuration history with authoritative preflight', async ({ page, request }) => {
  seedRuntimeAccount(primaryProjectId)
  await page.route('**/optimization-target-capabilities', route => route.fulfill({ json: { snapshot_id: 'e2e-snapshot', context_hash: 'e2e-context', observed_at: new Date().toISOString(), options: [{ external_action: '19', semantic_key: 'click', display_name: '点击量', need_assets: false }] } }))
  const suffix = Date.now().toString(36)
  const planName = `E2E 平台配置计划 ${suffix}`
  await page.goto(`/projects/${primaryProjectId}/delivery/plans`)
  await expect(page.getByRole('heading', { name: '计划草稿' })).toBeVisible()
  await expect(page.getByText('保存时由服务端解析策略任务版本并写入内容哈希与返回入口。')).toHaveCount(0)
  await expect(page.getByText('暂无可靠策略建议')).toHaveCount(0)
  await startNewPlan(page, planName)
  await page.getByRole('button', { name: '预算与排期', exact: true }).click()
  await page.getByRole('spinbutton', { name: /日预算|总预算/ }).fill('3000')
  await page.getByRole('button', { name: '素材引用', exact: true }).click()
  await expect(page.getByRole('tablist', { name: '基础素材类型' })).toBeVisible()

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
    passed: true,
    blocked: false,
    checks: expect.arrayContaining([
      expect.objectContaining({ code: 'delivery_intent_valid', passed: true }),
      expect.objectContaining({ code: 'platform_configuration_valid', passed: true }),
      expect.objectContaining({ code: 'marketing_product_outside_intent', passed: true }),
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
  await page.getByLabel('巨量账户', { exact: true }).selectOption(runtimeAccountId(primaryProjectId))
  await page.getByLabel('策略来源').selectOption('task_demo_precision_strategy')
  await page.getByLabel('营销目的', { exact: true }).selectOption('lead_generation')
  await page.getByRole('button', { name: '选择产品', exact: true }).click()
  await page.getByRole('dialog', { name: '营销产品选择器' }).getByRole('radio').first().check()
  await page.getByRole('button', { name: '确认选择', exact: true }).click()
  await page.getByRole('button', { name: '投放载体和监测', exact: true }).click()
  await page.getByLabel('获取线索方式', { exact: true }).selectOption('custom_lead')
  await page.getByLabel('投放载体', { exact: true }).selectOption('owned_landing_page')
  await page.getByLabel('自研落地页链接', { exact: true }).fill('https://example.test/e2e')
  await page.getByLabel('优化目标', { exact: true }).selectOption('19')
}

test('plan and configuration share Connector choices, preserve mixed materials, and discard stale product pages', async ({ page }) => {
  seedRuntimeAccount(primaryProjectId)
  const account = runtimeAccountId(primaryProjectId)
  let failCapability = false
  await page.route('**/optimization-target-capabilities', route => {
    if (failCapability) { failCapability = false; return route.fulfill({ status: 503, json: { error: { message: '能力读取暂时失败' } } }) }
    return route.fulfill({ json: { snapshot_id: 'shared-snapshot', context_hash: 'shared-context', observed_at: new Date().toISOString(), options: [{ external_action: '19', semantic_key: 'click', display_name: '点击量', need_assets: false }] } })
  })
  let releaseOldPage: (() => void) | undefined
  await page.route('**/platform-objects?**', async route => {
    const query = new URL(route.request().url()).searchParams
    const kind = query.get('object_kind') ?? ''
    const object = (id: string, display_name: string) => ({ id: `${kind}-${id}`, object_kind: kind, platform_object_id: id, account_id: account, organization_id: 'org_local', display_name, status: 'active', version: 3, metadata: kind === 'marketing_product' ? { unique_product_id: id, product_id: `product-${id}` } : {}, project_granted: true, preview_available: false })
    if (query.get('cursor') === 'old-page') {
      await new Promise<void>(resolve => { releaseOldPage = resolve })
      return route.fulfill({ json: { items: [object('old', '过期分页产品')], next_cursor: '' } })
    }
    if (kind === 'marketing_product') {
      const searching = Boolean(query.get('q'))
      return route.fulfill({ json: { items: [object(searching ? 'found' : 'initial', searching ? '目标 Connector 产品' : '初始 Connector 产品')], next_cursor: searching ? '' : 'old-page' } })
    }
    return route.fulfill({ json: { items: ['video_material', 'image_material', 'aweme_photo_material'].includes(kind) ? [object(`shared-${kind}`, `共享 ${kind}`)] : [], next_cursor: '' } })
  })
  await page.goto(`/projects/${primaryProjectId}/delivery/plans`)
  await page.getByRole('button', { name: '新建投放计划' }).click()
  await page.getByLabel('计划名称', { exact: true }).fill('共享目录回归计划')
  await page.getByLabel('巨量账户', { exact: true }).selectOption(account)
  await page.getByLabel('策略来源', { exact: true }).selectOption('task_demo_precision_strategy')
  await page.getByLabel('营销目的', { exact: true }).selectOption('lead_generation')
  await page.getByRole('button', { name: '选择产品', exact: true }).click()
  const products = page.getByRole('dialog', { name: '营销产品选择器' })
  await expect(products.getByText('初始 Connector 产品')).toBeVisible()
  await products.getByRole('button', { name: '加载更多', exact: true }).click()
  await expect.poll(() => Boolean(releaseOldPage)).toBe(true)
  await products.getByRole('textbox').fill('目标')
  await expect(products.getByText('目标 Connector 产品')).toBeVisible()
  const oldResponse = page.waitForResponse(response => response.url().includes('cursor=old-page'))
  releaseOldPage!()
  await oldResponse
  await expect(products.getByText('过期分页产品')).toHaveCount(0)
  await products.getByRole('radio').check()
  await products.getByRole('button', { name: '确认选择' }).click()
  await page.screenshot({ path: '.cache/delivery-plan-choices.png', fullPage: true })
  await page.getByRole('button', { name: '投放载体和监测', exact: true }).click()
  failCapability = true
  await page.getByLabel('投放载体', { exact: true }).selectOption('orange_landing_page')
  await expect(page.getByRole('button', { name: '重试优化目标' })).toBeVisible()
  await page.getByRole('button', { name: '重试优化目标' }).click()
  await page.getByLabel('优化目标', { exact: true }).selectOption('19')
  await page.getByRole('button', { name: '素材引用', exact: true }).click()
  for (const [tab, label, kind] of [['视频', '视频素材选择器', 'video_material'], ['图片', '图片素材选择器', 'image_material'], ['图文', '抖音图文素材选择器', 'aweme_photo_material']]) {
    await page.getByRole('tab', { name: new RegExp(`^${tab}`) }).click()
    await page.getByRole('button', { name: '选择素材', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: label, exact: true })
    await dialog.locator('label').filter({ hasText: `共享 ${kind}` }).getByRole('checkbox').check()
    await dialog.getByRole('button', { name: '确认选择' }).click()
  }
  const saveResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith(`/projects/${primaryProjectId}/plans`))
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const saved = await saveResponse
  expect(saved.status(), await saved.text()).toBe(201)
  const body = await saved.json()
  const ocean = body.current_version.platform_configuration.payload.ocean_engine
  expect(ocean.project.marketing_product_reference).toMatchObject({ namespace: 'oceanengine', id: 'found', scope: `account:${account}` })
  expect(ocean.promotions.flatMap((item: { base_material_references: Array<{ object_kind: string }> }) => item.base_material_references.map(reference => reference.object_kind))).toEqual(expect.arrayContaining(['asset_version', 'video_material', 'image_material', 'aweme_photo_material']))
  await expect(page.getByRole('button', { name: '素材引用', exact: true })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: '计划名称', exact: true })).toBeVisible()
  await page.getByRole('link', { name: '查看平台配置', exact: true }).click()
  await expect(page.getByRole('heading', { name: '编辑投放项目和推广单元' })).toBeVisible()
  await expect(page.getByText('目标 Connector 产品', { exact: true })).toBeVisible()
  await expect(page.getByLabel('优化目标', { exact: true })).toHaveValue('19')
  await page.screenshot({ path: '.cache/delivery-shared-choices.png', fullPage: true })
})
