import { expect, test, type Page } from '@playwright/test'

import { runtimeAccountId, seedRuntimeAccount } from './delivery-runtime-fixture'

const primaryProjectId = 'project_investor_precision_evidence'
const otherProjectId = 'project_local'

test('intelligent filling is limited to configuration and protects edits and saved provenance', async ({ page }) => {
  test.setTimeout(90_000)
  seedRuntimeAccount(primaryProjectId)
  await page.route('**/optimization-target-capabilities', route => route.fulfill({ json: { snapshot_id: 'filling-snapshot', context_hash: 'context', observed_at: new Date().toISOString(), options: [{ external_action: '19', semantic_key: 'click', display_name: '点击量' }] } }))
  await page.route('**/strategy-packages', route => route.fulfill({ json: { items: [] } }))
  let release: (() => void) | undefined
  let delay = true
  let calls = 0
  let lastTarget = ''
  await page.route('**/filling-suggestions', async route => {
    const input = route.request().postDataJSON()
    expect(input.page).toBe('configuration')
    expect(input.target_id).toBeTruthy()
    lastTarget = input.target_id
    expect(input.fields).not.toEqual(expect.arrayContaining(['daily_budget']))
    expect(input.fields).toContain('selling_points')
    expect(input.fields).toContain('search_keywords')
    expect(input.material_count).toBe(1)
    expect(input.finance).toEqual(calls === 0 ? undefined : { unit_weight: 2 })
    expect(input.copy_count).toBe(2)
    expect(input.instructions).toBe('只写真实产品信息')
    calls += 1
    if (delay) await new Promise<void>(resolve => { release = resolve })
    await route.fulfill({ json: { suggestions: calls >= 3 ? [{ field: 'materials', value: [{ namespace: 'cookies', object_kind: 'asset_version', scope: `project:${primaryProjectId}`, id: 'asset_demo_investor_creative_image', version: '1', state: 'resolved', audit_attributes: { media_kind: 'image' } }], reason: '图像匹配', sources: ['已确认素材'] }] : [{ field: 'copy', value: '查看产品详情', reason: '沿用项目身份', sources: ['当前项目'] }, ...(input.finance ? [{ field: 'daily_budget', value: 30000, reason: '按项目预算分配', sources: ['当前项目日预算'] }] : [])], finance: input.finance ? { rule_version: 'delivery-filling-finance/v1', entries: calls < 3 ? [{ field: 'daily_budget', amount_minor: 30000, minimum_minor: 30000, maximum_minor: 30000, basis: '按项目预算分配', sources: ['当前项目日预算'] }] : [], warnings: ['历史出价不足，保留原值。'] } : undefined, warnings: ['未选择已批准策略'], context_hash: 'hash', provider: 'ark_text', model: 'fixture-model' } })
  })
  await page.goto(`/projects/${primaryProjectId}/delivery/plans`)
  await expect(page.getByRole('heading', { name: '计划草稿' })).toBeVisible({ timeout: 30_000 })
  await startNewPlan(page, `智能填写原名称 ${Date.now()}`)
  await expect(page.getByRole('button', { name: '智能填写', exact: true })).toHaveCount(0)
  const create = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/plans'))
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const saved = await (await create).json()
  const id = saved.id
  await page.goto(`/projects/${primaryProjectId}/delivery/configuration?view=${encodeURIComponent('配置映射')}&plan_id=${id}`)
  await expect(page.locator('.delivery-config-project-editor > .delivery-filling')).toHaveCount(0)
  await page.locator('.delivery-config-editor > .delivery-filling').getByRole('button', { name: '智能填写', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '智能填写', exact: true })
  await expect(dialog).toBeVisible()
  await expect(page.locator('.delivery-config-unit-card .delivery-filling')).toHaveCount(0)
  await expect(dialog.getByLabel('填写单元')).toHaveValue(saved.current_version.platform_configuration.payload.ocean_engine.promotions[0].promotion_draft_id)
  await dialog.getByLabel('文案数量').fill('2')
  await dialog.getByRole('textbox', { name: '附加说明', exact: true }).fill('只写真实产品信息')
  await dialog.getByRole('button', { name: '开始填写', exact: true }).click()
  await expect.poll(() => calls).toBe(1)
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await page.getByRole('textbox', { name: /文案素材/ }).fill('取消后手工修改')
  release?.()
  delay = false
  await page.locator('.delivery-config-editor > .delivery-filling').getByRole('button', { name: '智能填写', exact: true }).click()
  await dialog.getByRole('checkbox', { name: '填写预算与出价' }).check()
  await dialog.getByLabel('当前单元预算权重').fill('2')
  await dialog.getByRole('button', { name: '开始填写', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('checkbox', { name: '应用文案建议' })).toHaveCount(0)
  await expect(page.getByText('沿用项目身份', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: /文案素材/ })).toHaveValue('查看产品详情')
  const update = page.waitForResponse(response => response.request().method() === 'PATCH' && new URL(response.url()).pathname.endsWith(`/plans/${id}`))
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const updated = await (await update).json()
  const expectedOcean = structuredClone(saved.current_version.platform_configuration.payload.ocean_engine)
  expectedOcean.promotions[0].copy_items = [{ text: '查看产品详情' }]
  expectedOcean.promotions[0].budget_and_bidding = { currency: 'CNY', bidding_strategy: expectedOcean.project.budget_and_bidding.bidding_strategy, charging_mode: expectedOcean.project.budget_and_bidding.charging_mode, bid_minor: 0, ...expectedOcean.promotions[0].budget_and_bidding, budget_mode: 'daily', daily_budget_minor: 30000 }
  expect(updated.current_version.platform_configuration.payload.ocean_engine).toEqual(expectedOcean)
  expect(updated.current_version.platform_configuration.compilation_metadata.filling_history[0].target_id).toBe(expectedOcean.promotions[0].promotion_draft_id)
  expect(updated.current_version.platform_configuration.compilation_metadata.filling_history).toHaveLength(1)
  await page.reload()
  await expect(page.getByLabel('预算与出价填写记录')).toContainText('按项目预算分配')
  await page.locator('.delivery-config-editor > .delivery-filling').getByRole('button', { name: '智能填写', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '智能填写', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '应用所选建议' })).toHaveCount(0)
  await dialog.getByRole('checkbox', { name: '填写预算与出价' }).check()
  await dialog.getByLabel('当前单元预算权重').fill('2')
  await dialog.getByLabel('文案数量').fill('2')
  await dialog.getByRole('textbox', { name: '附加说明', exact: true }).fill('只写真实产品信息')
  await dialog.getByRole('button', { name: '开始填写', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  const materialTabs = page.getByRole('tablist', { name: '基础素材类型' })
  await expect(materialTabs.getByRole('tab', { name: '图片 1', exact: true })).toHaveAttribute('aria-selected', 'true')
  await materialTabs.getByRole('tab', { name: '视频 0', exact: true }).click()
  await expect(materialTabs.getByRole('tab', { name: '视频 0', exact: true })).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('button', { name: '增加推广单元', exact: true }).click()
  await page.locator('.delivery-config-editor > .delivery-filling').getByRole('button', { name: '智能填写', exact: true }).click()
  const targetSelect = dialog.getByLabel('填写单元')
  await targetSelect.selectOption({ index: 1 })
  const selectedTarget = await targetSelect.inputValue()
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '开始填写', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  expect(lastTarget).toBe(selectedTarget)
  await expect(page.locator('.delivery-config-unit-card').last().getByRole('tab', { name: '图片 1', exact: true })).toHaveAttribute('aria-selected', 'true')


})

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
