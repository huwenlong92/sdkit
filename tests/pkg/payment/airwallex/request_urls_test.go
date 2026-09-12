//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex/httpapi"
)

func TestPerPaymentReturnURLAndRegisteredWebhook(t *testing.T) {
	calls := 0
	cfg := config(func(r *http.Request) (*http.Response, error) {
		calls++
		if res := login(r); res != nil {
			return res, nil
		}
		body := mustBody(t, r)
		if string(body["return_url"]) != `"https://merchant.example.test/orders/order-42"` {
			t.Fatal("return URL override not sent")
		}
		if _, ok := body["notify_url"]; ok {
			t.Fatal("unsupported notify_url sent to Airwallex")
		}
		return reply(201, intentJSON("REQUIRES_PAYMENT_METHOD", "12.34", "USD")), nil
	})
	cfg.ReturnURL = ""
	cfg.NotifyURL = "https://merchant.example.test/api/webhook"
	client, err := httpapi.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := createRequest()
	req.ReturnURL = "https://merchant.example.test/orders/order-42"
	req.NotifyURL = cfg.NotifyURL
	res, err := client.CreatePayment(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action.Params["successUrl"] != req.ReturnURL {
		t.Fatal("checkout used a fixed return URL")
	}
	before := calls
	req.NotifyURL = "https://merchant.example.test/api/other-webhook"
	if _, err := client.CreatePayment(context.Background(), req); !errors.Is(err, payment.ErrUnsupportedCapability) {
		t.Fatalf("unsupported callback override: %v", err)
	}
	if calls != before {
		t.Fatal("unsupported override reached network")
	}
}
