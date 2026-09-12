package payment

import (
	"fmt"
	"time"
)

// ValidateIdentity checks a verified or queried receipt against the original payment.
// expected.Extra may bind provider-specific account_id and environment values.
// This does not verify a webhook signature or validate the amount.
func (event *PaymentEvent) ValidateIdentity(expected PaymentEvent) error {
	if event == nil || expected.MerchantKey == "" || expected.OutTradeNo == "" || event.Provider != expected.Provider || event.Channel != expected.Channel || event.MerchantKey != expected.MerchantKey || event.OutTradeNo != expected.OutTradeNo || event.ProviderTradeID == "" || (expected.ProviderTradeID != "" && event.ProviderTradeID != expected.ProviderTradeID) {
		return fmt.Errorf("%w: payment receipt identity mismatch", ErrInvalidRequest)
	}
	for _, key := range []string{"account_id", "environment"} {
		if value, exists := event.Extra[key]; exists {
			actual, ok := value.(string)
			bound, boundOK := expected.Extra[key].(string)
			if !ok || !boundOK || actual != bound {
				return fmt.Errorf("%w: payment receipt %s mismatch", ErrInvalidRequest, key)
			}
		}
	}
	return nil
}

// PaymentDeadline returns the earlier caller/provider deadline without changing either.
func PaymentDeadline(local, provider *time.Time) *time.Time {
	if provider != nil && (local == nil || provider.Before(*local)) {
		return provider
	}
	return local
}

// PaymentExpired does not imply that the provider has closed the payment.
func IsPaymentExpired(local, provider *time.Time, now time.Time) bool {
	deadline := PaymentDeadline(local, provider)
	return deadline != nil && !now.Before(*deadline)
}

// PaymentClosed reports a provider-confirmed unsuccessful terminal state.
func IsPaymentClosed(status PaymentStatus) bool {
	return status == PaymentClosed || status == PaymentExpired || status == PaymentFailed
}
