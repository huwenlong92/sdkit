# SDIngest SDK 模块

## 作用

`pkg/sdingest` 是 SDIngest 版本化 OpenAPI 的公开 Go 客户端。它负责：

- App client-credentials Token 获取、并发缓存和提前刷新
- `/v1` HTTP 请求与 `{err_code,sub_code,msg,data}` envelope 解析
- HTTP 状态、业务错误、`Retry-After` 和 request ID 投影
- 写接口 `Idempotency-Key` 强制传入
- App、Job、Progress、Manifest、Artifact、Archive Job/Entry/Access 的公开 DTO
- Archive REST 查询、可靠写操作与带鉴权的 SSE 订阅
- Callback HMAC-SHA256 验签、时间窗口检查和原始 body 解码

内部 HTTP transport 复用 `pkg/request`：request 负责 URL、header、JSON body、HTTP 调用和响应大小限制；本包保留 Token、401 后唯一一次刷新重放、envelope 和安全错误语义。该 transport 固定 `MaxAttempts=1`，不会叠加通用自动重试。

`BaseURL` 默认使用服务根地址，例如 `https://ingest.example.com`；SDK 方法只追加 `/v1/...`，接口路径不包含 `/api`。如果部署方自行配置了反向代理前缀，可以把该前缀放进 `BaseURL`，但它不属于 SDIngest 的接口契约。

它不负责：

- 保存调用方业务回执或领域状态
- 自动生成跨进程稳定的幂等键
- 自动消费重复、乱序或丢失的 Callback
- 连接 SDIngest PostgreSQL、Redis、Queue、对象存储或 Worker
- 将 Artifact 映射为调用方业务模型

## 包路径

```go
import "github.com/huwenlong92/sdkit/pkg/sdingest"
```

## Client

```go
type Config struct {
	BaseURL          string
	AppID            string
	AppSecret        string
	HTTPClient       HTTPDoer
	TokenSkew        time.Duration
	MaxResponseBytes int64
	UserAgent        string
	Clock            func() time.Time
}

func NewClient(config Config) (*Client, error)
```

默认值：

- `HTTPClient`：`http.DefaultClient`
- Token 提前刷新窗口：30 秒
- 最大响应体：8 MiB
- User-Agent：`sdkit-sdingest-go/1.0`

生产调用方应显式提供带 timeout 和受控 transport 的 HTTP client。

## 对外方法

身份与应用：

- `Authenticate(ctx) (Token, error)`
- `GetAppProfile(ctx) (AppProfile, error)`

任务：

- `CreateJob(ctx, input, idempotencyKey) (Job, error)`
- `ListJobs(ctx, input) (Page[Job], error)`
- `GetJob(ctx, jobID) (Job, error)`
- `GetJobProgress(ctx, jobID) (JobProgress, error)`
- `GetItemProgress(ctx, jobID, itemID) (ItemProgress, error)`
- `GetJobManifest(ctx, jobID, page, limit) (ManifestPage, error)`
- `ConfirmJobManifest(ctx, jobID, revision, hash, idempotencyKey) (Job, error)`
- `ConfirmJobManifestSelection(ctx, input, idempotencyKey) (Job, error)`
- `CancelJob(ctx, jobID, idempotencyKey) (Job, error)`
- `RetryJob(ctx, jobID, idempotencyKey) (Job, error)`

打包任务：

- `CreateArchiveJob(ctx, input, idempotencyKey) (ArchiveJob, error)`
- `ListArchiveJobs(ctx, input) (Page[ArchiveJob], error)`
- `GetArchiveJob(ctx, jobID) (ArchiveJob, error)`
- `GetArchiveManifest(ctx, jobID, page, limit) (ArchiveManifestPage, error)`
- `ListArchiveCallbackLogs(ctx, jobID, page, limit) (Page[ArchiveCallbackLog], error)`
- `GetArchiveCallbackLog(ctx, jobID, eventID, attemptNo) (ArchiveCallbackLogDetail, error)`
- `GetArchiveAccess(ctx, jobID, ttl) (ArchiveAccess, error)`
- `RetryArchiveJob(ctx, jobID, idempotencyKey) (ArchiveJob, error)`
- `StreamArchiveJobEvents(ctx, jobID, handler) error`

产物：

- `ListArtifacts(ctx, input) (Page[Artifact], error)`
- `GetArtifact(ctx, artifactID) (Artifact, error)`
- `GetArtifactAccess(ctx, artifactID, ttl) (ArtifactAccess, error)`
- `GetArtifactAccessBatch(ctx, items, ttl) ([]ArtifactAccess, error)`
- `AcknowledgeArtifact(ctx, artifactID, idempotencyKey) (Artifact, error)`

Callback：

- `ListCallbacks(ctx, input) (Page[Callback], error)`
- `GetCallback(ctx, callbackID) (Callback, error)`
- `ListCallbackLogs(ctx, input) (Page[CallbackLog], error)`
- `CreateCallback(ctx, input, idempotencyKey) (CallbackCredential, error)`
- `RotateCallbackSecret(ctx, callbackID, idempotencyKey) (CallbackCredential, error)`
- `EnableCallback(ctx, callbackID, idempotencyKey) error`
- `DisableCallback(ctx, callbackID, idempotencyKey) error`
- `ReplayCallback(ctx, eventID, idempotencyKey) error`
- `VerifyAndDecodeCallback(header, body, signingSecret, now, tolerance) (CallbackEnvelope, error)`

## 鉴权并发语义

Token 缓存由 Client 内部 mutex 保护。缓存缺失或临近过期时，只允许一个 goroutine 获取新 Token；其他请求等待同一刷新结果。受保护接口返回 HTTP 401 或 envelope `err_code=401` 时：

1. 只在缓存仍是当前失败 Token 时清空，避免覆盖其他 goroutine 已刷新的 Token。
2. 重新获取 Token。
3. 原请求只重放一次。

SDK 不对普通网络错误或业务错误自动循环重试。调用方应在自己的任务调度中根据操作幂等性、`APIError.Retryable()` 和 `RetryAfter` 决定重试。

## Manifest 确认模式

`CreateJobInput.ManifestMode` 支持 `sdingest.ManifestModeAuto` 和 `sdingest.ManifestModeManual`。留空等同于 `auto`：枚举和过滤完成后直接进入下载。`manual` 会停在 `waiting / await_confirm`，调用方读取 Manifest 后必须通过 `ConfirmJobManifestSelection` 提交当前 `revision`、`hash` 和明确选择的 `item_ids`。

`CreateJobInput.CallbackURL` 可随任务直接传入 HTTP 或 HTTPS 回调地址，不需要预先创建回调配置，也不发送签名头。服务端会保留任务与每次投递使用的 URL 快照，便于查询、重试和审计。

旧方法 `ConfirmJobManifest` 保留用于已有自动模式或历史调用兼容；人工模式必须使用带选择项的新方法。确认请求属于幂等写操作，相同业务确认必须复用同一个 `Idempotency-Key`。

## Archive 打包任务

Archive 是独立任务域，不是搬运 Job 的附加阶段。`CreateArchiveJobInput` 直接携带完整文件清单：

- 每个 `ArchiveEntryInput` 使用 `target_id + source_path` 定位源对象。
- `archive_path` 决定文件在 ZIP 内的相对路径。
- `root_directory` 可选；为空时不额外包裹顶层目录。
- `output_target_id` 决定最终 ZIP 写入的目标存储。
- `callback_url` 是当前任务的回调地址快照，不依赖预创建的 Callback 配置。

创建和重试是幂等写操作，必须使用稳定的 `Idempotency-Key`。Manifest 与回调日志均独立分页；任务详情不会内嵌上千条 Entry 或完整回调 payload。

`StreamArchiveJobEvents` 使用 Client 的 Bearer Token 建立 SSE，请求遇到 401 时沿用普通 API 的规则，只刷新 Token 并重连一次。Handler 会收到 `snapshot` 与 `complete`；方法在 `complete`、handler 返回错误或 context 取消时退出。网络在终态前关闭会返回 `io.ErrUnexpectedEOF`，由调用方决定是否重新订阅或回退到 `GetArchiveJob` 查询。

终态成功后使用 `GetArchiveAccess` 获取有时效的 ZIP 下载地址。打包回调只发送 `archive.succeeded` 或 `archive.failed`；`ArchiveCallbackEnvelope` 与 `ArchiveCallbackEnvelopeData` 提供类型化解码字段。

## 错误契约

`APIError` 同时保留：

- `StatusCode`：真实 HTTP 状态
- `Code`：envelope `err_code`
- `SubCode`：稳定机器错误码
- `Message`：面向人的消息，不应用于分支判断
- `Data`：可选结构化数据
- `RequestID`：响应 request ID
- `RetryAfter`：429 等响应声明的等待时长

空响应、非 JSON、超出大小限制或缺失成功 `data` 返回 `ProtocolError`。Transport、context 取消和超时错误保留原始 error chain。

SDIngest 服务端必须在错误 envelope 中保留 `sub_code`。只返回数字 code 和本地化 message 无法支撑第三方稳定集成。

## Callback 协议

SDK验证以下 Header：

- `X-SDIngest-Event-ID`
- `X-SDIngest-Timestamp`
- `X-SDIngest-Signature`

签名内容：

```text
v1=hex(HMAC-SHA256(signing_secret, unix_timestamp + "." + raw_body))
```

验签使用常量时间比较，并检查时间戳与调用方传入 `now` 的差值不超过 tolerance。验签后还会确认 Header Event ID 与 body 一致，以及 JobID、revision 等最小字段有效。

SDK 不维护事件去重表。调用方必须持久化 Event ID，并在收到提示后重新查询 GET 权威状态。`ReplayCallback` 仅允许重放当前 App 自己的事件，且服务端会为重放生成新的 Event ID。

Manifest item、Artifact access 和 Callback item 都保留 `target_id` 与产物路径。`path` 表示源文件相对路径，`artifact_path` 表示目标存储对象路径；调用方不得混用。没有目标存储配置读取能力的调用方，可以把 `target_id + artifact_path` 交给 `GetArtifactAccessBatch` 批量换取短期访问地址。

## 更新记录

- 2026-09-30：新增 Archive 打包任务 typed client，覆盖创建、列表、详情、Manifest、回调日志、下载地址、重试和带鉴权 SSE。
- 2026-09-28：补齐 Manifest、Callback 和 Artifact access 的目标存储定位字段，新增按 `target_id + path` 批量获取临时地址。
- 2026-08-25：新增 Callback 人工重放，并为任务列表增加 Manifest 模式筛选。
- 2026-08-25：新增首版 typed Client，覆盖 App、Job、Progress、Manifest、Artifact、业务错误、Token 缓存、Callback 管理与验签。
