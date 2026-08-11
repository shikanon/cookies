import type { CommercePrerollState, CommercePrerollStep, FirstFrameCandidate, GeneratedPreroll, GenerationDraft, HookProposal, ProductField, ProductFacts, ProductReference, PrerollDuration, RiskFact, SourceAnalysis, SourceVideo } from './types'

export type CommercePrerollRetryOperation = 'analysis' | 'hooks' | 'frames' | 'video' | null

export function commercePrerollRetryOperation(step: CommercePrerollStep, errorScope?: CommercePrerollRetryOperation): CommercePrerollRetryOperation {
  if (errorScope) return errorScope
  if (step === 'understanding') return 'analysis'
  if (step === 'direction') return 'hooks'
  if (step === 'settings' || step === 'first-frame') return 'frames'
  if (step === 'video') return 'video'
  return null
}

export function commerceHookPreparationPlan(activeStage: string, hasHookBatch: boolean) {
  if (hasHookBatch || activeStage === 'hooks_ready') return { confirm: false, generate: false }
  if (activeStage === 'understanding_confirmed') return { confirm: false, generate: true }
  return { confirm: true, generate: true }
}

function createClientTaskId() {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `commerce-preroll-${Date.now()}`
}

export function createInitialCommercePrerollState(): CommercePrerollState {
  return {
    sessionVersion: 2,
    clientTaskId: createClientTaskId(),
    activeStep: 'source',
    source: null,
    rightsConfirmed: false,
    analysisStatus: 'idle',
    analysisStage: 0,
    analysis: null,
    productDraft: null,
    productConfirmed: false,
    productReference: null,
	productReferences: [],
    riskFacts: [],
    hooksStatus: 'idle',
    hooks: [],
    selectedHookId: '',
    duration: 8,
    extraInstruction: '',
    generationDraft: null,
    firstFramesStatus: 'idle',
    firstFrames: [],
    selectedFirstFrameId: '',
    videoStatus: 'idle',
    videoStage: 0,
    output: null,
    savedAssetId: '',
    error: '',
    errorScope: null,
    lastSavedAt: null,
  }
}

export type CommercePrerollAction =
  | { type: 'restore'; state: CommercePrerollState }
  | { type: 'open-step'; step: CommercePrerollStep }
  | { type: 'source-selected'; source: SourceVideo }
  | { type: 'rights-changed'; confirmed: boolean }
  | { type: 'analysis-started' }
  | { type: 'analysis-progress'; stage: number }
  | { type: 'analysis-ready'; analysis: SourceAnalysis; product: ProductFacts; reference: ProductReference; references?: ProductReference[]; risks: RiskFact[] }
  | { type: 'product-field-changed'; field: ProductField; value: string }
  | { type: 'risk-resolved'; id: string; status: 'confirmed' | 'removed' }
  | { type: 'reference-changed'; reference: ProductReference }
  | { type: 'hooks-started' }
  | { type: 'hooks-ready'; hooks: HookProposal[] }
  | { type: 'hook-selected'; id: string }
  | { type: 'duration-changed'; duration: PrerollDuration }
  | { type: 'instruction-changed'; value: string }
  | { type: 'draft-ready'; draft: GenerationDraft }
  | { type: 'frames-started' }
  | { type: 'frames-ready'; frames: FirstFrameCandidate[] }
  | { type: 'frame-selected'; id: string }
  | { type: 'video-started' }
  | { type: 'video-progress'; stage: number }
  | { type: 'video-ready'; output: GeneratedPreroll }
  | { type: 'output-saved'; assetId: string }
  | { type: 'operation-failed'; scope: 'analysis' | 'hooks' | 'frames' | 'video'; message: string }
  | { type: 'reset' }

const clearVideo = { videoStatus: 'idle' as const, videoStage: 0, output: null, savedAssetId: '' }
const clearFramesAndVideo = { firstFramesStatus: 'idle' as const, firstFrames: [], selectedFirstFrameId: '', ...clearVideo }
const clearHooksDownstream = { hooksStatus: 'idle' as const, hooks: [], selectedHookId: '', generationDraft: null, ...clearFramesAndVideo }

export function commercePrerollReducer(state: CommercePrerollState, action: CommercePrerollAction): CommercePrerollState {
  switch (action.type) {
    case 'restore': return action.state
    case 'open-step': return { ...state, activeStep: action.step, error: '', errorScope: null }
    case 'source-selected': return { ...createInitialCommercePrerollState(), clientTaskId: state.clientTaskId, source: action.source, rightsConfirmed: action.source.rightsStatus === 'confirmed' }
    case 'rights-changed': return { ...state, rightsConfirmed: action.confirmed }
    case 'analysis-started': return { ...state, activeStep: 'understanding', analysisStatus: 'loading', analysisStage: 0, analysis: null, productDraft: null, productConfirmed: false, productReference: null, productReferences: [], riskFacts: [], ...clearHooksDownstream, error: '', errorScope: null }
    case 'analysis-progress': return { ...state, analysisStage: action.stage }
    case 'analysis-ready': return { ...state, analysisStatus: 'ready', analysisStage: 5, analysis: action.analysis, productDraft: action.product, productReference: action.reference, productReferences: action.references?.length ? action.references : [action.reference], riskFacts: action.risks, error: '', errorScope: null }
    case 'product-field-changed': return state.productDraft ? { ...state, productDraft: { ...state.productDraft, [action.field]: action.value }, productConfirmed: false, ...clearHooksDownstream } : state
    case 'risk-resolved': return { ...state, riskFacts: state.riskFacts.map(item => item.id === action.id ? { ...item, status: action.status } : item) }
    case 'reference-changed': return { ...state, productReference: action.reference, productReferences: state.productReferences.some(item => item.id === action.reference.id) ? state.productReferences : [...state.productReferences, action.reference], ...clearFramesAndVideo }
    case 'hooks-started': return { ...state, productConfirmed: true, hooksStatus: 'loading', hooks: [], selectedHookId: '', generationDraft: null, ...clearFramesAndVideo, error: '', errorScope: null }
    case 'hooks-ready': return { ...state, activeStep: 'direction', hooksStatus: 'ready', hooks: action.hooks, error: '', errorScope: null }
    case 'hook-selected': return { ...state, selectedHookId: action.id, generationDraft: null, ...clearFramesAndVideo, error: '' }
    case 'duration-changed': return { ...state, duration: action.duration, generationDraft: null, ...clearVideo }
    case 'instruction-changed': return { ...state, extraInstruction: action.value, generationDraft: null, ...clearFramesAndVideo }
    case 'draft-ready': return { ...state, generationDraft: action.draft, error: '' }
    case 'frames-started': return { ...state, firstFramesStatus: 'loading', firstFrames: [], selectedFirstFrameId: '', ...clearVideo, error: '' }
    case 'frames-ready': return { ...state, activeStep: 'first-frame', firstFramesStatus: 'ready', firstFrames: action.frames, error: '' }
    case 'frame-selected': return { ...state, selectedFirstFrameId: action.id, activeStep: 'video', ...clearVideo }
    case 'video-started': return { ...state, activeStep: 'video', videoStatus: 'loading', videoStage: 0, output: null, savedAssetId: '', error: '' }
    case 'video-progress': return { ...state, videoStage: action.stage }
    case 'video-ready': return { ...state, videoStatus: 'ready', videoStage: 3, output: action.output, error: '' }
    case 'output-saved': return { ...state, savedAssetId: action.assetId, error: '' }
    case 'operation-failed': return {
      ...state,
      analysisStatus: action.scope === 'analysis' ? 'error' : state.analysisStatus,
      hooksStatus: action.scope === 'hooks' ? 'error' : state.hooksStatus,
      firstFramesStatus: action.scope === 'frames' ? 'error' : state.firstFramesStatus,
      videoStatus: action.scope === 'video' ? 'error' : state.videoStatus,
      error: action.message,
      errorScope: action.scope,
    }
    case 'reset': return createInitialCommercePrerollState()
  }
}

export function canOpenCommercePrerollStep(state: CommercePrerollState, step: CommercePrerollStep) {
  if (step === 'source') return true
  if (step === 'understanding') return Boolean(state.source && state.rightsConfirmed)
  if (step === 'direction') return state.hooksStatus === 'ready'
  if (step === 'settings') return Boolean(state.selectedHookId)
  if (step === 'first-frame') return state.firstFramesStatus === 'ready'
  return Boolean(state.selectedFirstFrameId) || state.videoStatus === 'loading' || state.videoStatus === 'ready'
}
