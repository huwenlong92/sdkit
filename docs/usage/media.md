# Media 使用指南

`pkg/media/ffprobe` 用本地 ffprobe 读取媒体技术元数据。调用方先确保文件已经下载到本机，并为操作设置 timeout。

## 创建 Prober

使用 PATH 中的 `ffprobe`：

```go
prober, err := ffprobe.New(ctx, ffprobe.Config{})
if err != nil {
    return err
}
```

生产环境建议显式设置二进制路径、允许版本和输出上限：

```go
prober, err := ffprobe.New(ctx, ffprobe.Config{
    BinaryPath:  "/opt/ffmpeg/bin/ffprobe",
    MinVersion:  "7.0",
    MaxVersion:  "8.1",
    OutputLimit: 8 << 20,
})
```

本包不自动下载或升级 ffprobe。二进制缺失返回 `media.ErrBinaryUnavailable`，版本不兼容返回 `media.ErrVersionUnsupported`。

## 探测本地文件

```go
ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
defer cancel()

info, err := prober.Probe(ctx, media.Input{
    Path: "/var/lib/myapp/downloads/episode.mp4",
}, media.ProbeOptions{})
if err != nil {
    return err
}

for _, video := range info.VideoStreams {
    fmt.Printf("video #%d: %s %dx%d %.3f fps\n",
        video.Index,
        video.Codec,
        video.Width,
        video.Height,
        video.FrameRate,
    )
}
```

首版只接受本地普通文件。`http://`、`https://` 等远程输入返回 `media.ErrInvalidInput`；路径不存在返回 `media.ErrInputUnavailable`。

## 读取结果

容器字段：

- `Container` / `ContainerLong`
- `Duration`
- `SizeBytes`
- `BitRate`

轨道分别位于：

- `VideoStreams`
- `AudioStreams`
- `SubtitleStreams`

stream 的 `Index` 是 ffprobe 原始轨道索引，可用于后续转码命令定位轨道。`FrameRateRatio` 保留有效的有理数文本，例如 `24000/1001`；`FrameRate` 提供便于比较和展示的浮点值。

纯音频文件的 `VideoStreams` 为空是正常结果。业务逻辑应根据实际 stream 列表判断类型，不要把“无视频轨”当成探测失败。

## 每次调用覆盖输出上限

```go
info, err := prober.Probe(ctx, input, media.ProbeOptions{
    OutputLimit: 2 << 20,
})
```

调用级 `OutputLimit` 大于 0 时覆盖构造配置。超出上限返回 `media.ErrOutputLimit`。

## 错误处理

```go
var probeErr *media.Error
switch {
case errors.Is(err, context.DeadlineExceeded):
    // ffprobe 超时并已停止。
case errors.Is(err, media.ErrBinaryUnavailable):
    // 部署缺少 ffprobe。
case errors.Is(err, media.ErrOutputInvalid):
    // ffprobe 输出与当前解析契约不兼容。
case errors.As(err, &probeErr):
    // 使用 Operation 和已脱敏 Summary 诊断。
}
```

driver 错误不会把本地媒体完整路径或底层命令参数写入摘要。业务日志仍应避免自行打印带隐私信息的输入路径。

## 可选真实集成测试

默认测试使用 fake runner，不要求本机安装 ffprobe。真实文件测试需要显式开启：

```bash
SDKIT_FFPROBE_INTEGRATION=1 \
SDKIT_FFPROBE_INPUT=/absolute/path/to/sample.mp4 \
go test ./tests/pkg/media/ffprobe -run TestFFProbeIntegration
```

未安装 ffprobe 或未提供输入文件时，该集成测试只会跳过。
