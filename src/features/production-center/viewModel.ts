import type { ProductionRunQuery } from './types'

export const productionViews = ['图片生成', '视频生成', '音频生成', '渲染队列', '源素材', '失败任务'] as const
export type ProductionView = typeof productionViews[number]

export function productionQueryForView(view: string): ProductionRunQuery | null {
  switch (view) {
    case '图片生成': return { media_kind: 'image', limit: 50 }
    case '视频生成': return { media_kind: 'video', limit: 50 }
    case '音频生成': return { media_kind: 'audio', limit: 50 }
    case '渲染队列': return { media_kind: 'render', limit: 50 }
    case '失败任务': return { status: ['failed', 'expired', 'partially_succeeded'], limit: 50 }
    case '源素材': return null
    default: return { media_kind: 'image', limit: 50 }
  }
}

export const activeProductionStatuses = new Set(['queued', 'running', 'ingesting'])
