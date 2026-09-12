//go:build sdkit_payment_airwallex

package airwallex

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

var _ payment.PayoutProvider = (*Adapter)(nil)

func (a *Adapter) CreatePayout(ctx context.Context, req payment.CreatePayoutRequest) (*payment.PayoutResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	payout, ok := client.(payment.PayoutProvider)
	if !ok {
		return nil, payment.ErrUnsupportedCapability
	}
	return payout.CreatePayout(ctx, req)
}
func (a *Adapter) QueryPayout(ctx context.Context, req payment.QueryPayoutRequest) (*payment.PayoutResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	payout, ok := client.(payment.PayoutProvider)
	if !ok {
		return nil, payment.ErrUnsupportedCapability
	}
	return payout.QueryPayout(ctx, req)
}
