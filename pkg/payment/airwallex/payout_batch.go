//go:build sdkit_payment_airwallex

package airwallex

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

var _ payment.BeneficiaryProvider = (*Adapter)(nil)
var _ payment.BatchPayoutProvider = (*Adapter)(nil)

func (a *Adapter) CreateBeneficiary(ctx context.Context, req payment.CreateBeneficiaryRequest) (*payment.BeneficiaryResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	provider, ok := client.(payment.BeneficiaryProvider)
	if !ok {
		return nil, payment.ErrUnsupportedCapability
	}
	return provider.CreateBeneficiary(ctx, req)
}

func (a *Adapter) QueryBeneficiary(ctx context.Context, req payment.QueryBeneficiaryRequest) (*payment.BeneficiaryResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	provider, ok := client.(payment.BeneficiaryProvider)
	if !ok {
		return nil, payment.ErrUnsupportedCapability
	}
	return provider.QueryBeneficiary(ctx, req)
}

func (a *Adapter) batchProvider(ctx context.Context, provider payment.Provider, channel payment.Channel, merchantKey string) (payment.BatchPayoutProvider, ClientCleanup, error) {
	client, cleanup, err := a.clientFor(ctx, provider, channel, merchantKey)
	if err != nil {
		return nil, nil, err
	}
	value, ok := client.(payment.BatchPayoutProvider)
	if !ok {
		cleanupClient(cleanup)
		return nil, nil, payment.ErrUnsupportedCapability
	}
	return value, cleanup, nil
}

func (a *Adapter) CreatePayoutBatch(ctx context.Context, req payment.CreatePayoutBatchRequest) (*payment.PayoutBatchResponse, error) {
	provider, cleanup, err := a.batchProvider(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	return provider.CreatePayoutBatch(ctx, req)
}
func (a *Adapter) AddPayoutBatchItems(ctx context.Context, req payment.AddPayoutBatchItemsRequest) (*payment.PayoutBatchResponse, error) {
	provider, cleanup, err := a.batchProvider(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	return provider.AddPayoutBatchItems(ctx, req)
}
func (a *Adapter) SubmitPayoutBatch(ctx context.Context, req payment.SubmitPayoutBatchRequest) (*payment.PayoutBatchResponse, error) {
	provider, cleanup, err := a.batchProvider(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	return provider.SubmitPayoutBatch(ctx, req)
}
func (a *Adapter) QueryPayoutBatch(ctx context.Context, req payment.QueryPayoutBatchRequest) (*payment.PayoutBatchResponse, error) {
	provider, cleanup, err := a.batchProvider(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	return provider.QueryPayoutBatch(ctx, req)
}
func (a *Adapter) ListPayoutBatchItems(ctx context.Context, req payment.ListPayoutBatchItemsRequest) (*payment.PayoutBatchItemsResponse, error) {
	provider, cleanup, err := a.batchProvider(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	return provider.ListPayoutBatchItems(ctx, req)
}
