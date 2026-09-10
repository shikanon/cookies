import assert from 'node:assert/strict'
import test from 'node:test'
import type { BrowserRpaEvidence, BrowserRpaRun, BrowserRpaRunStep } from '../src/features/browser-rpa-execution/model.ts'
import { fieldDrift, isSafePrepareRetryCandidate, nativePromotionSubmitNotCalibrated, presentControlledExecution, runMatchesExecutionView } from '../src/features/browser-rpa-execution/presentation.ts'

const hash = 'a'.repeat(64)

test('field recovery clears only the same object and retains other unresolved drift', () => {
  const evidence = (id: string, status: string) => ({ object_fingerprint: id, field_readback: { field_reconciliation_status: status } }) as BrowserRpaEvidence
  const original = evidence('project-1', 'drifted')
  assert.equal(fieldDrift([original, evidence('promotion-1', 'matched')]), true)
  assert.equal(fieldDrift([original, evidence('project-1', 'not_checked')]), true)
  assert.equal(fieldDrift([original, evidence('project-1', 'matched')]), false)
  assert.equal(fieldDrift([original, evidence('project-1', 'matched'), evidence('promotion-1', 'drifted')]), true)
})

test('controlled execution presentation keeps approval, confirmation, and result recovery distinct', () => {
  const cases: Array<{ name: string; run: Partial<BrowserRpaRun>; kind: string; retry: boolean }> = [
    { name: 'approval invalid', run: { state: 'awaiting_confirmation', blocking_reason: 'APPROVAL_INVALID' }, kind: 'approval_expired', retry: false },
    { name: 'confirmation invalid', run: { state: 'awaiting_confirmation', blocking_reason: 'FINAL_CONFIRMATION_INVALID' }, kind: 'confirmation_expired', retry: false },
    { name: 'partial', run: { state: 'partial' }, kind: 'partial', retry: false },
    { name: 'unknown', run: { state: 'result_unknown' }, kind: 'result_unknown', retry: false },
    { name: 'cancelled', run: { state: 'cancelled' }, kind: 'cancelled', retry: false },
    { name: 'kill switch', run: { state: 'preparing', blocking_reason: 'KILL_SWITCH_ACTIVE' }, kind: 'kill_switch_active', retry: false },
    { name: 'runner failure', run: { state: 'failed', blocking_reason: 'RUNNER_FAILURE' }, kind: 'runner_failure', retry: false },
    { name: 'page drift', run: { state: 'failed', blocking_reason: 'PAGE_DRIFT' }, kind: 'page_drift', retry: false },
    { name: 'native calibration', run: { state: 'failed', blocking_reason: 'NATIVE_PROMOTION_SUBMIT_NOT_CALIBRATED' }, kind: 'native_promotion_submit_not_calibrated', retry: false },
    { name: 'target absent', run: { state: 'failed', blocking_reason: 'TARGET_EFFECT_NOT_OBSERVED' }, kind: 'target_effect_not_observed', retry: false },
    { name: 'takeover', run: { state: 'awaiting_takeover' }, kind: 'awaiting_takeover', retry: true },
  ]

  for (const value of cases) {
    const presentation = presentControlledExecution(run(value.run))
    assert.equal(presentation.kind, value.kind, value.name)
    assert.equal(presentation.allowsNormalRetry, value.retry, value.name)
  }
})

test('page drift does not claim that the target effect is absent', () => {
  const presentation = presentControlledExecution(run({ state: 'failed', blocking_reason: 'PAGE_DRIFT' }))
  assert.equal(presentation.title, 'Runner 页面匹配失败')
  assert.match(presentation.detail, /不能证明目标效果不存在/)
})

test('native calibration failure explains the pre-click stop', () => {
  const presentation = presentControlledExecution(run({ state: 'failed', blocking_reason: 'NATIVE_PROMOTION_SUBMIT_NOT_CALIBRATED' }))
  assert.equal(presentation.title, '原生视频单元尚未完成提交校准')
  assert.match(presentation.detail, /未提交单元/)
  assert.doesNotMatch(presentation.detail, /结果未知|页面匹配/)
})

test('native submit gate uses the current preparation, including after page reload', () => {
  const prepare = (id: string, sequence: number): BrowserRpaRunStep => ({ id, run_id: 'run_1', sequence, workflow_step_id: 'prepare', action: 'prepare_and_readback', status: 'succeeded', attempt: 1, version: 1 })
  const evidence = (stepId: string, fieldReadback: Record<string, string>): BrowserRpaEvidence => ({ id: stepId, run_id: 'run_1', step_id: stepId, diff_keys: [], field_readback: fieldReadback, page_reference: '', object_fingerprint: '', selector_version: '', action_version: '', redaction_version: '', created_at: '' })
  const project = prepare('project', 1)
  const native = prepare('native', 2)
  const nativePlan = { steps: [{ id: 'title', kind: 'field_action', page_kind: 'promotion_create', field_key: 'promotion.title_mode', remote_write: false, blocked: false }] }
  assert.equal(nativePromotionSubmitNotCalibrated({ steps: [], evidence: [] }, nativePlan), true)
  assert.equal(nativePromotionSubmitNotCalibrated({ steps: [project, native], evidence: [evidence('project', {}), evidence('native', { 'promotion.title_mode': '投放原视频标题' })] }), false)
  assert.equal(nativePromotionSubmitNotCalibrated({ steps: [native], evidence: [evidence('native', { 'promotion.title_mode': '手动添加' })] }), true)
  assert.equal(nativePromotionSubmitNotCalibrated({ steps: [], evidence: [] }, { steps: [{ ...nativePlan.steps[0], value: '投放原视频标题' }] }), false)
  assert.equal(nativePromotionSubmitNotCalibrated({ steps: [native, prepare('later', 3)], evidence: [evidence('native', { 'promotion.title_mode': '投放原视频标题' }), evidence('later', { 'promotion.landing_page_reference': '123' })] }, nativePlan), false)
  assert.equal(nativePromotionSubmitNotCalibrated({ steps: [], evidence: [] }), false)
})

test('controlled execution tabs filter runs by workflow state', () => {
  const cases: Array<{ view: string; states: BrowserRpaRun['state'][] }> = [
    { view: '待执行', states: ['queued'] },
    { view: '执行中', states: ['environment_check', 'preparing', 'submitting', 'verifying'] },
    { view: '等待用户', states: ['awaiting_confirmation'] },
    { view: '结果未知', states: ['result_unknown'] },
    { view: '失败', states: ['failed', 'partial', 'cancelled'] },
    { view: '接管', states: ['awaiting_takeover'] },
    { view: '完成', states: ['succeeded'] },
  ]
  const allStates: BrowserRpaRun['state'][] = ['queued', 'environment_check', 'awaiting_takeover', 'preparing', 'awaiting_confirmation', 'submitting', 'verifying', 'succeeded', 'failed', 'partial', 'result_unknown', 'cancelled']

  for (const value of cases) {
    assert.deepEqual(allStates.filter(state => runMatchesExecutionView(run({ state }), value.view)), value.states, value.view)
  }
})

test('only a failed Prepare without a submit boundary can create a safe retry', () => {
  const candidate = {
    run: run({ state: 'failed', blocking_reason: 'PAGE_DRIFT', authority: { ...run({}).authority, plan_id: 'plan_1', plan_version: 3 } }),
    steps: [{ id: 'prepare', run_id: 'run_1', sequence: 1, workflow_step_id: 'prepare', action: 'prepare_and_readback', status: 'failed', attempt: 1, version: 1 }],
    evidence: [],
  }
  assert.equal(isSafePrepareRetryCandidate(candidate as never), true)
  assert.equal(isSafePrepareRetryCandidate({ ...candidate, steps: [...candidate.steps, { ...candidate.steps[0], id: 'submit', sequence: 2, action: 'submit_platform_configuration' }] } as never), false)
  assert.equal(isSafePrepareRetryCandidate({ ...candidate, steps: [{ ...candidate.steps[0], id: 'prior-submit', sequence: 0, action: 'submit_platform_configuration', status: 'succeeded' }, ...candidate.steps], evidence: [{ step_id: 'prior-submit', field_readback: { final_click_performed: 'true' } }] } as never), true)
  assert.equal(isSafePrepareRetryCandidate({ ...candidate, evidence: [{ field_readback: { final_click_performed: 'true' } }] } as never), false)
  assert.equal(isSafePrepareRetryCandidate({ ...candidate, run: run({ state: 'failed', blocking_reason: 'ACCOUNT_MISMATCH' }) } as never), false)
})

function run(patch: Partial<BrowserRpaRun>): BrowserRpaRun {
  return {
    schema_version: 'computer-use-run/v1', id: 'run_1', organization_id: 'org_1', project_id: 'project_1', platform: 'ocean_engine', account_id: 'account_1',
    authority: {
      schema_version: 'computer-use-authority/v1', business_execution_id: 'execution_1', change_set_id: 'change_1', approval_id: 'approval_1', approval_action_hash: hash,
      account_reference_id: 'account_1', object_fingerprint: 'object_1', action: 'create', budget_limit_minor: 100_00, currency: 'CNY',
      plan_canonical_hash: hash, intent_canonical_hash: hash, feedback_canonical_hash: hash, decision_canonical_hash: hash, configuration_canonical_hash: hash,
      workflow_id: 'workflow_1', workflow_canonical_hash: hash, workflow_step_id: 'step_1', skill_id: 'oceanengine-ecommerce-manual', skill_version: 'v0.1-calibration',
    },
    environment_id: 'environment_1', profile_id: 'profile_1', lease_id: 'lease_1', policy_id: 'policy_1', state: 'queued', paused: false, takeover_active: false,
    version: 1, idempotency_key: 'idempotency_1', request_hash: hash, created_by: 'user_1', created_at: '2026-08-12T00:00:00Z', updated_at: '2026-08-12T00:00:00Z',
    ...patch,
  }
}
