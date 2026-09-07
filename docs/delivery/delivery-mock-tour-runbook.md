# 历史投放演示已下线

Tour 准备与复位、旧 Recommendation 生成/采纳/拒绝、OutcomeSimulation 创建、演示指标生成和模拟告警生成已下线。旧写入接口返回 HTTP 410，错误码为 `DELIVERY_DEMO_RETIRED`。

历史计划、审批、执行、指标和演示运行仍可读取。历史哈希和来源标记不变。旧链接进入正常业务页面，不再启用演示模式。

新建计划只使用项目资料和用户填写的配置，不再填入演示落地页、Pixel ID 或无来源预算。缺少账户、必要追踪配置或预算时，先补齐表单。

真实投放请进入“执行中心”。受控 Browser RPA 保留审批、预算、平台证据和结果核验。能力未配置时返回 `EXECUTION_UNAVAILABLE`，不会回退到模拟执行。

上线前概率预测继续使用 Mechanistic Simulation；投后监控使用 Connector 巡检。预测结果不是平台已发生事实。
