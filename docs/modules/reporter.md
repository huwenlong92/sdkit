# reporter 模块说明

`core/reporter` 是通用事件上报框架，用于把一个业务事件分发给多个观察者。

它解决的问题是：

- 同一个事件可能要落不同业务表。
- 同一个事件可能要推多个实时 room。
- progress 需要被前端快速读取。
- 不同 sink 的失败策略不同。

## 设计边界

reporter 只包含框架级抽象：

- `Event`：通用事件载体。
- `Reporter`：同步 fan-out 调度器。
- `Sink`：业务观察者接口。
- `ProgressStore`：进度快照存储接口。
- `ProgressSnapshotSink`：通用进度快照更新策略。
- `ProgressCleanupSink`：终态事件后的 progress 清理策略。

reporter 不包含项目业务语义：

- 不定义业务日志表。
- 不定义 SSE room。
- 不绑定 GORM、Redis、cache facade。
- 不识别具体业务 ID，只通过 `Subject` 或显式字段传递。

## Event

`Event` 用于承载创建、核验、重建、执行等流程中的日志和进度。

关键字段：

- `Kind`：`log`、`progress`、`done`、`failed`、`canceled`。
- `OperationID`：一次操作的稳定 ID，用于聚合日志和 progress。
- `OperationType`：业务操作类型。
- `Step` / `ProgressKey`：前端进度阶段。
- `Percent` / `Status` / `Message`：展示状态。
- `Subject`：业务对象身份，例如 dataset_id、run_id。
- `Detail`：业务扩展信息。

## Sink

业务侧通过实现 `Sink` 接入：

```go
type Sink interface {
	Handle(ctx context.Context, event Event) error
}
```

典型 sink：

- 数据集操作日志 sink：写 `sd_dataset_operation_log`。
- 计算执行日志 sink：写 `sd_calculation_run_log`。
- 实时推送 sink：推 SSE 或 websocket room。
- 进度缓存 sink：写 Redis/cache 快照。
- 文件日志 sink：写本地文件。

## 错误策略

默认 sink 是 optional。optional sink 失败不会让 `Reporter.Emit` 返回错误，但可以通过 `WithErrorHandler` 观察。

使用 `WithRequired()` 注册的 sink 是 required。required sink 失败会返回 `SinkErrors`。

这个策略用于区分：

- 关键落库失败：应该影响任务。
- SSE 推送失败：通常不应影响任务。
- progress cache 写入失败：多数场景只记录，不中断任务。

## ProgressSnapshotSink

`ProgressSnapshotSink` 根据事件维护一个 `ProgressSnapshot`。

默认行为：

- key 使用 `OperationID`。
- running TTL 为 6 小时。
- terminal TTL 为 2 小时。
- `ProgressKey` 相同的 step 会覆盖更新。
- 最近事件默认保留 50 条。

业务项目可以通过实现 `ProgressStore` 接入自己的缓存系统。

## ProgressCleanupSink

`ProgressCleanupSink` 用于 `Done` / `Failed` / `Canceled` 后清理 progress cache。

它通常放在实时推送 sink 之后，并配合 `TerminalFilter()` 使用：

```go
reporter.WithSink(reporter.NewProgressCleanupSink(store), reporter.WithFilter(reporter.TerminalFilter()))
```

如果业务需要结束后仍能回看 progress，则不要注册这个 sink，或改用较短的 terminal TTL。

## 更新记录

- 增加 `ProgressCleanupSink`，支持终态事件后清理 progress cache。
- 新增 `core/reporter`，提供事件分发、sink 注册、错误策略、progress 快照和内存 progress store。
