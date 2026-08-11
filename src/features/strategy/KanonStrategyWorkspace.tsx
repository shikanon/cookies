import {
  AlertCircle,
  Archive,
  BadgeCheck,
  BookOpen,
  Check,
  ChevronRight,
  CircleCheck,
  ExternalLink,
  FileText,
  LoaderCircle,
  LockKeyhole,
  MessageSquare,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  ShieldCheck,
  Sparkles,
  Upload,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useProject } from '../../context/ProjectContext'
import type { SystemKey } from '../../types'
import { CreativeTaskPlanner } from './CreativeTaskPlanner'
import { StrategyConversationPane } from './StrategyConversationPane'
import { useStrategyWorkspace } from './useStrategyWorkspace'
import type {
  BriefDraft,
  DraftRevision,
  KnowledgeDocument,
  Review,
  StrategyDocument,
  StrategyDraft,
} from './types'

type Props = {
  activeView: string
  workspaceId?: string
  onOpenWorkspace: (workspaceId: string, view: string) => void
  onOpenCreative: (navId: string, view: string, contextId: string) => void
  onOpenProject: (id: string, system?: SystemKey, navId?: string, objectId?: string, view?: string, contextId?: string) => void
}

export function KanonStrategyWorkspace({
  activeView,
  workspaceId,
  onOpenWorkspace,
  onOpenCreative,
  onOpenProject,
}: Props) {
  const { currentProject } = useProject()
  const { state, actions } = useStrategyWorkspace(currentProject.id, workspaceId)
  const mainRef = useRef<HTMLElement>(null)
  useEffect(() => {
    mainRef.current?.scrollTo({ top: 0 })
  }, [activeView])

  if (state.isLoading) {
    return <div className="kanon-strategy-state" role="status">
      <LoaderCircle className="spin" size={24}/>
      <h2>正在恢复策略工作区</h2>
      <p>读取真实对话、Brief、策略版本和评审状态。</p>
    </div>
  }

  if (!state.detail) {
    return <div className="kanon-strategy-state">
      <MessageSquare size={28}/>
      <h2>创建当前 Project 的策略工作区</h2>
      <p>工作区会持久化同一条对话、Brief、策略修订和评审结果。</p>
      {state.error ? <div className="kanon-strategy-alert" role="alert"><AlertCircle size={15}/>{state.error}</div> : null}
      <button className="primary-button" disabled={Boolean(state.busy)} onClick={() => void actions.createWorkspace()}>
        <Plus size={16}/>{state.busy === 'workspace' ? '创建中…' : '创建主策略工作区'}
      </button>
    </div>
  }

  if (!state.detail.current_conversation || !state.detail.current_task) {
    return <div className="kanon-strategy-state">
      <Sparkles size={28}/>
      <h2>开始需求对话</h2>
      <p>系统将创建持久对话和 Brief 草稿；刷新页面后可以继续。</p>
      {state.error ? <div className="kanon-strategy-alert" role="alert"><AlertCircle size={15}/>{state.error}</div> : null}
      <button className="primary-button" disabled={Boolean(state.busy)} onClick={() => void actions.startConversation()}>
        <MessageSquare size={16}/>{state.busy === 'conversation' ? '正在启动…' : '开始策略梳理'}
      </button>
    </div>
  }

  const lifecycleLocked = Boolean(state.detail.current_task.discarded_at || state.draft?.archived_at)
  const showSummaryRail = activeView === '评审'
  const hasEvidence = Boolean(
    state.documents.length ||
    state.brief?.document.reference_ids?.length ||
    state.researchRun?.artifacts.length,
  )
  const showEvidenceRail = activeView === '研究' || (activeView === '策略' && hasEvidence)
  const railMode = showSummaryRail ? 'rail-summary' : showEvidenceRail ? 'rail-evidence' : 'rails-none'
  return <div className={`kanon-strategy-root${activeView === '对话' ? ' conversation-active' : ''}`}>
    {state.error ? <div className="kanon-strategy-alert" role="alert">
      <AlertCircle size={15}/><span>{state.error}</span>
      <button aria-label="重新加载策略工作区" onClick={() => void actions.reload()}><RefreshCw size={14}/></button>
    </div> : null}
    {lifecycleLocked ? <div className="kanon-lifecycle-banner" role="status"><Archive size={15}/><span><b>{state.detail.current_task.discarded_at ? '任务已废弃' : '策略已归档'}</b>当前工作区为只读，完整对话、Brief、策略版本和评审记录均已保留。请从“策略任务 → 已归档”恢复后继续操作。</span></div> : null}
    <div className="kanon-workspace-contextbar">
      <div><span>当前工作链</span><strong>{state.detail.workspace.name}</strong><small><LockKeyhole size={12}/>已锁定到当前 Project</small></div>
      <label><span>切换工作区</span><select
        aria-label="切换策略工作区"
        value={state.detail.workspace.id}
        onChange={event => onOpenWorkspace(event.target.value, activeView)}
      >{state.workspaces.map(workspace => <option key={workspace.id} value={workspace.id}>{workspace.name}{workspace.is_primary ? ' · 主工作区' : ''}</option>)}</select></label>
      <button className="icon-button" aria-label="刷新当前策略工作区" disabled={Boolean(state.busy)} onClick={() => void actions.reload()}><RefreshCw size={15}/></button>
    </div>
    <div className={`kanon-strategy-workspace ${railMode}`}>
      <main className="kanon-strategy-main" ref={mainRef}>
        <fieldset className="kanon-lifecycle-lock" disabled={lifecycleLocked}>
        {activeView === '概览' ? <OverviewPane
          state={state}
          onNavigate={view => onOpenProject(
            currentProject.id,
            'strategy',
            'workspaces',
            state.detail?.workspace.id,
            view,
          )}
        /> : null}
        {activeView === '对话' ? <StrategyConversationPane
          brief={state.brief}
          briefVersion={state.briefVersion}
          busy={state.busy}
          conversationCapabilities={state.conversationCapabilities}
          documents={state.documents}
          mediaArtifacts={state.mediaArtifacts}
          messages={state.messages}
          onConfirmRequirement={actions.confirmRequirement}
          onOpenBrief={() => onOpenProject(
            currentProject.id,
            'strategy',
            'workspaces',
            state.detail?.workspace.id,
            'Brief',
          )}
          onOpenFullStrategy={() => onOpenProject(
            currentProject.id,
            'strategy',
            'workspaces',
            state.detail?.workspace.id,
            '策略',
          )}
          onReadyViralRemake={taskId => onOpenCreative('video', '效果广告', taskId)}
          onSend={actions.sendMessage}
          onStartViralRemake={actions.createRequirementViralRemake}
          onUploadDocument={actions.uploadConversationDocument}
          onUploadMedia={actions.uploadConversationMedia}
          pending={Boolean(state.pendingAgentTaskId)}
        /> : null}
        {activeView === 'Brief' ? <BriefPane
          brief={state.brief}
          busy={state.busy}
          onConfirm={actions.confirmBrief}
          onConfirmFields={actions.confirmBriefFields}
          onField={actions.patchBriefField}
        /> : null}
        {activeView === '策略' ? <StrategyPane
          busy={state.busy}
          draft={state.draft}
          readiness={state.readiness}
          probe={state.probe}
          briefReady={Boolean(state.briefVersion)}
          onGenerate={actions.generateStrategy}
          onPatch={actions.patchStrategySection}
          onProbe={actions.probeGeneration}
          onRetry={actions.retryStrategy}
          onRevise={actions.reviseStrategy}
          onSubmit={actions.submitStrategy}
          pending={Boolean(state.pendingAgentTaskId)}
        /> : null}
        {activeView === '创意任务策略' ? <CreativeTaskPlanner
          briefVersion={state.briefVersion}
          draft={state.draft}
          onCreateRouteRevision={value => actions.patchStrategySection('channel_strategy', value)}
          onOpenCreative={onOpenCreative}
          onOpenStrategy={() => onOpenProject(
            currentProject.id,
            'strategy',
            'workspaces',
            state.detail?.workspace.id,
            '策略',
          )}
          projectId={currentProject.id}
        /> : null}
        {activeView === '评审' ? <ReviewPane
          busy={state.busy}
          comments={state.comments}
          deepReview={state.deepReview}
          deepReviewError={state.deepReviewError}
          draft={state.draft}
          review={state.review}
          revisions={state.revisions}
          onAddComment={actions.addComment}
          onApprove={actions.approveReview}
          onDeepReview={actions.startDeepReview}
          onReturn={actions.returnReview}
          onOpenCreative={() => onOpenProject(
            currentProject.id,
            'strategy',
            'workspaces',
            state.detail?.workspace.id,
            '创意任务策略',
          )}
        /> : null}
        {activeView === '研究' ? <ResearchPane
          brief={state.brief}
          busy={state.busy}
          documents={state.documents}
          researchRun={state.researchRun}
          onAdoption={actions.setResearchArtifactAdoption}
          onResearch={actions.runResearch}
          onUpload={actions.uploadDocument}
        /> : null}
        {activeView === '实验' ? <UnavailablePane
          title="实验编排尚未开放"
          detail="当前不会用静态实验结果冒充真实数据。策略中的实验矩阵会如实保留，后续接入素材与投放实验执行。"
        /> : null}
        {activeView === '变更记录' ? <ChangeLogPane
          comments={state.comments}
          revisions={state.revisions}
          review={state.review}
        /> : null}
        </fieldset>
      </main>
      {showSummaryRail ? <SummaryRail
        brief={state.brief}
        briefVersion={state.briefVersion?.version}
        draft={state.draft}
        publishedVersion={state.published?.version}
        review={state.review}
        workspaceName={state.detail.workspace.name}
      /> : null}
      {showEvidenceRail ? <EvidenceRail
        documents={state.documents}
        referenceIds={state.brief?.document.reference_ids ?? []}
        researchArtifacts={state.researchRun?.artifacts ?? []}
      /> : null}
    </div>
  </div>
}

type WorkspaceState = ReturnType<typeof useStrategyWorkspace>['state']

type StrategyWorkspaceView = '对话' | 'Brief' | '研究' | '策略' | '评审' | '创意任务策略'

function OverviewPane({ state, onNavigate }: { state: WorkspaceState; onNavigate: (view: StrategyWorkspaceView) => void }) {
  const document = state.draft?.revision?.document
  const hasConversation = state.messages.some(message => message.role === 'assistant')
  const briefConfirmed = state.brief?.status === 'confirmed'
  const strategyReady = Boolean(state.draft?.current_revision)
  const reviewApproved = state.review?.status === 'approved'
  const packagePublished = Boolean(state.published)
  const stages = [
    {
      label: '需求对话',
      view: '对话' as const,
      complete: hasConversation,
      detail: `${state.messages.length} 条消息`,
    },
    {
      label: 'Brief',
      view: 'Brief' as const,
      complete: briefConfirmed,
      detail: briefConfirmed
        ? `已冻结 v${state.briefVersion?.version ?? state.brief?.version ?? 1}`
        : state.brief?.completeness.ready ? '可以确认' : `${state.brief?.completeness.blockers.length ?? 0} 个阻断项`,
    },
    {
      label: '策略',
      view: '策略' as const,
      complete: strategyReady,
      detail: state.draft ? `Revision ${state.draft.current_revision} · ${statusLabel(state.draft.status)}` : '等待生成',
    },
    {
      label: '评审',
      view: '评审' as const,
      complete: reviewApproved,
      detail: state.review ? statusLabel(state.review.status) : '尚未提交',
    },
    {
      label: '创意交接',
      view: '创意任务策略' as const,
      complete: false,
      available: packagePublished,
      detail: packagePublished ? `策略包 v${state.published?.version} 已就绪` : '等待策略包发布',
    },
  ]

  const next = !hasConversation
    ? { view: '对话' as const, eyebrow: '从这里开始', title: '把业务问题说清楚', detail: '先通过对话补齐目标、受众和边界，再形成可冻结 Brief。', action: '开始策略对话' }
    : !briefConfirmed
      ? { view: 'Brief' as const, eyebrow: '需要你的判断', title: '确认策略输入', detail: '核对目标、人群、渠道与限制条件，冻结后再进入策略生成。', action: '检查并确认 Brief' }
      : !strategyReady || ['draft', 'returned', 'failed'].includes(state.draft?.status ?? '')
        ? { view: '策略' as const, eyebrow: '下一步', title: strategyReady ? '完善当前策略 Revision' : '生成第一版策略', detail: '聚焦核心主张、渠道分工、创意方向和可验证指标。', action: strategyReady ? '继续完善策略' : '进入策略生成' }
        : !reviewApproved
          ? { view: '评审' as const, eyebrow: '等待决策', title: '完成策略评审', detail: '先看核心判断与风险，再决定批准发布或退回修订。', action: '进入策略评审' }
          : { view: '创意任务策略' as const, eyebrow: '策略已就绪', title: '把策略变成可执行创意', detail: '选择业务 Route，补齐任务级约束，并一键进入图文或品牌广告生产。', action: '开始创意交接' }

  const evidenceCount = document?.evidence_refs?.length ?? state.brief?.document.reference_ids?.length ?? 0
  const evidenceSummary = state.brief?.document.product?.evidence?.[0]
    || (evidenceCount > 0 ? '已绑定来源文档，可回到研究页核对原文和采用状态。' : '当前没有已确认的产品事实或来源，创意不能把推测写成卖点。')
  const primaryChannel = document?.channel_strategy?.[0]
  const progress = Math.round((stages.filter(stage => stage.complete).length / (stages.length - 1)) * 100)
  const qualityReport = state.metadata?.quality_report
  const usesDecisionQualityGate = state.metadata?.prompt_version === 'strategy.generate.v4'
  const p0Metrics = state.p0Metrics
  const requirementRate = p0Metrics && p0Metrics.funnel.conversations_engaged > 0
    ? Math.round(p0Metrics.funnel.requirements_confirmed / p0Metrics.funnel.conversations_engaged * 100)
    : null

  return <section className="kanon-strategy-overview">
    <div className="kanon-command-hero">
      <div className="kanon-command-copy">
        <span className="kanon-command-eyebrow">{next.eyebrow} · {String(Math.min(stages.findIndex(stage => !stage.complete) + 1 || stages.length, stages.length)).padStart(2, '0')}</span>
        <h2>{next.title}</h2>
        <p>{next.detail}</p>
        <button className="primary-button kanon-command-cta" onClick={() => onNavigate(next.view)}>
          <span>{next.action}</span><i><ChevronRight size={15} strokeWidth={1.6}/></i>
        </button>
      </div>
      <div className="kanon-command-progress" aria-label={`策略完成度 ${progress}%`}>
        <span>STRATEGY<br/>READINESS</span>
        <strong>{progress}<small>%</small></strong>
        <div><i style={{ width: `${progress}%` }}/></div>
        <small>{packagePublished ? '策略包已发布，可以交接创意' : '每一步均使用真实持久化状态'}</small>
        <footer className={`kanon-command-quality${usesDecisionQualityGate ? '' : ' legacy'}`}>
          <span>{usesDecisionQualityGate ? 'V4 决策质量门' : qualityReport ? '旧版基础校验' : '等待质量校验'}</span>
          <b>{qualityReport ? `${qualityReport.score}/100` : '—'}</b>
        </footer>
      </div>
    </div>

    <nav className="kanon-journey" aria-label="策略到创意决策旅程">
      {stages.map((stage, index) => <button
        className={`${stage.complete ? 'complete' : ''}${stage.view === next.view ? ' current' : ''}`}
        key={stage.label}
        onClick={() => onNavigate(stage.view)}
        type="button"
      >
        <span>{stage.complete ? <Check size={14} strokeWidth={1.8}/> : String(index + 1).padStart(2, '0')}</span>
        <b>{stage.label}</b>
        <small>{stage.detail}</small>
      </button>)}
    </nav>

    {p0Metrics ? <section className="kanon-p0-evidence" aria-label="P0 业务成效证据">
      <header>
        <div><span className="section-label">P0 RELEASE EVIDENCE</span><h3>观察到的业务漏斗</h3></div>
        <small>近 {p0Metrics.window.days} 天 · 仅代表行为记录，不代表因果提升</small>
      </header>
      <div>
        <article>
          <span>需求冻结率</span>
          <strong>{requirementRate === null ? '—' : `${requirementRate}%`}</strong>
          <small>{p0Metrics.funnel.requirements_confirmed} / {p0Metrics.funnel.conversations_engaged} 个有效对话</small>
        </article>
        <article>
          <span>需求确认 P50</span>
          <strong>{formatMetricDuration(p0Metrics.timings.median_seconds_to_requirement)}</strong>
          <small>{p0Metrics.timings.requirement_samples} 个样本 · 平均 {p0Metrics.timings.average_user_turns_to_requirement ?? '—'} 轮输入</small>
        </article>
        <article>
          <span>进入创意的路径</span>
          <strong>{p0Metrics.paths.quick_intakes}<i> quick</i> / {p0Metrics.paths.full_intakes}<i> full</i></strong>
          <small>{p0Metrics.funnel.creative_tasks_created} 个 Creative Task</small>
        </article>
        <article>
          <span>高级能力实用量</span>
          <strong>{p0Metrics.turns.deep_turns}<i> deep</i> / {p0Metrics.turns.web_search_turns}<i> web</i></strong>
          <small>{p0Metrics.turns.failed_agent_turns} 次对话 Agent 失败</small>
        </article>
        <article>
          <span>明确有用反馈</span>
          <strong>{p0Metrics.feedback.useful_rate === null ? '—' : `${Math.round(p0Metrics.feedback.useful_rate * 100)}%`}</strong>
          <small>{p0Metrics.feedback.responses} 份人工反馈，不能用模型评分替代</small>
        </article>
      </div>
    </section> : null}

    <div className="kanon-decision-grid">
      <article className="kanon-decision-card primary">
        <span className="section-label">核心判断</span>
        <h3>{document?.proposition || document?.executive_summary || '策略生成后，这里会沉淀唯一核心判断。'}</h3>
        <p>{document?.executive_summary || '把复杂输入收敛为团队可以共同执行的一句话。'}</p>
        <button className="text-button" onClick={() => onNavigate('策略')} type="button">查看完整策略 <ChevronRight size={13}/></button>
      </article>
      <article className="kanon-decision-card audience">
        <span className="section-label">为谁而做</span>
        <h3>{document?.audience.primary || state.brief?.document.audience.primary || '待明确核心人群'}</h3>
        <ul>{document?.audience.insights?.slice(0, 3).map(insight => <li key={insight}>{insight}</li>) ?? <li>完成策略后显示关键人群洞察</li>}</ul>
      </article>
      <article className="kanon-decision-card channel">
        <span className="section-label">渠道与内容切口</span>
        <h3>{primaryChannel ? `${primaryChannel.platform} · ${primaryChannel.role}` : '待明确首要渠道角色'}</h3>
        <p>{primaryChannel?.formats?.length ? primaryChannel.formats.join(' / ') : '策略会明确内容形式、渠道分工与交付目的。'}</p>
        <small>品牌广告还是效果广告，会在“创意任务策略”中作为创作路线明确确认。</small>
      </article>
      <article className="kanon-decision-card evidence">
        <span className="section-label">用户为什么相信</span>
        <div className="kanon-evidence-number"><strong>{evidenceCount}</strong><small>条已绑定证据</small></div>
        <p>{evidenceSummary}</p>
        <small>可信依据是可追溯的产品事实、客户材料或研究来源，不是策略自己写出的判断。</small>
        <button className="text-button" onClick={() => onNavigate('研究')} type="button">查看证据 <ChevronRight size={13}/></button>
      </article>
      <article className="kanon-decision-card ideas">
        <span className="section-label">可进入创意的方向</span>
        {document?.creative_recommendations?.length
          ? <ol>{document.creative_recommendations.slice(0, 3).map((idea, index) => <li key={idea}><span>{String(index + 1).padStart(2, '0')}</span><p>{idea}</p></li>)}</ol>
          : <p>策略确认后，将在这里展示最值得进入创意生产的三个方向。</p>}
      </article>
    </div>

    <div className="kanon-strategy-note" role="status">
      <ShieldCheck size={18}/>
      <div><b>{packagePublished ? `策略包 v${state.published?.version} 已冻结并可追溯` : state.probe?.ready ? `真实模型已验证：${state.probe.model_version}` : '真实模型由服务端路由管理'}</b><p>{packagePublished ? '后续创意只继承已发布版本；任何修改都会创建新 Revision，不会悄悄覆盖当前交付。' : state.probe?.ready ? `结构化输出通过，耗时 ${state.probe.latency_ms} ms；路由与用量均已记录。` : '在“策略”步骤运行真实模型探针，可验证当前路由、结构化输出与凭据状态。'}</p></div>
    </div>
  </section>
}

function formatMetricDuration(seconds: number | null) {
  if (seconds === null) return '—'
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  return `${Math.round(seconds / 360) / 10}h`
}

const briefFields: Array<{
  path: string
  label: string
  value: (brief: BriefDraft) => string
  parse?: (value: string) => unknown
  multiline?: boolean
}> = [
  { path: 'brand.name', label: '品牌', value: brief => brief.document.brand?.name ?? '' },
  { path: 'product.name', label: '产品', value: brief => brief.document.product?.name ?? '' },
  { path: 'industry', label: '行业', value: brief => brief.document.industry ?? '' },
  { path: 'region', label: '地区', value: brief => brief.document.region ?? '' },
  { path: 'language', label: '语言', value: brief => brief.document.language ?? '' },
  { path: 'campaign.objective', label: '推广目标', value: brief => brief.document.campaign.objective, multiline: true },
  { path: 'audience.primary', label: '核心受众', value: brief => brief.document.audience.primary, multiline: true },
  { path: 'proposition', label: '核心主张', value: brief => brief.document.proposition, multiline: true },
  { path: 'channels', label: '渠道', value: brief => brief.document.channels.join('、'), parse: splitValues },
  { path: 'budget.total', label: '预算', value: brief => brief.document.budget.total },
  { path: 'schedule.window', label: '周期', value: brief => brief.document.schedule.window },
  { path: 'measurement.primary_kpi', label: '核心 KPI', value: brief => brief.document.measurement.primary_kpi },
  { path: 'constraints', label: '约束条件', value: brief => brief.document.constraints.join('\n'), parse: splitValues, multiline: true },
]

function BriefPane({ brief, busy, onConfirm, onConfirmFields, onField }: {
  brief: BriefDraft | null
  busy: string
  onConfirm: () => Promise<boolean>
  onConfirmFields: (operations: Array<{ fieldPath: string; value: unknown }>) => Promise<boolean>
  onField: (path: string, value: unknown) => Promise<boolean>
}) {
  if (!brief) return <UnavailablePane title="Brief 尚未创建" detail="请先进入对话并发送第一条需求信息。"/>
  const frozen = brief.status === 'confirmed'
  const populatedCount = briefFields.filter(field => field.value(brief).trim()).length
  const confirmedCount = briefFields.filter(field => field.value(brief).trim() && brief.field_states[field.path]?.confirmation === 'confirmed').length
  const optionalCreativeContext = briefFields.filter(field => ['brand.name', 'industry', 'region', 'language'].includes(field.path) && !field.value(brief).trim())
  const unconfirmedFields = briefFields.flatMap(field => {
    const value = field.value(brief)
    if (!value.trim() || brief.field_states[field.path]?.confirmation === 'confirmed') return []
    return [{ fieldPath: field.path, value: field.parse ? field.parse(value) : value }]
  })
  return <section className="kanon-brief-pane">
    <div className="kanon-strategy-heading">
      <div><span className="section-label">BRIEF DRAFT v{brief.version}</span><h2>确认策略输入</h2><p>字段修改使用服务端版本校验，确认后冻结为不可变 BriefVersion。</p></div>
      <span className={`source-chip ${brief.completeness.ready ? '' : 'alert'}`}>{frozen ? '已冻结' : brief.completeness.ready ? '可以确认' : `${brief.completeness.blockers.length} 个阻断项`}</span>
    </div>
    <div className="kanon-brief-health" role="status">
      <div><b>{populatedCount}<small> / {briefFields.length}</small></b><span>已填写</span></div>
      <div><b>{confirmedCount}</b><span>已确认</span></div>
      <p>{optionalCreativeContext.length
        ? `${optionalCreativeContext.map(field => field.label).join('、')}未提供；这些信息会在需要时于创作前补充，不再阻断当前交接。`
        : '品牌、市场与语言上下文完整，可直接用于创意交接。'}</p>
    </div>
    <div className="kanon-field-grid">
      {briefFields.map(field => <EditableField
        busy={busy === `brief:${field.path}`}
        disabled={frozen || Boolean(busy)}
        frozen={frozen}
        key={field.path}
        label={field.label}
        multiline={field.multiline}
        onSave={value => onField(field.path, field.parse ? field.parse(value) : value)}
        state={brief.field_states[field.path]}
        value={field.value(brief)}
      />)}
    </div>
    <div className="kanon-brief-footer">
      <div>
        {brief.completeness.blockers.map(blocker => <span key={`${blocker.field}-${blocker.reason}`}><AlertCircle size={13}/>{fieldLabel(blocker.field)}：{blocker.reason}</span>)}
        {!brief.completeness.blockers.length ? <span><CircleCheck size={13}/>必填信息与确认状态满足冻结条件</span> : null}
      </div>
      <div className="kanon-brief-actions">
        {!frozen && unconfirmedFields.length ? <button className="secondary-button" disabled={Boolean(busy)} onClick={() => void onConfirmFields(unconfirmedFields)}>
          <Check size={15}/>{busy === 'confirm-brief-fields' ? '确认中…' : `确认全部已填写字段（${unconfirmedFields.length}）`}
        </button> : null}
        <button className="primary-button" disabled={frozen || !brief.completeness.ready || Boolean(busy)} onClick={() => void onConfirm()}>
          <BadgeCheck size={16}/>{busy === 'confirm-brief' ? '确认中…' : frozen ? 'Brief 已冻结' : '确认并冻结 Brief'}
        </button>
      </div>
    </div>
  </section>
}

function EditableField({ busy, disabled, frozen, label, multiline, onSave, state, value }: {
  busy: boolean
  disabled: boolean
  frozen: boolean
  label: string
  multiline?: boolean
  onSave: (value: string) => Promise<boolean>
  state?: BriefDraft['field_states'][string]
  value: string
}) {
  const [draftValue, setDraftValue] = useState(value)
  useEffect(() => setDraftValue(value), [value])
  const changed = draftValue.trim() !== value.trim()
  const needsConfirmation = Boolean(value.trim()) && state?.confirmation !== 'confirmed'
  const empty = !draftValue.trim()
  return <label className={`kanon-field ${multiline ? 'wide' : ''}${empty ? ' empty' : ''}`}>
    <span>{label}<small>{state?.confirmation === 'confirmed' ? '已确认' : state ? `${state.confidence} 置信度` : '待补充'}</small></span>
    {frozen
      ? <output>{empty ? '未提供' : draftValue}</output>
      : multiline
      ? <textarea disabled={disabled} rows={3} value={draftValue} onChange={event => setDraftValue(event.target.value)}/>
      : <input disabled={disabled} value={draftValue} onChange={event => setDraftValue(event.target.value)}/>}
    {(changed || needsConfirmation) && !disabled ? <button disabled={busy} type="button" onClick={() => void onSave(draftValue)}>{busy ? '处理中…' : changed ? '保存并确认' : '确认此字段'}</button> : null}
  </label>
}

function StrategyPane({ briefReady, busy, draft, onGenerate, onPatch, onProbe, onRetry, onRevise, onSubmit, pending, probe, readiness }: {
  briefReady: boolean
  busy: string
  draft: StrategyDraft | null
  onGenerate: () => Promise<boolean>
  onPatch: (section: string, value: unknown) => Promise<boolean>
  onProbe: () => Promise<boolean>
  onRetry: () => Promise<boolean>
  onRevise: (instruction: string) => Promise<boolean>
  onSubmit: () => Promise<boolean>
  pending: boolean
  probe: WorkspaceState['probe']
  readiness: WorkspaceState['readiness']
}) {
  const [section, setSection] = useState('objective')
  const [sectionValue, setSectionValue] = useState<unknown>('')
  const [instruction, setInstruction] = useState('')
  const document = draft?.revision?.document
  useEffect(() => {
    if (!document) {
      setSectionValue('')
      return
    }
    const value = (document as unknown as Record<string, unknown>)[section]
    setSectionValue(structuredClone(value))
  }, [document, section])

  if (!draft) {
    return <section className="kanon-strategy-generate">
      <Sparkles size={28}/>
      <h2>生成第一版策略</h2>
      <p>{briefReady ? '已确认 Brief 将作为不可变输入，生成结果会保存为 Strategy revision。' : '请先完成并确认 Brief。'}</p>
      <div className="kanon-generation-mode">
        <span>生成模式</span>
        <b>{probe?.ready ? probe.model_version : readiness?.generation_mode ?? '不可用'}</b>
        <small>{probe?.ready ? `真实探针通过 · ${probe.latency_ms} ms · ${probe.api_mode ?? '默认 API'}` : readiness?.reason_code ?? '尚未执行真实模型探针'}</small>
        {readiness?.generation_mode === 'provider' ? <button className="text-button" disabled={Boolean(busy)} onClick={() => void onProbe()}>{busy === 'generation-probe' ? '正在验证…' : '验证真实模型'}</button> : null}
      </div>
      <button className="primary-button" disabled={!briefReady || Boolean(busy)} onClick={() => void onGenerate()}><Sparkles size={16}/>{busy === 'generate-strategy' ? '正在创建…' : '生成第一版策略'}</button>
    </section>
  }

  if (draft.status === 'generating' || pending) {
    return <div className="kanon-strategy-state" role="status"><LoaderCircle className="spin" size={24}/><h2>策略生成中</h2><p>Agent 完成后会自动读取服务端 Revision。</p></div>
  }

  if (!document) {
    if (draft.status === 'failed') {
      return <section className="kanon-strategy-generate">
        <AlertCircle size={28}/>
        <h2>策略生成未完成</h2>
        <p>失败 Draft 和 AgentTask 记录已保留，可以基于同一份已冻结 Brief 重新生成。</p>
        <button className="primary-button" disabled={Boolean(busy)} onClick={() => void onRetry()}>
          <RefreshCw size={16}/>{busy === 'retry-strategy' ? '正在重新生成…' : '重新生成策略'}
        </button>
      </section>
    }
    return <UnavailablePane title="策略没有可用 Revision" detail={`当前状态：${statusLabel(draft.status)}。请重新加载或检查 AgentTask。`}/>
  }

  const sections = Object.keys(document).filter(key => !['contract_version', 'compliance', 'lineage'].includes(key))
  const original = (document as unknown as Record<string, unknown>)[section]
  const changed = JSON.stringify(sectionValue) !== JSON.stringify(original)
  // Editing an approved draft creates a new revision and leaves the published
  // StrategyPackage immutable. The backend already enforces that boundary.
  const canEdit = draft.status === 'draft' || draft.status === 'returned' || draft.status === 'approved'

  return <section className="kanon-strategy-editor-pane">
    <div className="kanon-strategy-heading kanon-strategy-document-heading">
      <div>
        <span className="section-label">STRATEGY REVISION {draft.current_revision}</span>
        <h2>{document.executive_summary || document.objective}</h2>
        <p>{document.proposition}</p>
      </div>
      <div className="kanon-strategy-document-meta">
        <span className="source-chip">{statusLabel(draft.status)}</span>
        <small title={draft.revision?.content_hash}>{draft.revision?.content_hash.slice(0, 18)}…</small>
      </div>
    </div>
    <div className="kanon-strategy-section-editor">
      <nav aria-label="策略区块">{sections.map((value, index) => <button className={value === section ? 'active' : ''} key={value} onClick={() => setSection(value)}>
        <span>{String(index + 1).padStart(2, '0')}</span>{strategySectionLabel(value)}
      </button>)}</nav>
      <div>
        <div className="surface-toolbar"><div><h3>{strategySectionLabel(section)}</h3><small>编辑当前区块</small></div><small>保存将创建 Revision {draft.current_revision + 1}</small></div>
        <StructuredStrategyEditor disabled={!canEdit || Boolean(busy)} onChange={setSectionValue} section={section} value={sectionValue}/>
        <button className="secondary-button" disabled={!canEdit || Boolean(busy) || !changed} onClick={() => void onPatch(section, sectionValue)}>
          {busy === `strategy:${section}` ? '保存中…' : '保存为新 Revision'}
        </button>
      </div>
    </div>
    <div className="kanon-revise-box">
      <label htmlFor="strategy-revise">用自然语言修订</label>
      <textarea id="strategy-revise" disabled={!canEdit || Boolean(busy)} rows={2} value={instruction} onChange={event => setInstruction(event.target.value)} placeholder="例如：把小红书的内容节奏调整为首周种草、第二周测评扩散。"/>
      <button className="secondary-button" disabled={!canEdit || Boolean(busy) || !instruction.trim()} onClick={() => { void onRevise(instruction).then(ok => { if (ok) setInstruction('') }) }}><RotateCcw size={14}/>{busy === 'revise-strategy' ? '修订中…' : '生成修订'}</button>
    </div>
    <div className="kanon-brief-footer">
      <span>候选内容保存后才可提交评审；批准会绑定 Revision 与内容哈希。</span>
      <button className="primary-button" disabled={!canEdit || Boolean(busy)} onClick={() => void onSubmit()}><BadgeCheck size={16}/>{busy === 'submit-review' ? '提交中…' : '提交评审'}</button>
    </div>
  </section>
}

function StructuredStrategyEditor({ disabled, onChange, section, value }: {
  disabled: boolean
  onChange: (value: unknown) => void
  section: string
  value: unknown
}) {
  if (typeof value === 'string') {
    const singleLine = ['平台', '核心 KPI', '实验变量', '衡量指标'].includes(section) && value.length < 54
    return <label className="kanon-structured-field wide"><span>{strategySectionLabel(section)}</span>{singleLine
      ? <input disabled={disabled} value={value} onChange={event => onChange(event.target.value)}/>
      : <textarea disabled={disabled} rows={Math.max(3, Math.min(6, Math.ceil(value.length / 70) + 2))} value={value} onChange={event => onChange(event.target.value)}/>}</label>
  }
  if (typeof value === 'number') return <label className="kanon-structured-field"><span>{strategySectionLabel(section)}</span><input disabled={disabled} type="number" value={value} onChange={event => onChange(Number(event.target.value))}/></label>
  if (typeof value === 'boolean') return <label className="kanon-check"><input checked={value} disabled={disabled} type="checkbox" onChange={event => onChange(event.target.checked)}/><span>{strategySectionLabel(section)}</span></label>
  if (Array.isArray(value)) {
    const objectArray = ['channel_strategy', 'experiment_matrix', 'platform_plans'].includes(section) || value.some(item => Boolean(item) && typeof item === 'object')
    if (!objectArray && value.every(item => typeof item === 'string')) {
      return <label className="kanon-structured-field wide"><span>{strategySectionLabel(section)}<small>每行一项</small></span><textarea disabled={disabled} rows={Math.max(5, Math.min(10, value.length + 2))} value={value.join('\n')} onChange={event => onChange(event.target.value.split('\n').map(item => item.trim()).filter(Boolean))}/></label>
    }
    return <div className="kanon-structured-list">
      {value.map((item, index) => <article key={`${section}-${index}`}><div className="surface-toolbar"><h4>{strategySectionLabel(section)} {index + 1}</h4><button aria-label={`删除${strategySectionLabel(section)} ${index + 1}`} className="text-button danger" disabled={disabled} onClick={() => onChange(value.filter((_, itemIndex) => itemIndex !== index))} type="button">删除</button></div><StructuredObjectFields disabled={disabled} onChange={next => onChange(value.map((current, itemIndex) => itemIndex === index ? next : current))} section={section} value={item}/></article>)}
      {!value.length ? <div className="panel-empty">当前没有条目，可按需新增。</div> : null}
      <button className="secondary-button" disabled={disabled} onClick={() => onChange([...value, strategyArrayTemplate(section)])} type="button"><Plus size={14}/>新增{strategySectionLabel(section)}条目</button>
    </div>
  }
  if (value && typeof value === 'object') return <StructuredObjectFields disabled={disabled} onChange={onChange} section={section} value={value}/>
  return <label className="kanon-structured-field"><span>{strategySectionLabel(section)}</span><input disabled={disabled} value="" onChange={event => onChange(event.target.value)}/></label>
}

function StructuredObjectFields({ disabled, onChange, section, value }: {
  disabled: boolean
  onChange: (value: unknown) => void
  section: string
  value: unknown
}) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return <StructuredStrategyEditor disabled={disabled} onChange={onChange} section={section} value={value}/>
  const record = value as Record<string, unknown>
  return <div className="kanon-structured-grid">{Object.entries(record).map(([key, fieldValue]) => <div className={typeof fieldValue === 'string' ? '' : 'wide'} key={key}><StructuredStrategyEditor disabled={disabled} onChange={next => onChange({ ...record, [key]: next })} section={strategyFieldLabel(key)} value={fieldValue}/></div>)}</div>
}

function strategyArrayTemplate(section: string): unknown {
  if (section === 'channel_strategy') return { platform: '', role: '', formats: [] }
  if (section === 'experiment_matrix') return { hypothesis: '', variable: '', metric: '' }
  if (section === 'platform_plans') return { platform: '', role: '', audience_angle: '', content_pillars: [], formats: [], conversion_path: '', cadence: '', primary_kpi: '', creative_ideas: [], constraints: [] }
  return ''
}

function ReviewPane({ busy, comments, deepReview, deepReviewError, draft, onAddComment, onApprove, onDeepReview, onOpenCreative, onReturn, review, revisions }: {
  busy: string
  comments: WorkspaceState['comments']
  deepReview: WorkspaceState['deepReview']
  deepReviewError: string
  draft: StrategyDraft | null
  onAddComment: (body: string) => Promise<boolean>
  onApprove: () => Promise<boolean>
  onDeepReview: () => Promise<boolean>
  onOpenCreative: () => void
  onReturn: (reason: string) => Promise<boolean>
  review: Review | null
  revisions: DraftRevision[]
}) {
  const [comment, setComment] = useState('')
  const [reason, setReason] = useState('')
  const candidate = revisions.find(item => item.revision === review?.candidate_revision) ?? draft?.revision
  const previous = revisions.find(item => item.revision === (review?.candidate_revision ?? 1) - 1)
  const diffs = useMemo(() => strategyDiff(previous, candidate), [candidate, previous])
  const document = candidate?.document

  if (!review) return <UnavailablePane title="尚未提交评审" detail="在“策略”页签完成候选 Revision 后提交评审。"/>

  return <section className="kanon-review-pane">
    <div className="kanon-strategy-heading">
      <div><span className="section-label">REVIEW {review.id.slice(0, 12)}</span><h2>Revision {review.candidate_revision} 评审</h2><p>决策绑定候选内容哈希，过期版本不能批准。</p></div>
      <span className={`source-chip ${review.status === 'returned' ? 'alert' : ''}`}>{statusLabel(review.status)}</span>
    </div>
    {document ? <StrategyDecisionBrief document={document} review={review}/> : null}
    <section className="kanon-deep-review">
      <div className="surface-toolbar"><div><h3>第二视角检查</h3><small>模型辅助检查证据、渠道协同、衡量方式与执行风险</small></div>{deepReview?.status === 'succeeded' ? <span className="source-chip">已完成 · {deepReview.model_version}</span> : null}</div>
      {deepReviewError && !deepReview ? <DeepReviewFailure busy={busy} message={deepReviewError} onRetry={onDeepReview} reviewOpen={review.status === 'open'}/> : !deepReview ? <div className="kanon-deep-review-empty"><p>这一步是可选辅助，不会阻断人工确认。运行后会把问题整理为可执行建议。</p><button className="secondary-button" disabled={Boolean(busy) || review.status !== 'open'} onClick={() => void onDeepReview()}><Sparkles size={14}/>{busy === 'deep-review' ? '正在启动…' : '运行第二视角检查'}</button></div> : deepReview.status === 'pending' ? <div className="kanon-deep-review-pending" role="status"><LoaderCircle className="spin" size={16}/><span><b>第二视角正在后台检查</b><small>可以离开页面；完成后会自动恢复结果，不影响人工评审。</small></span></div> : deepReview.status === 'failed' ? <DeepReviewFailure busy={busy} message={deepReviewError || '本次模型任务未完成，请稍后重试。'} onRetry={onDeepReview} reviewOpen={review.status === 'open'}/> : <><p className="kanon-deep-review-summary">{deepReview.summary}</p><div className="kanon-deep-findings">{deepReview.findings.map((finding, index) => <article className={finding.severity} key={`${finding.section}-${index}`}><header><span>{finding.severity === 'blocker' ? '阻断风险' : finding.severity === 'warning' ? '需要关注' : '优化机会'}</span><small>{strategySectionLabel(finding.section)}</small></header><h4>{finding.title}</h4><p>{finding.detail}</p><div><b>建议</b>{finding.recommendation}</div></article>)}</div><details className="kanon-technical-details"><summary>查看运行信息</summary><code>{deepReview.api_mode} · background={String(deepReview.background)} · {deepReview.latency_ms ?? 0} ms{deepReview.usage ? ` · ${deepReview.usage.total_tokens} tokens` : ''}</code></details></>}
    </section>
    <div className="kanon-review-content">
      <div className="kanon-review-diffs">
        {!diffs.length && candidate ? <details className="kanon-review-full kanon-review-details"><summary>查看完整候选策略</summary><ReviewValue value={candidate.document}/></details> : diffs.map(diff => <article key={diff.section}>
          <h3>{strategySectionLabel(diff.section)}</h3>
          <div><section><small>之前</small><ReviewValue emptyLabel="暂无上一版本" value={diff.before}/></section><section><small>候选</small><ReviewValue value={diff.after}/></section></div>
        </article>)}
      </div>
      <aside className="kanon-review-comments">
        <h3>评论与决策</h3>
        {review.assignments?.map(assignment => <article key={assignment.id}><b>{assignment.review_mode === 'self_confirmation' ? '个人确认' : '指定审批人'}</b><p>{assignment.reviewer_user_id}</p><small>{assignment.status}</small></article>)}
        {comments.map(item => <article key={item.id}><b>{item.author_id}</b><p>{item.body}</p><small>{formatTime(item.created_at)}</small></article>)}
        {!comments.length ? <p className="panel-empty">尚无评论。</p> : null}
        {review.status === 'open' ? <>
          <textarea disabled={Boolean(busy)} rows={3} value={comment} onChange={event => setComment(event.target.value)} placeholder="留下可执行评论"/>
          <button className="secondary-button full" disabled={Boolean(busy) || !comment.trim()} onClick={() => { void onAddComment(comment).then(ok => { if (ok) setComment('') }) }}>添加评论</button>
          <button className="primary-button full" disabled={Boolean(busy)} onClick={() => void onApprove()}><BadgeCheck size={15}/>{busy === 'approve-review' ? '处理中…' : review.review_mode === 'self_confirmation' ? '确认并发布策略包' : '批准并发布策略包'}</button>
          <textarea disabled={Boolean(busy)} rows={2} value={reason} onChange={event => setReason(event.target.value)} placeholder="退回原因（必填）"/>
          <button className="secondary-button full" disabled={Boolean(busy) || !reason.trim()} onClick={() => void onReturn(reason)}>退回修改</button>
        </> : review.status === 'approved' ? <div className="kanon-review-approved">
          <CircleCheck size={18}/>
          <span><b>策略已确认并发布</b><small>下一步将冻结的判断转换成可执行创意任务。</small></span>
          <button className="primary-button full" onClick={onOpenCreative}>开始创意交接 <ChevronRight size={14}/></button>
        </div> : <div className="kanon-review-decision"><AlertCircle size={17}/><span>{review.decision_reason || '该评审已结束'}</span></div>}
      </aside>
    </div>
    <details className="kanon-technical-details kanon-review-proof"><summary>版本与完整性校验</summary><code>{review.candidate_content_hash}</code></details>
  </section>
}

function StrategyDecisionBrief({ document, review }: { document: StrategyDocument; review: Review }) {
  const evidenceCount = document.evidence_refs?.length ?? 0
  const riskCount = document.assumptions_and_gaps.length + (document.compliance?.issues.length ?? 0)
  return <section className="kanon-review-brief" aria-label="本次评审的核心决策">
    <div>
      <span className="section-label">一句话决策</span>
      <h3>{document.proposition}</h3>
      <p>{document.executive_summary || '围绕这一核心主张检查人群、渠道、证据和执行边界。'}</p>
    </div>
    <dl>
      <div><dt>核心人群</dt><dd>{document.audience.primary}</dd></div>
      <div><dt>渠道方案</dt><dd>{document.channel_strategy.length} 个</dd></div>
      <div><dt>证据引用</dt><dd>{evidenceCount} 条</dd></div>
      <div><dt>待关注</dt><dd className={riskCount ? 'warn' : ''}>{riskCount} 项</dd></div>
    </dl>
    <small>{review.review_mode === 'self_confirmation' ? '你的确认将发布不可变策略包' : '批准后将发布不可变策略包'}</small>
  </section>
}

function DeepReviewFailure({ busy, message, onRetry, reviewOpen }: {
  busy: string
  message: string
  onRetry: () => Promise<boolean>
  reviewOpen: boolean
}) {
  return <div className="kanon-deep-review-failed" role="status"><AlertCircle size={17}/><span><b>第二视角暂时没有完成</b><small>策略和已有评审内容均已保留，你仍可人工确认，或稍后重试。</small><details className="kanon-technical-details"><summary>查看技术原因</summary><code>{message}</code></details></span><button className="secondary-button" disabled={Boolean(busy) || !reviewOpen} onClick={() => void onRetry()}><RefreshCw size={14}/>重新运行</button></div>
}

function ResearchPane({ brief, busy, documents, onAdoption, onResearch, onUpload, researchRun }: {
  brief: BriefDraft | null
  busy: string
  documents: WorkspaceState['documents']
  onResearch: (
    category: 'general' | 'audience' | 'competitor' | 'industry',
    query: string,
    documentIds: string[],
  ) => Promise<boolean>
  onAdoption: (artifactId: string, adopted: boolean) => Promise<boolean>
  onUpload: (file: File) => Promise<boolean>
  researchRun: WorkspaceState['researchRun']
}) {
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState<'general' | 'audience' | 'competitor' | 'industry'>('general')
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([])
  const frozen = brief?.status === 'confirmed'
  const referenceIds = brief?.document.reference_ids ?? []
  useEffect(() => {
    const readyIDs = new Set(documents.filter(document => document.status === 'ready').map(document => document.id))
    setSelectedDocumentIds(current => current.filter(id => readyIDs.has(id)))
  }, [documents])
  return <section className="kanon-research-pane">
    <div className="kanon-strategy-heading">
      <div><span className="section-label">RESEARCH & EVIDENCE</span><h2>把外部与内部资料变成可引用证据</h2><p>研究结果由后端落为 ResearchArtifact，再写入 Brief reference IDs。</p></div>
      <span className="source-chip">{documents.length} 份项目资料</span>
    </div>
    <div className="kanon-research-grid">
      <section>
        <div className="surface-toolbar"><h3>品牌与项目资料</h3><label className="secondary-button" htmlFor="kanon-knowledge-file"><Upload size={14}/>上传资料</label></div>
        <input id="kanon-knowledge-file" type="file" accept=".md,.docx,.pdf" disabled={Boolean(busy) || frozen} onChange={event => { const file = event.target.files?.[0]; if (file) void onUpload(file) }}/>
        {documents.map(document => {
          const ready = document.status === 'ready'
          const selected = selectedDocumentIds.includes(document.id)
          return <article className={selected ? 'selected' : ''} key={document.id}>
            <FileText size={17}/>
            <span>
              <b>{document.title || document.filename}</b>
              <small>{document.source_type || document.mime_type} · {formatBytes(document.size_bytes)} · {documentStatusLabel(document)}</small>
            </span>
            <label className="kanon-document-select">
              <input
                aria-label={`选择资料 ${document.title || document.filename}`}
                checked={selected}
                disabled={!ready || Boolean(busy) || frozen}
                type="checkbox"
                onChange={event => setSelectedDocumentIds(current => event.target.checked
                  ? [...current, document.id]
                  : current.filter(id => id !== document.id))}
              />
              <span>{ready ? '用于本次研究' : '解析完成后可选'}</span>
            </label>
          </article>
        })}
        {!documents.length ? <div className="panel-empty">尚未导入品牌或项目资料。</div> : null}
      </section>
      <section className="kanon-research-form">
        <div className="surface-toolbar"><h3>外部研究</h3><span>Seed 联网搜索</span></div>
        <label>研究分类<select disabled={Boolean(busy) || frozen} value={category} onChange={event => setCategory(event.target.value as typeof category)}><option value="general">综合研究</option><option value="audience">受众研究</option><option value="competitor">竞品研究</option><option value="industry">行业研究</option></select></label>
        <label>要验证的问题<textarea disabled={Boolean(busy) || frozen} rows={4} value={query} onChange={event => setQuery(event.target.value)} placeholder="例如：近半年小红书工业品牌内容的有效切入点是什么？"/></label>
        <p className="kanon-research-disclosure">开始研究会把当前问题发送给 Seed 并记录披露字段。项目文件不会自动发送；若勾选资料，仅披露本地检索命中的最多 8 个片段，并记录实际片段 ID。</p>
        <button className="primary-button" disabled={Boolean(busy) || frozen || !query.trim()} onClick={() => void onResearch(category, query, selectedDocumentIds)}><Search size={15}/>{busy === 'research' ? '研究中…' : '开始联网研究'}</button>
      </section>
    </div>
    {researchRun ? <div className="kanon-research-result">
      <div><b>Research Run {researchRun.id.slice(0, 12)}</b><span>{researchRun.model_version || researchRun.provider_code}</span><span className={`source-chip ${researchRun.status === 'failed' || researchRun.status === 'unavailable' ? 'alert' : ''}`}>{researchRun.status}</span></div>
      {researchRun.error_message ? <p>{researchRun.error_message}</p> : null}
      {researchRun.artifacts.map(artifact => {
        const adopted = referenceIds.includes(artifact.id)
        return <article key={artifact.id}>
          <BookOpen size={17}/>
          <div>
            <b>{artifact.title}</b>
            <p>{artifact.content}</p>
            <div className="kanon-research-sources">
              {artifact.sources.map(source => <a href={source.url} key={`${source.id}-${source.start_index}-${source.end_index}`} rel="noreferrer" target="_blank">
                <ExternalLink size={12}/>{source.title || source.domain}<small>{source.verification_status === 'content_verified' ? '已核验' : '模型引用'}</small>
              </a>)}
            </div>
          </div>
          <button className={adopted ? 'secondary-button' : 'primary-button'} disabled={Boolean(busy) || frozen} onClick={() => void onAdoption(artifact.id, !adopted)}>
            {adopted ? '取消采纳' : '采纳到 Brief'}
          </button>
        </article>
      })}
    </div> : null}
  </section>
}

function ChangeLogPane({ comments, review, revisions }: {
  comments: WorkspaceState['comments']
  review: Review | null
  revisions: DraftRevision[]
}) {
  const items = [
    ...revisions.map(revision => ({
      id: `revision-${revision.revision}`,
      title: `Strategy Revision ${revision.revision}`,
      detail: `${revision.changed_sections.map(strategySectionLabel).join('、')} · ${revision.content_hash.slice(0, 16)}`,
      kind: '策略修订',
    })),
    ...comments.map(comment => ({
      id: comment.id,
      title: `${comment.author_id} 添加评审评论`,
      detail: comment.body,
      kind: formatTime(comment.created_at),
    })),
  ]
  return <section className="kanon-change-log">
    <div className="kanon-strategy-heading"><div><span className="section-label">CURRENT CHAIN</span><h2>当前工作链变更记录</h2><p>只展示服务端已有 revisions 和 review comments。</p></div>{review ? <span className="source-chip">{statusLabel(review.status)}</span> : null}</div>
    {items.map(item => <article key={item.id}><span/><div><small>{item.kind}</small><b>{item.title}</b><p>{item.detail}</p></div></article>)}
    {!items.length ? <div className="panel-empty">当前工作链尚无修订或评审评论。</div> : null}
  </section>
}

function SummaryRail({ brief, briefVersion, draft, publishedVersion, review, workspaceName }: {
  brief: BriefDraft | null
  briefVersion?: number
  draft: StrategyDraft | null
  publishedVersion?: number
  review: Review | null
  workspaceName: string
}) {
  const items = [
    ['工作区', workspaceName],
    ['Brief', brief ? `${brief.status === 'confirmed' ? '已冻结' : '草稿'} v${brief.status === 'confirmed' ? briefVersion ?? 1 : brief.version}` : '未创建'],
    ['完整度', brief ? brief.status === 'confirmed' ? '已确认' : brief.completeness.ready ? '可以确认' : `${brief.completeness.blockers.length} 个阻断项` : '—'],
    ['Strategy', draft ? `Revision ${draft.current_revision} · ${statusLabel(draft.status)}` : '未生成'],
    ['Review', review ? statusLabel(review.status) : '未提交'],
    ['Package', publishedVersion ? `已发布 v${publishedVersion}` : '未发布'],
  ]
  return <aside className="kanon-summary-rail">
    <div className="surface-toolbar"><h3>对象摘要</h3><span>真实状态</span></div>
    {items.map(([label, value]) => <div className="kv" key={label}><span>{label}</span><b>{value}</b></div>)}
    <div className="kanon-summary-checks">
      <b>发布检查</b>
      {[
        ['Brief 已冻结', brief?.status === 'confirmed'],
        ['策略 Revision', Boolean(draft?.current_revision)],
        ['人工确认/批准', review?.status === 'approved'],
        ['不可变策略包', Boolean(publishedVersion)],
      ].map(([label, complete]) => <div key={String(label)}>{complete ? <CircleCheck size={14}/> : <AlertCircle size={14}/>}<span>{label}</span></div>)}
    </div>
  </aside>
}

function EvidenceRail({ documents, referenceIds, researchArtifacts }: {
  documents: WorkspaceState['documents']
  referenceIds: string[]
  researchArtifacts: NonNullable<WorkspaceState['researchRun']>['artifacts']
}) {
  const referencedDocuments = documents.filter(document => referenceIds.includes(document.id))
  const artifacts = researchArtifacts.filter(artifact => referenceIds.includes(artifact.id))
  return <aside className="kanon-evidence-rail">
    <div className="surface-toolbar"><h3>证据</h3><span>{referenceIds.length} 个引用</span></div>
    {referencedDocuments.map(document => <article key={document.id}><span className="evidence-id">DOC</span><div><b>{document.title || document.filename}</b><small>{document.source_type || '项目资料'}</small><small>{document.id}</small></div></article>)}
    {artifacts.map(artifact => <article key={artifact.id}><span className="evidence-id">R</span><div><b>{artifact.title}</b><small>{artifact.citations[0] || '研究产物'}</small><small>{artifact.content_hash.slice(0, 18)}</small></div></article>)}
    {!referenceIds.length ? <div className="panel-empty">Brief 尚未引用资料或研究产物。</div> : null}
  </aside>
}

function UnavailablePane({ detail, title }: { detail: string; title: string }) {
  return <div className="kanon-strategy-state"><AlertCircle size={24}/><h2>{title}</h2><p>{detail}</p></div>
}

function strategyDiff(base?: DraftRevision, candidate?: DraftRevision) {
  if (!candidate) return []
  const before = base?.document ?? {} as StrategyDocument
  const sections = candidate.changed_sections.includes('all')
    ? Object.keys(candidate.document).filter(key => key !== 'contract_version')
    : candidate.changed_sections
  return sections.map(section => ({
    section,
    before: (before as unknown as Record<string, unknown>)[section],
    after: (candidate.document as unknown as Record<string, unknown>)[section],
  })).filter(item => JSON.stringify(item.before) !== JSON.stringify(item.after))
}

function splitValues(value: string) {
  return value.split(/[\n,，、]/).map(item => item.trim()).filter(Boolean)
}

function ReviewValue({ emptyLabel = '—', value }: { emptyLabel?: string; value: unknown }) {
  if (value === undefined || value === null || value === '') {
    return <p className="kanon-review-empty-value">{emptyLabel}</p>
  }
  if (Array.isArray(value)) {
    if (!value.length) return <p className="kanon-review-empty-value">{emptyLabel}</p>
    return <ul className="kanon-review-value-list">{value.map((item, index) => <li key={reviewValueKey(item, index)}><ReviewValue value={item}/></li>)}</ul>
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([key]) => key !== 'contract_version')
    if (!entries.length) return <p className="kanon-review-empty-value">{emptyLabel}</p>
    return <dl className="kanon-review-value-object">{entries.map(([key, fieldValue]) => <div key={key}><dt>{strategyFieldLabel(key)}</dt><dd><ReviewValue value={fieldValue}/></dd></div>)}</dl>
  }
  if (typeof value === 'boolean') return <p className="kanon-review-value-text">{value ? '是' : '否'}</p>
  return <p className="kanon-review-value-text">{String(value)}</p>
}

function reviewValueKey(value: unknown, index: number) {
  if (typeof value === 'string' || typeof value === 'number') return `${index}-${String(value).slice(0, 32)}`
  return String(index)
}

function strategySectionLabel(value: string) {
  const labels: Record<string, string> = {
    objective: '目标',
    audience: '受众',
    proposition: '核心主张',
    channel_strategy: '渠道策略',
    creative_recommendations: '创意建议',
    constraints: '约束',
    budget_and_cadence: '预算与节奏',
    experiment_matrix: '实验矩阵',
    measurement: '衡量指标',
    assumptions_and_gaps: '假设与缺口',
    executive_summary: '执行摘要',
    cross_platform_role: '跨平台协同',
    platform_plans: '分平台方案',
    evidence_refs: '证据引用',
    compliance: '合规报告',
    lineage: '生成追溯',
  }
  return labels[value] ?? value
}

function strategyFieldLabel(value: string) {
  const labels: Record<string, string> = {
    primary: '核心人群', insights: '人群洞察', platform: '平台', role: '平台角色',
    formats: '内容形式', audience_angle: '人群切入点', content_pillars: '内容支柱',
    conversion_path: '转化路径', cadence: '内容节奏', primary_kpi: '核心 KPI',
    creative_ideas: '创意方向', hypothesis: '实验假设', variable: '实验变量',
    metric: '衡量指标', budget: '预算',
    brief_id: 'Brief ID', brief_version: 'Brief 版本', project_context_version: 'Project 上下文版本',
    skill_versions: '技能版本', passed: '是否通过', issues: '检查项', rule_id: '规则',
    severity: '级别', message: '说明', checked_at: '检查时间',
  }
  return labels[value] ?? strategySectionLabel(value)
}

function fieldLabel(value: string) {
  const field = briefFields.find(item => item.path === value)
  return field?.label ?? value
}

function statusLabel(value: string) {
  const labels: Record<string, string> = {
    active: '进行中',
    waiting_user: '等待补充',
    ready_to_confirm: '待确认',
    completed: '已完成',
    generating: '生成中',
    draft: '草稿',
    ready_for_review: '待评审',
    returned: '已退回',
    approved: '已批准',
    invalidated: '已失效',
    open: '评审中',
    failed: '失败',
    cancelled: '已取消',
  }
  return labels[value] ?? value
}

function formatTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

function documentStatusLabel(document: KnowledgeDocument) {
  if (document.status === 'ready') return `${document.chunk_count} 个片段 · 已就绪`
  if (document.status === 'parse_failed') return document.parse_error_message || '解析失败'
  if (document.status === 'parsing') return '解析中'
  return '等待解析'
}
