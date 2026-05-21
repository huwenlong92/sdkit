# crontab 使用文档

`core/crontab` 是 Template-Driven Runtime 调度内核。业务层只声明 `Template` 并实现 `Handler`；调度、trace、timeout、任务锁、日志、metrics、失败回调都由 `Registry.Dispatch` 接管。

项目侧 `crontab` 包负责组织模板和服务接线：示例模板和示例实现放在根包 `crontab/demo_*.go`，DB store、Redis lock、realtime 等外部适配放在 `crontab/infra/`。对外管理能力通过 `core/crontab.Service` 和 `crontab/infra/capability/operations` 暴露，Admin 不创建模板，只读取允许绑定 DB 的模板并管理 DB Entry。

## 配置

```yaml
crontab:
  enabled: true
  driver: robfig
  reload_interval: 30s
  instance_id: ""
  lock:
    enabled: true
    ttl: 10m
  log:
    enabled: true
    batch: false
    batch_size: 100
    flush_interval: 3s
```

## 声明模板

模板直接使用 `corecron.Template`，不再通过 middleware、Router DSL 或兼容注册桥接：

```go
package crontab

import (
    "context"
    "time"

    corecron "github.com/huwenlong92/sdkit/core/crontab"
)

const SSEDemoKey = "cron_sse_demo"

var SSEDemoTemplate = corecron.Template{
    Key:          SSEDemoKey,
    Name:         "SSE 推送示例",
    Desc:         "通过 Crontab 服务的 realtime 能力推送一条 SSE demo 消息",
    Spec:         "*/5 * * * *",
    Enabled:      true,
    AllowDB:      true,
    AllowOverlap: false,
    Timeout:      3 * time.Minute,
    DefaultPayload: `{"event":"notify","data":{"title":"Crontab SSE demo"}}`,
    PayloadFormat:  "json",
    Handler:      corecron.RunHandlerFromFunc(runSSEDemo),
}

func runSSEDemo(ctx context.Context, job corecron.Job) error {
    // 示例实现：解析 payload，然后通过 realtime publisher 推送。
    return nil
}
```

`Spec` 非空时，Registry 会把模板生成一个内置调度实例，ID 为 `builtin.<template_key>`。`Spec` 为空时，模板只作为可被手动执行或 DB Entry 引用的能力定义存在。

## 注册模板

根包显式注册模板：

```go
func RegisterTemplates(registry *corecron.Registry) error {
    if registry == nil {
        return corecron.ErrRegistryRequired
    }
    return registry.RegisterAll(
        CacheGCTemplate,
        SSEDemoTemplate,
    )
}

func NewRegistry() (*corecron.Registry, error) {
    registry := corecron.NewRegistry()
    if err := RegisterTemplates(registry); err != nil {
        return nil, err
    }
    return registry, nil
}
```

`AllowDB=true` 时才允许后台或 DB 动态任务引用该模板。DB 动态任务只提供 Entry 层字段：模板 key、展示名、cron 表达式、payload、启用状态、执行次数上限。

## 执行函数

普通业务函数通过 `RunHandlerFromFunc` 适配：

```go
func runCacheGCDemo(ctx context.Context, job corecron.Job) error {
    logger.WithContext(ctx, logger.Named("crontab-cache-gc-demo")).Info("crontab cache gc demo")
    return nil
}
```

Handler 收到的 `ctx` 已带 `track_id`、`run_id`、`job_id` 和当前 `crontab.execute` span。业务内访问 DB、Redis、HTTP、realtime 时必须继续透传该 `ctx`。

业务日志不要打印敏感 payload。需要解析 `job.Payload` 时，由 handler 自己解析并校验。

## Runtime Governance

当前运行模型：

```text
Template
  -> Registry
  -> Scheduler Trigger
  -> Dispatch
  -> Runtime Governance
  -> Execute Handler
  -> Failure Callback
```

`Registry.Dispatch` 自动处理：

- tracing span: `crontab.execute`
- track_id 生成与透传
- runtime logger: start / success / failed / timeout / overlap skipped
- timeout: `Template.Timeout` 或 RunOnce 指定 timeout
- overlap lock: `AllowOverlap=false` 时使用 `crontab:entry:<entry_id>`
- running/final run log
- runtime state
- metrics
- panic recover
- failure callback

业务层禁止再挂 crontab middleware。需要审计、告警或通知时，使用 handler 内部逻辑或全局 failure callback。

## Failure Callback

失败回调只对真正执行失败触发：`failed`、`timeout`、`panic`。锁冲突、禁用、模板缺失等跳过状态不会触发失败回调。

```go
corecron.UseFailureHandler(func(ctx context.Context, report corecron.FailureReport) {
    logger.WithContext(ctx, logger.Named("crontab-failure")).Error(
        "crontab execute failed",
        zap.String("entry_id", report.EntryID),
        zap.String("template_key", report.TemplateKey),
        zap.Duration("duration", report.Duration),
        zap.String("trace_id", report.TraceID),
        zap.Error(report.Error),
    )
})
```

`FailureReport`：

```go
type FailureReport struct {
    EntryID     string
    TemplateKey string
    StartedAt   time.Time
    FinishedAt  time.Time
    Duration    time.Duration
    TraceID     string
    Error       error
}
```

## Tracing

每次执行任务时，Dispatch 创建 span：

```text
crontab.execute
```

span 属性包括：

```text
entry.id
entry_id
template.name
template
cron
allow_overlap
timeout
track_id
crontab.status
success
duration
```

## Metrics

`RuntimeMetricsSnapshot()` 返回：

```text
crontab_execute_total
crontab_execute_success_total
crontab_execute_failed_total
crontab_execute_duration
crontab_timeout_total
crontab_overlap_skipped_total
```

`failed_total` 统计 `failed`、`timeout`、`panic`。`overlap_skipped_total` 单独统计任务锁冲突。

## DB 动态任务

后台动态任务只配置 Entry 层字段：

```text
可配置：template_key/name、label、spec、payload、enabled、max_run_count
不可配置：handler、mode、timeout、allow_overlap、distributed、lock_ttl、queue、task_type
```

`max_run_count=0` 表示不限次数；大于 0 时，项目侧 Store 应在达到上限后停用 DB Entry，避免后续调度继续执行。

执行策略来自 Template：

- `Timeout` 控制超时
- `AllowOverlap` 控制任务级互斥
- `AllowDB` 控制是否允许 DB 引用
- `Enabled` 是代码级总开关
- `crontab.lock.enabled=false` 时跳过 runtime 分布式锁；开启时才按 Entry 粒度加锁

同一个模板可以被多条 DB 任务引用。默认锁粒度是 Entry，不是 Template，所以不同 Entry 互不阻塞。

## Operations Facade

`core/crontab.Service` 是模板查询和任务管理的对外 API：

```go
type Service interface {
    ListTemplates(ctx context.Context) ([]TemplateInfo, error)
    ListDBTemplates(ctx context.Context) ([]TemplateInfo, error)
    ListEntries(ctx context.Context) ([]EntryInfo, error)
    GetEntry(ctx context.Context, id string) (EntryInfo, error)
    CreateEntry(ctx context.Context, req CreateEntryRequest) (EntryInfo, error)
    UpdateEntry(ctx context.Context, id string, req UpdateEntryRequest) error
    DeleteEntry(ctx context.Context, id string) error
    EnableEntry(ctx context.Context, id string) error
    DisableEntry(ctx context.Context, id string) error
    RunOnce(ctx context.Context, req RunOnceRequest) error
}
```

`RunOnce` 是同步执行入口：handler 返回错误、timeout、panic 或锁冲突都会以 error 返回给调用方，同时仍写入运行日志和 runtime state。调用方可以用 `errors.Is(err, corecron.ErrJobRunning)` 单独处理任务正在执行的冲突。

项目侧通过 capability 注入给需要管理 crontab 的服务：

```go
crontabops.Use(
    crontabops.WithName(ctx.LocalName(crontabops.Name)),
    crontabops.WithConfigLoader(func(*runtime.App) (crontabops.Config, error) {
        return cronconfig.Load(ctx.ConfigFile)
    }),
)
```

HTTP handler 通过构造函数持有 `core/crontab.Service`，不要从路由 middleware 注入，也不要直接访问 `models.SystemCrontab` 或重新构建模板 catalog：

```go
crontabHandler := system.NewCrontabHandler(crontabops.FromServiceContext(ctx))
crontab.GET("/templates", crontabHandler.Templates)
crontab.POST("", crontabHandler.Create)
```

Admin 的职责是：

- 拉取 `ListDBTemplates` 返回的模板列表。
- 创建、更新、删除、启停 DB Entry。
- 手动触发已有 Entry。

模板定义、默认 payload、payload schema、是否允许 DB 绑定，都由 `crontab/demo_*.go` 中硬编码的 `Template` 决定。

## CLI

```bash
go run ./cmd/sdkitgo cron
go run ./cmd/sdkitgo cron start
go run ./cmd/sdkitgo cron commands
go run ./cmd/sdkitgo cron list
go run ./cmd/sdkitgo cron models
go run ./cmd/sdkitgo cron models --dynamic
go run ./cmd/sdkitgo cron run db.1
go run ./cmd/sdkitgo cron run-template cron_sse_demo --payload '{"event":"notify"}'
go run ./cmd/sdkitgo cron runs db.1
go run ./cmd/sdkitgo cron logs <run_id>
```

`cron` 根命令等价于 `cron start`。当前 CLI 不维护后台 daemon 或 pidfile，停止前台进程使用 Ctrl+C 或 SIGINT/SIGTERM。

## 服务启动

代码中通常通过骨架服务启动。`Provider()` 声明 Crontab runtime capability，runtime 初始化后调用 `NewServerWithContext` 构建调度器。

手动启动：

```go
serviceCtx := appbootstrap.NewServiceContext("crontab", "crontab", nil)
srv, err := crontab.NewServerWithContext(cfg, serviceCtx)
if err != nil {
    return err
}
if cfg.Crontab.Enabled {
    go func() {
        _ = srv.Start(ctx)
    }()
}
```

`NewServerWithContext` 复用当前已初始化的 `database.DB` 和 `redis.RDB` 作为 store、log writer 和分布式锁。
