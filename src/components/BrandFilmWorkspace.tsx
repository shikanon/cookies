import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowRight, Check, FileText, Film, Image, LoaderCircle, Lock, Plus, RefreshCw, Sparkles, Trash2, Upload, Volume2, VolumeX, WandSparkles } from 'lucide-react'
import { useProject } from '../context/ProjectContext'
import { api, type ApiAssetVersionRef, type ApiBrandAudioMixOperation, type ApiBrandAudioWorkspace, type ApiBrandBriefAnalysis, type ApiBrandBriefAssetCandidate, type ApiBrandCreativeConcept, type ApiBrandFilmGenerationAttempt, type ApiBrandFilmPlan, type ApiBrandFilmWorkspace, type ApiCreativeTaskSummary, type ApiSpeechCapability } from '../data/api'
import { BrandFilmWorkbenchShell } from '../features/brand-film/BrandFilmWorkbenchShell'
import { briefProductNames, extractAndUploadBrandBriefAssets } from '../features/brand-film/pdfBriefAssets'
import { recoverCompletedBrandFilmMutation } from '../features/brand-film/revisionRetry'
import { deriveBrandFilmStages, resolveBrandFilmStage } from '../features/brand-film/stage'
import { useBrandFilmStageRoute } from '../features/brand-film/useBrandFilmStageRoute'

type Props = {
  taskId?: string
  taskOptions: ApiCreativeTaskSummary[]
  onOpenTask: (taskId: string) => void
  onCreateNew: () => void
  onNotice: (message: string) => void
}
const last = <T,>(items?: T[] | null) => items?.at(-1)
const compactLines = (items: string[]) => items.map(item => item.trim()).filter(Boolean)
const editableBriefPayload = (brief: ApiBrandBriefAnalysis): ApiBrandBriefAnalysis => ({
  ...brief,
  selling_points: brief.selling_points.map(point => ({ ...point, text: point.text.trim() })).filter(point => Boolean(point.text)),
  mandatory_elements: compactLines(brief.mandatory_elements),
  prohibited_claims: compactLines(brief.prohibited_claims),
  image_requirements: compactLines(brief.image_requirements),
  video_requirements: compactLines(brief.video_requirements),
  uncertainties: compactLines(brief.uncertainties),
})
const isProductReference = (asset: Pick<ApiBrandBriefAssetCandidate, 'role'>) => asset.role === 'product_front' || asset.role.startsWith('product_')
const sameAssetRef = (left: ApiAssetVersionRef | null | undefined, right: ApiAssetVersionRef | null | undefined) => Boolean(left && right && left.asset_id === right.asset_id && left.version === right.version)
const savedProjectName = (displayName: string | undefined, fallback: string) => displayName && !displayName.startsWith('未命名') ? displayName : fallback
const fixtureReferenceUri = (sourceLocator: string, role: string, uri = '') => {
  if (uri && uri !== '/assets/guerlain-25x-bee-water.png') return uri
  if (!sourceLocator.startsWith('fixture://')) return ''
  return role === 'product_front' ? '/assets/guerlain-25x-bee-water-product-front.png' : role === 'logo' ? '/assets/guerlain-logo.png' : uri
}
const briefAssetSource = (sourceLocator: string, role: string) => {
  const match = sourceLocator.match(/page=(\d+)&image=([^&]+)/)
  if (match) return `Brief PDF 第 ${match[1]} 页 · 内嵌图片 ${match[2]}`
  if (sourceLocator.startsWith('fixture://') && role === 'product_front') return 'Brief PDF 第 9 页 · 内嵌图片 IM135'
  if (sourceLocator.startsWith('fixture://') && role === 'logo') return 'Brief PDF 第 1 页 · 内嵌图片 IM17'
  return sourceLocator
}

function ModelBadge({ alias, version }: { alias?: string; version?: string }) {
  if (!alias) return null
  const fallback = alias === 'fixture.deterministic'
  return <span className={fallback ? 'brand-model-badge fallback' : 'brand-model-badge'}>{fallback ? '固定样例回退' : alias}{version ? ` · ${version}` : ''}</span>
}

function ConceptCard({ concept, editing, busy, onChange, onSelect }: {
  concept: ApiBrandCreativeConcept
  editing: boolean
  busy: boolean
  onChange: (changes: Partial<ApiBrandCreativeConcept>) => void
  onSelect: () => void
}) {
  if (editing) return <article className="brand-concept-editor">
    <span>{concept.id}</span>
    <small>仅修改方向级表达；画面、运镜、旁白等执行细节在剧本分镜阶段编辑。</small>
    <label>方向标题<input value={concept.title} onChange={event => onChange({ title: event.target.value })}/></label>
    <label>核心创意句<textarea value={concept.one_liner} onChange={event => onChange({ one_liner: event.target.value })}/></label>
    <label>叙事机制<textarea value={concept.story_mechanism} onChange={event => onChange({ story_mechanism: event.target.value })}/></label>
  </article>
  return <article className={concept.selected ? 'selected' : ''}><span>{concept.id}</span><h4>{concept.title}</h4><b>{concept.one_liner}</b><p>{concept.story_mechanism}</p><dl><div><dt>品牌进入</dt><dd>{concept.brand_entrance}</dd></div><div><dt>视觉语言</dt><dd>{concept.visual_language.join('、')}</dd></div><div><dt>声音</dt><dd>{concept.sound_idea}</dd></div><div><dt>Brief 依据</dt><dd>{concept.brief_rationale}</dd></div><div><dt>风险</dt><dd>{concept.risk}</dd></div></dl><button className={concept.selected ? 'secondary-button' : 'primary-button'} disabled={busy || concept.selected} onClick={onSelect}>{concept.selected ? '已选择' : '选择此方向'}</button></article>
}

function ShotEditor({ shot, disabled, onChange }: { shot: ApiBrandFilmPlan['shots'][number]; disabled: boolean; onChange: (changes: Partial<ApiBrandFilmPlan['shots'][number]>) => void }) {
  return <article className="brand-shot-editor"><header><div><b>镜头 {String(shot.order).padStart(2, '0')}</b><span>{shot.start_second}s–{shot.end_second}s</span></div><small>{shot.purpose}</small></header><div className="brand-shot-editor-layout"><div className="brand-shot-canvas-fields"><label>画面<textarea disabled={disabled} value={shot.visual} onChange={event => onChange({ visual: event.target.value })}/></label><label>动作<textarea disabled={disabled} value={shot.action} onChange={event => onChange({ action: event.target.value })}/></label><label>旁白<textarea disabled={disabled} value={shot.voiceover} onChange={event => onChange({ voiceover: event.target.value })}/></label><label>屏幕字<textarea disabled={disabled} value={shot.on_screen_text} onChange={event => onChange({ on_screen_text: event.target.value })}/></label></div><aside className="brand-shot-inspector" aria-label={`镜头 ${shot.order} 属性`}><h4>镜头属性</h4><label>镜头目的<input disabled={disabled} value={shot.purpose} onChange={event => onChange({ purpose: event.target.value })}/></label><label>参考角色<input disabled={disabled} value={shot.reference_role} onChange={event => onChange({ reference_role: event.target.value })}/></label><label>运镜<textarea disabled={disabled} value={shot.camera} onChange={event => onChange({ camera: event.target.value })}/></label><label>光线<textarea disabled={disabled} value={shot.lighting} onChange={event => onChange({ lighting: event.target.value })}/></label><label>连贯性<textarea disabled={disabled} value={shot.continuity_notes} onChange={event => onChange({ continuity_notes: event.target.value })}/></label></aside></div></article>
}

function StoryboardEditor({ plan, disabled, onChange }: { plan: ApiBrandFilmPlan; disabled: boolean; onChange: (plan: ApiBrandFilmPlan) => void }) {
  const [selectedShotId, setSelectedShotId] = useState(() => plan.shots[0]?.id ?? '')
  const selectedShotIndex = Math.max(0, plan.shots.findIndex(shot => shot.id === selectedShotId))
  const selectedShot = plan.shots[selectedShotIndex]

  useEffect(() => {
    if (plan.shots.some(shot => shot.id === selectedShotId)) return
    setSelectedShotId(plan.shots[0]?.id ?? '')
  }, [plan.shots, selectedShotId])

  if (!selectedShot) return <div className="brand-film-empty compact"><p>当前剧本尚未包含镜头。</p></div>

  return <div className="brand-storyboard-editor">
    <nav className="brand-shot-rail" aria-label="镜头列表">{plan.shots.map(shot => <button key={shot.id} type="button" aria-pressed={shot.id === selectedShot.id} onClick={() => setSelectedShotId(shot.id)}><span>{String(shot.order).padStart(2, '0')}</span><div><b>{shot.purpose}</b><small>{shot.start_second}s–{shot.end_second}s · {shot.reference_role}</small></div></button>)}</nav>
    <div className="brand-shot-focus"><ShotEditor shot={selectedShot} disabled={disabled} onChange={changes => onChange({ ...plan, shots: plan.shots.map((shot, index) => index === selectedShotIndex ? { ...shot, ...changes } : shot) })}/></div>
    <div className="brand-shot-timeline" aria-label="镜头时间线"><span>00:00</span><div>{plan.shots.map(shot => <button key={shot.id} type="button" className={shot.id === selectedShot.id ? 'active' : ''} style={{ flexGrow: Math.max(1, shot.end_second - shot.start_second) }} onClick={() => setSelectedShotId(shot.id)}><b>{String(shot.order).padStart(2, '0')}</b><small>{shot.start_second}s–{shot.end_second}s</small></button>)}</div><span>{plan.shots.at(-1)?.end_second ?? 0}s</span></div>
  </div>
}

function GenerationUnitActions({ attempt, busy, feedback, onFeedback, onGenerate, onLock }: {
  attempt?: ApiBrandFilmGenerationAttempt
  busy: boolean
  feedback: string
  onFeedback: (value: string) => void
  onGenerate: (feedback?: string) => void
  onLock: (attemptId: string) => void
}) {
  if (!attempt) return <button className="primary-button" disabled={busy} onClick={() => onGenerate()}>生成此片段</button>
  if (attempt.status === 'queued' || attempt.status === 'running') {
    return <div className="brand-unit-progress"><LoaderCircle className="spin" size={14}/><span>{attempt.status === 'queued' ? '已进入生成队列，请稍候…' : '正在生成视频，请稍候…'}</span></div>
  }
  if (attempt.status === 'succeeded' && attempt.output_asset_ref) {
    return <><textarea placeholder="对当前视频填写局部反馈，例如：稳定瓶身标签，减少镜头环绕" value={feedback} onChange={event => onFeedback(event.target.value)}/><button className="secondary-button" disabled={busy || !feedback.trim()} onClick={() => onGenerate(feedback)}><RefreshCw size={13}/>按反馈重生成</button><button className="primary-button" disabled={busy} onClick={() => onLock(attempt.id)}><Lock size={13}/>锁定此片段</button></>
  }
  return <><div className="brand-unit-error"><b>本次生成未成功</b><span>{attempt.error_message || `状态：${attempt.status}`}</span></div><button className="primary-button" disabled={busy} onClick={() => onGenerate()}>重新生成此片段</button></>
}

function activeAudioMix(audio: ApiBrandAudioWorkspace) {
  const variant = audio.variants.find(item => item.id === audio.active_variant_id)
  return variant?.mix_versions.find(item => item.revision === audio.active_mix_revision)
}

function brandAudioMaterialized(audio: ApiBrandAudioWorkspace) {
  const mix = activeAudioMix(audio)
  return Boolean(mix && mix.tracks.flatMap(track => track.clips).every(clip => Boolean(clip.asset_ref)))
}

function brandAudioProgress(audio: ApiBrandAudioWorkspace) {
  if (audio.mixed_preview_asset_ref || audio.status === 'preview_ready') return { title: 'Audio A2 混音预览已就绪', next: '可继续微调音轨或切换 A/B 声音方案', detail: '当前完整视频已经包含旁白、音乐与音效；修改声音不会重新生成画面。' }
  if (audio.status === 'preview_queued' || audio.status === 'preview_rendering') return { title: 'Audio A2 正在混音', next: 'FFmpeg 正在合成完整视频', detail: '可以离开页面，任务状态和结果都会持久化。' }
  if (audio.status === 'preview_failed') return { title: 'Audio A2 混音需要重试', next: '修复音轨后重新生成混音预览', detail: '音频资产仍然保留，不需要重新生成画面或重新入库。' }
  if (brandAudioMaterialized(audio)) return { title: 'Audio A1 项目资产已就绪', next: '下一步：A2 FFmpeg 混音预览', detail: '旁白、音乐与音效均可试听和替换；当前 Fixture 会被明确标记。' }
  return { title: 'Audio A0 草稿已保存', next: '下一步：将 Fixture 物化为项目音频资产', detail: '结构已持久化，尚未产生可试听音频。' }
}

function AudioWorkspaceEditor({ audio, videoURL, mixedVideoURL, assetURLs, speechCapability, busy, onMaterialize, onReplace, onSave, onRender, onProbeSpeech, onGenerateVoice, onSelectVariant, onPlanDirector }: {
  audio: ApiBrandAudioWorkspace
  videoURL?: string
  mixedVideoURL?: string
  assetURLs: Record<string, string>
  speechCapability?: ApiSpeechCapability
  busy: boolean
  onMaterialize: () => void
  onReplace: (clipId: string, file?: File) => void
  onSave: (operations: ApiBrandAudioMixOperation[]) => void
  onRender: () => void
  onProbeSpeech: () => void
  onGenerateVoice: (clipId: string, voiceAlias: string) => void
  onSelectVariant: (variantId: string) => void
  onPlanDirector: () => void
}) {
  const mix = activeAudioMix(audio)
  const [trackDrafts, setTrackDrafts] = useState(() => Object.fromEntries((mix?.tracks ?? []).map(track => [track.id, { gain: track.gain_db, muted: track.muted }])))
  const [timingDrafts, setTimingDrafts] = useState(() => Object.fromEntries((mix?.tracks.find(track => track.type === 'voiceover')?.clips ?? []).map(clip => [clip.id, { start: clip.timeline_start_ms, end: clip.timeline_end_ms }])))
  const blueprint = audio.blueprint_versions.at(-1)
  const defaultVoiceAlias = audio.blueprint_versions.at(-1)?.voice_profile.voice_alias ?? 'cookies.voice.brand.warm_female'
  const [voiceAlias, setVoiceAlias] = useState(defaultVoiceAlias)
  if (!mix) return <p className="brand-locked">当前音轨修订不可用，请重新准备音轨草稿。</p>
  const materialized = brandAudioMaterialized(audio)
  const operations: ApiBrandAudioMixOperation[] = []
  for (const track of mix.tracks) {
    const draft = trackDrafts[track.id]
    if (!draft) continue
    if (draft.gain !== track.gain_db) operations.push({ op: 'set_track_gain', track_id: track.id, gain_db: draft.gain })
    if (draft.muted !== track.muted) operations.push({ op: 'set_track_muted', track_id: track.id, muted: draft.muted })
  }
  for (const clip of mix.tracks.find(track => track.type === 'voiceover')?.clips ?? []) {
    const timing = timingDrafts[clip.id]
    if (timing && (timing.start !== clip.timeline_start_ms || timing.end !== clip.timeline_end_ms)) operations.push({ op: 'set_clip_timing', clip_id: clip.id, timeline_start_ms: timing.start, timeline_end_ms: timing.end })
  }
  return <div className="brand-audio-workspace">
    {blueprint?.planner_version === 'brand-audio-director/v1' ? <section className="brand-audio-director"><header><div><span className="section-label">AUDIO A4 · AI 声音导演</span><h4>声画策略已经排好，每项决定都可以理解和调整</h4></div><div className="brand-audio-variants">{audio.variants.map(variant => <button key={variant.id} className={variant.id === audio.active_variant_id ? 'active' : ''} disabled={busy || variant.id === audio.active_variant_id} onClick={() => onSelectVariant(variant.id)}>{variant.label}<small>{variant.style_preset}</small></button>)}</div></header><div className="brand-director-grid"><article><h5>品牌发音词典</h5><div className="brand-pronunciations">{(blueprint.pronunciations ?? []).map(item => <span key={item.term}><b>{item.term}</b><em>{item.spoken_as}</em><small>{item.reason}</small></span>)}</div></article><article><h5>声画语义检查</h5>{(blueprint.semantic_checks ?? []).map(item => <div className={`brand-semantic-check ${item.status}`} key={item.id}><b>{item.status === 'pass' ? '通过' : '建议修复'} · {item.shot_id}</b><span>{item.summary}</span><small>{item.suggestion || item.evidence}</small></div>)}</article></div><div className="brand-narration-fit">{blueprint.narration_cues.map(cue => { const clip = mix.tracks.find(track => track.type === 'voiceover')?.clips.find(item => item.narration_source_ref?.shot_id === cue.shot_id); if (!clip) return null; const timing = timingDrafts[clip.id] ?? { start: clip.timeline_start_ms, end: clip.timeline_end_ms }; return <article className={cue.fit_status} key={cue.id}><header><b>{cue.shot_id} · {cue.fit_status === 'overrun' ? '预计超时' : cue.fit_status === 'spacious' ? '空间充足' : '时长合适'}</b><span>预计 {(cue.estimated_duration_ms / 1000).toFixed(1)}s / 可用 {(cue.available_duration_ms / 1000).toFixed(1)}s</span></header><p>{cue.text}</p>{cue.suggested_text ? <small>精简建议：{cue.suggested_text}</small> : null}<div><label>开始 ms<input type="number" min={0} max={audio.master_duration_ms - 1} value={timing.start} onChange={event => setTimingDrafts(value => ({ ...value, [clip.id]: { ...timing, start: Number(event.target.value) } }))}/></label><label>结束 ms<input type="number" min={1} max={audio.master_duration_ms} value={timing.end} onChange={event => setTimingDrafts(value => ({ ...value, [clip.id]: { ...timing, end: Number(event.target.value) } }))}/></label></div></article>})}</div><details><summary>查看 {(blueprint.director_decisions ?? []).length} 项自动决定</summary><div className="brand-director-decisions">{(blueprint.director_decisions ?? []).map(item => <article key={item.id}><b>{item.summary}</b><span>{item.reason}</span><small>置信度 {Math.round(item.confidence * 100)}% · {item.editable ? '可通过音轨参数修改' : '只读'}</small></article>)}</div></details></section> : <section className="brand-audio-director legacy"><div><span className="section-label">AUDIO A4 · UPGRADE</span><h4>当前是早期音轨草稿，可保留素材并升级为 AI 声音导演方案。</h4></div><button className="primary-button" disabled={busy} onClick={onPlanDirector}>升级声音导演</button></section>}
    <div className={speechCapability?.available ? 'brand-speech-capability ready' : 'brand-speech-capability'}><div><span className="section-label">AUDIO A3 · MINIMAX TTS</span><b>{speechCapability?.available ? `真实语音可用 · ${speechCapability.model}` : speechCapability ? 'MiniMax 不可用，继续使用 Fixture' : '尚未检测 MiniMax 语音能力'}</b><small>{speechCapability?.error_message || '能力检测会真实生成一句极短测试音频，不会暴露 API Key。'}</small></div>{speechCapability?.available ? <label>逻辑音色<select value={voiceAlias} onChange={event => setVoiceAlias(event.target.value)}>{speechCapability.voice_aliases.map(alias => <option key={alias} value={alias}>{alias}</option>)}</select></label> : <button className="secondary-button" disabled={busy} onClick={onProbeSpeech}>检测 MiniMax TTS</button>}</div>
    {speechCapability?.available ? <div className="brand-voice-clips">{mix.tracks.find(track => track.type === 'voiceover')?.clips.map(clip => { const attempt = audio.generation_attempts.filter(item => item.clip_id === clip.id).at(-1); return <article key={clip.id}><div><b>{clip.label}</b><small>{attempt?.status === 'succeeded' && !attempt.fixture_mode ? `MiniMax · ${attempt.provider_snapshot}` : attempt?.status === 'failed' ? `生成失败：${attempt.error_code} · 当前保留 Fixture` : '当前为 Fixture 旁白'}</small></div><button className="secondary-button" disabled={busy} onClick={() => onGenerateVoice(clip.id, voiceAlias)}>{attempt?.status === 'succeeded' && !attempt.fixture_mode ? '重新生成此句' : '生成真实旁白'}</button></article> })}</div> : null}
    <div className="brand-audio-preview">{mixedVideoURL ? <video controls src={mixedVideoURL}/> : videoURL ? <video controls muted src={videoURL}/> : <div><Film size={24}/><span>视觉预览正在恢复</span></div>}<aside><span className="section-label">{mixedVideoURL ? 'AUDIO A2 · MIXED PREVIEW' : materialized ? 'AUDIO A1 · ASSET READY' : 'AUDIO A0 · BLUEPRINT'}</span><b>{audio.variants.find(item => item.id === audio.active_variant_id)?.label ?? '默认声音方案'}</b><small>{audio.master_duration_ms / 1000} 秒 · Mix r{mix.revision}</small><p>{mixedVideoURL ? '当前播放的是画面与旁白、音乐、音效合成后的完整预览。修改任何轨道后可重新渲染，不会重生成画面。' : materialized ? '音频已经项目级入库，可逐段试听和替换；点击生成混音预览后会自动压低旁白下方的音乐并统一响度。' : '轨道结构已经排好。将 Fixture 物化为 WAV 项目资产后即可试听，并为后续真实 TTS 保留相同 AssetRef seam。'}</p></aside></div>
    <div className="brand-audio-timeline"><div className="brand-audio-ruler"><span>00:00</span><span>{String(audio.master_duration_ms / 2000).padStart(2, '0')}s</span><span>{audio.master_duration_ms / 1000}s</span></div>{mix.tracks.map(track => { const draft = trackDrafts[track.id] ?? { gain: track.gain_db, muted: track.muted }; return <article key={track.id}><header><span>{draft.muted ? <VolumeX size={14}/> : <Volume2 size={14}/>}<b>{track.role}</b><small>{track.rights_status}</small></span><label>音量 <input type="range" min={-48} max={12} step={1} value={draft.gain} onChange={event => setTrackDrafts(value => ({ ...value, [track.id]: { ...draft, gain: Number(event.target.value) } }))}/><em>{draft.gain} dB</em></label><label className="brand-audio-mute"><input type="checkbox" checked={draft.muted} onChange={event => setTrackDrafts(value => ({ ...value, [track.id]: { ...draft, muted: event.target.checked } }))}/>静音</label></header><div className="brand-audio-lane">{track.clips.map(clip => <span key={clip.id} style={{ left: `${clip.timeline_start_ms / audio.master_duration_ms * 100}%`, width: `${(clip.timeline_end_ms - clip.timeline_start_ms) / audio.master_duration_ms * 100}%` }} title={`${clip.timeline_start_ms / 1000}s–${clip.timeline_end_ms / 1000}s`}><b>{clip.label || clip.id}</b><small>{clip.timeline_start_ms / 1000}s–{clip.timeline_end_ms / 1000}s</small></span>)}</div>{track.clips.length ? <div className="brand-audio-assets">{track.clips.map(clip => <div key={clip.id}><span className="brand-audio-waveform" aria-label={`${clip.label} 波形`}>{(clip.waveform_peaks ?? []).map((peak, index) => <i key={index} style={{ height: `${Math.max(8, peak * 100)}%` }}/>)}</span><b>{clip.label || clip.id}</b>{assetURLs[clip.id] ? <audio controls preload="metadata" src={assetURLs[clip.id]}/> : <small>等待项目音频资产</small>}<label className="secondary-button brand-upload"><Upload size={12}/>替换<input type="file" accept="audio/wav,audio/mpeg,audio/aac" disabled={busy} onChange={event => { onReplace(clip.id, event.target.files?.[0]); event.target.value = '' }}/></label></div>)}</div> : null}</article>})}</div>
    <div className="brand-actions">{materialized ? <span className="brand-confirmed"><Check size={14}/>Audio A1 项目资产已就绪</span> : <button className="primary-button" disabled={busy} onClick={onMaterialize}>入库并生成可试听 Fixture</button>}<button className="secondary-button" disabled={busy || operations.length === 0} onClick={() => onSave(operations)}>保存音轨修改</button><button className="primary-button" disabled={busy || !materialized || operations.length > 0 || audio.status === 'preview_queued' || audio.status === 'preview_rendering'} onClick={onRender}>{audio.status === 'preview_queued' || audio.status === 'preview_rendering' ? <><LoaderCircle className="spin" size={14}/>正在合成音轨</> : mixedVideoURL ? '重新生成混音预览' : '生成混音预览'}</button></div>
  </div>
}

export function BrandFilmWorkspace({ taskId, taskOptions, onOpenTask, onCreateNew, onNotice }: Props) {
  const { currentProject } = useProject()
  const { requestedStage, navigateToStage } = useBrandFilmStageRoute()
  const [workspace, setWorkspace] = useState<ApiBrandFilmWorkspace | null>(null)
  const [brief, setBrief] = useState<ApiBrandBriefAnalysis | null>(null)
  const [briefEditMode, setBriefEditMode] = useState(false)
  const [conceptCandidates, setConceptCandidates] = useState<ApiBrandCreativeConcept[]>([])
  const [conceptEditMode, setConceptEditMode] = useState(false)
  const [plan, setPlan] = useState<ApiBrandFilmPlan | null>(null)
  const [planEditMode, setPlanEditMode] = useState(false)
  const [busy, setBusy] = useState('loading')
  const [assetPreviews, setAssetPreviews] = useState<Record<string, string>>({})
  const [generationReference, setGenerationReference] = useState<ApiAssetVersionRef | null>(null)
  const [generationReferencePreview, setGenerationReferencePreview] = useState('')
  const [attemptPreviews, setAttemptPreviews] = useState<Record<string, string>>({})
  const [finalPreview, setFinalPreview] = useState('')
  const [audioClipPreviews, setAudioClipPreviews] = useState<Record<string, string>>({})
  const [mixedAudioPreview, setMixedAudioPreview] = useState('')
  const [speechCapability, setSpeechCapability] = useState<ApiSpeechCapability>()
  const [feedbackByUnit, setFeedbackByUnit] = useState<Record<string, string>>({})
  const [pendingDestination, setPendingDestination] = useState<string | null | undefined>(undefined)
  const [projectName, setProjectName] = useState('')
  const planConfirmationInFlight = useRef(false)
  const conceptGenerationInFlight = useRef(false)
  const reloadWorkspace = useCallback(
    () => taskId ? api.getBrandFilmWorkspace(currentProject.id, taskId) : api.ensureBrandFilmFixtureWorkspace(currentProject.id),
    [currentProject.id, taskId],
  )

  const routeDraft = workspace?.video_draft.brand_film
  const stageStates = deriveBrandFilmStages({
    briefConfirmed: Boolean(last(routeDraft?.brief_analysis_versions)?.confirmed),
    conceptSelected: Boolean(routeDraft?.selected_concept_id),
    planConfirmed: Boolean(last(routeDraft?.film_plan_versions)?.confirmed),
    visualPreviewReady: Boolean(routeDraft?.generation?.preview_asset),
    audioPreviewReady: Boolean(routeDraft?.audio?.mixed_preview_asset_ref || routeDraft?.audio?.status === 'preview_ready'),
  })
  const activeStage = resolveBrandFilmStage(requestedStage, stageStates)

  useEffect(() => {
    if (!workspace || requestedStage === activeStage) return
    navigateToStage(activeStage, true)
  }, [activeStage, navigateToStage, requestedStage, workspace])

  useEffect(() => {
    let active = true
    setBusy('loading')
    void reloadWorkspace().then(value => { if (active) setWorkspace(value) }).catch(cause => {
      if (active) onNotice(cause instanceof Error ? cause.message : '品牌广告工作区载入失败。')
    }).finally(() => { if (active) setBusy('') })
    return () => { active = false }
  }, [onNotice, reloadWorkspace])

  useEffect(() => {
    if (!workspace) return
    const brand = workspace.video_draft.brand_film
    const currentBrief = last(brand.brief_analysis_versions) ?? null
    const currentConceptSet = last(brand.concept_sets)
    const currentPlan = last(brand.film_plan_versions) ?? null
    setBrief(currentBrief)
    setBriefEditMode(Boolean(currentBrief && !currentBrief.confirmed))
    setConceptCandidates(currentConceptSet?.candidates ?? [])
    setConceptEditMode(false)
    setPlan(currentPlan)
    setPlanEditMode(Boolean(currentPlan && !currentPlan.confirmed))
    const product = last(brand.brief_analysis_versions)?.asset_candidates.find(asset => isProductReference(asset) && asset.user_confirmed && asset.asset_ref)?.asset_ref
    setGenerationReference(brand.generation?.reference_asset ?? product ?? null)
  }, [workspace])

  useEffect(() => {
    const assets = last(workspace?.video_draft.brand_film.brief_analysis_versions)?.asset_candidates ?? []
    let active = true
    void Promise.all(assets.map(async asset => {
      if (asset.asset_ref) return [asset.id, await api.getProjectAssetPreview(currentProject.id, asset.asset_ref)] as const
      return [asset.id, fixtureReferenceUri(asset.source_locator, asset.role, asset.fixture_uri)] as const
    })).then(entries => {
      if (active) setAssetPreviews(Object.fromEntries(entries))
    }).catch(() => undefined)
    return () => { active = false }
  }, [currentProject.id, workspace])

  useEffect(() => {
    const generation = workspace?.video_draft.brand_film.generation
    if (!generation) return
    const refs = generation.units.flatMap(unit => unit.attempts.filter(attempt => attempt.output_asset_ref).map(attempt => ({ id: attempt.id, ref: attempt.output_asset_ref! })))
    void Promise.all(refs.map(async item => [item.id, await api.getProjectAssetPreview(currentProject.id, item.ref)] as const))
      .then(entries => setAttemptPreviews(Object.fromEntries(entries))).catch(() => undefined)
    if (generation.preview_asset) {
      void api.getProjectAssetPreview(currentProject.id, generation.preview_asset).then(setFinalPreview).catch(() => setFinalPreview(''))
    }
  }, [currentProject.id, workspace?.video_draft.brand_film.generation])

  useEffect(() => {
    const audio = workspace?.video_draft.brand_film.audio
    const mix = audio ? activeAudioMix(audio) : undefined
    const clips = (mix?.tracks ?? []).flatMap(track => track.clips).filter(clip => clip.asset_ref)
    let active = true
    if (!clips.length) {
      setAudioClipPreviews({})
      return () => { active = false }
    }
    void Promise.all(clips.map(async clip => [clip.id, await api.getProjectAssetPreview(currentProject.id, clip.asset_ref!)] as const)).then(entries => {
      if (active) setAudioClipPreviews(Object.fromEntries(entries))
    }).catch(() => { if (active) setAudioClipPreviews({}) })
    return () => { active = false }
  }, [currentProject.id, workspace?.video_draft.brand_film.audio?.active_mix_revision])

  useEffect(() => {
    const ref = workspace?.video_draft.brand_film.audio?.mixed_preview_asset_ref
    if (!ref) { setMixedAudioPreview(''); return }
    void api.getProjectAssetPreview(currentProject.id, ref).then(setMixedAudioPreview).catch(() => setMixedAudioPreview(''))
  }, [currentProject.id, workspace?.video_draft.brand_film.audio?.mixed_preview_asset_ref])

  const commit = async (key: string, action: () => Promise<ApiBrandFilmWorkspace>, message: string) => {
    setBusy(key)
    try {
      const value = await action()
      setWorkspace(value)
      onNotice(message)
      return value
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '品牌广告工作流操作失败。')
      return null
    } finally { setBusy('') }
  }

  if (!workspace) return <div className="brand-film-loading"><LoaderCircle className="spin" size={18}/><span>{busy === 'loading' ? '正在载入品牌广告任务…' : '尚未建立品牌广告工作区'}</span></div>

  const draft = workspace.video_draft.brand_film
  const conceptSet = last(draft.concept_sets)
  const source = draft.source_snapshot
  const revision = workspace.video_draft.revision
  const lockedBrief = Boolean(brief?.confirmed && !briefEditMode)
  const lockedPlan = Boolean(plan?.confirmed && !planEditMode)
  const planReady = Boolean(plan?.confirmed && !planEditMode)
  const productReferences = brief?.asset_candidates.filter(isProductReference) ?? []
  const logoReferences = brief?.asset_candidates.filter(asset => asset.role === 'logo') ?? []
  const referenceIsUsable = (asset: ApiBrandBriefAssetCandidate) => Boolean(asset.asset_ref || asset.fixture_uri)
  const allRequiredReferencesConfirmed = productReferences.length > 0 && logoReferences.length > 0
    && [...productReferences, ...logoReferences].every(asset => asset.user_confirmed && referenceIsUsable(asset) && asset.label.trim())
  const generationProductReferences = productReferences.filter(asset => asset.user_confirmed && asset.asset_ref)
  const selectedGenerationProduct = generationProductReferences.find(asset => sameAssetRef(asset.asset_ref, generationReference))
  const selectedGenerationPreview = generationReferencePreview || (selectedGenerationProduct ? assetPreviews[selectedGenerationProduct.id] : '')

  const analyze = async () => {
    setBusy('analyze')
    try {
      let analyzed = await api.analyzeBrandFilmBrief(currentProject.id, workspace.task.id, revision)
      const analyzedBrief = last(analyzed.video_draft.brand_film.brief_analysis_versions)
      const documentRef = source.evidence_refs.find(value => value.startsWith('knowledge://documents/'))
      const documentId = documentRef?.slice('knowledge://documents/'.length).split(/[?#]/)[0]
      const expectedProducts = briefProductNames(source.brief_text).length
      const usableProducts = analyzedBrief?.asset_candidates.filter(asset => isProductReference(asset) && referenceIsUsable(asset)).length ?? 0
      if (documentId && analyzedBrief && usableProducts < expectedProducts) {
        onNotice(`已识别 ${expectedProducts} 个商品，正在从原 PDF 提取对应正面图与 Logo…`)
        const document = await api.getKnowledgeDocument(currentProject.id, documentId)
        const extracted = await extractAndUploadBrandBriefAssets(currentProject.id, document)
        if (extracted.length) {
          analyzed = await api.updateBrandFilmBrief(currentProject.id, workspace.task.id, analyzed.video_draft.revision, {
            ...analyzedBrief,
            asset_candidates: extracted,
          })
        }
      }
      setWorkspace(analyzed)
      onNotice('Brief 已重新解析；商品、卖点和参考素材均可人工增删后再确认。')
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : 'Brief 重新解析失败。')
    } finally {
      setBusy('')
    }
  }
  const reanalyze = () => {
    if ((draft.concept_sets?.length ?? 0) > 0 && !window.confirm('重新解析会清空当前创意、分镜、生成与交付结果，是否继续？')) return
    return analyze()
  }
  const saveBrief = () => brief && commit('save-brief', () => api.updateBrandFilmBrief(currentProject.id, workspace.task.id, revision, editableBriefPayload(brief)), 'Brief 修改已保存为新的待确认修订，后续内容已等待重新生成。')
  const confirmBrief = async () => {
    if (!brief) return
    setBusy('confirm-brief')
    try {
      const saved = await api.updateBrandFilmBrief(currentProject.id, workspace.task.id, revision, editableBriefPayload(brief))
      setWorkspace(await api.confirmBrandFilmBrief(currentProject.id, workspace.task.id, saved.video_draft.revision))
      onNotice('Brief 与商品参考图已确认。')
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : 'Brief 确认失败。') } finally { setBusy('') }
  }
  const generateConcepts = async () => {
    if (conceptGenerationInFlight.current) return null
    conceptGenerationInFlight.current = true
    setBusy('concepts')
    const previousConceptSetCount = draft.concept_sets?.length ?? 0
    try {
      const result = await recoverCompletedBrandFilmMutation(
        () => api.generateBrandFilmConcepts(currentProject.id, workspace.task.id, revision),
        reloadWorkspace,
        latest => latest.video_draft.revision > revision
          && (latest.video_draft.brand_film.concept_sets?.length ?? 0) > previousConceptSetCount,
      )
      setWorkspace(result.value)
      onNotice(result.recovered ? '创意方向已生成；页面已自动同步到服务端最新修订。' : '已生成 3 个有差异的创意方向。')
      return result.value
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '创意方向生成失败。')
      return null
    } finally {
      conceptGenerationInFlight.current = false
      setBusy('')
    }
  }
  const regenerateConcepts = () => {
    if ((draft.film_plan_versions?.length ?? 0) > 0 && !window.confirm('重新生成创意会清空当前选择、分镜、视频生成与交付结果，是否继续？')) return
    return generateConcepts()
  }
  const updateConcept = (conceptId: string, changes: Partial<ApiBrandCreativeConcept>) => setConceptCandidates(items => items.map(item => item.id === conceptId ? { ...item, ...changes } : item))
  const saveConcepts = () => commit('save-concepts', () => api.updateBrandFilmConcepts(currentProject.id, workspace.task.id, revision, conceptCandidates), '创意方向修改已保存为新的待选择修订，后续内容已等待重新生成。')
  const cancelConceptEdit = () => {
    setConceptCandidates(conceptSet?.candidates ?? [])
    setConceptEditMode(false)
  }
  const selectConcept = (conceptId: string) => {
    if (conceptId === draft.selected_concept_id) return
    if (draft.selected_concept_id && !window.confirm('切换创意方向会清空当前剧本、视频生成与交付结果，是否继续？')) return
    return commit('select', () => api.selectBrandFilmConcept(currentProject.id, workspace.task.id, revision, conceptId), '创意方向已选择并冻结。')
  }
  const generatePlan = () => commit('plan', () => api.generateBrandFilmPlan(currentProject.id, workspace.task.id, revision), '15 秒剧本与分镜已生成。')
  const savePlan = () => plan && commit('save-plan', () => api.updateBrandFilmPlan(currentProject.id, workspace.task.id, revision, plan), '剧本与分镜修改已保存为新的待确认修订，视频生成结果已等待重新生成。')
  const confirmPlan = async () => {
    if (!plan || planConfirmationInFlight.current) return
    planConfirmationInFlight.current = true
    setBusy('confirm-plan')
    try {
      const saved = await api.updateBrandFilmPlan(currentProject.id, workspace.task.id, revision, plan)
      setWorkspace(await api.confirmBrandFilmPlan(currentProject.id, workspace.task.id, saved.video_draft.revision))
      onNotice('剧本与分镜已保存并确认。')
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : '剧本与分镜确认失败。'
      try {
        const latest = await reloadWorkspace()
        setWorkspace(latest)
        if (last(latest.video_draft.brand_film.film_plan_versions)?.confirmed) {
          onNotice('剧本与分镜已确认；页面已同步到服务端最新修订。')
        } else {
          onNotice(message)
        }
      } catch {
        onNotice(message)
      }
    } finally {
      planConfirmationInFlight.current = false
      setBusy('')
    }
  }

  const updateAsset = (id: string, changes: Partial<ApiBrandBriefAnalysis['asset_candidates'][number]>) => setBrief(value => value ? { ...value, asset_candidates: value.asset_candidates.map(asset => asset.id === id ? { ...asset, ...changes } : asset) } : value)
  const addProductAsset = () => setBrief(value => {
    if (!value) return value
    const ordinal = value.asset_candidates.filter(isProductReference).length + 1
    const id = `asset_product_manual_${Date.now()}`
    return {
      ...value,
      asset_candidates: [...value.asset_candidates, {
        id,
        role: 'product_front',
        label: `商品 ${ordinal}`,
        source_locator: `manual://brand-film/${workspace.task.id}/${id}`,
        rights_status: 'needs_confirmation',
        user_confirmed: false,
        replacement_note: '人工添加，等待上传商品图片',
      }],
    }
  })
  const removeProductAsset = (asset: ApiBrandBriefAssetCandidate) => {
    if (!isProductReference(asset)) return
    setBrief(value => value ? { ...value, asset_candidates: value.asset_candidates.filter(item => item.id !== asset.id) } : value)
    setAssetPreviews(value => {
      const next = { ...value }
      delete next[asset.id]
      return next
    })
    if (sameAssetRef(asset.asset_ref, generationReference)) setGenerationReference(null)
  }
  const uploadReferenceAsset = async (assetId: string, file?: File) => {
    if (!file) return
    setBusy('upload')
    try {
      const ref = await api.uploadProjectAsset(currentProject.id, file)
      setAssetPreviews(value => ({ ...value, [assetId]: URL.createObjectURL(file) }))
      updateAsset(assetId, { asset_ref: ref, user_confirmed: true, rights_status: 'user_confirmed', replacement_note: `用户上传：${file.name}` })
      onNotice('参考素材已上传；保存 Brief 后写入新的修订。')
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '参考素材上传失败。') } finally { setBusy('') }
  }
  const prepareGeneration = () => generationReference && commit('prepare-generation', () => api.prepareBrandFilmGeneration(currentProject.id, workspace.task.id, revision, generationReference), '已编排 GenerationUnit 并冻结 PromptPackage。')
  const generateUnit = async (unitId: string, feedback = '') => {
    setBusy(`generate-${unitId}`)
    try {
      const created = await api.generateBrandFilmUnit(currentProject.id, workspace.task.id, revision, unitId, feedback)
      setWorkspace(created.workspace)
      let job = created.job
      while (job.status === 'queued' || job.status === 'running') {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        job = await api.getBrandFilmUnitJob(currentProject.id, job.id)
      }
      setWorkspace(await api.reconcileBrandFilmUnit(currentProject.id, workspace.task.id, unitId, job.id))
      setFeedbackByUnit(value => ({ ...value, [unitId]: '' }))
      onNotice(job.status === 'succeeded' ? 'Seedance 片段已生成，请预览后锁定或反馈重生成。' : `片段生成未成功：${job.diagnostic ?? job.status}`)
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '片段生成失败。') } finally { setBusy('') }
  }
  const lockUnit = (unitId: string, attemptId: string) => commit(`lock-${unitId}`, () => api.lockBrandFilmUnit(currentProject.id, workspace.task.id, revision, unitId, attemptId), '片段已锁定。')
  const composePreview = () => commit('compose-preview', () => api.composeBrandFilmPreview(currentProject.id, workspace.task.id, revision), '15 秒品牌广告预览已完成裁切与拼接。')
  const prepareAudio = () => commit('prepare-audio', () => api.prepareBrandFilmAudio(currentProject.id, workspace.task.id, revision), '旁白、音乐与音效草稿已自动编排。')
  const startAudioDirector = async () => {
    setBusy('start-audio-director')
    try {
      let value = await api.prepareBrandFilmAudio(currentProject.id, workspace.task.id, revision)
	  setWorkspace(value)
      if (value.video_draft.brand_film.audio && !brandAudioMaterialized(value.video_draft.brand_film.audio)) {
        value = await api.materializeBrandFilmAudioAssets(currentProject.id, workspace.task.id, value.video_draft.revision)
		setWorkspace(value)
      }
      if (value.video_draft.brand_film.audio && brandAudioMaterialized(value.video_draft.brand_film.audio)) {
        value = await api.renderBrandFilmAudioPreview(currentProject.id, workspace.task.id, value.video_draft.revision)
		setWorkspace(value)
      }
      onNotice('AI 声音导演已完成默认编排与音频入库，完整混音预览正在生成。')
    } catch (cause) {
      try { setWorkspace(await reloadWorkspace()) } catch { /* keep the last successful step */ }
      onNotice(cause instanceof Error ? cause.message : 'AI 声音导演启动失败。')
    } finally { setBusy('') }
  }
  const materializeAudio = () => commit('materialize-audio', () => api.materializeBrandFilmAudioAssets(currentProject.id, workspace.task.id, revision), 'Audio A1 Fixture 已真实入库，现在可以逐段试听。')
  const replaceAudioClip = async (clipId: string, file?: File) => {
    if (!file) return
    setBusy(`replace-audio-${clipId}`)
    try {
      const ref = await api.uploadProjectAsset(currentProject.id, file)
      setWorkspace(await api.updateBrandFilmAudioMix(currentProject.id, workspace.task.id, revision, [{ op: 'replace_clip_asset', clip_id: clipId, asset_ref: ref }]))
      onNotice('音频片段已替换，并保存为新的 Mix 修订。')
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '音频替换失败。') } finally { setBusy('') }
  }
  const saveAudio = (operations: ApiBrandAudioMixOperation[]) => commit('save-audio', () => api.updateBrandFilmAudioMix(currentProject.id, workspace.task.id, revision, operations), '音轨修改已保存为新的 Mix 修订。')
  const selectAudioVariant = async (variantId: string) => {
    setBusy('select-audio-variant')
    try {
      let value = await api.selectBrandFilmAudioVariant(currentProject.id, workspace.task.id, revision, variantId)
	  setWorkspace(value)
      if (value.video_draft.brand_film.audio && !brandAudioMaterialized(value.video_draft.brand_film.audio)) {
        value = await api.materializeBrandFilmAudioAssets(currentProject.id, workspace.task.id, value.video_draft.revision)
		setWorkspace(value)
      }
      if (value.video_draft.brand_film.audio && brandAudioMaterialized(value.video_draft.brand_film.audio)) {
        value = await api.renderBrandFilmAudioPreview(currentProject.id, workspace.task.id, value.video_draft.revision)
		setWorkspace(value)
      }
      onNotice('声音 A/B 方案已切换并开始生成对应混音，锁定画面保持不变。')
    } catch (cause) {
      try { setWorkspace(await reloadWorkspace()) } catch { /* keep the last successful step */ }
      onNotice(cause instanceof Error ? cause.message : '声音方案切换失败。')
    } finally { setBusy('') }
  }
  const probeSpeech = async () => {
    setBusy('probe-speech')
    try {
      const capability = await api.probeBrandFilmSpeech(currentProject.id)
      setSpeechCapability(capability)
      onNotice(capability.available ? `MiniMax TTS 可用：${capability.model}` : `MiniMax TTS 不可用，继续使用 Fixture：${capability.error_code ?? '未配置'}`)
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : 'MiniMax TTS 能力检测失败。') } finally { setBusy('') }
  }
  const generateVoice = async (clipId: string, voiceAlias: string) => {
    setBusy(`generate-voice-${clipId}`)
    try {
      const value = await api.generateBrandFilmVoiceClip(currentProject.id, workspace.task.id, revision, clipId, voiceAlias)
      setWorkspace(value)
      const attempt = value.video_draft.brand_film.audio?.generation_attempts.filter(item => item.clip_id === clipId).at(-1)
      onNotice(attempt?.status === 'succeeded' ? 'MiniMax 真实旁白已生成并入库；请试听后重新生成混音预览。' : `MiniMax 生成失败，已保留 Fixture：${attempt?.error_code ?? 'unknown'}`)
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '旁白生成失败。') } finally { setBusy('') }
  }
  const renderAudioPreview = async () => {
    setBusy('render-audio-preview')
    try {
      let value = await api.renderBrandFilmAudioPreview(currentProject.id, workspace.task.id, revision)
      setWorkspace(value)
      for (let attempt = 0; attempt < 45 && ['preview_queued', 'preview_rendering'].includes(value.video_draft.brand_film.audio?.status ?? ''); attempt++) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        value = await reloadWorkspace()
        setWorkspace(value)
      }
      const audio = value.video_draft.brand_film.audio
      const failed = audio?.render_jobs.at(-1)?.status === 'failed'
      onNotice(failed ? `混音预览失败：${audio?.render_jobs.at(-1)?.error_message ?? '请重试'}` : audio?.mixed_preview_asset_ref ? 'Audio A2 混音预览已生成，画面没有重新生成。' : '混音任务仍在处理中，可稍后刷新查看。')
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '混音预览生成失败。') } finally { setBusy('') }
  }

  const leaveFor = (destination: string | null) => {
    if (destination) onOpenTask(destination)
    else onCreateNew()
  }
  const requestTaskChange = (destination: string | null) => {
    if (destination === workspace.task.id) return
    const hasLocalEdits = briefEditMode || conceptEditMode || planEditMode
    if (draft.stage === 'delivered' && !hasLocalEdits) {
      leaveFor(destination)
      return
    }
    setProjectName(savedProjectName(workspace.task.display_name, source.product_name || '未命名品牌广告'))
    setPendingDestination(destination)
  }
  const saveAndChangeTask = async () => {
    const name = projectName.trim()
    if (!name) return
    setBusy('save-project')
    try {
      let saved = workspace
      if (briefEditMode && brief) {
        saved = await api.updateBrandFilmBrief(currentProject.id, workspace.task.id, saved.video_draft.revision, editableBriefPayload(brief))
      } else if (conceptEditMode) {
        saved = await api.updateBrandFilmConcepts(currentProject.id, workspace.task.id, saved.video_draft.revision, conceptCandidates)
      } else if (planEditMode && plan) {
        saved = await api.updateBrandFilmPlan(currentProject.id, workspace.task.id, saved.video_draft.revision, plan)
      }
      await api.renameCreativeTask(currentProject.id, workspace.task.id, saved.task.version, name)
      const destination = pendingDestination
      setPendingDestination(undefined)
      onNotice(`“${name}”已保存，可随时从品牌广告任务下拉框继续。`)
      if (destination !== undefined) leaveFor(destination)
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '当前品牌广告保存失败，请重试。')
    } finally {
      setBusy('')
    }
  }

  return <><BrandFilmWorkbenchShell
    productName={source.product_name}
    briefName={source.brief_name}
    sourceLabel={source.source_kind === 'strategy_package'
      ? `策略交接 · ${source.strategy_package_id || source.intake_id || '已绑定'}`
      : `${source.fixture_id || 'PDF Brief'}${source.fixture_version ? ` v${source.fixture_version}` : ''}`}
    specification={`${source.duration_seconds}s · ${source.aspect_ratio} · ${source.channel}`}
    revision={revision}
    serverStage={draft.stage}
    stages={stageStates}
    activeStage={activeStage}
    busy={Boolean(busy)}
    assets={(brief?.asset_candidates ?? []).map(asset => ({ id: asset.id, label: asset.label, preview: assetPreviews[asset.id], source: briefAssetSource(asset.source_locator, asset.role), confirmed: asset.user_confirmed }))}
    headerActions={<div className="brand-project-switcher"><label><span>当前品牌广告</span><select aria-label="切换品牌广告任务" value={workspace.task.id} onChange={event => requestTaskChange(event.target.value)}><option value={workspace.task.id}>{savedProjectName(workspace.task.display_name, source.product_name)} · r{revision}</option>{taskOptions.filter(task => task.id !== workspace.task.id).map(task => <option value={task.id} key={task.id}>{savedProjectName(task.display_name, task.direction.focus || task.direction.concept || '未命名品牌广告')} · v{task.version}</option>)}</select></label><button className="primary-button" type="button" onClick={() => requestTaskChange(null)}><Plus size={14}/>新建品牌广告</button></div>}
    onStageChange={navigateToStage}
  >

      {activeStage === 'brief' ? <section className="brand-film-section"><header><div><span className="section-label">PHASE 01</span><h3>Brief 分析与事实确认</h3></div><ModelBadge alias={brief?.model_alias} version={brief?.model_version}/></header>
        {!brief ? <div className="brand-film-empty"><FileText size={24}/><p>使用 Seed-2-pro 解析固定娇兰 Brief；不可用时回退固定样例。</p><button className="primary-button" disabled={Boolean(busy)} onClick={() => void analyze()}><WandSparkles size={15}/>解析 Brief</button></div> : <>
          <div className="brand-form-grid"><label className="wide">Brief 摘要<textarea disabled={lockedBrief} value={brief.summary} onChange={event => setBrief({ ...brief, summary: event.target.value })}/></label><label>目标人群<textarea disabled={lockedBrief} value={brief.audience} onChange={event => setBrief({ ...brief, audience: event.target.value })}/></label><label>核心传播信息<textarea disabled={lockedBrief} value={brief.core_message} onChange={event => setBrief({ ...brief, core_message: event.target.value })}/></label><label className="wide">统一口播音色<textarea disabled={lockedBrief} value={brief.voice_direction} onChange={event => setBrief({ ...brief, voice_direction: event.target.value })}/></label></div>
          {!lockedBrief && brief.confirmed ? <div className="brand-edit-notice">当前正在修改已确认 Brief。保存后会创建新的待确认修订，并清空依赖旧 Brief 的创意、分镜、生成与交付结果。</div> : null}
          <div className="brand-fact-grid"><div><div className="brand-facts-heading"><h4>广告要点 / 卖点</h4>{!lockedBrief ? <button type="button" onClick={() => setBrief({ ...brief, selling_points: [...brief.selling_points, { text: '', locator: `user://brand-film/brief#selling-point-${brief.selling_points.length + 1}`, confidence: 1, status: 'brief_fact' }] })}><Plus size={13}/>添加卖点</button> : null}</div>{brief.selling_points.map((fact, index) => <div className="brand-fact-row" key={`${fact.locator}-${index}`}><label className="brand-fact"><input disabled={lockedBrief} value={fact.text} onChange={event => setBrief({ ...brief, selling_points: brief.selling_points.map((item, itemIndex) => itemIndex === index ? { ...item, text: event.target.value } : item) })}/><small>{fact.locator} · {Math.round(fact.confidence * 100)}% · {fact.status}</small></label>{!lockedBrief ? <button className="brand-fact-delete" type="button" aria-label={`删除卖点 ${index + 1}`} title={brief.selling_points.length <= 1 ? '至少保留一条卖点' : '删除这条卖点'} disabled={brief.selling_points.length <= 1} onClick={() => setBrief({ ...brief, selling_points: brief.selling_points.filter((_, itemIndex) => itemIndex !== index) })}><Trash2 size={14}/></button> : null}</div>)}</div><EditableList title="必须保留" items={brief.mandatory_elements} disabled={lockedBrief} onChange={items => setBrief({ ...brief, mandatory_elements: items })}/><EditableList title="禁用表述" items={brief.prohibited_claims} disabled={lockedBrief} onChange={items => setBrief({ ...brief, prohibited_claims: items })}/><EditableList title="图片要求" items={brief.image_requirements} disabled={lockedBrief} onChange={items => setBrief({ ...brief, image_requirements: items })}/><EditableList title="视频要求" items={brief.video_requirements} disabled={lockedBrief} onChange={items => setBrief({ ...brief, video_requirements: items })}/><EditableList title="待人工确认" items={brief.uncertainties} disabled={lockedBrief} onChange={items => setBrief({ ...brief, uncertainties: items })}/></div>
          <div className="brand-assets">
            <div className="brand-assets-heading"><div><h4>商品与品牌参考素材</h4><span>{productReferences.length} 个商品 · {logoReferences.length} 个品牌标识</span></div>{!lockedBrief ? <button className="secondary-button" type="button" onClick={addProductAsset}><Plus size={13}/>添加商品</button> : null}</div>
            <p>优先展示 Brief 中已提取并入库的图片。每个商品独立命名、确认和替换；未提取到图片时才需要人工上传。</p>
            {brief.asset_candidates.map(asset => {
              const sourceLabel = briefAssetSource(asset.source_locator, asset.role)
              const product = isProductReference(asset)
              const hasImage = referenceIsUsable(asset)
              return <article key={asset.id} className={hasImage ? 'has-image' : 'needs-image'}>
                <div className="brand-asset-thumb">{assetPreviews[asset.id] ? <img src={assetPreviews[asset.id]} alt={asset.label}/> : <Image size={22}/>}</div>
                <div className="brand-asset-copy">
                  <span className="brand-asset-kind">{product ? '商品参考图' : asset.role === 'logo' ? '品牌 Logo' : '参考素材'}</span>
                  {lockedBrief ? (
                    <b>{asset.label}</b>
                  ) : (
                    <input aria-label={`${product ? '商品' : '素材'}名称`} value={asset.label} onChange={event => updateAsset(asset.id, { label: event.target.value })}/>
                  )}
                  <small className="brand-asset-source" title={sourceLabel}>{sourceLabel || '人工补充'}</small>
                  <small className={hasImage ? 'brand-asset-state ready' : 'brand-asset-state'}>{asset.asset_ref ? `已入库 · Asset ${asset.asset_ref.asset_id} v${asset.asset_ref.version}` : asset.fixture_uri ? '固定样例素材 · 确认后可使用' : '未提取到可用图片，请上传'}</small>
                </div>
                <div className="brand-asset-controls">
                  <label className="brand-checkbox"><input type="checkbox" disabled={lockedBrief || !hasImage} checked={asset.user_confirmed} onChange={event => updateAsset(asset.id, { user_confirmed: event.target.checked, rights_status: event.target.checked ? 'user_confirmed' : 'needs_confirmation' })}/><Check size={13}/>确认使用</label>
                  {!lockedBrief ? <label className="secondary-button brand-upload"><Upload size={13}/>{hasImage ? '替换图片' : '上传图片'}<input type="file" accept="image/png,image/jpeg" onChange={event => { void uploadReferenceAsset(asset.id, event.target.files?.[0]) }}/></label> : null}
                  {!lockedBrief && product ? <button className="brand-asset-delete" type="button" aria-label={`删除${asset.label}`} onClick={() => removeProductAsset(asset)}><Trash2 size={13}/>删除</button> : null}
                </div>
              </article>
            })}
            {!productReferences.length ? <button className="brand-add-product-empty" type="button" disabled={lockedBrief} onClick={addProductAsset}><Plus size={16}/>添加第一个商品及参考图</button> : null}
          </div>
          <div className="brand-actions">{!lockedBrief ? <><button className="secondary-button" disabled={Boolean(busy)} onClick={() => void reanalyze()}><Sparkles size={14}/>重新解析</button><button className="secondary-button" disabled={Boolean(busy)} onClick={() => void saveBrief()}>保存修改</button><button className="primary-button" disabled={Boolean(busy) || !allRequiredReferencesConfirmed} onClick={() => void confirmBrief()}>确认 Brief</button></> : <><span className="brand-confirmed"><Check size={14}/>Brief 已确认</span><button className="secondary-button" disabled={Boolean(busy)} onClick={() => setBriefEditMode(true)}>编辑 Brief</button><button className="secondary-button" disabled={Boolean(busy)} onClick={() => void reanalyze()}><Sparkles size={14}/>重新解析</button><button className="primary-button brand-next-button" type="button" onClick={() => navigateToStage('concept')}>下一步：创意方向<ArrowRight size={15}/></button></>}</div>
        </>}
      </section> : null}

      {activeStage === 'concept' ? <section className="brand-film-section" aria-disabled={!brief?.confirmed}><header><div><span className="section-label">PHASE 02A</span><h3>有差异的创意方向</h3></div><ModelBadge alias={conceptSet?.model_alias} version={conceptSet?.model_version}/></header>{!brief?.confirmed ? <p className="brand-locked">确认 Brief 后开放。</p> : !conceptSet ? <div className="brand-film-empty compact"><p>一次生成 3 个叙事机制不同的方向，生成后可逐项人工修改。</p><button className="primary-button" disabled={Boolean(busy)} onClick={() => void generateConcepts()}>生成创意候选</button></div> : <>{conceptEditMode ? <div className="brand-edit-notice">这里只修改方向标题、核心创意句与叙事机制。保存后形成新的待选择修订；镜头执行细节继续在剧本分镜阶段处理。</div> : null}<div className="brand-concepts">{conceptCandidates.map(concept => <ConceptCard key={concept.id} concept={concept} editing={conceptEditMode} busy={Boolean(busy)} onChange={changes => updateConcept(concept.id, changes)} onSelect={() => void selectConcept(concept.id)}/>)}</div><div className="brand-actions">{conceptEditMode ? <><button className="secondary-button" disabled={Boolean(busy)} onClick={cancelConceptEdit}>取消编辑，返回选择</button><button className="primary-button" disabled={Boolean(busy)} onClick={() => void saveConcepts()}>保存创意修改</button></> : <>{draft.selected_concept_id ? <span className="brand-confirmed"><Check size={14}/>创意方向已选择</span> : <span className="brand-pending">请选择一个创意方向</span>}<button className="secondary-button" disabled={Boolean(busy)} onClick={() => setConceptEditMode(true)}>编辑方向文案</button><button className="secondary-button" disabled={Boolean(busy)} onClick={() => void regenerateConcepts()}>重新生成整组</button>{draft.selected_concept_id ? <button className="primary-button brand-next-button" type="button" onClick={() => navigateToStage('storyboard')}>下一步：剧本分镜<ArrowRight size={15}/></button> : null}</>}</div></>}</section> : null}

      {activeStage === 'storyboard' ? (
        <section className="brand-film-section" aria-disabled={!draft.selected_concept_id || conceptEditMode}>
          <header>
            <div><span className="section-label">PHASE 02B</span><h3>剧本与镜头表</h3></div>
            <ModelBadge alias={plan?.model_alias} version={plan?.model_version}/>
          </header>
          {conceptEditMode ? <p className="brand-locked">请先保存创意修改并重新选择方向。</p> : !draft.selected_concept_id ? <p className="brand-locked">选择创意方向后开放。</p> : !plan ? (
            <div className="brand-film-empty compact"><p>用户编辑剧本、旁白和镜头字段，底层 Prompt 不直接暴露。</p><button className="primary-button" disabled={Boolean(busy)} onClick={() => void generatePlan()}>生成剧本与分镜</button></div>
          ) : <>
            {!lockedPlan && plan.confirmed ? <div className="brand-edit-notice">当前正在修改已确认剧本。保存后会形成新的待确认修订，并清空旧视频生成与交付结果。</div> : null}
            <div className="brand-story-overview">
              <label>片名<input disabled={lockedPlan} value={plan.title} onChange={event => setPlan({ ...plan, title: event.target.value })}/></label>
              <label>音乐方向<input disabled={lockedPlan} value={plan.music_direction} onChange={event => setPlan({ ...plan, music_direction: event.target.value })}/></label>
              <label>故事概要<textarea disabled={lockedPlan} value={plan.story_summary} onChange={event => setPlan({ ...plan, story_summary: event.target.value })}/></label>
              <label>口播方向<textarea disabled={lockedPlan} value={plan.voice_direction} onChange={event => setPlan({ ...plan, voice_direction: event.target.value })}/></label>
            </div>
            <StoryboardEditor plan={plan} disabled={lockedPlan} onChange={setPlan}/>
            <div className="brand-actions">{!lockedPlan ? <><button className="secondary-button" disabled={Boolean(busy)} onClick={() => void generatePlan()}>重新生成整版</button><button className="primary-button" disabled={Boolean(busy)} onClick={() => void savePlan()}>保存剧本修改</button><button className="secondary-button" disabled={Boolean(busy)} onClick={() => void confirmPlan()}>确认剧本与分镜</button></> : <><span className="brand-confirmed"><Check size={14}/>剧本与分镜已确认</span><button className="secondary-button" disabled={Boolean(busy)} onClick={() => setPlanEditMode(true)}>编辑剧本与分镜</button><button className="primary-button brand-next-button" type="button" onClick={() => navigateToStage('generation')}>下一步：视频生成<ArrowRight size={15}/></button></>}</div>
          </>}
        </section>
      ) : null}

      {activeStage === 'generation' ? <section className="brand-film-section" aria-disabled={!planReady}><header><div><span className="section-label">PHASE 03</span><h3>视频生成、反馈重试与片段锁定</h3></div><span className="brand-model-badge">Seedance 2.0 · 单候选</span></header>{!planReady ? <p className="brand-locked">保存并确认剧本与分镜后开放。</p> : !draft.generation ? <div className="brand-film-empty brand-generation-setup"><Film size={24}/><div><h4>选择本次生成的主商品参考图</h4><p>已从 Brief 确认结果自动带入。系统仍按一镜头一个生成单元冻结 PromptPackage；其他商品素材继续保留在 Brief 中。</p></div>{generationProductReferences.length ? <div className="brand-generation-products">{generationProductReferences.map(asset => { const selected = sameAssetRef(asset.asset_ref, generationReference); return <button type="button" className={selected ? 'selected' : ''} aria-pressed={selected} key={asset.id} onClick={() => { setGenerationReference(asset.asset_ref ?? null); setGenerationReferencePreview('') }}><div>{assetPreviews[asset.id] ? <img src={assetPreviews[asset.id]} alt={asset.label}/> : <Image size={22}/>}</div><span><b>{asset.label}</b><small>Asset {asset.asset_ref?.asset_id} v{asset.asset_ref?.version}</small></span><i>{selected ? <Check size={14}/> : null}</i></button>})}</div> : <div className="brand-generation-missing"><Image size={18}/><div><b>Brief 中还没有已入库的商品图</b><span>返回 Brief，为每个商品上传并确认图片后再继续。</span></div><button className="secondary-button" type="button" onClick={() => navigateToStage('brief')}>返回 Brief 补充</button></div>} {generationReference ? <div className="brand-generation-reference selected">{selectedGenerationPreview ? <img src={selectedGenerationPreview} alt={selectedGenerationProduct?.label ?? '主商品参考图'}/> : null}<span><b>{selectedGenerationProduct?.label ?? '主商品参考图'}</b><small>本次 Seedance 生成使用 Asset {generationReference.asset_id} v{generationReference.version}</small></span></div> : null}<button className="primary-button" disabled={Boolean(busy) || !generationReference} onClick={() => void prepareGeneration()}>确认主商品并编排生成单元</button></div> : <><div className="brand-generation-units">{draft.generation.units.map(unit => { const latestAttempt = unit.attempts.at(-1); const locked = Boolean(unit.locked_attempt_id); return <article key={unit.id} className={locked ? 'locked' : ''}><header><div><b>生成单元 {String(unit.order).padStart(2, '0')}</b><span>{unit.start_second}s–{unit.end_second}s · {unit.shot_ids.join(' + ')}</span></div>{locked ? <span className="brand-confirmed"><Lock size={13}/>已锁定</span> : null}</header><small>PromptPackage r{unit.prompt_packages.at(-1)?.revision} · {unit.prompt_packages.at(-1)?.content_hash.slice(0, 18)}…</small>{latestAttempt?.output_asset_ref ? <video controls src={attemptPreviews[latestAttempt.id]}/> : <div className="brand-unit-placeholder"><Film size={20}/><span>{latestAttempt ? `Attempt ${latestAttempt.ordinal} · ${latestAttempt.status}` : '尚未生成候选'}</span></div>}{!locked ? <div className="brand-unit-actions"><GenerationUnitActions attempt={latestAttempt} busy={Boolean(busy)} feedback={feedbackByUnit[unit.id] ?? ''} onFeedback={feedback => setFeedbackByUnit(value => ({ ...value, [unit.id]: feedback }))} onGenerate={feedback => void generateUnit(unit.id, feedback)} onLock={attemptId => void lockUnit(unit.id, attemptId)}/></div> : null}</article>})}</div>{finalPreview ? <div className="brand-final-preview"><div><span className="section-label">15 SECOND PREVIEW</span><h4>已锁定片段合成预览</h4><small>720×1280 · H.264/AAC · 项目素材可追溯</small></div><video controls src={finalPreview}/></div> : null}<div className="brand-actions"><button className="primary-button" disabled={Boolean(busy) || draft.generation.units.some(unit => !unit.locked_attempt_id) || Boolean(draft.generation.preview_asset)} onClick={() => void composePreview()}>裁切并拼接 15 秒预览</button>{draft.generation.preview_asset ? <span className="brand-confirmed"><Check size={14}/>预览 Asset {draft.generation.preview_asset.asset_id} v{draft.generation.preview_asset.version}</span> : null}</div></>}</section> : null}

      {activeStage === 'audio' ? <section className="brand-film-section" aria-disabled={!draft.generation?.preview_asset}><header><div><span className="section-label">AUDIO A0–A4</span><h3>AI 声音导演、真实旁白与完整混音预览</h3></div><span className="brand-model-badge">Audio Director · MiniMax · FFmpeg</span></header>{!draft.generation?.preview_asset ? <p className="brand-locked">完成并合成视觉预览后开放。</p> : !draft.audio ? <div className="brand-film-empty"><Volume2 size={24}/><p>进入后自动获得带发音词典、时长适配、声画检查和 A/B 声音方案的默认音轨，不需要从空白配置开始。</p><button className="primary-button" disabled={Boolean(busy)} onClick={() => void startAudioDirector()}>启动 AI 声音导演</button></div> : <AudioWorkspaceEditor key={`${draft.audio.active_variant_id}-${activeAudioMix(draft.audio)?.content_hash}`} audio={draft.audio} videoURL={finalPreview} mixedVideoURL={mixedAudioPreview} assetURLs={audioClipPreviews} speechCapability={speechCapability} busy={Boolean(busy)} onMaterialize={() => void materializeAudio()} onReplace={(clipId, file) => void replaceAudioClip(clipId, file)} onSave={operations => void saveAudio(operations)} onRender={() => void renderAudioPreview()} onProbeSpeech={() => void probeSpeech()} onGenerateVoice={(clipId, voiceAlias) => void generateVoice(clipId, voiceAlias)} onSelectVariant={variantId => void selectAudioVariant(variantId)} onPlanDirector={() => void prepareAudio()}/>}</section> : null}

      {activeStage === 'audio' && draft.audio && planReady && !conceptEditMode ? (() => { const progress = brandAudioProgress(draft.audio); return <footer className="brand-generation-seam"><b>{progress.title}</b><span>{progress.next}</span><small>{progress.detail}</small></footer> })() : null}
  </BrandFilmWorkbenchShell>
    {pendingDestination !== undefined ? <div className="brand-project-save-layer" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setPendingDestination(undefined) }}><section className="brand-project-save-dialog" role="dialog" aria-modal="true" aria-labelledby="brand-project-save-title"><span className="section-label">SAVE CURRENT WORK</span><h3 id="brand-project-save-title">先保存当前品牌广告，再开始新的创作</h3><p>当前流程尚未完成。系统会保留已确认阶段和最新编辑修订，之后可以从任务下拉框继续。</p><label>品牌广告名称<input autoFocus maxLength={80} value={projectName} onChange={event => setProjectName(event.target.value)} placeholder="例如：娇兰黄金复原蜜 · 15 秒品牌广告"/></label><div><button className="secondary-button" type="button" disabled={Boolean(busy)} onClick={() => setPendingDestination(undefined)}>取消</button><button className="primary-button" type="button" disabled={Boolean(busy) || !projectName.trim()} onClick={() => void saveAndChangeTask()}>{busy === 'save-project' ? <LoaderCircle className="spin" size={14}/> : null}保存并继续</button></div></section></div> : null}
  </>
}

function EditableList({ title, items, disabled, onChange }: { title: string; items: string[]; disabled: boolean; onChange: (items: string[]) => void }) {
  return <label><h4>{title}</h4><textarea disabled={disabled} value={items.join('\n')} onChange={event => onChange(event.target.value.split('\n'))}/></label>
}
