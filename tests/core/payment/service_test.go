package payment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
)

type serviceAdapter struct {
	name       payment.Provider
	caps       payment.Capabilities
	createReq  payment.CreatePaymentRequest
	refundReq  payment.RefundRequest
	queryResp  *payment.QueryPaymentResponse
	actionType payment.ActionType
	action     *payment.PaymentAction
	notify     *payment.NotifyResult
}

func (a *serviceAdapter) Name() payment.Provider {
	return a.name
}

func (a *serviceAdapter) Capabilities() payment.Capabilities {
	return a.caps
}

func (a *serviceAdapter) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	a.createReq = req
	action := payment.PaymentAction{Type: a.actionType}
	if a.action != nil {
		action = *a.action
	}
	switch a.actionType {
	case payment.ActionRedirectURL:
		action.URL = "https://pay.example.test"
	case payment.ActionHTMLForm:
		action.HTML = "<form></form>"
	case payment.ActionQRCode:
		action.URL = "weixin://qr"
	case payment.ActionSDKParams:
		action.Params = map[string]any{"prepay_id": "prepay_1"}
	case payment.ActionClientToken:
		action.Token = "client_secret_1"
	}
	return &payment.CreatePaymentResponse{
		Provider: req.Provider,
		Channel:  req.Channel,
		Status:   payment.PaymentPending,
		Action:   action,
	}, nil
}

func (a *serviceAdapter) QueryPayment(context.Context, payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if a.queryResp != nil {
		return a.queryResp, nil
	}
	return &payment.QueryPaymentResponse{Status: payment.PaymentSucceeded}, nil
}

func (a *serviceAdapter) ClosePayment(context.Context, payment.ClosePaymentRequest) error {
	return nil
}

func (a *serviceAdapter) Refund(_ context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	a.refundReq = req
	return &payment.RefundResponse{Status: payment.RefundProcessing, Amount: req.Amount}, nil
}

func (a *serviceAdapter) QueryRefund(context.Context, payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	return &payment.QueryRefundResponse{Status: payment.RefundSucceeded}, nil
}

func (a *serviceAdapter) ParseNotify(context.Context, payment.NotifyRequest) (*payment.NotifyResult, error) {
	if a.notify != nil {
		return a.notify, nil
	}
	return &payment.NotifyResult{
		Verified: true,
		Event: &payment.PaymentEvent{
			Type:   payment.EventPaymentSucceeded,
			Status: payment.PaymentSucceeded,
		},
		Ack: payment.NotifyAck{StatusCode: 200},
	}, nil
}

func newServiceForTest(t *testing.T, adapter *serviceAdapter) *payment.Service {
	t.Helper()
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestServiceCreatePaymentNormalizesPricing(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{
			Provider:            payment.ProviderWechat,
			Channels:            []payment.Channel{payment.ChannelWechatMiniProgram},
			SupportedCurrencies: []string{"CNY"},
			SupportedActions:    []payment.ActionType{payment.ActionSDKParams},
		},
		actionType: payment.ActionSDKParams,
	}
	svc := newServiceForTest(t, adapter)

	resp, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatMiniProgram,
		OutTradeNo: "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 19900},
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if adapter.createReq.Pricing.PayAmount != (payment.Money{Amount: 19900, Currency: "CNY"}) {
		t.Fatalf("adapter pricing = %+v", adapter.createReq.Pricing)
	}
	if resp.Pricing.SettleAmount != (payment.Money{Amount: 19900, Currency: "CNY"}) {
		t.Fatalf("response pricing = %+v", resp.Pricing)
	}
}

func TestServiceCreatePaymentSelectsChannelFromMerchantKey(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{
			Provider:         payment.ProviderWechat,
			Channels:         []payment.Channel{payment.ChannelWechatMiniProgram},
			SupportedActions: []payment.ActionType{payment.ActionSDKParams},
		},
		actionType: payment.ActionSDKParams,
	}
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	selector, err := payment.NewStaticChannelSelector([]payment.ChannelBinding{{
		Key:         "school_a_p",
		Provider:    payment.ProviderWechat,
		Channel:     payment.ChannelWechatMiniProgram,
		MerchantKey: "school_a",
	}})
	if err != nil {
		t.Fatalf("new selector: %v", err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{
		Registry:        registry,
		ChannelSelector: selector,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		MerchantKey: "school_a_p",
		OutTradeNo:  "pay_1001",
		Pricing:     payment.CNY(100),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if adapter.createReq.Provider != payment.ProviderWechat ||
		adapter.createReq.Channel != payment.ChannelWechatMiniProgram ||
		adapter.createReq.MerchantKey != "school_a" {
		t.Fatalf("adapter request = %+v", adapter.createReq)
	}
	if resp.Provider != payment.ProviderWechat || resp.Channel != payment.ChannelWechatMiniProgram || resp.MerchantKey != "school_a" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestServiceCreatePaymentFallsBackToExplicitChannelWhenSelectorMisses(t *testing.T) {
	adapter := &serviceAdapter{
		name:       payment.ProviderWechat,
		caps:       payment.Capabilities{Provider: payment.ProviderWechat},
		actionType: payment.ActionSDKParams,
	}
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	selector, err := payment.NewStaticChannelSelector(nil)
	if err != nil {
		t.Fatalf("new selector: %v", err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{
		Registry:        registry,
		ChannelSelector: selector,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderWechat,
		Channel:     payment.ChannelWechatApp,
		MerchantKey: "direct_school_a",
		OutTradeNo:  "pay_1001",
		Pricing:     payment.CNY(100),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Provider != payment.ProviderWechat || resp.Channel != payment.ChannelWechatApp || resp.MerchantKey != "direct_school_a" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestServiceCreatePaymentRejectsUnsupportedChannel(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{
			Provider: payment.ProviderWechat,
			Channels: []payment.Channel{payment.ChannelWechatApp},
		},
		actionType: payment.ActionSDKParams,
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatMiniProgram,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrUnsupportedChannel) {
		t.Fatalf("err = %v, want ErrUnsupportedChannel", err)
	}
}

func TestServiceCreatePaymentRequiresPaymentReference(t *testing.T) {
	adapter := &serviceAdapter{
		name:       payment.ProviderWechat,
		caps:       payment.Capabilities{Provider: payment.ProviderWechat},
		actionType: payment.ActionSDKParams,
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider: payment.ProviderWechat,
		Channel:  payment.ChannelWechatApp,
		Pricing:  payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestServiceCreatePaymentRejectsExpiredRequest(t *testing.T) {
	adapter := &serviceAdapter{
		name:       payment.ProviderWechat,
		caps:       payment.Capabilities{Provider: payment.ProviderWechat},
		actionType: payment.ActionSDKParams,
	}
	svc := newServiceForTest(t, adapter)
	expireAt := time.Now().Add(-time.Minute)

	_, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
		ExpireAt:   &expireAt,
	})
	if !errors.Is(err, payment.ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestServiceCreatePaymentRejectsUnsupportedCurrency(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderStripe,
		caps: payment.Capabilities{
			Provider:            payment.ProviderStripe,
			Channels:            []payment.Channel{payment.ChannelStripeCheckout},
			SupportedCurrencies: []string{"CNY"},
		},
		actionType: payment.ActionRedirectURL,
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderStripe,
		Channel:    payment.ChannelStripeCheckout,
		OutTradeNo: "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount:      payment.Money{Amount: 1000, Currency: "EUR"},
			SettleCurrency: "CNY",
			ExchangeRate: &payment.ExchangeRateSnapshot{
				FromCurrency: "EUR",
				ToCurrency:   "CNY",
				Rate:         "7.8",
			},
		},
	})
	if !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestServiceCreatePaymentRejectsInvalidActionPayload(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderAggregate,
		caps: payment.Capabilities{
			Provider:         payment.ProviderAggregate,
			Channels:         []payment.Channel{payment.ChannelAggregateForm},
			SupportedActions: []payment.ActionType{payment.ActionRedirectURL},
		},
		action: &payment.PaymentAction{Type: payment.ActionRedirectURL},
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrInvalidAction) {
		t.Fatalf("err = %v, want ErrInvalidAction", err)
	}
}

func TestServiceCreatePaymentDefaultsHTMLFormMethod(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderAggregate,
		caps: payment.Capabilities{
			Provider:         payment.ProviderAggregate,
			Channels:         []payment.Channel{payment.ChannelAggregateForm},
			SupportedActions: []payment.ActionType{payment.ActionHTMLForm},
		},
		action: &payment.PaymentAction{
			Type:   payment.ActionHTMLForm,
			URL:    "https://pay.example.test/form",
			Fields: map[string]string{"token": "abc"},
		},
	}
	svc := newServiceForTest(t, adapter)

	resp, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Method != "POST" {
		t.Fatalf("method = %q, want POST", resp.Action.Method)
	}
}

func TestServiceCreatePaymentRejectsUnsupportedAction(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderAggregate,
		caps: payment.Capabilities{
			Provider:         payment.ProviderAggregate,
			Channels:         []payment.Channel{payment.ChannelAggregateForm},
			SupportedActions: []payment.ActionType{payment.ActionHTMLForm},
		},
		actionType: payment.ActionRedirectURL,
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestServiceCreatePaymentNormalizesEmptyActionToNone(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderAggregate,
		caps: payment.Capabilities{
			Provider:         payment.ProviderAggregate,
			Channels:         []payment.Channel{payment.ChannelAggregateForm},
			SupportedActions: []payment.ActionType{payment.ActionNone},
		},
	}
	svc := newServiceForTest(t, adapter)

	resp, err := svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Type != payment.ActionNone {
		t.Fatalf("action type = %q, want ActionNone", resp.Action.Type)
	}
}

func TestServiceRefundNormalizesAmount(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{
			Provider:       payment.ProviderWechat,
			Channels:       []payment.Channel{payment.ChannelWechatApp},
			SupportsRefund: true,
		},
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.Refund(context.Background(), payment.RefundRequest{
		Provider:  payment.ProviderWechat,
		Channel:   payment.ChannelWechatApp,
		PaymentID: "pay_1001",
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 100},
		},
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if adapter.refundReq.Amount.Refund != (payment.Money{Amount: 100, Currency: "CNY"}) {
		t.Fatalf("refund amount = %+v", adapter.refundReq.Amount)
	}
	if adapter.refundReq.Amount.Settle != adapter.refundReq.Amount.Refund {
		t.Fatalf("settle amount = %+v, want refund amount", adapter.refundReq.Amount.Settle)
	}
}

func TestServiceQueryRefundRequiresQueryRefundCapability(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{
			Provider:       payment.ProviderWechat,
			Channels:       []payment.Channel{payment.ChannelWechatApp},
			SupportsRefund: true,
		},
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider: payment.ProviderWechat,
		Channel:  payment.ChannelWechatApp,
		RefundID: "refund_1",
	})
	if !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestServiceQueryRefundFillsDefaults(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{
			Provider:            payment.ProviderWechat,
			Channels:            []payment.Channel{payment.ChannelWechatApp},
			SupportsQueryRefund: true,
		},
	}
	svc := newServiceForTest(t, adapter)

	resp, err := svc.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider:    payment.ProviderWechat,
		Channel:     payment.ChannelWechatApp,
		PaymentID:   "pay_1",
		RefundID:    "refund_1",
		OutRefundNo: "out_refund_1",
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if resp.Provider != payment.ProviderWechat || resp.RefundID != "refund_1" || resp.OutRefundNo != "out_refund_1" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestServiceQueryRequiresCapability(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{Provider: payment.ProviderWechat},
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:   payment.ProviderWechat,
		Channel:    payment.ChannelWechatApp,
		OutTradeNo: "pay_1001",
	})
	if !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestServiceHandleNotifyRejectsUnverifiedResult(t *testing.T) {
	adapter := &serviceAdapter{
		name: payment.ProviderWechat,
		caps: payment.Capabilities{
			Provider:       payment.ProviderWechat,
			Channels:       []payment.Channel{payment.ChannelWechatApp},
			SupportsNotify: true,
		},
		notify: &payment.NotifyResult{Verified: false},
	}
	svc := newServiceForTest(t, adapter)

	_, err := svc.HandleNotify(context.Background(), payment.NotifyRequest{
		Provider: payment.ProviderWechat,
		Channel:  payment.ChannelWechatApp,
	})
	if !errors.Is(err, payment.ErrNotifyVerificationFail) {
		t.Fatalf("err = %v, want ErrNotifyVerificationFail", err)
	}
}
