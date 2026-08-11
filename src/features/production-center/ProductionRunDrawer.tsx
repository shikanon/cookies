import { AlertTriangle, ExternalLink, RefreshCw, X } from 'lucide-react'
import { Fragment } from 'react'
import type { ProductionAsset, ProductionRunDetail } from './types'

function AssetPreview({ asset }: { asset: ProductionAsset }) {
  return <article className="pc-drawer-asset">
    {asset.media_kind === 'image' && asset.preview_url ? <img src={asset.preview_url} alt={asset.display_name ?? asset.asset_ref.asset_id}/>
      : asset.media_kind === 'video' && asset.preview_url ? <video controls preload="metadata" src={asset.preview_url}/>
        : asset.media_kind === 'audio' && asset.preview_url ? <audio controls preload="metadata" src={asset.preview_url}/>
          : <div className="pc-no-preview">暂无可用预览</div>}
    <b>{asset.display_name ?? asset.asset_ref.asset_id}</b><small>v{asset.asset_ref.version} · {asset.availability}</small>
  </article>
}

export function ProductionRunDrawer({ detail, loading, error, retrying, retryError, requiresSourceWorkflow, onClose, onOpenSource, onRetry }: {
  detail: ProductionRunDetail | null
  loading: boolean
  error: string | null
  retrying: boolean
  retryError: string | null
  requiresSourceWorkflow: boolean
  onClose: () => void
  onOpenSource: () => void
  onRetry: () => void
}) {
  return <aside className="pc-drawer" aria-label="制作任务详情">
    <header><div><span>制作任务详情</span><h2>{detail?.summary.operation_kind ?? '正在读取…'}</h2></div><button onClick={onClose} aria-label="关闭详情"><X size={19}/></button></header>
    {loading ? <div className="pc-drawer-state">正在从服务端读取任务事实…</div> : error ? <div className="pc-drawer-state error"><AlertTriangle size={18}/>{error}</div> : detail ? <div className="pc-drawer-content">
      {detail.summary.actions.retry ? <section className="pc-retry-panel"><div><h3>受控重试</h3><p>基于已冻结的输入创建一个新任务，原失败任务和重试链都会保留。</p></div><button onClick={onRetry} disabled={retrying}><RefreshCw size={14}/>{retrying ? '正在提交…' : '重试此任务'}</button>{retryError ? <p className="pc-retry-error"><AlertTriangle size={14}/>{retryError}</p> : null}{requiresSourceWorkflow && detail.summary.actions.open_source ? <button className="pc-open-source" onClick={onOpenSource}>返回来源工作流<ExternalLink size={13}/></button> : null}</section> : null}
      <section><h3>概览</h3><dl><dt>运行 ID</dt><dd>{detail.summary.ref.source}:{detail.summary.ref.id}</dd><dt>状态</dt><dd>{detail.summary.normalized_status} · {detail.summary.progress_percent}%</dd><dt>模型</dt><dd>{detail.summary.model ? `${detail.summary.model.logical_alias}${detail.summary.model.actual_model ? ` / ${detail.summary.model.actual_model}` : ''}` : '尚不可用'}</dd><dt>创建时间</dt><dd>{new Date(detail.summary.created_at).toLocaleString('zh-CN', { hour12: false })}</dd></dl></section>
      {detail.summary.source_task ? <section><h3>来源任务</h3><button className="pc-open-source" onClick={onOpenSource}>{detail.summary.source_task.display_name ?? detail.summary.source_task.object_id}<ExternalLink size={13}/></button></section> : null}
      <section><h3>输入资产</h3>{detail.input_assets.length ? <div className="pc-drawer-assets">{detail.input_assets.map(asset => <AssetPreview key={`${asset.asset_ref.asset_id}:${asset.asset_ref.version}`} asset={asset}/>)}</div> : <p className="pc-muted">当前来源未提供稳定输入 AssetVersion。</p>}</section>
      <section><h3>运行参数</h3>{Object.keys(detail.parameters).length ? <dl>{Object.entries(detail.parameters).map(([key, value]) => <Fragment key={key}><dt>{key}</dt><dd>{String(value)}</dd></Fragment>)}</dl> : <p className="pc-muted">当前来源未提供安全参数摘要。</p>}</section>
      <section><h3>输出资产</h3>{detail.output_assets.length ? <div className="pc-drawer-assets">{detail.output_assets.map(asset => <AssetPreview key={`${asset.asset_ref.asset_id}:${asset.asset_ref.version}`} asset={asset}/>)}</div> : <p className="pc-muted">尚无已入库的输出 AssetVersion。</p>}</section>
      <section><h3>成本</h3><p>{detail.summary.cost?.amount_minor !== null && detail.summary.cost?.amount_minor !== undefined ? `${detail.summary.cost.availability === 'estimated' ? '估算 ' : ''}${detail.summary.cost.currency} ${(detail.summary.cost.amount_minor / 100).toFixed(2)}` : detail.summary.cost?.unavailable_reason ?? '实际成本尚不可用'}</p></section>
      <section><h3>运行事件</h3>{detail.run_events.length ? <ol className="pc-events">{detail.run_events.map(event => <li key={event.ordinal}><time>{new Date(event.occurred_at).toLocaleString('zh-CN', { hour12: false })}</time><b>{event.stage}{event.error_code ? ` · ${event.error_code}` : ''}</b><p>{event.safe_message}</p></li>)}</ol> : <p className="pc-muted">当前来源尚未提供结构化安全事件。</p>}</section>
      <section><h3>错误</h3>{detail.summary.error ? <div className="pc-error-box"><b>{detail.summary.error.code}</b><p>{detail.summary.error.message}</p><span>{detail.summary.error.retryable ? '来源工作流标记为可重试' : '不可重试'}</span></div> : <p className="pc-muted">没有错误事实。</p>}</section>
      <section><h3>尝试与血缘</h3><dl><dt>尝试次数</dt><dd>{detail.attempt.attempt_count} / {detail.attempt.max_attempts}</dd><dt>输入版本</dt><dd>{detail.lineage.input_asset_refs.length}</dd><dt>输出版本</dt><dd>{detail.lineage.output_asset_refs.length}</dd><dt>重试链</dt><dd>{detail.retry_chain.length ? detail.retry_chain.map(ref => `${ref.source}:${ref.id}`).join(' → ') : '无'}</dd></dl></section>
    </div> : null}
  </aside>
}
