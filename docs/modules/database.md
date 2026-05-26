# Database 模块方案

## 目标

`core/database` 统一初始化并管理 GORM 和 pgxpool，支持在同一套配置、表名前缀和日志体系下混用两类数据库能力。

目标能力：

- 保留 GORM 的模型、AutoMigrate、普通 CRUD、关联和 Hook 能力
- 提供 pgxpool 用于复杂 SQL、CTE、LATERAL、`jsonb_agg`、批量插入、COPY 和高性能查询
- 统一读取数据库配置
- 统一应用 GORM `TablePrefix`
- 原生 SQL 可以安全获取模型对应表名
- pgx 日志接入 `core/logger`，并透传 context 中的链路字段
- 统一关闭 pgxpool 和 GORM 底层 `sql.DB`

## 模块边界

`core/database` 负责：

- 根据配置创建 GORM 连接
- 根据同一 DSN 创建 pgxpool
- 设置连接池参数和连接生命周期
- 提供默认全局入口和独立实例入口
- 提供模型表名解析与 SQL 标识符转义
- 提供简单分页参数辅助
- 提供 GORM 和 pgx 日志适配

`core/database` 不负责：

- 定义业务模型
- 执行业务 AutoMigrate
- 反向依赖 `models`
- 封装具体业务 SQL
- 规定所有 GORM 使用场景必须改成 pgx

## 当前目录

```txt
core/database/
  binding.go
  config.go
  database.go
  facade/
    config.go
    client.go
    use.go
    default.go
  gorm.go
  gorm_logger.go
  page.go
  pgx.go
  pgx_logger.go
  table.go
```

## Runtime Capability

`core/database` 是数据库实现包，Runtime Capability 接入层统一放在 `core/database/facade`：

```go
import databasecap "github.com/huwenlong92/sdkit/core/database/facade"

runtimeApp.RegisterCapabilities(
    databasecap.Use(
        databasecap.WithConfig(cfg.DB),
        databasecap.WithMode(cfg.App.Mode),
    ),
)
```

bootstrap 使用 `databasecap.WithConfigLoader(...)` 和 `databasecap.WithModeLoader(...)`，确保配置能力先初始化，再由 `databasecap.Use` 读取最终配置。
根包不实现 runtime `Use`；根包只保留数据库本体、全局快捷入口和 `Bind(app, db)` 容器绑定能力，避免与 facade 重复实现 capability 注册。根包的 `KeyDatabase`、`From(app)`、`Bind(app, db)` 统一放在 `binding.go`；`Use(...)`、`WithConfig(...)`、生命周期关闭只允许放在 `facade/use.go`。

`databasecap.Use()` 默认按框架底座能力处理，metadata `Internal=true`。需要在启动信息或 CLI 中对外展示 database capability 时，调用方必须显式传入 `databasecap.WithExternal()`。Database facade 不默认读取 `core/config.V`；配置和 mode 由 `WithConfig` / `WithConfigLoader` / `WithMode` / `WithModeLoader` 显式提供。未传配置或现成数据库实例时，会返回 `ErrConfigRequired`，不使用零值配置隐式连接数据库。

## 核心结构

```go
type Config struct {
    Driver          string
    DSN             string
    TablePrefix     string
    Schema          string
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime int
    LogLevel        string
}

type Database struct {
    Gorm   *gorm.DB
    PGX    *pgxpool.Pool
    Config Config
}
```

`PGX` 是 pgxpool 的唯一结构体字段，业务代码和框架内部都使用该字段。

## 初始化方案

全局初始化入口：

```go
database.Init(cfg.DB, cfg.App.Mode)
```

初始化成功后会设置：

```go
database.Default
database.DB
database.PGXPool
```

runtime facade 初始化成功后同样会通过 `database.Bind(app, db)` 设置以上全局快捷入口，并把 `*database.Database` 绑定到 runtime container 的 `database` key。

独立实例入口：

```go
db, err := database.New(ctx, cfg, mode)
if err != nil {
    return err
}
defer db.Close()
```

初始化流程：

1. `normalizeConfig` 设置默认值并校验 DSN。
2. `openGORM` 使用 `gorm.io/driver/postgres` 创建 GORM。
3. `openPGX` 使用同一 DSN 创建 pgxpool 并 `Ping`。
4. pgx 初始化失败时关闭已创建的 GORM 底层连接。

默认值：

| 字段 | 默认值 |
|------|--------|
| `Driver` | `postgres` |
| `MaxOpenConns` | `25` |
| `MaxIdleConns` | `5` |
| `ConnMaxLifetime` | `3600` 秒 |
| `LogLevel` | 非 dev 默认 warn，dev 默认 info |

DSN 必填。`MaxIdleConns` 大于 `MaxOpenConns` 时会被限制到 `MaxOpenConns`。

## 更新记录

- 2026-05-26：Database runtime facade 默认作为 internal 底座能力，新增 `WithExternal()` 显式对外展示；配置和 mode 必须通过 option 显式提供，移除 `core/config.V` 隐式读取。
- 2026-05-16：`core/database/facade` 作为唯一 Runtime Capability 接入层；根包移除重复的 `Use/UseOption`，保留 `Bind/From/Gorm/PGX/Transaction` 等数据库本体 API；根包运行时绑定原语统一放在 `binding.go`。
- 2026-05-15：新增 `database.Gorm(ctx)`、`database.PGX(ctx)`、`database.Transaction` 和 `database.PGXTransaction`，将 pgx 全局快捷变量重命名为 `PGXPool`。
- 2026-05-12：admin/api 请求路径数据库操作透传 `Request.Context()`，支持 HTTP trace 串联 GORM span。
- 2026-05-11：`database.Init` 改为直接接收 `database.Config`，不再依赖 `core/config.DB`。

## GORM 方案

GORM 使用 PostgreSQL driver，并启用：

```go
schema.NamingStrategy{
    TablePrefix:   cfg.TablePrefix,
    SingularTable: true,
}
```

连接池参数来自：

- `MaxIdleConns`
- `MaxOpenConns`
- `ConnMaxLifetime`

GORM 日志通过 `logger.Writer("gorm", "runtime.log")` 写入组件日志。`mode == "dev"` 时同时输出到 stdout。

## pgxpool 方案

pgxpool 使用 `pgxpool.ParseConfig(cfg.DSN)` 解析同一个 DSN，并设置：

- `MaxConns = MaxOpenConns`
- `MinConns = MaxIdleConns`
- `MinIdleConns = MaxIdleConns`
- `MaxConnLifetime = ConnMaxLifetime`
- `MaxConnIdleTime = 30m`

如果配置了 `Schema`，会设置 runtime param：

```go
search_path = cfg.Schema
```

pgx tracer 默认使用 `tracelog.TraceLog`，日志写入 `logger.Named("pgx")`，并追加 `logger.ContextFields(ctx)` 返回的 `track_id` / `trace_id` / `span_id` / `request_id` 等字段。启用 `core/tracing` 后，`openPGX` 会通过 `core/tracing.InstrumentPgxPoolConfig` 把 `otelpgx` 与现有 `tracelog` 合并为 multitracer，同时保留日志和 OpenTelemetry span。`otelpgx` 使用 tracing correlation wrapper 创建 span，因此 pgx query/batch/copy/prepare/connect/acquire span 会写入 `trace_id/span_id/track_id/request_id/traceparent`。

GORM 在 tracing 启用后会注册轻量 callback plugin，覆盖 create/query/update/delete/row/raw。业务必须使用 `db.WithContext(ctx)` 或 `database.Gorm(ctx)`，数据库 span 才能挂到上游 HTTP trace 下。GORM span 会写入 `trace_id/span_id/track_id/request_id/traceparent`，便于在 Jaeger tags 中确认链路归属。admin 的系统管理 handler、登录 hooks、admin/api Casbin middleware 已透传请求 context；新增请求路径需要保持相同约束。

## API DX Helper

handler 和 worker 统一使用 helper 获取数据库资源：

```go
db := database.Gorm(c)
pool := database.PGX(c)
```

`c` 可以是 `*gin.Context`，也可以是普通 `context.Context`。Runtime provider 场景继续使用：

```go
database.GormFrom(app)
database.PGXFrom(app)
```

事务入口：

```go
err := database.Transaction(c, func(tx *gorm.DB) error {
    return tx.Create(&row).Error
})

err = database.PGXTransaction(ctx, func(tx pgx.Tx) error {
    _, err := tx.Exec(ctx, sql, args...)
    return err
})
```

默认数据库未初始化时，`Gorm` 返回 nil、`PGX` 返回 nil，事务 helper 返回 `database.ErrNotInitialized`。

## 日志级别

GORM 支持：

- `silent`
- `none`
- `error`
- `warn`
- `info`
- `debug`

pgx 支持：

- `silent`
- `none`
- `error`
- `warn`
- `info`
- `debug`
- `trace`

未配置时，dev 模式下默认更便于排查，生产模式默认 warn。

## 表名方案

不要手写表名前缀，也不要拼接 `table_prefix + table`。

全局函数：

```go
database.TableName(&models.SystemUser{})
database.Table(&models.SystemUser{})
```

对象方法：

```go
db.TableName(&models.SystemUser{})
db.Table(&models.SystemUser{})
```

泛型方法：

```go
database.TableOf[models.SystemUser](db)
database.TableOf[*models.SystemUser](db)
```

差异：

| 方法 | 返回 | 用途 |
|------|------|------|
| `TableName` | `sd_system_user` | 日志、GORM `Table()`、可信内部场景 |
| `Table` | `"sd_system_user"` 或 `"schema"."sd_system_user"` | pgx 原生 SQL 标识符位置 |

`Table()` 使用 `pgx.Identifier.Sanitize()` 转义。如果配置了 `Schema`，返回 schema-qualified 标识符。

## 分页方案

`Page` 负责提供统一分页规则，pgx 原生 SQL 可以直接使用 `LIMIT` 和 `OFFSET`：

```go
p := database.Page{Page: page, PageSize: pageSize}
limit := p.Limit()
offset := p.Offset()
```

GORM 查询使用 `Paginate` Scope：

```go
err := database.DB.WithContext(ctx).
    Model(&models.SystemUser{}).
    Scopes(database.Paginate(page, pageSize)).
    Find(&users).Error
```

规则：

- `Page <= 0` 时按 1 处理
- `PageSize <= 0` 时按 20 处理
- `PageSize > 100` 时按 100 处理

## 关闭方案

实例关闭：

```go
err := db.Close()
```

全局关闭：

```go
database.Close()
```

关闭顺序：

1. 关闭 pgxpool。
2. 关闭 GORM 底层 `sql.DB`。
3. 清空全局入口。

`Database.Close()` 会返回 GORM 底层关闭错误；pgxpool `Close()` 本身不返回 error。

## 使用约束

- GORM 负责 AutoMigrate、普通 CRUD、模型关系、Hook、简单后台列表。
- pgx 负责复杂 SQL、CTE、LATERAL、`jsonb_agg`、批量插入、COPY、高性能查询。
- 原生 SQL 的值必须继续使用 `$1`、`$2` 这类参数绑定。
- 表名和列名不能参数绑定，必须使用 `database.Table()` 或 `pgx.Identifier`。
- `core/database` 不反向依赖业务模型，批量插入函数应放在具体业务包。
- 业务代码统一使用 `PGX` 字段。

## 已知限制

- 当前只支持 PostgreSQL。
- `Driver` 当前固定为 `postgres` 语义，未提供其他数据库 driver 实现。
- GORM 日志复用标准 writer，不是 Zap encoder。
- `database.TableName()` 在 `Default` 尚未初始化时返回空字符串。

## 更新记录

- 2026-05-16：GORM 和 pgx 数据库 span 随 `core/tracing` correlation helper 补齐 `span_id` attribute。
- 2026-05-15：API DX 阶段新增 `Gorm`/`PGX`/事务 helper，业务 handler 可直接透传 Gin context。
- 2026-05-13：GORM 和 pgx 数据库 span 统一写入 `trace_id/track_id/request_id/traceparent` correlation attributes。
- 2026-05-13：`Database` 结构体内统一使用 `PGX` 字段。
- 2026-05-10：补充 GORM + pgxpool 混用方案、表名规则、Schema/search_path、日志接入和关闭边界。
