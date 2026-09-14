import type { PlatformConfiguration } from '../api/delivery'
import type { FillingField } from '../api/deliveryFilling'
import { applyFillingOcean, fillingValues } from '../lib/deliveryFilling'
import { DeliveryFillingPanel } from './DeliveryFillingPanel'

export function DeliveryConfigurationFilling({ projectId, planId, value, targetId, disabled, onChange }: {
  projectId: string; planId: string; value: PlatformConfiguration; targetId?: string; disabled?: boolean; onChange: (value: PlatformConfiguration) => void;
}) {
  const ocean = value.payload.ocean_engine
  if (!ocean) return null
  const native = ocean.project.marketing_purpose === 'content_marketing' && ocean.project.carrier === 'douyin_account'
  const promotion = ocean.promotions.find(item => item.promotion_draft_id === targetId)
  const fields: FillingField[] = targetId
    ? native ? ['name', 'materials', ...(promotion?.settings.title_mode === 'manual' ? ['copy' as const] : [])] : ['name', 'materials', 'product_name', 'selling_points', 'copy', 'landing_page', 'identity']
    : ['name', 'account', 'marketing_purpose', 'product', 'carrier', 'optimization_target', 'daily_budget', 'bid', 'schedule', 'monitoring']
  return <DeliveryFillingPanel projectId={projectId} disabled={disabled} request={{ page: 'configuration', plan_id: planId, target_id: targetId, ocean, fields, current: fillingValues(ocean, targetId) }} history={value.compilation_metadata.filling_history?.filter(item => item.target_id === targetId)} onApply={acceptance => {
    onChange({ ...value, payload: { ...value.payload, ocean_engine: applyFillingOcean(ocean, acceptance.result.suggestions, targetId) }, compilation_metadata: { ...value.compilation_metadata, filling_history: [...(value.compilation_metadata.filling_history ?? []), acceptance] } })
  }}/>
}
