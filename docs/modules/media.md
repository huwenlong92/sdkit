# Media 模块设计

`pkg/media` 定义本地媒体元数据探测契约，`pkg/media/ffprobe` 提供首个实现。

Media 是独立可选公共能力，不依赖 `remotefs`、storage、core 或 runtime。典型消费流程由业务侧编排：先通过任意来源下载到本地文件，再调用 `media.Prober` 探测。

## 包结构

```text
pkg/media/
  types.go
  errors.go
  ffprobe/
    ffprobe.go
    parser.go
    runner.go
```

测试镜像生产目录：

```text
tests/pkg/media/ffprobe/
  ffprobe_test.go
  ffprobe_integration_test.go
  testdata/
```

## 公共契约

```go
type Prober interface {
    Probe(ctx context.Context, input Input, opts ProbeOptions) (Info, error)
}
```

首版 `Input` 只接受本地普通文件路径。远程 URL、header、Cookie、临时凭据和网络访问策略尚未形成稳定安全模型，因此不在公共契约内。

`Info` 归一化以下信息：

- 容器短名称、长名称、时长、文件大小和总码率。
- 视频 stream index、codec、profile、宽高、像素格式、平均帧率及原始有理数、旋转和色彩/HDR 信息。
- 音频 stream index、codec、profile、采样率、声道数、声道布局和语言。
- 字幕 stream index、codec、语言和标题。

没有视频流是合法结果。无法解析的帧率归一为 `FrameRate == 0` 且 `FrameRateRatio == ""`，不会导致整个媒体探测失败。

HDR 判断规则：

- `color_transfer == smpte2084`：PQ。
- `color_transfer == arib-std-b67`：HLG。
- 只有 BT.2020 primaries、但 transfer 信息不完整：标记 HDR，类型为 `unknown`。

## ffprobe driver

构造时执行 `ffprobe -version` 验证二进制可用性。`MinVersion` 和 `MaxVersion` 可选；配置后无法比较或超出范围都会返回 `ErrVersionUnsupported`。

探测命令固定使用参数数组，不经过 shell：

```text
-v error -print_format json -show_format -show_streams -i <local-file>
```

生产 runner 复用 `pkg/execx.RunOutput`，启用输出上限和进程组清理。调用方负责设置 context deadline；driver 负责：

- 在执行前验证 context 和本地普通文件。
- 限制 stdout/stderr 收集大小。
- 校验 JSON 并隔离 ffprobe 原始结构。
- 解析小数时长、整数字段和有理数帧率。
- 将 rotation tag 或 side data 归一为角度。
- 对执行失败返回不包含输入路径和底层命令参数的安全摘要。

## 错误语义

所有 driver 错误使用 `*media.Error`，支持 `errors.Is` 和 `errors.As`：

- `ErrInvalidInput`
- `ErrInputUnavailable`
- `ErrBinaryUnavailable`
- `ErrVersionUnsupported`
- `ErrProbeFailed`
- `ErrOutputInvalid`
- `ErrOutputLimit`

context canceled/deadline exceeded 原样保留，调用方可直接判断。

## 边界

`pkg/media` 负责媒体技术元数据，不负责：

- 下载文件或远程 URL 探测。
- 清晰度业务标签、剧集识别、命名规则和 manifest。
- Asset 建档、数据库写入或异步任务。
- 自动安装或更新 ffmpeg/ffprobe。
- 全局默认 Prober、Manager、bootstrap 或 runtime capability。

## 测试

```bash
go test ./tests/pkg/media/ffprobe
```

默认测试通过 fake runner 和脱敏 JSON fixtures 覆盖视频、纯音频、多音轨、字幕、异常帧率、HDR、旋转、取消、超大输出、非零退出和无效 JSON。真实本机 ffprobe 测试通过环境变量显式开启。

## 更新记录

- 2026-08-23：新增 pkg-only Media 契约与本地 ffprobe driver；不接入 core/runtime。
