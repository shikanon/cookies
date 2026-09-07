import { lazy, Suspense, useEffect } from 'react'
import { Shell } from './components/Shell'
import { ProjectFlowDashboard } from './components/ProjectWorkflow'
import { ProjectManagementPage } from './components/ProjectManagementPage'
import { LoginPage } from './components/LoginPage'
import { RenderErrorBoundary } from './components/RenderErrorBoundary'
import { StateBoundary } from './components/StateBoundary'
import { useAuth } from './context/AuthContext'
import { useProject } from './context/ProjectContext'
import { systems } from './data/navigation'
import { projectHomePath, projectManagePath, projectPath, useAppRoute, videoEditingPath } from './lib/router'
import {
  strategyWorkspacePath,
  type StrategyWorkspaceLocation,
} from './features/strategy/workspace/workspaceRoute'
import type { SystemKey } from './types'

const loadPages = () => import('./components/Pages')
const HomePage = lazy(() => loadPages().then(module => ({ default: module.HomePage })))
const ModulePage = lazy(() => loadPages().then(module => ({ default: module.ModulePage })))
const ModelSettingsPage = lazy(() => import('./components/ModelSettingsPage').then(module => ({ default: module.ModelSettingsPage })))

export default function App() {
  const { route, navigate } = useAppRoute()
  const { session, isLoading: isAuthLoading } = useAuth()
  const { currentProject, isLoading, reloadProjects, routeDiagnostic, selectProject, targetProjectId } = useProject()
  const system = systems.find(item => item.key === route.systemKey) ?? systems[0]
  const canonicalNavId = route.systemKey === 'delivery' && route.navId === 'three-tier' ? 'configuration' : route.navId
  const navItem = system.nav.find(item => item.id === canonicalNavId) ?? system.nav[0]

  useEffect(() => {
    if (route.systemKey === 'delivery' && route.navId === 'three-tier' && route.projectId) {
      navigate(projectPath(route.projectId, 'delivery', 'configuration', route.objectId, route.view, route.contextId), true)
    }
  }, [navigate, route])

  if (session.authenticated && route.systemKey === 'strategy' && route.navId === 'workspaces') {
    // Route intent is already authoritative here. Start both split points now
    // so the generic module shell and the workspace do not form a waterfall.
    void loadPages()
    void import('./features/strategy/workspace/StrategyWorkspaceRoute')
  }

  useEffect(() => {
    if (route.projectId) selectProject(route.projectId)
  }, [route.projectId, selectProject])

  useEffect(() => {
    if (!route.isLegacyProjectSystemRoute || isLoading || !currentProject.id) return
    navigate(projectPath(currentProject.id, route.systemKey, route.navId, route.objectId, route.view), true)
  }, [currentProject.id, isLoading, navigate, route])

  useEffect(() => {
    if (
      route.systemKey !== 'strategy' || route.navId !== 'workspaces' ||
      !route.projectId || !route.objectId || !route.strategyStage ||
      !route.strategyNeedsCanonicalRedirect
    ) return
    navigate(strategyWorkspacePath(route.projectId, route.objectId, {
      stage: route.strategyStage,
      panel: route.strategyPanel,
      resource: route.strategyResource,
    }), true, true)
  }, [navigate, route.navId, route.objectId, route.projectId, route.strategyNeedsCanonicalRedirect, route.strategyPanel, route.strategyResource, route.strategyStage, route.systemKey])

  useEffect(() => {
    if (!route.isLegacyVideoEditingRoute || !route.projectId) return
    navigate(videoEditingPath(route.projectId, route.contextId), true)
  }, [navigate, route.contextId, route.isLegacyVideoEditingRoute, route.projectId])

  useEffect(() => {
    if (!route.projectId || route.isHome || route.isProjectHome || route.isProjectManagement || route.isModelSettings) return
    const path = route.systemKey === 'strategy' && route.navId === 'workspaces' && route.objectId && route.strategyStage
      ? strategyWorkspacePath(route.projectId, route.objectId, {
          stage: route.strategyStage,
          panel: route.strategyPanel,
          resource: route.strategyResource,
        })
      : projectPath(route.projectId, route.systemKey, route.navId, route.objectId, route.view, route.contextId)
    rememberProjectSystemPath(route.projectId, route.systemKey, path)
  }, [route])

  if (isAuthLoading) return <div className="login-page"><div className="page-notice">正在检查登录状态…</div></div>
  if (!session.authenticated) return <LoginPage/>

  const systemLanding: Record<SystemKey, string> = { strategy: 'tasks', creative: 'tasks', insight: 'analysis', delivery: 'plans' }
  const activeProjectId = route.projectId ?? currentProject.id
  const changeSystem = (next: SystemKey) => navigate(projectPath(activeProjectId, next, systemLanding[next]))
  const openProject = (projectId: string, next?: SystemKey, navId?: string, objectId?: string, view?: string, contextId?: string) => {
    selectProject(projectId)
    if (next === 'creative' && navId === 'video' && view === '素材剪辑') {
      navigate(videoEditingPath(projectId, contextId))
      return
    }
    const rememberedPath = next && !navId ? getRememberedProjectSystemPath(projectId, next) : undefined
    navigate(next ? rememberedPath ?? projectPath(projectId, next, navId ?? systemLanding[next], objectId, view, contextId) : projectHomePath(projectId))
  }

  const openStrategyWorkspace = (projectId: string, workspaceId: string, location: StrategyWorkspaceLocation, replace = false) => {
    selectProject(projectId)
    navigate(strategyWorkspacePath(projectId, workspaceId, location), replace, true)
  }

  const manageProject = (projectId: string) => {
    selectProject(projectId)
    navigate(projectManagePath(projectId))
  }
  const routeNeedsProject = Boolean(route.projectId && !route.isHome && !route.isModelSettings)
  const routeProjectReady = !route.projectId || currentProject.id === route.projectId
  const projectRouteState = isLoading || targetProjectId !== route.projectId ? 'loading' : 'error'
  const content = route.isModelSettings ? <ModelSettingsPage/>
    : route.isHome ? <HomePage onSystemChange={changeSystem} onOpenProject={openProject} onManageProject={manageProject}/>
    : route.isLegacyProjectSystemRoute ? <ProjectRouteBoundary targetProjectId="默认 Project" diagnostic={`旧式模块路由 ${route.systemKey} 将在 Project 加载后自动跳转。`} state={isLoading || currentProject.id ? 'loading' : 'error'} onRetry={() => { void reloadProjects() }}/>
    : routeNeedsProject && !routeProjectReady ? <ProjectRouteBoundary targetProjectId={route.projectId!} diagnostic={routeDiagnostic} state={projectRouteState} onRetry={() => { void reloadProjects(route.projectId) }}/>
    : route.isProjectHome ? <ProjectFlowDashboard onOpenProject={openProject} onManageProject={manageProject}/>
    : route.isProjectManagement ? <ProjectManagementPage onOpenWorkbench={id => openProject(id)} onOpenProject={openProject}/>
    : <ModulePage
        key={`${currentProject.id}-${system.key}-${navItem.id}`}
        system={system}
        item={navItem}
        contextId={route.contextId}
        objectId={route.objectId}
        routeView={route.view}
        strategyStage={route.strategyStage}
        strategyPanel={route.strategyPanel}
        strategyResource={route.strategyResource}
        onOpenProject={openProject}
        onOpenStrategyWorkspace={openStrategyWorkspace}
      />

  const changeNavigation = (id: string) => {
    navigate(projectPath(activeProjectId, system.key, id))
  }

  return <Shell system={system} activeNav={navItem.id} isHome={route.isHome} isProjectHome={route.isProjectHome} isProjectManagement={route.isProjectManagement} isGlobalSettings={route.isModelSettings} onHome={() => navigate('/')} onModelSettings={() => navigate('/settings')} onSystemChange={changeSystem} onProjectChange={openProject} onProjectManage={manageProject} onNavChange={changeNavigation}>
    {/* 页面级错误边界：某一页渲染炸了，只让它那一块显示成错误，Shell 的导航、
        Project 切换、系统切换都还能用。切换路由时 resetKey 变化会自动清掉错误。
        它套在 Suspense 外面：这样懒加载本身失败（chunk 拉不下来）也归它管，
        而不是把整个 App 白屏。 */}
    <RenderErrorBoundary contextLabel={navItem.label} resetKey={`${activeProjectId}-${system.key}-${navItem.id}-${route.view ?? ''}`}>
      <Suspense fallback={<div className="page-notice" role="status">正在加载当前工作区…</div>}>
        {content}
      </Suspense>
    </RenderErrorBoundary>
  </Shell>
}

const recentProjectSystemPathKey = 'cookies.project-system-paths.v1'

function rememberProjectSystemPath(projectId: string, systemKey: SystemKey, path: string) {
  try {
    const current = readRememberedProjectSystemPaths()
    current[`${projectId}:${systemKey}`] = path
    window.localStorage.setItem(recentProjectSystemPathKey, JSON.stringify(current))
  } catch {
    // Navigation history is a convenience feature; storage failures should not block routing.
  }
}

function getRememberedProjectSystemPath(projectId: string, systemKey: SystemKey): string | undefined {
  try {
    return readRememberedProjectSystemPaths()[`${projectId}:${systemKey}`]
  } catch {
    return undefined
  }
}

function readRememberedProjectSystemPaths(): Record<string, string> {
  const raw = window.localStorage.getItem(recentProjectSystemPathKey)
  if (!raw) return {}
  const parsed = JSON.parse(raw) as unknown
  return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
    ? parsed as Record<string, string>
    : {}
}

function ProjectRouteBoundary({ targetProjectId, diagnostic, state, onRetry }: { targetProjectId: string; diagnostic: string | null; state: 'loading' | 'error'; onRetry: () => void }) {
  return <div className="module-page page-frame layout-workspace">
    <div className="page-notice" role="status">正在加载路由目标 Project：{targetProjectId}{diagnostic ? `。${diagnostic}` : ''}</div>
    <StateBoundary state={state} onRetry={onRetry}>
      <span/>
    </StateBoundary>
  </div>
}
