package payment

import (
	"context"
	"fmt"
	"strings"
)

type BeneficiarySchemaKind string

const (
	BeneficiarySchemaAPI  BeneficiarySchemaKind = "api"
	BeneficiarySchemaForm BeneficiarySchemaKind = "form"
)

type BeneficiarySchemaProvider interface {
	GenerateBeneficiarySchema(context.Context, GenerateBeneficiarySchemaRequest) (*BeneficiarySchemaResponse, error)
}

type GenerateBeneficiarySchemaRequest struct {
	Provider            Provider              `json:"provider"`
	Channel             Channel               `json:"channel"`
	MerchantKey         string                `json:"merchant_key,omitempty"`
	Kind                BeneficiarySchemaKind `json:"kind"`
	BankCountryCode     string                `json:"bank_country_code,omitempty"`
	AccountCurrency     string                `json:"account_currency,omitempty"`
	TransferMethod      string                `json:"transfer_method,omitempty"`
	LocalClearingSystem string                `json:"local_clearing_system,omitempty"`
	EntityType          string                `json:"entity_type,omitempty"`
	CountryCode         string                `json:"country_code,omitempty"`
}

type BeneficiarySchemaResponse struct {
	Provider    Provider              `json:"provider"`
	Channel     Channel               `json:"channel"`
	MerchantKey string                `json:"merchant_key,omitempty"`
	Kind        BeneficiarySchemaKind `json:"kind"`
	Condition   map[string]any        `json:"condition"`
	Fields      []map[string]any      `json:"fields"`
	Exchange    ProviderExchange      `json:"-"`
}

func (s *Service) GenerateBeneficiarySchema(ctx context.Context, req GenerateBeneficiarySchemaRequest) (*BeneficiarySchemaResponse, error) {
	provider, err := s.beneficiarySchemaProvider(ctx, "generate_beneficiary_schema", &req.Provider, &req.Channel, &req.MerchantKey)
	if err != nil {
		return nil, err
	}
	if err := normalizeBeneficiarySchemaRequest(&req); err != nil {
		return nil, err
	}
	return provider.GenerateBeneficiarySchema(ctx, req)
}

func (s *Service) beneficiarySchemaProvider(ctx context.Context, operation PaymentOperation, provider *Provider, channel *Channel, merchantKey *string) (BeneficiarySchemaProvider, error) {
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
	value, ok := adapter.(BeneficiarySchemaProvider)
	if !ok {
		return nil, fmt.Errorf("%w: beneficiary schema", ErrUnsupportedCapability)
	}
	return value, nil
}

func normalizeBeneficiarySchemaRequest(req *GenerateBeneficiarySchemaRequest) error {
	if req.Kind != BeneficiarySchemaAPI && req.Kind != BeneficiarySchemaForm {
		return ErrInvalidRequest
	}
	fields := []struct {
		value  *string
		length int
	}{
		{&req.BankCountryCode, 2},
		{&req.AccountCurrency, 3},
		{&req.CountryCode, 2},
	}
	for _, field := range fields {
		trimmed := strings.TrimSpace(*field.value)
		if trimmed != "" && (len(trimmed) != field.length || trimmed != strings.ToUpper(trimmed)) {
			return ErrInvalidRequest
		}
		*field.value = trimmed
	}
	req.TransferMethod = strings.TrimSpace(req.TransferMethod)
	if req.TransferMethod != "" && req.TransferMethod != "LOCAL" && req.TransferMethod != "SWIFT" {
		return ErrInvalidRequest
	}
	req.EntityType = strings.TrimSpace(req.EntityType)
	if req.EntityType != "" && req.EntityType != "COMPANY" && req.EntityType != "PERSONAL" {
		return ErrInvalidRequest
	}
	req.LocalClearingSystem = strings.TrimSpace(req.LocalClearingSystem)
	return nil
}

func GenerateBeneficiarySchema(ctx context.Context, req GenerateBeneficiarySchemaRequest) (*BeneficiarySchemaResponse, error) {
	s, err := Default()
	if err != nil {
		return nil, err
	}
	return s.GenerateBeneficiarySchema(ctx, req)
}
