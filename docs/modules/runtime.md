# Runtime

## 作用

Runtime 是框架唯一 orchestration 中心，也是统一初始化权、capability、provider、command contract 和 runtime object registry 的基础模块。它负责稳定 capability 初始化顺序、Provider 生命周期边界、signal/shutdown ownership，以及单服务和聚合服务运行时共享同一组基础能力。

## 初始化

入口：

```go
app := runtime.New()
```

项目入口可以使用 bootstrap facade 创建 runtime：

```go
app := bootstrap.New()
```

推荐顺序：

```go
app.RegisterCapabilities(...)
app.Register(...)
app.RegisterCommand(...)
return runtime.Execute(app, os.Args)
```

## 配置项

Runtime 本身不读取配置。

配置仍由 bootstrap 或具体 capability 装配。平台能力通过 capability API 接收配置：

以下示例中的 `*cap` 包均指对应 `core/*/facade` 包；业务代码仍使用各 core 根包 API。

```go
loggercap.Use(loggercap.WithConfig(logCfg))
databasecap.Use(databasecap.WithConfig(dbCfg), databasecap.WithMode(appMode))
rediscap.Use(rediscap.WithConfig(redisCfg))
cachecap.Use(cachecap.WithConfig(cacheCfg))
ratelimitcap.Use()
sessioncap.Use(sessioncap.WithConfig(sessionCfg))
eventbus.Use(eventbus.WithConfig(eventbusCfg))
```

## 对外接口

### App

```go
type App struct {
	container *Container
	registry  *Registry
	ctx       context.Context
	cancel    context.CancelFunc
}
```

主要方法：

- `New() *App`
- `Context() context.Context`
- `Container() *Container`
- `Use(...CapabilityContract) error`
- `RegisterCapabilities(...CapabilityContract) error`
- `Register(...ProviderContract) error`
- `RegisterCommand(...Command) error`
- `RegisterAdapters(...Adapter) error`
- `UseCapabilityAdapters(...CapabilityAdapter) error`
- `RegisterProviderAdapters(...ProviderAdapter) error`
- `RegisterCommandAdapters(...CommandAdapter) error`
- `RegisterPlugins(...Plugin) error`
- `Run(...context.Context) error`
- `RunAllProviders(...context.Context) error`
- `RunProvider(name string, ...context.Context) error`
- `Stop(ctx context.Context) error`
- `Capabilities() []Capability`
- `Capability(name string) (Capability, bool)`
- `CapabilitiesByGroup(group string) []Capability`
- `CapabilitiesByScope(scope string) []Capability`
- `Providers() []Provider`
- `Provider(name string) (Provider, bool)`
- `ProvidersByGroup(group string) []Provider`
- `ProviderStatus(name string) Health`
- `ProviderStatuses() []Health`
- `CapabilityStatus(name string) Health`
- `CapabilityStatuses() []Health`
- `Commands() []Command`
- `Command(name string) (Command, bool)`
- `CommandsByGroup(group string) []Command`
- `Adapters() []Adapter`
- `Adapter(name string) (Adapter, bool)`
- `AdaptersByType(adapterType string) []Adapter`
- `Plugins() []Plugin`
- `Plugin(name string) (Plugin, bool)`
- `PluginsByGroup(group string) []Plugin`
- `Dependencies() []DependencyMetadata`
- `ValidateDependencies() error`
- `RunProvider(ctx context.Context, app *App, name string) error`
- `RunAllProviders(ctx context.Context, app *App) error`

### Registry / Metadata

Runtime 内部持有 `Registry`，统一管理 capability、provider 和 command 的注册、索引、列表、查找和元数据。Registry 只做 metadata、lookup、index 和 register，不启动 provider，不关闭 provider，不管理 lifecycle。

Runtime Core 的 metadata 只记录 runtime contract 自身。模块级 runtime metadata 由各模块维护，例如 `core/queue` 的 `RuntimeMetadata`、`RegistryMetadata` 和 `OperationsRuntime` 记录 queue driver、worker、queue weight、retry、timeout、delay、trace、middleware、status 和 metrics。Runtime app 只通过 capability container 暴露这些对象，不把 queue driver internals 提升到 runtime core。

metadata 结构：

```go
type CapabilityMetadata struct {
	Name        string
	Description string
	Group       string
	Scope       string
	Internal    bool
}

type ProviderMetadata struct {
	Name        string
	Description string
	Group       string
	Internal    bool
	Mode        ProviderMode
}

type RuntimeCapabilityProvider interface {
	RuntimeCapabilities() []CapabilityContract
}

type CommandMetadata struct {
	Name        string
	Description string
	Group       string
	Internal    bool
}

type DependencyMetadata struct {
	Source   string
	Target   string
	Required bool
}

type Status string

type Health struct {
	Name   string
	Status Status
	Error  error
}

type AdapterMetadata struct {
	Name     string
	Type     string
	Internal bool
}

type PluginMetadata struct {
	Name        string
	Description string
	Group       string
	Internal    bool
}
```

内置 group 常量：

```go
const (
	GroupAPI      = "api"
	GroupWorker   = "worker"
	GroupSystem   = "system"
	GroupInternal = "internal"
)
```

内置 capability scope 常量：

```go
const (
	ScopeGlobal       = "global"
	ScopeServiceLocal = "service_local"
)
```

查询示例：

```go
provider, ok := app.Provider("api")
command, ok := app.Command("serve")
capability, ok := app.Capability("database")

apiProviders := app.ProvidersByGroup(runtime.GroupAPI)
systemCommands := app.CommandsByGroup(runtime.GroupSystem)
serviceLocalCaps := app.CapabilitiesByScope(runtime.ScopeServiceLocal)
```

`Internal` 只表示 runtime 内部对象标记，当前不实现 plugin visibility 或权限控制。

## Runtime Capability Architecture

当前 runtime capability 架构收敛为三层边界：

- Runtime 只负责 capability register、dependency graph、init order、provider lifecycle、shutdown reverse order 和 config inject。
- Package 负责 capability API 和业务 helper，例如 `database.Gorm(ctx)`、`redis.Client(ctx)`、`logger.Info(...)`、`queue.Enqueue(ctx, task)`。
- Business 按包直接调用，不把 `database.From(app)`、`queue.From(app)`、`logger.From(app)` 作为业务依赖扩散。

`From(app)` 类 API 只允许出现在 runtime wiring、provider startup 和 bootstrap lifecycle 中。业务 handler、service、domain logic 优先使用 package direct API 或显式注入的接口。

core 模块运行时文件约定：

- `core/<module>/binding.go`：只放 `KeyXXX`、`From(app)`、`Bind(app, value)` 等 root default 与 runtime container 的绑定原语。
- `core/<module>/facade/use.go`：只放 `Use(...)`、`WithConfig(...)`、`WithConfigLoader(...)`、依赖声明和 shutdown 生命周期。
- `core/<module>` 根包不实现 `Use` / `UseOption`；业务代码优先使用根包 API，例如 `database.Gorm(ctx)`、`redis.Client(ctx)`、`cache.Default()`、`eventbus.Publish(ctx, ev)`。

Capability 分为两类 scope：

- `runtime.ScopeGlobal`：进程级基础能力，例如 logger、database、redis、cache、filesystem、eventbus。
- `runtime.ScopeServiceLocal`：服务私有能力，例如 `api.session`、`api.queue.producer`、`admin.session`、`admin.queue.operations`、`api.openai`、`admin.payment`、`worker.oss`。

服务本地能力优先放在 `<service>/infra/capability/<name>`。简单 facade bridge 直接在该服务的 `RuntimeCapabilities` 列表中声明，不再包一层 `apiQueueCapability(ctx)` / `adminSessionCapability(ctx)` 这类只转调 facade 的薄函数；只有存在服务私有初始化、默认实例设置、自定义 metadata 或关闭逻辑时，才单独拆出 capability 函数或 infra package。该层负责第三方 SDK 或服务私有 runtime 的初始化、默认实例设置和关闭；业务目录只调用该 package 或 `infra/*` adapter 暴露的 API。

## Service Skeleton

Runtime 提供业务框架可复用的 service skeleton，但不读取业务配置，也不持有具体业务装配。

核心类型：

- `ServiceKind`：`http`、`queue`、`cli`
- `ServiceInfo`：服务名称、类型、地址、启用状态和本地能力展示信息
- `Service`：`Start(context.Context)` / `Shutdown(context.Context)` 生命周期接口
- `ServiceSpec`：`services.<name>` 下的 `type`、`enabled`、`config_key` 通用结构
- `ServiceContext[T]`：服务构建上下文，`T` 是业务项目自己的总配置类型
- `ServiceRegistry[T]`：按 service type 注册工厂并构建服务
- `LocalCapabilityRegistry`：保存 service-local 能力实例，关闭时按注册倒序释放

`ServiceRegistry[T]` 只负责框架规则：

- 根据 `ServiceSpec.Type` 查找服务工厂
- 把 `config_file`、`name`、`type`、`config_key`、`base config` 和本地能力注册表传给工厂
- 把工厂声明的本地能力名合并进 `ServiceInfo.Capabilities`
- 把 service runtime capability 标记为 `ScopeServiceLocal`
- 通过 `ServiceKind(serviceType)` 暴露已注册服务类型的 kind，供上层按 HTTP/Queue/CLI 类型声明公共 capability 依赖，避免按服务名称硬编码。

业务项目仍然负责：

- 从配置文件读取 `services` map
- 定义自己的总配置结构
- 注册具体 service factory
- 装配 database、redis、auth、models、migrate、seed 等业务初始化逻辑

示例：

```go
registry := runtime.NewServiceRegistry[*Config]()

registry.RegisterServiceDefinition(runtime.ServiceDefinition[*Config]{
	Type: "api",
	Kind: runtime.ServiceKindHTTP,
	ContextFactory: func(ctx runtime.ServiceContext[*Config]) (runtime.Service, error) {
		cfg, err := api.Load(ctx.ConfigFile, ctx.Name, ctx.Base, ctx.ConfigKey)
		if err != nil {
			return nil, runtime.WrapServiceConfigError(ctx.Name, ctx.Type, ctx.ConfigKey, err)
		}
		return api.NewServerWithContext(cfg, &ctx)
	},
})
```

`RuntimeCapabilityContext[T]` 用于声明 service-local runtime capability：

```go
registry.RegisterServiceDefinition(runtime.ServiceDefinition[*Config]{
	Type: "api",
	RuntimeCapabilityFactory: func(ctx runtime.RuntimeCapabilityContext[*Config]) []runtime.CapabilityContract {
		cfg, err := api.Load(ctx.ConfigFile, ctx.Name, ctx.BaseConfig(), ctx.ConfigKey)
		if err != nil {
			return []runtime.CapabilityContract{
				runtime.NewCapability(ctx.LocalName("config.error"), func(*runtime.App) error { return err }),
			}
		}
		return []runtime.CapabilityContract{
			sessioncap.Use(sessioncap.WithName(ctx.LocalName("session")), sessioncap.WithConfig(cfg.Session)),
		}
	},
})
```

Provider 依赖 capability 时使用显式名称：

```go
func (p *Provider) Dependencies() []runtime.Dependency {
	return runtime.RequireCapabilities("database", "queue")
}
```

Bootstrap 是 runtime 约定的配置加载锚点，名称由 core runtime 统一维护：

```go
const (
	CapabilityBootstrap      = "bootstrap"
	CapabilityBootstrapError = "bootstrap.error"
)

runtime.OptionalBootstrap()
runtime.RequireBootstrap()
```

core facade 需要等待 bootstrap 先执行时，使用 `runtime.OptionalBootstrap()`，不要手写 `"bootstrap"` 字符串。业务项目可以用自己的 capability 实现这个名称，但 `Config` 结构和配置加载逻辑仍由业务项目持有。

禁止在 provider dependency 中通过 `database.Capability()` 一类实现对象表达依赖。Capability 的具体实现由 runtime app 在启动时注册决定。

Provider 如果需要声明服务私有 capability，实现 `RuntimeCapabilityProvider`：

```go
func (p *Provider) RuntimeCapabilities() []runtime.CapabilityContract {
	return []runtime.CapabilityContract{
		openai.Use(openai.WithName("api.openai")),
		payment.Use(payment.WithName("admin.payment")),
	}
}
```

项目装配时先注册公共 capability，再注册 provider；provider 注册阶段 runtime 会收集 service local capability。`Run` 时统一做 dependency resolve、capability 初始化、provider register/start 和倒序 shutdown。禁止在 `Provider.Start` 中临时注册 capability。

bootstrap 的 `ServiceBuilder.RuntimeCapabilities(...)` 是项目服务接入这套机制的默认入口。它会把服务本地能力标记为 `ScopeServiceLocal`，并要求能力名使用服务命名空间，例如 `api.openai`、`admin.payment`、`worker.oss`。

简单服务级 facade 推荐直接写在 `RuntimeCapabilities` 返回值里：

```go
RuntimeCapabilities(func(ctx bootstrap.RuntimeCapabilityContext) []runtime.CapabilityContract {
	return []runtime.CapabilityContract{
		queueproducer.Use(
			queueproducer.WithName(ctx.LocalName(queueproducer.Name)),
			queueproducer.WithConfigLoader(func(*runtime.App) (queueproducer.Config, error) {
				cfg, err := apiconfig.Load(ctx.ConfigFile, ctx.Name, ctx.BaseConfig(), ctx.ConfigKey)
				if err != nil {
					return queueproducer.Config{}, err
				}
				return cfg.Queue, nil
			}),
		),
	}
})
```

### Runtime Adapter

Adapter 是外围系统接入 runtime contract 的边界对象。Runtime Core 只认识 runtime contract，不直接依赖 cobra、bootstrap facade、pkg driver 或第三方库实现。

当前支持三类 adapter：

```go
const (
	AdapterTypeCommand    = "command"
	AdapterTypeCapability = "capability"
	AdapterTypeProvider   = "provider"
)
```

contract：

```go
type Adapter interface {
	AdapterMetadata() AdapterMetadata
}

type CommandAdapter interface {
	Adapter
	Command() Command
}

type CapabilityAdapter interface {
	Adapter
	Capability() CapabilityContract
}

type ProviderAdapter interface {
	Adapter
	Provider() ProviderContract
}
```

注册方式：

```go
app.RegisterCommandAdapters(
	runtime.NewCommandAdapter(runtime.AdapterMetadata{Name: "cobra.serve"}, serveCommand),
)

app.UseCapabilityAdapters(
	runtime.NewCapabilityAdapter(runtime.AdapterMetadata{Name: "bootstrap.logger"}, loggercap.Use()),
)

app.RegisterProviderAdapters(
	runtime.NewProviderAdapter(runtime.AdapterMetadata{Name: "provider.api"}, apiProvider),
)
```

Adapter 只做 integration bridge，不持有 lifecycle、status、shutdown、signal 或 dependency ownership。Adapter 注册后仍然由 runtime registry 包装 contract，并由 runtime 维护状态。

当前阶段不实现 adapter graph、adapter lifecycle、自动发现或动态绑定。

### Plugin Boundary

Plugin 只定义 metadata boundary，用于 runtime plugin introspection：

```go
type Plugin interface {
	Metadata() PluginMetadata
}
```

注册和查询：

```go
app.RegisterPlugins(runtime.NewPlugin(runtime.PluginMetadata{
	Name:  "queue.redis",
	Group: runtime.GroupSystem,
}))

plugins := app.Plugins()
plugin, ok := app.Plugin("queue.redis")
systemPlugins := app.PluginsByGroup(runtime.GroupSystem)
```

当前阶段不实现 plugin loader、plugin lifecycle、热加载、自动发现或 visibility 权限模型。

### Container

```go
type Container struct {
	values sync.Map
}
```

Container key 使用强类型，避免裸字符串冲突：

```go
type Key string
```

主要方法：

- `Bind(key Key, value any) error`
- `Get(key Key) (any, bool)`
- `MustGet(key Key) any`

### Capability

```go
type CapabilityContract interface {
	Name() string
	Metadata() CapabilityMetadata
	Dependencies() []Dependency
	Register(app *App) error
	Shutdown(ctx context.Context) error
}

type Capability interface {
	CapabilityContract
	Status() Status
}
```

支持函数形式：

```go
type CapabilityFunc func(app *App) error
```

直接使用 `CapabilityFunc` 没有真实名称，`Use` 会拒绝。推荐使用：

```go
runtime.NewCapability("database", func(app *runtime.App) error {
	return app.Container().Bind(KeyDB, db)
})
```

需要描述、分组或 internal 标记时使用：

```go
runtime.NewCapabilityWithMetadata(
	runtime.CapabilityMetadata{
		Name:        "database",
		Description: "Database connection capability",
		Group:       runtime.GroupSystem,
	},
	register,
	shutdown,
)
```

如果 capability 持有需要释放的资源，使用：

```go
runtime.NewCapabilityWithShutdown(
	"database",
	func(app *runtime.App) error {
		return app.Container().Bind(KeyDB, db)
	},
	func(ctx context.Context) error {
		return db.Close()
	},
)
```

声明依赖时使用：

```go
runtime.NewCapabilityWithDependencies(
	"queue",
	[]runtime.Dependency{runtime.Require("redis")},
	register,
	shutdown,
)
```

`Required=false` 表示可选依赖。可选依赖缺失时允许启动；如果目标存在，runtime 会按依赖顺序排序。

Phase 2 已提供平台 capability：

- `loggercap.Use()`，Container key 为 `logger.KeyLogger`。
- `databasecap.Use()`，Container key 为 `database.KeyDatabase`。
- `rediscap.Use()`，Container key 为 `redis.KeyRedis`。
- `cachecap.Use()`，Container key 为 `cache.KeyCache`。
- `ratelimitcap.Use()`，Container key 为 `ratelimit.KeyRateLimit`。
- `sessioncap.Use()`，Container key 为 `session.KeySession`。
- `validatorcap.Use()`，Container key 为 `validator.KeyValidator`。
- `eventbus.Use()`，Container key 为 `eventbus.Name`。

这些 capability 负责初始化资源、维护兼容用的全局默认实例，并写入 runtime container。

Phase 5 起平台 capability 支持 shutdown contract：`cache` 清理默认缓存，`redis` 关闭默认客户端，`database` 关闭默认连接池，`logger` 执行 flush；`validator` 没有外部资源，关闭阶段为 no-op。

命名约束：

- `Name()` 必须稳定、唯一、不可为空。
- `Metadata().Name` 必须稳定唯一，未设置时 runtime 会回退使用 `Name()`。
- 禁止使用 `capability`、`default`、`main` 作为 Capability 名称。
- `Use` 会拒绝重复 Capability 名称。

### Provider

```go
type ProviderContract interface {
	Name() string
	Metadata() ProviderMetadata
	Dependencies() []Dependency
	Register(app *App) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Provider interface {
	ProviderContract
	Status() Status
}
```

Provider 是服务运行单元，不是资源初始化器。

ProviderMode 用于声明运行单元的生命周期语义：

```go
const (
	ProviderModeService ProviderMode = "service"
	ProviderModeJob     ProviderMode = "job"
)
```

`service` 是长期运行服务，`Start(ctx)` 必须阻塞到 `ctx.Done()` 或 provider fatal error。`job` 是一次性任务，`Start(ctx)` 允许执行完成后返回；未声明 mode 时按 `job` 处理。

命名约束：

- `Name()` 必须稳定、唯一、不可为空。
- `Metadata().Name` 必须稳定唯一，未设置时 runtime 会回退使用 `Name()`。
- 禁止使用 `provider`、`default`、`main` 作为 Provider 名称。
- `Register` 会拒绝重复 Provider 名称。

生命周期边界：

- `Register(app)` 只允许读取 capability、组装 router/service/handler/middleware、注册 route、初始化纯内存结构。
- `Register(app)` 不允许启动 goroutine、queue consumer、websocket、socket listener、cron 或连接外部系统。
- `Start(ctx)` 才允许启动 server、consumer、scheduler、websocket、background task；`service` 模式必须阻塞，`job` 模式可以完成后返回。
- `Stop(ctx)` 负责关闭长期运行资源并等待退出。

Provider 需要在 `Register(app)` 中保存 `app *runtime.App` 引用，后续按需通过 `logger.From(app)`、`database.From(app)`、`redis.From(app)`、`cache.From(app)`、`eventbus.From(app)` 获取 runtime capability。不要长期缓存 DB、Redis、Logger、EventBus 等 capability 副本。

Provider 必须显式声明运行依赖：

```go
func (p *WorkerProvider) Dependencies() []runtime.Dependency {
	return []runtime.Dependency{
		runtime.Require("queue"),
	}
}
```

`Run` 会先校验 dependency graph，再按 dependency-aware boot order 完成全部 Provider 的 `Register` 和 `Start`。`service` 模式 Provider 由 runtime 并发托管，多个 HTTP/worker/scheduler 可以共享同一个 runtime；如果 `service` Provider 在 runtime 停止前返回 nil，会被视为 contract 失败并标记为 `failed`。如果某个 Provider `Start` 失败，runtime 会将失败 Provider 标记为 `failed`，并对当前失败 Provider 和已启动 Provider 按倒序调用 `Stop(ctx)` rollback。正常关闭、signal 关闭和 rollback 都统一走 Provider `Stop(ctx)`。

### Service Runtime Integration

Phase 12 起，项目服务通过 runtime provider 接入：

- `app/api`：provider group 为 `api`，依赖 `bootstrap`、`logger`、`database`；`Register` 只构建 router/middleware/handler，`Start` 才启动 HTTP server，`Stop` 调用 HTTP graceful shutdown。
- `app/admin`：provider group 为 `api`，依赖 `bootstrap`、`logger`、`database`、`eventbus`；与 `app/api` 一样作为长期 HTTP service 由 runtime 托管。
- `worker`：provider group 为 `worker`，依赖 `bootstrap`、`logger`、`redis`、`eventbus`，`database` 为可选依赖。`Register` 只加载配置和准备 Queue Runtime Host，`Start` 通过 `core/queue.RuntimeKernel` 初始化 queue runtime、注册 event registry 并启动 queue consumer，`Stop` 负责 consumer shutdown、kernel close。
- `crontab`：provider group 为 `system`，依赖 `bootstrap`、`logger`、`eventbus`，`database`、`redis` 为可选依赖。`Register` 只装配任务模板和 scheduler，`Start` 才启动 scheduler/reload loop，`Stop` 负责 scheduler graceful shutdown。
- `realtime`：provider group 为 `realtime`，依赖 `bootstrap`、`logger`、`eventbus`，`redis`、`auth` 为可选依赖；`Register` 只装配 router/middleware、WebSocket/SSE route、eventbus binding 和 connection manager，`Start` 才启动 gateway server、连接生命周期和 eventbus consumer，`Stop` 负责连接、subscriber、consumer、HTTP server 和 gateway runtime graceful shutdown。

`sdkitgo serve` 当前接入 `api`、`admin`、`worker`、`crontab`、`realtime` provider。`sdkitgo run realtime` 只启动 realtime provider 及其 capability 依赖，不启动其它无关 provider。

`eventbus` 由 runtime capability 初始化并写入 container，只有声明依赖的 service provider 才会使用 shared eventbus。API Service Model 不依赖 eventbus；Provider 不再为 realtime 自行初始化 eventbus，注入的 shared eventbus 由 capability shutdown 统一关闭。

服务 provider 不自行维护 running/stopped/failed 状态，不直接处理 signal，也不直接关闭 DB/Redis/Logger。基础能力和 provider-local capability 由 runtime capability 关闭，服务只关闭自己启动的 listener、consumer、scheduler 和后台循环。

### API Service Model

`app/api` 是业务 HTTP 服务模板，目录边界固定为：

```text
app/api/
  provider.go
  router.go
  server.go
  handler/
  middleware/
```

职责边界：

- `provider.go`：声明服务类型、读取服务配置、把服务接入 runtime。
- `server.go`：持有 `http.Server`、Gin engine 和 graceful shutdown。
- `router.go`：集中注册全局 middleware、route group 和 handler 函数，不引入 RouterDependencies。
- `handler/`：直接处理 request/response、参数解析、业务逻辑和 helper 调用。
- `middleware/`：放 API 私有 Gin middleware，例如 access log、rate limit、Casbin adapter。

handler 允许直接使用：

```go
validator.BindJSON(c, &req)
database.Gorm(c)
database.PGX(c)
redis.Client(c)
queue.Enqueue(ctx, task)
eventbus.Publish(ctx, event)
response.Fail(c, err)
```

当前最小业务链路：

```text
GET /api/ping
GET /api/user/profile
GET /api/dashboard/stats
GET /api/redis/cache
POST /api/queue/push
POST /api/eventbus/publish
POST /api/transaction/demo
```

新增业务路由优先在 `router.go` 直接注册 handler 函数。只有业务复杂度真的需要时，才在 app 层引入局部 helper；不要恢复默认 service/repository 分层。

### Dependency / Boot Order

Runtime 支持 capability dependency 和 provider dependency：

```go
type Dependency struct {
	Name     string
	Required bool
}
```

辅助函数：

- `runtime.Require(name)`：声明必需依赖。
- `runtime.Optional(name)`：声明可选依赖。

校验规则：

- 必需依赖缺失会返回 `ErrDependencyMissing`。
- 同一个对象重复声明同名依赖会返回 `ErrDependencyDuplicate`。
- capability 或 provider 依赖形成环会返回 `ErrDependencyCycle`。
- capability 只能依赖其它 capability；provider 可以依赖 capability 或 provider。
- 可选依赖缺失时不报错，目标存在时参与排序。

启动顺序由 dependency graph 决定，并在无依赖关系时保持注册顺序稳定：

```text
redis
queue
worker
```

关闭顺序始终是 boot order 的倒序：

```text
worker.stop
queue.stop / queue.shutdown
redis.shutdown
```

Runtime 提供：

- `app.ValidateDependencies()`
- `app.Dependencies()`
- `runtime.ValidateDependencies(capabilities, providers)`
- `runtime.SortCapabilities(capabilities)`
- `runtime.SortProviders(providers, capabilities)`

### Runtime Status / Health

Runtime 统一维护 capability 和 provider 的运行状态，业务对象不自行保存运行状态。`Registry` 注册时会把对象包装成 runtime-managed 对象，因此 `app.Provider(...)` / `app.Capability(...)` 返回的对象支持 `Status()`。

状态常量：

```go
const (
	StatusBooting
	StatusRunning
	StatusStopping
	StatusStopped
	StatusFailed
)
```

状态转换：

```text
booting -> running
running -> stopping -> stopped
failed
```

查询接口：

```go
api := app.ProviderStatus("api")
database := app.CapabilityStatus("database")

providers := app.ProviderStatuses()
capabilities := app.CapabilityStatuses()
```

`Health.Error` 只记录 runtime 持有的失败原因，例如 dependency failure、register/start failure、shutdown timeout 或 shutdown error。当前阶段不实现 metrics、watch stream、dashboard 或复杂 healthcheck。

### Command

```go
type Command interface {
	Name() string
	Metadata() CommandMetadata
	Run(ctx context.Context, app *App, args []string) error
}
```

Command 是 Runtime Extension，只负责调用、控制或查询 runtime，不持有 DB、Redis、Logger 等全局资源，不直接初始化 capability，也不直接操作 provider 内部实现。

命名约束：

- `Name()` 必须稳定、唯一、不可为空。
- `Metadata().Name` 必须稳定唯一，未设置时 runtime 会回退使用 `Name()`。
- 禁止使用 `command`、`default`、`main` 作为 Command 名称。
- `RegisterCommand` 会拒绝重复 Command 名称。

执行入口：

```go
err := runtime.Execute(app, os.Args)
```

`Execute` 流程：

```text
解析 args
查找 command
调用 Command.Run(ctx, app, args)
```

当前只提供最小分发能力，不实现复杂 flags、help、子命令树或 cobra 适配。

### Bootstrap Facade

`bootstrap.New()` 只负责创建 `*runtime.App`，并可接收默认 capability：

```go
app := bootstrap.New(
	loggercap.Use(),
	databasecap.Use(),
)
```

bootstrap 不拥有 provider 生命周期、signal、shutdown、rollback 或 execute。项目入口层可以通过 bootstrap 装配默认 capability 和 provider，但实际运行必须交给 runtime。

### Run / Serve

`run` 模式只启动指定 Provider：

```go
err := runtime.RunProvider(ctx, app, "api")
```

生命周期：

```text
Capability.Register(app)
TargetProvider.Register(app)
TargetProvider.Start(ctx)
```

`serve` 模式启动全部 Provider：

```go
err := runtime.RunAllProviders(ctx, app)
```

生命周期：

```text
Capability.Register(app)
Provider.Register(app) // 全部完成
Provider.Start(ctx)    // 按 dependency-aware boot order 启动
```

`app.RunProvider` / `app.RunAllProviders` 是低层启动方法。`runtime.RunProvider` / `runtime.RunAllProviders` 是项目入口推荐方法，会在启动返回后统一调用 `app.Stop(context.Background())`，确保 provider 和 capability 的关闭入口仍由 runtime 持有。

`Run` 仍保留为 `RunAllProviders` 的兼容入口。聚合 serve 模式下同一个 runtime app 只执行一次 capability 初始化，多个 Provider 共享同一组 capability。

### Lifecycle / Graceful Shutdown

Runtime 持有可取消 context。`New()` 默认创建 `context.WithCancel(context.Background())`，调用 `Run(ctx)` / `RunProvider(ctx)` 时会派生当前 runtime context 并透传给 Provider。

`Run` 会在 capability `Register` 前标记 `booting`，成功后标记 `running`。Provider `Register` / `Start` 阶段统一标记 `booting`，`Start` 成功后标记 `running`。dependency validation、capability register、provider register 和 provider start 失败都会进入 `failed`。

统一关闭入口：

```go
err := app.Stop(ctx)
```

关闭顺序固定为：

```text
Provider.Stop(ctx)          // 已运行 provider 倒序
Capability.Shutdown(ctx)    // 已注册 capability 倒序
runtime context cancel
```

`Stop(ctx)` 会串行执行，重复调用是幂等的。传入的 ctx 没有 deadline 时，runtime 默认加 10 秒超时，避免 Provider shutdown 长时间卡住。

关闭时 runtime 会先把已运行 Provider 标记为 `stopping`，`Stop(ctx)` 成功后标记 `stopped`，失败或超时后标记 `failed`。Capability `Shutdown(ctx)` 使用同一规则。

Runtime 监听 `SIGINT` 和 `SIGTERM`。收到 signal 后会调用 `app.Stop(context.Background())`，因此 `ctrl + c` 与正常 Stop 使用同一套关闭顺序。

Command 不直接关闭 server、queue、cron 或底层资源。`run` / `serve` command 只调用 `runtime.RunProvider` / `runtime.RunAllProviders`，不直接调用 `app.Stop()`。

### Ownership

Runtime 拥有：

- context
- signal
- lifecycle
- registry
- provider
- capability
- shutdown
- status
- health
- execute
- run / serve

bootstrap 只允许作为项目入口 facade，负责默认装配和公共配置加载。Provider 只负责自身运行资源的 `Start` / `Stop` 实现。Command 只负责把用户意图转交给 runtime，不持有生命周期状态。

### External Integration Contract

外围系统接入规则：

```text
cobra -> command adapter -> runtime command -> runtime
bootstrap -> capability adapter / facade -> runtime
pkg/driver -> capability adapter / provider adapter -> runtime
```

禁止外围系统直接操作 runtime internal state、provider/capability status、signal owner 或 shutdown 编排。Runtime registry 返回的是 runtime-managed object，外部 contract 即使提供自己的 `Status()`，也不能覆盖 runtime status ownership。

### Orchestration

统一流程：

```text
runtime.Execute
runtime.RunProvider / runtime.RunAllProviders
Capability.Register
Provider.Register
Provider.Start
Provider.Stop
Capability.Shutdown
runtime context cancel
```

signal、正常退出、启动失败 rollback 和命令返回后的资源释放都必须回到 runtime lifecycle。bootstrap、command 和 provider 不能各自实现独立的 signal 或全局 shutdown 编排。

Runtime 同时拥有 failure ownership：dependency failure、boot failure、start failure、shutdown timeout 和 shutdown error 都由 runtime 统一写入 status/health。

## Queue Runtime Instance

Phase Worker-Refactor-3 起，Queue Runtime Platform 增加显式实例入口：

```go
queue.Runtime(ctx)
queueproducer.RuntimeFromServiceContext(serviceCtx)
queueops.RuntimeFromServiceContext(serviceCtx)
```

服务本地队列能力由 facade 从 `ServiceContext.Capabilities` 读取：API 使用 `queueproducer.RuntimeFromServiceContext(serviceCtx)`，Admin 使用 `queueops.RuntimeFromServiceContext(serviceCtx)`。`ServiceContext` 不提供 `QueueRuntime()` 这类队列专用方法，避免 bootstrap 绑定具体 core 模块。`queue.From(app)` 只保留给 runtime wiring、provider startup 和 bootstrap lifecycle，从 runtime-managed app 读取队列实例。

`queue.Enqueue(ctx, task, opts...)` 从执行上下文读取当前队列 runtime 并投递任务。Worker 启动 consumer 时会把 runtime 写入任务执行 context，handler 内继续透传该 context 即可；`core/queue` 不提供包级默认实例。

Queue Runtime Instance 统一持有：

- `Client` / `Worker` / `Manager` 或完整 `QueueRunner`
- `RuntimeKernel`
- `RegistryRuntime`
- `OperationsRuntime`
- runtime metadata，包括 driver、service、worker、queue weight、retry、timeout、delay、priority、trace、middleware、concurrency、rate limit

这是 Runtime API Boundary。Runtime app 只通过 capability container 暴露 queue runtime object、operations object、metadata 和 status API；runtime core 不识别 Gin、Cobra、HTTP、CLI、WebSocket 等 transport。

Runtime Host Boundary：

- Worker provider 是 Queue Runtime Host，负责 queue runtime startup、handler 注册、graceful drain 和 shutdown；业务 handler 不持有 runtime state。
- Admin provider 是 HTTP Runtime Host，负责 route、auth、response、validator 和 middleware；Admin queue route 只使用 `queueops.RuntimeFromServiceContext(ctx).Operations()` 或注入的 operations object。
- `worker/command/queue` 是 Queue CLI Host，归 worker 服务持有，负责 Cobra command 和输出格式；command 只调用 operations API，不操作 queue runtime internals。
- `crontab/command/cron` 是 Crontab CLI Host，归 crontab 服务持有；根 `command` 包只聚合注册服务命令。
- `core/queue` 不提供 route register、command register 或 transport package。

## 内部约束

- `Use` 只保存 capability，不执行初始化。
- `Run` 先校验 dependency graph，再按 boot order 执行 capability `Register`、provider `Register` 和 provider `Start`。
- Runtime registry 统一持有 capability、provider、command 的索引和 metadata。
- Adapter registry 只保存 adapter metadata 和 adapter object，用于 integration introspection，不参与 lifecycle。
- Plugin registry 只保存 plugin metadata 和 plugin object，用于 plugin introspection，不参与 lifecycle。
- Runtime 统一维护 capability/provider status 和 failure health。
- Registry 不负责启动、关闭或 lifecycle 编排。
- Capability 负责初始化资源并写入 container。
- Capability / Provider 必须通过 `Dependencies()` 显式声明依赖，没有依赖时返回 nil 或空 slice。
- Required dependency 缺失、重复 dependency 和循环 dependency 都会阻止启动。
- Shutdown 顺序必须始终是 boot order 的倒序。
- Provider 不初始化外部资源，只从 app/container 读取资源。
- Provider startup 或 runtime wiring 读取平台资源时可以使用 `logger.From(app)`、`database.From(app)`、`redis.From(app)`、`cache.From(app)`、`eventbus.From(app)`。
- 业务代码直接调用 package API，例如 `database.Gorm(ctx)`、`redis.Client(ctx)`、`logger.Info(...)`；不把 `From(app)` 扩散到 handler/service。
- Queue handler 执行链内投递任务优先使用 `queue.Enqueue(ctx, task, opts...)`；服务启动接线层读取 service local queue 时使用对应 facade，例如 `queueproducer.RuntimeFromServiceContext(ctx)` 或 `queueops.RuntimeFromServiceContext(ctx)`。
- Capability `Name()` 必须稳定唯一，不允许为空或使用保留名。
- Provider `Name()` 必须稳定唯一，不允许为空或使用保留名。
- Command `Name()` 必须稳定唯一，不允许为空或使用保留名。
- Capability / Provider / Command 必须返回 metadata，最终 `Name` 为空会被拒绝。
- Group 只用于 runtime introspection，不参与启动顺序。
- Internal object 只做元数据标记，不参与权限或可见性判断。
- Provider `Register` 必须无运行副作用，所有长期运行行为必须放入 `Start`。
- Provider `Start` 失败时，当前失败 provider 和已启动 provider 会按倒序 `Stop`。
- Provider service local capability 必须通过 `RuntimeCapabilities` 声明，由 Runtime 在 provider 启动前统一注册和初始化。
- `Stop` 先倒序停止 provider，再倒序 shutdown capability，最后 cancel runtime context。
- Provider / Capability 不允许自行维护 runtime 运行状态。
- Adapter / Plugin 不允许维护 runtime lifecycle、status、signal 或 shutdown。
- dependency failure、boot failure、start failure、shutdown timeout 和 shutdown error 必须进入 `failed`。
- Provider `Stop` 必须释放自己启动的长期运行资源，包括 HTTP server、queue consumer、cron、websocket、goroutine。
- Capability `Shutdown` 必须释放 capability 持有的资源，不在 Provider 中重复关闭基础资源。
- `RunProvider` 会注册并启动目标 Provider 及其 provider dependency，不会启动无关 Provider。
- `Execute` 只负责参数分发，不负责初始化业务资源。
- 当前不做 plugin loader 或自动发现。
- 当前不做 dynamic plugin、hot reload、adapter graph 或 plugin lifecycle manager。
- 当前不做复杂 hook、metrics、watch stream 或 dashboard。

## 注意事项

- 不要在 Phase 12 扩展 cluster runtime、distributed realtime、gateway mesh、dynamic plugin 或 hot reload。
- 不要把 command contract 侵入服务层。
- 不要让 provider 持有初始化权。
- 不要使用裸 string 作为 container key，模块内声明自己的 `runtime.Key` 常量。
- `MustGet` 为兼容便捷读取而保留，但未命中时返回 nil，不 panic。

## 已知限制

- Dependency 不包含版本约束。
- 不生成 graph visualization。
- 不做复杂 healthcheck、metrics、watch stream 或 dashboard。
- 无 reload / hot restart。
- Command dispatcher 只支持最小 args 分发，不支持复杂 CLI。
- 不做 Named Capability；当前只支持单 logger、单 DB、单 Redis、单 cache、单 eventbus。
- Runtime registry 不实现 plugin visibility 或自动发现。

## 更新记录

- Phase 1：新增 runtime App、Container、Capability、Provider、Command 基础 contract。
- Phase 1 Fix：修复 capability 初始化时机，新增 provider rollback、强类型 container key、App context 和具名 capability。
- Phase 2：新增 logger/database/redis/cache 平台 capability，bootstrap 平台资源初始化改为通过 runtime capability 执行。
- Phase 3：补充 Provider 命名校验、Register/Start/Stop 生命周期边界、Start rollback 顺序测试和聚合运行 capability 共享规范。
- Phase 4：新增 Command 命名校验、Provider/Command 查找、`Execute`、`RunProvider`、`RunAllProviders`，并将 run/serve 命令入口收敛到 runtime。
- Phase 5：新增 runtime Stop、Capability Shutdown、SIGINT/SIGTERM 统一关闭、Provider Stop 10 秒默认超时，并将 graceful shutdown 收敛到 Runtime lifecycle。
- Phase 6：新增 bootstrap facade 规范，`runtime.RunProvider` / `runtime.RunAllProviders` 统一负责入口级 Stop，command 不再直接持有 runtime shutdown。
- Phase 7：新增 Runtime Registry 和 Metadata，统一 capability/provider/command 的查找、列表、分组、internal 标记和重复名称校验。
- Phase 8：新增 Runtime Dependency、dependency validation、dependency-aware boot order、boot order 倒序 shutdown 和 dependency introspection。
- Phase 9：新增 Runtime Status / Health、ProviderStatus、CapabilityStatus、状态列表查询和 runtime failure ownership。
- Phase 10：新增 Runtime Adapter Contract、Plugin Boundary、adapter/plugin introspection、cobra command adapter 边界和 External Integration Contract。
- Phase 12：新增 eventbus runtime capability 接入、realtime provider 纳入 `sdkitgo run realtime` / `sdkitgo serve`、共享 EventBus 注入、Realtime Gateway lifecycle 与 shutdown 约束。
- Phase Runtime-Capability-1：新增 `RuntimeCapabilityProvider`，Provider 可声明 service local capability；Runtime 在依赖解析前收集并注册目标 Provider 能力，统一初始化和 shutdown。
- Phase Runtime-Capability-2：bootstrap `ServiceBuilder.RuntimeCapabilities` 接入 provider-local capability，`command/serve` 自动合并服务本地 capability dependency 并注入 `ServiceContext.Capabilities`。
- Phase API-1：收敛 `app/api` 为业务 HTTP 服务模板，建立 router/handler/service/repository 边界，API provider 仅依赖 logger/database，不再挂载旧 demo realtime/eventbus 能力。
- Phase API-2：API DX 改为 Go native 直接 handler 模型，移除 RouterDependencies 和 handler struct 注入，新增 validator/database/response/redis/auth/queue helper 示例。
- Phase Worker-Refactor-2：Queue Runtime Kernel ownership 继续收敛，`core/queue` 新增 `RuntimeKernel` 和 `Dispatcher`，worker bootstrap 只负责注入 Redis/DB/outbox/failure writer 等宿主依赖。
- Phase Worker-Refactor-3：新增 Queue Runtime Instance、Registry Runtime 和 Operations Runtime；worker host 启动、注册和关闭路径改为优先使用 runtime instance，后续已删除包级默认实例兼容层。
- Phase Worker-Refactor-4：Queue Runtime Metadata 和 Operations Platform 完整化，queue registry metadata 支持注册参数 introspection，operations 统一 runtime status、worker status、metrics、failed、clean、pause、resume 和 drain。
- Phase Runtime-Boundary-1：建立 Runtime API Boundary，queue runtime 不再暴露 Gin/Cobra transport；Admin/Command host 自主注册 route/command 并统一调用 Queue Operations API。
- Phase Worker-Refactor-1：worker 定位收敛为 Queue Runtime Host，业务注册入口改为 `worker.RegisterEvents`，queue runtime kernel、registry、typed payload handler 和 tracing middleware 由 `core/queue` 承担。
