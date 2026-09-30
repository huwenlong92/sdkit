# SDIngest SDK 使用

`pkg/sdingest` 面向调用 SDIngest 版本化 OpenAPI 的 Go 应用。调用方只配置公开服务地址和 App 凭据，不依赖 SDIngest 数据库、队列、Worker 或内部 model。默认把 SDIngest 服务根地址作为 `BaseURL`，SDK 方法只追加 `/v1/...`；接口路径没有 `/api` 标识。

## 创建客户端

```go
client, err := sdingest.NewClient(sdingest.Config{
	BaseURL:   "https://ingest.example.com",
	AppID:     config.AppID,
	AppSecret: config.AppSecret,
	HTTPClient: &http.Client{
		Timeout: 30 * time.Second,
	},
})
if err != nil {
	return err
}
```

SDK 会用 Basic AppID/AppSecret 获取 Bearer Token，并发请求共享缓存；Token 临近过期时提前刷新。受保护接口返回未授权后只刷新并重试一次。

所有网络方法都接收 `context.Context`。调用方负责设置符合自身请求生命周期的超时：

```go
ctx, cancel := context.WithTimeout(parent, 20*time.Second)
defer cancel()

profile, err := client.GetAppProfile(ctx)
```

## 创建并查询任务

写接口要求调用方传入稳定的幂等键。这个 key 应来自本地业务请求或回执 ID；同一业务请求跨进程重试时必须复用，不能每次随机生成。

```go
job, err := client.CreateJob(ctx, sdingest.CreateJobInput{
	CallbackURL: "http://callback.internal.example/task",
	ExternalRef: importReceipt.ID,
	Source: sdingest.Source{
		Mode:     "share_link",
		Provider: "baidupan",
		URL:      shareURL,
		Password: sharePassword,
	},
	FailPolicy: "all",
}, "asset-import:"+importReceipt.ID)
if err != nil {
	return err
}
```

查询权威状态：

```go
detail, err := client.GetJob(ctx, job.JobID)
if err != nil {
	return err
}
if detail.Terminal() {
	// succeeded、partial、failed、canceled
}

progress, err := client.GetJobProgress(ctx, job.JobID)
```

列表默认遵循服务端排序，最新创建的任务在前：

```go
page, err := client.ListJobs(ctx, sdingest.ListJobsInput{
	Page:        1,
	Limit:       20,
	ManifestMode: sdingest.ManifestModeManual,
	ExternalRef: importReceipt.ID,
})
```

## 创建并订阅打包任务

打包任务直接接收文件清单。每个文件可以来自不同 Target，最终 ZIP 单独写入 `OutputTargetID`：

```go
archive, err := client.CreateArchiveJob(ctx, sdingest.CreateArchiveJobInput{
	ExternalRef:    delivery.ID,
	CallbackURL:    "http://callback.internal.example/archive",
	ArchiveName:    "delivery.zip",
	RootDirectory:  "交付目录",
	OutputTargetID: outputTargetID,
	Entries: []sdingest.ArchiveEntryInput{
		{
			TargetID:    video.TargetID,
			SourcePath:  video.Path,
			ArchivePath: "视频/main.mp4",
			Size:        video.Size,
		},
		{
			TargetID:    subtitle.TargetID,
			SourcePath:  subtitle.Path,
			ArchivePath: "字幕/main.srt",
			Size:        subtitle.Size,
		},
	},
}, "archive-create:"+delivery.ID)
if err != nil {
	return err
}
```

清单最多 10,000 项。创建接口请求体上限由 SDIngest 服务端控制；调用方应提交稳定的相对路径，不要把临时下载 URL 当作 `source_path`。

详情、清单与回调记录分别查询：

```go
detail, err := client.GetArchiveJob(ctx, archive.JobID)
manifest, err := client.GetArchiveManifest(ctx, archive.JobID, 1, 100)
logs, err := client.ListArchiveCallbackLogs(ctx, archive.JobID, 1, 20)
```

需要实时进度时直接订阅 SDK 的鉴权 SSE，不需要自行获取 Token 或拼接 URL：

```go
err = client.StreamArchiveJobEvents(ctx, archive.JobID, func(event sdingest.ArchiveJobEvent) error {
	switch event.Event {
	case "snapshot":
		log.Printf("archive=%s progress=%d%% upload=%d%%", event.Job.JobID, event.Job.ProgressPercent, event.Job.UploadProgressPercent)
	case "complete":
		log.Printf("archive=%s status=%s", event.Job.JobID, event.Job.Status)
	}
	return nil
})
```

SSE 是实时展示通道。断线或进程重启后仍需调用 `GetArchiveJob` 校准权威状态；失败任务可用稳定幂等键重试：

```go
archive, err = client.RetryArchiveJob(ctx, archive.JobID, "archive-retry:"+retryReceipt.ID)
```

成功后获取 ZIP 临时下载地址：

```go
access, err := client.GetArchiveAccess(ctx, archive.JobID, 15*time.Minute)
if err != nil {
	return err
}
_ = access.URL
_ = access.TargetID
_ = access.Path
```

打包任务只投递 `archive.succeeded` 或 `archive.failed`。接收方可以把请求体解码为 `sdingest.ArchiveCallbackEnvelope`，并以 `event_id` 做幂等；任务级 `callback_url` 不发送签名头。

## Manifest 与产物回执

默认自动模式不需要调用方确认：

```go
manifest, err := client.GetJobManifest(ctx, job.JobID, 1, 100)
if err != nil {
	return err
}

for _, item := range manifest.Items {
	if item.ArtifactID == "" {
		continue
	}
	// item.Path 是源相对路径；item.ArtifactPath 是目标存储对象路径。
	_ = item.TargetID
	_ = item.ArtifactPath
}

artifacts, err := client.ListArtifacts(ctx, sdingest.ListArtifactsInput{
	Page:  1,
	Limit: 100,
	JobID: job.JobID,
})
```

需要由业务方选择文件时，在创建任务时声明人工模式：

```go
job, err := client.CreateJob(ctx, sdingest.CreateJobInput{
	ExternalRef:  importReceipt.ID,
	ManifestMode: sdingest.ManifestModeManual,
	Source: sdingest.Source{
		Mode:     "share_link",
		Provider: "baidupan",
		URL:      shareURL,
		Password: sharePassword,
	},
}, "job-create:"+importReceipt.ID)
if err != nil {
	return err
}

manifestPage, err := client.GetJobManifest(ctx, job.JobID, 1, 100)
if err != nil {
	return err
}

selectedItemIDs := []string{manifestPage.Items[0].ItemID}
job, err = client.ConfirmJobManifestSelection(ctx, sdingest.ConfirmJobManifestInput{
	JobID:    job.JobID,
	Revision: manifestPage.Manifest.Revision,
	Hash:     manifestPage.Manifest.Hash,
	ItemIDs:  selectedItemIDs,
}, "manifest-confirm:"+importReceipt.ID)
```

实际接入时应等待 Manifest 状态变为 `ready`，并读取完所需分页后再提交选择。只能选择当前 Manifest 中 `selected=true` 的候选项；过滤掉的文件不能被人工重新选入。

如果调用方需要用签名 URL 拉取对象：

```go
access, err := client.GetArtifactAccess(ctx, artifact.ArtifactID, 15*time.Minute)
```

调用方已经保存 `target_id + artifact_path` 时，可以批量换取临时地址，无需自行持有目标桶凭据：

```go
accessList, err := client.GetArtifactAccessBatch(ctx, []sdingest.ArtifactAccessLocator{
	{TargetID: item.TargetID, Path: item.ArtifactPath},
}, 15*time.Minute)
if err != nil {
	return err
}
```

批量接口一次最多提交 100 项，返回顺序与输入顺序一致。`ArtifactAccess` 同时保留 `TargetID` 和 `Path`，便于调用方关联本地记录。

只有在调用方自己的事务已经成功落库后再确认回执：

```go
_, err = client.AcknowledgeArtifact(
	ctx,
	artifact.ArtifactID,
	"artifact-ack:"+localReceipt.ID,
)
```

本地处理失败时不要提前 Ack。重试 Ack 必须复用原幂等键。

## 业务错误

SDIngest 即使在部分业务失败场景也可能返回 HTTP 200，因此 SDK 同时检查 HTTP 状态和响应 envelope。错误可解析为 `*sdingest.APIError`：

```go
var apiErr *sdingest.APIError
if errors.As(err, &apiErr) {
	if sdingest.IsSubCode(err, "IDEMPOTENCY_HASH_CONFLICT") {
		return err
	}
	if apiErr.Retryable() {
		// 结合 RetryAfter 和本地任务调度决定何时重试。
	}
}
```

不要根据中文 `Message` 做程序分支。应使用稳定的 `SubCode`；任务运行中的业务错误使用 `Job.ErrorCode`。

## 旧 Callback 配置与验签（过渡能力）

以下 Callback 配置实体与签名方法仅用于仍在使用旧回调配置的调用方。新建搬运和打包任务应直接传 `callback_url`，任务级回调不要求创建 Callback 配置，也不发送签名头。

Callback 可以通过 SDK 创建、查询、旋转密钥和启停。创建与旋转只在响应中返回一次 signing secret，调用方应立即加密保存；所有写操作仍使用稳定幂等键：

```go
callback, err := client.CreateCallback(ctx, sdingest.CreateCallbackInput{
	Name:   "SDIngest job events",
	URL:    "https://consumer.example.com/callbacks/sdingest",
	Events: []string{sdingest.CallbackEventSucceeded, sdingest.CallbackEventFailed},
}, "callback-create:"+integration.ID)
if err != nil {
	return err
}

rotated, err := client.RotateCallbackSecret(
	ctx,
	callback.CallbackID,
	"callback-rotate:"+rotation.ID,
)

err = client.ReplayCallback(ctx, eventID, "callback-replay:"+replayReceipt.ID)
```

不要记录 `callback.SigningSecret` 或 `rotated.SigningSecret`。

HTTP 接收层应保留原始 body，再交给 SDK 验签和解码：

```go
body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 1<<20))
if err != nil {
	return err
}

event, err := sdingest.VerifyAndDecodeCallback(
	request.Header,
	body,
	callbackSigningSecret,
	time.Now(),
	5*time.Minute,
)
if err != nil {
	return err
}
```

调用方应以 `event.EventID` 去重。Callback 只是状态变化提示；收到事件后仍通过 `GetJob`、`GetJobProgress` 或 `ListArtifacts` 获取权威状态，不依赖事件顺序推导最终结果。`ReplayCallback` 会创建一个新的事件 ID 并重新投递同类事件，不能把重放当作原事件的重复响应。

Callback 的 `Data.TargetID` 表示任务目标存储；item 事件还会在 `Data.Item` 中携带 `ArtifactID`、`TargetID` 和 `ArtifactPath`。这些字段用于定位产物，最终状态仍以查询接口为准。

## 敏感信息

不得记录以下内容：

- AppSecret 和 Access Token
- 分享链接及提取码
- Callback Signing Secret 和签名原文
- Artifact 签名访问 URL

SDK 的结构化错误不拼接原始响应 body，也不会在错误中返回请求体。
