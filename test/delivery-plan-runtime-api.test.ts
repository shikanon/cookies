import assert from 'node:assert/strict'
import test from 'node:test'
import { deliveryPlanApi, type DeliveryPlanDraft } from '../src/api/delivery.ts'

test('delivery plan client writes DeliveryIntent plus tagged PlatformConfiguration and reads v2 history', async t => {
  const originalFetch = globalThis.fetch
  let written: any
  globalThis.fetch = async (_url, init) => {
    written = JSON.parse(String(init?.body))
    const hash = 'a'.repeat(64)
    written.intent.canonical_hash = hash
    written.platform_configuration.canonical_hash = hash
    written.platform_configuration.intent = {
      schema_version: written.intent.schema_version,
      intent_id: written.intent.intent_id,
      version_number: written.intent.version_number,
      canonical_hash: hash,
    }
    const version = {
      schema_version: 'delivery-plan-version/v2', runtime_status: 'active', read_only: false,
      plan_id: 'plan_1', organization_id: 'org_1', project_id: 'project_1', version_number: 1,
      canonical_hash: hash, intent: written.intent, platform_configuration: written.platform_configuration,
      advertiser: { id: '', name: '', platform: '', source: 'mock', scenario: 'platform_configuration' },
      budget: { total_minor: 0, currency: '' },
      schedule: { start_at: '0001-01-01T00:00:00Z', end_at: '0001-01-01T00:00:00Z', timezone: '' },
      tracking: { landing_page: '', pixel_id: '', conversion_event: '' },
      platform: 'ocean_engine', source: 'mock', scenario: 'platform_configuration',
      created_by: { kind: 'user', id: 'user_1' }, created_at: '2026-08-10T00:00:00Z',
    }
    return new Response(JSON.stringify({
      id: 'plan_1', organization_id: 'org_1', project_id: 'project_1', status: 'draft', platform: 'ocean_engine',
      source: 'mock', scenario: 'platform_configuration', current_version_number: 1, current_version: version, versions: [version],
      created_by: 'user_1', created_at: version.created_at, updated_at: version.created_at,
    }), { headers: { 'Content-Type': 'application/json' } })
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const plan = await deliveryPlanApi.create('project_1', draft())
  assert.deepEqual(Object.keys(written).sort(), ['intent', 'platform_configuration'])
  assert.equal(written.intent.schema_version, 'delivery-intent/v1')
  assert.equal(written.intent.hash_algorithm, 'RFC8785-JCS-SHA256(canonical_payload)')
  assert.equal(written.platform_configuration.schema_version, 'delivery-platform-configuration/v2')
  assert.equal(written.platform_configuration.hash_algorithm, 'RFC8785-JCS-SHA256(canonical_payload)')
  assert.equal(written.platform_configuration.payload.profile, 'ocean_engine')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.marketing_purpose, 'lead_generation')
  assert.equal('product_selection_mode' in written.platform_configuration.payload.ocean_engine.project, false)
  assert.equal(written.platform_configuration.payload.ocean_engine.project.marketing_product_reference.id, 'product-1')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.deep_optimization_mode, 'disabled')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.placement_strategy, 'automatic')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.carrier, 'owned_landing_page')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.optimization_target_reference.audit_attributes.event_asset_type, 'web')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.schedule.mode, 'fixed_range')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.search_boost.bid_coefficient, 1.1)
  assert.equal(written.platform_configuration.payload.ocean_engine.project.monitoring_references.length, 5)
  assert.equal(written.platform_configuration.payload.ocean_engine.promotions.length, 1)
  assert.equal(written.three_tier_configuration, undefined)
  assert.equal(plan.currentVersion.canonicalHash, plan.currentVersion.platformConfiguration?.canonical_hash)
  assert.equal(plan.currentVersion.deliveryIntent?.payload.marketing_objective, 'qualified conversions')
  assert.equal(plan.currentVersion.marketingPurpose, 'lead_generation')
  assert.equal(plan.currentVersion.marketingProduct.id, 'product-1')
  assert.equal(plan.currentVersion.tracking.deliveryCarrier, 'owned_landing_page')
  assert.equal(plan.currentVersion.tracking.monitoringValidVideoPlay, 'https://monitor.example.test/valid-video-play')
  assert.deepEqual(plan.currentVersion.schedule, {
    mode: 'fixed_range',
    startAt: '2026-08-11T00:00:00+08:00',
    endAt: '2026-08-15T00:00:00+08:00',
    timezone: 'Asia/Shanghai',
  })

  const longTerm = draft()
  longTerm.schedule.mode = 'long_term'
  longTerm.tracking.deliveryCarrier = 'orange_landing_page'
  longTerm.tracking.landingPage = ''
  longTerm.tracking.optimizationTargetId = 'builtin:in_app_order'
  longTerm.tracking.optimizationTargetName = 'app内下单'
  longTerm.tracking.optimizationTargetSemanticKey = 'in_app_order'
  longTerm.tracking.eventAssetName = ''
  longTerm.tracking.eventAssetType = ''
  await deliveryPlanApi.create('project_1', longTerm)
  assert.equal(written.platform_configuration.payload.ocean_engine.project.schedule.mode, 'long_term')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.budget_and_bidding.daily_budget_minor, longTerm.budget.totalMinor)
  assert.equal(written.platform_configuration.payload.ocean_engine.project.optimization_target_reference.semantic_key, 'in_app_order')
  assert.equal(written.platform_configuration.payload.ocean_engine.promotions[0].settings.call_to_action, undefined)

  await deliveryPlanApi.update('project_1', 'plan_1', 4, draft())
  assert.equal(written.expected_version, 4)
  assert.equal(written.intent.intent_id, 'intent-plan_1-plan-v5')
  assert.equal(written.platform_configuration.configuration_id, 'configuration-plan_1-plan-v5')
  assert.equal(written.platform_configuration.payload.ocean_engine.project.project_draft_id, 'project-plan_1-5')
  assert.equal(written.platform_configuration.payload.ocean_engine.promotions[0].promotion_draft_id, 'promotion-plan_1-5-1')

  plan.currentVersion.deliveryIntent!.payload.product_references = []
  const editedConfiguration = structuredClone(plan.currentVersion.platformConfiguration!)
  editedConfiguration.payload.ocean_engine!.promotions[0].base_material_references = [{
    namespace: 'oceanengine', object_kind: 'video_material', scope: 'account:account-1', id: 'video-1', state: 'resolved',
  }]
  editedConfiguration.payload.ocean_engine!.promotions[0].product_image_references = [{
    namespace: 'oceanengine', object_kind: 'product_image', scope: 'account:account-1', id: 'image-1', state: 'resolved',
  }]
  await deliveryPlanApi.updatePlatformConfiguration('project_1', plan, editedConfiguration)
  assert.deepEqual(written.intent.payload.product_references.map((reference: { id?: string }) => reference.id), ['product-1'])
  assert.deepEqual(written.intent.payload.material_references.map((reference: { id?: string }) => reference.id), ['asset-1', 'video-1', 'image-1'])
  assert.equal(written.intent.intent_id, written.platform_configuration.intent.intent_id)
  assert.equal(written.platform_configuration.payload.ocean_engine.project.project_draft_id, 'project-plan_1-2')
  assert.equal(written.platform_configuration.payload.ocean_engine.promotions[0].promotion_draft_id, 'promotion-plan_1-2-1')
})

function draft(): DeliveryPlanDraft {
  return {
    name: 'Ocean launch', objective: 'qualified conversions', marketingPurpose: 'lead_generation',
    marketingProduct: { id: 'product-1', name: '测试商品', activityType: '常规', activityName: '测试活动', brandName: '测试品牌' },
    advertiser: { id: 'account-1', name: 'Account one', platform: 'ocean_engine' },
    budget: { totalMinor: 100000, currency: 'CNY' },
    schedule: { mode: 'fixed_range', startAt: '2026-08-11T00:00:00+08:00', endAt: '2026-08-15T00:00:00+08:00', timezone: 'Asia/Shanghai' },
    tracking: {
      deliveryCarrier: 'owned_landing_page', landingPage: 'https://example.test/landing', pixelId: 'pixel-1', conversionEvent: 'purchase',
      optimizationTargetId: 'target-1', optimizationTargetName: '表单提交', optimizationTargetSemanticKey: '', eventAssetName: '测试网页事件', eventAssetType: 'web',
      searchKeywords: '品牌词，商品词', searchBidCoefficient: 1.1, searchTargetingExpansion: true,
      monitoringImpression: 'https://monitor.example.test/impression', monitoringValidTouch: 'https://monitor.example.test/valid-touch',
      monitoringVideoPlay: 'https://monitor.example.test/video-play', monitoringVideoComplete: 'https://monitor.example.test/video-complete',
      monitoringValidVideoPlay: 'https://monitor.example.test/valid-video-play',
    },
    creativeReferences: [{ assetId: 'asset-1', version: 1, contentHash: 'b'.repeat(64), confirmed: true }],
    strategyReference: { taskId: 'strategy-1', version: 2, contentHash: 'c'.repeat(64) },
    sourceStrategyVersion: 'strategy-1@v2',
  }
}
