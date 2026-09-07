import { expect, test, type APIRequestContext } from '@playwright/test'
import { createRuntimePlan } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'

test('missing execution capability never creates simulated success', async ({ request }) => {
  const suffix = Date.now().toString(36)
  const changeSet = await approvedChangeSet(request, suffix)
  const base = `/api/delivery/v1/projects/${projectId}`
  const before = await (await request.get(`${base}/executions`)).json()
  const approval = await (await request.get(`${base}/change-sets/${changeSet.id}`)).json()
  for (const scenario of ['success', 'failed', 'partial', 'result_unknown'] as const) {
    const response = await execute(request, changeSet, `unavailable-${suffix}-${scenario}`, scenario)
    expect(response.status()).toBe(503)
    expect(await response.json()).toMatchObject({ error: { code: 'EXECUTION_UNAVAILABLE', retryable: false } })
  }
  const direct = await request.post(`${base}/plans/${approval.plan_id}/execute`, {
    headers: { 'Idempotency-Key': `direct-${suffix}` }, data: { expected_version: approval.plan_version },
  })
  expect(direct.status()).toBe(503)
  expect(await direct.json()).toMatchObject({ error: { code: 'EXECUTION_UNAVAILABLE' } })
  expect(await (await request.get(`${base}/executions`)).json()).toEqual(before)
  expect(await (await request.get(`${base}/change-sets/${changeSet.id}`)).json()).toEqual(approval)
})

async function approvedChangeSet(request: APIRequestContext, suffix: string): Promise<ChangeSet> {
	const plan = await createRuntimePlan(request, projectId, suffix) as { id: string; version: number }

  const changeSetResponse = await request.post(`/api/delivery/v1/projects/${projectId}/plans/${plan.id}:create-change-set`, {
    data: { expected_version: plan.version },
  })
  expect(changeSetResponse.status()).toBe(201)
  let changeSet = await changeSetResponse.json() as ChangeSet

  const preflight = await request.post(`/api/delivery/v1/projects/${projectId}/change-sets/${changeSet.id}:preflight`, {
    data: { expected_version: changeSet.version },
  })
  expect(preflight.status()).toBe(200)
  changeSet = await preflight.json() as ChangeSet

  const approval = await request.post(`/api/delivery/v1/projects/${projectId}/change-sets/${changeSet.id}:approve`, {
    data: { expected_version: changeSet.version },
  })
  expect(approval.status()).toBe(200)
  return approval.json() as Promise<ChangeSet>
}

function execute(request: APIRequestContext, changeSet: ChangeSet, idempotencyKey: string, scenario: ExecutionScenario) {
  return request.post(`/api/delivery/v1/projects/${projectId}/change-sets/${changeSet.id}:execute`, {
    headers: { 'Idempotency-Key': idempotencyKey },
    data: { expected_version: changeSet.version, scenario },
  })
}

type ChangeSet = { id: string; version: number }
type ExecutionScenario = 'success' | 'failed' | 'partial' | 'result_unknown'
