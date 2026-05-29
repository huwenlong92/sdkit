# sdkit

`sdkit` 是一组 Go 后端核心能力包，提供配置、日志、数据库、Redis、缓存、队列、定时任务、鉴权、会话、限流、实时通信、文件存储、链路追踪等基础能力。

当前模块路径：

```go
github.com/huwenlong92/sdkit
```

## 定位

`sdkit` 只承载可复用的核心能力，不包含具体业务应用、HTTP 路由入口、数据库业务模型、迁移命令或部署配置。

典型使用方是 `sdkitgo`：

```go
import (
	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/queue"
	queueasynq "github.com/huwenlong92/sdkit/pkg/queue/asynq"
)
```

## 目录结构

```text
core/   核心能力抽象、运行时、facade、中间件和公共契约
pkg/    core 依赖的具体实现、驱动、适配器和工具包
docs/   模块设计文档和使用文档
```

`core` 和 `pkg` 是一个整体：

- `core` 定义稳定 API、能力注册、运行时约束和业务侧主要入口。
- `pkg` 提供 Redis、Asynq、filesystem、realtime transport 等具体实现。
- 两者存在依赖关系，不建议拆成多个 Go module。

## 本地开发

在使用方项目中通过 `replace` 引入本地仓库：

```go
require github.com/huwenlong92/sdkit v0.0.0

replace github.com/huwenlong92/sdkit => ../sdkit
```

安装依赖：

```bash
go mod tidy
```

运行测试：

```bash
go test ./...
```

## 常用模块

- `core/config`：配置加载与配置模型
- `core/logger`：日志初始化、上下文日志字段
- `core/database`：Gorm、PGX、分页和数据库 facade
- `core/redis`：Redis 客户端与 tracing hook
- `core/cache`：缓存抽象、memory/redis 实现接入
- `core/queue`：队列任务、producer、runtime、middleware、失败处理
- `core/crontab`：定时任务注册、执行、日志和调度抽象
- `core/auth`：鉴权、JWT、Session Guard、Gin 适配器
- `core/session`：基于 gin-contrib/sessions 的 Gin session 薄封装
- `core/ratelimit`：限流策略、store、Gin middleware
- `core/realtime`：实时通信抽象、网关、presence、publisher
- `core/security`：验证码、可配置风控、安全错误码和 Gin 安全适配
- `core/sandbox`：Docker 隔离代码执行 runtime、语言 profile、资源限制和观测接入
- `core/tracing`：OpenTelemetry 初始化、传播、span 工具
- `core/tracking`：业务追踪 ID 生成与透传
- `core/ginresponder`：Gin middleware 响应注入点
- `core/storage`：本地、OSS、COS、S3 文件存储实现
- `pkg/queue/asynq`：Asynq 队列驱动
- `pkg/eventbus/*`：memory、Redis、Redis Stream 事件总线实现
- `pkg/realtime/*`：WebSocket、SSE、gateway、transport 实现

## 文档

模块文档位于：

```text
docs/modules/<module>.md
docs/usage/<module>.md
```

- `docs/modules/*`：模块设计、对外 API、内部约束和更新记录
- `docs/usage/*`：初始化方式、配置方式和使用示例

## 兼容性说明

`sdkit` 当前从 `sdkitgo` 的 `core/` 和 `pkg/` 拆分而来。迁移后使用方应直接引用 `github.com/huwenlong92/sdkit/...`，不再依赖 `sdkitgo/core/...` 或 `sdkitgo/pkg/...`。

部分 tracing 名称仍保留历史值，例如 `sdkitgo/core/queue`，用于避免迁移时改变现有观测指标命名。
