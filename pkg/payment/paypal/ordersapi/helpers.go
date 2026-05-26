package ordersapi

import (
	"fmt"
	"strconv"
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

func description(req payment.CreatePaymentRequest) string {
	return firstNonEmpty(req.Subject, req.Body, req.OutTradeNo, req.PaymentID, req.OrderID)
}

func optionalInvoiceID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 127 {
		return value[:127]
	}
	return value
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

func amountFromMoney(money payment.Money) (amountPayload, error) {
	if money.Amount <= 0 {
		return amountPayload{}, fmt.Errorf("%w: paypal amount must be positive", payment.ErrInvalidAmount)
	}
	currency := payment.NormalizeCurrency(money.Currency)
	meta, ok := payment.DefaultCurrencyMeta(currency)
	if !ok {
		return amountPayload{}, fmt.Errorf("%w: paypal currency %s", payment.ErrCurrencyMetaNotFound, currency)
	}
	return amountPayload{
		CurrencyCode: currency,
		Value:        formatMinorAmount(money.Amount, meta.MinorExp),
	}, nil
}

func moneyFromAmount(amount amountPayload) (payment.Money, error) {
	currency := payment.NormalizeCurrency(amount.CurrencyCode)
	meta, ok := payment.DefaultCurrencyMeta(currency)
	if !ok {
		return payment.Money{}, fmt.Errorf("%w: paypal currency %s", payment.ErrCurrencyMetaNotFound, currency)
	}
	minor, err := parseDecimalAmount(amount.Value, meta.MinorExp)
	if err != nil {
		return payment.Money{}, fmt.Errorf("%w: parse paypal amount %q: %v", payment.ErrInvalidAmount, amount.Value, err)
	}
	return payment.Money{Amount: minor, Currency: currency}, nil
}

func mustMoneyFromAmount(amount amountPayload, fallback payment.Money) payment.Money {
	money, err := moneyFromAmount(amount)
	if err != nil {
		return fallback
	}
	return money
}

func formatMinorAmount(amount int64, exp int) string {
	if exp <= 0 {
		return strconv.FormatInt(amount, 10)
	}
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	scale := int64(1)
	for i := 0; i < exp; i++ {
		scale *= 10
	}
	whole := amount / scale
	fraction := amount % scale
	return fmt.Sprintf("%s%d.%0*d", sign, whole, exp, fraction)
}

func parseDecimalAmount(value string, exp int) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty amount")
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	parts := strings.SplitN(value, ".", 2)
	whole := parts[0]
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > exp {
		fraction = fraction[:exp]
	}
	for len(fraction) < exp {
		fraction += "0"
	}
	combined := strings.TrimLeft(whole+fraction, "0")
	if combined == "" {
		combined = "0"
	}
	amount, err := strconv.ParseInt(combined, 10, 64)
	if err != nil {
		return 0, err
	}
	if negative {
		amount = -amount
	}
	return amount, nil
}

func linkByRel(links []linkPayload, rel string) string {
	for _, link := range links {
		if strings.EqualFold(link.Rel, rel) {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func firstCaptureID(raw captureOrderResponse) string {
	for _, unit := range raw.PurchaseUnits {
		for _, capture := range unit.Payments.Captures {
			if strings.TrimSpace(capture.ID) != "" {
				return strings.TrimSpace(capture.ID)
			}
		}
	}
	return ""
}

func orderStatus(status string) payment.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED":
		return payment.PaymentSucceeded
	case "APPROVED":
		return payment.PaymentAuthorized
	case "PAYER_ACTION_REQUIRED":
		return payment.PaymentRequiresAction
	case "VOIDED":
		return payment.PaymentClosed
	case "CREATED", "SAVED":
		return payment.PaymentPending
	default:
		return payment.PaymentPending
	}
}

func refundStatus(status string) payment.RefundStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED":
		return payment.RefundSucceeded
	case "PENDING":
		return payment.RefundProcessing
	case "CANCELLED":
		return payment.RefundClosed
	case "FAILED":
		return payment.RefundFailed
	default:
		return payment.RefundPending
	}
}
