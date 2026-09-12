//go:build sdkit_payment_airwallex

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/huwenlong92/sdkit/core/payment"
)

var decimalPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,3})?$`)

func requestID(extra map[string]any) (string, error) {
	value, ok := extra[ExtraRequestIDKey].(string)
	id, err := uuid.Parse(value)
	if !ok || err != nil || id.Version() != 4 || id.Variant() != uuid.RFC4122 || id.String() != value {
		return "", fmt.Errorf("%w: persist a canonical v4 UUID in Extra[request_id] before each mutation", payment.ErrInvalidRequest)
	}
	return value, nil
}

func resourceID(id, prefix string) error {
	if !strings.HasPrefix(id, prefix) || len(id) <= len(prefix) || len(id) > 128 || !safeCode(id) {
		return payment.ErrPaymentReference
	}
	return nil
}

func validateReturnURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("%w: airwallex return URL must be HTTPS", payment.ErrInvalidRequest)
	}
	return nil
}

func (c *Client) check(ctx context.Context, provider payment.Provider, channel payment.Channel) error {
	if ctx == nil {
		return payment.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if provider != "" && provider != payment.ProviderAirwallex {
		return payment.ErrUnsupportedProvider
	}
	if channel != payment.ChannelAirwallexHPP {
		return payment.ErrUnsupportedChannel
	}
	return nil
}

func (c *Client) major(m payment.Money) (json.Number, error) {
	meta, ok := c.currencies[m.Currency]
	if !ok {
		return "", payment.ErrCurrencyMetaNotFound
	}
	if m.Amount <= 0 {
		return "", payment.ErrInvalidAmount
	}
	s := strconv.FormatInt(m.Amount, 10)
	if meta.MinorExp == 0 {
		return json.Number(s), nil
	}
	for len(s) <= meta.MinorExp {
		s = "0" + s
	}
	n := len(s) - meta.MinorExp
	return json.Number(s[:n] + "." + s[n:]), nil
}

func (c *Client) minor(value json.Number, currency string) (payment.Money, error) {
	meta, ok := c.currencies[currency]
	if !ok {
		return payment.Money{}, payment.ErrCurrencyMetaNotFound
	}
	s := value.String()
	if len(s) > 64 || !decimalPattern.MatchString(s) {
		return payment.Money{}, payment.ErrInvalidAmount
	}
	rat, ok := new(big.Rat).SetString(s)
	if !ok {
		return payment.Money{}, payment.ErrInvalidAmount
	}
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(meta.MinorExp)), nil)
	rat.Mul(rat, new(big.Rat).SetInt(factor))
	if !rat.IsInt() || !rat.Num().IsInt64() || rat.Sign() <= 0 {
		return payment.Money{}, payment.ErrInvalidAmount
	}
	return payment.Money{Amount: rat.Num().Int64(), Currency: currency}, nil
}

func paymentStatus(raw string) payment.PaymentStatus {
	switch raw {
	case "REQUIRES_PAYMENT_METHOD":
		return payment.PaymentPending
	case "REQUIRES_CUSTOMER_ACTION":
		return payment.PaymentRequiresAction
	case "REQUIRES_CAPTURE":
		return payment.PaymentAuthorized
	case "PENDING", "PENDING_REVIEW":
		return payment.PaymentProcessing
	case "SUCCEEDED":
		return payment.PaymentSucceeded
	case "CANCELLED":
		return payment.PaymentClosed
	default:
		return ""
	}
}

func refundStatus(raw string) payment.RefundStatus {
	switch raw {
	case "RECEIVED":
		return payment.RefundPending
	case "ACCEPTED", "SETTLED":
		return payment.RefundSucceeded
	case "FAILED":
		return payment.RefundFailed
	default:
		return ""
	}
}

func (c *Client) intentResponse(raw intent, key string) (*payment.QueryPaymentResponse, error) {
	if err := resourceID(raw.ID, "int_"); err != nil {
		return nil, err
	}
	amount, err := c.minor(raw.Amount, raw.Currency)
	if err != nil {
		return nil, err
	}
	return &payment.QueryPaymentResponse{
		Provider:        payment.ProviderAirwallex,
		Channel:         payment.ChannelAirwallexHPP,
		MerchantKey:     key,
		OutTradeNo:      raw.MerchantOrderID,
		ProviderTradeID: raw.ID,
		Status:          paymentStatus(raw.Status),
		Pricing:         payment.PaymentPricing{PayAmount: amount},
		Extra:           map[string]any{"provider_status": raw.Status, "account_id": c.accountID, "environment": string(c.environment)},
	}, nil
}

func (c *Client) refundResponse(raw refund, key string) (*payment.QueryRefundResponse, error) {
	if err := resourceID(raw.ID, "rfd_"); err != nil {
		return nil, err
	}
	if err := resourceID(raw.PaymentIntentID, "int_"); err != nil {
		return nil, err
	}
	amount, err := c.minor(raw.Amount, raw.Currency)
	if err != nil {
		return nil, err
	}
	return &payment.QueryRefundResponse{
		Provider:         payment.ProviderAirwallex,
		Channel:          payment.ChannelAirwallexHPP,
		MerchantKey:      key,
		ProviderTradeID:  raw.PaymentIntentID,
		ProviderRefundID: raw.ID,
		Status:           refundStatus(raw.Status),
		Amount:           payment.RefundAmount{Refund: amount},
		Extra:            map[string]any{"provider_status": raw.Status, "account_id": c.accountID, "environment": string(c.environment)},
	}, nil
}
