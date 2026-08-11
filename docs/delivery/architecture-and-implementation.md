# 智能投放系统：架构路线图与当前实现

## 当前运行时权威链

权威边界为 `DeliveryIntent v1 -> tagged PlatformConfiguration v2 -> DeliveryPlanVersion v2 -> ChangeSet -> Approval`。五类记录均受 Organization/Project 隔离并绑定不可变版本。Repository 在同一事务中提交 Intent、PlatformConfiguration 与 PlanVersion。

读取按持久化 schema 分派：v2 使用 PlatformConfiguration 规范投影；历史 ThreeTier 使用冻结旧投影并始终只读。预检直接消费类型化配置：OceanEngine 必须有一个 Project、可以有空 Promotions，并要求执行所需稳定引用均已解析；Magnetic Engine 确定性返回 `CAPABILITY_PENDING`。

新运行时不会把 v2 机械投影回 ThreeTier，也不引入 Connector、平台 API、DecisionEngine、工作流编译器或 Computer Use 执行。

| 属性 | 内容 |
| --- | --- |
| 状态 | DeliveryIntent 与判别式平台配置已切换到本地运行时；真实平台写入仍保持关闭 |
| 记录日期 | 2026-07-29 |
| 实现快照日期 | 2026-08-10 |
| 关联文档 | [广告智能投放 PRD](../04-intelligent-delivery-prd.md)、[当前实现盘点与未实现项计划](../plans/2026-07-28-implementation-gap-plan.md) |

本文记录智能投放系统的领域架构路线图，以及当前交付的 DeliveryPlan 生命周期、服务端权威检查、内容哈希绑定审批、持久化平台操作演练、上线后指标/告警和证据驱动建议与人工操作包。除“当前实现快照”和以下冻结契约明确列出的内容外，真实平台能力仍是后续设计草案，不属于当前实现的行为契约。当前目标领域模型见[`DeliveryIntent` 与平台配置契约](./platform-configuration-contracts.md)；只读校准、Connector 阻塞边界和历史结论见[收口文档](./read-only-calibration-closeout.md)。

### 只读业务校准覆盖说明

历史 mock 闭环已结束，其两段审批和 `group → plan → creative` 三段结构是实现快照，不再被当作真实投手流程或巨量平台对象声明。只读业务校准按[巨量业务 Schema 校准](./oceanengine-schema-calibration.md)执行以下修正：

- 投手与产品语言统一为“投放账号 → 项目 → 单元”；`PlatformAccount/PlatformProject/PlatformPromotion` 只用于代码和适配层；
- 真实电商工作流覆盖商品/落地页/锚点/人群包准备、素材上传和质量过滤、多项目/多单元创建、开启及投后优化；
- 历史 ThreeTier 只保留当前 mock 运行时语义；新领域契约以平台无关 `DeliveryIntent` 绑定判别式 `PlatformConfiguration`，不再要求兼容投影；
- 产品目标覆盖完整竞价投放场景，当前电商路径只是第一条已校准路径，缺少分支持续进入业务 Schema 覆盖矩阵；
- 预检、模拟、影子分析和采纳建议不产生平台写入；目标流程默认只在最终真实创建/开启前进行一次确认，高风险动作按风险追加确认；
- 素材来自前序板块不可变版本或用户上传，Delivery 只保存版本引用、平台素材映射和审核/质量/使用状态，不复制素材内容或修改素材/洞察模块。

本节是只读业务校准后的产品目标，不反向篡改历史 mock 闭环已落库的审批、哈希和审计语义。后续 API/持久化切换应直接消费新契约并显式处理版本边界，不把历史 mock 流程继续暴露为投手标准操作，也不新增 ThreeTier 兼容 projector。

## 当前实现快照

当前交付严格限制在场景环境内的计划草稿、权威检查、版本绑定审批、持久化平台操作演练、同一 Execution 的指标/告警，以及三个内部配置区段的受控编译、证据驱动建议和人工操作包：

- API：Project-in-path 的 `/api/delivery/v1/projects/{project_id}/plans`、计划详情与乐观并发更新、不可变版本列表/详情、草稿检查、变更申请创建/详情/列表/检查/批准/打回、带 Idempotency-Key 的平台操作演练及 Execution 列表/详情。
- 数据：`delivery_plans`、`delivery_plan_versions`、`delivery_change_sets`、不可变 `delivery_approvals` 与持久化 execution/step/evidence；更新以 `expected_version` 做乐观并发，旧版本返回 `409 VERSION_CONFLICT` 或 `412 VERSION_CONFLICT`。
- 分层：`domain.go`、`approval.go`、`preflight.go`、`service.go`、`mysql_repository.go`、`backfill.go` 与 `httpapi/server.go`。
- 隔离：所有读写同时受 Organization 和 Project 约束；跨 Project 读取即使主体拥有两个 Project 的权限也会被拒绝。
- 权限：读取、写入、审批、执行分别要求 `delivery.read`、`delivery.write`、`delivery.approve`、`delivery.execute`；身份只来自 `ActorContext`。
- mock 透明度：API 和持久化记录继续显式携带 `source=mock` 与场景代码；前端每个 Delivery 页面统一使用一个低干扰 Mock 横幅，只在数据来源或证据诊断处重复显示来源。
- 页面顺序：策略/素材来源 → 目标与账户 → 预算与排期 → 追踪 → 草稿检查；服务端检查结果是唯一事实源。计划保存不可变的策略任务版本、素材版本、内容 hash 与可导航路由，不能只保留自由文本版本名。

版本绑定审批具有以下已实现语义：

- `DeliveryPlanVersion.canonical_hash` 使用共享 `contract.CanonicalJSONHash`，即 RFC 8785 JCS + SHA-256；内容范围只含投放业务字段和 source/platform 边界，不含创建人、创建时间等审计元数据。
- `DeliveryApproval.action_hash` 绑定 Organization、Project、Plan/PlanVersion、ChangeSet/ChangeSetVersion、Plan canonical hash、`action=execute`、`scope=execute_mock`、预算上限与币种。
- 审批固定在批准后 24 小时过期；身份来自 `ActorContext`，界面不展示无业务意义的 mock 姓名或负责人 ID。审批请求体只接受 `expected_version`；打回请求另要求持久化修改理由。
- `delivery_change_sets.approved_by/approved_at` 仅作为权威 Approval 的兼容投影；迁移会把历史投影一次性转为不可变 Approval，之后不允许覆盖。
- ChangeSet 列表与详情从绑定的不可变 PlanVersion 派生 `plan_name`，审批队列和详情显示真实计划标题而不是通用的 `Plan V*`。
- ChangeSet 详情动态返回 Approval 的 `valid`、`invalid_reason`、版本、hash、审批人、批准/过期时间、范围和预算快照。
- 任何模拟执行前重新校验 Approval 存在、未过期、PlanVersion 仍为 current、ChangeSetVersion/action hash 匹配、scope 与预算未超限。
- 成功执行和回滚只推进 ChangeSet 生命周期版本，不改变获批内容版本；回读时 `executed` 映射到当前版本减一、`rolled_back` 映射到当前版本减二，因此合法状态推进不会被误报为 `APPROVAL_CONTENT_MISMATCH`。
- 投放计划与审批中心当前只展示已接入真实数据源的主工作区；尚未实现服务端筛选的 L2 标签保持隐藏，待过滤契约完成后再开放。
- 内部配置编排只负责草稿检查和提交变更申请，不暴露批准或打回命令。提交后以精确 ChangeSet ID 导航到审批中心；审批中心独占批准/打回，并要求打回理由说明具体修改对象、字段和期望结果。
- 稳定错误包括 `APPROVAL_REQUIRED`、`APPROVAL_EXPIRED`、`APPROVAL_CONTENT_MISMATCH`、`APPROVAL_SCOPE_EXCEEDED` 与既有 `STALE_PLAN_VERSION`；响应继续明确 `source=mock` 和场景。

当前预检分级如下：

| 级别 | 规则 | 行为 |
| --- | --- | --- |
| error | 广告主缺失、预算为 0、排期无效、素材引用缺失、追踪配置缺失 | 阻断，并返回可定位的 repair target |
| warning | 素材版本尚未人工确认 | 不阻断，但要求投手明确处理 |

### 三段配置、建议与人工操作包冻结契约

配置编排在既有 PlanVersion 上增加一个**可选**的 `delivery-three-tier/v1` 快照。它只描述 cookies 内部受控 mock 配置，当前以 group、plan、creative 三个区段组织审计依赖；这些名称和嵌套关系不是对巨量当前平台对象层级的声明。旧计划和旧 ChangeSet 保留省略语义，因此既有计划的 canonical hash 不因该字段缺省而变化。快照出现时，canonical hash、ChangeSet 目标快照/hash 和 Approval action binding 一起覆盖整个快照，而不是只覆盖预算、素材或单一字段。

- 快照固定携带 `source=mock`、fixture `scenario`、生成时间和 evidence references，且结构为 `1..N groups -> plans -> creatives`。每一层的每个配置字段携带值、provenance、dependency、risk、`platform_pending` 和 confirmation 元数据；它们是审计对象，不是前端推断结果。
- `POST .../configuration:compile` 只接受 `{expected_version, fixture}`，由服务端从固定 fixture 编译新版本。支持的 fixture 是 `golden_path`、`missing_required_field`、`orphan_dependency`、`missing_confirmation` 和 `platform_fields_pending`。黄金 fixture 恰含 1 个组、2 个计划和 3 个创意；客户端不得提交自由结构来冒充编译结果。
- `POST .../configuration:override` 只改写明确定位的一个 group/plan/creative field，并且要求 `expected_version`、typed value 和人工确认；成功总是创建新的不可变 PlanVersion。它不更新旧快照，也不允许跨 Project 定位对象。
- 只有携带三层配置快照的版本才追加预检：结构完整性、required field、依赖是否孤立和 required confirmation。平台待补字段是显式风险/待办，不虚构真实平台枚举、预算上限、状态机或 API 限制。未携带该快照的版本继续运行既有规则，以保持原有结果与哈希。

`DeliveryRecommendation` 只能在同一 Plan 已有 succeeded Execution、至少两个持久指标窗口以及引用该 Execution 的 Alert 后生成。它冻结 Execution、Metric 和 Alert 的精确 evidence refs；缺一项即返回 `INVALID_STATE`。该规则是可重复的场景逻辑，不是经真实业务数据校准的效果预测模型。记录必须保存 fingerprint、base/target snapshot 与 hash、evidence、action、impact、risks、observation window、cooldown 和 provenance，并只能 `proposed -> accepted|rejected`。接受建议时必须有 `Idempotency-Key`：同 key/请求返回原 decision，冲突 key 返回稳定冲突；接受响应只含 decision 和**一个新的 draft ChangeSet**。它不得直接修改 Plan、批准变更申请或写入平台。

由三层配置或建议创建的 ChangeSet 保存不可变 target snapshot/hash，并可选关联 `recommendation_id`；Approval 可选保存同一 target snapshot hash 作为 action binding。它们仍须分别预检和审批，接受建议本身不构成预检通过或审批。

`ManualActionPackage` 仅可由已批准 ChangeSet 使用 `{expected_version}` 编译，并按幂等语义回放同一个不可变操作包。它提供按层排序的待填字段、字段来源、人工确认点、预期结果、禁止动作、evidence 和 provenance；读取或编译操作包不调用真实平台、不暗示已写入，也不取代执行审批。

迁移 `20260804140000_delivery_a06_three_tier.up.sql` 是只前进、追加式迁移；文件名作为已执行迁移的历史标识保持不变。阶段编号不得作为新领域文件、符号、CSS 类或用户界面名称；本文件名仅因完整路径已经成为 migration ID 而保留。它为既有 `delivery_change_sets` 和 `delivery_approvals` 添加 nullable 的 target snapshot/hash 绑定，并创建 `delivery_recommendations` 与 `delivery_manual_action_packages`。历史行保持新增字段为 nullable，不回填或重算旧 canonical hash；所有新表和查询继续以 Organization + Project 为复合边界。

### 平台校准边界

- 只读业务校准已冻结 [`oceanengine-bidding-schema/v0.1`](./schemas/oceanengine-bidding-schema-v0.1.json) 契约，覆盖 PlatformProject/PlatformPromotion、选择器、提交事件、电商与销售线索连续路径，以及应用下载 Android 的事件资产门禁；它不宣称是巨量永久全量表单，也不授权真实写入。
- 营销目的/模式会改变后续表单。营销场景、营销产品或商品 ID、投放载体、优化目标等字段的出现条件、枚举、必填性和可编辑性都必须来自可追溯页面或公开契约证据；未观察项保持 `platform_pending`，应用缺少事件资产时保持 `blocked_by_event_asset`，不由 fixture 猜测。
- `1..N groups -> plans -> creatives` 只服务内部快照、依赖校验和审批绑定。平台对象使用 `PlatformProject`、`PlatformPromotion` 等显式名称，通过 `PlatformEntityMapping` 版本化关联，不能直接复用内部嵌套推断远端层级。
- 当前闭环只负责把现有场景能力串成可重复验收的因果链，不负责补齐全量平台字段。平台 schema 校准归入只读业务校准，行为流程编译与字段映射归入后续的行为编译与影子分析。

### 上线后优化闭环 Tour 冻结契约

Tour 是对既有 Delivery 权威链的编排和可恢复投影，不创建第二套 Plan、Approval、Execution、Alert、Recommendation、ChangeSet 或 ManualActionPackage 模型。领域接口固定为：

- `POST /api/delivery/v1/projects/{project_id}/tour-runs/{run_id}:prepare`：准备或幂等回放同一运行；
- `GET /api/delivery/v1/projects/{project_id}/tour-runs/{run_id}`：从持久化领域事实重新计算从第 0 步开始的九步黄金路径、七个场景和下一链接；
- `POST /api/delivery/v1/projects/{project_id}/tour-runs/{run_id}:reset`：只删除该运行的精确因果闭包并返回逐表删除数。

`run_id` 是 3–64 位稳定小写标识，匹配 `^[a-z0-9][a-z0-9_-]{2,63}$`。唯一安全边界是可信 ActorContext 提供的 `organization_id / project_id / run_id / owner_id`；客户端不能提交 owner。Tour 专属 Plan 以 nullable `tour_run_id`、`tour_owner_id` 和 `tour_case` 标记，普通 Plan 保持这三个字段为空。复位事务先锁定同 owner 的 run，再只选择同 run/owner 的 Plan ID，并沿外键方向删除其因果记录；禁止按 Project、名称前缀或时间窗口推测归属。

黄金计划以“第 0 步：核对本次运行的投放计划”开始。Prepare 会在精确 `organization / Project / run_id / owner_id` 边界内创建并绑定该计划，因此准备完成后第 0 步默认完成；完成证据必须同时包含 run、Plan、策略与素材引用，不能由同 Project 的普通计划替代。随后调用已有配置、草稿检查、首次审批、平台操作演练、指标告警、建议、采纳、优化审批和人工操作包接口。其余六个计划分别固定为预检失败、审批过期、Plan stale、部分执行、结果未知、审核拒绝告警；场景之间不得复用 Plan 来制造多个结论。审核拒绝场景只预置 Execution 和指标前提，Prepare 时不创建 Alert，用户触发该场景后才生成事件与告警。Prepare 回放 prepared run 必须返回原七个 Plan ID；中断于 preparing 的 run 只在同一精确边界内清理后重建。

页面进度、当前步骤、异常状态和 evidence 全部由 Go API 返回。跨页 URL 固定携带 `tour_run_id`、`tour_case` 和 `plan_id`；通过侧边栏访问导致查询参数缺失时，前端只用版本化 localStorage 指针恢复当前 Project 最近的 run ID，再向 Go API 刷新权威进度。未来步骤在前序条件完成前保持锁定。所有 run、case 和 reset 响应显式携带 `source=mock`、scenario、时间和 evidence。

当前账号、数据和页面均为 mock，且内部配置与巨量当前动态页面仍有明显差距，因此当前闭环不收集没有业务意义的投手/审阅者姓名或负责人 ID，由开发者依据现有先验知识完成流程审阅。真实业务投手评审只在真实只读数据接入、对象映射和巨量页面/schema 校准完成后启动。接口 `owner_id` 只承担复位隔离，不表达真实业务岗位或责任分配。具体准备、走查和复位命令见 [Delivery 上线后优化闭环走查手册](delivery-mock-tour-runbook.md)。

当前交付不包含 OAuth、真实平台请求、自动补偿或自动化写入。后文涉及这些能力时，除上述受控 mock 接缝外，均表示后续路线图。

---

## 持久化模拟 Execution 场景

该实现将已有的即时本地模拟接缝替换为受控、可刷新读取的 `Execution` / `Step` 记录。它仍然**只**调用 deterministic `mock_ocean_engine` adapter：`source=mock`、`mode=local_simulation` 和 fixture `scenario` 在执行、步骤和证据中均会显式返回；它不代表 Computer Use、真实广告账户或任何真实平台写入。

### 对象边界与审批不变量

- `ChangeSet` 是被批准的不可变动作内容与审批门禁；`Execution` 是对该 ChangeSet 的一次持久化执行尝试。Execution 的终态不会改写 Plan/ChangeSet 的内容哈希，也不会取代版本绑定的 Approval 审计证据。
- 创建执行前仍重新验证版本绑定的 approval：不可变 approval、`execute_mock` scope、预算快照、24 小时有效期、Plan/ChangeSet version 与 canonical/action hash、Organization/Project 隔离均保持不变。
- 成功 Step 不会被再次运行。状态推进以 `expected_version` 和持久化并发保护完成，避免多个 worker 或客户端重试创建重复效果。

### HTTP、幂等与读取

`POST /api/delivery/v1/projects/{project_id}/change-sets/{change_set_id}:execute` 必须带 `Idempotency-Key`，请求体是 `{ "expected_version": number, "scenario": "success|failed|partial|result_unknown" }`。

- 首次请求返回 `201`；同一 key 且同一 canonical request hash 返回原来的 `Execution` 和 `200`；同一 key 但 hash 不同返回稳定的 `409 IDEMPOTENCY_CONFLICT`，绝不新建执行。
- canonical request hash 是 RFC 8785 JCS + SHA-256，输入为 `organization_id`、`project_id`、`change_set_id`、`expected_version`、`scenario` 和固定操作名 `execute_mock`；Idempotency-Key 不参与 hash。hash、key、scenario 和 provenance 都随 Execution 持久化。
- `GET /api/delivery/v1/projects/{project_id}/executions` 返回 `{items, source:"mock", scenario:"execution_list"}`；`GET .../executions/{execution_id}` 返回单个 `{change_set, execution, evidence}`。两者均受 Organization 与 Project 边界约束，刷新后的页面必须从这些 Go API 恢复状态。

### 状态、步骤与恢复决策

| 层级 | 状态 |
| --- | --- |
| Execution | `queued` → `validating_approval` → `executing` → `verifying` → `succeeded` / `failed` / `partial` / `result_unknown` / `cancelled` |
| Step | `pending` → `running` → `succeeded` / `failed` / `result_unknown`；或 `pending` → `skipped`（不调用 adapter） |

`failed` 只表示已确认目标效果**未**产生；中断或无法验证的情况不能被写成 failed，而应为 `result_unknown`。所有 terminal fixture 都返回 `retry_allowed=false`：同一 key 的操作只能回放同一条 Execution，不能创建第二次尝试。`partial` 明确保留已完成和未完成的 Step，以及可选的补偿候选项；补偿是新的受控动作，绝不自动回滚。`result_unknown` 必须先查询/重新识别目标并形成恢复决策（`query_and_reconcile`），不能盲目重试。允许的恢复动作仅为 `none`、`create_new_change_set`、`review_and_compensate` 和 `query_and_reconcile`。

为兼容既有版本绑定审批保留的 `:rollback` 生命周期接口只接受已经完成且 `succeeded` 的 mock Execution。非终态、`failed`、`partial` 和 `result_unknown` 均返回 `INVALID_STATE`：它不能被用来宣称未知效果已经回滚，也不能绕过新的受控补偿或查询复核流程。

每个 Step 保存 sequence、action、attempt、effect、outcome summary、evidence reference、时间戳和 version；Execution 保存 recovery action/reason、compensation candidates 和全部 Step。Evidence 也携带 `source=mock`、fixture scenario 和非敏感 references。

---

## 1. 背景与动机

当前仓库已有 ChangeSet 预检/审批/模拟执行/模拟回滚和审计的演示能力，但存在以下阻碍 Demo 验证的结构问题：

- **没有权威 DeliveryPlan**：投放页面的字段没有对应可保存、可刷新、可版本化的业务对象。
- **mock 逻辑不可替换**：当前模拟逻辑混在 `internal/platform/project` 共享包中，之后接真实 API 时业务层需要重写。
- **ChangeSet 领域所有权错误**：`platform_change_sets` 挂在 platform Schema 下，不属于 Delivery 系统自己的领域模型。
- **预检、审批、执行均不可信**：预检规则只有演示实现（`confirmed_brief` 永远为 true），审批可直接传任意 actor/role，模拟执行没有过程证据和异常分支。
- **缺少投手日常闭环**：页面能点击操作，但不能演示完整的开工巡检、日内处置、效果复盘流程。

第一阶段（mock 优先）的目标：**在不准等待真实平台账号的前提下，把上述问题全部修复，建立一个可被投手按脚本验收的完整 Mock 投放闭环**。

---

## 2. 系统架构

### 2.1 包结构

新功能全部进入 `internal/systems/delivery/`，自建 Schema、迁移和路由，不扩展 `server/` TypeScript 兼容服务。

```text
internal/systems/delivery/
├── domain.go              # DeliveryPlan / DeliveryPlanVersion / ChangeSet / Execution / Alert / Recommendation 领域类型
├── status.go              # 状态机（plan / execution / alert）
├── repository.go          # Repository 接口
├── mysql/                 # MySQL Repository 实现 + 迁移
│   ├── migrations/
│   └── repository.go
├── memory/                # In-memory Repository（测试用）
│   └── repository.go
├── service.go             # 业务逻辑层（preflight / approve / execute）
├── adapter.go             # PlatformAdapter 端口定义
├── adapters/
│   └── mock_ocean.go      # MockOceanEngineAdapter
├── preflight.go           # 预检规则引擎
├── http_handler.go        # HTTP handler（/api/delivery/v1/*）
└── http_handler_test.go
```

已在 `internal/platform/project` 下的 ChangeSet 相关代码不改动。在 delivery 包中建立新的 `DeliveryPlan` → `ChangeSet` → `Execution` 链路，旧 ChangeSet 待前端迁移完成后单独 PR 清理（P0-D11）。

### 2.2 与现有平台的对接

```
React (Project Shell)
  └── delivery 页面
        └── @/platform/client  ← Go 平台客户端
              └── /api/delivery/v1/*  ← 新路由（本项目）
                    └── delivery.Service
                          ├── delivery.Repository（MySQL）
                          ├── PlatformAdapter（端口）
                          │     └── MockOceanEngineAdapter（mock 阶段）
                          │     └── OceanEngineAdapter（企业开放平台准入后的可选分支）
                          └── Project/Strategy/Creative 只读引用（通过主键 + 版本号，不持有外键）
```

- `DeliveryPlan.current_version` 持有 `source_strategy_version` 和 `creative_version_refs` 作为只读快照引用，不作外键约束。
- Project 是上下文根，但 Delivery 系统独立管理自己的 Schema、事务和审计。
- 前端页面通过 Go 平台客户端调用 `/api/delivery/v1/*`，不再走 TypeScript 兼容服务。

---

## 3. 领域模型

### 3.1 聚合与表设计

#### 核心表

| 表 | 关键字段 | 说明 |
| --- | --- | --- |
| `delivery_plans` | `organization_id`、`project_id`、`current_version`、`status`、`platform`、`account_binding_id` | 投放计划聚合根；Project-scoped |
| `delivery_plan_versions` | `plan_id`、`version_number`、`source_strategy_version`、`creative_version_refs`、`config_json`、`canonical_hash`、`created_by` | 不可变版本；config_json 含目标/预算/受众/排期/追踪/素材引用 |
| `delivery_change_sets` | `base_plan_version`、`target_plan_version`、`diff_json`、`risk`、`content_hash`、`status` | 两版本间差异；内容哈希用于审批绑定 |
| `delivery_approvals` | `change_set_id`、`approver_id`、`scope`、`limit`、`action_hash`、`expires_at` | 一次审批绑定一个 ChangeSet 的不可变内容 |
| `delivery_executions` | `change_set_id`、`adapter`、`source`、`scenario`、`status`、`idempotency_key`、`started_at`、`finished_at` | 执行记录；scenario = `success` / `partial` / `failed` / `result_unknown` |
| `delivery_execution_steps` | `execution_id`、`sequence`、`action`、`request_ref`、`result_ref`、`status`、`attempt`、`evidence_id` | 每一步独立记录；支持部分成功与恢复 |
| `delivery_platform_entities` | `internal_type` / `internal_id`、`platform`、`advertiser_id`、`external_type` / `external_id`、`fingerprint`、`source` | 内部对象到外部 ID 的版本化映射 |
| `delivery_evidence` | `before_snapshot`、`after_snapshot`、`request_id`、`platform_status`、`redacted_payload_ref`、`source` | 操作前后快照与平台状态证据 |
| `delivery_alerts` | `organization_id`、`project_id`、`plan_id`、`execution_id`、`rule_id` / `rule_version`、`fingerprint`、`window`、`severity`、`status`、`owner`、`evidence_refs`、`fixture_version`、`freshness`、`source` | Project-scoped；由固定 demo fixture 指标快照驱动。fingerprint 绑定规则、实体、窗口、fixture/dataset 版本与证据引用，保证同一快照身份不会重复创建告警。 |
| `delivery_recommendations` | `evidence_refs`、`action`、`risk`、`observation_window`、`decision`、`source` | 优化建议；采纳后生成 ChangeSet 重新走预检和审批 |

#### 命名规范

巨量广告升级版的 `Project` 与 cookies 自有 `Project` 同名，代码中强制区分：

| 术语 | 含义 |
| --- | --- |
| `Project` | cookies 全局业务上下文 |
| `DeliveryPlan` | cookies 投放意图 |
| `PlatformProject` | 巨量广告升级版 Project |
| `PlatformPromotion` | 巨量广告升级版 Promotion |
| `PlatformEntityMapping` | 内部↔外部 ID 版本化映射 |

### 3.2 状态机

```text
DeliveryPlan:
draft → ready_for_preflight → preflight_failed | ready_for_approval
→ approved → executing → active | partially_applied | failed | result_unknown
→ paused | ended

Execution:
queued → validating_approval → executing → verifying
→ succeeded | partial | failed | result_unknown | cancelled

Alert:
open → acknowledged | dismissed
```

三个状态机独立管理。平台审核状态、投放启停状态和 cookies Execution 状态分别保存各自的 `status` 列，不合并。

---

## 4. PlatformAdapter 端口设计

### 4.1 端口定义

```go
// internal/systems/delivery/execution.go

type PlatformAdapter interface {
    Source() Source
    ExecuteStep(context.Context, PlatformStepRequest) (PlatformStepResult, error)
}
```

该端口刻意保持逐 Step：Service 先以 CAS 将对应 Step 从 `pending` 持久化为 `running`，随后才调用 `ExecuteStep`，并将返回结果推进为 `succeeded`、`failed` 或 `result_unknown`。`pending → skipped` 不调用 adapter。这样进程中断最多留下可查询的 `running`/`executing` 状态，不会在没有证据时写成 failed，也不会重新运行已成功的 Step。真实平台阶段可在同一端口后实现 action dispatch、查询复核和平台实体映射；暂停、读取与完整 `delivery_platform_entities` 仍是后续能力，不伪装成当前已实现行为。

### 4.2 三个实现

| 实现 | 阶段 | 行为 |
| --- | --- | --- |
| `DeterministicMockAdapter`（持久化标签 `mock_ocean_engine`） | 当前 mock 闭环 | 返回固定 mock 账号与场景的逐 Step 结果；所有响应显式标记 `source=mock`；可切换 success/partial/failed/result_unknown 场景 |
| `ComputerUseAdapter` | 只读校准及受控写入 | 在已登录的受控会话中读取页面、核验对象并在写入范围获批后执行逐步 UI 操作；处理接管、页面证据和结果未知，不保存账号凭据 |
| `OceanEngineAdapter` | 可选 API 分支 | 企业完成开发者、应用、scope、Secret 与授权准入后调用 Marketing API；实现限流、幂等、查询复核、错误分类和脱敏 |

`delivery.Service` 只依赖上述逐 Step 端口。未来真实 Adapter 仍须遵守“先持久 running、再产生平台效果”的调用边界；Computer Use 只读校准不依赖 OAuth，受控写入和可选 API 分支则分别受各自的授权、限流、实体映射与安全条件约束。它们都不属于当前持久化模拟执行能力。

---

## 5. 关键设计决策

### 5.1 Mock 显式标记（P0-D02）

所有 mock Adapter 的响应必须内嵌 `"source": "mock"` 和场景名（如 `"scenario": "golden_path"`）。前端页面和 API 响应中均强制执行此标记，防止评审者将模拟数据误解为真实投放结果。

### 5.2 审批绑定内容哈希（P0-D06）

已实现语义：

- `DeliveryPlanVersion.canonical_hash` 复用 `internal/platform/contract.CanonicalJSONHash`，不建立第二套 JSON canonicalizer。
- Plan canonical payload 覆盖 `name`、`objective`、`advertiser`、`budget`、`schedule`、`tracking`、`creative_references`、`source_strategy_version` 与 source/platform 边界；排除 `created_at`、`created_by` 等审计元数据。
- `DeliveryApproval.action_hash` 的 canonical payload 绑定 `organization_id`、`project_id`、`plan_id`、`plan_version`、`change_set_id`、`change_set_version`、`plan_canonical_hash`、`action`、`scope`、`budget_limit` 与 `currency`。
- mock `action=execute`、`scope=execute_mock`，预算上限等于批准 PlanVersion 的预算，固定 24 小时有效。
- 审批仅在 action hash、Plan/ChangeSet 版本、内容 hash、有效期、scope 与预算全部匹配时有效；计划产生任何新版本后旧审批永久失效，即使内容后来改回相同值。
- ChangeSet 从 `approved` 推进到 `executed` 或 `rolled_back` 时只增加生命周期版本；审批仍绑定原始批准版本并保持完整性有效，但状态机会阻止重复执行。
- Actor 身份从 Go `ActorContext` 注入，审批请求体不得接受 `actor`、`role`、`approver` 或任意 `scope`。

### 5.3 幂等键

每条 `delivery_executions` 在创建时校验 Project-scoped `idempotency_key` 的唯一性约束。canonical request hash 固定覆盖 Project、ChangeSet、`expected_version`、fixture scenario 与 `execute_mock` 操作名；同 key + 同 hash 返回原有 Execution， 同 key + 不同 hash 返回 `409 IDEMPOTENCY_CONFLICT`，不重复创建任何 mock 目标效果。

### 5.4 "回滚"改为补偿

广告平台多数动作只能补偿不能回滚。Demo 中不提供"回滚"按钮，改为"生成/执行补偿动作"，补偿经过与正向操作相同的预检 → 审批 → 执行流程。

### 5.5 服务端预检为唯一事实（P0-D05）

预检结果以后端 `POST /api/delivery/v1/plans/{id}/preflight` 返回为准。前端可做客户端预检，但最终决策以服务端结果为准。当前规则分级以本文开头的“当前实现快照”为准；后续新增规则必须同步更新 OpenAPI、服务端测试和页面 repair target。

---

## 6. 实现模块

以下模块以纵向功能为单位，每个模块改迁移、Go 服务、OpenAPI、React 页面和测试。实现顺序按依赖关系排列，具体拆分到 PR 时可根据实际情况调整（例如变更较小的模块可合并提交）。

### 模块 1：DeliveryPlan 生命周期

**做什么**：投手能创建、保存、刷新并编辑投放计划草稿。

- `internal/systems/delivery` 最小领域包：`DeliveryPlan`、`DeliveryPlanVersion` 类型定义
- 迁移：`delivery_plans`、`delivery_plan_versions` 表
- Repository 接口 + MySQL 实现（更新走乐观并发，`version_number` 冲突返回 409）
- Project-scoped API：`POST /api/delivery/v1/plans`、`GET /api/delivery/v1/plans/{id}`、`PATCH /api/delivery/v1/plans/{id}`
- 投放计划页接入新 API，所有 mock 账户明确标识 `source=mock`
- 固定黄金场景 fixture（销售线索单目标/固定预算区间/固定 mock 广告主）
- 测试：service 测试 / HTTP handler 测试 / 跨 Project 隔离测试 / E2E

### 模块 2：服务端预检

**做什么**：投手能看到一份服务端生成、可修复的投前检查结果。

- 预检规则引擎（`preflight.go`）：Brief/Strategy/Creative 引用完整性、素材授权状态、预算范围、追踪配置、平台能力
- `POST /api/delivery/v1/plans/{id}/preflight` → 返回 error/warning 分级结果
- 前端 preflight 结果展示 + repair 提示（点击预算超限 → 跳转计划编辑页）
- 以服务端为唯一预检事实源；前端 helper 检查仅供参考

### 模块 3：版本绑定审批（已实现）

**做什么**：审批人只能批准当前看到的那一版计划。

- `DeliveryPlanVersion.canonical_hash` 计算与现有版本确定性 Go backfill
- `delivery_change_sets` + 不可变 `delivery_approvals` 表与迁移
- `POST /api/delivery/v1/projects/{project_id}/plans/{plan_id}:create-change-set`（冻结当前 PlanVersion；完整 base/target diff 编辑器仍未实现）
- `POST /api/delivery/v1/projects/{project_id}/change-sets/{id}:approve`（校验 ActorContext + content/action hash + 24 小时有效期）
- 计划 V2 更新后 V1 的旧审批自动失效
- 审批页 + 审计定位（审批人、审批时间、过期时间、Plan/ChangeSet 版本、hash、scope、预算）

### 模块 4：场景化模拟执行（已实现）

**做什么**：投手能观察模拟执行的每一步及异常恢复。

- `PlatformAdapter` 端口定义 + `MockOceanEngineAdapter` 实现
- `delivery_executions` + `delivery_execution_steps` + `delivery_evidence` 表；完整 `delivery_platform_entities` 映射仍属后续真实平台阶段
- `POST /api/delivery/v1/projects/{project_id}/change-sets/{id}:execute`（`Idempotency-Key` + canonical request hash 去重；201 创建/200 回放/409 冲突）
- `GET /api/delivery/v1/projects/{project_id}/executions` 与 `GET /api/delivery/v1/projects/{project_id}/executions/{id}` → 步骤、状态、恢复决策和 `source=mock`
- 四种 mock 场景：success / partial（部分成功）/ failed / result_unknown
- 审批中心内的执行明细（持久 Step、脱敏 evidence reference、恢复决策与 scenario tag）；完整 before/after 平台快照仍属后续真实平台阶段

### 模块 5：Mock 监控告警

**做什么**：在平台操作演练成功后生成可重复的上线后指标窗口，再把同一证据链上的异常转成待办。

- succeeded Execution 自动创建两个 Project-scoped、同 execution_id 的指标窗口；`delivery_alerts` 的 fingerprint 是规则版本、受监控实体、窗口、dataset 版本和精确证据引用的稳定组合身份，同一身份评估时复用已有告警而不是重复创建
- `POST /api/delivery/v1/projects/{project_id}/alerts:evaluate`：先通过 Delivery-owned `InsightsConsumer` 读取版本化对象/指标事实，再由统一告警规则计算；当前 `SimulationInsightsReader` 只负责把 OutcomeSimulation 窗口归一化为同一 `DeliveryMetricFact` 输入。仅 `quality=usable` 允许确定性告警；响应同时返回 `insights_source`、`insights_quality`、fixture 版本和 evidence，保留 `source=post_launch_simulator` / `is_simulated=true` 的历史兼容字段
- `GET /api/delivery/v1/projects/{project_id}/alerts`：支持 `status`、`type`、`severity`、`fixture`、`limit` 与 opaque `cursor`；响应包含 `next_cursor`
- `PATCH /api/delivery/v1/projects/{project_id}/alerts/{alert_id}`：请求为 `{action: acknowledge|dismiss, expected_version}`；仅允许 `open → acknowledged|dismissed`，过期版本返回 `409 VERSION_CONFLICT`
- 四类告警：`review_rejected`（审核拒绝）、`spend_spike`（消耗突增）、`zero_conversion`（零转化）、`cost_worsening`（成本恶化）
- 每条告警显示并持久化 Project/Plan/Execution/受监控实体、规则 fingerprint/version、主窗口及可选基线窗口、指标口径、证据引用、负责人、fixture/dataset 版本、结构化 freshness、创建及处置审计字段
- 不做定时任务或真实平台轮询；审核拒绝事件只在对应异常场景被显式触发后生成，不在计划创建时预置任何告警

历史 mock 闭环后续已按[投放效果情景模拟器计划](./post-launch-scenario-simulator-plan.md)增加独立的 `OutcomeSimulationRun`，让预算、目标、出价、定向、创意和稳定 seed 形成可解释的确定性情景指标，并将投后效果情景与平台操作成功/失败演练分开。它仍是显式 mock，不是平台算法复现或真实效果预测；只读业务校准只把它作为消费端口与流程测试输入。

#### API、状态和证据契约

`api/openapi/delivery-v1.yaml` 是监控接口的唯一事实源。它是受控、Project-scoped 的演示监控，不是广告平台集成：

- `POST /api/delivery/v1/projects/{project_id}/alerts:evaluate` 只评估一个确定性 fixture：`normal_day`、`anomaly_day`、`stale_data` 或 `insufficient_data`
- `GET /api/delivery/v1/projects/{project_id}/alerts` 可按状态、类型、严重度和 fixture 过滤，使用有界 `limit` 与 opaque `cursor` 分页
- `PATCH /api/delivery/v1/projects/{project_id}/alerts/{alert_id}` 只接受 `{action: acknowledge|dismiss, expected_version}`；仅允许 `open → acknowledged|dismissed`，旧版本稳定返回 `409 VERSION_CONFLICT`

支持的告警类型固定为审核拒绝、消耗突增、零转化和成本恶化。每条告警必须带有 organization、Project、Plan、Execution、被监控的 DeliveryPlan 实体、规则 fingerprint/version、指标及分析窗口（含可选基线）、负责人、证据、dataset 版本与 freshness。响应显式标记 `source=post_launch_simulator` 和 `is_simulated=true`；freshness 保存状态、数据覆盖时间、评估时间、观测 age、最大允许 age 与可选缺失指标。创建与处置均保留 actor 和时间的审计投影。

告警 fingerprint 由 organization/Project、规则及版本、被监控实体、指标窗口、dataset/fixture 版本与证据引用的 canonical 内容确定。相同身份的重复评估必须复用既有告警；窗口或 provenance 变化才产生新的告警。客户端不得生成快照身份或告警结论。

#### 运营界面语言

运营主视图优先展示用户可读的告警结论、业务背景、已本地化的单位和可读的证据来源；规则 ID、枚举、schema 单位、fixture/actor 技术标识和原始证据 URI 只能作为次级技术披露，不能成为主要用户文案。后续模块 7 的完整 mock Tour 应审计既有投放界面是否满足同一规则，特别是既有 `AG-PREFLIGHT-002` 对“原始枚举不得作为主要用户文案”的要求。

### 模块 6：三段投放配置 mock、建议与人工操作包

> 本节描述当前运行时的历史 mock 行为，不是新平台配置领域模型。新实现以[`DeliveryIntent` 与平台配置契约](./platform-configuration-contracts.md)为准；本次契约冻结不修改该运行时。

**做什么**：把泛化的 DeliveryPlan 展开为广告组、广告计划、广告创意三个内部配置区段，使投手在没有真实写权限时也能完成完整配置、预检、审批和人工执行准备；区段名称不构成平台对象层级承诺。

- `DeliveryPlanVersion` 保存三段配置快照、对象依赖、来源策略/创意版本、推荐值、人工修改值、平台待补字段和风险。
- 预检、内容哈希、ChangeSet 与 Approval 必须绑定完整配置快照，不能只绑定预算或素材等局部字段。
- `delivery_recommendations` 保存同一 Execution/Metric/Alert 的 evidence refs、动作、影响范围、风险、观察窗口和冷却期；采纳只创建新的 ChangeSet，重新走预检和审批。
- `ManualActionPackage` 按内部配置区段输出待填写字段、字段来源、人工确认点、预期平台结果和人工执行安全边界；它不操作真实平台，也不暗示已写入。
- 不做自动执行和 LLM 自动优化；字段硬约束、预算与审批仍由确定性服务端规则裁决。
- Alert 是建议生成的必要证据，但不能直接创建 Recommendation、ChangeSet 或任何平台写入；建议仍须显式生成、人工采纳并再次审批。

### 模块 7：上线后优化闭环 Tour 与验收

**做什么**：评审者按固定脚本验证计划来源、内部配置、草稿检查、首次审批、平台操作演练、指标告警、建议、新变更申请、优化审批到人工操作包的闭环，并确认关键确认点与异常分支可解释。

- 固定黄金路径和至少五个异常场景（预检失败 / 审批过期或计划 stale / 执行失败或部分成功 / 结果未知 / 审核拒绝等）
- 黄金数据一键复位，且只处理明确属于 Tour run 的记录
- E2E 覆盖行为证据；人工验收脚本记录字段来源、确认点、异常处理与操作包可理解性
- 所有 mock 数据、操作包与 API 响应带显式来源标签，不声称真实平台写入
- 补齐规则驱动、可重放、对业务输入敏感的效果情景模拟器；由同一 SimulationRun 依次产生指标、告警和建议，计划创建时不得预置这些结果
- 审计既有投放界面的内部 enum、规则 ID、schema 单位、fixture/actor 标识和原始证据 URI；它们只能是技术披露，不能作为主要用户文案
- 复用模块 6 已有人工操作包，不创建第二套包格式；对外使用“完整 mock 闭环”，不使用可能暗示全量平台字段仿真的“全量模拟”

---

## 7. 边界与后续阶段

### 7.1 第一阶段边界

**在当前阶段实现**：DeliveryPlan 生命周期 / 服务端预检 / 版本绑定审批 / 场景化平台操作演练 / 规则驱动的效果情景模拟 / Mock 监控告警 / 三段内部配置 mock / 建议→ChangeSet 闭环 / 人工操作包 / Demo 巡演

**在当前阶段不做**：
- 巨量引擎 OAuth、账户授权、Token 管理和真实 API 调用
- Computer Use 控制面（环境/租约/会话/接管/证据/Kill Switch）及任何真实平台写入
- 真实数据采集、对象映射、对账和影子分析；当前监控仍是显式 fixture
- 巨量当前竞价投放的全量模式、动态表单和平台对象层级仿真
- 定时任务与 LLM 自动优化

### 7.2 后续阶段

| 阶段 | 目标 | 启动前提 |
| --- | --- | --- |
| 只读校准与 Connector 依赖对齐 | 在专用竞价投放账户（受 Git 管理文档仅记录尾号 `6391`）下，以已登录 Computer Use 会话只读校准所有可访问项目、PlatformProject/PlatformPromotion、动态表单和页面语义；真实报表采集、标准化和发布由数据洞察 Connector Owner 负责 | **已完成**；[只读 Schema v0.1](./schemas/oceanengine-bidding-schema-v0.1.json)已冻结；Delivery consumer port、mock/replay 已随 PR #38 进入 `upstream/main`；Connector 正式输出仍是外部依赖 |
| Delivery 配置模型与行为流程编译 | 由平台无关 `DeliveryIntent` 编译判别式 `PlatformConfiguration`；巨量 profile 为一个 Project 与零个或多个 Promotions，行为编译另行接入 | **领域契约已完成**；无需 ThreeTier 兼容映射、Connector 或平台写入；运行时切换另行立项 |
| 影子分析 | 仅消费数据洞察发布的真实只读指标运行影子告警/建议 | Connector 指标口径/新鲜度、对象映射、正式消费契约、样例和 Consumer Contract 测试可用；当前 **No-Go** |
| 受控写入 | 在测试项目内通过 Computer Use 填写草稿、读取回填值、人工确认提交并核验结果 | 明确写入范围、小额硬上限、人工负责人、Kill Switch、审批重验和防重演练 |
| 生产化 | 限流、事件重放、可靠性指标、凭据轮换、第二平台适配器 | 真实闭环证明瓶颈后启动 |

### 7.3 只读业务流校准任务分解

只读业务校准的目标是让真实投手能审核一个有证据的只读业务流，而不是把历史 mock Tour 误作真实业务验收。它不实施巨量数据采集器，也不产生任何平台写入；完整消费需求和模块边界见[投放对数据洞察 Connector 的消费需求](./insights-connector-consumer-requirements.md)。

| 子阶段 | Delivery 交付物 | 依赖 | 阶段门 |
| --- | --- | --- | --- |
| 范围与证据协议 | 已冻结尾号 `6391` 的专用竞价投放账户、所有可访问项目、导航域名、表单观察方式、写操作停止条件和登录接管方式 | 当前浏览器保持已登录；登录/验证码由人工处理 | 已完成；当前配置足以启动页面与对象走查 |
| 页面与对象走查 | 一个目标路径的页面状态、对象清单、审核状态与允许导出位置的连续证据 | 已登录受控会话；验证码、登录失效与未知页面由管理员接管 | **已完成**；脱敏证据见[巨量只读校准摘要](./oceanengine-readonly-calibration.md)，所有未观察字段明确标记，不以经验补齐 |
| 业务 schema 校准 | 版本化字段/依赖/枚举/默认值/错误状态与内部配置区段映射说明 | 真实投手只读评审 | **已完成**；[业务 Schema 校准](./oceanengine-schema-calibration.md)与[只读 Schema v0.1](./schemas/oceanengine-bidding-schema-v0.1.json)已冻结；未知项保留 `platform_pending` 或 `blocked_by_event_asset`，真实写入保留 `write_validation_pending` |
| Connector 消费需求 | 对象 ID、指标、窗口、口径、延迟、质量和 evidence 的版本化需求与验收样例 | 数据洞察 Owner 确认或发布契约 | Delivery 不修改洞察模块、不直连其数据库、不自建采集器 |
| 消费端口与 mock/replay | Delivery 内部 `InsightsConsumer`；Mock、Simulation bridge 和 Replay 已实现，未来 Connector 为另一实现 | Connector 样例/接口；Connector 可异步开发 | 每个告警/建议输入显示来源、质量、窗口、口径与证据；不合格数据不产生确定性结论 |
| 只读评审与结论 | 投手差异清单、回归样例、行为流程编译与影子分析的进入或阻塞报告 | 投手评审与 Connector 可用性说明 | 首个电商手动路径已校准，剩余差异可追踪；真实影子分析等待 Connector，不阻塞模型编译 |

执行顺序为“范围与证据协议 → 页面与对象走查 → 业务 Schema 校准 → Connector 消费需求 → 消费端口与 mock/replay → 只读评审与结论”。Connector 消费需求和消费端口可以与数据洞察 Owner 的真实采集实现并行，但真实指标的影子告警/建议只在 Connector 发布正式输出、且质量状态满足要求后开始。

#### 范围与证据冻结配置

- 导航白名单：`https://business.oceanengine.com/*`、`https://ad.oceanengine.com/*`；`https://sso.oceanengine.com/*` 仅供会话过期后的人工接管。
- 数据范围：尾号 `6391` 的专用竞价投放账户下所有可访问项目及其可见数据；该账户不承载敏感业务内容。
- 允许动作：查看列表、详情、审核状态、报表；打开新建/编辑页面观察表单。新建页面可以切换只影响未提交表单的选项，编辑已有项目只读取现值。
- 禁止动作：最终提交/保存、启用、暂停、删除、修改预算、修改出价及其他平台状态变更。发现自动保存草稿或远端变更迹象时立即停止。
- 接管条件：会话过期、验证码、重新登录或任何可能产生写入的未知状态。认证信息不得进入证据、日志或文档。

---

## 8. 可选的巨量引擎 Marketing API 适配分支

### 8.1 准入后的能力覆盖

公开文档可用于建立契约、mock/replay 和能力开关；但真实调用以企业获得开发者主体、应用、scope、Secret 管理和广告主授权为前提。它不是 Computer Use 只读校准或测试项目写入的前置条件。

满足准入条件后，以下能力可在 `OceanEngineAdapter` 中按需接入：

| 能力 | 接口 | cookies 用途 |
| --- | --- | --- |
| OAuth | `/open_api/oauth2/access_token/`、`refresh_token/`、`advertiser/get/` | 连接广告主和刷新授权 |
| 账户/资金 | Account、Advertiser、Fund 相关接口 | 账户健康和预算前置检查 |
| Project | `/open_api/v3.0/project/create\|list\|update\|status\|budget/...` | 创建和管理升级版 Project |
| Promotion | `/open_api/v3.0/promotion/create\|list\|update\|status\|budget/...` | 创建和管理 Promotion |
| 素材与审核 | 素材管理、素材预审核、拒审原因接口 | 上线前检查和拒审处置 |
| 报表 | `/open_api/v3.0/report/custom/get/` 及异步任务接口 | 指标同步、监控、分析 |

### 8.2 双执行器字段映射策略

内部配置快照是领域权威输入，但不是平台 schema。行为流程编译器结合已校准的平台证据生成 Computer Use 的页面操作与读取核验步骤；API adapter 则生成结构化请求。两者都必须回写远端对象 ID、能力版本、证据与 `executor=computer_use|api`，并显式处理字段能力、授权、异步状态和错误语义的差异。不得使用全局“真/假”开关假设两者可以完全无差别互换。

---

## 9. 验收标准

当前 mock 闭环完成后应满足：

1. 投手可在 20~30 分钟内完成“内部配置 → 预检 → 审批 → 模拟执行 → 监控 → 建议 → 新 ChangeSet → 再预检/审批 → 人工操作包”的完整闭环。
2. 除黄金路径外至少覆盖五个异常场景，包括预检失败、审批过期或计划 stale、执行失败或部分成功、结果未知、审核拒绝。
3. 所有模拟数据和 API 响应中显式标记 `source=mock`，不出现"已在巨量引擎上线"的误导。
4. E2E 测试覆盖主路径和至少 5 个异常路径；复位只影响带当前 Tour run 标识的记录。
