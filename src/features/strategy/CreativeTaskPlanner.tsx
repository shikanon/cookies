import {
  AlertCircle,
  Check,
  ChevronRight,
  CircleHelp,
  Download,
  Film,
  Image as ImageIcon,
  LoaderCircle,
  RefreshCw,
  Rocket,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { BackendApiError } from '../../backend/platform'
import type { ApiProjectMediaAsset } from '../../data/api'
import { platformClient } from '../../data/platformClient'
import { createMutationKey, strategyApi } from './api'
import { createRouteRevisionChannelStrategy, findPublishedPackageForDraft } from './creativeTaskPlanning'
import type {
  BriefVersion,
  CreativeBusinessCapability,
  CreativeBusinessProfile,
  CreativeBusinessRecommendation,
  CreativeBusinessRecommendationSnapshot,
  CreativeMediaAssessment,
  CreativeTaskPlan,
  PackageVersion,
  StrategyCreativeHandoff,
  StrategyCreativeHandoffIssue,
  StrategyDraft,
} from './types'

type Props = {
  briefVersion: BriefVersion | null
  draft: StrategyDraft | null
  onCreateRouteRevision: (channelStrategy: unknown) => Promise<boolean>
  onOpenCreative: (navId: string, view: string, contextId: string) => void
  onOpenStrategy: () => void
  projectId: string
}

export function CreativeTaskPlanner({ briefVersion, draft, onCreateRouteRevision, onOpenCreative, onOpenStrategy, projectId }: Props) {
  const [catalogHash, setCatalogHash] = useState('')
  const [profiles, setProfiles] = useState<CreativeBusinessProfile[]>([])
  const [capabilities, setCapabilities] = useState<CreativeBusinessCapability[]>([])
  const [mediaAssets, setMediaAssets] = useState<ApiProjectMediaAsset[]>([])
  const [recommendation, setRecommendation] = useState<CreativeBusinessRecommendationSnapshot | null>(null)
  const [plans, setPlans] = useState<CreativeTaskPlan[]>([])
  const [strategyPackage, setStrategyPackage] = useState<PackageVersion | null>(null)
  const [creativeHandoff, setCreativeHandoff] = useState<StrategyCreativeHandoff | null>(null)
  const [selectedRouteId, setSelectedRouteId] = useState('')
  const [selectedCode, setSelectedCode] = useState('')
  const [activePlanId, setActivePlanId] = useState('')
  const [answers, setAnswers] = useState<Record<string, unknown>>({})
  const [showSetup, setShowSetup] = useState(false)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const activePlan = plans.find(plan => plan.id === activePlanId) ?? null
  const selectedProfile = profiles.find(profile =>
    profile.business_code === (activePlan?.business_code || selectedCode),
  ) ?? activePlan?.profile ?? null
  const recommendedCodes = useMemo(
    () => new Set(recommendation?.recommended.map(item => item.business_code) ?? []),
    [recommendation],
  )
  const orderedProfiles = useMemo(() => {
    const rank = new Map(
      recommendation?.recommended.map(item => [item.business_code, item.rank ?? Number.MAX_SAFE_INTEGER]) ?? [],
    )
    return [...profiles].sort((left, right) => {
      const leftRank = rank.get(left.business_code)
      const rightRank = rank.get(right.business_code)
      if (leftRank !== undefined || rightRank !== undefined) {
        if (leftRank === undefined) return 1
        if (rightRank === undefined) return -1
        return leftRank - rightRank
      }
      return left.display_order - right.display_order
    })
  }, [profiles, recommendation])
  const selectedRecommendation = recommendation?.recommended.find(item =>
    item.business_code === selectedCode,
  ) ?? recommendation?.alternatives.find(item => item.business_code === selectedCode)
  const readyRoutes = useMemo(
    () => creativeHandoff?.routes.filter(route => route.route_readiness.status === 'ready') ?? [],
    [creativeHandoff],
  )
  const compatibleRoutes = useMemo(
    () => readyRoutes.filter(route => creativeBusinessMatchesRoute(selectedCode, route)),
    [readyRoutes, selectedCode],
  )
  const routeBlockers = Array.from(new Set([
    ...handoffHardBlockers(creativeHandoff, selectedRouteId),
    ...(creativeHandoff?.routes
      .filter(route => !selectedCode || creativeBusinessMatchesRoute(selectedCode, route))
      .flatMap(route => route.route_readiness.blockers ?? []) ?? []),
  ].map(routeIssueMessage)))
  const routeState = !strategyPackage
    ? {
        title: '当前策略尚未发布交接包',
        detail: draft?.status === 'approved'
          ? '未找到与当前 Strategy Revision 精确匹配的已发布策略包，请刷新后重试。'
          : '请先在“评审”中确认当前 Revision，发布后才能创建强绑定任务计划。',
      }
    : !creativeHandoff?.routes.length
      ? {
          title: '已发布策略包没有生成 Route',
          detail: routeBlockers[0] || '渠道策略需要明确图文内容形式、品牌或效果目的，保存新 Revision 后重新评审发布。',
        }
      : !compatibleRoutes.length
        ? {
            title: selectedProfile ? `当前策略没有可用的「${selectedProfile.display_name}」创作路线` : '当前创作路线尚未就绪',
            detail: routeBlockers[0] || '请回到策略补充对应的渠道、内容形式和品牌/效果目的，保存新 Revision 后重新发布。',
          }
        : null
  const createPlanLabel = !strategyPackage
    ? '等待策略评审发布'
    : !compatibleRoutes.length
      ? '等待可用创作路线'
      : '确认此业务并创建任务计划'
  const routeRevision = draft?.revision
    ? createRouteRevisionChannelStrategy(draft.revision.document.channel_strategy)
    : null
  const hasUnsavedAnswers = Boolean(activePlan && selectedProfile && selectedProfile.questions.some(question => {
    if (question.brief_source_path &&
      hasDisplayValue(readPath(briefVersion?.snapshot, question.brief_source_path))) return false
    const answerExists = Object.prototype.hasOwnProperty.call(answers, question.id)
    const savedAnswerExists = Object.prototype.hasOwnProperty.call(activePlan.answers, question.id)
    return (answerExists || savedAnswerExists) &&
      JSON.stringify(activePlan.answers[question.id]) !== JSON.stringify(answers[question.id])
  }))

  useEffect(() => {
    if (!selectedCode || !readyRoutes.length) return
    if (compatibleRoutes.some(route => route.route_id === selectedRouteId)) return
    setSelectedRouteId(compatibleRoutes[0]?.route_id ?? '')
  }, [compatibleRoutes, readyRoutes.length, selectedCode, selectedRouteId])

  const load = async (signal?: AbortSignal) => {
    if (!briefVersion) return
    const [catalog, nextRecommendation, planResult, nextMediaAssets, capabilityResult, packageResult] = await Promise.all([
      strategyApi.listCreativeBusinesses(projectId, signal),
      strategyApi.recommendCreativeBusinesses(projectId, briefVersion, signal),
      strategyApi.listCreativeTaskPlans(projectId, briefVersion.brief_id, signal),
      platformClient.listProjectMediaAssets(projectId),
      strategyApi.listCreativeBusinessCapabilities(projectId, signal),
      strategyApi.listStrategyPackages(projectId, signal),
    ])
    const nextPackage = findPublishedPackageForDraft(packageResult.items, briefVersion, draft)
    const nextHandoff = nextPackage
      ? await strategyApi.getStrategyCreativeHandoff(
        projectId, nextPackage.package_id, nextPackage.version, signal,
      )
      : null
    const matchingPlans = planResult.items.filter(plan => plan.brief_version === briefVersion.version)
    const nextPlan = matchingPlans.find(plan => plan.id === activePlanId) ?? matchingPlans[0] ?? null
    setCatalogHash(catalog.catalog_hash)
    setProfiles(catalog.items)
    setCapabilities(capabilityResult.items)
    setMediaAssets(nextMediaAssets)
    setRecommendation(nextRecommendation)
    setPlans(matchingPlans)
    setStrategyPackage(nextPackage)
    setCreativeHandoff(nextHandoff)
    setSelectedRouteId(current => {
      if (nextPlan?.selected_route_id && nextHandoff?.routes.some(route => route.route_id === nextPlan.selected_route_id && route.route_readiness.status === 'ready')) {
        return nextPlan.selected_route_id
      }
      if (nextHandoff?.routes.some(route => route.route_id === current && route.route_readiness.status === 'ready')) return current
      return nextHandoff?.routes.find(route => route.route_readiness.status === 'ready')?.route_id ?? ''
    })
    setActivePlanId(nextPlan?.id ?? '')
    setSelectedCode(nextPlan?.business_code ?? nextRecommendation.recommended[0]?.business_code ?? catalog.items[0]?.business_code ?? '')
    setAnswers(nextPlan?.answers ?? {})
  }

  useEffect(() => {
    if (!briefVersion) return
    const controller = new AbortController()
    setBusy('load')
    setError('')
    void load(controller.signal)
      .catch(cause => {
        if (!(cause instanceof DOMException && cause.name === 'AbortError')) {
          setError(messageOf(cause))
        }
      })
      .finally(() => setBusy(''))
    return () => controller.abort()
    // A new immutable Brief version is a new planning context.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [briefVersion?.brief_id, briefVersion?.version, draft?.current_revision, draft?.id, projectId])

  const selectPlan = (plan: CreativeTaskPlan) => {
    setActivePlanId(plan.id)
    setSelectedCode(plan.business_code)
    setAnswers(plan.answers)
    setError('')
  }

  const createPlan = async () => {
    if (!briefVersion || !selectedCode || !strategyPackage || !creativeHandoff || !selectedRouteId) return
    setBusy('create')
    setError('')
    try {
      const plan = await strategyApi.createCreativeTaskPlan(projectId, {
        contract_version: 'strategy-creative-task-plan/v2',
        strategy_package_ref: {
          package_id: strategyPackage.package_id,
          package_version: strategyPackage.version,
          package_content_hash: strategyPackage.content_hash,
          handoff_contract_version: creativeHandoff.contract_version,
          handoff_content_hash: creativeHandoff.handoff_content_hash,
        },
        selected_route_id: selectedRouteId,
        business_code: selectedCode,
        selection_source: recommendedCodes.has(selectedCode) ? 'recommended' : 'manual',
        catalog_hash: catalogHash,
      }, createMutationKey('creative-task-plan'))
      setPlans(current => [plan, ...current])
      selectPlan(plan)
    } catch (cause) {
      setError(messageOf(cause))
      if (cause instanceof BackendApiError && cause.code === 'CATALOG_CHANGED') {
        await load().catch(() => undefined)
      }
    } finally {
      setBusy('')
    }
  }

  const createRouteRevision = async () => {
    if (!routeRevision?.changed) {
      onOpenStrategy()
      return
    }
    setBusy('route-repair')
    setError('')
    try {
      if (await onCreateRouteRevision(routeRevision.value)) onOpenStrategy()
    } finally {
      setBusy('')
    }
  }

  const saveAnswers = async () => {
    if (!activePlan || !selectedProfile || !briefVersion) return
    const editableAnswers = Object.fromEntries(
      selectedProfile.questions
        .filter(question => !question.brief_source_path ||
          !hasDisplayValue(readPath(briefVersion.snapshot, question.brief_source_path)))
        .map(question => [question.id, answers[question.id]]),
    )
    const dirtyAnswers = Object.fromEntries(
      Object.entries(editableAnswers).filter(([key, value]) =>
        Object.prototype.hasOwnProperty.call(activePlan.answers, key) || value !== undefined
          ? JSON.stringify(activePlan.answers[key]) !== JSON.stringify(value)
          : false,
      ),
    )
    if (!Object.keys(dirtyAnswers).length) return
    setBusy('save')
    setError('')
    try {
      const plan = await strategyApi.patchCreativeTaskPlanAnswers(
        activePlan,
        dirtyAnswers,
        createMutationKey('creative-task-answers'),
      )
      setPlans(current => current.map(item => item.id === plan.id ? plan : item))
      setAnswers(plan.answers)
    } catch (cause) {
      setError(messageOf(cause))
      await refreshPlan(activePlan.id).catch(() => undefined)
    } finally {
      setBusy('')
    }
  }

  const refreshPlan = async (planId = activePlan?.id ?? '') => {
    if (!planId) return
    const plan = await strategyApi.getCreativeTaskPlan(planId)
    setPlans(current => current.some(item => item.id === plan.id)
      ? current.map(item => item.id === plan.id ? plan : item)
      : [plan, ...current])
    setActivePlanId(plan.id)
    setSelectedCode(plan.business_code)
    setAnswers(plan.answers)
  }

  const generate = async () => {
    if (!activePlan) return
    setBusy('generate')
    setError('')
    try {
      const result = await strategyApi.generateCreativeTaskStrategy(
        activePlan,
        createMutationKey('creative-task-generate'),
      )
      setPlans(current => current.map(item => item.id === result.plan.id ? result.plan : item))
      await pollGeneration(result.plan.id, result.agent_task.id)
    } catch (cause) {
      setError(messageOf(cause))
      await refreshPlan().catch(() => undefined)
    } finally {
      setBusy('')
    }
  }

  const pollGeneration = async (planId: string, agentTaskId: string) => {
    for (let attempt = 0; attempt < 80; attempt += 1) {
      const inspection = await strategyApi.getAgentTask(agentTaskId)
      if (inspection.task.status === 'succeeded') {
        await refreshPlan(planId)
        return
      }
      if (inspection.task.status === 'failed' || inspection.task.status === 'cancelled') {
        const problem = inspection.task.error ?? inspection.job?.error
        throw new Error(problem?.message || '创意任务策略生成失败。')
      }
      await new Promise(resolve => window.setTimeout(resolve, 1500))
    }
    throw new Error('生成仍在后台运行，请稍后刷新。')
  }

  const handoff = async (plan: CreativeTaskPlan, capability: CreativeBusinessCapability) => {
    if (!plan.current_strategy || capability.status !== 'available' ||
      !capability.destination_area || !capability.destination_view) return
    setBusy('handoff')
    setError('')
    try {
      const intake = await strategyApi.handoffCreativeTaskStrategy(
        projectId,
        plan,
        `task-strategy-handoff-${plan.id}-${plan.current_strategy.version}`,
      )
      if (intake.status !== 'ready') {
        const missing = intake.missing_fields?.map(field => {
          if (field === 'strategy_package.creative_ready') {
            return '冻结 Handoff 仍缺少市场、语言或可执行创作路线'
          }
          return field
        }) ?? []
        throw new Error(
          missing.length
            ? `创意交接尚未就绪：${missing.join('；')}`
            : '创意交接尚未就绪，请先处理冻结 Handoff 的阻断项',
        )
      }
      let destinationId = intake.id
      if (capability.can_create_task_immediately && capability.format === 'image_text') {
        const angle = plan.current_strategy.document.business_strategy.content_angle
        const task = await strategyApi.createImageTextTaskFromHandoff(
          projectId,
          intake.id,
          typeof angle === 'string' && angle.trim()
            ? angle
            : plan.current_strategy.document.core_message,
        )
        destinationId = task.id
      }
      onOpenCreative(
        capability.destination_area,
        capability.destination_view,
        destinationId,
      )
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy('')
    }
  }

  if (!briefVersion) {
    return <section className="creative-planner-empty">
      <Sparkles size={26}/>
      <h2>先确认 Brief，再选择创意业务</h2>
      <p>业务推荐只读取不可变 BriefVersion；未确认的草稿不会被拿来生成任务策略。</p>
    </section>
  }

  if (busy === 'load' && !profiles.length) {
    return <section className="creative-planner-empty" role="status">
      <LoaderCircle className="spin" size={24}/><h2>正在读取业务能力目录</h2>
    </section>
  }

  return <section className="creative-task-planner">
    <header className="creative-planner-heading">
      <div>
        <span className="section-label">CREATIVE TASK STRATEGY</span>
        <h2>选择业务，再生成可执行前的任务策略</h2>
        <p>推荐是辅助，不是限制。你可以选择任意可用业务；Strategy 只输出方向、变量和约束，不代替 Creative 写脚本或分镜。</p>
      </div>
      <button className="icon-button" aria-label="刷新创意任务策略" disabled={Boolean(busy)} onClick={() => void load()}>
        <RefreshCw size={15}/>
      </button>
    </header>

    {error ? <div className="kanon-strategy-alert" role="alert"><AlertCircle size={15}/><span>{error}</span></div> : null}

    {activePlan?.current_strategy && selectedProfile
      ? <StrategyResult
        busy={busy === 'handoff'}
        capability={capabilities.find(item => item.business_code === activePlan.business_code)}
        handoff={creativeHandoff}
        onHandoff={handoff}
        onRepair={onOpenStrategy}
        plan={activePlan}
        profile={selectedProfile}
      />
      : null}

    {activePlan?.current_strategy ? <button
      aria-expanded={showSetup}
      className="creative-setup-toggle"
      onClick={() => setShowSetup(value => !value)}
      type="button"
    >
      <span><b>{showSetup ? '收起任务策略设置' : '需要调整业务或任务输入？'}</b><small>当前结果已冻结；修改会创建新的计划或版本。</small></span>
      <ChevronRight className={showSetup ? 'expanded' : ''} size={15}/>
    </button> : null}

    {!activePlan?.current_strategy || showSetup ? <>
    <div className="creative-planner-layout">
      <div className="creative-business-picker">
        <div className="creative-planner-section-title">
          <div><b>01 选择业务</b><small>系统推荐排在前面，但不会替你做最终决定</small></div>
          {activePlan
            ? <button className="text-button" disabled={Boolean(busy)} onClick={() => {
              setActivePlanId('')
              setSelectedCode(recommendation?.recommended[0]?.business_code ?? profiles[0]?.business_code ?? '')
              setAnswers({})
              setError('')
            }}>新建业务选择</button>
            : <span>Brief v{briefVersion.version}</span>}
        </div>
        {recommendation ? <RecommendationBasis recommendation={recommendation} projectId={projectId}/> : null}
        <div className="creative-business-grid">
          {orderedProfiles.map(profile => {
            const recommendationItem = recommendation?.recommended.find(item => item.business_code === profile.business_code)
            const alternativeItem = recommendation?.alternatives.find(item => item.business_code === profile.business_code)
            const assessment = recommendationItem ?? alternativeItem
            return <button
              className={`${profile.business_code === selectedCode ? 'active' : ''}${recommendationItem ? ' recommended' : ''}`}
              disabled={!profile.selectable || Boolean(activePlan)}
              key={profile.business_code}
              aria-pressed={profile.business_code === selectedCode}
              onClick={() => setSelectedCode(profile.business_code)}
            >
              <span>{recommendationItem
                ? `${recommendationItem.rank === 1 ? '首选推荐' : `推荐 ${recommendationItem.rank}`} · ${confidenceLabel(recommendationItem.confidence)}`
                : '可自主选择'}</span>
              <b>{profile.display_name}</b>
              <p>{profile.summary}</p>
              <small>{recommendationItem?.reasons.slice(0, 2).join('；') ??
                assessment?.exclusion_reasons[0] ??
                '当前 Brief 依据不足，但可根据业务判断直接选择'}</small>
              <ChevronRight size={15}/>
            </button>
          })}
        </div>
        {!activePlan && selectedProfile ? <SelectedBusinessPreview
          profile={selectedProfile}
          recommendation={selectedRecommendation}
        /> : null}
        {!activePlan ? <div className="creative-question creative-route-picker">
          <span><label htmlFor="creative-route-select">本次创作路线</label><em>决定进入哪个创作工作台</em></span>
          <small>路线会冻结渠道、内容形式以及品牌/效果目的，避免 Strategy 与 Creative 各自理解一套。</small>
          {compatibleRoutes.length
            ? <select id="creative-route-select" value={selectedRouteId} onChange={event => setSelectedRouteId(event.target.value)}>
              {compatibleRoutes.map(route => <option key={route.route_id} value={route.route_id}>
                {creativeRouteLabel(route)} · {route.reason}
              </option>)}
            </select>
            : routeState ? <div className="creative-route-state" role="status"><AlertCircle size={16}/><span><b>{routeState.title}</b><small>{routeState.detail}</small>{routeBlockers.length > 1 ? <small>另有 {routeBlockers.length - 1} 项需补齐</small> : null}</span><button className="text-button" disabled={Boolean(busy)} onClick={() => void createRouteRevision()} type="button">{busy === 'route-repair' ? '正在创建…' : routeRevision?.changed ? '创建 Route 修订' : '去策略创建修订'}</button></div> : null}
        </div> : null}
        {!activePlan ? <button className="primary-button creative-plan-create" disabled={!selectedCode || !strategyPackage || !selectedRouteId || Boolean(busy)} onClick={() => void createPlan()}>
          {busy === 'create' ? <LoaderCircle className="spin" size={15}/> : <Check size={15}/>}
          {createPlanLabel}
        </button> : null}
      </div>

      <aside className="creative-plan-history">
        <div className="creative-planner-section-title"><div><b>已有计划</b><small>同一 Brief 可保留多次业务选择</small></div></div>
        {plans.map(plan => {
          const currentAssessment = recommendation?.recommended.find(item => item.business_code === plan.business_code) ??
            recommendation?.alternatives.find(item => item.business_code === plan.business_code)
          const selectionLabel = plan.selection_source === 'recommended'
            ? currentAssessment?.eligible === false ? '历史推荐；当前政策未推荐' : '来自当前推荐'
            : '用户自主选择'
          return <button className={plan.id === activePlanId ? 'active' : ''} key={plan.id} onClick={() => selectPlan(plan)}>
            <b>{plan.profile?.display_name ?? profileName(profiles, plan.business_code)}</b>
            <span>{plan.status} · Revision {plan.current_revision}</span>
            <small>{selectionLabel}</small>
          </button>
        })}
        {!plans.length ? <div className="panel-empty">当前 Brief 还没有任务计划。</div> : null}
      </aside>
    </div>

    {activePlan && selectedProfile ? <div className="creative-plan-workbench">
      <div className="creative-planner-section-title">
        <div><b>02 补充业务增量问题</b><small>Brief 已有信息只读展示，不重复询问</small></div>
        <span className={activePlan.completeness.ready ? 'ready' : 'blocked'}>
          {activePlan.completeness.ready ? '可以生成' : `${activePlan.completeness.blockers.length} 项待补`}
        </span>
      </div>
      <div className="creative-question-grid">
        {selectedProfile.questions.filter(question => {
          if (!question.depends_on) return true
          return answers[question.depends_on.question_id] === question.depends_on.equals
        }).map(question => {
          const briefValue = question.brief_source_path
            ? readPath(briefVersion.snapshot, question.brief_source_path)
            : undefined
          const inherited = question.brief_source_path ? hasDisplayValue(briefValue) : false
          const value = inherited ? briefValue : answers[question.id]
          const inputId = `creative-question-${activePlan.id}-${question.id}`
          return <div className={`creative-question${inherited ? ' readonly' : ''}`} key={question.id}>
            <span><label htmlFor={inherited ? undefined : inputId}>{question.label}</label>{question.required_for === 'strategy' ? <em>生成前必填</em> : null}</span>
            {inherited
              ? <output>{displayValue(value)}</output>
              : <QuestionInput
                disabled={Boolean(busy) || activePlan.status === 'generating'}
                inputId={inputId}
                profile={selectedProfile}
                questionId={question.id}
                value={value}
                mediaAssets={mediaAssets}
                onChange={next => setAnswers(current => ({ ...current, [question.id]: next }))}
              />}
            {question.help ? <small>{question.help}</small> : null}
          </div>
        })}
      </div>
      <div className="creative-plan-actions">
        <div>
          {activePlan.completeness.blockers.map(item => <span key={`${item.field}-${item.reason}`}><AlertCircle size={13}/>{item.reason}</span>)}
          {activePlan.completeness.warnings.map(item => <span className="warning" key={`${item.field}-${item.reason}`}>{item.reason}</span>)}
          {hasUnsavedAnswers ? <span className="unsaved">回答尚未保存；保存后会重新判断是否可以生成。</span> : null}
        </div>
        <button className="secondary-button" disabled={Boolean(busy) || !hasUnsavedAnswers} onClick={() => void saveAnswers()}>
          {busy === 'save' ? '保存中…' : '保存并重新校验'}
        </button>
        <button className="primary-button" disabled={!activePlan.completeness.ready || hasUnsavedAnswers || Boolean(busy) || activePlan.status === 'generated'} onClick={() => void generate()}>
          {busy === 'generate' ? <><LoaderCircle className="spin" size={15}/>生成中…</> : activePlan.status === 'generated' ? '已生成' : '生成任务策略'}
        </button>
      </div>
    </div> : null}
    </> : null}
  </section>
}

function QuestionInput({ disabled, inputId, profile, questionId, value, mediaAssets, onChange }: {
  disabled: boolean
  inputId: string
  profile: CreativeBusinessProfile
  questionId: string
  value: unknown
  mediaAssets: ApiProjectMediaAsset[]
  onChange: (value: unknown) => void
}) {
  const question = profile.questions.find(item => item.id === questionId)
  if (!question) return null
  if (question.type === 'asset_ref') {
    const selectableAssets = mediaAssets.filter(asset => {
      if (asset.kind === 'document') return false
      if (question.id.includes('video')) return asset.kind === 'video'
      if (question.id.includes('image')) return asset.kind === 'image'
      return asset.kind === 'image' || asset.kind === 'video'
    })
    const selected = isAssetRef(value) ? assetKey(value) : ''
    return <div className="creative-asset-answer">
      <select disabled={disabled} id={inputId} value={selected} onChange={event => {
        const asset = selectableAssets.find(item => assetKey({ asset_id: item.id, version: item.version }) === event.target.value)
        onChange(asset ? { asset_id: asset.id, version: asset.version } : undefined)
      }}>
        <option value="">请选择当前 Project 中的素材</option>
        {selectableAssets.map(asset => <option key={`${asset.id}:${asset.version}`} value={assetKey({
          asset_id: asset.id,
          version: asset.version,
        })}>{asset.kind === 'image' ? '图片' : asset.kind === 'video' ? '视频' : '文档'} · {asset.id} v{asset.version}{asset.durationSeconds ? ` · ${asset.durationSeconds.toFixed(1)}s` : ''}</option>)}
      </select>
      {!selectableAssets.length ? <small>当前 Project 暂无匹配素材；请先到素材库上传并完成处理。</small> : null}
    </div>
  }
  if (question.type === 'boolean') {
    return <select disabled={disabled} id={inputId} value={value === true ? 'true' : value === false ? 'false' : ''} onChange={event =>
      onChange(event.target.value === '' ? undefined : event.target.value === 'true')
    }><option value="">请选择</option><option value="true">是</option><option value="false">否</option></select>
  }
  if (question.type === 'single_select') {
    return <select disabled={disabled} id={inputId} value={typeof value === 'string' ? value : ''} onChange={event =>
      onChange(event.target.value || undefined)
    }>
      <option value="">请选择</option>
      {question.options?.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
    </select>
  }
  if (question.type === 'multi_select') {
    const selected = Array.isArray(value) ? value.map(String) : []
    return <div aria-labelledby={inputId} className="creative-question-options" id={inputId}>{question.options?.map(option => <label key={option.value}>
      <input checked={selected.includes(option.value)} disabled={disabled} type="checkbox" onChange={event => onChange(
        event.target.checked
          ? [...selected, option.value]
          : selected.filter(item => item !== option.value),
      )}/><span>{option.label}</span>
    </label>)}</div>
  }
  if (question.type === 'textarea') {
    return <textarea disabled={disabled} id={inputId} maxLength={question.validation?.max_length} rows={3} value={typeof value === 'string' ? value : ''} onChange={event => onChange(event.target.value)}/>
  }
  return <input disabled={disabled} id={inputId} maxLength={question.validation?.max_length} type={question.type === 'reference_locator' ? 'url' : 'text'} value={typeof value === 'string' ? value : ''} onChange={event => onChange(event.target.value)}/>
}

function RecommendationBasis({ recommendation, projectId }: {
  recommendation: CreativeBusinessRecommendationSnapshot
  projectId: string
}) {
  const signals = recommendation.signals
  const primary = recommendation.recommended[0]
  const signalItems = [
    ['目标', objectiveLabel(signals.objective_type)],
    ['渠道', signals.channels.map(channelLabel).join('、') || '未识别'],
    ['交付', deliverableLabel(signals.deliverable_type)],
    ['行业', signals.industry || '未填写'],
  ]
  return <section className="creative-recommendation-basis">
    <header>
      <div><ShieldCheck size={15}/><b>{primary ? `系统首选：${primary.display_name}` : '当前没有自动推荐'}</b><span>只读 Brief v{recommendation.brief_version}</span></div>
      <small>{recommendation.policy_version}</small>
    </header>
    <p className={primary ? 'recommendation-summary' : 'recommendation-summary warning'}>{primary
      ? `${confidenceLabel(primary.confidence)}：${primary.reasons.slice(0, 2).join('；')}`
      : 'Brief 缺少可识别的目标、渠道或交付形式；你仍可自主选择，但系统不会伪装成有依据的推荐。'}</p>
    <div className="creative-signal-chips">
      {signalItems.map(([label, value]) => <span key={label}><small>{label}</small>{value}</span>)}
    </div>
    {recommendation.media.items.length
      ? <MediaAssessmentPanel assessment={recommendation.media} projectId={projectId} title="Brief 素材有效性"/>
      : <p><CircleHelp size={13}/>Brief 没有可核验的图片或视频引用；推荐不会假装看过素材。</p>}
  </section>
}

function creativeBusinessMatchesRoute(
  businessCode: string,
  route: StrategyCreativeHandoff['routes'][number],
) {
  switch (businessCode) {
    case 'xiaohongshu_image_text':
      return route.deliverable_type === 'image_text'
    case 'wechat_article':
      return route.deliverable_type === 'image_text' && route.channels.some(channel =>
        channel === 'wechat_ecosystem' || channel === 'wechat_official_account',
      )
    case 'brand_video':
      return route.deliverable_type === 'video' && route.purpose === 'brand'
    case 'short_drama_preroll':
    case 'game_preroll':
    case 'commerce_preroll':
    case 'viral_remake':
      return route.deliverable_type === 'video' &&
        route.purpose === 'performance' &&
        route.performance_mode === businessCode
    default:
      return false
  }
}

function creativeRouteLabel(route: StrategyCreativeHandoff['routes'][number]) {
  const purpose = route.purpose === 'brand' ? '品牌广告' : '效果广告'
  const channels = route.channels.map(channelLabel).join(' / ') || '渠道待确认'
  return `${channels} · ${deliverableLabel(route.deliverable_type)} · ${purpose}`
}

function handoffIssueIsOptionalContext(issue: StrategyCreativeHandoffIssue) {
  return issue.code === 'market_missing' || issue.code === 'language_missing'
}

function handoffHardBlockers(handoff: StrategyCreativeHandoff | null, selectedRouteId: string) {
  if (!handoff) return []
  const selectedRouteReady = handoff.routes.some(route =>
    route.route_id === selectedRouteId && route.route_readiness.status === 'ready',
  )
  return (handoff.upstream_readiness.blockers ?? []).filter(issue =>
    !handoffIssueIsOptionalContext(issue) &&
    !(issue.code === 'strategy_creative_not_ready' && selectedRouteReady),
  )
}

function SelectedBusinessPreview({ profile, recommendation }: {
  profile: CreativeBusinessProfile
  recommendation?: CreativeBusinessRecommendation
}) {
  return <section className="creative-business-preview">
    <div>
      <span>{recommendation?.eligible ? '为什么适合' : '自主选择提示'}</span>
      <b>{profile.display_name}</b>
      <p>{recommendation?.eligible
        ? recommendation.reasons.join('；')
        : recommendation?.exclusion_reasons.join('；') ||
          '当前 Brief 没有足够依据自动推荐，但你仍可基于实际业务判断选择。'}</p>
    </div>
    <div>
      <span>生成策略前</span>
      <ul>{profile.requirements.strategy.map(item => <li key={item}>{item}</li>)}</ul>
    </div>
    <div>
      <span>进入生产前</span>
      <ul>{profile.requirements.production.map(item => <li key={item}>{item}</li>)}</ul>
    </div>
  </section>
}

function StrategyResult({ busy, capability, handoff, onHandoff, onRepair, plan, profile }: {
  busy: boolean
  capability?: CreativeBusinessCapability
  handoff: StrategyCreativeHandoff | null
  onHandoff: (plan: CreativeTaskPlan, capability: CreativeBusinessCapability) => Promise<void>
  onRepair: () => void
  plan: CreativeTaskPlan
  profile: CreativeBusinessProfile
}) {
  const strategy = plan.current_strategy
  if (!strategy) return null
  const document = strategy.document
  const hasFrozenLineage = plan.contract_version === 'strategy-creative-task-plan/v2' &&
    Boolean(plan.package_ref && plan.handoff_ref && plan.selected_route_id && strategy.task_overlay_ref)
  const selectedRoute = handoff?.routes.find(route => route.route_id === plan.selected_route_id)
  const hardBlockers = handoffHardBlockers(handoff, plan.selected_route_id ?? '')
  const contextWarnings = handoff?.upstream_readiness.blockers?.filter(handoffIssueIsOptionalContext) ?? []
  const routeReady = selectedRoute?.route_readiness.status === 'ready'
  const handoffAvailable = capability?.status === 'available' && hasFrozenLineage && routeReady && !hardBlockers.length
  const handoffLimitation = hasFrozenLineage
    ? hardBlockers[0] ? routeIssueMessage(hardBlockers[0]) : capability?.limitation || '当前创作路线尚未就绪。'
    : '历史 v1 计划没有冻结交接血缘；请新建业务选择并创建 v2 任务计划。'
  return <div className="creative-strategy-result">
    <div className="creative-result-heading">
      <div>
        <span className="section-label">READY FOR CREATIVE</span>
        <b>{document.core_message}</b>
        <small>{profile.display_name} · 任务策略 v{strategy.version}</small>
      </div>
      <div>
        <span className="creative-result-status"><ShieldCheck size={13}/>已冻结并可追溯</span>
        <button
          className="primary-button"
          disabled={busy || !handoffAvailable}
          onClick={() => capability && void onHandoff(plan, capability)}
          title={handoffAvailable
            ? '创建冻结的 CreativeIntake 并进入对应工作台'
            : handoffLimitation}
        >
          {busy ? <LoaderCircle className="spin" size={14}/> : <Rocket size={14}/>}
          {handoffAvailable ? '进入创意创作' : hasFrozenLineage ? '先修复交接条件' : '需新建 v2 计划'}
        </button>
        <a className="secondary-button" download href={`/api/strategy/v1/creative-task-plans/${encodeURIComponent(plan.id)}/strategy-versions/${strategy.version}/export.md`}>
          <Download size={14}/>导出 Markdown
        </a>
      </div>
    </div>
    <div className={`creative-handoff-status ${handoffAvailable ? 'available' : 'unavailable'}`}>
      <div>
        <b>{handoffAvailable ? '已可交接到创意工作台' : hasFrozenLineage ? '任务策略已冻结，但交接条件未完成' : '历史计划仅供查看'}</b>
        <span>{handoffAvailable
          ? '交接后会继承目标、受众、业务专属判断、约束、素材引用和版本血缘。'
          : handoffLimitation}</span>
      </div>
      {!handoffAvailable && hasFrozenLineage
        ? <button className="text-button" onClick={onRepair} type="button">回到策略修复</button>
        : capability?.production_inputs.length
        ? <small>进入生产后还需确认：{capability.production_inputs.join('、')}</small>
        : null}
    </div>
    {handoffAvailable && contextWarnings.length ? <div className="creative-context-warning" role="status">
      <CircleHelp size={14}/><span><b>可继续，不强迫补表</b><small>{contextWarnings
        .map(issue => routeIssueMessage(issue).replace(/[。；]+$/, ''))
        .join('；')}。需要时在创作前确认。</small></span>
    </div> : null}
    <div className="creative-strategy-summary">
      <article><span>目标</span><b>{document.objective}</b></article>
      <article><span>核心信息</span><b>{document.core_message}</b></article>
      <article><span>核心受众</span><b>{document.audience.primary}</b></article>
    </div>
    <BusinessStrategyResult profile={profile} values={document.business_strategy}/>
    <div className="creative-strategy-columns">
      <ResultList title="信息层级" items={document.message_hierarchy}/>
      <ResultList title="验证假设" items={document.hypotheses.map(item =>
        `${item.statement}（变量：${item.variable}；指标：${item.metric}）`
      )}/>
      <ResultList title="事实与证据" items={document.claims_and_evidence}/>
      <ResultList title="资产要求" items={document.asset_requirements.map(item => `${item.role}：${item.requirement}`)}/>
      <ResultList title="边界与约束" items={document.guardrails}/>
      <ResultList title="待确认问题" items={document.open_questions}/>
    </div>
    <MediaAssessmentPanel
      assessment={document.media}
      projectId={plan.project_id}
      title="素材如何参与了这份策略"
    />
    <details className="kanon-technical-details creative-result-lineage">
      <summary>查看版本血缘</summary>
      <code>{strategy.content_hash}</code>
    </details>
  </div>
}

function BusinessStrategyResult({ profile, values }: {
  profile: CreativeBusinessProfile
  values: Record<string, unknown>
}) {
  return <div className="creative-business-strategy-result">
    <div><span>业务专属判断</span><small>以下结论由已确认 Brief、业务答案和可用素材证据推导</small></div>
    <div>{profile.output_fields.map(field => {
      const value = values[field.key]
      return <article key={field.key}>
        <span>{field.label}</span>
        {Array.isArray(value)
          ? <ul>{value.map((item, index) => <li key={`${index}-${String(item)}`}>{String(item)}</li>)}</ul>
          : <p>{displayValue(value)}</p>}
      </article>
    })}</div>
  </div>
}

function MediaAssessmentPanel({ assessment, projectId, title }: {
  assessment?: CreativeMediaAssessment
  projectId?: string
  title: string
}) {
  if (!assessment?.items?.length) return null
  return <section className="creative-media-assessment">
    <header>
      <div><ImageIcon size={14}/><b>{title}</b></div>
      <span>{assessment.semantic_count} 项读过语义 · {assessment.production_only_count} 项仅核验规格</span>
    </header>
    <div>{assessment.items.map(item => {
      const url = projectId && item.status === 'ready'
        ? `/platform/v1/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(item.asset_ref.asset_id)}/versions/${item.asset_ref.version}/content`
        : ''
      return <article key={`${item.origin}:${item.role}:${assetKey(item.asset_ref)}`}>
        <MediaPreview item={item} url={url}/>
        <div>
          <span className={item.usefulness}>{mediaUsefulnessLabel(item.usefulness)}</span>
          <b>{item.role} · {item.asset_ref.asset_id} v{item.asset_ref.version}</b>
          <small>{mediaMetadata(item)}</small>
          {item.observations.slice(0, 3).map(value => <p key={value}>已读取：{value}</p>)}
          {item.limitations.map(value => <p className="limitation" key={value}>限制：{value}</p>)}
        </div>
      </article>
    })}</div>
    {assessment.warnings.map(value => <p className="creative-media-warning" key={value}>{value}</p>)}
  </section>
}

function MediaPreview({ item, url }: {
  item: CreativeMediaAssessment['items'][number]
  url: string
}) {
  const [failed, setFailed] = useState(false)
  const fallback = <div className="creative-media-preview unavailable">
    {item.kind === 'video' ? <Film size={19}/> : <ImageIcon size={19}/>}
    <small>{failed ? '预览文件不可用' : '暂无可用预览'}</small>
  </div>
  if (!url || failed) return fallback
  return <div className="creative-media-preview">
    {item.kind === 'image'
      ? <img alt={`${item.role} 素材预览`} loading="lazy" onError={() => setFailed(true)} src={url}/>
      : item.kind === 'video'
        ? <video aria-label={`${item.role} 视频预览`} controls onError={() => setFailed(true)} preload="metadata" src={url}/>
        : fallback}
  </div>
}

function ResultList({ title, items }: { title: string; items: string[] }) {
  const emphasized = title === '待确认问题' || title === '边界与约束'
  return <article className={emphasized ? 'emphasized' : ''}><b>{title}</b>{items.length
    ? <ul>{items.map((item, index) => <li key={`${index}-${item}`}>{item}</li>)}</ul>
    : <p className="creative-empty-value">当前没有已确认内容</p>}</article>
}

function isAssetRef(value: unknown): value is { asset_id: string; version: number } {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value) &&
    typeof (value as { asset_id?: unknown }).asset_id === 'string' &&
    typeof (value as { version?: unknown }).version === 'number')
}

function assetKey(value: { asset_id: string; version: number }) {
  return `${value.asset_id}:${value.version}`
}

function confidenceLabel(value: CreativeBusinessRecommendation['confidence']) {
  return value === 'high' ? '高依据' : value === 'medium' ? '中依据' : '低依据'
}

function objectiveLabel(value: string) {
  const labels: Record<string, string> = {
    awareness: '品牌认知', consideration: '种草/考虑', lead: '线索',
    conversion: '转化', sales: '销售', install: '安装', reactivation: '召回',
  }
  return labels[value] ?? '未识别'
}

function channelLabel(value: string) {
  const labels: Record<string, string> = {
    xiaohongshu: '小红书', douyin: '抖音', kuaishou: '快手',
    wechat_ecosystem: '微信生态', taobao_tmall: '淘宝/天猫',
  }
  return labels[value] ?? value
}

function deliverableLabel(value: string) {
  return value === 'image_text' ? '图文' : value === 'video' ? '视频' : value === 'mixed' ? '图文 + 视频' : '未识别'
}

function mediaUsefulnessLabel(value: CreativeMediaAssessment['items'][number]['usefulness']) {
  return value === 'semantic' ? '已用于策略' : value === 'production_only' ? '仅用于生产准备' : '当前不可用'
}

function mediaMetadata(item: CreativeMediaAssessment['items'][number]) {
  const dimensions = item.width_pixels && item.height_pixels ? `${item.width_pixels}×${item.height_pixels}` : ''
  const duration = item.duration_seconds ? `${item.duration_seconds.toFixed(1)} 秒` : ''
  return [item.kind === 'image' ? '图片' : item.kind === 'video' ? '视频' : item.kind, dimensions, duration]
    .filter(Boolean).join(' · ')
}

function readPath(value: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object' || Array.isArray(current)) return undefined
    return (current as Record<string, unknown>)[part]
  }, value)
}

function displayValue(value: unknown) {
  if (Array.isArray(value)) return value.join('、') || '未填写'
  if (value === true) return '是'
  if (value === false) return '否'
  if (value === null || value === undefined || value === '') return 'Brief 中未填写'
  return String(value)
}

function hasDisplayValue(value: unknown) {
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'string') return value.trim() !== ''
  return value !== null && value !== undefined
}

function profileName(profiles: CreativeBusinessProfile[], businessCode: string) {
  return profiles.find(profile => profile.business_code === businessCode)?.display_name ?? businessCode
}

function messageOf(cause: unknown) {
  if (cause instanceof BackendApiError && cause.code === 'FEATURE_DISABLED') {
    return '创意任务策略功能尚未在当前环境开放。'
  }
  if (cause instanceof BackendApiError && cause.code === 'CATALOG_CHANGED') {
    return '业务能力目录已经更新，页面已刷新，请重新确认选择。'
  }
  if (cause instanceof BackendApiError && cause.code === 'TASK_PLAN_BLOCKED') {
    return '还有生成前必填信息未完成。'
  }
  return cause instanceof Error ? cause.message : '创意任务策略操作失败。'
}

function routeIssueMessage(issue: { code: string; message: string }) {
  if (issue.code === 'creative_route_missing' || issue.code === 'creative_route_mode_missing') {
    return '请在渠道策略中明确“小红书图文笔记”等内容形式和品牌/效果目的，保存新 Revision 后重新评审发布。'
  }
  if (issue.code === 'route_purpose_missing' || issue.code === 'objective_type_missing') {
    return '请在策略目标或渠道角色中明确这是品牌认知还是效果获客，保存新 Revision 后重新评审发布。'
  }
  return issue.message
}
