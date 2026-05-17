# Bootstrap 使用说明

`bootstrap` 是骨架层入口，负责公共启动能力和服务接入标准。服务入口统一导入 `sdkitgo/bootstrap`。

Phase 6 起，bootstrap 是 runtime facade。推荐项目入口：

```go
app := bootstrap.New()
app.RegisterCapabilities(...)
app.Register(...)
return runtime.RunAllProviders(ctx, app)
```

bootstrap 不直接持有 provider 生命周期、signal 或 shutdown。

## 公共初始化

`appbootstrap.Init` 只初始化公共能力。内部会创建 runtime app，注册 `bootstrap.RuntimeCapabilities(...)`，再由 runtime 按依赖顺序初始化：

- 配置加载
- 日志
- Database
- Redis
- Cache
- RateLimit
- Session
- BBR 公共配置
- Casbin

`sdkitgo serve` 聚合启动不再让 `RuntimeCapability` 直接初始化全部公共基础设施。`RuntimeCapability` 只加载公共配置；logger、tracing、database、redis、cache、ratelimit、session 等由 `bootstrap.RuntimeCapabilities(...)` 返回为独立 runtime capability，交给主 runtime 统一初始化和关闭。Gin mode、auth guard 和业务迁移/种子数据由具体 HTTP 服务或命令自己处理；validator 由 HTTP 类型服务声明全局 runtime capability。

`BootConfig` 用来声明当前命令/服务的必需基础设施：

- `DB: true` 表示数据库是必需依赖，数据库连接或 ping 失败时 `Init` 返回 error，服务不得继续启动。
- `Redis: true` 表示 Redis 是必需依赖，Redis ping 失败时 `Init` 返回 error，服务不得继续启动。
- `DB: false` / `Redis: false` 表示当前命令不声明该依赖，bootstrap 不会把对应基础设施失败当作启动失败。

Cache、Session、RateLimit 这类明确支持本地/内存后端的能力，只在未声明必需 Redis 或 Redis 未绑定时使用自己的降级后端。降级只发生在这些能力内部，不代表 Redis 初始化成功。

服务自己的配置不放进 `bootstrap.Config`。服务通过 `provider.go` 声明需要哪些能力，具体能力适配放在服务自己的 `infra/`。

## 服务标准

服务接入骨架层需要实现：

```go
type ServiceInfo struct {
    Name         string
    Type         string
    Kind         ServiceKind
    Addr         string
    Enabled      bool
    Capabilities []string
}

type Service interface {
    ServiceInfo() ServiceInfo
    Start(context.Context) error
    Shutdown(context.Context) error
}
```

服务类型通过 `Provider` 声明：

```go
func Provider() bootstrap.Provider {
    return bootstrap.ProviderFunc(func(app *bootstrap.App) error {
        app.Service("api").
            Kind(bootstrap.ServiceKindHTTP).
            FactoryContext(func(ctx bootstrap.ServiceContext) (bootstrap.Service, error) {
                cfg, err := apiconfig.Load(ctx.ConfigFile, ctx.Name, ctx.Base)
                if err != nil {
                    return nil, err
                }
                srv := api.NewServer(cfg)
                return bootstrap.HTTPService{
                    InfoValue: cfg.ServiceInfo(),
                    StartFunc: srv.Start,
                    StopFunc:  srv.Shutdown,
                }, nil
            })
        return nil
    })
}
```

`register.go` 只保留 Provider 注册包装：

```go
func init() {
    _ = bootstrap.RegisterProvider(Provider())
}
```

服务注册统一使用 `Provider` / `ServiceBuilder`，新增服务生成 `provider.go`。

`FactoryContext` 会收到当前服务的构建上下文。服务本地能力不在这里手工注册，而是通过 `RuntimeCapabilities` 声明；runtime 初始化完成后会把能力名和实例注入 `ServiceContext.Capabilities`：

```go
type Capability interface {
    Name() string
}
```

```go
func Provider() bootstrap.Provider {
    return bootstrap.ProviderFunc(func(app *bootstrap.App) error {
        app.Service("admin").
            Kind(bootstrap.ServiceKindHTTP).
            FactoryContext(func(ctx bootstrap.ServiceContext) (bootstrap.Service, error) {
                cfg, err := adminconfig.Load(ctx.ConfigFile, ctx.Name, ctx.Base)
                if err != nil {
                    return nil, err
                }
                srv := admin.NewServerWithContext(cfg, &ctx)
                return bootstrap.HTTPService{
                    InfoValue: cfg.ServiceInfo(),
                    StartFunc: srv.Start,
                    StopFunc:  srv.Shutdown,
                }, nil
            })
        return nil
    })
}
```

服务私有 runtime capability 通过 `RuntimeCapabilities` 声明，由主 runtime 在服务 `Register/Start` 前完成初始化和关闭：

```go
func Provider() bootstrap.Provider {
    return bootstrap.ProviderFunc(func(app *bootstrap.App) error {
        app.Service("api").
            Kind(bootstrap.ServiceKindHTTP).
            RuntimeCapabilities(func(ctx bootstrap.RuntimeCapabilityContext) []runtime.CapabilityContract {
                return []runtime.CapabilityContract{
                    openai.Use(
                        openai.WithName(ctx.LocalName(openai.Name)), // api.openai
                        openai.WithConfigLoader(func(*runtime.App) (openai.Config, error) {
                            cfg, err := apiconfig.Load(ctx.ConfigFile, ctx.Name, ctx.BaseConfig(), ctx.ConfigKey)
                            if err != nil {
                                return openai.Config{}, err
                            }
                            return cfg.OpenAI, nil
                        }),
                    ),
                }
            }).
            FactoryContext(factory)
        return nil
    })
}
```

`RuntimeCapabilities` 返回的能力会自动标记为 `runtime.ScopeServiceLocal`。能力名称必须稳定，推荐使用 `ctx.LocalName("openai")` 形成 `api.openai`、`admin.payment` 这类服务命名空间。能力注册成功后，实例也会写入当前服务的 `ServiceContext.Capabilities`。

服务启动层统一接入 framework runtime。服务私有能力通过 `RuntimeCapabilities` 声明；Filesystem 这类全服务共享能力由 bootstrap 注册，并在 capability 的 `Register` 中把实例绑定到 runtime container；服务目录只保留业务 adapter：

```go
filesystemcap.Use(
    filesystemcap.WithConfigLoader(func(*runtime.App) (filesystemcap.Config, error) {
        cfg, err := boot.requireConfig()
        if err != nil {
            return filesystemcap.Config{}, err
        }
        return cfg.FileSystem, nil
    }),
)
```

DB、Redis、Cache、Logger、Tracing、RateLimit、Session、Filesystem 仍然是 bootstrap 初始化的基础公共能力，业务代码继续使用对应 core 根包 API 或服务 adapter。

Filesystem 这类多服务通用 framework runtime，由 `infra/capabilities/*` 提供默认实现；EventBus、Realtime 和 Queue producer 使用对应 `core/*/facade`。服务目录只保留 `infra/notify`、`infra/storage`、`infra/realtime` 这类业务 adapter。

## services 配置

`sdkitgo serve` 从 `services` 自动构造服务实例。`services` 必须显式配置；缺失或为空时 `bootstrap.BuildServices` 会返回错误，不再默认启动 `admin/api`。

```yaml
services:
  admin:
    type: admin
    enabled: true

  api:
    type: api
    enabled: true

  api2:
    type: api
    enabled: true
    # 可选：不配置时默认读取顶层 api2。
    # config_key: public_api

  worker:
    type: worker
    enabled: false

  realtime:
    type: realtime
    enabled: true

  crontab:
    type: crontab
    enabled: false
```

`api2` 只复用 `api` 服务类型的工厂，不复用 `api` 的运行配置。运行配置默认读取与实例名相同的顶层 key，因此 `api2` 必须存在同名配置：

```yaml
api2:
  addr: :8082
```

如果确实要让实例读取其它配置块，必须显式声明 `config_key`。服务配置缺失或关键字段为空时会直接返回错误，错误信息包含 service name、type 和 config key。`realtime`、`worker` 也走同一套注册机制。Crontab 是定时任务管理器，由 `command/serve` 在 `sdkitgo serve` 场景适配为可运行单元。新增服务实例只改 YAML；新增服务类型需要把服务包或装配适配器编译进命令。

## 启动输出

运行时会在配置加载成功后先打印 banner；服务实例注册完成后，再统一打印一次启动摘要和服务表。

```go
bootstrap.PrintBanner(cfg)
bootstrap.PrintServiceTableWithInfo(cfg, serviceInfos...)
```

表格会展示：

- `Mode`
- `Gin Mode`
- 服务提供的 `ServiceInfo`
- `Database`
- `Redis`
- `PID`

组合启动时，`Runtime Capabilities` 会按 runtime metadata 展示 `ScopeGlobal` 公共能力，`Enabled Services` 会聚合展示本次启用的所有服务，避免每个服务重复打印公共摘要。单服务启动仍只展示当前服务。
服务表的 `Capabilities` 只展示 `ScopeServiceLocal` 服务能力，并去掉服务名前缀，例如 `api.queue.producer` 显示为 `queue.producer`。
`Capabilities` 列较长时会按逗号优先自动换行，避免服务表被能力列表撑得过宽。
服务表会在服务块之间打印分隔线，能力列表换行时续行仍归属于同一个服务。

`:8080` 或 `0.0.0.0:8080` 形式会格式化为本机局域网 IP 的 HTTP 地址，方便本地开发直接访问。

## 单进程组合模式

`sdkitgo serve` 是单进程组合启动：

- `bootstrap.New` 创建 `*runtime.App`。
- `bootstrap.RuntimeCapability` 加载公共配置。
- `bootstrap.RuntimeCapabilities` 注册共享基础设施 capability。
- 单服务命令通过 `bootstrap.BuildService` 构建。
- `sdkitgo serve` 通过 `bootstrap.BuildServices` 根据 `services` 构建。
- 各服务通过 `Provider.RuntimeCapabilities` 声明自己的本地能力，由 runtime 在 provider register 前统一初始化。
- 任一服务启动返回 error 时，runtime 会取消启动上下文并执行 provider rollback。
- 收到 SIGINT/SIGTERM 时，runtime 会进入 `app.Stop(ctx)`，按 provider 反向顺序调用每个服务的 `Shutdown(ctx)`，并记录所有关闭错误。
- 服务 `Shutdown(ctx)` 只负责本服务持有的 HTTP server、订阅循环、访问日志 writer、worker consumer 等本地运行资源；DB、Redis、Cache、Queue producer、Tracing、Logger Sync 以及 provider-local capability 由 runtime capability 统一释放。
- 全局资源关闭由 runtime capability 负责，顺序为 Queue producer、Cache、Redis、Database、Tracing、Logger Sync。关闭函数可重复调用，重复调用只会变成 no-op。
