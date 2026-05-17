# 目录规则

## 总体分层

```txt
cmd/sdkitgo/          二进制入口，只放 main()
command/             Cobra 命令装配，只解析参数、初始化、调用服务能力
bootstrap/           骨架层，公共启动、服务标准、服务注册
app/<svc>/           HTTP 服务模块
worker/              Worker 服务模块
crontab/             Crontab 定时任务管理器
core/                核心能力
pkg/                 工具库能力
configs/             主配置和功能配置
docs/                文档
```

## app 服务目录规则

`app/` 只放 HTTP 类服务，包括普通 HTTP、WebSocket gateway、SSE。`sdkitgo-cli` 新增 app 服务时必须按下面模板生成。

```txt
app/<service>/
  config/            必须：服务配置结构、默认值合并、Load
    service.go       必须：ServiceConfig/Config 和 Load
  handler/           必须：HTTP/WebSocket/SSE handler
  internal/          可选：服务内部业务语义、私有 helper、不可跨服务复用的规则
  infra/             可选：服务私有基础设施适配
  middleware/        可选：服务私有 Gin middleware
  tests/             可选：跨 handler / 跨服务集成测试
  provider.go        必须：声明服务如何接入 bootstrap
  register.go        必须：Provider 注册入口
  router.go          普通 HTTP/SSE 必须；WebSocket gateway 可按需省略
  server.go          必须：服务启动、资源注入、关闭
  options.go         可选：运行参数归一化
```

目录职责：

| 目录/文件 | 职责 | 禁止 |
|---|---|---|
| `config/` | 定义服务配置、读取服务自己的配置 key、组合 core/pkg 配置 | 初始化 DB/Redis/Queue 等运行资源 |
| `handler/` | 只放入口 handler，处理请求、参数绑定、调用模块能力 | 放启动逻辑、全局资源初始化 |
| `internal/` | 服务内部业务语义和私有 helper，例如把 `auth.Identity.SubjectID` 解释为 AdminID/UserID | 放外部资源接入、跨服务通用能力 |
| `infra/` | 服务私有适配，例如 access log writer、auth hooks、文件系统实例 | 放可复用核心能力 |
| `middleware/` | 服务私有 middleware，例如 Admin Casbin、服务级 token bucket | 放跨服务通用 middleware |
| `provider.go` | 声明服务 type、kind、factory，是服务接入骨架的标准入口 | 放 DB/Redis/Queue 具体实现 |
| `register.go` | 调用 `bootstrap.RegisterProvider(Provider())`，把服务 Provider 注册到骨架 | 写业务逻辑、写能力实现 |
| `router.go` | 组装路由和 middleware 顺序 | 初始化长生命周期资源 |
| `server.go` | 注入依赖、创建 router、启动和 Shutdown | 解析 CLI flags |
| `options.go` | 将配置归一化为运行参数 | 读取配置文件 |
| `tests/` | 跨包或跨服务集成测试 | 放生产代码 |

`handler/` 可以承载简单业务逻辑，也允许直接使用 `database.DB` 做局部数据访问；这不是必须拆 service/repository 的场景。但 handler 内的 DB 调用必须显式处理 `Count`、`Find`、`First`、`Exec`、`Create`、`Updates`、`Delete` 等返回的 `Error`，分页接口必须分别处理总数和列表查询错误。涉及多条写语句的一致性更新必须使用事务；列表和关联数据查询要避免 N+1，优先使用批量查询或 join。原始 SQL 只允许用于明确、局部的场景，必须参数化，并检查执行错误。

`internal/` 是服务内部业务语义层，只给当前服务使用。它适合放当前服务专属的业务 helper、身份语义解释、服务内不可复用规则。例如 `app/admin/internal/auth.AdminID(c)` 将 `core/auth.Identity.SubjectID` 解释为 Admin ID，并校验 `SubjectType == "admin"`；`app/api/internal/auth.UserID(c)` 将其解释为 API User ID。

判断一个东西该不该放 `internal/`：

- 是当前服务多个 handler 都要用的业务语义 helper，放 `internal/`。
- 是不接 DB/Redis/Queue/第三方 SDK 的纯业务解释或轻量规则，放 `internal/`。
- 是当前服务边界内的私有规则，且不应该被其他服务 import，放 `internal/`。
- 是外部资源接入、core/pkg 能力适配、运行时组件初始化，不放 `internal/`，应该放 `infra/`。
- 是跨服务通用能力，不放 `app/<service>/internal`，应该沉到 `core/`、`pkg/` 或明确的共享包。

`internal/` 不等于强制拆 service/repository。简单业务仍然可以留在 `handler/`。只有当 helper 或规则需要被多个 handler 共享，但又不应该出当前服务边界时，才放 `internal/`。

`infra/` 是服务自己的基础设施适配层。它不是业务逻辑层，也不是框架核心层，主要负责把当前服务需要用到的外部资源、`core` 能力、`pkg` 工具，按服务自己的方式接起来。

判断一个东西该不该放 `infra/`：

- 是服务私有的资源接入，放 `infra/`。
- 是队列、文件系统、事件总线、第三方 SDK 等服务差异能力的适配，放 `infra/`。
- 是把 `core` 能力包装成当前服务可用的依赖，放 `infra/`。
- 是业务 handler，不放 `infra/`。
- 是通用能力，不放 `infra/`，应该放到 `core/` 或 `pkg/`。
- 是服务配置结构，不放 `infra/`，应该放到 `config/`。
- 是服务启动入口，不放 `infra/`，应该留在根包 `server.go` / `register.go`。

简单说：

```txt
config   = 这个服务怎么配置
handler  = 这个服务做什么业务
internal = 这个服务内部如何解释业务语义
infra    = 这个服务依赖什么外部能力，以及怎么接上
provider = 这个服务如何声明并接入骨架
server   = 这个服务怎么启动和关闭
register = Provider 注册入口
```

`infra/` 是接线层，不应该堆业务规则。业务规则应该留在 `handler/`、`internal/`、业务 service，或者更底层的 core/domain 模块里。以认证为例：登录查库、密码校验、构造身份的 hooks 属于 Admin 自己的 auth 组件，放 `app/admin/infra/component/auth`；从 Gin context 取出身份并解释成 AdminID/UserID 的 helper 属于服务内部业务语义，放各自服务的 `internal/auth`。

例如 Admin 投递 Worker 任务时，handler 使用已注入 request context 的 `queue.Enqueue(ctx, task)`，不直接持有底层队列 client。文件上传接口只调用 `app/admin/infra/storage`。`server.go` 负责把 `infra/capabilities/*` 或 core facade 初始化出的 framework runtime 交给服务入口。

`infra/` 内部先按类型分组，再按能力或组件分目录：

```txt
infra/
  storage/                文件系统业务 adapter
  realtime/               实时推送业务 adapter
  component/              只服务于本服务内部的基础设施组件
    auth/                 登录认证 hooks
    accesslog/            访问日志 writer
```

如果某个 adapter 当前只有一个文件，也要优先保留目录，方便后续扩展并保持结构一致。adapter 目录内部再按文件职责拆分，例如：

```txt
infra/notify/
  queue.go            任务投递和队列管理入口
```

服务 kind 规则：

| 服务目录 | 注册 type | 注册 kind |
|---|---|---|
| `app/admin` | `admin` | `http` |
| `app/api` | `api` | `http` |
| `app/realtime` | `realtime` | `http` |
| 新增普通 HTTP 服务 | 自定义，例如 `api3` | `http` |

运行产物禁止放在 `app/<service>/` 下，例如日志必须写到根目录 `logs/`。

## 非 app 服务目录规则

Worker 不是 `app/` 服务，Crontab 是根目录定时任务管理器。`sdkitgo-cli` 新增同类模块时也按这个规则生成。

```txt
worker/
  config/            必须：Worker 配置结构和 Load
  event/             必须：队列业务事件 handler
  registry.go        必须：显式注册 event 和按任务 middleware
  infra/             必须：Worker 私有基础设施适配，按能力分目录
    storage/         文件系统实例 adapter
    realtime/        SSE / realtime 推送 adapter
  taskdef/           必须：任务类型、payload、任务构造函数
  tests/             可选：Worker 集成测试
  README.md          必须：目录职责说明
  provider.go        必须：声明 type=worker kind=queue 和 factory
  register.go        必须：Provider 注册入口
  server.go          必须：队列启动和关闭

crontab/
  bootstrap/         必须：Crontab 启动装配和依赖构建
  config/            必须：Crontab 配置结构和 Load
  infra/             必须：Crontab 私有基础设施适配，按能力分目录
    capability/      可选：暴露给其他服务使用的 capability facade
    storage/         必须：文件系统实例
    realtime/        可选：SSE / realtime publisher 入口
    store/           必须：GORM store 和日志写入
    lock/            必须：Redis 分布式锁
  demo_*.go          必须：示例 corecron.Template 声明和示例执行实现
  README.md          必须：目录职责说明
  provider.go        必须：声明 type=crontab kind=cli 和 factory
  register.go        必须：Provider 注册入口
  server.go          必须：调度器启动和关闭
  router.go          必须：显式注册项目任务模板
```

服务自己的配置必须放在自己的 `config/` 子包。配置加载入口统一放在服务 `config/` 子包内。

非 app 服务根包只放入口和标准接入：

- `worker` 根包只保留 `provider.go`、`register.go`、`server.go`。
- `crontab` 根包保留 `provider.go`、`register.go`、`server.go`、`router.go`、`demo_*.go`。
- `crontab/router.go` 负责显式模板注册；后台/命令行通过 `core/crontab.Service` 查询模板和管理 DB Entry，不在业务侧重复定义 catalog。
- `crontab/demo_*.go` 直接定义示例 Template 和示例执行函数，不使用 `init()` 隐式注册；模板统一由根包 `router.go` 显式注册。
- 队列、文件系统、事件总线等服务差异能力的具体适配统一放各自服务的 `infra/`。
- `command/serve` 只 blank import `sdkitgo/crontab`，由 crontab Provider 接入 `sdkitgo serve`。

## sdkitgo-cli 生成规则

新增普通 HTTP 服务时，CLI 至少生成：

```txt
app/<service>/config/service.go
app/<service>/handler/ping.go
app/<service>/provider.go
app/<service>/register.go
app/<service>/router.go
app/<service>/server.go
```

同时修改或提示用户修改：

```yaml
services:
  <service>:
    type: <service>
    enabled: true

<service>:
  addr: :8082
```

生成代码必须满足：

- `services.<name>` 只读取 `type/enabled`，用于 `sdkitgo serve` 装配和启动开关。
- `config.Load(configFile, name, base)` 读取 `<service>` 顶层配置，例如 `api.addr`、`api.limiter`。
- `provider.go` 使用 `app.Service("<service>").Kind(...).FactoryContext(...)` 声明服务接入骨架的方式。
- `register.go` 只调用 `bootstrap.RegisterProvider(Provider())`，不写业务逻辑。
- `handler` 成功响应使用 `response.Success`；错误响应使用 `response.Error` + `core/errors.AppError`，不生成 `response.Fail` 或业务 `c.JSON(...)`。
- `server.go` 接收 `<service>/config.ServiceConfig`，可在组装层接收 `bootstrap.ServiceContext`，但不接收完整 `bootstrap.Config`。
- 服务需要 core/pkg 能力时，由本服务 `config/` 和 `server.go` 读取、组合、适配；framework runtime 由 `infra/capabilities/*` 或 core facade 初始化，业务侧入口放在 `infra/<adapter>`。
- 不修改 `bootstrap.Config` 增加服务私有字段。

## HTTP Middleware 规则

```txt
app/middleware/          所有 HTTP 服务共用的 Gin middleware
app/admin/middleware/    Admin 私有 middleware
app/api/middleware/      API 私有 middleware
core/ratelimit/          限流算法和配置类型
```

例如 BBR 是进程级 HTTP 过载保护，放在 `app/middleware` 做 Gin 适配，算法仍在 `core/ratelimit`。

## CLI 命令规则

`command/` 只负责命令层：

- 定义 Cobra 命令、flags、usage
- 调用 `bootstrap.Init`
- 加载对应服务或模块配置
- 调用服务的 `Start` / `Shutdown` 或模块 API
- 输出 CLI 结果

`command/` 不负责：

- 定义服务配置结构
- 写业务逻辑
- 直接拼装 handler、router、queue handler
- 持有长期运行资源的内部细节

组合启动命令统一为 `sdkitgo serve`。不要生成 `sdkitgo all`、`NewAll` 或其它组合启动入口。

新增 CLI 命令时：

```txt
command/report/report.go     # Cobra 命令
app/report/config/           # 如果是服务或业务模块，配置放这里
app/report/server.go         # 如果需要长期运行
core/report/                 # 如果是可复用核心能力
pkg/report/                  # 如果是纯工具库能力
```

命令实现应该是薄入口：

```go
func New(configFile *string) *cobra.Command {
    return &cobra.Command{
        Use: "report",
        Run: func(cmd *cobra.Command, args []string) {
            cfg, err := reportconfig.Load(*configFile)
            if err != nil {
                logger.L.Fatal("Report配置加载失败", zap.Error(err))
            }
            if err := report.Run(cmd.Context(), cfg); err != nil {
                logger.L.Fatal("Report执行失败", zap.Error(err))
            }
        },
    }
}
```

## 配置文件规则

`configs/config.yaml` 是主入口，功能配置通过 `imports` 拆分：

```yaml
imports:
  - services.yaml
  - admin.yaml
  - api.yaml
  - limiter.yaml
  - realtime.yaml
  - eventbus.yaml
  - worker.yaml
  - crontab.yaml
  - filesystem.yaml
```

文件名优先按服务或功能归属命名：

- `services.yaml`：`sdkitgo serve` 服务实例清单，只放 `type/enabled`
- `admin.yaml` / `api.yaml`：HTTP 服务运行配置，例如地址、服务级 JWT、Session、Limiter
- `limiter.yaml`：全局限流配置，例如 BBR
- `realtime.yaml`：Realtime Gateway 配置
- `eventbus.yaml`：EventBus 通道配置，声明 `eventbus` 或 `realtime` 的服务必须显式配置
- `worker.yaml`：Worker 服务配置，例如 `worker.queue`
- `crontab.yaml`：Crontab 服务配置
- `filesystem.yaml`：文件工具库配置

## 更新记录

- 2026-05-11：明确服务、CLI、config、middleware 的目录边界。
- 2026-05-13：明确 `register.go` 是正式 Provider 注册入口。
- 2026-05-13：CLI 生成规则收敛到 `sdkitgo serve`，新增 handler 错误响应统一生成 `response.Error`。
