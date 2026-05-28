# 可选依赖与框架适配规范

本文约束 `sdkit` 与 `sdkitgo` 的可选依赖、driver provider 和 Web 框架适配方式。目标是：业务功能保留，但未启用的 driver、provider、框架 SDK 不进入最终二进制。

## 基本原则

- `core` 只定义稳定抽象、配置结构、注册表、facade、runtime contract 和通用 helper。
- 具体第三方 SDK 实现放在 `pkg` 或框架 adapter 目录，不能由 `core` 根包自动 import。
- 可选能力必须显式启用：通过 build tag 编译 driver/provider，通过启动接线层注册 driver/provider。
- 一个二进制只编译它需要的 driver/provider。不要为了“默认全能”把多个实现一起打进去。
- 业务服务只注册自己需要的 capability。`bootstrap` 只注册公共基础能力，不替所有服务加载 web/admin/worker 私有能力。
- 规范优先级高于历史兼容。新框架、新 driver、新 provider 不保留旧包袱。

## Build Tag 命名

项目级可选能力统一使用 `sdkit_<module>_<provider>`：

```txt
sdkit_storage_s3
sdkit_storage_cos
sdkit_storage_oss

sdkit_queue_asynq
sdkit_queue_nats

sdkit_eventbus_redis
sdkit_eventbus_redis_stream
sdkit_eventbus_nats

sdkit_tracing_otel

sdkit_sms_aliyun
sdkit_sms_feige
sdkit_sms_twilio
sdkit_sms_tencentcloud
sdkit_sms_huawei

sdkit_payment_alipay
sdkit_payment_wechat
sdkit_payment_stripe
sdkit_payment_paypal
```

约定：

- tag 名必须稳定、语义明确，禁止使用临时名称作为正式能力开关。
- 正式能力 tag 使用 `sdkit_` 前缀。
- 临时验证入口可以使用 `worker_slim_*` 等测试 tag，但不能作为真实业务依赖规则。
- 一个 provider 一个 tag。不要用一个 tag 同时启用多个 SDK。
- provider 文件必须在文件第一行声明 `//go:build <tag>`。
- 需要提供未编译时的 stub 或说明文件时，使用反向 tag：`//go:build !<tag>`。

示例：

```go
//go:build sdkit_storage_cos

package cos
```

```go
//go:build !sdkit_storage_cos

package cos
```

## Driver / Provider 边界

新增 driver/provider 时按以下边界拆分：

```txt
core/<module>/              抽象、配置、registry、facade、runtime contract
pkg/<module>/<provider>/    具体 SDK 实现
app/.../drivers/            应用启动接线层，按已编译 tag 注册 provider
```

要求：

- `core/<module>` 禁止 blank import 具体 driver。
- `core/<module>` 禁止直接 import 厂商 SDK。
- `pkg/<module>/<provider>` 可以 import 第三方 SDK，但必须受 build tag 保护。
- driver 注册必须由启动接线层显式调用，例如 `Register()` 或 `core.RegisterDriver(...)`。
- 未编译 driver 时，启动应返回清晰错误，不能静默降级到另一个 driver。
- 多 driver 配置可以存在，但二进制只支持已编译的 driver。配置了未编译 driver 时必须启动失败。
- 默认配置文件可以列完整字段作为 example，但默认运行路径只启用一个 driver。

## 多 Driver 配置

配置允许表达多个实例，例如：

```yaml
storage:
  default: cos1
  disks:
    cos1:
      driver: cos
    minio1:
      driver: minio
```

规则：

- `default` 只表示运行时默认实例，不代表所有 driver 都应编译。
- 多实例配置是运行时选择能力，不是编译能力。
- 如果 `default` 指向未编译 driver，应直接返回错误。
- 如果某个服务没有声明使用该能力，不应因为配置文件存在而初始化该能力。
- driver 专属字段由对应 driver 读取，`core` 只保留跨 driver 的通用配置。

## build.yaml

`build.yaml` 是二进制裁剪入口，用来声明当前构建启用哪些 tag。

要求：

- build 脚本只把值为 `true` 的 tag 传给 `go build -tags`。
- `build.yaml` 不能替代代码边界；没有 build tag 的 provider 即使配置为 false 也可能被打包。
- 每次新增 provider，必须同时补齐：
  - provider 文件 `//go:build`
  - 未编译 stub 或清晰错误
  - build 配置项
  - 启动接线层注册逻辑
  - 打包验证命令

## Gin 适配规则

Gin 相关能力统一放在 `core/gin/*`，例如：

```txt
core/gin/accesslog
core/gin/casbin
core/gin/cors
core/gin/ratelimit
core/gin/recovery
core/gin/requestid
core/gin/security
core/gin/session
core/gin/tracing
core/gin/tracking
```

例外：

```txt
core/auth/adapter/gin
```

`auth` 已经有明确 adapter 分层，保持现状。

要求：

- 新增 Gin middleware、Gin helper、`*gin.Context` helper，必须放在 `core/gin/<module>`。
- `core/<module>` 根包不得 import `github.com/gin-gonic/gin` 或 `gin-contrib/*`。
- `core/<module>/facade` 不得 alias Gin middleware，facade 只负责 runtime/core 生命周期。
- 如果 Gin adapter 需要复用核心类型，可以在 `core/gin/<module>` 中声明 type alias，底层类型仍来自 `core/<module>`。
- 业务 HTTP 服务导入 `core/gin/*`；worker、crontab、queue handler、eventbus handler 不应导入 `core/gin/*`。
- 后续更换 Gin 或增加新 Web 框架时，按同样结构新增框架目录，例如 `core/fiber/*` 或 `core/nethttp/*`，不得把框架 API 塞回 `core/<module>`。

## 依赖检查

每次改动可选依赖或框架 adapter 后，至少验证 worker 依赖树：

```bash
go list -deps ./cmd/worker | rg "gin-gonic|gin-contrib|go-playground/validator|core/gin/|core/gin/session|core/storage|cos-go-sdk|alibaba-cloud-sdk-go|stripe-go|wechatpay|pkg/payment/(alipay|wechat|stripe|paypal)|pkg/sms/driver"
```

期望结果：

- worker 不使用 HTTP 时，上述命令无输出。
- worker 不使用 storage 时，不出现 `core/storage` 和对象存储 SDK。
- 未启用 OTel 时，不出现 OTel exporter/grpc 依赖。
- worker 不使用 sms/payment 时，不出现短信 driver、支付 provider 和官方 SDK。

打包验证：

```bash
go build -trimpath -ldflags "-s -w" -tags <enabled-tags> -o /tmp/sdkitgo-worker ./cmd/worker
```

记录字节数：

```bash
stat -f "%N %z" /tmp/sdkitgo-worker
```

## 新增能力 Checklist

- 是否放对了层级：`core` 抽象、`pkg` 实现、`core/gin` 框架适配、`app` 启动接线。
- 是否所有第三方 SDK 都被 build tag 包住。
- 是否没有在 `core` 根包 blank import 具体实现。
- 是否未编译时能返回清晰错误。
- 是否只在需要该能力的服务 provider 中注册。
- 是否跑过目标二进制的 `go list -deps`。
- 是否打包确认体积没有异常回升。
