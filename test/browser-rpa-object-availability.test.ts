import assert from 'node:assert/strict'
import test from 'node:test'
import { presentConfigurationIssue, presentObjectAvailability, presentPlanBlockedReason } from '../src/features/browser-rpa-execution/objectAvailabilityPresentation'

test('shows a friendly action for an unbound brand', () => {
  const result = presentObjectAvailability({
    field_key: 'promotions.0.settings.brand_reference',
    object_kind: 'brand',
    internal_object_id: 'brand-other',
    display_name: '其他',
    available: false,
    reason: '未绑定巨量平台 ID',
  })

  assert.equal(result.scopeLabel, '单元 1')
  assert.equal(result.kindLabel, '品牌')
  assert.equal(result.statusLabel, '需处理')
  assert.equal(result.statusDetail, '请先为“其他”绑定巨量品牌 ID。')
})

test('does not show a full landing-page URL as its name or platform ID', () => {
  const landingPageUrl = 'https://market.m.taobao.com/app/starlink/wakeup-transit/pages/download?projectid=__PROJECT_ID__'
  const result = presentObjectAvailability({
    field_key: 'promotions.0.landing_page_reference',
    object_kind: 'owned_landing_page',
    internal_object_id: landingPageUrl,
    display_name: landingPageUrl,
    platform_object_id: landingPageUrl,
    available: true,
  })

  assert.equal(result.name, 'market.m.taobao.com 落地页')
  assert.equal(result.statusLabel, '已绑定')
  assert.equal(result.statusDetail, '落地页可用。')
  assert.equal(result.platformId, undefined)
})

test('shows a manual direct link as ready without a platform binding', () => {
  const directLink = 'tbopen://m.taobao.com/tbopen/index.html?action=ali.open.nav&module=h5'
  const result = presentObjectAvailability({
    field_key: 'promotions.0.direct_link_reference',
    object_kind: 'direct_link',
    internal_object_id: directLink,
    available: true,
    reason: '手动填写链接，无需绑定平台 ID',
  })

  assert.equal(result.name, 'm.taobao.com 直达链接')
  assert.equal(result.statusLabel, '可直接填写')
  assert.equal(result.statusDetail, '手动链接可用，无需绑定平台 ID。')
  assert.equal(result.platformId, undefined)
})

test('keeps a normal platform ID and uses the business object name', () => {
  const result = presentObjectAvailability({
    field_key: 'project.marketing_product_reference',
    object_kind: 'product',
    internal_object_id: 'product-1',
    display_name: '限时福利快来薅羊毛啦',
    platform_object_id: '1786513565497554221',
    available: true,
  })

  assert.equal(result.scopeLabel, '项目')
  assert.equal(result.kindLabel, '营销商品')
  assert.equal(result.name, '限时福利快来薅羊毛啦')
  assert.equal(result.platformId, '1786513565497554221')
})

test('replaces the internal block code with a clear message', () => {
  assert.equal(
    presentPlanBlockedReason('PLATFORM_OBJECTS_UNAVAILABLE'),
    '有巨量对象尚未绑定。请处理下方标记为“需处理”的对象。',
  )
})

test('explains incomplete promotion fields without exposing its internal draft ID', () => {
  const issue = presentConfigurationIssue('promotion promotion-deliveryplan_123-1 requires copy, source, and name')

  assert.equal(issue, '投放单元缺少必要字段。请补充单元文案、素材来源和单元名称。')
  assert.doesNotMatch(issue, /deliveryplan_123/)
  assert.equal(
    presentPlanBlockedReason('DELIVERY_CONFIGURATION_INVALID'),
    '投放配置不完整。请补充下方字段，然后重新生成计划。',
  )
})

test('explains other configuration requirements with direct repair instructions', () => {
  assert.equal(
    presentConfigurationIssue('unsupported account path: marketing purpose unknown is not calibrated'),
    '当前营销目的不在校准清单中。请返回平台配置页并选择已校准的营销目的。',
  )
  assert.equal(
    presentConfigurationIssue('project: bid is outside the calibrated limit'),
    '项目出价超出当前 Runner 的校准范围。请返回平台配置页检查当前计费方式和出价范围。',
  )
  assert.equal(
    presentConfigurationIssue('project: bid is outside the calibrated limit for CPM: expected CNY 4.00 to 100.00'),
    '项目出价超出当前 Runner 的校准范围。CPM 出价必须是 4 至 100 元。',
  )
  assert.equal(
    presentConfigurationIssue('lead_generation with owned_landing_page requires project.optimization_target_reference click or impression and project.delivery_mode ubmax'),
    '销售线索使用自研落地页时，优化目标只能选择“点击量”或“展示量”，投放模式必须选择“UBMax”。',
  )
  assert.equal(
    presentConfigurationIssue('marketing product: reference 1786513565497554221 is outside the delivery intent'),
    '营销商品未加入投放意图。请返回平台配置页，保存当前配置并创建新执行。',
  )
  assert.equal(
    presentConfigurationIssue('promotion unit-1: base material: reference 7649703629105889290 is outside the delivery intent'),
    '基础素材未加入投放意图。请返回平台配置页，保存当前配置并创建新执行。',
  )
  assert.equal(
    presentConfigurationIssue('promotion unit-1: product image: reference 7673690181130207278 is outside the delivery intent'),
    '产品主图未加入投放意图。请返回平台配置页，保存当前配置并创建新执行。',
  )
  assert.equal(
    presentConfigurationIssue('promotion unit-1: product image requires a stable image_src_identity'),
    '产品主图缺少稳定图片路径。请重新同步巨量对象目录，然后重新选择产品主图。',
  )
  assert.equal(
    presentConfigurationIssue('promotion unit-1 call to action needs 1 to 10 unique values'),
    '投放单元缺少行动号召。请填写 1 至 10 个不重复的行动号召。',
  )
  assert.equal(
    presentConfigurationIssue('promotion unit-1: Runner v3 supports exactly one bound base material per form'),
    '投放单元的基础素材数量不正确。请选择一个可用素材。',
  )
  assert.equal(
    presentConfigurationIssue('project start date 2026-08-29 must be no earlier than 2026-08-30'),
    '此执行记录的项目开始日期是 2026-08-29。最早允许日期是 2026-08-30。',
  )
  assert.equal(
    presentConfigurationIssue('unknown configuration failure'),
    '投放配置未通过执行前检查。请返回平台配置页并检查标记为“必填”的字段。',
  )
})
