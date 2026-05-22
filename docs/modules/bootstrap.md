# bootstrap 模块说明

## 职责

`bootstrap` 是骨架层，不属于 `core`。它负责制定服务接入标准，并组合公共启动能力。

服务入口统一使用 `sdkitgo/bootstrap`。

Phase 6 起，bootstrap 是 runtime facade。它可以创建 `*runtime.App`、生成默认 capability、加载公共配置和注册项目入口，但不持有 provider 生命周期、signal、shutdown、rollback 或 execute。

## 公共配置边界

`bootstrap.Config` 只保存公共配置：

- `app`
- `log`
- `database`
- `redis`
- `cache`
- `session`
- `jwt`
- `bbr`

服务配置不进入 `bootstrap.Config`。`services.<name>` 只作为 `sdkitgo serve` 的启动清单，保存 `type/enabled`。例如 admin/api 的地址、服务级 JWT、服务级 Session、服务级 token bucket 限流分别放在 `admin.yaml`、`api.yaml`，由 `app/admin/config`、`app/api/config` 读取。BBR 是进程级 HTTP 过载保护，放在公共 `bbr`。

## 服务注册标准

骨架层定义标准接口：

```go
type Service interface {
    ServiceInfo() ServiceInfo
    Start(context.Context) error
    Shutdown(context.Context) error
}
```

服务包通过 `Provider` 接入骨架。`Provider` 只声明服务身份、类型和构建工厂，不写具体 DB/Redis/Queue 业务实现：

```go
func Provider() bootstrap.Provider {
    return bootstrap.ProviderFunc(func(app *bootstrap.App) error {
        app.Service("api").
            Kind(bootstrap.ServiceKindHTTP).
            FactoryContext(factory)
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

`FactoryContext` 会收到当前服务的构建上下文：

```go
type ServiceContext struct {
    ConfigFile   string
    Name         string
    Type         string
    Base         *Config
    Capabilities *CapabilityRegistry
}
```

服务私有 runtime capability 使用 `ServiceBuilder.RuntimeCapabilities(...)` 声明。骨架层会把它们交给 `core/runtime` 的 `RuntimeCapabilityProvider` 机制：

```go
app.Service("api").
    RuntimeCapabilities(func(ctx bootstrap.RuntimeCapabilityContext) []runtime.CapabilityContract {
        return []runtime.CapabilityContract{
            openai.Use(openai.WithName(ctx.LocalName(openai.Name))),
        }
    }).
    FactoryContext(factory)
```

规则：

- `RuntimeCapabilities` 返回的能力默认归属 `runtime.ScopeServiceLocal`。
- 能力名必须带服务命名空间，例如 `api.openai`、`admin.payment`、`worker.oss`。
- `ctx.LocalName(name)` 用当前服务实例名生成本地能力名。
- `ctx.BaseConfig()` 用于 capability config loader 延迟读取公共配置，避免 provider capability 收集阶段配置尚未加载。
- `command/serve` 会把 provider-local capability 自动追加到 provider dependency，并在服务构建前把已注册实例写入 `ServiceContext.Capabilities`。

`ServiceContext.Capabilities` 保存当前服务可见的能力名和已初始化实例，供服务构建期复用。启动表格不从这个 registry 猜测能力类型，而是读取 runtime capability metadata。底层能力接口仍只表示名称：

```go
type Capability interface {
    Name() string
}
```

framework capability 不再在服务构造阶段通过 `ctx/cfg` 手工注册。服务能力统一声明为 runtime capability，由主 runtime 完成注册、依赖解析、初始化顺序和 shutdown 顺序；注册后的 metadata 会用于启动表格、健康检查和后续 CLI 生成。

服务自己的 `ServiceInfo.Capabilities` 保持为空，不手写静态能力。启动表格按 `runtime.CapabilityMetadata.Scope` 分两类展示：`ScopeGlobal` 进入公共能力表；`ScopeServiceLocal` 进入对应服务行，并去掉服务名前缀，例如 `api.queue.producer` 展示为 `queue.producer`、`admin.queue.operations` 展示为 `queue.operations`。

DB、Redis、Cache、Logger、Tracing 由 bootstrap 生成 runtime capability 后交给主 runtime 初始化，业务代码继续使用 `core/database`、`core/redis`、`core/cache`、`core/logger`、`core/tracing` 的公共入口。

`BootConfig.DB` 和 `BootConfig.Redis` 是基础设施依赖声明，不是“尝试初始化”的开关。声明为 `true` 时，数据库或 Redis 初始化失败必须直接向调用方返回 error，避免服务以 `database.DB == nil` 或 `redis.RDB == nil` 的半初始化状态继续运行。只有明确支持本地/内存后端的能力可以在自身模块内降级，例如 cache、session、ratelimit；这类降级需要保留清晰日志，不能记录成基础设施初始化成功。

Filesystem 这类多服务通用 framework runtime 由 `infra/capabilities/*` 持有生命周期和默认实现；EventBus、Realtime 和 Queue producer 使用对应 `core/*/facade`。服务侧只保留业务 adapter，例如 `infra/notify`、`core/storage`、`infra/realtime`。

Crontab 是定时任务管理器，不在 `crontab` 包里主动注册骨架服务。`sdkitgo serve` 需要一起运行 Crontab 时，由 `command/serve` 装配层把它适配为 `kind=cli` 的运行单元。

标准服务类型：

| Kind | 适用服务 |
|---|---|
| `http` | app 下普通 HTTP 服务，例如 admin、api、api2 |
| `websocket` | WebSocket gateway |
| `sse` | SSE |
| `queue` | Worker 队列消费服务 |
| `cli` | Crontab 这类命令/调度型服务 |

`bootstrap.BuildServices(configFile, cfg)` 会读取显式配置的 `services` 实例清单。`services` 缺失或为空时返回错误，不再默认构建 `admin/api`。

```yaml
services:
  admin:
    type: admin
    enabled: true
  api:
    type: api
    enabled: true
  worker:
    type: worker
    enabled: false
```

构建流程：

1. 读取 `services` 实例清单。
2. 跳过 `enabled: false` 的实例。
3. 按 `type` 找到已注册工厂。
4. 调用服务自己的配置加载器。
5. 返回实现了 `Service` 的实例。

## 服务信息

服务通过 `ServiceInfo()` 向骨架层暴露运行信息：

```go
type ServiceInfo struct {
    Name         string
    Type         string
    Kind         ServiceKind
    Addr         string
    Enabled      bool
    Capabilities []string
}
```

当前启动表格使用该信息打印服务名和地址。服务实现只负责填写 `Name`、`Type`、`Kind`、`Addr`、`Enabled`，`Capabilities` 不手写静态值，由启动阶段按 runtime capability metadata 合并。`Capabilities` 是展示口径，不等同于 runtime dependency；公共基础设施依赖展示在公共能力表。后续如果需要把服务能力暴露给健康检查、命令列表或监控，也走同一个接口。

服务实际暴露的能力来自 `Provider.RuntimeCapabilities` 声明的 `ScopeServiceLocal` capability；公共能力来自 bootstrap 或其他公共装配处声明的 `ScopeGlobal` capability。`ServiceContext.Capabilities` 仍用于服务构建期取实例，避免把展示信息误认为依赖注入。

## 单进程组合模式

`sdkitgo serve` 组合模式由 runtime 统一编排，bootstrap 只负责项目默认装配：

- `bootstrap.New` 创建 runtime app。
- `bootstrap.RuntimeCapability` 加载公共配置。
- `bootstrap.RuntimeCapabilities` 生成 logger、tracing、database、redis、cache、ratelimit、session、filesystem 等公共 capability，由主 runtime 统一初始化和关闭；HTTP 服务按 `ServiceKindHTTP` 额外注册并依赖 validator capability。
- Provider 声明的 `RuntimeCapabilities` 会在 provider `Register` 前收集、依赖排序、初始化，并以 service-local scope 纳入统一 shutdown。
- `bootstrap.BuildServices` 根据配置构建服务。
- 每个服务自己决定接入哪些 core/pkg 能力。
- 启动阶段任一服务返回 error 时，由 runtime 取消启动上下文并回滚 Provider。
- 收到 SIGINT/SIGTERM 时，由 runtime 触发 `app.Stop(ctx)`，按反向顺序关闭 Provider 和 capability。

这个模式支持只启用 `admin` 和 `api`，不需要 websocket、crontab 或 worker 进入公共配置，但 `services` 清单本身必须显式存在。需要 worker 时只在 `services` 加 `type: worker`，由 worker 包自己的注册工厂接入；需要 crontab 时只在 `services` 加 `type: crontab`，由 crontab 包自己的 Provider 接入。

## 生命周期和资源关闭

生命周期 owner 约定：

- 服务 owner：HTTP server、WebSocket/SSE 订阅循环、Worker 消费循环、Crontab scheduler、访问日志和失败日志 writer。
- Runtime capability owner：DB、Redis、Cache、Filesystem、Queue producer、Tracing、Logger Sync、EventBus、Realtime 等进程级全局资源，以及 provider-local capability。
- WebSocket Shutdown 需要取消广播 bus、清理全局 default bus；组合进程里 runtime capability 的 tracing 关闭是幂等兜底。

Runtime 关闭顺序：

1. 取消服务运行 context。
2. 反向调用服务 `Shutdown(ctx)`，收集所有 error。
3. 关闭 Queue producer。
4. 关闭 Cache。
5. 关闭 Redis。
6. 关闭 Database。
7. 关闭 Tracing。
8. 执行 Logger Sync。

所有关闭入口必须允许重复调用。重复关闭不得 panic；已关闭的全局资源应清空默认指针或直接 no-op。

## 更新记录

- 2026-05-16：ServiceBuilder 新增 `RuntimeCapabilities`，服务级能力通过 provider-local capability 接入 runtime，并自动写入 `ServiceContext.Capabilities`。
- 2026-05-16：移除旧泛型注册器和服务构造期能力注册方式，Admin/Worker/Crontab 的本地能力统一由 Provider 声明并交给 runtime 管理。
- 2026-05-16：Queue 接入 `core/queue/facade/producer` 和 `core/queue/facade/operations` Runtime Capability；bootstrap 不再注册 queue 公共能力或 marker，Worker queue runtime 由 Worker 服务自行初始化。
- 2026-05-17：移除 bootstrap post 初始化，Gin mode、auth guard、migrate/seed 下沉到服务或命令；HTTP 服务按 kind 使用 validator runtime capability。
- 2026-05-16：Session 和 RateLimit 接入 `core/*/facade` Runtime Capability，由 `bootstrap.RuntimeCapabilities` 注册并在服务启动前完成初始化。
- 2026-05-15：公共能力加载收敛到 `bootstrap.RuntimeCapabilities`，`RuntimeCapability` 只负责加载配置，logger/database/redis/cache/tracing 等作为独立 runtime capability 由主 runtime 管生命周期。
- 2026-05-15：Phase 6 将 bootstrap 明确为 runtime facade，run/serve 的 signal、shutdown 和 provider lifecycle 统一归 runtime。
- 2026-05-12：统一 `sdkitgo serve`、单服务、Worker、Crontab、WebSocket/SSE 的关闭约定；启动失败会回滚已构建服务，并统一释放 Queue、Cache、Redis、Database、Tracing 和 Logger。
- 2026-05-10：服务表格补充 WebSocket 地址输出，并集中维护服务地址映射。
- 2026-05-10：明确 `sdkitgo serve` 模式下 Queue、EventBus、WebSocket 发布侧的初始化边界。
- 2026-05-11：应用公共配置与启动实现迁移到 `bootstrap`。
- 2026-05-11：新增服务注册标准，`sdkitgo serve` 基于 `services` 自动构建服务实例，服务配置迁移到服务自己的 config 包。
- 2026-05-13：服务注册入口统一为 `Provider/App/ServiceBuilder`。
- 2026-05-13：`BuildServices` 不再在缺失 `services` 时默认构建 `admin/api`，组合启动必须显式配置服务清单。
- 2026-05-11：新增 `ServiceContext/Capability/FactoryContext`，单服务命令和 `sdkitgo serve` 统一通过 Provider 构建服务，服务内能力由各服务自己适配后注册。
- 2026-05-12：明确 `BootConfig.DB/Redis` 为必需依赖声明，初始化失败直接返回 error；仅 cache、session、ratelimit 等明确可选能力允许内部降级。
