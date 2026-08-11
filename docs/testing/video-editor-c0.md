# 素材剪辑 C0 验收基线

## 验收命令

Windows/Docker Desktop：

```powershell
./scripts/verify-video-editor-c0.ps1
```

Linux 或 CI：

```sh
docker build --target verifier -f deployments/render-worker/Dockerfile -t cookies-render-verifier:c0 .
docker run --rm --volume "$PWD:/workspace" --workdir /workspace cookies-render-verifier:c0 sh scripts/verify-video-editor-c0.sh
```

该命令会在忽略目录 `.tmp/video-editor-c0` 中生成固定素材，校验全部素材
SHA-256，然后执行以下两个公开边界测试：

1. 素材经 UploadService 入库后可以使用原 AssetVersion 打开预览，真实
   ffprobe 元数据为 720×1280、6 秒、H.264；
2. 两段视频、循环音乐和中文字幕组成的 v1 时间线分别渲染 preview 与
   export，二者的画面/音频 framemd5 摘要相同，并等于仓库固定 Golden。

fixture 合同位于
`internal/platform/media/testdata/video-editor-c0/manifest.json`，包括生成器
版本、权利声明、输出规格、十类素材哈希和 Golden 摘要。生成结果还会
写入 `render-toolchain.txt` 与 `SHA256SUMS`，用于问题复现。

## 固定样例

- 竖版前贴和竖版原视频；
- 横版原视频；
- 不透明及带透明通道图片；
- 配音、音乐和音效 WAV；
- 中文字幕 SRT；
- Noto Sans CJK 字体文件。

这些文件由固定镜像内的 FFmpeg/filter source 生成，不提交二进制文件，
不依赖网络下载的业务素材，也不包含权利不明的第三方视频或音乐。

## CI 门禁

`Platform CI` 直接构建固定摘要的 verifier 镜像并运行上述命令。设置
`COOKIES_REQUIRE_TIMELINE_GOLDEN=1` 后，FFmpeg、ffprobe 或任一 fixture
缺失都会失败，不允许以 skip 通过。
