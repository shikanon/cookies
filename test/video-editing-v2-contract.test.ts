import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import Ajv2020 from 'ajv/dist/2020.js'

test('editing timeline v2 fixture satisfies the frozen JSON Schema', () => {
  const schema = JSON.parse(readFileSync('api/contracts/creative-editing-timeline-v2.schema.json', 'utf8'))
  const validate = new Ajv2020({ allErrors: true, strict: true }).compile(schema)
  const fixture = { schema_version: 'editing-timeline/v2', timebase: { frame_rate_num: 30, frame_rate_den: 1 }, canvas: { profile_id: 'vertical-720p-v1', width: 720, height: 1280, sample_rate: 48000, background: { type: 'color', value: '#000000' } }, duration_frames: 90,
    tracks: [{ id: 'visual-primary', kind: 'visual', role: 'primary', z_index: 0, muted: false, locked: false, clips: [{ id: 'clip-1', kind: 'video', asset_ref: { asset_id: 'asset_1', version: 1 }, timeline: { start_frame: 0, duration_frames: 90 }, source: { in_us: 0, out_us: 3000000 }, transform: { fit: 'contain', position_x: .5, position_y: .5, scale: 1, crop: { left: 0, top: 0, right: 0, bottom: 0 }, opacity: 1 }, original_audio: { enabled: true, gain_db: 0, fade_in_frames: 0, fade_out_frames: 0 } }] }] }
  assert.equal(validate(fixture), true, JSON.stringify(validate.errors))
  assert.equal(validate({ ...fixture, duration_frames: 1.5 }), false)
})
