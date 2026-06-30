# reporter 使用说明

`core/reporter` 提供通用事件上报管线。它只负责把事件分发给多个 sink，不关心业务表、SSE room、缓存实现或日志格式。

## 基本用法

业务侧实现自己的 sink：

```go
type OperationLogSink struct{}

func (OperationLogSink) Handle(ctx context.Context, event reporter.Event) error {
	// 写业务日志表
	return nil
}
```

注册并上报：

```go
r := reporter.New(
	OperationLogSink{},
	RealtimeSink{},
)

_ = r.Progress(ctx, reporter.Event{
	OperationID: "prepare:123",
	Step:        "download",
	ProgressKey: "download_data",
	Percent:     20,
	Status:      "running",
	Message:     "下载数据文件",
})
```

## 必须 sink

默认 sink 是可选的，失败不会中断上报。需要影响主流程时标记为 required：

```go
r := reporter.NewWithOptions(
	reporter.WithSink(OperationLogSink{}, reporter.WithRequired()),
	reporter.WithSink(RealtimeSink{}),
)
```

只有 required sink 失败时，`Emit` / `Log` / `Progress` / `Done` 才返回错误。

## Progress 快照

前端需要快速读取当前进度时，使用 `ProgressSnapshotSink`：

```go
store := reporter.NewMemoryProgressStore()
r := reporter.New(
	reporter.NewProgressSnapshotSink(store),
)
```

生产项目通常实现自己的 `ProgressStore`，例如基于 Redis/cache：

```go
type RedisProgressStore struct{}

func (RedisProgressStore) Save(ctx context.Context, key string, snapshot reporter.ProgressSnapshot, ttl time.Duration) error {
	return nil
}

func (RedisProgressStore) Load(ctx context.Context, key string) (reporter.ProgressSnapshot, bool, error) {
	return reporter.ProgressSnapshot{}, false, nil
}

func (RedisProgressStore) Delete(ctx context.Context, key string) error {
	return nil
}
```

`ProgressSnapshotSink` 会用 `OperationID` 作为默认 key。`ProgressKey` 相同的 step 会覆盖更新，适合展示下载、解析、入库等阶段的当前状态。

如果某个流程结束后不需要保留 progress cache，可以在实时推送 sink 后注册 `ProgressCleanupSink`：

```go
store := reporter.NewMemoryProgressStore()
r := reporter.NewWithOptions(
	reporter.WithSink(reporter.NewProgressSnapshotSink(store)),
	reporter.WithSink(RealtimeSink{}),
	reporter.WithSink(reporter.NewProgressCleanupSink(store), reporter.WithFilter(reporter.TerminalFilter())),
)
```

这样 `Done` / `Failed` / `Canceled` 事件会先更新快照并推送终态，再清理缓存。

## 边界

- reporter 不直接写数据库。
- reporter 不直接推 SSE。
- reporter 不直接依赖 Redis/cache。
- 落表、推房间、缓存快照都由业务 sink 决定。
