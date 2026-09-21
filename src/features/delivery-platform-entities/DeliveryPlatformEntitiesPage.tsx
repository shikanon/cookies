import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CircleAlert, CircleCheck, Database, RefreshCw } from 'lucide-react'
import { deliveryExecutionApi, type DeliveryPlatformEntityMapping } from '../../api/delivery'
import { api, type ApiConnectorAccount, type ApiConnectorObjectSnapshot } from '../../data/api'
import { PlatformEntityEditor } from './PlatformEntityEditor'
import './delivery-platform-entities.css'

type Props = { projectId: string; activeView: string }
type PageState = {
  accounts: ApiConnectorAccount[]
  selectedAccountId: string
  objects: ApiConnectorObjectSnapshot[]
  mappings: DeliveryPlatformEntityMapping[]
  mappingByPlatformRef: Map<string, DeliveryPlatformEntityMapping>
}

export function DeliveryPlatformEntitiesPage({ projectId, activeView }: Props) {
  const pageSize = 50
  const [state, setState] = useState<PageState>()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [editing, setEditing] = useState<DeliveryPlatformEntityMapping>()
  const [page, setPage] = useState(0)
  const selectedAccountRef = useRef('')
  const accountRequestRef = useRef(0)

  const loadAccount = useCallback(async (accountId: string, accounts?: ApiConnectorAccount[]) => {
    const request = ++accountRequestRef.current
    if (selectedAccountRef.current && selectedAccountRef.current !== accountId) setEditing(undefined)
    selectedAccountRef.current = accountId
    setError('')
    try {
      const [snapshot, mappings] = await Promise.all([
        api.getProjectConnectorSnapshot(projectId, accountId, new Date(Date.now() + 60_000).toISOString()),
        deliveryExecutionApi.listPlatformEntityMappings(projectId, accountId),
      ])
      const mappingEntries = await Promise.all(mappings
        .filter(item => item.platform_object_id)
        .map(async item => [await opaquePlatformRef(item.platform_object_id), item] as const))
      if (request !== accountRequestRef.current) return
      setState(current => ({
        accounts: accounts ?? current?.accounts ?? [],
        selectedAccountId: accountId,
        objects: latestPlatformEntities(snapshot.objects),
        mappings,
        mappingByPlatformRef: new Map(mappingEntries),
      }))
    } catch (cause) {
      if (request === accountRequestRef.current) setError(cause instanceof Error ? cause.message : '读取项目与单元失败。')
    }
  }, [projectId])

  const load = useCallback(async () => {
    setError('')
    try {
      const response = await api.listProjectConnectorAccounts(projectId)
      const accounts = response.items.filter(item => item.status !== 'revoked')
      if (!accounts.length) {
        setState({ accounts: [], selectedAccountId: '', objects: [], mappings: [], mappingByPlatformRef: new Map() })
        return
      }
      const selected = selectedAccountRef.current && accounts.some(item => item.id === selectedAccountRef.current)
        ? selectedAccountRef.current
        : accounts[0].id
      await loadAccount(selected, accounts)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '读取项目与单元失败。')
    }
  }, [loadAccount, projectId])

  useEffect(() => { selectedAccountRef.current = ''; void load(); return () => { accountRequestRef.current++ } }, [load])

  const sync = useCallback(async () => {
    if (!state?.selectedAccountId) return
    setBusy(true)
    setNotice('正在同步账号项目和单元。')
    try {
      const end = new Date()
      const start = new Date(end.getTime() - 24 * 60 * 60 * 1_000)
      const result = await api.syncProjectConnectorAccount(projectId, state.selectedAccountId, {
        start: start.toISOString(), end: end.toISOString(), time_zone: 'Asia/Shanghai', currency: 'CNY', sync_mode: 'inventory_only',
      }, `platform-entity-inventory-${state.selectedAccountId}-${crypto.randomUUID()}`)
      for (let attempt = 0; attempt < 150; attempt += 1) {
        await wait(2_000)
        const status = await api.getProjectConnectorSync(projectId, state.selectedAccountId, result.run_id)
        if (status.status === 'failed') throw new Error(`同步失败。最后阶段：${status.cursor || '未知'}`)
        if (status.status === 'completed') {
          await loadAccount(state.selectedAccountId)
          setNotice('账号项目和单元已同步。绑定状态已重新匹配。')
          return
        }
        setNotice(`正在同步。当前阶段：${status.cursor || '准备读取'}`)
      }
      setNotice('同步仍在后台运行。请稍后刷新。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '同步失败。')
    } finally {
      setBusy(false)
    }
  }, [loadAccount, projectId, state])

  const visibleObjects = useMemo(() => filterObjects(state?.objects ?? [], state?.mappingByPlatformRef ?? new Map(), activeView), [activeView, state])
  const pageCount = Math.max(1, Math.ceil(visibleObjects.length / pageSize))
  const currentPage = Math.min(page, pageCount - 1)
  const pagedObjects = visibleObjects.slice(currentPage * pageSize, (currentPage + 1) * pageSize)
  useEffect(() => { setPage(0) }, [activeView, state?.selectedAccountId])
  if (error) return <PageState title="项目与单元读取失败" detail={error} onRetry={() => void load()} />
  if (!state) return <PageState title="正在读取项目与单元" detail="正在读取平台对象和 Cookies 绑定。" />
  if (!state.accounts.length) return <PageState title="没有可用投放账号" detail="请先在账户与环境页登记投放账号。" />

  return <section className="platform-entity-page" aria-label="项目与单元管理">
    <header className="platform-entity-header">
      <div><span className="section-label">平台对象</span><h2>项目与单元</h2><p>查看账号中的投放项目、推广单元和 Cookies 绑定状态。</p></div>
      <div className="platform-entity-actions">
        <select value={state.selectedAccountId} onChange={event => void loadAccount(event.target.value)} aria-label="投放账号">
          {state.accounts.map(account => <option key={account.id} value={account.id}>{account.display_label || account.id}</option>)}
        </select>
        <button className="secondary-button" disabled={busy} onClick={() => void loadAccount(state.selectedAccountId)}><RefreshCw size={14}/>刷新</button>
        <button className="primary-button" disabled={busy} onClick={() => void sync()}><Database size={14}/>同步账号项目与单元</button>
      </div>
    </header>
    {editing ? <PlatformEntityEditor key={editing.id} projectId={projectId} mapping={editing} onClose={() => setEditing(undefined)}/> : null}
    {notice ? <div className="platform-entity-notice" role="status">{notice}</div> : null}
    <div className="platform-entity-summary">
      <article><b>{state.objects.filter(item => item.object_kind === 'project').length}</b><span>同步项目</span></article>
      <article><b>{state.objects.filter(item => item.object_kind === 'promotion').length}</b><span>同步单元</span></article>
      <article><b>{state.mappings.filter(item => item.status === 'confirmed').length}</b><span>Cookies 已绑定</span></article>
      <article><b>{state.mappings.filter(item => item.status !== 'confirmed').length}</b><span>待确认绑定</span></article>
    </div>
    {visibleObjects.length ? <><div className="platform-entity-table" role="table">
      <div className="heading" role="row"><span>平台对象</span><span>同步状态</span><span>Cookies 绑定</span><span>操作</span></div>
      {pagedObjects.map((item, index) => {
        const mapping = state.mappingByPlatformRef.get(item.object_ref)
        return <div key={`${item.object_kind}:${item.object_ref}`} role="row">
          <span><b>{entityName(item, currentPage * pageSize + index + 1)}</b><small>{item.object_kind === 'project' ? '项目' : '单元'}</small></span>
          <span>{entityStatus(item)}</span>
          <span className={mapping?.status === 'confirmed' ? 'bound' : 'unbound'}>{mapping?.status === 'confirmed' ? <CircleCheck size={14}/> : <CircleAlert size={14}/>}<span>{mapping ? `${mapping.internal_object_kind === 'project' ? '项目' : '单元'} · ${mapping.status === 'confirmed' ? '已确认' : '待确认'}` : '未绑定 Cookies 对象'}</span></span>
          <span>{mapping ? <button className="secondary-button" type="button" onClick={() => setEditing(mapping)}>编辑绑定</button> : <small>平台账号同步</small>}</span>
        </div>
      })}
    </div><nav className="platform-entity-pagination" aria-label="项目与单元分页"><span>第 {currentPage + 1} / {pageCount} 页，共 {visibleObjects.length} 个对象</span><div><button className="secondary-button" type="button" disabled={currentPage === 0} onClick={() => setPage(value => Math.max(0, value - 1))}>上一页</button><button className="secondary-button" type="button" disabled={currentPage >= pageCount - 1} onClick={() => setPage(value => Math.min(pageCount - 1, value + 1))}>下一页</button></div></nav></> : <PageState title="当前筛选没有对象" detail="运行账号同步，或切换到其他视图。" />}
    {state.mappings.length ? <details className="platform-entity-mappings">
      <summary>绑定技术详情（{state.mappings.length}）</summary>
      <div className="platform-entity-table" role="table">
        <div className="heading" role="row"><span>Cookies 对象</span><span>巨量对象</span><span>绑定状态</span><span>来源 Run</span></div>
        {state.mappings.map(mapping => <div key={mapping.id} role="row">
          <span><b>{mapping.internal_object_kind === 'project' ? '项目' : '单元'}</b><small>{mapping.internal_object_id}</small></span>
          <span><b>{mapping.platform_object_id || '尚未回写 ID'}</b><small>{mapping.platform_status || mapping.platform_object_kind}</small></span>
          <span className={mapping.status === 'confirmed' ? 'bound' : 'unbound'}>{mapping.status === 'confirmed' ? <CircleCheck size={14}/> : <CircleAlert size={14}/>}<span>{mapping.status === 'confirmed' ? '已确认' : '待确认'}</span></span>
          <span><code title={mapping.browser_rpa_run_id}>{shortRef(mapping.browser_rpa_run_id)}</code><small>更新于 {formatTime(mapping.updated_at)}</small><button className="secondary-button" type="button" onClick={() => setEditing(mapping)}>编辑{mapping.internal_object_kind === 'project' ? '项目' : '单元'}</button></span>
        </div>)}
      </div>
    </details> : null}
  </section>
}

function latestPlatformEntities(values: ApiConnectorObjectSnapshot[]): ApiConnectorObjectSnapshot[] {
  const latest = new Map<string, ApiConnectorObjectSnapshot>()
  for (const value of values) {
    if (value.object_kind !== 'project' && value.object_kind !== 'promotion') continue
    const key = `${value.object_kind}:${value.object_ref}`
    const current = latest.get(key)
    if (!current || current.available_at < value.available_at) latest.set(key, value)
  }
  return [...latest.values()].sort((left, right) => left.object_kind.localeCompare(right.object_kind) || entityName(left).localeCompare(entityName(right), 'zh-CN'))
}

function filterObjects(values: ApiConnectorObjectSnapshot[], mappings: Map<string, DeliveryPlatformEntityMapping>, view: string) {
  if (view === '项目') return values.filter(item => item.object_kind === 'project')
  if (view === '单元') return values.filter(item => item.object_kind === 'promotion')
  if (view === '未绑定') return values.filter(item => !mappings.has(item.object_ref))
  return values
}

function entityName(value: ApiConnectorObjectSnapshot, fallbackIndex?: number): string {
  for (const key of ['name', 'promotion_name', 'project_name', 'ad_name', 'title']) {
    const candidate = value.state[key]
    if (typeof candidate === 'string' && candidate.trim()) return candidate.trim()
  }
  const label = value.object_kind === 'project' ? '未命名项目' : '未命名单元'
  return fallbackIndex ? `${label} ${fallbackIndex}` : label
}

function entityStatus(value: ApiConnectorObjectSnapshot): string {
  const status = value.state.status ?? value.state.delivery_status ?? value.quality_status
  if (typeof status !== 'string') return '已同步'
  const labels: Record<string, string> = {
    accept: '可用',
    active: '启用中',
    enabled: '已启用',
    delivering: '投放中',
    paused: '已暂停',
    disabled: '已停用',
    deleted: '已删除',
  }
  return labels[status.toLowerCase()] ?? '已同步'
}

async function opaquePlatformRef(platformObjectId: string): Promise<string> {
  const payload = new TextEncoder().encode(JSON.stringify(platformObjectId))
  const digest = await crypto.subtle.digest('SHA-256', payload)
  return `ref_${[...new Uint8Array(digest)].map(value => value.toString(16).padStart(2, '0')).join('')}`
}

function shortRef(value: string): string { return value.length > 20 ? `${value.slice(0, 10)}…${value.slice(-6)}` : value }
function formatTime(value: string): string { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false }) }
function wait(ms: number) { return new Promise(resolve => window.setTimeout(resolve, ms)) }
function PageState({ title, detail, onRetry }: { title: string; detail: string; onRetry?: () => void }) { return <div className="platform-entity-state"><h2>{title}</h2><p>{detail}</p>{onRetry ? <button className="secondary-button" onClick={onRetry}>重试</button> : null}</div> }
