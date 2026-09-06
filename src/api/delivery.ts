import { platformClient } from '../data/platformClient'

export class DeliveryApiError extends Error {
  readonly violations: Array<{ field: string; reason: string }>
  constructor(readonly code: string | undefined, readonly status: number, message: string, violations: Array<{ field: string; reason: string }> = []) {
    super(message)
    this.name = 'DeliveryApiError'
    this.violations = violations
  }
}

export type PreflightCheck = {
  code: 'confirmed_brief' | 'ready_creative' | 'budget_boundary'
  passed: boolean
  message: string
  repair: string
}

export type DeliveryChangeSet = {
  id: string
  projectId: string
  name: string
  status: 'draft' | 'preflight_passed' | 'preflight_failed' | 'approved' | 'rejected' | 'executing' | 'executed' | 'rolled_back'
  artifactIds: string[]
  budgetLimit?: number
  preflight?: { passed: boolean; checks: PreflightCheck[]; checkedAt: string }
  execution?: { simulated: true; evidence: Array<{ step: string; status: string; message: string; recordedAt: string }>; executedAt: string }
  rollback?: { simulated: true; reason: string; rolledBackAt: string }
  version: number
  createdAt: string
  updatedAt: string
}

export const deliveryApi = {
  listChangeSets: (projectId?: string) => projectId ? platformClient.listChangeSets(projectId) : Promise.resolve([]),
  createChangeSet: (input: { projectId: string; name: string; artifactIds: string[]; budgetLimit: number }) =>
    platformClient.createChangeSet(input.projectId, input),
  preflight: (projectId: string, id: string) => platformClient.preflightChangeSet(projectId, id),
  approve: (projectId: string, id: string) => platformClient.approveChangeSet(projectId, id),
  execute: (projectId: string, id: string) => platformClient.executeChangeSet(projectId, id),
  rollback: (projectId: string, id: string, reason: string) => platformClient.rollbackChangeSet(projectId, id, reason),
}

export type DeliverySource = 'mock'
export type DeliveryScenario = 'golden_path' | 'budget_zero' | 'creative_unconfirmed' | 'tracking_missing' | 'incomplete_draft' | 'project_plan_list' | 'approval_queue' | 'missing_required_field' | 'orphan_dependency' | 'missing_confirmation' | 'platform_fields_pending' | 'platform_configuration' | 'capability_pending' | 'preflight_failure' | 'approval_expired' | 'plan_stale' | 'partial_execution' | 'result_unknown' | 'review_rejected_alert'

export type DeliveryPlatform = 'ocean_engine' | 'magnetic_engine'
export const oceanEngineMarketingPurposes = ['ecommerce', 'lead_generation', 'application', 'product_catalog', 'content_marketing'] as const
export type OceanEngineMarketingPurpose = typeof oceanEngineMarketingPurposes[number]
export type StableReference = {
  namespace: string
  object_kind: string
  scope: string
  id?: string
  version?: string
  content_hash?: string
  semantic_key?: string
  audit_attributes?: Record<string, string>
  state: 'resolved' | 'unresolved' | 'blocked' | 'redacted'
  reason?: string
  display_name_snapshot?: string
  evidence_version?: string
}

function stableReferenceKey(reference: StableReference): string {
  return [reference.namespace, reference.object_kind, reference.id ?? '', reference.version ?? '', reference.content_hash ?? '', reference.semantic_key ?? ''].join('\u0000')
}

function mergeStableReferences(current: StableReference[] | undefined, selected: StableReference[]): StableReference[] {
  const references = new Map<string, StableReference>()
  for (const reference of current ?? []) references.set(stableReferenceKey(reference), structuredClone(reference))
  for (const reference of selected) references.set(stableReferenceKey(reference), structuredClone(reference))
  return [...references.values()]
}

export type OceanEngineCalibrationManifestBinding = {
  schema_version: 'oceanengine-calibration-manifest/v1'
  manifest_id: string
}

export type DeliveryIntent = {
  schema_version: 'delivery-intent/v1'
  intent_id: string
  version_number: number
  hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)'
  canonical_hash?: string
  payload: {
    payload_schema_version: 'delivery-intent/v1'
    marketing_objective: string
    budget_boundary: { currency: 'CNY'; minimum_total_minor: number; maximum_total_minor: number; minimum_daily_minor?: number; maximum_daily_minor?: number }
    schedule_boundary: { earliest_start: string; latest_end: string; timezone: string }
    optimization_preferences: Array<{ metric: string; direction: 'minimize' | 'maximize'; target_value?: number; unit?: string }>
    product_references?: StableReference[]
    landing_page_references?: StableReference[]
    material_references: StableReference[]
    audience_constraints: { include_references?: StableReference[]; exclude_references?: StableReference[]; constraints?: string[] }
    strategy_reference: StableReference
    calibration_manifest: OceanEngineCalibrationManifestBinding
  }
  configuration_provenance: { kind: 'manual' | 'rule' | 'decision_engine' | 'import'; generator_ref?: string; policy_version?: string }
  fact_provenance: { source: 'mock' | 'replay' | 'connector' | 'page_evidence'; snapshot_ref?: string; evidence_refs?: string[]; observed_at?: string }
  audit?: { created_by?: string; created_at?: string }
}

export type PlatformConfiguration = {
  schema_version: 'delivery-platform-configuration/v2'
  configuration_id: string
  version_number: number
  platform: DeliveryPlatform
  profile_version: 'oceanengine-configuration/v1' | 'magnetic-engine-configuration/v1'
  intent?: { schema_version: 'delivery-intent/v1'; intent_id: string; version_number: number; canonical_hash?: string }
  hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)'
  canonical_hash?: string
  payload: {
    profile: DeliveryPlatform
    ocean_engine?: {
      profile: 'ocean_engine'
      calibration_manifest: OceanEngineCalibrationManifestBinding
      project: {
        draft_schema_version: 'oceanengine-configuration/v1'
        project_draft_id: string
        account_reference: StableReference
        marketing_purpose: string
        marketing_scenario: string
        marketing_product_reference?: StableReference
        application_reference?: StableReference
        application_scenario?: string
        operating_system?: string
        application_download_mode?: string
        lead_capture_mode?: string
        carrier: string
        optimization_target_reference?: StableReference
        deep_optimization_mode?: string
        aigc_dynamic_creative?: boolean
        delivery_mode: string
        targeting: { regions?: string[]; age_ranges?: string[]; gender?: string; smart_expansion: boolean }
        schedule: { mode?: 'long_term' | 'fixed_range'; start_at: string; end_at: string; timezone: string }
        budget_and_bidding: { budget_mode?: 'daily' | 'unlimited'; currency: 'CNY'; daily_budget_minor: number; bidding_strategy: string; charging_mode: string; bid_minor?: number }
        monitoring_references?: StableReference[]
        search_boost?: { keywords?: string[]; bid_coefficient?: number; targeting_expansion?: boolean }
        product_catalog_reference?: StableReference
        placement_strategy?: string
        placement_media?: string[]
        product_targeting?: { rta_redirect?: boolean; region_match?: boolean; delivery_conditions?: string[] }
        application_launch_mode?: string
        project_name: string
      }
      promotions: Array<{
        draft_schema_version: 'oceanengine-configuration/v1'
        promotion_draft_id: string
        delivery_identity: { mode: string; authorized_identity?: StableReference }
        base_material_references: StableReference[]
        copy_items: Array<{ text: string }>
        product_name?: string
        product_image_references?: StableReference[]
        product_selling_points?: string[]
        native_anchor_reference?: StableReference
        landing_page_reference?: StableReference
        direct_link_reference?: StableReference
        product_reference?: StableReference
        creative_component_references?: StableReference[]
        budget_and_bidding?: { currency: 'CNY'; daily_budget_minor: number; bidding_strategy: string; charging_mode: string; bid_minor?: number }
        settings: { call_to_action?: string[]; source_label?: string; comments_enabled?: boolean; smart_generation_enabled?: boolean; client_download_enabled?: boolean; direct_link_mode?: 'automatic' | 'manual'; category_reference?: StableReference; brand_reference?: StableReference }
        promotion_name: string
      }>
    }
    magnetic_engine?: { profile: 'magnetic_engine'; status: 'capability_pending'; reason_code: 'CAPABILITY_PENDING'; reason: string }
  }
  configuration_provenance: { kind: 'manual' | 'rule' | 'decision_engine' | 'import'; generator_ref?: string; policy_version?: string }
  fact_provenance: { source: 'mock' | 'replay' | 'connector' | 'page_evidence'; snapshot_ref?: string; evidence_refs?: string[]; observed_at?: string }
  audit?: { created_by?: string; created_at?: string }
  compilation_metadata: { field_evidence?: Array<{ field: string; state: 'observed' | 'sample_only' | 'operator_reviewed' | 'platform_pending' | 'blocked_by_event_asset' | 'write_validation_pending'; reason?: string }>; steps?: string[]; evidence_refs?: string[] }
}

export type DeliveryPlanDraft = {
  name: string
  objective: string
  /** A confirmed platform enum. It is separate from the free-text business objective. */
  marketingPurpose: OceanEngineMarketingPurpose | ''
  marketingProduct: {
    id: string
    oceanEngineProductId?: string
    name: string
    activityType: string
    activityName: string
    brandName: string
  }
  advertiser: {
    id: string
    name: string
    platform: 'ocean_engine'
  }
  budget: {
    totalMinor: number
    currency: 'CNY'
  }
  schedule: {
    mode: 'long_term' | 'fixed_range'
    startAt: string
    endAt: string
    timezone: string
  }
  tracking: {
    deliveryCarrier: '' | 'orange_landing_page' | 'owned_landing_page'
    landingPage: string
    pixelId: string
    conversionEvent: string
    optimizationTargetId: string
    optimizationTargetName: string
    optimizationTargetSemanticKey: string
    eventAssetName: string
    eventAssetType: string
    searchKeywords: string
    searchBidCoefficient: number
    searchTargetingExpansion: boolean
    monitoringImpression: string
    monitoringValidTouch: string
    monitoringVideoPlay: string
    monitoringVideoComplete: string
    monitoringValidVideoPlay: string
  }
  creativeReferences: Array<{
    assetId: string
    version: number
    contentHash?: string
    route?: string
    confirmed: boolean
    oceanEngineMaterialId?: string
  }>
  strategyReference: {
    taskId: string
    version: number
    contentHash?: string
    route?: string
  }
  sourceStrategyVersion: string
}

export type DeliveryPlanVersion = DeliveryPlanDraft & {

  schemaVersion?: 'delivery-plan-version/v2'
  runtimeStatus?: 'active' | 'capability_pending' | 'legacy_unsupported'
  readOnly?: boolean
  planId: string
  organizationId: string
  projectId: string
  versionNumber: number
  canonicalHash: string
  platform: 'ocean_engine_mock' | DeliveryPlatform
  advertiser: DeliveryPlanDraft['advertiser'] & {
    source: DeliverySource
    scenario: DeliveryScenario
  }
  source: DeliverySource
  scenario: DeliveryScenario
  createdBy: { kind: 'user' | 'service'; id: string }
  createdAt: string
  /** Frozen historical payload exists; its internal tree is intentionally not exposed. */
  legacyConfiguration?: true
  deliveryIntent?: DeliveryIntent
  platformConfiguration?: PlatformConfiguration
}

export type DeliveryRecommendation = {
  id: string
  planId: string
  planVersion: number
  status: 'pending' | 'accepted' | 'rejected' | string
  version: number
  evidenceRefs: string[]
  action: string
  impact: string
  risks: string[]
  observation: string
  cooldown?: string
  source: DeliverySource
  scenario: string
  baseConfiguration?: PlatformConfiguration
  targetConfiguration?: PlatformConfiguration
  baseSnapshotHash?: string
  targetSnapshotHash?: string
  runtimeStatus?: 'active' | 'capability_pending' | 'legacy_unsupported'
  readOnly?: boolean
  createdAt?: string
  updatedAt?: string
}

export type DeliveryDecisionCandidate = {
  id: string
  kind: 'conservative' | 'balanced' | 'exploratory'
  targetConfiguration: PlatformConfiguration
  budgetChangePercent: number
  rationale: string[]
  constraints: Array<{ code: string; passed: boolean; explanation: string }>
  risks: string[]
  uncertainty: 'low' | 'medium' | 'high'
  calibrationManifest: OceanEngineCalibrationManifestBinding
  optimizationFocus?: string
  proposedAction?: string
  actionMagnitudePercent?: number
  scenario?: string
  scenarioProbability?: number
}

export type DeliveryDecision = {
  schemaVersion: 'delivery-decision/v1'
  id: string
  organizationId: string
  projectId: string
  policyVersion: 'delivery-decision-policy/v1' | 'delivery-decision-policy/v2'
  diagnostic: { code: 'ready' | 'insufficient_data' | 'stale_data' | 'blocked_by_asset' | 'platform_pending'; explanation: string; nextAction: string }
  inputs: { planId: string; planVersion: number; planCanonicalHash: string; intentCanonicalHash: string; configurationCanonicalHash: string; factSnapshotRef: string; simulationRunId?: string; simulationInputHash?: string }
  candidates: DeliveryDecisionCandidate[]
  recommendedCandidateId: string
  evidence: string[]
  canonicalHash: string
  createdBy: string
  createdAt: string
}

export type CompiledDeliveryWorkflow = {
  schemaVersion: 'compiled-delivery-workflow/v1'
  id: string
  decisionId: string
  decisionCanonicalHash: string
  selectedCandidateId: string
  configurationCanonicalHash: string
  configurationId: string
  configurationVersion: number
  platform: 'ocean_engine'
  profileVersion: 'oceanengine-configuration/v1'
  accountReference: StableReference
  capabilityContractVersion: 'oceanengine-capability/v0.1'
  selectorContractVersion: 'oceanengine-selector-contract/v0.1'
  actionContractVersion: 'oceanengine-action-contract/v0.1'
  executionDriver: 'playwright-rpa/edge/v1'
  calibrationManifest: OceanEngineCalibrationManifestBinding
  compilerVersion: 'oceanengine-workflow-compiler/v1'
  status: 'ready_for_final_approval'
  remoteWriteEnabled: false
  steps: Array<{ id: string; sequence: number; page: string; action: string; risk: 'observe' | 'prepare_local_form' | 'remote_write'; preconditions: string[]; fields: Array<{ key: string; value: unknown; expectedReadback: unknown; evidenceRef: string }>; timeoutSeconds: number; recovery: string; blocked: boolean; blockReason?: 'PHASE_C_REMOTE_WRITE_PROHIBITED' }>
  canonicalHash: string
  createdAt: string
}

export type DeliveryDecisionSelection = {
  id: string
  decisionId: string
  decisionCanonicalHash: string
  candidateId: string
  configuration: PlatformConfiguration
  workflow: CompiledDeliveryWorkflow
  finalApprovalBinding: { status: 'ready_for_final_approval'; action: 'remote_write'; planCanonicalHash: string; intentCanonicalHash: string; decisionCanonicalHash: string; configurationCanonicalHash: string; workflowCanonicalHash: string }
  createdAt: string
}

export type DeliveryObservatoryRun = {
  schemaVersion: 'delivery-observatory-run/v1'
  id: string
  source: 'mock' | 'replay'
  mode: 'observe_existing' | 'prepare_new_local_form'
  inputHash: string
  binding: { selectionId: string; decisionId: string; decisionCanonicalHash: string; configurationCanonicalHash: string; workflowId: string; workflowCanonicalHash: string }
  dataState: 'ready' | 'insufficient_data' | 'stale_data' | 'blocked_by_asset' | 'platform_pending'
  dataStateReason: string
  status: 'completed' | 'blocked' | 'runner_failed'
  outcome: 'in_sync' | 'drift_detected' | 'local_form_prepared' | 'insufficient_data' | 'stale_data' | 'blocked_by_asset' | 'platform_pending' | 'runner_failure'
  remoteWriteEnabled: false
  steps: Array<{ stepId: string; sequence: number; page: string; workflowAction: string; executedAction: 'observe' | 'prepare_local_form'; status: string; selectorMatches: string[]; evidenceRefs: string[]; pageRefs: string[]; diffs: Array<{ key: string; evidenceRef: string; expectedValue: unknown; observedValue: unknown; matches: boolean }>; blockReason?: string }>
  evidenceRefs: string[]
  pageRefs: string[]
  canonicalHash: string
  createdAt: string
}

export type DeliveryObservatoryFeedback = {
  schemaVersion: 'delivery-observatory-feedback/v1'
  id: string
  runId: string
  runCanonicalHash: string
  runOutcome: DeliveryObservatoryRun['outcome']
  disposition: 'accepted' | 'modified' | 'rejected'
  reason: string
  diffKeys: string[]
  finalConfiguration?: PlatformConfiguration
  finalConfigurationCanonicalHash?: string
  canonicalHash: string
  createdBy: string
  createdAt: string
}

export type DeliveryPlan = {
  id: string
  organizationId: string
  projectId: string
  status: 'draft'
  platform: 'ocean_engine_mock' | DeliveryPlatform
  source: DeliverySource
  scenario: DeliveryScenario
  tourRunId?: string
  tourOwnerId?: string
  tourCase?: DeliveryTourCaseKey
  currentVersionNumber: number
  currentVersion: DeliveryPlanVersion
  versions: DeliveryPlanVersion[]
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type DeliveryApproval = {
  approvalId: string
  valid: boolean
  invalidReason?: 'APPROVAL_EXPIRED' | 'APPROVAL_CONTENT_MISMATCH' | 'APPROVAL_SCOPE_EXCEEDED' | 'STALE_PLAN_VERSION'
  planId: string
  planVersion: number
  changeSetId: string
  changeSetVersion: number
  planCanonicalHash: string
  targetSnapshotHash?: string
  configuration?: { schemaVersion: string; id: string; version: number; platform: DeliveryPlatform; profileVersion: string; canonicalHash: string }
  intent?: { schemaVersion: string; id: string; version: number; canonicalHash: string }
  actionHash: string
  hashSummary: string
  action: 'execute'
  scope: 'execute_mock'
  budgetLimit: { totalMinor: number; currency: 'CNY' }
  approvedBy: string
  approvedAt: string
  expiresAt: string
  source: DeliverySource
  scenario: DeliveryScenario
}

export type DeliveryControlChangeSet = {
  id: string
  organizationId: string
  projectId: string
  planId: string
  planName: string
  planVersion: number
  planCanonicalHash: string
  targetSnapshot?: PlatformConfiguration
  legacySnapshot?: true
  targetSnapshotHash?: string
  runtimeStatus?: 'active' | 'capability_pending' | 'legacy_unsupported'
  readOnly?: boolean
  recommendationId?: string
  budgetLimit: { totalMinor: number; currency: 'CNY' }
  status: 'draft' | 'preflight_passed' | 'preflight_failed' | 'approved' | 'rejected' | 'executed' | 'rolled_back'
  riskLevel: string
  preflightNotes: string[]
  approvedBy?: string
  approvedAt?: string
  rejectedBy?: string
  rejectedAt?: string
  rejectionReason?: string
  approval?: DeliveryApproval
  source: DeliverySource
  scenario: DeliveryScenario
  version: number
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type DeliveryExecutionScenario = 'success' | 'failed' | 'partial' | 'result_unknown'

export type DeliveryExecutionStatus =
  | 'queued'
  | 'validating_approval'
  | 'executing'
  | 'verifying'
  | 'succeeded'
  | 'failed'
  | 'partial'
  | 'result_unknown'
  | 'cancelled'

export type DeliveryExecutionStepStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'result_unknown' | 'skipped'

export type DeliveryExecutionStep = {
  id: string
  sequence: number
  action: string
  status: DeliveryExecutionStepStatus
  attempt: number
  effect: 'confirmed_applied' | 'confirmed_not_applied' | 'unknown' | 'none'
  outcomeSummary: string
  evidenceRef?: string
  startedAt?: string
  completedAt?: string
  version: number
}

export type DeliveryExecution = {
  id: string
  organizationId: string
  projectId: string
  changeSetId: string
  approvalId?: string
  status: DeliveryExecutionStatus
  version: number
  mode: 'local_simulation'
  adapter: 'mock_ocean_engine'
  source: DeliverySource
  scenario: DeliveryExecutionScenario
  idempotencyKey: string
  requestHash: string
  executedBy: string
  startedAt: string
  completedAt?: string
  retryAllowed: boolean
  recoveryAction: 'none' | 'create_new_change_set' | 'review_and_compensate' | 'query_and_reconcile'
  recoveryReason: string
  compensationCandidates: string[]
  steps: DeliveryExecutionStep[]
}

export type DeliveryExecutionEvidence = {
  id: string
  executionId: string
  summary: string
  source: DeliverySource
  scenario: DeliveryExecutionScenario
  references: string[]
}

export type DeliveryExecutionRecord = {
  changeSet: DeliveryControlChangeSet
  execution: DeliveryExecution
  evidence: DeliveryExecutionEvidence
}

type WireDeliveryPlanDraft = {
  name: string
  objective: string
  marketing_purpose?: OceanEngineMarketingPurpose
  advertiser: { id: string; name: string; platform: 'ocean_engine'; source?: DeliverySource; scenario?: DeliveryScenario }
  budget: { total_minor: number; currency: 'CNY' }
  schedule: { mode?: 'long_term' | 'fixed_range'; start_at: string; end_at: string; timezone: string }
  marketing_product?: { id?: string; ocean_engine_product_id?: string; name?: string; activity_type?: string; activity_name?: string; brand_name?: string }
  tracking: {
    delivery_carrier?: '' | 'orange_landing_page' | 'owned_landing_page'; landing_page: string; pixel_id: string; conversion_event: string
    optimization_target_id?: string; optimization_target_name?: string; event_asset_name?: string; event_asset_type?: string
    search_keywords?: string; search_bid_coefficient?: number; search_targeting_expansion?: boolean
    monitoring_impression?: string; monitoring_valid_touch?: string; monitoring_video_play?: string; monitoring_video_complete?: string; monitoring_valid_video_play?: string
  }
  creative_references: Array<{ asset_id: string; version: number; content_hash?: string; route?: string; confirmed: boolean; ocean_engine_material_id?: string }>
  strategy_reference?: { task_id: string; version: number; content_hash?: string; route?: string }
  source_strategy_version: string
}

type WireDeliveryPlanVersion = Partial<WireDeliveryPlanDraft> & {
  schema_version?: 'delivery-plan-version/v2'
  runtime_status?: 'active' | 'capability_pending' | 'legacy_unsupported'
  read_only?: boolean
  plan_id: string
  organization_id: string
  project_id: string
  version_number: number
  canonical_hash: string
  platform: 'ocean_engine_mock' | DeliveryPlatform
  advertiser?: WireDeliveryPlanDraft['advertiser'] & { source: DeliverySource; scenario: DeliveryScenario }
  source: DeliverySource
  scenario: DeliveryScenario
  created_by: { kind: 'user' | 'service'; id: string }
  created_at: string
  three_tier_configuration?: unknown
  intent?: DeliveryIntent | null
  platform_configuration?: PlatformConfiguration | null
}

export type DeliveryOutcomeScenario = 'steady' | 'cost_pressure' | 'under_delivery' | 'creative_fatigue' | 'tracking_anomaly' | 'review_rejected'

export type DeliveryOutcomeSimulation = {
  run: {
    id: string
    executionId: string
    planId: string
    planVersion: number
    modelVersion: string
    scenario: DeliveryOutcomeScenario
    stableSeed: string
    inputHash: string
    status: 'completed'
    input: {
      budgetMinor: number
      scheduleStart: string
      scheduleEnd: string
      optimizationGoal: string
      bidMinor: number
      audience: string
      strategyVersion: number
      creativeCount: number
    }
    parameters: {
      baseCpmMinor: number
      baseCtrBP: number
      baseCvrBP: number
      dailyBudgetMinor: number
      factors: Array<{ key: string; valueBP: number; explanation: string; evidence: string[] }>
    }
    events: Array<{ type: string; severity: string; windowSequence: number; explanation: string; evidence: string[] }>
    evidence: string[]
    completedAt: string
  }
  metricSnapshots: Array<{
    id: string
    simulationRunId: string
    windowSequence: number
    windowStart: string
    windowEnd: string
    impressions: number
    clicks: number
    conversions: number
    spendCents: number
    revenueCents: number
    calculationBasis: {
      formula: string
      spendMultiplierBP: number
      reachMultiplierBP: number
      ctrMultiplierBP: number
      cvrMultiplierBP: number
      trackingRateBP: number
    }
  }>
  replay: boolean
}

type WireDeliveryOutcomeSimulation = {
  run: {
    id: string
    execution_id: string
    plan_id: string
    plan_version: number
    model_version: string
    scenario: DeliveryOutcomeScenario
    stable_seed: string
    input_hash: string
    status: 'completed'
    input: {
      budget: { total_minor: number }
      schedule: { start_at: string; end_at: string }
      optimization_goal: string
      bid_minor: number
      audience: string
      strategy_reference: { version: number }
      creative_features: unknown[]
    }
    parameters: {
      base_cpm_minor: number
      base_ctr_bp: number
      base_cvr_bp: number
      daily_budget_minor: number
      factors: Array<{ key: string; value_bp: number; explanation: string; evidence: string[] }>
    }
    events: Array<{ type: string; severity: string; window_sequence: number; explanation: string; evidence: string[] }>
    evidence: string[]
    completed_at: string
  }
  metric_snapshots: Array<{
    id: string
    simulation_run_id: string
    window_sequence: number
    window_start: string
    window_end: string
    raw_metrics: { impressions: number; clicks: number; conversions: number; spend_cents: number; revenue_cents?: number }
    calculation_basis: { formula: string; spend_multiplier_bp: number; reach_multiplier_bp: number; ctr_multiplier_bp: number; cvr_multiplier_bp: number; tracking_rate_bp: number }
  }>
  replay: boolean
}

type WireDeliveryRecommendation = {
  id: string
  plan_id: string
  plan_version: number
  target_snapshot?: unknown
  base_configuration?: PlatformConfiguration | null
  target_configuration?: PlatformConfiguration | null
  base_snapshot_hash?: string
  target_snapshot_hash?: string
  runtime_status?: 'active' | 'capability_pending' | 'legacy_unsupported'
  read_only?: boolean
  status?: string
  state?: string
  version: number
  evidence?: string[] | null
  evidence_refs?: string[] | null
  action: unknown
  impact: unknown
  risks?: unknown[] | null
  observation?: unknown
  observation_window?: unknown
  cooldown_until?: string | null
  cooldown?: unknown
  provenance: string
  source?: DeliverySource
  scenario?: string
  created_at?: string
  updated_at?: string
}

type WireDeliveryDecision = {
  schema_version: 'delivery-decision/v1'
  id: string
  organization_id: string
  project_id: string
  policy_version: DeliveryDecision['policyVersion']
  diagnostic: { code: DeliveryDecision['diagnostic']['code']; explanation: string; next_action: string }
  inputs: { plan_id: string; plan_version: number; plan_canonical_hash: string; intent_canonical_hash: string; configuration_canonical_hash: string; fact_snapshot_ref: string; simulation_run_id?: string; simulation_input_hash?: string }
  candidates: Array<{ id: string; kind: DeliveryDecisionCandidate['kind']; target_configuration: PlatformConfiguration; budget_change_percent: number; rationale: string[]; constraints: DeliveryDecisionCandidate['constraints']; risks: string[]; uncertainty: DeliveryDecisionCandidate['uncertainty']; calibration_manifest: OceanEngineCalibrationManifestBinding; optimization_focus?: string; proposed_action?: string; action_magnitude_percent?: number; scenario?: string; scenario_probability?: number }>
  recommended_candidate_id: string
  evidence: string[]
  canonical_hash: string
  created_by: string
  created_at: string
}

type WireDecisionSelection = {
  id: string
  decision_id: string
  decision_canonical_hash: string
  candidate_id: string
  configuration: PlatformConfiguration
  workflow: {
    schema_version: 'compiled-delivery-workflow/v1'; id: string; decision_id: string; decision_canonical_hash: string; selected_candidate_id: string; configuration_canonical_hash: string; configuration_id: string; configuration_version: number; platform: 'ocean_engine'; profile_version: 'oceanengine-configuration/v1'; account_reference: StableReference; capability_contract_version: 'oceanengine-capability/v0.1'; selector_contract_version: 'oceanengine-selector-contract/v0.1'; action_contract_version: 'oceanengine-action-contract/v0.1'; execution_driver: 'playwright-rpa/edge/v1'; calibration_manifest: OceanEngineCalibrationManifestBinding; compiler_version: 'oceanengine-workflow-compiler/v1'; status: 'ready_for_final_approval'; remote_write_enabled: false
    steps: Array<{ id: string; sequence: number; page: string; action: string; risk: 'observe' | 'prepare_local_form' | 'remote_write'; preconditions: string[]; fields: Array<{ key: string; value: unknown; expected_readback: unknown; evidence_ref: string }>; timeout_seconds: number; recovery: string; blocked: boolean; block_reason?: 'PHASE_C_REMOTE_WRITE_PROHIBITED' }>
    canonical_hash: string; created_at: string
  }
  final_approval_binding: { status: 'ready_for_final_approval'; action: 'remote_write'; plan_canonical_hash: string; intent_canonical_hash: string; decision_canonical_hash: string; configuration_canonical_hash: string; workflow_canonical_hash: string }
  created_at: string
}

type WireObservatoryRun = {
  schema_version: 'delivery-observatory-run/v1'; id: string; source: DeliveryObservatoryRun['source']; mode: DeliveryObservatoryRun['mode']; input_hash: string
  binding: { selection_id: string; decision_id: string; decision_canonical_hash: string; configuration_canonical_hash: string; workflow_id: string; workflow_canonical_hash: string }
  data_state: DeliveryObservatoryRun['dataState']; data_state_reason: string; status: DeliveryObservatoryRun['status']; outcome: DeliveryObservatoryRun['outcome']; remote_write_enabled: false
  steps: Array<{ step_id: string; sequence: number; page: string; workflow_action: string; executed_action: 'observe' | 'prepare_local_form'; status: string; selector_matches: string[]; evidence_refs: string[]; page_refs: string[]; diffs: Array<{ key: string; evidence_ref: string; expected_value: unknown; observed_value: unknown; matches: boolean }>; block_reason?: string }>
  evidence_refs: string[]; page_refs: string[]; canonical_hash: string; created_at: string
}

type WireObservatoryFeedback = {
  schema_version: 'delivery-observatory-feedback/v1'; id: string; run_id: string; run_canonical_hash: string; run_outcome: DeliveryObservatoryRun['outcome']; disposition: DeliveryObservatoryFeedback['disposition']; reason: string; diff_keys: string[]
  final_configuration?: PlatformConfiguration; final_configuration_canonical_hash?: string; canonical_hash: string; created_by: string; created_at: string
}

type WireDeliveryPlan = {
  id: string
  organization_id: string
  project_id: string
  status: 'draft'
  platform: 'ocean_engine_mock' | DeliveryPlatform
  source: DeliverySource
  scenario: DeliveryScenario
  tour_run_id?: string | null
  tour_owner_id?: string | null
  tour_case?: DeliveryTourCaseKey | null
  current_version_number: number
  current_version: WireDeliveryPlanVersion
  versions: WireDeliveryPlanVersion[]
  created_by: string
  created_at: string
  updated_at: string
}

type WireDeliveryApproval = {
  approval_id: string
  valid: boolean
  invalid_reason?: DeliveryApproval['invalidReason']
  plan_id: string
  plan_version: number
  change_set_id: string
  change_set_version: number
  plan_canonical_hash: string
  target_snapshot_hash?: string
  configuration_schema_version?: string
  configuration_id?: string
  configuration_version?: number
  configuration_platform?: DeliveryPlatform
  configuration_profile_version?: string
  configuration_canonical_hash?: string
  intent_schema_version?: string
  intent_id?: string
  intent_version?: number
  intent_canonical_hash?: string
  action_hash: string
  hash_summary: string
  action: 'execute'
  scope: 'execute_mock'
  budget_limit: { total_minor: number; currency: 'CNY' }
  approved_by: string
  approved_at: string
  expires_at: string
  source: DeliverySource
  scenario: DeliveryScenario
}

type WireDeliveryControlChangeSet = {
  id: string
  organization_id: string
  project_id: string
  plan_id: string
  plan_name: string
  plan_version: number
  plan_canonical_hash: string
  target_snapshot?: PlatformConfiguration
  legacy_target_snapshot?: unknown
  target_snapshot_hash?: string
  runtime_status?: 'active' | 'capability_pending' | 'legacy_unsupported'
  read_only?: boolean
  recommendation_id?: string
  budget_limit: { total_minor: number; currency: 'CNY' }
  status: DeliveryControlChangeSet['status']
  risk_level: string
  preflight_notes: string[]
  approved_by?: string
  approved_at?: string
  rejected_by?: string
  rejected_at?: string
  rejection_reason?: string
  approval?: WireDeliveryApproval
  source: DeliverySource
  scenario: DeliveryScenario
  version: number
  created_by: string
  created_at: string
  updated_at: string
}

type WireDeliveryExecutionStep = {
  id: string
  sequence: number
  action: string
  status: DeliveryExecutionStepStatus
  attempt: number
  effect: DeliveryExecutionStep['effect']
  outcome_summary: string
  evidence_ref?: string | null
  started_at?: string | null
  completed_at?: string | null
  version: number
}

type WireDeliveryExecution = {
  id: string
  organization_id: string
  project_id: string
  change_set_id: string
  approval_id?: string | null
  status: DeliveryExecutionStatus
  version: number
  mode: 'local_simulation'
  adapter: 'mock_ocean_engine'
  source: DeliverySource
  scenario: DeliveryExecutionScenario
  idempotency_key: string
  request_hash: string
  executed_by: string
  started_at: string
  completed_at?: string | null
  retry_allowed: boolean
  recovery_action: DeliveryExecution['recoveryAction']
  recovery_reason: string
  compensation_candidates?: string[] | null
  steps?: WireDeliveryExecutionStep[] | null
}

type WireDeliveryExecutionEvidence = {
  id: string
  execution_id: string
  summary: string
  source?: DeliverySource
  scenario?: DeliveryExecutionScenario
  references?: string[] | null
}

type WireDeliveryExecutionRecord = {
  change_set: WireDeliveryControlChangeSet
  execution: WireDeliveryExecution
  evidence: WireDeliveryExecutionEvidence
}

export const deliveryPlanApi = {
  async list(projectId: string): Promise<DeliveryPlan[]> {
    const response = await deliveryPlanRequest<{ items: WireDeliveryPlan[]; source: DeliverySource; scenario: DeliveryScenario }>(
      projectId,
      '/plans',
    )
    return (response.items ?? []).map(toDeliveryPlan)
  },
  async create(projectId: string, draft: DeliveryPlanDraft): Promise<DeliveryPlan> {
    const response = await deliveryPlanRequest<WireDeliveryPlan>(projectId, '/plans', {
      method: 'POST',
      body: JSON.stringify(toPlatformRuntimeDraft(projectId, `new-${Date.now()}`, 1, draft)),
    })
    return toDeliveryPlan(response)
  },
  async update(projectId: string, planId: string, expectedVersion: number, draft: DeliveryPlanDraft): Promise<DeliveryPlan> {
    const response = await deliveryPlanRequest<WireDeliveryPlan>(projectId, `/plans/${encodeURIComponent(planId)}`, {
      method: 'PATCH',
      body: JSON.stringify({ expected_version: expectedVersion, ...toPlatformRuntimeDraft(projectId, planId, expectedVersion + 1, draft) }),
    })
    return toDeliveryPlan(response)
  },
  async updatePlatformConfiguration(projectId: string, plan: DeliveryPlan, configuration: PlatformConfiguration): Promise<DeliveryPlan> {
    const intent = plan.currentVersion.deliveryIntent
    if (!intent) throw new DeliveryApiError('LEGACY_CONFIGURATION_UNSUPPORTED', 409, '当前计划没有可编辑的业务意图。')
    const nextVersion = plan.currentVersionNumber + 1
    const oceanEngine = configuration.payload.ocean_engine
    const revisedOceanEngine = oceanEngine ? {
      ...oceanEngine,
      project: {
        ...oceanEngine.project,
        project_draft_id: `project-${plan.id}-${nextVersion}`,
      },
      promotions: oceanEngine.promotions.map((promotion, index) => ({
        ...promotion,
        promotion_draft_id: `promotion-${plan.id}-${nextVersion}-${index + 1}`,
      })),
    } : undefined
    const nextConfiguration = {
      ...configuration,
      configuration_id: planRevisionIdentity('configuration', plan.id, nextVersion),
      version_number: nextVersion,
      canonical_hash: undefined,
      intent: { schema_version: 'delivery-intent/v1' as const, intent_id: planRevisionIdentity('intent', plan.id, nextVersion), version_number: nextVersion },
      payload: {
        ...configuration.payload,
        ocean_engine: revisedOceanEngine,
      },
    }
    const marketingProduct = nextConfiguration.payload.ocean_engine?.project.marketing_product_reference
    const promotions = nextConfiguration.payload.ocean_engine?.promotions ?? []
    const configuredMaterials = promotions.flatMap(promotion => [
      ...promotion.base_material_references,
      ...(promotion.product_image_references ?? []),
    ])
    const configuredLandingPages = promotions.flatMap(promotion => promotion.landing_page_reference ? [promotion.landing_page_reference] : [])
    const nextIntent = {
      ...intent,
      intent_id: nextConfiguration.intent.intent_id,
      version_number: nextVersion,
      canonical_hash: undefined,
      payload: {
        ...intent.payload,
        product_references: marketingProduct ? [structuredClone(marketingProduct)] : intent.payload.product_references,
        material_references: mergeStableReferences(intent.payload.material_references, configuredMaterials),
        landing_page_references: mergeStableReferences(intent.payload.landing_page_references, configuredLandingPages),
      },
    }
    return toDeliveryPlan(await deliveryPlanRequest<WireDeliveryPlan>(projectId, `/plans/${encodeURIComponent(plan.id)}`, {
      method: 'PATCH',
      body: JSON.stringify({ expected_version: plan.currentVersionNumber, intent: nextIntent, platform_configuration: nextConfiguration }),
    }))
  },
  async listChangeSets(projectId: string): Promise<DeliveryControlChangeSet[]> {
    const response = await deliveryPlanRequest<{ items: WireDeliveryControlChangeSet[] }>(projectId, '/change-sets')
    return (response.items ?? []).map(toDeliveryControlChangeSet)
  },
  async getChangeSet(projectId: string, changeSetId: string): Promise<DeliveryControlChangeSet> {
    return toDeliveryControlChangeSet(await deliveryPlanRequest<WireDeliveryControlChangeSet>(
      projectId,
      `/change-sets/${encodeURIComponent(changeSetId)}`,
    ))
  },
  async approveChangeSet(projectId: string, changeSetId: string, expectedVersion: number): Promise<DeliveryControlChangeSet> {
    return deliveryChangeSetAction(projectId, changeSetId, 'approve', expectedVersion)
  },
  async rejectChangeSet(projectId: string, changeSetId: string, expectedVersion: number, reason: string): Promise<DeliveryControlChangeSet> {
    const response = await deliveryPlanRequest<WireDeliveryControlChangeSet>(
      projectId,
      `/change-sets/${encodeURIComponent(changeSetId)}:reject`,
      { method: 'POST', body: JSON.stringify({ expected_version: expectedVersion, reason }) },
    )
    return toDeliveryControlChangeSet(response)
  },
}

/** Phase C Decision -> CompiledWorkflow authority spine. Recommendation methods below remain historical-tour compatibility only. */
export const deliveryOptimizationApi = {
  async generateDecision(projectId: string, planId: string, expectedVersion: number): Promise<DeliveryDecision> {
    return toDeliveryDecision(await deliveryPlanRequest<WireDeliveryDecision>(
      projectId,
      `/plans/${encodeURIComponent(planId)}/decisions:generate`,
      { method: 'POST', body: JSON.stringify({ expected_version: expectedVersion }) },
    ))
  },
  async listDecisions(projectId: string): Promise<DeliveryDecision[]> {
    const response = await deliveryPlanRequest<{ items?: WireDeliveryDecision[] | null }>(projectId, '/decisions')
    return (response.items ?? []).map(toDeliveryDecision)
  },
  async getDecision(projectId: string, decisionId: string): Promise<DeliveryDecision> {
    return toDeliveryDecision(await deliveryPlanRequest<WireDeliveryDecision>(projectId, `/decisions/${encodeURIComponent(decisionId)}`))
  },
  async selectDecision(projectId: string, decisionId: string, candidateId: string, expectedPlanVersion: number, idempotencyKey: string): Promise<DeliveryDecisionSelection> {
    return toDecisionSelection(await deliveryPlanRequest<WireDecisionSelection>(
      projectId,
      `/decisions/${encodeURIComponent(decisionId)}:select`,
      { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ candidate_id: candidateId, expected_plan_version: expectedPlanVersion }) },
    ))
  },
  async runObservatory(projectId: string, selectionId: string, mode: DeliveryObservatoryRun['mode'], source: DeliveryObservatoryRun['source'] = 'replay'): Promise<DeliveryObservatoryRun> {
    const now = new Date()
    return toObservatoryRun(await deliveryPlanRequest<WireObservatoryRun>(projectId, `/decision-selections/${encodeURIComponent(selectionId)}/observatory-runs`, {
      method: 'POST', body: JSON.stringify({ source, mode, fixture: { fixture_id: `workspace-${selectionId}`, data_state: 'ready', data_state_reason: '', observed_at: now.toISOString(), data_through: now.toISOString(), observed_values: {}, selector_matches: {}, evidence_refs: [`${source}://workspace/${selectionId}`], page_refs: ['replay://local/observatory'] } }),
    }))
  },
  async listObservatoryRuns(projectId: string): Promise<DeliveryObservatoryRun[]> {
    const response = await deliveryPlanRequest<{ items?: WireObservatoryRun[] | null }>(projectId, '/observatory-runs')
    return (response.items ?? []).map(toObservatoryRun)
  },
  async getObservatoryRun(projectId: string, runId: string): Promise<DeliveryObservatoryRun> {
    return toObservatoryRun(await deliveryPlanRequest<WireObservatoryRun>(projectId, `/observatory-runs/${encodeURIComponent(runId)}`))
  },
  async listObservatoryFeedback(projectId: string, runId: string): Promise<DeliveryObservatoryFeedback[]> {
    const response = await deliveryPlanRequest<{ items?: WireObservatoryFeedback[] | null }>(projectId, `/observatory-runs/${encodeURIComponent(runId)}/feedback`)
    return (response.items ?? []).map(toObservatoryFeedback)
  },
  async submitObservatoryFeedback(projectId: string, runId: string, disposition: DeliveryObservatoryFeedback['disposition'], reason: string, diffKeys: string[], idempotencyKey: string, finalConfiguration?: PlatformConfiguration): Promise<DeliveryObservatoryFeedback> {
    return toObservatoryFeedback(await deliveryPlanRequest<WireObservatoryFeedback>(projectId, `/observatory-runs/${encodeURIComponent(runId)}/feedback`, {
      method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ disposition, reason, diff_keys: diffKeys, ...(finalConfiguration ? { final_configuration: finalConfiguration } : {}) }),
    }))
  },
  async generateRecommendations(projectId: string, planId: string, expectedVersion: number): Promise<DeliveryRecommendation> {
    const response = await deliveryPlanRequest<WireDeliveryRecommendation>(
      projectId,
      `/plans/${encodeURIComponent(planId)}/recommendations:generate`,
      { method: 'POST', body: JSON.stringify({ expected_version: expectedVersion }) },
    )
    return toDeliveryRecommendation(response)
  },
  async listRecommendations(projectId: string): Promise<DeliveryRecommendation[]> {
    const response = await deliveryPlanRequest<{ items?: WireDeliveryRecommendation[] | null }>(projectId, '/recommendations')
    return (response.items ?? []).map(toDeliveryRecommendation)
  },
  async getRecommendation(projectId: string, recommendationId: string): Promise<DeliveryRecommendation> {
    return toDeliveryRecommendation(await deliveryPlanRequest<WireDeliveryRecommendation>(projectId, `/recommendations/${encodeURIComponent(recommendationId)}`))
  },
  async acceptRecommendation(projectId: string, recommendationId: string, expectedVersion: number, idempotencyKey: string): Promise<{
    recommendation: DeliveryRecommendation
    changeSet: DeliveryControlChangeSet
  }> {
    const response = await deliveryPlanRequest<{ recommendation: WireDeliveryRecommendation; change_set: WireDeliveryControlChangeSet }>(
      projectId,
      `/recommendations/${encodeURIComponent(recommendationId)}:accept`,
      { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ expected_version: expectedVersion }) },
    )
    return { recommendation: toDeliveryRecommendation(response.recommendation), changeSet: toDeliveryControlChangeSet(response.change_set) }
  },
  async rejectRecommendation(projectId: string, recommendationId: string, expectedVersion: number): Promise<DeliveryRecommendation> {
    return toDeliveryRecommendation(await deliveryPlanRequest<WireDeliveryRecommendation>(
      projectId,
      `/recommendations/${encodeURIComponent(recommendationId)}:reject`,
      { method: 'POST', body: JSON.stringify({ expected_version: expectedVersion }) },
    ))
  },
}

export const deliveryExecutionApi = {
  async listPlatformEntityMappings(projectId: string, accountReferenceId: string): Promise<DeliveryPlatformEntityMapping[]> {
    const response = await deliveryPlanRequest<{ items?: DeliveryPlatformEntityMapping[] | null }>(
      projectId,
      `/platform-entity-mappings?account_reference_id=${encodeURIComponent(accountReferenceId)}`,
    )
    return response.items ?? []
  },
  async startBrowserRpaExecution(
    projectId: string,
    planId: string,
    expectedVersion: number,
    executionDriver: DeliveryExecutionDriver,
    idempotencyKey: string,
  ): Promise<{ controlled_change_set: { id: string }; controlled_execution: { id: string }; browser_rpa_run: { run_id: string } }> {
    return deliveryPlanRequest<{ controlled_change_set: { id: string }; controlled_execution: { id: string }; browser_rpa_run: { run_id: string } }>(
      projectId,
      `/plans/${encodeURIComponent(planId)}/browser-rpa-runs`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': idempotencyKey },
        body: JSON.stringify({ expected_version: expectedVersion, execution_driver: executionDriver }),
      },
    )
  },
  async execute(
    projectId: string,
    changeSetId: string,
    expectedVersion: number,
    scenario: DeliveryExecutionScenario,
    idempotencyKey: string,
  ): Promise<DeliveryExecutionRecord> {
    return toDeliveryExecutionRecord(await deliveryPlanRequest<WireDeliveryExecutionRecord>(
      projectId,
      `/change-sets/${encodeURIComponent(changeSetId)}:execute`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': idempotencyKey },
        body: JSON.stringify({ expected_version: expectedVersion, scenario }),
      },
    ))
  },
  async list(projectId: string): Promise<DeliveryExecutionRecord[]> {
    const response = await deliveryPlanRequest<{ items?: WireDeliveryExecutionRecord[] | null }>(projectId, '/executions')
    return (response.items ?? []).map(toDeliveryExecutionRecord)
  },
  async get(projectId: string, executionId: string): Promise<DeliveryExecutionRecord> {
    return toDeliveryExecutionRecord(await deliveryPlanRequest<WireDeliveryExecutionRecord>(
      projectId,
      `/executions/${encodeURIComponent(executionId)}`,
    ))
  },
  async runOutcomeSimulation(projectId: string, executionId: string, scenario: DeliveryOutcomeScenario, stableSeed: string): Promise<DeliveryOutcomeSimulation> {
    return toDeliveryOutcomeSimulation(await deliveryPlanRequest<WireDeliveryOutcomeSimulation>(
      projectId,
      `/executions/${encodeURIComponent(executionId)}/simulation-runs`,
      { method: 'POST', body: JSON.stringify({ scenario, stable_seed: stableSeed }) },
    ))
  },
  async getLatestOutcomeSimulation(projectId: string, executionId: string): Promise<DeliveryOutcomeSimulation> {
    return toDeliveryOutcomeSimulation(await deliveryPlanRequest<WireDeliveryOutcomeSimulation>(
      projectId,
      `/executions/${encodeURIComponent(executionId)}/simulation-run`,
    ))
  },
  async createMetricSnapshot(projectId: string, executionId: string): Promise<void> {
    await deliveryPlanRequest(
      projectId,
      `/executions/${encodeURIComponent(executionId)}/metric-snapshots`,
      {
        method: 'POST',
        body: JSON.stringify({ dataset_version: 'preroll-demo/v1' }),
      },
    )
  },
}

export type DeliveryExecutionDriver = 'oceanengine-web-api/session/v1' | 'playwright-rpa/edge/v3'

export type DeliveryPlatformEntityMapping = {
  id: string
  account_reference_id: string
  plan_id: string
  configuration_id: string
  business_execution_id: string
  browser_rpa_run_id: string
  internal_object_kind: 'project' | 'promotion' | string
  internal_object_id: string
  platform_object_kind: 'project' | 'promotion' | string
  platform_object_id: string
  platform_status: string
  status: 'pending_verification' | 'confirmed'
  version: number
  created_at: string
  updated_at: string
}

function toDeliveryOutcomeSimulation(value: WireDeliveryOutcomeSimulation): DeliveryOutcomeSimulation {
  return {
    run: {
      id: value.run.id,
      executionId: value.run.execution_id,
      planId: value.run.plan_id,
      planVersion: value.run.plan_version,
      modelVersion: value.run.model_version,
      scenario: value.run.scenario,
      stableSeed: value.run.stable_seed,
      inputHash: value.run.input_hash,
      status: value.run.status,
      input: {
        budgetMinor: value.run.input.budget.total_minor,
        scheduleStart: value.run.input.schedule.start_at,
        scheduleEnd: value.run.input.schedule.end_at,
        optimizationGoal: value.run.input.optimization_goal,
        bidMinor: value.run.input.bid_minor,
        audience: value.run.input.audience,
        strategyVersion: value.run.input.strategy_reference.version,
        creativeCount: value.run.input.creative_features.length,
      },
      parameters: {
        baseCpmMinor: value.run.parameters.base_cpm_minor,
        baseCtrBP: value.run.parameters.base_ctr_bp,
        baseCvrBP: value.run.parameters.base_cvr_bp,
        dailyBudgetMinor: value.run.parameters.daily_budget_minor,
        factors: value.run.parameters.factors.map(factor => ({ key: factor.key, valueBP: factor.value_bp, explanation: factor.explanation, evidence: factor.evidence ?? [] })),
      },
      events: value.run.events.map(event => ({ type: event.type, severity: event.severity, windowSequence: event.window_sequence, explanation: event.explanation, evidence: event.evidence ?? [] })),
      evidence: value.run.evidence ?? [],
      completedAt: value.run.completed_at,
    },
    metricSnapshots: value.metric_snapshots.map(metric => ({
      id: metric.id,
      simulationRunId: metric.simulation_run_id,
      windowSequence: metric.window_sequence,
      windowStart: metric.window_start,
      windowEnd: metric.window_end,
      impressions: metric.raw_metrics.impressions,
      clicks: metric.raw_metrics.clicks,
      conversions: metric.raw_metrics.conversions,
      spendCents: metric.raw_metrics.spend_cents,
      revenueCents: metric.raw_metrics.revenue_cents ?? 0,
      calculationBasis: {
        formula: metric.calculation_basis.formula,
        spendMultiplierBP: metric.calculation_basis.spend_multiplier_bp,
        reachMultiplierBP: metric.calculation_basis.reach_multiplier_bp,
        ctrMultiplierBP: metric.calculation_basis.ctr_multiplier_bp,
        cvrMultiplierBP: metric.calculation_basis.cvr_multiplier_bp,
        trackingRateBP: metric.calculation_basis.tracking_rate_bp,
      },
    })),
    replay: value.replay,
  }
}

/** Server-authoritative monitoring records. No client-side alert synthesis is permitted. */
export type DeliveryAlertFixture = 'normal_day' | 'anomaly_day' | 'stale_data' | 'insufficient_data'
export type DeliveryAlertStatus = 'open' | 'acknowledged' | 'dismissed'
export type DeliveryAlertAction = 'acknowledge' | 'dismiss'

export type DeliveryAlert = {
  id: string
  organizationId: string
  projectId: string
  planId: string
  executionId: string
  simulationRunId?: string
  monitoredEntity: { type: 'delivery_plan' | 'platform_promotion'; id: string; advertiserId: string }
  type: 'review_rejected' | 'spend_spike' | 'zero_conversion' | 'cost_worsening' | 'under_delivery' | 'creative_fatigue' | 'tracking_anomaly'
  ruleId: string
  ruleVersion: string
  fingerprint: string
  severity: 'critical' | 'high' | 'medium' | 'low'
  status: DeliveryAlertStatus
  version: number
  window: { start: string; end: string; timezone: string; dataThrough: string; baselineStart?: string; baselineEnd?: string }
  metricDefinition: { name: string; unit: string; numerator?: number; denominator?: number; observedValue?: number; baselineValue?: number; threshold?: number }
  owner: { id: string; displayName: string; source: string }
  evidenceRefs: string[]
  source: 'demo_fixture' | 'connector'
  isSimulated: boolean
  scenario: DeliveryAlertFixture | 'connector_inspection'
  datasetVersion: string
  fixtureVersion: string
  freshness: { status: 'fresh' | 'stale' | 'unknown' | 'insufficient_data'; asOf: string; evaluatedAt: string; ageSeconds: number; maxAgeSeconds: number; missingMetrics?: string[] }
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type DeliveryAlertEvaluation = {
  items: DeliveryAlert[]
  createdCount: number
  reusedCount: number
  source: 'demo_fixture'
  isSimulated: true
  scenario: DeliveryAlertFixture
  evaluatedAt: string
  insightsSource?: 'mock' | 'replay' | 'connector'
  insightsQuality?: 'usable' | 'empty' | 'stale' | 'incomplete' | 'schema_mismatch' | 'unavailable'
  insightsQualityReason?: string
  insightsFixtureVersion?: string
  insightsEvidenceRefs?: string[]
}

type WireDeliveryAlert = {
  id: string
  organization_id: string
  project_id: string
  plan_id: string
  execution_id: string
  simulation_run_id?: string
  monitored_entity: { type: 'delivery_plan' | 'platform_promotion'; id: string; advertiser_id: string }
  type: DeliveryAlert['type']
  rule_id: string
  rule_version: string
  fingerprint: string
  severity: DeliveryAlert['severity']
  status: DeliveryAlertStatus
  version: number
  window: { start: string; end: string; timezone: string; data_through: string; baseline_start?: string; baseline_end?: string }
  metric_definition: { name: string; unit: string; numerator?: number; denominator?: number; observed_value?: number; baseline_value?: number; threshold?: number }
  owner: { id: string; display_name: string; source: string }
  evidence_refs: string[]
  source: 'demo_fixture' | 'connector'
  is_simulated: boolean
  scenario: DeliveryAlertFixture | 'connector_inspection'
  dataset_version: string
  fixture_version: string
  freshness: { status: DeliveryAlert['freshness']['status']; as_of: string; evaluated_at: string; age_seconds: number; max_age_seconds: number; missing_metrics?: string[] | null }
  created_by: string
  created_at: string
  updated_at: string
}

type WireDeliveryAlertEvaluation = {
  items: WireDeliveryAlert[]
  created_count: number
  reused_count: number
  source: 'demo_fixture'
  is_simulated: true
  scenario: DeliveryAlertFixture
  evaluated_at: string
  insights_source?: 'mock' | 'replay' | 'connector'
  insights_quality?: DeliveryAlertEvaluation['insightsQuality']
  insights_quality_reason?: string
  insights_fixture_version?: string
  insights_evidence_refs?: string[]
}

export const deliveryAlertApi = {
  async evaluate(projectId: string, fixture: DeliveryAlertFixture, executionId?: string): Promise<DeliveryAlertEvaluation> {
    const response = await deliveryPlanRequest<WireDeliveryAlertEvaluation>(projectId, '/alerts:evaluate', {
      method: 'POST',
      body: JSON.stringify({ fixture, ...(executionId ? { execution_id: executionId } : {}) }),
    })
    return toDeliveryAlertEvaluation(response)
  },
  async inspect(projectId: string, planId: string, windowDays = 14): Promise<ConnectorInspection> {
    const response = await deliveryPlanRequest<WireConnectorInspection>(projectId, '/alerts:inspect', {
      method: 'POST', body: JSON.stringify({ plan_id: planId, window_days: windowDays }),
    })
    return {
      items: response.items.map(toDeliveryAlert), createdCount: response.created_count, reusedCount: response.reused_count,
      source: response.source, isSimulated: response.is_simulated, status: response.status, statusReason: response.status_reason,
      datasetVersion: response.dataset_version, evaluatedAt: response.evaluated_at, dataThrough: response.data_through,
      evidenceRefs: response.evidence_refs ?? [],
    }
  },
  async list(projectId: string, filter: { planId?: string; executionId?: string } = {}): Promise<DeliveryAlert[]> {
    const items: DeliveryAlert[] = []
    let cursor: string | null = null
    do {
      const query = new URLSearchParams()
      if (filter.planId) query.set('plan_id', filter.planId)
      if (filter.executionId) query.set('execution_id', filter.executionId)
      if (cursor) query.set('cursor', cursor)
      const response: { items: WireDeliveryAlert[]; next_cursor: string | null; source: 'demo_fixture'; is_simulated: true } = await deliveryPlanRequest(projectId, `/alerts${query.size ? `?${query}` : ''}`)
      items.push(...response.items.map(toDeliveryAlert))
      cursor = response.next_cursor ?? null
    } while (cursor !== null)
    return items
  },
  async action(projectId: string, alertId: string, action: DeliveryAlertAction, expectedVersion: number): Promise<DeliveryAlert> {
    return toDeliveryAlert(await deliveryPlanRequest<WireDeliveryAlert>(projectId, `/alerts/${encodeURIComponent(alertId)}`, {
      method: 'PATCH',
      body: JSON.stringify({ action, expected_version: expectedVersion }),
    }))
  },
}

export type ConnectorInspection = {
  items: DeliveryAlert[]
  createdCount: number
  reusedCount: number
  source: 'connector'
  isSimulated: false
  status: 'ready' | 'insufficient_data' | 'quarantined' | 'stale' | 'unavailable'
  statusReason?: string
  datasetVersion: string
  evaluatedAt: string
  dataThrough?: string
  evidenceRefs: string[]
}

type WireConnectorInspection = {
  items: WireDeliveryAlert[]
  created_count: number
  reused_count: number
  source: 'connector'
  is_simulated: false
  status: ConnectorInspection['status']
  status_reason?: string
  dataset_version: string
  evaluated_at: string
  data_through?: string
  evidence_refs?: string[]
}

export type MechanisticPriorSet = {
  version: string
  review_pass_probability: { value: number; source: string; unit: 'probability'; scope: string[]; uncertainty: string }
  delivery_probability: { value: number; source: string; unit: 'probability'; scope: string[]; uncertainty: string }
  budget_utilization: { minimum: number; mode: number; maximum: number; source: string; unit: 'ratio'; scope: string[]; uncertainty: string }
  cpm: { minimum: number; mode: number; maximum: number; source: string; unit: string; scope: string[]; uncertainty: string }
  ctr: { minimum: number; mode: number; maximum: number; source: string; unit: 'ratio'; scope: string[]; uncertainty: string }
  cvr: { minimum: number; mode: number; maximum: number; source: string; unit: 'ratio'; scope: string[]; uncertainty: string }
  tracking_observable_rate: { value: number; source: string; unit: 'probability'; scope: string[]; uncertainty: string }
  creative_fatigue?: { enabled: boolean; daily_rate: number; source: string; unit: 'ratio_per_day'; scope: string[]; uncertainty: string }
}

export type MechanisticSimulation = {
  id: string
  planId: string
  planVersion: number
  modelVersion: string
  priorSetVersion: string
  stableSeed: string
  predictionHorizon: string
  sampleCount: number
  status: string
  isSimulated: true
  calibrationStatus: 'assumption_driven' | 'account_product_calibrated'
  calibrationPriorRef?: string
  metricWindows: Array<{ sequence: number; start: string; end: string; timezone: string; metrics: Record<string, { available: boolean; unit: string; p10?: number; p50?: number; p90?: number; mean?: number }> }>
  scenarioProbabilities: Array<{ scenario: string; probability: number; status: string; limitations: string[] }>
  alerts: Array<{ type: string; severity: string; probability: number; limitations: string[] }>
  recommendationDrafts: MechanisticRecommendationDraft[]
  assumptions: string[]
  limitations: string[]
  evidenceRefs: string[]
}

export type MechanisticRecommendationDraft = {
  recommendation_type: string
  target_field: string
  current_value?: unknown
  suggested_range?: [number, number]
  expected_effect_range?: [number, number]
  confidence: string
  effect_basis?: string
  rationale: string
  evidence_refs?: string[]
  risks?: string[]
  guardrails?: string[]
  requires_human_review: boolean
}

type WireMechanisticSimulation = {
  id: string; plan_id: string; plan_version: number; model_version: string; prior_set_version: string; stable_seed: string
  prediction_horizon: string; sample_count: number; status: string; is_simulated: true; calibration_status: MechanisticSimulation['calibrationStatus']; calibration_prior_ref?: string
  metric_windows: Array<{ sequence: number; start: string; end: string; timezone: string; metrics: MechanisticSimulation['metricWindows'][number]['metrics'] }>
  scenario_probabilities: Array<{ scenario: string; probability: number; status: string; limitations: string[] }>
  alerts: MechanisticSimulation['alerts']; recommendation_drafts: MechanisticSimulation['recommendationDrafts']
  assumptions: string[]; limitations: string[]; evidence_refs: string[]
}

export const deliveryMechanisticSimulationApi = {
  async run(projectId: string, planId: string, version: number, request: { stableSeed: string; sampleCount: number; predictionHorizonDays: number; reviewState: 'unknown' | 'approved' | 'rejected'; priorSet: MechanisticPriorSet; calibrationAccountRef: string }): Promise<MechanisticSimulation> {
    const response = await deliveryPlanRequest<{ result: WireMechanisticSimulation }>(projectId, `/plans/${encodeURIComponent(planId)}/versions/${version}/mechanistic-simulation-runs`, {
      method: 'POST', body: JSON.stringify({ stable_seed: request.stableSeed, sample_count: request.sampleCount, prediction_horizon_days: request.predictionHorizonDays, review_state: request.reviewState, prior_set: request.priorSet, calibration_account_ref: request.calibrationAccountRef }),
    })
    return toMechanisticSimulation(response.result)
  },
  async getLatest(projectId: string, planId: string, version: number): Promise<MechanisticSimulation> {
    return toMechanisticSimulation(await deliveryPlanRequest<WireMechanisticSimulation>(projectId, `/plans/${encodeURIComponent(planId)}/versions/${version}/mechanistic-simulation-run`))
  },
}

function toMechanisticSimulation(value: WireMechanisticSimulation): MechanisticSimulation {
  return {
    id: value.id, planId: value.plan_id, planVersion: value.plan_version, modelVersion: value.model_version,
    priorSetVersion: value.prior_set_version, stableSeed: value.stable_seed, predictionHorizon: value.prediction_horizon,
    sampleCount: value.sample_count, status: value.status, isSimulated: value.is_simulated, calibrationStatus: value.calibration_status, calibrationPriorRef: value.calibration_prior_ref,
    metricWindows: value.metric_windows.map(window => ({ sequence: window.sequence, start: window.start, end: window.end, timezone: window.timezone, metrics: window.metrics })),
    scenarioProbabilities: value.scenario_probabilities, alerts: value.alerts, recommendationDrafts: value.recommendation_drafts,
    assumptions: value.assumptions, limitations: value.limitations, evidenceRefs: value.evidence_refs,
  }
}

async function deliveryChangeSetAction(
  projectId: string,
  changeSetId: string,
  action: 'preflight' | 'approve',
  expectedVersion: number,
) {
  const response = await deliveryPlanRequest<WireDeliveryControlChangeSet>(
    projectId,
    `/change-sets/${encodeURIComponent(changeSetId)}:${action}`,
    { method: 'POST', body: JSON.stringify({ expected_version: expectedVersion }) },
  )
  return toDeliveryControlChangeSet(response)
}

function toDeliveryAlertEvaluation(value: WireDeliveryAlertEvaluation): DeliveryAlertEvaluation {
  return {
    items: value.items.map(toDeliveryAlert),
    createdCount: value.created_count,
    reusedCount: value.reused_count,
    source: value.source,
    isSimulated: value.is_simulated,
    scenario: value.scenario,
    evaluatedAt: value.evaluated_at,
    insightsSource: value.insights_source,
    insightsQuality: value.insights_quality,
    insightsQualityReason: value.insights_quality_reason,
    insightsFixtureVersion: value.insights_fixture_version,
    insightsEvidenceRefs: value.insights_evidence_refs,
  }
}

function toDeliveryAlert(value: WireDeliveryAlert): DeliveryAlert {
  return {
    id: value.id,
    organizationId: value.organization_id,
    projectId: value.project_id,
    planId: value.plan_id,
    executionId: value.execution_id,
    simulationRunId: value.simulation_run_id,
    monitoredEntity: { type: value.monitored_entity.type, id: value.monitored_entity.id, advertiserId: value.monitored_entity.advertiser_id },
    type: value.type,
    ruleId: value.rule_id,
    ruleVersion: value.rule_version,
    fingerprint: value.fingerprint,
    severity: value.severity,
    status: value.status,
    version: value.version,
    window: {
      start: value.window.start,
      end: value.window.end,
      timezone: value.window.timezone,
      dataThrough: value.window.data_through,
      baselineStart: value.window.baseline_start,
      baselineEnd: value.window.baseline_end,
    },
    metricDefinition: {
      name: value.metric_definition.name,
      unit: value.metric_definition.unit,
      numerator: value.metric_definition.numerator,
      denominator: value.metric_definition.denominator,
      observedValue: value.metric_definition.observed_value,
      baselineValue: value.metric_definition.baseline_value,
      threshold: value.metric_definition.threshold,
    },
    owner: { id: value.owner.id, displayName: value.owner.display_name, source: value.owner.source },
    evidenceRefs: value.evidence_refs ?? [],
    source: value.source,
    isSimulated: value.is_simulated,
    scenario: value.scenario,
    datasetVersion: value.dataset_version,
    fixtureVersion: value.fixture_version,
    freshness: {
      status: value.freshness.status,
      asOf: value.freshness.as_of,
      evaluatedAt: value.freshness.evaluated_at,
      ageSeconds: value.freshness.age_seconds,
      maxAgeSeconds: value.freshness.max_age_seconds,
      missingMetrics: value.freshness.missing_metrics ?? undefined,
    },
    createdBy: value.created_by,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  }
}

export type DeliveryTourCaseKey = 'golden_path' | 'preflight_failure' | 'approval_expired' | 'plan_stale' | 'partial_execution' | 'result_unknown' | 'review_rejected_alert'

export type DeliveryTourCase = {
  key: DeliveryTourCaseKey
  title: string
  planId: string
  status: 'missing' | 'prepared' | 'ready' | 'observed'
  expectedOutcome: string
  startUrl: string
  source: DeliverySource
  scenario: DeliveryTourCaseKey
  evidence: string[]
  observedAt: string
}

export type DeliveryTourStep = {
  key: string
  title: string
  completionCondition: string
  complete: boolean
  url: string
  explanation: string
  evidence: string[]
}

async function deliveryPlanRequest<T>(projectId: string, path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body !== undefined) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api/delivery/v1/projects/${encodeURIComponent(projectId)}${path}`, { credentials: 'include', ...init, headers })
  const payload = await response.json() as T | { error?: { code?: string; message?: string; details?: Array<{ field?: string; reason?: string }> } }
  if (!response.ok) {
    const problem = payload as { error?: { code?: string; message?: string; details?: Array<{ field?: string; reason?: string }> } }
    const details = (problem.error?.details ?? []).map(item => ({ field: item.field ?? '', reason: item.reason ?? '' }))
    throw new DeliveryApiError(problem.error?.code, response.status, problem.error?.code === 'VERSION_CONFLICT'
      ? '计划已被其他版本更新，请刷新后再试。'
      : problem.error?.message ?? 'Delivery API 请求失败', details)
  }
  return payload as T
}

function toWireDraft(draft: DeliveryPlanDraft): WireDeliveryPlanDraft {
  return {
    name: draft.name,
    objective: draft.objective,
    marketing_purpose: draft.marketingPurpose || undefined,
    advertiser: draft.advertiser,
    budget: { total_minor: draft.budget.totalMinor, currency: draft.budget.currency },
    schedule: { start_at: draft.schedule.startAt, end_at: draft.schedule.endAt, timezone: draft.schedule.timezone },
    marketing_product: draft.marketingProduct.id ? {
      id: draft.marketingProduct.id, ocean_engine_product_id: draft.marketingProduct.oceanEngineProductId,
      name: draft.marketingProduct.name, activity_type: draft.marketingProduct.activityType,
      activity_name: draft.marketingProduct.activityName, brand_name: draft.marketingProduct.brandName,
    } : undefined,
    tracking: {
      landing_page: draft.tracking.landingPage,
      pixel_id: draft.tracking.pixelId,
      conversion_event: draft.tracking.conversionEvent,
    },
    creative_references: draft.creativeReferences.map(reference => ({
      asset_id: reference.assetId,
      version: reference.version,
      content_hash: reference.contentHash,
      route: reference.route,
      confirmed: reference.confirmed,
      ocean_engine_material_id: reference.oceanEngineMaterialId,
    })),
    strategy_reference: {
      task_id: draft.strategyReference.taskId,
      version: draft.strategyReference.version,
      content_hash: draft.strategyReference.contentHash,
      route: draft.strategyReference.route,
    },
    source_strategy_version: draft.sourceStrategyVersion,
  }
}

function toPlatformRuntimeDraft(projectId: string, identity: string, versionNumber: number, draft: DeliveryPlanDraft) {
  const scope = `project:${projectId}`
  const marketingProductReference: StableReference | undefined = draft.marketingProduct.id ? {
    namespace: 'cookies', object_kind: 'product', scope, id: draft.marketingProduct.id, state: 'resolved',
    display_name_snapshot: draft.marketingProduct.name,
    audit_attributes: { ocean_engine_product_id: draft.marketingProduct.oceanEngineProductId ?? '', activity_type: draft.marketingProduct.activityType, activity_name: draft.marketingProduct.activityName, brand_name: draft.marketingProduct.brandName },
  } : undefined
  const optimizationTargetReference: StableReference | undefined = draft.tracking.optimizationTargetId ? {
    namespace: 'oceanengine', object_kind: 'optimization_target', scope, id: draft.tracking.optimizationTargetId, state: 'resolved',
    semantic_key: draft.tracking.optimizationTargetSemanticKey || undefined,
    display_name_snapshot: draft.tracking.optimizationTargetName,
    audit_attributes: { event_asset_name: draft.tracking.eventAssetName, event_asset_type: draft.tracking.eventAssetType },
  } : undefined
  const monitoringReferences = [
    ['impression', draft.tracking.monitoringImpression], ['valid_touch', draft.tracking.monitoringValidTouch],
    ['video_play', draft.tracking.monitoringVideoPlay], ['video_complete', draft.tracking.monitoringVideoComplete],
    ['valid_video_play', draft.tracking.monitoringValidVideoPlay],
  ].flatMap(([kind, url]) => url ? [{ namespace: 'oceanengine', object_kind: `monitoring_link_${kind}`, scope, id: url, state: 'resolved' as const }] : [])
  const landingPageReference: StableReference | undefined = draft.tracking.deliveryCarrier === 'owned_landing_page' && draft.tracking.landingPage ? {
    namespace: 'cookies', object_kind: 'landing_page', scope, id: draft.tracking.landingPage, state: 'resolved',
  } : undefined
  const materialReferences: StableReference[] = draft.creativeReferences.map(reference => ({
    namespace: 'cookies', object_kind: 'asset_version', scope,
    id: reference.assetId, version: String(reference.version), content_hash: reference.contentHash,
    state: 'resolved', display_name_snapshot: reference.assetId,
    audit_attributes: { ocean_engine_material_id: reference.oceanEngineMaterialId ?? '' },
  }))
  const strategyReference: StableReference = {
    namespace: 'cookies', object_kind: 'strategy_version', scope,
    id: draft.strategyReference.taskId, version: String(draft.strategyReference.version), content_hash: draft.strategyReference.contentHash,
    state: 'resolved', display_name_snapshot: draft.sourceStrategyVersion,
  }
  const intent: DeliveryIntent = {
    schema_version: 'delivery-intent/v1', intent_id: planRevisionIdentity('intent', identity, versionNumber), version_number: versionNumber,
    hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
    payload: {
      payload_schema_version: 'delivery-intent/v1', marketing_objective: draft.objective,
      budget_boundary: { currency: 'CNY', minimum_total_minor: 0, maximum_total_minor: draft.budget.totalMinor },
      schedule_boundary: { earliest_start: draft.schedule.startAt, latest_end: draft.schedule.endAt, timezone: draft.schedule.timezone },
      optimization_preferences: [], material_references: materialReferences,
      landing_page_references: landingPageReference ? [landingPageReference] : [],
      audience_constraints: { constraints: [] }, strategy_reference: strategyReference,
      calibration_manifest: { schema_version: 'oceanengine-calibration-manifest/v1', manifest_id: 'oceanengine-calibration-current-test-account-2026-08-16' },
    },
    configuration_provenance: { kind: 'manual', generator_ref: 'delivery-plan-editor' },
    fact_provenance: { source: 'mock', snapshot_ref: `mock://delivery-intent/${identity}/${versionNumber}` },
  }
  const dailyBudget = draft.schedule.mode === 'long_term'
    ? draft.budget.totalMinor
    : Math.max(0, Math.floor(draft.budget.totalMinor / Math.max(1, Math.ceil((Date.parse(draft.schedule.endAt) - Date.parse(draft.schedule.startAt)) / 86_400_000))))
  const configuration: PlatformConfiguration = {
    schema_version: 'delivery-platform-configuration/v2', configuration_id: planRevisionIdentity('configuration', identity, versionNumber), version_number: versionNumber,
    platform: 'ocean_engine', profile_version: 'oceanengine-configuration/v1', hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
    payload: {
      profile: 'ocean_engine',
      ocean_engine: {
        profile: 'ocean_engine', calibration_manifest: { schema_version: 'oceanengine-calibration-manifest/v1', manifest_id: 'oceanengine-calibration-current-test-account-2026-08-16' },
        project: {
          draft_schema_version: 'oceanengine-configuration/v1', project_draft_id: `project-${identity}-${versionNumber}`,
          account_reference: { namespace: 'oceanengine', object_kind: 'advertiser_account', scope, id: draft.advertiser.id, state: 'resolved', display_name_snapshot: draft.advertiser.name },
          marketing_purpose: draft.marketingPurpose, marketing_scenario: 'short_video_image_text',
          marketing_product_reference: marketingProductReference,
          carrier: draft.tracking.deliveryCarrier, optimization_target_reference: optimizationTargetReference,
          deep_optimization_mode: 'disabled', delivery_mode: 'manual', placement_strategy: 'automatic',
          targeting: { smart_expansion: false },
          schedule: { mode: draft.schedule.mode, start_at: draft.schedule.startAt, end_at: draft.schedule.endAt, timezone: draft.schedule.timezone },
          budget_and_bidding: { currency: 'CNY', daily_budget_minor: dailyBudget, bidding_strategy: 'stable_cost', charging_mode: 'CPC', bid_minor: 0 },
          search_boost: { keywords: draft.tracking.searchKeywords.split(/[,，]/).map(value => value.trim()).filter(Boolean), bid_coefficient: draft.tracking.searchBidCoefficient, targeting_expansion: draft.tracking.searchTargetingExpansion },
          monitoring_references: monitoringReferences,
          project_name: draft.name,
        },
        promotions: materialReferences.map((reference, index) => ({
          draft_schema_version: 'oceanengine-configuration/v1', promotion_draft_id: `promotion-${identity}-${versionNumber}-${index + 1}`,
          delivery_identity: { mode: 'account_info' }, base_material_references: [reference], copy_items: [],
          product_name: draft.marketingProduct.name,
          landing_page_reference: landingPageReference,
          settings: {}, promotion_name: `${draft.name}-${index + 1}`,
        })),
      },
    },
    configuration_provenance: { kind: 'manual', generator_ref: 'delivery-plan-editor' },
    fact_provenance: { source: 'mock', snapshot_ref: `mock://platform-configuration/${identity}/${versionNumber}` },
    compilation_metadata: { field_evidence: [{ field: 'project', state: 'operator_reviewed' }], steps: ['manual_mapping'], evidence_refs: [] },
  }
  return { intent, platform_configuration: configuration }
}

function planRevisionIdentity(kind: 'intent' | 'configuration', identity: string, versionNumber: number) {
  return `${kind}-${identity}-plan-v${versionNumber}`
}

function toDeliveryPlan(plan: WireDeliveryPlan): DeliveryPlan {
  return {
    id: plan.id,
    organizationId: plan.organization_id,
    projectId: plan.project_id,
    status: plan.status,
    platform: plan.platform,
    source: plan.source,
    scenario: plan.scenario,
    tourRunId: plan.tour_run_id ?? undefined,
    tourOwnerId: plan.tour_owner_id ?? undefined,
    tourCase: plan.tour_case ?? undefined,
    currentVersionNumber: plan.current_version_number,
    currentVersion: toDeliveryPlanVersion(plan.current_version),
    versions: plan.versions.map(toDeliveryPlanVersion),
    createdBy: plan.created_by,
    createdAt: plan.created_at,
    updatedAt: plan.updated_at,
  }
}

function toDeliveryPlanVersion(version: WireDeliveryPlanVersion): DeliveryPlanVersion {
  const intent = version.intent ?? undefined
  const configuration = version.platform_configuration ?? undefined
  const project = configuration?.payload.ocean_engine?.project
  const firstPromotion = configuration?.payload.ocean_engine?.promotions[0]
  const materialReferences = intent?.payload.material_references ?? []
  const typedRuntime = Boolean(intent && configuration)
  const productAudit = project?.marketing_product_reference?.audit_attributes ?? {}
  const optimizationAudit = project?.optimization_target_reference?.audit_attributes ?? {}
  const monitoringValue = (kind: string) => project?.monitoring_references?.find(reference => reference.object_kind === `monitoring_link_${kind}`)?.id ?? ''
  const fallbackAdvertiser = { id: project?.account_reference.id ?? '', name: project?.account_reference.display_name_snapshot ?? '平台账户', platform: 'ocean_engine' as const, source: version.source, scenario: version.scenario }
  return {
    planId: version.plan_id,
    organizationId: version.organization_id,
    projectId: version.project_id,
    versionNumber: version.version_number,
    canonicalHash: version.canonical_hash,
    platform: version.platform,
    schemaVersion: version.schema_version,
    runtimeStatus: version.runtime_status,
    readOnly: version.read_only,
    name: typedRuntime ? project?.project_name ?? '平台投放配置' : version.name ?? '平台投放配置',
    objective: typedRuntime ? intent?.payload.marketing_objective ?? '' : version.objective ?? '',
    marketingPurpose: marketingPurposeValue(typedRuntime ? project?.marketing_purpose : version.marketing_purpose),
    marketingProduct: typedRuntime ? {
      id: project?.marketing_product_reference?.id ?? '', name: project?.marketing_product_reference?.display_name_snapshot ?? '',
      oceanEngineProductId: productAudit.ocean_engine_product_id ?? '',
      activityType: productAudit.activity_type ?? '', activityName: productAudit.activity_name ?? '', brandName: productAudit.brand_name ?? '',
    } : {
      id: version.marketing_product?.id ?? '', name: version.marketing_product?.name ?? '', activityType: version.marketing_product?.activity_type ?? '',
      activityName: version.marketing_product?.activity_name ?? '', brandName: version.marketing_product?.brand_name ?? '',
    },
    advertiser: {
      id: typedRuntime ? fallbackAdvertiser.id : version.advertiser?.id ?? '',
      name: typedRuntime ? fallbackAdvertiser.name : version.advertiser?.name ?? '',
      platform: typedRuntime ? fallbackAdvertiser.platform : version.advertiser?.platform ?? fallbackAdvertiser.platform,
      source: typedRuntime ? fallbackAdvertiser.source : version.advertiser?.source ?? fallbackAdvertiser.source,
      scenario: typedRuntime ? fallbackAdvertiser.scenario : version.advertiser?.scenario ?? fallbackAdvertiser.scenario,
    },
    budget: typedRuntime
      ? { totalMinor: intent?.payload.budget_boundary.maximum_total_minor ?? 0, currency: intent?.payload.budget_boundary.currency ?? 'CNY' }
      : { totalMinor: version.budget?.total_minor ?? 0, currency: version.budget?.currency ?? 'CNY' },
    schedule: typedRuntime
      ? { mode: project?.schedule.mode ?? 'fixed_range', startAt: intent?.payload.schedule_boundary.earliest_start ?? '', endAt: intent?.payload.schedule_boundary.latest_end ?? '', timezone: intent?.payload.schedule_boundary.timezone ?? 'Asia/Shanghai' }
      : { mode: version.schedule?.mode ?? 'fixed_range', startAt: version.schedule?.start_at ?? '', endAt: version.schedule?.end_at ?? '', timezone: version.schedule?.timezone ?? 'Asia/Shanghai' },
    tracking: {
      deliveryCarrier: typedRuntime ? (project?.carrier === 'orange_landing_page' || project?.carrier === 'owned_landing_page' ? project.carrier : '') : version.tracking?.delivery_carrier ?? '',
      landingPage: typedRuntime ? intent?.payload.landing_page_references?.[0]?.id ?? '' : version.tracking?.landing_page ?? '',
      pixelId: typedRuntime ? '' : version.tracking?.pixel_id ?? '',
      conversionEvent: typedRuntime ? firstPromotion?.settings.call_to_action?.[0] ?? '' : version.tracking?.conversion_event ?? '',
      optimizationTargetId: typedRuntime ? project?.optimization_target_reference?.id ?? '' : version.tracking?.optimization_target_id ?? '',
      optimizationTargetName: typedRuntime ? project?.optimization_target_reference?.display_name_snapshot ?? '' : version.tracking?.optimization_target_name ?? '',
      optimizationTargetSemanticKey: typedRuntime ? project?.optimization_target_reference?.semantic_key ?? '' : '',
      eventAssetName: typedRuntime ? optimizationAudit.event_asset_name ?? '' : version.tracking?.event_asset_name ?? '',
      eventAssetType: typedRuntime ? optimizationAudit.event_asset_type ?? '' : version.tracking?.event_asset_type ?? '',
      searchKeywords: typedRuntime ? project?.search_boost?.keywords?.join('，') ?? '' : version.tracking?.search_keywords ?? '',
      searchBidCoefficient: typedRuntime ? project?.search_boost?.bid_coefficient ?? 1.1 : version.tracking?.search_bid_coefficient ?? 1.1,
      searchTargetingExpansion: typedRuntime ? project?.search_boost?.targeting_expansion ?? false : version.tracking?.search_targeting_expansion ?? false,
      monitoringImpression: typedRuntime ? monitoringValue('impression') : version.tracking?.monitoring_impression ?? '',
      monitoringValidTouch: typedRuntime ? monitoringValue('valid_touch') : version.tracking?.monitoring_valid_touch ?? '',
      monitoringVideoPlay: typedRuntime ? monitoringValue('video_play') : version.tracking?.monitoring_video_play ?? '',
      monitoringVideoComplete: typedRuntime ? monitoringValue('video_complete') : version.tracking?.monitoring_video_complete ?? '',
      monitoringValidVideoPlay: typedRuntime ? monitoringValue('valid_video_play') : version.tracking?.monitoring_valid_video_play ?? '',
    },
    creativeReferences: (typedRuntime ? materialReferences.map(reference => ({ asset_id: reference.id ?? '', version: Number(reference.version ?? 1), content_hash: reference.content_hash, route: undefined, confirmed: reference.state === 'resolved', ocean_engine_material_id: reference.audit_attributes?.ocean_engine_material_id })) : version.creative_references ?? []).map(reference => ({
      assetId: reference.asset_id,
      version: reference.version,
      contentHash: reference.content_hash,
      route: reference.route,
      confirmed: reference.confirmed,
      oceanEngineMaterialId: reference.ocean_engine_material_id,
    })),
    strategyReference: !typedRuntime && version.strategy_reference ? {
      taskId: version.strategy_reference.task_id,
      version: version.strategy_reference.version,
      contentHash: version.strategy_reference.content_hash,
      route: version.strategy_reference.route,
    } : {
      // Compatibility for plan versions created before structured upstream
      // references were introduced. New writes always send strategy_reference.
      taskId: intent?.payload.strategy_reference.id ?? version.source_strategy_version ?? '',
      version: Number(intent?.payload.strategy_reference.version ?? version.source_strategy_version?.match(/(\d+)$/)?.[1] ?? 1),
      contentHash: intent?.payload.strategy_reference.content_hash,
    },
    sourceStrategyVersion: typedRuntime ? `${intent?.payload.strategy_reference.id ?? ''}@${intent?.payload.strategy_reference.version ?? '1'}` : version.source_strategy_version ?? '',
    source: version.source,
    scenario: version.scenario,
    createdBy: version.created_by,
    createdAt: version.created_at,
    legacyConfiguration: version.three_tier_configuration ? true : undefined,
    deliveryIntent: intent,
    platformConfiguration: configuration,
  }
}

function marketingPurposeValue(value: unknown): OceanEngineMarketingPurpose | '' {
  return typeof value === 'string' && (oceanEngineMarketingPurposes as readonly string[]).includes(value)
    ? value as OceanEngineMarketingPurpose
    : ''
}

function toDeliveryDecision(value: WireDeliveryDecision): DeliveryDecision {
  return {
    schemaVersion: value.schema_version, id: value.id, organizationId: value.organization_id, projectId: value.project_id, policyVersion: value.policy_version,
    diagnostic: { code: value.diagnostic.code, explanation: value.diagnostic.explanation, nextAction: value.diagnostic.next_action },
    inputs: {
      planId: value.inputs.plan_id, planVersion: value.inputs.plan_version, planCanonicalHash: value.inputs.plan_canonical_hash,
      intentCanonicalHash: value.inputs.intent_canonical_hash, configurationCanonicalHash: value.inputs.configuration_canonical_hash,
      factSnapshotRef: value.inputs.fact_snapshot_ref, simulationRunId: value.inputs.simulation_run_id, simulationInputHash: value.inputs.simulation_input_hash,
    },
    candidates: value.candidates.map(candidate => ({
      id: candidate.id, kind: candidate.kind, targetConfiguration: candidate.target_configuration, budgetChangePercent: candidate.budget_change_percent,
      rationale: candidate.rationale ?? [], constraints: candidate.constraints ?? [], risks: candidate.risks ?? [], uncertainty: candidate.uncertainty, calibrationManifest: candidate.calibration_manifest,
      optimizationFocus: candidate.optimization_focus, proposedAction: candidate.proposed_action, actionMagnitudePercent: candidate.action_magnitude_percent,
      scenario: candidate.scenario, scenarioProbability: candidate.scenario_probability,
    })),
    recommendedCandidateId: value.recommended_candidate_id, evidence: value.evidence ?? [], canonicalHash: value.canonical_hash, createdBy: value.created_by, createdAt: value.created_at,
  }
}

function toDecisionSelection(value: WireDecisionSelection): DeliveryDecisionSelection {
  return {
    id: value.id, decisionId: value.decision_id, decisionCanonicalHash: value.decision_canonical_hash, candidateId: value.candidate_id, configuration: value.configuration,
    workflow: {
      schemaVersion: value.workflow.schema_version, id: value.workflow.id, decisionId: value.workflow.decision_id, decisionCanonicalHash: value.workflow.decision_canonical_hash,
      selectedCandidateId: value.workflow.selected_candidate_id, configurationCanonicalHash: value.workflow.configuration_canonical_hash, compilerVersion: value.workflow.compiler_version,
      configurationId: value.workflow.configuration_id, configurationVersion: value.workflow.configuration_version, platform: value.workflow.platform, profileVersion: value.workflow.profile_version,
      accountReference: value.workflow.account_reference, capabilityContractVersion: value.workflow.capability_contract_version, selectorContractVersion: value.workflow.selector_contract_version, actionContractVersion: value.workflow.action_contract_version, executionDriver: value.workflow.execution_driver, calibrationManifest: value.workflow.calibration_manifest,
      status: value.workflow.status, remoteWriteEnabled: value.workflow.remote_write_enabled,
      steps: value.workflow.steps.map(step => ({ id: step.id, sequence: step.sequence, page: step.page, action: step.action, risk: step.risk, preconditions: step.preconditions ?? [], fields: step.fields.map(field => ({ key: field.key, value: field.value, expectedReadback: field.expected_readback, evidenceRef: field.evidence_ref })), timeoutSeconds: step.timeout_seconds, recovery: step.recovery, blocked: step.blocked, blockReason: step.block_reason })),
      canonicalHash: value.workflow.canonical_hash, createdAt: value.workflow.created_at,
    },
    finalApprovalBinding: {
      status: value.final_approval_binding.status, action: value.final_approval_binding.action, planCanonicalHash: value.final_approval_binding.plan_canonical_hash,
      intentCanonicalHash: value.final_approval_binding.intent_canonical_hash, decisionCanonicalHash: value.final_approval_binding.decision_canonical_hash,
      configurationCanonicalHash: value.final_approval_binding.configuration_canonical_hash, workflowCanonicalHash: value.final_approval_binding.workflow_canonical_hash,
    },
    createdAt: value.created_at,
  }
}

function toObservatoryRun(value: WireObservatoryRun): DeliveryObservatoryRun {
  return {
    schemaVersion: value.schema_version, id: value.id, source: value.source, mode: value.mode, inputHash: value.input_hash,
    binding: { selectionId: value.binding.selection_id, decisionId: value.binding.decision_id, decisionCanonicalHash: value.binding.decision_canonical_hash, configurationCanonicalHash: value.binding.configuration_canonical_hash, workflowId: value.binding.workflow_id, workflowCanonicalHash: value.binding.workflow_canonical_hash },
    dataState: value.data_state, dataStateReason: value.data_state_reason, status: value.status, outcome: value.outcome, remoteWriteEnabled: value.remote_write_enabled,
    steps: value.steps.map(step => ({ stepId: step.step_id, sequence: step.sequence, page: step.page, workflowAction: step.workflow_action, executedAction: step.executed_action, status: step.status, selectorMatches: step.selector_matches ?? [], evidenceRefs: step.evidence_refs ?? [], pageRefs: step.page_refs ?? [], diffs: (step.diffs ?? []).map(diff => ({ key: diff.key, evidenceRef: diff.evidence_ref, expectedValue: diff.expected_value, observedValue: diff.observed_value, matches: diff.matches })), blockReason: step.block_reason })),
    evidenceRefs: value.evidence_refs ?? [], pageRefs: value.page_refs ?? [], canonicalHash: value.canonical_hash, createdAt: value.created_at,
  }
}

function toObservatoryFeedback(value: WireObservatoryFeedback): DeliveryObservatoryFeedback {
  return { schemaVersion: value.schema_version, id: value.id, runId: value.run_id, runCanonicalHash: value.run_canonical_hash, runOutcome: value.run_outcome, disposition: value.disposition, reason: value.reason, diffKeys: value.diff_keys ?? [], finalConfiguration: value.final_configuration, finalConfigurationCanonicalHash: value.final_configuration_canonical_hash, canonicalHash: value.canonical_hash, createdBy: value.created_by, createdAt: value.created_at }
}

function toDeliveryRecommendation(value: WireDeliveryRecommendation): DeliveryRecommendation {
  return {
    id: value.id,
    planId: value.plan_id,
    planVersion: value.plan_version,
    status: value.state ?? value.status ?? 'proposed',
    version: value.version,
    evidenceRefs: value.evidence_refs ?? value.evidence ?? [],
    action: stringValue(value.action),
    impact: stringValue(value.impact),
    risks: (value.risks ?? []).map(stringValue),
    observation: stringValue(value.observation_window ?? value.observation),
    cooldown: value.cooldown_until ?? (value.cooldown === undefined ? undefined : stringValue(value.cooldown)),
    source: value.source ?? 'mock',
    scenario: value.scenario ?? (value.target_configuration ? 'platform_configuration' : 'legacy_unsupported'),
    baseConfiguration: value.base_configuration ?? undefined,
    targetConfiguration: value.target_configuration ?? undefined,
    baseSnapshotHash: value.base_snapshot_hash,
    targetSnapshotHash: value.target_snapshot_hash,
    runtimeStatus: value.runtime_status,
    readOnly: value.read_only,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  }
}

function stringValue(value: unknown) {
  if (typeof value === 'string') return value
  if (value === undefined || value === null) return '未提供'
  return JSON.stringify(value)
}

function toDeliveryControlChangeSet(value: WireDeliveryControlChangeSet): DeliveryControlChangeSet {
  return {
    id: value.id,
    organizationId: value.organization_id,
    projectId: value.project_id,
    planId: value.plan_id,
    planName: value.plan_name,
    planVersion: value.plan_version,
    planCanonicalHash: value.plan_canonical_hash,
    targetSnapshot: value.target_snapshot,
    legacySnapshot: value.legacy_target_snapshot ? true : undefined,
    targetSnapshotHash: value.target_snapshot_hash,
    runtimeStatus: value.runtime_status,
    readOnly: value.read_only,
    recommendationId: value.recommendation_id,
    budgetLimit: {
      totalMinor: value.budget_limit.total_minor,
      currency: value.budget_limit.currency,
    },
    status: value.status,
    riskLevel: value.risk_level,
    preflightNotes: value.preflight_notes ?? [],
    approvedBy: value.approved_by,
    approvedAt: value.approved_at,
    rejectedBy: value.rejected_by,
    rejectedAt: value.rejected_at,
    rejectionReason: value.rejection_reason,
    approval: value.approval ? {
      approvalId: value.approval.approval_id,
      valid: value.approval.valid,
      invalidReason: value.approval.invalid_reason,
      planId: value.approval.plan_id,
      planVersion: value.approval.plan_version,
      changeSetId: value.approval.change_set_id,
      changeSetVersion: value.approval.change_set_version,
      planCanonicalHash: value.approval.plan_canonical_hash,
      targetSnapshotHash: value.approval.target_snapshot_hash,
      configuration: value.approval.configuration_id && value.approval.configuration_version && value.approval.configuration_platform && value.approval.configuration_schema_version && value.approval.configuration_profile_version && value.approval.configuration_canonical_hash ? {
        schemaVersion: value.approval.configuration_schema_version,
        id: value.approval.configuration_id,
        version: value.approval.configuration_version,
        platform: value.approval.configuration_platform,
        profileVersion: value.approval.configuration_profile_version,
        canonicalHash: value.approval.configuration_canonical_hash,
      } : undefined,
      intent: value.approval.intent_id && value.approval.intent_version && value.approval.intent_schema_version && value.approval.intent_canonical_hash ? {
        schemaVersion: value.approval.intent_schema_version,
        id: value.approval.intent_id,
        version: value.approval.intent_version,
        canonicalHash: value.approval.intent_canonical_hash,
      } : undefined,
      actionHash: value.approval.action_hash,
      hashSummary: value.approval.hash_summary,
      action: value.approval.action,
      scope: value.approval.scope,
      budgetLimit: {
        totalMinor: value.approval.budget_limit.total_minor,
        currency: value.approval.budget_limit.currency,
      },
      approvedBy: value.approval.approved_by,
      approvedAt: value.approval.approved_at,
      expiresAt: value.approval.expires_at,
      source: value.approval.source,
      scenario: value.approval.scenario,
    } : undefined,
    source: value.source,
    scenario: value.scenario,
    version: value.version,
    createdBy: value.created_by,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  }
}

function toDeliveryExecutionRecord(value: WireDeliveryExecutionRecord): DeliveryExecutionRecord {
  const execution = value.execution
  return {
    changeSet: toDeliveryControlChangeSet(value.change_set),
    execution: {
      id: execution.id,
      organizationId: execution.organization_id,
      projectId: execution.project_id,
      changeSetId: execution.change_set_id,
      approvalId: execution.approval_id ?? undefined,
      status: execution.status,
      version: execution.version,
      mode: execution.mode,
      adapter: execution.adapter,
      source: execution.source,
      scenario: execution.scenario,
      idempotencyKey: execution.idempotency_key,
      requestHash: execution.request_hash,
      executedBy: execution.executed_by,
      startedAt: execution.started_at,
      completedAt: execution.completed_at ?? undefined,
      retryAllowed: execution.retry_allowed,
      recoveryAction: execution.recovery_action,
      recoveryReason: execution.recovery_reason,
      compensationCandidates: execution.compensation_candidates ?? [],
      steps: (execution.steps ?? []).map(step => ({
        id: step.id,
        sequence: step.sequence,
        action: step.action,
        status: step.status,
        attempt: step.attempt,
        effect: step.effect,
        outcomeSummary: step.outcome_summary,
        evidenceRef: step.evidence_ref ?? undefined,
        startedAt: step.started_at ?? undefined,
        completedAt: step.completed_at ?? undefined,
        version: step.version,
      })),
    },
    evidence: {
      id: value.evidence.id,
      executionId: value.evidence.execution_id,
      summary: value.evidence.summary,
      source: value.evidence.source ?? execution.source,
      scenario: value.evidence.scenario ?? execution.scenario,
      references: value.evidence.references ?? [],
    },
  }
}
