# 公共库

可被外部项目导入的公共 Go 包。注意：本项目的内部包放在 `core/` 下，`pkg/` 用于需要对外暴露的库。

## 外部文件与媒体

- `pkg/remotefs`：外部层级文件系统公共契约，包含 local 与 BaiduPan driver。参见 [模块设计](../docs/modules/remotefs.md) 和 [使用指南](../docs/usage/remotefs.md)。
- `pkg/media`：本地媒体元数据探测契约，包含 ffprobe driver。参见 [模块设计](../docs/modules/media.md) 和 [使用指南](../docs/usage/media.md)。

## 服务 SDK

- `pkg/sdingest`：SDIngest typed Go SDK。`BaseURL` 携带部署前缀（例如 `/api`），SDK 只追加 `/v1/...`，并管理 App Token、幂等写请求、任务/产物 DTO、业务错误和 Callback。参见 [模块设计](../docs/modules/sdingest.md) 和 [使用指南](../docs/usage/sdingest.md)。
