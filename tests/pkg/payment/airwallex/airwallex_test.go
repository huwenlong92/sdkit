//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex/httpapi"
)

const mutationID = "d515ef21-597b-4db8-a454-6c4a679286c3"
const testSecret = "fictional-webhook-secret"

var fixedNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

type roundTrip func(*http.Request) (*http.Response, error)

func (fn roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func config(rt roundTrip) httpapi.Config {
	return httpapi.Config{Environment: httpapi.Sandbox, ClientID: "fictional-client", APIKey: "fictional-api-key", AccountID: "acct_fixture", WebhookSecret: testSecret, ReturnURL: "https://merchant.example.test/result", Clock: func() time.Time { return fixedNow }, HTTPClient: &http.Client{Transport: rt}}
}
func newClient(t *testing.T, rt roundTrip) *httpapi.Client {
	t.Helper()
	c, err := httpapi.NewClient(config(rt))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func login(req *http.Request) *http.Response {
	if req.URL.Path != "/api/v1/authentication/login" {
		return nil
	}
	return reply(200, `{"token":"fixture-access-token","expires_at":"2026-01-02T04:04:05Z"}`)
}
func createRequest() payment.CreatePaymentRequest {
	return payment.CreatePaymentRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "platform", PaymentID: "payment-fixture", OrderID: "order-fixture", OutTradeNo: "trade-fixture", Pricing: payment.PaymentPricing{PayAmount: payment.Money{Amount: 1234, Currency: "USD"}, SettleCurrency: "USD"}, Extra: map[string]any{httpapi.ExtraRequestIDKey: mutationID}}
}
func queryRequest() payment.QueryPaymentRequest {
	return payment.QueryPaymentRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "platform", ProviderTradeID: "int_fixture", OutTradeNo: "trade-fixture"}
}
func refundRequest() payment.RefundRequest {
	return payment.RefundRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "platform", ProviderTradeID: "int_fixture", RefundID: "refund-fixture", OutRefundNo: "refund-number", Amount: payment.RefundAmount{Refund: payment.Money{Amount: 234, Currency: "USD"}}, Extra: map[string]any{httpapi.ExtraRequestIDKey: mutationID}}
}
func intentJSON(status, amount, currency string) string {
	return fmt.Sprintf(`{"id":"int_fixture","request_id":%q,"merchant_order_id":"trade-fixture","status":%q,"amount":%s,"currency":%q,"client_secret":"fictional-checkout-token"}`, mutationID, status, amount, currency)
}
func refundJSON(status string) string {
	return fmt.Sprintf(`{"id":"rfd_fixture","request_id":%q,"payment_intent_id":"int_fixture","status":%q,"amount":2.34,"currency":"USD"}`, mutationID, status)
}
func mustBody(t *testing.T, r *http.Request) map[string]json.RawMessage {
	t.Helper()
	defer r.Body.Close()
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestCoreFacadeLifecycle(t *testing.T) {
	var auth, creates, refunds, queries int
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.sandbox.airwallex.com" {
			t.Fatalf("host = %s", r.URL.Host)
		}
		if result := login(r); result != nil {
			auth++
			if r.Header.Get("x-api-key") != "fictional-api-key" || r.Header.Get("x-client-id") != "fictional-client" || r.Header.Get("x-login-as") != "acct_fixture" {
				t.Fatal("authentication headers")
			}
			return result, nil
		}
		if r.Header.Get("Authorization") != "Bearer fixture-access-token" || r.Header.Get("x-api-key") != "" {
			t.Fatal("payment authentication isolation")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/pa/payment_intents/create":
			creates++
			body := mustBody(t, r)
			if string(body["amount"]) != "12.34" || string(body["request_id"]) != fmt.Sprintf("%q", mutationID) || string(body["merchant_order_id"]) != `"trade-fixture"` {
				t.Fatalf("create body = %s", body)
			}
			if _, ok := body["funds_split_data"]; ok {
				t.Fatal("unexpected funds split")
			}
			return reply(201, intentJSON("REQUIRES_PAYMENT_METHOD", "12.34", "USD")), nil
		case "GET /api/v1/pa/payment_intents/int_fixture":
			queries++
			return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
		case "POST /api/v1/pa/payment_intents/int_fixture/cancel":
			body := mustBody(t, r)
			if string(body["request_id"]) != fmt.Sprintf("%q", mutationID) {
				t.Fatal("cancel id")
			}
			return reply(200, intentJSON("CANCELLED", "12.34", "USD")), nil
		case "POST /api/v1/pa/refunds/create":
			refunds++
			body := mustBody(t, r)
			if string(body["amount"]) != "2.34" || string(body["payment_intent_id"]) != `"int_fixture"` {
				t.Fatal("refund body")
			}
			return reply(201, refundJSON("RECEIVED")), nil
		case "GET /api/v1/pa/refunds/rfd_fixture":
			return reply(200, refundJSON("SETTLED")), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
			return nil, nil
		}
	})
	var loads, cleanups int
	adapter, err := airwallex.NewAdapter(airwallex.Config{ClientLoader: airwallex.ClientLoaderFunc(func(ctx context.Context, key string) (airwallex.Client, airwallex.ClientCleanup, error) {
		loads++
		if key != "platform" {
			t.Fatalf("merchant = %s", key)
		}
		return c, func() error { cleanups++; return nil }, nil
	}), NotifyMerchantKey: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	selector, err := payment.NewStaticChannelSelector([]payment.ChannelBinding{{Key: "checkout", Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "platform"}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := payment.NewService(payment.ServiceConfig{Registry: registry, ChannelSelector: selector})
	if err != nil {
		t.Fatal(err)
	}
	payment.SetDefault(service)
	t.Cleanup(payment.Close)
	req := createRequest()
	req.Provider = ""
	req.Channel = ""
	req.MerchantKey = "checkout"
	for range 2 {
		got, err := payment.CreatePayment(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != payment.PaymentPending || got.ProviderTradeID != "int_fixture" || got.Action.Type != payment.ActionSDKParams || got.Action.Params["env"] != "sandbox" || got.Action.Params["client_secret"] != "fictional-checkout-token" {
			t.Fatalf("create result = %+v", got)
		}
		if got.Raw != nil {
			t.Fatal("raw must not expose credentials")
		}
	}
	query, err := payment.QueryPayment(context.Background(), queryRequest())
	if err != nil || query.Status != payment.PaymentSucceeded {
		t.Fatalf("query=%+v err=%v", query, err)
	}
	if err := payment.ClosePayment(context.Background(), payment.ClosePaymentRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "platform", ProviderTradeID: "int_fixture", Extra: map[string]any{httpapi.ExtraRequestIDKey: mutationID}}); err != nil {
		t.Fatal(err)
	}
	refunded, err := payment.Refund(context.Background(), refundRequest())
	if err != nil || refunded.Status != payment.RefundPending || refunded.RefundID != "refund-fixture" {
		t.Fatalf("refund=%+v err=%v", refunded, err)
	}
	qr, err := payment.QueryRefund(context.Background(), payment.QueryRefundRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "platform", ProviderRefundID: "rfd_fixture"})
	if err != nil || qr.Status != payment.RefundSucceeded {
		t.Fatalf("refund query=%+v err=%v", qr, err)
	}
	notify := signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow)
	notify.Query = map[string][]string{"merchant_key": {"attacker"}}
	result, err := payment.HandleNotify(context.Background(), notify)
	if err != nil || result.Event.MerchantKey != "platform" || result.Event.Type != payment.EventPaymentSucceeded {
		t.Fatalf("notify=%+v err=%v", result, err)
	}
	if auth != 1 || creates != 2 || refunds != 1 || queries != 2 || loads != 7 || loads != cleanups {
		t.Fatalf("auth=%d create=%d refund=%d query=%d load=%d cleanup=%d", auth, creates, refunds, queries, loads, cleanups)
	}
}

func TestMoneyPrecisionAndUnknownStatus(t *testing.T) {
	for _, tc := range []struct {
		name, amount, currency string
		minor                  int64
		wantErr                bool
	}{
		{"two decimals", "12.34", "USD", 1234, false}, {"zero decimals", "1234", "JPY", 1234, false}, {"three decimals", "1.234", "KWD", 1234, false}, {"exponent", "1.234e1", "USD", 1234, false},
		{"int64 maximum", "92233720368547758.07", "USD", 9223372036854775807, false}, {"extra precision", "1.001", "USD", 0, true}, {"overflow", "92233720368547758.08", "USD", 0, true}, {"missing currency", "1", "", 0, true}, {"unknown currency", "1", "ZZZ", 0, true}, {"negative", "-1", "USD", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config(func(r *http.Request) (*http.Response, error) {
				if v := login(r); v != nil {
					return v, nil
				}
				return reply(200, intentJSON("FUTURE_STATE", tc.amount, tc.currency)), nil
			})
			cfg.CurrencyMetadata = map[string]payment.CurrencyMeta{"KWD": {Code: "KWD", MinorExp: 3}}
			c, err := httpapi.NewClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.QueryPayment(context.Background(), queryRequest())
			if (err != nil) != tc.wantErr {
				t.Fatalf("got=%+v err=%v", got, err)
			}
			if err == nil && (got.Pricing.PayAmount.Amount != tc.minor || got.Status != "" || got.Extra["provider_status"] != "FUTURE_STATE" || got.Raw != nil) {
				t.Fatalf("got=%+v", got)
			}
		})
	}
}

func TestPaymentStates(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want payment.PaymentStatus
	}{{"REQUIRES_PAYMENT_METHOD", payment.PaymentPending}, {"REQUIRES_CUSTOMER_ACTION", payment.PaymentRequiresAction}, {"REQUIRES_CAPTURE", payment.PaymentAuthorized}, {"PENDING", payment.PaymentProcessing}, {"PENDING_REVIEW", payment.PaymentProcessing}, {"SUCCEEDED", payment.PaymentSucceeded}, {"CANCELLED", payment.PaymentClosed}} {
		t.Run(tc.raw, func(t *testing.T) {
			c := newClient(t, func(r *http.Request) (*http.Response, error) {
				if v := login(r); v != nil {
					return v, nil
				}
				return reply(200, intentJSON(tc.raw, "12.34", "USD")), nil
			})
			got, err := c.QueryPayment(context.Background(), queryRequest())
			if err != nil || got.Status != tc.want {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
func TestValidationBeforeNetwork(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*payment.CreatePaymentRequest)
	}{
		{"missing id", func(r *payment.CreatePaymentRequest) { r.Extra = nil }}, {"invalid id", func(r *payment.CreatePaymentRequest) { r.Extra[httpapi.ExtraRequestIDKey] = "not-uuid" }}, {"v1 uuid", func(r *payment.CreatePaymentRequest) {
			r.Extra[httpapi.ExtraRequestIDKey] = "d515ef21-597b-1db8-a454-6c4a679286c3"
		}}, {"wrong channel", func(r *payment.CreatePaymentRequest) { r.Channel = payment.ChannelPayPalOrder }}, {"wrong provider", func(r *payment.CreatePaymentRequest) { r.Provider = payment.ProviderStripe }}, {"expiry", func(r *payment.CreatePaymentRequest) { r.ExpireAt = &fixedNow }}, {"zero amount", func(r *payment.CreatePaymentRequest) { r.Pricing.PayAmount.Amount = 0 }}, {"return URL", func(r *payment.CreatePaymentRequest) { r.ReturnURL = "http://merchant.example.test/" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network"); return nil, nil })
			req := createRequest()
			tc.change(&req)
			if _, err := c.CreatePayment(context.Background(), req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	c := newClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network"); return nil, nil })
	req := queryRequest()
	req.ProviderTradeID = "int_../authentication/login"
	if _, err := c.QueryPayment(context.Background(), req); err == nil {
		t.Fatal("path traversal")
	}
	if _, err := c.CreatePayment(nil, createRequest()); !errors.Is(err, payment.ErrNilContext) {
		t.Fatalf("nil ctx: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.CreatePayment(ctx, createRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}

func signed(name, object string, at time.Time) payment.NotifyRequest {
	body := []byte(fmt.Sprintf(`{"id":"evt_fixture","name":%q,"account_id":"acct_fixture","version":"fixture-version","data":{"object":%s}}`, name, object))
	timestamp := fmt.Sprint(at.UnixMilli())
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	return payment.NotifyRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, Method: "POST", Header: map[string][]string{"x-timestamp": {timestamp}, "x-signature": {hex.EncodeToString(mac.Sum(nil))}}, Body: body}
}
func TestWebhookSecurityAndMapping(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("webhook must not make HTTP calls")
		return nil, nil
	})
	for _, tc := range []struct {
		name   string
		mutate func(*payment.NotifyRequest)
	}{
		{"tampered body", func(r *payment.NotifyRequest) { r.Body = append(r.Body, ' ') }}, {"bad signature", func(r *payment.NotifyRequest) { r.Header["x-signature"] = []string{"invalid"} }}, {"duplicate signature", func(r *payment.NotifyRequest) { r.Header["X-Signature"] = r.Header["x-signature"] }}, {"stale", func(r *payment.NotifyRequest) {
			*r = signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow.Add(-6*time.Minute))
		}}, {"future", func(r *payment.NotifyRequest) {
			*r = signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow.Add(6*time.Minute))
		}}, {"GET", func(r *payment.NotifyRequest) { r.Method = "GET" }}, {"mismatched state", func(r *payment.NotifyRequest) {
			*r = signed("payment_intent.succeeded", intentJSON("PENDING", "12.34", "USD"), fixedNow)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow)
			tc.mutate(&req)
			if _, err := c.ParseNotify(context.Background(), req); !errors.Is(err, payment.ErrNotifyVerificationFail) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	wrong := config(nil)
	wrong.AccountID = "acct_other"
	other, err := httpapi.NewClient(wrong)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.ParseNotify(context.Background(), signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow)); !errors.Is(err, payment.ErrNotifyVerificationFail) {
		t.Fatalf("wrong account: %v", err)
	}
	for _, tc := range []struct {
		name, object string
		event        payment.EventType
		refund       payment.RefundStatus
	}{
		{"payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), payment.EventPaymentSucceeded, ""},
		{"refund.received", refundJSON("RECEIVED"), payment.EventRefundCreated, payment.RefundPending},
		{"refund.accepted", refundJSON("ACCEPTED"), payment.EventRefundSucceeded, payment.RefundSucceeded},
		{"refund.settled", refundJSON("SETTLED"), payment.EventRefundSucceeded, payment.RefundSucceeded},
		{"refund.failed", refundJSON("FAILED"), payment.EventRefundFailed, payment.RefundFailed},
		{"payment_attempt.failed", `{}`, payment.EventUnknown, ""},
		{"payment_intent.new_event", intentJSON("SUCCEEDED", "12.34", "USD"), payment.EventUnknown, ""},
		{"refund.new_event", refundJSON("SETTLED"), payment.EventUnknown, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for range 2 {
				got, err := c.ParseNotify(context.Background(), signed(tc.name, tc.object, fixedNow))
				if err != nil {
					t.Fatal(err)
				}
				if !got.Verified || got.Ack.StatusCode != 200 || got.Event.EventID != "evt_fixture" || got.Event.Type != tc.event || got.Event.RefundStatus != tc.refund || len(got.Event.Raw) != 0 {
					t.Fatalf("got=%+v event=%+v", got, got.Event)
				}
				if tc.event == payment.EventUnknown && got.Event.Status != "" {
					t.Fatal("unknown event must not advance payment")
				}
			}
		})
	}
}

func TestTokenConcurrencyRefreshAndCancellation(t *testing.T) {
	var logins atomic.Int32
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if v := login(r); v != nil {
			logins.Add(1)
			return v, nil
		}
		return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
	})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.QueryPayment(context.Background(), queryRequest()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if logins.Load() != 1 {
		t.Fatalf("logins=%d", logins.Load())
	}
	entered, release := make(chan struct{}), make(chan struct{})
	blocked := newClient(t, func(r *http.Request) (*http.Response, error) {
		if v := login(r); v != nil {
			close(entered)
			<-release
			return v, nil
		}
		return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
	})
	done := make(chan error, 1)
	go func() { _, err := blocked.QueryPayment(context.Background(), queryRequest()); done <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := blocked.QueryPayment(ctx, queryRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestAPIErrorDoesNotLeakAnd401DoesNotReplay(t *testing.T) {
	var auth, calls int
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if v := login(r); v != nil {
			auth++
			return v, nil
		}
		calls++
		return reply(401, `{"code":"unauthorized","message":"fictional-api-key fictional-checkout-token"}`), nil
	})
	for range 2 {
		_, err := c.CreatePayment(context.Background(), createRequest())
		var apiErr *httpapi.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 || apiErr.Code != "unauthorized" {
			t.Fatalf("err=%v", err)
		}
		if strings.Contains(fmt.Sprintf("%+v", err), "fictional") {
			t.Fatal("sensitive error")
		}
	}
	if auth != 2 || calls != 2 {
		t.Fatalf("auth=%d calls=%d", auth, calls)
	}
}
func TestRefundRejectsWrongCurrencyBeforeMutation(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if v := login(r); v != nil {
			return v, nil
		}
		if r.Method != "GET" {
			t.Fatal("unexpected mutation")
		}
		return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
	})
	req := refundRequest()
	req.Amount.Refund.Currency = "JPY"
	if _, err := c.Refund(context.Background(), req); !errors.Is(err, payment.ErrInvalidCurrency) {
		t.Fatalf("err=%v", err)
	}
}

func TestConfigurationAndClientCleanup(t *testing.T) {
	cfg := config(nil)
	cfg.Environment = ""
	if _, err := httpapi.NewClient(cfg); err == nil {
		t.Fatal("environment required")
	}
	cfg = config(nil)
	cfg.AccountID = ""
	if _, err := httpapi.NewClient(cfg); err == nil {
		t.Fatal("account required")
	}
	if _, err := airwallex.NewAdapter(airwallex.Config{}); err == nil {
		t.Fatal("dynamic loader required")
	}
	var cleanup int
	adapter, err := airwallex.NewAdapter(airwallex.Config{ClientLoader: airwallex.ClientLoaderFunc(func(context.Context, string) (airwallex.Client, airwallex.ClientCleanup, error) {
		return nil, func() error { cleanup++; return nil }, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CreatePayment(context.Background(), createRequest()); !errors.Is(err, payment.ErrAdapterNotFound) || cleanup != 1 {
		t.Fatalf("cleanup=%d err=%v", cleanup, err)
	}
	if _, err := adapter.ParseNotify(context.Background(), signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow)); !errors.Is(err, payment.ErrInvalidRequest) || cleanup != 1 {
		t.Fatalf("trusted notify key required: %v", err)
	}
}

func TestTokenExpiryAndEnvironmentIsolation(t *testing.T) {
	var now = fixedNow
	var calls int
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/authentication/login" {
			calls++
			return reply(200, fmt.Sprintf(`{"token":"token-%d","expires_at":%q}`, calls, now.Add(time.Hour).Format(time.RFC3339))), nil
		}
		if r.URL.Host != "api.airwallex.com" {
			t.Fatalf("host=%s", r.URL.Host)
		}
		return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
	})
	cfg := config(rt)
	cfg.Environment = httpapi.Production
	cfg.Clock = func() time.Time { return now }
	c, err := httpapi.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, advance := range []time.Duration{0, 5 * time.Minute, 56 * time.Minute} {
		now = now.Add(advance)
		if _, err := c.QueryPayment(context.Background(), queryRequest()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("login count=%d", calls)
	}
	// A second account/client must not reuse another client's authentication cache.
	cfg.AccountID = "acct_second"
	second, err := httpapi.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.QueryPayment(context.Background(), queryRequest()); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("isolated login count=%d", calls)
	}
}

func TestMutationRetryPreservesPayloadAndRejectsMismatches(t *testing.T) {
	var payloads []string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if v := login(r); v != nil {
			return v, nil
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		payloads = append(payloads, string(data))
		if len(payloads) == 1 {
			return reply(503, `{"code":"unavailable"}`), nil
		}
		return reply(201, intentJSON("REQUIRES_PAYMENT_METHOD", "12.34", "USD")), nil
	})
	if _, err := c.CreatePayment(context.Background(), createRequest()); err == nil || len(payloads) != 1 {
		t.Fatal("must return ambiguous error without automatic retry")
	}
	if _, err := c.CreatePayment(context.Background(), createRequest()); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 2 || payloads[0] != payloads[1] {
		t.Fatal("retry changed request id or payload")
	}
	for _, tc := range []struct{ name, body string }{
		{"amount", intentJSON("REQUIRES_PAYMENT_METHOD", "12.35", "USD")},
		{"currency", intentJSON("REQUIRES_PAYMENT_METHOD", "12.34", "EUR")},
		{"order", strings.ReplaceAll(intentJSON("REQUIRES_PAYMENT_METHOD", "12.34", "USD"), "trade-fixture", "other-order")},
		{"secret", strings.ReplaceAll(intentJSON("REQUIRES_PAYMENT_METHOD", "12.34", "USD"), "fictional-checkout-token", "")},
		{"malformed JSON", "not-json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, func(r *http.Request) (*http.Response, error) {
				if v := login(r); v != nil {
					return v, nil
				}
				return reply(200, tc.body), nil
			})
			if _, err := c.CreatePayment(context.Background(), createRequest()); err == nil {
				t.Fatal("expected invalid response")
			}
		})
	}
}

func TestRedirectAndWebhookSecretIsolation(t *testing.T) {
	var requests int
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		v := reply(307, "")
		v.Header.Set("Location", "https://other.example.test/stolen")
		return v, nil
	})
	if _, err := c.QueryPayment(context.Background(), queryRequest()); err == nil || requests != 1 {
		t.Fatalf("redirect forwarded: requests=%d err=%v", requests, err)
	}
	cfg := config(nil)
	cfg.WebhookSecret = ""
	c, err := httpapi.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := signed("payment_intent.succeeded", intentJSON("SUCCEEDED", "12.34", "USD"), fixedNow)
	req.Header["client-secret-key"] = []string{testSecret}
	if _, err := c.ParseNotify(context.Background(), req); !errors.Is(err, payment.ErrNotifyVerificationFail) {
		t.Fatal("request cannot provide verification secret")
	}
}
