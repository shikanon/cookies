import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import {
  ArrowRight,
  Check,
  ChevronDown,
  CircleAlert,
  CircleCheck,
  ClipboardCheck,
  Download,
  ExternalLink,
  FileText,
  Film,
  Image,
  LoaderCircle,
  Play,
  RotateCcw,
  Save,
  Scissors,
  Send,
  ShieldCheck,
  Sparkles,
  ThumbsDown,
  ThumbsUp,
  Upload,
  Video,
  WandSparkles,
} from 'lucide-react'
import { useProject } from '../context/ProjectContext'
import { useModelConfig } from '../context/ModelConfigContext'
import { commerceHookTemplates, commerceTemplateApiId, guerlainPromptCopy, hookStoryboard } from '../data/commerceHooks'
import { api, buildHitAnalysisInput, buildLocalHitAnalysis, buildVideoReplicationPrompt, type ApiAdAccountBinding, type ApiAgencyWorkbench, type ApiArtifact, type ApiAssetFeature, type ApiAssetVersionPointer, type ApiBrandBriefAssetCandidate, type ApiCommercePrerollWorkspace, type ApiCreativeDirection, type ApiCreativeDirectionBatch, type ApiCreativeIntakeBootstrap, type ApiCreativeSourceOption, type ApiCreativeTaskSummary, type ApiGenerationJob, type ApiHitAnalysis, type ApiMaterialConfirmation, type ApiPreparedCommercePreroll, type ApiPrerollScope, type ApiProjectMediaAsset, type ApiShortDramaGenerationConfig, type ApiShortDramaHookStrategy, type ApiShortDramaPaceProfile, type ApiShortDramaPrerollCandidate, type ApiShortDramaPrerollPlan, type ApiShortDramaPrerollWorkspace, type ApiShortDramaStoryContext, type ApiShortDramaSubtitleStyle, type ApiTaskStrategyCreativeIntake, type ApiViralRemakeWorkspace, type ApiVideoPromptDimension, type ApiVideoReplicationPrompt } from '../data/api'
import { resolveBrandVideoRouteTarget } from '../features/creative/brandVideoRoute'
import { extractAndUploadBrandBriefAssets } from '../features/brand-film/pdfBriefAssets'
import { activeBrandVideoTasks, availableBrandDirections, brandDirectionFailureMessage, brandVideoTaskStatusLabel, isBrandDirectionGenerating } from '../features/creative/brandDirectionGeneration'
import type { ArtifactKey, BusinessTaskType, DataState } from '../types'
import { deliveryApi, type DeliveryChangeSet } from '../api/delivery'
import { StateBoundary } from './StateBoundary'
import { shortId } from '../data/shortId'
import { industryProfile } from '../data/industry-profiles'
import { findLocalShortDramaBrief, localShortDramaBriefs, shortDramaVideoLabel } from '../data/shortDramaBriefs'
import { GamePrerollWorkspace } from './GamePrerollWorkspace'
import { BrandFilmWorkspace } from './BrandFilmWorkspace'

const creativeTaskDisplayName = (task: ApiCreativeTaskSummary) => task.display_name && !task.display_name.startsWith('未命名')
  ? task.display_name
  : task.direction.focus || task.direction.concept || '未命名品牌广告'
import { ShortDramaPrerollWorkspace } from '../features/short-drama-preroll-v2/ShortDramaPrerollWorkspace'
import { CommercePrerollWorkspace } from '../features/commerce-preroll-v2'
import { editingApi } from '../features/video-editing/api'
import {
  TaskStrategyHandoffBanner,
  taskStrategyPerformanceMode,
  useTaskStrategyCreativeIntake,
  useTaskStrategyTaskHandoffDetail,
} from '../features/creative/TaskStrategyHandoff'

const AINativeAdWorkspace = lazy(() => import('../features/ai-native-ad/AINativeAdWorkspace').then(module => ({ default: module.AINativeAdWorkspace })))
const VideoEditingWorkspaceV2 = lazy(() => import('../features/video-editing/VideoEditingWorkspace').then(module => ({ default: module.VideoEditingWorkspaceV2 })))

export { DeliveryPlanLifecyclePage as DeliveryPlanPage } from './DeliveryPlanLifecyclePage'
export { DeliveryApprovalCenterPage as ApprovalCenterPage } from './DeliveryApprovalCenterPage'

function IndustrySchema({ module, profile, industry }: { module: string; industry: string; profile: { fields: string[]; format: string } }) {
  return <section className="industry-schema" aria-label={`${industry}${module}配置`}>
    <span>{industry} · {module}</span><b>{profile.format}</b>
    <div>{profile.fields.map(field => <small key={field}>{field}</small>)}</div>
  </section>
}

export function ArtifactFlow({ compact = false }: { compact?: boolean }) {
  const { currentProject } = useProject()
  const order: ArtifactKey[] = ['brief', 'strategy', 'creative', 'insight', 'delivery']
  return <div className={compact ? 'artifact-flow compact' : 'artifact-flow'} aria-label="Project 产物链路">{order.map((key, index) => { const artifact = currentProject.artifacts[key]; return <div className="artifact-node" key={key}><span>{String(index + 1).padStart(2, '0')}</span><div><b>{artifact.label} {artifact.version}</b><small>{artifact.status} · {artifact.owner}</small><small>{artifact.sourceVersion ?? `更新于 ${artifact.updatedAt}`}</small></div>{index < order.length - 1 ? <ArrowRight size={14}/> : null}</div> })}</div>
}

export function LegacyImageTextCreationPage({ state, activeTaskId }: { state: DataState, activeTaskId?: string }) {
  const { currentProject, reloadProjects, updateArtifact } = useProject()
  const { providers } = useModelConfig()
  const [selected, setSelected] = useState(0)
  const [channel, setChannel] = useState('小红书 4:5')
  const [headline, setHeadline] = useState('看得见的精度，兑现你的创新。')
  const [version, setVersion] = useState(8)
  const [notice, setNotice] = useState('')
  const [job, setJob] = useState<ApiGenerationJob | null>(null)
  const [confirmedBriefId, setConfirmedBriefId] = useState('')
  const activeTask = currentProject.tasks.find(task => task.id === activeTaskId)
  const handoffDetail = useTaskStrategyTaskHandoffDetail(currentProject.id, activeTaskId)
  const inheritedFocus = handoffDetail?.task.direction.focus
  const pages = ['封面主张', '精度证据', '制造场景', '行动引导']
  const configuredProvider = providers.find(provider => provider.status === '已配置')
  const save = async () => {
    const nextVersion = `v1.${version + 1}`
    try {
      await updateArtifact('creative', { version: nextVersion, status: '制作中', sourceVersion: `策略 ${currentProject.artifacts.strategy.version}`, summary: `${channel} 图文 4 页，品牌检查通过` })
      setVersion(value => value + 1)
      setNotice(`已保存为 ${nextVersion}`)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '保存创意版本失败，请重试。')
    }
  }
  useEffect(() => {
    if (inheritedFocus) setHeadline(inheritedFocus.slice(0, 24))
  }, [inheritedFocus])
  useEffect(() => {
    let active = true
    void Promise.all([api.listArtifacts(currentProject.id), api.listJobs(currentProject.id)]).then(([artifacts, jobs]) => {
      const latest = jobs.filter(candidate => candidate.artifactKind === 'image').at(-1)
      const brief = artifacts.filter(artifact => artifact.kind === 'brief' && artifact.status === 'ready').at(-1)
      if (active) {
        setJob(latest ?? null)
        setConfirmedBriefId(brief?.id ?? '')
      }
    }).catch(() => undefined)
    return () => { active = false }
  }, [currentProject.id])
  useEffect(() => {
    if (!job || !['queued', 'running'].includes(job.status)) return
    const timer = window.setInterval(() => {
      void api.getJob(job.id).then(next => {
        setJob(next)
        if (next.status === 'succeeded') {
          void reloadProjects()
          setNotice('图片生成完成，稳定资产已关联到当前 Project。')
        }
      }).catch(cause => setNotice(cause instanceof Error ? cause.message : '任务状态读取失败'))
    }, 1500)
    return () => window.clearInterval(timer)
  }, [job, updateArtifact])
  const generateImage = async () => {
    const prompt = `${headline}。${channel}，工业制造品牌图文主视觉，产品精度证据，品牌安全区清晰，中文排版。`
    if (!confirmedBriefId) {
      setNotice('请先在需求中心确认 Brief，再生成当前主视觉。')
      return
    }
    try {
      const next = await api.createMedia(currentProject.id, 'image', prompt, confirmedBriefId)
      setJob(next)
      setNotice(next.status === 'succeeded' ? '图片生成完成，资产已保存。' : '图片生成任务已创建，正在轮询。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '创建图片生成任务失败，请重试。')
    }
  }
  return <StateBoundary state={state} onRetry={() => setNotice('已重新加载')} onCreate={() => setNotice('已创建空白画板')}><div className="image-editor-specialized">
    <aside className="creative-structure"><div className="surface-toolbar"><h3>图文结构</h3><button aria-label="新增图文页面"><Image size={16}/></button></div>{pages.map((page, index) => <button key={page} className={selected === index ? 'creative-page active' : 'creative-page'} onClick={() => setSelected(index)}><span>{String(index + 1).padStart(2, '0')}</span><b>{page}</b><small>{index === 0 ? '主视觉' : index === 3 ? 'CTA' : '内容页'}</small></button>)}<div className="version-block"><span>来源</span><b>{currentProject.artifacts.strategy.version}</b><small>{currentProject.artifacts.strategy.summary}</small></div></aside>
    <section className="image-canvas-workspace"><div className="canvas-toolbar light"><span>{activeTask ? `${activeTask.name} · 图文 v1.${version}` : `${currentProject.name} · 图文 v1.${version}`}</span><div><button onClick={() => setNotice('预览链接已生成')}><ExternalLink size={14}/>预览</button><button onClick={() => setNotice('PNG 导出任务已创建')}><Download size={14}/>导出</button></div></div>{handoffDetail ? <TaskStrategyHandoffBanner intake={handoffDetail.intake}/> : activeTask ? <div className="creative-task-banner"><span>统一创意任务入口</span><b>{activeTask.name}</b><small>{activeTask.objective}</small></div> : null}<div className="portrait-stage"><div className="social-poster"><img src="/assets/white-precision-cnc.png" alt="CNC 设备加工高精度金属零件"/><div className="poster-copy"><small>WHITE PRECISION</small><h2>{headline}</h2><p>±0.01mm 精度 · 98%+ 准时交付</p></div><span className="poster-index">0{selected + 1} / 04</span></div></div><div className="page-strip">{pages.map((page, index) => <button key={page} className={selected === index ? 'active' : ''} onClick={() => setSelected(index)}><span>{index + 1}</span>{page}</button>)}</div></section>
    <aside className="creative-inspector"><div className="surface-toolbar"><h3>页面属性</h3><span className="status success"><span/>品牌检查通过</span></div><label>渠道与画幅<select value={channel} onChange={event => setChannel(event.target.value)}><option>小红书 4:5</option><option>公众号 16:9</option><option>信息流 1:1</option></select></label><label>主标题<textarea value={headline} onChange={event => setHeadline(event.target.value)} maxLength={24}/><small>{headline.length} / 24 字</small></label><div className="check-list"><span><Check size={14}/>安全区未遮挡</span><span><Check size={14}/>核心信息有证据</span><span><Check size={14}/>品牌用语一致</span></div>{!configuredProvider ? <div className="model-required"><CircleAlert size={15}/><span>服务端尚未配置 ARK_API_KEY，无法发起图片生成。</span></div> : null}{!confirmedBriefId ? <div className="model-required"><CircleAlert size={15}/><span>请先在需求中心确认 Brief，系统才会允许生成图片。</span></div> : null}<button className="primary-button full" disabled={!configuredProvider || !confirmedBriefId || ['queued', 'running'].includes(job?.status ?? '')} onClick={() => void generateImage()}><WandSparkles size={15}/>{job && ['queued', 'running'].includes(job.status) ? '图片生成中…' : '生成当前主视觉'}</button><button className="secondary-button full" onClick={save}><Save size={15}/>保存新版本</button>{job ? <div className="inline-notice" role="status">任务 {shortId(job.id)} · {job.status} · {job.model ?? '模型待分配'}{job.diagnostic ? ` · ${job.diagnostic}` : ''}</div> : null}{notice ? <div className="inline-notice" role="status">{notice}</div> : null}</aside>
  </div></StateBoundary>
}

export { ImageTextWorkspacePage as ImageTextCreationPage } from './ImageTextWorkspacePage'

const prerollModes = [
  { id: 'short-drama', label: '短剧前贴', detail: '用人物冲突、风险升级和结果反转，在 6 秒内建立继续观看的理由。', guard: '人物连续性与静音可理解' },
  { id: 'game', label: '游戏前贴', detail: '用可读目标、失败瞬间和即时反馈建立挑战感，再衔接产品或正片。', guard: '玩法真实性与结果可读性' },
  { id: 'pre-roll', label: '电商前贴', detail: '理解原视频后生成独立的 6–10 秒高注意力开场。', guard: '商品保真与来源可追溯' },
]

const performanceSections = [
  { id: 'preroll', label: '前贴广告', detail: '短剧、游戏与电商短片段开场' },
  { id: 'viral-remake', label: '爆款复刻', detail: '结构拆解、品牌映射与原创改写' },
  { id: 'ai-native', label: 'AI 效果广告生成', detail: '从商品需求到完整广告视频' },
] as const

type PerformanceSectionId = (typeof performanceSections)[number]['id']

function initialPerformanceSection(): PerformanceSectionId {
  const section = typeof window === 'undefined' ? null : new URLSearchParams(window.location.search).get('section')
  return performanceSections.some(item => item.id === section) ? section as PerformanceSectionId : 'preroll'
}

function rememberPerformanceSection(section: PerformanceSectionId) {
  const search = new URLSearchParams(window.location.search)
  search.set('section', section)
  window.history.replaceState(null, '', `${window.location.pathname}?${search.toString()}${window.location.hash}`)
}

function initialPrerollMode() {
  if (typeof window === 'undefined') return 'short-drama'
  const search = new URLSearchParams(window.location.search)
  const mode = search.get('preroll')
  if (prerollModes.some(item => item.id === mode)) return mode as string
  return search.has('cpTask') || search.has('cpStep') ? 'pre-roll' : 'short-drama'
}

function rememberPrerollMode(mode: string) {
  const search = new URLSearchParams(window.location.search)
  search.set('preroll', mode)
  window.history.replaceState(window.history.state, '', `${window.location.pathname}?${search.toString()}${window.location.hash}`)
}

const preRollPresets = {
  'short-drama': {
    eyebrow: 'SHORT DRAMA HOOK',
    title: '“交期又要延期？”——先把冲突推到观众面前。',
    detail: '00:00 采购负责人收到延期消息；00:02 切入精密加工现场；00:05 用 98%+ 准时交付完成反转。',
    source: '客户访谈 × 交期风险策略',
    shots: ['消息弹窗与人物停顿', '切入高速 CNC 现场', '交付数据与品牌定格'],
  },
  game: {
    eyebrow: 'GAMEPLAY HOOK',
    title: '±0.01mm 精度挑战：你能一次过关吗？',
    detail: '00:00 显示目标公差；00:02 第一次加工失败；00:04 参数修正并成功过关；00:06 衔接真实制造画面。',
    source: '挑战机制 × 精度证据策略',
    shots: ['展示公差挑战目标', '失败反馈与进度掉落', '修正参数、一击过关'],
  },
}

export function VideoCreationPage({ state, activeView, activeTaskId, onOpenTask, onOpenBrandTask, onOpenEditTask }: { state: DataState, activeView: string, activeTaskId?: string, onOpenTask: (id: string) => void, onOpenBrandTask: (id: string) => void, onOpenEditTask: (id: string) => void }) {
  const { currentProject, createTask } = useProject()
  const industry = industryProfile(currentProject.industry)
  const [selectedSection, setSelectedSection] = useState<PerformanceSectionId>(initialPerformanceSection)
  const [selectedPreroll, setSelectedPreroll] = useState(initialPrerollMode)
  const [notice, setNotice] = useState('')
  const [brandIntake, setBrandIntake] = useState<ApiCreativeIntakeBootstrap | null>(null)
  const [brandDirectionBatch, setBrandDirectionBatch] = useState<ApiCreativeDirectionBatch | null>(null)
  const [brandDirections, setBrandDirections] = useState<ApiCreativeDirection[]>([])
  const [brandTask, setBrandTask] = useState<ApiCreativeTaskSummary | null>(null)
  const [brandBusy, setBrandBusy] = useState('')
  const [brandIntakeError, setBrandIntakeError] = useState('')
  const [brandIntakeRetry, setBrandIntakeRetry] = useState(0)
  const [brandTaskOptions, setBrandTaskOptions] = useState<ApiCreativeTaskSummary[]>([])
  const [brandIntakeOptions, setBrandIntakeOptions] = useState<ApiCreativeIntakeBootstrap[]>([])
  const [brandUploadBusy, setBrandUploadBusy] = useState(false)
  const [brandDuration, setBrandDuration] = useState(15)
  const [brandContextLoading, setBrandContextLoading] = useState(false)
  const [brandContextError, setBrandContextError] = useState('')
  const [brandContextRetry, setBrandContextRetry] = useState(0)
  const category = activeView === '品牌广告' ? 'brand' : activeView === '素材剪辑' ? 'editing' : 'performance'
  const activeTask = currentProject.tasks.find(task => task.id === activeTaskId)
  const activeTaskType = activeTask?.type
  const handoffIntake = useTaskStrategyCreativeIntake(
    currentProject.id,
    activeTaskId,
    Boolean(activeTaskId && !activeTask),
  )
  const expectsBrandIntake = category === 'brand' && Boolean(activeTaskId && !activeTask)
  const activePrerollMode = prerollModes.find(item => item.id === selectedPreroll) ?? prerollModes[0]
  const activePerformanceLabel = selectedSection === 'viral-remake' ? '爆款复刻' : selectedSection === 'ai-native' ? 'AI 效果广告生成' : activePrerollMode.label
  const selectLegacyPerformanceMode = (mode: string) => {
    if (mode === 'viral-remake') {
      setSelectedSection('viral-remake')
      return
    }
    if (prerollModes.some(item => item.id === mode)) {
      setSelectedSection('preroll')
      setSelectedPreroll(mode)
    }
  }
  useEffect(() => {
    if (new URLSearchParams(window.location.search).has('cpTask')) return
    if (!activeTaskType) return
    const modeByType: Partial<Record<BusinessTaskType, string>> = {
      short_drama_preroll: 'short-drama',
      game_preroll: 'game',
      commerce_preroll: 'pre-roll',
      viral_remake: 'viral-remake',
      video: 'short-drama',
    }
    const nextMode = modeByType[activeTaskType]
    if (nextMode) selectLegacyPerformanceMode(nextMode)
  }, [activeTaskType])
  useEffect(() => {
    if (!handoffIntake) return
    const nextMode = taskStrategyPerformanceMode(
      handoffIntake.request.task_strategy_input.business_code,
    )
    if (nextMode) selectLegacyPerformanceMode(nextMode)
    setNotice('任务策略已冻结到 Creative；请补齐生产素材和人工确认后继续。')
  }, [handoffIntake])
  useEffect(() => {
    if (category !== 'brand') return
    let active = true
    setBrandContextLoading(true)
    setBrandContextError('')
    void Promise.all([api.listCreativeTasks(currentProject.id, 30), api.listCreativeIntakes(currentProject.id, 60)])
      .then(([tasks, intakes]) => {
        if (!active) return
        setBrandTaskOptions(activeBrandVideoTasks(tasks.items))
        setBrandIntakeOptions(intakes.items.filter(intake => {
          if (intake.source !== 'strategy_package') return false
          try { resolveBrandVideoRouteTarget(intake); return true } catch { return false }
        }))
      })
      .catch(cause => {
        if (!active) return
        setBrandTaskOptions([])
        setBrandIntakeOptions([])
        setBrandContextError(cause instanceof Error ? cause.message : '品牌广告任务列表读取失败')
      })
      .finally(() => {
        if (active) setBrandContextLoading(false)
      })
    return () => { active = false }
  }, [activeTaskId, brandContextRetry, category, currentProject.id])
  useEffect(() => {
    if (category !== 'brand' || !activeTaskId || activeTask) {
      if (!activeTaskId || category !== 'brand') setBrandTask(null)
      setBrandIntake(null)
      setBrandDirectionBatch(null)
      setBrandDirections([])
      setBrandIntakeError('')
      return
    }
    let active = true
    setBrandIntakeError('')
    setBrandDirectionBatch(null)
    setBrandDirections([])
    setBrandTask(null)
    void api.getCreativeIntake(currentProject.id, activeTaskId)
      .then(async value => {
        if (!active) return
        if (value.source === 'strategy_package') {
          setBrandBusy('materialize')
          const target = resolveBrandVideoRouteTarget(value)
          const task = await api.createBrandFilmTaskFromIntake(currentProject.id, value.id, target.selectedRouteId, target.channel)
          if (!active) return
          setNotice('策略交接已绑定到品牌广告任务，接下来先确认 Brief，再在品牌模块内生成创意候选。')
          onOpenBrandTask(task.id)
          return
        }
        setBrandIntakeError('当前交接不是可用的品牌策略包来源。')
      })
      .catch(async cause => {
        try {
          const detail = await api.getCreativeTaskHandoffDetail(currentProject.id, activeTaskId)
          if (!active || detail.task.format !== 'video') return
          setBrandTask(detail.task as unknown as ApiCreativeTaskSummary)
          setNotice('品牌视频任务已恢复，可继续完成 Brief、创意、分镜、生成与声音。')
        } catch {
          if (active) {
            const message = cause instanceof Error ? cause.message : '品牌策略交接读取失败'
            setBrandIntakeError(message)
            setNotice(message)
          }
        }
      })
      .finally(() => { if (active) setBrandBusy('') })
    return () => { active = false }
  }, [activeTask, activeTaskId, brandIntakeRetry, category, currentProject.id, onOpenBrandTask])
  useEffect(() => {
    if (!brandIntake || !isBrandDirectionGenerating(brandDirectionBatch)) return
    let active = true
    let timer: ReturnType<typeof setTimeout> | undefined
    const poll = async () => {
      try {
        const batch = await api.getLatestCreativeDirectionBatch(currentProject.id, brandIntake.id)
        if (!active || !batch) return
        setBrandDirectionBatch(batch)
        if (batch.status === 'ready') {
          setBrandDirections(availableBrandDirections(batch))
          setNotice('三个品牌方向已通过质量门，请选择一个进入视频任务。')
          return
        }
        if (batch.status === 'failed') {
          setBrandDirections([])
          setNotice(brandDirectionFailureMessage(batch.failure_code))
          return
        }
        timer = setTimeout(poll, 2000)
      } catch (cause) {
        if (!active) return
        setNotice(cause instanceof Error ? cause.message : '品牌方向状态刷新失败，系统稍后会继续重试。')
        timer = setTimeout(poll, 4000)
      }
    }
    timer = setTimeout(poll, 1200)
    return () => {
      active = false
      if (timer) clearTimeout(timer)
    }
  }, [brandDirectionBatch?.batch_id, brandDirectionBatch?.status, brandIntake?.id, currentProject.id])
  const generateBrandDirections = async () => {
    if (!brandIntake) return
    setBrandBusy('generate')
    setNotice('正在生成品牌创意领地；系统会自动拒绝同质化或效果广告式方案。')
    try {
      const batch = await api.generateCreativeDirections(currentProject.id, brandIntake.id)
      setBrandDirectionBatch(batch)
      setBrandDirections(availableBrandDirections(batch))
      setNotice(batch.status === 'ready'
        ? '三个品牌方向已通过质量门，请选择一个进入视频任务。'
        : '生成任务已进入后台队列，刷新或离开页面不会中断。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '品牌方向生成失败')
    } finally {
      setBrandBusy('')
    }
  }
  const confirmBrandDirection = async (direction: ApiCreativeDirection) => {
    if (!brandIntake) return
    setBrandBusy(direction.direction_id)
    try {
      const confirmed = await api.confirmCreativeDirection(currentProject.id, direction.direction_id)
      const target = resolveBrandVideoRouteTarget(brandIntake)
      const task = await api.createBrandVideoTaskFromDirection(
        currentProject.id,
        brandIntake.id,
        confirmed.direction_id,
        target.selectedRouteId,
        target.channel,
      )
      setBrandTask(task)
      setNotice('品牌方向已确认，真实视频任务已创建并保留完整策略血缘。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '品牌方向确认失败')
    } finally {
      setBrandBusy('')
    }
  }
  const startBrandIntake = async (intake: ApiCreativeIntakeBootstrap) => {
    setBrandBusy(intake.id)
    try {
      const target = resolveBrandVideoRouteTarget(intake)
      const task = await api.createBrandFilmTaskFromIntake(currentProject.id, intake.id, target.selectedRouteId, target.channel)
      setNotice('策略已绑定，正在进入品牌广告制作。')
      onOpenBrandTask(task.id)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '策略绑定失败')
    } finally { setBrandBusy('') }
  }
  const uploadBrandBrief = async (file?: File) => {
    if (!file) return
    if (!/\.(pdf|docx|md)$/i.test(file.name)) {
      setNotice('请上传 PDF、DOCX 或 Markdown Brief。')
      return
    }
    setBrandUploadBusy(true)
    try {
      let document = await api.uploadKnowledgeDocument(currentProject.id, file)
      setNotice(document.status === 'ready' ? 'Brief 已解析，正在创建品牌广告任务。' : 'Brief 已安全入库，正在解析正文…')
      for (let attempt = 0; document.status === 'parse_queued' && attempt < 45; attempt += 1) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        document = await api.getKnowledgeDocument(currentProject.id, document.id)
      }
      if (document.status !== 'ready') throw new Error(document.parse_error_message || 'Brief 解析尚未完成，请稍后重试。')
      let extractedAssets: ApiBrandBriefAssetCandidate[] = []
      if (document.mime_type === 'application/pdf') {
        setNotice('Brief 正文已解析，正在提取商品正面图与品牌 Logo…')
        try {
          extractedAssets = await extractAndUploadBrandBriefAssets(currentProject.id, document)
        } catch (cause) {
          setNotice(cause instanceof Error ? `正文已解析，但图片自动提取失败：${cause.message}。进入后仍可人工补充。` : '正文已解析，但图片自动提取失败；进入后仍可人工补充。')
        }
      }
      const intake = await api.createManualBrandFilmIntake(currentProject.id, document, brandDuration, extractedAssets)
      const task = await api.createBrandFilmTaskFromIntake(currentProject.id, intake.id, 'route_fixture_brand_video_guerlain_v1', 'douyin')
      setNotice('PDF Brief 已解析并建立可追溯任务，正在进入 Brief 确认。')
      onOpenBrandTask(task.id)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : 'PDF Brief 导入失败')
    } finally { setBrandUploadBusy(false) }
  }
  const create = async () => {
    const name = category === 'performance' ? activePerformanceLabel : category === 'brand' ? '品牌广告' : '素材剪辑 EditTask'
    const type: BusinessTaskType = category === 'brand' ? 'brand_video'
      : category === 'editing' ? 'video_edit'
      : selectedSection === 'viral-remake' ? 'viral_remake'
      : activePrerollMode.id === 'short-drama' ? 'short_drama_preroll'
      : activePrerollMode.id === 'game' ? 'game_preroll'
      : 'commerce_preroll'
    try {
      const task = await createTask({
        type,
        name: `${currentProject.name} · ${name}`,
        objective: `${currentProject.goal}；继承策略 ${currentProject.artifacts.strategy.version} 与品牌约束。`,
      })
      setNotice(`${name}创作任务已写入服务端；已保留 Project、策略、来源与版本链。`)
      onOpenTask(task.id)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '创建创作任务失败，请重试。')
    }
  }
  const openCreativeTaskInEditor = async () => {
    if (!activeTaskId) return
    try {
      const editTask = await editingApi.openCreativeVersion(currentProject.id, activeTaskId)
      onOpenEditTask(editTask.id)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '广告成片暂时无法进入素材剪辑')
    }
  }
  const title = category === 'performance' ? '效果广告，以可测试的转化表达组织创作。' : category === 'brand' ? '品牌广告，从 Brief 确认到剧本分镜形成可追溯闭环。' : '素材剪辑，将已授权素材组织为可交付的视频版本。'
  const description = category === 'performance' ? '选择一种生成类型，系统会继承策略、品牌规则、渠道规格与来源授权。' : category === 'brand' ? '从 Brief、创意与分镜，到生成锁定、质量确认和版本交付，形成可追溯的品牌广告制作闭环。' : '独立 EditTask 可从品牌、效果任务或存量项目素材进入；字幕、音频与转场在编辑器内完成。'
  return <StateBoundary state={state} onRetry={() => setNotice('创作配置已重新加载')} onCreate={() => { void create() }}><section className="video-creation-workspace">
    <header className="video-workspace-header"><div><span className="section-label">视频创作 · {activeView}</span><h2>{title}</h2><p>{description}</p>{handoffIntake ? <TaskStrategyHandoffBanner intake={handoffIntake}/> : activeTask ? <div className="creative-task-banner compact"><span>统一创意任务入口</span><b>{activeTask.name}</b><small>{activeTask.objective}</small></div> : null}</div>{category === 'performance' && selectedSection !== 'ai-native' ? <button className="primary-button" onClick={() => void create()}><Video size={16}/>新建{activePerformanceLabel}</button> : null}</header>
    {category !== 'editing' && activeTaskId ? <div className="creative-task-banner compact"><span>成片后续处理</span><b>将当前广告成片带入素材剪辑</b><small>只有已冻结且已入库的最终视频可以进入；原资产不会被覆盖。</small><button className="secondary-button" onClick={() => void openCreativeTaskInEditor()}><Scissors size={15}/>进入素材剪辑</button></div> : null}
    {category !== 'brand' ? <><IndustrySchema module="创意创作" industry={industry.label} profile={industry.creative}/><ProjectMediaContext /></> : null}
    {category === 'performance' ? <>
      <div className="performance-mode-tabs level-one" role="tablist" aria-label="效果广告一级模块">{performanceSections.map(section => <button key={section.id} id={`performance-section-${section.id}`} role="tab" aria-selected={selectedSection === section.id} className={selectedSection === section.id ? 'active' : ''} onClick={() => { setSelectedSection(section.id); rememberPerformanceSection(section.id); setNotice('') }}><b>{section.label}</b><small>{section.detail}</small></button>)}</div>
      {selectedSection === 'preroll' ? <>
        <div className="preroll-subnav">
          <span className="preroll-subnav-label"><b>前贴广告</b><i>/</i>选择类型</span>
          <div className="performance-mode-tabs preroll-mode-tabs" role="tablist" aria-label="前贴广告类型">{prerollModes.map(mode => <button key={mode.id} id={`preroll-mode-${mode.id}`} role="tab" title={mode.guard} aria-selected={selectedPreroll === mode.id} className={selectedPreroll === mode.id ? 'active' : ''} onClick={() => { setSelectedPreroll(mode.id); rememberPrerollMode(mode.id); setNotice('') }}><b>{mode.label}</b></button>)}</div>
        </div>
        {selectedPreroll === 'pre-roll' ? <CommercePrerollWorkspace onNotice={setNotice}/> : selectedPreroll === 'game' ? <GamePrerollWorkspace onNotice={setNotice}/> : <ShortDramaPrerollWorkspace onNotice={setNotice} onOpenEditTask={onOpenEditTask}/>}
      </> : selectedSection === 'viral-remake' ? <ViralRemixWorkspace handoffIntake={handoffIntake ?? undefined} onNotice={setNotice}/> : <Suspense fallback={<div className="ai-native-feature-loading">正在加载 AI 效果广告工作台…</div>}><AINativeAdWorkspace projectId={currentProject.id} onNotice={setNotice}/></Suspense>}
    </> : category === 'brand' && brandTask ? <BrandFilmWorkspace taskId={brandTask.id} taskOptions={brandTaskOptions} onOpenTask={onOpenBrandTask} onCreateNew={() => onOpenBrandTask('')} onNotice={setNotice}/>
      : category === 'brand' && !activeTaskId ? <section className="brand-creation-hub" aria-labelledby="brand-hub-title">
        <header><div><span className="section-label">BRAND CREATION HUB</span><h2 id="brand-hub-title">从已确认策略开始，也可以直接导入 Brief</h2><p>策略与任务彼此独立保存。选择来源后才会创建或恢复对应的品牌广告任务。</p></div><span>{brandIntakeOptions.length} 个可用策略 · {brandTaskOptions.length} 个制作任务</span></header>
        {brandContextLoading ? <div className="brand-hub-loading"><LoaderCircle className="spin" size={20}/>正在同步策略与任务…</div> : brandContextError ? <div className="brand-hub-error"><CircleAlert size={20}/><span>{brandContextError}</span><button className="secondary-button" onClick={() => setBrandContextRetry(value => value + 1)}>重试</button></div> : <div className="brand-hub-grid">
          <div className="brand-source-panel"><div className="brand-hub-panel-title"><div><span className="section-label">01 · CHOOSE A SOURCE</span><h3>新建品牌广告创作</h3></div><small>策略交接或独立 Brief</small></div>
            <div className="brand-strategy-list">{brandIntakeOptions.length ? brandIntakeOptions.map((intake, index) => <article key={intake.id}><div className="brand-strategy-mark">{String(index + 1).padStart(2, '0')}</div><div><span>已确认策略</span><h4>{intake.base_handoff?.creative_view?.communication?.single_minded_proposition || intake.request?.core_message || '品牌传播策略'}</h4><p>{intake.base_handoff?.creative_view?.objective?.statement || intake.request?.objective || '策略事实、品牌边界与渠道路线已冻结。'}</p><small>{intake.id}</small></div><button className="primary-button" disabled={Boolean(brandBusy)} onClick={() => void startBrandIntake(intake)}>{brandBusy === intake.id ? <LoaderCircle className="spin" size={14}/> : <ArrowRight size={14}/>}开始创作</button></article>) : <div className="brand-source-empty"><Sparkles size={20}/><div><b>暂无可用的品牌策略</b><p>策略模块完成“品牌广告”交接后，会自动出现在这里。</p></div></div>}</div>
            <div className="brand-upload-options"><span>成片规格</span><div>{[15, 30].map(duration => <button key={duration} type="button" className={brandDuration === duration ? 'active' : ''} onClick={() => setBrandDuration(duration)}>{duration === 15 ? '15 秒标准广告 · 3 镜头' : '30 秒品牌故事 · 6 镜头'}</button>)}</div></div>
            <label className={brandUploadBusy ? 'brand-brief-dropzone busy' : 'brand-brief-dropzone'}><Upload size={22}/><div><b>{brandUploadBusy ? '正在解析并建立任务…' : '不依赖策略，上传自己的 Brief'}</b><span>支持 PDF、DOCX、Markdown · 解析后进入同一套 Brief 确认流程</span></div><em>{brandUploadBusy ? '处理中' : '选择文件'}</em><input type="file" accept=".pdf,.docx,.md,application/pdf" disabled={brandUploadBusy} onChange={event => { void uploadBrandBrief(event.target.files?.[0]); event.target.value = '' }}/></label>
          </div>
          <div className="brand-task-panel"><div className="brand-hub-panel-title"><div><span className="section-label">02 · CONTINUE</span><h3>继续已有任务</h3></div><small>最近更新</small></div>
            <div className="brand-task-list">{brandTaskOptions.length ? brandTaskOptions.map(task => <article key={task.id}><div className="brand-task-status"><span>{brandVideoTaskStatusLabel(task.status)}</span><small>{task.channel || '品牌视频'} · r{task.version}</small></div><h4>{creativeTaskDisplayName(task)}</h4><p>{task.direction.core_message || '任务已保留完整来源与修改记录。'}</p><footer><time dateTime={task.updated_at}>{new Date(task.updated_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</time><button className="secondary-button" onClick={() => onOpenBrandTask(task.id)}>继续制作<ArrowRight size={14}/></button></footer></article>) : <div className="brand-source-empty"><Film size={20}/><div><b>还没有品牌广告任务</b><p>从左侧选择策略或上传 Brief，即可建立第一条任务。</p></div></div>}</div>
          </div>
        </div>}
      </section>
      : category === 'brand' && brandIntake ? <div className="image-text-direction-gate brand-direction-gate">
      <header><span className="section-label">BRAND DIRECTION DECISION</span><h2>{brandTask ? '品牌视频任务已就绪' : '先选品牌创意领地，再进入制作'}</h2><p>{brandIntake.base_handoff?.creative_view?.objective?.statement || '策略事实、品牌边界与渠道规格已冻结。'}</p></header>
      {brandTask ? <section className="creative-handoff-status available"><div><b>{brandTask.direction.focus}</b><span>{brandTask.id} · {brandTask.channel} · 方向血缘已绑定</span></div><small>下一步：补齐 Logo、产品画面与音乐/声音权利，再进入剧本和分镜。</small></section>
        : isBrandDirectionGenerating(brandDirectionBatch) ? <section className="image-text-v2-start brand-direction-progress" role="status"><Sparkles size={24}/><div><h3>品牌方向正在后台生成</h3><p>可以安全刷新、切换页面或稍后回来；任务不会中断，完成后会自动显示 3 个候选方向。</p><small>任务批次 {brandDirectionBatch?.batch_id}</small></div><button className="secondary-button" disabled>生成并校验中…</button></section>
        : brandDirectionBatch?.status === 'failed' ? <section className="image-text-v2-start brand-direction-failed" role="alert"><CircleAlert size={24}/><div><h3>本次生成没有产出可用方向</h3><p>{brandDirectionFailureMessage(brandDirectionBatch.failure_code)}</p><small>失败代码：{brandDirectionBatch.failure_code || 'DIRECTION_GENERATION_FAILED'}</small></div><button className="primary-button" disabled={Boolean(brandBusy)} onClick={() => void generateBrandDirections()}><RotateCcw size={15}/>{brandBusy ? '正在重试…' : '重新生成'}</button></section>
        : brandDirections.length === 0 ? <section className="image-text-v2-start"><Sparkles size={24}/><div><h3>生成 3 个真正不同的品牌方向</h3><p>至少两个为情绪或电影化领地；效果 CTA、伪造制作规格和同质化清单会被服务端拒绝。</p></div><button className="primary-button" disabled={Boolean(brandBusy)} onClick={() => void generateBrandDirections()}><WandSparkles size={15}/>{brandBusy ? '正在创建任务…' : '生成品牌方向'}</button></section>
        : <><div className="image-text-direction-cards">{brandDirections.map((direction, index) => <article key={direction.direction_id}><span>方向 0{index + 1} · {direction.direction_mode === 'cinematic' ? '电影化' : direction.direction_mode === 'emotional' ? '情绪叙事' : '实用备选'}</span><h3>{direction.concept}</h3><p>{direction.creative_rationale}</p><dl className="brand-direction-evidence"><div><dt>情绪弧</dt><dd>{direction.emotional_arc}</dd></div><div><dt>影像语法</dt><dd>{direction.visual_grammar}</dd></div><div><dt>记忆装置</dt><dd>{direction.brand_memory_device}</dd></div><div><dt>人物瞬间</dt><dd>{direction.human_moment}</dd></div></dl><button className="primary-button full" disabled={Boolean(brandBusy)} onClick={() => void confirmBrandDirection(direction)}>{brandBusy === direction.direction_id ? '正在冻结并创建…' : '确认方向并创建视频任务'}</button></article>)}</div><button className="secondary-button" disabled={Boolean(brandBusy)} onClick={() => void generateBrandDirections()}><RotateCcw size={15}/>重新生成</button></>}
    </div> : category === 'brand' && expectsBrandIntake ? <section className="image-text-v2-start" role={brandIntakeError ? 'alert' : 'status'}>
      {brandIntakeError ? <CircleAlert size={24}/> : <Sparkles size={24}/>}<div><h3>{brandIntakeError ? '品牌策略交接读取失败' : '正在恢复品牌策略交接'}</h3><p>{brandIntakeError || '正在校验策略包、冻结路线与任务 Overlay 血缘。'}</p></div>
      {brandIntakeError ? <button className="secondary-button" onClick={() => setBrandIntakeRetry(value => value + 1)}><RotateCcw size={15}/>重试</button> : null}
    </section> : category === 'brand' && brandContextLoading
      ? <section className="image-text-v2-start" role="status"><LoaderCircle className="spin" size={24}/><div><h3>正在读取品牌广告任务</h3><p>这里只展示当前 Project 的可继续任务，不会自动进入任何一条任务。</p></div></section>
      : category === 'brand' && brandContextError
        ? <section className="image-text-v2-start brand-direction-failed" role="alert"><CircleAlert size={24}/><div><h3>品牌广告任务读取失败</h3><p>{brandContextError}</p></div><button className="secondary-button" onClick={() => setBrandContextRetry(value => value + 1)}><RotateCcw size={15}/>重试</button></section>
        : category === 'brand' && brandTaskOptions.length > 0
          ? <section className="brand-route-entry" aria-labelledby="brand-task-entry-title">
            <header><div><span className="section-label">BRAND VIDEO TASKS</span><h2 id="brand-task-entry-title">选择要继续的品牌广告任务</h2><p>任务不会被系统自动绑定。进入后，地址才会写入对应的 context。</p></div><span>{brandTaskOptions.length} 个可继续任务</span></header>
            <div className="brand-route-task-grid">{brandTaskOptions.map(task => <article key={task.id} className="brand-route-task-card">
              <div className="brand-route-task-card-heading"><span>{brandVideoTaskStatusLabel(task.status)}</span><small>{task.channel || '品牌视频'}</small></div>
              <h3>{creativeTaskDisplayName(task)}</h3>
              <p>{task.direction.core_message || task.direction.concept || '策略与品牌方向已绑定，可进入任务继续完善制作信息。'}</p>
              <footer><time dateTime={task.updated_at}>v{task.version} · 更新于 {new Date(task.updated_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</time><button className="secondary-button" onClick={() => onOpenBrandTask(task.id)}>进入任务<ArrowRight size={15}/></button></footer>
            </article>)}</div>
          </section>
          : category === 'brand'
            ? <section className="image-text-v2-start" role="status"><Film size={24}/><div><h3>当前 Project 暂无可继续的品牌广告任务</h3><p>请先在策略工作台选择“品牌广告”并完成交接；创建任务后会显示在这里，由你明确选择进入。</p></div></section>
      : <Suspense fallback={<div className="ai-native-feature-loading">正在加载素材剪辑工作区…</div>}><VideoEditingWorkspaceV2 onNotice={setNotice} editTaskId={activeTaskId} onOpenEditTask={onOpenEditTask}/></Suspense>}
    {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
  </section></StateBoundary>
}

const viralDimensionLabels: Record<ApiVideoPromptDimension['id'], string> = {
  task_goal_type: '任务目标类型',
  quality_style_lighting: '画质&风格&光影规范',
  environment_atmosphere: '环境氛围',
  camera_content: '镜头画面内容',
  music_sound: '音乐&音效',
}

function promptFromViralWorkspace(
  workspace: ApiViralRemakeWorkspace,
  sourceTitle: string,
  sourceFileName: string,
  referenceImageName: string,
): ApiVideoReplicationPrompt | null {
  const viral = workspace.video_draft.viral_remake
  const promptDraft = viral.prompt_draft
  if (!promptDraft) return null
  const evidence = new Map(
    (viral.analysis_snapshot?.dimensions ?? []).map(dimension => [
      dimension.id,
      dimension.evidence_refs.length > 0
        ? dimension.evidence_refs.join('；')
        : `Seed-2-pro 置信度 ${Math.round(dimension.confidence * 100)}%`,
    ]),
  )
  return {
    source_asset: viral.input_snapshot.reference_video,
    source_title: sourceTitle,
    source_file_name: sourceFileName || undefined,
    reference_image_name: referenceImageName || undefined,
    user_instruction: viral.input_snapshot.user_instruction,
    dimensions: Object.entries(promptDraft.dimensions).map(([id, prompt]) => ({
      id: id as ApiVideoPromptDimension['id'],
      label: viralDimensionLabels[id as ApiVideoPromptDimension['id']],
      prompt,
      evidence: evidence.get(id as ApiVideoPromptDimension['id']) ?? '来自已持久化的分析快照',
    })),
    composite_prompt: promptDraft.composite_prompt,
    model_directive: '只复用抽象节奏、镜头功能与转化结构，替换原片人物、商标、字幕、音乐和受保护表达',
  }
}

function ProjectMediaContext() {
  const { currentProject } = useProject()
  const [assets, setAssets] = useState<ApiProjectMediaAsset[]>([])
  const [selectedId, setSelectedId] = useState('')
  useEffect(() => {
    let active = true
    void api.listProjectMediaAssets(currentProject.id).then(items => {
      if (!active) return
      const videos = items.filter(item => item.kind === 'video')
      setAssets(items)
      setSelectedId(current => videos.some(item => item.id === current) ? current : videos[0]?.id ?? '')
    }).catch(() => {
      if (active) setAssets([])
    })
    return () => { active = false }
  }, [currentProject.id])
  const videos = assets.filter(asset => asset.kind === 'video')
  const brief = assets.find(asset => asset.mimeType === 'application/pdf')
  const selected = videos.find(asset => asset.id === selectedId) ?? videos[0]
  if (!assets.length) return null
  return <section className="project-media-context" aria-label="当前 Project 导入媒体">
    <div><span className="section-label">PROJECT MEDIA</span><b>{videos.length} 个视频 · {brief ? '1 个 PDF Brief' : '未发现 PDF Brief'}</b><small>全部由平台 API 返回，视频可直接作为创作参考或加入混剪。</small></div>
    <div className="project-media-context-list">{videos.slice(0, 6).map((asset, index) => <button key={asset.id} className={selected?.id === asset.id ? 'active' : ''} onClick={() => setSelectedId(asset.id)} aria-label={shortDramaVideoLabel(asset, index)}><Play size={13} fill="currentColor"/><span>{asset.durationSeconds ? `${asset.durationSeconds.toFixed(0)}s` : `正片 ${String(index + 1).padStart(2, '0')}`}</span></button>)}</div>
    {selected ? <video className="project-media-context-preview" controls preload="metadata" src={selected.contentUrl}/> : null}
  </section>
}

function ViralRemixWorkspace({ onNotice, handoffIntake }: { onNotice: (message: string) => void; handoffIntake?: ApiTaskStrategyCreativeIntake }) {
  const { currentProject, reloadProjects } = useProject()
  const { providers } = useModelConfig()
  const [sourceAssetId, setSourceAssetId] = useState('source_video')
  const [sourceVersion, setSourceVersion] = useState(1)
  const [sourceTitle, setSourceTitle] = useState('30 秒爆款结构样本')
  const [durationSeconds, setDurationSeconds] = useState(30)
  const [sourceFileName, setSourceFileName] = useState('')
  const [sourcePreviewUrl, setSourcePreviewUrl] = useState('')
  const [referenceImageName, setReferenceImageName] = useState('')
  const [referenceImagePreviewUrl, setReferenceImagePreviewUrl] = useState('')
  const [referenceImageAsset, setReferenceImageAsset] = useState<{ asset_id: string; version: number } | undefined>()
  const [userInstruction, setUserInstruction] = useState('保留原视频的强停留节奏，但改写为当前产品的原创广告表达。')
  const [productName, setProductName] = useState(currentProject.product)
  const [sellingPoint, setSellingPoint] = useState('±0.01mm 精度')
  const [secondSellingPoint, setSecondSellingPoint] = useState('98% 准时交付')
  const [cta, setCta] = useState('预约获取打样方案')
  const [viralWorkspace, setViralWorkspace] = useState<ApiViralRemakeWorkspace | null>(null)
  const [analysis, setAnalysis] = useState<ApiHitAnalysis | null>(null)
  const [replicationPrompt, setReplicationPrompt] = useState<ApiVideoReplicationPrompt | null>(null)
  const [job, setJob] = useState<ApiGenerationJob | null>(null)
  const [confirmedBriefId, setConfirmedBriefId] = useState('')
  const [viralTaskId, setViralTaskId] = useState('')
  const [generationReady, setGenerationReady] = useState(false)
  const [rightsConfirmed, setRightsConfirmed] = useState(false)
  const [uploadingSource, setUploadingSource] = useState(false)
  const [uploadingReference, setUploadingReference] = useState(false)
  const [busyStep, setBusyStep] = useState<'analysis' | 'generate' | ''>('')
  const configuredProvider = providers.find(provider => provider.status === '已配置')
  const isGenerating = job?.status === 'queued' || job?.status === 'running'
  const makeAsset = (assetId: string, version = 1) => ({ asset_id: assetId.trim(), version })
  useEffect(() => {
    let active = true
    void Promise.all([
      api.listArtifacts(currentProject.id),
      api.getLatestViralRemakeWorkspace(currentProject.id),
    ]).then(([artifacts, workspace]) => {
      if (!active) return
      setConfirmedBriefId(artifacts.filter(artifact => artifact.kind === 'brief' && artifact.status === 'ready').at(-1)?.id ?? '')
      if (!workspace) return
      const input = workspace.video_draft.viral_remake.input_snapshot
      setViralTaskId(workspace.task.id)
      setViralWorkspace(workspace)
      setGenerationReady(workspace.video_draft.viral_remake.readiness.generation_ready)
      setRightsConfirmed(
        input.reference_video_rights === 'confirmed'
        && (!input.reference_image || input.reference_image_rights === 'confirmed'),
      )
      setSourceAssetId(input.reference_video.asset_id)
      setSourceVersion(input.reference_video.version)
      setReferenceImageAsset(input.reference_image)
      setProductName(input.product_name)
      setSellingPoint(input.selling_points[0] ?? '')
      setSecondSellingPoint(input.selling_points[1] ?? '')
      setCta(input.call_to_action)
      setUserInstruction(input.user_instruction)
      setReplicationPrompt(promptFromViralWorkspace(workspace, input.reference_video.asset_id, '', ''))
      const latestCandidate = (workspace.video_draft.viral_remake.candidates ?? []).at(-1)
      if (latestCandidate) {
        void api.getViralVideoJob(currentProject.id, latestCandidate.provider_job_id).then(setJob).catch(() => undefined)
      }
      onNotice(`已恢复爆款复刻任务 ${workspace.task.id.slice(0, 8)}，素材引用和手工输入未丢失。`)
    }).catch(() => undefined)
    return () => { active = false }
  }, [currentProject.id, onNotice])
  useEffect(() => () => {
    if (sourcePreviewUrl) window.URL.revokeObjectURL(sourcePreviewUrl)
  }, [sourcePreviewUrl])
  useEffect(() => () => {
    if (referenceImagePreviewUrl) window.URL.revokeObjectURL(referenceImagePreviewUrl)
  }, [referenceImagePreviewUrl])
  useEffect(() => {
    if (!job || !isGenerating) return
    const timer = window.setInterval(() => {
      void api.getViralVideoJob(currentProject.id, job.id).then(next => {
        setJob(next)
        if (next.status === 'succeeded') {
          void reloadProjects()
          if (viralTaskId) {
            void api.getViralRemakeWorkspace(currentProject.id, viralTaskId).then(workspace => {
              setViralWorkspace(workspace)
              setGenerationReady(workspace.video_draft.viral_remake.readiness.generation_ready)
            }).catch(() => undefined)
          }
          onNotice('复刻视频生成完成，已作为新视频资产关联到当前 Project。')
        }
      }).catch(cause => onNotice(cause instanceof Error ? cause.message : '复刻视频任务状态读取失败。'))
    }, 1500)
    return () => window.clearInterval(timer)
  }, [currentProject.id, job, isGenerating, reloadProjects, onNotice, viralTaskId])
  const composePrompt = (prompt: ApiVideoReplicationPrompt, dimensions = prompt.dimensions) => [
    `源视频参考：${prompt.source_title}${prompt.source_file_name ? `（${prompt.source_file_name}）` : ''}，Asset ${prompt.source_asset.asset_id} v${prompt.source_asset.version}。`,
    `多模态输入：源视频用于复刻节奏、镜头功能和声音结构；${prompt.user_instruction ? `文本指令优先约束内容改写：${prompt.user_instruction}；` : ''}${prompt.reference_image_name ? `参考图片用于约束主体外观、产品形态、色彩或构图气质：${prompt.reference_image_name}；` : ''}`,
    ...dimensions.map(dimension => `【${dimension.label}】${dimension.prompt}`),
    '生成要求：视频参考负责节奏和镜头功能，图片参考负责主体视觉，文本指令负责内容改写和约束；三者冲突时以文本指令和版权安全为最高优先级。不复制原视频人物、商标、字幕、画面构图或受版权保护的表达。',
  ].join('\n')
  const handleSourceFile = async (file?: File) => {
    if (sourcePreviewUrl) window.URL.revokeObjectURL(sourcePreviewUrl)
    if (!file) {
      setSourceFileName('')
      setSourcePreviewUrl('')
      return
    }
    setSourceFileName(file.name)
    setSourceTitle(file.name.replace(/\.[^.]+$/, '') || sourceTitle)
    setSourcePreviewUrl(window.URL.createObjectURL(file))
    setUploadingSource(true)
    try {
      const ref = await api.uploadProjectAsset(currentProject.id, file)
      setSourceAssetId(ref.asset_id)
      setSourceVersion(ref.version)
      setViralTaskId('')
      setViralWorkspace(null)
      setReplicationPrompt(null)
      setJob(null)
      setGenerationReady(false)
      setRightsConfirmed(false)
      onNotice(`源视频 ${file.name} 已上传并固定为 Asset ${ref.asset_id} v${ref.version}。`)
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '源视频上传失败。')
    } finally {
      setUploadingSource(false)
    }
  }
  const handleReferenceImage = async (file?: File) => {
    if (referenceImagePreviewUrl) window.URL.revokeObjectURL(referenceImagePreviewUrl)
    if (!file) {
      setReferenceImageName('')
      setReferenceImagePreviewUrl('')
      return
    }
    setReferenceImageName(file.name)
    setReferenceImagePreviewUrl(window.URL.createObjectURL(file))
    setUploadingReference(true)
    try {
      const ref = await api.uploadProjectAsset(currentProject.id, file)
      setReferenceImageAsset(ref)
      setViralTaskId('')
      setViralWorkspace(null)
      setReplicationPrompt(null)
      setJob(null)
      setGenerationReady(false)
      setRightsConfirmed(false)
      onNotice(`参考图片 ${file.name} 已上传并固定为 Asset ${ref.asset_id} v${ref.version}。`)
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '参考图片上传失败。')
    } finally {
      setUploadingReference(false)
    }
  }
  const updateDimension = (id: ApiVideoPromptDimension['id'], promptText: string) => {
    setReplicationPrompt(current => {
      if (!current) return current
      const dimensions = current.dimensions.map(dimension => dimension.id === id ? { ...dimension, prompt: promptText } : dimension)
      return { ...current, dimensions, composite_prompt: composePrompt(current, dimensions) }
    })
  }
  const analyze = async () => {
    if (!sourceAssetId.trim() || sourceAssetId === 'source_video') {
      onNotice('请先上传爆款源视频，系统需要真实的 AssetVersionRef。')
      return
    }
    setBusyStep('analysis')
    try {
      let taskId = viralTaskId
      if (!taskId) {
        const workspace = await api.createManualViralRemakeWorkspace(currentProject.id, {
          parentIntakeId: handoffIntake?.id,
          sourceVideo: makeAsset(sourceAssetId, sourceVersion),
          referenceImage: referenceImageAsset,
          productName,
          sellingPoints: [sellingPoint, secondSellingPoint],
          callToAction: cta,
          userInstruction,
          objective: handoffIntake?.request.objective || '复用高停留结构，生成当前产品的原创转化广告',
          audience: handoffIntake?.request.audience || '当前 Project 的目标受众',
          coreMessage: handoffIntake?.request.core_message || [sellingPoint, secondSellingPoint].filter(Boolean).join('；'),
          durationSeconds,
        })
        taskId = workspace.task.id
        setViralTaskId(taskId)
        setViralWorkspace(workspace)
        setGenerationReady(workspace.video_draft.viral_remake.readiness.generation_ready)
        onNotice(`已创建并持久化爆款复刻任务 ${taskId.slice(0, 8)}，正在使用 Seed-2-pro 分析源视频。`)
      }
      const workspace = await api.analyzeViralRemake(currentProject.id, taskId)
      const prompt = promptFromViralWorkspace(workspace, sourceTitle, sourceFileName, referenceImageName)
      if (!prompt) throw new Error('分析已返回，但没有生成可编辑的五维提示词。')
      setViralWorkspace(workspace)
      setReplicationPrompt(prompt)
      setGenerationReady(workspace.video_draft.viral_remake.readiness.generation_ready)
      setJob(null)
      onNotice(`Seed-2-pro 已完成真实拆解并保存不可变分析快照，生成 ${prompt.dimensions.length} 个可编辑维度。`)
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '视觉理解拆解失败。')
    } finally {
      setBusyStep('')
    }
  }
  const generateReplica = async () => {
    if (!replicationPrompt || !viralWorkspace || !viralTaskId) {
      onNotice('请先完成五维视觉理解拆解。')
      return
    }
    if (!rightsConfirmed) {
      onNotice('请先确认源视频和参考图片具有可用于本次生成的授权。')
      return
    }
    if (!configuredProvider) {
      onNotice('服务端尚未配置视频生成模型，无法生成复刻视频。')
      return
    }
    setBusyStep('generate')
    try {
      const dimensions = Object.fromEntries(
        replicationPrompt.dimensions.map(dimension => [dimension.id, dimension.prompt.trim()]),
      ) as Record<ApiVideoPromptDimension['id'], string>
      const updated = await api.updateViralPrompt(
        currentProject.id,
        viralTaskId,
        viralWorkspace.video_draft.revision,
        dimensions,
      )
      const confirmed = await api.confirmViralGeneration(
        currentProject.id,
        viralTaskId,
        updated.video_draft.revision,
        Boolean(referenceImageAsset),
      )
      setViralWorkspace(confirmed)
      setGenerationReady(true)
      const created = await api.createViralVideoJob(currentProject.id, viralTaskId)
      setJob(created)
      onNotice(created.status === 'succeeded'
        ? '复刻视频生成完成，候选视频已进入项目素材库。'
        : '提示词与版权确认已冻结为 PromptPackage，Seedance 生成任务正在运行。')
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '创建复刻视频生成任务失败。')
    } finally {
      setBusyStep('')
    }
  }
  const latestCandidate = (viralWorkspace?.video_draft.viral_remake.candidates ?? []).at(-1)
  const submitCandidateReview = async () => {
    if (!viralTaskId || !latestCandidate) return
    setBusyStep('generate')
    try {
      const workspace = await api.submitViralCandidateReview(currentProject.id, viralTaskId, latestCandidate.id)
      setViralWorkspace(workspace)
      onNotice('候选视频已通过最小检查并提交评审，完整 Prompt 与素材血缘已保留。')
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '候选视频提交评审失败。')
    } finally {
      setBusyStep('')
    }
  }
  return <div className="viral-remake-lab">
    <aside className="viral-source-panel">
      <span className="section-label">多模态输入</span>
      <label className="viral-upload">上传爆款源视频<input type="file" accept="video/mp4" onChange={event => { void handleSourceFile(event.target.files?.[0]) }}/><small>{uploadingSource ? '正在上传并写入项目素材库…' : sourceFileName || '视频用于拆解节奏、镜头功能和声音结构。'}</small></label>
      <div className="viral-source-preview">{sourcePreviewUrl ? <video src={sourcePreviewUrl} controls muted aria-label="源视频预览"/> : <><Film size={24}/><span>等待源视频预览</span></>}</div>
      <label className="viral-upload image">上传参考图片<input type="file" accept="image/png,image/jpeg" onChange={event => { void handleReferenceImage(event.target.files?.[0]) }}/><small>{uploadingReference ? '正在上传并写入项目素材库…' : referenceImageName || '图片用于约束主体外观、产品形态和视觉风格。'}</small></label>
      {referenceImagePreviewUrl ? <div className="viral-image-preview"><img src={referenceImagePreviewUrl} alt="参考图片预览"/><span>{referenceImageName}</span></div> : null}
      <label>文本指令<textarea className="viral-text-instruction" value={userInstruction} onChange={event => setUserInstruction(event.target.value)} placeholder="例如：更年轻化，突出夏季户外场景，避免出现原视频人物和 Logo。"/></label>
      <label>源视频 Asset ID<input value={sourceAssetId} onChange={event => setSourceAssetId(event.target.value)}/></label>
      <label>源视频版本<input type="number" min={1} value={sourceVersion} onChange={event => setSourceVersion(Number(event.target.value))}/></label>
      <label>视频标题<input value={sourceTitle} onChange={event => setSourceTitle(event.target.value)}/></label>
      <label>时长（秒）<input type="number" min={9} max={180} value={durationSeconds} onChange={event => setDurationSeconds(Number(event.target.value))}/></label>
      <button className="primary-button full" disabled={busyStep === 'analysis' || uploadingSource || uploadingReference} onClick={() => void analyze()}><WandSparkles size={15}/>{busyStep === 'analysis' ? '保存任务与准备分析…' : '视觉理解拆解五维提示词'}</button>
    </aside>
    <section className="viral-dimension-panel">
      <div className="viral-dimension-hero"><div><span className="section-label">VLM PROMPT DNA</span><h3>{viralWorkspace?.video_draft.viral_remake.analysis_snapshot ? sourceTitle : '等待视觉理解模型拆解'}</h3><p>{viralWorkspace?.video_draft.viral_remake.analysis_snapshot ? '已将源视频拆为任务、画质风格、环境氛围、镜头内容和音乐音效五个可编辑提示词维度。' : '输入源视频后，系统会把爆款视频拆成可控的生成指令，再送入视频生成模型。'}</p></div><div><b>{replicationPrompt ? replicationPrompt.dimensions.length : 0}</b><small>Prompt 维度</small></div></div>
      <div className="viral-dimension-grid">{replicationPrompt ? replicationPrompt.dimensions.map(dimension => <article className="viral-dimension-card" key={dimension.id}><div><span>{dimension.label}</span><small>{dimension.evidence}</small></div><textarea aria-label={dimension.label} value={dimension.prompt} onChange={event => updateDimension(dimension.id, event.target.value)}/></article>) : ['任务目标类型', '画质&风格&光影规范', '环境氛围', '镜头画面内容', '音乐&音效'].map(label => <article className="viral-dimension-card empty" key={label}><div><span>{label}</span><small>等待模型输出</small></div><p>上传或填写源视频后点击拆解。</p></article>)}</div>
    </section>
    <aside className="viral-generation-panel">
      <span className="section-label">复刻视频生成</span>
      <label>目标产品<input value={productName} onChange={event => setProductName(event.target.value)}/></label>
      <label>卖点 1<input value={sellingPoint} onChange={event => setSellingPoint(event.target.value)}/></label>
      <label>卖点 2<input value={secondSellingPoint} onChange={event => setSecondSellingPoint(event.target.value)}/></label>
      <label>CTA<input value={cta} onChange={event => setCta(event.target.value)}/></label>
      {!configuredProvider ? <div className="model-required"><CircleAlert size={15}/><span>服务端尚未配置视频生成模型。</span></div> : null}
      {!confirmedBriefId ? <div className="model-required"><CircleAlert size={15}/><span>Strategy 尚未接线；当前任务明确使用 manual Intake，不伪造 Brief。</span></div> : null}
      {!generationReady ? <div className="model-required"><CircleAlert size={15}/><span>完成分析并确认提示词与素材授权后，系统会冻结 PromptPackage 再开始正式生成。</span></div> : null}
      <label className="viral-rights-confirmation"><input type="checkbox" checked={rightsConfirmed} onChange={event => setRightsConfirmed(event.target.checked)}/><span>我确认源视频及参考图片可用于本次分析与原创广告生成</span></label>
      <label>复刻视频总提示词<textarea className="viral-composite-prompt" readOnly value={replicationPrompt?.composite_prompt ?? '五维拆解完成后自动生成总提示词。'}/></label>
      <button className="primary-button full" disabled={!replicationPrompt || !rightsConfirmed || !configuredProvider || isGenerating || busyStep === 'generate'} onClick={() => void generateReplica()}><Video size={15}/>{isGenerating || busyStep === 'generate' ? '生成中…' : latestCandidate?.status === 'failed' ? '重试生成复刻视频' : '生成复刻视频'}</button>
      {job ? <div className="inline-notice" role="status">复刻任务 {shortId(job.id)} · {job.status} · {job.model ?? '模型待分配'}{job.diagnostic ? ` · ${job.diagnostic}` : ''}</div> : null}
      {latestCandidate ? <div className="viral-candidate-summary">
        <b>候选 {latestCandidate.id.slice(0, 8)} · {latestCandidate.status}</b>
        {latestCandidate.output_asset_ref ? <small>已入库 Asset {latestCandidate.output_asset_ref.asset_id} v{latestCandidate.output_asset_ref.version}</small> : null}
        {latestCandidate.checks.map(check => <small key={check.code}>{check.passed ? '✓' : '×'} {check.message}</small>)}
        {latestCandidate.error_message ? <small>{latestCandidate.error_code} · {latestCandidate.error_message}</small> : null}
        {latestCandidate.status === 'succeeded' && latestCandidate.checks.every(check => check.passed)
          ? <button className="secondary-button" disabled={busyStep === 'generate'} onClick={() => void submitCandidateReview()}>提交候选评审</button>
          : null}
      </div> : null}
      {replicationPrompt ? <div className="viral-safety-note"><ShieldCheck size={15}/><span>{replicationPrompt.model_directive}；只复刻结构与生成指令，不复制原片受保护表达。</span></div> : null}
    </aside>
  </div>
}

const defaultShortDramaGenerationConfig: ApiShortDramaGenerationConfig = {
  subtitle_style: 'high_contrast_dynamic',
  hook_strength: 4,
  pace_profile: 'auto',
}

function mapShortDramaWorkspacePlan(workspace: ApiShortDramaPrerollWorkspace): ApiShortDramaPrerollPlan {
  const activeConfig = workspace.video_draft.short_drama_preroll.active_candidate_batch?.generation_config
    ?? defaultShortDramaGenerationConfig
  const candidates = workspace.video_draft.short_drama_preroll.candidates.map(candidate => ({
    id: candidate.id,
    hookType: (candidate.hook_strategy === 'conflict_reversal' ? 'conflict'
      : candidate.hook_strategy === 'suspense_reveal' ? 'suspense'
        : candidate.hook_strategy === 'identity_contrast' ? 'reversal' : 'selling_point_bridge') as ApiShortDramaPrerollCandidate['hookType'],
    executionAngle: candidate.execution_angle,
    executionAngleLabel: candidate.execution_angle === 'dialogue_confrontation'
      ? '台词对峙'
      : candidate.execution_angle === 'action_reveal'
        ? '关键动作'
        : candidate.execution_angle === 'reaction_escalation' ? '群体反应' : '结果先行',
    score: candidate.score,
    scoreMeaning: candidate.score_meaning,
    evidence: candidate.evidence,
    primaryTestVariable: candidate.primary_test_variable ?? candidate.execution_angle,
    pacingProfile: candidate.pacing_profile ?? activeConfig.pace_profile,
    visualGrammar: candidate.visual_grammar ?? candidate.visual_intent,
    variantHypothesis: candidate.variant_hypothesis ?? '以不同钩子机制验证前 1 秒的注意力。',
    hookLine: candidate.hook_line,
    voiceover: candidate.voiceover,
    storyboard: candidate.storyboard.map(beat => ({
      startSeconds: beat.start_seconds,
      endSeconds: beat.end_seconds,
      visual: beat.visual,
      copy: beat.copy,
    })),
    visualIntent: candidate.visual_intent,
    transitionLine: candidate.transition_line,
    promptPackage: {
      compiledPrompt: candidate.prompt_package.compiled_prompt,
      contentHash: candidate.prompt_package.content_hash,
      directorSpec: candidate.prompt_package.director_spec,
      candidateBatchId: candidate.prompt_package.candidate_batch_id,
      promptCompilerVersion: candidate.prompt_package.prompt_compiler_version,
      generationConfig: candidate.prompt_package.generation_config ?? activeConfig,
      subtitleSpec: candidate.prompt_package.subtitle_spec ?? {
        mode: activeConfig.subtitle_style,
        max_lines: activeConfig.subtitle_style === 'brand_minimal' ? 1 : 2,
        safe_area: '9:16',
        keyword_emphasis: activeConfig.subtitle_style === 'high_contrast_dynamic',
        animation_density: activeConfig.subtitle_style === 'high_contrast_dynamic' ? 'high' : 'low',
        contrast_policy: 'model_generated_readable',
      },
    },
  }))
  return { version: 'short_drama_preroll_v1', candidates }
}

function PreRollWorkspace({ mode, onNotice, handoffIntake }: { mode: 'short-drama' | 'game'; onNotice: (message: string) => void; handoffIntake?: ApiTaskStrategyCreativeIntake }) {
  const { currentProject, reloadProjects } = useProject()
  const { providers } = useModelConfig()
  const preset = preRollPresets[mode]
  const isShortDrama = mode === 'short-drama'
  const scope: ApiPrerollScope = {
    projectId: currentProject.id,
    purpose: 'preroll',
    prerollType: mode === 'short-drama' ? 'short_drama' : 'game',
  }
  const [selectedShot, setSelectedShot] = useState(0)
  const [job, setJob] = useState<ApiGenerationJob | null>(null)
  const [confirmedBriefId, setConfirmedBriefId] = useState('')
  const [hasPersistedAsset, setHasPersistedAsset] = useState(false)
  const [interactionFeedback, setInteractionFeedback] = useState('请选择一个镜头以更新中央预览。')
  const [storyContext, setStoryContext] = useState<ApiShortDramaStoryContext>({
    title: '',
    synopsis: '',
    reviewedSellingPoints: [...localShortDramaBriefs[0].reviewedSellingPoints],
  })
  const [plan, setPlan] = useState<ApiShortDramaPrerollPlan | null>(null)
  const [selectedCandidateId, setSelectedCandidateId] = useState('')
  const [isPlanning, setIsPlanning] = useState(false)
  const [shortDramaWorkspace, setShortDramaWorkspace] = useState<ApiShortDramaPrerollWorkspace | null>(null)
  const [selectedBriefId, setSelectedBriefId] = useState(localShortDramaBriefs[0].id)
  const [hookStrategy, setHookStrategy] = useState<ApiShortDramaHookStrategy>('conflict_reversal')
  const [subtitleStyle, setSubtitleStyle] = useState<ApiShortDramaSubtitleStyle>('high_contrast_dynamic')
  const [hookStrength, setHookStrength] = useState(4)
  const [paceProfile, setPaceProfile] = useState<ApiShortDramaPaceProfile>('auto')
  const [generationConfigDirty, setGenerationConfigDirty] = useState(false)
  const [generatedVideoUrl, setGeneratedVideoUrl] = useState('')
  const selectedBrief = findLocalShortDramaBrief(selectedBriefId)
  const selectedCandidate = plan?.candidates.find(candidate => candidate.id === selectedCandidateId)
  const currentShot = isShortDrama ? selectedCandidate?.visualIntent ?? '请先生成并人工选择一个短剧前贴候选。' : preset.shots[selectedShot]
  const configuredProvider = providers.find(provider => provider.status === '已配置')
  const generated = job?.status === 'succeeded' && hasPersistedAsset
  const isGenerating = job?.status === 'queued' || job?.status === 'running'
  const selectShot = (index: number) => {
    setSelectedShot(index)
    setInteractionFeedback(`当前镜头：${String(index + 1).padStart(2, '0')} · ${preset.shots[index]}。`)
  }
  const moveShotFocus = (index: number, key: string) => {
    const nextIndex = key === 'Home' ? 0
      : key === 'End' ? preset.shots.length - 1
      : key === 'ArrowUp' || key === 'ArrowLeft' ? Math.max(0, index - 1)
      : Math.min(preset.shots.length - 1, index + 1)
    selectShot(nextIndex)
    document.getElementById(`preroll-shot-${mode}-${nextIndex}`)?.focus()
  }
  useEffect(() => {
    let active = true
    void (async () => {
      const artifacts = await api.listArtifacts(currentProject.id)
      if (!active) return
      setConfirmedBriefId(artifacts.filter(artifact => artifact.kind === 'brief' && artifact.status === 'ready').at(-1)?.id ?? '')

      if (isShortDrama) {
        const workspace = await api.getLatestShortDramaPrerollWorkspace(currentProject.id)
        if (!active || !workspace) return
        const draft = workspace.video_draft.short_drama_preroll
        const snapshot = draft.input_snapshot
        const restoredConfig = draft.active_candidate_batch?.generation_config ?? {
          subtitle_style: snapshot.subtitle_style,
          hook_strength: snapshot.hook_strength,
          pace_profile: snapshot.pace_profile ?? 'auto',
        }
        setShortDramaWorkspace(workspace)
        setPlan(mapShortDramaWorkspacePlan(workspace))
        setSelectedCandidateId(draft.selected_candidate_id ?? '')
        setStoryContext({
          title: snapshot.story_title,
          synopsis: snapshot.synopsis,
          reviewedSellingPoints: [...snapshot.reviewed_selling_points],
        })
        setSelectedBriefId(findLocalShortDramaBrief(snapshot.brief_id).id)
        setHookStrategy(snapshot.hook_strategy)
        setSubtitleStyle(restoredConfig.subtitle_style)
        setHookStrength(restoredConfig.hook_strength)
        setPaceProfile(restoredConfig.pace_profile)
        setGenerationConfigDirty(false)

        const latestAttempt = workspace.short_drama_generation_attempts
          ?.filter(attempt => attempt.draft_revision === workspace.video_draft.revision
            && attempt.candidate_id === draft.selected_candidate_id)
          .at(-1)
        if (!latestAttempt) {
          setJob(null)
          setHasPersistedAsset(false)
          setInteractionFeedback(draft.selected_candidate_id
            ? '已恢复 Brief、候选批次和人工选择，可继续生成前贴视频。'
            : '已恢复 Brief 与候选批次，请人工选择一个方案。')
          return
        }
        const restoredJob = await api.getShortDramaPrerollVideoJob(currentProject.id, latestAttempt.provider_job_id)
        if (!active) return
        setJob(restoredJob)
        if (restoredJob.status === 'succeeded' && restoredJob.artifactId) {
          const media = await api.listProjectMediaAssets(currentProject.id)
          if (!active) return
          const video = media.find(asset => asset.id === restoredJob.artifactId)
          setHasPersistedAsset(Boolean(video))
          setGeneratedVideoUrl(video?.contentUrl ?? '')
          setInteractionFeedback(video
            ? '已恢复 Brief、已选候选、生成任务和视频结果。'
            : '已恢复生成任务，但视频资产仍在入库。')
        } else {
          setHasPersistedAsset(false)
          setInteractionFeedback(restoredJob.status === 'failed'
            ? '已恢复上一次失败任务，可调整配置后重新生成。'
            : '已恢复正在进行的视频任务。')
        }
        return
      }

      const [prerollArtifacts, jobs] = await Promise.all([
        api.listPrerollArtifacts(scope),
        api.listPrerollJobs(scope),
      ])
      if (!active) return
      const latest = jobs.at(-1) ?? null
      const persisted = latest?.status === 'succeeded'
        && prerollArtifacts.some(artifact => artifact.id === latest.artifactId && artifact.kind === 'video' && artifact.status === 'ready')
      setJob(latest)
      setHasPersistedAsset(persisted)
      if (latest?.status === 'succeeded' && !persisted) {
        setInteractionFeedback('任务已成功，但服务端产物尚未就绪；暂不能加入素材箱。')
      }
    })().catch(cause => {
      if (active) setInteractionFeedback(cause instanceof Error ? cause.message : '无法读取服务端任务状态。')
    })
    return () => { active = false }
  }, [currentProject.id, isShortDrama, scope.prerollType])
  useEffect(() => {
    if (!job || !isGenerating) return
    const timer = window.setInterval(() => {
      const readJob = isShortDrama ? api.getShortDramaPrerollVideoJob(currentProject.id, job.id) : api.getPrerollJob(job.id, scope)
      void readJob.then(async next => {
        setJob(next)
        if (next.status === 'succeeded') {
          const media = isShortDrama ? await api.listProjectMediaAssets(currentProject.id) : []
          const artifacts = isShortDrama ? [] : await api.listPrerollArtifacts(scope)
          const video = isShortDrama
            ? media.find(asset => asset.id === next.artifactId)
            : undefined
          const persisted = isShortDrama
            ? Boolean(video)
            : artifacts.some(artifact => artifact.id === next.artifactId && artifact.kind === 'video' && artifact.status === 'ready')
          setHasPersistedAsset(persisted)
          if (persisted) {
            void reloadProjects()
            setInteractionFeedback('前贴分镜已生成且产物已持久化，可以加入混剪素材箱。')
            onNotice(`${mode === 'short-drama' ? '短剧' : '游戏'}前贴分镜已生成，稳定资产已关联到当前 Project。`)
          } else {
            setInteractionFeedback('任务已成功，但服务端产物尚未就绪；暂不能加入素材箱。')
          }
          if (isShortDrama) setGeneratedVideoUrl(video?.contentUrl ?? '')
        } else if (next.status === 'failed' || next.status === 'cancelled') {
          setHasPersistedAsset(false)
          setInteractionFeedback(next.status === 'cancelled' ? '前贴分镜任务已取消，可以修改配置后重试。' : `前贴分镜生成失败${next.diagnostic ? `：${next.diagnostic}` : '，请重试。'}`)
        }
      }).catch(cause => setInteractionFeedback(cause instanceof Error ? cause.message : '任务状态读取失败。'))
    }, 1500)
    return () => window.clearInterval(timer)
  }, [currentProject.id, isGenerating, isShortDrama, job, mode, onNotice, reloadProjects, scope.projectId, scope.prerollType])
  const generateStoryboard = async () => {
    if (!configuredProvider) {
      setInteractionFeedback('服务端尚未配置 ARK_API_KEY，无法发起前贴分镜生成。')
      return
    }
    if (!isShortDrama && !confirmedBriefId) {
      setInteractionFeedback('请先在需求中心确认 Brief，再生成前贴分镜。')
      return
    }
    if (isShortDrama && (!plan || !selectedCandidate || !shortDramaWorkspace?.video_draft.short_drama_preroll.readiness.generation_ready)) {
      setInteractionFeedback('请先从 AI 生成候选中明确选择一个短剧前贴方案，再创建视频任务。')
      return
    }
    // A retry never presents a prior successful asset as the pending request's result.
    setJob(null)
    setHasPersistedAsset(false)
    setInteractionFeedback('正在创建新的前贴分镜任务，旧结果不会用于本次生成。')
    try {
      let next: ApiGenerationJob
      if (isShortDrama) {
        if (!plan || !selectedCandidate || !shortDramaWorkspace) return
        next = await api.createShortDramaPrerollVideoJob(
          currentProject.id,
          shortDramaWorkspace.task.id,
          shortDramaWorkspace.video_draft.revision,
          selectedCandidate.id,
        )
      } else {
        next = await api.createPrerollVideo(
          scope,
          `${preset.title}。${preset.detail}。分镜：${preset.shots.join('；')}。9:16 竖版，6 秒，静音可理解，品牌事实已校验，结尾保留稳定拼接点。`,
          confirmedBriefId,
        )
      }
      setJob(next)
      if (next.status === 'succeeded') {
        const media = isShortDrama ? await api.listProjectMediaAssets(currentProject.id) : []
        const artifacts = isShortDrama ? [] : await api.listPrerollArtifacts(scope)
        const video = isShortDrama
          ? media.find(asset => asset.id === next.artifactId)
          : undefined
        const persisted = isShortDrama
          ? Boolean(video)
          : artifacts.some(artifact => artifact.id === next.artifactId && artifact.kind === 'video' && artifact.status === 'ready')
        setHasPersistedAsset(persisted)
        if (isShortDrama) setGeneratedVideoUrl(video?.contentUrl ?? '')
        setInteractionFeedback(persisted
          ? '前贴分镜已生成且产物已持久化，可以加入混剪素材箱。'
          : '任务已成功，但服务端产物尚未就绪；暂不能加入素材箱。')
      } else {
        setHasPersistedAsset(false)
        setInteractionFeedback('前贴分镜任务已创建，正在查询服务端状态。')
      }
    } catch (cause) {
      setJob(null)
      setHasPersistedAsset(false)
      setInteractionFeedback(cause instanceof Error ? cause.message : '创建前贴分镜任务失败，请重试。')
    }
  }
  const cancelStoryboard = async () => {
    if (!job || !isGenerating) return
    try {
      setJob(await api.cancelJob(job.id, scope))
      setHasPersistedAsset(false)
      setInteractionFeedback('前贴分镜任务已取消，可以修改配置后重试。')
    } catch (cause) {
      setInteractionFeedback(cause instanceof Error ? cause.message : '取消前贴分镜任务失败，请重试。')
    }
  }
  const clearShortDramaWorkspace = () => {
    setPlan(null)
    setSelectedCandidateId('')
    setShortDramaWorkspace(null)
    setJob(null)
    setHasPersistedAsset(false)
    setGeneratedVideoUrl('')
    setGenerationConfigDirty(false)
  }
  const updateStoryContext = (field: keyof ApiShortDramaStoryContext, value: string) => {
    setStoryContext(context => ({ ...context, [field]: value }))
    clearShortDramaWorkspace()
  }
  const selectLocalBrief = (briefId: string) => {
    const brief = findLocalShortDramaBrief(briefId)
    setSelectedBriefId(brief.id)
    setHookStrategy(brief.recommendedHookStrategy)
    setStoryContext(context => ({ ...context, reviewedSellingPoints: [...brief.reviewedSellingPoints] }))
    clearShortDramaWorkspace()
    setInteractionFeedback(`已选择“${brief.name}”，卖点、CTA 和推荐钩子策略已自动带入。`)
  }
  const planShortDrama = async () => {
    setIsPlanning(true)
    try {
      const generationConfig: ApiShortDramaGenerationConfig = {
        subtitle_style: subtitleStyle,
        hook_strength: hookStrength,
        pace_profile: paceProfile,
      }
      const workspace = shortDramaWorkspace
        ? await api.regenerateShortDramaPrerollCandidates(
          currentProject.id,
          shortDramaWorkspace.task.id,
          shortDramaWorkspace.video_draft.revision,
          generationConfig,
        )
        : await api.createManualShortDramaPrerollWorkspace(currentProject.id, {
          parentIntakeId: handoffIntake?.id,
          briefId: selectedBrief.id,
          briefVersion: selectedBrief.version,
          briefName: selectedBrief.name,
          title: storyContext.title,
          synopsis: storyContext.synopsis,
          reviewedSellingPoints: Array.from(new Set([
            ...storyContext.reviewedSellingPoints.filter(Boolean),
            ...(handoffIntake?.request.core_message ? [handoffIntake.request.core_message] : []),
          ])),
          openingLine: storyContext.openingLine || undefined,
          hookStrategy,
          subtitleStyle,
          transition: 'hard_cut',
          hookStrength,
          paceProfile,
          objective: handoffIntake?.request.objective || selectedBrief.objective,
          audience: handoffIntake?.request.audience || `偏好${selectedBrief.applicableGenres.join('、')}内容的竖屏短剧观众`,
          prohibitedClaims: selectedBrief.prohibited,
          callToAction: selectedBrief.callToAction,
        })
      setShortDramaWorkspace(workspace)
      setPlan(mapShortDramaWorkspacePlan(workspace))
      setSelectedCandidateId('')
      setJob(null)
      setHasPersistedAsset(false)
      setGeneratedVideoUrl('')
      setGenerationConfigDirty(false)
      setInteractionFeedback(shortDramaWorkspace
        ? '已重新生成 3 个机制不同的候选，旧批次仍保留在服务端版本历史中。'
        : '候选已按本地 Brief、钩子策略和生成配置生成。请人工选择一个方案。')
    } catch (cause) {
      setInteractionFeedback(cause instanceof Error ? cause.message : '短剧前贴候选规划失败。请检查故事上下文后重试。')
    } finally {
      setIsPlanning(false)
    }
  }
  const selectShortDramaCandidate = async (candidate: ApiShortDramaPrerollCandidate) => {
    if (!shortDramaWorkspace) return
    try {
      const workspace = await api.selectShortDramaPrerollCandidate(
        currentProject.id,
        shortDramaWorkspace.task.id,
        shortDramaWorkspace.video_draft.revision,
        candidate.id,
      )
      setShortDramaWorkspace(workspace)
      setSelectedCandidateId(candidate.id)
      setJob(null)
      setHasPersistedAsset(false)
      setGeneratedVideoUrl('')
      setInteractionFeedback(`已人工选择“${candidate.executionAngleLabel}”候选；中央预览已更新，可创建视频任务。`)
    } catch (cause) {
      setInteractionFeedback(cause instanceof Error ? cause.message : '选择短剧候选失败。')
    }
  }
  return <div className="preroll-workspace">
    {isShortDrama ? <aside className="preroll-candidate-panel" aria-label="短剧前贴 AI 候选">
      <details open>
        <summary><span className="section-label">AI 候选</span><b>需人工选择</b><ChevronDown size={15}/></summary>
        <p>分数是结构、可执行性与合规性的启发式编导评分，不代表 CTR 或转化效果预测。</p>
        {!plan ? <div className="preroll-candidate-empty">选择本地 Brief、填写故事上下文并生成候选后，在此处完成人工选择。</div> : plan.candidates.map(candidate => <article className="preroll-candidate-card" key={candidate.id}><button type="button" className={selectedCandidateId === candidate.id ? 'active' : ''} aria-pressed={selectedCandidateId === candidate.id} onClick={() => void selectShortDramaCandidate(candidate)}><span><b>{candidate.executionAngleLabel}</b><small>启发式编导分 {candidate.score}</small></span><strong>{candidate.voiceover}</strong><small>{candidate.evidence.join(' ')}</small></button><div className="preroll-candidate-detail"><b>主测试变量</b><p>{candidate.primaryTestVariable} · {candidate.pacingProfile}</p><b>差异假设</b><p>{candidate.variantHypothesis}</p><b>文案</b><p>{candidate.hookLine}</p><b>分镜</b><ol>{candidate.storyboard.map(beat => <li key={`${candidate.id}-${beat.startSeconds}`}><small>{beat.startSeconds}–{beat.endSeconds} 秒</small><span>{beat.copy}</span></li>)}</ol><details><summary>PromptPackage</summary><small>{candidate.promptPackage.promptCompilerVersion ?? 'legacy'} · {candidate.promptPackage.contentHash}</small><small>字幕 {candidate.promptPackage.generationConfig.subtitle_style} · 强度 {candidate.promptPackage.generationConfig.hook_strength} · 节奏 {candidate.promptPackage.generationConfig.pace_profile}</small><pre>{candidate.promptPackage.compiledPrompt}</pre></details></div></article>)}
      </details>
    </aside> : <aside className="preroll-storyboard" aria-label="6 秒前贴分镜">
      <div className="surface-toolbar"><h3>镜头</h3><span>{generated ? 'v1.1' : '草稿'}</span></div>
      <p className="preroll-keyboard-hint">上下方向键切换镜头</p>
      {preset.shots.map((shot, index) => <button id={`preroll-shot-${mode}-${index}`} key={shot} className={selectedShot === index ? 'active' : ''} aria-current={selectedShot === index ? 'step' : undefined} onClick={() => selectShot(index)} onKeyDown={event => {
        if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
          event.preventDefault()
          moveShotFocus(index, event.key)
        }
      }}><span>0{index + 1}</span><div><b>{shot}</b><small>00:0{index * 2}–00:0{index * 2 + 2} · {index === 2 ? '稳定拼接点' : '保持节奏推进'}</small></div><ArrowRight size={15}/></button>)}
    </aside>}
    <section className={`preroll-preview ${mode}`} aria-label="当前镜头预览">
      <div className="preroll-preview-header"><span className="section-label">当前镜头</span><b>{isShortDrama ? (selectedCandidate?.executionAngleLabel ?? '待选择') : `0${selectedShot + 1} / 03`}</b><span>{generated ? '分镜已生成' : isGenerating ? '正在生成' : job?.status === 'failed' ? '生成失败' : job?.status === 'cancelled' ? '已取消' : '待生成'}</span></div>
      <div className="preroll-screen">{isShortDrama && generatedVideoUrl ? <video controls playsInline preload="metadata" src={generatedVideoUrl} aria-label="生成好的短剧前贴视频"/> : <><span>{isShortDrama ? 'SHORT DRAMA · HUMAN SELECTED' : preset.eyebrow}</span><h3>{currentShot}</h3><p>{isShortDrama ? selectedCandidate?.voiceover ?? '候选生成后，请从辅助面板选择一个已审核的钩子方案。' : preset.detail}</p><button aria-label={`播放${mode === 'short-drama' ? '短剧' : '游戏'}前贴预览`} disabled={!generated} onClick={() => onNotice(`${mode === 'short-drama' ? '短剧' : '游戏'}前贴预览正在播放：${currentShot}`)}><Play size={20} fill="currentColor"/></button><small>{isShortDrama ? selectedCandidate?.transitionLine ?? '等待人工选择后显示衔接语。' : `00:0${selectedShot * 2} / 00:06 · 9:16`}</small></>}</div>
      <div className="preroll-source"><span className="section-label">策略来源</span>{isShortDrama ? <><label className="preroll-brief-selector"><span>选择创作 Brief</span><select aria-label="选择创作 Brief" value={selectedBriefId} onChange={event => selectLocalBrief(event.target.value)}>{localShortDramaBriefs.map(brief => <option key={brief.id} value={brief.id}>{brief.name}</option>)}</select></label><b>本地预置 Brief · {selectedBrief.name}</b><small>目标：{selectedBrief.objective} · 抖音 9:16 · 独立 6 秒成片</small><small>适用：{selectedBrief.applicableGenres.join(' / ')}</small><small>已审核卖点：{selectedBrief.reviewedSellingPoints.join(' / ')} · CTA：{selectedBrief.callToAction}</small><small>禁用：{selectedBrief.prohibited.join('；')}</small><small>{selectedCandidate ? `已选候选：${selectedCandidate.executionAngleLabel} · ${plan?.version}` : '待生成候选并人工选择'}</small></> : <><b>{preset.source}</b><small>已确认 Brief · 品牌规则通过 · 无真实平台写入</small></>}</div>
      <p className="preroll-feedback" role="status" aria-live="polite">{interactionFeedback}</p>
    </section>
    <aside className="preroll-config">
      <span className="section-label">生成配置</span><h3>{mode === 'short-drama' ? '短剧前贴策略' : '挑战反馈型'}</h3>
      {isShortDrama ? <label>钩子策略<select value={hookStrategy} onChange={event => { setHookStrategy(event.target.value as ApiShortDramaHookStrategy); clearShortDramaWorkspace() }}><option value="conflict_reversal">冲突反转型（推荐）</option><option value="suspense_reveal">悬念揭示型</option><option value="identity_contrast">身份反差型</option><option value="selling_point_bridge">卖点剧情桥接型</option></select></label> : null}
      {isShortDrama ? <div className="short-drama-context">
        <label>短剧标题<input value={storyContext.title} onChange={event => updateStoryContext('title', event.target.value)} placeholder="已审核短剧标题"/></label>
        <label>故事梗概<textarea value={storyContext.synopsis} onChange={event => updateStoryContext('synopsis', event.target.value)} placeholder="至少 40 字，描述已审核的剧情上下文。"/></label>
        <label>已审核剧情卖点<input value={storyContext.reviewedSellingPoints[0] ?? ''} onChange={event => { setStoryContext(context => ({ ...context, reviewedSellingPoints: [event.target.value] })); clearShortDramaWorkspace() }} placeholder="至少一条已审核剧情卖点"/></label>
        <button className="secondary-button full" disabled={isPlanning} aria-busy={isPlanning} onClick={() => void planShortDrama()}><Sparkles size={15}/>{isPlanning ? '正在规划候选…' : shortDramaWorkspace ? '重新生成 3 个候选' : '生成 3 个 AI 候选'}</button>
      </div> : null}
      {isShortDrama ? <>
        <label>字幕样式<select value={subtitleStyle} onChange={event => { setSubtitleStyle(event.target.value as ApiShortDramaSubtitleStyle); setGenerationConfigDirty(Boolean(shortDramaWorkspace)) }}><option value="high_contrast_dynamic">高对比动态字幕</option><option value="brand_minimal">极简字幕</option></select></label>
        <label>节奏<select value={paceProfile} onChange={event => { setPaceProfile(event.target.value as ApiShortDramaPaceProfile); setGenerationConfigDirty(Boolean(shortDramaWorkspace)) }}><option value="auto">跟随钩子自动匹配</option><option value="punchy">强节奏快切</option><option value="balanced">均衡推进</option><option value="suspense_hold">悬念停顿</option></select></label>
        <label>钩子强度 <b>{hookStrength}</b><input aria-label="钩子强度" type="range" min="1" max="5" value={hookStrength} onInput={event => { setHookStrength(Number(event.currentTarget.value)); setGenerationConfigDirty(Boolean(shortDramaWorkspace)) }}/></label>
        {generationConfigDirty ? <div className="model-required"><CircleAlert size={15}/><span>配置已修改，请重新生成候选；新配置会写入每条 PromptPackage。</span></div> : null}
      </> : <>
        <label>字幕样式<select defaultValue="高对比动态字幕"><option>高对比动态字幕</option><option>品牌极简字幕</option></select></label>
        <label>钩子强度<input aria-label="钩子强度" type="range" min="1" max="5" defaultValue="4"/></label>
      </>}
      {['静音可理解', '品牌事实已校验', '人物与画面连续', '结尾 CTA 清晰'].map(item => <span className="analysis-check" key={item}><Check size={14}/>{item}</span>)}
      {!configuredProvider ? <div className="model-required"><CircleAlert size={15}/><span>服务端尚未配置 ARK_API_KEY，无法发起前贴分镜生成。</span></div> : null}
      {!isShortDrama && !confirmedBriefId ? <div className="model-required"><CircleAlert size={15}/><span>请先在需求中心确认 Brief，系统才会允许生成前贴分镜。</span></div> : null}
      <button className="primary-button full" disabled={!configuredProvider || (!isShortDrama && !confirmedBriefId) || isGenerating || (isShortDrama && (generationConfigDirty || !shortDramaWorkspace?.video_draft.short_drama_preroll.readiness.generation_ready))} aria-busy={isGenerating} onClick={() => void generateStoryboard()}><WandSparkles size={15}/>{isGenerating ? '正在生成前贴视频…' : generated ? '重新生成前贴视频' : '生成前贴视频'}</button>
      {isGenerating ? <button className="secondary-button full" onClick={() => void cancelStoryboard()}>取消生成</button> : null}
      <button className="secondary-button full" disabled={!generated} aria-describedby={!generated ? `preroll-export-hint-${mode}` : undefined} onClick={() => onNotice('前贴视频产物已持久化，可在素材剪辑中选择。')}>加入混剪素材箱</button>
      {!generated ? <small className="preroll-action-hint" id={`preroll-export-hint-${mode}`}>仅任务成功且服务端产物持久化后，才能加入混剪素材箱。</small> : null}
      {job ? <div className="inline-notice">任务 {shortId(job.id)} · {job.status} · {job.model ?? '模型待分配'}{job.diagnostic ? ` · ${job.diagnostic}` : ''}</div> : null}
    </aside>
  </div>
}

function CommerceHookWorkspace({ onNotice, handoffIntake }: { onNotice: (message: string) => void; handoffIntake?: ApiTaskStrategyCreativeIntake }) {
  const { currentProject, reloadProjects, updateArtifact } = useProject()
  const { providers } = useModelConfig()
  const [selectedId, setSelectedId] = useState(commerceHookTemplates[0].id)
  const [fidelity, setFidelity] = useState(commerceHookTemplates[0].fidelity)
  const [camera, setCamera] = useState(commerceHookTemplates[0].camera)
  const [motion, setMotion] = useState(commerceHookTemplates[0].motion)
  const [environment, setEnvironment] = useState(commerceHookTemplates[0].environment)
  const [result, setResult] = useState(commerceHookTemplates[0].result)
  const [guardrails, setGuardrails] = useState(commerceHookTemplates[0].guardrails)
  const [previewing, setPreviewing] = useState(false)
  const [job, setJob] = useState<ApiGenerationJob | null>(null)
  const [generatedAsset, setGeneratedAsset] = useState<ApiArtifact | null>(null)
  const [sourceOptions, setSourceOptions] = useState<ApiCreativeSourceOption[]>([])
  const [selectedSourceKey, setSelectedSourceKey] = useState('fixture:guerlain')
  const [prepared, setPrepared] = useState<ApiPreparedCommercePreroll | null>(null)
  const [commerceWorkspace, setCommerceWorkspace] = useState<ApiCommercePrerollWorkspace | null>(null)
  const [sourceNotice, setSourceNotice] = useState('')
  const [preparing, setPreparing] = useState(false)
  const selected = commerceHookTemplates.find(item => item.id === selectedId) ?? commerceHookTemplates[0]
  const configuredProvider = providers.find(provider => provider.status === '已配置')
  const selectedSource = sourceOptions.find(option =>
    `${option.source_ref.kind}:${option.source_ref.id}:${option.source_ref.version}` === selectedSourceKey,
  )
  const usingFixture = !selectedSource
  const fixtureProductAsset = commerceWorkspace?.video_draft.commerce_preroll.input_snapshot.product_asset_ref
  const inheritedProductAsset = handoffIntake?.request.task_strategy_input.media.find(item =>
    item.kind === 'image' && item.status === 'ready',
  )?.asset_ref
  const selectedProductAsset = inheritedProductAsset
    ?? selectedSource?.product.product_asset_refs[0]
    ?? (usingFixture ? fixtureProductAsset : undefined)
  const sourcePreview = selectedProductAsset
    ? `/platform/v1/projects/${encodeURIComponent(currentProject.id)}/assets/${encodeURIComponent(selectedProductAsset.asset_id)}/versions/${selectedProductAsset.version}/content`
    : selected.image
  const effectivePreparedBlockers = prepared?.readiness.blockers.filter(blocker =>
    !(handoffIntake && selectedProductAsset && blocker === 'PRODUCT_IMAGE_MISSING'),
  ) ?? []
  const effectivePreparedReady = Boolean(prepared && effectivePreparedBlockers.length === 0)
  const effectiveSourceNotice = prepared && prepared.readiness.blockers.length > 0 && effectivePreparedReady
    ? '已用冻结任务策略中的商品素材补齐 Brief 缺口；提示词已合并业务专属判断和约束。'
    : sourceNotice

  useEffect(() => {
    let active = true
    void (async () => {
      try {
        const [sources, existingWorkspace] = await Promise.all([
          api.listCommercePrerollSources(currentProject.id).catch(() => []),
          api.getLatestCommercePrerollWorkspace(currentProject.id),
        ])
        const workspace = existingWorkspace ?? await api.ensureCommercePrerollFixtureWorkspace(currentProject.id)
        if (!active) return
        setSourceOptions(sources)
        setSelectedSourceKey('fixture:guerlain')
        setCommerceWorkspace(workspace)
        const templateId = workspace.video_draft.commerce_preroll.plan.template.template_id.replace('commerce.', '')
        setSelectedId(templateId)
        const latestAttempt = workspace.commerce_preroll_generation_attempts?.at(-1)
        if (!latestAttempt) {
          setJob(null)
          setGeneratedAsset(null)
          return
        }
        const restoredJob = await api.getViralVideoJob(currentProject.id, latestAttempt.provider_job_id)
        if (!active) return
        setJob(restoredJob)
        if (restoredJob.status === 'succeeded') {
          const artifacts = await api.listArtifacts(currentProject.id)
          if (!active) return
          setGeneratedAsset(artifacts.find(candidate =>
            candidate.kind === 'video'
            && candidate.status === 'ready'
            && candidate.sourceJobId === latestAttempt.provider_job_id,
          ) ?? null)
        }
      } catch (cause) {
        if (active) setSourceNotice(cause instanceof Error ? cause.message : '电商前贴工作区恢复失败。')
      }
    })()
    return () => { active = false }
  }, [currentProject.id])

  useEffect(() => {
    let active = true
    setPreviewing(false)
    setPrepared(null)
    if (!selectedSource) {
      const savedPrompt = commerceWorkspace?.video_draft.commerce_preroll.plan.prompt
      if (savedPrompt && commerceWorkspace.video_draft.commerce_preroll.plan.template.template_id === commerceTemplateApiId(selected.id)) {
        setFidelity(savedPrompt.fidelity)
        setCamera(savedPrompt.camera)
        setMotion(savedPrompt.timeline.find(item => item.purpose === 'single_transformation')?.instruction ?? '')
        setEnvironment(savedPrompt.environment)
        setResult(savedPrompt.timeline.find(item => item.purpose === 'product_hold')?.instruction ?? '')
        setGuardrails(savedPrompt.guardrails.join('；'))
        setSourceNotice(`娇兰固定样例已由服务端恢复 · Prompt revision ${savedPrompt.prompt_version}`)
      } else {
        const copy = guerlainPromptCopy(selected.id)
        setFidelity(copy.fidelity)
        setCamera(copy.camera)
        setMotion(copy.motion)
        setEnvironment(copy.environment)
        setResult(copy.result)
        setGuardrails(copy.guardrails)
        setSourceNotice('正在使用娇兰固定样例；保存后会形成新的服务端 Prompt revision。')
      }
      return () => { active = false }
    }
    setPreparing(true)
    setSourceNotice('正在根据已确认来源编译提示词…')
    void api.prepareCommercePreroll(
      currentProject.id,
      selectedSource,
      commerceTemplateApiId(selected.id),
    ).then(value => {
      if (!active) return
      const timeline = value.plan.prompt.timeline
      setPrepared(value)
      setFidelity(value.plan.prompt.fidelity)
      setCamera(value.plan.prompt.camera)
      setEnvironment(value.plan.prompt.environment)
      setMotion(timeline.find(item => item.purpose === 'single_transformation')?.instruction ?? '')
      setResult(timeline.find(item => item.purpose === 'product_hold')?.instruction ?? '')
      setGuardrails(value.plan.prompt.guardrails.join('；'))
      setSourceNotice(value.readiness.generation_ready
        ? '提示词已根据所选来源编译，可以人工确认后生成。'
        : `提示词已生成，但正式生成仍缺少：${value.readiness.blockers.join('、')}`)
    }).catch(cause => {
      if (!active) return
      setSourceNotice(cause instanceof Error ? cause.message : '提示词编译失败。')
    }).finally(() => {
      if (active) setPreparing(false)
    })
    return () => { active = false }
  }, [commerceWorkspace, currentProject.id, selected.id, selectedSource])

  const inheritedStrategyPrompt = handoffIntake
    ? [
        `冻结任务策略目标：${handoffIntake.request.objective}`,
        `核心信息：${handoffIntake.request.core_message}`,
        `业务专属判断：${JSON.stringify(handoffIntake.request.task_strategy_input.business_strategy)}`,
        `任务策略约束：${handoffIntake.request.task_strategy_input.guardrails.join('；')}`,
      ].join('\n')
    : ''
  const prompt = `${fidelity}\n${camera}\n${motion}\n${environment}\n${result}\n${guardrails}${inheritedStrategyPrompt ? `\n${inheritedStrategyPrompt}` : ''}`
  const storyboard = prepared
    ? prepared.plan.prompt.timeline.map((segment, index) => ({
        time: `00:${segment.start_seconds.toFixed(1).padStart(4, '0')}–00:${segment.end_seconds.toFixed(1).padStart(4, '0')}`,
        name: hookStoryboard[index]?.name ?? `阶段 ${index + 1}`,
        detail: segment.instruction.replace(/^[^：]+：/, ''),
      }))
    : hookStoryboard
  useEffect(() => {
    if (!job || !['queued', 'running'].includes(job.status)) return
    const timer = window.setInterval(() => {
      void api.getViralVideoJob(currentProject.id, job.id).then(next => {
        setJob(next)
        if (next.status === 'succeeded') {
          void Promise.all([reloadProjects(), api.listArtifacts(currentProject.id)]).then(([, artifacts]) => {
            const asset = artifacts.find(candidate =>
              candidate.kind === 'video'
              && candidate.status === 'ready'
              && (candidate.id === next.artifactId || candidate.sourceJobId === next.id),
            )
            if (asset) setGeneratedAsset(asset)
            onNotice(`「${selected.name}」生成完成，已保存到素材库并进入素材检查队列。`)
          }).catch(cause => onNotice(cause instanceof Error ? cause.message : '生成资产读取失败'))
        }
      }).catch(cause => onNotice(cause instanceof Error ? cause.message : '任务状态读取失败'))
    }, 1500)
    return () => window.clearInterval(timer)
  }, [currentProject.id, job, onNotice, reloadProjects, selected.name])
  const persistFixtureDraft = async (
    templateId = selected.id,
    includeEdits = true,
  ) => {
    const workspace = commerceWorkspace ?? await api.ensureCommercePrerollFixtureWorkspace(currentProject.id)
    const next = await api.updateCommercePrerollDraft(
      currentProject.id,
      workspace.task.id,
      {
        expected_revision: workspace.video_draft.revision,
        template_ref: {
          template_id: commerceTemplateApiId(templateId),
          template_version: 1,
        },
        ...(includeEdits ? {
          fidelity,
          camera,
          motion,
          environment,
          result,
          guardrails: guardrails.split('；').map(item => item.trim()).filter(Boolean),
        } : {}),
      },
    )
    setCommerceWorkspace(next)
    return next
  }
  const selectTemplate = async (templateId: string) => {
    if (!usingFixture || !commerceWorkspace) {
      setSelectedId(templateId)
      return
    }
    if (commerceWorkspace.video_draft.commerce_preroll.plan.template.template_id === commerceTemplateApiId(templateId)) {
      setSelectedId(templateId)
      return
    }
    try {
      setPreparing(true)
      const next = await persistFixtureDraft(templateId, false)
      setSelectedId(templateId)
      onNotice(`「${commerceHookTemplates.find(item => item.id === templateId)?.name ?? templateId}」提示词已由服务端生成并保存。`)
      return next
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '切换电商前贴模板失败。')
    } finally {
      setPreparing(false)
    }
  }
  const save = async () => {
    try {
      if (usingFixture) {
        const next = await persistFixtureDraft()
        onNotice(`「${selected.name}」已保存为 Prompt revision ${next.video_draft.revision}。`)
        return
      }
      await updateArtifact('creative', { status: '制作中', sourceVersion: `策略 ${currentProject.artifacts.strategy.version}`, summary: `广告前贴 · ${selected.name} · ${selected.frameStrategy}` })
      onNotice(`「${selected.name}」已保存为广告前贴策略草稿，并保留来源版本。`)
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '保存广告前贴策略失败，请重试。')
    }
  }
  const copyPrompt = async () => {
    try { await navigator.clipboard.writeText(prompt); onNotice('完整视频提示词已复制。') }
    catch { onNotice('提示词已准备好，请从右侧字段中复制。') }
  }
  const generate = async () => {
    if (selectedSource && !prepared) {
      onNotice('当前 Brief 的提示词还没有准备完成，请稍后再试。')
      return
    }
    if (selectedSource && !effectivePreparedReady) {
      onNotice(`当前输入还不能正式生成：${effectivePreparedBlockers.join('、') || '缺少商品素材'}`)
      return
    }
    try {
      setPreparing(true)
      setGeneratedAsset(null)
      const sourceId = handoffIntake?.id ?? selectedSource?.source_ref.id ?? 'creative-video-intake-commerce-preroll-guerlain-v1'
      const productAsset = selectedProductAsset
      let confirmedWorkspace = commerceWorkspace
      if (usingFixture && !handoffIntake) {
        const saved = await persistFixtureDraft()
        confirmedWorkspace = await api.confirmCommercePrerollGeneration(
          currentProject.id,
          saved.task.id,
          saved.video_draft.revision,
        )
        setCommerceWorkspace(confirmedWorkspace)
      }
      const next = usingFixture && !handoffIntake && confirmedWorkspace
        ? await api.createCommercePrerollWorkspaceVideoJob(currentProject.id, confirmedWorkspace)
        : productAsset
          ? await api.createPreparedCommercePrerollVideo(currentProject.id, prompt, sourceId, productAsset)
          : await api.createMedia(currentProject.id, 'video', prompt, sourceId)
      setJob(next)
      if (usingFixture) {
        const refreshed = await api.getLatestCommercePrerollWorkspace(currentProject.id)
        if (refreshed) setCommerceWorkspace(refreshed)
      }
      if (next.status === 'succeeded') {
        const artifacts = await api.listArtifacts(currentProject.id)
        setGeneratedAsset(artifacts.find(candidate =>
          candidate.kind === 'video'
          && candidate.status === 'ready'
          && (candidate.id === next.artifactId || candidate.sourceJobId === next.id),
        ) ?? null)
      }
      onNotice(next.status === 'succeeded' ? '视频生成完成，已进入素材库和素材检查。' : '视频生成任务已创建，正在轮询。')
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '创建视频生成任务失败。')
    } finally {
      setPreparing(false)
    }
  }
  return <div className="commerce-hook-workspace">
    <aside className="hook-template-rail">
      <div className="hook-rail-heading"><span className="section-label">场景策略库</span><b>电商前贴 / 钩子</b><small>学习资料 revision 399</small></div>
      {commerceHookTemplates.map((template, index) => <button key={template.id} disabled={preparing} className={selectedId === template.id ? 'active' : ''} onClick={() => void selectTemplate(template.id)}><span>{String(index + 1).padStart(2, '0')}</span><div><b>{template.name}</b><small>{template.category} · {template.duration}</small></div></button>)}
      <a href="https://bytedance.larkoffice.com/wiki/H5uQwNji9iYH0TkNXaxcvFhUn2c" target="_blank" rel="noreferrer"><ExternalLink size={13}/>查看学习来源</a>
    </aside>
    <section className="hook-canvas">
      <div className="hook-canvas-toolbar"><div><span className="source-chip">{selected.frameStrategy}</span><b>{selected.name}</b>{generatedAsset ? <span className="source-chip ready">已进入素材检查</span> : null}</div><button onClick={copyPrompt}><ClipboardCheck size={14}/>复制提示词</button></div>
      <div className="hook-preview-stage">
        <div className={generatedAsset ? 'hook-phone-frame has-generated-video' : 'hook-phone-frame'}>
          {generatedAsset
            ? <video key={`${generatedAsset.id}-v${generatedAsset.version}`} controls playsInline preload="metadata" src={generatedAsset.content} aria-label={`${selected.name}生成视频`}/>
            : <><img src={sourcePreview} alt={`${selected.name}${selected.imageLabel}`}/><div className="hook-preview-shade"/><span className="hook-frame-label">{selectedProductAsset ? 'Brief 商品素材' : selected.imageLabel}</span><div className="hook-preview-copy"><small>ECOMMERCE HOOK · 9:16</small><b>{selected.hook}</b><span>{selected.duration} · 静音可理解</span></div><button aria-label={previewing ? '暂停钩子预览' : '播放钩子预览'} onClick={() => setPreviewing(value => !value)}><Play size={17} fill="currentColor"/></button></>}
        </div>
        <div className="hook-proof"><span className="section-label">策略依据</span><h3>先建立信息缺口，再完成一次清晰变化。</h3><p>一个主动作、一个结果状态、一个稳定的商品定格。环境只提供辅助运动。</p><div>{selected.tags.map(tag => <span key={tag}>{tag}</span>)}</div></div>
      </div>
      <div className="hook-storyboard">{storyboard.map((step, index) => <div key={step.time}><span>{step.time}</span><i/><b>{String(index + 1).padStart(2, '0')} · {step.name}</b><small>{step.detail}</small></div>)}</div>
    </section>
    <aside className="hook-inspector">
      <div className="surface-toolbar"><h3>提示词构建器</h3><span>{usingFixture ? '娇兰固定样例' : selectedSource?.status === 'approved' ? '策略包已批准' : 'Brief 已确认'}</span></div>
      <label>创意来源<select value={selectedSourceKey} onChange={event => setSelectedSourceKey(event.target.value)}>
        {sourceOptions.map(option => {
          const key = `${option.source_ref.kind}:${option.source_ref.id}:${option.source_ref.version}`
          const sourceType = option.source_ref.kind === 'strategy_package' ? '策略包' : 'Brief'
          return <option key={key} value={key}>{option.product.brand_name || '未命名品牌'} · {option.product.product_name || '未命名商品'} · {sourceType} v{option.source_ref.version}</option>
        })}
        <option value="fixture:guerlain">娇兰第三代黄金复原蜜 · 固定样例</option>
      </select></label>
      <label>商品保真约束<textarea value={fidelity} onChange={event => setFidelity(event.target.value)}/></label>
      <label>镜头与光影<textarea value={camera} onChange={event => setCamera(event.target.value)}/></label>
      <label>唯一主动作<textarea value={motion} onChange={event => setMotion(event.target.value)}/></label>
      <label>结果与停留<textarea value={result} onChange={event => setResult(event.target.value)}/></label>
      <div className="hook-guardrail"><ShieldCheck size={15}/><span><b>自动附加生成护栏</b><small>{guardrails}</small></span></div>
      {configuredProvider ? <div className="hook-model"><CircleCheck size={15}/><span><b>{configuredProvider.name}</b><small>服务端媒体模型目录</small></span></div> : <div className="hook-model missing"><CircleAlert size={15}/><span><b>尚未配置模型</b><small>请在服务端配置 ARK_API_KEY 后重新检查能力。</small></span></div>}
      {effectiveSourceNotice ? <div className={prepared && !effectivePreparedReady ? 'hook-model missing' : 'hook-model'}><CircleAlert size={15}/><span><b>来源与准备状态</b><small>{effectiveSourceNotice}</small></span></div> : null}
      <div className="hook-actions"><button className="secondary-button" disabled={preparing} onClick={() => void save()}><Save size={14}/>保存策略</button><button className="primary-button" disabled={!configuredProvider || preparing || Boolean(selectedSource && !effectivePreparedReady) || ['queued', 'running'].includes(job?.status ?? '')} onClick={() => void generate()}><WandSparkles size={14}/>{preparing ? '准备素材…' : job && ['queued', 'running'].includes(job.status) ? '生成中…' : generatedAsset ? '重新生成视频' : '生成视频'}</button></div>
      {job ? <div className="inline-notice" role="status">任务 {shortId(job.id)} · {job.status} · {job.model ?? '模型待分配'}{job.diagnostic ? ` · ${job.diagnostic}` : ''}</div> : null}
    </aside>
  </div>
}

function featureForVideoAsset(asset: ApiArtifact, features: ApiAssetFeature[]): ApiAssetFeature | undefined {
  return features
    .filter(feature => feature.assetId === asset.id && feature.assetVersion === asset.version)
    .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))[0]
}

function videoFeatureSummary(feature: ApiAssetFeature): string {
  const sellingPoint = feature.sellingPoints[0] ? ` · ${feature.sellingPoints[0]}` : ''
  return `商品露出 ${featurePercent(feature.productVisibility)} · ${riskText(feature.similarityRisk)}${sellingPoint}`
}

function featurePercent(value: number): string {
  return `${Math.round(value * 100)}%`
}

function riskText(risk: ApiAssetFeature['similarityRisk']): string {
  return risk === 'high' ? '高相似风险' : risk === 'medium' ? '中相似风险' : '低相似风险'
}

export function ReportCenterPage({ state }: { state: DataState }) {
  const { currentProject, updateArtifact } = useProject()
  const [section, setSection] = useState('执行摘要')
  const [version, setVersion] = useState(4)
  const [notice, setNotice] = useState('')
  const sections = ['执行摘要', '发生了什么', '为什么发生', '创意样本', '下一步行动']
  const evidence = currentProject.operations.filter(record => record.kind === 'evidence')
  const metric = currentProject.operations.find(record => record.kind === 'metric')
  const metricField = (key: string, fallback: string) => String(metric?.fields[key] ?? fallback)
  const save = async () => {
    const nextVersion = `v1.${version + 1}`
    try {
      await updateArtifact('insight', { version: nextVersion, status: '已确认', sourceVersion: `创意 ${currentProject.artifacts.creative.version}`, summary: '证据前置版本点击率较基线提升 18%，95% 置信范围 +12% 至 +23%' })
      setVersion(value => value + 1)
      setNotice(`报告 ${nextVersion} 已保存`)
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '保存报告失败，请重试。')
    }
  }
  return <StateBoundary
    state={state}
    contextLabel="素材洞察 / 报告中心"
    emptyTitle="当前 Project 暂无可保存报告"
    emptyDetail="生成投后复盘或沉淀经验后，报告中心会展示版本、引用来源和导出动作。"
    errorDetail="报告版本或引用证据读取失败。请确认服务端可用后重新加载。"
  ><div className="report-workspace">
    <aside className="report-outline"><div className="surface-toolbar"><h3>报告结构</h3><button aria-label="新增报告章节"><FileText size={15}/></button></div>{sections.map((item, index) => <button className={section === item ? 'active' : ''} key={item} onClick={() => setSection(item)}><span>{String(index + 1).padStart(2, '0')}</span>{item}</button>)}<div className="version-block"><span>报告版本</span><b>v1.{version}</b><small>数据截止 2026-07-22 16:00</small></div></aside>
    <article className="report-document"><div className="document-meta"><span>{currentProject.name}</span><span>效果分析报告 v1.{version}</span><button onClick={() => setNotice('PDF 导出任务已创建')}><Download size={14}/>导出 PDF</button></div><h1>{section === '执行摘要' ? metricField('summary', '暂无服务端指标摘要。') : section}</h1><p className="report-lead">{metric?.title ?? '暂无服务端趋势记录。'}</p><div className="report-metric-line"><div><small>当前指标</small><b>{metricField('latest', '—')}</b><span>{metricField('comparison', '暂无对比数据')}</span></div><div><small>样本</small><b>{metricField('sample', '—')}</b><span>服务端已存档</span></div><div><small>置信范围</small><b>{metricField('confidence', '—')}</b><span>{metricField('unit', '—')}</span></div></div><h2>结论与边界</h2><p>{metricField('scope', '暂无服务端适用范围说明。')}</p><div className="report-callout"><b>建议行动</b><p>{metricField('recommendation', '暂无服务端建议动作。')}</p></div></article>
    <aside className="report-sources"><div className="surface-toolbar"><h3>引用与版本</h3><button aria-label="报告更多操作"><ChevronDown size={15}/></button></div>{evidence.map(item => <button key={item.id}><span>{item.id}</span><div><b>{item.title}</b><small>{String(item.fields.source ?? '—')} · {new Date(item.occurredAt).toLocaleDateString('zh-CN')}</small></div><ExternalLink size={13}/></button>)}{!evidence.length ? <div className="panel-empty">暂无服务端证据记录。</div> : null}<button className="primary-button full" onClick={() => void save()}><Save size={15}/>保存报告版本</button>{notice ? <div className="inline-notice" role="status">{notice}</div> : null}</aside>
  </div></StateBoundary>
}

type DeliveryGateCheck = {
  code: string
  label: string
  passed: boolean
  repair: string
}

type DeliveryGateGroup = {
  title: string
  checks: DeliveryGateCheck[]
}

function statusIsHealthy(status?: ApiAdAccountBinding['permissionStatus']) {
  return status === 'normal'
}

function confirmedMaterialFor(pointer: ApiAssetVersionPointer, confirmations: ApiMaterialConfirmation[]) {
  return confirmations.find(item => item.projectId === pointer.projectId && item.assetId === pointer.assetId && item.assetVersion === pointer.workingVersion && item.status === 'confirmed')
}

function deliveryPlanSignature(account: ApiAdAccountBinding | undefined, budget: number, materials: ApiAssetVersionPointer[]) {
  const materialPart = materials.map(item => `${item.assetId}@v${item.workingVersion}`).sort().join('|') || 'no-material'
  return [account?.id ?? 'no-account', budget, materialPart].join(':')
}

function deliveryPlanVersion(signature: string) {
  let hash = 0
  for (const char of signature) hash = (hash * 31 + char.charCodeAt(0)) % 100000
  return `plan-v${String(hash).padStart(5, '0')}`
}

function buildDeliveryGateGroups(account: ApiAdAccountBinding | undefined, budget: number, budgetLimit: number, materials: ApiAssetVersionPointer[], confirmations: ApiMaterialConfirmation[]): DeliveryGateGroup[] {
  const confirmedCount = materials.filter(pointer => confirmedMaterialFor(pointer, confirmations)).length
  return [
    {
      title: '输入完整性',
      checks: [
        { code: 'account', label: account ? `账户已选择：${account.accountName}` : '未选择广告账户', passed: Boolean(account), repair: '选择与当前 Project 绑定的广告账户。' },
        { code: 'budget', label: `预算 ¥${budget.toLocaleString('zh-CN')} / 护栏 ¥${budgetLimit.toLocaleString('zh-CN')}`, passed: budget > 0 && budget <= budgetLimit, repair: '预算必须大于 0 且不超过 Project 护栏。' },
        { code: 'materials', label: `素材组合 ${materials.length} 个版本`, passed: materials.length > 0, repair: '至少选择一个已纳入当前 Project 的素材版本。' },
      ],
    },
    {
      title: '账户权限',
      checks: [
        { code: 'permission', label: `权限：${account?.permissionStatus ?? '未连接'}`, passed: statusIsHealthy(account?.permissionStatus), repair: '重新授权广告账户或联系账户负责人。' },
        { code: 'login', label: `登录：${account?.loginStatus ?? '未连接'}`, passed: statusIsHealthy(account?.loginStatus), repair: '恢复账户登录状态后重新预检。' },
      ],
    },
    {
      title: '素材品牌版权',
      checks: [
        { code: 'human-confirmed', label: `人工确认版本 ${confirmedCount}/${materials.length}`, passed: materials.length > 0 && confirmedCount === materials.length, repair: '仅允许使用 MaterialConfirmation 已确认的当前素材版本。' },
        { code: 'brand-scope', label: '品牌、版权和使用范围绑定到当前 Project', passed: materials.length > 0 && confirmedCount === materials.length, repair: '回到素材检查页完成品牌版权复核和人工确认。' },
      ],
    },
    {
      title: '预算追踪回滚',
      checks: [
        { code: 'tracking', label: `像素追踪：${account?.trackingStatus ?? '未连接'}`, passed: statusIsHealthy(account?.trackingStatus), repair: '修复像素或转化 API 追踪异常。' },
        { code: 'rollback', label: '已配置模拟执行证据和回滚说明', passed: Boolean(account) && budget > 0, repair: '补齐账户与预算后才能生成可回滚执行证据。' },
      ],
    },
  ]
}

function gateGroupsPassed(groups: DeliveryGateGroup[]) {
  return groups.every(group => group.checks.every(check => check.passed))
}

function LegacyDeliveryPlanPage({ state }: { state: DataState }) {
  const { currentProject, addChangeSet, preflightChangeSet } = useProject()
  const industry = industryProfile(currentProject.industry)
  const [step, setStep] = useState('计划配置')
  const [notice, setNotice] = useState('')
  const [budget, setBudget] = useState(currentProject.budget)
  const [latest, setLatest] = useState<DeliveryChangeSet>()
  const [busy, setBusy] = useState(false)
  const [workbench, setWorkbench] = useState<ApiAgencyWorkbench | null>(null)
  const [selectedAccountId, setSelectedAccountId] = useState('')
  const [preflightSignature, setPreflightSignature] = useState('')
  const planPeriod = '2026-07-25 至 2026-08-31'
  const audience = `${currentProject.brand} 高意向人群 / 近 30 天互动用户`
  const landingPage = `https://demo.cookies.local/lp/${currentProject.code.toLowerCase()}`
  const pixelId = `PX-${currentProject.code}-LEAD`
  const namingRule = `${currentProject.code}_{{account}}_{{asset}}_{{date}}`
  const projectAccounts = useMemo(() => workbench?.adAccountBindings.filter(account => account.projectIds.includes(currentProject.id)) ?? [], [currentProject.id, workbench])
  const selectedAccount = projectAccounts.find(account => account.id === selectedAccountId) ?? projectAccounts[0]
  const materials = useMemo(() => workbench?.assetVersionPointers.filter(pointer => pointer.projectId === currentProject.id) ?? [], [currentProject.id, workbench])
  const confirmations = workbench?.materialConfirmations ?? []
  const gateGroups = useMemo(() => buildDeliveryGateGroups(selectedAccount, budget, currentProject.budget, materials, confirmations), [budget, confirmations, currentProject.budget, materials, selectedAccount])
  const planSignature = deliveryPlanSignature(selectedAccount, budget, materials)
  const planVersion = deliveryPlanVersion(planSignature)
  const preflightStale = Boolean(latest?.preflight) && Boolean(preflightSignature) && preflightSignature !== planSignature
  const canRunPreflight = latest !== undefined && latest.status === 'draft' && gateGroupsPassed(gateGroups)
  useEffect(() => {
    setBudget(currentProject.budget)
    setSelectedAccountId('')
    setPreflightSignature('')
  }, [currentProject.id, currentProject.budget])
  useEffect(() => {
    let active = true
    void Promise.all([deliveryApi.listChangeSets(currentProject.id), api.listAgencyWorkbench({ projectIds: [currentProject.id] })]).then(([records, agency]) => {
      if (!active) return
      const changeSet = records.at(-1)
      setLatest(changeSet)
      setWorkbench(agency)
      if (changeSet?.preflight?.passed) {
        const account = agency.adAccountBindings.find(item => item.projectIds.includes(currentProject.id))
        const projectMaterials = agency.assetVersionPointers.filter(item => item.projectId === currentProject.id)
        setPreflightSignature(deliveryPlanSignature(account, currentProject.budget, projectMaterials))
      }
    }).catch(() => undefined)
    return () => { active = false }
  }, [currentProject.id])
  const createChange = async () => {
    setBusy(true)
    try {
      const changeSet = await addChangeSet(budget)
      setLatest(changeSet)
      setPreflightSignature('')
      setNotice(`${changeSet.id} 已在服务端创建；当前计划版本为 ${planVersion}，尚未执行任何真实广告平台写入。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '创建 ChangeSet 失败')
    } finally {
      setBusy(false)
    }
  }
  const preflight = async () => {
    if (!latest) return
    if (!gateGroupsPassed(gateGroups)) {
      setNotice('预检未通过：请先修复账户权限、预算、素材人工确认或追踪回滚问题。')
      return
    }
    setBusy(true)
    try {
      const changeSet = await preflightChangeSet(latest.id)
      setLatest(changeSet)
      setPreflightSignature(planSignature)
      setNotice(changeSet.preflight?.passed ? `预检通过并绑定 ${planVersion}，可进入执行确认。` : '预检未通过，请按修复建议补齐输入。')
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '预检失败')
    } finally {
      setBusy(false)
    }
  }
  return <StateBoundary
    state={state}
    contextLabel="智能投放 / 投放计划"
    emptyTitle="当前 Project 暂无投放计划"
    emptyDetail="先选择广告账户、素材组合和预算排期，再生成服务端 ChangeSet 进入预检。"
    errorDetail="投放计划、账户或素材门禁读取失败。请确认服务端和代理商工作台 API 可用后重新加载。"
    createLabel="生成 ChangeSet"
    onCreate={() => { void createChange() }}
   ><div className="delivery-plan-workspace">
     <IndustrySchema module="智能投放" industry={industry.label} profile={industry.delivery}/>
    <section className="plan-main"><ArtifactFlow compact/><div className="plan-tabs">{['计划配置', '素材组合', '预算与排期', '校验'].map(item => <button className={step === item ? 'active' : ''} key={item} onClick={() => setStep(item)}>{item}</button>)}</div><div className="plan-form"><div><label>计划名称<input defaultValue="销售线索增长计划 06"/></label><label>广告账户<select value={selectedAccount?.id ?? ''} onChange={event => setSelectedAccountId(event.target.value)}>{projectAccounts.length ? projectAccounts.map(account => <option key={account.id} value={account.id}>{account.platform} · {account.accountName}</option>) : <option value="">无绑定账户</option>}</select></label></div><div><label>总预算（CNY）<input type="number" value={budget} onChange={event => setBudget(Number(event.target.value))}/></label><label>投放周期<input readOnly value={planPeriod}/></label></div><div><label>受众<input readOnly value={audience}/></label><label>落地页<input readOnly value={landingPage}/></label></div><div><label>像素<input readOnly value={pixelId}/></label><label>命名规则<input readOnly value={namingRule}/></label></div><label>素材组合<div className="delivery-material-combo">{materials.map(pointer => { const confirmation = confirmedMaterialFor(pointer, confirmations); return <span key={pointer.id} className={confirmation ? 'confirmed' : 'blocked'}><b>{pointer.assetId} v{pointer.workingVersion}</b><small>{confirmation ? `已人工确认 · ${confirmation.confirmedBy}` : '未人工确认，禁止执行'}</small></span> })}{materials.length === 0 ? <span className="blocked"><b>暂无素材版本</b><small>请先完成素材制作和人工确认。</small></span> : null}</div></label></div><div className="validation-list delivery-gate-list"><h3>上线前预检 · {planVersion}</h3>{gateGroups.flatMap(group => group.checks.map(check => <span key={`${group.title}-${check.code}`} className={check.passed ? '' : 'preflight-failed'}>{check.passed ? <CircleCheck size={16}/> : <CircleAlert size={16}/>}<b>{group.title} · {check.label}</b>{!check.passed ? <small>{check.repair}</small> : null}</span>))}</div></section>
    <aside className="changeset-panel"><div className="surface-toolbar"><h3>ChangeSet</h3><span className="source-chip">本地模拟</span></div>{latest ? <><div className="changeset-title"><span>{latest.id} · v{latest.version}</span><h2>{latest.name}</h2><small>预算边界 ¥{latest.budgetLimit?.toLocaleString('zh-CN') ?? 0} · {latest.status}</small><small>当前计划版本 {planVersion}</small></div>{latest.preflight ? <div className="validation-list delivery-gate-list">{preflightStale ? <span className="preflight-failed"><CircleAlert size={16}/><b>预检版本已失效</b><small>计划账户、预算或素材组合变化后，必须重新生成 ChangeSet 并预检。</small></span> : <span><CircleCheck size={16}/><b>预检绑定 {planVersion}</b><small>{latest.preflight.checkedAt}</small></span>}{latest.preflight.checks.map(check => <span key={check.code} className={check.passed ? '' : 'preflight-failed'}>{check.passed ? <CircleCheck size={16}/> : <CircleAlert size={16}/>}<b>{check.message}</b>{!check.passed ? <small>{check.repair}</small> : null}</span>)}</div> : <div className="rollback-copy"><ShieldCheck size={16}/><span><b>待运行预检</b><small>系统会校验输入完整性、账户权限、素材品牌版权、预算追踪回滚四组门禁。</small></span></div>}<div className="rollback-copy"><ShieldCheck size={16}/><span><b>执行确认门禁</b><small>仅当预检绑定当前计划版本且素材均为人工确认版本时允许执行。</small></span></div></> : <div className="panel-empty">尚未创建服务端 ChangeSet</div>}<button className="secondary-button full" onClick={createChange} disabled={busy}>生成 ChangeSet</button><button className="primary-button full" onClick={preflight} disabled={!canRunPreflight || busy}><Send size={15}/>{latest?.status === 'preflight_passed' && !preflightStale ? '已通过预检' : '运行上线前预检'}</button>{notice ? <div className="inline-notice" role="status">{notice}</div> : null}</aside>
  </div></StateBoundary>
}

export function LegacyApprovalCenterPage({ state }: { state: DataState }) {
  const { currentProject, approveChangeSet, executeChangeSet, rollbackChangeSet } = useProject()
  const [changeSets, setChangeSets] = useState<DeliveryChangeSet[]>([])
  const [selectedId, setSelectedId] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [workbench, setWorkbench] = useState<ApiAgencyWorkbench | null>(null)
  const selected = useMemo(() => changeSets.find(item => item.id === selectedId), [changeSets, selectedId])
  const projectAccounts = workbench?.adAccountBindings.filter(account => account.projectIds.includes(currentProject.id)) ?? []
  const selectedAccount = projectAccounts[0]
  const materials = workbench?.assetVersionPointers.filter(pointer => pointer.projectId === currentProject.id) ?? []
  const confirmations = workbench?.materialConfirmations ?? []
  const approvalGateGroups = buildDeliveryGateGroups(selectedAccount, selected?.budgetLimit ?? currentProject.budget, currentProject.budget, materials, confirmations)
  const executionGatePassed = gateGroupsPassed(approvalGateGroups)
  const objectCount = Math.max(materials.length, 1) * (selectedAccount ? 1 : 0)
  const riskLabel = executionGatePassed ? '低：账户、素材、预算和回滚均已满足' : '高：存在未确认素材、账户异常或预算追踪阻断'
  const refresh = async () => {
    setBusy(true)
    try {
      const [records, agency] = await Promise.all([deliveryApi.listChangeSets(currentProject.id), api.listAgencyWorkbench({ projectIds: [currentProject.id] })])
      setChangeSets(records)
      setWorkbench(agency)
      setSelectedId(current => records.some(item => item.id === current) ? current : records[0]?.id ?? '')
      setNotice(records.length ? '已从服务端加载投放模拟队列。' : '尚未创建服务端 ChangeSet，请先在投放计划中生成。')
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '加载审批队列失败')
    } finally {
      setBusy(false)
    }
  }
  useEffect(() => { void refresh() }, [currentProject.id])
  const apply = async (action: 'approve' | 'execute' | 'rollback') => {
    if (!selected) return
    if (action === 'execute' && !executionGatePassed) {
      setNotice('执行被拦截：素材必须是人工确认版本，且账户、预算、追踪和回滚门禁均需通过。')
      return
    }
    setBusy(true)
    try {
      const updated = action === 'approve' ? await approveChangeSet(selected.id) : action === 'execute' ? await executeChangeSet(selected.id) : await rollbackChangeSet(selected.id, '演示用户确认回滚模拟结果')
      setChangeSets(current => current.map(item => item.id === updated.id ? updated : item))
      setNotice(action === 'approve' ? '已由演示审批人批准，可执行本地模拟。' : action === 'execute' ? '模拟执行完成，未写入真实广告平台。' : '模拟回滚完成，原计划未受真实平台影响。')
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '投放模拟操作失败')
    } finally {
      setBusy(false)
    }
  }
  return <StateBoundary
    state={state}
    contextLabel="智能投放 / 审批中心"
    emptyTitle="当前 Project 暂无待审批 ChangeSet"
    emptyDetail="从投放计划生成并通过预检后，ChangeSet 会进入审批队列；这里不会展示其他 Project 的审批状态。"
    errorDetail="审批队列暂时无法读取。请确认投放服务可用后刷新队列。"
    retryLabel="刷新审批队列"
    onRetry={() => { void refresh() }}
  ><div className="approval-workspace">
    <aside className="approval-queue"><div className="surface-toolbar"><h3>审批队列</h3><button onClick={() => void refresh()} disabled={busy} aria-label="刷新审批队列"><RotateCcw size={15}/></button></div>{changeSets.map(item => <button key={item.id} className={selectedId === item.id ? 'active' : ''} onClick={() => setSelectedId(item.id)}><span>{shortId(item.id)}</span><b>{item.name}</b><small>{item.status} · ¥{item.budgetLimit?.toLocaleString('zh-CN') ?? 0}</small></button>)}</aside>
    <section className="approval-detail">{selected ? <><div className="approval-heading"><div><span>{shortId(selected.id)} · ChangeSet v{selected.version}</span><h2>{selected.name}</h2><p>服务端受控投放模拟。只有预检通过、人工批准且执行确认门禁通过后才能执行。</p></div><span className={`approval-status ${selected.status}`}>{selected.status}</span></div><div className="execution-confirmation"><h3>执行确认</h3><div><b>账户</b><span>{selectedAccount ? `${selectedAccount.platform} · ${selectedAccount.accountName}` : '无绑定账户'}</span></div><div><b>预算</b><span>¥{selected.budgetLimit?.toLocaleString('zh-CN') ?? currentProject.budget.toLocaleString('zh-CN')}</span></div><div><b>对象数量</b><span>{objectCount} 个广告对象 / {materials.length} 个素材版本</span></div><div><b>预计影响</b><span>仅本地模拟执行，记录投放对象、预算和审计证据。</span></div><div><b>风险</b><span className={executionGatePassed ? '' : 'danger-text'}>{riskLabel}</span></div><div><b>回滚能力</b><span>支持模拟回滚并保留原因、时间和执行证据。</span></div></div><div className="approval-evidence"><h3>素材人工确认版本</h3>{materials.map(pointer => { const confirmation = confirmedMaterialFor(pointer, confirmations); return <div key={pointer.id}><ClipboardCheck size={16}/><span><b>{pointer.assetId} v{pointer.workingVersion}</b><small>{confirmation ? `已确认 · ${confirmation.confirmedBy} · ${confirmation.createdAt}` : '未人工确认，禁止执行'}</small></span></div> })}{materials.length === 0 ? <div><CircleAlert size={16}/><span><b>暂无素材版本</b><small>请先完成素材检查和人工确认。</small></span></div> : null}</div><div className="approval-evidence"><h3>预检与执行证据</h3>{selected.preflight?.checks.map(check => <div key={check.code}><ClipboardCheck size={16}/><span><b>{check.message}</b><small>{check.passed ? '预检通过' : check.repair}</small></span></div>)}{approvalGateGroups.flatMap(group => group.checks.filter(check => !check.passed).map(check => <div key={`${group.title}-${check.code}`}><CircleAlert size={16}/><span><b>{group.title} · {check.label}</b><small>{check.repair}</small></span></div>))}{selected.execution?.evidence.map(item => <div key={item.step}><CircleCheck size={16}/><span><b>{item.message}</b><small>{item.recordedAt}</small></span></div>)}</div>{selected.rollback ? <div className="rollback-copy"><RotateCcw size={16}/><span><b>已完成模拟回滚</b><small>{selected.rollback.reason}</small></span></div> : null}<div className="approval-actions"><button className="secondary-button" onClick={() => void apply('rollback')} disabled={busy || selected.status !== 'executed'}><RotateCcw size={15}/>回滚模拟</button><button className="secondary-button" onClick={() => void apply('execute')} disabled={busy || selected.status !== 'approved' || !executionGatePassed}><Play size={15}/>模拟执行</button><button className="primary-button" onClick={() => void apply('approve')} disabled={busy || selected.status !== 'preflight_passed'}><ThumbsUp size={15}/>以演示审批人批准</button></div></> : <div className="panel-empty">没有服务端 ChangeSet</div>}{notice ? <div className="inline-notice" role="status">{notice}</div> : null}</section>
    <aside className="approval-audit"><span className="section-label">权限与边界</span><div><time>演示角色</time><span>demo-approver</span></div><div><time>执行范围</time><span>本地模拟，无真实广告平台写入</span></div><div><time>审计</time><span>预检、审批、执行和回滚均由服务端记录</span></div><div><time>硬门禁</time><span>未人工确认素材不能执行</span></div></aside>
  </div></StateBoundary>
}
