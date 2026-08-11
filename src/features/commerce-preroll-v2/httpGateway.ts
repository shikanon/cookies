import { api } from '../../data/api'
import type { AnalysisResult, CommercePrerollGateway, SaveResult } from './gateway'
import { commerceHookPreparationPlan, createInitialCommercePrerollState } from './reducer'
import type { CommercePrerollCreativeVersion, CommercePrerollState, CommercePrerollTaskSummary, CreativeBeat, FirstFrameCandidate, GeneratedPreroll, GenerationDraft, HookProposal, PrerollDuration, ProductFacts, ProductReference, SourceVideo } from './types'

type AssetVersionRef = { asset_id: string; version: number }
type AssetRef = { project_id: string; asset_version: AssetVersionRef }
type AsyncResource = { status: string; progress?: number }
type ApiWorkspace = {
  revision: number
  active_stage: string
  source_video: AssetRef
  source_metadata?: { WidthPixels?: number; HeightPixels?: number; DurationMS?: number }
  analysis: AsyncResource & { content: {
    product: { name: string; category: string; description: string; selling_points: string[]; appearance_guardrails: string[]; logo_guardrails: string[] }
    visual_style: string; subtitle_summary: string; voice_summary: string; audio_mood: string; opening_shot: string
    evidence: unknown[]; risks: string[]
  } }
  product_reference?: { asset?: AssetRef; timestamp_ms: number }
  product_reference_batch?: { selected_id?: string; candidates: Array<{ id: string; source_kind: string; label: string; frame: { asset?: AssetRef; timestamp_ms: number }; scores: { overall?: number; frontality?: number; sharpness?: number; completeness?: number; logo_readability?: number; occlusion?: number } }> }
  hook_batch?: AsyncResource & { id: string; selected_hook_id?: string; items: Array<{ id: string; recipe_version?: string; name: string; mechanism?: string; concept: string; rationale: string; selling_point: string; primary_action: string; visual_signature?: string; suitable_for?: string[]; why_for_this_source?: string[]; opening_state?: string; result_state?: string; continuity_plan?: string; risk_notes?: string[]; match_score?: number; recommendation_level?: 'primary' | 'alternative' }> }
  prompt_draft?: { revision: number; prompt_summary: string; compiled_prompt: string; creative_prompt?: string; locked_constraints?: string[]; edit_mode?: 'storyboard_compiled' | 'manual_creative_override'; beats: Array<{ id: 'hook' | 'change' | 'lockup'; label: string; start_ms: number; end_ms: number; detail: string; visual_description?: string; subject_action?: string; camera?: string; scene_and_lighting?: string; product_state?: string; transition_in?: string; transition_out?: string; on_screen_text?: string; audio_instruction?: string }> }
  first_frame_batch?: AsyncResource & { id: string; selected_id?: string; candidates: Array<{ id: string; provider_job_id: string; status: string; asset?: AssetRef; variant_key: string; title: string; description: string }> }
	latest_video_attempt_id?: string
	video_error?: { message?: string }
  output_asset?: AssetRef
  adopted_asset?: AssetRef
}
type TaskDetail = { task: { id: string; display_name: string; version: number; status: string; updated_at: string }; video_draft: { revision: number; commerce_preroll_v2: ApiWorkspace } }

const backendOrigin = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? ''
const wait = (ms: number) => new Promise<void>(resolve => window.setTimeout(resolve, ms))
const key = (kind: string) => `commerce-v2-${kind}-${Date.now()}-${Math.random().toString(36).slice(2)}`

function workspaceAspectRatio(workspace: ApiWorkspace) {
  const width = workspace.source_metadata?.WidthPixels ?? 0
  const height = workspace.source_metadata?.HeightPixels ?? 0
  if (!width || !height) return '9:16'
  let a = width
  let b = height
  while (b) [a, b] = [b, a % b]
  return `${width / a}:${height / a}`
}

async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await fetch(`${backendOrigin}/api/creative/v1${path}`, {
    method,
    credentials: 'include',
    headers: { ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...(method === 'GET' ? {} : { 'Idempotency-Key': key(method.toLowerCase()) }) },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  const payload = text ? JSON.parse(text) as T | { error?: { message?: string } } : {}
  if (!response.ok) throw new Error((payload as { error?: { message?: string } }).error?.message ?? `请求失败（HTTP ${response.status}）`)
  return payload as T
}

async function previewUrl(projectId: string, ref: AssetRef) {
  const version = ref.asset_version
  const response = await fetch(`${backendOrigin}/platform/v1/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(version.asset_id)}/versions/${version.version}/preview`, { credentials: 'include' })
  const payload = await response.json() as { url?: string; error?: { message?: string } }
  if (!response.ok || !payload.url) throw new Error(payload.error?.message ?? '素材预览地址不可用')
  return payload.url.startsWith('/') ? `${backendOrigin}${payload.url}` : payload.url
}

function facts(workspace: ApiWorkspace): ProductFacts {
  const product = workspace.analysis.content.product
  return {
    name: product.name,
    category: product.category,
    description: product.description,
    sellingPoints: product.selling_points.join('\n'),
    appearanceGuardrails: [...product.appearance_guardrails, ...product.logo_guardrails].join('\n'),
  }
}

function generationDraft(workspace: ApiWorkspace): GenerationDraft {
  const draft = workspace.prompt_draft
  if (!draft) throw new Error('服务端没有返回生成 Prompt')
  return { promptSummary: draft.prompt_summary, compiledPrompt: draft.compiled_prompt, creativePrompt: draft.creative_prompt ?? draft.compiled_prompt, lockedConstraints: draft.locked_constraints ?? [], editMode: draft.edit_mode, revision: draft.revision, beats: draft.beats.map(beat => ({ id: beat.id, label: beat.label, startMs: beat.start_ms, endMs: beat.end_ms, timeLabel: `${(beat.start_ms / 1000).toFixed(1)}s–${(beat.end_ms / 1000).toFixed(1)}s`, detail: beat.detail, visualDescription: beat.visual_description, subjectAction: beat.subject_action, camera: beat.camera, sceneAndLighting: beat.scene_and_lighting, productState: beat.product_state, transitionIn: beat.transition_in, transitionOut: beat.transition_out, onScreenText: beat.on_screen_text, audioInstruction: beat.audio_instruction })) }
}

function productPayload(product: ProductFacts) {
  return {
    name: product.name,
    category: product.category,
    description: product.description,
    selling_points: product.sellingPoints.split(/\n|；/).map(value => value.trim()).filter(Boolean),
    appearance_guardrails: product.appearanceGuardrails.split(/\n|；/).map(value => value.trim()).filter(Boolean),
    logo_guardrails: ['保持原视频可见品牌字样与 Logo，不得改写'],
  }
}

export class HttpCommercePrerollGateway implements CommercePrerollGateway {
  private taskId = ''
	private taskVersion = 0
  private taskName = ''
	private forceNewTask = false
  private revision = 0
  private workspace: ApiWorkspace | null = null
  private productReferenceUrl = ''
	private draftMutation: Promise<void> = Promise.resolve()

  constructor(private readonly projectId: string) {}

  async uploadSource(file: File, source: SourceVideo) {
    const ref = await api.uploadProjectAsset(this.projectId, file)
    const durableUrl = await previewUrl(this.projectId, { project_id: this.projectId, asset_version: ref })
    return { ...source, id: `${ref.asset_id}:${ref.version}`, videoUrl: durableUrl, uploaded: false, assetId: ref.asset_id, assetVersion: ref.version, version: `v${ref.version}`, rightsStatus: 'unconfirmed' as const }
  }

  async uploadProductReference(file: File) {
    const ref = await api.uploadProjectAsset(this.projectId, file)
    const workspace = await this.command('bind-product-reference', { expected_revision: this.revision, asset: ref })
    const asset = workspace.product_reference?.asset
    if (!asset) throw new Error('商品参考图绑定失败')
    this.productReferenceUrl = await previewUrl(this.projectId, asset)
    return { id: `${ref.asset_id}:${ref.version}`, imageUrl: this.productReferenceUrl, label: file.name, sourceLabel: '用户上传商品图', kind: 'uploaded' as const }
  }

	async selectProductReference(reference: ProductReference) {
		if (reference.kind === 'uploaded') return
		await this.command('select-product-reference', { expected_revision: this.revision, candidate_id: reference.id })
	}

	async reextractProductReferences() {
		const workspace = await this.command('prepare-references', { expected_revision: this.revision })
		return Promise.all((workspace.product_reference_batch?.candidates ?? []).filter(item => item.frame.asset).map(async item => ({ id: item.id, imageUrl: await previewUrl(this.projectId, item.frame.asset!), label: item.label, sourceLabel: `来自原视频 ${(item.frame.timestamp_ms / 1000).toFixed(1)}s`, kind: item.source_kind === 'user_upload' ? 'uploaded' as const : 'extracted' as const, overallScore: item.scores.overall, qualitySummary: item.source_kind === 'user_upload' ? '用户上传' : `正面 ${Math.round((item.scores.frontality ?? 0) * 100)}% · 清晰 ${Math.round((item.scores.sharpness ?? 0) * 100)}%` })))
	}

  async uploadCustomFirstFrame(file: File) {
    const ref = await api.uploadProjectAsset(this.projectId, file)
    const workspace = await this.command('bind-custom-first-frame', { expected_revision: this.revision, asset: ref, title: file.name })
    const batch = workspace.first_frame_batch
    const item = batch?.candidates.at(-1)
    if (!batch || !item || !item.asset) throw new Error('自定义首帧绑定失败')
    return { id: item.id, imageUrl: await previewUrl(this.projectId, item.asset), label: '自定义首帧', title: item.title || file.name, description: item.description, batchId: batch.id, assetId: ref.asset_id, assetVersion: ref.version }
  }

  private set(detail: TaskDetail) {
    this.taskId = detail.task.id
	this.taskVersion = detail.task.version
	this.taskName = detail.task.display_name
    this.revision = detail.video_draft.revision
    this.workspace = detail.video_draft.commerce_preroll_v2
    return this.workspace
  }

  currentTaskId() { return this.taskId }

  async ensureTask(source: SourceVideo) {
	if (!source.assetId || !source.assetVersion) throw new Error('请等待原视频上传完成后再保存')
	if (this.taskId && this.sourceMatches(source)) return
	const created = await request<TaskDetail>(`/projects/${encodeURIComponent(this.projectId)}/commerce-preroll-v2`, 'POST', {
	  source_video: { asset_id: source.assetId, version: source.assetVersion }, rights_confirmed: true, duration_seconds: 8, channel: 'douyin',
	})
	this.set(created)
	this.forceNewTask = false
  }

  async listTasks(): Promise<CommercePrerollTaskSummary[]> {
	const response = await api.listCreativeTasks(this.projectId, 100)
	return response.items.filter(item => item.performance_mode === 'commerce_preroll').map(item => ({ id: item.id, displayName: item.display_name, status: item.status, version: item.version, updatedAt: item.updated_at }))
  }

  async openTask(taskId: string): Promise<CommercePrerollState> {
	const workspace = this.set(await request<TaskDetail>(`/projects/${encodeURIComponent(this.projectId)}/creative-tasks/${encodeURIComponent(taskId)}/commerce-preroll-v2`))
	const sourceUrl = await previewUrl(this.projectId, workspace.source_video)
	const referenceItems = await Promise.all((workspace.product_reference_batch?.candidates ?? []).filter(item => item.frame.asset).map(async item => ({ id: item.id, imageUrl: await previewUrl(this.projectId, item.frame.asset!), label: item.label, sourceLabel: `来自原视频 ${(item.frame.timestamp_ms / 1000).toFixed(1)}s`, kind: item.source_kind === 'user_upload' ? 'uploaded' as const : 'extracted' as const, overallScore: item.scores.overall, qualitySummary: item.source_kind === 'user_upload' ? '用户上传' : `正面 ${Math.round((item.scores.frontality ?? 0) * 100)}% · 清晰 ${Math.round((item.scores.sharpness ?? 0) * 100)}%` })))
	const reference = referenceItems.find(item => item.id === workspace.product_reference_batch?.selected_id) ?? null
	const hookItems = (workspace.hook_batch?.items ?? []).map(item => ({ id: item.id, recipeVersion: item.recipe_version, name: item.name, imageUrl: reference?.imageUrl ?? '', concept: item.concept, rationale: item.rationale, sellingPoint: item.selling_point, action: item.primary_action, mechanism: item.mechanism, visualSignature: item.visual_signature, suitableFor: item.suitable_for, whyForSource: item.why_for_this_source, openingState: item.opening_state, resultState: item.result_state, continuityPlan: item.continuity_plan, riskNotes: item.risk_notes, matchScore: item.match_score, recommendationLevel: item.recommendation_level }))
	const frames = await Promise.all((workspace.first_frame_batch?.candidates ?? []).filter(item => item.asset).map(async item => ({ id: item.id, imageUrl: await previewUrl(this.projectId, item.asset!), label: item.variant_key, title: item.title, description: item.description, providerJobId: item.provider_job_id, batchId: workspace.first_frame_batch!.id, assetId: item.asset!.asset_version.asset_id, assetVersion: item.asset!.asset_version.version })))
	const stage = workspace.active_stage
	const activeStep = stage.includes('video') || stage === 'output_adopted' ? 'video' : stage.includes('frame') ? 'first-frame' : stage === 'hook_selected' ? 'settings' : stage === 'hooks_ready' ? 'direction' : stage.includes('understanding') ? 'understanding' : 'source'
	const base = createInitialCommercePrerollState()
	let output = null
	if (workspace.output_asset) output = { id: `${workspace.output_asset.asset_version.asset_id}:${workspace.output_asset.asset_version.version}`, videoUrl: await previewUrl(this.projectId, workspace.output_asset), posterUrl: '', duration: (workspace.prompt_draft?.beats.at(-1)?.end_ms ?? 8000) / 1000 as PrerollDuration, aspectRatio: workspaceAspectRatio(workspace), createdAt: new Date().toISOString(), assetId: workspace.output_asset.asset_version.asset_id, assetVersion: workspace.output_asset.asset_version.version }
	return { ...base, clientTaskId: this.taskId, activeStep, source: { id: `${workspace.source_video.asset_version.asset_id}:${workspace.source_video.asset_version.version}`, name: this.taskName || '电商原视频', videoUrl: sourceUrl, posterUrl: '', durationSeconds: (workspace.source_metadata as { DurationMS?: number } | undefined)?.DurationMS ? (workspace.source_metadata as { DurationMS: number }).DurationMS / 1000 : 0, aspectRatio: workspaceAspectRatio(workspace), resolution: `${workspace.source_metadata?.WidthPixels ?? ''}×${workspace.source_metadata?.HeightPixels ?? ''}`, sizeLabel: '', version: `v${workspace.source_video.asset_version.version}`, sourceLabel: '当前 Project 素材', rightsStatus: 'confirmed', assetId: workspace.source_video.asset_version.asset_id, assetVersion: workspace.source_video.asset_version.version }, rightsConfirmed: true, analysisStatus: workspace.analysis.status === 'ready' ? 'ready' : 'idle', analysisStage: workspace.analysis.status === 'ready' ? 5 : 0, analysis: workspace.analysis.status === 'ready' ? { original: facts(workspace), visualStyle: workspace.analysis.content.visual_style, subtitleSummary: workspace.analysis.content.subtitle_summary, voiceSummary: workspace.analysis.content.voice_summary, audioMood: workspace.analysis.content.audio_mood, openingShot: workspace.analysis.content.opening_shot, evidenceCount: workspace.analysis.content.evidence.length } : null, productDraft: workspace.analysis.status === 'ready' ? facts(workspace) : null, productConfirmed: Boolean(workspace.hook_batch), productReference: reference, productReferences: referenceItems, riskFacts: [], hooksStatus: workspace.hook_batch ? 'ready' : 'idle', hooks: hookItems, selectedHookId: workspace.hook_batch?.selected_hook_id ?? '', duration: ((workspace.prompt_draft?.beats.at(-1)?.end_ms ?? 8000) / 1000) as PrerollDuration, generationDraft: workspace.prompt_draft ? generationDraft(workspace) : null, firstFramesStatus: workspace.first_frame_batch ? 'ready' : 'idle', firstFrames: frames, selectedFirstFrameId: workspace.first_frame_batch?.selected_id ?? '', videoStatus: output ? 'ready' : stage === 'video_generating' ? 'loading' : 'idle', output, savedAssetId: workspace.adopted_asset?.asset_version.asset_id ?? '' }
  }

	async openLatest(): Promise<CommercePrerollState | null> {
		const detail = await this.latest()
		return detail ? this.openTask(detail.task.id) : null
	}

	startNew() { this.taskId = ''; this.taskVersion = 0; this.taskName = ''; this.workspace = null; this.forceNewTask = true }

  async renameTask(displayName: string): Promise<CommercePrerollTaskSummary> {
	const task = await api.renameCreativeTask(this.projectId, this.taskId, this.taskVersion, displayName)
	this.taskVersion = task.version; this.taskName = task.display_name
	return { id: task.id, displayName: task.display_name, status: task.status, version: task.version, updatedAt: task.updated_at }
  }

  async listVersions(): Promise<CommercePrerollCreativeVersion[]> {
	const response = await request<{ items: Array<{ id: string; version: number; draft_version: number; created_at: string }> }>(`/projects/${encodeURIComponent(this.projectId)}/creative-tasks/${encodeURIComponent(this.taskId)}/commerce-preroll-v2/versions`)
	return response.items.map(item => ({ id: item.id, version: item.version, draftRevision: item.draft_version, createdAt: item.created_at }))
  }

  async saveVersion(displayName: string): Promise<CommercePrerollCreativeVersion> {
	const renamed = this.taskName !== displayName
	const item = await request<{ id: string; version: number; draft_version: number; created_at: string }>(`/projects/${encodeURIComponent(this.projectId)}/creative-tasks/${encodeURIComponent(this.taskId)}/commerce-preroll-v2:save-version`, 'POST', { expected_revision: this.revision, expected_task_version: this.taskVersion, display_name: displayName })
	this.taskName = displayName
	if (renamed) this.taskVersion += 1
	return { id: item.id, version: item.version, draftRevision: item.draft_version, createdAt: item.created_at }
  }

  async restoreVersion(versionId: string) { await this.command('restore-version', { expected_revision: this.revision, version_id: versionId }) }

  private async latest() {
	try {
	  const detail = await request<TaskDetail>(`/projects/${encodeURIComponent(this.projectId)}/commerce-preroll-v2:latest`)
	  this.set(detail)
	  return detail
	} catch {
	  return null
	}
  }

  private sourceMatches(source: SourceVideo) {
	const ref = this.workspace?.source_video.asset_version
	return Boolean(ref && ref.asset_id === source.assetId && ref.version === source.assetVersion)
  }

  private async command(action: string, body: Record<string, unknown>) {
	if (!this.taskId) {
	  const latest = await request<TaskDetail>(`/projects/${encodeURIComponent(this.projectId)}/commerce-preroll-v2:latest`)
	  this.set(latest)
	  if ('expected_revision' in body) body.expected_revision = this.revision
	}
    return this.set(await request<TaskDetail>(`/projects/${encodeURIComponent(this.projectId)}/creative-tasks/${encodeURIComponent(this.taskId)}/commerce-preroll-v2:${action}`, 'POST', body))
  }

	private enqueueDraftMutation<T>(operation: () => Promise<T>): Promise<T> {
		const result = this.draftMutation.then(operation, operation)
		this.draftMutation = result.then(() => undefined, () => undefined)
		return result
	}

  async analyzeSource(source: SourceVideo, onProgress: (stage: number) => void): Promise<AnalysisResult> {
    if (!source.assetId || !source.assetVersion) throw new Error('请等待原视频上传完成后再解析')
    onProgress(1)
	if (!this.taskId && !this.forceNewTask) await this.latest()
	if (!this.sourceMatches(source)) await this.ensureTask(source)
    onProgress(2)
	const analyzed = this.workspace?.analysis.status === 'ready'
	  ? this.workspace
	  : await this.command('analyze-source', { expected_revision: this.revision })
    onProgress(4)
	const prepared = analyzed.product_reference?.asset
	  ? analyzed
	  : await this.command('prepare-references', { expected_revision: this.revision })
    onProgress(5)
    const product = facts(prepared)
    const ref = prepared.product_reference?.asset
    if (!ref) throw new Error('解析完成，但没有生成商品参考图')
    this.productReferenceUrl = await previewUrl(this.projectId, ref)
    const references = await Promise.all((prepared.product_reference_batch?.candidates ?? []).filter(item => item.frame.asset).map(async item => ({ id: item.id, imageUrl: await previewUrl(this.projectId, item.frame.asset!), label: item.label, sourceLabel: `来自原视频 ${(item.frame.timestamp_ms / 1000).toFixed(1)}s`, kind: item.source_kind === 'user_upload' ? 'uploaded' as const : 'extracted' as const, overallScore: item.scores.overall, qualitySummary: item.source_kind === 'user_upload' ? '用户上传' : `正面 ${Math.round((item.scores.frontality ?? 0) * 100)}% · 清晰 ${Math.round((item.scores.sharpness ?? 0) * 100)}%` })))
    const reference: ProductReference = references.find(item => item.id === prepared.product_reference_batch?.selected_id) ?? { id: `${ref.asset_version.asset_id}:${ref.asset_version.version}`, imageUrl: this.productReferenceUrl, label: '原视频商品清晰帧', sourceLabel: `来自原视频 ${((prepared.product_reference?.timestamp_ms ?? 0) / 1000).toFixed(1)}s`, kind: 'extracted' }
    return {
      product,
      reference,
	  references: references.length ? references : [reference],
      analysis: {
        original: product, visualStyle: analyzed.analysis.content.visual_style,
        subtitleSummary: analyzed.analysis.content.subtitle_summary, voiceSummary: analyzed.analysis.content.voice_summary,
        audioMood: analyzed.analysis.content.audio_mood, openingShot: analyzed.analysis.content.opening_shot,
        evidenceCount: analyzed.analysis.content.evidence.length,
      },
      risks: analyzed.analysis.content.risks.map((text, index) => ({ id: `risk-${index + 1}`, text, sourceLabel: '原视频多模态解析', status: 'pending' as const })),
    }
  }

  async compileHookProposals(product: ProductFacts, _source: SourceVideo): Promise<HookProposal[]> {
    if (!this.workspace) throw new Error('请先恢复电商前贴任务后再生成钩子')
    const plan = commerceHookPreparationPlan(this.workspace.active_stage, Boolean(this.workspace.hook_batch))
    if (plan.confirm) await this.command('confirm-understanding', { expected_revision: this.revision, product: productPayload(product), accepted_risks: [] })
    const workspace = plan.generate ? await this.command('generate-hooks', { expected_revision: this.revision }) : this.workspace
    return (workspace.hook_batch?.items ?? []).map(item => ({ id: item.id, recipeVersion: item.recipe_version, name: item.name, imageUrl: this.productReferenceUrl, concept: item.concept, rationale: item.rationale, sellingPoint: item.selling_point, action: item.primary_action, mechanism: item.mechanism, visualSignature: item.visual_signature, suitableFor: item.suitable_for, whyForSource: item.why_for_this_source, openingState: item.opening_state, resultState: item.result_state, continuityPlan: item.continuity_plan, riskNotes: item.risk_notes, matchScore: item.match_score, recommendationLevel: item.recommendation_level }))
  }

  async compileGenerationDraft(input: { product: ProductFacts; hook: HookProposal; duration: PrerollDuration; extraInstruction: string }): Promise<GenerationDraft> {
    const workspace = await this.command('select-hook', { expected_revision: this.revision, hook_id: input.hook.id, duration_seconds: input.duration, extra_instruction: input.extraInstruction })
    return generationDraft(workspace)
  }

  async updateStoryboard(beats: CreativeBeat[]) {
	return this.enqueueDraftMutation(async () => {
	  const workspace = await this.command('update-storyboard', { expected_revision: this.revision, beats: beats.map(beat => ({ id: beat.id, label: beat.label, start_ms: beat.startMs, end_ms: beat.endMs, detail: beat.detail, visual_description: beat.visualDescription, subject_action: beat.subjectAction, camera: beat.camera, scene_and_lighting: beat.sceneAndLighting, product_state: beat.productState, transition_in: beat.transitionIn, transition_out: beat.transitionOut, on_screen_text: beat.onScreenText, audio_instruction: beat.audioInstruction })) })
	  return generationDraft(workspace)
	})
  }

  async updatePrompt(creativePrompt: string) {
	return this.enqueueDraftMutation(async () => generationDraft(await this.command('update-prompt', { expected_revision: this.revision, creative_prompt: creativePrompt })))
  }

  async generateFirstFrames(_draft: GenerationDraft, _reference: ProductReference, onProgress: (stage: number) => void): Promise<FirstFrameCandidate[]> {
    onProgress(1)
	if (!this.taskId) await this.latest()
	let workspace = this.workspace
	const resumable = workspace?.first_frame_batch && ['queued', 'running', 'partial', 'ready'].includes(workspace.first_frame_batch.status)
	if (!workspace || !resumable) workspace = await this.command('generate-first-frames', { expected_revision: this.revision })
    for (let attempts = 0; attempts < 90; attempts += 1) {
      const pending = workspace.first_frame_batch?.candidates.find(item => !['ready', 'failed', 'cancelled'].includes(item.status))
      if (!pending) break
      await wait(2000)
      workspace = await this.command('reconcile-first-frame', { expected_revision: this.revision, candidate_id: pending.id, provider_job_id: pending.provider_job_id })
      onProgress(Math.min(3, 1 + Math.round((workspace.first_frame_batch?.candidates.filter(item => item.status === 'ready').length ?? 0) / 2)))
    }
    const batch = workspace.first_frame_batch
    if (!batch) throw new Error('首帧任务状态丢失')
    const ready = batch.candidates.filter(item => item.status === 'ready' && item.asset)
    if (!ready.length) throw new Error('三个首帧候选均生成失败')
    return Promise.all(ready.map(async item => ({ id: item.id, imageUrl: await previewUrl(this.projectId, item.asset!), label: item.variant_key, title: item.title, description: item.description, providerJobId: item.provider_job_id, batchId: batch.id, assetId: item.asset!.asset_version.asset_id, assetVersion: item.asset!.asset_version.version })))
  }

  async createVideo(input: { draft: GenerationDraft; frame: FirstFrameCandidate; duration: PrerollDuration }, onProgress: (stage: number) => void): Promise<GeneratedPreroll> {
    if (!input.frame.batchId) throw new Error('所选首帧缺少服务端批次信息')
    await this.command('select-first-frame', { expected_revision: this.revision, batch_id: input.frame.batchId, candidate_id: input.frame.id })
    onProgress(1)
    let workspace = await this.command('generate-video', { expected_revision: this.revision, model_alias: 'cookies.video.standard' })
    const jobId = workspace.latest_video_attempt_id
    if (!jobId) throw new Error('视频任务没有返回 Provider Job')
    for (let attempts = 0; attempts < 180; attempts += 1) {
      await wait(3000)
      workspace = await this.command('reconcile-video', { expected_revision: this.revision, provider_job_id: jobId })
      onProgress(attempts < 2 ? 2 : 3)
      if (workspace.output_asset || workspace.video_error) break
    }
    if (!workspace.output_asset) throw new Error(workspace.video_error?.message ?? '视频生成未完成')
    const ref = workspace.output_asset
    return { id: `${ref.asset_version.asset_id}:${ref.asset_version.version}`, videoUrl: await previewUrl(this.projectId, ref), posterUrl: '', duration: input.duration, aspectRatio: workspaceAspectRatio(workspace), createdAt: new Date().toISOString(), assetId: ref.asset_version.asset_id, assetVersion: ref.asset_version.version }
  }

  async saveOutputToLibrary(_output: GeneratedPreroll): Promise<SaveResult> {
    const workspace = await this.command('adopt-output', { expected_revision: this.revision })
    const ref = workspace.adopted_asset
    if (!ref) throw new Error('保存结果失败')
    return { assetId: ref.asset_version.asset_id }
  }
}
