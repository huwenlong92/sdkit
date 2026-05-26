package stripego

import (
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/core/payment"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringPtr(value string) *string {
	return &value
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringPtrs(values []string) []*string {
	out := make([]*string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, stringPtr(value))
		}
	}
	return out
}

func description(req payment.CreatePaymentRequest) string {
	return firstNonEmpty(req.Subject, req.Body, req.OutTradeNo, req.PaymentID, req.OrderID)
}

func reference(req payment.CreatePaymentRequest) string {
	return firstNonEmpty(req.OutTradeNo, req.PaymentID, req.OrderID)
}

func stripeCurrency(currency string) string {
	return strings.ToLower(payment.NormalizeCurrency(currency))
}

func metadata(req payment.CreatePaymentRequest) map[string]string {
	return compactMetadata(map[string]string{
		"merchant_key":  req.MerchantKey,
		"payment_id":    req.PaymentID,
		"order_id":      req.OrderID,
		"out_trade_no":  req.OutTradeNo,
		"notify_url":    req.NotifyURL,
		"return_url":    req.ReturnURL,
		"payment_subj":  req.Subject,
		"payment_body":  req.Body,
		"pay_amount":    fmt.Sprintf("%d", req.Pricing.PayAmount.Amount),
		"pay_currency":  payment.NormalizeCurrency(req.Pricing.PayAmount.Currency),
		"settle_amount": fmt.Sprintf("%d", req.Pricing.SettleAmount.Amount),
	})
}

func compactMetadata(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		value = strings.TrimSpace(value)
		if value != "" && value != "0" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringExtra(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func stringSliceExtra(extra map[string]any, key string) []string {
	if extra == nil {
		return nil
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return normalizeStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, strings.TrimSpace(fmt.Sprint(item)))
		}
		return normalizeStrings(values)
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return normalizeStrings(strings.Split(typed, ","))
	default:
		return normalizeStrings([]string{fmt.Sprint(typed)})
	}
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
