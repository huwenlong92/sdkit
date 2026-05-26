package payment

import (
	"context"
	"time"
)

type CreatePaymentRequest struct {
	Provider    Provider       `json:"provider"`
	Channel     Channel        `json:"channel"`
	MerchantKey string         `json:"merchant_key,omitempty"`
	PaymentID   string         `json:"payment_id,omitempty"`
	OrderID     string         `json:"order_id,omitempty"`
	OutTradeNo  string         `json:"out_trade_no,omitempty"`
	Subject     string         `json:"subject,omitempty"`
	Body        string         `json:"body,omitempty"`
	Pricing     PaymentPricing `json:"pricing"`
	NotifyURL   string         `json:"notify_url,omitempty"`
	ReturnURL   string         `json:"return_url,omitempty"`
	ExpireAt    *time.Time     `json:"expire_at,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

type CreatePaymentResponse struct {
	Provider        Provider       `json:"provider"`
	Channel         Channel        `json:"channel"`
	MerchantKey     string         `json:"merchant_key,omitempty"`
	PaymentID       string         `json:"payment_id,omitempty"`
	OrderID         string         `json:"order_id,omitempty"`
	OutTradeNo      string         `json:"out_trade_no,omitempty"`
	ProviderTradeID string         `json:"provider_trade_id,omitempty"`
	Status          PaymentStatus  `json:"status"`
	Pricing         PaymentPricing `json:"pricing"`
	Action          PaymentAction  `json:"action"`
	Raw             any            `json:"raw,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type QueryPaymentRequest struct {
	Provider        Provider       `json:"provider"`
	Channel         Channel        `json:"channel"`
	MerchantKey     string         `json:"merchant_key,omitempty"`
	PaymentID       string         `json:"payment_id,omitempty"`
	OrderID         string         `json:"order_id,omitempty"`
	OutTradeNo      string         `json:"out_trade_no,omitempty"`
	ProviderTradeID string         `json:"provider_trade_id,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type QueryPaymentResponse struct {
	Provider        Provider       `json:"provider"`
	Channel         Channel        `json:"channel"`
	MerchantKey     string         `json:"merchant_key,omitempty"`
	PaymentID       string         `json:"payment_id,omitempty"`
	OrderID         string         `json:"order_id,omitempty"`
	OutTradeNo      string         `json:"out_trade_no,omitempty"`
	ProviderTradeID string         `json:"provider_trade_id,omitempty"`
	Status          PaymentStatus  `json:"status"`
	Pricing         PaymentPricing `json:"pricing"`
	PaidAt          *time.Time     `json:"paid_at,omitempty"`
	Raw             any            `json:"raw,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type ClosePaymentRequest struct {
	Provider        Provider       `json:"provider"`
	Channel         Channel        `json:"channel"`
	MerchantKey     string         `json:"merchant_key,omitempty"`
	PaymentID       string         `json:"payment_id,omitempty"`
	OrderID         string         `json:"order_id,omitempty"`
	OutTradeNo      string         `json:"out_trade_no,omitempty"`
	ProviderTradeID string         `json:"provider_trade_id,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type RefundRequest struct {
	Provider        Provider       `json:"provider"`
	Channel         Channel        `json:"channel"`
	MerchantKey     string         `json:"merchant_key,omitempty"`
	PaymentID       string         `json:"payment_id,omitempty"`
	OrderID         string         `json:"order_id,omitempty"`
	OutTradeNo      string         `json:"out_trade_no,omitempty"`
	ProviderTradeID string         `json:"provider_trade_id,omitempty"`
	RefundID        string         `json:"refund_id,omitempty"`
	OutRefundNo     string         `json:"out_refund_no,omitempty"`
	Amount          RefundAmount   `json:"amount"`
	Reason          string         `json:"reason,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type RefundAmount struct {
	Refund Money `json:"refund"`
	Settle Money `json:"settle"`
}

type RefundResponse struct {
	Provider         Provider       `json:"provider"`
	Channel          Channel        `json:"channel"`
	MerchantKey      string         `json:"merchant_key,omitempty"`
	PaymentID        string         `json:"payment_id,omitempty"`
	OrderID          string         `json:"order_id,omitempty"`
	OutTradeNo       string         `json:"out_trade_no,omitempty"`
	ProviderTradeID  string         `json:"provider_trade_id,omitempty"`
	RefundID         string         `json:"refund_id,omitempty"`
	OutRefundNo      string         `json:"out_refund_no,omitempty"`
	ProviderRefundID string         `json:"provider_refund_id,omitempty"`
	Status           RefundStatus   `json:"status"`
	Amount           RefundAmount   `json:"amount"`
	Raw              any            `json:"raw,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
}

type QueryRefundRequest struct {
	Provider         Provider       `json:"provider"`
	Channel          Channel        `json:"channel"`
	MerchantKey      string         `json:"merchant_key,omitempty"`
	PaymentID        string         `json:"payment_id,omitempty"`
	OutTradeNo       string         `json:"out_trade_no,omitempty"`
	ProviderTradeID  string         `json:"provider_trade_id,omitempty"`
	RefundID         string         `json:"refund_id,omitempty"`
	OutRefundNo      string         `json:"out_refund_no,omitempty"`
	ProviderRefundID string         `json:"provider_refund_id,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
}

type QueryRefundResponse struct {
	Provider         Provider       `json:"provider"`
	Channel          Channel        `json:"channel"`
	MerchantKey      string         `json:"merchant_key,omitempty"`
	PaymentID        string         `json:"payment_id,omitempty"`
	OutTradeNo       string         `json:"out_trade_no,omitempty"`
	ProviderTradeID  string         `json:"provider_trade_id,omitempty"`
	RefundID         string         `json:"refund_id,omitempty"`
	OutRefundNo      string         `json:"out_refund_no,omitempty"`
	ProviderRefundID string         `json:"provider_refund_id,omitempty"`
	Status           RefundStatus   `json:"status"`
	Amount           RefundAmount   `json:"amount"`
	Raw              any            `json:"raw,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
}

type NotifyRequest struct {
	Provider   Provider            `json:"provider"`
	Channel    Channel             `json:"channel"`
	Method     string              `json:"method,omitempty"`
	Header     map[string][]string `json:"header,omitempty"`
	Query      map[string][]string `json:"query,omitempty"`
	Form       map[string][]string `json:"form,omitempty"`
	Body       []byte              `json:"body,omitempty"`
	Raw        any                 `json:"-"`
	ReceivedAt time.Time           `json:"received_at"`
}

type NotifyResult struct {
	Verified bool          `json:"verified"`
	Event    *PaymentEvent `json:"event,omitempty"`
	Ack      NotifyAck     `json:"ack"`
}

type NotifyAck struct {
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
}

type ProviderAdapter interface {
	Name() Provider
	Capabilities() Capabilities
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResponse, error)
	QueryPayment(ctx context.Context, req QueryPaymentRequest) (*QueryPaymentResponse, error)
	ClosePayment(ctx context.Context, req ClosePaymentRequest) error
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
	QueryRefund(ctx context.Context, req QueryRefundRequest) (*QueryRefundResponse, error)
	ParseNotify(ctx context.Context, req NotifyRequest) (*NotifyResult, error)
}
