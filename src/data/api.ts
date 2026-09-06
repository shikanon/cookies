import {
  attachKanonBriefProductAsset,
  createKanonCommercePrerollVideo,
  createKanonBrief,
  createKanonPreparedCommercePrerollVideo,
  createKanonMedia,
  createKanonProject,
  confirmKanonBrief,
  ensureKanonGuerlainCommerceFixtureAssets,
  getKanonCapabilities,
  getKanonJob,
  listKanonArtifacts,
  listKanonCommercePrerollSources,
  listKanonJobs,
  listKanonProjects,
  listKanonTasks,
  loadKanonAgencyWorkbench,
  prepareKanonCommercePreroll,
  unsupportedKanonWrite,
} from '../backend/kanon-api.js'
import type { CreativeIntakeStatus, CreativeTaskStatus } from '../contracts/creative'
// 纯类型的循环引用：verdict.ts 反过来从这里取 ApiConfidenceLevel。
// import type 会被 TS 完全擦除，运行时不成环。
import type { Judgement, UpgradePath, Verdict } from './verdict'
import { platformClient } from './platformClient.js'
import { uploadProjectAssetFile } from './projectAssetUpload.js'

export type ApiProject = {
  id: string
  name: string
  brand: string
  objective: string
  industry?: 'short_drama' | 'game' | 'ecommerce' | 'automotive_brand'
  runtime: {
    code: string
    product: string
    stage: string
    progress: number
    status: 'active' | 'completed'
    owner: string
    budget: number
    currency: 'CNY'
    timezone: 'Asia/Shanghai'
  }
  version: number
  createdAt: string
  updatedAt: string
  products?: Array<{ id: string; name: string; oceanEngineProductId?: string }>
}

export type ApiPublicInsightIndustryStat = {
  name: string
  count: number
  views: number
}

export type ApiPublicInsightOverview = {
  total_videos: number
  total_views: number
  average_like_rate: number
  average_finish_rate: number
  ai_ratio: number
  industries: ApiPublicInsightIndustryStat[]
  files: Array<{ filename: string; row_count: number; modified_at: string }>
  loaded_at: string
  data_dir: string
}

export type ApiPublicInsightFilterOption = {
  value: string
  count: number
}

export type ApiPublicInsightFilters = {
  industries: ApiPublicInsightFilterOption[]
  visual_styles: ApiPublicInsightFilterOption[]
  ai_types: string[]
  date_range: { min: string; max: string }
}

export type ApiPublicInsightVideoListItem = {
  item_id: string
  url: string
  frame_first: string
  item_title: string
  item_create_day: string
  author_cert_type: string
  vv_all: number
  like_cnt_all: number
  comment_cnt_all: number
  share_cnt_all: number
  favourite_cnt_all: number
  finish_vv_all: number
  ctr: string
  bounce_rate_map: string
  has_ai_generated: string
  industry: string
  date: string
  finish_rate: number
  like_rate: number
  playback_url: string
}

export type ApiPublicInsightVideoDetail = ApiPublicInsightVideoListItem & {
  storyboard_structure: string
  ai_creative_type: string
  item_asr: string
  item_ocr: string
  first3s_visual_creative_type: string
  main_visual_elements: string
  shooting_scene: string
  characters_relation: string
  mentioned_brand: string
  oral_product_desc: string
  bgm_style: string
  bgm_bpm: string
  bgm_emotion: string
  voice_type: string
  speech_speed: string
  oral_script: string
  storyboard_prompt: string
  visual_style: string
  creative_highlight: string
  source_file: string
  storyboard: unknown[]
}

export type ApiPublicInsightVideoPage = {
  items: ApiPublicInsightVideoListItem[]
  total: number
  page: number
  page_size: number
  pages: number
}

export type ApiAgencyHealthStatus = 'healthy' | 'watch' | 'blocked'
export type ApiAdPlatform = '巨量引擎' | '腾讯广告' | '快手磁力'
export type ApiBindingHealthStatus = 'normal' | 'warning' | 'expired'
export type ApiProjectProgressStage = 'intake' | 'strategy' | 'creative' | 'quality_check' | 'human_review' | 'delivery' | 'completed'
export type ApiQualityCheckStatus = 'queued' | 'running' | 'passed' | 'failed'
export type ApiQualityIssueSeverity = 'minor' | 'major' | 'critical'
export type ApiMaterialConfirmationStatus = 'confirmed' | 'changes_requested'

export type ApiOrganization = {
  id: string
  code: string
  name: string
  owner: string
  currency: 'CNY'
  timezone: 'Asia/Shanghai'
  updatedAt: string
}

export type ApiClient = {
  id: string
  organizationId: string
  code: string
  name: string
  industry: string
  owner: string
  healthStatus: ApiAgencyHealthStatus
  updatedAt: string
}

export type ApiBrand = {
  id: string
  organizationId: string
  clientId: string
  code: string
  name: string
  category: string
  productLines: string[]
  owner: string
  guidelineStatus: 'ready' | 'missing' | 'outdated'
  updatedAt: string
}

export type ApiAdAccountBinding = {
  id: string
  organizationId: string
  clientId: string
  brandId: string
  projectIds: string[]
  platform: ApiAdPlatform
  accountName: string
  accountDisplayId: string
  currency: 'CNY'
  timezone: 'Asia/Shanghai'
  permissionStatus: ApiBindingHealthStatus
  loginStatus: ApiBindingHealthStatus
  trackingStatus: ApiBindingHealthStatus
  owner: string
  boundAssetIds: string[]
  lastSyncedAt: string
}

export type ApiProjectProgress = {
  stage: ApiProjectProgressStage
  stageLabel: string
  stagePercent: number
  taskPercent: number
  riskStatus: ApiAgencyHealthStatus
  blocker?: string
  updatedAt: string
}

export type ApiAgencyProject = ApiProject & {
  organizationId: string
  clientId: string
  brandId: string
  progressDetail: ApiProjectProgress
}

export type ApiQualityCheckIssue = {
  id: string
  severity: ApiQualityIssueSeverity
  rule: string
  evidence: string
  suggestion: string
}

export type ApiQualityCheckRun = {
  id: string
  organizationId: string
  projectId: string
  assetId: string
  assetVersion: number
  status: ApiQualityCheckStatus
  model: string
  ruleVersion: string
  promptVersion: string
  summary: string
  issues: ApiQualityCheckIssue[]
  createdAt: string
  completedAt?: string
}

export type ApiMaterialConfirmation = {
  id: string
  organizationId: string
  projectId: string
  qualityCheckRunId: string
  assetId: string
  assetVersion: number
  status: ApiMaterialConfirmationStatus
  scope: string
  confirmedBy: string
  note: string
  createdAt: string
}

export type ApiAssetVersionRecord = {
  version: number
  createdBy: string
  sourceTaskId: string
  sourceType: 'model_generation' | 'manual_edit'
  sourceLabel: string
  createdAt: string
  changeSummary: string
}

export type ApiAssetAuthorizationScope = {
  platforms: ApiAdPlatform[]
  regions: string[]
  rightsHolder: string
  expiresAt: string
  note: string
}

export type ApiAssetVersionPointer = {
  id: string
  organizationId: string
  projectId: string
  assetId: string
  mediaKind?: 'image' | 'video' | 'audio'
  contentUrl?: string
  sourceJobId?: string
  workingVersion: number
  qualityCheckedVersion?: number
  humanConfirmedVersion?: number
  deliveryVersion?: number
  versions: ApiAssetVersionRecord[]
  authorization: ApiAssetAuthorizationScope
  deliveryTarget: {
    platform: ApiAdPlatform
    region: string
  }
  owner: string
  updatedAt: string
  oceanEngineMaterialId?: string
}

export type ApiAgencyWorkbench = {
  organizations: ApiOrganization[]
  clients: ApiClient[]
  brands: ApiBrand[]
  projects: ApiAgencyProject[]
  adAccountBindings: ApiAdAccountBinding[]
  qualityCheckRuns: ApiQualityCheckRun[]
  materialConfirmations: ApiMaterialConfirmation[]
  assetVersionPointers: ApiAssetVersionPointer[]
}

export type ApiArtifact = {
  id: string
  projectId: string
  kind: 'brief' | 'image' | 'video' | 'document'
  purpose?: ApiVideoPurpose
  prerollType?: ApiPrerollType
  shortDramaPreroll?: ApiShortDramaPrerollSnapshot
  status: 'draft' | 'ready' | 'archived'
  content: string
  sourceJobId?: string
  briefTaskId?: string
  briefDraftVersion?: number
  version: number
  createdAt: string
  updatedAt: string
}

export type ApiProjectMediaAsset = {
  id: string
  projectId: string
  version: number
  kind: 'video' | 'image' | 'audio' | 'document'
  sourceType?: 'upload' | 'provider_generated' | 'imported' | 'captured' | 'rendered'
  mimeType: string
  sizeBytes: number
  durationSeconds?: number
  width?: number
  height?: number
  createdAt: string
  contentUrl: string
  rightsStatus?: 'unverified' | 'active' | 'revoked'
  useAllowed?: boolean
  useDenialCode?: string
}

export type ApiAssetFeature = {
  id: string
  organizationId: string
  projectId: string
  assetId: string
  assetVersion: number
  schemaVersion: 'asset_feature_v1'
  featureVersion: string
  hookStrength: number
  productVisibility: number
  sceneTags: string[]
  productTags: string[]
  personTags: string[]
  actionTags: string[]
  emotionTags: string[]
  sellingPoints: string[]
  ctaPresence: boolean
  similarityGroup?: string
  similarityRisk: 'low' | 'medium' | 'high'
  evidence: string[]
  version: number
  createdAt: string
  updatedAt: string
}

export type ApiGenerationJob = {
  id: string
  projectId: string
  artifactKind: ApiArtifact['kind']
  purpose?: ApiVideoPurpose
  prerollType?: ApiPrerollType
  briefArtifactId?: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'
  model?: string
  diagnostic?: string
  artifactId?: string
  version: number
  createdAt: string
  updatedAt: string
}

export type ApiCommerceTemplateId =
  | 'commerce.product-cut'
  | 'commerce.window-reveal'
  | 'commerce.one-click'
  | 'commerce.miniature'
  | 'commerce.device-summon'

export type ApiCreativeSourceRef = {
  kind: 'confirmed_brief' | 'strategy_package'
  id: string
  version: number
  content_hash: string
}

export type ApiCommerceProductFacts = {
  brand_name: string
  product_name: string
  product_category?: string
  selling_points: string[]
  tone: string[]
  visual_keywords: string[]
  mandatory_elements: string[]
  prohibited_claims: string[]
  product_asset_refs: Array<{ asset_id: string; version: number }>
}

export type ApiCreativeSourceOption = {
  source_ref: ApiCreativeSourceRef
  status: 'confirmed' | 'approved'
  product: ApiCommerceProductFacts
  confirmed_at: string
  preferred: boolean
}

export type ApiTaskStrategyCreativeIntake = {
  id: string
  source: 'task_strategy'
  status: 'draft' | CreativeIntakeStatus
  request: {
    format: 'image_text' | 'video'
    performance_mode?: string
    objective: string
    audience: string
    core_message: string
    concept: string
    task_strategy_input: {
      business_code: string
      business_strategy: Record<string, unknown>
      guardrails: string[]
      open_questions: string[]
      media: Array<{
        asset_ref: ApiAssetVersionRef
        role: string
        kind?: string
        status: string
        usefulness: string
      }>
      reference_use: {
        locator?: string
        rights_status: string
        intended_use: string
        warnings: string[]
      }
    }
  }
  missing_fields: string[]
  warnings: string[]
}

export type ApiCreativeTaskHandoffDetail = {
  task: {
    id: string
    intake_id: string
    format: 'image_text' | 'video'
    channel: string
    status: string
    direction: {
      focus: string
      audience: string
      core_message: string
      call_to_action: string
    }
  }
  intake: ApiTaskStrategyCreativeIntake
}

export type ApiImageTextAttemptStatus =
  | 'queued'
  | 'running'
  | 'base_asset_ready'
  | 'rendering'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'stale'

export type ApiImageTextAttempt = {
  contract_version: 'creative-image-generation-attempt/v1'
  id: string
  draft_revision: number
  image_plan_order: number
  attempt_no: number
  provider_job_id?: string
  status: ApiImageTextAttemptStatus
  final_asset_ref?: ApiAssetVersionRef
  error_code?: string
  error_message?: string
  created_at: string
}

export type ApiImageTextWorkspace = {
  contract_version: 'creative-image-text-workspace/v1'
  task: {
    id: string
    version: number
    status: CreativeTaskStatus
    direction: {
      direction_version_id: string
      focus: string
      audience: string
      core_message: string
      call_to_action: string
    }
  }
  intake: ApiTaskStrategyCreativeIntake
  direction: {
    direction_id: string
    concept: string
    creative_rationale: string
  }
  draft: {
    contract_version?: string
    version: number
    generation_source_version?: number
    selected_title?: string
    title_candidates: string[]
    body: string
    topics: string[]
    image_plan: Array<{
      order: number
      role?: 'cover' | 'proof' | 'cta'
      purpose: string
      visual_brief: string
      caption: string
      overlay_copy?: string
      layout_preset?: string
      asset_ref?: ApiAssetVersionRef
    }>
  }
  slots: Array<{
    order: number
    role: 'cover' | 'proof' | 'cta'
    status?: ApiImageTextAttemptStatus
    adopted_attempt_id?: string
    selection_version: number
    attempts: ApiImageTextAttempt[]
  }>
  readiness: {
    draft_generation_ready: boolean
    image_generation_ready: boolean
    review_ready: boolean
    blocking_reasons: string[]
  }
}

export type ApiCreativeDirection = {
  contract_version: 'creative-direction/v1'
  direction_id: string
  concept: string
  creative_rationale: string
  message_plan: string[]
  execution_outline: string[]
  guardrail_trace: string[]
  direction_mode?: 'emotional' | 'cinematic' | 'utility'
  emotional_arc?: string
  visual_grammar?: string
  brand_memory_device?: string
  human_moment?: string
  status: 'candidate' | 'confirmed' | 'superseded'
}

export type ApiCreativeDirectionBatch = {
  contract_version: 'creative-direction-candidate-batch/v1'
  batch_id: string
  intake_id: string
  status: 'generating' | 'ready' | 'failed'
  candidates: ApiCreativeDirection[]
  prompt_version?: string
  failure_code?: string
  brand_brief_ref?: { revision: number; content_hash: string }
  created_at: string
}

export type ApiBrandBriefReview = {
  contract_version: 'creative-brand-brief-review/v1'
  intake_id: string
  input_identity_hash: string
  status: 'draft' | 'confirmed'
  revision: number
  document: {
    summary: string
    market: string
    language: string
    objective: { objective_type: string; statement: string; success_signals: string[] }
    audience_segments: Array<{
      segment_id: string
      label: string
      priority: number
      insight: string
      tension: string
      evidence_ref_ids: string[]
    }>
    product: {
      product_ref_ids: string[]
      brand_name: string
      product_name: string
      selling_points: string[]
      proof_points: string[]
      usage_scenarios: string[]
      campaign_mechanism: string
      offer_text: string
      landing_destination: string
    }
    communication: {
      single_minded_proposition: string
      message_hierarchy: Array<{ priority: number; message: string; evidence_ref_ids: string[] }>
      cta_intent: string
      approved_ctas: string[]
      tone_constraints: string[]
    }
    guardrails: Array<{ guardrail_id: string; kind: string; severity: string; scope: string; text: string; source_ref_ids: string[] }>
    claims: Array<{ claim_id: string; approved_text: string; evidence_ref_ids: string[]; required_disclaimer: string }>
    assets: Array<{
      asset_ref: { asset_id: string; version: number }
      role: string
      rights: { status: string; generative_ai_allowed: boolean; derivative_work_allowed: boolean }
    }>
    route: {
      route_id: string
      channels: string[]
      reason: string
      spec: { target_duration_seconds: number; aspect_ratio: string; resolution: string }
      cta_policy: { cta_intent: string; required_for_generation: boolean; required_for_delivery: boolean }
      claim_refs: string[]
      asset_requirements: Array<{ role: string; required_stage: string }>
    }
    audio_intent: {
      narration_required: boolean | null
      voice_direction: string
      overall_mood: string
      music_required: boolean | null
      sound_effects_required: boolean | null
    }
    open_questions: Array<{ code: string; stage: string; message: string }>
    source_refs: Array<{ ref_id: string; ref_type: string; producer: string; version: string; content_hash: string }>
    creative_notes: string[]
  }
  blockers: string[]
  warnings: string[]
  content_hash: string
  confirmed_by?: string
  confirmed_at?: string
}

export type ApiCreativeIntakeBootstrap = {
  id: string
  source: string
  status: string
  input_identity_hash?: string
  selected_route_id?: string
  request?: {
    objective?: string
    audience?: string
    core_message?: string
    concept?: string
    selected_route_id?: string
    creative_routes?: Array<{
      route_id: string
      route_type?: string
      channels: string[]
    }>
  }
  base_handoff?: {
    routes?: Array<{
      route_id: string
      deliverable_type?: string
      purpose?: string
      performance_mode?: string
      channels: string[]
    }>
    creative_view?: {
      objective?: { statement?: string }
      communication?: { single_minded_proposition?: string }
    }
  }
}

/**
 * 一个创意版本。字段只取洞察这边用得上的那几个，完整结构在
 * internal/systems/creative/model.go 的 CreativeVersion。
 *
 * 洞察要它只为一件事：把创意组已经批准的素材登记进分析索引。批准过的版本是
 * 不可变的，所以拿它当分析对象不会出现「分析完了原件又改了」。
 */
export type ApiCreativeVersionSummary = {
  id: string
  creative_task_id: string
  format: 'image_text' | 'video'
  version: number
  status: 'created' | 'checked' | 'approved' | 'superseded'
  created_at: string
  snapshot?: { selected_title?: string; title_candidates?: string[] }
  // 视频版本才有成品文件引用。图文版本的正文和配图存在 snapshot 里，
  // 没有单一的媒体资产 ID——所以导进来的图文素材不带 platform_asset_id。
  video_snapshot?: { final_video?: { asset_id: string; version: number } }
  approval?: { approved_by?: string; approved_at?: string }
}

export type ApiCreativeTaskSummary = {
  id: string
  display_name: string
  organization_id: string
  project_id: string
  intake_id: string
  format: 'image_text' | 'video'
  channel: string
  video_purpose?: string
  performance_mode?: string
  status: string
  direction: {
    direction_version_id?: string
    input_identity_hash?: string
    content_type?: string
    focus: string
    audience: string
    core_message: string
    call_to_action: string
    concept: string
    tone: string[]
    visual_keywords: string[]
  }
  version: number
  created_at: string
  updated_at: string
}

export type ApiStrategyBrandWorkflow = {
  contract_version: 'creative-strategy-brand-workflow/v1'
  mode: 'brief_review_required' | 'direction_ready' | 'direction_selection_required' | 'task_ready' | 'legacy_task_upgrade_required'
  intake_id: string
  input_identity_hash: string
  brand_brief?: ApiBrandBriefReview
  latest_direction_batch?: ApiCreativeDirectionBatch
  confirmed_direction?: ApiCreativeDirection
  task?: ApiCreativeTaskSummary
  issues: Array<{ code: string; stage: string; path?: string; message: string; source: string }>
  next_action: 'prepare_brief' | 'review_brief' | 'generate_directions' | 'wait_for_directions' | 'retry_directions' | 'select_direction' | 'create_task' | 'open_task' | 'review_legacy_task'
}

export type ApiCreateManualImageTextInput = {
  objective: string
  audience: string
  coreMessage: string
  callToAction: string
  tone: string[]
  visualKeywords: string[]
  mandatoryElements: string[]
  prohibitedClaims: string[]
}

export type ApiCreativeVersion = {
  id: string
  creative_task_id: string
  draft_version: number
  status: 'created' | 'checked' | 'approved' | 'delivered'
  check?: { passed: boolean; blockers: string[]; warnings: string[] }
}

export type ApiCreativePackage = {
  id: string
  creative_version_id: string
  format: 'image_text' | 'video'
  content_hash: string
  created_at: string
}

export type ApiPreparedCommercePreroll = {
  contract_version: 'creative-commerce-preroll-preparation/v1'
  source_ref: ApiCreativeSourceRef
  product: ApiCommerceProductFacts
  plan: {
    template: {
      template_id: ApiCommerceTemplateId
      template_version: 1
    }
    frame_plan: {
      start_frame_kind: string
      tail_frame_kind: string
    }
    prompt: {
      fidelity: string
      camera: string
      environment: string
      timeline: Array<{
        start_seconds: number
        end_seconds: number
        purpose: 'information_gap' | 'single_transformation' | 'product_hold'
        instruction: string
      }>
      guardrails: string[]
      compiled_prompt: string
      prompt_hash: string
    }
  }
  readiness: {
    planning_ready: boolean
    generation_ready: boolean
    blockers: string[]
    warnings: string[]
  }
  prepared_at: string
}

export type ApiCommercePrerollWorkspace = {
  task: {
    id: string
    status: CreativeTaskStatus
    performance_mode: 'commerce_preroll'
    updated_at: string
  }
  intake: {
    id: string
    version: number
    request: {
      objective: string
      audience: string
      core_message: string
      call_to_action: string
      manual_commerce_preroll: {
        fixture_id: string
        fixture_version: number
        fixture_content_hash: string
        brand_name: string
        product_name: string
        product_category?: string
        selling_points: string[]
        visual_keywords: string[]
        product_asset_ref: ApiAssetVersionRef
        first_frame_asset_ref: ApiAssetVersionRef
        last_frame_asset_ref: ApiAssetVersionRef
        template_ref: { template_id: ApiCommerceTemplateId; template_version: 1 }
      }
    }
  }
  video_draft: {
    revision: number
    prompt: string
    commerce_preroll: {
      contract_version: 'creative-commerce-preroll-draft/v1'
      revision: number
      input_hash: string
      input_snapshot: {
        fixture_id: string
        fixture_version: number
        fixture_content_hash: string
        brand_name: string
        product_name: string
        product_category?: string
        selling_points: string[]
        visual_keywords: string[]
        mandatory_elements: string[]
        prohibited_claims: string[]
        product_asset_ref: ApiAssetVersionRef
        first_frame_asset_ref: ApiAssetVersionRef
        last_frame_asset_ref: ApiAssetVersionRef
      }
      plan: {
        template: { template_id: ApiCommerceTemplateId; template_version: 1 }
        frame_plan: {
          start_frame_kind: string
          tail_frame_kind: string
          product_asset_ref: ApiAssetVersionRef
        }
        prompt: {
          prompt_version: number
          fidelity: string
          camera: string
          environment: string
          timeline: Array<{
            start_seconds: number
            end_seconds: number
            purpose: 'information_gap' | 'single_transformation' | 'product_hold'
            instruction: string
          }>
          guardrails: string[]
          compiled_prompt: string
          prompt_hash: string
        }
        generation_spec: {
          generation_spec_hash: string
          duration_seconds: 6
          aspect_ratio: '9:16'
          resolution: '720p'
          generation_ready: boolean
          production_ready: boolean
        }
      }
      approval?: {
        generation_spec_hash: string
        confirmed_by: string
        confirmed_at: string
      }
      readiness: {
        planning_ready: boolean
        generation_ready: boolean
        production_ready: boolean
        missing_fields: string[]
        blockers: string[]
      }
    }
  }
  commerce_preroll_generation_attempts?: Array<{
    id: string
    task_id: string
    draft_revision: number
    template_ref: { template_id: ApiCommerceTemplateId; template_version: 1 }
    prompt_hash: string
    generation_spec_hash: string
    provider_job_id: string
    retry_of_attempt_id?: string
    output_asset_version?: ApiAssetVersionRef
    created_at: string
  }>
}

export type ApiBrandBriefFact = {
  text: string
  locator: string
  confidence: number
  status: 'brief_fact' | 'needs_confirmation'
}

export type ApiBrandBriefAssetCandidate = {
  id: string
  role: string
  label: string
  source_locator: string
  fixture_uri?: string
  asset_ref?: ApiAssetVersionRef
  rights_status: string
  user_confirmed: boolean
  replacement_note?: string
}

export type ApiBrandBriefAnalysis = {
  revision: number
  summary: string
  audience: string
  core_message: string
  selling_points: ApiBrandBriefFact[]
  mandatory_elements: string[]
  prohibited_claims: string[]
  image_requirements: string[]
  video_requirements: string[]
  voice_direction: string
  asset_candidates: ApiBrandBriefAssetCandidate[]
  uncertainties: string[]
  confirmed: boolean
  confirmed_by?: string
  confirmed_at?: string
  model_alias: string
  model_version: string
  route_revision_id?: string
  prompt_version: string
  created_at: string
}

export type ApiBrandCreativeConcept = {
  id: string
  title: string
  one_liner: string
  story_mechanism: string
  brand_entrance: string
  visual_language: string[]
  sound_idea: string
  brief_rationale: string
  risk: string
  selected: boolean
  confirmed: boolean
}

export type ApiBrandFilmShot = {
  id: string
  order: number
  start_second: number
  end_second: number
  purpose: string
  visual: string
  action: string
  camera: string
  lighting: string
  voiceover: string
  on_screen_text: string
  reference_role: string
  continuity_notes: string
}

export type ApiBrandFilmPlan = {
  revision: number
  master_duration_ms?: number
  concept_id: string
  title: string
  story_summary: string
  voice_direction: string
  music_direction: string
  shots: ApiBrandFilmShot[]
  confirmed: boolean
  confirmed_by?: string
  confirmed_at?: string
  model_alias: string
  model_version: string
  route_revision_id?: string
  prompt_version: string
  created_at: string
}

export type ApiBrandAudioClip = {
  id: string
  track_id: string
  order: number
  fixture_uri?: string
  asset_ref?: ApiAssetVersionRef
  label: string
  timeline_start_ms: number
  timeline_end_ms: number
  gain_db: number
  fade_in_ms: number
  fade_out_ms: number
  narration_source_ref?: { plan_revision: number; shot_id: string; voiceover_hash: string }
  cue_ref?: string
  generation_attempt_id?: string
  waveform_peaks?: number[]
  word_timings?: Array<{ text: string; begin_ms: number; end_ms: number }>
}

export type ApiBrandAudioTrack = {
  id: string
  type: 'source_audio' | 'voiceover' | 'music' | 'sfx'
  role: string
  muted: boolean
  solo: boolean
  gain_db: number
  locked: boolean
  rights_status: string
  clips: ApiBrandAudioClip[]
}

export type ApiBrandAudioWorkspace = {
  contract_version: 'creative-brand-audio-workspace/v1'
  plan_revision: number
  master_duration_ms: number
  visual_preview_asset_ref: ApiAssetVersionRef
  blueprint_versions: Array<{
    revision: number
    plan_revision: number
    master_duration_ms: number
    voice_profile: { voice_alias: string; language: string; direction: string; speed: number; volume: number; pitch: number; emotion: string }
    narration_cues: Array<{ id: string; shot_id: string; start_ms: number; end_ms: number; text: string; reason: string; confidence: number; estimated_duration_ms: number; available_duration_ms: number; fit_status: 'fits' | 'spacious' | 'overrun'; suggested_text?: string }>
    music_arc: { start_ms: number; end_ms: number; direction: string }
    sound_effect_cues: Array<{ id: string; shot_id: string; start_ms: number; end_ms: number; label: string; reason: string }>
    pronunciations: Array<{ term: string; spoken_as: string; reason: string }>
    director_decisions: Array<{ id: string; kind: string; target_id: string; summary: string; reason: string; confidence: number; editable: boolean }>
    semantic_checks: Array<{ id: string; shot_id: string; status: 'pass' | 'warning'; summary: string; evidence: string; suggestion?: string }>
    planner_version: string
    status: string
    content_hash: string
  }>
  variants: Array<{
    id: string
    label: string
    variant_type: 'tone' | 'language' | 'custom'
    language: string
    style_preset: string
    mix_versions: Array<{
      id: string
      revision: number
      content_hash: string
      master_duration_ms: number
      status: string
      tracks: ApiBrandAudioTrack[]
    }>
    active_mix_revision: number
    status: string
  }>
  active_variant_id: string
  active_mix_revision: number
  mixed_preview_asset_ref?: ApiAssetVersionRef
  final_mixed_asset_ref?: ApiAssetVersionRef
  generation_attempts: Array<{
    id: string
    clip_id: string
    ordinal: number
    retry_of?: string
    status: 'succeeded' | 'failed'
    provider?: string
    provider_snapshot?: string
    provider_job_id?: string
    output_asset_ref?: ApiAssetVersionRef
    fixture_mode: boolean
    error_code?: string
    error_message?: string
  }>
  render_jobs: Array<{
    id: string
    mix_revision: number
    mix_content_hash: string
    kind: 'preview' | 'final'
    status: 'queued' | 'running' | 'succeeded' | 'failed'
    renderer_version: string
    output_asset_ref?: ApiAssetVersionRef
    error_code?: string
    error_message?: string
  }>
  status: string
  updated_at: string
}

export type ApiSpeechCapability = {
  provider: string
  model?: string
  voice_id?: string
  available: boolean
  error_code?: string
  error_message?: string
  voice_aliases: string[]
}

export type ApiBrandAudioMixOperation =
  | { op: 'set_track_gain'; track_id: string; gain_db: number }
  | { op: 'set_track_muted'; track_id: string; muted: boolean }
  | { op: 'replace_clip_asset'; clip_id: string; asset_ref: ApiAssetVersionRef }
  | { op: 'set_clip_timing'; clip_id: string; timeline_start_ms: number; timeline_end_ms: number }

export type ApiBrandFilmGenerationAttempt = {
  id: string
  ordinal: number
  prompt_hash: string
  provider_job_id: string
  retry_of?: string
  feedback?: string
  status: string
  output_asset_ref?: ApiAssetVersionRef
  error_message?: string
  created_at: string
  updated_at: string
}

export type ApiBrandFilmGenerationUnit = {
  id: string
  order: number
  shot_ids: string[]
  start_second: number
  end_second: number
  prompt_packages: Array<{
    revision: number
    content_hash: string
    feedback?: string
    duration_seconds: number
    composite_prompt: string
  }>
  attempts: ApiBrandFilmGenerationAttempt[]
  locked_attempt_id?: string
}

export type ApiBrandFilmManualCheck = {
  code: 'product_fidelity' | 'brand_logo_packaging' | 'subtitle_voiceover' | 'sound_music'
  passed: boolean
  note?: string
  unit_id?: string
}

export type ApiBrandFilmQualityRun = {
  id: string
  revision: number
  preview_asset: ApiAssetVersionRef
  status: 'failed' | 'awaiting_human' | 'passed'
  checks: Array<{
    code: string
    category: 'shot' | 'technical' | 'copy' | 'audio' | 'brand'
    scope: string
    passed: boolean
    severity: 'info' | 'blocking'
    evidence: string
    repair_advice?: string
  }>
  manual_checks: ApiBrandFilmManualCheck[]
  metrics: {
    unit_count: number
    attempt_count: number
    succeeded_attempts: number
    failed_attempts: number
    regeneration_count: number
    success_rate: number
    availability_rate: number
    regeneration_reasons: Record<string, number>
  }
  automatic_passed: boolean
  human_confirmed: boolean
  human_confirmed_by?: string
  human_confirmed_at?: string
  created_at: string
  updated_at: string
}

export type ApiBrandFilmWorkspace = {
  task: {
    id: string
    display_name: string
    status: string
    performance_mode: 'brand_video'
    version: number
    updated_at: string
  }
  intake: {
    id: string
    version: number
  }
  video_draft: {
    revision: number
    brand_film: {
      contract_version: 'creative-brand-film-draft/v1'
      revision: number
      stage: 'waiting_for_input' | 'brief_analysis_draft' | 'brief_confirmed' | 'concept_selection' | 'concept_confirmed' | 'production_plan_draft' | 'production_plan_confirmed' | 'generation_ready' | 'generating' | 'generation_review' | 'generation_locked' | 'audio_draft' | 'quality_review' | 'ready_for_review' | 'approved' | 'delivered'
      source_snapshot: {
		source_type?: 'strategy_handoff'
		fixture_id?: string
		fixture_version?: number
		fixture_hash?: string
		source_kind?: 'fixture' | 'strategy_package' | 'task_strategy' | 'manual_document' | 'manual'
        intake_id?: string
        input_identity_hash?: string
        strategy_package_id?: string
        strategy_package_version?: number
		strategy_package_hash?: string
        handoff_contract_version?: string
        handoff_content_hash?: string
        brand_brief_revision?: number
        brand_brief_content_hash?: string
        direction_batch_id?: string
        direction_id?: string
        direction_version?: number
		direction_content_hash?: string
		route_id?: string
        brief_name: string
        brief_text: string
        product_name: string
        channel: string
        duration_seconds: number
        aspect_ratio: string
        resolution?: string
        evidence_refs: string[]
      }
      brief_analysis_versions: ApiBrandBriefAnalysis[] | null
      concept_sets: Array<{
        revision: number
        analysis_revision: number
        candidates: ApiBrandCreativeConcept[]
        model_alias: string
        model_version: string
        route_revision_id?: string
        prompt_version: string
        created_at: string
      }> | null
      selected_concept_id?: string
      film_plan_versions: ApiBrandFilmPlan[] | null
      readiness: {
        planning_ready: boolean
        generation_ready: boolean
        production_ready: boolean
        blockers: string[]
      }
      generation_seam: {
        contract_version: 'creative-brand-generation-seam/v1'
        unit_policy: string
        prompt_contract: string
        attempt_policy: string
      }
      generation?: {
        contract_version: 'creative-brand-film-generation/v1'
        plan_revision: number
        master_duration_ms?: number
        reference_asset: ApiAssetVersionRef
        units: ApiBrandFilmGenerationUnit[]
        preview_asset?: ApiAssetVersionRef
        created_at: string
        updated_at: string
      }
      audio?: ApiBrandAudioWorkspace
      quality_runs: ApiBrandFilmQualityRun[] | null
      delivery?: {
        quality_run_id: string
        creative_version_id?: string
        creative_package_id?: string
        approved_by?: string
        approved_at?: string
        delivered_by?: string
        delivered_at?: string
      }
      updated_at: string
    }
  }
}

export type ApiVideoPurpose = 'preroll'
export type ApiPrerollType = 'short_drama' | 'game' | 'commerce'

export type ApiShortDramaStoryContext = {
  title: string
  synopsis: string
  reviewedSellingPoints: string[]
  openingLine?: string
}

export type ApiShortDramaPrerollCandidate = {
  id: string
  hookType: 'conflict' | 'reversal' | 'suspense' | 'selling_point_bridge'
  executionAngle: 'dialogue_confrontation' | 'action_reveal' | 'reaction_escalation' | 'result_first'
  executionAngleLabel: string
  score: number
  scoreMeaning: 'editorial_quality_heuristic'
  evidence: string[]
  primaryTestVariable: string
  pacingProfile: string
  visualGrammar: string
  variantHypothesis: string
  hookLine: string
  voiceover: string
  storyboard: Array<{ startSeconds: number; endSeconds: number; visual: string; copy: string }>
  visualIntent: string
  transitionLine: string
  promptPackage: {
    compiledPrompt: string
    contentHash: string
    directorSpec: Record<string, string>
    candidateBatchId?: string
    promptCompilerVersion?: string
    generationConfig: ApiShortDramaGenerationConfig
    subtitleSpec: {
      mode: string
      max_lines: number
      safe_area: string
      keyword_emphasis: boolean
      animation_density: string
      contrast_policy: string
    }
  }
}

export type ApiShortDramaPrerollPlan = {
  version: 'short_drama_preroll_v1'
  candidates: ApiShortDramaPrerollCandidate[]
}

export type ApiShortDramaHookStrategy = 'conflict_reversal' | 'suspense_reveal' | 'identity_contrast' | 'selling_point_bridge'
export type ApiShortDramaSubtitleStyle = 'high_contrast_dynamic' | 'brand_minimal'
export type ApiShortDramaPaceProfile = 'auto' | 'punchy' | 'balanced' | 'suspense_hold'
export type ApiShortDramaVariationIntent = 'balanced' | 'more_visual' | 'more_dialogue' | 'more_suspense'

export type ApiShortDramaGenerationConfig = {
  subtitle_style: ApiShortDramaSubtitleStyle
  hook_strength: number
  pace_profile: ApiShortDramaPaceProfile
}

type ApiShortDramaCandidateWire = {
  id: string
  hook_strategy: ApiShortDramaHookStrategy
  execution_angle: ApiShortDramaPrerollCandidate['executionAngle']
  primary_test_variable?: string
  pacing_profile?: string
  visual_grammar?: string
  variant_hypothesis?: string
  score: number
  score_meaning: 'editorial_quality_heuristic'
  evidence: string[]
  hook_line: string
  voiceover: string
  storyboard: Array<{ start_seconds: number; end_seconds: number; visual: string; copy: string }>
  visual_intent: string
  transition_line: string
  prompt_package: {
    compiled_prompt: string
    content_hash: string
    director_spec: Record<string, string>
    candidate_batch_id?: string
    prompt_compiler_version?: string
    generation_config?: ApiShortDramaGenerationConfig
    subtitle_spec?: ApiShortDramaPrerollCandidate['promptPackage']['subtitleSpec']
  }
}

export type ApiShortDramaPrerollWorkspace = {
  task: { id: string; performance_mode: 'short_drama_preroll'; status: string }
  video_draft: {
    revision: number
    short_drama_preroll: {
      revision: number
      selected_candidate_id?: string
      input_snapshot: {
        brief_id: string
        brief_version: number
        brief_name: string
        story_title: string
        synopsis: string
        reviewed_selling_points: string[]
        opening_line?: string
        hook_strategy: ApiShortDramaHookStrategy
        subtitle_style: ApiShortDramaSubtitleStyle
        transition: 'hard_cut' | 'action_match' | 'audio_bridge'
        hook_strength: number
        pace_profile?: ApiShortDramaPaceProfile
        call_to_action: string
      }
      readiness: { planning_ready: boolean; generation_ready: boolean; production_ready: boolean; blockers: string[] }
      active_candidate_batch?: {
        id: string
        revision: number
        planner_version: string
        prompt_compiler_version: string
        diversity_nonce: string
        generation_config: ApiShortDramaGenerationConfig
        variation_intent: ApiShortDramaVariationIntent
        generated_candidate_count: number
        candidates: ApiShortDramaCandidateWire[]
        created_at: string
      }
      candidates: ApiShortDramaCandidateWire[]
    }
  }
  short_drama_generation_attempts?: Array<{
    id: string
    task_id: string
    draft_revision: number
    candidate_batch_id: string
    candidate_id: string
    prompt_package_hash: string
    generation_spec_hash: string
    provider_job_id: string
    output_asset_version?: ApiAssetVersionRef
    created_at: string
  }>
}

export type ApiCreateManualShortDramaPrerollInput = {
  parentIntakeId?: string
  briefId: string
  briefVersion: number
  briefName: string
  title: string
  synopsis: string
  reviewedSellingPoints: string[]
  openingLine?: string
  hookStrategy: ApiShortDramaHookStrategy
  subtitleStyle: ApiShortDramaSubtitleStyle
  transition: 'hard_cut' | 'action_match' | 'audio_bridge'
  hookStrength: number
  paceProfile: ApiShortDramaPaceProfile
  objective: string
  audience: string
  prohibitedClaims: string[]
  callToAction: string
}

export type ApiShortDramaV2ProjectAssetRef = {
  project_id: string
  asset_version: ApiAssetVersionRef
}

export type ApiShortDramaV2AnalysisContent = {
  title: string
  episode?: string
  synopsis: string
  opening_beat: string
  core_conflict: string
  unresolved_hook: string
  tone: string
  characters: Array<{ name: string; description: string; relationship?: string }>
  visual_keywords: string[]
  evidence: Array<{ id: string; timestamp_ms: number; transcript?: string; frame_asset_id?: string }>
}

export type ApiShortDramaV2Direction = {
  id: string
  category: 'curiosity' | 'summary'
  title: string
  hook_copy: string
  description: string
  rationale: string
  visual_intent: string
  grounding_evidence_ids: string[]
}

export type ApiShortDramaCanvas = {
  width_pixels: number
  height_pixels: number
  aspect_ratio: number
  duration_ms: number
  frame_rate?: string
}

export type ApiShortDramaModelCanvas = {
  ratio: string
  resolution: string
  width: number
  height: number
  image_width: number
  image_height: number
}

export type ApiShortDramaOutputCanvas = {
  width: number
  height: number
  aspect_num: number
  aspect_den: number
  frame_rate: number
  normalize_mode: string
}

export type ApiShortDramaReferenceBoardPlan = {
  version: string
  layout: '2x2_v1'
  vibe_intent: {
    version: string
    visual_anchor: string
    behavior_state: string
    local_tone: string
    theme: string
    hard_constraints: string[]
    evidence_ids: string[]
  }
  panels: Array<{ slot: 'A' | 'B' | 'C' | 'D'; role: string; description: string; evidence_ids: string[] }>
  global_style: string
  negative_rules: string[]
  content_hash: string
}

export type ApiShortDramaV2Workspace = {
  contract_version: 'creative-short-drama-preroll-workspace/v2' | 'creative-short-drama-preroll-workspace/v3' | 'creative-short-drama-preroll-workspace/v4'
  task_id: string
  revision: number
  active_stage: 'source_ready' | 'analyzing' | 'analysis_ready' | 'directions_ready' | 'prompts_ready' | 'first_frames_generating' | 'first_frames_ready' | 'first_frame_selected' | 'video_generating' | 'normalizing_output' | 'completed'
  source_video: ApiShortDramaV2ProjectAssetRef
  source_canvas?: ApiShortDramaCanvas
  model_canvas?: ApiShortDramaModelCanvas
  output_canvas?: ApiShortDramaOutputCanvas
  analysis: {
    status: string
    revision: number
    input_hash?: string
    prompt_version?: string
    content: ApiShortDramaV2AnalysisContent
  }
  direction_batch?: {
    status: string
    id: string
    revision: number
    analysis_revision: number
    planner_version?: string
    items: ApiShortDramaV2Direction[]
    selected_direction_id?: string
  }
  prompt_draft?: {
    revision: number
    direction_id: string
    duration_seconds: 10 | 12 | 15
    image_prompt: string
    video_description: string
    video_prompt: string
    base_video_prompt?: string
    selected_variant_key?: string
    compiler_version: string
    content_hash: string
    reference_board_plan?: ApiShortDramaReferenceBoardPlan
  }
  first_frame_batch?: {
    status: string
    id: string
    revision: number
    prompt_revision: number
    candidates: Array<{
      id: string
      variant_index: number
      provider_job_id?: string
      status: string
      asset?: ApiShortDramaV2ProjectAssetRef
      model_canvas_asset?: ApiShortDramaV2ProjectAssetRef
      output_canvas_asset?: ApiShortDramaV2ProjectAssetRef
      variant_key?: string
      visual_mechanism?: string
      style_profile?: string
      error_message?: string
    }>
    selected_asset?: ApiShortDramaV2ProjectAssetRef
    selected_output_asset?: ApiShortDramaV2ProjectAssetRef
  }
  reference_board_batch?: {
    status: string
    id: string
    revision: number
    prompt_revision: number
    analysis_revision: number
    candidates: Array<{
      id: string
      variant_index: number
      primary_test_variable: string
      plan: ApiShortDramaReferenceBoardPlan
      provider_job_id?: string
      status: string
      asset?: ApiShortDramaV2ProjectAssetRef
      model_reference_asset?: ApiShortDramaV2ProjectAssetRef
      error_code?: string
      error_message?: string
      current_attempt_id?: string
      recovery_state?: string
      failure_class?: string
      recoverable?: boolean
      attempts?: Array<{
        id: string
        ordinal: number
        mode: 'initial' | 'transient_retry' | 'policy_rewrite' | 'style_fallback'
        rewrite_policy_version?: string
        source_prompt_hash?: string
        prompt_hash: string
        provider_job_id?: string
        status: string
        provider_error_code?: string
        failure_class?: string
        retryable?: boolean
        created_at: string
        completed_at?: string
      }>
    }>
    selected_candidate_id?: string
    selected_asset?: ApiShortDramaV2ProjectAssetRef
    desired_count?: number
    ready_count?: number
    running_count?: number
    failed_count?: number
    recoverable_failed_count?: number
  }
  source_opening_frame?: { status: string; asset?: ApiShortDramaV2ProjectAssetRef; timestamp_ms: number }
  trusted_materials?: {
    provider_code: 'ark-video'
    first_frame_asset_id: string
    last_frame_asset_id: string
  }
  generation_spec?: {
    input_mode: 'text_only' | 'reference_image' | 'first_last_frame'
    fallback_mode?: 'text_only_realistic'
    fallback_reason?: string
    spec_hash: string
  }
  latest_video_attempt_id?: string
  video_error?: { code: string; message: string; retryable: boolean }
  raw_output_asset?: ApiShortDramaV2ProjectAssetRef
  output_asset?: ApiShortDramaV2ProjectAssetRef
}

export type ApiShortDramaV2TaskDetail = {
  task: {
    id: string
    display_name: string
    performance_mode: 'short_drama_preroll'
    status: string
    version: number
    created_at: string
    updated_at: string
  }
  video_draft: { revision: number; short_drama_preroll_v2: ApiShortDramaV2Workspace }
}

export type ApiShortDramaPrerollSnapshot = {
  planVersion: ApiShortDramaPrerollPlan['version']
  storyContext: Omit<ApiShortDramaStoryContext, 'openingLine'>
  selectedCandidate: ApiShortDramaPrerollCandidate
  prompt: string
}

export type ApiGameEvidenceMoment = {
  id: string
  kind: 'skill_choice' | 'wave_progress' | 'battle'
  start_milliseconds: number
  end_milliseconds: number
  description: string
  verified_copy: string[]
}

export type ApiGameHookMechanism =
  | 'choice_challenge'
  | 'tactical_tradeoff'
  | 'wave_escalation'
  | 'failure_reversal'
  | 'merge_upgrade'
  | 'reward_reveal'

export type ApiGamePrerollCandidate = {
  id: string
  hook_mechanism: ApiGameHookMechanism
  execution_angle: string
  primary_test_variable: string
  variant_hypothesis: string
  score: number
  score_meaning: 'evidence_grounded_hook_relevance'
  hook_line: string
  evidence_moment_ids: string[]
  storyboard: Array<{
    start_milliseconds: number
    end_milliseconds: number
    visual: string
    copy: string
    evidence_moment_id: string
  }>
  prompt_package: {
    prompt_compiler_version: string
    input_snapshot_hash: string
    candidate_batch_id: string
    candidate_id: string
    generation_config: {
      subtitle_style: 'high_contrast_dynamic' | 'brand_minimal'
      hook_strength: number
      pace_profile: 'punchy' | 'balanced'
    }
    director_spec: Record<string, string>
    negative_constraints: string[]
    compiled_prompt: string
    content_hash: string
  }
}

export type ApiGamePrerollWorkspace = {
  task: { id: string; performance_mode: 'game_preroll'; status: string }
  video_draft: {
    revision: number
    game_preroll: {
      contract_version: 'creative-game-preroll-draft/v1' | 'creative-game-preroll-draft/v2'
      revision: number
      selected_candidate_id?: string
      input_snapshot: {
        brief_id: string
        brief_version: number
        brief_name: string
        game_name: string
        gameplay_summary: string
        source_video: ApiAssetVersionRef
        source_video_rights: 'confirmed'
        call_to_action: string
        evidence_moments: ApiGameEvidenceMoment[]
        allowed_mechanisms: ApiGameHookMechanism[]
        prohibited_mechanisms: ApiGameHookMechanism[]
      }
      readiness: {
        planning_ready: boolean
        generation_ready: boolean
        production_ready: boolean
        blockers: string[]
      }
      active_candidate_batch: {
        id: string
        revision: number
        planner_version: string
        prompt_compiler_version: string
        generation_config: {
          subtitle_style: 'high_contrast_dynamic' | 'brand_minimal'
          hook_strength: number
          pace_profile: 'punchy' | 'balanced'
        }
        generated_candidate_count: number
        candidates: ApiGamePrerollCandidate[]
        created_at: string
      }
      candidates: ApiGamePrerollCandidate[]
      evidence_assets?: {
        source_video: ApiAssetVersionRef
        status: 'ready' | 'preparing' | 'failed'
        content_hash: string
        frames: Array<{
          evidence_moment_id: string
          source_start_milliseconds: number
          source_end_milliseconds: number
          representative_frame_milliseconds: number
          frame_asset: { project_id: string; asset_version: ApiAssetVersionRef }
          extraction_version: string
        }>
      }
      generation_spec?: {
        contract_version: 'creative-game-preroll-generation-spec/v1'
        input_mode: 'first_last_frame'
        conditioning_assets: Array<{
          role: 'first_frame' | 'last_frame'
          evidence_moment_id: string
          reference: { project_id: string; asset_version: ApiAssetVersionRef }
        }>
        hash: string
      }
    }
  }
  game_preroll_generation_attempts?: Array<{
    id: string
    task_id: string
    draft_revision: number
    candidate_batch_id: string
    candidate_id: string
    prompt_package_hash: string
    generation_spec_hash: string
    provider_job_id: string
    output_asset_version?: ApiAssetVersionRef
    created_at: string
  }>
}

export type ApiCreateManualGamePrerollInput = {
  briefId: string
  briefVersion: number
  briefName: string
  gameName: string
  gameplaySummary: string
  sourceVideo: ApiAssetVersionRef
  evidenceMoments: ApiGameEvidenceMoment[]
  allowedMechanisms: ApiGameHookMechanism[]
  prohibitedMechanisms: ApiGameHookMechanism[]
  subtitleStyle: 'high_contrast_dynamic' | 'brand_minimal'
  hookStrength: number
  paceProfile: 'punchy' | 'balanced'
  objective: string
  audience: string
  coreMessage: string
  callToAction: string
  mandatoryElements: string[]
  prohibitedClaims: string[]
}

export type ApiPrerollScope = {
  projectId: string
  purpose: 'preroll'
  prerollType: ApiPrerollType
}

export type ApiBusinessTaskType =
  | 'strategy'
  | 'creative'
  | 'video'
  | 'brand_video'
  | 'short_drama_preroll'
  | 'game_preroll'
  | 'commerce_preroll'
  | 'viral_remake'
  | 'video_edit'

export type ApiBusinessTask = {
  id: string
  projectId: string
  type: ApiBusinessTaskType
  name: string
  objective: string
  status: 'draft' | 'in_progress' | 'ready' | 'completed' | 'failed'
  sourceTaskIds: string[]
  sourceArtifactIds: string[]
  outputArtifactIds: string[]
  version: number
  createdAt: string
  updatedAt: string
}

export type ApiAuditEvent = {
  id: string
  projectId: string
  actor: string
  action: string
  entityType: 'project' | 'business_task' | 'artifact' | 'generation_job' | 'change_set' | 'operational_record'
  entityId: string
  metadata: Record<string, unknown>
  createdAt: string
}

export type ApiOperationalRecordKind =
  | 'work_item'
  | 'evidence'
  | 'activity'
  | 'metric'
  | 'performance_ad'
  | 'audience_mix'
  | 'method'
  | 'delivery_diagnostic'
  | 'delivery_action'
  | 'unified_record'

export type ApiOperationalRecord = {
  id: string
  projectId: string
  kind: ApiOperationalRecordKind
  title: string
  status: string
  occurredAt: string
  fields: Record<string, unknown>
  createdAt: string
  updatedAt: string
}

export type ApiRemixEvalCase = {
  id: string
  type: 'mcq' | 'rubric'
  title: string
  prompt: string
  planner_version: string
  prompt_version: string
  choices?: Array<{ id: string; label: string }>
  expected_choice?: string
  rubric?: Array<{ id: string; label: string; signal: string; weight: number; required: boolean }>
  passing_score: number
  created_at: string
}

export type ApiRemixEvalSubmission = {
  case_id: string
  choice_id?: string
  answer_text?: string
  rubric_evidence?: string[]
}

export type ApiRemixEvalRun = {
  id: string
  status: 'succeeded'
  planner_version: string
  prompt_version: string
  score: number
  total_cases: number
  passed_cases: number
  failed_cases: string[]
  results: Array<{
    id: string
    case_id: string
    case_type: string
    score: number
    passed: boolean
    expected: string
    actual: string
    reason: string
  }>
  created_at: string
}

export type ApiKnowledgeDocument = {
  id: string
  organization_id: string
  project_id: string
  title: string
  source_uri: string
  source_type: string
  chunk_count: number
  filename?: string
  mime_type?: string
  size_bytes?: number
  content_sha256?: string
  text_sha256?: string
  extracted_text?: string
  status?: 'parse_queued' | 'ready' | 'parse_failed'
  parse_error_code?: string
  parse_error_message?: string
  created_at: string
  updated_at: string
}

export type ApiKnowledgeCitation = {
  document_id: string
  chunk_id: string
  title: string
  source_uri: string
  section: string
  start_line: number
  end_line: number
  snippet: string
}

export type ApiKnowledgeSearchResult = {
  chunk: {
    id: string
    document_id: string
    project_id: string
    index: number
    text: string
    source_uri: string
    section: string
    start_line: number
    end_line: number
  }
  score: number
  citations: ApiKnowledgeCitation[]
}

// 复盘报告（03 AM-015）。一次投放执行对应一份报告：汇总素材表现、结论，
// 确认之后从它沉淀经验。后端权威定义在 internal/systems/insights/service.go。
export type ApiReportStatus = 'draft' | 'confirmed'

// 一次投放执行。后端权威定义在 api/openapi/delivery-v1.yaml 的 ExecutionResult。
// 这里只取报告要用到的几个字段：挑投放时人要看的是「哪天跑的、跑了什么」。
export type ApiDeliveryExecutionResult = {
  execution: {
    id: string
    change_set_id: string
    status: string
    mode: string
    executed_by: string
    started_at: string
    completed_at: string
  }
  evidence: {
    id: string
    execution_id: string
    summary: string
    mode: string
    reversible: boolean
    created_at: string
  }
}

// 报告里的一条发现。四块按 kind 分组，顺序即下面这个联合类型的顺序。
// 后端权威定义在 internal/systems/insights/report_digest.go。
export type ApiReportSectionKind = 'asset_performance' | 'experiment' | 'experience' | 'recommendation'

export type ApiReportFinding = {
  kind: ApiReportSectionKind
  text: string
  // pinned 是人在分析页记的，system 是复盘时按规则补的。复盘页要把两者分开标
  // （● 我记的 / ○ 系统补的）：混在一起，人就分不清哪几条是自己挑的。
  origin: 'system' | 'pinned'
  // 出自素材对比的发现才有。可归因的排在方向性前面。
  strength?: ApiVariantVerdict
  confidence?: ApiConfidenceLevel
  // 三档判定平铺在这里（后端是内嵌结构体）。档位文案由后端给，前端不自己翻译。
  verdict?: Verdict
  verdict_label?: string
  upgrade?: UpgradePath
  note?: string
  // 判出这条时生效的阈值版本。定格在发现上，之后改阈值也不会改写它——
  // 一份复盘里的发现可能来自不同时间的分析，报告顶部标一个号是不够的。
  threshold_version?: number
  // 这条出自哪个视图、说的哪个变量。两者构成去重键：人记过的，系统不再补一条。
  dimension?: string
  variable?: string
  // 这条发现的来源 ID（素材、实验或经验），供人跳回去核对。
  source_ref?: string
  // 记这一笔的人和时刻。origin 是 pinned 才有。
  pinned_by?: string
  pinned_at?: string
  // 被人工删掉的条目留在数组里，只是标记为 true——报告要能说清
  // 「系统给了什么、人拿掉了哪几条」。
  dropped: boolean
}

export type ApiInsightReport = {
  id: string
  organization_id: string
  project_id: string
  execution_id: string
  delivery_mode: string
  evidence_id: string
  evidence_summary: string
  metric_snapshot_id: string
  creative_package_id: string
  // 模拟投放的报告不能和真实投放的报告混着看：结论的分量不一样。
  is_simulated: boolean
  dataset_version: string
  status: ApiReportStatus
  summary: string
  // 旧报告的读法，保留不动。新报告读 digest。
  findings: string[]
  // 定格的四块发现。老报告是空数组。
  digest?: ApiReportFinding[]
  // 定格的数据窗口。老报告没有窗口概念，这两个字段为空。
  window_start?: string
  window_end?: string
  version: number
  created_by: string
  confirmed_by?: string
  confirmed_at?: string
  created_at: string
  updated_at: string
}

// 三态：待定 → 在用 → 停用。
// 「待复审」不在这里，它是「在用」上的一个标记（见 ApiExperience.needs_review）：
// 被标记的经验仍然在用、仍然能被引用，只是该重新看一眼。做成状态的话，每个读
// 经验的地方都得判断「confirmed 或者 needs_review」，漏一处它就凭空消失。
export type ApiExperienceStatus = 'pending' | 'confirmed' | 'retired'

export type ApiExperience = {
  id: string
  organization_id: string
  project_id: string
  lineage_id: string
  revision: number
  supersedes_id?: string
  superseded_by_id?: string
  report_id: string
  source_execution_id: string
  source_evidence_id: string
  source_metric_snapshot_id: string
  conclusion: string
  conditions: string[]
  counterexamples: string[]
  // 洞察卡九字段（03 §8.1）在 Experience 上是全的，投前洞察的 /prelaunch 只是它的投影。
  // 经验库详情要靠这几项判断一条结论该不该确认，缺了就只能看结论一句话点确认。
  card_type: ApiInsightCardType
  confidence: ApiConfidenceLevel
  // 三档判定平铺在这里（后端是内嵌结构体）。档位文案由后端给，前端不自己翻译。
  verdict: Verdict
  verdict_label: string
  upgrade: UpgradePath
  note: string
  recommended_action: string
  applicability: ApiApplicability
  data_basis: ApiDataBasis
  content_basis: ApiContentBasis
  status: ApiExperienceStatus
  // 「该看一眼了」。它不影响这条经验能不能用，只影响界面上要不要挂个提示。
  needs_review: boolean
  status_reason: string
  status_changed_by: string
  status_changed_at?: string
  confirmed_by?: string
  confirmed_at?: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
}

// 一条命中。matched 说清「凭什么推给你」，default 说清「能不能直接照着做」。
//
// default 是后端算的两道闸（状态在用 + 判定 ✅ 能归因），前端不重算：
// 重算一次就多一套规则，两边哪天不一致，同一条经验会在两个页面上一个能用一个不能用。
export type ApiExperienceMatch = {
  experience: ApiExperience
  matched: string[]
  default: boolean
  // 抄到别处时的完整说法（结论 + 适用条件 + 来源）。后端拼好发过来，
  // 前端不自己再拼一遍——两处拼法迟早对不上，而对不上的时候没人知道该信哪个。
  citation_text: string
}

// 「查」的条件。每一格空着表示不限。字段名跟后端 ExperienceLookup 逐字对齐。
export type ApiExperienceLookup = {
  brand?: string
  product?: string
  channel?: string
  ad_type?: string
  objective?: string
  audience?: string
  feature?: string
  include_observed?: boolean
  limit?: number
}

export type ApiExperienceAudit = {
  id: string
  organization_id: string
  project_id: string
  experience_id: string
  from_status: string
  to_status: ApiExperienceStatus
  reason: string
  actor_id: string
  created_at: string
}

// 引用之后发生了什么。四挡是有序的：只是引用 → 照做 → 改了之后用 → 没采纳。
export type ApiExperienceReferenceOutcome = 'referenced' | 'adopted' | 'modified' | 'rejected'

export type ApiExperienceReference = {
  id: string
  organization_id: string
  project_id: string
  experience_id: string
  consumer_kind: string
  consumer_id: string
  outcome: string
  note: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
}

// 投前洞察卡（03 §8.1 九字段）。洞察卡不是一张新表，是经验库的投影：
// 后端权威定义在 internal/systems/insights/prelaunch.go，契约见 api/openapi/insights-v1.yaml。

// 类型承担 03 §2 目标⑥：把事实、计算、推断和人的结论分开。
// 只有 fact 和 statistic 能被下游当证据引用——拿假设当证据是循环论证。
export type ApiInsightCardType = 'fact' | 'statistic' | 'hypothesis' | 'recommendation'

export type ApiApplicability = {
  brands?: string[]
  products?: string[]
  channels?: string[]
  creative_types?: string[]
  objectives?: string[]
  audiences?: string[]
  time_range_note?: string
}

export type ApiDataBasis = {
  asset_count?: number
  sample_size?: number
  window_start?: string
  window_end?: string
  metrics?: string[]
  baseline?: string
}

export type ApiContentBasis = {
  features?: string[]
  example_asset_versions?: string[]
  note?: string
}

/** 修订请求体（AM-013）。对应后端 insights.ReviseExperienceRequest。 */
export type ReviseExperienceBody = {
  expected_version: number
  reason: string
  conclusion: string
  conditions: string[]
  counterexamples: string[]
  card_type: ApiInsightCardType
  confidence: ApiConfidenceLevel
  recommended_action: string
  applicability: ApiApplicability
  data_basis: ApiDataBasis
  content_basis: ApiContentBasis
}

export type ApiInsightCard = {
  experience_id: string
  lineage_id: string
  revision: number
  conclusion: string
  type: ApiInsightCardType
  type_label: string
  applicability: ApiApplicability
  conditions: string[]
  data_basis: ApiDataBasis
  content_basis: ApiContentBasis
  confidence: ApiConfidenceLevel
  confidence_hint: string
  counterexamples: string[]
  recommended_action: string
  status: ApiExperienceStatus
  // 九字段没填全的卡照样返回，缺哪几项写在这里。藏起来会让人以为经验库是空的。
  missing_fields: string[]
  reference_count: number
  updated_at: string
}

export type ApiFeaturePattern = {
  feature: string
  card_count: number
  channels: string[]
  // 取最强置信而不是平均：一条充分证据和一条样本不足不该被平均成方向性。
  best_confidence: ApiConfidenceLevel
  conclusions: string[]
}

export type ApiPreLaunchFacets = {
  channels: string[]
  creative_types: string[]
  objectives: string[]
}

export type ApiPreLaunchInsight = {
  project_id: string
  cards: ApiInsightCard[]
  patterns: ApiFeaturePattern[]
  facets: ApiPreLaunchFacets
  // 筛选期间被排除的「没写适用范围」的经验条数。不静默丢弃。
  unscoped_excluded: number
  mixed_channels: string[]
  cross_channel_comparison: boolean
  // quality_checked=false 表示这一次没能做数据质量校验，不是「校验通过」。
  quality_checked: boolean
  strong_conclusions_allowed: boolean
  quality_blockers: string[]
  experience_references: ApiExperience[]
  disclosure: string
}

export type ApiPreLaunchFilter = {
  channel?: string
  creative_type?: string
  objective?: string
  q?: string
  cross_channel?: boolean
}

// 分析素材库与内容分析（03 §9 AM-001~006）。后端权威定义在
// internal/systems/insights/assets.go 与 features.go，契约见 api/openapi/insights-v1.yaml。

export type ApiAnalysisStatus =
  | 'awaiting_data' | 'awaiting_match' | 'analysable' | 'analysing'
  | 'pending_confirmation' | 'confirmed' | 'needs_review' | 'retired'

export type ApiInsightAssetType =
  | 'xiaohongshu_note' | 'wechat_article' | 'brand_ad'
  | 'digital_human_ad' | 'preroll_ad' | 'hit_replica_ad'

export type ApiAssetSourceKind = 'creative' | 'upload' | 'external'

/**
 * AI 推断与人工结论是两层，互不覆盖（03 §14）。
 *
 * 三类的可信度不是一回事：derived 客观可测（从文件本身算出来的时长、分辨率、镜头数，
 * 同一个文件算两遍结果一样）、human 人工标注、ai 模型推断。只有前两类能进归因结论；
 * ai 行被人复核认可之后按 human 算。这条规则由后端执行，前端只读 admissible。
 */
export type ApiFeatureSource = 'ai' | 'human' | 'derived'

export type ApiConfidence = 'low' | 'medium' | 'high'

// authored 是「AI 没提过这一项，人第一个填的」，和 rejected（有推断但人不认）分开。
// 混成一个值会让人手填的特征被投后分析当成被否掉的推断丢掉。
export type ApiReviewState = 'pending' | 'confirmed' | 'rejected' | 'authored'

export type ApiMappingStatus = 'unmatched' | 'matched' | 'ignored'

export type ApiFeatureValueKind =
  | 'text' | 'tags' | 'enum' | 'enum_multi' | 'number' | 'bool' | 'duration_seconds'

export type ApiFeatureValue = {
  kind: ApiFeatureValueKind
  text?: string
  terms?: string[]
  number?: number
  bool?: boolean
}

export type ApiInsightAsset = {
  id: string
  organization_id: string
  project_id: string
  lineage_id: string
  revision: number
  title: string
  source_kind: ApiAssetSourceKind
  source_ref?: string
  source_job_id?: string
  platform_asset_id?: string
  platform_asset_version?: number
  asset_type?: ApiInsightAssetType
  asset_type_source?: ApiFeatureSource
  asset_type_confidence?: ApiConfidence
  analysis_status: ApiAnalysisStatus
  analysis_status_reason?: string
  analysis_status_changed_at?: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
}

/**
 * 登记素材的请求体。对应后端 insights.IndexAssetRequest。
 *
 * `lineage_id` 留空就是新建一条血缘；填上现有素材的 lineage_id，就是给那条创意
 * 加一个新版本——修订号由后端接着排，人不用管。这是「同一条创意改了三版」和
 * 「三条不同创意」的唯一区别，填错了投后分析会把它们混着比。
 */
export type IndexInsightAssetBody = {
  title: string
  source_kind: ApiAssetSourceKind
  source_ref?: string
  lineage_id?: string
  // 媒体资产引用。后端要求这两个要么都给要么都不给（assets.go 的 validate），
  // 给了才能从洞察点回素材库看原件。
  platform_asset_id?: string
  platform_asset_version?: number
  asset_type?: ApiInsightAssetType
  asset_type_source?: ApiFeatureSource
  asset_type_confidence?: ApiConfidence
}

export type ApiInsightAssetMapping = {
  id: string
  organization_id: string
  project_id: string
  platform: string
  platform_object_kind: string
  platform_object_id: string
  platform_object_name?: string
  asset_id?: string
  status: ApiMappingStatus
  match_source?: string
  matched_by?: string
  matched_at?: string
  note?: string
  version: number
  created_at: string
  updated_at: string
}

export type ApiInsightAssetFeature = {
  id: string
  organization_id: string
  project_id: string
  asset_id: string
  asset_type: ApiInsightAssetType
  key: string
  value: ApiFeatureValue
  source: ApiFeatureSource
  confidence?: ApiConfidence
  review_state: ApiReviewState
  skill_id?: string
  skill_version?: string
  extracted_at?: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
}

// 写入用的特征条目：AI 层必须带 confidence，人工层必须不带（internal/systems/insights/assets.go FeatureInput）。
export type ApiFeatureInput = {
  key: string
  value: ApiFeatureValue
  confidence?: ApiConfidence
  review_state?: ApiReviewState
}

// 一次分析任务的留痕。这里没有输入正文，只有它的指纹和规模——
// 外部返回的内容和输入全文都不入日志（doc09 §7）。
export type ApiAnalysisRun = {
  id: string
  kind: 'feature_extraction'
  asset_id: string
  status: 'running' | 'succeeded' | 'failed'
  asset_type: ApiInsightAssetType
  skill_id?: string
  skill_version?: string
  skill_content_hash?: string
  prompt_version?: string
  provider_code?: string
  model_alias?: string
  model_version?: string
  generation_mode?: 'model' | 'template'
  input_hash?: string
  result_hash?: string
  feature_count: number
  dropped_fields?: string[]
  data_through?: string
  prompt_tokens: number
  completion_tokens: number
  latency_ms: number
  error_code?: string
  error_message?: string
  started_at: string
  finished_at?: string
  created_by: string
}

export type ApiAnalyzeAssetResult = {
  run: ApiAnalysisRun
  features: ApiInsightAssetFeature[]
  dropped_fields?: string[]
}

export type ApiFeatureField = {
  key: string
  label: string
  group: string
  kind: ApiFeatureValueKind
  vocabulary?: string[]
  unit?: string
  note?: string
}

export type ApiFeatureSchema = {
  asset_type: ApiInsightAssetType
  label: string
  source: string
  fields: ApiFeatureField[]
  // 这类素材的本体是不是一段视频。是的话，提取时人不用再写一遍画面描述——
  // 后端把视频交给多模态自己去看。判断放在后端（insights.AssetType.IsVideo），
  // 这里只读结果，不再自己列一份类型清单。
  is_video: boolean
}

export type ApiFeatureMatrixCell = {
  asset_id: string
  source: ApiFeatureSource
  confidence?: ApiConfidence
  review_state: ApiReviewState
  value: ApiFeatureValue
}

export type ApiFeatureMatrixRow = {
  key: string
  label: string
  group: string
  kind: ApiFeatureValueKind
  cells: ApiFeatureMatrixCell[]
}

export type ApiFeatureMatrix = {
  assets: ApiInsightAsset[]
  asset_types: ApiInsightAssetType[]
  rows: ApiFeatureMatrixRow[]
  disclosure: string
}

// 相似素材（internal/systems/insights/similar.go）。
//
// 「像在哪」必须说得出来，所以每条结果都带 reasons；带 source 是因为读的人有权
// 知道这一条相似是量出来的、人标的，还是模型猜的——三者在复盘会上的分量不一样。
export type ApiSimilarityReason = {
  key: string
  label: string
  value: string
  source: ApiFeatureSource
}

export type ApiSimilarAsset = {
  asset_id: string
  title: string
  // overlap 是重叠的变量数，admissible_overlap 是其中能进归因的那些（量出来的 + 人标的）。
  // 前者回答「像不像」，后者回答「拉进来之后能不能真的做归因」。
  overlap: number
  admissible_overlap: number
  score: number
  reasons: ApiSimilarityReason[]
}

export type ApiSimilarAssetResult = {
  // probe 是这次检索按哪几个变量找的。不给出来，人没法判断结果值不值得信。
  probe: ApiSimilarityReason[]
  items: ApiSimilarAsset[]
  note?: string
}

// 外部素材（internal/systems/insights/external.go）。没有版本、没有血缘、没有状态。
export type ApiExternalPurpose = 'benchmark' | 'reference'

export type ApiExternalAsset = {
  id: string
  organization_id: string
  project_id: string
  title: string
  source_note?: string
  asset_type?: ApiInsightAssetType
  purpose: ApiExternalPurpose
  purpose_note?: string
  storage_key?: string
  // original_purged 为 true 表示原件已按留存期清掉，只剩下人标的变量。
  original_purged: boolean
  features: Record<string, ApiFeatureValue>
  retention_until: string
  created_by: string
  created_at: string
  updated_at: string
}

export type ApiInsightAssetFilter = {
  statuses?: ApiAnalysisStatus[]
  assetTypes?: ApiInsightAssetType[]
  sourceKinds?: ApiAssetSourceKind[]
  lineageId?: string
  limit?: number
}

// 数据接入与投后分析指标（10-ad-data-connectors.md）。后端权威定义在
// internal/systems/insights/connectors.go，契约见 api/openapi/insights-v1.yaml。

export type ApiPlatform = 'douyin' | 'kuaishou' | 'xiaohongshu' | 'wechat' | 'tencent_ads' | 'other'

export type ApiIngestMode = 'api' | 'service_account' | 'file_import' | 'computer_use' | 'business'

export type ApiDataSourceStatus = 'draft' | 'active' | 'paused' | 'revoked'

/** doc10 §11。healthy 以外的任何取值都会阻止这个数据源的数字生成强结论（§12.4）。 */
export type ApiQualityStatus =
  | 'healthy' | 'delayed' | 'partial' | 'mapping_incomplete'
  | 'tracking_broken' | 'reconciling' | 'blocked'

export type ApiImportKind = 'sync' | 'backfill' | 'file' | 'correction'

export type ApiImportStatus = 'pending' | 'running' | 'succeeded' | 'partial' | 'failed'

/** 置信提示的四个档位（03 §9），刻意不是数字。 */
export type ApiConfidenceLevel = 'sufficient' | 'directional' | 'low_sample' | 'confounded'

/** 口径：两个数字并排放之前必须一致的东西（doc10 §6）。 */
export type ApiMetricCaliber = {
  time_zone: string
  currency: string
  attribution_window: string
  metric_schema_version: string
}

export type ApiMetricCounts = {
  impressions: number
  clicks: number
  conversions: number
  video_views: number
  video_completions: number
  spend_cents: number
  revenue_cents: number
}

/**
 * 派生指标。字段缺失表示「不可用」——分母为零时后端不返回该字段（doc10 §6）。
 * 前端必须显示「不可用」，不能当成 0 参与排序或比较。
 */
export type ApiMetricRates = {
  ctr?: number
  cvr?: number
  completion_rate?: number
  cpa_cents?: number
  cpm_cents?: number
  roas?: number
}

export type ApiRateInterval = { low: number; high: number }

export type ApiDataSource = {
  id: string
  organization_id: string
  project_id: string
  platform: ApiPlatform
  account_label?: string
  account_ref?: string
  ingest_mode: ApiIngestMode
  credential_ref?: string
  status: ApiDataSourceStatus
  quality_status: ApiQualityStatus
  quality_note?: string
  caliber: ApiMetricCaliber
  field_mapping?: Record<string, string>
  data_through?: string
  last_synced_at?: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
}

export type ApiImportBatch = {
  id: string
  organization_id: string
  project_id: string
  data_source_id: string
  kind: ApiImportKind
  status: ApiImportStatus
  source_label?: string
  window_start?: string
  window_end?: string
  content_hash?: string
  requested_rows: number
  accepted_rows: number
  rejected_rows: number
  error_summary?: string
  errors?: string[]
  corrects_batch_id?: string
  started_at?: string
  finished_at?: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
}

/**
 * 带三档判定的类型一律写成 `{...} & Judgement`：后端那边是 embedded struct，
 * 四个字段在 JSON 里平铺在宿主对象上。抄八遍字段的话，改一次要改八处，
 * 改漏一处就有一页在说另一套话——这正是这轮重构要消灭的东西。
 */
export type ApiAssetMetricPerformance = {
  asset_id?: string
  asset_title: string
  asset_type?: ApiInsightAssetType
  objects: number
  counts: ApiMetricCounts
  rates: ApiMetricRates
  attributable: boolean
} & Judgement

export type ApiPerformancePoint = {
  date: string
  counts: ApiMetricCounts
  rates: ApiMetricRates
}

export type ApiPlatformTotal = {
  platform: ApiPlatform
  label: string
  counts: ApiMetricCounts
  rates: ApiMetricRates
}

export type ApiSourceHealth = {
  data_source_id: string
  platform: ApiPlatform
  label: string
  quality_status: ApiQualityStatus
  quality_note?: string
  data_through?: string
  freshness_days: number
}

export type ApiMetricOverview = {
  window: { start: string; end: string }
  caliber: ApiMetricCaliber
  caliber_conflicts?: string[]
  comparable: boolean
  comparable_reason?: string
  totals: ApiMetricCounts
  rates: ApiMetricRates
  ctr_interval?: ApiRateInterval
  series: ApiPerformancePoint[]
  assets: ApiAssetMetricPerformance[]
  unmatched_objects: number
  unmatched_spend_cents: number
  sources: ApiSourceHealth[]
  warnings?: string[]
  platforms: ApiPlatformTotal[]
  /** note 原来叫 confidence_note。同一个意思两个键名，前端要写两套渲染，已统一。 */
} & Judgement

/**
 * AM-009 的判定阶梯，从严到松。**只有 attributable 是「能归到这个变量」**，
 * 其余四档都是「不能」，只是不能的理由不同。
 */
export type ApiVariantVerdict = 'attributable' | 'directional' | 'confounded' | 'low_sample' | 'no_features'

/** 两个素材之间某个特征的取值差异。未记录的一侧写作「（未记录）」。 */
export type ApiFeatureDiff = {
  key: string
  label: string
  group: string
  baseline: string
  variant: string
  /**
   * 变量来源。derived 从文件算出、human 人工标注、ai 模型推断。
   * 两侧来源不同时取更弱的那一个——只要有一边是猜的，这条差异就是猜的。
   */
  source: ApiFeatureSource
  /**
   * 这条差异能否进入归因结论。ai 来源恒为 false。为 false 的差异照样要显示出来，
   * 只是不参与「改了几个变量」的计数——准入规则由后端定，前端不要再实现一遍。
   */
  admissible: boolean
}

/**
 * 素材对比（03 §7.3 / AM-009）。changed_features 长度 > 1 时 verdict 一定是
 * confounded——差异归不到其中任何一个变量上，哪怕数字差得很多。
 */
export type ApiVariantComparison = {
  baseline_asset_id: string
  baseline_title: string
  variant_asset_id: string
  variant_title: string
  asset_type?: ApiInsightAssetType
  changed_features: ApiFeatureDiff[]
  /** 两侧取值相同的特征数量——受控变量越多，单变量判定越可信。 */
  controlled_count: number
  baseline_counts: ApiMetricCounts
  variant_counts: ApiMetricCounts
  baseline_rates: ApiMetricRates
  variant_rates: ApiMetricRates
  baseline_ctr_interval?: ApiRateInterval
  variant_ctr_interval?: ApiRateInterval
  /** 任一侧区间算不出来时为 true——不知道差异是否显著，就不能说它显著。 */
  intervals_overlap: boolean
  ctr_lift?: number
  /**
   * 素材对比专有的五档，比三档更细：它还回答「归不了因是因为变量太多，
   * 还是因为压根没有特征数据」。**键名不是 verdict**——那个键归三档
   * （见后端 internal/systems/insights/verdict.go）。
   */
  variant_verdict: ApiVariantVerdict
} & Judgement

/** direction 为 unknown 表示天数不足或前半段无曝光，不能当成持平。 */
export type ApiAssetTrend = {
  asset_id: string
  asset_title: string
  asset_type?: ApiInsightAssetType
  points: ApiPerformancePoint[]
  /** 真正有数据的天数，不是窗口长度。 */
  active_days: number
  direction: 'rising' | 'flat' | 'declining' | 'unknown'
  ctr_change?: number
} & Judgement

/** likely 需要两项条件同时成立；单项恶化只到 watch。 */
export type ApiFatigueSeverity = 'none' | 'watch' | 'likely'

export type ApiFatigueSignal = {
  asset_id: string
  asset_title: string
  asset_type?: ApiInsightAssetType
  first_half: ApiMetricCounts
  second_half: ApiMetricCounts
  first_rates: ApiMetricRates
  last_rates: ApiMetricRates
  ctr_change?: number
  cpa_change?: number
  impression_change?: number
  severity: ApiFatigueSeverity
  /** 没能排除的其他解释。这里列的是「排除不了」，不是「已排除」。 */
  alternative_explanations?: string[]
} & Judgement

export type ApiAnomalyKind = 'spike' | 'drop' | 'gap'

/** 用中位数与 MAD 判定偏离（阈值 3.5），不用均值与标准差：一次大促尖峰会把标准差本身抬高。 */
export type ApiMetricAnomaly = {
  date: string
  scope: 'project' | 'asset'
  asset_id?: string
  asset_title?: string
  metric: string
  kind: ApiAnomalyKind
  observed: number
  median: number
  /** 偏离中位数多少个 MAD。 */
  deviation: number
  /** 异常永远只到 👁：这一天不对劲是事实，为什么不对劲这里答不了。 */
} & Judgement

/**
 * 驱动因素：某个特征取值的素材组与其余素材的对比。**这不是因果**——
 * covarying_features 非空时 confidence 一律降为 confounded。
 */
export type ApiFeatureDriver = {
  asset_type?: ApiInsightAssetType
  key: string
  label: string
  group: string
  value: string
  assets: number
  rest_assets: number
  counts: ApiMetricCounts
  rest_counts: ApiMetricCounts
  rates: ApiMetricRates
  rest_rates: ApiMetricRates
  ctr_interval?: ApiRateInterval
  rest_ctr_interval?: ApiRateInterval
  intervals_overlap: boolean
  ctr_lift?: number
  /** 与本特征完全同向变化的其他特征，分不开谁在起作用。 */
  covarying_features?: string[]
} & Judgement

/**
 * 投后分析五个二级视图（素材对比 / 趋势 / 疲劳 / 异常 / 驱动因素）的共用载荷。
 * 五个视图共用一次返回，否则「趋势里看到的」和「疲劳里算的」会来自两次不同的读取。
 */
export type ApiPerformanceAnalysis = {
  window: { start: string; end: string }
  caliber: ApiMetricCaliber
  /** 口径不一致时为 false，所有 attributable 会被降级为 directional。 */
  comparable: boolean
  comparable_reason?: string
  comparisons: ApiVariantComparison[]
  trends: ApiAssetTrend[]
  fatigue: ApiFatigueSignal[]
  anomalies: ApiMetricAnomaly[]
  drivers: ApiFeatureDriver[]
  assets_in_window: number
  /** 其中有内容特征的素材数。远小于 assets_in_window 时，对比和驱动因素都会大面积空着。 */
  assets_with_features: number
  /**
   * 整屏的档位，取五类结论里最弱的那一档。这是唯一一处 judgement 以嵌套对象
   * 出现的地方（后端那边它是具名字段而非 embedded），其余都平铺。
   */
  judgement: Judgement
  notes?: string[]
}

/** 质量问题的五个类别，对应数据质量的前五个二级视图。第六个「修复队列」是跨类别的队列视图。 */
export type ApiDataQualityIssueKind = 'freshness' | 'missing' | 'anomaly' | 'caliber' | 'reconciliation'

/** blocking 会让整个 Project 暂停强结论与自动优化（PRD §10.3、doc10 §12.4）。 */
export type ApiDataQualitySeverity = 'blocking' | 'warning' | 'info'

/**
 * 问题当前的处置状态。open 与 reopened 不入库，由后端把实时检测结果和处置记录比对算出。
 * reopened = 有人报了修但问题在那之后又被观测到——这件事必须被看见，
 * 否则点一次「已修复」就能让问题永久消失。
 */
export type ApiDataQualityIssueState = 'open' | 'acknowledged' | 'resolved' | 'ignored' | 'reopened'

/** 能被写进库的三种处置，是 ApiDataQualityIssueState 的子集。 */
export type ApiDataQualityDispositionState = 'acknowledged' | 'resolved' | 'ignored'

export type ApiDataQualityDisposition = {
  id: string
  organization_id: string
  project_id: string
  fingerprint: string
  issue_kind: ApiDataQualityIssueKind
  state: ApiDataQualityDispositionState
  note: string
  observed_through: string
  version: number
  decided_by: string
  created_at: string
  updated_at: string
}

export type ApiDataQualityIssue = {
  /** 同一个问题反复出现时保持不变，且不含数据窗口——换窗口看到的还是同一条。 */
  fingerprint: string
  kind: ApiDataQualityIssueKind
  severity: ApiDataQualitySeverity
  title: string
  detail: string
  /** 下一步该查什么。PRD §10.3 要求不能只报警不给动作。 */
  suggestion: string
  scope_label: string
  data_source_id?: string
  platform?: ApiPlatform
  affected_spend_cents: number
  affected_days?: number
  stat_date?: string
  /** 处置时要原样回传，后端靠它判断处置之后问题有没有再出现。 */
  last_observed_at: string
  state: ApiDataQualityIssueState
  disposition?: ApiDataQualityDisposition
}

export type ApiDataQualityReport = {
  window: { start: string; end: string }
  generated_at: string
  /** 已按「在队列里 → 严重度 → 影响花费」排好序（20 §4.1 错误与延迟置顶）。 */
  issues: ApiDataQualityIssue[]
  by_kind: Partial<Record<ApiDataQualityIssueKind, number>>
  open_count: number
  queue_count: number
  strong_conclusions_allowed: boolean
  blocked_reason?: string
  sources?: ApiSourceHealth[]
}

// ---- 能力运营（03 §一级导航；20 §4.1）----
// 五个二级视图共用一次请求。这个模块一张表都不建：特征体系读后端的六套 schema，
// 指标字典是后端声明的常量，Skill 与评测集从已有的特征行现算。

/** `fact` 可跨天相加；`derived` 只能用「总量除总量」重算，日均比率是另一个数。 */
export type ApiMetricKind = 'fact' | 'derived'

export type ApiCaliberFactor = 'time_zone' | 'currency' | 'attribution_window' | 'metric_schema_version'

export type ApiMetricDictionaryEntry = {
  key: string
  label: string
  kind: ApiMetricKind
  unit?: string
  definition: string
  formula?: string
  /** 这个指标的口径依赖哪些要素；只有依赖的要素冲突了才会影响它。 */
  caliber_factors?: ApiCaliberFactor[]
  source: string
  /** 本项目实际有多少天导过。0 表示一条都没有，和「有值但为 0」不是一回事。 */
  day_count: number
  total: number
  /** false 表示各数据源在它依赖的口径要素上不一致，跨源对比前要先展示差异（03 §7）。 */
  comparable: boolean
  conflict_notes?: ApiCaliberFactor[]
}

export type ApiCaliberConflict = {
  factor: ApiCaliberFactor
  label: string
  values: string[]
  note: string
}

export type ApiFeatureValueUsage = {
  value: string
  asset_count: number
}

export type ApiFeatureFieldUsage = ApiFeatureField & {
  /** 词表已发布。false 的枚举字段目前接受任何取值，这正是碎片化的敞口。 */
  governed: boolean
  asset_count: number
  distinct_values: number
  /** 按使用量降序，最多 12 个。统计用生效值：人工结论覆盖机器结论，机器旧值不再计入。 */
  values?: ApiFeatureValueUsage[]
  /** 全项目只有一条素材用过的取值。是候选不是结论——系统不做语义猜测。 */
  merge_candidates?: string[]
  off_vocabulary?: string[]
  /**
   * 生效取值分别由谁写的，键是来源，值是素材条数。没人填过这个字段时缺省。
   *
   * 归因只认量出来的和人标的。一个八成取值都是模型猜的字段，拿它分组比出来的差异
   * 说明不了问题，而只看 asset_count 看不出这一点——填得越满反而越像可信。
   */
  source_counts?: Partial<Record<ApiFeatureSource, number>>
}

export type ApiFeatureSystemHealth = {
  asset_type: ApiInsightAssetType
  label: string
  source: string
  asset_count: number
  field_count: number
  used_field_count: number
  open_enum_count: number
  fields: ApiFeatureFieldUsage[]
}

export type ApiSkillHealth = {
  skill_id: string
  skill_version: string
  extraction_count: number
  asset_count: number
  field_keys: string[]
  high_confidence: number
  medium_confidence: number
  low_confidence: number
  first_extracted_at?: string
  last_extracted_at?: string
  /** 按最近一次提取时间判在用，不按版本号字符串排序（否则 v9 会排在 v10 后面）。 */
  latest: boolean
}

export type ApiEvaluationExample = {
  asset_id: string
  asset_title?: string
  feature_key: string
  label: string
  ai_value: string
  human_value: string
}

export type ApiFieldEvaluation = {
  key: string
  label: string
  reviewed: number
  agreed: number
}

/**
 * 一个 Skill 版本的人机一致率。**不是独立评测集**：样本全部来自人工复核记录，
 * 回答的是「被人看过的地方机器错了多少」，不是整体准确率。没人复核过的提取一条都
 * 不计入——算成「机器对了」，准确率会随提取量自动上涨。
 */
export type ApiSkillEvaluation = {
  skill_id: string
  skill_version: string
  reviewed: number
  agreed: number
  disagreed: number
  /** 样本少于 10 条时恒为 0，此时要藏起这个数而不是显示 0%。 */
  accuracy: number
  confidence: ApiConfidenceLevel
  note: string
  fields?: ApiFieldEvaluation[]
  examples?: ApiEvaluationExample[]
}

export type ApiOperationsTodo = {
  kind: string
  severity: string
  title: string
  detail: string
  asset_type?: ApiInsightAssetType
  feature_key?: string
}

export type ApiOperationsDashboard = {
  feature_field_total: number
  feature_field_used: number
  open_vocabulary_fields: number
  merge_candidate_count: number
  off_vocabulary_count: number
  caliber_conflict_count: number
  skill_version_count: number
  evaluation_samples: number
  /** 词表待办只覆盖已经有人用过的开放枚举字段，否则真正在散的那几个会被埋掉。 */
  todos: ApiOperationsTodo[]
}

export type ApiCapabilityOperations = {
  window: { start: string; end: string }
  generated_at: string
  feature_systems: ApiFeatureSystemHealth[]
  metrics: ApiMetricDictionaryEntry[]
  caliber_conflicts: ApiCaliberConflict[]
  skills: ApiSkillHealth[]
  evaluations: ApiSkillEvaluation[]
  dashboard: ApiOperationsDashboard
}

/**
 * 一条设置。effect 和 recommended 不是可选注释：20 §121 要求「重要阈值显示影响说明
 * 和默认推荐」，22 §239 记的问题正是「缺少实际阈值影响说明」。
 */
export type ApiSettingItem = {
  key: string
  label: string
  /** 现在生效的值，后端已格式化好（含单位），前端不要再拼。 */
  value: string
  effect: string
  recommended: string
  /**
   * 当前值偏离了推荐值，页面要提示「有人调过它」。由后端判定，前端不要自己拿
   * value 和 recommended 比字符串——确认权限那组两边说的是不同的事（管到哪些操作
   * vs 该发给谁），字面永远不同，一比就会给每一条都打上凭空造出来的告警。
   */
  deviates: boolean
  /** 值在代码里的位置，例如 internal/systems/insights/connectors.go:267。 */
  source: string
  /** 文档依据；没有依据会显式写「无文档指定值」，不会留空。 */
  basis: string
  /**
   * 非空表示这一条可以改，值是它在保存请求 values 里的键名。
   *
   * 可写与否标在**条**上而不是标在组上：同一组里既有能改的判定阈值，也有不能改的
   * 保护性上限（导入行数、异常判定倍数这类）。标在组上只能整组开或整组关——
   * 要么把防呆开关暴露出去，要么把该调的锁死。
   */
  editable_key?: string
}

/**
 * `draft` 设计中，变量/分组/门槛都还能改；`running` 已开跑，**分组冻结**；
 * `concluded` 已下结论。冻结是「事先登记」的全部依据——没有它，谁都能在看完数据
 * 之后往表现好的那组补两条素材。
 */
export type ApiExperimentStatus = 'draft' | 'running' | 'concluded'

/** 系统给的判定，不是人写的解读。 */
export type ApiExperimentVerdict = 'supported' | 'refuted' | 'inconclusive'

/**
 * 组间比较结果。实验中心和投后分析「驱动因素」共用同一套算法，差别只在 note 的
 * 措辞：事先登记的分组能说到「归因到这个变量」，事后凑出的分组只能说到「相关」。
 */
export type ApiGroupComparison = {
  counts: ApiMetricCounts
  rest_counts: ApiMetricCounts
  rates: ApiMetricRates
  rest_rates: ApiMetricRates
  ctr_interval?: ApiRateInterval
  rest_ctr_interval?: ApiRateInterval
  /** 为 true 时差异可能只是波动，不管相对差看起来有多大。 */
  intervals_overlap: boolean
  ctr_lift?: number
  /** 事先登记的实验这里恒为空：混杂由入组那道关把守，不在事后翻账。 */
  covarying_features?: string[]
  confidence: ApiConfidenceLevel
  note: string
}

export type ApiExperimentVariant = {
  id: string
  organization_id: string
  project_id: string
  experiment_id: string
  name: string
  /** 这一组在被测变量上的取值。入组素材必须和它对得上。 */
  variable_value: string
  is_baseline: boolean
  asset_ids: string[]
  position: number
  created_at: string
  updated_at: string
}

export type ApiExperiment = {
  id: string
  organization_id: string
  project_id: string
  title: string
  hypothesis: string
  /** 假设的出处：投前洞察的假设卡「拿去验证」过来时带上；为空表示空白新建。 */
  source_experience_id?: string
  asset_type: ApiInsightAssetType
  variable_key: string
  variable_label: string
  /** 要求各组一致的其他特征。不一致不拦，入组时给黄牌。 */
  controlled_keys: string[]
  /** **事先**定的每组最低展示量。开跑之后不能改。 */
  min_impressions: number
  window_start: string
  window_end: string
  status: ApiExperimentStatus
  verdict?: ApiExperimentVerdict
  /** 人写的解读。判定由系统给，解读由人负责，两者分开存。 */
  interpretation?: string
  concluded_by?: string
  concluded_at?: string
  started_at?: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
  variants: ApiExperimentVariant[]
}

/**
 * 一组的样本量。**这一层永远有数**，包括不够的时候：知道还差多少才知道还要投多久。
 * 不够时被藏起来的是对比数字，不是样本数字。
 */
export type ApiVariantSample = {
  variant_id: string
  name: string
  variable_value: string
  is_baseline: boolean
  assets: number
  assets_with_data: number
  impressions: number
  clicks: number
  meets_threshold: boolean
  short_by: number
}

export type ApiExperimentComparison = {
  variant_id: string
  variant_name: string
  variant_value: string
  baseline_id: string
  baseline_name: string
  baseline_value: string
  /**
   * **为 true 时 result 和 verdict 一定不存在。** 样本不达标却显示 CTR 差异，
   * 人会先看见数字再看见提示，然后记住数字。
   */
  blocked: boolean
  blocker?: string
  result?: ApiGroupComparison
  verdict?: ApiExperimentVerdict
}

export type ApiExperimentReadout = {
  window: { start: string; end: string }
  caliber: ApiMetricCaliber
  comparable: boolean
  comparable_reason?: string
  samples: ApiVariantSample[]
  comparisons: ApiExperimentComparison[]
  /** 每一组都过了门槛才为 true。为 false 时下结论会被拒绝。 */
  ready: boolean
  verdict?: ApiExperimentVerdict
  notes: string[]
}

export type ApiExperimentDetail = {
  experiment: ApiExperiment
  readout: ApiExperimentReadout
}

export type ApiCreateVariantInput = {
  name: string
  variable_value: string
  is_baseline?: boolean
}

export type ApiCreateExperimentInput = {
  title: string
  hypothesis?: string
  source_experience_id?: string
  asset_type: ApiInsightAssetType
  variable_key: string
  controlled_keys?: string[]
  min_impressions: number
  window_start: string
  window_end: string
  /** 至少两组，且恰好一组 is_baseline。只有一组就没有对照，也就没有实验。 */
  variants: ApiCreateVariantInput[]
}

export type ApiAttachExperimentAssetResult = {
  variant: ApiExperimentVariant
  /** 变量取值对不上是硬拦（抛错）；控住的变量不一致只到这里，不拦。 */
  warnings: string[]
}

/**
 * 这一组落在设置页的哪一段。空串表示不单开一段（后端只有 not_built 的组才允许为空）。
 *
 * 后端六个组和页面四个视图不是一一对应：样本门槛和观察窗口都归「判定阈值」，
 * 通知和报告模板还没建设，不上页面。由后端给而不是前端映射，是因为「哪一组该出现在
 * 哪一段」是设置本身的属性——前端另写一张表，加一个组就要改两处，漏改的那一处
 * 会让新组在页面上凭空消失。
 */
export type ApiSettingsView = '' | 'thresholds' | 'health' | 'dictionary' | 'permission' | 'ocean-engine-session'

export type ApiSettingGroup = {
  key: string
  label: string
  /** in_effect 现在真的在生效；not_built 还没有任何东西，此时 items 为空。 */
  state: 'in_effect' | 'not_built'
  view: ApiSettingsView
  /** 这一组里有至少一条 editable_key 非空。由后端汇总，前端不要自己数。 */
  editable: boolean
  summary: string
  /** 只在 not_built 时有内容。 */
  missing: string[]
  items: ApiSettingItem[]
}

export type ApiInsightSettings = {
  generated_at: string
  /** 有东西可改。为 false 时整页只读，不要渲染禁用输入框。 */
  editable: boolean
  editable_note: string
  /** 恒为 false：这些值对整个组织生效，路径上的 project 只用于鉴权。 */
  project_scoped: boolean
  groups: ApiSettingGroup[]
}

/**
 * 现在生效的那一份阈值：每格都有值，且带着版本号。
 *
 * version 0 = 一版都没存过，跑的是代码里的出厂设定。
 */
export type ApiResolvedThresholds = {
  version: number
  sufficient_impressions: number
  directional_impressions: number
  min_trend_days: number
  min_anomaly_days: number
  min_driver_assets: number
  max_comparison_assets: number
  quality_window_days: number
}

/** 落库的一版。改动史用它，看的是「谁在什么时候、为什么改的」。 */
export type ApiThresholdSet = {
  id: string
  organization_id: string
  version: number
  /** 只有被调过的格子有值；没有的格子跑出厂设定。 */
  values: Partial<Omit<ApiResolvedThresholds, 'version'>>
  reason: string
  changed_by: string
  changed_at: string
}

/** 一行 canonical 日指标。stat_date 是数据源时区下的当地日期 YYYY-MM-DD。 */
export type ApiMetricRow = {
  platform_object_kind: string
  platform_object_id: string
  platform_object_name?: string
  stat_date: string
  counts: ApiMetricCounts
  raw?: Record<string, unknown>
}

export type ApiImportResult = {
  batch: ApiImportBatch
  new_mappings: number
}

export type ApiDataSourceFilter = {
  statuses?: ApiDataSourceStatus[]
  platforms?: ApiPlatform[]
  limit?: number
}

export type ApiImportBatchFilter = {
  dataSourceId?: string
  statuses?: ApiImportStatus[]
  limit?: number
}

export type ApiRemixRenderJob = {
  id: string
  organization_id: string
  project_id: string
  plan_id: string
  status: 'queued' | 'running' | 'requires_review' | 'succeeded' | 'failed'
  progress: number
  target_format: 'mp4'
  target_quality: 'draft' | 'standard' | 'high'
  requires_review: boolean
  quality_report_id?: string
  output_asset?: {
    project_id: string
    asset_version: {
      asset_id: string
      version: number
    }
  }
  output_preview?: {
    url: string
  }
  provenance?: {
    plan_id: string
    render_job_id: string
    input_assets: ApiAssetVersionRef[]
  }
  error_code?: string
  error_message?: string
  created_at: string
  updated_at: string
}

export type ApiFeedbackEventType = 'rating' | 'comment' | 'asset_selected' | 'render_succeeded'
export type ApiFeedbackTargetType = 'remix_plan' | 'render_job' | 'asset'

export type ApiCreateFeedbackEventInput = {
  event_type: ApiFeedbackEventType
  target_type: ApiFeedbackTargetType
  target_id: string
  asset_version?: ApiAssetVersionRef
  rating?: number
  comment?: string
}

export type ApiFeedbackEvent = ApiCreateFeedbackEventInput & {
  id: string
  organization_id: string
  project_id: string
  created_at: string
}

export type ApiAssetPerformance = {
  asset_version: ApiAssetVersionRef
  selected_count: number
  render_succeeded_count: number
  feedback_count: number
  average_rating: number
  updated_at: string
}

export type ApiPlannerWeightSnapshot = {
  id: string
  organization_id: string
  project_id: string
  asset_weights: Array<{
    asset_version: ApiAssetVersionRef
    weight: number
    reasons: string[]
  }>
  created_at: string
}

export type ApiQualityVerdict = 'pass' | 'major' | 'critical'
export type ApiRemixHookType = 'conflict' | 'reversal' | 'suspense' | 'selling_point_bridge' | 'product_demo' | 'offer'
export type ApiPrerollMode = 'prompt_only' | 'generate_video'
export type ApiPrerollStatus = 'draft' | 'ready' | 'failed' | 'applied'

export type ApiQualityReport = {
  id: string
  organization_id: string
  project_id: string
  render_job_id: string
  output_asset?: {
    project_id: string
    asset_version: ApiAssetVersionRef
  }
  verdict: ApiQualityVerdict
  score: number
  dimensions: Array<{
    name: string
    score: number
    verdict: ApiQualityVerdict
    summary: string
  }>
  issues: Array<{
    code: string
    severity: 'major' | 'critical'
    dimension: string
    start_seconds: number
    end_seconds: number
    description: string
    repair_suggestion: string
  }>
  evidence: Array<{
    kind: string
    timestamp_sec: number
    summary: string
  }>
  repair_suggestions: string[]
  created_at: string
  updated_at: string
}

export type ApiAssetVersionRef = {
  asset_id: string
  version: number
}

export type ApiViralRemakeWorkspace = {
  task: {
    id: string
    performance_mode: 'viral_remake'
    status: string
  }
  intake: {
    id: string
    request: {
      call_to_action: string
      manual_viral_remake: {
        product_name: string
        selling_points: string[]
        user_instruction: string
        reference_video: ApiAssetVersionRef
        reference_image?: ApiAssetVersionRef
        reference_video_rights: 'pending' | 'confirmed'
        reference_image_rights?: 'pending' | 'confirmed'
      }
    }
  }
  video_draft: {
    revision: number
    viral_remake: {
      revision: number
      status:
        | 'waiting_for_analysis'
        | 'analysis_ready'
        | 'generation_ready'
        | 'generating'
        | 'candidate_ready'
        | 'provider_failed'
        | 'ready_for_review'
      selected_route_id: 'route_manual_viral_remake_v1'
      input_snapshot: {
        reference_video: ApiAssetVersionRef
        reference_image?: ApiAssetVersionRef
        product_name: string
        selling_points: string[]
        call_to_action: string
        user_instruction: string
        reference_video_rights: 'pending' | 'confirmed'
        reference_image_rights?: 'pending' | 'confirmed'
      }
      readiness: {
        planning_ready: boolean
        generation_ready: boolean
        production_ready: boolean
        missing_fields: string[]
        blockers: string[]
      }
      analysis_snapshot?: {
        contract_version: 'creative-viral-analysis-snapshot/v1'
        task_id: string
        source_asset_ref: ApiAssetVersionRef
        dimensions: Array<{
          id: ApiVideoPromptDimension['id']
          prompt: string
          evidence_refs: string[]
          confidence: number
          source: 'ai_extracted'
        }>
        preserve_rules: string[]
        replace_rules: string[]
        transcript?: string
        confidence: number
        evidence_refs: string[]
        model_lineage: {
          model_alias: string
          route_revision_id: string
          prompt_version: string
        }
        content_hash: string
        created_at: string
      }
      prompt_draft?: {
        revision: number
        dimensions: Record<ApiVideoPromptDimension['id'], string>
        composite_prompt: string
        updated_at: string
      }
      prompt_package?: {
        contract_version: 'creative-viral-prompt-package/v1'
        prompt_version: number
        content_hash: string
        composite_prompt: string
        generation_spec: {
          model_alias: string
          duration_seconds: number
          aspect_ratio: string
          resolution: string
          candidate_count: number
        }
        confirmed_by: string
        confirmed_at: string
      }
      candidates: Array<{
        id: string
        provider_job_id: string
        prompt_hash: string
        status: 'queued' | 'running' | 'succeeded' | 'failed' | 'reviewed'
        output_asset_ref?: ApiAssetVersionRef
        checks: Array<{ code: string; passed: boolean; message: string }>
        error_code?: string
        error_message?: string
        created_at: string
        updated_at: string
      }>
    }
  }
  production_jobs: Array<{ provider_job_id: string; kind: string }>
}

export type ApiCreateManualViralRemakeInput = {
  parentIntakeId?: string
  sourceVideo: ApiAssetVersionRef
  referenceImage?: ApiAssetVersionRef
  productName: string
  sellingPoints: string[]
  callToAction: string
  userInstruction: string
  objective: string
  audience: string
  coreMessage: string
  durationSeconds: number
}

export type ApiHitSegmentRole = 'hook' | 'problem' | 'proof' | 'offer' | 'cta'

export type ApiHitAnalysis = {
  id: string
  organization_id: string
  project_id: string
  source_asset: ApiAssetVersionRef
  title: string
  video_meta: {
    duration_seconds: number
    language: string
  }
  segments: Array<{
    id: string
    start_seconds: number
    end_seconds: number
    role: ApiHitSegmentRole
    summary: string
    script: string
    visual_element: string
    conversion_cue: string
    replication_hint: string
  }>
  scripts: Array<{ segment_id: string; text: string }>
  visual_elements: string[]
  conversion_nodes: Array<{ segment_id: string; cue: string }>
  replication_insights: string[]
  created_at: string
  updated_at: string
}

export type ApiCreateHitAnalysisInput = {
  source_asset: ApiAssetVersionRef
  title: string
  duration_seconds: number
  language?: string
  notes?: string
}

export type ApiVideoPromptDimensionId =
  | 'task_goal_type'
  | 'quality_style_lighting'
  | 'environment_atmosphere'
  | 'camera_content'
  | 'music_sound'

export type ApiVideoPromptDimension = {
  id: ApiVideoPromptDimensionId
  label: string
  prompt: string
  evidence: string
}

export type ApiVideoReplicationPrompt = {
  source_asset: ApiAssetVersionRef
  source_title: string
  source_file_name?: string
  reference_image_name?: string
  user_instruction?: string
  dimensions: ApiVideoPromptDimension[]
  composite_prompt: string
  model_directive: string
}

export type ApiProductProfile = {
  name: string
  selling_points: string[]
  cta: string
}

export type ApiReplacementRule = {
  role: ApiHitSegmentRole
  target_asset: ApiAssetVersionRef
  message: string
}

export type ApiCreateProductMappingInput = {
  hit_analysis_id: string
  target_product: ApiProductProfile
  required_assets: ApiAssetVersionRef[]
  replacement_rules: ApiReplacementRule[]
  constraints?: string[]
  target_seconds?: number
  pace?: 'fast' | 'balanced' | 'story'
}

export type ApiProductMapping = ApiCreateProductMappingInput & {
  id: string
  organization_id: string
  project_id: string
  created_at: string
  updated_at: string
}

export type ApiCreateRemixPrerollInput = {
  plan_id: string
  hook_type: ApiRemixHookType
  reference_asset: ApiAssetVersionRef
  style_constraints: string[]
  duration_seconds: number
  mode: ApiPrerollMode
}

export type ApiRemixPreroll = ApiCreateRemixPrerollInput & {
  id: string
  organization_id: string
  project_id: string
  prompt_draft: string
  output_asset?: {
    project_id: string
    asset_version: ApiAssetVersionRef
  }
  quality_verdict: ApiQualityVerdict
  status: ApiPrerollStatus
  error_code?: string
  error_message?: string
  applied_plan_id?: string
  created_at: string
  updated_at: string
}

export type ApiRemixPlan = {
  id: string
  organization_id: string
  project_id: string
  schema_version: 'remix_plan_v1' | 'remix_plan_v2'
  client_plan_id: string
  target_seconds: number
  actual_seconds: number
  pace: 'fast' | 'balanced' | 'story'
  segments: Array<{
    segment: 'opening' | 'middle' | 'ending'
    label: string
    target_seconds: number
    actual_seconds: number
    shots: Array<{
      id: string
      source: 'existing_asset'
      asset_version: ApiAssetVersionRef
      timeline?: {
        start_seconds: number
        duration_seconds: number
        in_point_seconds: number
        out_point_seconds: number
      }
      creative: {
        scene: string
        shot_type: string
        dialogue_or_narration: string
        subtitle: string
        cta_element: string
      }
    }>
  }>
  warnings: string[]
  summary: {
    selected_assets: number
    used_assets: number
    coverage_percent: number
    strategy: string
  }
}

export type ApiAgentRun = {
  id: string
  organization_id: string
  project_id: string
  workflow: 'render_diagnosis'
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'
  target: {
    render_job_id: string
  }
  steps: Array<{
    id: string
    label: string
    status: ApiAgentRun['status']
    summary: string
    started_at: string
    ended_at?: string
  }>
  tool_calls: Array<{
    id: string
    name: string
    status: 'pending' | 'running' | 'succeeded' | 'failed'
    input: Record<string, unknown>
    output?: Record<string, unknown>
    error_message?: string
    references?: Array<{ type: string; id: string; version?: number }>
    started_at: string
    ended_at?: string
  }>
  trace_spans: Array<{
    id: string
    parent_id?: string
    name: string
    kind: 'agent' | 'tool' | 'model'
    status: 'running' | 'succeeded' | 'failed' | 'cancelled'
    model?: string
    input_tokens?: number
    output_tokens?: number
    error_message?: string
    started_at: string
    ended_at?: string
  }>
  output?: Record<string, unknown>
  error_message?: string
  created_at: string
  updated_at: string
  completed_at?: string
}

export type ApiProviderCapabilities = {
  provider: string
  status: 'configured' | 'not_configured'
  capabilities: Array<{
    capability: string
    model: string
    available: boolean
  }>
  credential?: {
    source?: 'environment' | 'workspace'
    maskedApiKey?: string
    updatedAt?: string
  }
  checkedAt: string
}

export type ApiAuthSession = {
  authenticated: boolean
  user?: {
    id: string
    email?: string
    displayName: string
  }
  organization?: {
    id: string
    name: string
    status: string
  }
  membership?: {
    role: 'owner' | 'admin' | 'member' | 'auditor'
    status: string
    updatedAt: string
  }
  scopes?: string[]
}

type PlatformActor = {
  organization_id: string
  principal: { kind: 'user' | 'service'; id: string }
  scopes: string[]
}

type PlatformRequestContext = {
  actor: PlatformActor
}

type PlatformLoginResult = PlatformRequestContext & {
  session_id: string
}

export type ApiProviderConfiguration = {
  provider: 'ark'
  status: 'configured' | 'not_configured'
  baseUrl: string
  source?: 'environment' | 'workspace'
  maskedApiKey?: string
  updatedAt?: string
  capabilities: ApiProviderCapabilities
}

const viteEnv = (import.meta as unknown as { env?: { VITE_API_BASE_URL?: string } }).env
const backendOrigin = viteEnv?.VITE_API_BASE_URL ?? ''
const apiBase = `${backendOrigin}/api`
const platformBase = `${backendOrigin}/platform/v1`

type AgencyWorkbenchOptions = {
  projectIds?: string[]
  includeDemoProject?: boolean
}

const emptyAgencyWorkbench: ApiAgencyWorkbench = {
  organizations: [],
  clients: [],
  brands: [],
  projects: [],
  adAccountBindings: [],
  qualityCheckRuns: [],
  materialConfirmations: [],
  assetVersionPointers: [],
}

async function loadPersistedAgencyWorkbench(projectIds: string[]): Promise<ApiAgencyWorkbench> {
  const results = await Promise.all([...new Set(projectIds)].map(async projectId => {
    const [snapshot, workbench, mediaAssets] = await Promise.all([
      platformClient.getProjectSnapshot(projectId),
      platformClient.getWorkbench(projectId),
      platformClient.listProjectMediaAssets(projectId),
    ])
    return workbench ? workbenchFromResponse(snapshot.project, workbench, mediaAssets) : emptyAgencyWorkbench
  }))
  return results.reduce<ApiAgencyWorkbench>((all, current) => ({
    organizations: [...all.organizations, ...current.organizations], clients: [...all.clients, ...current.clients], brands: [...all.brands, ...current.brands], projects: [...all.projects, ...current.projects], adAccountBindings: [...all.adAccountBindings, ...current.adAccountBindings], qualityCheckRuns: [...all.qualityCheckRuns, ...current.qualityCheckRuns], materialConfirmations: [...all.materialConfirmations, ...current.materialConfirmations], assetVersionPointers: [...all.assetVersionPointers, ...current.assetVersionPointers],
  }), emptyAgencyWorkbench)
}

function workbenchFromResponse(
  project: ApiProject,
  response: import('./platformClient.js').PlatformProjectWorkbench,
  mediaAssets: ApiProjectMediaAsset[],
): ApiAgencyWorkbench {
  const { organization, client, brand, project: progress } = response
  const mediaByAssetVersion = new Map(mediaAssets.map(asset => [`${asset.id}:${asset.version}`, asset]))
  return {
    organizations: [{ id: organization.id, code: organization.code, name: organization.name, owner: organization.owner, currency: organization.currency as 'CNY', timezone: organization.timezone as 'Asia/Shanghai', updatedAt: organization.updated_at }],
    clients: [{ id: client.id, organizationId: client.organization_id, code: client.code, name: client.name, industry: client.industry, owner: client.owner, healthStatus: client.health_status as ApiAgencyHealthStatus, updatedAt: client.updated_at }],
    brands: [{ id: brand.id, organizationId: brand.organization_id, clientId: brand.client_id, code: brand.code, name: brand.name, category: brand.category, productLines: brand.product_lines, owner: brand.owner, guidelineStatus: brand.guideline_status as ApiBrand['guidelineStatus'], updatedAt: brand.updated_at }],
    projects: [{ ...project, products: (response.products ?? []).map(product => ({ id: product.id, name: product.name, oceanEngineProductId: product.ocean_engine_product_id })), organizationId: progress.organization_id, clientId: progress.client_id, brandId: progress.brand_id, progressDetail: { stage: progress.stage as ApiProjectProgressStage, stageLabel: progress.stage_label, stagePercent: progress.stage_percent, taskPercent: progress.task_percent, riskStatus: progress.risk_status as ApiAgencyHealthStatus, blocker: progress.blocker || undefined, updatedAt: progress.updated_at } }],
    adAccountBindings: response.ad_account_bindings.map(item => ({ id: item.id, organizationId: item.organization_id, clientId: item.client_id, brandId: item.brand_id, projectIds: [project.id], platform: item.platform as ApiAdPlatform, accountName: item.account_name, accountDisplayId: item.account_display_id, currency: item.currency as 'CNY', timezone: item.timezone as 'Asia/Shanghai', permissionStatus: item.permission_status as ApiBindingHealthStatus, loginStatus: item.login_status as ApiBindingHealthStatus, trackingStatus: item.tracking_status as ApiBindingHealthStatus, owner: item.owner, boundAssetIds: item.bound_asset_ids, lastSyncedAt: item.last_synced_at })),
    qualityCheckRuns: response.quality_check_runs.map(item => ({ id: item.id, organizationId: item.organization_id, projectId: item.project_id, assetId: item.asset_id, assetVersion: item.asset_version, status: item.status as ApiQualityCheckStatus, model: item.model, ruleVersion: item.rule_version, promptVersion: item.prompt_version, summary: item.summary, issues: item.issues.map(issue => ({ id: issue.id, severity: issue.severity as ApiQualityIssueSeverity, rule: issue.rule, evidence: issue.evidence, suggestion: issue.suggestion })), createdAt: item.created_at, completedAt: item.completed_at ?? undefined })),
    materialConfirmations: response.material_confirmations.map(item => ({ id: item.id, organizationId: item.organization_id, projectId: item.project_id, qualityCheckRunId: item.quality_check_run_id, assetId: item.asset_id, assetVersion: item.asset_version, status: item.status as ApiMaterialConfirmationStatus, scope: item.scope, confirmedBy: item.confirmed_by, note: item.note, createdAt: item.created_at })),
    assetVersionPointers: response.asset_version_pointers.map(item => {
      const media = mediaByAssetVersion.get(`${item.asset_id}:${item.working_version}`)
      return {
        id: item.id,
        organizationId: item.organization_id,
        projectId: item.project_id,
        assetId: item.asset_id,
        oceanEngineMaterialId: item.ocean_engine_material_id,
        mediaKind: media?.kind === 'document' ? undefined : media?.kind,
        contentUrl: media?.contentUrl,
        workingVersion: item.working_version,
        qualityCheckedVersion: item.quality_checked_version ?? undefined,
        humanConfirmedVersion: item.human_confirmed_version ?? undefined,
        deliveryVersion: item.delivery_version ?? undefined,
        versions: item.versions.map(version => ({
          version: version.version,
          createdBy: version.created_by,
          sourceTaskId: version.source_task_id,
          sourceType: version.source_type as ApiAssetVersionRecord['sourceType'],
          sourceLabel: version.source_label,
          createdAt: version.created_at,
          changeSummary: version.change_summary,
        })),
        authorization: {
          platforms: item.authorization.platforms as ApiAdPlatform[],
          regions: item.authorization.regions,
          rightsHolder: item.authorization.rights_holder,
          expiresAt: item.authorization.expires_at,
          note: item.authorization.note,
        },
        deliveryTarget: {
          platform: item.delivery_target.platform as ApiAdPlatform,
          region: item.delivery_target.region,
        },
        owner: item.owner,
        updatedAt: item.updated_at,
      }
    }),
  }
}

// 后端对同一类冲突只回一句「当前状态不允许该操作」，具体原因不在响应体里。
// 把状态码和错误码带出来，页面才有机会按场景说人话；message 保持原样，老的 catch 不受影响。
export class ApiRequestError extends Error {
  readonly status: number
  readonly code: string

  constructor(message: string, status: number, code: string) {
    super(message)
    this.name = 'ApiRequestError'
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, method = 'GET', body?: unknown, headers?: Record<string, string>): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    method,
    credentials: 'include',
    headers: { ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...(headers ?? {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const payloadText = await response.text()
  let payload: T | { error?: { message?: string; code?: string } } | undefined
  try {
    payload = payloadText ? JSON.parse(payloadText) as T | { error?: { message?: string; code?: string } } : undefined
  } catch {
    payload = undefined
  }
  if (!response.ok) {
    const error = (payload ?? {}) as { error?: { message?: string; code?: string } }
    throw new ApiRequestError(error.error?.message ?? `API 请求失败（HTTP ${response.status}）`, response.status, error.error?.code ?? '')
  }
  return payload as T
}

function authSessionFromActor(actor: PlatformActor, username?: string): ApiAuthSession {
  const identity = username?.trim() || actor.principal.id
  return {
    authenticated: true,
    user: { id: actor.principal.id, email: '', displayName: identity },
  }
}

async function platformRequest<T>(path: string, method = 'GET', body?: unknown, headers?: Record<string, string>): Promise<T> {
  const response = await fetch(`${platformBase}${path}`, {
    method,
    credentials: 'include',
    headers: {
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(headers ?? {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const payloadText = await response.text()
  const payload = payloadText ? JSON.parse(payloadText) as T | { error?: { message?: string } } : undefined
  if (!response.ok) {
    const error = (payload ?? {}) as { error?: { message?: string } }
    throw new Error(error.error?.message ?? '平台 API 请求失败')
  }
  return payload as T
}

export class CreativeApiError extends Error {
  constructor(message: string, readonly status: number, readonly code = '') {
    super(message)
    this.name = 'CreativeApiError'
  }
}

async function creativeRequest<T>(path: string, method = 'GET', body?: unknown, headers?: Record<string, string>): Promise<T> {
  const response = await fetch(`${backendOrigin}/api/creative/v1${path}`, {
    method,
    headers: {
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(headers ?? {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const responseText = await response.text()
  let payload: T | { error?: { message?: string; request_id?: string; code?: string } }
  try {
    payload = responseText ? JSON.parse(responseText) as T | { error?: { message?: string; request_id?: string; code?: string } } : {}
  } catch {
    throw new Error(`Creative API 返回了无法解析的响应（HTTP ${response.status}）`)
  }
  if (!response.ok) {
    const error = payload as { error?: { message?: string; request_id?: string; code?: string } }
    const requestId = error.error?.request_id ? `（request_id: ${error.error.request_id}）` : ''
    throw new CreativeApiError(`${error.error?.message ?? `Creative API 请求失败（HTTP ${response.status}）`}${requestId}`, response.status, error.error?.code ?? '')
  }
  return payload as T
}

function getTaskStrategyCreativeIntake(projectId: string, intakeId: string) {
  return creativeRequest<ApiTaskStrategyCreativeIntake>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}`,
  )
}

function getCreativeTaskHandoffDetail(projectId: string, taskId: string) {
  return creativeRequest<ApiCreativeTaskHandoffDetail>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}`,
  )
}

function getImageTextWorkspace(projectId: string, taskId: string) {
  return creativeRequest<ApiImageTextWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/image-text-workspace`,
  )
}

function getCreativeIntake(projectId: string, intakeId: string) {
  return creativeRequest<ApiCreativeIntakeBootstrap>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}`,
  )
}

function prepareBrandBriefReview(projectId: string, intakeId: string) {
  return creativeRequest<ApiBrandBriefReview>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}/brand-brief:prepare`,
    'POST',
  )
}

function getStrategyBrandWorkflow(projectId: string, intakeId: string) {
  return creativeRequest<ApiStrategyBrandWorkflow>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}/brand-workflow`,
  )
}

function prepareStrategyBrandWorkflow(projectId: string, intake: ApiCreativeIntakeBootstrap) {
  const selectedRouteId = intake.selected_route_id || intake.request?.selected_route_id || ''
  const inputIdentityHash = intake.input_identity_hash || ''
  if (!selectedRouteId || !inputIdentityHash) throw new Error('品牌策略交接缺少冻结 Route 或输入身份。')
  return creativeRequest<ApiStrategyBrandWorkflow>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intake.id)}/brand-workflow:prepare`,
    'POST',
    {
      expected_input_identity_hash: inputIdentityHash,
      selected_route_id: selectedRouteId,
      accept_strategy_projection: true,
    },
    { 'Idempotency-Key': `strategy-brand-prepare-${inputIdentityHash}` },
  )
}

function updateBrandBriefReview(projectId: string, intakeId: string, review: ApiBrandBriefReview) {
  return creativeRequest<ApiBrandBriefReview>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}/brand-brief`,
    'PATCH',
    { expected_revision: review.revision, document: review.document },
  )
}

function confirmBrandBriefReview(projectId: string, intakeId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandBriefReview>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}/brand-brief:confirm`,
    'POST',
    { expected_revision: expectedRevision },
  )
}

function listCreativeTasks(projectId: string, limit = 100) {
  return creativeRequest<{ items: ApiCreativeTaskSummary[] }>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks?limit=${limit}`,
  )
}

export type ApiExtractedDocumentMedia = {
  filename: string
  mime_type: 'image/png' | 'image/jpeg'
  page_number: number
  page_text?: string
  width: number
  height: number
  size_bytes: number
  sha256: string
  content: string
}

function renameCreativeTask(projectId: string, taskId: string, expectedVersion: number, displayName: string) {
  return creativeRequest<ApiCreativeTaskSummary>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/metadata`,
    'PATCH',
    { expected_version: expectedVersion, display_name: displayName },
  )
}

/**
 * 列这个 Project 的创意版本。不传 task_id 就是全项目。
 *
 * 后端不按状态筛（creative_handlers.go 的 listCreativeVersions 只认 task_id 和
 * limit），要「只看已批准的」得在调用方自己过一遍。
 */
function listCreativeVersions(projectId: string, limit = 100) {
  return creativeRequest<{ items: ApiCreativeVersionSummary[] }>(
    `/projects/${encodeURIComponent(projectId)}/creative-versions?limit=${limit}`,
  )
}

function listCreativeIntakes(projectId: string, limit = 100) {
  return creativeRequest<{ items: ApiCreativeIntakeBootstrap[] }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes?limit=${limit}`,
  )
}

async function uploadKnowledgeDocument(projectId: string, file: File): Promise<ApiKnowledgeDocument> {
  const form = new FormData()
  form.append('file', file)
  const response = await fetch(`${platformBase}/projects/${encodeURIComponent(projectId)}/knowledge/documents`, {
    method: 'POST',
    credentials: 'include',
    body: form,
  })
  const payload = await response.json() as ApiKnowledgeDocument | { error?: { message?: string } }
  if (!response.ok) throw new Error('error' in payload ? payload.error?.message ?? 'Brief 上传失败' : 'Brief 上传失败')
  const document = payload as ApiKnowledgeDocument
  if (document.status === 'ready' && !document.extracted_text?.trim()) {
    return getKnowledgeDocument(projectId, document.id)
  }
  return document
}

function getKnowledgeDocument(projectId: string, documentId: string) {
  return platformRequest<ApiKnowledgeDocument>(
    `/projects/${encodeURIComponent(projectId)}/knowledge/documents/${encodeURIComponent(documentId)}`,
  )
}

function extractKnowledgeDocumentMedia(projectId: string, documentId: string) {
  return platformRequest<{ items: ApiExtractedDocumentMedia[] }>(
    `/projects/${encodeURIComponent(projectId)}/knowledge/documents/${encodeURIComponent(documentId)}/media:extract`,
    'POST',
  )
}

function createManualBrandFilmIntake(projectId: string, document: ApiKnowledgeDocument, durationSeconds = 15, assetCandidates: ApiBrandBriefAssetCandidate[] = []) {
  const filename = document.filename || document.title || '品牌 Brief.pdf'
  const productName = filename.replace(/\.(pdf|docx|md)$/i, '').trim() || '未命名品牌项目'
  const briefText = document.extracted_text?.trim() || ''
  if (!briefText || !document.content_sha256) throw new Error('Brief 尚未解析完成，暂时不能创建品牌广告任务。')
  return creativeRequest<ApiCreativeIntakeBootstrap>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes`,
    'POST',
    {
      contract_version: 'creative-intake-create/v3',
      source: 'manual',
      format: 'video',
      performance_mode: 'brand_video',
      channel: 'douyin',
      objective: `基于《${productName}》Brief 制作 ${durationSeconds} 秒品牌广告`,
      audience: '以 Brief 解析与人工确认为准',
      core_message: productName,
      call_to_action: '',
      concept: '等待 Brief 确认后生成创意方向',
      tone: [],
      visual_keywords: [],
      mandatory_elements: [],
      prohibited_claims: [],
      creative_routes: [{
        route_id: 'route_fixture_brand_video_guerlain_v1',
        route_type: 'brand_video',
        video_purpose: 'brand',
        channels: ['douyin'],
        reason: '用户上传 PDF Brief 后创建的品牌广告制作路线',
        target_duration_seconds: durationSeconds,
        aspect_ratio: '9:16',
        source_asset_refs: assetCandidates.flatMap(candidate => candidate.asset_ref ? [candidate.asset_ref] : []),
        evidence_refs: [`knowledge://documents/${document.id}`],
        requires_human_confirmation: true,
      }],
      manual_brand_film: {
        document_id: document.id,
        fixture_id: '',
        fixture_version: 0,
        fixture_hash: `sha256:${document.content_sha256}`,
        brief_name: filename,
        brief_text: briefText,
        product_name: productName,
        asset_candidates: assetCandidates,
      },
    },
    { 'Idempotency-Key': `manual-brand-film-${document.id}-${durationSeconds}` },
  )
}

function createManualImageTextIntake(
  projectId: string,
  input: ApiCreateManualImageTextInput,
) {
  return creativeRequest<ApiCreativeIntakeBootstrap>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes`,
    'POST',
    {
      contract_version: 'creative-intake-create/v3',
      source: 'manual',
      channel: 'xiaohongshu',
      objective: input.objective.trim(),
      audience: input.audience.trim(),
      core_message: input.coreMessage.trim(),
      call_to_action: input.callToAction.trim(),
      concept: '',
      tone: input.tone,
      visual_keywords: input.visualKeywords,
      mandatory_elements: input.mandatoryElements,
      prohibited_claims: input.prohibitedClaims,
    },
    { 'Idempotency-Key': `manual-image-text-${Date.now()}-${Math.random().toString(36).slice(2)}` },
  )
}

function generateCreativeDirections(projectId: string, intakeId: string) {
  return creativeRequest<ApiCreativeDirectionBatch>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}/direction-candidate-batches`,
    'POST',
    { candidate_count: 3 },
  )
}

async function getLatestCreativeDirectionBatch(projectId: string, intakeId: string) {
  try {
    return await creativeRequest<ApiCreativeDirectionBatch>(
      `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}/direction-candidate-batches/latest`,
    )
  } catch (cause) {
    if (cause instanceof CreativeApiError && cause.status === 404) return null
    throw cause
  }
}

function confirmCreativeDirection(projectId: string, directionId: string) {
  return creativeRequest<ApiCreativeDirection>(
    `/projects/${encodeURIComponent(projectId)}/creative-directions/${encodeURIComponent(directionId)}/confirm`,
    'POST',
  )
}

function createImageTextTaskFromDirection(
  projectId: string,
  intakeId: string,
  directionId: string,
) {
  return creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}:create-task`,
    'POST',
    { content_type: 'custom', direction_id: directionId },
  )
}

function createBrandVideoTaskFromDirection(
  projectId: string,
  intakeId: string,
  directionId: string,
  selectedRouteId: string,
  channel: 'xiaohongshu' | 'douyin' | 'kuaishou',
) {
  return creativeRequest<ApiCreativeTaskSummary>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}:create-video-task`,
    'POST',
    {
      selected_route_id: selectedRouteId,
      direction_id: directionId,
      channel,
      mandatory_elements: [],
      prohibited_claims: [],
      confirm_route: true,
    },
  )
}

function createBrandFilmTaskFromIntake(
  projectId: string,
  intakeId: string,
  selectedRouteId: string,
  channel: 'xiaohongshu' | 'douyin' | 'kuaishou',
) {
  return creativeRequest<ApiCreativeTaskSummary>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intakeId)}:create-video-task`,
    'POST',
    {
      selected_route_id: selectedRouteId,
      channel,
      mandatory_elements: [],
      prohibited_claims: [],
      confirm_route: true,
    },
  )
}

function generateImageTextDraft(projectId: string, taskId: string, expectedTaskVersion: number, expectedDirectionId: string) {
  return creativeRequest<ApiImageTextWorkspace['draft']>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/image-text-draft:generate`,
    'POST',
    { expected_task_version: expectedTaskVersion, expected_direction_id: expectedDirectionId },
  )
}

function updateImageTextDraft(
  projectId: string,
  taskId: string,
  workspace: ApiImageTextWorkspace,
  input: {
    selectedTitle: string
    body: string
    overlayCopy: Record<number, string>
    visualBrief?: Record<number, string>
    caption?: Record<number, string>
  },
) {
  return creativeRequest<ApiImageTextWorkspace['draft']>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/image-text-draft`,
    'PATCH',
    {
      expected_task_version: workspace.task.version,
      expected_draft_revision: workspace.draft.version,
      title_candidates: workspace.draft.title_candidates.map(title =>
        title === workspace.draft.selected_title ? input.selectedTitle : title
      ),
      selected_title: input.selectedTitle,
      body: input.body,
      topics: workspace.draft.topics,
      image_plan: workspace.draft.image_plan.map(item => ({
        ...item,
        overlay_copy: input.overlayCopy[item.order] ?? item.overlay_copy,
        visual_brief: input.visualBrief?.[item.order] ?? item.visual_brief,
        caption: input.caption?.[item.order] ?? item.caption,
      })),
    },
  )
}

function generateImageTextSlot(
  projectId: string,
  taskId: string,
  order: number,
  expectedTaskVersion: number,
  draftRevision: number,
  retry = false,
) {
  return creativeRequest<{ attempt: ApiImageTextAttempt }>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/image-slots/${order}:${retry ? 'retry' : 'generate'}`,
    'POST',
    { expected_task_version: expectedTaskVersion, draft_revision: draftRevision },
    { 'Idempotency-Key': `image-slot-${taskId}-${draftRevision}-${order}-${Date.now()}` },
  )
}

function adoptImageTextAttempt(
  projectId: string,
  taskId: string,
  order: number,
  attemptId: string,
  expectedTaskVersion: number,
  expectedSelectionVersion: number,
) {
  return creativeRequest(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/image-slots/${order}/attempts/${encodeURIComponent(attemptId)}:adopt`,
    'POST',
    {
      expected_task_version: expectedTaskVersion,
      expected_selection_version: expectedSelectionVersion,
    },
  )
}

async function getProjectAssetPreview(projectId: string, ref: ApiAssetVersionRef) {
  const value = await platformRequest<{ url: string; headers?: Record<string, string> }>(
    `/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(ref.asset_id)}/versions/${ref.version}/preview`,
  )
  return value.url.startsWith('/') ? `${backendOrigin}${value.url}` : value.url
}

function listImageTextVersions(projectId: string, taskId: string) {
  return creativeRequest<{ items: ApiCreativeVersion[] }>(
    `/projects/${encodeURIComponent(projectId)}/creative-versions?task_id=${encodeURIComponent(taskId)}`,
  )
}

function listCreativePackages(projectId: string, limit = 100) {
  return creativeRequest<{ items: ApiCreativePackage[] }>(
    `/projects/${encodeURIComponent(projectId)}/creative-packages?limit=${limit}`,
  )
}

function freezeImageTextVersion(projectId: string, taskId: string, draftVersion: number) {
  return creativeRequest<ApiCreativeVersion>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}:freeze-version`,
    'POST',
    { draft_version: draftVersion },
    { 'Idempotency-Key': `image-text-freeze-${taskId}-${draftVersion}` },
  )
}

function checkImageTextVersion(projectId: string, versionId: string) {
  return creativeRequest<ApiCreativeVersion>(
    `/projects/${encodeURIComponent(projectId)}/creative-versions/${encodeURIComponent(versionId)}:check`,
    'POST',
  )
}

function approveImageTextVersion(projectId: string, versionId: string) {
  return creativeRequest<ApiCreativeVersion>(
    `/projects/${encodeURIComponent(projectId)}/creative-versions/${encodeURIComponent(versionId)}:approve`,
    'POST',
  )
}

function deliverImageTextVersion(projectId: string, versionId: string) {
  return creativeRequest<ApiCreativePackage>(
    `/projects/${encodeURIComponent(projectId)}/creative-versions/${encodeURIComponent(versionId)}:deliver`,
    'POST',
  )
}

async function uploadProjectAsset(projectId: string, file: File): Promise<ApiAssetVersionRef> {
  return uploadProjectAssetFile(backendOrigin, projectId, file, 'viral-upload')
}

async function createManualShortDramaPrerollV2Workspace(
  projectId: string,
  sourceVideo: ApiAssetVersionRef,
): Promise<ApiShortDramaV2TaskDetail> {
  const intake = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes`,
    'POST',
    {
      source: 'manual',
      format: 'video',
      performance_mode: 'short_drama_preroll',
      channel: 'douyin',
      objective: '通过独立前贴钩子吸引用户观看短剧正片',
      audience: '竖屏短剧观众',
      core_message: '基于上传短剧的真实剧情生成前贴方向',
      call_to_action: '点击观看正片',
      concept: '短剧前贴 V2',
      tone: ['紧凑', '悬念'],
      visual_keywords: ['剧情连续', '信息缺口'],
      mandatory_elements: [],
      prohibited_claims: ['不得虚构上传视频中不存在的剧情事实'],
      creative_routes: [{
        route_id: 'route_manual_short_drama_preroll_v2',
        route_type: 'short_drama_preroll',
        video_purpose: 'performance',
        channels: ['douyin'],
        reason: '用户在短剧前贴工作区选择项目视频并确认生成',
        target_duration_seconds: 10,
        aspect_ratio: '9:16',
        resolution: '720p',
        source_asset_refs: [sourceVideo],
        evidence_refs: [],
        requires_human_confirmation: true,
      }],
      manual_short_drama_preroll_v2: {
        source_video: sourceVideo,
        source_video_rights: 'confirmed',
      },
    },
    { 'Idempotency-Key': `manual-short-drama-v2-${sourceVideo.asset_id}-${sourceVideo.version}-${Date.now()}` },
  )
  const task = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intake.id)}:create-video-task`,
    'POST',
    {
      selected_route_id: 'route_manual_short_drama_preroll_v2',
      channel: 'douyin',
      source_video: sourceVideo,
      concept: '短剧前贴 V2',
      prompt: '等待理解上传短剧内容并生成前贴方向',
      call_to_action: '点击观看正片',
      mandatory_elements: [],
      prohibited_claims: ['不得虚构上传视频中不存在的剧情事实'],
      confirm_route: true,
    },
    { 'Idempotency-Key': `manual-short-drama-v2-task-${intake.id}` },
  )
  return getShortDramaPrerollV2Workspace(projectId, task.id)
}

function getShortDramaPrerollV2Workspace(projectId: string, taskId: string) {
  return creativeRequest<ApiShortDramaV2TaskDetail>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}`,
  )
}

function shortDramaV2Command(
  projectId: string,
  taskId: string,
  action: string,
  body: unknown,
) {
  const serialized = JSON.stringify(body)
  let hash = 2166136261
  for (let index = 0; index < serialized.length; index += 1) {
    hash ^= serialized.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return creativeRequest<ApiShortDramaV2TaskDetail>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/short-drama-preroll-v2:${action}`,
    'POST',
    body,
    { 'Idempotency-Key': `short-drama-v3-${action}-${taskId}-${(hash >>> 0).toString(36)}` },
  )
}

const analyzeShortDramaV2Source = (projectId: string, taskId: string, expectedRevision: number) =>
  shortDramaV2Command(projectId, taskId, 'analyze-source', { expected_revision: expectedRevision })

const updateShortDramaV2Analysis = (projectId: string, taskId: string, expectedRevision: number, content: ApiShortDramaV2AnalysisContent) =>
  shortDramaV2Command(projectId, taskId, 'update-analysis', { expected_revision: expectedRevision, content })

const generateShortDramaV2Directions = (projectId: string, taskId: string, expectedRevision: number) =>
  shortDramaV2Command(projectId, taskId, 'generate-directions', { expected_revision: expectedRevision })

const selectShortDramaV2Direction = (projectId: string, taskId: string, expectedRevision: number, directionBatchId: string, directionId: string, durationSeconds: number) =>
  shortDramaV2Command(projectId, taskId, 'select-direction', {
    expected_revision: expectedRevision,
    direction_batch_id: directionBatchId,
    direction_id: directionId,
    duration_seconds: durationSeconds,
  })

const updateShortDramaV2Prompts = (projectId: string, taskId: string, expectedRevision: number, imagePrompt: string, videoDescription: string, videoPrompt: string) =>
  shortDramaV2Command(projectId, taskId, 'update-prompts', {
    expected_revision: expectedRevision,
    image_prompt: imagePrompt,
    video_description: videoDescription,
    video_prompt: videoPrompt,
  })

const prepareShortDramaV2OpeningFrame = (projectId: string, taskId: string, expectedRevision: number) =>
  shortDramaV2Command(projectId, taskId, 'prepare-opening-frame', { expected_revision: expectedRevision })

const generateShortDramaReferenceBoards = (projectId: string, taskId: string, expectedRevision: number) =>
  shortDramaV2Command(projectId, taskId, 'generate-reference-boards', { expected_revision: expectedRevision })

const reconcileShortDramaReferenceBoard = (projectId: string, taskId: string, expectedRevision: number, candidateId: string, providerJobId: string) =>
  shortDramaV2Command(projectId, taskId, 'reconcile-reference-board', {
    expected_revision: expectedRevision,
    candidate_id: candidateId,
    provider_job_id: providerJobId,
  })

const retryShortDramaReferenceBoardCandidate = (projectId: string, taskId: string, expectedRevision: number, batchId: string, candidateId: string, failedAttemptId: string) =>
  shortDramaV2Command(projectId, taskId, 'retry-reference-board-candidate', {
    expected_revision: expectedRevision,
    batch_id: batchId,
    candidate_id: candidateId,
    failed_attempt_id: failedAttemptId,
  })

const selectShortDramaReferenceBoard = (projectId: string, taskId: string, expectedRevision: number, batchId: string, candidateId: string) =>
  shortDramaV2Command(projectId, taskId, 'select-reference-board', {
    expected_revision: expectedRevision,
    batch_id: batchId,
    candidate_id: candidateId,
  })

const generateShortDramaV2FirstFrames = (projectId: string, taskId: string, expectedRevision: number) =>
  shortDramaV2Command(projectId, taskId, 'generate-first-frames', { expected_revision: expectedRevision })

const reconcileShortDramaV2FirstFrame = (projectId: string, taskId: string, expectedRevision: number, candidateId: string, providerJobId: string) =>
  shortDramaV2Command(projectId, taskId, 'reconcile-first-frame', {
    expected_revision: expectedRevision,
    candidate_id: candidateId,
    provider_job_id: providerJobId,
  })

const selectShortDramaV2FirstFrame = (projectId: string, taskId: string, expectedRevision: number, batchId: string, candidateId: string) =>
  shortDramaV2Command(projectId, taskId, 'select-first-frame', {
    expected_revision: expectedRevision,
    batch_id: batchId,
    candidate_id: candidateId,
  })

const bindShortDramaV2TrustedMaterials = (projectId: string, taskId: string, expectedRevision: number, firstFrameAssetId: string, lastFrameAssetId: string) =>
  shortDramaV2Command(projectId, taskId, 'bind-trusted-materials', {
    expected_revision: expectedRevision,
    first_frame_asset_id: firstFrameAssetId,
    last_frame_asset_id: lastFrameAssetId,
  })

const generateShortDramaV2Video = (projectId: string, taskId: string, expectedRevision: number) =>
  shortDramaV2Command(projectId, taskId, 'generate-video', { expected_revision: expectedRevision, model_alias: 'cookies.video.standard' })

const reconcileShortDramaV2Video = (projectId: string, taskId: string, expectedRevision: number, providerJobId: string) =>
  shortDramaV2Command(projectId, taskId, 'reconcile-video', { expected_revision: expectedRevision, provider_job_id: providerJobId })

function getShortDramaV2ProviderJob(projectId: string, jobId: string): Promise<ApiGenerationJob> {
  return platformRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/model/jobs/${encodeURIComponent(jobId)}`,
  ).then(mapViralProviderJob)
}

async function createManualViralRemakeWorkspace(
  projectId: string,
  input: ApiCreateManualViralRemakeInput,
): Promise<ApiViralRemakeWorkspace> {
  const duration = Math.min(60, Math.max(4, Math.round(input.durationSeconds)))
  const intake = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes`,
    'POST',
    {
      source: 'manual',
      parent_intake_id: input.parentIntakeId,
      format: 'video',
      performance_mode: 'viral_remake',
      channel: 'douyin',
      objective: input.objective,
      audience: input.audience,
      core_message: input.coreMessage,
      call_to_action: input.callToAction,
      concept: input.userInstruction,
      tone: ['清晰', '高节奏'],
      visual_keywords: ['高停留开场', '产品证明', '原创表达'],
      mandatory_elements: [],
      prohibited_claims: ['不得复用原片人物、商标、字幕、音乐或逐字台词'],
      creative_routes: [{
        route_id: 'route_manual_viral_remake_v1',
        route_type: 'viral_remake',
        video_purpose: 'performance',
        channels: ['douyin'],
        reason: '用户在 Creative 爆款复刻工作区明确选择该路线',
        target_duration_seconds: duration,
        aspect_ratio: '9:16',
        source_asset_refs: [input.sourceVideo, ...(input.referenceImage ? [input.referenceImage] : [])],
        evidence_refs: [],
        requires_human_confirmation: true,
      }],
      manual_viral_remake: {
        product_name: input.productName,
        selling_points: input.sellingPoints.filter(Boolean),
        user_instruction: input.userInstruction,
        reference_video: input.sourceVideo,
        reference_image: input.referenceImage,
        reference_video_rights: 'pending',
        reference_image_rights: input.referenceImage ? 'pending' : undefined,
      },
    },
    { 'Idempotency-Key': `manual-viral-${Date.now()}-${Math.random().toString(36).slice(2)}` },
  )
  const task = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intake.id)}:create-video-task`,
    'POST',
    {
      selected_route_id: 'route_manual_viral_remake_v1',
      channel: 'douyin',
      source_video: input.sourceVideo,
      concept: input.userInstruction,
      prompt: '等待 Phase 2 视频理解后编译五维提示词',
      call_to_action: input.callToAction,
      mandatory_elements: [],
      prohibited_claims: ['不得复制原片受保护表达'],
      confirm_route: true,
    },
  )
  return creativeRequest<ApiViralRemakeWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(task.id)}/viral-remake`,
  )
}

async function createManualShortDramaPrerollWorkspace(
  projectId: string,
  input: ApiCreateManualShortDramaPrerollInput,
): Promise<ApiShortDramaPrerollWorkspace> {
  const intake = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes`,
    'POST',
    {
      source: 'manual', parent_intake_id: input.parentIntakeId,
      format: 'video', performance_mode: 'short_drama_preroll', channel: 'douyin',
      objective: input.objective, audience: input.audience,
      core_message: input.reviewedSellingPoints.filter(Boolean).join('；'), call_to_action: input.callToAction,
      concept: '短剧导流广告前贴', tone: ['紧凑', '悬念'], visual_keywords: ['人物连续', '高对比字幕', 'CTA 收束'],
      mandatory_elements: [], prohibited_claims: input.prohibitedClaims,
      creative_routes: [{
        route_id: 'route_manual_short_drama_preroll_v1', route_type: 'short_drama_preroll', video_purpose: 'performance',
        channels: ['douyin'], reason: '用户在短剧前贴工作区明确选择本地预置 Brief', target_duration_seconds: 6,
        aspect_ratio: '9:16', source_asset_refs: [], evidence_refs: [], requires_human_confirmation: true,
      }],
      manual_short_drama_preroll: {
        brief_id: input.briefId, brief_version: input.briefVersion, brief_name: input.briefName,
        story_title: input.title, synopsis: input.synopsis, reviewed_selling_points: input.reviewedSellingPoints.filter(Boolean),
        opening_line: input.openingLine || undefined, hook_strategy: input.hookStrategy, subtitle_style: input.subtitleStyle,
        transition: input.transition, hook_strength: input.hookStrength, pace_profile: input.paceProfile, character_references: [],
      },
    },
    { 'Idempotency-Key': `manual-short-drama-${Date.now()}-${Math.random().toString(36).slice(2)}` },
  )
  const task = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intake.id)}:create-video-task`,
    'POST',
    {
      selected_route_id: 'route_manual_short_drama_preroll_v1', channel: 'douyin',
      concept: '短剧导流广告前贴', prompt: '等待人工选择短剧候选后由服务端编译 PromptPackage',
      call_to_action: input.callToAction, mandatory_elements: [], prohibited_claims: ['不得虚构未确认剧情事实'], confirm_route: true,
    },
  )
  return creativeRequest<ApiShortDramaPrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(task.id)}/short-drama-preroll`,
  )
}

async function selectShortDramaPrerollCandidate(
  projectId: string,
  taskId: string,
  expectedRevision: number,
  candidateId: string,
): Promise<ApiShortDramaPrerollWorkspace> {
  return creativeRequest<ApiShortDramaPrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/short-drama-preroll:select-candidate`,
    'POST',
    { expected_revision: expectedRevision, candidate_id: candidateId },
    { 'Idempotency-Key': `short-drama-select-${taskId}-${expectedRevision}-${candidateId}` },
  )
}

async function regenerateShortDramaPrerollCandidates(
  projectId: string,
  taskId: string,
  expectedRevision: number,
  generationConfig: ApiShortDramaGenerationConfig,
  variationIntent: ApiShortDramaVariationIntent = 'balanced',
): Promise<ApiShortDramaPrerollWorkspace> {
  return creativeRequest<ApiShortDramaPrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/short-drama-preroll:regenerate-candidates`,
    'POST',
    {
      expected_revision: expectedRevision,
      generation_config: generationConfig,
      variation_intent: variationIntent,
    },
    {
      'Idempotency-Key': [
        'short-drama-regenerate',
        taskId,
        expectedRevision,
        generationConfig.subtitle_style,
        generationConfig.hook_strength,
        generationConfig.pace_profile,
        variationIntent,
      ].join('-'),
    },
  )
}

async function createShortDramaPrerollVideoJob(
  projectId: string,
  taskId: string,
  draftRevision: number,
  candidateId: string,
): Promise<ApiGenerationJob> {
  const job = await creativeRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}:video-job`,
    'POST',
    { model_alias: 'cookies.video.standard' },
    { 'Idempotency-Key': `short-drama-video-${taskId}-${draftRevision}-${candidateId}` },
  )
  return mapViralProviderJob(job)
}

async function getShortDramaPrerollVideoJob(projectId: string, jobId: string): Promise<ApiGenerationJob> {
  const job = await platformRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/model/jobs/${encodeURIComponent(jobId)}`,
  )
  return mapViralProviderJob(job)
}

async function getLatestShortDramaPrerollWorkspace(projectId: string): Promise<ApiShortDramaPrerollWorkspace | null> {
  try {
    return await creativeRequest<ApiShortDramaPrerollWorkspace>(
      `/projects/${encodeURIComponent(projectId)}/creative-workspaces/short-drama-preroll`,
    )
  } catch (cause) {
    if (cause instanceof CreativeApiError && cause.status === 404) return null
    throw cause
  }
}

function commerceRequestFingerprint(value: unknown) {
  const text = JSON.stringify(value)
  let hash = 2166136261
  for (let index = 0; index < text.length; index += 1) {
    hash ^= text.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0).toString(36)
}

function brandFilmPath(projectId: string, taskId: string, suffix: string) {
  return `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/brand-film${suffix}`
}

function brandFilmIdempotencyKey(...parts: Array<string | number>) {
  const raw = parts.map(String).join('-')
  const safe = raw.replace(/[^A-Za-z0-9_-]+/g, '_')
  if (safe.length <= 255) return safe
  return `${safe.slice(0, 242)}-${commerceRequestFingerprint(raw)}`
}

async function getBrandFilmWorkspace(projectId: string, taskId: string): Promise<ApiBrandFilmWorkspace> {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ''))
}

async function initializeStrategyBrandFilmWorkspace(projectId: string, taskId: string): Promise<ApiBrandFilmWorkspace> {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':initialize-from-strategy'), 'POST')
}

async function restoreBrandFilmWorkspace(projectId: string, taskId: string): Promise<ApiBrandFilmWorkspace> {
  try {
    return await getBrandFilmWorkspace(projectId, taskId)
  } catch (cause) {
    if (!(cause instanceof CreativeApiError) || cause.status !== 409) throw cause
    return initializeStrategyBrandFilmWorkspace(projectId, taskId)
  }
}

async function ensureBrandFilmFixtureWorkspace(projectId: string): Promise<ApiBrandFilmWorkspace> {
  return creativeRequest<ApiBrandFilmWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-workspaces/brand-film:ensure-fixture`,
    'POST',
    undefined,
    { 'Idempotency-Key': `brand-film-fixture-${projectId}-guerlain-v1` },
  )
}

async function analyzeBrandFilmBrief(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, ':analyze-brief'),
    'POST',
    { expected_revision: expectedRevision },
    { 'Idempotency-Key': `brand-film-analyze-${taskId}-${expectedRevision}` },
  )
}

async function updateBrandFilmBrief(projectId: string, taskId: string, expectedRevision: number, analysis: ApiBrandBriefAnalysis) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, '/brief'),
    'PATCH',
    { expected_revision: expectedRevision, analysis },
    { 'Idempotency-Key': brandFilmIdempotencyKey('brand-film-brief', taskId, expectedRevision) },
  )
}

async function confirmBrandFilmBrief(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, ':confirm-brief'),
    'POST',
    { expected_revision: expectedRevision },
    { 'Idempotency-Key': `brand-film-confirm-brief-${taskId}-${expectedRevision}` },
  )
}

async function generateBrandFilmConcepts(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, ':generate-concepts'),
    'POST',
    { expected_revision: expectedRevision },
    { 'Idempotency-Key': `brand-film-concepts-${taskId}-${expectedRevision}` },
  )
}

async function updateBrandFilmConcepts(projectId: string, taskId: string, expectedRevision: number, candidates: ApiBrandCreativeConcept[]) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, '/concepts'),
    'PATCH',
    { expected_revision: expectedRevision, candidates },
    { 'Idempotency-Key': brandFilmIdempotencyKey('brand-film-concepts-update', taskId, expectedRevision) },
  )
}

async function selectBrandFilmConcept(projectId: string, taskId: string, expectedRevision: number, conceptId: string) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, ':select-concept'),
    'POST',
    { expected_revision: expectedRevision, concept_id: conceptId },
    { 'Idempotency-Key': `brand-film-select-${taskId}-${expectedRevision}-${conceptId}` },
  )
}

async function generateBrandFilmPlan(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, ':generate-plan'),
    'POST',
    { expected_revision: expectedRevision },
    { 'Idempotency-Key': `brand-film-plan-${taskId}-${expectedRevision}` },
  )
}

async function updateBrandFilmPlan(projectId: string, taskId: string, expectedRevision: number, plan: ApiBrandFilmPlan) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, '/plan'),
    'PATCH',
    { expected_revision: expectedRevision, plan },
    { 'Idempotency-Key': brandFilmIdempotencyKey('brand-film-plan-update', taskId, expectedRevision) },
  )
}

async function confirmBrandFilmPlan(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(
    brandFilmPath(projectId, taskId, ':confirm-plan'),
    'POST',
    { expected_revision: expectedRevision },
    { 'Idempotency-Key': `brand-film-confirm-plan-${taskId}-${expectedRevision}` },
  )
}

async function prepareBrandFilmGeneration(projectId: string, taskId: string, expectedRevision: number, referenceAsset: ApiAssetVersionRef) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':prepare-generation'), 'POST', {
    expected_revision: expectedRevision,
    reference_asset: referenceAsset,
  }, { 'Idempotency-Key': `brand-film-prepare-${taskId}-${expectedRevision}-${referenceAsset.asset_id}-${referenceAsset.version}` })
}

async function generateBrandFilmUnit(projectId: string, taskId: string, expectedRevision: number, unitId: string, feedback = '') {
  const value = await creativeRequest<{ workspace: ApiBrandFilmWorkspace; provider_job: ApiProviderJobWire }>(
    brandFilmPath(projectId, taskId, ':generate-unit'), 'POST',
    { expected_revision: expectedRevision, unit_id: unitId, feedback, model_alias: 'cookies.video.standard' },
    { 'Idempotency-Key': `brand-film-unit-${taskId}-${unitId}-${Date.now()}` },
  )
  return { workspace: value.workspace, job: mapViralProviderJob(value.provider_job) }
}

async function getBrandFilmUnitJob(projectId: string, jobId: string) {
  return getViralVideoJob(projectId, jobId)
}

async function reconcileBrandFilmUnit(projectId: string, taskId: string, unitId: string, providerJobId: string) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':reconcile-unit'), 'POST', {
    unit_id: unitId,
    provider_job_id: providerJobId,
  }, { 'Idempotency-Key': `brand-film-reconcile-${taskId}-${providerJobId}` })
}

async function lockBrandFilmUnit(projectId: string, taskId: string, expectedRevision: number, unitId: string, attemptId: string) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':lock-unit'), 'POST', {
    expected_revision: expectedRevision,
    unit_id: unitId,
    attempt_id: attemptId,
  }, { 'Idempotency-Key': `brand-film-lock-${taskId}-${unitId}-${attemptId}` })
}

async function composeBrandFilmPreview(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':compose-preview'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-preview-${taskId}-${expectedRevision}` })
}

async function prepareBrandFilmAudio(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':prepare-audio'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-audio-${taskId}-${expectedRevision}` })
}

async function materializeBrandFilmAudioAssets(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':materialize-audio-assets'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-audio-assets-${taskId}-${expectedRevision}` })
}

async function updateBrandFilmAudioMix(projectId: string, taskId: string, expectedRevision: number, operations: ApiBrandAudioMixOperation[]) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, '/audio/mix'), 'PATCH', {
    expected_revision: expectedRevision,
    operations,
  }, { 'Idempotency-Key': brandFilmIdempotencyKey('brand-film-audio-mix', taskId, expectedRevision) })
}

async function selectBrandFilmAudioVariant(projectId: string, taskId: string, expectedRevision: number, variantId: string) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, '/audio:select-variant'), 'POST', {
    expected_revision: expectedRevision,
    variant_id: variantId,
  }, { 'Idempotency-Key': `brand-film-audio-variant-${taskId}-${expectedRevision}-${variantId}` })
}

async function renderBrandFilmAudioPreview(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':render-audio-preview'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-audio-preview-${taskId}-${expectedRevision}` })
}

async function probeBrandFilmSpeech(projectId: string) {
  return creativeRequest<ApiSpeechCapability>(`/projects/${encodeURIComponent(projectId)}/brand-film/speech-capability`)
}

async function generateBrandFilmVoiceClip(projectId: string, taskId: string, expectedRevision: number, clipId: string, voiceAlias: string) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':generate-voice'), 'POST', {
    expected_revision: expectedRevision,
    clip_id: clipId,
    voice_alias: voiceAlias,
  }, { 'Idempotency-Key': brandFilmIdempotencyKey('brand-film-voice', taskId, expectedRevision, clipId, voiceAlias) })
}

async function runBrandFilmQuality(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':run-quality'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-quality-${taskId}-${expectedRevision}` })
}

async function confirmBrandFilmQuality(projectId: string, taskId: string, expectedRevision: number, manualChecks: ApiBrandFilmManualCheck[]) {
  return creativeRequest<ApiBrandFilmWorkspace>(brandFilmPath(projectId, taskId, ':confirm-quality'), 'POST', {
    expected_revision: expectedRevision,
    manual_checks: manualChecks,
  }, { 'Idempotency-Key': `brand-film-confirm-quality-${taskId}-${expectedRevision}` })
}

async function finalizeBrandFilmVersion(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<{ workspace: ApiBrandFilmWorkspace; creative_version: ApiCreativeVersion }>(brandFilmPath(projectId, taskId, ':finalize-version'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-finalize-${taskId}-${expectedRevision}` })
}

async function approveBrandFilmVersion(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<{ workspace: ApiBrandFilmWorkspace; creative_version: ApiCreativeVersion }>(brandFilmPath(projectId, taskId, ':approve-version'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-approve-${taskId}-${expectedRevision}` })
}

async function deliverBrandFilmVersion(projectId: string, taskId: string, expectedRevision: number) {
  return creativeRequest<{ workspace: ApiBrandFilmWorkspace; creative_package: { id: string; creative_version_id: string; content_hash: string } }>(brandFilmPath(projectId, taskId, ':deliver-version'), 'POST', {
    expected_revision: expectedRevision,
  }, { 'Idempotency-Key': `brand-film-deliver-${taskId}-${expectedRevision}` })
}

async function ensureCommercePrerollFixtureWorkspace(
  projectId: string,
): Promise<ApiCommercePrerollWorkspace> {
  const assets = await ensureKanonGuerlainCommerceFixtureAssets(projectId)
  return creativeRequest<ApiCommercePrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-workspaces/commerce-preroll:ensure-fixture`,
    'POST',
    {
      template_ref: {
        template_id: 'commerce.window-reveal',
        template_version: 1,
      },
      product_asset_ref: assets.productAsset,
      first_frame_asset_ref: assets.firstFrame,
      last_frame_asset_ref: assets.lastFrame,
    },
    { 'Idempotency-Key': `commerce-fixture-${projectId}-guerlain-v1` },
  )
}

async function getLatestCommercePrerollWorkspace(
  projectId: string,
): Promise<ApiCommercePrerollWorkspace | null> {
  try {
    return await creativeRequest<ApiCommercePrerollWorkspace>(
      `/projects/${encodeURIComponent(projectId)}/creative-workspaces/commerce-preroll`,
    )
  } catch (cause) {
    if (cause instanceof CreativeApiError && cause.status === 404) return null
    throw cause
  }
}

async function updateCommercePrerollDraft(
  projectId: string,
  taskId: string,
  input: {
    expected_revision: number
    template_ref: { template_id: ApiCommerceTemplateId; template_version: 1 }
    fidelity?: string
    camera?: string
    motion?: string
    environment?: string
    result?: string
    guardrails?: string[]
  },
): Promise<ApiCommercePrerollWorkspace> {
  return creativeRequest<ApiCommercePrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/commerce-preroll-draft`,
    'PATCH',
    input,
    {
      'Idempotency-Key': [
        'commerce-draft',
        taskId,
        input.expected_revision,
        commerceRequestFingerprint(input),
      ].join('-'),
    },
  )
}

async function confirmCommercePrerollGeneration(
  projectId: string,
  taskId: string,
  expectedRevision: number,
): Promise<ApiCommercePrerollWorkspace> {
  return creativeRequest<ApiCommercePrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/commerce-preroll:confirm-generation`,
    'POST',
    { expected_revision: expectedRevision },
    { 'Idempotency-Key': `commerce-confirm-${taskId}-${expectedRevision}` },
  )
}

async function createCommercePrerollWorkspaceVideoJob(
  projectId: string,
  workspace: ApiCommercePrerollWorkspace,
): Promise<ApiGenerationJob> {
  const taskId = workspace.task.id
  const draft = workspace.video_draft.commerce_preroll
  const attemptOrdinal = (workspace.commerce_preroll_generation_attempts?.length ?? 0) + 1
  const job = await creativeRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}:video-job`,
    'POST',
    { model_alias: 'cookies.video.standard' },
    {
      'Idempotency-Key': [
        'commerce-video',
        taskId,
        draft.revision,
        attemptOrdinal,
        draft.plan.generation_spec.generation_spec_hash.slice(-12),
      ].join('-'),
    },
  )
  return mapViralProviderJob(job)
}

async function createManualGamePrerollWorkspace(
  projectId: string,
  input: ApiCreateManualGamePrerollInput,
): Promise<ApiGamePrerollWorkspace> {
  const intake = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes`,
    'POST',
    {
      source: 'manual',
      format: 'video',
      performance_mode: 'game_preroll',
      channel: 'douyin',
      objective: input.objective,
      audience: input.audience,
      core_message: input.coreMessage,
      call_to_action: input.callToAction,
      concept: input.briefName,
      tone: ['紧张', '清晰', '真实玩法'],
      visual_keywords: ['技能三选一', '波次推进', '竖屏塔防'],
      mandatory_elements: input.mandatoryElements,
      prohibited_claims: input.prohibitedClaims,
      creative_routes: [{
        route_id: 'route_manual_game_preroll_v1',
        route_type: 'game_preroll',
        video_purpose: 'performance',
        channels: ['douyin'],
        reason: '用户确认使用《保卫向日葵》授权实录固定样例跑通游戏前贴',
        target_duration_seconds: 6,
        aspect_ratio: '9:16',
        source_asset_refs: [input.sourceVideo],
        evidence_refs: input.evidenceMoments.map(moment => moment.id),
        requires_human_confirmation: true,
      }],
      manual_game_preroll: {
        brief_id: input.briefId,
        brief_version: input.briefVersion,
        brief_name: input.briefName,
        game_name: input.gameName,
        gameplay_summary: input.gameplaySummary,
        source_video: input.sourceVideo,
        source_video_rights: 'confirmed',
        evidence_moments: input.evidenceMoments,
        allowed_mechanisms: input.allowedMechanisms,
        prohibited_mechanisms: input.prohibitedMechanisms,
        subtitle_style: input.subtitleStyle,
        hook_strength: input.hookStrength,
        pace_profile: input.paceProfile,
      },
    },
    { 'Idempotency-Key': `manual-game-preroll-${Date.now()}-${Math.random().toString(36).slice(2)}` },
  )
  const task = await creativeRequest<{ id: string }>(
    `/projects/${encodeURIComponent(projectId)}/creative-intakes/${encodeURIComponent(intake.id)}:create-video-task`,
    'POST',
    {
      selected_route_id: 'route_manual_game_preroll_v1',
      channel: 'douyin',
      source_video: input.sourceVideo,
      concept: input.briefName,
      prompt: '等待人工选择游戏前贴候选后由服务端编译 PromptPackage',
      call_to_action: input.callToAction,
      mandatory_elements: input.mandatoryElements,
      prohibited_claims: input.prohibitedClaims,
      confirm_route: true,
    },
  )
  return creativeRequest<ApiGamePrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(task.id)}`,
  )
}

async function selectGamePrerollCandidate(
  projectId: string,
  taskId: string,
  expectedRevision: number,
  candidateId: string,
): Promise<ApiGamePrerollWorkspace> {
  return creativeRequest<ApiGamePrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/game-preroll:select-candidate`,
    'POST',
    { expected_revision: expectedRevision, candidate_id: candidateId },
    { 'Idempotency-Key': `game-preroll-select-${taskId}-${expectedRevision}-${candidateId}` },
  )
}

async function prepareGamePrerollEvidence(
  projectId: string,
  taskId: string,
  expectedRevision: number,
): Promise<ApiGamePrerollWorkspace> {
  return creativeRequest<ApiGamePrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/game-preroll:prepare-evidence`,
    'POST',
    { expected_revision: expectedRevision },
    { 'Idempotency-Key': `game-preroll-evidence-${taskId}-${expectedRevision}` },
  )
}

async function regenerateGamePrerollCandidates(
  projectId: string,
  taskId: string,
  expectedRevision: number,
): Promise<ApiGamePrerollWorkspace> {
  return creativeRequest<ApiGamePrerollWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/game-preroll:regenerate-candidates`,
    'POST',
    {
      expected_revision: expectedRevision,
      generation_config: {
        subtitle_style: 'high_contrast_dynamic',
        hook_strength: 4,
        pace_profile: 'punchy',
      },
    },
    { 'Idempotency-Key': `game-preroll-regenerate-${taskId}-${expectedRevision}` },
  )
}

async function createGamePrerollVideoJob(
  projectId: string,
  taskId: string,
  draftRevision: number,
  candidateId: string,
): Promise<ApiGenerationJob> {
  const job = await creativeRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}:video-job`,
    'POST',
    { model_alias: 'cookies.video.standard' },
    { 'Idempotency-Key': `game-preroll-video-${taskId}-${draftRevision}-${candidateId}` },
  )
  return mapViralProviderJob(job)
}

async function getLatestGamePrerollWorkspace(projectId: string): Promise<ApiGamePrerollWorkspace | null> {
  try {
    return await creativeRequest<ApiGamePrerollWorkspace>(
      `/projects/${encodeURIComponent(projectId)}/creative-workspaces/game-preroll`,
    )
  } catch (cause) {
    if (cause instanceof CreativeApiError && cause.status === 404) return null
    throw cause
  }
}

async function getGamePrerollVideoJob(projectId: string, jobId: string): Promise<ApiGenerationJob> {
  const job = await platformRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/model/jobs/${encodeURIComponent(jobId)}`,
  )
  return mapViralProviderJob(job)
}

async function getLatestViralRemakeWorkspace(projectId: string): Promise<ApiViralRemakeWorkspace | null> {
  const result = await creativeRequest<{ items: Array<{ id: string; performance_mode?: string; status: string }> }>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks?limit=100`,
  )
  const task = result.items.find(item => item.performance_mode === 'viral_remake' && item.status !== 'archived')
  if (!task) return null
  return creativeRequest<ApiViralRemakeWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(task.id)}/viral-remake`,
  )
}

async function getViralRemakeWorkspace(projectId: string, taskId: string): Promise<ApiViralRemakeWorkspace> {
  return creativeRequest<ApiViralRemakeWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/viral-remake`,
  )
}

async function analyzeViralRemake(projectId: string, taskId: string): Promise<ApiViralRemakeWorkspace> {
  return creativeRequest<ApiViralRemakeWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/viral-remake:analyze-reference`,
    'POST',
    undefined,
    { 'Idempotency-Key': `viral-analysis-${taskId}-${Date.now()}` },
  )
}

async function updateViralPrompt(
  projectId: string,
  taskId: string,
  expectedRevision: number,
  dimensions: Record<ApiVideoPromptDimension['id'], string>,
): Promise<ApiViralRemakeWorkspace> {
  return creativeRequest<ApiViralRemakeWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/viral-remake/prompt-draft`,
    'PATCH',
    { expected_revision: expectedRevision, dimensions },
  )
}

async function confirmViralGeneration(
  projectId: string,
  taskId: string,
  expectedRevision: number,
  confirmReferenceImageRights: boolean,
): Promise<ApiViralRemakeWorkspace> {
  return creativeRequest<ApiViralRemakeWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/viral-remake:confirm-generation`,
    'POST',
    {
      expected_revision: expectedRevision,
      confirm_reference_video_rights: true,
      confirm_reference_image_rights: confirmReferenceImageRights,
    },
    { 'Idempotency-Key': `viral-confirm-${taskId}-${expectedRevision}` },
  )
}

type ApiProviderJobWire = {
  id: string
  project_id: string
  kind: string
  execution_status: 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled'
  provider_status: string
  project_asset_refs: Array<{ asset_version: ApiAssetVersionRef }>
  error?: { message?: string }
  version: number
  created_at: string
  updated_at: string
}

function mapViralProviderJob(job: ApiProviderJobWire): ApiGenerationJob {
  const providerStatus = job.provider_status
  let status: ApiGenerationJob['status'] = 'queued'
  if (job.execution_status === 'failed' || providerStatus === 'failed' || providerStatus === 'expired') status = 'failed'
  else if (job.execution_status === 'cancelled' || providerStatus === 'cancelled') status = 'cancelled'
  else if (job.execution_status === 'succeeded' || providerStatus === 'succeeded' || providerStatus === 'partially_succeeded') status = 'succeeded'
  else if (job.execution_status === 'running' || providerStatus !== 'submitted') status = 'running'
  return {
    id: job.id,
    projectId: job.project_id,
    artifactKind: 'video',
    status,
    model: 'cookies.video.standard',
    diagnostic: job.error?.message,
    artifactId: job.project_asset_refs.at(-1)?.asset_version.asset_id,
    version: job.version,
    createdAt: job.created_at,
    updatedAt: job.updated_at,
  }
}

async function createViralVideoJob(projectId: string, taskId: string): Promise<ApiGenerationJob> {
  const job = await creativeRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}:video-job`,
    'POST',
    {},
    { 'Idempotency-Key': `viral-video-${taskId}-${Date.now()}` },
  )
  return mapViralProviderJob(job)
}

async function getViralVideoJob(projectId: string, jobId: string): Promise<ApiGenerationJob> {
  const job = await platformRequest<ApiProviderJobWire>(
    `/projects/${encodeURIComponent(projectId)}/model/jobs/${encodeURIComponent(jobId)}`,
  )
  return mapViralProviderJob(job)
}

async function submitViralCandidateReview(
  projectId: string,
  taskId: string,
  candidateId: string,
): Promise<ApiViralRemakeWorkspace> {
  return creativeRequest<ApiViralRemakeWorkspace>(
    `/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(taskId)}/viral-remake/candidates/${encodeURIComponent(candidateId)}:submit-review`,
    'POST',
  )
}

function projectQuery(projectId?: string): string {
  return projectId ? `?projectId=${encodeURIComponent(projectId)}` : ''
}

function prerollQuery(scope: ApiPrerollScope): string {
  const search = new URLSearchParams({
    projectId: scope.projectId,
    purpose: scope.purpose,
    prerollType: scope.prerollType,
  })
  return `?${search.toString()}`
}

function assetFeatureQuery(projectId: string, organizationId: string): string {
  const search = new URLSearchParams({ organizationId, projectId })
  return `?${search.toString()}`
}

export function buildHitAnalysisInput(sourceAsset: ApiAssetVersionRef, title: string, durationSeconds: number): ApiCreateHitAnalysisInput {
  return {
    source_asset: sourceAsset,
    title,
    duration_seconds: durationSeconds,
    language: 'zh-CN',
    notes: '由爆款复刻流程提交，先用视觉理解模型拆解五维视频提示词，再输入视频生成模型生成复刻视频。',
  }
}

export function buildLocalHitAnalysis(projectId: string, input: ApiCreateHitAnalysisInput): ApiHitAnalysis {
  const now = new Date().toISOString()
  const duration = Math.max(9, input.duration_seconds)
  const openingEnd = Math.max(3, Math.round(duration * 0.16))
  const problemEnd = Math.max(openingEnd + 3, Math.round(duration * 0.36))
  const proofEnd = Math.max(problemEnd + 3, Math.round(duration * 0.68))
  const offerEnd = Math.max(proofEnd + 2, Math.round(duration * 0.86))
  return {
    id: `local-hit-${input.source_asset.asset_id}-${Date.now()}`,
    organization_id: 'demo-org',
    project_id: projectId,
    source_asset: input.source_asset,
    title: input.title,
    video_meta: {
      duration_seconds: duration,
      language: input.language ?? 'zh-CN',
    },
    segments: [
      {
        id: 'seg-hook',
        start_seconds: 0,
        end_seconds: openingEnd,
        role: 'hook',
        summary: '用强冲突或高反差画面在开头建立停留理由。',
        script: '先别划走，这个结果和你想的不一样。',
        visual_element: '近景人物停顿、屏幕弹窗、快速推入主体。',
        conversion_cue: '建立注意力和问题意识。',
        replication_hint: '复刻开头停顿、反差和字幕强调，不复用原片人物和构图。',
      },
      {
        id: 'seg-problem',
        start_seconds: openingEnd,
        end_seconds: problemEnd,
        role: 'problem',
        summary: '放大受众痛点，让观众理解为什么需要继续看。',
        script: '如果你也遇到这个问题，先看这一步。',
        visual_element: '问题场景、失败瞬间、对比字幕。',
        conversion_cue: '让痛点与目标产品建立关联。',
        replication_hint: '保留问题升级节奏，替换为当前 Project 的业务场景。',
      },
      {
        id: 'seg-proof',
        start_seconds: problemEnd,
        end_seconds: proofEnd,
        role: 'proof',
        summary: '用过程、数据或细节镜头证明解决方案有效。',
        script: '关键不是更复杂，而是更稳定地做到。',
        visual_element: '产品特写、流程证明、结果对比、数据角标。',
        conversion_cue: '展示可信证据和卖点。',
        replication_hint: '复刻证明段功能，用授权素材和新产品卖点重建。',
      },
      {
        id: 'seg-offer',
        start_seconds: proofEnd,
        end_seconds: offerEnd,
        role: 'offer',
        summary: '将卖点转成可感知的利益点。',
        script: '现在就能把这套方法用到你的场景里。',
        visual_element: '利益点字幕、轻微加速剪辑、正向反馈画面。',
        conversion_cue: '降低行动成本。',
        replication_hint: '复刻利益转译方式，避免照搬原口播。',
      },
      {
        id: 'seg-cta',
        start_seconds: offerEnd,
        end_seconds: duration,
        role: 'cta',
        summary: '用清晰行动指令完成转化收口。',
        script: '点击预约，获取你的专属方案。',
        visual_element: 'CTA 定格、品牌露出、按钮式字幕。',
        conversion_cue: '引导点击或留资。',
        replication_hint: '保留清晰收口和停顿，不复制原片品牌资产。',
      },
    ],
    scripts: [
      { segment_id: 'seg-hook', text: '先别划走，这个结果和你想的不一样。' },
      { segment_id: 'seg-proof', text: '关键不是更复杂，而是更稳定地做到。' },
      { segment_id: 'seg-cta', text: '点击预约，获取你的专属方案。' },
    ],
    visual_elements: ['高反差开场', '问题场景', '产品证明', '数据角标', 'CTA 定格'],
    conversion_nodes: [
      { segment_id: 'seg-hook', cue: '停留' },
      { segment_id: 'seg-proof', cue: '信任' },
      { segment_id: 'seg-cta', cue: '行动' },
    ],
    replication_insights: ['复刻结构与镜头功能，不复刻原片具体表达。', '先建立冲突，再给证据，最后用明确 CTA 收口。'],
    created_at: now,
    updated_at: now,
  }
}

export function buildVideoReplicationPrompt(
  analysis: ApiHitAnalysis,
  input: {
    productName: string
    sellingPoints: string[]
    cta: string
    sourceFileName?: string
    referenceImageName?: string
    userInstruction?: string
  },
): ApiVideoReplicationPrompt {
  const opening = analysis.segments.find(segment => segment.role === 'hook') ?? analysis.segments[0]
  const proof = analysis.segments.find(segment => segment.role === 'proof') ?? analysis.segments[Math.min(2, analysis.segments.length - 1)]
  const ctaSegment = analysis.segments.find(segment => segment.role === 'cta') ?? analysis.segments.at(-1)
  const sellingPoints = input.sellingPoints.filter(Boolean)
  const sellingPointText = sellingPoints.length ? sellingPoints.join('、') : input.productName
  const rhythm = analysis.segments.map(segment => `${segment.start_seconds}-${segment.end_seconds}s ${segment.role}`).join('；')
  const instructionText = input.userInstruction?.trim()
  const imageText = input.referenceImageName?.trim()
  const multimodalReference = [
    '源视频用于复刻节奏、镜头功能和声音结构',
    instructionText ? `文本指令优先约束内容改写：${instructionText}` : '',
    imageText ? `参考图片用于约束主体外观、产品形态、色彩或构图气质：${imageText}` : '',
  ].filter(Boolean).join('；')
  const dimensions: ApiVideoPromptDimension[] = [
    {
      id: 'task_goal_type',
      label: '任务目标类型',
      prompt: `生成一条 ${analysis.video_meta.duration_seconds}s 左右的爆款复刻广告视频，目标是复刻源视频的停留结构、节奏推进和转化节点，同时将主题替换为「${input.productName}」。核心转化目标：${input.cta}。${instructionText ? `必须遵守文本指令：${instructionText}。` : ''}`,
      evidence: `源视频结构：${rhythm}`,
    },
    {
      id: 'quality_style_lighting',
      label: '画质&风格&光影规范',
      prompt: `保持高质感商业广告画质，画面干净锐利，主体边缘清晰；参考源视频的强对比停留点与 ${proof?.visual_element ?? '产品证明镜头'}，使用真实材质、高光轮廓、局部微距和可读字幕。${imageText ? `参考图片「${imageText}」用于校准主体形态、材质、配色和视觉气质。` : ''}避免低清、闪烁、畸变和过度相似的原片复制。`,
      evidence: [proof?.visual_element ?? analysis.visual_elements.join(' / '), imageText ? `参考图片：${imageText}` : ''].filter(Boolean).join('；'),
    },
    {
      id: 'environment_atmosphere',
      label: '环境氛围',
      prompt: `营造与「${input.productName}」匹配的可信商业场景：先制造问题压力，再进入解决方案展示，最后转向明确行动氛围；整体情绪从紧张、好奇推进到信任、确定。${instructionText ? `氛围表达需贴合文本指令中的限制和创意方向。` : ''}`,
      evidence: analysis.replication_insights.join('；'),
    },
    {
      id: 'camera_content',
      label: '镜头画面内容',
      prompt: `按源视频镜头功能复刻而非逐帧照抄：开场 ${opening?.summary ?? '用强钩子建立停留'}；中段展示 ${sellingPointText} 的证据镜头；结尾呈现 ${ctaSegment?.conversion_cue ?? input.cta}。镜头包含人物/产品近景、过程证明、字幕强调、CTA 定格。`,
      evidence: analysis.segments.map(segment => `${segment.role}: ${segment.summary}`).join('；'),
    },
    {
      id: 'music_sound',
      label: '音乐&音效',
      prompt: `音乐使用短视频平台高留存节奏：前 2 秒有清晰冲击音效，中段用递进鼓点承接证明信息，CTA 前加入短暂停顿和确认音；旁白与字幕语义一致，静音观看也能理解。`,
      evidence: analysis.scripts.map(script => script.text).join(' / '),
    },
  ]
  const compositePrompt = [
    `源视频参考：${analysis.title}${input.sourceFileName ? `（${input.sourceFileName}）` : ''}，Asset ${analysis.source_asset.asset_id} v${analysis.source_asset.version}。`,
    `多模态输入：${multimodalReference}。`,
    ...dimensions.map(dimension => `【${dimension.label}】${dimension.prompt}`),
    '生成要求：视频参考负责节奏和镜头功能，图片参考负责主体视觉，文本指令负责内容改写和约束；三者冲突时以文本指令和版权安全为最高优先级。不复制原视频人物、商标、字幕、画面构图或受版权保护的表达。',
  ].join('\n')
  return {
    source_asset: analysis.source_asset,
    source_title: analysis.title,
    source_file_name: input.sourceFileName,
    reference_image_name: input.referenceImageName,
    user_instruction: instructionText,
    dimensions,
    composite_prompt: compositePrompt,
    model_directive: `multimodal video remake: source video + reference image + text instruction；输出 ${analysis.video_meta.duration_seconds}s，9:16 竖版，目标产品 ${input.productName}，CTA ${input.cta}`,
  }
}

export function buildProductMappingInput(
  analysis: Pick<ApiHitAnalysis, 'id'>,
  targetProduct: ApiProductProfile,
  targetAssets: {
    hook: ApiAssetVersionRef
    proof: ApiAssetVersionRef
    cta: ApiAssetVersionRef
  },
): ApiCreateProductMappingInput {
  return {
    hit_analysis_id: analysis.id,
    target_product: targetProduct,
    required_assets: [targetAssets.hook, targetAssets.proof, targetAssets.cta],
    replacement_rules: [
      { role: 'hook', target_asset: targetAssets.hook, message: `${targetProduct.name}：先用最高差异卖点制造停留。` },
      { role: 'proof', target_asset: targetAssets.proof, message: `${targetProduct.selling_points[0] ?? targetProduct.name}：用授权素材重建证明段。` },
      { role: 'cta', target_asset: targetAssets.cta, message: targetProduct.cta },
    ],
    constraints: ['不得默认复用原视频二进制', '仅引用当前 Project 授权素材'],
    target_seconds: 30,
    pace: 'balanced',
  }
}

export function buildRemixPrerollInput(
	planId: string,
	hookType: ApiRemixHookType,
	referenceAsset: ApiAssetVersionRef,
	mode: ApiPrerollMode,
	durationSeconds = 6,
	styleConstraints: string[] = ['9:16 竖版', '静音可理解', '保留 opening 拼接点'],
): ApiCreateRemixPrerollInput {
	return {
		plan_id: planId,
		hook_type: hookType,
		reference_asset: referenceAsset,
		style_constraints: styleConstraints,
		duration_seconds: durationSeconds,
		mode,
	}
}

export type ApiMiyunConnection = {
  id: string; organization_id: string; project_id: string
  status: 'unverified' | 'ready' | 'auth_required' | 'disabled'
  session_expires_at?: string; last_verified_at?: string; last_successful_request_at?: string; cooldown_until?: string
  last_error_kind?: string; last_error_code?: string; last_error_at?: string; version: number; created_by: string; created_at: string; updated_at: string
}
export type ApiOceanEngineSession = {
  id: string; organization_id: string; project_id: string
  status: 'unverified' | 'ready' | 'auth_required' | 'disabled'
  credential_ref_present: boolean
  last_verified_at?: string; last_successful_request_at?: string
  cooldown_until?: string; last_error_kind?: string; last_error_code?: string; last_error_at?: string
  version: number; created_by: string; created_at: string; updated_at: string
}
export type ApiConnectorAccount = {
  id: string; organization_id: string; project_id?: string
  platform: 'ocean_engine'; display_label: string
  status: 'pending' | 'verified' | 'revoked' | 'blocked'
  verified_at?: string; last_checked_at?: string; credential_ref_present: boolean
}
export type ApiConnectorAccountSession = {
  id: string; organization_id: string; account_id: string
  status: 'unverified' | 'ready' | 'auth_required' | 'disabled'
  credential_ref_present: boolean; last_verified_at?: string
  version: number; created_at: string; updated_at: string
}
export type ApiConnectorSyncResult = {
  run_id: string; replayed: boolean; object_count: number; metric_count: number
  platform_objects?: Partial<Record<ApiConnectorPlatformObjectKind, { created: number; updated: number; unchanged: number; unavailable: number }>>
}
export type ApiConnectorSyncStatus = {
  id: string; organization_id: string; project_id?: string; account_ref: string
  status: 'queued' | 'running' | 'completed' | 'failed'; cursor?: string
  attempt: number; started_at: string; completed_at?: string
}
export type ApiOptimizationTargetContext = {
  campaign_type: number; landing_type: number; asset_type: number
  micro_app_id: string; cdp_marketing_goal: number; dpa_ad_type: number
  micro_promotion_type: number; micro_app_instance_id: string
  multi_asset_types?: number[]; need_assets: boolean
}
export type ApiOptimizationTargetCapability = {
  external_action: string; semantic_key: string; display_name: string
  optimization_event_type?: string; asset_types?: string[]; track_types?: string[]
  is_gray: boolean; deep_goal_required: boolean; need_assets: boolean
  limits: { delivery_modes?: number[]; auto_ad_types?: number[]; delivery_packages?: number[] }
  event_assets?: Array<{ asset_id: string; asset_name?: string; role?: string }>
}
export type ApiOptimizationTargetCapabilitySnapshot = {
  schema_version: 'oceanengine-optimization-target-capability/v1'
  snapshot_id: string; account_id: string; context: ApiOptimizationTargetContext; context_hash: string
  options: ApiOptimizationTargetCapability[]; asset_ids?: string[]; show_other: boolean; observed_at: string
}

export type ApiOceanEngineAccountCapabilitySnapshot = {
  schema_version: 'oceanengine-account-capability/v1'
  snapshot_id: string
  account_id: string
  external_actions: Array<{ key: string; display_name: string; value: string; step?: string; default?: boolean }>
  deep_external_actions: Array<{ key: string; display_name: string; value: string; step?: string; default?: boolean }>
  creative_components: Array<{
    component_type_ids: string[]
    access_flags?: string[]
    landing_types?: string[]
    campaign_types?: string[]
    inventory_types?: string[]
    inventory_catalogs?: string[]
    image_modes?: string[]
    content_types?: string[]
  }>
  budget_rules?: Record<string, unknown>
  bid_constraints?: Record<string, unknown>
  quotas?: Record<string, unknown>
  feature_rules?: Record<string, unknown>
  orange_site_domains?: string[]
  interfaces?: Array<{ method?: string; path: string; description?: string; empty_events?: string[] }>
  observed_at: string
}
export type ApiConnectorPlatformObjectKind = 'image_material' | 'product_image' | 'video_material' | 'aweme_photo_material' | 'marketing_product' | 'orange_landing_page' | 'optimization_target' | 'conversion_event_asset' | 'industry_category' | 'brand' | 'authorized_identity'
export type ApiConnectorPlatformObject = {
  id: string; organization_id: string; account_id: string
  object_kind: ApiConnectorPlatformObjectKind; platform_object_id: string
  display_name: string; status: 'active' | 'unavailable'; metadata: Record<string, unknown>
  observed_at: string; version: number; project_granted: true
  preview_available: boolean; preview_kind?: 'image' | 'video_poster' | 'landing_page'
  preview_expires_at?: string; preview_url?: string
  performance?: {
    available: boolean; spend_minor: number; impressions: number; clicks: number
    conversions: number; ctr: number; data_through?: string
  }
}

export type ApiConnectorObjectSnapshot = {
  id: string
  object_kind: 'account' | 'project' | 'campaign' | 'promotion' | 'product' | 'material' | string
  object_ref: string
  parent_ref?: string
  state: Record<string, unknown>
  available_at: string
  data_through: string
  quality_status: string
}

export type ApiConnectorCanonicalSnapshot = {
  dataset_version: string
  prediction_cutoff: string
  objects: ApiConnectorObjectSnapshot[]
}
export type ApiLaunchBatchMetricDistribution = { metric: string; p10: number; p50: number; p90: number }
export type ApiLaunchBatchCalibration = {
  id: string; account_id: string; schema_version: string; model_version: string; status: 'ready_for_probabilistic_shadow'
  training_batches: number; training_dates: number; evaluation_batches: number; evaluation_dates: number
  breakout_threshold_minor: number; breakout_probability: number
  typical: ApiLaunchBatchMetricDistribution[]; breakout: ApiLaunchBatchMetricDistribution[]
  brier_score: number; calibration_error: number; final_brier_score: number; final_calibration_error: number
  created_at: string
}

export type ApiMiyunAssetVersionRef = { asset_id: string; version: number }
export type ApiMediaUnderstandingArtifact = {
  id: string
  status: 'running' | 'ready' | 'partial' | 'failed'
  error_message?: string
  warnings: string[]
}
export type ApiMediaUnderstandingCapabilities = {
  vision_semantic_enabled: boolean
  asr_enabled: boolean
  vision_model_alias: string
  profile_version: string
}
export type ApiMiyunProductSource = {
  project_name: string
  brand_name: string
  category_name: string
  products: Array<{ id: string; name: string }>
}
export type ApiMiyunProfileQuery = { product_name: string; category_id?: string; category_name?: string; keywords: string[]; material_types: string[]; material_content_types: string[]; window_start: string; window_end: string }
export type ApiMiyunProfileFieldSource = { field: string; source_kind: string; source_refs: string[]; confidence: 'high' | 'medium' | 'low' | 'unknown'; review_state: 'suggested' | 'unknown' | 'human_confirmed'; explanation: string }
export type ApiMiyunProductProfile = {
  id: string; organization_id: string; project_id: string; connection_id: string; status: 'draft' | 'confirmed' | 'superseded'; product_id: string; product_name: string; brand_name?: string; category_id?: string; category_name?: string
  keywords: string[]; material_types?: string[]; material_content_types: string[]; window_start: string; window_end: string; project_context_version: number; product_asset_refs: ApiMiyunAssetVersionRef[]; knowledge_document_ids: string[]
  rule_version: 'miyun-product-profile-rules/v1' | 'miyun-product-profile-rules/v2'; model_version?: string; analysis_method: 'rules'; input_hash: string; input_snapshot: Record<string, unknown>; field_sources: ApiMiyunProfileFieldSource[]; analysis_warnings: string[]
  confirmed_by?: string; confirmed_at?: string; version: number; created_by: string; created_at: string; updated_at: string
}
export type ApiMiyunCrawlJob = {
  id: string; organization_id: string; project_id: string; connection_id: string; product_profile_id: string
  status: 'queued' | 'running' | 'cooling_down' | 'auth_required' | 'partial' | 'succeeded' | 'failed' | 'cancelled'; operation: 'product' | 'cid'; query_schema_version: 'miyun-query/v1'; query_snapshot: Record<string, unknown>
  runtime_job_id: string; completed_pages: number; discovered_count: number; deduplicated_count: number; downloaded_count: number; failed_count: number; cooldown_until?: string; last_error_kind?: string; last_error_code?: string; version: number; created_by: string; created_at: string; updated_at: string
}
export type ApiMiyunMaterial = {
  id: string; organization_id: string; project_id: string; miyun_material_id: string; first_seen_crawl_job_id?: string; import_method: 'crawler' | 'manual'; resource_id?: string; resource_expected_size?: number
  source_ref?: string; source_ref_status: 'verified' | 'unknown'; title?: string; selection_status: 'discovered' | 'confirmed' | 'rejected'; import_status: 'pending' | 'downloading' | 'imported' | 'deduplicated' | 'failed' | 'skipped'
  platform_asset_id?: string; platform_asset_version?: number; insight_asset_id?: string; external_import_id?: string; decision_by?: string; decision_at?: string; decision_note?: string; last_import_error_kind?: string; last_import_error_code?: string; version: number; created_by: string; created_at: string; updated_at: string
}
export type ApiMiyunMaterialSnapshot = {
  id: string; organization_id: string; project_id: string; material_id: string; crawl_job_id?: string; source_page: number; import_method: 'crawler' | 'manual'; schema_version: string; captured_at: string; first_published_at?: string; last_published_at?: string
  delivery_days: number; cumulative_impressions: number; cumulative_impressions_raw: string; related_ads: number; related_creators: number; related_creators_raw: string; related_creators_known: boolean; material_score: number; views: number; likes: number; comments: number; shares: number; saves: number; sanitized_raw?: Record<string, unknown>; created_at: string
}
export type ApiMiyunMaterialDetail = { material: ApiMiyunMaterial; snapshots: ApiMiyunMaterialSnapshot[] }
  export type ApiMiyunHandoffReturn = {
    id: string; handoff_id: string; handoff_version: number; manifest_version: string; input_hash: string; parameter_version: string; product_profile_id: string
    crawl_job_id?: string; source_material_id?: string; association_source: 'crawl_job' | 'filename' | 'manifest_xlsx'; container_filename?: string
  status: 'created' | 'uploaded' | 'failed' | 'returned'; filename?: string; asset_version?: { asset_id: string; version: number }; mime_type?: 'video/mp4'; sha256?: string; size_bytes?: number; insight_asset_id?: string; uploaded_by?: string; uploaded_at?: string; failure_code?: string; returned_by?: string; returned_at?: string; version: number; created_at: string; updated_at: string
}
export type ApiMiyunHandoff = {
  id: string; organization_id: string; project_id: string; source_material_id: string; source_material_ids: string[]; product_profile_id: string; crawl_job_id?: string
  status: 'exporting' | 'exported' | 'delivered' | 'returned' | 'failed'; manifest_version: string; parameter_version: string
  product_files_snapshot: Record<string, unknown>; source_snapshot: Record<string, unknown>; profile_snapshot: Record<string, unknown>; input_hash: string
  version: number; created_by: string; created_at: string; updated_at: string; returns?: ApiMiyunHandoffReturn[]
}

export const api = {
  listAgencyWorkbench: async (options: AgencyWorkbenchOptions = {}) => {
    // Workbench data always follows the caller's accessible Project scope.
    // includeDemoProject is retained as a compatibility hint, but it must not
    // introduce a hard-coded Project outside the current identity.
    const projectIds = options.projectIds ?? []
    return loadPersistedAgencyWorkbench(projectIds)
  },
  getSession: async () => authSessionFromActor((await platformRequest<PlatformRequestContext>('/context')).actor),
  login: async (input: { username: string; password: string }) => {
    const result = await platformRequest<PlatformLoginResult>('/auth/login', 'POST', {
      username: input.username,
      password: input.password,
    })
    return authSessionFromActor(result.actor, input.username)
  },
  logout: async () => {
    await platformRequest<void>('/auth/logout', 'POST')
    return { authenticated: false }
  },
  getCapabilities: getKanonCapabilities,
  getProviderConfiguration: () => request<ApiProviderConfiguration>('/provider/configuration'),
  updateProviderConfiguration: (input: { apiKey: string; baseUrl?: string }) =>
    request<ApiProviderConfiguration>('/provider/configuration', 'PUT', input),
  deleteProviderConfiguration: () => request<ApiProviderConfiguration>('/provider/configuration', 'DELETE'),
  getPublicInsightOverview: () => request<ApiPublicInsightOverview>('/public-insights/overview'),
  getPublicInsightFilters: () => request<ApiPublicInsightFilters>('/public-insights/filters'),
  listPublicInsightVideos: (input: {
    page?: number
    pageSize?: number
    keyword?: string
    industry?: string
    aiGenerated?: string
    visualStyle?: string
    sortBy?: string
    sortOrder?: 'asc' | 'desc'
  } = {}) => {
    const search = new URLSearchParams({
      page: String(input.page ?? 1),
      page_size: String(input.pageSize ?? 20),
      keyword: input.keyword ?? '',
      industry: input.industry ?? '',
      ai_generated: input.aiGenerated ?? '全部',
      visual_style: input.visualStyle ?? '',
      sort_by: input.sortBy ?? 'vv_all',
      sort_order: input.sortOrder ?? 'desc',
    })
    return request<ApiPublicInsightVideoPage>(`/public-insights/videos?${search.toString()}`)
  },
  getPublicInsightVideo: (itemId: string) =>
    request<ApiPublicInsightVideoDetail>(`/public-insights/videos/${encodeURIComponent(itemId)}`),
  listProjects: () => platformClient.listProjects(),
  getProjectSnapshot: (projectId: string) => platformClient.getProjectSnapshot(projectId),
  listProjectMediaAssets: (projectId: string) => platformClient.listProjectMediaAssets(projectId),
  runMaterialQualityCheck: async (projectId: string, assetId: string, version: number) => {
    const item = await platformRequest<{
      id: string
      organization_id: string
      project_id: string
      asset_id: string
      asset_version: number
      status: string
      model: string
      rule_version: string
      prompt_version: string
      summary: string
      issues: Array<{ id: string; severity: string; rule: string; evidence: string; suggestion: string }>
      created_at: string
      completed_at?: string
    }>(
      `/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(assetId)}/versions/${version}/quality-checks`,
      'POST',
      {},
      { 'Idempotency-Key': `material-qc-${projectId}-${assetId}-${version}` },
    )
    return {
      id: item.id,
      organizationId: item.organization_id,
      projectId: item.project_id,
      assetId: item.asset_id,
      assetVersion: item.asset_version,
      status: item.status as ApiQualityCheckStatus,
      model: item.model,
      ruleVersion: item.rule_version,
      promptVersion: item.prompt_version,
      summary: item.summary,
      issues: item.issues.map(issue => ({ ...issue, severity: issue.severity as ApiQualityIssueSeverity })),
      createdAt: item.created_at,
      completedAt: item.completed_at,
    } satisfies ApiQualityCheckRun
  },
  recordMaterialConfirmation: async (projectId: string, assetId: string, version: number, input: { status: ApiMaterialConfirmationStatus; scope: string; note: string }) => {
    const item = await platformRequest<{
      id: string
      organization_id: string
      project_id: string
      quality_check_run_id: string
      asset_id: string
      asset_version: number
      status: string
      scope: string
      confirmed_by: string
      note: string
      created_at: string
    }>(
      `/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(assetId)}/versions/${version}/confirmations`,
      'POST',
      input,
      { 'Idempotency-Key': `material-confirm-${projectId}-${assetId}-${version}-${input.status}` },
    )
    return {
      id: item.id,
      organizationId: item.organization_id,
      projectId: item.project_id,
      qualityCheckRunId: item.quality_check_run_id,
      assetId: item.asset_id,
      assetVersion: item.asset_version,
      status: item.status as ApiMaterialConfirmationStatus,
      scope: item.scope,
      confirmedBy: item.confirmed_by,
      note: item.note,
      createdAt: item.created_at,
    } satisfies ApiMaterialConfirmation
  },
  setMaterialDeliveryVersion: (projectId: string, assetId: string, version: number) =>
    platformRequest(
      `/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(assetId)}/version-pointer`,
      'PATCH',
      { delivery_version: version },
      { 'Idempotency-Key': `material-delivery-${projectId}-${assetId}-${version}` },
    ),
  createProject: (input: Pick<ApiProject, 'name' | 'brand' | 'objective' | 'industry'>) =>
    platformClient.createProject(input),
  updateProject: (id: string, input: Partial<Pick<ApiProject, 'name' | 'brand' | 'objective' | 'industry'>> & { expectedContextVersion?: number }) =>
    platformClient.updateProject(id, input),
  listArtifacts: (projectId?: string) =>
    projectId ? platformClient.listArtifacts(projectId) : Promise.resolve([]),
  listPrerollArtifacts: async (scope: ApiPrerollScope) =>
    (await listKanonArtifacts(scope.projectId))
      .filter(artifact => artifact.kind === 'video')
      .map(artifact => ({ ...artifact, purpose: scope.purpose, prerollType: scope.prerollType })),
  listAssetFeatures: (_projectId: string, _organizationId = 'demo-org') =>
    Promise.resolve<{ items: ApiAssetFeature[] }>({ items: [] }),
  listTasks: (projectId?: string) =>
    projectId ? listKanonTasks(projectId) : Promise.resolve([]),
  getTask: (id: string) =>
    request<ApiBusinessTask>(`/tasks/${encodeURIComponent(id)}`),
  createTask: (input: {
    projectId: string
    type: ApiBusinessTaskType
    name: string
    objective: string
    sourceTaskIds?: string[]
    sourceArtifactIds?: string[]
  }) => Promise.reject<ApiBusinessTask>(unsupportedKanonWrite(`“${input.name}”任务创建`)),
  updateTask: (
    projectId: string,
    id: string,
    input: Partial<Pick<ApiBusinessTask, 'name' | 'objective' | 'status' | 'sourceTaskIds' | 'sourceArtifactIds' | 'outputArtifactIds'>>,
  ) => Promise.reject<ApiBusinessTask>(unsupportedKanonWrite(`任务 ${id} 更新`)),
  createArtifact: (input: {
    projectId: string
    kind: ApiArtifact['kind']
    content: string
    status?: ApiArtifact['status']
    sourceJobId?: string
  }) => platformClient.createArtifact(input.projectId, input),
  updateArtifact: (
    projectId: string,
    id: string,
    input: Partial<Pick<ApiArtifact, 'content' | 'status' | 'sourceJobId' | 'version'>>,
  ) => platformClient.updateArtifact(projectId, id, input),
  listJobs: (projectId?: string) =>
    projectId ? listKanonJobs(projectId) : Promise.resolve([]),
  listPrerollJobs: async (scope: ApiPrerollScope) =>
    (await listKanonJobs(scope.projectId))
      .filter(job => job.artifactKind === 'video')
      .map(job => ({ ...job, purpose: scope.purpose, prerollType: scope.prerollType })),
  getJob: getKanonJob,
  getPrerollJob: async (id: string, scope: ApiPrerollScope) => ({
    ...await getKanonJob(id),
    purpose: scope.purpose,
    prerollType: scope.prerollType,
  }),
  cancelJob: async (id: string, _scope?: ApiPrerollScope) =>
    Promise.reject<ApiGenerationJob>(unsupportedKanonWrite(`模型作业 ${id} 取消`)),
  generateBrief: async (_projectId: string, _prompt: string) =>
    createKanonBrief(_projectId, _prompt),
  confirmBrief: (artifact: ApiArtifact) => confirmKanonBrief(artifact),
  attachBriefProductAsset: (artifact: ApiArtifact, asset: ApiAssetVersionRef) =>
    attachKanonBriefProductAsset(artifact, asset),
  createMedia: (
    projectId: string,
    kind: 'image' | 'video',
    prompt: string,
    briefId: string,
  ) => createKanonMedia(projectId, kind, prompt, briefId),
  createCommercePrerollVideo: (
    projectId: string,
    prompt: string,
    briefId: string,
  ) => createKanonCommercePrerollVideo(projectId, prompt, briefId),
  createPreparedCommercePrerollVideo: (
    projectId: string,
    prompt: string,
    sourceId: string,
    productAsset: { asset_id: string; version: number },
  ) => createKanonPreparedCommercePrerollVideo(projectId, prompt, sourceId, productAsset),
  listCommercePrerollSources: (projectId: string) =>
    listKanonCommercePrerollSources(projectId),
  prepareCommercePreroll: (
    projectId: string,
    source: ApiCreativeSourceOption,
    templateId: ApiCommerceTemplateId,
  ) => prepareKanonCommercePreroll(projectId, source, templateId),
  createPrerollVideo: (
    scope: ApiPrerollScope,
    prompt: string,
    briefId: string,
  ) => createKanonMedia(scope.projectId, 'video', prompt, briefId)
    .then(job => ({ ...job, purpose: scope.purpose, prerollType: scope.prerollType })),
  planShortDramaPreroll: async (
    _projectId: string,
    _briefId: string,
    _storyContext: ApiShortDramaStoryContext,
  ) => Promise.reject<ApiShortDramaPrerollPlan>(
    unsupportedKanonWrite('短剧前贴候选规划'),
  ),
  createShortDramaPrerollVideo: (
    scope: ApiPrerollScope & { prerollType: 'short_drama' },
    briefId: string,
    planVersion: ApiShortDramaPrerollPlan['version'],
    candidateId: string,
    storyContext: ApiShortDramaStoryContext,
  ) => createKanonMedia(
    scope.projectId,
    'video',
    [
      `短剧：${storyContext.title}`,
      storyContext.synopsis,
      `已审核卖点：${storyContext.reviewedSellingPoints.join('；')}`,
      storyContext.openingLine ? `开场台词：${storyContext.openingLine}` : '',
      `候选方案：${candidateId}（${planVersion}）`,
      '生成 9:16、独立 6 秒、静音可理解的短剧广告前贴，并以清晰 CTA 收束。',
    ].filter(Boolean).join('。'),
    briefId,
  ).then(job => ({ ...job, purpose: scope.purpose, prerollType: scope.prerollType })),
  listAuditEvents: (projectId?: string) =>
    projectId ? platformClient.listAuditEvents(projectId) : Promise.resolve([]),
  listOperations: (projectId: string) =>
    request<ApiOperationalRecord[]>(`/projects/${encodeURIComponent(projectId)}/operations`),
  listRemixEvalCases: (projectId: string) =>
    platformRequest<{ items: ApiRemixEvalCase[] }>(`/projects/${encodeURIComponent(projectId)}/remix-eval-cases`),
  createRemixEvalRun: (projectId: string, input: {
    planner_version: string
    prompt_version: string
    submissions: ApiRemixEvalSubmission[]
  }) => platformRequest<ApiRemixEvalRun>(`/projects/${encodeURIComponent(projectId)}/remix-eval-runs`, 'POST', input),
  getRemixEvalRun: (projectId: string, runId: string) =>
    platformRequest<ApiRemixEvalRun>(`/projects/${encodeURIComponent(projectId)}/remix-eval-runs/${encodeURIComponent(runId)}`),
  createRemixRenderJob: (projectId: string, input: {
    plan_id: string
    target_format?: 'mp4'
    target_quality?: 'draft' | 'standard' | 'high'
  }, idempotencyKey: string) =>
    platformRequest<ApiRemixRenderJob>(
      `/projects/${encodeURIComponent(projectId)}/remix-render-jobs`,
      'POST',
      input,
      { 'Idempotency-Key': idempotencyKey },
    ),
  getRemixRenderJob: (projectId: string, jobId: string) =>
    platformRequest<ApiRemixRenderJob>(`/projects/${encodeURIComponent(projectId)}/remix-render-jobs/${encodeURIComponent(jobId)}`),
  createRemixQualityReport: (projectId: string, input: {
    render_job_id: string
    output_asset?: ApiQualityReport['output_asset']
    policy?: 'fail_critical' | 'review_all_issues'
  }) => platformRequest<ApiQualityReport>(`/projects/${encodeURIComponent(projectId)}/remix-quality-reports`, 'POST', input),
  getRemixRenderJobQualityReport: (projectId: string, jobId: string) =>
    platformRequest<{ quality_report: ApiQualityReport | null }>(`/projects/${encodeURIComponent(projectId)}/remix-render-jobs/${encodeURIComponent(jobId)}/quality-report`),
  createHitAnalysis: (projectId: string, input: ApiCreateHitAnalysisInput) =>
    platformRequest<ApiHitAnalysis>(`/projects/${encodeURIComponent(projectId)}/remix-hit-analyses`, 'POST', input),
  uploadProjectAsset,
  createManualShortDramaPrerollV2Workspace,
  getShortDramaPrerollV2Workspace,
  analyzeShortDramaV2Source,
  updateShortDramaV2Analysis,
  generateShortDramaV2Directions,
  selectShortDramaV2Direction,
  updateShortDramaV2Prompts,
  prepareShortDramaV2OpeningFrame,
  generateShortDramaReferenceBoards,
  reconcileShortDramaReferenceBoard,
  retryShortDramaReferenceBoardCandidate,
  selectShortDramaReferenceBoard,
  generateShortDramaV2FirstFrames,
  reconcileShortDramaV2FirstFrame,
  selectShortDramaV2FirstFrame,
  bindShortDramaV2TrustedMaterials,
  generateShortDramaV2Video,
  reconcileShortDramaV2Video,
  getShortDramaV2ProviderJob,
  createManualViralRemakeWorkspace,
  createManualShortDramaPrerollWorkspace,
  getTaskStrategyCreativeIntake,
  getCreativeTaskHandoffDetail,
  listCreativeTasks,
  renameCreativeTask,
  listCreativeVersions,
  getBrandFilmWorkspace,
  initializeStrategyBrandFilmWorkspace,
  restoreBrandFilmWorkspace,
  listCreativeIntakes,
  uploadKnowledgeDocument,
  getKnowledgeDocument,
  extractKnowledgeDocumentMedia,
  createManualBrandFilmIntake,
  ensureBrandFilmFixtureWorkspace,
  analyzeBrandFilmBrief,
  updateBrandFilmBrief,
  confirmBrandFilmBrief,
  generateBrandFilmConcepts,
  updateBrandFilmConcepts,
  selectBrandFilmConcept,
  generateBrandFilmPlan,
  updateBrandFilmPlan,
  confirmBrandFilmPlan,
  prepareBrandFilmGeneration,
  generateBrandFilmUnit,
  getBrandFilmUnitJob,
  reconcileBrandFilmUnit,
  lockBrandFilmUnit,
  composeBrandFilmPreview,
  prepareBrandFilmAudio,
  materializeBrandFilmAudioAssets,
  updateBrandFilmAudioMix,
  selectBrandFilmAudioVariant,
  renderBrandFilmAudioPreview,
  probeBrandFilmSpeech,
  generateBrandFilmVoiceClip,
  runBrandFilmQuality,
  confirmBrandFilmQuality,
  finalizeBrandFilmVersion,
  approveBrandFilmVersion,
  deliverBrandFilmVersion,
  getImageTextWorkspace,
  getCreativeIntake,
  prepareBrandBriefReview,
  getStrategyBrandWorkflow,
  prepareStrategyBrandWorkflow,
  updateBrandBriefReview,
  confirmBrandBriefReview,
  createManualImageTextIntake,
  generateCreativeDirections,
  getLatestCreativeDirectionBatch,
  confirmCreativeDirection,
  createImageTextTaskFromDirection,
  createBrandVideoTaskFromDirection,
  createBrandFilmTaskFromIntake,
  generateImageTextDraft,
  updateImageTextDraft,
  generateImageTextSlot,
  adoptImageTextAttempt,
  getProjectAssetPreview,
  listImageTextVersions,
  listCreativePackages,
  freezeImageTextVersion,
  checkImageTextVersion,
  approveImageTextVersion,
  deliverImageTextVersion,
  selectShortDramaPrerollCandidate,
  regenerateShortDramaPrerollCandidates,
  createShortDramaPrerollVideoJob,
  getShortDramaPrerollVideoJob,
  getLatestShortDramaPrerollWorkspace,
  ensureCommercePrerollFixtureWorkspace,
  getLatestCommercePrerollWorkspace,
  updateCommercePrerollDraft,
  confirmCommercePrerollGeneration,
  createCommercePrerollWorkspaceVideoJob,
  createManualGamePrerollWorkspace,
  prepareGamePrerollEvidence,
  selectGamePrerollCandidate,
  regenerateGamePrerollCandidates,
  createGamePrerollVideoJob,
  getLatestGamePrerollWorkspace,
  getGamePrerollVideoJob,
  getLatestViralRemakeWorkspace,
  getViralRemakeWorkspace,
  analyzeViralRemake,
  updateViralPrompt,
  confirmViralGeneration,
  createViralVideoJob,
  getViralVideoJob,
  submitViralCandidateReview,
  getHitAnalysis: (projectId: string, analysisId: string) =>
    platformRequest<ApiHitAnalysis>(`/projects/${encodeURIComponent(projectId)}/remix-hit-analyses/${encodeURIComponent(analysisId)}`),
  createProductMapping: (projectId: string, input: ApiCreateProductMappingInput) =>
    platformRequest<ApiProductMapping>(`/projects/${encodeURIComponent(projectId)}/remix-product-mappings`, 'POST', input),
  getProductMapping: (projectId: string, mappingId: string) =>
    platformRequest<ApiProductMapping>(`/projects/${encodeURIComponent(projectId)}/remix-product-mappings/${encodeURIComponent(mappingId)}`),
  generatePlanFromProductMapping: (projectId: string, mappingId: string) =>
    platformRequest<ApiRemixPlan>(`/projects/${encodeURIComponent(projectId)}/remix-product-mappings/${encodeURIComponent(mappingId)}/plans`, 'POST'),
  createRemixPreroll: (projectId: string, input: ApiCreateRemixPrerollInput) =>
    platformRequest<ApiRemixPreroll>(`/projects/${encodeURIComponent(projectId)}/remix-prerolls`, 'POST', input),
  getRemixPreroll: (projectId: string, prerollId: string) =>
    platformRequest<ApiRemixPreroll>(`/projects/${encodeURIComponent(projectId)}/remix-prerolls/${encodeURIComponent(prerollId)}`),
  applyRemixPreroll: (projectId: string, prerollId: string) =>
    platformRequest<ApiRemixPlan>(`/projects/${encodeURIComponent(projectId)}/remix-prerolls/${encodeURIComponent(prerollId)}/apply`, 'POST'),
  createRemixFeedbackEvent: (projectId: string, input: ApiCreateFeedbackEventInput) =>
    platformRequest<ApiFeedbackEvent>(`/projects/${encodeURIComponent(projectId)}/remix-feedback-events`, 'POST', input),
  listRemixFeedbackEvents: (projectId: string, targetType?: ApiFeedbackTargetType, targetId?: string, limit = 20) => {
    const search = new URLSearchParams({ limit: String(limit) })
    if (targetType) search.set('target_type', targetType)
    if (targetId) search.set('target_id', targetId)
    return platformRequest<{ items: ApiFeedbackEvent[] }>(`/projects/${encodeURIComponent(projectId)}/remix-feedback-events?${search.toString()}`)
  },
  getRemixAssetPerformance: (projectId: string) =>
    platformRequest<{ items: ApiAssetPerformance[] }>(`/projects/${encodeURIComponent(projectId)}/remix-asset-performance`),
  createPlannerWeightSnapshot: (projectId: string) =>
    platformRequest<ApiPlannerWeightSnapshot>(`/projects/${encodeURIComponent(projectId)}/remix-planner-weight-snapshots`, 'POST'),
  listAgentRuns: (projectId: string, limit = 20) =>
    platformRequest<{ items: ApiAgentRun[] }>(`/projects/${encodeURIComponent(projectId)}/agent-runs?limit=${limit}`),
  createAgentRun: (projectId: string, renderJobId: string) =>
    platformRequest<ApiAgentRun>(`/projects/${encodeURIComponent(projectId)}/agent-runs`, 'POST', {
      workflow: 'render_diagnosis',
      target: { render_job_id: renderJobId },
    }),
  getAgentRun: (projectId: string, runId: string) =>
    platformRequest<ApiAgentRun>(`/projects/${encodeURIComponent(projectId)}/agent-runs/${encodeURIComponent(runId)}`),
  cancelAgentRun: (projectId: string, runId: string) =>
    platformRequest<ApiAgentRun>(`/projects/${encodeURIComponent(projectId)}/agent-runs/${encodeURIComponent(runId)}/cancel`, 'POST'),
  importKnowledgeDocument: (projectId: string, input: { title: string; source_uri?: string; source_type?: string; text: string }) =>
    platformRequest<ApiKnowledgeDocument>(`/projects/${encodeURIComponent(projectId)}/knowledge/documents`, 'POST', input),
  listKnowledgeDocuments: (projectId: string, limit = 20) =>
    platformRequest<{ items: ApiKnowledgeDocument[] }>(`/projects/${encodeURIComponent(projectId)}/knowledge/documents?limit=${limit}`),
  searchKnowledge: (projectId: string, query: string, limit = 10) =>
    platformRequest<{ query: string; items: ApiKnowledgeSearchResult[] }>(
      `/projects/${encodeURIComponent(projectId)}/knowledge/search?q=${encodeURIComponent(query)}&limit=${limit}`,
    ),
  listReports: (projectId: string, limit = 100) =>
    request<{ items: ApiInsightReport[] }>(`${insightProjectPath(projectId)}/reports?limit=${limit}`),
  // 投放执行清单。定格报告时要问「这份报告算哪次投放」，答案只能从这里挑——
  // 让人手打一个执行 ID，打错了报告就挂在了另一次投放上，而两边都不会报错。
  listDeliveryExecutions: (projectId: string, limit = 50) =>
    request<{ items: ApiDeliveryExecutionResult[] }>(
      `/delivery/v1/projects/${encodeURIComponent(projectId)}/executions?limit=${limit}`,
    ),
  // 定格一份复盘报告。window 传的是投后分析页当前选的那两个日期——
  // 人看到什么就定格什么，不让后端另挑一个窗口。
  createReport: (projectId: string, body: { execution_id: string; window: { start: string; end: string } }) =>
    request<ApiInsightReport>(`${insightProjectPath(projectId)}/reports`, 'POST', body),
  // 记一笔：把分析页上的一条结论钉进本轮复盘草稿。
  //
  // 请求里**没有** confidence / verdict——判定由后端拿 (window, dimension,
  // source_ref, variable) 回到那次分析结果里找回来。能从这里传的话，页面上标的
  // 三档就是装饰：改一个字段就能把「算不出来」记成「能归因」。
  //
  // 目标草稿按 (项目 + 窗口) 自动 find-or-create，不需要先建复盘。
  pinFinding: (projectId: string, body: {
    window: { start: string; end: string }
    dimension: string
    source_ref?: string
    variable?: string
    text?: string
  }) => request<ApiInsightReport>(`${insightProjectPath(projectId)}/findings`, 'POST', body),
  // 人工删减。报告页上加不了新的一条：写进报告的每条发现都得能回溯到某次对比、
  // 某个实验或某条经验，手打一条就断了这个链子。要加只能回分析页记一笔。
  dropReportFinding: (
    projectId: string,
    reportId: string,
    body: { expected_version: number; index: number; dropped: boolean },
  ) =>
    request<ApiInsightReport>(
      `${insightProjectPath(projectId)}/reports/${encodeURIComponent(reportId)}:drop-finding`, 'POST', body,
    ),
  // 提交这一轮复盘：补上「算哪次投放」、把系统发现定格进去、置为已确认，后端一次做完。
  // 和 confirmReport 的区别是它带 execution_id——草稿是记一笔时自动建的，那会儿
  // 还没到「这算哪次投放」这个问题，提交才是全流程唯一必须回答它的地方。
  submitReview: (projectId: string, reportId: string, body: {
    execution_id: string
    expected_version: number
  }) => request<ApiInsightReport>(
    `${insightProjectPath(projectId)}/reports/${encodeURIComponent(reportId)}/submit`, 'POST', body,
  ),
  confirmReport: (projectId: string, reportId: string, expectedVersion: number) =>
    request<ApiInsightReport>(
      `${insightProjectPath(projectId)}/reports/${encodeURIComponent(reportId)}:confirm`, 'POST',
      { expected_version: expectedVersion },
    ),
  // 从复盘沉淀经验。九字段能填多少填多少：复盘是最有依据的一次，
  // 这里少填一个字段，后面投前洞察里那张卡就永远缺一格。
  createExperienceFromReport: (
    projectId: string,
    reportId: string,
    body: {
      expected_report_version: number
      conclusion: string
      conditions?: string[]
      counterexamples?: string[]
      card_type?: ApiInsightCardType
      confidence?: ApiConfidenceLevel
      recommended_action?: string
      applicability?: ApiApplicability
      data_basis?: ApiDataBasis
      content_basis?: ApiContentBasis
    },
  ) => request<ApiExperience>(
    `${insightProjectPath(projectId)}/reports/${encodeURIComponent(reportId)}:create-experience`, 'POST', body,
  ),
  listExperiences: (projectId: string, status?: ApiExperienceStatus, limit = 100) => {
    const search = new URLSearchParams({ limit: String(limit) })
    if (status) search.set('status', status)
    return request<{ items: ApiExperience[] }>(
      `${insightProjectPath(projectId)}/experiences?${search.toString()}`,
    )
  },
  // 「查」用 POST：条件有七格、好几格是自由文本，塞 query string 里既难读也容易漏转义。
  lookupExperiences: (projectId: string, body: ApiExperienceLookup) =>
    request<{ items: ApiExperienceMatch[] }>(
      `${insightProjectPath(projectId)}/experiences/lookup`, 'POST', body,
    ),
  listExperienceAudits: (projectId: string, experienceId: string, limit = 50) =>
    request<{ items: ApiExperienceAudit[] }>(
      `${insightExperiencePath(projectId, experienceId)}/audits?limit=${limit}`,
    ),
  listExperienceReferences: (projectId: string, experienceId: string, limit = 50) =>
    request<{ items: ApiExperienceReference[] }>(
      `${insightExperiencePath(projectId, experienceId)}/references?limit=${limit}`,
    ),
  listProjectExperienceReferences: (projectId: string, limit = 100) =>
    request<{ items: ApiExperienceReference[] }>(
      `${insightProjectPath(projectId)}/experience-references?limit=${limit}`,
    ),
  listExperienceLineage: (projectId: string, experienceId: string) =>
    request<{ items: ApiExperience[] }>(
      `${insightExperiencePath(projectId, experienceId)}/lineage`,
    ),
  confirmExperience: (projectId: string, experienceId: string, expectedVersion: number) =>
    request<ApiExperience>(
      `${insightExperiencePath(projectId, experienceId)}:confirm`, 'POST',
      { expected_version: expectedVersion },
    ),
  rejectExperience: (projectId: string, experienceId: string, expectedVersion: number, reason: string) =>
    request<ApiExperience>(
      `${insightExperiencePath(projectId, experienceId)}:reject`, 'POST',
      { expected_version: expectedVersion, reason },
    ),
  requestExperienceReview: (projectId: string, experienceId: string, expectedVersion: number, reason: string) =>
    request<ApiExperience>(
      `${insightExperiencePath(projectId, experienceId)}:request-review`, 'POST',
      { expected_version: expectedVersion, reason },
    ),
  retireExperience: (projectId: string, experienceId: string, expectedVersion: number, reason: string) =>
    request<ApiExperience>(
      `${insightExperiencePath(projectId, experienceId)}:retire`, 'POST',
      { expected_version: expectedVersion, reason },
    ),
  // 修订不是编辑：后端会新建一条修订并把旧的那条标成被取代，两条都留着。
  // 所以这里必须把九个字段整份发过去——没发的字段不是「保持不变」，是「新版本里没有」。
  reviseExperience: (projectId: string, experienceId: string, body: ReviseExperienceBody) =>
    request<ApiExperience>(
      `${insightExperiencePath(projectId, experienceId)}:revise`, 'POST', body,
    ),
  // AM-014 的闭环：下游引用了哪条结论、最后是照做还是改了还是没采纳，
  // 记在经验自己身上，而不是只在 Brief 里留一句话。
  recordExperienceReference: (
    projectId: string,
    experienceId: string,
    body: { consumer_kind: string; consumer_id: string; outcome: ApiExperienceReferenceOutcome; note?: string },
  ) => request<ApiExperienceReference>(
    `${insightExperiencePath(projectId, experienceId)}:record-reference`, 'POST', body,
  ),
  // 分析素材库的每个视图都是一次不同的查询，而不是同一批数据换个标签（22 §8.3）。
  listInsightAssets: (projectId: string, filter: ApiInsightAssetFilter = {}) => {
    const search = new URLSearchParams({ limit: String(filter.limit ?? 100) })
    filter.statuses?.forEach(status => search.append('status', status))
    filter.assetTypes?.forEach(assetType => search.append('asset_type', assetType))
    filter.sourceKinds?.forEach(sourceKind => search.append('source_kind', sourceKind))
    if (filter.lineageId) search.set('lineage_id', filter.lineageId)
    return request<{ items: ApiInsightAsset[] }>(
      `${insightProjectPath(projectId)}/assets?${search.toString()}`,
    )
  },
  // 登记一个素材。正常情况下素材由创意模块产出后自动流进来，但外部投放的素材
  // （别处剪的片子、代理商给的图文）没有那条通路，只能人在这里手工登记，
  // 否则平台回流的广告对象认不到任何素材上，它的花费就永远算不到人头上。
  indexInsightAsset: (projectId: string, body: IndexInsightAssetBody) =>
    request<ApiInsightAsset>(`${insightProjectPath(projectId)}/assets`, 'POST', body),
  getInsightAsset: (projectId: string, assetId: string) =>
    request<ApiInsightAsset>(`${insightAssetPath(projectId, assetId)}`),
  listInsightAssetLineage: (projectId: string, assetId: string) =>
    request<{ items: ApiInsightAsset[] }>(`${insightAssetPath(projectId, assetId)}/lineage`),
  listInsightAssetFeatures: (projectId: string, assetId: string) =>
    request<{ items: ApiInsightAssetFeature[] }>(`${insightAssetPath(projectId, assetId)}/features`),
  // 人工结论另起一行写入，不改 AI 那一层，后台再跑也不会盖掉（03 AM-006、§14）。
  patchInsightAssetFeatures: (
    projectId: string,
    assetId: string,
    body: { expected_version: number; features: ApiFeatureInput[]; reason: string },
  ) => request<{ items: ApiInsightAssetFeature[] }>(`${insightAssetPath(projectId, assetId)}/features`, 'PATCH', body),
  // AI 提特征。**只有人点按钮才会调到这里**：登记素材时自动排队会把复核队列
  // 灌满没人要看的结果，而复核是这套东西唯一的质量闸门（03 AM-005）。
  // content 必须由调用方带上——素材库存的是素材的身份和状态，不存正文。
  analyzeInsightAsset: (
    projectId: string,
    assetId: string,
    body: { expected_version: number; content: string; note?: string },
  ) => request<ApiAnalyzeAssetResult>(`${insightAssetPath(projectId, assetId)}:analyze`, 'POST', body),
  // 量客观变量：时长、画幅。和 analyze 是两回事——不调模型，不花钱，读的是素材库
  // 上传这个文件时就探测好的数，同一条素材按几次结果都一样，所以按钮可以随便点。
  // 落成「客观可测」层，直接能进归因，不进复核队列。
  // 只对**从创意导入**的素材有效：手工登记的那些洞察这边只有一条索引，没有文件。
  deriveInsightAssetFeatures: (
    projectId: string,
    assetId: string,
    body: { expected_version: number },
  ) => request<{ items: ApiInsightAssetFeature[] }>(
    `${insightAssetPath(projectId, assetId)}:derive-features`, 'POST', body,
  ),
  // 分析历史。失败的也在里面：只列成功的话，成功率永远是 100%。
  listInsightAssetAnalysisRuns: (projectId: string, assetId: string, limit = 20) =>
    request<{ items: ApiAnalysisRun[] }>(
      `${insightAssetPath(projectId, assetId)}/analysis-runs?limit=${limit}`,
    ),
  identifyInsightAssetType: (
    projectId: string,
    assetId: string,
    body: { expected_version: number; asset_type: ApiInsightAssetType; source: ApiFeatureSource; confidence?: ApiConfidence; reason: string },
  ) => request<ApiInsightAsset>(`${insightAssetPath(projectId, assetId)}:identify-type`, 'POST', body),
  listInsightAssetMappings: (projectId: string, status?: ApiMappingStatus, limit = 100) => {
    const search = new URLSearchParams({ limit: String(limit) })
    if (status) search.set('status', status)
    return request<{ items: ApiInsightAssetMapping[] }>(
      `${insightProjectPath(projectId)}/asset-mappings?${search.toString()}`,
    )
  },
  // 认领：把一个平台对象认到某个素材版本上，或者明确忽略它。认领之后它的花费
  // 才算得到这一版素材头上（doc10 §5）。
  resolveInsightAssetMapping: (
    projectId: string,
    mappingId: string,
    body: { expected_version: number; status: ApiMappingStatus; asset_id?: string; note: string },
  ) => request<ApiInsightAssetMapping>(
    `${insightProjectPath(projectId)}/asset-mappings/${encodeURIComponent(mappingId)}:resolve`, 'POST', body),
  listFeatureSchemas: (projectId: string) =>
    request<{ items: ApiFeatureSchema[] }>(`${insightProjectPath(projectId)}/feature-schemas`),
  getFeatureMatrix: (projectId: string, assetIds: string[]) =>
    request<ApiFeatureMatrix>(
      `${insightProjectPath(projectId)}/feature-matrix?asset_ids=${encodeURIComponent(assetIds.join(','))}`,
    ),
  // 找相似素材。两种问法：给素材 ID 问「和它像的还有哪些」，或者给一组变量取值问
  // 「时长 15 秒的还有哪些」。后一种是 ❓「算不出来」的升级通道。
  findSimilarAssets: (projectId: string, body: {
    asset_id?: string
    features?: Record<string, string>
    limit?: number
  }) => request<ApiSimilarAssetResult>(`${insightProjectPath(projectId)}/assets/similar`, 'POST', body),
  // 外部素材。它们**永远不进共享素材库**：那里的素材可以被拿去投放，而这些没有
  // 那份授权。收它们只有一个用处——解释本轮结果时有个参照。
  importExternalAsset: (projectId: string, body: {
    title: string
    source_note: string
    purpose: 'benchmark' | 'reference'
    purpose_note?: string
    asset_type?: string
    window_end: string
    features?: Record<string, string>
  }) => request<ApiExternalAsset>(`${insightProjectPath(projectId)}/external-assets`, 'POST', body),
  listExternalAssets: (projectId: string, limit = 50) =>
    request<{ items: ApiExternalAsset[] }>(`${insightProjectPath(projectId)}/external-assets?limit=${limit}`),
  // 数据接入（doc10）。五个视图各自是一次不同的查询：数据源与字段映射读同一批行
  // 但看不同字段，导入任务与同步记录是同一张表按 kind 过滤（22 §8.3）。
  listDataSources: (projectId: string, filter: ApiDataSourceFilter = {}) => {
    const search = new URLSearchParams({ limit: String(filter.limit ?? 100) })
    filter.statuses?.forEach(status => search.append('status', status))
    filter.platforms?.forEach(platform => search.append('platform', platform))
    return request<{ items: ApiDataSource[] }>(
      `${insightProjectPath(projectId)}/data-sources?${search.toString()}`,
    )
  },
  getDataSource: (projectId: string, dataSourceId: string) =>
    request<ApiDataSource>(insightDataSourcePath(projectId, dataSourceId)),
  registerDataSource: (
    projectId: string,
    body: {
      platform: ApiPlatform
      ingest_mode: ApiIngestMode
      account_label?: string
      account_ref?: string
      credential_ref?: string
      caliber?: ApiMetricCaliber
      field_mapping?: Record<string, string>
    },
  ) => request<ApiDataSource>(`${insightProjectPath(projectId)}/data-sources`, 'POST', body),
  updateDataSource: (
    projectId: string,
    dataSourceId: string,
    body: {
      expected_version: number
      status?: ApiDataSourceStatus
      account_label?: string
      caliber?: ApiMetricCaliber
      field_mapping?: Record<string, string>
    },
  ) => request<ApiDataSource>(insightDataSourcePath(projectId, dataSourceId), 'PATCH', body),
  setDataSourceQuality: (
    projectId: string,
    dataSourceId: string,
    body: { expected_version: number; quality_status: ApiQualityStatus; note?: string },
  ) => request<ApiDataSource>(`${insightDataSourcePath(projectId, dataSourceId)}:set-quality`, 'POST', body),
  listImportBatches: (projectId: string, filter: ApiImportBatchFilter = {}) => {
    const search = new URLSearchParams({ limit: String(filter.limit ?? 100) })
    if (filter.dataSourceId) search.set('data_source_id', filter.dataSourceId)
    filter.statuses?.forEach(status => search.append('status', status))
    return request<{ items: ApiImportBatch[] }>(
      `${insightProjectPath(projectId)}/import-batches?${search.toString()}`,
    )
  },
  // 部分成功也会正常返回：批次建出来了，被拒的行在 batch.errors 里逐条说明。
  importMetrics: (
    projectId: string,
    body: {
      data_source_id: string
      kind: ApiImportKind
      rows: ApiMetricRow[]
      source_label?: string
      content_hash?: string
      corrects_batch_id?: string
      register_objects?: boolean
    },
  ) => request<ApiImportResult>(`${insightProjectPath(projectId)}/import-batches`, 'POST', body),
  // 窗口写在 URL 里而不是让后端偷偷默认，20 §4.1 要求数据窗口必须能被看到。
  getMetricOverview: (projectId: string, start?: string, end?: string) => {
    const search = new URLSearchParams()
    if (start) search.set('start', start)
    if (end) search.set('end', end)
    const query = search.toString()
    return request<ApiMetricOverview>(
      `${insightProjectPath(projectId)}/metric-overview${query ? `?${query}` : ''}`,
    )
  },
  // 和 getMetricOverview 分成两个请求，但五个视图共用这一次返回：
  // 拆开的话「趋势里看到的」和「疲劳里算的」会来自两次读取，对不上时没人解释得清。
  getPerformanceAnalysis: (projectId: string, start?: string, end?: string) => {
    const search = new URLSearchParams()
    if (start) search.set('start', start)
    if (end) search.set('end', end)
    const query = search.toString()
    return request<ApiPerformanceAnalysis>(
      `${insightProjectPath(projectId)}/performance-analysis${query ? `?${query}` : ''}`,
    )
  },
  // cross_channel 只认字面 true：跨渠道比较默认关闭（03 §10.3②），
  // 缺参数、空串、"false" 都是关闭，不做 truthy 判断。
  getPreLaunch: (projectId: string, filter: ApiPreLaunchFilter = {}) => {
    const search = new URLSearchParams()
    if (filter.channel) search.set('channel', filter.channel)
    if (filter.creative_type) search.set('creative_type', filter.creative_type)
    if (filter.objective) search.set('objective', filter.objective)
    if (filter.q) search.set('q', filter.q)
    if (filter.cross_channel) search.set('cross_channel', 'true')
    const query = search.toString()
    return request<ApiPreLaunchInsight>(
      `${insightProjectPath(projectId)}/prelaunch${query ? `?${query}` : ''}`,
    )
  },
  // 六个二级视图共用这一次请求：它们共用同一套排序和队列判断，
  // 拆成六个请求会让「队列里还有几条」在不同视图里算出不同的数。
  getDataQuality: (projectId: string, start?: string, end?: string) => {
    const search = new URLSearchParams()
    if (start) search.set('start', start)
    if (end) search.set('end', end)
    const query = search.toString()
    return request<ApiDataQualityReport>(
      `${insightProjectPath(projectId)}/data-quality${query ? `?${query}` : ''}`,
    )
  },
  // 五个二级视图共用这一次请求：它们算的是同一批素材、特征、数据源和日指标，
  // 拆开会让治理面上「特征数」和「待办数」在不同视图里对不上。
  getCapabilityOperations: (projectId: string, start?: string, end?: string) => {
    const search = new URLSearchParams()
    if (start) search.set('start', start)
    if (end) search.set('end', end)
    const query = search.toString()
    return request<ApiCapabilityOperations>(
      `${insightProjectPath(projectId)}/capability-operations${query ? `?${query}` : ''}`,
    )
  },
  // 设置页的说明文本全部由后端现算：判定阈值那几条取当前生效的值，其余仍是代码常量本身。
  // 前端抄一份的话，改了 Go 忘了改这里，这一页就从说明变成误导——那比不做更糟。
  getInsightSettings: (projectId: string) =>
    request<ApiInsightSettings>(`${insightProjectPath(projectId)}/settings`),
  getThresholds: (projectId: string) =>
    request<ApiResolvedThresholds>(`${insightProjectPath(projectId)}/thresholds`),
  // 用 PUT 而不是 POST：从调用方看这是「把阈值设成这样」。落库仍是追加一版，
  // 不改任何已有的行——已经判过的结论保持它当初按的那一版。
  //
  // values 里的 null 表示「这一格改回出厂设定」，不是 0；reason 必填。
  saveThresholds: (projectId: string, body: {
    values: Record<string, number | null>
    reason: string
  }) => request<ApiResolvedThresholds>(`${insightProjectPath(projectId)}/thresholds`, 'PUT', body),
  listThresholdHistory: (projectId: string, limit = 20) =>
    request<{ items: ApiThresholdSet[] }>(
      `${insightProjectPath(projectId)}/thresholds/history?limit=${limit}`,
    ),
  getOceanEngineSession: (projectId: string) => request<ApiOceanEngineSession>(`${insightProjectPath(projectId)}/ocean-engine-session`),
  updateOceanEngineSession: (projectId: string, body: { session: string; expected_version?: number }) => request<ApiOceanEngineSession>(`${insightProjectPath(projectId)}/ocean-engine-session`, 'PUT', body),
  verifyOceanEngineSession: (projectId: string, expectedVersion: number) => request<ApiOceanEngineSession>(`${insightProjectPath(projectId)}/ocean-engine-session:verify`, 'POST', { expected_version: expectedVersion }),
  listConnectorAccounts: () => request<{ items: ApiConnectorAccount[] }>('/connector/v1/accounts'),
  registerConnectorAccount: (body: { external_id: string; display_label?: string }) => request<ApiConnectorAccount>('/connector/v1/accounts', 'POST', body),
  getConnectorAccountSession: (accountId: string) => request<ApiConnectorAccountSession>(`/connector/v1/accounts/${encodeURIComponent(accountId)}/session`),
  updateConnectorAccountSession: (accountId: string, body: { session: string; expected_version: number }) => request<ApiConnectorAccountSession>(`/connector/v1/accounts/${encodeURIComponent(accountId)}/session`, 'PUT', body),
  verifyConnectorAccount: (accountId: string) => request<ApiConnectorAccount>(`/connector/v1/accounts/${encodeURIComponent(accountId)}/verify`, 'POST'),
  syncConnectorAccount: (accountId: string, body: { start: string; end: string; time_zone: string; currency: string; sync_mode?: 'full' | 'metrics_only' | 'inventory_only' }, idempotencyKey: string) => request<ApiConnectorSyncResult>(`/connector/v1/accounts/${encodeURIComponent(accountId)}/syncs`, 'POST', body, { 'Idempotency-Key': idempotencyKey }),
  getConnectorSync: (accountId: string, syncId: string) => request<ApiConnectorSyncStatus>(`/connector/v1/accounts/${encodeURIComponent(accountId)}/syncs/${encodeURIComponent(syncId)}`),
  getConnectorLaunchBatchCalibration: (accountId: string) => request<ApiLaunchBatchCalibration>(`/connector/v1/accounts/${encodeURIComponent(accountId)}/launch-batch-calibration`),
  listProjectConnectorAccounts: (projectId: string) => request<{ items: ApiConnectorAccount[] }>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts`),
  registerProjectConnectorAccount: (projectId: string, body: { external_id: string; display_label?: string }) => request<ApiConnectorAccount>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts`, 'POST', body),
  claimProjectConnectorAccount: (projectId: string, accountId: string) => request<ApiConnectorAccount>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts:claim`, 'POST', { account_ref: accountId }),
  getProjectConnectorAccountSession: (projectId: string, accountId: string) => request<ApiConnectorAccountSession>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/session`),
  updateProjectConnectorAccountSession: (projectId: string, accountId: string, body: { session: string; expected_version: number }) => request<ApiConnectorAccountSession>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/session`, 'PUT', body),
  verifyProjectConnectorAccount: (projectId: string, accountId: string) => request<ApiConnectorAccount>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/verify`, 'POST'),
  syncProjectConnectorAccount: (projectId: string, accountId: string, body: { start: string; end: string; time_zone: string; currency: string; sync_mode?: 'full' | 'metrics_only' | 'inventory_only' }, idempotencyKey: string) => request<ApiConnectorSyncResult>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/syncs`, 'POST', body, { 'Idempotency-Key': idempotencyKey }),
  getProjectConnectorSync: (projectId: string, accountId: string, syncId: string) => request<ApiConnectorSyncStatus>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/syncs/${encodeURIComponent(syncId)}`),
  getProjectConnectorSnapshot: (projectId: string, accountId: string, predictionCutoff = new Date().toISOString()) => request<ApiConnectorCanonicalSnapshot>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/canonical-snapshots?prediction_cutoff=${encodeURIComponent(predictionCutoff)}`),
  listProjectConnectorPlatformObjects: (projectId: string, accountId: string, filter: { objectKind?: ApiConnectorPlatformObjectKind; status?: 'active' | 'unavailable'; q?: string; cursor?: string; limit?: number; sortBy?: 'created_at' | 'ctr' | 'conversions'; sortOrder?: 'asc' | 'desc' } = {}) => {
    const search = new URLSearchParams({ limit: String(filter.limit ?? 100) })
    if (filter.objectKind) search.set('object_kind', filter.objectKind)
    if (filter.status) search.set('status', filter.status)
    if (filter.q) search.set('q', filter.q)
    if (filter.cursor) search.set('cursor', filter.cursor)
    if (filter.sortBy) search.set('sort_by', filter.sortBy)
    if (filter.sortOrder) search.set('sort_order', filter.sortOrder)
    return request<{ items: ApiConnectorPlatformObject[]; next_cursor: string }>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/platform-objects?${search.toString()}`)
  },
  readProjectOptimizationTargetCapabilities: (projectId: string, accountId: string, context: ApiOptimizationTargetContext) => request<ApiOptimizationTargetCapabilitySnapshot>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/optimization-target-capabilities`, 'POST', { context }),
  readProjectOceanEngineAccountCapabilities: (projectId: string, accountId: string) => request<ApiOceanEngineAccountCapabilitySnapshot>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/capabilities`),
  getProjectConnectorLaunchBatchCalibration: (projectId: string, accountId: string) => request<ApiLaunchBatchCalibration>(`/connector/v1/projects/${encodeURIComponent(projectId)}/accounts/${encodeURIComponent(accountId)}/launch-batch-calibration`),
  getMiyunConnection: (projectId: string) => request<ApiMiyunConnection>(`${miyunProjectPath(projectId)}/connection`),
  updateMiyunConnection: (projectId: string, body: { session: string; session_expires_at?: string; expected_version?: number }) => request<ApiMiyunConnection>(`${miyunProjectPath(projectId)}/connection`, 'PUT', body),
  verifyMiyunConnection: (projectId: string, expectedVersion: number) => request<ApiMiyunConnection>(`${miyunProjectPath(projectId)}/connection:verify`, 'POST', { expected_version: expectedVersion }),
  listMiyunProductProfiles: (projectId: string, limit = 50) => request<{ items: ApiMiyunProductProfile[] }>(`${miyunProjectPath(projectId)}/product-profiles?limit=${limit}`),
  getMiyunProductSource: (projectId: string) => request<ApiMiyunProductSource>(`${miyunProjectPath(projectId)}/product-source`),
  getMediaUnderstandingCapabilities: () => request<ApiMediaUnderstandingCapabilities>('/media/v1/capabilities'),
  requestMediaUnderstanding: (projectId: string, asset: ApiMiyunAssetVersionRef) => request<ApiMediaUnderstandingArtifact>(`/media/v1/projects/${encodeURIComponent(projectId)}/understandings`, 'POST', { asset_id: asset.asset_id, version: asset.version }),
  getMediaUnderstanding: (projectId: string, artifactId: string) => request<ApiMediaUnderstandingArtifact>(`/media/v1/projects/${encodeURIComponent(projectId)}/understandings/${encodeURIComponent(artifactId)}`),
  analyzeMiyunProductProfile: (projectId: string, body: { connection_id: string; product_id?: string; product_name?: string; category_name?: string; product_asset_refs: ApiMiyunAssetVersionRef[]; knowledge_document_ids: string[] }) => request<ApiMiyunProductProfile>(`${miyunProjectPath(projectId)}/product-profiles:analyze`, 'POST', body),
  getMiyunProductProfile: (projectId: string, profileId: string) => request<ApiMiyunProductProfile>(`${miyunProjectPath(projectId)}/product-profiles/${encodeURIComponent(profileId)}`),
  confirmMiyunProductProfile: (projectId: string, profileId: string, expectedVersion: number, query: ApiMiyunProfileQuery) => request<ApiMiyunProductProfile>(`${miyunProjectPath(projectId)}/product-profiles/${encodeURIComponent(profileId)}:confirm`, 'POST', { expected_version: expectedVersion, query }),
  listMiyunCrawlJobs: (projectId: string, limit = 50) => request<{ items: ApiMiyunCrawlJob[] }>(`${miyunProjectPath(projectId)}/crawl-jobs?limit=${limit}`),
  createMiyunCrawlJob: (projectId: string, body: { product_profile_id: string; operation: ApiMiyunCrawlJob['operation']; max_pages: number }, idempotencyKey: string) => request<ApiMiyunCrawlJob>(`${miyunProjectPath(projectId)}/crawl-jobs`, 'POST', body, { 'Idempotency-Key': idempotencyKey }),
  getMiyunCrawlJob: (projectId: string, jobId: string) => request<ApiMiyunCrawlJob>(`${miyunProjectPath(projectId)}/crawl-jobs/${encodeURIComponent(jobId)}`),
  cancelMiyunCrawlJob: (projectId: string, jobId: string, expectedVersion: number) => request<ApiMiyunCrawlJob>(`${miyunProjectPath(projectId)}/crawl-jobs/${encodeURIComponent(jobId)}:cancel`, 'POST', { expected_version: expectedVersion }),
  retryMiyunCrawlJob: (projectId: string, jobId: string, idempotencyKey: string) => request<ApiMiyunCrawlJob>(`${miyunProjectPath(projectId)}/crawl-jobs/${encodeURIComponent(jobId)}:retry`, 'POST', undefined, { 'Idempotency-Key': idempotencyKey }),
  listMiyunMaterials: (projectId: string, options: { crawlJobId?: string; limit?: number; offset?: number; q?: string; sort?: string; handoffEligible?: boolean } = {}) => {
    const search = new URLSearchParams({ limit: String(options.limit ?? 100) })
    if (options.crawlJobId) search.set('crawl_job_id', options.crawlJobId)
    if (options.offset) search.set('offset', String(options.offset))
    if (options.q) search.set('q', options.q)
    if (options.sort) search.set('sort', options.sort)
    if (options.handoffEligible) search.set('handoff_eligible', 'true')
    return request<{ items: ApiMiyunMaterial[]; total: number; limit: number; offset: number }>(`${miyunProjectPath(projectId)}/materials?${search.toString()}`)
  },
  getMiyunMaterial: (projectId: string, materialId: string) => request<ApiMiyunMaterialDetail>(`${miyunProjectPath(projectId)}/materials/${encodeURIComponent(materialId)}`),
  // This is deliberately a relative, same-origin URL. Never expose source_ref/resource URLs to the browser.
  getMiyunMaterialPreviewUrl: (projectId: string, materialId: string) => `/api${miyunProjectPath(projectId)}/materials/${encodeURIComponent(materialId)}/preview`,
  confirmMiyunMaterial: (projectId: string, materialId: string, expectedVersion: number, note?: string) => request<ApiMiyunMaterial>(`${miyunProjectPath(projectId)}/materials/${encodeURIComponent(materialId)}:confirm`, 'POST', { expected_version: expectedVersion, ...(note === undefined ? {} : { note }) }),
  rejectMiyunMaterial: (projectId: string, materialId: string, expectedVersion: number, note?: string) => request<ApiMiyunMaterial>(`${miyunProjectPath(projectId)}/materials/${encodeURIComponent(materialId)}:reject`, 'POST', { expected_version: expectedVersion, ...(note === undefined ? {} : { note }) }),
  retryMiyunMaterialImport: (projectId: string, materialId: string, expectedVersion: number) => request<ApiMiyunMaterial>(`${miyunProjectPath(projectId)}/materials/${encodeURIComponent(materialId)}:retry-import`, 'POST', { expected_version: expectedVersion }),
  createMiyunHandoff: (projectId: string, body: { source_material_ids: string[]; product_profile_id: string; crawl_job_id: string }, idempotencyKey: string) => request<ApiMiyunHandoff>(`${miyunProjectPath(projectId)}/handoffs`, 'POST', body, { 'Idempotency-Key': idempotencyKey }),
  listMiyunHandoffs: (projectId: string, limit = 50) => request<{ items: ApiMiyunHandoff[] }>(`${miyunProjectPath(projectId)}/handoffs?limit=${limit}`),
  getMiyunHandoff: (projectId: string, handoffId: string) => request<ApiMiyunHandoff>(`${miyunProjectPath(projectId)}/handoffs/${encodeURIComponent(handoffId)}`),
  getMiyunHandoffExportUrl: (projectId: string, handoffId: string, packageKind: 'sources' | 'project') => `/api${miyunProjectPath(projectId)}/handoffs/${encodeURIComponent(handoffId)}/export?package=${packageKind}`,
  markMiyunHandoffDelivered: (projectId: string, handoffId: string, expectedVersion: number) => request<ApiMiyunHandoff>(`${miyunProjectPath(projectId)}/handoffs/${encodeURIComponent(handoffId)}:mark-delivered`, 'POST', { expected_version: expectedVersion }),
  // observed_through 要回传界面上那条问题的 last_observed_at，不要用当前时间：
  // 「你处置的是你看到的那个版本」靠它成立，中间问题若又恶化不会被一并盖掉。
  resolveQualityIssue: (
    projectId: string,
    body: {
      fingerprint: string
      issue_kind: ApiDataQualityIssueKind
      state: ApiDataQualityDispositionState
      note: string
      observed_through: string
    },
  ) => request<ApiDataQualityDisposition>(
    `${insightProjectPath(projectId)}/data-quality/dispositions`, 'POST', body,
  ),

  listInsightExperiments: (projectId: string, status?: ApiExperimentStatus, limit = 50) => {
    const search = new URLSearchParams({ limit: String(limit) })
    if (status) search.set('status', status)
    return request<{ items: ApiExperiment[] }>(
      `${insightProjectPath(projectId)}/insight-experiments?${search.toString()}`,
    )
  },
  createInsightExperiment: (projectId: string, body: ApiCreateExperimentInput) =>
    request<ApiExperiment>(`${insightProjectPath(projectId)}/insight-experiments`, 'POST', body),
  // 样本量没有单独的端点：它是详情的一部分，每次现算跟着详情一起回。两个端点会让
  // 「样本检查」和「实验结论」拿到对不上的数字，而这一页唯一要回答的就是够不够。
  getInsightExperiment: (projectId: string, experimentId: string) =>
    request<ApiExperimentDetail>(insightExperimentPath(projectId, experimentId)),
  attachInsightExperimentAsset: (projectId: string, experimentId: string, variantId: string, assetId: string) =>
    request<ApiAttachExperimentAssetResult>(
      `${insightExperimentPath(projectId, experimentId)}/variants/${encodeURIComponent(variantId)}/assets`,
      'POST', { asset_id: assetId },
    ),
  detachInsightExperimentAsset: (projectId: string, experimentId: string, variantId: string, assetId: string) =>
    request<ApiExperimentVariant>(
      `${insightExperimentPath(projectId, experimentId)}/variants/${encodeURIComponent(variantId)}/assets/${encodeURIComponent(assetId)}`,
      'DELETE',
    ),
  startInsightExperiment: (projectId: string, experimentId: string, expectedVersion: number) =>
    request<ApiExperiment>(
      `${insightExperimentPath(projectId, experimentId)}:start`, 'POST', { expected_version: expectedVersion },
    ),
  // 入参里没有 verdict：判定要是能传，事先定的门槛就形同虚设。人只写解读。
  concludeInsightExperiment: (projectId: string, experimentId: string, expectedVersion: number, interpretation: string) =>
    request<ApiExperiment>(
      `${insightExperimentPath(projectId, experimentId)}:conclude`, 'POST',
      { expected_version: expectedVersion, interpretation },
    ),
}

// Insights 走 /api/insights/v1；request() 已经带上 /api 前缀。
function insightProjectPath(projectId: string): string {
  return `/insights/v1/projects/${encodeURIComponent(projectId)}`
}

function miyunProjectPath(projectId: string): string {
  return `${insightProjectPath(projectId)}/miyun`
}

// 动作端点形如 .../experiences/{id}:confirm，冒号是路径的一部分，不参与编码。
function insightExperiencePath(projectId: string, experienceId: string): string {
  return `${insightProjectPath(projectId)}/experiences/${encodeURIComponent(experienceId)}`
}

function insightExperimentPath(projectId: string, experimentId: string): string {
  return `${insightProjectPath(projectId)}/insight-experiments/${encodeURIComponent(experimentId)}`
}

function insightAssetPath(projectId: string, assetId: string): string {
  return `${insightProjectPath(projectId)}/assets/${encodeURIComponent(assetId)}`
}

function insightDataSourcePath(projectId: string, dataSourceId: string): string {
  return `${insightProjectPath(projectId)}/data-sources/${encodeURIComponent(dataSourceId)}`
}
