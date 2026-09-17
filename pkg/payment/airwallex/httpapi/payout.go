//go:build sdkit_payment_airwallex

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/huwenlong92/sdkit/core/payment"
)

type transfer struct {
	exchangeResult
	ID        string      `json:"id"`
	RequestID string      `json:"request_id"`
	Status    string      `json:"status"`
	Amount    json.Number `json:"transfer_amount"`
	Currency  string      `json:"transfer_currency"`
}

func (c *Client) CreatePayout(ctx context.Context, req payment.CreatePayoutRequest) (*payment.PayoutResponse, error) {
	if req.Channel != payment.ChannelAirwallexTransfer {
		return nil, payment.ErrUnsupportedChannel
	}
	if err := c.check(ctx, req.Provider, payment.ChannelAirwallexHPP); err != nil {
		return nil, err
	}
	id, err := requestID(map[string]any{ExtraRequestIDKey: req.RequestID})
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(req.BeneficiaryID); err != nil {
		return nil, payment.ErrInvalidRequest
	}
	amount, err := c.major(req.Amount)
	if err != nil {
		return nil, err
	}
	method, _ := req.Extra["transfer_method"].(string)
	if method != "LOCAL" && method != "SWIFT" {
		return nil, payment.ErrInvalidRequest
	}
	if req.Reason == "" {
		return nil, payment.ErrInvalidRequest
	}
	// This operation funds in the payment currency; FX is never requested implicitly.
	payload := struct {
		RequestID      string      `json:"request_id"`
		BeneficiaryID  string      `json:"beneficiary_id"`
		Amount         json.Number `json:"transfer_amount"`
		Currency       string      `json:"transfer_currency"`
		SourceCurrency string      `json:"source_currency"`
		Method         string      `json:"transfer_method"`
		Reference      string      `json:"reference,omitempty"`
		Reason         string      `json:"reason"`
	}{id, req.BeneficiaryID, amount, req.Amount.Currency, req.Amount.Currency, method, req.Reference, req.Reason}
	requestBody, _ := json.Marshal(payload)
	var raw transfer
	if err := c.call(ctx, http.MethodPost, "/api/v1/transfers/create", payload, &raw); err != nil {
		return nil, err
	}
	response, err := c.payoutResponse(raw, req.MerchantKey, req.PayoutID)
	if err != nil {
		return nil, err
	}
	if raw.RequestID != id || response.Amount != req.Amount {
		return nil, payment.ErrInvalidRequest
	}
	response.Exchange = providerExchange(requestBody, raw.exchangeResult)
	return response, nil
}
func (c *Client) QueryPayout(ctx context.Context, req payment.QueryPayoutRequest) (*payment.PayoutResponse, error) {
	if req.Channel != payment.ChannelAirwallexTransfer {
		return nil, payment.ErrUnsupportedChannel
	}
	if err := c.check(ctx, req.Provider, payment.ChannelAirwallexHPP); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.ProviderPayoutID)
	if err != nil || id.String() != req.ProviderPayoutID {
		return nil, payment.ErrInvalidRequest
	}
	var raw transfer
	if err := c.call(ctx, http.MethodGet, "/api/v1/transfers/"+req.ProviderPayoutID, nil, &raw); err != nil {
		return nil, err
	}
	if raw.ID != req.ProviderPayoutID {
		return nil, payment.ErrPaymentReference
	}
	response, err := c.payoutResponse(raw, req.MerchantKey, req.PayoutID)
	if err == nil {
		response.Exchange = providerExchange(nil, raw.exchangeResult)
	}
	return response, err
}
func (c *Client) payoutResponse(raw transfer, key, payoutID string) (*payment.PayoutResponse, error) {
	if _, err := uuid.Parse(raw.ID); err != nil {
		return nil, payment.ErrPaymentReference
	}
	amount, err := c.minor(raw.Amount, raw.Currency)
	if err != nil {
		return nil, err
	}
	var status payment.PayoutStatus
	switch raw.Status {
	case "CREATED", "PENDING_APPROVAL", "IN_APPROVAL", "APPROVAL_RECALLED", "APPROVAL_REJECTED", "APPROVAL_BLOCKED", "SCHEDULED":
		status = payment.PayoutPending
	case "PROCESSING", "OVERDUE", "CANCELLATION_REQUESTED":
		status = payment.PayoutProcessing
	case "SENT":
		status = payment.PayoutSent
	case "PAID":
		status = payment.PayoutSucceeded
	case "FAILED":
		status = payment.PayoutFailed
	case "CANCELLED":
		status = payment.PayoutCancelled
	case "RETURNED":
		status = payment.PayoutReturned
	default:
		return nil, payment.ErrInvalidStateTransition
	}
	return &payment.PayoutResponse{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, MerchantKey: key, PayoutID: payoutID, ProviderPayoutID: raw.ID, Status: status, Amount: amount, ProviderStatus: raw.Status}, nil
}
