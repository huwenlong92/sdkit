//go:build sdkit_payment_airwallex

package airwallex_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/pkg/payment/airwallex"
)

func TestGenerateBeneficiarySchemas(t *testing.T) {
	paths := []string{
		"/api/v1/beneficiary_api_schemas/generate",
		"/api/v1/beneficiary_form_schemas/generate",
	}
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		if res := login(r); res != nil {
			return res, nil
		}
		if r.Method != http.MethodPost || len(paths) == 0 || r.URL.Path != paths[0] {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		paths = paths[1:]
		if r.Header.Get("x-api-version") != "2024-09-27" {
			t.Fatalf("x-api-version = %q", r.Header.Get("x-api-version"))
		}
		body := mustBody(t, r)
		if string(body["bank_country_code"]) != `"CN"` || string(body["account_currency"]) != `"CNY"` || string(body["transfer_method"]) != `"LOCAL"` || string(body["entity_type"]) != `"COMPANY"` || string(body["country_code"]) != `"CN"` {
			t.Fatalf("schema body = %s", body)
		}
		if _, ok := body["local_clearing_system"]; ok {
			t.Fatal("empty local clearing system must be omitted")
		}
		response := reply(http.StatusOK, `{"condition":{"bank_country_code":"CN","account_currency":"CNY"},"fields":[{"key":"account_name","path":"beneficiary.bank_details.account_name","required":true,"rule":{"type":"string"}}]}`)
		response.Header.Set("Request-Id", "schema-request")
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
	for _, kind := range []payment.BeneficiarySchemaKind{payment.BeneficiarySchemaAPI, payment.BeneficiarySchemaForm} {
		result, err := svc.GenerateBeneficiarySchema(context.Background(), payment.GenerateBeneficiarySchemaRequest{
			Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, MerchantKey: "platform", Kind: kind,
			BankCountryCode: "CN", AccountCurrency: "CNY", TransferMethod: "LOCAL", EntityType: "COMPANY", CountryCode: "CN",
		})
		if err != nil || result.Kind != kind || len(result.Fields) != 1 || result.Condition["bank_country_code"] != "CN" || len(result.Exchange.RequestBody) == 0 || len(result.Exchange.ResponseBody) == 0 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if values := result.Exchange.Headers["Request-Id"]; len(values) != 1 || values[0] != "schema-request" {
			t.Fatalf("headers=%v", result.Exchange.Headers)
		}
	}
	if len(paths) != 0 {
		t.Fatalf("unvisited paths = %v", paths)
	}
}

func TestGenerateBeneficiarySchemaRejectsInvalidConditionsBeforeProviderCall(t *testing.T) {
	registry := payment.NewRegistry()
	adapter, err := airwallex.NewAdapter(airwallex.Config{ClientMode: airwallex.ClientModeStatic, Client: newClient(t, func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected provider call %s", r.URL.Path)
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	svc, err := payment.NewService(payment.ServiceConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.GenerateBeneficiarySchema(context.Background(), payment.GenerateBeneficiarySchemaRequest{
		Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, Kind: payment.BeneficiarySchemaAPI,
		BankCountryCode: "cn", AccountCurrency: "CNY", TransferMethod: "LOCAL", EntityType: "COMPANY",
	})
	if !errors.Is(err, payment.ErrInvalidRequest) {
		t.Fatalf("err=%v", err)
	}
}

func TestGenerateBeneficiarySchemaRejectsIncompleteProviderResponse(t *testing.T) {
	client := newClient(t, func(r *http.Request) (*http.Response, error) {
		if res := login(r); res != nil {
			return res, nil
		}
		return reply(http.StatusOK, `{"condition":{}}`), nil
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
	_, err = svc.GenerateBeneficiarySchema(context.Background(), payment.GenerateBeneficiarySchemaRequest{
		Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, Kind: payment.BeneficiarySchemaAPI,
	})
	if !errors.Is(err, payment.ErrPaymentReference) {
		t.Fatalf("err=%v", err)
	}
	if exchange, ok := payment.ProviderExchangeFromError(err); !ok || exchange.StatusCode != http.StatusOK || len(exchange.ResponseBody) == 0 {
		t.Fatalf("exchange=%+v ok=%v", exchange, ok)
	}
}
