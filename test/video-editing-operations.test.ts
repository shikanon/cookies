import assert from 'node:assert/strict'
import test from 'node:test'
import { buildPrimaryTrackOperations, buildVisualOperations } from '../src/features/video-editing/operations'
import type { EditorTimeline } from '../src/features/video-editing/timeline'
import { createEmptyVisualDocument, insertVisualAsset, moveVisualClip, setCanvasProfile, setVisualTrackState, updateVisualTransform } from '../src/features/video-editing/visualTimeline'
import { createCaptionDocument, importSubtitleText, updateCaption } from '../src/features/video-editing/captionDocument'
import { insertAudioAsset, updateAudioClip } from '../src/features/video-editing/audioDocument'

test('editor changes become a deterministic operation batch against one base version', () => {
  const base: EditorTimeline = { durationMs: 2000, clips: [clip('a', 'asset-a', 0, 1000, 0, 1000), clip('b', 'asset-b', 1000, 2000, 0, 1000)] }
  const next: EditorTimeline = { durationMs: 2500, clips: [clip('b', 'asset-b', 0, 500, 100, 600), clip('c', 'asset-c', 500, 2500, 0, 2000)] }
  const batch = buildPrimaryTrackOperations(base, next, 7, 'batch-fixed')
  assert.equal(batch.base_timeline_version, 7)
  assert.deepEqual(batch.operations.map(operation => operation.type), ['delete_clip', 'insert_asset', 'move_clip', 'trim_clip'])
  assert.ok(batch.operations.every(operation => operation.base_timeline_version === 7))
})

test('C3 visual changes serialize across tracks with transform and canvas operations', () => {
  const asset = { assetId: 'image-a', assetVersion: 3, kind: 'image' as const, durationMs: 3000, name: '产品图', previewUrl: '' }
  const base = setVisualTrackState(insertVisualAsset(createEmptyVisualDocument(), asset, 'visual-overlay-1', 1000), 'visual-overlay-1', { locked: true })
  const id = base.tracks[1].clips[0].id
  const unlocked = setVisualTrackState(base, 'visual-overlay-1', { locked: false })
  const moved = moveVisualClip(unlocked, id, 'visual-overlay-2', 2500)
  const transformed = updateVisualTransform(moved, id, { scale: 1.4, opacity: 0.7 })
  const next = setCanvasProfile(transformed, 'square-1080-v1')
  const batch = buildVisualOperations(base, next, 9, 'batch-c3')
  assert.deepEqual(batch.operations.map(operation => operation.type), ['set_track_locked', 'move_clip', 'update_visual_transform', 'set_canvas_profile'])
  assert.equal(batch.operations[1].target_track_id, 'visual-overlay-2')
  assert.equal(batch.operations[2].transform?.scale, 1.4)
})

test('C4 caption changes serialize as stable upsert and delete operations', () => {
  const imported = importSubtitleText('captions.srt', '1\n00:00:01,000 --> 00:00:02,000\n品牌关键词\n')
  const base = { ...createEmptyVisualDocument(), captions: createCaptionDocument(imported.captions) }
  const changedCaptions = updateCaption(base.captions, base.captions.captions[0].id, { text: '品牌 关键词', emphasis: [{ start: 3, end: 6 }] })
  const next = { ...base, captions: changedCaptions }
  const batch = buildVisualOperations(base, next, 4, 'batch-c4')
  assert.equal(batch.operations.length, 1)
  assert.equal(batch.operations[0].type, 'upsert_caption')
  assert.deepEqual(batch.operations[0].style_ref, { style_id: 'brand-default', version: 1 })
  assert.deepEqual(batch.operations[0].emphasis, [{ start_rune: 3, end_rune: 6 }])
})

test('C5 audio changes serialize trim-safe gain fade loop and AssetVersion operations', () => {
  const base = createEmptyVisualDocument()
  const insertedAudio = insertAudioAsset(base.audio, { kind: 'audio', assetId: 'audio-1', assetVersion: 3, durationMs: 4000, name: '音乐', previewUrl: '/audio.wav', waveformPeaks: [] }, 'audio-music', 1000)
  const clip = insertedAudio.tracks[1].clips[0]
  const next = { ...base, audio: updateAudioClip(insertedAudio, clip.id, { gainDb: -18, fadeInMs: 500, fadeOutMs: 1000, loop: true }) }
  const batch = buildVisualOperations(base, next, 6, 'batch-c5')
  assert.equal(batch.operations.length, 1)
  assert.deepEqual(batch.operations[0], {
    operation_id: `batch-c5-insert-audio-${clip.id}`, type: 'insert_asset', base_timeline_version: 6, actor: '', track_id: 'audio-music', clip_id: clip.id,
    asset_kind: 'audio', asset_ref: { asset_id: 'audio-1', version: 3 }, at_frame: 30, duration_frames: 120,
    source: { in_us: 0, out_us: 4000000 }, gain_db: -18, fade_in_frames: 15, fade_out_frames: 30, loop: true,
  })
})

function clip(id: string, assetId: string, timelineStartMs: number, timelineEndMs: number, sourceInMs: number, sourceOutMs: number) {
  return { id, assetId, assetVersion: 1, name: id, previewUrl: '', timelineStartMs, timelineEndMs, sourceInMs, sourceOutMs, sourceDurationMs: 3000 }
}
