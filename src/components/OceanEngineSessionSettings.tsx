import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Check, CircleAlert, Database, LockKeyhole, RefreshCw, Save, Search } from 'lucide-react'
import { ApiRequestError, api, type ApiConnectorAccount, type ApiConnectorAccountSession, type ApiConnectorPlatformObject, type ApiConnectorPlatformObjectKind } from '../data/api'

type LoadState = 'loading' | 'ready' | 'error'
const formatter = new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
const formatTime = (value?: string | Date | null) => value ? formatter.format(new Date(value)) : '暂无记录'

const statusCopy: Record<ApiConnectorAccountSession['status'], { title: string; detail: string }> = {
  unverified: { title: '会话尚未验证', detail: '执行只读验证后，系统才允许同步。' },
  ready: { title: '只读连接正常', detail: '账号可以执行授权范围内的只读同步。' },
  auth_required: { title: '需要更新会话', detail: '请更新 Cookie，然后重新验证。' },
  disabled: { title: '连接已停用', detail: '当前会话不能用于同步。' },
}

const accountStatusCopy: Record<ApiConnectorAccount['status'], string> = {
  pending: '待验证',
  verified: '已验证',
  revoked: '已停用',
  blocked: '已阻止',
}

function errorMessage(error: unknown) {
  if (error instanceof ApiRequestError && error.status === 409) return '数据版本已变化。请刷新后重试。'
  if (error instanceof ApiRequestError && error.status === 422) return 'Cookie 已失效，或缺少该投放账号上下文。请从该账号成功的广告列表请求中重新复制完整 Cookie。'
  if (error instanceof ApiRequestError && error.status === 502) return '巨量引擎验证接口当前不可用。会话未改变，请稍后重试。'
  if (error instanceof ApiRequestError && error.status === 403) return '你没有管理当前 Project Connector 的权限。'
  return error instanceof Error ? error.message : '连接操作失败。请稍后重试。'
}

function syncWindow(days: number) {
  const end = new Date()
  const start = new Date(end)
  start.setUTCDate(start.getUTCDate() - days)
  return { start: start.toISOString(), end: end.toISOString() }
}

const wait = (milliseconds: number) => new Promise(resolve => setTimeout(resolve, milliseconds))
const objectKindCopy: Record<ApiConnectorPlatformObjectKind, string> = {
  image_material: '图片素材', product_image: '我的图片', video_material: '视频素材', aweme_photo_material: '抖音图文',
  marketing_product: '营销产品', orange_landing_page: '橙子落地页',
  optimization_target: '优化目标', conversion_event_asset: '转化事件资产',
  industry_category: '行业类目', brand: '品牌', authorized_identity: '授权身份',
}
function objectSyncSummary(value: Awaited<ReturnType<typeof api.syncProjectConnectorAccount>>['platform_objects']) {
  const stats = Object.values(value ?? {}).reduce((total, item) => ({
    created: total.created + (item?.created ?? 0), updated: total.updated + (item?.updated ?? 0),
    unchanged: total.unchanged + (item?.unchanged ?? 0), unavailable: total.unavailable + (item?.unavailable ?? 0),
  }), { created: 0, updated: 0, unchanged: 0, unavailable: 0 })
  return `新增 ${stats.created}，更新 ${stats.updated}，未变化 ${stats.unchanged}，失效 ${stats.unavailable}`
}

export function OceanEngineSessionSettings({ projectId }: { projectId: string }) {
  const externalIDRef = useRef<HTMLInputElement>(null)
  const labelRef = useRef<HTMLInputElement>(null)
  const sessionInputRef = useRef<HTMLInputElement>(null)
  const requestRef = useRef(0)
  const [accounts, setAccounts] = useState<ApiConnectorAccount[]>([])
  const [legacyAccounts, setLegacyAccounts] = useState<ApiConnectorAccount[]>([])
  const [selectedAccountID, setSelectedAccountID] = useState('')
  const [session, setSession] = useState<ApiConnectorAccountSession | null>(null)
  const [loadState, setLoadState] = useState<LoadState>('loading')
  const [busy, setBusy] = useState(false)
  const [lastSyncedAt, setLastSyncedAt] = useState<Date | null>(null)
  const [notice, setNotice] = useState('')
  const [platformObjects, setPlatformObjects] = useState<ApiConnectorPlatformObject[]>([])
  const [objectKind, setObjectKind] = useState<ApiConnectorPlatformObjectKind | ''>('')
  const [objectSearch, setObjectSearch] = useState('')
  const [nextObjectCursor, setNextObjectCursor] = useState('')

  const loadObjects = useCallback(async (
    accountID: string,
    cursor = '',
    kind: ApiConnectorPlatformObjectKind | '' = objectKind,
    search = objectSearch,
  ) => {
    if (!accountID) {
      setPlatformObjects([])
      setNextObjectCursor('')
      return
    }
    const response = await api.listProjectConnectorPlatformObjects(projectId, accountID, {
      objectKind: kind || undefined, status: 'active', q: search.trim() || undefined,
      cursor: cursor || undefined, limit: 100,
    })
    setPlatformObjects(current => cursor ? [...current, ...response.items] : response.items)
    setNextObjectCursor(response.next_cursor)
  }, [objectKind, objectSearch, projectId])

  const load = useCallback(async (preferredAccountID = '') => {
    const requestID = ++requestRef.current
    setLoadState('loading')
    try {
      const [response, legacyResponse] = await Promise.all([
        api.listProjectConnectorAccounts(projectId),
        api.listConnectorAccounts(),
      ])
      if (requestID !== requestRef.current) return
      setAccounts(response.items)
      setLegacyAccounts(legacyResponse.items)
      const accountID = response.items.some(item => item.id === preferredAccountID)
        ? preferredAccountID
        : response.items[0]?.id ?? ''
      setSelectedAccountID(accountID)
      if (!accountID) {
        setSession(null)
        setPlatformObjects([])
        setLoadState('ready')
        return
      }
      try {
        const [nextSession, objects] = await Promise.all([
          api.getProjectConnectorAccountSession(projectId, accountID),
          api.listProjectConnectorPlatformObjects(projectId, accountID, { status: 'active', limit: 100 }),
        ])
        if (requestID !== requestRef.current) return
        setSession(nextSession)
        setPlatformObjects(objects.items)
        setNextObjectCursor(objects.next_cursor)
      } catch (error) {
        if (!(error instanceof ApiRequestError && error.status === 404)) throw error
        setSession(null)
      }
      setLoadState('ready')
      setLastSyncedAt(new Date())
    } catch (error) {
      if (requestID !== requestRef.current) return
      setLoadState('error')
      setNotice(errorMessage(error))
    }
  }, [projectId])

  useEffect(() => { void load() }, [load])

  const selectAccount = async (accountID: string) => {
    setSelectedAccountID(accountID)
    setNotice('')
    if (!accountID) return setSession(null)
    setLoadState('loading')
    try {
      const [nextSession, objects] = await Promise.all([
        api.getProjectConnectorAccountSession(projectId, accountID),
        api.listProjectConnectorPlatformObjects(projectId, accountID, { status: 'active', limit: 100 }),
      ])
      setSession(nextSession)
      setPlatformObjects(objects.items)
      setNextObjectCursor(objects.next_cursor)
    } catch (error) {
      if (error instanceof ApiRequestError && error.status === 404) setSession(null)
      else setNotice(errorMessage(error))
    } finally {
      setLoadState('ready')
    }
  }

  const register = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const externalID = externalIDRef.current?.value.trim() ?? ''
    const displayLabel = labelRef.current?.value.trim() ?? ''
    if (!externalID) return setNotice('请输入投放账号 ID。')
    setBusy(true); setNotice('')
    try {
      const account = await api.registerProjectConnectorAccount(projectId, { external_id: externalID, display_label: displayLabel })
      if (externalIDRef.current) externalIDRef.current.value = ''
      if (labelRef.current) labelRef.current.value = ''
      await load(account.id)
      setNotice('账号已登记。原始账号 ID 不会在页面回显。请继续保存只读会话。')
    } catch (error) {
      setNotice(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const saveSession = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const value = sessionInputRef.current?.value.trim() ?? ''
    if (!selectedAccountID) return setNotice('请先登记投放账号。')
    if (value.length < 8) return setNotice('请粘贴完整 Cookie 值。')
    if (value.length > 16384) return setNotice('Cookie 超过 16 KiB。请确认只复制 Cookie 值。')
    setBusy(true); setNotice('')
    try {
      const next = await api.updateProjectConnectorAccountSession(projectId, selectedAccountID, { session: value, expected_version: session?.version ?? 0 })
      if (sessionInputRef.current) sessionInputRef.current.value = ''
      setSession(next)
      setNotice('会话已加密保存，输入框已清空。请执行只读验证。')
    } catch (error) {
      setNotice(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const verify = async () => {
    if (!selectedAccountID || !session) return
    setBusy(true); setNotice('')
    try {
      await api.verifyProjectConnectorAccount(projectId, selectedAccountID)
      const next = await api.getProjectConnectorAccountSession(projectId, selectedAccountID)
      setSession(next)
      setAccounts(current => current.map(account => account.id === selectedAccountID ? { ...account, status: 'verified', verified_at: next.last_verified_at } : account))
      setNotice('只读验证通过。现在可以同步历史数据。')
    } catch (error) {
      setNotice(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const runSync = async (days: number, mode: 'full' | 'metrics_only' | 'inventory_only') => {
    if (!selectedAccountID || session?.status !== 'ready') return
    setBusy(true); setNotice('')
    try {
      const window = syncWindow(days)
      const operation = mode === 'full' ? '历史同步' : mode === 'inventory_only' ? '巨量对象目录同步' : days > 30 ? '历史日级补数' : '指标巡检'
      const idempotencyKey = `${projectId}-${mode}-${selectedAccountID}-${crypto.randomUUID()}`
      const result = await api.syncProjectConnectorAccount(projectId, selectedAccountID, { ...window, time_zone: 'Asia/Shanghai', currency: 'CNY', sync_mode: mode }, idempotencyKey)
      setNotice(`${operation}已进入后台。页面将持续读取同步状态。`)
      for (let attempt = 0; attempt < 1_350; attempt += 1) {
        await wait(2_000)
        try {
          const status = await api.getProjectConnectorSync(projectId, selectedAccountID, result.run_id)
          if (status.status === 'completed') {
            if (mode !== 'metrics_only') await loadObjects(selectedAccountID)
            const summary = objectSyncSummary(result.platform_objects)
            setNotice(mode === 'full' ? `历史同步已完成。对象目录、对象快照和指标窗口已经写入 Connector。${summary}。` : mode === 'inventory_only' ? `巨量对象目录同步已完成。当前 Project 已自动获得对象使用权限。${summary}。` : `${operation}已完成。指标窗口和转换修订已经写入 Connector。`)
            return
          }
          if (status.status === 'failed') {
            setNotice(`${operation}失败。最后阶段：${status.cursor || '尚未取得平台数据'}。`)
            return
          }
          setNotice(`${operation}正在后台运行。当前阶段：${status.cursor || '准备平台读取'}。`)
        } catch (error) {
          if (!(error instanceof ApiRequestError && error.status === 404)) throw error
        }
      }
      setNotice(`${operation}仍在后台运行。请稍后刷新页面查看状态。`)
    } catch (error) {
      setNotice(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const claimAccount = async (accountID: string) => {
    setBusy(true); setNotice('')
    try {
      const account = await api.claimProjectConnectorAccount(projectId, accountID)
      await load(account.id)
      setNotice('账号已绑定当前 Project。组织会话、同步记录和校准结果保持不变。')
    } catch (error) {
      setNotice(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const selectedAccount = accounts.find(account => account.id === selectedAccountID)
  const copy = session ? statusCopy[session.status] : { title: '尚未保存会话', detail: 'Cookie 会话归属当前组织账号。' }
  return <section className="miyun-connection-settings" aria-labelledby="ocean-engine-session-title">
    <div className="miyun-settings-main">
      <header><div><span>组织会话 · Project 绑定</span><h2 id="ocean-engine-session-title">巨量投放账号</h2><p>Plan 可选择已绑定当前 Project 的验证账号。</p></div><div className="miyun-settings-status-group" aria-live="polite"><span className={`miyun-connection-status ${session?.status === 'ready' ? 'ready' : ''}`}>{session?.status === 'ready' ? <Check size={14} aria-hidden="true" /> : <CircleAlert size={14} aria-hidden="true" />}{loadState === 'loading' ? '正在读取…' : copy.title}</span><small>页面同步于 {formatTime(lastSyncedAt)}</small></div></header>
      <div className="miyun-settings-secret-policy"><LockKeyhole size={18} aria-hidden="true" /><p><b>账号和凭据分开保存</b>{copy.detail} 原始账号 ID 和 Cookie 都不会进入校准导出。</p></div>

      <div className="oe-connector-flow">
        <section className="oe-settings-card" aria-labelledby="oe-account-section-title">
          <header className="oe-settings-card-header"><span>01</span><div><h3 id="oe-account-section-title">投放账号</h3><p>选择已有账号，或登记另一个账号。</p></div>{selectedAccount ? <strong className={`oe-account-state ${selectedAccount.status}`}>{accountStatusCopy[selectedAccount.status]}</strong> : null}</header>
          {accounts.length > 0 ? <div className="oe-account-picker">
            <label htmlFor="ocean-engine-local-account">当前账号</label>
            <select id="ocean-engine-local-account" name="ocean_engine_local_account" value={selectedAccountID} onChange={event => void selectAccount(event.target.value)} disabled={busy}>
              {accounts.map(account => <option key={account.id} value={account.id}>{account.display_label || '未命名账号'} · {accountStatusCopy[account.status]}</option>)}
            </select>
            <div className="oe-account-reference"><span>本地账号引用</span><code translate="no" title={selectedAccount?.id}>{selectedAccount?.id ?? '无'}</code></div>
          </div> : <div className="oe-account-empty"><CircleAlert size={18} aria-hidden="true" /><p><b>尚未登记账号</b><span>填写下方信息后，系统会创建匿名本地引用。</span></p></div>}
          <form className="oe-register-form" onSubmit={register}>
            <div className="oe-form-heading"><b>{accounts.length > 0 ? '登记其他账号' : '登记第一个账号'}</b></div>
            <div className="oe-register-fields"><label htmlFor="ocean-engine-account-id">投放账号 ID<input id="ocean-engine-account-id" name="ocean_engine_account_id" ref={externalIDRef} type="password" autoComplete="off" spellCheck={false} placeholder="仅用于登记，保存后不回显…" /></label><label htmlFor="ocean-engine-account-label">本地显示名称<input id="ocean-engine-account-label" name="ocean_engine_account_label" ref={labelRef} autoComplete="off" maxLength={255} placeholder="例如：历史校准账号…" /></label></div>
            <button className="secondary-button" type="submit" disabled={busy}><Save size={15} aria-hidden="true" />登记账号</button>
          </form>
          {legacyAccounts.length ? <div className="oe-account-empty"><CircleAlert size={18} aria-hidden="true" /><p><b>可绑定的组织账号</b><span>绑定会复用组织会话，并保留同步记录和校准结果。</span></p>{legacyAccounts.map(account => <button key={account.id} className="secondary-button" type="button" disabled={busy} onClick={() => void claimAccount(account.id)}>绑定 {account.display_label || '未命名账号'}</button>)}</div> : null}
        </section>

        <section className={`oe-settings-card ${selectedAccountID ? '' : 'disabled'}`} aria-labelledby="oe-session-section-title">
          <header className="oe-settings-card-header"><span>02</span><div><h3 id="oe-session-section-title">只读会话</h3><p>保存 Cookie，并验证账号读取权限。</p></div><LockKeyhole size={18} aria-hidden="true" /></header>
          <form className="miyun-session-form" onSubmit={saveSession}>
            <label htmlFor="ocean-engine-account-session">巨量引擎 Cookie 请求头</label>
            <input id="ocean-engine-account-session" name="ocean_engine_cookie" ref={sessionInputRef} type="password" autoComplete="off" spellCheck={false} placeholder={selectedAccountID ? '粘贴完整 Cookie 值，仅用于本次保存…' : '请先登记投放账号…'} disabled={!selectedAccountID || busy} />
            <small>Cookie 属于组织登录会话。可复用该组织内成功请求的完整 Cookie。保存后，系统立即清空输入框。</small>
            <dl className="miyun-connection-metadata"><div><dt>会话状态</dt><dd>{session ? statusCopy[session.status].title : '尚未保存'}</dd></div><div><dt>最近验证</dt><dd>{formatTime(session?.last_verified_at)}</dd></div></dl>
            <div className="miyun-settings-actions"><button className="secondary-button" type="button" onClick={() => void load(selectedAccountID)} disabled={busy || !selectedAccountID}><RefreshCw size={15} aria-hidden="true" />刷新状态</button><button className="secondary-button" type="button" onClick={() => void verify()} disabled={busy || !session}><Check size={15} aria-hidden="true" />只读验证</button><button className="primary-button" type="submit" disabled={busy || !selectedAccountID}><Save size={15} aria-hidden="true" />保存会话</button></div>
          </form>
        </section>

        <section className={`oe-sync-card ${session?.status === 'ready' ? 'ready' : ''}`} aria-labelledby="oe-sync-section-title">
          <div className="oe-sync-icon"><Database size={20} aria-hidden="true" /></div><div className="oe-sync-copy"><span>03 · 数据读取</span><h3 id="oe-sync-section-title">历史同步与每日巡检</h3><p>{session?.status === 'ready' ? '对象目录读取素材、产品、落地页、优化目标、类目、品牌和授权身份。日级补数只读取指标。' : '完成只读验证后，系统才允许读取数据。'}</p><div className="oe-sync-facts"><span>只读请求</span><span>服务端分页</span><span>转换修订</span></div></div><div className="oe-sync-actions"><button className="secondary-button" type="button" onClick={() => void runSync(180, 'inventory_only')} disabled={busy || session?.status !== 'ready'}><Database size={15} aria-hidden="true" />同步巨量对象目录</button><button className="secondary-button" type="button" onClick={() => void runSync(14, 'metrics_only')} disabled={busy || session?.status !== 'ready'}><RefreshCw size={15} aria-hidden="true" />巡检最近 14 天</button><button className="secondary-button" type="button" onClick={() => void runSync(180, 'metrics_only')} disabled={busy || session?.status !== 'ready'}><Database size={15} aria-hidden="true" />补齐近 180 天日级指标</button><button className="primary-button" type="button" onClick={() => void runSync(180, 'full')} disabled={busy || session?.status !== 'ready'}><Database size={15} aria-hidden="true" />同步最近 180 天</button></div>
        </section>

        <section className="oe-settings-card oe-object-catalog" aria-labelledby="oe-object-catalog-title">
          <header className="oe-settings-card-header"><span>04</span><div><h3 id="oe-object-catalog-title">已导入巨量对象</h3><p>这里只显示当前 Project 已获授权的可用对象。</p></div><Database size={18} aria-hidden="true" /></header>
          <div className="oe-object-counts" aria-label="对象类型筛选">
            {(Object.keys(objectKindCopy) as ApiConnectorPlatformObjectKind[]).map(kind => <button key={kind} type="button" className={objectKind === kind ? 'active' : ''} onClick={() => {
              const nextKind = objectKind === kind ? '' : kind
              setObjectKind(nextKind)
              void loadObjects(selectedAccountID, '', nextKind, objectSearch)
            }} disabled={!selectedAccountID || busy}><span>{objectKindCopy[kind]}</span></button>)}
            <small>当前结果 {platformObjects.length} 个</small>
          </div>
          <form className="oe-object-search" onSubmit={event => { event.preventDefault(); void loadObjects(selectedAccountID) }}>
            <label htmlFor="oe-object-search">搜索对象<input id="oe-object-search" value={objectSearch} onChange={event => setObjectSearch(event.target.value)} placeholder="输入对象名称" disabled={!selectedAccountID || busy} /></label>
            <button className="secondary-button" type="submit" disabled={!selectedAccountID || busy}><Search size={15} aria-hidden="true" />搜索</button>
          </form>
          <div className="oe-object-list" aria-live="polite">
            {platformObjects.length ? platformObjects.map(value => <article key={value.id}>{value.preview_url ? value.preview_kind === 'landing_page' ? <a className="oe-object-preview oe-object-preview--link" href={value.preview_url} target="_blank" rel="noreferrer">预览</a> : <a className="oe-object-preview" href={value.preview_url} target="_blank" rel="noreferrer"><img src={value.preview_url} alt={`${value.display_name || objectKindCopy[value.object_kind]}预览`} loading="lazy" /></a> : <span className="oe-object-preview oe-object-preview--empty">无预览</span>}<div><b>{value.display_name || objectKindCopy[value.object_kind]}</b><span>{objectKindCopy[value.object_kind]} · 最近观察 {formatTime(value.observed_at)}</span></div><code title={value.platform_object_id}>{value.platform_object_id}</code></article>) : <div className="oe-account-empty"><CircleAlert size={18} aria-hidden="true" /><p><b>尚无可用对象</b><span>先完成只读验证，再同步巨量对象目录。</span></p></div>}
          </div>
          {nextObjectCursor ? <button className="secondary-button" type="button" disabled={busy} onClick={() => void loadObjects(selectedAccountID, nextObjectCursor)}>加载更多对象</button> : null}
        </section>
      </div>
      {notice ? <p className="miyun-settings-notice" role="status" aria-live="polite">{notice}</p> : null}
    </div>
    <aside className="miyun-cookie-guide"><h3>安全边界</h3><ol><li><span>01</span><p>Cookie 会话归属当前组织账号。</p></li><li><span>02</span><p>Plan 只能选择已绑定当前 Project 的账号。</p></li><li><span>03</span><p>Cookie 只提交到服务端加密存储。</p></li><li><span>04</span><p>验证和同步只使用读取请求。</p></li></ol></aside>
  </section>
}
