export type ProductionRunSource = 'provider' | 'creative_render' | 'editing_render' | 'audio_render'
export type ProductionMediaKind = 'image' | 'video' | 'audio' | 'render'
export type ProductionStatus = 'queued' | 'running' | 'ingesting' | 'succeeded' | 'partially_succeeded' | 'failed' | 'expired' | 'cancelled'

export type AssetVersionRef = { asset_id: string; version: number }
export type ProductionRunRef = { source: ProductionRunSource; id: string }
export type ProductionSourceTask = { system: 'creative'; object_type: 'creative_task' | 'edit_task'; object_id: string; display_name: string | null }
export type ProductionNativeStatus = {
  family: 'provider' | ProductionRunSource
  execution_status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | null
  provider_status: 'submitted' | 'running' | 'outputs_ready' | 'ingesting' | 'succeeded' | 'partially_succeeded' | 'failed' | 'cancelled' | 'expired' | null
  render_status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | null
}
export type ProductionModel = { logical_alias: string; actual_model: string | null; degraded: boolean }
export type ProductionCost = { availability: 'actual' | 'estimated' | 'unavailable'; currency: string; amount_minor: number | null; unavailable_reason: string | null }
export type ProductionError = { code: string; message: string; retryable: boolean }
export type ProductionActions = { retry: boolean; open_source: boolean }
export type ProductionSourceHealth = { source: ProductionRunSource | 'assets'; status: 'available' | 'unavailable'; message: string | null }

export type ProductionRunSummary = {
  ref: ProductionRunRef
  project_id: string
  media_kind: ProductionMediaKind
  operation_kind: string
  source_task: ProductionSourceTask | null
  normalized_status: ProductionStatus
  native_status: ProductionNativeStatus
  progress_percent: number
  model: ProductionModel | null
  output_count: number
  cost: ProductionCost | null
  error: ProductionError | null
  actions: ProductionActions
  created_at: string
  updated_at: string
}

export type ProductionRunPage = {
  contract_version: 'creative-production-run-page/v1'
  project_id: string
  items: ProductionRunSummary[]
  next_cursor: string | null
  source_health: ProductionSourceHealth[]
}

export type ProductionAsset = {
  asset_ref: AssetVersionRef
  role: 'input' | 'output'
  media_kind: Exclude<ProductionMediaKind, 'render'>
  display_name: string | null
  availability: 'processing' | 'ready' | 'quarantined' | 'failed' | 'archived'
  mime_type: string | null
  width_pixels: number | null
  height_pixels: number | null
  duration_ms: number | null
  preview_url: string | null
  preview_expires_at: string | null
}

export type ProductionRunDetail = {
  contract_version: 'creative-production-run-detail/v1'
  summary: ProductionRunSummary
  input_assets: ProductionAsset[]
  output_assets: ProductionAsset[]
  parameters: Record<string, string | number>
  prompt_ref: { type: string; id: string; version?: number } | null
  attempt: { attempt_count: number; max_attempts: number; retry_of: ProductionRunRef | null }
  retry_chain: ProductionRunRef[]
  run_events: Array<{ ordinal: number; stage: string; safe_message: string; error_code: string | null; occurred_at: string }>
  lineage: { source_task: ProductionSourceTask | null; input_asset_refs: AssetVersionRef[]; output_asset_refs: AssetVersionRef[] }
  source_health: ProductionSourceHealth[]
}

export type ProductionAssetItem = { asset: ProductionAsset; used_by_runs: ProductionRunRef[] }
export type ProductionAssetPage = {
  contract_version: 'creative-production-asset-page/v1'
  project_id: string
  items: ProductionAssetItem[]
  next_cursor: string | null
  source_health: ProductionSourceHealth[]
}

export type RetryResult = {
  contract_version: 'creative-production-retry/v1'
  status: 'accepted'
  previous_run: ProductionRunRef
  new_run: ProductionRunRef
  source_task: ProductionSourceTask | null
}

export type ProductionRunQuery = {
  media_kind?: ProductionMediaKind
  status?: ProductionStatus[]
  source_task_id?: string
  created_after?: string
  created_before?: string
  q?: string
  cursor?: string
  limit?: number
}

export type ProductionAssetQuery = {
  role?: 'input' | 'output'
  media_kind?: Exclude<ProductionMediaKind, 'render'>
  run_source?: ProductionRunSource
  cursor?: string
  limit?: number
}
