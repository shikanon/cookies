import type { DeliveryPlanDraft } from '../api/delivery'
import type { FillingField, FillingRequest } from '../api/deliveryFilling'
import { applyFillingOcean, fillingValues } from '../lib/deliveryFilling'
import { applyPlanConfiguration, planCreativeReference, planFormConfiguration } from '../lib/deliveryPlanForm'
import { DeliveryFillingPanel } from './DeliveryFillingPanel'

export function DeliveryPlanFilling({ projectId, planId, draft, changeDraft, disabled }: {
  projectId: string; planId?: string; draft: DeliveryPlanDraft; changeDraft: (update: (current: DeliveryPlanDraft) => DeliveryPlanDraft) => void; disabled?: boolean;
}) {
  const ocean = planFormConfiguration(projectId, draft)
  const fields: FillingField[] = ['name', 'objective', 'account', 'marketing_purpose', 'product', 'carrier', 'optimization_target', ...(draft.schedule.mode === 'fixed_range' ? ['total_budget' as const] : []), 'daily_budget', 'bid', 'schedule', 'monitoring', ...(!planId ? ['materials' as const] : [])]
  const request: FillingRequest = { page: 'plan', plan_id: planId, ocean, fields, current: { ...fillingValues(ocean), name: draft.name, objective: draft.objective, total_budget: draft.budget.totalMinor, materials: draft.creativeReferences.flatMap(item => item.reference ? [item.reference] : []) } }
  return <DeliveryFillingPanel projectId={projectId} request={request} disabled={disabled} history={draft.fillingHistory} onApply={acceptance => changeDraft(current => {
    const next = applyPlanConfiguration(current, applyFillingOcean(planFormConfiguration(projectId, current), acceptance.result.suggestions))
    for (const suggestion of acceptance.result.suggestions) {
      if (suggestion.field === 'name') next.name = suggestion.value
      if (suggestion.field === 'objective') next.objective = suggestion.value
      if (suggestion.field === 'total_budget') next.budget = { ...next.budget, totalMinor: suggestion.value }
      if (suggestion.field === 'schedule') next.schedule = { mode: suggestion.value.mode ?? 'fixed_range', startAt: suggestion.value.start_at, endAt: suggestion.value.end_at, timezone: suggestion.value.timezone }
      if (suggestion.field === 'materials' && !planId) next.creativeReferences = suggestion.value.map(planCreativeReference)
    }
    next.fillingHistory = [...(current.fillingHistory ?? []), acceptance]
    return next
  })}/>
}
