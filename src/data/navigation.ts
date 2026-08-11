import {
  Activity, Aperture, Archive, BadgeCheck, BarChart3, BookOpenCheck, Bot,
  Boxes, BrainCircuit, ChartNoAxesCombined, CircleGauge, ClipboardCheck,
  Database, FileCheck2, FileSearch, Film, FlaskConical, FolderKanban,
  GalleryHorizontalEnd, Library, ListChecks,
  Megaphone, MonitorCog, PackageCheck, PanelTop, PlaySquare, Rocket, Route,
  SearchCheck, Send, Settings2, ShieldCheck, SlidersHorizontal, Sparkles,
  TableProperties, Target, TrendingUp, UsersRound, Video, WandSparkles,
} from 'lucide-react'
import type { SystemDefinition } from '../types'

export const systems: SystemDefinition[] = [
  {
    key: 'strategy', label: '需求与策略', shortLabel: '策略', icon: BrainCircuit,
    statement: '把模糊需求转化为可追溯、可执行的广告策略。',
    nav: [
      { id: 'tasks', label: '策略任务', icon: ListChecks, group: '工作', layout: 'workspace', description: '创建真实持久化任务，串联 Brief、研究证据、策略版本与评审结果。', views: ['全部任务', '进行中', '待评审', '已完成', '已归档'] },
      { id: 'workspaces', label: '策略工作区', icon: FolderKanban, group: '工作', layout: 'workspace', description: '在对话、Brief、研究、策略、创意任务策略、实验与评审之间保持同一上下文。', views: ['概览', '对话', 'Brief', '研究', '策略', '创意任务策略', '实验', '评审', '变更记录'] },
      { id: 'briefs', label: '需求中心', icon: ClipboardCheck, group: '资产与方法', layout: 'table', description: '管理 Brief 完整度、来源、冲突、确认与版本。', views: ['Brief 列表', '待补充', '待确认', '版本库', '冲突队列'] },
      { id: 'strategies', label: '策略中心', icon: Target, group: '资产与方法', layout: 'analysis', description: '沉淀方向、受众、主张、渠道预算与实验方案。', views: ['策略库', '渠道策略', '方案对比', '实验方案', '版本库'] },
      { id: 'research', label: '研究洞察', icon: FileSearch, group: '资产与方法', layout: 'analysis', description: '组织受众、竞品、行业研究和可引用证据。', views: ['受众', '竞品', '行业', '资料来源', '研究任务'] },
      { id: 'reviews', label: '评审中心', icon: BadgeCheck, group: '协作', layout: 'table', description: '集中处理待评审内容、评论、审批与变更。', views: ['待我评审', '我发起的', '评论与提及', '已完成', '变更记录'] },
    ],
  },
  {
    key: 'creative', label: '创意创作', shortLabel: '创意', icon: WandSparkles,
    statement: '把批准策略转化为可评审、可交付的图文与视频作品。',
    nav: [
      { id: 'tasks', label: '创意任务', icon: ListChecks, group: '工作', layout: 'table', description: '统一进入图文、视频、前贴和素材剪辑工作区，串联策略来源、制作、评审与交付。', views: ['全部', '进行中', '等待输入', '生成中', '待评审', '已完成', '失败', '归档'] },
      { id: 'image-text', label: '图文创作', icon: GalleryHorizontalEnd, group: '创作', layout: 'editor', description: '完成文案、封面、图组、排版、素材和渠道检查。', views: ['小红书', '公众号', '草稿箱', '图文版本'] },
      { id: 'video', label: '视频创作', icon: Video, group: '创作', layout: 'editor', description: '效果广告、品牌广告与素材剪辑三类稳定工作区。', views: ['效果广告', '品牌广告', '素材剪辑'] },
      { id: 'production', label: '制作中心', icon: Aperture, group: '创作', layout: 'operations', description: '管理图片、视频、音频生成与渲染队列。', views: ['图片生成', '视频生成', '音频生成', '渲染队列', '源素材', '失败任务'] },
      { id: 'reviews', label: '素材检查', icon: BadgeCheck, group: '协作与输出', layout: 'workspace', description: '按素材版本完成大模型质检、人工确认和退回占位。', views: ['全部素材', '待质检', '未通过', '待人工确认', '已确认'] },
      { id: 'deliveries', label: '交付中心', icon: PackageCheck, group: '协作与输出', layout: 'table', description: '生成稳定发布包、投放包和授权清单。', views: ['待交付', '发布包', '投放包', '下载记录', '停用版本'] },
    ],
  },
  {
    key: 'insight', label: '素材洞察', shortLabel: '洞察', icon: ChartNoAxesCombined,
    statement: '投前支持策略与创意，投后解释效果并沉淀经验。',
    nav: [
      // 五个视图取自 03 §8；原来只有一个「策略与创意证据」，把证据、建议、模式和
      // 风险压成一张表，等于让人自己从混在一起的卡里挑出哪条能当证据（基线文档 §5 冲突 6）。
      { id: 'prelaunch', label: '投前洞察', icon: SearchCheck, group: '工作', layout: 'analysis', description: '按当前 Project 组合历史素材、经验与证据，支持 Brief、策略和创意决策。', views: ['策略证据', '创意建议', '历史模式', '风险与反例', '引用记录'] },
      // 六个视图取自 03 §一级导航。19 §5.2 在同一位置写的是「指标总览、素材对比、
      // Cohort、趋势、疲劳、异常」——差别是第三个：Cohort（分群）需要受众构成数据，
      // 而当前一条受众数据都没有接入，那一格只能是空的；03 的「驱动因素」用的是已经
      // 落库的素材特征，能真出数。冲突已登记在基线文档 §5 冲突 7。
      { id: 'performance', label: '投后分析', icon: TrendingUp, group: '工作', layout: 'analysis', description: '连接投放指标与素材特征，解释表现、疲劳、异常和驱动因素。', views: ['指标总览', '素材对比', '趋势', '疲劳', '异常', '驱动因素'] },
      { id: 'connections', label: '数据接入', icon: Database, group: '数据', layout: 'operations', description: '管理平台数据源、字段映射、素材映射和同步。', views: ['数据源', '导入任务', '字段映射', '素材映射', '同步记录'] },
      { id: 'assets', label: '分析素材库', icon: Library, group: '素材与分析', layout: 'table', description: '建立可分析素材索引、版本、特征与血缘。', views: ['全部素材', '待匹配', '待提取', '特征', '版本与血缘'] },
      // 视图按素材类型切分，取自 03 §一级导航与 19 §5.2；原来的「案例拆解/漫剧大盘/制作方法/包装检查」
      // 来自参考站点，与 PRD 的六类特征体系对不上（基线文档 §5 冲突 5）。
      { id: 'content', label: '内容分析', icon: Film, group: '素材与分析', layout: 'analysis', description: '把创意内容转成可比较的变量：按素材类型提取特征，同时服务投前参考与投后解释。', views: ['小红书', '公众号', '品牌广告', '数字人', '广告前贴', '爆款复刻', '单素材拆解'] },
      // 03 §73 和 19 §284 都写五个视图（多「A/B 变体」，末位是「实验结论」不是「实验结果」），
      // 代码原为四个（基线文档 §5 冲突 12）。这一页整体尚未启用，标签换成文档口径不影响现有功能。
      { id: 'experiments', label: '实验中心', icon: FlaskConical, group: '素材与分析', layout: 'analysis', description: '管理 A/B 变量、样本检查和可归因结果。', views: ['实验列表', 'A/B 变体', '变量矩阵', '样本检查', '实验结论'] },
      { id: 'knowledge', label: '经验库', icon: BookOpenCheck, group: '经验与输出', layout: 'workspace', description: '沉淀结论、适用条件、反例、复审与跨环节引用。', views: ['候选经验', '已确认', '待复审', '已失效', '引用记录'] },
      // 只留有数据模型的三个视图。「周期报告 / 自定义报告 / 协作 / 版本与导出」写在
      // 03 §5 和 19 §5.2 的二级功能表里，但 03 §9 的功能需求表里没有任何一条 FR 支撑，
      // 后端也没有对应模型——报告中心目前唯一的 P0 需求是 AM-015 任务复盘（基线文档 §5 冲突 9）。
      { id: 'reports', label: '报告中心', icon: PanelTop, group: '经验与输出', layout: 'editor', description: '按任务汇总素材表现、实验结论与下一轮建议，确认后沉淀成经验。', views: ['全部复盘', '待确认', '已沉淀经验'] },
      { id: 'quality', label: '数据质量', icon: ShieldCheck, group: '治理', layout: 'operations', description: '监控新鲜度、缺失、口径、异常和修复队列。', views: ['新鲜度', '缺失', '异常', '口径', '对账', '修复队列'] },
      // 第五个视图原来写「版本与质量」，03 §一级导航和 19 §288 都写「质量看板」，
      // 这里对齐文档（基线文档 §5 冲突 11）。
      { id: 'operations', label: '能力运营', icon: SlidersHorizontal, group: '治理', layout: 'operations', description: '治理特征体系、指标字典、Skills 与评测集。', views: ['特征体系', '指标字典', '分析 Skills', '评测集', '质量看板'] },
      { id: 'settings', label: '系统设置', icon: Settings2, group: '治理', layout: 'settings', description: '配置样本、窗口、通知、确认权限与报告模板。', views: ['样本门槛', '观察窗口', '通知', '确认权限', '报告模板'] },
    ],
  },
  {
    key: 'delivery', label: '智能投放', shortLabel: '投放', icon: Rocket,
    statement: '把批准策略和创意转化为安全、可审计的投放动作。',
    nav: [
      { id: 'tour', label: '上线后优化闭环', icon: Route, group: '计划与执行', layout: 'workspace', description: '从计划来源、首次上线授权、平台操作演练走到指标、告警、优化申请与人工操作包。', views: ['走测总览'] },
      { id: 'plans', label: '投放计划', icon: Megaphone, group: '计划与执行', layout: 'workspace', description: '配置目标、预算、受众、版位、创意和校验。', views: ['全部计划', '草稿', '待审批', '执行中', '已完成', '版本'] },
      { id: 'configuration', label: '平台配置', icon: Boxes, group: '计划与执行', layout: 'workspace', description: '查看 DeliveryIntent 到平台配置的不可变映射，并提交预检与审批。', views: ['配置映射', '检查与提交', '人工操作包'] },
      { id: 'execution', label: '执行中心', icon: PlaySquare, group: '计划与执行', layout: 'operations', description: '管理受控执行、等待用户、接管、恢复和验证。', views: ['待执行', '执行中', '等待用户', '结果未知', '失败', '接管', '完成'] },
      { id: 'monitoring', label: '监控告警', icon: Activity, group: '监控与优化', layout: 'analysis', description: '运行可重复的投放效果情景模拟，并从同一 SimulationRun 的指标与事件生成告警。', views: ['全部告警', '审核拒绝', '跑量不足', '素材疲劳', '追踪异常', '成本恶化'] },
      { id: 'optimization', label: '优化中心', icon: TrendingUp, group: '监控与优化', layout: 'analysis', description: '基于同一 SimulationRun 的指标与告警生成建议，由人工采纳或拒绝并跟踪优化草稿。', views: ['待处理建议', '已采纳', '观察中', '已拒绝', '效果跟踪'] },
      { id: 'accounts', label: '账户与环境', icon: UsersRound, group: '资源', layout: 'table', description: '管理广告账户、平台资产、权限和执行环境。', views: ['广告账户', '平台资产', '权限', '登录状态', '执行设备'] },
      { id: 'approvals', label: '审批中心', icon: FileCheck2, group: '审批与审计', layout: 'workspace', description: '审查预算、上线、暂停、扩量和紧急动作。', views: ['待我审批', '我发起的', '预算', '上线', '暂停与扩量', '已完成'] },
      { id: 'evidence', label: '证据与审计', icon: Archive, group: '审批与审计', layout: 'operations', description: '保存执行时间线、截图、结构化日志和前后差异。', views: ['执行时间线', '页面截图', '结构化日志', '前后差异', '导出'] },
    ],
  },
]

export const quickActions = [
  { label: '新建策略工作区', detail: '从需求对话开始', system: 'strategy' as const },
  { label: '创建创意任务', detail: '基于已批准策略', system: 'creative' as const },
  { label: '查看投前洞察', detail: '为策略与创意引用证据', system: 'insight' as const },
  { label: '创建投放计划', detail: '先校验再执行', system: 'delivery' as const },
]
