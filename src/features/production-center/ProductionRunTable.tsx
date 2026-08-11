import { ChevronRight, CircleAlert, ExternalLink, Image, Music2, Video } from 'lucide-react'
import type { ProductionRunRef, ProductionRunSummary, ProductionStatus } from './types'

const statusLabels: Record<ProductionStatus, string> = {
  queued: '排队中', running: '运行中', ingesting: '资产入库中', succeeded: '已完成',
  partially_succeeded: '部分成功', failed: '失败', expired: '已过期', cancelled: '已取消',
}
function RunIcon({ kind }: { kind: ProductionRunSummary['media_kind'] }) {
  if (kind === 'image') return <Image size={17}/>
  if (kind === 'audio') return <Music2 size={17}/>
  return <Video size={17}/>
}

function costLabel(run: ProductionRunSummary) {
  if (!run.cost || run.cost.availability === 'unavailable' || run.cost.amount_minor === null) return '尚不可用'
  const amount = (run.cost.amount_minor / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return `${run.cost.availability === 'estimated' ? '估算 ' : ''}${run.cost.currency} ${amount}`
}

export function ProductionRunTable({ items, selected, onSelect, onOpenSource }: {
  items: ProductionRunSummary[]
  selected?: ProductionRunRef
  onSelect: (ref: ProductionRunRef) => void
  onOpenSource: (run: ProductionRunSummary) => void
}) {
  return <div className="pc-table-wrap">
    <table className="pc-table">
      <thead><tr><th>制作任务</th><th>来源任务</th><th>状态 / 进度</th><th>输出</th><th>实际成本</th><th>最后更新</th><th aria-label="操作"/></tr></thead>
      <tbody>{items.map(run => {
        const active = selected?.source === run.ref.source && selected.id === run.ref.id
        return <tr key={`${run.ref.source}:${run.ref.id}`} className={active ? 'selected' : ''} onClick={() => onSelect(run.ref)}>
          <td><div className={`pc-run-icon ${run.media_kind}`}><RunIcon kind={run.media_kind}/></div><div><b>{run.operation_kind}</b><small>{run.ref.source} · {run.ref.id}</small></div></td>
          <td>{run.source_task ? <button className="pc-source-link" onClick={event => { event.stopPropagation(); onOpenSource(run) }}>{run.source_task.display_name ?? run.source_task.object_id}<ExternalLink size={12}/></button> : <span className="pc-muted">未关联</span>}</td>
          <td><span className={`pc-status ${run.normalized_status}`}>{run.error ? <CircleAlert size={13}/> : null}{statusLabels[run.normalized_status]}</span><div className="pc-progress"><i style={{ width: `${Math.max(0, Math.min(100, run.progress_percent))}%` }}/></div><small>{run.progress_percent}%</small></td>
          <td><b>{run.output_count}</b><small> 个稳定输出</small></td>
          <td>{costLabel(run)}</td>
          <td>{new Date(run.updated_at).toLocaleString('zh-CN', { hour12: false })}</td>
          <td><button aria-label={`查看 ${run.ref.id} 详情`}><ChevronRight size={17}/></button></td>
        </tr>
      })}</tbody>
    </table>
  </div>
}
