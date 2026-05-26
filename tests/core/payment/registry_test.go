package payment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
)

type fakeAdapter struct {
	name payment.Provider
}

func (a fakeAdapter) Name() payment.Provider {
	return a.name
}

func (a fakeAdapter) Capabilities() payment.Capabilities {
	return payment.Capabilities{Provider: a.name}
}

func (a fakeAdapter) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, payment.ErrUnsupportedCapability
}

func (a fakeAdapter) QueryPayment(context.Context, payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	return nil, payment.ErrUnsupportedCapability
}

func (a fakeAdapter) ClosePayment(context.Context, payment.ClosePaymentRequest) error {
	return payment.ErrUnsupportedCapability
}

func (a fakeAdapter) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, payment.ErrUnsupportedCapability
}

func (a fakeAdapter) QueryRefund(context.Context, payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	return nil, payment.ErrUnsupportedCapability
}

func (a fakeAdapter) ParseNotify(context.Context, payment.NotifyRequest) (*payment.NotifyResult, error) {
	return nil, payment.ErrUnsupportedCapability
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	registry := payment.NewRegistry()
	adapter := fakeAdapter{name: payment.ProviderWechat}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := registry.Adapter(payment.ProviderWechat)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if got.Name() != payment.ProviderWechat {
		t.Fatalf("provider = %q", got.Name())
	}
}

func TestRegistryRejectsDuplicateAdapter(t *testing.T) {
	registry := payment.NewRegistry()
	if err := registry.Register(fakeAdapter{name: payment.ProviderWechat}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	err := registry.Register(fakeAdapter{name: payment.ProviderWechat})
	if !errors.Is(err, payment.ErrAdapterAlreadyExists) {
		t.Fatalf("err = %v, want ErrAdapterAlreadyExists", err)
	}
}

func TestRegistryReturnsAdapterCopy(t *testing.T) {
	registry := payment.NewRegistry()
	if err := registry.Register(fakeAdapter{name: payment.ProviderWechat}); err != nil {
		t.Fatalf("register: %v", err)
	}
	adapters := registry.Adapters()
	delete(adapters, payment.ProviderWechat)

	if _, err := registry.Adapter(payment.ProviderWechat); err != nil {
		t.Fatalf("adapter after mutating copy: %v", err)
	}
}
