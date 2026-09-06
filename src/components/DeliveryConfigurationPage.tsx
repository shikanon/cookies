import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Check, CircleAlert, Package, Plus, RefreshCw, Save, Trash2 } from 'lucide-react'
import {
  DeliveryApiError,
  deliveryPlanApi,
  deliveryExecutionApi,
  type DeliveryPlan,
  type DeliveryExecutionDriver,
  type PlatformConfiguration,
  type StableReference,
} from '../api/delivery'
import { useProject } from '../context/ProjectContext'
import { ApiRequestError, api, type ApiAssetVersionPointer, type ApiConnectorAccount, type ApiConnectorPlatformObject, type ApiConnectorPlatformObjectKind, type ApiOptimizationTargetCapabilitySnapshot, type ApiOptimizationTargetContext } from '../data/api'
import { oceanEngineCalibrationDispositions, visibleOceanEngineManifestFields, type CalibrationDisposition, type VisibleManifestField } from '../lib/oceanengineCalibrationManifest'
import { fromShanghaiEndDate, fromShanghaiStartDate, toShanghaiDateInput } from '../lib/deliverySchedule'
import { carrierUsesOrangeLandingPage, changeOceanEngineCarrier, normalizeOceanEngineLandingPages } from '../lib/deliveryCarrier'
import { isOceanEngineImageSourceIdentity, oceanEngineImageSourceIdentity } from '../lib/oceanengine-product-image'
import { oceanEngineLeadCaptureMode, oceanEngineOptimizationTargetContext, optimizationCapabilitySelectionMatches } from '../lib/oceanengineBranchConstraints'
import { formatOceanEngineMoneyRange, resolveOceanEngineBidConstraint, resolveOceanEngineChargingMode } from '../lib/oceanengineBidConstraints'
import { projectPath } from '../lib/router'
import type { DataState } from '../types'
import { StateBoundary } from './StateBoundary'

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) : '暂无记录'
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof ApiRequestError) return error.message
  if (error instanceof DeliveryApiError) {
    if (error.code === 'LEGACY_CONFIGURATION_UNSUPPORTED') return '这份历史配置仅供查看，不能继续提交或修改。请新建投放计划。'
    if (error.code === 'VERSION_CONFLICT') return '计划版本已更新，请刷新后重试。'
    return error.message
  }
  return error instanceof Error ? error.message : fallback
}

function formatManifestValue(value: unknown, unit?: string, valueLabels: Record<string, string> = {}, propertyLabels: Record<string, string> = {}): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'number' && unit === 'CNY_fen') return (value / 100).toLocaleString('zh-CN', { style: 'currency', currency: 'CNY' })
  if (typeof value === 'boolean') return value ? '开启' : '关闭'
  if (typeof value === 'string' || typeof value === 'number') return valueLabels[String(value)] ?? String(value)
  if (Array.isArray(value)) return value.map(item => formatManifestValue(item, unit, valueLabels, propertyLabels)).filter(Boolean).join('、')
  if (typeof value === 'object') {
    const record = value as Record<string, unknown>
    if (typeof record.display_name_snapshot === 'string' && record.display_name_snapshot) return record.display_name_snapshot
    if (typeof record.text === 'string') return record.text
    if (typeof record.id === 'string' && record.id) return '已选择平台对象'
    if (typeof record.start_at === 'string' && typeof record.end_at === 'string') {
      const start = new Date(record.start_at).toLocaleDateString('zh-CN')
      const end = new Date(record.end_at).toLocaleDateString('zh-CN')
      return `${start} — ${end}${typeof record.timezone === 'string' ? ` · ${record.timezone}` : ''}`
    }
    return Object.entries(record).flatMap(([key, entry]) => {
      if (typeof entry !== 'string' && typeof entry !== 'number' && typeof entry !== 'boolean') return []
      const label = propertyLabels[key] ?? key
      return [`${label}：${formatManifestValue(entry, undefined, valueLabels, propertyLabels)}`]
    }).join(' · ')
  }
  return ''
}

function ManifestFieldList({ fields }: { fields: VisibleManifestField[] }) {
  if (!fields.length) return null
  return <dl className="delivery-config-project-facts">{fields.map(field => <div key={field.key}><dt>{field.label}</dt><dd>{formatManifestValue(field.value, field.unit, field.valueLabels, field.propertyLabels)}</dd></div>)}</dl>
}

const dispositionLabels: Record<CalibrationDisposition['state'], string> = {
  ready: '可手动配置',
  evidence_only: '仅校准证据',
  blocked: '已阻断',
  platform_pending: '等待平台条件',
  condition_unmet: '当前条件未满足',
  missing_value: '缺少当前值',
}

function CalibrationDispositionList({ title, items }: { title: string; items: CalibrationDisposition[] }) {
  return <section className="delivery-config-disposition-group"><h4>{title}</h4><ol>{items.map(item => <li key={item.key}>
    <div><b>{item.label}</b><code>{item.key}</code></div>
    <strong data-state={item.state}>{dispositionLabels[item.state]}</strong>
    <p>{item.reason}</p>
  </li>)}</ol></section>
}

function CalibrationDispositionView({ value }: { value: PlatformConfiguration }) {
  if (value.platform !== 'ocean_engine' || !value.payload.ocean_engine) return <div className="delivery-config-empty-inline"><CircleAlert size={18}/>当前平台没有可读取的字段校准记录。</div>
  const configuration = value.payload.ocean_engine
  return <section className="delivery-config-calibration-card">
    <header><div><span className="section-label">只读</span><h3>字段校准与处置</h3><p>状态和原因直接来自冻结 Manifest。此视图不填写、不保存、不提交平台表单。</p></div></header>
    <CalibrationDispositionList title="项目字段" items={oceanEngineCalibrationDispositions(configuration, 'project')}/>
    <CalibrationDispositionList title="推广单元字段" items={oceanEngineCalibrationDispositions(configuration, 'promotion')}/>
  </section>
}

function PlatformConfigurationDetails({ value }: { value: PlatformConfiguration }) {
  if (value.platform === 'magnetic_engine') {
    return <div className="delivery-config-empty-inline"><CircleAlert size={18}/><div><b>磁力引擎能力尚未开放</b><p>{value.payload.magnetic_engine?.reason}</p></div></div>
  }
  const ocean = value.payload.ocean_engine
  if (!ocean?.project) return <div className="delivery-config-empty-inline"><CircleAlert size={18}/>平台配置缺少主投放项目。</div>
  const project = ocean.project
  const projectFields = visibleOceanEngineManifestFields(ocean, 'project')
  return <div className="delivery-config-business-map">
    <section className="delivery-config-project-card">
      <header><div><span className="delivery-config-eyebrow">主投放项目</span><h4>{project.project_name}</h4><p>预算、排期和营销目标在此项目下统一生效。</p></div><strong className="delivery-config-ready-state">配置已就绪</strong></header>
      <ManifestFieldList fields={projectFields}/>
    </section>
    <section className="delivery-config-promotion-section">
      <header><div><span className="delivery-config-eyebrow">推广单元</span><h4>素材与文案组合</h4><p>所有推广单元均归属于上方主投放项目。</p></div><strong>{ocean.promotions.length} 个</strong></header>
      {ocean.promotions.length ? <div className="delivery-config-promotion-grid">{ocean.promotions.map((promotion, index) => <article key={promotion.promotion_draft_id}>
        <header><span>推广单元 {index + 1}</span><h5>{promotion.promotion_name}</h5></header>
        <ManifestFieldList fields={visibleOceanEngineManifestFields(ocean, 'promotion', promotion)}/>
      </article>)}</div> : <div className="delivery-config-empty-inline">暂未添加推广单元，可稍后从投放计划补充素材与文案。</div>}
    </section>
  </div>
}

type OceanConfiguration = NonNullable<PlatformConfiguration['payload']['ocean_engine']>
type OceanPromotion = OceanConfiguration['promotions'][number]
type PromotionRequiredField = 'promotion_name' | 'base_materials' | 'copy_items' | 'landing_page' | 'direct_link' | 'product_name' | 'product_images' | 'product_selling_points' | 'call_to_action' | 'source_label' | 'category'

function multiLeadLandingActions(metadata: Record<string, unknown> | undefined): string[] {
  if (!metadata) return []
  const actions = metadata.multi_lead_external_actions
  if (Array.isArray(actions)) return actions.map(String)
  if (typeof actions === 'string') return actions.split(',').map(value => value.trim()).filter(Boolean)
  return metadata.multi_conversion_eligible === true ? ['100'] : []
}

function referenceSupportsMultiLeadAction(reference: StableReference | undefined, action: string): boolean {
  if (!reference || !action) return false
  const actions = reference.audit_attributes?.multi_lead_external_actions?.split(',').map(value => value.trim()) ?? []
  return actions.includes(action) || (action === '100' && reference.audit_attributes?.multi_conversion_eligible === 'true')
}

function requiredMultiLeadLandingAction(project: OceanConfiguration['project']): string {
  if (project.marketing_purpose !== 'lead_generation' || oceanEngineLeadCaptureMode(project) !== 'smart_lead' || !carrierUsesOrangeLandingPage(project.carrier)) return ''
  return project.optimization_target_reference?.id?.trim() ?? ''
}

const promotionRequiredFieldLabels: Record<PromotionRequiredField, string> = {
  promotion_name: '单元名称',
  base_materials: '基础素材',
  copy_items: '广告文案',
  landing_page: '落地页',
  direct_link: '直达链接',
  product_name: '产品名称',
  product_images: '产品主图',
  product_selling_points: '产品卖点',
  source_label: '来源',
  call_to_action: '行动号召',
  category: '所属类别',
}

function missingPromotionRequiredFields(promotion: OceanPromotion, project: OceanConfiguration['project']): PromotionRequiredField[] {
  const missing: PromotionRequiredField[] = []
  if (!promotion.promotion_name.trim()) missing.push('promotion_name')
  if (!promotion.base_material_references.length) missing.push('base_materials')
  if (!promotion.copy_items.some(item => item.text.trim())) missing.push('copy_items')
  const requiredLandingAction = requiredMultiLeadLandingAction(project)
  if ((carrierUsesOrangeLandingPage(project.carrier) || project.carrier === 'owned_landing_page') && (!promotion.landing_page_reference?.id || (requiredLandingAction && !referenceSupportsMultiLeadAction(promotion.landing_page_reference, requiredLandingAction)))) missing.push('landing_page')
  if (promotion.settings.direct_link_mode === 'manual' && !promotion.direct_link_reference?.id?.trim()) missing.push('direct_link')
  if (!promotion.product_name?.trim() && !project.marketing_product_reference?.display_name_snapshot?.trim()) missing.push('product_name')
  if (!promotion.product_image_references?.some(reference => reference.object_kind === 'product_image')) missing.push('product_images')
  if (!promotion.product_selling_points?.some(value => value.trim())) missing.push('product_selling_points')
  if (!promotion.settings.source_label?.trim()) missing.push('source_label')
  if (!promotion.settings.call_to_action?.some(value => value.trim())) missing.push('call_to_action')
  if (!promotion.settings.category_reference?.id) missing.push('category')
  return missing
}

function RequiredFieldLabel({ label, missing }: { label: string; missing: boolean }) {
  return <span className="delivery-config-required-label">{label}<em>{missing ? '必填 · 待补' : '必填'}</em></span>
}

function updateReference(current: StableReference | undefined, id: string, objectKind: string): StableReference | undefined {
  const value = id.trim()
  if (!value) return undefined
  return { ...current, namespace: 'cookies', object_kind: objectKind, scope: current?.scope ?? 'current_project', state: 'resolved', id: value }
}

function intentHasReference(references: StableReference[] | undefined, selected: StableReference): boolean {
  return Boolean(selected.id && references?.some(reference =>
    reference.state === 'resolved' && reference.namespace === selected.namespace && reference.object_kind === selected.object_kind && reference.id === selected.id,
  ))
}

function configuredReferenceIntentIssues(configuration: PlatformConfiguration | undefined, plan: DeliveryPlan | undefined): string[] {
  const ocean = configuration?.payload.ocean_engine
  const intent = plan?.currentVersion.deliveryIntent?.payload
  if (!ocean || !intent) return []
  const issues: string[] = []
  const product = ocean.project.marketing_product_reference
  if (product && !intentHasReference(intent.product_references, product)) issues.push('营销商品')
  ocean.promotions.forEach((promotion, index) => {
    promotion.base_material_references.forEach(reference => {
      if (!intentHasReference(intent.material_references, reference)) issues.push(`推广单元 ${index + 1} · 基础素材`)
    })
    promotion.product_image_references?.forEach(reference => {
      if (!intentHasReference(intent.material_references, reference)) issues.push(`推广单元 ${index + 1} · 产品主图`)
      if (!reference.audit_attributes?.image_src_identity) issues.push(`推广单元 ${index + 1} · 产品主图身份引用`)
    })
    const landingPage = promotion.landing_page_reference
    const isOwnedLandingPage = ocean.project.carrier === 'owned_landing_page' && landingPage?.object_kind === 'owned_landing_page'
    if (landingPage && !isOwnedLandingPage && !intentHasReference(intent.landing_page_references, landingPage)) issues.push(`推广单元 ${index + 1} · 落地页`)
  })
  return [...new Set(issues)]
}

function normalizeProjectExecutionDefaults(configuration: PlatformConfiguration): PlatformConfiguration {
  const next = structuredClone(configuration)
  const project = next.payload.ocean_engine?.project
  if (!project) return next
  project.deep_optimization_mode ||= 'disabled'
  if (project.marketing_purpose === 'lead_generation') project.delivery_mode = 'ubmax'
  if (project.delivery_mode === 'manual' && (!project.placement_strategy || project.placement_strategy === 'smart')) project.placement_strategy = 'automatic'
  const chargingMode = resolveOceanEngineChargingMode(project.optimization_target_reference, project.budget_and_bidding.charging_mode)
  if (chargingMode) project.budget_and_bidding.charging_mode = chargingMode
  return next
}

type PlatformObjectPage = { items: ApiConnectorPlatformObject[]; next_cursor: string }
type PlatformObjectSort = 'created_at' | 'ctr' | 'conversions'
type PlatformObjectLoader = (query: string, cursor: string | undefined, sortBy: PlatformObjectSort, sortOrder: 'asc' | 'desc') => Promise<PlatformObjectPage>

async function addProductImagePickerEvidence(configuration: PlatformConfiguration, loadProductImages: PlatformObjectLoader): Promise<PlatformConfiguration> {
  const ocean = configuration.payload.ocean_engine
  const needsEvidence = ocean?.promotions.some(promotion => promotion.product_image_references?.some(reference => !isOceanEngineImageSourceIdentity(reference.audit_attributes?.image_src_identity)))
  if (!ocean || !needsEvidence) return configuration

  const selectedReferences = ocean.promotions.flatMap(promotion => promotion.product_image_references ?? [])
  const selectedIDs = new Set<string>()
  for (const reference of selectedReferences) {
    if (reference.id) selectedIDs.add(reference.id)
    const connectorID = reference.audit_attributes?.connector_platform_object_id
    if (connectorID) selectedIDs.add(connectorID)
  }
  const observedByID = new Map<string, ApiConnectorPlatformObject>()
  let cursor: string | undefined
  for (let pageIndex = 0; pageIndex < 100; pageIndex += 1) {
    const page = await loadProductImages('', cursor, 'created_at', 'desc')
    for (const item of page.items) {
      if (selectedIDs.has(item.id) || selectedIDs.has(item.platform_object_id)) {
        observedByID.set(item.id, item)
        observedByID.set(item.platform_object_id, item)
      }
    }
    if ([...selectedIDs].every(id => observedByID.has(id)) || !page.next_cursor) break
    cursor = page.next_cursor
  }
  const next = structuredClone(configuration)
  next.payload.ocean_engine!.promotions = next.payload.ocean_engine!.promotions.map(promotion => ({
    ...promotion,
    product_image_references: promotion.product_image_references?.map(reference => {
      const connectorID = reference.audit_attributes?.connector_platform_object_id
      const observed = observedByID.get(connectorID ?? '') ?? (reference.id ? observedByID.get(reference.id) : undefined)
      if (!observed) throw new Error('已选产品主图不在当前 Connector 图片目录中。请重新同步或重新选择产品主图。')
      const imageSourceIdentity = oceanEngineImageSourceIdentity(observed.metadata.web_uri ?? observed.preview_url)
      if (!imageSourceIdentity) throw new Error('产品主图缺少稳定图片路径。请重新同步巨量对象目录。')
      return {
        ...reference,
        audit_attributes: {
          ...reference.audit_attributes,
          image_src_identity: imageSourceIdentity,
          minimum_visible: '1',
        },
      }
    }),
  }))
  return next
}

function mergePlatformObjects(current: ApiConnectorPlatformObject[], next: ApiConnectorPlatformObject[]) {
  const values = new Map(current.map(item => [item.id, item]))
  next.forEach(item => values.set(item.id, item))
  return [...values.values()]
}

function formatConnectorSpend(value?: ApiConnectorPlatformObject['performance']) {
  if (!value?.available) return '--'
  return `¥${(value.spend_minor / 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })}`
}

function formatConnectorCTR(value?: ApiConnectorPlatformObject['performance']) {
  if (!value?.available) return '--'
  return `${(value.ctr * 100).toFixed(2)}%`
}

function platformObjectCreatedAt(item: ApiConnectorPlatformObject) {
  const value = item.metadata.create_time
  return typeof value === 'string' ? formatTime(value) : '创建时间未知'
}

function materialSelectionID(reference: StableReference) {
  return reference.audit_attributes?.connector_platform_object_id ? `connector:${reference.audit_attributes.connector_platform_object_id}` : reference.id ?? ''
}

function MaterialObjectPicker({ label, assets, platformObjects = [], value, objectKind, loadPlatformObjects, onChange }: { label: string; assets: ApiAssetVersionPointer[]; platformObjects?: ApiConnectorPlatformObject[]; value: StableReference[]; objectKind: string; loadPlatformObjects?: PlatformObjectLoader; onChange: (value: StableReference[]) => void }) {
  const productImageMode = objectKind === 'product_image'
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [preview, setPreview] = useState<{ url: string; mediaKind: 'image' | 'video'; label: string }>()
  const [remoteObjects, setRemoteObjects] = useState<ApiConnectorPlatformObject[]>(platformObjects)
  const [knownPlatformObjects, setKnownPlatformObjects] = useState<ApiConnectorPlatformObject[]>(platformObjects)
  const [nextCursor, setNextCursor] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [sortBy, setSortBy] = useState<PlatformObjectSort>('created_at')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')
  const searchGenerationRef = useRef(0)
  const [draftIds, setDraftIds] = useState<string[]>(value.map(materialSelectionID))
  const draftSelected = useMemo(() => new Set(draftIds), [draftIds])
  const normalizedQuery = query.trim().toLowerCase()
  const filteredAssets = useMemo(() => assets.filter(asset => `${asset.assetId} ${asset.oceanEngineMaterialId ?? ''}`.toLowerCase().includes(normalizedQuery)), [assets, normalizedQuery])
  const filteredPlatformObjects = useMemo(() => loadPlatformObjects ? remoteObjects : platformObjects.filter(item => `${item.display_name} ${item.platform_object_id}`.toLowerCase().includes(normalizedQuery)), [loadPlatformObjects, normalizedQuery, platformObjects, remoteObjects])
  const assetByID = useMemo(() => new Map(assets.map(asset => [asset.assetId, asset])), [assets])
  const platformObjectByID = useMemo(() => new Map(knownPlatformObjects.map(item => [item.id, item])), [knownPlatformObjects])
  const selectedPreviews = useMemo(() => value.map(reference => {
    const connectorID = reference.audit_attributes?.connector_platform_object_id
    const platformObject = connectorID ? platformObjectByID.get(connectorID) : undefined
    const asset = assetByID.get(reference.id ?? '')
    const previewURL = platformObject?.preview_url || asset?.contentUrl || reference.audit_attributes?.preview_url || ''
    const kind = platformObject?.object_kind ?? reference.object_kind
    return {
      key: `${reference.namespace}-${reference.id}`,
      label: reference.display_name_snapshot ?? reference.id,
      previewURL,
      mediaKind: kind === 'video_material' || asset?.mediaKind === 'video' ? 'video' : kind === 'aweme_photo_material' ? 'graphic' : 'image',
      useVideoElement: asset?.mediaKind === 'video',
    }
  }), [assetByID, platformObjectByID, value])

  useEffect(() => {
    setKnownPlatformObjects(current => mergePlatformObjects(current, platformObjects))
  }, [platformObjects])

  useEffect(() => {
    if (!open || !loadPlatformObjects) return
    const generation = ++searchGenerationRef.current
    const timer = window.setTimeout(() => {
      setLoading(true)
      setLoadError('')
      void loadPlatformObjects(query.trim(), undefined, sortBy, sortOrder).then(page => {
        if (generation !== searchGenerationRef.current) return
        setRemoteObjects(page.items)
        setKnownPlatformObjects(current => mergePlatformObjects(current, page.items))
        setNextCursor(page.next_cursor)
      }).catch(error => {
        if (generation !== searchGenerationRef.current) return
        setRemoteObjects([])
        setNextCursor('')
        setLoadError(errorMessage(error, '读取 Connector 素材失败。'))
      }).finally(() => {
        if (generation === searchGenerationRef.current) setLoading(false)
      })
    }, 250)
    return () => window.clearTimeout(timer)
  }, [loadPlatformObjects, open, query, sortBy, sortOrder])

  const loadMore = async () => {
    if (!loadPlatformObjects || !nextCursor || loading) return
    setLoading(true)
    setLoadError('')
    try {
      const page = await loadPlatformObjects(query.trim(), nextCursor, sortBy, sortOrder)
      setRemoteObjects(current => mergePlatformObjects(current, page.items))
      setKnownPlatformObjects(current => mergePlatformObjects(current, page.items))
      setNextCursor(page.next_cursor)
    } catch (error) {
      setLoadError(errorMessage(error, '读取更多 Connector 素材失败。'))
    } finally {
      setLoading(false)
    }
  }
  const toggle = (asset: ApiAssetVersionPointer) => setDraftIds(current => current.includes(asset.assetId) ? current.filter(id => id !== asset.assetId) : [...current, asset.assetId])
  const togglePlatformObject = (item: ApiConnectorPlatformObject) => {
    const id = `connector:${item.id}`
    setDraftIds(current => current.includes(id) ? current.filter(value => value !== id) : [...current, id])
  }
  const confirm = () => {
    onChange(draftIds.map((id): StableReference => {
      const platformObject = knownPlatformObjects.find(item => `connector:${item.id}` === id)
      if (platformObject) return { namespace: 'oceanengine', object_kind: platformObject.object_kind, scope: `account:${platformObject.account_id}`, id: platformObject.platform_object_id, version: String(platformObject.version), state: 'resolved' as const, display_name_snapshot: platformObject.display_name || platformObject.platform_object_id, audit_attributes: { connector_platform_object_id: platformObject.id, ocean_engine_material_id: platformObject.platform_object_id, preview_url: platformObject.preview_url ?? '', ...(productImageMode ? { image_src_identity: oceanEngineImageSourceIdentity(platformObject.metadata.web_uri ?? platformObject.preview_url), minimum_visible: '1' } : {}) } } satisfies StableReference
      const asset = assets.find(item => item.assetId === id)
      const existing = value.find(reference => materialSelectionID(reference) === id)
      if (!asset && existing) return existing
      return { namespace: 'cookies', object_kind: objectKind, scope: 'current_project', id, version: String(asset?.humanConfirmedVersion ?? asset?.workingVersion ?? 0), state: 'resolved' as const, display_name_snapshot: id, audit_attributes: { ocean_engine_material_id: asset?.oceanEngineMaterialId ?? '' } } satisfies StableReference
    }))
    setOpen(false)
  }
  return <fieldset className={`delivery-config-object-picker${productImageMode ? ' delivery-config-object-picker--product-image' : ''}`}>
    <legend>{label}</legend>
    <div className="delivery-config-object-summary"><div className="delivery-config-selected-previews" aria-label={`已选${label}预览`}>{selectedPreviews.map(item => <span key={item.key} className="delivery-config-selected-preview" title={item.label}>{item.previewURL ? item.useVideoElement ? <video src={item.previewURL} muted preload="metadata"/> : <img src={item.previewURL} alt="" loading="lazy"/> : <span className="delivery-config-selected-preview-fallback">{item.mediaKind === 'video' ? '视频' : item.mediaKind === 'graphic' ? '图文' : '图片'}</span>}{productImageMode ? null : <small>{item.mediaKind === 'video' ? '视频' : item.mediaKind === 'graphic' ? '图文' : '图片'}</small>}</span>)}</div><button className="secondary-button" type="button" onClick={() => { setDraftIds(value.map(materialSelectionID)); setOpen(true) }}>{productImageMode ? '选择图片' : '选择素材'}</button></div>
    {!assets.length && !platformObjects.length && !loadPlatformObjects ? <p>当前 Project 没有可选择素材。</p> : null}
    {open ? <div className="delivery-material-modal-backdrop" role="presentation" onClick={() => setOpen(false)}>
      <section className={`delivery-material-modal${productImageMode ? ' delivery-material-modal--product-image' : ''}`} role="dialog" aria-modal="true" aria-label={`${label}选择器`} onClick={event => event.stopPropagation()}>
        <header><div><span className="section-label">{productImageMode ? 'PRODUCT IMAGE' : 'MATERIAL PICKER'}</span><h3>{label}</h3><p>{productImageMode ? '选择巨量“我的图片”中的 1:1 产品主图。' : '选择 Cookies 素材，或 Connector 已导入素材。'}</p></div><button className="text-button" type="button" onClick={() => setOpen(false)}>关闭</button></header>
        <div className="delivery-material-modal-toolbar">
          {productImageMode ? null : <input autoFocus placeholder="搜索素材名称或巨量素材 ID" value={query} onChange={event => setQuery(event.target.value)}/>}
          {loadPlatformObjects && !productImageMode ? <div className="delivery-material-sort"><label><span>排序</span><select value={sortBy} onChange={event => setSortBy(event.target.value as PlatformObjectSort)}><option value="created_at">创建时间</option><option value="ctr">点击率</option><option value="conversions">转化</option></select></label><button type="button" className="text-button" onClick={() => setSortOrder(current => current === 'desc' ? 'asc' : 'desc')}>{sortOrder === 'desc' ? '降序' : '升序'}</button></div> : null}
          <span>已选 {draftIds.length} 个</span><button className="text-button" type="button" onClick={() => setDraftIds([])}>清空</button>
        </div>
        {loadError ? <div className="delivery-material-error" role="alert">{loadError}</div> : null}
        <div className={`delivery-material-modal-grid${productImageMode ? ' delivery-material-modal-grid--product-image' : ''}`}>
          {filteredPlatformObjects.map(item => {
            const selection = `connector:${item.id}`
            const displayName = item.display_name || item.platform_object_id
            const materialKind = item.object_kind === 'video_material' ? '视频' : item.object_kind === 'aweme_photo_material' ? '图文' : '图片'
            if (productImageMode) return <label key={item.id} className={`delivery-material-card delivery-material-card--product-image${draftSelected.has(selection) ? ' selected' : ''}`} aria-label={draftSelected.has(selection) ? `已选择图片 ${displayName}` : `选择图片 ${displayName}`} title={displayName}>
              <input type="checkbox" checked={draftSelected.has(selection)} onChange={() => togglePlatformObject(item)}/>
              {item.preview_url ? <span className="delivery-material-preview-button delivery-material-preview-button--square"><img src={item.preview_url} alt={displayName} loading="lazy" /></span> : <span className="delivery-material-platform-object delivery-material-platform-object--square">图片</span>}
            </label>
            return <label key={item.id} className={`delivery-material-card delivery-material-card--oceanengine${draftSelected.has(selection) ? ' selected' : ''}`}>
              <input type="checkbox" checked={draftSelected.has(selection)} onChange={() => togglePlatformObject(item)}/>
              {item.preview_url ? <button type="button" className="delivery-material-preview-button delivery-material-preview-button--portrait" onClick={event => { event.preventDefault(); event.stopPropagation(); setPreview({ url: item.preview_url!, mediaKind: 'image', label: displayName }) }}><img src={item.preview_url} alt="" loading="lazy" /><span className="delivery-material-preview-type">{materialKind}</span>{item.object_kind === 'video_material' ? <span className="delivery-material-preview-play" aria-hidden="true"/> : null}</button> : <span className="delivery-material-platform-object"><span>{materialKind}</span><small>Connector</small></span>}
              <b title={displayName}>{displayName}</b>
              <small title={`${materialKind}素材 · ${platformObjectCreatedAt(item)}`}>Connector · {materialKind}素材<br/>{platformObjectCreatedAt(item)}</small>
              <span className="delivery-material-card-metrics"><span><small>消耗</small><strong>{formatConnectorSpend(item.performance)}</strong></span><span><small>点击率</small><strong>{formatConnectorCTR(item.performance)}</strong></span><span><small>转化</small><strong>{item.performance?.available ? item.performance.conversions : '--'}</strong></span></span>
            </label>
          })}
          {filteredAssets.map(asset => <label key={asset.id} className={`delivery-material-card delivery-material-card--cookies${draftSelected.has(asset.assetId) ? ' selected' : ''}`}><input type="checkbox" checked={draftSelected.has(asset.assetId)} onChange={() => toggle(asset)}/><button type="button" className="delivery-material-preview-button delivery-material-preview-button--landscape" onClick={() => asset.contentUrl && setPreview({ url: asset.contentUrl, mediaKind: asset.mediaKind === 'video' ? 'video' : 'image', label: asset.assetId })}>{asset.contentUrl ? asset.mediaKind === 'video' ? <video src={asset.contentUrl} preload="metadata"/> : <img src={asset.contentUrl} alt=""/> : <span>无预览</span>}<span className="delivery-material-preview-type">{asset.mediaKind === 'video' ? '视频' : '图片'}</span></button><b title={asset.assetId}>{asset.assetId}</b><small>{asset.oceanEngineMaterialId ? 'Cookies · 已录入巨量' : 'Cookies · 待 RPA 录入'}</small></label>)}
          {loading && !filteredPlatformObjects.length ? <div className="delivery-material-empty">正在读取 Connector 素材…</div> : null}
          {!loading && !filteredAssets.length && !filteredPlatformObjects.length ? <div className="delivery-material-empty">{productImageMode ? '没有可选择的图片。' : '没有匹配的素材。'}</div> : null}
        </div>
        {nextCursor ? <button className="secondary-button delivery-material-load-more" type="button" disabled={loading} onClick={() => void loadMore()}>{loading ? '读取中…' : '加载更多'}</button> : null}
        <footer><button className="secondary-button" type="button" onClick={() => setOpen(false)}>取消</button><button className="primary-button" type="button" disabled={loading} onClick={confirm}>确认选择</button></footer>
        {preview ? <div className="delivery-material-preview-overlay" role="dialog" aria-label="素材预览" onClick={() => setPreview(undefined)}>{preview.mediaKind === 'video' ? <video src={preview.url} controls autoPlay onClick={event => event.stopPropagation()}/> : <img src={preview.url} alt={preview.label} onClick={event => event.stopPropagation()}/>}</div> : null}
      </section>
    </div> : null}
  </fieldset>
}

type MarketingProductOption = { id: string; name: string; oceanEngineProductId?: string }

function marketingProductSelectionID(value?: StableReference) {
  if (!value) return ''
  if (value.namespace === 'cookies') return `cookies:${value.id}`
  return `connector:${value.audit_attributes?.connector_platform_object_id ?? value.id}`
}

function connectorMarketingProductIDs(item: ApiConnectorPlatformObject) {
  const metadataUniqueID = typeof item.metadata.unique_product_id === 'string' ? item.metadata.unique_product_id.trim() : ''
  const uniqueProductID = metadataUniqueID || item.platform_object_id
  const metadataProductID = typeof item.metadata.product_id === 'string' ? item.metadata.product_id.trim() : ''
  const productID = metadataProductID || (item.platform_object_id !== uniqueProductID ? item.platform_object_id : '')
  return { uniqueProductID, productID }
}

function MarketingProductPicker({ value, cookiesProducts, loadPlatformObjects, onChange }: { value?: StableReference; cookiesProducts: MarketingProductOption[]; loadPlatformObjects: PlatformObjectLoader; onChange: (value?: StableReference) => void }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<ApiConnectorPlatformObject[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [selectedID, setSelectedID] = useState(() => marketingProductSelectionID(value))
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const generationRef = useRef(0)
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const filteredCookiesProducts = useMemo(() => cookiesProducts.filter(product => !normalizedQuery || `${product.name} ${product.id} ${product.oceanEngineProductId ?? ''}`.toLocaleLowerCase().includes(normalizedQuery)), [cookiesProducts, normalizedQuery])

  useEffect(() => {
    if (!open) return
    const generation = ++generationRef.current
    const timer = window.setTimeout(() => {
      setLoading(true)
      setLoadError('')
      void loadPlatformObjects(query.trim(), undefined, 'created_at', 'desc').then(page => {
        if (generation !== generationRef.current) return
        setItems(page.items)
        setNextCursor(page.next_cursor)
      }).catch(error => {
        if (generation !== generationRef.current) return
        setItems([])
        setNextCursor('')
        setLoadError(errorMessage(error, '读取巨量营销产品失败。'))
      }).finally(() => {
        if (generation === generationRef.current) setLoading(false)
      })
    }, 250)
    return () => window.clearTimeout(timer)
  }, [loadPlatformObjects, open, query])

  const loadMore = async () => {
    if (!nextCursor || loading) return
    setLoading(true)
    try {
      const page = await loadPlatformObjects(query.trim(), nextCursor, 'created_at', 'desc')
      setItems(current => mergePlatformObjects(current, page.items))
      setNextCursor(page.next_cursor)
    } catch (error) {
      setLoadError(errorMessage(error, '读取更多巨量营销产品失败。'))
    } finally {
      setLoading(false)
    }
  }
  const confirm = () => {
    if (!selectedID) {
      onChange(undefined)
      setOpen(false)
      return
    }
    const cookiesProduct = selectedID.startsWith('cookies:') ? cookiesProducts.find(candidate => `cookies:${candidate.id}` === selectedID) : undefined
    if (cookiesProduct) {
      onChange({
        namespace: 'cookies', object_kind: 'product', scope: 'current_project', id: cookiesProduct.id,
        state: 'resolved', display_name_snapshot: cookiesProduct.name,
        audit_attributes: { ocean_engine_product_id: cookiesProduct.oceanEngineProductId ?? '' },
      })
      setOpen(false)
      return
    }
    const item = items.find(candidate => `connector:${candidate.id}` === selectedID)
    if (!item) {
      if (selectedID === marketingProductSelectionID(value)) {
        onChange(value)
        setOpen(false)
        return
      }
      setLoadError('当前选择不在搜索结果中。请重新搜索并选择。')
      return
    }
    const { uniqueProductID, productID } = connectorMarketingProductIDs(item)
    onChange({
      namespace: 'oceanengine', object_kind: 'product', scope: `account:${item.account_id}`,
      id: uniqueProductID, version: String(item.version), state: 'resolved',
      display_name_snapshot: item.display_name || item.platform_object_id,
      audit_attributes: {
        connector_platform_object_id: item.id,
        platform_object_id: uniqueProductID,
        unique_product_id: uniqueProductID,
        product_id: productID,
        ocean_engine_product_id: uniqueProductID,
      },
    })
    setOpen(false)
  }
  return <fieldset className="delivery-config-object-picker">
    <legend>营销产品</legend>
    <div className="delivery-config-object-summary"><div className="delivery-product-summary">{value ? <><span><Package size={20} aria-hidden="true"/></span><div><b>{value.display_name_snapshot || value.id}</b><small>{value.namespace === 'cookies' ? 'Cookies' : 'Connector'}</small></div></> : <small>尚未选择营销产品</small>}</div><button className="secondary-button" type="button" onClick={() => { setSelectedID(marketingProductSelectionID(value)); setOpen(true) }}>选择产品</button></div>
    {open ? <div className="delivery-product-picker-backdrop" role="presentation" onClick={() => setOpen(false)}><section className="delivery-product-picker" role="dialog" aria-modal="true" aria-label="营销产品选择器" onClick={event => event.stopPropagation()}>
      <header><div><span className="section-label">MARKETING PRODUCT</span><h3>营销产品</h3><p>从 Cookies 产品或 Connector 已导入产品中选择一个。</p></div><button className="text-button" type="button" onClick={() => setOpen(false)}>关闭</button></header>
      <div className="delivery-product-picker-toolbar"><input autoFocus placeholder="搜索产品名称、product_id 或 unique_product_id" value={query} onChange={event => setQuery(event.target.value)}/><span>当前结果 {filteredCookiesProducts.length + items.length} 个</span></div>
      {loadError ? <div className="delivery-material-error" role="alert">{loadError}</div> : null}
      <div className="delivery-product-picker-grid">
        {filteredCookiesProducts.map(product => { const selection = `cookies:${product.id}`; return <label key={selection} className={`delivery-product-card${selectedID === selection ? ' selected' : ''}`}><input type="radio" name="marketing_product" checked={selectedID === selection} onChange={() => setSelectedID(selection)}/><span className="delivery-product-card-thumbnail"><Package size={26} aria-hidden="true"/></span><span className="delivery-product-card-body"><b title={product.name}>{product.name}</b><small>{product.oceanEngineProductId ? '已录入巨量' : '待 RPA 录入'}</small><code>{product.oceanEngineProductId || product.id}</code></span><span className="delivery-product-card-source">Cookies</span></label> })}
        {items.map(item => { const selection = `connector:${item.id}`; const name = item.display_name || item.platform_object_id; const { uniqueProductID, productID } = connectorMarketingProductIDs(item); return <label key={selection} className={`delivery-product-card${selectedID === selection ? ' selected' : ''}`}><input type="radio" name="marketing_product" checked={selectedID === selection} onChange={() => setSelectedID(selection)}/><span className="delivery-product-card-thumbnail">{item.preview_url ? <img src={item.preview_url} alt="" loading="lazy"/> : <Package size={26} aria-hidden="true"/>}</span><span className="delivery-product-card-body"><b title={name}>{name}</b><small>{typeof item.metadata.brand_name === 'string' && item.metadata.brand_name ? item.metadata.brand_name : '品牌未知'} · {typeof item.metadata.category_name === 'string' && item.metadata.category_name ? item.metadata.category_name : '类目未知'}</small><code>选择 ID：{uniqueProductID}</code>{productID ? <small>product_id：{productID}</small> : null}</span><span className="delivery-product-card-source">Connector</span></label> })}
        {!loading && !filteredCookiesProducts.length && !items.length ? <div className="delivery-product-picker-empty">没有匹配的营销产品。</div> : null}
      </div>
      {nextCursor ? <button className="secondary-button delivery-product-picker-more" type="button" disabled={loading} onClick={() => void loadMore()}>{loading ? '读取中…' : '加载更多'}</button> : null}
      <footer><button className="secondary-button" type="button" onClick={() => setOpen(false)}>取消</button><button className="text-button" type="button" onClick={() => setSelectedID('')}>清空</button><button className="primary-button" type="button" onClick={confirm}>确认选择</button></footer>
    </section></div> : null}
  </fieldset>
}

function ReferenceObjectPicker({ label, pickerTitle, value, objectKind, loadPlatformObjects, requiredContext, onChange }: { label: string; pickerTitle: string; value?: StableReference; objectKind: string; loadPlatformObjects: PlatformObjectLoader; requiredContext?: string; onChange: (value?: StableReference) => void }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<ApiConnectorPlatformObject[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [selectedID, setSelectedID] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const generationRef = useRef(0)
  const visibleItems = useMemo(() => requiredContext ? items.filter(item => Array.isArray(item.metadata.contexts) && item.metadata.contexts.includes(requiredContext)) : items, [items, requiredContext])

  useEffect(() => {
    if (!open) return
    const generation = ++generationRef.current
    const timer = window.setTimeout(() => {
      setLoading(true)
      setLoadError('')
      void loadPlatformObjects(query.trim(), undefined, 'created_at', 'desc').then(page => {
        if (generation !== generationRef.current) return
        setItems(page.items)
        setNextCursor(page.next_cursor)
      }).catch(error => {
        if (generation !== generationRef.current) return
        setItems([])
        setNextCursor('')
        setLoadError(errorMessage(error, `读取${label}失败。`))
      }).finally(() => {
        if (generation === generationRef.current) setLoading(false)
      })
    }, 250)
    return () => window.clearTimeout(timer)
  }, [label, loadPlatformObjects, open, query])

  const loadMore = async () => {
    if (!nextCursor || loading) return
    setLoading(true)
    try {
      const page = await loadPlatformObjects(query.trim(), nextCursor, 'created_at', 'desc')
      setItems(current => mergePlatformObjects(current, page.items))
      setNextCursor(page.next_cursor)
    } catch (error) {
      setLoadError(errorMessage(error, `读取更多${label}失败。`))
    } finally {
      setLoading(false)
    }
  }
  const confirm = () => {
    if (!selectedID) {
      onChange(undefined)
      setOpen(false)
      return
    }
    const item = items.find(candidate => candidate.id === selectedID)
    if (!item) {
      if (value?.audit_attributes?.connector_platform_object_id === selectedID) {
        onChange(value)
        setOpen(false)
        return
      }
      setLoadError('当前选择不在搜索结果中。请重新搜索并选择。')
      return
    }
    onChange({
      namespace: 'oceanengine', object_kind: objectKind, scope: `account:${item.account_id}`,
      id: item.platform_object_id, version: String(item.version), state: 'resolved',
      display_name_snapshot: item.display_name || item.platform_object_id,
      audit_attributes: { connector_platform_object_id: item.id, platform_object_id: item.platform_object_id },
    })
    setOpen(false)
  }
  return <fieldset className="delivery-config-object-picker">
    <legend>{label}</legend>
    <div className="delivery-config-object-summary"><div className="delivery-product-summary">{value ? <><span><Package size={20} aria-hidden="true"/></span><div><b>{value.display_name_snapshot || value.id}</b><small>Connector · {value.id}</small></div></> : <small>尚未选择{label}</small>}</div><button className="secondary-button" type="button" onClick={() => { setSelectedID(value?.audit_attributes?.connector_platform_object_id ?? ''); setOpen(true) }}>{pickerTitle}</button></div>
    {open ? <div className="delivery-product-picker-backdrop" role="presentation" onClick={() => setOpen(false)}><section className="delivery-product-picker" role="dialog" aria-modal="true" aria-label={`${label}选择器`} onClick={event => event.stopPropagation()}>
      <header><div><span className="section-label">CONNECTOR OBJECT</span><h3>{label}</h3><p>从当前巨量账户已同步的对象中选择一个。</p></div><button className="text-button" type="button" onClick={() => setOpen(false)}>关闭</button></header>
      <div className="delivery-product-picker-toolbar"><input autoFocus placeholder={`搜索${label}名称或平台 ID`} value={query} onChange={event => setQuery(event.target.value)}/><span>当前结果 {visibleItems.length} 个</span></div>
      {loadError ? <div className="delivery-material-error" role="alert">{loadError}</div> : null}
      <div className="delivery-product-picker-grid">
        {visibleItems.map(item => { const name = item.display_name || item.platform_object_id; return <label key={item.id} className={`delivery-product-card${selectedID === item.id ? ' selected' : ''}`}><input type="radio" name={`reference_${objectKind}`} checked={selectedID === item.id} onChange={() => setSelectedID(item.id)}/><span className="delivery-product-card-thumbnail">{item.preview_url ? <img src={item.preview_url} alt="" loading="lazy"/> : <Package size={26} aria-hidden="true"/>}</span><span className="delivery-product-card-body"><b title={name}>{name}</b><small>{label}</small><code>{item.platform_object_id}</code></span><span className="delivery-product-card-source">Connector</span></label> })}
        {!loading && !visibleItems.length ? <div className="delivery-product-picker-empty">没有匹配的{label}。</div> : null}
      </div>
      {nextCursor ? <button className="secondary-button delivery-product-picker-more" type="button" disabled={loading} onClick={() => void loadMore()}>{loading ? '读取中…' : '加载更多'}</button> : null}
      <footer><button className="secondary-button" type="button" onClick={() => setOpen(false)}>取消</button><button className="text-button" type="button" onClick={() => setSelectedID('')}>清空</button><button className="primary-button" type="button" onClick={confirm}>确认选择</button></footer>
    </section></div> : null}
  </fieldset>
}

function OptimizationTargetCapabilityField({ accountID, value, snapshot, loading, error, onChange }: { accountID: string; value?: StableReference; snapshot?: ApiOptimizationTargetCapabilitySnapshot; loading: boolean; error: string; onChange: (value?: StableReference) => void }) {
  const current = snapshot && optimizationCapabilitySelectionMatches({ optimization_target_reference: value }, snapshot.snapshot_id, snapshot.options.map(option => option.external_action))
  const selected = current ? snapshot.options.find(option => option.external_action === value?.id) : undefined
  return <label><span>优化目标 · {selected ? '必填' : '必填 · 待补'}</span>
    <select value={selected?.external_action ?? ''} disabled={loading || !snapshot} onChange={event => {
      const option = snapshot?.options.find(item => item.external_action === event.target.value)
      onChange(option && snapshot ? {
        namespace: 'oceanengine_capability', object_kind: 'optimization_target', scope: `account:${accountID}`,
        id: option.external_action, state: 'resolved', semantic_key: option.semantic_key,
        display_name_snapshot: option.display_name,
        audit_attributes: {
          selection_kind: 'account_capability', external_action: option.external_action,
          optimization_event_type: option.optimization_event_type ?? '', capability_snapshot_id: snapshot.snapshot_id,
          capability_context_hash: snapshot.context_hash, capability_observed_at: snapshot.observed_at,
        },
      } : undefined)
    }}>
      <option value="">{loading ? '正在读取当前分支…' : '请选择当前分支可用目标'}</option>
      {snapshot?.options.map(option => <option key={option.external_action} value={option.external_action}>{option.display_name}</option>)}
    </select>
    {snapshot ? <small>当前账户和分支返回 {snapshot.options.length} 个优化目标。</small> : null}
    {selected ? <small>已选 external_action {selected.external_action}{selected.need_assets ? ' · 需要事件资产' : ''}</small> : null}
    {error ? <small className="field-error">{error}</small> : null}
    {!loading && snapshot && !snapshot.options.length ? <small>当前账户和分支没有可用优化目标。</small> : null}
  </label>
}

type MaterialEditorTab = 'video' | 'image' | 'graphic'

function lineListValues(value: string, limit: number, unique = false) {
  const values = value.split('\n').map(item => item.trim()).filter(Boolean)
  return (unique ? values.filter((item, index) => values.indexOf(item) === index) : values).slice(0, limit)
}

function LineListTextarea({ values, limit, unique = false, rows, placeholder, required = false, invalid = false, onValuesChange }: { values: string[]; limit: number; unique?: boolean; rows: number; placeholder: string; required?: boolean; invalid?: boolean; onValuesChange: (values: string[]) => void }) {
  const serialized = values.join('\n')
  const [draft, setDraft] = useState(serialized)
  const editing = useRef(false)

  useEffect(() => {
    if (!editing.current) setDraft(serialized)
  }, [serialized])

  return <textarea
    rows={rows}
    value={draft}
    placeholder={placeholder}
    required={required}
    aria-invalid={invalid}
    aria-required={required}
    className={invalid ? 'field-missing' : undefined}
    onFocus={() => { editing.current = true }}
    onChange={event => {
      const nextDraft = event.target.value
      setDraft(nextDraft)
      onValuesChange(lineListValues(nextDraft, limit, unique))
    }}
    onBlur={() => {
      editing.current = false
      const normalized = lineListValues(draft, limit, unique)
      setDraft(normalized.join('\n'))
      onValuesChange(normalized)
    }}
  />
}

function materialReferenceKind(reference: StableReference, assets: ApiAssetVersionPointer[]): MaterialEditorTab | undefined {
  if (reference.object_kind === 'video_material') return 'video'
  if (reference.object_kind === 'image_material') return 'image'
  if (reference.object_kind === 'aweme_photo_material') return 'graphic'
  const asset = assets.find(item => item.assetId === reference.id)
  if (!asset) return undefined
  return asset.mediaKind === 'video' ? 'video' : 'image'
}

function PromotionMaterialEditor({ promotion, carrier, requiredMultiLeadExternalAction, projectProductName, assets, platformObjects, loadVideos, loadImages, loadProductImages, loadPhotos, missingRequiredFields, onChange }: { promotion: OceanPromotion; carrier: string; requiredMultiLeadExternalAction: string; projectProductName: string; assets: ApiAssetVersionPointer[]; platformObjects: ApiConnectorPlatformObject[]; loadVideos: PlatformObjectLoader; loadImages: PlatformObjectLoader; loadProductImages: PlatformObjectLoader; loadPhotos: PlatformObjectLoader; missingRequiredFields: ReadonlySet<PromotionRequiredField>; onChange: (patch: Partial<OceanPromotion>) => void }) {
  const [tab, setTab] = useState<MaterialEditorTab>('video')
  const referencesFor = (kind: MaterialEditorTab) => promotion.base_material_references.filter(reference => materialReferenceKind(reference, assets) === kind)
  const updateReferences = (kind: MaterialEditorTab, next: StableReference[]) => onChange({ base_material_references: [...promotion.base_material_references.filter(reference => materialReferenceKind(reference, assets) !== kind), ...next] })
  const tabCounts = {
    video: referencesFor('video').length,
    image: referencesFor('image').length,
    graphic: referencesFor('graphic').length,
  }
  const additionalProductName = promotion.product_name ?? ''
  const eligibleLandingPages = platformObjects.filter(item => item.object_kind === 'orange_landing_page' && (!requiredMultiLeadExternalAction || multiLeadLandingActions(item.metadata).includes(requiredMultiLeadExternalAction)))
  const selectedLandingIsEligible = !requiredMultiLeadExternalAction || referenceSupportsMultiLeadAction(promotion.landing_page_reference, requiredMultiLeadExternalAction)
  return <section className="delivery-config-material-editor" aria-label="单元素材">
    <h5>04 单元素材</h5>
    <div className="delivery-config-material-group">
      <header><RequiredFieldLabel label="基础素材" missing={missingRequiredFields.has('base_materials')}/><small>视频 {tabCounts.video}/30 · 图片 {tabCounts.image}/50 · 图文 {tabCounts.graphic}/10</small></header>
      <div className="delivery-config-material-tabs" role="tablist" aria-label="基础素材类型">
        {(['video', 'image', 'graphic'] as const).map(value => <button key={value} type="button" role="tab" aria-selected={tab === value} className={tab === value ? 'active' : ''} onClick={() => setTab(value)}>{value === 'video' ? '视频' : value === 'image' ? '图片' : '图文'} <span>{tabCounts[value]}</span></button>)}
      </div>
      <div className="delivery-config-material-tab-panel" role="tabpanel">
        {tab === 'video' ? <MaterialObjectPicker label="视频素材" assets={assets.filter(asset => asset.mediaKind === 'video')} platformObjects={platformObjects.filter(item => item.object_kind === 'video_material')} loadPlatformObjects={loadVideos} value={referencesFor('video')} objectKind="material" onChange={value => updateReferences('video', value)}/> : null}
        {tab === 'image' ? <MaterialObjectPicker label="图片素材" assets={assets.filter(asset => asset.mediaKind !== 'video')} platformObjects={platformObjects.filter(item => item.object_kind === 'image_material')} loadPlatformObjects={loadImages} value={referencesFor('image')} objectKind="material" onChange={value => updateReferences('image', value)}/> : null}
        {tab === 'graphic' ? <MaterialObjectPicker label="抖音图文素材" assets={[]} platformObjects={platformObjects.filter(item => item.object_kind === 'aweme_photo_material')} loadPlatformObjects={loadPhotos} value={referencesFor('graphic')} objectKind="material" onChange={value => updateReferences('graphic', value)}/> : null}
      </div>
    </div>
    <div className="delivery-config-material-group delivery-config-material-fields">
      <label><RequiredFieldLabel label={`文案素材（${promotion.copy_items.length}/10）`} missing={missingRequiredFields.has('copy_items')}/><LineListTextarea rows={2} values={promotion.copy_items.map(item => item.text)} limit={10} placeholder="每行一条文案" required invalid={missingRequiredFields.has('copy_items')} onValuesChange={values => onChange({ copy_items: values.map(text => ({ text })) })}/></label>
      <label><span className="delivery-config-required-label">原生锚点<em>平台条件字段</em></span><input value={promotion.native_anchor_reference?.id ?? ''} placeholder="不填写时不启用" onChange={event => onChange({ native_anchor_reference: updateReference(promotion.native_anchor_reference, event.target.value, 'native_anchor') })}/><small>当前配置只保存原生锚点引用。自动生成模式尚未接入 Runner。</small></label>
      {carrierUsesOrangeLandingPage(carrier) ? <label><RequiredFieldLabel label="橙子落地页" missing={missingRequiredFields.has('landing_page')}/><select className={missingRequiredFields.has('landing_page') ? 'field-missing' : undefined} value={selectedLandingIsEligible ? promotion.landing_page_reference?.id ?? '' : ''} onChange={event => { const item = platformObjects.find(value => value.object_kind === 'orange_landing_page' && value.platform_object_id === event.target.value); const actions = multiLeadLandingActions(item?.metadata); onChange({ landing_page_reference: item ? { namespace: 'oceanengine', object_kind: 'orange_landing_page', scope: `account:${item.account_id}`, id: item.platform_object_id, version: String(item.version), state: 'resolved', display_name_snapshot: item.display_name || item.platform_object_id, audit_attributes: { connector_platform_object_id: item.id, platform_object_id: item.platform_object_id, ...(actions.length ? { multi_lead_external_actions: actions.join(',') } : {}), ...(actions.includes('100') ? { multi_conversion_eligible: 'true' } : {}) } } : undefined }) }}><option value="">{requiredMultiLeadExternalAction ? `请选择支持当前优化目标（${requiredMultiLeadExternalAction}）和多留资组件的落地页` : '请选择已导入落地页'}</option>{eligibleLandingPages.map(item => <option key={item.id} value={item.platform_object_id}>{item.display_name || item.platform_object_id}</option>)}</select>{requiredMultiLeadExternalAction && !eligibleLandingPages.length ? <small>当前账户没有支持优化目标 {requiredMultiLeadExternalAction} 和多留资组件的橙子落地页。请同步巨量对象，或更改投放分支。</small> : null}</label> : null}
      {carrier === 'owned_landing_page' ? <label><RequiredFieldLabel label="自研落地页链接" missing={missingRequiredFields.has('landing_page')}/><input className={missingRequiredFields.has('landing_page') ? 'field-missing' : undefined} type="url" placeholder="请输入 HTTPS 落地页链接" value={promotion.landing_page_reference?.object_kind === 'owned_landing_page' ? promotion.landing_page_reference.id ?? '' : ''} onChange={event => onChange({ landing_page_reference: updateReference(promotion.landing_page_reference, event.target.value, 'owned_landing_page') })}/></label> : null}
      <label><span>直达链接方式</span><select value={promotion.settings.direct_link_mode ?? 'automatic'} onChange={event => { const directLinkMode = event.target.value as 'automatic' | 'manual'; onChange({ settings: { ...promotion.settings, direct_link_mode: directLinkMode }, ...(directLinkMode === 'automatic' ? { direct_link_reference: undefined } : {}) }) }}><option value="automatic">自动生成</option><option value="manual">手动填写</option></select></label>
      {promotion.settings.direct_link_mode === 'manual' ? <label><RequiredFieldLabel label="直达链接" missing={missingRequiredFields.has('direct_link')}/><input className={missingRequiredFields.has('direct_link') ? 'field-missing' : undefined} aria-invalid={missingRequiredFields.has('direct_link')} value={promotion.direct_link_reference?.id ?? ''} placeholder="请输入 tbopen://、https:// 或 http:// 链接" onChange={event => onChange({ direct_link_reference: updateReference(promotion.direct_link_reference, event.target.value, 'direct_link') })}/><small>可以直接填写链接。手动链接不需要绑定巨量平台 ID。</small></label> : null}
    </div>
    <div className="delivery-config-material-group">
      <header><b>产品信息</b><small>产品名称默认继承项目营销产品。</small></header>
      <div className="delivery-config-material-fields">
        <label><RequiredFieldLabel label="产品名称" missing={missingRequiredFields.has('product_name')}/><input className={missingRequiredFields.has('product_name') ? 'field-missing' : undefined} value={additionalProductName} placeholder={projectProductName || '请输入产品名称'} onChange={event => onChange({ product_name: event.target.value })}/><small>{projectProductName ? `默认：${projectProductName}。填写后使用额外产品名称。` : '当前项目没有默认产品名称。'}</small></label>
        <div>
          <MaterialObjectPicker label={`产品主图 · ${missingRequiredFields.has('product_images') ? '必填 · 待补' : '必填'}（${promotion.product_image_references?.filter(reference => reference.object_kind === 'product_image').length ?? 0}/10）`} assets={[]} platformObjects={platformObjects.filter(item => item.object_kind === 'product_image')} loadPlatformObjects={loadProductImages} value={promotion.product_image_references?.filter(reference => reference.object_kind === 'product_image') ?? []} objectKind="product_image" onChange={product_image_references => onChange({ product_image_references })}/>
        </div>
        <label><RequiredFieldLabel label={`产品卖点（${promotion.product_selling_points?.length ?? 0}/10）`} missing={missingRequiredFields.has('product_selling_points')}/><LineListTextarea rows={2} values={promotion.product_selling_points ?? []} limit={10} placeholder="每行一个卖点" invalid={missingRequiredFields.has('product_selling_points')} onValuesChange={product_selling_points => onChange({ product_selling_points })}/></label>
      </div>
    </div>
    <div className="delivery-config-material-group">
      <header><b>创意组件</b><small>行动号召最多 10 个。</small></header>
      <div className="delivery-config-material-fields">
        <label><span>附加创意组件</span><input readOnly value="当前 Runner 暂未支持"/><small>字段保留在正确层级。接入对象选择协议后开放。</small></label>
        <label><RequiredFieldLabel label={`行动号召（${promotion.settings.call_to_action?.length ?? 0}/10）`} missing={missingRequiredFields.has('call_to_action')}/><LineListTextarea rows={3} values={promotion.settings.call_to_action ?? []} limit={10} unique placeholder="每行一个行动号召" required invalid={missingRequiredFields.has('call_to_action')} onValuesChange={call_to_action => onChange({ settings: { ...promotion.settings, call_to_action } })}/></label>
        <ToggleField label="开启智能生成" checked={promotion.settings.smart_generation_enabled ?? false} onChange={smart_generation_enabled => onChange({ settings: { ...promotion.settings, smart_generation_enabled } })}/>
        <ToggleField label="允许客户端下载" checked={promotion.settings.client_download_enabled ?? false} onChange={client_download_enabled => onChange({ settings: { ...promotion.settings, client_download_enabled } })}/>
      </div>
    </div>
  </section>
}

function PromotionSettingsEditor({ promotion, index, accountID, loadCategories, loadBrands, missingRequiredFields, onChange }: { promotion: OceanPromotion; index: number; accountID: string; loadCategories: PlatformObjectLoader; loadBrands: PlatformObjectLoader; missingRequiredFields: ReadonlySet<PromotionRequiredField>; onChange: (patch: Partial<OceanPromotion>) => void }) {
  const customBrandName = promotion.settings.brand_reference?.id === '-1' ? promotion.settings.brand_reference.display_name_snapshot ?? '' : ''
  return <section className="delivery-config-settings-editor" aria-label="单元设置">
    <h5>05 单元设置</h5>
    <div className="delivery-config-unit-fields delivery-config-unit-fields--wide">
      <label><RequiredFieldLabel label="来源" missing={missingRequiredFields.has('source_label')}/><input name={`promotion_${index}_source`} aria-invalid={missingRequiredFields.has('source_label')} aria-required="true" required className={missingRequiredFields.has('source_label') ? 'field-missing' : undefined} placeholder="请输入产品或公司名称" value={promotion.settings.source_label ?? ''} onChange={event => onChange({ settings: { ...promotion.settings, source_label: event.target.value } })}/><small>来源是产品或公司名称，不是备注说明。</small></label>
      <ToggleField label="单元评论" checked={promotion.settings.comments_enabled ?? false} onChange={comments_enabled => onChange({ settings: { ...promotion.settings, comments_enabled } })}/>
      <ReferenceObjectPicker label={`所属类别 · ${missingRequiredFields.has('category') ? '必填 · 待补' : '必填'}`} pickerTitle="选择类别" value={promotion.settings.category_reference} objectKind="industry_category" loadPlatformObjects={loadCategories} onChange={category_reference => onChange({ settings: { ...promotion.settings, category_reference } })}/>
      <ReferenceObjectPicker label="品牌名称" pickerTitle="选择或填写品牌" value={promotion.settings.brand_reference} objectKind="brand" loadPlatformObjects={loadBrands} onChange={brand_reference => onChange({ settings: { ...promotion.settings, brand_reference } })}/>
      <label><span>自定义品牌名称</span><input value={customBrandName} placeholder="账户品牌列表中没有时填写" onChange={event => { const name = event.target.value.trimStart(); onChange({ settings: { ...promotion.settings, brand_reference: name ? { namespace: 'oceanengine', object_kind: 'brand', scope: `account:${accountID}`, id: '-1', state: 'resolved', display_name_snapshot: name, audit_attributes: { platform_object_id: '-1', selection_kind: 'text_option' } } : undefined } }) }}/><small>Runner 会选择“自定义品牌名称”，再填写此值。</small></label>
    </div>
  </section>
}

function ToggleField({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return <label className="delivery-config-toggle-field"><span>{label}</span><span className="delivery-config-toggle-control"><span>{checked ? '已开启' : '未开启'}</span><input type="checkbox" role="switch" checked={checked} onChange={event => onChange(event.target.checked)}/></span></label>
}

function PlatformConfigurationEditor({ value, onChange, products, assets, platformObjects, connectorAccounts, platformObjectError, loadVideos, loadImages, loadProductImages, loadPhotos, loadProducts, loadOptimizationTargets, loadOptimizationCapabilities, loadAuthorizedIdentities, loadCategories, loadBrands }: { value: PlatformConfiguration; onChange: (value: PlatformConfiguration) => void; products: Array<{ id: string; name: string; oceanEngineProductId?: string }>; assets: ApiAssetVersionPointer[]; platformObjects: ApiConnectorPlatformObject[]; connectorAccounts: ApiConnectorAccount[]; platformObjectError: string; loadVideos: PlatformObjectLoader; loadImages: PlatformObjectLoader; loadProductImages: PlatformObjectLoader; loadPhotos: PlatformObjectLoader; loadProducts: PlatformObjectLoader; loadOptimizationTargets: PlatformObjectLoader; loadOptimizationCapabilities: (accountID: string, context: ApiOptimizationTargetContext) => Promise<ApiOptimizationTargetCapabilitySnapshot>; loadAuthorizedIdentities: PlatformObjectLoader; loadCategories: PlatformObjectLoader; loadBrands: PlatformObjectLoader }) {
  const ocean = value.payload.ocean_engine
  const leadCaptureMode = ocean?.project ? oceanEngineLeadCaptureMode(ocean.project) : 'smart_lead'
  const optimizationContext = useMemo(() => ocean?.project ? oceanEngineOptimizationTargetContext(ocean.project) : undefined, [ocean?.project?.carrier, ocean?.project?.lead_capture_mode, ocean?.project?.marketing_purpose])
  const [optimizationSnapshot, setOptimizationSnapshot] = useState<ApiOptimizationTargetCapabilitySnapshot>()
  const [optimizationLoading, setOptimizationLoading] = useState(false)
  const [optimizationError, setOptimizationError] = useState('')
  useEffect(() => {
    const accountID = ocean?.project?.account_reference.id
    if (!accountID || !optimizationContext) {
      setOptimizationSnapshot(undefined)
      setOptimizationError('')
      setOptimizationLoading(false)
      return
    }
    let active = true
    setOptimizationSnapshot(undefined)
    setOptimizationError('')
    setOptimizationLoading(true)
    void loadOptimizationCapabilities(accountID, optimizationContext).then(snapshot => {
      if (active) setOptimizationSnapshot(snapshot)
    }).catch(error => {
      if (active) setOptimizationError(errorMessage(error, '读取当前分支的优化目标失败。'))
    }).finally(() => {
      if (active) setOptimizationLoading(false)
    })
    return () => { active = false }
  }, [loadOptimizationCapabilities, ocean?.project?.account_reference.id, optimizationContext])
  if (!ocean?.project) return null
  const promotionRequirements = ocean.promotions.map(promotion => missingPromotionRequiredFields(promotion, ocean.project))
  const optimizationTargetMissing = optimizationContext
    ? !optimizationSnapshot || !optimizationCapabilitySelectionMatches(ocean.project, optimizationSnapshot.snapshot_id, optimizationSnapshot.options.map(option => option.external_action))
    : !ocean.project.optimization_target_reference?.id
  const missingRequiredCount = promotionRequirements.reduce((count, fields) => count + fields.length, 0)
  const projectExecutionIssues: string[] = []
  if (ocean.project.marketing_purpose !== 'product_catalog' && !['short_video_image_text', 'manual_delivery'].includes(ocean.project.marketing_scenario)) projectExecutionIssues.push('当前 Runner 只支持“短视频与图文”营销场景。')
  if (optimizationTargetMissing) projectExecutionIssues.push('请选择当前分支允许的优化目标。')
  const projectBidMinor = ocean.project.budget_and_bidding.bid_minor
  const projectBidRequired = ['stable_cost', 'cost_cap'].includes(ocean.project.budget_and_bidding.bidding_strategy) && !['conversion_roi', 'net_roi'].includes(ocean.project.deep_optimization_mode ?? 'disabled')
  const projectChargingMode = resolveOceanEngineChargingMode(ocean.project.optimization_target_reference, ocean.project.budget_and_bidding.charging_mode)
  const projectBidConstraint = projectChargingMode
    ? resolveOceanEngineBidConstraint(projectChargingMode, ocean.project.budget_and_bidding.daily_budget_minor)
    : undefined
  if (ocean.project.marketing_purpose === 'lead_generation' && (ocean.project.budget_and_bidding.budget_mode === 'unlimited' || ocean.project.budget_and_bidding.daily_budget_minor < 30000)) projectExecutionIssues.push('销售线索项目必须设置日预算，且不能低于 300 元。')
  if (!projectChargingMode || !projectBidConstraint) projectExecutionIssues.push('当前优化目标无法解析计费方式。')
  if (projectBidRequired && projectBidMinor != null && projectBidConstraint && (projectBidMinor < projectBidConstraint.minimumMinor || projectBidMinor > projectBidConstraint.maximumMinor)) projectExecutionIssues.push(`项目出价必须在 ${formatOceanEngineMoneyRange(projectBidConstraint)}之间。`)
  const updateOcean = (next: OceanConfiguration) => onChange({ ...value, payload: { ...value.payload, ocean_engine: next } })
  const updateProject = (patch: Partial<OceanConfiguration['project']>) => updateOcean({ ...ocean, project: { ...ocean.project, ...patch } })
  const updateOptimizationTarget = (optimizationTargetReference?: StableReference) => {
    const chargingMode = resolveOceanEngineChargingMode(optimizationTargetReference, ocean.project.budget_and_bidding.charging_mode)
    updateProject({
      optimization_target_reference: optimizationTargetReference,
      ...(chargingMode ? { budget_and_bidding: { ...ocean.project.budget_and_bidding, charging_mode: chargingMode } } : {}),
    })
  }
  const updateMarketingPurpose = (marketingPurpose: string) => updateProject({
    marketing_purpose: marketingPurpose,
    ...(marketingPurpose === 'lead_generation' ? {
      delivery_mode: 'ubmax',
      budget_and_bidding: {
        ...ocean.project.budget_and_bidding,
        budget_mode: 'daily',
        daily_budget_minor: Math.max(ocean.project.budget_and_bidding.daily_budget_minor, 30000),
      },
    } : {}),
    ...(marketingPurpose === 'product_catalog' ? {} : { product_targeting: undefined }),
  })
  const updatePromotion = (index: number, patch: Partial<OceanPromotion>) => updateOcean({ ...ocean, promotions: ocean.promotions.map((promotion, itemIndex) => itemIndex === index ? { ...promotion, ...patch } : promotion) })
  const updateCarrier = (carrier: string) => {
    if (carrier === ocean.project.carrier) return
    updateOcean(changeOceanEngineCarrier(ocean, carrier))
  }
  const updateLeadCaptureMode = (leadCaptureMode: string) => {
    const carrier = leadCaptureMode === 'smart_lead' && ocean.project.carrier === 'owned_landing_page' ? 'orange_landing_page' : ocean.project.carrier
    updateOcean({
      ...ocean,
      project: { ...ocean.project, lead_capture_mode: leadCaptureMode, carrier },
      promotions: carrier === ocean.project.carrier ? ocean.promotions : ocean.promotions.map(promotion => ({ ...promotion, landing_page_reference: undefined })),
    })
  }
  const addPromotion = () => updateOcean({ ...ocean, promotions: [...ocean.promotions, {
    draft_schema_version: 'oceanengine-configuration/v1',
    promotion_draft_id: `promotion-local-${Date.now()}`,
    delivery_identity: { mode: 'account_info' }, base_material_references: [], copy_items: [], settings: { comments_enabled: false },
    promotion_name: `${ocean.project.project_name}-${ocean.promotions.length + 1}`,
  }] })
  const removePromotion = (index: number) => {
    const promotionName = ocean.promotions[index]?.promotion_name || `推广单元 ${index + 1}`
    if (!window.confirm(`删除“${promotionName}”？此操作只修改当前本地草稿。`)) return
    updateOcean({ ...ocean, promotions: ocean.promotions.filter((_, itemIndex) => itemIndex !== index) })
  }
  const accountID = ocean.project.account_reference.id
  const accountAvailable = connectorAccounts.some(account => account.id === accountID)
  const updateAccount = (nextAccountID: string) => {
    const account = connectorAccounts.find(item => item.id === nextAccountID)
    if (!account || account.id === accountID) return
    const keepCurrentAccountReference = (reference: StableReference) => reference.namespace !== 'oceanengine'
    updateOcean({
      ...ocean,
      project: {
        ...ocean.project,
        account_reference: {
          ...ocean.project.account_reference,
          namespace: 'oceanengine', object_kind: 'advertiser_account', id: account.id,
          state: 'resolved', display_name_snapshot: account.display_label || account.id,
        },
      },
      promotions: ocean.promotions.map(promotion => ({
        ...promotion,
        landing_page_reference: promotion.landing_page_reference?.namespace === 'oceanengine' ? undefined : promotion.landing_page_reference,
        base_material_references: promotion.base_material_references.filter(keepCurrentAccountReference),
        product_image_references: promotion.product_image_references?.filter(keepCurrentAccountReference),
      })),
    })
  }
  return <section className="delivery-config-editor" aria-labelledby="platform-config-editor-title">
    <header className="delivery-config-editor-intro"><div><span className="section-label">本地配置</span><h3 id="platform-config-editor-title">编辑投放项目和推广单元</h3><p>保存后生成 cookies 计划版本。Playwright RPA 在执行阶段读取该版本。</p></div><span className="delivery-config-local-badge">不会写入巨量</span></header>
    {missingRequiredCount ? <div className="delivery-config-required-summary" role="alert">
      <CircleAlert size={18} aria-hidden="true"/>
      <div><b>执行前还需填写 {missingRequiredCount} 个必填项</b><ul>{promotionRequirements.flatMap((fields, index) => fields.length ? <li key={ocean.promotions[index].promotion_draft_id}>推广单元 {index + 1}：{fields.map(field => promotionRequiredFieldLabels[field]).join('、')}</li> : [])}</ul><small>可以保存未完成草稿。生成 Runner 计划前必须补全这些字段。</small></div>
    </div> : null}
    {projectExecutionIssues.length ? <div className="delivery-config-required-summary" role="alert">
      <CircleAlert size={18} aria-hidden="true"/>
      <div><b>当前项目路径不能生成 Runner 计划</b><ul>{projectExecutionIssues.map(issue => <li key={issue}>{issue}</li>)}</ul><small>可以保存草稿，但执行会保持阻塞。</small></div>
    </div> : null}
    <div className="delivery-config-project-editor">
      <div className="delivery-config-subheading"><div><span>01</span><div><h4>投放项目</h4><p>设置营销路径、预算、竞价、排期和定向。</p></div></div></div>
      <div className="delivery-config-editor-fields delivery-config-editor-fields--wide">
        <label><span>巨量账户</span><select value={accountAvailable ? accountID : ''} onChange={event => updateAccount(event.target.value)}><option value="">请选择当前 Project 已验证账户</option>{connectorAccounts.map(account => <option key={account.id} value={account.id}>{account.display_label || account.id}</option>)}</select><small>切换账户会清除旧账户的巨量对象引用。Cookies 素材引用保持不变。</small></label>
        {!accountAvailable ? <div className="delivery-config-account-error" role="alert"><CircleAlert size={16}/><span>计划账户 <code>{accountID || '未设置'}</code> 未绑定当前 Project。请选择已验证账户。</span></div> : null}
        {platformObjectError && !platformObjectError.startsWith('计划账户 ') ? <div className="delivery-config-account-error" role="alert"><CircleAlert size={16}/><span>{platformObjectError}</span></div> : null}
        <label><span>项目名称</span><input name="oceanengine_project_name" autoComplete="off" value={ocean.project.project_name} onChange={event => updateProject({ project_name: event.target.value })}/></label>
        <label><span>营销目的</span><select value={ocean.project.marketing_purpose} onChange={event => updateMarketingPurpose(event.target.value)}><option value="ecommerce">电商</option><option value="lead_generation">销售线索</option><option value="application">应用</option><option value="product_catalog">商品</option><option value="content_marketing">内容营销</option></select></label>
        {ocean.project.marketing_purpose !== 'product_catalog' ? <label><span>营销场景</span><select value={ocean.project.marketing_scenario} onChange={event => updateProject({ marketing_scenario: event.target.value })}><option value="short_video_image_text">短视频与图文</option><option value="live_stream" disabled>直播（Runner 暂不支持）</option></select></label> : null}
        <MarketingProductPicker value={ocean.project.marketing_product_reference} cookiesProducts={products} loadPlatformObjects={loadProducts} onChange={marketing_product_reference => updateProject({ marketing_product_reference })}/>
        {ocean.project.marketing_purpose === 'application' ? <>
          <label><span>应用引用</span><input value={ocean.project.application_reference?.id ?? ''} placeholder="应用链接或应用对象 ID" onChange={event => updateProject({ application_reference: updateReference(ocean.project.application_reference, event.target.value, 'application') })}/></label>
          <label><span>应用场景</span><input value={ocean.project.application_scenario ?? ''} onChange={event => updateProject({ application_scenario: event.target.value })}/></label>
          <label><span>操作系统</span><select value={ocean.project.operating_system ?? ''} onChange={event => updateProject({ operating_system: event.target.value })}><option value="">请选择</option><option value="android">安卓</option><option value="ios">iOS</option><option value="harmonyos">鸿蒙</option></select></label>
          <label><span>下载方式</span><select value={ocean.project.application_download_mode ?? ''} onChange={event => updateProject({ application_download_mode: event.target.value })}><option value="">请选择</option><option value="direct_download">直接下载</option><option value="reservation_download">预约下载</option></select></label>
          <label><span>调起方式</span><input value={ocean.project.application_launch_mode ?? ''} onChange={event => updateProject({ application_launch_mode: event.target.value })}/></label>
        </> : null}
        {ocean.project.marketing_purpose === 'lead_generation' ? <label><span>获取线索方式</span><select value={leadCaptureMode} onChange={event => updateLeadCaptureMode(event.target.value)}><option value="smart_lead">智能优选</option><option value="custom_lead">自定义</option></select></label> : null}
        {ocean.project.marketing_purpose === 'lead_generation'
          ? <label><span>投放模式</span><input value="UBMax（平台固定）" readOnly/><small>销售线索页面固定使用 delivery_mode=3。Runner 不操作此字段。</small></label>
          : <label><span>投放模式</span><select value={ocean.project.delivery_mode} onChange={event => updateProject({ delivery_mode: event.target.value })}><option value="manual">手动投放</option><option value="ubmax">UBMax</option></select></label>}
        <label><span>深度优化方式</span><select value={ocean.project.deep_optimization_mode ?? 'disabled'} onChange={event => updateProject({ deep_optimization_mode: event.target.value })}><option value="disabled">不启用</option><option value="conversion_roi">成交 ROI</option><option value="net_order">净成交下单</option><option value="net_roi">净成交 ROI</option></select><small>平台会按当前场景限制可用选项。</small></label>
        {['lead_generation', 'ecommerce'].includes(ocean.project.marketing_purpose) ? <ToggleField label="AIGC 动态创意" checked={ocean.project.aigc_dynamic_creative ?? false} onChange={aigc_dynamic_creative => updateProject({ aigc_dynamic_creative })}/> : null}
        <label><span>竞价策略</span><select value={ocean.project.budget_and_bidding.bidding_strategy} onChange={event => updateProject({ budget_and_bidding: { ...ocean.project.budget_and_bidding, bidding_strategy: event.target.value } })}><option value="stable_cost">稳定成本 · 成本稳定在出价附近</option><option value="cost_cap">最优成本 · 均匀消耗预算，成本不超过出价</option><option value="maximum_conversion">最大转化 · 花完预算，拿到最大转化（价值）</option></select></label>
        <label><span>付费方式</span><input value={projectChargingMode ? ({ CPC: '按点击付费（CPC）', CPM: '按展示付费（CPM）', OCPC: '按目标转化出价（oCPC）', OCPM: '按目标转化出价（oCPM）' }[projectChargingMode]) : '等待优化目标'} readOnly/><small>由当前优化目标决定，不能单独修改。</small></label>
        <label><span>项目日预算</span>{ocean.project.marketing_purpose !== 'lead_generation' ? <select value={ocean.project.budget_and_bidding.budget_mode ?? (ocean.project.budget_and_bidding.daily_budget_minor === 0 ? 'unlimited' : 'daily')} onChange={event => updateProject({ budget_and_bidding: { ...ocean.project.budget_and_bidding, budget_mode: event.target.value as 'daily' | 'unlimited', daily_budget_minor: event.target.value === 'unlimited' ? 0 : Math.max(ocean.project.budget_and_bidding.daily_budget_minor, 30000) } })}><option value="daily">设置日预算</option><option value="unlimited">不限</option></select> : <small>销售线索页面要求设置日预算。</small>}{ocean.project.marketing_purpose === 'lead_generation' || (ocean.project.budget_and_bidding.budget_mode ?? 'daily') !== 'unlimited' ? <div className="delivery-config-money-input"><input type="number" inputMode="decimal" min="300" value={ocean.project.budget_and_bidding.daily_budget_minor / 100} onChange={event => updateProject({ budget_and_bidding: { ...ocean.project.budget_and_bidding, budget_mode: 'daily', daily_budget_minor: Math.round(Number(event.target.value) * 100) } })}/><small>元 / 天 · 最低 300 元</small></div> : <small>预算不设上限</small>}</label>
        {projectBidRequired ? <label><span>项目出价</span><div className="delivery-config-money-input"><input type="number" inputMode="decimal" min={projectBidConstraint ? projectBidConstraint.minimumMinor / 100 : undefined} max={projectBidConstraint ? projectBidConstraint.maximumMinor / 100 : undefined} step="0.01" value={(ocean.project.budget_and_bidding.bid_minor ?? 0) / 100} onChange={event => updateProject({ budget_and_bidding: { ...ocean.project.budget_and_bidding, bid_minor: Math.round(Number(event.target.value) * 100) } })}/><small>元{projectBidConstraint ? ` · 当前范围 ${formatOceanEngineMoneyRange(projectBidConstraint)}` : ' · 等待计费方式'}</small></div></label> : null}
        <label><span>投放周期</span><select value={ocean.project.schedule.mode ?? 'fixed_range'} onChange={event => updateProject({ schedule: { ...ocean.project.schedule, mode: event.target.value as 'long_term' | 'fixed_range' } })}><option value="long_term">从今天起长期投放</option><option value="fixed_range">设置开始和结束日期</option></select></label>
        <label><span>开始日期</span><input type="date" value={toShanghaiDateInput(ocean.project.schedule.start_at)} onChange={event => updateProject({ schedule: { ...ocean.project.schedule, start_at: fromShanghaiStartDate(event.target.value) } })}/></label>
        {ocean.project.schedule.mode !== 'long_term' ? <label><span>结束日期</span><input type="date" value={toShanghaiDateInput(ocean.project.schedule.end_at)} onChange={event => updateProject({ schedule: { ...ocean.project.schedule, end_at: fromShanghaiEndDate(event.target.value) } })}/></label> : null}
        {ocean.project.delivery_mode === 'manual' ? <fieldset className="delivery-config-inline-fieldset"><legend>投放版位</legend><label><span>投放位置</span><select value={ocean.project.placement_strategy ?? 'automatic'} onChange={event => updateProject({ placement_strategy: event.target.value, placement_media: event.target.value === 'automatic' ? undefined : ocean.project.placement_media })}><option value="automatic">通投智选</option><option value="preferred_media">首选媒体</option></select></label>{ocean.project.placement_strategy === 'preferred_media' ? <div className="delivery-config-check-grid">{[{ key: 'all', label: '全选' }, { key: 'toutiao', label: '今日头条' }, { key: 'xigua', label: '西瓜视频' }, { key: 'douyin', label: '抖音' }, { key: 'fanqie', label: '番茄系媒体' }, { key: 'pangolin', label: '穿山甲' }].map(option => { const media = ['toutiao', 'xigua', 'douyin', 'fanqie', 'pangolin']; const checked = option.key === 'all' ? media.every(item => ocean.project.placement_media?.includes(item)) : ocean.project.placement_media?.includes(option.key) ?? false; return <label key={option.key}><input type="checkbox" checked={checked} onChange={event => updateProject({ placement_media: option.key === 'all' ? event.target.checked ? media : [] : event.target.checked ? [...new Set([...(ocean.project.placement_media ?? []), option.key])] : (ocean.project.placement_media ?? []).filter(item => item !== option.key) })}/><span>{option.label}</span></label> })}</div> : null}</fieldset> : null}
        <label><span>地域</span><select multiple value={ocean.project.targeting.regions ?? []} onChange={event => updateProject({ targeting: { ...ocean.project.targeting, regions: Array.from(event.currentTarget.selectedOptions, option => option.value) } })}>{['不限', '北京市', '上海市', '广东省', '浙江省', '江苏省', '四川省'].map(region => <option key={region} value={region === '不限' ? 'all' : region}>{region}</option>)}</select><small>按住 Ctrl 可多选。</small></label>
        <label><span>年龄</span><select multiple value={ocean.project.targeting.age_ranges ?? []} onChange={event => { const values = Array.from(event.currentTarget.selectedOptions, option => option.value); updateProject({ targeting: { ...ocean.project.targeting, age_ranges: values.includes('all') ? ['all'] : values } }) }}>{[['all', '不限'], ['18-23', '18-23'], ['24-30', '24-30'], ['31-40', '31-40'], ['41-49', '41-49'], ['50+', '50+']].map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select><small>按住 Ctrl 可多选；选择“不限”会清除其他年龄。</small></label>
        <label><span>性别</span><select value={ocean.project.targeting.gender ?? ''} onChange={event => updateProject({ targeting: { ...ocean.project.targeting, gender: event.target.value } })}><option value="">不限</option><option value="male">男</option><option value="female">女</option></select></label>
        <ToggleField label="智能定向扩展" checked={ocean.project.targeting.smart_expansion} onChange={smart_expansion => updateProject({ targeting: { ...ocean.project.targeting, smart_expansion } })}/>
      </div>
    </div>
    <div className="delivery-config-project-editor">
      <div className="delivery-config-subheading"><div><span>02</span><div><h4>投放载体和监测</h4><p>设置落地页、优化目标、搜索快投和第三方监测。</p></div></div></div>
      <div className="delivery-config-editor-fields delivery-config-editor-fields--wide">
        <label><span>投放载体</span><select value={ocean.project.carrier} onChange={event => updateCarrier(event.target.value)}><option value="orange_landing_page">橙子落地页</option>{ocean.project.marketing_purpose === 'lead_generation' && leadCaptureMode === 'smart_lead' ? <option value="orange_landing_page_and_im">橙子落地页 + 抖音私信页</option> : null}{ocean.project.marketing_purpose !== 'lead_generation' || leadCaptureMode === 'custom_lead' ? <><option value="owned_landing_page">自研落地页</option><option value="im">抖音私信页（原抖音主页）</option></> : null}<option value="byte_miniapp" disabled>字节小程序（暂不支持）</option><option value="wechat_miniapp" disabled>微信小程序（暂不支持）</option></select></label>
        {optimizationContext
          ? <OptimizationTargetCapabilityField accountID={accountID ?? ''} value={ocean.project.optimization_target_reference} snapshot={optimizationSnapshot} loading={optimizationLoading} error={optimizationError} onChange={updateOptimizationTarget}/>
          : <ReferenceObjectPicker label={`优化目标 · ${optimizationTargetMissing ? '必填 · 待补' : '必填'}`} pickerTitle="选择优化目标" value={ocean.project.optimization_target_reference} objectKind="optimization_target" loadPlatformObjects={loadOptimizationTargets} requiredContext={ocean.project.carrier === 'owned_landing_page' ? 'owned_landing_page' : 'orange_landing_page'} onChange={updateOptimizationTarget}/>}

        {ocean.project.marketing_purpose === 'product_catalog' ? <fieldset className="delivery-config-inline-fieldset"><legend>商品定向</legend>
          <ToggleField label="RTA 重定向" checked={ocean.project.product_targeting?.rta_redirect ?? false} onChange={rta_redirect => updateProject({ product_targeting: { ...ocean.project.product_targeting, rta_redirect } })}/>
          <label><span>商品定向方式</span><select value={ocean.project.product_targeting?.region_match == null ? '' : ocean.project.product_targeting.region_match ? 'region_match' : 'delivery_conditions'} onChange={event => updateProject({ product_targeting: { ...ocean.project.product_targeting, region_match: event.target.value ? event.target.value === 'region_match' : undefined } })}><option value="">请选择</option><option value="region_match">地域匹配</option><option value="delivery_conditions">商品投放条件</option></select><small>该选项仅用于商品目录项目。</small></label>
        </fieldset> : null}
        <label><span>搜索关键词</span><textarea rows={2} value={ocean.project.search_boost?.keywords?.join(', ') ?? ''} placeholder="多个关键词用逗号分隔" onChange={event => updateProject({ search_boost: { ...ocean.project.search_boost, keywords: event.target.value.split(/[,，\n]/).map(item => item.trim()).filter(Boolean) } })}/></label>
        <label><span>搜索出价系数</span><input type="number" min="1" step="0.1" value={ocean.project.search_boost?.bid_coefficient ?? 1.1} onChange={event => updateProject({ search_boost: { ...ocean.project.search_boost, bid_coefficient: Number(event.target.value) } })}/></label>
        <ToggleField label="搜索定向扩展" checked={ocean.project.search_boost?.targeting_expansion ?? false} onChange={targeting_expansion => updateProject({ search_boost: { ...ocean.project.search_boost, targeting_expansion } })}/>
        {['impression', 'valid_touch', 'video_play', 'video_complete', 'valid_video_play'].map((kind, index) => { const labels = ['展示监测链接', '有效触点监测链接', '视频播放监测链接', '视频播完监测链接', '视频有效播放监测链接']; const reference = ocean.project.monitoring_references?.find(item => item.object_kind === `monitoring_link_${kind}`); return <label key={kind}><span>{labels[index]}</span><input type="url" value={reference?.id ?? ''} onChange={event => { const remaining = ocean.project.monitoring_references?.filter(item => item.object_kind !== `monitoring_link_${kind}`) ?? []; const next = updateReference(reference, event.target.value, `monitoring_link_${kind}`); updateProject({ monitoring_references: next ? [...remaining, next] : remaining }) }}/></label> })}
      </div>
    </div>
    <div className="delivery-config-unit-editor">
      <div className="delivery-config-subheading"><div><span>03</span><div><h4>推广单元</h4><p>每个单元使用独立身份、预算、出价、落地页和设置。</p></div></div><button className="secondary-button" type="button" onClick={addPromotion}><Plus size={15} aria-hidden="true"/>增加推广单元</button></div>
      <div className="delivery-config-unit-list">{ocean.promotions.map((promotion, index) => { const missingRequiredFields = new Set(promotionRequirements[index]); return <article key={promotion.promotion_draft_id} className={`delivery-config-unit-card${missingRequiredFields.size ? ' has-missing-fields' : ''}`}>
        <header><div><span>推广单元 {String(index + 1).padStart(2, '0')}</span><strong>{promotion.promotion_name || '未命名单元'}</strong>{missingRequiredFields.size ? <em>{missingRequiredFields.size} 项待补</em> : null}</div><button className="delivery-config-delete-unit" type="button" aria-label={`删除推广单元 ${index + 1}`} onClick={() => removePromotion(index)}><Trash2 size={15} aria-hidden="true"/></button></header>
        <div className="delivery-config-unit-fields delivery-config-unit-fields--wide">
          <label><RequiredFieldLabel label="单元名称" missing={missingRequiredFields.has('promotion_name')}/><input name={`promotion_${index}_name`} autoComplete="off" aria-invalid={missingRequiredFields.has('promotion_name')} aria-required="true" required className={missingRequiredFields.has('promotion_name') ? 'field-missing' : undefined} value={promotion.promotion_name} onChange={event => updatePromotion(index, { promotion_name: event.target.value })}/></label>
          <label><span>投放身份</span><select value={promotion.delivery_identity.mode} onChange={event => updatePromotion(index, { delivery_identity: { ...promotion.delivery_identity, mode: event.target.value } })}><option value="account_info">账户信息</option><option value="authorized_identity">授权身份</option></select></label>
          {promotion.delivery_identity.mode === 'authorized_identity' ? <ReferenceObjectPicker label="授权身份" pickerTitle="选择授权身份" value={promotion.delivery_identity.authorized_identity} objectKind="authorized_identity" loadPlatformObjects={loadAuthorizedIdentities} onChange={authorized_identity => updatePromotion(index, { delivery_identity: { ...promotion.delivery_identity, authorized_identity } })}/> : null}
          <label><span>单元日预算</span><div className="delivery-config-money-input"><input name={`promotion_${index}_daily_budget`} autoComplete="off" type="number" inputMode="decimal" min="0" value={(promotion.budget_and_bidding?.daily_budget_minor ?? 0) / 100} onChange={event => updatePromotion(index, { budget_and_bidding: { currency: 'CNY', bidding_strategy: promotion.budget_and_bidding?.bidding_strategy ?? 'stable_cost', charging_mode: promotion.budget_and_bidding?.charging_mode ?? 'CPC', ...promotion.budget_and_bidding, daily_budget_minor: Math.round(Number(event.target.value) * 100) } })}/><small>元 / 天</small></div></label>
          <label><span>单元出价</span><div className="delivery-config-money-input"><input name={`promotion_${index}_bid`} autoComplete="off" type="number" inputMode="decimal" min="0" step="0.01" value={(promotion.budget_and_bidding?.bid_minor ?? 0) / 100} onChange={event => updatePromotion(index, { budget_and_bidding: { currency: 'CNY', daily_budget_minor: promotion.budget_and_bidding?.daily_budget_minor ?? 0, bidding_strategy: promotion.budget_and_bidding?.bidding_strategy ?? 'stable_cost', charging_mode: promotion.budget_and_bidding?.charging_mode ?? 'CPC', ...promotion.budget_and_bidding, bid_minor: Math.round(Number(event.target.value) * 100) } })}/><small>元</small></div></label>
        </div>
        <PromotionMaterialEditor promotion={promotion} carrier={ocean.project.carrier} requiredMultiLeadExternalAction={requiredMultiLeadLandingAction(ocean.project)} projectProductName={ocean.project.marketing_product_reference?.display_name_snapshot ?? ''} assets={assets} platformObjects={platformObjects} loadVideos={loadVideos} loadImages={loadImages} loadProductImages={loadProductImages} loadPhotos={loadPhotos} missingRequiredFields={missingRequiredFields} onChange={patch => updatePromotion(index, patch)}/>
        <PromotionSettingsEditor promotion={promotion} index={index} accountID={ocean.project.account_reference.id ?? ''} loadCategories={loadCategories} loadBrands={loadBrands} missingRequiredFields={missingRequiredFields} onChange={patch => updatePromotion(index, patch)}/>
        <footer><span>{promotion.base_material_references.length} 个素材</span><small>{promotion.base_material_references.length ? '素材已关联' : '尚未关联素材'}</small></footer>
      </article>})}</div>
      {!ocean.promotions.length ? <div className="delivery-config-empty-units"><b>还没有推广单元</b><p>增加一个推广单元，然后设置预算、出价和素材。</p><button className="secondary-button" type="button" onClick={addPromotion}><Plus size={15} aria-hidden="true"/>增加推广单元</button></div> : null}
    </div>
  </section>
}

export function DeliveryConfigurationPage({ state, activeView, tourRunId, tourCase }: { state: DataState; activeView: string; tourRunId?: string; tourCase?: string }) {
  const { currentProject, agencyWorkbench } = useProject()
  const projectId = currentProject.id
  const confirmedAssets = useMemo(() => (agencyWorkbench?.assetVersionPointers ?? []).filter(asset => asset.projectId === projectId && asset.humanConfirmedVersion), [agencyWorkbench, projectId])
  const [plans, setPlans] = useState<DeliveryPlan[]>([])
  const [selectedId, setSelectedId] = useState(() => new URLSearchParams(window.location.search).get('plan_id') ?? '')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [executionDriver, setExecutionDriver] = useState<DeliveryExecutionDriver>('oceanengine-web-api/session/v1')
  const [editableConfiguration, setEditableConfiguration] = useState<PlatformConfiguration>()
  const [platformObjects, setPlatformObjects] = useState<ApiConnectorPlatformObject[]>([])
  const [connectorAccounts, setConnectorAccounts] = useState<ApiConnectorAccount[]>([])
  const [accountsLoaded, setAccountsLoaded] = useState(false)
  const [platformObjectError, setPlatformObjectError] = useState('')
  const refreshGenerationRef = useRef(0)
  const executionStartKeyRef = useRef('')

  const selectedPlan = useMemo(() => plans.find(plan => plan.id === selectedId), [plans, selectedId])
  const platformConfiguration = selectedPlan?.currentVersion.platformConfiguration
  const legacyReadOnly = Boolean(selectedPlan?.currentVersion.readOnly || (selectedPlan && !platformConfiguration))
  const showConfiguration = activeView === '配置映射'
  const showCalibration = activeView === '字段校准与处置'
  const showPreflight = activeView === '检查与提交'
  const planEditorURL = projectPath(projectId, 'delivery', 'plans', undefined, '计划列表', undefined, tourRunId, tourCase)
  const referenceIntentIssues = useMemo(() => configuredReferenceIntentIssues(editableConfiguration, selectedPlan), [editableConfiguration, selectedPlan])

  const refresh = async () => {
    const generation = ++refreshGenerationRef.current
    setBusy(true)
    try {
      const nextPlans = await deliveryPlanApi.list(projectId)
      if (generation !== refreshGenerationRef.current) return
      setPlans(nextPlans)
      setSelectedId(current => nextPlans.some(plan => plan.id === current) ? current : nextPlans[0]?.id ?? '')
      setNotice(nextPlans.length ? '已刷新当前 Project 的平台配置。' : '当前 Project 暂无投放计划。')
    } catch (error) {
      if (generation === refreshGenerationRef.current) setNotice(errorMessage(error, '读取平台配置失败。'))
    } finally {
      if (generation === refreshGenerationRef.current) setBusy(false)
    }
  }

  useEffect(() => { void refresh() }, [projectId])
  useEffect(() => {
    if (!platformConfiguration) {
      setEditableConfiguration(undefined)
      return
    }
    const next = normalizeProjectExecutionDefaults(platformConfiguration)
    if (next.payload.ocean_engine) {
      next.payload.ocean_engine = normalizeOceanEngineLandingPages(next.payload.ocean_engine)
      next.payload.ocean_engine.promotions = next.payload.ocean_engine.promotions.map(promotion => ({ ...promotion, settings: { ...promotion.settings, comments_enabled: promotion.settings.comments_enabled ?? false } }))
    }
    setEditableConfiguration(next)
  }, [selectedId, selectedPlan?.currentVersionNumber])
  useEffect(() => { executionStartKeyRef.current = '' }, [executionDriver, selectedId, selectedPlan?.currentVersionNumber])
  useEffect(() => {
    let active = true
    setAccountsLoaded(false)
    void api.listProjectConnectorAccounts(projectId).then(response => {
      if (!active) return
      setConnectorAccounts(response.items.filter(account => account.status === 'verified'))
      setPlatformObjectError('')
    }).catch(error => {
      if (!active) return
      setConnectorAccounts([])
      setPlatformObjectError(errorMessage(error, '读取当前 Project 的巨量账户失败。'))
    }).finally(() => {
      if (active) setAccountsLoaded(true)
    })
    return () => { active = false }
  }, [projectId])
  useEffect(() => {
    const accountID = editableConfiguration?.payload.ocean_engine?.project?.account_reference?.id
    if (!accountID) {
      setPlatformObjects([])
      return
    }
    if (!accountsLoaded) return
    if (!connectorAccounts.some(account => account.id === accountID)) {
      setPlatformObjects([])
      setPlatformObjectError(`计划账户 ${accountID} 未绑定当前 Project。请选择已验证账户。`)
      return
    }
    let active = true
    setPlatformObjectError('')
    void api.listProjectConnectorPlatformObjects(projectId, accountID, { objectKind: 'orange_landing_page', status: 'active', limit: 100 })
      .then(value => { if (active) setPlatformObjects(value.items) })
      .catch(error => {
        if (!active) return
        setPlatformObjects([])
        setPlatformObjectError(errorMessage(error, '读取 Connector 对象失败。'))
      })
    return () => { active = false }
  }, [accountsLoaded, connectorAccounts, editableConfiguration?.payload.ocean_engine?.project?.account_reference?.id, projectId])

  const loadPlatformObjectPage = useCallback(async (objectKind: ApiConnectorPlatformObjectKind, query: string, cursor: string | undefined, sortBy: PlatformObjectSort, sortOrder: 'asc' | 'desc') => {
    const accountID = editableConfiguration?.payload.ocean_engine?.project?.account_reference?.id
    if (!accountID || !connectorAccounts.some(account => account.id === accountID)) throw new Error('计划没有绑定当前 Project 的已验证巨量账户。')
    return api.listProjectConnectorPlatformObjects(projectId, accountID, { objectKind, status: 'active', q: query || undefined, cursor, limit: 60, sortBy, sortOrder })
  }, [connectorAccounts, editableConfiguration?.payload.ocean_engine?.project?.account_reference?.id, projectId])

  const loadVideos = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('video_material', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadImages = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('image_material', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadProductImages = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('product_image', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadPhotos = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('aweme_photo_material', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadProducts = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('marketing_product', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadOptimizationTargets = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('optimization_target', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadOptimizationCapabilities = useCallback((accountID: string, context: ApiOptimizationTargetContext) => api.readProjectOptimizationTargetCapabilities(projectId, accountID, context), [projectId])
  const loadAuthorizedIdentities = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('authorized_identity', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadCategories = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('industry_category', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadBrands = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('brand', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  useEffect(() => {
    if (!selectedId) return
    const url = new URL(window.location.href)
    url.searchParams.set('plan_id', selectedId)
    window.history.replaceState(window.history.state, '', url)
  }, [selectedId])

  const saveConfiguration = async () => {
    if (!selectedPlan || !editableConfiguration) return
    setBusy(true)
    try {
      let normalized = normalizeProjectExecutionDefaults(editableConfiguration)
      normalized = await addProductImagePickerEvidence(normalized, loadProductImages)
      if (normalized.payload.ocean_engine) normalized.payload.ocean_engine = normalizeOceanEngineLandingPages(normalized.payload.ocean_engine)
      setEditableConfiguration(normalized)
      const updated = await deliveryPlanApi.updatePlatformConfiguration(projectId, selectedPlan, normalized)
      setPlans(current => current.map(plan => plan.id === updated.id ? updated : plan))
      setNotice(`平台配置已保存为 V${updated.currentVersionNumber}。未执行巨量远端操作。`)
    } catch (error) { setNotice(errorMessage(error, '保存平台配置失败。')) } finally { setBusy(false) }
  }

  const startRealExecution = async () => {
    if (!selectedPlan) return
    setBusy(true)
    try {
      const project = selectedPlan.currentVersion.platformConfiguration?.payload.ocean_engine?.project
      const capabilityContext = project ? oceanEngineOptimizationTargetContext(project) : undefined
      if (project && capabilityContext) {
        const accountID = project.account_reference.id ?? ''
        const snapshot = await loadOptimizationCapabilities(accountID, capabilityContext)
        if (!optimizationCapabilitySelectionMatches(project, snapshot.snapshot_id, snapshot.options.map(option => option.external_action))) {
          setNotice('优化目标能力已变化。请返回“配置映射”，重新选择优化目标并保存。系统未创建执行。')
          return
        }
      }
      if (!executionStartKeyRef.current) executionStartKeyRef.current = `browser-rpa-${executionDriver === 'playwright-rpa/edge/v3' ? 'playwright-v3' : 'web-api-v1'}-${selectedPlan.id}-v${selectedPlan.currentVersionNumber}`
      const idempotencyKey = executionStartKeyRef.current
      const result = await deliveryExecutionApi.startBrowserRpaExecution(projectId, selectedPlan.id, selectedPlan.currentVersionNumber, executionDriver, idempotencyKey)
      window.location.assign(projectPath(projectId, 'delivery', 'execution', result.browser_rpa_run.run_id))
    } catch (error) {
      if (error instanceof DeliveryApiError && error.code === 'VALIDATION_FAILED' && error.violations.length) {
        setNotice(`当前配置无法投放：${error.violations.map(item => item.reason).join('；')}`)
      } else {
        setNotice(errorMessage(error, '创建真实受控执行失败。'))
      }
    } finally { setBusy(false) }
  }

  return <StateBoundary state={state} contextLabel="智能投放 / 平台配置" errorDetail="当前 Project 的平台配置无法读取。">
    <div className="delivery-config-workspace">
      <section className="delivery-config-toolbar"><label><span>投放计划</span><select name="delivery_plan" autoComplete="off" value={selectedId} onChange={event => setSelectedId(event.target.value)}>{plans.map(plan => <option value={plan.id} key={plan.id}>{plan.currentVersion.name} · V{plan.currentVersionNumber}</option>)}</select></label><a className="secondary-button" href={planEditorURL}>查看投放计划</a></section>

      {!selectedPlan ? <div className="panel-empty">当前 Project 暂无投放计划。<a href={planEditorURL}>前往创建</a></div> : legacyReadOnly ? <section className="delivery-config-config-card">
        <div className="delivery-config-empty-inline"><CircleAlert size={20}/><div><b>历史配置，仅供查看</b><p>这份计划不能继续修改、检查或提交。若要继续投放，请新建计划并选择目标广告平台。</p></div></div>
      </section> : <>
        {showConfiguration && platformConfiguration && editableConfiguration ? <section className="delivery-config-config-card"><header><div><span>当前计划 · V{selectedPlan.currentVersionNumber}</span><h3>{selectedPlan.currentVersion.name}</h3><p>更新于 {formatTime(selectedPlan.updatedAt)}</p></div><div className="delivery-config-contract"><span>配置草稿</span><button className="primary-button" type="button" onClick={() => void saveConfiguration()} disabled={busy}><Save size={15} aria-hidden="true"/>{busy ? '保存中…' : '保存'}</button></div></header><PlatformConfigurationEditor value={editableConfiguration} onChange={setEditableConfiguration} products={currentProject.products ?? []} assets={confirmedAssets} platformObjects={platformObjects} connectorAccounts={connectorAccounts} platformObjectError={platformObjectError} loadVideos={loadVideos} loadImages={loadImages} loadProductImages={loadProductImages} loadPhotos={loadPhotos} loadProducts={loadProducts} loadOptimizationTargets={loadOptimizationTargets} loadOptimizationCapabilities={loadOptimizationCapabilities} loadAuthorizedIdentities={loadAuthorizedIdentities} loadCategories={loadCategories} loadBrands={loadBrands}/><details className="delivery-config-mapping-details"><summary>查看 Manifest 字段映射</summary><PlatformConfigurationDetails value={editableConfiguration}/></details></section> : null}
        {showCalibration && platformConfiguration ? <CalibrationDispositionView value={platformConfiguration}/> : null}
        {showPreflight ? <section className="delivery-config-flow-grid delivery-config-flow-grid--preflight"><article className="delivery-config-preflight-card">
          <header><div><span className="section-label">真实受控执行</span><h3>选择驱动并检查配置</h3></div><strong className="delivery-config-preflight-state">尚未创建执行</strong></header>
          <fieldset className="delivery-config-driver-options">
            <legend>执行驱动</legend>
            <label className={executionDriver === 'oceanengine-web-api/session/v1' ? 'selected' : undefined}>
              <input type="radio" name="execution_driver" value="oceanengine-web-api/session/v1" checked={executionDriver === 'oceanengine-web-api/session/v1'} onChange={() => setExecutionDriver('oceanengine-web-api/session/v1')}/>
              <span><b>Web API</b><small>默认路线。使用已校准的巨量接口模板。</small></span>
            </label>
            <label className={executionDriver === 'playwright-rpa/edge/v3' ? 'selected' : undefined}>
              <input type="radio" name="execution_driver" value="playwright-rpa/edge/v3" checked={executionDriver === 'playwright-rpa/edge/v3'} onChange={() => setExecutionDriver('playwright-rpa/edge/v3')}/>
              <span><b>Playwright · Edge</b><small>使用本机 Edge 页面。适用于页面路径测试和明确回退。</small></span>
            </label>
          </fieldset>
          <div className="delivery-config-preflight-summary"><b>执行前置检查</b><p>服务端检查结构、预算、日期、引用和平台对象。创建执行后，驱动选择不能更改。</p><small>{executionDriver === 'playwright-rpa/edge/v3' ? 'Prepare 会连接本机 Edge，并停在最终点击边界。' : 'Web API 会使用本地模板，并在每个对象写入前要求一次性确认。'}</small></div>
          {referenceIntentIssues.length ? <div className="delivery-config-empty-inline"><CircleAlert size={20}/><div><b>平台对象未加入投放意图</b><p>{referenceIntentIssues.join('、')}。返回“配置映射”并保存。系统会生成包含这些引用的新计划版本。</p></div></div> : null}
          <div className="delivery-config-actions delivery-config-preflight-actions">
            <button className="primary-button" type="button" onClick={() => void startRealExecution()} disabled={busy || legacyReadOnly || referenceIntentIssues.length > 0}><Check size={14}/>{busy ? '正在创建…' : `使用${executionDriver === 'playwright-rpa/edge/v3' ? ' Playwright' : ' Web API'}创建执行`}</button>
          </div>
        </article></section> : null}
      </>}
      {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
    </div>
  </StateBoundary>
}
