# Runtime 使用说明

Runtime 是框架收敛初始化权、服务生命周期和 runtime object registry 的基础入口。当前架构约束是：Runtime 管生命周期，Package 管能力 API，业务代码按包直接调用能力。

## 初始化

创建应用：

```go
app := runtime.New()
```

项目入口也可以使用 bootstrap facade：

```go
app := bootstrap.New()
```

`bootstrap.New` 返回的仍是 `*runtime.App`，bootstrap 只做默认装配，不持有 runtime 生命周期。

注册 capability：

以下示例中的 `*cap` 包均指对应 `core/*/facade` 包；业务代码仍使用各 core 根包 API。

```go
err := app.RegisterCapabilities(
	loggercap.Use(),
	databasecap.Use(),
	rediscap.Use(),
	cachecap.Use(),
	ratelimitcap.Use(),
	sessioncap.Use(),
	eventbus.Use(),
)
```

`RegisterCapabilities` 只保存 capability，不执行初始化。真正的 `Capability.Register(app)` 由 `Run` 统一执行，避免 config、logger、trace 等基础上下文尚未就绪时提前初始化资源。`Use` 是兼容别名，新增代码优先使用 `RegisterCapabilities` 表达 runtime 只负责注册和生命周期。

实际服务启动时仍由 bootstrap 负责读取配置，然后把配置传入 capability：

```go
app.RegisterCapabilities(
	loggercap.Use(loggercap.WithConfig(logCfg)),
	databasecap.Use(databasecap.WithConfig(dbCfg), databasecap.WithMode(appMode)),
	rediscap.Use(rediscap.WithConfig(redisCfg)),
	cachecap.Use(cachecap.WithConfig(cacheCfg)),
	ratelimitcap.Use(),
	sessioncap.Use(sessioncap.WithConfig(sessionCfg)),
	eventbus.Use(eventbus.WithConfig(eventbusCfg)),
)
```

平台资源的初始化入口统一收敛到 capability。Provider、infra、service 不再直接创建 logger、DB、Redis、cache、eventbus。业务代码不从 runtime app 取能力，直接使用 package API：

```go
db := database.Gorm(ctx)
rdb := redis.Client(ctx)
logger.Info("user synced")
queueRuntime := queue.Runtime(ctx)
```

注册 provider：

```go
err := app.Register(
	api.Provider(),
	worker.Provider(),
)
```

`Register` 只把 provider 注册到 runtime registry。`Run` 时按 dependency-aware boot order 调用：

```text
Provider.RuntimeCapabilities() // 收集 service local capability
Capability.Register(app)       // dependency-aware boot order
Provider.Register(app)         // dependency-aware boot order，全部完成
Provider.Start(ctx)            // dependency-aware boot order，再统一启动
```

Provider 可以通过可选接口声明服务私有能力。Runtime 会在 provider 启动前统一注册这些 capability，再做依赖解析、初始化和 shutdown 顺序管理：

```go
type RuntimeCapabilityProvider interface {
	RuntimeCapabilities() []runtime.CapabilityContract
}

func (p *Provider) RuntimeCapabilities() []runtime.CapabilityContract {
	return []runtime.CapabilityContract{
		openai.Use(
			openai.WithName("api.openai"),
			openai.WithConfig(p.cfg.OpenAI),
		),
	}
}
```

服务私有 capability 仍然要用稳定名称，推荐带服务前缀，例如 `api.openai`、`admin.payment`。Provider 依赖本地能力时显式声明：

```go
func (p *Provider) Dependencies() []runtime.Dependency {
	return []runtime.Dependency{
		runtime.Require("database"),
		runtime.Require("api.openai"),
	}
}
```

通过 bootstrap `ServiceBuilder.RuntimeCapabilities(...)` 声明服务能力时，bootstrap 会把返回的 capability 自动标记为 `runtime.ScopeServiceLocal`，并把能力名追加到 provider dependency。服务级 facade 应暴露 `WithName(...)`，确保 runtime 名称和 container bind key 使用同一个服务本地名称。

简单 facade 能力直接写在 `RuntimeCapabilities` 返回值里，不再额外定义 `apiQueueCapability(ctx)`、`adminSessionCapability(ctx)` 这类只做转调的薄函数。只有存在服务私有初始化、默认实例设置、自定义 metadata 或关闭逻辑时，才单独拆出 capability 函数或放进 `infra/capability`。

## Service Skeleton

业务项目可以用 runtime 的通用 service skeleton 承载服务注册、构建和 service-local capability：

```go
type Config struct {
	ConfigFile string
}

registry := runtime.NewServiceRegistry[*Config]()

registry.RegisterServiceDefinition(runtime.ServiceDefinition[*Config]{
	Type: "api",
	Kind: runtime.ServiceKindHTTP,
	ContextFactory: func(ctx runtime.ServiceContext[*Config]) (runtime.Service, error) {
		ctx.Capabilities.Set(ctx.Name+".queue.producer", producer)
		return runtime.HTTPService{
			InfoValue: runtime.ServiceInfo{
				Name:    ctx.Name,
				Enabled: true,
			},
			StartFunc: func() error {
				return server.ListenAndServe()
			},
			StopFunc: func(ctx context.Context) error {
				return server.Shutdown(ctx)
			},
		}, nil
	},
})

svc, err := registry.BuildService(
	"configs/config.yaml",
	"api",
	"api",
	"api",
	&Config{ConfigFile: "configs/config.yaml"},
)
```

构建后的 `ServiceInfo()` 会带上注册时的 `Type`、`Kind` 和本地能力名。能力名如果带服务名前缀，例如 `api.queue.producer`，展示时会压缩为 `queue.producer`。

如果服务需要运行时私有 capability，用 `RuntimeCapabilityFactory`：

```go
registry.RegisterServiceDefinition(runtime.ServiceDefinition[*Config]{
	Type: "api",
	RuntimeCapabilityFactory: func(ctx runtime.RuntimeCapabilityContext[*Config]) []runtime.CapabilityContract {
		return []runtime.CapabilityContract{
			openai.Use(openai.WithName(ctx.LocalName("openai"))),
		}
	},
})

caps := registry.RuntimeCapabilitiesForService(
	runtime.NewRuntimeCapabilityContext("configs/config.yaml", "api", "api", "", baseConfig),
)
```

`RuntimeCapabilitiesForService` 返回的 capability 会被标记为 `runtime.ScopeServiceLocal`。业务项目仍然负责读取 `services` 配置、注册实际 service factory，并决定这些 capability 如何进入 runtime app。

如果某个 provider `Start` 失败，runtime 会对当前失败 provider 和已经启动成功的 provider 按倒序调用 `Stop(ctx)` 做 rollback。

Phase 5 起，正常关闭、signal 关闭和 rollback 都统一走 runtime lifecycle：

```text
Provider.Stop(ctx)          // 倒序
Capability.Shutdown(ctx)    // 倒序
runtime context cancel
```

Phase 9 起，runtime 会统一维护 provider/capability status：

```go
api := app.ProviderStatus("api")
database := app.CapabilityStatus("database")

providers := app.ProviderStatuses()
capabilities := app.CapabilityStatuses()
```

注册 command：

```go
err := app.RegisterCommand(
	run.NewRuntimeCommand(),
	serve.NewRuntimeCommand(),
)
```

Command 通过 runtime 执行：

```go
err := runtime.Execute(app, os.Args)
```

当前执行器只做最小 args 分发：

```text
sdkitgo run api
sdkitgo run worker
sdkitgo serve
```

不提供复杂 flags、help、子命令树或自动发现。

## Registry / Metadata

Runtime registry 统一管理 capability、provider 和 command 的查找、列表、分组和 metadata。

对象都需要提供 metadata：

```go
type ProviderContract interface {
	Name() string
	Metadata() runtime.ProviderMetadata
	Dependencies() []runtime.Dependency
	Register(app *runtime.App) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Provider interface {
	ProviderContract
	Status() runtime.Status
}
```

metadata 支持：

```go
type ProviderMetadata struct {
	Name        string
	Description string
	Group       string
	Internal    bool
	Mode        runtime.ProviderMode
}
```

ProviderMode 用于区分长期服务和一次性任务：

```go
const (
	runtime.ProviderModeService // 长期运行，Start 必须阻塞到 ctx.Done 或 fatal error
	runtime.ProviderModeJob     // 一次性任务，Start 允许执行完成后返回
)
```

Capability 和 Command 分别使用 `CapabilityMetadata`、`CommandMetadata`，字段一致。

查找对象：

```go
provider, ok := app.Provider("api")
command, ok := app.Command("serve")
capability, ok := app.Capability("database")
```

列表和分组：

```go
providers := app.Providers()
commands := app.Commands()
capabilities := app.Capabilities()

apiProviders := app.ProvidersByGroup(runtime.GroupAPI)
systemCommands := app.CommandsByGroup(runtime.GroupSystem)
```

Capability metadata 额外支持 scope：

```go
runtime.ScopeGlobal       // database、redis、logger、queue 等进程级能力
runtime.ScopeServiceLocal // openai、payment、admin queue producer 等服务私有能力
```

查询服务私有能力：

```go
items := app.CapabilitiesByScope(runtime.ScopeServiceLocal)
```

`Internal=true` 只表示 runtime 内部对象标记，不做权限、插件可见性或自动发现。

## Capability Usage Boundary

Capability 分两层：

- core contract：`core/database`、`core/redis`、`core/logger`、`core/queue`，负责接口、helper API、runtime key。
- implementation：`pkg/*` 或 `app/<service>/infra/capability/*`，负责初始化第三方 SDK、配置和生命周期。

服务私有 capability 放在服务目录内，例如：

```txt
core/queue/facade/operations
app/api/infra/capability/openai
```

业务目录使用该 package 暴露的直接 API 或显式接口注入。`database.From(app)`、`queue.From(app)`、`logger.From(app)` 只用于 runtime wiring、provider startup、bootstrap lifecycle，不进入 handler 或业务 service。

## Runtime Adapter Boundary

外围系统通过 adapter 接入 runtime contract。Runtime Core 不直接依赖 cobra、bootstrap facade、pkg driver 或第三方库实现。

当前支持：

```go
runtime.AdapterTypeCommand
runtime.AdapterTypeCapability
runtime.AdapterTypeProvider
```

命令入口示例：

```go
adapter := runtime.NewCommandAdapter(runtime.AdapterMetadata{
	Name:     "cobra.serve",
	Internal: true,
}, serveCommand)

if err := app.RegisterCommandAdapters(adapter); err != nil {
	return err
}

return runtime.ExecuteContext(ctx, app, []string{"serve"})
```

能力和 Provider 接入示例：

```go
err := app.UseCapabilityAdapters(
	runtime.NewCapabilityAdapter(runtime.AdapterMetadata{Name: "bootstrap.redis"}, rediscap.Use()),
)

err = app.RegisterProviderAdapters(
	runtime.NewProviderAdapter(runtime.AdapterMetadata{Name: "provider.api"}, apiProvider),
)
```

Adapter 只做 integration bridge，不启动 provider，不关闭资源，不处理 signal，不维护 runtime status。注册后依然由 runtime registry 包装 capability/provider，并由 runtime 统一维护 lifecycle 和 health。

可查询 adapter metadata：

```go
adapters := app.Adapters()
commandAdapters := app.AdaptersByType(runtime.AdapterTypeCommand)
adapter, ok := app.Adapter("cobra.serve")
```

当前阶段不实现 adapter graph、动态绑定或自动发现。

## Plugin Boundary

Plugin 当前只用于 metadata introspection，不参与 lifecycle：

```go
type Plugin interface {
	Metadata() runtime.PluginMetadata
}
```

注册：

```go
err := app.RegisterPlugins(runtime.NewPlugin(runtime.PluginMetadata{
	Name:        "queue.redis",
	Description: "Redis queue integration",
	Group:       runtime.GroupSystem,
}))
```

查询：

```go
plugins := app.Plugins()
plugin, ok := app.Plugin("queue.redis")
systemPlugins := app.PluginsByGroup(runtime.GroupSystem)
```

当前阶段不实现 plugin loader、plugin lifecycle manager、热加载或自动发现。

## Dependency / Boot Order

Capability 和 Provider 都必须声明依赖。没有依赖时返回 nil：

```go
func (p *APIProvider) Dependencies() []runtime.Dependency {
	return runtime.RequireCapabilities("api.queue.producer")
}
```

Capability 可以使用带依赖的构造函数：

```go
queueCap := runtime.NewCapabilityWithDependencies(
	"queue",
	[]runtime.Dependency{runtime.Require("redis")},
	register,
	shutdown,
)
```

规则：

- `runtime.Require("redis")` 表示必需依赖，缺失会拒绝启动。
- `runtime.RequireCapabilities("database", "queue")` 是 provider 声明 capability 依赖的推荐写法。
- `runtime.Optional("redis")` 表示可选依赖，缺失不报错；如果目标存在，会参与排序。
- `runtime.OptionalCapabilities("redis")` 用于批量声明可选 capability。
- 重复 dependency 会拒绝启动。
- 循环 dependency 会拒绝启动。
- capability 只能依赖 capability。
- provider 可以依赖 capability 或 provider。

示例启动顺序：

```text
redis
queue
worker
```

对应关闭顺序：

```text
worker.stop
queue.stop / queue.shutdown
redis.shutdown
```

可直接查询 dependency 元数据：

```go
deps := app.Dependencies()
err := app.ValidateDependencies()
```

## Container

`Container` 是 runtime 里的最小资源容器：

```go
const KeyDB runtime.Key = "database"

err := app.Container().Bind(KeyDB, db)
value, ok := app.Container().Get(KeyDB)
value := app.Container().MustGet(KeyDB)
```

约束：

- key 不能为空。
- value 不能为 nil。
- `MustGet` 不触发 panic，未找到时返回 nil。

## App Context

`App` 内置 runtime context：

```go
ctx := app.Context()
```

`New()` 默认使用 `context.WithCancel(context.Background())`。调用 `Run(ctx)` 时，传入的 ctx 会派生为当前 runtime context，并透传给 provider `Start` / rollback `Stop`。

停止应用：

```go
err := app.Stop(context.Background())
```

`Stop(ctx)` 会先停止 Provider，再关闭 Capability，最后 cancel runtime context。传入的 ctx 没有 deadline 时，runtime 会默认使用 10 秒超时。

## Runtime Status / Health

Runtime status 只表示运行态：

```text
booting
running
stopping
stopped
failed
```

生命周期转换：

```text
booting -> running
running -> stopping -> stopped
failed
```

查询单个对象：

```go
health := app.ProviderStatus("api")
if health.Status == runtime.StatusFailed {
	return health.Error
}
```

查询列表：

```go
for _, health := range app.CapabilityStatuses() {
	fmt.Println(health.Name, health.Status)
}
```

`Health.Error` 只记录 runtime 持有的失败原因，例如 dependency 缺失、register/start 失败、shutdown timeout 或 shutdown error。当前阶段不做 metrics、dashboard、watch stream 或复杂 healthcheck。

## Provider 约束

Provider 不负责初始化外部资源。

禁止在 provider 中直接执行：

```go
gorm.Open(...)
redis.NewClient(...)
```

Provider 只从 runtime container 读取已经由 capability 初始化完成的资源。

推荐读取方式：

```go
log := logger.From(app)
db := database.From(app)
rdb := redis.From(app)
c := cache.From(app)
bus := eventbus.From(app)
```

当前已有全局变量仍保留兼容，但新增 runtime/provider 代码优先使用 `From(app)`。

Queue 是模块级 runtime platform。Handler 投递任务时直接使用 package API：

```go
info, err := queue.Enqueue(ctx, task)
```

Provider 或 command 需要队列管理能力时，优先读取 runtime instance 和 operations：

```go
queueRuntime := queue.Runtime(ctx)
operations := queueRuntime.Operations()

status, err := operations.RuntimeStatus(ctx)
metrics, err := operations.Metrics(ctx)
```

handler 执行链内使用：

```go
metadata, ok := queue.MetadataFromContext(ctx)
```

`core/queue` 不再提供包级默认实例。新增 provider/server/command 必须通过 runtime context、service local facade 或显式 client/operations 读取队列能力。

Provider 名称必须稳定、唯一、不可为空。`Metadata().Name` 未设置时 runtime 会回退使用 `Name()`。`provider`、`default`、`main` 是保留名，不能作为 Provider 名称。

Provider 生命周期边界：

- `Register(app)`：只做组装，包括读取 capability、构建 service/handler/middleware、注册 route、初始化纯内存结构。
- `Start(ctx)`：启动 server、consumer、scheduler、websocket、goroutine、background task。`service` 模式必须阻塞到 `ctx.Done()` 或 provider fatal error；`job` 模式允许完成后返回。
- `Stop(ctx)`：关闭长期运行资源，等待 goroutine/consumer/server 退出。

`Register(app)` 必须无运行副作用，不启动 goroutine，不连接外部系统，不启动 queue consumer、websocket、cron 或 socket listener。Provider 可以在 `Register(app)` 保存 `app *runtime.App` 引用，后续在 `Start/Stop` 中按需读取 runtime capability，但不要长期缓存 DB、Redis、Logger 等资源副本。

Provider 不自行维护 runtime 运行状态。运行状态由 runtime 在 `Register` / `Start` / `Stop` 周期中统一写入，可通过 `app.ProviderStatus(name)` 查询。

聚合 serve 模式下多个 Provider 共享同一个 runtime：

```go
app := bootstrap.New()

app.RegisterCapabilities(
	loggercap.Use(),
	databasecap.Use(),
	rediscap.Use(),
	cachecap.Use(),
	ratelimitcap.Use(),
	sessioncap.Use(),
	eventbus.Use(),
)

app.Register(
	api.Provider(),
	worker.Provider(),
)

return runtime.RunAllProviders(ctx, app)
```

约束是全部 Provider 先完成 `Register`，然后才进入 `Start`。Provider 不能假设自己是唯一服务。

## Run / Serve 命令

`run` 命令只启动目标 Provider：

```go
app := bootstrap.New()

app.RegisterCapabilities(...)
app.Register(
	api.Provider(),
	worker.Provider(),
)

return runtime.RunProvider(ctx, app, "api")
```

执行顺序：

```text
Capability.Register(app)
api.Register(app)
api.Start(ctx)
```

如果 `api` 声明了 provider dependency，runtime 会先启动这些依赖；无关 Provider 不会被注册或启动。

`serve` 命令启动当前 App 内全部 Provider：

```go
app := bootstrap.New()

app.RegisterCapabilities(...)
app.Register(
	api.Provider(),
	worker.Provider(),
)

return runtime.RunAllProviders(ctx, app)
```

执行顺序：

```text
Capability.Register(app) // dependency-aware boot order
Provider.Register(app)   // dependency-aware boot order
Provider.Start(ctx)      // dependency-aware boot order
```

同一次 `serve` 生命周期内，capability 只初始化一次。

Command 本身不初始化 DB、Redis、Logger，不持有 provider 或 capability 全局变量。Command 只调用 runtime，例如 `RunProvider`、`RunAllProviders` 或 `Execute`。cobra 只作为 command adapter，负责把 CLI 参数交给 runtime command，不持有 runtime lifecycle。

`run` / `serve` command 不直接关闭 server 或底层资源，也不直接调用 `app.Stop()`。入口级运行使用 package-level 方法：

```go
err := runtime.RunProvider(ctx, app, "api")
err := runtime.RunAllProviders(ctx, app)
```

这两个方法会在启动返回后统一进入 `app.Stop(context.Background())`，因此 provider shutdown、capability shutdown、signal 和 rollback 都在 runtime lifecycle 内完成。

## Service Runtime Integration

Phase 12 已接入五类真实服务：

```bash
sdkitgo run api
sdkitgo run admin
sdkitgo run worker
sdkitgo run crontab
sdkitgo run realtime
sdkitgo serve
```

`sdkitgo serve` 会从 `services` 配置读取启用项，并将 `api`、`admin`、`worker`、`crontab`、`realtime` 注册为 `service` 模式 runtime provider。需要 eventbus 的服务共享同一个 runtime eventbus capability；API Service Model 不依赖 eventbus。

生命周期边界：

- `api.Register`：读取配置，构建 router/middleware/handler；`api.Start`：`ListenAndServe` 并阻塞；`api.Stop`：`http.Server.Shutdown(ctx)`。
- `admin.Register`：读取配置、构建后台 router/middleware/handler；`admin.Start`：`ListenAndServe` 并阻塞；`admin.Stop`：`http.Server.Shutdown(ctx)`。
- `worker.Register`：读取配置、准备 Queue Runtime Host；`worker.Start`：初始化 `queue.RuntimeKernel`、注册 event registry、启动 queue consumer 并跟随 runtime context；`worker.Stop`：关闭 consumer、queue kernel 和 tracing。
- `crontab.Provider`：声明 `type=crontab` 服务工厂；`crontab.Start`：注册模板、构建 scheduler、启动 scheduler 和 reload loop，并阻塞到 runtime context 结束；`crontab.Stop`：停止 scheduler。
- `realtime.Register`：组装 router、middleware、WebSocket/SSE route、eventbus binding 和 connection manager；`realtime.Start`：启动 HTTP gateway、WebSocket/SSE 连接生命周期和 eventbus consumer；`realtime.Stop`：关闭连接、subscriber、consumer、HTTP server 和 gateway runtime。

依赖声明：

```text
bootstrap -> logger/database/redis/cache/queue/eventbus -> api/admin/worker/crontab/realtime
```

`filesystem` 是 bootstrap 公共能力。`api` 显式依赖 `database`、`logger`，并依赖 `api.session`、`api.queue.producer` 等本地能力；`admin` 显式依赖 `database`、`logger`、`eventbus`，并依赖 `admin.session`、`admin.queue.operations`、`admin.example` 等本地能力；`worker` 显式依赖 `redis`、`logger`、`eventbus`；`crontab` 显式依赖 `logger`、`eventbus`，并将 `database`、`redis` 声明为可选依赖。`realtime` 显式依赖 `logger`、`eventbus`，并将 `redis`、`auth` 声明为可选依赖。运行状态和 signal/shutdown 都由 runtime 持有。

启动表格中的能力展示只读 runtime capability metadata，不维护能力名称黑名单：`ScopeGlobal` 进入公共能力表；`ScopeServiceLocal` 进入服务行，并去掉服务名前缀，例如 `api.queue.producer` 展示为 `queue.producer`、`admin.queue.operations` 展示为 `queue.operations`。真实初始化顺序和依赖校验以 runtime dependency 为准。

## API Service Model

API 服务目录用于承载普通业务开发：

```text
app/api/
  provider.go
  router.go
  server.go
  handler/
  middleware/
```

开发规则：

- 路由集中从 `router.go` 进入，根分组为 `/api`。
- handler 直接承载请求解析、校验、业务逻辑和响应输出。
- handler 可以直接使用 `database.Gorm(c)`、`database.PGX(c)`、`redis.Client(c)`、从 request context 注入的 `queue.Client`、`eventbus.Publish`。
- 不再为简单 API 强制拆 `service` / `repository` / handler struct / RouterDependencies。
- middleware 统一放在 `app/api/middleware`，基础链路保持 recovery、tracking、tracing、request id、access log。

当前最小 API：

```text
GET /api/ping
GET /api/user/profile
GET /api/dashboard/stats
GET /api/redis/cache
POST /api/queue/push
POST /api/queue/demo
POST /api/eventbus/publish
POST /api/transaction/demo
```

`GET /api/user/profile` 是直接 handler 示例：

```go
func Profile(c *gin.Context) {
    db := database.Gorm(c)
    response.Success(c, data)
}
```

Realtime Gateway 约束：

- `realtime` provider 必须是 `ProviderModeService`，`Start(ctx)` 长期阻塞运行。
- `Register` 不启动 websocket loop、SSE push loop、goroutine 或后台任务。
- `Stop` 必须释放 connection manager、WebSocket/SSE 连接、eventbus subscription 和本 provider 持有的 runtime gateway。
- 注入的 shared `eventbus` 不由 realtime provider 关闭，统一交给 eventbus capability shutdown。

## Graceful Shutdown

Runtime 默认监听：

```text
SIGINT
SIGTERM
```

因此 `ctrl + c` 会触发同一个 `app.Stop()` 流程。

Capability 如果持有资源，需要实现 Shutdown：

```go
runtime.NewCapabilityWithShutdown(
	"redis",
	func(app *runtime.App) error {
		return app.Container().Bind(redis.KeyRedis, client)
	},
	func(ctx context.Context) error {
		return client.Close()
	},
)
```

平台 capability 已内置 shutdown 行为：

- `cachecap.Use()`：关闭默认缓存。
- `rediscap.Use()`：关闭默认 Redis 客户端。
- `databasecap.Use()`：关闭默认数据库连接。
- `ratelimitcap.Use()`：恢复默认限流 store。
- `sessioncap.Use()`：绑定默认会话 store。
- `eventbus.Use()`：关闭默认事件总线。
- `loggercap.Use()`：flush logger。

Provider 只释放自己启动的运行资源，例如 HTTP server、queue consumer、cron、websocket、后台 goroutine。DB、Redis、Logger、EventBus 等基础资源交给对应 capability 关闭。

如果 Provider `Stop(ctx)` 或 Capability `Shutdown(ctx)` 返回 error，或关闭 ctx 超时，runtime 会将对应对象标记为 `failed`，并在 `Health.Error` 中保留错误。

## Queue Runtime 使用

Queue Runtime Platform 已接入显式 runtime instance。handler 执行链中优先从 context 读取：

```go
runtime := queue.Runtime(ctx)
operations := runtime.Operations()
metadata := runtime.Metadata()
```

启动接线层需要读取 service context 时使用对应 facade：

```go
runtime := queueproducer.RuntimeFromServiceContext(serviceCtx)
adminRuntime := queueops.RuntimeFromServiceContext(serviceCtx)
```

Worker host 会在启动 consumer 前初始化 queue runtime。该 runtime 只属于 worker consumer 生命周期，不写入 `ServiceContext.Capabilities`。业务注册通过当前 runtime 创建 registry：

```go
runtime, err := workerbootstrap.EnsureQueueRuntime(cfg)
if err != nil {
    return err
}

return worker.RegisterEvents(runtime.NewRegistry())
```

Admin API 和 `sdkitgo queue` command 使用 `OperationsRuntime` 复用队列查询、retry、failed/archive、clean/delete、pause、resume、stats 等操作。`sdkitgo queue` 的实现归 `worker/command/queue` 持有，`sdkitgo cron` 的实现归 `crontab/command/cron` 持有，根 `command` 包只聚合注册服务命令。新增代码使用 runtime context、service local facade 或显式 client/operations 注入，`queue.From(app)` 只保留在 runtime wiring、provider startup 和 bootstrap lifecycle。

Queue transport ownership 属于具体 host：

- Admin host 注册 Gin route，并负责鉴权、response、validator 和 middleware。
- 服务自己的 Command host 注册 Cobra command，并负责参数绑定和输出格式。
- Worker host 启动 queue consumer，并负责 graceful drain/shutdown。
- `core/queue` 只暴露 runtime、operations、metadata、status、metrics 和标准接口，不暴露 route/command 注册函数。

## Runtime Ownership 规范

Runtime 是唯一 orchestration 中心，拥有 context、signal、registry、provider lifecycle、capability lifecycle、status、health、shutdown、execute、run 和 serve。

bootstrap 是 facade，只允许创建 `*runtime.App`、装配默认 capability、加载公共配置和注册项目入口。bootstrap 不持有 registry，不启动 provider，不处理 signal，不做 shutdown 编排。

command 只把用户意图转交给 runtime。新增 run/serve 命令必须调用 `runtime.RunProvider` 或 `runtime.RunAllProviders`，不能直接启动 provider 或关闭底层资源。

外部接入规则：

```text
cobra -> command adapter -> runtime command -> runtime
bootstrap -> facade / capability adapter -> runtime
pkg/driver -> capability adapter / provider adapter -> runtime
```

Adapter 和 Plugin 都不能维护 runtime lifecycle、status、signal 或 shutdown。Runtime registry 返回 runtime-managed object，外部对象不能覆盖 runtime status ownership。

Runtime 统一拥有 failure ownership：dependency failure、boot failure、start failure、shutdown timeout 和 shutdown error 都由 runtime 写入 status/health。Provider、Capability、Command 不自行修正或恢复 runtime status。

## 最小示例

```go
app := runtime.New()

if err := app.RegisterCapabilities(
	loggercap.Use(),
	databasecap.Use(),
	rediscap.Use(),
	cachecap.Use(),
	ratelimitcap.Use(),
	sessioncap.Use(),
	eventbus.Use(),
); err != nil {
	return err
}

if err := app.Register(
	api.Provider(),
); err != nil {
	return err
}

if err := app.RegisterCommand(
	run.NewRuntimeCommand(),
	serve.NewRuntimeCommand(),
); err != nil {
	return err
}

return runtime.Execute(app, os.Args)
```

如果不走 command，也优先使用 package-level runtime 入口：

```go
return runtime.RunAllProviders(ctx, app)
```

## 当前限制

- 不包含复杂 lifecycle registry。
- Dependency 不包含版本约束。
- 不生成 dependency graph visualization。
- Command 执行器只支持最小 args 分发。
- 不做 reload / hot restart。
- 不做复杂 hook、metrics、dashboard、watch stream 或复杂 healthcheck。
- 不做 plugin loader、plugin lifecycle、plugin visibility、hot reload 或自动发现。
- 不做 adapter graph 或动态 adapter binding。
