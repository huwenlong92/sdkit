package payment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
)

func TestNormalizePricingDefaultsToCNY(t *testing.T) {
	got, err := payment.NormalizePricing(context.Background(), payment.PaymentPricing{
		PayAmount: payment.Money{Amount: 19900},
	})
	if err != nil {
		t.Fatalf("normalize pricing: %v", err)
	}
	if got.PayAmount != (payment.Money{Amount: 19900, Currency: payment.DefaultCurrency}) {
		t.Fatalf("pay amount = %+v", got.PayAmount)
	}
	if got.OrderAmount != got.PayAmount {
		t.Fatalf("order amount = %+v, want pay amount", got.OrderAmount)
	}
	if got.SettleCurrency != payment.DefaultCurrency {
		t.Fatalf("settle currency = %q", got.SettleCurrency)
	}
	if got.SettleAmount != got.PayAmount {
		t.Fatalf("settle amount = %+v, want pay amount", got.SettleAmount)
	}
	if got.ExchangeRate != nil {
		t.Fatalf("exchange rate = %+v, want nil", got.ExchangeRate)
	}
}

func TestNormalizePricingCalculatesCrossCurrencySettleAmount(t *testing.T) {
	quotedAt := time.Now().Unix()
	got, err := payment.NormalizePricing(context.Background(), payment.PaymentPricing{
		PayAmount:      payment.Money{Amount: 1000, Currency: "eur"},
		SettleCurrency: "cny",
		ExchangeRate: &payment.ExchangeRateSnapshot{
			FromCurrency: "EUR",
			ToCurrency:   "CNY",
			Rate:         "7.8000",
			Source:       "billing",
			QuotedAt:     quotedAt,
		},
	})
	if err != nil {
		t.Fatalf("normalize pricing: %v", err)
	}
	if got.PayAmount != (payment.Money{Amount: 1000, Currency: "EUR"}) {
		t.Fatalf("pay amount = %+v", got.PayAmount)
	}
	if got.SettleAmount != (payment.Money{Amount: 7800, Currency: "CNY"}) {
		t.Fatalf("settle amount = %+v, want 7800 CNY", got.SettleAmount)
	}
	if got.OrderAmount != got.PayAmount {
		t.Fatalf("order amount = %+v, want pay amount", got.OrderAmount)
	}
}

func TestNormalizePricingRequiresExchangeRateForCrossCurrency(t *testing.T) {
	_, err := payment.NormalizePricing(context.Background(), payment.PaymentPricing{
		PayAmount:      payment.Money{Amount: 1000, Currency: "EUR"},
		SettleCurrency: "CNY",
	})
	if !errors.Is(err, payment.ErrInvalidExchangeRate) {
		t.Fatalf("err = %v, want ErrInvalidExchangeRate", err)
	}
}

func TestNormalizePricingRejectsMismatchedExplicitSettleAmount(t *testing.T) {
	_, err := payment.NormalizePricing(context.Background(), payment.PaymentPricing{
		PayAmount:      payment.Money{Amount: 1000, Currency: "EUR"},
		SettleCurrency: "CNY",
		SettleAmount:   payment.Money{Amount: 7900, Currency: "CNY"},
		ExchangeRate: &payment.ExchangeRateSnapshot{
			FromCurrency: "EUR",
			ToCurrency:   "CNY",
			Rate:         "7.8000",
		},
	})
	if !errors.Is(err, payment.ErrSettleAmountMismatch) {
		t.Fatalf("err = %v, want ErrSettleAmountMismatch", err)
	}
}

func TestNormalizePricingUsesCurrencyMinorExp(t *testing.T) {
	got, err := payment.NormalizePricing(context.Background(), payment.PaymentPricing{
		PayAmount:      payment.Money{Amount: 1000, Currency: "CNY"},
		SettleCurrency: "JPY",
		ExchangeRate: &payment.ExchangeRateSnapshot{
			FromCurrency: "CNY",
			ToCurrency:   "JPY",
			Rate:         "20.123",
		},
	})
	if err != nil {
		t.Fatalf("normalize pricing: %v", err)
	}
	if got.SettleAmount != (payment.Money{Amount: 201, Currency: "JPY"}) {
		t.Fatalf("settle amount = %+v, want 201 JPY", got.SettleAmount)
	}
}

func TestNormalizePricingSupportsCustomCurrencyMeta(t *testing.T) {
	policy := payment.NewDefaultPricingPolicy()
	policy.Currencies["TST"] = payment.CurrencyMeta{Code: "TST", MinorExp: 3}

	got, err := policy.NormalizePricing(context.Background(), payment.PaymentPricing{
		PayAmount:      payment.Money{Amount: 1234, Currency: "TST"},
		SettleCurrency: "CNY",
		ExchangeRate: &payment.ExchangeRateSnapshot{
			FromCurrency: "TST",
			ToCurrency:   "CNY",
			Rate:         "2",
		},
	})
	if err != nil {
		t.Fatalf("normalize pricing: %v", err)
	}
	if got.SettleAmount != (payment.Money{Amount: 247, Currency: "CNY"}) {
		t.Fatalf("settle amount = %+v, want 247 CNY", got.SettleAmount)
	}
}

func TestNormalizePricingSupportsCustomCurrencyMetaWithoutCode(t *testing.T) {
	policy := payment.NewDefaultPricingPolicy()
	policy.Currencies["TST"] = payment.CurrencyMeta{MinorExp: 3}

	got, err := policy.NormalizePricing(context.Background(), payment.PaymentPricing{
		PayAmount:      payment.Money{Amount: 1234, Currency: "TST"},
		SettleCurrency: "TST",
	})
	if err != nil {
		t.Fatalf("normalize pricing: %v", err)
	}
	if got.PayAmount.Currency != "TST" || got.SettleAmount.Currency != "TST" {
		t.Fatalf("pricing = %+v, want TST currencies", got)
	}
}

func TestCNYHelper(t *testing.T) {
	got, err := payment.NormalizePricing(context.Background(), payment.CNY(500))
	if err != nil {
		t.Fatalf("normalize pricing: %v", err)
	}
	if got.PayAmount != (payment.Money{Amount: 500, Currency: "CNY"}) {
		t.Fatalf("pay amount = %+v", got.PayAmount)
	}
}
