import type { CommercePrerollState } from './types'
const sessionVersion = 2

export function commercePrerollSessionKey(projectId: string) {
  return `cookies.commerce-preroll-v2:${projectId}`
}

export function readCommercePrerollSession(projectId: string): CommercePrerollState | null {
  try {
    const raw = localStorage.getItem(commercePrerollSessionKey(projectId))
    if (!raw) return null
    const parsed = JSON.parse(raw) as CommercePrerollState
    if (parsed.sessionVersion !== sessionVersion || !parsed.clientTaskId) return null
    const restored: CommercePrerollState = {
      ...parsed,
      analysisStatus: parsed.analysisStatus === 'error' ? (parsed.analysis ? 'ready' : 'idle') : parsed.analysisStatus,
      hooksStatus: parsed.hooksStatus === 'error' ? (parsed.hooks.length ? 'ready' : 'idle') : parsed.hooksStatus,
      firstFramesStatus: parsed.firstFramesStatus === 'error' ? (parsed.firstFrames.length ? 'ready' : 'idle') : parsed.firstFramesStatus,
      videoStatus: parsed.videoStatus === 'error' ? (parsed.output ? 'ready' : 'idle') : parsed.videoStatus,
      error: '',
      errorScope: null,
    }
    if (restored.source?.uploaded && !restored.source.assetId) return { ...restored, activeStep: 'source', source: null, rightsConfirmed: false, error: '尚未完成上传的视频需要重新选择，其他草稿信息已保留。', errorScope: 'analysis' }
    return restored
  } catch {
    return null
  }
}

export function writeCommercePrerollSession(projectId: string, state: CommercePrerollState) {
  const stored = { ...state, lastSavedAt: new Date().toISOString() }
  localStorage.setItem(commercePrerollSessionKey(projectId), JSON.stringify(stored))
}

export function clearCommercePrerollSession(projectId: string) {
  localStorage.removeItem(commercePrerollSessionKey(projectId))
}
