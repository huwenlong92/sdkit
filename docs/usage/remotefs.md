# RemoteFS 使用指南

`pkg/remotefs` 只提供已经打开的远端文件系统契约。业务代码显式构造实例、按需断言能力并负责关闭；不要在业务项目里再包一层通用 Registry 或 `config any` Runtime。

## Local 来源

```go
fs, err := local.New(local.Config{Root: "/mnt/media"})
if err != nil {
    return err
}
defer fs.Close()

page, err := fs.List(ctx, remotefs.Reference{Path: "/shows"}, remotefs.ListOptions{
    PageSize: 100,
    SortBy:   remotefs.SortByName,
    Order:    remotefs.SortAscending,
})
```

远端 Path 使用 `/` 开头。Root 外路径、`..`、符号链接和特殊文件会被拒绝。Local `Open` 返回普通 `io.ReadCloser`；context 只约束打开动作，后续读取由调用方关闭 reader：

```go
opener, ok := fs.(remotefs.Opener)
if !ok {
    return remotefs.ErrUnsupported
}
reader, err := opener.Open(ctx, remotefs.Reference{Path: "/shows/episode.mp4"}, remotefs.OpenOptions{})
if err != nil {
    return err
}
defer reader.Close()
```

Local 默认不暴露 `Remover`。确实允许删除来源时才配置 `AllowRemove: true`。

## Baidu Runtime 与账号 Session

进程启动时只创建一次 Runtime：

```go
runtime, err := baidupan.NewRuntime(ctx, baidupan.RuntimeConfig{
    BinaryPath:  "/opt/tools/BaiduPCS-Go",
    MinVersion:  "v4.0.0",
    MaxVersion:  "v4.0.0",
    ConfigRoot:  "/var/lib/myapp/baidupcs",
    OutputLimit: 10 << 20,
})
if err != nil {
    return err
}
```

获得业务账号 Lease 后，为该账号打开 FileSystem：

```go
fs, err := runtime.Open(ctx, baidupan.SessionConfig{
    SessionKey:          account.ConfigKey,
    ExpectedProviderUID: account.ProviderUID,
    CredentialRevision: strconv.FormatInt(account.CredentialVersion, 10),
    BDUSS:               secret.BDUSS,
    STOKEN:              secret.STOKEN,
    DownloadConcurrency: account.DownloadConcurrency,
    DownloadMode:        account.DownloadMode,
})
if err != nil {
    return err
}
defer fs.Close()
```

不要把 `Reference` 当作账号引用。它只能在创建它的 FileSystem 实例中使用：

```go
ref := remotefs.Reference{ID: remoteID, Path: "/shows/episode.mp4"}
```

账号池、Lease、冷却、权重、任务粘性和重试属于消费方。Driver 只保证同一 Session 的 cwd 状态操作跨实例、跨进程串行，不同 Session 仍可并行。

`SessionKey` 应使用稳定的业务配置键，不直接使用 Secret。轮换凭据时必须增加 `CredentialRevision`。已知 Provider UID 时必须传 `ExpectedProviderUID`，避免复用错误登录态。

## 分享解析与转存

纯校验可直接调用：

```go
parsed, err := baidupan.ParseShare(remotefs.ShareRequest{
    URL:      shareURL,
    Password: sharePassword,
})
```

`ParseShare` 只接受 `https://pan.baidu.com/s/...`，会从 URL 中移除 `pwd`。不要记录 `parsed.Password` 或原始分享 URL。

转存使用可选能力：

```go
stager, ok := fs.(remotefs.ShareStager)
if !ok {
    return remotefs.ErrUnsupported
}
staged, err := stager.StageShare(ctx, remotefs.StageRequest{
    Share:       remotefs.ShareRequest{URL: shareURL, Password: sharePassword},
    Destination: remotefs.Reference{Path: "/staging/task-123"},
})
```

目标必须是绝对远端目录。Driver 会幂等创建目录，并在同一 Session 临界区完成 mkdir/cd/transfer；它不决定业务目录何时清理。

## 条件下载

先保存 `Stat` 快照，再把 ID/Version 传入 Download：

```go
entry, err := fs.Stat(ctx, remotefs.Reference{Path: "/shows/episode.mp4"})
if err != nil {
    return err
}
downloader, ok := fs.(remotefs.Downloader)
if !ok {
    return remotefs.ErrUnsupported
}
result, err := downloader.Download(ctx, remotefs.DownloadRequest{
    Reference:       entry.Reference,
    ExpectedVersion: entry.Version,
    Destination:     "/var/lib/myapp/downloads/episode.mp4",
    Overwrite:       remotefs.OverwriteDeny,
    PartialFile:     remotefs.PartialFileRemove,
}, remotefs.ProgressSinkFunc(func(ctx context.Context, progress remotefs.Progress) error {
    return persistProgress(progress)
}))
```

Driver 只发送 `preparing`、`transferring`、`finalizing`。`Download` 返回 nil 才表示目标已经提交；业务方应在返回后单独写完成状态或发送完成通知。完成通知失败不能反过来把已提交文件当作 Provider 下载失败。

Sink 同步执行。Sink 返回的数据库、消息或 context 错误会原样返回，调用方不能因此冷却 Provider 账号。

`PartialFileKeep` 仅保留 `<destination>.partial` 作为诊断现场，不保证续传。并发已有者返回 `ErrConflict`。

## 条件清理

```go
remover, ok := fs.(remotefs.Remover)
if !ok {
    return remotefs.ErrUnsupported
}
err := remover.Remove(ctx, remotefs.RemoveRequest{
    Reference:       stagedEntry.Reference,
    ExpectedVersion: stagedEntry.Version,
})
```

ID 或 Version 不匹配时返回 `ErrSourceChanged`，不得改成只按 Path 强删。对于路径已被新对象复用的场景，业务方应进入人工/对账状态。

## Quota

```go
reader, ok := fs.(remotefs.QuotaReader)
if !ok {
    return remotefs.ErrUnsupported
}
quota, err := reader.Quota(ctx)
if err != nil {
    return err
}
free := quota.TotalBytes - quota.UsedBytes
```

Quota 单位为字节。API/Admin 按自己的 DTO 计算 free；查询失败时显示未知，不能覆盖为 0。

## Walk

```go
err := remotefs.Walk(ctx, fs, remotefs.Reference{Path: "/shows"}, remotefs.WalkOptions{
    PageSize:   100,
    MaxDepth:   8,
    MaxEntries: 10_000,
    Filter: func(entry remotefs.Entry) bool {
        return entry.IsDir() || strings.HasSuffix(strings.ToLower(entry.Name), ".mp4")
    },
    Prune: func(entry remotefs.Entry) bool {
        return entry.IsDir() && entry.Name == "trash"
    },
}, func(ctx context.Context, entry remotefs.Entry) error {
    return enqueue(entry)
})
```

Walk 不访问传入的 root，采用广度优先。Filter 不剪枝；Prune 才阻止进入目录。Cursor 是不透明的 query-bound best-effort offset，不要持久化后跨目录、排序方式或目录内容变化重用。Baidu 大目录仍会先完整读取 `ls` 输出。

## Context 与错误

所有方法都要求非 nil context。调用方负责 deadline：

```go
ctx, cancel := context.WithTimeout(parentCtx, 10*time.Minute)
defer cancel()
```

Baidu Runner 在取消时终止命令进程组。Local Open 的 context 不绑定已返回 reader 的整个生命周期。

```go
var remoteErr *remotefs.Error
switch {
case errors.Is(err, context.DeadlineExceeded):
    // 调用方超时。
case errors.Is(err, baidupan.ErrSharePasswordInvalid):
    // 永久分享错误，不盲目重试。
case errors.Is(err, remotefs.ErrRateLimited):
    // 由业务账号调度决定等待。
case errors.Is(err, remotefs.ErrSourceChanged):
    // 来源快照变化，停止下载或清理。
case errors.As(err, &remoteErr):
    // 只记录脱敏后的 Operation、Driver、Summary。
}
```

不要依赖 Provider 原始中文输出，也不要记录 BDUSS、STOKEN、分享口令或完整命令参数。

## 测试

```bash
go test ./tests/pkg/remotefs/...
go test -race ./tests/pkg/remotefs/...
```

真实 Baidu 测试必须使用 opt-in 环境变量和 Secret Provider；默认测试全部使用 fake runner。
