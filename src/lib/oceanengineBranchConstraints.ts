import type { PlatformConfiguration } from '../api/delivery'
import type { ApiOptimizationTargetContext } from '../data/api'

type OceanProject = NonNullable<PlatformConfiguration['payload']['ocean_engine']>['project']

export function oceanEngineLeadCaptureMode(project: OceanProject): 'smart_lead' | 'custom_lead' {
  if (project.lead_capture_mode === 'smart_lead' || project.lead_capture_mode === 'custom_lead') {
    return project.lead_capture_mode
  }
  if (project.carrier === 'owned_landing_page' || project.carrier === 'im') {
    return 'custom_lead'
  }
  return 'smart_lead'
}

// OceanEngine computes lead-generation optimization targets from the complete
// parent form branch. These values come from the observed request matrix.
export function oceanEngineOptimizationTargetContext(project: OceanProject): ApiOptimizationTargetContext | undefined {
  if (project.marketing_purpose !== 'lead_generation') return undefined
  const leadCaptureMode = oceanEngineLeadCaptureMode(project)
  const base = {
    campaign_type: 1,
    landing_type: 1,
    micro_app_id: '',
    cdp_marketing_goal: 1,
    dpa_ad_type: 0,
    micro_promotion_type: 2,
    micro_app_instance_id: '',
  }
  if (project.carrier === 'orange_landing_page_and_im') {
    return { ...base, asset_type: 2, multi_asset_types: [2, 1002], need_assets: false }
  }
  if (project.carrier === 'orange_landing_page') {
    return {
      ...base,
      asset_type: 2,
      ...(leadCaptureMode === 'smart_lead' ? { multi_asset_types: [2] } : {}),
      need_assets: false,
    }
  }
  if (project.carrier === 'owned_landing_page') {
    return { ...base, asset_type: 3, need_assets: true }
  }
  if (project.carrier === 'im') {
    return { ...base, asset_type: 1002, need_assets: false }
  }
  return undefined
}

export function optimizationCapabilitySelectionMatches(project: Pick<OceanProject, 'optimization_target_reference'>, snapshotID: string, availableExternalActions: readonly string[]) {
  const reference = project.optimization_target_reference
  return Boolean(
    reference?.id
      && reference.audit_attributes?.capability_snapshot_id === snapshotID
      && availableExternalActions.includes(reference.id),
  )
}
