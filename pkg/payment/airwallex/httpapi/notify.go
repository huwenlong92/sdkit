//go:build sdkit_payment_airwallex

package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/payment"
)

func (c *Client) ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
	if err := c.check(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if req.Method != http.MethodPost || len(req.Body) == 0 || len(req.Body) > 1<<20 || c.webhookSecret == "" {
		return nil, payment.ErrNotifyVerificationFail
	}
	timestamp, ok := oneHeader(req.Header, "x-timestamp")
	if !ok {
		return nil, payment.ErrNotifyVerificationFail
	}
	signature, ok := oneHeader(req.Header, "x-signature")
	if !ok {
		return nil, payment.ErrNotifyVerificationFail
	}
	millis, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || millis <= 0 || strconv.FormatInt(millis, 10) != timestamp {
		return nil, payment.ErrNotifyVerificationFail
	}
	now := c.clock()
	sent := time.UnixMilli(millis)
	if sent.Before(now.Add(-c.tolerance)) || sent.After(now.Add(c.tolerance)) {
		return nil, payment.ErrNotifyVerificationFail
	}
	expected, err := hex.DecodeString(signature)
	if err != nil || len(expected) != sha256.Size {
		return nil, payment.ErrNotifyVerificationFail
	}
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write(req.Body)
	if !hmac.Equal(expected, mac.Sum(nil)) {
		return nil, payment.ErrNotifyVerificationFail
	}
	var envelope struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		AccountID string `json:"account_id"`
		Version   string `json:"version"`
		Data      struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(req.Body, &envelope) != nil || envelope.ID == "" || envelope.Name == "" || envelope.AccountID != c.accountID {
		return nil, payment.ErrNotifyVerificationFail
	}
	event := &payment.PaymentEvent{Type: payment.EventUnknown}
	switch {
	case strings.HasPrefix(envelope.Name, "payment_intent."):
		var raw intent
		if json.Unmarshal(envelope.Data.Object, &raw) != nil {
			return nil, payment.ErrNotifyVerificationFail
		}
		response, err := c.intentResponse(raw, "")
		if err != nil {
			return nil, err
		}
		event = payment.EventFromQueryPaymentResponse(response)
		if event == nil {
			event = &payment.PaymentEvent{Type: payment.EventUnknown, ProviderTradeID: raw.ID, OutTradeNo: raw.MerchantOrderID, Amount: response.Pricing.PayAmount}
		}
		// Require both event name and object state for any actionable event.
		allowed := map[string]string{
			"payment_intent.created":                  "REQUIRES_PAYMENT_METHOD",
			"payment_intent.requires_payment_method":  "REQUIRES_PAYMENT_METHOD",
			"payment_intent.requires_customer_action": "REQUIRES_CUSTOMER_ACTION",
			"payment_intent.requires_capture":         "REQUIRES_CAPTURE",
			"payment_intent.pending":                  "PENDING",
			"payment_intent.pending_review":           "PENDING_REVIEW",
			"payment_intent.succeeded":                "SUCCEEDED",
			"payment_intent.cancelled":                "CANCELLED",
		}
		if expected, ok := allowed[envelope.Name]; ok {
			if expected != raw.Status {
				return nil, payment.ErrNotifyVerificationFail
			}
			if envelope.Name == "payment_intent.created" {
				event.Type = payment.EventPaymentCreated
			}
		} else {
			event.Type = payment.EventUnknown
			event.Status = ""
		}
		event.Extra = response.Extra
	case strings.HasPrefix(envelope.Name, "refund."):
		var raw refund
		if json.Unmarshal(envelope.Data.Object, &raw) != nil {
			return nil, payment.ErrNotifyVerificationFail
		}
		response, err := c.refundResponse(raw, "")
		if err != nil {
			return nil, err
		}
		event = payment.EventFromQueryRefundResponse(response)
		if event == nil {
			event = &payment.PaymentEvent{Type: payment.EventUnknown, ProviderRefundID: raw.ID, Amount: response.Amount.Refund}
		}
		event.ProviderTradeID = raw.PaymentIntentID
		allowed := map[string]string{"refund.received": "RECEIVED", "refund.accepted": "ACCEPTED", "refund.settled": "SETTLED", "refund.failed": "FAILED"}
		if expected, ok := allowed[envelope.Name]; ok {
			if expected != raw.Status {
				return nil, payment.ErrNotifyVerificationFail
			}
		} else {
			event.Type = payment.EventUnknown
			event.RefundStatus = ""
		}
		event.Extra = response.Extra
	}
	event.EventID = envelope.ID
	event.Provider = payment.ProviderAirwallex
	event.Channel = payment.ChannelAirwallexHPP
	if event.Extra == nil {
		event.Extra = map[string]any{}
	}
	event.Extra["provider_event"] = envelope.Name
	event.Extra["api_version"] = envelope.Version
	event.Extra["account_id"] = c.accountID
	event.Extra["environment"] = string(c.environment)
	// Do not copy the raw callback (which can contain client_secret or PII).
	// Ack is a suggestion; the receiver must durably persist/deduplicate first.
	return &payment.NotifyResult{Verified: true, Event: event, Ack: payment.NotifyAck{StatusCode: 200, ContentType: "text/plain", Body: []byte("ok")}}, nil
}

func oneHeader(headers map[string][]string, name string) (string, bool) {
	var value string
	count := 0
	for key, values := range headers {
		if strings.EqualFold(key, name) {
			for _, v := range values {
				value = v
				count++
			}
		}
	}
	return value, count == 1 && value != ""
}
