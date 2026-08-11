import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { Captions, Download, Eye, EyeOff, Image as ImageIcon, Lock, Magnet, Music2, Pause, Play, Redo2, Save, Scissors, Search, Trash2, Undo2, Unlock, Upload, Volume2, VolumeX } from 'lucide-react'

import { useProject } from '../../context/ProjectContext'
import { api, type ApiProjectMediaAsset } from '../../data/api'
import { shortId } from '../../data/shortId'
import { editingApi, type ApiEditTask, type ApiEditingCreativePackage, type ApiEditingCreativeVersion, type ApiEditingRenderJob } from './api'
import { buildVisualOperations } from './operations'
import { createCaptionDocument, deleteCaption, importSubtitleText, mergeCaptionWithNext, splitCaption, updateCaption, type CaptionClip } from './captionDocument'
import { audioClips, deleteAudioClip, insertAudioAsset, moveAudioClip, setAudioTrackMuted, splitAudioClip, trimAudioClip, updateAudioClip, type AudioAsset, type AudioClip, type AudioRole } from './audioDocument'
import { ASSET_DRAG_MIME } from './dragContract'
import {
  alignFrame,
  canvasForProfile,
  createEmptyVisualDocument,
  deleteVisualClip,
  findVisualClip,
  insertVisualAsset,
  moveVisualClip,
  restoreVisualDocument,
  setCanvasProfile,
  setVisualTrackState,
  splitVisualClip,
  toTimelineV2,
  trimVisualClip,
  updateVisualTransform,
  updateOriginalAudio,
  visualClips,
  type CanvasProfileId,
  type VisualAsset,
  type VisualClip,
  type VisualEditorDocument,
  type VisualTrack,
} from './visualTimeline'

type Props = { editTaskId: string; onNotice: (message: string) => void; onOpenEditTask: (id: string) => void }
type History = { past: VisualEditorDocument[]; present: VisualEditorDocument; future: VisualEditorDocument[] }
type SaveState = 'clean' | 'dirty' | 'saving' | 'error' | 'conflict'

export function VideoEditingCanvasWorkspace({ editTaskId, onNotice, onOpenEditTask }: Props) {
  const { currentProject } = useProject()
  const [assets, setAssets] = useState<ApiProjectMediaAsset[]>([])
  const [task, setTask] = useState<ApiEditTask | null>(null)
  const [history, setHistory] = useState<History>(() => ({ past: [], present: createEmptyVisualDocument(), future: [] }))
  const [selectedClipId, setSelectedClipId] = useState('')
  const [selectedCaptionId, setSelectedCaptionId] = useState('')
  const [selectedAudioId, setSelectedAudioId] = useState('')
  const [leftMode, setLeftMode] = useState<'media' | 'captions' | 'audio'>('media')
  const [activeAudioTrackId, setActiveAudioTrackId] = useState('audio-music')
  const [activeTrackId, setActiveTrackId] = useState('video-primary')
  const [playheadMs, setPlayheadMs] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [zoom, setZoom] = useState(1)
  const [snapEnabled, setSnapEnabled] = useState(true)
  const [filter, setFilter] = useState<'all' | 'video' | 'image'>('all')
  const [query, setQuery] = useState('')
  const [uploading, setUploading] = useState(false)
  const [saveState, setSaveState] = useState<SaveState>('clean')
  const [render, setRender] = useState<ApiEditingRenderJob | null>(null)
	const [creativeVersion, setCreativeVersion] = useState<ApiEditingCreativeVersion | null>(null)
	const [creativePackage, setCreativePackage] = useState<ApiEditingCreativePackage | null>(null)
	const [reviewBusy, setReviewBusy] = useState(false)
  const savedRef = useRef(createEmptyVisualDocument())
  const saveRevisionRef = useRef(0)
  const restoredRef = useRef('')
  const document = history.present
  const clips = useMemo(() => visualClips(document), [document])
  const selectedClip = useMemo(() => findVisualClip(document, selectedClipId), [document, selectedClipId])
  const selectedCaption = useMemo(() => document.captions.captions.find(caption => caption.id === selectedCaptionId), [document, selectedCaptionId])
  const selectedAudio = useMemo(() => audioClips(document.audio).find(clip => clip.id === selectedAudioId), [document, selectedAudioId])
  const playbackRef = useRef({ frame: 0, previous: 0 })

  useEffect(() => {
    if (!playing || !document.durationMs) return
    playbackRef.current.previous = performance.now()
    const tick = (now: number) => {
      const delta = now - playbackRef.current.previous
      playbackRef.current.previous = now
      setPlayheadMs(current => {
        const next = current + delta
        if (next >= document.durationMs) { setPlaying(false); return document.durationMs }
        return next
      })
      playbackRef.current.frame = requestAnimationFrame(tick)
    }
    playbackRef.current.frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(playbackRef.current.frame)
  }, [playing, document.durationMs])

  const loadAssets = async () => {
    const values = await api.listProjectMediaAssets(currentProject.id)
    setAssets(values.filter(asset => (asset.kind === 'video' || asset.kind === 'image' || asset.kind === 'audio') && asset.useAllowed !== false))
  }

  useEffect(() => { void loadAssets().catch(() => onNotice('素材箱加载失败，请刷新重试。')) }, [currentProject.id])
  useEffect(() => { void editingApi.get(currentProject.id, editTaskId).then(setTask).catch(cause => onNotice(cause instanceof Error ? cause.message : '剪辑任务读取失败')) }, [currentProject.id, editTaskId, onNotice])
	useEffect(() => { void editingApi.listVersions(currentProject.id, editTaskId).then(result => setCreativeVersion(result.items[0] ?? null)).catch(() => undefined) }, [currentProject.id, editTaskId])

  const visualAssets = useMemo(() => assets.filter(asset => asset.kind === 'video' || asset.kind === 'image').map(toVisualAsset), [assets])
  const audioAssets = useMemo(() => assets.filter(asset => asset.kind === 'audio').map(toAudioAsset), [assets])
  useEffect(() => {
    if (!task || restoredRef.current === `${task.id}:v${task.current_timeline?.version ?? 0}`) return
    if (task.current_timeline && !visualAssets.length) return
    try {
      const restored = task.current_timeline ? restoreVisualDocument(task.current_timeline.timeline, [...visualAssets, ...audioAssets]) : createEmptyVisualDocument()
      restoredRef.current = `${task.id}:v${task.current_timeline?.version ?? 0}`
      savedRef.current = restored
      setHistory({ past: [], present: restored, future: [] })
      setSelectedClipId(visualClips(restored)[0]?.id ?? '')
      setSaveState('clean')
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '时间线恢复失败') }
  }, [task, visualAssets, audioAssets, onNotice])

  const commit = (change: (value: VisualEditorDocument) => VisualEditorDocument) => setHistory(current => {
    const next = change(current.present)
    if (next === current.present) return current
    saveRevisionRef.current++
    setSaveState('dirty')
    return { past: [...current.past, current.present], present: next, future: [] }
  })

  const undo = () => setHistory(current => {
    const previous = current.past.at(-1)
    if (!previous) return current
    saveRevisionRef.current++
    setSaveState('dirty')
    return { past: current.past.slice(0, -1), present: previous, future: [current.present, ...current.future] }
  })
  const redo = () => setHistory(current => {
    const next = current.future[0]
    if (!next) return current
    saveRevisionRef.current++
    setSaveState('dirty')
    return { past: [...current.past, current.present], present: next, future: current.future.slice(1) }
  })

  const save = async (snapshot = document) => {
    if (!task || !visualClips(snapshot).length) return task
    const revision = saveRevisionRef.current
    setSaveState('saving')
    try {
      let saved: ApiEditTask
      if (!task.current_timeline) saved = await editingApi.saveTimeline(currentProject.id, task.id, 0, toTimelineV2(snapshot))
      else {
        const batchId = `batch-${typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : Date.now()}`
        const batch = buildVisualOperations(savedRef.current, snapshot, task.current_timeline.version, batchId)
        saved = batch.operations.length ? await editingApi.applyOperations(currentProject.id, task.id, batch) : task
      }
      setTask(saved)
      savedRef.current = snapshot
      restoredRef.current = `${saved.id}:v${saved.current_timeline?.version ?? 0}`
      setSaveState(saveRevisionRef.current === revision ? 'clean' : 'dirty')
      return saved
    } catch (cause) {
      setSaveState(cause instanceof Error && 'status' in cause && cause.status === 409 ? 'conflict' : 'error')
      onNotice(cause instanceof Error ? cause.message : '时间线保存失败')
      return null
    }
  }

  const loadServerVersion = async () => {
    try {
      const serverTask = await editingApi.get(currentProject.id, editTaskId)
      const restored = serverTask.current_timeline ? restoreVisualDocument(serverTask.current_timeline.timeline, [...visualAssets, ...audioAssets]) : createEmptyVisualDocument()
      setTask(serverTask)
      savedRef.current = restored
      restoredRef.current = `${serverTask.id}:v${serverTask.current_timeline?.version ?? 0}`
      saveRevisionRef.current++
      setHistory({ past: [], present: restored, future: [] })
      setSelectedClipId(visualClips(restored)[0]?.id ?? '')
      setSelectedCaptionId('')
      setSelectedAudioId('')
      setSaveState('clean')
      onNotice(`已载入服务端时间线 v${serverTask.current_timeline?.version ?? 0}。`)
    } catch (cause) {
      onNotice(cause instanceof Error ? cause.message : '服务端版本载入失败')
    }
  }

  const saveAsNewTask = async () => {
    if (!clips.length) return
    setSaveState('saving')
    try {
      const saved = await editingApi.create(currentProject.id, { display_name: `${task?.display_name ?? '素材剪辑'}（冲突副本）`, timeline: toTimelineV2(document) })
      setTask(saved)
      savedRef.current = document
      restoredRef.current = `${saved.id}:v${saved.current_timeline?.version ?? 0}`
      saveRevisionRef.current++
      setSaveState('clean')
      onOpenEditTask(saved.id)
      onNotice(`当前修改已另存为 EditTask ${saved.id}。`)
    } catch (cause) {
      setSaveState('error')
      onNotice(cause instanceof Error ? cause.message : '另存 EditTask 失败')
    }
  }

  useEffect(() => {
    if (saveState !== 'dirty' || !clips.length) return
    const timer = window.setTimeout(() => { void save(document) }, 1200)
    return () => window.clearTimeout(timer)
  }, [document, saveState, clips.length])

  useEffect(() => {
    if (!render || !['queued', 'running'].includes(render.status)) return
    const timer = window.setInterval(() => { void editingApi.getRender(currentProject.id, render.id).then(next => { setRender(next); if (!['queued', 'running'].includes(next.status)) { void loadAssets(); void editingApi.get(currentProject.id, editTaskId).then(setTask) } }) }, 1200)
    return () => window.clearInterval(timer)
  }, [render, currentProject.id])

  const createRender = async (kind: 'preview' | 'export') => {
    const saved = saveState === 'clean' ? task : await save()
    if (!saved) return
    try { setRender(await editingApi.createRender(currentProject.id, saved.id, kind)) } catch (cause) { onNotice(cause instanceof Error ? cause.message : '渲染任务创建失败') }
  }

	const submitForReview = async () => {
		if (!task?.current_timeline || !render || render.kind !== 'export' || render.status !== 'succeeded') return
		setReviewBusy(true)
		try {
			const value = await editingApi.submitVersion(currentProject.id, task.id, render.id, task.current_timeline.version)
			setCreativeVersion(value); onNotice('正式成片已冻结为 CreativeVersion，可以执行规格检查。')
		} catch (cause) { onNotice(cause instanceof Error ? cause.message : '提交评审失败') }
		finally { setReviewBusy(false) }
	}
	const checkVersion = async () => {
		if (!creativeVersion) return
		setReviewBusy(true)
		try { setCreativeVersion(await editingApi.checkVersion(currentProject.id, creativeVersion.id)) } catch (cause) { onNotice(cause instanceof Error ? cause.message : '成片检查失败') } finally { setReviewBusy(false) }
	}
	const approveVersion = async () => {
		if (!creativeVersion) return
		setReviewBusy(true)
		try { const value = await editingApi.approveVersion(currentProject.id, creativeVersion.id); setCreativeVersion(value); void editingApi.get(currentProject.id, editTaskId).then(setTask) } catch (cause) { onNotice(cause instanceof Error ? cause.message : '成片批准失败') } finally { setReviewBusy(false) }
	}
	const deliverVersion = async () => {
		if (!creativeVersion) return
		setReviewBusy(true)
		try { setCreativePackage(await editingApi.deliverVersion(currentProject.id, creativeVersion.id)); onNotice('CreativePackage 已创建，成片进入交付主链路。') } catch (cause) { onNotice(cause instanceof Error ? cause.message : '成片交付失败') } finally { setReviewBusy(false) }
	}

  const addAsset = (asset: ApiProjectMediaAsset, trackId = activeTrackId, atMs = playheadMs) => {
    const value = toVisualAsset(asset)
    commit(current => insertVisualAsset(current, value, trackId, atMs || (trackId === 'video-primary' ? current.durationMs : 0)))
    window.setTimeout(() => setSelectedClipId(`clip-${value.assetId}-v${value.assetVersion}-${visualClips(document).filter(clip => clip.assetId === value.assetId && clip.assetVersion === value.assetVersion).length + 1}`))
  }

  const addAudio = (asset: ApiProjectMediaAsset) => {
    const value = toAudioAsset(asset)
    commit(current => ({ ...current, audio: insertAudioAsset(current.audio, value, activeAudioTrackId, playheadMs) }))
    setSelectedAudioId(`audio-${value.assetId}-v${value.assetVersion}-${audioClips(document.audio).filter(clip => clip.assetId === value.assetId && clip.assetVersion === value.assetVersion).length + 1}`)
    setSelectedClipId(''); setSelectedCaptionId('')
  }

  const upload = async (file?: File) => {
    if (!file) return
    setUploading(true)
    try { await api.uploadProjectAsset(currentProject.id, file); await loadAssets(); onNotice('素材已上传，需点击“加入所选轨道”才会进入时间线。') }
    catch (cause) { onNotice(cause instanceof Error ? cause.message : '素材上传失败') }
    finally { setUploading(false) }
  }

  const importCaptions = async (file?: File) => {
    if (!file) return
    try {
      const imported = importSubtitleText(file.name, await file.text())
      commit(current => ({ ...current, captions: createCaptionDocument(imported.captions, current.captions.language, current.captions.trackId) }))
      setSelectedCaptionId(imported.captions[0]?.id ?? '')
      setSelectedClipId('')
      onNotice(`已导入 ${imported.captions.length} 条字幕，请逐条校对后保存。`)
    } catch (cause) { onNotice(cause instanceof Error ? cause.message : '字幕导入失败') }
  }

  const shownAssets = assets.filter(asset => asset.kind !== 'audio' && (filter === 'all' || asset.kind === filter) && (!query || assetLabel(asset).toLowerCase().includes(query.toLowerCase()) || asset.id.toLowerCase().includes(query.toLowerCase())))
  const canvas = canvasForProfile(document.canvasProfileId)
	const authoritativeAsset = render?.status === 'succeeded' && render.output_asset ? assets.find(asset => asset.id === render.output_asset?.asset_version.asset_id && asset.version === render.output_asset.asset_version.version) : undefined
  return <div className="video-editing-workspace c3-editor-workspace">
    <header className="editing-toolbar"><div><span className="section-label">EditTask · {shortId(task?.id ?? editTaskId)}</span><b>三轨素材剪辑</b><small>时间线 v{task?.current_timeline?.version ?? 0} · {saveStateLabel(saveState)} · {editTaskStatusLabel(task?.status)}</small></div><div><button className="secondary-button" disabled={!clips.length || !!render && ['queued', 'running'].includes(render.status)} onClick={() => void createRender('preview')}><Play size={14}/>低清预览</button><button className="primary-button" disabled={!clips.length || !!render && ['queued', 'running'].includes(render.status)} onClick={() => void createRender('export')}><Download size={14}/>正式导出</button></div></header>
    <div className="c3-editor-shell">
      <aside className="c3-assets">
        <div className="surface-toolbar"><h3>{leftMode === 'media' ? '视频 / 图片素材' : leftMode === 'captions' ? '字幕校对' : '音频素材'}</h3><span>{leftMode === 'media' ? `${visualAssets.length} 个` : leftMode === 'captions' ? `${document.captions.captions.length} 条` : `${audioAssets.length} 个`}</span></div>
        <div className="c4-library-tabs c5-library-tabs"><button className={leftMode === 'media' ? 'active' : ''} onClick={() => setLeftMode('media')}>素材</button><button className={leftMode === 'captions' ? 'active' : ''} onClick={() => setLeftMode('captions')}><Captions size={12}/>字幕</button><button className={leftMode === 'audio' ? 'active' : ''} onClick={() => setLeftMode('audio')}><Music2 size={12}/>音频</button></div>
        {leftMode === 'media' ? <>
        <label className="video-editor-upload"><Upload size={14}/>{uploading ? '上传中…' : '上传视频或图片'}<input type="file" accept="video/*,image/jpeg,image/png,image/webp" disabled={uploading} onChange={event => { void upload(event.target.files?.[0]); event.currentTarget.value = '' }}/></label>
        <div className="c3-asset-filters"><button className={filter === 'all' ? 'active' : ''} onClick={() => setFilter('all')}>全部</button><button className={filter === 'video' ? 'active' : ''} onClick={() => setFilter('video')}>视频</button><button className={filter === 'image' ? 'active' : ''} onClick={() => setFilter('image')}>图片</button></div>
        <label className="c3-search"><Search size={13}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索素材 ID"/></label>
        <div className="c3-asset-list">{shownAssets.map(asset => <article key={`${asset.id}:v${asset.version}`} draggable onDragStart={event => { event.dataTransfer.setData(ASSET_DRAG_MIME, JSON.stringify({ assetId: asset.id, assetVersion: asset.version })); event.dataTransfer.effectAllowed = 'copy' }}>
          <div className="c3-asset-thumb">{asset.kind === 'image' ? <img src={asset.contentUrl} alt=""/> : <video src={asset.contentUrl} muted preload="metadata"/>}<span>{asset.kind === 'image' ? <ImageIcon size={12}/> : <Play size={12}/>}</span></div>
          <b>{assetLabel(asset)}</b><small>{asset.kind === 'video' ? `${(asset.durationSeconds ?? 0).toFixed(1)} 秒` : `${asset.width ?? '?'}×${asset.height ?? '?'}`} · v{asset.version}</small><em>{asset.contentUrl ? '预览就绪' : '代理处理中'}</em>
          <button onClick={() => addAsset(asset)}>加入所选轨道</button>
        </article>)}</div></> : leftMode === 'captions' ? <CaptionLibrary captions={document.captions.captions} selectedId={selectedCaptionId} onSelect={caption => { setSelectedCaptionId(caption.id); setSelectedClipId(''); setSelectedAudioId(''); setPlayheadMs(caption.timelineStartMs) }} onImport={importCaptions}/> : <AudioLibrary assets={assets.filter(asset => asset.kind === 'audio')} activeTrackId={activeAudioTrackId} uploading={uploading} onTrack={setActiveAudioTrackId} onAdd={addAudio} onUpload={upload}/>}
      </aside>
      <main className="c3-center">
        {authoritativeAsset ? <AuthoritativePreview asset={authoritativeAsset} render={render}/> : <VisualCanvas document={document} selectedClipId={selectedClipId} playheadMs={playheadMs} playing={playing} onTogglePlaying={() => { if (playheadMs >= document.durationMs) setPlayheadMs(0); setPlaying(value => !value) }} onSelect={id => { setSelectedClipId(id); setSelectedCaptionId(''); setSelectedAudioId('') }} onTransform={(id, patch) => commit(current => updateVisualTransform(current, id, patch))}/>}<AudioPlayback document={document} playheadMs={playheadMs} playing={playing}/>
        <VisualTimeline document={document} assets={visualAssets} playheadMs={playheadMs} selectedClipId={selectedClipId} selectedCaptionId={selectedCaptionId} selectedAudioId={selectedAudioId} activeTrackId={activeTrackId} zoom={zoom} snapEnabled={snapEnabled} canUndo={!!history.past.length} canRedo={!!history.future.length} onSeek={setPlayheadMs} onSelect={id => { setSelectedClipId(id); setSelectedCaptionId(''); setSelectedAudioId('') }} onSelectCaption={id => { setSelectedCaptionId(id); setSelectedClipId(''); setSelectedAudioId(''); setLeftMode('captions') }} onSelectAudio={id => { setSelectedAudioId(id); setSelectedClipId(''); setSelectedCaptionId(''); setLeftMode('audio') }} onActiveTrack={setActiveTrackId} onZoom={setZoom} onToggleSnap={() => setSnapEnabled(value => !value)} onUndo={undo} onRedo={redo} onDelete={() => { if (selectedAudioId) commit(current => ({ ...current, audio: deleteAudioClip(current.audio, selectedAudioId) })); else if (selectedCaptionId) commit(current => ({ ...current, captions: deleteCaption(current.captions, selectedCaptionId) })); else if (selectedClipId) commit(current => deleteVisualClip(current, selectedClipId)) }} onSplit={() => { if (selectedAudioId) commit(current => ({ ...current, audio: splitAudioClip(current.audio, selectedAudioId, playheadMs) })); else if (selectedCaptionId && selectedCaption) commit(current => ({ ...current, captions: splitCaption(current.captions, selectedCaptionId, playheadMs, Math.max(1, Math.floor([...selectedCaption.text].length / 2))) })); else if (selectedClipId) commit(current => splitVisualClip(current, selectedClipId, playheadMs)) }} onMove={(id, trackId, atMs) => commit(current => moveVisualClip(current, id, trackId, atMs))} onInsert={(asset, trackId, atMs) => commit(current => insertVisualAsset(current, asset, trackId, atMs))} onMoveAudio={(id, trackId, atMs) => commit(current => ({ ...current, audio: moveAudioClip(current.audio, id, trackId, atMs) }))}/>
      </main>
      <aside className="c3-inspector">
        <div className="surface-toolbar"><h3>画布与属性</h3><button title="立即保存" disabled={saveState === 'saving' || !clips.length} onClick={() => void save()}><Save size={14}/></button></div>
        <label>画布比例<select value={document.canvasProfileId} onChange={event => commit(current => setCanvasProfile(current, event.target.value as CanvasProfileId))}><option value="vertical-720p-v1">9:16 · 720×1280</option><option value="landscape-720p-v1">16:9 · 1280×720</option><option value="square-1080-v1">1:1 · 1080×1080</option></select></label>
        <div className="c3-output-summary"><b>{canvas.width} × {canvas.height}</b><span>MP4 / H.264 / AAC · 30fps</span><small>安全区已显示在中央画布</small></div>
        {selectedAudio ? <AudioInspector clip={selectedAudio} onChange={patch => commit(current => ({ ...current, audio: updateAudioClip(current.audio, selectedAudio.id, patch) }))} onTrim={(start, end, sourceIn, sourceOut) => commit(current => ({ ...current, audio: trimAudioClip(current.audio, selectedAudio.id, start, end, sourceIn, sourceOut) }))}/> : selectedCaption ? <CaptionInspector caption={selectedCaption} hasNext={document.captions.captions.findIndex(item => item.id === selectedCaption.id) < document.captions.captions.length - 1} onChange={patch => commit(current => ({ ...current, captions: updateCaption(current.captions, selectedCaption.id, patch) }))} onSplit={() => commit(current => ({ ...current, captions: splitCaption(current.captions, selectedCaption.id, (selectedCaption.timelineStartMs + selectedCaption.timelineEndMs) / 2, Math.max(1, Math.floor([...selectedCaption.text].length / 2))) }))} onMerge={() => commit(current => ({ ...current, captions: mergeCaptionWithNext(current.captions, selectedCaption.id) }))}/> : selectedClip ? <TransformInspector clip={selectedClip} onChange={patch => commit(current => updateVisualTransform(current, selectedClip.id, patch))} onTrim={(start, end, sourceIn, sourceOut) => commit(current => trimVisualClip(current, selectedClip.id, start, end, sourceIn, sourceOut))} onOriginalAudio={patch => commit(current => updateOriginalAudio(current, selectedClip.id, patch))}/> : <p className="c3-empty-inspector">选择时间线片段、字幕或音频后，可在这里调整属性。</p>}
        <div className="c3-track-inspector"><b>视觉轨状态</b>{document.tracks.slice().reverse().map(track => <TrackState key={track.id} track={track} active={activeTrackId === track.id} onActivate={() => setActiveTrackId(track.id)} onChange={patch => commit(current => setVisualTrackState(current, track.id, patch))}/>)}</div>
        <div className="c3-track-inspector"><b>音频总线</b>{document.audio.tracks.map(track => <div key={track.id} className={`c3-track-state${activeAudioTrackId === track.id ? ' active' : ''}`} onClick={() => { setActiveAudioTrackId(track.id); setLeftMode('audio') }}><span>{audioRoleLabel(track.role)} · {track.clips.length} 段</span><button title={track.muted ? '取消静音' : '静音轨道'} onClick={event => { event.stopPropagation(); commit(current => ({ ...current, audio: setAudioTrackMuted(current.audio, track.id, !track.muted) })) }}>{track.muted ? <VolumeX size={13}/> : <Volume2 size={13}/>}</button></div>)}</div>
        {saveState === 'conflict' ? <div className="timeline-conflict-actions" role="alert"><b>服务端已有更新</b><small>可放弃本地修改载入最新版本，或保留当前内容另存为新任务。</small><button className="secondary-button full" type="button" onClick={() => void loadServerVersion()}>载入服务端版本</button><button className="secondary-button full" type="button" onClick={() => void saveAsNewTask()}>另存为新 EditTask</button></div> : null}
        {render ? <div className={`c3-render-state status-${render.status}`}><b>{render.kind === 'preview' ? '低清预览' : '正式导出'} · {render.status}</b><span>{render.progress_percent}%</span>{render.error_message ? <small>{render.error_message}</small> : null}</div> : null}
		<C6ReviewPanel task={task} render={render} version={creativeVersion} creativePackage={creativePackage} busy={reviewBusy} onSubmit={submitForReview} onCheck={checkVersion} onApprove={approveVersion} onDeliver={deliverVersion}/>
      </aside>
    </div>
  </div>
}

function AuthoritativePreview({ asset, render }: { asset: ApiProjectMediaAsset; render: ApiEditingRenderJob | null }) {
	if (!render) return null
	return <section className="c6-authoritative-preview"><video src={asset.contentUrl} controls playsInline preload="metadata"/><div><b>权威{render.kind === 'preview' ? '低清预览' : '正式成片'}</b><span>该画面来自 Render Worker，不是浏览器临时拼接。</span><small>{shortId(render.id)} · {shortId(render.renderer_fingerprint)} · Asset v{asset.version}</small></div></section>
}

function C6ReviewPanel({ task, render, version, creativePackage, busy, onSubmit, onCheck, onApprove, onDeliver }: { task: ApiEditTask | null; render: ApiEditingRenderJob | null; version: ApiEditingCreativeVersion | null; creativePackage: ApiEditingCreativePackage | null; busy: boolean; onSubmit: () => void; onCheck: () => void; onApprove: () => void; onDeliver: () => void }) {
	const canSubmit = render?.kind === 'export' && render.status === 'succeeded' && task?.current_timeline?.version === render.timeline.version && !version
	return <section className="c6-review-panel"><header><b>成片检查与评审</b><span>{creativePackage ? '已交付' : version ? version.status : '未提交'}</span></header>
		<ol><li className={render?.kind === 'export' && render.status === 'succeeded' ? 'done' : ''}>正式导出</li><li className={version ? 'done' : ''}>冻结版本</li><li className={version?.check?.passed ? 'done' : ''}>规格检查</li><li className={version?.status === 'approved' ? 'done' : ''}>人工批准</li><li className={creativePackage ? 'done' : ''}>交付封包</li></ol>
		{canSubmit ? <button className="primary-button" disabled={busy} onClick={onSubmit}>提交评审</button> : null}
		{version?.status === 'created' ? <button className="primary-button" disabled={busy} onClick={onCheck}>执行成片检查</button> : null}
		{version?.status === 'checked' && version.check?.passed ? <button className="primary-button" disabled={busy} onClick={onApprove}>批准成片</button> : null}
		{version?.status === 'approved' && !creativePackage ? <button className="primary-button" disabled={busy} onClick={onDeliver}>创建交付包</button> : null}
		{version?.check && !version.check.passed ? <div className="c6-blockers"><b>检查未通过</b>{version.check.blockers.map(item => <small key={item}>{item}</small>)}</div> : null}
		{version?.video_snapshot.editing ? <details><summary>查看完整血缘</summary><dl><dt>Timeline</dt><dd>v{version.video_snapshot.editing.timeline_version} · {shortId(version.video_snapshot.editing.timeline_hash)}</dd><dt>Compiler</dt><dd>{version.video_snapshot.editing.compiler_version}</dd><dt>Renderer</dt><dd>{shortId(version.video_snapshot.editing.renderer_fingerprint)}</dd><dt>输入素材</dt><dd>{version.video_snapshot.editing.input_assets.map(ref => `${shortId(ref.asset_id)} v${ref.version}`).join('、')}</dd><dt>输出</dt><dd>{shortId(version.video_snapshot.editing.output_asset.asset_id)} v{version.video_snapshot.editing.output_asset.version}</dd></dl></details> : null}
	</section>
}

function editTaskStatusLabel(status?: ApiEditTask['status']) {
	return ({ draft: '草稿', rendering: '渲染中', review_ready: '待评审', completed: '已完成', failed: '失败', archived: '已归档' } as const)[status ?? 'draft']
}

function VisualCanvas({ document, selectedClipId, playheadMs, playing, onTogglePlaying, onSelect, onTransform }: { document: VisualEditorDocument; selectedClipId: string; playheadMs: number; playing: boolean; onTogglePlaying: () => void; onSelect: (id: string) => void; onTransform: (id: string, patch: Partial<VisualClip['transform']>) => void }) {
  const profile = canvasForProfile(document.canvasProfileId)
  const active = document.tracks.flatMap(track => track.hidden ? [] : track.clips.filter(clip => playheadMs >= clip.timelineStartMs && playheadMs < clip.timelineEndMs).map(clip => ({ clip, z: track.zIndex, muted: track.muted }))).sort((a, b) => a.z - b.z)
  const caption = document.captions.captions.find(item => playheadMs >= item.timelineStartMs && playheadMs < item.timelineEndMs)
  return <section className="c3-preview"><div className="c3-canvas" style={{ aspectRatio: `${profile.width}/${profile.height}`, background: document.background }} data-profile={document.canvasProfileId}>
    {active.map(({ clip, z, muted }) => <VisualLayer key={clip.id} clip={clip} z={z} muted={muted} selected={clip.id === selectedClipId} playheadMs={playheadMs} playing={playing} onSelect={onSelect} onTransform={onTransform}/>) }
    {caption ? <div className="c4-caption-preview">{renderCaptionPreview(caption)}</div> : null}
    <div className="c3-safe-area"/><span className="c3-profile-label">{document.canvasProfileId.startsWith('vertical') ? '9:16' : document.canvasProfileId.startsWith('landscape') ? '16:9' : '1:1'} 安全区</span>
  </div><button className="c3-preview-play" aria-label={playing ? '暂停时间线预览' : '播放时间线预览'} onClick={onTogglePlaying}>{playing ? <Pause size={17} fill="currentColor"/> : <Play size={17} fill="currentColor"/>}</button><time>{formatTime(playheadMs)} / {formatTime(document.durationMs)}</time></section>
}

function VisualLayer({ clip, z, muted, selected, playheadMs, playing, onSelect, onTransform }: { clip: VisualClip; z: number; muted: boolean; selected: boolean; playheadMs: number; playing: boolean; onSelect: (id: string) => void; onTransform: (id: string, patch: Partial<VisualClip['transform']>) => void }) {
  const mediaRef = useRef<HTMLVideoElement | null>(null)
  const dragRef = useRef<{ x: number; y: number; positionX: number; positionY: number } | null>(null)
  const resizeRef = useRef<{ x: number; scale: number } | null>(null)
  useEffect(() => { const video = mediaRef.current; if (video) { const target = (clip.sourceInMs + playheadMs - clip.timelineStartMs) / 1000; if (Math.abs(video.currentTime - target) > 0.2) video.currentTime = Math.max(0, target) } }, [clip, playheadMs])
  useEffect(() => { const video = mediaRef.current; if (!video) return; if (playing) void video.play().catch(() => undefined); else video.pause() }, [playing])
  const style = { zIndex: z + 1, width: `${clip.transform.scale * 100}%`, height: `${clip.transform.scale * 100}%`, left: `${clip.transform.positionX * 100}%`, top: `${clip.transform.positionY * 100}%`, transform: `translate(${-clip.transform.positionX * 100}%, ${-clip.transform.positionY * 100}%)`, opacity: clip.transform.opacity, clipPath: `inset(${clip.transform.crop.top * 100}% ${clip.transform.crop.right * 100}% ${clip.transform.crop.bottom * 100}% ${clip.transform.crop.left * 100}%)` }
  const move = (event: ReactPointerEvent<HTMLButtonElement>) => { if (!dragRef.current) return; const bounds = event.currentTarget.parentElement?.getBoundingClientRect(); if (!bounds) return; onTransform(clip.id, { positionX: dragRef.current.positionX + (event.clientX - dragRef.current.x) / bounds.width, positionY: dragRef.current.positionY + (event.clientY - dragRef.current.y) / bounds.height }) }
  return <button className={`c3-visual-layer${selected ? ' selected' : ''}`} style={style} onClick={event => { event.stopPropagation(); onSelect(clip.id) }} onPointerDown={event => { if ((event.target as HTMLElement).closest('.c3-resize-handle')) return; event.currentTarget.setPointerCapture(event.pointerId); dragRef.current = { x: event.clientX, y: event.clientY, positionX: clip.transform.positionX, positionY: clip.transform.positionY } }} onPointerMove={move} onPointerUp={event => { dragRef.current = null; event.currentTarget.releasePointerCapture(event.pointerId) }}>
    {clip.kind === 'image' ? <img src={clip.previewUrl} alt={clip.name} style={{ objectFit: clip.transform.fit }}/> : <video ref={mediaRef} src={clip.previewUrl} muted playsInline preload="auto" style={{ objectFit: clip.transform.fit }}/>}<span className="c3-layer-name">{clip.name}</span>{selected ? <span className="c3-resize-handle" onPointerDown={event => { event.stopPropagation(); event.currentTarget.setPointerCapture(event.pointerId); resizeRef.current = { x: event.clientX, scale: clip.transform.scale } }} onPointerMove={event => { if (resizeRef.current) onTransform(clip.id, { scale: resizeRef.current.scale + (event.clientX - resizeRef.current.x) / 240 }) }} onPointerUp={() => { resizeRef.current = null }}/> : null}
  </button>
}

function VisualTimeline(props: { document: VisualEditorDocument; assets: VisualAsset[]; playheadMs: number; selectedClipId: string; selectedCaptionId: string; selectedAudioId: string; activeTrackId: string; zoom: number; snapEnabled: boolean; canUndo: boolean; canRedo: boolean; onSeek: (ms: number) => void; onSelect: (id: string) => void; onSelectCaption: (id: string) => void; onSelectAudio: (id: string) => void; onActiveTrack: (id: string) => void; onZoom: (value: number) => void; onToggleSnap: () => void; onUndo: () => void; onRedo: () => void; onDelete: () => void; onSplit: () => void; onMove: (id: string, trackId: string, atMs: number) => void; onInsert: (asset: VisualAsset, trackId: string, atMs: number) => void; onMoveAudio: (id: string, trackId: string, atMs: number) => void }) {
  const pxPerMs = 0.055 * props.zoom
  const laneWidth = Math.max(760, props.document.durationMs * pxPerMs + 160)
  const allEdges = [...props.document.tracks.flatMap(track => track.clips.flatMap(clip => [clip.timelineStartMs, clip.timelineEndMs])), ...props.document.captions.captions.flatMap(caption => [caption.timelineStartMs, caption.timelineEndMs]), ...audioClips(props.document.audio).flatMap(clip => [clip.timelineStartMs, clip.timelineEndMs])]
  const snap = (ms: number) => { if (!props.snapEnabled) return alignFrame(ms); const nearest = allEdges.reduce((best, edge) => Math.abs(edge - ms) < Math.abs(best - ms) ? edge : best, ms); return Math.abs(nearest - ms) * pxPerMs <= 8 ? nearest : alignFrame(ms) }
  const timeAt = (event: React.DragEvent<HTMLElement>) => { const lane = event.currentTarget.closest('.c3-timeline-scroll') as HTMLElement | null; if (!lane) return 0; return snap((event.clientX - lane.getBoundingClientRect().left + lane.scrollLeft - 92) / pxPerMs) }
  const hasSelection = !!(props.selectedClipId || props.selectedCaptionId || props.selectedAudioId)
  return <section className="c3-timeline"><div className="c3-timeline-tools"><button disabled={!props.canUndo} onClick={props.onUndo}><Undo2 size={13}/>撤销</button><button disabled={!props.canRedo} onClick={props.onRedo}><Redo2 size={13}/>重做</button><button onClick={props.onSplit} disabled={!hasSelection}><Scissors size={13}/>分割</button><button onClick={props.onDelete} disabled={!hasSelection}><Trash2 size={13}/>删除</button><button className={props.snapEnabled ? 'active' : ''} onClick={props.onToggleSnap}><Magnet size={13}/>吸附</button><label>缩放<input type="range" min="0.5" max="2.5" step="0.1" value={props.zoom} onChange={event => props.onZoom(Number(event.target.value))}/></label></div>
    <div className="c3-timeline-scroll" onClick={event => { if (!(event.target as HTMLElement).closest('.c3-track-clip')) { const bounds = event.currentTarget.getBoundingClientRect(); props.onSeek(snap((event.clientX - bounds.left + event.currentTarget.scrollLeft - 92) / pxPerMs)) } }}><div className="c3-timeline-canvas" style={{ width: laneWidth }}><div className="c3-ruler" style={{ marginLeft: 92 }}>{Array.from({ length: Math.ceil(props.document.durationMs / 5000) + 2 }, (_, i) => <span key={i} style={{ left: i * 5000 * pxPerMs }}>{i * 5}s</span>)}</div>
      {props.document.tracks.slice().reverse().map(track => <div key={track.id} className={`c3-track-row${props.activeTrackId === track.id ? ' active' : ''}`} onClick={() => props.onActiveTrack(track.id)} onDragOver={event => { event.preventDefault(); event.dataTransfer.dropEffect = event.dataTransfer.types.includes(ASSET_DRAG_MIME) ? 'copy' : 'move' }} onDrop={event => { event.preventDefault(); const at = timeAt(event); const encoded = event.dataTransfer.getData(ASSET_DRAG_MIME); if (encoded) { const value = JSON.parse(encoded) as { assetId: string; assetVersion: number }; const asset = props.assets.find(item => item.assetId === value.assetId && item.assetVersion === value.assetVersion); if (asset) props.onInsert(asset, track.id, at) } else { const id = event.dataTransfer.getData('application/x-cookies-visual-clip'); if (id) props.onMove(id, track.id, at) } }}><div className="c3-track-label"><b>{track.zIndex === 0 ? '主视频' : `叠加 ${track.zIndex}`}</b><small>Z{track.zIndex}{track.locked ? ' · 已锁' : ''}</small></div><div className="c3-track-lane" style={{ width: laneWidth - 92 }}>{track.clips.map(clip => <button key={clip.id} draggable={!track.locked} className={`c3-track-clip kind-${clip.kind}${props.selectedClipId === clip.id ? ' selected' : ''}`} style={{ left: clip.timelineStartMs * pxPerMs, width: Math.max(36, (clip.timelineEndMs - clip.timelineStartMs) * pxPerMs) }} onClick={event => { event.stopPropagation(); props.onSelect(clip.id); props.onSeek(clip.timelineStartMs) }} onDragStart={event => { event.dataTransfer.setData('application/x-cookies-visual-clip', clip.id); event.dataTransfer.effectAllowed = 'move' }}><span>{clip.kind === 'image' ? <ImageIcon size={11}/> : <Play size={11}/>}</span><b>{clip.name}</b><small>{formatTime(clip.timelineEndMs - clip.timelineStartMs)}</small></button>)}</div></div>)}
      <div className="c3-track-row c4-caption-row"><div className="c3-track-label"><b>字幕</b><small>{props.document.captions.language}</small></div><div className="c3-track-lane" style={{ width: laneWidth - 92 }}>{props.document.captions.captions.map(caption => <button key={caption.id} className={`c3-track-clip kind-caption${props.selectedCaptionId === caption.id ? ' selected' : ''}`} style={{ left: caption.timelineStartMs * pxPerMs, width: Math.max(36, (caption.timelineEndMs - caption.timelineStartMs) * pxPerMs) }} onClick={event => { event.stopPropagation(); props.onSelectCaption(caption.id); props.onSeek(caption.timelineStartMs) }}><Captions size={11}/><b>{caption.text}</b><small>{formatTime(caption.timelineEndMs - caption.timelineStartMs)}</small></button>)}</div></div>
      {props.document.audio.tracks.map(track => <div key={track.id} className="c3-track-row c5-audio-row" onDragOver={event => { event.preventDefault(); event.dataTransfer.dropEffect = 'move' }} onDrop={event => { event.preventDefault(); const id = event.dataTransfer.getData('application/x-cookies-audio-clip'); if (id) props.onMoveAudio(id, track.id, timeAt(event)) }}><div className="c3-track-label"><b>{audioRoleLabel(track.role)}</b><small>{track.muted ? '已静音' : '48kHz stereo'}</small></div><div className="c3-track-lane" style={{ width: laneWidth - 92 }}>{track.clips.map(clip => <button key={clip.id} draggable className={`c3-track-clip kind-audio role-${track.role}${props.selectedAudioId === clip.id ? ' selected' : ''}`} style={{ left: clip.timelineStartMs * pxPerMs, width: Math.max(36, (clip.timelineEndMs - clip.timelineStartMs) * pxPerMs) }} onClick={event => { event.stopPropagation(); props.onSelectAudio(clip.id); props.onSeek(clip.timelineStartMs) }} onDragStart={event => { event.dataTransfer.setData('application/x-cookies-audio-clip', clip.id); event.dataTransfer.effectAllowed = 'move' }}><Music2 size={11}/><b>{clip.name}</b><small>{clip.gainDb}dB · {formatTime(clip.timelineEndMs - clip.timelineStartMs)}</small></button>)}</div></div>)}
      <button className="c3-playhead" style={{ left: 92 + props.playheadMs * pxPerMs }} aria-label="播放头"><span/></button></div></div>
  </section>
}

function CaptionLibrary({ captions, selectedId, onSelect, onImport }: { captions: CaptionClip[]; selectedId: string; onSelect: (caption: CaptionClip) => void; onImport: (file?: File) => void }) {
  return <div className="c4-caption-library"><label className="video-editor-upload"><Upload size={14}/>导入 SRT / ASS<input type="file" accept=".srt,.ass,text/plain" onChange={event => { void onImport(event.target.files?.[0]); event.currentTarget.value = '' }}/></label><p className="c4-asr-note">自动字幕暂不可用：当前环境未配置 ASR 凭据。可先导入字幕并完整校对，不会伪造转写结果。</p><div className="c4-caption-list">{captions.map((caption, index) => <button key={caption.id} className={selectedId === caption.id ? 'active' : ''} onClick={() => onSelect(caption)}><span>{index + 1}</span><b>{caption.text}</b><small>{formatTime(caption.timelineStartMs)}–{formatTime(caption.timelineEndMs)}</small></button>)}</div></div>
}

function AudioLibrary({ assets, activeTrackId, uploading, onTrack, onAdd, onUpload }: { assets: ApiProjectMediaAsset[]; activeTrackId: string; uploading: boolean; onTrack: (id: string) => void; onAdd: (asset: ApiProjectMediaAsset) => void; onUpload: (file?: File) => void }) {
  return <div className="c5-audio-library"><label className="video-editor-upload"><Upload size={14}/>{uploading ? '上传中…' : '上传 WAV / MP3 / AAC'}<input type="file" accept="audio/wav,audio/mpeg,audio/aac,.wav,.mp3,.aac" disabled={uploading} onChange={event => { void onUpload(event.target.files?.[0]); event.currentTarget.value = '' }}/></label><div className="c5-role-tabs">{(['voiceover', 'music', 'sfx'] as AudioRole[]).map(role => <button key={role} className={activeTrackId === `audio-${role}` ? 'active' : ''} onClick={() => onTrack(`audio-${role}`)}>{audioRoleLabel(role)}</button>)}</div><p className="c5-audio-note">上传只进入素材箱；点击加入后才写入所选音轨。渲染统一转换为 48kHz stereo，并执行响度规范化。</p><div className="c5-audio-assets">{assets.map(asset => <article key={`${asset.id}:v${asset.version}`}><AudioWaveform url={asset.contentUrl}/><b>{assetLabel(asset)}</b><small>{(asset.durationSeconds ?? 0).toFixed(1)} 秒 · v{asset.version}</small><audio src={asset.contentUrl} controls preload="metadata"/><button onClick={() => onAdd(asset)}>加入{audioRoleLabel(activeTrackId.replace('audio-', '') as AudioRole)}轨</button></article>)}</div></div>
}

function AudioWaveform({ url }: { url: string }) {
  const [peaks, setPeaks] = useState<number[]>([])
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let cancelled = false
    const run = async () => {
      const response = await fetch(url, { credentials: 'include' })
      if (!response.ok) throw new Error('waveform source unavailable')
      const context = new AudioContext()
      try {
        const buffer = await context.decodeAudioData(await response.arrayBuffer())
        const samples = buffer.getChannelData(0)
        const size = Math.max(1, Math.floor(samples.length / 32))
        const values = Array.from({ length: 32 }, (_, bucket) => {
          let peak = 0
          for (let index = bucket * size; index < Math.min(samples.length, (bucket + 1) * size); index++) peak = Math.max(peak, Math.abs(samples[index]))
          return peak
        })
        if (!cancelled) setPeaks(values)
      } finally { void context.close() }
    }
    void run().catch(() => { if (!cancelled) setFailed(true) })
    return () => { cancelled = true }
  }, [url])
  if (failed) return <div className="c5-waveform-status">波形暂不可用，音频仍可预览和剪辑</div>
  if (!peaks.length) return <div className="c5-waveform-status">正在解析真实波形…</div>
  const maxPeak = Math.max(...peaks, .01)
  return <div className="c5-waveform">{peaks.map((peak, index) => <i key={index} style={{ height: `${Math.max(6, peak / maxPeak * 100)}%` }}/>)}</div>
}

function AudioInspector({ clip, onChange, onTrim }: { clip: AudioClip; onChange: (patch: Partial<Pick<AudioClip, 'gainDb' | 'fadeInMs' | 'fadeOutMs' | 'loop'>>) => void; onTrim: (start: number, end: number, sourceIn: number, sourceOut: number) => void }) {
  const duration = clip.timelineEndMs - clip.timelineStartMs
  return <div className="c5-audio-inspector"><div><span>当前音频</span><b>{clip.name}</b><small>{clip.assetId} · v{clip.assetVersion}</small></div><NumberControl label="增益 dB" value={clip.gainDb} min={-96} max={24} step={1} onChange={gainDb => onChange({ gainDb })}/><NumberControl label="淡入秒" value={clip.fadeInMs / 1000} min={0} max={duration / 1000} step={0.1} onChange={value => onChange({ fadeInMs: value * 1000 })}/><NumberControl label="淡出秒" value={clip.fadeOutMs / 1000} min={0} max={duration / 1000} step={0.1} onChange={value => onChange({ fadeOutMs: value * 1000 })}/><label className="c5-loop"><input type="checkbox" checked={clip.loop} onChange={event => onChange({ loop: event.target.checked })}/>素材不足时循环至片段结束</label><div className="c3-trim-actions"><button disabled={duration <= 500} onClick={() => onTrim(clip.timelineStartMs + 500, clip.timelineEndMs, clip.sourceInMs + 500, clip.sourceOutMs)}>左裁 0.5s</button><button disabled={duration <= 500} onClick={() => onTrim(clip.timelineStartMs, clip.timelineEndMs - 500, clip.sourceInMs, clip.sourceOutMs - 500)}>右裁 0.5s</button></div><div className="c5-loudness"><b>响度检查</b><span>导出目标：-16 LUFS · True Peak ≤ -1.5 dBTP</span><small>权威数值在低清预览或正式导出后由 FFmpeg loudnorm 生成；编辑器不伪造实时 LUFS。</small></div></div>
}

function AudioPlayback({ document, playheadMs, playing }: { document: VisualEditorDocument; playheadMs: number; playing: boolean }) {
  const channels = [
    ...document.tracks.flatMap(track => track.muted ? [] : track.clips.filter(clip => clip.kind === 'video' && clip.originalAudio.enabled).map(clip => ({ id: `original-${clip.id}`, url: clip.previewUrl, start: clip.timelineStartMs, end: clip.timelineEndMs, sourceIn: clip.sourceInMs, gain: clip.originalAudio.gainDb, fadeIn: clip.originalAudio.fadeInMs, fadeOut: clip.originalAudio.fadeOutMs, loop: false, muted: false }))),
    ...document.audio.tracks.flatMap(track => track.muted ? [] : track.clips.map(clip => ({ id: clip.id, url: clip.previewUrl, start: clip.timelineStartMs, end: clip.timelineEndMs, sourceIn: clip.sourceInMs, gain: clip.gainDb, fadeIn: clip.fadeInMs, fadeOut: clip.fadeOutMs, loop: clip.loop, muted: false }))),
  ]
  return <div className="c5-playback" aria-hidden="true">{channels.map(channel => <AudioChannel key={channel.id} {...channel} playheadMs={playheadMs} playing={playing}/>)}</div>
}

function AudioChannel({ url, start, end, sourceIn, gain, fadeIn, fadeOut, loop, playheadMs, playing }: { url: string; start: number; end: number; sourceIn: number; gain: number; fadeIn: number; fadeOut: number; loop: boolean; playheadMs: number; playing: boolean }) {
  const ref = useRef<HTMLAudioElement | null>(null)
  const active = playheadMs >= start && playheadMs < end
  const relative = Math.max(0, playheadMs - start)
  const fadeGain = fadeIn > 0 && relative < fadeIn ? relative / fadeIn : fadeOut > 0 && end - playheadMs < fadeOut ? (end - playheadMs) / fadeOut : 1
  useEffect(() => {
    const audio = ref.current
    if (!audio) return
    audio.volume = Math.min(1, Math.max(0, Math.pow(10, gain / 20) * fadeGain))
    const target = (sourceIn + relative) / 1000
    if (active && Math.abs(audio.currentTime - target) > .2) audio.currentTime = target
    if (playing && active) void audio.play().catch(() => undefined)
    else audio.pause()
  }, [active, fadeGain, gain, playing, relative, sourceIn])
  return <audio ref={ref} src={url} loop={loop} preload="auto"/>
}

function CaptionInspector({ caption, hasNext, onChange, onSplit, onMerge }: { caption: CaptionClip; hasNext: boolean; onChange: (patch: Partial<Omit<CaptionClip, 'id'>>) => void; onSplit: () => void; onMerge: () => void }) {
  const [keyword, setKeyword] = useState('')
  const emphasize = () => {
    const start = caption.text.indexOf(keyword)
    if (!keyword || start < 0) return
    onChange({ emphasis: [{ start: [...caption.text.slice(0, start)].length, end: [...caption.text.slice(0, start + keyword.length)].length }] })
  }
  return <div className="c4-caption-inspector"><div><span>当前字幕</span><b>{caption.styleId} · v{caption.styleVersion}</b></div><label>字幕文本<textarea rows={4} value={caption.text} onChange={event => onChange({ text: event.target.value })}/></label><div className="c4-time-fields"><label>开始（秒）<input type="number" min="0" step="0.033" value={(caption.timelineStartMs / 1000).toFixed(3)} onChange={event => onChange({ timelineStartMs: Number(event.target.value) * 1000 })}/></label><label>结束（秒）<input type="number" min="0.033" step="0.033" value={(caption.timelineEndMs / 1000).toFixed(3)} onChange={event => onChange({ timelineEndMs: Number(event.target.value) * 1000 })}/></label></div><label>品牌样式<select value={`${caption.styleId}:${caption.styleVersion}`} onChange={() => undefined}><option value="brand-default:1">品牌白字描边 · v1</option></select></label><label>关键词强调<div className="c4-keyword"><input value={keyword} onChange={event => setKeyword(event.target.value)} placeholder="输入本条字幕中的词"/><button disabled={!keyword || !caption.text.includes(keyword)} onClick={emphasize}>应用</button></div></label><div className="c4-caption-actions"><button disabled={[...caption.text].length < 2 || caption.timelineEndMs - caption.timelineStartMs < 100} onClick={onSplit}>从中间拆分</button><button disabled={!hasNext} onClick={onMerge}>与下一条合并</button></div><button className="c4-clear-emphasis" disabled={!caption.emphasis.length} onClick={() => onChange({ emphasis: [] })}>清除关键词强调</button><small>字幕保持在 7% 安全区内；预览和导出使用相同时间码与 ASS 转义规则。</small></div>
}

function renderCaptionPreview(caption: CaptionClip) {
  const runes = [...caption.text]
  const nodes: React.ReactNode[] = []
  let cursor = 0
  caption.emphasis.forEach((span, index) => {
    if (span.start > cursor) nodes.push(runes.slice(cursor, span.start).join(''))
    nodes.push(<strong key={`${caption.id}-emphasis-${index}`}>{runes.slice(span.start, span.end).join('')}</strong>)
    cursor = span.end
  })
  if (cursor < runes.length) nodes.push(runes.slice(cursor).join(''))
  return nodes
}

function TransformInspector({ clip, onChange, onTrim, onOriginalAudio }: { clip: VisualClip; onChange: (patch: Partial<VisualClip['transform']>) => void; onTrim: (start: number, end: number, sourceIn?: number, sourceOut?: number) => void; onOriginalAudio: (patch: Partial<VisualClip['originalAudio']>) => void }) {
  return <div className="c3-transform"><div><span>当前片段</span><b>{clip.name}</b><small>{clip.kind === 'image' ? '图片' : '视频'} · {shortId(clip.assetId)} · v{clip.assetVersion}</small></div><label>适配<select value={clip.transform.fit} onChange={event => onChange({ fit: event.target.value as 'contain' | 'cover' })}><option value="contain">完整显示 contain</option><option value="cover">铺满画布 cover</option></select></label>
    <NumberControl label="水平位置" value={clip.transform.positionX} min={0} max={1} step={0.01} onChange={positionX => onChange({ positionX })}/><NumberControl label="垂直位置" value={clip.transform.positionY} min={0} max={1} step={0.01} onChange={positionY => onChange({ positionY })}/><NumberControl label="缩放" value={clip.transform.scale} min={0.05} max={4} step={0.05} onChange={scale => onChange({ scale })}/><NumberControl label="透明度" value={clip.transform.opacity} min={0} max={1} step={0.05} onChange={opacity => onChange({ opacity })}/>
    <div className="c3-crop-grid">{(['left', 'top', 'right', 'bottom'] as const).map(side => <NumberControl key={side} label={`裁切 ${side}`} value={clip.transform.crop[side]} min={0} max={0.9} step={0.01} onChange={value => onChange({ crop: { ...clip.transform.crop, [side]: value } })}/>)}</div>
    <div className="c3-trim-actions"><button disabled={clip.timelineEndMs - clip.timelineStartMs <= 500} onClick={() => onTrim(clip.timelineStartMs + 500, clip.timelineEndMs, clip.kind === 'video' ? clip.sourceInMs + 500 : undefined, clip.kind === 'video' ? clip.sourceOutMs : undefined)}>左裁 0.5s</button><button disabled={clip.timelineEndMs - clip.timelineStartMs <= 500} onClick={() => onTrim(clip.timelineStartMs, clip.timelineEndMs - 500, clip.kind === 'video' ? clip.sourceInMs : undefined, clip.kind === 'video' ? clip.sourceOutMs - 500 : undefined)}>右裁 0.5s</button><button disabled={clip.kind === 'video' && clip.sourceInMs <= 0 || clip.timelineStartMs <= 0} onClick={() => onTrim(Math.max(0, clip.timelineStartMs - 500), clip.timelineEndMs, clip.kind === 'video' ? Math.max(0, clip.sourceInMs - 500) : undefined, clip.kind === 'video' ? clip.sourceOutMs : undefined)}>左扩</button><button disabled={clip.kind === 'video' && clip.sourceOutMs >= clip.sourceDurationMs} onClick={() => onTrim(clip.timelineStartMs, clip.timelineEndMs + 500, clip.kind === 'video' ? clip.sourceInMs : undefined, clip.kind === 'video' ? Math.min(clip.sourceDurationMs, clip.sourceOutMs + 500) : undefined)}>右扩</button></div>
    {clip.kind === 'video' ? <div className="c5-original-audio"><label><input type="checkbox" checked={clip.originalAudio.enabled} onChange={event => onOriginalAudio({ enabled: event.target.checked })}/>保留原视频声音</label><NumberControl label="原声增益" value={clip.originalAudio.gainDb} min={-96} max={24} step={1} onChange={gainDb => onOriginalAudio({ gainDb })}/><NumberControl label="淡入秒" value={clip.originalAudio.fadeInMs / 1000} min={0} max={(clip.timelineEndMs - clip.timelineStartMs) / 1000} step={0.1} onChange={value => onOriginalAudio({ fadeInMs: value * 1000 })}/><NumberControl label="淡出秒" value={clip.originalAudio.fadeOutMs / 1000} min={0} max={(clip.timelineEndMs - clip.timelineStartMs) / 1000} step={0.1} onChange={value => onOriginalAudio({ fadeOutMs: value * 1000 })}/></div> : null}
  </div>
}

function NumberControl({ label, value, min, max, step, onChange }: { label: string; value: number; min: number; max: number; step: number; onChange: (value: number) => void }) { return <label className="c3-number"><span>{label}</span><input type="range" value={value} min={min} max={max} step={step} onChange={event => onChange(Number(event.target.value))}/><input type="number" value={Number(value.toFixed(2))} min={min} max={max} step={step} onChange={event => onChange(Number(event.target.value))}/></label> }
function TrackState({ track, active, onActivate, onChange }: { track: VisualTrack; active: boolean; onActivate: () => void; onChange: (patch: Partial<Pick<VisualTrack, 'locked' | 'muted' | 'hidden'>>) => void }) { return <div className={`c3-track-state${active ? ' active' : ''}`} onClick={onActivate}><span>Z{track.zIndex} · {track.role === 'primary' ? '主视频' : `叠加 ${track.zIndex}`}</span><div><button title="锁定轨道" onClick={event => { event.stopPropagation(); onChange({ locked: !track.locked }) }}>{track.locked ? <Lock size={13}/> : <Unlock size={13}/>}</button><button title="静音轨道" onClick={event => { event.stopPropagation(); onChange({ muted: !track.muted }) }}>{track.muted ? <VolumeX size={13}/> : <Volume2 size={13}/>}</button><button title="隐藏轨道" onClick={event => { event.stopPropagation(); onChange({ hidden: !track.hidden }) }}>{track.hidden ? <EyeOff size={13}/> : <Eye size={13}/>}</button></div></div> }
function toVisualAsset(asset: ApiProjectMediaAsset): VisualAsset { return { assetId: asset.id, assetVersion: asset.version, kind: asset.kind === 'image' ? 'image' : 'video', durationMs: asset.kind === 'image' ? 3000 : Math.max(250, Math.round((asset.durationSeconds ?? 0) * 1000)), name: assetLabel(asset), previewUrl: asset.contentUrl } }
function toAudioAsset(asset: ApiProjectMediaAsset): AudioAsset { return { kind: 'audio', assetId: asset.id, assetVersion: asset.version, durationMs: Math.max(250, Math.round((asset.durationSeconds ?? 0) * 1000)), name: assetLabel(asset), previewUrl: asset.contentUrl, waveformPeaks: [] } }
function assetLabel(asset: ApiProjectMediaAsset) { return `${asset.sourceType === 'upload' ? '导入' : asset.sourceType === 'rendered' ? '成片' : '项目'}${asset.kind === 'image' ? '图片' : asset.kind === 'audio' ? '音频' : '视频'} · ${shortId(asset.id)}` }
function audioRoleLabel(role: AudioRole) { return role === 'voiceover' ? '配音' : role === 'music' ? '音乐' : '音效' }
function saveStateLabel(value: SaveState) { return value === 'clean' ? '已保存' : value === 'dirty' ? '有未保存修改' : value === 'saving' ? '保存中…' : value === 'conflict' ? '版本冲突' : '保存失败' }
function formatTime(ms: number) { const seconds = Math.max(0, ms) / 1000; return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${(seconds % 60).toFixed(1).padStart(4, '0')}` }
