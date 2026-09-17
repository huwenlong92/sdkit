# Payment 使用

`payment` 提供统一支付能力。业务侧只面向 `core/payment` 的统一 API；微信、支付宝、Stripe、PayPal 等平台差异放在 `pkg/payment/*` adapter 里。

## Build Tag

支付 provider 按需编译。只打开当前二进制实际需要的 provider tag：

| provider | build tag |
| --- | --- |
| alipay | `sdkit_payment_alipay` |
| wechat | `sdkit_payment_wechat` |
| stripe | `sdkit_payment_stripe` |
| paypal | `sdkit_payment_paypal` |
| airwallex | `sdkit_payment_airwallex` |

示例：

```bash
go build -tags sdkit_payment_alipay,sdkit_payment_wechat ./cmd/server
```

按需 import。下面示例同时展示四类 provider，真实项目只保留已启用 provider 的 import：

```go
import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
	paymentfacade "github.com/huwenlong92/sdkit/core/payment/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
	paymentalipay "github.com/huwenlong92/sdkit/pkg/payment/alipay"
	"github.com/huwenlong92/sdkit/pkg/payment/alipay/openapi"
	paymentpaypal "github.com/huwenlong92/sdkit/pkg/payment/paypal"
	"github.com/huwenlong92/sdkit/pkg/payment/paypal/ordersapi"
	paymentstripe "github.com/huwenlong92/sdkit/pkg/payment/stripe"
	"github.com/huwenlong92/sdkit/pkg/payment/stripe/stripego"
	paymentwechat "github.com/huwenlong92/sdkit/pkg/payment/wechat"
	"github.com/huwenlong92/sdkit/pkg/payment/wechat/apiv3"
)
```

## 推荐用法

推荐路径：

```text
bootstrap 注册 payment facade
  -> ConfigLoader 加载 payment.channels
  -> WithSetup 注册 provider adapter

handler 调 payment.CreatePayment
  -> 传 MerchantKey，例如 school_a_wechat_mini
  -> core 自动解析 provider/channel/真实 merchant_key
  -> provider adapter 每次请求创建 client
  -> 请求结束后 cleanup
```

业务调用时不需要拿 `svc`：

```go
resp, err := payment.CreatePayment(ctx, payment.CreatePaymentRequest{
	MerchantKey: "school_a_wechat_mini",
	OrderID:     "order_1001",
	OutTradeNo:  "pay_202605260001",
	Subject:     "会员年卡",
	Pricing:     payment.CNY(19900),
	Extra: map[string]any{
		"openid": openID,
	},
})
if err != nil {
	return err
}
```

返回结果会带实际支付通道：

```go
_ = resp.Provider    // wechat
_ = resp.Channel     // wechat_mini_program
_ = resp.MerchantKey // school_a_wechat
_ = resp.Action      // 前端下一步动作
```

## 配置文件

`core/payment/facade.Config` 只负责加载业务 channel key 到真实支付通道的映射，不保存平台密钥。

```yaml
payment:
  channels:
    - key: school_a_wechat_mini
      provider: wechat
      channel: wechat_mini_program
      merchant_key: school_a_wechat

    - key: school_a_alipay_page
      provider: alipay
      channel: alipay_page
      merchant_key: school_a_alipay

    - key: school_a_stripe
      provider: stripe
      channel: stripe_checkout
      merchant_key: school_a_stripe_us
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `key` | 业务调用时传入的 `MerchantKey` |
| `provider` | 支付服务商，例如 `wechat`、`alipay` |
| `channel` | 支付产品形态，例如 `wechat_mini_program` |
| `merchant_key` | provider adapter 加载真实平台配置时使用的 key |

平台配置，例如微信 appid、mchid、证书，支付宝 appid、私钥，Stripe key，PayPal client id，属于业务配置或数据库配置，由 adapter 的 `ClientLoader` 按 `merchant_key` 加载。

## Bootstrap 初始化

在 `sdkitgo/bootstrap/global_capabilities.go` 这种集中注册能力的地方接入：

```go
capabilities = append(capabilities, paymentfacade.Use(
	paymentfacade.WithInternal(),
	paymentfacade.WithConfigLoader(loadPaymentConfig),
	paymentfacade.WithSetup(registerPaymentAdapters),
))
```

配置 loader 示例：

```go
func loadPaymentConfig(*runtime.App) (paymentfacade.Config, error) {
	cfg, err := boot.requireConfig()
	if err != nil {
		return paymentfacade.Config{}, err
	}
	return cfg.Payment, nil
}
```

Payment facade 不读取 `core/config.V`，也不知道业务项目的配置文件结构。即使业务项目使用 `payment.channels` 作为配置节点，也必须在业务侧通过 `WithConfig` 或 `WithConfigLoader` 显式传入。

## 注册 Adapter

默认使用动态 client 模式。adapter 初始化时只注册能力和 loader，不创建每个商户的 SDK client。

```go
func registerPaymentAdapters(app *runtime.App, registry *payment.Registry) error {
	wechatAdapter, err := paymentwechat.NewAdapter(paymentwechat.Config{
		ClientLoader: paymentwechat.ClientLoaderFunc(func(ctx context.Context, merchantKey string) (paymentwechat.Client, paymentwechat.ClientCleanup, error) {
			cfg, err := loadWechatConfig(ctx, merchantKey)
			if err != nil {
				return nil, nil, err
			}
			client, err := apiv3.NewClient(cfg)
			if err != nil {
				return nil, nil, err
			}
			return client, func() error { return nil }, nil
		}),
		SupportsQuery:       true,
		SupportsClose:       true,
		SupportsRefund:      true,
		SupportsQueryRefund: true,
	})
	if err != nil {
		return err
	}
	if err := registry.Register(wechatAdapter); err != nil {
		return err
	}

	alipayAdapter, err := paymentalipay.NewAdapter(paymentalipay.Config{
		ClientLoader: paymentalipay.ClientLoaderFunc(func(ctx context.Context, merchantKey string) (paymentalipay.Client, paymentalipay.ClientCleanup, error) {
			cfg, err := loadAlipayConfig(ctx, merchantKey)
			if err != nil {
				return nil, nil, err
			}
			client, err := openapi.NewClient(cfg)
			if err != nil {
				return nil, nil, err
			}
			return client, func() error { return nil }, nil
		}),
		SupportsQuery:       true,
		SupportsRefund:      true,
		SupportsQueryRefund: true,
	})
	if err != nil {
		return err
	}
	if err := registry.Register(alipayAdapter); err != nil {
		return err
	}

	return nil
}
```

`ClientLoader` 每次请求都会执行：

```text
Load config by merchant_key -> New official client -> Call provider -> cleanup
```

如果 client 有临时资源，放进 cleanup：

```go
return client, func() error {
	return closer.Close()
}, nil
```

cleanup 返回错误不会覆盖支付主流程错误。

## 静态 Client

静态 client 仅用于单商户、固定配置或测试。必须显式设置 `ClientModeStatic`：

```go
wechatClient, err := apiv3.NewClient(cfg)
if err != nil {
	return err
}
wechatAdapter, err := paymentwechat.NewAdapter(paymentwechat.Config{
	ClientMode: paymentwechat.ClientModeStatic,
	Client:     wechatClient,
})
```

如果不设置 `ClientMode`，默认就是动态模式，必须提供 `ClientLoader`。

## Reload 渠道映射

如果运行中新增、删除或修改了 `payment.channels`，handler 里重新读取完整配置，然后调用：

```go
func ReloadPaymentChannelsHandler(w http.ResponseWriter, r *http.Request) {
	bindings, err := loadPaymentChannelBindings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := payment.ReloadChannels(bindings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`ReloadChannels` 只替换 `key -> provider/channel/merchant_key` 映射。平台密钥、证书、appid 等由 `ClientLoader` 每次调用时读取；如果你的 loader 读的是数据库或最新配置缓存，平台配置变更会在下一次支付调用生效。

## 创建支付

人民币主路径：

```go
resp, err := payment.CreatePayment(ctx, payment.CreatePaymentRequest{
	MerchantKey: "school_a_wechat_mini",
	OrderID:     "order_1001",
	OutTradeNo:  "pay_202605260001",
	Subject:     "会员年卡",
	Pricing:     payment.CNY(19900),
	NotifyURL:   "https://example.com/api/pay/notify/wechat",
	Extra: map[string]any{
		"openid": userOpenID,
	},
})
```

跨币种：

```go
resp, err := payment.CreatePayment(ctx, payment.CreatePaymentRequest{
	MerchantKey: "school_a_stripe",
	OutTradeNo:  "stripe_1001",
	Subject:     "International Plan",
	ReturnURL:   "https://example.com/pay/success",
	Pricing: payment.PaymentPricing{
		PayAmount:      payment.Money{Amount: 1000, Currency: "EUR"},
		SettleCurrency: "CNY",
		ExchangeRate: &payment.ExchangeRateSnapshot{
			FromCurrency: "EUR",
			ToCurrency:   "CNY",
			Rate:         "7.8000",
			Source:       "internal",
			QuotedAt:     time.Now().Unix(),
		},
	},
})
```

前端按 `Action.Type` 处理：

```go
switch resp.Action.Type {
case payment.ActionSDKParams:
	return json.NewEncoder(w).Encode(resp.Action.Params)
case payment.ActionRedirectURL:
	http.Redirect(w, r, resp.Action.URL, http.StatusFound)
case payment.ActionHTMLForm:
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = w.Write([]byte(resp.Action.HTML))
	return err
case payment.ActionQRCode:
	return json.NewEncoder(w).Encode(map[string]any{"qr_code": resp.Action.URL})
case payment.ActionClientToken:
	return json.NewEncoder(w).Encode(map[string]any{"client_secret": resp.Action.Token})
default:
	return json.NewEncoder(w).Encode(resp)
}
```

## 查询、关闭、退款

查询支付：

```go
resp, err := payment.QueryPayment(ctx, payment.QueryPaymentRequest{
	MerchantKey: "school_a_wechat_mini",
	OutTradeNo:  "pay_202605260001",
})
```

关闭支付：

```go
err := payment.ClosePayment(ctx, payment.ClosePaymentRequest{
	MerchantKey: "school_a_wechat_mini",
	OutTradeNo:  "pay_202605260001",
})
```

退款：

```go
resp, err := payment.Refund(ctx, payment.RefundRequest{
	MerchantKey: "school_a_wechat_mini",
	OutTradeNo:  "pay_202605260001",
	OutRefundNo: "refund_202605260001",
	Amount: payment.RefundAmount{
		Refund: payment.Money{Amount: 9900},
	},
	Extra: map[string]any{
		"total_amount": int64(19900),
	},
})
```

查询退款：

```go
resp, err := payment.QueryRefund(ctx, payment.QueryRefundRequest{
	MerchantKey:  "school_a_wechat_mini",
	OutTradeNo:   "pay_202605260001",
	OutRefundNo:  "refund_202605260001",
})
```

## Provider 参数

### WeChat

`loadWechatConfig(ctx, merchantKey)` 通常返回 `apiv3.Config`：

| 字段 | 说明 |
| --- | --- |
| `AppID` | 微信开放平台、公众号或小程序 app id |
| `MerchantID` | 微信支付商户号 |
| `MerchantSerialNo` | 商户 API 证书序列号 |
| `PrivateKey` | 商户 API 证书私钥 |
| `APIv3Key` | 微信支付 API v3 密钥 |
| `NotifyURL` | 支付结果回调地址 |

请求额外参数：

| Channel | 参数 |
| --- | --- |
| `wechat_mini_program` | `Extra["openid"]` 必填 |
| `wechat_h5` | `Extra["client_ip"]` 必填 |
| `wechat_app` | 无额外必填 |
| `wechat_native` | 无额外必填 |

微信退款需要 `Extra["total_amount"]`，单位为分。

### Alipay

`loadAlipayConfig(ctx, merchantKey)` 通常返回 `openapi.Config`：

| 字段 | 说明 |
| --- | --- |
| `AppID` | 支付宝应用 app id |
| `GatewayURL` | 沙箱或正式网关 |
| `Signer` | 应用私钥构造出的 RSA2 signer |
| `NotifyURL` | 异步回调地址 |
| `ReturnURL` | WAP/Page 同步跳转地址 |

沙箱网关：

```text
https://openapi-sandbox.dl.alipaydev.com/gateway.do
```

正式网关：

```text
https://openapi.alipay.com/gateway.do
```

### Stripe

`loadStripeConfig(ctx, merchantKey)` 通常返回 `stripego.Config`：

| 字段 | 说明 |
| --- | --- |
| `APIKey` | Stripe secret key |
| `SuccessURL` | Checkout 成功跳转 |
| `CancelURL` | Checkout 取消跳转 |

常用 `Extra`：

| 参数 | 说明 |
| --- | --- |
| `Extra["cancel_url"]` | 覆盖默认取消跳转 |
| `Extra["customer_id"]` | Stripe Customer ID |
| `Extra["receipt_email"]` | 收据邮箱 |
| `Extra["payment_method_types"]` | 支付方式列表 |
| `Extra["charge_id"]` | 按 Charge 退款 |

### PayPal

`loadPayPalConfig(ctx, merchantKey)` 通常返回 `ordersapi.Config`：

| 字段 | 说明 |
| --- | --- |
| `ClientID` | PayPal REST app client id |
| `ClientSecret` | PayPal REST app secret |
| `BaseURL` | 沙箱或正式 API |
| `ReturnURL` | 买家批准后跳转 |
| `CancelURL` | 买家取消后跳转 |

沙箱：

```text
https://api-m.sandbox.paypal.com
```

正式：

```text
https://api-m.paypal.com
```

退款需要 `Extra["paypal_capture_id"]`。

## 回调

统一回调入口：

```go
result, err := payment.HandleNotify(ctx, payment.NotifyRequest{
	Provider:   payment.ProviderWechat,
	Channel:    payment.ChannelWechatMiniProgram,
	Method:     r.Method,
	Header:     r.Header,
	Query:      r.URL.Query(),
	Body:       body,
	ReceivedAt: time.Now(),
})
if err != nil {
	return err
}
w.WriteHeader(result.Ack.StatusCode)
_, _ = w.Write(result.Ack.Body)
```

回调需要能定位真实商户配置。常见做法是在回调 URL 或 query 中带 `merchant_key`，provider adapter 会用它加载对应配置验签。

## Debug

路由 debug：

```go
router, err := channelrouter.NewAdapter(channelrouter.Config{
	Provider:         payment.ProviderStripe,
	DebugPayloadMode: channelrouter.DebugPayloadFull,
	DebugLogger: channelrouter.DebugFunc(func(ctx context.Context, event channelrouter.DebugEvent) {
		log.Debug("payment channel",
			zap.String("stage", string(event.Stage)),
			zap.String("provider", string(event.Provider)),
			zap.String("channel", string(event.Channel)),
			zap.String("requested_merchant_key", event.RequestedMerchantKey),
			zap.String("resolved_merchant_key", event.ResolvedMerchantKey),
			zap.Any("request", event.Request),
			zap.Any("response", event.Response),
			zap.Error(event.Err),
		)
	}),
})
```

Stripe 和 PayPal 平台请求级 debug：

```go
stripeClient, err := stripego.NewClient(stripego.Config{
	APIKey:           cfg.APIKey,
	SuccessURL:       cfg.SuccessURL,
	CancelURL:        cfg.CancelURL,
	DebugPayloadMode: debuglog.PayloadFull,
	DebugLogger:      debugLogger,
})
```

`PayloadFull` 可能包含 openid、邮箱、订单备注、client secret、approval URL 等敏感信息，只建议在本地、沙箱或受控调试环境打开。平台密钥、私钥、证书不要写入 debug payload。

## Mock

测试中可以使用 mock adapter：

```go
adapter := mock.New(
	payment.ProviderAggregate,
	mock.WithChannels(payment.ChannelAggregateForm),
	mock.WithCurrencies("CNY"),
	mock.WithActions(payment.ActionHTMLForm),
	mock.WithAction(payment.PaymentAction{
		Type:   payment.ActionHTMLForm,
		URL:    "https://pay.example.test/form",
		Fields: map[string]string{"token": "abc"},
	}),
)
```

## 沙箱测试

支付宝沙箱测试默认跳过，需要设置环境变量：

```bash
SDKIT_ALIPAY_SANDBOX=1
SDKIT_ALIPAY_APP_ID=...
SDKIT_ALIPAY_PRIVATE_KEY_PEM='-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----'
SDKIT_ALIPAY_NOTIFY_URL=https://example.com/api/pay/notify/alipay
SDKIT_ALIPAY_RETURN_URL=https://example.com/pay/result
SDKIT_ALIPAY_GATEWAY_URL=https://openapi-sandbox.dl.alipaydev.com/gateway.do
```

执行：

```bash
go test -tags sdkit_payment_alipay ./tests/pkg/payment/alipay/openapi -run TestSandboxConfigBuildsPagePayRequest
```

查询、退款和退款查询：

```bash
SDKIT_ALIPAY_QUERY_OUT_TRADE_NO=sdkit_sandbox_202605260001 \
go test -tags sdkit_payment_alipay ./tests/pkg/payment/alipay/openapi -run TestSandboxQueryPayment

SDKIT_ALIPAY_REFUND_OUT_TRADE_NO=sdkit_sandbox_202605260001 \
SDKIT_ALIPAY_REFUND_OUT_REQUEST_NO=sdkit_refund_202605260001 \
go test -tags sdkit_payment_alipay ./tests/pkg/payment/alipay/openapi -run TestSandboxRefund

SDKIT_ALIPAY_REFUND_QUERY_OUT_TRADE_NO=sdkit_sandbox_202605260001 \
SDKIT_ALIPAY_REFUND_QUERY_OUT_REQUEST_NO=sdkit_refund_202605260001 \
go test -tags sdkit_payment_alipay ./tests/pkg/payment/alipay/openapi -run TestSandboxQueryRefund
```


## Airwallex 普通支付（2026-09-11）

`pkg/payment/airwallex` 实现既有 `ProviderAdapter`；`httpapi` 基于现有 `pkg/request` 调用官方 REST API。官方目前提供 Node.js Beta 服务端 SDK，未列出 Go 服务端 SDK，因此本实现没有引入第三方 Go SDK。

编译启用 `sdkit_payment_airwallex`，业务沿用 `core/payment`。接口调用顺序为 facade/Service → Registry → adapter → httpapi；core 不 import 具体 provider。

### 初始化与 configs

配置文件仍由应用的 core config loader 加载。`payment.channels` 是 facade 已有通道映射，凭据通过应用自己的显式 loader 传入 `httpapi.Config`，不能假定 facade 会自动读取平台密钥。

```yaml
payment:
  channels:
    - key: platform_airwallex_sandbox_v1
      provider: airwallex
      channel: airwallex_hpp
      merchant_key: platform_airwallex_sandbox_v1
```

服务器 configs / 本地私密覆写中的参数对应关系：

| 参数 | httpapi.Config | 说明 |
| --- | --- | --- |
| environment | Environment | 必须显式选择 `httpapi.Sandbox` 或 `httpapi.Production`；分别使用官方 sandbox / production 域名，不能只更换域名复用密钥 |
| client_id | ClientID | 所用 Scoped Key 对应的 Client ID |
| api_key | APIKey | 服务端私密凭据 |
| account_id | AccountID | 必须显式配置回调所属账户；不随认证模式切换而关闭账号校验 |
| authentication_mode | AuthenticationMode | 空值或 `httpapi.AuthenticateAccount` 保持登录发送 x-login-as；单账号 Scoped Key 可显式选择 `httpapi.AuthenticateDefault` 不发送该头，AccountID 仍必填 |
| webhook_secret | WebhookSecret | 每个 endpoint 对应的验签密钥；未配置时 HTTP 操作仍可用，但所有回调拒绝 |
| return_url | ReturnURL | HTTPS 默认返回地址，可由服务端 CreatePaymentRequest.ReturnURL 覆写 |

其他配置：`WebhookTolerance` 默认 5 分钟，负值拒绝；`Clock` 可注入时钟；`HTTPClient` 可注入 transport 供离线测试，重定向始终关闭；`CurrencyMetadata` 可补充 core 默认列表之外的币种精度，允许 0–6 位，应用的 PricingPolicy 也需使用相同精度。币种元数据不表示商户已开通该币种。

接线示例（假设 `cfg` 已由应用 loader 填好）：

```go
import (
    "context"

    "github.com/huwenlong92/sdkit/core/payment"
    "github.com/huwenlong92/sdkit/pkg/payment/airwallex"
    "github.com/huwenlong92/sdkit/pkg/payment/airwallex/httpapi"
)

client, err := httpapi.NewClient(cfg)
if err != nil {
    return err
}
adapter, err := airwallex.NewAdapter(airwallex.Config{
    NotifyMerchantKey: "platform_airwallex_sandbox_v1",
    ClientLoader: airwallex.ClientLoaderFunc(func(ctx context.Context, key string) (airwallex.Client, airwallex.ClientCleanup, error) {
        if key != "platform_airwallex_sandbox_v1" {
            return nil, nil, payment.ErrAdapterNotFound
        }
        return client, nil, nil
    }),
    SupportedCurrencies: []string{"USD"},
})
if err != nil {
    return err
}
return registry.Register(adapter)
```

动态模式是默认值，必须给 ClientLoader；单固定账户也可显式选择 `ClientModeStatic` 并提供 Client。缓存并复用同一个账户/环境/凭据版本的 httpapi.Client，避免每次执行 ClientLoader 都新建客户端、丢失 token 缓存。变更凭据时创建新 client，不能原地修改正在使用的 client。

### 创建支付和前端动作

每次创建、关闭、退款前，业务必须生成并持久化一个 v4 UUID，放入 `Extra[httpapi.ExtraRequestIDKey]`。不同操作用不同 UUID；同一操作重试保持同一 UUID、账户与完整参数。adapter 不随机生成幂等键，不保存业务幂等记录。

```go
resp, err := payment.CreatePayment(ctx, payment.CreatePaymentRequest{
    MerchantKey: "platform_airwallex_sandbox_v1",
    PaymentID:   paymentID,
    OrderID:     orderID,
    OutTradeNo:  paymentNumber,
    Pricing: payment.PaymentPricing{
        PayAmount:      payment.Money{Amount: 1234, Currency: "USD"},
    },
    Extra: map[string]any{httpapi.ExtraRequestIDKey: persistedRequestID},
})
```

`OutTradeNo`（未提供则 PaymentID）映射 `merchant_order_id`；支付意图 ID 返回为 `ProviderTradeID`。支付金额以最小货币单位传入，以精确 JSON decimal 发送；仅传 PayAmount 即可；省略 SettleCurrency 不会触发换汇。adapter 不宣称查询结果里的付款金额就是渠道最终结算金额，因此查询只提供 PayAmount，退款只提供 Refund 金额。

初始待付款或需要操作时，`Action.Type = sdk_params`，`Action.Params` 包含 `env`（sandbox/prod）、`mode`、`intent_id`、`client_secret`、`currency`、`successUrl`。前端用官方 `@airwallex/components-sdk` 初始化后调用 `payments.redirectToCheckout(params)`；不自行拼接 URL。已付款、已关闭或处理中等无需新动作的结果返回 `none`，未知状态为空并保留 `Extra.provider_status`，业务不得自动视为成功。

`client_secret` 只通过受授权的支付动作响应交给对应付款人，不记录或向其他角色展示。Raw 不回传供应商原始响应。ReturnURL 只是导航，不能决定支付结果。

### 查询、关闭、退款

- QueryPayment / ClosePayment / Refund 必须提供已持久化的 `ProviderTradeID`（Airwallex intent ID）；不通过订单号猜测远端资源。
- ClosePayment 调 cancel API，仅在返回相同 ID 且状态为 CANCELLED 时成功；渠道不允许取消时原样返回错误，不模拟关单。`ExpireAt` 尚不支持，创建时显式拒绝。
- Refund 先读取原支付，核对付款成功、币种与退款金额范围，再调用退款 API。剩余可退金额及并发退款由渠道约束，业务也必须维护退款幂等与累计金额。
- QueryRefund 必须提供 `ProviderRefundID`。RECEIVED → pending，ACCEPTED/SETTLED → succeeded，FAILED → failed；保留原始状态区分已受理完成与渠道对账，不据此宣称持卡人银行已入账。
- 不支持独立 capture、Connected Accounts 或 Funds Split；Transfers 通过独立 CreatePayout/QueryPayout 能力提供。

### 回调与错误处理

回调入口由业务 Web 服务提供，调用 `payment.HandleNotify`，传 POST、原始 body 和原始 header。`NotifyRequest.MerchantKey` 必须由服务端已配置的 endpoint 映射确定，不能把 URL query/body 的 merchant_key 当作账户授权；未传时兼容 adapter 的 `NotifyMerchantKey`。多账户由同一 adapter 的 ClientLoader 加载各自验签配置。

验签使用 `x-timestamp` 的原始毫秒字符串 + 原始 body，HMAC-SHA256 恒定时间比较，并校验本机时间窗口和 account_id。请求中传入的 `client-secret-key` 不作为验签密钥。事件名与对象状态不一致时拒绝；未知事件返回 verified + EventUnknown，不推进成功状态。PaymentAttempt 失败不代表整个 PaymentIntent 失败。

统一事件包含 EventID、远端支付/退款 ID、金额和状态；Raw 留空，不复制 client_secret 或身份资料。只有业务完成持久化、重复事件去重和订单匹配后才返回 `Ack` 建议的 200。退款 accepted/settled 是不同事件但同为退款成功，消费者须防止重复退款记账；旧事件/乱序状态也由业务拒绝回退。

HTTP 请求复用 pkg/request（默认单次请求），不自动重试资金操作；401 清除过期 token，由调用方以原幂等键重试。`*httpapi.APIError` 保留 HTTP StatusCode/Code，不包含上游 message/body。超时、5xx、响应损坏或响应匹配失败都不能认定“没有扣款”，应使用原请求重试/主动查询核实。

参考：[官方 SDK 列表](https://www.airwallex.com/docs/developer-tools/sdks)、[HPP 参数](https://www.airwallex.com/docs/js/payments/hosted-payment-page)、[PaymentIntents](https://www.airwallex.com/docs/payments/get-started/using-payments-intent-api)、[Refund API](https://www.airwallex.com/docs/api/payments/refunds/create)、[Webhook 验签](https://www.airwallex.com/docs/developer-tools/webhooks/listen-for-webhook-events)。


## 2026-09-11：支付主流程与可选能力

支付金额/币种/状态是主流程。支付币种为空默认 CNY；未指定结算币种时不隐式换汇。显式设置结算币种及 ExchangeRate 的计算能力继续保留。ProviderSettlement 是下单、查询及回调共有的可选渠道事实，和调用方计算的 Pricing 分开；缺失不等于零，不要求补齐，也不额外请求结算报告。

CreatePayout/QueryPayout 根据可选 PayoutProvider 接口分派，普通支付 adapter 不实现时返回 ErrUnsupportedCapability。打款请求使用收款人 ID、最小单位金额、币种、持久化 RequestID；业务方负责授权及幂等存储。Airwallex 使用 Transfers 协议，SENT 仅表示已发出，不能当作收款成功。

同一种 provider 可有多个 merchant_key；一个 adapter 通过 ClientLoader 选择商户实例。ChannelBinding.disabled_for_new 阻止新下单/打款，查询、退款和回调继续使用原绑定。NotifyRequest.MerchantKey 由服务端回调路由选择，adapter 据此加载验签配置；签名校验后还须匹配原支付商户。不得因超时自动换商户重试。

Airwallex 目前官方服务端 SDK 仅列出 Node.js Beta，Go 协议实现复用 sdkit/pkg/request。参考：https://www.airwallex.com/docs/developer-tools/sdks/server-side-sdks-%28beta%29 。普通 PaymentIntent 文档未提供逐笔完整结算金额/汇率；它们通过单独的 Settlement Records API 提供。本轮不主动查询该 API，不伪造或从支付金额推导 ProviderSettlement。

运行时可调用 payment.RegisterProvider(adapter) 注册新类型，payment.ReloadChannels(bindings) 原子替换商户绑定；重复注册同名类型返回 ErrAdapterAlreadyExists。动态商户凭据通过 adapter 的 ClientLoader 提供，core 不持有业务配置和密钥。请求显式指定 Provider/Channel/MerchantKey；回调也支持服务端指定 MerchantKey。Airwallex 打款使用 ChannelAirwallexTransfer，同币种出款；接收人管理及自动换汇不在此操作范围。PAID 是渠道处理成功状态，之后仍可能失败，业务方不得将其视为永不可变的状态。


### 运行时注册与指定商户

下列代码在默认 Service 已初始化、使用可重载 ChannelSelector 后执行。`adapter` 是已构造的 ProviderAdapter，一个 provider 类型注册一次，多账号由它的 ClientLoader 按 merchant_key 加载。

```go
if err := payment.RegisterProvider(adapter); err != nil {
    return err
}
// ReloadChannels 原子替换整份绑定；更新时保留其他 provider 和旧订单所需绑定。
if err := payment.ReloadChannels([]payment.ChannelBinding{
    {Key: "airwallex_a_v1", Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "airwallex_a_v1", DisabledForNew: true},
    {Key: "airwallex_b_v1", Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "airwallex_b_v1"},
}); err != nil {
    return err
}
resp, err := payment.CreatePayment(ctx, payment.CreatePaymentRequest{
    Provider: payment.ProviderAirwallex,
    Channel: payment.ChannelAirwallexHPP,
    MerchantKey: "airwallex_b_v1",
    PaymentID: paymentID,
    Pricing: payment.PaymentPricing{
        PayAmount: payment.Money{Amount: 10000, Currency: "USD"},
    },
    Extra: map[string]any{httpapi.ExtraRequestIDKey: persistedRequestID},
})
```

账号 A 停止新支付后，旧支付查询和回调仍携带 A 的 merchant_key。新增账号用新的 key，保留旧凭据版本；ReloadChannels 不是 YAML 文件监听器，也不能把在途请求自动迁到账号 B。已配置选择器会校验显式 provider/channel 与商户绑定一致；无选择器时可直接按显式 provider/channel 调用注册的 adapter。

Transfers 请求固定发送 `x-api-version: 2024-09-27`，避免账号默认旧版本改变资源名称及字段。参考：[API 版本约定](https://www.airwallex.com/docs/api/core_resources/global_accounts/api)、[Transfers 版本变更](https://www.airwallex.com/docs/payouts/transfers/create-a-transfer)。


### 下单地址参数（2026-09-11）

`CreatePaymentRequest.ReturnURL` 和 `NotifyURL` 是单笔支付参数。支持的 adapter 使用请求值，未传时使用该商户可选默认值；同一次渠道幂等重试应保留首次参数。ReturnURL 只决定客户端返回页面，不是付款成功凭据。

Airwallex 支持请求级 ReturnURL。它的 PaymentIntent API 不接受逐笔 notify_url，通知地址通过 Webhook 订阅预先注册。`httpapi.Config.NotifyURL` 记录已在 Airwallex 注册的地址，不会创建或修改订阅；请求 NotifyURL 为空或等于该地址时正常下单，其他值在网络请求前返回 ErrUnsupportedCapability。微信/支付宝沿用已有请求 NotifyURL 优先于 client 默认值的行为。

参考：[Airwallex Webhook 订阅](https://www.airwallex.com/docs/developer-tools/webhooks/webhooks-overview)、[PaymentIntent API](https://www.airwallex.com/docs/api/payments/payment_intents/create)。


### Provider expiry (2026-09-11)

`CreatePaymentRequest.ExpireAt` requests a provider expiry only when `SupportsExpireAt` is supported. Create/query responses and `PaymentEvent` optionally return the provider-confirmed `ExpireAt`; nil means unknown. `PaymentExpired` / `EventPaymentExpired` represent provider-confirmed expiration. Business deadlines stay with the caller and are not proof that a provider cannot collect. A verified late success may supersede expiration; stale intermediate results cannot reopen an expired payment. `CreatePaymentResponse.PaidAt` optionally carries the provider payment timestamp.

Airwallex token 到期时间兼容 RFC3339 与 ISO-8601 无冒号时区偏移（如 `+0000`），保留严格到期校验和提前刷新。


业务方每次明确重新支付应创建新的支付单号和 request_id。相同 HTTP 命令重放保持原 request_id；已知 ProviderTradeID 时可读取既有渠道单返回付款动作。Airwallex 不再在 duplicate_request 后自动检索或重建支付单；渠道错误原样交由调用方处理。

### Accounting evidence (2026-09-15)

After an authenticated Airwallex `QueryPayment`, persist `resp.RawBody` as private immutable evidence if required, before applying the normalized payment state. The field retains exact successful HTTP response bytes and is excluded from JSON serialization. Do not write it to logs, browser storage or customer API responses: it can contain client_secret and personal information. Label query evidence separately from real webhook delivery; historical missing original bytes cannot be recreated from normalized fields.

`resp.Details` / normalized `event.Details` optionally report actual method, attempt ID, underlying transaction ID, card brand and four-digit tail. Only project these safe fields into an authorized customer payment receipt. `int_` identifies the intent; `att_` identifies a payment attempt; `payment_method_transaction_id` is the method provider's transaction reference. Missing references or fees are not zero or fabricated. Field definitions: https://www.airwallex.com/docs/api/2024-04-30/payments/payment_intents .

## Airwallex 受益人与批量打款（2026-09-17）

受益人资料使用 Airwallex 动态 schema 组织为 `Details`。业务系统先保存并审核自己的账户资料，再创建受益人并持久化返回的 `BeneficiaryID`；后续批量打款复用该 ID。银行账户等实质资料变化后是否清除并重建绑定，由业务系统决定，sdkit 不维护账户修订号。

Airwallex 创建和查询受益人的响应标识位于顶层 `id`；adapter 会将其映射为 `BeneficiaryResponse.BeneficiaryID`。请求批量打款项目时仍按 Airwallex 协议发送 `beneficiary_id`。

```go
beneficiary, err := payment.CreateBeneficiary(ctx, payment.CreateBeneficiaryRequest{
    Provider:    payment.ProviderAirwallex,
    Channel:     payment.ChannelAirwallexTransfer,
    MerchantKey: merchantKey,
    Details:     beneficiaryDetails,
})
if err != nil {
    if exchange, ok := payment.ProviderExchangeFromError(err); ok {
        // 写入受控的追加式证据存储；不要写普通日志或返回客户端。
        persistProviderExchange(exchange)
    }
    return err
}
persistProviderExchange(beneficiary.Exchange)
```

批量打款按“创建 → 分段添加项目 → 提交 → 查询批次和项目”执行。每次 `AddPayoutBatchItems` 最多 100 项；调用方负责限制单个渠道批次不超过 Airwallex 的 1,000 项。所有 mutation 的 `RequestID` 必须在调用前持久化，超时后以原 ID 查询恢复，不能重新生成。

```go
batch, err := payment.CreatePayoutBatch(ctx, payment.CreatePayoutBatchRequest{
    Provider:    payment.ProviderAirwallex,
    Channel:     payment.ChannelAirwallexTransfer,
    MerchantKey: merchantKey,
    RequestID:   persistedBatchRequestID,
    Name:        "2026-09 settlement",
})
if err != nil {
    return retainExchangeAndReturn(err)
}
persistProviderExchange(batch.Exchange)

batch, err = payment.AddPayoutBatchItems(ctx, payment.AddPayoutBatchItemsRequest{
    Provider:        payment.ProviderAirwallex,
    Channel:         payment.ChannelAirwallexTransfer,
    MerchantKey:     merchantKey,
    ProviderBatchID: batch.ProviderBatchID,
    Items: []payment.PayoutBatchItemRequest{{
        RequestID:      persistedItemRequestID,
        BeneficiaryID:  beneficiaryID,
        Amount:         payment.Money{Amount: 4560, Currency: "USD"},
        TransferMethod: "LOCAL",
        Reference:      "SET-202609-001",
        Reason:         "business_services",
    }},
})
if err != nil {
    return retainExchangeAndReturn(err)
}
persistProviderExchange(batch.Exchange)

batch, err = payment.SubmitPayoutBatch(ctx, payment.SubmitPayoutBatchRequest{
    Provider:        payment.ProviderAirwallex,
    Channel:         payment.ChannelAirwallexTransfer,
    MerchantKey:     merchantKey,
    ProviderBatchID: batch.ProviderBatchID,
})
```

查询批次时 `ProviderBatchID` 与创建时的 `RequestID` 二选一；按 RequestID 查询适合创建请求结果未知时恢复远端批次。`ListPayoutBatchItems` 返回每个项目的渠道 item ID、Transfer ID、原始状态和首个失败信息。通用层不把 `SCHEDULED`、`BOOKING`、`BOOKED` 或单笔 Transfer 的 `PAID` 自动解释为业务结算完成，业务系统必须按自己的状态机和渠道后续结果收口。

成功响应的 `Exchange` 及 `ProviderExchangeFromError(err)` 返回的失败交换数据均为敏感证据，字段通过 `json:"-"` 排除普通序列化，但仍可能包含账户或个人资料。只允许写入受权限和审计控制的原文存储。网络超时或取消可能只有请求原文，没有 HTTP 状态和响应体；这表示结果未知，不能据此重试出款或标记失败。
