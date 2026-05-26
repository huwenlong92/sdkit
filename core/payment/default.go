package payment

import (
	"context"
	"sync"
)

var (
	defaultMu      sync.RWMutex
	defaultService *Service
)

func SetDefault(service *Service) {
	defaultMu.Lock()
	defaultService = service
	defaultMu.Unlock()
}

func Default() (*Service, error) {
	defaultMu.RLock()
	service := defaultService
	defaultMu.RUnlock()
	if service == nil {
		return nil, ErrAdapterNotFound
	}
	return service, nil
}

func CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResponse, error) {
	service, err := Default()
	if err != nil {
		return nil, err
	}
	return service.CreatePayment(ctx, req)
}

func QueryPayment(ctx context.Context, req QueryPaymentRequest) (*QueryPaymentResponse, error) {
	service, err := Default()
	if err != nil {
		return nil, err
	}
	return service.QueryPayment(ctx, req)
}

func ClosePayment(ctx context.Context, req ClosePaymentRequest) error {
	service, err := Default()
	if err != nil {
		return err
	}
	return service.ClosePayment(ctx, req)
}

func Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	service, err := Default()
	if err != nil {
		return nil, err
	}
	return service.Refund(ctx, req)
}

func QueryRefund(ctx context.Context, req QueryRefundRequest) (*QueryRefundResponse, error) {
	service, err := Default()
	if err != nil {
		return nil, err
	}
	return service.QueryRefund(ctx, req)
}

func HandleNotify(ctx context.Context, req NotifyRequest) (*NotifyResult, error) {
	service, err := Default()
	if err != nil {
		return nil, err
	}
	return service.HandleNotify(ctx, req)
}

func ReloadChannels(bindings []ChannelBinding) error {
	service, err := Default()
	if err != nil {
		return err
	}
	return service.ReloadChannels(bindings)
}

func Close() {
	SetDefault(nil)
}
