package channelrouter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/channelrouter"
)

type fakeAdapter struct {
	provider    payment.Provider
	channel     payment.Channel
	merchantKey string
	caps        payment.Capabilities
	createReq   payment.CreatePaymentRequest
}

func (a *fakeAdapter) Name() payment.Provider {
	return a.provider
}

func (a *fakeAdapter) Capabilities() payment.Capabilities {
	return a.caps
}

func (a *fakeAdapter) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	a.createReq = req
	return &payment.CreatePaymentResponse{
		Provider:        a.provider,
		Channel:         a.channel,
		MerchantKey:     a.merchantKey,
		ProviderTradeID: "trade_" + a.merchantKey,
		Status:          payment.PaymentPending,
		Pricing:         req.Pricing,
		Action:          payment.PaymentAction{Type: payment.ActionRedirectURL, URL: "https://pay.example.test/" + a.merchantKey},
	}, nil
}

func (a *fakeAdapter) QueryPayment(_ context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	return &payment.QueryPaymentResponse{Status: payment.PaymentPending}, nil
}

func (a *fakeAdapter) ClosePayment(context.Context, payment.ClosePaymentRequest) error {
	return nil
}

func (a *fakeAdapter) Refund(_ context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	return &payment.RefundResponse{Status: payment.RefundSucceeded}, nil
}

func (a *fakeAdapter) QueryRefund(_ context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	return &payment.QueryRefundResponse{Status: payment.RefundSucceeded}, nil
}

func (a *fakeAdapter) ParseNotify(context.Context, payment.NotifyRequest) (*payment.NotifyResult, error) {
	return nil, payment.ErrUnsupportedCapability
}

func TestRouterUsesExplicitMerchantKey(t *testing.T) {
	main := routeAdapter("main")
	school := routeAdapter("school_a")
	router, err := channelrouter.NewAdapter(channelrouter.Config{
		Provider: payment.ProviderStripe,
		Routes: []channelrouter.Route{
			{
				Provider:    payment.ProviderStripe,
				Channels:    []payment.Channel{payment.ChannelStripeCheckout},
				MerchantKey: "main",
				Adapter:     main,
			},
			{
				Provider:    payment.ProviderStripe,
				Channels:    []payment.Channel{payment.ChannelStripeCheckout},
				MerchantKey: "school_a",
				Adapter:     school,
			},
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	resp, err := router.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderStripe,
		Channel:     payment.ChannelStripeCheckout,
		MerchantKey: "school_a",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.MerchantKey != "school_a" || resp.ProviderTradeID != "trade_school_a" {
		t.Fatalf("response = %+v", resp)
	}
	if school.createReq.MerchantKey != "school_a" {
		t.Fatalf("routed request merchant = %q", school.createReq.MerchantKey)
	}
	if main.createReq.MerchantKey != "" {
		t.Fatalf("main should not be called: %+v", main.createReq)
	}
}

func TestRouterRequiresExplicitMerchantWhenNoDefault(t *testing.T) {
	router, err := channelrouter.NewAdapter(channelrouter.Config{
		Provider: payment.ProviderStripe,
		Routes: []channelrouter.Route{{
			Provider:    payment.ProviderStripe,
			Channels:    []payment.Channel{payment.ChannelStripeCheckout},
			MerchantKey: "main",
			Adapter:     routeAdapter("main"),
		}},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	_, err = router.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider: payment.ProviderStripe,
		Channel:  payment.ChannelStripeCheckout,
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatalf("err = nil, want missing route")
	}
}

func TestRouterCanUseOptionalDefault(t *testing.T) {
	router, err := channelrouter.NewAdapter(channelrouter.Config{
		Provider: payment.ProviderStripe,
		Routes: []channelrouter.Route{{
			Provider:    payment.ProviderStripe,
			Channels:    []payment.Channel{payment.ChannelStripeCheckout},
			MerchantKey: "main",
			Default:     true,
			Adapter:     routeAdapter("main"),
		}},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	resp, err := router.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider: payment.ProviderStripe,
		Channel:  payment.ChannelStripeCheckout,
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.MerchantKey != "main" {
		t.Fatalf("merchant key = %q, want main", resp.MerchantKey)
	}
}

func TestRouterDebugLoggerReceivesRouteEvents(t *testing.T) {
	var events []channelrouter.DebugEvent
	router, err := channelrouter.NewAdapter(channelrouter.Config{
		Provider: payment.ProviderStripe,
		DebugLogger: channelrouter.DebugFunc(func(_ context.Context, event channelrouter.DebugEvent) {
			events = append(events, event)
		}),
		Routes: []channelrouter.Route{{
			Provider:    payment.ProviderStripe,
			Channels:    []payment.Channel{payment.ChannelStripeCheckout},
			MerchantKey: "school_a",
			Adapter:     routeAdapter("school_a"),
		}},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	resp, err := router.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderStripe,
		Channel:     payment.ChannelStripeCheckout,
		MerchantKey: "school_a",
		PaymentID:   "pay_1001",
		OutTradeNo:  "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.MerchantKey != "school_a" {
		t.Fatalf("response = %+v", resp)
	}
	if len(events) != 4 {
		t.Fatalf("events = %+v, want 4 events", events)
	}
	wantStages := []channelrouter.DebugStage{
		channelrouter.DebugStageResolveStart,
		channelrouter.DebugStageResolveSucceeded,
		channelrouter.DebugStageOperationStart,
		channelrouter.DebugStageOperationDone,
	}
	for i, want := range wantStages {
		if events[i].Stage != want {
			t.Fatalf("event[%d].stage = %q, want %q; events=%+v", i, events[i].Stage, want, events)
		}
		if events[i].RequestedMerchantKey != "school_a" && i != 1 {
			t.Fatalf("event[%d].requested merchant = %q", i, events[i].RequestedMerchantKey)
		}
		if i > 0 && events[i].ResolvedMerchantKey != "school_a" {
			t.Fatalf("event[%d].resolved merchant = %q", i, events[i].ResolvedMerchantKey)
		}
	}
	if events[3].ProviderTradeID != "trade_school_a" {
		t.Fatalf("done event = %+v", events[3])
	}
	if events[2].Request != nil || events[3].Response != nil {
		t.Fatalf("payload should be disabled by default: %+v", events)
	}
}

func TestRouterDebugLoggerCanReceiveFullPayload(t *testing.T) {
	var events []channelrouter.DebugEvent
	router, err := channelrouter.NewAdapter(channelrouter.Config{
		Provider:         payment.ProviderStripe,
		DebugPayloadMode: channelrouter.DebugPayloadFull,
		DebugLogger: channelrouter.DebugFunc(func(_ context.Context, event channelrouter.DebugEvent) {
			events = append(events, event)
		}),
		Routes: []channelrouter.Route{{
			Provider:    payment.ProviderStripe,
			Channels:    []payment.Channel{payment.ChannelStripeCheckout},
			MerchantKey: "school_a",
			Adapter:     routeAdapter("school_a"),
		}},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	_, err = router.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderStripe,
		Channel:     payment.ChannelStripeCheckout,
		MerchantKey: "school_a",
		PaymentID:   "pay_1001",
		OutTradeNo:  "pay_1001",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
		Extra: map[string]any{"debug_marker": "visible"},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("events = %+v, want 4 events", events)
	}
	req, ok := events[2].Request.(payment.CreatePaymentRequest)
	if !ok {
		t.Fatalf("operation start request type = %T", events[2].Request)
	}
	if req.Extra["debug_marker"] != "visible" {
		t.Fatalf("request payload = %+v", req)
	}
	resp, ok := events[3].Response.(*payment.CreatePaymentResponse)
	if !ok {
		t.Fatalf("operation done response type = %T", events[3].Response)
	}
	if resp.ProviderTradeID != "trade_school_a" {
		t.Fatalf("response payload = %+v", resp)
	}
}

func TestRouterDebugLoggerReceivesResolveError(t *testing.T) {
	var events []channelrouter.DebugEvent
	router, err := channelrouter.NewAdapter(channelrouter.Config{
		Provider: payment.ProviderStripe,
		DebugLogger: channelrouter.DebugFunc(func(_ context.Context, event channelrouter.DebugEvent) {
			events = append(events, event)
		}),
		Routes: []channelrouter.Route{{
			Provider:    payment.ProviderStripe,
			Channels:    []payment.Channel{payment.ChannelStripeCheckout},
			MerchantKey: "school_a",
			Adapter:     routeAdapter("school_a"),
		}},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	_, err = router.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:    payment.ProviderStripe,
		Channel:     payment.ChannelStripeCheckout,
		MerchantKey: "missing",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
	})
	if err == nil {
		t.Fatalf("err = nil, want route error")
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want 2 events", events)
	}
	if events[1].Stage != channelrouter.DebugStageResolveFailed || events[1].Err == nil {
		t.Fatalf("failed event = %+v", events[1])
	}
	if !errors.Is(events[1].Err, payment.ErrAdapterNotFound) {
		t.Fatalf("event err = %v, want ErrAdapterNotFound", events[1].Err)
	}
}

func routeAdapter(merchantKey string) *fakeAdapter {
	return &fakeAdapter{
		provider:    payment.ProviderStripe,
		channel:     payment.ChannelStripeCheckout,
		merchantKey: merchantKey,
		caps: payment.Capabilities{
			Provider:            payment.ProviderStripe,
			Channels:            []payment.Channel{payment.ChannelStripeCheckout},
			SupportedCurrencies: []string{"USD"},
			SupportedActions:    []payment.ActionType{payment.ActionRedirectURL},
			SupportsQuery:       true,
			SupportsRefund:      true,
			SupportsQueryRefund: true,
		},
	}
}
