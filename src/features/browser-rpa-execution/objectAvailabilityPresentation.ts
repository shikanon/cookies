import type { RunnerV3Plan } from './model'

type ObjectAvailabilityItem = NonNullable<RunnerV3Plan['object_availability']>[number]

export type ObjectAvailabilityPresentation = {
  available: boolean
  kindLabel: string
  name: string
  scopeLabel: string
  statusLabel: string
  statusDetail: string
  platformId?: string
  technicalType: string
}

const OBJECT_KIND_LABELS: Record<string, string> = {
  application: '应用',
  brand: '品牌',
  category: '行业分类',
  creative_component: '创意组件',
  delivery_identity: '投放身份',
  direct_link: '直达链接',
  industry_category: '行业分类',
  landing_page: '落地页',
  material: '视频素材',
  native_anchor: '原生锚点',
  owned_landing_page: '落地页',
  product: '营销商品',
  product_catalog: '商品库',
  product_image: '商品图片',
  video_material: '视频素材',
}

function isUrl(value: string | undefined): boolean {
  if (!value) return false
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' || url.protocol === 'tbopen:'
  } catch {
    return false
  }
}

function urlHost(value: string): string {
  try {
    return new URL(value).hostname
  } catch {
    return ''
  }
}

function kindLabel(item: ObjectAvailabilityItem): string {
  if (item.field_key === 'project.marketing_product_reference') return '营销商品'
  if (item.field_key.includes('base_material_references')) return '视频素材'
  return OBJECT_KIND_LABELS[item.object_kind] || '平台对象'
}

function scopeLabel(fieldKey: string): string {
  const promotion = /^promotions\.(\d+)\./.exec(fieldKey)
  if (promotion) return `单元 ${Number(promotion[1]) + 1}`
  if (fieldKey.startsWith('project.')) return '项目'
  return '投放计划'
}

function displayName(item: ObjectAvailabilityItem, label: string): string {
  const candidate = item.display_name || item.internal_object_id
  if (isUrl(candidate)) {
    const host = urlHost(candidate)
    return host ? `${host} ${label}` : label
  }
  return candidate || label
}

export function presentObjectAvailability(item: ObjectAvailabilityItem): ObjectAvailabilityPresentation {
  const label = kindLabel(item)
  const name = displayName(item, label)
  const publicPlatformId = isUrl(item.platform_object_id) ? undefined : item.platform_object_id

  if (item.available) {
    const isManualDirectLink = item.object_kind === 'direct_link' && item.reason === '手动填写链接，无需绑定平台 ID'
    return {
      available: true,
      kindLabel: label,
      name,
      scopeLabel: scopeLabel(item.field_key),
      statusLabel: isManualDirectLink ? '可直接填写' : '已绑定',
      statusDetail: isManualDirectLink ? '手动链接可用，无需绑定平台 ID。' : publicPlatformId ? '平台对象可用。' : `${label}可用。`,
      platformId: publicPlatformId,
      technicalType: item.object_kind,
    }
  }

  return {
    available: false,
    kindLabel: label,
    name,
    scopeLabel: scopeLabel(item.field_key),
    statusLabel: '需处理',
    statusDetail: `请先为“${name}”绑定巨量${label} ID。`,
    technicalType: item.object_kind,
  }
}

export function presentPlanBlockedReason(reason: string): string {
  if (reason === 'PLATFORM_OBJECTS_UNAVAILABLE') {
    return '有巨量对象尚未绑定。请处理下方标记为“需处理”的对象。'
  }
  if (reason === 'DELIVERY_CONFIGURATION_INVALID') {
    return '投放配置不完整。请补充下方字段，然后重新生成计划。'
  }
  return reason
}

export function presentConfigurationIssue(issue: string): string {
  if (/unsupported account path: marketing purpose .* is not calibrated/i.test(issue)) {
    return '当前营销目的不在校准清单中。请返回平台配置页并选择已校准的营销目的。'
  }
  const bidLimit = issue.match(/project: bid is outside the calibrated limit for (CPC|CPM|OCPC|OCPM): expected CNY ([0-9.]+) to ([0-9.]+)/i)
  if (bidLimit) {
    return `项目出价超出当前 Runner 的校准范围。${bidLimit[1].toUpperCase()} 出价必须是 ${Number(bidLimit[2])} 至 ${Number(bidLimit[3])} 元。`
  }
  if (/project: bid is outside the calibrated limit/i.test(issue)) {
    return '项目出价超出当前 Runner 的校准范围。请返回平台配置页检查当前计费方式和出价范围。'
  }
  if (/configuration has no calibrated optimization target key/i.test(issue)) {
    return '优化目标不在当前 Runner 的校准范围内。请返回平台配置页并重新选择优化目标。'
  }
  if (/lead_generation with owned_landing_page requires project\.optimization_target_reference click or impression and project\.delivery_mode ubmax/i.test(issue)) {
    return '销售线索使用自研落地页时，优化目标只能选择“点击量”或“展示量”，投放模式必须选择“UBMax”。'
  }
  if (/marketing product: reference .* is outside the delivery intent/i.test(issue)) {
    return '营销商品未加入投放意图。请返回平台配置页，保存当前配置并创建新执行。'
  }
  if (/base material: reference .* is outside the delivery intent/i.test(issue)) {
    return '基础素材未加入投放意图。请返回平台配置页，保存当前配置并创建新执行。'
  }
  if (/product image: reference .* is outside the delivery intent/i.test(issue)) {
    return '产品主图未加入投放意图。请返回平台配置页，保存当前配置并创建新执行。'
  }
  if (/product image requires a stable image_src_identity/i.test(issue)) {
    return '产品主图缺少稳定图片路径。请重新同步巨量对象目录，然后重新选择产品主图。'
  }
  if (/landing page: reference .* is outside the delivery intent/i.test(issue)) {
    return '落地页未加入投放意图。请返回平台配置页，保存当前配置并创建新执行。'
  }
  if (/requires copy, source, and name/i.test(issue)) {
    return '投放单元缺少必要字段。请补充单元文案、素材来源和单元名称。'
  }
  if (/call to action needs 1 to 10 unique values/i.test(issue)) {
    return '投放单元缺少行动号召。请填写 1 至 10 个不重复的行动号召。'
  }
  if (/exactly one bound base material/i.test(issue)) {
    return '投放单元的基础素材数量不正确。请选择一个可用素材。'
  }
  if (/daily budget must be at least CNY 300/i.test(issue)) {
    return '日预算过低。请将对应项目或单元的日预算设为至少 300 元。'
  }
  const dateBoundary = issue.match(/project start date (\d{4}-\d{2}-\d{2}) must be no earlier than (\d{4}-\d{2}-\d{2})/i)
  if (dateBoundary) {
    return `此执行记录的项目开始日期是 ${dateBoundary[1]}。最早允许日期是 ${dateBoundary[2]}。`
  }
  if (/project start date must be no earlier than the next day/i.test(issue)) {
    return '项目开始日期过早。请将开始日期设为次日或更晚。'
  }
  return '投放配置未通过执行前检查。请返回平台配置页并检查标记为“必填”的字段。'
}
