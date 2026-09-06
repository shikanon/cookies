import assert from 'node:assert/strict'
import test from 'node:test'
import { carrierUsesOrangeLandingPage, changeOceanEngineCarrier, normalizeOceanEngineLandingPages } from '../src/lib/deliveryCarrier.js'

test('Orange landing-page carriers use the Orange selector', () => {
  assert.equal(carrierUsesOrangeLandingPage('orange_landing_page'), true)
  assert.equal(carrierUsesOrangeLandingPage('orange_landing_page_and_im'), true)
  assert.equal(carrierUsesOrangeLandingPage('owned_landing_page'), false)
  assert.equal(carrierUsesOrangeLandingPage('im'), false)
})

test('changing the carrier clears every promotion landing-page reference', () => {
  const original = {
    project: { carrier: 'orange_landing_page', name: 'project' },
    promotions: [
      { id: 'promotion-1', landing_page_reference: { object_kind: 'orange_landing_page', id: '1' } },
      { id: 'promotion-2', landing_page_reference: { object_kind: 'orange_landing_page', id: '2' } },
    ],
  }

  const changed = changeOceanEngineCarrier(original, 'owned_landing_page')
  assert.equal(changed.project.carrier, 'owned_landing_page')
  assert.deepEqual(changed.promotions.map(item => item.landing_page_reference), [undefined, undefined])
  assert.equal(original.promotions[0].landing_page_reference.id, '1')
})

test('selecting the current carrier keeps the configuration identity', () => {
  const configuration = { project: { carrier: 'owned_landing_page' }, promotions: [] }
  assert.equal(changeOceanEngineCarrier(configuration, 'owned_landing_page'), configuration)
})

test('restoring an owned carrier upgrades a legacy URL reference', () => {
  const configuration = {
    project: { carrier: 'owned_landing_page' },
    promotions: [{ landing_page_reference: { namespace: 'oceanengine', object_kind: 'landing_page', id: 'https://example.test/owned' } }],
  }
  const normalized = normalizeOceanEngineLandingPages(configuration)
  assert.deepEqual(normalized.promotions[0].landing_page_reference, {
    namespace: 'cookies', object_kind: 'owned_landing_page', id: 'https://example.test/owned',
  })
})

test('saving a non-landing carrier removes a stale landing-page reference', () => {
  const configuration = {
    project: { carrier: 'im' },
    promotions: [{ landing_page_reference: { namespace: 'oceanengine', object_kind: 'orange_landing_page', id: '1' } }],
  }
  const normalized = normalizeOceanEngineLandingPages(configuration)
  assert.equal(normalized.promotions[0].landing_page_reference, undefined)
})
