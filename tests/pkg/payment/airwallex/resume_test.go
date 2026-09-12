//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
)

func TestResumeExistingPaymentDoesNotCreate(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if result := login(r); result != nil {
			return result, nil
		}
		if r.Method != "GET" || r.URL.Path != "/api/v1/pa/payment_intents/int_fixture" {
			t.Fatalf("resume must read existing intent: %s %s", r.Method, r.URL.Path)
		}
		return reply(200, intentJSON("REQUIRES_PAYMENT_METHOD", "12.34", "USD")), nil
	})
	req := createRequest()
	req.ProviderTradeID = "int_fixture"
	got, err := c.CreatePayment(context.Background(), req)
	if err != nil || got == nil || got.Action.Type != payment.ActionSDKParams {
		t.Fatalf("resume action: %v", err)
	}
}

func TestDuplicateCreationReturnsProviderErrorWithoutLookup(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if result := login(r); result != nil {
			return result, nil
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/pa/payment_intents/create" {
			t.Fatalf("unexpected recovery request: %s %s", r.Method, r.URL.Path)
		}
		return reply(400, `{"code":"duplicate_request"}`), nil
	})
	if _, err := c.CreatePayment(context.Background(), createRequest()); err == nil {
		t.Fatal("expected provider duplicate error")
	}
}
