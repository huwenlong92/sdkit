package ordersapi_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/debuglog"
	"github.com/huwenlong92/sdkit/pkg/payment/paypal/ordersapi"
)

type fakeHTTP struct {
	requests []*http.Request
	bodies   []string
}

func (h *fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		h.bodies = append(h.bodies, string(raw))
	} else {
		h.bodies = append(h.bodies, "")
	}
	h.requests = append(h.requests, req)

	switch {
	case req.URL.Path == "/v1/oauth2/token":
		return jsonResponse(200, `{"access_token":"access_1","token_type":"Bearer","expires_in":3600}`), nil
	case req.Method == http.MethodPost && req.URL.Path == "/v2/checkout/orders":
		return jsonResponse(201, `{"id":"ORDER-1","status":"CREATED","links":[{"href":"https://paypal.example.test/checkout/ORDER-1","rel":"approve","method":"GET"}]}`), nil
	case req.Method == http.MethodGet && req.URL.Path == "/v2/checkout/orders/ORDER-1":
		return jsonResponse(200, `{"id":"ORDER-1","status":"APPROVED","purchase_units":[{"amount":{"currency_code":"USD","value":"12.00"}}]}`), nil
	case req.Method == http.MethodPost && req.URL.Path == "/v2/checkout/orders/ORDER-1/capture":
		return jsonResponse(201, `{"id":"ORDER-1","status":"COMPLETED","purchase_units":[{"payments":{"captures":[{"id":"CAPTURE-1","status":"COMPLETED","amount":{"currency_code":"USD","value":"12.00"}}]}}]}`), nil
	case req.Method == http.MethodPost && req.URL.Path == "/v2/payments/captures/CAPTURE-1/refund":
		return jsonResponse(201, `{"id":"REFUND-1","status":"COMPLETED","amount":{"currency_code":"USD","value":"5.00"}}`), nil
	case req.Method == http.MethodGet && req.URL.Path == "/v2/payments/refunds/REFUND-1":
		return jsonResponse(200, `{"id":"REFUND-1","status":"PENDING","amount":{"currency_code":"USD","value":"5.00"}}`), nil
	default:
		return jsonResponse(404, `{"name":"not_found"}`), nil
	}
}

func TestClientCreatePayment(t *testing.T) {
	httpClient := &fakeHTTP{}
	client := newClient(t, httpClient)

	resp, err := client.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderPayPal,
		Channel:    payment.ChannelPayPalOrder,
		PaymentID:  "pay_1001",
		OutTradeNo: "pay_1001",
		Subject:    "Test Order",
		ReturnURL:  "https://example.test/return",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if resp.ProviderTradeID != "ORDER-1" || resp.Action.Type != payment.ActionRedirectURL || resp.Action.URL == "" {
		t.Fatalf("response = %+v", resp)
	}
	if len(httpClient.requests) != 2 {
		t.Fatalf("request count = %d", len(httpClient.requests))
	}
	if got := httpClient.requests[1].Header.Get("Authorization"); got != "Bearer access_1" {
		t.Fatalf("authorization = %q", got)
	}
	if !strings.Contains(httpClient.bodies[1], `"value":"12.00"`) || !strings.Contains(httpClient.bodies[1], `"return_url":"https://example.test/return"`) {
		t.Fatalf("create body = %s", httpClient.bodies[1])
	}
}

func TestClientQueryCaptureRefundAndQueryRefund(t *testing.T) {
	httpClient := &fakeHTTP{}
	client := newClient(t, httpClient)

	queryResp, err := client.QueryPayment(context.Background(), payment.QueryPaymentRequest{
		Provider:        payment.ProviderPayPal,
		Channel:         payment.ChannelPayPalOrder,
		ProviderTradeID: "ORDER-1",
	})
	if err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if queryResp.Status != payment.PaymentAuthorized || queryResp.Pricing.PayAmount.Amount != 1200 {
		t.Fatalf("query response = %+v", queryResp)
	}

	captureResp, err := client.CapturePayment(context.Background(), ordersapi.CapturePaymentRequest{
		Provider:        payment.ProviderPayPal,
		Channel:         payment.ChannelPayPalOrder,
		ProviderTradeID: "ORDER-1",
	})
	if err != nil {
		t.Fatalf("capture payment: %v", err)
	}
	if captureResp.Status != payment.PaymentSucceeded || captureResp.Extra[ordersapi.ExtraCaptureIDKey] != "CAPTURE-1" {
		t.Fatalf("capture response = %+v", captureResp)
	}

	refundResp, err := client.Refund(context.Background(), payment.RefundRequest{
		Provider: payment.ProviderPayPal,
		Channel:  payment.ChannelPayPalOrder,
		Extra: map[string]any{
			ordersapi.ExtraCaptureIDKey: "CAPTURE-1",
		},
		Amount: payment.RefundAmount{Refund: payment.Money{Amount: 500, Currency: "USD"}},
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refundResp.ProviderRefundID != "REFUND-1" || refundResp.Status != payment.RefundSucceeded {
		t.Fatalf("refund response = %+v", refundResp)
	}

	refundQueryResp, err := client.QueryRefund(context.Background(), payment.QueryRefundRequest{
		Provider:         payment.ProviderPayPal,
		Channel:          payment.ChannelPayPalOrder,
		ProviderRefundID: "REFUND-1",
	})
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if refundQueryResp.Status != payment.RefundProcessing || refundQueryResp.Amount.Refund.Amount != 500 {
		t.Fatalf("refund query response = %+v", refundQueryResp)
	}
}

func TestClientDebugLoggerReceivesHTTPPayload(t *testing.T) {
	httpClient := &fakeHTTP{}
	var events []debuglog.Event
	client, err := ordersapi.NewClient(ordersapi.Config{
		ClientID:         "client_1",
		ClientSecret:     "secret_1",
		BaseURL:          "https://api-m.sandbox.paypal.com",
		ReturnURL:        "https://example.test/return",
		CancelURL:        "https://example.test/cancel",
		HTTPClient:       httpClient,
		DebugPayloadMode: debuglog.PayloadFull,
		DebugLogger: debuglog.Func(func(_ context.Context, event debuglog.Event) {
			events = append(events, event)
		}),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Provider:   payment.ProviderPayPal,
		Channel:    payment.ChannelPayPalOrder,
		OutTradeNo: "pay_1001",
		ReturnURL:  "https://example.test/return",
		Pricing: payment.PaymentPricing{
			PayAmount: payment.Money{Amount: 1200, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("events = %+v, want request and response", events)
	}
	if events[0].Operation != "POST /v2/checkout/orders" || events[0].Stage != debuglog.StageRequest {
		t.Fatalf("request event = %+v", events[0])
	}
	if events[0].Request == nil {
		t.Fatalf("request payload missing")
	}
	if events[1].Stage != debuglog.StageResponse || events[1].Response == nil {
		t.Fatalf("response event = %+v", events[1])
	}
}

func newClient(t *testing.T, httpClient *fakeHTTP) *ordersapi.Client {
	t.Helper()
	client, err := ordersapi.NewClient(ordersapi.Config{
		ClientID:     "client_1",
		ClientSecret: "secret_1",
		BaseURL:      "https://api-m.sandbox.paypal.com",
		ReturnURL:    "https://example.test/return",
		CancelURL:    "https://example.test/cancel",
		HTTPClient:   httpClient,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
