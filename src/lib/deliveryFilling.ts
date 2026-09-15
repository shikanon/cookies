import type { FillingAcceptance, FillingField, FillingRequest, FillingResult, FillingSuggestion, FillingValues } from '../api/deliveryFilling'
import type { OceanConfiguration } from './deliveryChoices'
import { changeConfigurationAccount, changeConfigurationProject, changeProjectChoice } from './deliveryChoices'

export const fillingLabels: Record<FillingField, string> = { search_expansion: '搜索定向扩展', search_keywords: '搜索关键词', search_terms: '搜索词', regions: '地域', age_ranges: '年龄', gender: '性别', smart_expansion: '智能定向扩展', source_label: '来源', call_to_action: '行动号召', category: '所属类别', product_images: '产品主图', name: '名称', objective: '业务目标', account: '账户', marketing_purpose: '营销目的', product: '产品', materials: '素材', carrier: '投放载体', optimization_target: '优化目标', product_name: '产品名称', selling_points: '卖点', copy: '文案', total_budget: '计划总预算', daily_budget: '日预算', bid: '单元出价', project_bid: '项目出价', schedule: '排期', landing_page: '落地页', monitoring: '监测引用', identity: '授权身份' }

export function fillingValues(ocean: OceanConfiguration, targetId?: string): Partial<FillingValues> {
  const p = ocean.project
  const unit = ocean.promotions.find(item => item.promotion_draft_id === targetId)
  return {
    search_expansion: p.search_boost?.targeting_expansion ?? false, search_keywords: p.search_boost?.keywords ?? [], search_terms: unit?.settings.search_terms ?? [], regions: p.targeting.regions ?? [], age_ranges: p.targeting.age_ranges ?? [], gender: p.targeting.gender ?? '', smart_expansion: p.targeting.smart_expansion,
    name: unit ? unit.promotion_name : p.project_name, account: p.account_reference, marketing_purpose: p.marketing_purpose,
    product: p.marketing_product_reference, carrier: p.carrier, optimization_target: p.optimization_target_reference,
    daily_budget: unit ? unit.budget_and_bidding?.daily_budget_minor ?? 0 : p.budget_and_bidding.daily_budget_minor,
    project_bid: p.budget_and_bidding.bid_minor,
    bid: unit ? unit.budget_and_bidding?.bid_minor : p.budget_and_bidding.bid_minor, schedule: p.schedule, monitoring: p.monitoring_references ?? [],
    ...(unit ? { source_label: unit.settings.source_label ?? '', call_to_action: unit.settings.call_to_action ?? [], category: unit.settings.category_reference, product_images: unit.product_image_references ?? [], materials: unit.base_material_references, product_name: unit.product_name || p.marketing_product_reference?.display_name_snapshot || '', selling_points: unit.product_selling_points ?? [], copy: unit.copy_items.map(item => item.text).join('\n'), landing_page: unit.landing_page_reference, identity: unit.delivery_identity.authorized_identity } : {}),
  }
}

export function fillingContextKey(request: FillingRequest): string {
  const p = request.ocean.project
  const unit = request.ocean.promotions.find(item => item.promotion_draft_id === request.target_id)
  return JSON.stringify([unit?.settings.title_mode, unit?.delivery_identity.mode, unit?.base_material_references, p.optimization_target_reference, request.page, request.plan_id, request.target_id, request.strategy, request.search, request.instructions, request.material_count, request.copy_count, request.fields, request.finance,
    p.account_reference, p.marketing_purpose, p.marketing_scenario, p.lead_capture_mode, p.carrier, p.marketing_product_reference,
    p.delivery_mode, p.application_reference, request.ocean.promotions.map(item => item.promotion_draft_id)])
}

export function isEmptyFillingValue(value: unknown): boolean {
  if (value === undefined || value === null || (typeof value === 'string' && value.trim() === '') || value === 0) return true
  if (Array.isArray(value)) return value.length === 0
  return typeof value === 'object' && 'state' in value && !('id' in value && value.id)
}

export function acceptedFilling(result: FillingResult, original: FillingRequest, current: FillingRequest, selected: FillingField[]): FillingAcceptance {
  if (fillingContextKey(original) !== fillingContextKey(current)) throw new Error('账户、业务分支或编辑对象已变化，请重新生成建议。')
  let suggestions = result.suggestions.filter(item => selected.includes(item.field))
  if (suggestions.some(item => JSON.stringify(original.current[item.field]) !== JSON.stringify(current.current[item.field]))) throw new Error('待填写字段已被修改，请重新生成建议，避免覆盖当前编辑。')
  const parentChanges = suggestions.filter(item => ['account', 'marketing_purpose', 'carrier', 'product'].includes(item.field) && JSON.stringify(current.current[item.field]) !== JSON.stringify(item.value))
  // A new parent context needs fresh capabilities and catalog suggestions.
  if (parentChanges.length) suggestions = [parentChanges[0]]
  return { result: { ...result, suggestions }, target_id: current.target_id, accepted_at: new Date().toISOString() }
}

export function applyFillingOcean(value: OceanConfiguration, suggestions: FillingSuggestion[], targetId?: string): OceanConfiguration {
  let next = structuredClone(value)
  for (const item of structuredClone(suggestions)) {
    const p = next.project
    const unit = next.promotions.find(promotion => promotion.promotion_draft_id === targetId)
    switch (item.field) {
      case 'search_expansion': p.search_boost = { ...p.search_boost, targeting_expansion: item.value }; break
      case 'search_keywords': p.search_boost = { ...p.search_boost, keywords: item.value }; break
      case 'search_terms': if (unit) unit.settings.search_terms = item.value; break
      case 'regions': p.targeting.regions = item.value; break
      case 'age_ranges': p.targeting.age_ranges = item.value; break
      case 'gender': p.targeting.gender = item.value; break
      case 'smart_expansion': p.targeting.smart_expansion = item.value; break
      case 'source_label': if (unit) unit.settings.source_label = item.value; break
      case 'call_to_action': if (unit) unit.settings.call_to_action = item.value; break
      case 'category': if (unit) unit.settings.category_reference = item.value; break
      case 'product_images': if (unit) unit.product_image_references = item.value; break
      case 'name': if (unit) unit.promotion_name = item.value; else p.project_name = item.value; break
      case 'account': next = changeConfigurationAccount(next, { id: item.value.id ?? '', display_label: item.value.display_name_snapshot ?? '' }); next.project.account_reference = item.value; break
      case 'marketing_purpose': next = changeConfigurationProject(next, changeProjectChoice(p, { marketing_purpose: item.value })); break
      case 'carrier': next = changeConfigurationProject(next, changeProjectChoice(p, { carrier: item.value })); break
      case 'product': p.marketing_product_reference = item.value; break
      case 'optimization_target': p.optimization_target_reference = item.value; break
      case 'daily_budget':
        if (unit) unit.budget_and_bidding = { currency: p.budget_and_bidding.currency, bidding_strategy: p.budget_and_bidding.bidding_strategy, charging_mode: p.budget_and_bidding.charging_mode, bid_minor: 0, ...unit.budget_and_bidding, budget_mode: 'daily', daily_budget_minor: item.value }
        else p.budget_and_bidding.daily_budget_minor = item.value
        break
      case 'project_bid': p.budget_and_bidding.bid_minor = item.value; break
      case 'bid': if (unit?.budget_and_bidding) unit.budget_and_bidding.bid_minor = item.value; else if (!unit) p.budget_and_bidding.bid_minor = item.value; break
      case 'schedule': p.schedule = item.value; break
      case 'monitoring': p.monitoring_references = item.value; break
      case 'materials': if (unit) unit.base_material_references = item.value; break
      case 'product_name': if (unit) unit.product_name = item.value; break
      case 'selling_points': if (unit) unit.product_selling_points = item.value; break
      case 'copy': if (unit) unit.copy_items = item.value.split('\n').filter(Boolean).map(text => ({ text })); break
      case 'landing_page': if (unit) unit.landing_page_reference = item.value; break
      case 'identity': if (unit) unit.delivery_identity = { mode: 'douyin_account', authorized_identity: item.value }; break
    }
  }
  return next
}

export function fillingDisplay(value: unknown): string {
  if (isEmptyFillingValue(value)) return '未填写'
  if (Array.isArray(value)) return value.map(fillingDisplay).join('、')
  if (typeof value === 'object' && value) {
    if ('display_name_snapshot' in value) return String(value.display_name_snapshot || ('id' in value ? value.id : ''))
    if ('id' in value) return String(value.id)
    if ('start_at' in value && 'end_at' in value) return `${value.start_at} 至 ${value.end_at}`
    return JSON.stringify(value)
  }
  return String(value)
}

export function fillingFieldDisplay(field: FillingField, value: unknown): string {
  if (['total_budget', 'daily_budget', 'bid', 'project_bid'].includes(field) && typeof value === 'number') return `¥${(value / 100).toFixed(2)}`
  const labels: Record<string, string> = { ecommerce: '电商', lead_generation: '销售线索', product_catalog: '商品', content_marketing: '内容营销', orange_landing_page: '橙子落地页', orange_landing_page_and_im: '橙子落地页 + 抖音私信页', owned_landing_page: '自研落地页', im: '抖音私信页', douyin_account: '抖音号' }
  if (['marketing_purpose', 'carrier'].includes(field) && typeof value === 'string' && labels[value]) return labels[value]
  return fillingDisplay(value)
}

export function automaticFilling(result: FillingResult, original: FillingRequest, current: FillingRequest) {
  if (fillingContextKey(original) !== fillingContextKey(current)) throw new Error('填写上下文已变化，请重新生成。')
  const financeChanged = JSON.stringify([original.ocean.project.budget_and_bidding, original.ocean.project.deep_optimization_mode, original.ocean.promotions.map(item => item.budget_and_bidding)]) !== JSON.stringify([current.ocean.project.budget_and_bidding, current.ocean.project.deep_optimization_mode, current.ocean.promotions.map(item => item.budget_and_bidding)])
  const financial = (field: FillingField) => ['daily_budget', 'bid', 'project_bid'].includes(field)
  const changed = result.suggestions.filter(item => (financial(item.field) && financeChanged) || JSON.stringify(original.current[item.field]) !== JSON.stringify(current.current[item.field]))
  const suggestions = result.suggestions.filter(item => (financial(item.field) ? Boolean(original.finance) : original.fields.includes(item.field)) && !changed.includes(item) && JSON.stringify(current.current[item.field]) !== JSON.stringify(item.value))
  return { acceptance: { result: { ...result, suggestions, finance: result.finance ? { ...result.finance, entries: result.finance.entries.filter(entry => suggestions.some(item => item.field === entry.field)) } : undefined }, target_id: current.target_id, accepted_at: new Date().toISOString() }, skipped: changed.map(item => fillingLabels[item.field]) }
}
