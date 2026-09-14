import { useEffect, useRef, useState } from 'react'
import { deliveryFillingApi, type FillingAcceptance, type FillingField, type FillingRequest, type FillingResult, type FillingStrategy } from '../api/deliveryFilling'
import { acceptedFilling, fillingContextKey, fillingFieldDisplay, fillingLabels, isEmptyFillingValue } from '../lib/deliveryFilling'

export function DeliveryFillingPanel({ projectId, request, disabled, history = [], onApply }: {
  projectId: string; request: FillingRequest; disabled?: boolean; history?: FillingAcceptance[]; onApply: (acceptance: FillingAcceptance) => void;
}) {
  const [open, setOpen] = useState(false)
  const [strategy, setStrategy] = useState<FillingStrategy>()
  const [strategies, setStrategies] = useState<Array<FillingStrategy & { label: string }>>([])
  const [strategyError, setStrategyError] = useState('')
  const [search, setSearch] = useState('')
  const [retry, setRetry] = useState(0)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [response, setResponse] = useState<{ result: FillingResult; request: FillingRequest }>()
  const [selected, setSelected] = useState<FillingField[]>([])
  const generation = useRef(0)
  const latest = useRef<FillingRequest>({ ...request, strategy, search })
  latest.current = { ...request, strategy: strategy ? { package_id: strategy.package_id, version: strategy.version, content_hash: strategy.content_hash } : undefined, search }
  const key = `${projectId}:${fillingContextKey(latest.current)}`
  useEffect(() => { generation.current += 1; setBusy(false); setResponse(undefined); setSelected([]) }, [key])
  useEffect(() => { setStrategy(undefined); setStrategies([]) }, [projectId])
  useEffect(() => {
    if (!open) return
    let active = true
    setStrategyError('')
    void deliveryFillingApi.strategies(projectId).then(items => { if (active) setStrategies(items) }).catch(error => {
      if (active) setStrategyError(error instanceof Error ? error.message : '策略读取失败。')
    })
    return () => { active = false }
  }, [projectId, open, retry])
  useEffect(() => () => { generation.current += 1 }, [])

  const generate = async () => {
    const id = ++generation.current
    const snapshot = structuredClone(latest.current)
    setBusy(true); setNotice(''); setResponse(undefined)
    try {
      const result = await deliveryFillingApi.suggest(projectId, snapshot)
      if (id !== generation.current || fillingContextKey(snapshot) !== fillingContextKey(latest.current)) return
      setResponse({ result, request: snapshot })
      setSelected(result.suggestions.filter(item => !['total_budget', 'daily_budget', 'bid', 'schedule'].includes(item.field) && isEmptyFillingValue(snapshot.current[item.field])).map(item => item.field))
    } catch (error) { if (id === generation.current) setNotice(error instanceof Error ? error.message : '生成失败，请重试。') }
    finally { if (id === generation.current) setBusy(false) }
  }
  const apply = () => {
    if (!response || disabled) return
    try {
      const acceptance = acceptedFilling(response.result, response.request, latest.current, selected)
      onApply(acceptance)
      setResponse(undefined)
      setNotice(acceptance.result.suggestions.length < selected.length ? '已应用上层字段。请按新上下文重新生成其他建议，再保存草稿。' : '已应用所选建议，请检查并保存草稿。')
    } catch (error) { setNotice(error instanceof Error ? error.message : '应用失败。') }
  }
  return <section className="delivery-filling">
    <button type="button" className="secondary-button" disabled={disabled} aria-expanded={open} onClick={() => setOpen(value => !value)}>智能填写</button>
    {open ? <div className="delivery-filling-body">
      <p>先查看建议，再选择需要填写的字段。已有内容不会自动覆盖。</p>
      <label><span>约束策略（可选）</span><select value={strategy ? `${strategy.package_id}@${strategy.version}` : ''} onChange={event => setStrategy(strategies.find(item => `${item.package_id}@${item.version}` === event.target.value))}>
        <option value="">不使用已批准策略</option>{strategies.map(item => <option key={`${item.package_id}@${item.version}`} value={`${item.package_id}@${item.version}`}>{item.label}</option>)}
      </select></label>
      {strategyError ? <p role="alert">{strategyError}<button type="button" onClick={() => setRetry(value => value + 1)}>重试读取策略</button></p> : null}
      <label><span>目录搜索（可选）</span><input value={search} maxLength={100} placeholder="产品或素材名称；留空读取全部候选" onChange={event => setSearch(event.target.value)}/></label>
      <button type="button" className="primary-button" disabled={busy || disabled} onClick={() => void generate()}>{busy ? '正在生成建议…' : '生成建议'}</button>
      {response ? <>
        {response.result.warnings.map((warning, index) => <p key={index}>{warning}</p>)}
        {!response.result.suggestions.length ? <p role="status">暂无有依据的填写建议，请补充项目资料或缩小目录搜索范围。</p> : <div className="delivery-filling-results"><table><thead><tr><th>应用</th><th>字段</th><th>当前值</th><th>建议值</th><th>依据</th></tr></thead><tbody>
          {response.result.suggestions.map(item => <tr key={item.field}><td><input type="checkbox" aria-label={`应用${fillingLabels[item.field]}建议`} checked={selected.includes(item.field)} onChange={event => setSelected(values => event.target.checked ? [...values, item.field] : values.filter(field => field !== item.field))}/></td><th>{fillingLabels[item.field]}</th><td>{fillingFieldDisplay(item.field, latest.current.current[item.field])}</td><td>{fillingFieldDisplay(item.field, item.value)}</td><td>{item.reason}<small>{item.sources.join('；')}</small></td></tr>)}
        </tbody></table></div>}
        <button type="button" className="primary-button" disabled={disabled || !selected.length} onClick={apply}>应用所选建议</button>
      </> : null}
      {notice ? <p role="status">{notice}</p> : null}
      {history.length ? <details><summary>已接受建议的来源（{history.length} 次）</summary>{history.map((item, index) => <p key={index}>{item.result.suggestions.map(suggestion => fillingLabels[suggestion.field]).join('、')} · {item.result.model} · {item.result.strategy ? `策略 ${item.result.strategy.package_id} V${item.result.strategy.version}` : '项目与目录'} · {new Date(item.accepted_at).toLocaleString('zh-CN')}</p>)}</details> : null}
    </div> : null}
  </section>
}
