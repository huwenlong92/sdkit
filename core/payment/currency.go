package payment

import "strings"

var defaultCurrencyMeta = map[string]CurrencyMeta{
	"CNY": {Code: "CNY", MinorExp: 2},
	"USD": {Code: "USD", MinorExp: 2},
	"EUR": {Code: "EUR", MinorExp: 2},
	"GBP": {Code: "GBP", MinorExp: 2},
	"HKD": {Code: "HKD", MinorExp: 2},
	"MOP": {Code: "MOP", MinorExp: 2},
	"TWD": {Code: "TWD", MinorExp: 2},
	"SGD": {Code: "SGD", MinorExp: 2},
	"AUD": {Code: "AUD", MinorExp: 2},
	"CAD": {Code: "CAD", MinorExp: 2},
	"JPY": {Code: "JPY", MinorExp: 0},
	"KRW": {Code: "KRW", MinorExp: 0},
}

func NormalizeCurrency(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return DefaultCurrency
	}
	return strings.ToUpper(code)
}

func DefaultCurrencyMeta(code string) (CurrencyMeta, bool) {
	meta, ok := defaultCurrencyMeta[NormalizeCurrency(code)]
	return meta, ok
}

func DefaultCurrencyMetadata() map[string]CurrencyMeta {
	out := make(map[string]CurrencyMeta, len(defaultCurrencyMeta))
	for code, meta := range defaultCurrencyMeta {
		out[code] = meta
	}
	return out
}

func CNY(amount int64) PaymentPricing {
	return PaymentPricing{
		PayAmount: Money{Amount: amount, Currency: DefaultCurrency},
	}
}
