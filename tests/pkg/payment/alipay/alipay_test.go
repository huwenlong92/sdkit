package alipay_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	paymentalipay "github.com/huwenlong92/sdkit/pkg/payment/alipay"
)

type client struct {
	createReq payment.CreatePaymentRequest
	action    payment.PaymentAction
}

func (c *client) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	c.createReq = req
	action := c.action
	if action.Type == "" {
		action = actionForChannel(req.Channel)
	}
	return &payment.CreatePaymentResponse{
		Provider:   req.Provider,
		Channel:    req.Channel,
		OutTradeNo: req.OutTradeNo,
		Status:     payment.PaymentPending,
		Pricing:    req.Pricing,
		Action:     action,
	}, nil
}

func TestAlipayAdapterCapabilities(t *testing.T) {
	adapter, err := paymentalipay.NewAdapter(paymentalipay.Config{ClientMode: paymentalipay.ClientModeStatic, Client: &client{}})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	caps := adapter.Capabilities()
	if caps.Provider != payment.ProviderAlipay {
		t.Fatalf("provider = %q, want alipay", caps.Provider)
	}
	for _, channel := range []payment.Channel{
		payment.ChannelAlipayApp,
		payment.ChannelAlipayWap,
		payment.ChannelAlipayPage,
	} {
		if !hasChannel(caps.Channels, channel) {
			t.Fatalf("channels = %+v, missing %s", caps.Channels, channel)
		}
	}
	for _, action := range []payment.ActionType{
		payment.ActionSDKParams,
		payment.ActionHTMLForm,
		payment.ActionRedirectURL,
	} {
		if !hasAction(caps.SupportedActions, action) {
			t.Fatalf("actions = %+v, missing %s", caps.SupportedActions, action)
		}
	}
}

func TestAlipayAdapterCreatePaymentChannels(t *testing.T) {
	for _, tt := range []struct {
		name       string
		channel    payment.Channel
		actionType payment.ActionType
	}{
		{name: "app", channel: payment.ChannelAlipayApp, actionType: payment.ActionSDKParams},
		{name: "wap", channel: payment.ChannelAlipayWap, actionType: payment.ActionHTMLForm},
		{name: "page", channel: payment.ChannelAlipayPage, actionType: payment.ActionHTMLForm},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &client{}
			adapter, err := paymentalipay.NewAdapter(paymentalipay.Config{ClientMode: paymentalipay.ClientModeStatic, Client: client})
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
				Provider:   payment.ProviderAlipay,
				Channel:    tt.channel,
				OutTradeNo: "pay_1001",
				Pricing:    payment.CNY(100),
			})
			if err != nil {
				t.Fatalf("create payment: %v", err)
			}
			if resp.Action.Type != tt.actionType {
				t.Fatalf("action type = %q, want %q", resp.Action.Type, tt.actionType)
			}
			if client.createReq.Pricing.PayAmount.Currency != "CNY" {
				t.Fatalf("pricing = %+v", client.createReq.Pricing)
			}
		})
	}
}

func TestAlipayAdapterAllowsRedirectForWapAndPage(t *testing.T) {
	for _, channel := range []payment.Channel{payment.ChannelAlipayWap, payment.ChannelAlipayPage} {
		t.Run(string(channel), func(t *testing.T) {
			adapter, err := paymentalipay.NewAdapter(paymentalipay.Config{
				ClientMode: paymentalipay.ClientModeStatic,
				Client: &client{action: payment.PaymentAction{
					Type: payment.ActionRedirectURL,
					URL:  "https://openapi.alipay.example.test/pay",
				}},
			})
			if err != nil {
				t.Fatalf("new adapter: %v", err)
			}

			_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				Provider:   payment.ProviderAlipay,
				Channel:    channel,
				OutTradeNo: "pay_1001",
				Pricing:    payment.CNY(100),
			})
			if err != nil {
				t.Fatalf("create payment: %v", err)
			}
		})
	}
}

func TestAlipayAdapterRejectsChannelActionMismatch(t *testing.T) {
	adapter, err := paymentalipay.NewAdapter(paymentalipay.Config{
		ClientMode: paymentalipay.ClientModeStatic,
		Client: &client{action: payment.PaymentAction{
			Type: payment.ActionQRCode,
			URL:  "https://qr.example.test",
		}},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAlipay,
		Channel:    payment.ChannelAlipayApp,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrInvalidAction) {
		t.Fatalf("err = %v, want ErrInvalidAction", err)
	}
}

func TestAlipayAdapterDynamicClientLoaderCleansUp(t *testing.T) {
	calls := 0
	cleanups := 0
	adapter, err := paymentalipay.NewAdapter(paymentalipay.Config{
		ClientLoader: paymentalipay.ClientLoaderFunc(func(ctx context.Context, merchantKey string) (paymentalipay.Client, paymentalipay.ClientCleanup, error) {
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
		Provider:    payment.ProviderAlipay,
		Channel:     payment.ChannelAlipayPage,
		MerchantKey: "school_a",
		OutTradeNo:  "pay_1001",
		Pricing:     payment.CNY(100),
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
	case payment.ChannelAlipayApp:
		return payment.PaymentAction{
			Type:   payment.ActionSDKParams,
			Params: map[string]any{"order_string": "alipay_sdk=abc"},
		}
	case payment.ChannelAlipayWap, payment.ChannelAlipayPage:
		return payment.PaymentAction{
			Type:   payment.ActionHTMLForm,
			URL:    "https://openapi.alipay.example.test/gateway.do",
			Fields: map[string]string{"biz_content": "{}"},
		}
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
