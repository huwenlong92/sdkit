package alipay

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

type Client interface {
	CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error)
}

type QueryClient interface {
	QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error)
}

type CloseClient interface {
	ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error
}

type RefundClient interface {
	Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error)
}

type QueryRefundClient interface {
	QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error)
}

type NotifyClient interface {
	ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error)
}

type ClientMode string

const (
	ClientModeDynamic ClientMode = "dynamic"
	ClientModeStatic  ClientMode = "static"
)

type ClientCleanup func() error

type ClientLoader interface {
	LoadPaymentClient(ctx context.Context, merchantKey string) (Client, ClientCleanup, error)
}

type ClientLoaderFunc func(ctx context.Context, merchantKey string) (Client, ClientCleanup, error)

func (fn ClientLoaderFunc) LoadPaymentClient(ctx context.Context, merchantKey string) (Client, ClientCleanup, error) {
	return fn(ctx, merchantKey)
}

type Config struct {
	Client                Client
	ClientLoader          ClientLoader
	ClientMode            ClientMode
	SupportedCurrencies   []string
	SupportsExpireAt      bool
	SupportsMultiMerchant bool
	SupportsClose         bool
	SupportsRefund        bool
	SupportsQueryRefund   bool
	SupportsQuery         bool
	SupportsNotify        bool
}
