package wechat_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	paymentwechat "github.com/huwenlong92/sdkit/pkg/payment/wechat"
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

func TestWechatAdapterCapabilities(t *testing.T) {
	adapter, err := paymentwechat.NewAdapter(paymentwechat.Config{ClientMode: paymentwechat.ClientModeStatic, Client: &client{}})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	caps := adapter.Capabilities()
	if caps.Provider != payment.ProviderWechat {
		t.Fatalf("provider = %q, want wechat", caps.Provider)
	}
	for _, channel := range []payment.Channel{
		payment.ChannelWechatApp,
		payment.ChannelWechatMiniProgram,
		payment.ChannelWechatH5,
		payment.ChannelWechatNative,
	} {
		if !hasChannel(caps.Channels, channel) {
			t.Fatalf("channels = %+v, missing %s", caps.Channels, channel)
		}
	}
	for _, action := range []payment.ActionType{
		payment.ActionSDKParams,
		payment.ActionRedirectURL,
		payment.ActionQRCode,
	} {
		if !hasAction(caps.SupportedActions, action) {
			t.Fatalf("actions = %+v, missing %s", caps.SupportedActions, action)
		}
	}
}

func TestWechatAdapterCreatePaymentChannels(t *testing.T) {
	for _, tt := range []struct {
		name       string
		channel    payment.Channel
		actionType payment.ActionType
	}{
		{name: "app", channel: payment.ChannelWechatApp, actionType: payment.ActionSDKParams},
		{name: "mini program", channel: payment.ChannelWechatMiniProgram, actionType: payment.ActionSDKParams},
		{name: "h5", channel: payment.ChannelWechatH5, actionType: payment.ActionRedirectURL},
		{name: "native", channel: payment.ChannelWechatNative, actionType: payment.ActionQRCode},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &client{}
			adapter, err := paymentwechat.NewAdapter(paymentwechat.Config{ClientMode: paymentwechat.ClientModeStatic, Client: client})
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
				Provider:   payment.ProviderWechat,
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

func TestWechatAdapterRejectsChannelActionMismatch(t *testing.T) {
	adapter, err := paymentwechat.NewAdapter(paymentwechat.Config{
		ClientMode: paymentwechat.ClientModeStatic,
		Client: &client{action: payment.PaymentAction{
			Type: payment.ActionRedirectURL,
			URL:  "https://pay.example.test/h5",
		}},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatNative,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrInvalidAction) {
		t.Fatalf("err = %v, want ErrInvalidAction", err)
	}
}

func TestWechatAdapterDynamicClientLoaderCleansUp(t *testing.T) {
	calls := 0
	cleanups := 0
	adapter, err := paymentwechat.NewAdapter(paymentwechat.Config{
		ClientLoader: paymentwechat.ClientLoaderFunc(func(ctx context.Context, merchantKey string) (paymentwechat.Client, paymentwechat.ClientCleanup, error) {
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
		Provider:    payment.ProviderWechat,
		Channel:     payment.ChannelWechatMiniProgram,
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
	case payment.ChannelWechatApp, payment.ChannelWechatMiniProgram:
		return payment.PaymentAction{
			Type:   payment.ActionSDKParams,
			Params: map[string]any{"prepay_id": "wx_prepay_1"},
		}
	case payment.ChannelWechatH5:
		return payment.PaymentAction{
			Type: payment.ActionRedirectURL,
			URL:  "https://wxpay.example.test/h5",
		}
	case payment.ChannelWechatNative:
		return payment.PaymentAction{
			Type: payment.ActionQRCode,
			URL:  "weixin://wxpay/bizpayurl?pr=abc",
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
