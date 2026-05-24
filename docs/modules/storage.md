# Storage 模块方案

## 目标

`core/storage` 提供统一文件存储入口，支持默认 store 和多个命名 store。应用默认走配置中的 store，需要把部分资源放到其他后端时，通过 store 名称显式选择。

目标能力：

- 默认存储从 `storage` 配置初始化
- 未配置 `storage` 节点时使用本地默认 store
- 配置了 `storage` 节点时，`storage.default` 必须显式配置
- 支持多个命名 store
- 支持按名称懒加载非默认 store
- 暴露上传、分片上传、下载、hook、图片处理等完整文件能力
- Runtime capability 统一放在 `core/storage/facade`

## 模块边界

`core/storage` 负责：

- 管理默认 `Manager`
- 按配置创建默认 store
- 按名称创建和复用其他 store
- 提供 `Default()`、`Use(name)`、`New(policy)` 等入口
- 绑定 storage manager 到 runtime container

`core/storage` 不负责：

- 定义业务文件分类和业务目录规则
- 管理上传后的业务表数据
- 决定哪些业务资源使用哪个 store
- 在业务侧隐藏跨 store 的选择逻辑

具体 driver 和底层上传能力放在 `pkg/storage`。应用侧只依赖 `core/storage`，不要直接依赖 `pkg/storage`，除非是在 core 内部扩展 driver。

## 当前目录

```txt
core/storage/
  binding.go
  config.go
  default.go
  errors.go
  manager.go
  options.go
  types.go
  facade/
    facade.go
    use.go

pkg/storage/
  chunk/
  core/
  driver/
    cos/
    local/
    oss/
    s3/
```

## 配置模型

只使用 `storage` 节点：

```yaml
storage:
  default: cos
  stores:
    cos:
      driver: cos
      bucket: app-assets
      endpoint: https://example.cos.ap-shanghai.myqcloud.com
      secret_id: ${COS_SECRET_ID}
      secret_key: ${COS_SECRET_KEY}
    local:
      driver: local
      local_dir: storage
      endpoint: https://admin.example.com/storage/source
      secret_key: ${STORAGE_SOURCE_SECRET}
    minio:
      driver: minio
      bucket: private-assets
      endpoint: http://127.0.0.1:9000
      access_key: minio
      secret_key: minio-secret
    r2:
      driver: r2
      bucket: app-assets
      endpoint: https://<ACCOUNT_ID>.r2.cloudflarestorage.com
      access_key: ${R2_ACCESS_KEY_ID}
      secret_key: ${R2_SECRET_ACCESS_KEY}
```

`StoreConfig` 会转换成统一 `Policy`：

- `driver` 决定底层 driver
- `bucket`、`endpoint`、`region`、`access_key`、`secret_key` 等通用字段写入 `Policy`
- local 的 `endpoint` 用于临时私有下载入口，`secret_key` 用于签名
- `secret_id` 映射为 `AccessKey`，用于 COS
- `access_key_id` 和 `access_secret` 映射为 `AccessKey` / `SecretKey`，用于 OSS
- `dir` 和 `local_dir` 都映射为本地存储目录
- Cloudflare R2 复用 S3 兼容实现，`driver` 为 `r2`，`endpoint` 使用 R2 S3 API endpoint，`region` 默认 `auto`
- S3 / MinIO / R2 driver 使用 AWS SDK for Go v2；自建 S3 兼容服务的 `endpoint` 建议显式带协议，未带协议时默认按 `https://` 处理。

## Runtime Capability

Runtime 接入层统一放在 `core/storage/facade`：

```go
import storagecap "github.com/huwenlong92/sdkit/core/storage/facade"

app.RegisterCapabilities(storagecap.Use())
```

`facade.Use()` 的初始化顺序：

1. 优先使用 `WithManager`
2. 其次使用 `WithConfig`
3. 其次使用 `WithConfigLoader`
4. 最后从 `core/config.V` 读取 `storage`
5. 未配置 `storage` 时使用 `DefaultConfig()`，即 `default/local/storage`

注册成功后，manager 会写入 runtime container，也会成为包级默认 manager。

## 对外 API

核心入口：

```go
func NewManager(cfg Config) (*Manager, error)
func Default() (*FileSystem, error)
func DefaultCDNURL() string
func Use(name string) (*FileSystem, error)
func New(policy Policy, opts ...Option) (*FileSystem, error)
func PolicyOf(name string) (Policy, error)
func AccessPath(name string, objectPath string) string
func SourceHandler(fs *FileSystem, secret string) http.Handler
func Close() error
```

常用类型、hook、上传请求、分片状态、图片处理类型在 `core/storage` 直接导出。应用侧不要为了这些能力直接 import 底层包。

上传使用 result 模式。普通方法不接收本次操作 hook；需要本次操作 hook 时使用对应的 `WithHook` 方法：

```go
result := fs.UploadStream(ctx, reader, info)
if result.Error != nil {
    if result.Uploaded {
        // 主上传已成功，后续 hook 失败
    }
    return result.Error
}
```

当前成对提供：

- `Put` / `PutWithHook`
- `Get` / `GetWithHook`
- `Delete` / `DeleteWithHook`
- `List` / `ListWithHook`
- `Source` / `SourceWithHook`
- `Token` / `TokenWithHook`
- `Upload` / `UploadWithHook`
- `UploadStream` / `UploadStreamWithHook`
- `UploadFromURL` / `UploadFromURLWithHook`
- `InitUpload` / `InitUploadWithHook`
- `UploadChunk` / `UploadChunkWithHook`
- `Download` / `DownloadWithHook`
- `UploadCroppedImage` / `UploadCroppedImageWithHook`

临时私有访问链接统一使用 `Source(path, ttl)`：

- `ttl > 0`：生成带有效期的私有访问链接，R2 会走 S3 兼容 presigned URL
- `ttl <= 0`：保持公开访问语义，优先返回 `cdn_url`

对象存储 driver 使用各自 SDK 的 GET 签名 URL。local driver 生成带 `path`、`expires`、`signature` 的链接，需要应用把 `SourceHandler` 挂载到 local `endpoint` 对应路由上，由 handler 校验签名和过期时间后再读取文件。local handler 默认返回 `inline` 供浏览器在线预览；追加 `download=1` 时返回 `attachment` 强制下载。

本次操作 hook：

```go
storage.BeforeUpload(hook)
storage.AfterUpload(hook)
storage.AfterUploadFailed(hook)
storage.BeforeDownload(hook)
storage.AfterDownload(hook)
storage.AfterDownloadFailed(hook)
storage.BeforeGet(hook)
storage.AfterGet(hook)
storage.AfterGetFailed(hook)
storage.BeforeDelete(hook)
storage.AfterDelete(hook)
storage.AfterDeleteFailed(hook)
storage.BeforeList(hook)
storage.AfterList(hook)
storage.AfterListFailed(hook)
storage.BeforeSource(hook)
storage.AfterSource(hook)
storage.AfterSourceFailed(hook)
storage.BeforeToken(hook)
storage.AfterToken(hook)
storage.AfterTokenFailed(hook)
```

`Before*` 报错后主操作不执行；`After*` 报错时主操作已成功，result 中保留文件信息和错误。

## 内部约束

- `NewManager` 会立即初始化默认 store，启动阶段暴露默认配置错误
- 显式配置 `storage` 后，`storage.default` 为空时返回 `ErrDefaultRequired`
- 非默认 named store 懒加载，未使用的 minio、cos、oss 不在启动阶段初始化
- `Use("")` 等同于默认 store
- store 不存在时返回 `ErrStoreNotFound`
- `AccessPath("", path)` 和 `AccessPath(defaultName, path)` 固定返回原始 `path`
- `AccessPath(nonDefaultName, path)` 仅在该 store 配置了 `cdn_url` 时返回完整访问 URL
- `Delete` 会把匹配当前 store `cdn_url` 的访问 URL 还原为对象路径后再传给 driver
- `Close()` 只关闭已经初始化过的 store
- 通过操作参数传入的 hook 只对本次存储操作生效
- driver 返回上传凭证时必须显式填写 `UploadCredential.Mode`，应用和前端只消费该字段，不根据 `gateway`、`upload_urls`、`complete_url` 反推上传方式
- `Source(path, ttl)` 中 `ttl > 0` 表示临时私有访问；配置了 `cdn_url` 时也不会返回静态公开 URL
- `Source(path, ttl)` 中 `ttl <= 0` 保持历史公开访问行为；对象存储没有公开 URL 时默认生成 7 天有效期签名 URL
- local 临时私有访问必须配置 `secret_key`，并挂载 `SourceHandler`
- 当前上传模式包括：
  - `local_chunk`：客户端把分片上传到应用服务接口
  - `direct_put`：客户端使用单个预签名 URL 直传对象存储
  - `multipart_put`：客户端使用多个预签名 URL 上传分片，并调用完成合并 URL

## 更新记录

- S3 / MinIO / R2 driver 从 AWS SDK for Go v1 迁移到 AWS SDK for Go v2，移除已 EOL 的 `github.com/aws/aws-sdk-go` 依赖。
- 新增 Cloudflare R2 存储 driver，复用 S3 兼容协议，默认 region 为 `auto`。
- `Source(path, ttl)` 支持所有现有 driver 的临时私有访问链接；COS 补齐 GET 签名，local 增加签名 URL 和 `SourceHandler`。
- 上传凭证增加 `mode` 字段，后续新增 driver 必须同步声明上传模式。
- 新增 `core/storage`，支持默认 store、named store、runtime facade 和完整文件能力导出。
