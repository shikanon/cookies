import assert from 'node:assert/strict'
import test from 'node:test'

import { initialShortDramaPrerollState, shortDramaPrerollReducer } from '../src/features/short-drama-preroll-v2/reducer'

test('short drama preroll defaults to ten seconds', () => {
  assert.equal(initialShortDramaPrerollState.duration, 10)
})

test('changing duration invalidates prompts and requires direction reselection', () => {
  const state = {
    ...initialShortDramaPrerollState,
    activeStep: 'first-frame' as const,
    selectedHookId: 'direction_1',
    imagePrompt: 'old image prompt',
    videoDescription: 'old video description',
    videoPrompt: 'old video prompt',
  }

  const changed = shortDramaPrerollReducer(state, { type: 'duration-changed', duration: 12 })

  assert.equal(changed.duration, 12)
  assert.equal(changed.activeStep, 'direction')
  assert.equal(changed.selectedHookId, '')
  assert.equal(changed.imagePrompt, '')
  assert.equal(changed.videoDescription, '')
  assert.equal(changed.videoPrompt, '')
})
