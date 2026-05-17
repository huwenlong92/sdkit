# Worker 文件上传任务

`worker/taskdef.FileGenerateUploadPayload` 用于把异步任务生成或获取到的文件上传到 filesystem 支持的存储后端，例如 OSS、COS、S3、MinIO 或本地存储。

## 适用场景

| 场景 | 推荐字段 | 说明 |
|------|----------|------|
| Worker 业务过程生成临时文件，例如每日 Excel 报表 | `temp_file_path` | Worker 从本地临时文件流式上传，上传成功后删除临时文件 |
| 拉取第三方视频、图片或附件再上传 | `source_url` | Worker 从 URL 流式读取后上传，不把文件内容放进队列 |
| Worker 可访问的持久文件或共享卷文件 | `source_path` | Worker 从本地路径流式上传，上传后不删除源文件 |
| 小文本、小 JSON 或测试数据 | `content` | 只用于小内容输入；不要用于大文件 |

大文件不要放进 `content`。`content` 会进入队列 payload，文件越大，对 Redis、内存和重试都会越不友好。

## 字段优先级

Handler 按以下顺序选择输入源：

1. `source_url`
2. `temp_file_path`
3. `source_path`
4. `content`

`upload_dir` 只控制对象 key 前缀，不控制 driver 根目录或 bucket。真实存储位置由 `policy` 决定。

## 每日 Excel 报表上传 OSS

适合 worker 在任务执行过程中生成 `.xlsx` 文件，然后上传到 OSS 保存。

```go
tmpPath := "/tmp/report-2026-05-11.xlsx"

payload := taskdef.FileGenerateUploadPayload{
    Policy: fscore.StoragePolicy{
        Driver:    "oss",
        Bucket:    "my-bucket",
        Endpoint:  "oss-cn-beijing.aliyuncs.com",
        AccessKey: "access-key-id",
        SecretKey: "access-secret",
    },
    UploadDir:    "daily-report",
    FileName:     "2026-05-11.xlsx",
    MIMEType:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    TempFilePath: tmpPath,
}

_, err := queue.Enqueue(ctx,
    taskdef.NewFileGenerateUploadTask(payload),
    queue.Queue(queue.DefaultQueueName),
    queue.MaxRetry(3),
    queue.Timeout(5*time.Minute),
)
if err != nil {
    return err
}
```

执行行为：

- Worker 打开 `temp_file_path`，按 stream 上传到 OSS。
- 上传成功后删除 `temp_file_path`。
- 上传失败时不删除临时文件，保留给队列重试。

## 拉取第三方视频后上传 OSS

适合从第三方 URL 拉取视频，再保存到自己的 OSS。

```go
payload := taskdef.FileGenerateUploadPayload{
    Policy: fscore.StoragePolicy{
        Driver:    "oss",
        Bucket:    "my-bucket",
        Endpoint:  "oss-cn-beijing.aliyuncs.com",
        AccessKey: "access-key-id",
        SecretKey: "access-secret",
    },
    UploadDir: "video",
    FileName:  "demo.mp4",
    MIMEType:  "video/mp4",
    SourceURL: "https://example.com/demo.mp4",
}

_, err := queue.Enqueue(ctx,
    taskdef.NewFileGenerateUploadTask(payload),
    queue.Queue(queue.DefaultQueueName),
    queue.MaxRetry(3),
    queue.Timeout(5*time.Minute),
)
if err != nil {
    return err
}
```

执行行为：

- Worker 使用 `UploadFromURL` 发起 HTTP GET。
- 响应 body 作为 reader 直接传给 filesystem。
- OSS driver 以流式方式上传，不需要把视频整体读入队列 payload。

## 本机路径上传

如果文件不是临时文件，或者由共享卷管理生命周期，用 `source_path`：

```go
payload := taskdef.FileGenerateUploadPayload{
    Policy:     policy,
    UploadDir:  "archive",
    FileName:   "data.csv",
    MIMEType:   "text/csv",
    SourcePath: "/data/export/data.csv",
}
```

`source_path` 上传成功后不会删除源文件。

## 小内容输入

`content` 只适合小文件：

```go
payload := taskdef.FileGenerateUploadPayload{
    Policy:    policy,
    UploadDir: "debug",
    FileName:  "hello.txt",
    MIMEType:  "text/plain",
    Content:   "hello",
}
```

不要用 `content` 传 Excel、视频、压缩包等大文件。

## 注意事项

- `temp_file_path` 和 `source_path` 必须是 worker 进程可访问的路径。
- 服务独立部署时，API 服务本地路径通常不能直接给 worker 用；跨服务文件来源优先用 `source_url`。
- `temp_file_path` 上传成功后会被 worker 删除；需要保留源文件时使用 `source_path`。
- `source_url` 下载失败或返回非 2xx 状态会导致任务失败，由队列重试策略处理。
- `policy` 可以来自配置或数据库策略行，driver 专属字段由 filesystem driver 自己读取。
