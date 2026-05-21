# crontab 模块设计

`core/crontab` 是成熟的 scheduler runtime kernel。当前版本完成 Template-Driven Runtime freeze：Template 是唯一 runtime definition source，Entry 是 runtime schedule instance，Dispatch 是 runtime orchestration 入口，Handler 只负责业务执行。

## 目标

```text
业务层负责声明 Template 与实现 Handler
core/crontab 负责全部 runtime governance
```

`core/crontab` 负责：

- template registry
- scheduler dispatch
- tracing
- track_id / run_id / job_id 透传
- timeout
- overlap lock
- panic recover
- runtime logger
- run log
- runtime state
- metrics
- failure callback

业务层只负责：

- 定义 Template
- 实现 Handler
- 解析和校验 payload
- 调用业务服务并透传 context

## Runtime Model

```text
Template
  -> Registry
  -> Scheduler Trigger
  -> Dispatch
  -> Runtime Governance
  -> Execute Handler
  -> Failure Callback
```

## Template

```go
type Template struct {
    Key string

    Name string
    Desc string
    Spec string

    Enabled bool
    AllowDB bool

    AllowOverlap bool
    Timeout time.Duration

    Handler RunHandler

    DefaultPayload string
    PayloadFormat string
    PayloadSchema string
}
```

字段语义：

- `Key`：模板唯一键，DB Entry 和手动执行都通过它引用模板。
- `Name` / `Desc`：展示信息。
- `Spec`：默认内置调度表达式。非空时 Registry 生成 `builtin.<key>` 内置任务；为空时不生成内置任务。
- `Enabled`：代码级总开关。
- `AllowDB`：是否允许 DB 动态任务引用。
- `AllowOverlap`：是否允许同一 Entry 并发执行。
- `Timeout`：模板级执行超时。
- `Handler`：业务执行入口。
- `DefaultPayload` / `PayloadFormat` / `PayloadSchema`：模板展示和后台表单元数据，由 `Registry.ListTemplateInfo` / `ListDBTemplateInfo` 导出，不参与 runtime 执行。

Template 不承载 middleware、event runtime、retry、deadletter 或 queue task type。payload 的实际解析和校验仍由 handler 负责，Template 里的 payload 字段只用于展示默认值和表单提示。

## Handler

```go
type RunHandler func(*RunContext) RunResult

func RunHandlerFromFunc(func(context.Context, Job) error) RunHandler
```

`RunHandlerFromFunc` 会把 `RunContext` 转成业务常用的 `context.Context + Job`。handler 收到的 context 已包含：

- `track_id`
- OpenTelemetry span
- `run_id`
- `job_id`
- Entry 信息
- JobLogger

业务代码必须继续透传 context，不能 panic，error 必须返回给 runtime。

## Registry

Registry 只维护：

```go
templates map[string]Template
```

公开能力：

```go
func NewRegistry() *Registry
func (r *Registry) Register(t Template) error
func (r *Registry) RegisterAll(templates ...Template) error
func (r *Registry) MustRegister(t Template)
func (r *Registry) Get(key string) (Template, bool)
func (r *Registry) List() []Template
func (r *Registry) ListTemplateInfo() []TemplateInfo
func (r *Registry) ListDBTemplateInfo() []TemplateInfo
func (r *Registry) ValidateJob(job Job) (Template, error)
func (r *Registry) BuiltinJobs() []Job
func (r *Registry) Dispatch(ctx context.Context, entry *Entry) error
```

`BuiltinJobs()` 从 `Template.Spec` 生成内置任务：

```text
ID      = builtin.<template_key>
Name    = template_key
Label   = template name
Spec    = template Spec
Source  = builtin
Mode    = local
Timeout = template Timeout
```

不再存在独立 BuiltinJob registry。内置调度也是 Template 的派生 Entry。

## Operations API

`core/crontab.Service` 是 crontab 暴露给服务侧和 Admin 的 capability facade。它覆盖两类能力：

- 模板能力：`ListTemplates`、`ListDBTemplates`。
- 任务管理能力：`ListEntries`、`GetEntry`、`CreateEntry`、`UpdateEntry`、`DeleteEntry`、`EnableEntry`、`DisableEntry`、`RunOnce`、运行日志和 runtime 查询。

边界规则：

- 模板只在项目 `crontab` 包硬编码声明并注册。
- Admin 不创建模板，也不维护业务侧 catalog。
- Admin 只通过 `ListDBTemplates` 获取 `AllowDB=true` 的模板，然后把模板绑定到 DB Entry。
- DB Entry 只能覆盖实例字段，不能覆盖 handler、timeout、allow_overlap、queue/task_type 等执行策略。

`crontab/infra/capability/operations` 负责把项目侧 `crontab.NewOperations` 注入到需要管理 crontab 的服务本地 capability 中。Admin router 作为 composition root，把 `core/crontab.Service` 传给 `CrontabHandler` 构造函数；handler 不通过 middleware 从 request context 获取 crontab service，避免把能力接线扩散到每个请求。

## Entry

Entry 是 runtime schedule instance：

```go
type Entry struct {
    ID          string
    Name        string
    TemplateKey string
    Spec        string
    Payload     string
    Source      Source
    Enabled     bool
    Timeout     time.Duration
    Distributed bool
    LockTTL     time.Duration
    LockKey     string
    MaxRunCount int64
}
```

DB 动态任务只允许覆盖 Entry 层字段：

```text
template_key/name
label
spec
payload
enabled
max_run_count
```

`MaxRunCount` / `max_run_count` 为 0 表示不限次数；大于 0 时，项目侧 Store 应在达到上限后停用 DB Entry，避免后续调度继续执行。

执行策略不从 DB 覆盖，统一来自 Template 和 Dispatch。

## Dispatch

`Registry.Dispatch` 是 runtime governance 的唯一入口。Scheduler 和 Runner 只触发 Entry，最终执行都进入 Dispatch。

Dispatch 顺序：

```text
1. normalize context / entry
2. ensure track_id
3. start crontab.execute span
4. validate template
5. build Job from Entry + Template
6. prepare RunContext
7. acquire entry-scoped lock when AllowOverlap=false
8. write running log and runtime state
9. inject JobLogger
10. execute Handler with timeout and panic recover
11. finish span
12. record metrics
13. write final log and runtime state
14. write runtime logger
15. notify failure callback when failed/timeout/panic
```

锁 key：

```text
crontab:entry:<entry_id>
```

`AllowOverlap=true` 或 `crontab.lock.enabled=false` 时不加锁。开启锁时，锁 TTL 优先使用任务 LockTTL，其次使用全局 `crontab.lock.ttl`，最后按 timeout 派生兜底值。

`RunOnce` 通过 Runner 同步返回本次执行结果。handler 失败、timeout、panic 会返回对应 error；锁冲突返回 `ErrJobRunning`。无论调用方是否处理 error，runtime 仍会写 running/final log、runtime state、metrics 和 failure callback。

`RuntimeState.SetSchedule` 以本次 reload 后的有效调度为准，已经从 DB 删除或不再调度的 Entry 会从内存状态中移除，避免后台运行态展示旧任务。

## Failure Callback

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

type FailureHandler func(ctx context.Context, report FailureReport)

func UseFailureHandler(handlers ...FailureHandler)
```

触发状态：

- `failed`
- `timeout`
- `panic`

不触发状态：

- `locked`
- `disabled`
- `skipped`
- `template_missing`
- `template_disabled`

failure handler 会继承 Dispatch context，因此日志可继续带 `track_id` / `trace_id`。handler panic 会被 runtime recover，不影响主执行链。

## Tracing

span 名称：

```text
crontab.execute
```

主要属性：

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

handler 内下游调用必须使用收到的 context。

## Logger

runtime logger 统一在 `core/crontab` 内输出：

- `crontab execute start`
- `crontab execute success`
- `crontab execute failed`
- `crontab execute timeout`
- `crontab overlap skipped`
- `crontab execute skipped`

日志字段至少包含：

- `template_key`
- `template_name`
- `entry_id`
- `run_id`
- `status`
- `duration_ms`
- context 中的 `track_id` / `trace_id`

error 日志必须带 `err` 字段，不打印敏感 payload。

## Metrics

```go
type RuntimeMetrics struct {
    CrontabExecuteTotal        int64
    CrontabExecuteSuccessTotal int64
    CrontabExecuteFailedTotal  int64
    CrontabExecuteDuration     time.Duration
    CrontabOverlapSkippedTotal int64
    CrontabTimeoutTotal        int64
}
```

`failed_total` 只统计 `failed`、`timeout`、`panic`。锁冲突计入 `overlap_skipped_total`。

## Removed Runtime Paths

已删除：

- `RunMiddleware`
- middleware pipeline
- `BuildRunPipeline`
- project crontab middleware registration
- Template events runtime
- Router DSL
- BuiltinJob registry
- compatibility template build path

保留：

- `RunHandler`
- `RunContext`
- `Template`
- `Registry.Dispatch`
- package-level `Register` / `MustRegister` for default registry

## Project Layout

```text
crontab/
  infra/            store / lock / realtime / storage adapters
  demo_*.go         示例 Template 声明和示例执行实现
  router.go         显式注册模板到 Registry
  server.go         服务装配
```

`demo_*.go` 同文件声明 Template 和示例执行函数，不做 `init()` 隐式注册。

## Update Record

- 2026-05-21：`crontab.lock.enabled` 开始控制 runtime 加锁；`RunOnce` 同步返回 handler 失败、timeout、panic 和 `ErrJobRunning`；runtime schedule reload 会清理已移除 Entry 的内存状态。
- 2026-05-17：新增 crontab operations facade，Admin 改为通过 `core/crontab.Service` 查询模板和管理 DB Entry；模板仍由项目 `crontab` 包硬编码注册，Admin 不维护 catalog。
- 2026-05-17：Template-Driven Runtime freeze。删除 crontab middleware runtime、Router DSL、BuiltinJob registry 和 Template events；Dispatch 接管 tracing、logger、metrics、timeout、overlap lock、run log、panic recover 和 failure callback。
