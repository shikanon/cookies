import assert from 'node:assert/strict'
import test from 'node:test'

import {
  resolveOceanEngineBidConstraint,
  resolveOceanEngineChargingMode,
} from '../src/lib/oceanengineBidConstraints'

test('resolves charging mode from optimization target', () => {
  assert.equal(resolveOceanEngineChargingMode({ semantic_key: 'impression' }, 'CPC'), 'CPM')
  assert.equal(resolveOceanEngineChargingMode({ display_name_snapshot: '点击量' }, 'CPM'), 'CPC')
  assert.equal(resolveOceanEngineChargingMode({ semantic_key: 'button_jump' }, 'CPC'), 'OCPM')
  assert.equal(resolveOceanEngineChargingMode({ semantic_key: 'button_jump' }, 'OCPC'), 'OCPC')
})

test('uses pricing-specific bid ranges', () => {
  assert.deepEqual(resolveOceanEngineBidConstraint('CPM', 30000), {
    schemaVersion: 'oceanengine-bid-constraints/v1',
    chargingMode: 'CPM',
    minimumMinor: 400,
    maximumMinor: 10000,
    maximumSource: 'static',
  })
  assert.equal(resolveOceanEngineBidConstraint('CPC', 30000)?.minimumMinor, 10)
  assert.equal(resolveOceanEngineBidConstraint('OCPM', 30000)?.maximumMinor, 30000)
})
