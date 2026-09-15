import { useEffect, useRef, useState } from 'react'
import { deliveryFillingApi, type FillingAcceptance, type FillingRequest, type FillingFinance, type FillingStrategy } from '../api/deliveryFilling'
import { automaticFilling, fillingContextKey, fillingLabels } from '../lib/deliveryFilling'

export function DeliveryFillingPanel({ projectId, request, disabled, financialHistory, targets, onTargetChange, onApply }: {
  targets: Array<{ id: string; label: string; disabled: boolean }>; onTargetChange: (id: string) => void;
  projectId: string; request: FillingRequest; disabled?: boolean; financialHistory?: FillingFinance; onApply: (acceptance: FillingAcceptance) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const pending = useRef<AbortController | undefined>(undefined)
  const applyLatest = useRef(onApply)
  applyLatest.current = onApply
  const latest = useRef(request)
  latest.current = request
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [strategy, setStrategy] = useState('')
  const [strategies, setStrategies] = useState<Array<FillingStrategy & { label: string }>>([])
  const [strategyError, setStrategyError] = useState('')
  const [retry, setRetry] = useState(0)
  const [search, setSearch] = useState('')
  const [instructions, setInstructions] = useState('')
  const [materialCount, setMaterialCount] = useState(1)
  const [copyCount, setCopyCount] = useState(3)
  const [fillFinance, setFillFinance] = useState(false)
  const [unitWeight, setUnitWeight] = useState(1)
  const [finance, setFinance] = useState<FillingFinance | undefined>(financialHistory)
  useEffect(() => { setFinance(financialHistory) }, [financialHistory])
  useEffect(() => { if (open) dialog.current?.showModal(); else dialog.current?.close() }, [open])
  useEffect(() => {
    if (!open) return
    let active = true
    setStrategyError('')
    void deliveryFillingApi.strategies(projectId).then(items => { if (active) setStrategies(items) }).catch(() => { if (active) setStrategyError('策略读取失败，可重试或不使用策略。') })
    return () => { active = false }
  }, [open, projectId, retry])
  useEffect(() => { pending.current?.abort(); setBusy(false); setOpen(false); setStrategy(''); setNotice(''); return () => pending.current?.abort() }, [projectId, request.plan_id, disabled])
  useEffect(() => { pending.current?.abort(); setBusy(false); setNotice('') }, [request.target_id])
  const close = () => { pending.current?.abort(); setBusy(false); setOpen(false) }
  const generate = async () => {
    if (disabled || busy) return
    const controller = new AbortController()
    pending.current = controller
    const selected = strategies.find(item => `${item.package_id}@${item.version}` === strategy)
    const options = { finance: fillFinance ? { unit_weight: unitWeight } : undefined, instructions, search, material_count: materialCount, copy_count: copyCount, strategy: selected ? { package_id: selected.package_id, version: selected.version, content_hash: selected.content_hash } : undefined }
    const snapshot = structuredClone({ ...latest.current, ...options })
    setBusy(true); setNotice('')
    try {
      const result = await deliveryFillingApi.suggest(projectId, snapshot, controller.signal)
      if (controller.signal.aborted) return
      const current = { ...latest.current, ...options }
      if (fillingContextKey(snapshot) !== fillingContextKey(current)) throw new Error('账户、产品或单元已变化，请重新生成。')
      const { acceptance, skipped } = automaticFilling(result, snapshot, current)
      if (acceptance.result.suggestions.length) applyLatest.current(acceptance)
      setFinance(acceptance.result.finance)
      setNotice([acceptance.result.suggestions.length ? `已填写 ${acceptance.result.suggestions.length} 项，请检查并保存草稿。` : '没有可填写的内容。', skipped.length ? `已保留期间手工修改的${skipped.join('、')}。` : '', ...result.warnings].filter(Boolean).join(' '))
      setOpen(false)
    } catch (error) { if (!controller.signal.aborted) setNotice(error instanceof Error ? error.message : '智能填写失败，请重试。') }
    finally { if (pending.current === controller) setBusy(false) }
  }
  return <section className="delivery-filling">
    <button type="button" className="secondary-button" disabled={disabled} onClick={() => { setNotice(''); setOpen(true) }}>智能填写</button>
    {!open && notice ? <p role="status">{notice}</p> : null}
    {!open && finance ? <div className="delivery-filling-finance" aria-label="预算与出价填写记录">
      {finance.entries.length ? <><strong>本次金额填写记录</strong><ul>{finance.entries.map(entry => <li key={entry.field}>{fillingLabels[entry.field]}：¥{(entry.amount_minor / 100).toFixed(2)}{entry.field === 'daily_budget' ? ' / 天' : `（历史区间 ¥${(entry.minimum_minor / 100).toFixed(2)}～¥${(entry.maximum_minor / 100).toFixed(2)}）`}<small>{entry.basis}</small></li>)}</ul></> : null}
      {finance.warnings.map((warning, index) => <p key={index}>{warning}</p>)}
    </div> : null}
    <dialog ref={dialog} className="delivery-filling-dialog" aria-labelledby={`filling-title-${request.target_id}`} onCancel={event => { event.preventDefault(); close() }}>
      <form onSubmit={event => { event.preventDefault(); void generate() }}>
        <header><h3 id={`filling-title-${request.target_id}`}>智能填写</h3><button type="button" aria-label="关闭智能填写" onClick={close}>×</button></header>
        <p>填写投放项目与所选单元。搜索关键词、定向及项目出价作用于整个项目；素材、文案及单元金额只写入所选单元。不会自动保存。</p>
        <fieldset disabled={busy}>
          <label>填写单元<select value={request.target_id} onChange={event => onTargetChange(event.target.value)}>{targets.map(target => <option key={target.id} value={target.id} disabled={target.disabled}>{target.label}{target.disabled ? '（已绑定，不可填写）' : ''}</option>)}</select></label>
          <div className="delivery-filling-counts">
            <label>选用素材数量<input type="number" min={1} max={request.ocean.project.marketing_purpose === 'content_marketing' && request.ocean.project.carrier === 'douyin_account' ? 1 : 10} required value={materialCount} onChange={event => setMaterialCount(Number(event.target.value))}/></label>
            {request.fields.includes('copy') ? <label>生成文案数量<input type="number" min={1} max={10} required value={copyCount} onChange={event => setCopyCount(Number(event.target.value))}/></label> : null}
          </div>
          <label className="delivery-filling-finance-toggle"><input type="checkbox" checked={fillFinance} onChange={event => setFillFinance(event.target.checked)}/>填写预算与出价</label>
          {fillFinance ? <div className="delivery-filling-finance">
            <p>项目日预算 ¥{(request.ocean.project.budget_and_bidding.daily_budget_minor / 100).toFixed(2)}。保留其他单元已设预算，将剩余预算按权重分配，仅填写当前单元。出价只参考条件一致的真实历史；资料不足时保留原值。</p>
            <label>当前单元预算权重<input type="number" min={1} max={10} step={1} required value={unitWeight} onChange={event => setUnitWeight(Number(event.target.value))}/></label>
            <small>其他未设预算单元权重为 1。项目统一控预算的模式不填写单元预算。附加说明不会直接改变金额规则。</small>
          </div> : null}
          <label>附加说明<textarea rows={4} maxLength={2000} value={instructions} placeholder="例如：面向上海年轻用户，突出活动入口，避免承诺具体优惠金额" onChange={event => setInstructions(event.target.value)}/></label>
          <label>素材筛选（可选）<input maxLength={100} value={search} placeholder="素材名称关键词；候选过多时缩小范围" onChange={event => setSearch(event.target.value)}/></label>
          <label>约束策略（可选）<select value={strategy} onChange={event => setStrategy(event.target.value)}><option value="">使用产品资料和附加说明</option>{strategies.map(item => <option key={`${item.package_id}@${item.version}`} value={`${item.package_id}@${item.version}`}>{item.label}</option>)}</select></label>
        </fieldset>
        {strategyError ? <p role="alert">{strategyError}<button type="button" onClick={() => setRetry(value => value + 1)}>重试</button></p> : null}
        {notice ? <p role="alert">{notice}</p> : null}
        <footer><button type="button" className="secondary-button" onClick={close}>取消</button><button type="submit" className="primary-button" disabled={busy || disabled}>{busy ? '正在填写…' : '开始填写'}</button></footer>
      </form>
    </dialog>
  </section>
}
