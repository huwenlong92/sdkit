package payment_test

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/mock"
)

func TestDefaultServiceCreatePayment(t *testing.T) {
	t.Cleanup(payment.Close)

	registry := payment.NewRegistry()
	adapter := mock.New(
		payment.ProviderStripe,
		mock.WithChannels(payment.ChannelStripeCheckout),
		mock.WithCurrencies("USD"),
		mock.WithActions(payment.ActionRedirectURL),
		mock.WithAction(payment.PaymentAction{
			Type: payment.ActionRedirectURL,
			URL:  "https://pay.example.test/checkout",
		}),
	)
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	service, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	payment.SetDefault(service)

	resp, err := payment.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderStripe,
		Channel:     payment.ChannelStripeCheckout,
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
	if resp.MerchantKey != "school_a" || resp.Action.URL == "" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestDefaultServiceReloadChannels(t *testing.T) {
	t.Cleanup(payment.Close)

	registry := payment.NewRegistry()
	adapter := mock.New(
		payment.ProviderStripe,
		mock.WithChannels(payment.ChannelStripeCheckout),
		mock.WithCurrencies("USD"),
		mock.WithActions(payment.ActionRedirectURL),
		mock.WithAction(payment.PaymentAction{
			Type: payment.ActionRedirectURL,
			URL:  "https://pay.example.test/checkout",
		}),
	)
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	selector, err := payment.NewStaticChannelSelector([]payment.ChannelBinding{
		{
			Key:         "old_p",
			Provider:    payment.ProviderStripe,
			Channel:     payment.ChannelStripeCheckout,
			MerchantKey: "old",
		},
	})
	if err != nil {
		t.Fatalf("new selector: %v", err)
	}
	service, err := payment.NewService(payment.ServiceConfig{
		Registry:        registry,
		ChannelSelector: selector,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	payment.SetDefault(service)

	if err := payment.ReloadChannels([]payment.ChannelBinding{
		{
			Key:         "school_a_p",
			Provider:    payment.ProviderStripe,
			Channel:     payment.ChannelStripeCheckout,
			MerchantKey: "school_a",
		},
	}); err != nil {
		t.Fatalf("reload channels: %v", err)
	}

	resp, err := payment.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		MerchantKey: "school_a_p",
		OutTradeNo:  "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount:      payment.Money{Amount: 1200, Currency: "USD"},
			SettleCurrency: "USD",
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.MerchantKey != "school_a" {
		t.Fatalf("response = %+v", resp)
	}
}
