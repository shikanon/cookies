import { useEffect, useMemo, useReducer, useRef, useState } from 'react'
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  ChevronDown,
  CircleAlert,
  Clock3,
  Eye,
  FileVideo2,
  Image as ImageIcon,
  Info,
  LoaderCircle,
  LockKeyhole,
  Maximize2,
  Play,
  RefreshCw,
  RotateCcw,
  Save,
  ScanSearch,
  ShieldCheck,
  Sparkles,
  Upload,
  WandSparkles,
  X,
} from 'lucide-react'
import { useProject } from '../../context/ProjectContext'
import type { CommercePrerollGateway } from './gateway'
import { HttpCommercePrerollGateway } from './httpGateway'
import { canOpenCommercePrerollStep, commercePrerollReducer, commercePrerollRetryOperation, createInitialCommercePrerollState } from './reducer'
import { clearCommercePrerollSession, readCommercePrerollSession, writeCommercePrerollSession } from './sessionStore'
import type { CommercePrerollCreativeVersion, CommercePrerollState, CommercePrerollStep, CommercePrerollTaskSummary, FirstFrameCandidate, HookProposal, ProductField, ProductReference, PrerollDuration, SourceVideo } from './types'
import './commerce-preroll-v2.css'

const steps: Array<{ id: CommercePrerollStep; index: string; label: string; detail: string }> = [
  { id: 'source', index: '01', label: '原视频', detail: '选择已完成的电商视频' },
  { id: 'understanding', index: '02', label: '内容理解', detail: '确认商品与卖点' },
  { id: 'direction', index: '03', label: '钩子方向', detail: '选择开场创意机制' },
  { id: 'settings', index: '04', label: '生成设置', detail: '时长与三段节奏' },
  { id: 'first-frame', index: '05', label: '参考首帧', detail: '选择视觉起点' },
  { id: 'video', index: '06', label: '前贴成片', detail: '生成与保存结果' },
]

const analysisStages = ['读取视频与音轨', '提取关键画面', '识别商品与卖点', '分析字幕与口播', '生成钩子建议']
const videoStages = ['提交生成规格', 'Seedance 生成画面', '回收并准备播放']
const validSteps = new Set<CommercePrerollStep>(steps.map(step => step.id))

function sourceFromUpload(file: File): SourceVideo {
  return {
    id: `local-source-${Date.now()}`,
    name: file.name,
    videoUrl: URL.createObjectURL(file),
    posterUrl: '',
    durationSeconds: 0,
    aspectRatio: '等待读取',
    resolution: '等待读取',
    sizeLabel: `${(file.size / 1024 / 1024).toFixed(1)} MB`,
    version: '本地草稿',
    sourceLabel: '本地上传',
    rightsStatus: 'unconfirmed',
    uploaded: true,
  }
}

function initialState(projectId: string) {
  const restored = readCommercePrerollSession(projectId)
  const base = restored ?? createInitialCommercePrerollState()
  const queryStep = new URL(window.location.href).searchParams.get('cpStep') as CommercePrerollStep | null
  return queryStep && validSteps.has(queryStep) && canOpenCommercePrerollStep(base, queryStep) ? { ...base, activeStep: queryStep } : base
}

function formatSavedAt(value: string | null) {
  if (!value) return '草稿将在本机自动保存'
  return `已自动保存 · ${new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}`
}

function StepRail({ state, dispatch }: { state: CommercePrerollState; dispatch: React.Dispatch<Parameters<typeof commercePrerollReducer>[1]> }) {
  const activeIndex = steps.findIndex(step => step.id === state.activeStep)
  return <aside className="commerce-preroll-v2-rail">
    <div className="commerce-preroll-v2-source-summary">
      <span className="commerce-preroll-v2-mono">SOURCE VIDEO</span>
      <div className="commerce-preroll-v2-source-thumb">
        {state.source ? <video src={state.source.videoUrl} poster={state.source.posterUrl} muted preload="metadata"/> : <FileVideo2 size={26}/>}<span><Play size={12} fill="currentColor"/></span>
      </div>
      <b>{state.source?.name ?? '尚未选择原视频'}</b>
      <small>{state.source ? `${state.source.durationSeconds || '—'} 秒 · ${state.source.aspectRatio}` : '先从 Project 素材或本地上传'}</small>
    </div>
    <nav aria-label="电商前贴创作流程"><ol>{steps.map((step, index) => {
      const enabled = canOpenCommercePrerollStep(state, step.id)
      const active = step.id === state.activeStep
      const completed = index < activeIndex || (step.id === 'video' && state.videoStatus === 'ready')
      const running = (step.id === 'understanding' && state.analysisStatus === 'loading') || (step.id === 'video' && state.videoStatus === 'loading')
      return <li key={step.id} className={`${active ? 'active' : ''} ${completed ? 'completed' : ''} ${running ? 'running' : ''}`}>
        <button type="button" disabled={!enabled} aria-current={active ? 'step' : undefined} onClick={() => dispatch({ type: 'open-step', step: step.id })} title={!enabled ? '完成前一步后即可进入' : undefined}>
          <span className="commerce-preroll-v2-step-index">{running ? <LoaderCircle className="commerce-preroll-v2-spin" size={14}/> : completed ? <Check size={14}/> : step.index}</span>
          <span><b>{step.label}</b><small>{step.detail}</small></span>
          <ArrowRight size={14}/>
        </button>
      </li>
    })}</ol></nav>
    <div className="commerce-preroll-v2-rail-note"><ShieldCheck size={16}/><span><b>独立前贴</b><small>输出 6–10 秒视频，本阶段不拼接原视频。</small></span></div>
  </aside>
}

function StageHeader({ state }: { state: CommercePrerollState }) {
  const step = steps.find(item => item.id === state.activeStep) ?? steps[0]
  const title = state.activeStep === 'understanding' && state.analysisStatus === 'loading' ? '正在理解原视频'
    : state.activeStep === 'understanding' ? '确认系统理解的商品信息'
    : state.activeStep === 'video' && state.videoStatus === 'loading' ? '正在生成前贴视频'
    : state.activeStep === 'video' && state.output ? '前贴视频已生成'
    : step.label
  const detail = state.activeStep === 'source' ? '选择要制作前贴的原视频，它只用于理解商品与风格，不会直接提交给 Seedance。'
    : state.activeStep === 'understanding' ? '系统提取结论，用户只确认需要决策的商品信息与风险事实。'
    : state.activeStep === 'direction' ? '五种创意机制已经结合当前原视频生成具体方案。'
    : state.activeStep === 'settings' ? '选择时长，系统将商品理解、钩子规则与镜头约束编译为 Prompt。'
    : state.activeStep === 'first-frame' ? '选择一张首帧作为 Seedance 的视觉参考。'
    : '生成一条独立前贴，并在当前页面直接播放与保存。'
  return <header className="commerce-preroll-v2-stage-header"><div><span className="commerce-preroll-v2-mono">COMMERCE PREROLL · {step.index}</span><h3>{title}</h3><p>{detail}</p></div><span className="commerce-preroll-v2-save-state"><CheckCircle2 size={13}/>{formatSavedAt(state.lastSavedAt)}</span></header>
}

function SourceStage({ state, onAnalyze, onSourceFile, dispatch }: { state: CommercePrerollState; onAnalyze: () => void; onSourceFile: (file: File) => void; dispatch: React.Dispatch<Parameters<typeof commercePrerollReducer>[1]> }) {
  const [detailsOpen, setDetailsOpen] = useState(false)
  return <section className="commerce-preroll-v2-stage commerce-preroll-v2-source-stage">
    <div className="commerce-preroll-v2-stage-toolbar"><div><b>选择要制作前贴的原视频</b><small>支持当前 Project 视频或本地 MP4 / WebM</small></div><label className="commerce-preroll-v2-secondary"><Upload size={15}/>上传新视频<input type="file" accept="video/mp4,video/webm,video/quicktime" onChange={event => { const file = event.target.files?.[0]; if (file) onSourceFile(file); event.target.value = '' }}/></label></div>
    <div className="commerce-preroll-v2-source-grid">
      <div className="commerce-preroll-v2-media-canvas">{state.source ? <video src={state.source.videoUrl} poster={state.source.posterUrl} controls preload="metadata"/> : <div><FileVideo2 size={34}/><b>选择一条原视频</b><small>用于提取商品、卖点与画面风格</small></div>}</div>
      <aside className="commerce-preroll-v2-inspector-card"><span>已选原视频</span><h4>{state.source?.name ?? '等待选择'}</h4>
        <dl><div><dt>基础信息</dt><dd>{state.source ? `${state.source.durationSeconds || '—'} 秒 · ${state.source.aspectRatio}` : '—'}</dd></div><div><dt>来源</dt><dd>{state.source?.sourceLabel ?? '—'}</dd></div><div><dt>素材版本</dt><dd>{state.source?.version ?? '—'}</dd></div></dl>
        <label className="commerce-preroll-v2-rights"><input type="checkbox" checked={state.rightsConfirmed} disabled={!state.source} onChange={event => dispatch({ type: 'rights-changed', confirmed: event.target.checked })}/><ShieldCheck size={16}/><span><b>确认拥有生成使用权</b><small>本步骤只记录确认，不替代正式授权审计。</small></span></label>
        <button className="commerce-preroll-v2-text-button" type="button" onClick={() => setDetailsOpen(value => !value)}>{detailsOpen ? '收起素材详情' : '查看素材详情'}<ChevronDown className={detailsOpen ? 'open' : ''} size={14}/></button>
        {detailsOpen && state.source ? <dl className="commerce-preroll-v2-detail-list"><div><dt>分辨率</dt><dd>{state.source.resolution}</dd></div><div><dt>文件大小</dt><dd>{state.source.sizeLabel}</dd></div><div><dt>技术状态</dt><dd>浏览器可播放</dd></div></dl> : null}
      </aside>
    </div>
    <footer><small>后续仍可返回更换原视频；更换后会清空依赖旧视频的分析与生成结果。</small><button className="commerce-preroll-v2-primary" disabled={!state.source || !state.rightsConfirmed} onClick={onAnalyze}><ScanSearch size={16}/>解析原视频</button></footer>
  </section>
}

function AnalysisProgress({ state }: { state: CommercePrerollState }) {
  return <section className="commerce-preroll-v2-stage commerce-preroll-v2-analysis-stage">
    <div className="commerce-preroll-v2-media-canvas compact"><video src={state.source?.videoUrl} poster={state.source?.posterUrl} muted autoPlay loop playsInline/><span className="commerce-preroll-v2-media-status"><ScanSearch size={14}/>正在抽取关键画面</span></div>
    <div className="commerce-preroll-v2-progress-panel" role="status"><div className="commerce-preroll-v2-progress-head"><div><span>分析进度</span><b>已完成 {Math.min(state.analysisStage, 5)} / 5</b></div><Clock3 size={20}/></div><div className="commerce-preroll-v2-progress-track"><i style={{ width: `${(state.analysisStage / 5) * 100}%` }}/></div>
      <ol>{analysisStages.map((label, index) => { const number = index + 1; const done = state.analysisStage >= number; const running = state.analysisStage + 1 === number; return <li key={label} className={done ? 'done' : running ? 'running' : ''}><span>{done ? <Check size={13}/> : running ? <LoaderCircle className="commerce-preroll-v2-spin" size={13}/> : number}</span><b>{label}</b><small>{done ? '已完成' : running ? '处理中' : '等待前序阶段'}</small></li> })}</ol>
      <div className="commerce-preroll-v2-info"><Info size={15}/><span>可以离开当前页面，任务会继续；刷新后会恢复当前阶段。</span></div>
    </div>
  </section>
}

function ProductFieldControl({ label, field, value, multiline, onChange }: { label: string; field: ProductField; value: string; multiline?: boolean; onChange: (field: ProductField, value: string) => void }) {
  return <label className={multiline ? 'wide' : ''}><span>{label}</span>{multiline ? <textarea value={value} onChange={event => onChange(field, event.target.value)}/> : <input value={value} onChange={event => onChange(field, event.target.value)}/>}</label>
}

function UnderstandingReady({ state, dispatch, onConfirm, onReferenceFile, onReferenceSelect, onReextract }: { state: CommercePrerollState; dispatch: React.Dispatch<Parameters<typeof commercePrerollReducer>[1]>; onConfirm: () => void; onReferenceFile: (file: File) => void; onReferenceSelect: (reference: ProductReference) => void; onReextract: () => void }) {
  const product = state.productDraft
  if (!product) return null
  const edit = (field: ProductField, value: string) => dispatch({ type: 'product-field-changed', field, value })
  const pendingRisk = state.riskFacts.some(item => item.status === 'pending')
  return <section className="commerce-preroll-v2-stage commerce-preroll-v2-understanding-ready">
    <div className="commerce-preroll-v2-understanding-grid"><div>
      <div className="commerce-preroll-v2-section-heading"><div><span className="commerce-preroll-v2-mono">EDITABLE FACTS</span><h4>商品与卖点</h4></div><small>{state.analysis?.evidenceCount ?? 0} 条后台证据已记录</small></div>
      <div className="commerce-preroll-v2-form-grid"><ProductFieldControl label="商品名称" field="name" value={product.name} onChange={edit}/><ProductFieldControl label="品类" field="category" value={product.category} onChange={edit}/><ProductFieldControl label="商品描述" field="description" value={product.description} multiline onChange={edit}/><ProductFieldControl label="核心卖点" field="sellingPoints" value={product.sellingPoints} multiline onChange={edit}/><ProductFieldControl label="商品外观与 Logo 保真" field="appearanceGuardrails" value={product.appearanceGuardrails} multiline onChange={edit}/></div>
      {state.riskFacts.filter(item => item.status === 'pending').map(item => <div className="commerce-preroll-v2-risk" key={item.id}><CircleAlert size={17}/><div><b>需要确认：{item.text}</b><small>{item.sourceLabel}，属于功效表达。</small></div><button onClick={() => dispatch({ type: 'risk-resolved', id: item.id, status: 'confirmed' })}>确认保留</button><button onClick={() => dispatch({ type: 'risk-resolved', id: item.id, status: 'removed' })}>删除</button></div>)}
      <details className="commerce-preroll-v2-analysis-details"><summary>查看画面、字幕与音频理解</summary><dl><div><dt>视觉风格</dt><dd>{state.analysis?.visualStyle}</dd></div><div><dt>字幕</dt><dd>{state.analysis?.subtitleSummary}</dd></div><div><dt>口播</dt><dd>{state.analysis?.voiceSummary}</dd></div><div><dt>音频气质</dt><dd>{state.analysis?.audioMood}</dd></div><div><dt>开场镜头</dt><dd>{state.analysis?.openingShot}</dd></div></dl></details>
      <section className="commerce-preroll-v2-reference-library">
        <div className="commerce-preroll-v2-reference-library-heading"><div><span className="commerce-preroll-v2-mono">REFERENCE CANDIDATES</span><h4>商品参考图候选</h4><p>选择一张作为后续前贴生成的商品保真依据。</p></div><div><label className="commerce-preroll-v2-secondary"><Upload size={14}/>上传商品图<input type="file" accept="image/*" onChange={event => { const file = event.target.files?.[0]; if (file) onReferenceFile(file); event.target.value = '' }}/></label><button className="commerce-preroll-v2-secondary" onClick={onReextract}><RefreshCw size={14}/>重新提取</button></div></div>
        <div className="commerce-preroll-v2-reference-candidates">{state.productReferences.map((reference, index) => <button type="button" className={reference.id === state.productReference?.id ? 'selected' : ''} key={reference.id} onClick={() => onReferenceSelect(reference)}><img src={reference.imageUrl} alt={reference.label}/><span>{index === 0 && reference.kind === 'extracted' ? '系统主推 · ' : ''}{reference.label}</span><small>{reference.qualitySummary ?? reference.sourceLabel}</small><small>{reference.sourceLabel}</small></button>)}</div>
      </section>
    </div><aside className="commerce-preroll-v2-reference-panel"><span className="commerce-preroll-v2-mono">SELECTED PRODUCT REFERENCE</span><h4>已选商品图</h4>{state.productReference ? <img src={state.productReference.imageUrl} alt={state.productReference.label}/> : <div className="commerce-preroll-v2-reference-empty">请选择一张候选图</div>}<p>{state.productReference?.label ?? '尚未选择商品参考图'}</p><small>{state.productReference?.qualitySummary ?? state.productReference?.sourceLabel}</small></aside></div>
    <footer><button className="commerce-preroll-v2-secondary" onClick={() => dispatch({ type: 'open-step', step: 'source' })}><ArrowLeft size={15}/>返回原视频</button><button className="commerce-preroll-v2-primary" disabled={pendingRisk || !product.name.trim() || !product.sellingPoints.trim()} onClick={onConfirm}>{state.hooksStatus === 'loading' ? <LoaderCircle className="commerce-preroll-v2-spin" size={16}/> : <Sparkles size={16}/>}确认并查看钩子</button></footer>
  </section>
}

function HookCard({ hook, selected, onSelect }: { hook: HookProposal; selected: boolean; onSelect: () => void }) {
  return <button type="button" className={`commerce-preroll-v2-hook-card ${selected ? 'selected' : ''}`} onClick={onSelect}>{selected ? <span className="commerce-preroll-v2-selected-mark"><Check size={13}/></span> : null}<img src={hook.imageUrl} alt=""/><div><span className="commerce-preroll-v2-mono">{hook.recommendationLevel === 'primary' ? '系统主推荐' : '备选创意'} · {Math.round((hook.matchScore ?? 0.7) * 100)}% 匹配</span><h4>{hook.name}</h4><p>{hook.concept}</p><small><b>视觉特点</b>{hook.visualSignature ?? hook.mechanism}</small><small><b>推荐理由</b>{hook.rationale}</small><small><b>使用卖点</b>{hook.sellingPoint}</small></div></button>
}

function DirectionStage({ state, onSelectHook, onRefresh, onContinue, dispatch }: { state: CommercePrerollState; onSelectHook: (hook: HookProposal) => void; onRefresh: () => void; onContinue: () => void; dispatch: React.Dispatch<Parameters<typeof commercePrerollReducer>[1]> }) {
  const selected = state.hooks.find(item => item.id === state.selectedHookId)
  return <section className="commerce-preroll-v2-stage commerce-preroll-v2-direction-stage"><div className="commerce-preroll-v2-stage-toolbar"><div><b>选择前贴钩子方向</b><small>不在此步骤固定秒数，时长将在下一步选择。</small></div><button className="commerce-preroll-v2-secondary" onClick={onRefresh}><RefreshCw size={14}/>重新推荐</button></div>
    <div className="commerce-preroll-v2-hook-grid">{state.hooks.map(hook => <HookCard key={hook.id} hook={hook} selected={hook.id === state.selectedHookId} onSelect={() => onSelectHook(hook)}/>)}</div>
    {selected ? <div className="commerce-preroll-v2-hook-detail"><div><span>为什么匹配原片</span><b>{selected.whyForSource?.join('；') ?? selected.rationale}</b></div><div><span>适用场景</span><b>{selected.suitableFor?.join('；') ?? '当前商品与已确认卖点'}</b></div><div><span>开场状态</span><b>{selected.openingState ?? selected.concept}</b></div><div><span>唯一主动作</span><b>{selected.action}</b></div><div><span>最终定格</span><b>{selected.resultState ?? '商品正面清晰稳定'}</b></div><div><span>衔接方式</span><b>{selected.continuityPlan ?? '向原视频开场的构图与光线靠拢'}</b></div><div><span>风险提醒</span><b>{selected.riskNotes?.join('；') ?? '不得改变商品外观与 Logo'}</b></div></div> : null}
    <footer><button className="commerce-preroll-v2-secondary" onClick={() => dispatch({ type: 'open-step', step: 'understanding' })}><ArrowLeft size={15}/>上一步</button><button className="commerce-preroll-v2-primary" disabled={!selected} onClick={onContinue}>使用此钩子<ArrowRight size={15}/></button></footer>
  </section>
}

type EditableBeatField = 'visualDescription' | 'subjectAction' | 'camera' | 'sceneAndLighting' | 'productState' | 'transitionIn' | 'transitionOut' | 'onScreenText' | 'audioInstruction'

function SettingsStage({ state, onDuration, onInstruction, onFrames, onStoryboard, onPrompt, dispatch }: { state: CommercePrerollState; onDuration: (duration: PrerollDuration) => void; onInstruction: (value: string) => void; onFrames: () => void; onStoryboard: (index: number, field: EditableBeatField, value: string) => void; onPrompt: (value: string) => void; dispatch: React.Dispatch<Parameters<typeof commercePrerollReducer>[1]> }) {
  const hook = state.hooks.find(item => item.id === state.selectedHookId)
  const beats = state.generationDraft?.beats ?? []
  return <section className="commerce-preroll-v2-stage commerce-preroll-v2-settings-stage"><div className="commerce-preroll-v2-settings-grid"><div>
    <div className="commerce-preroll-v2-section-heading"><div><span className="commerce-preroll-v2-mono">DURATION</span><h4>前贴时长</h4></div><small>钩子卡不锁时长</small></div><div className="commerce-preroll-v2-duration">{([6, 7, 8, 9, 10] as PrerollDuration[]).map(duration => <button className={state.duration === duration ? 'selected' : ''} key={duration} onClick={() => onDuration(duration)}>{duration}<small>秒</small></button>)}</div>
    <div className="commerce-preroll-v2-section-heading beats"><div><span className="commerce-preroll-v2-mono">EDITABLE STORYBOARD</span><h4>镜头故事板</h4></div><small>每段都会编译进最终 Prompt</small></div><div className="commerce-preroll-v2-beats editable">{beats.map((beat, index) => <article key={beat.id}><span>{String(index + 1).padStart(2, '0')}</span><time>{beat.timeLabel}</time><h5>{beat.label}</h5><label>画面<textarea value={beat.visualDescription ?? beat.detail} onChange={event => onStoryboard(index, 'visualDescription', event.target.value)}/></label><label>主体动作<input value={beat.subjectAction ?? ''} onChange={event => onStoryboard(index, 'subjectAction', event.target.value)}/></label><label>镜头<input value={beat.camera ?? ''} onChange={event => onStoryboard(index, 'camera', event.target.value)}/></label><label>场景与光线<input value={beat.sceneAndLighting ?? ''} onChange={event => onStoryboard(index, 'sceneAndLighting', event.target.value)}/></label><label>商品状态<textarea value={beat.productState ?? ''} onChange={event => onStoryboard(index, 'productState', event.target.value)}/></label><label>转入<input value={beat.transitionIn ?? ''} onChange={event => onStoryboard(index, 'transitionIn', event.target.value)}/></label><label>转出<input value={beat.transitionOut ?? ''} onChange={event => onStoryboard(index, 'transitionOut', event.target.value)}/></label><label>屏幕字幕<input value={beat.onScreenText ?? ''} placeholder="可留空" onChange={event => onStoryboard(index, 'onScreenText', event.target.value)}/></label><label>音频指令<input value={beat.audioInstruction ?? ''} onChange={event => onStoryboard(index, 'audioInstruction', event.target.value)}/></label></article>)}</div>
    <label className="commerce-preroll-v2-instruction"><span>补充要求（可选）</span><textarea placeholder="例如：整体更克制，不要出现手部特写。" value={state.extraInstruction} onChange={event => onInstruction(event.target.value)}/></label>
  </div><aside className="commerce-preroll-v2-basis-panel"><span className="commerce-preroll-v2-mono">SEEDANCE INPUT BASIS</span><h4>生成依据</h4>{['已确认的商品名称、品类与卖点', `${hook?.name ?? '当前钩子'}的动作与镜头规则`, `${state.duration} 秒三段节奏与商品定格要求`, state.analysis?.visualStyle ?? '原视频视觉风格', '商品外观、Logo 保真与禁止项'].map(item => <div key={item}><CheckCircle2 size={15}/><span>{item}</span></div>)}<details open><summary>完整创意 Prompt（可编辑）</summary><textarea className="commerce-preroll-v2-prompt-editor" value={state.generationDraft?.creativePrompt ?? state.generationDraft?.compiledPrompt ?? ''} onChange={event => onPrompt(event.target.value)}/><p>系统锁定约束：{state.generationDraft?.lockedConstraints?.join('；') || '商品保真、画幅和禁止项由服务端自动附加'}</p></details><small><LockKeyhole size={13}/>修改故事板会重新编译 Prompt；系统锁定约束不可删除。</small></aside></div>
    <footer><button className="commerce-preroll-v2-secondary" onClick={() => dispatch({ type: 'open-step', step: 'direction' })}><ArrowLeft size={15}/>上一步</button><button className="commerce-preroll-v2-primary" disabled={state.firstFramesStatus === 'loading'} onClick={onFrames}>{state.firstFramesStatus === 'loading' ? <LoaderCircle className="commerce-preroll-v2-spin" size={16}/> : <ImageIcon size={16}/>}生成参考首帧</button></footer>
  </section>
}

function FirstFrameStage({ state, onRegenerate, onSelect, onUpload, onPreview, dispatch }: { state: CommercePrerollState; onRegenerate: () => void; onSelect: (frame: FirstFrameCandidate) => void; onUpload: (file: File) => void; onPreview: (frame: FirstFrameCandidate) => void; dispatch: React.Dispatch<Parameters<typeof commercePrerollReducer>[1]> }) {
  return <section className="commerce-preroll-v2-stage commerce-preroll-v2-frame-stage"><div className="commerce-preroll-v2-stage-toolbar"><div><b>选择钩子首帧</b><small>三张图具有不同视觉强度，但共享同一商品保真规则。</small></div><button className="commerce-preroll-v2-secondary" disabled={state.firstFramesStatus === 'loading'} onClick={onRegenerate}><RefreshCw size={14}/>重新生成</button></div>
    {state.firstFramesStatus === 'loading' ? <div className="commerce-preroll-v2-frame-loading"><LoaderCircle className="commerce-preroll-v2-spin" size={28}/><b>正在生成 3 张参考首帧</b><small>正在结合商品参考图、钩子与光影风格</small></div> : <div className="commerce-preroll-v2-frame-grid">{state.firstFrames.map(frame => <article key={frame.id} className={state.selectedFirstFrameId === frame.id ? 'selected' : ''}><button className="commerce-preroll-v2-frame-preview" onClick={() => onPreview(frame)}><img src={frame.imageUrl} alt={frame.title}/><span><Maximize2 size={14}/>放大查看</span></button><div><span>{frame.label}</span><h4>{frame.title}</h4><p>{frame.description}</p><button className="commerce-preroll-v2-secondary" onClick={() => onSelect(frame)}>{state.selectedFirstFrameId === frame.id ? <Check size={14}/> : null}{state.selectedFirstFrameId === frame.id ? '已选择' : '选择此首帧'}</button></div></article>)}</div>}
    <div className="commerce-preroll-v2-frame-note"><Info size={15}/><span>所选首帧会与自动 Prompt、商品参考图和保真规则一并提交。</span><label className="commerce-preroll-v2-text-button"><Upload size={14}/>上传自定义首帧<input type="file" accept="image/*" onChange={event => { const file = event.target.files?.[0]; if (file) onUpload(file); event.target.value = '' }}/></label></div>
    <footer><button className="commerce-preroll-v2-secondary" onClick={() => dispatch({ type: 'open-step', step: 'settings' })}><ArrowLeft size={15}/>上一步</button><button className="commerce-preroll-v2-primary" disabled={!state.selectedFirstFrameId} onClick={() => dispatch({ type: 'open-step', step: 'video' })}>确认首帧<ArrowRight size={15}/></button></footer>
  </section>
}

function VideoStage({ state, onGenerate, onSave, onReset, dispatch }: { state: CommercePrerollState; onGenerate: () => void; onSave: () => void; onReset: () => void; dispatch: React.Dispatch<Parameters<typeof commercePrerollReducer>[1]> }) {
  const selectedHook = state.hooks.find(item => item.id === state.selectedHookId)
  const selectedFrame = state.firstFrames.find(item => item.id === state.selectedFirstFrameId)
  if (state.videoStatus === 'loading') return <section className="commerce-preroll-v2-stage commerce-preroll-v2-generating-stage"><div className="commerce-preroll-v2-generation-visual"><div className="commerce-preroll-v2-orbit"><WandSparkles size={28}/></div><span className="commerce-preroll-v2-mono">SEEDANCE VIDEO JOB</span><h4>正在生成独立前贴</h4><p>可以离开当前页面，任务会继续并在刷新后恢复。</p><ol>{videoStages.map((label, index) => { const number = index + 1; const done = state.videoStage >= number; const running = state.videoStage + 1 === number; return <li className={done ? 'done' : running ? 'running' : ''} key={label}><span>{done ? <Check size={13}/> : number}</span><b>{label}</b></li> })}</ol><small>任务 {state.clientTaskId.slice(0, 12)}</small></div></section>
  if (state.output) return <section className="commerce-preroll-v2-stage commerce-preroll-v2-result-stage"><div className="commerce-preroll-v2-result-grid"><div className="commerce-preroll-v2-media-canvas result"><video src={state.output.videoUrl} poster={state.output.posterUrl} controls autoPlay loop playsInline/></div><aside><div className="commerce-preroll-v2-success"><CheckCircle2 size={17}/><span><b>{state.savedAssetId ? '已保存到素材库' : '视频与创作记录已恢复'}</b><small>{state.savedAssetId || '当前结果可直接播放'}</small></span></div><span className="commerce-preroll-v2-mono">GENERATED PREROLL</span><h4>{selectedHook?.name ?? '电商前贴'} · 候选 01</h4><p>{selectedHook?.concept}</p><dl><div><dt>时长 / 画幅</dt><dd>{state.output.duration} 秒 · {state.output.aspectRatio}</dd></div><div><dt>来源原视频</dt><dd>{state.source?.name}</dd></div><div><dt>参考首帧</dt><dd>{selectedFrame?.title}</dd></div></dl><div className="commerce-preroll-v2-result-actions"><button className="commerce-preroll-v2-secondary" onClick={onGenerate}><RotateCcw size={15}/>再生成一版</button><button className="commerce-preroll-v2-primary" disabled={Boolean(state.savedAssetId)} onClick={onSave}><Save size={15}/>{state.savedAssetId ? '已保存' : '保存到素材库'}</button></div></aside></div><footer><button className="commerce-preroll-v2-secondary" onClick={onReset}><RefreshCw size={15}/>新建另一个前贴</button><small>本次生成：原视频理解 + 钩子规则 + 商品参考图 + 自动 Prompt</small></footer></section>
  return <section className="commerce-preroll-v2-stage commerce-preroll-v2-ready-stage"><div className="commerce-preroll-v2-reference-flow"><div><span>选中首帧</span>{selectedFrame ? <img src={selectedFrame.imageUrl} alt={selectedFrame.title}/> : null}</div><ArrowRight size={20}/><div className="commerce-preroll-v2-generation-method"><WandSparkles size={22}/><b>Prompt + 单张参考图</b><small>原视频仅提供理解结果，不会直接提交给生成模型。</small></div><div><b>{state.duration}s</b><small>独立前贴</small></div></div><div className="commerce-preroll-v2-ready-copy"><Sparkles size={22}/><div><b>生成参数已就绪</b><small>{selectedHook?.name} · {selectedFrame?.title} · {state.duration} 秒</small></div><button className="commerce-preroll-v2-primary" onClick={onGenerate}><WandSparkles size={16}/>生成前贴视频</button></div><footer><button className="commerce-preroll-v2-secondary" onClick={() => dispatch({ type: 'open-step', step: 'first-frame' })}><ArrowLeft size={15}/>返回首帧</button><small>结果将在当前页面直接播放，不需要跳去素材库查看。</small></footer></section>
}

export function CommercePrerollWorkspace({ onNotice, gateway: suppliedGateway }: { onNotice: (message: string) => void; gateway?: CommercePrerollGateway }) {
  const { currentProject } = useProject()
  const gateway = useMemo(() => suppliedGateway ?? new HttpCommercePrerollGateway(currentProject.id), [currentProject.id, suppliedGateway])
  const [state, dispatch] = useReducer(commercePrerollReducer, currentProject.id, initialState)
  const [previewFrame, setPreviewFrame] = useState<FirstFrameCandidate | null>(null)
	const [tasks, setTasks] = useState<CommercePrerollTaskSummary[]>([])
	const [versions, setVersions] = useState<CommercePrerollCreativeVersion[]>([])
	const [showSaveDialog, setShowSaveDialog] = useState(false)
	const [draftName, setDraftName] = useState('')
	const [saveDialogError, setSaveDialogError] = useState('')
  const objectUrls = useRef<string[]>([])
  const runningAnalysis = useRef(false)
  const runningVideo = useRef(false)
	const storyboardSaveTimer = useRef<number | null>(null)
	const promptSaveTimer = useRef<number | null>(null)

  const selectedHook = useMemo(() => state.hooks.find(item => item.id === state.selectedHookId) ?? null, [state.hooks, state.selectedHookId])
  const selectedFrame = useMemo(() => state.firstFrames.find(item => item.id === state.selectedFirstFrameId) ?? null, [state.firstFrames, state.selectedFirstFrameId])

	useEffect(() => {
		let active = true
		void (async () => {
			const items = await gateway.listTasks?.() ?? []
			if (!active) return
			setTasks(items)
			const requestedTask = new URL(window.location.href).searchParams.get('cpTask') ?? ''
			let restored: CommercePrerollState | null | undefined
			if (items.some(item => item.id === requestedTask)) restored = await gateway.openTask?.(requestedTask)
			else if (state.source && !gateway.currentTaskId?.()) {
				const latest = await gateway.openLatest?.()
				const latestSource = latest?.source
				if (latestSource?.assetId === state.source.assetId && latestSource?.assetVersion === state.source.assetVersion) restored = latest
			}
			if (active && restored) {
				dispatch({ type: 'restore', state: restored })
				void gateway.listVersions?.().then(values => { if (active) setVersions(values) })
			}
		})().catch(cause => { if (active) dispatch({ type: 'operation-failed', scope: 'analysis', message: cause instanceof Error ? cause.message : '任务恢复失败' }) })
		return () => { active = false }
	// Only hydrate when the Project/Gateway changes; local state is an input snapshot.
	// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [gateway])

	const refreshVersions = () => { void gateway.listVersions?.().then(setVersions) }
	const switchTask = async (taskId: string) => { const restored = await gateway.openTask?.(taskId); if (restored) { dispatch({ type: 'restore', state: restored }); refreshVersions() } }
	const beginNew = () => {
		if (state.source || state.productDraft || state.generationDraft || state.output) {
			setDraftName(state.productDraft?.name ? `${state.productDraft.name} · ${selectedHook?.name ?? '电商前贴'}` : '未完成电商前贴')
			setSaveDialogError('')
			setShowSaveDialog(true)
			return
		}
		gateway.startNew?.(); dispatch({ type: 'reset' })
	}
	const saveAndCreate = async () => {
		if (!draftName.trim() || !state.source) return
		try {
			setSaveDialogError('')
			if (!state.rightsConfirmed) throw new Error('保存前需要确认原视频权利状态')
			await gateway.ensureTask?.(state.source)
			await gateway.saveVersion?.(draftName.trim())
			setTasks(await gateway.listTasks?.() ?? [])
			setShowSaveDialog(false); gateway.startNew?.(); clearCommercePrerollSession(currentProject.id); dispatch({ type: 'reset' })
		} catch (cause) {
			setSaveDialogError(cause instanceof Error ? cause.message : '保存当前任务失败')
		}
	}

  useEffect(() => {
    writeCommercePrerollSession(currentProject.id, state)
    const url = new URL(window.location.href)
    url.searchParams.set('cpStep', state.activeStep)
    url.searchParams.set('cpTask', state.clientTaskId)
    window.history.replaceState(window.history.state, '', url)
  }, [currentProject.id, state])

  useEffect(() => () => { objectUrls.current.forEach(url => URL.revokeObjectURL(url)) }, [])

  const runAnalysis = async () => {
    if (!state.source || runningAnalysis.current) return
    runningAnalysis.current = true
    dispatch({ type: 'analysis-started' })
    try {
      const result = await gateway.analyzeSource(state.source, stage => dispatch({ type: 'analysis-progress', stage }))
      dispatch({ type: 'analysis-ready', ...result })
	  setTasks(await gateway.listTasks?.() ?? [])
	  refreshVersions()
    } catch (cause) {
      dispatch({ type: 'operation-failed', scope: 'analysis', message: cause instanceof Error ? cause.message : '原视频解析失败，请重试。' })
    } finally { runningAnalysis.current = false }
  }

  const confirmUnderstanding = async () => {
    if (!state.productDraft || !state.source) return
    dispatch({ type: 'hooks-started' })
    try {
      const hooks = await gateway.compileHookProposals(state.productDraft, state.source)
      dispatch({ type: 'hooks-ready', hooks })
    } catch (cause) {
      dispatch({ type: 'operation-failed', scope: 'hooks', message: cause instanceof Error ? cause.message : '钩子方案生成失败，请重试。' })
    }
  }

  const compileDraft = async (hook = selectedHook, duration = state.duration, instruction = state.extraInstruction) => {
    if (!hook || !state.productDraft) return null
    const draft = await gateway.compileGenerationDraft({ product: state.productDraft, hook, duration, extraInstruction: instruction })
    dispatch({ type: 'draft-ready', draft })
    return draft
  }

  const selectHook = async (hook: HookProposal) => {
    dispatch({ type: 'hook-selected', id: hook.id })
    await compileDraft(hook)
  }

  const changeDuration = async (duration: PrerollDuration) => {
	if (state.generationDraft?.editMode === 'manual_creative_override' && !window.confirm('修改时长会重新排布三段镜头并覆盖已手工编辑的创意 Prompt，是否继续？')) return
    dispatch({ type: 'duration-changed', duration })
    await compileDraft(selectedHook, duration)
  }

	const updateStoryboard = (index: number, field: EditableBeatField, value: string) => {
		if (!state.generationDraft) return
		if (state.generationDraft.editMode === 'manual_creative_override' && !window.confirm('修改故事板会重新编译并覆盖已手工编辑的创意 Prompt，是否继续？')) return
		const beats = state.generationDraft.beats.map((beat, beatIndex) => beatIndex === index ? { ...beat, [field]: value } : beat)
		dispatch({ type: 'draft-ready', draft: { ...state.generationDraft, beats, editMode: 'storyboard_compiled' } })
		if (storyboardSaveTimer.current) window.clearTimeout(storyboardSaveTimer.current)
		storyboardSaveTimer.current = window.setTimeout(() => { void gateway.updateStoryboard?.(beats).then(draft => dispatch({ type: 'draft-ready', draft })).catch(cause => dispatch({ type: 'operation-failed', scope: 'frames', message: cause instanceof Error ? cause.message : '故事板保存失败' })) }, 700)
	}

	const updatePrompt = (value: string) => {
		if (!state.generationDraft) return
		dispatch({ type: 'draft-ready', draft: { ...state.generationDraft, creativePrompt: value, editMode: 'manual_creative_override' } })
		if (promptSaveTimer.current) window.clearTimeout(promptSaveTimer.current)
		promptSaveTimer.current = window.setTimeout(() => { void gateway.updatePrompt?.(value).then(draft => dispatch({ type: 'draft-ready', draft })).catch(cause => dispatch({ type: 'operation-failed', scope: 'frames', message: cause instanceof Error ? cause.message : 'Prompt 保存失败' })) }, 700)
	}

  const generateFrames = async () => {
    if (!state.productReference || !selectedHook) return
    dispatch({ type: 'frames-started' })
    try {
	  if (storyboardSaveTimer.current) window.clearTimeout(storyboardSaveTimer.current)
	  if (promptSaveTimer.current) window.clearTimeout(promptSaveTimer.current)
	  let draft = state.generationDraft ?? await compileDraft()
	  if (draft && gateway.updateStoryboard) draft = await gateway.updateStoryboard(draft.beats)
	  if (draft?.creativePrompt && gateway.updatePrompt) draft = await gateway.updatePrompt(draft.creativePrompt)
      if (!draft) throw new Error('生成依据尚未准备完成。')
      const frames = await gateway.generateFirstFrames(draft, state.productReference, () => undefined)
      dispatch({ type: 'frames-ready', frames })
    } catch (cause) {
      dispatch({ type: 'operation-failed', scope: 'frames', message: cause instanceof Error ? cause.message : '参考首帧生成失败，请重试。' })
    }
  }

  const generateVideo = async () => {
    if (!state.generationDraft || !selectedFrame || runningVideo.current) return
    runningVideo.current = true
    dispatch({ type: 'video-started' })
    try {
      const output = await gateway.createVideo({ draft: state.generationDraft, frame: selectedFrame, duration: state.duration }, stage => dispatch({ type: 'video-progress', stage }))
      dispatch({ type: 'video-ready', output })
      onNotice('电商前贴已生成，可在当前页面播放并保存到素材库。')
    } catch (cause) {
      dispatch({ type: 'operation-failed', scope: 'video', message: cause instanceof Error ? cause.message : '视频生成失败，请重试。' })
    } finally { runningVideo.current = false }
  }

  const saveOutput = async () => {
    if (!state.output) return
    try {
      const result = await gateway.saveOutputToLibrary(state.output)
      dispatch({ type: 'output-saved', assetId: result.assetId })
      onNotice('前贴视频已保存到当前 Project 素材库。')
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '保存到素材库失败。') }
  }

  const selectSourceFile = async (file: File) => {
    const source = sourceFromUpload(file)
    objectUrls.current.push(source.videoUrl)
    dispatch({ type: 'source-selected', source })
    try {
      const uploaded = gateway.uploadSource ? await gateway.uploadSource(file, source) : source
      dispatch({ type: 'source-selected', source: uploaded })
    } catch (cause) {
      dispatch({ type: 'operation-failed', scope: 'analysis', message: cause instanceof Error ? cause.message : '原视频上传失败，请重试。' })
    }
  }

  const selectReferenceFile = async (file: File) => {
    const imageUrl = URL.createObjectURL(file)
    objectUrls.current.push(imageUrl)
    const localReference: ProductReference = { id: `uploaded-reference-${Date.now()}`, imageUrl, label: file.name, sourceLabel: '用户上传商品图', kind: 'uploaded' }
    dispatch({ type: 'reference-changed', reference: localReference })
    try {
      const reference = gateway.uploadProductReference ? await gateway.uploadProductReference(file) : localReference
      dispatch({ type: 'reference-changed', reference })
    } catch (cause) {
      dispatch({ type: 'operation-failed', scope: 'frames', message: cause instanceof Error ? cause.message : '商品参考图上传失败，请重试。' })
    }
  }

	const selectProductReference = async (reference: ProductReference) => {
		try {
			await gateway.selectProductReference?.(reference)
			dispatch({ type: 'reference-changed', reference })
		} catch (cause) { dispatch({ type: 'operation-failed', scope: 'frames', message: cause instanceof Error ? cause.message : '商品参考图切换失败' }) }
	}

	const reextractProductReferences = async () => {
		try {
			const references = await gateway.reextractProductReferences?.()
			if (references?.[0]) dispatch({ type: 'analysis-ready', analysis: state.analysis!, product: state.productDraft!, reference: references[0], references, risks: state.riskFacts })
		} catch (cause) { dispatch({ type: 'operation-failed', scope: 'analysis', message: cause instanceof Error ? cause.message : '商品图重新提取失败' }) }
	}

  const uploadFirstFrame = async (file: File) => {
    const imageUrl = URL.createObjectURL(file)
    objectUrls.current.push(imageUrl)
    const localFrame: FirstFrameCandidate = { id: `uploaded-frame-${Date.now()}`, imageUrl, label: '自定义首帧', title: file.name, description: '用户上传并确认的视觉参考。' }
    try {
      const frame = gateway.uploadCustomFirstFrame ? await gateway.uploadCustomFirstFrame(file) : localFrame
      dispatch({ type: 'frames-ready', frames: [...state.firstFrames, frame] })
      dispatch({ type: 'frame-selected', id: frame.id })
    } catch (cause) {
      dispatch({ type: 'operation-failed', scope: 'frames', message: cause instanceof Error ? cause.message : '自定义首帧上传失败，请重试。' })
    }
  }

  const reset = () => {
    clearCommercePrerollSession(currentProject.id)
    dispatch({ type: 'reset' })
  }

  useEffect(() => {
    if (state.analysisStatus === 'loading' && state.source && !runningAnalysis.current) void runAnalysis()
  // Only resume an interrupted local task when its durable identity changes.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.analysisStatus, state.source?.id, state.clientTaskId])

  useEffect(() => {
    if (state.videoStatus === 'loading' && state.generationDraft && selectedFrame && !runningVideo.current) void generateVideo()
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.videoStatus, state.selectedFirstFrameId, state.clientTaskId])

  const retryFailedOperation = () => {
    const operation = commercePrerollRetryOperation(state.activeStep, state.errorScope)
    if (operation === 'analysis') void runAnalysis()
    else if (operation === 'hooks') void confirmUnderstanding()
    else if (operation === 'frames') void generateFrames()
    else if (operation === 'video') void generateVideo()
  }

  return <><div className="commerce-preroll-v2-taskbar"><label>电商前贴任务<select value={gateway.currentTaskId?.() ?? ''} onChange={event => void switchTask(event.target.value)}><option value="">当前草稿</option>{tasks.map(task => <option key={task.id} value={task.id}>{task.displayName} · {task.status}</option>)}</select></label><label>方案版本<select defaultValue="" onChange={event => { const id = event.target.value; if (id) void gateway.restoreVersion?.(id).then(() => switchTask(gateway.currentTaskId?.() ?? '')) }}><option value="">当前工作草稿</option>{versions.map(version => <option key={version.id} value={version.id}>V{version.version} · 草稿修订 {version.draftRevision}</option>)}</select></label><button className="commerce-preroll-v2-primary" onClick={beginNew}>新建电商前贴</button></div><section className="commerce-preroll-v2" aria-label="电商前贴创作工作区">
    <StepRail state={state} dispatch={dispatch}/>
    <main className="commerce-preroll-v2-main"><StageHeader state={state}/>{state.error ? <div className="commerce-preroll-v2-error" role="alert"><CircleAlert size={16}/><span>{state.error}</span><button onClick={retryFailedOperation}>重试</button></div> : null}
      {state.activeStep === 'source' ? <SourceStage state={state} dispatch={dispatch} onAnalyze={() => void runAnalysis()} onSourceFile={selectSourceFile}/>
        : state.activeStep === 'understanding' && state.analysisStatus === 'loading' ? <AnalysisProgress state={state}/>
        : state.activeStep === 'understanding' ? <UnderstandingReady state={state} dispatch={dispatch} onConfirm={() => void confirmUnderstanding()} onReferenceFile={selectReferenceFile} onReferenceSelect={reference => void selectProductReference(reference)} onReextract={() => void reextractProductReferences()}/>
        : state.activeStep === 'direction' ? <DirectionStage state={state} dispatch={dispatch} onSelectHook={hook => void selectHook(hook)} onRefresh={() => void confirmUnderstanding()} onContinue={() => dispatch({ type: 'open-step', step: 'settings' })}/>
        : state.activeStep === 'settings' ? <SettingsStage state={state} dispatch={dispatch} onDuration={duration => void changeDuration(duration)} onInstruction={value => dispatch({ type: 'instruction-changed', value })} onStoryboard={updateStoryboard} onPrompt={updatePrompt} onFrames={() => void generateFrames()}/>
        : state.activeStep === 'first-frame' ? <FirstFrameStage state={state} dispatch={dispatch} onRegenerate={() => void generateFrames()} onSelect={frame => dispatch({ type: 'frame-selected', id: frame.id })} onUpload={uploadFirstFrame} onPreview={setPreviewFrame}/>
        : <VideoStage state={state} dispatch={dispatch} onGenerate={() => void generateVideo()} onSave={() => void saveOutput()} onReset={reset}/>} </main>
    {previewFrame ? <div className="commerce-preroll-v2-lightbox" role="dialog" aria-modal="true" aria-label={`${previewFrame.title}大图预览`}><div><button aria-label="关闭预览" onClick={() => setPreviewFrame(null)}><X size={18}/></button><img src={previewFrame.imageUrl} alt={previewFrame.title}/><footer><b>{previewFrame.title}</b><span>{previewFrame.description}</span></footer></div></div> : null}
  </section>{showSaveDialog ? <div className="commerce-preroll-v2-save-dialog" role="dialog" aria-modal="true" aria-label="保存当前电商前贴"><div><h3>先保存当前电商前贴</h3><p>当前流程尚未完成。命名保存后，可从任务和版本下拉框继续找回。</p><label>任务名称<input value={draftName} autoFocus onChange={event => { setDraftName(event.target.value); setSaveDialogError('') }}/></label>{saveDialogError ? <div className="commerce-preroll-v2-dialog-error" role="alert"><CircleAlert size={15}/><span>{saveDialogError}</span></div> : null}<footer><button className="commerce-preroll-v2-secondary" onClick={() => setShowSaveDialog(false)}>继续编辑</button><button className="commerce-preroll-v2-primary" disabled={!draftName.trim()} onClick={() => void saveAndCreate()}>保存并新建</button></footer></div></div> : null}</>
}
