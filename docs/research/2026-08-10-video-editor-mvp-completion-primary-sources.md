# 素材剪辑从当前阶段到完整 MVP 的一手资料约束（2026-08-10）

## 1. 文档用途与调研边界

本文为 [`21-video-material-editor-spec.md`](../21-video-material-editor-spec.md) 第 7 节七项 MVP 验收提供外部一手资料依据。它不是新的产品规格，也不重复 [`2026-08-10-video-editor-phase1-primary-research.md`](2026-08-10-video-editor-phase1-primary-research.md) 已经确认的第一阶段结论；本文只补齐从当前单主轨编辑能力继续扩展到多轨、字幕、音频、多比例、权威预览/导出和最终验收时必须遵守的技术约束。

资料范围严格限定为：

- FFmpeg 官方文档、官方源码文档与官方许可说明；
- OpenCut 官方 GitHub 仓库、固定 Classic 提交及其 MIT License；
- WHATWG HTML Living Standard、W3C File API、W3C WebVTT；
- Mozilla MDN 仅用于补充浏览器兼容性或易误用行为说明，不作为 cookies 业务决策来源。

核心结论：

1. 后续开发不能为字幕、音频和叠加轨另起一条渲染链。低清预览和正式导出必须由同一 `TimelineVersion -> RenderPlan -> FFmpeg filtergraph` 编译器生成，只允许输出 profile 不同。
2. 浏览器 `<video>` 是即时交互反馈，不是逐帧验收依据；固定样例一致性必须由服务端产物、`ffprobe` 和解码后的 `framemd5` 证明。
3. 字幕一致性不仅是文字和时间一致，还依赖 libass、字体文件、字体 fallback、HarfBuzz/Fontconfig 等 FFmpeg build 能力。因此字体包及 FFmpeg build 必须进入 renderer fingerprint。
4. 音频混合必须显式固定各轨时间戳、采样率、声道、增益、fade、混合结束策略和归一化策略，不能依赖 FFmpeg 默认值。
5. OpenCut Classic 只能作为固定 SHA 下的算法来源。它已被官方归档且不再维护；cookies 必须保持“可替换 adapter”，不得把 OpenCut 存储、项目、导出或运行时模型变成业务事实源。

## 2. 与既有调研的关系

既有第一阶段调研已经确认以下事实，本文直接继承、不再重新论证：

- cookies 自有 `AssetVersionRef`、`TimelineVersion`、`RenderJob` 是唯一业务事实源；
- OpenCut Classic 固定提交为 `cf5e79e919144200294fb9fed22a222592a0aeea`；
- 用户上传素材的确定性裁切/拼接主路径使用 filtergraph 重编码，而不是 concat demuxer + stream copy；
- 每个片段先 `trim/atrim`，再 `setpts/asetpts` 归零，规格归一后进入 concat；
- 浏览器即时预览不替代服务端低清预览和正式导出；
- 保存采用 expected-version 与 latest-wins 协调，RenderJob 冻结不可变时间线版本。

本文新增的内容是：

- 叠加画面、三种画布比例、字幕字体、配音/音乐/音效的 FFmpeg 编译约束；
- 浏览器拖放、上传预览、媒体 seek、逐帧回调和画面完整显示的规范边界；
- 完整 MVP 第 4 项固定样例对比的可执行证据组合；
- OpenCut 复用的源码级清单、许可和退出条件；
- 从七项验收反推的外部条件与不可省略的自动化门禁。

## 3. FFmpeg 确定性渲染约束

### 3.1 Filtergraph 是唯一权威编译目标

FFmpeg 官方将复杂 filtergraph 定义为可具有多个输入和多个输出的图，并用具名 pad 表达分支、组合与映射；官方示例通过 `split -> crop/vflip -> overlay` 展示多支路合成。这正好覆盖 cookies 后续主视频、叠加视频/图片、字幕和多音轨的组合模型。[来源：FFmpeg Filters Documentation 的 filtering introduction](https://ffmpeg.org/ffmpeg-filters.html#Filtering-Introduction)、[Complex filtergraphs](https://ffmpeg.org/ffmpeg.html#Complex-filtergraphs)

因此实施方案应只保留一个 renderer-neutral 编译入口：

```text
TimelineVersion
  -> Timeline decoder / validator
  -> RenderPlan（类型化节点，不含 shell 字符串）
  -> FFmpeg compiler
  -> filter_complex + explicit maps + output options
```

`RenderPlan` 至少需要表达：

- 固定输入 `AssetVersionRef` 与解析后的受控本地路径/URL；
- 每个 clip 的 source range、timeline range、track role 和 z-order；
- 画布尺寸、帧率、像素格式、背景和 visual transform；
- 字幕 cue、style ref 和字体资源版本；
- 原声、配音、音乐、音效的 gain、mute、fade 和混合策略；
- 输出 profile、编码器和 renderer fingerprint。

FFmpeg 命令字符串只能在最外层 adapter 中产生。用户文本、文件名、字体名和任意 Timeline 字段不能直接拼接进 shell；compiler 应构造参数数组并对 filter 参数做专用 escaping。

### 3.2 裁切、拼接与时间戳

FFmpeg `trim`/`atrim` 只保留输入的连续子区间，但官方文档明确说明 trim 不会修改时间戳；要让每段从零开始，需要继续使用 `setpts`/`asetpts`。concat filter 又要求各段从时间戳 0 开始。[来源：FFmpeg `trim`](https://ffmpeg.org/ffmpeg-filters.html#trim)、[`atrim`](https://ffmpeg.org/ffmpeg-filters.html#atrim)、[`setpts, asetpts`](https://ffmpeg.org/ffmpeg-filters.html#setpts_002c-asetpts)、[`concat`](https://ffmpeg.org/ffmpeg-filters.html#concat)

权威主轨片段的最小编译模板应为：

```text
video_i:
  trim(source_in, source_out)
  -> setpts(PTS-STARTPTS)
  -> canvas normalize
  -> fps(output_fps)
  -> format(output_pixel_format)

audio_i:
  atrim(source_in, source_out)
  -> asetpts(PTS-STARTPTS)
  -> aresample(output_sample_rate)
  -> aformat(output_sample_format, channel_layout)

all normalized segment pairs
  -> concat(n=N, v=1, a=1)
```

concat filter 会选择每段除最后流之外最长流的时长，并在较短音轨末尾补静音；不同帧率还可能产生可变帧率输出。cookies 不能把这些默认行为当作产品语义，必须在 RenderPlan 中明确“缺音轨如何补静音、片段尾部如何截齐、输出是否强制 30fps”。[来源：FFmpeg `concat` filter](https://ffmpeg.org/ffmpeg-filters.html#concat)、[FFmpeg `fps` filter](https://ffmpeg.org/ffmpeg-filters.html#fps)

### 3.3 三种画布比例与完整画面

规格要求首期覆盖 9:16、16:9、1:1。FFmpeg 官方提供 `scale`、`pad`、`crop` 和 `overlay`，但这些只是机制；产品必须为每个 clip 固化 `fit_mode`：

- `contain`：保持宽高比缩放后 pad，完整画面可见；
- `cover`：保持宽高比放大后 crop，可填满画布但允许裁切；
- `free`：显式 scale/position/crop，由用户可见控制。

官方 `scale` 支持 `force_original_aspect_ratio`，`pad` 用于填充输出画布，`crop` 用于裁切，`overlay` 用于将一支视频覆盖到另一支。compiler 应根据结构化 transform 生成固定图，而不是让 UI 直接提交 FFmpeg 表达式。[来源：FFmpeg `scale`](https://ffmpeg.org/ffmpeg-filters.html#scale)、[`pad`](https://ffmpeg.org/ffmpeg-filters.html#pad)、[`crop`](https://ffmpeg.org/ffmpeg-filters.html#crop)、[`overlay`](https://ffmpeg.org/ffmpeg-filters.html#overlay)

浏览器端也有明确默认：在没有相反 CSS 的情况下，`<video>` 内容应居中，以保持宽高比的最大完整尺寸显示；比例不同应 letterbox/pillarbox。若预览只出现半张脸，那是实现选择了 cover/crop 或容器裁剪，并非 `<video>` 必然行为。[来源：WHATWG HTML video rendering](https://html.spec.whatwg.org/multipage/media.html#the-video-element)

这意味着 MVP 默认策略应是：

- 素材预览默认 `contain`，任何裁切都必须是用户显式编辑结果；
- 输出 profile 可定义背景色或模糊背景，但不能悄悄切换为 cover；
- 浏览器预览与 FFmpeg 使用同一个结构化 `fit_mode + transform`；
- 9:16、16:9、1:1 各自必须有可视化与 golden fixture。

### 3.4 三条画面轨与叠加

多画面轨不应通过“先导出一层，再把中间产物重新导入”实现。FFmpeg filtergraph 可以并行解码和规范化多输入，再通过 `overlay` 逐层合成。[来源：FFmpeg filtering introduction](https://ffmpeg.org/ffmpeg-filters.html#Filtering-Introduction)、[`overlay`](https://ffmpeg.org/ffmpeg-filters.html#overlay)

RenderPlan 需要把每个 visual clip 编译为：

```text
source trim
  -> timestamps rebased to clip-local zero
  -> scale/crop
  -> setpts(PTS + timeline_start/TB)
  -> overlay on previous composition by z-order
```

工程约束：

- Track ID 是稳定身份，UI 行号不是身份；
- z-order、timeline interval、position/scale/crop/opacity 必须结构化保存；
- overlay 的 `eof_action`、`shortest`、是否重复末帧必须由 compiler 显式固定，不能依赖默认值；
- 不在画面轨存字幕字符串或音量，不把不同媒体类型塞入一个无类型 payload。

### 3.5 字幕、字体与样式一致性

WebVTT 是浏览器外部文本轨格式，cue 具有时间区间和文本，适合导入、校对和浏览器辅助预览；W3C 同时指出 WebVTT 的 CSS 只允许有限集合，不同实现需要产生等价渲染。[来源：W3C WebVTT introduction](https://www.w3.org/TR/webvtt1/#introduction)、[WebVTT rendering conformance](https://www.w3.org/TR/webvtt1/#rendering)

MVP 验收要求“字幕、字体”在低清预览和正式导出间一致，不能把浏览器原生 `<track>` 样式当成最终权威。建议：

- Timeline 保存结构化 cue：`id/start/end/text/style_ref`；
- WebVTT/SRT 只作为导入导出格式，不作为业务事实源；
- renderer 将 cue 和 style 编译成受控 ASS（或等价受控中间格式）；
- 低清预览和正式导出使用同一个字幕中间文件与同一字体包；
- 浏览器即时预览由同一 cue/style model 绘制，但验收以服务端渲染产物为准。

FFmpeg 官方 `subtitles`/`ass` filter 使用 libass；`fontsdir` 可指定额外字体目录，`original_size` 影响字体按原始画面缩放，复杂文字 shaping 依赖 HarfBuzz 能力。官方源码文档也明确把它称为“libass subtitles burning filter”。[来源：FFmpeg `subtitles`](https://ffmpeg.org/ffmpeg-filters.html#subtitles-1)、[`ass`](https://ffmpeg.org/ffmpeg-filters.html#ass)、[FFmpeg `vf_subtitles.c`](https://ffmpeg.org/doxygen/trunk/vf__subtitles_8c.html)

因此 renderer fingerprint 必须至少包含：

- FFmpeg 完整版本与 configure flags；
- libass 版本；
- HarfBuzz/Fontconfig 是否启用及版本；
- 品牌字体 `AssetVersionRef`、文件哈希和字体 fallback 清单；
- ASS compiler 版本和字幕 style schema 版本。

只保存 `font-family: 某某字体` 不足以验收：Worker 上没有同一字体文件时会 fallback，画面看似有字幕但字形、换行、宽度和位置会改变。

### 3.6 原声、配音、音乐与音效

FFmpeg 官方 `amix` 支持多路音频混合，并允许定义输出时长、各输入权重、结束时归一化过渡和是否归一化。`amix` 只处理浮点样本，整数输入可能自动插入 `aresample`。[来源：FFmpeg `amix`](https://ffmpeg.org/ffmpeg-filters.html#amix)

为了避免不同机器/版本下的隐式转换改变结果，各轨进入 mix 前应显式编译：

```text
atrim(source range)
-> asetpts(PTS-STARTPTS)
-> adelay(timeline_start)
-> volume(gain)
-> afade(in/out, if configured)
-> aresample(48000)
-> aformat(sample format, stereo)
-> amix(inputs=N, duration=explicit, weights=explicit, normalize=explicit)
```

官方 `aresample` 可在采样率转换之外，根据时间戳伸缩、注入静音或裁掉音频；`afade` 表达淡入淡出；`volume` 负责增益。[来源：FFmpeg `aresample`](https://ffmpeg.org/ffmpeg-filters.html#aresample)、[`afade`](https://ffmpeg.org/ffmpeg-filters.html#afade)、[`volume`](https://ffmpeg.org/ffmpeg-filters.html#volume)

RenderPlan 必须显式定义：

- source audio、voiceover、music、sfx 的角色与默认 gain；
- clip-local source range 与 timeline start；
- mute、fade-in、fade-out；
- 输出采样率和声道布局；
- mix 采用 `longest`、`shortest` 还是 `first`；
- `normalize` 与 limiter/loudness policy；
- 无音频素材的静音补齐策略。

若这些字段没有进入不可变 TimelineVersion，同一画面时间线在以后增加音轨后无法可靠恢复，也无法满足固定样例对比。

### 3.7 低清预览和正式导出必须共享语义

低清预览与正式导出允许不同的参数只有：

- 输出分辨率/码率/编码 preset；
- 是否带可识别的 preview 水印；
- 产物保留周期和缓存策略。

以下内容必须完全相同：

- TimelineVersion 与输入 AssetVersion；
- clip 顺序、时间范围、transform、轨道 z-order；
- 字幕 cue、style、字体文件与 fallback；
- 音轨 timing、gain、fade、mix 策略；
- compiler 语义版本。

因此同一份 RenderPlan 应在最终一步应用 `preview_profile` 或 `export_profile`，禁止维护两套独立命令模板。

### 3.8 输出探测与固定样例证据

`ffprobe` 官方定位是以人类和机器可读形式输出容器及媒体流信息，并提供 JSON writer。它适合验证 codec、分辨率、帧率、采样率、声道和时长，但它不能证明逐帧画面或逐样本音频相同。[来源：ffprobe description](https://ffmpeg.org/ffprobe.html#Description)、[ffprobe writers](https://ffmpeg.org/ffprobe.html#Writers)

FFmpeg `framemd5` 是逐包 MD5 测试格式，对解码后的音视频帧/包产生校验记录；官方 `hash` muxer也说明原始音视频哈希可以进行内容相等检查，而不必比较整个容器二进制。[来源：FFmpeg `framemd5`](https://ffmpeg.org/ffmpeg-formats.html#framemd5)、[`hash`](https://ffmpeg.org/ffmpeg-formats.html#hash)

MVP 第 4 项的固定样例证据应分三层：

1. `ffprobe -of json`：验证 MP4、H.264、AAC、目标宽高、30fps、48kHz、音视频流数量和总时长容差；
2. 时间点抽帧/波形断言：验证裁切点、片段顺序、叠加出现区间、字幕出现区间、淡入淡出区间；
3. `framemd5`：在固定 FFmpeg build、固定字体包、固定输入 fixture 下比较低清基准和正式导出经过同规格归一后的解码内容。

不能直接比较 MP4 文件 MD5：容器 metadata、编码 preset 和码率不同会产生不同字节，即使时间线语义相同。也不能只比较 `ffprobe`：codec/时长一致不代表画面、字幕和音轨内容一致。

为使 golden 可复现，fixture manifest 必须固定：

- 输入文件哈希及 Asset ID/Version；
- TimelineVersion JSON 与 content hash；
- output profile；
- FFmpeg build/version/configure flags；
- 字体文件哈希；
- compiler 版本；
- 期望 ffprobe JSON 子集、抽帧断言和 framemd5 基准。

FFmpeg format option `bitexact` 用于只写与平台、build 和时间无关的数据，主要用途是回归测试；可在可用位置启用，但它不能替代固定编译环境和显式 profile。[来源：FFmpeg format options `bitexact`](https://ffmpeg.org/ffmpeg-formats.html#Format-Options)

## 4. 浏览器端媒体与拖放约束

### 4.1 素材卡拖入时间线

WHATWG 对 HTML Drag and Drop 的数据存储模式有明确限制：`dragstart` 时可读写，`drop` 时只读，其余 DnD 事件处于 protected mode，只能枚举类型、不能读取实际数据。[来源：WHATWG drag data store](https://html.spec.whatwg.org/multipage/dnd.html#the-drag-data-store)

因此实现应：

- 在素材卡 `dragstart` 写入单一自定义 MIME，例如 `application/x-cookies-asset-ref`；
- payload 只含当前 Project 内可再次校验的 asset/version 标识，不传签名 URL、数据库对象或完整 Timeline；
- `dragover` 只判断类型、显示插入位置并阻止默认拒绝行为；
- `drop` 才读取 payload，再调用与“加入时间线”按钮相同的 command；
- 不在 `drag`/`dragenter` 中读取 payload；
- pointer 拖动 clip 与 HTML DnD 导入素材分成两个 session，避免事件模型冲突。

内部拖放完成后仍须由服务端校验 Project scope 和 AssetVersion 状态，拖放 payload 不是授权凭证。

### 4.2 上传选择只是提示，不是验证

HTML 标准明确说明 file input 的 `accept` 只是给 User Agent 的接受类型提示，例如 `video/*`；它不构成可信媒体探测或安全验证。[来源：WHATWG File Upload state and `accept`](https://html.spec.whatwg.org/multipage/input.html#file-upload-state-(type=file))、[MDN `accept`](https://developer.mozilla.org/en-US/docs/Web/HTML/Attributes/accept)

因此上传链路必须在服务端继续执行：

- 文件大小、MIME sniff/probe、容器和 codec 白名单；
- ffprobe 时长/流数量/尺寸限制；
- 解码失败、压缩炸弹和超长媒体防护；
- 入库前扫描与状态转换；
- Project scope、rights state 与 AssetVersion 固化。

本地预览可用 `URL.createObjectURL(file)`，但 W3C File API 指出 Blob URL 会持续存在直至撤销或文档卸载，并建议成对调用 `URL.revokeObjectURL()` 以降低泄漏风险。[来源：W3C File API Blob URL lifetime](https://www.w3.org/TR/FileAPI/#lifeTime)、[creating and revoking Blob URLs](https://www.w3.org/TR/FileAPI/#creating-revoking)

### 4.3 播放头和逐帧反馈

HTML 标准规定 `currentTime` 以秒表示；设置它会触发 seek，但 getter 返回的是在脚本执行期间保持稳定的“official playback position”，不是一个可以作为帧整数事实源的值。[来源：WHATWG media timeline and `currentTime`](https://html.spec.whatwg.org/multipage/media.html#offsets-into-the-media-resource)

`timeupdate` 也不是逐帧事件。对于需要知道实际送往合成器帧的场景，`requestVideoFrameCallback()` 提供 `mediaTime`、呈现帧计数和预计显示时间；MDN 记录其 Baseline 2024，但较旧浏览器仍需要 fallback。[来源：MDN `requestVideoFrameCallback`](https://developer.mozilla.org/en-US/docs/Web/API/HTMLVideoElement/requestVideoFrameCallback)

工程约束：

- 编辑器权威时间使用整数 frame/tick，不使用 `<video>.currentTime` 反向覆盖 Timeline；
- seek 前将 UI 时间归一到 master frame，seek 完成后读取 presented-frame metadata 更新画面反馈；
- `requestVideoFrameCallback` 仅用于同步 UI，不改变服务端时间线；
- golden 不依赖浏览器 `currentTime` 或截图单独证明裁切帧正确。

### 4.4 跨域预览与 Canvas

HTML 标准的 canvas 安全模型规定：绘制非 origin-clean 的跨域媒体会把 canvas 标记为非 origin-clean，后续 `getImageData`、`toBlob`、`toDataURL` 会抛出 `SecurityError`。[来源：WHATWG canvas security](https://html.spec.whatwg.org/multipage/canvas.html#security-with-canvas-elements)

如果编辑器需要抽帧缩略图、波形封面或画布叠加预览，签名媒体 URL 必须返回正确 CORS 头，并为 `<video>` 设置匹配的 `crossorigin`。即便浏览器能直接播放跨域视频，也不代表能合法读取其像素。

## 5. OpenCut 固定版本、来源与退出边界

### 5.1 官方现状

OpenCut 主仓库官方声明正在从头重写，并明确建议当前使用 Classic；Classic 官方仓库又已于 2026-05-17 归档且只读，README 明确说明它不再维护。[来源：OpenCut 主仓库 Status](https://github.com/OpenCut-app/OpenCut#status)、[OpenCut Classic 官方仓库](https://github.com/OpenCut-app/opencut-classic)

所以 cookies 不应：

- 跟随浮动 `main`；
- 安装完整 OpenCut 应用作为运行时依赖；
- 把 OpenCut project/element JSON 直接持久化；
- 复用 OpenCut 数据库、登录、对象存储、导出或项目模型；
- 等待 OpenCut rewrite 后再开始 cookies MVP。

### 5.2 唯一允许的上游快照

唯一批准来源为 Classic commit [`cf5e79e919144200294fb9fed22a222592a0aeea`](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea)。其 MIT License 允许使用、复制、修改、分发和商业使用，但要求在软件副本或实质部分中保留版权与许可声明。[来源：固定提交 LICENSE](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/LICENSE)

当前及后续可选择性借鉴的源码位置：

| 能力 | 固定提交官方源码 | cookies 采用边界 |
| --- | --- | --- |
| 时间线类型分层 | [`timeline/types.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/types.ts) | 只借鉴 track/element/source trim 分离，不持久化 OpenCut 类型 |
| 移动 command | [`move-elements.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/timeline/element/move-elements.ts) | 改写为 cookies 纯 command，输入输出均为 cookies document |
| 分割 command | [`split-elements.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/timeline/element/split-elements.ts) | 借鉴 source/timeline range 拆分不变量 |
| 撤销/重做 | [`commands.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/commands.ts) | 仅会话 command history，不替代服务端 TimelineVersion |
| 裁切 | [`resize-controller.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/controllers/resize-controller.ts) | pointer session 与最终 semantic command 分离 |
| 缩放 | [`zoom-controller.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/controllers/zoom-controller.ts) | 只复用坐标/缩放算法 |
| 吸附 | [`timeline/snapping`](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/snapping) | 借鉴候选点与阈值，最终结果仍按 cookies master frame 归一 |
| 保存协调 | [`save-manager.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/save-manager.ts) | 只借鉴 `isSaving + hasPendingSave`，持久化仍用 cookies expected-version API |
| 时间类型 | [`media-time.ts`](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/wasm/media-time.ts) | 借鉴集中舍入，不引入 OpenCut WASM runtime |

任何新增复制/改写都必须同步更新 `NOTICE.md`、SBOM、来源文件清单和测试基线。OpenCut adapter 的退出标准是：删除/替换适配代码不需要迁移 cookies 数据库或 TimelineVersion；若删除 OpenCut 代码需要迁移持久化数据，说明隔离边界已经失效。

## 6. FFmpeg 版本与许可门禁

FFmpeg 官方许可说明指出：基础 FFmpeg 为 LGPL 2.1+，启用特定可选 GPL 部分后整个 FFmpeg build 适用 GPL；启用 `--enable-nonfree` 会影响可分发性。官方清单还要求准确记录源代码、修改和 configure 信息。[来源：FFmpeg License and Legal Considerations](https://ffmpeg.org/legal.html)、[FFmpeg source License](https://ffmpeg.org/doxygen/trunk/md_LICENSE.html)

因此 Worker 镜像/二进制进入 MVP 环境前必须归档：

- FFmpeg 完整版本和源码来源；
- 完整 configure flags；
- 启用的外部 codec/filter 库及其版本；
- LGPL/GPL/nonfree 判定；
- 对应源码与修改获取位置；
- Worker 镜像 digest；
- H.264/AAC 编码器实际选择及其许可审查。

技术方案不能只写“使用 FFmpeg”。特别是字幕需要 libass，字体 fallback/复杂脚本又可能依赖 Fontconfig/HarfBuzz；编码器选择也会改变许可证和 renderer fingerprint。

## 7. 对规格第 7 节七项验收的逐项约束

| MVP 验收 | 一手资料带来的硬约束 | 必须形成的仓库证据 |
| --- | --- | --- |
| 1. 独立/广告任务入口 | HTML 拖放与 File Upload 只提供交互机制，最终导入必须统一进入 cookies command/API | 入口 E2E；前贴携带稳定 AssetVersion；独立空任务 E2E；上传、预览、加入、拖入动作分离 |
| 2. 稳定 Asset ID/Version、原片不覆盖 | OpenCut `mediaId` 只能是 adapter 会话字段，不能进入权威持久化 | schema/contract test；所有 clip 固定 asset/version；输出新建 AssetVersion；输入对象哈希不变 |
| 3. 刷新/换设备恢复、冲突不静默覆盖 | OpenCut save manager 只可借鉴 pending-save 状态；服务端必须继续 expected-version/409 | 并发保存集成测试；旧回包不误标 clean；刷新和第二浏览器上下文恢复 E2E |
| 4. 预览与导出固定样例一致 | FFmpeg trim 不改时间戳、concat 要求归零；字幕依赖 libass/字体；音频 mix 默认必须显式化；ffprobe 不能代替 framemd5 | 同一 RenderPlan 双 profile；ffprobe JSON；抽帧/音频断言；framemd5；字体/FFmpeg fingerprint |
| 5. RenderJob 生命周期 | FFmpeg `-progress` 可输出机器可读进度，但队列、取消、重试和幂等是 cookies 责任 | queued/running/terminal 状态测试；入队失败终态；取消清理；重试 lineage；缓存键测试；轮询超时走查 |
| 6. 无权/过期/范围不符不可用 | `accept=video/*`、DnD payload、可播放 URL 都不是授权证明；跨域播放与可读像素也不同 | Project scope/rights/expiry/channel/purpose policy；preview/render 双重校验；跨 Project 与过期 URL 安全测试 |
| 7. OpenCut 治理 | 官方 rewrite、Classic 归档，必须固定 SHA；MIT 要保留许可；adapter 必须可删除 | LICENSE、NOTICE、SBOM、固定 SHA、来源清单、性能基线、升级/退出演练与 CI 门禁 |

## 8. 技术方案必须显式列出的外部条件

以下条件不能靠代码推断，详细实施计划必须标记负责人和到位日期：

### 8.1 媒体与渲染环境

- 一组有权使用、可提交哈希的固定 fixture：前贴、横屏原视频、竖屏原视频、含/不含原声视频、配音、音乐、音效；
- 可运行的 FFmpeg/ffprobe Worker，具备所需 filters/encoders；
- 固定 FFmpeg build、镜像 digest、configure flags 与许可审查结果；
- 可读写的测试对象存储和签名 URL；
- 异步队列/Worker、取消进程和临时产物清理能力。

### 8.2 字幕与字体

- ASR Provider 不是人工字幕编辑的前置条件；没有 ASR 时仍应支持手工创建/校对 cue；
- 至少一套已授权、可随 Worker 使用的中文品牌字体文件；
- 字体版本、哈希、fallback 和缺字策略；
- libass/HarfBuzz/Fontconfig 能力确认；
- 固定字幕样例，覆盖中文换行、标点、数字/英文混排和安全区。

### 8.3 音频

- 配音、音乐和音效测试素材的使用授权；
- 默认 gain、淡入淡出、ducking 是否进入 MVP 的产品决策；
- 响度/峰值标准与渠道规则；若暂未确定，也必须固定第一版显式默认值，不能依赖 `amix` 默认。

### 8.4 权利与授权模型

- rights state 枚举；
- 有效期、地域、渠道、用途和 Project scope 字段；
- `unknown/unverified` 在素材预览、时间线编辑、低清预览、正式导出的放行/阻断规则；
- 用户上传权利确认和审计留痕。

没有这组业务决策，规格第 6 项无法完成最终验收；代码最多只能先建立统一 `AssetUsePolicy` seam。

## 9. 自动化验收的最低来源驱动门禁

### 9.1 Unit / property

- master frame/tick 换算、重复 trim/split 无漂移；
- 多轨 z-order、重叠区间和 clip ID 唯一；
- 字幕 cue/style 与字体 ref 往返；
- 音频 gain/fade/delay/mix plan；
- OpenCut adapter 输入输出不泄漏 OpenCut 持久化类型。

### 9.2 Contract / security

- v1 历史快照持续可读；v2 schema 严格校验；
- `v1 -> v2` 迁移无损且幂等；
- AssetVersion ready/scope/rights/expiry/source-range；
- conflict 409、稳定 error code、RenderJob 冻结 snapshot；
- 上传 MIME 声明与 ffprobe 结果不一致时拒绝。

### 9.3 Integration

- 同一 TimelineVersion 分别创建低清 preview 与正式 export，编译出的语义节点一致；
- 字幕中间文件与字体包在两种任务中相同；
- 音频轨顺序、delay、gain、fade 和 mix 参数相同；
- 成片入库后记录全部输入 AssetVersion、TimelineVersion、RenderJob、compiler/renderer fingerprint。

### 9.4 Golden

- 9:16 contain：证明完整画面未被默认裁掉；
- 16:9 与 1:1：证明 fit/position/crop 一致；
- 三画面轨：证明叠加起止时间与 z-order；
- 中文字幕：证明文本、时间、字体、换行、安全区；
- 原声 + 配音 + 音乐 + 音效：证明对齐、fade、增益和总时长；
- 每个样例同时保存 ffprobe 子集、关键时间点断言和 framemd5。

### 9.5 Browser E2E / visual

- 点击素材只预览，不改时间线；按钮加入和拖入均通过同一 command；
- dragstart/drop 自定义 MIME 可用，拒绝非法 payload；
- 画布默认 contain，横竖视频不显示为“只有半张脸”；
- seek/split 落在 master frame；
- Blob URL 在素材切换/组件卸载后撤销；
- 签名 URL/CORS 足以播放并在需要时抽帧；
- 1366×768、1440×900、1920×1080、窄屏均可滚动到全部操作区。

## 10. 供后续分阶段技术方案采用的决策摘要

1. 先冻结 `editing-timeline/v2`、`RenderPlan` 和 `v1 -> v2`，再开发多轨 UI；否则字幕/音频/transform 会被迫塞入 v1 或 React 临时状态。
2. 多比例、多轨、字幕、音频全部扩展同一个 compiler；禁止阶段性脚本或第二套 preview renderer。
3. 浏览器即时预览追求交互速度，服务端 preview/export 才是权威证据；二者共享 document 和 compiler 语义，不共享“浏览器实际解码帧”作为事实源。
4. 字体是版本化渲染资源，不是一个 CSS 名称；音频默认值是产品/编译器契约，不是 FFmpeg 默认。
5. Golden 使用 `ffprobe + 关键时间点断言 + framemd5`，三者缺一不能覆盖规格第 4 项。
6. OpenCut 继续保持固定 SHA、选择性改写、MIT 归档和可删除 adapter；不得因后续多轨开发扩大为完整应用依赖。
7. 最终 MVP 不能只在开发机通过：FFmpeg build、字体包、fixture、对象存储、Worker、授权规则和 CI required checks 都是交付条件。

## 11. 一手资料索引

### FFmpeg

- [FFmpeg Filters Documentation](https://ffmpeg.org/ffmpeg-filters.html)
- [FFmpeg complex filtergraphs](https://ffmpeg.org/ffmpeg.html#Complex-filtergraphs)
- [FFmpeg concat filter](https://ffmpeg.org/ffmpeg-filters.html#concat)
- [FFmpeg trim / atrim](https://ffmpeg.org/ffmpeg-filters.html#trim)
- [FFmpeg setpts / asetpts](https://ffmpeg.org/ffmpeg-filters.html#setpts_002c-asetpts)
- [FFmpeg scale / pad / crop / overlay](https://ffmpeg.org/ffmpeg-filters.html#scale)
- [FFmpeg subtitles / ass](https://ffmpeg.org/ffmpeg-filters.html#subtitles-1)
- [FFmpeg amix / aresample / afade / volume](https://ffmpeg.org/ffmpeg-filters.html#amix)
- [ffprobe Documentation](https://ffmpeg.org/ffprobe.html)
- [FFmpeg framemd5](https://ffmpeg.org/ffmpeg-formats.html#framemd5)
- [FFmpeg License and Legal Considerations](https://ffmpeg.org/legal.html)

### OpenCut

- [OpenCut 主仓库 Status](https://github.com/OpenCut-app/OpenCut#status)
- [OpenCut Classic 官方仓库](https://github.com/OpenCut-app/opencut-classic)
- [OpenCut Classic 固定提交](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea)
- [固定提交 MIT License](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/LICENSE)

### Web 标准

- [WHATWG HTML Drag and Drop](https://html.spec.whatwg.org/multipage/dnd.html)
- [WHATWG media elements](https://html.spec.whatwg.org/multipage/media.html)
- [WHATWG file upload input](https://html.spec.whatwg.org/multipage/input.html#file-upload-state-(type=file))
- [WHATWG canvas security](https://html.spec.whatwg.org/multipage/canvas.html#security-with-canvas-elements)
- [W3C File API](https://www.w3.org/TR/FileAPI/)
- [W3C WebVTT](https://www.w3.org/TR/webvtt1/)
- [MDN requestVideoFrameCallback](https://developer.mozilla.org/en-US/docs/Web/API/HTMLVideoElement/requestVideoFrameCallback)
