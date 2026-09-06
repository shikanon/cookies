# OceanEngine Runner v3 控制面收口

| 属性 | 内容 |
| --- | --- |
| 日期 | 2026-08-25 |
| 关联 PR | #66 |
| 状态 | 控制面实现完成；完整 Cookies 端到端实测延期 |
| 延期原因 | 当前 Cookies 没有可用于新建单元的巨量素材 |

## 1. 已完成范围

PR #66 完成以下能力：

- 使用持久化 Edge 会话执行只读校准。
- 覆盖 26 个校准案例。结果为 20 个成功和 6 个稳定阻塞。
- 使用 Runner v3 执行 `prepare` 和一次性授权 `submit`。
- 从 Cookies 投放配置编译项目和单元表单计划。
- 在打开平台页面前检查素材、图片、落地页、商品、品牌和分类引用。
- 顺序创建一个项目和多个单元。
- 在每个阶段回写巨量项目 ID 或单元 ID。
- 字段漂移时保留已确认的对象 ID，并停止后续阶段。
- `field_reconciliation_status=not_checked` 时返回 `result_unknown`。
- 前端显示计划、对象可用性、回读、差异、最终点击边界和平台 ID。
- 投放配置页从当前计划创建真实受控执行，并进入对应 Browser RPA Run。
- 正式“进入真实受控执行”按钮不再调用 `local_simulation`。
- 通过只读探针检查本地 Edge CDP、登录状态和页面广告账户。
- Prepare 在执行前重复会话检查。前端检查结果不作为长期授权。

控制面当前支持以下动作：

- `create_project_and_promotions`
- `create_promotions_in_existing_project`
- `update_promotion_budget`

旧 Runner 仅作为显式回退路径保留。系统不会自动从 v3 切换到旧协议。

## 2. 已取得的真实页面证据

直接 Runner v3 实测已证明：

- 项目新建和编辑可用。
- 单元新建和编辑可用。
- Prepare 停在最终点击边界。
- Submit 最多执行一次最终点击。
- 项目和单元 ID 可以从平台结果取得。
- 异步落地页列表加载后有 25 条记录。
- “我的图片”有 1 个可用图片卡片。
- 行动号召是最多 10 项的多选字段。

证据文件为
[`oceanengine-runner-v3-live-validation-2026-08-25.json`](./evidence/oceanengine-runner-v3-live-validation-2026-08-25.json)。

该证据来自 Runner v3 直接执行。它不证明完整 Cookies 控制面链路。

## 3. 尚未取得的端到端证据

以下完整路径已经接通，但还没有真实成功证据：

```text
Cookies UI
→ Browser RPA API
→ Control Plane Worker
→ Runner v3
→ local Edge
→ OceanEngine
→ platform ID write-back
→ Cookies UI result
```

本次不执行该验证。当前 Cookies 没有可用巨量素材。

取得可用素材后，应使用一次性项目执行该测试。测试结束后删除项目和单元。

## 4. 当前稳定阻塞

| 阻塞项 | 当前处理 |
| --- | --- |
| 素材替换 | 没有 Runner v3 单表单控制面协议。前端阻止执行。 |
| 暂停单元 | 没有 Runner v3 单表单控制面协议。前端阻止执行。 |
| 启用单元 | 没有 Runner v3 单表单控制面协议。前端阻止执行。 |
| 落地页写后集合校验 | Runner 返回 `not_checked`。控制面转为 `result_unknown`。 |
| 行动号召集合校验 | 多选集合还未完成逐项核对。控制面禁止把 `not_checked` 当作成功。 |
| 未绑定巨量对象 | 计划编译阶段阻止执行。后续可用批量导入缓解。 |
| 字节小程序和微信小程序 | 前端明确标记为暂不支持。 |

## 5. 下一次真实测试入口

1. 准备至少一个可用巨量素材引用。
2. 启动并登记本地 Edge 会话。
3. 在投放配置的“检查与提交”页单击“进入真实受控执行”。
4. 在执行中心单击“检查 Edge 会话”。
5. 确认 CDP、登录状态和广告账户均匹配。
6. 生成 Runner v3 计划。
7. 检查所有巨量对象均已绑定平台 ID。
8. 执行 Prepare，并检查字段回读和差异。
9. 用户确认后执行 Submit。
10. 检查项目 ID 和每个单元 ID 已回写。
11. 删除本次测试创建的单元和项目。

不要在 `result_unknown` 后再次 Submit。
