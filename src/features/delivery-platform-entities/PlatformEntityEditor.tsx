import { useEffect, useRef, useState } from 'react'
import { deliveryExecutionApi, deliveryPlanApi, type DeliveryPlan, type DeliveryPlanObjectPreview, type DeliveryPlatformEntityMapping } from '../../api/delivery'
import { projectPath } from '../../lib/router'
import { PlanObjectActions } from './PlanObjectActions'

export function PlatformEntityEditor({ projectId, mapping, onClose }: { projectId: string; mapping: DeliveryPlatformEntityMapping; onClose: () => void }) {
  const editorRef = useRef<HTMLElement>(null)
  const [plan, setPlan] = useState<DeliveryPlan>()
  const [preview, setPreview] = useState<DeliveryPlanObjectPreview>()
  const [name, setName] = useState('')
  const [budget, setBudget] = useState('')
  const [currentBudget, setCurrentBudget] = useState('')
  const [verified, setVerified] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [failed, setFailed] = useState(false)
  const [change, setChange] = useState<{ id: string; version: number; current: string; target: string; key: string }>()
  const isProject = mapping.internal_object_kind === 'project'
  useEffect(() => {
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
    editorRef.current?.scrollIntoView({ block: 'start' })
    editorRef.current?.focus({ preventScroll: true })
    return () => { if (trigger?.isConnected) trigger.focus({ preventScroll: true }) }
  }, [mapping.id])
  useEffect(() => {
    let active = true
    void Promise.all([deliveryPlanApi.get(projectId, mapping.plan_id), deliveryExecutionApi.previewPlanObjects(projectId, mapping.plan_id)]).then(([selected, nextPreview]) => {
      if (!active) return
      const ocean = selected?.currentVersion.platformConfiguration?.payload.ocean_engine
      const object = isProject ? ocean?.project : ocean?.promotions.find(value => value.promotion_draft_id === mapping.internal_object_id)
      if (!selected || !object) throw new Error('当前配置中找不到绑定对象，请先核对历史版本。')
      setPlan(selected)
      setPreview({ ...nextPreview, objects: nextPreview.objects.filter(value => value.mapping_id === mapping.id) })
      setName('project_name' in object ? object.project_name : object.promotion_name)
      setBudget(String((object.budget_and_bidding?.daily_budget_minor ?? 0) / 100))
    }).catch(error => { if (active) { setFailed(true); setNotice(error instanceof Error ? error.message : '读取对象失败。') } })
    return () => { active = false }
  }, [projectId, mapping.id, mapping.plan_id, mapping.internal_object_id, isProject])

  const persistBudgetTarget = async () => {
    const configuration = plan?.currentVersion.platformConfiguration
    if (isProject || !plan || !configuration?.payload.ocean_engine) throw new Error('当前对象不能修改日预算。')
    const amount = Math.round(Number(budget) * 100)
    if (!Number.isInteger(Number(budget)) || amount < 30000) throw new Error('目标日预算至少 300 元，请按整元填写。')
    const next = structuredClone(configuration)
    const promotion = next.payload.ocean_engine!.promotions.find(value => value.promotion_draft_id === mapping.internal_object_id)
    if (!promotion?.budget_and_bidding) throw new Error('缺少单元预算基线，请先核对平台对象。')
    if (promotion.budget_and_bidding.daily_budget_minor === amount) return
    promotion.budget_and_bidding.daily_budget_minor = amount
    const saved = await deliveryPlanApi.updatePlatformConfiguration(projectId, plan, next)
    setPlan(saved)
    const updated = await deliveryExecutionApi.previewPlanObjects(projectId, plan.id)
    setPreview({ ...updated, objects: updated.objects.filter(value => value.mapping_id === mapping.id) })
  }

  const save = async () => {
    if (busy) return
    setBusy(true)
    setFailed(false)
    try {
      await persistBudgetTarget()
      setNotice('单元预算目标已保存。平台 ID 保持不变。尚未修改巨量对象。')
    } catch (error) { setFailed(true); setNotice(error instanceof Error ? error.message : '保存失败。') }
    finally { setBusy(false) }
  }

  const compileBudget = async () => {
    if (!verified || busy) return
    setBusy(true)
    setFailed(false)
    try {
      await persistBudgetTarget()
      const result = await deliveryExecutionApi.compileObjectBudgetChange(projectId, mapping, Math.round(Number(currentBudget) * 100), Math.round(Number(budget) * 100))
      setChange({ ...result, current: currentBudget, target: budget, key: crypto.randomUUID() })
      setNotice(`已生成单元预算变更 ${result.id}。目标：巨量单元 ${mapping.platform_object_id}；${currentBudget} 元 → ${budget} 元。变更待审批，尚未提交平台。`)
    } catch (error) { setFailed(true); setNotice(error instanceof Error ? error.message : '生成预算变更失败。') }
    finally { setBusy(false) }
  }

  const start = async () => {
    if (!change || busy) return
    setBusy(true)
    setFailed(false)
    try {
      const result = await deliveryExecutionApi.startObjectExecution(projectId, change.id, change.version, change.key)
      window.location.assign(projectPath(projectId, 'delivery', 'execution', result.browser_rpa_run.run_id))
    } catch (error) { setFailed(true); setNotice(error instanceof Error ? error.message : '准备执行失败。') }
    finally { setBusy(false) }
  }

  return <section ref={editorRef} tabIndex={-1} className="platform-entity-editor" aria-label={`编辑${isProject ? '项目' : '单元'}`} onKeyDown={event => { if (event.key === 'Escape') onClose() }}>
    <header><h3>编辑{isProject ? '项目' : '单元'} · {mapping.platform_object_id || '绑定待确认'}</h3><button type="button" className="secondary-button" onClick={onClose}>关闭</button></header>
    {notice ? <p role={failed ? 'alert' : 'status'}>{notice}</p> : null}
    {plan ? <>
      <p>{isProject ? '项目已创建。可查看所属单元，或在原项目下新增单元。' : '日预算单独生成变更。保存目标草稿不会新建平台对象。'}</p>
      <label>名称<input aria-label="对象名称" value={name} readOnly/><small>已有对象的名称修改尚待 Runner 校准。</small></label>
      <label>目标日预算（元）<input aria-label="目标日预算" type="number" min={isProject ? 0 : 300} step="1" value={budget} readOnly={isProject || mapping.status !== 'confirmed'} onChange={event => { setBudget(event.target.value); setChange(undefined) }}/></label>
      {!isProject ? <button type="button" className="primary-button" disabled={busy || mapping.status !== 'confirmed'} onClick={() => void save()}>保存预算目标草稿</button> : <p>项目修改链路尚未校准。当前显示配置基线，暂不开放自动修改。</p>}
      {!isProject && mapping.status === 'confirmed' ? <fieldset><legend>单独修改平台日预算</legend><p>日预算至少 300 元，目标预算按整元填写。此变更只包含日预算。</p><label>平台当前日预算（元）<input aria-label="平台当前日预算" type="number" min="300" step="0.01" value={currentBudget} onChange={event => { setCurrentBudget(event.target.value); setVerified(false); setChange(undefined) }}/></label><label><input type="checkbox" checked={verified} onChange={event => setVerified(event.target.checked)}/>已核对上述巨量单元 ID 和平台当前预算</label><button type="button" className="secondary-button" disabled={busy || !verified || !currentBudget || !Number.isInteger(Number(budget)) || Number(budget) < 300 || Number(currentBudget) === Number(budget)} onClick={() => void compileBudget()}>生成预算变更供审批</button></fieldset> : null}
      <section><h4>{isProject ? '所属单元与新增单元' : '所属项目'}</h4><p>{plan.currentVersion.platformConfiguration?.payload.ocean_engine?.project.project_name}</p>{isProject ? <ul>{plan.currentVersion.platformConfiguration?.payload.ocean_engine?.promotions.map(value => <li key={value.promotion_draft_id}>{value.promotion_name}</li>)}</ul> : null}<a href={`${projectPath(projectId, 'delivery', 'configuration', undefined, '配置映射')}&plan_id=${encodeURIComponent(plan.id)}`}>打开同一项目的配置{isProject ? '，添加单元' : ''}</a></section>
      {change ? <section aria-label="确认单元预算变更"><h4>确认单元预算变更</h4><p>巨量单元 {mapping.platform_object_id}：{change.current} 元 → {change.target} 元。</p><p>确认后准备此单元的编辑表单。平台最终保存仍需在执行页确认。</p><button type="button" className="primary-button" disabled={busy} onClick={() => void start()}>确认此变更并准备执行</button></section> : null}
      {preview ? <PlanObjectActions preview={preview} projectId={projectId}/> : null}
    </> : !failed ? <p>正在读取对象配置…</p> : null}
  </section>
}
