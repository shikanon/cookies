# 素材剪辑第一阶段一手资料调研（2026-08-10）

## 1. 调研目的与结论

本调研服务于 [`21-video-material-editor-spec.md`](../21-video-material-editor-spec.md) 的最终 MVP，而不是把“前贴 + 原视频拼接”做成一次性页面。第一阶段应交付真正可操作的单主视频轨闭环，同时固定四条不会在多轨、字幕和音频阶段推倒重来的边界：

1. cookies 自有 Timeline JSON、`AssetVersionRef`、`TimelineVersion` 和 `RenderJob` 是业务事实源；OpenCut 只提供经过隔离的交互/命令实现。该边界直接来自上位规格的架构约束。[来源：本仓库规格 §4.2、§5](../21-video-material-editor-spec.md)
2. OpenCut 采用固定提交、局部抽取和可删除 Adapter，不嵌入完整应用，也不追踪浮动 `main`。OpenCut 官方目前明确处于重写阶段，并建议当前使用 Classic；Classic 官方仓库已归档且只读。[来源：OpenCut 官方 README](https://github.com/OpenCut-app/OpenCut#status)、[OpenCut Classic 官方仓库](https://github.com/OpenCut-app/opencut-classic)
3. 第一阶段正式渲染应采用统一的解码、裁切、时间戳归零、规格归一和重编码图，不把 concat demuxer 的 stream-copy 捷径作为主引擎。FFmpeg 官方说明 concat demuxer 的 `inpoint/outpoint` 对非帧内编码可能输出范围外的包；官方 FAQ 则把 concat filter 推荐给需要重编码的场景。[来源：FFmpeg concat demuxer](https://ffmpeg.org/ffmpeg-formats.html#concat)、[FFmpeg FAQ §3.14](https://ffmpeg.org/faq.html#How-can-I-join-video-files_003f)
4. 浏览器即时预览只负责交互反馈；低清预览与正式导出必须由同一个时间线快照和同一个服务端编译器产生。该要求是上位规格“同一快照、确定性渲染”的明确约束。[来源：本仓库规格 §4.2、§7](../21-video-material-editor-spec.md)

因此，第一阶段的正确终点是“单轨编辑能力完整 + 保存恢复可靠 + 真实 FFmpeg 预览/导出闭环 + 为 v2 多轨保留稳定扩展缝”，而不是扩充当前 UI 上的静态轨道条。

## 2. 一手资料与采用范围

| 来源 | 已确认事实 | 第一阶段决策 |
| --- | --- | --- |
| [OpenCut 主仓库 README（当前）](https://github.com/OpenCut-app/OpenCut#status) | 主仓库正在从头重写，方向包括 Rust core、Editor API、插件、headless；官方建议当前使用 Classic | 主仓库只作为未来方向观察源，不作为第一阶段运行时依赖 |
| [OpenCut Classic 固定提交](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea) | Classic 是已有 Web 编辑器实现，但仓库已归档、不再维护 | 只从固定 SHA 抽取边界清楚的 timeline 算法与交互；由 cookies 自维护 |
| [Classic timeline 类型](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/types.ts) | 模型把 main、overlay、audio 分组，并区分 video/text/audio/image 等元素；元素具有 `startTime/duration/trimStart/trimEnd` | 借鉴 track/element/源区间分离，不把 OpenCut 类型直接持久化 |
| [Classic split command](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/timeline/element/split-elements.ts)、[commands manager](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/commands.ts)、[snapping](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/snapping) | Classic 已有分割、命令历史与吸附的独立实现位置 | 复用/改写纯逻辑，禁止把 `EditorCore`、存储或渲染器一起带入 |
| [Classic save manager](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/save-manager.ts) | 保存状态显式区分 `isSaving` 与保存期间到达的 `hasPendingSave`，并使用 debounce | 借鉴 pending-save 状态机，修复旧保存回包误清除新编辑的竞态 |
| [Classic MediaTime](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/wasm/media-time.ts) | 时间使用受控整数 tick/branded type，并集中完成构造和舍入 | 第一阶段集中毫秒/帧换算，v2 可换为 tick/frame 而不改 command 接口 |
| [Classic MIT License（固定提交）](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/LICENSE) | MIT 允许使用、复制、修改、分发和商业使用，但副本或实质部分必须保留版权和许可声明 | 固定 SHA、保留 LICENSE/NOTICE、记录复制/改写文件；新增代码须更新清单 |
| [FFmpeg concat demuxer](https://ffmpeg.org/ffmpeg-formats.html#concat) | concat script 支持 `duration/inpoint/outpoint`；非帧内编码的裁切可能包含范围外包，片段时间戳也可能重叠 | 不把 demuxer + `-c copy` 当作用户上传素材的确定性裁切方案 |
| [FFmpeg concat filter](https://ffmpeg.org/ffmpeg-filters.html#concat) | filter 可连接多段音视频；各片段必须从时间戳 0 开始，分辨率等参数需由调用方显式统一 | 每个 Clip 先 `trim/atrim`、`setpts/asetpts`，再规格归一和 concat |
| [FFmpeg complex filtergraph](https://ffmpeg.org/ffmpeg.html#Complex-filtergraphs) | `-filter_complex` 可表达多输入/多输出图；`overlay` 与 `amix` 是官方示例 | 第一阶段即输出具名输入/输出的编译计划，后续叠加和混音只增加节点 |
| [FFmpeg `-progress`](https://ffmpeg.org/ffmpeg.html#toc-Generic-options) | 以周期性 `key=value` 输出机器可读进度，每组以 `progress=continue/end` 结束，周期由 `-stats_period` 控制 | Worker 解析稳定字段并写 RenderJob；原始 stderr 仅用于日志，不作为 API |
| [ffprobe JSON writer](https://ffmpeg.org/ffprobe.html#Writers) | ffprobe 可输出机器可读 JSON | 上传入库、导出验收与 golden 测试统一用 ffprobe 结构化探测 |
| [FFmpeg 官方许可说明](https://ffmpeg.org/legal.html) | FFmpeg 基础许可为 LGPL 2.1+；启用部分可选 GPL 组件后整体适用 GPL，`--enable-nonfree` 还影响可分发性 | 固定 Worker 的 FFmpeg 版本、构建来源和 configure flags；许可证审计不能只写“用了 FFmpeg” |

## 3. OpenCut 可复用边界

### 3.1 固定版本

第一阶段唯一允许抽取代码的上游快照为 OpenCut Classic `cf5e79e919144200294fb9fed22a222592a0aeea`。本仓库已经用 [`third_party/opencut-timeline/NOTICE.md`](../../third_party/opencut-timeline/NOTICE.md) 记录来源，用 [`third_party/opencut-timeline/UPSTREAM.md`](../../third_party/opencut-timeline/UPSTREAM.md) 固定升级与退出政策，并保存了完整 [`LICENSE`](../../third_party/opencut-timeline/LICENSE)。这些文件必须成为 CI 许可门禁的一部分，而不是只用于说明。

固定提交而非浮动分支的理由不是偏好：OpenCut 主仓库官方明确声明正在重写；Classic 官方仓库又已归档。任何自动跟随上游都会同时引入“持续变化”和“无人维护”两类风险。[来源：OpenCut 官方 README](https://github.com/OpenCut-app/OpenCut#status)、[OpenCut Classic 官方仓库](https://github.com/OpenCut-app/opencut-classic)

### 3.2 第一阶段允许复用

- 时间线坐标换算、命中测试、播放头和缩放交互；上游位置见 [timeline components](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/components) 与 [zoom controller](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/controllers/zoom-controller.ts)。
- Clip 移动、裁切、分割、吸附的纯算法和 pointer session；上游位置见 [move command](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/timeline/element/move-elements.ts)、[resize controller](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/controllers/resize-controller.ts)、[split command](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/timeline/element/split-elements.ts) 与 [snapping](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/snapping)。
- command/undo/redo 的组织方式；上游位置见 [commands manager](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/commands.ts) 与 [base command](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/base-command.ts)。
- 保存期间新修改不丢失的状态机；上游 [save manager](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/save-manager.ts) 显式维护 debounce、`isSaving` 和 `hasPendingSave`，可以借鉴状态转换，但不能复用其持久化后端。

### 3.3 第一阶段禁止复用

- 不嵌入 OpenCut 页面、Next 路由、用户/项目/数据库、IndexedDB、上传、签名 URL、浏览器导出或完整 `EditorCore`。Classic 官方项目结构包含 Web、数据库/Redis 和 Rust/WASM 等完整应用依赖，这不是可嵌入的小型时间线包。[来源：OpenCut Classic 官方 README](https://github.com/OpenCut-app/opencut-classic#project-structure)
- 不把 OpenCut 当作可安装的稳定 timeline npm 包。Classic 根目录是 Bun workspace，Web manifest 标记为 `private` 并依赖 `opencut-wasm` 等完整应用组件；正确固定方式是 commit SHA + 抽取文件清单/哈希 + NOTICE，而不是新增运行时 npm 依赖。[来源：Classic 根 `package.json`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/package.json)、[Classic Web `package.json`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/package.json)
- 不让 `mediaId`、OpenCut track/element 类型进入 Go API 或数据库。上位规格要求 OpenCut 代码位于独立适配层，Creative API 与业务库只能看到 cookies Timeline JSON。[来源：本仓库规格 §4.2](../21-video-material-editor-spec.md)
- 不复用上游预览/导出作为权威结果。OpenCut Classic 自身曾明确把预览和导出列为重构区域；cookies 的低清预览与最终导出必须继续使用受控 FFmpeg Worker。[来源：OpenCut Classic 官方 README 的 contributing 说明](https://github.com/OpenCut-app/opencut-classic#contributing)、[本仓库规格 §4.2](../21-video-material-editor-spec.md)

### 3.4 Adapter 契约

`OpenCutAdapter` 应是纯函数边界：

```text
OpenCut-derived UI command
  -> cookies EditorCommand
  -> reduce + normalize + validate
  -> cookies EditingTimeline snapshot
  -> save immutable TimelineVersion
```

最低映射为 `startTime/duration/trimStart/trimEnd/mediaId` 到 `timeline_start_ms/timeline_end_ms/source_in_ms/source_out_ms/AssetVersionRef`。OpenCut 的 `mediaId` 只能是前端会话标识，持久化前必须恢复为稳定 `asset_id + version`。OpenCut 官方类型确实把 `mediaId` 与时间范围放在 element 中；cookies v1 schema 则明确使用 `asset_ref` 和源/时间线区间。[来源：OpenCut Classic `types.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/types.ts)、[本仓库 v1 schema](../schemas/editing-timeline-v1.schema.json)

## 4. FFmpeg 渲染技术结论

### 4.1 为什么不能以 concat demuxer 为主路径

concat demuxer 适合同格式、同流参数且不需要重编码的场景，但官方文档明确警告：`inpoint/outpoint` 对非帧内编码可能产生裁切范围外的包，片段包时间戳可能重叠；用户上传素材无法保证 GOP、分辨率、像素格式、音频布局和采样率一致。[来源：FFmpeg concat demuxer](https://ffmpeg.org/ffmpeg-formats.html#concat)

推论：若第一阶段以 `-c copy` 拼接为正式引擎，后续一加入精确裁切、统一画布、字幕、叠加或混音就必须重写。因此第一阶段应直接使用 filtergraph 重编码路径；只有满足严格同构条件且有等价 golden 的缓存优化，未来才可增加 stream-copy 快路径。

### 4.2 第一阶段规范化编译图

每个主视频 Clip 编译为独立的音视频支路：

```text
video: trim=start:end -> setpts=PTS-STARTPTS
       -> scale/pad/crop to output profile -> fps -> format
audio: atrim=start:end -> asetpts=PTS-STARTPTS
       -> aresample=48000 -> channel layout normalize
all clip A/V pairs -> concat=n=N:v=1:a=1 -> encode MP4/H.264/AAC
```

FFmpeg concat filter 要求各片段从时间戳 0 开始，并要求调用方显式统一分辨率等参数；`setpts/asetpts` 正是官方提供的时间戳变换过滤器。[来源：FFmpeg concat filter](https://ffmpeg.org/ffmpeg-filters.html#concat)、[FFmpeg setpts/asetpts](https://ffmpeg.org/ffmpeg-filters.html#setpts_002c-asetpts)

无音轨视频必须在编译计划中显式处理（生成静音或按输出策略省略），不能依赖 FFmpeg 自动选流；复杂 filtergraph 的输出应使用 label 和 `-map` 明确绑定。FFmpeg 官方说明复杂 filtergraph 可有多输入/输出，具名输出由 `-map` 选择。[来源：FFmpeg complex filtergraphs](https://ffmpeg.org/ffmpeg.html#Complex-filtergraphs)

### 4.3 为多轨、字幕和音频预留的唯一扩展点

第一阶段 compiler 不应直接拼命令字符串，而应先生成 renderer-neutral `RenderPlan`：inputs、trim ranges、normalized visual/audio streams、compositions、outputs。FFmpeg 命令只是该计划的一个后端。

后续扩展只新增节点：

- 三条视频/图片叠加轨：主画面归一后增加 `overlay` 链；FFmpeg 官方把 `overlay` 作为多输入复杂图的标准示例。[来源：FFmpeg complex filtergraphs](https://ffmpeg.org/ffmpeg.html#Complex-filtergraphs)
- 字幕：从结构化 caption clips 生成确定性的 ASS/SRT 中间件并用 `subtitles` 烧录；字体目录与样式必须进入 RenderPlan 和 renderer fingerprint。[来源：FFmpeg subtitles filter](https://ffmpeg.org/ffmpeg-filters.html#subtitles-1)
- 配音、音乐、音效：各轨先 `atrim/asetpts/adelay/volume/afade`，最后进入 `amix`；FFmpeg 官方说明 `amix` 可混合多输入并定义结束策略和归一化。[来源：FFmpeg amix](https://ffmpeg.org/ffmpeg-filters.html#amix)、[FFmpeg afade](https://ffmpeg.org/ffmpeg-filters.html#afade)

这是一项基于官方 filtergraph 能力与本项目 MVP 的工程推论：第一阶段不实现这些 UI，但 compiler、timeline v2 设计和 golden fixture 不能假设“永远只有一条视频和一条自动音轨”。

### 4.4 异步 RenderJob

FFmpeg 是渲染进程，不提供 cookies 所需的业务队列、权限、幂等和跨设备恢复；这些由 Render Worker/RenderJob 承担。上位规格明确要求 RenderJob 包含输入时间线版本、输出规格、进度、成本、日志和结果，并支持排队、取消、失败原因、重试和代理复用。[来源：本仓库规格 §5、§7](../21-video-material-editor-spec.md)

第一阶段应固定如下状态机：

```text
queued -> running -> succeeded
                  -> failed -> queued (retry creates/requeues an auditable attempt)
queued/running -> cancel_requested -> cancelled
```

实现约束：

- Job 必须冻结 `timeline_version_id + output_profile + renderer_fingerprint`，不能读取“当前最新草稿”。该约束来自不可变 TimelineVersion 与 RenderJob 输入模型。[来源：本仓库规格 §5](../21-video-material-editor-spec.md)
- 幂等/复用键至少覆盖以上三项和所有输入 `AssetVersionRef`；否则同一 TimelineVersion 在 FFmpeg/字体/编码器版本变化后可能错误命中旧结果。这是为满足“部分代理复用”和预览/导出一致性作出的工程推论。[来源：本仓库规格 §7](../21-video-material-editor-spec.md)
- Worker 用 `-progress pipe:1 -stats_period ...` 获取机器可读进度；API 只发布归一化进度和稳定 `error_code`，stderr 作为诊断日志。[来源：FFmpeg `-progress`](https://ffmpeg.org/ffmpeg.html#toc-Generic-options)
- 取消必须同时持久化意图并终止对应子进程；输出先写 job 私有临时文件，成功探测后再原子发布，避免取消/失败产生可见半成品。这是应用层可靠性设计，不是 FFmpeg 自动提供的能力。
- 入队失败不能留下永久 `queued` 记录：创建 Job 与投递应通过事务 outbox、可扫描持久队列或周期性 reconciliation 保证最终一致。这是异步任务可靠性约束。

## 5. 不给 MVP 后续留坑的数据约束

### 5.1 v1 保持可读，v2 另立版本

现有 [`editing-timeline-v1.schema.json`](../schemas/editing-timeline-v1.schema.json) 已表达 `AssetVersionRef`、源区间、时间线区间、固定输出规格以及 caption/voiceover/music/sfx 的基础角色，但 `additionalProperties: false`，且无法表达三条叠加轨、z-order、画布变换、字幕样式与音频 fade。不能偷偷向 v1 写未声明字段。

第一阶段继续读写 v1；在开始多轨 UI 前定义 `editing-timeline/v2`，提供显式 `v1 -> v2` 纯迁移，并永远保留历史 v1 snapshot 的读取/渲染能力。这与既有实施方案的版本策略一致。[来源：本仓库实施方案 §3.3、§9.1](../plans/2026-08-07-video-material-editor-opencut-integration-plan.md)

### 5.2 v2 现在就要冻结的概念边界

第一阶段无需实现 v2，但以下概念必须进入 v2 ADR/schema 草案，避免单轨代码固化错误假设：

| 概念 | 约束 | 来源 |
| --- | --- | --- |
| Track | `id/type/role/order-or-z-index/clips`；不要用 UI 行号作身份 | MVP 要求 3 条视频/叠加轨及字幕、配音、音乐、音效轨；[规格 §6](../21-video-material-editor-spec.md) |
| Clip | 稳定 `id`、`asset_ref` 或内容载荷、timeline range、source range；timeline 与 source 不能混成一个 duration | OpenCut element 明确区分 start/duration/trim；cookies v1 已分开表达；[OpenCut types](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/types.ts)、[v1 schema](../schemas/editing-timeline-v1.schema.json) |
| Canvas | 比例、宽高、帧率、安全区属于版本化时间线/输出 profile，不属于单个视频素材 | MVP 要求 9:16、16:9、1:1 及裁切定位；[规格 §6](../21-video-material-editor-spec.md) |
| Visual transform | position/scale/crop/opacity 是 clip 属性；默认值显式化，预览和渲染共用 | MVP 要求基础裁切与定位；[规格 §6](../21-video-material-editor-spec.md) |
| Audio | source audio、voiceover、music、sfx 都按 clip/track 表达；gain/mute/fades 是结构化值 | MVP 要求音量和淡入淡出；[规格 §6](../21-video-material-editor-spec.md) |
| Caption/style | 文本、时间范围和 style ref 分离；品牌字体必须固定为可渲染资源版本 | MVP 要求人工校对、品牌样式，验收比较字幕/字体；[规格 §6、§7](../21-video-material-editor-spec.md) |
| Provenance | 每个媒体引用使用 `asset_id + version`；输出记录所有输入 refs、TimelineVersion、RenderJob 和 renderer fingerprint | MVP 验收要求稳定 Asset ID/Version、原片不覆盖；[规格 §7](../21-video-material-editor-spec.md) |

### 5.3 时间语义

v1 使用整数毫秒，而首发输出固定 30fps；一帧约为 33.333…ms，不能被整数毫秒精确表示。[来源：v1 的 `frame_rate: 30` 与整数 `*_ms`](../schemas/editing-timeline-v1.schema.json)

OpenCut Classic 也没有在 UI 中散落浮点秒，而是用受控整数 tick 的 `MediaTime` 类型并集中构造/舍入；这支持“时间换算只能在 adapter/normalizer 出现”的边界，而不意味着 cookies 应持久化 OpenCut 的 `MediaTime`。[来源：OpenCut Classic `media-time.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/wasm/media-time.ts)

为避免反复分割/裁切后的累积漂移，第一阶段必须只有一个时间归一函数，并规定：

- UI 拖动值先吸附到输出帧边界，command/reducer 使用同一舍入规则；
- 保存 v1 时集中转换为整数毫秒，不允许各组件自行 `round/floor`；
- compiler 再从 snapshot 生成精确 FFmpeg 时间表达，并用 golden 验证首帧、末帧和总时长；
- v2 应评估用整数 tick + rational time base（或整数 frame index）作为权威时间，API 边缘才换算秒/毫秒。

最后一点是基于 30fps 与整数毫秒不可整除事实的工程建议；在 v2 定稿前须用既有 v1 fixture 做无漂移迁移验证。

### 5.4 命令、历史与服务端版本分层

- 一次用户 gesture 只提交一个语义 command；pointer move 只更新临时预览，pointer up 才进入 undo history 和 autosave。
- command 必须是 DOM/React 无关的纯数据操作，并统一经过 `normalize + validate`；这样同一 reducer 可用于 UI、单测、批量 `EditOperation` 和未来 Codex 操作。
- undo/redo 是当前会话的 command history；`TimelineVersion` 是服务端不可变业务快照，两者不能混为一套栈。OpenCut Classic 的 command manager 提供前者的组织参考，上位规格定义后者。[来源：OpenCut command manager](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/commands.ts)、[本仓库规格 §5](../21-video-material-editor-spec.md)
- 自动保存必须携带 `expected_version`，409 后停下自动覆盖并显示“加载服务端版本/另存新版本”；该要求直接对应“冲突不静默覆盖”的 MVP 验收。[来源：本仓库规格 §5、§7](../21-video-material-editor-spec.md)
- autosave completion 必须按请求序号或保存时 snapshot identity 判定，旧请求完成不得把更新后的本地状态标记为 clean。OpenCut Classic 的 save manager 用 `isSaving + hasPendingSave` 表达同类问题，可借鉴其状态机；cookies 仍必须叠加服务端 expected version。[来源：OpenCut Classic save manager](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/save-manager.ts)、[本仓库规格 §5、§7](../21-video-material-editor-spec.md)

## 6. 第一阶段建议交付切片

### Slice A：合同与隔离门禁

- 固定 OpenCut SHA、LICENSE、NOTICE、UPSTREAM、复制文件清单和依赖/SBOM 检查。
- 定义 `EditorClip`、`EditorCommand`、`OpenCutAdapter` 和单一 time normalization API。
- v1 schema/Go/TypeScript contract tests 保证往返无损；预先写 v2 概念 ADR，但不让 v2 假字段进入 UI。

### Slice B：真实单轨交互

- 素材预览与“加入时间线”互不干扰；支持按钮加入和卡片拖放。
- Clip 选中、素材换序、左右裁切、播放头分割、删除、吸附、缩放、撤销/重做、键盘操作。
- 所有操作通过 command kernel，统一保证主轨从 0 连续、无重叠，source range 不越过资产时长。

### Slice C：保存与恢复

- 允许先创建空 EditTask；首次有内容后生成 TimelineVersion。
- 自动保存与手动保存共用同一排队/去重机制；刷新恢复最新确认版本。
- expected version 冲突可见，不静默覆盖；旧 autosave 回包不能覆盖新编辑状态。

### Slice D：权威预览与正式导出

- TimelineVersion 编译为 renderer-neutral RenderPlan，再编译为 FFmpeg filtergraph。
- 低清预览与正式导出只改变 output profile/编码质量，不改变时间线语义。
- RenderJob 支持 durable enqueue、进度、取消、稳定失败原因、重试和缓存键。
- 成片探测通过后入库为新 AssetVersion，回流素材箱并记录完整 lineage。

### Slice E：验收与基线

- Unit：add/move/trim/split/delete/snap/normalize、命令逆操作、Adapter 往返。
- Contract：v1 schema、AssetVersion 权限/ready/source range、409、RenderJob 状态机。
- Integration：保存→刷新→预览 RenderJob→正式导出→资产回流。
- Golden：固定前贴和原视频，比较 ffprobe 时长/分辨率/帧率/音轨，以及裁切点抽帧；为后续字幕/字体/音轨 golden 保留同一 fixture manifest。ffprobe 官方支持 JSON 输出。[来源：ffprobe writers](https://ffmpeg.org/ffprobe.html#Writers)
- E2E/Visual：覆盖既有 12 步验收，并在 1366×768、1440×900、1920×1080 验证可滚动、无底部遮挡。[来源：本仓库实施方案 §7.4、§8](../plans/2026-08-07-video-material-editor-opencut-integration-plan.md)
- 性能：固定浏览器/设备/fixture，记录 30 秒 10/50/100 Clip 的首次渲染、拖动和缩放 p50/p95、峰值内存；数据连同 commit、build 与采样脚本入库。该基线用于满足规格第 7 项，而不是只写“体验流畅”。[来源：本仓库规格 §7](../21-video-material-editor-spec.md)

## 7. 第一阶段完成后的可见效果

唯一主验收链路应完整跑通：

1. 短剧前贴 V2 生成并入库，点击“进入素材剪辑”后前贴已在主时间线；也可直接进入剪辑器创建空任务。
2. 用户上传原视频，通过按钮或拖放加入主轨，能区分“预览素材”和“加入时间线”。
3. 用户可把前贴拖到原视频前，裁掉原视频开头 2 秒，在播放头分割并删除中间 3 秒，撤销再重做。
4. 即时预览按当前片段顺序和 source range 播放；吸附、缩放和播放头操作可用。
5. 保存后刷新恢复一致；并发冲突不静默覆盖。
6. 从冻结 TimelineVersion 生成可打开的低清预览，再异步导出 MP4/H.264/AAC、720×1280、9:16、30fps、48kHz 成片。
7. 成片自动回流素材箱，可追溯到前贴、原视频、TimelineVersion、RenderJob、FFmpeg build 与输出 profile。

以上与现有实施方案的第一阶段 12 步验收一致；缺任一步都不能把第一阶段标记为完成。[来源：本仓库实施方案 §8](../plans/2026-08-07-video-material-editor-opencut-integration-plan.md)

## 8. 外部条件与决策门禁

第一阶段开工前需要确认：

1. 项目负责人允许复制/修改 OpenCut MIT 代码并随产品保留必要声明；许可文本本身允许使用和修改，但保留声明是条件。[来源：OpenCut Classic LICENSE](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/LICENSE)
2. 固定可验收的前贴与 20–30 秒有权原视频，并允许测试产物保存在非生产 fixture/object storage。
3. 验收环境的 MySQL、对象存储、签名 URL 和 FFmpeg Worker 可用；需要记录 FFmpeg 完整版本、configure flags、编码器和字体包。
4. 产品确认首发输出仍为 MP4/H.264/AAC、720×1280、9:16、30fps、48kHz；若修改，必须先改 output profile 与 golden，不在命令行中散落常量。

后续 ASR Provider、品牌字体授权、音乐/音效商业授权、更多比例与高并发 Worker 容量不阻塞第一阶段，但其资源引用、权限和 renderer fingerprint 位置必须按本文预留。[来源：本仓库规格 §6、§7](../21-video-material-editor-spec.md)

## 9. 进入第二阶段前的硬门禁

只有同时满足以下条件，才能开始多轨、字幕和音频 UI：

- 第一阶段主验收 E2E 使用真实视频通过，并有可复现的 FFmpeg/ffprobe golden；
- 单轨所有 command 都从 React 组件抽离，Adapter 往返无损；
- v1 历史版本可继续读取/渲染，v2 schema 与 `v1 -> v2` migration 已评审；
- RenderPlan 已存在，FFmpeg 命令不是散落在业务 service 中的字符串拼接；
- 自动保存竞态、409、入队丢失、取消半成品和输出 lineage 已有自动化测试；
- OpenCut 固定 SHA、LICENSE、NOTICE、SBOM、性能基线、升级与退出策略均有仓库证据；
- `git diff --check`、前端测试与 build、相关 Go tests、Playwright 主验收和 CI 全部通过。[来源：仓库 `AGENTS.md`](../../AGENTS.md)、[本仓库实施方案 §12](../plans/2026-08-07-video-material-editor-opencut-integration-plan.md)

这组门禁的意义是：第二阶段只扩展轨道与渲染节点，不重新定义素材身份、保存模型、时间语义、渲染任务或许可证边界。
