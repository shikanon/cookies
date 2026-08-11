import { useEffect, useMemo, useRef, useState } from 'react'
import { CircleAlert, CircleCheck, FilePlus, History, Plus, Save, Send, Wrench } from 'lucide-react'
import {
  deliveryPlanApi,
  type DeliveryPlan,
  type DeliveryPlanDraft,
  type DeliveryScenario,
  type DeliveryPlanVersion,
  type DeliveryPreflightResult,
} from '../api/delivery'
import { useProject } from '../context/ProjectContext'
import type { ApiAssetVersionPointer } from '../data/api'
import type { DataState, ProjectRecord } from '../types'
import { StateBoundary } from './StateBoundary'

const planSections = ['目标与账户', '预算与排期', '追踪', '素材引用', '投前检查'] as const
type PlanSection = typeof planSections[number]

const scenarioLabels: Record<DeliveryScenario | 'unsaved_draft', string> = {
  golden_path: '黄金路径',
  budget_zero: '预算为 0',
  creative_unconfirmed: '素材待确认',
  tracking_missing: '追踪缺失',
  incomplete_draft: '草稿不完整',
  project_plan_list: '计划列表',
  approval_queue: '审批队列',
  missing_required_field: '三层配置必填项缺失',
  orphan_dependency: '三层配置依赖缺失',
  missing_confirmation: '三层配置待人工确认',
  platform_fields_pending: '平台字段待人工填写',
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

const preflightCheckLabels: Record<DeliveryPreflightResult['checks'][number]['code'], string> = {
  advertiser_available: '广告主可用',
  budget_positive: '预算大于 0',
  schedule_valid: '排期有效',
  creative_present: '素材引用完整',
  creative_confirmed: '素材已确认',
  tracking_complete: '追踪配置完整',
  three_tier_structure: '三层配置结构有效',
  three_tier_required_fields: '三层配置必填项完整',
  three_tier_dependencies: '三层配置依赖可解析',
  three_tier_confirmation: '三层配置已人工确认',
  three_tier_platform_pending: '平台字段待人工填写',
  upstream_references_resolved: '策略任务与素材版本可追溯',
  delivery_intent_valid: '业务意图有效',
  platform_configuration_valid: '平台配置有效',
  INVALID_STABLE_REFERENCE: '稳定引用无效',
  CANONICAL_HASH_MISMATCH: '规范哈希不匹配',
  CAPABILITY_PENDING: '平台能力待补',
  platform_pending: '平台字段待补',
  blocked_by_event_asset: '事件资产阻塞',
  write_validation_pending: '真实写入待验证',
}

function scenarioMetadata(scenario: DeliveryScenario | 'unsaved_draft') {
  return `${scenarioLabels[scenario]} · scenario=${scenario}`
}

export function DeliveryPlanLifecyclePage({ state }: { state: DataState }) {
  const { currentProject, agencyWorkbench } = useProject()
  const projectId = currentProject.id
  const [plans, setPlans] = useState<DeliveryPlan[]>([])
  const requestedPlanId = useRef(new URLSearchParams(window.location.search).get('plan_id') ?? '')
  const [selectedId, setSelectedId] = useState(requestedPlanId.current)
  const [draft, setDraft] = useState<DeliveryPlanDraft>(() => newMockDraft(currentProject, agencyWorkbench))
  const [section, setSection] = useState<PlanSection>('目标与账户')
  const [isNew, setIsNew] = useState(true)
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [preflight, setPreflight] = useState<DeliveryPreflightResult>()
  const [inspectedVersionNumber, setInspectedVersionNumber] = useState<number>()
  const [repairField, setRepairField] = useState('')
  const preserveEditorState = useRef(false)

  const selectedPlan = useMemo(() => plans.find(plan => plan.id === selectedId), [plans, selectedId])
  const inspectedVersion = useMemo(
    () => selectedPlan?.versions.find(version => version.versionNumber === inspectedVersionNumber),
    [inspectedVersionNumber, selectedPlan],
  )
  const strategyTasks = useMemo(() => currentProject.tasks.filter(task => task.type === 'strategy' && (task.status === 'ready' || task.status === 'completed')), [currentProject.tasks])
  const confirmedAssets = useMemo(() => (agencyWorkbench?.assetVersionPointers ?? []).filter(pointer => pointer.projectId === projectId && pointer.humanConfirmedVersion), [agencyWorkbench, projectId])

  useEffect(() => {
    let active = true
    if (!projectId) return () => { active = false }
    preserveEditorState.current = false
    setBusy(true)
    void deliveryPlanApi.list(projectId).then(records => {
      if (!active) return
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
        setDraft(newMockDraft(currentProject, agencyWorkbench))
        setIsNew(true)
        setInspectedVersionNumber(undefined)
        setNotice('当前 Project 尚无投放计划，可创建第一份计划草稿。')
      }
      setDirty(false)
      setPreflight(undefined)
    }).catch(error => {
      if (active) setNotice(error instanceof Error ? error.message : '加载投放计划失败')
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

  useEffect(() => {
    if (!repairField) return
    const target = document.getElementById(repairField)
    if (!target) return
    target.focus()
    target.scrollIntoView({ behavior: 'smooth', block: 'center' })
    setRepairField('')
  }, [repairField, section])

  const changeDraft = (update: (current: DeliveryPlanDraft) => DeliveryPlanDraft) => {
    preserveEditorState.current = true
    setDraft(update)
    setDirty(true)
    setPreflight(undefined)
  }

  const beginNew = () => {
    preserveEditorState.current = true
    setSelectedId('')
    setDraft(newMockDraft(currentProject, agencyWorkbench))
    setSection('目标与账户')
    setIsNew(true)
    setDirty(true)
    setPreflight(undefined)
    setInspectedVersionNumber(undefined)
    setNotice('已创建未保存的计划，请填写后保存草稿。')
  }

  const selectPlan = (plan: DeliveryPlan) => {
    preserveEditorState.current = true
    setSelectedId(plan.id)
    setDraft(draftFromVersion(plan.currentVersion))
    setIsNew(false)
    setDirty(false)
    setPreflight(undefined)
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
      setPreflight(undefined)
      setInspectedVersionNumber(saved.currentVersionNumber)
      setNotice(`${saved.id} 已保存为 V${saved.currentVersionNumber}；source=${saved.source} · scenario=${saved.scenario}。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '保存投放计划失败')
    } finally {
      setBusy(false)
    }
  }

  const runPreflight = async () => {
    if (!selectedPlan || dirty) return
    setBusy(true)
    try {
      const result = await deliveryPlanApi.preflight(projectId, selectedPlan.id)
      setPreflight(result)
      setSection('投前检查')
      const warningCount = result.checks.filter(check => !check.passed && check.severity === 'warning').length
      setNotice(result.blocked
        ? `服务端预检阻断：scenario=${result.scenario}。`
        : `服务端预检通过${warningCount ? `，含 ${warningCount} 条 warning` : ''}；scenario=${result.scenario}。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '运行服务端预检失败')
    } finally {
      setBusy(false)
    }
  }

  const createChangeSet = async () => {
    if (!selectedPlan || dirty || !preflight?.passed) return
    setBusy(true)
    try {
      const created = await deliveryPlanApi.createChangeSet(projectId, selectedPlan.id, selectedPlan.currentVersionNumber)
      const checked = await deliveryPlanApi.preflightChangeSet(projectId, created.id, created.version)
      setNotice(
        checked.status === 'preflight_passed'
          ? `${checked.id} 已冻结 Plan V${checked.planVersion} 并通过服务端预检，可前往审批中心。`
          : `${checked.id} 的最终检查未通过，请修复计划后重新提交变更申请。`,
      )
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '提交变更申请失败')
    } finally {
      setBusy(false)
    }
  }

  const repair = (target: { field: string; section: string }) => {
    if (isPlanSection(target.section)) setSection(target.section)
    setRepairField(target.field)
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
            <p>草稿只写入 cookies Delivery 服务；投前检查结果仅采用服务端返回。</p>
          </div>
        </header>

        <nav className="plan-tabs" aria-label="投放计划编辑顺序">
          {planSections.map(item => <button key={item} className={section === item ? 'active' : ''} onClick={() => setSection(item)}>{item}</button>)}
        </nav>

        <section className="delivery-plan-form" aria-label={`${section}编辑区`}>
          {section === '目标与账户' ? <TargetAccountFields draft={draft} changeDraft={changeDraft} strategyTasks={strategyTasks}/> : null}
          {section === '预算与排期' ? <BudgetScheduleFields draft={draft} changeDraft={changeDraft}/> : null}
          {section === '追踪' ? <TrackingFields draft={draft} changeDraft={changeDraft}/> : null}
          {section === '素材引用' ? <CreativeFields draft={draft} changeDraft={changeDraft} confirmedAssets={confirmedAssets}/> : null}
          {section === '投前检查' ? <PreflightPanel result={preflight} onRepair={repair}/> : null}
        </section>

        <footer className="delivery-editor-actions">
          <span>{dirty ? '有未保存修改' : selectedPlan ? `已保存 V${selectedPlan.currentVersionNumber}` : '等待创建'}</span>
          <button
            className="secondary-button"
            title="创建空白草稿，不继承当前表单内容"
            onClick={beginNew}
            disabled={busy}
          ><FilePlus size={15}/>新建空白草稿</button>
          <button className="secondary-button" onClick={() => void save()} disabled={busy || (!dirty && !isNew)}><Save size={15}/>{isNew ? '保存草稿' : '保存新版本'}</button>
          <button className="primary-button" onClick={() => void runPreflight()} disabled={busy || !selectedPlan || dirty}><Send size={15}/>检查当前草稿</button>
          <button className="primary-button" onClick={() => void createChangeSet()} disabled={busy || !selectedPlan || dirty || !preflight?.passed}><FilePlus size={15}/>提交变更申请</button>
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
  const hasPlanAdvertiserOption = Boolean(draft.advertiser.id && draft.advertiser.id !== 'mock-advertiser-001')
  return <div className="delivery-field-grid">
    <label>计划名称<input id="plan_name" aria-label="计划名称" value={draft.name} onChange={event => changeDraft(current => ({ ...current, name: event.target.value }))}/></label>
    <label>业务目标<textarea id="plan_objective" aria-label="业务目标" value={draft.objective} onChange={event => changeDraft(current => ({ ...current, objective: event.target.value }))}/></label>
    <label><span className="delivery-field-label">投放平台</span><input aria-label="投放平台" readOnly value="巨量引擎"/></label>
    <label><span className="delivery-field-label">账户边界{!draft.advertiser.id ? <em>必填</em> : null}</span><select id="advertiser_id" aria-label="账户边界" aria-required="true" required className={!draft.advertiser.id ? 'field-missing' : undefined} value={draft.advertiser.id} onChange={event => changeDraft(current => ({
      ...current,
      advertiser: event.target.value
        ? { id: event.target.value, name: '当前演示账户边界', platform: 'ocean_engine' }
        : { id: '', name: '', platform: 'ocean_engine' },
    }))}><option value="">请选择账户边界</option>{hasPlanAdvertiserOption ? <option value={draft.advertiser.id}>{draft.advertiser.name || '当前计划账户边界'}</option> : null}<option value="mock-advertiser-001">当前演示账户边界</option></select></label>
    <label><span className="delivery-field-label">策略来源{!draft.strategyReference.taskId ? <em>必填</em> : null}</span><select id="strategy_reference" aria-label="策略来源" aria-required="true" required className={!draft.strategyReference.taskId ? 'field-missing' : undefined} value={draft.strategyReference.taskId} onChange={event => {
      const task = strategyTasks.find(candidate => candidate.id === event.target.value)
      changeDraft(current => ({ ...current, strategyReference: { taskId: task?.id ?? '', version: task?.version ?? 0 }, sourceStrategyVersion: task ? `${task.id}@v${task.version}` : '' }))
    }}><option value="">请选择已就绪策略任务</option>{strategyTasks.map(task => <option key={task.id} value={task.id}>{task.name} · V{task.version}</option>)}</select></label>
    <div className="field-provenance"><b>可追溯来源</b><span>保存时由服务端解析策略任务版本并写入内容哈希与返回入口。</span></div>
  </div>
}

function BudgetScheduleFields({ draft, changeDraft }: FieldProps) {
  return <div className="delivery-field-grid">
    <label>总预算（CNY）<input id="budget_total" aria-label="总预算" type="number" min="0" step="100" value={draft.budget.totalMinor / 100} onChange={event => changeDraft(current => ({
      ...current,
      budget: { ...current.budget, totalMinor: Math.max(0, Math.round(Number(event.target.value) * 100)) },
    }))}/></label>
    <label>币种<input aria-label="币种" readOnly value={draft.budget.currency}/></label>
    <label>开始时间<input id="schedule_start" aria-label="开始时间" type="datetime-local" value={toDateTimeLocal(draft.schedule.startAt)} onChange={event => changeDraft(current => ({
      ...current,
      schedule: { ...current.schedule, startAt: fromDateTimeLocal(event.target.value) },
    }))}/></label>
    <label>结束时间<input id="schedule_end" aria-label="结束时间" type="datetime-local" value={toDateTimeLocal(draft.schedule.endAt)} onChange={event => changeDraft(current => ({
      ...current,
      schedule: { ...current.schedule, endAt: fromDateTimeLocal(event.target.value) },
    }))}/></label>
    <label>时区<input aria-label="投放时区" readOnly value={draft.schedule.timezone}/></label>
  </div>
}

function TrackingFields({ draft, changeDraft }: FieldProps) {
  return <div className="delivery-field-grid">
    <label>落地页<input id="tracking_landing_page" aria-label="追踪落地页" value={draft.tracking.landingPage} onChange={event => changeDraft(current => ({
      ...current,
      tracking: { ...current.tracking, landingPage: event.target.value },
    }))}/></label>
    <label>像素 ID<input id="tracking_pixel_id" aria-label="追踪像素 ID" value={draft.tracking.pixelId} onChange={event => changeDraft(current => ({
      ...current,
      tracking: { ...current.tracking, pixelId: event.target.value },
    }))}/></label>
    <label>转化事件<input id="tracking_conversion_event" aria-label="转化事件" value={draft.tracking.conversionEvent} onChange={event => changeDraft(current => ({
      ...current,
      tracking: { ...current.tracking, conversionEvent: event.target.value },
    }))}/></label>
  </div>
}

function CreativeFields({ draft, changeDraft, confirmedAssets = [] }: FieldProps) {
  const reference = draft.creativeReferences[0] ?? { assetId: '', version: 1, confirmed: true }
  const updateReference = (patch: Partial<typeof reference>) => changeDraft(current => ({
    ...current,
    creativeReferences: [{ ...(current.creativeReferences[0] ?? reference), ...patch }],
  }))
  return <div className="delivery-field-grid">
    <label><span className="delivery-field-label">已确认素材{!reference.assetId ? <em>必填</em> : null}</span><select id="creative_asset_id" aria-label="已确认素材" aria-required="true" required className={!reference.assetId ? 'field-missing' : undefined} value={reference.assetId} onChange={event => {
      const pointer = confirmedAssets.find(candidate => candidate.assetId === event.target.value)
      updateReference({ assetId: pointer?.assetId ?? '', version: pointer?.humanConfirmedVersion ?? 0, confirmed: Boolean(pointer) })
    }}><option value="">请选择已人工确认素材</option>{confirmedAssets.map(pointer => <option key={pointer.id} value={pointer.assetId}>{pointer.assetId} · V{pointer.humanConfirmedVersion}</option>)}</select></label>
    <label>引用版本<input id="creative_version" aria-label="素材版本" readOnly value={reference.version || '—'}/></label>
    <div className="field-provenance"><b>{reference.confirmed ? '已人工确认' : '尚未选择'}</b><span>保存时由服务端校验 Workbench 指针并冻结内容哈希。</span></div>
  </div>
}

function PreflightPanel({ result, onRepair }: { result?: DeliveryPreflightResult; onRepair: (target: { field: string; section: string }) => void }) {
  if (!result) return <div className="preflight-authority-empty"><Send size={22}/><h3>等待服务端预检</h3><p>先保存当前草稿，再运行预检。页面不会使用本地 helper 替代服务端结论。</p></div>
  const failed = result.checks.filter(check => !check.passed)
  return <div className="server-preflight-panel">
    <header>
      <div>{result.blocked ? <CircleAlert size={22}/> : <CircleCheck size={22}/>}<span><b>{result.blocked ? '服务端预检阻断' : '服务端预检通过'}</b><small>V{result.planVersion} · {new Date(result.checkedAt).toLocaleString('zh-CN')}</small></span></div>
      <small>{scenarioMetadata(result.scenario)}</small>
    </header>
    <div className="preflight-checks" role="list" aria-label="服务端投前检查结果">
      {result.checks.map(check => <article key={check.code} className={`preflight-check ${check.passed ? 'passed' : check.severity}`}>
        <span className="preflight-severity">{check.passed ? 'pass' : check.severity}</span>
        <div><b>{preflightCheckLabels[check.code]}</b><small>{check.code}</small><p>{check.message}</p></div>
        {!check.passed && check.repair ? <button aria-label={`修复 ${check.code}`} onClick={() => onRepair(check.repair!)}><Wrench size={14}/>{check.repair.label}</button> : null}
      </article>)}
    </div>
    {!failed.length ? <div className="preflight-golden"><CircleCheck size={16}/>黄金场景全部通过，可继续后续受控流程。</div> : null}
  </div>
}

function VersionSnapshot({ version }: { version: DeliveryPlanVersion }) {
  return <div className="version-snapshot">
    <span>历史快照 · V{version.versionNumber}</span>
    <dl>
      <div><dt>目标</dt><dd>{version.objective}</dd></div>
      <div><dt>广告主</dt><dd>{version.advertiser.name}</dd></div>
      <div><dt>预算</dt><dd>¥{formatMinor(version.budget.totalMinor)}</dd></div>
      <div><dt>排期</dt><dd>{new Date(version.schedule.startAt).toLocaleDateString('zh-CN')} → {new Date(version.schedule.endAt).toLocaleDateString('zh-CN')}</dd></div>
      <div><dt>策略来源</dt><dd>{version.strategyReference.route ? <a href={version.strategyReference.route}>{version.strategyReference.taskId}@V{version.strategyReference.version}</a> : version.sourceStrategyVersion}</dd></div>
      <div><dt>素材</dt><dd>{version.creativeReferences.map(reference => reference.route ? <a key={`${reference.assetId}-${reference.version}`} href={reference.route}>{reference.assetId}@V{reference.version}</a> : `${reference.assetId}@V${reference.version}`)}</dd></div>
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
  confirmedAssets?: ApiAssetVersionPointer[]
}

function newMockDraft(project: ProjectRecord, workbench: ReturnType<typeof useProject>['agencyWorkbench']): DeliveryPlanDraft {
  const code = project.code && project.code !== '—' ? project.code : 'LOCAL'
  const strategy = project.tasks.find(task => task.type === 'strategy' && (task.status === 'ready' || task.status === 'completed'))
  const creative = workbench?.assetVersionPointers.find(pointer => pointer.projectId === project.id && pointer.humanConfirmedVersion)
  return {
    name: `${project.brand && project.brand !== '—' ? project.brand : 'Cookies'} 销售线索增长计划`,
    objective: project.goal && !project.goal.startsWith('请启动') ? project.goal : '获取高质量销售线索',
    advertiser: { id: 'mock-advertiser-001', name: '当前演示账户边界', platform: 'ocean_engine' },
    budget: { totalMinor: Math.max(project.budget || 3000, 0) * 100, currency: 'CNY' },
    schedule: {
      startAt: '2026-08-01T00:00:00.000Z',
      endAt: '2026-08-31T00:00:00.000Z',
      timezone: project.timezone || 'Asia/Shanghai',
    },
    tracking: {
      landingPage: `https://demo.cookies.local/lead/${code.toLowerCase()}`,
      pixelId: `PX-${code}-LEAD`,
      conversionEvent: 'lead_submit',
    },
    creativeReferences: [{
      assetId: creative?.assetId ?? '',
      version: creative?.humanConfirmedVersion ?? 0,
      confirmed: Boolean(creative),
    }],
    strategyReference: { taskId: strategy?.id ?? '', version: strategy?.version ?? 0 },
    sourceStrategyVersion: strategy ? `${strategy.id}@v${strategy.version}` : '',
  }
}

function draftFromVersion(version: DeliveryPlanVersion): DeliveryPlanDraft {
  return {
    name: version.name,
    objective: version.objective,
    advertiser: { id: version.advertiser.id, name: version.advertiser.name, platform: version.advertiser.platform },
    budget: { ...version.budget },
    schedule: { ...version.schedule },
    tracking: { ...version.tracking },
    creativeReferences: version.creativeReferences.map(reference => ({ ...reference })),
    strategyReference: { ...version.strategyReference },
    sourceStrategyVersion: version.sourceStrategyVersion,
  }
}

function toDateTimeLocal(value: string) {
  return value ? new Date(value).toISOString().slice(0, 16) : ''
}

function fromDateTimeLocal(value: string) {
  return value ? new Date(`${value}:00+08:00`).toISOString() : ''
}

function formatMinor(value: number) {
  return (value / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function isPlanSection(value: string): value is PlanSection {
  return planSections.includes(value as PlanSection)
}
