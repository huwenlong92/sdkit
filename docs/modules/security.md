# Security 模块设计

`core/security` 是安全与风控基础能力中心，负责加解密、签名、密码哈希、随机 token、人机验证适配、请求指纹、黑名单、审计、风控编排和 Gin 中间件。

不负责登录态、JWT、Session、RBAC、菜单权限和普通访问日志；这些继续由 `core/auth`、`core/session`、`core/casbin`、`core/accesslog` 等模块维护。

## 包结构

- `crypto`：AES-GCM、HMAC-SHA256、SHA256、随机字节和 OpenAPI 签名 payload。
- `password`：bcrypt 密码哈希与校验。
- `token`：随机 token、nonce、数字验证码。
- `captcha`：验证码 Provider 接口、内存 Provider、noop Provider。
- `fingerprint`：从 HTTP 请求提取 IP、UA、DeviceID，并生成请求指纹。
- `state`：短期 TTL 状态存储接口，提供 memory 和 Redis 实现。
- `blacklist`：黑名单接口、内存实现和基于 state 的 Redis-like 实现。
- `audit`：安全事件接口、内存 Writer 和 GORM model 定义。
- `risk`：Context、Result、Action、Checker、Manager。
- `risk/checkers`：内置登录防爆破、短信防轰炸、OpenAPI 签名、后台防扫描、评论防刷、注册防机器、异常登录、ratelimit 适配。
- `middleware`：签名校验、风控检查、安全响应头。

## Redis 与 DB 分工

Redis 或 `state.Store` 用于短期状态、计数、TTL、临时封禁、防重放：

- `security:nonce:{nonce}`
- `security:login_fail:uid:{uid}`
- `security:login_fail:ip:{ip}`
- `security:sms:*`
- `security:block:*`
- `security:register:*`
- `security:comment:*`
- `security:last_login:*`
- `security:device_seen:*`

DB 用于长期审计、黑白名单主数据、规则配置和设备记录。表模型统一放在根目录 `models/security.go`：

- `SecurityEvent`
- `SecurityBlacklist`
- `SecurityWhitelist`
- `SecurityRule`
- `SecurityDevice`

## 核心接口

`risk.Checker` 是风控规则入口：

```go
type Checker interface {
    Name() string
    Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error)
}
```

`risk.Manager` 串行执行 checker，合并风险分、动作和原因，并通过 `audit.Writer` 写安全事件。

`captcha.Provider` 用于接入图形验证码、Turnstile、reCAPTCHA 或业务自研 Provider。

`state.Store` 抽象 Redis 常用操作，测试默认使用 memory，生产可使用 `state.NewRedisStore(redisClient)`。

## 内置场景

- 登录防爆破：同 UID/IP 失败计数，达到阈值要求验证码或封禁。
- 短信验证码防轰炸：手机号冷却、每日限制、IP 小时限制、验证码 TTL。
- OpenAPI 签名与 nonce 防重放：HMAC-SHA256、timestamp 偏移、nonce SetNX、防签名失败封禁 IP。
- 后台 Admin 防扫描：后台登录失败计数和常见扫描路径识别。
- 评论/提交防刷：UID/IP/设备窗口计数和敏感词审核。
- 注册防机器：IP/设备注册阈值、验证码和封禁。
- 异常登录检测：新设备、地区变化、UA 变化累积分，达到阈值要求二次验证。

## 与其他模块关系

- `core/auth` 保持独立，security 只产出风险结果，不签发登录态或 JWT。
- `core/ratelimit` 保持独立，security 通过 `RateLimitChecker` 适配已有 `ratelimit.Limiter`，不重复替换限流模块。
- `core/accesslog` 继续记录普通访问日志；security 只写安全事件。
- `core/response` 被 security middleware 用于统一响应结构。

## 更新记录

- 新增 `core/security` 基础能力、7 个内置业务场景、Gin 中间件和测试。
