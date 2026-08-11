import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, Database, RefreshCw } from 'lucide-react'
import { useProject } from '../../context/ProjectContext'
import type { ProductionCenterGateway } from './gateway'
import { HttpProductionCenterGateway, ProductionGatewayError } from './httpGateway'
import { ProductionAssetList } from './ProductionAssetList'
import { emptyProductionFilters, ProductionFilters, type ProductionFilterState } from './ProductionFilters'
import { ProductionRunDrawer } from './ProductionRunDrawer'
import { ProductionRunTable } from './ProductionRunTable'
import type { ProductionAssetPage, ProductionRunDetail, ProductionRunPage, ProductionRunQuery, ProductionRunRef, ProductionRunSummary } from './types'
import { activeProductionStatuses, productionQueryForView } from './viewModel'
import './production-center.css'

const defaultGateway = new HttpProductionCenterGateway()

function selectedRef(value?: string): ProductionRunRef | undefined {
  if (!value) return undefined
  const [source, ...rest] = value.split('~')
  const id = rest.join('~')
  if (!id || !['provider', 'creative_render', 'editing_render', 'audio_render'].includes(source)) return undefined
  return { source: source as ProductionRunRef['source'], id }
}

function initialFilters(): ProductionFilterState {
  if (typeof window === 'undefined') return emptyProductionFilters
  const query = new URLSearchParams(window.location.search)
  const localDate = (value: string | null) => {
    if (!value) return ''
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? '' : parsed.toISOString().slice(0, 16)
  }
  return {
    query: query.get('q') ?? '',
    status: (query.get('status') ?? '') as ProductionFilterState['status'],
    sourceTaskId: query.get('source_task_id') ?? '',
    createdAfter: localDate(query.get('created_after')),
    createdBefore: localDate(query.get('created_before')),
  }
}

function withFilters(base: ProductionRunQuery, filters: ProductionFilterState, cursor: string): ProductionRunQuery {
  return {
    ...base,
    q: filters.query.trim() || undefined,
    status: filters.status ? [filters.status] : base.status,
    source_task_id: filters.sourceTaskId.trim() || undefined,
    created_after: filters.createdAfter ? new Date(filters.createdAfter).toISOString() : undefined,
    created_before: filters.createdBefore ? new Date(filters.createdBefore).toISOString() : undefined,
    cursor: cursor || undefined,
  }
}

export function ProductionCenterPage({ activeView, objectId, gateway = defaultGateway, onOpenRun, onCloseRun, onOpenSource }: {
  activeView: string
  objectId?: string
  gateway?: ProductionCenterGateway
  onOpenRun: (ref: ProductionRunRef) => void
  onCloseRun: () => void
  onOpenSource: (run: ProductionRunSummary) => void
}) {
  const { currentProject } = useProject()
  const projectId = currentProject.id
  const ref = useMemo(() => selectedRef(objectId), [objectId])
  const [filters, setFilters] = useState<ProductionFilterState>(initialFilters)
  const [cursor, setCursor] = useState(() => typeof window === 'undefined' ? '' : new URLSearchParams(window.location.search).get('cursor') ?? '')
  const [cursorHistory, setCursorHistory] = useState<string[]>([])
  const [runs, setRuns] = useState<ProductionRunPage | null>(null)
  const [assets, setAssets] = useState<ProductionAssetPage | null>(null)
  const [detail, setDetail] = useState<ProductionRunDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [detailLoading, setDetailLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [retrying, setRetrying] = useState(false)
  const [retryError, setRetryError] = useState<string | null>(null)
  const [requiresSourceWorkflow, setRequiresSourceWorkflow] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const requestVersion = useRef(0)
  const detailRequestVersion = useRef(0)
  const retryKey = useRef<{ ref: string; key: string } | null>(null)
  const isAssets = activeView === '源素材'

  const changeFilters = useCallback((next: ProductionFilterState) => {
    setFilters(next)
    setCursor('')
    setCursorHistory([])
  }, [])

  useEffect(() => {
    setRuns(null)
    setAssets(null)
    setDetail(null)
    setError(null)
    setRetryError(null)
    setRequiresSourceWorkflow(false)
    setCursor('')
    setCursorHistory([])
  }, [activeView, projectId])

  useEffect(() => {
    setRetryError(null)
    setRequiresSourceWorkflow(false)
    setRetrying(false)
  }, [ref?.source, ref?.id])

  useEffect(() => {
    if (!projectId) return
    const version = ++requestVersion.current
    const handle = window.setTimeout(() => {
      setLoading(true)
      setError(null)
      const request = isAssets
        ? gateway.listAssets(projectId, { cursor: cursor || undefined, limit: 50 })
        : gateway.listRuns(projectId, withFilters(productionQueryForView(activeView) ?? { limit: 50 }, filters, cursor))
      void request.then(page => {
        if (requestVersion.current !== version) return
        if (isAssets) setAssets(page as ProductionAssetPage)
        else setRuns(page as ProductionRunPage)
      }).catch(cause => {
        if (requestVersion.current !== version || cause instanceof DOMException && cause.name === 'AbortError') return
        setError(cause instanceof ProductionGatewayError && cause.status === 403 ? '当前角色没有查看制作任务的权限。' : cause instanceof Error ? cause.message : '制作中心读取失败。')
      }).finally(() => {
        if (requestVersion.current === version) setLoading(false)
      })
    }, 180)
    return () => window.clearTimeout(handle)
  }, [activeView, cursor, filters, gateway, isAssets, projectId, refresh])

  useEffect(() => {
    if (!projectId || !ref) {
      detailRequestVersion.current++
      setDetail(null)
      setDetailError(null)
      return
    }
    const version = ++detailRequestVersion.current
    setDetail(null)
    setDetailLoading(true)
    setDetailError(null)
    void gateway.getRun(projectId, ref).then(value => {
      if (detailRequestVersion.current === version) setDetail(value)
    }).catch(cause => {
      if (detailRequestVersion.current !== version || cause instanceof DOMException && cause.name === 'AbortError') return
      setDetailError(cause instanceof Error ? cause.message : '制作任务详情读取失败。')
    }).finally(() => {
      if (detailRequestVersion.current === version) setDetailLoading(false)
    })
  }, [gateway, projectId, ref?.id, ref?.source])

  useEffect(() => {
    if (typeof window === 'undefined') return
    const url = new URL(window.location.href)
    const values: Record<string, string> = {
      q: filters.query.trim(), status: filters.status, source_task_id: filters.sourceTaskId.trim(), cursor,
      created_after: filters.createdAfter ? new Date(filters.createdAfter).toISOString() : '',
      created_before: filters.createdBefore ? new Date(filters.createdBefore).toISOString() : '',
    }
    for (const [key, value] of Object.entries(values)) value ? url.searchParams.set(key, value) : url.searchParams.delete(key)
    window.history.replaceState({}, '', `${url.pathname}${url.search}`)
  }, [cursor, filters])

  const hasActiveRuns = runs?.items.some(item => activeProductionStatuses.has(item.normalized_status)) ?? false
  useEffect(() => {
    if (!hasActiveRuns) return
    const timer = window.setTimeout(() => setRefresh(value => value + 1), 5000)
    return () => window.clearTimeout(timer)
  }, [hasActiveRuns, refresh, runs])
  useEffect(() => {
    const onFocus = () => setRefresh(value => value + 1)
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [])

  const health = (isAssets ? assets?.source_health : runs?.source_health) ?? []
  const unavailable = health.filter(item => item.status === 'unavailable')
  const items = isAssets ? assets?.items ?? [] : runs?.items ?? []
  const nextCursor = isAssets ? assets?.next_cursor : runs?.next_cursor
  const openSource = (run: ProductionRunSummary) => run.actions.open_source && run.source_task ? onOpenSource(run) : undefined
  const detailRun = detail?.summary ?? runs?.items.find(item => item.ref.source === ref?.source && item.ref.id === ref?.id)
  const retryRun = useCallback(async () => {
    if (!projectId || !ref || retrying) return
    const refKey = `${ref.source}:${ref.id}`
    if (retryKey.current?.ref !== refKey) {
      retryKey.current = { ref: refKey, key: `production_retry_${crypto.randomUUID().replaceAll('-', '_')}` }
    }
    setRetrying(true)
    setRetryError(null)
    setRequiresSourceWorkflow(false)
    try {
      const result = await gateway.retryRun(projectId, ref, retryKey.current.key)
      retryKey.current = null
      setRefresh(value => value + 1)
      onOpenRun(result.new_run)
    } catch (cause) {
      const requiresSource = cause instanceof ProductionGatewayError && cause.code === 'PRODUCTION_RETRY_REQUIRES_SOURCE_WORKFLOW'
      setRequiresSourceWorkflow(requiresSource)
      setRetryError(requiresSource ? '该任务必须回到来源工作流重试，制作中心不会绕过来源约束创建任务。' : cause instanceof Error ? cause.message : '重试提交失败。')
    } finally {
      setRetrying(false)
    }
  }, [gateway, onOpenRun, projectId, ref, retrying])

  return <section className="pc-shell">
    <div className="pc-intro"><div><p>当前 Project 的模型任务、渲染任务与稳定资产血缘</p></div><span><Database size={14}/>服务端权威数据 · 自动刷新运行中任务</span></div>
    {!isAssets ? <ProductionFilters value={filters} onChange={changeFilters} onRefresh={() => setRefresh(value => value + 1)} loading={loading}/> : <div className="pc-assets-toolbar"><div><b>生产血缘素材</b><span>仅展示制作任务实际引用的输入 / 输出 AssetVersion，不是通用素材库。</span></div><button className="pc-refresh" onClick={() => setRefresh(value => value + 1)} disabled={loading}><RefreshCw size={15}/>{loading ? '刷新中' : '刷新'}</button></div>}
    {unavailable.length ? <div className="pc-health-warning" role="status"><AlertTriangle size={17}/><div><b>部分来源暂不可用</b><span>{unavailable.map(item => `${item.source}${item.message ? `：${item.message}` : ''}`).join('；')}。当前结果仍来自其余可用权威来源。</span></div></div> : null}
    <div className={`pc-content${ref ? ' has-drawer' : ''}`}>
      <div className="pc-list-panel">
        {loading && !runs && !assets ? <div className="pc-state">正在读取制作中心数据…</div>
          : error ? <div className="pc-state error"><AlertTriangle size={19}/><b>无法读取制作中心</b><span>{error}</span><button onClick={() => setRefresh(value => value + 1)}>重新加载</button></div>
            : items.length === 0 ? <div className="pc-state"><Database size={22}/><b>{isAssets ? '暂无生产血缘素材' : `${activeView}暂无任务`}</b><span>{isAssets ? '当制作任务引用或产出稳定 AssetVersion 后，会出现在这里。' : '这里不会用示例任务冒充服务端记录。'}</span></div>
              : isAssets ? <ProductionAssetList items={assets?.items ?? []} onOpenRun={onOpenRun}/>
                : <ProductionRunTable items={runs?.items ?? []} selected={ref} onSelect={onOpenRun} onOpenSource={openSource}/>}
        <div className="pc-pagination"><span>本页 {items.length} 条</span><div><button disabled={!cursorHistory.length || loading} onClick={() => { const history = [...cursorHistory]; setCursor(history.pop() ?? ''); setCursorHistory(history) }}>上一页</button><button disabled={!nextCursor || loading} onClick={() => { setCursorHistory(history => [...history, cursor]); setCursor(nextCursor ?? '') }}>下一页</button></div></div>
      </div>
      {ref ? <ProductionRunDrawer detail={detail} loading={detailLoading} error={detailError} retrying={retrying} retryError={retryError} requiresSourceWorkflow={requiresSourceWorkflow} onRetry={() => void retryRun()} onClose={onCloseRun} onOpenSource={() => detailRun && openSource(detailRun)}/> : null}
    </div>
  </section>
}

export default ProductionCenterPage
