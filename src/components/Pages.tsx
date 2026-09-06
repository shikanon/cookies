import { lazy, Suspense, useEffect, useMemo, useState, type CSSProperties } from 'react'
import { ArrowRight, Bot, Check, ChevronDown, CircleAlert, CircleCheck, ClipboardCheck, Clock3, Download, ExternalLink, Filter, MoreHorizontal, Pencil, Plus, Search, Send, ShieldCheck, SlidersHorizontal } from 'lucide-react'
import { systems, quickActions } from '../data/navigation'
import { api, type ApiAdAccountBinding, type ApiAgencyWorkbench, type ApiAgentRun, type ApiArtifact, type ApiAssetVersionPointer, type ApiAuditEvent, type ApiBindingHealthStatus, type ApiMaterialConfirmation, type ApiOperationalRecord, type ApiOperationalRecordKind, type ApiQualityCheckRun, type ApiRemixEvalCase, type ApiRemixEvalRun } from '../data/api'
import { useProject } from '../context/ProjectContext'
import { useModelConfig } from '../context/ModelConfigContext'
import type { BusinessTaskRecord, BusinessTaskType, DataState, NavItem, ProjectRecord, SystemDefinition, SystemKey } from '../types'
import { calculateProjectProgress, progressPercentLabel, progressReasonLabel, progressStatusLabel } from '../lib/project-progress'
import { TrendChart } from './Icons'
import { shortId } from '../data/shortId'
// 洞察各入口的页面本身在下面跟着其他页面一起 lazy，这里只静态引它们的视图类型
// ——类型在编译期就消掉了，不会把 chunk 拽回主包。
import type { PreLaunchView } from './insight/prelaunch'
import type { AnalysisView } from './insight/analysis'
import type { AssetsView } from './insight/assets'
import type { ReviewView } from './insight/review'
import type { ExperienceView } from './insight/experience'
import type { SettingsView } from './insight/settings'
import { StateBoundary, StatePreview } from './StateBoundary'
import { KanonStrategyTaskCenter, KanonStrategyTaskDialog } from '../features/strategy/KanonStrategyTaskCenter'
import { KanonBriefCenter, KanonResearchEvidenceCenter, KanonStrategyLibrary } from '../features/strategy/KanonStrategyCenters'
import type { StrategyTaskBundle } from '../features/strategy/types'
import { KanonSkillsOperations } from '../features/strategy/KanonSkillsOperations'
import { KanonReviewCenter } from '../features/strategy/KanonReviewCenter'
import { strategyStageLabel } from '../features/strategy/workspace/StageRail'
import type { StrategyPanel, StrategyStage, StrategyWorkspaceLocation } from '../features/strategy/workspace/workspaceRoute'
import { industryProfile } from '../data/industry-profiles'
import { OceanEngineSessionSettings } from './OceanEngineSessionSettings'

const ApprovalCenterPage = lazy(() => import('./SpecializedPages').then(module => ({ default: module.ApprovalCenterPage })))
const ArtifactFlow = lazy(() => import('./SpecializedPages').then(module => ({ default: module.ArtifactFlow })))
const DeliveryPlanPage = lazy(() => import('./SpecializedPages').then(module => ({ default: module.DeliveryPlanPage })))
const ImageTextCreationPage = lazy(() => import('./SpecializedPages').then(module => ({ default: module.ImageTextCreationPage })))
const VideoCreationPage = lazy(() => import('./SpecializedPages').then(module => ({ default: module.VideoCreationPage })))
const DeliveryMonitoringPage = lazy(() => import('./DeliveryMonitoringPage').then(module => ({ default: module.DeliveryMonitoringPage })))
const DeliveryOptimizationPage = lazy(() => import('./DeliveryOptimizationPage').then(module => ({ default: module.DeliveryOptimizationPage })))
const DeliveryConfigurationPage = lazy(() => import('./DeliveryConfigurationPage').then(module => ({ default: module.DeliveryConfigurationPage })))
const DeliveryMockEnvironmentBanner = lazy(() => import('./DeliveryTourPage').then(module => ({ default: module.DeliveryMockEnvironmentBanner })))
const ProductsPage = lazy(() => import('./ProductsPage').then(module => ({ default: module.ProductsPage })))
const ExperimentCenterPage = lazy(() => import('./ExperimentCenterPage').then(module => ({ default: module.ExperimentCenterPage })))
const PreLaunchPage = lazy(() => import('./insight/prelaunch/PreLaunchPage').then(module => ({ default: module.PreLaunchPage })))
const AnalysisPage = lazy(() => import('./insight/analysis/AnalysisPage').then(module => ({ default: module.AnalysisPage })))
const AssetsPage = lazy(() => import('./insight/assets/AssetsPage').then(module => ({ default: module.AssetsPage })))
const ReviewPage = lazy(() => import('./insight/review/ReviewPage').then(module => ({ default: module.ReviewPage })))
const ExperiencePage = lazy(() => import('./insight/experience/ExperiencePage').then(module => ({ default: module.ExperiencePage })))
const SettingsPage = lazy(() => import('./insight/settings/SettingsPage').then(module => ({ default: module.SettingsPage })))
// 米云素材（同步自上游）。上游是静态 import 的，这里跟其他洞察页一样改成 lazy：
// 这一页两千多行，静态引会整块压进主包，而它不在主线上，日常多数人不会点进来。
const MiyunMaterialsPage = lazy(() => import('./MiyunMaterialsPage').then(module => ({ default: module.MiyunMaterialsPage })))
const TaskCenterPage = lazy(() => import('./BusinessTaskPages').then(module => ({ default: module.TaskCenterPage })))
const TaskCreateDialog = lazy(() => import('./BusinessTaskPages').then(module => ({ default: module.TaskCreateDialog })))

const StrategyWorkspaceRoute = lazy(() => import('../features/strategy/workspace/StrategyWorkspaceRoute').then(module => ({
  default: module.StrategyWorkspaceRoute,
})))
const ProductionCenterPage = lazy(() => import('../features/production-center/ProductionCenterPage').then(module => ({
  default: module.ProductionCenterPage,
})))

const ControlledExecutionWorkspace = lazy(() => import('../features/browser-rpa-execution/BrowserRpaExecutionWorkspace').then(module => ({
  default: module.BrowserRpaExecutionWorkspace,
})))
const DeliveryPlatformEntitiesPage = lazy(() => import('../features/delivery-platform-entities/DeliveryPlatformEntitiesPage').then(module => ({
  default: module.DeliveryPlatformEntitiesPage,
})))

type OpenProject = (id: string, system?: SystemKey, navId?: string, objectId?: string, view?: string, contextId?: string, tourRunId?: string, tourCase?: string) => void
type OpenStrategyWorkspace = (projectId: string, workspaceId: string, location: StrategyWorkspaceLocation, replace?: boolean) => void

function creativeTaskDestination(task: BusinessTaskRecord): { navId: string; view?: string } {
  if (task.type === 'creative') return { navId: 'image-text' }
  if (task.type === 'brand_video') return { navId: 'video', view: '品牌广告' }
  if (task.type === 'video_edit') return { navId: 'video', view: '素材剪辑' }
  return { navId: 'video', view: '效果广告' }
}

const dashboardJourneys: Record<SystemKey, Array<{ label: string; detail: string; navId: string }>> = {
  strategy: [
    { label: '需求完整度', detail: '补齐目标、受众、边界和成功指标', navId: 'briefs' },
    { label: '策略任务', detail: '从 Brief 创建可追溯的策略任务', navId: 'tasks' },
    { label: '研究证据', detail: '组织受众、竞品与行业依据', navId: 'research' },
    { label: '策略评审', detail: '确认策略版本并交给创意生产', navId: 'reviews' },
  ],
  creative: [
    { label: '创意任务', detail: '继承已批准策略和渠道交付规格', navId: 'tasks' },
    { label: '图文创作', detail: '组织文案、版式、品牌和渠道检查', navId: 'image-text' },
    { label: '视频创作', detail: '品牌、效果广告与视频包装', navId: 'video' },
    { label: '生产与评审', detail: '跟踪生成、失败恢复和交付版本', navId: 'production' },
  ],
  insight: [
    { label: '分析', detail: '一轮投放跑完，为什么是这个结果', navId: 'analysis' },
    { label: '复盘', detail: '这一轮收尾，决定留下什么', navId: 'review' },
    { label: '经验', detail: '以前什么有效、在什么条件下成立', navId: 'experience' },
    { label: '素材', detail: '能拿来分析的素材有哪些、还差什么', navId: 'assets' },
  ],
  delivery: [
    { label: '投放计划', detail: '选择创意组合、预算和排期', navId: 'plans' },
    { label: '策略优化', detail: '把洞察转成受控 ChangeSet', navId: 'optimization' },
    { label: '审批中心', detail: '完成预检、审批和权限控制', navId: 'approvals' },
    { label: '执行与回滚', detail: '保留执行证据和回滚能力', navId: 'execution' },
  ],
}

function Status({ value }: { value: string }) {
  const kind = value.includes('完成') || value.includes('通过') ? 'success' : value.includes('失败') || value.includes('需处理') ? 'danger' : value.includes('生成') || value.includes('执行') ? 'info' : 'warning'
  return <span className={`status ${kind}`}><span />{value}</span>
}

const materialFilters = ['全部素材', '待质检', '未通过', '待人工确认', '已确认'] as const
type MaterialFilter = typeof materialFilters[number]

interface MaterialQueueItem {
  pointer: ApiAssetVersionPointer
  title: string
  versions: number[]
}

function MaterialCheckWorkspace({ state, activeView, objectId, onOpenProject }: { state: DataState; activeView: string; objectId?: string; onOpenProject: OpenProject }) {
  const { currentProject } = useProject()
  const [workbench, setWorkbench] = useState<ApiAgencyWorkbench | null>(null)
  const [workbenchError, setWorkbenchError] = useState(false)
  const [notice, setNotice] = useState('')
  const [changeNote, setChangeNote] = useState('')
  const filter = materialFilters.includes(activeView as MaterialFilter) ? activeView as MaterialFilter : '全部素材'
  const routeTarget = parseMaterialTarget(objectId)

  useEffect(() => {
    let active = true
    setWorkbenchError(false)
    void api.listAgencyWorkbench({ projectIds: [currentProject.id] }).then(next => {
      if (active) setWorkbench(next)
    }).catch(() => {
      if (active) {
        setWorkbench(null)
        setWorkbenchError(true)
      }
    })
    return () => { active = false }
  }, [currentProject.id])

  if (state === 'forbidden') return <MaterialCheckState title="无权限" detail="当前身份不能查看该 Project 的素材质检和人工确认记录，请联系项目负责人开通权限。" />
  if (state === 'error' || workbenchError) return <MaterialCheckState title="服务未连接" detail="素材检查服务暂不可用，队列、质检结果和确认占位无法加载。" />
  if (state === 'loading' || !workbench) return <MaterialCheckState title="正在加载素材检查" detail="正在同步素材版本指针、质检记录和人工确认记录。" />

  const allItems = buildMaterialQueue(workbench, currentProject.id)
  const filteredItems = allItems.filter(item => filter === '全部素材' || materialVersionState(item.pointer, item.pointer.workingVersion, workbench).filter === filter)
  if (state === 'empty' || allItems.length === 0) return <MaterialCheckState title="暂无待检查" detail="当前 Project 还没有素材版本指针。完成制作或生成新版本后，会在这里进入素材检查队列。" />
  if (filteredItems.length === 0) return <MaterialCheckState title="筛选无结果" detail={`当前 Project 没有“${filter}”状态的素材版本，可切换到“全部素材”查看完整队列。`} />

  const selectedItem = allItems.find(item => item.pointer.assetId === routeTarget.assetId) ?? filteredItems[0]
  const selectedVersion = routeTarget.assetId === selectedItem.pointer.assetId && routeTarget.version && selectedItem.versions.includes(routeTarget.version)
    ? routeTarget.version
    : selectedItem.pointer.workingVersion
  const selectedState = materialVersionState(selectedItem.pointer, selectedVersion, workbench)
  const selectedVersionRecord = selectedItem.pointer.versions.find(version => version.version === selectedVersion)
  const selectedHistory = materialVersionHistory(selectedItem.pointer, selectedVersion, workbench)
  const authorizationGate = materialAuthorizationGate(selectedItem.pointer)
  const openMaterial = (item: MaterialQueueItem, version = item.pointer.workingVersion) => {
    onOpenProject(currentProject.id, 'creative', 'reviews', materialTarget(item.pointer.assetId, version), filter)
  }
  const hasCompletedQualityRun = Boolean(selectedState.qualityRun?.completedAt)
  const canConfirmMaterial = selectedState.qualityRun?.status === 'passed' && hasCompletedQualityRun
  const canSetDeliveryVersion = selectedState.confirmation?.status === 'confirmed' && authorizationGate.allowed
  const upsertWorkbenchForSelected = (updater: (current: ApiAgencyWorkbench) => ApiAgencyWorkbench) => {
    setWorkbench(current => current ? updater(current) : current)
  }
  const handleRunQualityCheck = () => {
    const now = new Date().toISOString()
    const run: ApiQualityCheckRun = {
      id: `qc-${selectedItem.pointer.assetId}-v${selectedVersion}-${Date.now()}`,
      organizationId: selectedItem.pointer.organizationId,
      projectId: currentProject.id,
      assetId: selectedItem.pointer.assetId,
      assetVersion: selectedVersion,
      status: 'passed',
      model: 'demo-quality-vision-v1',
      ruleVersion: 'agency-material-rules-2026-07',
      promptVersion: 'material-check-2026-07-27',
      summary: '大模型已完成品牌、权益、画面安全和渠道规格检查，未发现阻断问题。',
      issues: [],
      createdAt: now,
      completedAt: now,
    }
    upsertWorkbenchForSelected(current => ({
      ...current,
      qualityCheckRuns: [run, ...current.qualityCheckRuns],
      assetVersionPointers: current.assetVersionPointers.map(pointer => pointer.id === selectedItem.pointer.id ? {
        ...pointer,
        qualityCheckedVersion: selectedVersion,
        updatedAt: now,
      } : pointer),
    }))
    setNotice(`已完成 v${selectedVersion} 大模型质检，可进入人工确认。`)
  }
  const handleConfirmMaterial = () => {
    const qualityRun = selectedState.qualityRun
    if (!qualityRun?.completedAt) {
      setNotice('人工确认前必须先完成当前素材版本的大模型质检。')
      return
    }
    if (qualityRun.status !== 'passed') {
      setNotice('当前质检未通过，不能确认素材；请先选择“需要修改”。')
      return
    }
    const now = new Date().toISOString()
    const confirmation: ApiMaterialConfirmation = {
      id: `confirm-${selectedItem.pointer.assetId}-v${selectedVersion}-${Date.now()}`,
      organizationId: selectedItem.pointer.organizationId,
      projectId: currentProject.id,
      qualityCheckRunId: qualityRun.id,
      assetId: selectedItem.pointer.assetId,
      assetVersion: selectedVersion,
      status: 'confirmed',
      scope: `${currentProject.name} / ${materialTitle(selectedItem.pointer.assetId)}`,
      confirmedBy: currentProject.owner,
      note: '已确认当前版本可进入投放计划和交付预检。',
      createdAt: now,
    }
    upsertWorkbenchForSelected(current => ({
      ...current,
      materialConfirmations: [confirmation, ...current.materialConfirmations],
      assetVersionPointers: current.assetVersionPointers.map(pointer => pointer.id === selectedItem.pointer.id ? {
        ...pointer,
        qualityCheckedVersion: selectedVersion,
        humanConfirmedVersion: selectedVersion,
        updatedAt: now,
      } : pointer),
    }))
    setNotice(`已写入 v${selectedVersion} 人工确认，确认记录绑定质检 ${qualityRun.id}。`)
  }
  const handleRequestChanges = () => {
    const qualityRun = selectedState.qualityRun
    if (!qualityRun?.completedAt) {
      setNotice('需要修改前必须先完成当前素材版本的大模型质检。')
      return
    }
    const note = changeNote.trim()
    if (!note) {
      setNotice('需要修改至少要填写一条问题说明。')
      return
    }
    const now = new Date().toISOString()
    const confirmation: ApiMaterialConfirmation = {
      id: `changes-${selectedItem.pointer.assetId}-v${selectedVersion}-${Date.now()}`,
      organizationId: selectedItem.pointer.organizationId,
      projectId: currentProject.id,
      qualityCheckRunId: qualityRun.id,
      assetId: selectedItem.pointer.assetId,
      assetVersion: selectedVersion,
      status: 'changes_requested',
      scope: `${currentProject.name} / ${materialTitle(selectedItem.pointer.assetId)}`,
      confirmedBy: currentProject.owner,
        note,
      createdAt: now,
    }
    upsertWorkbenchForSelected(current => ({
      ...current,
      materialConfirmations: [confirmation, ...current.materialConfirmations],
      assetVersionPointers: current.assetVersionPointers.map(pointer => pointer.id === selectedItem.pointer.id ? {
        ...pointer,
        humanConfirmedVersion: pointer.humanConfirmedVersion === selectedVersion ? undefined : pointer.humanConfirmedVersion,
        updatedAt: now,
      } : pointer),
    }))
    setChangeNote('')
    setNotice(`已将 v${selectedVersion} 标记为需要修改，并返回制作环节。`)
  }
  const handleCreateNewVersion = () => {
    const nextVersion = selectedItem.pointer.workingVersion + 1
    const now = new Date().toISOString()
    upsertWorkbenchForSelected(current => ({
      ...current,
      assetVersionPointers: current.assetVersionPointers.map(pointer => pointer.id === selectedItem.pointer.id ? {
        ...pointer,
        workingVersion: nextVersion,
        versions: [{
          version: nextVersion,
          createdBy: currentProject.owner,
          sourceTaskId: `manual-${selectedItem.pointer.assetId}-v${nextVersion}`,
          sourceType: 'manual_edit',
          sourceLabel: '人工新增版本',
          createdAt: now,
          changeSummary: '在旧版本基础上新增修订版本；历史版本保持不可覆盖。',
        }, ...pointer.versions],
        updatedAt: now,
      } : pointer),
    }))
    setNotice(`已生成 v${nextVersion}，旧质检和确认记录保留，新版本回到待质检流程。`)
    onOpenProject(currentProject.id, 'creative', 'reviews', materialTarget(selectedItem.pointer.assetId, nextVersion), filter)
  }
  const handleSetDeliveryVersion = () => {
    if (selectedState.confirmation?.status !== 'confirmed') {
      setNotice('只有已人工确认的素材版本才能进入交付版本。')
      return
    }
    if (!authorizationGate.allowed) {
      setNotice(`授权门禁阻止交付：${authorizationGate.reason}`)
      return
    }
    const now = new Date().toISOString()
    upsertWorkbenchForSelected(current => ({
      ...current,
      assetVersionPointers: current.assetVersionPointers.map(pointer => pointer.id === selectedItem.pointer.id ? {
        ...pointer,
        deliveryVersion: selectedVersion,
        updatedAt: now,
      } : pointer),
    }))
    setNotice(`已将 v${selectedVersion} 设为交付版本，授权覆盖 ${selectedItem.pointer.deliveryTarget.platform} / ${selectedItem.pointer.deliveryTarget.region}。`)
  }

  return <div className="material-check-workspace">
    <aside className="material-queue-panel" aria-label="素材检查队列">
      <div className="surface-toolbar"><h3>检查队列</h3><span>{filteredItems.length} / {allItems.length}</span></div>
      <div className="material-filter-strip">{materialFilters.map(option => <button key={option} className={filter === option ? 'active' : ''} onClick={() => onOpenProject(currentProject.id, 'creative', 'reviews', objectId, option)}>{option}</button>)}</div>
      {filteredItems.map(item => {
        const rowState = materialVersionState(item.pointer, item.pointer.workingVersion, workbench)
        return <button key={item.pointer.assetId} className={item.pointer.assetId === selectedItem.pointer.assetId ? 'material-queue-row active' : 'material-queue-row'} onClick={() => openMaterial(item)}>
          <span className="material-thumb" aria-hidden="true">{item.title.slice(0, 2).toUpperCase()}</span>
          <span><b>{item.title}</b><small>{item.pointer.assetId}</small></span>
          <strong className={`material-pill ${rowState.tone}`}>v{item.pointer.workingVersion} · {rowState.label}</strong>
          <small>{item.pointer.owner} · {new Date(item.pointer.updatedAt).toLocaleString('zh-CN', { hour12: false })}</small>
        </button>
      })}
    </aside>
    <section className="material-preview-panel" aria-label="素材预览和版本">
      <div className="surface-toolbar">
        <h3>{selectedItem.title}</h3>
        <span className="material-preview-actions">
          <button onClick={handleCreateNewVersion}><Plus size={14}/>生成新版本</button>
          <button onClick={() => openMaterial(selectedItem, selectedVersion)}><ExternalLink size={14}/>打开深链</button>
        </span>
      </div>
      <div className="material-preview-frame">
        {selectedItem.pointer.contentUrl && selectedItem.pointer.mediaKind === 'video'
          ? <video key={`${selectedItem.pointer.assetId}-v${selectedVersion}`} controls playsInline preload="metadata" src={selectedItem.pointer.contentUrl} aria-label={`${selectedItem.title}素材检查预览`}/>
          : selectedItem.pointer.contentUrl && selectedItem.pointer.mediaKind === 'image'
            ? <img src={selectedItem.pointer.contentUrl} alt={`${selectedItem.title}素材检查预览`}/>
            : <div className="material-preview-card">
              <span>ASSET</span>
              <b>{selectedItem.pointer.assetId}</b>
              <small>当前预览版本 v{selectedVersion}</small>
            </div>}
      </div>
      <div className="material-version-strip" aria-label="素材版本">
        {selectedItem.versions.map(version => {
          const versionState = materialVersionState(selectedItem.pointer, version, workbench)
          return <button key={version} className={version === selectedVersion ? 'active' : ''} onClick={() => openMaterial(selectedItem, version)}>
            <b>v{version}</b>
            <small>{versionState.label}</small>
          </button>
        })}
      </div>
      <VersionProvenanceCard version={selectedVersionRecord} selectedVersion={selectedVersion} />
      <div className="material-version-ledger">
        <span><b>workingVersion</b>{`v${selectedItem.pointer.workingVersion}`}</span>
        <span><b>qualityCheckedVersion</b>{selectedItem.pointer.qualityCheckedVersion ? `v${selectedItem.pointer.qualityCheckedVersion}` : '未产生'}</span>
        <span><b>humanConfirmedVersion</b>{selectedItem.pointer.humanConfirmedVersion ? `v${selectedItem.pointer.humanConfirmedVersion}` : '未确认'}</span>
        <span><b>deliveryVersion</b>{selectedItem.pointer.deliveryVersion ? `v${selectedItem.pointer.deliveryVersion}` : '未交付'}</span>
      </div>
    </section>
    <aside className="material-inspector-panel" aria-label="质检结果与人工确认">
      <div className="surface-toolbar"><h3>质检 / 确认</h3><span className={`material-pill ${selectedState.tone}`}>{selectedState.label}</span></div>
      <QualityRunCard run={selectedState.qualityRun} />
      <ConfirmationCard confirmation={selectedState.confirmation} />
      <AuthorizationGateCard pointer={selectedItem.pointer} gate={authorizationGate} />
      <MaterialHistoryCard items={selectedHistory} />
      <div className="material-action-stack">
        <button className="secondary-button full" onClick={handleRunQualityCheck}><ShieldCheck size={15}/>运行大模型质检</button>
        <button className="primary-button full" disabled={!canConfirmMaterial} onClick={handleConfirmMaterial}><Check size={15}/>确认素材</button>
        <label className="material-change-note">修改问题说明<textarea value={changeNote} onChange={event => setChangeNote(event.target.value)} placeholder="例如：画面中的 CTA 与新版 Brief 不一致"/></label>
        <button className="secondary-button full" disabled={!hasCompletedQualityRun} onClick={handleRequestChanges}><CircleAlert size={15}/>需要修改</button>
        <button className="primary-button full" disabled={!canSetDeliveryVersion} onClick={handleSetDeliveryVersion}><ArrowRight size={15}/>设为交付版本</button>
      </div>
      {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
    </aside>
  </div>
}

function QualityRunCard({ run }: { run?: ApiQualityCheckRun }) {
  if (!run) return <div className="material-empty-card"><Clock3 size={16}/><b>暂无大模型质检</b><small>当前版本还没有完成的 QualityCheckRun。</small></div>
  return <div className="material-check-card">
    <span>大模型质检</span>
    <h4>{qualityStatusLabel(run.status)}</h4>
    <p>{run.summary}</p>
    <dl><div><dt>模型</dt><dd>{run.model}</dd></div><div><dt>规则</dt><dd>{run.ruleVersion}</dd></div><div><dt>Prompt</dt><dd>{run.promptVersion}</dd></div><div><dt>完成时间</dt><dd>{run.completedAt ?? '未完成'}</dd></div></dl>
    <div className="material-issue-list">{run.issues.length ? run.issues.map(issue => <article key={issue.id}><strong>{severityLabel(issue.severity)} · {issue.rule}</strong><small>证据位置：{issue.evidence}</small><small>修复建议：{issue.suggestion}</small></article>) : <article><CircleCheck size={14}/><strong>未发现阻断问题</strong><small>当前版本可进入人工确认。</small></article>}</div>
  </div>
}

function ConfirmationCard({ confirmation }: { confirmation?: ApiMaterialConfirmation }) {
  if (!confirmation) return <div className="material-empty-card"><ClipboardCheck size={16}/><b>暂无人工确认</b><small>当前版本尚未写入 MaterialConfirmation，需先完成大模型质检。</small></div>
  return <div className="material-check-card">
    <span>人工确认</span>
    <h4>{confirmation.status === 'confirmed' ? '已确认素材' : '需要修改'}</h4>
    <p>{confirmation.note}</p>
    <dl><div><dt>确认人</dt><dd>{confirmation.confirmedBy}</dd></div><div><dt>范围</dt><dd>{confirmation.scope}</dd></div><div><dt>时间</dt><dd>{confirmation.createdAt}</dd></div></dl>
  </div>
}

function VersionProvenanceCard({ version, selectedVersion }: { version?: ApiAssetVersionPointer['versions'][number]; selectedVersion: number }) {
  if (!version) return <div className="material-provenance-card"><b>v{selectedVersion}</b><span>该版本缺少创建元数据，请补录来源任务、创建人和修改说明。</span></div>
  return <div className="material-provenance-card">
    <b>v{version.version} 不可覆盖版本</b>
    <span>{version.sourceType === 'model_generation' ? '模型生成' : '人工编辑'} · {version.sourceLabel}</span>
    <dl>
      <div><dt>创建人</dt><dd>{version.createdBy}</dd></div>
      <div><dt>来源任务</dt><dd>{version.sourceTaskId}</dd></div>
      <div><dt>创建时间</dt><dd>{new Date(version.createdAt).toLocaleString('zh-CN', { hour12: false })}</dd></div>
      <div><dt>修改说明</dt><dd>{version.changeSummary}</dd></div>
    </dl>
  </div>
}

function AuthorizationGateCard({ pointer, gate }: { pointer: ApiAssetVersionPointer; gate: { allowed: boolean; reason: string } }) {
  return <div className={gate.allowed ? 'material-gate-card allowed' : 'material-gate-card blocked'}>
    <span>授权门禁</span>
    <h4>{gate.allowed ? '授权覆盖交付目标' : '禁止进入交付版本'}</h4>
    <p>{gate.reason}</p>
    <dl>
      <div><dt>目标平台</dt><dd>{pointer.deliveryTarget.platform}</dd></div>
      <div><dt>目标地区</dt><dd>{pointer.deliveryTarget.region}</dd></div>
      <div><dt>授权平台</dt><dd>{pointer.authorization.platforms.join(' / ')}</dd></div>
      <div><dt>授权地区</dt><dd>{pointer.authorization.regions.join(' / ')}</dd></div>
      <div><dt>权利方</dt><dd>{pointer.authorization.rightsHolder}</dd></div>
      <div><dt>到期时间</dt><dd>{new Date(pointer.authorization.expiresAt).toLocaleDateString('zh-CN')}</dd></div>
    </dl>
    <small>{pointer.authorization.note}</small>
  </div>
}

function MaterialHistoryCard({ items }: { items: Array<{ id: string; label: string; detail: string; occurredAt: string; tone: 'success' | 'warning' | 'danger' | 'info' }> }) {
  return <div className="material-history-card">
    <span>版本历史</span>
    {items.map(item => <article key={item.id}>
      <i className={`material-history-dot ${item.tone}`}/>
      <b>{item.label}</b>
      <small>{item.detail}</small>
      <time>{new Date(item.occurredAt).toLocaleString('zh-CN', { hour12: false })}</time>
    </article>)}
    {!items.length ? <small>暂无质检、确认、返修或被新版替代历史。</small> : null}
  </div>
}

function MaterialCheckState({ title, detail }: { title: string; detail: string }) {
  return <div className="material-state"><Filter size={18}/><b>{title}</b><p>{detail}</p></div>
}

function buildMaterialQueue(workbench: ApiAgencyWorkbench, projectId: string): MaterialQueueItem[] {
  return workbench.assetVersionPointers
    .filter(pointer => pointer.projectId === projectId)
    .map(pointer => {
      const versions = new Set<number>([pointer.workingVersion])
      if (pointer.qualityCheckedVersion) versions.add(pointer.qualityCheckedVersion)
      if (pointer.humanConfirmedVersion) versions.add(pointer.humanConfirmedVersion)
      if (pointer.deliveryVersion) versions.add(pointer.deliveryVersion)
      pointer.versions.forEach(version => versions.add(version.version))
      workbench.qualityCheckRuns.filter(run => run.assetId === pointer.assetId).forEach(run => versions.add(run.assetVersion))
      workbench.materialConfirmations.filter(confirmation => confirmation.assetId === pointer.assetId).forEach(confirmation => versions.add(confirmation.assetVersion))
      return { pointer, title: materialTitle(pointer.assetId), versions: Array.from(versions).sort((left, right) => right - left) }
    })
}

function materialAuthorizationGate(pointer: ApiAssetVersionPointer): { allowed: boolean; reason: string } {
  const platformCovered = pointer.authorization.platforms.includes(pointer.deliveryTarget.platform)
  const regionCovered = pointer.authorization.regions.includes(pointer.deliveryTarget.region)
  if (!platformCovered && !regionCovered) return { allowed: false, reason: `授权未覆盖 ${pointer.deliveryTarget.platform} 和 ${pointer.deliveryTarget.region}。` }
  if (!platformCovered) return { allowed: false, reason: `授权未覆盖目标平台 ${pointer.deliveryTarget.platform}。` }
  if (!regionCovered) return { allowed: false, reason: `授权未覆盖目标地区 ${pointer.deliveryTarget.region}。` }
  return { allowed: true, reason: `授权覆盖 ${pointer.deliveryTarget.platform} / ${pointer.deliveryTarget.region}，可进入交付版本。` }
}

function materialVersionHistory(pointer: ApiAssetVersionPointer, version: number, workbench: ApiAgencyWorkbench): Array<{ id: string; label: string; detail: string; occurredAt: string; tone: 'success' | 'warning' | 'danger' | 'info' }> {
  const versionRecord = pointer.versions.find(item => item.version === version)
  const qualityRuns = workbench.qualityCheckRuns.filter(run => run.assetId === pointer.assetId && run.assetVersion === version).map(run => ({
    id: run.id,
    label: `大模型质检：${qualityStatusLabel(run.status)}`,
    detail: run.summary,
    occurredAt: run.completedAt ?? run.createdAt,
    tone: run.status === 'passed' ? 'success' as const : run.status === 'failed' ? 'danger' as const : 'warning' as const,
  }))
  const confirmations = workbench.materialConfirmations.filter(item => item.assetId === pointer.assetId && item.assetVersion === version).map(item => ({
    id: item.id,
    label: item.status === 'confirmed' ? '人工确认通过' : '要求修改',
    detail: item.note,
    occurredAt: item.createdAt,
    tone: item.status === 'confirmed' ? 'success' as const : 'danger' as const,
  }))
  const replacement = pointer.workingVersion > version ? [{
    id: `${pointer.id}-replaced-v${version}`,
    label: '已被新版替代',
    detail: `当前 workingVersion 已推进至 v${pointer.workingVersion}，v${version} 仅保留历史和授权追溯。`,
    occurredAt: pointer.updatedAt,
    tone: 'info' as const,
  }] : []
  const created = versionRecord ? [{
    id: `${pointer.id}-created-v${version}`,
    label: '版本创建',
    detail: `${versionRecord.sourceLabel}：${versionRecord.changeSummary}`,
    occurredAt: versionRecord.createdAt,
    tone: 'info' as const,
  }] : []
  return [...created, ...qualityRuns, ...confirmations, ...replacement].sort((left, right) => right.occurredAt.localeCompare(left.occurredAt))
}

function materialVersionState(pointer: ApiAssetVersionPointer, version: number, workbench: ApiAgencyWorkbench): { filter: MaterialFilter; label: string; tone: 'success' | 'warning' | 'danger' | 'info'; qualityRun?: ApiQualityCheckRun; confirmation?: ApiMaterialConfirmation } {
  const qualityRun = workbench.qualityCheckRuns.filter(run => run.assetId === pointer.assetId && run.assetVersion === version).sort((left, right) => right.createdAt.localeCompare(left.createdAt))[0]
  const confirmation = workbench.materialConfirmations.filter(item => item.assetId === pointer.assetId && item.assetVersion === version).sort((left, right) => right.createdAt.localeCompare(left.createdAt))[0]
  if (confirmation?.status === 'confirmed') return { filter: '已确认', label: '已确认', tone: 'success', qualityRun, confirmation }
  if (confirmation?.status === 'changes_requested') return { filter: '未通过', label: '需要修改', tone: 'danger', qualityRun, confirmation }
  if (!qualityRun) return { filter: '待质检', label: version === pointer.workingVersion ? '待质检' : '无质检记录', tone: 'warning', qualityRun, confirmation }
  if (qualityRun.status === 'failed') return { filter: '未通过', label: '质检未通过', tone: 'danger', qualityRun, confirmation }
  if (qualityRun.status === 'passed') return { filter: '待人工确认', label: '待人工确认', tone: 'info', qualityRun, confirmation }
  return { filter: '待质检', label: qualityStatusLabel(qualityRun.status), tone: 'warning', qualityRun, confirmation }
}

function parseMaterialTarget(objectId?: string): { assetId?: string; version?: number } {
  const match = objectId?.match(/^(.+)@v(\d+)$/)
  return match ? { assetId: match[1], version: Number(match[2]) } : { assetId: objectId }
}

function materialTarget(assetId: string, version: number) {
  return `${assetId}@v${version}`
}

function materialTitle(assetId: string) {
  return assetId.replace(/^asset-/, '').split('-').map(part => part.charAt(0).toUpperCase() + part.slice(1)).join(' ')
}

function qualityStatusLabel(status: ApiQualityCheckRun['status']) {
  const labels: Record<ApiQualityCheckRun['status'], string> = { queued: '排队中', running: '质检中', passed: '质检通过', failed: '质检未通过' }
  return labels[status]
}

function severityLabel(severity: ApiQualityCheckRun['issues'][number]['severity']) {
  const labels: Record<ApiQualityCheckRun['issues'][number]['severity'], string> = { minor: '轻微', major: '重要', critical: '严重' }
  return labels[severity]
}

function operationRecords(records: ApiOperationalRecord[], kind: ApiOperationalRecordKind) {
  return records.filter(record => record.kind === kind)
}

function operationField(record: ApiOperationalRecord, key: string): string {
  const value = record.fields[key]
  return value === undefined ? '—' : String(value)
}

function bindingStatusScore(status: ApiBindingHealthStatus): number {
  return status === 'expired' ? 2 : status === 'warning' ? 1 : 0
}

function bindingStatusLabel(status: ApiBindingHealthStatus): string {
  return status === 'expired' ? '已失效' : status === 'warning' ? '需复核' : '正常'
}

function BindingStatus({ value }: { value: ApiBindingHealthStatus }) {
  const kind = value === 'expired' ? 'danger' : value === 'warning' ? 'warning' : 'success'
  return <span className={`status ${kind}`}><span />{bindingStatusLabel(value)}</span>
}

function bindingAttentionSummary(binding: ApiAdAccountBinding): string {
  const items = [
    binding.permissionStatus !== 'normal' ? `权限${bindingStatusLabel(binding.permissionStatus)}` : '',
    binding.loginStatus !== 'normal' ? `登录${bindingStatusLabel(binding.loginStatus)}` : '',
    binding.trackingStatus !== 'normal' ? `追踪${bindingStatusLabel(binding.trackingStatus)}` : '',
  ].filter(Boolean)
  return items.length ? items.join(' / ') : '账户健康'
}

function projectNextStep(project: ProjectRecord): { label: string; detail: string; system: SystemKey; navId: string; blocker: string } {
  const hasConfirmedBrief = project.artifacts.brief.status === '已确认'
  const failedTask = project.tasks.find(task => task.status === 'failed')
  const pendingChange = project.changeSets.find(change => ['草稿', '待审批'].includes(change.status))
  const hasReadyCreative = project.artifacts.creative.status === '已完成'

  if (failedTask) {
    return { label: '处理失败任务', detail: failedTask.name, system: 'creative', navId: 'tasks', blocker: '存在失败任务，需先恢复后再推进。' }
  }
  if (!hasConfirmedBrief) {
    return { label: '确认策略 Brief', detail: '明确目标、受众与创意边界', system: 'strategy', navId: 'workspaces', blocker: '策略 Brief 尚未确认。' }
  }
  if (!hasReadyCreative) {
    return { label: '进入创意生产', detail: '基于已确认 Brief 生成并评审素材', system: 'creative', navId: 'tasks', blocker: '缺少可用于投放的已完成创意。' }
  }
  if (pendingChange) {
    return { label: '处理 ChangeSet', detail: pendingChange.title, system: 'delivery', navId: 'approvals', blocker: `ChangeSet ${pendingChange.id} 等待受控处理。` }
  }
  return { label: '查看项目进展', detail: '复核当前阶段与跨模块工作', system: 'strategy', navId: 'workspaces', blocker: '当前没有阻塞项。' }
}

type PortfolioRecord = {
  id: string
  title: string
  client: string
  brand: string
  project: string
  projectId: string
  object: string
  priority: '高' | '中' | '低'
  owner: string
  dueAt: string
  detail: string
  system: SystemKey
  navId: string
  tone: 'danger' | 'warning' | 'info' | 'success'
}

function priorityTone(priority: PortfolioRecord['priority']) {
  return priority === '高' ? 'danger' : priority === '中' ? 'warning' : 'info'
}

function healthText(status: string) {
  if (status === 'blocked') return '阻塞'
  if (status === 'watch') return '观察'
  return '健康'
}

export function HomePage({ onSystemChange, onOpenProject, onManageProject }: { onSystemChange: (key: SystemKey) => void; onOpenProject: (id: string, system?: SystemKey, navId?: string, objectId?: string) => void; onManageProject: (id: string) => void }) {
  const { error: projectError, isLoading } = useProject()
  const [workbench, setWorkbench] = useState<ApiAgencyWorkbench | null>(null)
  const [workbenchError, setWorkbenchError] = useState('')

  useEffect(() => {
    let active = true
    void api.listAgencyWorkbench({ includeDemoProject: true }).then(data => {
      if (active) setWorkbench(data)
    }).catch(cause => {
      if (active) setWorkbenchError(cause instanceof Error ? cause.message : '加载代理商工作台失败')
    })
    return () => { active = false }
  }, [])

  const portfolio = useMemo(() => {
    const empty = { metrics: [], today: [], checks: [], deliveries: [], health: [], load: [], recent: [] } as {
      metrics: Array<{ label: string; value: number; detail: string; tone: PortfolioRecord['tone'] }>
      today: PortfolioRecord[]
      checks: PortfolioRecord[]
      deliveries: PortfolioRecord[]
      health: Array<{ id: string; name: string; owner: string; status: string; projects: number; issues: number }>
      load: Array<{ owner: string; count: number; detail: string }>
      recent: ApiAgencyWorkbench['projects']
    }
    if (!workbench) return empty

    const clients = new Map(workbench.clients.map(client => [client.id, client]))
    const brands = new Map(workbench.brands.map(brand => [brand.id, brand]))
    const projects = new Map(workbench.projects.map(project => [project.id, project]))
    const confirmedAssets = new Set(workbench.materialConfirmations.filter(item => item.status === 'confirmed').map(item => `${item.projectId}:${item.assetId}:${item.assetVersion}`))
    const ownerLoad = new Map<string, number>()
    const addOwnerLoad = (owner: string) => ownerLoad.set(owner, (ownerLoad.get(owner) ?? 0) + 1)
    const context = (projectId: string, fallbackBrandId?: string, fallbackClientId?: string) => {
      const project = projects.get(projectId)
      const brand = brands.get(project?.brandId ?? fallbackBrandId ?? '')
      const client = clients.get(project?.clientId ?? brand?.clientId ?? fallbackClientId ?? '')
      return {
        client: client?.name ?? '未关联客户',
        brand: brand?.name ?? project?.brand ?? '未关联品牌',
        project: project?.name ?? '未关联 Project',
        owner: project?.runtime.owner ?? brand?.owner ?? client?.owner ?? '未分配',
      }
    }
    const makeRecord = (record: Omit<PortfolioRecord, 'tone'> & { tone?: PortfolioRecord['tone'] }): PortfolioRecord => ({
      ...record,
      tone: record.tone ?? priorityTone(record.priority),
    })

    workbench.projects.forEach(project => addOwnerLoad(project.runtime.owner))
    workbench.assetVersionPointers.forEach(pointer => addOwnerLoad(pointer.owner))
    workbench.adAccountBindings.forEach(binding => addOwnerLoad(binding.owner))

    const accountIssues = workbench.adAccountBindings
      .filter(binding => [binding.permissionStatus, binding.loginStatus, binding.trackingStatus].some(status => status !== 'normal'))
      .map(binding => {
        const projectId = binding.projectIds[0] ?? ''
        const ctx = context(projectId, binding.brandId, binding.clientId)
        return makeRecord({
          id: `account-${binding.id}`,
          title: '账户异常需检查',
          ...ctx,
          projectId,
          object: `${binding.platform} · ${binding.accountName}`,
          priority: [binding.permissionStatus, binding.loginStatus, binding.trackingStatus].includes('expired') ? '高' : '中',
          owner: binding.owner,
          dueAt: '今日 12:00',
          detail: bindingAttentionSummary(binding),
          system: 'delivery',
          navId: 'plans',
        })
      })

    const qualityFailures = workbench.qualityCheckRuns
      .filter(run => run.status === 'failed')
      .map(run => {
        const ctx = context(run.projectId)
        return makeRecord({
          id: `qc-${run.id}`,
          title: '质检问题待处理',
          ...ctx,
          projectId: run.projectId,
          object: `${run.assetId} v${run.assetVersion}`,
          priority: run.issues.some(issue => issue.severity === 'critical') ? '高' : '中',
          owner: ctx.owner,
          dueAt: '今日 15:00',
          detail: run.summary,
          system: 'creative',
          navId: 'tasks',
        })
      })

    const confirmationIssues = workbench.materialConfirmations
      .filter(item => item.status === 'changes_requested')
      .map(item => {
        const ctx = context(item.projectId)
        return makeRecord({
          id: `confirm-${item.id}`,
          title: '返修要求待跟进',
          ...ctx,
          projectId: item.projectId,
          object: `${item.assetId} v${item.assetVersion}`,
          priority: '高',
          owner: item.confirmedBy,
          dueAt: '今日 18:00',
          detail: item.note,
          system: 'creative',
          navId: 'tasks',
        })
      })

    const riskProjects = workbench.projects
      .filter(project => project.progressDetail.riskStatus !== 'healthy')
      .map(project => {
        const ctx = context(project.id)
        return makeRecord({
          id: `risk-${project.id}`,
          title: project.progressDetail.riskStatus === 'blocked' ? '阻塞 Project 待查看' : '风险 Project 待观察',
          ...ctx,
          projectId: project.id,
          object: project.progressDetail.stageLabel,
          priority: project.progressDetail.riskStatus === 'blocked' ? '高' : '中',
          owner: project.runtime.owner,
          dueAt: '今日内',
          detail: project.progressDetail.blocker ?? project.objective,
          system: project.progressDetail.stage === 'delivery' ? 'delivery' : 'creative',
          navId: project.progressDetail.stage === 'delivery' ? 'plans' : 'tasks',
        })
      })

    const passedChecks = workbench.qualityCheckRuns
      .filter(run => run.status === 'passed' && !confirmedAssets.has(`${run.projectId}:${run.assetId}:${run.assetVersion}`))
      .map(run => {
        const ctx = context(run.projectId)
        return makeRecord({
          id: `human-${run.id}`,
          title: '待人工检查',
          ...ctx,
          projectId: run.projectId,
          object: `${run.assetId} v${run.assetVersion}`,
          priority: '中',
          owner: ctx.owner,
          dueAt: '今日 17:00',
          detail: run.summary,
          system: 'creative',
          navId: 'tasks',
          tone: 'info',
        })
      })

    const uncheckedPointers = workbench.assetVersionPointers
      .filter(pointer => pointer.workingVersion > (pointer.qualityCheckedVersion ?? 0))
      .map(pointer => {
        const ctx = context(pointer.projectId)
        return makeRecord({
          id: `pointer-${pointer.id}`,
          title: '新版本待质检',
          ...ctx,
          projectId: pointer.projectId,
          object: `${pointer.assetId} v${pointer.workingVersion}`,
          priority: '中',
          owner: pointer.owner,
          dueAt: '明日 10:00',
          detail: `当前质检版本 v${pointer.qualityCheckedVersion ?? 0}，需下钻查看新版本状态。`,
          system: 'creative',
          navId: 'tasks',
          tone: 'warning',
        })
      })

    const deliveries = workbench.projects
      .filter(project => project.runtime.status === 'active')
      .sort((left, right) => right.runtime.progress - left.runtime.progress)
      .map((project, index) => {
        const ctx = context(project.id)
        return makeRecord({
          id: `delivery-${project.id}`,
          title: index === 0 ? '48 小时内交付' : '本周交付节点',
          ...ctx,
          projectId: project.id,
          object: project.runtime.stage,
          priority: project.progressDetail.riskStatus === 'blocked' ? '高' : '中',
          owner: project.runtime.owner,
          dueAt: index === 0 ? '明日 18:00' : `7/${29 + index} 18:00`,
          detail: `${project.objective} 当前 ${project.runtime.progress}%`,
          system: project.progressDetail.stage === 'delivery' ? 'delivery' : 'creative',
          navId: project.progressDetail.stage === 'delivery' ? 'plans' : 'tasks',
          tone: project.progressDetail.riskStatus === 'blocked' ? 'danger' : 'warning',
        })
      })

    const health = workbench.clients.map(client => {
      const clientProjects = workbench.projects.filter(project => project.clientId === client.id)
      const issues = clientProjects.filter(project => project.progressDetail.riskStatus !== 'healthy').length
        + workbench.adAccountBindings.filter(binding => binding.clientId === client.id && [binding.permissionStatus, binding.loginStatus, binding.trackingStatus].some(status => status !== 'normal')).length
      return { id: client.id, name: client.name, owner: client.owner, status: healthText(client.healthStatus), projects: clientProjects.length, issues }
    })

    const load = [...ownerLoad.entries()]
      .sort((left, right) => right[1] - left[1])
      .map(([owner, count]) => ({ owner, count, detail: count >= 4 ? '负载偏高' : count >= 2 ? '正常承接' : '可承接' }))

    const today = [...accountIssues, ...qualityFailures, ...confirmationIssues, ...riskProjects].slice(0, 6)
    const checks = [...passedChecks, ...uncheckedPointers].slice(0, 5)
    return {
      metrics: [
        { label: '今日待处理', value: today.length, detail: '只读下钻队列', tone: 'danger' },
        { label: '待人工检查', value: checks.length, detail: '质检通过或新版本', tone: 'info' },
        { label: '临期交付', value: deliveries.length, detail: '48 小时/本周节点', tone: 'warning' },
        { label: '账户异常', value: accountIssues.length, detail: '权限、登录或追踪', tone: accountIssues.length ? 'danger' : 'success' },
      ],
      today,
      checks,
      deliveries,
      health,
      load,
      recent: [...workbench.projects].sort((left, right) => right.updatedAt.localeCompare(left.updatedAt)).slice(0, 4),
    }
  }, [workbench])

  const openRecord = (record: PortfolioRecord) => onOpenProject(record.projectId, record.system, record.navId, record.id)

  return <div className="home-page agency-home">
    <section className="home-hero agency-hero">
      <div><span className="section-label">AGENCY PORTFOLIO</span><h1>代理商客户组合工作台</h1><p>聚合跨客户待处理、待检查、临期交付、客户健康与团队负载；Home 只做下钻导航，不直接生成、确认或投放。</p></div>
      <button className="secondary-button" onClick={() => onSystemChange('creative')}>进入创意队列<ArrowRight size={15}/></button>
    </section>
    {projectError ? <div className="page-notice warning" role="status"><CircleAlert size={16}/>{projectError}。Home 暂时不能读取 Project 服务端数据，请确认本地 API 已启动后刷新。</div> : null}
    {workbenchError ? <div className="page-notice warning" role="status"><CircleAlert size={16}/>{workbenchError}。代理商组合队列暂不可用，请稍后重试或直接进入当前 Project。</div> : null}
    <section className="agency-metrics" aria-label="代理商组合指标">
      {portfolio.metrics.map(metric => <button key={metric.label} className={`agency-metric ${metric.tone}`} onClick={() => onSystemChange(metric.label === '账户异常' || metric.label === '临期交付' ? 'delivery' : 'creative')}>
        <span>{metric.label}</span><b>{metric.value}</b><small>{metric.detail}</small>
      </button>)}
    </section>
    <div className="agency-workbench-grid">
      <section className="agency-panel agency-main-panel">
        <div className="section-header"><div><span className="section-label">TODAY</span><h2>今日待处理</h2></div><span className="readonly-chip">只下钻</span></div>
        <div className="agency-record-list">
          {portfolio.today.map(record => <button key={record.id} className="agency-record" onClick={() => openRecord(record)}>
            <span className={`record-priority ${record.tone}`}>{record.priority}</span>
            <span className="record-main"><b>{record.title}</b><small>{record.client} / {record.brand} / {record.project}</small><em>{record.object} · {record.detail}</em></span>
            <span className="record-meta"><small>负责人</small><b>{record.owner}</b></span>
            <span className="record-meta"><small>截止</small><b>{record.dueAt}</b></span>
            <ArrowRight size={16}/>
          </button>)}
          {!portfolio.today.length ? <div className="project-empty">{isLoading && !workbench ? '正在恢复代理商工作台…' : workbenchError ? '服务未连接，暂时不能读取跨客户待处理队列。' : '今日没有需要下钻处理的事项。可从最近 Project 或创意队列继续。'}</div> : null}
        </div>
      </section>
      <aside className="agency-panel">
        <div className="section-header"><div><span className="section-label">CHECK</span><h2>待检查</h2></div></div>
        <div className="agency-mini-list">
          {portfolio.checks.map(record => <button key={record.id} onClick={() => openRecord(record)}><span><b>{record.object}</b><small>{record.client} · {record.brand}</small></span><em>{record.dueAt}</em><ArrowRight size={14}/></button>)}
          {!portfolio.checks.length ? <div className="panel-empty">暂无待检查素材。</div> : null}
        </div>
      </aside>
      <section className="agency-panel">
        <div className="section-header"><div><span className="section-label">DELIVERY</span><h2>临期交付</h2></div></div>
        <div className="agency-mini-list">
          {portfolio.deliveries.map(record => <button key={record.id} onClick={() => openRecord(record)}><span><b>{record.project}</b><small>{record.object} · {record.owner}</small></span><em>{record.dueAt}</em><ArrowRight size={14}/></button>)}
        </div>
      </section>
      <section className="agency-panel">
        <div className="section-header"><div><span className="section-label">CLIENT HEALTH</span><h2>客户健康</h2></div></div>
        <div className="agency-health-list">
          {portfolio.health.map(client => <div key={client.id}><span><b>{client.name}</b><small>{client.owner} · {client.projects} 个 Project</small></span><Status value={client.issues ? `${client.status} · ${client.issues} 项风险` : client.status}/></div>)}
        </div>
      </section>
      <section className="agency-panel">
        <div className="section-header"><div><span className="section-label">TEAM LOAD</span><h2>团队负载</h2></div></div>
        <div className="agency-load-list">
          {portfolio.load.map(item => <div key={item.owner}><span><b>{item.owner}</b><small>{item.detail}</small></span><i><em style={{ width: `${Math.min(100, item.count * 20)}%` }}/></i><strong>{item.count}</strong></div>)}
        </div>
      </section>
      <section className="agency-panel">
        <div className="section-header"><div><span className="section-label">RECENT PROJECT</span><h2>最近 Project</h2></div></div>
        <div className="agency-mini-list">
          {portfolio.recent.map(project => <button key={project.id} onClick={() => onManageProject(project.id)}><span><b>{project.name}</b><small>{project.runtime.code} · {project.progressDetail.stageLabel}</small></span><em>{project.updatedAt.slice(5, 10)}</em><ArrowRight size={14}/></button>)}
        </div>
      </section>
    </div>
  </div>
}

function ViewTabs({ item, activeView, onViewChange, vertical = false }: {
  item: NavItem
  activeView: string
  onViewChange: (view: string) => void
  vertical?: boolean
}) {
  return <nav
    className={vertical ? 'strategy-workspace-view-nav' : 'tabs'}
    role="tablist"
    aria-label={`${item.label}视图`}
    aria-orientation={vertical ? 'vertical' : 'horizontal'}
  >
    {item.views.map(view => <button
      key={view}
      role="tab"
      aria-selected={view === activeView}
      className={view === activeView ? (vertical ? 'active' : 'tab active') : (vertical ? '' : 'tab')}
      onClick={() => onViewChange(view)}
    >{vertical ? <span/> : null}{view}</button>)}
  </nav>
}

function PageHeader({ item, activeView, onViewChange, onPrimaryAction, busy, actionLabel, showTabs = true, showDescription = true }: { item: NavItem; activeView: string; onViewChange: (v: string) => void; onPrimaryAction: () => void; busy: boolean; actionLabel?: string; showTabs?: boolean; showDescription?: boolean }) {
  return <>
    <div className="page-header">
      <div><h1>{item.label}</h1><p>{item.description}</p></div>
      {actionLabel ? <button className="primary-button" onClick={onPrimaryAction} disabled={busy}>{busy ? '正在保存…' : <><Plus size={16} />{actionLabel}</>}</button> : <span className="page-context-label">Project 数据自动关联 · 无需重复建任务</span>}
    </div>
    {showTabs && item.views.length > 1 ? <ViewTabs item={item} activeView={activeView} onViewChange={onViewChange}/> : null}
  </>
}

export function DashboardPage({ system, onSystemChange, onOpenProject }: { system: SystemDefinition; onSystemChange: (key: SystemKey) => void; onOpenProject: OpenProject }) {
  const { currentProject } = useProject()
  const { configuredCount } = useModelConfig()
  const [notice, setNotice] = useState('')
  const [taskDomain, setTaskDomain] = useState<'strategy' | 'creative' | null>(null)
  const systemIndex = systems.findIndex(s => s.key === system.key)
  const workItems = operationRecords(currentProject.operations, 'work_item')
  const currentItem = workItems[systemIndex] ?? workItems[0]
  const journey = dashboardJourneys[system.key]
  const projectProgress = calculateProjectProgress(currentProject)
  const dashboardAction = system.key === 'strategy' ? '新建策略任务' : system.key === 'creative' ? '新建创意任务' : system.key === 'insight' ? '查看广告数据' : '配置投放计划'
  const runDashboardAction = () => {
    if (system.key === 'strategy' || system.key === 'creative') setTaskDomain(system.key)
    else onOpenProject(currentProject.id, system.key, system.key === 'insight' ? 'analysis' : 'plans')
  }
  const taskCreated = (task: BusinessTaskRecord) => {
    setTaskDomain(null)
    setNotice(`${task.name} 已写入服务端并关联当前 Project`)
    onOpenProject(currentProject.id, system.key, 'tasks', task.id)
  }
  return <div className="page-frame dashboard-page">
    <div className="dashboard-intro">
      <div><div className="eyeline">2026 年 7 月 22 日，星期三</div><h1>早上好，Amelia</h1><p>{system.statement} 这里优先呈现需要你判断的工作。</p></div>
      <button className="primary-button" onClick={runDashboardAction}><Plus size={16} />{dashboardAction}</button>
    </div>
    {notice ? <div className="page-notice" role="status"><CircleCheck size={16}/>{notice}<button aria-label="关闭提示" onClick={() => setNotice('')}>×</button></div> : null}
    <section className={`demo-guide system-${system.key}`} aria-label={`${system.label}业务路径`}>
      <div className="demo-guide-heading"><div><span className="section-label">{system.shortLabel.toUpperCase()} WORKFLOW</span><h2>{system.label}的独立工作路径</h2><p>{system.statement} 所有页面都读取当前 Project 的同一条业务链路。</p></div><span className="source-chip">{currentProject.code}</span></div>
      <div className="demo-step-list">{journey.map((step, index) => <button key={step.label} onClick={() => onOpenProject(currentProject.id, system.key, step.navId)}><span>{String(index + 1).padStart(2, '0')}</span><div><b>{step.label}</b><small>{step.detail}</small></div><ArrowRight size={15}/></button>)}</div>
      {configuredCount === 0 ? <div className="demo-provider-notice" role="status"><CircleAlert size={15}/><span>未配置方舟 Provider：可完整讲解预置项目、预检、审批与审计；AI 生成按钮会保持禁用，不会使用或展示浏览器端密钥。</span></div> : null}
    </section>
    <section className="focus-band">
      <div className="focus-number">01</div>
      <div className="focus-main"><span className="section-label">现在需要关注</span><h2>{currentProject.name}</h2><p>{projectProgress.available ? `${projectProgress.stageLabel}已推进至 ${progressPercentLabel(projectProgress)}，下一步需要确认关键决策与证据边界。` : progressReasonLabel(projectProgress)}</p><div className="focus-meta">{currentItem ? <><Status value={currentItem.status} /><span>负责人 {operationField(currentItem, 'owner')}</span></> : <span>暂无服务端工作项</span>}<span>更新于 {currentProject.updatedAt}</span></div></div>
      <div className="focus-progress"><div className="progress-ring" style={{'--progress': `${projectProgress.available && projectProgress.taskPercent !== null ? projectProgress.taskPercent * 3.6 : 0}deg`} as CSSProperties}><span>{projectProgress.available && projectProgress.taskPercent !== null ? <>{projectProgress.taskPercent}<small>%</small></> : <small>无法计算</small>}</span></div><button className="text-button" onClick={() => onOpenProject(currentProject.id, system.key, system.key === 'strategy' ? 'workspaces' : system.key === 'creative' ? 'tasks' : system.key === 'insight' ? 'experience' : 'approvals')}>继续工作<ArrowRight size={15} /></button></div>
    </section>
    <div className="dashboard-grid">
      <section className="open-section workstream">
        <div className="section-header"><div><span className="section-label">跨系统进度</span><h2>{currentProject.name}</h2></div><button className="secondary-button" onClick={() => onOpenProject(currentProject.id, 'strategy', 'workspaces')}>查看项目总览</button></div>
        <ArtifactFlow compact/>
        <div className="work-list">
          {workItems.slice(0, 4).map(item => <div className="work-row" key={item.id}><div className="work-name"><b>{item.title}</b><small>{operationField(item, 'type')} · {operationField(item, 'owner')}</small></div><div className="inline-progress"><span style={{width: `${operationField(item, 'progress')}%`}} /></div><strong>{operationField(item, 'progress')}%</strong><Status value={item.status} /><button aria-label="更多操作"><MoreHorizontal size={17} /></button></div>)}
          {!workItems.length ? <div className="panel-empty">暂无服务端工作项。</div> : null}
        </div>
      </section>
      <aside className="attention-rail">
        <div className="section-header"><div><span className="section-label">你的队列</span><h2>{workItems.length} 项服务端工作</h2></div></div>
        <div className="queue-list">{workItems.slice(0, 3).map((item, index) => <button key={item.id} onClick={() => onOpenProject(currentProject.id, index === 0 ? 'strategy' : index === 1 ? 'creative' : 'delivery', index === 2 ? 'approvals' : 'tasks', item.id)}><span className={`queue-icon ${index === 1 ? 'danger' : index === 2 ? 'info' : 'warning'}`}>{index === 1 ? <CircleAlert size={16} /> : index === 2 ? <Bot size={16} /> : <Clock3 size={16} />}</span><span><b>{item.title}</b><small>{operationField(item, 'type')} · {item.status}</small></span><ArrowRight size={15} /></button>)}{!workItems.length ? <div className="panel-empty">暂无服务端待处理项。</div> : null}</div>
        <div className="quick-actions"><span className="section-label">快速开始</span>{quickActions.map(action => <button key={action.label} onClick={() => onSystemChange(action.system)}><span><b>{action.label}</b><small>{action.detail}</small></span><ArrowRight size={15} /></button>)}</div>
      </aside>
    </div>
    {taskDomain ? <TaskCreateDialog domain={taskDomain} onClose={() => setTaskDomain(null)} onCreated={taskCreated}/> : null}
  </div>
}

function WorkspaceSurface({ item, activeView }: { item: NavItem; activeView: string }) {
  const { currentProject, reloadProjects, updateArtifact } = useProject()
  const industry = industryProfile(currentProject.industry)
  const [briefPrompt, setBriefPrompt] = useState('')
  const [brief, setBrief] = useState<ApiArtifact | null>(null)
  const [briefModel, setBriefModel] = useState('')
  const [briefNotice, setBriefNotice] = useState('')
  const [isGenerating, setIsGenerating] = useState(false)
  const evidence = operationRecords(currentProject.operations, 'evidence')

  useEffect(() => {
    let active = true
    void Promise.all([api.listArtifacts(currentProject.id), api.listJobs(currentProject.id)]).then(([artifacts, jobs]) => {
      const latest = artifacts.filter(artifact => artifact.kind === 'brief').at(-1)
      if (active && latest) {
        setBrief(latest)
        setBriefModel(jobs.find(job => job.id === latest.sourceJobId)?.model ?? '服务端已存档')
      }
    }).catch(() => undefined)
    return () => { active = false }
  }, [currentProject.id])

  const generateBrief = async () => {
    if (!briefPrompt.trim()) {
      setBriefNotice('请先输入需求、受众或业务目标。')
      return
    }
    setIsGenerating(true)
    try {
      const result = await api.generateBrief(currentProject.id, briefPrompt)
      setBrief(result.artifact)
      setBriefModel(result.job.model ?? '服务端默认文本模型')
      await reloadProjects()
      setBriefNotice('策略 Brief 草稿已生成，等待人工确认。')
    } catch (cause) {
      setBriefNotice(cause instanceof Error ? cause.message : '生成策略 Brief 失败，请重试。')
    } finally {
      setIsGenerating(false)
    }
  }

  const confirmBrief = async () => {
    if (!brief) return
    try {
      await updateArtifact('brief', { status: '已确认', summary: brief.content.slice(0, 52) })
      setBrief({ ...brief, status: 'ready' })
      setBriefNotice('Brief 已确认，可进入创意生成。')
    } catch (cause) {
      setBriefNotice(cause instanceof Error ? cause.message : '确认 Brief 失败，请重试。')
    }
  }

  return <div className="workspace-surface">
    <section className="document-panel">
      <IndustrySchema module="需求与策略" profile={industry.strategy} industry={industry.label}/>
      <div className="surface-toolbar"><div><span className="ai-chip"><Bot size={14} />{activeView}</span><span>{currentProject.artifacts.strategy.version} · 已引用 4 条证据</span></div><button className="secondary-button"><Pencil size={14} />编辑</button></div>
      {[
        ['推荐定位', '白域精工，以精密制造的可靠性为品牌核心，为创新产品提供高精度、高一致性与稳定交付。'],
        ['目标受众', '电子消费品品牌采购与供应链负责人（25–45 岁）\n产品研发工程师与工业设计师（25–40 岁）'],
        ['核心信息', '看得见的精度，兑现你的创新。精度 ±0.01mm，交付准时率 98%+。'],
        ['创意路线', '理性证据线：以精度、良率、交期为核心证据\n场景应用线：展示真实应用案例与制造过程'],
        ['成功指标', '官网表单提交量提升 ≥30%\n关键行业线索成本（CPL）降低 ≥20%'],
      ].map(([label, content], index) => <div className="strategy-row" key={label}><h3>{label}</h3><p>{content}</p><span className="citation">[{index + 1}]</span><button aria-label={`编辑${label}`}><Pencil size={15} /></button></div>)}
      <div className="prompt-box"><label htmlFor="ai-prompt">输入广告需求，生成策略 Brief</label><div><input id="ai-prompt" value={briefPrompt} onChange={event => setBriefPrompt(event.target.value)} placeholder="例如：面向研发工程师，突出新品精度与交期，获取销售线索"/><button aria-label="生成策略 Brief" onClick={() => void generateBrief()} disabled={isGenerating}>{isGenerating ? '生成中…' : <Send size={18} />}</button></div></div>
      {brief ? <div className="insight-note"><span>AI 生成 Brief · {brief.status === 'ready' ? '已确认' : '草稿'}</span><p>{brief.content}</p><small>模型：{briefModel || '服务端已存档'} · 任务：{brief.sourceJobId ?? '手工创建'}</small>{brief.status !== 'ready' ? <button className="secondary-button" onClick={() => void confirmBrief()}>确认 Brief</button> : null}</div> : null}
      {briefNotice ? <div className="inline-notice" role="status">{briefNotice}</div> : null}
    </section>
    <section className="brief-panel"><div className="surface-toolbar"><h3>对象摘要</h3><button className="text-button">编辑</button></div>{[['项目', currentProject.name], ['目标', currentProject.goal], ['核心产品', currentProject.product], ['主要区域', '中国大陆（华东、华南）'], ['预算', `¥${currentProject.budget.toLocaleString('zh-CN')}`], ['周期', '2026-07-25 至 2026-08-31']].map(([label, value]) => <div className="kv" key={label}><span>{label}</span><b>{value}</b></div>)}<div className="decision-block"><div><b>关键决策</b><span>4/5 已确认</span></div>{['品牌主张', '核心信息', '受众定义', '创意路线', '成功指标'].map((v, i) => <div key={v}><span>{v}</span><Status value={i === 0 ? '已确认' : 'AI 建议'} /></div>)}</div></section>
    <aside className="evidence-panel"><div className="surface-toolbar"><h3>证据</h3><button className="text-button">收起</button></div>{evidence.map(item => <button className="evidence-item" key={item.id}><span className="evidence-id">{item.id}</span><span><b>{item.title}</b><small>来源：{operationField(item, 'source')}</small><small>{new Date(item.occurredAt).toLocaleDateString('zh-CN')} · {operationField(item, 'confidence')}相关</small></span><ExternalLink size={14} /></button>)}{!evidence.length ? <div className="panel-empty">暂无服务端证据记录。</div> : null}</aside>
  </div>
}

function AnalysisSurface({ item, activeView }: { item: NavItem; activeView: string }) {
  const { currentProject } = useProject()
  const metric = operationRecords(currentProject.operations, 'metric')[0]
  const chartPoints = metric?.fields.points?.toString().split(',').map(Number).filter(Number.isFinite) ?? []
  return <div className="analysis-layout">
    <section className="analysis-main"><div className="analysis-heading"><div><span className="section-label">{activeView}</span><h2>{item.label}中，什么正在改变？</h2><p>指标基于当前 Project 的服务端运营记录。</p></div><div className="metric-pair"><span><small>当前</small><b>{chartPoints.at(-1) ?? '—'}{chartPoints.length ? '%' : ''}</b></span><span><small>指标</small><b className="positive">{metric ? operationField(metric, 'unit') : '—'}</b></span></div></div>{chartPoints.length ? <><TrendChart points={chartPoints} /><div className="chart-axis"><span>W1</span><span>W4</span><span>W8</span><span>W12</span></div></> : <div className="panel-empty">暂无服务端指标趋势。</div>}<div className="insight-note"><span>关键转折</span><p>{metric?.title ?? `${activeView}暂无服务端分析结论。`}</p></div></section>
    <aside className="analysis-rail"><span className="section-label">解释与行动</span><h3>服务端指标说明</h3><div className="driver"><span>01</span><b>{metric?.title ?? '暂无指标记录'}</b><strong>{metric ? operationField(metric, 'unit') : '—'}</strong></div><button className="secondary-button full">查看证据与样本</button></aside>
  </div>
}

function IndustrySchema({ module, profile, industry }: { module: string; industry: string; profile: { fields: string[]; format: string } }) {
  return <section className="industry-schema" aria-label={`${industry}${module}配置`}>
    <span>{industry} · {module}</span><b>{profile.format}</b>
    <div>{profile.fields.map(field => <small key={field}>{field}</small>)}</div>
  </section>
}

function EditorSurface({ item, activeView }: { item: NavItem; activeView: string }) {
  const { providers } = useModelConfig()
  const { currentProject, reloadProjects } = useProject()
  const [selected, setSelected] = useState(1)
  const [description, setDescription] = useState('高速主轴切削金属零件的微距镜头，冷白光，真实工业质感。')
  const [notice, setNotice] = useState('')
  const [job, setJob] = useState<Awaited<ReturnType<typeof api.createMedia>> | null>(null)
  const [confirmedBriefId, setConfirmedBriefId] = useState('')
  const configuredProvider = providers.find(provider => provider.status === '已配置')
  const mediaKind = item.id === 'video' ? 'video' : 'image'

  useEffect(() => {
    let active = true
    void Promise.all([api.listArtifacts(currentProject.id), api.listJobs(currentProject.id)]).then(([artifacts, jobs]) => {
      const latest = jobs.filter(candidate => candidate.artifactKind === mediaKind).at(-1)
      const brief = artifacts.filter(artifact => artifact.kind === 'brief' && artifact.status === 'ready').at(-1)
      if (active) {
        setJob(latest ?? null)
        setConfirmedBriefId(brief?.id ?? '')
      }
    }).catch(() => undefined)
    return () => { active = false }
  }, [currentProject.id, mediaKind])

  useEffect(() => {
    if (!job || !['queued', 'running'].includes(job.status)) return
    const timer = window.setInterval(() => {
      void api.getJob(job.id).then(next => {
        setJob(next)
        if (next.status === 'succeeded') {
          void reloadProjects()
          setNotice('生成完成，稳定资产已关联到当前任务。')
        }
      }).catch(cause => setNotice(cause instanceof Error ? cause.message : '任务状态读取失败'))
    }, 1500)
    return () => window.clearInterval(timer)
  }, [job, mediaKind, reloadProjects])

  const generate = async () => {
    if (!confirmedBriefId) {
      setNotice('请先在需求中心确认 Brief，再发起图片或视频生成。')
      return
    }
    try {
      const next = await api.createMedia(currentProject.id, mediaKind, description, confirmedBriefId)
      setJob(next)
      setNotice(next.status === 'succeeded' ? '生成完成，资产已保存。' : '生成任务已创建，正在轮询状态。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '创建生成任务失败，请重试。')
    }
  }

  const cancel = async () => {
    if (!job) return
    try {
      setJob(await api.cancelJob(job.id))
      setNotice('生成任务已取消。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '取消任务失败。')
    }
  }

  return <div className="editor-layout">
    <aside className="asset-rail"><div className="surface-toolbar"><h3>结构与素材</h3><button aria-label="新增镜头"><Plus size={15}/></button></div>{['开场：精度的瞬间', '产品与制造过程', '真实应用场景', '品牌主张与 CTA'].map((label, i) => <button className={i === selected ? 'asset-row active' : 'asset-row'} onClick={() => setSelected(i)} key={label}><span>{String(i + 1).padStart(2, '0')}</span><b>{label}</b><small>{i === 1 ? '00:06–00:18' : `${i * 8 + 1} 秒`}</small></button>)}</aside>
    <section className="canvas-area"><div className="canvas-toolbar"><span>{item.label} · v1.2</span><div><button>50%</button><button><Download size={15}/>导出预览</button></div></div><div className="media-canvas"><div className="precision-art"><img src="/assets/white-precision-cnc.png" alt="高精度 CNC 设备加工金属零件"/><div className="art-copy"><small>WHITE PRECISION</small><h2>看得见的精度，<br/>兑现你的创新。</h2><p>±0.01mm · 98%+ 准时交付</p></div></div></div><div className="timeline"><div className="time-ruler">00:00 <span>00:06</span><span>00:12</span><span>00:18</span><span>00:24</span><span>00:30</span></div>{['画面', '字幕', '音乐'].map((track, index) => <div className="track" key={track}><b>{track}</b><span className={`clip clip-${index + 1}`}>{index === 0 ? '精密加工 · 06–18s' : index === 1 ? '品牌主张' : 'Precision Theme.wav'}</span></div>)}</div></section>
    <aside className="inspector"><div className="surface-toolbar"><h3>{activeView}属性</h3><button aria-label="属性更多操作"><MoreHorizontal size={16}/></button></div>{['内容', '画面', '声音', '品牌检查'].map((tab, i) => <button className={i === 0 ? 'inspector-tab active' : 'inspector-tab'} key={tab}>{tab}<ChevronDown size={14}/></button>)}<div className="field"><label>镜头描述</label><textarea value={description} onChange={event => setDescription(event.target.value)}/></div><div className="field"><label>生成模型</label><button className="select-field">{configuredProvider ? `${configuredProvider.name} · 服务端模型目录` : '服务端未配置模型'}<ChevronDown size={14}/></button></div>{!configuredProvider ? <div className="model-required"><CircleAlert size={15}/><span>请在服务端设置 ARK_API_KEY 后重新检查能力。</span></div> : null}{!confirmedBriefId ? <div className="model-required"><CircleAlert size={15}/><span>请先在需求中心确认 Brief，系统才会允许生成媒体。</span></div> : null}<button className="primary-button full" disabled={!configuredProvider || !confirmedBriefId || ['queued', 'running'].includes(job?.status ?? '')} onClick={() => void generate()}>{job && ['queued', 'running'].includes(job.status) ? '正在生成…' : `生成选中${mediaKind === 'image' ? '图片' : '视频'}`}</button>{job ? <div className="inline-notice" role="status">任务 {shortId(job.id)} · {job.status} · {job.model ?? '模型待分配'}{job.diagnostic ? ` · ${job.diagnostic}` : ''}{['queued', 'running'].includes(job.status) ? <button onClick={() => void cancel()}>取消</button> : job.status === 'failed' || job.status === 'cancelled' ? <button onClick={() => void generate()}>重试</button> : null}</div> : null}{notice ? <div className="inline-notice" role="status">{notice}</div> : null}</aside>
  </div>
}

function TableSurface(props: { item: NavItem; activeView: string; onOpenRecord: (id: string) => void }) {
  if (props.activeView === '广告账户') return <ProjectAdAccountSurface/>
  return <GenericTableSurface {...props}/>
}

function ProjectAdAccountSurface() {
  const { currentProject } = useProject()
  return <OceanEngineSessionSettings projectId={currentProject.id}/>
}

function AdAccountBindingSurface({ item, activeView }: { item: NavItem; activeView: string }) {
  const { currentProject } = useProject()
  const [search, setSearch] = useState('')
  const [attentionOnly, setAttentionOnly] = useState(false)
  const [notice, setNotice] = useState('')
  const [workbench, setWorkbench] = useState<Awaited<ReturnType<typeof api.listAgencyWorkbench>> | null>(null)

  useEffect(() => {
    let active = true
    void api.listAgencyWorkbench({ projectIds: [currentProject.id] }).then(next => {
      if (active) setWorkbench(next)
    }).catch(cause => {
      if (active) setNotice(cause instanceof Error ? cause.message : '读取账户绑定失败。')
    })
    return () => { active = false }
  }, [])

  const clientsById = useMemo(() => new Map(workbench?.clients.map(client => [client.id, client]) ?? []), [workbench])
  const brandsById = useMemo(() => new Map(workbench?.brands.map(brand => [brand.id, brand]) ?? []), [workbench])
  const projectBindings = useMemo(() => {
    const bindings = workbench?.adAccountBindings.filter(binding => binding.projectIds.includes(currentProject.id)) ?? []
    return bindings.sort((left, right) => {
      const leftPriority = bindingStatusScore(left.permissionStatus) * 100 + bindingStatusScore(left.loginStatus) * 80 + bindingStatusScore(left.trackingStatus) * 60
      const rightPriority = bindingStatusScore(right.permissionStatus) * 100 + bindingStatusScore(right.loginStatus) * 80 + bindingStatusScore(right.trackingStatus) * 60
      return rightPriority - leftPriority || right.lastSyncedAt.localeCompare(left.lastSyncedAt)
    })
  }, [currentProject.id, workbench])
  const filtered = useMemo(() => projectBindings.filter(binding => {
    const client = clientsById.get(binding.clientId)
    const brand = brandsById.get(binding.brandId)
    const haystack = `${binding.platform} ${client?.name ?? ''} ${brand?.name ?? ''} ${binding.accountName} ${binding.accountDisplayId} ${binding.owner}`.toLowerCase()
    const needsAttention = [binding.permissionStatus, binding.loginStatus, binding.trackingStatus].some(status => status !== 'normal')
    return haystack.includes(search.toLowerCase()) && (!attentionOnly || needsAttention)
  }), [attentionOnly, brandsById, clientsById, projectBindings, search])

  const copyAccountId = async (accountId: string) => {
    try {
      await navigator.clipboard.writeText(accountId)
      setNotice(`账户 ID ${accountId} 已复制。`)
    } catch {
      setNotice(`账户 ID ${accountId} 可在表格中手动复制。`)
    }
  }

  return <section className="table-surface account-binding-surface">
    <div className="table-toolbar">
      <div className="search-field"><Search size={16}/><input aria-label="搜索广告账户绑定" value={search} onChange={event => setSearch(event.target.value)} placeholder={`搜索${item.label}`}/></div>
      <button className={attentionOnly ? 'secondary-button active-filter' : 'secondary-button'} onClick={() => setAttentionOnly(value => !value)} aria-pressed={attentionOnly}><Filter size={15}/>异常优先</button>
      <span className="table-count">{activeView} · 共 {filtered.length} 个绑定</span>
    </div>
    {projectBindings.length ? <>
      <table>
        <thead><tr><th>平台</th><th>客户 / 品牌</th><th>账户名称与 ID</th><th>权限</th><th>登录</th><th>追踪</th><th>绑定资产</th><th>负责人</th><th>最近同步</th></tr></thead>
        <tbody>{filtered.map(binding => {
          const client = clientsById.get(binding.clientId)
          const brand = brandsById.get(binding.brandId)
          return <tr key={binding.id}>
            <td><span className="source-chip">{binding.platform}</span></td>
            <td><b>{client?.name ?? '未知客户'}</b><small>{brand?.name ?? '未知品牌'}</small></td>
            <td><b>{binding.accountName}</b><button className="account-id-copy" onClick={() => void copyAccountId(binding.accountDisplayId)} aria-label={`复制账户 ID ${binding.accountDisplayId}`}><span className="code">{binding.accountDisplayId}</span><ClipboardCheck size={14}/></button></td>
            <td><BindingStatus value={binding.permissionStatus}/></td>
            <td><BindingStatus value={binding.loginStatus}/></td>
            <td><BindingStatus value={binding.trackingStatus}/></td>
            <td><b>{binding.boundAssetIds.length} 个资产</b><small>{binding.boundAssetIds.join(' / ') || '尚未绑定资产'}</small></td>
            <td>{binding.owner}</td>
            <td><time>{new Date(binding.lastSyncedAt).toLocaleString('zh-CN', { hour12: false })}</time><small>{bindingAttentionSummary(binding)}</small></td>
          </tr>
        })}</tbody>
      </table>
      {!filtered.length ? <div className="table-empty">没有匹配的账户绑定，请调整搜索或关闭异常筛选。</div> : null}
    </> : <div className="account-empty"><ShieldCheck size={22}/><h3>当前 Project 尚未绑定广告账户</h3><p>请在组织资源中连接广告平台，再将客户、品牌和广告账户绑定到当前 Project。绑定后页面会展示账户 ID、权限、登录、追踪、资产和同步状态。</p><small>安全边界：本页不返回也不展示平台 Token、密钥或登录凭据。</small></div>}
    {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
  </section>
}

function GenericTableSurface({ item, activeView, onOpenRecord }: { item: NavItem; activeView: string; onOpenRecord: (id: string) => void }) {
  const { currentProject } = useProject()
  const [search, setSearch] = useState('')
  const [attentionOnly, setAttentionOnly] = useState(false)
  const [showOwner, setShowOwner] = useState(true)
  const [page, setPage] = useState(0)
  const pageSize = 4
  const records = operationRecords(currentProject.operations, 'unified_record')
  const filtered = useMemo(() => records.filter(record => `${record.id} ${record.title} ${operationField(record, 'kind')} ${record.status}`.toLowerCase().includes(search.toLowerCase()) && (!attentionOnly || ['待审批', '待确认'].includes(record.status))), [records, search, attentionOnly])
  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize))
  const rows = filtered.slice(page * pageSize, page * pageSize + pageSize)
  return <section className="table-surface"><div className="table-toolbar"><div className="search-field"><Search size={16}/><input aria-label="搜索列表" value={search} onChange={event => { setSearch(event.target.value); setPage(0) }} placeholder={`搜索${item.label}`}/></div><button className={attentionOnly ? 'secondary-button active-filter' : 'secondary-button'} onClick={() => { setAttentionOnly(value => !value); setPage(0) }} aria-pressed={attentionOnly}><Filter size={15}/>待处理</button><button className="secondary-button" onClick={() => setShowOwner(value => !value)} aria-pressed={showOwner}><SlidersHorizontal size={15}/>{showOwner ? '隐藏负责人' : '显示负责人'}</button><span className="table-count">{activeView} · 共 {filtered.length} 条</span></div><table><thead><tr><th>编号</th><th>名称</th><th>类型</th><th>状态</th>{showOwner ? <th>负责人</th> : null}<th>最后更新</th><th aria-label="操作"/></tr></thead><tbody>{rows.map(row => <tr key={row.id}><td className="code">{row.id}</td><td><button className="table-object-link" onClick={() => onOpenRecord(row.id)}><b>{row.title}</b><small>{currentProject.name}</small></button></td><td>{operationField(row, 'kind')}</td><td><Status value={row.status}/></td>{showOwner ? <td>{operationField(row, 'owner')}</td> : null}<td>{new Date(row.updatedAt).toLocaleString('zh-CN', { hour12: false })}</td><td><button aria-label={`${row.title}更多操作`} onClick={() => onOpenRecord(row.id)}><MoreHorizontal size={17}/></button></td></tr>)}</tbody></table>{!rows.length ? <div className="table-empty">没有服务端记录，请调整搜索或筛选条件。</div> : null}<div className="table-footer"><span>第 {page + 1} / {pageCount} 页</span><div><button disabled={page === 0} onClick={() => setPage(value => Math.max(0, value - 1))}>上一页</button><button disabled={page >= pageCount - 1} onClick={() => setPage(value => Math.min(pageCount - 1, value + 1))}>下一页</button></div></div></section>
}

function OperationsSurface({ item }: { item: NavItem }) {
  const { currentProject } = useProject()
  const activity = operationRecords(currentProject.operations, 'activity')
  return <div className="ops-layout"><section className="ops-main"><div className="ops-status"><span className="signal ok"><CircleCheck size={18}/></span><div><span className="section-label">系统状态</span><h2>{item.label}运行稳定</h2><p>当前状态和活动均来自当前 Project 的服务端运营记录。</p></div><button className="secondary-button">查看运行记录</button></div><div className="ops-list">{activity.slice(0, 4).map(record => <div key={record.id}><span>{record.title}</span><b>{operationField(record, 'detail')}</b><Status value={record.status}/><button aria-label={`查看${record.title}详情`}><ArrowRight size={15}/></button></div>)}{!activity.length ? <div className="panel-empty">暂无服务端运行记录。</div> : null}</div></section><aside className="ops-rail"><span className="section-label">最近活动</span>{activity.map(record => <div className="activity-item" key={record.id}><time>{new Date(record.occurredAt).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}</time><span><b>{record.title}</b><small>{operationField(record, 'actor')} · {operationField(record, 'detail')}</small></span></div>)}{!activity.length ? <div className="panel-empty">暂无服务端活动。</div> : null}</aside></div>
}

function AuditEvidenceSurface() {
  const { currentProject } = useProject()
  const [events, setEvents] = useState<ApiAuditEvent[]>([])
  const [notice, setNotice] = useState('')

  useEffect(() => {
    let active = true
    void api.listAuditEvents(currentProject.id).then(records => {
      if (active) setEvents(records)
    }).catch(cause => {
      if (active) setNotice(cause instanceof Error ? cause.message : '读取审计记录失败')
    })
    return () => { active = false }
  }, [currentProject.id])

  return <div className="audit-evidence-surface">
    <section>
      <div className="audit-evidence-heading"><div><span className="section-label">SERVER AUDIT</span><h2>服务端审计轨迹</h2><p>记录预置项目的创建、产物确认、预检、审批、模拟执行与回滚；不会连接真实广告平台。</p></div><span className="source-chip">不可变事件</span></div>
      <div className="audit-event-list">{events.length ? events.map(event => <article key={event.id}><span>{new Date(event.createdAt).toLocaleString('zh-CN', { hour12: false })}</span><div><b>{auditActionLabel(event.action)}</b><small>{event.actor} · {event.entityType} · {shortId(event.entityId)}</small></div><CircleCheck size={16}/></article>) : <div className="panel-empty">正在读取服务端审计记录…</div>}</div>
    </section>
    <aside className="audit-boundary"><ShieldCheck size={18}/><h3>模拟边界</h3><p>这些事件只记录本地 MVP 的受控投放模拟。审批、执行和回滚不会对广告账户或外部平台写入。</p></aside>
    {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
  </div>
}

function RemixMMLUEvalSurface() {
  const { currentProject } = useProject()
  const [cases, setCases] = useState<ApiRemixEvalCase[]>([])
  const [run, setRun] = useState<ApiRemixEvalRun | null>(null)
  const [notice, setNotice] = useState('')
  const [isRunning, setIsRunning] = useState(false)

  useEffect(() => {
    let active = true
    void api.listRemixEvalCases(currentProject.id).then(response => {
      if (active) setCases(response.items)
    }).catch(cause => {
      if (active) setNotice(cause instanceof Error ? cause.message : '读取 Remix-MMLU 评测集失败')
    })
    return () => { active = false }
  }, [currentProject.id])

  const runEval = async () => {
    setIsRunning(true)
    setNotice('')
    try {
      const nextRun = await api.createRemixEvalRun(currentProject.id, {
        planner_version: 'planner.v1',
        prompt_version: 'prompt.v1',
        submissions: [
          { case_id: 'remix_mmlu_hook_mcq_v1', choice_id: 'b' },
          { case_id: 'remix_mmlu_rubric_v1', answer_text: 'authorized timeline risk' },
        ],
      })
      setRun(nextRun)
      setNotice(`评测运行 ${nextRun.id} 已完成，可复现实验分数。`)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '运行 Remix-MMLU 评测失败')
    } finally {
      setIsRunning(false)
    }
  }

  const failedCaseTitles = run?.failed_cases.map(id => cases.find(item => item.id === id)?.title ?? id) ?? []
  return <div className="agent-eval-stack">
    <div className="ops-layout remix-eval-surface">
      <section className="ops-main">
        <div className="ops-status"><span className="signal ok"><CircleCheck size={18}/></span><div><span className="section-label">REMIX-MMLU</span><h2>Planner 与 Prompt 回归评测</h2><p>保存 MCQ/rubric seed cases，使用 deterministic scorer 验证混剪 Planner 输出是否稳定。</p></div><button className="primary-button" onClick={() => void runEval()} disabled={isRunning}>{isRunning ? '运行中…' : '运行固定评测'}</button></div>
        <div className="ops-list">
          {cases.map(testCase => <div key={testCase.id}><span>{testCase.title}</span><b>{testCase.type.toUpperCase()} · {testCase.planner_version} / {testCase.prompt_version}</b><Status value={testCase.type === 'mcq' ? '选择题' : `Rubric ${Math.round(testCase.passing_score * 100)}分通过`}/><button aria-label={`查看${testCase.title}详情`}><ArrowRight size={15}/></button></div>)}
          {!cases.length ? <div className="panel-empty">暂无评测 case，等待服务端 seed cases 初始化。</div> : null}
        </div>
      </section>
      <aside className="ops-rail">
        <span className="section-label">最新运行</span>
        {run ? <>
          <div className="activity-item"><time>{Math.round(run.score * 100)}%</time><span><b>{run.passed_cases} / {run.total_cases} cases passed</b><small>{run.planner_version} · {run.prompt_version}</small></span></div>
          {run.results.map(result => <div className="activity-item" key={result.id}><time>{result.passed ? 'PASS' : 'FAIL'}</time><span><b>{cases.find(item => item.id === result.case_id)?.title ?? result.case_id}</b><small>{result.reason} · {Math.round(result.score * 100)}%</small></span></div>)}
          {failedCaseTitles.length ? <div className="inline-notice" role="status">失败 case：{failedCaseTitles.join('、')}</div> : <div className="inline-notice" role="status">全部 case 通过，结果可重复。</div>}
        </> : <div className="panel-empty">点击运行后展示总分、失败 case 和关联版本。</div>}
        {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
      </aside>
    </div>
    <AgentRunTracePanel />
  </div>
}

function AgentRunTracePanel() {
  const { currentProject } = useProject()
  const [renderJobId, setRenderJobId] = useState('remixrender_1')
  const [runs, setRuns] = useState<ApiAgentRun[]>([])
  const [selectedRunId, setSelectedRunId] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let active = true
    void api.listAgentRuns(currentProject.id, 10).then(response => {
      if (!active) return
      setRuns(response.items)
      setSelectedRunId(current => current || response.items[0]?.id || '')
    }).catch(cause => {
      if (active) setNotice(cause instanceof Error ? cause.message : '读取 Agent Run 失败')
    })
    return () => { active = false }
  }, [currentProject.id])

  const selectedRun = runs.find(run => run.id === selectedRunId) ?? runs[0]
  const startDiagnosis = async () => {
    if (!renderJobId.trim()) {
      setNotice('请先输入 RenderJob ID。')
      return
    }
    setBusy(true)
    setNotice('')
    try {
      const run = await api.createAgentRun(currentProject.id, renderJobId.trim())
      setRuns(current => [run, ...current.filter(item => item.id !== run.id)])
      setSelectedRunId(run.id)
      setNotice(run.status === 'failed' ? `诊断失败：${run.error_message ?? '未知错误'}` : `Agent Run ${shortId(run.id)} 已完成。`)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '启动 Agent Run 失败')
    } finally {
      setBusy(false)
    }
  }
  const retryDiagnosis = () => {
    if (selectedRun?.target.render_job_id) setRenderJobId(selectedRun.target.render_job_id)
    void startDiagnosis()
  }
  const cancelRun = async () => {
    if (!selectedRun) return
    setBusy(true)
    try {
      const cancelled = await api.cancelAgentRun(currentProject.id, selectedRun.id)
      setRuns(current => current.map(item => item.id === cancelled.id ? cancelled : item))
      setNotice('Agent Run 已取消。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '取消 Agent Run 失败')
    } finally {
      setBusy(false)
    }
  }

  return <section className="agent-trace-surface">
    <div className="agent-trace-heading">
      <div><span className="section-label">AGENT RUN / TRACE</span><h2>渲染诊断 Agent</h2><p>持久化展示步骤、工具调用、模型 span、错误和重试入口。</p></div>
      <div className="agent-run-controls"><input aria-label="RenderJob ID" value={renderJobId} onChange={event => setRenderJobId(event.target.value)} placeholder="输入 remix render job id"/><button className="primary-button" disabled={busy} onClick={() => void startDiagnosis()}>{busy ? '执行中…' : '启动诊断'}</button></div>
    </div>
    <div className="agent-trace-grid">
      <aside className="agent-run-list">
        {runs.map(run => <button key={run.id} className={run.id === selectedRun?.id ? 'active' : ''} onClick={() => setSelectedRunId(run.id)}>
          <span>{run.id.slice(-12)}</span><b>{run.workflow}</b><small>{run.status} · {run.target.render_job_id}</small>
        </button>)}
        {!runs.length ? <div className="panel-empty">暂无 Agent Run，输入 RenderJob ID 后可创建诊断。</div> : null}
      </aside>
      <div className="agent-run-detail">
        {selectedRun ? <>
          <div className="agent-run-summary"><Status value={agentStatusLabel(selectedRun.status)}/><b>{String(selectedRun.output?.diagnosis ?? selectedRun.error_message ?? '等待诊断输出')}</b><span>{String(selectedRun.output?.recommendation ?? '工具和模型 span 会在运行完成后展示。')}</span></div>
          <div className="agent-trace-columns">
            <TraceColumn title="步骤" items={selectedRun.steps.map(step => ({ id: step.id, title: step.label, meta: step.status, body: step.summary }))}/>
            <TraceColumn title="工具调用" items={selectedRun.tool_calls.map(call => ({ id: call.id, title: call.name, meta: call.status, body: call.error_message ?? String(call.output?.recommendation ?? call.output?.diagnosis ?? '无输出') }))}/>
            <TraceColumn title="模型 Span" items={selectedRun.trace_spans.map(span => ({ id: span.id, title: span.name, meta: span.parent_id ? `${span.kind} · parent ${shortId(span.parent_id)}` : span.kind, body: span.error_message ?? `${span.status}${span.model ? ` · ${span.model}` : ''}` }))}/>
          </div>
          <div className="agent-run-actions"><button className="secondary-button" disabled={busy || !['queued', 'running'].includes(selectedRun.status)} onClick={() => void cancelRun()}>取消</button><button className="primary-button" disabled={busy} onClick={retryDiagnosis}>重试诊断</button></div>
        </> : <div className="panel-empty">选择 Agent Run 后查看 trace 详情。</div>}
      </div>
    </div>
    {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
  </section>
}

function TraceColumn({ title, items }: { title: string; items: Array<{ id: string; title: string; meta: string; body: string }> }) {
  return <div className="trace-column"><span className="section-label">{title}</span>{items.map(item => <article key={item.id}><small>{item.meta}</small><b>{item.title}</b><p>{item.body}</p></article>)}{!items.length ? <div className="panel-empty">暂无{title}</div> : null}</div>
}

function agentStatusLabel(status: ApiAgentRun['status']): string {
  const labels: Record<ApiAgentRun['status'], string> = {
    queued: '排队中',
    running: '执行中',
    succeeded: '已完成',
    failed: '失败',
    cancelled: '已取消',
  }
  return labels[status]
}

function auditActionLabel(action: string): string {
  const labels: Record<string, string> = {
    'project.created': '已创建路演项目',
    'artifact.created': '已保存路演产物',
    'artifact.updated': '已更新产物状态',
    'change_set.created': '已创建 ChangeSet',
    'change_set.preflight_completed': '已完成投放预检',
    'change_set.approved': '已通过人工审批',
    'change_set.simulation_started': '已开始模拟执行',
    'change_set.simulation_completed': '已完成模拟执行',
    'change_set.rollback_started': '已开始模拟回滚',
    'change_set.rolled_back': '已完成模拟回滚',
  }
  return labels[action] ?? action
}

function ObjectDetail({ system, item, objectId, onOpenProject }: { system: SystemDefinition; item: NavItem; objectId: string; onOpenProject: OpenProject }) {
  const { currentProject } = useProject()
  const record = operationRecords(currentProject.operations, 'unified_record').find(value => value.id === objectId)
  const name = record?.title ?? `${item.label}草稿 ${objectId}`
  const next = system.key === 'strategy' ? ['creative', 'tasks', 'CR-2607-42', '基于此策略创建创意任务'] as const : system.key === 'creative' ? ['creative', 'reviews', 'CR-2607-42', '提交评审'] as const : system.key === 'insight' ? ['strategy', 'workspaces', 'STR-2607-08', '将洞察应用到策略'] as const : ['delivery', 'execution', objectId, '进入执行中心'] as const
  return <aside className="object-detail" aria-label={`${name}详情`}><div><span className="section-label">服务端对象详情</span><h2>{name}</h2><p>{record ? `${operationField(record, 'kind')} · ${record.status} · ${operationField(record, 'owner')}` : `当前 Project：${currentProject.name}`}</p></div><div className="detail-kv"><span>对象 ID</span><b>{objectId}</b></div><div className="detail-kv"><span>来源版本</span><b>{currentProject.artifacts.strategy.version} → {currentProject.artifacts.creative.version}</b></div><button className="primary-button full" onClick={() => onOpenProject(currentProject.id, next[0], next[1], next[2])}>{next[3]}<ArrowRight size={15}/></button><button className="secondary-button full" onClick={() => onOpenProject(currentProject.id, system.key, item.id)}>返回{item.label}列表</button></aside>
}

// 侧栏的二级视图名 → 分析页的视图键。认不出来的名字落到总览，
// 不留空白页：视图名改错了应该看到总览，而不是一片空。
// 投前入口的两个视图。认不出来的落到「结论」——那是开工前真正要读的一屏，
// 「历史模式」是回头看统计，不该在人没主动要求时先出现。
const preLaunchViews: Record<string, PreLaunchView> = {
  结论: 'conclusions',
  历史模式: 'patterns',
}

const analysisViews: Record<string, AnalysisView> = {
  指标总览: 'overview',
  素材对比: 'comparisons',
  趋势: 'trends',
  疲劳: 'fatigue',
  异常: 'anomalies',
  驱动因素: 'drivers',
}

// 素材入口的五个视图。数据接入和变量这两段是整页委托过去的，它们自己还有分段
// （数据源/导入任务/…、按素材类型），那些分段名不在这张表里——落到默认值即可，
// 委托过去的页面认得它们。
const assetsViews: Record<string, AssetsView> = {
  总览: 'overview',
  数据接入: 'intake',
  变量: 'features',
  找相似: 'similar',
  外部素材: 'external',
}

// 同理，认不出来的名字落到「本轮」——日常最常进的那一屏。
const reviewViews: Record<string, ReviewView> = {
  本轮: 'current',
  全部复盘: 'all',
  已沉淀经验: 'harvest',
}

// 经验入口的两个模式。认不出来的落到「查」：那是高频的一屏，
// 而「管」会摆出一排能改状态的按钮，不该在人没主动要求时出现。
const experienceViews: Record<string, ExperienceView> = {
  查经验: 'lookup',
  管经验: 'manage',
}

// 设置入口的四组。认不出来的落到「判定阈值」——那是这一版唯一新增的能力，
// 也是四组里唯一能改东西的一屏。
const settingsViews: Record<string, SettingsView> = {
  判定阈值: 'thresholds',
  数据体检: 'health',
  变量字典: 'dictionary',
  确认权限: 'permission',
}

export function ModulePage({
  system,
  item,
  contextId,
  objectId,
  routeView,
  strategyStage,
  strategyPanel,
  strategyResource,
  tourRunId,
  tourCase,
  onOpenProject,
  onOpenStrategyWorkspace,
}: {
  system: SystemDefinition
  item: NavItem
  contextId?: string
  objectId?: string
  routeView?: string
  strategyStage?: StrategyStage
  strategyPanel?: StrategyPanel
  strategyResource?: string
  tourRunId?: string
  tourCase?: string
  onOpenProject: OpenProject
  onOpenStrategyWorkspace: OpenStrategyWorkspace
}) {
  const normalizedRouteView = routeView
  const strategyWorkspaceLocation = useMemo<StrategyWorkspaceLocation>(() => ({
    stage: strategyStage ?? 'intake',
    panel: strategyPanel,
    resource: strategyResource,
  }), [strategyPanel, strategyResource, strategyStage])
  const [activeView, setActiveView] = useState(() => normalizedRouteView && item.views.includes(normalizedRouteView) ? normalizedRouteView : item.views[0])
  const [dataState, setDataState] = useState<DataState>('ready')
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [taskDialog, setTaskDialog] = useState<{ domain: 'strategy' | 'creative'; initialType?: BusinessTaskType } | null>(null)
  const { currentProject } = useProject()

  useEffect(() => { if (routeView && item.views.includes(routeView)) setActiveView(routeView) }, [item.views, routeView])

  const primaryAction = async () => {
    if (system.key === 'strategy' && item.id === 'tasks') {
      setTaskDialog({ domain: 'strategy', initialType: 'strategy' })
      return
    }
    if (system.key === 'creative' && item.id === 'tasks') {
      setTaskDialog({ domain: 'creative', initialType: 'creative' })
      return
    }
    setBusy(true)
    try {
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '保存失败，请在服务恢复后重试。')
    } finally {
      setBusy(false)
    }
  }

  let surface
  const taskDomain = system.key === 'strategy' || system.key === 'creative' ? system.key : null
  const taskCenter = item.id === 'tasks' && taskDomain !== null
  const specialized = system.key === 'strategy' && item.id === 'tasks' ? <KanonStrategyTaskCenter activeView={activeView} onOpenWorkspace={id => onOpenProject(currentProject.id, 'strategy', 'workspaces', id, '概览')} onRequestCreate={() => setTaskDialog({ domain: 'strategy', initialType: 'strategy' })}/>
    : taskCenter && taskDomain ? <TaskCenterPage state={dataState} domain={taskDomain} activeView={activeView} selectedId={objectId} onOpenTask={id => onOpenProject(currentProject.id, taskDomain, 'tasks', id, activeView)} onRequestCreate={() => setTaskDialog({ domain: taskDomain, initialType: taskDomain === 'strategy' ? 'strategy' : 'creative' })} onContinueTask={taskDomain === 'creative' ? task => { const destination = creativeTaskDestination(task); onOpenProject(currentProject.id, 'creative', destination.navId, task.id, destination.view) } : undefined} onOpenProject={onOpenProject}/>
    : system.key === 'strategy' && item.id === 'workspaces' ? <Suspense fallback={<div className="kanon-strategy-state" role="status">正在加载策略工作区…</div>}>
      <StrategyWorkspaceRoute
        location={strategyWorkspaceLocation}
        projectId={currentProject.id}
        workspaceId={objectId}
        onNavigate={(workspaceId, location, replace) => onOpenStrategyWorkspace(currentProject.id, workspaceId, location, replace)}
        onOpenCreative={(navId, view, contextId) => onOpenProject(currentProject.id, 'creative', navId, undefined, view, contextId)}
      />
    </Suspense>
    : system.key === 'strategy' && item.id === 'briefs' ? <KanonBriefCenter activeView={activeView} onOpenWorkspace={(id, view) => onOpenProject(currentProject.id, 'strategy', 'workspaces', id, view)}/>
    : system.key === 'strategy' && item.id === 'strategies' ? <KanonStrategyLibrary activeView={activeView} onOpenWorkspace={(id, view) => onOpenProject(currentProject.id, 'strategy', 'workspaces', id, view)}/>
    : system.key === 'strategy' && item.id === 'research' ? <KanonResearchEvidenceCenter activeView={activeView}/>
    : system.key === 'strategy' && item.id === 'operations' ? <KanonSkillsOperations activeView={activeView}/>
    : system.key === 'strategy' && item.id === 'reviews' ? <KanonReviewCenter activeView={activeView} onOpenReview={() => onOpenProject(currentProject.id, 'strategy', 'workspaces', undefined, '评审')}/>
    : system.key === 'creative' && item.id === 'image-text' ? <ImageTextCreationPage
        state={dataState}
        activeTaskId={contextId ?? objectId}
        onTaskCreated={id => onOpenProject(currentProject.id, 'creative', 'image-text', undefined, activeView, id)}
        onBack={() => onOpenProject(currentProject.id, 'creative', 'image-text', undefined, activeView)}
      />
    : system.key === 'creative' && item.id === 'video' ? <VideoCreationPage state={dataState} activeView={activeView} activeTaskId={contextId ?? objectId} onOpenTask={id => onOpenProject(currentProject.id, 'creative', 'tasks', id)} onOpenBrandIntake={id => onOpenProject(currentProject.id, 'creative', 'video', undefined, '品牌广告', `intake:${id}`)} onOpenBrandTask={id => onOpenProject(currentProject.id, 'creative', 'video', undefined, '品牌广告', `task:${id}`)} onOpenEditTask={id => onOpenProject(currentProject.id, 'creative', 'video', undefined, '素材剪辑', id)}/>
    : system.key === 'creative' && item.id === 'production' ? <Suspense fallback={<div className="pc-state" role="status">正在加载制作中心…</div>}>
      <ProductionCenterPage
        activeView={activeView}
        objectId={objectId}
        onOpenRun={ref => onOpenProject(currentProject.id, 'creative', 'production', `${ref.source}~${ref.id}`, activeView)}
        onCloseRun={() => onOpenProject(currentProject.id, 'creative', 'production', undefined, activeView)}
        onOpenSource={run => {
          if (!run.source_task) return
          if (run.source_task.object_type === 'edit_task') onOpenProject(currentProject.id, 'creative', 'video', undefined, '素材剪辑', run.source_task.object_id)
          else onOpenProject(currentProject.id, 'creative', 'tasks', run.source_task.object_id)
        }}
      />
    </Suspense>
    : system.key === 'creative' && item.id === 'reviews' ? <MaterialCheckWorkspace state={dataState} activeView={activeView} objectId={objectId} onOpenProject={onOpenProject}/>
    : system.key === 'insight' && item.id === 'prelaunch' ? <PreLaunchPage state={dataState} view={preLaunchViews[activeView] ?? 'conclusions'}/>
    : system.key === 'insight' && item.id === 'analysis' ? <AnalysisPage state={dataState} view={analysisViews[activeView] ?? 'overview'} onOpenExperiments={() => onOpenProject(currentProject.id, 'insight', 'experiments')}/>
    : system.key === 'insight' && item.id === 'assets' ? <AssetsPage
      state={dataState}
      view={assetsViews[activeView] ?? 'overview'}
      onOpenView={setActiveView}
      onOpenLibrary={() => onOpenProject(currentProject.id, 'creative', 'production', undefined, '源素材')}
      onOpenAnalysis={() => onOpenProject(currentProject.id, 'insight', 'analysis')}/>
    : system.key === 'insight' && item.id === 'experience' ? <ExperiencePage state={dataState} view={experienceViews[activeView] ?? 'lookup'}/>
    : system.key === 'insight' && item.id === 'review' ? <ReviewPage state={dataState} view={reviewViews[activeView] ?? 'current'} objectId={objectId}/>
    : system.key === 'insight' && item.id === 'miyun-materials' ? <MiyunMaterialsPage state={dataState} activeView={activeView}/>
    : system.key === 'insight' && item.id === 'experiments' ? <ExperimentCenterPage state={dataState} activeView={activeView}/>
    : system.key === 'insight' && item.id === 'settings' ? <SettingsPage state={dataState} view={settingsViews[activeView] ?? 'thresholds'}/>
    : system.key === 'delivery' && item.id === 'plans' ? <DeliveryPlanPage state={dataState}/>
    : system.key === 'delivery' && item.id === 'configuration' ? <DeliveryConfigurationPage state={dataState} activeView={activeView} tourRunId={tourRunId} tourCase={tourCase}/>
    : system.key === 'delivery' && item.id === 'approvals' ? <ApprovalCenterPage state={dataState} tourCase={tourCase} tourRunId={tourRunId} selectedChangeSetId={objectId}/>
    : system.key === 'delivery' && item.id === 'execution' ? <Suspense fallback={<div className="page-notice" role="status">正在加载受控执行中心…</div>}>
      <ControlledExecutionWorkspace projectId={currentProject.id} runId={objectId} activeView={activeView}/>
    </Suspense>
    : system.key === 'delivery' && item.id === 'platform-entities' ? <Suspense fallback={<div className="page-notice" role="status">正在加载项目与单元…</div>}>
      <DeliveryPlatformEntitiesPage projectId={currentProject.id} activeView={activeView}/>
    </Suspense>
    : system.key === 'delivery' && item.id === 'monitoring' ? <DeliveryMonitoringPage tourCase={tourCase}/>
    : system.key === 'delivery' && item.id === 'optimization' ? <DeliveryOptimizationPage state={dataState} activeView={activeView} tourRunId={tourRunId} tourCase={tourCase}/>
    : system.key === 'delivery' && item.id === 'evidence' ? <AuditEvidenceSurface/>
    : system.key === 'delivery' && item.id === 'products' ? <Suspense fallback={<div className="page-notice" role="status">正在加载产品目录…</div>}><ProductsPage activeView={activeView}/></Suspense>
    : null
  if (specialized) surface = specialized
  else {
    const genericSurface = item.layout === 'workspace' ? <WorkspaceSurface item={item} activeView={activeView}/> : item.layout === 'analysis' ? <AnalysisSurface item={item} activeView={activeView}/> : item.layout === 'editor' ? <EditorSurface item={item} activeView={activeView}/> : item.layout === 'table' ? <TableSurface item={item} activeView={activeView} onOpenRecord={id => onOpenProject(currentProject.id, system.key, item.id, id, activeView)}/> : <OperationsSurface item={item}/>
    surface = <StateBoundary
      state={dataState}
      contextLabel={`${system.label} / ${item.label}`}
      emptyTitle={`${item.label}暂无当前 Project 数据`}
      emptyDetail="这里不会用示例内容冒充已保存结果。请先完成上游步骤、创建业务对象，或切换到已有数据的 Project。"
      errorDetail="页面数据读取失败，当前内容不会被覆盖。请确认本地 MVP API 正常运行后重新加载。"
      forbiddenDetail="当前角色不能查看或操作此页面，请联系 Project 管理员授予相应权限。"
      createLabel="创建业务对象"
      onRetry={() => setDataState('ready')}
      onCreate={primaryAction}
    >{genericSurface}</StateBoundary>
  }

  const actionLabel = system.key === 'strategy' && item.id === 'tasks' ? '新建策略任务'
    : system.key === 'creative' && item.id === 'tasks' ? '新建创意任务'
    // 素材洞察的系统设置整页只读，没有一处可保存，所以这里不给「保存配置」按钮——
    // 留一个点下去什么都不发生的按钮，会被读成「保存失败」而不是「不需要保存」。
    : undefined
  const taskCreated = (task: BusinessTaskRecord) => {
    setTaskDialog(null)
    setNotice(`${task.name} 已写入服务端并关联当前 Project`)
    onOpenProject(currentProject.id, system.key, 'tasks', task.id)
  }
  const strategyTaskCreated = (bundle: StrategyTaskBundle) => {
    setTaskDialog(null)
    setNotice(`${bundle.workspace.name} 已创建，工作区、对话与 Brief 已持久化`)
    onOpenProject(currentProject.id, 'strategy', 'workspaces', bundle.workspace.id, '概览')
  }

  const projectProgress = calculateProjectProgress(currentProject)
  const showObjectDetail = Boolean(objectId && !taskCenter && !(system.key === 'creative' && (item.id === 'reviews' || item.id === 'production')) && !(system.key === 'strategy' && item.id === 'workspaces') && !(system.key === 'delivery' && item.id === 'approvals'))
  const isStrategyWorkspace = system.key === 'strategy' && item.id === 'workspaces'
  const hasImplementedHeaderViews = !(system.key === 'delivery' && (item.id === 'plans' || item.id === 'approvals' || item.id === 'monitoring'))
  const changeView = (view: string) => {
    setActiveView(view)
    onOpenProject(currentProject.id, system.key, item.id, isStrategyWorkspace ? objectId : undefined, view, undefined, tourRunId, tourCase)
  }
  const deliveryEnvironment = system.key === 'delivery' && (tourRunId || tourCase) ? <DeliveryMockEnvironmentBanner/> : null
  const pageSurface = <>{deliveryEnvironment}<div className={showObjectDetail ? 'page-surface with-object-detail' : 'page-surface'}>{surface}{showObjectDetail ? <ObjectDetail system={system} item={item} objectId={objectId!} onOpenProject={onOpenProject}/> : null}</div></>

  const strategyStatusLabel = isStrategyWorkspace ? strategyStageLabel(strategyStage ?? 'intake') : activeView
  return <div className={`module-page page-frame layout-${item.layout}${isStrategyWorkspace ? ' strategy-workspace-page' : ''}`}>{isStrategyWorkspace ? null : <PageHeader item={item} activeView={activeView} onViewChange={changeView} onPrimaryAction={() => { void primaryAction() }} busy={busy} actionLabel={actionLabel} showTabs={hasImplementedHeaderViews} showDescription={!(system.key === 'delivery' && item.id === 'configuration')}/>}{import.meta.env.VITE_SHOW_STATE_PREVIEW === 'true' ? <StatePreview value={dataState} onChange={setDataState}/> : null}{notice ? <div className="page-notice" role="status"><CircleCheck size={16}/>{notice}<button aria-label="关闭提示" onClick={() => setNotice('')}>×</button></div> : null}{isStrategyWorkspace ? <div className="strategy-workspace-shell">{pageSurface}</div> : pageSurface}{system.key === 'strategy' && specialized ? <footer className="statusbar"><span>Project：{currentProject.name}</span><span>模块：{item.label}</span><span>阶段：{strategyStatusLabel}</span><span>状态源：Strategy 服务</span><strong>持久化：已启用</strong></footer> : system.key === 'strategy' ? <footer className="statusbar"><span>Project：{currentProject.name}</span><span>模块：{item.label}</span><span>视图：{activeView}</span><span>状态源：通用页面</span><strong>尚未接入专用数据源</strong></footer> : <footer className="statusbar"><span>Project：{currentProject.name}</span><span>阶段：{projectProgress.stageLabel}</span><span>进度：{progressPercentLabel(projectProgress)}</span><span>更新时间：{currentProject.updatedAt}</span><strong>进度状态：{progressStatusLabel(projectProgress)}</strong></footer>}{taskDialog?.domain === 'strategy' ? <KanonStrategyTaskDialog onClose={() => setTaskDialog(null)} onCreated={strategyTaskCreated}/> : taskDialog ? <TaskCreateDialog domain={taskDialog.domain} initialType={taskDialog.initialType} onClose={() => setTaskDialog(null)} onCreated={taskCreated}/> : null}</div>
}
