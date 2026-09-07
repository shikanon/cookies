import { expect, test } from '@playwright/test'

const hash = 'a'.repeat(64)
const projectId = 'project_local'
const runId = 'browser_rpa_run_fake_1'

test('controlled execution center stays server-authoritative and safe on desktop and narrow layouts', async ({ page }) => {
  const runRoute = new RegExp(`/api/platform/v1/browser-rpa/projects/${projectId}/runs/${runId}(?:/(?:events|evidence))?$`)
  await page.route(runRoute, async route => {
    const url = new URL(route.request().url())
    if (route.request().method() !== 'GET') {
      await route.fulfill({ status: 405, json: { error: 'fake E2E does not authorize writes' } })
      return
    }
    if (url.pathname.endsWith('/events')) {
      await route.fulfill({ json: { items: [{ id: 'event_1', run_id: runId, sequence: 8, kind: 'result_reconciliation_required', summary: 'result unknown; query before any recovery', actor: 'fake-worker', created_at: '2026-08-12T10:00:00Z' }] } })
      return
    }
    if (url.pathname.endsWith('/evidence')) {
      await route.fulfill({ json: { items: [{ id: 'evidence_1', run_id: runId, step_id: 'submit_1', diff_keys: ['project_name', 'daily_budget'], redaction_version: 'browser-rpa-redaction/v1', selector_version: 'fake-selector/v1', created_at: '2026-08-12T10:00:00Z' }] } })
      return
    }
    await route.fulfill({ json: fakeRun() })
  })

  await page.route(new RegExp(`/api/platform/v1/browser-rpa/projects/${projectId}/(?:environments|browser-profiles|site-policies)/`), async route => {
    const url = route.request().url()
    const account_id = 'account_fake_1'
    const value = url.includes('/environments/')
      ? { id: 'environment_fake_1', account_id, mode: 'local_visible', browser_version: 'Edge', region: 'local', healthy: true, version: 1 }
      : url.includes('/browser-profiles/')
        ? { id: 'profile_fake_1', environment_id: 'environment_fake_1', account_id, state: 'ready', version: 1 }
        : { id: 'policy_fake_1', account_id, allowed_page_kinds: ['promotion_create'], allowed_platform_project_ids: [], version: 1 }
    await route.fulfill({ json: value })
  })

  await page.goto(`/projects/${projectId}/delivery/execution/${runId}`)
  const workspace = page.getByRole('region', { name: '受控执行中心' })
  await expect(workspace).toBeVisible()
  await expect(workspace.getByText('不要再次 Submit。先查询平台对象，再重新识别当前页面。必要时执行人工接管。确认平台结果后，创建新的补偿计划。')).toBeVisible()
  await expect(workspace.getByText('result unknown; query before any recovery')).toBeVisible()
  await expect(workspace.getByText('redaction=browser-rpa-redaction/v1 · selector=fake-selector/v1')).toBeVisible()
  await expect(workspace.getByRole('button', { name: /重试提交/ })).toHaveCount(0)
  await expect(workspace.getByText('approval_fake_1')).toBeVisible()
  await expect(workspace.getByRole('button', { name: '生成计划', exact: true })).toBeDisabled()
  await expect(workspace.getByRole('button', { name: '执行 Prepare', exact: true })).toBeDisabled()

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(workspace).toBeVisible()
  await expect(workspace.getByRole('button', { name: '从服务端刷新' })).toBeVisible()
  await expect(workspace.getByRole('button', { name: /取消运行/ })).toBeDisabled()
})

function fakeRun() {
  return {
    schema_version: 'browser-rpa-run/v1', id: runId, organization_id: 'org_local', project_id: projectId,
    platform: 'ocean_engine', account_id: 'account_fake_1',
    authority: {
      schema_version: 'browser-rpa-authority/v1', organization_id: 'org_local', project_id: projectId,
      business_execution_id: 'execution_fake_1', change_set_id: 'change_fake_1', approval_id: 'approval_fake_1', approval_action_hash: hash,
      account_reference_id: 'account_fake_1', object_fingerprint: hash, action: 'create_project_and_promotions', budget_limit_minor: 300000, currency: 'CNY',
      plan_canonical_hash: hash, intent_canonical_hash: hash, feedback_canonical_hash: hash, decision_canonical_hash: hash, configuration_canonical_hash: hash,
      workflow_id: 'workflow_fake_1', workflow_canonical_hash: hash, workflow_step_id: 'step_fake_1', skill_id: 'oceanengine-ecommerce-manual', skill_version: 'v0.1-calibration',
    },
    environment_id: 'environment_fake_1', profile_id: 'profile_fake_1', lease_id: 'lease_fake_1', policy_id: 'policy_fake_1',
    state: 'result_unknown', blocking_reason: 'RESULT_RECONCILIATION_REQUIRED', paused: false, takeover_active: false,
    version: 8, idempotency_key: 'fake-run-key', request_hash: hash, created_by: 'fake-e2e', created_at: '2026-08-12T10:00:00Z', updated_at: '2026-08-12T10:01:00Z',
  }
}
