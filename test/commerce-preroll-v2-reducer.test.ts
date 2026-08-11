import assert from 'node:assert/strict'
import test from 'node:test'
import { fixtureAnalysis, fixtureFirstFrames, fixtureHooks, fixtureProductFacts, fixtureProductReference, fixtureRiskFacts, fixtureSourceVideo } from '../src/features/commerce-preroll-v2/fixtures'
import { canOpenCommercePrerollStep, commerceHookPreparationPlan, commercePrerollReducer, commercePrerollRetryOperation, createInitialCommercePrerollState } from '../src/features/commerce-preroll-v2/reducer'
import { readCommercePrerollSession, writeCommercePrerollSession } from '../src/features/commerce-preroll-v2/sessionStore'

test('hook retry resumes after confirmation instead of confirming the same draft twice', () => {
  assert.deepEqual(commerceHookPreparationPlan('understanding_ready', false), { confirm: true, generate: true })
  assert.deepEqual(commerceHookPreparationPlan('understanding_confirmed', false), { confirm: false, generate: true })
  assert.deepEqual(commerceHookPreparationPlan('hooks_ready', true), { confirm: false, generate: false })
})

test('commerce preroll session restore discards transient service errors', () => {
  const values = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  } })
  const failed = commercePrerollReducer(createInitialCommercePrerollState(), { type: 'operation-failed', scope: 'hooks', message: 'INVALID_STATE' })
  writeCommercePrerollSession('project_retry', failed)
  const restored = readCommercePrerollSession('project_retry')
  assert.equal(restored?.error, '')
  assert.equal(restored?.hooksStatus, 'idle')
})

test('commerce preroll retry maps settings and first-frame failures to frame generation', () => {
  assert.equal(commercePrerollRetryOperation('understanding'), 'analysis')
  assert.equal(commercePrerollRetryOperation('understanding', 'hooks'), 'hooks')
  assert.equal(commercePrerollRetryOperation('settings'), 'frames')
  assert.equal(commercePrerollRetryOperation('first-frame'), 'frames')
  assert.equal(commercePrerollRetryOperation('video'), 'video')
})

test('commerce preroll steps unlock only after durable prerequisites exist', () => {
  let state = createInitialCommercePrerollState()
  assert.equal(canOpenCommercePrerollStep(state, 'understanding'), false)
  state = commercePrerollReducer(state, { type: 'source-selected', source: fixtureSourceVideo })
  assert.equal(canOpenCommercePrerollStep(state, 'understanding'), true)
  state = commercePrerollReducer(state, { type: 'analysis-ready', analysis: fixtureAnalysis, product: fixtureProductFacts, reference: fixtureProductReference, risks: fixtureRiskFacts })
  state = commercePrerollReducer(state, { type: 'hooks-ready', hooks: fixtureHooks })
  assert.equal(canOpenCommercePrerollStep(state, 'direction'), true)
  state = commercePrerollReducer(state, { type: 'hook-selected', id: fixtureHooks[0].id })
  assert.equal(canOpenCommercePrerollStep(state, 'settings'), true)
  state = commercePrerollReducer(state, { type: 'frames-ready', frames: fixtureFirstFrames })
  assert.equal(canOpenCommercePrerollStep(state, 'first-frame'), true)
  state = commercePrerollReducer(state, { type: 'frame-selected', id: fixtureFirstFrames[0].id })
  assert.equal(canOpenCommercePrerollStep(state, 'video'), true)
})

test('editing confirmed product facts invalidates hook and generation descendants', () => {
  let state = createInitialCommercePrerollState()
  state = commercePrerollReducer(state, { type: 'source-selected', source: fixtureSourceVideo })
  state = commercePrerollReducer(state, { type: 'analysis-ready', analysis: fixtureAnalysis, product: fixtureProductFacts, reference: fixtureProductReference, risks: [] })
  state = commercePrerollReducer(state, { type: 'hooks-ready', hooks: fixtureHooks })
  state = commercePrerollReducer(state, { type: 'hook-selected', id: fixtureHooks[1].id })
  state = commercePrerollReducer(state, { type: 'frames-ready', frames: fixtureFirstFrames })
  state = commercePrerollReducer(state, { type: 'frame-selected', id: fixtureFirstFrames[0].id })
  state = commercePrerollReducer(state, { type: 'product-field-changed', field: 'sellingPoints', value: '新的已确认卖点' })
  assert.equal(state.productDraft?.sellingPoints, '新的已确认卖点')
  assert.equal(state.hooks.length, 0)
  assert.equal(state.selectedHookId, '')
  assert.equal(state.firstFrames.length, 0)
  assert.equal(state.selectedFirstFrameId, '')
  assert.equal(state.output, null)
})
