import { platformClient } from '../data/platformClient'

export class DeliveryApiError extends Error {
  constructor(readonly code: string | undefined, readonly status: number, message: string) {
    super(message)
    this.name = 'DeliveryApiError'
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
export type StableReference = {
  namespace: string
  object_kind: string
  scope: string
  id?: string
  version?: string
  content_hash?: string
  state: 'resolved' | 'unresolved' | 'blocked' | 'redacted'
  reason?: string
  display_name_snapshot?: string
  evidence_version?: string
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
      project: {
        draft_schema_version: 'oceanengine-configuration/v1'
        project_draft_id: string
        account_reference: StableReference
        marketing_purpose: string
        marketing_scenario: string
        carrier: string
        delivery_mode: string
        targeting: { regions?: string[]; age_ranges?: string[]; gender?: string; smart_expansion: boolean }
        schedule: { start_at: string; end_at: string; timezone: string }
        budget_and_bidding: { currency: 'CNY'; daily_budget_minor: number; bidding_strategy: string; charging_mode: string; bid_minor?: number }
        project_name: string
      }
      promotions: Array<{
        draft_schema_version: 'oceanengine-configuration/v1'
        promotion_draft_id: string
        delivery_identity: { mode: string; authorized_identity?: StableReference }
        base_material_references: StableReference[]
        copy_items: Array<{ text: string }>
        landing_page_reference?: StableReference
        settings: { call_to_action?: string; source_label?: string; comments_enabled?: boolean }
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
    startAt: string
    endAt: string
    timezone: string
  }
  tracking: {
    landingPage: string
    pixelId: string
    conversionEvent: string
  }
  creativeReferences: Array<{
    assetId: string
    version: number
    contentHash?: string
    route?: string
    confirmed: boolean
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
  /** Server-compiled, three-tier mock configuration. */
  threeTierConfiguration?: DeliveryThreeTierConfiguration
  deliveryIntent?: DeliveryIntent
  platformConfiguration?: PlatformConfiguration
}

export type DeliveryFieldValue = string | number | boolean | null | string[]

export type DeliveryThreeTierField = {
  key: string
  label: string
  recommendedValue?: DeliveryFieldValue
  manualValue?: DeliveryFieldValue
  effectiveValue?: DeliveryFieldValue
  valueType: string
  effectiveSource: string
  sourceRefs: string[]
  dependencyRefs: string[]
  riskRefs: string[]
  evidenceRefs: string[]
  mockRequired: boolean
  platformRequired: boolean
  platformStatus: string
  editable: boolean
  confirmation?: { required: boolean; label?: string; confirmed?: boolean }
}

export type DeliveryThreeTierCreative = { id: string; label: string; fields: DeliveryThreeTierField[] }
export type DeliveryThreeTierPlan = { id: string; label: string; fields: DeliveryThreeTierField[]; creatives: DeliveryThreeTierCreative[] }
export type DeliveryThreeTierGroup = { id: string; label: string; fields: DeliveryThreeTierField[]; plans: DeliveryThreeTierPlan[] }

export type DeliveryThreeTierConfiguration = {
  schema: string
  source: DeliverySource
  scenario: string
  generatedAt: string
  evidenceRefs: string[]
  groups: DeliveryThreeTierGroup[]
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
  createdAt?: string
  updatedAt?: string
}

export type ManualActionPackage = {
  id: string
  changeSetId: string
  planId: string
  source: DeliverySource
  scenario: string
  generatedAt: string
  optimizedPlanVersion: number
  optimizedPlanHash: string
  configuration?: { schemaVersion: string; id: string; version: number; platform: DeliveryPlatform; profileVersion: string; canonicalHash: string }
  intent?: { schemaVersion: string; id: string; version: number; canonicalHash: string }
  instructions: Array<{ fieldKey: string; effectiveValue: DeliveryFieldValue; source: string; confirmationRequired: boolean; expectedResult: string; evidenceRefs: string[] }>
  forbiddenActions: string[]
  evidenceRefs: string[]
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

export type DeliveryPreflightCheck = {
  code: 'advertiser_available' | 'budget_positive' | 'schedule_valid' | 'creative_present' | 'creative_confirmed' | 'tracking_complete' | 'upstream_references_resolved' | 'three_tier_structure' | 'three_tier_required_fields' | 'three_tier_dependencies' | 'three_tier_confirmation' | 'three_tier_platform_pending' | 'delivery_intent_valid' | 'platform_configuration_valid' | 'INVALID_STABLE_REFERENCE' | 'CANONICAL_HASH_MISMATCH' | 'CAPABILITY_PENDING' | 'platform_pending' | 'blocked_by_event_asset' | 'write_validation_pending'
  severity: 'error' | 'warning'
  passed: boolean
  message: string
  repair?: {
    field: string
    section: string
    label: string
  }
}

export type DeliveryPreflightResult = {
  planId: string
  planVersion: number
  passed: boolean
  blocked: boolean
  checks: DeliveryPreflightCheck[]
  source: DeliverySource
  scenario: DeliveryScenario
  checkedAt: string
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
  legacyTargetSnapshot?: DeliveryThreeTierConfiguration
  targetSnapshotHash?: string
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
  advertiser: { id: string; name: string; platform: 'ocean_engine'; source?: DeliverySource; scenario?: DeliveryScenario }
  budget: { total_minor: number; currency: 'CNY' }
  schedule: { start_at: string; end_at: string; timezone: string }
  tracking: { landing_page: string; pixel_id: string; conversion_event: string }
  creative_references: Array<{ asset_id: string; version: number; content_hash?: string; route?: string; confirmed: boolean }>
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
  three_tier_configuration?: WireDeliveryThreeTierConfiguration | null
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

type WireDeliveryThreeTierField = {
  key: string
  label?: string
  recommended: { type: string; value: DeliveryFieldValue }
  manual?: { type: string; value: DeliveryFieldValue } | null
  effective: { type: string; value: DeliveryFieldValue }
  source: string
  effective_source?: string
  source_refs?: string[] | null
  dependency?: string
  dependency_refs?: string[] | null
  risk?: string
  risk_refs?: string[] | null
  evidence_refs: string[]
  mock_required: boolean
  platform_required: boolean
  platform_status: string
  editable: boolean
  confirmation: boolean
}

type WireDeliveryThreeTierCreative = { id: string; name: string; fields: WireDeliveryThreeTierField[] }
type WireDeliveryThreeTierPlan = { id: string; name: string; fields?: WireDeliveryThreeTierField[] | null; creatives: WireDeliveryThreeTierCreative[] }
type WireDeliveryThreeTierGroup = { id: string; name: string; fields?: WireDeliveryThreeTierField[] | null; plans: WireDeliveryThreeTierPlan[] }
type WireDeliveryThreeTierConfiguration = {
  schema?: string
  contract_version?: string
  source: DeliverySource
  scenario: string
  fixture_scenario?: string
  generated_at?: string
  evidence?: string[] | null
  evidence_refs?: string[] | null
  groups: WireDeliveryThreeTierGroup[]
}

type WireDeliveryRecommendation = {
  id: string
  plan_id: string
  plan_version: number
  target_snapshot?: WireDeliveryThreeTierConfiguration | null
  base_configuration?: PlatformConfiguration | null
  target_configuration?: PlatformConfiguration | null
  base_snapshot_hash?: string
  target_snapshot_hash?: string
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

type WireManualActionPackage = {
  id: string
  change_set_id: string
  instructions?: Array<{ field_key: string; effective: { type: string; value: DeliveryFieldValue }; source: string; confirmation_required: boolean; expected_result: string; evidence_refs?: string[] | null }> | null
  layers?: Array<{ fields: Array<{ field_key: string; value: { type: string; value: DeliveryFieldValue }; source: string; confirmation: { required: boolean; confirmed: boolean }; expected_result: string; evidence_refs?: string[] | null }> }> | null
  source?: DeliverySource
  scenario?: string
  evidence?: string[] | null
  evidence_refs?: string[] | null
  forbidden_actions?: string[] | null
  created_at: string
  optimized_plan_version: number
  optimized_plan_hash: string
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

type WirePreflightResult = {
  plan_id: string
  plan_version: number
  passed: boolean
  blocked: boolean
  checks: Array<{
    code: DeliveryPreflightCheck['code']
    severity: DeliveryPreflightCheck['severity']
    passed: boolean
    message: string
    repair: DeliveryPreflightCheck['repair'] | null
  }>
  source: DeliverySource
  scenario: DeliveryScenario
  checked_at: string
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
  legacy_target_snapshot?: WireDeliveryThreeTierConfiguration
  target_snapshot_hash?: string
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
  async preflight(projectId: string, planId: string): Promise<DeliveryPreflightResult> {
    const response = await deliveryPlanRequest<WirePreflightResult>(projectId, `/plans/${encodeURIComponent(planId)}/preflight`, { method: 'POST' })
    return {
      planId: response.plan_id,
      planVersion: response.plan_version,
      passed: response.passed,
      blocked: response.blocked,
      checks: response.checks.map(check => ({ ...check, repair: check.repair ?? undefined })),
      source: response.source,
      scenario: response.scenario,
      checkedAt: response.checked_at,
    }
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
  async createChangeSet(projectId: string, planId: string, expectedVersion: number): Promise<DeliveryControlChangeSet> {
    return toDeliveryControlChangeSet(await deliveryPlanRequest<WireDeliveryControlChangeSet>(
      projectId,
      `/plans/${encodeURIComponent(planId)}:create-change-set`,
      { method: 'POST', body: JSON.stringify({ expected_version: expectedVersion }) },
    ))
  },
  async preflightChangeSet(projectId: string, changeSetId: string, expectedVersion: number): Promise<DeliveryControlChangeSet> {
    return deliveryChangeSetAction(projectId, changeSetId, 'preflight', expectedVersion)
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

/** Three-tier configuration compilation and recommendation lifecycle; all records remain mock-only. */
export const deliveryConfigurationApi = {
  async compile(_projectId: string, _planId: string, _expectedVersion: number, _fixture: string): Promise<DeliveryPlan> {
    throw new DeliveryApiError('LEGACY_CONFIGURATION_UNSUPPORTED', 409, '旧版 ThreeTier 配置仅支持只读访问。')
  },
  async override(_projectId: string, _planId: string, _input: {
    expectedVersion: number
    groupId: string
    planId: string
    creativeId: string
    fieldKey: string
    value: { type: string; value: DeliveryFieldValue }
    confirmed: boolean
  }): Promise<DeliveryPlan> {
    throw new DeliveryApiError('LEGACY_CONFIGURATION_UNSUPPORTED', 409, '旧版 ThreeTier 配置仅支持只读访问。')
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
  async compileManualActionPackage(projectId: string, changeSetId: string, expectedVersion: number): Promise<ManualActionPackage> {
    return toManualActionPackage(await deliveryPlanRequest<WireManualActionPackage>(
      projectId,
      `/change-sets/${encodeURIComponent(changeSetId)}/manual-action-package`,
      { method: 'POST', body: JSON.stringify({ expected_version: expectedVersion }) },
    ))
  },
  async getManualActionPackage(projectId: string, changeSetId: string): Promise<ManualActionPackage> {
    return toManualActionPackage(await deliveryPlanRequest<WireManualActionPackage>(
      projectId,
      `/change-sets/${encodeURIComponent(changeSetId)}/manual-action-package`,
    ))
  },
}

export const deliveryExecutionApi = {
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
  monitoredEntity: { type: 'delivery_plan'; id: string; advertiserId: string }
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
  source: 'demo_fixture'
  isSimulated: true
  scenario: DeliveryAlertFixture
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
  monitored_entity: { type: 'delivery_plan'; id: string; advertiser_id: string }
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
  source: 'demo_fixture'
  is_simulated: true
  scenario: DeliveryAlertFixture
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

export type DeliveryTourRun = {
  id: string
  organizationId: string
  projectId: string
  ownerId: string
  status: 'preparing' | 'prepared' | 'reset'
  source: DeliverySource
  scenario: 'delivery_tour'
  preparedAt?: string
  resetAt?: string
  createdAt: string
  updatedAt: string
  cases: DeliveryTourCase[]
  steps: DeliveryTourStep[]
  currentStep: string
  suggestedNextUrl: string
}

export type DeliveryTourResetResult = {
  run: DeliveryTourRun
  deleted: Record<string, number>
  source: DeliverySource
  scenario: 'delivery_tour_reset'
  resetAt: string
  isolationKey: string
}

type WireDeliveryTourCase = {
  key: DeliveryTourCaseKey
  title: string
  plan_id: string
  status: DeliveryTourCase['status']
  expected_outcome: string
  start_url: string
  source: DeliverySource
  scenario: DeliveryTourCaseKey
  evidence?: string[] | null
  observed_at: string
}

type WireDeliveryTourRun = {
  id: string
  organization_id: string
  project_id: string
  owner_id: string
  status: DeliveryTourRun['status']
  source: DeliverySource
  scenario: 'delivery_tour'
  prepared_at?: string | null
  reset_at?: string | null
  created_at: string
  updated_at: string
  cases?: WireDeliveryTourCase[] | null
  steps?: Array<{
    key: string
    title: string
    completion_condition: string
    complete: boolean
    url: string
    explanation: string
    evidence?: string[] | null
  }> | null
  current_step: string
  suggested_next_url: string
}

type WireDeliveryTourResetResult = {
  run: WireDeliveryTourRun
  deleted?: Record<string, number> | null
  source: DeliverySource
  scenario: 'delivery_tour_reset'
  reset_at: string
  isolation_key: string
}

export const deliveryTourApi = {
  async prepare(projectId: string, runId: string): Promise<DeliveryTourRun> {
    return toDeliveryTourRun(await deliveryPlanRequest<WireDeliveryTourRun>(projectId, `/tour-runs/${encodeURIComponent(runId)}:prepare`, { method: 'POST' }))
  },
  async get(projectId: string, runId: string): Promise<DeliveryTourRun> {
    return toDeliveryTourRun(await deliveryPlanRequest<WireDeliveryTourRun>(projectId, `/tour-runs/${encodeURIComponent(runId)}`))
  },
  async reset(projectId: string, runId: string): Promise<DeliveryTourResetResult> {
    const value = await deliveryPlanRequest<WireDeliveryTourResetResult>(projectId, `/tour-runs/${encodeURIComponent(runId)}:reset`, { method: 'POST' })
    return {
      run: toDeliveryTourRun(value.run),
      deleted: value.deleted ?? {},
      source: value.source,
      scenario: value.scenario,
      resetAt: value.reset_at,
      isolationKey: value.isolation_key,
    }
  },
}

function toDeliveryTourRun(value: WireDeliveryTourRun): DeliveryTourRun {
  return {
    id: value.id,
    organizationId: value.organization_id,
    projectId: value.project_id,
    ownerId: value.owner_id,
    status: value.status,
    source: value.source,
    scenario: value.scenario,
    preparedAt: value.prepared_at ?? undefined,
    resetAt: value.reset_at ?? undefined,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
    cases: (value.cases ?? []).map(tourCase => ({
      key: tourCase.key,
      title: tourCase.title,
      planId: tourCase.plan_id,
      status: tourCase.status,
      expectedOutcome: tourCase.expected_outcome,
      startUrl: tourCase.start_url,
      source: tourCase.source,
      scenario: tourCase.scenario,
      evidence: tourCase.evidence ?? [],
      observedAt: tourCase.observed_at,
    })),
    steps: (value.steps ?? []).map(step => ({
      key: step.key,
      title: step.title,
      completionCondition: step.completion_condition,
      complete: step.complete,
      url: step.url,
      explanation: step.explanation,
      evidence: step.evidence ?? [],
    })),
    currentStep: value.current_step,
    suggestedNextUrl: value.suggested_next_url,
  }
}

async function deliveryPlanRequest<T>(projectId: string, path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body !== undefined) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api/delivery/v1/projects/${encodeURIComponent(projectId)}${path}`, { credentials: 'include', ...init, headers })
  const payload = await response.json() as T | { error?: { code?: string; message?: string } }
  if (!response.ok) {
    const problem = payload as { error?: { code?: string; message?: string } }
    throw new DeliveryApiError(problem.error?.code, response.status, problem.error?.code === 'VERSION_CONFLICT'
      ? '计划已被其他版本更新，请刷新后再试。'
      : problem.error?.message ?? 'Delivery API 请求失败')
  }
  return payload as T
}

function toWireDraft(draft: DeliveryPlanDraft): WireDeliveryPlanDraft {
  return {
    name: draft.name,
    objective: draft.objective,
    advertiser: draft.advertiser,
    budget: { total_minor: draft.budget.totalMinor, currency: draft.budget.currency },
    schedule: { start_at: draft.schedule.startAt, end_at: draft.schedule.endAt, timezone: draft.schedule.timezone },
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
  const materialReferences: StableReference[] = draft.creativeReferences.map(reference => ({
    namespace: 'cookies', object_kind: 'asset_version', scope,
    id: reference.assetId, version: String(reference.version), content_hash: reference.contentHash,
    state: 'resolved', display_name_snapshot: reference.assetId,
  }))
  const strategyReference: StableReference = {
    namespace: 'cookies', object_kind: 'strategy_version', scope,
    id: draft.strategyReference.taskId, version: String(draft.strategyReference.version), content_hash: draft.strategyReference.contentHash,
    state: 'resolved', display_name_snapshot: draft.sourceStrategyVersion,
  }
  const intent: DeliveryIntent = {
    schema_version: 'delivery-intent/v1', intent_id: `intent-${identity}`, version_number: versionNumber,
    hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
    payload: {
      payload_schema_version: 'delivery-intent/v1', marketing_objective: draft.objective,
      budget_boundary: { currency: 'CNY', minimum_total_minor: 0, maximum_total_minor: draft.budget.totalMinor },
      schedule_boundary: { earliest_start: draft.schedule.startAt, latest_end: draft.schedule.endAt, timezone: draft.schedule.timezone },
      optimization_preferences: [], material_references: materialReferences,
      landing_page_references: [{ namespace: 'cookies', object_kind: 'landing_page', scope, id: draft.tracking.landingPage, state: 'resolved' }],
      audience_constraints: { constraints: [] }, strategy_reference: strategyReference,
    },
    configuration_provenance: { kind: 'manual', generator_ref: 'delivery-plan-editor' },
    fact_provenance: { source: 'mock', snapshot_ref: `mock://delivery-intent/${identity}/${versionNumber}` },
  }
  const dailyBudget = Math.max(0, Math.floor(draft.budget.totalMinor / Math.max(1, Math.ceil((Date.parse(draft.schedule.endAt) - Date.parse(draft.schedule.startAt)) / 86_400_000))))
  const configuration: PlatformConfiguration = {
    schema_version: 'delivery-platform-configuration/v2', configuration_id: `configuration-${identity}`, version_number: versionNumber,
    platform: 'ocean_engine', profile_version: 'oceanengine-configuration/v1', hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
    payload: {
      profile: 'ocean_engine',
      ocean_engine: {
        profile: 'ocean_engine',
        project: {
          draft_schema_version: 'oceanengine-configuration/v1', project_draft_id: `project-${identity}-${versionNumber}`,
          account_reference: { namespace: 'oceanengine', object_kind: 'advertiser_account', scope, id: draft.advertiser.id, state: 'resolved', display_name_snapshot: draft.advertiser.name },
          marketing_purpose: draft.objective, marketing_scenario: 'manual_delivery', carrier: 'landing_page', delivery_mode: 'manual',
          targeting: { smart_expansion: false },
          schedule: { start_at: draft.schedule.startAt, end_at: draft.schedule.endAt, timezone: draft.schedule.timezone },
          budget_and_bidding: { currency: 'CNY', daily_budget_minor: dailyBudget, bidding_strategy: 'manual_bid', charging_mode: 'CPC', bid_minor: 0 },
          project_name: draft.name,
        },
        promotions: materialReferences.map((reference, index) => ({
          draft_schema_version: 'oceanengine-configuration/v1', promotion_draft_id: `promotion-${identity}-${index + 1}`,
          delivery_identity: { mode: 'account_info' }, base_material_references: [reference], copy_items: [],
          landing_page_reference: { namespace: 'cookies', object_kind: 'landing_page', scope, id: draft.tracking.landingPage, state: 'resolved' },
          settings: { call_to_action: draft.tracking.conversionEvent }, promotion_name: `${draft.name}-${index + 1}`,
        })),
      },
    },
    configuration_provenance: { kind: 'manual', generator_ref: 'delivery-plan-editor' },
    fact_provenance: { source: 'mock', snapshot_ref: `mock://platform-configuration/${identity}/${versionNumber}` },
    compilation_metadata: { field_evidence: [{ field: 'project', state: 'operator_reviewed' }], steps: ['manual_mapping'], evidence_refs: [] },
  }
  return { intent, platform_configuration: configuration }
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
      ? { startAt: intent?.payload.schedule_boundary.earliest_start ?? '', endAt: intent?.payload.schedule_boundary.latest_end ?? '', timezone: intent?.payload.schedule_boundary.timezone ?? 'Asia/Shanghai' }
      : { startAt: version.schedule?.start_at ?? '', endAt: version.schedule?.end_at ?? '', timezone: version.schedule?.timezone ?? 'Asia/Shanghai' },
    tracking: {
      landingPage: typedRuntime ? intent?.payload.landing_page_references?.[0]?.id ?? '' : version.tracking?.landing_page ?? '',
      pixelId: typedRuntime ? '' : version.tracking?.pixel_id ?? '',
      conversionEvent: typedRuntime ? firstPromotion?.settings.call_to_action ?? '' : version.tracking?.conversion_event ?? '',
    },
    creativeReferences: (typedRuntime ? materialReferences.map(reference => ({ asset_id: reference.id ?? '', version: Number(reference.version ?? 1), content_hash: reference.content_hash, route: undefined, confirmed: reference.state === 'resolved' })) : version.creative_references ?? []).map(reference => ({
      assetId: reference.asset_id,
      version: reference.version,
      contentHash: reference.content_hash,
      route: reference.route,
      confirmed: reference.confirmed,
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
    threeTierConfiguration: version.three_tier_configuration ? toThreeTierConfiguration(version.three_tier_configuration, version.created_at) : undefined,
    deliveryIntent: intent,
    platformConfiguration: configuration,
  }
}

function toThreeTierField(value: WireDeliveryThreeTierField): DeliveryThreeTierField {
  return {
    key: value.key,
    label: value.label ?? '未标注字段',
    recommendedValue: value.recommended.value,
    manualValue: value.manual?.value,
    effectiveValue: value.effective.value,
    valueType: value.effective.type,
    effectiveSource: value.effective_source ?? value.source,
    sourceRefs: value.source_refs ?? [],
    dependencyRefs: value.dependency_refs ?? (value.dependency ? [value.dependency] : []),
    riskRefs: value.risk_refs ?? (value.risk ? [value.risk] : []),
    evidenceRefs: value.evidence_refs ?? [],
    mockRequired: value.mock_required,
    platformRequired: value.platform_required,
    platformStatus: value.platform_status,
    editable: value.editable,
    confirmation: { required: !value.confirmation, confirmed: value.confirmation },
  }
}

function toThreeTierConfiguration(value: WireDeliveryThreeTierConfiguration, versionCreatedAt: string): DeliveryThreeTierConfiguration {
  return {
    schema: value.schema ?? value.contract_version ?? 'delivery-three-tier/v1',
    source: value.source,
    scenario: value.fixture_scenario ?? value.scenario,
    generatedAt: value.generated_at ?? versionCreatedAt,
    evidenceRefs: value.evidence_refs ?? value.evidence ?? [],
    groups: value.groups.map(group => ({
      id: group.id,
      label: group.name,
      fields: (group.fields ?? []).map(toThreeTierField),
      plans: group.plans.map(plan => ({
        id: plan.id,
        label: plan.name,
        fields: (plan.fields ?? []).map(toThreeTierField),
        creatives: plan.creatives.map(creative => ({
          id: creative.id,
          label: creative.name,
          fields: creative.fields.map(toThreeTierField),
        })),
      })),
    })),
  }
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
    source: value.source ?? value.target_snapshot?.source ?? 'mock',
    scenario: value.scenario ?? value.target_snapshot?.scenario ?? (value.target_configuration ? 'platform_configuration' : 'golden_path'),
    baseConfiguration: value.base_configuration ?? undefined,
    targetConfiguration: value.target_configuration ?? undefined,
    baseSnapshotHash: value.base_snapshot_hash,
    targetSnapshotHash: value.target_snapshot_hash,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  }
}

function stringValue(value: unknown) {
  if (typeof value === 'string') return value
  if (value === undefined || value === null) return '未提供'
  return JSON.stringify(value)
}

function toManualActionPackage(value: WireManualActionPackage): ManualActionPackage {
  const layerInstructions = (value.layers ?? []).flatMap(layer => layer.fields.map(field => ({
    fieldKey: field.field_key,
    effectiveValue: field.value.value,
    source: field.source,
    confirmationRequired: field.confirmation.required && !field.confirmation.confirmed,
    expectedResult: field.expected_result,
    evidenceRefs: field.evidence_refs ?? [],
  })))
  return {
    id: value.id,
    changeSetId: value.change_set_id,
    planId: '',
    source: value.source ?? 'mock',
    scenario: value.scenario ?? 'manual_action_package',
    generatedAt: value.created_at,
    optimizedPlanVersion: value.optimized_plan_version,
    optimizedPlanHash: value.optimized_plan_hash,
    configuration: value.configuration_id && value.configuration_version && value.configuration_platform && value.configuration_schema_version && value.configuration_profile_version && value.configuration_canonical_hash ? {
      schemaVersion: value.configuration_schema_version, id: value.configuration_id, version: value.configuration_version,
      platform: value.configuration_platform, profileVersion: value.configuration_profile_version, canonicalHash: value.configuration_canonical_hash,
    } : undefined,
    intent: value.intent_id && value.intent_version && value.intent_schema_version && value.intent_canonical_hash ? {
      schemaVersion: value.intent_schema_version, id: value.intent_id, version: value.intent_version, canonicalHash: value.intent_canonical_hash,
    } : undefined,
    instructions: layerInstructions.length ? layerInstructions : (value.instructions ?? []).map(instruction => ({
      fieldKey: instruction.field_key,
      effectiveValue: instruction.effective.value,
      source: instruction.source,
      confirmationRequired: instruction.confirmation_required,
      expectedResult: instruction.expected_result,
      evidenceRefs: instruction.evidence_refs ?? [],
    })),
    forbiddenActions: value.forbidden_actions ?? [],
    evidenceRefs: value.evidence_refs ?? value.evidence ?? [],
  }
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
    legacyTargetSnapshot: value.legacy_target_snapshot ? toThreeTierConfiguration(value.legacy_target_snapshot, value.created_at) : undefined,
    targetSnapshotHash: value.target_snapshot_hash,
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
