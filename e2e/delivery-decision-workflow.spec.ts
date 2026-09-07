import { expect, test } from '@playwright/test'
import { createRuntimePlan } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'

test('DeliveryDecision diagnoses missing facts without forcing a candidate or enabling remote writes', async ({ page, request }) => {
  test.setTimeout(120_000)
  const suffix = `decision-${Date.now().toString(36)}`
  const plan = await createRuntimePlan(request, projectId, suffix)
  const response = await request.post(`/api/delivery/v1/projects/${projectId}/plans/${plan.id}/decisions:generate`, { data: { expected_version: plan.current_version_number } })
  expect(response.status()).toBe(201)
  const decision = await response.json()
  expect(decision).toMatchObject({
    schema_version: 'delivery-decision/v1',
    policy_version: 'delivery-decision-policy/v2',
    diagnostic: { code: 'insufficient_data' },
    candidates: [],
    recommended_candidate_id: '',
  })
  expect(decision.canonical_hash).toMatch(/^[0-9a-f]{64}$/)

  await page.goto(`/projects/${projectId}/delivery/optimization`)
  await page.locator('.delivery-optimization-toolbar select').selectOption(plan.id)
  await expect(page.getByRole('button', { name: '生成优化方案' })).toBeVisible()
  await expect(page.getByText('数据不足', { exact: true })).toBeVisible()
  await expect(page.getByText('所有调整先保存为本地方案，不会自动修改广告平台。')).toBeVisible()
  await expect(page.getByRole('button', { name: '生成优化建议' })).toHaveCount(0)
})
