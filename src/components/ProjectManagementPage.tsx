import { useEffect, useMemo, useState, type FormEvent } from 'react'
import {
  ArrowRight,
  Boxes,
  Check,
  CircleAlert,
  FileStack,
  Goal,
  Save,
  UsersRound,
} from 'lucide-react'
import { useProject } from '../context/ProjectContext'
import { calculateProjectProgress, progressBarWidth, progressPercentLabel, progressReasonLabel } from '../lib/project-progress'
import type { BusinessTaskType, SystemKey } from '../types'

type OpenProject = (id: string, system?: SystemKey, navId?: string, objectId?: string, view?: string) => void
type ManagementTab = 'overview' | 'scope' | 'members' | 'assets'

const tabs: Array<{ id: ManagementTab; label: string }> = [
  { id: 'overview', label: '项目概览' },
  { id: 'scope', label: '范围与目标' },
  { id: 'members', label: '成员与职责' },
  { id: 'assets', label: '资产与版本' },
]

const moduleDefinitions: Array<{
  system: SystemKey
  navId: string
  label: string
  responsibility: string
  owner: string
}> = [
  { system: 'strategy', navId: 'tasks', label: '需求与策略', responsibility: '定义 Brief、策略任务和研究证据', owner: '品牌负责人 / 策略' },
  { system: 'creative', navId: 'tasks', label: '创意创作', responsibility: '完成脚本、素材生成、剪辑和评审', owner: '创意策划 / AI 制作' },
  { system: 'insight', navId: 'analysis', label: '素材洞察', responsibility: '连接广告数据、复盘结论与经验资产', owner: '素材分析 / 投手' },
  { system: 'delivery', navId: 'plans', label: '智能投放', responsibility: '管理计划、ChangeSet、审批与执行', owner: '广告投手 / 审批人' },
]

const taskTypeLabels: Record<BusinessTaskType, string> = {
  strategy: '策略任务',
  creative: '创意任务',
  video: '视频创作',
  brand_video: '品牌广告',
  short_drama_preroll: '短剧前贴',
  game_preroll: '游戏前贴',
  commerce_preroll: '电商前贴',
  viral_remake: '爆款复刻',
  video_edit: '视频包装',
}

const taskStatusLabels = {
  draft: '草稿',
  in_progress: '进行中',
  ready: '待评审',
  completed: '已完成',
  failed: '失败',
}

export function ProjectManagementPage({ onOpenWorkbench, onOpenProject }: {
  onOpenWorkbench: (id: string) => void
  onOpenProject: OpenProject
}) {
  const { currentProject, updateProject } = useProject()
  const [activeTab, setActiveTab] = useState<ManagementTab>('overview')
  const [name, setName] = useState(currentProject.name)
  const [brand, setBrand] = useState(currentProject.brand)
  const [goal, setGoal] = useState(currentProject.goal)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState('')

  useEffect(() => {
    setName(currentProject.name)
    setBrand(currentProject.brand)
    setGoal(currentProject.goal)
    setNotice('')
  }, [currentProject.id, currentProject.name, currentProject.brand, currentProject.goal])

  const readyArtifacts = Object.values(currentProject.artifacts).filter(artifact =>
    artifact.id && ['已确认', '已完成'].includes(artifact.status),
  ).length
  const activeTasks = currentProject.tasks.filter(task => ['draft', 'in_progress', 'ready'].includes(task.status)).length
  const completedTasks = currentProject.tasks.filter(task => task.status === 'completed').length
  const recentTasks = useMemo(
    () => currentProject.tasks.slice().sort((left, right) => right.updatedAt.localeCompare(left.updatedAt)).slice(0, 5),
    [currentProject.tasks],
  )
  const failedTask = currentProject.tasks.find(task => task.status === 'failed')
  const hasConfirmedBrief = currentProject.artifacts.brief.status === '已确认'
  const hasCompletedCreative = currentProject.artifacts.creative.status === '已完成'
  const pendingChangeSet = currentProject.changeSets.find(change => ['草稿', '待审批'].includes(change.status))
  const projectProgress = calculateProjectProgress(currentProject)
  const nextAction = failedTask
    ? { label: '处理失败任务', detail: failedTask.name, system: 'creative' as const, navId: 'tasks', blocker: '存在失败任务，需先恢复后再继续推进。' }
    : !hasConfirmedBrief
      ? { label: '确认策略 Brief', detail: '明确目标、受众与创意边界', system: 'strategy' as const, navId: 'workspaces', blocker: '策略 Brief 尚未确认。' }
      : !hasCompletedCreative
        ? { label: '进入创意生产', detail: '基于已确认 Brief 生成并评审素材', system: 'creative' as const, navId: 'tasks', blocker: '缺少可用于投放的已完成创意。' }
        : pendingChangeSet
          ? { label: '检查投放计划', detail: pendingChangeSet.title, system: 'delivery' as const, navId: 'plans', blocker: `变更申请 ${pendingChangeSet.id} 待复核。` }
          : { label: '查看项目进展', detail: '复核当前阶段与跨模块工作', system: 'strategy' as const, navId: 'workspaces', blocker: '当前没有阻塞项。' }

  const saveScope = async (event: FormEvent) => {
    event.preventDefault()
    if (!name.trim() || !brand.trim() || !goal.trim()) return
    setSaving(true)
    setNotice('')
    try {
      await updateProject({ name: name.trim(), brand: brand.trim(), goal: goal.trim() })
      setNotice('项目范围已保存为新的服务端版本，全部流程节点将继续使用该 Project 上下文。')
    } catch (cause) {
      setNotice(cause instanceof Error ? cause.message : '项目保存失败，请稍后重试。')
    } finally {
      setSaving(false)
    }
  }

  return <div className="project-management-page">
    <header className="project-management-heading">
      <div><span className="section-label">PROJECT MANAGEMENT · {currentProject.code}</span><h1>{currentProject.name}</h1><p>先处理当前阶段的关键行动；范围、治理和资产留在辅助区随时可查。</p></div>
      <button className="secondary-button" onClick={() => onOpenWorkbench(currentProject.id)}>进入项目工作台<ArrowRight size={15}/></button>
    </header>

    <nav className="project-management-tabs" aria-label="项目管理视图">
      {tabs.map(tab => <button key={tab.id} className={activeTab === tab.id ? 'active' : ''} onClick={() => setActiveTab(tab.id)} aria-pressed={activeTab === tab.id}>{tab.label}</button>)}
    </nav>

    {notice ? <div className="page-notice" role="status"><Check size={15}/>{notice}</div> : null}

    {activeTab === 'overview' ? <div className="project-management-overview">
      <section className="project-action-workspace" aria-label="项目当前行动">
        <div className="project-action-stage"><span className="section-label">当前阶段</span><b>{projectProgress.stageLabel}</b><strong>{progressPercentLabel(projectProgress, 'stagePercent')}</strong><i><em style={{ width: progressBarWidth(projectProgress, 'stagePercent') }}/></i><small>负责人 · {currentProject.owner}</small></div>
        <div className="project-action-main"><span className="section-label">下一步</span><h2>{nextAction.label}</h2><p>{nextAction.detail}</p><button className="primary-button" onClick={() => onOpenProject(currentProject.id, nextAction.system, nextAction.navId)}>{nextAction.label}<ArrowRight size={15}/></button></div>
        <div className="project-action-blocker"><CircleAlert size={17}/><div><small>阻塞项 / 状态判断</small><b>{progressReasonLabel(projectProgress)}</b><p>{activeTasks} 个任务推进中 · {readyArtifacts} / 5 个核心产物已确认</p></div></div>
      </section>
      <div className="project-management-grid">
        <section className="project-module-map">
          <div className="management-section-heading"><div><span className="section-label">工作域</span><h2>按业务阶段进入工作</h2></div><small>当前 Project 共享同一上下文</small></div>
          {moduleDefinitions.map((module, index) => <button key={module.system} onClick={() => onOpenProject(currentProject.id, module.system, module.navId)}>
            <span>{String(index + 1).padStart(2, '0')}</span><div><b>{module.label}</b><p>{module.responsibility}</p><small>{module.owner}</small></div><ArrowRight size={15}/>
          </button>)}
        </section>
        <aside className="project-management-summary project-context-rail">
          <span className="section-label">项目上下文</span>
          <dl><div><dt>业务目标</dt><dd>{currentProject.goal}</dd></div><div><dt>预算基线</dt><dd>¥{currentProject.budget.toLocaleString('zh-CN')} · {currentProject.currency}</dd></div><div><dt>治理与审计</dt><dd>{currentProject.changeSets.length} 个 ChangeSet · {currentProject.knowledgeCount} 条经验</dd></div><div><dt>最近更新</dt><dd>{currentProject.updatedAt}</dd></div></dl>
          <button className="secondary-button full" onClick={() => setActiveTab('scope')}>编辑范围与目标</button>
        </aside>
      </div>
      <section className="recent-project-tasks">
        <div className="management-section-heading"><div><span className="section-label">项目任务</span><h2>最近业务任务</h2></div><span>{currentProject.tasks.length} 个任务全部限定在当前 Project</span></div>
        {recentTasks.length ? recentTasks.map(task => <button key={task.id} onClick={() => onOpenProject(currentProject.id, task.type === 'strategy' ? 'strategy' : 'creative', 'tasks', task.id)}>
          <span>{taskTypeLabels[task.type]}</span><div><b>{task.name}</b><small>{task.objective}</small></div><em className={task.status}>{taskStatusLabels[task.status]}</em><strong>v{task.version}</strong><ArrowRight size={14}/>
        </button>) : <div className="project-management-empty">项目还没有业务任务，请从项目工作台的“需求下达”开始。</div>}
      </section>
    </div> : null}

    {activeTab === 'scope' ? <form className="project-scope-editor" onSubmit={saveScope}>
      <header><Goal size={20}/><div><h2>项目范围与目标</h2><p>这里保存的是所有 Brief、创意、洞察和投放动作共同继承的项目级边界。</p></div></header>
      <label><span>项目名称</span><input value={name} onChange={event => setName(event.target.value)} required/><small>用于 Home、工作台、任务与审计记录。</small></label>
      <label><span>品牌或产品</span><input value={brand} onChange={event => setBrand(event.target.value)} required/><small>作为创意品牌约束和数据归属。</small></label>
      <label className="wide"><span>项目目标</span><textarea value={goal} onChange={event => setGoal(event.target.value)} required/><small>描述业务结果，不在这里写具体创意方案。</small></label>
      <div className="project-scope-inheritance"><Boxes size={18}/><div><b>修改后的继承范围</b><p>需求与策略、创意创作、素材洞察、智能投放，以及此后新建的任务和产物。</p></div></div>
      <footer><button type="button" className="secondary-button" onClick={() => { setName(currentProject.name); setBrand(currentProject.brand); setGoal(currentProject.goal) }}>恢复当前值</button><button type="submit" className="primary-button" disabled={saving || !name.trim() || !brand.trim() || !goal.trim()}><Save size={15}/>{saving ? '正在保存…' : '保存项目范围'}</button></footer>
    </form> : null}

    {activeTab === 'members' ? <section className="project-responsibility-page">
      <header><UsersRound size={20}/><div><h2>成员与职责</h2><p>职责按流程阶段编排；同一个项目内的交接不会改变数据归属。</p></div></header>
      {moduleDefinitions.map((module, index) => <article key={module.system}><span>{String(index + 1).padStart(2, '0')}</span><div><b>{module.label}</b><small>{module.responsibility}</small></div><div><small>默认职责</small><strong>{module.owner}</strong></div><em><Check size={12}/>已纳入项目</em></article>)}
      <div className="project-governance-note"><CircleAlert size={16}/><span><b>权限原则</b><small>成员只读取当前 Project 的任务、产物和 ChangeSet；跨项目引用必须经过来源校验。</small></span></div>
    </section> : null}

    {activeTab === 'assets' ? <section className="project-assets-page">
      <header><FileStack size={20}/><div><h2>资产与版本</h2><p>这里查看项目级资产总账；内容编辑仍在对应业务模块完成。</p></div></header>
      {Object.values(currentProject.artifacts).map(artifact => <article key={artifact.key}>
        <span>{artifact.label}</span><div><b>{artifact.summary}</b><small>{artifact.owner} · 更新于 {artifact.updatedAt}</small></div><strong>{artifact.version}</strong><em>{artifact.status}</em>
      </article>)}
      <footer><span>经验资产 <b>{currentProject.knowledgeCount}</b> 条</span><span>ChangeSet <b>{currentProject.changeSets.length}</b> 个</span><span>业务任务 <b>{currentProject.tasks.length}</b> 个</span></footer>
    </section> : null}
  </div>
}
