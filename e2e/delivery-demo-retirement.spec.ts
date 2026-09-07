import { expect, test } from '@playwright/test'
import { createRuntimePlan } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'
const base = `/api/delivery/v1/projects/${projectId}`

test('retired demo endpoints return Gone and leave historical lists unchanged', async ({ request }) => {
  const beforePlans = await (await request.get(`${base}/plans`)).json()
  const beforeExecutions = await (await request.get(`${base}/executions`)).json()
  for (const path of ['tour-runs/old-run:prepare', 'tour-runs/old-run:reset', 'plans/old-plan/recommendations:generate', 'recommendations/old-recommendation:accept', 'recommendations/old-recommendation:reject', 'executions/old-execution/simulation-runs', 'executions/old-execution/metric-snapshots', 'alerts:evaluate']) {
    const response = await request.post(`${base}/${path}`, { data: {} })
    expect(response.status(), path).toBe(410)
    expect(await response.json()).toMatchObject({ error: { code: 'DELIVERY_DEMO_RETIRED', retryable: false } })
  }
  expect(await (await request.get(`${base}/plans`)).json()).toEqual(beforePlans)
  expect(await (await request.get(`${base}/executions`)).json()).toEqual(beforeExecutions)
})

test('normal and old Tour URLs use the same business optimization page', async ({ page }) => {
  const retiredRequests: string[] = []
  page.on('request', request => { if (/tour-runs|recommendations(?::generate|\/|$)|simulation-runs/.test(new URL(request.url()).pathname) && !request.url().includes('mechanistic')) retiredRequests.push(request.url()) })
  for (const query of ['', '?tour_run_id=old-run&tour_case=golden_path']) {
    await page.goto(`/projects/${projectId}/delivery/optimization${query}`)
    await expect(page.getByRole('button', { name: '生成优化方案', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '生成优化建议', exact: true })).toHaveCount(0)
    await expect(page.getByLabel('Delivery Mock 环境')).toHaveCount(0)
  }
  expect(retiredRequests).toEqual([])
})

test('new draft leaves tracking empty and blocks missing inputs', async ({ page }) => {
  await page.goto(`/projects/${projectId}/delivery/plans?tour_run_id=old-run`)
  await page.getByRole('button', { name: '新建投放计划', exact: true }).click()
  await page.getByRole('button', { name: '预算与排期', exact: true }).click()
  await page.getByRole('spinbutton', { name: /日预算|总预算/ }).fill('')
  await expect(page.getByText(/还需填写：.*大于 0 的预算/)).toBeVisible()
  await page.getByRole('button', { name: '投放载体和监测', exact: true }).click()
  await page.getByLabel('投放载体', { exact: true }).selectOption('owned_landing_page')
  await expect(page.getByRole('textbox', { name: '自研落地页链接', exact: true })).toHaveValue('')
  await expect(page.getByRole('button', { name: '保存', exact: true })).toBeDisabled()
  await expect(page.getByText(/还需填写：.*自研落地页链接/)).toBeVisible()
})

test('configuration hands off to the controlled execution center without demo writes', async ({ page, request }) => {
  const plan = await createRuntimePlan(request, projectId, `handoff-${Date.now().toString(36)}`)
  const retiredRequests: string[] = []
  page.on('request', request => {
    if (request.method() === 'POST' && /(?:\/execute|:execute|tour-runs|simulation-runs)/.test(new URL(request.url()).pathname)) retiredRequests.push(request.url())
  })
  const runId = 'retirement-handoff-fixture'
  await page.route(`**/plans/${plan.id}/browser-rpa-runs`, async route => {
    expect(route.request().postDataJSON()).toEqual({ expected_version: plan.current_version_number, execution_driver: 'playwright-rpa/edge/v3' })
    await route.fulfill({ status: 201, json: { controlled_change_set: { id: 'controlled-fixture' }, controlled_execution: { id: 'execution-fixture' }, browser_rpa_run: { run_id: runId } } })
  })
  await page.goto(`/projects/${projectId}/delivery/configuration?view=${encodeURIComponent('检查与提交')}&plan_id=${plan.id}&tour_run_id=old-run`)
  await expect(page.getByRole('button', { name: '确认投放', exact: true })).toHaveCount(0)
  await expect(page.getByRole('radio', { name: /Web API/ })).toBeChecked()
  await page.getByRole('radio', { name: /Playwright/ }).check()
  await page.getByRole('button', { name: '使用 Playwright创建执行', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`/projects/${projectId}/delivery/execution/${runId}$`))
  await expect(page.getByRole('heading', { name: '暂无受控执行 Run' })).toBeVisible()
  expect(retiredRequests).toEqual([])
  await page.screenshot({ path: 'test-results/delivery-retirement-execution.png', fullPage: true })
})
