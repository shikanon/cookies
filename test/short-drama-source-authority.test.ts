import assert from 'node:assert/strict'
import test from 'node:test'

import { findAuthoritativeVideo, requireAuthoritativeVideo } from '../src/features/short-drama-preroll-v2/sourceAuthority'

const videos = [
  { id: 'asset_current', version: 2, kind: 'video' as const },
  { id: 'asset_other', version: 1, kind: 'video' as const },
]

test('short-drama restore accepts only the exact project asset version bound to the task', () => {
  assert.equal(findAuthoritativeVideo(videos, { asset_id: 'asset_current', version: 2 }), videos[0])
  assert.equal(findAuthoritativeVideo(videos, { asset_id: 'asset_current', version: 1 }), null)
  assert.equal(findAuthoritativeVideo(videos, { asset_id: 'asset_missing', version: 1 }), null)
})

test('short-drama upload never fabricates a browser-local asset when persistence is not observable', () => {
  assert.throws(
    () => requireAuthoritativeVideo(videos, { asset_id: 'asset_missing', version: 1 }),
    /后端尚未确认该视频素材/,
  )
})
