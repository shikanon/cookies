# 智能投放只读校准收口与 Delivery 配置契约

## 运行时执行状态（2026-08-10）

此前冻结的切换规则现已在代码中执行。新的公共 Plan 写请求必须同时提交完整 DeliveryIntent 与 tagged PlatformConfiguration；旧 flat-plan 与 ThreeTier 写请求返回 `LEGACY_CONFIGURATION_UNSUPPORTED`。持久化旧版本在读取后标记为 `legacy_unsupported`/只读，保留原 canonical hash，不能进入新预检、提交、审批或执行链。

旧 `/configuration:compile` 与 `/configuration:override` 仅保留为稳定的 deprecated 错误面。前端主路由为 `/delivery/configuration`；`/delivery/three-tier` 作为深链兼容别名重定向，旧数据只展示不编辑。

| 属性 | 内容 |
| --- | --- |
| 状态 | 只读业务校准已收口；`oceanengine-bidding-schema/v0.1` 保持冻结；Delivery-owned 新配置模型进入实现准备 |
| 基线 | `upstream/main` = `f3ee8a9`；Delivery Insights consumer PR #38 已合并 |
| 范围 | 只读证据、消费端口、mock/replay、配置模型和后续 PR 切片 |
| 明确不做 | 不修改 Insights/Connector；不做持久化迁移、API/前端重构或巨量平台写入 |
| 关联 | [只读证据摘要](./oceanengine-readonly-calibration.md)、[业务 Schema](./oceanengine-schema-calibration.md)、[Connector 消费需求](./insights-connector-consumer-requirements.md)、[机器契约](./schemas/delivery-platform-configuration-v1.json) |

## 1. 收口结论

只读校准可以关闭为一个可追溯的业务基线：投放账号下的真实页面语言统一为“投放账号 → 项目 → 单元”，首条已连续验证的业务路径是电商手动投放；平台动态字段、提交校验和真实写后回读仍属于 `platform_pending` 或 `write_validation_pending`。历史 mock 闭环继续作为实现回归面，不再作为真实平台层级或投手标准流程。

Delivery 可以继续实现新的配置模型和行为流程编译器，但真实数据影子告警/建议必须等待 Connector 发布正式消费契约、对象映射、质量/新鲜度和 Consumer Contract 测试。Connector 的实现、表结构、枚举和采集器不属于本轮改动范围。

## 2. 实现和 PR/CI 事实

| 能力 | 已验证事实 | 当前含义 |
| --- | --- | --- |
| Delivery Insights consumer port | `internal/systems/delivery/insights_consumer.go` 已提供 `InsightsConsumer.Read(InsightsQuery)`、`DeliveryInsightsSnapshot`、对象父子关系、窗口、质量、来源和 evidence 校验 | 告警只消费版本化事实，不直接读取洞察仓储 |
| Mock | `MockInsightsReader` 嵌入六类 fixture：usable、empty、stale、incomplete、schema mismatch、unavailable | 仅在 `Service.Insights` 未注入且没有 Repository 时作为 fallback |
| Replay | `ReplayInsightsReader` 按 Organization + Project 校验范围，并将来源标记为 `replay` | 显式构造/测试使用，可重放脱敏快照；不代表实时平台数据 |
| Simulation bridge | `SimulationInsightsReader` 将同一 Execution/SimulationRun 的指标窗口归一化为消费端口事实 | 当前主程序启动时注入的单一 Consumer；规则不读取原始指标表 |
| 质量门 | 只有 `quality=usable`、新鲜窗口、完整基础指标和合法对象层级才允许确定性告警/建议 | 空、滞后、不完整、schema 不匹配和不可用只产生观察提示 |
| PR #38 | `feat(delivery): consume execution-scoped insights facts`，head `25fc8cf`，merge `f3ee8a9`，状态 `MERGED` | 已进入 `upstream/main`，不再是本地待合并需求 |
| CI | `verify`、`migrations`、`Repository quality`、`Secret scan` 均为 `SUCCESS` | 以 `f3ee8a9` 作为本轮文档和模型审查基线 |

因此，本地规划中的“消费需求、消费端口、mock/replay 待实现”应更新为：消费需求已发布并等待 Connector 正式输出；消费端口、mock、simulation bridge 和 replay 已在 Delivery 完成；当前运行时由启动装配选择单一 Consumer，请求级 source 路由是后续能力；真实 Connector adapter 和真实数据影子分析仍未开始。

## 3. 已验证事实

- 只读会话覆盖了投放账号、项目列表/表单、单元列表/表单、数据中心和账户/项目/单元/基础素材报表；期间没有保存、提交、启停、删除、复制或预算/出价修改。
- 平台稳定业务骨架是 `PlatformAccount → PlatformProject → PlatformPromotion`；内部 `ThreeTierGroup/Plan/Creative` 不是平台层级。
- 项目承载营销目的/场景、营销产品或应用、载体、目标、投放模式、定向、排期、预算、竞价/出价、监测和项目名称。
- 单元必须属于既有项目，承载投放身份、素材/文案、原生锚点、落地页、商品信息、创意组件和单元设置。
- 优化目标、深度优化方式、计费和模式是条件字段；商品/应用和事件资产会改变可用目标，不能编译成静态全局枚举。
- 空报表只证明列名、默认粒度和字段别名；不能证明真实值类型、延迟、归因、重复或对账结果。
- 素材、商品、落地页、锚点、人群包和身份是外部资产；Delivery 只保存稳定引用和状态快照，不复制其他模块实体。
- 预检、模拟、影子分析和采纳建议本身不写平台；目标流程默认只在最终真实创建/开启前确认一次，高风险动作再追加针对性确认。

## 4. 首条电商手动投放路径

```text
需求与商品链接
→ 进入既有投放账号
→ 准备商品、落地页、锚点、人群包
→ 上传素材并等待平台审核/质量过滤
→ 选择可投、高质量、使用较少的素材
→ 为商品/优惠券/素材/定向组合创建一个或多个项目
→ 配置营销产品、目标、手动投放、版位、定向、排期、预算、竞价和监测
→ 在每个项目下创建一个或多个单元
→ 配置账户信息或已授权抖音号、素材、锚点、敏感落地链路和商品信息
→ 预检、模拟或影子分析
→ 生成最终操作包并进行一次最终人工确认
→ （未来受控写入）创建并开启项目/单元
→ 读取数据，按素材、定向、预算、出价和项目组合持续优化
```

当前实现只允许走到只读定位、mock/replay 编译和操作包；最后两步保持 `write_validation_pending`，不得点击平台。

## 5. 完整场景覆盖矩阵

状态含义：`observed` 为页面事实，`implemented` 为 Delivery 已有 mock/消费能力，`reviewed` 为投手评审，`platform_pending` 为平台证据不足，`blocked_by_event_asset` 为真实条件阻塞，`write_validation_pending` 为未获授权的写入验证。

| 场景 | 页面/业务覆盖 | Delivery 状态 | 继续条件 |
| --- | --- | --- | --- |
| 电商 → 短视频+图文 → 橙子/自研落地页 → 手动投放 | 商品、目标、深度方式、版位、定向包、行为兴趣、达人、设备网络、智能放量、竞价、搜索快投、排期/预算、身份、素材、锚点和落地页 | `observed + reviewed`；首条业务路径 | 可进入模型实现；真实提交保持 `write_validation_pending` |
| 电商 → 自动投放（UBMax） | 商品双库、AIGC、受众、排期/预算、监测和目标样本 | `observed + sample_only` | 作为自动模式对照，不外推完整目标/枚举 |
| 销售线索 → 短视频+图文 | 商品/智能识别、线索方式、橙子/私信/自研落地页/小程序分支、目标和定向 | `observed + sample_only` | 补齐平台代码、单元条件和提交校验 |
| 应用下载 → Android → 已有应用 | 应用选择、下载方式、商店优先、版本、过滤和监测链接 | `observed + blocked_by_event_asset` | 取得合格事件资产后再补目标/计费/单元字段 |
| 应用调起、预约下载、iOS、鸿蒙、直播、小程序、其他开放目的 | 入口或标签可见，后续动态表单未完整走查 | `sample_only + platform_pending` | 新证据追加版本，不修改 v0.1 已冻结语义 |
| 多项目/多单元 | 项目→单元父子关系已确认；容量、继承和不同目的差异未全量证明 | `reviewed + platform_pending` | 继续只读校准和模型条件测试 |
| 投后优化 | 素材/定向/预算/出价/组合调整的业务意图已评审 | mock/模拟 `implemented`；真实数据 `platform_pending` | Connector 质量合格后才可运行真实影子分析 |
| 消费事实 usable | 账户/项目/单元或 promotion/素材对象和四项基础指标齐全 | `implemented`（mock/replay/simulation） | Connector 发布同等契约后再切换来源 |
| 消费事实 empty/stale/incomplete/schema mismatch/unavailable | 数据不可用于确定性结论 | `implemented` | 只展示原因/evidence，不生成告警、建议或写入 |
| 保存、创建、开启、暂停、重启 | 动作位置已定位，服务端校验和写后回读未验证 | `write_validation_pending` | 明确授权、测试项目、硬上限、Kill Switch 和人工责任人 |

## 6. 剩余未知项

| 未知项 | 当前状态 | 不能做的推断 |
| --- | --- | --- |
| 其他营销目的、载体、目标、版位、定向和竞价的完整枚举 | `platform_pending` | 不把单账户样本写成封闭枚举 |
| 目标平台代码、深度优化和计费兼容关系 | `platform_pending` / `blocked_by_event_asset` | 不凭显示名或其他账户经验生成目标 |
| 保存/提交后的客户端与服务端错误、幂等和写后对象关系 | `write_validation_pending` | 不执行平台写入，不宣称校验已通过 |
| 真实报表数值、归因窗口、延迟、重复导入和对账 | Connector `owner_pending` | 不把空表或 mock 指标当真实数据 |
| 平台素材审核、质量、使用历史和疲劳的真实引用 | 外部资产/Connector 依赖 | 不在 Delivery 建立第二素材中心 |
| 项目/单元容量、字段继承、不同路径的只读字段 | `platform_pending` | 不用一个编辑样例外推全量规则 |

## 7. Connector 阻塞项与非阻塞项

### 非阻塞

- Delivery-owned `PlatformProjectDraft`、`PlatformPromotionDraft[]` 的字段归属、条件表达、稳定引用和历史兼容设计。
- 以现有 `MockInsightsReader`、`ReplayInsightsReader` 和 `SimulationInsightsReader` 做模型/行为编译器的单元测试、fixture 回放和证据绑定。
- 只读页面 Schema、SelectorContract、ActionBoundary、最终确认边界和 `platform_pending` 传播。
- 受控写入前的操作包编译；它只能描述动作和停止条件，不能调用平台。

### 阻塞真实影子分析

- Connector 正式 JSON Schema/OpenAPI/AsyncAPI、版本和兼容策略。
- 账户/项目/单元/基础素材对象目录、父子关系、稳定平台 ID 和内部映射。
- 基础指标、深度转化、单位/币种、归因窗口、`data_through`、质量/新鲜度和 evidence 的可消费样例。
- 空报表、重复导入、非 healthy 数据和字段别名的 Consumer Contract 测试。

这些是“真实数据影子分析”的外部依赖，不是 Delivery 继续做模型和编译器的阻塞项。Delivery 不等待 Connector 正式实现，也不修改 `internal/systems/insights`、Connector 文件或其 OpenAPI。

## 8. Go/No-Go 决策

| 能力 | 决策 | 依据 |
| --- | --- | --- |
| 新配置模型与字段映射 | **Go** | 只依赖已冻结只读 Schema、内部快照和 Delivery 自有引用规则 |
| 行为流程编译器（只读定位、条件、确认、恢复分支） | **Go** | 可用现有页面 evidence、SelectorContract、ActionBoundary 和 mock/replay fixture 验证 |
| mock/replay 告警和建议回归 | **Go** | PR #38 已提供消费端口、质量门和可重放输入 |
| 真实 Connector 数据影子告警/建议 | **No-Go** | 缺正式对象/指标/质量契约和 Consumer Contract 测试 |
| 巨量平台创建、开启、暂停或优化写入 | **No-Go** | 本 Goal 不含授权、Computer Use 写入、写后回读和 Kill Switch |

## 9. 历史 v1 目标配置模型冻结

> 本节至第 12 节记录只读校准收口时的 `delivery-platform-configuration/v1` 设计，保留用于解释历史 fixture 和决策。当前领域契约已升级为平台无关 `delivery-intent/v1` + 判别式 `delivery-platform-configuration/v2`，以[`DeliveryIntent` 与平台配置契约](./platform-configuration-contracts.md)为准；v1 不修改、不扩展，也不作为新运行时的中间 projector。

### 9.1 根模型

新模型的根是不可变的 `DeliveryPlanVersion`，其平台目标配置只保存 Delivery-owned 草稿，不复制平台或其他模块业务实体：

```text
DeliveryPlanVersion (immutable, version + canonical_hash)
└─ PlatformProjectDraft (one)
   └─ PlatformPromotionDraft[] (zero or more; business language: units)
```

本节当时的机器契约版本为 `delivery-platform-configuration/v1`。`delivery-three-tier/v1` 与该 v1 配置现在都是不可变历史契约，不能通过改名、字段覆盖或重新计算 hash 变成 v2。新 v2 领域模型已在内存/文档契约中实现，持久化字段和 API 切换另行立项。

每个 payload 结构上只含**一个** `PlatformProjectDraft`，所有 `PlatformPromotionDraft[]` 都属于它：项目—单元父关系是结构隐式的，单元不携带 `parent_project_draft_id` 字段（父字段是冗余的，机器契约 `additionalProperties: false` 会拒绝），也不引用未提交的 `platform_id`。

Envelope 的 hash projection 是机器可执行的：`canonical_hash = SHA-256(RFC 8785 JCS(payload))`。`payload` 包含平台业务字段、`source`、`platform` 以及自描述版本标记 `payload_schema_version`（项目/单元内另有 `draft_schema_version` 常量）；`canonical_hash`、组织/Project/Plan 身份、envelope `version_number` 以及 `compilation_metadata`（evidence、条件、页面动作）都在 payload 外。机器契约中的 `hash_algorithm` 固定为 `RFC8785-JCS-SHA256(payload)`，因此不存在自引用，也不会因页面 evidence 或动作编译元数据变化而使业务审批 hash 失效。

### 9.2 字段归属

| 目标字段 | 归属 | 条件/状态表达 |
| --- | --- | --- |
| `account_ref` | 项目上下文 | 稳定投放账号引用；未解析为 `unresolved` |
| `marketing_purpose`, `marketing_scenario`, `marketing_product_ref`, `application_ref`, `carrier` | 项目 | 按路径条件出现，未知值 `platform_pending` |
| `optimization_target_ref`, `deep_optimization_mode`, `delivery_mode` | 项目 | `carrier → target → optional deep mode → compatible mode`；事件资产不足为 `blocked_by_event_asset` |
| `targeting`, `schedule`, `budget_and_bidding`, `monitoring`, `project_name` | 项目 | 可见不等于必填；分别记录 `required_state`、`editable_state`、`enum_state` |
| `project_draft_id` | 项目 | Delivery-owned 稳定内部草稿 ID；创建平台项目前也存在，不依赖 `platform_id` |
| 项目—单元父关系 | 结构隐式 | payload 有且仅有一个 `platform_project`，`platform_promotions[]` 全部属于它；单元不携带 `parent_project_draft_id`（冗余且被机器契约拒绝），不用未提交的 `platform_id` |
| `delivery_identity` | 单元 | `account_info` 或已授权 `douyin_account` 二选一；授权动作不属于 Delivery |
| `base_material_refs`, `copy_items`, `native_anchor_ref`, `landing_page_ref`, `direct_link_ref`, `product_information`, `creative_components`, `promotion_settings`, `promotion_name` | 单元 | 引用/内嵌配置分开；敏感链接只保存受控引用或密文句柄 |
| `evidence_states`, `conditions[]`, `action_boundaries` | 编译元数据 | 不下发为平台业务实体；任何未知分支保持 `platform_pending` 或 `write_validation_pending` |

条件字段使用机器契约中的可组合表达而不是隐式 Go `if`：`all`、`any`、`not`、`equals`、`exists`、`reference_state_is`。表达式结果必须带 evidence、状态和 repair/blocked reason；这些内容位于 `compilation_metadata`，不进入 `payload` hash projection。

`targeting`、`schedule`、`budget_and_bidding`、`monitoring`、`product_information`、`creative_components`、`promotion_settings`、`copy_items` 等业务段在 v1 机器契约中**有意**保持 `{type: object}` 不透明：字段归属与条件语义由第 12 节切片另行收紧，不属本契约冻结范围。后续收紧只改变子 schema 的校验语义、不改变 hash projection（canonical hash 覆盖不透明内容本身），因此不会使已冻结 hash 失效。

### 9.3 稳定外部引用规则

所有商品、素材、落地页、锚点、人群包、投放身份和策略来源使用同一类不可变引用：

```text
namespace + object_kind + scope(account/project) + external_id
+ source_version + content_hash(if content exists)
+ display_name_snapshot + evidence_version + reference_state
```

- 商品：`product` 的平台/账户范围 ID；不复制商品详情。
- 素材：内部 `AssetID + Version + ContentHash`；上传后可附账户范围的 `platform_material_id`、审核/质量/使用状态。
- 落地页：`landing_page` 的平台/账户范围 ID，敏感直链只存受控引用或密文句柄。
- 锚点：`anchor` 的平台/账户范围 ID和证据版本；不把商品介绍字段复制为 Delivery 商品实体。
- 人群包：`audience_package` 的平台/账户范围 ID；项目内联定向仍是项目字段，复用包只保存引用。
- 投放身份：`account_info` 为账户上下文引用；`douyin_account` 必须引用已授权身份版本，不创建授权记录。
- 策略来源：内部 Strategy/Task 的 `task_id + version + content_hash + route`；Delivery 保存来源快照引用，不复制策略实体。

没有稳定 ID 时只能使用 `unresolved`/`blocked` 引用；显示名快照用于审计和 UI，不得作为稳定主键。引用状态、来源版本和 hash 参与新模型 canonical payload；审计人、创建时间、页面 evidence、条件和动作元数据不参与。新契约自包含 `StableReference` 定义，不再复用旧 `PlatformReference`。

## 10. ThreeTier 历史分析边界

| 旧对象/字段 | 新归属 | 处理 |
| --- | --- | --- |
| `ThreeTierGroup.ID`, `Name` | Delivery 内部编排/审计 | 保留为历史快照上下文；不写成项目 ID 或平台名称 |
| `group_name`、`group_objective`、`advertiser`、`business_asset_boundary` | `project_name`、项目目标/引用、`account_ref` 或内部边界 | `group_name` 只作内部标签；`group_objective` 拆成营销目的/目标引用；`advertiser` 改为稳定账户引用；`business_asset_boundary` 不下发平台 |
| `ThreeTierPlan.ID` | 内部关联标识 | 不映射成平台项目 ID；通过映射表/编译结果关联新 `PlatformProjectDraft` |
| `ThreeTierPlan.Name` | 项目名称候选或内部标签 | 只能选择一次归属；不得同时写项目和单元名称 |
| `placement`、`optimization`、`audience`、`budget`、`bid`、`schedule`、`conversion`、`tracking` | 项目字段 | 分别进入版位/目标/定向/预算/竞价/排期/优化目标/监测；条件不满足时保留 `platform_pending` |
| `ThreeTierCreative.ID`, `Name` | 单元内部引用/展示标签 | 不宣称存在独立 PlatformCreative；不可变素材版本改用稳定素材引用 |
| `creative_name`、`asset_version`、`title`、`format`、`landing_page`、`call_to_action` | 单元字段或稳定外部引用 | `creative_name` 只作单元标签；`asset_version` 转内部素材引用；标题/格式/CTA/落地页按路径进入单元 |
| `review_status`、`disclosure` | 只读结果/人工确认元数据 | 不作为创建草稿的业务实体字段；由平台回读或人工操作包呈现 |
| `ThreeTierField.Source`, `SourceRefs`, `Dependency*`, `Risk*`, `EvidenceRefs`, `PlatformStatus`, `Confirmed` | 新模型编译元数据 | 不复制为平台实体字段；`PlatformStatus` 未确认时保持 `platform_pending` |
| `schema`, `source`, `scenario`, `fixture_scenario`, `generated_at`, `evidence` | 历史快照元数据 | `delivery-three-tier/v1` 不可变；新模型使用新 schema/version |

禁止把 `group → plan → creative` 机械重命名为 `project → promotion → creative`，或把外部商品/素材/策略复制进 Delivery 表。此表只解释历史字段归属，不要求实现 ThreeTier → v2 的兼容映射；后续切换从明确版本化的 `DeliveryIntent` 直接生成目标平台 profile。

## 11. 历史版本策略

1. 已存在的 `delivery-three-tier/v1` 快照及其 canonical hash 永远保持不可变；历史版本继续可读，不能通过回填或重新计算变成新模型。
2. `delivery-platform-configuration/v1` 及其 fixture/hash 同样保持不可变；当前目标使用 `delivery-intent/v1` 和 `delivery-platform-configuration/v2`，不修改旧 JSON、旧 hash、旧 Approval/action hash 或旧 API 响应语义。
3. 不实现 `derived_from_legacy` 兼容投影，也不让 v2 校验器猜测 ThreeTier 字段含义；历史 API 按历史 schema 读取。
4. 需要转入新流程时，调用方必须显式创建新的 `DeliveryIntent` 版本，再生成绑定其 id/version/hash 的平台配置；不得把旧快照原地升级。
5. 后续 ChangeSet/Approval 接入只能绑定新版本的 schema、内容 hash 和意图快照 hash；旧 Approval 不可复用到新模型。

## 12. 下一阶段可执行 PR 切片

| 切片 | 代码/文档边界 | 验收输出 | 明确不含 |
| --- | --- | --- | --- |
| 配置领域契约 | **已完成**：`DeliveryIntent`、判别式 `PlatformConfiguration`、稳定引用、JSON Schema、fixtures 与生产 JCS hash 向量 | 能验证平台无关 intent；巨量一个 Project + 零到多个 Promotions；磁力只返回 `CAPABILITY_PENDING` | 持久化迁移、API、前端、运行时切换 |
| 运行时直接切换（后续） | API/持久化消费者显式接入新 schema；历史 schema 保持只读 | 新写入不经过 ThreeTier projector；版本/hash/审批绑定可追溯 | 改写历史行、推测式字段迁移、平台写入 |
| 行为流程编译器 | 基于 SelectorContract/ActionBoundary 编译只读定位、条件、确认和恢复分支 | 对电商手动、销售线索、Android 阻塞和质量异常 fixture 可重放；输出停止条件和 evidence | Computer Use 执行、真实提交 |
| Mock/replay 消费回归 | 复用 PR #38 的 reader，覆盖 usable 与五类非 usable 质量 | 告警/建议只在 usable 输入生成，来源/窗口/hash/evidence 可追溯 | Connector 目录改动 |
| Connector adapter（后续） | Connector Owner 发布正式契约后，仅增加 Delivery 适配器和 Consumer Contract 测试 | mock/replay/connector 可按请求来源切换，质量故障不静默回退 | 修改 Connector 实现、影子结论前置 |
| 真实影子分析门 | 仅在 adapter、对象映射和新鲜度通过后接入影子告警/建议 | 结论绑定真实 evidence；非 healthy 只提示 | 任何平台写入 |

以上切片可以独立 PR 交付，切片之间不共享未冻结的内部对象含义。当前应先执行前四个切片；Connector adapter 和真实影子分析保持 No-Go，直到外部契约满足第 7 节条件。
