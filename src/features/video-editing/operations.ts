import type { EditingAssetRef } from './api'
import type { EditorTimeline } from './timeline'
import type { VisualEditorDocument, VisualTransform } from './visualTimeline'

export type EditingOperation = {
  operation_id: string
  type: 'insert_asset' | 'move_clip' | 'trim_clip' | 'delete_clip' | 'update_visual_transform' | 'update_original_audio' | 'update_audio_clip' | 'set_track_muted' | 'set_track_hidden' | 'set_track_locked' | 'set_canvas_profile' | 'upsert_caption' | 'delete_caption'
  base_timeline_version: number
  actor: string
  track_id?: string
  target_track_id?: string
  clip_id: string
  asset_ref?: EditingAssetRef
  asset_kind?: 'video' | 'image' | 'audio'
  at_frame?: number
  duration_frames?: number
  source?: { in_us: number; out_us: number }
  timeline?: { start_frame: number; duration_frames: number }
  transform?: { fit: 'contain' | 'cover'; position_x: number; position_y: number; scale: number; crop: { left: number; top: number; right: number; bottom: number }; opacity: number }
  muted?: boolean
  hidden?: boolean
  locked?: boolean
  canvas_profile_id?: string
  text?: string
  style_ref?: { style_id: string; version: number }
  emphasis?: Array<{ start_rune: number; end_rune: number }>
  original_audio?: { enabled: boolean; gain_db: number; fade_in_frames: number; fade_out_frames: number }
  gain_db?: number
  fade_in_frames?: number
  fade_out_frames?: number
  loop?: boolean
}

export function buildVisualOperations(base: VisualEditorDocument, next: VisualEditorDocument, baseVersion: number, batchId: string): EditingOperationBatch {
  const operations: EditingOperation[] = []
  const postOperations: EditingOperation[] = []
  const common = { base_timeline_version: baseVersion, actor: '' }
  const baseClips = new Map(base.tracks.flatMap(track => track.clips.map(clip => [clip.id, { clip, trackId: track.id }] as const)))
  const nextClips = new Map(next.tracks.flatMap(track => track.clips.map(clip => [clip.id, { clip, trackId: track.id }] as const)))
  for (const track of next.tracks) {
    const previous = base.tracks.find(item => item.id === track.id)
    if (previous?.locked && !track.locked) operations.push({ ...common, operation_id: `${batchId}-unlock-${track.id}`, type: 'set_track_locked', track_id: track.id, clip_id: '', locked: false })
  }
  for (const [id] of baseClips) if (!nextClips.has(id)) operations.push({ ...common, operation_id: `${batchId}-delete-${id}`, type: 'delete_clip', clip_id: id })
  for (const [id, located] of nextClips) {
    const previous = baseClips.get(id)
    const clip = located.clip
    if (!previous) {
      operations.push({ ...common, operation_id: `${batchId}-insert-${id}`, type: 'insert_asset', track_id: located.trackId, clip_id: id, asset_kind: clip.kind,
        asset_ref: { asset_id: clip.assetId, version: clip.assetVersion }, at_frame: msToFrame(clip.timelineStartMs), duration_frames: msToFrame(clip.timelineEndMs - clip.timelineStartMs),
        ...(clip.kind === 'video' ? { source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 } } : {}), transform: serializeTransform(clip.transform) })
      continue
    }
    if (previous.trackId !== located.trackId || previous.clip.timelineStartMs !== clip.timelineStartMs) operations.push({ ...common, operation_id: `${batchId}-move-${id}`, type: 'move_clip', clip_id: id, target_track_id: located.trackId, at_frame: msToFrame(clip.timelineStartMs) })
    if (previous.clip.timelineEndMs - previous.clip.timelineStartMs !== clip.timelineEndMs - clip.timelineStartMs || previous.clip.sourceInMs !== clip.sourceInMs || previous.clip.sourceOutMs !== clip.sourceOutMs) operations.push({ ...common, operation_id: `${batchId}-trim-${id}`, type: 'trim_clip', clip_id: id, timeline: { start_frame: msToFrame(clip.timelineStartMs), duration_frames: msToFrame(clip.timelineEndMs - clip.timelineStartMs) }, ...(clip.kind === 'video' ? { source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 } } : {}) })
    if (JSON.stringify(previous.clip.transform) !== JSON.stringify(clip.transform)) operations.push({ ...common, operation_id: `${batchId}-transform-${id}`, type: 'update_visual_transform', clip_id: id, transform: serializeTransform(clip.transform) })
    if (clip.kind === 'video' && JSON.stringify(previous.clip.originalAudio) !== JSON.stringify(clip.originalAudio)) operations.push({ ...common, operation_id: `${batchId}-original-audio-${id}`, type: 'update_original_audio', clip_id: id, original_audio: { enabled: clip.originalAudio.enabled, gain_db: clip.originalAudio.gainDb, fade_in_frames: msToFrame(clip.originalAudio.fadeInMs), fade_out_frames: msToFrame(clip.originalAudio.fadeOutMs) } })
  }
  for (const track of next.tracks) {
    const previous = base.tracks.find(item => item.id === track.id)
    if (!previous) continue
    if (previous.muted !== track.muted) postOperations.push({ ...common, operation_id: `${batchId}-mute-${track.id}`, type: 'set_track_muted', track_id: track.id, clip_id: '', muted: track.muted })
    if (previous.hidden !== track.hidden) postOperations.push({ ...common, operation_id: `${batchId}-hide-${track.id}`, type: 'set_track_hidden', track_id: track.id, clip_id: '', hidden: track.hidden })
    if (!previous.locked && track.locked) postOperations.push({ ...common, operation_id: `${batchId}-lock-${track.id}`, type: 'set_track_locked', track_id: track.id, clip_id: '', locked: true })
  }
  if (base.canvasProfileId !== next.canvasProfileId) postOperations.push({ ...common, operation_id: `${batchId}-canvas`, type: 'set_canvas_profile', clip_id: '', canvas_profile_id: next.canvasProfileId })

  const baseCaptions = new Map(base.captions.captions.map(caption => [caption.id, caption]))
  const nextCaptions = new Map(next.captions.captions.map(caption => [caption.id, caption]))
  for (const [id] of baseCaptions) {
    if (!nextCaptions.has(id)) operations.push({ ...common, operation_id: `${batchId}-delete-caption-${id}`, type: 'delete_caption', clip_id: id })
  }
  for (const [id, caption] of nextCaptions) {
    const previous = baseCaptions.get(id)
    if (previous && JSON.stringify(previous) === JSON.stringify(caption)) continue
    operations.push({ ...common, operation_id: `${batchId}-upsert-caption-${id}`, type: 'upsert_caption', track_id: next.captions.trackId, clip_id: id,
      timeline: { start_frame: msToFrame(caption.timelineStartMs), duration_frames: Math.max(1, msToFrame(caption.timelineEndMs - caption.timelineStartMs)) },
      text: caption.text, style_ref: { style_id: caption.styleId, version: caption.styleVersion }, emphasis: caption.emphasis.map(span => ({ start_rune: span.start, end_rune: span.end })) })
  }

  const baseAudio = new Map(base.audio.tracks.flatMap(track => track.clips.map(clip => [clip.id, { clip, trackId: track.id }] as const)))
  const nextAudio = new Map(next.audio.tracks.flatMap(track => track.clips.map(clip => [clip.id, { clip, trackId: track.id }] as const)))
  for (const [id] of baseAudio) if (!nextAudio.has(id)) operations.push({ ...common, operation_id: `${batchId}-delete-audio-${id}`, type: 'delete_clip', clip_id: id })
  for (const [id, located] of nextAudio) {
	const previous = baseAudio.get(id)
	const clip = located.clip
	if (!previous) {
	  operations.push({ ...common, operation_id: `${batchId}-insert-audio-${id}`, type: 'insert_asset', track_id: located.trackId, clip_id: id, asset_kind: 'audio', asset_ref: { asset_id: clip.assetId, version: clip.assetVersion }, at_frame: msToFrame(clip.timelineStartMs), duration_frames: msToFrame(clip.timelineEndMs - clip.timelineStartMs), source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 }, gain_db: clip.gainDb, fade_in_frames: msToFrame(clip.fadeInMs), fade_out_frames: msToFrame(clip.fadeOutMs), loop: clip.loop })
	  continue
	}
	if (previous.trackId !== located.trackId || previous.clip.timelineStartMs !== clip.timelineStartMs) operations.push({ ...common, operation_id: `${batchId}-move-audio-${id}`, type: 'move_clip', clip_id: id, target_track_id: located.trackId, at_frame: msToFrame(clip.timelineStartMs) })
	if (previous.clip.timelineEndMs - previous.clip.timelineStartMs !== clip.timelineEndMs - clip.timelineStartMs || previous.clip.sourceInMs !== clip.sourceInMs || previous.clip.sourceOutMs !== clip.sourceOutMs) operations.push({ ...common, operation_id: `${batchId}-trim-audio-${id}`, type: 'trim_clip', clip_id: id, timeline: { start_frame: msToFrame(clip.timelineStartMs), duration_frames: msToFrame(clip.timelineEndMs - clip.timelineStartMs) }, source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 } })
	if (previous.clip.gainDb !== clip.gainDb || previous.clip.fadeInMs !== clip.fadeInMs || previous.clip.fadeOutMs !== clip.fadeOutMs || previous.clip.loop !== clip.loop) operations.push({ ...common, operation_id: `${batchId}-update-audio-${id}`, type: 'update_audio_clip', clip_id: id, gain_db: clip.gainDb, fade_in_frames: msToFrame(clip.fadeInMs), fade_out_frames: msToFrame(clip.fadeOutMs), loop: clip.loop })
  }
  for (const track of next.audio.tracks) {
	const previous = base.audio.tracks.find(item => item.id === track.id)
	if (previous && previous.muted !== track.muted) operations.push({ ...common, operation_id: `${batchId}-mute-audio-${track.id}`, type: 'set_track_muted', track_id: track.id, clip_id: '', muted: track.muted })
  }
  operations.push(...postOperations)
  return { batch_id: batchId, base_timeline_version: baseVersion, actor: '', operations }
}

function serializeTransform(transform: VisualTransform) {
  return { fit: transform.fit, position_x: transform.positionX, position_y: transform.positionY, scale: transform.scale, crop: { ...transform.crop }, opacity: transform.opacity }
}

export type EditingOperationBatch = {
  batch_id: string
  base_timeline_version: number
  actor: string
  operations: EditingOperation[]
}

export function buildPrimaryTrackOperations(base: EditorTimeline, next: EditorTimeline, baseVersion: number, batchId: string): EditingOperationBatch {
  const operations: EditingOperation[] = []
  const baseById = new Map(base.clips.map(clip => [clip.id, clip]))
  const nextById = new Map(next.clips.map(clip => [clip.id, clip]))
  const common = { base_timeline_version: baseVersion, actor: '' }
  for (const clip of base.clips) {
    if (!nextById.has(clip.id)) operations.push({ ...common, operation_id: `${batchId}-delete-${clip.id}`, type: 'delete_clip', clip_id: clip.id })
  }
  for (const clip of next.clips) {
    if (baseById.has(clip.id)) continue
    operations.push({ ...common, operation_id: `${batchId}-insert-${clip.id}`, type: 'insert_asset', track_id: 'video-primary', clip_id: clip.id,
      asset_ref: { asset_id: clip.assetId, version: clip.assetVersion }, at_frame: msToFrame(clip.timelineStartMs), duration_frames: msToFrame(clip.timelineEndMs - clip.timelineStartMs),
      source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 } })
  }
  for (const clip of next.clips) {
    const previous = baseById.get(clip.id)
    if (!previous) continue
    if (previous.timelineStartMs !== clip.timelineStartMs) operations.push({ ...common, operation_id: `${batchId}-move-${clip.id}`, type: 'move_clip', clip_id: clip.id, target_track_id: 'video-primary', at_frame: msToFrame(clip.timelineStartMs) })
    if (previous.sourceInMs !== clip.sourceInMs || previous.sourceOutMs !== clip.sourceOutMs || previous.timelineEndMs - previous.timelineStartMs !== clip.timelineEndMs - clip.timelineStartMs) {
      operations.push({ ...common, operation_id: `${batchId}-trim-${clip.id}`, type: 'trim_clip', clip_id: clip.id,
        source: { in_us: clip.sourceInMs * 1000, out_us: clip.sourceOutMs * 1000 }, timeline: { start_frame: msToFrame(clip.timelineStartMs), duration_frames: msToFrame(clip.timelineEndMs - clip.timelineStartMs) } })
    }
  }
  return { batch_id: batchId, base_timeline_version: baseVersion, actor: '', operations }
}

function msToFrame(value: number): number { return Math.round(value * 30 / 1000) }
