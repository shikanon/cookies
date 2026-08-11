# 制作中心前后端技术调研：一手来源 Findings

> 日期：2026-08-11
> 范围：仅调研本仓库中的 Kanon 架构、PRD、OpenAPI、领域代码、迁移与测试约束。
> 目的：为制作中心前后端技术设计提供可追溯事实，不新增架构未授权的产品能力。

## 1. 调研方法与结论级别

本文将证据分成三类：

1. **目标职责**：架构与 PRD 已明确要求制作中心承担的能力。
2. **现有事实**：当前代码、OpenAPI 和数据表已经具备的对象与接口。
3. **实现缺口**：目标职责与现有事实之间可直接证明的差距。

“实现缺口”不等于授权新增产品功能；它只说明要实现既有架构规定的制作中心，还缺少哪些查询、投影、字段或测试。

## 2. 制作中心的权威定位

### 2.1 它是 Creative 系统内的生产作业控制面

- Creative 系统拥有 `CreativeTask`、`EditTask`、`TimelineVersion`、`RenderJob`、`CreativeVersion`、`ProductionAsset`、`CreativePackage` 等对象；共享底座只提供 Provider、存储、任务和审计等能力。来源：`docs/02-creative-studio-prd.md:47-55`。
- 制作中心的职责是管理图片生成、视频生成、渲染队列、源素材和失败任务；它管理生成/剪辑作业和制作素材，但不承担效果分析。来源：`docs/02-creative-studio-prd.md:72`。
- 当前信息架构进一步规定六个视图：图片生成、视频生成、音频生成、渲染队列、源素材、失败任务；详情显示任务、输入、参数、输出、成本和运行日志。来源：`docs/19-module-navigation-architecture.md:179`。
- 模块分析规定 P0 展示形态为“队列表 + 状态筛选 + 任务预览抽屉”，失败项突出可重试原因，详情显示输入、输出、模型、成本与日志。来源：`docs/20-module-submodule-analysis.md:80`。
- 项目导航整改文件建议将制作中心改称“生成与渲染队列”，并明确只管理模型任务、输入、输出、成本、失败原因、重试和日志。来源：`docs/22-project-centered-navigation-remediation-plan.md:175`。

**技术结论：**制作中心应是 Project 范围内的生产作业查询/操作界面，而不是新的编辑器、新的模型管理后台或新的资产库。

### 2.2 必须保持 Project 隔离

- CreativeTask、EditTask、TimelineVersion、CreativeVersion、CreativePackage 等必须携带非空 `project_id`，页面标题持续显示当前 Project。来源：`docs/02-creative-studio-prd.md:59`。
- 原始媒体可以是组织共享资产，但 CreativeTask 中的素材使用关系、生成任务、评审和交付始终属于具体 Project。来源：`docs/02-creative-studio-prd.md:62`。
- Provider Job 数据表以 `(organization_id, project_id, created_at)` 建有范围索引。来源：`migrations/provider/20260722133000_provider_jobs.up.sql:1-33`，尤其 `:31`。
- 所有现有 Provider 与 Editing Render HTTP 路由均显式携带 `project_id` 并经过项目访问中间件。来源：`internal/platform/httpserver/server.go:422-423`、`:471-475`。

**技术结论：**制作中心的列表、详情、重试、取消及源素材读取都必须以 `organization_id + project_id` 约束；不能先按全局 ID 查询再由前端过滤。

## 3. 对象所有权与不可越界关系

### 3.1 CreativeTask 与 ProductionJob

- `CreativeTask` 拥有生产状态、草稿与生产血缘。来源：`internal/systems/creative/CONTEXT.md:8-11`。
- `ProductionJob` 只是 CreativeTask 到 Provider Job 的引用，记录生产血缘，不成为任务业务状态。来源：`internal/systems/creative/CONTEXT.md:24-25`。
- 当前 `ProductionJob` 只有 `task_id`、`kind`、`provider_job_id`、`created_at`。来源：`internal/systems/creative/model.go:1128-1133`。
- 当前持久化接口只能随 TaskDetail 读取生产血缘，基础仓储没有 Project 范围的 `ListProductionJobs`。来源：`internal/systems/creative/repository.go:35-61`，尤其 `:42-50`。

**约束：**制作中心可以投影 CreativeTask 的显示名称和业务状态，但不得用 Provider Job 状态覆盖或驱动 CreativeTask 的权威业务状态。

### 3.2 Provider Job

- Provider Gateway 归 Platform 团队所有，知道适配器、逻辑模型别名、重试策略、配额、用量和成本；它不得直接创建或更新 Assets 记录。来源：`internal/platform/provider/README.md:1-12`。
- Provider Job 公共状态包括 `submitted`、`running`、`outputs_ready`、`ingesting`、`succeeded`、`partially_succeeded`、`failed`、`cancelled`、`expired`。来源：`internal/platform/contract/provider.go:9-20`。
- Provider Job 公共对象当前包含执行状态、Provider 状态、进度、项目资产引用、错误、尝试次数、版本和时间戳。来源：`internal/platform/contract/provider.go:25-38`；OpenAPI 对应 `api/openapi/platform-v1.yaml:2599-2675`。
- Provider 数据表还持久化 `model_alias`、`source_system`、`source_task_id`、`provider_code`、`model_version`、`external_task_id`、`input_payload` 与标准化错误，但这些字段没有出现在当前公共 `ProviderJob` OpenAPI 响应中。来源：`migrations/provider/20260722133000_provider_jobs.up.sql:7-29` 对照 `api/openapi/platform-v1.yaml:2599-2675`。
- PRD 要求长任务逐项保存并展示 Skill、模型、提示词/参数、成本、素材来源和错误。来源：`docs/02-creative-studio-prd.md:410`。

**实现缺口：**Provider 内部已有部分生产详情，但当前公共 Job DTO 无模型、输入参数、来源任务、外部任务、用量、成本和运行日志，无法直接满足制作中心详情要求。

### 3.3 Assets 与生成结果接入

- Assets 模块拥有媒体资产、版本、来源血缘、权利、上传、生成结果接入和稳定 `ProjectAssetRef`；它不调用模型厂商 SDK。来源：`internal/platform/assets/README.md:1-8`。
- Provider 成功产物必须转存为 Asset，Assets 拥有二进制对象和技术元数据，但不拥有创意是否批准、素材是否有效等业务结论。来源：`docs/11-media-asset-platform.md:12-16`。
- 厂商临时 URL 不得进入 CreativePackage 或长期引用；生成结果由 Assets 下载、校验、扫描、哈希并转存，部分成功结果分别建 Asset，失败项可以单独重试。来源：`docs/11-media-asset-platform.md:62-68`。
- 业务系统不得把 Bucket/Key 作为公开契约，只保存 Asset ID 与版本。来源：`docs/11-media-asset-platform.md:90-100`。
- `POST /platform/v1/projects/{project_id}/assets/generated-intakes` 是 Provider 输出成为持久项目资产的唯一接口。来源：`api/openapi/platform-v1.yaml:819-824`。
- Assets 保存 `render_job_id`、`provider_job_id`、`provider_output_id`、`project_context_version`。来源：`internal/platform/assets/model.go:63-67`。
- Editing Render 完成后通过 Assets writer 接入结果，再把稳定 `ProjectAssetRef` 写回 RenderJob。来源：`internal/systems/creative/editing_render.go:254-266`。

**约束：**制作中心“输出”和“源素材”必须使用稳定 `AssetVersionRef`/`ProjectAssetRef`；不得让页面长期依赖厂商 URL、对象存储地址或制作中心自建文件表。

### 3.4 RenderJob 不是 Provider Job

- 编辑 RenderJob 绑定 `EditTaskID`、不可变 TimelineVersion、渲染类型、renderer fingerprint、状态、进度、输出资产、错误和 `retry_of`。来源：`internal/systems/creative/editing_render.go:21-45`。
- 编辑 RenderJob 状态为 `queued`、`running`、`succeeded`、`failed`、`cancelled`。来源：`internal/systems/creative/editing_render.go:47-55`。
- Preview 与 Export 是两个现有渲染类型。来源：`internal/systems/creative/editing_render.go:40-45`。
- 创建作业时会冻结当前 TimelineVersion；调度入队失败会把作业标记为 `SCHEDULER_ENQUEUE_FAILED`，而不是无限停在 queued。来源：`internal/systems/creative/editing_render.go:123-142`。
- 执行前会按用途重新校验素材权利，撤销时以 `ASSET_USE_REVOKED` 失败；执行中写入单调递增进度。来源：`internal/systems/creative/editing_render.go:194-250`。
- Retry 只能针对 failed 作业，创建新作业并用 `retry_of` 指向旧作业，不覆盖旧记录。来源：`internal/systems/creative/editing_render.go:292-320`。
- 另有早期 Pre-roll `RenderJob`，字段和状态不同，只包含 pre-roll/main video、输出、错误和 queued/running/succeeded/failed。来源：`internal/systems/creative/render.go:21-39`。
- Creative 内还存在 `AudioMixRenderJob`、`AINativeGenerationAttempt`、`BrandFilmGenerationAttempt`、`ImageGenerationAttempt` 等垂类持久对象。来源：`internal/systems/creative/brand_film_audio.go:260-277`、`internal/systems/creative/ai_native_production.go:34`、`internal/systems/creative/brand_film.go:413`、`internal/systems/creative/image_text_v2.go:135`。

**实现缺口：**当前生成/渲染对象分散在多个垂类聚合中，状态与字段不完全相同；制作中心不能把其中一个表直接当成全部生产队列。需要只读投影/适配层统一展示字段，同时保留每个权威对象和原始状态。

### 3.5 素材检查

- 素材检查现有接口针对确定的 `asset_id + version` 执行质量检查和人工确认。来源：`api/openapi/platform-v1.yaml:461-501`。
- 服务端要求资产版本存在；已通过的同版本质检可幂等返回旧结果。来源：`internal/platform/project/workflow_service.go:145-183`。
- 人工确认必须建立在同一资产版本已通过质检之上，状态只能是 `confirmed` 或 `changes_requested`。来源：`internal/platform/project/workflow_service.go:186-227`。
- 当前质检实现直接创建 `passed` 结果，固定模型为 `cookies.quality.basic`，尚不是一个真实异步模型质检闭环。来源：`internal/platform/project/workflow_service.go:167-176`。

**边界：**制作中心只证明生产作业及 Assets 接入是否完成；素材检查才拥有 AssetVersion 的质检和人工确认。制作中心不得写入 `QualityCheckRun` 或 `MaterialConfirmation`。

### 3.6 CreativeVersion、检查、批准与交付

- `CreativeVersion` 是从草稿冻结的不可变 Creative 快照；检查、评审、批准、交付及后续系统引用稳定版本，而不是 Draft 或 Provider Job。来源：`internal/systems/creative/CONTEXT.md:15-20`。
- Provider 成功只会产生可评估 Candidate，不代表批准。来源：`internal/systems/creative/CONTEXT.md:39-42`。
- `CreativePackage` 只能从 approved CreativeVersion 创建，包含冻结内容和 `AssetVersionRef`；Delivery 与 Insights 不接收可变 CreativeTask。来源：`internal/systems/creative/CONTEXT.md:21-23`。
- 实际服务在 `version.Status != CreativeVersionApproved` 时拒绝 Deliver，并从批准版本创建 CreativePackage。来源：`internal/systems/creative/service.go:1537-1559`。
- Delivery 的边界是把不可变 CreativePackage 转成可审计的广告平台变更；Delivery 不拥有 Creative 内容、Provider jobs 或投后分析。来源：`internal/systems/delivery/CONTEXT.md:3-15`。

**边界：**制作成功、Asset ready、素材质检通过、CreativeVersion approved、CreativePackage delivered 是不同事实，制作中心不得把前一个状态自动等价为后一个状态。

### 3.7 Insights

- 素材洞察拥有分析索引、特征、指标快照、分析任务、洞察和经验；只保存分析所需的版本引用与预览。来源：`docs/03-asset-management-prd.md:47`。
- Insights 消费 `creative.approved.v1`、`delivery.metrics.updated.v1`、`delivery.executed.v1`，不拥有 CreativeVersion 原文事实或 DeliveryPlan。来源：`docs/03-asset-management-prd.md:53-55`。
- Insights 代码明确不拥有媒体 Assets、Creative versions、Delivery plans、平台 executions 或原始 Provider jobs。来源：`internal/systems/insights/CONTEXT.md:5-10`。

**边界：**制作中心只提供可追溯生产血缘；效果结论、素材特征和投放分析留在 Insights。Provider Job 成功不能直接触发“有效素材”结论。

## 4. 当前可复用接口事实

### 4.1 Provider

- 创建模型 Job：`POST /platform/v1/projects/{project_id}/model/jobs`。来源：`api/openapi/platform-v1.yaml:775-800`。
- 按 ID 查询 Job：`GET /platform/v1/projects/{project_id}/model/jobs/{job_id}`。来源：`api/openapi/platform-v1.yaml:801-818`。
- 当前 HTTP Server 只注册上述 create/get 两条 Provider Job 路由。来源：`internal/platform/httpserver/server.go:422-423`。
- Provider 架构文档还规定 cancel，但当前 platform OpenAPI 与 server 路由没有对应实现。目标来源：`docs/07-unified-model-provider.md:154-156`；现状来源：`api/openapi/platform-v1.yaml:775-818`、`internal/platform/httpserver/server.go:422-423`。

### 4.2 Creative 垂类生成与重试

- Pre-roll 视频 Job 创建会通过 Creative API 生成 Provider Job 并注册到 CreativeTask。来源：`api/openapi/creative-v1.yaml:1390-1402`。
- 图片槽位 Retry 是现有垂类命令，要求 Idempotency-Key，只重试一个 slot。来源：`api/openapi/creative-v1.yaml:1236-1252`。
- AI Native 生产支持开始、失败 Unit 重试和 best-effort 取消，且明确重试只追加 Attempt、不重跑成功 Unit。来源：`api/openapi/creative-v1.yaml:329-351`。

**约束：**制作中心的“重试”应调用各权威垂类已经定义的命令，而不是直接修改 Provider/Creative 表或用一个无语义的通用重试替代业务前置条件。

### 4.3 Render

- Pre-roll 创建/查询已有 OpenAPI：`POST ...creative-tasks/{task_id}:render-preroll`、`GET ...creative-render-jobs/{render_job_id}`。来源：`api/openapi/creative-v1.yaml:1403-1423`。
- Editing Render 实际 Server 已有 create/get/cancel/retry 路由。来源：`internal/platform/httpserver/server.go:471-475`。
- 但当前 Creative OpenAPI 搜索不到 `edit-renders` 或 Editing Render 路径；Server 实现与 OpenAPI 不一致。来源：`internal/platform/httpserver/server.go:471-475` 对照 `api/openapi/creative-v1.yaml`。

### 4.4 资产接入、素材检查和交付

- Provider 输出进入资产库的唯一接口：`POST .../assets/generated-intakes`。来源：`api/openapi/platform-v1.yaml:819-824`。
- 素材检查与人工确认通过平台 AssetVersion 路径提供。来源：`api/openapi/platform-v1.yaml:461-501`。
- CreativeVersion check/approve/deliver 为独立命令；deliver 从 approved 版本创建稳定 CreativePackage，供 Delivery 与 Insights 使用。来源：`api/openapi/creative-v1.yaml:1452-1467`。

## 5. 当前前端实现状态

- 导航已配置“制作中心”，布局仍标为 `operations`，六个 views 已与信息架构一致。来源：`src/data/navigation.ts:32`。
- 当前只有素材检查命中专用 `MaterialCheckWorkspace`；production 没有专用页面分发。来源：`src/components/Pages.tsx:1479-1504`。
- production 因此回退到通用 `OperationsSurface`。来源：`src/components/Pages.tsx:1505-1507`。
- 通用 `OperationsSurface` 只读取当前 Project 的泛化 operation records，展示“运行稳定”和最近活动；它不接收 activeView，也不查询 ProductionJob、Provider Job 或 RenderJob。来源：`src/components/Pages.tsx:1213-1217`。
- 前端已有多个按 Job ID 查询 Provider Job 的垂类函数，但没有制作中心 Project 队列 API。来源：`src/data/api.ts:4318-4321`、`:4492-4495`、`:4997-5000`、`:5105-5108`。
- 前端 `ApiProviderJobWire` 仅映射 ID、项目、kind、两套状态、资产引用、简单错误、版本和时间；映射时还把 `partially_succeeded` 压成普通 `succeeded`，且模型被固定显示为 `cookies.video.standard`。来源：`src/data/api.ts:5060-5090`。

**实现缺口：**

1. 六个 tab 目前没有独立数据视图。
2. 没有 Project 范围统一作业列表/分页/筛选数据源。
3. 没有生产详情 DTO 覆盖输入、参数、模型、成本、日志与完整错误。
4. `partially_succeeded` 在现有前端映射中丢失，无法正确支持“保留成功项、仅重试失败项”。
5. 生产类型被某些垂类映射固定为视频，不能服务图片、音频和 RenderJob 的统一展示。

## 6. 当前后端实现缺口

### 6.1 缺少 Project 队列查询端口

- Provider OpenAPI 只有 create/get，没有 list。来源：`api/openapi/platform-v1.yaml:775-818`。
- Creative 基础仓储有 ListTasks，但 ProductionJob 和 RenderJob 只有注册/创建/按 ID 获取，没有 Project 列表。来源：`internal/systems/creative/repository.go:35-61`。
- EditingRenderRepository 同样只有 create/get/reuse/state update，没有 Project 列表。来源：`internal/systems/creative/editing_render.go:57-66`。
- 底层 Provider 表已有 Project/时间索引，Editing Render 表已有按 EditTask/时间索引，但不存在面向制作中心的只读聚合契约。来源：`migrations/provider/20260722133000_provider_jobs.up.sql:31-32`、`migrations/creative/20260806133000_creative_edit_render_jobs_m2.up.sql:21-26`。

**技术结论：**为了实现架构已规定的队列页面，需要补充只读查询 seam；它应聚合/适配现有权威对象，不新建第二套可写作业状态。

### 6.2 详情字段与可观测性不完整

- PRD 要求模型、提示词/参数、成本、素材来源、错误。来源：`docs/02-creative-studio-prd.md:410`。
- Provider 数据表持久化模型与 input payload，但公共 ProviderJob DTO 未暴露。来源：`migrations/provider/20260722133000_provider_jobs.up.sql:10-24` 对照 `api/openapi/platform-v1.yaml:2599-2675`。
- Provider README 声明该模块可能知道 usage 与 cost，但当前 ProviderJob contract、OpenAPI 和主表没有 usage/cost 字段。来源：`internal/platform/provider/README.md:5-7`、`internal/platform/contract/provider.go:25-38`、`migrations/provider/20260722133000_provider_jobs.up.sql:1-33`。
- EditingRenderJob 没有 cost 或 log reference。来源：`internal/systems/creative/editing_render.go:21-38`。

**技术结论：**“成本”和“运行日志”是已授权的制作中心能力，但当前没有完整权威数据契约。设计文档必须将其列为数据契约缺口，不能在前端伪造或用泛化运营记录代替。

### 6.3 作业家族分散且状态不同

- Provider、Pre-roll Render、Editing Render、AudioMix Render 与垂类 GenerationAttempt 分别持有状态。来源见第 3.4 节。
- ProductionJob 仅是血缘引用，不能作为权威状态。来源：`internal/systems/creative/CONTEXT.md:24-25`。

**技术结论：**统一队列只能统一“显示模型”，不能统一改写领域状态机。每条投影必须保留 `job_family`、`source_object_type`、`source_object_id`、原始状态和原始详情入口，以便操作路由回权威领域命令。这里是实现映射要求，不是新增用户功能。

### 6.4 事件目录与运行时存在差距

- PRD 规定 Creative 发布 `creative.approved.v1`、`creative.delivered.v1`、`creative.deactivated.v1`。来源：`docs/02-creative-studio-prd.md:53-55`。
- Creative MySQL repository 实际写入 `creative.approved.v1` 和 `creative.delivered.v1` outbox。来源：`internal/systems/creative/mysql_repository.go:820-821`、`:866-867`。
- 当前 `api/events/README.md` 只登记 `model.job.completed.v1` 与 `asset.ready.v1`。来源：`api/events/README.md:1-9`。
- 仓库只找到 Assets 实际创建 `asset.ready.v1`；没有找到 Provider 运行时创建 `model.job.completed.v1` 的实现。来源：`internal/platform/assets/upload_service.go:786`，以及仓库对 `model.job.completed.v1` 的全局引用仅出现在事件 schema/文档。

**技术结论：**制作中心首版不能假设所有 Provider 完成事件已可靠发布；若使用事件维护投影，必须先补齐并验证 producer/outbox，或以权威库查询/对账作为事实来源。不得由制作中心自行发布 Creative 批准或交付事件。

## 7. 不影响其他板块的硬边界

以下均由现有所有权直接推出：

1. 不修改 CreativeTask 状态机来适配列表；只读取其业务状态和血缘。证据：`internal/systems/creative/CONTEXT.md:8-25`。
2. 不复制 Provider Job 为新的可写“制作任务”真相；统一模型只能是 read model。证据：`internal/platform/provider/README.md:3-12`。
3. 不接管 Asset 二进制、版本、权利、扫描或处理状态。证据：`internal/platform/assets/README.md:3-8`、`docs/11-media-asset-platform.md:12-16`。
4. 不从厂商临时 URL 直接预览长期结果；只使用 Asset 预览/稳定引用。证据：`docs/11-media-asset-platform.md:64-68`、`:100`。
5. 不写素材检查的 QualityCheckRun/MaterialConfirmation。证据：`internal/platform/project/workflow_service.go:145-227`。
6. 不因为生产成功自动 check、approve、deliver。证据：`internal/systems/creative/CONTEXT.md:15-23`、`internal/systems/creative/service.go:1537-1559`。
7. 不在制作中心创建效果结论或分析索引。证据：`docs/02-creative-studio-prd.md:72`、`:89`，`docs/03-asset-management-prd.md:47-55`。
8. 不暴露 Provider 凭据、供应商配置或全局模型管理。证据：`docs/02-creative-studio-prd.md:88`。
9. 不把视频时间线、字幕、音轨编辑搬到制作中心。制作中心只观察 RenderJob；编辑器规范将 TimelineVersion 和 RenderJob 分开。证据：`docs/21-video-material-editor-spec.md:108-128`。

## 8. 文档范围差异，不能擅自补产品范围

### 8.1 3D

- 模块分析把 3D 生成列入制作中心能力。来源：`docs/20-module-submodule-analysis.md:80`。
- 当前导航架构、创意 PRD 和实际前端导航没有 3D tab，只列六个现有视图。来源：`docs/19-module-navigation-architecture.md:179`、`docs/02-creative-studio-prd.md:72`、`src/data/navigation.ts:32`。

**处理结论：**当前技术方案不应新增 3D 可见入口；只可保证作业 kind 的适配方式不硬编码为图片/视频。是否增加 3D 产品入口应由 Kanon/产品另行确认。

### 8.2 音频

- 当前正式导航有“音频生成”。来源：`docs/19-module-navigation-architecture.md:179`、`src/data/navigation.ts:32`。
- 创意代码已有 `AudioGenerationAttempt` 和 `AudioMixRenderJob`，但基础 ProductionJob/Provider 公共创建 OpenAPI 当前主要描述图片与视频。来源：`internal/systems/creative/brand_film_audio.go:260-277`、`api/openapi/platform-v1.yaml:775-794`。

**处理结论：**音频 tab 是既定范围，但数据源必须适配现有 Creative 音频对象；不能假设所有音频都已经统一成公共 ProviderJob。

## 9. 测试与交付约束

### 9.1 仓库硬门禁

- 所有必需 GitHub Actions 必须通过；commit/push 前运行 `git diff --check`。来源：`AGENTS.md:5-11`。
- 前端变更 push 前必须运行 `npm run build`，并运行变更区域相关测试。来源：`AGENTS.md:8-10`。
- 只能 stage 当前任务文件，不得削弱或删除质量检查；应采用保持现有行为的最小根因修复。来源：`AGENTS.md:14-18`。

### 9.2 制作中心后端至少需要保护的既有契约

这些不是新增产品能力，而是由现有边界推导出的测试面：

- Project/Organization 隔离：跨 Project Job、AssetVersion、RenderJob 不可见且不可操作。
- Read model 不回写权威状态：读取制作中心列表不能修改 CreativeTask、ProviderJob、Asset 或 RenderJob。
- 原始状态保真：`partially_succeeded`、`outputs_ready`、`ingesting` 不得被错误折叠。
- 分项结果保留：部分成功只对失败 Unit/Slot 调用已有 retry 命令；旧 Attempt 和成功 AssetVersion 保持不变。依据：`docs/02-creative-studio-prd.md:410`、`:433-434`、`:452`。
- Editing Render retry 保留 `retry_of`，调度失败形成稳定 failed 记录。依据：`internal/systems/creative/editing_render.go:137-140`、`:292-320`。
- 输出只引用 Assets 生成的稳定 ProjectAssetRef。依据：`api/openapi/platform-v1.yaml:819-824`。
- 生产成功不绕过质检/批准/交付门禁。依据：`internal/systems/creative/CONTEXT.md:15-23`、`internal/systems/creative/service.go:1537-1559`。
- 列表分页/筛选必须使用 Project scoped query，不能通过前端全量拉取后过滤。

### 9.3 可复用测试命令事实

- 前端构建：`npm run build`，其内容为 TypeScript noEmit + Vite build。来源：`package.json:6-10`，尤其 `:8`。
- 前端单测：`npm test`；现有 provider client 测试在 `test/platform-client.test.ts`。来源：`package.json:10`、`test/platform-client.test.ts:82-158`。
- Go 模块使用 Go 1.26/toolchain 1.26.5。来源：`go.mod:1-5`。
- 相关 Go 包已有测试基础：`internal/platform/provider/*_test.go`、`internal/platform/assets/*_test.go`、`internal/systems/creative/*_test.go`、`internal/platform/httpserver/server_test.go`。

## 10. 供技术设计直接采用的事实摘要

1. 制作中心是 Project-scoped 的生产队列与详情控制面，不是新的领域真相。
2. 六个已授权视图固定为图片、视频、音频、渲染、源素材、失败任务。
3. 权威状态分别留在 Provider Job、各 Creative GenerationAttempt 与各 RenderJob；ProductionJob 只是血缘引用。
4. 输出必须先经 Assets intake 成为稳定 AssetVersion，才能被素材检查、CreativeVersion、Delivery 和 Insights 引用。
5. 素材检查、版本批准、交付、效果分析都不是制作中心动作。
6. 当前前端只有通用 operations 壳；当前后端没有统一 Project 作业列表。
7. Provider Job 公共契约不足以展示架构要求的模型、输入参数、成本和日志；这是明确的数据契约缺口。
8. 统一队列应采用只读投影/adapter，操作必须回到各权威垂类命令，避免影响其他板块状态机。
9. `partially_succeeded`、`retry_of`、稳定错误码、AssetVersion 血缘和 Project 隔离必须端到端保真。
10. 事件目录与部分运行时 producer 尚未闭合，不能在首版设计里把未验证事件当成唯一事实来源。
