# 制作中心 Phase 2 验收记录

日期：2026-08-11

## 契约与边界

- 六个视图严格保持为：图片生成、视频生成、音频生成、渲染队列、源素材、失败任务。
- 列表和详情读取 `/api/creative/v1/projects/{project_id}/production-runs`；源素材只读取 `/production-assets`，未退回通用素材库或 Artifact 推断。
- 未调用或写入 QualityCheckRun、MaterialConfirmation、CreativePackage、Delivery 或 Insights 对象。
- Phase 2 未开放中央重试命令；成本、事件、错误和预览均只显示服务端实际返回值。

## 浏览器闭环

在 1672 × 941 概念稿原生尺寸和 780 × 900 窄屏视口验证：

1. 视频生成视图读取到真实 Provider Run，包含 succeeded 与 failed，音频来源缺失以 partial-source warning 呈现。
2. 点击详情按钮打开 36% 右侧抽屉；概览、输入、参数、输出、成本、事件、错误、Attempt 与血缘均来自详情 DTO。
3. 点击来源任务可返回 Creative 原任务；不会在制作中心复制或编辑来源对象。
4. 源素材视图读取生产血缘 AssetVersion，重复输入版本合并并列出 `used_by_runs`。
5. 失败任务视图只返回 failed/expired/partially_succeeded；搜索写入 URL，刷新后恢复筛选与结果。
6. 780px 窄屏无页面级横向溢出；宽表仅在表格容器内部横向滚动。

## 概念稿忠实度清单

- 保留现有 cookies 顶部栏、Project 上下文与创意创作侧栏，不创建第二套外壳。
- 保留低饱和蓝白视觉、白色筛选面板、浅灰表头和单一蓝色主强调。
- 保留来源异常的黄色提示带，并明确“当前结果来自其余可用权威来源”。
- 保留任务、来源任务、状态/进度、输出、成本、更新时间的扫描顺序。
- 保留右侧详情抽屉；小屏改为覆盖式抽屉，大屏保持列表上下文可见。
- 与概念稿的文字差异只来自真实契约：模型不在列表摘要中重复显示；成本不可用时显示“尚不可用”；输出只显示稳定 AssetVersion 数量。

## 自动化门禁

- `npx tsx --test test/production-center-gateway.test.ts test/production-center-view-model.test.ts`
- `go test ./internal/systems/creative ./internal/platform/httpserver`
- 最终交付前运行仓库全量测试、契约检查、前端构建和 `git diff --check`。
