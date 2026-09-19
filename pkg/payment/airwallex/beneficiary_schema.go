//go:build sdkit_payment_airwallex

package airwallex

import (
	"context"

	"github.com/huwenlong92/sdkit/core/payment"
)

var _ payment.BeneficiarySchemaProvider = (*Adapter)(nil)

func (a *Adapter) GenerateBeneficiarySchema(ctx context.Context, req payment.GenerateBeneficiarySchemaRequest) (*payment.BeneficiarySchemaResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	provider, ok := client.(payment.BeneficiarySchemaProvider)
	if !ok {
		return nil, payment.ErrUnsupportedCapability
	}
	return provider.GenerateBeneficiarySchema(ctx, req)
}
