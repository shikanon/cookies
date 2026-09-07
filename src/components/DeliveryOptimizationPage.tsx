import { useCallback, useEffect, useRef, useState } from 'react'
import { CheckCircle2, CircleAlert, RefreshCw, Send, ShieldCheck } from 'lucide-react'
import {
  DeliveryApiError,
  deliveryMechanisticSimulationApi,
  deliveryOptimizationApi,
  deliveryPlanApi,
  type DeliveryDecision,
  type DeliveryDecisionCandidate,
  type DeliveryDecisionSelection,
  type DeliveryObservatoryRun,
  type DeliveryPlan,
  type MechanisticRecommendationDraft,
  type MechanisticSimulation,
} from '../api/delivery'
import { useProject } from '../context/ProjectContext'
import type { DataState } from '../types'
import { StateBoundary } from './StateBoundary'

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) : '尚无记录'
}

function formatCny(value: number) {
  return (value / 100).toLocaleString('zh-CN', { style: 'currency', currency: 'CNY' })
}

const decisionCandidateLabel: Record<DeliveryDecisionCandidate['kind'], string> = {
  conservative: '稳健方案',
  balanced: '均衡方案',
  exploratory: '探索方案',
}

const decisionUncertaintyLabel: Record<DeliveryDecisionCandidate['uncertainty'], string> = {
  low: '调整强度较低',
  medium: '调整强度中等',
  high: '调整强度较高',
}

const decisionDiagnosticCopy: Record<DeliveryDecision['diagnostic']['code'], { label: string; explanation: string; nextAction: string }> = {
  ready: { label: '可决策', explanation: '计划、模拟结果和指标证据均已完整绑定。', nextAction: '请选择一个候选方案并编译本地工作流。' },
  insufficient_data: { label: '数据不足', explanation: '当前证据不足以生成可靠的候选方案。', nextAction: '请先为当前 PlanVersion 运行上线前概率模拟。' },
  stale_data: { label: '数据已过期', explanation: '用于决策的平台事实或指标窗口已经过期。', nextAction: '请刷新平台事实并重新采集最新指标。' },
  blocked_by_asset: { label: '素材受阻', explanation: '候选方案依赖的素材尚不可用。', nextAction: '请解决素材状态或更换素材后重新生成。' },
  platform_pending: { label: '等待平台能力', explanation: '当前平台配置尚未具备生成候选方案的条件。', nextAction: '请完善平台配置或等待相应能力就绪。' },
}

function decisionPolicyLabel(value: DeliveryDecision['policyVersion']) {
  return `决策规则 ${value.split('/').at(-1)?.toUpperCase() ?? 'V1'}`
}

function decisionRationaleLabel(value: string) {
  const cpaMatch = /^current CPA is ([0-9.]+)x the baseline CPA$/.exec(value)
  if (cpaMatch) return `当前转化成本为基准值的 ${cpaMatch[1]} 倍`
  const scenarioMatch = /^scenario (\S+) has probability ([0-9.]+)$/.exec(value)
  if (scenarioMatch) return `方案针对“${scenarioCopyLabel(scenarioMatch[1])}”情景，概率为 ${(Number(scenarioMatch[2]) * 100).toFixed(1)}%`
  const actionMatch = /^action (\S+) uses magnitude (\d+)%$/.exec(value)
  if (actionMatch) return `${candidateActionLabel(actionMatch[1])}，动作范围 ${actionMatch[2]}%`
  const reductionMatch = /^policy \S+ applies a (\d+)% budget reduction$/.exec(value)
  if (reductionMatch) return `根据当前决策规则，建议将每日预算下调 ${reductionMatch[1]}%`
  return recommendationLabel(value)
}

function businessObjectiveLabel(value?: string) {
  if (!value) return '请选择投放计划'
  return ({ 'qualified conversions': '获取高质量转化' } as Record<string, string>)[value] ?? value
}

function biddingStrategyLabel(value?: string) {
  return ({ manual_bid: '手动出价' } as Record<string, string>)[value ?? ''] ?? value ?? '未设置'
}

function chargingModeLabel(value?: string) {
  return ({ CPC: '按点击计费（CPC）', CPM: '按千次展示计费（CPM）', OCPM: '按优化目标计费（OCPM）' } as Record<string, string>)[value ?? ''] ?? value ?? '未设置'
}

function proposalComparison(plan: DeliveryPlan | undefined, selection: DeliveryDecisionSelection | undefined, candidate?: DeliveryDecisionCandidate) {
  const before = plan?.currentVersion.platformConfiguration?.payload.ocean_engine
  const after = selection?.configuration.payload.ocean_engine
  if (!before?.project || !after?.project) return []
  const beforeMaterials = before.promotions.reduce((total, promotion) => total + promotion.base_material_references.length, 0)
  const afterMaterials = after.promotions.reduce((total, promotion) => total + promotion.base_material_references.length, 0)
  return [
    ...(candidate?.proposedAction ? [{ key: 'optimization_action', label: '优化动作', before: '当前方案未执行该动作', after: `${candidateActionLabel(candidate.proposedAction)}（${candidate.actionMagnitudePercent ?? 0}%）`, changed: true }] : []),
    { key: 'daily_budget', label: '每日预算', before: formatCny(before.project.budget_and_bidding.daily_budget_minor), after: formatCny(after.project.budget_and_bidding.daily_budget_minor), changed: before.project.budget_and_bidding.daily_budget_minor !== after.project.budget_and_bidding.daily_budget_minor },
    { key: 'bidding_strategy', label: '出价方式', before: biddingStrategyLabel(before.project.budget_and_bidding.bidding_strategy), after: biddingStrategyLabel(after.project.budget_and_bidding.bidding_strategy), changed: before.project.budget_and_bidding.bidding_strategy !== after.project.budget_and_bidding.bidding_strategy },
    { key: 'charging_mode', label: '计费方式', before: chargingModeLabel(before.project.budget_and_bidding.charging_mode), after: chargingModeLabel(after.project.budget_and_bidding.charging_mode), changed: before.project.budget_and_bidding.charging_mode !== after.project.budget_and_bidding.charging_mode },
    { key: 'promotions', label: '推广单元', before: `${before.promotions.length} 个`, after: `${after.promotions.length} 个`, changed: before.promotions.length !== after.promotions.length },
    { key: 'materials', label: '关联素材', before: `${beforeMaterials} 个`, after: `${afterMaterials} 个`, changed: beforeMaterials !== afterMaterials },
  ]
}

const proposalDecisionLabel = {
  accepted: '已接受优化方案',
  modified: '已保存修改后的方案',
  rejected: '已拒绝优化方案',
} as const

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

function scenarioCopyLabel(value: string) {
  return ({ typical_launch: '普通跑量情景', breakout_launch: '跑量号情景', steady: '稳定投放', under_delivery: '跑量不足', cost_pressure: '成本压力', creative_fatigue: '素材疲劳', tracking_anomaly: '追踪异常', review_rejected: '审核拒绝', zero_conversion: '零转化', spend_spike: '消耗突增' } as Record<string, string>)[value] ?? value
}

function candidateFocusLabel(value?: string) {
  return ({ parallel_launch_observation: '并行项目跑量观察', parallel_material_portfolio: '并行素材组合', material_rotation: '并行素材组合（旧结果）', tracking_integrity: '追踪完整性', delivery_constraints: '跑量约束', review_compliance: '审核合规', conversion_funnel: '转化漏斗', budget_pacing: '预算节奏', cost_control: '成本控制', maintain_and_observe: '保持并观察' } as Record<string, string>)[value ?? ''] ?? value ?? '预算控制'
}

function candidateActionLabel(value?: string) {
  return ({
    observe_existing_portfolio: '保护现有跑量对象并等待成熟窗口', launch_controlled_parallel_test: '新建并行项目或单元进行受控跑量', expand_parallel_test_and_prune_mature_losers: '扩大并行跑量，并在窗口成熟后淘汰低效对象',
    observe_first_seven_days: '观察首个七日窗口', prepare_parallel_project_test: '准备并行项目测试', prepare_parallel_project_and_promotion_test: '准备并行项目与单元测试',
    prepare_material_rotation: '准备并行素材测试（旧结果）', controlled_material_rotation_test: '执行并行素材测试（旧结果）', expanded_material_rotation_test: '扩大并行素材测试（旧结果）',
    verify_primary_conversion_event: '核对主转化事件', audit_tracking_chain: '审计完整追踪链路', parallel_tracking_validation: '执行并行追踪验证',
    review_delivery_restrictions: '检查跑量限制', controlled_constraint_test: '执行受控约束测试', expanded_constraint_test: '扩大约束测试',
    review_rejection_reason: '检查审核拒绝原因', replace_flagged_material: '替换风险素材', prepare_compliant_material_set: '准备合规素材组',
    verify_conversion_event: '核对转化事件', audit_conversion_funnel: '审计转化漏斗', controlled_funnel_test: '执行受控漏斗测试',
    limit_budget_pacing: '限制预算节奏', tighten_budget_pacing: '收紧预算节奏', strong_budget_pacing_control: '执行强预算节奏控制',
    limited_cost_control: '执行有限成本控制', cpa_weighted_cost_control: '按转化成本调整预算', strong_cost_control: '执行强成本控制',
    observe_short_window: '观察短周期结果', observe_full_horizon: '观察完整预测周期', prepare_controlled_test: '准备下一轮受控测试',
  } as Record<string, string>)[value ?? ''] ?? value ?? '按既有预算规则调整'
}

function decisionDiagnosticDetail(decision: DeliveryDecision) {
  const explanation = ({
    'no mechanistic simulation is available for the current plan version': '当前精确 PlanVersion 没有概率模拟。',
    'at least two mechanistic metric windows are required': '当前模拟少于两个预测窗口。',
    'the first mechanistic window has no usable median metrics': '首个预测窗口缺少可用的 P50 指标。',
    'the latest mechanistic window has no usable median metrics': '末个预测窗口缺少可用的 P50 指标。',
    'no outcome simulation is available for the current rehearsal': '该记录来自旧模拟链路，现已失效。',
  } as Record<string, string>)[decision.diagnostic.explanation] ?? decisionDiagnosticCopy[decision.diagnostic.code].explanation
  const nextAction = decision.diagnostic.code === 'insufficient_data' && decision.diagnostic.explanation === 'no outcome simulation is available for the current rehearsal'
    ? '请使用当前最新决策。'
    : decisionDiagnosticCopy[decision.diagnostic.code].nextAction
  return { explanation, nextAction }
}

function mechanisticRecommendationLabel(value: string) {
  return ({ review_compliance: '审核合规方案', tracking_review: '追踪链路检查方案', delivery_review: '跑量约束检查方案', cost_review: '成本控制方案', creative_test: '并行素材测试方案（旧结果）', portfolio_test: '并行跑量与淘汰方案', portfolio_observation: '并行跑量观察方案', conversion_funnel_review: '转化漏斗检查方案', budget_pacing_review: '预算节奏检查方案' } as Record<string, string>)[value] ?? value
}

function mechanisticRationaleLabel(value: string) {
  return ({ 'Review the rejection reason or replace non-compliant material.': '检查审核拒绝原因，并替换不合规素材。', 'Check tracking before budget or bid changes.': '在调整预算或出价前，先检查转化追踪链路。', 'Review delivery constraints and use a controlled test.': '检查定向、出价和库存约束，并使用受控测试验证。', 'Review cost assumptions before increasing budget or bid.': '提高预算或出价前，先复核成本先验和成本边界。', 'Consider a controlled creative rotation test.': '使用并行项目或单元测试不同素材，不修改现有跑量对象。', 'Launch parallel projects or promotions with distinct materials; protect stable delivery objects and prune only after a mature observation window.': '为同一商品新建并行项目或单元。使用不同素材跑量。保护稳定对象，只在观察窗口成熟后淘汰低效对象。', 'Launch parallel projects or promotions, protect emerging winners, and prune only after the seven-day observation window matures.': '建立并行项目或单元。保护开始跑量的对象。只在七日观察窗口成熟后淘汰低效对象。', 'Check the conversion funnel before changing delivery settings.': '调整投放设置前，先检查转化漏斗。', 'Review budget pacing before changing the daily budget.': '调整日预算前，先检查预算消耗节奏。', 'Simulation assumptions can differ from platform outcomes.': '模拟假设可能与平台实际结果不同。', 'Winner identity is unknown before platform learning.': '平台学习前不能确定最终跑量单元。', 'Do not execute automatically.': '禁止自动执行。', 'Do not rotate material on an established winner.': '不要轮换已经稳定跑量对象的素材。', 'Do not change budget or bid from this shadow result.': '不要根据该影子结果调整预算或出价。', 'Use a controlled test after human approval.': '人工批准后使用受控测试。' } as Record<string, string>)[value] ?? value
}

function mechanisticTargetLabel(value: string) {
  return ({ material_references: '素材引用', monitoring_references: '监测与追踪配置', 'platform_configuration.payload.ocean_engine.project': '项目投放约束', 'platform_configuration.payload.ocean_engine.project.budget_and_bidding': '预算与出价', 'platform_configuration.payload.ocean_engine.project.optimization_target_reference': '优化目标与转化漏斗', 'platform_configuration.payload.ocean_engine.project.budget_and_bidding.daily_budget_minor': '每日预算' } as Record<string, string>)[value] ?? value
}

function MechanisticRecommendationCard({ item, index }: { item: MechanisticRecommendationDraft; index: number }) {
  return <article className="delivery-recommendation-card">
    <header><div><span>模拟优化方向 {index + 1}</span><h3>{mechanisticRecommendationLabel(item.recommendation_type)}</h3></div><strong className="delivery-recommendation-status proposed">需人工复核</strong></header>
    <dl className="delivery-recommendation-summary"><div><dt>建议目标</dt><dd>{mechanisticTargetLabel(item.target_field)}</dd></div><div><dt>置信度</dt><dd>{item.confidence === 'low' ? '低置信度' : item.confidence === 'medium' ? '中置信度' : '高置信度'}</dd></div><div className="wide"><dt>建议依据</dt><dd>{mechanisticRationaleLabel(item.rationale)}</dd></div><div className="wide"><dt>风险与约束</dt><dd>{[...(item.risks ?? []), ...(item.guardrails ?? [])].map(mechanisticRationaleLabel).join('；')}</dd></div></dl>
    <footer><span>该方案来自上线前概率模拟。它尚未生成冻结配置或 ChangeSet。</span></footer>
  </article>
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof DeliveryApiError) {
    if (error.code === 'VERSION_CONFLICT') return '版本已经更新，请刷新后重新操作。'
    if (error.status === 401 || error.status === 403) return '当前身份无权执行此受控操作。'
    return error.message
  }
  return error instanceof Error ? error.message : fallback
}

export function DeliveryOptimizationPage({ state }: { state: DataState; activeView: string }) {
  const { currentProject } = useProject()
  const projectId = currentProject.id
  const [plans, setPlans] = useState<DeliveryPlan[]>([])
  const [decisions, setDecisions] = useState<DeliveryDecision[]>([])
  const [selectedPlanId, setSelectedPlanId] = useState('')
  const [selection, setSelection] = useState<DeliveryDecisionSelection>()
  const [selectingCandidateId, setSelectingCandidateId] = useState('')
  const [observatoryRuns, setObservatoryRuns] = useState<DeliveryObservatoryRun[]>([])
  const [modifiedBudgets, setModifiedBudgets] = useState<Record<string, string>>({})
  const [proposalDecision, setProposalDecision] = useState<{ selectionId: string; disposition: 'accepted' | 'modified' | 'rejected' }>()
  const workflowRef = useRef<HTMLElement>(null)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [mechanisticSimulation, setMechanisticSimulation] = useState<MechanisticSimulation>()
  const simulationGeneration = useRef(0)

  const refresh = useCallback(async () => {
    setBusy(true)
    try {
      const [nextPlans, nextDecisions, nextRuns] = await Promise.all([deliveryPlanApi.list(projectId), deliveryOptimizationApi.listDecisions(projectId), deliveryOptimizationApi.listObservatoryRuns(projectId)])
      setPlans(nextPlans)
      setDecisions(nextDecisions)
      setObservatoryRuns(nextRuns)
      setSelectedPlanId(current => nextPlans.some(plan => plan.id === current) ? current : nextPlans[0]?.id ?? '')
    } catch (error) {
      setNotice(errorMessage(error, '读取投放决策失败。'))
    } finally {
      setBusy(false)
    }
  }, [projectId])

  useEffect(() => { void refresh() }, [refresh])
  const selectedPlan = plans.find(plan => plan.id === selectedPlanId)
  const selectedPlanDecisions = decisions.filter(decision => decision.inputs.planId === selectedPlanId)
  const currentVersionDecisions = selectedPlanDecisions.filter(decision => decision.inputs.planVersion === selectedPlan?.currentVersionNumber)
  const planDecisions = currentVersionDecisions.slice(0, 1)
  const historicalDecisions = selectedPlanDecisions.filter(decision => decision.id !== planDecisions[0]?.id)

  useEffect(() => {
    const generation = ++simulationGeneration.current
    if (!selectedPlan) {
      setMechanisticSimulation(undefined)
      return
    }
    void deliveryMechanisticSimulationApi.getLatest(projectId, selectedPlan.id, selectedPlan.currentVersionNumber).then(value => {
      if (generation === simulationGeneration.current) setMechanisticSimulation(value)
    }).catch(error => {
      if (generation !== simulationGeneration.current) return
      if (error instanceof DeliveryApiError && error.status === 404) {
        setMechanisticSimulation(undefined)
        return
      }
      setNotice(errorMessage(error, '读取最新概率模拟失败。'))
    })
    return () => { simulationGeneration.current += 1 }
  }, [projectId, selectedPlan?.id, selectedPlan?.currentVersionNumber])

  const generate = async () => {
    if (!selectedPlan) return
    setBusy(true)
    setNotice('')
    try {
      const decision = await deliveryOptimizationApi.generateDecision(projectId, selectedPlan.id, selectedPlan.currentVersionNumber)
      setDecisions(current => [decision, ...current.filter(item => item.id !== decision.id)])
      setNotice('优化方案已生成，请比较方案依据和调整幅度。')
    } catch (error) {
      setNotice(errorMessage(error, '生成优化方案失败。'))
    } finally {
      setBusy(false)
    }
  }

  const selectCandidate = async (decision: DeliveryDecision, candidate: DeliveryDecisionCandidate) => {
    setBusy(true)
    setSelectingCandidateId(candidate.id)
    setNotice('')
    try {
      const result = await deliveryOptimizationApi.selectDecision(projectId, decision.id, candidate.id, decision.inputs.planVersion, `decision-${decision.id}-${candidate.id}`)
      setSelection(result)
      setProposalDecision(undefined)
      setNotice('已生成待确认方案，请核对调整前后差异并作出运营决定。')
      requestAnimationFrame(() => workflowRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' }))
    } catch (error) {
      setNotice(errorMessage(error, '准备待确认方案失败。'))
    } finally {
      setSelectingCandidateId('')
      setBusy(false)
    }
  }

  const submitProposalDecision = async (disposition: 'accepted' | 'modified' | 'rejected') => {
    if (!selection) return
    setBusy(true)
    setNotice('')
    try {
      let run = observatoryRuns.find(item => item.binding.selectionId === selection.id && item.mode === 'prepare_new_local_form')
      if (!run) {
        run = await deliveryOptimizationApi.runObservatory(projectId, selection.id, 'prepare_new_local_form')
        setObservatoryRuns(current => [run!, ...current.filter(item => item.id !== run!.id)])
      }
      const diffKeys = run.steps.flatMap(step => step.diffs.filter(diff => !diff.matches).map(diff => diff.key))
      const reason = disposition === 'accepted' ? '运营已核对方案依据与调整前后差异并接受优化方案' : disposition === 'modified' ? '运营已修改预算并保存调整后的优化方案' : '运营根据方案依据与调整影响拒绝当前优化方案'
      let finalConfiguration
      if (disposition === 'modified') {
        const dailyBudgetYuan = Number(modifiedBudgets[selection.id])
        const dailyBudget = Math.round(dailyBudgetYuan * 100)
        const currentBudget = selection.configuration.payload.ocean_engine?.project.budget_and_bidding.daily_budget_minor
        if (!Number.isFinite(dailyBudgetYuan) || dailyBudgetYuan < 0 || !Number.isInteger(dailyBudgetYuan * 100) || dailyBudget === currentBudget) {
          setNotice('请输入与当前值不同的有效日预算金额，再记录修改后配置。')
          return
        }
        finalConfiguration = structuredClone(selection.configuration)
        finalConfiguration.payload.ocean_engine!.project.budget_and_bidding.daily_budget_minor = dailyBudget
        delete finalConfiguration.canonical_hash
      }
      await deliveryOptimizationApi.submitObservatoryFeedback(projectId, run.id, disposition, reason, diffKeys, `observatory-feedback-${run.id}-${disposition}`, finalConfiguration)
      setProposalDecision({ selectionId: selection.id, disposition })
      setNotice(disposition === 'accepted' ? '优化方案已接受并保存；当前仍不会自动修改广告平台。' : disposition === 'modified' ? '修改后的优化方案已保存；当前仍不会自动修改广告平台。' : '当前优化方案已拒绝，可返回候选方案重新选择或重新生成。')
    } catch (error) {
      setNotice(errorMessage(error, '保存方案处理结果失败。'))
    } finally {
      setBusy(false)
    }
  }

  const selectedCandidate = selection ? decisions.find(decision => decision.id === selection.decisionId)?.candidates.find(candidate => candidate.id === selection.candidateId) : undefined
  const comparisonRows = proposalComparison(selectedPlan, selection, selectedCandidate)
  const selectedRuns = selection ? observatoryRuns.filter(run => run.binding.selectionId === selection.id) : []
  const currentProposalDecision = proposalDecision && proposalDecision.selectionId === selection?.id ? proposalDecision.disposition : undefined

  return <StateBoundary state={state} contextLabel="智能投放 / 优化方案" errorDetail="当前 Project 的优化方案无法读取，请确认 Delivery 服务可用后刷新。">
    <div className="delivery-optimization-workspace">
      <section className="delivery-optimization-toolbar">
        <label>投放计划<select value={selectedPlanId} onChange={event => setSelectedPlanId(event.target.value)}>{plans.map(plan => <option key={plan.id} value={plan.id}>{plan.currentVersion.name} · V{plan.currentVersionNumber}</option>)}</select></label>
        <div className="delivery-optimization-toolbar-actions">
          <button className="secondary-button" onClick={() => void refresh()} disabled={busy}><RefreshCw size={14}/>刷新</button>
          <button className="primary-button" onClick={() => void generate()} disabled={busy || !selectedPlan?.currentVersion.platformConfiguration}><Send size={14}/>生成优化方案</button>
        </div>
      </section>
      <section className="delivery-optimization-context">
        <div><span>方案处理流程</span><b>生成方案 → 对比影响 → 运营确认</b><small>所有调整先保存为本地方案，不会自动修改广告平台。</small></div>
        <div><span>当前业务目标</span><b>{businessObjectiveLabel(selectedPlan?.currentVersion.objective)}</b><small>{selectedPlan ? `计划 ${selectedPlan.currentVersion.name} · V${selectedPlan.currentVersionNumber}` : '选择计划后显示冻结目标与候选配置。'}</small></div>
        <div><CheckCircle2 size={17}/><span><b>方案确认后：等待正式审批</b><small>当前阶段只完成方案确认，不会自动修改广告平台。</small></span></div>
      </section>
      {mechanisticSimulation ? <section className="delivery-optimization-context">
        <div><span>最新上线前模拟</span><b>PlanVersion V{mechanisticSimulation.planVersion} · {mechanisticSimulation.sampleCount.toLocaleString('zh-CN')} 个样本</b><small>校准状态：假设驱动。Run ID：{mechanisticSimulation.id}</small></div>
        <div><ShieldCheck size={17}/><span><b>{mechanisticSimulation.recommendationDrafts.length} 个模拟优化方向</b><small>这些方向会自动随最新模拟显示。正式配置方案仍需人工选择和复核。</small></span></div>
      </section> : null}
      {mechanisticSimulation ? <div className="delivery-config-recommendations delivery-optimization-list">
        {mechanisticSimulation.recommendationDrafts.map((item, index) => <MechanisticRecommendationCard key={`${item.recommendation_type}-${item.target_field}`} item={item} index={index}/>)}
        {!mechanisticSimulation.recommendationDrafts.length ? <div className="panel-empty"><CircleAlert size={18}/>最新概率模拟未触发优化方向。</div> : null}
      </div> : null}
      <div className="delivery-config-recommendations delivery-optimization-list">
        {planDecisions.map(decision => <article className="delivery-recommendation-card delivery-optimization-card" key={decision.id}>
          <header><div><span>{decisionPolicyLabel(decision.policyVersion)} · 计划 V{decision.inputs.planVersion}</span><h3>优化方案建议</h3></div><strong className={`delivery-recommendation-status ${decision.diagnostic.code === 'ready' ? 'proposed' : 'stale'}`}>{decisionDiagnosticCopy[decision.diagnostic.code].label}</strong></header>
          {decision.diagnostic.code !== 'ready' ? <div className="delivery-optimization-stale"><CircleAlert size={17}/><span><b>{decisionDiagnosticDetail(decision).explanation}</b><small>{decisionDiagnosticDetail(decision).nextAction}</small></span></div> : null}
          <section className="delivery-optimization-evidence"><header><div><span>输入绑定</span><b>{decision.evidence.length} 条证据</b></div><code>{decision.canonicalHash.slice(0, 12)}</code></header></section>
          <div className="delivery-config-recommendations">
            {decision.candidates.map(candidate => {
              const selected = selection?.decisionId === decision.id && selection.candidateId === candidate.id
              const selecting = selectingCandidateId === candidate.id
              return <article className="delivery-recommendation-card" key={candidate.id}>
                <header><div><span>{decisionUncertaintyLabel[candidate.uncertainty]}</span><h3>{decisionCandidateLabel[candidate.kind]}</h3></div>{selected ? <strong className="delivery-recommendation-status accepted">已选择</strong> : candidate.id === decision.recommendedCandidateId ? <strong className="delivery-recommendation-status accepted">推荐</strong> : null}</header>
                <dl className="delivery-recommendation-summary"><div><dt>优化焦点</dt><dd>{candidateFocusLabel(candidate.optimizationFocus)}</dd></div><div><dt>方案动作</dt><dd>{candidateActionLabel(candidate.proposedAction)}</dd></div><div><dt>动作范围</dt><dd>{candidate.actionMagnitudePercent ?? 0}%</dd></div><div><dt>情景概率</dt><dd>{candidate.scenarioProbability == null ? '旧规则未记录' : `${(candidate.scenarioProbability * 100).toFixed(1)}%`}</dd></div><div><dt>预算变化</dt><dd>{candidate.budgetChangePercent === 0 ? '保持不变' : `${candidate.budgetChangePercent}%`}</dd></div><div><dt>最终日预算</dt><dd>{candidate.targetConfiguration.payload.ocean_engine?.project ? formatCny(candidate.targetConfiguration.payload.ocean_engine.project.budget_and_bidding.daily_budget_minor) : '待平台能力确认'}</dd></div><div><dt>硬约束</dt><dd>{candidate.constraints.filter(item => item.passed).length}/{candidate.constraints.length} 通过</dd></div><div className="wide"><dt>理由</dt><dd>{candidate.rationale.map(decisionRationaleLabel).join('；')}</dd></div></dl>
                <footer><span>{selected ? '已选为待确认方案' : `方案版本 ${candidate.targetConfiguration.canonical_hash?.slice(0, 8)}`}</span><button className="primary-button" aria-pressed={selected} disabled={busy || selected} onClick={() => void selectCandidate(decision, candidate)}>{selecting ? '正在准备方案…' : selected ? '待运营确认' : '选为待确认方案'}</button></footer>
              </article>
            })}
          </div>
        </article>)}
        {!planDecisions.length ? <div className="panel-empty"><CircleAlert size={18}/>{mechanisticSimulation ? '上方已显示模拟优化方向。点击“生成优化方案”可继续生成冻结候选配置。' : '先运行上线前概率模拟，再为当前计划生成优化方案。'}</div> : null}
      </div>
      {historicalDecisions.length ? <details className="delivery-optimization-history"><summary>历史生成记录（{historicalDecisions.length}）</summary><div>{historicalDecisions.map(decision => <p key={decision.id}><b>计划 V{decision.inputs.planVersion} · {decisionDiagnosticCopy[decision.diagnostic.code].label}</b><span>{formatTime(decision.createdAt)} · {decisionDiagnosticDetail(decision).explanation}</span></p>)}</div></details> : null}
      {selection ? <section ref={workflowRef} className="delivery-proposal-review" aria-live="polite">
        <header className="delivery-proposal-review-heading">
          <div><span>待运营确认</span><h3>优化方案调整明细</h3><p>请根据方案依据和调整前后差异决定是否接受。接受后只保存优化方案，不会自动修改广告平台。</p></div>
          {currentProposalDecision ? <strong className={`delivery-recommendation-status ${currentProposalDecision === 'rejected' ? 'rejected' : 'accepted'}`}>{proposalDecisionLabel[currentProposalDecision]}</strong> : <strong className="delivery-recommendation-status">等待处理</strong>}
        </header>
        <div className="delivery-proposal-comparison" role="table" aria-label="优化方案调整前后对比">
          <div className="delivery-proposal-comparison-head" role="row"><b role="columnheader">调整项</b><b role="columnheader">当前设置</b><b role="columnheader">优化后</b><b role="columnheader">变化</b></div>
          {comparisonRows.map(row => <div className={row.changed ? 'is-changed' : ''} role="row" key={row.key}><b role="cell">{row.label}</b><span role="cell">{row.before}</span><strong role="cell">{row.after}</strong><em role="cell">{row.changed ? '将调整' : '保持不变'}</em></div>)}
        </div>
        <div className="delivery-proposal-safety"><ShieldCheck size={17}/><span><b>方案尚未应用到广告平台</b><small>系统只会保存本次运营决定和方案版本，平台写入保持禁用。</small></span></div>
        <div className="delivery-proposal-actions">
          <button className="secondary-button delivery-proposal-reject" disabled={busy || Boolean(currentProposalDecision)} onClick={() => void submitProposalDecision('rejected')}>拒绝此方案</button>
          <div className="delivery-proposal-budget-edit">
            <label htmlFor={`proposal-budget-${selection.id}`}>需要调整预算？</label>
            <div className="delivery-proposal-budget-input"><span>¥</span><input id={`proposal-budget-${selection.id}`} aria-label="修改后的每日预算" type="number" min="0" step="0.01" placeholder="输入新的每日预算" value={modifiedBudgets[selection.id] ?? ''} onChange={event => setModifiedBudgets(current => ({ ...current, [selection.id]: event.target.value }))}/></div>
            <button className="secondary-button" disabled={busy || Boolean(currentProposalDecision)} onClick={() => void submitProposalDecision('modified')}>保存修改后的方案</button>
          </div>
          <button className="primary-button delivery-proposal-accept" disabled={busy || Boolean(currentProposalDecision)} onClick={() => void submitProposalDecision('accepted')}>接受优化方案</button>
        </div>
        {selectedRuns.length ? <details className="delivery-proposal-audit"><summary>查看系统校验与审计记录</summary><div>{selectedRuns.map(run => <article key={run.id}><span>{formatTime(run.createdAt)}</span><b>{run.status === 'completed' ? '本地校验已完成' : '本地校验需要处理'}</b><small>{run.evidenceRefs.length} 条证据 · 未执行平台写入</small><code>{run.canonicalHash.slice(0, 12)}</code></article>)}</div></details> : null}
      </section> : null}
      {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
    </div>
  </StateBoundary>
}
