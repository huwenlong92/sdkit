package payment

import (
	corepayment "github.com/huwenlong92/sdkit/core/payment"
	"github.com/huwenlong92/sdkit/core/runtime"
)

const Name = string(corepayment.KeyPayment)

type Service = corepayment.Service
type ServiceConfig = corepayment.ServiceConfig
type Registry = corepayment.Registry
type ProviderAdapter = corepayment.ProviderAdapter
type PricingPolicy = corepayment.PricingPolicy
type ChannelSelector = corepayment.ChannelSelector
type ReloadableChannelSelector = corepayment.ReloadableChannelSelector
type ChannelBinding = corepayment.ChannelBinding
type ChannelSelection = corepayment.ChannelSelection
type ChannelSelectionRequest = corepayment.ChannelSelectionRequest

func FromDefault() (*Service, error) {
	return corepayment.Default()
}

func From(app *runtime.App) *Service {
	return corepayment.From(app)
}
