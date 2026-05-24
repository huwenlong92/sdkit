# Sandbox 模块方案

## 作用

`core/sandbox` 提供统一、安全、可观测的代码执行 runtime，用于 AI 算法竞赛、在线判题和受控代码执行场景。

业务层只依赖：

```go
type Sandbox interface {
	Run(ctx context.Context, req *RunRequest) (*RunResult, error)
}
```

Docker SDK 封装在 `core/sandbox/internal/docker` 内部，业务层不能直接 import Docker backend。

## 目录

```text
core/sandbox/
  sandbox.go
  types.go
  files.go
  logs.go
  metrics.go
  prometheus.go
  internal/docker/
    runtime.go
    container.go
    image.go
    logs.go
    stats.go
    cleanup.go
  security/
    security.go
    seccomp.go
  tracing/
    tracing.go
  profile/
    python.go
    golang.go
    cpp.go
```

`internal/docker` 是当前唯一 backend。Kubernetes Job、containerd、Firecracker VM 只保留 `Backend` 接口边界，暂不实现。

## 对外 API

请求字段：

- `SubmissionID`
- `Language`
- `Image`
- `Cmd` / `CompileCmd` / `RunCmd`
- `Env`
- `Files`
- `Timeout` / `CompileTimeout` / `RunTimeout`
- `CPUNano`
- `MemoryBytes`
- `PidsLimit`
- `Ulimits`
- `WorkingDir`
- `Stdin`
- `NetworkDisabled`
- `ReadonlyRootfs`
- `RegistryAuth`
- `PullPolicy`

结果字段：

- `ExitCode`
- `Stdout`
- `Stderr`
- `StdoutTruncated`
- `StderrTruncated`
- `Duration`
- `MemoryUsed`
- `CPUUsed`
- `TimedOut`
- `ContainerID`
- `Phase`

## Backend 边界

`core/sandbox` 内部通过 `Backend` 抽象隔离执行 backend：

```go
type Backend interface {
	Run(ctx context.Context, spec *RunSpec) (*RunResult, error)
}
```

当前 `sandbox.New` 默认装配 Docker backend。未来新增 backend 时，只新增内部适配，不改变业务侧 `Sandbox` 接口。

后续接 Kubernetes、containerd 或 Firecracker 时按这个顺序做：

1. 在 `core/sandbox/internal/<backend>` 新增 backend 包，例如 `internal/kubernetes`。
2. 实现 `Backend.Run(ctx, spec)`，把 `RunSpec` 映射为 Job、containerd task 或 VM execution。
3. 复用 `core/sandbox` 已有 request normalization、文件注入校验、profile、tracing、metrics 和错误类型。
4. 新增构造函数或 option 选择 backend，例如 `NewKubernetes(opts)`，业务侧仍只持有 `Sandbox`。
5. 不把 Kubernetes、containerd、Firecracker SDK 类型暴露到 `RunRequest`、`RunResult` 或 `Options` 公共字段里。

公共层保持稳定，backend 特有配置只放在对应 backend 的 options 或内部 adapter 中。这样替换 backend 时，调用方不需要改 `Run(ctx, req)`。

## Docker 生命周期

Docker backend 负责：

1. image whitelist 校验。
2. 按 pull policy 检查或拉取镜像。
3. 创建 compile container。
4. 启动并等待 compile container。
5. 收集 compile stdout/stderr 和 stats。
6. 删除 compile container。
7. 创建 run container。
8. 写入 stdin。
9. 启动并等待 run container。
10. 超时 kill。
11. 收集 run stdout/stderr 和 stats。
12. 删除 run container、volume、临时 workspace。

container cleanup 使用独立 cleanup context，不复用已经 canceled 的运行 context。

## 安全默认值

默认开启：

- `NetworkMode=none`
- `ReadonlyRootfs=true`
- `CapDrop=ALL`
- `NoNewPrivileges`
- `/tmp`、`/run`、`/var/tmp` tmpfs
- Docker daemon 默认 seccomp profile
- 非 root 用户 `65534:65534`
- `PidsLimit`
- CPU / Memory 限制
- `MemorySwap=MemoryBytes`

workspace 单独 bind mount 到 `/workspace`，rootfs readonly 不影响工作目录写入。

## 文件注入

`Files` 写入运行前临时 workspace，再 bind mount 到容器。

限制：

- 禁止空路径。
- 禁止绝对路径。
- 禁止 `..` 逃逸。
- 禁止重复路径。
- 限制文件数量、单文件大小和总大小。

## Language Profile

内置 profile：

- `python`：`python:3.12-slim`，运行 `python main.py`。
- `golang`：`golang:1.22-alpine`，先 `go build`，再运行 `/workspace/main`。
- `cpp`：`gcc:13`，先 `g++ -O2 -std=c++17`，再运行 `/workspace/main`。

请求显式传入的 `Image`、`CompileCmd`、`RunCmd`、`WorkingDir` 优先于 profile 默认值。

## Observability

Tracing 复用 `core/tracing`：

- 不在 sandbox 内创建 TracerProvider。
- 不重复实现 traceparent 传播。
- span 使用 `sdkitgo/core/sandbox` tracer。
- span：`sandbox.run`、`sandbox.create`、`sandbox.start`、`sandbox.wait`、`sandbox.logs`、`sandbox.cleanup`。
- attributes：`container.id`、`image`、`submission.id`、`timeout`、`memory.limit`、`exit.code`、`timed_out`、`memory.used`、`cpu.used`。

日志复用 `core/logger.WithContext(ctx, log)`，额外追加：

- `container_id`
- `submission_id`
- `phase`
- `exit_code`
- `timed_out`

stdout/stderr 收集复用 `pkg/execx` 的 `Sink` / `Event` / `Stream` 语义，不单独定义一套日志事件模型。

Prometheus metrics 通过 `MetricsRecorder` 注入，默认 noop。`NewPrometheusMetrics` 注册：

- `sandbox_run_total`
- `sandbox_run_duration`
- `sandbox_timeout_total`
- `sandbox_memory_usage`
- `sandbox_cpu_usage`

## 内部约束

- 业务层禁止 import `core/sandbox/internal/docker`。
- 不允许业务层直接调用 Docker SDK。
- 所有操作必须透传 context。
- 所有错误必须 wrap。
- cleanup 必须幂等。
- stats/logs goroutine 必须随 context 或 body close 退出。
- 能复用仓库已有模块时优先复用，例如 tracing、logger、execx stream 语义。
- import 能使用原包名时不写别名；只有命名冲突或语义不清时才使用别名。
- 不为未来 backend 提前实现复杂逻辑，只保留接口边界。
- seccomp 默认使用 Docker daemon profile；只有提供经过验证的 profile 时才覆盖，避免手写过窄 profile 导致 runtime 启动失败。

## 更新记录

- 2026-05-24：新增 `core/sandbox` Docker backend、language profile、tracing、Prometheus recorder、文件注入、安全默认值和 demo。
