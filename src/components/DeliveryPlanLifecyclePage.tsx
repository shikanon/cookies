import { DeliveryPlanPlatformFields } from './DeliveryPlanPlatformFields'
import { ScheduleModeOptions } from './DeliveryChoiceFields'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Boxes, History, Plus, Save } from 'lucide-react'
import {
  deliveryPlanApi,
  type DeliveryPlan,
  type DeliveryPlanDraft,
  type DeliveryScenario,
  type DeliveryPlanVersion,
  type OceanEngineMarketingPurpose,
} from '../api/delivery'
import { useProject } from '../context/ProjectContext'
import { api, type ApiConnectorAccount } from '../data/api'
import { fromShanghaiEndDate, fromShanghaiStartDate, toShanghaiDateInput } from '../lib/deliverySchedule'
import { projectPath } from '../lib/router'
import type { DataState, ProjectRecord } from '../types'
import { StateBoundary } from './StateBoundary'

const planSections = ['目标与账户', '预算与排期', '投放载体和监测', '素材引用'] as const
type PlanSection = typeof planSections[number]

const scenarioLabels: Partial<Record<DeliveryScenario | 'unsaved_draft', string>> = {
  golden_path: '黄金路径',
  budget_zero: '预算为 0',
  creative_unconfirmed: '素材待确认',
  tracking_missing: '追踪缺失',
  incomplete_draft: '草稿不完整',
  project_plan_list: '计划列表',
  approval_queue: '审批队列',
  platform_configuration: '平台配置',
  capability_pending: '能力待补',
  preflight_failure: '预检失败演示',
  approval_expired: '审批过期演示',
  plan_stale: '计划版本过期演示',
  partial_execution: '执行部分成功演示',
  result_unknown: '执行结果未知演示',
  review_rejected_alert: '审核拒绝告警演示',
  unsaved_draft: '未保存草稿',
}

function scenarioMetadata(scenario: DeliveryScenario | 'unsaved_draft') {
  return `${scenarioLabels[scenario] ?? '历史记录'} · scenario=${scenario}`
}

export function DeliveryPlanLifecyclePage({ state }: { state: DataState }) {
  const { currentProject, agencyWorkbench } = useProject()
  const projectId = currentProject.id
  const [plans, setPlans] = useState<DeliveryPlan[]>([])
  const [connectorAccounts, setConnectorAccounts] = useState<ApiConnectorAccount[]>([])
  const requestedPlanId = useRef(new URLSearchParams(window.location.search).get('plan_id') ?? '')
  const [selectedId, setSelectedId] = useState(requestedPlanId.current)
  const [draft, setDraft] = useState<DeliveryPlanDraft>(() => newPlanDraft(currentProject, agencyWorkbench))
  const [section, setSection] = useState<PlanSection>('目标与账户')
  const [isNew, setIsNew] = useState(true)
  const activeSection = !isNew && section === '素材引用' ? '目标与账户' : section
  const visibleSections = isNew ? planSections : planSections.filter(item => item !== '素材引用')
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [inspectedVersionNumber, setInspectedVersionNumber] = useState<number>()
  const preserveEditorState = useRef(false)

  const selectedPlan = useMemo(() => plans.find(plan => plan.id === selectedId), [plans, selectedId])
  const configurationLaunchDisabled = isNew || dirty || busy || !selectedPlan
  const projectProducts = agencyWorkbench?.projects.find(project => project.id === projectId)?.products ?? currentProject.products ?? []
  const configurationLaunchURL = useMemo(() => {
    const base = projectPath(projectId, 'delivery', 'configuration', undefined, '配置映射')
    return selectedPlan ? `${base}&plan_id=${encodeURIComponent(selectedPlan.id)}` : base
  }, [projectId, selectedPlan])
  const inspectedVersion = useMemo(
    () => selectedPlan?.versions.find(version => version.versionNumber === inspectedVersionNumber),
    [inspectedVersionNumber, selectedPlan],
  )
  const strategyTasks = useMemo(() => currentProject.tasks.filter(task => task.type === 'strategy' && (task.status === 'ready' || task.status === 'completed')), [currentProject.tasks])
  const confirmedAssets = useMemo(() => (agencyWorkbench?.assetVersionPointers ?? []).filter(pointer => pointer.projectId === projectId && pointer.humanConfirmedVersion), [agencyWorkbench, projectId])
  const missingPlatformFields = useMemo(() => {
    const missing: string[] = []
    if (!Number.isFinite(draft.budget.totalMinor) || draft.budget.totalMinor <= 0) missing.push('大于 0 的预算')
    if (!draft.advertiser.id) missing.push('账户边界')
    if (!draft.strategyReference.taskId) missing.push('策略来源')
    if (!draft.marketingPurpose) missing.push('巨量营销目的')
    if (!draft.tracking.deliveryCarrier) missing.push('投放载体')
    if ((draft.tracking.deliveryCarrier === 'orange_landing_page' || draft.tracking.deliveryCarrier === 'owned_landing_page') && !draft.tracking.optimizationTargetId) missing.push('优化目标')
    if (draft.tracking.deliveryCarrier === 'owned_landing_page' && !draft.tracking.landingPage) missing.push('自研落地页链接')
    if (!(draft.tracking.searchBidCoefficient > 0)) missing.push('搜索出价系数')
    if (draft.marketingPurpose && draft.marketingPurpose !== 'product_catalog' && !draft.marketingProduct.id) missing.push('营销产品')
    return missing
  }, [draft])
  const platformFieldsComplete = missingPlatformFields.length === 0

  useEffect(() => {
    let active = true
    if (!projectId) return () => { active = false }
    preserveEditorState.current = false
    setBusy(true)
    void Promise.allSettled([deliveryPlanApi.list(projectId), api.listProjectConnectorAccounts(projectId)]).then(([planResult, accountResult]) => {
      if (!active) return
      const verifiedAccounts = accountResult.status === 'fulfilled'
        ? accountResult.value.items.filter(account => account.status === 'verified')
        : []
      setConnectorAccounts(verifiedAccounts)
      if (planResult.status === 'rejected') {
        const message = planResult.reason instanceof Error ? planResult.reason.message : '加载投放计划失败'
        setNotice(accountResult.status === 'rejected' ? `${message}；账号列表也加载失败。` : `${message}。账号列表仍可使用。`)
        return
      }
      const records = planResult.value
      setPlans(records)
      if (preserveEditorState.current) return
      const preferred = records.find(plan => plan.id === requestedPlanId.current) ?? records[0]
      if (preferred) {
        setSelectedId(preferred.id)
        setDraft(draftFromVersion(preferred.currentVersion))
        setIsNew(false)
        setInspectedVersionNumber(preferred.currentVersionNumber)
        setNotice(`已从服务端恢复 ${records.length} 份计划草稿。`)
      } else {
        setSelectedId('')
        setDraft(newPlanDraft(currentProject, agencyWorkbench, verifiedAccounts))
        setIsNew(true)
        setInspectedVersionNumber(undefined)
        setNotice('当前 Project 尚无投放计划，可创建第一份计划草稿。')
      }
      if (accountResult.status === 'rejected') setNotice('投放计划已加载，但账号列表加载失败。请刷新后重试。')
      setDirty(false)
    }).finally(() => {
      if (active) setBusy(false)
    })
    return () => { active = false }
  }, [projectId])

  useEffect(() => {
    const url = new URL(window.location.href)
    if (selectedId) url.searchParams.set('plan_id', selectedId)
    else url.searchParams.delete('plan_id')
    window.history.replaceState(window.history.state, '', url)
  }, [selectedId])

  const changeDraft = (update: (current: DeliveryPlanDraft) => DeliveryPlanDraft) => {
    preserveEditorState.current = true
    setDraft(update)
    setDirty(true)
  }

  const beginNew = () => {
    preserveEditorState.current = true
    setSelectedId('')
    setDraft(newPlanDraft(currentProject, agencyWorkbench, connectorAccounts))
    setSection('目标与账户')
    setIsNew(true)
    setDirty(true)
    setInspectedVersionNumber(undefined)
    setNotice('已创建未保存的计划，请填写后保存草稿。')
  }

  const selectPlan = (plan: DeliveryPlan) => {
    preserveEditorState.current = true
    setSelectedId(plan.id)
    setDraft(draftFromVersion(plan.currentVersion))
    setIsNew(false)
    setDirty(false)
    setInspectedVersionNumber(plan.currentVersionNumber)
    setNotice(`已加载 ${plan.id} 的 V${plan.currentVersionNumber}。`)
  }

  const save = async () => {
    setBusy(true)
    try {
      const saved = isNew || !selectedPlan
        ? await deliveryPlanApi.create(projectId, draft)
        : await deliveryPlanApi.update(projectId, selectedPlan.id, selectedPlan.currentVersionNumber, draft)
      setPlans(current => [...current.filter(plan => plan.id !== saved.id), saved])
      setSelectedId(saved.id)
      setDraft(draftFromVersion(saved.currentVersion))
      setIsNew(false)
      setDirty(false)
      setInspectedVersionNumber(saved.currentVersionNumber)
      setNotice(`${saved.id} 已保存为 V${saved.currentVersionNumber}；source=${saved.source} · scenario=${saved.scenario}。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '保存投放计划失败')
    } finally {
      setBusy(false)
    }
  }

  return <StateBoundary
    state={state}
    contextLabel="智能投放 / 投放计划"
    errorDetail="DeliveryPlan 或服务端预检读取失败。请确认 Go API 与数据库可用后重试。"
  >
    <div className="delivery-lifecycle-workspace">
      <aside className="delivery-plan-list" aria-label="Project 投放计划列表">
        <div className="surface-toolbar">
          <div><span className="section-label">DeliveryPlan</span><h3>计划草稿</h3></div>
          <button aria-label="新建投放计划" onClick={beginNew}><Plus size={15}/></button>
        </div>
        <div className="delivery-plan-scroll">
          {plans.map(plan => <button
            key={plan.id}
            className={plan.id === selectedId ? 'delivery-plan-list-item active' : 'delivery-plan-list-item'}
            onClick={() => selectPlan(plan)}
          >
            <span>{plan.id}</span>
            <b>{plan.currentVersion.name}</b>
            <small>V{plan.currentVersionNumber} · {scenarioMetadata(plan.scenario)}</small>
          </button>)}
          {!plans.length ? <div className="panel-empty">当前 Project 还没有服务端计划。</div> : null}
        </div>
        <button className="secondary-button full" onClick={beginNew}><Plus size={15}/>创建投放计划</button>
      </aside>

      <main className="delivery-plan-editor">
        <header className="delivery-editor-header">
          <div>
            <span className="section-label">{isNew ? '新计划' : `${selectedPlan?.id} · V${selectedPlan?.currentVersionNumber}`}</span>
            <h2>{draft.name || '未命名投放计划'}</h2>
            <p>保存只写入 cookies Delivery 草稿并触发服务端校验；平台配置页可查看编译结果，真实操作在受控执行中心完成。</p>
          </div>
        </header>

        <nav className="plan-tabs" aria-label="投放计划编辑顺序">
          {visibleSections.map(item => <button key={item} className={activeSection === item ? 'active' : ''} onClick={() => setSection(item)}>{item}</button>)}
        </nav>

        <fieldset disabled={Boolean(selectedPlan?.currentVersion.readOnly)} className="delivery-plan-form" aria-label={`${activeSection}编辑区`}>
          {activeSection === '目标与账户' ? <TargetAccountFields draft={draft} changeDraft={changeDraft} strategyTasks={strategyTasks}/> : null}
          {activeSection === '预算与排期' ? <BudgetScheduleFields draft={draft} changeDraft={changeDraft}/> : null}
          {activeSection === '投放载体和监测' ? <TrackingFields draft={draft} changeDraft={changeDraft}/> : null}

          {activeSection !== '预算与排期' ? <div className="delivery-plan-shared-fields"><DeliveryPlanPlatformFields section={activeSection === '目标与账户' ? 'target' : activeSection === '投放载体和监测' ? 'tracking' : 'materials'} projectId={projectId} draft={draft} changeDraft={changeDraft} accounts={connectorAccounts} products={projectProducts} assets={confirmedAssets} isNew={isNew}/></div> : null}
        </fieldset>

        <footer className="delivery-editor-actions">
          <span>{dirty ? '有未保存修改' : selectedPlan ? `已保存 V${selectedPlan.currentVersionNumber}` : '等待创建'}</span>
          {missingPlatformFields.length ? <span className="delivery-required-summary" role="status">还需填写：{missingPlatformFields.join('、')}</span> : null}
          <button className="secondary-button" onClick={() => void save()} disabled={Boolean(selectedPlan?.currentVersion.readOnly) || busy || !platformFieldsComplete || (!dirty && !isNew)} title={missingPlatformFields.length ? `还需填写：${missingPlatformFields.join('、')}` : '保存投放计划'}><Save size={15}/>保存</button>
          {configurationLaunchDisabled
            ? <button className="primary-button" type="button" disabled title="请先保存计划"><Boxes size={15}/>查看平台配置</button>
            : <a className="primary-button" href={configurationLaunchURL}><Boxes size={15}/>查看平台配置</a>}
        </footer>
        {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
      </main>

      <aside className="delivery-version-panel" aria-label="不可变版本历史">
        <div className="surface-toolbar"><div><span className="section-label">Immutable</span><h3>版本历史</h3></div><History size={17}/></div>
        <div className="delivery-version-scroll">
          {selectedPlan?.versions.map(version => <button
            key={version.versionNumber}
            className={inspectedVersionNumber === version.versionNumber ? 'version-history-item active' : 'version-history-item'}
            aria-label={`查看版本 V${version.versionNumber}`}
            onClick={() => setInspectedVersionNumber(version.versionNumber)}
          >
            <span>V{version.versionNumber}</span>
            <b>¥{formatMinor(version.budget.totalMinor)}</b>
            <small>{new Date(version.createdAt).toLocaleString('zh-CN')}</small>
          </button>)}
          {inspectedVersion ? <VersionSnapshot version={inspectedVersion}/> : <div className="panel-empty">保存后可追溯每个不可变版本。</div>}
        </div>
      </aside>
    </div>
  </StateBoundary>
}

function TargetAccountFields({ draft, changeDraft, strategyTasks = [] }: FieldProps) {
  return <div className="delivery-field-grid">
    <label>计划名称<input id="plan_name" aria-label="计划名称" value={draft.name} onChange={event => changeDraft(current => ({ ...current, name: event.target.value }))}/></label>
    <label>业务目标<textarea id="plan_objective" aria-label="业务目标" value={draft.objective} onChange={event => changeDraft(current => ({ ...current, objective: event.target.value }))}/></label>
    <label><span className="delivery-field-label">投放平台</span><input aria-label="投放平台" readOnly value="巨量引擎"/></label>
    <label><span className="delivery-field-label">策略来源{!draft.strategyReference.taskId ? <em>必填</em> : null}</span><select id="strategy_reference" aria-label="策略来源" aria-required="true" required className={!draft.strategyReference.taskId ? 'field-missing' : undefined} value={draft.strategyReference.taskId} onChange={event => {
      const task = strategyTasks.find(candidate => candidate.id === event.target.value)
      changeDraft(current => ({ ...current, strategyReference: { taskId: task?.id ?? '', version: task?.version ?? 0 }, sourceStrategyVersion: task ? `${task.id}@v${task.version}` : '' }))
    }}><option value="">请选择已就绪策略任务</option>{strategyTasks.map(task => <option key={task.id} value={task.id}>{task.name} · V{task.version}</option>)}</select></label>
  </div>
}

function BudgetScheduleFields({ draft, changeDraft }: FieldProps) {
  return <div className="delivery-field-grid">
    <label>{draft.schedule.mode === 'long_term' ? '日预算（CNY）' : '总预算（CNY）'}<input id="budget_total" aria-label={draft.schedule.mode === 'long_term' ? '日预算' : '总预算'} type="number" min="0" step="100" value={draft.budget.totalMinor > 0 ? draft.budget.totalMinor / 100 : ''} onChange={event => changeDraft(current => ({
      ...current,
      budget: { ...current.budget, totalMinor: Math.max(0, Math.round(Number(event.target.value) * 100)) },
    }))}/></label>
    <label>币种<input aria-label="币种" readOnly value={draft.budget.currency}/></label>
    <label>投放周期<select id="schedule_mode" aria-label="投放周期" value={draft.schedule.mode} onChange={event => changeDraft(current => ({ ...current, schedule: { ...current.schedule, mode: event.target.value as DeliveryPlanDraft['schedule']['mode'] } }))}><ScheduleModeOptions/></select></label>
    <label>开始日期<input id="schedule_start" aria-label="开始日期" type="date" value={toShanghaiDateInput(draft.schedule.startAt)} onChange={event => changeDraft(current => ({
      ...current,
      schedule: { ...current.schedule, startAt: fromShanghaiStartDate(event.target.value) },
    }))}/></label>
    {draft.schedule.mode === 'fixed_range' ? <label>结束日期<input id="schedule_end" aria-label="结束日期" type="date" value={toShanghaiDateInput(draft.schedule.endAt)} onChange={event => changeDraft(current => ({
      ...current,
      schedule: { ...current.schedule, endAt: fromShanghaiEndDate(event.target.value) },
    }))}/></label> : null}
    <label>时区<input aria-label="投放时区" readOnly value={draft.schedule.timezone}/></label>
  </div>
}

function TrackingFields({ draft, changeDraft }: FieldProps) {
  return <div className="delivery-field-grid">
    {draft.tracking.deliveryCarrier === 'owned_landing_page' ? <label><span className="delivery-field-label">自研落地页链接{!draft.tracking.landingPage ? <em>必填</em> : null}</span><input id="tracking_landing_page" aria-label="自研落地页链接" aria-required="true" className={!draft.tracking.landingPage ? 'field-missing' : undefined} type="url" required value={draft.tracking.landingPage} onChange={event => changeDraft(current => ({
      ...current,
      tracking: { ...current.tracking, landingPage: event.target.value },
    }))}/></label> : null}
    <label>搜索关键词<input id="tracking_search_keywords" aria-label="搜索关键词" placeholder="使用逗号分隔" value={draft.tracking.searchKeywords} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, searchKeywords: event.target.value } }))}/></label>
    <label><span className="delivery-field-label">搜索出价系数{!(draft.tracking.searchBidCoefficient > 0) ? <em>必填</em> : null}</span><input id="tracking_search_bid_coefficient" aria-label="搜索出价系数" aria-required="true" className={!(draft.tracking.searchBidCoefficient > 0) ? 'field-missing' : undefined} type="number" min="1" step="0.1" required value={draft.tracking.searchBidCoefficient} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, searchBidCoefficient: Number(event.target.value) } }))}/></label>
    <label className="delivery-toggle-field"><span><b>定向扩展</b><small>允许平台扩大搜索流量的定向范围。</small></span><input id="tracking_search_expansion" aria-label="定向扩展" type="checkbox" role="switch" checked={draft.tracking.searchTargetingExpansion} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, searchTargetingExpansion: event.target.checked } }))}/></label>
    <label>展示监测链接<input aria-label="展示监测链接" type="url" value={draft.tracking.monitoringImpression} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, monitoringImpression: event.target.value } }))}/></label>
    <label>有效触点监测链接<input aria-label="有效触点监测链接" type="url" value={draft.tracking.monitoringValidTouch} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, monitoringValidTouch: event.target.value } }))}/></label>
    <label>视频播放监测链接<input aria-label="视频播放监测链接" type="url" value={draft.tracking.monitoringVideoPlay} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, monitoringVideoPlay: event.target.value } }))}/></label>
    <label>视频播完监测链接<input aria-label="视频播完监测链接" type="url" value={draft.tracking.monitoringVideoComplete} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, monitoringVideoComplete: event.target.value } }))}/></label>
    <label>视频有效播放监测链接<input aria-label="视频有效播放监测链接" type="url" value={draft.tracking.monitoringValidVideoPlay} onChange={event => changeDraft(current => ({ ...current, tracking: { ...current.tracking, monitoringValidVideoPlay: event.target.value } }))}/></label>
  </div>
}

function VersionSnapshot({ version }: { version: DeliveryPlanVersion }) {
  return <div className="version-snapshot">
    <span>历史快照 · V{version.versionNumber}</span>
    <dl>
      <div><dt>目标</dt><dd>{version.objective}</dd></div>
      <div><dt>巨量营销目的</dt><dd>{version.marketingPurpose ? marketingPurposeLabel(version.marketingPurpose) : '历史版本未记录'}</dd></div>
      <div><dt>广告主</dt><dd>{version.advertiser.name}</dd></div>
      <div><dt>预算</dt><dd>¥{formatMinor(version.budget.totalMinor)}</dd></div>
      <div><dt>排期</dt><dd>{new Date(version.schedule.startAt).toLocaleDateString('zh-CN')} → {new Date(version.schedule.endAt).toLocaleDateString('zh-CN')}</dd></div>
      <div><dt>策略来源</dt><dd>{version.strategyReference.route ? <a href={version.strategyReference.route}>{version.strategyReference.taskId}@V{version.strategyReference.version}</a> : version.sourceStrategyVersion}</dd></div>
      <div><dt>素材</dt><dd>{version.creativeReferences.map((item, index) => <div key={index}>{item.route ? <a href={item.route}>{item.reference?.display_name_snapshot || item.assetId}</a> : item.reference?.display_name_snapshot || item.assetId} · {item.reference?.namespace || 'cookies'} · V{item.reference?.version ?? item.version}</div>)}</dd></div>
      <div><dt>来源 Hash</dt><dd title={version.strategyReference.contentHash}>{version.strategyReference.contentHash?.slice(0, 12) ?? '—'}</dd></div>
      <div><dt>内容 Hash</dt><dd title={version.canonicalHash}>{version.canonicalHash.slice(0, 12)}</dd></div>
    </dl>
    <small>source={version.source} · scenario={version.scenario}</small>
  </div>
}

type FieldProps = {
  draft: DeliveryPlanDraft
  changeDraft: (update: (current: DeliveryPlanDraft) => DeliveryPlanDraft) => void
  strategyTasks?: ProjectRecord['tasks']
}

function marketingPurposeLabel(value: OceanEngineMarketingPurpose) {
  return ({ ecommerce: '电商', lead_generation: '销售线索', application: '应用', product_catalog: '商品', content_marketing: '内容营销' } as const)[value]
}

function newPlanDraft(project: ProjectRecord, workbench: ReturnType<typeof useProject>['agencyWorkbench'], accounts: ApiConnectorAccount[] = []): DeliveryPlanDraft {
  const strategy = project.tasks.find(task => task.type === 'strategy' && (task.status === 'ready' || task.status === 'completed'))
  const creative = workbench?.assetVersionPointers.find(pointer => pointer.projectId === project.id && pointer.humanConfirmedVersion)
  return {
    name: `${project.brand && project.brand !== '—' ? project.brand : 'Cookies'} 销售线索增长计划`,
    objective: project.goal && !project.goal.startsWith('请启动') ? project.goal : '获取高质量销售线索',
    marketingPurpose: '',
    marketingProduct: { id: '', name: '', activityType: '', activityName: '', brandName: '' },
    advertiser: { id: accounts[0]?.id ?? '', name: accounts[0]?.display_label ?? '', platform: 'ocean_engine' },
    budget: { totalMinor: Math.max(project.budget || 0, 0) * 100, currency: 'CNY' },
    schedule: {
      mode: 'long_term',
      startAt: todayInShanghaiISO(),
      endAt: '2099-12-31T23:59:59+08:00',
      timezone: project.timezone || 'Asia/Shanghai',
    },
    tracking: {
      deliveryCarrier: '',
      landingPage: '',
      pixelId: '',
      conversionEvent: '',
      optimizationTargetId: '', optimizationTargetName: '', optimizationTargetSemanticKey: '', eventAssetName: '', eventAssetType: '',
      searchKeywords: '', searchBidCoefficient: 1.1, searchTargetingExpansion: false,
      monitoringImpression: '', monitoringValidTouch: '', monitoringVideoPlay: '', monitoringVideoComplete: '', monitoringValidVideoPlay: '',
    },
    creativeReferences: creative ? [{
      assetId: creative?.assetId ?? '',
      version: creative?.humanConfirmedVersion ?? 0,
      confirmed: Boolean(creative),
    }] : [],
    strategyReference: { taskId: strategy?.id ?? '', version: strategy?.version ?? 0 },
    sourceStrategyVersion: strategy ? `${strategy.id}@v${strategy.version}` : '',
  }
}

function draftFromVersion(version: DeliveryPlanVersion): DeliveryPlanDraft {
  return {
    platformProject: version.platformProject ? structuredClone(version.platformProject) : undefined,
    name: version.name,
    objective: version.objective,
    marketingPurpose: version.marketingPurpose,
    marketingProduct: version.marketingProduct ?? { id: '', name: '', activityType: '', activityName: '', brandName: '' },
    advertiser: { id: version.advertiser.id, name: version.advertiser.name, platform: version.advertiser.platform },
    budget: { ...version.budget },
    schedule: { ...version.schedule },
    tracking: { ...version.tracking },
    creativeReferences: version.creativeReferences.map(reference => ({ ...reference })),
    strategyReference: { ...version.strategyReference },
    sourceStrategyVersion: version.sourceStrategyVersion,
  }
}

function todayInShanghaiISO() {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date())
  const value = Object.fromEntries(parts.map(part => [part.type, part.value]))
  return `${value.year}-${value.month}-${value.day}T00:00:00+08:00`
}

function formatMinor(value: number) {
  return (value / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
