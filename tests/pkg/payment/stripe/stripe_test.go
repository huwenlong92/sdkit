//go:build sdkit_payment_stripe

package stripe_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/stripe"
)

type client struct {
	action payment.PaymentAction
}

func (c *client) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	action := c.action
	if action.Type == "" {
		action = actionForChannel(req.Channel)
	}
	return &payment.CreatePaymentResponse{
		Provider:        req.Provider,
		Channel:         req.Channel,
		ProviderTradeID: "stripe_trade_1",
		Status:          payment.PaymentPending,
		Pricing:         req.Pricing,
		Action:          action,
	}, nil
}

func (c *client) QueryPayment(context.Context, payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	return &payment.QueryPaymentResponse{Status: payment.PaymentPending}, nil
}

func (c *client) ClosePayment(context.Context, payment.ClosePaymentRequest) error {
	return nil
}

func (c *client) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return &payment.RefundResponse{Status: payment.RefundSucceeded}, nil
}

func (c *client) QueryRefund(context.Context, payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	return &payment.QueryRefundResponse{Status: payment.RefundSucceeded}, nil
}

func TestStripeAdapterCapabilities(t *testing.T) {
	adapter, err := stripe.NewAdapter(stripe.Config{ClientMode: stripe.ClientModeStatic, Client: &client{}})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	caps := adapter.Capabilities()
	if caps.Provider != payment.ProviderStripe {
		t.Fatalf("provider = %q, want stripe", caps.Provider)
	}
	for _, channel := range []payment.Channel{payment.ChannelStripeCheckout, payment.ChannelStripeIntent} {
		if !hasChannel(caps.Channels, channel) {
			t.Fatalf("channels = %+v, missing %s", caps.Channels, channel)
		}
	}
	for _, action := range []payment.ActionType{payment.ActionRedirectURL, payment.ActionClientToken} {
		if !hasAction(caps.SupportedActions, action) {
			t.Fatalf("actions = %+v, missing %s", caps.SupportedActions, action)
		}
	}
	if !caps.SupportsQuery || !caps.SupportsClose || !caps.SupportsRefund || !caps.SupportsQueryRefund {
		t.Fatalf("capabilities = %+v", caps)
	}
}

func TestStripeAdapterCreatePaymentChannels(t *testing.T) {
	for _, tt := range []struct {
		name       string
		channel    payment.Channel
		actionType payment.ActionType
	}{
		{name: "checkout", channel: payment.ChannelStripeCheckout, actionType: payment.ActionRedirectURL},
		{name: "intent", channel: payment.ChannelStripeIntent, actionType: payment.ActionClientToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := stripe.NewAdapter(stripe.Config{ClientMode: stripe.ClientModeStatic, Client: &client{}})
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
				Provider:   payment.ProviderStripe,
				Channel:    tt.channel,
				OutTradeNo: "pay_1001",
				Pricing: payment.PaymentPricing{
					PayAmount:      payment.Money{Amount: 1200, Currency: "usd"},
					SettleCurrency: "usd",
				},
			})
			if err != nil {
				t.Fatalf("create payment: %v", err)
			}
			if resp.Action.Type != tt.actionType {
				t.Fatalf("action = %+v, want %s", resp.Action, tt.actionType)
			}
			if resp.Pricing.PayAmount.Currency != "USD" {
				t.Fatalf("pricing = %+v", resp.Pricing)
			}
		})
	}
}

func TestStripeAdapterRejectsChannelActionMismatch(t *testing.T) {
	adapter, err := stripe.NewAdapter(stripe.Config{
		ClientMode: stripe.ClientModeStatic,
		Client:     &client{action: payment.PaymentAction{Type: payment.ActionQRCode, URL: "https://qr.example.test"}},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderStripe,
		Channel:    payment.ChannelStripeCheckout,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrInvalidAction) {
		t.Fatalf("err = %v, want ErrInvalidAction", err)
	}
}

func TestStripeAdapterDynamicClientLoaderCleansUp(t *testing.T) {
	calls := 0
	cleanups := 0
	adapter, err := stripe.NewAdapter(stripe.Config{
		ClientLoader: stripe.ClientLoaderFunc(func(ctx context.Context, merchantKey string) (stripe.Client, stripe.ClientCleanup, error) {
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
	if calls != 1 || cleanups != 1 {
		t.Fatalf("calls=%d cleanups=%d", calls, cleanups)
	}
}

func actionForChannel(channel payment.Channel) payment.PaymentAction {
	switch channel {
	case payment.ChannelStripeCheckout:
		return payment.PaymentAction{Type: payment.ActionRedirectURL, URL: "https://checkout.stripe.example.test/pay/cs_1"}
	case payment.ChannelStripeIntent:
		return payment.PaymentAction{Type: payment.ActionClientToken, Token: "pi_secret_1"}
	default:
		return payment.PaymentAction{Type: payment.ActionNone}
	}
}

func hasChannel(channels []payment.Channel, target payment.Channel) bool {
	for _, channel := range channels {
		if channel == target {
			return true
		}
	}
	return false
}

func hasAction(actions []payment.ActionType, target payment.ActionType) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}
