# 智能投放对数据洞察 Connector 的消费需求

| 属性 | 内容 |
| --- | --- |
| 状态 | 消费需求已发布；Delivery consumer port、mock/replay 已合并到 `upstream/main`；等待 Connector Owner 发布正式接口/事件契约 |
| 日期 | 2026-08-07 |
| 文档 Owner | 智能投放模块（仅维护消费需求，不定义或实现 Connector） |
| 关联 | [广告数据 Connector](../10-ad-data-connectors.md)、[智能投放架构](./architecture-and-implementation.md)、[脱敏走查证据](./oceanengine-readonly-calibration.md)、[收口与配置契约](./read-only-calibration-closeout.md) |

## 1. 边界与决策

数据洞察模块拥有平台数据的接入、采集、授权引用、同步、原始记录、标准化、对象映射、口径、新鲜度、质量和对外发布；智能投放只消费其已发布的数据契约。两个模块不得各自创建巨量 API 客户端、Computer Use 只读采集器或第二套指标事实库。

智能投放拥有平台无关意图、目标平台配置、“投放账号上下文 → 项目 → 单元”的巨量语义、页面 schema 校准、配置编译、预检、最终确认、受控写入、执行证据、告警和建议。历史 ThreeTier 差异分析与页面字段/写入步骤由 Delivery 维护；Connector 只负责把获准读取的页面/API/导出数据变成可追溯事实。

Computer Use 的环境、登录会话、接管和截图治理由共享平台运行时负责。数据洞察通过该运行时执行只读采集；智能投放通过该运行时执行已获批的校准或写入步骤。两者不绕过验证码、登录或平台权限。

## 2. 当前过渡方式

Connector 尚未具备巨量真实 API 或 Computer Use 读取实现时，智能投放继续使用确定性 mock fixture。所有投放侧指标、告警、建议、快照和页面必须显式标记 `source=mock`、`is_simulated=true` 与 fixture/dataset 版本；不得把它们描述为真实投放数据。

Delivery 已适配一个只读的 `InsightsConsumer` 消费端口（旧规划中的 `DeliveryMetricsReader` 需求名不再作为实现名）：

- `MockInsightsReader`：读取确定性 fixture，覆盖 usable、empty、stale、incomplete、schema mismatch、unavailable；
- `ReplayInsightsReader`：按 Organization + Project 校验范围并回放不可变快照；
- `SimulationInsightsReader`：把同一 Execution/SimulationRun 的指标窗口归一化为消费端口事实；
- `InsightsConnectorReader`：尚未实现，仅在数据洞察 Owner 发布稳定接口、样例与消费者契约测试后接入；
- 当前运行时不是请求级 source 路由：`Service.Insights` 是启动时注入的单一 `InsightsConsumer`。主程序当前固定注入 `SimulationInsightsReader`；若未注入 Consumer 且存在 Repository，Service fallback 到 Simulation；只有没有 Repository 时才 fallback 到 `MockInsightsReader`。`ReplayInsightsReader` 主要通过显式构造/测试使用。`InsightsQuery` 当前没有 `source` 字段，快照/指标的 `source` 是输出事实元数据；请求级 source 路由列为后续能力，不设全局“真/假”开关，也不静默以 mock 覆盖数据质量故障。

该端口是投放模块内部的消费适配层，不是对洞察表、仓储或凭据的直接访问。PR #38（`25fc8cf`，合并提交 `f3ee8a9`）已进入 `upstream/main`，其 `verify`、`migrations`、`Repository quality`、`Secret scan` 均通过。

当前状态因此明确为：Delivery 端口、Simulation bridge、mock/replay reader 已完成；主程序默认走 Simulation bridge；Connector 端实现、真实数据读取和影子分析尚未开始。没有 Connector 时使用 Simulation/mock 是启动装配结果，不是请求级 source 选择或静默降级。

## 3. 交付给数据洞察 Owner 的最小需求

### 3.1 对象与时间范围

Connector 发布的每个事实至少可按以下条件过滤或识别：

- `organization_id`、`project_id`、`platform`、`account_ref`/广告主稳定标识；
- 版本化的 `platform_object_kind` 与 `platform_object_id`；只读走查已证明首期至少需要表达投放账户、项目、单元、基础素材四种粒度及项目到单元的父子关系，正式枚举名由 Connector 实现所在文件夹的所有者维护；
- 指标发生窗口、平台账户时区、`data_through`、读取/同步时间；
- `data_source_id`、`import_batch_id`、原始字段/导出证据引用、指标 schema/归因窗口/币种版本。

投放只把这些外部对象映射到自己的 `DeliveryPlanVersion`、`ChangeSet` 和 `Execution` 只读引用；不反向写洞察的对象映射或指标表。

### 3.2 指标与质量

首期 P0 请求指标为消耗、展示、点击、转化；只读走查报表还稳定暴露深度转化数，建议作为 P1 canonical 基础计数，是否首发由数据洞察 Owner 确认。视频播放、完播、收入等按数据源实际可用性可选提供。金额使用分与币种，派生指标（CPM、CTR、CPC、CVR、CPA、深度 CVR、深度 CPA、ROAS）必须带公式/口径版本，分母为零时返回不可用而不是零。

### 3.2.1 只读走查字段和别名输入

| 报表粒度 | 需要保留的对象/维度字段 | 关联要求 |
| --- | --- | --- |
| 账户 | `时间-天`；账户 ID 不在行内 | 从查询上下文或 ImportBatch evidence 补充 `account_ref` |
| 项目 | `时间-天`、`项目ID`、`项目名称` | `项目ID` 映射平台项目标识，不映射 cookies `project_id` |
| 单元 | `时间-天`、`单元名称`、`单元ID`、`转化目标`、`单元出价` | 保存父项目关联；页面“优化目标”和导出“转化目标”是来源别名，不是两个 canonical 字段 |
| 基础素材 | `素材名称`、`素材内容`、`素材类型`、`素材ID` | 默认无行内日期，必须保留查询窗口；素材映射可为空但需要质量状态 |

四类报表共享的源指标为 `消耗`、`展示数`、`平均千次展现费用(元)`、`点击数`、`点击率%`、`平均点击单价(元)`、`转化数`、`平均转化成本`、`转化率%`、`深度转化数`、`深度转化成本`、`深度转化率%`。其中计数和消耗应作为 canonical 基础事实；平均值和比率优先由版本化公式计算，同时保留平台源值用于对账。

字段映射不能只保存“当前中文标题 → canonical 名称”。每个 schema evidence 至少需要：`report_kind`、`source_field_name`、`canonical_field_name`、`source_unit`、`source_schema_version`、查询时间粒度、观察/抓取时间和原始文件/页面 evidence。字段名称差异详见[只读走查证据摘要](./oceanengine-readonly-calibration.md)。

每个读取结果必须携带 `healthy`、`delayed`、`partial`、`mapping_incomplete`、`tracking_broken`、`reconciling` 或 `blocked` 等质量状态及原因。除 `healthy` 外，投放可以展示观察提示和人工复核入口，但不得生成确定性告警、自动建议或写入动作。

### 3.3 读取与事件

数据洞察 Owner 需选择并发布一种或两种受支持方式：

1. Project-scoped 的只读查询 API，支持对象范围和时间窗口；
2. 版本化的数据更新事件，事件只包含数据源、对象范围、窗口、快照版本、质量和证据定位，消费者再按权限查询详情。

正式契约须给出 JSON Schema/OpenAPI 或 AsyncAPI、示例、字段分级、文件所有者、消费者、兼容策略、最长延迟和 Consumer Contract 测试。Delivery 在此之前不假定路径、表名或事件名，也不修改 Connector 目录文件。

### 3.4 消费者验收样例

数据洞察 Owner 发布首个可消费版本时，至少提供以下脱敏 fixture 和 Consumer Contract 测试：

1. 账户、项目、单元、基础素材各一个非空日粒度样例，包含 `account_ref`、平台对象标识、父对象关联、窗口、时区、币种、口径版本、质量和 evidence；
2. 一个只有表头、没有事实行的空报表样例；它可以登记 schema/import evidence，但不得伪造一条全零 MetricFact；
3. 一个页面“优化目标”与导出“转化目标”的别名归一样例；源字段仍可追溯；
4. 一个默认无行内日期的基础素材报表样例；输出仍保留查询窗口；
5. 一个未知素材映射和一个非 `healthy` 质量样例；Delivery 只展示观察提示，不产生确定性建议或写入动作；
6. 同一事实的重复导入样例；不会产生重复 canonical 记录。

## 4. 分阶段协作

| 阶段 | 数据洞察 Owner | 智能投放 Owner |
| --- | --- | --- |
| 只读业务校准 | 在已允许的专用竞价投放账户所有可访问项目中验证只读采集路径、数据质量和可发布对象/指标；处理登录或验证码时请求管理员接管 | 只读校准广告组/计划/创意页面语义、字段依赖、动态表单与对象映射需求；持续使用 mock 监控 |
| 行为流程编译与影子分析 | 发布可供消费的指标快照/API/事件及样例 | 接入消费端口，在数据质量合格时运行影子告警/建议；把异常结论绑定到 Connector evidence |
| 受控写入 | 保持只读数据回读、对账和新鲜度服务 | 经审批的 Computer Use 受控写入、回填核验和执行证据；不承担指标采集 |

### 4.1 当前 Go/No-Go

- **Go**：`DeliveryIntent` 与判别式平台配置领域契约已冻结；后续行为流程编译器和 mock/replay 回归可继续，不需要 ThreeTier 兼容 projector，也不修改 Connector 或依赖真实指标。
- **No-Go**：真实 Connector 数据影子告警/建议，直到正式对象/指标/质量契约、样例和 Consumer Contract 测试可消费。
- **No-Go**：任何巨量平台创建、开启、暂停或优化写入；本轮只冻结只读契约和后续 PR 切片。

## 5. 与当前洞察实现的已知差距

以下是消费侧审查结果，不授权智能投放修改 `internal/systems/insights`、`api/openapi/insights-v1.yaml` 或洞察数据库：

1. 当前 `Platform` 使用 `douyin` 等枚举，尚不明确它是否同时代表巨量引擎广告平台；需确认沿用 `douyin` 还是增加独立平台代码。
2. 当前 OpenAPI 的 `MetricRow.platform_object_kind` 和 AssetMapping 只允许 `creative`、`ad`，不足以无损表达只读走查已证明的账户、项目、单元、基础素材粒度及父子关系。
3. 当前 `MetricCounts` 覆盖展示、点击、转化、播放、消耗和收入，但没有深度转化基础计数；仅放进 `raw` 无法支持稳定的 canonical 深度 CPA/CVR 消费。
4. 当前 `DataSource.FieldMapping` 是简单字符串映射，尚无明确位置保存报表种类、源单位、schema 版本、别名、查询时间粒度和 schema-only evidence。
5. 当前导入请求要求至少一行 `MetricRow`；空报表不应被伪造成全零事实，需要明确独立 schema 登记方式或允许零事实的成功 ImportBatch。
6. 当前事实模型只有单个 `platform_object_id`，父项目关联、单元的优化目标/出价等对象维度应由对象目录/元数据契约承载，不能全部塞进指标 `raw` 后让消费者自行解析。

这些差距不阻塞 Delivery 继续使用显式 mock，也不授权 Delivery 建立第二套采集器；它们阻塞的是“真实 Connector 数据已可无感切换”的声明。

## 6. Connector 接入前置条件

以下内容由对应 Connector 文件夹所有者在实现中冻结；它们不阻塞 Delivery 继续维护页面 Schema、ThreeTier 拆分和 mock 消费端口：

1. 巨量引擎使用现有 `douyin` 平台代码还是独立代码；账户、项目、单元、基础素材采用什么正式 `platform_object_kind`，以及如何表达父子关系。
2. 深度转化是首发 canonical 指标还是只保留 Raw；优化目标、单元出价等对象维度通过什么对象目录/元数据接口发布。
3. 报表 schema/字段别名/单位/查询时间粒度和空报表 evidence 的保存位置与版本规则。
4. Connector 的首个对外交付方式（查询 API、事件或两者）、脱敏 fixture、Consumer Contract 测试以及可接受的同步延迟。
