import assert from 'node:assert/strict'
import test from 'node:test'
import { canOpenShortDramaStep, initialShortDramaPrerollState, shortDramaPrerollReducer } from '../src/features/short-drama-preroll-v2/reducer'
import type { FirstFrameCandidate, HookDirection, StoryAnalysis } from '../src/features/short-drama-preroll-v2/types'

const analysis: StoryAnalysis = {
  title: '武则天权力之路', episode: '第 1 集', synopsis: '武则天进入宫廷并面对权力抉择。',
  openingBeat: '宫廷局势突变', characters: ['武则天｜权力中心人物'], visualKeywords: ['宫廷', '权力'],
}
const hooks: HookDirection[] = [{
  id: 'direction-1', category: 'curiosity', eyebrow: '猎奇吸睛 01', title: '她为何突然回头',
  description: '用信息缺口建立悬念', hookCopy: '所有人都低估了她接下来的选择。', rationale: '来自剧情证据',
}]
const images: FirstFrameCandidate[] = [{ id: 'frame-1', label: '参考图 1', imageUrl: '/frame-1', composition: '人物近景' }]

const source = { id: 'video-1', projectId: 'project-1', version: 1, kind: 'video' as const, sourceType: 'upload' as const, mimeType: 'video/mp4', sizeBytes: 1024, durationSeconds: 120, createdAt: '2026-08-05T00:00:00Z', contentUrl: '/assets/video-1' }

function readyState() {
  let state = shortDramaPrerollReducer(initialShortDramaPrerollState, { type: 'source-selected', source })
  state = shortDramaPrerollReducer(state, { type: 'analysis-ready', analysis })
  state = shortDramaPrerollReducer(state, { type: 'hooks-ready', hooks })
  state = shortDramaPrerollReducer(state, { type: 'hook-selected', id: hooks[0].id, imagePrompt: '宫廷人物首帧', videoDescription: '宫廷权力变化', videoPrompt: '6 秒宫廷钩子', duration: 6 })
  state = shortDramaPrerollReducer(state, { type: 'images-ready', images })
  return shortDramaPrerollReducer(state, { type: 'image-selected', id: images[0].id })
}

test('short drama preroll unlocks workflow steps only after their required decision', () => {
  const empty = { ...initialShortDramaPrerollState, source }
  assert.equal(canOpenShortDramaStep(empty, 'understanding'), true)
  assert.equal(canOpenShortDramaStep(empty, 'direction'), false)
  const analyzed = shortDramaPrerollReducer(empty, { type: 'analysis-ready', analysis })
  assert.equal(canOpenShortDramaStep(analyzed, 'direction'), true)
  assert.equal(canOpenShortDramaStep(analyzed, 'first-frame'), false)
})

test('editing story summary invalidates hooks, prompts, frames and output', () => {
  const changed = shortDramaPrerollReducer(readyState(), { type: 'summary-changed', value: '新的梗概' })
  assert.equal(changed.summaryDraft, '新的梗概')
  assert.deepEqual(changed.hooks, [])
  assert.equal(changed.selectedHookId, '')
  assert.equal(changed.imagePrompt, '')
  assert.deepEqual(changed.images, [])
  assert.equal(changed.output, null)
})

test('changing duration clears the stale direction, first frame, and video output', () => {
  const generated = shortDramaPrerollReducer(readyState(), { type: 'video-ready', output: { id: 'output-1', videoUrl: '/output.mp4', duration: 6, createdAt: '2026-08-05T00:00:00Z' } })
  const changed = shortDramaPrerollReducer(generated, { type: 'duration-changed', duration: 10 })
  assert.equal(changed.duration, 10)
  assert.equal(changed.selectedHookId, '')
  assert.equal(changed.selectedImageId, '')
  assert.equal(changed.output, null)
  assert.equal(changed.videoPrompt, '')
  assert.equal(changed.activeStep, 'direction')
})

test('selecting a hook stores editable prompts returned by the server', () => {
  let state = shortDramaPrerollReducer(initialShortDramaPrerollState, { type: 'analysis-ready', analysis })
  state = shortDramaPrerollReducer(state, { type: 'hooks-ready', hooks })
  state = shortDramaPrerollReducer(state, { type: 'hook-selected', id: hooks[0].id, imagePrompt: '真实首帧提示词', videoDescription: '真实视频描述', videoPrompt: '真实视频提示词', duration: 6 })
  assert.equal(state.imagePrompt, '真实首帧提示词')
  assert.equal(state.videoPrompt, '真实视频提示词')
  assert.equal(state.activeStep, 'first-frame')
})

test('selecting a hook navigates immediately while prompt compilation is pending', () => {
  let state = shortDramaPrerollReducer(initialShortDramaPrerollState, { type: 'analysis-ready', analysis })
  state = shortDramaPrerollReducer(state, { type: 'hooks-ready', hooks })
  state = shortDramaPrerollReducer(state, { type: 'hook-selection-started', id: hooks[0].id })
  assert.equal(state.activeStep, 'first-frame')
  assert.equal(state.selectingHookId, hooks[0].id)
  assert.equal(state.error, '')

  state = shortDramaPrerollReducer(state, { type: 'hook-selection-failed', message: '草稿已更新' })
  assert.equal(state.activeStep, 'direction')
  assert.equal(state.selectedHookId, '')
  assert.equal(state.error, '草稿已更新')
})

test('selecting a first frame stores the candidate-specific compiled video prompt', () => {
  const selected = shortDramaPrerollReducer(readyState(), {
    type: 'image-selected',
    id: images[0].id,
    videoPrompt: '基础提示词；使用国漫半写实首帧与环境悬念机制。',
  })
  assert.equal(selected.selectedImageId, images[0].id)
  assert.match(selected.videoPrompt, /国漫半写实/)
  assert.equal(selected.activeStep, 'video')
})

test('starting a new first-frame batch removes stale candidates and their selection', () => {
  const state = readyState()
  const generating = shortDramaPrerollReducer(state, { type: 'images-started' })
  assert.equal(generating.imagesStatus, 'loading')
  assert.deepEqual(generating.images, [])
  assert.equal(generating.selectedImageId, '')
  assert.equal(generating.selectingImageId, '')
  assert.equal(generating.output, null)
})

test('first-frame selection exposes its pending state and clears it on failure', () => {
  let state = shortDramaPrerollReducer(readyState(), { type: 'image-selection-started', id: images[0].id })
  assert.equal(state.selectingImageId, images[0].id)
  state = shortDramaPrerollReducer(state, { type: 'image-selection-failed', message: '候选批次已更新' })
  assert.equal(state.selectingImageId, '')
  assert.equal(state.error, '候选批次已更新')
})
