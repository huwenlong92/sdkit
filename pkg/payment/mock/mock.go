package mock

import (
	"context"
	"sync"

	"github.com/huwenlong92/sdkit/core/payment"
)

type Adapter struct {
	mu sync.Mutex

	name         payment.Provider
	capabilities payment.Capabilities

	createPayment func(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error)
	queryPayment  func(context.Context, payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error)
	closePayment  func(context.Context, payment.ClosePaymentRequest) error
	refund        func(context.Context, payment.RefundRequest) (*payment.RefundResponse, error)
	queryRefund   func(context.Context, payment.QueryRefundRequest) (*payment.QueryRefundResponse, error)
	parseNotify   func(context.Context, payment.NotifyRequest) (*payment.NotifyResult, error)

	CreateRequests      []payment.CreatePaymentRequest
	QueryRequests       []payment.QueryPaymentRequest
	CloseRequests       []payment.ClosePaymentRequest
	RefundRequests      []payment.RefundRequest
	QueryRefundRequests []payment.QueryRefundRequest
	NotifyRequests      []payment.NotifyRequest
}

func New(provider payment.Provider, opts ...Option) *Adapter {
	if provider == "" {
		provider = payment.ProviderAggregate
	}
	a := &Adapter{
		name: provider,
		capabilities: payment.Capabilities{
			Provider: provider,
		},
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.capabilities.Provider == "" {
		a.capabilities.Provider = provider
	}
	return a
}

func (a *Adapter) Name() payment.Provider {
	return a.name
}

func (a *Adapter) Capabilities() payment.Capabilities {
	return a.capabilities
}

func (a *Adapter) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	a.mu.Lock()
	a.CreateRequests = append(a.CreateRequests, req)
	fn := a.createPayment
	a.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return &payment.CreatePaymentResponse{
		Provider:    req.Provider,
		Channel:     req.Channel,
		MerchantKey: req.MerchantKey,
		PaymentID:   req.PaymentID,
		OrderID:     req.OrderID,
		OutTradeNo:  req.OutTradeNo,
		Status:      payment.PaymentPending,
		Pricing:     req.Pricing,
		Action:      payment.PaymentAction{Type: payment.ActionNone},
	}, nil
}

func (a *Adapter) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	a.mu.Lock()
	a.QueryRequests = append(a.QueryRequests, req)
	fn := a.queryPayment
	a.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return &payment.QueryPaymentResponse{
		Provider:        req.Provider,
		Channel:         req.Channel,
		MerchantKey:     req.MerchantKey,
		PaymentID:       req.PaymentID,
		OrderID:         req.OrderID,
		OutTradeNo:      req.OutTradeNo,
		ProviderTradeID: req.ProviderTradeID,
		Status:          payment.PaymentSucceeded,
	}, nil
}

func (a *Adapter) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	a.mu.Lock()
	a.CloseRequests = append(a.CloseRequests, req)
	fn := a.closePayment
	a.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return nil
}

func (a *Adapter) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	a.mu.Lock()
	a.RefundRequests = append(a.RefundRequests, req)
	fn := a.refund
	a.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return &payment.RefundResponse{
		Provider:    req.Provider,
		Channel:     req.Channel,
		MerchantKey: req.MerchantKey,
		PaymentID:   req.PaymentID,
		OrderID:     req.OrderID,
		RefundID:    req.RefundID,
		Status:      payment.RefundProcessing,
		Amount:      req.Amount,
	}, nil
}

func (a *Adapter) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	a.mu.Lock()
	a.QueryRefundRequests = append(a.QueryRefundRequests, req)
	fn := a.queryRefund
	a.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return &payment.QueryRefundResponse{
		Provider:         req.Provider,
		Channel:          req.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		RefundID:         req.RefundID,
		OutRefundNo:      req.OutRefundNo,
		ProviderRefundID: req.ProviderRefundID,
		Status:           payment.RefundSucceeded,
	}, nil
}

func (a *Adapter) ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
	a.mu.Lock()
	a.NotifyRequests = append(a.NotifyRequests, req)
	fn := a.parseNotify
	a.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return &payment.NotifyResult{
		Verified: true,
		Event: &payment.PaymentEvent{
			Type:     payment.EventPaymentSucceeded,
			Provider: req.Provider,
			Channel:  req.Channel,
			Status:   payment.PaymentSucceeded,
		},
		Ack: payment.NotifyAck{
			StatusCode:  200,
			ContentType: "text/plain; charset=utf-8",
			Body:        []byte("success"),
		},
	}, nil
}

func (a *Adapter) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Snapshot{
		CreateRequests:      append([]payment.CreatePaymentRequest(nil), a.CreateRequests...),
		QueryRequests:       append([]payment.QueryPaymentRequest(nil), a.QueryRequests...),
		CloseRequests:       append([]payment.ClosePaymentRequest(nil), a.CloseRequests...),
		RefundRequests:      append([]payment.RefundRequest(nil), a.RefundRequests...),
		QueryRefundRequests: append([]payment.QueryRefundRequest(nil), a.QueryRefundRequests...),
		NotifyRequests:      append([]payment.NotifyRequest(nil), a.NotifyRequests...),
	}
}

type Snapshot struct {
	CreateRequests      []payment.CreatePaymentRequest
	QueryRequests       []payment.QueryPaymentRequest
	CloseRequests       []payment.ClosePaymentRequest
	RefundRequests      []payment.RefundRequest
	QueryRefundRequests []payment.QueryRefundRequest
	NotifyRequests      []payment.NotifyRequest
}
