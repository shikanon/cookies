import { useState } from 'react'
import type { PlatformConfiguration } from '../api/delivery'
import type { FillingField } from '../api/deliveryFilling'
import { applyFillingOcean, fillingValues } from '../lib/deliveryFilling'
import { DeliveryFillingPanel } from './DeliveryFillingPanel'

export function DeliveryConfigurationFilling({ projectId, planId, value, boundObjects, disabled, onChange }: {
  projectId: string; planId: string; value: PlatformConfiguration; boundObjects: ReadonlySet<string>; disabled?: boolean; onChange: (value: PlatformConfiguration) => void;
}) {
  const [selectedTarget, setSelectedTarget] = useState('')
  const ocean = value.payload.ocean_engine
  if (!ocean) return null
  const targets = ocean.promotions.map((item, index) => ({ id: item.promotion_draft_id, label: `推广单元 ${index + 1} · ${item.promotion_name || '未命名'}`, disabled: boundObjects.has(item.promotion_draft_id) }))
  const targetId = targets.find(item => item.id === selectedTarget && !item.disabled)?.id ?? targets.find(item => !item.disabled)?.id ?? targets[0]?.id
  const projectLocked = boundObjects.has(ocean.project.project_draft_id)
  const native = ocean.project.marketing_purpose === 'content_marketing' && ocean.project.carrier === 'douyin_account'
  const promotion = ocean.promotions.find(item => item.promotion_draft_id === targetId)
  if (!promotion) return <section className="delivery-filling"><button type="button" className="secondary-button" disabled>智能填写</button><small>请先增加推广单元。</small></section>
  const fields: FillingField[] = native
    ? ['name', 'materials', 'source_label', 'category', 'search_terms', 'regions', 'age_ranges', 'gender', 'smart_expansion', ...(promotion.settings.title_mode === 'manual' ? ['copy' as const] : [])]
    : ['name', 'search_keywords', 'search_expansion', 'regions', 'age_ranges', 'gender', 'smart_expansion', 'materials', 'product_name', 'product_images', 'selling_points', 'copy', 'source_label', 'call_to_action', 'category', ...(ocean.project.carrier === 'orange_landing_page' || ocean.project.carrier === 'orange_landing_page_and_im' ? ['landing_page' as const] : []), ...(promotion.delivery_identity.mode === 'douyin_account' ? ['identity' as const] : [])]
  const editableFields = projectLocked ? fields.filter(field => !['search_keywords', 'search_expansion', 'regions', 'age_ranges', 'gender', 'smart_expansion'].includes(field)) : fields
  const financialHistory = value.compilation_metadata.filling_history?.filter(item => item.target_id === targetId && item.result.finance).at(-1)?.result.finance
  return <DeliveryFillingPanel financialHistory={financialHistory} targets={targets} onTargetChange={setSelectedTarget} projectId={projectId} disabled={disabled || boundObjects.has(promotion.promotion_draft_id)} request={{ page: 'configuration', plan_id: planId, target_id: targetId, ocean, fields: editableFields, current: fillingValues(ocean, targetId) }} onApply={acceptance => {
    onChange({ ...value, payload: { ...value.payload, ocean_engine: applyFillingOcean(ocean, acceptance.result.suggestions, targetId) }, compilation_metadata: { ...value.compilation_metadata, filling_history: [...(value.compilation_metadata.filling_history ?? []), acceptance] } })
  }}/>
}
