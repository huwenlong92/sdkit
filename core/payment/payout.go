package payment

import (
	"context"
	"fmt"
)

// PayoutProvider is optional; ordinary payment adapters do not need to implement it.
type PayoutProvider interface {
	CreatePayout(context.Context, CreatePayoutRequest) (*PayoutResponse, error)
	QueryPayout(context.Context, QueryPayoutRequest) (*PayoutResponse, error)
}

type PayoutStatus string

const (
	PayoutPending    PayoutStatus = "pending"
	PayoutProcessing PayoutStatus = "processing"
	PayoutSent       PayoutStatus = "sent"
	PayoutSucceeded  PayoutStatus = "succeeded"
	PayoutFailed     PayoutStatus = "failed"
	PayoutCancelled  PayoutStatus = "cancelled"
	PayoutReturned   PayoutStatus = "returned"
)

type CreatePayoutRequest struct {
	Provider      Provider       `json:"provider"`
	Channel       Channel        `json:"channel"`
	MerchantKey   string         `json:"merchant_key,omitempty"`
	PayoutID      string         `json:"payout_id"`
	RequestID     string         `json:"request_id"`
	BeneficiaryID string         `json:"beneficiary_id"`
	Amount        Money          `json:"amount"`
	Reference     string         `json:"reference,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}
type QueryPayoutRequest struct {
	Provider         Provider `json:"provider"`
	Channel          Channel  `json:"channel"`
	MerchantKey      string   `json:"merchant_key,omitempty"`
	PayoutID         string   `json:"payout_id,omitempty"`
	ProviderPayoutID string   `json:"provider_payout_id"`
}
type PayoutResponse struct {
	Provider         Provider         `json:"provider"`
	Channel          Channel          `json:"channel"`
	MerchantKey      string           `json:"merchant_key,omitempty"`
	PayoutID         string           `json:"payout_id,omitempty"`
	ProviderPayoutID string           `json:"provider_payout_id"`
	Status           PayoutStatus     `json:"status"`
	Amount           Money            `json:"amount"`
	ProviderStatus   string           `json:"provider_status,omitempty"`
	Exchange         ProviderExchange `json:"-"`
}

func (s *Service) CreatePayout(ctx context.Context, req CreatePayoutRequest) (*PayoutResponse, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.selectPayoutAccount(ctx, PaymentOperation("create_payout"), &req.Provider, &req.MerchantKey); err != nil {
		return nil, err
	}
	adapter, _, err := s.adapter(req.Provider)
	if err != nil {
		return nil, err
	}
	payout, ok := adapter.(PayoutProvider)
	if !ok {
		return nil, fmt.Errorf("%w: create payout", ErrUnsupportedCapability)
	}
	if req.Amount.Amount <= 0 {
		return nil, ErrInvalidAmount
	}
	req.Amount.Currency = NormalizeCurrency(req.Amount.Currency)
	if req.PayoutID == "" || req.RequestID == "" || req.BeneficiaryID == "" {
		return nil, ErrInvalidRequest
	}
	response, err := payout.CreatePayout(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, ErrInvalidRequest
	}
	return response, nil
}
func (s *Service) QueryPayout(ctx context.Context, req QueryPayoutRequest) (*PayoutResponse, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.selectPayoutAccount(ctx, PaymentOperation("query_payout"), &req.Provider, &req.MerchantKey); err != nil {
		return nil, err
	}
	adapter, _, err := s.adapter(req.Provider)
	if err != nil {
		return nil, err
	}
	payout, ok := adapter.(PayoutProvider)
	if !ok {
		return nil, fmt.Errorf("%w: query payout", ErrUnsupportedCapability)
	}
	if req.ProviderPayoutID == "" {
		return nil, ErrInvalidRequest
	}
	response, err := payout.QueryPayout(ctx, req)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, ErrInvalidRequest
	}
	return response, nil
}
func CreatePayout(ctx context.Context, req CreatePayoutRequest) (*PayoutResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.CreatePayout(ctx, req)
}
func QueryPayout(ctx context.Context, req QueryPayoutRequest) (*PayoutResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.QueryPayout(ctx, req)
}
