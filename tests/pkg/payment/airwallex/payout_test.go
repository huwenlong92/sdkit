//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex"
)

func TestPayoutUsesSharedRequestAndPreservesIdentity(t *testing.T) {
	id := "99c36dff-4f21-49f9-9ffa-6698323772c5"
	calls := 0
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		if res := login(r); res != nil {
			return res, nil
		}
		if r.Header.Get("x-api-version") != "2024-09-27" {
			t.Fatal("transfer API version must be pinned")
		}
		calls++
		status := "SENT"
		if r.Method == "POST" {
			if r.URL.Path != "/api/v1/transfers/create" {
				t.Fatalf("path %s", r.URL.Path)
			}
			body := mustBody(t, r)
			if string(body["transfer_amount"]) != "12.34" || string(body["source_currency"]) != `"USD"` || string(body["request_id"]) != fmt.Sprintf("%q", mutationID) {
				t.Fatalf("payload %s", body)
			}
			status = "SCHEDULED"
		} else if r.Method != "GET" || r.URL.Path != "/api/v1/transfers/"+id {
			t.Fatalf("query %s %s", r.Method, r.URL.Path)
		}
		return reply(200, fmt.Sprintf(`{"id":%q,"request_id":%q,"status":%q,"transfer_amount":12.34,"transfer_currency":"USD"}`, id, mutationID, status)), nil
	})
	adapter, err := airwallex.NewAdapter(airwallex.Config{ClientMode: airwallex.ClientModeStatic, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	registry := payment.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	req := payment.CreatePayoutRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, PayoutID: "payout-1", RequestID: mutationID, BeneficiaryID: id, Amount: payment.Money{Amount: 1234, Currency: "USD"}, Reason: "goods_purchased", Extra: map[string]any{"transfer_method": "LOCAL"}}
	result, err := svc.CreatePayout(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != payment.PayoutPending || result.ProviderPayoutID != id {
		t.Fatalf("create %+v", result)
	}
	result, err = svc.QueryPayout(context.Background(), payment.QueryPayoutRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, ProviderPayoutID: id})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != payment.PayoutSent || calls != 2 {
		t.Fatalf("sent incorrectly treated as paid: %+v", result)
	}
}

func TestCallbackUsesRequestedMerchant(t *testing.T) {
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		t.Fatal("callback must not make HTTP calls")
		return nil, nil
	})
	var loaded string
	adapter, err := airwallex.NewAdapter(airwallex.Config{NotifyMerchantKey: "old", ClientLoader: airwallex.ClientLoaderFunc(func(ctx context.Context, key string) (airwallex.Client, airwallex.ClientCleanup, error) {
		loaded = key
		return client, nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	// Invalid signature is expected; merchant selection must still use the trusted request key.
	_, err = adapter.ParseNotify(context.Background(), payment.NotifyRequest{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "second", Body: []byte(`{}`)})
	if err == nil || loaded != "second" {
		t.Fatalf("callback merchant=%s err=%v", loaded, err)
	}
}
