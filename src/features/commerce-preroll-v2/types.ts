export type CommercePrerollStep = 'source' | 'understanding' | 'direction' | 'settings' | 'first-frame' | 'video'
export type AsyncStatus = 'idle' | 'loading' | 'ready' | 'error'
export type PrerollDuration = 6 | 7 | 8 | 9 | 10
export type ProductField = 'name' | 'category' | 'description' | 'sellingPoints' | 'appearanceGuardrails'

export type SourceVideo = {
  id: string
	recipeVersion?: string
  name: string
  videoUrl: string
  posterUrl: string
  durationSeconds: number
  aspectRatio: string
  resolution: string
  sizeLabel: string
  version: string
  sourceLabel: string
  rightsStatus: 'confirmed' | 'unconfirmed'
  uploaded?: boolean
  assetId?: string
  assetVersion?: number
}

export type ProductFacts = {
  name: string
  category: string
  description: string
  sellingPoints: string
  appearanceGuardrails: string
}

export type RiskFact = {
  id: string
  text: string
  sourceLabel: string
  status: 'pending' | 'confirmed' | 'removed'
}

export type SourceAnalysis = {
  original: ProductFacts
  visualStyle: string
  subtitleSummary: string
  voiceSummary: string
  audioMood: string
  openingShot: string
  evidenceCount: number
}

export type ProductReference = {
  id: string
  imageUrl: string
  label: string
  sourceLabel: string
  kind: 'extracted' | 'uploaded' | 'generated'
	overallScore?: number
	qualitySummary?: string
}

export type CommercePrerollTaskSummary = { id: string; displayName: string; status: string; version: number; updatedAt: string }
export type CommercePrerollCreativeVersion = { id: string; version: number; draftRevision: number; createdAt: string }

export type HookProposal = {
  id: string
  name: string
  imageUrl: string
  concept: string
  rationale: string
  sellingPoint: string
  action: string
	mechanism?: string
	visualSignature?: string
	suitableFor?: string[]
	whyForSource?: string[]
	openingState?: string
	resultState?: string
	continuityPlan?: string
	riskNotes?: string[]
	matchScore?: number
	recommendationLevel?: 'primary' | 'alternative'
}

export type CreativeBeat = {
  id: 'hook' | 'change' | 'lockup'
  label: string
  timeLabel: string
  detail: string
	startMs?: number
	endMs?: number
	visualDescription?: string
	subjectAction?: string
	camera?: string
	sceneAndLighting?: string
	productState?: string
	transitionIn?: string
	transitionOut?: string
	onScreenText?: string
	audioInstruction?: string
}

export type GenerationDraft = {
  promptSummary: string
  compiledPrompt: string
  beats: CreativeBeat[]
  revision: number
	creativePrompt?: string
	lockedConstraints?: string[]
	editMode?: 'storyboard_compiled' | 'manual_creative_override'
}

export type FirstFrameCandidate = {
  id: string
  imageUrl: string
  label: string
  title: string
  description: string
  providerJobId?: string
  batchId?: string
  assetId?: string
  assetVersion?: number
}

export type GeneratedPreroll = {
  id: string
  videoUrl: string
  posterUrl: string
  duration: PrerollDuration
  aspectRatio: string
  createdAt: string
  assetId?: string
  assetVersion?: number
}

export type CommercePrerollState = {
  sessionVersion: 2
  clientTaskId: string
  activeStep: CommercePrerollStep
  source: SourceVideo | null
  rightsConfirmed: boolean
  analysisStatus: AsyncStatus
  analysisStage: number
  analysis: SourceAnalysis | null
  productDraft: ProductFacts | null
  productConfirmed: boolean
  productReference: ProductReference | null
	productReferences: ProductReference[]
  riskFacts: RiskFact[]
  hooksStatus: AsyncStatus
  hooks: HookProposal[]
  selectedHookId: string
  duration: PrerollDuration
  extraInstruction: string
  generationDraft: GenerationDraft | null
  firstFramesStatus: AsyncStatus
  firstFrames: FirstFrameCandidate[]
  selectedFirstFrameId: string
  videoStatus: AsyncStatus
  videoStage: number
  output: GeneratedPreroll | null
  savedAssetId: string
  error: string
  errorScope: 'analysis' | 'hooks' | 'frames' | 'video' | null
  lastSavedAt: string | null
}

export type CommercePrerollSession = Omit<CommercePrerollState, 'source'> & {
  source: SourceVideo | null
}
