import { Search, SlidersHorizontal } from 'lucide-react'
import type { ProductionStatus } from './types'

export type ProductionFilterState = {
  query: string
  status: ProductionStatus | ''
  sourceTaskId: string
  createdAfter: string
  createdBefore: string
}
export const emptyProductionFilters: ProductionFilterState = {
  query: '', status: '', sourceTaskId: '', createdAfter: '', createdBefore: '',
}

export function ProductionFilters({ value, onChange, onRefresh, loading }: {
  value: ProductionFilterState
  onChange: (value: ProductionFilterState) => void
  onRefresh: () => void
  loading: boolean
}) {
  const patch = (next: Partial<ProductionFilterState>) => onChange({ ...value, ...next })
  return <div className="pc-filters" aria-label="制作任务筛选">
    <label className="pc-search"><Search size={16}/><input value={value.query} onChange={event => patch({ query: event.target.value })} placeholder="搜索任务 ID、来源任务或错误码" aria-label="搜索制作任务"/></label>
    <label><span>状态</span><select value={value.status} onChange={event => patch({ status: event.target.value as ProductionStatus | '' })}>
      <option value="">全部状态</option>
      <option value="queued">排队中</option><option value="running">运行中</option><option value="ingesting">资产入库中</option>
      <option value="succeeded">已完成</option><option value="partially_succeeded">部分成功</option><option value="failed">失败</option>
      <option value="expired">已过期</option><option value="cancelled">已取消</option>
    </select></label>
    <label><span>来源任务</span><input value={value.sourceTaskId} onChange={event => patch({ sourceTaskId: event.target.value })} placeholder="任务 ID"/></label>
    <label><span>开始时间</span><input type="datetime-local" value={value.createdAfter} onChange={event => patch({ createdAfter: event.target.value })}/></label>
    <label><span>结束时间</span><input type="datetime-local" value={value.createdBefore} onChange={event => patch({ createdBefore: event.target.value })}/></label>
    <button className="pc-refresh" onClick={onRefresh} disabled={loading}><SlidersHorizontal size={15}/>{loading ? '刷新中' : '刷新'}</button>
  </div>
}
