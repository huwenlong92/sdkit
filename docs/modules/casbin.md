# Casbin 模块方案

## 职责

`core/casbin` 提供通用 RBAC manager 和 Gin middleware：

- 加载 Casbin model
- 按 `casbin_rule` 表加载策略
- 自动补充超级角色通配策略
- 提供 `Manager.Enforce`
- 提供服务可组合的 Gin middleware
- 通过 runtime capability 暴露全局 manager

`core/casbin` 不负责业务角色模型、菜单权限、接口扫描和服务路径归一化。这些逻辑放在 app 层。

## Runtime Capability

Runtime 接入层位于 `core/casbin/facade`：

```go
import casbinfacade "github.com/huwenlong92/sdkit/core/casbin/facade"

app.RegisterCapabilities(
    casbinfacade.Use(
        casbinfacade.WithConfig(casbinfacade.Config{
            ModelPath:       "configs/rbac_model.conf",
            SuperRole:       "admin",
            AutoCreateTable: true,
        }),
    ),
)
```

facade 默认作为内部 capability 注册；需要对外展示时显式使用 `WithExternal()`。

capability 名称为 `casbin`，依赖：

- `bootstrap`：可选，用于确保配置先初始化。
- `database`：可选，存在时先于 Casbin 初始化。
- `logger`：可选，存在时先于 Casbin 初始化。

bootstrap 在 `BootConfig.DB == true` 时默认注册 Casbin capability，配置保持历史默认值：

- `ModelPath`: `configs/rbac_model.conf`
- `SuperRole`: `admin`
- `AutoCreateTable`: `true`
- `RuleTable`: `casbin_rule`

## 对外 API

核心入口：

```go
casbin.Init(db, cfg)
casbin.InitContext(ctx, db, cfg)
casbin.New(db, cfg)
casbin.NewContext(ctx, db, cfg)
casbin.From(app)
casbin.Bind(app, manager)
casbin.Reload()
```

facade 入口：

```go
casbinfacade.Use(...)
casbinfacade.From(app)
casbinfacade.Default()
casbinfacade.EnforcerFrom(app)
```

`Init/New/Reload` 兼容旧入口，内部使用 `context.Background()`；runtime 路径由 facade 使用 `NewContext` 透传 app context，并通过 `Bind` 写入 runtime 容器。

## 内部约束

- 默认 manager 通过 `casbin.Default` 保留兼容。
- runtime capability 注册成功后会把 manager 绑定到容器 key `casbin`。
- `RuleTable` 会通过 pgx identifier 转义，禁止拼接未转义表名。
- Middleware 不持有业务角色逻辑，只调用服务传入的 `RoleResolver` 和 `ObjectResolver`。
- 没有 manager、enforcer 或 role resolver 时，中间件直接放行，保持历史兼容行为。

## 更新记录

- 2026-05-26：facade 默认内部注册，新增 `WithExternal()`；移除 `Use` 内无实际分支意义的 `hasConfig` 状态。
- 2026-05-16：新增 `core/casbin/facade` capability facade；bootstrap 在数据库启用时通过 facade 注册 Casbin；新增 `InitContext/NewContext/ReloadContext` 和 `Bind/From` 以支持 runtime context 透传与容器绑定。
