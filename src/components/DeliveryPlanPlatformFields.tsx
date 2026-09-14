import { useCallback } from 'react'
import type { DeliveryPlanDraft } from '../api/delivery'
import type { ApiAssetVersionPointer, ApiConnectorAccount } from '../data/api'
import { changeConfigurationAccount, changeConfigurationProject, changeProjectChoice, type OceanProject } from '../lib/deliveryChoices'
import { applyPlanConfiguration, planFormConfiguration, planCreativeReference } from '../lib/deliveryPlanForm'
import { oceanEngineLeadCaptureMode } from '../lib/oceanengineBranchConstraints'
import { resolveOceanEngineChargingMode } from '../lib/oceanengineBidConstraints'
import { AccountChoice, CarrierChoice, MarketingPurposeChoice, OptimizationChoice, useOptimizationCapabilities } from './DeliveryChoiceFields'
import { BaseMaterialsField, MarketingProductPicker, ReferenceObjectPicker } from './DeliveryObjectPickers'
import { useDeliveryCatalog } from './useDeliveryCatalog'

export function DeliveryPlanPlatformFields({ section, projectId, draft, changeDraft, accounts, products, assets, isNew }: {
  section: 'target' | 'tracking' | 'materials'; projectId: string; draft: DeliveryPlanDraft;
  changeDraft: (update: (draft: DeliveryPlanDraft) => DeliveryPlanDraft) => void;
  accounts: ApiConnectorAccount[]; products: Array<{ id: string; name: string; oceanEngineProductId?: string }>;
  assets: ApiAssetVersionPointer[]; isNew: boolean;
}) {
  const ocean = planFormConfiguration(projectId, draft)
  const project = ocean.project
  const catalog = useDeliveryCatalog(projectId, draft.advertiser.id, accounts)
  const loadNativeVideos = useCallback((query: string, cursor: string | undefined, sortBy: 'created_at' | 'ctr' | 'conversions', sortOrder: 'asc' | 'desc') => catalog.loadDouyinVideos('', query, cursor, sortBy, sortOrder), [catalog.loadDouyinVideos])
  const capabilities = useOptimizationCapabilities(project, catalog.loadOptimizationCapabilities)
  const updateProject = (patch: Partial<OceanProject>) => changeDraft(current => {
    const configuration = planFormConfiguration(projectId, current)
    return applyPlanConfiguration(current, changeConfigurationProject(configuration, changeProjectChoice(configuration.project, patch)))
  })
  const native = project.marketing_purpose === 'content_marketing' && project.carrier === 'douyin_account'
  if (section === 'target') return <>
    <fieldset className="delivery-plan-choice-context">
      <AccountChoice value={project.account_reference} accounts={accounts} onChange={id => {
        const account = accounts.find(item => item.id === id)
        if (account) changeDraft(current => applyPlanConfiguration(current, changeConfigurationAccount(planFormConfiguration(projectId, current), account)))
      }}/>
      <MarketingPurposeChoice value={project.marketing_purpose} onChange={marketing_purpose => updateProject({ marketing_purpose })}/>
    </fieldset>
    {!isNew ? <small>切换账户或营销目的后，请到平台配置确认受影响的单元。</small> : null}
    <MarketingProductPicker key={draft.advertiser.id} value={project.marketing_product_reference} cookiesProducts={products} loadPlatformObjects={catalog.loadProducts} onChange={marketing_product_reference => updateProject({ marketing_product_reference })}/>
  </>
  if (section === 'tracking') return <>
    <fieldset className="delivery-plan-choice-context">
      {project.marketing_purpose === 'lead_generation' ? <label><span>获取线索方式</span><select aria-label="获取线索方式" value={oceanEngineLeadCaptureMode(project)} onChange={event => updateProject({ lead_capture_mode: event.target.value })}><option value="smart_lead">智能优选</option><option value="custom_lead">自定义</option></select></label> : null}
      {project.marketing_purpose !== 'application' ? <CarrierChoice project={project} onChange={carrier => updateProject({ carrier })}/> : <small>应用暂不支持。请在平台配置中查看历史应用设置。</small>}
    </fieldset>
    {!isNew ? <small>切换载体或获客方式后，请到平台配置确认落地页和单元素材。</small> : null}
    <OptimizationChoice key={`${draft.advertiser.id}:${project.carrier}:${project.marketing_purpose}:${project.lead_capture_mode}`} project={project} capabilities={capabilities} loadObjects={catalog.loadOptimizationTargets} onChange={optimization_target_reference => {
      const charging_mode = resolveOceanEngineChargingMode(optimization_target_reference, project.budget_and_bidding.charging_mode)
      updateProject({ optimization_target_reference, ...(charging_mode ? { budget_and_bidding: { ...project.budget_and_bidding, charging_mode } } : {}) })
    }}/>
  </>
  if (!isNew) return null
  const value = ocean.promotions.flatMap(promotion => promotion.base_material_references)
  return native
    ? <ReferenceObjectPicker key={draft.advertiser.id} label="抖音原生视频" pickerTitle="选择已同步视频" value={value[0]} objectKind="douyin_video" loadPlatformObjects={loadNativeVideos} onChange={reference => changeDraft(current => ({ ...current, creativeReferences: reference ? [planCreativeReference(reference)] : [] }))}/>
    : <BaseMaterialsField key={draft.advertiser.id} value={value} assets={assets} loadVideos={catalog.loadVideos} loadImages={catalog.loadImages} loadPhotos={catalog.loadPhotos} onChange={references => changeDraft(current => ({ ...current, creativeReferences: references.map(planCreativeReference) }))}/>
}
