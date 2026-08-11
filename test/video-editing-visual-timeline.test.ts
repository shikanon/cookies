import assert from 'node:assert/strict'
import test from 'node:test'

import {
  createEmptyVisualDocument,
  deleteVisualClip,
  findVisualClip,
  insertVisualAsset,
  moveVisualClip,
  setCanvasProfile,
  setVisualTrackState,
  splitVisualClip,
  toTimelineV2,
  trimVisualClip,
  updateVisualTransform,
} from '../src/features/video-editing/visualTimeline'

const video = { assetId: 'video-1', assetVersion: 2, kind: 'video' as const, durationMs: 6000, name: '主视频', previewUrl: '/video.mp4' }
const image = { assetId: 'image-1', assetVersion: 1, kind: 'image' as const, durationMs: 0, name: '产品图', previewUrl: '/image.png' }

test('C3 visual document supports three tracks, gaps, images, transforms and profiles immutably', () => {
  const empty = createEmptyVisualDocument()
  const withVideo = insertVisualAsset(empty, video, 'video-primary', 0)
  const withImage = insertVisualAsset(withVideo, image, 'visual-overlay-1', 1000)
  const imageClip = withImage.tracks[1].clips[0]
  const moved = moveVisualClip(withImage, imageClip.id, 'visual-overlay-2', 2500)
  const transformed = updateVisualTransform(moved, imageClip.id, { fit: 'cover', positionX: 0.2, positionY: 0.8, scale: 1.5, opacity: 0.6 })
  const square = setCanvasProfile(transformed, 'square-1080-v1')

  assert.equal(empty.durationMs, 0)
  assert.equal(withImage.tracks[1].clips.length, 1)
  assert.equal(moved.tracks[1].clips.length, 0)
  assert.equal(moved.tracks[2].clips[0].timelineStartMs, 2500)
  assert.deepEqual(findVisualClip(square, imageClip.id)?.transform, { fit: 'cover', positionX: 0.2, positionY: 0.8, scale: 1.5, crop: { left: 0, top: 0, right: 0, bottom: 0 }, opacity: 0.6 })
  const contract = toTimelineV2(square)
  assert.deepEqual(contract.canvas, { profile_id: 'square-1080-v1', width: 1080, height: 1080, sample_rate: 48000, background: { type: 'color', value: '#000000' } })
  assert.equal(contract.tracks[2].clips[0].kind, 'image')
  assert.equal(contract.tracks[2].clips[0].source, undefined)
})

test('C3 split, trim, delete and track lock preserve free timeline positions', () => {
  const initial = insertVisualAsset(createEmptyVisualDocument(), video, 'visual-overlay-1', 1000)
  const clip = initial.tracks[1].clips[0]
  const trimmed = trimVisualClip(initial, clip.id, 1500, 4500, 500, 3500)
  const split = splitVisualClip(trimmed, clip.id, 3000)
  assert.deepEqual(split.tracks[1].clips.map(item => [item.timelineStartMs, item.timelineEndMs, item.sourceInMs, item.sourceOutMs]), [[1500, 3000, 500, 2000], [3000, 4500, 2000, 3500]])

  const locked = setVisualTrackState(split, 'visual-overlay-1', { locked: true })
  assert.equal(deleteVisualClip(locked, split.tracks[1].clips[0].id), locked)
  const unlocked = setVisualTrackState(locked, 'visual-overlay-1', { locked: false, hidden: true, muted: true })
  assert.equal(deleteVisualClip(unlocked, split.tracks[1].clips[0].id).tracks[1].clips.length, 1)
})
