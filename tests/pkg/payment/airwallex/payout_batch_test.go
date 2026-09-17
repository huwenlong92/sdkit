//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex"
)

func TestBeneficiaryAndBatchPayoutLifecycle(t *testing.T) {
	beneficiaryID := "75d4cc8a-acde-42f5-af24-a97589bb68f1"
	batchID := "497bc88d-acde-4b2d-bd36-24d4af418cdc"
	itemID := "8e02ae88-acde-4a28-80f3-bf8e37535177"
	transferID := "dd042aac-acde-4a56-a22f-acde506a35ad"
	calls := make([]string, 0)
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		if res := login(r); res != nil {
			return res, nil
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		var response *http.Response
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/beneficiaries/create":
			body := mustBody(t, r)
			if body["nickname"] == nil || body["beneficiary"] == nil {
				t.Fatalf("beneficiary payload %s", body)
			}
			response = reply(200, fmt.Sprintf(`{"beneficiary_id":%q}`, beneficiaryID))
		case "GET /api/v1/beneficiaries/" + beneficiaryID:
			response = reply(200, fmt.Sprintf(`{"beneficiary_id":%q}`, beneficiaryID))
		case "POST /api/v1/batch_transfers/create":
			body := mustBody(t, r)
			if string(body["request_id"]) != fmt.Sprintf("%q", mutationID) {
				t.Fatalf("batch body %s", body)
			}
			response = reply(200, fmt.Sprintf(`{"id":%q,"status":"DRAFTING","total_item_count":0,"valid_item_count":0}`, batchID))
		case "POST /api/v1/batch_transfers/" + batchID + "/add_items":
			body := mustBody(t, r)
			if string(body["items"]) == "" {
				t.Fatalf("items body %s", body)
			}
			response = reply(200, fmt.Sprintf(`{"id":%q,"status":"DRAFTING","total_item_count":1,"valid_item_count":1}`, batchID))
		case "POST /api/v1/batch_transfers/" + batchID + "/submit":
			response = reply(200, fmt.Sprintf(`{"id":%q,"status":"SCHEDULED","total_item_count":1,"valid_item_count":1}`, batchID))
		case "GET /api/v1/batch_transfers/" + batchID:
			response = reply(200, fmt.Sprintf(`{"id":%q,"status":"BOOKING","total_item_count":1,"valid_item_count":1}`, batchID))
		case "GET /api/v1/batch_transfers/" + batchID + "/items":
			if r.URL.Query().Get("page_size") != "1000" {
				t.Fatal("page size")
			}
			response = reply(200, fmt.Sprintf(`{"items":[{"id":%q,"request_id":%q,"status":"BOOKED","transfer_id":%q}]}`, itemID, mutationID, transferID))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.String())
		}
		response.Header.Set("Request-Id", "provider-request")
		return response, nil
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
	base := struct {
		Provider    payment.Provider
		Channel     payment.Channel
		MerchantKey string
	}{payment.ProviderAirwallex, payment.ChannelAirwallexTransfer, "platform"}
	beneficiary, err := svc.CreateBeneficiary(context.Background(), payment.CreateBeneficiaryRequest{Provider: base.Provider, Channel: base.Channel, MerchantKey: base.MerchantKey, Details: map[string]any{"nickname": "fixture", "beneficiary": map[string]any{"entity_type": "COMPANY"}}})
	if err != nil || beneficiary.BeneficiaryID != beneficiaryID || len(beneficiary.Exchange.ResponseBody) == 0 {
		t.Fatalf("beneficiary=%+v err=%v", beneficiary, err)
	}
	if _, err := svc.QueryBeneficiary(context.Background(), payment.QueryBeneficiaryRequest{Provider: base.Provider, Channel: base.Channel, MerchantKey: base.MerchantKey, BeneficiaryID: beneficiaryID}); err != nil {
		t.Fatal(err)
	}
	batch, err := svc.CreatePayoutBatch(context.Background(), payment.CreatePayoutBatchRequest{Provider: base.Provider, Channel: base.Channel, MerchantKey: base.MerchantKey, RequestID: mutationID, Name: "fixture"})
	if err != nil || batch.ProviderBatchID != batchID || batch.ProviderStatus != "DRAFTING" {
		t.Fatalf("batch=%+v err=%v", batch, err)
	}
	batch, err = svc.AddPayoutBatchItems(context.Background(), payment.AddPayoutBatchItemsRequest{Provider: base.Provider, Channel: base.Channel, MerchantKey: base.MerchantKey, ProviderBatchID: batchID, Items: []payment.PayoutBatchItemRequest{{RequestID: mutationID, BeneficiaryID: beneficiaryID, Amount: payment.Money{Amount: 1234, Currency: "USD"}, TransferMethod: "LOCAL", Reference: "SET-1", Reason: "business_services"}}})
	if err != nil || batch.ValidItemCount != 1 {
		t.Fatalf("add=%+v err=%v", batch, err)
	}
	if _, err := svc.SubmitPayoutBatch(context.Background(), payment.SubmitPayoutBatchRequest{Provider: base.Provider, Channel: base.Channel, MerchantKey: base.MerchantKey, ProviderBatchID: batchID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QueryPayoutBatch(context.Background(), payment.QueryPayoutBatchRequest{Provider: base.Provider, Channel: base.Channel, MerchantKey: base.MerchantKey, ProviderBatchID: batchID}); err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListPayoutBatchItems(context.Background(), payment.ListPayoutBatchItemsRequest{Provider: base.Provider, Channel: base.Channel, MerchantKey: base.MerchantKey, ProviderBatchID: batchID})
	if err != nil || len(items.Items) != 1 || items.Items[0].ProviderPayoutID != transferID || len(calls) != 7 {
		t.Fatalf("items=%+v calls=%v err=%v", items, calls, err)
	}
}

func TestBatchPayoutRejectsMoreThanOneHundredItemsPerAdd(t *testing.T) {
	registry := payment.NewRegistry()
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]payment.PayoutBatchItemRequest, 101)
	_, err = svc.AddPayoutBatchItems(context.Background(), payment.AddPayoutBatchItemsRequest{ProviderBatchID: "fixture", Items: items})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBatchPayoutFailurePreservesProviderExchange(t *testing.T) {
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		if res := login(r); res != nil {
			return res, nil
		}
		response := reply(http.StatusUnprocessableEntity, `{"code":"invalid_beneficiary"}`)
		response.Header.Set("Request-Id", "failed-request")
		return response, nil
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
	_, err = svc.CreatePayoutBatch(context.Background(), payment.CreatePayoutBatchRequest{
		Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer,
		MerchantKey: "platform", RequestID: mutationID, Name: "failure fixture",
	})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	exchange, ok := payment.ProviderExchangeFromError(err)
	if !ok || exchange.StatusCode != http.StatusUnprocessableEntity || len(exchange.RequestBody) == 0 || string(exchange.ResponseBody) != `{"code":"invalid_beneficiary"}` {
		t.Fatalf("exchange=%+v ok=%v err=%v", exchange, ok, err)
	}
	if values := exchange.Headers["Request-Id"]; len(values) != 1 || values[0] != "failed-request" {
		t.Fatalf("headers=%v", exchange.Headers)
	}
}

func TestBatchPayoutTimeoutPreservesRequestEvidence(t *testing.T) {
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		if res := login(r); res != nil {
			return res, nil
		}
		return nil, context.DeadlineExceeded
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
	_, err = svc.CreatePayoutBatch(context.Background(), payment.CreatePayoutBatchRequest{
		Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer,
		MerchantKey: "platform", RequestID: mutationID, Name: "timeout fixture",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	exchange, ok := payment.ProviderExchangeFromError(err)
	if !ok || len(exchange.RequestBody) == 0 || len(exchange.ResponseBody) != 0 || exchange.StatusCode != 0 {
		t.Fatalf("exchange=%+v ok=%v", exchange, ok)
	}
}
