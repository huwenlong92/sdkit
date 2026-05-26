package payment

import (
	"context"
	"fmt"
	"math/big"
	"strings"
)

type DefaultPricingPolicy struct {
	DefaultSettleCurrency string
	Currencies            map[string]CurrencyMeta
	AllowExplicitSettle   bool
}

func NewDefaultPricingPolicy() DefaultPricingPolicy {
	return DefaultPricingPolicy{
		DefaultSettleCurrency: DefaultCurrency,
		Currencies:            DefaultCurrencyMetadata(),
	}
}

func NormalizePricing(ctx context.Context, pricing PaymentPricing) (PaymentPricing, error) {
	return NewDefaultPricingPolicy().NormalizePricing(ctx, pricing)
}

func (p DefaultPricingPolicy) NormalizePricing(ctx context.Context, pricing PaymentPricing) (PaymentPricing, error) {
	if ctx == nil {
		return PaymentPricing{}, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return PaymentPricing{}, err
	}
	if pricing.PayAmount.Amount <= 0 {
		return PaymentPricing{}, fmt.Errorf("%w: pay amount must be positive", ErrInvalidAmount)
	}

	payCurrency := NormalizeCurrency(pricing.PayAmount.Currency)
	settleCurrency := NormalizeCurrency(pricing.SettleCurrency)
	if p.DefaultSettleCurrency != "" && strings.TrimSpace(pricing.SettleCurrency) == "" {
		settleCurrency = NormalizeCurrency(p.DefaultSettleCurrency)
	}

	payMeta, err := p.currencyMeta(payCurrency)
	if err != nil {
		return PaymentPricing{}, err
	}
	settleMeta, err := p.currencyMeta(settleCurrency)
	if err != nil {
		return PaymentPricing{}, err
	}

	pricing.PayAmount.Currency = payCurrency
	pricing.SettleCurrency = settleCurrency

	if pricing.OrderAmount.Amount == 0 && strings.TrimSpace(pricing.OrderAmount.Currency) == "" {
		pricing.OrderAmount = pricing.PayAmount
	} else {
		if pricing.OrderAmount.Amount <= 0 {
			return PaymentPricing{}, fmt.Errorf("%w: order amount must be positive", ErrInvalidAmount)
		}
		pricing.OrderAmount.Currency = NormalizeCurrency(pricing.OrderAmount.Currency)
		if _, err := p.currencyMeta(pricing.OrderAmount.Currency); err != nil {
			return PaymentPricing{}, err
		}
	}

	settleAmount, err := p.calculateSettleAmount(pricing.PayAmount, payMeta, settleMeta, pricing.ExchangeRate)
	if err != nil {
		return PaymentPricing{}, err
	}

	if pricing.SettleAmount.Amount == 0 && strings.TrimSpace(pricing.SettleAmount.Currency) == "" {
		pricing.SettleAmount = settleAmount
	} else {
		if pricing.SettleAmount.Amount <= 0 {
			return PaymentPricing{}, fmt.Errorf("%w: settle amount must be positive", ErrInvalidAmount)
		}
		pricing.SettleAmount.Currency = NormalizeCurrency(pricing.SettleAmount.Currency)
		if pricing.SettleAmount.Currency != settleCurrency {
			return PaymentPricing{}, fmt.Errorf("%w: settle amount currency %s does not match settle currency %s", ErrInvalidCurrency, pricing.SettleAmount.Currency, settleCurrency)
		}
		if !p.AllowExplicitSettle && pricing.SettleAmount != settleAmount {
			return PaymentPricing{}, fmt.Errorf("%w: got %d %s, want %d %s", ErrSettleAmountMismatch, pricing.SettleAmount.Amount, pricing.SettleAmount.Currency, settleAmount.Amount, settleAmount.Currency)
		}
	}

	if pricing.FeeAmount != nil {
		if pricing.FeeAmount.Amount < 0 {
			return PaymentPricing{}, fmt.Errorf("%w: fee amount must not be negative", ErrInvalidAmount)
		}
		pricing.FeeAmount.Currency = NormalizeCurrency(pricing.FeeAmount.Currency)
		if _, err := p.currencyMeta(pricing.FeeAmount.Currency); err != nil {
			return PaymentPricing{}, err
		}
	}

	return pricing, nil
}

func (p DefaultPricingPolicy) currencyMeta(code string) (CurrencyMeta, error) {
	code = NormalizeCurrency(code)
	if p.Currencies != nil {
		if meta, ok := p.Currencies[code]; ok {
			if strings.TrimSpace(meta.Code) == "" {
				meta.Code = code
			} else {
				meta.Code = NormalizeCurrency(meta.Code)
			}
			if meta.MinorExp < 0 {
				return CurrencyMeta{}, fmt.Errorf("%w: %s", ErrInvalidCurrency, code)
			}
			return meta, nil
		}
	}
	if meta, ok := DefaultCurrencyMeta(code); ok {
		return meta, nil
	}
	return CurrencyMeta{}, fmt.Errorf("%w: %s", ErrCurrencyMetaNotFound, code)
}

func (p DefaultPricingPolicy) calculateSettleAmount(pay Money, payMeta, settleMeta CurrencyMeta, rate *ExchangeRateSnapshot) (Money, error) {
	if pay.Currency == settleMeta.Code {
		return Money{Amount: pay.Amount, Currency: settleMeta.Code}, nil
	}
	if rate == nil {
		return Money{}, fmt.Errorf("%w: exchange rate is required for %s to %s", ErrInvalidExchangeRate, pay.Currency, settleMeta.Code)
	}
	fromCurrency := NormalizeCurrency(rate.FromCurrency)
	toCurrency := NormalizeCurrency(rate.ToCurrency)
	if fromCurrency != pay.Currency || toCurrency != settleMeta.Code {
		return Money{}, fmt.Errorf("%w: exchange rate direction must be %s to %s", ErrInvalidExchangeRate, pay.Currency, settleMeta.Code)
	}
	rateValue, err := parsePositiveDecimalRat(rate.Rate)
	if err != nil {
		return Money{}, err
	}

	source := big.NewRat(pay.Amount, 1)
	source.Quo(source, pow10Rat(payMeta.MinorExp))
	source.Mul(source, rateValue)
	source.Mul(source, pow10Rat(settleMeta.MinorExp))

	amount, err := roundHalfUpRatToInt64(source)
	if err != nil {
		return Money{}, err
	}
	if amount <= 0 {
		return Money{}, fmt.Errorf("%w: calculated settle amount must be positive", ErrInvalidAmount)
	}
	return Money{Amount: amount, Currency: settleMeta.Code}, nil
}

func parsePositiveDecimalRat(raw string) (*big.Rat, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: empty rate", ErrInvalidExchangeRate)
	}
	negative := strings.HasPrefix(raw, "-")
	if strings.HasPrefix(raw, "+") || negative {
		raw = raw[1:]
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" && len(parts) == 1 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidExchangeRate, raw)
	}
	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if intPart == "" {
		intPart = "0"
	}
	digits := intPart + fracPart
	if digits == "" {
		return nil, fmt.Errorf("%w: empty rate", ErrInvalidExchangeRate)
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("%w: %s", ErrInvalidExchangeRate, raw)
		}
	}
	n := new(big.Int)
	if _, ok := n.SetString(digits, 10); !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidExchangeRate, raw)
	}
	if negative || n.Sign() <= 0 {
		return nil, fmt.Errorf("%w: rate must be positive", ErrInvalidExchangeRate)
	}
	return new(big.Rat).SetFrac(n, pow10Int(len(fracPart))), nil
}

func pow10Rat(exp int) *big.Rat {
	return new(big.Rat).SetInt(pow10Int(exp))
}

func pow10Int(exp int) *big.Int {
	n := big.NewInt(1)
	if exp <= 0 {
		return n
	}
	return n.Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
}

func roundHalfUpRatToInt64(v *big.Rat) (int64, error) {
	if v.Sign() < 0 {
		return 0, fmt.Errorf("%w: negative calculated amount", ErrInvalidAmount)
	}
	n := new(big.Int).Set(v.Num())
	d := new(big.Int).Set(v.Denom())
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	r.Mul(r, big.NewInt(2))
	if r.Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("%w: amount overflow", ErrInvalidAmount)
	}
	return q.Int64(), nil
}
