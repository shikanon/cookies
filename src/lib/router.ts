import { useCallback, useEffect, useState } from 'react'
import type { SystemKey } from '../types'

export interface AppRoute {
  isHome: boolean
  isProjectHome: boolean
  isProjectManagement: boolean
  isModelSettings: boolean
  isLegacyProjectSystemRoute: boolean
  isLegacyVideoEditingRoute?: boolean
  projectId?: string
  systemKey: SystemKey
  navId: string
  objectId?: string
  contextId?: string
  view?: string
  tourRunId?: string
  tourCase?: string
}

const systemKeys = new Set<SystemKey>(['strategy', 'creative', 'insight', 'delivery'])

export function parseRoute(location = `${window.location.pathname}${window.location.search}`): AppRoute {
  const origin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin
  const url = new URL(location, origin)
  const parts = url.pathname.split('/').filter(Boolean)
  if (parts[0] === 'settings') return { isHome: false, isProjectHome: false, isProjectManagement: false, isModelSettings: true, isLegacyProjectSystemRoute: false, systemKey: 'strategy', navId: 'tasks' }
  if (parts[0] !== 'projects' || !parts[1]) return { isHome: true, isProjectHome: false, isProjectManagement: false, isModelSettings: false, isLegacyProjectSystemRoute: false, systemKey: 'strategy', navId: 'tasks' }
  if (systemKeys.has(parts[1] as SystemKey)) {
    const systemKey = parts[1] as SystemKey
    return { isHome: false, isProjectHome: false, isProjectManagement: false, isModelSettings: false, isLegacyProjectSystemRoute: true, systemKey, navId: parts[2] || defaultNavForSystem(systemKey), objectId: parts[3], view: url.searchParams.get('view') ?? undefined }
  }
  if (!parts[2] || parts[2] === 'home') return { isHome: false, isProjectHome: true, isProjectManagement: false, isModelSettings: false, isLegacyProjectSystemRoute: false, projectId: parts[1], systemKey: 'strategy', navId: 'tasks' }
  if (parts[2] === 'manage') return { isHome: false, isProjectHome: false, isProjectManagement: true, isModelSettings: false, isLegacyProjectSystemRoute: false, projectId: parts[1], systemKey: 'strategy', navId: 'tasks' }
  if (parts[2] === 'assets') {
    return { isHome: false, isProjectHome: false, isProjectManagement: false, isModelSettings: false, isLegacyProjectSystemRoute: false, projectId: parts[1], systemKey: 'creative', navId: 'assets', objectId: parts[3], view: url.searchParams.get('view') ?? undefined }
  }
  if (parts[2] === 'provider-jobs') {
    return { isHome: false, isProjectHome: false, isProjectManagement: false, isModelSettings: false, isLegacyProjectSystemRoute: false, projectId: parts[1], systemKey: 'creative', navId: 'production', view: url.searchParams.get('view') ?? undefined }
  }
  if (parts[2] === 'creative' && parts[3] === 'video' && parts[4] === 'editing') {
    return {
      isHome: false,
      isProjectHome: false,
      isProjectManagement: false,
      isModelSettings: false,
      isLegacyProjectSystemRoute: false,
      isLegacyVideoEditingRoute: false,
      projectId: decodeURIComponent(parts[1]),
      systemKey: 'creative',
      navId: 'video',
      contextId: parts[5] ? decodeURIComponent(parts[5]) : undefined,
      view: '素材剪辑',
    }
  }
  const normalizedSystem = parts[2] === 'insights' ? 'insight' : parts[2]
  const systemKey = systemKeys.has(normalizedSystem as SystemKey) ? normalizedSystem as SystemKey : 'strategy'
  const view = url.searchParams.get('view') ?? undefined
  const contextId = url.searchParams.get('context') ?? undefined
  return {
    isHome: false,
    isProjectHome: false,
    isProjectManagement: false,
    isModelSettings: false,
    isLegacyProjectSystemRoute: false,
    projectId: parts[1],
    systemKey,
    navId: parts[3] || 'tasks',
    objectId: parts[4],
    contextId,
    view,
    isLegacyVideoEditingRoute: systemKey === 'creative' && (parts[3] || 'tasks') === 'video' && view === '素材剪辑',
    tourRunId: url.searchParams.get('tour_run_id') ?? undefined,
    tourCase: url.searchParams.get('tour_case') ?? undefined,
  }
}

export function videoEditingPath(projectId: string, editTaskId?: string) {
  const base = `/projects/${encodeURIComponent(projectId)}/creative/video/editing`
  return editTaskId ? `${base}/${encodeURIComponent(editTaskId)}` : base
}

function defaultNavForSystem(systemKey: SystemKey) {
  if (systemKey === 'insight') return 'prelaunch'
  if (systemKey === 'delivery') return 'plans'
  return 'tasks'
}

export function projectHomePath(projectId: string) {
  return `/projects/${projectId}/home`
}

export function projectManagePath(projectId: string) {
  return `/projects/${projectId}/manage`
}

export function projectPath(projectId: string, systemKey: SystemKey, navId: string, objectId?: string, view?: string, contextId?: string, tourRunId?: string, tourCase?: string) {
  const path = `/projects/${projectId}/${systemKey}/${navId}${objectId ? `/${objectId}` : ''}`
  const search = new URLSearchParams()
  if (view) search.set('view', view)
  if (contextId) search.set('context', contextId)
  if (tourRunId) search.set('tour_run_id', tourRunId)
  if (tourCase) search.set('tour_case', tourCase)
  return search.size ? `${path}?${search.toString()}` : path
}

export function useAppRoute() {
  const [route, setRoute] = useState<AppRoute>(() => parseRoute())
  useEffect(() => {
    const sync = () => setRoute(parseRoute())
    window.addEventListener('popstate', sync)
    return () => window.removeEventListener('popstate', sync)
  }, [])
  const navigate = useCallback((path: string, replace = false) => {
    window.history[replace ? 'replaceState' : 'pushState']({}, '', path)
    setRoute(parseRoute(path))
    window.scrollTo({ top: 0 })
  }, [])
  return { route, navigate }
}
