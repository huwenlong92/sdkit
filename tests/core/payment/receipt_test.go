package payment_test

import (
	payment "github.com/huwenlong92/sdkit/core/payment"
	"testing"
	"time"
)

func TestReceiptIdentityRejectsCrossAccountAndMalformedMetadata(t *testing.T) {
	expected := payment.PaymentEvent{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexHPP, MerchantKey: "account_one", OutTradeNo: "payment_one", ProviderTradeID: "remote_one", Extra: map[string]any{"account_id": "one", "environment": "sandbox"}}
	if err := expected.ValidateIdentity(expected); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*payment.PaymentEvent){
		func(e *payment.PaymentEvent) { e.MerchantKey = "account_two" },
		func(e *payment.PaymentEvent) { e.ProviderTradeID = "remote_two" },
		func(e *payment.PaymentEvent) { e.Extra = map[string]any{"account_id": "two"} },
		func(e *payment.PaymentEvent) { e.Extra = map[string]any{"account_id": []string{"one"}} },
	} {
		actual := expected
		change(&actual)
		if actual.ValidateIdentity(expected) == nil {
			t.Fatal("mismatched account accepted")
		}
	}
	var absent *payment.PaymentEvent
	if absent.ValidateIdentity(expected) == nil {
		t.Fatal("nil event accepted")
	}
}

func TestMergeSettlementKeepsPartialAndZeroFacts(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	next := now.Add(time.Hour)
	current := &payment.ProviderSettlement{Currency: "USD", Amount: &payment.Money{Amount: 10000, Currency: "USD"}, UpdatedAt: &now}
	incoming := &payment.ProviderSettlement{FeeAmount: &payment.Money{Amount: 0, Currency: "USD"}, UpdatedAt: &next}
	merged := payment.MergeProviderSettlement(current, incoming)
	if merged.Amount.Amount != 10000 || merged.FeeAmount == nil || merged.FeeAmount.Amount != 0 || merged.NetAmount != nil || merged.ExchangeRate != nil {
		t.Fatal("optional fields lost or fabricated")
	}
	if current.FeeAmount != nil || incoming.Amount != nil {
		t.Fatal("merge mutated input")
	}
	stale := payment.MergeProviderSettlement(merged, &payment.ProviderSettlement{Amount: &payment.Money{Amount: 99, Currency: "USD"}, UpdatedAt: &past})
	if stale.Amount.Amount != 10000 {
		t.Fatal("older settlement overwrote current data")
	}
	inconsistent := payment.MergeProviderSettlement(merged, &payment.ProviderSettlement{Currency: "CNY", UpdatedAt: &next})
	if inconsistent.Currency != "USD" {
		t.Fatal("inconsistent currency accepted")
	}
	if payment.MergeProviderSettlement(nil, nil) != nil {
		t.Fatal("absent settlement fabricated")
	}
}
