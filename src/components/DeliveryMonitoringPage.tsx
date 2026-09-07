import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Check, CircleAlert, Clock3, Database, Play, ShieldAlert, X } from 'lucide-react'
import {
  DeliveryApiError, deliveryAlertApi, deliveryMechanisticSimulationApi, deliveryPlanApi,
  type ConnectorInspection, type DeliveryAlert, type DeliveryPlan, type MechanisticPriorSet, type MechanisticSimulation,
} from '../api/delivery'
import { useProject } from '../context/ProjectContext'
import { api, type ApiConnectorAccount, type ApiLaunchBatchCalibration } from '../data/api'
import { StateBoundary } from './StateBoundary'

const typeLabel: Record<DeliveryAlert['type'], string> = {
  review_rejected: '审核拒绝', spend_spike: '消耗突增', zero_conversion: '零转化', cost_worsening: '成本恶化',
  under_delivery: '跑量不足', creative_fatigue: '素材疲劳', tracking_anomaly: '追踪异常',
}
const statusLabel: Record<DeliveryAlert['status'], string> = { open: '需行动', acknowledged: '已确认', dismissed: '已忽略' }
const severityLabel: Record<DeliveryAlert['severity'], string> = { critical: '严重', high: '高', medium: '中', low: '低' }
const freshnessLabel: Record<DeliveryAlert['freshness']['status'], string> = { fresh: '数据新鲜', stale: '数据滞后', unknown: '新鲜度未知', insufficient_data: '数据不足' }
const metricLabel: Record<string, string> = {
  spend: '预计消耗', impressions: '预计曝光', clicks: '预计点击', true_conversions: '预计真实转化', observed_conversions: '预计可观测转化',
  cpm: '千次展示成本（CPM）', ctr: '点击率（CTR）', cpc: '单次点击成本（CPC）', cvr: '转化率（CVR）', cpa: '单次转化成本（CPA）',
}
const scenarioCopy: Record<string, { label: string; detail: string }> = {
  typical_launch: { label: '普通跑量情景', detail: '账号历史中普通项目首个七日窗口的情景概率。' },
  breakout_launch: { label: '跑量号情景', detail: '账号历史中进入高消耗尾部的项目首个七日窗口概率。模型不能提前识别具体跑量单元。' },
  steady: { label: '稳定投放', detail: '审核通过、正常起量且产生转化的模拟样本占比。' },
  under_delivery: { label: '跑量不足', detail: '未起量或没有消耗的模拟样本占比。' },
  cost_pressure: { label: '成本承压', detail: '千次展示成本高于显式先验众数的模拟样本占比。' },
  creative_fatigue: { label: '素材疲劳风险', detail: '显式素材衰减假设生效的模拟样本占比。' },
  tracking_anomaly: { label: '疑似追踪异常', detail: '产生真实转化但没有可观测转化的模拟样本占比。' },
  review_rejected: { label: '审核拒绝风险', detail: '审核未通过的模拟样本占比。' },
  zero_conversion: { label: '零转化风险', detail: '产生点击但没有真实转化的模拟样本占比。' },
  spend_spike: { label: '高预算消耗风险', detail: '单日消耗超过日预算九成的模拟样本占比。' },
}
const scenarioStatusLabel: Record<string, string> = { calibrated: '账号校准结果', simulated: '概率模拟结果', suspected: '疑似风险', known_state: '已知状态' }

type CalibratedAccount = { account: ApiConnectorAccount; calibration?: ApiLaunchBatchCalibration }

type PriorForm = {
  review: number; delivery: number; budgetMin: number; budgetMode: number; budgetMax: number
  cpmMin: number; cpmMode: number; cpmMax: number; ctrMin: number; ctrMode: number; ctrMax: number
  cvrMin: number; cvrMode: number; cvrMax: number; tracking: number; fatigue: number
}
const initialPrior: PriorForm = {
  review: 0.9, delivery: 0.85, budgetMin: 0.4, budgetMode: 0.75, budgetMax: 1,
  cpmMin: 1400, cpmMode: 2800, cpmMax: 4200, ctrMin: 0.005, ctrMode: 0.02, ctrMax: 0.06,
  cvrMin: 0.002, cvrMode: 0.02, cvrMax: 0.08, tracking: 0.9, fatigue: 0.03,
}

export function DeliveryMonitoringPage() {
  const { currentProject } = useProject()
  const queryPlanID = new URLSearchParams(window.location.search).get('plan_id') ?? ''
  const [plans, setPlans] = useState<DeliveryPlan[] | null>(null)
  const [planID, setPlanID] = useState(queryPlanID)
  const [alerts, setAlerts] = useState<DeliveryAlert[] | null>(null)
  const [simulation, setSimulation] = useState<MechanisticSimulation>()
  const [inspection, setInspection] = useState<ConnectorInspection>()
  const [calibratedAccounts, setCalibratedAccounts] = useState<CalibratedAccount[]>([])
  const [calibrationAccountRef, setCalibrationAccountRef] = useState('')
  const [prior, setPrior] = useState<PriorForm>(initialPrior)
  const [stableSeed, setStableSeed] = useState('prelaunch-assumption-v1')
  const [sampleCount, setSampleCount] = useState(5000)
  const [busy, setBusy] = useState<string>()
  const [error, setError] = useState<string>()
  const [forbidden, setForbidden] = useState(false)
  const loadGeneration = useRef(0)
  const selectedPlan = useMemo(() => plans?.find(plan => plan.id === planID), [plans, planID])
  const selectedCalibration = useMemo(() => calibratedAccounts.find(value => value.account.id === calibrationAccountRef)?.calibration, [calibratedAccounts, calibrationAccountRef])

  const load = useCallback(async () => {
    const generation = ++loadGeneration.current
    setError(undefined)
    setForbidden(false)
    try {
      const [loadedPlans, loadedAlerts, connectorAccounts] = await Promise.all([
        deliveryPlanApi.list(currentProject.id),
        deliveryAlertApi.list(currentProject.id, queryPlanID ? { planId: queryPlanID } : {}),
        api.listProjectConnectorAccounts(currentProject.id),
      ])
      const accountCalibrations = await Promise.all(connectorAccounts.items.filter(account => account.status === 'verified').map(async account => {
        try { return { account, calibration: await api.getProjectConnectorLaunchBatchCalibration(currentProject.id, account.id) } }
        catch { return { account } }
      }))
      const nextPlanID = queryPlanID || loadedPlans[0]?.id || ''
      const nextPlan = loadedPlans.find(plan => plan.id === nextPlanID)
      const accountAvailable = accountCalibrations.some(value => value.account.id === nextPlan?.currentVersion.advertiser.id)
      const latestSimulation = nextPlan && accountAvailable ? await readLatestSimulation(currentProject.id, nextPlan) : undefined
      if (generation !== loadGeneration.current) return
      setPlans(loadedPlans)
      setPlanID(current => current || nextPlanID)
      setAlerts(nextPlanID ? loadedAlerts.filter(alert => alert.planId === nextPlanID && alert.source === 'connector') : [])
      setSimulation(latestSimulation)
      setCalibratedAccounts(accountCalibrations)
      setCalibrationAccountRef(nextPlan?.currentVersion.advertiser.id ?? '')
    } catch (reason) {
      if (generation !== loadGeneration.current) return
      setForbidden(reason instanceof DeliveryApiError && (reason.status === 403 || reason.code === 'PROJECT_ACCESS_DENIED'))
      setError(reason instanceof Error ? reason.message : '无法读取投放工作区。')
      setPlans([])
      setAlerts(null)
    }
  }, [currentProject.id, queryPlanID])

  useEffect(() => {
    void load()
    return () => { loadGeneration.current += 1 }
  }, [load])

  const selectPlan = async (nextPlanID: string) => {
    setPlanID(nextPlanID)
    setCalibrationAccountRef(plans?.find(plan => plan.id === nextPlanID)?.currentVersion.advertiser.id ?? '')
    setSimulation(undefined)
    setInspection(undefined)
    setError(undefined)
    try {
      const nextPlan = plans?.find(plan => plan.id === nextPlanID)
      if (!nextPlan) {
        setAlerts([])
        return
      }
      const accountAvailable = calibratedAccounts.some(value => value.account.id === nextPlan.currentVersion.advertiser.id)
      const [loadedAlerts, latestSimulation] = await Promise.all([
        deliveryAlertApi.list(currentProject.id, { planId: nextPlanID }), accountAvailable ? readLatestSimulation(currentProject.id, nextPlan) : Promise.resolve(undefined),
      ])
      setAlerts(loadedAlerts.filter(alert => alert.source === 'connector'))
      setSimulation(latestSimulation)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法读取 Connector 告警。')
    }
  }

  const runSimulation = async () => {
    if (!selectedPlan) return
    setBusy('simulation')
    setError(undefined)
    try {
      setSimulation(await deliveryMechanisticSimulationApi.run(currentProject.id, selectedPlan.id, selectedPlan.currentVersionNumber, {
        stableSeed: stableSeed.trim(), sampleCount, predictionHorizonDays: 7, reviewState: 'unknown', priorSet: buildPriorSet(prior), calibrationAccountRef,
      }))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法运行上线前概率模拟。')
    } finally { setBusy(undefined) }
  }

  const inspect = async () => {
    if (!selectedPlan) return
    setBusy('inspection')
    setError(undefined)
    try {
      const result = await deliveryAlertApi.inspect(currentProject.id, selectedPlan.id)
      setInspection(result)
      setAlerts(result.items)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法运行 Connector 巡检。')
    } finally { setBusy(undefined) }
  }

  const update = async (alert: DeliveryAlert, action: 'acknowledge' | 'dismiss') => {
    setBusy(alert.id)
    setError(undefined)
    try {
      const updated = await deliveryAlertApi.action(currentProject.id, alert.id, action, alert.version)
      setAlerts(current => current?.map(item => item.id === updated.id ? updated : item) ?? null)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法更新告警状态。')
    } finally { setBusy(undefined) }
  }

  const state = forbidden ? 'forbidden' : error && alerts === null ? 'error' : alerts === null ? 'loading' : alerts.length === 0 ? 'empty' : 'ready'
  return <section className="delivery-monitoring" aria-label="投放模拟与巡检">
    <header className="delivery-monitoring__header">
      <div><span className="section-label">智能投放</span><h2>上线前概率模拟与 Connector 巡检</h2><p>概率模拟直接读取冻结的 PlanVersion。上线后告警只读取 Connector 事实。两条链不依赖 Computer Use 执行。</p></div>
      <label>投放计划<select value={planID} onChange={event => void selectPlan(event.target.value)}><option value="">请选择计划</option>{plans?.map(plan => <option key={plan.id} value={plan.id}>{plan.currentVersion.name} · V{plan.currentVersionNumber}</option>)}</select></label>
    </header>
    {error ? <div className="delivery-monitoring__error" role="alert"><CircleAlert size={15}/>{error}</div> : null}
    <section className="delivery-simulation-workspace" aria-label="上线前概率模拟">
      <header><div><span className="section-label">上线前</span><h3>账号校准概率模拟</h3><p>模型读取所选账号的历史项目首个七日窗口。结果区分普通情景和跑量号情景。</p></div><span className={`delivery-simulation-status ${simulation ? 'is-complete' : ''}`}>{simulation ? '模拟已完成' : '等待运行'}</span></header>
      <div className="delivery-simulation-controls">
        <label>Plan 绑定账号<input value={calibratedAccounts.find(value => value.account.id === calibrationAccountRef)?.account.display_label || (calibrationAccountRef ? '账号不属于当前 Project' : '当前 Plan 未绑定账号')} disabled /></label>
        <label>稳定 seed<input value={stableSeed} onChange={event => setStableSeed(event.target.value)} /></label>
        <label>预测窗口<input value="首个 7 日" disabled /></label>
        <label>样本数<input type="number" min={100} max={100000} value={sampleCount} onChange={event => setSampleCount(Number(event.target.value))} /></label>
        <button className="primary-button" disabled={!selectedPlan || !selectedCalibration || busy !== undefined || !stableSeed.trim()} onClick={() => void runSimulation()}><Play size={14} fill="currentColor"/>{busy === 'simulation' ? '模拟中…' : '运行概率模拟'}</button>
      </div>
      {calibrationAccountRef ? <CalibrationSummary value={selectedCalibration}/> : <div className="delivery-monitoring__caution"><ShieldAlert size={15}/>请先为 Plan 绑定包含可用校准结果的 Project 账号。</div>}
      <details className="delivery-alert-card__technical"><summary>补充假设：审核与转化</summary><p>账号报表未校准审核通过率、转化率和追踪可观测率。模型仅把这些值用于转化诊断。</p><div className="delivery-simulation-controls">
        <PriorInput label="审核通过概率" value={prior.review} onChange={review => setPrior(value => ({ ...value, review }))}/>
        <PriorInput label="转化率（CVR）低值" value={prior.cvrMin} onChange={cvrMin => setPrior(value => ({ ...value, cvrMin }))}/>
        <PriorInput label="转化率（CVR）众数" value={prior.cvrMode} onChange={cvrMode => setPrior(value => ({ ...value, cvrMode }))}/>
        <PriorInput label="转化率（CVR）高值" value={prior.cvrMax} onChange={cvrMax => setPrior(value => ({ ...value, cvrMax }))}/>
        <PriorInput label="追踪可观测率" value={prior.tracking} onChange={tracking => setPrior(value => ({ ...value, tracking }))}/>
      </div></details>
      {simulation ? <MechanisticResult value={simulation}/> : <div className="delivery-simulation-empty"><Database size={17}/><span>选择计划并运行模拟。系统不会要求计划先上线。</span></div>}
    </section>
    <section className="delivery-simulation-workspace" aria-label="Connector 巡检">
      <header><div><span className="section-label">上线后</span><h3>Connector 事实巡检</h3><p>巡检使用账户指标窗口。隔离、过期或不完整数据不会生成确定性业务告警。</p></div><button className="secondary-button" disabled={!selectedPlan || busy !== undefined} onClick={() => void inspect()}><ShieldAlert size={14}/>{busy === 'inspection' ? '巡检中…' : '立即巡检'}</button></header>
      {inspection ? <div className={`delivery-monitoring__notice ${inspection.status !== 'ready' ? 'delivery-monitoring__caution' : ''}`}><Database size={15}/><span>状态：{inspectionStatus(inspection.status)}。{inspection.statusReason} 数据集：{inspection.datasetVersion}。{inspection.dataThrough ? `数据覆盖到 ${formatTime(inspection.dataThrough)}。` : ''}</span></div> : null}
      <StateBoundary state={state} contextLabel="投放 / Connector 巡检" emptyTitle="当前没有 Connector 告警" emptyDetail={inspection ? '本次巡检没有生成业务告警。请同时检查上方的数据质量状态。' : '运行 Connector 巡检后，系统会显示数据质量和真实告警。'} onRetry={() => void load()}>
        <div className="delivery-monitoring__list">{alerts?.map(alert => <AlertCard key={alert.id} alert={alert} busy={busy === alert.id} onAction={update}/>)}</div>
      </StateBoundary>
    </section>
  </section>
}

function PriorInput({ label, value, onChange }: { label: string; value: number; onChange: (value: number) => void }) {
  return <label>{label}<input type="number" step="any" value={value} onChange={event => onChange(Number(event.target.value))}/></label>
}
function CalibrationSummary({ value }: { value?: ApiLaunchBatchCalibration }) {
  if (!value) return <div className="delivery-monitoring__caution"><ShieldAlert size={15}/>该账号没有可用的项目首七日校准结果。</div>
  const spend = (items: ApiLaunchBatchCalibration['typical']) => items.find(item => item.metric === 'spend_minor')
  const typicalSpend = spend(value.typical)
  const breakoutSpend = spend(value.breakout)
  return <div className="delivery-monitoring__notice"><Database size={15}/><span>已加载 {value.training_batches} 个训练项目批次。跑量号概率为 {(value.breakout_probability * 100).toFixed(1)}%。普通情景七日消耗 P50 为 {formatQuantile(typicalSpend?.p50, 'CNY_minor')}。跑量号情景七日消耗 P50 为 {formatQuantile(breakoutSpend?.p50, 'CNY_minor')}。</span></div>
}
function buildPriorSet(value: PriorForm): MechanisticPriorSet {
  const source = 'operator://delivery-monitoring-page'
  const scope = ['selected_plan', 'operator_editable_assumption']
  const probability = (input: number) => ({ value: input, source, unit: 'probability' as const, scope, uncertainty: 'Uncalibrated operator assumption. Not an industry benchmark.' })
  const range = (minimum: number, mode: number, maximum: number, unit: string) => ({ minimum, mode, maximum, source, unit, scope, uncertainty: 'Uncalibrated operator range.' })
  return {
    version: 'operator-assumption/v1', review_pass_probability: probability(value.review), delivery_probability: probability(value.delivery),
    budget_utilization: range(value.budgetMin, value.budgetMode, value.budgetMax, 'ratio') as MechanisticPriorSet['budget_utilization'],
    cpm: range(value.cpmMin, value.cpmMode, value.cpmMax, 'CNY_minor_per_1000_impressions'),
    ctr: range(value.ctrMin, value.ctrMode, value.ctrMax, 'ratio') as MechanisticPriorSet['ctr'],
    cvr: range(value.cvrMin, value.cvrMode, value.cvrMax, 'ratio') as MechanisticPriorSet['cvr'],
    tracking_observable_rate: probability(value.tracking),
    creative_fatigue: { enabled: true, daily_rate: value.fatigue, source, unit: 'ratio_per_day', scope, uncertainty: 'Uncalibrated operator assumption.' },
  }
}
function MechanisticResult({ value }: { value: MechanisticSimulation }) {
  const finalWindow = value.metricWindows.at(-1)
  return <div className="delivery-simulation-result">
    <div className="delivery-simulation-summary"><dl><div><dt>计划版本</dt><dd>V{value.planVersion}</dd></div><div><dt>模型</dt><dd>{value.modelVersion}</dd></div><div><dt>先验版本</dt><dd>{value.priorSetVersion}</dd></div><div><dt>样本数</dt><dd>{value.sampleCount.toLocaleString('zh-CN')}</dd></div><div><dt>校准状态</dt><dd>{value.calibrationStatus === 'account_product_calibrated' ? '账号与商品已校准' : '假设驱动'}</dd></div><div><dt>Run ID</dt><dd>{value.id || '平台引用待解析'}</dd></div></dl></div>
    {finalWindow ? <div className="delivery-simulation-metrics">{Object.entries(finalWindow.metrics).map(([name, quantiles]) => <article key={name}><header><span>{metricLabel[name] ?? name}</span><time>窗口 {finalWindow.sequence}</time></header><dl><div><dt>P10</dt><dd>{formatQuantile(quantiles.p10, quantiles.unit)}</dd></div><div><dt>P50</dt><dd>{formatQuantile(quantiles.p50, quantiles.unit)}</dd></div><div><dt>P90</dt><dd>{formatQuantile(quantiles.p90, quantiles.unit)}</dd></div></dl></article>)}</div> : <div className="delivery-monitoring__caution"><ShieldAlert size={15}/>计划存在未解析的平台引用。模拟器已 fail closed。</div>}
    <div className="delivery-simulation-explanation"><div><h4>情景概率</h4>{value.scenarioProbabilities.map(item => { const copy = scenarioCopy[item.scenario] ?? { label: item.scenario, detail: '该情景暂无中文说明。' }; return <p key={item.scenario}><strong>{copy.label} · {(item.probability * 100).toFixed(1)}%</strong><span>{copy.detail} {scenarioStatusLabel[item.status] ?? item.status}。</span></p> })}</div><div><h4>反馈建议草案</h4>{value.recommendationDrafts.length ? value.recommendationDrafts.map(item => <p key={`${item.recommendation_type}-${item.target_field}`}><strong>{recommendationTypeLabel(item.recommendation_type)} · {confidenceLabel(item.confidence)}</strong><span>{recommendationRationale(item.rationale)} 需要人工复核。</span></p>) : <p><span>当前分布未触发建议草案。</span></p>}</div></div>
  </div>
}
function AlertCard({ alert, busy, onAction }: { alert: DeliveryAlert; busy: boolean; onAction: (alert: DeliveryAlert, action: 'acknowledge' | 'dismiss') => Promise<void> }) {
  return <article className={`delivery-alert-card severity-${alert.severity}`}><header><div><span className="delivery-alert-card__severity">风险等级：{severityLabel[alert.severity]}</span><h3>{typeLabel[alert.type]}</h3></div><span className={`delivery-alert-card__status status-${alert.status}`}>{statusLabel[alert.status]}</span></header>
    <p className="delivery-alert-card__summary">{describeAlert(alert)}</p><dl><div><dt>平台对象</dt><dd>{alert.monitoredEntity.id}</dd></div><div><dt>监控窗口</dt><dd>{formatTime(alert.window.start)} 至 {formatTime(alert.window.end)}</dd></div><div><dt>数据覆盖到</dt><dd>{formatTime(alert.window.dataThrough)} · {freshnessLabel[alert.freshness.status]}</dd></div><div><dt>证据</dt><dd>{alert.evidenceRefs.length} 条 Connector 记录</dd></div></dl>
    <details className="delivery-alert-card__technical"><summary>查看技术标识</summary><span>规则：{alert.ruleId} {alert.ruleVersion} · 数据集：{alert.datasetVersion} · 口径：{alert.fixtureVersion}</span></details>
    {alert.status === 'open' ? <footer><button className="secondary-button" disabled={busy} onClick={() => void onAction(alert, 'acknowledge')}><Check size={14}/>确认跟进</button><button className="secondary-button" disabled={busy} onClick={() => void onAction(alert, 'dismiss')}><X size={14}/>忽略</button></footer> : <footer><Clock3 size={14}/>{alert.status === 'acknowledged' ? '已确认。告警记录保留。' : '已忽略。处置记录保留。'}</footer>}
  </article>
}
function describeAlert(alert: DeliveryAlert) {
  const metric = alert.metricDefinition
  if (alert.type === 'zero_conversion') return `当前窗口有 ${metric.denominator ?? 0} 次点击，但没有转化。`
  return `当前值 ${formatQuantile(metric.observedValue, metric.unit)}。上一窗口 ${formatQuantile(metric.baselineValue, metric.unit)}。规则阈值 ${formatQuantile(metric.threshold, metric.unit)}。`
}
function inspectionStatus(value: ConnectorInspection['status']) { return ({ ready: '可评估', insufficient_data: '数据不足', quarantined: '数据隔离', stale: '数据过期', unavailable: 'Connector 不可用' })[value] }
function recommendationTypeLabel(value: string) { return ({ review_compliance: '审核合规检查', tracking_review: '追踪链路检查', delivery_review: '跑量约束检查', cost_review: '成本控制检查', creative_test: '并行素材测试（旧结果）', portfolio_test: '并行跑量与淘汰', portfolio_observation: '并行跑量观察', conversion_funnel_review: '转化漏斗检查', budget_pacing_review: '预算节奏检查' } as Record<string, string>)[value] ?? value }
function confidenceLabel(value: string) { return ({ low: '低置信度', medium: '中置信度', high: '高置信度' } as Record<string, string>)[value] ?? value }
function recommendationRationale(value: string) { return ({ 'Review the rejection reason or replace non-compliant material.': '检查审核拒绝原因，并替换不合规素材。', 'Check tracking before budget or bid changes.': '在调整预算或出价前，先检查转化追踪链路。', 'Review delivery constraints and use a controlled test.': '检查定向、出价和库存约束，并使用受控测试验证。', 'Review cost assumptions before increasing budget or bid.': '提高预算或出价前，先复核成本先验和成本边界。', 'Consider a controlled creative rotation test.': '使用并行项目或单元测试不同素材，不修改现有跑量对象。', 'Launch parallel projects or promotions with distinct materials; protect stable delivery objects and prune only after a mature observation window.': '为同一商品新建并行项目或单元。使用不同素材跑量。保护稳定对象，只在观察窗口成熟后淘汰低效对象。', 'Launch parallel projects or promotions, protect emerging winners, and prune only after the seven-day observation window matures.': '建立并行项目或单元。保护开始跑量的对象。只在七日观察窗口成熟后淘汰低效对象。', 'Check the conversion funnel before changing delivery settings.': '调整投放设置前，先检查转化漏斗。', 'Review budget pacing before changing the daily budget.': '调整日预算前，先检查预算消耗节奏。' } as Record<string, string>)[value] ?? value }
async function readLatestSimulation(projectID: string, plan: DeliveryPlan) {
  try {
    return await deliveryMechanisticSimulationApi.getLatest(projectID, plan.id, plan.currentVersionNumber)
  } catch (reason) {
    if (reason instanceof DeliveryApiError && reason.status === 404) return undefined
    throw reason
  }
}
function formatTime(value: string) { const date = new Date(value); return Number.isNaN(date.valueOf()) ? value : date.toLocaleString('zh-CN', { hour12: false }) }
function formatQuantile(value: number | undefined, unit: string) { if (value === undefined) return '不可用'; if (unit.includes('minor') || unit.includes('fen')) return `¥${(value / 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })}`; if (unit === 'ratio') return `${(value * 100).toFixed(2)}%`; return value.toLocaleString('zh-CN', { maximumFractionDigits: 2 }) }
