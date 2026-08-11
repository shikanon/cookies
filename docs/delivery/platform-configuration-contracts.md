# DeliveryIntent 与平台配置契约

## 运行时切换（2026-08-10）

当前 Delivery 写链已在每个 `delivery-plan-version/v2` 中同时持久化完整的 `delivery-intent/v1` 与 `delivery-platform-configuration/v2`。服务端重算两者的 RFC 8785 哈希并覆盖审计元数据；`DeliveryPlanVersion.canonical_hash` 必须与 PlatformConfiguration 哈希完全相同，DeliveryIntent 哈希保持为独立绑定。

OceanEngine 配置恰好包含一个 Project，并允许零个或多个 Promotions。Magnetic Engine 保持稳定的 `CAPABILITY_PENDING` profile，在可执行预检中阻断，且不虚构平台字段。

`ChangeSet.target_snapshot` 冻结完整的判别式 PlatformConfiguration。Approval 除 Plan 与 ChangeSet 身份外，还绑定配置的 schema/id/version/platform/profile/hash，以及 Intent 的 schema/id/version/hash。

迁移保持增量：`delivery_intents` 与 `delivery_platform_configurations` 保存独立版本的不可变 envelope，同时为 PlanVersion、ChangeSet 和 Approval 增加判别器/绑定列。既有 ThreeTier JSON、canonical hash 与 action hash 均不改写。

历史 `delivery-three-tier/v1` 与 `delivery-platform-configuration/v1` 使用冻结的旧哈希投影读取，返回 `runtime_status=legacy_unsupported`、`read_only=true`，不能进入新建、更新、预检、ChangeSet、审批或执行写链。旧 compile/override 端点稳定返回 `LEGACY_CONFIGURATION_UNSUPPORTED`。

| 属性 | 内容 |
| --- | --- |
| 状态 | 领域契约已冻结并接入本地运行时；真实平台写入仍不在范围内 |
| 版本 | `delivery-intent/v1`、`delivery-platform-configuration/v2` |
| 平台 profile | `oceanengine-configuration/v1`、`magnetic-engine-configuration/v1` |
| 历史边界 | `delivery-three-tier/v1` 与 `delivery-platform-configuration/v1` 保持不可变、仅按原语义读取 |

本文是 Delivery 新配置领域契约的规范入口。它把平台无关的业务意图与平台字段分离，避免把当前 ThreeTier mock 或巨量字段误当成所有平台共用的领域模型。

## 1. 根模型与版本边界

```text
DeliveryIntent (platform-neutral, immutable version)
└─ PlatformConfiguration (tagged by platform + profile_version)
   ├─ ocean_engine
   │  └─ Project (exactly one)
   │     └─ Promotion[] (zero or more, parentage is structural)
   └─ magnetic_engine
      └─ CAPABILITY_PENDING (no guessed platform fields)
```

- [`delivery-intent-v1.json`](./schemas/delivery-intent-v1.json)只表达营销目标、预算/排期边界、优化偏好以及商品、落地页、素材、人群和策略的稳定引用，不含 `carrier`、`delivery_mode`、项目或单元字段。
- [`delivery-platform-configuration-v2.json`](./schemas/delivery-platform-configuration-v2.json)以 `platform`、`profile_version` 和 payload discriminator 形成判别联合。平台/profile 不一致必须失败，未知 schema/profile 不得静默降级。
- 巨量 profile 有且只有一个 Project，允许零个或多个 Promotions。Promotion 不携带冗余父 Project ID，也不引用尚未创建的平台 Project ID。
- 磁力引擎 profile 当前只有可校验、可哈希的 `CAPABILITY_PENDING` 结构；在取得已验证表单/写入证据前，不增加 Project、Promotion、selector 或猜测字段。
- 当前 API、数据库、前端和 ThreeTier 流程本 Goal 不切换。后续切换直接消费新契约；不新增 ThreeTier → 新模型兼容 projector。

## 2. 稳定引用

`StableReference` 用 `namespace + object_kind + scope + state` 描述外部对象，已解析时再绑定稳定 `id`，可选绑定 `version` 和 `content_hash`：

- `resolved` 必须有稳定 ID；
- `unresolved`、`blocked`、`redacted` 必须有原因；
- `display_name_snapshot` 与 `evidence_version` 只服务 UI/审计，不是实体身份，也不进入 canonical payload；
- Delivery 不复制商品、素材、策略、落地页、人群包或授权身份实体。

## 3. 来源语义

配置生成来源与事实来源是两条独立轴，不能共用一个 `source` 字段：

- `configuration_provenance.kind`：`manual`、`rule`、`decision_engine`、`import`；
- `fact_provenance.source`：`mock`、`replay`、`connector`、`page_evidence`；
- `audit` 和 `compilation_metadata` 记录创建人、页面证据、字段证据和编译步骤，不改变业务配置身份。
- `platform_pending` 只属于已知平台路径的字段证据状态；它不是 Connector/Insights 的数据质量状态，也不能与 `fact_provenance.source` 混用。

## 4. Canonical hash

两类契约都复用生产实现 `internal/platform/contract.CanonicalJSONHash`：先按 RFC 8785 JCS 规范化，再计算 SHA-256，输出 64 位小写十六进制。数组顺序具有业务含义，不排序。

`DeliveryIntent.canonical_hash` 覆盖规范化后的 intent payload。稳定引用中的 `display_name_snapshot`、`evidence_version` 被显式排除；envelope 身份、生成来源、事实来源和审计元数据不参与。

`PlatformConfiguration.canonical_hash` 覆盖以下显式投影：

```text
schema_version + platform + profile_version
+ intent(schema_version, intent_id, version_number, canonical_hash)
+ payload.profile
+ selected platform profile payload
```

平台业务字段及稳定引用的身份/状态/版本/hash 参与；配置生成来源、事实来源、审计、页面 evidence、编译步骤和引用显示快照不参与。算法标记固定为 `RFC8785-JCS-SHA256(canonical_payload)`。

## 5. 稳定失败结果

Go 校验器使用稳定错误码：

| 错误码 | 含义 |
| --- | --- |
| `UNKNOWN_SCHEMA_VERSION` | schema 版本不受支持 |
| `UNKNOWN_PROFILE_VERSION` | 平台 profile 版本不受支持 |
| `PLATFORM_PROFILE_MISMATCH` | 平台、profile 或 payload discriminator 不一致 |
| `INVALID_STABLE_REFERENCE` | 稳定引用缺少身份或状态所需字段 |
| `INVALID_DELIVERY_INTENT` | 平台无关意图结构无效 |
| `INVALID_PLATFORM_CONFIGURATION` | 平台配置 envelope 无效 |
| `OCEANENGINE_PROJECT_REQUIRED` | 巨量 profile 缺少唯一 Project 或 Project 无效 |
| `INVALID_OCEANENGINE_PROMOTION` | Promotion 结构、身份或引用无效 |
| `CANONICAL_HASH_MISMATCH` | 声明 hash 与生产投影不一致 |
| `CAPABILITY_PENDING` | 平台能力尚无足够证据；结构可保存，但不可执行 |

## 6. Fixtures 与验证

- 平台无关 intent：[`delivery-intent-v1-valid.json`](./fixtures/delivery-intent-v1-valid.json)
- 巨量电商手动 profile：[`delivery-platform-configuration-v2-oceanengine-valid.json`](./fixtures/delivery-platform-configuration-v2-oceanengine-valid.json)
- 磁力能力占位：[`delivery-platform-configuration-v2-magnetic-pending.json`](./fixtures/delivery-platform-configuration-v2-magnetic-pending.json)
- 生产 hash 向量：[`delivery-platform-contracts-v2-hash-vectors.json`](./fixtures/delivery-platform-contracts-v2-hash-vectors.json)

无效 descriptor 覆盖已解析引用缺 ID、discriminator 冲突、未知 schema/profile、缺 Project 和无效 Promotion 引用。仓库使用已有 Ajv Draft 2020 校验栈和同一个 Go canonicalizer；命名契约门为 `npm run contract:check`。
