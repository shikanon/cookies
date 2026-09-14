import type { FillingAcceptance, FillingField, FillingRequest, FillingResult, FillingSuggestion, FillingValues } from '../api/deliveryFilling'
import type { OceanConfiguration } from './deliveryChoices'
import { changeConfigurationAccount, changeConfigurationProject, changeProjectChoice } from './deliveryChoices'

export const fillingLabels: Record<FillingField, string> = { name: '名称', objective: '业务目标', account: '账户', marketing_purpose: '营销目的', product: '产品', materials: '素材', carrier: '投放载体', optimization_target: '优化目标', product_name: '产品名称', selling_points: '卖点', copy: '文案', total_budget: '计划总预算', daily_budget: '日预算', bid: '出价', schedule: '排期', landing_page: '落地页', monitoring: '监测引用', identity: '授权身份' }

export function fillingValues(ocean: OceanConfiguration, targetId?: string): Partial<FillingValues> {
  const p = ocean.project
  const unit = ocean.promotions.find(item => item.promotion_draft_id === targetId)
  return {
    name: unit ? unit.promotion_name : p.project_name, account: p.account_reference, marketing_purpose: p.marketing_purpose,
    product: p.marketing_product_reference, carrier: p.carrier, optimization_target: p.optimization_target_reference,
    daily_budget: (unit?.budget_and_bidding ?? p.budget_and_bidding).daily_budget_minor,
    bid: (unit?.budget_and_bidding ?? p.budget_and_bidding).bid_minor, schedule: p.schedule, monitoring: p.monitoring_references ?? [],
    ...(unit ? { materials: unit.base_material_references, product_name: unit.product_name ?? '', selling_points: unit.product_selling_points ?? [], copy: unit.copy_items.map(item => item.text).join('\n'), landing_page: unit.landing_page_reference, identity: unit.delivery_identity.authorized_identity } : {}),
  }
}

export function fillingContextKey(request: FillingRequest): string {
  const p = request.ocean.project
  return JSON.stringify([request.page, request.plan_id, request.target_id, request.strategy, request.search, request.fields,
    p.account_reference, p.marketing_purpose, p.marketing_scenario, p.lead_capture_mode, p.carrier, p.marketing_product_reference,
    p.delivery_mode, p.application_reference, request.ocean.promotions.map(item => item.promotion_draft_id)])
}

export function isEmptyFillingValue(value: unknown): boolean {
  if (value === undefined || value === null || value === '' || value === 0) return true
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
      case 'name': if (unit) unit.promotion_name = item.value; else p.project_name = item.value; break
      case 'account': next = changeConfigurationAccount(next, { id: item.value.id ?? '', display_label: item.value.display_name_snapshot ?? '' }); next.project.account_reference = item.value; break
      case 'marketing_purpose': next = changeConfigurationProject(next, changeProjectChoice(p, { marketing_purpose: item.value })); break
      case 'carrier': next = changeConfigurationProject(next, changeProjectChoice(p, { carrier: item.value })); break
      case 'product': p.marketing_product_reference = item.value; break
      case 'optimization_target': p.optimization_target_reference = item.value; break
      case 'daily_budget': (unit?.budget_and_bidding ?? p.budget_and_bidding).daily_budget_minor = item.value; break
      case 'bid': (unit?.budget_and_bidding ?? p.budget_and_bidding).bid_minor = item.value; break
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
  if (['total_budget', 'daily_budget', 'bid'].includes(field) && typeof value === 'number') return `¥${(value / 100).toFixed(2)}`
  const labels: Record<string, string> = { ecommerce: '电商', lead_generation: '销售线索', product_catalog: '商品', content_marketing: '内容营销', orange_landing_page: '橙子落地页', orange_landing_page_and_im: '橙子落地页 + 抖音私信页', owned_landing_page: '自研落地页', im: '抖音私信页', douyin_account: '抖音号' }
  if (['marketing_purpose', 'carrier'].includes(field) && typeof value === 'string' && labels[value]) return labels[value]
  return fillingDisplay(value)
}
