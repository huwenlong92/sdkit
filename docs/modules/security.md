# Security 模块设计

`core/security` 是安全与风控基础能力中心，负责人机验证适配、短期状态、黑名单、审计、风控编排和 Gin 中间件。加解密、签名、密码哈希、随机 token、请求指纹等纯工具位于 `pkg/security`。

不负责登录态、JWT、Session、RBAC、菜单权限和普通访问日志；这些继续由 `core/auth`、`core/gin/session`、`core/casbin`、`core/accesslog` 等模块维护。

## 包结构

- `pkg/security/crypto`：AES-GCM、HMAC-SHA256、SHA256、随机字节和 OpenAPI 签名 payload。
- `pkg/security/password`：bcrypt 密码哈希与校验。
- `pkg/security/token`：随机 token、nonce、数字验证码。
- `pkg/security/fingerprint`：从 HTTP 请求提取 IP、UA、DeviceID，并生成请求指纹。
- `captcha`：验证码 Provider 接口、Manager、内存 Provider、base64 图片验证码 Provider、noop Provider。
- `state`：短期 TTL 状态存储接口，提供 memory 和 Redis 实现。
- `blacklist`：黑名单接口、内存实现和基于 state 的 Redis-like 实现。
- `audit`：安全事件接口、内存 Writer 和 GORM model 定义。
- `risk`：旧版 Context、Result、Action、Checker、Manager，主要服务内置 checker 和历史 Gin 中间件。
- `risk2`：事件、规则、名单、频率、命中决策的纯引擎抽象；只定义 Store 接口和评估流程，不依赖 Gin、Gorm 或业务表。
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
- `security:risk2:freq:*`

DB 用于长期审计、黑白名单主数据、规则配置和设备记录。表模型统一放在根目录 `models/security.go`：

- `SecurityEvent`
- `SecurityBlacklist`
- `SecurityWhitelist`
- `SecurityRule`
- `SecurityDevice`

risk2 的职责边界：

- core 只定义事件、规则、名单、命中决策、Store、Counter 和 Gin 适配。
- core 不定义后台菜单、业务场景目录、业务表结构、Gorm model、SQL 查询、响应结构。
- 应用层负责实现 Store，读取业务表中的场景、事件定义、黑白名单、频率规则，并写入事件日志与命中记录。
- 应用层负责决定接入 Redis、cache、DB，以及是否对配置做短 TTL 缓存。
- 应用层负责决定事件日志是否同步写入、异步写入、批量写入或进入队列。

## 核心接口

`risk.Checker` 是风控规则入口：

```go
type Checker interface {
    Name() string
    Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error)
}
```

`risk.Manager` 串行执行 checker，合并风险分、动作和原因，并通过 `audit.Writer` 写安全事件。

`risk2.Engine` 面向后台可配置风控，核心只依赖应用实现的 `risk2.Store`。Gorm、SQL、业务表模型实现必须留在应用层；core 不持有业务数据库结构：

```go
type Store interface {
    LoadScene(ctx context.Context, service, scene string) (*Scene, error)
    MatchList(ctx context.Context, event Event, listType string) (*ListRule, error)
    ListFrequencyRules(ctx context.Context, event Event) ([]FrequencyRule, error)
    CountEvents(ctx context.Context, query EventCountQuery) (int64, error)
    SaveDecision(ctx context.Context, event Event, decision *Decision) error
}
```

Store 的 Gorm、SQL、业务表模型实现必须留在应用层；core 不持有业务数据库结构。

`ListRule` 和 `FrequencyRule` 可以携带响应配置：`ResponseCode`、`ResponseMessage`、`ResponseAction`、`ResponsePayload`、`HTTPStatus`。这些字段不参与规则是否命中，只在命中后传递给 `HitDecision` 和最终 `Decision`，由 Gin 适配优先用于拦截响应。`ResponseAction` 推荐使用内置常量：

- `ResponseActionNone`
- `ResponseActionToast`
- `ResponseActionCaptcha`
- `ResponseActionAppeal`
- `ResponseActionRetryLater`
- `ResponseActionContact`

多个规则同时命中时，Engine 按 `action` 严重程度选择主规则；`action` 相同取 `score` 更高的规则；再相同保留先命中规则。主规则的响应配置会写入最终 `Decision`。

`risk2.Counter` 用于把频率统计从事件表 `COUNT` 下沉到短期计数能力：

```go
type Counter interface {
    Incr(ctx context.Context, key CounterKey) (int64, error)
}
```

`NewRedisCounter` 是生产推荐实现，内部使用 Redis Lua + ZSET 做滑动窗口计数。一次 `Incr` 会在同一个脚本中完成：

1. 清理窗口外的 ZSET 成员。
2. 写入当前事件。
3. 设置 ZSET 与序列 key 的 TTL。
4. 返回窗口内当前次数。

Redis key 使用 `security:risk2:freq:` 前缀，业务维度会 hash 到 key 中，维度包括 service、scene、event、target_type、target_value、window_seconds。ZSET key 和序列 key 使用同一个 Redis Cluster hash tag，保证脚本在集群模式下仍访问同一个 slot。

`NewCacheCounter` 保留为测试或单进程场景的可选实现，语义是固定窗口；生产多实例频率规则不应使用内存 cache 计数。若 counter 不可用或返回错误，Engine 会回退到 Store 的 `CountEvents`，保持可用性。

运行时配置可通过 `risk2.NewCachedStore` 包一层短 TTL 缓存，缓存内容包括场景、黑白名单匹配结果和频率规则。该缓存只缓存 Store 查询结果，不引入业务配置结构；后台修改配置后依赖短 TTL 自然生效，应用也可以按需自行删除相关 key。

事件日志和命中记录可通过 `risk2.Logger` 异步写入。core 定义 `DecisionWriter`，应用层实现批量落库：

```go
type DecisionWriter interface {
    WriteDecisionBatch(ctx context.Context, records []DecisionRecord) error
}
```

Engine 在配置 `WithLogger(logger)` 后会优先把事件决策推入 logger 队列；推入成功后不等待 DB 写入，降低请求路径延迟。若没有 logger 或队列已满，Engine 会同步调用 Store 的 `SaveDecision`，保证高压时仍尽量保留审计日志。异步写入失败只能记录日志，不能再影响已经返回的请求。

配置缓存和频率计数不要混用：

- 配置缓存：使用 `core/cache`，适合短 TTL 缓存场景、名单、规则等读多写少数据。
- 频率计数：生产默认使用 Redis counter，保证多实例共享统计窗口。
- DB：保留事件日志、命中记录、配置主数据，也作为 Redis 不可用时的统计回退。
- 事件日志写入：生产建议使用 `risk2.Logger` + 应用层 `DecisionWriter` 批量写入；强一致审计场景可以不配置 logger，继续走 Store 同步写入。

`captcha.Provider` 用于接入图片验证码、滑块验证码、Turnstile、reCAPTCHA 或业务自研 Provider。Provider 负责生成 challenge 并校验答案，Manager 按 `Kind` 路由到具体 Provider。

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
- `core/ratelimit` 保持独立，security 通过 `RateLimitChecker` 适配 `pkg/ratelimit.Limiter`，不依赖 ratelimit 的 Runtime Capability 或 Gin middleware。
- `core/accesslog` 继续记录普通访问日志；security 只写安全事件。
- security Gin middleware 通过 `core/gin/responder` 支持应用层注入统一响应结构；未注入时使用 core 默认 JSON fallback。
- `core/gin/security/risk2` 只做 Gin 到 `risk2.Event` 的适配、middleware 和统一拦截响应；不依赖应用层 response 包。

## 更新记录

- 2026-05-29：`risk2` 的名单规则和频率规则新增响应配置字段，支持规则级错误码、提示文案、前台动作、动作参数和 HTTP 状态；最终响应配置跟随主命中规则。
- 2026-05-28：`risk2` 新增通用 Counter 和短 TTL CachedStore，支持应用侧把频率统计切到 Redis 滑动窗口，并缓存场景、名单和频率规则配置。
- 2026-05-28：新增 `risk2` 事件/规则/名单/频率评估引擎抽象和 Gin 适配，应用层通过 Store 实现持久化。
- 2026-05-21：纯安全工具下沉到 `pkg/security`；captcha Provider 改为生成 challenge + 校验答案的多类型接口，预留图片验证码和滑块验证码扩展。
- 新增 `core/security` 基础能力、7 个内置业务场景、Gin 中间件和测试。
