import type { BrowserRpaRun, ControlledExecutionPresentation, ControlledExecutionWorkspace } from './model'

const statePresentation: Record<BrowserRpaRun['state'], Omit<ControlledExecutionPresentation, 'kind' | 'allowsNormalRetry'>> = {
  queued: { tone: 'neutral', title: '已排队等待环境检查', detail: '尚未开始页面或平台动作。' },
  environment_check: { tone: 'neutral', title: '正在检查执行环境', detail: '服务端正在核验环境、账户、站点策略和会话租约。' },
  awaiting_takeover: { tone: 'warning', title: '等待人工接管', detail: '首期接管模式的正常状态。接管前不会继续自动动作。' },
  preparing: { tone: 'neutral', title: '正在准备表单', detail: '仅允许导航、字段填写和逐字段回读；尚未触发远端提交。' },
  awaiting_confirmation: { tone: 'warning', title: '等待一次性最终确认', detail: '正式写入 Approval 不等于最终提交许可；确认必须绑定当前 Run。' },
  submitting: { tone: 'warning', title: '正在受控提交', detail: '提交前由权威服务再次校验 Approval、Lease、allowlist 与 Kill Switch。' },
  verifying: { tone: 'neutral', title: '正在写后验证', detail: '正在核验平台对象、状态与列表页二次读取。' },
  succeeded: { tone: 'success', title: '已确认完成', detail: '仅在目标效果和写后映射均已确认时显示成功。' },
  failed: { tone: 'danger', title: '执行失败', detail: '系统没有足够证据确认目标效果。请检查阻塞代码和运行证据。' },
  partial: { tone: 'warning', title: '部分完成，需要受控恢复', detail: '已确认范围会保留；任何补偿都必须作为新的受控动作。' },
  result_unknown: { tone: 'danger', title: '结果未知，禁止普通重试', detail: '只能查询、重新识别或人工接管；绝不能再次点击提交。' },
  cancelled: { tone: 'neutral', title: '运行已取消', detail: '运行保持可审计；如需继续，必须建立新的受控流程。' },
}

export function presentControlledExecution(run: BrowserRpaRun): ControlledExecutionPresentation {
  if (run.blocking_reason === 'KILL_SWITCH_ACTIVE') {
    return { kind: 'kill_switch_active', tone: 'danger', title: 'Kill Switch 已激活', detail: '已阻止新的写入尝试；仍可查看证据、取消或进行人工接管。', allowsNormalRetry: false }
  }
  if (run.blocking_reason === 'APPROVAL_INVALID') {
    return { kind: 'approval_expired', tone: 'danger', title: '正式写入 Approval 已失效', detail: '审批可能已过期或绑定内容发生漂移。必须重新生成差异并重新审批。', allowsNormalRetry: false }
  }
  if (run.blocking_reason === 'FINAL_CONFIRMATION_INVALID') {
    return { kind: 'confirmation_expired', tone: 'danger', title: '一次性最终确认已失效', detail: '确认已过期、被消费、被拒绝或与当前绑定不一致。不得提交。', allowsNormalRetry: false }
  }
  if (run.blocking_reason === 'RUNNER_FAILURE') {
    return { kind: 'runner_failure', tone: 'danger', title: 'Runner 执行链路失败', detail: '页面字段不一定漂移。请检查 Runner 结果协议、进程状态和服务端日志。', allowsNormalRetry: false }
  }
  if (run.blocking_reason === 'PAGE_DRIFT') {
    return { kind: 'page_drift', tone: 'danger', title: 'Runner 页面匹配失败', detail: 'Runner 已停止。该状态不能证明目标效果不存在，也不能证明提交结果未知。', allowsNormalRetry: false }
  }
  if (run.blocking_reason === 'TARGET_EFFECT_NOT_OBSERVED') {
    return { kind: 'target_effect_not_observed', tone: 'danger', title: '已确认未产生目标效果', detail: '只读核对已确认目标对象不存在，或平台未收到写入请求。', allowsNormalRetry: false }
  }
  const base = statePresentation[run.state]
  return {
    kind: run.state,
    ...base,
    // Terminal outcomes must be reconciled or started as a new ChangeSet; a
    // generic retry would bypass the controlled recovery decision.
    allowsNormalRetry: !isTerminalControlledExecutionState(run.state),
  }
}

export function isTerminalControlledExecutionState(state: BrowserRpaRun['state']) {
  return state === 'succeeded' || state === 'failed' || state === 'partial' || state === 'result_unknown' || state === 'cancelled'
}

const executionViewStates: Record<string, ReadonlySet<BrowserRpaRun['state']>> = {
  '待执行': new Set(['queued']),
  '执行中': new Set(['environment_check', 'preparing', 'submitting', 'verifying']),
  '等待用户': new Set(['awaiting_confirmation']),
  '结果未知': new Set(['result_unknown']),
  '失败': new Set(['failed', 'partial', 'cancelled']),
  '接管': new Set(['awaiting_takeover']),
  '完成': new Set(['succeeded']),
}

export function runMatchesExecutionView(run: BrowserRpaRun, view: string) {
  return executionViewStates[view]?.has(run.state) ?? true
}

export function isSafePrepareRetryCandidate(workspace: ControlledExecutionWorkspace): boolean {
  const { run, steps, evidence } = workspace
  if (run.state !== 'failed') return false
  if (!run.authority.plan_id || !run.authority.plan_version || run.takeover_active) return false
  if (run.blocking_reason === 'TARGET_EFFECT_NOT_OBSERVED') {
    return evidence.some(item => item.field_readback?.reconciliation === 'not_found'
      && ((item.field_readback?.read_only_reconciliation === 'true'
        && item.field_readback?.platform_write_performed === 'false'
        && item.field_readback?.exact_name_matches === '0')
        || (item.field_readback?.final_click_performed === 'true'
          && item.field_readback?.platform_write_request_observed === 'false')))
  }
  if (run.blocking_reason !== 'PAGE_DRIFT' && run.blocking_reason !== 'RUNNER_FAILURE') return false
  if (!steps.some(step => step.action === 'prepare_and_readback' && step.status === 'failed')) return false
  if (steps.some(step => step.action.toLowerCase().includes('submit') || step.status === 'result_unknown')) return false
  return !evidence.some(item => item.field_readback?.final_click_performed === 'true' || item.after_page_facts?.final_click_performed === 'true')
}

export function shortHash(value: string) {
  return value.length > 16 ? `${value.slice(0, 16)}…` : value
}
