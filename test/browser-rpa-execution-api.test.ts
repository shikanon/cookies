import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import { ControlledExecutionApiError, controlledExecutionApi } from '../src/features/browser-rpa-execution/api.ts'

test('controlled execution API supports the complete Runner v3 browser flow', async () => {
  const calls: Array<{ url: string; method: string; body?: string }> = []
  const originalFetch = globalThis.fetch
  const run = {
    id: 'run_1', project_id: 'project_1', environment_id: 'env_1', profile_id: 'profile_1', policy_id: 'policy_1', lease_id: 'lease_1',
    version: 4, account_id: '1855554434276391', state: 'awaiting_confirmation',
    authority: { approval_action_hash: 'a'.repeat(64) },
  }
  globalThis.fetch = async (input, init) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    calls.push({ url, method, body: typeof init?.body === 'string' ? init.body : undefined })
    let value: unknown = run
    if (url.endsWith('/runs')) value = { items: [run] }
    if (url.endsWith('/events') || url.endsWith('/evidence') || url.endsWith('/steps')) value = { items: [] }
    if (url.includes('/environments/')) value = { id: 'env_1', account_id: run.account_id, mode: 'local_visible', browser_version: 'Edge', region: 'local', healthy: true, version: 1 }
    if (url.includes('/browser-profiles/')) value = { id: 'profile_1', environment_id: 'env_1', account_id: run.account_id, state: 'ready', version: 1 }
    if (url.includes('/site-policies/')) value = { id: 'policy_1', account_id: run.account_id, allowed_page_kinds: ['promotion_create'], allowed_platform_project_ids: ['project-platform-1'], version: 1 }
    if (url.includes('/leases/')) value = { id: 'lease_1', run_id: 'run_1', holder: 'user', fencing_token: 3, version: 2, expires_at: '2026-08-25T12:00:00Z', heartbeat_deadline: '2026-08-25T12:00:00Z' }
    if (url.endsWith(':plan')) value = { schema_version: 'oceanengine-playwright-rpa-plan/v3', plan_kind: 'promotion_create', mode: 'prepare', status: 'ready', steps: [], blocked_reasons: [], allow_remote_write: false, maximum_final_clicks: 0 }
    if (url.endsWith(':check-session')) value = { schema_version: 'browser-rpa-edge-session-probe/v1', checked_at: '2026-08-25T10:00:00Z', status: 'ready', reason: 'session_ready', cdp_available: true, oceanengine_page_available: true, logged_in: true, account_matched: true }
    if (url.endsWith('/confirmations')) value = { confirmation: { id: 'confirmation_1' }, token: 'memory-only-token' }
    return new Response(JSON.stringify(value), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }

  try {
    const runs = await controlledExecutionApi.listRuns('project_1')
    const workspace = await controlledExecutionApi.getWorkspace('project_1', 'run_1')
    assert.equal(runs[0]?.id, 'run_1')
    assert.equal(workspace.environment.healthy, true)
    assert.deepEqual(workspace.steps, [])
    assert.equal(workspace.profile.state, 'ready')
    assert.equal(workspace.lease?.fencing_token, 3)

    await controlledExecutionApi.generatePlan('project_1', 'run_1')
    const probe = await controlledExecutionApi.checkSession('project_1', 'run_1')
    await controlledExecutionApi.heartbeatLease('project_1', 'run_1', workspace.lease!)
    await controlledExecutionApi.prepare('project_1', 'run_1')
    await controlledExecutionApi.reconcileResult('project_1', 'run_1')
    const confirmation = await controlledExecutionApi.confirm('project_1', run as never)
    await controlledExecutionApi.submit('project_1', run as never, workspace.lease!, confirmation)

    assert.equal(calls.find(call => call.url.endsWith(':plan'))?.method, 'POST')
    assert.equal(calls.find(call => call.url.endsWith(':check-session'))?.method, 'POST')
    assert.equal(calls.find(call => call.url.endsWith(':reconcile-result'))?.method, 'POST')
    assert.equal(probe.account_matched, true)
    assert.match(calls.find(call => call.url.endsWith('/confirmations'))?.body ?? '', /"binding_hash":"a{64}"/)
    const submit = calls.find(call => call.url.endsWith(':submit'))
    assert.match(submit?.body ?? '', /"fencing_token":3/)
    assert.match(submit?.body ?? '', /"token":"memory-only-token"/)
    assert.match(submit?.body ?? '', /"step_id":"run_1-submit-v4"/)
    assert.match(submit?.body ?? '', /"idempotency_key":"run_1-submit-v4"/)
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('controlled execution UI reports the real Edge probe and keeps unsupported actions blocked', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/features/browser-rpa-execution/BrowserRpaExecutionWorkspace.tsx'), 'utf8')
  assert.match(source, /Edge 会话可用。CDP、登录状态和广告账户均匹配/)
  assert.match(source, /DevTools WebSocket/)
  assert.match(source, /当前动作没有 Runner v3 单表单协议/)
  assert.doesNotMatch(source, /尚未连接；Prepare 时检查/)
  assert.match(source, /受控平台执行记录/)
  assert.match(source, /只读查询平台结果/)
  assert.match(source, /oceanengine-web-api\/session\/v1/)
  assert.match(source, /Connector 组织账号会话/)
  assert.match(source, /controlledExecutionApi\.listRuns/)
  assert.match(source, /此执行记录固定使用投放计划 v/)
  assert.match(source, /当前计划的后续修改不会更新此记录/)
  assert.match(source, /Prepare 已启动。Runner v3 正在进入巨量表单并执行字段回读/)
  assert.match(source, /Prepare 未完成/)
  assert.match(source, /prepared\.state !== 'awaiting_confirmation'/)
  assert.match(source, /Prepare 服务端任务/)
  assert.match(source, /new Date\(currentLease\.heartbeat_deadline\)\.getTime\(\) > Date\.now\(\)/)
  assert.match(source, /observedRunState/)
  assert.match(source, /重试 Prepare/)
  assert.match(source, /isSafePrepareRetryCandidate/)
  assert.match(source, /deliveryExecutionApi\.startBrowserRpaExecution/)
  assert.match(source, /失败 Run 和证据会保留/)
})

test('controlled execution API shows a structured platform error', async () => {
  const originalFetch = globalThis.fetch
  globalThis.fetch = async () => new Response(JSON.stringify({ error: { code: 'INTERNAL', message: 'Runner 读取失败' } }), {
    status: 500,
    headers: { 'Content-Type': 'application/json' },
  })
  try {
    await assert.rejects(
      controlledExecutionApi.generatePlan('project_1', 'run_1'),
      (error: unknown) => error instanceof ControlledExecutionApiError && error.message === 'Runner 读取失败',
    )
  } finally {
    globalThis.fetch = originalFetch
  }
})
