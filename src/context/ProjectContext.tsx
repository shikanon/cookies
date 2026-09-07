import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { api, type ApiAgencyWorkbench, type ApiArtifact, type ApiBusinessTask, type ApiBusinessTaskType, type ApiGenerationJob, type ApiOperationalRecord, type ApiProject } from '../data/api'
import type { ArtifactKey, ArtifactStatus, BusinessTaskRecord, ChangeSetRecord, ProjectArtifact, ProjectRecord } from '../types'
import type { DeliveryChangeSet } from '../api/delivery'
import { presentCreativeStatus } from '../lib/media-status'
import { useAuth } from './AuthContext'

interface ProjectContextValue {
  projects: ProjectRecord[]
  currentProject: ProjectRecord
  agencyWorkbench: ApiAgencyWorkbench | null
  targetProjectId: string
  loadedProjectId: string
  isLoading: boolean
  error: string | null
  routeDiagnostic: string | null
  reloadProjects: (expectedProjectId?: string) => Promise<void>
  selectProject: (id: string) => void
  createProject: (input: Pick<ProjectRecord, 'name' | 'brand' | 'goal' | 'industry'>) => Promise<ProjectRecord>
  updateProject: (input: Pick<ProjectRecord, 'name' | 'brand' | 'goal'>) => Promise<void>
  createTask: (input: { type: ApiBusinessTaskType; name: string; objective: string }) => Promise<BusinessTaskRecord>
  updateTask: (id: string, patch: Partial<Pick<BusinessTaskRecord, 'name' | 'objective' | 'status' | 'sourceTaskIds' | 'sourceArtifactIds' | 'outputArtifactIds'>>) => Promise<BusinessTaskRecord>
  advanceArtifact: (key: ArtifactKey, status: ProjectRecord['artifacts'][ArtifactKey]['status']) => Promise<void>
  updateArtifact: (key: ArtifactKey, patch: Partial<ProjectRecord['artifacts'][ArtifactKey]>) => Promise<void>
}

const ProjectContext = createContext<ProjectContextValue | null>(null)

export function ProjectProvider({ children }: { children: ReactNode }) {
  const { session, isLoading: isAuthLoading } = useAuth()
  const [projects, setProjects] = useState<ProjectRecord[]>([])
  const [agencyWorkbench, setAgencyWorkbench] = useState<ApiAgencyWorkbench | null>(null)
  const [targetProjectId, setTargetProjectId] = useState('')
  const [loadedProjectId, setLoadedProjectId] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [routeDiagnostic, setRouteDiagnostic] = useState<string | null>(null)
  const projectsRef = useRef<ProjectRecord[]>([])
  const targetProjectIdRef = useRef('')
  const loadedProjectIdRef = useRef('')
  const reloadRequestRef = useRef(0)
  const currentProject = projects.find(project => project.id === loadedProjectId) ?? emptyProject

  useEffect(() => { projectsRef.current = projects }, [projects])

  const reloadProjects = useCallback(async (expectedProjectId?: string) => {
    const requestId = reloadRequestRef.current + 1
    reloadRequestRef.current = requestId
    setIsLoading(true)
    try {
      const apiProjects = await api.listProjects()
      const requestedProjectId = expectedProjectId || targetProjectIdRef.current
      const activeProjectId = requestedProjectId && apiProjects.some(project => project.id === requestedProjectId)
        ? requestedProjectId
        : !requestedProjectId
          ? apiProjects[0]?.id ?? ''
          : ''
      const activeProject = apiProjects.find(project => project.id === activeProjectId)
      const [activeSnapshot, workbench] = activeProject
        ? await Promise.all([
            api.getProjectSnapshot(activeProject.id),
            api.listAgencyWorkbench({ projectIds: [activeProject.id] }),
          ])
        : [null, await api.listAgencyWorkbench({ projectIds: [] })]
      const existingProjects = new Map(projectsRef.current.map(project => [project.id, project]))
      const productsByProjectId = new Map(workbench.projects.map(project => [project.id, project.products ?? []]))
      const nextProjects = apiProjects.map(project => {
        if (activeSnapshot?.project.id === project.id) {
          return toProjectRecord(withProjectProducts(activeSnapshot.project, productsByProjectId.get(project.id)), activeSnapshot.artifacts, activeSnapshot.jobs, activeSnapshot.tasks, activeSnapshot.changeSets, activeSnapshot.operations)
        }
        return existingProjects.get(project.id) ?? toProjectRecord(withProjectProducts(project, productsByProjectId.get(project.id)))
      })
      if (reloadRequestRef.current !== requestId) {
        setRouteDiagnostic(`已忽略过期的 Project 加载响应，当前路由目标为 ${targetProjectIdRef.current || '未选择 Project'}。`)
        return
      }
      if (expectedProjectId && targetProjectIdRef.current !== expectedProjectId) {
        setRouteDiagnostic(`已忽略 Project ${expectedProjectId} 的过期响应，当前路由目标为 ${targetProjectIdRef.current || '未选择 Project'}。`)
        return
      }
      setProjects(nextProjects)
      setAgencyWorkbench(workbench)
      projectsRef.current = nextProjects
      const activeTargetId = targetProjectIdRef.current
      const nextLoadedProjectId = activeTargetId && nextProjects.some(project => project.id === activeTargetId)
        ? activeTargetId
        : !activeTargetId && nextProjects.length
          ? nextProjects[0].id
          : ''
      if (!activeTargetId && nextLoadedProjectId) {
        targetProjectIdRef.current = nextLoadedProjectId
        setTargetProjectId(nextLoadedProjectId)
      }
      loadedProjectIdRef.current = nextLoadedProjectId
      setLoadedProjectId(nextLoadedProjectId)
      setRouteDiagnostic(activeTargetId && !nextLoadedProjectId ? `路由目标 Project ${activeTargetId} 未在服务端返回结果中找到。` : null)
      setError(null)

      const remainingProjects = apiProjects.filter(project => project.id !== activeProjectId)
      void (async () => {
        for (let offset = 0; offset < remainingProjects.length; offset += 2) {
          const batch = remainingProjects.slice(offset, offset + 2)
          const hydrated = await Promise.all(batch.map(async project => {
            try {
              const [snapshot, projectWorkbench] = await Promise.all([
                api.getProjectSnapshot(project.id),
                api.listAgencyWorkbench({ projectIds: [project.id] }),
              ])
              const projectProducts = projectWorkbench.projects.find(candidate => candidate.id === project.id)?.products
              return {
                project: toProjectRecord(withProjectProducts(snapshot.project, projectProducts), snapshot.artifacts, snapshot.jobs, snapshot.tasks, snapshot.changeSets, snapshot.operations),
                workbench: projectWorkbench,
              }
            } catch {
              return null
            }
          }))
          if (reloadRequestRef.current !== requestId) return
          const successful = hydrated.filter((value): value is NonNullable<typeof value> => value !== null)
          if (!successful.length) continue
          const records = new Map(successful.map(value => [value.project.id, value.project]))
          setProjects(current => {
            const next = current.map(project => records.get(project.id) ?? project)
            projectsRef.current = next
            return next
          })
          setAgencyWorkbench(current => successful.reduce(
            (merged, value) => mergeAgencyWorkbench(merged, value.workbench),
            current ?? emptyAgencyWorkbenchValue,
          ))
        }
      })()
    } catch (cause) {
      if (reloadRequestRef.current !== requestId) {
        setRouteDiagnostic(`已忽略过期的 Project 加载失败响应，当前路由目标为 ${targetProjectIdRef.current || '未选择 Project'}。`)
        return
      }
      setError(cause instanceof Error ? cause.message : '加载项目失败')
    } finally {
      if (reloadRequestRef.current === requestId) setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    if (isAuthLoading) return
    if (!session.authenticated) {
      reloadRequestRef.current += 1
      projectsRef.current = []
      targetProjectIdRef.current = ''
      loadedProjectIdRef.current = ''
      setProjects([])
      setAgencyWorkbench(null)
      setTargetProjectId('')
      setLoadedProjectId('')
      setError(null)
      setRouteDiagnostic(null)
      setIsLoading(false)
      return
    }
    void reloadProjects()
  }, [isAuthLoading, reloadProjects, session.authenticated])

  const selectProject = useCallback((id: string) => {
    targetProjectIdRef.current = id
    setTargetProjectId(id)
    setRouteDiagnostic(null)
    if (loadedProjectIdRef.current === id) {
      setIsLoading(false)
      return
    }
    loadedProjectIdRef.current = ''
    setLoadedProjectId('')
    setIsLoading(true)
    void reloadProjects(id)
  }, [reloadProjects])

  const createProject = useCallback(async (input: Pick<ProjectRecord, 'name' | 'brand' | 'goal' | 'industry'>) => {
    const created = toProjectRecord(await api.createProject({ name: input.name, brand: input.brand || '未指定品牌', objective: input.goal, industry: input.industry }))
    setProjects(current => {
      const nextProjects = [created, ...current]
      projectsRef.current = nextProjects
      return nextProjects
    })
    targetProjectIdRef.current = created.id
    loadedProjectIdRef.current = created.id
    setTargetProjectId(created.id)
    setLoadedProjectId(created.id)
    setRouteDiagnostic(null)
    return created
  }, [])

  const updateProject = useCallback(async (input: Pick<ProjectRecord, 'name' | 'brand' | 'goal'>) => {
    const project = projects.find(candidate => candidate.id === loadedProjectId)
    if (!project) throw new Error('请先选择已保存的 Project。')
    await api.updateProject(project.id, { name: input.name, brand: input.brand, objective: input.goal, expectedContextVersion: project.version })
    await reloadProjects()
  }, [loadedProjectId, projects, reloadProjects])

  const createTask = useCallback(async (input: { type: ApiBusinessTaskType; name: string; objective: string }) => {
    const project = projects.find(candidate => candidate.id === loadedProjectId)
    if (!project) throw new Error('请先选择已保存的 Project。')
    const artifacts = await api.listArtifacts(project.id)
    const insightReference = artifacts
      .filter(artifact => artifact.status === 'ready' && (
        artifact.content.startsWith('[prelaunch-insight]')
        || artifact.content.startsWith('[knowledge]')
      ))
      .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))[0]
    const sourceArtifactIds = taskSourceArtifactIds(project, input.type, insightReference?.id)
    const task = await api.createTask({
      projectId: project.id,
      ...input,
      sourceTaskIds: taskSourceTaskIds(project, input.type),
      sourceArtifactIds,
    })
    await reloadProjects()
    return task
  }, [loadedProjectId, projects, reloadProjects])

  const updateTask = useCallback(async (
    id: string,
    patch: Partial<Pick<BusinessTaskRecord, 'name' | 'objective' | 'status' | 'sourceTaskIds' | 'sourceArtifactIds' | 'outputArtifactIds'>>,
  ) => {
    const project = projects.find(candidate => candidate.id === loadedProjectId)
    if (!project) throw new Error('请先选择已保存的 Project。')
    const task = await api.updateTask(project.id, id, patch)
    await reloadProjects()
    return task
  }, [reloadProjects])

  const updateArtifact = useCallback(async (key: ArtifactKey, patch: Partial<ProjectRecord['artifacts'][ArtifactKey]>) => {
    const project = projects.find(candidate => candidate.id === loadedProjectId)
    if (!project) throw new Error('请先选择已保存的 Project。')
    const artifact = project.artifacts[key]
    const summary = patch.summary ?? artifact.summary
    const content = key === 'brief' || key === 'creative' ? summary : `[${key}] ${summary}`
    const status = artifactStatus(patch.status ?? artifact.status)
    if (artifact.id) {
      await api.updateArtifact(project.id, artifact.id, { content, status })
    } else {
      await api.createArtifact({ projectId: project.id, kind: artifactKind(key), content, status })
    }
    await reloadProjects()
  }, [loadedProjectId, projects, reloadProjects])

  const advanceArtifact = useCallback((key: ArtifactKey, status: ProjectRecord['artifacts'][ArtifactKey]['status']) => updateArtifact(key, { status }), [updateArtifact])

  const value = useMemo(() => ({ projects, currentProject, agencyWorkbench, targetProjectId, loadedProjectId, isLoading, error, routeDiagnostic, reloadProjects, selectProject, createProject, updateProject, createTask, updateTask, advanceArtifact, updateArtifact }), [projects, currentProject, agencyWorkbench, targetProjectId, loadedProjectId, isLoading, error, routeDiagnostic, reloadProjects, selectProject, createProject, updateProject, createTask, updateTask, advanceArtifact, updateArtifact])
  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
}

function withProjectProducts(project: ApiProject, products?: ApiProject['products']): ApiProject {
  return products ? { ...project, products } : project
}

function toProjectRecord(project: ApiProject, artifacts: ApiArtifact[] = [], jobs: ApiGenerationJob[] = [], tasks: ApiBusinessTask[] = [], changeSets: DeliveryChangeSet[] = [], operations: ApiOperationalRecord[] = []): ProjectRecord {
  const brief = latestArtifact(artifacts.filter(artifact => artifact.status === 'ready'), 'brief')
    ?? latestArtifact(artifacts, 'brief')
  const latestMediaJob = latestMainCreativeGenerationJob(jobs)
  const media = latestMediaJob?.artifactId
    ? artifacts.find(artifact => artifact.id === latestMediaJob.artifactId)
    : latestMainCreativeArtifact(artifacts)
  const updatedAt = formatDate(project.updatedAt)
  const documents = artifacts.filter(artifact => artifact.kind === 'document')
  const seededBudget = Math.max(0, ...changeSets.map(changeSet => changeSet.budgetLimit ?? 0))
  return {
    id: project.id,
    version: project.version,
    code: project.runtime.code,
    name: project.name,
    brand: project.brand,
    product: project.runtime.product,
    products: project.products,
    goal: project.objective,
    industry: project.industry ?? 'ecommerce',
    stage: project.runtime.stage,
    progress: project.runtime.progress,
    status: project.runtime.status === 'completed' ? '已完成' : '进行中',
    owner: project.runtime.owner,
    updatedAt,
    // The platform runtime deliberately has no mutable budget field yet. For
    // seeded and persisted projects, the ChangeSet boundary is the source of
    // truth that lets the delivery preflight run with its configured guardrail.
    budget: project.runtime.budget > 0 ? project.runtime.budget : seededBudget,
    currency: project.runtime.currency,
    timezone: project.runtime.timezone,
    artifacts: {
      brief: toArtifactRecord('brief', brief, project.updatedAt),
      strategy: toArtifactRecord('strategy', latestDocument(documents, 'strategy'), project.updatedAt),
      creative: toCreativeRecord(media, latestMediaJob, project.updatedAt),
      insight: toArtifactRecord('insight', latestDocument(documents, 'insight'), project.updatedAt),
      delivery: toArtifactRecord('delivery', latestDocument(documents, 'delivery'), project.updatedAt),
    },
    tasks,
    changeSets: changeSets.map(toChangeSetRecord),
    operations,
    knowledgeCount: documents.filter(artifact => artifact.content.startsWith('[knowledge]')).length,
  }
}

const emptyAgencyWorkbenchValue: ApiAgencyWorkbench = {
  organizations: [],
  clients: [],
  brands: [],
  projects: [],
  adAccountBindings: [],
  qualityCheckRuns: [],
  materialConfirmations: [],
  assetVersionPointers: [],
}

function mergeAgencyWorkbench(current: ApiAgencyWorkbench, incoming: ApiAgencyWorkbench): ApiAgencyWorkbench {
  return {
    organizations: mergeRecords(current.organizations, incoming.organizations),
    clients: mergeRecords(current.clients, incoming.clients),
    brands: mergeRecords(current.brands, incoming.brands),
    projects: mergeRecords(current.projects, incoming.projects),
    adAccountBindings: mergeRecords(current.adAccountBindings, incoming.adAccountBindings),
    qualityCheckRuns: mergeRecords(current.qualityCheckRuns, incoming.qualityCheckRuns),
    materialConfirmations: mergeRecords(current.materialConfirmations, incoming.materialConfirmations),
    assetVersionPointers: mergeRecords(current.assetVersionPointers, incoming.assetVersionPointers),
  }
}

function mergeRecords<T extends { id: string }>(current: T[], incoming: T[]): T[] {
  return [...new Map([...current, ...incoming].map(value => [value.id, value])).values()]
}

const emptyProject: ProjectRecord = {
  id: '', version: 0, code: '—', name: '尚未连接到服务端', brand: '—', product: '—', goal: '请启动本地 MVP API 后重试。', industry: 'ecommerce',
  stage: '等待恢复', progress: 0, status: '进行中', owner: '—', updatedAt: '—', budget: 0, currency: 'CNY', timezone: 'Asia/Shanghai',
  artifacts: Object.fromEntries((['brief', 'strategy', 'creative', 'insight', 'delivery'] as ArtifactKey[]).map(key => [key, toArtifactRecord(key, undefined, '')])) as ProjectRecord['artifacts'],
  tasks: [],
  changeSets: [],
  operations: [],
  knowledgeCount: 0,
}

function taskSourceArtifactIds(project: ProjectRecord, type: ApiBusinessTaskType, insightReferenceId?: string): string[] {
  const briefId = project.artifacts.brief.id
  const strategyId = project.artifacts.strategy.id
  const creativeId = project.artifacts.creative.id
  if (type === 'strategy') return [briefId, insightReferenceId].filter((id): id is string => Boolean(id))
  if (type === 'video_edit') return [briefId, strategyId, creativeId, insightReferenceId].filter((id): id is string => Boolean(id))
  return [briefId, strategyId, insightReferenceId].filter((id): id is string => Boolean(id))
}

function taskSourceTaskIds(project: ProjectRecord, type: ApiBusinessTaskType): string[] {
  if (type === 'strategy') return []
  const latestStrategy = project.tasks.filter(task => task.type === 'strategy').at(-1)
  if (type === 'creative') return [latestStrategy?.id].filter((id): id is string => Boolean(id))
  const latestCreative = project.tasks.filter(task => task.type === 'creative').at(-1)
  return [latestStrategy?.id, latestCreative?.id].filter((id): id is string => Boolean(id))
}

function toArtifactRecord(key: ArtifactKey, artifact: ApiArtifact | undefined, fallbackTime: string): ProjectArtifact {
  return {
    id: artifact?.id, key, label: artifactLabel(key), version: `v${artifact?.version ?? 0}.0`,
    status: (artifact?.status === 'ready' ? '已确认' : artifact ? '草稿' : key === 'brief' ? '草稿' : '待确认') as ArtifactStatus,
    owner: '服务端存档', updatedAt: formatDate(artifact?.updatedAt ?? fallbackTime),
    summary: artifact?.content ?? '尚无服务端产物。',
    sourceVersion: artifact?.sourceJobId ? `生成任务 ${artifact.sourceJobId.slice(0, 8)}` : undefined,
  }
}

function toCreativeRecord(artifact: ApiArtifact | undefined, job: ApiGenerationJob | undefined, fallbackTime: string): ProjectArtifact {
  const record = toArtifactRecord('creative', artifact, fallbackTime)
  const presentation = presentCreativeStatus(artifact, job, formatDate)
  return {
    ...record,
    ...presentation,
  }
}

function toChangeSetRecord(changeSet: DeliveryChangeSet): ChangeSetRecord {
  const statusMap: Record<DeliveryChangeSet['status'], ChangeSetRecord['status']> = { draft: '草稿', preflight_passed: '待审批', preflight_failed: '草稿', approved: '已批准', rejected: '已拒绝', executing: '执行中', executed: '已执行', rolled_back: '已回滚' }
  return { id: changeSet.id, title: changeSet.name, status: statusMap[changeSet.status], risk: '中', budgetImpact: changeSet.budgetLimit ?? 0, createdAt: formatDate(changeSet.createdAt), createdBy: '服务端演示用户', version: changeSet.version, evidenceIds: changeSet.execution?.evidence.map(item => item.step) ?? [], rollbackPlan: changeSet.rollback?.reason ?? '服务端将记录模拟回滚原因。', changes: [{ field: '预算边界', before: '¥0', after: `¥${(changeSet.budgetLimit ?? 0).toLocaleString('zh-CN')}` }] }
}

function artifactKind(key: ArtifactKey): ApiArtifact['kind'] {
  return key === 'brief' ? 'brief' : key === 'creative' ? 'image' : 'document'
}

function artifactStatus(status: ProjectRecord['artifacts'][ArtifactKey]['status']): ApiArtifact['status'] {
  return status === '已确认' ? 'ready' : 'draft'
}

function artifactLabel(key: ArtifactKey): string {
  return ({ brief: '策略 Brief', strategy: '策略方案', creative: '创意产物', insight: '洞察报告', delivery: '投放计划' })[key]
}

function latestDocument(artifacts: ApiArtifact[], key: string): ApiArtifact | undefined {
  return artifacts.filter(artifact => artifact.content.startsWith(`[${key}]`)).sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))[0]
}

function latestArtifact(artifacts: ApiArtifact[], kind: ApiArtifact['kind']): ApiArtifact | undefined {
  return artifacts.filter(artifact => artifact.kind === kind).sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))[0]
}

function latestMainCreativeGenerationJob(jobs: ApiGenerationJob[]): ApiGenerationJob | undefined {
  return jobs
    .filter(job => (job.artifactKind === 'image' || job.artifactKind === 'video')
      && job.purpose === undefined
      && job.prerollType === undefined)
    .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))[0]
}

function latestMainCreativeArtifact(artifacts: ApiArtifact[]): ApiArtifact | undefined {
  return artifacts
    .filter(artifact => (artifact.kind === 'image' || artifact.kind === 'video')
      && artifact.purpose === undefined
      && artifact.prerollType === undefined)
    .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))[0]
}

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-')
}

export function useProject() {
  const value = useContext(ProjectContext)
  if (!value) throw new Error('useProject must be used inside ProjectProvider')
  return value
}
