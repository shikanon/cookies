import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import type { PlatformConfiguration } from '../src/api/delivery'
import type { FillingRequest, FillingResult } from '../src/api/deliveryFilling'
import { acceptedFilling, applyFillingOcean, fillingValues, isEmptyFillingValue } from '../src/lib/deliveryFilling'

function fixture(): FillingRequest {
  const config = JSON.parse(readFileSync(new URL('../docs/delivery/fixtures/delivery-platform-configuration-v2-oceanengine-valid.json', import.meta.url), 'utf8')) as PlatformConfiguration
  const ocean = config.payload.ocean_engine!
  return { page: 'configuration', plan_id: 'plan', target_id: ocean.promotions[0].promotion_draft_id, ocean, fields: ['name', 'copy', 'daily_budget'], current: fillingValues(ocean, ocean.promotions[0].promotion_draft_id) }
}
const result: FillingResult = { suggestions: [{ field: 'name', value: '新名称', sources: ['项目'], reason: '项目命名' }], warnings: [], context_hash: 'hash', provider: 'ark_text', model: 'model' }

test('filling protects edits during generation and rejects changed account or branch', () => {
  const original = fixture()
  for (const change of [(v: FillingRequest) => { v.current.name = '手工修改' }, (v: FillingRequest) => { v.ocean.project.account_reference.id = 'another' }, (v: FillingRequest) => { v.ocean.project.lead_capture_mode = 'changed' }]) {
    const current = structuredClone(original); change(current)
    assert.throws(() => acceptedFilling(result, original, current, ['name']), /重新生成/)
  }
  const current = structuredClone(original); current.current.copy = '只修改其他字段'
  assert.equal(acceptedFilling(result, original, current, ['name']).result.suggestions.length, 1)
})

test('accepting name or daily budget preserves every other promotion and advanced field', () => {
  const request = fixture()
  const previous = structuredClone(request.ocean)
  previous.promotions[0].product_image_references = [{ namespace: 'oceanengine', object_kind: 'product_image', scope: 'account:a', id: 'image', state: 'resolved' }]
  const next = applyFillingOcean(previous, result.suggestions, request.target_id)
  const expected = structuredClone(previous); expected.promotions[0].promotion_name = '新名称'
  assert.deepEqual(next, expected)
  const budget = applyFillingOcean(previous, [{ field: 'daily_budget', value: 60000, reason: '已有预算', sources: ['计划'] }])
  assert.deepEqual(budget.promotions, previous.promotions)
  const expectedProject = { ...previous.project, budget_and_bidding: { ...previous.project.budget_and_bidding, daily_budget_minor: 60000 } }
  assert.deepEqual(budget.project, expectedProject)
})

test('parent choice applies alone and keeps full source identity', () => {
  const request = fixture()
  const product = { namespace: 'oceanengine', object_kind: 'product', scope: 'account:a', id: 'same', version: '7', content_hash: 'hash', state: 'resolved' as const, audit_attributes: { connector_platform_object_id: 'connector-1' } }
  const response: FillingResult = { ...result, suggestions: [{ field: 'product', value: product, reason: '匹配项目', sources: ['目录'] }, ...result.suggestions] }
  const accepted = acceptedFilling(response, request, request, ['product', 'name'])
  assert.deepEqual(accepted.result.suggestions.map(item => item.field), ['product'])
  assert.deepEqual(applyFillingOcean(request.ocean, accepted.result.suggestions).project.marketing_product_reference, product)
  assert.equal(isEmptyFillingValue('已有文案'), false)
  assert.equal(isEmptyFillingValue([]), true)
  assert.equal(isEmptyFillingValue({ state: 'unresolved' }), true)
})
