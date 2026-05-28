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
