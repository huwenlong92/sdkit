# Storage 存储

`core/storage` 是文件存储的核心入口，提供默认存储和多个命名存储。

方案文档见 [../modules/storage.md](../modules/storage.md)。

## 配置

推荐使用 `storage` 节点：

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

没有配置 `storage` 节点时，`facade.Use()` 会使用本地默认配置：`default` store，driver 为 `local`，目录为 `storage`。只要配置了 `storage` 节点，`default` 就必须显式配置，并且必须指向 `stores` 中存在的名称。业务需要把一部分资源放到其他存储时，通过 store 名称显式选择。

## 初始化

Runtime 接入层放在 `core/storage/facade`：

```go
import storagecap "github.com/huwenlong92/sdkit/core/storage/facade"

app := runtime.New()
if err := storagecap.Use().Register(app); err != nil {
    return err
}
```

已经有配置对象时可以直接传入：

```go
capability := storagecap.Use(storagecap.WithConfig(storagecap.Config{
    Default: "local",
    Stores: map[string]storagecap.StoreConfig{
        "local": {
            Driver:   "local",
            LocalDir: "storage",
        },
    },
}))
```

## 使用默认存储

```go
fs, err := storage.Default()
if err != nil {
    return err
}

result := fs.UploadStream(ctx, reader, storage.FileInfo{
    Name: "avatar.png",
    Size: size,
})
if result.Error != nil {
    return result.Error
}

info := result.File
```

需要默认存储的文件访问域名时，不要在应用层解析 policy：

```go
cdnURL := storage.DefaultCDNURL()
```

已经持有 `FileSystem` 实例时也可以直接读取：

```go
cdnURL := fs.CDNURL()
```

保存给前端的访问值由 manager 统一判断 default 与非 default：

```go
value := storage.AccessPath(storeName, objectPath)
```

规则固定为：默认 store 返回原始 `path`；非默认 store 配置了 `cdn_url` 时返回 `cdn_url + path`，否则仍返回原始 `path`。

删除时可以传对象路径，也可以传当前 store 的访问值。`Delete` 会先把匹配当前 store `cdn_url` 的 URL 还原成对象路径：

```go
_ = fs.Delete("https://cdn.example.com/uploads/avatar.png")
```

## 临时私有访问链接

需要把私有文件临时发给别人时，使用 `Source(path, ttl)`。`ttl > 0` 表示生成带有效期的私有访问链接：

```go
url, err := fs.Source("private/report.pdf", 30*time.Minute)
if err != nil {
    return err
}
```

对象存储 driver 会使用原生签名 URL：

- `s3` / `minio`：生成 presigned GET URL
- `r2`：生成 Cloudflare R2 S3 兼容 presigned GET URL
- `oss`：生成 GET 签名 URL
- `cos`：生成 GET 签名 URL

`s3`、`minio`、`r2` 底层使用 AWS SDK for Go v2。配置自建 S3 兼容服务时，`endpoint` 建议带上 `http://` 或 `https://`；未带协议时默认按 `https://` 处理。

`local` driver 没有对象存储签名能力，会生成带 `path`、`expires`、`signature` 的应用访问链接。需要配置签名密钥，并在应用路由中挂载校验 handler：

```yaml
storage:
  default: local
  stores:
    local:
      driver: local
      local_dir: storage
      endpoint: https://admin.example.com/storage/source
      secret_key: ${STORAGE_SOURCE_SECRET}
```

```go
fs, err := storage.Default()
if err != nil {
    return err
}
router.GET("/storage/source", gin.WrapH(storage.SourceHandler(fs, sourceSecret)))
```

local 的 `endpoint` 是对外签名访问入口地址；为空时会生成相对路径 `/storage/source`。`secret_key` 必须和 handler 使用的密钥一致。local 访问链接默认使用 `inline`，浏览器支持的图片、PDF 等文件会在线预览；需要强制下载时追加 `download=1`。

`ttl <= 0` 保持公开访问语义：如果配置了 `cdn_url`，优先返回静态公开 URL；没有公开 URL 时，对象存储会使用默认 7 天有效期的签名 URL，local 返回本地文件路径。

## 指定存储

```go
fs, err := storage.Use("minio")
if err != nil {
    return err
}

result := fs.UploadStream(ctx, reader, storage.FileInfo{
    Name: "private.pdf",
    Size: size,
})
if result.Error != nil {
    return result.Error
}
```

空名称等同默认存储。store 不存在时返回 `ErrStoreNotFound`。

## 临时策略

需要按 DB 策略、租户策略或业务规则临时创建实例时使用 `New`，不要污染默认 manager：

```go
fs, err := storage.New(storage.Policy{
    Driver:   "minio",
    Bucket:   "tenant-assets",
    Endpoint: "http://127.0.0.1:9000",
})
if err != nil {
    return err
}
defer fs.Close()
```

## 本次操作 Hook

hook 支持上传、下载、读取、删除、列表、签名地址和上传凭证等存储操作。通过参数传入的 hook 只对本次操作生效，执行完自动失效：

```go
result := fs.UploadStreamWithHook(ctx, reader, storage.FileInfo{
    Name: "avatar.png",
    Size: size,
},
    storage.BeforeUpload(func(ctx context.Context, event storage.Event) error {
        return validateFile(event.File)
    }),
    storage.AfterUpload(func(ctx context.Context, event storage.Event) error {
        return fileRepo.Create(ctx, event.File)
    }),
)
if result.Error != nil {
    if result.Uploaded {
        _ = fs.Delete(result.File.Path)
    }
    return result.Error
}
```

执行规则：

- `BeforeUpload` 报错时不会执行上传
- 上传失败后执行 `AfterUploadFailed`
- `AfterUpload` 报错时，`result.Uploaded` 为 `true`，`result.File` 是已上传文件
- 多个 hook 按传入顺序执行
- 本次操作 hook 不会注册到全局 store

## 客户端上传凭证

`InitUpload` 返回的凭证会带上 `mode`，前端按该字段选择上传方式，不要根据 driver 名称或 URL 字段自行推断：

```go
cred, err := fs.InitUpload(ctx, storage.UploadInitRequest{
    FileName:  "avatar.png",
    Path:      "avatar/avatar.png",
    TotalSize: size,
    MIMEType:  "image/png",
})
if err != nil {
    return err
}

switch cred.Mode {
case storage.UploadModeLocalChunk:
    // 上传到应用服务分片接口
case storage.UploadModeDirectPut:
    // 使用 cred.UploadURLs[0] 直传
case storage.UploadModeMultipartPut:
    // 使用 cred.UploadURLs 上传分片，再调用 cred.CompleteURL
}
```

## 约定

- 应用默认存储放在 `storage.default`
- 配置了 `storage` 节点时，`storage.default` 必须显式配置
- 额外存储放在 `storage.stores.<name>`
- 业务代码需要跨 driver 时通过 `storage.Use(name)` 显式选择
- 配置入口只使用 `storage`，不要再新增 `filesystem` 配置
- Cloudflare R2 使用 `driver: r2`，`endpoint` 为 `https://<ACCOUNT_ID>.r2.cloudflarestorage.com`，region 默认按 R2 要求使用 `auto`
- 新增 driver 时必须在上传凭证中补齐 `mode`
