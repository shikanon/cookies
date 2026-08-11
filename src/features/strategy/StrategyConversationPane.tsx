import {
  ArrowUpRight,
  Bot,
  BrainCircuit,
  Check,
  CircleCheck,
  FileText,
  Globe2,
  Image as ImageIcon,
  LoaderCircle,
  Paperclip,
  Send,
  Sparkles,
  Video,
  X,
} from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import {
  buildConversationLens,
  compactDocumentTitle,
  intakeMissingLabel,
} from './strategyConversationModel'
import type {
  BriefDraft,
  BriefVersion,
  ConversationCapabilities,
  CreativeIntakeV4,
  KnowledgeDocument,
  MediaUnderstandingArtifact,
  Message,
  MessageContentBlock,
  MessageRequestedPolicy,
} from './types'

type ViralRemakeResult = { intake: CreativeIntakeV4; taskId?: string }

type Props = {
  brief: BriefDraft | null
  briefVersion: BriefVersion | null
  busy: string
  conversationCapabilities: ConversationCapabilities | null
  documents: KnowledgeDocument[]
  mediaArtifacts: MediaUnderstandingArtifact[]
  messages: Message[]
  pending: boolean
  onConfirmRequirement: () => Promise<boolean>
  onOpenBrief: () => void
  onOpenFullStrategy: () => void
  onReadyViralRemake: (taskId: string) => void
  onSend: (
    content: string,
    documents?: KnowledgeDocument[],
    media?: MediaUnderstandingArtifact[],
    requestedPolicy?: MessageRequestedPolicy,
  ) => Promise<boolean>
  onStartViralRemake: () => Promise<ViralRemakeResult | null>
  onUploadDocument: (file: File) => Promise<KnowledgeDocument | null>
  onUploadMedia: (file: File) => Promise<MediaUnderstandingArtifact | null>
}

const starterPrompts = [
  '我有一条参考视频，想保留节奏结构但做成原创版本',
  '我们要推广一个新品，目标是先让核心人群理解它的价值',
  '我有一份内部 PDF，请先帮我提炼可用于创作的事实',
]

export function StrategyConversationPane({
  brief,
  briefVersion,
  busy,
  conversationCapabilities,
  documents,
  mediaArtifacts,
  messages,
  onConfirmRequirement,
  onOpenBrief,
  onOpenFullStrategy,
  onReadyViralRemake,
  onSend,
  onStartViralRemake,
  onUploadDocument,
  onUploadMedia,
  pending,
}: Props) {
  const [content, setContent] = useState('')
  const [feedback, setFeedback] = useState('')
  const [deepReasoning, setDeepReasoning] = useState(false)
  const [webSearch, setWebSearch] = useState(false)
  const [attachedDocumentIds, setAttachedDocumentIds] = useState<string[]>([])
  const [attachedMediaIds, setAttachedMediaIds] = useState<string[]>([])
  const listRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const lens = buildConversationLens(brief, documents)
  const locked = Boolean(briefVersion)
  const attachedDocuments = attachedDocumentIds.flatMap(id => {
    const document = documents.find(value => value.id === id)
    return document ? [document] : []
  })
  const attachedMedia = attachedMediaIds.flatMap(id => {
    const artifact = mediaArtifacts.find(value => value.id === id)
    return artifact ? [artifact] : []
  })
  const documentsReady = attachedDocuments.every(document => document.status === 'ready')
  const mediaReady = attachedMedia.every(artifact => artifact.status === 'ready' || artifact.status === 'partial')
  const attachmentsReady = documentsReady && mediaReady
  const pendingPolicy = [...messages].reverse().find(message => message.role === 'user')?.requested_policy

  useEffect(() => {
    const list = listRef.current
    if (list) list.scrollTop = list.scrollHeight
  }, [messages, pending])

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = 'auto'
    textarea.style.height = `${Math.min(Math.max(textarea.scrollHeight, 82), 170)}px`
  }, [content])

  useEffect(() => {
    if (!conversationCapabilities?.deep_reasoning.available) setDeepReasoning(false)
    if (!conversationCapabilities?.web_search.available) setWebSearch(false)
  }, [conversationCapabilities])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const value = content.trim()
    if (!value && !attachedDocuments.length && !attachedMedia.length) return
    if (!attachmentsReady) {
      setFeedback('请等待附件解析完成；这样 Agent 读取的是正文与来源定位，而不是文件名占位符。')
      return
    }
    setFeedback('')
    const requestedPolicy: MessageRequestedPolicy | undefined = deepReasoning || webSearch
      ? {
          ...(deepReasoning ? { reasoning_mode: 'deep' as const } : {}),
          ...(webSearch ? { web_search: 'allowed' as const } : {}),
        }
      : undefined
    const sent = await onSend(value, attachedDocuments, attachedMedia, requestedPolicy)
    if (sent) {
      setContent('')
      setAttachedDocumentIds([])
      setAttachedMediaIds([])
      setDeepReasoning(false)
      setWebSearch(false)
    }
  }

  const upload = async (file: File | undefined) => {
    if (!file) return
    setFeedback('')
    const document = await onUploadDocument(file)
    if (document) {
      setAttachedDocumentIds(current => current.includes(document.id) ? current : [...current, document.id])
      setFeedback(`${file.name} 已进入解析队列；完成后会随下一条消息一起发送，并保留 chunk 来源。`)
    }
  }

  const uploadMedia = async (file: File | undefined) => {
    if (!file) return
    setFeedback('')
    const artifact = await onUploadMedia(file)
    if (artifact) {
      setAttachedMediaIds(current => current.includes(artifact.id) ? current : [...current, artifact.id])
      setFeedback(`${file.name} 已进入理解队列；只有带时间点或画面定位的直接证据会进入下一轮对话。`)
    }
  }

  const startViralRemake = async () => {
    setFeedback('')
    const result = await onStartViralRemake()
    if (!result) return
    if (result.taskId) {
      onReadyViralRemake(result.taskId)
      return
    }
    const missing = result.intake.missing_fields.map(intakeMissingLabel)
    setFeedback(missing.length
      ? `还不能进入创作：${missing.join('；')}。`
      : '当前需求仍需补充一项创作前信息。')
  }

  return <section className="kanon-conversation-workbench">
    <header className="kanon-conversation-header">
      <div>
        <span className="section-label">CONVERSATIONAL REQUIREMENT</span>
        <h2>先说清楚要解决什么，AI 负责收敛。</h2>
        <p>不要求先填完整表单；产品、目标和核心受众足够后，就能冻结需求并选择创作路径。</p>
      </div>
      <span className={`kanon-requirement-state ${locked ? 'locked' : lens.coreReady ? 'ready' : ''}`}>
        {locked ? <CircleCheck size={14}/> : <Sparkles size={14}/>}
        {locked ? `需求 v${briefVersion?.version} 已确认` : `${lens.completedCore} / ${lens.totalCore} 项核心事实`}
      </span>
    </header>

    <div className="kanon-conversation-grid">
      <div className="kanon-conversation-thread">
        <div className="kanon-message-list" ref={listRef}>
          {!messages.length ? <div className="kanon-conversation-empty-v2">
            <span><Bot size={19}/></span>
            <div>
              <small>从一句真实需求开始</small>
              <h3>不用整理成 Brief，也不用猜字段。</h3>
              <p>说出你已经确定的内容；缺的信息我只追问会改变创作结果的部分。</p>
            </div>
            <div className="kanon-starter-prompts" aria-label="需求示例">
              {starterPrompts.map(prompt => <button key={prompt} onClick={() => {
                setContent(prompt)
                textareaRef.current?.focus()
              }} type="button">{prompt}<ArrowUpRight size={13}/></button>)}
            </div>
          </div> : null}
          {messages.map(message => <ConversationMessage key={message.id} message={message}/>)}
          {pending ? <article className="kanon-message assistant thinking" aria-live="polite">
            <span>AI</span><div><small>Strategy 助手</small><p><LoaderCircle className="spin" size={14}/>{pendingPolicy?.reasoning_mode === 'deep'
              ? '正在进行本轮深度分析，联网证据与内部资料会分别标注…'
              : pendingPolicy?.web_search === 'allowed'
                ? '正在联网搜索，搜索完成后再生成本轮回答…'
                : '正在区分事实、假设和仍需确认的问题…'}</p></div>
          </article> : null}
        </div>

        <form className="kanon-composer-v2" onSubmit={submit}>
          <div className="kanon-composer-bezel">
            <textarea
              aria-describedby="kanon-strategy-message-help"
              id="kanon-strategy-message"
              maxLength={4000}
              onChange={event => setContent(event.target.value)}
              onKeyDown={event => {
                if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
                  event.preventDefault()
                  event.currentTarget.form?.requestSubmit()
                }
              }}
              placeholder="例如：我们要给 FlowKit 做一条面向效率工具用户的短视频，希望提高试用转化…"
              ref={textareaRef}
              rows={3}
              value={content}
            />
            {attachedDocuments.length ? <div className="kanon-composer-attachments">
              {attachedDocuments.map(document => <span className={document.status} key={document.id}>
                {document.status === 'parse_queued' || document.status === 'parsing'
                  ? <LoaderCircle className="spin" size={12}/>
                  : <FileText size={12}/>}
                <b>{compactDocumentTitle(document)}</b>
                <small>{document.status === 'ready' ? `${document.chunk_count} 个片段` : document.status === 'parse_failed' ? '解析失败' : '解析中'}</small>
                <button aria-label={`移除附件 ${compactDocumentTitle(document)}`} onClick={() => setAttachedDocumentIds(current => current.filter(id => id !== document.id))} type="button"><X size={12}/></button>
              </span>)}
            </div> : null}
            {attachedMedia.length ? <div className="kanon-composer-attachments media">
              {attachedMedia.map(artifact => <span className={artifact.status} key={artifact.id}>
                {artifact.status === 'running'
                  ? <LoaderCircle className="spin" size={12}/>
                  : artifact.asset_kind === 'video' ? <Video size={12}/> : <ImageIcon size={12}/>}
                <b>{artifact.asset_kind === 'video' ? '短视频理解' : '图片理解'}</b>
                <small>{mediaArtifactStatus(artifact)}</small>
                <button aria-label="移除媒体附件" onClick={() => setAttachedMediaIds(current => current.filter(id => id !== artifact.id))} type="button"><X size={12}/></button>
              </span>)}
            </div> : null}
            {conversationCapabilities?.deep_reasoning.available || conversationCapabilities?.web_search.available
              ? <div className="kanon-composer-modes" aria-label="本轮 AI 能力">
                  {conversationCapabilities.deep_reasoning.available ? <button
                    aria-pressed={deepReasoning}
                    className={deepReasoning ? 'active' : ''}
                    disabled={Boolean(busy) || pending}
                    onClick={() => setDeepReasoning(current => !current)}
                    title={`仅本轮启用更强推理，发送成功后自动关闭；预计增加约 ${conversationCapabilities.deep_reasoning.estimated_wait_seconds ?? 30} 秒`}
                    type="button"
                  ><BrainCircuit size={13}/><span>深度思考</span><small>本轮</small></button> : null}
                  {conversationCapabilities.web_search.available ? <button
                    aria-pressed={webSearch}
                    className={webSearch ? 'active' : ''}
                    disabled={Boolean(busy) || pending}
                    onClick={() => setWebSearch(current => !current)}
                    title={`只向外部服务发送当前问题，不发送附件正文；预计约 ${conversationCapabilities.web_search.estimated_wait_seconds ?? 45} 秒`}
                    type="button"
                  ><Globe2 size={13}/><span>联网查证</span><small>仅外发问题</small></button> : null}
                </div>
              : null}
            <footer>
              <div>
                {conversationCapabilities?.multimodal_input.available ? <>
                <label className={`kanon-attach-action ${busy === 'upload-document' ? 'busy' : ''}`} htmlFor="kanon-conversation-document">
                  {busy === 'upload-document' ? <LoaderCircle className="spin" size={14}/> : <Paperclip size={14}/>}
                  <span>{busy === 'upload-document' ? '正在上传' : '添加资料'}</span>
                </label>
                <input
                  accept=".pdf,.docx,.md"
                  disabled={Boolean(busy)}
                  id="kanon-conversation-document"
                  onChange={event => {
                    const file = event.target.files?.[0]
                    event.target.value = ''
                    void upload(file)
                  }}
                  type="file"
                />
                <label className={`kanon-attach-action ${busy === 'upload-media' ? 'busy' : ''}`} htmlFor="kanon-conversation-media">
                  {busy === 'upload-media' ? <LoaderCircle className="spin" size={14}/> : <ImageIcon size={14}/>}
                  <span>{busy === 'upload-media' ? '正在理解' : '图片 / 视频'}</span>
                </label>
                <input
                  accept="image/jpeg,image/png,video/mp4"
                  disabled={Boolean(busy)}
                  id="kanon-conversation-media"
                  onChange={event => {
                    const file = event.target.files?.[0]
                    event.target.value = ''
                    void uploadMedia(file)
                  }}
                  type="file"
                />
                </> : null}
                <span id="kanon-strategy-message-help">Enter 发送 · Shift + Enter 换行</span>
              </div>
              <div><small>{content.length} / 4000</small><button aria-label="发送需求消息" disabled={Boolean(busy) || pending || (!content.trim() && !attachedDocuments.length && !attachedMedia.length) || !attachmentsReady} type="submit"><Send size={15}/></button></div>
            </footer>
          </div>
        </form>
        {busy === 'web-search'
          ? <div className="kanon-conversation-feedback searching" role="status"><LoaderCircle className="spin" size={12}/>正在联网查证；只外发当前问题，原消息会在证据固化后发送。</div>
          : feedback ? <div className="kanon-conversation-feedback" role="status">{feedback}</div> : null}
      </div>

      <aside className="kanon-understanding-lens" aria-label="AI 当前理解">
        <header>
          <span>UNDERSTANDING LENS</span>
          <h3>AI 当前理解</h3>
          <p>这里只展示会影响下一步的事实，不把内部结构化字段甩给用户。</p>
        </header>
        <div className="kanon-lens-progress" aria-label={`已识别 ${lens.completedCore} 项，共 ${lens.totalCore} 项核心事实`}>
          <span style={{ width: `${lens.completedCore / lens.totalCore * 100}%` }}/>
        </div>
        <div className="kanon-lens-facts">
          {lens.items.map(item => <article className={item.value ? 'captured' : ''} key={item.key}>
            <span>{item.value ? <Check size={12}/> : <i/>}</span>
            <div>
              <small>{item.label}{item.required ? '' : ' · 可选'}</small>
              <p>{item.value || '还没有可靠信息'}</p>
              {item.value && item.sourceLabel ? <em className="kanon-fact-source">{item.sourceLabel}</em> : null}
            </div>
          </article>)}
        </div>

        <section className="kanon-lens-sources">
          <div><span>来源资料</span><small>{lens.readySourceCount + mediaArtifacts.filter(value => value.status === 'ready' || value.status === 'partial').length} / {lens.sourceCount + mediaArtifacts.length} 可用</small></div>
          {documents.length ? documents.slice(0, 3).map(document => <article key={document.id}>
            <FileText size={14}/><span><b>{compactDocumentTitle(document)}</b><small>{document.status === 'ready' ? `${document.chunk_count} 个可检索片段` : document.status === 'parse_failed' ? '解析失败' : '正在解析'}</small></span>
          </article>) : null}
          {mediaArtifacts.slice(0, 3).map(artifact => <article key={artifact.id}>
            {artifact.asset_kind === 'video' ? <Video size={14}/> : <ImageIcon size={14}/>}
            <span><b>{artifact.asset_kind === 'video' ? '短视频证据' : '图片证据'}</b><small>{mediaArtifactStatus(artifact)}</small></span>
          </article>)}
          {!documents.length && !mediaArtifacts.length ? <p>{conversationCapabilities?.multimodal_input.available
            ? '可直接添加文档、图片或 15–90 秒 MP4；只有可定位的直接证据会影响理解。'
            : '当前灰度未开放附件输入；仍可直接用自然语言说明需求。'}</p> : null}
        </section>

        <footer className="kanon-lens-action">
          {locked ? conversationCapabilities?.quick_viral_remake.available ? <>
            <div><Video size={16}/><span><b>爆款裂变快速路径</b><small>跳过形式化策略文档，直接校验参考视频和创作输入。</small></span></div>
            <button className="primary-button full" disabled={Boolean(busy)} onClick={() => void startViralRemake()} type="button">
              {busy === 'viral-remake' ? <LoaderCircle className="spin" size={14}/> : <Sparkles size={14}/>}
              {busy === 'viral-remake' ? '正在创建创作任务…' : '进入爆款裂变'}
            </button>
          </> : <>
            <div><Sparkles size={16}/><span><b>进入完整策略路径</b><small>快速裂变当前未灰度开放；已确认需求仍可直接生成品牌与创作策略。</small></span></div>
            <button className="primary-button full" onClick={onOpenFullStrategy} type="button"><ArrowUpRight size={14}/>进入完整策略</button>
          </> : lens.coreReady ? <>
            <div><CircleCheck size={16}/><span><b>核心信息已经够用</b><small>确认会冻结不可变 Requirement 版本；其他信息仍可在新版本补充。</small></span></div>
            <button className="primary-button full" disabled={Boolean(busy) || pending} onClick={() => void onConfirmRequirement()} type="button">
              {busy === 'confirm-requirement' ? <LoaderCircle className="spin" size={14}/> : <Check size={14}/>}
              确认理解并锁定需求
            </button>
          </> : <div className="kanon-lens-next"><span>{lens.totalCore - lens.completedCore}</span><p><b>还差少量关键信息</b>继续对话即可，不必打开表单逐项填写。</p></div>}
          <button className="kanon-lens-detail-link" onClick={onOpenBrief} type="button">查看或修正完整理解 <ArrowUpRight size={12}/></button>
        </footer>
      </aside>
    </div>
  </section>
}

function ConversationMessage({ message }: { message: Message }) {
  return <article className={`kanon-message ${message.role}`}>
    <span>{message.role === 'user' ? '我' : message.role === 'assistant' ? 'AI' : '·'}</span>
    <div>
      <small>{message.role === 'user' ? '需求方' : message.role === 'assistant' ? 'Strategy 助手' : '系统事件'} · {formatTime(message.created_at)}</small>
      {message.requested_policy?.reasoning_mode === 'deep' || message.requested_policy?.web_search === 'allowed'
        ? <div className="kanon-message-policy" aria-label="本轮实际请求能力">
            {message.requested_policy.reasoning_mode === 'deep' ? <span><BrainCircuit size={11}/>深度思考</span> : null}
            {message.requested_policy.web_search === 'allowed' ? <span><Globe2 size={11}/>已联网查证</span> : null}
          </div>
        : null}
      {message.content_blocks?.length
        ? <div className="kanon-message-blocks">{message.content_blocks.map((block, index) => <MessageBlock block={block} key={`${block.type}-${index}`}/>)}</div>
        : <p>{message.content}</p>}
    </div>
  </article>
}

function MessageBlock({ block }: { block: MessageContentBlock }) {
  if (block.type === 'text') return <p>{block.text}</p>
  if (block.type === 'document_ref') {
    return <span className="kanon-message-ref" title={`Document ${block.document_id}`}><FileText size={13}/>资料附件 · 来源已锁定</span>
  }
  if (block.type === 'research_ref') {
    return <span className="kanon-message-ref research" title={`Research artifact ${block.research_artifact_id}`}><Globe2 size={13}/>联网证据 · 内容哈希已锁定</span>
  }
  return <span className="kanon-message-ref" title={`Asset ${block.asset_id} · v${block.asset_version}`}>
    {block.asset_kind === 'video' ? <Video size={13}/> : <ImageIcon size={13}/>}
    {block.asset_kind === 'video' ? '参考视频' : '参考图片'} · 已锁定版本 v{block.asset_version}
  </span>
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function mediaArtifactStatus(artifact: MediaUnderstandingArtifact) {
  if (artifact.status === 'running') return '正在提取可定位证据'
  if (artifact.status === 'failed') return artifact.error_message || '理解失败'
  if (artifact.status === 'partial') {
    const qualifiers = [
      artifact.asset_kind === 'video' && artifact.keyframes.length ? `${artifact.keyframes.length} 个时间点` : '',
      artifact.warnings.includes('transcript_unavailable') ? '无音频转写' : '',
    ].filter(Boolean)
    const summary = artifact.summary || '仅获得部分证据，请人工复核'
    return qualifiers.length ? `${qualifiers.join(' · ')} · ${summary}` : summary
  }
  const evidenceCount = artifact.visible_text.length + artifact.observations.length
  if (artifact.asset_kind === 'video') return `${artifact.keyframes.length} 个时间点 · ${evidenceCount} 条直接证据`
  return `${evidenceCount} 条直接证据`
}
