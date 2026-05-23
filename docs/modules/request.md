# request 模块

## 作用

`pkg/request` 是公开 HTTP 客户端工具包，统一封装项目内工具调用和外部 API 调用的基础能力：

- context 透传
- client 与 transport 复用
- BaseURL、默认 header、默认 query
- JSON、form、raw body、multipart 请求体
- 普通响应读取与流式响应分离
- 响应 body 大小限制
- 结构化状态码错误
- 可选 retry
- before/after hooks
- 自定义 `http.Client`、transport、proxy、TLS

模块只负责 HTTP 协议层封装，不处理业务协议、业务鉴权、接口签名规则、日志脱敏策略或链路追踪 SDK 绑定。

## 包路径

```go
import "github.com/huwenlong92/sdkit/pkg/request"
```

## 对外 API

客户端：

- `NewClient(opts ...ClientOption) (*Client, error)`
- `DefaultClient() *Client`

包级快捷入口：

- `Do(ctx, method, target, opts...)`
- `Get(ctx, target, opts...)`
- `Post(ctx, target, opts...)`
- `Put(ctx, target, opts...)`
- `Patch(ctx, target, opts...)`
- `Delete(ctx, target, opts...)`
- `Head(ctx, target, opts...)`
- `Options(ctx, target, opts...)`
- `StreamRequest(ctx, method, target, opts...)`

Client 方法：

- `Do`
- `Get`
- `Post`
- `Put`
- `Patch`
- `Delete`
- `Head`
- `Options`
- `Stream`

响应：

```go
type Response struct {
	Request    *http.Request
	Raw        *http.Response
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
}
```

响应方法：

- `Bytes() []byte`
- `String() string`
- `DecodeJSON(v any) error`

流式响应：

```go
type Stream struct {
	Request  *http.Request
	Response *http.Response
}
```

- `Close() error`

## Client Options

- `WithHTTPClient(client *http.Client)`
- `WithTransport(transport http.RoundTripper)`
- `WithTimeout(timeout time.Duration)`
- `WithBaseURL(rawURL string)`
- `WithDefaultHeader(name, value string)`
- `WithDefaultHeaders(headers http.Header)`
- `WithDefaultQuery(name, value string)`
- `WithMaxBodyBytes(n int64)`
- `WithRetry(cfg RetryConfig)`
- `WithHook(hook Hook)`
- `WithBeforeHook(fn)`
- `WithAfterHook(fn)`
- `WithStatusValidator(fn)`
- `WithProxy(proxyURL string)`
- `WithTLSConfig(tlsConfig *tls.Config)`

默认值：

- `DefaultTimeout = 30s`
- `DefaultMaxBodyBytes = 10MB`
- 状态码验证：`200 <= status <= 299`
- retry：关闭，`MaxAttempts = 1`

## Request Options

- `WithQuery(name, value string)`
- `WithQueryValues(values url.Values)`
- `WithHeader(name, value string)`
- `WithHeaders(headers http.Header)`
- `WithBearerToken(token string)`
- `WithBasicAuth(username, password string)`
- `WithJSON(v any)`
- `WithForm(values url.Values)`
- `WithFormMap(values map[string]string)`
- `WithBody(contentType string, body io.Reader)`
- `WithBytes(contentType string, body []byte)`
- `WithString(contentType, body string)`
- `WithMultipart(parts ...MultipartPart)`
- `WithDecodeJSON(v any)`
- `WithExpectedStatus(codes ...int)`
- `WithRequestStatusValidator(fn)`
- `WithRequestMaxBodyBytes(n int64)`
- `WithRequestRetry(cfg RetryConfig)`
- `WithRequestTimeout(timeout time.Duration)`
- `WithRequestHook(hook Hook)`

同一个请求只能设置一种 body。重复设置 body 会返回 `ErrBodyAlreadySet`。

## URL 拼接规则

`WithBaseURL("https://api.example.com/v1")` 与 `"/users"` 会拼成：

```text
https://api.example.com/v1/users
```

如果请求目标是绝对 URL，则不使用 BaseURL。

BaseURL 自身 query、目标 URL query、默认 query、单次请求 query 会合并。相同 key 允许多个值。

## 错误约定

基础错误：

- `ErrNilContext`
- `ErrNilHTTPClient`
- `ErrNilTransport`
- `ErrNilBody`
- `ErrBodyAlreadySet`
- `ErrBodyNotReplayable`
- `ErrBodyTooLarge`
- `ErrNilMultipartPart`

请求错误：

```go
type RequestError struct {
	Method string
	URL    string
	Err    error
}
```

`RequestError` 实现 `Unwrap`，调用方可以用 `errors.Is` 判断 context 取消、超时、body 过大等底层错误。

状态码错误：

```go
type StatusError struct {
	Response *Response
}
```

非期望状态码返回 `*StatusError`，并保留响应 body、header 和 status code。

JSON 解析错误：

```go
type DecodeError struct {
	Err  error
	Body []byte
}
```

## Retry 约束

默认不重试。`DefaultRetryConfig` 提供保守配置：

- 最大 3 次
- 指数退避
- 重试网络错误
- 仅重试幂等方法
- 重试常见临时状态码

不会重试：

- context canceled
- context deadline exceeded
- `ErrBodyTooLarge`
- `ErrBodyNotReplayable`
- 默认配置下的 `POST`

需要重试非幂等请求时，调用方必须显式配置 `RetryConfig.Methods`，并保证业务幂等。

## Stream 约束

普通请求会读取并关闭 body。`Stream` 返回后 body 生命周期交给调用方，调用方必须调用 `Close`。

`Stream` 不读取成功响应 body；非期望状态码时会读取受限大小的错误 body 并关闭连接。

`WithRequestTimeout` 不控制 stream 生命周期。流式请求必须通过传入的 context 控制取消。

## Hooks 约束

Hooks 分为：

- `BeforeRequest`
- `AfterResponse`

Hook 同步执行。`BeforeRequest` 返回错误时不会发送请求。`AfterResponse` 返回错误时会作为请求错误返回。Hook 适合实现：

- 请求签名
- 日志
- 指标
- trace 注入
- header 注入

Hook 内不要启动不可回收 goroutine。

## 设计约束

- 不默认跳过 TLS 校验。
- 不默认 retry。
- 不默认打印请求或响应。
- 不绑定 logger、tracing、metrics SDK。
- 不隐藏文件打开和关闭。
- 不在普通响应模型中处理 SSE 或下载。
- 不吞非 2xx 响应 body。
- 不把业务错误码解析为模块错误。

## 更新记录

- 新增 `pkg/request`，提供统一 HTTP client 封装、结构化响应、状态码错误、retry、stream、multipart、hooks 和文档。
