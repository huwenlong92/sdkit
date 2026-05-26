package aggregate

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

const ExtraGatewayKey = "gateway"

type Gateway interface {
	CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error)
}

type QueryGateway interface {
	QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error)
}

type CloseGateway interface {
	ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error
}

type RefundGateway interface {
	Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error)
}

type QueryRefundGateway interface {
	QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error)
}

type NotifyGateway interface {
	ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error)
}

type Config struct {
	DefaultGateway string
	Gateways       map[string]Gateway
	Capabilities   payment.Capabilities
}
