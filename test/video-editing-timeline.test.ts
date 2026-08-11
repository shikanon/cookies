import assert from 'node:assert/strict'
import test from 'node:test'

import { addClip, commitTimeline, createEmptyEditorTimeline, createTimelineHistory, deleteClip, findSnapTarget, insertClip, moveClip, redoTimeline, restoreEditorTimeline, splitClip, toEditingTimeline, trimClip, undoTimeline } from '../src/features/video-editing/timeline.ts'

test('user can add a project video to the end of the primary timeline', () => {
  const timeline = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-preroll',
    assetVersion: 3,
    durationMs: 5_000,
    name: '短剧前贴',
    previewUrl: '/assets/preroll/content',
  })

  assert.deepEqual(timeline.clips, [{
    id: 'clip-asset-preroll-v3-1',
    assetId: 'asset-preroll',
    assetVersion: 3,
    name: '短剧前贴',
    previewUrl: '/assets/preroll/content',
    timelineStartMs: 0,
    timelineEndMs: 5_000,
    sourceInMs: 0,
    sourceOutMs: 5_000,
    sourceDurationMs: 5_000,
  }])
  assert.equal(timeline.durationMs, 5_000)
})

test('dragging a project asset inserts it at the requested primary-track position', () => {
  const first = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 1, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })
  const inserted = insertClip(first, {
    assetId: 'asset-preroll', assetVersion: 1, durationMs: 5_000,
    name: '前贴', previewUrl: '/preroll',
  }, 0)

  assert.deepEqual(inserted.clips.map(clip => ({ assetId: clip.assetId, start: clip.timelineStartMs, end: clip.timelineEndMs })), [
    { assetId: 'asset-preroll', start: 0, end: 5_000 },
    { assetId: 'asset-source', start: 5_000, end: 25_000 },
  ])
})

test('saved timeline restores clip metadata from project assets', () => {
  const contract = toEditingTimeline(trimClip(addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  }), 'clip-asset-source-v2-1', 2_000, 18_000))

  const restored = restoreEditorTimeline(contract, [{
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '恢复后的原视频', previewUrl: '/signed/source',
  }])

  assert.equal(restored.clips[0].name, '恢复后的原视频')
  assert.equal(restored.clips[0].previewUrl, '/signed/source')
  assert.equal(restored.clips[0].sourceDurationMs, 20_000)
  assert.equal(restored.durationMs, 16_000)
})

test('editor state serializes to the authoritative editing-timeline v1 contract', () => {
  const added = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })
  const timeline = trimClip(added, added.clips[0].id, 2_000, 18_000)

  assert.deepEqual(toEditingTimeline(timeline), {
    schema_version: 'editing-timeline/v1',
    output_profile: { id: 'cookies-editing-vertical-v1', width: 720, height: 1280, frame_rate: 30, sample_rate: 48000 },
    duration_ms: 16_000,
    tracks: [{
      id: 'video-primary',
      role: 'primary_video',
      clips: [{
        id: 'clip-asset-source-v2-1',
        asset_ref: { asset_id: 'asset-source', version: 2 },
        timeline_start_ms: 0,
        timeline_end_ms: 16_000,
        source_in_ms: 2_000,
        source_out_ms: 18_000,
      }],
    }],
  })
})

test('user can undo and redo an editing operation', () => {
  const timeline = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 1, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })
  const history = commitTimeline(createTimelineHistory(timeline), deleteClip(timeline, timeline.clips[0].id))

  const undone = undoTimeline(history)
  assert.equal(undone.present.clips.length, 1)
  assert.equal(undone.future.length, 1)

  const redone = redoTimeline(undone)
  assert.equal(redone.present.clips.length, 0)
  assert.equal(redone.past.length, 1)
})

test('user can delete a clip and remaining clips close the gap', () => {
  const first = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-preroll', assetVersion: 1, durationMs: 5_000,
    name: '前贴', previewUrl: '/preroll',
  })
  const second = addClip(first, {
    assetId: 'asset-source', assetVersion: 1, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })

  const deleted = deleteClip(second, 'clip-asset-preroll-v1-1')

  assert.equal(deleted.clips.length, 1)
  assert.equal(deleted.clips[0].timelineStartMs, 0)
  assert.equal(deleted.durationMs, 20_000)
})

test('user can split a clip at the playhead and preserve adjacent source ranges', () => {
  const timeline = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })

  const split = splitClip(timeline, 'clip-asset-source-v2-1', 8_000)

  assert.deepEqual(split.clips.map(clip => ({
    id: clip.id,
    start: clip.timelineStartMs,
    end: clip.timelineEndMs,
    sourceIn: clip.sourceInMs,
    sourceOut: clip.sourceOutMs,
  })), [
    { id: 'clip-asset-source-v2-1-a', start: 0, end: 8_000, sourceIn: 0, sourceOut: 8_000 },
    { id: 'clip-asset-source-v2-1-b', start: 8_000, end: 20_000, sourceIn: 8_000, sourceOut: 20_000 },
  ])
})

test('split aligns the playhead to the fixed 30fps master frame', () => {
  const timeline = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })

  const split = splitClip(timeline, timeline.clips[0].id, 8_016)

  assert.equal(split.clips[0].timelineEndMs, 8_000)
  assert.equal(split.clips[1].sourceInMs, 8_000)
})

test('repeated split creates stable unique clip identities', () => {
  const timeline = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })
  const first = splitClip(timeline, timeline.clips[0].id, 10_000)
  const second = splitClip(first, first.clips[0].id, 5_000)
  const third = splitClip(second, second.clips[1].id, 7_500)

  assert.equal(new Set(third.clips.map(clip => clip.id)).size, third.clips.length)
})

test('snap finds the nearest clip edge or playhead inside the pixel threshold', () => {
  assert.deepEqual(findSnapTarget(4_948, [0, 5_000, 12_000], 6, 100), { timeMs: 5_000, distancePx: 5.2 })
  assert.equal(findSnapTarget(4_900, [0, 5_000, 12_000], 6, 100), null)
})

test('user can trim a clip without changing its source asset', () => {
  const timeline = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })

  const trimmed = trimClip(timeline, 'clip-asset-source-v2-1', 2_000, 18_000)

  assert.deepEqual(trimmed.clips[0], {
    ...timeline.clips[0],
    sourceInMs: 2_000,
    sourceOutMs: 18_000,
    timelineEndMs: 16_000,
  })
  assert.equal(trimmed.durationMs, 16_000)
})

test('user can reorder clips and the primary timeline stays contiguous', () => {
  const first = addClip(createEmptyEditorTimeline(), {
    assetId: 'asset-preroll', assetVersion: 1, durationMs: 5_000,
    name: '前贴', previewUrl: '/preroll',
  })
  const second = addClip(first, {
    assetId: 'asset-source', assetVersion: 2, durationMs: 20_000,
    name: '原视频', previewUrl: '/source',
  })

  const moved = moveClip(second, 'clip-asset-source-v2-1', 0)

  assert.deepEqual(moved.clips.map(clip => ({ id: clip.id, start: clip.timelineStartMs, end: clip.timelineEndMs })), [
    { id: 'clip-asset-source-v2-1', start: 0, end: 20_000 },
    { id: 'clip-asset-preroll-v1-1', start: 20_000, end: 25_000 },
  ])
})
