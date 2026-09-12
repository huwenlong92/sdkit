# RemoteFS 模块设计

`pkg/remotefs` 提供外部层级文件系统的最小公共契约。当前实现 `local` 与 `baidupan` 两个 driver。它只描述“一个已经打开、已经绑定来源身份的文件系统”，不负责账号池、租约、任务状态、数据库或业务 Runtime。

## 包结构

```text
pkg/remotefs/
  types.go
  errors.go
  progress.go
  walk.go
  internal/offsetcursor/
  driver/local/
  driver/baidupan/
```

测试位于 `tests/pkg/remotefs/`，生产包中不放测试文件。

## 基础契约

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
```

`Reference` 不携带 driver、connection 或账号。构造 `FileSystem` 时身份已经绑定，后续引用只表达该实例内的对象 ID 和规范路径。`Entry.Reference.Path` 是唯一路径字段；Provider 版本写入 `Entry.Version`，不通过通用 metadata 泄漏。

`ModTime == nil` 表示 Provider 没有给出可靠时间。目录大小固定为 0。`Quota` 只包含 `TotalBytes` 和 `UsedBytes`，剩余容量由消费方自己的响应 projection 计算。

## 可选能力

调用方直接断言所需的小接口：

- `Downloader`
- `Opener`
- `Remover`
- `ShareStager`
- `HealthChecker`
- `QuotaReader`

公共包不提供 Registry、Features 探测或 `config any` 工厂。Local 使用 `local.New`；Baidu 使用进程级 `baidupan.Runtime` 加账号级 `SessionConfig`。如果未来出现真正的多 Driver 动态注册需求，再另行设计类型安全注册机制。

`Remove` 默认不暴露。启用后一次只删除一个对象，并在删除前校验 `Reference.ID` 与 `ExpectedVersion`。`DownloadRequest` 也支持相同的条件快照，来源变化返回 `ErrSourceChanged`。

## 下载事务与 Progress

Driver 的下载顺序固定为：

```text
Prepare → Transfer → Verify → Commit → Return Success
```

- `OverwriteDeny` 使用原子 no-replace 提交；并发写同一目标只能有一个成功。
- `OverwriteReplace` 是显式原子替换。
- 数据先写入排他创建的 staging/partial 文件，验证来源快照和字节数后才提交。
- `PartialFileKeep` 只保留失败现场，不承诺续传；已有 `<destination>.partial` 时返回 `ErrConflict`，不会覆盖。
- Driver 只发布 `Preparing`、`Transferring`、`Finalizing`，不发布业务完成事件。`Download` 成功返回才是唯一提交完成点。
- Progress 数值必须非负；总量已知时 transferred 不得超过 total。Baidu 还拒绝传输字节倒退或 total 在同一次下载中变化。
- Baidu 在实时和收尾进度调用 `EmitProgress` 前，将超过正数 total 的 transferred 截断为 total，以兼容 CLI 大小舍入；原始进度仍用于倒退和总量变化检查。截断不改变落盘大小校验或 `DownloadResult.BytesWritten`，公共 `EmitProgress` 校验保持不变。
- Sink 同步执行并传导背压。提交前 Sink 失败原样返回，不能包装成 Provider 临时错误。

## Walk 与分页

`Walk` 使用广度优先顺序访问 root 的后代，不访问 root 自身。`MaxDepth=1` 只访问直接子项。`Filter` 只决定是否调用 visitor，不剪枝；`Prune` 专门阻止目录子树遍历。

每个目录记录所有已见 cursor，A→B→A 等循环返回 `ErrProtocol`。目录 visited key 优先使用稳定 ID，否则使用规范 Path。达到 `MaxEntries` 返回 `ErrWalkLimitExceeded`。

Local 和 Baidu 的当前 cursor 都是绑定 directory、sort 和 order 的不透明 best-effort offset；跨查询复用返回 `ErrInvalidOption`，但不提供目录快照一致性。Baidu `ls` 仍读取完整远端目录后在本地切页，不能把它描述成 Provider 侧分页优化。

## 标准错误

公共 `*remotefs.Error` 只保留 `Operation`、`Driver`、脱敏 `Summary` 和可 `errors.Is` 的底层错误。主要公共错误包括：

- 访问类：`ErrNotFound`、`ErrUnauthenticated`、`ErrPermissionDenied`、`ErrRateLimited`、`ErrQuotaExceeded`
- 参数类：`ErrNilContext`、`ErrInvalidArgument`、`ErrInvalidOption`、`ErrInvalidReference`
- 文件类：`ErrNotDirectory`、`ErrNotRegularFile`、`ErrConflict`、`ErrSourceChanged`、`ErrIntegrity`
- 协议类：`ErrProtocol`、`ErrIdentityMismatch`、`ErrTemporary`、`ErrUnsupported`

二进制、版本和分享链接错误属于 `driver/baidupan`：`ErrBinaryUnavailable`、`ErrVersionUnsupported`、`ErrShareInvalid`、`ErrShareExpired`、`ErrSharePasswordInvalid`。公共 `remotefs.Error` 不含没有真实来源的 RetryAfter 字段。

`SafeSummary` 按 UTF-8 边界截断并移除 ANSI/控制字符。错误链不得保留 BDUSS、STOKEN、分享口令、完整命令参数或不受控 Provider 原始输出。

## Local driver

Local 使用 Go `os.Root` 将所有来源操作限制在目录句柄内：

- root 必须是普通目录，最终符号链接 root 被拒绝。
- Stat/List/Open/Download 的路径分量都拒绝符号链接、越级和特殊文件。
- Open/Download 只接受普通文件；FIFO、Socket 和目录不会被打开，因此不会阻塞。
- Remove 可以删除引用到的符号链接本身，但不会跟随并删除目标；父路径中的符号链接仍被拒绝。
- Download 默认拒绝把 destination 写回来源 root。
- Open 的 context 只约束打开动作；返回 reader 后，读取生命周期由调用方通过 `Close` 管理。

## Baidu Runtime 与 Session

`RuntimeConfig` 是进程级配置：BinaryPath、版本范围、ConfigRoot、OutputLimit。`NewRuntime` 校验绝对、可执行、非符号链接的普通二进制，校验并缓存版本；`HealthChecker.Check` 可显式重新验证。

`SessionConfig` 是账号级配置：SessionKey、ExpectedProviderUID、CredentialRevision、BDUSS、STOKEN、下载并发/模式和 Remove 权限。Session 目录由 SessionKey 与 CredentialRevision 共同派生；凭据轮换后不会继续复用旧目录。`who` 返回 UID 与 ExpectedProviderUID 不一致时返回 `ErrIdentityMismatch`。

同一 Session 的 login、secureSession、mkdir/cd/transfer 使用同一个进程锁和 Session 目录下的文件锁，跨 FileSystem 实例、跨进程串行；不同 Session 可以并行。标准 BaiduPCS-Go v4.0.0 的 transfer 仍依赖远端 cwd，因此当前不能去掉这个临界区。

Runner 使用 clean environment，只注入必要变量。BDUSS、STOKEN 和分享口令通过交互 stdin 进入 CLI，不出现在实际 argv。ConfigRoot、SessionDir 和锁文件会检查权限、类型与符号链接。

## 验证

```bash
go test ./tests/pkg/remotefs/...
go test -race ./tests/pkg/remotefs/...
```

真实 Baidu 验收必须显式开启 opt-in 环境变量，默认测试不依赖网络、真实账号或 Secret。

## 更新记录

- 2026-09-12：限制 Baidu 下载上报进度不超过已知总量，避免 CLI 大小舍入导致下载失败；增加回归测试，保留来源和落盘大小校验。
- 2026-08-25：删除 Registry、Features、ShareResolver、重复身份字段和 Provider metadata；增加 Runtime/Session、条件操作、原子提交、Session 文件锁、`os.Root` confinement 和不透明 query-bound cursor。
- 2026-08-23：新增 RemoteFS 首版、Local 与 Baidu driver。
