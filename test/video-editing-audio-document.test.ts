import assert from 'node:assert/strict'
import test from 'node:test'

import {
  createEmptyAudioDocument,
  deleteAudioClip,
  insertAudioAsset,
  moveAudioClip,
  setAudioTrackMuted,
  splitAudioClip,
  trimAudioClip,
  updateAudioClip,
  type AudioAsset,
} from '../src/features/video-editing/audioDocument'

const music: AudioAsset = { kind: 'audio', assetId: 'music-1', assetVersion: 2, durationMs: 4000, name: '品牌节奏', previewUrl: '/music.wav', waveformPeaks: [0.1, 0.8, 0.4] }

test('C5 audio document inserts and edits stable audio AssetVersions immutably', () => {
  const base = createEmptyAudioDocument()
  const inserted = insertAudioAsset(base, music, 'audio-music', 1000)
  const clip = inserted.tracks[1].clips[0]
  assert.equal(base.tracks[1].clips.length, 0)
  assert.deepEqual([clip.timelineStartMs, clip.timelineEndMs, clip.sourceInMs, clip.sourceOutMs], [1000, 5000, 0, 4000])
  const changed = updateAudioClip(inserted, clip.id, { gainDb: -12, fadeInMs: 500, fadeOutMs: 750, loop: true })
  assert.deepEqual([changed.tracks[1].clips[0].gainDb, changed.tracks[1].clips[0].fadeInMs, changed.tracks[1].clips[0].fadeOutMs, changed.tracks[1].clips[0].loop], [-12, 500, 767, true])
  assert.equal(setAudioTrackMuted(changed, 'audio-music', true).tracks[1].muted, true)
})

test('C5 audio document supports move trim split and delete without losing source ranges', () => {
  const inserted = insertAudioAsset(createEmptyAudioDocument(), music, 'audio-music', 0)
  const id = inserted.tracks[1].clips[0].id
  const trimmed = trimAudioClip(inserted, id, 500, 3000, 500, 3000)
  const moved = moveAudioClip(trimmed, id, 'audio-sfx', 2000)
  const split = splitAudioClip(moved, id, 3000)
  assert.deepEqual(split.tracks[2].clips.map(clip => [clip.timelineStartMs, clip.timelineEndMs, clip.sourceInMs, clip.sourceOutMs]), [[2000, 3000, 500, 1500], [3000, 4500, 1500, 3000]])
  assert.equal(deleteAudioClip(split, split.tracks[2].clips[0].id).tracks[2].clips.length, 1)
})
