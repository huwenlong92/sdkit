package channelrouter

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

type ResolveRequest struct {
	Provider    payment.Provider
	Channel     payment.Channel
	MerchantKey string
	Operation   Operation
}

type Operation string

const (
	OperationCreate      Operation = "create"
	OperationQuery       Operation = "query"
	OperationClose       Operation = "close"
	OperationRefund      Operation = "refund"
	OperationQueryRefund Operation = "query_refund"
	OperationNotify      Operation = "notify"
)

type ResolvedRoute struct {
	Provider    payment.Provider
	Channel     payment.Channel
	MerchantKey string
	Adapter     payment.ProviderAdapter
}

type Resolver interface {
	ResolvePaymentChannel(ctx context.Context, req ResolveRequest) (*ResolvedRoute, error)
}

type Route struct {
	Provider    payment.Provider
	Channels    []payment.Channel
	MerchantKey string
	Default     bool
	Adapter     payment.ProviderAdapter
}

type Config struct {
	Provider         payment.Provider
	Resolver         Resolver
	Routes           []Route
	DebugLogger      DebugLogger
	DebugPayloadMode DebugPayloadMode
}

type Adapter struct {
	provider         payment.Provider
	resolver         Resolver
	debugLogger      DebugLogger
	debugPayloadMode DebugPayloadMode
	capabilities     payment.Capabilities
}

type DebugLogger interface {
	DebugPaymentChannel(ctx context.Context, event DebugEvent)
}

type DebugFunc func(ctx context.Context, event DebugEvent)

func (fn DebugFunc) DebugPaymentChannel(ctx context.Context, event DebugEvent) {
	if fn != nil {
		fn(ctx, event)
	}
}

type DebugEvent struct {
	Stage                DebugStage       `json:"stage"`
	Operation            Operation        `json:"operation"`
	Provider             payment.Provider `json:"provider"`
	Channel              payment.Channel  `json:"channel"`
	RequestedMerchantKey string           `json:"requested_merchant_key,omitempty"`
	ResolvedMerchantKey  string           `json:"resolved_merchant_key,omitempty"`
	PaymentID            string           `json:"payment_id,omitempty"`
	OrderID              string           `json:"order_id,omitempty"`
	OutTradeNo           string           `json:"out_trade_no,omitempty"`
	ProviderTradeID      string           `json:"provider_trade_id,omitempty"`
	RefundID             string           `json:"refund_id,omitempty"`
	OutRefundNo          string           `json:"out_refund_no,omitempty"`
	ProviderRefundID     string           `json:"provider_refund_id,omitempty"`
	Err                  error            `json:"-"`
	Request              any              `json:"request,omitempty"`
	Response             any              `json:"response,omitempty"`
}

type DebugStage string

const (
	DebugStageResolveStart     DebugStage = "resolve_start"
	DebugStageResolveSucceeded DebugStage = "resolve_succeeded"
	DebugStageResolveFailed    DebugStage = "resolve_failed"
	DebugStageOperationStart   DebugStage = "operation_start"
	DebugStageOperationDone    DebugStage = "operation_done"
	DebugStageOperationFailed  DebugStage = "operation_failed"
)

type DebugPayloadMode string

const (
	DebugPayloadNone DebugPayloadMode = ""
	DebugPayloadFull DebugPayloadMode = "full"
)
