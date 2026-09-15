import { useEffect, useMemo, useRef, useState } from 'react'
import { Check, Package } from 'lucide-react'
import type { StableReference } from '../api/delivery'
import type { ApiAssetVersionPointer, ApiConnectorPlatformObject, ApiOptimizationTargetCapabilitySnapshot } from '../data/api'
import { oceanEngineImageSourceIdentity } from '../lib/oceanengine-product-image'
import { optimizationCapabilitySelectionMatches } from '../lib/oceanengineBranchConstraints'
function formatTime(value?: string) { return value ? new Date(value).toLocaleString('zh-CN') : '未知' }
function errorMessage(error: unknown, fallback: string) { return error instanceof Error ? error.message : fallback }

export type PlatformObjectPage = { items: ApiConnectorPlatformObject[]; next_cursor: string }
export type PlatformObjectSort = 'created_at' | 'ctr' | 'conversions'
export type PlatformObjectLoader = (query: string, cursor: string | undefined, sortBy: PlatformObjectSort, sortOrder: 'asc' | 'desc') => Promise<PlatformObjectPage>
export type DouyinVideoLoader = (iesCoreUserID: string, query: string, cursor: string | undefined, sortBy: PlatformObjectSort, sortOrder: 'asc' | 'desc') => Promise<PlatformObjectPage>

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
  return reference.audit_attributes?.connector_platform_object_id ? `connector:${reference.audit_attributes.connector_platform_object_id}` : reference.namespace === 'cookies' ? `cookies:${reference.id}@${reference.version ?? '1'}` : `${reference.namespace}:${reference.scope}:${reference.object_kind}:${reference.id}`
}

export function MaterialObjectPicker({ label, assets, platformObjects = [], value, objectKind, loadPlatformObjects, onChange }: { label: string; assets: ApiAssetVersionPointer[]; platformObjects?: ApiConnectorPlatformObject[]; value: StableReference[]; objectKind: string; loadPlatformObjects?: PlatformObjectLoader; onChange: (value: StableReference[]) => void }) {
  const productImageMode = objectKind === 'product_image'
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [preview, setPreview] = useState<{ url: string; mediaKind: 'image' | 'video'; label: string }>()
  const [remoteObjects, setRemoteObjects] = useState<ApiConnectorPlatformObject[]>(platformObjects)
  const [knownPlatformObjects, setKnownPlatformObjects] = useState<ApiConnectorPlatformObject[]>(platformObjects)
  const [nextCursor, setNextCursor] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [retry, setRetry] = useState(0)
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
    const asset = reference.namespace === 'cookies' ? assetByID.get(reference.id ?? '') : undefined
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
    setLoading(true)
    setNextCursor('')
    setRemoteObjects([])
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
    return () => { window.clearTimeout(timer); searchGenerationRef.current += 1 }
  }, [loadPlatformObjects, open, query, sortBy, sortOrder, retry])

  const loadMore = async () => {
    if (!loadPlatformObjects || !nextCursor || loading) return
    const generation = searchGenerationRef.current
    setLoading(true)
    setLoadError('')
    try {
      const page = await loadPlatformObjects(query.trim(), nextCursor, sortBy, sortOrder)
      if (generation !== searchGenerationRef.current) return
      setRemoteObjects(current => mergePlatformObjects(current, page.items))
      setKnownPlatformObjects(current => mergePlatformObjects(current, page.items))
      setNextCursor(page.next_cursor)
    } catch (error) {
      if (generation !== searchGenerationRef.current) return
      setLoadError(errorMessage(error, '读取更多 Connector 素材失败。'))
    } finally {
      if (generation === searchGenerationRef.current) setLoading(false)
    }
  }
  const toggle = (asset: ApiAssetVersionPointer) => setDraftIds(current => current.includes(`cookies:${asset.assetId}@${asset.humanConfirmedVersion}`) ? current.filter(id => id !== `cookies:${asset.assetId}@${asset.humanConfirmedVersion}`) : [...current, `cookies:${asset.assetId}@${asset.humanConfirmedVersion}`])
  const togglePlatformObject = (item: ApiConnectorPlatformObject) => {
    const id = `connector:${item.id}`
    setDraftIds(current => current.includes(id) ? current.filter(value => value !== id) : [...current, id])
  }
  const confirm = () => {
    onChange(draftIds.map((id): StableReference => {
      const existing = value.find(reference => materialSelectionID(reference) === id)
      if (existing) return existing
      const platformObject = knownPlatformObjects.find(item => `connector:${item.id}` === id)
      if (platformObject) return { namespace: 'oceanengine', object_kind: platformObject.object_kind, scope: `account:${platformObject.account_id}`, id: platformObject.platform_object_id, version: String(platformObject.version), state: 'resolved' as const, display_name_snapshot: platformObject.display_name || platformObject.platform_object_id, audit_attributes: { connector_platform_object_id: platformObject.id, ocean_engine_material_id: platformObject.platform_object_id, preview_url: platformObject.preview_url ?? '', ...(productImageMode ? { image_src_identity: oceanEngineImageSourceIdentity(platformObject.metadata.web_uri ?? platformObject.preview_url), minimum_visible: '1' } : {}) } } satisfies StableReference
      const asset = assets.find(item => `cookies:${item.assetId}@${item.humanConfirmedVersion}` === id)
      return { namespace: 'cookies', object_kind: 'asset_version', scope: `project:${asset?.projectId}`, id: asset?.assetId, version: String(asset?.humanConfirmedVersion ?? asset?.workingVersion ?? 0), state: 'resolved' as const, display_name_snapshot: asset?.assetId, audit_attributes: { ocean_engine_material_id: asset?.oceanEngineMaterialId ?? '', media_kind: asset?.mediaKind ?? 'image' } } satisfies StableReference
    }))
    setOpen(false)
  }
  return <fieldset className={`delivery-config-object-picker${productImageMode ? ' delivery-config-object-picker--product-image' : ''}`}>
    <legend>{label}</legend>
    {value.filter(reference => reference.state !== 'resolved').map((reference, index) => <p key={index} role="status">{reference.display_name_snapshot || reference.id}：{reference.reason || '原引用当前不可用，请重新选择。'}</p>)}
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
        {loadError ? <div className="delivery-material-error" role="alert">{loadError}<button type="button" onClick={() => setRetry(value => value + 1)}>重试</button></div> : null}
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
          {filteredAssets.map(asset => <label key={asset.id} className={`delivery-material-card delivery-material-card--cookies${draftSelected.has(`cookies:${asset.assetId}@${asset.humanConfirmedVersion}`) ? ' selected' : ''}`}><input type="checkbox" checked={draftSelected.has(`cookies:${asset.assetId}@${asset.humanConfirmedVersion}`)} onChange={() => toggle(asset)}/><button type="button" className="delivery-material-preview-button delivery-material-preview-button--landscape" onClick={() => asset.contentUrl && setPreview({ url: asset.contentUrl, mediaKind: asset.mediaKind === 'video' ? 'video' : 'image', label: asset.assetId })}>{asset.contentUrl ? asset.mediaKind === 'video' ? <video src={asset.contentUrl} preload="metadata"/> : <img src={asset.contentUrl} alt=""/> : <span>无预览</span>}<span className="delivery-material-preview-type">{asset.mediaKind === 'video' ? '视频' : '图片'}</span></button><b title={asset.assetId}>{asset.assetId}</b><small>{asset.oceanEngineMaterialId ? 'Cookies · 已录入巨量' : 'Cookies · 待 RPA 录入'}</small></label>)}
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

export function MarketingProductPicker({ value, cookiesProducts, loadPlatformObjects, onChange }: { value?: StableReference; cookiesProducts: MarketingProductOption[]; loadPlatformObjects: PlatformObjectLoader; onChange: (value?: StableReference) => void }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<ApiConnectorPlatformObject[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [selectedID, setSelectedID] = useState(() => marketingProductSelectionID(value))
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [retry, setRetry] = useState(0)
  const generationRef = useRef(0)
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const filteredCookiesProducts = useMemo(() => cookiesProducts.filter(product => !normalizedQuery || `${product.name} ${product.id} ${product.oceanEngineProductId ?? ''}`.toLocaleLowerCase().includes(normalizedQuery)), [cookiesProducts, normalizedQuery])

  useEffect(() => {
    if (!open) return
    const generation = ++generationRef.current
    setLoading(true)
    setNextCursor('')
    setItems([])
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
    return () => { window.clearTimeout(timer); generationRef.current += 1 }
  }, [loadPlatformObjects, open, query, retry])

  const loadMore = async () => {
    if (!nextCursor || loading) return
    const generation = generationRef.current
    setLoading(true)
    try {
      const page = await loadPlatformObjects(query.trim(), nextCursor, 'created_at', 'desc')
      if (generation !== generationRef.current) return
      setItems(current => mergePlatformObjects(current, page.items))
      setNextCursor(page.next_cursor)
    } catch (error) {
      if (generation !== generationRef.current) return
      setLoadError(errorMessage(error, '读取更多巨量营销产品失败。'))
    } finally {
      if (generation === generationRef.current) setLoading(false)
    }
  }
  const confirm = () => {
    if (!selectedID) {
      onChange(undefined)
      setOpen(false)
      return
    }
    if (selectedID === marketingProductSelectionID(value)) { onChange(value); setOpen(false); return }
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
    {value && value.state !== 'resolved' ? <p role="status">原产品：{value.display_name_snapshot || value.id}；{value.reason || '当前不可用，请重新选择。'}</p> : null}
    <div className="delivery-config-object-summary"><div className="delivery-product-summary">{value ? <><span><Package size={20} aria-hidden="true"/></span><div><b>{value.display_name_snapshot || value.id}</b><small>{value.namespace === 'cookies' ? 'Cookies' : 'Connector'}</small></div></> : <small>尚未选择营销产品</small>}</div><button className="secondary-button" type="button" onClick={() => { setSelectedID(marketingProductSelectionID(value)); setOpen(true) }}>选择产品</button></div>
    {open ? <div className="delivery-product-picker-backdrop" role="presentation" onClick={() => setOpen(false)}><section className="delivery-product-picker" role="dialog" aria-modal="true" aria-label="营销产品选择器" onClick={event => event.stopPropagation()}>
      <header><div><span className="section-label">MARKETING PRODUCT</span><h3>营销产品</h3><p>从 Cookies 产品或 Connector 已导入产品中选择一个。</p></div><button className="text-button" type="button" onClick={() => setOpen(false)}>关闭</button></header>
      <div className="delivery-product-picker-toolbar"><input autoFocus placeholder="搜索产品名称、product_id 或 unique_product_id" value={query} onChange={event => setQuery(event.target.value)}/><span>当前结果 {filteredCookiesProducts.length + items.length} 个</span></div>
      {!loading && value && !filteredCookiesProducts.some(product => `cookies:${product.id}` === marketingProductSelectionID(value)) && !items.some(item => `connector:${item.id}` === marketingProductSelectionID(value)) ? <p role="status">已选产品不在当前搜索结果中，保留原引用。</p> : null}
      {loadError ? <div className="delivery-material-error" role="alert">{loadError}<button type="button" onClick={() => setRetry(value => value + 1)}>重试</button></div> : null}
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

export function ReferenceObjectPicker({ label, pickerTitle, value, objectKind, loadPlatformObjects, requiredContext, onChange }: { label: string; pickerTitle: string; value?: StableReference; objectKind: string; loadPlatformObjects: PlatformObjectLoader; requiredContext?: string; onChange: (value?: StableReference) => void }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<ApiConnectorPlatformObject[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [selectedID, setSelectedID] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [retry, setRetry] = useState(0)
  const generationRef = useRef(0)
  const visibleItems = useMemo(() => requiredContext ? items.filter(item => Array.isArray(item.metadata.contexts) && item.metadata.contexts.includes(requiredContext)) : items, [items, requiredContext])

  useEffect(() => {
    if (!open) return
    const generation = ++generationRef.current
    setLoading(true)
    setNextCursor('')
    setItems([])
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
    return () => { window.clearTimeout(timer); generationRef.current += 1 }
  }, [label, loadPlatformObjects, open, query, retry])

  const loadMore = async () => {
    if (!nextCursor || loading) return
    const generation = generationRef.current
    setLoading(true)
    try {
      const page = await loadPlatformObjects(query.trim(), nextCursor, 'created_at', 'desc')
      if (generation !== generationRef.current) return
      setItems(current => mergePlatformObjects(current, page.items))
      setNextCursor(page.next_cursor)
    } catch (error) {
      if (generation !== generationRef.current) return
      setLoadError(errorMessage(error, `读取更多${label}失败。`))
    } finally {
      if (generation === generationRef.current) setLoading(false)
    }
  }
  const confirm = () => {
    if (!selectedID) {
      onChange(undefined)
      setOpen(false)
      return
    }
    if (selectedID === (value?.audit_attributes?.connector_platform_object_id ?? value?.id)) { onChange(value); setOpen(false); return }
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
      audit_attributes: {
        connector_platform_object_id: item.id, platform_object_id: item.platform_object_id,
        ...(objectKind === 'douyin_video' ? { ies_core_user_id: String(item.metadata.ies_core_user_id ?? ''), video_id: String(item.metadata.video_id ?? '') } : {}),
        ...(objectKind === 'application' ? {
          basic_package_id: String(item.metadata.basic_package_id ?? ''),
          package_name: String(item.metadata.package_name ?? ''),
        } : {}),
      },
    })
    setOpen(false)
  }
  return <fieldset className="delivery-config-object-picker">
    <legend>{label}</legend>
    <div className="delivery-config-object-summary"><div className="delivery-product-summary">{value ? <><span><Package size={20} aria-hidden="true"/></span><div><b>{value.display_name_snapshot || value.id}</b><small>Connector · {value.id}</small></div></> : <small>尚未选择{label}</small>}</div><button className="secondary-button" type="button" onClick={() => { setSelectedID(value?.audit_attributes?.connector_platform_object_id ?? value?.id ?? ''); setOpen(true) }}>{pickerTitle}</button></div>
    {open ? <div className="delivery-product-picker-backdrop" role="presentation" onClick={() => setOpen(false)}><section className="delivery-product-picker" role="dialog" aria-modal="true" aria-label={`${label}选择器`} onClick={event => event.stopPropagation()}>
      <header><div><span className="section-label">CONNECTOR OBJECT</span><h3>{label}</h3><p>从当前巨量账户已同步的对象中选择一个。</p></div><button className="text-button" type="button" onClick={() => setOpen(false)}>关闭</button></header>
      <div className="delivery-product-picker-toolbar"><input autoFocus placeholder={objectKind === 'douyin_video' ? '搜索视频标题、作者或视频 ID' : `搜索${label}名称或平台 ID`} value={query} onChange={event => setQuery(event.target.value)}/><span>当前结果 {visibleItems.length} 个</span></div>
      {loadError ? <div className="delivery-material-error" role="alert">{loadError}<button type="button" onClick={() => setRetry(value => value + 1)}>重试</button></div> : null}
      <div className="delivery-product-picker-grid">
        {visibleItems.map(item => { const name = item.display_name || item.platform_object_id; return <label key={item.id} className={`delivery-product-card${selectedID === item.id ? ' selected' : ''}`}><input type="radio" name={`reference_${objectKind}`} checked={selectedID === item.id} onChange={() => setSelectedID(item.id)}/><span className="delivery-product-card-thumbnail">{item.preview_url ? <img src={item.preview_url} alt="" loading="lazy"/> : <Package size={26} aria-hidden="true"/>}</span><span className="delivery-product-card-body"><b title={name}>{name}</b><small>{objectKind === 'douyin_video' ? String(item.metadata.aweme_nickname ?? '抖音原生视频') : label}</small>{objectKind === 'douyin_video' ? <small>抖音号 ID：{String(item.metadata.ies_core_user_id ?? '未知')}</small> : null}<code>{item.platform_object_id}</code></span><span className="delivery-product-card-source">Connector</span></label> })}
        {!loading && !visibleItems.length ? <div className="delivery-product-picker-empty">没有匹配的{label}。</div> : null}
      </div>
      {nextCursor ? <button className="secondary-button delivery-product-picker-more" type="button" disabled={loading} onClick={() => void loadMore()}>{loading ? '读取中…' : '加载更多'}</button> : null}
      <footer><button className="secondary-button" type="button" onClick={() => setOpen(false)}>取消</button><button className="text-button" type="button" onClick={() => setSelectedID('')}>清空</button><button className="primary-button" type="button" onClick={confirm}>确认选择</button></footer>
    </section></div> : null}
  </fieldset>
}

export function OptimizationTargetCapabilityField({ accountID, value, snapshot, loading, error, onChange }: { accountID: string; value?: StableReference; snapshot?: ApiOptimizationTargetCapabilitySnapshot; loading: boolean; error: string; onChange: (value?: StableReference) => void }) {
  const current = snapshot && optimizationCapabilitySelectionMatches({ optimization_target_reference: value }, snapshot.snapshot_id, snapshot.options.map(option => option.external_action))
  const selected = current ? snapshot.options.find(option => option.external_action === value?.id) : undefined
  return <label><span>优化目标 · {selected ? '必填' : '必填 · 待补'}</span>
    <select aria-label="优化目标" value={selected?.external_action ?? ''} disabled={loading || !snapshot} onChange={event => {
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
function materialReferenceKind(reference: StableReference, assets: ApiAssetVersionPointer[]): MaterialEditorTab | undefined {
  if (reference.object_kind === 'video_material') return 'video'
  if (reference.object_kind === 'image_material') return 'image'
  if (reference.object_kind === 'aweme_photo_material') return 'graphic'
  if (reference.audit_attributes?.media_kind === 'video') return 'video'
  if (reference.audit_attributes?.media_kind === 'image') return 'image'
  const asset = assets.find(item => item.assetId === reference.id)
  if (!asset) return undefined
  return asset.mediaKind === 'video' ? 'video' : 'image'
}

export function BaseMaterialsField({ value, assets, platformObjects = [], loadVideos, loadImages, loadPhotos, onChange }: { value: StableReference[]; assets: ApiAssetVersionPointer[]; platformObjects?: ApiConnectorPlatformObject[]; loadVideos: PlatformObjectLoader; loadImages: PlatformObjectLoader; loadPhotos: PlatformObjectLoader; onChange: (value: StableReference[]) => void }) {
  const [tab, setTab] = useState<MaterialEditorTab>('video')
  const referencesFor = (kind: MaterialEditorTab) => value.filter(reference => materialReferenceKind(reference, assets) === kind)
  const updateReferences = (kind: MaterialEditorTab, next: StableReference[]) => onChange([...value.filter(reference => materialReferenceKind(reference, assets) !== kind), ...next])
  const tabCounts = {
    video: referencesFor('video').length,
    image: referencesFor('image').length,
    graphic: referencesFor('graphic').length,
  }
  const { video, image, graphic } = tabCounts
  useEffect(() => {
    setTab(current => {
      const counts = { video, image, graphic }
      if (counts[current]) return current
      return (['video', 'image', 'graphic'] as const).find(kind => counts[kind]) ?? current
    })
  }, [video, image, graphic])

  return (
    <div className="delivery-config-material-group">
      <header><b>基础素材{!value.length ? ' · 必填 · 待补' : ''}</b><small>视频 {tabCounts.video}/30 · 图片 {tabCounts.image}/50 · 图文 {tabCounts.graphic}/10</small></header>
      {value.filter(reference => !materialReferenceKind(reference, assets)).map((reference, index) => <p key={index}>原素材：{reference.display_name_snapshot || reference.id}（当前目录未确认，保留原引用）</p>)}
      <div className="delivery-config-material-tabs" role="tablist" aria-label="基础素材类型">
        {(['video', 'image', 'graphic'] as const).map(value => <button key={value} type="button" role="tab" aria-selected={tab === value} className={tab === value ? 'active' : ''} onClick={() => setTab(value)}>{value === 'video' ? '视频' : value === 'image' ? '图片' : '图文'} <span>{tabCounts[value]}</span></button>)}
      </div>
      <div className="delivery-config-material-tab-panel" role="tabpanel">
        {tab === 'video' ? <MaterialObjectPicker label="视频素材" assets={assets.filter(asset => asset.mediaKind === 'video')} platformObjects={platformObjects.filter(item => item.object_kind === 'video_material')} loadPlatformObjects={loadVideos} value={referencesFor('video')} objectKind="material" onChange={value => updateReferences('video', value)}/> : null}
        {tab === 'image' ? <MaterialObjectPicker label="图片素材" assets={assets.filter(asset => asset.mediaKind !== 'video')} platformObjects={platformObjects.filter(item => item.object_kind === 'image_material')} loadPlatformObjects={loadImages} value={referencesFor('image')} objectKind="material" onChange={value => updateReferences('image', value)}/> : null}
        {tab === 'graphic' ? <MaterialObjectPicker label="抖音图文素材" assets={[]} platformObjects={platformObjects.filter(item => item.object_kind === 'aweme_photo_material')} loadPlatformObjects={loadPhotos} value={referencesFor('graphic')} objectKind="material" onChange={value => updateReferences('graphic', value)}/> : null}
      </div>
    </div>
  )
}
