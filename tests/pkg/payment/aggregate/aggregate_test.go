package aggregate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	paymentaggregate "github.com/huwenlong92/sdkit/pkg/payment/aggregate"
)

type gateway struct {
	createReq payment.CreatePaymentRequest
	notifyReq payment.NotifyRequest
	action    payment.PaymentAction
}

type fullGateway struct {
	gateway

	queryReq       payment.QueryPaymentRequest
	closeReq       payment.ClosePaymentRequest
	refundReq      payment.RefundRequest
	queryRefundReq payment.QueryRefundRequest
}

func (g *gateway) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	g.createReq = req
	action := g.action
	if action.Type == "" {
		action = payment.PaymentAction{
			Type:   payment.ActionHTMLForm,
			URL:    "https://pay.example.test/form",
			Fields: map[string]string{"token": "abc"},
		}
	}
	return &payment.CreatePaymentResponse{
		Provider:    req.Provider,
		Channel:     req.Channel,
		MerchantKey: req.MerchantKey,
		OutTradeNo:  req.OutTradeNo,
		Status:      payment.PaymentPending,
		Pricing:     req.Pricing,
		Action:      action,
	}, nil
}

func (g *gateway) ParseNotify(_ context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
	g.notifyReq = req
	return &payment.NotifyResult{
		Verified: true,
		Event: &payment.PaymentEvent{
			Type:       payment.EventPaymentSucceeded,
			Provider:   payment.ProviderAggregate,
			Channel:    req.Channel,
			OutTradeNo: "pay_1001",
			Status:     payment.PaymentSucceeded,
		},
		Ack: payment.NotifyAck{StatusCode: 200, Body: []byte("success")},
	}, nil
}

func (g *fullGateway) QueryPayment(_ context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	g.queryReq = req
	return &payment.QueryPaymentResponse{
		Provider:   req.Provider,
		Channel:    req.Channel,
		OutTradeNo: req.OutTradeNo,
		Status:     payment.PaymentSucceeded,
	}, nil
}

func (g *fullGateway) ClosePayment(_ context.Context, req payment.ClosePaymentRequest) error {
	g.closeReq = req
	return nil
}

func (g *fullGateway) Refund(_ context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	g.refundReq = req
	return &payment.RefundResponse{
		Provider: req.Provider,
		Channel:  req.Channel,
		RefundID: req.RefundID,
		Status:   payment.RefundProcessing,
		Amount:   req.Amount,
	}, nil
}

func (g *fullGateway) QueryRefund(_ context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	g.queryRefundReq = req
	return &payment.QueryRefundResponse{
		Provider: req.Provider,
		Channel:  req.Channel,
		RefundID: req.RefundID,
		Status:   payment.RefundSucceeded,
	}, nil
}

func TestAggregateAdapterRoutesCreatePaymentByMerchantKey(t *testing.T) {
	cqu := &gateway{}
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu_gateway": cqu,
		},
	})
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
		Provider:    payment.ProviderAggregate,
		Channel:     payment.ChannelAggregateForm,
		MerchantKey: "cqu_gateway",
		OutTradeNo:  "pay_1001",
		Pricing:     payment.CNY(100),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Type != payment.ActionHTMLForm || resp.Action.Method != "POST" {
		t.Fatalf("action = %+v", resp.Action)
	}
	if cqu.createReq.Pricing.PayAmount != (payment.Money{Amount: 100, Currency: "CNY"}) {
		t.Fatalf("create req pricing = %+v", cqu.createReq.Pricing)
	}
}

func TestAggregateAdapterDefaultCapabilities(t *testing.T) {
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": &gateway{},
		},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	caps := adapter.Capabilities()
	if caps.Provider != payment.ProviderAggregate {
		t.Fatalf("provider = %q, want aggregate", caps.Provider)
	}
	if len(caps.Channels) != 1 || caps.Channels[0] != payment.ChannelAggregateForm {
		t.Fatalf("channels = %+v", caps.Channels)
	}
	if len(caps.SupportedCurrencies) != 1 || caps.SupportedCurrencies[0] != payment.DefaultCurrency {
		t.Fatalf("currencies = %+v", caps.SupportedCurrencies)
	}
	if !caps.SupportsMultiMerchant {
		t.Fatalf("supports multi merchant = false")
	}
	if !hasAction(caps.SupportedActions, payment.ActionHTMLForm) ||
		!hasAction(caps.SupportedActions, payment.ActionRedirectURL) ||
		!hasAction(caps.SupportedActions, payment.ActionQRCode) ||
		!hasAction(caps.SupportedActions, payment.ActionSDKParams) {
		t.Fatalf("actions = %+v", caps.SupportedActions)
	}
}

func TestAggregateAdapterUsesDefaultGateway(t *testing.T) {
	defaultGateway := &gateway{}
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		DefaultGateway: "default",
		Gateways: map[string]paymentaggregate.Gateway{
			"default": defaultGateway,
			"cqu":     &gateway{},
		},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if defaultGateway.createReq.OutTradeNo != "pay_1001" {
		t.Fatalf("default gateway was not selected: %+v", defaultGateway.createReq)
	}
}

func TestAggregateAdapterMerchantKeyWinsOverExtraGateway(t *testing.T) {
	merchantGateway := &gateway{}
	extraGateway := &gateway{}
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"merchant": merchantGateway,
			"extra":    extraGateway,
		},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderAggregate,
		Channel:     payment.ChannelAggregateForm,
		MerchantKey: "merchant",
		OutTradeNo:  "pay_1001",
		Pricing:     payment.CNY(100),
		Extra: map[string]any{
			paymentaggregate.ExtraGatewayKey: "extra",
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if merchantGateway.createReq.OutTradeNo != "pay_1001" {
		t.Fatalf("merchant gateway was not selected: %+v", merchantGateway.createReq)
	}
	if extraGateway.createReq.OutTradeNo != "" {
		t.Fatalf("extra gateway should not be selected: %+v", extraGateway.createReq)
	}
}

func TestAggregateAdapterRoutesByExtraGatewayWhenMerchantKeyDoesNotMatch(t *testing.T) {
	cqu := &gateway{}
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		DefaultGateway: "default",
		Gateways: map[string]paymentaggregate.Gateway{
			"default": &gateway{},
			"cqu":     cqu,
		},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderAggregate,
		Channel:     payment.ChannelAggregateForm,
		MerchantKey: "unknown_merchant",
		OutTradeNo:  "pay_1001",
		Pricing:     payment.CNY(100),
		Extra: map[string]any{
			paymentaggregate.ExtraGatewayKey: "cqu",
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if cqu.createReq.OutTradeNo != "pay_1001" {
		t.Fatalf("gateway was not selected: %+v", cqu.createReq)
	}
}

func TestAggregateAdapterNotifyRoutesByQueryGateway(t *testing.T) {
	cqu := &gateway{}
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": cqu,
		},
		Capabilities: payment.Capabilities{
			SupportsNotify: true,
		},
	})
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

	result, err := svc.HandleNotify(context.Background(), payment.NotifyRequest{
		Provider: payment.ProviderAggregate,
		Channel:  payment.ChannelAggregateForm,
		Query: map[string][]string{
			paymentaggregate.ExtraGatewayKey: {"cqu"},
		},
		Body: []byte(`{"id":"evt_1"}`),
	})
	if err != nil {
		t.Fatalf("handle notify: %v", err)
	}
	if !result.Verified || result.Event == nil || result.Event.OutTradeNo != "pay_1001" {
		t.Fatalf("notify result = %+v", result)
	}
	if string(cqu.notifyReq.Body) != `{"id":"evt_1"}` {
		t.Fatalf("notify req = %+v", cqu.notifyReq)
	}
}

func TestAggregateAdapterNotifyRoutesByFormGateway(t *testing.T) {
	cqu := &gateway{}
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": cqu,
		},
		Capabilities: payment.Capabilities{
			SupportsNotify: true,
		},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	result, err := adapter.ParseNotify(context.Background(), payment.NotifyRequest{
		Provider: payment.ProviderAggregate,
		Channel:  payment.ChannelAggregateForm,
		Form: map[string][]string{
			paymentaggregate.ExtraGatewayKey: {"cqu"},
		},
		Body: []byte(`{"id":"evt_1"}`),
	})
	if err != nil {
		t.Fatalf("parse notify: %v", err)
	}
	if !result.Verified || cqu.notifyReq.Channel != payment.ChannelAggregateForm {
		t.Fatalf("notify result = %+v, req = %+v", result, cqu.notifyReq)
	}
}

func TestAggregateAdapterOptionalCapabilitySuccessPaths(t *testing.T) {
	cqu := &fullGateway{}
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": cqu,
		},
		Capabilities: payment.Capabilities{
			SupportsQuery:       true,
			SupportsClose:       true,
			SupportsRefund:      true,
			SupportsQueryRefund: true,
		},
	})
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

	queryResp, err := svc.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:    payment.ProviderAggregate,
		Channel:     payment.ChannelAggregateForm,
		MerchantKey: "cqu",
		OutTradeNo:  "pay_1001",
	})
	if err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if queryResp.Status != payment.PaymentSucceeded || cqu.queryReq.OutTradeNo != "pay_1001" {
		t.Fatalf("query response = %+v, req = %+v", queryResp, cqu.queryReq)
	}

	err = svc.ClosePayment(context.Background(), payment.ClosePaymentRequest{
		Provider:    payment.ProviderAggregate,
		Channel:     payment.ChannelAggregateForm,
		MerchantKey: "cqu",
		OutTradeNo:  "pay_1001",
	})
	if err != nil {
		t.Fatalf("close payment: %v", err)
	}
	if cqu.closeReq.OutTradeNo != "pay_1001" {
		t.Fatalf("close req = %+v", cqu.closeReq)
	}

	refundResp, err := svc.Refund(context.Background(), payment.RefundRequest{
		Provider:    payment.ProviderAggregate,
		Channel:     payment.ChannelAggregateForm,
		MerchantKey: "cqu",
		PaymentID:   "pay_1001",
		RefundID:    "refund_1001",
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 50},
		},
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refundResp.Amount.Refund.Currency != "CNY" || cqu.refundReq.Amount.Refund.Currency != "CNY" {
		t.Fatalf("refund response = %+v, req = %+v", refundResp, cqu.refundReq)
	}

	refundQueryResp, err := svc.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider:    payment.ProviderAggregate,
		Channel:     payment.ChannelAggregateForm,
		MerchantKey: "cqu",
		RefundID:    "refund_1001",
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if refundQueryResp.Status != payment.RefundSucceeded || cqu.queryRefundReq.RefundID != "refund_1001" {
		t.Fatalf("query refund response = %+v, req = %+v", refundQueryResp, cqu.queryRefundReq)
	}
}

func TestAggregateAdapterServiceRejectsUnsupportedAction(t *testing.T) {
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": &gateway{action: payment.PaymentAction{
				Type: payment.ActionQRCode,
				URL:  "weixin://qr",
			}},
		},
		Capabilities: payment.Capabilities{
			SupportedActions: []payment.ActionType{payment.ActionHTMLForm},
		},
	})
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

	_, err = svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestAggregateAdapterServiceRejectsUnsupportedChannelAndCurrency(t *testing.T) {
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": &gateway{},
		},
		Capabilities: payment.Capabilities{
			Channels:            []payment.Channel{payment.ChannelAggregateForm},
			SupportedCurrencies: []string{"CNY"},
		},
	})
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

	_, err = svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAlipayPage,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
	})
	if !errors.Is(err, payment.ErrUnsupportedChannel) {
		t.Fatalf("err = %v, want ErrUnsupportedChannel", err)
	}

	_, err = svc.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount:      payment.Money{Amount: 100, Currency: "EUR"},
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

func TestAggregateAdapterReturnsUnsupportedCapability(t *testing.T) {
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": &gateway{},
		},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
	})
	if !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestAggregateAdapterRejectsUnknownGateway(t *testing.T) {
	adapter, err := paymentaggregate.New(paymentaggregate.Config{
		Gateways: map[string]paymentaggregate.Gateway{
			"cqu": &gateway{},
		},
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderAggregate,
		Channel:    payment.ChannelAggregateForm,
		OutTradeNo: "pay_1001",
		Pricing:    payment.CNY(100),
		Extra: map[string]any{
			paymentaggregate.ExtraGatewayKey: "unknown",
		},
	})
	if !errors.Is(err, payment.ErrAdapterNotFound) {
		t.Fatalf("err = %v, want ErrAdapterNotFound", err)
	}
}

func hasAction(actions []payment.ActionType, target payment.ActionType) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}
