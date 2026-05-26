package payment

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ServiceConfig struct {
	Registry        *Registry
	PricingPolicy   PricingPolicy
	ChannelSelector ChannelSelector
}

type Service struct {
	registry        *Registry
	pricingPolicy   PricingPolicy
	channelSelector ChannelSelector
}

func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.Registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrAdapterNotFound)
	}
	policy := cfg.PricingPolicy
	if policy == nil {
		defaultPolicy := NewDefaultPricingPolicy()
		policy = defaultPolicy
	}
	return &Service{
		registry:        cfg.Registry,
		pricingPolicy:   policy,
		channelSelector: cfg.ChannelSelector,
	}, nil
}

func (s *Service) CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResponse, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := s.selectChannel(ctx, PaymentOperationCreate, &req.Provider, &req.Channel, &req.MerchantKey); err != nil {
		return nil, err
	}
	if err := validateCreatePaymentRequest(req, time.Now()); err != nil {
		return nil, err
	}
	adapter, caps, err := s.adapter(req.Provider)
	if err != nil {
		return nil, err
	}
	if err := validateChannel(caps, req.Channel); err != nil {
		return nil, err
	}
	pricing, err := s.pricingPolicy.NormalizePricing(ctx, req.Pricing)
	if err != nil {
		return nil, err
	}
	if err := validateCurrency(caps, pricing.PayAmount.Currency); err != nil {
		return nil, err
	}
	req.Pricing = pricing

	resp, err := adapter.CreatePayment(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: nil create payment response", ErrPaymentActionRequired)
	}
	action, err := normalizeAndValidateAction(resp.Action)
	if err != nil {
		return nil, err
	}
	resp.Action = action
	if err := validateAction(caps, resp.Action.Type); err != nil {
		return nil, err
	}
	fillCreatePaymentResponseDefaults(resp, req)
	if resp.Pricing.PayAmount.Amount == 0 {
		resp.Pricing = pricing
	}
	return resp, nil
}

func (s *Service) QueryPayment(ctx context.Context, req QueryPaymentRequest) (*QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := s.selectChannel(ctx, PaymentOperationQuery, &req.Provider, &req.Channel, &req.MerchantKey); err != nil {
		return nil, err
	}
	if err := validateQueryPaymentRequest(req); err != nil {
		return nil, err
	}
	adapter, caps, err := s.adapter(req.Provider)
	if err != nil {
		return nil, err
	}
	if err := validateChannel(caps, req.Channel); err != nil {
		return nil, err
	}
	if !caps.SupportsQuery {
		return nil, fmt.Errorf("%w: query payment", ErrUnsupportedCapability)
	}
	resp, err := adapter.QueryPayment(ctx, req)
	if err != nil {
		return nil, err
	}
	fillQueryPaymentResponseDefaults(resp, req)
	return resp, nil
}

func (s *Service) ClosePayment(ctx context.Context, req ClosePaymentRequest) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := s.selectChannel(ctx, PaymentOperationClose, &req.Provider, &req.Channel, &req.MerchantKey); err != nil {
		return err
	}
	if err := validateClosePaymentRequest(req); err != nil {
		return err
	}
	adapter, caps, err := s.adapter(req.Provider)
	if err != nil {
		return err
	}
	if err := validateChannel(caps, req.Channel); err != nil {
		return err
	}
	if !caps.SupportsClose {
		return fmt.Errorf("%w: close payment", ErrUnsupportedCapability)
	}
	return adapter.ClosePayment(ctx, req)
}

func (s *Service) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := s.selectChannel(ctx, PaymentOperationRefund, &req.Provider, &req.Channel, &req.MerchantKey); err != nil {
		return nil, err
	}
	if err := validateRefundRequest(req); err != nil {
		return nil, err
	}
	adapter, caps, err := s.adapter(req.Provider)
	if err != nil {
		return nil, err
	}
	if err := validateChannel(caps, req.Channel); err != nil {
		return nil, err
	}
	if !caps.SupportsRefund {
		return nil, fmt.Errorf("%w: refund", ErrUnsupportedCapability)
	}
	amount, err := normalizeRefundAmount(req.Amount)
	if err != nil {
		return nil, err
	}
	if err := validateCurrency(caps, amount.Refund.Currency); err != nil {
		return nil, err
	}
	req.Amount = amount
	return adapter.Refund(ctx, req)
}

func (s *Service) QueryRefund(ctx context.Context, req QueryRefundRequest) (*QueryRefundResponse, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := s.selectChannel(ctx, PaymentOperationQueryRefund, &req.Provider, &req.Channel, &req.MerchantKey); err != nil {
		return nil, err
	}
	if err := validateQueryRefundRequest(req); err != nil {
		return nil, err
	}
	adapter, caps, err := s.adapter(req.Provider)
	if err != nil {
		return nil, err
	}
	if err := validateChannel(caps, req.Channel); err != nil {
		return nil, err
	}
	if !caps.SupportsQueryRefund {
		return nil, fmt.Errorf("%w: query refund", ErrUnsupportedCapability)
	}
	resp, err := adapter.QueryRefund(ctx, req)
	if err != nil {
		return nil, err
	}
	fillQueryRefundResponseDefaults(resp, req)
	return resp, nil
}

func (s *Service) HandleNotify(ctx context.Context, req NotifyRequest) (*NotifyResult, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := validateNotifyRequest(req); err != nil {
		return nil, err
	}
	adapter, caps, err := s.adapter(req.Provider)
	if err != nil {
		return nil, err
	}
	if err := validateChannel(caps, req.Channel); err != nil {
		return nil, err
	}
	if !caps.SupportsNotify {
		return nil, fmt.Errorf("%w: notify", ErrUnsupportedCapability)
	}
	result, err := adapter.ParseNotify(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: nil notify result", ErrNotifyVerificationFail)
	}
	if !result.Verified {
		return nil, ErrNotifyVerificationFail
	}
	return result, nil
}

func (s *Service) ReloadChannels(bindings []ChannelBinding) error {
	selector, ok := s.channelSelector.(ReloadableChannelSelector)
	if !ok {
		return fmt.Errorf("%w: payment channel selector is not reloadable", ErrUnsupportedCapability)
	}
	return selector.Reload(bindings)
}

func (s *Service) selectChannel(ctx context.Context, operation PaymentOperation, provider *Provider, channel *Channel, merchantKey *string) error {
	if s.channelSelector == nil || merchantKey == nil || *merchantKey == "" {
		return nil
	}
	selection, err := s.channelSelector.SelectPaymentChannel(ctx, ChannelSelectionRequest{
		Provider:    valueProvider(provider),
		Channel:     valueChannel(channel),
		MerchantKey: *merchantKey,
		Operation:   operation,
	})
	if err != nil {
		if provider != nil && *provider != "" && channel != nil && *channel != "" && errors.Is(err, ErrAdapterNotFound) {
			return nil
		}
		return err
	}
	if selection == nil {
		return fmt.Errorf("%w: nil payment channel selection", ErrAdapterNotFound)
	}
	if selection.Provider == "" {
		return fmt.Errorf("%w: selected provider is required", ErrUnsupportedProvider)
	}
	if selection.Channel == "" {
		return fmt.Errorf("%w: selected channel is required", ErrUnsupportedChannel)
	}
	if provider != nil && *provider != "" && *provider != selection.Provider {
		return fmt.Errorf("%w: selected provider %s does not match %s", ErrUnsupportedProvider, selection.Provider, *provider)
	}
	if channel != nil && *channel != "" && *channel != selection.Channel {
		return fmt.Errorf("%w: selected channel %s does not match %s", ErrUnsupportedChannel, selection.Channel, *channel)
	}
	if provider != nil {
		*provider = selection.Provider
	}
	if channel != nil {
		*channel = selection.Channel
	}
	if selection.MerchantKey != "" {
		*merchantKey = selection.MerchantKey
	}
	return nil
}

func valueProvider(provider *Provider) Provider {
	if provider == nil {
		return ""
	}
	return *provider
}

func valueChannel(channel *Channel) Channel {
	if channel == nil {
		return ""
	}
	return *channel
}

func (s *Service) adapter(provider Provider) (ProviderAdapter, Capabilities, error) {
	adapter, err := s.registry.Adapter(provider)
	if err != nil {
		return nil, Capabilities{}, err
	}
	caps := adapter.Capabilities()
	if caps.Provider != "" && caps.Provider != provider {
		return nil, Capabilities{}, fmt.Errorf("%w: capability provider %s does not match %s", ErrUnsupportedProvider, caps.Provider, provider)
	}
	if caps.Provider == "" {
		caps.Provider = provider
	}
	return adapter, caps, nil
}

func validateChannel(caps Capabilities, channel Channel) error {
	if len(caps.Channels) == 0 {
		return nil
	}
	for _, supported := range caps.Channels {
		if supported == channel {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedChannel, channel)
}

func validateCurrency(caps Capabilities, currency string) error {
	if len(caps.SupportedCurrencies) == 0 {
		return nil
	}
	currency = NormalizeCurrency(currency)
	for _, supported := range caps.SupportedCurrencies {
		if NormalizeCurrency(supported) == currency {
			return nil
		}
	}
	return fmt.Errorf("%w: currency %s", ErrUnsupportedCapability, currency)
}

func validateAction(caps Capabilities, action ActionType) error {
	if len(caps.SupportedActions) == 0 || action == "" {
		return nil
	}
	for _, supported := range caps.SupportedActions {
		if supported == action {
			return nil
		}
	}
	return fmt.Errorf("%w: action %s", ErrUnsupportedCapability, action)
}

func normalizeRefundAmount(amount RefundAmount) (RefundAmount, error) {
	if amount.Refund.Amount <= 0 {
		return RefundAmount{}, fmt.Errorf("%w: refund amount must be positive", ErrInvalidAmount)
	}
	amount.Refund.Currency = NormalizeCurrency(amount.Refund.Currency)
	if amount.Settle.Amount == 0 && amount.Settle.Currency == "" {
		amount.Settle = amount.Refund
		return amount, nil
	}
	if amount.Settle.Amount <= 0 {
		return RefundAmount{}, fmt.Errorf("%w: refund settle amount must be positive", ErrInvalidAmount)
	}
	amount.Settle.Currency = NormalizeCurrency(amount.Settle.Currency)
	return amount, nil
}

func fillCreatePaymentResponseDefaults(resp *CreatePaymentResponse, req CreatePaymentRequest) {
	if resp.Provider == "" {
		resp.Provider = req.Provider
	}
	if resp.Channel == "" {
		resp.Channel = req.Channel
	}
	if resp.MerchantKey == "" {
		resp.MerchantKey = req.MerchantKey
	}
	if resp.PaymentID == "" {
		resp.PaymentID = req.PaymentID
	}
	if resp.OrderID == "" {
		resp.OrderID = req.OrderID
	}
	if resp.OutTradeNo == "" {
		resp.OutTradeNo = req.OutTradeNo
	}
}

func fillQueryPaymentResponseDefaults(resp *QueryPaymentResponse, req QueryPaymentRequest) {
	if resp == nil {
		return
	}
	if resp.Provider == "" {
		resp.Provider = req.Provider
	}
	if resp.Channel == "" {
		resp.Channel = req.Channel
	}
	if resp.MerchantKey == "" {
		resp.MerchantKey = req.MerchantKey
	}
	if resp.PaymentID == "" {
		resp.PaymentID = req.PaymentID
	}
	if resp.OrderID == "" {
		resp.OrderID = req.OrderID
	}
	if resp.OutTradeNo == "" {
		resp.OutTradeNo = req.OutTradeNo
	}
	if resp.ProviderTradeID == "" {
		resp.ProviderTradeID = req.ProviderTradeID
	}
}

func fillQueryRefundResponseDefaults(resp *QueryRefundResponse, req QueryRefundRequest) {
	if resp == nil {
		return
	}
	if resp.Provider == "" {
		resp.Provider = req.Provider
	}
	if resp.Channel == "" {
		resp.Channel = req.Channel
	}
	if resp.MerchantKey == "" {
		resp.MerchantKey = req.MerchantKey
	}
	if resp.PaymentID == "" {
		resp.PaymentID = req.PaymentID
	}
	if resp.OutTradeNo == "" {
		resp.OutTradeNo = req.OutTradeNo
	}
	if resp.ProviderTradeID == "" {
		resp.ProviderTradeID = req.ProviderTradeID
	}
	if resp.RefundID == "" {
		resp.RefundID = req.RefundID
	}
	if resp.ProviderRefundID == "" {
		resp.ProviderRefundID = req.ProviderRefundID
	}
	if resp.OutRefundNo == "" {
		resp.OutRefundNo = req.OutRefundNo
	}
}
