//go:build sdkit_payment_airwallex

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/huwenlong92/sdkit/core/payment"
)

type beneficiaryResult struct {
	exchangeResult
	ID string `json:"id"`
}

type batchTransferResult struct {
	exchangeResult
	ID             string `json:"id"`
	Status         string `json:"status"`
	TotalItemCount int    `json:"total_item_count"`
	ValidItemCount int    `json:"valid_item_count"`
}

type batchTransferItemsResult struct {
	exchangeResult
	Items []struct {
		ID         string `json:"id"`
		RequestID  string `json:"request_id"`
		Status     string `json:"status"`
		TransferID string `json:"transfer_id"`
		Errors     []struct {
			Code   json.RawMessage `json:"code"`
			Source string          `json:"source"`
		} `json:"errors"`
	} `json:"items"`
}

type batchTransferListResult struct {
	exchangeResult
	Items []batchTransferResult `json:"items"`
}

func providerExchange(requestBody []byte, result exchangeResult) payment.ProviderExchange {
	headers := make(map[string][]string)
	for _, key := range []string{"Content-Type", "Request-Id", "X-Request-Id"} {
		if values := result.Headers.Values(key); len(values) > 0 {
			headers[key] = append([]string(nil), values...)
		}
	}
	return payment.ProviderExchange{
		RequestBody: append([]byte(nil), requestBody...), ResponseBody: append([]byte(nil), result.RawBody...),
		StatusCode: result.StatusCode, Headers: headers,
	}
}

func (c *Client) validatePayoutOperation(ctx context.Context, provider payment.Provider, channel payment.Channel) error {
	if channel != payment.ChannelAirwallexTransfer {
		return payment.ErrUnsupportedChannel
	}
	return c.check(ctx, provider, payment.ChannelAirwallexHPP)
}

func (c *Client) CreateBeneficiary(ctx context.Context, req payment.CreateBeneficiaryRequest) (*payment.BeneficiaryResponse, error) {
	if err := c.validatePayoutOperation(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	requestBody, err := json.Marshal(req.Details)
	if err != nil || len(req.Details) == 0 {
		return nil, payment.ErrInvalidRequest
	}
	var raw beneficiaryResult
	if err := c.call(ctx, http.MethodPost, "/api/v1/beneficiaries/create", req.Details, &raw); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(raw.ID); err != nil {
		return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(requestBody, raw.exchangeResult))
	}
	return &payment.BeneficiaryResponse{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, MerchantKey: req.MerchantKey, BeneficiaryID: raw.ID, Exchange: providerExchange(requestBody, raw.exchangeResult)}, nil
}

func (c *Client) QueryBeneficiary(ctx context.Context, req payment.QueryBeneficiaryRequest) (*payment.BeneficiaryResponse, error) {
	if err := c.validatePayoutOperation(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(req.BeneficiaryID); err != nil {
		return nil, payment.ErrInvalidRequest
	}
	var raw beneficiaryResult
	if err := c.call(ctx, http.MethodGet, "/api/v1/beneficiaries/"+url.PathEscape(req.BeneficiaryID), nil, &raw); err != nil {
		return nil, err
	}
	if raw.ID != req.BeneficiaryID {
		return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(nil, raw.exchangeResult))
	}
	return &payment.BeneficiaryResponse{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, MerchantKey: req.MerchantKey, BeneficiaryID: raw.ID, Exchange: providerExchange(nil, raw.exchangeResult)}, nil
}

func (c *Client) CreatePayoutBatch(ctx context.Context, req payment.CreatePayoutBatchRequest) (*payment.PayoutBatchResponse, error) {
	if err := c.validatePayoutOperation(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	body := map[string]any{"request_id": req.RequestID}
	if req.Name != "" {
		body["name"] = req.Name
	}
	if req.Remarks != "" {
		body["remarks"] = req.Remarks
	}
	requestBody, _ := json.Marshal(body)
	var raw batchTransferResult
	if err := c.call(ctx, http.MethodPost, "/api/v1/batch_transfers/create", body, &raw); err != nil {
		return nil, err
	}
	return c.batchResponse(req.MerchantKey, requestBody, raw)
}

func (c *Client) AddPayoutBatchItems(ctx context.Context, req payment.AddPayoutBatchItemsRequest) (*payment.PayoutBatchResponse, error) {
	if err := c.validatePayoutOperation(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(req.ProviderBatchID); err != nil {
		return nil, payment.ErrInvalidRequest
	}
	items := make([]map[string]any, 0, len(req.Items))
	for _, item := range req.Items {
		amount, err := c.major(item.Amount)
		if err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"request_id": item.RequestID, "beneficiary_id": item.BeneficiaryID, "source_currency": item.Amount.Currency, "transfer_currency": item.Amount.Currency, "transfer_amount": amount, "transfer_method": item.TransferMethod, "fee_paid_by": "PAYER", "reference": item.Reference, "reason": item.Reason})
	}
	body := map[string]any{"items": items}
	requestBody, _ := json.Marshal(body)
	var raw batchTransferResult
	if err := c.call(ctx, http.MethodPost, "/api/v1/batch_transfers/"+url.PathEscape(req.ProviderBatchID)+"/add_items", body, &raw); err != nil {
		return nil, err
	}
	if raw.ID != req.ProviderBatchID {
		return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(requestBody, raw.exchangeResult))
	}
	return c.batchResponse(req.MerchantKey, requestBody, raw)
}

func (c *Client) SubmitPayoutBatch(ctx context.Context, req payment.SubmitPayoutBatchRequest) (*payment.PayoutBatchResponse, error) {
	return c.batchMutation(ctx, req.Provider, req.Channel, req.MerchantKey, req.ProviderBatchID, "/submit")
}

func (c *Client) QueryPayoutBatch(ctx context.Context, req payment.QueryPayoutBatchRequest) (*payment.PayoutBatchResponse, error) {
	if err := c.validatePayoutOperation(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if req.ProviderBatchID == "" {
		if strings.TrimSpace(req.RequestID) == "" {
			return nil, payment.ErrInvalidRequest
		}
		var listed batchTransferListResult
		if err := c.call(ctx, http.MethodGet, "/api/v1/batch_transfers?request_id="+url.QueryEscape(req.RequestID), nil, &listed); err != nil {
			return nil, err
		}
		if len(listed.Items) != 1 || listed.Items[0].ID == "" {
			return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(nil, listed.exchangeResult))
		}
		listed.Items[0].exchangeResult = listed.exchangeResult
		return c.batchResponse(req.MerchantKey, nil, listed.Items[0])
	}
	if _, err := uuid.Parse(req.ProviderBatchID); err != nil {
		return nil, payment.ErrInvalidRequest
	}
	var raw batchTransferResult
	if err := c.call(ctx, http.MethodGet, "/api/v1/batch_transfers/"+url.PathEscape(req.ProviderBatchID), nil, &raw); err != nil {
		return nil, err
	}
	if raw.ID != req.ProviderBatchID {
		return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(nil, raw.exchangeResult))
	}
	return c.batchResponse(req.MerchantKey, nil, raw)
}

func (c *Client) batchMutation(ctx context.Context, provider payment.Provider, channel payment.Channel, merchantKey, id, suffix string) (*payment.PayoutBatchResponse, error) {
	if err := c.validatePayoutOperation(ctx, provider, channel); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, payment.ErrInvalidRequest
	}
	var raw batchTransferResult
	if err := c.call(ctx, http.MethodPost, "/api/v1/batch_transfers/"+url.PathEscape(id)+suffix, nil, &raw); err != nil {
		return nil, err
	}
	if raw.ID != id {
		return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(nil, raw.exchangeResult))
	}
	return c.batchResponse(merchantKey, nil, raw)
}

func (c *Client) batchResponse(merchantKey string, requestBody []byte, raw batchTransferResult) (*payment.PayoutBatchResponse, error) {
	if _, err := uuid.Parse(raw.ID); err != nil || strings.TrimSpace(raw.Status) == "" {
		return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(requestBody, raw.exchangeResult))
	}
	return &payment.PayoutBatchResponse{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, MerchantKey: merchantKey, ProviderBatchID: raw.ID, ProviderStatus: raw.Status, TotalItemCount: raw.TotalItemCount, ValidItemCount: raw.ValidItemCount, Exchange: providerExchange(requestBody, raw.exchangeResult)}, nil
}

func (c *Client) ListPayoutBatchItems(ctx context.Context, req payment.ListPayoutBatchItemsRequest) (*payment.PayoutBatchItemsResponse, error) {
	if err := c.validatePayoutOperation(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(req.ProviderBatchID); err != nil {
		return nil, payment.ErrInvalidRequest
	}
	var raw batchTransferItemsResult
	if err := c.call(ctx, http.MethodGet, "/api/v1/batch_transfers/"+url.PathEscape(req.ProviderBatchID)+"/items?page_size=1000", nil, &raw); err != nil {
		return nil, err
	}
	result := &payment.PayoutBatchItemsResponse{Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer, MerchantKey: req.MerchantKey, ProviderBatchID: req.ProviderBatchID, Items: make([]payment.PayoutBatchItemResponse, 0, len(raw.Items)), Exchange: providerExchange(nil, raw.exchangeResult)}
	for _, item := range raw.Items {
		failureCode, failureMessage := "", ""
		if len(item.Errors) > 0 {
			failureCode = strings.Trim(string(item.Errors[0].Code), `"`)
			failureMessage = item.Errors[0].Source
		}
		result.Items = append(result.Items, payment.PayoutBatchItemResponse{ProviderItemID: item.ID, ProviderPayoutID: item.TransferID, RequestID: item.RequestID, ProviderStatus: item.Status, FailureCode: failureCode, FailureMessage: failureMessage})
	}
	return result, nil
}
