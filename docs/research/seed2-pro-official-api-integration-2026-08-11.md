# Doubao Seed 2.0 Pro 官方 API 与 Adapter 接入调研

> 状态：官方一手资料调研结论，不包含密钥轮换或运行配置变更
>
> 调研日期：2026-08-11
>
> 目标模型：`doubao-seed-2-0-pro-260215`
>
> 本地逻辑别名：`Seed-2-pro`（仅用于 cookies 内部路由，不是方舟模型名）

## 1. 结论

Doubao Seed 2.0 Pro 的方舟在线推理官方协议是：

| 项目 | 官方值 |
| --- | --- |
| 在线推理 Base URL | `https://ark.cn-beijing.volces.com/api/v3` |
| Chat Completions 完整地址 | `POST https://ark.cn-beijing.volces.com/api/v3/chat/completions` |
| Responses 完整地址 | `POST https://ark.cn-beijing.volces.com/api/v3/responses` |
| 鉴权 | `Authorization: Bearer <ARK_API_KEY>` |
| Content-Type | `application/json` |
| 在线推理 Model ID | `doubao-seed-2-0-pro-260215` |
| 可替代的 `model` 值 | 已配置且有权访问的 `ep-...` Endpoint ID |

方舟官方 Chat Completions 文档明确允许 `model` 使用 Model ID 或 Endpoint ID；
方舟基础模型 API 同时确认 `doubao-seed-2-0-pro` 的版本号是 `260215`。
[Chat Completions API](https://api.volcengine.com/api-docs/view?action=ChatCompletions&serviceCode=ark&version=2024-01-01)
[基础模型版本 API](https://api.volcengine.com/api-explorer/?action=ListFoundationModelVersions&groupName=%E5%9F%BA%E7%A1%80%E6%A8%A1%E5%9E%8B&serviceCode=ark&version=2024-01-01)

方舟官方 Go SDK 的默认 Base URL 也是
`https://ark.cn-beijing.volces.com/api/v3`，Chat 和 Responses 分别追加
`/chat/completions` 与 `/responses`；使用 API Key 时 SDK 写入
`Authorization: Bearer ...`。
[SDK config.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/config.go)
[SDK chat_completion.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/chat_completion.go)
[SDK responses.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/responses.go)
[SDK client.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/client.go)

因此，如果老板要求“走 Adapter”，正确设计不是把方舟路径机械地改成
`http://118.196.44.61:9060/api/v3`，而是：

```text
cookies
  -- Authorization: Bearer <ADAPTER_SERVICE_TOKEN>
  --> Adapter /v1/chat/completions
        -- Authorization: Bearer <ARK_API_KEY>
        --> Ark /api/v3/chat/completions
```

这两层 Bearer 值用途不同：前者认证 cookies 是否有权调用企业 Adapter，后者认证
Adapter 是否有权调用火山方舟。除非 Adapter 的设计明确采用 API Key 透传，否则不能把
方舟 API Key 当作 Adapter 服务 Token，也不能把 Adapter Token 直接发给方舟。

## 2. 在线推理与 Coding Plan 不可混用

`doubao-seed-2-0-pro-260215` 是方舟在线推理 Model ID，对应
`https://ark.cn-beijing.volces.com/api/v3`。方舟 Coding Plan 是另一套计费与路由：

| 类型 | Base URL | `model` 示例 |
| --- | --- | --- |
| 在线推理 | `https://ark.cn-beijing.volces.com/api/v3` | `doubao-seed-2-0-pro-260215` |
| Coding Plan | `https://ark.cn-beijing.volces.com/api/coding/v3` | `doubao-seed-2.0-pro` |

官方资料明确提醒这两套 Base URL 和模型命名不同。AI 原生广告当前表中记录的是
`doubao-seed-2-0-pro-260215`，所以本方案按“在线推理”对接；只有负责人明确给出
Coding Plan Key 与套餐授权时，才切换到 Coding Plan 契约。
[方舟官方社区：在线推理与 Coding Plan 区别](https://developer.volcengine.com/articles/7616633140483719219)

## 3. 推荐接口：Chat Completions

cookies 当前文本生成抽象使用 `messages`，并消费
`choices[0].message.content`，与方舟 Chat Completions 契约直接一致。因此第一阶段应继续
使用 Chat Completions，不必为了接入 Seed 2.0 Pro 同时迁移 Responses API。

### 3.1 最小请求

```bash
curl https://ark.cn-beijing.volces.com/api/v3/chat/completions \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seed-2-0-pro-260215",
    "messages": [
      {"role": "system", "content": "你是广告策划助手。"},
      {"role": "user", "content": "只回复 OK。"}
    ],
    "stream": false,
    "max_tokens": 32
  }'
```

官方请求字段包括 `model`、`messages`、`stream`、`max_tokens`、`temperature`、
`top_p`、`tools` 等。最小连通性验证只应发送稳定、必需的字段；JSON 格式约束等能力要在
连通后单独做模型能力测试，不能把一次 `400` 参数错误误判为鉴权失败。
[Chat Completions API](https://api.volcengine.com/api-docs/view?action=ChatCompletions&serviceCode=ark&version=2024-01-01)

### 3.2 最小响应

非流式响应应至少包含：

```json
{
  "id": "0217...",
  "object": "chat.completion",
  "model": "doubao-seed-2-0-pro-260215",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "OK"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 18,
    "completion_tokens": 1,
    "total_tokens": 19
  }
}
```

业务解析应读取 `choices[0].message.content`；`usage` 用于计量；`model` 记录实际执行的
模型版本。流式调用返回 SSE，最后以 `data: [DONE]` 结束。本轮对接先用非流式请求，避免
把 SSE 解析问题与 Adapter 鉴权问题混在一起。

### 3.3 OpenAI Python SDK 兼容方式

方舟接口兼容 OpenAI SDK 的 Chat Completions 调用形状。关键是 Base URL 必须是方舟的
`/api/v3`，SDK 会再追加 `/chat/completions`：

```python
import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["ARK_API_KEY"],
    base_url="https://ark.cn-beijing.volces.com/api/v3",
)

response = client.chat.completions.create(
    model="doubao-seed-2-0-pro-260215",
    messages=[{"role": "user", "content": "只回复 OK。"}],
    max_tokens=32,
)

print(response.choices[0].message.content)
```

“OpenAI 兼容”指请求和响应形状兼容，不代表方舟路径变成 OpenAI 官方的 `/v1`，也不代表
所有 OpenAI 扩展字段均被当前模型支持。应以方舟对应接口的字段表为准。

## 4. Responses API 是可选的后续能力

方舟官方当前快速开始更偏向 Responses API：

```bash
curl https://ark.cn-beijing.volces.com/api/v3/responses \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seed-2-0-pro-260215",
    "input": "只回复 OK"
  }'
```

OpenAI SDK 对应调用为：

```python
response = client.responses.create(
    model="doubao-seed-2-0-pro-260215",
    input="只回复 OK",
)
```

Responses 的请求字段是 `input`，响应主体是 `object: "response"`，内容位于 `output`
数组的 message/content 项中；这与当前 Chat Completions 的 `messages` 和
`choices[0].message.content` 不同。若后续需要内置工具、跨轮 `previous_response_id` 或
后台响应任务，再独立启用 `api_mode=responses`，不要让 Adapter 静默把两种 schema 混用。
[方舟快速开始](https://www.volcengine.com/docs/82379/1795150)
[方舟 Responses 工具调用](https://www.volcengine.com/docs/82379/1958524?lang=zh)

## 5. 企业 Adapter 的正确转发契约

### 5.1 Adapter 对 cookies 暴露的接口

如果现有 Adapter 是按 OpenAI 兼容方式设计，对 cookies 应保持：

```http
POST http://118.196.44.61:9060/v1/chat/completions
Authorization: Bearer <ADAPTER_SERVICE_TOKEN>
Content-Type: application/json
```

请求体保持 Chat Completions 形状：

```json
{
  "model": "doubao-seed-2-0-pro-260215",
  "messages": [{"role": "user", "content": "只回复 OK。"}],
  "stream": false,
  "max_tokens": 32
}
```

`Seed-2-pro` 可以继续作为 cookies 的路由别名，但 Adapter 收到的 `model` 必须是
`doubao-seed-2-0-pro-260215`，或由 Adapter 的显式 allowlist 映射得到该值。

### 5.2 Adapter 对方舟发出的请求

Adapter 验证 cookies 调用凭证后，应重新构造上游请求：

```http
POST https://ark.cn-beijing.volces.com/api/v3/chat/completions
Authorization: Bearer <ARK_API_KEY>
Content-Type: application/json
```

Adapter 必须：

1. 在服务端安全配置中持有方舟 API Key，不把它返回给 cookies 或浏览器；
2. 丢弃下游传入的 `Host` 和 `Authorization`，由 Adapter 写入方舟 Host 与方舟 Bearer；
3. 只允许预先配置的模型，避免调用者任意改写 `model`；
4. 保留方舟 HTTP 状态码、错误码和 Request ID，便于区分 `401`、`403`、`404`、`429`、
   `5xx`；
5. 对成功响应保持 Chat Completions schema，至少完整保留 `model`、`choices`、`usage`；
6. 对 `401/403` 不自动重试；只对超时、`429` 和可恢复 `5xx` 做有上限的退避重试；
7. 日志只记录 token 指纹或凭证版本，不记录明文凭证与完整商品提示词。

官方 Go SDK 的重试判断同样只把 `429`、`5xx`、EOF 等视为可重试，不把鉴权错误纳入
自动重试。
[SDK client.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/client.go)

### 5.3 两个独立的预检

Adapter 上线前必须分别验证两跳，不能只验证 `/healthz`：

```text
预检 A：在 Adapter 机器上用 ARK_API_KEY 直连方舟官方地址
预检 B：在 cookies 机器上用 ADAPTER_SERVICE_TOKEN 调 Adapter /v1 地址
```

判定方式：

| 现象 | 能证明什么 | 不能证明什么 |
| --- | --- | --- |
| Adapter `/healthz` 为 200 | Adapter 进程存活 | 上游方舟 Key、模型权限可用 |
| Adapter `/v1/chat/completions` 为 401，错误为 `invalid or missing bearer token` | Adapter 前门拒绝调用凭证 | 方舟 Key 是否有效 |
| Adapter `/v1/chat/completions` 为 401，错误包含 `provider` 与 `model error` | 调用凭证已进入模型路由，但 Adapter 的上游 Provider 鉴权或模型映射失败 | Adapter 前门 Token 无效 |
| Adapter `/api/v3/chat/completions` 为 404 | Adapter 未暴露方舟原生路径 | 官方方舟路径错误 |
| 直连方舟 `/api/v3/chat/completions` 为 401/403 | 方舟 API Key 或账户授权有问题 | Adapter 服务 Token 是否有效 |
| 直连方舟成功、Adapter 失败 | Adapter 调用凭证、路由或转发配置有问题 | 模型本身不可用 |

## 6. 与 cookies 当前代码的对齐

仓库已经具备两条不同的文本链路：

1. [`ark_text_adapter.go`](../../internal/platform/provider/ark_text_adapter.go) 默认使用
   `https://ark.cn-beijing.volces.com/api/v3`，追加 `/chat/completions` 并发送方舟 Bearer；
2. [`adapter_gateway_text.go`](../../internal/platform/provider/adapter_gateway_text.go) 对
   Adapter 发送 OpenAI 兼容请求；
3. [`gateway_config.go`](../../internal/platform/provider/gateway_config.go) 明确规定：
   `connection_type=ark` 时在 Base URL 后追加 `/chat/completions`，
   `connection_type=adapter_gateway` 时使用 `/v1/chat/completions`。

因此有两种合法部署：

| 部署方式 | cookies connection type | cookies Base URL | cookies 加密凭证 |
| --- | --- | --- | --- |
| 直连方舟 | `ark` | `https://ark.cn-beijing.volces.com/api/v3` | 方舟 API Key |
| 经企业 Adapter | `adapter_gateway` | `http://118.196.44.61:9060`（本地联调） | Adapter 服务 Token |

老板要求“走 Adapter”时应选择第二行；方舟 API Key 要配置在 Adapter 的上游 provider
配置里，而不是写入 cookies 的 `connection_adapter_shared`。现有
`connection_adapter_shared` 中的加密凭证语义是 Adapter 调用方 Token。

特别注意：把 `adapter_gateway` 的 Base URL 直接设为
`http://118.196.44.61:9060/api/v3` 会被 cookies 拼成
`.../api/v3/v1/chat/completions`，不是方舟协议，也不是当前 Adapter 协议。

## 7. 推荐实施顺序

1. 让 Adapter 负责人提供一条经过脱敏但结构完整的成功 curl，确认其前门固定为
   `/v1/chat/completions`、Header 为 `Authorization: Bearer`；
2. Adapter 负责人用方舟控制台创建的 API Key 在 Adapter 主机执行“直连方舟”预检；
3. 在 Adapter 配置中把模型 `doubao-seed-2-0-pro-260215` 的上游改为
   `https://ark.cn-beijing.volces.com/api/v3/chat/completions`；
4. 为 cookies 签发或确认专用 Adapter 服务 Token；
5. 先用最小文本请求验证 Adapter，成功后再加 JSON 输出提示、长商品上下文和超时参数；
6. Adapter 预检通过后，才在 cookies 数据库轮换加密后的 Adapter 服务 Token；
7. 以 AI 原生广告“发送并分析”做端到端验收，并核对 Adapter 与方舟两个 Request ID。

### 7.1 2026-08-11 本地对照探测

对同一 Adapter 地址、同一 Seed-2-pro 请求做了控制变量对照：

- 明确无效的随机 Bearer 返回 `401`，错误为 `invalid or missing bearer token`；
- 新签发的 Adapter Bearer 返回 `401`，但错误为 `model error`，并标记
  `provider: ominilink`。

这证明新 Bearer 已通过 Adapter 前门，当前阻塞位于 Adapter 内部的 `ominilink` 上游凭据、
模型授权或模型映射。不能把第二种 `401` 继续归因于 cookies 保存的 Adapter Token。

### 7.2 当前远端修复单

Cookies 侧已经固定使用 Adapter，当前不能再通过更换 cookies 里的 Bearer、修改请求字段或
改用模型别名解决故障。Adapter 负责人需要在 `118.196.44.61:9060` 对应服务中把
Seed-2-pro 的上游配置收敛为下面这一条：

| 配置项 | 必须值 |
| --- | --- |
| Adapter 对外路径 | `/v1/chat/completions` |
| Adapter 调用方鉴权 | `Authorization: Bearer <ADAPTER_SERVICE_TOKEN>` |
| 允许的请求模型 | `doubao-seed-2-0-pro-260215` |
| 上游 Provider | `ark`，不能落到当前报错的 `ominilink` 路由 |
| 上游 Base URL | `https://ark.cn-beijing.volces.com/api/v3` |
| 上游路径 | `/chat/completions` |
| 上游鉴权 Header | `Authorization` |
| 上游鉴权值 | `Bearer <ARK_API_KEY>` |
| 上游请求模型 | `doubao-seed-2-0-pro-260215`，或已获授权的 `ep-...` |

远端修改时应使用失败请求的 `Trace-Id` 与 `X-Request-Id` 检索 Adapter 日志，确认请求为何被
路由到了 `ominilink`。修复后先在 Adapter 主机直连方舟做预检 A，再从 cookies 主机做预检 B；
两次都返回 HTTP 200 后，才算链路恢复。只验证 `/healthz`、只验证 Adapter Token 或只验证
方舟 Key 都不能作为验收结果。

## 8. 验收标准

- 直连方舟最小请求返回 HTTP 200，`choices[0].message.content` 非空；
- 经 Adapter 的相同请求返回 HTTP 200，实际 `model` 为
  `doubao-seed-2-0-pro-260215`；
- 错误的 Adapter Token 在 Adapter 前门返回 401，且不会访问方舟；
- 正确 Adapter Token + 错误方舟 Key 能返回可识别的上游鉴权错误，且包含关联 Request ID；
- cookies 中只保存 Adapter 服务 Token 的加密密文，浏览器、日志、数据库路由表均无明文；
- AI 原生广告需求分析成功生成并持久化，离开页面后可恢复；
- `429`/可恢复 `5xx` 有界重试，`401/403` 不重试。

## 9. 官方来源

所有来源访问日期均为 2026-08-11：

1. [火山方舟 Chat Completions API](https://api.volcengine.com/api-docs/view?action=ChatCompletions&serviceCode=ark&version=2024-01-01)：完整 URL、API Key 鉴权、请求/响应字段、Model ID 或 Endpoint ID。
2. [火山方舟快速开始](https://www.volcengine.com/docs/82379/1795150)：官方 Base URL、Responses curl、OpenAI SDK 初始化方式。
3. [火山方舟 Responses 工具调用](https://www.volcengine.com/docs/82379/1958524?lang=zh)：Responses `input`、`output`、`previous_response_id` 契约。
4. [火山方舟基础模型版本 API](https://api.volcengine.com/api-explorer/?action=ListFoundationModelVersions&groupName=%E5%9F%BA%E7%A1%80%E6%A8%A1%E5%9E%8B&serviceCode=ark&version=2024-01-01)：`doubao-seed-2-0-pro` 与版本 `260215`。
5. [火山方舟官方 Go SDK config.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/config.go)：默认 Base URL。
6. [火山方舟官方 Go SDK chat_completion.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/chat_completion.go)：Chat 路径与请求方法。
7. [火山方舟官方 Go SDK responses.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/responses.go)：Responses 路径与生命周期。
8. [火山方舟官方 Go SDK client.go](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/client.go)：Bearer Header、重试分类和完整 URL 拼接。
9. [火山方舟官方社区：在线推理与 Coding Plan 区别](https://developer.volcengine.com/articles/7616633140483719219)：两套 Base URL、模型命名和计费边界。
