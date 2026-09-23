import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Archive, Check, ImagePlus, Link2, Package, Pencil, Plus, RefreshCw, Save, Trash2, X } from 'lucide-react'
import {
  activityTypeLabels,
  brandTypeLabels,
  platformClient,
  productPriceBandLabels,
  type PlatformBrandType,
  type PlatformProduct,
  type PlatformProductCategory,
  type PlatformProductPriceBand,
  type PlatformProductProjectRef,
} from '../data/platformClient'
import { useProject } from '../context/ProjectContext'

type ProductForm = {
  category: PlatformProductCategory
  name: string
  priceBand: PlatformProductPriceBand | ''
  activityType: string
  brandType: PlatformBrandType | ''
  brandName: string
  description: string
  oceanEngineProductID: string
}

const emptyForm: ProductForm = {
  category: 'product', name: '', priceBand: '', activityType: '', brandType: '', brandName: '', description: '', oceanEngineProductID: '',
}

type ProductDialog = { mode: 'create' } | { mode: 'edit'; product: PlatformProduct }

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
}

function categoryLabel(category?: PlatformProductCategory) {
  return category === 'activity' ? '活动' : '商品'
}

function brandTypeLabel(brandType?: PlatformBrandType) {
  return brandType ? brandTypeLabels[brandType] : '—'
}

function priceBandLabel(band?: PlatformProductPriceBand) {
  return band ? productPriceBandLabels[band] : '—'
}

function activityLabel(product: PlatformProduct) {
  if (product.category === 'activity') {
    const type = product.activity_type ? (activityTypeLabels[product.activity_type] ?? product.activity_type) : '活动'
    return `${type} · ${product.activity_name || product.name}`
  }
  return `${priceBandLabel(product.price_band)} · ${product.brand_name || '未设置品牌'}`
}

function mappingStatus(product: PlatformProduct) {
  return product.ocean_engine_product_id
    ? <span className="status success"><span/>已录入 · {product.ocean_engine_product_id}</span>
    : <span className="status warning"><span/>待录入</span>
}

export function ProductsPage({ activeView }: { activeView: string }) {
  const { currentProject } = useProject()
  const isMapping = activeView === '巨量映射'
  const [products, setProducts] = useState<PlatformProduct[]>([])
  const [selectedId, setSelectedId] = useState('')
  const [dialog, setDialog] = useState<ProductDialog | null>(null)
  const [form, setForm] = useState<ProductForm>(emptyForm)
  const [imageFile, setImageFile] = useState<File | null>(null)
  const [removeImage, setRemoveImage] = useState(false)
  const [mappingInput, setMappingInput] = useState('')
  const [projectRefs, setProjectRefs] = useState<PlatformProductProjectRef[]>([])
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const loadRequestRef = useRef(0)

  const selected = useMemo(() => products.find(product => product.id === selectedId), [products, selectedId])

  const load = useCallback(async () => {
    const projectId = currentProject.id
    const requestId = loadRequestRef.current + 1
    loadRequestRef.current = requestId
    setBusy(true)
    setProducts([])
    setSelectedId('')
    setProjectRefs([])
    try {
      const values = await platformClient.listProducts()
      const linked = await Promise.all(values.map(async product => ({
        product,
        projects: await platformClient.listProductProjects(product.id),
      })))
      const scoped = linked.filter(item => item.projects.some(project => project.project_id === projectId)).map(item => item.product)
      if (loadRequestRef.current !== requestId) return
      setProducts(scoped)
      setSelectedId(scoped[0]?.id ?? '')
    } catch (error) {
      if (loadRequestRef.current === requestId) setNotice(error instanceof Error ? error.message : '加载产品目录失败')
    } finally {
      if (loadRequestRef.current === requestId) setBusy(false)
    }
  }, [currentProject.id])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    setSelectedId(current => products.some(product => product.id === current) ? current : products[0]?.id ?? '')
  }, [products])

  useEffect(() => {
    let active = true
    setProjectRefs([])
    if (!selectedId) return () => { active = false }
    void platformClient.listProductProjects(selectedId).then(refs => {
      if (active) setProjectRefs(refs)
    }).catch(() => {
      if (active) setProjectRefs([])
    })
    return () => { active = false }
  }, [selectedId])

  const refreshScopedProducts = async () => {
    await load()
  }

  const openCreate = () => {
    setForm(emptyForm)
    setImageFile(null)
    setRemoveImage(false)
    setDialog({ mode: 'create' })
  }

  const openEdit = (product: PlatformProduct) => {
    setForm({
      category: product.category,
      name: product.name,
      priceBand: product.price_band ?? '',
      activityType: product.activity_type ?? '',
      brandType: product.brand_type ?? '',
      brandName: product.brand_name ?? '',
      description: product.description ?? '',
      oceanEngineProductID: product.ocean_engine_product_id ?? '',
    })
    setImageFile(null)
    setRemoveImage(false)
    setDialog({ mode: 'edit', product })
  }

  const submitDialog = async () => {
    if (!form.name.trim()) {
      setNotice(`${form.category === 'activity' ? '活动' : '商品'}名称不能为空。`)
      return
    }
    if (form.category === 'activity' && !form.activityType.trim()) {
      setNotice('请选择活动类型。')
      return
    }
    const input = {
      name: form.name.trim(),
      category: form.category,
      price_band: form.category === 'product' && form.priceBand ? form.priceBand : undefined,
      activity_type: form.category === 'activity' ? form.activityType.trim() || undefined : undefined,
      activity_name: form.category === 'activity' ? form.name.trim() || undefined : undefined,
      brand_type: form.brandType || undefined,
      brand_name: form.brandName.trim() || undefined,
      description: form.description.trim() || undefined,
      ocean_engine_product_id: form.oceanEngineProductID.trim() || undefined,
    }
    setBusy(true)
    try {
      if (dialog?.mode === 'edit') {
        const updated = await platformClient.updateProduct(dialog.product.id, input)
        setProducts(current => current.map(item => item.id === updated.id ? updated : item))
        await applyImageChanges(dialog.product.id)
        setNotice(`${updated.name} 已更新。`)
      } else {
        const created = await platformClient.createProduct(input)
        await platformClient.linkProductToProject(created.id, currentProject.id)
        await applyImageChanges(created.id)
        await refreshScopedProducts()
        setNotice(`${created.name} 已创建并关联到「${currentProject.name}」。`)
      }
      setDialog(null)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : dialog?.mode === 'edit' ? '更新产品失败' : '创建产品失败')
    } finally {
      setBusy(false)
    }
  }

  // applyImageChanges uploads a newly selected image or clears the stored
  // image after the product object exists.
  const applyImageChanges = async (productId: string) => {
    if (imageFile) {
      const uploaded = await platformClient.putProductImage(productId, imageFile)
      setProducts(current => current.map(item => item.id === uploaded.id ? uploaded : item))
    } else if (removeImage) {
      const cleared = await platformClient.updateProduct(productId, { product_image: '' })
      setProducts(current => current.map(item => item.id === cleared.id ? cleared : item))
    }
  }

  const bindMapping = async (product: PlatformProduct) => {
    const value = mappingInput.trim()
    if (!value) return
    setBusy(true)
    try {
      const updated = await platformClient.updateProduct(product.id, { ocean_engine_product_id: value })
      setProducts(current => current.map(item => item.id === updated.id ? updated : item))
      setMappingInput('')
      setNotice(`${updated.name} 已回绑巨量商品 ${updated.ocean_engine_product_id}。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '回绑巨量商品失败')
    } finally {
      setBusy(false)
    }
  }

  const clearMapping = async (product: PlatformProduct) => {
    setBusy(true)
    try {
      const updated = await platformClient.updateProduct(product.id, { ocean_engine_product_id: undefined })
      setProducts(current => current.map(item => item.id === updated.id ? updated : item))
      setNotice(`${updated.name} 已清空巨量商品映射。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '清空映射失败')
    } finally {
      setBusy(false)
    }
  }

  const deleteProduct = async (product: PlatformProduct) => {
    if (!window.confirm(`确定删除「${product.name}」吗？删除后会解除所有项目关联，且不可恢复。`)) return
    setBusy(true)
    try {
      await platformClient.deleteProduct(product.id)
      await refreshScopedProducts()
      setNotice(`${product.name} 已删除。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '删除产品失败')
    } finally {
      setBusy(false)
    }
  }

  const toggleArchive = async (product: PlatformProduct) => {
    setBusy(true)
    try {
      const updated = await platformClient.updateProduct(product.id, { status: product.status === 'archived' ? 'active' : 'archived' })
      setProducts(current => current.map(item => item.id === updated.id ? updated : item))
      setNotice(updated.status === 'archived' ? `${updated.name} 已归档。` : `${updated.name} 已恢复为启用。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '更新产品状态失败')
    } finally {
      setBusy(false)
    }
  }

  const linkedToCurrentProject = selected && projectRefs.some(ref => ref.project_id === currentProject.id)

  const linkToCurrentProject = async () => {
    if (!selected || linkedToCurrentProject) return
    setBusy(true)
    try {
      await platformClient.linkProductToProject(selected.id, currentProject.id)
      const refs = await platformClient.listProductProjects(selected.id)
      setProjectRefs(refs)
      setNotice(`${selected.name} 已关联到「${currentProject.name}」，投放计划下拉即可选择。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '关联项目失败')
    } finally {
      setBusy(false)
    }
  }

  return <div className="products-view">
    <div className="table-surface">
      <div className="surface-toolbar">
        <div><span className="section-label">产品目录</span><h3>{isMapping ? '巨量映射' : '产品列表'}</h3><span className="products-count">{products.length} 个{isMapping ? '' : '产品'}</span></div>
        <div className="products-toolbar-actions">
          <button aria-label="刷新" onClick={() => void refreshScopedProducts()} disabled={busy}><RefreshCw size={15}/></button>
          <button className="primary-button" onClick={openCreate} disabled={busy}><Plus size={15}/>新建产品</button>
        </div>
      </div>

      {isMapping ? <table>
        <thead><tr><th>名称</th><th>类型 / 规格</th><th>映射状态</th><th>巨量商品 ID</th></tr></thead>
        <tbody>
          {products.map(product => <tr key={product.id}>
            <td><b>{product.name}</b><small>{product.id}</small></td>
            <td><b>{categoryLabel(product.category)}</b><small>{activityLabel(product)}</small></td>
            <td>{mappingStatus(product)}</td>
            <td>{product.ocean_engine_product_id
              ? <span className="products-mapping-value"><code>{product.ocean_engine_product_id}</code><button className="text-button" onClick={() => void clearMapping(product)} disabled={busy}>清空</button></span>
              : <form className="products-mapping-form" onSubmit={event => { event.preventDefault(); void bindMapping(product) }}>
                  <input placeholder="粘贴巨量商品 ID" value={mappingInput} onChange={event => setMappingInput(event.target.value)}/>
                  <button className="primary-button" type="submit" disabled={busy || !mappingInput.trim()}><Check size={14}/>回绑</button>
                </form>}
            </td>
          </tr>)}
        </tbody>
      </table> : <table>
        <thead><tr><th>名称</th><th>类型 / 规格</th><th>品牌</th><th>巨量映射</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          {products.map(product => <tr key={product.id} className={product.id === selectedId ? 'products-row-selected' : undefined} onClick={() => setSelectedId(product.id)}>
            <td><b>{product.name}</b><small className="products-id">{product.id}</small></td>
            <td><b>{categoryLabel(product.category)}</b><small>{activityLabel(product)}</small></td>
            <td><b>{brandTypeLabel(product.brand_type)}</b><small>{product.brand_name || '—'}</small></td>
            <td>{mappingStatus(product)}</td>
            <td><span className={`status ${product.status === 'archived' ? 'warning' : 'success'}`}><span/>{product.status === 'archived' ? '已归档' : '启用'}</span></td>
            <td><span className="products-row-actions">
              <button className="text-button" onClick={event => { event.stopPropagation(); openEdit(product) }} disabled={busy}><Pencil size={14}/>编辑</button>
              <button className="text-button" onClick={event => { event.stopPropagation(); void toggleArchive(product) }} disabled={busy}><Archive size={14}/>{product.status === 'archived' ? '恢复' : '归档'}</button>
              <button className="text-button danger" onClick={event => { event.stopPropagation(); void deleteProduct(product) }} disabled={busy}><Trash2 size={14}/>删除</button>
            </span></td>
          </tr>)}
        </tbody>
      </table>}
      {!products.length && !busy ? <div className="panel-empty"><Package size={22}/><h3>当前 Project 还没有产品</h3><p>点击右上角「新建产品」创建并关联当前 Project 的第一个产品。</p></div> : null}
    </div>

    {!isMapping && selected ? <div className="products-detail">
      <header>
        <div><span className="section-label">{categoryLabel(selected.category)}对象</span><h2>{selected.name}</h2><p>产品资料以 Cookies 目录为准；巨量商品 ID 只是平台映射。</p></div>
        <span className="products-detail-id">{selected.id}</span>
      </header>
      <dl>
        {selected.category === 'activity'
          ? <><div><dt>活动类型</dt><dd>{selected.activity_type ? (activityTypeLabels[selected.activity_type] ?? selected.activity_type) : '—'}</dd></div>
              <div><dt>活动名称</dt><dd>{selected.activity_name || selected.name}</dd></div></>
          : <><div><dt>价格带</dt><dd>{priceBandLabel(selected.price_band)}</dd></div>
              <div><dt>商品图片</dt><dd>{selected.product_image ? <img className="products-detail-image" src={platformClient.productImageUrl(selected.id)} alt={selected.name}/> : '—'}</dd></div></>}
        <div><dt>品牌类型</dt><dd>{brandTypeLabel(selected.brand_type)}</dd></div>
        <div><dt>品牌名称</dt><dd>{selected.brand_name || '—'}</dd></div>
        <div><dt>描述</dt><dd>{selected.description || '—'}</dd></div>
        <div><dt>创建时间</dt><dd>{formatTime(selected.created_at)}</dd></div>
      </dl>
      <footer>
        <div><span>巨量映射</span>{mappingStatus(selected)}</div>
        <div><span>项目关联</span><em>{projectRefs.length ? projectRefs.map(ref => ref.name).join('、') : '尚未关联项目'}</em>
          {!linkedToCurrentProject ? <button className="text-button" onClick={() => void linkToCurrentProject()} disabled={busy}><Link2 size={14}/>关联到当前项目</button> : <span className="products-linked-badge">已关联到当前项目</span>}
        </div>
      </footer>
    </div> : null}

    {dialog ? <div className="task-dialog-backdrop" role="dialog" aria-modal="true" onClick={() => { if (!busy) setDialog(null) }}>
      <form className="products-dialog" onSubmit={event => { event.preventDefault(); void submitDialog() }} onClick={event => event.stopPropagation()}>
        <header>
          <div><span className="section-label">{dialog.mode === 'edit' ? '编辑产品' : '新建产品'}</span><h3>{dialog.mode === 'edit' ? `编辑 ${dialog.product.name}` : '新建产品'}</h3></div>
          <button type="button" aria-label="关闭" onClick={() => setDialog(null)} disabled={busy}><X size={16}/></button>
        </header>

        <label><span className="products-dialog-label">商品类型</span>
          <span className="products-category-switch">
            <button type="button" className={form.category === 'product' ? 'active' : ''} onClick={() => setForm(current => ({ ...current, category: 'product', activityType: '' }))}>商品</button>
            <button type="button" className={form.category === 'activity' ? 'active' : ''} onClick={() => setForm(current => ({ ...current, category: 'activity', priceBand: '' }))}>活动</button>
          </span>
        </label>

        {form.category === 'activity' ? <>
          <label><span className="products-dialog-label">活动类型</span>
            <select value={form.activityType} onChange={event => setForm(current => ({ ...current, activityType: event.target.value }))}>
              <option value="">请选择活动类型</option>
              {Object.entries(activityTypeLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
          </label>
          <label><span className="products-dialog-label">活动名称</span><input autoFocus required placeholder="如 双十一红包雨" value={form.name} onChange={event => setForm(current => ({ ...current, name: event.target.value }))}/></label>
        </> : <>
          <label><span className="products-dialog-label">商品名称</span><input autoFocus required placeholder="如 山茶花面霜" value={form.name} onChange={event => setForm(current => ({ ...current, name: event.target.value }))}/></label>
          <label><span className="products-dialog-label">商品图片</span>
            <span className="products-image-picker">
              {imageFile
                ? <><img className="products-image-preview" src={URL.createObjectURL(imageFile)} alt="商品图片预览"/><small>{imageFile.name}</small></>
                : dialog.mode === 'edit' && !removeImage && dialog.product.product_image
                  ? <><img className="products-image-preview" src={platformClient.productImageUrl(dialog.product.id)} alt="当前商品图片"/><small>当前图片</small></>
                  : <span className="products-image-empty"><ImagePlus size={16}/>选择图片</span>}
              <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" onChange={event => {
                const file = event.target.files?.[0]
                if (file) { setImageFile(file); setRemoveImage(false) }
                event.target.value = ''
              }}/>
              {(imageFile || (dialog.mode === 'edit' && dialog.product.product_image && !removeImage)) ? <button type="button" className="text-button" onClick={() => { setImageFile(null); setRemoveImage(true) }}><Trash2 size={13}/>移除</button> : null}
            </span>
          </label>
          <label><span className="products-dialog-label">价格带</span>
            <select value={form.priceBand} onChange={event => setForm(current => ({ ...current, priceBand: event.target.value as PlatformProductPriceBand }))}>
              <option value="">请选择价格带</option>
              {Object.entries(productPriceBandLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
          </label>
        </>}

        <label><span className="products-dialog-label">品牌类型</span>
          <select value={form.brandType} onChange={event => setForm(current => ({ ...current, brandType: event.target.value as PlatformBrandType }))}>
            <option value="">请选择品牌类型</option>
            {Object.entries(brandTypeLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <label><span className="products-dialog-label">品牌名称</span><input placeholder="如 香缇卡 / 自营旗舰店" value={form.brandName} onChange={event => setForm(current => ({ ...current, brandName: event.target.value }))}/></label>
        <label><span className="products-dialog-label">{form.category === 'activity' ? '活动描述' : '商品描述'}</span><textarea rows={3} placeholder={form.category === 'activity' ? '如 全场满减红包活动' : '如 保湿面霜 50ml'} value={form.description} onChange={event => setForm(current => ({ ...current, description: event.target.value }))}/></label>
        <label><span className="products-dialog-label">巨量商品 ID <small>可选 · 人工预先在巨量创建后填写</small></span><input placeholder="如 1700000000000000000" value={form.oceanEngineProductID} onChange={event => setForm(current => ({ ...current, oceanEngineProductID: event.target.value }))}/></label>

        <footer>
          <button type="button" className="secondary-button" onClick={() => setDialog(null)} disabled={busy}>取消</button>
          <button type="submit" className="primary-button" disabled={busy}><Save size={14}/>{busy ? '保存中…' : dialog.mode === 'edit' ? '保存修改' : '创建产品'}</button>
        </footer>
      </form>
    </div> : null}

    {notice ? <div className="inline-notice" role="status">{notice}</div> : null}
  </div>
}
