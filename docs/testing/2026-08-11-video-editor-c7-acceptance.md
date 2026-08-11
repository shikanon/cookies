# 素材剪辑 C7 本地 MVP 验收证据

日期：2026-08-11

范围：`21-video-material-editor-spec.md` 第 7 节，以及 C0–C7 完成计划的最终发布硬化。

## 结论

C7 的本地代码、真实浏览器、MySQL、BlobStore 与固定 FFmpeg 门禁已通过。远端 required checks 尚未执行，因为本轮没有提交或推送；在远端全部绿色前不标记“已发布”。

## 七项证据

| 验收项 | 本地状态 | 可重复证据 |
| --- | --- | --- |
| 1. 入口 | 通过 | Playwright 从独立入口新建 EditTask，稳定 URL 刷新恢复；Go 服务测试覆盖短剧前贴预填、CreativeVersion 入口和跨项目拒绝；任务卡显示 `manual / short_drama_preroll_v2 / creative_version` 来源。 |
| 2. 稳定媒体引用 | 通过 | v1/v2 contract、codec、operation、compiler 与 lineage 测试仅持久化 Asset ID/Version；C7 compiler 将字幕 style ref 解析为固定 Noto CJK 字体 SHA；输出 lineage 含全部输入与 TimelineVersion。 |
| 3. 恢复与冲突 | 通过 | 两个真实 browser context 同时编辑同一空任务，只有一个保存成功，另一个得到 HTTP 409；新版三轨编辑器提供“载入服务端版本”和“另存为新 EditTask”，并验证载入后恢复 v1。 |
| 4. 预览/导出一致 | 通过 | 固定 FFmpeg 8.1.2 + Noto CJK fixture 同时覆盖 contain/cover/crop、两层图片、原声、配音、音乐、音效、loop/fade、字幕样式与强调；preview/export 视频和音频 framemd5 完全一致并固定哈希。 |
| 5. RenderJob | 通过 | 测试覆盖 enqueue 失败终态、queued/running/succeeded/failed/cancelled、进度、取消、retry_of、缓存复用、runtime 失败终态、lease 续约/回收；UI 轮询不再永久停留 queued。 |
| 6. 权限与权利 | 通过 | policy matrix 覆盖 revoked、expired、channel、territory、purpose；新增 TOCTOU 测试在任务排队后撤权，worker 二次授权并以 `ASSET_USE_REVOKED` 失败，FFmpeg 不启动。 |
| 7. OpenCut 治理 | 通过 | SHA、MIT、NOTICE、SBOM 固定；生产入口的 114 个 runtime module 不再依赖隔离的 OpenCut 适配文件；10/50/100 clip 性能预算为可执行门禁；CI 会重复退出演练。 |

## C7 修复

- 修复新版三轨工作区丢失冲突恢复面板的问题。
- 将编辑时间线冲突统一为稳定 HTTP 409 / `EDIT_TIMELINE_VERSION_CONFLICT`。
- 删除不可达的旧单轨工作区实现，OpenCut 适配代码退出生产依赖图。
- 将字幕 style/version 和固定字体 SHA 接入正式 FFmpeg renderer，并升级 renderer fingerprint 版本以失效旧缓存。
- 修复 `20260811171000` 在 MySQL 中 drop/add 同名外键失败的问题；每个 DDL 步骤可在中断后重入。
- E2E 使用固定镜像生成真实视频/图片并写入 filesystem BlobStore，不再让缺失 demo blob 的 500 被 UI 测试忽略。

## 已运行门禁

```text
npm test                                                        168/168 passed
go test ./...                                                    passed
npm run build                                                    passed
npx playwright test e2e/video-editor-phase1.spec.ts ...          3/3 passed
docker ... sh scripts/verify-video-editor-c0.sh                  passed
npm run test:video-editor-governance                             passed
go test ./internal/platform/migration -run TestC7...             passed
git diff --check                                                 passed
```

浏览器验收宽度：1280、1366、1440、1920；验证资产栏、画布、属性栏和正式导出操作可见，页面无根级横向溢出。

## 发布前剩余动作

1. 按任务范围整理并提交当前 C0–C7 变更，避免混入工作区内其他模块修改。
2. 推送后等待 GitHub required checks 全绿。
3. 生产/预发布环境补一次容量基线和真实对象存储、队列、worker 观察；本地证据不能替代环境容量签字。

当前构建仍报告主应用 chunk 约 1.12 MB 的既有警告；素材剪辑懒加载 chunk 约 67 KB。该警告不阻断本地构建，但应作为全站拆包任务跟踪。
