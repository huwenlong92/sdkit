//go:build sdkit_payment_airwallex

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/huwenlong92/sdkit/core/payment"
)

func (c *Client) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if err := c.check(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	// Airwallex delivers notifications to registered Webhook subscriptions;
	// PaymentIntents cannot override their destination for a single payment.
	if req.NotifyURL != "" && req.NotifyURL != c.notifyURL {
		return nil, fmt.Errorf("%w: airwallex per-payment notify URL override", payment.ErrUnsupportedCapability)
	}
	id, err := requestID(req.Extra)
	if err != nil {
		return nil, err
	}
	if req.ExpireAt != nil {
		return nil, fmt.Errorf("%w: airwallex HPP expiry is not supported", payment.ErrUnsupportedCapability)
	}
	order := req.OutTradeNo
	if order == "" {
		order = req.PaymentID
	}
	if order == "" || utf8.RuneCountInString(order) > 64 {
		return nil, payment.ErrPaymentReference
	}
	amount, err := c.major(req.Pricing.PayAmount)
	if err != nil {
		return nil, err
	}
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = c.returnURL
	}
	if err := validateReturnURL(returnURL); err != nil {
		return nil, err
	}
	payload := struct {
		RequestID       string      `json:"request_id"`
		Amount          json.Number `json:"amount"`
		Currency        string      `json:"currency"`
		MerchantOrderID string      `json:"merchant_order_id"`
		ReturnURL       string      `json:"return_url"`
	}{id, amount, req.Pricing.PayAmount.Currency, order, returnURL}
	var raw intent
	resumed := req.ProviderTradeID != ""
	if req.ProviderTradeID != "" {
		if err := resourceID(req.ProviderTradeID, "int_"); err != nil {
			return nil, err
		}
		if err := c.call(ctx, http.MethodGet, "/api/v1/pa/payment_intents/"+req.ProviderTradeID, nil, &raw); err != nil {
			return nil, err
		}
		if raw.ID != req.ProviderTradeID {
			return nil, payment.ErrInvalidRequest
		}
	} else if err := c.call(ctx, http.MethodPost, "/api/v1/pa/payment_intents/create", payload, &raw); err != nil {
		return nil, err
	}
	query, err := c.intentResponse(raw, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if (!resumed && raw.RequestID != id) || raw.MerchantOrderID != order || query.Pricing.PayAmount != req.Pricing.PayAmount {
		return nil, fmt.Errorf("%w: airwallex create response mismatch", payment.ErrInvalidRequest)
	}
	action := payment.PaymentAction{Type: payment.ActionNone}
	switch query.Status {
	case payment.PaymentPending, payment.PaymentRequiresAction:
		if raw.ClientSecret == "" {
			return nil, payment.ErrPaymentActionRequired
		}
		env := "sandbox"
		if c.environment == Production {
			env = "prod"
		}
		action = payment.PaymentAction{Type: payment.ActionSDKParams, Params: map[string]any{
			"env":           env,
			"mode":          "payment",
			"intent_id":     raw.ID,
			"client_secret": raw.ClientSecret,
			"currency":      raw.Currency,
			"successUrl":    returnURL,
		}}
	}
	return &payment.CreatePaymentResponse{
		Provider:           payment.ProviderAirwallex,
		Channel:            payment.ChannelAirwallexHPP,
		MerchantKey:        req.MerchantKey,
		PaymentID:          req.PaymentID,
		OrderID:            req.OrderID,
		OutTradeNo:         order,
		ProviderTradeID:    raw.ID,
		Status:             query.Status,
		Pricing:            req.Pricing,
		Action:             action,
		Extra:              query.Extra,
		ProviderSettlement: query.ProviderSettlement,
	}, nil
}

func (c *Client) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if err := c.check(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if err := resourceID(req.ProviderTradeID, "int_"); err != nil {
		return nil, err
	}
	var raw intent
	if err := c.call(ctx, http.MethodGet, "/api/v1/pa/payment_intents/"+req.ProviderTradeID, nil, &raw); err != nil {
		return nil, err
	}
	if raw.ID != req.ProviderTradeID || (req.OutTradeNo != "" && raw.MerchantOrderID != req.OutTradeNo) {
		return nil, fmt.Errorf("%w: airwallex intent mismatch", payment.ErrInvalidRequest)
	}
	result, err := c.intentResponse(raw, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	result.PaymentID = req.PaymentID
	result.OrderID = req.OrderID
	return result, nil
}

func (c *Client) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	if err := c.check(ctx, req.Provider, req.Channel); err != nil {
		return err
	}
	if err := resourceID(req.ProviderTradeID, "int_"); err != nil {
		return err
	}
	id, err := requestID(req.Extra)
	if err != nil {
		return err
	}
	var raw intent
	if err := c.call(ctx, http.MethodPost, "/api/v1/pa/payment_intents/"+req.ProviderTradeID+"/cancel", struct {
		RequestID string `json:"request_id"`
	}{id}, &raw); err != nil {
		return err
	}
	if raw.ID != req.ProviderTradeID || raw.Status != "CANCELLED" {
		return fmt.Errorf("%w: airwallex cancellation not confirmed", payment.ErrInvalidStateTransition)
	}
	return nil
}

func (c *Client) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if err := c.check(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if err := resourceID(req.ProviderTradeID, "int_"); err != nil {
		return nil, err
	}
	id, err := requestID(req.Extra)
	if err != nil {
		return nil, err
	}
	amount, err := c.major(req.Amount.Refund)
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(req.Reason) > 128 {
		return nil, payment.ErrInvalidRequest
	}
	// Refund API has no currency input; verify it against the original intent
	// before sending a mutation so that minor units cannot change its meaning.
	original, err := c.QueryPayment(ctx, payment.QueryPaymentRequest{Provider: req.Provider, Channel: req.Channel, MerchantKey: req.MerchantKey, ProviderTradeID: req.ProviderTradeID, OutTradeNo: req.OutTradeNo})
	if err != nil {
		return nil, err
	}
	if original.Pricing.PayAmount.Currency != req.Amount.Refund.Currency {
		return nil, payment.ErrInvalidCurrency
	}
	if original.Status != payment.PaymentSucceeded {
		return nil, payment.ErrInvalidStateTransition
	}
	if req.Amount.Refund.Amount > original.Pricing.PayAmount.Amount {
		return nil, payment.ErrInvalidAmount
	}
	payload := struct {
		RequestID       string      `json:"request_id"`
		PaymentIntentID string      `json:"payment_intent_id"`
		Amount          json.Number `json:"amount"`
		Reason          string      `json:"reason,omitempty"`
	}{id, req.ProviderTradeID, amount, req.Reason}
	var raw refund
	if err := c.call(ctx, http.MethodPost, "/api/v1/pa/refunds/create", payload, &raw); err != nil {
		return nil, err
	}
	result, err := c.refundResponse(raw, req.MerchantKey)
	if err != nil {
		return nil, payment.WithProviderExchange(err, providerExchange(requestBodyForRefund(payload), raw.exchangeResult))
	}
	if raw.RequestID != id || raw.PaymentIntentID != req.ProviderTradeID || result.Amount.Refund != req.Amount.Refund {
		return nil, payment.WithProviderExchange(fmt.Errorf("%w: airwallex refund mismatch", payment.ErrInvalidRequest), providerExchange(requestBodyForRefund(payload), raw.exchangeResult))
	}
	return &payment.RefundResponse{
		Provider:         result.Provider,
		Channel:          result.Channel,
		MerchantKey:      req.MerchantKey,
		PaymentID:        req.PaymentID,
		OrderID:          req.OrderID,
		OutTradeNo:       req.OutTradeNo,
		ProviderTradeID:  raw.PaymentIntentID,
		RefundID:         req.RefundID,
		OutRefundNo:      req.OutRefundNo,
		ProviderRefundID: raw.ID,
		Status:           result.Status,
		Amount:           result.Amount,
		Exchange:         providerExchange(requestBodyForRefund(payload), raw.exchangeResult),
		Extra:            result.Extra,
	}, nil
}

func requestBodyForRefund(payload any) []byte {
	body, _ := json.Marshal(payload)
	return body
}

func (c *Client) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	if err := c.check(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if err := resourceID(req.ProviderRefundID, "rfd_"); err != nil {
		return nil, err
	}
	var raw refund
	if err := c.call(ctx, http.MethodGet, "/api/v1/pa/refunds/"+req.ProviderRefundID, nil, &raw); err != nil {
		return nil, err
	}
	if raw.ID != req.ProviderRefundID || (req.ProviderTradeID != "" && raw.PaymentIntentID != req.ProviderTradeID) {
		return nil, payment.WithProviderExchange(fmt.Errorf("%w: airwallex refund reference mismatch", payment.ErrInvalidRequest), providerExchange(nil, raw.exchangeResult))
	}
	result, err := c.refundResponse(raw, req.MerchantKey)
	if err != nil {
		return nil, payment.WithProviderExchange(err, providerExchange(nil, raw.exchangeResult))
	}
	result.PaymentID = req.PaymentID
	result.OutTradeNo = req.OutTradeNo
	result.RefundID = req.RefundID
	result.OutRefundNo = req.OutRefundNo
	return result, nil
}
