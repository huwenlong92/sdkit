# CAS 使用

`core/cas` 提供显式构造的 CAS v2/v3 客户端。不依赖 Gin、数据库或 runtime，不保存账号、身份或 Session。

```go
client, err := cas.NewClient(cas.Config{
    Enabled: true,
    ServerURL: "https://cas.example.edu/authserver",
    CallbackURL: "https://portal.example.edu/auth/cas/callback",
    Protocol: cas.ProtocolV3,
    RequestTimeout: 10 * time.Second,
})
if err != nil { return err }
loginURL, err := client.LoginURL("/student")
if err != nil { return err }
// 浏览器访问 loginURL；收到 ticket 后，使用同一个 redirect 重建完全一致的 service。
serviceURL, err := client.ServiceURL("/student")
if err != nil { return err }
principal, err := client.ValidateTicket(ctx, serviceURL, ticket)
if err != nil { return err }
// 调用方根据 principal.Subject 查询自己的用户身份并创建本地登录态。
```

## 配置与边界

- `protocol` 默认 `2`，使用 `/serviceValidate`；`3` 使用 `/p3/serviceValidate`。`server_url` 包含学校的路径前缀，如 `/authserver`。
- 主体默认来自 CAS `<user>`。学校明确使用 `uid` 等属性时设置 `subject_attribute`；配置后该属性缺失会拒绝认证，不静默改用另一个标识。
- `Principal` 保留主体、姓名、邮箱与重复属性。姓名识别 `cn/displayName/name`，邮箱识别 `mail/email`，属性名不区分大小写；学校身份类型映射由调用方处理。
- `ServiceURL` 在配置的回调上加入经过站内检查的 `redirect` 参数。授权与校验必须传相同的 redirect。应用还应实现并校验与浏览器会话绑定的一次性 state；本客户端不提供完整登录 CSRF 防护。
- `LogoutURL(returnURL)` 仅生成 CAS `/logout?service=...` 地址；returnURL 必须来自可信应用配置。本地 Session 必须由应用先清除。学校是否接受退出后跳转取决于服务端策略。
- 请求透传 context，最长受 `request_timeout`（默认 10 秒）约束，响应不超过 1 MiB。即使注入 HTTP client，ticket 校验也不跟随 3xx，且不会修改注入对象。
- 生产必须使用 HTTPS。HTTP 仅为本地测试或遗留服务器兼容；不要关闭 TLS 校验。
- 不记录 ticket、完整认证响应和个人属性。对外提示使用稳定业务文案，不直接回显错误或属性。

## 验证

`go test ./tests/core/cas -race`

协议参考：[Apereo CAS 规范](https://apereo.github.io/cas/development/protocol/CAS-Protocol-Specification.html)。
