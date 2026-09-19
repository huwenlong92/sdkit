//go:build sdkit_payment_airwallex

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/huwenlong92/sdkit/core/payment"
)

type beneficiarySchemaResult struct {
	exchangeResult
	Condition map[string]any   `json:"condition"`
	Fields    []map[string]any `json:"fields"`
}

func (c *Client) GenerateBeneficiarySchema(ctx context.Context, req payment.GenerateBeneficiarySchemaRequest) (*payment.BeneficiarySchemaResponse, error) {
	if err := c.validatePayoutOperation(ctx, req.Provider, req.Channel); err != nil {
		return nil, err
	}
	path := "/api/v1/beneficiary_api_schemas/generate"
	if req.Kind == payment.BeneficiarySchemaForm {
		path = "/api/v1/beneficiary_form_schemas/generate"
	} else if req.Kind != payment.BeneficiarySchemaAPI {
		return nil, payment.ErrInvalidRequest
	}
	body := make(map[string]string, 6)
	for key, value := range map[string]string{
		"bank_country_code":     req.BankCountryCode,
		"account_currency":      req.AccountCurrency,
		"transfer_method":       req.TransferMethod,
		"local_clearing_system": req.LocalClearingSystem,
		"entity_type":           req.EntityType,
		"country_code":          req.CountryCode,
	} {
		if value != "" {
			body[key] = value
		}
	}
	requestBody, _ := json.Marshal(body)
	var raw beneficiarySchemaResult
	if err := c.call(ctx, http.MethodPost, path, body, &raw); err != nil {
		return nil, err
	}
	if raw.Condition == nil || raw.Fields == nil {
		return nil, payment.WithProviderExchange(payment.ErrPaymentReference, providerExchange(requestBody, raw.exchangeResult))
	}
	return &payment.BeneficiarySchemaResponse{
		Provider: payment.ProviderAirwallex, Channel: payment.ChannelAirwallexTransfer,
		MerchantKey: req.MerchantKey, Kind: req.Kind, Condition: raw.Condition, Fields: raw.Fields,
		Exchange: providerExchange(requestBody, raw.exchangeResult),
	}, nil
}
