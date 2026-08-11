# 巨量引擎业务 Schema 校准

| 属性 | 内容 |
| --- | --- |
| 状态 | 只读业务校准已达到 v0.1 冻结条件并完成阶段收口：电商手动路径为首条业务路径，应用下载 Android 记录到真实事件资产门禁，其余入口进入覆盖矩阵；真实写入统一留待受控写入阶段 |
| Schema 版本 | `oceanengine-bidding-schema/v0.1`（只读契约已冻结；观察 fixture 保留 `v0.1-draft`） |
| 证据基线 | [巨量只读校准摘要](./oceanengine-readonly-calibration.md)；[收口与配置契约](./read-only-calibration-closeout.md) |
| 范围 | 产品目标覆盖完整竞价投放场景；当前页面证据来自尾号 `6391` 专用测试账户，不记录完整账户、对象 ID、业务链接或身份信息 |
| 本批不做 | 未授权平台写入、Connector 文件修改、用单条路径外推完整枚举 |

## 1. 目标与证据状态

本次业务校准把只读走查的页面事实转成版本化、可评审的业务 Schema。它描述平台对象、字段、条件和内部映射，但不负责取得指标数据，也不把页面观察直接编译成可执行点击步骤。

每个字段和条件必须使用以下一种证据状态：

| evidence_state | 含义 | 本契约允许的使用方式 |
| --- | --- | --- |
| `observed` | 页面区块、字段或关系在只读走查中被直接观察 | 可以进入首个路径 Schema，但仍需单独记录枚举完整性和必填性 |
| `sample_only` | 只观察到一个值、一个对象或一个模式，不能证明完整枚举/默认规则 | 可以作为 fixture 样例，不得作为封闭枚举或全局默认值 |
| `platform_pending` | 当前账户或页面证据不足 | 保留字段位置和未知原因，不生成可执行平台值 |
| `owner_pending` | Delivery 需要消费另一个文件夹尚未发布的稳定输出 | 由该文件夹所有者在其目录内实现；Delivery 不修改其文件，也不把自身页面 Schema 或对象拆分标成此状态 |
| `operator_reviewed` | 真实投手对业务顺序、常用做法或产品可用性给出评审结论 | 可用于修正业务流和产品要求；与页面证据冲突时必须显式保留并重新校准 |

`required_state`、`editable_state` 和 `enum_state` 分别记录，不能因为字段可见就推断必填、可写或枚举完整：

```text
unknown | observed_optional | observed_required | conditional | verification_pending
unknown | observed_readonly | observed_editable | conditional
unknown | sample_only | observed_current_set | observed_open_set | observed_closed_set | dynamic_reference
```

当记录新建页自然初始值时，额外使用 `default_state`：`unknown`、`observed_current_path_initial` 或 `inherited_candidate`。其中 `observed_current_path_initial` 只证明当前父项目与黄金路径下未交互时的页面值，不代表平台全局默认；是否由父项目继承仍未知时不得写成 `inherited_candidate`。

`required_state=verification_pending` 只用于页面证据与投手业务评审直接冲突的字段；它要求定向复核条件分支，不能进入真实执行预检。

## 2. 平台对象语义 v0.1

| object_kind（Delivery 暂用名） | 父级/引用关系 | 只读稳定证据 | 语义与边界 | 状态 |
| --- | --- | --- | --- | --- |
| `PlatformAccount` | 无；通过 cookies Project 的账户绑定引用 | 当前投放账户上下文；账户报表行内没有账户 ID | 投放账号是既有业务上下文，不是每次投放流程新建的对象；承载预算/余额/报表和项目入口范围 | `observed` / `operator_reviewed` |
| `PlatformProject` | 属于 `PlatformAccount` | 项目 ID、项目名称、项目列表和项目表单 | 承载营销产品与目标、模式、受众、排期、预算、竞价/出价、监测和名称 | `observed` |
| `PlatformPromotion` | 必须属于一个既有 `PlatformProject`，不能脱离项目单独创建 | 单元 ID、所属项目、单元列表和单元表单；投手复核确认新建单元依赖既有项目 | 承载投放身份、素材/文案、锚点、落地页、产品信息、创意组件和单元设置 | `observed` / `operator_reviewed` |
| `PlatformMaterial` | 可被 Promotion 引用；不建模成严格子对象 | 素材 ID、类型、内容和基础素材报表 | 优先引用前序板块产出的 `Asset ID + Version + ContentHash`，或用户自行上传的素材；上传平台后附加账户范围的平台素材引用 | `observed` / `operator_reviewed` |
| `PlatformOptimizationTarget` | 依赖营销链路、应用/商品和事件资产 | 既有项目出现一个目标样本；未提交表单可能返回“无可用目标” | Delivery 维护动态引用结构，保存账户范围、平台对象 ID、显示名快照、适用链路和证据版本；不是固定页面枚举 | `sample_only` / `operator_reviewed` |
| `PlatformIdentity` | 被 Promotion 的“抖音号”身份分支引用 | 新建页显示投放身份必选；投手复核确认可选“账户信息”或“抖音号” | “账户信息”直接使用投放账户信息且无需附加引用；“抖音号”必须引用已授权账号。授权动作不属于选择动作 | `observed` / `operator_reviewed` |
| `PlatformLandingPage` | 被 Promotion 引用 | 自研落地页支持敏感值手填或从资产库单选；资产库可搜索名称/ID并按类型、来源过滤 | 保存引用而非把完整业务 URL 写入 Schema evidence；管理落地页是写入相邻动作 | `observed` |
| `PlatformCreativeComponent` | 被 Promotion 引用或内嵌 | 单元编辑页存在附加创意组件、行动号召和智能生成控件 | 素材库入口证明组件可能引用既有资产，但独立对象 ID、内嵌字段和完整枚举尚未证明 | `observed` / `platform_pending` |

当前可确认的层级是：

```text
PlatformAccount
└─ PlatformProject
   └─ PlatformPromotion
      ├─ selects account identity or references an authorized PlatformIdentity
      ├─ references PlatformMaterial / Copy
      ├─ references PlatformLandingPage
      ├─ references Product / Application / EventAsset
      └─ embeds or references PlatformCreativeComponent（待确认）
```

### 2.1 投手语言与首个真实业务流程

`PlatformAccount`、`PlatformProject`、`PlatformPromotion` 只用于 Delivery 代码和适配层。投手评审、产品页面与操作说明统一使用“投放账号 → 项目 → 单元”，不要求投手理解内部对象名。

首个电商场景按 2026-08-06 投手评审冻结为：

```text
甲方需求与淘宝商品链接
→ 进入既有竞价投放账号
→ 准备商品、落地页、锚点、人群包等平台资产
→ 素材中心上传
→ 平台审核与质量过滤
→ 选取可投、高质量、使用较少的素材
→ 围绕商品、优惠券、素材和定向组合创建多个项目
→ 为各项目配置类型、商品、目标、手动投放、版位、定向、排期、预算与出价
→ 在项目下创建一个或多个单元
→ 配置身份、素材、锚点、绑定 CID 的敏感落地链路与商品信息
→ 最终确认并开启项目/单元
→ 查看数据并持续调整素材、定向、预算、出价和项目组合
```

完整投放意图由项目与其单元共同实现。智能投放的价值重点是辅助管理多个手动项目及其素材、定向、预算、出价和实验组合，不是简单替用户选择平台自动投放模式。深度优化方式和倍数调价属于数据驱动策略，不能由静态表单规则直接决定。

素材上传、平台审核、质量判定、使用历史和疲劳事实由素材/洞察侧拥有；Delivery 只消费带版本、内容哈希、审核/质量/使用状态的素材引用。上游尚未可用时使用显式 mock 覆盖“可投、打回、低质、已高频使用”等状态，不在 Delivery 建立第二套素材中心。

### 2.2 Delivery 自主职责与稳定引用

文件夹归属即职责边界：`docs/delivery/**` 与 `internal/systems/delivery/**` 内的平台配置语义、真实页面 Schema、字段拆分和预检由 Delivery 负责，不再作为 Owner 确认项向上抛；Delivery 不修改 Connector 或 Insights 文件。

以下 ThreeTier 解释只用于记录历史 mock 与页面校准之间的差异，不是当前目标领域模型：

- `ThreeTierGroup` 是内部组织单元，不强行等同某个巨量对象；
- `ThreeTierPlan` 按真实页面字段拆入项目或单元；
- `ThreeTierCreative` 引用前序板块产出的不可变素材版本或用户上传素材；
- 只有创建投放上下文前必须存在的跨模块元数据，才向所属模块请求稳定引用。

首版稳定引用规则由 Delivery 冻结：

| 引用 | 最小稳定内容 |
| --- | --- |
| 优化/转化目标 | 平台、账户范围、对象 ID、显示名快照、适用链路、证据版本 |
| 商品 | 平台、账户范围、商品对象 ID、显示名快照、证据版本 |
| 素材 | 内部 Asset ID、Version、ContentHash；上传后可附平台素材引用 |
| 落地页、锚点、人群包 | 平台、账户范围、对象类型、对象 ID、显示名快照、证据版本 |

未取得平台对象 ID 时使用显式 unresolved reference，不得以显示名冒充稳定标识。

### 2.3 确认与审批的产品结论

真实平台新建项目和单元没有固定的双审批流程。预检、情景模拟、影子分析和采纳建议均不产生平台写入，因此不应分别要求审批。目标流程为：

```text
生成项目/单元草稿
→ 预检
→ 模拟或影子分析
→ 修改或采纳建议
→ 生成最终操作包
→ 一次最终确认
→ 创建并开启项目/单元
```

默认人工确认点仅位于最终真实创建/开启。预算显著提高、扩大定向、启用智能放量等高风险动作可以触发针对性确认，但确认点由风险与自动化成熟度决定，不是固定业务步骤。历史 mock 闭环中已实现的两段审批属于演示安全链，不能继续作为真实投手流程或目标架构。

## 3. 首版条件矩阵

完整场景覆盖是固定产品目标，不再作为范围决策项。当前矩阵同时承担覆盖清单：已观察路径记录证据，尚未覆盖的营销目的、载体、目标、模式、版位、定向、竞价和单元形态继续追加；缺少路径时直接进入后续工作，不通过缩小范围关闭问题。未观察的组合仍不得凭经验伪造字段值。

每条路径持续记录 `observed`、`implemented`、`reviewed`、`platform_pending` 和 `write_validation_pending` 状态。当前电商路径只是第一条已校准路径，不代表最终范围。

| matrix_id | 营销目的/链路 | 投放模式 | 已观察的差异区块 | 尚未证明 | evidence_state |
| --- | --- | --- | --- | --- | --- |
| `OE-M01` | 电商 → 短视频+图文 → 橙子落地页 | 自动投放（UBMax） | 商品库双版本、AIGC、受众、排期/预算、5 类监测链接、6 个优化目标；选中黄金路径商品后目标集合未变化；`app内下单` 新增 4 类深度优化 | 优化目标平台代码、其他商品/载体的完整目标目录、提交/审核结果 | `observed` + `platform_pending` |
| `OE-M02` | 电商 | 手动投放 | 已校准版位、新建/已有定向包、行为兴趣、抖音达人、设备网络、转化过滤、智能放量、搜索快投和 3 类竞价策略；深度优化与手动模式存在兼容约束 | 当前账户没有真实手动项目；服务端提交校验和真实效果 | `observed` + `operator_reviewed` + `sample_only` |
| `OE-M03` | 销售线索 → 短视频+图文 | 当前页面未显示独立模式选择；项目名称以自动投放生成 | 商品手动选择/智能识别；智能优选/自定义线索方式；橙子落地页、私信页、自研落地页、字节小程序分支；5 个当前目标；线索专属定向、预算和竞价 | 各目标的平台代码、单元字段、服务端校验；页面未显示独立模式选择的原因 | `observed` + `sample_only` + `platform_pending` |
| `OE-M04` | 应用 → 应用下载 → 安卓 → 已有应用 | 自动投放（UBMax）/手动投放入口均可见 | 两条下载方式、优先应用商店下载、应用选择器、项目额度、安卓版本、转化过滤时间、监测链接组、预算和竞价 | 当前两个应用样本均缺少可用事件目标；因此目标后兼容模式、单元字段和真实计费规则无法继续 | `observed` + `blocked_by_event_asset` |
| `OE-M05` | 应用调起、预约下载、iOS、鸿蒙、直播 | 未冻结 | 页面入口或标签可见 | 切换后的字段、枚举、默认、校验和对象可用性 | `sample_only` / `platform_pending` |
| `OE-M06` | 内容营销及其他开放中目的 | 未冻结 | 入口和开放状态文案可见 | 完整表单不可用或未展开 | `platform_pending` |

### 3.1 电商手动路径页面校准

本批在未提交的新建项目页切换到手动投放，只改变本地未保存表单状态，没有保存、提交、开启、上传或进入资产管理写操作。已取得以下页面事实：

| 区块 | 已观察结构 | 证据限制 |
| --- | --- | --- |
| 版位 | 通投智选、首选媒体；首选媒体当前列出今日头条、西瓜视频、抖音、番茄系媒体、穿山甲 | 这是当前页面集合；平台提示仍可能扩展到其他流量，不能编译成绝对排他规则 |
| 定向模式 | 新建定向、选择已有定向包；当前账户的已有定向包搜索结果为空 | 空态不代表平台不支持人群包，也不代表其他账户为空 |
| 基础定向 | 地域、性别、年龄、自定义人群、平台、设备、网络、已安装用户、已转化用户、手机品牌与价格 | 枚举只对当前电商手动路径成立 |
| 行为兴趣 | 行为和兴趣分组独立；支持类目词、关键词、批量添加、搜索、自动选词；行为窗口当前初始 30 天；每组最多 300 个 | 覆盖人数为动态事实，不写入静态 Schema |
| 抖音达人 | 可按达人/昵称/分类搜索，添加达人分类或具体达人，当前最多 30 个 | 当前只证明关注/互动受众入口和控件结构，不冻结达人目录 |
| 智能放量 | 当前初始关闭；可放宽地域、性别、年龄、自定义人群 | 仍需逐营销目的检查维度是否变化 |
| 排期与竞价 | 长期/自定义日期、全天/7×24 时段；稳定成本、最优成本、最大转化；日预算与条件出价 | 无商品/目标时显示的计费方式不能外推到选定目标后的结果 |
| 搜索快投 | 关键词、自动更新、出价系数、定向扩展；当前无关键词 | 平台建议系数和规模估算是动态建议，不作为固定策略 |

### 3.2 销售线索连续路径

本批在未保存表单中切换到“销售线索 → 短视频+图文”，并选择一个当前可用目标，没有保存或提交。当前连续路径为：

```text
销售线索
→ 短视频+图文
→ 商品[手动选择 | 智能识别]
→ 获取线索方式[智能优选 | 自定义]
→ 投放载体
→ 优化目标
→ 线索专属定向
→ 排期、预算与出价
```

| 分支 | 当前页面事实 |
| --- | --- |
| 智能优选 | 载体为橙子落地页+抖音私信页或橙子落地页；留资方式在创建单元时配置 |
| 自定义 | 载体为橙子落地页、自研落地页、抖音私信页、字节小程序；平台不执行线索方式优选 |
| 优化目标 | 当前集合为表单提交、多转化、回访_信息确认、回访_高潜成交、私信留资中的四个可见候选以页面实际返回为准；本批实际选择“表单提交”形成连续路径 |
| 定向差异 | 新建/已有定向包、地域/性别/年龄/自定义人群、行为兴趣、过滤高活跃用户、过滤自己粉丝、过滤高关注用户、网络、已转化用户、智能定向拓展和手机价格 |
| 竞价 | 稳定成本、最大转化；日预算和出价输入可见 |

页面目标选择器本批实际列出“表单提交、多转化、回访_信息确认、回访_高潜成交、私信留资”五个文本项；其中目标数量和适用载体仍应作为动态集合，不写死为全局枚举。机器证据见 [`fixtures/oceanengine-leads-short-video-v0.1-draft.json`](./fixtures/oceanengine-leads-short-video-v0.1-draft.json)。

### 3.3 应用下载 Android 路径

本批选择一个脱敏 Android 已有应用，只改变未保存表单状态。应用选择器当前可见两个已发布应用样本，支持直接使用应用包；“选择分包”是否可用依赖具体应用。

```text
应用
→ 应用下载
→ 短视频+图文
→ Android
→ 已有应用
→ [直接下载 | 落地页下载]
→ 优先应用商店下载[不启用 | 启用]
→ 优化目标(event asset)
```

选定应用后新增或确认以下字段：应用名称、优先应用商店下载、自动项目额度、过滤已转化用户的应用/时间窗口、Android 版本、监测链接组。当前应用在优化目标选择器中返回“当前链路下没有任何可用的优化目标”，因此路径以 `blocked_by_event_asset` 结束；不能凭其他账户经验补造目标、深度优化、计费或单元字段。机器证据见 [`fixtures/oceanengine-app-download-android-v0.1-draft.json`](./fixtures/oceanengine-app-download-android-v0.1-draft.json)。

### 3.4 全场景覆盖追踪维度

覆盖追踪不要求一次穷举平台的笛卡尔积，而是确保每个可达分支都能被版本化追踪。以下维度缺一不可；每条路径同时记录页面可达性、字段集合、默认值、必填/禁用、枚举来源、父子引用、客户端校验、服务端校验、审核状态和写入验证状态。

| coverage_dimension | 当前覆盖 | 下一批可执行任务 |
| --- | --- | --- |
| 营销目的/场景 | 电商、销售线索已有连续表单路径；应用下载 Android 到事件资产门禁；内容营销等仅入口 | 继续校准应用调起/预约下载及 iOS/鸿蒙；不可用入口记录首个阻塞，不伪造字段 |
| 商品/应用/事件资产 | 电商商品双库；Android 应用选择器、已发布应用和无事件目标状态已观察 | 后续由真实事件资产或其他账户样本补齐应用目标；当前保持显式阻塞 |
| 载体→目标→深度方式→模式 | 电商链路已复核；销售线索载体/目标集合已观察；应用下载在目标处阻塞 | 对应用调起、预约下载和小程序逐条记录同构依赖图 |
| 自动/手动模式 | 电商两种模式可达；应用自动有样本 | 补齐不同目的下模式禁用原因及模式切换导致的字段重置 |
| 版位与定向 | 电商手动、销售线索和应用下载 Android 均有字段差异证据 | 继续校准应用调起/预约下载和小程序差异 |
| 预算、排期、竞价、计费 | 电商及销售线索有目标后样本；应用无目标时只记录页面初始字段 | 应用取得事件目标后补齐真实计费/出价关系；服务端错误留待写入阶段 |
| 项目→单元父子关系 | 已确认单元不能脱离既有项目创建 | 补齐不同项目类型下单元容量、继承与只读字段 |
| 投放身份 | 已冻结账户信息/已授权抖音号二选一 | 只读复核两种模式的字段差异；授权仍是写入相邻动作，不进入 |
| 素材/文案/审核质量 | 选择器结构、容量标签、低效/审核驳回过滤已观察 | 等素材板块发布稳定引用；其间继续使用显式 mock 覆盖审核、低质、疲劳和使用量状态 |
| 锚点/落地页/直达链 | 锚点与身份解耦；自研落地页和敏感链接引用已观察 | 补齐其他载体下的锚点/落地页存在性和校验，不记录真实 URL |
| 保存/开启/审核/错误 | 当前全部只读，禁止写入 | 获得明确授权后在测试项目执行最小写入验证；此前保持 `write_validation_pending` |

条件关系的当前最小表达为：

```text
marketing_purpose
  → marketing_scenario / marketing_product / carrier
  → selected_product_or_application
  → available_optimization_targets(event_assets)

delivery_mode
  → placement / targeting / material_supplement
  → budget_and_bidding / search_boost

optimization_target_ref
  → deep_optimization_mode
  → compatible_delivery_modes
  → charging_mode / bid_or_roi_field
```

电商链路的已复核条件顺序必须按依赖方向建模：

```text
carrier
  → optimization_target_ref
  → optional deep_optimization_mode
  → compatible_delivery_modes

橙子落地页 → app内下单 → 支持深度优化 → 手动投放支持[不启用, 成交ROI]
自研落地页 → app内下单 → 支持深度优化 → 手动投放支持[不启用, 成交ROI]
```

`净成交下单`、`净成交 ROI` 曾在自动路径页面出现，但不进入当前手动投放兼容集合；其在其他模式下的完整兼容性继续按路径取证，不能反向推断。

## 4. PlatformProject 字段骨架 v0.1

`visible_when` 只记录已观察条件；空白不表示无条件。`required_state=unknown` 的字段不能进入真实执行预检。

| field_key | 平台标签/语义 | value_type | visible_when（首版） | required / enum / editable | evidence_state |
| --- | --- | --- | --- | --- | --- |
| `marketing_purpose` | 营销目的 | `dynamic_enum` | 项目新建 | `unknown / sample_only / observed_editable` | `observed` |
| `marketing_scenario` | 营销场景 | `dynamic_enum` | 随营销目的 | 当前电商分支“短视频+图文”选中，“直播”禁用；其他目的状态未知 | `observed` |
| `marketing_product` | 营销产品/商品 | `dynamic_reference` | 随营销目的/场景 | 新建页有显式必填标记；商品选择器分升级版/通用版、支持来源/类目或商品库过滤及名称/ID搜索，当前单选上限为 1 | `observed` |
| `application_ref` | 已选应用 | `reference` | 应用链路 | `conditional / dynamic_reference / observed_editable` | `observed` |
| `carrier` | 投放载体 | `dynamic_enum` | 随营销链路；电商当前集合为橙子落地页、自研落地页、字节小程序、微信小程序 | 当前电商分支橙子落地页选中；完整跨目的集合未知 | `observed` |
| `download_method` | 直接下载/落地页下载 | `dynamic_enum` | 应用下载 | `unknown / sample_only / observed_editable` | `sample_only` |
| `optimization_target_ref` | 优化目标/转化目标 | `dynamic_reference` | 随营销目的、场景、载体、产品/应用和事件资产 | 显式必填；当前电商路径为 6 个选项，选择黄金路径商品前后集合未变；可搜索并链接事件管理/落地页管理 | `observed` |
| `deep_optimization_mode` | 深度优化方式 | `dynamic_enum` | 橙子落地页/自研落地页 + `app内下单` 目标 | 自动路径曾观察到不启用、成交 ROI、净成交下单、净成交 ROI；投手复核确认当前手动投放兼容集合仅为不启用、成交 ROI。选择目标会重算深度方式和可用投放模式 | `observed` / `operator_reviewed` / `sample_only` |
| `delivery_mode` | 自动投放（UBMax）/手动投放 | `dynamic_enum` | 已选营销链路 | 当前页面初始为自动投放且手动可用；投手首个真实场景选择手动投放，以便控制版位、定向、竞价和出价组合 | `observed` / `operator_reviewed` |
| `placement` | 投放版位 | `dynamic_enum` | 电商 + 手动投放（已观察） | 通投智选/首选媒体；首选媒体当前显示今日头条、西瓜视频、抖音、番茄系媒体、穿山甲，平台仍可能为规模和成本扩量到其他流量；`conditional / observed_current_set / observed_editable` | `observed` |
| `material_supplement` | 素材补充方式 | `object` | 自动/手动条件不同 | `unknown / unknown / conditional` | `observed` |
| `aigc_material_supplement` | AIGC 动态创意（项目级素材补充） | `boolean` | 电商自动投放当前可见 | 观察默认关闭且控件可用；与 Promotion 级 AIGC 开关不是同一字段 | `observed` |
| `audience` | 地域、性别、年龄、自定义人群及模式专属定向 | `object` | 随目的和投放模式 | 电商自动默认地域/性别/年龄/自定义人群均不限；`unknown / observed_current_set / conditional` | `observed` |
| `audience_package_ref` | 新建定向/选择已有定向包 | `dynamic_reference` | 手动投放 + 用户定向 | 当前账户已有定向包选择器为空；没有时按商品内容建立并通过转化效果迭代。选择器可按名称搜索并提供相邻管理入口，本阶段不进入管理写操作 | `observed` / `operator_reviewed` |
| `behavior_interest` | 行为兴趣 | `object` | 手动投放 + 新建定向 + 自定义行为兴趣 | 行为与兴趣为两组独立条件；行为窗口当前初始 30 天；两组均支持类目词、关键词、批量添加、搜索和自动选词，每组当前最多添加 300 个 | `observed` / `operator_reviewed` |
| `douyin_influencer_targeting` | 抖音达人 | `object` | 手动投放 + 新建定向 + 自定义达人 | 可按达人名称/昵称/分类搜索，支持添加达人分类或具体达人，用于覆盖关注或互动用户；当前最多添加 30 个 | `observed` |
| `device_targeting` | 平台、设备、网络、手机品牌与价格 | `object` | 手动投放 + 新建定向 | 当前显示 iOS/Android/鸿蒙、手机/平板、WiFi/2G/3G/4G/5G、手机品牌和价格条件 | `observed` |
| `smart_expansion` | 智能放量及可放宽维度 | `object` | 手动投放 + 定向 | 当前初始关闭；可放宽维度包括地域、性别、年龄、自定义人群。首个场景一般关闭，启用属于需要解释影响范围的风险动作 | `observed` / `operator_reviewed` |
| `installed_user_filter` | 已安装用户：不限/过滤/定向 | `dynamic_enum` | 当前电商自动路径 | 默认不限；过滤禁用；定向可用 | `observed` |
| `converted_user_filter` | 过滤已转化用户的对象范围 | `dynamic_enum` | 当前电商自动路径 | 当前集合为不限、单元、项目、投放账户、公司账户、组织账户；样本默认单元 | `observed` |
| `schedule` | 日期与投放时段 | `object` | 排期区块 | 默认从今天起长期投放且时段不限；自定义日期显示起止日期输入，自定义时段显示 7×24 网格 | `observed` |
| `bidding_strategy` | 稳定成本、最优成本、最大转化等 | `dynamic_enum` | 随目标和投放模式 | 显式必填；手动路径当前显示稳定成本（初始）、最优成本、最大转化，部分策略会按目标禁用 | `observed` / `sample_only` |
| `daily_budget` | 日预算 | `money_or_unlimited` | 排期与预算区块 | 显式必填且当前可选不限/设置预算；金额本地校验为 `300～9999999.99` 元 | `observed` |
| `charging_mode` | oCPM/CPC 等 | `dynamic_enum` | 随优化目标和投放模式 | `app内下单` 当前为 oCPM，点击量当前为 CPC；由平台条件决定，不是自由输入 | `observed` / `sample_only` |
| `bid` | 项目出价/深度出价 | `money` | 稳定成本 + 支持出价的目标/深度方式 | 本地校验为 `0.01～10000` 元；不启用深度优化和净成交下单均出现出价，ROI 方式改为系数字段 | `observed` |
| `roi_coefficient` | ROI/净成交 ROI 系数 | `decimal` | `app内下单` + 成交 ROI/净成交 ROI | 本地校验为 `0.01～100`；与金额出价互斥 | `observed` |
| `monitoring_links` | 第三方监测链接 | `object` | 当前电商自动路径直接显示；其他链路可能依赖优化目标/事件配置 | 当前字段为展示、有效触点、视频播放、视频播完、视频有效播放；输入可编辑 | `observed` |
| `project_name` | 项目名称 | `string` | 项目表单 | 显式必填，按模式/时间生成且可编辑；本地校验为 1～50 个字，唯一性仍需服务端确认 | `observed` |
| `auto_project_quota` | 应用在当前账户的已建/剩余项目数 | `readonly_status` | 应用 + 自动投放 + 已选应用 | `observed_readonly` | `sample_only` |
| `search_boost` | 搜索快投 | `object` | 电商 + 手动投放（已观察） | 包含关键词、自动更新、出价系数和定向扩展；当前无关键词，平台提供 1.1 系数建议及动态规模估算，但建议值不是稳定规则；`conditional / observed_current_set / observed_editable` | `observed` |
| `learning_and_adjustment_strategy` | 学习期、跑量与后续调价策略 | `strategy_reference` | 项目创建和投后优化 | 前期通常优先取得足够投放量以建立人群画像，后续根据真实数据调整出价；深度优化方式和调价倍数不得固化为静态规则 | `operator_reviewed` |

### 4.1 PlatformProject 商品、目标与竞价校准结论

商品选择器存在两套来源，但都返回单个 `marketing_product_ref`：

| 商品库模式 | 选择器结构 | 当前账户样例 | 写边界 |
| --- | --- | --- | --- |
| 升级版 | 商品来源、类目、名称/ID 搜索、商品质量说明 | 当前 5 条共享商品，单选 `0/1` | “管理商品”不进入 |
| 通用版 | 商品库来源、商品库、名称/ID 搜索 | 当前默认商品库 1 条商品，单选 `0/1` | 工作台商品管理不进入 |

选择一个脱敏黄金路径商品后，当前 6 个优化目标没有增减。自动路径选择 `app内下单` 后曾出现深度优化方式：不启用、成交 ROI、净成交下单、净成交 ROI；投手复核确认手动投放当前只兼容不启用、成交 ROI。目标/深度方式不是独立字段：

```text
选择 optimization_target_ref
→ 平台设置当前路径的 deep_optimization_mode 初始值
→ 重新计算 compatible_delivery_modes
→ 显示 money bid 或 ROI coefficient
→ 重新计算 charging_mode
```

本批直接观察到两条兼容性规则：

- 橙子落地页或自研落地页 + `app内下单` 均支持深度优化；当前手动模式只采用不启用、成交 ROI；
- `净成交 ROI` 在本次自动路径观察中不支持切至手动投放；点击手动模式只显示不兼容提示，不切换页面状态；
- `点击量 + 手动投放` 使用 CPC，搜索快投出价系数在没有关键词时禁用；`app内下单` 当前使用 oCPM。

本地未提交校验已取得：日预算 `300～9999999.99` 元、项目/深度出价 `0.01～10000` 元、ROI 系数 `0.01～100`、项目名称 1～50 个字。它们可以进入黄金路径预检，但仍需保留平台版本和条件路径；不得外推到其他营销目的、载体或平台。

## 5. PlatformPromotion 字段骨架 v0.1

| field_key | 平台标签/语义 | value_type | 关系/条件 | required / enum / editable | evidence_state |
| --- | --- | --- | --- | --- | --- |
| `parent_project_ref` | 所属项目 | `reference` | 新建或编辑单元必须指向一个既有 PlatformProject，不能脱离项目创建 | `observed_required / dynamic_reference / observed_readonly` | `observed` / `operator_reviewed` |
| `delivery_identity` | 投放身份 | `discriminated_object` | 单元必选；`account_info` 直接使用投放账户信息，`douyin_account` 必须携带已授权抖音号引用 | `observed_required / observed_current_set / conditional`；账户信息无需附加字段，抖音号需要 `authorized_douyin_identity_ref` | `observed` / `operator_reviewed` |
| `aigc_dynamic_creative` | AIGC 动态创意 | `boolean` | 单元素材区块；与 Project 级素材补充开关不同 | 当前黄金路径新建初始态与编辑样例均显示已开启，但未出现直接开关；可能由上游配置继承 | `observed` / `platform_pending` |
| `base_material_refs` | 基础素材 | `reference[]` | 当前路径显示视频、图片、图文三个槽位和三种独立选择器 | 引用前序板块或用户上传的素材版本，并携带平台审核、质量、使用/消耗和疲劳状态；当前容量标签为 `0/30`、`0/50`、`0/10`，不能外推为所有路径统一上限 | `observed` / `operator_reviewed` / `sample_only` |
| `copy_items` | 标题/文案素材 | `object[]` | 新建页预建一个空标题输入槽，并提供推荐标题/标题库选择器 | 行计数 `1/10` 不等于已有一条有效文案；手工输入提示 5～55 字，选择器显示最多选择 9 条 | `observed` / `sample_only` |
| `native_anchor_ref` | 原生锚点 | `reference` | 与投放身份无依赖；用于介绍商品数据并提供落地页入口 | 锚点是素材库中的可管理资产，除引用外还关联样式、标题、价格等商品元数据；当前支持不启用、自动生成、手动选择，单选上限 1 | `observed` / `operator_reviewed` |
| `landing_page_ref` | 落地页 | `reference_or_sensitive_value` | 当前路径为自研落地页 | 显式必填并预建空输入槽；可手填或从落地页资产库按名称/ID选择，单个选择器上限 1，行上限标签 10 | `observed` |
| `direct_link` | 直达链接 | `sensitive_reference` | 当前路径支持自动生成/手动填写 | 电商真实场景可能使用绑定 CID 的淘宝深链；只保存受控引用或密文句柄，不在 Schema、fixture 或日志保存完整链接 | `observed` / `operator_reviewed` |
| `backup_link` | 备用链接 | `sensitive_reference` | 随手动直达链路 | 当前初始为 Universal link，Applink 可选；服务端必填条件仍不通过提交探测 | `observed` / `sample_only` |
| `product_information` | 产品名称、主图、卖点等 | `object` | 随营销产品和商品引用 | 新建页产品名、主图、卖点均有必填标记；主产品名由项目继承且禁用，可附加一个名称；主图和卖点初始均为 `0/10` | `observed` / `sample_only` |
| `call_to_action` | 行动号召 | `dynamic_enum` | 随落地页/产品链路 | 新建页有显式必填标记，显示 5 个当前候选并在预览中使用一个初始值；完整集合及平台代码待确认 | `observed` / `sample_only` |
| `creative_components` | 创意组件 | `reference_or_object[]` | 当前样例显示素材库选择、行动号召和智能生成 | 素材库资产引用已观察；独立对象还是内嵌配置、数量含义与完整枚举待确认 | `observed` / `platform_pending` |
| `source_label` | 来源 | `string` | 单元设置 | 新建页有显式必填标记并从当前业务链路预填；输入可编辑，正式服务端校验待确认 | `observed` |
| `comment_setting` | 单元评论 | `dynamic_enum` | 单元设置 | 显式必填；当前集合为不启用/启用，黄金路径新建初始为启用 | `observed` / `sample_only` |
| `category_ref` | 所属类别 | `dynamic_reference` | 单元设置 | 新建页有显式必填标记且初始为空；选择器支持搜索和分级类别，当前只观察到顶层样例，完整树待确认 | `observed` / `sample_only` |
| `brand_ref` | 品牌名称 | `dynamic_reference_or_custom` | 单元设置 | 新建页有显式必填标记且初始为空；选择器支持搜索、平台候选和“自定义品牌名称”分支 | `observed` / `sample_only` |
| `promotion_name` | 单元名称 | `string` | 单元设置 | 新建页有显式必填标记，按产品/时间生成初始名称且可编辑；长度与唯一性待确认 | `observed` |
| `platform_status` | 单元投放状态 | `dynamic_enum` | 列表/详情，不等于审核或诊断状态 | `observed_readonly / sample_only` | `sample_only` |
| `review_status` | 平台审核状态 | `dynamic_enum` | 与投放状态独立 | `observed_readonly / unknown` | `platform_pending` |
| `diagnosis_summary` | 列表诊断摘要 | `readonly_evidence` | 不替代诊断抽屉事实 | `observed_readonly` | `observed` |

### 5.1 PlatformPromotion 第三批只读校准结论

本批只读取一个现有自动投放样例的编辑页，没有修改字段、保存或提交。页面区块顺序稳定显示为：

```text
投放身份
→ 单元素材（基础素材 → 文案素材）
→ 原生锚点
→ 落地页（落地页引用 → 直达链接 → 备用链接）
→ 产品信息
→ 创意组件
→ 单元设置
→ 单元名称
```

由此得到的首版引用拓扑为：

```text
PlatformPromotionDraft
├─ parent_project_ref（必须指向既有项目；编辑样例中固定、只读）
├─ delivery_identity{mode, authorized_douyin_identity_ref?}
├─ base_material_refs[] + copy_items[]
├─ native_anchor_ref
├─ landing_page_ref + direct_link_ref + backup_link_ref
├─ product_information（主产品引用 + 可编辑补充信息）
├─ creative_components[] / call_to_action
└─ source_label + comment_setting + category_ref + brand_ref + promotion_name
```

这次观察证明了“当前编辑样例有哪些值、哪些输入可编辑、当前计数如何显示”，但不能证明：

- 新建单元的默认值、最小数量和必填规则；
- `30/50/10` 等容量是否跨营销目的、载体和投放模式通用；
- 保存后的服务端校验、审核结果和错误文案；
- AIGC 动态创意、锚点、评论、类别和品牌在其他分支的条件关系；
- 创意组件究竟是独立平台对象、素材库引用还是 Promotion 内嵌配置。

因此 `PlatformPromotionDraft` 必须保留引用与内嵌配置的区分，且所有链接使用敏感引用，不能把业务 URL、身份账号或完整平台 ID 固化到 Schema/fixture。

### 5.2 PlatformPromotion 第四批新建初始态结论

本批直接打开同一黄金路径父项目下的“新建单元”表单，只读取页面自然初始态，不选择、不填写、不保存。稳定后页面只有七个顶层区块；编辑样例中的“原生锚点”尚未出现，因此比编辑态少一个区块。后续投手复核确认这是当时页面状态的共现，不代表锚点依赖投放身份：

| 字段/区块 | 新建初始态 | 对 Schema 的影响 |
| --- | --- | --- |
| 投放身份 | 当前路径页面显示必填且初始未选择 | 必选二选一：账户信息无需附加输入；抖音号必须选择已授权账号 |
| 原生锚点 | 整个区块未出现 | 只保留初始页面观察；投手复核已否定“依赖投放身份”的因果推断，锚点独立承载商品介绍和落地页入口 |
| 基础素材 | 显式必填；视频、图片、图文均为 0 | 三类容量标签被观察，但跨路径上限仍为 `sample_only` |
| 文案素材 | 自动创建一个空标题行并显示 `1/10` | 计数代表输入行槽，不代表已有有效文案；内容约束显示为 5～55 字 |
| 自研落地页 | 显式必填；自动创建一个空输入行并显示 `1/10` | 计数代表行槽，不代表已有有效落地页引用 |
| 直达链接 | “生成方式”显式必填；内容输入为空 | 生成模式与链接内容必须拆分，内容必填性依赖模式 |
| 产品信息 | 产品名、主图、卖点均显式必填 | 主产品名由父项目营销产品继承且禁用；主图、卖点初始 `0/10` |
| 行动号召 | 显式必填；当前显示 5 个候选，预览已有初始值 | 当前集合只对黄金路径成立；智能生成复选框初始选中 |
| 来源、评论、类别、品牌、名称 | 均有显式必填标记 | 来源与名称有路径相关初始值；类别、品牌初始为空；评论初始为“启用” |

页面停留期间 URL 仅出现临时表单参数，没有 Promotion ID、已保存/自动保存提示或远端草稿证据。`temp_id` 的客户端/服务端语义未确认，因此本批只能记为“未观察到远端草稿”，不能宣称平台绝不会自动保存。

机器可读初始态见 [`fixtures/oceanengine-promotion-create-v0.1-draft.json`](./fixtures/oceanengine-promotion-create-v0.1-draft.json)。它记录空槽和必填标记，不含真实父项目、产品、账户、身份或链接值。

### 5.3 PlatformPromotion 第五批动态选择器结论

本批只打开选择器、读取结构并取消，没有选中任何身份、素材、标题、类别、品牌或落地页。已完整打开七类选择器，另识别出落地页混合控件；其引用弹层在第六批补齐：

| selector_kind | 入口与结构 | 当前空态/约束 | 取消与写边界 |
| --- | --- | --- | --- |
| `authorized_identity_list` | 投放身份的抖音号分支内联编辑器；按抖音 ID 搜索；列表项含脱敏显示名、稳定身份 ID 与授权类型 | 当前账户可见多条已授权身份，列表继续加载；未校准完整数量 | 点击选择器旁空白区域即可无更改退出；绝不点击“抖音号授权” |
| `video_asset_library` | 我的/组织共享/上传三类入口；名称或 ID 搜索；类型、来源、评估及更多过滤 | 当前账户结果为 0；默认过滤低效和审核不通过素材；选择计数 `0/30`，确定按钮禁用 | 弹层取消返回表单；上传/管理是写入相邻能力，本阶段不进入 |
| `image_asset_library` | 我的图片/上传图片；名称或 ID 搜索；管理入口 | 当前账户结果为 0；选择计数 `0/50`，确定按钮禁用 | 弹层取消返回表单；上传/管理不进入 |
| `image_text_asset_library` | 我的图文/新建图文；管理入口 | 当前未见可选条目；选择计数 `0/10`，确定按钮禁用 | 弹层取消返回表单；新建/管理图文不进入 |
| `copy_library` | 推荐标题/标题库；行业条件、关键词输入 | 当前要求先输入关键词；选择计数 `0/9` | 弹层取消返回表单；未测试无选择时“确定”的校验 |
| `category_tree` | 可搜索的分级类别下拉 | 当前只看到两个顶层样例，不能视为封闭枚举 | Escape 可关闭且不改变表单 |
| `brand_search_or_custom` | 搜索已有品牌候选，另有自定义品牌名称入口 | 当前候选仅为样例；自定义分支字段与校验未展开 | Escape 可关闭；未进入自定义分支 |

自研落地页输入框是“可直接填写 + 选择引用”的混合控件。只聚焦输入框不会打开资产列表，不能把它简单归入素材库 selector；旁侧引用入口的弹层结构在第六批补齐。

这说明行为流程编译至少需要 `selector_kind`、搜索键、来源范围、过滤器、容量、空态、确认可用条件、取消策略和 write-adjacent actions。`select_reference(ref)` 这种不区分控件类型的抽象不足以安全覆盖真实页面。

机器可读选择器契约见 [`fixtures/oceanengine-promotion-selectors-v0.1-draft.json`](./fixtures/oceanengine-promotion-selectors-v0.1-draft.json)。所有候选名称、身份 ID 和平台对象值均已脱敏。

### 5.4 PlatformPromotion 第六批引用分支与安全退出结论

本批在未保存的新建表单中选择一个已授权身份，以观察页面动态变化；随后取消整表，没有保存、提交、授权、上传或管理资产。后续投手复核已将身份与锚点解耦，因此以下仅保留页面共现证据：

- 选择身份时同时出现“原生锚点”顶层区块，初始为“不启用”，另有“自动生成”和“手动选择”；该共现不构成业务依赖。手动选择进入独立锚点资产库，支持我的/组织锚点、名称或 ID 搜索、审核状态、锚点类型和来源过滤，当前为空，单选上限 1。
- 落地页是手填值与引用并存的混合控件；引用弹层支持名称或 ID 搜索、第三方落地页类型、来源和当前时刻可用性过滤，当前为空，30 条/页、单选上限 1。
- 直达链接新建初始为“手动填写”；“自动生成”在当前无可用落地页路径下禁用。该禁用条件不能外推为平台全局规则。
- 备用链接初始类型为 `Universal link`，另有 `Applink`；单元评论初始为“启用”；行动号召当前有 5 个候选，预览使用一个初始候选；智能生成复选框初始选中。
- 身份内联编辑器可通过点击选择器旁空白区域无更改退出；整表取消仍可作为异常兜底，但不应作为正常流程步骤。

上述两个资产弹层的“管理落地页/管理原生锚点”均是写入相邻入口，本阶段没有进入。机器契约已同步到 selector fixture。

### 5.5 最终提交事件的只读定位

本批只定位动作元素、父项目门禁和页面状态，没有点击任何保存、创建、开启或状态切换动作：

| 页面 | 已定位动作 | 当前页面事实 | 编译语义 |
| --- | --- | --- | --- |
| 新建项目 | `保存并新建单元`、`保存并关闭`、`取消` | 两个保存动作均为普通按钮；必填项为空时没有预先禁用，校验可能在点击后发生 | 必须拆成 `save_project_and_continue_to_promotion` 与 `save_project_and_close`，不能合并为无语义的 `submit` |
| 新建单元第一步 | 选择已有项目/新建项目、`下一步`、`取消` | 已有项目选择是本地步骤；抽样父项目在“下一步”后返回单元数量已满 | `next` 是父项目容量和可创建性门禁，不等于创建单元 |
| 编辑已有单元 | `保存并关闭`、`取消` | 编辑页完整表单可读，最终保存动作唯一；本批未修改任何字段 | `save_promotion_and_close` 是远端写入事件，取消必须保持原对象不变 |
| 项目/单元列表 | 新建项目、新建单元、编辑、状态控件 | 新建/编辑只是进入表单；开启、暂停等状态控件另属生命周期动作 | 导航事件、表单写入事件和生命周期事件必须分开建模 |

当前账户余额与福利余额均显示为零，这会降低实际消耗风险，但不会改变上述动作的远端写入性质。只读校准继续只定位控件、前置条件和预期事件；点击后的客户端/服务端校验、对象创建和写后回读留到受控写入阶段。

## 6. 当前内部三段到平台对象的初始映射

当前 `delivery-three-tier/v1` 是 mock-only 不可变快照。以下只是迁移分析，不改变其校验、审批哈希或接口行为。

| 内部层/字段 | 平台候选语义 | mapping_state | 说明 |
| --- | --- | --- | --- |
| `ThreeTierGroup` | Delivery 内部组织单元 | `internal_only` | 不强行映射为巨量项目；只组织多个内部方案、依赖与审计上下文 |
| `group_name` | Delivery 内部组织名称 | `internal_only` | 不直接写入平台；具体项目和单元分别使用自己的名称 |
| `group_objective` | `marketing_purpose` + `optimization_target_ref` | `split_required` | 平台把营销目的和优化目标分开，且目标依赖事件资产 |
| `advertiser` | `PlatformAccount.account_ref` | `reference` | 应改用受控账户绑定，不把显示名当稳定标识 |
| `business_asset_boundary` | cookies Project 资产访问策略 | `internal_only` | 不编译为平台字段 |
| `ThreeTierPlan` | 横跨项目配置与单元候选 | `split_required` | 拆分由 Delivery 按已观察页面负责：目标、模式、版位、定向、排期、预算、竞价和出价进入项目；身份、素材、锚点、落地页和商品补充进入单元 |
| `plan_name` | `project_name` 或内部组织名 | `contextual` | 项目与单元分别命名；不得把一个内部名称同时写入两个平台层级 |
| `placement`、`audience` | PlatformProject 条件字段 | `candidate` | 是否出现取决于营销目的/模式 |
| `optimization`、`conversion` | `optimization_target_ref` | `merge_required` | 当前两个字段可能表达同一平台动态目标的不同内部来源，不能重复写入 |
| `budget`、`bid`、`schedule`、`tracking` | PlatformProject 配置 | `candidate` | 需要金额单位、时区、条件和监测宏 schema 校准 |
| `ThreeTierCreative` | PlatformPromotion 内的创意配置组合 | `candidate` | 不等于已证明存在的独立 PlatformCreative 对象 |
| `asset_version` | 内部 Asset/CreativeVersion → PlatformMaterial 映射 | `reference` | 保存内部 Asset ID、Version、ContentHash；素材上传平台后附加账户范围的平台素材引用，不依赖 Connector 决定 Delivery 字段 |
| `title`、`format`、`landing_page`、`call_to_action` | Promotion 配置 | `candidate` | 类型/数量/出现条件仍需逐分支校准 |
| `review_status` | 平台审核状态只读结果 | `not_input` | 不能作为创建草稿输入字段；应从写后查询/Connector 读取 |
| `disclosure` | 人工合规确认/平台披露字段候选 | `platform_pending` | 当前值仅为 mock 审核提示，没有页面字段证据 |

这意味着不能通过简单改名把 `group → plan → creative` 变成 `project → promotion → creative`。该表保留为历史分析证据；新领域模型不实现 ThreeTier 兼容 projector，而是从版本化平台无关 intent 直接形成目标平台 profile。

当前根模型冻结为平台无关 [`DeliveryIntent`](./schemas/delivery-intent-v1.json) 绑定判别式 [`PlatformConfiguration`](./schemas/delivery-platform-configuration-v2.json)。巨量 profile 明确为一个 Project 与零个或多个 Promotions，父关系结构隐式；磁力引擎仅表达 `CAPABILITY_PENDING`，不猜测平台字段。`delivery-three-tier/v1` 与 [`delivery-platform-configuration/v1`](./schemas/delivery-platform-configuration-v1.json) 都保持不可变历史语义。完整版本、引用、hash 与切换边界见[新契约说明](./platform-configuration-contracts.md)。

## 7. 当前批次进度

第一批已完成：

- 冻结只读校准的证据状态和字段状态词汇；
- 建立平台账户、项目、单元、素材及关键引用的对象语义；
- 建立 6 条已观察/待确认条件路径；
- 建立 PlatformProject、PlatformPromotion 字段骨架；
- 审查 `delivery-three-tier/v1` 并标出候选、需拆分、需合并、内部专属和待平台确认映射。

第二批以“电商 → 短视频+图文 → 橙子落地页 → 自动投放（UBMax）”为黄金路径，已完成：

- 确认项目表单区块顺序：营销产品与目标 → 投放模式 → 素材补充方式 → 受众倾向与过滤 → 排期与预算 → 监测链接 → 项目名称；
- 确认短视频+图文、橙子落地页和自动投放的当前选中态，直播在该分支禁用；
- 确认 AIGC 项目级素材补充开关默认关闭且可操作；
- 确认受众默认值、已安装用户可用性和已转化用户过滤范围；
- 确认自定义日期、7×24 投放时段和设置预算分别显示条件输入；
- 确认 5 类第三方监测链接输入和动态项目名称；
- 确认当前条件下优化目标集合为：按钮跳转、`app内下单`、点击量、展示量、调起店铺、店铺停留；选择脱敏黄金路径商品前后集合未改变，但该结论只对当前商品和业务路径成立。

第三批以现有自动投放单元编辑样例为证据，已完成：

- 冻结 PlatformPromotion 的八个顶层区块顺序（其中单元素材含基础素材与文案素材）和父项目、身份、素材、锚点、落地页、产品、组件引用拓扑；
- 区分 Project 级 AIGC 素材补充与 Promotion 级 AIGC 动态创意；
- 记录素材、标题、落地页、产品主图和卖点的样例计数，但不将其外推为跨路径统一上限；
- 确认标题、落地页、直达/备用链接、补充产品名、来源和单元名称等编辑控件，以及父项目和主产品名的当前只读状态；
- 冻结链接、身份和业务对象的脱敏引用规则，不在 fixture 保存真实值。

第四批以同一黄金路径下的 PlatformPromotion 新建自然初始态为证据，已完成：

- 确认投放身份、素材、落地页、直达链接生成方式、产品信息、行动号召及单元设置的显式必填标记；
- 区分“已添加行槽数”和“有效配置对象数”，修正标题、落地页 `1/10` 的计数语义；
- 记录原生锚点在该次未选择身份的初始页面中未出现；后续投手复核已否定身份与锚点的因果依赖；
- 确认类别和品牌是动态选择/输入控件，不能因底层文本框 `readOnly` 属性误判为业务只读；
- 页面未出现远端草稿证据，但临时表单参数语义仍保持 `platform_pending`。

第五批以新建页动态选择器为证据，已完成：

- 区分授权身份、视频库、图片库、图文库、标题库、类别树和品牌搜索/自定义七种 selector kind；
- 记录搜索键、来源范围、过滤器、空态、容量、确认禁用条件与写入相邻入口；
- 验证素材/标题弹层取消后回到未变更表单，表单级取消返回项目列表；
- 投手复核确认身份内联选择器可点击旁边空白区域退出，不需要人工接管；
- 确认自研落地页是手填值与平台引用并存的混合控件。

第六批以 Promotion 引用分支为证据，已完成：

- 通过本地选择已授权身份时同时观察到原生锚点区块，确认“不启用/自动生成/手动选择”三种模式及锚点资产库结构；该共现不再解释为依赖；
- 补齐落地页引用选择器的搜索、过滤、排序、分页、空态、容量与写入相邻入口；
- 冻结直达链接、备用链接、评论、行动号召和智能生成的当前路径初始态；
- 局部退出方式已由投手复核为“点击选择器旁空白区域”，整表取消仅保留为其他异常时的兜底。

第七批回到 PlatformProject 黄金路径，在不保存、不提交的本地表单中完成商品与竞价校准：

- 补齐“我的商品-升级版/通用版”双库结构、搜索与过滤条件；当前脱敏样本分别可见 5 条和 1 条，单选上限 1；
- 选择脱敏商品后，当前 6 个优化目标候选未改变；选择 `app内下单` 后出现“不启用/成交ROI/净成交下单/净成交ROI”四种深度优化方式；
- 确认深度优化方式会约束投放模式：当前“净成交ROI”不支持手动投放，切换为“不启用”后可进入手动投放；
- 记录当前样本的计费示例：`app内下单` 显示 `oCPM`，`点击量 + 手动投放` 显示 `CPC`；这些仅是条件样本，不是完整映射表；
- 通过未保存表单的客户端校验确认日预算为 300～9,999,999.99 元，出价为 0.01～10,000 元，ROI 系数为 0.01～100，项目名称为 1～50 字符；
- 记录手动路径显式必填项：营销产品、优化目标、投放时间、投放时段、竞价策略、日预算、付费方式和项目名称。监测链接未显示必填标记，但不能仅据此宣称跨路径可选。

第八批完成 Q1～Q3 首轮业务评审：

- 废止面向投手的 `PlatformAccount/PlatformProject/PlatformPromotion` 术语问题，产品语言统一为“投放账号 → 项目 → 单元”；
- 将黄金业务流前移到商品、落地页、锚点、人群包和素材准备，并覆盖素材上传、平台审核/质量过滤、多项目/多单元创建、最终开启与投后优化；
- 明确首个真实电商场景以手动投放为主，重点覆盖版位、人群包、行为兴趣、智能放量、连续时段、学习期和数据驱动调价；
- 将深度优化方式和倍数调价改为数据驱动策略，不固化为静态推荐；
- 确认历史 ThreeTier 差异分析、页面 Schema 和稳定引用由 Delivery 自主维护；当前目标以 `DeliveryIntent` + 平台 profile 建模，不要求其他 Owner 或兼容 projector 解释 ThreeTier；
- 冻结完整场景覆盖目标，缺少的营销目的、载体、目标、模式、定向、竞价和单元形态持续加入覆盖矩阵；
- 纠正投放身份语义：字段必选，账户信息分支无需更多输入，抖音号分支必须引用已授权账号；
- 将两段固定审批修正为历史 mock 行为；目标流程默认只在最终真实创建/开启前进行一次确认，高风险动作按风险追加确认。

第九批完成身份/锚点纠偏与电商手动路径页面校准：

- 确认单元必须在既有项目下创建，不能用脱离父项目的新建页观察外推真实单元规则；
- 冻结投放身份的 `account_info` / `douyin_account` 判别联合结构，以及抖音号授权引用约束；
- 明确锚点与投放身份无依赖，锚点只负责商品介绍数据和落地页入口；
- 冻结“载体 → 优化目标 → 可选深度优化方式 → 兼容投放模式”的条件顺序；电商橙子/自研落地页 + app 内下单的手动深度方式当前为不启用、成交 ROI；
- 补齐电商手动路径的版位、定向包、行为兴趣、抖音达人、设备网络、智能放量、竞价与搜索快投控件；
- 将“其他营销目的下定向字段是否移动”等模糊问题改为逐路径页面校准任务，不再要求投手从产品实现角度回答。

正式只读契约见 [`schemas/oceanengine-bidding-schema-v0.1.json`](./schemas/oceanengine-bidding-schema-v0.1.json)。机器可读的非执行观察 fixture 见 [`fixtures/oceanengine-ecommerce-ubmax-v0.1-draft.json`](./fixtures/oceanengine-ecommerce-ubmax-v0.1-draft.json)、[`fixtures/oceanengine-leads-short-video-v0.1-draft.json`](./fixtures/oceanengine-leads-short-video-v0.1-draft.json)、[`fixtures/oceanengine-app-download-android-v0.1-draft.json`](./fixtures/oceanengine-app-download-android-v0.1-draft.json)、[`fixtures/oceanengine-promotion-edit-v0.1-draft.json`](./fixtures/oceanengine-promotion-edit-v0.1-draft.json)、[`fixtures/oceanengine-promotion-create-v0.1-draft.json`](./fixtures/oceanengine-promotion-create-v0.1-draft.json) 与 [`fixtures/oceanengine-promotion-selectors-v0.1-draft.json`](./fixtures/oceanengine-promotion-selectors-v0.1-draft.json)。

### 7.1 只读数据获取结论与后续任务

当前账户、当前已访问路径和禁止写操作边界内，可安全取得的高优先级页面数据已经完成采集。电商和销售线索形成连续路径；应用下载 Android 在事件资产处形成明确阻塞证据。继续重复浏览相同页面不会补出新的高价值契约，其他路径作为后续增量版本继续扩展，不再阻塞 v0.1。

后续任务按 Delivery 自身职责推进：

1. 在 Delivery 实现中消费已冻结的 `PlatformProjectDraft`、`PlatformPromotionDraft`、`SelectorContract` 与 `ActionBoundary`；
2. 应用调起、预约下载、iOS、鸿蒙、小程序和后续开放目的按新证据追加 v0.2 候选，不修改 v0.1 已冻结语义；
3. 当前应用取得合格事件资产后补充目标、模式、计费和单元字段；此前保持 `blocked_by_event_asset`；
4. 无法取得稳定 ID 的对象继续使用 unresolved reference，不以显示名冒充平台标识；
5. 受控写入阶段获得明确授权后验证服务端提交校验、保存后对象关系、开启状态、错误文案和写后回读；
6. 将投手提供的平台数量限制保存为 `operator_input`，后续以平台页面或公开契约校验，不直接固化为代码常量。

### 7.2 写操作是否属于智能投放

余额为零只影响资金风险，不决定模块职责。按“智能投放生成和持续优化项目/单元，其他模块管理其平台资产”的边界，写操作分为三类：

| 操作 | 是否会在智能投放发生 | Delivery 职责与阶段 |
| --- | --- | --- |
| 保存/创建项目和单元 | 会，是核心产出 | 只读校准阶段仅定位和建模；受控写入阶段在最终人工确认后执行 |
| 修改预算、出价、排期、定向 | 会，是投后优化核心动作 | 只作用于 Delivery 自建或明确允许的关闭项目；每次变更生成差异、风险说明和写后回读 |
| 开启、暂停、重启项目或单元 | 会，是执行与安全生命周期 | 开启是投放交付动作；暂停是止损/Kill Switch；重启需重新检查预算、排期和状态。只读校准阶段均不点击 |
| 删除项目或单元 | 可能，但不是常规优化 | 仅用于清理 Delivery 自己创建的测试对象或明确补偿流程；默认优先暂停/关闭，禁止删除他人对象 |
| 选择素材并绑定到单元 | 会，是核心组装动作 | Delivery 消费已存在、可投且带审核/质量状态的素材引用 |
| 上传、创建、管理素材 | 默认不会 | 属于素材板块的平台资产生命周期；Delivery 缺少可用平台素材时返回 unresolved/blocking reference，不复制素材中心 |
| 配置项目内定向字段 | 会 | 性别、年龄、地域、行为兴趣、达人、智能放量等属于项目 Schema |
| 创建/修改可复用人群包 | 默认不会 | 作为独立平台资产交由相应资产所有者；Delivery 可引用已有包或使用项目内联定向 |
| 创建/修改商品、落地页、锚点 | 默认不会 | 属于上游平台资产管理；Delivery 只选择和绑定稳定引用，并校验状态与链路兼容性 |
| 选择账户信息或已授权抖音号 | 会 | 投放身份配置属于单元；仅允许账户信息或现有授权引用 |
| 新增/撤销抖音号授权 | 不会 | 永远是人工扫描/账户权限操作，超出 Delivery 职责 |
| 读取素材审核、项目审核、诊断和驳回状态 | 会，是核心只读反馈 | 读取既有平台事实并影响预检/建议；不为制造驳回样本而提交资产或对象 |
| 主动制造审核/驳回场景 | 不会 | 使用既有状态或显式 mock/replay，不把平台审核当成 Delivery 测试工具 |

因此未来真实智能投放最小写集合是：`create/update project`、`create/update promotion`、`status control`，以及对既有素材、商品、身份、落地页、锚点和人群包的引用绑定。资产创建、身份授权和人为制造审核状态不进入该集合。

Connector 的正式代码、对象枚举和指标契约由其文件夹所有者负责；Delivery 仅等待可消费输出，不把 Connector 命名或实现列为平台对象拆分的阻塞项，也不修改其文件。

因此只读业务校准可以关闭并冻结 `oceanengine-bidding-schema/v0.1`。它是当前已观测路径的版本化只读契约，不宣称是巨量永久全量表单，也不授权真实写入；所有保存、创建、开启、状态控制和服务端校验继续标记为 `write_validation_pending`，进入受控写入阶段。
