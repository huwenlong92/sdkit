//go:build sdkit_payment_airwallex

package airwallex

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

// Client implements ordinary payments. It does not own business state or settlement.
type Client interface {
	CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error)
	QueryPayment(context.Context, payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error)
	ClosePayment(context.Context, payment.ClosePaymentRequest) error
	Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error)
	QueryRefund(context.Context, payment.QueryRefundRequest) (*payment.QueryRefundResponse, error)
	ParseNotify(context.Context, payment.NotifyRequest) (*payment.NotifyResult, error)
}
type ClientMode string

const (
	ClientModeDynamic ClientMode = "dynamic"
	ClientModeStatic  ClientMode = "static"
)

type ClientCleanup func() error
type ClientLoader interface {
	LoadPaymentClient(context.Context, string) (Client, ClientCleanup, error)
}
type ClientLoaderFunc func(context.Context, string) (Client, ClientCleanup, error)

func (fn ClientLoaderFunc) LoadPaymentClient(ctx context.Context, key string) (Client, ClientCleanup, error) {
	return fn(ctx, key)
}

type Config struct {
	Client              Client
	ClientLoader        ClientLoader
	ClientMode          ClientMode
	SupportedCurrencies []string
	// NotifyMerchantKey is a trusted server configuration value. Never derive it
	// from callback query/body fields. Multi-account receivers dispatch by endpoint.
	NotifyMerchantKey string
}
