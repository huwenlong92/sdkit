# 文件系统

基于驱动模式的多后端文件存储库，支持服务端上传和客户端直传。

## 结构

```
core/storage/
├── fs.go                 # filesystem.New() 工厂
├── core/
│   ├── types.go          # Handler 接口 + 文件类型 + Config + 凭证
│   └── namer.go          # 文件名生成器（模板变量）
├── chunk/
│   └── chunk.go          # 分片管理（保存/合并/清理）
└── driver/
    ├── local/local.go    # 本地磁盘驱动
    ├── s3/s3.go          # S3 / MinIO 驱动
    ├── cos/cos.go        # 腾讯云 COS 驱动
    └── oss/oss.go        # 阿里云 OSS 驱动
```

## 配置

```yaml
filesystem:
  driver: local                # local / s3 / minio / cos / oss
  upload_dir: uploads          # 默认上传目录
  temp_dir: storage/.chunks    # local 服务端分片临时目录，默认 local.dir/.chunks
  max_size: 0                  # 单文件大小限制，0 表示不限制
  chunk_size: 5242880          # 服务端 local 分片大小
  token_ttl: 2h                # 直传凭证有效期
  dir_rule: "{date}"           # 目录规则
  file_rule: "{originname}{ext}" # 文件名规则
  allowed_extensions: [".jpg", ".png", ".txt"] # 空表示不限制
  policy:
    driver: local              # 优先使用统一策略字段
    name: default
    bucket: ""
    endpoint: ""               # 服务端外网 endpoint
    endpoint_inner: ""         # 服务端内网 endpoint
    public_url: https://static.example.com/files
    cdn_url: https://cdn.example.com/files
    region: ""
    access_key: ""             # 统一访问标识，COS 的 SecretID / 部分服务的 AppID 也放这里
    secret_key: ""             # 统一访问密钥，部分服务的 AppKey 也放这里
    use_ssl: true
    local_dir: storage
  local:
    dir: storage                # 本地存储根目录
    public_url: https://static.example.com/files # 用户访问域名前缀，可选
    cdn_url: https://cdn.example.com/files       # CDN 域名前缀，优先于 public_url
  s3:
    bucket: my-bucket
    endpoint: s3.amazonaws.com  # 服务端外网 endpoint，MinIO 用 192.168.1.x:9000
    endpoint_inner: ""          # 服务端内网 endpoint，可选
    public_url: https://static.example.com/files # 用户访问域名前缀，可选
    cdn_url: https://cdn.example.com/files       # CDN 域名前缀，优先于 public_url
    region: us-east-1
    access_key: xxx
    secret_key: xxx
    use_ssl: true               # MinIO 用 false
  cos:
    bucket: my-bucket
    endpoint: https://cos.ap-guangzhou.myqcloud.com       # 外网
    endpoint_inner: https://cos.ap-guangzhou.internal.com  # 内网（可选）
    public_url: https://static.example.com/files
    cdn_url: https://cdn.example.com/files
    secret_id: ""
    secret_key: ""
  oss:
    bucket: my-bucket
    endpoint: oss-cn-beijing.aliyuncs.com                 # 外网
    endpoint_inner: oss-cn-beijing-internal.aliyuncs.com  # 内网（可选）
    public_url: https://static.example.com/files
    cdn_url: https://cdn.example.com/files
    access_key_id: ""
    access_secret: ""
```

推荐把策略表设计成一行一个统一策略，而不是按厂商拆字段：

| 字段 | 说明 |
|------|------|
| `driver` | `local` / `s3` / `minio` / `cos` / `oss` |
| `bucket` | bucket / bucket name |
| `endpoint` | 服务端外网 endpoint |
| `endpoint_inner` | 服务端内网 endpoint，存在时 SDK 优先使用 |
| `public_url` | 给用户访问文件的公开域名前缀 |
| `cdn_url` | CDN 加速域名前缀，`Source()` 优先使用 |
| `region` | S3/MinIO 等需要 region 时填写 |
| `access_key` | 统一访问标识；厂商叫 AccessKey、SecretID、AppID 时都存这里 |
| `secret_key` | 统一访问密钥；厂商叫 SecretKey、AccessSecret、AppKey 时都存这里 |
| `use_ssl` | S3/MinIO SDK 连接是否使用 SSL |
| `local_dir` | local 驱动根目录 |

`filesystem.New` 会在初始化时按 `driver` 分配具体驱动。统一 `policy` 可直接用于数据库策略行；旧的 `local/s3/cos/oss` 嵌套配置仍兼容，作为 driver 自己读取的配置段。

项目从 yaml 配置创建文件系统时，由 bootstrap 通过 `core/storage/facade.Use(...)` 注册公共 runtime capability，handler 或任务通过本服务 `core/storage` 的 `Default()` 获取实例。服务目录只保留业务默认入口，未显式设置服务内实例时 fallback 到 bootstrap 公共 filesystem。配置装配函数不作为业务 API 暴露；driver 专属字段由对应 driver 自己读取。

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

如果策略来自数据库，推荐直接用 `NewFromPolicy`：

```go
policy := core.StoragePolicy{
    Driver:    "oss",
    Bucket:    row.Bucket,
    Endpoint:  row.Endpoint,
    PublicURL: row.PublicURL,
    CDNURL:    row.CDNURL,
    AccessKey: row.AccessKey, // 厂商叫 AppID / SecretID 时也放这里
    SecretKey: row.SecretKey, // 厂商叫 AppKey / AccessSecret 时也放这里
}

fs, err := filesystem.NewFromPolicy(
    policy,
    filesystem.WithUploadDir("uploads"),
    filesystem.WithChunkSize(5<<20),
    filesystem.WithNameRules("{date}", "{uuid}{ext}"),
    filesystem.WithAllowedExtensions(".jpg", ".png", ".pdf"),
)
if err != nil {
    return err
}
defer fs.Close()
```

`New(*core.Config)` 主要用于读取完整 yaml 配置或兼容旧配置；策略表场景不需要构造完整 `Config`。

地址约定：

| 字段 | 用途 |
|------|------|
| `endpoint` | 服务端 SDK 使用的外网 endpoint |
| `endpoint_inner` | 服务端 SDK 优先使用的内网 endpoint |
| `public_url` | 给用户访问文件的公开域名前缀 |
| `cdn_url` | CDN 加速域名前缀，`Source()` 优先使用 |

## FileSystem 入口

`filesystem.New` 返回高层 `*filesystem.FileSystem`。它内部通过 `DispatchHandler()` 按配置分配 local / s3 / minio / cos / oss 驱动，同时提供公共库能力：

- 服务端普通上传：`UploadStream`
- 服务端分片上传：`InitUpload` + `UploadChunk` + `UploadStatus`
- 客户端直传凭证：对象存储驱动返回 presigned URL
- URL 拉取保存：`UploadFromURL`
- 下载到 writer：`Download`
- 上传/下载进度回调
- 上传 hooks
- 文件名、大小、扩展名验证
- 图片格式/尺寸读取和显式裁剪上传

底层驱动接口仍可通过 `fs.Handler()` 取得。

## Handler 接口

```go
type Handler interface {
    Put(file FileHeader) error                          // 服务端上传
    Get(path string) (io.ReadCloser, error)             // 下载
    Delete(paths ...string) error                       // 删除
    List(path string) ([]Object, error)                 // 列目录
    Source(path string, ttl time.Duration) (string, error) // 访问 URL
    Token(fileInfo FileInfo, ttl time.Duration)          // 客户端直传凭证
        (*UploadCredential, error)
}
```

## 两种上传模式

### 模式一：服务端上传

文件流经服务端转发到后端存储。

```go
import "github.com/huwenlong92/sdkit/core/storage"
import "github.com/huwenlong92/sdkit/core/storage"

fs, _ := filesystem.New(&core.Config{
    Policy: core.StoragePolicy{
        Driver:   "local",
        LocalDir: "storage",
    },
})

info, err := fs.UploadStream(ctx, reader, core.FileInfo{
    Name: "photo.jpg",
    Size: 1024,
})
if err != nil {
    return err
}

reader, _ := fs.Get(info.Path)
fs.Delete(info.Path)
objects, _ := fs.List("uploads/")
```

```bash
curl -X POST /files/upload -F "file=@photo.jpg"
```

| 驱动 | 上传方式 |
|------|---------|
| local | 写入本地磁盘 |
| s3/minio | s3manager.Upload 自动处理单请求/分片 |
| cos | 小文件 PUT，大文件自动 MultipartUpload |
| oss | 小文件 PUT，大文件自动分片上传 |

### 模式二：客户端直传（推荐大文件）

服务端生成凭证，客户端直接上传到后端存储，不经过服务端中转。

**流程：**

```
客户端                    服务端                    S3/MinIO
  │                        │                        │
  ├─ POST /upload/init ───→│                        │
  │←────── presigned URLs ─┤                        │
  │                        │                        │
  ├─ PUT chunk0 ──────────────────────────────────→│
  ├─ PUT chunk1 ──────────────────────────────────→│
  ├─ ...                   │                        │
  ├─ POST complete ───────────────────────────────→│
  │                        │                        │
```

**local 驱动：** init 返回 upload_id，客户端逐片 POST 到 `/upload/chunk`，服务端合并。合并后的保存也走 `UploadStream`，因此会触发统一 hooks。

**S3/MinIO / COS / OSS 驱动：**

```bash
# 1. 初始化，获取 presigned URLs
curl -X POST /files/upload/init \
  -H 'Content-Type: application/json' \
  -d '{"file_name":"big.zip","total_size":20971520,"mime_type":"application/zip"}'

# 返回：
{
  "gateway": "s3",
  "upload_id": "xxx",           # S3 UploadId
  "path": "uploads/20260508/...",
  "chunk_size": 5242880,
  "chunk_num": 4,
  "upload_urls": [              # 每片的直传 URL
    "https://bucket.s3.amazonaws.com/...partNumber=1...",
    "https://bucket.s3.amazonaws.com/...partNumber=2...",
    "https://bucket.s3.amazonaws.com/...partNumber=3...",
    "https://bucket.s3.amazonaws.com/...partNumber=4..."
  ],
  "complete_url": "https://bucket.s3.amazonaws.com/...uploadId=xxx..."
}

# 2. 客户端逐片 PUT 到 upload_urls（不需要经过服务端）
curl -X PUT "https://bucket.s3.amazonaws.com/...partNumber=1..." \
  -H "Content-Type: application/zip" \
  --data-binary @part1

# 3. 所有分片传完后，调用 complete_url 完成合并
curl -X POST "https://bucket.s3.amazonaws.com/...uploadId=xxx..."
```

| 驱动 | init 返回 | 上传方式 |
|------|-----------|---------|
| local | upload_id + chunk_size | 客户端 POST `/upload/chunk`，服务端合并 |
| s3/minio | presigned URLs | 客户端直 PUT 到 S3，最后调用 complete_url |
| cos | presigned URLs | 客户端直 PUT 到 COS，最后调用 complete_url |
| oss | presigned URLs | 客户端直 PUT 到 OSS |

## 文件名生成

支持模板变量，默认规则 `{date}/{uuid}{ext}`：

```go
namer := &core.Namer{
    DirRule:  "{date}/{randomkey8}",
    FileRule: "{uuid}{ext}",
}
path := namer.Generate("photo.jpg")
// → "20260508/Ab3Xy9/a1b2c3d4.jpg"
```

| 变量 | 示例 | 说明 |
|------|------|------|
| `{uuid}` | `a1b2c3d4e5f6...` | UUID，无横线 |
| `{randomkey16}` | `Ab3X...` | 16 位随机字母数字 |
| `{randomkey8}` | `Xy9...` | 8 位随机字母数字 |
| `{timestamp}` | `1746612345` | Unix 时间戳 |
| `{datetime}` | `20260508123000` | 年月日时分秒 |
| `{date}` | `20260508` | 年月日 |
| `{year}` | `2026` | 年 |
| `{month}` | `05` | 月 |
| `{day}` | `08` | 日 |
| `{hour}` | `12` | 时 |
| `{minute}` | `30` | 分 |
| `{second}` | `00` | 秒 |
| `{originname}` | `photo` | 原始文件名（不含扩展名） |
| `{ext}` | `.jpg` | 文件扩展名 |

## HTTP 接口

Admin（`/admin/v1/files/xxx`）和 API（`/api/v1/files/xxx`）均提供相同接口：

| 方法 | 路径 | 说明 | 请求 |
|------|------|------|------|
| POST | `/files/upload` | 服务端上传 | multipart `file` |
| GET | `/files/download` | 下载文件 | query `path` |
| POST | `/files/delete` | 删除文件 | body `path` |
| GET | `/files/list` | 列出目录 | query `path` |
| POST | `/files/upload/init` | 初始化直传凭证 | body `file_name,total_size,mime_type` |
| POST | `/files/upload/chunk` | 上传分片(local) | multipart `upload_id,chunk_index,file` |
| GET | `/files/upload/status` | 查询分片状态 | query `upload_id` |

## Hooks

`FileSystem` 支持按事件注册 hooks：

```go
fs.Use(filesystem.HookBeforeUpload, func(ctx context.Context, fs *filesystem.FileSystem, file core.FileInfo) error {
    return fs.ValidateFileInfo(file)
})

fs.Use(filesystem.HookAfterUpload, func(ctx context.Context, fs *filesystem.FileSystem, file core.FileInfo) error {
    // 写业务表、发送消息、记录审计日志
    return nil
})
```

内置事件：

| Hook | 触发时机 |
|------|----------|
| `BeforeUpload` | 服务端普通上传、URL 上传、local 分片最终合并入库、直传初始化前 |
| `AfterUpload` | 服务端写入成功后 |
| `AfterUploadFailed` | 服务端写入或生成凭证失败 |
| `AfterValidateFailed` | 上传前校验或后置业务校验失败 |
| `BeforeDownload` | `Download` 开始前 |
| `AfterDownload` | `Download` 成功后 |
| `AfterDownloadFailed` | `Download` 失败后 |

## 上传进度

```go
info := core.FileInfo{
    Size: fileSize,
    Progress: func(uploaded, total int64) {
        pct := float64(uploaded) / float64(total) * 100
        log.Printf("上传: %.1f%%", pct)
    },
}
file := core.NewFileStream(reader, info)
fs.Put(file)
```

## 下载和下载进度

```go
var out bytes.Buffer
err := fs.Download(ctx, "uploads/20260508/a.txt", &out, func(downloaded, total int64) {
    // total 对 local 文件可取到实际大小；对象存储场景可能为 0
})
```

`Get(path)` 仍保留为低级接口，适合 HTTP handler 直接流式返回。

## 从 URL 拉取保存

```go
info, err := fs.UploadFromURL(ctx, "https://example.com/a.png", core.FileInfo{
    Name: "a.png",
})
```

`UploadFromURL` 只支持 `http/https`，会复用上传 hooks 和验证逻辑。

## Worker 异步文件上传

worker 文件上传任务基于 filesystem 的服务端上传能力实现。任务 payload 只描述文件来源和存储策略，不在队列中携带大文件内容。

| 来源 | payload 字段 | filesystem 入口 |
|------|--------------|-----------------|
| 第三方 URL | `source_url` | `UploadFromURL` |
| worker 生成的临时文件 | `temp_file_path` | `UploadStream` |
| worker 可访问的普通路径 | `source_path` | `UploadStream` |
| 小文本内容 | `content` | `UploadStream` |

`temp_file_path` 上传成功后由 worker 删除；`source_path` 上传成功后不删除源文件。Excel 报表、视频转存等场景见 [worker-file-upload.md](worker-file-upload.md)。

## 文件验证公共方法

```go
err := fs.ValidateFileInfo(core.FileInfo{Name: "photo.jpg", Size: 1024})
ok := fs.ValidateFileName("photo.jpg", "")
ok = fs.ValidateFileSize(1024)
ok = fs.ValidateExtension("photo.jpg", "")
```

这些方法与默认 `BeforeUpload` hook 使用同一套规则。

## 图片工具

默认上传不解码整张图片。图片基础信息使用 `image.DecodeConfig`，只读取头部信息：

```go
img, err := filesystem.DecodeImageInfo(reader)
err = filesystem.ValidateImageInfo(img, filesystem.ImageLimit{
    MaxWidth:  4096,
    MaxHeight: 4096,
})
```

裁剪是显式操作，调用时才会完整解码并重新编码：

```go
info, err := fs.UploadCroppedImage(ctx, reader, core.FileInfo{Name: "avatar.png"},
    filesystem.CropRect{X: 0, Y: 0, Width: 256, Height: 256},
    "png",
    0,
)
```

当前裁剪编码支持 `png` 和 `jpeg`。

## 多存储实例

一个 `FileSystem` 实例对应一个存储策略。服务端多 OSS 管理可以在业务层维护多个实例：

```go
stores := map[string]*filesystem.FileSystem{
    "public":  publicFS,
    "private": privateFS,
}

fs := stores["public"]
info, err := fs.UploadStream(ctx, reader, core.FileInfo{Name: "a.txt"})
```

后续如果需要统一管理租户、bucket、内外网 endpoint，可在 `filesystem` 上层增加 manager，但底层上传、下载、hooks 和验证能力无需重复实现。

## 增加新驱动

在 `driver/` 下新建包，实现 `core.Handler`：

```go
package mydriver

import "github.com/huwenlong92/sdkit/core/storage"

type Driver struct{}

func (d *Driver) Put(file core.FileHeader) error           { /*...*/ }
func (d *Driver) Get(path string) (io.ReadCloser, error)    { /*...*/ }
func (d *Driver) Delete(paths ...string) error              { /*...*/ }
func (d *Driver) List(path string) ([]core.Object, error)   { /*...*/ }
func (d *Driver) Source(path string, ttl time.Duration) (string, error) { /*...*/ }
func (d *Driver) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) { /*...*/ }
```

然后在 `fs.go` 注册：

```go
case "mydriver":
    return mydriver.New(cfg), nil
```
