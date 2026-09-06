import { useCallback, useEffect, useMemo, useState } from 'react'
import { CircleAlert, CircleCheck, Database, RefreshCw } from 'lucide-react'
import { deliveryExecutionApi, type DeliveryPlatformEntityMapping } from '../../api/delivery'
import { api, type ApiConnectorAccount, type ApiConnectorObjectSnapshot } from '../../data/api'
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
  const [state, setState] = useState<PageState>()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')

  const loadAccount = useCallback(async (accountId: string, accounts?: ApiConnectorAccount[]) => {
    const [snapshot, mappings] = await Promise.all([
      api.getProjectConnectorSnapshot(projectId, accountId, new Date(Date.now() + 60_000).toISOString()),
      deliveryExecutionApi.listPlatformEntityMappings(projectId, accountId),
    ])
    const mappingEntries = await Promise.all(mappings
      .filter(item => item.platform_object_id)
      .map(async item => [await opaquePlatformRef(item.platform_object_id), item] as const))
    setState(current => ({
      accounts: accounts ?? current?.accounts ?? [],
      selectedAccountId: accountId,
      objects: latestPlatformEntities(snapshot.objects),
      mappings,
      mappingByPlatformRef: new Map(mappingEntries),
    }))
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
      const selected = state?.selectedAccountId && accounts.some(item => item.id === state.selectedAccountId)
        ? state.selectedAccountId
        : accounts[0].id
      await loadAccount(selected, accounts)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '读取项目与单元失败。')
    }
  }, [loadAccount, projectId, state?.selectedAccountId])

  useEffect(() => { void load() }, [load])

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
  if (error) return <PageState title="项目与单元读取失败" detail={error} onRetry={() => void load()} />
  if (!state) return <PageState title="正在读取项目与单元" detail="正在并行读取 Connector 快照和 Cookies 绑定。" />
  if (!state.accounts.length) return <PageState title="没有可用投放账号" detail="请先在账户与环境页登记投放账号。" />

  return <section className="platform-entity-page" aria-label="项目与单元管理">
    <header className="platform-entity-header">
      <div><span className="section-label">Ocean Engine inventory</span><h2>项目与单元</h2><p>统一查看账号同步对象、Cookies 绑定、巨量 ID 和来源 Run。</p></div>
      <div className="platform-entity-actions">
        <select value={state.selectedAccountId} onChange={event => void loadAccount(event.target.value)} aria-label="投放账号">
          {state.accounts.map(account => <option key={account.id} value={account.id}>{account.display_label || account.id}</option>)}
        </select>
        <button className="secondary-button" disabled={busy} onClick={() => void loadAccount(state.selectedAccountId)}><RefreshCw size={14}/>刷新</button>
        <button className="primary-button" disabled={busy} onClick={() => void sync()}><Database size={14}/>同步账号项目与单元</button>
      </div>
    </header>
    {notice ? <div className="platform-entity-notice" role="status">{notice}</div> : null}
    <div className="platform-entity-summary">
      <article><b>{state.objects.filter(item => item.object_kind === 'project').length}</b><span>同步项目</span></article>
      <article><b>{state.objects.filter(item => item.object_kind === 'promotion').length}</b><span>同步单元</span></article>
      <article><b>{state.mappings.filter(item => item.status === 'confirmed').length}</b><span>Cookies 已绑定</span></article>
      <article><b>{state.mappings.filter(item => item.status !== 'confirmed').length}</b><span>待确认绑定</span></article>
    </div>
    {visibleObjects.length ? <div className="platform-entity-table" role="table">
      <div className="heading" role="row"><span>平台对象</span><span>平台引用</span><span>Cookies 绑定</span><span>来源</span></div>
      {visibleObjects.map(item => {
        const mapping = state.mappingByPlatformRef.get(item.object_ref)
        return <div key={`${item.object_kind}:${item.object_ref}`} role="row">
          <span><b>{entityName(item)}</b><small>{item.object_kind === 'project' ? '项目' : '单元'} · {entityStatus(item)}</small></span>
          <code title={item.object_ref}>{shortRef(item.object_ref)}</code>
          <span className={mapping?.status === 'confirmed' ? 'bound' : 'unbound'}>{mapping?.status === 'confirmed' ? <CircleCheck size={14}/> : <CircleAlert size={14}/>}<span>{mapping ? `${mapping.internal_object_kind} · ${mapping.internal_object_id}` : '未绑定 Cookies 对象'}</span></span>
          <span>{mapping ? <><code title={mapping.platform_object_id}>{mapping.platform_object_id}</code><small>Run {shortRef(mapping.browser_rpa_run_id)}</small></> : <small>来自 Connector 账号同步</small>}</span>
        </div>
      })}
    </div> : <PageState title="当前筛选没有对象" detail="运行账号同步，或切换到其他视图。" />}
    {state.mappings.length ? <section className="platform-entity-mappings">
      <header><div><span className="section-label">Cookies bindings</span><h3>Cookies 绑定记录</h3></div><small>此列表不依赖 Connector 同步结果。</small></header>
      <div className="platform-entity-table" role="table">
        <div className="heading" role="row"><span>Cookies 对象</span><span>巨量对象</span><span>绑定状态</span><span>来源 Run</span></div>
        {state.mappings.map(mapping => <div key={mapping.id} role="row">
          <span><b>{mapping.internal_object_kind === 'project' ? '项目' : '单元'}</b><small>{mapping.internal_object_id}</small></span>
          <span><b>{mapping.platform_object_id || '尚未回写 ID'}</b><small>{mapping.platform_status || mapping.platform_object_kind}</small></span>
          <span className={mapping.status === 'confirmed' ? 'bound' : 'unbound'}>{mapping.status === 'confirmed' ? <CircleCheck size={14}/> : <CircleAlert size={14}/>}<span>{mapping.status === 'confirmed' ? '已确认' : '待确认'}</span></span>
          <span><code title={mapping.browser_rpa_run_id}>{shortRef(mapping.browser_rpa_run_id)}</code><small>更新于 {formatTime(mapping.updated_at)}</small></span>
        </div>)}
      </div>
    </section> : null}
    {state.mappings.some(mapping => !mapping.platform_object_id) ? <section className="platform-entity-pending"><h3>待确认绑定</h3>{state.mappings.filter(mapping => !mapping.platform_object_id).map(mapping => <div key={mapping.id}><span>{mapping.internal_object_kind} · {mapping.internal_object_id}</span><small>Run {mapping.browser_rpa_run_id} · {mapping.status}</small></div>)}</section> : null}
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

function entityName(value: ApiConnectorObjectSnapshot): string {
  for (const key of ['name', 'promotion_name', 'project_name', 'ad_name', 'title']) {
    const candidate = value.state[key]
    if (typeof candidate === 'string' && candidate.trim()) return candidate.trim()
  }
  return value.object_kind === 'project' ? `项目 ${shortRef(value.object_ref)}` : `单元 ${shortRef(value.object_ref)}`
}

function entityStatus(value: ApiConnectorObjectSnapshot): string {
  const status = value.state.status ?? value.state.delivery_status ?? value.quality_status
  return typeof status === 'string' ? status : '已同步'
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
