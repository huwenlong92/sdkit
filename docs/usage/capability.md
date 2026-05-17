# Capability 使用说明

Capability 是 framework runtime 向骨架声明“当前进程启用了什么能力”的标准。业务层不直接初始化外部资源；服务启动层接入 `infra/capabilities/*` 或 core facade，再把默认实例交给本服务的业务 adapter。

## 两类能力

### Framework Runtime

多个服务通用的 runtime 默认实现放在 `infra/capabilities/*`；core 模块自己的 runtime 接入放在 `core/<module>/facade`。服务通过 `Provider().RuntimeCapabilities(...)` 声明需要的能力，主 runtime 负责读取配置、创建实例、写入 container，并在服务构建前注入到 `ServiceContext.Capabilities`。

| Runtime | 默认实现 |
|---|---|
| `filesystem` | `infra/capabilities/filesystem` |
| `queue` | `core/queue/facade/producer` 或 `core/queue/facade/operations` |
| `eventbus` | `core/eventbus/facade` |
| `realtime` | `core/realtime/facade` |

### Service Adapter

服务目录只保留业务 adapter，例如：

| 服务 | Adapter |
|---|---|
| Admin | `app/admin/infra/storage`、`app/admin/infra/realtime` |
| API | `app/api/infra/storage`、`app/api/infra/realtime` |
| Worker | `worker/infra/storage`、`worker/infra/realtime` |
| Crontab | `crontab/infra/storage`、`crontab/infra/realtime` |
| Realtime Gateway | `app/realtime/infra/realtime` |

如果默认实现不够用，当前服务可以在自己的业务 adapter 中包一层，但 handler/task 不再复制 runtime bootstrap。

实时推送入口统一复用 `core/realtime/facade`。如果后面 Admin 需要事件名前缀、租户补充、脱敏或审计，只改 `app/admin/infra/realtime`。

## 注册规则

服务构造只接收上下文，不做 runtime wiring：

```go
func NewServer(cfg config.ServiceConfig) *Server {
    return NewServerWithContext(cfg, bootstrap.NewServiceContext(cfg.Name, cfg.Type, nil))
}
```

`sdkitgo serve` 时，服务本地能力声明放在 Provider 上；Filesystem 已由 bootstrap 作为公共能力注册，不在服务 Provider 中重复声明：

```go
func Provider() bootstrap.Provider {
    return bootstrap.ProviderFunc(func(app *bootstrap.App) error {
        app.Service("admin").
            RuntimeCapabilities(func(ctx bootstrap.RuntimeCapabilityContext) []runtime.CapabilityContract {
                return []runtime.CapabilityContract{
                    queueops.Use(
                        queueops.WithName(ctx.LocalName(queueops.Name)),
                        queueops.WithConfigLoader(func(*runtime.App) (queueops.Config, error) {
                            cfg, err := config.Load(ctx.ConfigFile, ctx.Name, ctx.BaseConfig(), ctx.ConfigKey)
                            if err != nil {
                                return queueops.Config{}, err
                            }
                            return queueops.NewConfig(cfg.Name, cfg.Type, cfg.Queue), nil
                        }),
                    ),
                }
            }).
            FactoryContext(func(ctx bootstrap.ServiceContext) (bootstrap.Service, error) {
                cfg, err := config.Load(ctx.ConfigFile, ctx.Name, ctx.Base, ctx.ConfigKey)
                if err != nil {
                    return nil, err
                }
                srv, err := NewServerWithContext(cfg, &ctx)
                if err != nil {
                    return nil, err
                }
                return bootstrap.HTTPService{StartFunc: srv.Start, StopFunc: srv.Shutdown}, nil
            })
        return nil
    })
}
```

`RuntimeCapabilities` 返回的服务本地能力名必须使用服务命名空间，例如 `api.queue.producer`、`admin.queue.operations`。`command/serve` 会把这些能力自动追加到 provider dependency，并把初始化后的实例写入当前服务的 `ServiceContext.Capabilities`。启动表格按 runtime capability metadata 展示：`ScopeGlobal` 放公共能力表，`ScopeServiceLocal` 去掉服务名前缀后放服务行。

业务代码只调用能力包暴露的业务接口：

```go
info, err := queue.Enqueue(ctx, task)
fs, err := storage.DefaultFileSystem()
```

配置装配函数不暴露给业务层。能力关闭由 runtime 的 `Capability.Shutdown(ctx)` 统一处理，不在 `NewServerWithContext` 或服务 `Shutdown` 中重复关闭。

## 必填配置

服务声明了某个能力，就必须在 capability config loader 中明确读取对应配置。对于 `eventbus` / `realtime`，必须确保 eventbus 配置已显式提供；缺失时直接启动失败。
