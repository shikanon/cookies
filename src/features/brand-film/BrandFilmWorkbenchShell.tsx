import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Check, ChevronRight, FileText, Info, LockKeyhole, X } from 'lucide-react'
import type { BrandFilmStageId, BrandFilmStageState } from './stage'

export type BrandFilmContextAsset = {
  id: string
  label: string
  preview?: string
  source: string
  confirmed: boolean
}

type Props = {
  productName: string
  briefName: string
  sourceLabel: string
  specification: string
  revision: number
  serverStage: string
  stages: BrandFilmStageState[]
  activeStage: BrandFilmStageId
  busy: boolean
  assets: BrandFilmContextAsset[]
  headerActions?: ReactNode
  onStageChange: (stage: BrandFilmStageId) => void
  children: ReactNode
}

export function BrandFilmWorkbenchShell({
  productName,
  briefName,
  sourceLabel,
  specification,
  revision,
  serverStage,
  stages,
  activeStage,
  busy,
  assets,
  headerActions,
  onStageChange,
  children,
}: Props) {
  const [drawerOpen, setDrawerOpen] = useState(false)
  const drawerRef = useRef<HTMLElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const active = stages.find(stage => stage.id === activeStage) ?? stages[0]

  useEffect(() => {
    if (!drawerOpen) return
    drawerRef.current?.focus()
    const close = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setDrawerOpen(false)
      triggerRef.current?.focus()
    }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [drawerOpen])

  return <div className="brand-film-workbench">
    <header className="brand-film-workbench-header">
      <div className="brand-film-workbench-title">
        <span>品牌广告</span>
        <h2>{productName}</h2>
        <p>{active.description}</p>
      </div>
      <div className="brand-film-workbench-meta">
        {headerActions}
        <span className={busy ? 'brand-save-state saving' : 'brand-save-state'}>{busy ? '正在处理…' : `已同步 · r${revision}`}</span>
        <button ref={triggerRef} className="secondary-button" type="button" aria-haspopup="dialog" aria-expanded={drawerOpen} onClick={() => setDrawerOpen(true)}><Info size={14}/>资料与来源</button>
      </div>
    </header>

    <nav className="brand-film-stage-nav" aria-label="品牌广告制作阶段">
      <ol>
        {stages.map(stage => <li key={stage.id} className={stage.id === activeStage ? 'active' : stage.complete ? 'complete' : stage.accessible ? 'available' : 'locked'}>
          <button
            type="button"
            aria-current={stage.id === activeStage ? 'step' : undefined}
            aria-disabled={!stage.accessible}
            title={stage.lockedReason}
            onClick={() => { if (stage.accessible) onStageChange(stage.id) }}
          >
            <span className="brand-film-stage-number">{stage.complete ? <Check size={14}/> : stage.accessible ? String(stage.order).padStart(2, '0') : <LockKeyhole size={13}/>}</span>
            <span><b>{stage.label}</b><small>{stage.id === activeStage ? stage.description : stage.lockedReason ?? (stage.complete ? '已完成，可返回查看' : '可继续')}</small></span>
            {stage.id !== stages.at(-1)?.id ? <ChevronRight className="brand-film-stage-chevron" size={14}/> : null}
          </button>
        </li>)}
      </ol>
    </nav>

    <section className="brand-film-stage-content" id={`brand-film-stage-${activeStage}`} role="region" aria-label={active.label}>{children}</section>

    {drawerOpen ? <div className="brand-film-context-layer" onMouseDown={event => { if (event.target === event.currentTarget) setDrawerOpen(false) }}>
      <aside ref={drawerRef} className="brand-film-context-drawer" role="dialog" aria-modal="false" aria-labelledby="brand-film-context-title" tabIndex={-1}>
        <header><div><h3 id="brand-film-context-title">资料与来源</h3><p>当前制作任务使用的 Brief、规格、版本和参考素材。</p></div><button type="button" aria-label="关闭资料与来源" onClick={() => { setDrawerOpen(false); triggerRef.current?.focus() }}><X size={18}/></button></header>
        <dl className="brand-film-context-facts">
          <div><dt>Brief</dt><dd>{briefName}</dd></div>
          <div><dt>来源</dt><dd>{sourceLabel}</dd></div>
          <div><dt>制作规格</dt><dd>{specification}</dd></div>
          <div><dt>服务端阶段</dt><dd>{serverStage}</dd></div>
          <div><dt>当前修订</dt><dd>r{revision}</dd></div>
        </dl>
        <section className="brand-film-context-assets"><h4>商品与品牌参考素材</h4>{assets.length ? assets.map(asset => <article key={asset.id}>
          <div className="brand-film-context-thumb">{asset.preview ? <img src={asset.preview} alt=""/> : <FileText size={18}/>}</div>
          <div><b>{asset.label}</b><small>{asset.source}</small></div>
          <span className={asset.confirmed ? 'confirmed' : ''}>{asset.confirmed ? '已确认' : '待确认'}</span>
        </article>) : <p>解析 Brief 后将在这里显示商品与 Logo。</p>}</section>
      </aside>
    </div> : null}
  </div>
}
