# Config 使用说明

`core/config` 只负责通用配置加载，不维护应用级或服务级配置结构。

## 基础加载

调用方定义自己的配置结构：

```go
type Config struct {
    Worker struct {
        Queue queue.Config `mapstructure:"queue"`
    } `mapstructure:"worker"`
}

var cfg Config
err := config.Load("configs/config.yaml", &cfg)
```

只读取局部配置：

```go
var cfg queue.Config
err := config.LoadKey("configs/config.yaml", "worker.queue", &cfg)
```

读取必填配置：

```go
var cfg realtime.PublisherConfig
err := config.LoadRequiredKey("configs/config.yaml", "eventbus", &cfg)
```

`LoadRequiredKey` 会在 key 不存在时报错。服务声明 `eventbus` 或 `realtime` capability 时应该使用它，避免用户漏引入 `eventbus.yaml` 后服务静默使用零值配置。

默认值由调用方传入，`core/config` 不知道具体模块：

```go
err := config.Load(path, &cfg, config.WithDefault("worker.queue.concurrency", 10))
```

## 拆分配置

主配置文件可以通过 `imports` 引入功能配置。调用方仍然只传主配置路径：

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
  - storage.yaml
  - tracing.yaml

app:
  name: sdkitgo
  mode: dev
```

导入路径相对主配置文件所在目录解析。主文件会先读取，随后按 `imports` 顺序合并功能文件；后合并的文件会覆盖前面的同名 key。

例如 `configs/worker.yaml`：

```yaml
worker:
  queue:
    addr: 127.0.0.1:6379
    concurrency: 10
```

业务代码仍然这样读取：

```go
var cfg queue.Config
err := config.LoadKey("configs/config.yaml", "worker.queue", &cfg)
```

如果项目只有 api 和 admin，不需要 realtime gateway 或 crontab，直接从 `configs/config.yaml` 的 `imports` 删除 `realtime.yaml`、`crontab.yaml` 即可。

如果服务声明了 `eventbus` 或 `realtime`，不能删除 `eventbus.yaml`；否则配置加载会直接失败，避免消息静默落到本进程 memory bus。

## 放置规则

- 公共启动配置放在 `bootstrap.Config`，例如 app、log、database、redis、cache、jwt、session、tracing。
- 服务配置放在服务自己的 `config` 包，例如 `app/admin/config.ServiceConfig`、`app/api/config.ServiceConfig`。
- 模块配置放在模块内，例如 `core/queue.Config`、`core/storage.Config`。
- 服务需要 core 或 pkg 能力时，由服务配置加载器自己组合，不向 `bootstrap.Config` 继续加字段。
- HTTP 服务配置覆盖公共配置。例如 `admin.jwt` 覆盖全局 `jwt`，`admin.session` 覆盖全局 `session`。

## 服务配置

`services` 是 `sdkitgo serve` 的服务实例清单，只控制启动和服务类型。`services` 必须显式配置；缺失或为空时组合启动会直接报错。`type` 表示使用哪个已注册的服务类型，key 是实例名：

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
```

`type` 只决定使用哪个已注册服务工厂，不决定运行配置 key。运行配置默认读取同名顶层 key；`api2/type=api` 需要存在 `api2:` 配置，不会继承 `api:`。如果要读取其它配置块，显式配置 `config_key`。

HTTP 服务运行配置放在独立配置文件，例如 `configs/admin.yaml`：

```yaml
admin:
  addr: :8080
  jwt:
    secret: admin-secret
    issuer: sdkitgo-admin
    expire: 3600
  session:
    prefix: admin:session:
```

`configs/api.yaml`：

```yaml
api:
  addr: :8081
  jwt:
    secret: api-secret
    issuer: sdkitgo-api
    expire: 7200
  session:
    prefix: api:session:

api2:
  addr: :8082
```

`api2` 不需要新增 Go 命令代码，因为 `api` 类型已经由 `app/api/provider.go` 声明、`app/api/register.go` 注册给骨架层；但它必须在 `api.yaml` 中配置自己的 `api2.addr` 等运行参数。服务配置缺失或关键字段为空时会直接报错，错误信息包含 service name、type 和 config key。Admin/API 只读取服务实例顶层配置，不读取 `server.admin` 或 `server.api`。新增一个全新的服务类型时，需要编译进对应服务包，并在包内提供 `provider.go`。

## Admin 示例

服务配置定义在 `app/admin/config`：

```go
type ServiceConfig struct {
    Name       string
    Type       string
    Enabled    bool
    Addr       string
    JWT        auth.JWTConfig
    Session    session.Config
    Queue      queue.Config
    Storage storage.Config
}
```

加载器先拿公共配置作为基础值，再读取服务自己的顶层配置覆盖：

```go
cfg, err := adminconfig.Load(configFile, "admin", bootCfg)
srv := admin.NewServer(cfg)
```

服务对骨架层暴露启动信息：

```go
func (c ServiceConfig) ServiceInfo() bootstrap.ServiceInfo {
    return bootstrap.ServiceInfo{
        Name:    c.Name,
        Type:    c.Type,
        Addr:    c.Addr,
        Enabled: c.Enabled,
    }
}
```

## API 示例

`app/api/config.ServiceConfig` 只保留普通业务 HTTP 服务需要的运行配置，不读取 filesystem、SSE/WebSocket publisher 或 eventbus 配置；API queue 投递示例复用 `worker.queue` 作为 producer 配置：

```go
type ServiceConfig struct {
    Name    string
    Type    string
    Enabled bool
    Addr    string
    JWT     auth.JWTConfig
    Session session.Config
    Limiter LimiterConfig
    BBR     BBRConfig
    Queue   queue.Config
}
```

实例名可以是 `api`、`api2`、`api3`：

```go
cfg, err := apiconfig.Load(configFile, "api2", bootCfg)
srv := api.NewServer(cfg)
```

只要配置里声明：

```yaml
services:
  api2:
    type: api
    enabled: true

api2:
  addr: :8082
```

`sdkitgo serve` 会通过注册表自动构造该实例，并在启动表格里打印它的 `ServiceInfo`。

显式配置 key 示例：

```yaml
services:
  api2:
    type: api
    enabled: true
    config_key: public_api

public_api:
  addr: :8082
```

## Worker 示例

Worker 不从 `bootstrap.Config` 读取 queue、filesystem，而是在自己的配置加载器中按需读取。EventBus 和 Realtime 由 runtime capability 统一初始化：

```go
type Config struct {
    Name       string
    Type       string
    Enabled    bool
    Queue      queue.Config
    Storage storage.Config
}

func Load(configPath string, name string, base *bootstrap.Config) (Config, error) {
    var cfg Config
    configKey := name
    if configKey == "" {
        configKey = "worker"
    }
    if err := config.LoadRequiredKey(configPath, configKey+".queue", &cfg.Queue); err != nil {
        return cfg, err
    }
    if err := config.LoadKey(configPath, "filesystem", &cfg.FileSystem); err != nil {
        return cfg, err
    }
    return cfg, nil
}
```

这保持了边界：骨架层只提供标准和公共能力，服务自己决定要接入哪些 core/pkg 能力。

只要服务声明了 `eventbus` 或 `realtime` capability，就必须显式提供 `eventbus` 配置：

```yaml
imports:
  - eventbus.yaml

eventbus:
  driver: memory
  topic: rt:events
```

缺少 `eventbus` key、缺少 `eventbus.driver`、缺少 `eventbus.topic`，都应该在服务启动阶段直接返回 error。`driver: memory` 可以用于本地和 `sdkitgo serve` 单进程模式，但也必须显式写出，避免用户不知道消息不会跨进程。

Worker 也通过 `ServiceInfo()` 暴露能力信息，并通过 `worker/provider.go` 声明 `worker` 服务类型，`worker/register.go` 只负责注册 Provider。

## 模块配置

模块配置描述“模块自己怎么初始化”。模块默认值和校验留在模块内。

Queue 示例：

```go
package queue

type Config struct {
    Addr           string         `mapstructure:"addr" yaml:"addr"`
    Password       string         `mapstructure:"password" yaml:"password"`
    DB             int            `mapstructure:"db" yaml:"db"`
    Concurrency    int            `mapstructure:"concurrency" yaml:"concurrency"`
    Queues         map[string]int `mapstructure:"queues" yaml:"queues"`
    StrictPriority bool           `mapstructure:"strict_priority" yaml:"strict_priority"`
}
```

Auth 示例：

```go
package auth

type JWTConfig struct {
    Secret string `mapstructure:"secret" yaml:"secret"`
    Issuer string `mapstructure:"issuer" yaml:"issuer"`
    Expire int    `mapstructure:"expire" yaml:"expire"`
}
```

Database 示例：

```go
package database

type Config struct {
    Driver          string `mapstructure:"driver" yaml:"driver"`
    DSN             string `mapstructure:"dsn" yaml:"dsn"`
    TablePrefix     string `mapstructure:"table_prefix" yaml:"table_prefix"`
    Schema          string `mapstructure:"schema" yaml:"schema"`
    MaxOpenConns    int    `mapstructure:"max_open_conns" yaml:"max_open_conns"`
    MaxIdleConns    int    `mapstructure:"max_idle_conns" yaml:"max_idle_conns"`
    ConnMaxLifetime int    `mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime"`
    LogLevel        string `mapstructure:"log_level" yaml:"log_level"`
}
```
