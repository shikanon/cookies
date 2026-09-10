import { useState } from 'react'
import { deliveryExecutionApi, type DeliveryPlanObjectAction, type DeliveryPlanObjectPreview, type FieldCapabilitySnapshot } from '../../api/delivery'

const actions = { create: '新增', update: '待同步', unchanged: '未变更', blocked: '待核对' }
const states: Record<string, string> = { editable: '支持', immutable: '暂不支持', conditional: '部分支持', unverified: '尚未支持' }
const labels: Record<string, string> = {
  account_reference: '投放账户', marketing_purpose: '营销目的', marketing_scenario: '营销场景', project_name: '项目名称', promotion_name: '单元名称', budget_and_bidding: '预算与出价', delivery_mode: '投放模式', carrier: '投放载体', targeting: '定向', schedule: '排期', delivery_identity: '投放身份', base_material_references: '素材', copy_items: '文案', landing_page_reference: '落地页', settings: '单元设置', product_name: '商品名称', product_image_references: '商品图片', product_selling_points: '商品卖点', marketing_product_reference: '营销产品', optimization_target_reference: '优化目标', monitoring_references: '监测链接', placement_strategy: '版位策略', placement_media: '投放版位', deep_optimization_mode: '深度优化', search_boost: '搜索快投', aigc_dynamic_creative: '动态创意', product_reference: '商品', creative_component_references: '创意组件', direct_link_reference: '直达链接', native_anchor_reference: '原生锚点',
}

export function PlanObjectActions({ preview, onEdit, projectId }: { projectId?: string; preview: DeliveryPlanObjectPreview; onEdit?: (id: string) => void }) {
  return <section className="platform-object-actions" aria-label="对象变更预览">
    <h3>对象变更 · V{preview.version}</h3><p>计划版本记录配置历史。执行按对象处理，已有对象保留平台 ID。</p>
    {preview.objects.map(object => <article key={`${object.kind}:${object.internal_id}`}>
      <header><b>{object.name || object.internal_id}</b><strong>{actions[object.action]}</strong>{onEdit ? <button className="secondary-button" type="button" onClick={() => onEdit(object.internal_id)}>编辑{object.kind === 'project' ? '项目' : '单元'}</button> : null}</header>
      <p>{object.platform_id ? `巨量 ID ${object.platform_id} · ` : ''}{object.reason || (object.action === 'create' ? '创建一个新平台对象。' : '保留已有对象身份。')}</p>
      {object.changed_fields.length ? <p>变更字段：{object.changed_fields.map(key => labels[key] ?? key).join('、')}</p> : null}
      {object.differences?.length ? <dl>{object.differences.map(difference => <div key={difference.key}><dt>{labels[difference.key] ?? difference.key}</dt><dd>{differenceValue(difference.before)} → {differenceValue(difference.after)}</dd></div>)}</dl> : null}
      <FieldCapabilityDetails key={object.internal_id} object={object} projectId={projectId}/>
    </article>)}
  </section>
}

function differenceValue(value: unknown): string {
  if (value == null) return '未设置'
  if (typeof value === 'boolean') return value ? '开启' : '关闭'
  if (typeof value === 'string' || typeof value === 'number') return String(value)
  if (Array.isArray(value)) return value.length ? value.map(differenceValue).join('、') : '无'
  if (typeof value === 'object') {
    if ('daily_budget_minor' in value && typeof value.daily_budget_minor === 'number') return `日预算 ${value.daily_budget_minor / 100} 元`
    if ('text' in value && typeof value.text === 'string') return value.text
    if ('id' in value && typeof value.id === 'string') return value.id
    return Object.entries(value).map(([key, entry]) => `${labels[key] ?? key}：${differenceValue(entry)}`).join('；')
  }
  return '未设置'
}

export function PlanObjectStatus({ object, loading, onEdit, projectId }: { projectId: string; object?: DeliveryPlanObjectAction; loading: boolean; onEdit: (id: string) => void }) {
  const action = object?.action ?? 'create'
  return <div className="delivery-config-object-status-content">
    <span className={`delivery-config-object-badge is-${loading ? 'loading' : action}`}>{loading ? '读取状态中' : actions[action]}</span>
    {object?.action === 'blocked' ? <span role="alert">{object.reason}</span> : null}
    {object?.action === 'update' ? <span>已保存，尚未同步到巨量</span> : null}
    {object?.kind === 'project' && object.platform_id ? <span>巨量项目 ID：{object.platform_id}</span> : null}
    {object?.kind === 'promotion' && object.mapping_id ? <button className="secondary-button" type="button" onClick={() => onEdit(object.internal_id)}>编辑单元</button> : null}
    {object?.mapping_id ? <FieldCapabilityDetails key={object.mapping_id} object={object} projectId={projectId}/> : null}
  </div>
}

const platformStates: Record<string, string> = { editable: '页面可编辑', readonly: '页面只读', unknown: '未确认', conditional: '部分可编辑', not_applicable: '尚未创建' }
const nestedLabels: Record<string, string> = { daily_budget_minor: '日预算', bid_minor: '出价', charging_mode: '付费方式', bidding_strategy: '竞价策略', budget_mode: '预算类型', region: '地域', gender: '性别', age: '年龄', mode: '类型', start_time: '开始时间', end_time: '结束时间', dayparting: '投放时段', bid_coefficient: '出价系数', display: '展示监测', action: '有效触点监测', kind: '类型', additional: '附加名称', backup: '备用链接', source: '来源', comment: '评论', category: '分类', brand: '品牌' }
function fieldLabel(key: string) { return key.split('.').map(part => labels[part] ?? nestedLabels[part] ?? part).join(' · ') }

export function FieldCapabilityDetails({ object, projectId }: { object: DeliveryPlanObjectAction; projectId?: string }) {
  const [snapshot, setSnapshot] = useState<FieldCapabilitySnapshot>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const refresh = async () => {
    if (!projectId || !object.mapping_id || busy) return
    setBusy(true); setError(''); setSnapshot(undefined)
    try { setSnapshot(await deliveryExecutionApi.readObjectFieldCapabilities(projectId, object.mapping_id)) }
    catch (error) { setError(error instanceof Error ? error.message : '读取失败，请确认 Edge 已登录。') }
    finally { setBusy(false) }
  }
  const policies = snapshot?.fields ?? object.fields ?? []
  const keys = Array.from(new Set([...policies.map(field => field.key), ...snapshot?.observations.map(field => field.key) ?? []]))
  return <details className="field-capability-details"><summary>字段编辑权限</summary>
    <p>平台页面状态与 Cookies 支持情况分别显示。检查结果仅反映当时页面，不会开放未支持的修改。</p>
    {object.mapping_id && projectId ? <button type="button" className="secondary-button" disabled={busy} onClick={() => void refresh()}>{busy ? '正在检查…' : '检查平台字段状态'}</button> : null}
    {error ? <p role="alert">{error}</p> : null}
    {snapshot ? <p>检查于 {new Date(snapshot.observed_at).toLocaleString()}{snapshot.object_can_edit !== undefined ? `；项目接口：${snapshot.object_can_edit ? '允许编辑项目（不代表所有字段）' : '不允许编辑项目'}` : ''}</p> : null}
    <div className="field-capability-table"><table><thead><tr><th>字段</th><th>平台页面</th><th>Cookies</th></tr></thead><tbody>{keys.map(key => {
      const policy = policies.find(field => field.key === key) ?? policies.find(field => field.key === key.split('.')[0])
      const observation = snapshot?.observations.find(field => field.key === key)
      const platform = observation?.platform ?? (key.includes('.') ? undefined : policy?.platform)
      const cookiesState = key === 'budget_and_bidding.daily_budget_minor' && object.kind === 'promotion' ? 'conditional' : key.startsWith('budget_and_bidding.') ? 'unverified' : policy?.cookies?.state ?? policy?.state ?? 'unverified'
      return <tr key={key}><th>{fieldLabel(key)}</th><td title={platform?.reason}>{platformStates[platform?.state ?? 'unknown'] ?? '未确认'}</td><td title={policy?.cookies?.reason ?? policy?.reason}>{states[cookiesState] ?? '尚未支持'}</td></tr>
    })}</tbody></table></div>
  </details>
}
