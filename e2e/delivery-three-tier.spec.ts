import { expect, test, type APIRequestContext } from '@playwright/test'

const projectId = 'project_investor_precision_evidence'
const otherProjectId = 'project_local'

test('DeliveryIntent and tagged PlatformConfiguration stay immutable, project-scoped, and approval-bound', async ({ page, request }) => {
  const suffix = Date.now().toString(36)
  const created = await createPlan(request, suffix, 2)
  expect(created.current_version).toMatchObject({
    schema_version: 'delivery-plan-version/v2', runtime_status: 'active', read_only: false,
    canonical_hash: created.current_version.platform_configuration.canonical_hash,
    intent: { schema_version: 'delivery-intent/v1', version_number: 1 },
    platform_configuration: {
      schema_version: 'delivery-platform-configuration/v2', platform: 'ocean_engine', profile_version: 'oceanengine-configuration/v1',
      payload: { profile: 'ocean_engine', ocean_engine: { project: expect.any(Object), promotions: expect.any(Array) } },
    },
  })
  expect(created.current_version.platform_configuration.payload.ocean_engine.promotions).toHaveLength(2)
  expect(created.current_version.three_tier_configuration).toBeUndefined()

  const isolated = await request.get(`/api/delivery/v1/projects/${otherProjectId}/plans/${created.id}`)
  expect(isolated.status()).toBe(404)

  const legacyCompile = await request.post(`${planURL(created.id)}/configuration:compile`, { data: { expected_version: 1, fixture: 'golden_path' } })
  expect(legacyCompile.status()).toBe(409)
  expect(await legacyCompile.json()).toMatchObject({ error: { code: 'LEGACY_CONFIGURATION_UNSUPPORTED' } })
  const legacyOverride = await request.post(`${planURL(created.id)}/configuration:override`, { data: {} })
  expect(legacyOverride.status()).toBe(409)

  const planPreflight = await request.post(`${planURL(created.id)}/preflight`)
  expect(planPreflight.status()).toBe(200)
  expect(await planPreflight.json()).toMatchObject({
    passed: true, blocked: false, scenario: 'platform_configuration',
    checks: expect.arrayContaining([
      expect.objectContaining({ code: 'delivery_intent_valid', passed: true }),
      expect.objectContaining({ code: 'platform_configuration_valid', passed: true }),
    ]),
  })

  const draftResponse = await request.post(`${planURL(created.id)}:create-change-set`, { data: { expected_version: 1 } })
  expect(draftResponse.status()).toBe(201)
  let changeSet = await draftResponse.json() as ChangeSet
  expect(changeSet.target_snapshot).toMatchObject({ schema_version: 'delivery-platform-configuration/v2', configuration_id: `configuration-${suffix}` })
  expect(changeSet.target_snapshot_hash).toBe(created.current_version.canonical_hash)
  const checked = await request.post(changeSetActionURL(changeSet.id, 'preflight'), { data: { expected_version: changeSet.version } })
  expect(checked.status()).toBe(200)
  changeSet = await checked.json() as ChangeSet
  const approved = await request.post(changeSetActionURL(changeSet.id, 'approve'), { data: { expected_version: changeSet.version } })
  expect(approved.status()).toBe(200)
  changeSet = await approved.json() as ChangeSet
  expect(changeSet.approval).toMatchObject({
    valid: true,
    target_snapshot_hash: created.current_version.canonical_hash,
    configuration_schema_version: 'delivery-platform-configuration/v2',
    configuration_id: `configuration-${suffix}`,
    configuration_version: 1,
    configuration_platform: 'ocean_engine',
    configuration_profile_version: 'oceanengine-configuration/v1',
    configuration_canonical_hash: created.current_version.canonical_hash,
    intent_schema_version: 'delivery-intent/v1',
    intent_id: `intent-${suffix}`,
    intent_version: 1,
    intent_canonical_hash: created.current_version.intent.canonical_hash,
  })

  const updateResponse = await request.patch(planURL(created.id), { data: runtimePayload(suffix, 2, 0, 'Updated Ocean project') })
  expect(updateResponse.status()).toBe(200)
  const updated = await updateResponse.json() as DeliveryPlan
  expect(updated.current_version_number).toBe(2)
  expect(updated.versions).toHaveLength(2)
  expect(updated.current_version.platform_configuration.payload.ocean_engine.promotions).toHaveLength(0)
  expect(updated.current_version.canonical_hash).not.toBe(created.current_version.canonical_hash)

  const staleExecution = await request.post(changeSetActionURL(changeSet.id, 'execute'), {
    headers: { 'Idempotency-Key': `stale-${suffix}` }, data: { expected_version: changeSet.version, scenario: 'success' },
  })
  expect(staleExecution.status()).toBe(409)
  expect(await staleExecution.json()).toMatchObject({ error: { code: expect.stringMatching(/STALE_PLAN_VERSION|APPROVAL_CONTENT_MISMATCH/) } })

  await page.goto(`/projects/${projectId}/delivery/configuration?plan_id=${created.id}&view=${encodeURIComponent('配置映射')}`)
  await expect(page.getByRole('heading', { name: 'Updated Ocean project', level: 4 })).toBeVisible()
  await expect(page.getByText('每日预算', { exact: true })).toBeVisible()
  await page.getByText('查看技术信息', { exact: true }).click()
  await expect(page.getByText('schema=delivery-platform-configuration/v2')).toBeVisible()
  await expect(page.getByText('暂未添加推广单元')).toBeVisible()
  await expect(page.getByRole('button', { name: /编译三层配置|人工覆盖/ })).toHaveCount(0)
  await page.goto(`/projects/${projectId}/delivery/three-tier?plan_id=${created.id}`)
  await expect.poll(() => new URL(page.url()).pathname).toBe(`/projects/${projectId}/delivery/configuration`)
})

test('Magnetic Engine is a stable non-executable CAPABILITY_PENDING profile', async ({ request }) => {
  const suffix = `magnetic-${Date.now().toString(36)}`
  const input = runtimePayload(suffix, 1, 0, 'Magnetic pending')
  input.platform_configuration = {
    schema_version: 'delivery-platform-configuration/v2', configuration_id: `configuration-${suffix}`, version_number: 1,
    platform: 'magnetic_engine', profile_version: 'magnetic-engine-configuration/v1', hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
    payload: { profile: 'magnetic_engine', magnetic_engine: { profile: 'magnetic_engine', status: 'capability_pending', reason_code: 'CAPABILITY_PENDING', reason: 'no verified field evidence' } },
    configuration_provenance: { kind: 'manual', generator_ref: 'e2e' }, fact_provenance: { source: 'mock', snapshot_ref: `mock://${suffix}` }, compilation_metadata: {},
  }
  const response = await request.post(`/api/delivery/v1/projects/${projectId}/plans`, { data: { intent: input.intent, platform_configuration: input.platform_configuration } })
  expect(response.status()).toBe(201)
  const plan = await response.json() as DeliveryPlan
  expect(plan.current_version).toMatchObject({ runtime_status: 'capability_pending', scenario: 'capability_pending' })
  const preflight = await request.post(`${planURL(plan.id)}/preflight`)
  expect(preflight.status()).toBe(200)
  expect(await preflight.json()).toMatchObject({ passed: false, blocked: true, checks: [expect.objectContaining({ code: 'CAPABILITY_PENDING' })] })
})

async function createPlan(request: APIRequestContext, suffix: string, promotions: number): Promise<DeliveryPlan> {
  const payload = runtimePayload(suffix, 1, promotions, 'Ocean project')
  const response = await request.post(`/api/delivery/v1/projects/${projectId}/plans`, { data: { intent: payload.intent, platform_configuration: payload.platform_configuration } })
  expect(response.status()).toBe(201)
  return response.json() as Promise<DeliveryPlan>
}

function runtimePayload(suffix: string, version: number, promotionCount: number, name: string): any {
  const ref = (kind: string, id: string) => ({ namespace: 'cookies', object_kind: kind, scope: `project:${projectId}`, id, version: 'v1', content_hash: 'a'.repeat(64), state: 'resolved' })
  const material = ref('asset_version', 'asset_demo_investor_creative_video')
  const intent = {
    schema_version: 'delivery-intent/v1', intent_id: `intent-${suffix}`, version_number: version, hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
    payload: {
      payload_schema_version: 'delivery-intent/v1', marketing_objective: 'qualified conversions',
      budget_boundary: { currency: 'CNY', minimum_total_minor: 0, maximum_total_minor: 300000 },
      schedule_boundary: { earliest_start: '2026-08-11T00:00:00+08:00', latest_end: '2026-08-25T00:00:00+08:00', timezone: 'Asia/Shanghai' },
      optimization_preferences: [], material_references: [material], audience_constraints: {}, strategy_reference: ref('strategy_version', 'task_demo_precision_strategy'),
    },
    configuration_provenance: { kind: 'manual', generator_ref: 'e2e' }, fact_provenance: { source: 'mock', snapshot_ref: `mock://${suffix}/intent/${version}` },
  }
  const promotions = Array.from({ length: promotionCount }, (_, index) => ({
    draft_schema_version: 'oceanengine-configuration/v1', promotion_draft_id: `promotion-${suffix}-${index + 1}`,
    delivery_identity: { mode: 'account_info' }, base_material_references: [material], copy_items: [{ text: `copy ${index + 1}` }], settings: {}, promotion_name: `Promotion ${index + 1}`,
  }))
  return {
    expected_version: version - 1,
    intent,
    platform_configuration: {
      schema_version: 'delivery-platform-configuration/v2', configuration_id: `configuration-${suffix}`, version_number: version,
      platform: 'ocean_engine', profile_version: 'oceanengine-configuration/v1', hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
      payload: { profile: 'ocean_engine', ocean_engine: { profile: 'ocean_engine', project: {
        draft_schema_version: 'oceanengine-configuration/v1', project_draft_id: `project-${suffix}-${version}`, account_reference: ref('advertiser_account', 'account-1'),
        marketing_purpose: 'ecommerce', marketing_scenario: 'manual_delivery', carrier: 'landing_page', delivery_mode: 'manual', targeting: { smart_expansion: false },
        schedule: { start_at: '2026-08-11T00:00:00+08:00', end_at: '2026-08-25T00:00:00+08:00', timezone: 'Asia/Shanghai' },
        budget_and_bidding: { currency: 'CNY', daily_budget_minor: 20000, bidding_strategy: 'manual_bid', charging_mode: 'CPC', bid_minor: 100 }, project_name: name,
      }, promotions } },
      configuration_provenance: { kind: 'manual', generator_ref: 'e2e' }, fact_provenance: { source: 'mock', snapshot_ref: `mock://${suffix}/configuration/${version}` },
      compilation_metadata: { field_evidence: [{ field: 'project', state: 'operator_reviewed' }], evidence_refs: [] },
    },
  }
}

function planURL(planId: string) { return `/api/delivery/v1/projects/${projectId}/plans/${planId}` }
function changeSetActionURL(id: string, action: string) { return `/api/delivery/v1/projects/${projectId}/change-sets/${id}:${action}` }

type DeliveryPlan = {
  id: string
  current_version_number: number
  versions: Array<Record<string, unknown>>
  current_version: any
}
type ChangeSet = { id: string; version: number; target_snapshot: any; target_snapshot_hash: string; approval?: any }
