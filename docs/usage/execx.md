# execx 使用

`pkg/execx` 封装本地命令执行，提供普通执行、一次性输出收集、实时输出流和长期进程管理。它不依赖 Gin、SSE、logger 或业务权限系统，业务层负责命令白名单、参数校验、脱敏和前端推送。

## 普通执行

```go
result, err := execx.Run(ctx, "git", []string{"status", "--short"})
if err != nil {
	return err
}
_ = result.ExitCode
```

`execx` 不默认走 shell。需要 shell 语义时，调用方必须显式传入 `sh -c` 或平台对应命令，避免命令注入边界不清晰。

## Shell 执行

需要管道、重定向、通配符、变量展开或多命令组合时，可以使用 Shell 入口：

```go
output, err := execx.RunShellOutput(ctx, "ps aux | grep xray")
if err != nil {
	return err
}
fmt.Println(string(output.Stdout))
```

Shell 入口包括：

- `RunShell`
- `RunShellOutput`
- `RunShellStream`
- `StartShell`

默认 shell：

- Unix/macOS：`sh -c`
- Windows：`cmd /C`

需要 bash、PowerShell 或其他 shell 时，用 `WithShell` 覆盖：

```go
output, err := execx.RunShellOutput(
	ctx,
	"source ~/.profile && go test ./...",
	execx.WithShell("bash", "-lc"),
)
```

`RunShell*` 只适合可信脚本、内部固定模板和运维命令。用户输入不要直接拼进 shell 字符串；用户参数优先使用 `Run(ctx, name, args)` 的非 shell 形式。

## 收集输出

```go
output, err := execx.RunOutput(ctx, "git", []string{"status", "--short"})
if err != nil {
	return err
}
fmt.Println(string(output.Stdout))
```

`RunOutput` 默认最多收集 10MB 输出，超过限制返回 `ErrOutputLimitExceeded`。需要调整时：

```go
output, err := execx.RunOutput(
	ctx,
	"git",
	[]string{"log", "--oneline"},
	execx.WithOutputLimit(2<<20),
)
```

兼容 `CombinedOutput` 场景时可以合并 stderr：

```go
output, err := execx.RunOutput(
	ctx,
	"BaiduPCS-Go",
	args,
	execx.WithMergeStderr(),
)
```

## 实时日志

```go
sink := execx.SinkFunc(func(ctx context.Context, event execx.Event) error {
	switch event.Stream {
	case execx.StreamStdout:
		fmt.Println(event.Text)
	case execx.StreamStderr:
		fmt.Println("ERR:", event.Text)
	}
	return nil
})

_, err := execx.RunStream(ctx, "go", []string{"test", "./..."}, sink)
```

Shell 场景的实时输出：

```go
_, err := execx.RunShellStream(
	ctx,
	"go test ./... 2>&1 | tee test.log",
	sink,
)
```

事件包含：

- `Stream`：stdout 或 stderr
- `Data`：原始 bytes
- `Text`：解码后的文本
- `Time`：接收时间
- `DecodeErr`：解码错误，默认不终止命令

前端 SSE、日志落库、进度解析都应该在业务层用 `Sink` 实现。`Sink` 返回错误时，`execx` 会停止命令并返回该错误，适合前端断开连接后的清理。

## 进度条输出

部分 CLI 使用 `\r` 刷新进度条，可以使用 `SplitCRLF`：

```go
_, err := execx.RunStream(
	ctx,
	"BaiduPCS-Go",
	args,
	sink,
	execx.WithSplitMode(execx.SplitCRLF),
	execx.WithMergeStderr(),
)
```

如果输出不是按行文本，可以使用 `SplitChunk` 按原始块推送。

## stdin

交互确认可以通过 stdin 输入：

```go
_, err := execx.RunOutput(
	ctx,
	"BaiduPCS-Go",
	[]string{"logout"},
	execx.WithInputString("y\n"),
)
```

也可以使用 `WithStdin(reader)` 传入自定义输入流。

## 工作目录与环境变量

```go
_, err := execx.Run(
	ctx,
	"npm",
	[]string{"run", "build"},
	execx.WithDir("/data/app"),
	execx.WithEnv([]string{"NODE_ENV=production"}),
)
```

默认继承当前进程环境，并追加 `WithEnv`。需要隔离环境时：

```go
_, err := execx.Run(
	ctx,
	"env",
	nil,
	execx.WithCleanEnv(),
	execx.WithEnv([]string{"PATH=/usr/bin:/bin"}),
)
```

## 文本解码

默认按 UTF-8 字符串处理。需要兼容 GB18030 等输出时，可以传入解码函数：

```go
decoder := simplifiedchinese.GB18030.NewDecoder()
_, err := execx.RunStream(ctx, command, args, sink, execx.WithDecodeFunc(func(data []byte) (string, error) {
	out, err := decoder.Bytes(data)
	return string(out), err
}))
```

如果希望解码失败直接终止命令，增加 `WithStrictDecode()`。

## 长期进程

`Start` 用于 xray、worker、sidecar 这类长期进程：

```go
p, err := execx.Start(
	ctx,
	"xray",
	[]string{"-c", "/etc/xray/config.json"},
	execx.WithRingBuffer(200),
)
if err != nil {
	return err
}

// 查看最近输出
events := p.RecentEvents()

// 服务退出时停止进程
stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
_ = p.Stop(stopCtx)
```

`Stop` 会先尝试终止进程，超时后再 kill。Unix 下长期进程默认启用进程组清理，避免子进程残留。

## 错误处理

```go
result, err := execx.Run(ctx, command, args)
if err != nil {
	var exitErr *execx.ExitError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.As(err, &exitErr):
		return fmt.Errorf("command exit code %d: %w", exitErr.Result.ExitCode, err)
	default:
		return err
	}
}
_ = result
```

错误语义：

- 命令不存在或权限不足：返回启动错误。
- 非 0 退出：返回 `*execx.ExitError`，可读取 `Result.ExitCode`。
- context 取消或超时：支持 `errors.Is` 判断。
- 输出超过限制：返回 `ErrOutputLimitExceeded`。
- `Sink` 返回错误：停止命令并返回该错误。
