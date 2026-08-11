import { expect, test, type APIRequestContext } from '@playwright/test'
import { createRuntimePlan } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'

test('complete delivery mock tour is repeatable, resumable, observable, and safely reset', async ({ page, request }) => {
  test.setTimeout(120_000)
  const suffix = Date.now().toString(36)
  const runId = `delivery-tour-${suffix}`
  const otherRunId = `delivery-tour-other-${suffix}`
  const sentinel = await createOrdinaryPlan(request, `Ordinary plan ${suffix}`)

  const preparedResponse = await request.post(tourURL(runId, 'prepare'))
  expect(preparedResponse.status()).toBe(201)
  const prepared = await preparedResponse.json() as TourRun
  expectTourContract(prepared, runId)
  expect(prepared.current_step).toBe('first_approval')
  expect(Object.fromEntries(prepared.cases.filter(tourCase => tourCase.key !== 'golden_path').map(tourCase => [tourCase.key, tourCase.status]))).toEqual({
    preflight_failure: 'observed',
    approval_expired: 'observed',
    plan_stale: 'observed',
    partial_execution: 'observed',
    result_unknown: 'observed',
    review_rejected_alert: 'prepared',
  })
  const originalPlanIDs = Object.fromEntries(prepared.cases.map(tourCase => [tourCase.key, tourCase.plan_id]))

  const replayResponse = await request.post(tourURL(runId, 'prepare'))
  expect(replayResponse.status()).toBe(200)
  const replay = await replayResponse.json() as TourRun
  expect(Object.fromEntries(replay.cases.map(tourCase => [tourCase.key, tourCase.plan_id]))).toEqual(originalPlanIDs)

  await page.goto(`/projects/${projectId}/delivery/tour?tour_run_id=${runId}`)
  await expect(page.getByRole('heading', { name: '从计划创建到优化操作包，一次走完上线后闭环' })).toBeVisible()
  await expect(page.locator('.delivery-tour-cases article')).toHaveCount(6)
  await expect(page.locator('.delivery-tour-case-status.observed')).toHaveCount(5)
  await expect(page.getByText('Mock 环境', { exact: true }).first()).toBeVisible()
  await expect(page.getByRole('link', { name: '继续下一步' })).toBeVisible()

  // A sidebar-style visit omits the query string; the latest owner/project run
  // must be restored and refreshed instead of silently losing progress.
  await page.goto(`/projects/${projectId}/delivery/tour`)
  await expect.poll(() => new URL(page.url()).searchParams.get('tour_run_id')).toBe(runId)

  const goldenPlanId = originalPlanIDs.golden_path
  await page.goto(`/projects/${projectId}/delivery/plans?plan_id=${goldenPlanId}&tour_case=golden_path&tour_run_id=${runId}`)
  await expect(page.getByRole('heading', { name: `上线后优化闭环 · 黄金路径 · ${runId}` })).toBeVisible()
  await expect(page.getByLabel('账户边界')).toHaveValue('mock-tour-advertiser')
  await expect.poll(() => new URL(page.url()).searchParams.get('plan_id')).toBe(goldenPlanId)
  await page.goto(`/projects/${projectId}/delivery/configuration?plan_id=${goldenPlanId}&tour_case=golden_path&tour_run_id=${runId}&view=${encodeURIComponent('配置映射')}`)
  await expect(page.getByRole('heading', { name: '平台投放配置' })).toBeVisible()
  await expect(page.getByRole('button', { name: '创建投放计划' })).toHaveCount(0)
  await expect(page.getByRole('definition').filter({ hasText: '巨量引擎' })).toBeVisible()
  await expect(page.getByText('已生成，可进入检查', { exact: true })).toBeVisible()
  await expect(page.getByText('内容已按当前计划版本锁定', { exact: true })).toBeVisible()
  await expect(page.getByText('当前计划 V1', { exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByText('内容已按当前计划版本锁定', { exact: true })).toBeVisible()
  await expect(page.getByText('当前计划 V1', { exact: true })).toBeVisible()
  await page.getByRole('link', { name: '返回走测总览' }).click()
  await expect(page.getByText('2 / 9 步完成', { exact: true })).toBeVisible()
  await expect(page.getByText('下一步：提交首个变更申请并前往审批中心', { exact: true })).toBeVisible()
  let plan = await apiJSON<Plan>(request, 'get', planURL(goldenPlanId), undefined, 200)
  let firstChangeSet = await apiJSON<ChangeSet>(request, 'post', `${planURL(goldenPlanId)}:create-change-set`, { expected_version: plan.version }, 201)
  firstChangeSet = await apiJSON<ChangeSet>(request, 'post', changeSetAction(firstChangeSet.id, 'preflight'), { expected_version: firstChangeSet.version }, 200)
  firstChangeSet = await apiJSON<ChangeSet>(request, 'post', changeSetAction(firstChangeSet.id, 'approve'), { expected_version: firstChangeSet.version }, 200)
  const execution = await apiJSON<ExecutionResult>(request, 'post', changeSetAction(firstChangeSet.id, 'execute'), { expected_version: firstChangeSet.version, scenario: 'success' }, 201, { 'Idempotency-Key': `tour-golden-${suffix}` })
  await page.goto(`/projects/${projectId}/delivery/monitoring?plan_id=${goldenPlanId}&tour_case=golden_path&tour_run_id=${runId}`)
  await page.getByRole('button', { name: '运行投放效果情景模拟' }).click()
  await expect(page.locator('.delivery-simulation-metrics article')).toHaveCount(3)
  await page.getByRole('button', { name: '根据本次指标运行告警规则' }).click()
  await expect(page.locator('.delivery-alert-card').first()).toBeVisible()
  await page.getByRole('link', { name: '返回走测总览' }).click()
  await expect.poll(() => new URL(page.url()).searchParams.get('tour_run_id')).toBe(runId)
  const recommendation = await apiJSON<Recommendation>(request, 'post', `${planURL(goldenPlanId)}/recommendations:generate`, { expected_version: plan.version }, 201)
  const accepted = await apiJSON<RecommendationAcceptance>(request, 'post', `/api/delivery/v1/projects/${projectId}/recommendations/${recommendation.id}:accept`, { expected_version: recommendation.version }, 201, { 'Idempotency-Key': `tour-accept-${suffix}` })
  let secondChangeSet = await apiJSON<ChangeSet>(request, 'post', changeSetAction(accepted.change_set.id, 'preflight'), { expected_version: accepted.change_set.version }, 200)
  secondChangeSet = await apiJSON<ChangeSet>(request, 'post', changeSetAction(secondChangeSet.id, 'approve'), { expected_version: secondChangeSet.version }, 200)
  await apiJSON(request, 'post', `/api/delivery/v1/projects/${projectId}/change-sets/${secondChangeSet.id}/manual-action-package`, { expected_version: secondChangeSet.version }, 201)

  const completed = await apiJSON<TourRun>(request, 'get', tourURL(runId), undefined, 200)
  expect(completed.current_step).toBe('complete')
  expect(completed.steps).toHaveLength(9)
  expect(completed.steps.every(step => step.complete)).toBe(true)
  expect(completed.steps.every(step => step.evidence.length > 0)).toBe(true)

  await page.reload()
  await expect(page.locator('.delivery-tour-steps li.complete')).toHaveCount(9)
  await expect(page.getByText('黄金路径已经完成，可检查异常场景或安全复位。')).toBeVisible()
  await page.locator('.delivery-tour-steps li').first().getByRole('link').click()
  await expect.poll(() => new URL(page.url()).searchParams.get('tour_run_id')).toBe(runId)
  await expect.poll(() => new URL(page.url()).searchParams.get('tour_case')).toBe('golden_path')
  await expect(page.getByRole('link', { name: '返回走测总览' })).toBeVisible()

  const reviewCase = prepared.cases.find(tourCase => tourCase.key === 'review_rejected_alert')!
  await page.goto(reviewCase.start_url)
  await expect(page.locator('.delivery-alert-card')).toHaveCount(0)
  await page.getByRole('button', { name: '运行投放效果情景模拟' }).click()
  await page.getByRole('button', { name: '根据本次指标运行告警规则' }).click()
  await expect(page.locator('.delivery-alert-card').filter({ hasText: '审核拒绝' })).toHaveCount(1)

  const otherRun = await apiJSON<TourRun>(request, 'post', tourURL(otherRunId, 'prepare'), undefined, 201)
  const otherPlanId = otherRun.cases[0].plan_id
  const reset = await apiJSON<TourReset>(request, 'post', tourURL(runId, 'reset'), undefined, 200)
  expect(reset).toMatchObject({ source: 'mock', scenario: 'delivery_tour_reset', run: { id: runId, status: 'reset' } })
  expect(reset.deleted.delivery_plans).toBe(7)
  expect(reset.isolation_key).toContain(`/${projectId}/${runId}/`)
  expect((await request.get(planURL(sentinel.id))).status()).toBe(200)
  expect((await request.get(planURL(otherPlanId))).status()).toBe(200)
  for (const planId of Object.values(originalPlanIDs)) expect((await request.get(planURL(planId))).status()).toBe(404)

  const rebuilt = await apiJSON<TourRun>(request, 'post', tourURL(runId, 'prepare'), undefined, 201)
  expectTourContract(rebuilt, runId)
  expect(rebuilt.cases.map(tourCase => tourCase.plan_id)).not.toEqual(Object.values(originalPlanIDs))

  await apiJSON(request, 'post', tourURL(runId, 'reset'), undefined, 200)
  await apiJSON(request, 'post', tourURL(otherRunId, 'reset'), undefined, 200)
})

function expectTourContract(run: TourRun, runId: string) {
  expect(run).toMatchObject({ id: runId, project_id: projectId, owner_id: 'user_local', status: 'prepared', source: 'mock', scenario: 'delivery_tour' })
  expect(run.cases).toHaveLength(7)
  expect(run.steps).toHaveLength(9)
  expect(new Set(run.cases.map(tourCase => tourCase.key))).toEqual(new Set(['golden_path', 'preflight_failure', 'approval_expired', 'plan_stale', 'partial_execution', 'result_unknown', 'review_rejected_alert']))
  for (const tourCase of run.cases) {
    expect(tourCase).toMatchObject({ source: 'mock', scenario: tourCase.key, observed_at: expect.any(String), start_url: expect.stringContaining(`tour_run_id=${runId}`) })
    expect(tourCase.evidence.length).toBeGreaterThan(0)
  }
}

async function createOrdinaryPlan(request: APIRequestContext, name: string): Promise<Plan> {
  return createRuntimePlan(request, projectId, `sentinel-${Date.now().toString(36)}-${name.length}`) as Promise<Plan>
}

async function apiJSON<T = unknown>(request: APIRequestContext, method: 'get' | 'post', url: string, data: unknown, status: number, headers?: Record<string, string>): Promise<T> {
  const response = method === 'get' ? await request.get(url, { headers }) : await request.post(url, { data, headers })
  if (response.status() !== status) {
    expect(response.status(), `${method.toUpperCase()} ${url}: ${await response.text()}`).toBe(status)
  }
  return response.json() as Promise<T>
}

function tourURL(runId: string, action?: 'prepare' | 'reset') {
  const suffix = action ? `:${action}` : ''
  return `/api/delivery/v1/projects/${projectId}/tour-runs/${runId}${suffix}`
}

function planURL(planId: string) {
  return `/api/delivery/v1/projects/${projectId}/plans/${planId}`
}

function changeSetAction(changeSetId: string, action: 'preflight' | 'approve' | 'execute') {
  return `/api/delivery/v1/projects/${projectId}/change-sets/${changeSetId}:${action}`
}

type TourCase = { key: string; plan_id: string; status: string; source: string; scenario: string; start_url: string; evidence: string[]; observed_at: string }
type TourStep = { key: string; complete: boolean; evidence: string[] }
type TourRun = { id: string; project_id: string; owner_id: string; status: string; source: string; scenario: string; cases: TourCase[]; steps: TourStep[]; current_step: string }
type TourReset = { run: TourRun; deleted: Record<string, number>; source: string; scenario: string; isolation_key: string }
type Plan = { id: string; version: number }
type ChangeSet = { id: string; version: number }
type ExecutionResult = { execution: { id: string } }
type Recommendation = { id: string; version: number }
type RecommendationAcceptance = { change_set: ChangeSet }
