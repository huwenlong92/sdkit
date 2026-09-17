package payment

import (
	"context"
	"fmt"
)

type BeneficiaryProvider interface {
	CreateBeneficiary(context.Context, CreateBeneficiaryRequest) (*BeneficiaryResponse, error)
	QueryBeneficiary(context.Context, QueryBeneficiaryRequest) (*BeneficiaryResponse, error)
}

type CreateBeneficiaryRequest struct {
	Provider    Provider       `json:"provider"`
	Channel     Channel        `json:"channel"`
	MerchantKey string         `json:"merchant_key,omitempty"`
	Details     map[string]any `json:"details"`
}

type QueryBeneficiaryRequest struct {
	Provider      Provider `json:"provider"`
	Channel       Channel  `json:"channel"`
	MerchantKey   string   `json:"merchant_key,omitempty"`
	BeneficiaryID string   `json:"beneficiary_id"`
}

type BeneficiaryResponse struct {
	Provider      Provider         `json:"provider"`
	Channel       Channel          `json:"channel"`
	MerchantKey   string           `json:"merchant_key,omitempty"`
	BeneficiaryID string           `json:"beneficiary_id"`
	Exchange      ProviderExchange `json:"-"`
}

type BatchPayoutProvider interface {
	CreatePayoutBatch(context.Context, CreatePayoutBatchRequest) (*PayoutBatchResponse, error)
	AddPayoutBatchItems(context.Context, AddPayoutBatchItemsRequest) (*PayoutBatchResponse, error)
	SubmitPayoutBatch(context.Context, SubmitPayoutBatchRequest) (*PayoutBatchResponse, error)
	QueryPayoutBatch(context.Context, QueryPayoutBatchRequest) (*PayoutBatchResponse, error)
	ListPayoutBatchItems(context.Context, ListPayoutBatchItemsRequest) (*PayoutBatchItemsResponse, error)
}

type CreatePayoutBatchRequest struct {
	Provider    Provider `json:"provider"`
	Channel     Channel  `json:"channel"`
	MerchantKey string   `json:"merchant_key,omitempty"`
	RequestID   string   `json:"request_id"`
	Name        string   `json:"name,omitempty"`
	Remarks     string   `json:"remarks,omitempty"`
}

type PayoutBatchItemRequest struct {
	RequestID      string `json:"request_id"`
	BeneficiaryID  string `json:"beneficiary_id"`
	Amount         Money  `json:"amount"`
	TransferMethod string `json:"transfer_method"`
	Reference      string `json:"reference"`
	Reason         string `json:"reason"`
}

type AddPayoutBatchItemsRequest struct {
	Provider        Provider                 `json:"provider"`
	Channel         Channel                  `json:"channel"`
	MerchantKey     string                   `json:"merchant_key,omitempty"`
	ProviderBatchID string                   `json:"provider_batch_id"`
	Items           []PayoutBatchItemRequest `json:"items"`
}

type SubmitPayoutBatchRequest struct {
	Provider        Provider `json:"provider"`
	Channel         Channel  `json:"channel"`
	MerchantKey     string   `json:"merchant_key,omitempty"`
	ProviderBatchID string   `json:"provider_batch_id"`
}

type QueryPayoutBatchRequest struct {
	Provider        Provider `json:"provider"`
	Channel         Channel  `json:"channel"`
	MerchantKey     string   `json:"merchant_key,omitempty"`
	ProviderBatchID string   `json:"provider_batch_id,omitempty"`
	RequestID       string   `json:"request_id,omitempty"`
}
type ListPayoutBatchItemsRequest = SubmitPayoutBatchRequest

type PayoutBatchResponse struct {
	Provider        Provider         `json:"provider"`
	Channel         Channel          `json:"channel"`
	MerchantKey     string           `json:"merchant_key,omitempty"`
	ProviderBatchID string           `json:"provider_batch_id"`
	ProviderStatus  string           `json:"provider_status"`
	TotalItemCount  int              `json:"total_item_count"`
	ValidItemCount  int              `json:"valid_item_count"`
	Exchange        ProviderExchange `json:"-"`
}

type PayoutBatchItemResponse struct {
	ProviderItemID   string `json:"provider_item_id"`
	ProviderPayoutID string `json:"provider_payout_id,omitempty"`
	RequestID        string `json:"request_id"`
	ProviderStatus   string `json:"provider_status"`
	FailureCode      string `json:"failure_code,omitempty"`
	FailureMessage   string `json:"failure_message,omitempty"`
}

type PayoutBatchItemsResponse struct {
	Provider        Provider                  `json:"provider"`
	Channel         Channel                   `json:"channel"`
	MerchantKey     string                    `json:"merchant_key,omitempty"`
	ProviderBatchID string                    `json:"provider_batch_id"`
	Items           []PayoutBatchItemResponse `json:"items"`
	Exchange        ProviderExchange          `json:"-"`
}

func (s *Service) CreateBeneficiary(ctx context.Context, req CreateBeneficiaryRequest) (*BeneficiaryResponse, error) {
	provider, err := s.beneficiaryProvider(ctx, "create_beneficiary", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if len(req.Details) == 0 {
		return nil, ErrInvalidRequest
	}
	return provider.CreateBeneficiary(ctx, req)
}

func (s *Service) QueryBeneficiary(ctx context.Context, req QueryBeneficiaryRequest) (*BeneficiaryResponse, error) {
	provider, err := s.beneficiaryProvider(ctx, "query_beneficiary", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if req.BeneficiaryID == "" {
		return nil, ErrInvalidRequest
	}
	return provider.QueryBeneficiary(ctx, req)
}

func (s *Service) beneficiaryProvider(ctx context.Context, operation PaymentOperation, provider *Provider, channel *Channel, merchantKey *string) (BeneficiaryProvider, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if channel == nil || *channel == "" {
		return nil, ErrUnsupportedChannel
	}
	if err := s.selectPayoutAccount(ctx, operation, provider, merchantKey); err != nil {
		return nil, err
	}
	adapter, _, err := s.adapter(*provider)
	if err != nil {
		return nil, err
	}
	value, ok := adapter.(BeneficiaryProvider)
	if !ok {
		return nil, fmt.Errorf("%w: beneficiary", ErrUnsupportedCapability)
	}
	return value, nil
}

func (s *Service) batchPayoutProvider(ctx context.Context, operation PaymentOperation, provider *Provider, channel *Channel, merchantKey *string) (BatchPayoutProvider, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if channel == nil || *channel == "" {
		return nil, ErrUnsupportedChannel
	}
	if err := s.selectPayoutAccount(ctx, operation, provider, merchantKey); err != nil {
		return nil, err
	}
	adapter, _, err := s.adapter(*provider)
	if err != nil {
		return nil, err
	}
	value, ok := adapter.(BatchPayoutProvider)
	if !ok {
		return nil, fmt.Errorf("%w: batch payout", ErrUnsupportedCapability)
	}
	return value, nil
}

func (s *Service) selectPayoutAccount(ctx context.Context, operation PaymentOperation, provider *Provider, merchantKey *string) error {
	return s.selectChannel(ctx, operation, provider, nil, merchantKey)
}

func (s *Service) CreatePayoutBatch(ctx context.Context, req CreatePayoutBatchRequest) (*PayoutBatchResponse, error) {
	provider, err := s.batchPayoutProvider(ctx, "create_payout_batch", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if req.RequestID == "" {
		return nil, ErrInvalidRequest
	}
	return provider.CreatePayoutBatch(ctx, req)
}

func (s *Service) AddPayoutBatchItems(ctx context.Context, req AddPayoutBatchItemsRequest) (*PayoutBatchResponse, error) {
	provider, err := s.batchPayoutProvider(ctx, "add_payout_batch_items", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if req.ProviderBatchID == "" || len(req.Items) == 0 || len(req.Items) > 100 {
		return nil, ErrInvalidRequest
	}
	for _, item := range req.Items {
		if item.RequestID == "" || item.BeneficiaryID == "" || item.Amount.Amount <= 0 || item.Reference == "" || item.Reason == "" || (item.TransferMethod != "LOCAL" && item.TransferMethod != "SWIFT") {
			return nil, ErrInvalidRequest
		}
	}
	return provider.AddPayoutBatchItems(ctx, req)
}

func (s *Service) SubmitPayoutBatch(ctx context.Context, req SubmitPayoutBatchRequest) (*PayoutBatchResponse, error) {
	provider, err := s.batchPayoutProvider(ctx, "submit_payout_batch", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if req.ProviderBatchID == "" {
		return nil, ErrInvalidRequest
	}
	return provider.SubmitPayoutBatch(ctx, req)
}

func (s *Service) QueryPayoutBatch(ctx context.Context, req QueryPayoutBatchRequest) (*PayoutBatchResponse, error) {
	provider, err := s.batchPayoutProvider(ctx, "query_payout_batch", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if (req.ProviderBatchID == "") == (req.RequestID == "") {
		return nil, ErrInvalidRequest
	}
	return provider.QueryPayoutBatch(ctx, req)
}

func (s *Service) ListPayoutBatchItems(ctx context.Context, req ListPayoutBatchItemsRequest) (*PayoutBatchItemsResponse, error) {
	provider, err := s.batchPayoutProvider(ctx, "list_payout_batch_items", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if req.ProviderBatchID == "" {
		return nil, ErrInvalidRequest
	}
	return provider.ListPayoutBatchItems(ctx, req)
}

func CreateBeneficiary(ctx context.Context, req CreateBeneficiaryRequest) (*BeneficiaryResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.CreateBeneficiary(ctx, req)
}
func QueryBeneficiary(ctx context.Context, req QueryBeneficiaryRequest) (*BeneficiaryResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.QueryBeneficiary(ctx, req)
}
func CreatePayoutBatch(ctx context.Context, req CreatePayoutBatchRequest) (*PayoutBatchResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.CreatePayoutBatch(ctx, req)
}
func AddPayoutBatchItems(ctx context.Context, req AddPayoutBatchItemsRequest) (*PayoutBatchResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.AddPayoutBatchItems(ctx, req)
}
func SubmitPayoutBatch(ctx context.Context, req SubmitPayoutBatchRequest) (*PayoutBatchResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.SubmitPayoutBatch(ctx, req)
}
func QueryPayoutBatch(ctx context.Context, req QueryPayoutBatchRequest) (*PayoutBatchResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.QueryPayoutBatch(ctx, req)
}
func ListPayoutBatchItems(ctx context.Context, req ListPayoutBatchItemsRequest) (*PayoutBatchItemsResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.ListPayoutBatchItems(ctx, req)
}
