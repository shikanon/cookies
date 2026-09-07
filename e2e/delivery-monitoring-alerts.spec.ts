import { expect, test } from '@playwright/test'
import { createRuntimePlan } from './delivery-runtime-fixture'

const projectId = 'project_investor_precision_evidence'

test('Connector inspection reports missing data without generating simulated alerts', async ({ request }) => {
  const plan = await createRuntimePlan(request, projectId, `monitoring-${Date.now().toString(36)}`)
  const base = `/api/delivery/v1/projects/${projectId}`
  const response = await request.post(`${base}/alerts:inspect`, { data: { plan_id: plan.id, window_days: 14 } })
  expect(response.status()).toBe(200)
  const inspection = await response.json()
  expect(inspection).toMatchObject({ source: 'connector', is_simulated: false, items: [], created_count: 0 })
  expect(inspection.status).not.toBe('ready')
  expect(inspection.status_reason).toEqual(expect.any(String))
  const alerts = await request.get(`${base}/alerts?plan_id=${plan.id}`)
  expect(alerts.status()).toBe(200)
  expect((await alerts.json()).items).toEqual([])
  const crossProject = await request.post('/api/delivery/v1/projects/project_local/alerts:inspect', { data: { plan_id: plan.id } })
  expect(crossProject.status()).toBe(404)
})
