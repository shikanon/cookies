import { useEffect } from 'react'
import { Shell } from './components/Shell'
import { HomePage, ModulePage } from './components/Pages'
import { ProjectFlowDashboard } from './components/ProjectWorkflow'
import { ProjectManagementPage } from './components/ProjectManagementPage'
import { ModelSettingsPage } from './components/ModelSettingsPage'
import { LoginPage } from './components/LoginPage'
import { StateBoundary } from './components/StateBoundary'
import { useAuth } from './context/AuthContext'
import { useProject } from './context/ProjectContext'
import { systems } from './data/navigation'
import { projectHomePath, projectManagePath, projectPath, useAppRoute } from './lib/router'
import type { SystemKey } from './types'
import { getLatestDeliveryTourRunId } from './components/DeliveryTourPage'

export default function App() {
  const { route, navigate } = useAppRoute()
  const { session, isLoading: isAuthLoading } = useAuth()
  const { currentProject, isLoading, reloadProjects, routeDiagnostic, selectProject, targetProjectId } = useProject()
  const system = systems.find(item => item.key === route.systemKey) ?? systems[0]
  const canonicalNavId = route.systemKey === 'delivery' && route.navId === 'three-tier' ? 'configuration' : route.navId
  const navItem = system.nav.find(item => item.id === canonicalNavId) ?? system.nav[0]

  useEffect(() => {
    if (route.systemKey === 'delivery' && route.navId === 'three-tier' && route.projectId) {
      navigate(projectPath(route.projectId, 'delivery', 'configuration', route.objectId, route.view, route.contextId, route.tourRunId, route.tourCase), true)
    }
  }, [navigate, route])

  useEffect(() => {
    if (route.projectId) selectProject(route.projectId)
  }, [route.projectId, selectProject])

  useEffect(() => {
    if (!route.isLegacyProjectSystemRoute || isLoading || !currentProject.id) return
    navigate(projectPath(currentProject.id, route.systemKey, route.navId, route.objectId, route.view), true)
  }, [currentProject.id, isLoading, navigate, route])

  useEffect(() => {
    if (!route.projectId || route.isHome || route.isProjectHome || route.isProjectManagement || route.isModelSettings) return
    rememberProjectSystemPath(route.projectId, route.systemKey, projectPath(route.projectId, route.systemKey, route.navId, route.objectId, route.view, route.contextId, route.tourRunId, route.tourCase))
  }, [route])

  if (isAuthLoading) return <div className="login-page"><div className="page-notice">正在检查登录状态…</div></div>
  if (!session.authenticated) return <LoginPage/>

  const systemLanding: Record<SystemKey, string> = { strategy: 'tasks', creative: 'tasks', insight: 'prelaunch', delivery: 'plans' }
  const activeProjectId = route.projectId ?? currentProject.id
  const changeSystem = (next: SystemKey) => navigate(projectPath(activeProjectId, next, systemLanding[next]))
  const openProject = (projectId: string, next?: SystemKey, navId?: string, objectId?: string, view?: string, contextId?: string, tourRunId?: string, tourCase?: string) => {
    selectProject(projectId)
    const rememberedPath = next && !navId ? getRememberedProjectSystemPath(projectId, next) : undefined
    navigate(next ? rememberedPath ?? projectPath(projectId, next, navId ?? systemLanding[next], objectId, view, contextId, tourRunId, tourCase) : projectHomePath(projectId))
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
    : <ModulePage key={`${currentProject.id}-${system.key}-${navItem.id}`} system={system} item={navItem} contextId={route.contextId} objectId={route.objectId} routeView={route.view} tourRunId={route.tourRunId} tourCase={route.tourCase} onOpenProject={openProject}/>

  const changeNavigation = (id: string) => {
    const runId = system.key === 'delivery' ? route.tourRunId ?? getLatestDeliveryTourRunId(activeProjectId) : undefined
    navigate(projectPath(activeProjectId, system.key, id, undefined, undefined, undefined, runId, runId ? route.tourCase : undefined))
  }

  return <Shell system={system} activeNav={navItem.id} isHome={route.isHome} isProjectHome={route.isProjectHome} isProjectManagement={route.isProjectManagement} isGlobalSettings={route.isModelSettings} onHome={() => navigate('/')} onModelSettings={() => navigate('/settings')} onSystemChange={changeSystem} onProjectChange={openProject} onProjectManage={manageProject} onNavChange={changeNavigation}>
    {content}
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
