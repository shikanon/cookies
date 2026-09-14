import { toPlatformRuntimeDraft, type DeliveryPlanDraft, type StableReference } from '../api/delivery'
import type { OceanConfiguration } from './deliveryChoices'

export function planFormConfiguration(projectId: string, draft: DeliveryPlanDraft): OceanConfiguration {
  return toPlatformRuntimeDraft(projectId, 'form', 1, draft).platform_configuration.payload.ocean_engine!
}

export function planCreativeReference(reference: StableReference): DeliveryPlanDraft['creativeReferences'][number] {
  return { reference: structuredClone(reference), assetId: reference.id ?? '', version: Number(reference.version ?? 1), contentHash: reference.content_hash, confirmed: reference.state === 'resolved', oceanEngineMaterialId: reference.audit_attributes?.ocean_engine_material_id }
}

export function applyPlanConfiguration(draft: DeliveryPlanDraft, ocean: OceanConfiguration): DeliveryPlanDraft {
  const project = ocean.project
  const product = project.marketing_product_reference
  const optimization = project.optimization_target_reference
  const days = draft.schedule.mode === 'long_term' ? 1 : Math.max(1, Math.ceil((Date.parse(draft.schedule.endAt) - Date.parse(draft.schedule.startAt)) / 86400000))
  const previousDaily = Math.floor(draft.budget.totalMinor / days)
  return {
    ...draft, platformProject: structuredClone(project),
    marketingPurpose: project.marketing_purpose as DeliveryPlanDraft['marketingPurpose'],
    marketingProduct: { reference: product, id: product?.id ?? '', name: product?.display_name_snapshot ?? '', oceanEngineProductId: product?.audit_attributes?.ocean_engine_product_id, activityType: product?.audit_attributes?.activity_type ?? '', activityName: product?.audit_attributes?.activity_name ?? '', brandName: product?.audit_attributes?.brand_name ?? '' },
    advertiser: { ...draft.advertiser, id: project.account_reference.id ?? '', name: project.account_reference.display_name_snapshot ?? '' },
    budget: project.budget_and_bidding.daily_budget_minor === previousDaily ? draft.budget : { ...draft.budget, totalMinor: project.budget_and_bidding.daily_budget_minor * days },
    tracking: {
      ...draft.tracking, deliveryCarrier: project.carrier,
      landingPage: project.carrier === draft.tracking.deliveryCarrier ? draft.tracking.landingPage : '',
      optimizationTargetReference: optimization,
      optimizationTargetId: optimization?.id ?? '', optimizationTargetName: optimization?.display_name_snapshot ?? '', optimizationTargetSemanticKey: optimization?.semantic_key ?? '',
      eventAssetName: optimization?.audit_attributes?.event_asset_name ?? '', eventAssetType: optimization?.audit_attributes?.event_asset_type ?? '',
      ...(project.account_reference.id !== draft.advertiser.id ? { monitoringImpression: '', monitoringValidTouch: '', monitoringVideoPlay: '', monitoringVideoComplete: '', monitoringValidVideoPlay: '' } : {}),
    },
    creativeReferences: ocean.promotions.flatMap(promotion => promotion.base_material_references).map(planCreativeReference),
  }
}
