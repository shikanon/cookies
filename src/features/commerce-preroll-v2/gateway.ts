import { fixtureAnalysis, fixtureFirstFrames, fixtureHooks, fixtureOutput, fixtureProductFacts, fixtureProductReference, fixtureRiskFacts } from './fixtures'
import type { CommercePrerollCreativeVersion, CommercePrerollState, CommercePrerollTaskSummary, CreativeBeat, FirstFrameCandidate, GeneratedPreroll, GenerationDraft, HookProposal, PrerollDuration, ProductFacts, ProductReference, RiskFact, SourceAnalysis, SourceVideo } from './types'

export type AnalysisResult = { analysis: SourceAnalysis; product: ProductFacts; reference: ProductReference; references?: ProductReference[]; risks: RiskFact[] }
export type SaveResult = { assetId: string }

export interface CommercePrerollGateway {
	listTasks?(): Promise<CommercePrerollTaskSummary[]>
	openTask?(taskId: string): Promise<CommercePrerollState>
	openLatest?(): Promise<CommercePrerollState | null>
	startNew?(): void
	ensureTask?(source: SourceVideo): Promise<void>
	currentTaskId?(): string
	renameTask?(displayName: string): Promise<CommercePrerollTaskSummary>
	listVersions?(): Promise<CommercePrerollCreativeVersion[]>
	saveVersion?(displayName: string): Promise<CommercePrerollCreativeVersion>
	restoreVersion?(versionId: string): Promise<void>
  uploadSource?(file: File, source: SourceVideo): Promise<SourceVideo>
  uploadProductReference?(file: File): Promise<ProductReference>
	selectProductReference?(reference: ProductReference): Promise<void>
	reextractProductReferences?(): Promise<ProductReference[]>
  uploadCustomFirstFrame?(file: File): Promise<FirstFrameCandidate>
  analyzeSource(source: SourceVideo, onProgress: (stage: number) => void): Promise<AnalysisResult>
  compileHookProposals(product: ProductFacts, source: SourceVideo): Promise<HookProposal[]>
  compileGenerationDraft(input: { product: ProductFacts; hook: HookProposal; duration: PrerollDuration; extraInstruction: string }): Promise<GenerationDraft>
	updateStoryboard?(beats: CreativeBeat[]): Promise<GenerationDraft>
	updatePrompt?(creativePrompt: string): Promise<GenerationDraft>
  generateFirstFrames(draft: GenerationDraft, reference: ProductReference, onProgress: (stage: number) => void): Promise<FirstFrameCandidate[]>
  createVideo(input: { draft: GenerationDraft; frame: FirstFrameCandidate; duration: PrerollDuration }, onProgress: (stage: number) => void): Promise<GeneratedPreroll>
  saveOutputToLibrary(output: GeneratedPreroll): Promise<SaveResult>
}

function wait(ms: number) {
  return new Promise<void>(resolve => window.setTimeout(resolve, ms))
}

function beatsFor(duration: PrerollDuration, hook: HookProposal) {
  const hookEnd = Math.max(2, Math.round(duration * 0.25))
  const changeEnd = Math.max(hookEnd + 2, duration - 2)
  return [
    { id: 'hook' as const, label: '建立钩子', timeLabel: `00:00–00:0${hookEnd}`, detail: `${hook.name}先制造信息缺口，让商品轮廓成为第一注意点。` },
    { id: 'change' as const, label: '完成变化', timeLabel: `00:0${hookEnd}–00:0${changeEnd}`, detail: hook.action },
    { id: 'lockup' as const, label: '商品定格', timeLabel: `00:0${changeEnd}–00:${String(duration).padStart(2, '0')}`, detail: '商品正面、稳定、无遮挡定格，不追加未经确认的字幕。' },
  ]
}

export class FixtureCommercePrerollGateway implements CommercePrerollGateway {
  async analyzeSource(_source: SourceVideo, onProgress: (stage: number) => void): Promise<AnalysisResult> {
    for (let stage = 1; stage <= 5; stage += 1) {
      await wait(360)
      onProgress(stage)
    }
    return { analysis: fixtureAnalysis, product: fixtureProductFacts, reference: fixtureProductReference, risks: fixtureRiskFacts }
  }

  async compileHookProposals(_product: ProductFacts, _source: SourceVideo) {
    await wait(520)
    return fixtureHooks
  }

  async compileGenerationDraft(input: { product: ProductFacts; hook: HookProposal; duration: PrerollDuration; extraInstruction: string }) {
    await wait(280)
    const promptSummary = `竖版高端护肤广告前贴。${input.hook.concept} ${input.product.appearanceGuardrails}${input.extraInstruction ? ` 补充要求：${input.extraInstruction}` : ''}`
    return {
      promptSummary,
      compiledPrompt: `${promptSummary} 时长 ${input.duration} 秒。仅执行一个主动作，保持商品名称、瓶型、标签、Logo、色彩与文字布局一致。`,
      beats: beatsFor(input.duration, input.hook),
      revision: Date.now(),
    }
  }

  async generateFirstFrames(_draft: GenerationDraft, _reference: ProductReference, onProgress: (stage: number) => void) {
    onProgress(1)
    await wait(620)
    onProgress(2)
    await wait(620)
    onProgress(3)
    return fixtureFirstFrames
  }

  async createVideo(input: { draft: GenerationDraft; frame: FirstFrameCandidate; duration: PrerollDuration }, onProgress: (stage: number) => void) {
    for (let stage = 1; stage <= 3; stage += 1) {
      await wait(760)
      onProgress(stage)
    }
    return fixtureOutput(input.duration)
  }

  async saveOutputToLibrary(_output: GeneratedPreroll) {
    await wait(420)
    return { assetId: `asset_commerce_preroll_${Date.now()}` }
  }
}

export const fixtureCommercePrerollGateway = new FixtureCommercePrerollGateway()
