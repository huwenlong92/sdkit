# Framework Capability

## 作用

Capability 是 framework runtime 接入骨架的能力标准。它只描述“当前进程启用了什么运行时能力，以及这个能力如何在启动时初始化”，不做业务逻辑，也不承担 `core` / `pkg` 的职责。

适合做成 capability 的内容：

- Queue producer / consumer 接入
- FileSystem 默认实例
- EventBus 接入
- Realtime Publisher、EventBus 等 framework runtime
- 需要在启动信息、健康检查、CLI 生成中被骨架识别的能力

不适合做成 capability 的内容：

- DB、Redis、Cache、Logger 这类 bootstrap 全局基础能力
- handler 业务逻辑
- model 查询逻辑
- 纯内部组件，例如 accesslog writer、auth hooks
- 服务私有业务 adapter，例如 notify、storage、presence、audit

## 接口

底层 registry 仍保留只包含名称的能力接口，用于启动表格和后续 CLI 识别：

```go
type Capability interface {
    Name() string
}
```

服务能力不再在 `NewServerWithContext` 里手工注册。需要主 runtime 管理的能力必须实现或返回 `runtime.CapabilityContract`，并由 `Provider` 通过 `RuntimeCapabilities` 声明：

```go
app.Service("admin").
    RuntimeCapabilities(func(ctx bootstrap.RuntimeCapabilityContext) []runtime.CapabilityContract {
        return []runtime.CapabilityContract{
            openai.Use(openai.WithName(ctx.LocalName(openai.Name))),
        }
    }).
    FactoryContext(factory)
```

runtime 注册成功后，`command/serve` 会把 container 里的实例写入当前服务的 `ServiceContext.Capabilities`。服务代码只消费 `ServiceContext` 或本服务 `infra/*` adapter，不关心 runtime wiring。

`CapabilityRegistry` 仍保留给服务构建期读取已注册能力实例，并保留 `Names()` 的能力展示语义：

```go
value, ok := ctx.Capabilities.Get("admin.openai")
```

## 目录规则

framework capability 默认实现按归属放置：

```txt
infra/capabilities/<name>/
  <name>.go
  tests/

core/<module>/facade/
  use.go
```

约定：

- `infra/capabilities/*` 放不属于某个 core root 的多服务通用 runtime 能力。
- `core/<module>/facade` 放 core 模块自己的 runtime facade。
- Queue producer 使用 `core/queue/facade/producer`；Queue 管理能力使用 `core/queue/facade/operations`；Worker queue runtime 由 Worker 服务自行初始化，不作为 bootstrap capability。
- capability 包放运行时生命周期、默认实现和最小注册 helper。
- 服务私有业务入口放服务目录的 `infra/<adapter>`，例如 `core/storage`、`app/admin/infra/realtime`。
- handler/task 不直接初始化 framework runtime，只调用业务 adapter 或 `core/*` 公共入口。
- 配置装配函数使用私有函数，例如 `configure`，不暴露给 handler。

## 示例

framework capability 通过 runtime capability 注册。Filesystem 属于 bootstrap 公共能力，服务 provider 不再单独声明：

```go
storagefacade.Use(
    storagefacade.WithConfigLoader(func(*runtime.App) (storagefacade.Config, error) {
        cfg, err := boot.requireConfig()
        if err != nil {
            return storagefacade.Config{}, err
        }
        return cfg.Storage, nil
    }),
)
```

## 当前服务

| 服务 | 业务 adapter |
|---|---|
| Admin | `core/storage`、`app/admin/infra/realtime` |
| API | `core/storage`、`app/api/infra/realtime` |
| Realtime Gateway | `app/realtime/infra/realtime` |
| Worker | `core/storage`、`worker/infra/realtime` |
| Crontab | `core/storage`、`crontab/infra/realtime` |

通用 realtime 推送能力放在：

```txt
core/realtime/facade
```

需要向 realtime gateway / EventFlow 发消息的服务，推荐注册通用 `realtime` capability，并通过 `realtime.PushUser`、`realtime.PushRoom`、`realtime.Broadcast` 或服务私有默认入口调用。

旧 SSE/WebSocket publisher bridge 不再保留。服务需要向 SSE/WebSocket 客户端推送消息时，统一依赖 `realtime` capability；服务目录的 `infra/realtime` 只保留业务适配和默认入口。

Realtime Gateway 本体是消息接收方，不声明额外 publisher capability。

Admin 当前复用的通用能力实现：

| 能力 | 通用实现 |
|---|---|
| `filesystem` | `core/storage/facade` |
| `queue` | `core/queue/facade/producer` 或 `core/queue/facade/operations` |
| `eventbus` | `core/eventbus/facade` |
| `realtime` | `core/realtime/facade` |

服务目录仍保留自己的业务 adapter，例如 `core/storage`。它的职责是保存 Admin 默认入口，并在需要定制时包一层服务私有逻辑。

## 通用 EventBus Capability

通用 EventBus capability 位于：

```txt
core/eventbus/facade
```

core runtime facade 的文件边界固定为：根包 `binding.go` 只放 `Key/From/Bind` 绑定原语；`facade/use.go` 才放 `Use/WithConfig/WithConfigLoader` 和生命周期逻辑。不要在 root 与 facade 两边同时实现 runtime `Use`。

## Core Runtime Facade 默认规则

core runtime facade 的默认行为必须集中在 `defaultUseOptions()`，避免调用方为了补齐内部约定反复传入样板 option。`Use(...)` 只负责应用 option、创建 `runtime.Capability`、执行初始化和关闭流程。

适用范围：

- Redis、Database、Cache、Logger、Tracing、Storage、Email、SMS、Casbin、Ratelimit 等框架底座能力，默认是内部能力，`defaultUseOptions()` 必须设置 `internal: true`。
- EventBus、Realtime、Queue producer、Queue operations 等可能被服务或 CLI 展示的能力，需要按业务展示语义单独判断，不因为有 facade 就自动默认 internal。
- 服务本地 capability 由服务侧 `RuntimeCapabilities` 和 `ScopeServiceLocal` 控制，不套用全局底座能力默认值。

实现规则：

- 每个 core facade 优先提供 `defaultUseOptions()`，在里面声明默认依赖、默认 `internal` 等零值以外的语义。
- `WithInternal()` 只保留兼容或少数显式场景；新代码不应依赖调用方传 `WithInternal()` 才能得到框架默认行为。
- 默认 internal 的 facade 如需外部展示，应提供 `WithExternal()`，由调用方显式把 `internal` 设置为 `false`。
- 配置来源必须显式注入。core facade 不默认读取 `core/config.V`；需要配置时由 bootstrap、服务 provider 或独立入口通过 `WithConfig` / `WithConfigLoader` 传入。
- 如果模块没有安全默认配置，且调用方没有传 `WithConfig` / `WithConfigLoader` / 现成实例，初始化必须返回明确 error，不允许静默使用零值连接外部资源。
- facade 不读取服务私有配置字段。服务侧必须在 config loader 中把服务配置映射成 facade 自己的 `Config`。

推荐骨架：

```go
func defaultUseOptions() useOptions {
    return useOptions{
        dependencies: []runtime.Dependency{
            runtime.OptionalBootstrap(),
        },
        internal: true,
    }
}

func WithExternal() UseOption {
    return func(o *useOptions) {
        o.internal = false
    }
}

func Use(opts ...UseOption) runtime.Capability {
    o := defaultUseOptions()
    for _, opt := range opts {
        if opt != nil {
            opt(&o)
        }
    }

    // register lifecycle...
}
```

职责：

- 按配置选择 `memory`、`redis`、`redis_stream` driver。
- 复用 `pkg/eventbus/memory`、`pkg/eventbus/redis`、`pkg/eventbus/redisstream` 创建 driver，不重新实现 driver 行为。
- 设置或复用 `core/eventbus` default bus。
- 暴露 `Publish`、`Subscribe`、`Bus`、`Driver`、`Close`。
- 在 `Close` 时回收 capability 自己创建或显式托管的 bus。

边界：

- 不 import app、worker、crontab 服务包。
- 不读取服务私有配置字段，服务侧在 `RuntimeCapabilities` 的 config loader 中映射到 `eventbus.Config`。
- 不创建 Redis client；`redis` 和 `redis_stream` 必须通过 `WithRedisClient(*redis.Client)` 显式传入外部 Redis client。
- 不解释业务 topic、业务身份、websocket/SSE/mqtt 连接或 realtime 在线状态。

配置映射示例：

```go
func eventbusCapability(ctx bootstrap.RuntimeCapabilityContext) runtime.CapabilityContract {
    name := ctx.LocalName(eventbuscap.Name)
    return runtime.NewCapability(name, func(app *runtime.App) error {
        cfg, err := loadServiceConfig(ctx)
        if err != nil {
            return err
        }
        capability, err := eventbuscap.New(eventbuscap.Config{
            Driver:       cfg.EventBus.Driver,
            TopicPrefix:  cfg.EventBus.TopicPrefix,
            NodeName:     cfg.EventBus.NodeName,
            StreamMaxLen: cfg.EventBus.StreamMaxLen,
        })
        if err != nil {
            return err
        }
        return app.Container().Bind(runtime.Key(name), capability)
    })
}
```

生命周期规则：

- capability 新建的 bus 默认会注册为 `core/eventbus` default，并在 `Close` 时关闭和清理 default。
- `WithoutDefault()` 可创建不注册 default 的 bus，但该 bus 仍由 capability 持有并在 `Close` 时关闭。
- `WithBus()` 注入外部 bus，默认不关闭外部 bus；如注册为 default，`Close` 只清理 default 指针。
- `WithOwnedBus()` 表示外部注入 bus 的生命周期交给 capability，`Close` 会关闭该 bus。

## 通用 Realtime Capability

通用 Realtime capability 位于：

```txt
core/realtime/facade
```

职责：

- 基于 `core/realtime.Publisher` 提供服务启动层的实时推送入口。
- 通过 `core/eventbus` 发布 realtime event，不直接操作 websocket/SSE/mqtt 连接。
- 暴露 `PushUser`、`PushRoom`、`Broadcast`、`Close`、`Topic` 和包级默认入口。
- 在服务启动时通过 `RuntimeCapabilities` 注册 `realtime` capability，并可保存服务私有默认入口。

配置：

```go
type Config struct {
    Topic string `mapstructure:"topic" yaml:"topic"`
}
```

`Topic` 为空时使用 `core/realtime.DefaultTopic`，当前默认值是 `rt:events`。

配置映射示例：

```go
func realtimeCapability(ctx bootstrap.RuntimeCapabilityContext) runtime.CapabilityContract {
    name := ctx.LocalName(realtimecap.Name)
    return runtime.NewCapability(name, func(app *runtime.App) error {
        cfg, err := loadServiceConfig(ctx)
        if err != nil {
            return err
        }
        capability, err := realtimecap.New(realtimecap.Config{Topic: cfg.Realtime.Topic})
        if err != nil {
            return err
        }
        defaultRealtime = capability
        return app.Container().Bind(runtime.Key(name), capability)
    })
}
```

publisher 来源顺序：

- `WithPublisher()` 显式注入已有 `core/realtime.Publisher`。
- `WithEventBus()` 显式注入 `core/eventbus.Publisher`，内部创建 `core/realtime.NewPublisher`。
- 未显式注入时复用 `core/eventbus.Default()`。

边界：

- 不创建 eventbus driver，不创建 Redis client；底层 eventbus 由通用 EventBus capability 或启动层初始化。
- 不 import app、worker、crontab 服务包。
- 不读取服务私有配置字段，服务侧在 `RuntimeCapabilities` 的 config loader 中映射到 `realtime.Config`。
- 不实现 app/realtime gateway、认证、consumer、room membership 或 online manager。
- `Close` 不关闭外部 eventbus，只清理 realtime 默认入口和内部 publisher。

错误规则：

- eventbus default 未初始化时，`New` 返回可匹配 `ErrEventBusNotConfigured` 和 `core/eventbus.ErrDefaultNotInitialized` 的错误。
- capability 未初始化或关闭后继续 push，返回 `ErrNotConfigured`。
- publish 失败必须向上传递 error，不允许 panic。

## 约束

- `Provider` 声明 `type/kind/factory`，服务私有能力统一通过 `RuntimeCapabilities` 声明。
- core facade 层对外统一暴露 `Name` 作为 runtime capability 名称；底层 container 的 typed key 只保留在 core 根包绑定实现中，业务和 facade 调用处不要再导出或依赖 `KeyXxx` 别名。
- `ServiceInfo.Capabilities` 不手写静态展示能力；启动表格从 runtime capability metadata 合并。
- `ScopeGlobal` capability 展示在公共能力表；`ScopeServiceLocal` capability 去掉服务名前缀后展示在服务行。
- `ServiceContext.Capabilities` 同时保存 capability 实例；需要在服务内复用通用能力时，优先从 registry 或业务 adapter 获取，不要再复制 service 私有 runtime bootstrap。
- capability 的 `Shutdown` 由主 runtime 管理，不再依赖 `ServiceContext.Capabilities.Close()`。
- 能力初始化失败必须返回 error 或记录 warn，不允许 panic。
- goroutine 必须跟随服务 `Shutdown` 或 context 回收。

## 更新记录

- 2026-05-26：统一 core facade capability 命名出口，移除 facade 层 `KeyXxx` 别名，对外依赖和 metadata 使用 `Name`。
- 2026-05-26：新增 core runtime facade 默认规则：默认行为集中到 `defaultUseOptions()`，框架底座能力默认 internal，外部展示使用 `WithExternal()` 显式声明，配置由 `WithConfig` / `WithConfigLoader` 显式注入。
- 2026-05-16：移除旧泛型注册器、各 `bootstrap/capabilities/*` 的旧声明入口，以及服务内注册函数；服务本地能力统一由 Provider `RuntimeCapabilities` 声明，生命周期交给 runtime。
- 2026-05-16：`filesystem` 从 `bootstrap/capabilities` 迁移到 `infra/capabilities`；`realtime` 下沉到 `core/realtime/facade`；旧 SSE/WebSocket publisher capability 删除；Queue producer 收敛到 `core/queue/facade/producer`，Queue 管理能力收敛到 `core/queue/facade/operations`，Worker queue runtime 由 Worker 服务自行初始化。
- 2026-05-16：删除旧 SSE/WebSocket publisher bridge，实时推送统一通过 `realtime` capability 和服务 `infra/realtime` adapter。
- 2026-05-15：删除服务私有 capability bootstrap，Admin/API/Worker/Crontab 迁移到 `core/storage`、`infra/realtime`、`infra/notify` 等业务 adapter，Realtime Gateway 迁移到 `app/realtime/infra/realtime`。
- 2026-05-11：将实时推送能力收敛到 `core/realtime/facade`，Admin、API、Worker、Crontab 只做服务接入，Realtime Gateway 负责接收 EventBus 并推给浏览器。
- 2026-05-11：补充 Admin 的 `filesystem`、`queue` 通用能力实现，服务目录只保留接入声明和默认入口。
- 2026-05-16：通用 `eventbus` runtime facade 下沉到 `core/eventbus/facade`，`core/eventbus` 根包只保留最小运行时原语。
- 2026-05-16：记录 core runtime facade 文件边界：root `binding.go` 放 `Key/From/Bind`，`facade/use.go` 放 `Use/WithConfig` 和生命周期。
- 2026-05-14：新增通用 eventbus capability 文档，明确 driver 来源、Redis client 外部传入、default bus 设置和 Close 生命周期边界。
- 2026-05-14：新增通用 `core/realtime/facade` 文档，明确 realtime capability 初始化、eventbus 依赖、默认入口、错误模型和 Close 边界。
- 2026-05-14：`CapabilityRegistry` 支持保存 capability 实例、按名称取回和统一 Close 生命周期；服务侧复用通用 capability 时优先从 `ServiceContext.Capabilities` 获取实例。
