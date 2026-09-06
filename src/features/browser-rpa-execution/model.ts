export type BrowserRpaRunState =
  | 'queued'
  | 'environment_check'
  | 'awaiting_takeover'
  | 'preparing'
  | 'awaiting_confirmation'
  | 'submitting'
  | 'verifying'
  | 'succeeded'
  | 'failed'
  | 'partial'
  | 'result_unknown'
  | 'cancelled'

export type BrowserRpaBlockingReason =
  | 'FINAL_CONFIRMATION_REQUIRED'
  | 'FINAL_CONFIRMATION_INVALID'
  | 'APPROVAL_INVALID'
  | 'LEASE_INVALID'
  | 'KILL_SWITCH_ACTIVE'
  | 'ACCOUNT_MISMATCH'
  | 'PROJECT_NOT_ALLOWED'
  | 'SITE_NOT_ALLOWED'
  | 'PAGE_DRIFT'
  | 'RUNNER_FAILURE'
  | 'WORKFLOW_DRIFT'
  | 'SKILL_DRIFT'
  | 'RESULT_RECONCILIATION_REQUIRED'
  | 'TARGET_EFFECT_NOT_OBSERVED'

export type BrowserRpaAuthorityBinding = {
  schema_version: 'browser-rpa-authority/v1' | 'computer-use-authority/v1'
  authority_origin?: 'plan_execution'
  preflight_canonical_hash?: string
  business_execution_id: string
  change_set_id: string
  approval_id: string
  approval_action_hash: string
  account_reference_id: string
  parent_platform_project_id?: string
  target_mapping_id?: string
  target_mapping_version?: number
  target_platform_object_id?: string
  target_platform_object_kind?: 'promotion'
  operator_principal_id?: string
  promotion_mutation?: {
    current_daily_budget_minor: number
    target_daily_budget_minor: number
    current_materials?: Array<{ reference_id: string; authorization_evidence_id: string }>
    target_materials?: Array<{ reference_id: string; authorization_evidence_id: string }>
    current_state_hash: string
    target_state_hash: string
  }
  promotion_control?: {
    current_daily_budget_minor: number
    current_platform_status: 'delivering'
    target_platform_status: 'paused'
    current_state_hash: string
    target_state_hash: string
  }
  promotion_restart?: {
    current_daily_budget_minor: number
    approved_daily_budget_minor: number
    current_platform_status: 'paused'
    target_platform_status: 'delivering'
    schedule: { start_at: string; end_at: string; timezone: string }
    materials: Array<{ reference_id: string; authorization_evidence_id: string }>
    landing_page: { reference_id: string; authorization_evidence_id: string }
    current_state_hash: string
    target_state_hash: string
  }
  object_fingerprint: string
  action: string
  plan_id?: string
  plan_version?: number
  project_budget_mode?: 'daily' | 'unlimited'
  project_budget_limit_minor?: number
  promotion_budget_limit_minor?: number
  budget_limit_minor: number
  currency: 'CNY'
  plan_canonical_hash: string
  intent_canonical_hash: string
  feedback_canonical_hash: string
  decision_canonical_hash: string
  configuration_canonical_hash: string
  workflow_id: string
  workflow_canonical_hash: string
  workflow_step_id: string
  skill_id?: string
  skill_version?: string
}

export type BrowserRpaRun = {
  schema_version: 'browser-rpa-run/v1' | 'computer-use-run/v1'
  id: string
  organization_id: string
  project_id: string
  platform: 'ocean_engine'
  account_id: string
  execution_driver?: 'oceanengine-web-api/session/v1' | 'playwright-rpa/edge/v3'
  authority: BrowserRpaAuthorityBinding
  environment_id: string
  profile_id: string
  lease_id: string
  policy_id: string
  state: BrowserRpaRunState
  blocking_reason?: BrowserRpaBlockingReason
  paused: boolean
  takeover_active: boolean
  version: number
  idempotency_key: string
  request_hash: string
  created_by: string
  created_at: string
  updated_at: string
}

export type BrowserRpaRunEvent = {
  id: string
  run_id: string
  sequence: number
  kind: string
  summary: string
  actor: string
  created_at: string
}

export type BrowserRpaRunStep = {
  id: string
  run_id: string
  sequence: number
  workflow_step_id: string
  action: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'result_unknown' | 'skipped'
  blocking_reason?: BrowserRpaBlockingReason
  attempt: number
  version: number
}

/** Evidence is redacted by the platform service before the UI receives it. */
export type BrowserRpaEvidence = {
  id: string
  run_id: string
  step_id: string
  diff_keys: string[]
  before_page_facts?: Record<string, string>
  after_page_facts?: Record<string, string>
  field_readback?: Record<string, string>
  page_reference: string
  screenshot_reference?: string
  object_fingerprint: string
  skill_version?: string
  selector_version: string
  action_version: string
  redaction_version: string
  created_at: string
}

export type BrowserRpaEnvironment = {
  id: string
  account_id: string
  mode: 'local_visible'
  browser_version: string
  region: string
  healthy: boolean
  version: number
}

export type BrowserRpaProfile = {
  id: string
  environment_id: string
  account_id: string
  state: 'ready' | 'takeover_required' | 'disabled'
  version: number
}

export type BrowserRpaSitePolicy = {
  id: string
  account_id: string
  allowed_page_kinds: string[]
  allowed_platform_project_ids: string[]
  version: number
}

export type BrowserRpaLease = {
  id: string
  run_id: string
  holder: string
  fencing_token: number
  version: number
  expires_at: string
  heartbeat_deadline: string
  released_at?: string
}

export type EdgeSessionProbe = {
  schema_version: 'browser-rpa-edge-session-probe/v1'
  checked_at: string
  status: 'ready' | 'blocked'
  reason: 'session_ready' | 'cdp_unavailable' | 'oceanengine_page_missing' | 'login_required' | 'account_mismatch'
  cdp_available: boolean
  oceanengine_page_available: boolean
  logged_in: boolean
  account_matched: boolean
}

export type RunnerV3PlanStep = {
  id: string
  kind: string
  page_kind: string
  field_key?: string
  operation?: string
  scope?: string
  target?: string
  value?: unknown
  value_state?: string
  remote_write: boolean
  blocked: boolean
  block_reason?: string
}

export type RunnerV3Plan = {
  schema_version: string
  plan_kind: string
  browser: string
  mode: 'prepare' | 'submit'
  status: string
  account_reference: string
  object_reference?: string
  parent_project_reference?: string
  internal_object_kind?: 'project' | 'promotion'
  internal_object_id?: string
  blocked_reasons: string[]
  configuration_issues?: string[]
  object_availability?: Array<{
    field_key: string
    object_kind: string
    internal_object_id: string
    display_name?: string
    platform_object_id?: string
    available: boolean
    reason?: string
  }>
  steps: RunnerV3PlanStep[]
  allow_remote_write: boolean
  maximum_final_clicks: number
}

export type IssuedFinalConfirmation = {
  confirmation: {
    id: string
    run_id: string
    binding_hash: string
    issued_at: string
    expires_at: string
  }
  token: string
}

export type ControlledExecutionWorkspace = {
  run: BrowserRpaRun
  steps: BrowserRpaRunStep[]
  events: BrowserRpaRunEvent[]
  evidence: BrowserRpaEvidence[]
  environment: BrowserRpaEnvironment
  profile: BrowserRpaProfile
  policy: BrowserRpaSitePolicy
  lease?: BrowserRpaLease
}

export type ControlledExecutionTransportState =
  | { kind: 'loading' }
  | { kind: 'empty' }
  | { kind: 'forbidden'; message: string }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; workspace: ControlledExecutionWorkspace }

export type ControlledExecutionPresentation = {
  kind:
    | 'queued'
    | 'environment_check'
    | 'awaiting_takeover'
    | 'preparing'
    | 'awaiting_confirmation'
    | 'submitting'
    | 'verifying'
    | 'succeeded'
    | 'failed'
    | 'approval_expired'
    | 'confirmation_expired'
    | 'partial'
    | 'result_unknown'
    | 'cancelled'
    | 'kill_switch_active'
    | 'runner_failure'
    | 'page_drift'
    | 'target_effect_not_observed'
    | 'blocked'
  tone: 'neutral' | 'warning' | 'danger' | 'success'
  title: string
  detail: string
  allowsNormalRetry: boolean
}
