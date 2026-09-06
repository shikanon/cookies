import assert from 'node:assert/strict'
import test from 'node:test'
import { deliveryExecutionApi } from '../src/api/delivery.ts'

const now = '2026-08-03T08:00:00.000Z'

test('delivery execution client starts a real Browser RPA run from one plan version', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    return jsonResponse({ controlled_change_set: { id: 'controlled_1' }, controlled_execution: { id: 'execution_1' }, browser_rpa_run: { run_id: 'curun_1' } })
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const result = await deliveryExecutionApi.startBrowserRpaExecution('project_1', 'plan_1', 3, 'playwright-rpa/edge/v3', 'real-run-1')

  assert.equal(calls[0].url, '/api/delivery/v1/projects/project_1/plans/plan_1/browser-rpa-runs')
  assert.equal(calls[0].init?.method, 'POST')
  assert.equal(new Headers(calls[0].init?.headers).get('Idempotency-Key'), 'real-run-1')
  assert.deepEqual(JSON.parse(calls[0].init?.body as string), { expected_version: 3, execution_driver: 'playwright-rpa/edge/v3' })
  assert.equal(result.browser_rpa_run.run_id, 'curun_1')
})

test('delivery execution client sends the frozen idempotent execute request', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    return jsonResponse(sampleExecutionRecord())
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const record = await deliveryExecutionApi.execute('project_1', 'changeset_1', 4, 'partial', 'idem-123')

  assert.equal(calls[0].url, '/api/delivery/v1/projects/project_1/change-sets/changeset_1:execute')
  assert.equal(calls[0].init?.method, 'POST')
  assert.equal(new Headers(calls[0].init?.headers).get('Idempotency-Key'), 'idem-123')
  assert.deepEqual(JSON.parse(calls[0].init?.body as string), { expected_version: 4, scenario: 'partial' })
  assert.equal(record.execution.status, 'partial')
  assert.deepEqual(record.execution.compensationCandidates, ['remove_mock_delivery'])
  assert.deepEqual(record.evidence.references, ['mock://execution/partial'])
})

test('delivery execution client reloads authoritative list and detail records with nullable arrays', async t => {
  const originalFetch = globalThis.fetch
  const urls: string[] = []
  globalThis.fetch = async url => {
    urls.push(String(url))
    const record = sampleExecutionRecord()
    record.execution.steps = null
    record.execution.compensation_candidates = null
    record.evidence.references = null
    return jsonResponse(String(url).endsWith('/executions') ? { items: [record], source: 'mock', scenario: 'execution_list' } : record)
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const listed = await deliveryExecutionApi.list('project_1')
  const detail = await deliveryExecutionApi.get('project_1', 'execution_1')

  assert.deepEqual(urls, [
    '/api/delivery/v1/projects/project_1/executions',
    '/api/delivery/v1/projects/project_1/executions/execution_1',
  ])
  assert.deepEqual(listed[0].execution.steps, [])
  assert.deepEqual(detail.execution.compensationCandidates, [])
  assert.deepEqual(detail.evidence.references, [])
})

test('delivery execution client runs and reads the same deterministic outcome simulation', async t => {
  const originalFetch = globalThis.fetch
  const calls: Array<{ url: string; init?: RequestInit }> = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init })
    return jsonResponse(sampleOutcomeSimulation())
  }
  t.after(() => { globalThis.fetch = originalFetch })

  const created = await deliveryExecutionApi.runOutcomeSimulation('project_1', 'execution_1', 'cost_pressure', 'stable-1')
  const restored = await deliveryExecutionApi.getLatestOutcomeSimulation('project_1', 'execution_1')

  assert.equal(calls[0].url, '/api/delivery/v1/projects/project_1/executions/execution_1/simulation-runs')
  assert.equal(calls[0].init?.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init?.body as string), { scenario: 'cost_pressure', stable_seed: 'stable-1' })
  assert.equal(calls[1].url, '/api/delivery/v1/projects/project_1/executions/execution_1/simulation-run')
  assert.equal(created.run.modelVersion, 'delivery-outcome-scenario/v1')
  assert.equal(created.run.parameters.factors[0].valueBP, 10000)
  assert.equal(created.metricSnapshots[0].conversions, 12)
  assert.equal(restored.run.id, created.run.id)
})

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
}

function sampleExecutionRecord() {
  return {
    change_set: {
      id: 'changeset_1', organization_id: 'org_1', project_id: 'project_1', plan_id: 'plan_1', plan_name: 'Mock plan', plan_version: 2,
      plan_canonical_hash: 'plan-hash', budget_limit: { total_minor: 250000, currency: 'CNY' }, status: 'approved', risk_level: 'low',
      preflight_notes: [], approved_by: 'user_1', approved_at: now, source: 'mock', scenario: 'approval_queue', version: 4,
      created_by: 'user_1', created_at: now, updated_at: now,
    },
    execution: {
      id: 'execution_1', organization_id: 'org_1', project_id: 'project_1', change_set_id: 'changeset_1', approval_id: 'approval_1',
      status: 'partial', version: 1, mode: 'local_simulation', adapter: 'mock_ocean_engine', source: 'mock', scenario: 'partial',
      idempotency_key: 'idem-123', request_hash: 'request-hash', executed_by: 'user_1', started_at: now, completed_at: now,
      retry_allowed: false, recovery_action: 'review_and_compensate', recovery_reason: 'some target effects completed',
      compensation_candidates: ['remove_mock_delivery'],
      steps: [{
        id: 'step_1', sequence: 1, action: 'apply_mock_delivery', status: 'succeeded', attempt: 1, effect: 'confirmed_applied',
        outcome_summary: 'fixture applied', evidence_ref: 'mock://execution/partial', started_at: now, completed_at: now, version: 1,
      }],
    },
    evidence: {
      id: 'evidence_1', execution_id: 'execution_1', summary: 'redacted mock evidence', source: 'mock', scenario: 'partial',
      references: ['mock://execution/partial'],
    },
  }
}

function sampleOutcomeSimulation() {
  return {
    run: {
      id: 'simulation_1', execution_id: 'execution_1', plan_id: 'plan_1', plan_version: 2,
      model_version: 'delivery-outcome-scenario/v1', scenario: 'cost_pressure', stable_seed: 'stable-1', input_hash: 'input-hash', status: 'completed',
      input: {
        budget: { total_minor: 250000 }, schedule: { start_at: now, end_at: now }, optimization_goal: 'conversion', bid_minor: 2500,
        audience: 'high-intent', strategy_reference: { version: 2 }, creative_features: [{ asset_id: 'asset_1' }],
      },
      parameters: {
        base_cpm_minor: 2800, base_ctr_bp: 480, base_cvr_bp: 520, daily_budget_minor: 25000,
        factors: [{ key: 'budget', value_bp: 10000, explanation: 'budget changes scale', evidence: ['plan://budget'] }],
      },
      events: [{ type: 'cost_worsening', severity: 'high', window_sequence: 3, explanation: 'CPA worsened', evidence: ['simulation://metric-window/1'] }],
      evidence: ['simulation://execution/execution_1'], completed_at: now,
    },
    metric_snapshots: [{
      id: 'metric_1', simulation_run_id: 'simulation_1', window_sequence: 1, window_start: now, window_end: now,
      raw_metrics: { impressions: 1000, clicks: 50, conversions: 12, spend_cents: 12000, revenue_cents: 24000 },
      calculation_basis: { formula: 'spend to outcomes', spend_multiplier_bp: 7000, reach_multiplier_bp: 10000, ctr_multiplier_bp: 10000, cvr_multiplier_bp: 10000, tracking_rate_bp: 10000 },
    }],
    replay: false,
  }
}
