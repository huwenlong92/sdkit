# execx 模块

## 作用

`pkg/execx` 是本地进程执行工具包，统一封装 `os/exec` 的常见使用方式：

- 普通命令执行
- Shell 命令执行
- stdout/stderr 一次性收集
- stdout/stderr 实时流式接收
- 长期进程启动、停止、kill、signal
- context 取消和超时
- 输出大小限制
- `\n`、`\r`、chunk 三类输出分割
- 最近日志环形缓存

模块只处理本地进程生命周期和输出传输，不处理业务权限、命令白名单、敏感信息脱敏、SSE 推送、日志落库或业务状态更新。

## 包路径

```go
import "github.com/huwenlong92/sdkit/pkg/execx"
```

## 对外 API

执行入口：

- `Run(ctx, name, args, opts...) (Result, error)`
- `RunOutput(ctx, name, args, opts...) (OutputResult, error)`
- `RunStream(ctx, name, args, sink, opts...) (Result, error)`
- `Start(ctx, name, args, opts...) (*Process, error)`
- `RunShell(ctx, script, opts...) (Result, error)`
- `RunShellOutput(ctx, script, opts...) (OutputResult, error)`
- `RunShellStream(ctx, script, sink, opts...) (Result, error)`
- `StartShell(ctx, script, opts...) (*Process, error)`

长期进程：

- `PID() int`
- `Done() <-chan Result`
- `Wait() (Result, error)`
- `Stop(ctx context.Context) error`
- `Kill() error`
- `Signal(sig os.Signal) error`
- `Running() bool`
- `RecentEvents() []Event`

事件：

```go
type Event struct {
	Stream    Stream
	Data      []byte
	Text      string
	Time      time.Time
	DecodeErr error
}
```

结果：

```go
type Result struct {
	Command    string
	Args       []string
	Dir        string
	PID        int
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
}
```

## Options

- `WithDir(dir string)`：设置工作目录。
- `WithEnv(env []string)`：追加环境变量，默认继承当前环境。
- `WithCleanEnv()`：不继承当前环境。
- `WithStdin(r io.Reader)`：设置 stdin。
- `WithInputString(s string)`：用字符串作为 stdin。
- `WithOutputLimit(n int64)`：设置 `RunOutput` 最大收集字节数，`n <= 0` 表示不限制。
- `WithLineBufferSize(n int)`：设置行扫描最大 token 大小。
- `WithSplitMode(mode SplitMode)`：设置输出分割方式。
- `WithMergeStderr()`：把 stderr 事件按 stdout 处理，兼容合并输出解析。
- `WithDecoder(decoder Decoder)` / `WithDecodeFunc(fn)`：自定义文本解码。
- `WithStrictDecode()`：解码失败时终止命令。
- `WithKillProcessGroup()`：Unix 下 kill 进程组。
- `WithStopTimeout(d time.Duration)`：设置默认停止超时。
- `WithWaitDelay(d time.Duration)`：设置 `exec.Cmd.WaitDelay`。
- `WithRingBuffer(n int)`：设置长期进程最近事件数量。
- `WithSink(sink Sink)`：给 `Start` 挂载额外事件接收器。
- `WithShell(name string, args ...string)`：覆盖 `RunShell*` / `StartShell` 使用的 shell。

## Shell 入口

`RunShell*` 和 `StartShell` 是显式 shell 入口，适合需要管道、重定向、通配符、变量展开、多命令组合的场景。

默认 shell：

- Unix/macOS：`sh -c`
- Windows：`cmd /C`

调用方可以使用 `WithShell("bash", "-lc")` 或 `WithShell("powershell", "-NoProfile", "-Command")` 覆盖默认 shell。

Shell 入口不做参数转义，也不承诺防注入。业务层必须保证脚本来源可信；用户输入参数应优先通过非 shell 的 `Run(ctx, name, args)` 传递。

## 输出分割

`SplitLine` 是默认模式，按 `\n` 分割，适合普通日志。

`SplitCRLF` 同时按 `\n` 和 `\r` 分割，适合 BaiduPCS-Go 这类使用回车刷新进度条的 CLI。

`SplitChunk` 按原始块推送，适合不稳定换行或需要最大程度保留原始输出的场景。`RunOutput` 内部固定使用 `SplitChunk`，避免丢失换行符。

## 进程清理

`Run`、`RunOutput`、`RunStream` 使用 `exec.CommandContext`，context 取消时会停止进程。调用 `WithKillProcessGroup()` 后，Unix 下会为命令设置独立进程组并 kill 整个进程组。

`Start` 面向长期进程，Unix 下默认启用进程组清理。`Stop` 先发送终止信号，调用方传入的 context 超时后再 kill。

## Sink

`Sink` 是实时输出接收接口：

```go
type Sink interface {
	WriteCommandEvent(ctx context.Context, event Event) error
}
```

内置实现：

- `SinkFunc`
- `MultiSink`
- `WriterSink`
- `RingSink`

`Sink` 同步执行，天然传导背压。业务层如果需要异步缓冲，可以在自己的 `Sink` 内实现队列策略。

## 错误约定

- 启动失败直接返回原始错误。
- 非 0 退出返回 `*ExitError`。
- context 取消和超时保留 `errors.Is` 可判断性。
- 输出超过限制返回 `ErrOutputLimitExceeded`。
- `Sink` 返回错误时，命令会被停止并返回该错误。

## 约束

- 不默认使用 shell。
- 使用 shell 必须显式调用 `RunShell*` / `StartShell`。
- 不解析业务输出。
- 不打印命令、参数或环境变量。
- 不做命令白名单和权限控制。
- stdout/stderr 并发读取，事件时间只能表示接收时间，不承诺完全复现终端全局顺序。
- 默认面向文本输出；二进制输出建议使用 `SplitChunk` 并读取 `Event.Data`。

## 更新记录

- 新增 `pkg/execx`，覆盖普通执行、Shell 执行、输出收集、实时输出和长期进程管理。
