package mock

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

type Option func(*Adapter)

func WithCapabilities(caps payment.Capabilities) Option {
	return func(a *Adapter) {
		if caps.Provider == "" {
			caps.Provider = a.name
		}
		a.capabilities = caps
	}
}

func WithChannels(channels ...payment.Channel) Option {
	return func(a *Adapter) {
		a.capabilities.Channels = append([]payment.Channel(nil), channels...)
	}
}

func WithCurrencies(currencies ...string) Option {
	return func(a *Adapter) {
		a.capabilities.SupportedCurrencies = append([]string(nil), currencies...)
	}
}

func WithActions(actions ...payment.ActionType) Option {
	return func(a *Adapter) {
		a.capabilities.SupportedActions = append([]payment.ActionType(nil), actions...)
	}
}

func WithAllCoreCapabilities() Option {
	return func(a *Adapter) {
		a.capabilities.SupportsClose = true
		a.capabilities.SupportsRefund = true
		a.capabilities.SupportsQueryRefund = true
		a.capabilities.SupportsQuery = true
		a.capabilities.SupportsNotify = true
	}
}

func WithCreatePayment(fn func(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error)) Option {
	return func(a *Adapter) {
		a.createPayment = fn
	}
}

func WithQueryPayment(fn func(context.Context, payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error)) Option {
	return func(a *Adapter) {
		a.queryPayment = fn
	}
}

func WithClosePayment(fn func(context.Context, payment.ClosePaymentRequest) error) Option {
	return func(a *Adapter) {
		a.closePayment = fn
	}
}

func WithRefund(fn func(context.Context, payment.RefundRequest) (*payment.RefundResponse, error)) Option {
	return func(a *Adapter) {
		a.refund = fn
	}
}

func WithQueryRefund(fn func(context.Context, payment.QueryRefundRequest) (*payment.QueryRefundResponse, error)) Option {
	return func(a *Adapter) {
		a.queryRefund = fn
	}
}

func WithParseNotify(fn func(context.Context, payment.NotifyRequest) (*payment.NotifyResult, error)) Option {
	return func(a *Adapter) {
		a.parseNotify = fn
	}
}

func WithAction(action payment.PaymentAction) Option {
	return WithCreatePayment(func(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
		return &payment.CreatePaymentResponse{
			Provider:    req.Provider,
			Channel:     req.Channel,
			MerchantKey: req.MerchantKey,
			PaymentID:   req.PaymentID,
			OrderID:     req.OrderID,
			OutTradeNo:  req.OutTradeNo,
			Status:      payment.PaymentPending,
			Pricing:     req.Pricing,
			Action:      action,
		}, nil
	})
}
