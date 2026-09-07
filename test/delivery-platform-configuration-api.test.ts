import assert from 'node:assert/strict'
import test from 'node:test'
import { deliveryOptimizationApi } from '../src/api/delivery.ts'

const now = '2026-08-11T08:00:00.000Z'

test('decision workflow client freezes candidate selection at the Phase C write boundary', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    if (String(url).endsWith('/decisions')) return jsonResponse({ items: [decisionPayload()] })
    if (String(url).endsWith(':select')) return jsonResponse(selectionPayload())
    return jsonResponse(decisionPayload())
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const generated = await deliveryOptimizationApi.generateDecision('project_1', 'plan_1', 2)
  const listed = await deliveryOptimizationApi.listDecisions('project_1')
  const selection = await deliveryOptimizationApi.selectDecision('project_1', generated.id, 'balanced', 2, 'decision-selection-1')

  assert.equal(generated.diagnostic.code, 'ready')
  assert.equal(generated.candidates.length, 3)
  assert.equal(listed[0].recommendedCandidateId, 'balanced')
  assert.equal(selection.workflow.status, 'ready_for_final_approval')
  assert.equal(selection.workflow.remoteWriteEnabled, false)
  assert.equal(selection.workflow.steps.at(-1)?.blockReason, 'PHASE_C_REMOTE_WRITE_PROHIBITED')
  assert.equal(new Headers(calls[2].init?.headers).get('Idempotency-Key'), 'decision-selection-1')
  assert.deepEqual(JSON.parse(calls[2].init?.body as string), { candidate_id: 'balanced', expected_plan_version: 2 })
})

test('observatory client exposes deterministic replay evidence and immutable feedback', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    if (String(url).endsWith('/feedback')) return jsonResponse({ schema_version: 'delivery-observatory-feedback/v1', id: 'feedback_1', run_id: 'observatory_1', run_canonical_hash: 'f'.repeat(64), run_outcome: 'drift_detected', disposition: 'accepted', reason: 'reviewed', diff_keys: ['step.field'], canonical_hash: 'a'.repeat(64), created_by: 'user_1', created_at: now })
    if (String(url).endsWith('/observatory-runs')) return init?.method === 'POST' ? jsonResponse(observatoryRunPayload()) : jsonResponse({ items: [observatoryRunPayload()] })
    return jsonResponse(observatoryRunPayload())
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const run = await deliveryOptimizationApi.runObservatory('project_1', 'selection_1', 'observe_existing')
  const listed = await deliveryOptimizationApi.listObservatoryRuns('project_1')
  const feedback = await deliveryOptimizationApi.submitObservatoryFeedback('project_1', run.id, 'accepted', 'reviewed', ['step.field'], 'feedback-key-1')

  assert.equal(run.remoteWriteEnabled, false)
  assert.equal(run.steps.at(-1)?.blockReason, 'PHASE_C_REMOTE_WRITE_PROHIBITED')
  assert.equal(listed[0].binding.workflowCanonicalHash, 'e'.repeat(64))
  assert.equal(feedback.disposition, 'accepted')
  assert.equal(new Headers(calls[2].init?.headers).get('Idempotency-Key'), 'feedback-key-1')
})

test('historical recommendation reads preserve v2 snapshots', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    const path = String(url)
    if (path.endsWith('/recommendations')) return jsonResponse({ items: [recommendationPayload()], source: 'mock' })
    return jsonResponse(recommendationPayload())
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const listed = await deliveryOptimizationApi.listRecommendations('project_1')
  const restored = await deliveryOptimizationApi.getRecommendation('project_1', 'recommendation_1')
  assert.equal(listed.length, 1)
  assert.equal(restored.id, listed[0].id)

})

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
}

function configuration() {
  return {
    schema_version: 'delivery-platform-configuration/v2', configuration_id: 'configuration_1', version_number: 2,
    platform: 'ocean_engine', profile_version: 'oceanengine-configuration/v1', canonical_hash: 'a'.repeat(64),
    hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)', intent: {}, payload: { profile: 'ocean_engine', ocean_engine: { profile: 'ocean_engine', project: {}, promotions: [] } },
    configuration_provenance: {}, fact_provenance: {}, compilation_metadata: {},
  }
}

function recommendationPayload() {
  return {
    id: 'recommendation_1', organization_id: 'org_1', project_id: 'project_1', plan_id: 'plan_1', plan_version: 2,
    simulation_run_id: 'simulation_1', fingerprint: 'fingerprint', base_snapshot_hash: 'a'.repeat(64), target_snapshot_hash: 'a'.repeat(64),
    base_configuration: configuration(), target_configuration: configuration(), runtime_status: 'active', read_only: false,
    evidence: ['simulation://run/1'], action: 'reduce_budget_10_percent', impact: 'reviewed budget reduction', risks: [], observation: 'measured evidence',
    provenance: 'post-launch-simulator/v1', status: 'proposed', version: 1, created_by: 'user_1', created_at: now, updated_at: now,
  }
}

function decisionPayload() {
  const candidate = (kind: 'conservative' | 'balanced' | 'exploratory') => ({
    id: kind, kind, target_configuration: configuration(), budget_change_percent: -10, rationale: ['policy'], constraints: [{ code: 'SAFE', passed: true, explanation: 'safe' }], risks: [], uncertainty: kind === 'conservative' ? 'low' : kind === 'balanced' ? 'medium' : 'high',
  })
  return {
    schema_version: 'delivery-decision/v1', id: 'decision_1', organization_id: 'org_1', project_id: 'project_1', policy_version: 'delivery-decision-policy/v1',
    diagnostic: { code: 'ready', explanation: 'facts ready', next_action: 'select' },
    inputs: { plan_id: 'plan_1', plan_version: 2, plan_canonical_hash: 'a'.repeat(64), intent_canonical_hash: 'b'.repeat(64), configuration_canonical_hash: 'c'.repeat(64), fact_snapshot_ref: 'mock://facts/1', simulation_run_id: 'simulation_1', simulation_input_hash: 'd'.repeat(64) },
    candidates: [candidate('conservative'), candidate('balanced'), candidate('exploratory')], recommended_candidate_id: 'balanced', evidence: ['simulation://run/1'], canonical_hash: 'e'.repeat(64), created_by: 'user_1', created_at: now,
  }
}

function selectionPayload() {
  return {
    id: 'selection_1', decision_id: 'decision_1', decision_canonical_hash: 'e'.repeat(64), candidate_id: 'balanced', configuration: configuration(),
    workflow: {
      schema_version: 'compiled-delivery-workflow/v1', id: 'workflow_1', decision_id: 'decision_1', decision_canonical_hash: 'e'.repeat(64), selected_candidate_id: 'balanced', configuration_canonical_hash: 'a'.repeat(64),
      configuration_id: 'configuration_1', configuration_version: 2, platform: 'ocean_engine', profile_version: 'oceanengine-configuration/v1', account_reference: { namespace: 'cookies', object_kind: 'advertiser_account', scope: 'project:project_1', id: 'account_1', state: 'resolved' },
      capability_contract_version: 'oceanengine-capability/v0.1', selector_contract_version: 'oceanengine-selector-contract/v0.1', action_contract_version: 'oceanengine-action-contract/v0.1', compiler_version: 'oceanengine-workflow-compiler/v1', status: 'ready_for_final_approval', remote_write_enabled: false,
      steps: [{ id: 'submit', sequence: 1, page: 'review', action: 'submit', risk: 'remote_write', preconditions: [], fields: [], timeout_seconds: 0, recovery: 'not executable', blocked: true, block_reason: 'PHASE_C_REMOTE_WRITE_PROHIBITED' }], canonical_hash: 'f'.repeat(64), created_at: now,
    },
    final_approval_binding: { status: 'ready_for_final_approval', action: 'remote_write', plan_canonical_hash: 'a'.repeat(64), intent_canonical_hash: 'b'.repeat(64), decision_canonical_hash: 'e'.repeat(64), configuration_canonical_hash: 'a'.repeat(64), workflow_canonical_hash: 'f'.repeat(64) }, created_at: now,
  }
}

function observatoryRunPayload() {
  return {
    schema_version: 'delivery-observatory-run/v1', id: 'observatory_1', organization_id: 'org_1', project_id: 'project_1', runner_version: 'mock-replay-observatory-runner/v1', source: 'replay', mode: 'observe_existing', input_hash: 'a'.repeat(64),
    binding: { selection_id: 'selection_1', decision_id: 'decision_1', decision_canonical_hash: 'b'.repeat(64), configuration_id: 'configuration_1', configuration_version: 2, configuration_canonical_hash: 'c'.repeat(64), workflow_id: 'workflow_1', workflow_canonical_hash: 'e'.repeat(64), decision_schema_version: 'delivery-decision/v1', configuration_schema_version: 'delivery-platform-configuration/v2', workflow_schema_version: 'compiled-delivery-workflow/v1' },
    data_state: 'ready', data_state_reason: '', observed_at: now, data_through: now, status: 'completed', outcome: 'drift_detected', remote_write_enabled: false,
    steps: [{ step_id: 'remote-write-boundary', sequence: 1, page: 'review', workflow_action: 'submit_platform_configuration', executed_action: 'observe', status: 'blocked', selector_matches: [], evidence_refs: [], page_refs: [], diffs: [], block_reason: 'PHASE_C_REMOTE_WRITE_PROHIBITED' }],
    evidence_refs: ['replay://fixture/1'], page_refs: [], canonical_hash: 'f'.repeat(64), created_by: 'user_1', created_at: now,
  }
}
