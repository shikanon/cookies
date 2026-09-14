import assert from 'node:assert/strict'
import test from 'node:test'
import { deliveryPlanApi, toPlatformRuntimeDraft, type DeliveryPlanDraft, type StableReference } from '../src/api/delivery'
import { applyPlanConfiguration, planCreativeReference } from '../src/lib/deliveryPlanForm'
import { carrierOptions, changeConfigurationAccount, changeConfigurationProject, changeProjectChoice } from '../src/lib/deliveryChoices'
import { oceanEngineOptimizationTargetContext } from '../src/lib/oceanengineBranchConstraints'

const reference = (namespace: string, object_kind: string, id: string): StableReference => ({ namespace, object_kind, scope: namespace === 'cookies' ? 'project:p' : 'account:a', id, version: '7', content_hash: 'b'.repeat(64), state: 'resolved', display_name_snapshot: `${namespace} ${id}`, audit_attributes: { selection_kind: 'async_row', connector_platform_object_id: `connector-${object_kind}-${id}` } })

function draft(): DeliveryPlanDraft {
  return {
    name: '计划', objective: '成交', marketingPurpose: 'ecommerce',
    marketingProduct: { reference: reference('oceanengine', 'product', 'same-id'), id: 'same-id', name: '产品', activityType: '', activityName: '', brandName: '' },
    advertiser: { id: 'a', name: '账户 A', platform: 'ocean_engine' },
    budget: { currency: 'CNY', totalMinor: 300000 },
    schedule: { mode: 'long_term', startAt: '2026-09-14T00:00:00+08:00', endAt: '2099-12-31T23:59:59+08:00', timezone: 'Asia/Shanghai' },
    tracking: { deliveryCarrier: 'owned_landing_page', landingPage: 'https://example.test', pixelId: '', conversionEvent: '', optimizationTargetId: '19', optimizationTargetName: '点击', optimizationTargetSemanticKey: 'click', eventAssetName: '', eventAssetType: '', optimizationTargetReference: { ...reference('oceanengine_capability', 'optimization_target', '19'), audit_attributes: { capability_snapshot_id: 'snapshot-1', capability_context_hash: 'context-1' } }, searchKeywords: '', searchBidCoefficient: 1.1, searchTargetingExpansion: false, monitoringImpression: 'https://example.test/monitor', monitoringValidTouch: '', monitoringVideoPlay: '', monitoringVideoComplete: '', monitoringValidVideoPlay: '' },
    creativeReferences: [reference('cookies', 'asset_version', 'same-id'), reference('oceanengine', 'video_material', 'same-id'), reference('oceanengine', 'image_material', 'image'), reference('oceanengine', 'aweme_photo_material', 'photo')].map(planCreativeReference),
    strategyReference: { taskId: 'strategy', version: 1 }, sourceStrategyVersion: 'strategy@v1',
  }
}

test('mixed references and hidden configuration survive a plan-summary update', async t => {
  const initial = draft()
  const runtime = toPlatformRuntimeDraft('p', 'plan', 1, initial)
  const ocean = runtime.platform_configuration.payload.ocean_engine!
  const image = reference('oceanengine', 'product_image', 'product-image')
  ocean.promotions[0].product_image_references = [image]
  ocean.promotions[0].copy_items = [{ text: '保留文案' }]
  ocean.project.targeting.regions = ['上海']
  ocean.project.budget_and_bidding.bid_minor = 900
  runtime.intent.payload.material_references.push(image)
  runtime.intent.payload.optimization_preferences = [{ metric: 'roi', direction: 'maximize' }]
  const plan = { id: 'plan', project_id: 'p', organization_id: 'org', current_version_number: 1, versions: [] as object[], current_version: { plan_id: 'plan', project_id: 'p', organization_id: 'org', version_number: 1, schema_version: 'delivery-plan-version/v2', runtime_status: 'active', read_only: false, source: 'mock', scenario: 'platform_configuration', platform: 'ocean_engine', ...runtime } }
  plan.current_version = { ...plan.current_version, ...{ intent: runtime.intent, platform_configuration: runtime.platform_configuration } }
  const originalFetch = globalThis.fetch
  let written: ReturnType<typeof toPlatformRuntimeDraft> | undefined
  globalThis.fetch = async (_url, init) => {
    if (init?.body) {
      written = JSON.parse(String(init.body)) as ReturnType<typeof toPlatformRuntimeDraft>
      plan.current_version = { ...plan.current_version, ...written }
    }
    return Response.json(plan)
  }
  t.after(() => { globalThis.fetch = originalFetch })
  const loaded = await deliveryPlanApi.get('p', 'plan')
  assert.equal(loaded.currentVersion.creativeReferences.length, 4, 'product images must not become base materials')
  assert.deepEqual(loaded.currentVersion.creativeReferences.map(item => item.reference), initial.creativeReferences.map(item => item.reference))
  assert.deepEqual(loaded.currentVersion.marketingProduct.reference, initial.marketingProduct.reference)
  assert.deepEqual(loaded.currentVersion.tracking.optimizationTargetReference, initial.tracking.optimizationTargetReference)
  await deliveryPlanApi.update('p', 'plan', 1, { ...loaded.currentVersion, name: '新名称', budget: { currency: 'CNY', totalMinor: 420000 } })
  assert.ok(written)
  assert.deepEqual(written.platform_configuration.payload.ocean_engine!.promotions, ocean.promotions)
  assert.deepEqual(written.platform_configuration.payload.ocean_engine!.project.targeting, ocean.project.targeting)
  assert.equal(written.platform_configuration.payload.ocean_engine!.project.budget_and_bidding.bid_minor, 900)
  assert.equal(written.platform_configuration.payload.ocean_engine!.project.budget_and_bidding.daily_budget_minor, 420000)
  assert.deepEqual(written.intent.payload.material_references, runtime.intent.payload.material_references)
  assert.deepEqual(written.intent.payload.optimization_preferences, runtime.intent.payload.optimization_preferences)
  assert.deepEqual(written.intent.payload.product_references, [initial.marketingProduct.reference])
})

test('account changes clear all account-bound objects while retaining Cookies references', () => {
  const initial = draft()
  const ocean = toPlatformRuntimeDraft('p', 'plan', 1, initial).platform_configuration.payload.ocean_engine!
  ocean.promotions[0].settings.brand_reference = reference('oceanengine', 'brand', 'brand')
  ocean.promotions[0].delivery_identity.authorized_identity = reference('oceanengine', 'identity', 'identity')
  ocean.promotions[0].product_image_references = [reference('oceanengine', 'product_image', 'image')]
  const changed = changeConfigurationAccount(ocean, { id: 'b', display_label: 'B' })
  assert.equal(changed.project.marketing_product_reference, undefined)
  assert.equal(changed.project.optimization_target_reference, undefined)
  assert.deepEqual(changed.project.monitoring_references, [])
  assert.deepEqual(changed.promotions.flatMap(item => item.base_material_references), [initial.creativeReferences[0].reference])
  assert.equal(changed.promotions[0].settings.brand_reference, undefined)
  assert.equal(changed.promotions[0].delivery_identity.authorized_identity, undefined)
  assert.deepEqual(changed.promotions[0].product_image_references, [])
  assert.deepEqual(changed.promotions.map(item => item.promotion_draft_id), ocean.promotions.map(item => item.promotion_draft_id))
  const form = applyPlanConfiguration(initial, changed)
  assert.equal(form.marketingProduct.id, '')
  assert.equal(form.tracking.optimizationTargetId, '')
  assert.equal(form.tracking.monitoringImpression, '')
})

test('lead and native-content branches share eligible carriers and invalidate dependent choices', () => {
  const ocean = toPlatformRuntimeDraft('p', 'plan', 1, draft()).platform_configuration.payload.ocean_engine!
  const lead = changeProjectChoice(ocean.project, { marketing_purpose: 'lead_generation', lead_capture_mode: 'custom_lead' })
  assert.ok(carrierOptions(lead).some(option => option.value === 'owned_landing_page'))
  const smart = changeProjectChoice(lead, { lead_capture_mode: 'smart_lead' })
  assert.equal(smart.carrier, 'orange_landing_page')
  assert.equal(smart.optimization_target_reference, undefined)
  assert.equal(carrierOptions(smart).some(option => option.value === 'owned_landing_page'), false)
  assert.deepEqual(oceanEngineOptimizationTargetContext(smart)?.multi_asset_types, [2])
  const native = changeConfigurationProject(ocean, changeProjectChoice(ocean.project, { marketing_purpose: 'content_marketing' }))
  assert.equal(native.project.carrier, 'douyin_account')
  assert.ok(native.promotions.every(item => !item.base_material_references.length && !item.landing_page_reference))
  assert.deepEqual(native.promotions.map(item => item.promotion_draft_id), ocean.promotions.map(item => item.promotion_draft_id))
})
