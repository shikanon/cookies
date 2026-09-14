import { useEffect, useMemo, useState } from 'react'
import type { ApiConnectorAccount, ApiOptimizationTargetCapabilitySnapshot, ApiOptimizationTargetContext } from '../data/api'
import type { StableReference } from '../api/delivery'
import { carrierOptions, marketingPurposeOptions, scheduleModeOptions, type OceanProject } from '../lib/deliveryChoices'
import { oceanEngineOptimizationTargetContext, optimizationCapabilitySelectionMatches } from '../lib/oceanengineBranchConstraints'
import { OptimizationTargetCapabilityField, ReferenceObjectPicker, type PlatformObjectLoader } from './DeliveryObjectPickers'

export function AccountChoice({ value, accounts, onChange }: { value: StableReference; accounts: ApiConnectorAccount[]; onChange: (id: string) => void }) {
  const available = accounts.some(account => account.id === value.id)
  return <label><span>巨量账户</span><select aria-label="巨量账户" value={value.id ?? ''} onChange={event => onChange(event.target.value)}>
    <option value="">请选择当前 Project 已验证账户</option>
    {value.id && !available ? <option value={value.id} disabled>{value.display_name_snapshot || value.id}（未绑定当前 Project）</option> : null}
    {accounts.map(account => <option key={account.id} value={account.id}>{account.display_label || account.id}</option>)}
  </select><small>切换账户会清除旧账户对象引用。Cookies 素材引用保持不变。</small></label>
}

export function MarketingPurposeChoice({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return <label><span>营销目的</span><select aria-label="营销目的" value={value} onChange={event => onChange(event.target.value)}><option value="">请选择营销目的</option>{marketingPurposeOptions.map(option => <option key={option.value} value={option.value} disabled={option.disabled}>{option.label}</option>)}</select></label>
}

export function CarrierChoice({ project, onChange }: { project: OceanProject; onChange: (value: string) => void }) {
  const options = carrierOptions(project)
  return <label><span>投放载体</span><select aria-label="投放载体" value={project.carrier} onChange={event => onChange(event.target.value)}><option value="">请选择投放载体</option>
    {project.carrier && !options.some(option => option.value === project.carrier) ? <option value={project.carrier} disabled>{project.carrier}（当前分支不可用）</option> : null}
    {options.map(option => <option key={option.value} value={option.value} disabled={option.disabled}>{option.label}</option>)}
  </select></label>
}

export function ScheduleModeOptions() {
  return <>{scheduleModeOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</>
}

export function useOptimizationCapabilities(project: OceanProject | undefined, load: (accountID: string, context: ApiOptimizationTargetContext) => Promise<ApiOptimizationTargetCapabilitySnapshot>) {
  const context = useMemo(() => project ? oceanEngineOptimizationTargetContext(project) : undefined, [project?.carrier, project?.lead_capture_mode, project?.marketing_purpose])
  const [snapshot, setSnapshot] = useState<ApiOptimizationTargetCapabilitySnapshot>()
  const requestKey = JSON.stringify([project?.account_reference.id, context])
  const [resolvedKey, setResolvedKey] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [revision, retry] = useState(0)
  useEffect(() => {
    let active = true
    setSnapshot(undefined)
    setError('')
    const accountID = project?.account_reference.id
    if (!accountID || !context) { setLoading(false); return }
    setLoading(true)
    void load(accountID, context).then(value => { if (active) { setSnapshot(value); setResolvedKey(requestKey) } }).catch(error => {
      if (active) setError(error instanceof Error ? error.message : '读取当前分支的优化目标失败。')
    }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [project?.account_reference.id, context, requestKey, load, revision])
  return { context, snapshot: resolvedKey === requestKey ? snapshot : undefined, loading, error, retry: () => retry(value => value + 1) }
}

export function OptimizationChoice({ project, capabilities, loadObjects, onChange }: { project: OceanProject; capabilities: ReturnType<typeof useOptimizationCapabilities>; loadObjects: PlatformObjectLoader; onChange: (value?: StableReference) => void }) {
  const { context, snapshot, loading, error, retry } = capabilities
  return context ? <div>
    <OptimizationTargetCapabilityField accountID={project.account_reference.id ?? ''} value={project.optimization_target_reference} snapshot={snapshot} loading={loading} error={error} onChange={onChange}/>
    {error ? <button type="button" className="secondary-button" onClick={retry}>重试优化目标</button> : null}
    {!loading && project.optimization_target_reference && (!snapshot || !optimizationCapabilitySelectionMatches(project, snapshot.snapshot_id, snapshot.options.map(option => option.external_action))) ? <small role="status">原优化目标：{project.optimization_target_reference.display_name_snapshot || project.optimization_target_reference.id}（当前能力未确认，请重新选择）</small> : null}
  </div> : <ReferenceObjectPicker label="优化目标" pickerTitle="选择优化目标" value={project.optimization_target_reference} objectKind="optimization_target" loadPlatformObjects={loadObjects} requiredContext={project.carrier === 'owned_landing_page' ? 'owned_landing_page' : 'orange_landing_page'} onChange={onChange}/>
}
