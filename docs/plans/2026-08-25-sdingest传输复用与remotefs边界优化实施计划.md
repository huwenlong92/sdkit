# pkg/sdingest 传输复用与 pkg/remotefs 边界优化实施计划

> 日期：2026-08-25  
> 状态：外部条件阻塞（除双真实账号并行外已完成；连续三次真实审计均只有 1 个启用有凭据账号、0 个已绑定 Provider UID，条件化验收代码已就绪）  
> 范围：`/Users/huwenlong/data/lab/sdkit`，以及必要的 `/Users/huwenlong/data/sdkit/sdingest/sdingest` 消费方迁移  
> 不包含：DreamIP 代码修改、后台页面需求、任务产品逻辑重写、发布 tag、commit 或 push

## 1. 背景

本计划处理两组已经确认的边界问题：

1. `pkg/sdingest` 重复实现 URL、Header、认证请求、JSON Body、响应读取和 Body 上限，应复用现有 `pkg/request`，自身只保留 SDIngest 协议、Token、错误和回调语义。
2. `pkg/remotefs` 当前存在类型擦除、身份重复、Provider 信息泄漏、下载提交不一致、同账号 Session 锁失效、CLI 凭据暴露、Local 路径竞态和错误分类过宽等问题，需要先修正确性和安全问题，再收紧公共契约。

本轮目标不是继续增加抽象层，而是删除无效抽象、明确所有权，并让公共契约能够被 SDIngest 和后续应用稳定使用。

## 2. 强约束

- 不在 sdkit 中新增 runtime capability、全局默认实例、账号池、任务调度器或业务 facade。
- 不把 GORM、Gin、Asynq、SDIngest Job/Stage/Item 模型带入 `pkg/sdingest` 或 `pkg/remotefs`。
- 不通过 `config any`、`map[string]any` 或 Provider Metadata 继续扩展公共 API。
- 不让 `pkg/request` 自动重试 SDIngest API；401 Token 刷新和唯一一次重放继续由 `pkg/sdingest` 控制。
- 不在公共错误中暴露原始 API Body、BaiduPCS-Go 原始输出、BDUSS、STOKEN、分享口令或完整命令参数。
- `context.Context` 必须由调用方传入并完整透传；nil context 返回明确错误，不静默替换为 `context.Background()`。
- 测试继续放在仓库根目录 `tests/`，不在 `pkg/` 内新增测试文件。
- 修改公共契约时，同一阶段同步迁移 SDIngest；不长期保留新旧两套适配层。
- 不修改 DreamIP。本计划完成后，DreamIP 只通过未来发布的 sdkit SDK 使用这些能力。

## 3. 已确认的影响面

| 目标 | 风险 | 已知影响 |
| --- | --- | --- |
| `pkg/sdingest.Client.do` | HIGH | 18 个 SDK 方法直接依赖，适合集中替换内部 transport，但不能同时改变公开方法和错误契约。 |
| `remotefs.FileSystem` | MEDIUM（sdkit 内）/跨仓库较高 | sdkit 测试与 SDIngest Worker、Baidu Account Runtime 直接使用。 |
| `remotefs.Registry` | LOW（sdkit 内） | 主要生产消费者是 SDIngest RemoteFS facade，适合协调删除。 |
| `Reference`、`Entry` | 跨仓库高影响 | SDIngest Manifest、Stage、Download、Cleanup 都读取这些字段，必须一次协调迁移。 |
| `baidupan.StageShare` | 高正确性风险 | 依赖 CLI 远端 cwd，当前锁只对单个实例有效。 |
| 两个 Driver 的 `Download` | 高数据风险 | 存在并发覆盖、提交成功后返回失败和缺少结果完整性校验的问题。 |

执行任何现有 symbol 修改前，仍需按仓库规则重新运行 GitNexus upstream impact；如果结果为 HIGH 或 CRITICAL，先报告影响再修改。

### 3.1 与 SDIngest 联动执行规则

本计划不能采用“先把 sdkit 全部改完，最后再让 SDIngest 适配”的方式执行。对应的消费方计划是：

`/Users/huwenlong/data/sdkit/sdingest/sdingest/plans/2026-08-25-sdkit-remotefs联动迁移与真实任务回归计划.md`

每个阶段必须同时满足：

1. sdkit 先补公共契约失败测试并完成最小实现。
2. 同一阶段立即迁移 SDIngest 对应消费点。
3. 运行 SDIngest 相关包测试，确认任务状态、错误分类、账号健康、重试和 Cleanup 没有漂移。
4. 阶段收口时同时运行 sdkit 与 SDIngest 门禁，不能只凭 core 单测勾选完成。

联动关系：

| sdkit 修改 | SDIngest 同阶段验证 |
| --- | --- |
| `pkg/sdingest` 复用 `pkg/request` | 以真实 BaseURL 调用本地 OpenAPI，覆盖 Token、401、幂等、错误 Envelope 和 Callback。 |
| Download 提交与 Progress 语义 | Worker 只在 Download 成功返回后写 Completed；Sink/数据库错误不污染 Provider 健康。 |
| ID/Version 条件操作 | Manifest 校验、Download 和 Cleanup 使用同一来源快照，路径复用时不误删新对象。 |
| Baidu Session/凭据轮换 | 同账号多任务、不同账号并行、CredVer 变化和 ExpectedProviderUID 真实验证。 |
| RemoteFS 公共类型精简 | Worker、infra capability、Baidu Account Runtime、Admin/API projection 一次迁移。 |
| 错误体系调整 | SDIngest 分别映射永久失败、Provider 等待、来源变化、目标冲突和本地 Sink 错误。 |

## 4. 目标边界

### 4.1 `pkg/sdingest`

`pkg/request` 负责：

- BaseURL、Path 和 Query。
- Header、Basic Auth、Bearer Token 和 JSON 请求体。
- `http.Client` 调用、响应 Body 生命周期和大小限制。
- 网络层错误和 context 取消。

`pkg/sdingest` 负责：

- BaseURL 的 SDIngest 安全校验。
- AppID、AppSecret 和 Token 生命周期。
- 401 后清除 Token、重新认证并重放一次。
- `{err_code, sub_code, msg, data}` Envelope 解码。
- `APIError`、`ProtocolError`、RequestID、RetryAfter 和幂等键规则。
- Callback 签名、时间窗口和重放保护。
- Endpoint DTO 和公开 SDK 方法。

不得直接使用会向调用者保留原始响应 Body 的通用 DecodeError；SDIngest Envelope 必须在 `pkg/sdingest` 内安全解码。

### 4.2 `pkg/remotefs`

公共层只表达：

- 已打开文件系统中的文件引用。
- Stat/List 和可选的 Download/Open/Remove 能力。
- Provider 无关的 Entry、版本、校验值、容量和错误语义。
- 通用 Walk 和进度事件。

Driver 层负责：

- Provider 配置、认证、Session 和命令/API Transport。
- Provider 输出解析与严格校验。
- Provider 错误到公共错误的映射。
- 自己引入的内部状态锁，例如 BaiduPCS-Go cwd。

消费方负责：

- 账号选择、权重、冷却、任务粘性和业务租约。
- Job/Stage/Item 持久化。
- Workspace、目标存储上传和任务回调。
- 将公共模型映射成 Admin/API 各自需要的响应字段。

## 5. 目标公共契约草案

实施时允许根据测试结果微调命名，但不得重新引入已删除的重复身份和 `any` 配置。

```go
type Reference struct {
    ID   string
    Path string
}

type Entry struct {
    Reference Reference
    Name      string
    Type      EntryType
    Size      int64
    ModTime   *time.Time
    Version   string
    Checksum  *Checksum
}

type FileSystem interface {
    Driver() string
    Stat(ctx context.Context, ref Reference) (Entry, error)
    List(ctx context.Context, dir Reference, opts ListOptions) (ListPage, error)
    Close() error
}

type Downloader interface {
    Download(ctx context.Context, req DownloadRequest, sink ProgressSink) (DownloadResult, error)
}

type Remover interface {
    Remove(ctx context.Context, req RemoveRequest) error
}
```

约束：

- `Reference` 不再重复携带已经由 FileSystem 绑定的 Driver 和 Connection。
- `Entry.Path` 删除，统一读取 `Entry.Reference.Path`。
- `Metadata["app_id"]` 改为 Provider 无关的 `Entry.Version`。
- `Reference.ID` 如果保留，就必须在条件下载和条件删除中真正参与校验；不能继续作为装饰字段。
- `DownloadRequest` 和 `RemoveRequest` 增加可选的 `ExpectedVersion`，用于防止路径复用后操作到新对象。
- `DownloadResult` 只返回最终本地路径、实际字节数和已验证 Checksum，不重复返回请求 Reference。
- `Quota` 只保存 Total 和 Used；Free 由方法或 API DTO 计算。
- Provider 原始扩展字段不得通过通用 `Metadata` 泄漏到业务层。
- `Remove` 首版只处理一个对象；确需批量删除时另行定义带逐项结果的 `RemoveMany`。

## 6. 分阶段实施任务

### 阶段 0：基线、索引和测试清单

- [x] 检查 sdkit 与 SDIngest 当前 Git 状态，记录并避开用户已有改动。
- [x] 确认 GitNexus 的 sdkit 与 sdingest 索引新鲜；有 stale 警告时重新索引。
- [x] 对准备修改的 `Client.do`、`FileSystem`、`Download`、`StageShare`、`Reference`、`Entry`、`Walk`、`Registry.Open` 分别运行 upstream impact。
- [x] 运行并记录基线：`go test ./tests/pkg/request/... ./tests/pkg/sdingest/... ./tests/pkg/remotefs/...`。
- [x] 运行 sdkit 全量基线：`go test ./...`。
- [x] 在 SDIngest 运行基线：`go test ./...` 和 `make build-all`。
- [x] 列出 SDIngest 中所有 `Reference`、`Entry.Path`、`Metadata["app_id"]`、`Registry.Open`、`FeaturesOf` 和 `ShareResolver` 消费点。

验收：基线结果和既有失败有明确记录，后续能区分本次新增问题。

### 阶段 1：先补失败契约测试

#### `pkg/sdingest`

- [x] 覆盖成功 Envelope、业务错误 Envelope、非 JSON、空 Body、超大 Body 和非 2xx Envelope。
- [x] 覆盖 Basic Auth 换 Token、Bearer Token、Token 缓存和并发只刷新一次。
- [x] 覆盖 401 后只刷新并重放一次，第二次 401 不循环。
- [x] 覆盖 RequestID、RetryAfter、幂等键和 context 取消。
- [x] 断言 `ProtocolError` 和错误字符串不包含原始响应 Body。

#### `pkg/remotefs`

- [x] 两个并发 `OverwriteDeny` 下载同一目标，只允许一个成功，另一个返回目标冲突，成功文件不能被后写覆盖。
- [x] 目标文件提交后，Completed 通知失败不能把已提交下载变成可重试失败。
- [x] Progress Sink 在提交前失败时，必须原样返回；Baidu 账号不得被标记为 `ErrTemporary`。
- [x] `.partial` 并发保留不能相互覆盖；Keep 失败必须有可判断结果。
- [x] 下载前后 Size/ID/Version 不一致时返回 `ErrSourceChanged` 或 `ErrIntegrity`。
- [x] 条件 Remove 的 ID/Version 不匹配时不得删除路径上的新对象。
- [x] 同账号两个独立 FileSystem 实例并发 StageShare 必须串行；不同账号仍可并行。
- [x] CredentialRevision 变化后不能继续使用旧 Session。
- [x] ExpectedProviderUID 与 `who` 返回身份不一致时拒绝操作。
- [x] 文件名包含“网络错误”“未登录”“操作失败”时，成功 List 不能被误判为 Provider 错误。
- [x] Baidu List/Stat 拒绝非绝对路径、越级路径、非直接子项、非法名称和不完整时间字段。
- [x] Local 拒绝目录、FIFO、Socket 等非普通文件的 Open/Download。
- [x] Local 符号链接替换不能逃逸 Root；Remove 不得跟随链接删除目标。
- [x] Local Download 默认不得把目标写回来源 Root。
- [x] nil context、非法枚举、非法 PageSize/Concurrency/OutputLimit 返回明确参数错误。
- [x] Walk 覆盖 A→B→A Cursor 循环、目录循环、root 是否访问和目录 prune 语义。

验收：新增用例在旧实现上能够稳定暴露问题，测试不依赖真实账号和真实 Secret。

### 阶段 2：`pkg/sdingest` 复用 `pkg/request`

- [x] `Client` 内部持有 `*request.Client`，删除重复的 URL、Header、Body 读取和网络调用代码。
- [x] `NewClient` 将 BaseURL、HTTP Client、Accept、User-Agent 和 MaxBodyBytes 配置到 `request.Client`。
- [x] request 层保持 `MaxAttempts=1`，不启用通用重试。
- [x] 配置 transport 接受所有 HTTP Status，由 `pkg/sdingest` 统一解析 Envelope。
- [x] `pkg/sdingest` 自己执行 Envelope JSON 解码，不向外保留原始 Body。
- [x] 将 `request.ErrBodyTooLarge` 转成安全的 `ProtocolError`，保留 Status、Header 和 RequestID。
- [x] 保留 Token Mutex、缓存、Skew、401 失效和唯一一次重放行为。
- [x] 收紧 `Config.HTTPClient` 为实际使用的 `*http.Client`；如现有公开消费者需要自定义 Doer，先根据 impact 决定是否保留兼容入口。
- [x] 默认超时行为不得静默改变；需要改变时单独记录为契约变更。
- [x] 更新 `docs/modules/sdingest.md` 和 `docs/usage/sdingest.md`。
- [x] 同阶段运行 SDIngest 的 `tests/api`、`tests/openapi` 和 API contract tests；使用 SDK 对本地 API 做一次真实 Token、Job 查询和错误响应验证。

验收：公开 SDK 方法签名、Endpoint Path、DTO、APIError 和 ProtocolError 行为保持一致；新增及原有测试全部通过。

### 阶段 3：下载事务、目标文件和错误语义

- [x] 明确下载状态：Prepare → Transfer → Verify → Commit → Return Success。
- [x] 为 `OverwriteDeny` 实现真正的原子 no-replace 提交，不再使用“Stat 后 Rename”。
- [x] 为 `OverwriteReplace` 保留显式原子替换；目录、特殊文件和权限错误分别映射。
- [x] 两个 Driver 统一 `.partial` 所有权、清理和保留语义；`Keep` 明确为保留失败现场，不宣称支持续传。
- [x] Driver 不再发布 `ProgressCompleted`；调用方在 Download 成功返回后发布完成事件。
- [x] Progress 必须满足非负、单调、Transferred ≤ Total（Total 已知时）和有效速率。
- [x] Baidu Sink 错误与 Runner/Provider 错误分离，原样向上传递。
- [x] 新增 `ErrInvalidArgument`、`ErrInvalidOption`、`ErrNotDirectory`、`ErrNotRegularFile`、`ErrProtocol`、`ErrIntegrity`、`ErrSourceChanged` 等必要错误，停止用 `ErrTemporary` 包装确定性问题。
- [x] `RetryAfter` 要么由实际 Provider 限流解析并赋值，要么从首版 Error 中删除。
- [x] `SafeSummary` 按 UTF-8 边界截断，并清除 ANSI/控制字符。
- [x] 未识别 Provider 输出使用固定安全摘要；原始文件名和完整命令输出不进入公共错误。
- [x] 同阶段迁移 SDIngest Worker 的 Download/Progress/Error 映射，并验证“文件已提交但状态失败”和“Sink 错误导致账号冷却”不会出现。

验收：并发覆盖、提交后假失败、错误重试分类和数据完整性测试全部通过。

### 阶段 4：Baidu Runtime 与 Session 边界

- [x] 将现有 `baidupan.Config` 拆成进程级 `RuntimeConfig` 和账号级 `SessionConfig`。
- [x] Runtime 只负责 BinaryPath、版本范围、ConfigRoot、OutputLimit 和 Session 创建。
- [x] SessionConfig 只负责 SessionKey、ExpectedProviderUID、CredentialRevision、BDUSS、STOKEN、下载模式、并发和 Remove 权限。
- [x] Binary 版本在 Runtime 构造时校验并缓存；HealthCheck 可以显式重新验证。
- [x] 调研 BaiduPCS-Go 是否能避免 `cd`，优先使用显式目标目录参数。
- [x] 如果无法避免 cwd，在 Driver 内实现同 Session 的跨实例互斥；有多进程共享 ConfigRoot 时使用同目录锁文件保护 login/cd/transfer 序列。
- [x] 身份验证解析出 UID/用户名，不再只返回 bool；UID 不符合 ExpectedProviderUID 时返回身份不匹配错误。
- [x] CredentialRevision 改变时创建新 Session 或强制重新登录，不能继续接受旧 `who` 状态。
- [x] 在第一次执行 CLI 前完成 ConfigRoot 和 SessionDir 的 Lstat、权限及符号链接校验。
- [x] Login、secureSession 和其他可能写配置的操作使用同一个 Session 锁，避免并发 chmod/写配置。
- [x] 凭据和分享口令不得出现在 argv；优先 stdin、受保护输入文件或 Provider API。
- [x] Runner 使用 clean environment，只注入运行所需变量；不继承服务进程全部 Secret。
- [x] BinaryPath 要求绝对、可执行的普通文件；不自动联网安装或升级。
- [x] 为 DownloadConcurrency、OutputLimit 设置合理上限，超过上限返回配置错误。
- [x] 同阶段在 SDIngest 使用真实账号 Lease 创建同账号并发任务，确认 FileSystem 多实例不会串 cwd；CredVer 变化后旧 Session 不再被使用。

阻断条件：如果目标 BaiduPCS-Go 版本无法避免在 argv 暴露 BDUSS/STOKEN 或分享口令，不能宣称生产安全；需要先决定直接 Provider API 或受控 wrapper 方案。

验收：同账号并发、凭据轮换、错账号、Session 目录安全和 Secret 扫描全部通过。

### 阶段 5：Local Driver 文件系统安全

- [x] 使用 Go 1.25 `os.Root` 或等价的目录句柄方案替代 EvalSymlinks 后再操作的 TOCTOU 流程。
- [x] Stat/List/Open/Download/Remove 统一 no-escape、no-follow 规则。
- [x] 明确内部符号链接策略：首版优先拒绝符号链接和特殊文件，不把它们伪装成普通文件或目录。
- [x] `Open` 只允许普通文件，并明确 context 是只约束打开动作还是约束整个 Reader 生命周期。
- [x] 如果 Reader 生命周期绑定 context，确保取消 goroutine 可回收且 Close 幂等。
- [x] Remove 删除引用自身，不得因路径解析而删除符号链接目标。
- [x] 默认拒绝 Download Destination 位于来源 Root；如确有同 Root 写入需求，必须新增显式危险开关和独立测试。
- [x] 下载完成后验证 BytesWritten 和源文件前后状态，来源变化时不提交目标文件。
- [x] 目录 Size 统一为 0；无法确认的 ModTime 返回 nil。
- [x] 同阶段运行 SDIngest mounted/local source 回归，覆盖 Manifest、Download、取消、源变化和 Cleanup。

验收：路径替换、内部链接循环、特殊文件、写回来源和来源变化测试全部通过；Local contract suite 继续通过。

### 阶段 6：精简 RemoteFS 公共契约

- [x] 精简 `Reference`，移除 Driver/Connection；FileSystem 在构造时已经绑定身份。
- [x] 删除 `Entry.Path`，只保留 `Entry.Reference.Path`。
- [x] 将 `Metadata["app_id"]` 替换为 `Entry.Version`，删除无明确消费者的 Metadata。
- [x] 将 `ModTime` 改为可选值，修正 SDIngest Manifest 映射。
- [x] 将 `Download` 从基础 FileSystem 移到 `Downloader` 可选接口。
- [x] 将 `Remove(ctx, refs...)` 收窄为单对象条件删除。
- [x] 删除无生产消费者的 `FeaturesOf`；调用方直接断言所需小接口。
- [x] 删除语义错误的 `ShareResolver`；在 `baidupan` 中提供纯 `ParseShare`，StageShare 继续接受经过验证的分享输入。
- [x] 拆出 Provider/CLI 专用错误，不让 Binary、Version、Share 错误污染最小文件系统错误集合。
- [x] 精简 Quota 的重复 Free 字段；API 层按自身响应需要计算。
- [x] 删除当前 `Registry.Open(..., config any)` 使用链。
- [x] Local 直接使用类型化构造；Baidu 通过类型化 Runtime/Session Factory 打开。
- [x] 不新增替代 Registry 的通用 Manager；只有真实 Driver 选择场景出现时，再设计类型安全注册机制。
- [x] 本阶段的公共类型修改和 SDIngest 消费点迁移作为一个不可拆分交付；任一仓库不能编译时不得结束阶段。

验收：`pkg/remotefs` 不存在 `config any`、重复身份字段和 Provider Metadata；两个 Driver 通过新的基础契约测试。

### 阶段 7：协调迁移 SDIngest

- [x] 更新 `app/infra/capability/remotefs`，删除 Registry 和 `Open(..., cfg any)`。
- [x] capability 只持有类型化的 Local 配置和 Baidu Runtime，不包装通用业务 Service。
- [x] 更新 Baidu Account RuntimeConfig，传入 ConfigKey、ProviderUID 和 CredVer 对应的 Session 信息。
- [x] 更新 StageShare、Enumerate、Download、Cleanup 和 Quota 调用。
- [x] Manifest 使用 `Entry.Reference.Path`、`Entry.Version` 和可选 ModTime。
- [x] Cleanup 使用 ID/Version 条件删除，不再只按 Path 删除。
- [x] Download 成功返回后，由 Worker 发布 Completed；Sink 错误不再污染 Provider 健康状态。
- [x] Admin/API Handler 不直接返回 remotefs 公共结构；各自按接口字段构造响应 DTO。
- [x] 删除 `FeaturesOf`、`ShareResolver`、旧 Reference 字段和 Metadata 消费点。
- [x] 不在 `app/admin/handler/ops` 范围内做无关修改。
- [x] 不修改 DreamIP。

验收：SDIngest `go test ./...` 和 `make build-all` 通过，不存在旧 API 适配层。

### 阶段 8：Walk、分页和大型目录

- [x] 明确 Walk 是否访问 Root；保持一种语义并写入文档和测试。
- [x] 将 Entry 过滤与目录 prune 分开，避免 Filter 语义含混。
- [x] 每个目录记录全部已见 Cursor，发现任意循环立即返回 `ErrProtocol`。
- [x] 使用稳定 ID/规范 Path 做目录 visited key，覆盖内部链接循环。
- [x] 明确 List Cursor 是 Provider snapshot cursor 还是 best-effort offset。
- [x] 当前 Local/Baidu offset cursor 至少绑定目录、SortBy 和 Order，跨查询复用返回参数错误。
- [x] 文档明确 Baidu `ls` 仍会读取完整目录；不得把本地切页描述成 Provider 侧分页优化。
- [x] 评估 Local 超大目录的内存和排序成本；没有真实需求前不引入有状态分页服务。

验收：Cursor 漂移和循环不会无限执行；文档不承诺实现做不到的 snapshot consistency。

### 阶段 9：文档、全量验证和真实场景

- [x] 更新 `docs/modules/request.md` 中与 SDIngest 复用相关的稳定边界。
- [x] 更新 `docs/modules/sdingest.md`、`docs/usage/sdingest.md`。
- [x] 更新 `docs/modules/remotefs.md`、`docs/usage/remotefs.md`。
- [x] 在模块文档记录 Session 锁、CredentialRevision、条件删除、下载提交点和 Progress 语义。
- [x] 检查文档示例不含 `Registry`、`config any`、旧 Reference 字段和 Metadata。
- [x] 运行 `go test ./tests/pkg/request/... ./tests/pkg/sdingest/... ./tests/pkg/remotefs/...`。
- [x] 运行 `go test -race ./tests/pkg/remotefs/...`。
- [x] 运行 sdkit `go test ./...`。
- [x] 运行 SDIngest `go test ./...` 和 `make build-all`。
- [x] 运行 `git diff --check`。
- [x] 运行 GitNexus `detect_changes`，确认只影响预期 SDK、Driver 和 SDIngest 消费流程。
- [x] 使用 opt-in 真实 Baidu 环境验证：有效分享、错误口令、失效分享、认证过期、下载取消、重复转存和清理保护。
- [x] 在 SDIngest 使用后台任务和 API 任务各完成至少一轮：单任务、多任务并发、同账号多任务、失败重试和取消。
- [ ] 使用两个不同真实账号完成并行任务验收；条件化真实子场景已就绪，但当前环境只有一个真实账号且没有已绑定 Provider UID，不使用隔离副本伪造通过。
- [x] 检查日志、错误、测试输出、进程 argv 和子进程环境不包含真实 Secret。

验收：单元测试、race、全量构建和真实场景都有结果记录；所有失败均能区分永久失败、Provider 可重试、调用方 Sink 错误、来源变化和目标冲突。

## 7. 必须新增的回归场景矩阵

| 场景 | 预期结果 |
| --- | --- |
| 两任务同时 `OverwriteDeny` 到同一路径 | 一个成功，一个目标冲突；成功文件内容不被覆盖。 |
| 目标提交后 Completed 发布失败 | Download 仍视为成功；通知错误由调用方单独处理。 |
| Sink 在传输中失败 | 下载终止并返回原始 Sink 错误；不标记 Provider 临时失败。 |
| 下载期间来源 ID/Version 变化 | 不提交目标文件，返回来源变化。 |
| Cleanup 时路径已被新对象复用 | ID/Version 不匹配，拒绝删除。 |
| 同账号两个独立实例同时 StageShare | cwd 序列不交叉。 |
| 不同账号同时 StageShare/Download | 能够并行，Session 目录完全隔离。 |
| 凭据轮换但旧 Session 仍有效 | 不继续使用旧 Session；按 CredentialRevision 重新认证。 |
| `who` 返回非期望 UID | 拒绝操作并返回身份不匹配。 |
| 文件名包含错误关键词 | 成功输出仍按成功解析。 |
| 错误分享口令 | `ErrSharePasswordInvalid`，不盲目重试。 |
| 失效分享链接 | `ErrShareExpired`，不盲目重试。 |
| Provider 限流且有等待时间 | `ErrRateLimited` 和 RetryAfter 可判断。 |
| Local 路径在校验后被替换为符号链接 | 不能逃逸 Root。 |
| Local Remove 指向符号链接 | 不删除链接目标。 |
| Local Open FIFO/Socket/目录 | 返回非普通文件错误，不阻塞。 |
| Cursor A→B→A | Walk 返回协议错误，不无限循环。 |

## 8. 完成定义

只有同时满足以下条件，计划才可以标记完成：

1. `pkg/sdingest` 已真实复用 `pkg/request`，没有保留第二套 HTTP transport。
2. SDK 的 Token、401 重放、Envelope、错误安全和公开 DTO 契约通过回归测试。
3. RemoteFS 下载具备可靠的 no-replace、显式 replace、完整性验证和唯一提交点。
4. 同账号 Session 状态锁跨 FileSystem 实例有效，凭据轮换和 UID 绑定有效。
5. 凭据与分享口令不出现在命令 argv，子进程不继承无关 Secret。
6. Local Driver 不存在已知 root escape、follow-remove、特殊文件阻塞和写回来源漏洞。
7. `Reference`、`Entry`、Registry、Features 和 Share 契约完成减法，SDIngest 已一次迁移完成。
8. sdkit 与 SDIngest 的单元测试、race、全量构建和真实场景验收通过。
9. 对应模块与使用文档已经同步，GitNexus change detection 没有发现意外流程。
10. 没有修改 DreamIP，没有 commit、push 或发布 tag。

## 9. 执行记录

执行时在这里追加每个阶段的日期、命令结果、遗留问题和用户确认，不用在生产代码中保留临时兼容说明。

### 2026-08-25：阶段 0

- 已读取 sdkit、SDIngest 工作区与后端 AGENTS，以及 Go sdkitgo 的 workflow、testing、core、infra、worker、config、provider 规范。
- 两个工作树开始前均已有大量用户改动；本计划只在明确范围内追加修改，不覆盖既有变更，不修改 DreamIP。
- GitNexus 索引无 stale 警告。`Client.do` 为 HIGH；`Reference` 精确分析为 CRITICAL（32 个直接影响、5 条执行流）；`FileSystem` 为 MEDIUM。已按高风险契约采用先测试、分阶段迁移策略。
- 定向基线通过：`go test ./tests/pkg/request/... ./tests/pkg/sdingest/... ./tests/pkg/remotefs/...`。
- sdkit 全量基线通过：`go test ./...`。
- SDIngest 全量基线通过：`go test ./...`、`make build-all`。
- SDIngest OpenAPI 真实用例可执行；未设置 `SDINGEST_LIVE=1` 和 `SDINGEST_LIVE_CALLBACK=1` 时按设计明确跳过，留待 Phase 8 使用本地真实环境运行。
- 已索引 worker stage/run/pipeline、infra capability、Baidu Account Runtime 中的旧 Reference、Metadata version 和 Registry/Open 消费点。

### 2026-08-25：core 契约完成与 SDIngest 真实消费回归

- `pkg/sdingest.Client` 已持有 `*request.Client`，transport 固定 `MaxAttempts=1`，SDIngest 自身继续负责 Token、唯一一次 401 重放、Envelope、幂等和安全错误；`/v1/...` path 未引入 `/api` 前缀。
- `pkg/remotefs` 已完成 Reference/Entry/Downloader/Remover/Quota 减法；Local 和 Baidu Driver 的 no-replace、来源版本、条件删除、Progress、Session 隔离、CredentialRevision、ExpectedProviderUID 与 Secret 环境隔离均有契约测试。
- 真实消费方验收在 SDIngest `tests/` 完成，而不是在 core 中模拟业务：4 条 Smoke 与 100 条 Admin/OpenAPI 批次均通过，100 条全部进入终态，Attempt/Lease/Cleanup/Stage 收敛且执行节点无空值。
- 真实批次固定分布为 Admin 50、OpenAPI 50；各自包含 20 有效来源、15 错误口令、15 失效分享；覆盖 Auto、2 条 Manual Manifest、2 条取消、同账号并发、账号容量等待和 Worker 滚动重启。
- `go test ./... -count=1` 在 sdkit 与 SDIngest 均通过；SDIngest `make build-all` 通过；core 与 SDIngest 关键包 race 测试通过；两仓 `git diff --check` 通过。
- GitNexus `detect_changes` 对两仓报告 `HIGH`，对应的是本计划内 SDK、RemoteFS、execx、Worker、Callback、列表和迁移的整波影响面；没有把该结果误报成单点低风险。
- 用户授权后已完成 Webhook.site 真实公网回调：2xx 成功、真实 HTTP 500、约 1 分钟首次重试、HMAC、密钥轮换、旧密钥拒绝和幂等重放均通过；临时配置和 DNS 映射已清理。
- SDIngest 新增真实批次审计：`run_id=dky3acslw9j4` 最终为 38 `succeeded`、2 `canceled`、60 `failed`；逐个成功任务的 Job/Manifest/Artifact、size、checksum、remote ID/version 和 Cleanup 全部通过。
- 真实消费方的 Callback 管理测试覆盖 create/list/detail/rotate/enable/disable/replay、重复 URL 稳定错误和非法输入；SDIngest OpenAPI 文档已补齐逐接口入参、出参、Scope、幂等与验签说明。
- 当前只有一个真实 Baidu 账号；跨账号并发只有自动化隔离测试，真实双账号验收保留为环境项。
- SDIngest 新增 opt-in `TestLiveWorkerFaultRecovery`：以隔离 PostgreSQL 表、临时 mounted_path 和独立对象前缀验证完整 Pipeline、Progress/Reporter 故障、Checkpoint 失败后的完整文件复用、`.partial` 清理、取消、来源变化和远端对象清理。
- 该真实消费回归发现并修复 Local `RemoteID` 被误当成 Path 的问题；Reference 现在按 Path 定位、以 ID 校验身份。`make test-live-worker-fault` 全场景通过。
- 新增 SDIngest opt-in `TestLiveBaiduLeaseSessionIsolation` 与 `make test-live-baidu-session`。测试只读复制真实账号与 21 条成功任务中的加密分享来源，在唯一 PostgreSQL 表前缀、临时 ConfigRoot 和唯一远端目录中完成：两个 Lease/两个 FileSystem 并发 StageShare 路径不串、CredVer 轮换后新 Session 不接受旧 `who`、错误 ExpectedProviderUID 在 Stage 前进入账号错误态、失效凭据不污染健康账号任务。
- 真实 Session 用例耗时 32.25 秒，四个子场景全部通过；结束后复核 `bs_` 隔离 relation/function 均为 0，远端唯一测试目录清理无错误。测试未修改真实账号记录，未输出账号标识、分享地址或口令。
- 真实验收发现 SDIngest 未把 core `ErrIdentityMismatch` 归类为账号错误；已增加 `ACCOUNT_IDENTITY_MISMATCH`，映射为 blocked 账号健康、账号等待任务和人工处理筛选。GitNexus 对错误分类函数为 MEDIUM、健康映射和 Admin List 为 LOW，没有 HIGH/CRITICAL 单点改动。
- 当前数据库只有 1 个启用且有凭据的真实百度账号，并且真实记录尚未绑定 `provider_uid`；隔离副本已验证 UID 不匹配拒绝契约，但不同真实账号并行仍保留为环境待验收项。
- DreamIP 未修改；未 commit、未 push、未发布 tag。
- 2026-08-26 最终门禁复跑通过：sdkit 与 SDIngest `go test ./... -count=1`、两边关键包 `go test -race`、SDIngest `make build-all`、Admin `pnpm build` 和三仓 `git diff --check` 均通过；`pnpm vue-tsc` 只报告仓库已记录的历史基线错误，本次改动文件没有新增错误。
- 完成定义反向审计发现 Admin/API 百度账号 `Test`、`QuotaRefresh` 仍在 Handler 内创建 Runtime Service，并由 API/额度刷新直接暴露共享 `BaiduAccountView`。已删除该通用 View，Handler 改为调用窄职责账号外部操作门面后分别查询自身 projection；Admin 使用分段 `model := ... + Select + Take`，API 复用自身写后 DTO。新增架构契约测试禁止这两个 Handler 再引入 RemoteFS Runtime、构造器或共享 View。
- 最终 GitNexus `detect_changes`：sdkit 为 `HIGH`（41 个符号、13 个文件、8 条流程），SDIngest 为 `CRITICAL`（154 个符号、38 个文件、16 条流程）；新增流程是 Admin `Test`、`QuotaRefresh` 响应链，累计影响面均属于本计划，已由上述全量、race、构建和真实链路覆盖。
- Handler 边界收口后再次通过 SDIngest `go test ./... -count=1`、相关包 `go test -race`、`make build-all` 和 `make test-live-baidu-session`。真实用例已增加“双真实账号并行 StageShare/Download”条件分支：要求两个启用、有凭据且已绑定不同 Provider UID 的账号；当前审计为 1 个账号、0 个已绑定 UID，因此该子场景明确 Skip，其余场景 36.73 秒通过且隔离资源清理完成。
- 最新代码已重启 Admin、API、Realtime、3 个 Worker 与 Crontab；后端 `8080/8081/8092` 和 Admin 前端 `5200` 均可访问。除“两个不同真实账号并行”这一环境项外，其余可执行验收项已完成。
- 2026-08-26 第三次阻塞审计：`make test-live-baidu-session` 再次确认 `enabled=1`、`credentialed=1`、`provider_uid_bound=0`、可用的不同已绑定 Session 为 0；双账号子场景明确 Skip，其余真实场景 35.21 秒通过。根据连续三轮相同外部条件，计划状态改为外部阻塞；补充第二个已绑定不同 Provider UID 的真实账号后可直接重跑该命令恢复验收。
- 用户确认可复制现有账号补测多账号记录。真实用例已在隔离表创建两份使用同一真实凭据、但具有不同 PublicID、ConfigKey 和 Session 目录的有效账号记录，并发 StageShare/Download 子场景 21.21 秒通过，整体用例 53.97 秒通过且清理成功。该证据证明多账号记录与多 Session 不串号，但不替代两个不同百度身份/Provider UID 的最终验收。
