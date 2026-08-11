import { ExternalLink, FileImage, FileMusic, FileVideo } from 'lucide-react'
import type { ProductionAssetItem, ProductionMediaKind, ProductionRunRef } from './types'

function AssetIcon({ kind }: { kind: Exclude<ProductionMediaKind, 'render'> }) {
  if (kind === 'image') return <FileImage size={21}/>
  if (kind === 'audio') return <FileMusic size={21}/>
  return <FileVideo size={21}/>
}
export function ProductionAssetList({ items, onOpenRun }: { items: ProductionAssetItem[]; onOpenRun: (ref: ProductionRunRef) => void }) {
  return <div className="pc-assets">
    {items.map(item => <article className="pc-asset-card" key={`${item.asset.asset_ref.asset_id}:${item.asset.asset_ref.version}`}>
      <div className="pc-asset-preview">
        {item.asset.media_kind === 'image' && item.asset.preview_url ? <img src={item.asset.preview_url} alt={item.asset.display_name ?? item.asset.asset_ref.asset_id}/> : <AssetIcon kind={item.asset.media_kind}/>}
        <span>{item.asset.role === 'input' ? '输入' : '输出'}</span>
      </div>
      <div className="pc-asset-body"><b>{item.asset.display_name ?? item.asset.asset_ref.asset_id}</b><small>AssetVersion {item.asset.asset_ref.version} · {item.asset.availability}</small>
        <div className="pc-asset-meta"><span>{item.asset.mime_type ?? '类型未知'}</span><span>{item.asset.width_pixels && item.asset.height_pixels ? `${item.asset.width_pixels} × ${item.asset.height_pixels}` : item.asset.duration_ms !== null ? `${(item.asset.duration_ms / 1000).toFixed(1)} 秒` : '尺寸未知'}</span></div>
        <div className="pc-lineage-links">用于 {item.used_by_runs.length} 个制作任务{item.used_by_runs.slice(0, 3).map(ref => <button key={`${ref.source}:${ref.id}`} onClick={() => onOpenRun(ref)}>{ref.id}<ExternalLink size={11}/></button>)}</div>
      </div>
    </article>)}
  </div>
}
