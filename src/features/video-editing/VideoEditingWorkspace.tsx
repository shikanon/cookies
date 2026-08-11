import { useEffect, useState } from 'react'
import { Film, Plus } from 'lucide-react'

import { useProject } from '../../context/ProjectContext'
import { shortId } from '../../data/shortId'
import { editingApi, type ApiEditTask } from './api'
import { VideoEditingCanvasWorkspace } from './VideoEditingCanvasWorkspace'

type VideoEditingWorkspaceProps = {
  onNotice: (message: string) => void
  editTaskId?: string
  onOpenEditTask: (id: string) => void
}

const editTaskStatusLabel: Record<ApiEditTask['status'], string> = {
  draft: '草稿', rendering: '渲染中', review_ready: '待评审', completed: '已完成', failed: '失败', archived: '已归档',
}

const editTaskSourceLabel: Record<ApiEditTask['entry_source'], string> = {
  manual: '素材剪辑', short_drama_preroll_v2: '短剧前贴', creative_version: '创意成片',
}

export function VideoEditingWorkspaceV2(props: VideoEditingWorkspaceProps) {
  if (!props.editTaskId) return <VideoEditingTaskHub {...props} />
  return <VideoEditingCanvasWorkspace {...props} editTaskId={props.editTaskId} />
}

function VideoEditingTaskHub({ onNotice, onOpenEditTask }: VideoEditingWorkspaceProps) {
  const { currentProject } = useProject()
  const [tasks, setTasks] = useState<ApiEditTask[]>([])
  const [status, setStatus] = useState<'' | ApiEditTask['status']>('')
  const [state, setState] = useState<'loading' | 'ready' | 'error' | 'creating'>('loading')

  useEffect(() => {
    let active = true
    setState('loading')
    void editingApi.list(currentProject.id, status || undefined).then(result => {
      if (!active) return
      setTasks(result.items ?? [])
      setState('ready')
    }).catch(cause => {
      if (!active) return
      setState('error')
      onNotice(cause instanceof Error ? cause.message : '剪辑任务读取失败')
    })
    return () => { active = false }
  }, [currentProject.id, onNotice, status])

  const createTask = async () => {
    setState('creating')
    try {
      const task = await editingApi.create(currentProject.id, { display_name: '素材剪辑' })
      onOpenEditTask(task.id)
      onNotice(`已创建剪辑任务 ${task.id}`)
    } catch (cause) {
      setState('error')
      onNotice(cause instanceof Error ? cause.message : '剪辑任务创建失败')
    }
  }

  return <section className="video-editing-task-hub">
    <header>
      <div><small>VIDEO EDITING</small><h2>素材剪辑任务</h2><p>从已有草稿继续，或明确创建一个新的空白剪辑。</p></div>
      <button className="primary-button" disabled={state === 'creating'} onClick={() => void createTask()}><Plus size={15}/>{state === 'creating' ? '创建中…' : '新建剪辑'}</button>
    </header>
    <nav aria-label="剪辑任务状态筛选">
      {([['', '全部'], ['draft', '草稿'], ['rendering', '渲染中'], ['review_ready', '待评审'], ['completed', '已完成'], ['failed', '失败']] as const).map(([value, label]) =>
        <button key={value || 'all'} className={status === value ? 'active' : ''} onClick={() => setStatus(value)}>{label}</button>)}
    </nav>
    {state === 'loading' ? <div className="editing-task-empty">正在读取剪辑任务…</div>
      : state === 'error' ? <div className="editing-task-empty">任务读取失败，请稍后重试。</div>
      : tasks.length === 0 ? <div className="editing-task-empty"><Film size={24}/><b>当前筛选下没有任务</b><span>新建剪辑后才会生成 EditTask，不再自动产生空草稿。</span></div>
      : <div className="editing-task-grid">{tasks.map(task => <button key={task.id} className="editing-task-card" onClick={() => onOpenEditTask(task.id)}>
          <span className={`editing-task-status status-${task.status}`}>{editTaskStatusLabel[task.status]}</span>
          <Film size={20}/><b>{task.display_name}</b>
          <span>{editTaskSourceLabel[task.entry_source]}{task.source_creative_task_id ? ` · ${shortId(task.source_creative_task_id)}` : ''}</span>
          <small>时间线 v{task.current_timeline?.version ?? 0} · 更新于 {new Date(task.updated_at).toLocaleString('zh-CN')}</small>
        </button>)}</div>}
  </section>
}
