import assert from 'node:assert/strict'
import test from 'node:test'

import { SaveCoordinator } from '../src/features/video-editing/saveCoordinator.ts'

test('save coordinator persists the latest edit made while an older save is in flight', async () => {
  const pending: Array<{ value: string; resolve: (version: number) => void }> = []
  const coordinator = new SaveCoordinator<string, number>((value, baseVersion) => new Promise(resolve => {
    assert.equal(baseVersion, pending.length)
    pending.push({ value, resolve })
  }))

  const first = coordinator.submit('timeline-v1', 0)
  const latest = coordinator.submit('timeline-v2', 0)
  assert.deepEqual(pending.map(item => item.value), ['timeline-v1'])

  pending[0].resolve(1)
  await first
  await new Promise(resolve => setTimeout(resolve, 0))
  assert.deepEqual(pending.map(item => item.value), ['timeline-v1', 'timeline-v2'])

  pending[1].resolve(2)
  assert.equal(await latest, 2)
  assert.equal(coordinator.state.status, 'clean')
  assert.equal(coordinator.state.version, 2)
})
