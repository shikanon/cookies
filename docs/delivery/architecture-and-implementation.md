# Delivery 架构与实现

## 当前权威运行时

Delivery 的新写入只有一条配置路径：不可变 `DeliveryIntent` 绑定带平台判别器的 `PlatformConfiguration`。

- `DeliveryIntent` 使用 `delivery-intent/v1`，表达平台无关的营销目标、预算和排期边界、稳定引用与优化偏好。
- `PlatformConfiguration` 使用 `delivery-platform-configuration/v2`，由 `platform` 与 `profile_version` 判别平台 payload。
- 巨量引擎 profile 是一个 Project 与零个或多个 Promotions。
- 磁力引擎当前确定性返回 `CAPABILITY_PENDING`，不猜测未经校准的字段。
- PlanVersion 的 canonical hash 等于其平台配置 canonical hash；Approval 继续绑定计划、配置、意图和 ChangeSet 的不可变身份。

当前实现已包含 `DeliveryDecisionEngine` 与 `CompiledDeliveryWorkflow` 的 Phase C 权威主线。Decision 选择会生成新的不可变平台配置版本和工作流，并严格停在 `ready_for_final_approval`；它不创建正式 Approval，也不代表广告平台已执行。

Phase D 已接入 Browser RPA 控制面和 OceanEngine Runner v3。控制面可以将
Cookies 配置编译为项目或单元计划。它支持顺序创建项目和多个单元，并回写平台
ID。它也支持已绑定单元的预算修改。素材替换、暂停和启用仍被前端与编译器阻止。

真实 Edge 会话检查已接入控制面。该检查验证 DevTools WebSocket、巨量登录状态和
页面广告账户。完整 Cookies UI 到平台 ID 回写的实测仍未完成。当前 Cookies 没有
可用于新建单元的巨量素材。详细状态见
[`oceanengine-runner-v3-control-plane-closeout.md`](./oceanengine-runner-v3-control-plane-closeout.md)。

## 写入边界

创建和更新计划必须同时提交完整的 `DeliveryIntent` 与 `PlatformConfiguration`。以下操作在 Service 与 MySQL 写入边界都要求 v2 运行时：

1. 创建或更新 Plan；
2. 创建 ChangeSet；
3. 运行计划或 ChangeSet 检查；
4. 批准、打回或执行 ChangeSet；
5. 创建 OutcomeSimulation；
6. 生成不可变 DeliveryDecision；
7. 选择候选并编译 write-disabled workflow。

任何历史配置进入这些路径都返回稳定错误 `LEGACY_CONFIGURATION_UNSUPPORTED`。仓储层不会写入新的历史 PlanVersion、历史 ChangeSet 或历史 Recommendation。旧 Recommendation 的生成、采纳与拒绝已下线，接口返回 HTTP 410 `DELIVERY_DEMO_RETIRED`；业务以 Decision 为唯一优化主路径。

旧 `configuration:compile`、`configuration:override` 与操作包 POST 只保留为 deprecated 兼容墓碑。它们不解析业务请求，不依赖 Service，也不产生副作用。

## 历史读取边界

已落库的 `delivery-three-tier/v1` PlanVersion、ChangeSet、Recommendation、Approval 与旧操作包仍可读取，但必须同时满足：

- 返回 `runtime_status=legacy_unsupported` 与 `read_only=true`；
- 原始 JSON 不迁移、不改名、不重新序列化；
- 原 canonical hash 与 action hash 使用冻结投影验证，字节语义保持不变；
- 不把旧对象投影、物化或升级为 v2 对象；
- 旧操作包只作为历史审计载荷读取，不是未来工作流输入。

历史 DTO、判别式解码器、clone 与 hash verifier 是唯一允许继续理解旧结构的代码。它们不得被业务写路径调用。

## 检查与审批

权威检查直接消费类型化配置：

- 验证 DeliveryIntent 与 PlatformConfiguration 的结构、判别器、引用和 canonical hash；
- OceanEngine 必须恰有一个 Project，Promotions 可以为空；
- 执行所需稳定引用必须已解析；
- MagneticEngine 返回 `CAPABILITY_PENDING`；
- 未验证的平台写入证据继续保持 pending，不被描述为成功。

ChangeSet 冻结完整 `PlatformConfiguration` 目标快照与 hash。Decision 冻结 Plan、Intent、当前配置、事实快照、SimulationRun 与指标内容 hash，并确定性生成 conservative、balanced、exploratory 三类候选。候选选择不是正式审批，而是生成独立 `DecisionSelection`、新配置版本和工作流，同时冻结 Phase D 所需的 Plan/Intent/Decision/Configuration/Workflow 五元 hash 绑定。

CompiledWorkflow 显式绑定平台、账户引用、配置身份与 hash，以及 capability/selector/action/compiler 四类版本契约，并按页面组织 `observe`、`prepare_local_form` 与 `remote_write` 步骤。Phase C 的 `remote_write_enabled` 永远为 false；最终提交步骤必须带 `PHASE_C_REMOTE_WRITE_PROHIBITED` 阻断原因。MySQL 表同时用 CHECK 约束禁止远程写入状态和非 `ready_for_final_approval` 状态入库。

## Mock/Replay 观测闭环

`DeliveryObservatoryRun` 只接受 `mock` 或 `replay` fixture，并绑定 Selection、Decision、Configuration、Workflow 的精确身份、canonical hash 与 schema 版本。Runner 是不持有网络或平台 adapter 的纯函数；同一冻结输入通过 canonical input hash 派生同一 run identity，数据库唯一约束保证重复运行返回原记录。

- `observe_existing` 把 fixture 中的只读观测值与编译配置逐字段比较；不一致是 `drift_detected`，不是 runner 失败。
- `prepare_new_local_form` 只准备本地未提交值，结果为 `local_form_prepared`，不会提交表单。
- `insufficient_data`、`stale_data`、`blocked_by_asset`、`platform_pending` 在数据门禁处安全停止并保留原因和证据。
- Mock/Replay 的受控步骤故障单独记录为 `runner_failure`。
- 每个 run 都包含最终 `PHASE_C_REMOTE_WRITE_PROHIBITED` 边界观测；可执行动作集合不包含 `remote_write`，数据库再次要求 `remote_write_enabled=false`。

运营反馈以独立不可变记录追加：`accepted` 与 `rejected` 记录理由和相关 diff；`modified` 额外冻结最终 `PlatformConfiguration` 及其 canonical hash。反馈不会更新或覆盖原始 Decision、Selection、Run、步骤或证据。该 Mock/Replay 段不执行真实平台写入。真实写入使用独立的 Browser RPA 权威链。

## OutcomeSimulation 与监控

OutcomeSimulation 仅保留历史读取；演示执行、指标和模拟告警不会再通过业务页面生成。

上线前 Mechanistic Simulation 使用独立端口、存储和 API。它直接绑定不可变 PlanVersion、DeliveryIntent、OceanEngineConfiguration 和校准 Manifest。它不要求 Execution、Approval 或 Browser RPA Run。

Mechanistic Simulation v1 显式选择本地 Connector 账号。服务读取该账号已保存的七日启动批次先验。模型输出普通情景和跑量号情景。模型不提前识别跑量单元。模型不生成预算或出价变更。显式 CVR 和追踪先验只用于转化诊断。

## 历史 Tour

准备与复位已下线，旧写入接口返回 HTTP 410。历史记录保持可读，前端忽略 Tour 参数。旧审批页仅查看历史快照；真实投放进入受控执行中心。未配置执行适配器时返回 `EXECUTION_UNAVAILABLE`，不得回退到 Mock。

## 前端信息架构

- 主路由：`/projects/{project_id}/delivery/configuration`；
- 兼容深链：`/delivery/three-tier` 仅重定向到主路由；
- 活动视图只有“配置映射”和“检查与提交”；
- 历史计划只显示“历史配置，仅供查看”，不展示旧树、覆写、fixture、旧检查文案或操作包入口；
- 技术 schema/hash 信息不作为历史状态的主要文案。

## 数据与迁移

迁移保持严格增量。`delivery_intents`、`delivery_platform_configurations`、`delivery_decisions`、`delivery_decision_selections`、`delivery_compiled_workflows`、`delivery_observatory_runs` 与 `delivery_observatory_feedback` 保存独立不可变 envelope；现有表只增加判别器与绑定列。禁止修改或删除旧迁移，禁止 UPDATE 历史 payload，禁止重算历史 hash。

完整机器契约见：

- [`schemas/delivery-intent-v1.json`](./schemas/delivery-intent-v1.json)
- [`schemas/delivery-platform-configuration-v2.json`](./schemas/delivery-platform-configuration-v2.json)
- [`schemas/delivery-decision-v1.json`](./schemas/delivery-decision-v1.json)
- [`schemas/compiled-delivery-workflow-v1.json`](./schemas/compiled-delivery-workflow-v1.json)
- [`schemas/delivery-observatory-run-v1.json`](./schemas/delivery-observatory-run-v1.json)
- [`schemas/delivery-observatory-feedback-v1.json`](./schemas/delivery-observatory-feedback-v1.json)
- [`platform-configuration-contracts.md`](./platform-configuration-contracts.md)
- [`read-only-calibration-closeout.md`](./read-only-calibration-closeout.md)
- [`oceanengine-schema-calibration.md`](./oceanengine-schema-calibration.md)
