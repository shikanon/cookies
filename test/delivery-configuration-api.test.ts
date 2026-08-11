import assert from 'node:assert/strict'
import test from 'node:test'
import { DeliveryApiError, deliveryConfigurationApi } from '../src/api/delivery.ts'

const now = '2026-08-04T08:00:00.000Z'

test('delivery configuration client blocks all legacy ThreeTier writes before transport', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    return jsonResponse(planPayload())
  }
  t.after(() => { globalThis.fetch = originalFetch })

  await assert.rejects(deliveryConfigurationApi.compile('project_1', 'plan_1', 2, 'golden_path'), (error: unknown) => error instanceof DeliveryApiError && error.code === 'LEGACY_CONFIGURATION_UNSUPPORTED')
  await assert.rejects(deliveryConfigurationApi.override('project_1', 'plan_1', {
    expectedVersion: 3,
    groupId: 'group_1',
    planId: 'tier_plan_1',
    creativeId: 'creative_1',
    fieldKey: 'budget',
    value: { type: 'integer', value: 320000 },
    confirmed: true,
  }), (error: unknown) => error instanceof DeliveryApiError && error.code === 'LEGACY_CONFIGURATION_UNSUPPORTED')
  assert.equal(calls.length, 0)
})

test('delivery configuration recommendation and approved manual-package endpoints preserve their safety boundary', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    const path = String(url)
    if (path.endsWith('/recommendations')) return jsonResponse({ items: [recommendationPayload()], source: 'mock' })
    if (path.endsWith(':accept')) return jsonResponse({ recommendation: { ...recommendationPayload(), status: 'accepted' }, change_set: changeSetPayload() })
    if (path.endsWith(':reject')) return jsonResponse({ ...recommendationPayload(), status: 'rejected' })
    if (path.includes('manual-action-package')) return jsonResponse(manualPackagePayload())
    return jsonResponse(recommendationPayload())
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const generated = await deliveryConfigurationApi.generateRecommendations('project_1', 'plan_1', 2)
  const listed = await deliveryConfigurationApi.listRecommendations('project_1')
  const accepted = await deliveryConfigurationApi.acceptRecommendation('project_1', generated.id, generated.version, 'idem-three-tier')
  const rejected = await deliveryConfigurationApi.rejectRecommendation('project_1', generated.id, generated.version)
  const manual = await deliveryConfigurationApi.compileManualActionPackage('project_1', accepted.changeSet.id, accepted.changeSet.version)
  const reread = await deliveryConfigurationApi.getManualActionPackage('project_1', accepted.changeSet.id)

  assert.equal(generated.status, 'proposed')
  assert.equal(listed.length, 1)
  assert.equal(new Headers(calls[2].init?.headers).get('Idempotency-Key'), 'idem-three-tier')
  assert.deepEqual(JSON.parse(calls[2].init?.body as string), { expected_version: 1 })
  assert.equal(calls[4].url, '/api/delivery/v1/projects/project_1/change-sets/changeset_1/manual-action-package')
  assert.equal(calls[4].init?.method, 'POST')
  assert.equal(calls[5].url, '/api/delivery/v1/projects/project_1/change-sets/changeset_1/manual-action-package')
  assert.equal(reread.instructions[0]?.fieldKey, 'budget')
  assert.equal(rejected.status, 'rejected')
  assert.equal(manual.instructions[0]?.effectiveValue, 300000)
})

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
}

function planPayload() {
  return {
    id: 'plan_1', organization_id: 'org_1', project_id: 'project_1', status: 'draft', platform: 'ocean_engine_mock', source: 'mock', scenario: 'golden_path', current_version_number: 3,
    current_version: versionPayload(), versions: [versionPayload()], created_by: 'user_1', created_at: now, updated_at: now,
  }
}

function versionPayload() {
  return {
    plan_id: 'plan_1', organization_id: 'org_1', project_id: 'project_1', version_number: 3, canonical_hash: 'hash', platform: 'ocean_engine_mock',
    name: 'Mock plan', objective: 'Leads', advertiser: { id: 'advertiser_1', name: 'Mock advertiser', platform: 'ocean_engine', source: 'mock', scenario: 'golden_path' },
    budget: { total_minor: 300000, currency: 'CNY' }, schedule: { start_at: now, end_at: '2026-08-30T08:00:00.000Z', timezone: 'Asia/Shanghai' },
    tracking: { landing_page: 'https://demo.cookies.local', pixel_id: 'PX-1', conversion_event: 'lead_submit' }, creative_references: [], source_strategy_version: 'strategy-v1',
    source: 'mock', scenario: 'golden_path', created_by: { kind: 'user', id: 'user_1' }, created_at: now,
    three_tier_configuration: {
      schema: 'delivery-three-tier/v1', source: 'mock', scenario: 'golden_path', fixture_scenario: 'golden_path', generated_at: now, evidence_refs: ['mock://delivery-three-tier/v1'],
      groups: [{ id: 'group_1', name: 'Golden group', plans: [{ id: 'tier_plan_1', name: 'Plan 1', creatives: [{ id: 'creative_1', name: 'Creative 1', fields: [{
        key: 'budget', label: '预算', recommended: { type: 'integer', value: 300000 }, effective: { type: 'integer', value: 300000 }, source: 'mock_fixture', effective_source: 'recommended', source_refs: ['mock://strategy'], evidence_refs: ['mock://budget'], mock_required: true, platform_required: false, platform_status: 'not_requested', editable: true, confirmation: true,
      }] }] }] }],
    },
  }
}

function recommendationPayload() {
  return {
    id: 'recommendation_1', organization_id: 'org_1', project_id: 'project_1', plan_id: 'plan_1', plan_version: 3, fingerprint: 'fingerprint', base_snapshot_hash: 'base', target_snapshot_hash: 'target',
    target_snapshot: versionPayload().three_tier_configuration, evidence: ['mock://delivery-three-tier/v1'], action: 'apply_mock_three_tier_configuration', impact: 'deterministic mock recommendation', risks: [], observation: 'generated from immutable PlanVersion', provenance: 'plan_version', status: 'proposed', version: 1, created_by: 'user_1', created_at: now, updated_at: now,
  }
}

function changeSetPayload() {
  return {
    id: 'changeset_1', organization_id: 'org_1', project_id: 'project_1', plan_id: 'plan_1', plan_name: 'Mock plan', plan_version: 3, plan_canonical_hash: 'hash', budget_limit: { total_minor: 300000, currency: 'CNY' }, status: 'draft', risk_level: 'low', preflight_notes: [], source: 'mock', scenario: 'golden_path', version: 1, created_by: 'user_1', created_at: now, updated_at: now,
  }
}

function manualPackagePayload() {
  return {
    id: 'package_1', organization_id: 'org_1', project_id: 'project_1', change_set_id: 'changeset_1', target_snapshot_hash: 'target', content_hash: 'content',
    instructions: [{ group_id: 'group_1', plan_id: 'tier_plan_1', creative_id: 'creative_1', field_key: 'budget', effective: { type: 'integer', value: 300000 }, source: 'mock_fixture', confirmation_required: false, expected_result: 'set effective mock value', evidence_refs: ['mock://budget'] }],
    forbidden_actions: ['submit', 'platform_api_call', 'automatic_execution'], evidence: ['mock://budget'], provenance: 'approved_change_set', source: 'mock', scenario: 'manual_action_package', created_at: now,
  }
}
