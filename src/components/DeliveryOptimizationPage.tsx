import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Activity, ArrowRight, CheckCircle2, CircleAlert, Clock3, RefreshCw, Send, XCircle } from 'lucide-react'
import {
  DeliveryApiError,
  deliveryConfigurationApi,
  deliveryPlanApi,
  type DeliveryControlChangeSet,
  type DeliveryPlan,
  type DeliveryRecommendation,
} from '../api/delivery'
import { useProject } from '../context/ProjectContext'
import { projectPath } from '../lib/router'
import type { DataState } from '../types'
import { StateBoundary } from './StateBoundary'

type RecommendationStatus = 'proposed' | 'accepted' | 'rejected'

const viewStatus: Record<string, RecommendationStatus | 'observing' | 'tracking'> = {
  待处理建议: 'proposed',
  已采纳: 'accepted',
  已拒绝: 'rejected',
  观察中: 'observing',
  效果跟踪: 'tracking',
}

const recommendationStatusLabel: Record<RecommendationStatus, string> = {
  proposed: '待决策',
  accepted: '已采纳',
  rejected: '已拒绝',
}

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) : '尚无记录'
}

function formatCny(value: number) {
  return (value / 100).toLocaleString('zh-CN', { style: 'currency', currency: 'CNY' })
}

function recommendationLabel(value: string) {
  const budgetMatch = /^reduce_budget_(\d+)_percent$/.exec(value)
  if (budgetMatch) return `计划预算下调 ${budgetMatch[1]}%`
  return ({
    reduce_mock_budget: '降低计划预算',
    'reduces only the mock budget by 10%': '仅将计划预算下调 10%，不扩大花费。',
    mock_budget_reduction_only: '仅限预算下调',
    'observe mock conversion cost for 24 hours after manual application': '人工应用后观察 24 小时转化成本。',
  } as Record<string, string>)[value] ?? value
}

function evidenceLabel(reference: string) {
  if (reference.startsWith('simulation://run/')) return '投放效果情景模拟'
  if (reference.startsWith('simulation://execution/')) return '平台操作演练'
  if (reference.startsWith('simulation://metric/')) return '持久指标窗口'
  if (reference.startsWith('simulation://alert/')) return '监控告警'
  return '服务端证据'
}

function normalizeStatus(value: string): RecommendationStatus {
  return value === 'accepted' ? 'accepted' : value === 'rejected' ? 'rejected' : 'proposed'
}

function addQuery(path: string, values: Record<string, string | undefined>) {
  const url = new URL(path, window.location.origin)
  for (const [key, value] of Object.entries(values)) {
    if (value) url.searchParams.set(key, value)
  }
  return `${url.pathname}${url.search}`
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof DeliveryApiError) {
    if (error.code === 'VERSION_CONFLICT') return '版本已经更新，请刷新后重新操作。'
    if (error.status === 401 || error.status === 403) return '当前身份无权执行此受控操作。'
    return error.message
  }
  return error instanceof Error ? error.message : fallback
}

function RecommendationCard({
  item,
  plan,
  changeSet,
  busy,
  monitoringURL,
  draftURL,
  onAccept,
  onReject,
}: {
  item: DeliveryRecommendation
  plan?: DeliveryPlan
  changeSet?: DeliveryControlChangeSet
  busy: boolean
  monitoringURL: string
  draftURL?: string
  onAccept: (item: DeliveryRecommendation) => void
  onReject: (item: DeliveryRecommendation) => void
}) {
  const status = normalizeStatus(item.status)
  const optimized = Boolean(plan && plan.currentVersionNumber > item.planVersion)
  const stale = status === 'proposed' && Boolean(plan && plan.currentVersionNumber !== item.planVersion)
  return <article className="delivery-recommendation-card delivery-optimization-card">
    <header>
      <div><span>{plan?.currentVersion.name ?? item.planId} · 基于 V{item.planVersion}</span><h3>{recommendationLabel(item.action)}</h3></div>
      <strong className={`delivery-recommendation-status ${stale ? 'stale' : status}`}>{stale ? '计划已变化' : recommendationStatusLabel[status]}</strong>
    </header>
    <dl className="delivery-recommendation-summary">
      <div className="wide"><dt>建议产生的变化</dt><dd>{recommendationLabel(item.impact)}</dd></div>
      <div><dt>当前计划</dt><dd>{plan ? `V${plan.currentVersionNumber} · ${formatCny(plan.currentVersion.budget.totalMinor)}` : `计划 ${item.planId}`}</dd></div>
      <div><dt>变更申请</dt><dd>{changeSet ? `${changeSet.status} · ${changeSet.id}` : status === 'accepted' ? '正在恢复关联申请' : '尚未创建'}</dd></div>
      <div><dt>观察窗口</dt><dd>{recommendationLabel(item.observation)}</dd></div>
      <div><dt>再次决策时间</dt><dd>{item.cooldown ? formatTime(item.cooldown) : '暂无冷却期'}</dd></div>
      <div className="wide"><dt>风险与约束</dt><dd>{item.risks.length ? <ul>{item.risks.map(risk => <li key={risk}>{recommendationLabel(risk)}</li>)}</ul> : '未记录额外风险'}</dd></div>
    </dl>
    <section className="delivery-optimization-evidence" aria-label="建议因果证据">
      <header><div><span>因果证据链</span><b>{item.evidenceRefs.length} 条服务端引用</b></div><a href={monitoringURL}>查看指标与告警<ArrowRight size={13}/></a></header>
      <div>{item.evidenceRefs.map(reference => <span key={reference}><CheckCircle2 size={14}/><b>{evidenceLabel(reference)}</b><code>{reference}</code></span>)}</div>
    </section>
    {stale ? <div className="delivery-optimization-stale" role="status"><CircleAlert size={17}/><span><b>这条建议基于 V{item.planVersion}，当前计划已经是 V{plan?.currentVersionNumber}</b><small>旧建议仅保留审计记录，不能再采纳。请基于当前版本重新运行情景模拟、告警和建议生成。</small></span></div> : null}
    {status === 'accepted' ? <div className={`delivery-optimization-handoff ${optimized ? 'is-observing' : ''}`}>
      <Activity size={18}/><span><b>{optimized ? `优化计划 V${plan?.currentVersionNumber} 已生成` : '优化草稿等待检查与审批'}</b><small>{optimized ? '下一次平台操作演练和 SimulationRun 将用于效果跟踪，不复用优化前指标。' : '采纳只创建草稿；请检查配置并提交审批，不会自动修改平台。'}</small></span>
      {draftURL ? <a className="primary-button" href={draftURL}>{optimized ? '查看优化配置' : '检查优化草稿'}<ArrowRight size={14}/></a> : null}
    </div> : null}
    <footer>
      <span>模型：{item.source} / {item.scenario} · 生成于 {formatTime(item.createdAt)}</span>
      {status === 'proposed' ? <div>
        <button className="secondary-button" onClick={() => onReject(item)} disabled={busy}><XCircle size={14}/>拒绝建议</button>
        <button className="primary-button" onClick={() => onAccept(item)} disabled={busy || stale}><CheckCircle2 size={14}/>{stale ? '建议已过期' : '采纳并生成草稿'}</button>
      </div> : null}
    </footer>
  </article>
}

export function DeliveryOptimizationPage({ state, activeView, tourRunId, tourCase }: { state: DataState; activeView: string; tourRunId?: string; tourCase?: string }) {
  const { currentProject } = useProject()
  const projectId = currentProject.id
  const [plans, setPlans] = useState<DeliveryPlan[]>([])
  const [recommendations, setRecommendations] = useState<DeliveryRecommendation[]>([])
  const [changeSets, setChangeSets] = useState<DeliveryControlChangeSet[]>([])
  const [selectedPlanId, setSelectedPlanId] = useState(() => new URLSearchParams(window.location.search).get('plan_id') ?? '')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const refreshGeneration = useRef(0)

  const refresh = useCallback(async () => {
    const generation = ++refreshGeneration.current
    setBusy(true)
    try {
      const [nextPlans, nextRecommendations, nextChangeSets] = await Promise.all([
        deliveryPlanApi.list(projectId),
        deliveryConfigurationApi.listRecommendations(projectId),
        deliveryPlanApi.listChangeSets(projectId),
      ])
      if (generation !== refreshGeneration.current) return
      setPlans(nextPlans)
      setRecommendations(nextRecommendations)
      setChangeSets(nextChangeSets)
      setSelectedPlanId(current => nextPlans.some(plan => plan.id === current) ? current : nextPlans[0]?.id ?? '')
    } catch (error) {
      if (generation === refreshGeneration.current) setNotice(errorMessage(error, '读取优化建议失败。'))
    } finally {
      if (generation === refreshGeneration.current) setBusy(false)
    }
  }, [projectId])

  useEffect(() => { void refresh() }, [refresh])
  useEffect(() => {
    if (!selectedPlanId) return
    const url = new URL(window.location.href)
    url.searchParams.set('plan_id', selectedPlanId)
    window.history.replaceState(window.history.state, '', url)
  }, [selectedPlanId])

  const selectedPlan = useMemo(() => plans.find(plan => plan.id === selectedPlanId), [plans, selectedPlanId])
  const selectedRecommendations = useMemo(
    () => recommendations.filter(item => item.planId === selectedPlanId),
    [recommendations, selectedPlanId],
  )
  const changeSetByRecommendation = useMemo(() => {
    const result = new Map<string, DeliveryControlChangeSet>()
    for (const item of changeSets) {
      if (!item.recommendationId) continue
      const current = result.get(item.recommendationId)
      if (!current || item.updatedAt > current.updatedAt) result.set(item.recommendationId, item)
    }
    return result
  }, [changeSets])
  const counts = useMemo(() => selectedRecommendations.reduce((result, item) => {
    result[normalizeStatus(item.status)] += 1
    return result
  }, { proposed: 0, accepted: 0, rejected: 0 }), [selectedRecommendations])
  const filteredRecommendations = useMemo(() => {
    const filter = viewStatus[activeView] ?? 'proposed'
    return selectedRecommendations.filter(item => {
      const status = normalizeStatus(item.status)
      if (filter === 'tracking') return status === 'accepted'
      if (filter === 'observing') {
        const plan = plans.find(candidate => candidate.id === item.planId)
        return status === 'accepted' && Boolean(plan && plan.currentVersionNumber > item.planVersion)
      }
      return status === filter
    })
  }, [activeView, plans, selectedRecommendations])

  const generateRecommendation = async () => {
    if (!selectedPlan) return
    setBusy(true)
    setNotice('')
    try {
      const generated = await deliveryConfigurationApi.generateRecommendations(projectId, selectedPlan.id, selectedPlan.currentVersionNumber)
      setRecommendations(current => [generated, ...current.filter(item => item.id !== generated.id)])
      setNotice('已依据同一 SimulationRun 的指标与告警生成建议；当前仍等待人工决策。')
    } catch (error) {
      setNotice(errorMessage(error, '生成建议失败。'))
    } finally { setBusy(false) }
  }

  const acceptRecommendation = async (item: DeliveryRecommendation) => {
    setBusy(true)
    setNotice('')
    try {
      const accepted = await deliveryConfigurationApi.acceptRecommendation(projectId, item.id, item.version, `optimization-${item.id}-${item.version}`)
      setRecommendations(current => current.map(value => value.id === item.id ? accepted.recommendation : value))
      setChangeSets(current => [accepted.changeSet, ...current.filter(value => value.id !== accepted.changeSet.id)])
      setNotice('建议已采纳并生成一个优化草稿；请前往内部配置编排检查并提交审批。')
    } catch (error) {
      setNotice(errorMessage(error, '采纳建议失败。'))
    } finally { setBusy(false) }
  }

  const rejectRecommendation = async (item: DeliveryRecommendation) => {
    setBusy(true)
    setNotice('')
    try {
      const rejected = await deliveryConfigurationApi.rejectRecommendation(projectId, item.id, item.version)
      setRecommendations(current => current.map(value => value.id === item.id ? rejected : value))
      setNotice('建议已拒绝，没有创建优化草稿或变更申请。')
    } catch (error) {
      setNotice(errorMessage(error, '拒绝建议失败。'))
    } finally { setBusy(false) }
  }

  const monitoringBaseURL = projectPath(projectId, 'delivery', 'monitoring', undefined, undefined, undefined, tourRunId, tourCase)

  return <StateBoundary state={state} contextLabel="智能投放 / 优化中心" errorDetail="当前 Project 的优化建议无法读取，请确认 Delivery 服务可用后刷新。">
    <div className="delivery-optimization-workspace">
      <section className="delivery-optimization-toolbar">
        <label>投放计划<select value={selectedPlanId} onChange={event => setSelectedPlanId(event.target.value)}>{plans.map(plan => <option key={plan.id} value={plan.id}>{plan.currentVersion.name} · V{plan.currentVersionNumber}</option>)}</select></label>
        <div className="delivery-optimization-counts"><span><b>{counts.proposed}</b>待决策</span><span><b>{counts.accepted}</b>已采纳</span><span><b>{counts.rejected}</b>已拒绝</span></div>
        <div className="delivery-optimization-toolbar-actions">
          <button className="secondary-button" onClick={() => void refresh()} disabled={busy}><RefreshCw size={14}/>刷新</button>
          <button className="primary-button" onClick={() => void generateRecommendation()} disabled={busy || !selectedPlan?.currentVersion.platformConfiguration || selectedPlan.currentVersion.runtimeStatus !== 'active'}><Send size={14}/>生成优化建议</button>
        </div>
      </section>

      {!selectedPlan ? <div className="panel-empty">当前 Project 尚无投放计划。</div> : <>
        <section className="delivery-optimization-context">
          <div><span>当前决策基线</span><b>{selectedPlan.currentVersion.name} · V{selectedPlan.currentVersionNumber}</b><small>预算 {formatCny(selectedPlan.currentVersion.budget.totalMinor)} · 策略 {selectedPlan.currentVersion.sourceStrategyVersion}</small></div>
          <div><Clock3 size={17}/><span><b>{activeView}</b><small>{activeView === '效果跟踪' ? '优化后需要新的平台操作演练与 SimulationRun，不能沿用优化前指标。' : '建议必须拥有执行、SimulationRun、指标和告警的完整证据链。'}</small></span></div>
        </section>
        <div className="delivery-config-recommendations delivery-optimization-list">
          {filteredRecommendations.map(item => {
            const plan = plans.find(candidate => candidate.id === item.planId)
            const changeSet = changeSetByRecommendation.get(item.id)
            const monitoringURL = addQuery(monitoringBaseURL, { plan_id: item.planId })
            const configBaseURL = projectPath(projectId, 'delivery', 'configuration', undefined, '检查与提交', undefined, tourRunId, tourCase)
            const draftURL = changeSet ? addQuery(configBaseURL, { plan_id: item.planId, change_set_id: changeSet.id }) : undefined
            return <RecommendationCard key={item.id} item={item} plan={plan} changeSet={changeSet} busy={busy} monitoringURL={monitoringURL} draftURL={draftURL} onAccept={acceptRecommendation} onReject={rejectRecommendation}/>
          })}
          {!filteredRecommendations.length ? <div className="panel-empty"><CircleAlert size={18}/>{activeView === '待处理建议' ? '当前计划没有待决策建议。先完成平台操作演练、投放效果情景模拟和告警评估，再生成建议。' : `当前计划在“${activeView}”中没有记录。`}</div> : null}
        </div>
        {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
      </>}
    </div>
  </StateBoundary>
}
