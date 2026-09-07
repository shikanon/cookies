import { expect, test, type APIRequestContext } from '@playwright/test'
import { createRuntimePlan, deliveryRuntimePayload } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'
const base = `/api/delivery/v1/projects/${projectId}`

test('historical approval snapshots stay immutable and readable after demo retirement', async ({ page, request }) => {
  const suffix = `approval-${Date.now().toString(36)}`
  const plan = await createRuntimePlan(request, projectId, suffix)
  const approved = await approveFixture(request, plan.id, plan.version)
  expect(approved.approval).toMatchObject({ valid: true, plan_version: 1, plan_canonical_hash: plan.current_version.canonical_hash, scope: 'execute_mock' })
  expect(Date.parse(approved.approval.expires_at) - Date.parse(approved.approval.approved_at)).toBe(24 * 60 * 60 * 1000)
  await page.goto(`/projects/${projectId}/delivery/approvals/${approved.id}?tour_run_id=old-run&tour_case=golden_path`)
  await expect(page.getByText('此页仅查看历史审批；演示授权不适用于真实平台。')).toBeVisible()
  await expect(page.getByText(approved.approval.plan_canonical_hash.slice(0, 12), { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: /批准平台操作演练|启动平台操作演练|打回修改/ })).toHaveCount(0)
  await expect(page.getByRole('link', { name: '进入受控执行中心' })).toHaveAttribute('href', `/projects/${projectId}/delivery/execution`)

  const payload = deliveryRuntimePayload(projectId, suffix, 2)
  payload.intent.payload.budget_boundary.maximum_total_minor = 420000
  const updatedResponse = await request.patch(`${base}/plans/${plan.id}`, { data: payload })
  expect(updatedResponse.status()).toBe(200)
  const updated = await updatedResponse.json()
  expect(updated.current_version.canonical_hash).not.toBe(plan.current_version.canonical_hash)
  await page.reload()
  await expect(page.getByText('旧审批已失效', { exact: true })).toBeVisible()
  await expect(page.getByText('计划已产生新版本，旧版本审批永久失效。', { exact: true })).toBeVisible()
  const oldAudit = await (await request.get(`${base}/change-sets/${approved.id}`)).json()
  expect(oldAudit.approval).toMatchObject({ approval_id: approved.approval.approval_id, action_hash: approved.approval.action_hash, plan_canonical_hash: approved.approval.plan_canonical_hash, valid: false, invalid_reason: 'STALE_PLAN_VERSION' })
  const stale = await request.post(`${base}/change-sets/${approved.id}:execute`, { headers: { 'Idempotency-Key': `stale-${suffix}` }, data: { expected_version: approved.version, scenario: 'success' } })
  expect(stale.status()).toBe(409)
  expect(await stale.json()).toMatchObject({ error: { code: 'STALE_PLAN_VERSION' } })
  expect((await request.get(`/api/delivery/v1/projects/project_local/change-sets/${approved.id}`)).status()).toBe(404)
})

async function approveFixture(request: APIRequestContext, planId: string, version: number) {
  const created = await request.post(`${base}/plans/${planId}:create-change-set`, { data: { expected_version: version } })
  expect(created.status()).toBe(201)
  let changeSet = await created.json()
  for (const action of ['preflight', 'approve']) {
    const response = await request.post(`${base}/change-sets/${changeSet.id}:${action}`, { data: { expected_version: changeSet.version } })
    expect(response.status()).toBe(200)
    changeSet = await response.json()
  }
  return changeSet
}
