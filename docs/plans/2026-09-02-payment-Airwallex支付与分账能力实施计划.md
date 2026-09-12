# sdkit payment Airwallex 支付与分账能力实施计划

状态：历史方案，已被普通支付接入方案替代；下方 Connected Accounts / Hosted Flow / Funds Split 设计不得直接执行。

2026-09-11：消费方已改为平台统一收款、自主分账。本轮实施范围见 [Airwallex 普通支付适配器](./2026-09-11-payment-Airwallex普通支付适配器.md)，不扩充可选渠道分账接口。

创建日期：2026-09-02

当前消费方前置：`/Users/huwenlong/data/sdkit/dreamip/dreamip/plans/current/2026-09-02-DreamIP三端支付与店铺结算本地验收纵切实施计划.md`

真实 Provider 计划：`/Users/huwenlong/data/sdkit/dreamip/dreamip/plans/deferred/2026-09-02-DreamIP-Airwallex支付与分账接入计划.md`

## 1. 目标

在不破坏现有微信、支付宝、Stripe、PayPal 和聚合支付能力的前提下，为 `payment` 增加可复用的“平台型支付”扩展点，并以 Airwallex 作为第一个实现：

- 普通收单继续使用现有统一创建、查询、关闭、退款、退款查询和支付 webhook；
- Connected Account 与 Hosted Onboarding 使用可选商户账户接口；
- Funds Split、manual release 和 reversal 使用可选分账接口；
- account/funds split 等非普通 PaymentEvent 回调使用可选平台事件接口；
- `core/payment` 只表达 provider-neutral 契约，Airwallex DTO、header、状态和签名留在 `pkg/payment/airwallex`；
- 全部实现先通过离线 contract test，消费方资料齐备后才连接 Sandbox。

## 2. 冻结的架构结论

### 2.1 Airwallex 放在哪里

Airwallex 既不独立于 payment 自建一套生命周期，也不整体塞进 core。目标分层为：

```text
业务应用
  -> core/payment
       ProviderAdapter（现有普通支付契约，保持不变）
       MerchantAccountAdapter（新增可选契约）
       FundsSplitAdapter（新增可选契约）
       PlatformNotifyAdapter（新增可选契约）
     -> pkg/payment/airwallex
          provider adapter / capability / 状态和错误映射
        -> pkg/payment/airwallex/httpapi
             官方 HTTP API / token cache / webhook signature / DTO
```

`core/payment` 不 import Airwallex 包。业务应用只把已编译的 Airwallex adapter 注册到现有 Registry。

### 2.2 不扩大必选 ProviderAdapter

GitNexus 对直接修改 `core/payment.ProviderAdapter` 的结果为 CRITICAL：当前有五十多个 adapter、router、facade 和测试直接或间接受影响。实现阶段保持现有必选方法不变，不给微信、支付宝、Stripe、PayPal 强加开户和分账方法。

新增能力遵循 sdkit 已有“小型可选接口 + capability/type assertion”模式。Service 在调用前同时检查能力声明和接口实现；两者不一致时返回 `ErrUnsupportedCapability`，不能 panic。

### 2.3 普通支付与分账共存

Airwallex adapter 必须先实现完整普通支付能力。是否执行 Funds Split 由业务领域在支付成功后显式调用，不在 adapter 内看到“支付成功”就自动分账。

原因：

- 同一 Provider 同时存在不分账的官方直营订单和需要分账的商户订单；
- 分账金额来自业务冻结的费率、退款、税费和结算规则，sdkit 无权计算；
- manual release 的时点依赖合同、交付、争议期和财务审批，不能由通用 adapter 决定；
- adapter 自动分账会把网络调用和领域事务绑在一起，无法可靠封闭“Provider 已接收、本地未落库”的窗口。

## 3. core/payment 公共契约草案

### 3.1 Provider、Channel 与 Capability

实施时增加：

```go
ProviderAirwallex
ChannelAirwallexHPP
```

Airwallex HPP 创建结果优先映射为现有 `ActionClientToken`：`Token` 放短时 `client_secret`，`Params` 放前端初始化所需且可公开的 PaymentIntent ID、环境和 locale。若最终选择 Airwallex 全页托管跳转，则使用现有 `ActionRedirectURL`，不得再发明 Airwallex 专用 action。

`Capabilities` 计划增加：

```go
SupportsMerchantAccounts
SupportsHostedOnboarding
SupportsFundsSplit
SupportsManualSplitRelease
SupportsSplitReversal
SupportsPlatformNotify
```

具体命名在实现前结合 Go 兼容性再确认；布尔声明只用于发现能力，实际调用仍必须通过可选接口断言。

### 3.2 商户账户契约

公共模型只表达：

- 业务 `MerchantKey`；
- 外部账户引用；
- Business/Individual 等通用主体类型；
- 主联系人和条款同意的最小输入；
- provider-neutral 账户状态和能力；
- Hosted Onboarding 的 flow 引用、短时 redirect action 和错误摘要。

候选接口：

```go
type MerchantAccountAdapter interface {
	CreateMerchantAccount(context.Context, CreateMerchantAccountRequest) (*MerchantAccount, error)
	QueryMerchantAccount(context.Context, QueryMerchantAccountRequest) (*MerchantAccount, error)
	CreateHostedOnboarding(context.Context, HostedOnboardingRequest) (*HostedOnboardingSession, error)
	AuthorizeHostedOnboarding(context.Context, AuthorizeHostedOnboardingRequest) (*HostedOnboardingSession, error)
}
```

Hosted Flow 的 URL 是短时 action，不是长期账户属性。core 不承担 KYB 字段全集，不保存证件或 RFI 材料，也不假设所有 Provider 都支持托管开户。

### 3.3 分账契约

公共模型至少包含：

- 稳定 `RequestID`；
- `MerchantKey` 和 Provider/Channel；
- source 类型与外部 source ID；
- destination 外部账户引用；
- 最小货币单位 `Money`；
- `AutoRelease`；
- provider-neutral 状态、Provider split ID、时间和最小 Extra；
- reversal 的独立 request ID、金额和 Provider reversal ID。

候选接口：

```go
type FundsSplitAdapter interface {
	CreateFundsSplit(context.Context, CreateFundsSplitRequest) (*FundsSplit, error)
	QueryFundsSplit(context.Context, QueryFundsSplitRequest) (*FundsSplit, error)
	ReleaseFundsSplit(context.Context, ReleaseFundsSplitRequest) (*FundsSplit, error)
	ReverseFundsSplit(context.Context, ReverseFundsSplitRequest) (*FundsSplitReversal, error)
	QueryFundsSplitReversal(context.Context, QueryFundsSplitReversalRequest) (*FundsSplitReversal, error)
}
```

core 只校验必填引用、正金额、币种、Provider/Channel、capability 和响应非空。它不校验某订单可分多少钱、不决定 destination、不计算平台佣金、不决定 release 时间。

### 3.4 平台 webhook 契约

现有 `NotifyResult.PaymentEvent` 继续承接 payment/refund 事件。account、funds split、reversal 等事件不伪装成 PaymentEvent，计划新增独立 `PlatformEvent`/`PlatformNotifyResult` 和可选解析接口。

通用事件至少保留：

- Provider Event ID；
- Provider、MerchantKey、resource type、resource ID、event type；
- provider-neutral status；
- occurred at；
- 已验签标志和 ACK；
- 原始字节或 provider payload 只作为调用方可选择加密持久化的数据，不进入普通日志。

sdkit 只负责验签和解析，不负责 Event ID 数据库幂等、乱序聚合、Outbox、Worker 或 ACK 前的持久化；这些属于消费方。

## 4. pkg/payment/airwallex 设计

### 4.1 包结构

```text
pkg/payment/airwallex/
  doc.go
  types.go
  adapter.go
  mapping.go
  httpapi/
    doc.go
    client.go
    authentication.go
    payment_intents.go
    refunds.go
    accounts.go
    hosted_flows.go
    funds_splits.go
    webhooks.go
    types.go
    errors.go

tests/pkg/payment/airwallex/
tests/pkg/payment/airwallex/httpapi/
```

provider 使用 build tag `sdkit_payment_airwallex`；未开启 tag 时保留可安全 import 的空包文档，与现有 provider 规则一致。

### 4.2 Client 生命周期

Airwallex 平台账户使用一组应用级 Client ID/API Key 管理多个 Connected Account，不为每个店铺创建一套凭据或完整 client。

实现要求：

- `httpapi.Client` 持有共享 authenticator；
- access token 按 `expires_at` 提前安全窗口刷新，使用 mutex/singleflight 防止并发登录风暴；
- 401 只允许一次受控 token 失效与重试；非幂等请求重试必须复用调用方传入的稳定 request ID；
- adapter 的 AccountResolver/ClientLoader 只把业务 MerchantKey 解析成服务器可信的账户上下文；
- `x-on-behalf-of` 只能来自可信 resolver，不接收浏览器透传值；
- 平台普通收款、代表 Connected Account 操作和 Funds Split 使用明确的请求上下文，不能依赖上一次请求遗留 header。

### 4.3 金额与幂等

core 继续使用最小货币单位 `int64`。HTTP 层按现有 CurrencyMeta 精确转换为 Airwallex major unit decimal，反向解析也使用 decimal/string 或 `json.Number`，禁止 float。

下列命令必须要求稳定 v4 UUID `RequestID`，不能在网络调用内部临时生成：

- Create PaymentIntent；
- Create Refund；
- Create FundsSplit；
- Release FundsSplit；
- Create FundsSplit Reversal；
- Create Connected Account/Hosted Flow 中 Airwallex 支持幂等的操作。

如果现有普通支付请求没有独立 RequestID，实施时优先增加向后兼容的可选 `RequestID` 字段；Airwallex adapter 在必需操作中验证它，不能把可能不满足 UUID 规则的 OutTradeNo 偷换成 request ID。

### 4.4 状态与错误映射

Airwallex 原始状态不进入 core 常量。adapter 建立显式映射并对未知新状态安全降级：

- PaymentIntent 映射到 `pending / requires_action / processing / authorized / succeeded / failed / closed`；
- Refund 使用当前 webhook/API 版本的 `SETTLED` 语义映射到 core `RefundSucceeded`，同时保留脱敏原始状态；
- Account 映射到 `created / onboarding / under_review / action_required / active / suspended / rejected / closed / unknown`；
- FundsSplit/Reversal 使用独立通用状态，不能复用 PaymentStatus 或 RefundStatus。

错误使用支持 `errors.Is/As` 的 typed error，至少区分 validation、unauthenticated、permission denied、account suspended、capability unavailable、rate limited、conflict/duplicate、not found、timeout、temporary upstream 和 unknown outcome，并保存可脱敏的 Provider request ID。

## 5. Webhook 安全与可靠性

实现遵循 Airwallex 当前官方协议：

1. 从 header 读取 `x-timestamp` 和 `x-signature`；
2. 在任何 JSON 解析或重序列化之前，拼接原始 timestamp 字符串和原始 body；
3. 使用该 webhook URL 对应 secret 做 HMAC-SHA256；
4. 使用 constant-time compare；
5. 校验可配置 timestamp tolerance；
6. 验签后才解析 Event ID、name、account_id 和 `data.object`；
7. 返回供业务 handler 使用的 ACK，不在 adapter 内启动 goroutine 或写数据库。

测试覆盖：正确签名、错误签名、错误 secret、缺 header、过期/未来 timestamp、格式化后 body 不同、未知事件、重复 Event ID fixture、account/payment/refund/split/reversal 事件和大 body 上限。

## 6. 配置边界

sdkit 只定义 Config 输入，不读取业务项目的环境变量、数据库或 Secret Manager。消费方负责提供：

```text
Environment / BaseURL
ClientID
APIKey
PlatformAccountID（如账户配置要求）
WebhookSecret
WebhookTolerance
HostedFlowTemplateOpenID
APIVersion
WebhookVersion（用于消费方订阅和 fixture）
HTTPClient
AccountResolver / ClientLoader
```

真实 secret 不进入 sdkit 测试、示例、日志或文档。文档只使用明显的占位符。debug 事件默认不得输出 Authorization、x-api-key、client_secret、webhook secret、authorization code、完整 account ID、完整请求/响应 body 或 KYB 数据。

## 7. 测试与兼容门禁

### 7.1 core contract test

- 现有 ProviderAdapter 不变，全部原 provider 测试继续编译；
- adapter 未声明/未实现可选能力时返回统一 unsupported error；
- Service 对 nil context、空 request ID、非法金额/币种、nil response 做稳定校验；
- Airwallex HPP action 满足现有 action contract；
- 新 provider/channel 不改变现有 StaticChannelSelector、Registry 和 facade 行为。

### 7.2 Airwallex HTTP contract test

- 全部使用 `httptest.Server`，断言 method、path、header、JSON、decimal 和响应映射；
- 并发请求只触发一次 token login；token 过期和一次 401 刷新可预测；
- `x-on-behalf-of` 不跨请求泄漏；
- 相同 request ID 的超时重试保持请求体一致；
- 4xx/5xx/429/timeout/invalid JSON/body too large 映射正确；
- webhook fixture 只使用测试 secret，不依赖真实账户或网络。

### 7.3 全仓门禁

```bash
go test ./tests/core/payment/... ./tests/pkg/payment/airwallex/...
go test -tags sdkit_payment_airwallex ./tests/core/payment/... ./tests/pkg/payment/...
go test ./...
```

实现后使用 goimports，只格式化本轮 Go 文件；更新 `docs/modules/payment.md` 和 `docs/usage/payment.md`；运行 GitNexus `detect_changes` 并核对受影响范围。不 commit、不发布 tag，等待用户确认。

## 8. 实施顺序

DreamIP 先用服务端 `local_acceptance` 完成 Buyer 收银台/支付账单、Owner 店铺结算和 Admin 监控纵切。该实现属于消费方领域验证，不进入 sdkit，也不伪装 Airwallex adapter。纵切验收后，再从实际调用面提炼下述最小公共契约。

### Phase 0：消费方契约回收与 Provider 合同冻结（暂缓，0/6）

- [ ] DreamIP 三端本地支付与店铺结算纵切完成自动化和浏览器验收；
- [ ] 用户确认 core 可选能力与 provider 分层；
- [ ] Airwallex 确认 Payments、Connected Accounts、Hosted Flow、Funds Split、manual release/reversal 和 webhook 版本；
- [ ] 冻结官方 API path、字段、状态、错误、事件清单和 Sandbox base URL；
- [ ] 对拟修改的现有 symbol 分别运行 GitNexus impact 并报告 HIGH/CRITICAL 风险；
- [ ] 冻结 public API 命名，决定普通请求 `RequestID` 的向后兼容添加方式。

### Phase 1：core 可选契约（0/5）

- [ ] 新增 provider/channel/capability；
- [ ] 新增 MerchantAccount、FundsSplit、Reversal、PlatformEvent 类型；
- [ ] 新增三个可选接口和 Service/default/facade 方法；
- [ ] 补齐 validation、fill default、state 和 contract test；
- [ ] 确认现有 ProviderAdapter 和所有 provider 无需改方法签名。

### Phase 2：Airwallex 离线实现（0/7）

- [ ] 建立 build-tag package 和 adapter；
- [ ] 实现认证和共享 token cache；
- [ ] 实现 PaymentIntent/Refund；
- [ ] 实现 Connected Account/Hosted Flow；
- [ ] 实现 FundsSplit/Release/Reversal；
- [ ] 实现 webhook 验签和事件解析；
- [ ] 完成 httptest contract、并发、错误、脱敏和状态映射测试。

### Phase 3：文档与消费方交付（0/4）

- [ ] 更新 `docs/modules/payment.md` 的稳定架构、接口、状态和约束；
- [ ] 更新 `docs/usage/payment.md` 的 build tag、初始化、配置占位符和调用示例；
- [ ] 提供无真实 secret 的 fake/httptest 示例；
- [ ] 由 DreamIP 通过本地依赖替换现有 `local_acceptance` 执行边界，完成 fake adapter 回归验收。

### Phase 4：真实 Sandbox 联调（消费方资料齐备后，0/5）

- [ ] 使用消费方 Secret 注入，不改 sdkit 仓库；
- [ ] 对照真实响应修正 DTO、状态和错误映射，不把业务规则回灌 core；
- [ ] 完成普通支付、开户、分账、release、reversal、退款和 webhook 矩阵；
- [ ] 保留脱敏 fixture 并回归全部离线测试；
- [ ] 消费方验收后由用户决定 sdkit 版本发布。

## 9. 当前不做

- 当前不创建任何 Go 文件或 build tag；
- 当前不把 DreamIP 的 `local_acceptance` 迁入 sdkit；它只是消费方开发验收 Provider；
- 当前不要求 Client ID、API Key、Webhook Secret 或 Template Open ID；
- sdkit 不保存商户账户、支付单、分账单、回调幂等或审计数据；
- sdkit 不提供 KYB 页面、数据库 migration、Admin 审核、FeePolicy、Settlement、Payout 或 Reconciliation；
- 不把 FundsSplit 自动挂在 CreatePayment 或 ParseNotify 后执行；
- 不用 Airwallex 接口形状反向污染微信/支付宝未来的二级商户和分账适配；
- 不在没有官方合同和离线测试的情况下连接生产环境。
