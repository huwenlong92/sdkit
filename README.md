# sdkit

`sdkit` 是一组可复用的 Go 后端基础能力包，面向 `sdkitgo` 和其他 Go 后端项目。它将配置、日志、数据访问、异步任务、安全和通信等能力集中维护，供消费方按需接入。

模块路径：`github.com/huwenlong92/sdkit`。

## 项目定位

- 提供公共契约、具体驱动和运行时生命周期管理。
- 支持按模块直接使用，也支持通过 Runtime capability 统一初始化和回收资源。
- 业务模型、业务路由、服务组合与部署配置由消费方维护。

`core/` 和 `pkg/` 同属一个 Go module，两层均可被外部项目导入。通常从 `core/` 的公共 API 接入框架能力，从 `pkg/` 选择具体驱动或使用独立工具。

## 主要能力

| 能力 | 模块入口 |
| --- | --- |
| 配置加载与拆分配置 | [core/config](core/config) |
| 日志、链路追踪与请求标识 | [core/logger](core/logger)、[core/tracing](core/tracing)、[core/tracking](core/tracking)、[core/requestid](core/requestid) |
| GORM、PGX、Redis 与缓存 | [core/database](core/database)、[core/redis](core/redis)、[core/cache](core/cache) |
| 队列、定时任务与事件总线 | [core/queue](core/queue)、[core/crontab](core/crontab)、[core/eventbus](core/eventbus) |
| 鉴权、权限、安全与限流 | [core/auth](core/auth)、[core/casbin](core/casbin)、[core/security](core/security)、[core/ratelimit](core/ratelimit) |
| 实时通信与 Gin 接入 | [core/realtime](core/realtime)、[core/gin](core/gin) |
| 文件存储、邮件与短信 | [core/storage](core/storage)、[core/email](core/email)、[core/sms](core/sms) |
| 支付能力与渠道适配 | [core/payment](core/payment)、[pkg/payment](pkg/payment) |
| 系统探针与隔离代码执行 | [core/sysprobe](core/sysprobe)、[core/sandbox](core/sandbox) |
| 应用运行时与进度报告 | [core/runtime](core/runtime)、[core/reporter](core/reporter) |
| HTTP 请求、ID 与媒体工具 | [pkg/request](pkg/request)、[pkg/id](pkg/id)、[pkg/hashid](pkg/hashid)、[pkg/media](pkg/media) |
| SDIngest Go SDK | [pkg/sdingest](pkg/sdingest) |

## 环境与安装

当前 `go.mod` 要求 Go **1.25.10 或更高版本**。

在消费方项目中添加依赖：

```bash
go get github.com/huwenlong92/sdkit
```

外部服务按所用模块准备：例如数据库、Redis、NATS、对象存储或 Docker。下面的最小示例只使用配置和日志，无需启动这些服务。

## 最小接入示例

在消费方项目中创建 `config.yaml`：

```yaml
app:
  name: demo
  mode: dev
log:
  level: info
```

创建 `main.go`，由应用定义配置结构并处理初始化错误：

```go
package main

import (
	"fmt"
	"os"

	"github.com/huwenlong92/sdkit/core/config"
	"github.com/huwenlong92/sdkit/core/logger"
	"go.uber.org/zap"
)

type Config struct {
	App struct {
		Name string `mapstructure:"name"`
		Mode string `mapstructure:"mode"`
	} `mapstructure:"app"`
	Log struct {
		Level string `mapstructure:"level"`
	} `mapstructure:"log"`
}

func main() {
	var cfg Config
	if err := config.Load("config.yaml", &cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := logger.Init(cfg.App.Name, cfg.Log.Level, cfg.App.Mode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("application initialized", zap.String("app", cfg.App.Name))
}
```

在该项目目录运行：

```bash
go run .
```

`dev` 模式下日志同时输出到控制台，文件日志默认写入 `logs/`。更完整的应用可以通过各模块的 `facade` 接入 `core/runtime`，统一管理启动、依赖和停止过程。

## 目录结构

```text
core/       公共契约、运行时、生命周期、框架入口与中间件
pkg/        具体实现、驱动、适配器、服务 SDK 与独立工具
tests/      按模块组织的测试
examples/   示例应用
```

隔离代码执行示例位于 [examples/sandbox-demo](examples/sandbox-demo)。

## 本地开发与验证

在消费方项目中使用本地仓库进行联调：

```bash
go mod edit -replace github.com/huwenlong92/sdkit=../sdkit
go mod tidy
```

`../sdkit` 应替换为实际的本地仓库路径。

在 sdkit 仓库根目录运行全量测试：

```bash
go test ./...
```

开发时可先验证受影响的模块，例如：

```bash
go test ./tests/core/config ./tests/core/logger/...
```

新增测试统一放在根目录 `tests/` 下，按模块组织。涉及外部服务的测试需要按对应模块准备运行环境。
