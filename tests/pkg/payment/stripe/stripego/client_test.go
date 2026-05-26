package stripego_test

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
	"github.com/huwenlong92/sdkit/pkg/payment/stripe/stripego"
	official "github.com/stripe/stripe-go/v85"
)

type fakeServices struct {
	checkoutCreate  *official.CheckoutSessionCreateParams
	checkoutGetID   string
	checkoutExpire  string
	intentCreate    *official.PaymentIntentCreateParams
	intentGetID     string
	intentCancelID  string
	refundCreate    *official.RefundCreateParams
	refundRetrieve  string
	checkoutSession *official.CheckoutSession
	intent          *official.PaymentIntent
	refund          *official.Refund
}

func (s *fakeServices) CreateCheckoutSession(_ context.Context, params *official.CheckoutSessionCreateParams) (*official.CheckoutSession, error) {
	s.checkoutCreate = params
	if s.checkoutSession != nil {
		return s.checkoutSession, nil
	}
	return &official.CheckoutSession{
		ID:            "cs_test_1",
		URL:           "https://checkout.stripe.example.test/pay/cs_test_1",
		PaymentStatus: "unpaid",
		Status:        "open",
		AmountTotal:   1200,
		Currency:      "usd",
		PaymentIntent: &official.PaymentIntent{ID: "pi_test_1"},
	}, nil
}

func (s *fakeServices) RetrieveCheckoutSession(_ context.Context, id string, _ *official.CheckoutSessionRetrieveParams) (*official.CheckoutSession, error) {
	s.checkoutGetID = id
	return &official.CheckoutSession{
		ID:            id,
		PaymentStatus: "paid",
		Status:        "complete",
		AmountTotal:   1200,
		Currency:      "usd",
		PaymentIntent: &official.PaymentIntent{ID: "pi_test_1"},
	}, nil
}

func (s *fakeServices) ExpireCheckoutSession(_ context.Context, id string, _ *official.CheckoutSessionExpireParams) (*official.CheckoutSession, error) {
	s.checkoutExpire = id
	return &official.CheckoutSession{ID: id, Status: "expired"}, nil
}

func (s *fakeServices) CreatePaymentIntent(_ context.Context, params *official.PaymentIntentCreateParams) (*official.PaymentIntent, error) {
	s.intentCreate = params
	if s.intent != nil {
		return s.intent, nil
	}
	return &official.PaymentIntent{
		ID:           "pi_test_1",
		ClientSecret: "pi_test_1_secret",
		Status:       "requires_payment_method",
		Amount:       1200,
		Currency:     "usd",
	}, nil
}

func (s *fakeServices) RetrievePaymentIntent(_ context.Context, id string, _ *official.PaymentIntentRetrieveParams) (*official.PaymentIntent, error) {
	s.intentGetID = id
	return &official.PaymentIntent{
		ID:       id,
		Status:   "succeeded",
		Amount:   1200,
		Currency: "usd",
	}, nil
}

func (s *fakeServices) CancelPaymentIntent(_ context.Context, id string, _ *official.PaymentIntentCancelParams) (*official.PaymentIntent, error) {
	s.intentCancelID = id
	return &official.PaymentIntent{ID: id, Status: "canceled"}, nil
}

func (s *fakeServices) CreateRefund(_ context.Context, params *official.RefundCreateParams) (*official.Refund, error) {
	s.refundCreate = params
	if s.refund != nil {
		return s.refund, nil
	}
	return &official.Refund{
		ID:            "re_test_1",
		Status:        "succeeded",
		Amount:        500,
		Currency:      "usd",
		PaymentIntent: &official.PaymentIntent{ID: "pi_test_1"},
	}, nil
}

func (s *fakeServices) RetrieveRefund(_ context.Context, id string, _ *official.RefundRetrieveParams) (*official.Refund, error) {
	s.refundRetrieve = id
	return &official.Refund{
		ID:            id,
		Status:        "pending",
		Amount:        500,
		Currency:      "usd",
		PaymentIntent: &official.PaymentIntent{ID: "pi_test_1"},
	}, nil
}

func TestClientCreateCheckoutSession(t *testing.T) {
	fake := &fakeServices{}
	client := newClient(t, fake)

	resp, err := client.CreatePayment(context.Background(), createRequest(payment.ChannelStripeCheckout))
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Type != payment.ActionRedirectURL || resp.Action.URL == "" {
		t.Fatalf("action = %+v", resp.Action)
	}
	if resp.ProviderTradeID != "cs_test_1" || resp.Extra[stripego.ExtraPaymentIntentIDKey] != "pi_test_1" {
		t.Fatalf("response = %+v", resp)
	}
	if value(fake.checkoutCreate.SuccessURL) != "https://example.test/success" || value(fake.checkoutCreate.CancelURL) != "https://example.test/cancel" {
		t.Fatalf("urls = success %q cancel %q", value(fake.checkoutCreate.SuccessURL), value(fake.checkoutCreate.CancelURL))
	}
	if fake.checkoutCreate.LineItems[0].PriceData == nil ||
		value(fake.checkoutCreate.LineItems[0].PriceData.UnitAmount) != int64(1200) ||
		value(fake.checkoutCreate.LineItems[0].PriceData.Currency) != "usd" {
		t.Fatalf("line item = %+v", fake.checkoutCreate.LineItems[0])
	}
}

func TestClientCreatePaymentIntent(t *testing.T) {
	fake := &fakeServices{}
	client := newClient(t, fake)

	req := createRequest(payment.ChannelStripeIntent)
	req.Extra = nil
	resp, err := client.CreatePayment(context.Background(), req)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.Action.Type != payment.ActionClientToken || resp.Action.Token != "pi_test_1_secret" {
		t.Fatalf("action = %+v", resp.Action)
	}
	if value(fake.intentCreate.Amount) != int64(1200) || value(fake.intentCreate.Currency) != "usd" {
		t.Fatalf("intent create = %+v", fake.intentCreate)
	}
	if fake.intentCreate.AutomaticPaymentMethods == nil || !value(fake.intentCreate.AutomaticPaymentMethods.Enabled) {
		t.Fatalf("automatic payment methods = %+v", fake.intentCreate.AutomaticPaymentMethods)
	}
}

func TestClientDebugLoggerReceivesStripeParams(t *testing.T) {
	fake := &fakeServices{}
	var events []debuglog.Event
	client, err := stripego.NewClient(stripego.Config{
		SuccessURL:           "https://example.test/success",
		CancelURL:            "https://example.test/cancel",
		CheckoutService:      fake,
		PaymentIntentService: fake,
		RefundService:        fake,
		DebugPayloadMode:     debuglog.PayloadFull,
		DebugLogger: debuglog.Func(func(_ context.Context, event debuglog.Event) {
			events = append(events, event)
		}),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.CreatePayment(context.Background(), createRequest(payment.ChannelStripeCheckout))
	if err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("events = %+v, want request and response", events)
	}
	if events[0].Operation != "create_checkout_session" || events[0].Stage != debuglog.StageRequest {
		t.Fatalf("request event = %+v", events[0])
	}
	params, ok := events[0].Request.(*official.CheckoutSessionCreateParams)
	if !ok {
		t.Fatalf("request payload type = %T", events[0].Request)
	}
	if value(params.SuccessURL) != "https://example.test/success" ||
		value(params.LineItems[0].PriceData.UnitAmount) != int64(1200) {
		t.Fatalf("checkout params = %+v", params)
	}
	if events[1].Stage != debuglog.StageResponse || events[1].Response == nil {
		t.Fatalf("response event = %+v", events[1])
	}
}

func TestClientQueryCloseRefundAndQueryRefund(t *testing.T) {
	fake := &fakeServices{}
	client := newClient(t, fake)

	queryResp, err := client.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:        payment.ProviderStripe,
		Channel:         payment.ChannelStripeIntent,
		ProviderTradeID: "pi_test_1",
	})
	if err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if fake.intentGetID != "pi_test_1" || queryResp.Status != payment.PaymentSucceeded {
		t.Fatalf("query id = %q response = %+v", fake.intentGetID, queryResp)
	}

	if err := client.ClosePayment(context.Background(), payment.ClosePaymentRequest{
		Provider:        payment.ProviderStripe,
		Channel:         payment.ChannelStripeCheckout,
		ProviderTradeID: "cs_test_1",
	}); err != nil {
		t.Fatalf("close checkout: %v", err)
	}
	if fake.checkoutExpire != "cs_test_1" {
		t.Fatalf("checkout expire id = %q", fake.checkoutExpire)
	}

	refundResp, err := client.Refund(context.Background(), payment.RefundRequest{
		Provider:        payment.ProviderStripe,
		Channel:         payment.ChannelStripeIntent,
		PaymentID:       "pay_1001",
		OutTradeNo:      "pay_1001",
		ProviderTradeID: "pi_test_1",
		RefundID:        "refund_1001",
		OutRefundNo:     "refund_1001",
		Reason:          "requested_by_customer",
		Amount: payment.RefundAmount{
			Refund: payment.Money{Amount: 500, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refundResp.ProviderRefundID != "re_test_1" || refundResp.Status != payment.RefundSucceeded {
		t.Fatalf("refund response = %+v", refundResp)
	}
	if value(fake.refundCreate.PaymentIntent) != "pi_test_1" || value(fake.refundCreate.Amount) != int64(500) || value(fake.refundCreate.Reason) != "requested_by_customer" {
		t.Fatalf("refund params = %+v", fake.refundCreate)
	}

	refundQueryResp, err := client.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider:         payment.ProviderStripe,
		Channel:          payment.ChannelStripeIntent,
		ProviderRefundID: "re_test_1",
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if fake.refundRetrieve != "re_test_1" || refundQueryResp.Status != payment.RefundProcessing {
		t.Fatalf("query refund id = %q response = %+v", fake.refundRetrieve, refundQueryResp)
	}
}

func newClient(t *testing.T, fake *fakeServices) *stripego.Client {
	t.Helper()
	client, err := stripego.NewClient(stripego.Config{
		SuccessURL:           "https://example.test/success",
		CancelURL:            "https://example.test/cancel",
		CheckoutService:      fake,
		PaymentIntentService: fake,
		RefundService:        fake,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func createRequest(channel payment.Channel) payment.CreatePaymentRequest {
	return payment.CreatePaymentRequest{
		Provider:   payment.ProviderStripe,
		Channel:    channel,
		PaymentID:  "pay_1001",
		OutTradeNo: "pay_1001",
		Subject:    "Test Order",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
		Extra: map[string]any{
			stripego.ExtraPaymentMethodTypesKey: []string{"card"},
		},
	}
}

func value[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}
