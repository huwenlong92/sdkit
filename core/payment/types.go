package payment

import (
	"context"
	"time"
)

const DefaultCurrency = "CNY"

type Provider string

const (
	ProviderWechat    Provider = "wechat"
	ProviderAlipay    Provider = "alipay"
	ProviderPayPal    Provider = "paypal"
	ProviderStripe    Provider = "stripe"
	ProviderAggregate Provider = "aggregate"
)

type Channel string

const (
	ChannelWechatApp         Channel = "wechat_app"
	ChannelWechatMiniProgram Channel = "wechat_mini_program"
	ChannelWechatH5          Channel = "wechat_h5"
	ChannelWechatNative      Channel = "wechat_native"

	ChannelAlipayApp  Channel = "alipay_app"
	ChannelAlipayWap  Channel = "alipay_wap"
	ChannelAlipayPage Channel = "alipay_page"

	ChannelPayPalOrder    Channel = "paypal_order"
	ChannelStripeCheckout Channel = "stripe_checkout"
	ChannelStripeIntent   Channel = "stripe_payment_intent"
	ChannelAggregateForm  Channel = "aggregate_form"
)

type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type PaymentPricing struct {
	OrderAmount    Money                 `json:"order_amount"`
	PayAmount      Money                 `json:"pay_amount"`
	SettleCurrency string                `json:"settle_currency,omitempty"`
	SettleAmount   Money                 `json:"settle_amount"`
	FeeAmount      *Money                `json:"fee_amount,omitempty"`
	ExchangeRate   *ExchangeRateSnapshot `json:"exchange_rate,omitempty"`
}

type ExchangeRateSnapshot struct {
	FromCurrency string `json:"from_currency"`
	ToCurrency   string `json:"to_currency"`
	Rate         string `json:"rate"`
	Source       string `json:"source"`
	QuotedAt     int64  `json:"quoted_at"`
}

type CurrencyMeta struct {
	Code     string
	MinorExp int
}

type PricingPolicy interface {
	NormalizePricing(ctx context.Context, pricing PaymentPricing) (PaymentPricing, error)
}

type ActionType string

const (
	ActionNone        ActionType = "none"
	ActionRedirectURL ActionType = "redirect_url"
	ActionHTMLForm    ActionType = "html_form"
	ActionQRCode      ActionType = "qr_code"
	ActionSDKParams   ActionType = "sdk_params"
	ActionClientToken ActionType = "client_token"
)

type PaymentAction struct {
	Type        ActionType        `json:"type"`
	Method      string            `json:"method,omitempty"`
	URL         string            `json:"url,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
	HTML        string            `json:"html,omitempty"`
	EncodedHTML string            `json:"encoded_html,omitempty"`
	Token       string            `json:"token,omitempty"`
	Params      map[string]any    `json:"params,omitempty"`
}

type PaymentStatus string

const (
	PaymentPending         PaymentStatus = "pending"
	PaymentProcessing      PaymentStatus = "processing"
	PaymentRequiresAction  PaymentStatus = "requires_action"
	PaymentAuthorized      PaymentStatus = "authorized"
	PaymentSucceeded       PaymentStatus = "succeeded"
	PaymentFailed          PaymentStatus = "failed"
	PaymentClosed          PaymentStatus = "closed"
	PaymentRefunding       PaymentStatus = "refunding"
	PaymentPartialRefunded PaymentStatus = "partial_refunded"
	PaymentRefunded        PaymentStatus = "refunded"
)

type RefundStatus string

const (
	RefundPending    RefundStatus = "pending"
	RefundProcessing RefundStatus = "processing"
	RefundSucceeded  RefundStatus = "succeeded"
	RefundFailed     RefundStatus = "failed"
	RefundClosed     RefundStatus = "closed"
)

type EventType string

const (
	EventPaymentCreated    EventType = "payment.created"
	EventPaymentSucceeded  EventType = "payment.succeeded"
	EventPaymentFailed     EventType = "payment.failed"
	EventPaymentClosed     EventType = "payment.closed"
	EventPaymentAuthorized EventType = "payment.authorized"
	EventPaymentCaptured   EventType = "payment.captured"
	EventPaymentRefunding  EventType = "payment.refunding"
	EventPaymentRefunded   EventType = "payment.refunded"
	EventPaymentPartRefund EventType = "payment.partial_refunded"
	EventRefundCreated     EventType = "refund.created"
	EventRefundProcessing  EventType = "refund.processing"
	EventRefundSucceeded   EventType = "refund.succeeded"
	EventRefundFailed      EventType = "refund.failed"
	EventDisputeCreated    EventType = "dispute.created"
	EventDisputeClosed     EventType = "dispute.closed"
	EventUnknown           EventType = "unknown"
)

type PaymentEvent struct {
	EventID          string         `json:"event_id"`
	Type             EventType      `json:"type"`
	Provider         Provider       `json:"provider"`
	Channel          Channel        `json:"channel"`
	MerchantKey      string         `json:"merchant_key,omitempty"`
	PaymentID        string         `json:"payment_id,omitempty"`
	OrderID          string         `json:"order_id,omitempty"`
	OutTradeNo       string         `json:"out_trade_no,omitempty"`
	ProviderTradeID  string         `json:"provider_trade_id,omitempty"`
	RefundID         string         `json:"refund_id,omitempty"`
	ProviderRefundID string         `json:"provider_refund_id,omitempty"`
	Status           PaymentStatus  `json:"status,omitempty"`
	RefundStatus     RefundStatus   `json:"refund_status,omitempty"`
	Amount           Money          `json:"amount"`
	PaidAt           *time.Time     `json:"paid_at,omitempty"`
	Raw              []byte         `json:"raw,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
}

type Capabilities struct {
	Provider              Provider     `json:"provider"`
	Channels              []Channel    `json:"channels,omitempty"`
	SupportedCurrencies   []string     `json:"supported_currencies,omitempty"`
	SupportedActions      []ActionType `json:"supported_actions,omitempty"`
	SupportsMultiMerchant bool         `json:"supports_multi_merchant,omitempty"`
	SupportsExpireAt      bool         `json:"supports_expire_at,omitempty"`
	SupportsClose         bool         `json:"supports_close,omitempty"`
	SupportsRefund        bool         `json:"supports_refund,omitempty"`
	SupportsQueryRefund   bool         `json:"supports_query_refund,omitempty"`
	SupportsPartialRefund bool         `json:"supports_partial_refund,omitempty"`
	SupportsQuery         bool         `json:"supports_query,omitempty"`
	SupportsNotify        bool         `json:"supports_notify,omitempty"`
	SupportsCapture       bool         `json:"supports_capture,omitempty"`
	SupportsConfirm       bool         `json:"supports_confirm,omitempty"`
	SupportsReconcile     bool         `json:"supports_reconcile,omitempty"`
	SupportsDispute       bool         `json:"supports_dispute,omitempty"`
	SupportsSubscription  bool         `json:"supports_subscription,omitempty"`
}
