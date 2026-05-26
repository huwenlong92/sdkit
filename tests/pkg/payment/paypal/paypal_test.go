package paypal_test

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/paypal"
)

type client struct{}

func (c *client) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return &payment.CreatePaymentResponse{
		Provider:        req.Provider,
		Channel:         req.Channel,
		ProviderTradeID: "paypal_order_1",
		Status:          payment.PaymentPending,
		Pricing:         req.Pricing,
		Action:          payment.PaymentAction{Type: payment.ActionRedirectURL, URL: "https://paypal.example.test/checkout"},
	}, nil
}

func (c *client) QueryPayment(context.Context, payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	return &payment.QueryPaymentResponse{Status: payment.PaymentPending}, nil
}

func (c *client) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return &payment.RefundResponse{Status: payment.RefundSucceeded}, nil
}

func (c *client) QueryRefund(context.Context, payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	return &payment.QueryRefundResponse{Status: payment.RefundSucceeded}, nil
}

func TestPayPalAdapterCapabilities(t *testing.T) {
	adapter, err := paypal.NewAdapter(paypal.Config{ClientMode: paypal.ClientModeStatic, Client: &client{}})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	caps := adapter.Capabilities()
	if caps.Provider != payment.ProviderPayPal {
		t.Fatalf("provider = %q, want paypal", caps.Provider)
	}
	if len(caps.Channels) != 1 || caps.Channels[0] != payment.ChannelPayPalOrder {
		t.Fatalf("channels = %+v", caps.Channels)
	}
	if !caps.SupportsQuery || !caps.SupportsRefund || !caps.SupportsQueryRefund {
		t.Fatalf("capabilities = %+v", caps)
	}
}

func TestPayPalAdapterCreatePayment(t *testing.T) {
	adapter, err := paypal.NewAdapter(paypal.Config{ClientMode: paypal.ClientModeStatic, Client: &client{}})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderPayPal,
		Channel:    payment.ChannelPayPalOrder,
		OutTradeNo: "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount:      payment.Money{Amount: 1200, Currency: "USD"},
			SettleCurrency: "USD",
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Type != payment.ActionRedirectURL {
		t.Fatalf("action = %+v", resp.Action)
	}
}

func TestPayPalAdapterDynamicClientLoaderCleansUp(t *testing.T) {
	calls := 0
	cleanups := 0
	adapter, err := paypal.NewAdapter(paypal.Config{
		ClientLoader: paypal.ClientLoaderFunc(func(ctx context.Context, merchantKey string) (paypal.Client, paypal.ClientCleanup, error) {
			calls++
			if merchantKey != "school_a" {
				t.Fatalf("merchant key = %q", merchantKey)
			}
			return &client{}, func() error {
				cleanups++
				return nil
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderPayPal,
		Channel:     payment.ChannelPayPalOrder,
		MerchantKey: "school_a",
		OutTradeNo:  "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount:      payment.Money{Amount: 1200, Currency: "USD"},
			SettleCurrency: "USD",
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if calls != 1 || cleanups != 1 {
		t.Fatalf("calls=%d cleanups=%d", calls, cleanups)
	}
}
