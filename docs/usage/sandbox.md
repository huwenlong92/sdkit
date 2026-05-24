# Sandbox 使用文档

## 最小示例

```go
runner, err := sandbox.New(sandbox.Options{
	AllowedImages: []string{"python:3.12-slim"},
})
if err != nil {
	return err
}

result, err := runner.Run(ctx, &sandbox.RunRequest{
	SubmissionID: "submission-1",
	Language:     sandbox.LanguagePython,
	Files: []sandbox.File{
		{Path: "main.py", Content: []byte(`print("ok")`)},
	},
	Timeout:     5 * time.Second,
	MemoryBytes: 128 << 20,
	CPUNano:     500_000_000,
})
if err != nil {
	return err
}
fmt.Println(string(result.Stdout))
```

## 运行 demo

本机需要可用 Docker daemon：

```bash
go run ./examples/sandbox-demo
```

输出示例：

```text
container=<container-id> exit=0 timed_out=false
stdout=hello sdkit
```

## 语言 profile

Python：

```go
req := &sandbox.RunRequest{
	Language: sandbox.LanguagePython,
	Files: []sandbox.File{
		{Path: "main.py", Content: []byte(`print("hello")`)},
	},
}
```

Golang：

```go
req := &sandbox.RunRequest{
	Language: sandbox.LanguageGo,
	Files: []sandbox.File{
		{Path: "main.go", Content: []byte(`package main
import "fmt"
func main() { fmt.Println("hello") }
`)},
	},
}
```

CPP：

```go
req := &sandbox.RunRequest{
	Language: sandbox.LanguageCPP,
	Files: []sandbox.File{
		{Path: "main.cpp", Content: []byte(`#include <iostream>
int main() { std::cout << "hello\n"; }
`)},
	},
}
```

## 自定义镜像和命令

```go
req := &sandbox.RunRequest{
	Image:      "python:3.12-slim",
	RunCmd:     []string{"python", "main.py"},
	WorkingDir: "/workspace",
	Files: []sandbox.File{
		{Path: "main.py", Content: code},
	},
}
```

显式字段优先于 language profile 默认值。

## 私有镜像

```go
req := &sandbox.RunRequest{
	Image:      "registry.example.com/judge/python@sha256:...",
	PullPolicy: sandbox.PullIfNotPresent,
	RegistryAuth: &sandbox.RegistryAuth{
		ServerAddress: "registry.example.com",
		Username:      "user",
		Password:      "password",
	},
}
```

生产环境建议使用 digest 固定镜像，并通过 `AllowedImages` 开启白名单。

## Prometheus

```go
metrics, err := sandbox.NewPrometheusMetrics(nil)
if err != nil {
	return err
}

runner, err := sandbox.New(sandbox.Options{
	Metrics: metrics,
})
```

注册指标：

- `sandbox_run_total`
- `sandbox_run_duration`
- `sandbox_timeout_total`
- `sandbox_memory_usage`
- `sandbox_cpu_usage`

## Tracing

Sandbox 复用 `core/tracing`。只要上游 context 已有 span，`sandbox.run` 和 Docker lifecycle span 会挂在同一条 trace 下。

本地 Jaeger + OTel Collector：

```bash
docker compose -f deploy/sandbox/docker-compose.yaml up -d
```

Jaeger UI：

```text
http://127.0.0.1:16686
```

## 错误判断

```go
result, err := runner.Run(ctx, req)
switch {
case errors.Is(err, sandbox.ErrTimeout):
	// 超时
case errors.Is(err, sandbox.ErrCompileFailed):
	// 编译失败，查看 result.Stderr
case errors.Is(err, sandbox.ErrImageNotAllowed):
	// 镜像未在白名单中
}
```

即使返回 error，`result` 也可能包含 `ExitCode`、`Stdout`、`Stderr`、`ContainerID` 等诊断信息。
