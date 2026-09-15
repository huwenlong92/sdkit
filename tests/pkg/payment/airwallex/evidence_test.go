//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
)

func TestQueryPreservesPrivateRawBytesAndActualPaymentDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		card   string
		last4  string
	}{
		{name: "card", method: "card", card: `,"card":{"brand":"visa","last4":"0008","unknown_card_field":"retained-only-in-raw"}`, last4: "0008"},
		{name: "alipay", method: "alipaycn"},
		{name: "wechat", method: "wechatpay"},
		{name: "malformed-card-tail", method: "card", card: `,"card":{"brand":"visa","last4":"4242424242424242"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := " {\n" + strings.TrimPrefix(intentJSON("SUCCEEDED", "12.34", "USD"), "{")
			body = strings.TrimSuffix(body, "}") + `,"unknown_accounting_field":{"fee_currency":"USD","fee":"0.44"},"latest_payment_attempt":{"id":"att_fixture","payment_method_transaction_id":"method_tx_fixture","payment_method":{"type":"` + tc.method + `"` + tc.card + `}}}` + "\n"
			client := newClient(t, func(r *http.Request) (*http.Response, error) {
				if result := login(r); result != nil {
					return result, nil
				}
				return reply(200, body), nil
			})
			result, err := client.QueryPayment(context.Background(), queryRequest())
			if err != nil {
				t.Fatal(err)
			}
			if string(result.RawBody) != body {
				t.Fatal("original provider bytes were altered or fields lost")
			}
			if result.Details == nil || result.Details.Method != tc.method || result.Details.AttemptID != "att_fixture" || result.Details.TransactionID != "method_tx_fixture" || result.Details.CardLast4 != tc.last4 {
				t.Fatalf("details=%+v, want actual method/attempt/transaction and tail %q", result.Details, tc.last4)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "fictional-checkout-token") || strings.Contains(string(encoded), "unknown_accounting_field") {
				t.Fatal("private original response escaped through JSON serialization")
			}
			event := payment.EventFromQueryPaymentResponse(result)
			if event == nil || string(event.Raw) != body || event.Details == nil || event.Details.Method != tc.method {
				t.Fatal("query normalization lost accounting evidence")
			}
			result.RawBody[0] = 'x'
			if string(event.Raw) != body {
				t.Fatal("query event aliases mutable response bytes")
			}
		})
	}
}

func TestQueryWithoutAttemptDoesNotInventPaymentMethod(t *testing.T) {
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		if result := login(r); result != nil {
			return result, nil
		}
		return reply(200, intentJSON("SUCCEEDED", "12.34", "USD")), nil
	})
	result, err := client.QueryPayment(context.Background(), queryRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Details != nil {
		t.Fatalf("invented payment details: %+v", result.Details)
	}
}
