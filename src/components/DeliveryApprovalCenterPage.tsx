import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CircleAlert, CircleCheck, FileCheck2, RotateCcw, ShieldCheck, ThumbsDown, ThumbsUp } from 'lucide-react'
import {
  deliveryPlanApi,
  type DeliveryApproval,
  type DeliveryControlChangeSet,
} from '../api/delivery'
import { useProject } from '../context/ProjectContext'
import { projectPath } from '../lib/router'
import type { DataState } from '../types'
import { StateBoundary } from './StateBoundary'
import { DeliveryExecutionPanel } from './DeliveryExecutionPanel'

const invalidReasonLabels: Record<NonNullable<DeliveryApproval['invalidReason']>, string> = {
  APPROVAL_EXPIRED: '审批已超过 24 小时有效期，需要重新预检并审批。',
  APPROVAL_CONTENT_MISMATCH: '变更申请或批准内容与审批快照不一致。',
  APPROVAL_SCOPE_EXCEEDED: '执行范围或预算超过批准快照。',
  STALE_PLAN_VERSION: '计划已产生新版本，旧版本审批永久失效。',
}

const preflightPassedStatuses = new Set<DeliveryControlChangeSet['status']>([
  'preflight_passed',
  'approved',
  'executed',
  'rolled_back',
])

export function DeliveryApprovalCenterPage({ state, tourCase, tourRunId, selectedChangeSetId }: { state: DataState; tourCase?: string; tourRunId?: string; selectedChangeSetId?: string }) {
  const { currentProject } = useProject()
  const projectId = currentProject.id
  const [changeSets, setChangeSets] = useState<DeliveryControlChangeSet[]>([])
  const [selectedId, setSelectedId] = useState(selectedChangeSetId ?? '')
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [rejectionReason, setRejectionReason] = useState('')
  const listRequest = useRef(0)
  const activeProjectId = useRef(projectId)
  activeProjectId.current = projectId

  const selected = useMemo(
    () => changeSets.find(changeSet => changeSet.id === selectedId),
    [changeSets, selectedId],
  )

  const refresh = useCallback(async () => {
    if (!projectId) return
    const request = ++listRequest.current
    const requestedProjectId = projectId
    setBusy(true)
    try {
      const records = await deliveryPlanApi.listChangeSets(projectId)
      if (request !== listRequest.current || activeProjectId.current !== requestedProjectId) return
      setChangeSets(records)
      setSelectedId(current => selectedChangeSetId && records.some(record => record.id === selectedChangeSetId)
        ? selectedChangeSetId
        : records.some(record => record.id === current) ? current : records[0]?.id ?? '')
      setNotice(records.length ? `已加载 ${records.length} 个变更申请。` : '当前 Project 暂无变更申请。')
    } catch (error) {
      if (request !== listRequest.current || activeProjectId.current !== requestedProjectId) return
      setNotice(error instanceof Error ? error.message : '加载审批队列失败')
    } finally {
      if (request === listRequest.current && activeProjectId.current === requestedProjectId) setBusy(false)
    }
  }, [projectId, selectedChangeSetId])

  useEffect(() => {
    void refresh()
    return () => { listRequest.current += 1 }
  }, [refresh])

  useEffect(() => {
    setRejectionReason('')
  }, [selectedId])

  const apply = async (action: 'approve' | 'reject') => {
    if (!selected) return
	if (action === 'reject' && rejectionReason.trim().length < 3) {
	  setNotice('打回时请填写至少 3 个字符的修改原因。')
	  return
	}
    listRequest.current += 1
    setBusy(true)
    try {
      const updated = action === 'approve'
        ? await deliveryPlanApi.approveChangeSet(projectId, selected.id, selected.version)
        : await deliveryPlanApi.rejectChangeSet(projectId, selected.id, selected.version, rejectionReason.trim())
      setChangeSets(current => current.map(item => item.id === updated.id ? updated : item))
	  if (action === 'reject') setRejectionReason('')
      setNotice(action === 'approve' ? `已批准${updated.recommendationId ? '优化写入' : '平台操作演练'}；授权将在 24 小时后过期。` : '已打回变更申请，并保留修改原因。')
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '审批失败')
    } finally {
      setBusy(false)
    }
  }

  const approval = selected?.approval
  const approvalValid = selected?.status === 'approved' && approval?.valid === true
  const preflightPassed = selected ? preflightPassedStatuses.has(selected.status) : false
  const optimizationApproval = Boolean(selected?.recommendationId)
  const manualPackageURL = selected ? projectPath(projectId, 'delivery', 'configuration', undefined, '人工操作包', undefined, tourRunId, tourCase) : undefined

  return <StateBoundary
    state={state}
    contextLabel="智能投放 / 审批中心"
    emptyTitle="当前 Project 暂无待审批变更申请"
    emptyDetail="在投放计划页检查草稿并提交变更申请后，审批快照会出现在这里。"
    errorDetail="审批队列暂时无法读取。请确认 Delivery 服务可用后刷新。"
    retryLabel="刷新审批队列"
  >
    <div className="approval-workspace">
      <aside className="approval-queue">
        <div className="surface-toolbar">
          <h3>审批队列</h3>
          <button onClick={() => void refresh()} disabled={busy} aria-label="刷新审批队列"><RotateCcw size={15}/></button>
        </div>
        {changeSets.map(item => <button
          key={item.id}
          className={selectedId === item.id ? 'active' : ''}
          aria-label={`${item.id} ${item.planName} Plan V${item.planVersion} ${item.status}`}
          onClick={() => setSelectedId(item.id)}
        >
          <span title={item.id}>{item.id.slice(-12)}</span>
          <b>{item.planName}</b>
          <small>Plan V{item.planVersion} · {approvalStatusLabel(item.status)} · ¥{formatMinor(item.budgetLimit.totalMinor)}</small>
        </button>)}
      </aside>

      <section className="approval-detail">
        {selected ? <>
          <div className="approval-heading">
            <div>
              <span title={selected.id}>{selected.id.slice(-12)} · 变更申请 v{selected.version}</span>
              <h2>{selected.planName}</h2>
              <p>Plan V{selected.planVersion} · 批准内容由计划快照、变更版本、执行范围和预算边界共同绑定。</p>
            </div>
            <span className={`approval-status ${selected.status}`}>{approvalStatusLabel(selected.status)}</span>
          </div>

          {approval && !approval.valid ? <div className="inline-notice danger approval-invalid-notice" role="alert">
            <CircleAlert size={16}/>
            <span><b>旧审批已失效</b><small>{approval.invalidReason ? invalidReasonLabels[approval.invalidReason] : '审批快照不再满足执行门禁。'}</small></span>
          </div> : null}

          <div className="execution-confirmation">
            <h3>批准内容快照</h3>
            <div><b>PlanVersion</b><span>V{selected.planVersion}</span></div>
            <div><b>申请版本</b><span>v{approval?.changeSetVersion ?? selected.version}</span></div>
            <div><b>内容 Hash</b><span title={approval?.planCanonicalHash ?? selected.planCanonicalHash}>{approval?.hashSummary ?? selected.planCanonicalHash.slice(0, 12)}</span></div>
            <div><b>平台配置</b><span title={approval?.configuration?.canonicalHash}>{approval?.configuration ? `${approval.configuration.platform} · ${approval.configuration.id} V${approval.configuration.version}` : '旧版快照绑定'}</span></div>
            <div><b>业务意图</b><span title={approval?.intent?.canonicalHash}>{approval?.intent ? `${approval.intent.id} V${approval.intent.version}` : '旧版无独立 Intent'}</span></div>
            <div><b>Action Hash</b><span title={approval?.actionHash}>{approval?.actionHash.slice(0, 12) ?? '批准后生成'}</span></div>
            <div><b>授权用途</b><span>{optimizationApproval ? '优化配置写入' : approval ? '平台操作演练' : '首次上线操作演练'}</span></div>
            <div><b>预算上限</b><span>¥{formatMinor(approval?.budgetLimit.totalMinor ?? selected.budgetLimit.totalMinor)} {approval?.budgetLimit.currency ?? selected.budgetLimit.currency}</span></div>
            <div><b>批准时间</b><span>{approval ? formatTime(approval.approvedAt) : '—'}</span></div>
            <div><b>有效期</b><span>{approval ? `至 ${formatTime(approval.expiresAt)}` : '批准后 24 小时'}</span></div>
          </div>

          {selected.status === 'rejected' ? <div className="inline-notice danger" role="status"><CircleAlert size={16}/><span><b>已打回</b><small>{selected.rejectionReason}</small></span></div> : null}

          <div className="approval-evidence">
            <h3>执行门禁</h3>
            <GateRow passed={preflightPassed} label="服务端预检" detail={preflightPassed ? '复用 RunPreflight' : '尚未通过'} />
            <GateRow passed={Boolean(approval)} label="不可变审批记录" detail={approval?.approvalId ?? '等待审批'} />
            <GateRow passed={approval?.valid === true} label="审批有效性" detail={approval?.valid ? '内容、有效期、范围和预算匹配' : approval?.invalidReason ?? '尚未批准'} />
          </div>

          {selected.status === 'preflight_passed' ? <section className="approval-decision-panel" aria-labelledby="approval-decision-title">
            <header className="approval-decision-heading">
              <div><span className="section-label">审批决定</span><h3 id="approval-decision-title">批准当前快照，或说明原因后打回</h3></div>
              <small>{optimizationApproval ? '本次授权仅用于生成优化操作包。' : '本次授权仅用于启动平台操作演练。'}</small>
            </header>
            <label className="approval-rejection-reason" htmlFor="approval-rejection-reason">
              <span>打回修改说明 <em>打回时必填</em></span>
              <textarea id="approval-rejection-reason" value={rejectionReason} onChange={event => setRejectionReason(event.target.value)} placeholder="例如：预算上限与本轮目标不一致，请调整为……" disabled={busy} maxLength={500} aria-describedby="approval-rejection-help"/>
              <small id="approval-rejection-help"><span>请具体说明需要修改的对象、字段和期望结果，至少 3 个字符。</span><b>{rejectionReason.length} / 500</b></small>
            </label>
            <div className="approval-decision-actions">
              <button className="secondary-button approval-reject-button" onClick={() => void apply('reject')} disabled={busy || rejectionReason.trim().length < 3}><ThumbsDown size={15}/>打回修改</button>
              <button className="primary-button" onClick={() => void apply('approve')} disabled={busy}><ThumbsUp size={15}/>{optimizationApproval ? '批准优化写入' : '批准平台操作演练'}</button>
            </div>
          </section> : selected.status === 'approved' ? <div className="approval-decision-result" role="status"><CircleCheck size={18}/><span><b>{optimizationApproval ? '优化写入已批准' : '平台操作演练已批准'}</b><small>审批决定和不可变内容快照已保存。</small></span></div> : null}
          {optimizationApproval ? <div className="approval-optimization-handoff"><span><b>优化申请不在这里执行</b><small>审批通过后返回配置编排，生成人工操作包并由授权人员操作平台。</small></span>{approvalValid && manualPackageURL ? <a className="primary-button" href={manualPackageURL}>返回生成操作包</a> : null}</div> : <DeliveryExecutionPanel
            projectId={projectId}
            changeSet={selected}
            canExecute={approvalValid}
            goldenPath={tourCase === 'golden_path'}
            onExecutionCreated={updated => {
              setChangeSets(current => current.map(item => item.id === updated.id ? updated : item))
            }}
          />}
        </> : <div className="panel-empty"><FileCheck2 size={24}/>没有可显示的变更申请。</div>}
        {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
      </section>

      <aside className="approval-audit">
        <div className="approval-audit-heading">
          <ShieldCheck size={18}/>
          <span className="section-label">权限与审计</span>
        </div>
        <div><time>身份来源</time><span>ActorContext</span></div>
        <div><time>授权动作</time><span>{optimizationApproval ? '优化配置写入' : '平台操作演练'}</span></div>
        <div><time>授权范围</time><span>{optimizationApproval ? '仅限当前优化内容快照' : '仅限当前上线内容快照'}</span></div>
        <div><time>有效期</time><span>固定 24 小时</span></div>
        <div><time>审计策略</time><span>旧 Approval 保留，不覆盖</span></div>
      </aside>
    </div>
  </StateBoundary>
}

function GateRow({ passed, label, detail }: { passed: boolean; label: string; detail: string }) {
  return <div className={`gate-row ${passed ? 'passed' : 'failed'}`}>
    {passed ? <CircleCheck size={16}/> : <CircleAlert size={16}/>}
    <span><b>{label}</b><small>{detail}</small></span>
  </div>
}

function formatMinor(value: number) {
  return (value / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN')
}

function approvalStatusLabel(status: DeliveryControlChangeSet['status']) {
  const labels: Record<DeliveryControlChangeSet['status'], string> = {
    draft: '草稿',
    preflight_passed: '待审批',
    preflight_failed: '检查未通过',
    approved: '已批准',
    rejected: '已打回',
    executed: '已执行',
    rolled_back: '已回滚',
  }
  return labels[status]
}
