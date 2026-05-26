package debuglog

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

type PayloadMode string

const (
	PayloadNone PayloadMode = ""
	PayloadFull PayloadMode = "full"
)

type Logger interface {
	DebugPayment(ctx context.Context, event Event)
}

type Func func(ctx context.Context, event Event)

func (fn Func) DebugPayment(ctx context.Context, event Event) {
	if fn != nil {
		fn(ctx, event)
	}
}

type Event struct {
	Component        string           `json:"component"`
	Stage            Stage            `json:"stage"`
	Operation        string           `json:"operation"`
	Provider         payment.Provider `json:"provider,omitempty"`
	Channel          payment.Channel  `json:"channel,omitempty"`
	MerchantKey      string           `json:"merchant_key,omitempty"`
	PaymentID        string           `json:"payment_id,omitempty"`
	OrderID          string           `json:"order_id,omitempty"`
	OutTradeNo       string           `json:"out_trade_no,omitempty"`
	ProviderTradeID  string           `json:"provider_trade_id,omitempty"`
	RefundID         string           `json:"refund_id,omitempty"`
	OutRefundNo      string           `json:"out_refund_no,omitempty"`
	ProviderRefundID string           `json:"provider_refund_id,omitempty"`
	Request          any              `json:"request,omitempty"`
	Response         any              `json:"response,omitempty"`
	Err              error            `json:"-"`
}

type Stage string

const (
	StageRequest  Stage = "request"
	StageResponse Stage = "response"
	StageError    Stage = "error"
)
