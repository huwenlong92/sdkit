package payment

// MergeProviderSettlement preserves optional fields and rejects older/conflicting updates.
// Neither input is modified. It never calculates settlement amounts or exchange rates.
func MergeProviderSettlement(current, incoming *ProviderSettlement) *ProviderSettlement {
	if incoming == nil {
		return current
	}
	var merged ProviderSettlement
	if current != nil {
		merged = *current
	}
	original := current
	current = &merged
	newer := current.UpdatedAt == nil || (incoming.UpdatedAt != nil && incoming.UpdatedAt.After(*current.UpdatedAt))
	if current.UpdatedAt != nil && incoming.UpdatedAt != nil && incoming.UpdatedAt.Before(*current.UpdatedAt) {
		return original
	}
	mergeMoney := func(dst **Money, src *Money) {
		if src != nil && src.Currency != "" && src.Amount >= 0 && (*dst == nil || (newer && incoming.UpdatedAt != nil)) {
			copy := *src
			*dst = &copy
		}
	}
	if incoming.Currency != "" && (current.Currency == "" || (newer && incoming.UpdatedAt != nil)) {
		current.Currency = incoming.Currency
	}
	mergeMoney(&current.Amount, incoming.Amount)
	mergeMoney(&current.NetAmount, incoming.NetAmount)
	mergeMoney(&current.FeeAmount, incoming.FeeAmount)
	if incoming.ExchangeRate != nil && (current.ExchangeRate == nil || (newer && incoming.UpdatedAt != nil)) {
		copy := *incoming.ExchangeRate
		current.ExchangeRate = &copy
	}
	if incoming.Reference != "" && (current.Reference == "" || (newer && incoming.UpdatedAt != nil)) {
		current.Reference = incoming.Reference
	}
	if incoming.UpdatedAt != nil && newer {
		value := *incoming.UpdatedAt
		current.UpdatedAt = &value
	}
	if current.Amount != nil && current.Currency != "" && current.Amount.Currency != current.Currency {
		return original
	}
	return current
}
