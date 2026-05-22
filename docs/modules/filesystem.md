# 文件系统

## 作用

`core/storage` 提供统一文件能力，屏蔽 local、S3/MinIO、COS、OSS 的差异。它只作为基础工具库存在，不负责 router、server 或业务注入。

主要覆盖：

- 服务端上传、流式上传、URL 拉取上传
- worker 异步文件上传的底层存储能力
- local 服务端分片上传
- 对象存储直传凭证
- 下载与进度回调
- 文件大小、扩展名、文件名验证
- 图片信息读取与显式裁剪上传

## 初始化

工具库入口：

```go
fs, err := pkgfs.NewFromPolicy(policy, pkgfs.WithUploadDir("uploads"))
if err != nil {
    return err
}
defer fs.Close()
```

文件系统作为 bootstrap 公共 runtime capability 注册，服务 provider 不再声明 `admin.filesystem`、`worker.filesystem` 或 `crontab.filesystem`：

```go
storagefacade.Use(
    storagefacade.WithConfigLoader(func(*runtime.App) (storagefacade.Config, error) {
        cfg, err := boot.requireConfig()
        if err != nil {
            return storagefacade.Config{}, err
        }
        return cfg.FileSystem, nil
    }),
)
```

服务目录只保留业务侧入口，例如 `core/storage`、`core/storage`、`core/storage`、`core/storage`。这些 adapter 优先使用服务内显式设置的默认实例，未设置时 fallback 到 bootstrap 公共 filesystem。handler/task 使用服务 adapter：

```go
fs, err := storage.Default()
if err != nil {
    return err
}
```

默认实例在 bootstrap runtime 初始化时创建，在 runtime capability shutdown 时释放。`NewFileSystem()` 只用于确实需要独立实例的场景，业务工厂不放在 `core/storage` 中。

各服务可以独立运行；开发阶段的统一启动只做进程编排，不改变服务内依赖的初始化边界。framework runtime 生命周期实现放在 `core/storage/facade`，服务侧 adapter 只保存当前服务默认入口。配置装配是启动层细节，对 handler/task 只暴露文件系统使用接口。通用默认值由 `core/storage` 处理，driver 专属字段由各 driver 自己读取，不放到 `core` 或根目录 `internal`。

## 配置项

核心策略为 `core/storage.StoragePolicy`：

| 字段 | 说明 |
|------|------|
| `driver` | `local` / `s3` / `minio` / `cos` / `oss` |
| `bucket` | 对象存储 bucket |
| `endpoint` | 服务端外网 endpoint |
| `endpoint_inner` | 服务端内网 endpoint，存在时 SDK 优先使用 |
| `public_url` | 用户访问文件的公开域名前缀 |
| `cdn_url` | CDN 域名前缀，`Source()` 优先使用 |
| `region` | S3/MinIO region |
| `access_key` | 统一访问标识，兼容 AccessKey、SecretID、AppID 等命名 |
| `secret_key` | 统一访问密钥，兼容 SecretKey、AccessSecret、AppKey 等命名 |
| `use_ssl` | S3/MinIO 是否使用 SSL |
| `local_dir` | local 驱动根目录 |

驱动自身的 SDK 配置只保留在对应 driver 内部，对外不暴露 `S3Config`、`COSConfig`、`OSSConfig` 这类厂商结构。

## 对外接口

高层入口：

- `New(cfg *core.Config) (*FileSystem, error)`
- `NewFromPolicy(policy core.StoragePolicy, opts ...Option) (*FileSystem, error)`
- `NewFileSystem(policy core.StoragePolicy, opts ...Option) (*FileSystem, error)`
- `Close() error`
- `Recycle()`

核心能力：

- `Upload(ctx, fileHeader)`
- `UploadStream(ctx, reader, info)`
- `UploadFromURL(ctx, rawURL, info)`
- `InitUpload(ctx, req)`
- `UploadChunk(ctx, req)`
- `UploadStatus(uploadID)`
- `Download(ctx, path, writer, progress)`
- `ValidateFileInfo(info)`
- `DecodeImageInfo(reader)`
- `CropImage(reader, rect, format)`
- `UploadCroppedImage(ctx, reader, rect, info)`

底层兼容能力：

- `Handler() core.Handler`
- `Put/Get/Delete/List/Source/Token`

## Hook

当前内置 hook：

- `HookBeforeUpload`
- `HookAfterUpload`
- `HookBeforeDelete`
- `HookAfterDelete`

默认启用上传前文件验证，业务可通过 `Use` 追加 hook，通过 `CleanHooks` 清理。

## 使用示例

HTTP handler 使用服务自己的默认实例：

```go
fs, err := storage.Default()
if err != nil {
    response.Error(c, apperrors.NewCodeWithData(apperrors.CodeInternal, "文件系统初始化失败", nil))
    return
}
```

Worker 任务如果只是做通用文件上传，优先使用 `worker/taskdef.FileGenerateUploadPayload`：

```go
payload := taskdef.FileGenerateUploadPayload{
    Policy:       policy,
    UploadDir:    "daily-report",
    FileName:     "2026-05-11.xlsx",
    MIMEType:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    TempFilePath: tmpPath,
}
```

如果业务 handler 需要直接操作 filesystem，可在 worker 服务内创建独立实例或使用本服务默认实例：

```go
fs, err := pkgfs.NewFromPolicy(policy)
if err != nil {
    return err
}
defer fs.Close()

info, err := fs.UploadStream(ctx, reader, core.FileInfo{
    Name: "report.txt",
    Size: size,
    Progress: func(written, total int64) {
        logger.WithContext(ctx).Info("文件上传进度", zap.Int64("written", written), zap.Int64("total", total))
    },
})
```

worker 文件上传任务的完整用法见 [worker-file-upload.md](../usage/worker-file-upload.md)。

## 注意事项

- `endpoint` / `endpoint_inner` 是服务端 SDK 使用地址，不直接给用户访问。
- `public_url` / `cdn_url` 是文件访问域名前缀，`cdn_url` 优先级更高。
- 图片裁剪是显式调用能力，不在普通上传链路中自动执行，避免额外 CPU 和内存成本。
- HTTP 服务建议在服务启动时初始化默认 `FileSystem`，handler 复用默认实例，服务关闭时调用 `Close()`。
- worker 文件上传的大文件来源优先使用 `temp_file_path` 或 `source_url`，不要把 Excel、视频、压缩包等放进队列 `content`。
- local 分片上传会话绑定当前 `FileSystem` 实例，`InitUpload` 和后续 `UploadChunk` 应走同一个服务默认实例。

## 已知限制

- local 分片上传会在服务端保存临时分片，需要定期清理过期会话或临时目录。
- GIF 等动画图片裁剪不会保留动画帧。
- 真正的云 OSS/COS/S3 集成测试需要真实账号和网络环境，本地测试只覆盖调度、参数映射和 local 行为。

## Breaking Changes

- `core/storage.Config` 支持统一 `policy`，同时通过通用 `DriverConfig` 兼容旧的 `local/s3/cos/oss` 嵌套配置；driver 专属字段由对应 driver 读取。
- HTTP 文件接口不再使用包级全局 `InitFileSystem` 存放 `fsClient`，改为由服务启动层复用 `core/storage/facade` 初始化默认实例，并由本服务 `core/storage` 在 handler/task 侧提供入口。
