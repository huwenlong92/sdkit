# 数据库

`core/database` 同时初始化 GORM 和 pgxpool：

| 入口 | 用途 |
|------|------|
| `database.Default` | 当前默认数据库对象，包含 `Gorm` 和 `PGX` |
| `database.DB` | `*gorm.DB` 全局快捷入口 |
| `database.PGXPool` | `*pgxpool.Pool` 全局快捷入口 |
| `database.Gorm(ctx)` | 返回绑定 context 的 `*gorm.DB` |
| `database.PGX(ctx)` | 返回默认 `*pgxpool.Pool` |

职责划分：

- GORM：AutoMigrate、普通 CRUD、模型关系、Hook、简单后台列表
- pgx：复杂 SQL、CTE、LATERAL、`jsonb_agg`、批量插入、COPY、高性能查询

数据库使用规范：

- 不要在业务 SQL 里手写表前缀。
- 不要直接拼 `table_prefix + table`。
- 不要用 pgx 重写所有 GORM 能力。
- 原生 SQL 的值必须用参数绑定。
- 原生 SQL 的表名、列名必须通过 `database.Table()` 或 `pgx.Identifier` 转义。

## 初始化

`core/database` 保留数据库实现和业务 API；Runtime 接入门面放在 `core/database/facade`：

```go
import databasecap "github.com/huwenlong92/sdkit/core/database/facade"

app := runtime.New()
app.RegisterCapabilities(
    databasecap.Use(
        databasecap.WithConfig(appCfg.DB),
        databasecap.WithMode(appCfg.App.Mode),
    ),
)
```

bootstrap 会在主 Runtime 中先加载配置，再通过 `core/database/facade` 的 `databasecap.Use(databasecap.WithConfigLoader(...))` 注册公共 database 能力。业务查询仍然直接使用根包 `github.com/huwenlong92/sdkit/core/database`。
根包 `core/database` 的 `Key/From/Bind` 约定统一放在 `binding.go`；真正的 runtime `Use` 只在 `core/database/facade/use.go`。

`databasecap.Use()` 默认是内部底座能力。只有需要把 database capability 展示给外部启动信息或 CLI 时，才传入 `databasecap.WithExternal()`。未传 `WithConfig` / `WithConfigLoader` / `WithDatabase` 时会返回 `ErrConfigRequired`，不会从 `core/config.V` 隐式读取 database 配置或 app mode。

初始化完成后：

```go
database.Default.Gorm // *gorm.DB
database.Default.PGX  // *pgxpool.Pool

database.DB      // database.Default.Gorm
database.PGXPool // database.Default.PGX
```

也可以直接创建独立数据库对象，适合测试、工具命令或多实例场景：

```go
db, err := database.New(ctx, database.Config{
    Driver:          "postgres",
    DSN:             dsn,
    TablePrefix:     "sd_",
    Schema:          "public",
    MaxOpenConns:    25,
    MaxIdleConns:    5,
    ConnMaxLifetime: 3600,
    LogLevel:        "warn",
}, mode)
if err != nil {
    return err
}
defer db.Close()
```

`database.Init(cfg, mode)` 会维护全局快捷入口 `database.DB` 和 `database.PGXPool`。`cfg` 使用 `database.Config`，由应用或服务配置组合传入。

GORM 日志会忽略 `ErrRecordNotFound`，正常未命中查询不会输出 `record not found` SQL 日志；业务代码仍应显式判断并处理 `gorm.ErrRecordNotFound`。

## 表名前缀

表名前缀在 `configs/config.yaml` 的 `database.table_prefix` 配置：

```yaml
database:
  table_prefix: sd_
  schema: public
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 3600
  log_level: warn
```

改为空字符串即为无前缀。

`schema` 会用于 pgxpool 的 `search_path`，并影响 `database.Table()` 返回的原生 SQL 标识符；为空时不限定 schema。

连接池配置使用 `max_open_conns` 和 `max_idle_conns`。`conn_max_lifetime` 单位为秒。

日志级别：

- GORM：`silent` / `none` / `error` / `warn` / `info` / `debug`
- pgx：`silent` / `none` / `error` / `warn` / `info` / `debug` / `trace`

## GORM 用法

AutoMigrate、普通 CRUD、模型关系、Hook、简单后台列表继续使用 GORM：

```go
database.DB.Model(&models.SystemUser{}).Where("username = ?", name).First(&user)
database.DB.Model(&models.SystemUser{}).Where("id = ?", id).Update("status", 1)
```

也可以通过对象入口使用：

```go
database.Default.Gorm.Create(&user)
```

启用 `core/tracing` 后，GORM create/query/update/delete/row/raw 会自动生成数据库 span。业务代码必须透传 context：

```go
database.DB.WithContext(ctx).Model(&models.SystemUser{}).Where("username = ?", name).First(&user)
```

GORM span 会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`，便于在 Jaeger tags 中直接确认数据库操作属于哪条链路和哪次业务请求。

Gin handler 中使用：

```go
db := database.Gorm(c)
if err := db.First(&user, id).Error; err != nil {
    return err
}
```

Worker 或普通后台逻辑中使用：

```go
db := database.Gorm(ctx)
if err := db.First(&user, id).Error; err != nil {
    return err
}
```

## pgx 用法

复杂 SQL、CTE、LATERAL、`jsonb_agg`、批量插入、COPY、高性能查询使用 pgx：

```go
table := database.Table(&models.SystemUser{})

rows, err := database.PGX(ctx).Query(ctx, "SELECT id, username FROM "+table+" WHERE status = $1", 1)
if err != nil {
    return err
}
defer rows.Close()
```

启用 `core/tracing` 后，pgx query/batch/copy/prepare/connect/acquire 会自动生成数据库 span。现有 pgx SQL 日志仍然保留，pgx span 也会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`。

SQL 值必须用 pgx 参数绑定；表名、列名这类标识符不能参数绑定，必须使用 `database.Table()` 或 `pgx.Identifier` 转义后再拼接。

不要因为引入 pgx 就把普通 CRUD、模型关系、Hook 或后台列表重写成 pgx。

## 表名

不要硬编码表名，不要手写表前缀，不要直接拼 `table_prefix + table`。用以下方式获取完整表名：

### 1. `models.TableXxx` 变量（推荐）

bootstrap 初始化数据库后会调用 `models.InitTableNames()`，此时可直接使用，最简洁：

```go
import "sdkitgo/models"

database.DB.Exec("DELETE FROM "+models.UserT+" WHERE id = ?", id)
database.DB.Raw("SELECT * FROM "+models.UserT+" WHERE id = ?", id)
```

独立测试、工具命令或手动调用 `database.New()` 的场景，如果要使用 `models.XxxT`，需要在数据库初始化后显式调用：

```go
models.InitTableNames()
```

预定义变量：

| 变量 | 模型 | 示例值 |
|------|------|--------|
| `models.UserT` | SystemUser | `sd_system_user` |
| `models.RoleT` | SystemRole | `sd_system_role` |
| `models.MenuT` | SystemMenu | `sd_system_menu` |
| `models.ApiT` | SystemApi | `sd_system_api` |
| `models.ConfigureT` | SystemConfigure | `sd_system_configure` |
| `models.AccessLogT` | SystemAccessLog | `sd_system_access_log` |

### 2. `database.TableName()` / `database.Table()`

运行时根据模型解析表名。`TableName()` 返回未加 SQL 引号的表名，适合日志、GORM `Table()` 或已有可信拼接场景；`Table()` 返回 pgx 安全转义后的标识符，适合原生 SQL 的表名位置。

```go
name := database.TableName(&models.SystemUser{}) // sd_system_user
table := database.Table(&models.SystemUser{})    // "sd_system_user"
```

如果配置了 `database.schema`，`Table()` 会返回 schema-qualified 标识符：

```go
table := database.Table(&models.SystemUser{}) // "public"."sd_system_user"
```

对象方法也可用：

```go
name := database.Default.TableName(&models.SystemUser{})
table := database.Default.Table(&models.SystemUser{})
```

泛型写法：

```go
table := database.TableOf[models.SystemUser](database.Default)
```

`database.TableName()` / `database.Table()` 在 `database.Default` 未初始化时返回空字符串。需要明确实例边界时优先使用对象方法。

### 3. 直接用 GORM Model()

GORM 自动处理表名，推荐在有模型操作时使用，不需要手动取表名：

```go
database.DB.Model(&models.SystemUser{}).Where("username = ?", name).First(&user)
database.DB.Model(&models.SystemUser{}).Where("id = ?", id).Update("status", 1)
```

项目不提供 `database.T()` 简写。

## 分页

GORM 列表查询使用 `database.Paginate` Scope：

```go
err := database.DB.WithContext(ctx).
    Model(&models.SystemUser{}).
    Scopes(database.Paginate(request.Page, request.PageSize)).
    Order("id DESC").
    Find(&users).Error
```

pgx 原生 SQL 使用 `database.Page`：

```go
p := database.Page{Page: request.Page, PageSize: request.PageSize}

rows, err := database.PGX(ctx).Query(ctx,
    "SELECT id, username FROM "+database.Table(&models.SystemUser{})+" ORDER BY id DESC LIMIT $1 OFFSET $2",
    p.Limit(),
    p.Offset(),
)
```

规则：

- `page <= 0` 默认 1
- `page_size <= 0` 默认 20
- `page_size` 最大 100

GORM 和 pgx 都复用同一套 `database.Page` 规则；`Paginate` 只是 GORM Scope 适配层。

## 事务 Helper

HTTP handler 中优先使用统一事务入口，context 会随 Gin request 透传：

```go
err := database.Transaction(c, func(tx *gorm.DB) error {
    if err := tx.Create(&order).Error; err != nil {
        return err
    }
    return tx.Model(&account).Update("balance", gorm.Expr("balance - ?", amount)).Error
})
```

pgx 事务使用：

```go
err := database.PGXTransaction(ctx, func(tx pgx.Tx) error {
    _, err := tx.Exec(ctx, "UPDATE "+table+" SET status = $1 WHERE id = $2", 1, id)
    return err
})
```

`database.Transaction` 和 `database.PGXTransaction` 在默认数据库未初始化时返回 `database.ErrNotInitialized`。

## 批量插入示例

批量插入适合放在具体业务包内，避免 `core/database` 反向依赖 `models`。示例：

```go
func InsertAccessLogs(ctx context.Context, db *database.Database, logs []models.SystemAccessLog) error {
    if len(logs) == 0 {
        return nil
    }

    table := db.Table(&models.SystemAccessLog{})
    batch := &pgx.Batch{}
    for _, item := range logs {
        batch.Queue(
            "INSERT INTO "+table+" (source, trace_id, uid, method, path, status_code, latency, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)",
            item.Source,
            item.TraceID,
            item.UID,
            item.Method,
            item.Path,
            item.StatusCode,
            item.Latency,
            item.CreatedAt,
            item.UpdatedAt,
        )
    }

    br := db.PGX.SendBatch(ctx, batch)
    defer br.Close()
    for range logs {
        if _, err := br.Exec(); err != nil {
            return err
        }
    }
    return nil
}
```

## 关闭

独立创建的数据库对象必须关闭：

```go
db, err := database.New(ctx, cfg, mode)
if err != nil {
    return err
}
defer db.Close()
```

应用退出时可关闭全局默认连接：

```go
database.Close()
```

关闭会释放 pgxpool 和 GORM 底层 `sql.DB`。不要在请求处理过程中关闭全局连接。
