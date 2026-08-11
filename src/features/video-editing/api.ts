const viteEnv = (import.meta.env ?? {}) as Record<string, string | undefined>
const backendOrigin = viteEnv.VITE_API_BASE_URL ?? ''

export type EditingAssetRef = { asset_id: string; version: number }

export type EditingTimelineV1 = {
  schema_version: 'editing-timeline/v1'
  output_profile: { id: 'cookies-editing-vertical-v1'; width: 720; height: 1280; frame_rate: 30; sample_rate: 48000 }
  duration_ms: number
  tracks: Array<{
    id: string
    role: 'primary_video' | 'caption' | 'voiceover' | 'music' | 'sfx'
    clips: Array<{
      id: string
      asset_ref?: EditingAssetRef
      timeline_start_ms: number
      timeline_end_ms: number
      source_in_ms?: number
      source_out_ms?: number
      text?: string
      gain_db?: number
      loop?: boolean
    }>
  }>
}

export type EditingTimelineV2 = {
  schema_version: 'editing-timeline/v2'
  timebase: { frame_rate_num: 30; frame_rate_den: 1 }
  canvas: { profile_id: string; width: number; height: number; sample_rate: 48000; background: { type: 'color'; value: string } }
  duration_frames: number
  tracks: Array<{
    id: string
    kind: 'visual' | 'caption' | 'audio'
	role?: 'primary' | 'overlay' | 'voiceover' | 'music' | 'sfx'
	z_index?: number
	muted?: boolean
	locked?: boolean
	hidden?: boolean
	language?: string
    clips: Array<{
      id: string
      kind: 'video' | 'image' | 'caption' | 'audio'
      asset_ref?: EditingAssetRef
      timeline: { start_frame: number; duration_frames: number }
		source?: { in_us: number; out_us: number }
		transform?: { fit: 'contain' | 'cover'; position_x: number; position_y: number; scale: number; crop: { left: number; top: number; right: number; bottom: number }; opacity: number }
      text?: string
	  style_ref?: { style_id: string; version: number }
	  emphasis?: Array<{ start_rune: number; end_rune: number }>
	  original_audio?: { enabled: boolean; gain_db: number; fade_in_frames: number; fade_out_frames: number }
	  gain_db?: number
	  fade_in_frames?: number
	  fade_out_frames?: number
	  loop?: boolean
    }>
  }>
}

export type EditingTimeline = EditingTimelineV1 | EditingTimelineV2

export type ApiTimelineVersion = {
  version: number
  schema_version: 'editing-timeline/v1' | 'editing-timeline/v2'
  timeline: EditingTimeline
  parent_version?: number
  change_summary?: string
  operation_batch_id?: string
  compiler_compatibility: string
  content_hash: string
  created_at: string
}

export type ApiEditTask = {
  id: string
  display_name: string
  status: 'draft' | 'rendering' | 'review_ready' | 'completed' | 'failed' | 'archived'
  entry_source: 'manual' | 'short_drama_preroll_v2' | 'creative_version'
  source_creative_task_id?: string
  current_timeline: ApiTimelineVersion | null
  created_at: string
  updated_at: string
}

export type ApiEditingRenderJob = {
  id: string
  edit_task_id: string
  timeline: { version: number; content_hash: string }
  kind: 'preview' | 'export'
	renderer_fingerprint: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'
  progress_percent: number
  output_asset?: { asset_version: EditingAssetRef }
  error_code?: string
  error_message?: string
}

export type ApiEditingCreativeVersion = {
	id: string
	edit_task_id: string
	status: 'created' | 'checked' | 'approved' | 'superseded'
	content_hash: string
	check?: { passed: boolean; blockers: string[]; warnings: string[] }
	video_snapshot: { final_video: EditingAssetRef; editing: { timeline_version: number; timeline_schema: string; timeline_hash: string; compiler_version: string; renderer_fingerprint: string; render_job_id: string; output_asset: EditingAssetRef; input_assets: EditingAssetRef[]; width: number; height: number; frame_rate: number; sample_rate: number; duration_ms: number; video_codec: string; audio_codec: string; target_lufs: number } }
}

export type ApiEditingCreativePackage = { id: string; creative_version_id: string; edit_task_id: string; content_hash: string; created_at: string }

export class EditingApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message)
    this.name = 'EditingApiError'
  }
}

async function editingRequest<T>(path: string, method = 'GET', body?: unknown, extraHeaders?: Record<string, string>): Promise<T> {
  const response = await fetch(`${backendOrigin}/api/creative/v1${path}`, {
    method,
    credentials: 'include',
    headers: body === undefined && !extraHeaders ? undefined : { ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...extraHeaders },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let payload: T | { error?: { message?: string } } = {}
  try { payload = text ? JSON.parse(text) as T | { error?: { message?: string } } : {} } catch { throw new EditingApiError(`素材剪辑服务返回了无效响应（HTTP ${response.status}）`, response.status) }
  if (!response.ok) throw new EditingApiError((payload as { error?: { message?: string } }).error?.message ?? `素材剪辑请求失败（HTTP ${response.status}）`, response.status)
  return payload as T
}

export const editingApi = {
  create: (projectId: string, input: { display_name: string; timeline?: EditingTimeline }) => editingRequest<ApiEditTask>(`/projects/${encodeURIComponent(projectId)}/edit-tasks`, 'POST', input),
  list: (projectId: string, status?: ApiEditTask['status']) => editingRequest<{ items: ApiEditTask[] }>(`/projects/${encodeURIComponent(projectId)}/edit-tasks${status ? `?status=${encodeURIComponent(status)}` : ''}`),
  get: (projectId: string, editTaskId: string) => editingRequest<ApiEditTask>(`/projects/${encodeURIComponent(projectId)}/edit-tasks/${encodeURIComponent(editTaskId)}`),
  listTimelineVersions: (projectId: string, editTaskId: string) => editingRequest<{ items: ApiTimelineVersion[] }>(`/projects/${encodeURIComponent(projectId)}/edit-tasks/${encodeURIComponent(editTaskId)}/timeline-versions`),
  saveTimeline: (projectId: string, editTaskId: string, expectedVersion: number, timeline: EditingTimeline) => editingRequest<ApiEditTask>(`/projects/${encodeURIComponent(projectId)}/edit-tasks/${encodeURIComponent(editTaskId)}/timeline`, 'PATCH', { expected_version: expectedVersion, timeline }),
  applyOperations: (projectId: string, editTaskId: string, batch: import('./operations').EditingOperationBatch) => editingRequest<ApiEditTask>(`/projects/${encodeURIComponent(projectId)}/edit-tasks/${encodeURIComponent(editTaskId)}/operations:batch`, 'POST', batch),
  openShortDramaV2: (projectId: string, creativeTaskId: string) => editingRequest<ApiEditTask>(`/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(creativeTaskId)}/short-drama-preroll-v2:open-editor`, 'POST'),
  openCreativeVersion: (projectId: string, creativeTaskId: string) => editingRequest<ApiEditTask>(`/projects/${encodeURIComponent(projectId)}/creative-tasks/${encodeURIComponent(creativeTaskId)}/open-editor`, 'POST'),
  createRender: (projectId: string, editTaskId: string, kind: 'preview' | 'export') => editingRequest<ApiEditingRenderJob>(`/projects/${encodeURIComponent(projectId)}/edit-tasks/${encodeURIComponent(editTaskId)}/renders`, 'POST', { kind }),
  getRender: (projectId: string, renderJobId: string) => editingRequest<ApiEditingRenderJob>(`/projects/${encodeURIComponent(projectId)}/edit-renders/${encodeURIComponent(renderJobId)}`),
  cancelRender: (projectId: string, renderJobId: string) => editingRequest<ApiEditingRenderJob>(`/projects/${encodeURIComponent(projectId)}/edit-renders/${encodeURIComponent(renderJobId)}/cancel`, 'POST'),
  retryRender: (projectId: string, renderJobId: string) => editingRequest<ApiEditingRenderJob>(`/projects/${encodeURIComponent(projectId)}/edit-renders/${encodeURIComponent(renderJobId)}/retry`, 'POST'),
	submitVersion: (projectId: string, editTaskId: string, renderJobId: string, timelineVersion: number) => editingRequest<ApiEditingCreativeVersion>(`/projects/${encodeURIComponent(projectId)}/edit-tasks/${encodeURIComponent(editTaskId)}/versions:submit`, 'POST', { render_job_id: renderJobId, expected_timeline_version: timelineVersion }, { 'Idempotency-Key': `editing-submit-${editTaskId}-${timelineVersion}-${renderJobId}` }),
	listVersions: (projectId: string, editTaskId: string) => editingRequest<{ items: ApiEditingCreativeVersion[] }>(`/projects/${encodeURIComponent(projectId)}/creative-versions?task_id=${encodeURIComponent(editTaskId)}`),
	checkVersion: (projectId: string, versionId: string) => editingRequest<ApiEditingCreativeVersion>(`/projects/${encodeURIComponent(projectId)}/creative-versions/${encodeURIComponent(versionId)}:check`, 'POST'),
	approveVersion: (projectId: string, versionId: string) => editingRequest<ApiEditingCreativeVersion>(`/projects/${encodeURIComponent(projectId)}/creative-versions/${encodeURIComponent(versionId)}:approve`, 'POST'),
	deliverVersion: (projectId: string, versionId: string) => editingRequest<ApiEditingCreativePackage>(`/projects/${encodeURIComponent(projectId)}/creative-versions/${encodeURIComponent(versionId)}:deliver`, 'POST'),
}
