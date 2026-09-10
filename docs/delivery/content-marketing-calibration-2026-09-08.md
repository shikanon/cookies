# 内容营销 Runner 校准

## 已验证路径

内容营销的“抖音号 + UBMax + 互动”路径完成项目创建及单元 Prepare。原视频标题模式完成一次受控创建，并通过保存详情独立核验视频 ID 和标题模式。

手动标题模式完成 Prepare，尚未开放 Submit。测试对象、账户和素材标识不随此文档发布；回归测试使用合成标识。

## 字段和数据来源

- 优化目标按完整父条件查询：`landing_type=7`、`asset_type=2`、`delivery_product=5001`，不要求转化资产。配置绑定当前能力快照。
- UBMax 的预算与出价位于项目层。推荐出价与硬约束分开解析。
- 原生单元只编译实际存在的字段，不生成通用单元的产品主图、落地页、行动号召等字段。
- 原生素材使用 `douyin_video`。当前每个单元绑定一个视频，按精确 ID 及标题核对可见卡片和选中数量。
- `title_mode` 和 `search_terms` 进入 JSON Schema、Go 校验及规范哈希。搜索词最多三个，每个不超过十四字。
- 类别搜索使用末级名称，再核对完整路径。
- 保存详情 `/superior/api/ad/promotion/detail` 中的 `material_group.video_material_info[].aweme_item_id` 和 `data.native_info.origin_item_title_switch` 用于写后核验。

## 执行边界

手动标题使用 `native_promotion_submit_not_calibrated` 阻塞，发生在页面写入和授权消耗前。错误视频 ID、额外视频、缺失标题模式或错误对象身份都不能通过核验。

手动投放、其他载体或目标、指定抖音号尚未完成该路径的提交校准。账户能力不等于 Runner 支持范围。

## 验证

相关 Go、字段契约、Runner 和浏览器测试通过，覆盖原生字段组合、错误素材类型、手动标题缺失、重复选择、视频身份、类别搜索、搜索词清空和提交门禁。原视频标题模式的受控提交及独立回查已通过现场验证。
