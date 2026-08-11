import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { Magnet, Redo2, Scissors, Trash2, Undo2, ZoomIn, ZoomOut } from 'lucide-react'

import { alignToMasterFrame, findSnapTarget, type EditorClip, type EditorTimeline } from './timeline'
import { ASSET_DRAG_MIME } from './dragContract'

type TrimDraft = { clipId: string; sourceInMs: number; sourceOutMs: number } | null

export type VideoTimelineProps = {
  timeline: EditorTimeline
  selectedClipId: string
  playheadMs: number
  zoom: number
  canUndo: boolean
  canRedo: boolean
  snapEnabled: boolean
  onSelectClip: (clipId: string) => void
  onSeek: (timeMs: number) => void
  onZoomChange: (zoom: number) => void
  onMoveClip: (clipId: string, targetIndex: number) => void
  onInsertAsset: (assetId: string, assetVersion: number, targetIndex: number) => void
  onTrimClip: (clipId: string, sourceInMs: number, sourceOutMs: number) => void
  onSplitClip: () => void
  onDeleteClip: () => void
  onUndo: () => void
  onRedo: () => void
  onToggleSnap: () => void
}

const BASE_PIXELS_PER_SECOND = 44
const MIN_CLIP_DURATION_MS = 250
export function VideoTimeline({
  timeline,
  selectedClipId,
  playheadMs,
  zoom,
  canUndo,
  canRedo,
  snapEnabled,
  onSelectClip,
  onSeek,
  onZoomChange,
  onMoveClip,
  onInsertAsset,
  onTrimClip,
  onSplitClip,
  onDeleteClip,
  onUndo,
  onRedo,
  onToggleSnap,
}: VideoTimelineProps) {
  const laneRef = useRef<HTMLDivElement | null>(null)
  const trimSession = useRef<{ clip: EditorClip; side: 'left' | 'right'; startX: number } | null>(null)
  const trimDraftRef = useRef<TrimDraft>(null)
  const draggingPlayheadRef = useRef(false)
  const [trimDraft, setTrimDraft] = useState<TrimDraft>(null)
  const [draggingClipId, setDraggingClipId] = useState('')
  const [snapTargetMs, setSnapTargetMs] = useState<number | null>(null)
  const pixelsPerMs = BASE_PIXELS_PER_SECOND * zoom / 1000
  const laneWidth = Math.max(620, timeline.durationMs * pixelsPerMs + 80)
  const selectedClip = timeline.clips.find(clip => clip.id === selectedClipId)

  const displayClips = useMemo(() => timeline.clips.map(clip => {
    if (!trimDraft || trimDraft.clipId !== clip.id) return clip
    const durationMs = trimDraft.sourceOutMs - trimDraft.sourceInMs
    return {
      ...clip,
      sourceInMs: trimDraft.sourceInMs,
      sourceOutMs: trimDraft.sourceOutMs,
      timelineEndMs: clip.timelineStartMs + durationMs,
    }
  }), [timeline.clips, trimDraft])

  useEffect(() => {
    const handlePointerMove = (event: PointerEvent) => {
      const session = trimSession.current
      if (!session) return
      const deltaMs = Math.round((event.clientX - session.startX) / pixelsPerMs)
      if (session.side === 'left') {
        const sourceInMs = Math.max(0, Math.min(session.clip.sourceOutMs - MIN_CLIP_DURATION_MS, session.clip.sourceInMs + deltaMs))
        const draft = { clipId: session.clip.id, sourceInMs, sourceOutMs: session.clip.sourceOutMs }
        trimDraftRef.current = draft
        setTrimDraft(draft)
      } else {
        const sourceOutMs = Math.max(session.clip.sourceInMs + MIN_CLIP_DURATION_MS, Math.min(session.clip.sourceDurationMs, session.clip.sourceOutMs + deltaMs))
        const draft = { clipId: session.clip.id, sourceInMs: session.clip.sourceInMs, sourceOutMs }
        trimDraftRef.current = draft
        setTrimDraft(draft)
      }
    }
    const handlePointerUp = () => {
      const draft = trimDraftRef.current
      trimSession.current = null
      if (draft) onTrimClip(draft.clipId, draft.sourceInMs, draft.sourceOutMs)
      trimDraftRef.current = null
      setTrimDraft(null)
    }
    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', handlePointerUp)
    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', handlePointerUp)
    }
  }, [onTrimClip, pixelsPerMs])

  const startTrim = (event: ReactPointerEvent, clip: EditorClip, side: 'left' | 'right') => {
    event.preventDefault()
    event.stopPropagation()
    onSelectClip(clip.id)
    trimSession.current = { clip, side, startX: event.clientX }
    const draft = { clipId: clip.id, sourceInMs: clip.sourceInMs, sourceOutMs: clip.sourceOutMs }
    trimDraftRef.current = draft
    setTrimDraft(draft)
  }

  const seekFromPointer = (clientX: number) => {
    const lane = laneRef.current
    if (!lane) return
    const timeMs = (clientX - lane.getBoundingClientRect().left + lane.scrollLeft) / pixelsPerMs
    const bounded = Math.max(0, Math.min(timeline.durationMs, alignToMasterFrame(timeMs)))
    const candidates = timeline.clips.flatMap(clip => [clip.timelineStartMs, clip.timelineEndMs])
    const snap = snapEnabled ? findSnapTarget(bounded, candidates, 8, BASE_PIXELS_PER_SECOND * zoom) : null
    setSnapTargetMs(snap?.timeMs ?? null)
    onSeek(snap?.timeMs ?? bounded)
  }

  const insertionIndexFromPointer = (clientX: number) => {
    const lane = laneRef.current
    if (!lane) return timeline.clips.length
    const timeMs = (clientX - lane.getBoundingClientRect().left + lane.scrollLeft) / pixelsPerMs
    const index = timeline.clips.findIndex(clip => timeMs < (clip.timelineStartMs + clip.timelineEndMs) / 2)
    return index < 0 ? timeline.clips.length : index
  }

  useEffect(() => {
    const handlePointerMove = (event: PointerEvent) => {
      if (draggingPlayheadRef.current) seekFromPointer(event.clientX)
    }
    const handlePointerUp = () => { draggingPlayheadRef.current = false; setSnapTargetMs(null) }
    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', handlePointerUp)
    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', handlePointerUp)
    }
  }, [pixelsPerMs, timeline.durationMs])

  return <section className="real-video-timeline" aria-label="视频剪辑时间线">
    <div className="real-timeline-toolbar">
      <div className="timeline-edit-actions">
        <button type="button" onClick={onUndo} disabled={!canUndo} title="撤销 Ctrl+Z"><Undo2 size={14}/>撤销</button>
        <button type="button" onClick={onRedo} disabled={!canRedo} title="重做 Ctrl+Shift+Z"><Redo2 size={14}/>重做</button>
        <button type="button" onClick={() => onSeek(Math.max(0, playheadMs - 500))} disabled={!timeline.clips.length || playheadMs <= 0}>后退 0.5s</button>
        <button type="button" onClick={() => onSeek(Math.min(timeline.durationMs, playheadMs + 500))} disabled={!timeline.clips.length || playheadMs >= timeline.durationMs}>前进 0.5s</button>
        <button type="button" onClick={onSplitClip} disabled={!selectedClip || playheadMs <= selectedClip.timelineStartMs || playheadMs >= selectedClip.timelineEndMs}><Scissors size={14}/>在播放头分割</button>
        <button type="button" onClick={onDeleteClip} disabled={!selectedClip}><Trash2 size={14}/>删除片段</button>
        <button type="button" className={snapEnabled ? 'active' : ''} aria-pressed={snapEnabled} onClick={onToggleSnap} title="吸附到片段边界"><Magnet size={14}/>吸附</button>
      </div>
      <label className="timeline-zoom"><ZoomOut size={13}/><input aria-label="时间线缩放" type="range" min="0.55" max="2.4" step="0.05" value={zoom} onChange={event => onZoomChange(Number(event.target.value))}/><ZoomIn size={13}/><span>{Math.round(zoom * 100)}%</span></label>
    </div>
    <div className="timeline-scroll" ref={laneRef} onDragOver={event => {
      if (event.dataTransfer.types.includes(ASSET_DRAG_MIME)) {
        event.preventDefault()
        event.dataTransfer.dropEffect = 'copy'
      }
    }} onDrop={event => {
      const encoded = event.dataTransfer.getData(ASSET_DRAG_MIME)
      if (!encoded) return
      event.preventDefault()
      try {
        const value = JSON.parse(encoded) as { assetId?: string; assetVersion?: number }
        if (value.assetId && Number.isInteger(value.assetVersion)) onInsertAsset(value.assetId, Number(value.assetVersion), insertionIndexFromPointer(event.clientX))
      } catch { /* Ignore foreign drag payloads. */ }
    }} onClick={event => {
      if ((event.target as HTMLElement).closest('.real-timeline-clip')) return
      seekFromPointer(event.clientX)
    }}>
      <div className="timeline-canvas" style={{ width: laneWidth }}>
        <div className="timeline-ruler" aria-hidden="true">{Array.from({ length: Math.ceil(timeline.durationMs / 5000) + 1 }, (_, index) => <span key={index} style={{ left: index * 5000 * pixelsPerMs }}>{index * 5}s</span>)}</div>
        <div className="timeline-track-label">主视频轨</div>
        <div className="timeline-primary-lane">
          {snapTargetMs !== null ? <span className="timeline-snap-guide" style={{ left: snapTargetMs * pixelsPerMs }} aria-hidden="true"/> : null}
          {displayClips.map((clip, index) => <button
            type="button"
            key={clip.id}
            className={`real-timeline-clip${selectedClipId === clip.id ? ' selected' : ''}${draggingClipId === clip.id ? ' dragging' : ''}`}
            style={{ left: clip.timelineStartMs * pixelsPerMs, width: Math.max(44, (clip.sourceOutMs - clip.sourceInMs) * pixelsPerMs) }}
            draggable
            onClick={event => { event.stopPropagation(); onSelectClip(clip.id); onSeek(clip.timelineStartMs) }}
            onDragStart={event => { setDraggingClipId(clip.id); event.dataTransfer.setData('text/plain', clip.id); event.dataTransfer.effectAllowed = 'move' }}
            onDragEnd={() => setDraggingClipId('')}
            onDragOver={event => { event.preventDefault(); event.dataTransfer.dropEffect = 'move' }}
            onDrop={event => {
              event.preventDefault()
              const encoded = event.dataTransfer.getData(ASSET_DRAG_MIME)
              if (encoded) {
                try {
                  const value = JSON.parse(encoded) as { assetId?: string; assetVersion?: number }
                  if (value.assetId && Number.isInteger(value.assetVersion)) onInsertAsset(value.assetId, Number(value.assetVersion), index)
                } catch { /* Ignore foreign drag payloads. */ }
              } else {
                const sourceId = event.dataTransfer.getData('text/plain')
                if (sourceId) onMoveClip(sourceId, index)
              }
              setDraggingClipId('')
            }}
            aria-pressed={selectedClipId === clip.id}
            aria-label={`${clip.name}，${formatTime(clip.sourceOutMs - clip.sourceInMs)}`}
          >
            <span className="clip-trim-handle left" role="slider" aria-label={`${clip.name} 左裁切手柄`} onPointerDown={event => startTrim(event, clip, 'left')}/>
            <span className="clip-thumbnail" style={{ backgroundImage: `linear-gradient(90deg, rgba(7,22,40,.2), rgba(7,22,40,.8)), url(${JSON.stringify(clip.previewUrl).slice(1, -1)})` }}/>
            <span className="clip-copy"><b>{clip.name}</b><small>{formatTime(clip.sourceOutMs - clip.sourceInMs)} · v{clip.assetVersion}</small></span>
            <span className="clip-trim-handle right" role="slider" aria-label={`${clip.name} 右裁切手柄`} onPointerDown={event => startTrim(event, clip, 'right')}/>
          </button>)}
          {!timeline.clips.length ? <div className="timeline-empty">从左侧选择或拖入视频，开始剪辑</div> : null}
        </div>
        <button type="button" className="timeline-playhead" aria-label={`播放头 ${formatTime(playheadMs)}`} style={{ left: playheadMs * pixelsPerMs }} onPointerDown={event => { event.preventDefault(); event.stopPropagation(); draggingPlayheadRef.current = true; seekFromPointer(event.clientX) }}><span/></button>
      </div>
    </div>
    <footer className="timeline-status"><span>{timeline.clips.length} 个片段 · {formatTime(timeline.durationMs)}</span><span>拖素材加入 · 拖动排序 · 两端裁切 · {snapEnabled ? '吸附已开启' : '吸附已关闭'}</span></footer>
  </section>
}

function formatTime(timeMs: number): string {
  const totalSeconds = Math.max(0, timeMs) / 1000
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds - minutes * 60
  return `${String(minutes).padStart(2, '0')}:${seconds.toFixed(1).padStart(4, '0')}`
}
