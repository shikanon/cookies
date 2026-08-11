import { useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { Check, ChevronRight, CircleAlert, Clapperboard, Clock3, Film, Image as ImageIcon, LoaderCircle, Play, Plus, RefreshCw, Save, Sparkles, Upload, WandSparkles, X, ZoomIn } from 'lucide-react'
import { useProject } from '../../context/ProjectContext'
import { api, CreativeApiError, type ApiCreativeTaskSummary, type ApiProjectMediaAsset, type ApiShortDramaV2TaskDetail } from '../../data/api'
import { editingApi } from '../video-editing/api'
import { canOpenShortDramaStep, initialShortDramaPrerollState, shortDramaPrerollReducer } from './reducer'
import { createAsyncActionGate } from './asyncActionGate'
import { findAuthoritativeVideo, requireAuthoritativeVideo, sourceUnavailableMessage } from './sourceAuthority'
import type { FirstFrameCandidate, PrerollDuration, ShortDramaPrerollState, ShortDramaStep } from './types'
import './short-drama-preroll-v2.css'

const steps: Array<{ id: ShortDramaStep; index: string; label: string; detail: string }> = [
  { id: 'understanding', index: '01', label: '素材理解', detail: '识别剧情与开场信息' },
  { id: 'direction', index: '02', label: '前贴方向', detail: '人工选择钩子方向' },
  { id: 'first-frame', index: '03', label: '首帧参考', detail: '生成并选择首帧图' },
  { id: 'video', index: '04', label: '视频生成', detail: '确认参数并生成前贴' },
]

const prerollDurations: readonly PrerollDuration[] = [10, 12, 15]

const wait = (ms: number) => new Promise(resolve => window.setTimeout(resolve, ms))

async function waitForProviderJob(projectId: string, jobId: string) {
  for (let attempt = 0; attempt < 180; attempt += 1) {
    const job = await api.getShortDramaV2ProviderJob(projectId, jobId)
    if (job.status === 'succeeded' || job.status === 'failed' || job.status === 'cancelled') return job
    await wait(2000)
  }
  throw new Error('模型任务等待超时，请稍后重试。')
}

function formatDuration(seconds?: number) {
  if (!seconds) return '时长待识别'
  const minutes = Math.floor(seconds / 60)
  return `${minutes}:${String(Math.round(seconds % 60)).padStart(2, '0')}`
}

// Keep the historical key so in-progress V2 tasks survive the V3 UI migration.
function storageKey(projectId: string) { return `cookies.short-drama-preroll-v2:${projectId}` }
function taskStorageKey(projectId: string, taskId: string) { return `${storageKey(projectId)}:task:${taskId}` }

type ShortDramaSession = {
  taskId: string
  activeStep?: ShortDramaStep
  summaryDraft?: string
  imagePrompt?: string
  videoDescription?: string
  videoPrompt?: string
  duration?: PrerollDuration
}

function readSession(projectId: string): ShortDramaSession | null {
  try {
    const raw = window.localStorage.getItem(storageKey(projectId))
    if (!raw) return null
    const parsed = JSON.parse(raw) as ShortDramaSession & { version?: number }
    return (parsed.version === 2 || parsed.version === 3 || parsed.version === 4) && parsed.taskId ? parsed : null
  } catch { return null }
}

function readTaskSession(projectId: string, taskId: string): ShortDramaSession | null {
  try {
    const raw = window.localStorage.getItem(taskStorageKey(projectId, taskId))
    if (!raw) return null
    const parsed = JSON.parse(raw) as ShortDramaSession & { version?: number }
    return parsed.taskId === taskId ? parsed : null
  } catch { return null }
}

function isShortDramaPrerollTask(task: ApiCreativeTaskSummary) {
  return task.format === 'video' && task.performance_mode === 'short_drama_preroll' && task.status !== 'archived'
}

function sameAsset(left?: { asset_version: { asset_id: string; version: number } }, right?: { asset_version: { asset_id: string; version: number } }) {
  return Boolean(left && right && left.asset_version.asset_id === right.asset_version.asset_id && left.asset_version.version === right.asset_version.version)
}

async function restoreState(projectId: string, detail: ApiShortDramaV2TaskDetail, source: ApiProjectMediaAsset, session: ShortDramaSession | null): Promise<ShortDramaPrerollState> {
  const workspace = detail.video_draft.short_drama_preroll_v2
  const analysisReady = workspace.analysis.status === 'ready'
  const hooks = hookDirections(detail)
  const prompt = workspace.prompt_draft
  const selectedDirectionId = workspace.direction_batch?.selected_direction_id ?? ''
  const readyCandidates = workspace.first_frame_batch?.candidates.filter(candidate => candidate.status === 'ready' && (candidate.output_canvas_asset || candidate.asset)) ?? []
  const images = await Promise.all(readyCandidates.map(async candidate => ({
    id: candidate.id,
    label: candidate.style_profile || `参考图 ${candidate.variant_index}`,
    imageUrl: await api.getProjectAssetPreview(projectId, (candidate.output_canvas_asset || candidate.asset)!.asset_version),
    composition: candidate.visual_mechanism || `构图方案 ${candidate.variant_index}`,
    variantKey: candidate.variant_key,
    visualMechanism: candidate.visual_mechanism,
    styleProfile: candidate.style_profile,
  })))
  const selectedCandidate = workspace.first_frame_batch?.candidates.find(candidate =>
    sameAsset(candidate.output_canvas_asset, workspace.first_frame_batch?.selected_output_asset)
      || sameAsset(candidate.model_canvas_asset || candidate.asset, workspace.first_frame_batch?.selected_asset),
  )
  const output = workspace.output_asset ? {
    id: workspace.output_asset.asset_version.asset_id,
    videoUrl: await api.getProjectAssetPreview(projectId, workspace.output_asset.asset_version),
    duration: (prompt?.duration_seconds ?? 10) as PrerollDuration,
    createdAt: new Date().toISOString(),
  } : null
  let activeStep: ShortDramaStep = 'understanding'
  if (analysisReady) activeStep = 'direction'
  if (selectedDirectionId && prompt) activeStep = 'first-frame'
  if (selectedCandidate || workspace.active_stage === 'video_generating' || workspace.active_stage === 'completed') activeStep = 'video'
  let restored: ShortDramaPrerollState = {
    ...initialShortDramaPrerollState,
    source,
    activeStep,
    analysisStatus: analysisReady ? 'ready' : workspace.analysis.status === 'failed' ? 'error' : 'idle',
    analysis: analysisReady ? storyAnalysis(detail) : null,
    summaryDraft: analysisReady ? workspace.analysis.content.synopsis : '',
    hooksStatus: hooks.length ? 'ready' : 'idle',
    hooks,
    selectedHookId: selectedDirectionId,
    duration: (prompt?.duration_seconds ?? 10) as PrerollDuration,
    imagePrompt: prompt?.image_prompt ?? '',
    videoDescription: prompt?.video_description ?? '',
    videoPrompt: prompt?.video_prompt ?? '',
    imagesStatus: images.length ? 'ready' : workspace.first_frame_batch && ['queued', 'running'].includes(workspace.first_frame_batch.status) ? 'loading' : workspace.first_frame_batch?.status === 'failed' ? 'error' : 'idle',
    images,
    selectedImageId: selectedCandidate?.id ?? '',
    videoStatus: output ? 'ready' : workspace.active_stage === 'video_generating' ? 'loading' : workspace.video_error ? 'error' : 'idle',
    output,
    error: workspace.video_error?.message ?? '',
  }
  if (session?.taskId === detail.task.id) {
    restored = {
      ...restored,
      summaryDraft: session.summaryDraft ?? restored.summaryDraft,
      imagePrompt: session.imagePrompt ?? restored.imagePrompt,
      videoDescription: session.videoDescription ?? restored.videoDescription,
      videoPrompt: session.videoPrompt ?? restored.videoPrompt,
      duration: session.duration ?? restored.duration,
    }
    if (session.activeStep && canOpenShortDramaStep(restored, session.activeStep)) restored.activeStep = session.activeStep
  }
  return restored
}

async function resumeWorkspaceJobs(projectId: string, detail: ApiShortDramaV2TaskDetail): Promise<{ detail: ApiShortDramaV2TaskDetail; error?: string }> {
  let current = detail
  const batch = current.video_draft.short_drama_preroll_v2.first_frame_batch
  const pendingFrames = batch?.candidates.filter(candidate => candidate.provider_job_id && ['queued', 'running'].includes(candidate.status)) ?? []
  if (pendingFrames.length) {
    await Promise.all(pendingFrames.map(candidate => waitForProviderJob(projectId, candidate.provider_job_id!)))
    for (const candidate of pendingFrames) {
      current = await api.reconcileShortDramaV2FirstFrame(projectId, current.task.id, current.video_draft.revision, candidate.id, candidate.provider_job_id!)
    }
  }
  const workspace = current.video_draft.short_drama_preroll_v2
  if (workspace.active_stage === 'video_generating' && workspace.latest_video_attempt_id) {
    const job = await waitForProviderJob(projectId, workspace.latest_video_attempt_id)
    current = await api.reconcileShortDramaV2Video(projectId, current.task.id, current.video_draft.revision, workspace.latest_video_attempt_id)
    if (job.status !== 'succeeded') return { detail: current, error: job.diagnostic || '前贴视频生成失败。' }
  }
  return { detail: current }
}

function storyAnalysis(detail: ApiShortDramaV2TaskDetail) {
  const content = detail.video_draft.short_drama_preroll_v2.analysis.content
  return {
    title: content.title,
    episode: content.episode ?? '',
    synopsis: content.synopsis,
    openingBeat: content.opening_beat,
    characters: content.characters.map(item => `${item.name}｜${item.description}`),
    visualKeywords: content.visual_keywords,
  }
}

function hookDirections(detail: ApiShortDramaV2TaskDetail) {
  return (detail.video_draft.short_drama_preroll_v2.direction_batch?.items ?? []).map((item, index) => ({
    id: item.id,
    category: item.category,
    eyebrow: `${item.category === 'curiosity' ? '猎奇吸睛' : '剧情总结'} ${String(index % 2 + 1).padStart(2, '0')}`,
    title: item.title,
    description: item.description,
    hookCopy: item.hook_copy,
    rationale: item.rationale,
  }))
}

function isRecoverableWorkspaceConflict(cause: unknown) {
  return cause instanceof CreativeApiError && (
    cause.status === 412
    || cause.code === 'CREATIVE_VERSION_CONFLICT'
    || (cause.status === 409 && cause.code === 'INVALID_STATE')
  )
}

export function ShortDramaPrerollWorkspace({ onNotice, onOpenEditTask }: { onNotice: (message: string) => void; onOpenEditTask?: (editTaskId: string) => void }) {
  const { currentProject } = useProject()
  const [state, dispatch] = useReducer(shortDramaPrerollReducer, initialShortDramaPrerollState)
  const [assets, setAssets] = useState<ApiProjectMediaAsset[]>([])
  const [workspace, setWorkspace] = useState<ApiShortDramaV2TaskDetail | null>(null)
  const [savedWorks, setSavedWorks] = useState<ApiCreativeTaskSummary[]>([])
  const [switchingWork, setSwitchingWork] = useState(false)
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [saveName, setSaveName] = useState('')
  const [savingWork, setSavingWork] = useState(false)
  const [mediaLoading, setMediaLoading] = useState(true)
  const [uploading, setUploading] = useState(false)
  const [localPreviewUrl, setLocalPreviewUrl] = useState('')
  const [directionSelectionGate] = useState(createAsyncActionGate)
  const [firstFrameGenerationGate] = useState(createAsyncActionGate)
  const [firstFrameSelectionGate] = useState(createAsyncActionGate)
  const hydratedProject = useRef('')
  const fileInput = useRef<HTMLInputElement>(null)

  const refreshSavedWorks = async () => {
    const result = await api.listCreativeTasks(currentProject.id, 100)
    setSavedWorks(result.items.filter(isShortDramaPrerollTask).sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at)))
  }

  useEffect(() => {
    void refreshSavedWorks().catch(() => setSavedWorks([]))
  }, [currentProject.id])

  useEffect(() => {
    let cancelled = false
    setMediaLoading(true)
    void api.listProjectMediaAssets(currentProject.id).then(async result => {
      if (cancelled) return
      const videos = result.filter(asset => asset.kind === 'video')
      setAssets(videos)
      const session = readSession(currentProject.id)
      if (session) {
        try {
          const restored = await api.getShortDramaPrerollV2Workspace(currentProject.id, session.taskId)
          if (cancelled) return
          const sourceRef = restored.video_draft.short_drama_preroll_v2.source_video.asset_version
          const source = findAuthoritativeVideo(videos, sourceRef)
          if (!source) throw new Error(sourceUnavailableMessage)
          const restoredState = await restoreState(currentProject.id, restored, source, session)
          if (cancelled) return
          setWorkspace(restored)
          dispatch({ type: 'restore', state: restoredState })
          const restoredWorkspace = restored.video_draft.short_drama_preroll_v2
          const hasPendingFrames = restoredWorkspace.first_frame_batch?.candidates.some(candidate => candidate.provider_job_id && ['queued', 'running'].includes(candidate.status))
          if (hasPendingFrames || restoredWorkspace.active_stage === 'video_generating') {
            void resumeWorkspaceJobs(currentProject.id, restored).then(async resumed => {
              if (cancelled) return
              const resumedState = await restoreState(currentProject.id, resumed.detail, source, session)
              if (cancelled) return
              setWorkspace(resumed.detail)
              dispatch({ type: 'restore', state: resumedState })
              if (resumed.error) dispatch({ type: 'operation-failed', message: resumed.error })
            }).catch(cause => {
              if (!cancelled) dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '恢复生成任务失败' })
            })
          }
        } catch (cause) {
          window.localStorage.removeItem(storageKey(currentProject.id))
          setWorkspace(null)
          dispatch({
            type: 'restore',
            state: {
              ...initialShortDramaPrerollState,
              error: cause instanceof Error && cause.message === sourceUnavailableMessage
                ? sourceUnavailableMessage
                : '短剧前贴草稿无法恢复，请重新选择源视频。',
            },
          })
        }
      } else if (videos[0]) dispatch({ type: 'source-selected', source: videos[0] })
      hydratedProject.current = currentProject.id
    }).catch(() => {
      if (!cancelled) onNotice('项目视频素材暂时无法读取，也可以从本地选择视频继续搭建流程。')
    }).finally(() => { if (!cancelled) setMediaLoading(false) })
    return () => { cancelled = true }
  }, [currentProject.id, onNotice])

  useEffect(() => () => {
    if (localPreviewUrl) URL.revokeObjectURL(localPreviewUrl)
  }, [localPreviewUrl])

  useEffect(() => {
    if (hydratedProject.current !== currentProject.id || !workspace) return
    const session = JSON.stringify({
      version: 4,
      taskId: workspace.task.id,
      activeStep: state.activeStep,
      summaryDraft: state.summaryDraft,
      imagePrompt: state.imagePrompt,
      videoDescription: state.videoDescription,
      videoPrompt: state.videoPrompt,
      duration: state.duration,
    })
    window.localStorage.setItem(storageKey(currentProject.id), session)
    window.localStorage.setItem(taskStorageKey(currentProject.id, workspace.task.id), session)
  }, [currentProject.id, state.activeStep, state.duration, state.imagePrompt, state.summaryDraft, state.videoDescription, state.videoPrompt, workspace])

  const selectedHook = useMemo(() => state.hooks.find(item => item.id === state.selectedHookId) ?? null, [state.hooks, state.selectedHookId])
  const selectedImage = useMemo(() => state.images.find(item => item.id === state.selectedImageId) ?? null, [state.images, state.selectedImageId])
  const sourceUrl = localPreviewUrl || state.source?.contentUrl || ''
  const outputCanvas = workspace?.video_draft.short_drama_preroll_v2.output_canvas
  const outputAspectLabel = outputCanvas ? `${outputCanvas.aspect_num}:${outputCanvas.aspect_den}` : (state.source?.width && state.source?.height ? `${state.source.width}:${state.source.height}` : '跟随源视频')

  const beginFreshWorkspace = () => {
    if (localPreviewUrl) URL.revokeObjectURL(localPreviewUrl)
    setLocalPreviewUrl('')
    setWorkspace(null)
    window.localStorage.removeItem(storageKey(currentProject.id))
    dispatch({ type: 'restore', state: initialShortDramaPrerollState })
  }

  const requestNewWorkspace = () => {
    if (!workspace) {
      beginFreshWorkspace()
      return
    }
    setSaveName(workspace.task.display_name || state.analysis?.title || '未命名短剧前贴')
    setShowSaveDialog(true)
  }

  const saveAndCreateWorkspace = async () => {
    const name = saveName.trim()
    if (!workspace || !name || savingWork) return
    setSavingWork(true)
    try {
      await api.renameCreativeTask(currentProject.id, workspace.task.id, workspace.task.version, name)
      window.localStorage.setItem(taskStorageKey(currentProject.id, workspace.task.id), JSON.stringify({
        version: 4,
        taskId: workspace.task.id,
        activeStep: state.activeStep,
        summaryDraft: state.summaryDraft,
        imagePrompt: state.imagePrompt,
        videoDescription: state.videoDescription,
        videoPrompt: state.videoPrompt,
        duration: state.duration,
      }))
      await refreshSavedWorks()
      setShowSaveDialog(false)
      beginFreshWorkspace()
      onNotice(`已保存“${name}”，可从作品下拉框继续编辑。`)
    } catch (cause) {
      dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '保存短剧前贴失败' })
      setShowSaveDialog(false)
    } finally {
      setSavingWork(false)
    }
  }

  const openSavedWorkspace = async (taskId: string) => {
    if (!taskId || taskId === workspace?.task.id || switchingWork) return
    setSwitchingWork(true)
    try {
      const [detail, videos] = await Promise.all([
        api.getShortDramaPrerollV2Workspace(currentProject.id, taskId),
        api.listProjectMediaAssets(currentProject.id).then(items => items.filter(item => item.kind === 'video')),
      ])
      const sourceRef = detail.video_draft.short_drama_preroll_v2.source_video.asset_version
      const source = requireAuthoritativeVideo(videos, sourceRef)
      const restored = await restoreState(currentProject.id, detail, source, readTaskSession(currentProject.id, taskId))
      setAssets(videos)
      setWorkspace(detail)
      dispatch({ type: 'restore', state: restored })
      onNotice(`已恢复“${detail.task.display_name || '未命名短剧前贴'}”。`)
    } catch (cause) {
      dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '恢复短剧前贴失败' })
    } finally {
      setSwitchingWork(false)
    }
  }

  const selectSource = (source: ApiProjectMediaAsset) => {
    if (localPreviewUrl) URL.revokeObjectURL(localPreviewUrl)
    setLocalPreviewUrl('')
    setWorkspace(null)
    window.localStorage.removeItem(storageKey(currentProject.id))
    dispatch({ type: 'source-selected', source })
  }
  const selectLocalFile = async (file?: File) => {
    if (!file) return
    if (localPreviewUrl) URL.revokeObjectURL(localPreviewUrl)
    const url = URL.createObjectURL(file)
    setLocalPreviewUrl(url)
    setUploading(true)
    try {
      const ref = await api.uploadProjectAsset(currentProject.id, file)
      await api.getProjectAssetPreview(currentProject.id, ref)
      const videos = (await api.listProjectMediaAssets(currentProject.id)).filter(item => item.kind === 'video')
      setAssets(videos)
      const uploaded = requireAuthoritativeVideo(videos, ref)
      setWorkspace(null)
      window.localStorage.removeItem(storageKey(currentProject.id))
      dispatch({ type: 'source-selected', source: uploaded })
      URL.revokeObjectURL(url)
      setLocalPreviewUrl('')
      onNotice('视频已上传为项目素材，可以开始真实内容理解。')
    } catch (cause) {
      setLocalPreviewUrl('')
      dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '视频上传失败' })
    } finally {
      setUploading(false)
    }
  }

  const analyze = async () => {
    if (!state.source) return
    dispatch({ type: 'analysis-started' })
    try {
      const videos = (await api.listProjectMediaAssets(currentProject.id)).filter(item => item.kind === 'video')
      const source = requireAuthoritativeVideo(videos, { asset_id: state.source.id, version: state.source.version })
      setAssets(videos)
      let current = workspace
      const sourceRef = current?.video_draft.short_drama_preroll_v2.source_video.asset_version
      if (!current || sourceRef?.asset_id !== source.id || sourceRef.version !== source.version) {
        current = await api.createManualShortDramaPrerollV2Workspace(currentProject.id, { asset_id: source.id, version: source.version })
      }
      const analyzed = await api.analyzeShortDramaV2Source(currentProject.id, current.task.id, current.video_draft.revision)
      setWorkspace(analyzed)
      void refreshSavedWorks()
      dispatch({ type: 'analysis-ready', analysis: storyAnalysis(analyzed) })
      onNotice('已根据当前上传视频完成真实内容理解。')
    } catch (cause) {
      if (cause instanceof Error && cause.message.includes('后端尚未确认该视频素材')) {
        setWorkspace(null)
        window.localStorage.removeItem(storageKey(currentProject.id))
        dispatch({ type: 'restore', state: { ...initialShortDramaPrerollState, error: sourceUnavailableMessage } })
        return
      }
      dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '视频理解失败' })
    }
  }
  const generateHooks = async () => {
    if (!workspace) return
    dispatch({ type: 'hooks-started' })
    try {
      let current = workspace
      const content = current.video_draft.short_drama_preroll_v2.analysis.content
      if (state.summaryDraft.trim() !== content.synopsis.trim()) {
        current = await api.updateShortDramaV2Analysis(currentProject.id, current.task.id, current.video_draft.revision, { ...content, synopsis: state.summaryDraft.trim() })
      }
      const planned = await api.generateShortDramaV2Directions(currentProject.id, current.task.id, current.video_draft.revision)
      setWorkspace(planned)
      dispatch({ type: 'hooks-ready', hooks: hookDirections(planned) })
    } catch (cause) {
      dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '前贴方向生成失败' })
    }
  }
  const selectHook = async (id: string, duration = state.duration) => {
    const batch = workspace?.video_draft.short_drama_preroll_v2.direction_batch
    if (!workspace || !batch || directionSelectionGate.isActive()) return
    dispatch({ type: 'hook-selection-started', id })
    await directionSelectionGate.run(async () => {
      try {
        const selected = await api.selectShortDramaV2Direction(currentProject.id, workspace.task.id, workspace.video_draft.revision, batch.id, id, duration)
        const prompt = selected.video_draft.short_drama_preroll_v2.prompt_draft
        if (!prompt) throw new Error('服务端没有返回前贴提示词。')
        setWorkspace(selected)
        dispatch({
          type: 'hook-selected', id, duration: prompt.duration_seconds,
          imagePrompt: prompt.image_prompt, videoDescription: prompt.video_description, videoPrompt: prompt.video_prompt,
        })
      } catch (cause) {
        if (cause instanceof CreativeApiError && (cause.status === 412 || cause.code === 'CREATIVE_VERSION_CONFLICT')) {
          try {
            const latest = await api.getShortDramaPrerollV2Workspace(currentProject.id, workspace.task.id)
            const latestWorkspace = latest.video_draft.short_drama_preroll_v2
            if (!state.source) throw new Error(sourceUnavailableMessage)
            const restored = await restoreState(currentProject.id, latest, state.source, readSession(currentProject.id))
            setWorkspace(latest)
            dispatch({ type: 'restore', state: restored })
            if (latestWorkspace.direction_batch?.selected_direction_id === id && latestWorkspace.prompt_draft) {
              onNotice('前贴方向已选择，页面已同步到最新草稿。')
              return
            }
          } catch {
            // Fall through to the actionable message below.
          }
          dispatch({ type: 'hook-selection-failed', message: '草稿刚刚发生了更新，已刷新到最新状态，请重新选择一次前贴方向。' })
          return
        }
        dispatch({ type: 'hook-selection-failed', message: cause instanceof Error ? cause.message : '选择前贴方向失败' })
      }
    })
  }
  const changeDuration = (duration: PrerollDuration) => {
    dispatch({ type: 'duration-changed', duration })
  }
  const synchronizeWorkspace = async () => {
    if (!workspace || !state.source) throw new Error(sourceUnavailableMessage)
    const latest = await api.getShortDramaPrerollV2Workspace(currentProject.id, workspace.task.id)
    const resumed = await resumeWorkspaceJobs(currentProject.id, latest)
    const restored = await restoreState(currentProject.id, resumed.detail, state.source, readSession(currentProject.id))
    setWorkspace(resumed.detail)
    dispatch({ type: 'restore', state: restored })
    if (resumed.error) throw new Error(resumed.error)
    return resumed.detail
  }
  const generateImages = async () => {
    if (!workspace || firstFrameGenerationGate.isActive()) return
    dispatch({ type: 'images-started' })
    await firstFrameGenerationGate.run(async () => {
      try {
        let current = workspace
        const prompt = current.video_draft.short_drama_preroll_v2.prompt_draft
        if (!prompt) throw new Error('请先选择一个前贴方向。')
        if (prompt.image_prompt !== state.imagePrompt || prompt.video_description !== state.videoDescription || prompt.video_prompt !== state.videoPrompt) {
          current = await api.updateShortDramaV2Prompts(currentProject.id, current.task.id, current.video_draft.revision, state.imagePrompt, state.videoDescription, state.videoPrompt)
          setWorkspace(current)
        }
        current = await api.generateShortDramaV2FirstFrames(currentProject.id, current.task.id, current.video_draft.revision)
        setWorkspace(current)
        const batch = current.video_draft.short_drama_preroll_v2.first_frame_batch
        if (!batch) throw new Error('服务端没有创建首帧候选任务。')
        await Promise.all(batch.candidates.map(candidate => candidate.provider_job_id ? waitForProviderJob(currentProject.id, candidate.provider_job_id) : Promise.resolve()))
        for (const candidate of batch.candidates) {
          if (!candidate.provider_job_id) continue
          current = await api.reconcileShortDramaV2FirstFrame(currentProject.id, current.task.id, current.video_draft.revision, candidate.id, candidate.provider_job_id)
        }
        const reconciled = current.video_draft.short_drama_preroll_v2.first_frame_batch
        const ready = reconciled?.candidates.filter(candidate => candidate.status === 'ready' && (candidate.output_canvas_asset || candidate.asset)) ?? []
        const images = await Promise.all(ready.map(async candidate => ({
          id: candidate.id,
          label: candidate.style_profile || `参考图 ${candidate.variant_index}`,
          imageUrl: await api.getProjectAssetPreview(currentProject.id, (candidate.output_canvas_asset || candidate.asset)!.asset_version),
          composition: candidate.visual_mechanism || `构图方案 ${candidate.variant_index}`,
          variantKey: candidate.variant_key,
          visualMechanism: candidate.visual_mechanism,
          styleProfile: candidate.style_profile,
        })))
        if (!images.length) throw new Error('3 张首帧参考图均未生成成功。')
        setWorkspace(current)
        dispatch({ type: 'images-ready', images })
      } catch (cause) {
        if (isRecoverableWorkspaceConflict(cause)) {
          try {
            await synchronizeWorkspace()
            onNotice('检测到已有首帧任务，已自动同步最新生成结果。')
            return
          } catch {
            dispatch({ type: 'operation-failed', message: '首帧任务状态刚刚发生变化，请稍后再点击“重新生成”。' })
            return
          }
        }
        dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '首帧参考图生成失败' })
      }
    })
  }
  const selectImage = async (id: string) => {
    const batch = workspace?.video_draft.short_drama_preroll_v2.first_frame_batch
    if (!workspace || !batch || firstFrameSelectionGate.isActive()) return
    dispatch({ type: 'image-selection-started', id })
    await firstFrameSelectionGate.run(async () => {
      try {
        const selected = await api.selectShortDramaV2FirstFrame(currentProject.id, workspace.task.id, workspace.video_draft.revision, batch.id, id)
        const prompt = selected.video_draft.short_drama_preroll_v2.prompt_draft
        setWorkspace(selected)
        dispatch({ type: 'image-selected', id, videoPrompt: prompt?.video_prompt })
      } catch (cause) {
        if (isRecoverableWorkspaceConflict(cause)) {
          try {
            const latest = await synchronizeWorkspace()
            const latestBatch = latest.video_draft.short_drama_preroll_v2.first_frame_batch
            const latestSelectedCandidate = latestBatch?.candidates.find(candidate =>
              sameAsset(candidate.output_canvas_asset, latestBatch.selected_output_asset)
                || sameAsset(candidate.model_canvas_asset || candidate.asset, latestBatch.selected_asset),
            )
            if (latestSelectedCandidate?.id === id) return
            dispatch({ type: 'image-selection-failed', message: '候选批次已更新，页面已同步到最新结果，请重新选择一张首帧。' })
            return
          } catch {
            dispatch({ type: 'image-selection-failed', message: '首帧任务状态刚刚发生变化，请刷新后重新选择。' })
            return
          }
        }
        dispatch({ type: 'image-selection-failed', message: cause instanceof Error ? cause.message : '选择首帧失败' })
      }
    })
  }
  const generateVideo = async () => {
    if (!workspace) return
    dispatch({ type: 'video-started' })
    try {
      let current = workspace
      const prompt = current.video_draft.short_drama_preroll_v2.prompt_draft
      if (!prompt) throw new Error('前贴提示词尚未生成。')
      if (prompt.video_description !== state.videoDescription || prompt.video_prompt !== state.videoPrompt) {
        current = await api.updateShortDramaV2Prompts(currentProject.id, current.task.id, current.video_draft.revision, state.imagePrompt, state.videoDescription, state.videoPrompt)
      }
      current = await api.generateShortDramaV2Video(currentProject.id, current.task.id, current.video_draft.revision)
      const jobId = current.video_draft.short_drama_preroll_v2.latest_video_attempt_id
      if (!jobId) throw new Error('服务端没有返回视频生成任务。')
      const job = await waitForProviderJob(currentProject.id, jobId)
      current = await api.reconcileShortDramaV2Video(currentProject.id, current.task.id, current.video_draft.revision, jobId)
      setWorkspace(current)
      if (job.status !== 'succeeded') throw new Error(job.diagnostic || '前贴视频生成失败。')
      const output = current.video_draft.short_drama_preroll_v2.output_asset
      if (!output) throw new Error('视频任务成功，但没有生成可预览的项目资产。')
      const videoUrl = await api.getProjectAssetPreview(currentProject.id, output.asset_version)
      setWorkspace(current)
      dispatch({ type: 'video-ready', output: { id: output.asset_version.asset_id, videoUrl, duration: state.duration, createdAt: new Date().toISOString() } })
      onNotice('真实前贴视频已生成并保存为项目素材。')
    } catch (cause) {
      dispatch({ type: 'operation-failed', message: cause instanceof Error ? cause.message : '前贴视频生成失败' })
    }
  }
  const openEditor = async () => {
    if (!workspace?.video_draft.short_drama_preroll_v2.output_asset) return
    try {
      const editTask = await editingApi.openShortDramaV2(currentProject.id, workspace.task.id)
      onOpenEditTask?.(editTask.id)
      onNotice('已创建素材剪辑任务：前贴视频与原视频均已预填入时间线。')
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '无法创建素材剪辑任务')
    }
  }

  return <section className="short-drama-v2" aria-label="短剧前贴创作工作区">
    <aside className="short-drama-v2-rail">
      <div className="short-drama-v2-source-card">
        <span className="short-drama-v2-kicker">SOURCE VIDEO</span>
        <div className="short-drama-v2-source-thumb">{sourceUrl ? <video src={sourceUrl} muted preload="metadata"/> : <Film size={28}/>}<span><Play size={13} fill="currentColor"/></span></div>
        <b>{state.analysis?.title || (state.source ? `项目视频 ${state.source.id.slice(0, 8)}` : '尚未选择短剧素材')}</b>
        <small>{state.analysis?.episode || formatDuration(state.source?.durationSeconds)} · {state.source?.width && state.source?.height ? `${state.source.width}×${state.source.height}` : '竖屏待识别'}</small>
        <button type="button" disabled={uploading} onClick={() => fileInput.current?.click()}><Upload size={14}/>{uploading ? '上传中…' : '更换视频'}</button>
        <input ref={fileInput} hidden type="file" accept="video/*" onChange={event => { void selectLocalFile(event.target.files?.[0]) }}/>
      </div>
      {assets.length > 1 ? <select aria-label="选择项目视频" value={localPreviewUrl ? '' : state.source?.id || ''} onChange={event => { const asset = assets.find(item => item.id === event.target.value); if (asset) selectSource(asset) }}><option value="">选择项目视频</option>{assets.map(asset => <option key={asset.id} value={asset.id}>{`视频 ${asset.id.slice(0, 8)} · ${formatDuration(asset.durationSeconds)}`}</option>)}</select> : null}
      <nav aria-label="短剧前贴流程"><ol>{steps.map((step, index) => {
        const enabled = canOpenShortDramaStep(state, step.id)
        const active = state.activeStep === step.id
        const completed = steps.findIndex(item => item.id === state.activeStep) > index || (step.id === 'video' && state.videoStatus === 'ready')
        return <li key={step.id} className={`${active ? 'active' : ''} ${completed ? 'completed' : ''}`}>
          <button type="button" disabled={!enabled} aria-current={active ? 'step' : undefined} onClick={() => dispatch({ type: 'open-step', step: step.id })}>
            <span className="short-drama-v2-step-index">{completed ? <Check size={14}/> : step.index}</span>
            <span><b>{step.label}</b><small>{step.detail}</small></span><ChevronRight size={15}/>
          </button>
        </li>
      })}</ol></nav>
      <div className="short-drama-v2-rail-note"><span>FLOW V3</span><small>单首帧参考生成，输出跟随源视频画幅，不执行拼接。</small></div>
    </aside>

    <main className="short-drama-v2-main">
      <header><div><span className="short-drama-v2-kicker">SHORT DRAMA · PREROLL LAB</span><h3>{steps.find(step => step.id === state.activeStep)?.label}</h3><p>{steps.find(step => step.id === state.activeStep)?.detail}</p></div><span className="short-drama-v2-autosave"><Check size={13}/>草稿自动保存</span></header>
      {state.error ? <div className="short-drama-v2-error"><CircleAlert size={16}/>{state.error}</div> : null}
      {state.activeStep === 'understanding' ? <UnderstandingStage state={state} sourceUrl={sourceUrl} mediaLoading={mediaLoading} onAnalyze={() => void analyze()}/> : null}
      {state.activeStep === 'direction' ? <DirectionStage state={state} onSummary={value => dispatch({ type: 'summary-changed', value })} onSelect={id => { void selectHook(id) }}/> : null}
      {state.activeStep === 'first-frame' ? <FirstFrameStage state={state} outputAspectLabel={outputAspectLabel} onPrompt={value => dispatch({ type: 'image-prompt-changed', value })} onGenerate={() => void generateImages()} onSelect={id => { void selectImage(id) }}/> : null}
      {state.activeStep === 'video' ? <VideoStage state={state} outputAspectLabel={outputAspectLabel} selectedImageUrl={selectedImage?.imageUrl || ''} onDescription={value => dispatch({ type: 'video-description-changed', value })} onPrompt={value => dispatch({ type: 'video-prompt-changed', value })} onGenerate={() => void generateVideo()} onOpenEditor={() => void openEditor()}/> : null}
    </main>

    <aside className="short-drama-v2-inspector">
      <section className="short-drama-v2-work-switcher">
        <span className="short-drama-v2-kicker">PREROLL WORKS</span>
        <label htmlFor="short-drama-v2-work">当前短剧前贴</label>
        <select id="short-drama-v2-work" value={workspace?.task.id || ''} disabled={switchingWork} onChange={event => { void openSavedWorkspace(event.target.value) }}>
          <option value="">{switchingWork ? '正在恢复…' : '新短剧前贴（未保存）'}</option>
          {savedWorks.map(item => <option key={item.id} value={item.id}>{item.display_name || `短剧前贴 ${item.id.slice(0, 8)}`}</option>)}
        </select>
        <button type="button" onClick={requestNewWorkspace}><Plus size={14}/>新建短剧前贴</button>
      </section>
      <div className="short-drama-v2-inspector-head"><span>生成配置</span><b>{state.activeStep === 'understanding' ? '视频理解' : state.activeStep === 'direction' ? '方向选择' : state.activeStep === 'first-frame' ? '首帧生成' : '视频生成'}</b></div>
      {state.activeStep === 'understanding' ? <><InspectorBlock label="输入状态"><b>{state.source ? '素材已就绪' : '等待视频'}</b><small>{state.source ? `${state.source.mimeType} · ${(state.source.sizeBytes / 1024 / 1024).toFixed(1)} MB` : '请选择项目视频或本地文件'}</small></InspectorBlock><button className="short-drama-v2-primary" disabled={!state.source || state.analysisStatus === 'loading'} onClick={() => void analyze()}>{state.analysisStatus === 'loading' ? <LoaderCircle className="spin" size={16}/> : <Sparkles size={16}/>}理解视频内容</button></> : null}
      {state.activeStep === 'direction' ? <><InspectorBlock label="前贴时长"><div className="short-drama-v2-duration">{prerollDurations.map(duration => <button type="button" disabled={Boolean(state.selectingHookId)} className={state.duration === duration ? 'active' : ''} key={duration} onClick={() => changeDuration(duration)}>{duration}s</button>)}</div><small>先确定时长，再选择一个前贴方向；系统会自动生成匹配该时长的首帧与视频提示词。</small></InspectorBlock><InspectorBlock label="方向构成"><b>猎奇吸睛 × 2</b><b>剧情总结 × 2</b><small>点击方向后将锁定当前时长，并进入首帧生成。</small></InspectorBlock><button className="short-drama-v2-primary" disabled={!state.summaryDraft.trim() || state.hooksStatus === 'loading'} onClick={() => void generateHooks()}>{state.hooksStatus === 'loading' ? <LoaderCircle className="spin" size={16}/> : <WandSparkles size={16}/>}生成 4 个前贴方向</button></> : null}
      {state.activeStep === 'first-frame' ? <><InspectorBlock label="已选方向"><b>{selectedHook?.title || '尚未选择'}</b><small>{selectedHook?.hookCopy}</small><small>已锁定时长：{state.duration}s</small></InspectorBlock><InspectorBlock label="输出画幅"><b>{outputAspectLabel}</b><small>参考图预览与最终视频都按源视频画幅呈现。</small></InspectorBlock><button className="short-drama-v2-primary" disabled={Boolean(state.selectingHookId) || !state.imagePrompt.trim() || state.imagesStatus === 'loading'} onClick={() => void generateImages()}>{state.selectingHookId || state.imagesStatus === 'loading' ? <LoaderCircle className="spin" size={16}/> : <ImageIcon size={16}/>} {state.selectingHookId ? '正在生成提示词' : '生成 3 张首帧图'}</button></> : null}
      {state.activeStep === 'video' ? <><InspectorBlock label="参考链路"><small>模型输入：选中的一张 AI 首帧</small><small>生成方式：Prompt + 单张 reference_image</small><small>输出：独立前贴 · {outputAspectLabel}</small></InspectorBlock><button className="short-drama-v2-primary" disabled={!state.selectedImageId || !state.videoPrompt.trim() || state.videoStatus === 'loading'} onClick={() => void generateVideo()}>{state.videoStatus === 'loading' ? <LoaderCircle className="spin" size={16}/> : <Clapperboard size={16}/>}生成前贴视频</button></> : null}
      <div className="short-drama-v2-contract"><span>WORKSPACE V3</span><small>任务、草稿、首帧选择、生成进度和源画幅视频结果均由服务端持久化。</small></div>
    </aside>
    {showSaveDialog ? <div className="short-drama-v2-save-dialog" role="dialog" aria-modal="true" aria-labelledby="short-drama-v2-save-title">
      <form onSubmit={event => { event.preventDefault(); void saveAndCreateWorkspace() }}>
        <span className="short-drama-v2-kicker">SAVE CURRENT WORK</span>
        <h3 id="short-drama-v2-save-title">先保存当前短剧前贴</h3>
        <p>当前流程已有内容。命名保存后，它会出现在左侧作品下拉框中，之后可以继续恢复编辑。</p>
        <label htmlFor="short-drama-v2-save-name">作品名称</label>
        <input id="short-drama-v2-save-name" autoFocus maxLength={80} value={saveName} onChange={event => setSaveName(event.target.value)} placeholder="例如：武则天·无字碑悬念前贴"/>
        <div><button type="button" onClick={() => setShowSaveDialog(false)}>取消</button><button type="submit" disabled={!saveName.trim() || savingWork}>{savingWork ? <LoaderCircle className="spin" size={15}/> : <Save size={15}/>}保存并新建</button></div>
      </form>
    </div> : null}
  </section>
}

function InspectorBlock({ label, children }: { label: string; children: React.ReactNode }) { return <section className="short-drama-v2-inspector-block"><span>{label}</span>{children}</section> }

function UnderstandingStage({ state, sourceUrl, mediaLoading, onAnalyze }: { state: ShortDramaPrerollState; sourceUrl: string; mediaLoading: boolean; onAnalyze: () => void }) {
  return <div className="short-drama-v2-stage">
    <section className="short-drama-v2-media-canvas">{sourceUrl ? <video src={sourceUrl} controls preload="metadata"/> : <div><Film size={34}/><b>{mediaLoading ? '正在读取项目素材…' : '选择一条短剧视频开始'}</b><small>支持从项目素材或本地视频进入</small></div>}<span>INPUT / SHORT DRAMA</span></section>
    {state.analysis ? <section className="short-drama-v2-analysis-grid"><article><span>剧情梗概</span><h4>{state.analysis.title}</h4><p>{state.summaryDraft}</p></article><article><span>开场信息</span><p>{state.analysis.openingBeat}</p><div className="short-drama-v2-tags">{state.analysis.visualKeywords.map(item => <small key={item}>{item}</small>)}</div></article></section> : <section className="short-drama-v2-empty-action"><Sparkles size={20}/><div><b>先让系统理解输入视频</b><small>将提取标题、梗概、人物、开场动作与视觉关键词，结果允许人工修改。</small></div><button disabled={!state.source || state.analysisStatus === 'loading'} onClick={onAnalyze}>{state.analysisStatus === 'loading' ? '分析中…' : '开始理解'}</button></section>}
  </div>
}

function DirectionStage({ state, onSummary, onSelect }: { state: ShortDramaPrerollState; onSummary: (value: string) => void; onSelect: (id: string) => void }) {
  return <div className="short-drama-v2-stage"><section className="short-drama-v2-editor-card"><div><span>EDITABLE STORY SUMMARY</span><b>视频梗概</b></div><textarea value={state.summaryDraft} onChange={event => onSummary(event.target.value)} rows={4}/><small>修改梗概会使已有方向、首帧与视频结果失效。</small></section>
    {state.hooks.length ? <div className="short-drama-v2-hook-groups">{(['curiosity', 'summary'] as const).map(category => <section key={category}><header><span>{category === 'curiosity' ? 'CURIOUSITY HOOK' : 'STORY SUMMARY'}</span><b>{category === 'curiosity' ? '猎奇吸睛' : '剧情总结'}</b></header><div>{state.hooks.filter(item => item.category === category).map(hook => <button type="button" key={hook.id} className={state.selectedHookId === hook.id ? 'selected' : ''} onClick={() => onSelect(hook.id)}><span>{hook.eyebrow}</span><h4>{hook.title}</h4><p>{hook.hookCopy}</p><small>{hook.description}</small>{state.selectedHookId === hook.id ? <i><Check size={13}/></i> : null}</button>)}</div></section>)}</div> : <section className="short-drama-v2-empty-action"><WandSparkles size={20}/><div><b>生成两类、四个方向</b><small>请使用右侧唯一的“生成 4 个前贴方向”按钮；结果会按猎奇吸睛和剧情总结分组展示在这里。</small></div></section>}
  </div>
}

function FirstFrameStage({ state, outputAspectLabel, onPrompt, onGenerate, onSelect }: { state: ShortDramaPrerollState; outputAspectLabel: string; onPrompt: (value: string) => void; onGenerate: () => void; onSelect: (id: string) => void }) {
  const [previewImage, setPreviewImage] = useState<FirstFrameCandidate | null>(null)
  useEffect(() => {
    if (!previewImage) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setPreviewImage(null)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [previewImage])
  if (state.selectingHookId) return <div className="short-drama-v2-stage"><section className="short-drama-v2-empty-action short-drama-v2-selection-loading"><LoaderCircle className="spin" size={20}/><div><b>方向已选定，正在生成创作提示词</b><small>系统正在根据该方向编译首帧提示词、视频描述和视频提示词，完成后会停留在本步骤。</small></div></section></div>
  return <div className="short-drama-v2-stage"><section className="short-drama-v2-editor-card"><div><span>SEEDREAM IMAGE PROMPT</span><b>首帧图提示词</b></div><textarea value={state.imagePrompt} onChange={event => onPrompt(event.target.value)} rows={6}/><small>提示词包含主体、环境、构图、镜头、光影、风格与禁止项，可人工编辑。</small></section>
    {state.images.length ? <section className="short-drama-v2-image-grid"><header><div><span>FIRST FRAME OPTIONS · {outputAspectLabel}</span><b>点击图片放大查看，再选用一张作为视频唯一参考图</b></div><button type="button" disabled={state.imagesStatus === 'loading'} onClick={onGenerate}><RefreshCw size={14}/>重新生成 3 张</button></header><div>{state.images.map(image => <article key={image.id} className={state.selectedImageId === image.id ? 'selected' : ''}><button type="button" className="short-drama-v2-image-preview" aria-label={`放大查看 ${image.label}`} onClick={() => setPreviewImage(image)}><img src={image.imageUrl} alt={image.composition} style={{ aspectRatio: outputAspectLabel.replace(':', ' / ') }}/><span><ZoomIn size={16}/>放大查看</span></button><div><b>{image.label}</b><small>{image.composition}</small><button type="button" className="short-drama-v2-image-select" disabled={Boolean(state.selectingImageId)} onClick={() => onSelect(image.id)}>{state.selectingImageId === image.id ? <><LoaderCircle className="spin" size={14}/>正在选用…</> : state.selectedImageId === image.id ? <><Check size={14}/>已选用</> : '选用此图'}</button></div>{state.selectedImageId === image.id ? <i><Check size={14}/></i> : null}</article>)}</div></section> : <section className="short-drama-v2-empty-action"><ImageIcon size={20}/><div><b>生成 3 张机制与风格不同的首帧参考</b><small>动漫电影、国漫半写实、电影写实各一张，预览画幅跟随源视频。</small></div><button disabled={!state.imagePrompt || state.imagesStatus === 'loading'} onClick={onGenerate}>{state.imagesStatus === 'loading' ? '生成中…' : '生成首帧'}</button></section>}
    {previewImage ? <div className="short-drama-v2-lightbox" role="dialog" aria-modal="true" aria-label={`${previewImage.label} 大图预览`} onClick={() => setPreviewImage(null)}><div onClick={event => event.stopPropagation()}><button type="button" className="short-drama-v2-lightbox-close" aria-label="关闭大图预览" onClick={() => setPreviewImage(null)}><X size={20}/></button><img src={previewImage.imageUrl} alt={previewImage.composition}/><footer><b>{previewImage.label}</b><span>{previewImage.composition}</span></footer></div></div> : null}
  </div>
}

function VideoStage({ state, outputAspectLabel, selectedImageUrl, onDescription, onPrompt, onGenerate, onOpenEditor }: { state: ShortDramaPrerollState; outputAspectLabel: string; selectedImageUrl: string; onDescription: (value: string) => void; onPrompt: (value: string) => void; onGenerate: () => void; onOpenEditor: () => void }) {
  return <div className="short-drama-v2-stage"><section className="short-drama-v2-reference-flow single-reference"><div><span>SELECTED REFERENCE IMAGE</span><img src={selectedImageUrl} alt="已选前贴首帧"/></div><ChevronRight/><div className="short-drama-v2-reference-method"><span>GENERATION INPUT</span><b>Prompt + 单张首帧参考</b><small>不使用短剧首帧作为尾帧，不要求可信素材 Asset ID。</small></div><div className="short-drama-v2-reference-meta"><Clock3 size={16}/><b>{state.duration}s</b><small>独立前贴 · {outputAspectLabel}</small></div></section>
    <section className="short-drama-v2-editor-card compact"><div><span>VIDEO DESCRIPTION</span><b>视频描述</b></div><textarea value={state.videoDescription} onChange={event => onDescription(event.target.value)} rows={2}/></section>
    <section className="short-drama-v2-editor-card"><div><span>SEEDANCE VIDEO PROMPT</span><b>前贴视频提示词</b></div><textarea value={state.videoPrompt} onChange={event => onPrompt(event.target.value)} rows={7}/><small>提示词已写入所选首帧的风格与视觉机制；模型只接收这一张参考图，结果按源视频画幅归一化。</small></section>
    {state.output ? <section className="short-drama-v2-output"><header><div><span>GENERATED PREROLL</span><b>前贴视频已生成</b></div><small>已持久化为项目视频素材</small></header>{state.output.videoUrl ? <video src={state.output.videoUrl} controls/> : null}<button type="button" onClick={onOpenEditor}><Film size={15}/>进入素材剪辑</button></section> : <section className="short-drama-v2-empty-action"><Clapperboard size={20}/><div><b>参数已就绪</b><small>确认提示词、描述与时长后，生成一条独立前贴视频。</small></div><button disabled={state.videoStatus === 'loading'} onClick={onGenerate}>{state.videoStatus === 'loading' ? '生成中…' : '生成视频'}</button></section>}
  </div>
}
