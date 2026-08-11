import { expect, test, type APIRequestContext } from '@playwright/test'
import { createRuntimePlan } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'
const otherProjectId = 'project_local'
const alertTypes = ['review_rejected', 'spend_spike', 'zero_conversion', 'cost_worsening', 'under_delivery', 'creative_fatigue', 'tracking_anomaly'] as const

test('Delivery monitoring evaluates fixtures, preserves provenance, paginates, and resolves open alerts optimistically', async ({ request }) => {
  const executionId = await createMonitoringMetricSnapshot(request)
  const normal = await evaluate(request, 'normal_day', executionId)
  expect(normal.status()).toBe(200)
  const normalBody = await normal.json() as AlertEvaluationResult
  expect(normalBody).toMatchObject({
    source: 'post_launch_simulator',
    is_simulated: true,
    scenario: 'normal_day',
  })
  expect(normalBody.items.map(item => item.type)).toEqual(['cost_worsening'])

  await runOutcomeSimulation(request, executionId, 'review_rejected', 'review-seed')
  const anomalous = await evaluate(request, 'anomaly_day', executionId)
  expect(anomalous.status()).toBe(200)
  const anomaly = await anomalous.json() as AlertEvaluationResult
  expect(anomaly).toMatchObject({
    source: 'post_launch_simulator',
    is_simulated: true,
    scenario: 'anomaly_day',
  })
  expect(anomaly.created_count + anomaly.reused_count).toBe(anomaly.items.length)
  expect(anomaly.items.map(item => item.type)).toEqual(['review_rejected'])

  await runOutcomeSimulation(request, executionId, 'tracking_anomaly', 'tracking-seed')
  const trackingEvaluation = await evaluate(request, 'normal_day', executionId)
  expect(trackingEvaluation.status()).toBe(200)
  const tracking = await trackingEvaluation.json() as AlertEvaluationResult
  expect(tracking.items.map(item => item.type).sort()).toEqual(['cost_worsening', 'tracking_anomaly', 'zero_conversion'])

  for (const alert of [...anomaly.items, ...tracking.items]) {
    expect(alert).toMatchObject({
      organization_id: expect.any(String),
      project_id: projectId,
      plan_id: expect.any(String),
      execution_id: expect.any(String),
      monitored_entity: { type: 'delivery_plan', id: alert.plan_id, advertiser_id: expect.any(String) },
      rule_id: expect.any(String),
      rule_version: 'v3',
      fingerprint: expect.any(String),
      status: 'open',
      version: expect.any(Number),
      source: 'post_launch_simulator',
      is_simulated: true,
      simulation_run_id: expect.any(String),
      scenario: alert.type === 'review_rejected' ? 'anomaly_day' : 'normal_day',
      dataset_version: expect.stringMatching(/^post-launch-simulator\/v1\/delivery-outcome-scenario\/v1\/[a-f0-9]{12}$/),
      fixture_version: expect.stringMatching(/^post-launch-simulator\/v1\/delivery-outcome-scenario\/v1\/[a-f0-9]{12}$/),
      owner: { source: 'workflow_context' },
      evidence_refs: expect.arrayContaining([expect.any(String)]),
      freshness: { status: 'fresh', as_of: expect.any(String), evaluated_at: expect.any(String) },
    })
    expect(alert.window).toMatchObject({ start: expect.any(String), end: expect.any(String), timezone: expect.any(String), data_through: expect.any(String) })
    expect(alert.metric_definition).toMatchObject({ name: expect.any(String), unit: expect.any(String) })
  }
  const byType = Object.fromEntries(tracking.items.map(item => [item.type, item]))
  expect(byType.zero_conversion.metric_definition).toMatchObject({ observed_value: 0, numerator: 0, threshold: 1 })
  expect(byType.cost_worsening.metric_definition.observed_value).toBeGreaterThan(byType.cost_worsening.metric_definition.baseline_value!)

  const firstPage = await request.get(alertsURL('?status=open&fixture=normal_day&limit=2'))
  expect(firstPage.status()).toBe(200)
  const pageOne = await firstPage.json() as AlertList
  expect(pageOne).toMatchObject({ source: 'post_launch_simulator', is_simulated: true })
  expect(pageOne.items).toHaveLength(2)
  expect(pageOne.next_cursor).toEqual(expect.any(String))
  const secondPage = await request.get(alertsURL(`?status=open&fixture=normal_day&limit=2&cursor=${encodeURIComponent(pageOne.next_cursor!)}`))
  expect(secondPage.status()).toBe(200)
  const pageTwo = await secondPage.json() as AlertList
  expect(pageTwo.items.length).toBeGreaterThan(0)
  expect(pageTwo.items.length).toBeLessThanOrEqual(2)
  expect(new Set([...pageOne.items, ...pageTwo.items].map(item => item.id)).size).toBe(pageOne.items.length + pageTwo.items.length)

  const typed = await request.get(alertsURL('?type=tracking_anomaly&severity=critical&limit=10'))
  expect(typed.status()).toBe(200)
  const typedBody = await typed.json() as AlertList
  expect(typedBody.items).toEqual(expect.arrayContaining([
    expect.objectContaining({ type: 'tracking_anomaly', severity: 'critical' }),
  ]))

  const toAcknowledge = anomaly.items[0]
  const isolatedRead = await request.get(`/api/delivery/v1/projects/${otherProjectId}/alerts?limit=10`)
  expect(isolatedRead.status()).toBe(200)
  expect((await isolatedRead.json() as AlertList).items.map(item => item.id)).not.toContain(toAcknowledge.id)
  const isolatedEvaluation = await request.post(`/api/delivery/v1/projects/${otherProjectId}/alerts:evaluate`, { data: { fixture: 'anomaly_day' } })
  expect(isolatedEvaluation.status()).toBe(200)
  expect((await isolatedEvaluation.json() as AlertEvaluationResult).items).toEqual([])
  const crossProjectPatch = await request.patch(`/api/delivery/v1/projects/${otherProjectId}/alerts/${toAcknowledge.id}`, { data: { action: 'dismiss', expected_version: toAcknowledge.version } })
  expect(crossProjectPatch.status()).toBe(404)
  expect(await crossProjectPatch.json()).toMatchObject({ error: { code: 'RESOURCE_NOT_FOUND' } })
  const acknowledged = await update(request, toAcknowledge.id, 'acknowledge', toAcknowledge.version)
  expect(acknowledged.status()).toBe(200)
  const acknowledgedBody = await acknowledged.json() as DeliveryMonitoringAlert
  expect(acknowledgedBody).toMatchObject({ id: toAcknowledge.id, status: 'acknowledged', version: toAcknowledge.version + 1 })

  const staleUpdate = await update(request, toAcknowledge.id, 'dismiss', toAcknowledge.version)
  expect(staleUpdate.status()).toBe(409)
  expect(await staleUpdate.json()).toMatchObject({ error: { code: 'VERSION_CONFLICT' } })

  const toDismiss = tracking.items[0]
  const dismissed = await update(request, toDismiss.id, 'dismiss', toDismiss.version)
  expect(dismissed.status()).toBe(200)
  expect(await dismissed.json()).toMatchObject({ id: toDismiss.id, status: 'dismissed' })

  for (const fixture of ['stale_data', 'insufficient_data'] as const) {
    const response = await evaluate(request, fixture, executionId)
    expect(response.status()).toBe(200)
    const result = await response.json() as AlertEvaluationResult
    expect(result).toMatchObject({ source: 'post_launch_simulator', is_simulated: true, scenario: fixture })
    // Freshness-only fixtures never manufacture a fifth alert class. They leave
    // the four alert classifications reserved for actionable anomaly evidence.
    expect(result.items).toEqual([])
  }
})

function evaluate(request: APIRequestContext, fixture: MonitoringFixture, executionId?: string) {
  return request.post(`/api/delivery/v1/projects/${projectId}/alerts:evaluate`, { data: { fixture, execution_id: executionId } })
}

function update(request: APIRequestContext, alertId: string, action: 'acknowledge' | 'dismiss', expectedVersion: number) {
  return request.patch(`/api/delivery/v1/projects/${projectId}/alerts/${alertId}`, { data: { action, expected_version: expectedVersion } })
}

function alertsURL(query = '') {
  return `/api/delivery/v1/projects/${projectId}/alerts${query}`
}

async function createMonitoringMetricSnapshot(request: APIRequestContext) {
  const suffix = `monitoring-${Date.now().toString(36)}`
	const plan = await createRuntimePlan(request, projectId, suffix) as { id: string; version: number }

  const changeSetResponse = await request.post(`/api/delivery/v1/projects/${projectId}/plans/${plan.id}:create-change-set`, { data: { expected_version: plan.version } })
  expect(changeSetResponse.status()).toBe(201)
  let changeSet = await changeSetResponse.json() as { id: string; version: number }
  const preflight = await request.post(`/api/delivery/v1/projects/${projectId}/change-sets/${changeSet.id}:preflight`, { data: { expected_version: changeSet.version } })
  expect(preflight.status()).toBe(200)
  changeSet = await preflight.json() as { id: string; version: number }
  const approval = await request.post(`/api/delivery/v1/projects/${projectId}/change-sets/${changeSet.id}:approve`, { data: { expected_version: changeSet.version } })
  expect(approval.status()).toBe(200)
  changeSet = await approval.json() as { id: string; version: number }
  const execution = await request.post(`/api/delivery/v1/projects/${projectId}/change-sets/${changeSet.id}:execute`, {
    headers: { 'Idempotency-Key': `monitoring-${suffix}` },
    data: { expected_version: changeSet.version, scenario: 'success' },
  })
  expect(execution.status()).toBe(201)
  const executionBody = await execution.json() as { execution: { id: string } }
  await runOutcomeSimulation(request, executionBody.execution.id, 'cost_pressure', suffix)
  return executionBody.execution.id
}

async function runOutcomeSimulation(request: APIRequestContext, executionId: string, scenario: string, stableSeed: string) {
  const response = await request.post(`/api/delivery/v1/projects/${projectId}/executions/${executionId}/simulation-runs`, {
    data: { scenario, stable_seed: stableSeed },
  })
  expect([200, 201]).toContain(response.status())
  return response
}

type MonitoringFixture = 'normal_day' | 'anomaly_day' | 'stale_data' | 'insufficient_data'

type DeliveryMonitoringAlert = {
  id: string
  type: typeof alertTypes[number]
  plan_id: string
  version: number
  status: 'open' | 'acknowledged' | 'dismissed'
  severity: 'critical' | 'high' | 'medium' | 'low'
  window: { start: string; end: string; timezone: string; data_through: string }
  metric_definition: { name: string; unit: string; observed_value?: number; baseline_value?: number; threshold?: number; numerator?: number; denominator?: number }
  freshness: { status: 'fresh' | 'stale' | 'unknown' | 'insufficient_data'; as_of: string; evaluated_at: string }
}

type AlertEvaluationResult = {
  items: DeliveryMonitoringAlert[]
  created_count: number
  reused_count: number
}

type AlertList = { items: DeliveryMonitoringAlert[]; next_cursor: string | null }
