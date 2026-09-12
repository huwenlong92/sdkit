package payment

import "time"

// ProviderSettlement contains only facts supplied by the provider. It is optional
// and must never be populated from caller-side Pricing calculations.
type ProviderSettlement struct {
	Currency     string                `json:"currency,omitempty"`
	Amount       *Money                `json:"amount,omitempty"`
	NetAmount    *Money                `json:"net_amount,omitempty"`
	FeeAmount    *Money                `json:"fee_amount,omitempty"`
	ExchangeRate *ExchangeRateSnapshot `json:"exchange_rate,omitempty"`
	Reference    string                `json:"reference,omitempty"`
	UpdatedAt    *time.Time            `json:"updated_at,omitempty"`
}
