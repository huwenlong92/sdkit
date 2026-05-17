# Config 模块说明

## 职责

`core/config` 是基础配置加载器，职责边界：

- 创建并配置 `viper.Viper`
- 读取 YAML 配置文件
- 按主配置 `imports` 合并功能配置文件
- 绑定 `SDKITGO_` 环境变量
- 将配置反序列化到调用方提供的结构体
- 支持调用方注入默认值

## 非职责

`core/config` 不负责：

- 定义应用级聚合配置
- 定义业务服务配置
- 引用 database、queue、filesystem 等模块类型
- 维护模块默认值
- 初始化 logger、db、redis 或其他运行时资源

公共启动配置放在 `bootstrap`。服务私有配置归属服务本身，例如 `app/admin/config`、`app/api/config`、`worker.Config`。OpenTelemetry tracing 属于公共启动配置，由 `bootstrap.Config.Tracing` 读取 `tracing` key。

## 服务覆盖规则

公共配置只提供基础值：

```yaml
jwt:
  secret: sdkitgo-secret
  issuer: sdkitgo
  expire: 86400

session:
  prefix: session:
```

服务配置按实例覆盖基础值。`services.yaml` 只控制启动，HTTP 运行参数放独立文件：

```yaml
# configs/services.yaml
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

# configs/admin.yaml
admin:
  addr: :8080
  jwt:
    issuer: sdkitgo-admin
    expire: 3600
  session:
    prefix: admin:session:

# configs/api.yaml
api:
  addr: :8081
  jwt:
    issuer: sdkitgo-api

api2:
  addr: :8082
```

`services` 必须显式配置；缺失或为空时 `sdkitgo serve` 直接报错，不再默认启动 `admin/api`。`services.<name>.type` 只选择服务工厂，不选择运行配置。运行配置默认读取同名顶层 key，例如 `api2/type=api` 读取 `api2:`，不会继承 `api:`。如果需要让服务实例读取其它配置块，必须显式设置 `config_key`。服务配置缺失或关键字段为空时，服务配置加载器必须返回错误，并带上 service name、type、config key。Admin/API 只读取服务实例顶层配置，不读取 `server.admin` 或 `server.api`。

覆盖逻辑在服务配置包内实现，`core/config` 只负责读取。

## 拆分配置

主配置可以声明功能文件：

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
  - tracing.yaml
```

约束：

- `imports` 路径相对主配置文件所在目录解析。
- 主文件先读取，功能文件按声明顺序合并。
- 后合并的文件覆盖先前的同名 key。
- 调用方仍然只传主配置路径，例如 `configs/config.yaml`。

## API

```go
func Load(configPath string, out any, opts ...Option) error
func LoadKey(configPath string, key string, out any, opts ...Option) error
func LoadRequiredKey(configPath string, key string, out any, opts ...Option) error
func New(configPath string, opts ...Option) (*viper.Viper, error)
func WithDefault(key string, value any) Option
func WithDefaults(values map[string]any) Option
```

`LoadKey` 适合可选配置，key 不存在时会得到结构体零值。

`LoadRequiredKey` 适合声明了 capability 后必须存在的配置，例如 `eventbus`。它会先检查 key 是否存在，不存在时返回明确错误，避免服务静默使用零值配置。

## 更新记录

- 2026-05-13：明确多服务实例配置语义：`type` 只选择工厂，运行配置默认读取实例同名 key，可用 `config_key` 显式指定；缺失配置直接报错，不做 type 配置继承。
- 2026-05-13：Admin/API 服务配置只读取服务实例顶层配置。
- 2026-05-13：`sdkitgo serve` 不再为缺失 `services` 的配置默认启动 `admin/api`，服务清单必须显式声明。
- 2026-05-11：补充 `services.yaml` 只控制服务启用，服务运行参数放各自配置文件；`eventbus`、`sse`、`websocket`、`worker`、`crontab` 等按功能拆分。
- 2026-05-12：公共启动配置增加 `tracing`，配置拆分到 `configs/tracing.yaml`，用于 OpenTelemetry OTLP/Jaeger 接入。
- 2026-05-11：新增 `LoadRequiredKey`，供 `eventbus`、`realtime` 等必填 capability 配置使用。
- 2026-05-11：支持主配置 `imports`，允许 `configs` 按功能拆分，调用方继续使用主配置路径。
- 2026-05-11：`core/config` 定位为通用 loader，应用公共配置归属 `bootstrap`，服务配置归属各服务包。
