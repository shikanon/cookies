import type { EditingTimeline, EditingTimelineV2 } from './api'
import { createCaptionDocument, type CaptionDocument } from './captionDocument'
import { createEmptyAudioDocument, type AudioAsset, type AudioDocument } from './audioDocument'

export type CanvasProfileId = 'vertical-720p-v1' | 'landscape-720p-v1' | 'square-1080-v1'
export type VisualMediaKind = 'video' | 'image'

export type VisualTransform = {
  fit: 'contain' | 'cover'
  positionX: number
  positionY: number
  scale: number
  crop: { left: number; top: number; right: number; bottom: number }
  opacity: number
}

export type VisualAsset = {
  assetId: string
  assetVersion: number
  kind: VisualMediaKind
  durationMs: number
  name: string
  previewUrl: string
}

export type VisualClip = VisualAsset & {
  id: string
  trackId: string
  timelineStartMs: number
  timelineEndMs: number
  sourceInMs: number
  sourceOutMs: number
  sourceDurationMs: number
  transform: VisualTransform
  originalAudio: { enabled: boolean; gainDb: number; fadeInMs: number; fadeOutMs: number }
}

export type VisualTrack = {
  id: string
  role: 'primary' | 'overlay'
  zIndex: 0 | 1 | 2
  muted: boolean
  locked: boolean
  hidden: boolean
  clips: VisualClip[]
}

export type VisualEditorDocument = {
  canvasProfileId: CanvasProfileId
  background: string
  durationMs: number
  tracks: [VisualTrack, VisualTrack, VisualTrack]
  captions: CaptionDocument
  audio: AudioDocument
}

export const C3_TRACK_IDS = ['video-primary', 'visual-overlay-1', 'visual-overlay-2'] as const

export function defaultVisualTransform(): VisualTransform {
  return { fit: 'contain', positionX: 0.5, positionY: 0.5, scale: 1, crop: { left: 0, top: 0, right: 0, bottom: 0 }, opacity: 1 }
}

export function createEmptyVisualDocument(): VisualEditorDocument {
  return {
    canvasProfileId: 'vertical-720p-v1', background: '#000000', durationMs: 0,
    tracks: [
      { id: C3_TRACK_IDS[0], role: 'primary', zIndex: 0, muted: false, locked: false, hidden: false, clips: [] },
      { id: C3_TRACK_IDS[1], role: 'overlay', zIndex: 1, muted: false, locked: false, hidden: false, clips: [] },
      { id: C3_TRACK_IDS[2], role: 'overlay', zIndex: 2, muted: false, locked: false, hidden: false, clips: [] },
    ], captions: createCaptionDocument(), audio: createEmptyAudioDocument(),
  }
}

export function visualClips(document: VisualEditorDocument): VisualClip[] {
  return document.tracks.flatMap(track => track.clips)
}

export function findVisualClip(document: VisualEditorDocument, clipId: string): VisualClip | undefined {
  for (const track of document.tracks) {
    const clip = track.clips.find(item => item.id === clipId)
    if (clip) return clip
  }
}

export function insertVisualAsset(document: VisualEditorDocument, asset: VisualAsset, trackId: string, atMs: number, durationMs = asset.kind === 'image' ? 3000 : asset.durationMs): VisualEditorDocument {
  const track = document.tracks.find(item => item.id === trackId)
  if (!track || track.locked || durationMs <= 0) return document
  const start = alignFrame(atMs)
  const duration = alignFrame(durationMs)
  const ordinal = visualClips(document).filter(clip => clip.assetId === asset.assetId && clip.assetVersion === asset.assetVersion).length + 1
  const clip: VisualClip = {
    ...asset, id: `clip-${asset.assetId}-v${asset.assetVersion}-${ordinal}`, trackId, timelineStartMs: start,
    timelineEndMs: start + duration, sourceInMs: 0, sourceOutMs: asset.kind === 'video' ? Math.min(duration, asset.durationMs) : 0,
    sourceDurationMs: asset.durationMs, transform: defaultVisualTransform(), originalAudio: { enabled: asset.kind === 'video', gainDb: 0, fadeInMs: 0, fadeOutMs: 0 },
  }
  return normalizeVisualDocument({ ...document, tracks: document.tracks.map(item => item.id === trackId ? { ...item, clips: [...item.clips, clip] } : item) as VisualEditorDocument['tracks'] })
}

export function moveVisualClip(document: VisualEditorDocument, clipId: string, targetTrackId: string, atMs: number): VisualEditorDocument {
  const source = document.tracks.find(track => track.clips.some(clip => clip.id === clipId))
  const target = document.tracks.find(track => track.id === targetTrackId)
  if (!source || !target || source.locked || target.locked) return document
  const clip = source.clips.find(item => item.id === clipId)
  if (!clip) return document
  const moved = { ...clip, trackId: target.id, timelineStartMs: alignFrame(atMs), timelineEndMs: alignFrame(atMs) + clip.timelineEndMs - clip.timelineStartMs }
  const tracks = document.tracks.map(track => {
    const without = track.clips.filter(item => item.id !== clipId)
    return track.id === target.id ? { ...track, clips: [...without, moved] } : { ...track, clips: without }
  }) as VisualEditorDocument['tracks']
  return normalizeVisualDocument({ ...document, tracks })
}

export function trimVisualClip(document: VisualEditorDocument, clipId: string, timelineStartMs: number, timelineEndMs: number, sourceInMs?: number, sourceOutMs?: number): VisualEditorDocument {
  const current = findVisualClip(document, clipId)
  if (!current || timelineEndMs <= timelineStartMs) return document
  if (current.kind === 'video' && (sourceInMs === undefined || sourceOutMs === undefined || sourceInMs < 0 || sourceOutMs > current.sourceDurationMs || sourceOutMs <= sourceInMs)) return document
  return mapVisualClip(document, clipId, clip => ({ ...clip, timelineStartMs: alignFrame(timelineStartMs), timelineEndMs: alignFrame(timelineEndMs), sourceInMs: sourceInMs ?? 0, sourceOutMs: sourceOutMs ?? 0 }))
}

export function splitVisualClip(document: VisualEditorDocument, clipId: string, atMs: number): VisualEditorDocument {
  const track = document.tracks.find(item => item.clips.some(clip => clip.id === clipId))
  const clip = findVisualClip(document, clipId)
  const split = alignFrame(atMs)
  if (!track || !clip || track.locked || split <= clip.timelineStartMs || split >= clip.timelineEndMs) return document
  const sourceSplit = clip.kind === 'video' ? clip.sourceInMs + split - clip.timelineStartMs : 0
  const left = { ...clip, id: `${clip.id}-a`, timelineEndMs: split, sourceOutMs: clip.kind === 'video' ? sourceSplit : 0 }
  const right = { ...clip, id: `${clip.id}-b`, timelineStartMs: split, sourceInMs: clip.kind === 'video' ? sourceSplit : 0 }
  return normalizeVisualDocument({ ...document, tracks: document.tracks.map(item => item.id === track.id ? { ...item, clips: item.clips.flatMap(value => value.id === clipId ? [left, right] : [value]) } : item) as VisualEditorDocument['tracks'] })
}

export function deleteVisualClip(document: VisualEditorDocument, clipId: string): VisualEditorDocument {
  const track = document.tracks.find(item => item.clips.some(clip => clip.id === clipId))
  if (!track || track.locked) return document
  return normalizeVisualDocument({ ...document, tracks: document.tracks.map(item => ({ ...item, clips: item.clips.filter(clip => clip.id !== clipId) })) as VisualEditorDocument['tracks'] })
}

export function updateVisualTransform(document: VisualEditorDocument, clipId: string, patch: Partial<VisualTransform>): VisualEditorDocument {
  return mapVisualClip(document, clipId, clip => ({ ...clip, transform: sanitizeTransform({ ...clip.transform, ...patch, crop: patch.crop ? { ...patch.crop } : clip.transform.crop }) }))
}

export function updateOriginalAudio(document: VisualEditorDocument, clipId: string, patch: Partial<VisualClip['originalAudio']>): VisualEditorDocument {
  return mapVisualClip(document, clipId, clip => clip.kind === 'video' ? { ...clip, originalAudio: { ...clip.originalAudio, ...patch } } : clip)
}

export function setVisualTrackState(document: VisualEditorDocument, trackId: string, patch: Partial<Pick<VisualTrack, 'locked' | 'muted' | 'hidden'>>): VisualEditorDocument {
  return { ...document, tracks: document.tracks.map(track => track.id === trackId ? { ...track, ...patch } : track) as VisualEditorDocument['tracks'] }
}

export function setCanvasProfile(document: VisualEditorDocument, canvasProfileId: CanvasProfileId): VisualEditorDocument {
  return { ...document, canvasProfileId }
}

export function toTimelineV2(document: VisualEditorDocument): EditingTimelineV2 {
  const canvas = canvasForProfile(document.canvasProfileId)
  return {
    schema_version: 'editing-timeline/v2', timebase: { frame_rate_num: 30, frame_rate_den: 1 },
    canvas: { profile_id: document.canvasProfileId, ...canvas, sample_rate: 48000, background: { type: 'color', value: document.background } },
    duration_frames: Math.max(1, msToFrame(document.durationMs)),
    tracks: [...document.tracks.map(track => ({
      id: track.id, kind: 'visual' as const, role: track.role, z_index: track.zIndex, muted: track.muted, locked: track.locked, hidden: track.hidden,
      clips: track.clips.map(clip => ({
        id: clip.id, kind: clip.kind, asset_ref: { asset_id: clip.assetId, version: clip.assetVersion },
        timeline: { start_frame: msToFrame(clip.timelineStartMs), duration_frames: Math.max(1, msToFrame(clip.timelineEndMs - clip.timelineStartMs)) },
        ...(clip.kind === 'video' ? { source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 } } : {}),
        transform: { fit: clip.transform.fit, position_x: clip.transform.positionX, position_y: clip.transform.positionY, scale: clip.transform.scale, crop: clip.transform.crop, opacity: clip.transform.opacity },
		...(clip.kind === 'video' ? { original_audio: { enabled: clip.originalAudio.enabled, gain_db: clip.originalAudio.gainDb, fade_in_frames: msToFrame(clip.originalAudio.fadeInMs), fade_out_frames: msToFrame(clip.originalAudio.fadeOutMs) } } : {}),
      })),
    })), {
      id: document.captions.trackId, kind: 'caption' as const, language: document.captions.language,
      clips: document.captions.captions.map(caption => ({
        id: caption.id, kind: 'caption' as const,
        timeline: { start_frame: msToFrame(caption.timelineStartMs), duration_frames: Math.max(1, msToFrame(caption.timelineEndMs - caption.timelineStartMs)) },
        text: caption.text, style_ref: { style_id: caption.styleId, version: caption.styleVersion },
        emphasis: caption.emphasis.map(span => ({ start_rune: span.start, end_rune: span.end })),
      })),
    }, ...document.audio.tracks.map(track => ({ id: track.id, kind: 'audio' as const, role: track.role, muted: track.muted, clips: track.clips.map(clip => ({
	  id: clip.id, kind: 'audio' as const, asset_ref: { asset_id: clip.assetId, version: clip.assetVersion },
	  timeline: { start_frame: msToFrame(clip.timelineStartMs), duration_frames: Math.max(1, msToFrame(clip.timelineEndMs - clip.timelineStartMs)) },
	  source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 }, gain_db: clip.gainDb,
	  fade_in_frames: msToFrame(clip.fadeInMs), fade_out_frames: msToFrame(clip.fadeOutMs), loop: clip.loop,
	})) }))],
  }
}

export function restoreVisualDocument(timeline: EditingTimeline, assets: Array<VisualAsset | AudioAsset>): VisualEditorDocument {
	const byRef = new Map(assets.filter((asset): asset is VisualAsset => asset.kind !== 'audio').map(asset => [`${asset.assetId}:v${asset.assetVersion}`, asset]))
  const empty = createEmptyVisualDocument()
  if (timeline.schema_version === 'editing-timeline/v1') {
    const source = timeline.tracks.find(track => track.role === 'primary_video')
    const clips = (source?.clips ?? []).map(clip => {
      if (!clip.asset_ref) throw new Error(`片段 ${clip.id} 缺少素材引用`)
      const asset = byRef.get(`${clip.asset_ref.asset_id}:v${clip.asset_ref.version}`)
      if (!asset) throw new Error(`片段 ${clip.id} 引用的素材版本不可用`)
      return { ...asset, kind: 'video' as const, id: clip.id, trackId: C3_TRACK_IDS[0], timelineStartMs: clip.timeline_start_ms, timelineEndMs: clip.timeline_end_ms, sourceInMs: clip.source_in_ms ?? 0, sourceOutMs: clip.source_out_ms ?? clip.timeline_end_ms - clip.timeline_start_ms, sourceDurationMs: asset.durationMs, transform: defaultVisualTransform(), originalAudio: { enabled: true, gainDb: 0, fadeInMs: 0, fadeOutMs: 0 } }
    })
    return { ...empty, durationMs: timeline.duration_ms, tracks: [{ ...empty.tracks[0], clips }, empty.tracks[1], empty.tracks[2]] }
  }
  const tracks = [...timeline.tracks.filter(track => track.kind === 'visual')].sort((a, b) => (a.z_index ?? 0) - (b.z_index ?? 0)).slice(0, 3).map((track, index) => ({
    id: track.id, role: index === 0 ? 'primary' as const : 'overlay' as const, zIndex: index as 0 | 1 | 2,
    muted: track.muted ?? false, locked: track.locked ?? false, hidden: track.hidden ?? false,
    clips: track.clips.filter(clip => clip.kind === 'video' || clip.kind === 'image').map(clip => {
      if (!clip.asset_ref) throw new Error(`片段 ${clip.id} 缺少素材引用`)
      const asset = byRef.get(`${clip.asset_ref.asset_id}:v${clip.asset_ref.version}`)
      if (!asset) throw new Error(`片段 ${clip.id} 引用的素材版本不可用`)
      const sourceInMs = Math.round((clip.source?.in_us ?? 0) / 1000)
      const sourceOutMs = Math.round((clip.source?.out_us ?? 0) / 1000)
      return { ...asset, kind: clip.kind as VisualMediaKind, id: clip.id, trackId: track.id, timelineStartMs: frameToMs(clip.timeline.start_frame), timelineEndMs: frameToMs(clip.timeline.start_frame + clip.timeline.duration_frames), sourceInMs, sourceOutMs, sourceDurationMs: asset.durationMs,
		originalAudio: clip.kind === 'video' ? { enabled: clip.original_audio?.enabled ?? true, gainDb: clip.original_audio?.gain_db ?? 0, fadeInMs: frameToMs(clip.original_audio?.fade_in_frames ?? 0), fadeOutMs: frameToMs(clip.original_audio?.fade_out_frames ?? 0) } : { enabled: false, gainDb: 0, fadeInMs: 0, fadeOutMs: 0 },
        transform: clip.transform ? { fit: clip.transform.fit, positionX: clip.transform.position_x, positionY: clip.transform.position_y, scale: clip.transform.scale, crop: { ...clip.transform.crop }, opacity: clip.transform.opacity } : defaultVisualTransform() }
    }),
  }))
  while (tracks.length < 3) {
    const index = tracks.length as 0 | 1 | 2
    tracks.push({ ...empty.tracks[index], clips: [] })
  }
  const captionTrack = timeline.tracks.find(track => track.kind === 'caption')
  const captions = createCaptionDocument((captionTrack?.clips ?? []).filter(clip => clip.kind === 'caption' && clip.text && clip.style_ref).map(clip => ({
    id: clip.id, text: clip.text ?? '', timelineStartMs: frameToMs(clip.timeline.start_frame), timelineEndMs: frameToMs(clip.timeline.start_frame + clip.timeline.duration_frames),
    styleId: clip.style_ref?.style_id ?? 'brand-default', styleVersion: clip.style_ref?.version ?? 1,
    emphasis: (clip.emphasis ?? []).map(span => ({ start: span.start_rune, end: span.end_rune })),
  })), captionTrack?.language ?? 'zh-CN', captionTrack?.id ?? 'captions-main')
  const emptyAudio = createEmptyAudioDocument()
	const audioAssets = new Map(assets.filter((asset): asset is AudioAsset => asset.kind === 'audio').map(asset => [`${asset.assetId}:v${asset.assetVersion}`, asset]))
  const audioTracks = (['voiceover', 'music', 'sfx'] as const).map((role, index) => {
	const track = timeline.tracks.find(item => item.kind === 'audio' && item.role === role)
	return { id: track?.id ?? emptyAudio.tracks[index].id, role, muted: track?.muted ?? false, clips: (track?.clips ?? []).filter(clip => clip.kind === 'audio' && clip.asset_ref).map(clip => {
	  const asset = audioAssets.get(`${clip.asset_ref?.asset_id}:v${clip.asset_ref?.version}`)
	  if (!asset) throw new Error(`音频 ${clip.id} 引用的素材版本不可用`)
	  return { ...asset, id: clip.id, trackId: track?.id ?? emptyAudio.tracks[index].id, timelineStartMs: frameToMs(clip.timeline.start_frame), timelineEndMs: frameToMs(clip.timeline.start_frame + clip.timeline.duration_frames), sourceInMs: Math.round((clip.source?.in_us ?? 0) / 1000), sourceOutMs: Math.round((clip.source?.out_us ?? 0) / 1000), gainDb: clip.gain_db ?? 0, fadeInMs: frameToMs(clip.fade_in_frames ?? 0), fadeOutMs: frameToMs(clip.fade_out_frames ?? 0), loop: clip.loop ?? false }
	}) }
  }) as AudioDocument['tracks']
  return { canvasProfileId: timeline.canvas.profile_id as CanvasProfileId, background: timeline.canvas.background.value, durationMs: frameToMs(timeline.duration_frames), tracks: tracks as VisualEditorDocument['tracks'], captions, audio: { tracks: audioTracks } }
}

function mapVisualClip(document: VisualEditorDocument, clipId: string, update: (clip: VisualClip) => VisualClip): VisualEditorDocument {
  const track = document.tracks.find(item => item.clips.some(clip => clip.id === clipId))
  if (!track || track.locked) return document
  return normalizeVisualDocument({ ...document, tracks: document.tracks.map(item => ({ ...item, clips: item.clips.map(clip => clip.id === clipId ? update(clip) : clip) })) as VisualEditorDocument['tracks'] })
}

function normalizeVisualDocument(document: VisualEditorDocument): VisualEditorDocument {
  const tracks = document.tracks.map(track => ({ ...track, clips: [...track.clips].sort((a, b) => a.timelineStartMs - b.timelineStartMs || a.id.localeCompare(b.id)) })) as VisualEditorDocument['tracks']
  const durationMs = Math.max(0, ...tracks.flatMap(track => track.clips.map(clip => clip.timelineEndMs)))
  return { ...document, tracks, durationMs }
}

function sanitizeTransform(value: VisualTransform): VisualTransform {
  const clamp = (n: number, min: number, max: number) => Math.min(max, Math.max(min, Number.isFinite(n) ? n : min))
  return { ...value, positionX: clamp(value.positionX, 0, 1), positionY: clamp(value.positionY, 0, 1), scale: clamp(value.scale, 0.05, 8), opacity: clamp(value.opacity, 0, 1), crop: { left: clamp(value.crop.left, 0, 0.95), top: clamp(value.crop.top, 0, 0.95), right: clamp(value.crop.right, 0, 0.95 - value.crop.left), bottom: clamp(value.crop.bottom, 0, 0.95 - value.crop.top) } }
}

export function canvasForProfile(profile: CanvasProfileId): { width: number; height: number } {
  if (profile === 'landscape-720p-v1') return { width: 1280, height: 720 }
  if (profile === 'square-1080-v1') return { width: 1080, height: 1080 }
  return { width: 720, height: 1280 }
}

export function alignFrame(ms: number): number { return Math.max(0, Math.round(Math.round(ms * 30 / 1000) * 1000 / 30)) }
function msToFrame(ms: number): number { return Math.round(ms * 30 / 1000) }
function frameToMs(frame: number): number { return Math.round(frame * 1000 / 30) }
