import { expect, test, type APIRequestContext } from '@playwright/test'
import { createRuntimePlan } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'
const otherProjectId = 'project_local'

test('Delivery execution scenarios persist steps, enforce idempotency, and preserve Project isolation', async ({ page, request }) => {
  const suffix = Date.now().toString(36)
  const success = await approvedChangeSet(request, `${suffix}-success`)
  const key = `execution-${suffix}-success`

  const created = await execute(request, success, key, 'success')
  expect(created.status()).toBe(201)
  const createdBody = await created.json() as ExecutionResult
  expect(createdBody).toMatchObject({
    change_set: { id: success.id },
    execution: {
      change_set_id: success.id,
      status: 'succeeded',
      mode: 'local_simulation',
      adapter: 'mock_ocean_engine',
      source: 'mock',
      scenario: 'success',
      idempotency_key: key,
      retry_allowed: false,
      recovery_action: 'none',
    },
    evidence: { source: 'mock', scenario: 'success' },
  })
  expect(createdBody.execution.request_hash).toMatch(/^[0-9a-f]{64}$/)
  expect(createdBody.execution.steps).toEqual(expect.arrayContaining([
    expect.objectContaining({ status: 'succeeded', evidence_ref: expect.any(String) }),
  ]))

  const replay = await execute(request, success, key, 'success')
  expect(replay.status()).toBe(200)
  expect((await replay.json() as ExecutionResult).execution.id).toBe(createdBody.execution.id)

  const conflict = await execute(request, success, key, 'failed')
  expect(conflict.status()).toBe(409)
  expect(await conflict.json()).toMatchObject({
    error: { code: 'IDEMPOTENCY_CONFLICT' },
    source: 'mock',
  })

  const detail = await request.get(executionURL(projectId, createdBody.execution.id))
  expect(detail.status()).toBe(200)
  expect(await detail.json()).toMatchObject({ execution: { id: createdBody.execution.id, source: 'mock', scenario: 'success' } })

  // The A04 panel must recover durable server state after a full browser refresh.
  const executionIdSuffix = createdBody.execution.id.slice(-12)
  let detailReads = 0
  await page.route(`**${executionURL(projectId, createdBody.execution.id)}`, async route => {
    const response = await route.fetch()
    const body = await response.json() as ExecutionResult
    detailReads += 1
    if (detailReads > 1) {
      body.execution.status = 'result_unknown'
      body.execution.retry_allowed = false
      body.execution.recovery_action = 'query_and_reconcile'
      body.execution.recovery_reason = 'refreshed authoritative recovery decision'
    }
    await route.fulfill({ response, json: body })
  })
  await page.goto(`/projects/${projectId}/delivery/approvals`)
  const executionPanel = page.getByRole('region', { name: '平台操作演练记录' })
  await expect(executionPanel).toBeVisible()
  await expect(executionPanel.getByText(executionIdSuffix, { exact: true }).first()).toBeVisible()
  await expect(executionPanel.locator('.execution-detail-heading h4')).toHaveText('succeeded')
  await executionPanel.getByRole('button', { name: '从 Go API 刷新' }).click()
  await expect(executionPanel.locator('.execution-detail-heading h4')).toHaveText('result_unknown')
  await expect(executionPanel.getByText('refreshed authoritative recovery decision').first()).toBeVisible()
  await page.reload()
  await expect(executionPanel).toBeVisible()
  await expect(executionPanel.getByText(executionIdSuffix, { exact: true }).first()).toBeVisible()
  const afterRefresh = await request.get(executionURL(projectId, createdBody.execution.id))
  expect(afterRefresh.status()).toBe(200)
  expect((await afterRefresh.json() as ExecutionResult).execution.steps).toEqual(createdBody.execution.steps)

  const list = await request.get(`/api/delivery/v1/projects/${projectId}/executions`)
  expect(list.status()).toBe(200)
  const listed = await list.json() as { source: string; scenario: string; items: ExecutionResult[] }
  expect(listed).toMatchObject({
    source: 'mock',
    scenario: 'execution_list',
  })
  expect(listed.items).toEqual(expect.arrayContaining([
    expect.objectContaining({ execution: expect.objectContaining({ id: createdBody.execution.id }) }),
  ]))

  const crossProjectRead = await request.get(executionURL(otherProjectId, createdBody.execution.id))
  expect(crossProjectRead.status()).toBe(404)
  expect(await crossProjectRead.json()).toMatchObject({ error: { code: 'RESOURCE_NOT_FOUND' }, source: 'mock' })

  for (const scenario of ['failed', 'partial', 'result_unknown'] as const) {
    const changeSet = await approvedChangeSet(request, `${suffix}-${scenario}`)
    const response = await execute(request, changeSet, `execution-${suffix}-${scenario}`, scenario)
    expect(response.status()).toBe(201)
    const body = await response.json() as ExecutionResult
    expect(body).toMatchObject({
      execution: { source: 'mock', scenario, status: scenario === 'failed' ? 'failed' : scenario },
      evidence: { source: 'mock', scenario },
    })
    if (scenario === 'partial') {
      expect(body.execution.compensation_candidates.length).toBeGreaterThan(0)
      expect(body.execution.steps).toEqual(expect.arrayContaining([
        expect.objectContaining({ status: 'succeeded' }),
        expect.objectContaining({ status: 'failed' }),
      ]))
    }
    if (scenario === 'result_unknown') {
      expect(body.execution).toMatchObject({ retry_allowed: false, recovery_action: 'query_and_reconcile' })
      expect(body.execution.steps).toEqual(expect.arrayContaining([
        expect.objectContaining({ status: 'result_unknown' }),
      ]))
    }
    if (scenario === 'failed') {
      expect(body.execution.steps).toEqual(expect.arrayContaining([
        expect.objectContaining({ status: 'failed' }),
      ]))
    }
    const rollback = await request.post(`/api/delivery/v1/projects/${projectId}/change-sets/${changeSet.id}:rollback`, {
      data: { expected_version: body.change_set.version },
    })
    expect(rollback.status()).toBe(409)
    expect(await rollback.json()).toMatchObject({ error: { code: 'INVALID_STATE' }, source: 'mock' })
  }
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

function executionURL(targetProjectId: string, executionId: string) {
  return `/api/delivery/v1/projects/${targetProjectId}/executions/${executionId}`
}

type ExecutionScenario = 'success' | 'failed' | 'partial' | 'result_unknown'

type ChangeSet = { id: string; version: number }

type ExecutionResult = {
  change_set: ChangeSet
  execution: {
    id: string
    status: string
    request_hash: string
    steps: Array<{ status: string; evidence_ref: string }>
    compensation_candidates: string[]
    retry_allowed: boolean
    recovery_action: string
    recovery_reason: string
  }
  evidence: {
    source: string
    scenario: ExecutionScenario
  }
}
