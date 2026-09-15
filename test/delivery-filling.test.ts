import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import type { PlatformConfiguration } from '../src/api/delivery'
import type { FillingRequest, FillingResult } from '../src/api/deliveryFilling'
import { automaticFilling, acceptedFilling, applyFillingOcean, fillingValues, isEmptyFillingValue } from '../src/lib/deliveryFilling'

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

test('unit content suggestions preserve business settings and keep product images separate', () => {
  const request = fixture()
  const image = { namespace: 'oceanengine', object_kind: 'product_image', scope: 'account:a', id: 'image', version: '3', state: 'resolved' as const }
  const next = applyFillingOcean(request.ocean, [
    { field: 'copy', value: '查看详情', reason: '产品资料', sources: ['产品'] },
    { field: 'source_label', value: '产品名称', reason: '产品资料', sources: ['产品'] },
    { field: 'call_to_action', value: ['查看详情'], reason: '通用提示', sources: ['产品'] },
    { field: 'product_images', value: [image], reason: '产品主图目录', sources: ['目录'] },
  ], request.target_id)
  assert.deepEqual(next.project, request.ocean.project)
  assert.deepEqual(next.promotions[0].base_material_references, request.ocean.promotions[0].base_material_references)
  assert.deepEqual(next.promotions[0].product_image_references, [image])
  assert.deepEqual(next.promotions[0].settings.call_to_action, ['查看详情'])
  assert.equal(next.promotions[0].settings.source_label, '产品名称')
  assert.equal(isEmptyFillingValue('   '), true)
})


test('automatic filling replaces content but preserves later edits and unrelated fields', () => {
 const original=fixture();original.fields=['copy','selling_points','regions','source_label','search_keywords']
 const current=structuredClone(original);current.current.copy='用户新文案'
 const response:FillingResult={...result,suggestions:[
  {field:'copy',value:'模型文案',reason:'内部依据',sources:[]},
  {field:'selling_points',value:['便捷选购'],reason:'资料',sources:[]},
  {field:'regions',value:['上海市'],reason:'用户要求',sources:[]},
  {field:'search_keywords',value:['淘宝'],reason:'平台',sources:[]},
  {field:'source_label',value:'淘宝',reason:'brand_name',sources:[]},
 ]}
 const {acceptance,skipped}=automaticFilling(response,original,current)
 assert.deepEqual(skipped,['文案']);assert.equal(acceptance.result.suggestions.length,4)
 current.ocean.promotions[0].copy_items=[{text:'用户新文案'}]
 const next=applyFillingOcean(current.ocean,acceptance.result.suggestions,current.target_id)
 assert.deepEqual(next.promotions[0].copy_items,[{text:'用户新文案'}])
 assert.deepEqual(next.promotions[0].base_material_references,current.ocean.promotions[0].base_material_references)
 assert.deepEqual(next.project.budget_and_bidding,current.ocean.project.budget_and_bidding)
 assert.deepEqual(next.project.targeting.regions,['上海市'])
 assert.equal(next.promotions[0].settings.source_label,'淘宝')
 assert.deepEqual(next.project.search_boost?.keywords,['淘宝'])
 const changed=structuredClone(current);changed.ocean.project.account_reference.id='other'
 assert.throws(()=>automaticFilling(response,original,changed),/上下文已变化/)
})

test('finance is opt-in, preserves unrelated fields and rejects budget changes during generation', () => {
  const original = fixture()
  const response: FillingResult = { ...result, suggestions: [
    { field: 'daily_budget', value: 50000, sources: ['预算分配'], reason: '规则' },
    { field: 'project_bid', value: 120, sources: ['真实历史'], reason: '中位数' },
    { field: 'bid', value: 100, sources: ['真实历史'], reason: '中位数' },
  ], finance: { rule_version: 'v1', entries: [{ field: 'daily_budget', amount_minor: 50000, minimum_minor: 50000, maximum_minor: 50000, basis: '分配', sources: [] }], warnings: [] } }
  assert.equal(automaticFilling(response, original, original).acceptance.result.suggestions.length, 0)
  original.finance = { unit_weight: 1 }
  const accepted = automaticFilling(response, original, original).acceptance
  const next = applyFillingOcean(original.ocean, accepted.result.suggestions, original.target_id)
  const expected = structuredClone(original.ocean)
  expected.project.budget_and_bidding.bid_minor = 120
  expected.promotions[0].budget_and_bidding = { currency: 'CNY', bidding_strategy: expected.project.budget_and_bidding.bidding_strategy, charging_mode: expected.project.budget_and_bidding.charging_mode, ...expected.promotions[0].budget_and_bidding!, budget_mode: 'daily', daily_budget_minor: 50000, bid_minor: 100 }
  assert.deepEqual(next, expected)
  const edited = structuredClone(original)
  edited.ocean.project.budget_and_bidding.daily_budget_minor += 1
  const skipped = automaticFilling(response, original, edited)
  assert.equal(skipped.acceptance.result.suggestions.length, 0)
  assert.deepEqual(skipped.acceptance.result.finance?.entries, [])
  assert.equal(skipped.skipped.length, 3)
  const missing = structuredClone(original.ocean)
  delete missing.promotions[0].budget_and_bidding
  const allocated = applyFillingOcean(missing, [response.suggestions[0]], original.target_id)
  assert.deepEqual(allocated.project, missing.project)
  assert.equal(allocated.promotions[0].budget_and_bidding?.daily_budget_minor, 50000)
})
