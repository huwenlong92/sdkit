# request 使用

`pkg/request` 封装标准库 `net/http`，用于项目内工具调用、第三方 API 调用和内部服务调用。它只处理 HTTP 请求生命周期，不绑定业务鉴权、日志系统、链路追踪或具体接口协议。

## 创建客户端

```go
client, err := request.NewClient(
	request.WithBaseURL("https://api.example.com/v1"),
	request.WithTimeout(10*time.Second),
	request.WithDefaultHeader("User-Agent", "sdkit"),
	request.WithDefaultQuery("source", "worker"),
)
if err != nil {
	return err
}
```

所有请求入口都必须传入 `context.Context`：

```go
resp, err := client.Get(ctx, "/users/1")
if err != nil {
	return err
}
fmt.Println(resp.String())
```

包级快捷方法使用默认 client，适合一次性绝对 URL 请求：

```go
resp, err := request.Get(ctx, "https://api.example.com/ping")
```

## JSON 请求与响应

```go
var out struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

resp, err := client.Post(
	ctx,
	"/users",
	request.WithJSON(map[string]string{"name": "alice"}),
	request.WithDecodeJSON(&out),
)
if err != nil {
	return err
}
_ = resp.StatusCode
```

也可以手动解析：

```go
resp, err := client.Get(ctx, "/users/1")
if err != nil {
	return err
}
if err := resp.DecodeJSON(&out); err != nil {
	return err
}
```

## Query、Header 与认证

```go
resp, err := client.Get(
	ctx,
	"/search",
	request.WithQuery("q", "sdkit"),
	request.WithHeader("X-Request-ID", requestID),
	request.WithBearerToken(token),
)
```

Basic Auth：

```go
resp, err := client.Get(ctx, "/profile", request.WithBasicAuth(user, password))
```

## Form 与原始 Body

```go
resp, err := client.Post(
	ctx,
	"/login",
	request.WithFormMap(map[string]string{
		"username": "alice",
		"password": "secret",
	}),
)
```

原始 body：

```go
resp, err := client.Put(
	ctx,
	"/objects/1",
	request.WithString("text/plain", "content"),
)
```

`WithBody(contentType, reader)` 适合流式输入，但 reader 默认不可重放。需要配合 retry 的请求优先使用 `WithJSON`、`WithBytes`、`WithString`、`WithForm` 或 `WithMultipart`。

## Multipart

```go
file, err := os.Open("example.txt")
if err != nil {
	return err
}
defer file.Close()

resp, err := client.Post(
	ctx,
	"/upload",
	request.WithMultipart(
		request.FieldPart("name", "example"),
		request.FilePart("file", "example.txt", "text/plain", file),
	),
)
```

文件打开和关闭由调用方负责，避免隐藏文件生命周期。

## 状态码错误

默认只接受 2xx。非 2xx 返回 `*request.StatusError`，并保留响应 body：

```go
resp, err := client.Get(ctx, "/private")
if err != nil {
	var statusErr *request.StatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode() == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized: %s", string(statusErr.Body()))
	}
	return err
}
_ = resp
```

单次请求可以覆盖期望状态码：

```go
resp, err := client.Delete(ctx, "/jobs/1", request.WithExpectedStatus(http.StatusNoContent))
```

## 响应大小限制

默认最多读取 10MB 响应 body，避免工具调用读爆内存。可以在 client 或单次请求覆盖：

```go
client, err := request.NewClient(request.WithMaxBodyBytes(2 << 20))
resp, err := client.Get(ctx, "/large", request.WithRequestMaxBodyBytes(512<<10))
```

`n <= 0` 表示不限制。超过限制返回可用 `errors.Is(err, request.ErrBodyTooLarge)` 判断的错误，`Response.Body` 保留已读取的截断内容。

## Retry

默认不重试。需要时显式开启：

```go
retry := request.DefaultRetryConfig()
retry.MaxAttempts = 3
retry.WaitMin = 100 * time.Millisecond
retry.WaitMax = time.Second

client, err := request.NewClient(request.WithRetry(retry))
```

默认 retry 只覆盖幂等方法：

- `GET`
- `HEAD`
- `OPTIONS`
- `PUT`
- `DELETE`

默认可重试状态码：

- `408`
- `425`
- `429`
- `500`
- `502`
- `503`
- `504`

`POST` 不会默认重试。确实需要重试 `POST` 时，调用方必须显式设置 `RetryConfig.Methods`，并确认接口具备幂等能力。

## Stream

普通 `Do/Get/Post` 会读取并关闭响应 body。下载、SSE 或长连接使用 `Stream`：

```go
stream, err := client.Stream(ctx, http.MethodGet, "/events")
if err != nil {
	return err
}
defer stream.Close()

_, err = io.Copy(dst, stream.Response.Body)
```

`WithRequestTimeout` 不作用于 `Stream` 的生命周期。流式请求应由调用方传入带取消或超时的 context，并在不再读取时调用 `Close`。

## Hooks

Hooks 用于日志、指标、trace、签名和脱敏，不直接绑定具体日志库：

```go
client, err := request.NewClient(
	request.WithBeforeHook(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-Signature", sign(req))
		return nil
	}),
	request.WithAfterHook(func(ctx context.Context, resp *request.Response, err error) error {
		recordMetric(resp, err)
		return nil
	}),
)
```

Hook 返回错误会中断请求或作为请求错误返回。

## 自定义 HTTP 能力

复用已有 `http.Client`：

```go
client, err := request.NewClient(request.WithHTTPClient(existingClient))
```

自定义 transport、proxy 或 TLS：

```go
client, err := request.NewClient(
	request.WithProxy("http://127.0.0.1:7890"),
	request.WithTLSConfig(tlsConfig),
)
```

模块不会默认跳过 TLS 校验。
