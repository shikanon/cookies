import type { PlatformConfiguration, StableReference } from '../api/delivery'
import type { ApiConnectorAccount } from '../data/api'
import { oceanEngineLeadCaptureMode } from './oceanengineBranchConstraints'

export type OceanConfiguration = NonNullable<PlatformConfiguration['payload']['ocean_engine']>
export type OceanProject = OceanConfiguration['project']
export const marketingPurposeOptions = [
  { value: 'ecommerce', label: '电商' }, { value: 'lead_generation', label: '销售线索' },
  { value: 'application', label: '应用（暂不支持）', disabled: true },
  { value: 'product_catalog', label: '商品' }, { value: 'content_marketing', label: '内容营销' },
]
export const scheduleModeOptions = [{ value: 'long_term', label: '从今天起长期投放' }, { value: 'fixed_range', label: '设置开始和结束日期' }]

export function carrierOptions(project: OceanProject) {
  const content = project.marketing_purpose === 'content_marketing'
  const lead = project.marketing_purpose === 'lead_generation'
  const custom = oceanEngineLeadCaptureMode(project) === 'custom_lead'
  return [
    ...(content ? [{ value: 'douyin_account', label: '抖音号' }] : []),
    { value: 'orange_landing_page', label: '橙子落地页' },
    ...(lead && !custom ? [{ value: 'orange_landing_page_and_im', label: '橙子落地页 + 抖音私信页' }] : []),
    ...(!lead || custom ? [{ value: 'owned_landing_page', label: '自研落地页' }, ...(!content ? [{ value: 'im', label: '抖音私信页（原抖音主页）' }] : [])] : []),
    ...(!content ? [{ value: 'byte_miniapp', label: '字节小程序（暂不支持）', disabled: true }, { value: 'wechat_miniapp', label: '微信小程序（暂不支持）', disabled: true }] : []),
  ]
}

export function changeProjectChoice(project: OceanProject, patch: Partial<OceanProject>): OceanProject {
  const next = { ...project, ...patch }
  if (patch.marketing_purpose !== undefined && patch.marketing_purpose !== project.marketing_purpose) {
    next.optimization_target_reference = undefined
    if (next.marketing_purpose !== 'product_catalog') next.product_targeting = undefined
    if (next.marketing_purpose === 'content_marketing') {
      Object.assign(next, { carrier: 'douyin_account', delivery_mode: 'ubmax', deep_optimization_mode: 'disabled', placement_strategy: 'preferred_media', placement_media: ['douyin'] })
    } else if (project.carrier === 'douyin_account') next.carrier = 'orange_landing_page'
    if (['lead_generation', 'content_marketing'].includes(next.marketing_purpose)) {
      next.delivery_mode = 'ubmax'
      next.budget_and_bidding = { ...next.budget_and_bidding, ...(next.marketing_purpose === 'content_marketing' ? { bidding_strategy: 'stable_cost' } : {}), budget_mode: 'daily', daily_budget_minor: Math.max(next.budget_and_bidding.daily_budget_minor, 30000) }
    }
    if (next.carrier && next.marketing_purpose !== 'application' && !carrierOptions(next).some(option => option.value === next.carrier && !option.disabled)) next.carrier = 'orange_landing_page'
  }
  if (patch.lead_capture_mode !== undefined && patch.lead_capture_mode !== project.lead_capture_mode) {
    next.optimization_target_reference = undefined
    if (!carrierOptions(next).some(option => option.value === next.carrier && !option.disabled)) next.carrier = 'orange_landing_page'
  }
  if (next.carrier !== project.carrier) next.optimization_target_reference = undefined
  return next
}

export function changeConfigurationProject(ocean: OceanConfiguration, project: OceanProject): OceanConfiguration {
  const native = (value: OceanProject) => value.marketing_purpose === 'content_marketing' && value.carrier === 'douyin_account'
  const changedNative = native(project) !== native(ocean.project)
  return { ...ocean, project, promotions: ocean.promotions.map(promotion => ({
    ...promotion,
    ...(project.carrier !== ocean.project.carrier ? { landing_page_reference: undefined } : {}),
    ...(changedNative ? { delivery_identity: { mode: native(project) ? 'all_douyin_accounts' : 'account_info' }, base_material_references: [], landing_page_reference: undefined } : {}),
  })) }
}

export function changeConfigurationAccount(ocean: OceanConfiguration, account: Pick<ApiConnectorAccount, 'id' | 'display_label'>): OceanConfiguration {
  if (account.id === ocean.project.account_reference.id) return ocean
  const keep = (reference?: StableReference) => reference?.namespace === 'cookies' ? reference : undefined
  const project = ocean.project
  return { ...ocean, project: {
    ...project, account_reference: { namespace: 'oceanengine', object_kind: 'advertiser_account', scope: project.account_reference.scope, id: account.id, state: 'resolved', display_name_snapshot: account.display_label || account.id },
    marketing_product_reference: keep(project.marketing_product_reference), application_reference: undefined,
    optimization_target_reference: undefined, product_catalog_reference: undefined,
    monitoring_references: [],
  }, promotions: ocean.promotions.map(promotion => ({
    ...promotion, base_material_references: promotion.base_material_references.filter(reference => keep(reference)),
    product_image_references: promotion.product_image_references?.filter(reference => keep(reference)),
    landing_page_reference: keep(promotion.landing_page_reference), direct_link_reference: keep(promotion.direct_link_reference),
    product_reference: keep(promotion.product_reference), native_anchor_reference: undefined,
    creative_component_references: promotion.creative_component_references?.filter(reference => keep(reference)),
    delivery_identity: { ...promotion.delivery_identity, authorized_identity: undefined },
    settings: { ...promotion.settings, category_reference: undefined, brand_reference: undefined },
  })) }
}
