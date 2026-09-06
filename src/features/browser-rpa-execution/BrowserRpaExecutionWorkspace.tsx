import { useCallback, useEffect, useMemo, useRef, useState, useTransition } from 'react'
import { CircleAlert, CircleCheck, Clock3, FileCheck2, Hand, ListChecks, MonitorCheck, Pause, Play, RefreshCw, Search, Send, ShieldAlert, XCircle } from 'lucide-react'
import { controlledExecutionApi, ControlledExecutionApiError } from './api'
import { deliveryExecutionApi } from '../../api/delivery'
import type { BrowserRpaEvidence, BrowserRpaRun, BrowserRpaRunEvent, ControlledExecutionTransportState, ControlledExecutionWorkspace, EdgeSessionProbe, RunnerV3Plan } from './model'
import { presentConfigurationIssue, presentObjectAvailability, presentPlanBlockedReason } from './objectAvailabilityPresentation'
import { isSafePrepareRetryCandidate, isTerminalControlledExecutionState, presentControlledExecution, runMatchesExecutionView, shortHash } from './presentation'
import './browser-rpa-execution.css'

type Props = {
  projectId: string
  /** The route should supply a server-issued BrowserRpaRun id; an absent id is a real empty state, not a fixture. */
  runId?: string
  activeView: string
}

export function BrowserRpaExecutionWorkspace({ projectId, runId, activeView }: Props) {
  return runId ? <BrowserRpaExecutionDetail projectId={projectId} runId={runId}/> : <BrowserRpaRunList projectId={projectId} activeView={activeView}/>
}

function BrowserRpaRunList({ projectId, activeView }: { projectId: string; activeView: string }) {
  const [state, setState] = useState<{ kind: 'loading' } | { kind: 'ready'; runs: BrowserRpaRun[] } | { kind: 'error'; message: string }>({ kind: 'loading' })

  const load = useCallback(async (signal?: AbortSignal) => {
    setState({ kind: 'loading' })
    try {
      const runs = await controlledExecutionApi.listRuns(projectId, signal)
      if (!signal?.aborted) setState({ kind: 'ready', runs })
    } catch (error) {
      if (!signal?.aborted) setState({ kind: 'error', message: error instanceof Error ? error.message : '读取执行记录失败。' })
    }
  }, [projectId])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  if (state.kind === 'loading') return <WorkspaceState kind="loading" />
  if (state.kind === 'error') return <WorkspaceState kind="error" message={state.message} onRetry={() => void load()} />
  const visibleRuns = state.runs.filter(run => runMatchesExecutionView(run, activeView))
  return <section className="controlled-execution-run-list" aria-label="受控平台执行记录">
    <header className="controlled-execution-header"><div><span className="section-label">Controlled platform execution</span><h2>执行中心</h2><p>查看当前 Project 的受控平台执行。选择一条记录可继续检查、Prepare 或 Submit。</p></div><button className="secondary-button" onClick={() => void load()}><RefreshCw size={14}/>刷新</button></header>
    {visibleRuns.length ? <div className="controlled-execution-run-list-grid">{visibleRuns.map(run => {
      const presentation = presentControlledExecution(run)
      return <a key={run.id} href={`/projects/${encodeURIComponent(projectId)}/delivery/execution/${encodeURIComponent(run.id)}?view=${encodeURIComponent(activeView)}`} className={`controlled-execution-run-card ${presentation.tone}`}>
        <div><span>{runActionLabel(run.authority.action)}</span><b>{presentation.title}</b><small>{presentation.detail}</small></div>
        <dl><div><dt>广告账户</dt><dd>{run.account_id}</dd></div><div><dt>执行驱动</dt><dd>{executionDriverLabel(run)}</dd></div><div><dt>Run</dt><dd>{shortHash(run.id)}</dd></div><div><dt>更新时间</dt><dd>{formatTime(run.updated_at)}</dd></div></dl>
      </a>
    })}</div> : <div className="controlled-execution-run-empty"><Clock3 size={24}/><h3>{state.runs.length ? `${activeView}视图暂无记录` : '暂无执行记录'}</h3><p>{state.runs.length ? '请选择其他状态视图，或刷新执行记录。' : '请先在平台配置页检查计划，然后进入真实受控执行。'}</p></div>}
  </section>
}

function BrowserRpaExecutionDetail({ projectId, runId }: { projectId: string; runId: string }) {
  const [transport, setTransport] = useState<ControlledExecutionTransportState>(() => runId ? { kind: 'loading' } : { kind: 'empty' })
  const [notice, setNotice] = useState('')
  const [actionPending, setActionPending] = useState(false)
  const [plan, setPlan] = useState<RunnerV3Plan>()
  const [sessionProbe, setSessionProbe] = useState<EdgeSessionProbe>()
  const [reviewed, setReviewed] = useState(false)
  const [isRefreshPending, startRefreshTransition] = useTransition()
  const requestId = useRef(0)

  const load = useCallback(async (signal?: AbortSignal) => {
    if (!runId) {
      setTransport({ kind: 'empty' })
      return
    }
    const currentRequest = ++requestId.current
    setTransport(current => current.kind === 'ready' ? current : { kind: 'loading' })
    try {
      const workspace = await controlledExecutionApi.getWorkspace(projectId, runId, signal)
      if (currentRequest !== requestId.current || signal?.aborted) return
      startRefreshTransition(() => setTransport({ kind: 'ready', workspace }))
    } catch (error) {
      if (currentRequest !== requestId.current || signal?.aborted) return
      const message = error instanceof Error ? error.message : '读取受控执行中心失败。'
      setTransport(error instanceof ControlledExecutionApiError && error.status === 404
        ? { kind: 'empty' }
        : error instanceof ControlledExecutionApiError && error.status === 403
          ? { kind: 'forbidden', message }
          : { kind: 'error', message })
    }
  }, [projectId, runId, startRefreshTransition])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => {
      requestId.current += 1
      controller.abort()
    }
  }, [load])

  const observedRunState = transport.kind === 'ready' ? transport.workspace.run.state : undefined
  useEffect(() => {
    const transient = observedRunState && ['environment_check', 'preparing', 'submitting', 'verifying'].includes(observedRunState)
    if (!actionPending && !transient) return
    let stopped = false
    let timer: number | undefined
    const poll = async () => {
      await load()
      if (!stopped) timer = window.setTimeout(poll, 2_000)
    }
    timer = window.setTimeout(poll, 800)
    return () => {
      stopped = true
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [actionPending, load, observedRunState])

  const refresh = useCallback(() => {
    setNotice('')
    void load()
  }, [load])

  useEffect(() => {
    if (transport.kind !== 'ready' || !transport.workspace.lease || transport.workspace.run.state !== 'awaiting_confirmation') return
    const { run, lease } = transport.workspace
    let currentLease = lease
    let stopped = false
    const renew = () => {
      void controlledExecutionApi.heartbeatLease(projectId, run.id, currentLease).then(updated => {
        if (stopped) return
        currentLease = updated
        setTransport(current => current.kind === 'ready' && current.workspace.run.id === run.id
          ? { kind: 'ready', workspace: { ...current.workspace, lease: updated } }
          : current)
      }).catch(error => {
        if (!stopped) setNotice(error instanceof Error ? `租约续期失败：${error.message}` : '租约续期失败。')
      })
    }
    renew()
    const timer = window.setInterval(renew, 25_000)
    return () => {
      stopped = true
      window.clearInterval(timer)
    }
  }, [projectId, transport.kind === 'ready' ? transport.workspace.lease?.id : '', transport.kind === 'ready' ? transport.workspace.run.state : ''])

  const runWorkflow = useCallback(async (action: 'check' | 'plan' | 'prepare' | 'submit') => {
    if (transport.kind !== 'ready') return
    const { run } = transport.workspace
    const apiDriver = effectiveExecutionDriver(run) === 'oceanengine-web-api/session/v1'
    setActionPending(true)
    setNotice('')
    try {
      if (action === 'check') {
        const workspace = await controlledExecutionApi.getWorkspace(projectId, run.id)
        const probe = await controlledExecutionApi.checkSession(projectId, run.id)
        setTransport({ kind: 'ready', workspace })
        setSessionProbe(probe)
        setNotice(registeredBindingReady(workspace) && probe.status === 'ready' ? 'Edge 会话可用。CDP、登录状态和广告账户均匹配。' : 'Edge 会话不可用。请修复下方失败项。')
      } else if (action === 'plan') {
        const nextPlan = await controlledExecutionApi.generatePlan(projectId, run.id)
        setPlan(nextPlan)
        setNotice(nextPlan.blocked_reasons.length ? '计划已生成，但存在阻塞原因。' : apiDriver ? 'API 编译输入已生成。该操作未写入平台。' : 'Runner v3 执行计划已生成。该操作未打开页面。')
      } else if (action === 'prepare') {
        let currentRun = run
        const currentLease = transport.workspace.lease
        const leaseActive = Boolean(
          currentLease
          && currentLease.id === currentRun.lease_id
          && !currentLease.released_at
          && new Date(currentLease.heartbeat_deadline).getTime() > Date.now(),
        )
        if (!leaseActive) {
          const acquired = await controlledExecutionApi.acquireLease(projectId, currentRun.id, currentRun.version)
          currentRun = acquired.run
        }
        setNotice(apiDriver ? 'Prepare 已启动。系统正在检查 Connector 会话和请求 DTO。' : 'Prepare 已启动。Runner v3 正在进入巨量表单并执行字段回读。该过程最长需要 3 分钟。')
        const prepared = await controlledExecutionApi.prepare(projectId, currentRun.id)
        if (prepared.state !== 'awaiting_confirmation') {
          const result = presentControlledExecution(prepared)
          setNotice(`Prepare 未完成：${result.title}。${result.detail}${prepared.blocking_reason ? ` 阻塞代码：${prepared.blocking_reason}` : ''}`)
          await load()
          return
        }
        setReviewed(false)
        setNotice('Prepare 已完成。请检查字段回读和差异。')
        await load()
      } else {
        const lease = transport.workspace.lease
        if (!lease) throw new Error('缺少有效租约。请刷新页面后重试。')
        const confirmation = await controlledExecutionApi.confirm(projectId, run)
        // Keep the token only in this call stack. Do not persist it in UI state.
        const updated = await controlledExecutionApi.submit(projectId, run, lease, confirmation)
        setReviewed(false)
        if (updated.state === 'environment_check') {
          setPlan(undefined)
          setSessionProbe(undefined)
          setNotice(apiDriver ? '当前对象已创建并回写平台 ID。请生成下一个对象的 API 编译输入。' : '当前对象已创建并回写平台 ID。请重新检查 Edge 会话，然后生成下一个对象计划。')
        } else {
          setNotice('Submit 已执行。请检查平台结果和写后证据。')
        }
        await load()
      }
    } catch (error) {
      if (action === 'plan') setPlan(undefined)
      setNotice(error instanceof Error ? error.message : '执行请求失败。')
      await load()
    } finally {
      setActionPending(false)
    }
  }, [load, projectId, transport])

  const runControl = useCallback(async (action: 'pause' | 'resume' | 'cancel' | 'takeover' | 'release_takeover') => {
    if (transport.kind !== 'ready') return
    const { run } = transport.workspace
    setActionPending(true)
    setNotice('')
    try {
      const updated = await controlledExecutionApi[
        action === 'takeover' ? 'takeOver' : action === 'release_takeover' ? 'releaseTakeover' : action
      ](projectId, run.id, run.version)
      setTransport(current => current.kind === 'ready' && current.workspace.run.id === run.id
        ? { kind: 'ready', workspace: { ...current.workspace, run: updated } }
        : current)
      setNotice(controlNotice(action))
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '运行控制请求失败。')
    } finally {
      setActionPending(false)
    }
  }, [projectId, transport])

  const retryPrepare = useCallback(async () => {
    if (transport.kind !== 'ready') return
    const { run } = transport.workspace
    if (!isSafePrepareRetryCandidate(transport.workspace)) {
      setNotice('当前失败不允许普通重试。请先修复配置或账号问题，或执行结果识别。')
      return
    }
    if (!run.authority.plan_id || !run.authority.plan_version) {
      setNotice('当前 Run 缺少投放计划版本，不能创建安全重试。')
      return
    }
    setActionPending(true)
    setNotice('正在重新检查固定计划。系统不会修改或删除失败 Run。')
    try {
      const retryPlan = await controlledExecutionApi.generatePlan(projectId, run.id)
      setPlan(retryPlan)
      if (retryPlan.blocked_reasons.length || retryPlan.configuration_issues?.length) {
        setNotice('固定计划仍有配置阻塞。请先修复平台配置，再创建新执行。')
        return
      }
      const result = await deliveryExecutionApi.startBrowserRpaExecution(
        projectId,
        run.authority.plan_id,
        run.authority.plan_version,
        effectiveExecutionDriver(run),
        `browser-rpa-retry-${run.id}-${crypto.randomUUID()}`,
      )
      window.location.assign(`/projects/${encodeURIComponent(projectId)}/delivery/execution/${encodeURIComponent(result.browser_rpa_run.run_id)}?view=${encodeURIComponent('待执行')}`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '创建安全重试失败。')
    } finally {
      setActionPending(false)
    }
  }, [projectId, transport])

  const reconcileResult = useCallback(async () => {
    if (transport.kind !== 'ready' || transport.workspace.run.state !== 'result_unknown') return
    const { run } = transport.workspace
    setActionPending(true)
    setNotice('正在只读查询巨量列表。系统不会再次点击 Submit。')
    try {
      const updated = await controlledExecutionApi.reconcileResult(projectId, run.id)
      setNotice(updated.state === 'failed'
        ? '只读查询已确认目标对象不存在。现在可以创建安全重试。'
        : '只读查询已找到目标对象，并完成平台 ID 回写。')
      await load()
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '只读结果核对失败。')
      await load()
    } finally {
      setActionPending(false)
    }
  }, [load, projectId, transport])

  if (transport.kind === 'loading') return <WorkspaceState kind="loading" />
  if (transport.kind === 'empty') return <WorkspaceState kind="empty" />
  if (transport.kind === 'forbidden') return <WorkspaceState kind="forbidden" message={transport.message} />
  if (transport.kind === 'error') return <WorkspaceState kind="error" message={transport.message} onRetry={refresh} />

  return <WorkspaceReady
    workspace={transport.workspace}
    busy={actionPending || isRefreshPending}
    notice={notice}
    plan={plan}
    sessionProbe={sessionProbe}
    reviewed={reviewed}
    onReviewed={setReviewed}
    onWorkflow={runWorkflow}
    onRefresh={refresh}
    onControl={runControl}
    onRetryPrepare={retryPrepare}
    onReconcileResult={reconcileResult}
  />
}

function runActionLabel(action: string): string {
  if (action === 'create_project_and_promotions') return '新建项目和单元'
  if (action === 'create_promotions_in_existing_project') return '在已有项目中新建单元'
  if (action === 'update_promotion_budget') return '修改单元预算'
  return '受控平台操作'
}

function WorkspaceReady({ workspace, busy, notice, plan, sessionProbe, reviewed, onReviewed, onWorkflow, onRefresh, onControl, onRetryPrepare, onReconcileResult }: {
  workspace: Extract<ControlledExecutionTransportState, { kind: 'ready' }>['workspace']
  busy: boolean
  notice: string
  plan: RunnerV3Plan | undefined
  sessionProbe: EdgeSessionProbe | undefined
  reviewed: boolean
  onReviewed: (value: boolean) => void
  onWorkflow: (action: 'check' | 'plan' | 'prepare' | 'submit') => void
  onRefresh: () => void
  onControl: (action: 'pause' | 'resume' | 'cancel' | 'takeover' | 'release_takeover') => void
  onRetryPrepare: () => void
  onReconcileResult: () => void
}) {
  const { run, events, evidence } = workspace
  const presentation = useMemo(() => presentControlledExecution(run), [run])
  const apiDriver = effectiveExecutionDriver(run) === 'oceanengine-web-api/session/v1'
  const terminal = isTerminalControlledExecutionState(run.state)
  // A Kill Switch blocks new writes, never the operator's ability to take over,
  // pause, cancel, or inspect the run.
  const showTakeover = !terminal

  return <section className="controlled-execution-workspace" aria-label="受控执行中心">
    <header className="controlled-execution-header">
      <div>
        <span className="section-label">Controlled platform execution</span>
        <h2>受控执行中心</h2>
        <p>{apiDriver ? '按顺序检查 Connector 会话、生成 API 编译输入、执行 Prepare，并复核写入门禁。' : '按顺序检查真实 Edge 会话、生成计划、执行 Prepare、复核差异，再使用一次性授权执行 Submit。'}</p>
      </div>
      <button className="secondary-button" onClick={onRefresh} disabled={busy}><RefreshCw size={15} />从服务端刷新</button>
    </header>

    <AuthorityChain run={run} />
    <StatusBanner presentation={presentation} />
    <ExecutionFlowPanel
      workspace={workspace}
      plan={plan}
      busy={busy}
      sessionProbe={sessionProbe}
      reviewed={reviewed}
      onReviewed={onReviewed}
      onWorkflow={onWorkflow}
      onRetryPrepare={onRetryPrepare}
      onReconcileResult={onReconcileResult}
    />
    <SessionAndTargetPanel workspace={workspace} sessionProbe={sessionProbe} />
    {plan ? <PlanPanel plan={plan} run={run} /> : null}
    {evidence.length ? <ReadbackPanel evidence={evidence} /> : null}
    {evidence.length ? <CreatedObjectsPanel evidence={evidence} /> : null}
    {run.authority.promotion_mutation || run.authority.promotion_control || run.authority.promotion_restart ? <PromotionChangeDiff run={run} /> : null}

    <div className="controlled-execution-layout">
      <section className="controlled-execution-main" aria-label="运行状态与步骤">
        <RunTimeline run={run} />
        <ControlPanel run={run} busy={busy} terminal={terminal} showTakeover={showTakeover} onControl={onControl} />
        <RecoveryPanel kind={presentation.kind} />
        <PlatformResultPanel run={run} evidence={evidence} />
        <EvidencePanel evidence={evidence} events={events} />
      </section>
      <aside className="controlled-execution-audit" aria-label="授权与审计摘要">
        <dl>
          <div><dt>Run</dt><dd title={run.id}>{run.id}</dd></div>
          <div><dt>账户</dt><dd>{run.account_id}</dd></div>
          <div><dt>执行驱动</dt><dd>{executionDriverLabel(run)}</dd></div>
          <div><dt>ChangeSet</dt><dd title={run.authority.change_set_id}>{run.authority.change_set_id}</dd></div>
          <div><dt>正式 Approval</dt><dd title={run.authority.approval_id}>{run.authority.approval_id}</dd></div>
          {run.authority.target_mapping_id ? <div><dt>目标映射版本</dt><dd title={run.authority.target_mapping_id}>{shortHash(run.authority.target_mapping_id)} · v{run.authority.target_mapping_version}</dd></div> : null}
          {run.authority.target_platform_object_id ? <div><dt>目标推广单元</dt><dd title={run.authority.target_platform_object_id}>{shortHash(run.authority.target_platform_object_id)}</dd></div> : null}
          {run.authority.operator_principal_id ? <div><dt>绑定操作人</dt><dd title={run.authority.operator_principal_id}>{run.authority.operator_principal_id}</dd></div> : null}
          <div><dt>预算上限</dt><dd>¥{formatMinor(run.authority.budget_limit_minor)} {run.authority.currency}</dd></div>
          <div><dt>Workflow</dt><dd title={run.authority.workflow_canonical_hash}>{shortHash(run.authority.workflow_canonical_hash)}</dd></div>
          <div><dt>Platform Skill</dt><dd>{run.authority.skill_id && run.authority.skill_version ? <>{run.authority.skill_id} · {run.authority.skill_version}<small>仅代表已校准路径；执行当轮仍须复核页面和字段。</small></> : '未绑定；真实执行不可用'}</dd></div>
          <div><dt>租约</dt><dd title={run.lease_id}>{run.lease_id}</dd></div>
          <div><dt>策略</dt><dd title={run.policy_id}>{run.policy_id}</dd></div>
        </dl>
      </aside>
    </div>
    {notice ? <div className="controlled-execution-notice" role="status">{notice}</div> : null}
  </section>
}

function ExecutionFlowPanel({ workspace, plan, busy, sessionProbe, reviewed, onReviewed, onWorkflow, onRetryPrepare, onReconcileResult }: {
  workspace: ControlledExecutionWorkspace
  plan?: RunnerV3Plan
  busy: boolean
  sessionProbe: EdgeSessionProbe | undefined
  reviewed: boolean
  onReviewed: (value: boolean) => void
  onWorkflow: (action: 'check' | 'plan' | 'prepare' | 'submit') => void
  onRetryPrepare: () => void
  onReconcileResult: () => void
}) {
  const { run, steps: runSteps, events, evidence, lease } = workspace
  const apiDriver = effectiveExecutionDriver(run) === 'oceanengine-web-api/session/v1'
  const sessionReady = apiDriver ? registeredBindingReady(workspace) : Boolean(sessionProbe?.status === 'ready' && sessionProbe.cdp_available && sessionProbe.logged_in && sessionProbe.account_matched)
  const prepareStarted = events.some(event => event.kind === 'state_transition' && event.summary.includes('-> preparing'))
    || ['preparing', 'awaiting_confirmation', 'submitting', 'verifying', 'succeeded', 'partial', 'result_unknown'].includes(run.state)
  const restoredSessionReady = prepareStarted && run.blocking_reason !== 'ACCOUNT_MISMATCH'
  const bindingReady = registeredBindingReady(workspace) && (sessionReady || restoredSessionReady)
  const actionSupported = runnerV3ActionSupported(run.authority.action)
  const generatedPlanReady = Boolean(plan && plan.blocked_reasons.length === 0 && !plan.allow_remote_write)
  const planReady = generatedPlanReady || prepareStarted
  const prepared = ['awaiting_confirmation', 'submitting', 'verifying', 'succeeded', 'failed', 'partial', 'result_unknown'].includes(run.state)
  const drift = fieldDrift(evidence)
  const leaseReady = Boolean(lease && !lease.released_at && new Date(lease.heartbeat_deadline).getTime() > Date.now())
  const canPrepare = sessionReady && actionSupported && generatedPlanReady && (run.state === 'queued' || run.state === 'environment_check')
  const canSubmit = run.state === 'awaiting_confirmation' && prepared && reviewed && !drift && leaseReady
  const submitStarted = ['submitting', 'verifying', 'succeeded', 'partial', 'result_unknown'].includes(run.state)
  const flowSteps = [
    { label: apiDriver ? '检查 Connector 会话' : '检查真实 Edge 会话', done: bindingReady, active: !bindingReady },
    { label: apiDriver ? '生成 API 编译输入' : '生成 Runner v3 计划', done: planReady, active: bindingReady && !planReady },
    { label: '执行 Prepare', done: prepared, active: run.state === 'preparing' },
    { label: '复核回读与差异', done: prepared && reviewed, active: run.state === 'awaiting_confirmation' && !reviewed },
    { label: '一次性确认并 Submit', done: submitStarted, active: run.state === 'submitting' || run.state === 'verifying' },
  ]
  const prepareStep = runSteps.find(step => step.action === 'prepare_and_readback')
  const canRetryPrepare = isSafePrepareRetryCandidate(workspace)
  return <section className="controlled-execution-flow" aria-label="执行操作闭环">
    <header><div><span className="section-label">Operation flow</span><h3>执行操作闭环</h3></div><small>Submit 会跨越最终点击边界。确认令牌仅在当前请求内存中存在。</small></header>
    <ol>{flowSteps.map((step, index) => <li key={step.label} className={step.done ? 'complete' : step.active ? 'active' : ''}><span>{step.done ? <CircleCheck size={15} /> : index + 1}</span>{step.label}</li>)}</ol>
    {prepareStep ? <p className={`controlled-execution-step-status ${prepareStep.status}`}><Clock3 size={14} />Prepare 服务端任务：{runStepStatusLabel(prepareStep.status)}{prepareStep.blocking_reason ? ` · ${prepareStep.blocking_reason}` : ''}</p> : null}
    <div className="controlled-execution-flow-actions">
      {!apiDriver ? <button className="secondary-button" disabled={busy || isTerminalControlledExecutionState(run.state)} onClick={() => onWorkflow('check')}><MonitorCheck size={15} />检查 Edge 会话</button> : null}
      <button className="secondary-button" disabled={busy || !bindingReady || !actionSupported || isTerminalControlledExecutionState(run.state)} onClick={() => onWorkflow('plan')}><ListChecks size={15} />生成计划</button>
      <button className="secondary-button" disabled={busy || !canPrepare} onClick={() => onWorkflow('prepare')}><Play size={15} />执行 Prepare</button>
      {canRetryPrepare ? <button className="secondary-button" disabled={busy} onClick={onRetryPrepare}><RefreshCw size={15} />重试 Prepare</button> : null}
      {run.state === 'result_unknown' && !apiDriver ? <button className="secondary-button" disabled={busy} onClick={onReconcileResult}><Search size={15} />只读查询平台结果</button> : null}
    </div>
    {canRetryPrepare ? <p className="controlled-execution-retry-note">重试会创建新 Run。失败 Run 和证据会保留。服务端会再次检查最终点击边界。</p> : null}
    {!actionSupported ? <p className="danger-copy">当前动作没有 Runner v3 单表单协议。系统不会生成可执行计划。</p> : null}
    {run.state === 'awaiting_confirmation' ? <div className="controlled-execution-confirm">
      <label><input type="checkbox" checked={reviewed} onChange={event => onReviewed(event.target.checked)} disabled={busy || drift} />我已核对当前账户、目标对象、字段回读、差异和最终点击边界。</label>
      <button className="primary-button" disabled={busy || !canSubmit} onClick={() => onWorkflow('submit')}><Send size={15} />确认并执行 Submit</button>
      {!leaseReady ? <small>租约已缺失或过期。刷新页面后重新取得有效运行状态。</small> : null}
      {drift ? <small>检测到字段漂移。系统阻止 Submit。</small> : null}
    </div> : null}
  </section>
}

function SessionAndTargetPanel({ workspace, sessionProbe }: { workspace: ControlledExecutionWorkspace; sessionProbe: EdgeSessionProbe | undefined }) {
  const { run, environment, profile, policy } = workspace
  const apiDriver = effectiveExecutionDriver(run) === 'oceanengine-web-api/session/v1'
  const accountMatches = environment.account_id === run.account_id && profile.account_id === run.account_id && policy.account_id === run.account_id
  const projectAllowed = !run.authority.parent_platform_project_id || policy.allowed_platform_project_ids.includes(run.authority.parent_platform_project_id)
  const registered = registeredBindingReady(workspace)
  const ready = registered && (apiDriver || sessionProbe?.status === 'ready')
  return <section className="controlled-execution-context" aria-label="执行会话和目标">
    <article className={ready ? 'ready' : 'blocked'}><header><MonitorCheck size={18} /><b>{apiDriver ? 'Connector 组织账号会话' : '真实 Edge 会话'}</b></header><dl>
      <div><dt>控制面登记</dt><dd>{registered ? '一致' : '不一致'}</dd></div>
      <div><dt>环境</dt><dd>{apiDriver ? environment.mode : `${environment.mode} · Edge ${environment.browser_version}`}</dd></div>
      <div><dt>Profile</dt><dd>{profile.state}</dd></div>
      <div><dt>登记账户一致</dt><dd>{accountMatches ? '是' : '否'}</dd></div>
      {apiDriver ? <div><dt>会话检查</dt><dd>Prepare 时读取 ready 会话</dd></div> : <><div><dt>DevTools WebSocket</dt><dd>{sessionProbe ? sessionProbe.cdp_available ? '可用' : '不可用' : '等待检查'}</dd></div><div><dt>巨量页面已登录</dt><dd>{sessionProbe ? sessionProbe.logged_in ? '是' : '否' : '等待检查'}</dd></div><div><dt>页面账户匹配</dt><dd>{sessionProbe ? sessionProbe.account_matched ? '是' : '否' : '等待检查'}</dd></div>{sessionProbe ? <><div><dt>结果</dt><dd>{sessionProbeReason(sessionProbe.reason)}</dd></div><div><dt>检查时间</dt><dd>{formatTime(sessionProbe.checked_at)}</dd></div></> : null}</>}
    </dl></article>
    <article className={projectAllowed ? 'ready' : 'blocked'}><header><FileCheck2 size={18} /><b>平台目标</b></header><dl>
      <div><dt>当前广告账户</dt><dd>{run.account_id}</dd></div>
      <div><dt>目标项目</dt><dd>{run.authority.parent_platform_project_id || '新建项目'}</dd></div>
      <div><dt>目标单元</dt><dd>{run.authority.target_platform_object_id || '新建投放单元'}</dd></div>
      <div><dt>账号路径</dt><dd>{projectAllowed ? '策略允许' : '策略阻止'}</dd></div>
    </dl></article>
  </section>
}

function effectiveExecutionDriver(run: BrowserRpaRun) {
  return run.execution_driver ?? 'playwright-rpa/edge/v3'
}

function executionDriverLabel(run: BrowserRpaRun) {
  return effectiveExecutionDriver(run) === 'oceanengine-web-api/session/v1' ? 'Web API 会话' : 'Playwright · Edge'
}

function runStepStatusLabel(status: string) {
  return ({ pending: '等待执行', running: '正在操作页面', succeeded: '已完成字段回读', failed: '执行失败', result_unknown: '结果未知', skipped: '已跳过' } as Record<string, string>)[status] ?? status
}

function PlanPanel({ plan, run }: { plan: RunnerV3Plan; run: BrowserRpaRun }) {
  const fields = plan.steps.filter(step => step.kind === 'field_action')
  const objectAvailability = plan.object_availability ?? []
  const objectPresentations = objectAvailability.map(presentObjectAvailability)
  const unavailableCount = objectPresentations.filter(item => !item.available).length
  const configurationIssues = plan.configuration_issues ?? []
  const blocked = plan.blocked_reasons.length > 0
  const boundary = plan.steps.find(step => step.remote_write) ?? plan.steps.at(-1)
  return <section className="controlled-execution-plan" aria-label="Runner v3 执行计划">
    <header><div><span className="section-label">Runner v3 plan</span><h3>{plan.plan_kind}</h3></div><span className={plan.blocked_reasons.length ? 'blocked' : 'ready'}>{plan.blocked_reasons.length ? '计划被阻止' : '计划可执行'}</span></header>
    <div className="controlled-execution-plan-summary"><span>账户 <b>{plan.account_reference}</b></span><span>当前阶段 <b>{plan.internal_object_kind === 'project' ? '创建项目' : plan.internal_object_kind === 'promotion' ? '创建单元' : plan.plan_kind}</b></span><span>Cookies 对象 <b>{plan.internal_object_id || '未提供'}</b></span><span>父项目 <b>{plan.parent_project_reference || '等待项目回写'}</b></span><span>字段 <b>{fields.length}</b></span></div>
    {plan.blocked_reasons.length ? <p className="danger-copy">{plan.blocked_reasons.map(presentPlanBlockedReason).join('；')}</p> : null}
    {configurationIssues.length ? <section className="controlled-execution-configuration-issues" aria-label="投放配置需补充">
      <h4>投放配置需补充</h4>
      {run.authority.plan_version ? <p>此执行记录固定使用投放计划 v{run.authority.plan_version}。当前计划的后续修改不会更新此记录。</p> : null}
      <ul>{configurationIssues.map((issue, index) => <li key={`${index}-${issue}`}>{presentConfigurationIssue(issue)}</li>)}</ul>
      <p>如果当前计划已修正，请返回投放配置页并创建新执行。</p>
    </section> : null}
    {objectAvailability.length ? <section className="controlled-execution-object-availability" aria-label="巨量对象可用性">
      <header><div><h4>巨量对象检查</h4><small>已检查 {objectAvailability.length} 项。{objectAvailability.length - unavailableCount} 项可用，{unavailableCount} 项需处理。</small></div><span className={unavailableCount ? 'blocked' : 'ready'}>{unavailableCount ? `${unavailableCount} 项需处理` : '全部可用'}</span></header>
      {objectPresentations.map((item, index) => <article key={`${objectAvailability[index].field_key}-${objectAvailability[index].internal_object_id}`} className={item.available ? 'ready' : 'blocked'}>
        <div className="controlled-execution-object-identity">
          <small>{item.scopeLabel} · {item.kindLabel}</small>
          <b>{item.name}</b>
        </div>
        <div className="controlled-execution-object-status">
          <strong>{item.statusLabel}</strong>
          <small>{item.statusDetail}</small>
          {item.platformId ? <span>平台 ID：<code>{item.platformId}</code></span> : null}
          <details><summary>技术信息</summary><code>{objectAvailability[index].field_key}</code><span>对象类型：{item.technicalType}</span></details>
        </div>
      </article>)}
    </section> : null}
    <details><summary>查看字段计划和目标值</summary><div className="controlled-execution-plan-fields">{fields.map(step => <div key={step.id}><b>{step.field_key}</b><span>{step.operation}</span><code>{formatPlanValue(step.value)}</code></div>)}</div></details>
    <div className="controlled-execution-boundary"><ShieldAlert size={18} /><div><b>远程写入边界</b><span>{blocked ? '未开放' : boundary?.scope || boundary?.target || boundary?.id || '未定义'}</span><small>{blocked ? '执行前检查未通过。系统不会打开平台页面。' : `Prepare：禁止远端写入。Submit：最多 ${Math.max(1, plan.maximum_final_clicks || 1)} 次最终点击。`}</small></div></div>
  </section>
}

function CreatedObjectsPanel({ evidence }: { evidence: BrowserRpaEvidence[] }) {
  const objects = new Map<string, { internalId: string; platformId: string }>()
  for (const item of evidence) {
    const readback = item.field_readback ?? item.after_page_facts ?? {}
    if (readback.reconciliation === 'matched' && readback.platform_object_id) {
      objects.set(item.object_fingerprint, { internalId: item.object_fingerprint, platformId: readback.platform_object_id })
    }
  }
  if (!objects.size) return null
  return <section className="controlled-execution-created-objects" aria-label="已匹配平台对象">
    <header><div><span className="section-label">Runner reconciliation</span><h3>已匹配平台对象</h3></div><span>{objects.size} 个</span></header>
    {[...objects.values()].map(item => <div key={item.internalId}><b>{item.internalId}</b><code>{item.platformId}</code></div>)}
  </section>
}

function ReadbackPanel({ evidence }: { evidence: BrowserRpaEvidence[] }) {
  const latest = evidence.at(-1)
  const readback = latest?.field_readback ?? latest?.after_page_facts ?? {}
  const rows = Object.entries(readback).filter(([key]) => !key.startsWith('plan_diff.'))
  const planned = Object.entries(readback).filter(([key]) => key.startsWith('plan_diff.') && key.endsWith('.target'))
  const drift = fieldDrift(evidence)
  return <section className="controlled-execution-readback" aria-label="Prepare 字段回读和差异">
    <header><div><span className="section-label">Prepare result</span><h3>字段回读和差异</h3></div><span className={drift ? 'blocked' : 'ready'}>{drift ? '检测到字段漂移' : '未检测到字段漂移'}</span></header>
    <div className="controlled-execution-readback-grid"><div><h4>字段回读</h4>{rows.length ? rows.map(([key, value]) => <div key={key}><b>{key}</b><span>{value}</span></div>) : <p>Evidence 未返回字段回读。</p>}</div><div><h4>计划差异</h4>{planned.length ? planned.map(([key, value]) => <div key={key}><b>{key.slice(10, -7)}</b><span>{value}</span></div>) : <p>{latest?.diff_keys.length ? latest.diff_keys.join('、') : '无计划差异。'}</p>}</div></div>
  </section>
}

function PlatformResultPanel({ run, evidence }: { run: BrowserRpaRun; evidence: BrowserRpaEvidence[] }) {
  if (!isTerminalControlledExecutionState(run.state)) return null
  const latest = evidence.at(-1)
  const readback = latest?.field_readback ?? latest?.after_page_facts ?? {}
  const objectID = readback.platform_object_id || run.authority.target_platform_object_id
  return <section className={`controlled-execution-platform-result ${run.state}`} aria-label="平台执行结果">
    <header><CircleCheck size={18} /><div><span className="section-label">Platform result</span><h3>{run.state}</h3></div></header>
    <dl><div><dt>平台对象 ID</dt><dd>{objectID || '平台未返回对象 ID'}</dd></div><div><dt>字段校验</dt><dd>{readback.field_reconciliation_status || '未返回'}</dd></div><div><dt>结果证据</dt><dd>{latest?.page_reference || '无'}</dd></div></dl>
  </section>
}

function registeredBindingReady(workspace: ControlledExecutionWorkspace) {
  const { run, environment, profile, policy } = workspace
  return environment.healthy && profile.state === 'ready'
    && environment.account_id === run.account_id && profile.account_id === run.account_id && policy.account_id === run.account_id
}

function runnerV3ActionSupported(action: string) {
  return action === 'create_project_and_promotions'
    || action === 'create_promotions_in_existing_project'
    || action === 'update_promotion_budget'
}

function fieldDrift(evidence: BrowserRpaEvidence[]) {
  return evidence.some(item => {
    const readback = item.field_readback ?? item.after_page_facts ?? {}
    return readback.field_reconciliation_status === 'drifted'
  })
}

function formatPlanValue(value: unknown) {
  if (typeof value === 'string') return value
  if (value === undefined) return '未提供'
  return JSON.stringify(value)
}

function AuthorityChain({ run }: { run: BrowserRpaRun }) {
  const changing = Boolean(run.authority.promotion_mutation || run.authority.promotion_control || run.authority.promotion_restart)
  const emergencyPause = run.authority.action === 'pause_promotion'
  const controlledRestart = run.authority.action === 'resume_promotion'
  const formalApproved = run.blocking_reason !== 'APPROVAL_INVALID'
  const confirmationReady = ['submitting', 'verifying', 'succeeded', 'failed', 'partial', 'result_unknown'].includes(run.state)
    && run.blocking_reason !== 'FINAL_CONFIRMATION_INVALID'
  return <ol className="controlled-execution-authority-chain" aria-label="受控写入授权链">
    <li className="complete"><span>1</span><div><b>{emergencyPause ? '核对投放状态并创建紧急暂停' : controlledRestart ? '完成全部重检并创建受控重启' : changing ? '读取当前值并创建新变更' : '接受优化方案'}</b><small>{changing ? '当前值、目标值、对象、操作人和 Mapping 版本已经冻结；创建时的审批不可复用。' : '已接受/修改的反馈才可创建 ChangeSet；这不是写入批准。'}</small></div></li>
    <li className={formalApproved ? 'complete' : 'blocked'}><span>2</span><div><b>批准平台写入</b><small>正式 Approval 绑定账户、预算、配置、Workflow 与阶段 B Skill 校准版本；这不代表实时 DOM 已复核。</small></div></li>
    <li className={confirmationReady ? 'complete' : 'waiting'}><span>3</span><div><b>一次性最终确认</b><small>仅对当前 Run 有效；签发或过期都不等于已经提交。</small></div></li>
  </ol>
}

function PromotionChangeDiff({ run }: { run: BrowserRpaRun }) {
  const mutation = run.authority.promotion_mutation
  const control = run.authority.promotion_control
  const restart = run.authority.promotion_restart
  if (!mutation && !control && !restart) return null
  const emergencyPause = run.authority.action === 'pause_promotion' && Boolean(control)
  const controlledRestart = run.authority.action === 'resume_promotion' && Boolean(restart)
  const actionLabel = ({
    update_promotion_budget: '修改推广单元日预算',
    update_promotion_materials: '更换或增减授权素材',
    pause_promotion: '紧急暂停推广单元',
    resume_promotion: '受控重启推广单元',
  } as Record<string, string>)[run.authority.action] ?? '修改推广单元'
  const rows = restart
    ? [
        { label: '投放状态', current: statusLabel(restart.current_platform_status), target: statusLabel(restart.target_platform_status) },
        { label: '每日预算（重新核准）', current: `¥${formatMinor(restart.current_daily_budget_minor)}`, target: `¥${formatMinor(restart.approved_daily_budget_minor)}` },
        { label: '当前有效排期', current: formatSchedule(restart.schedule), target: '校验通过后保持不变' },
        { label: '授权素材可用性', current: `${restart.materials.length} 项`, target: '最终点击前再次核对' },
        { label: '授权落地页可用性', current: '已绑定 1 个授权落地页', target: '最终点击前再次核对' },
      ]
    : control
    ? [
        { label: '投放状态', current: statusLabel(control.current_platform_status), target: statusLabel(control.target_platform_status) },
        { label: '每日预算（保持不变）', current: `¥${formatMinor(control.current_daily_budget_minor)}`, target: `¥${formatMinor(control.current_daily_budget_minor)}` },
      ]
    : mutationRows(run.authority.action, mutation!)
  return <section className={`controlled-execution-mutation${emergencyPause ? ' emergency' : ''}${controlledRestart ? ' restart' : ''}`} aria-label="当前值与目标值">
    <header><div><span className="section-label">{emergencyPause ? '高优先级止损动作' : controlledRestart ? '严格重检后的独立动作' : '本次受控变更'}</span><h3>{actionLabel}</h3></div><small>{emergencyPause ? '这是平台推广单元的状态变更，不是暂停本页执行流程。只允许单击一次，随后必须回读“已暂停”。' : controlledRestart ? '重启不是自动补偿。账户、父项目、对象、预算、有效排期、素材、落地页和 Kill Switch 任一不通过都会阻止最终确认。' : '提交前必须逐字段回读一致；任何差异都会阻止一次性确认。'}</small></header>
    <div className="controlled-execution-mutation-table" role="table" aria-label="变更差异">
      <div className="heading" role="row"><b role="columnheader">字段</b><b role="columnheader">当前值</b><b role="columnheader">目标值</b></div>
      {rows.map(row => <div key={row.label} role="row"><span role="cell">{row.label}</span><strong role="cell">{row.current}</strong><strong role="cell" className="target">{row.target}</strong></div>)}
    </div>
  </section>
}

function statusLabel(status: string) {
  return ({ delivering: '投放中', paused: '已暂停' } as Record<string, string>)[status] ?? status
}

function mutationRows(action: string, mutation: NonNullable<BrowserRpaRun['authority']['promotion_mutation']>) {
  if (action === 'update_promotion_materials') {
    return [{ label: '已授权素材', current: `${mutation.current_materials?.length ?? 0} 个`, target: `${mutation.target_materials?.length ?? 0} 个` }]
  }
  return [{ label: '每日预算', current: `¥${formatMinor(mutation.current_daily_budget_minor)}`, target: `¥${formatMinor(mutation.target_daily_budget_minor)}` }]
}

function formatSchedule(value?: { start_at: string; end_at: string; timezone: string }) {
  if (!value) return '未提供'
  return `${formatTime(value.start_at)} 至 ${formatTime(value.end_at)}（${value.timezone}）`
}

function StatusBanner({ presentation }: { presentation: ReturnType<typeof presentControlledExecution> }) {
  const Icon = presentation.tone === 'success' ? CircleCheck : presentation.tone === 'neutral' ? Clock3 : ShieldAlert
  return <div className={`controlled-execution-status ${presentation.tone}`} role={presentation.tone === 'danger' ? 'alert' : 'status'}>
    <Icon size={20} />
    <span><b>{presentation.title}</b><small>{presentation.detail}</small></span>
  </div>
}

function RunTimeline({ run }: { run: BrowserRpaRun }) {
  const steps: Array<{ state: BrowserRpaRun['state']; label: string }> = [
    { state: 'environment_check', label: '环境检查' },
    { state: 'preparing', label: '准备表单' },
    { state: 'awaiting_confirmation', label: '核对差异与等待确认' },
    { state: 'submitting', label: '受控提交' },
    { state: 'verifying', label: '写后验证' },
  ]
  const activeIndex = isTerminalControlledExecutionState(run.state)
    ? steps.length
    : Math.max(0, steps.findIndex(step => step.state === run.state))
  return <section className="controlled-execution-timeline" aria-label="执行阶段">
    <h3>执行阶段</h3>
    <ol>{steps.map((step, index) => <li key={step.state} className={index < activeIndex ? 'complete' : index === activeIndex ? 'active' : ''}><span>{index + 1}</span>{step.label}</li>)}</ol>
    {run.paused ? <p><Pause size={14} />执行流程已暂停；这不代表平台推广单元已经暂停。恢复流程时必须重新识别页面与账户。</p> : null}
  </section>
}

function ControlPanel({ run, busy, terminal, showTakeover, onControl }: {
  run: BrowserRpaRun
  busy: boolean
  terminal: boolean
  showTakeover: boolean
  onControl: (action: 'pause' | 'resume' | 'cancel' | 'takeover' | 'release_takeover') => void
}) {
  return <section className="controlled-execution-controls" aria-label="运行控制">
    <div><span className="section-label">运行控制</span><h3>执行流程的暂停、接管和取消</h3><p>这里仅控制 Playwright RPA 执行流程，不会暂停广告平台上的推广单元；平台紧急暂停必须使用独立 ChangeSet 和 Approval。</p></div>
    <div className="controlled-execution-control-actions">
      {run.paused ? <button className="secondary-button" onClick={() => onControl('resume')} disabled={busy || terminal}>恢复并重新识别</button> : <button className="secondary-button" onClick={() => onControl('pause')} disabled={busy || terminal}>暂停执行流程</button>}
      {showTakeover ? <button className="secondary-button" onClick={() => onControl(run.takeover_active ? 'release_takeover' : 'takeover')} disabled={busy || terminal}><Hand size={15} />{run.takeover_active ? '释放接管' : '人工接管'}</button> : null}
      <button className="danger-button" onClick={() => onControl('cancel')} disabled={busy || terminal}><XCircle size={15} />取消运行</button>
    </div>
  </section>
}

function RecoveryPanel({ kind }: { kind: ReturnType<typeof presentControlledExecution>['kind'] }) {
  if (kind === 'result_unknown') return <section className="controlled-execution-recovery danger"><CircleAlert size={18} /><div><b>未知结果处理指令</b><p>不要再次 Submit。先查询平台对象，再重新识别当前页面。必要时执行人工接管。确认平台结果后，创建新的补偿计划。</p></div></section>
  if (kind === 'partial') return <section className="controlled-execution-recovery"><CircleAlert size={18} /><div><b>已完成范围保留，补偿需重新获批</b><p>请以事件与证据确认完成范围；不要把补偿或回滚伪装为普通恢复。</p></div></section>
  if (kind === 'approval_expired' || kind === 'confirmation_expired') return <section className="controlled-execution-recovery danger"><FileCheck2 size={18} /><div><b>重新生成授权链</b><p>从差异与正式 Approval 重新开始；旧审批或确认不能用于当前 Run。</p></div></section>
  return null
}

function EvidencePanel({ evidence, events }: { evidence: BrowserRpaEvidence[]; events: BrowserRpaRunEvent[] }) {
  return <section className="controlled-execution-evidence" aria-label="证据与事件">
    <header><div><span className="section-label">Evidence & Audit</span><h3>运行证据和历史记录</h3></div><small>服务端已脱敏字段值。页面不显示凭据和一次性确认令牌。</small></header>
    <div className="controlled-execution-evidence-grid">
      <div><h4>运行证据</h4>{evidence.length ? evidence.map(item => <article key={item.id}><b>{item.step_id}</b><span>{item.diff_keys.length ? item.diff_keys.join('、') : '无字段差异键'}</span><span title={item.page_reference}>{item.page_reference}</span>{item.screenshot_reference ? <small title={item.screenshot_reference}>截图证据：{item.screenshot_reference}</small> : null}<small>redaction={item.redaction_version} · selector={item.selector_version}</small></article>) : <p>服务端尚未返回 Evidence。</p>}</div>
      <div><h4>事件时间线</h4>{events.length ? events.map(item => <article key={item.id}><b>{item.kind}</b><span>{item.summary}</span><small>{formatTime(item.created_at)} · {item.actor}</small></article>) : <p>服务端尚未返回 Run Event。</p>}</div>
    </div>
  </section>
}

function WorkspaceState({ kind, message, onRetry }: { kind: 'loading' | 'empty' | 'forbidden' | 'error'; message?: string; onRetry?: () => void }) {
  if (kind === 'loading') return <section className="controlled-execution-state loading" role="status" aria-busy="true"><span /><span /><span /><span /></section>
  const forbidden = kind === 'forbidden'
  return <section className={`controlled-execution-state ${kind}`} role={forbidden || kind === 'error' ? 'alert' : 'status'}>
    {forbidden || kind === 'error' ? <CircleAlert size={28} /> : <FileCheck2 size={28} />}
    <h2>{forbidden ? '无权查看此受控执行' : kind === 'error' ? '无法读取受控执行' : '暂无受控执行 Run'}</h2>
    <p>{message ?? '路由需要提供服务端创建的 Run ID；此页不会用本地示例填充执行记录。'}</p>
    {onRetry ? <button className="secondary-button" onClick={onRetry}><RefreshCw size={15} />重新加载</button> : null}
  </section>
}

function controlNotice(action: 'pause' | 'resume' | 'cancel' | 'takeover' | 'release_takeover') {
  return ({ pause: '已请求暂停执行流程；这不会改变平台推广单元状态。', resume: '已请求恢复执行流程；服务端将先重新识别页面与账户。', cancel: '已请求取消；证据与审计记录保持可读。', takeover: '已请求人工接管；请等待服务端确认租约状态。', release_takeover: '已请求释放人工接管。' })[action]
}

function sessionProbeReason(reason: EdgeSessionProbe['reason']) {
  return ({
    session_ready: '会话可用',
    cdp_unavailable: 'DevTools WebSocket 不可用',
    oceanengine_page_missing: '未找到巨量引擎页面',
    login_required: '巨量引擎需要登录',
    account_mismatch: '页面广告账户不匹配',
  } as const)[reason]
}

function formatMinor(value: number) {
  return (value / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN')
}
