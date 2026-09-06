import {
  Activity, Aperture, Archive, BadgeCheck, BarChart3, BookOpenCheck, Bot,
  Boxes, BrainCircuit, ChartNoAxesCombined, CircleGauge, ClipboardCheck, Compass,
  Database, FileCheck2, FileSearch, Film, FlaskConical, FolderKanban,
  GalleryHorizontalEnd, Library, Lightbulb, ListChecks,
  Megaphone, MonitorCog, Package, PackageCheck, PanelTop, PlaySquare, Rocket,
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
      { id: 'workspaces', label: '策略工作区', icon: FolderKanban, group: '工作', layout: 'workspace', description: '沿五个稳定阶段推进需求、策略确认与创意交接，研究、资料和历史按需打开。', views: ['理解需求', 'Brief', '策略', '确认 / 评审', '创意交接'] },
      { id: 'briefs', label: '需求中心', icon: ClipboardCheck, group: '策略中枢', prominence: 'hub', navHint: 'Brief、版本与确认', layout: 'table', description: '管理 Brief 完整度、来源、冲突、确认与版本。', views: ['Brief 列表', '待补充', '待确认', '版本库', '冲突队列'] },
      { id: 'research', label: '研究洞察', icon: FileSearch, group: '策略中枢', prominence: 'hub', navHint: '联网研究与证据', layout: 'analysis', description: '组织受众、竞品、行业研究和可引用证据。', views: ['受众', '竞品', '行业', '资料来源', '研究任务'] },
      { id: 'strategies', label: '策略中心', icon: Target, group: '策略中枢', prominence: 'hub', navHint: '策略方案与版本', layout: 'analysis', description: '沉淀方向、受众、主张、渠道预算与实验方案。', views: ['策略库', '渠道策略', '方案对比', '实验方案', '版本库'] },
      { id: 'reviews', label: '评审中心', icon: BadgeCheck, group: '策略中枢', prominence: 'hub', navHint: '确认、协作与变更', layout: 'table', description: '集中处理待评审内容、评论、审批与变更。', views: ['待我评审', '我发起的', '评论与提及', '已完成', '变更记录'] },
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
      // 「投前」= 原「投前洞察」。8-04 那版重构把它并进了「经验 → 查经验」，理由是
      // 「同一批数据的两种读法」——数据上没错，但导航上抹掉了「下一轮该做什么」
      // 这件事：人只看见「经验」，不会意识到那是开工前该来的地方。
      //
      // 拎回来之后它跟「查经验」的分工是：这一页只给 ✅ 能归因的，带数据体检闸门和
      // 跨渠道警告，是**决定**；「查经验」是完整目录，六个维度筛、能看 👁 只是观察的，
      // 是**翻账**。所以这一页敢少给，那一页不敢。
      //
      // 排在最前面：一轮的顺序是投前 → 分析 → 复盘 → 经验，导航顺序就是这个顺序。
      {
        id: 'prelaunch', label: '投前', icon: Compass, group: '工作', layout: 'analysis',
        description: '开工前先看以前什么有效、在什么条件下成立。只列能照着做的结论，数据体检没过会当场说。',
        views: ['结论', '历史模式'],
      },
      // 「分析」= 原「投后分析」（六个视图）+ 原「实验中心」（作为 👁 的升级通道）。
      // 合并的理由：实验不是一个独立的地方，它是「只是观察」升成「能归因」的那一步。
      // 摆成独立入口，人得先意识到自己需要一个实验，才会想到点进去。
      //
      // 六个视图取自 03 §一级导航。19 §5.2 在同一位置写的是「指标总览、素材对比、
      // Cohort、趋势、疲劳、异常」——差别是第三个：Cohort（分群）需要受众构成数据，
      // 而当前一条受众数据都没有接入，那一格只能是空的；03 的「驱动因素」用的是已经
      // 落库的素材特征，能真出数。冲突已登记在基线文档 §5 冲突 7。
      {
        id: 'analysis', label: '分析', icon: TrendingUp, group: '工作', layout: 'analysis',
        description: '一轮投放跑完，为什么是这个结果。六个视图自由看，每一屏标清能不能归因；看到值得留的按「记一笔」。',
        views: ['指标总览', '素材对比', '趋势', '疲劳', '异常', '驱动因素'],
      },
      // 03 §73 和 19 §284 都写五个视图（多「A/B 变体」，末位是「实验结论」不是「实验结果」），
      // 代码原为四个（基线文档 §5 冲突 12）。
      //
      // 侧栏里不再列它（hidden）：实验是从一条「只是观察」的结论上跳过来的，
      // 不是一个人会先想起来去逛的地方。两条进入路径都在分析页上——结论徽章里的
      // 「做个实验」，和工具栏上那个常驻按钮。路由保留，直接删掉的话页面就断了。
      { id: 'experiments', label: '实验中心', icon: FlaskConical, group: '素材与分析', layout: 'analysis', hidden: true, description: '管理 A/B 变量、样本检查和可归因结果。', views: ['实验列表', 'A/B 变体', '变量矩阵', '样本检查', '实验结论'] },
      // 只留有数据模型的三个视图。「周期报告 / 自定义报告 / 协作 / 版本与导出」写在
      // 03 §5 和 19 §5.2 的二级功能表里，但 03 §9 的功能需求表里没有任何一条 FR 支撑，
      // 后端也没有对应模型——这里唯一的 P0 需求是 AM-015 任务复盘（基线文档 §5 冲突 9）。
      //
      // 「复盘」= 原报告中心。改名不是换皮：报告中心听起来是个存档的地方，
      // 而这里是一轮投放收尾时人真正要做事的地方——决定这一轮留下什么。
      // 位置也从「经验与输出」挪到「工作」，紧跟「分析」：记一笔发生在分析页，
      // 收尾发生在这里，两步是同一件事的前后半段，分在两个组里人会以为是两码事。
      {
        // layout 跟「分析」一样用 analysis：这一页也是左边列表右边详情的两栏，
        // 而 layout 只决定没有专用页面时的兜底外观和 .layout-* 那个 CSS 钩子。
        id: 'review', label: '复盘', icon: BookOpenCheck, group: '工作', layout: 'analysis',
        description: '一轮投放收尾。看你一路记的那几笔，系统把漏的补齐，逐条决定留不留，提交，把值得复用的沉淀成经验。',
        views: ['本轮', '全部复盘', '已沉淀经验'],
      },
      // 「经验」= 原「投前洞察」+ 原「经验库」。
      //
      // 合并的理由：这两页是同一批数据的两种读法。投前洞察的五个视图里，前四个只是
      // 同一批经验按不同条件筛，第五个（引用记录）是每条经验自己的属性——做成独立
      // 视图，人得先在那一页找到这条经验才能看它的历史，现在收进卡片的展开层里。
      //
      // 剩下的两个视图分的是这个人现在要干什么：查是下一轮开工时看以前什么有效，
      // 管是给这些结论背书。经验库原来的五个视图（待定 / 在用 / 该看一眼 / 停用 /
      // 引用记录）不再是五栏——「该看一眼」本来就不是第四个状态，是「在用」上的
      // 一个标记；三个状态一起列、要人做事的排前面，比点三次栏目少绕两步。
      {
        id: 'experience', label: '经验', icon: Lightbulb, group: '工作', layout: 'analysis',
        description: '以前什么有效、在什么条件下成立、凭什么这么说。',
        views: ['查经验', '管经验'],
      },
      // 「素材」= 原「数据接入」+ 原「分析素材库」+ 原「内容分析」。
      //
      // 三个入口原来各管一段，但人来这里只有一个问题：能拿来分析的素材有哪些、还差
      // 什么。接数据、看变量、找相似，都是为了回答这一个问题的手段，摆成三个平级
      // 入口等于让人自己去猜该先点哪个。总览先说清缺什么，另外四个视图是补的地方。
      //
      // 内容分析原来的七个视图按素材类型切分（小红书/公众号/…），现在收进「变量」
      // 里按类型切换——素材类型是筛选条件，不是七个不同的功能。
      //
      // 排在最后：前三个是一轮投放的主线（看结果 → 收尾 → 沉淀），素材是给这条
      // 主线备料的地方，日常不是每次都要进。
      {
        id: 'assets', label: '素材', icon: Library, group: '工作', layout: 'table',
        description: '能拿来分析的素材有哪些、还差什么、样本不够时从哪里补。',
        views: ['总览', '数据接入', '变量', '找相似', '外部素材'],
      },
      // 米云素材（同步自上游 shikanon/cookies）。没有并进上面的「素材」，因为它不是
      // 备料的另一种方式，是另一条生产线：从产品资料出发去外部授权采集，人工挑候选，
      // 再裂变。上面那一页管的是「我手里已有的素材够不够分析」，进来的人问的问题不一样。
      //
      // 排在「素材」后面、「设置」前面：备料的两页挨着，主线（投前→分析→复盘→经验）
      // 不被打断。
      {
        id: 'miyun-materials', label: '米云素材', icon: Aperture, group: '工作', layout: 'workspace',
        description: '围绕产品资料、授权采集和透明数据卡完成米云素材的人工候选决策。',
        views: ['产品分析', '采集任务', '素材候选', '裂变任务'],
      },
      // 原来的「数据质量」「能力运营」「系统设置」合成这一个。
      //
      // 三个入口回答的是同一个问题：这套系统凭什么这么判。判定阈值是标准，数据体检
      // 是按这个标准量出来的数据够不够干净，变量字典是这些数和词各自指什么，确认
      // 权限是谁能把机器说的变成我们认的。摆成三个平级入口，人得先猜「我这个疑问
      // 归哪一页管」才找得到。
      //
      // 「判定阈值」排第一，也是这一版唯一新增的能力：以前那七个数字散在三个 Go
      // 文件里，看不见也调不了——而一个看不见的阈值和一个错的阈值，在使用者那里是
      // 同一种东西。
      {
        id: 'settings', label: '设置', icon: Settings2, group: '治理', layout: 'settings',
        description: '判定标准、数据体检、变量字典、确认权限。',
        views: ['判定阈值', '数据体检', '变量字典', '确认权限'],
      },
    ],
  },
  {
    key: 'delivery', label: '智能投放', shortLabel: '投放', icon: Rocket,
    statement: '把批准策略和创意转化为安全、可审计的投放动作。',
    nav: [
      { id: 'plans', label: '投放计划', icon: Megaphone, group: '计划与执行', layout: 'workspace', description: '配置目标、预算、受众、版位、创意和校验。', views: ['全部计划', '草稿', '待审批', '执行中', '已完成', '版本'] },
        { id: 'configuration', label: '平台配置', icon: Boxes, group: '计划与执行', layout: 'workspace', description: '查看业务意图到平台配置的不可变映射，并提交检查与审批。', views: ['配置映射', '字段校准与处置', '检查与提交'] },
      { id: 'execution', label: '执行中心', icon: PlaySquare, group: '计划与执行', layout: 'operations', description: '管理受控执行、等待用户、接管、恢复和验证。', views: ['待执行', '执行中', '等待用户', '结果未知', '失败', '接管', '完成'] },
      { id: 'platform-entities', label: '项目与单元', icon: Database, group: '资源', layout: 'table', description: '同步投放账号的项目和单元，并统一查看 Cookies 绑定。', views: ['全部对象', '项目', '单元', '未绑定'] },
      { id: 'monitoring', label: '模拟与巡检', icon: Activity, group: '监控与优化', layout: 'analysis', description: '上线前运行 Mechanistic 概率模拟。上线后使用 Connector 事实巡检告警。', views: ['概率模拟', 'Connector 巡检', '消耗异常', '转化异常', '跑量不足'] },
      { id: 'optimization', label: '优化中心', icon: TrendingUp, group: '监控与优化', layout: 'analysis', description: '基于同一 SimulationRun 的指标与告警生成建议，由人工采纳或拒绝并跟踪优化草稿。', views: ['待处理建议', '已采纳', '观察中', '已拒绝', '效果跟踪'] },
      { id: 'accounts', label: '账户与环境', icon: UsersRound, group: '资源', layout: 'table', description: '管理广告账户、平台资产、权限和执行环境。', views: ['广告账户', '平台资产', '权限', '登录状态', '执行设备'] },
      { id: 'products', label: '产品目录', icon: Package, group: '资源', layout: 'table', description: '管理组织级业务产品对象；产品在巨量平台录入后回绑商品 ID。', views: ['产品列表', '巨量映射'] },
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
