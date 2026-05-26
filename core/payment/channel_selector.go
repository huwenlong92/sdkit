package payment

import (
	"context"
	"fmt"
	"sync"
)

type PaymentOperation string

const (
	PaymentOperationCreate      PaymentOperation = "create"
	PaymentOperationQuery       PaymentOperation = "query"
	PaymentOperationClose       PaymentOperation = "close"
	PaymentOperationRefund      PaymentOperation = "refund"
	PaymentOperationQueryRefund PaymentOperation = "query_refund"
)

type ChannelSelectionRequest struct {
	Provider    Provider
	Channel     Channel
	MerchantKey string
	Operation   PaymentOperation
}

type ChannelSelection struct {
	Provider    Provider
	Channel     Channel
	MerchantKey string
}

type ChannelSelector interface {
	SelectPaymentChannel(ctx context.Context, req ChannelSelectionRequest) (*ChannelSelection, error)
}

type ReloadableChannelSelector interface {
	ChannelSelector
	Reload(bindings []ChannelBinding) error
}

type ChannelBinding struct {
	Key         string   `mapstructure:"key" yaml:"key"`
	Provider    Provider `mapstructure:"provider" yaml:"provider"`
	Channel     Channel  `mapstructure:"channel" yaml:"channel"`
	MerchantKey string   `mapstructure:"merchant_key" yaml:"merchant_key"`
}

type StaticChannelSelector struct {
	mu       sync.RWMutex
	bindings map[string]ChannelSelection
}

func NewStaticChannelSelector(bindings []ChannelBinding) (*StaticChannelSelector, error) {
	next, err := buildChannelBindingMap(bindings)
	if err != nil {
		return nil, err
	}
	return &StaticChannelSelector{bindings: next}, nil
}

func (s *StaticChannelSelector) Reload(bindings []ChannelBinding) error {
	next, err := buildChannelBindingMap(bindings)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.bindings = next
	s.mu.Unlock()
	return nil
}

func (s *StaticChannelSelector) SelectPaymentChannel(ctx context.Context, req ChannelSelectionRequest) (*ChannelSelection, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.MerchantKey == "" {
		return nil, fmt.Errorf("%w: merchant key is required", ErrInvalidRequest)
	}
	s.mu.RLock()
	selection, ok := s.bindings[req.MerchantKey]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: payment channel binding %s", ErrAdapterNotFound, req.MerchantKey)
	}
	if req.Provider != "" && req.Provider != selection.Provider {
		return nil, fmt.Errorf("%w: channel binding %s provider %s does not match %s", ErrUnsupportedProvider, req.MerchantKey, selection.Provider, req.Provider)
	}
	if req.Channel != "" && req.Channel != selection.Channel {
		return nil, fmt.Errorf("%w: channel binding %s channel %s does not match %s", ErrUnsupportedChannel, req.MerchantKey, selection.Channel, req.Channel)
	}
	return &selection, nil
}

func buildChannelBindingMap(bindings []ChannelBinding) (map[string]ChannelSelection, error) {
	out := make(map[string]ChannelSelection, len(bindings))
	for _, binding := range bindings {
		key, selection, err := normalizeChannelBinding(binding)
		if err != nil {
			return nil, err
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("%w: duplicate channel binding %s", ErrAdapterAlreadyExists, key)
		}
		out[key] = selection
	}
	return out, nil
}

func normalizeChannelBinding(binding ChannelBinding) (string, ChannelSelection, error) {
	if binding.Key == "" {
		return "", ChannelSelection{}, fmt.Errorf("%w: channel binding key is required", ErrInvalidRequest)
	}
	if binding.Provider == "" {
		return "", ChannelSelection{}, fmt.Errorf("%w: channel binding provider is required", ErrUnsupportedProvider)
	}
	if binding.Channel == "" {
		return "", ChannelSelection{}, fmt.Errorf("%w: channel binding channel is required", ErrUnsupportedChannel)
	}
	merchantKey := binding.MerchantKey
	if merchantKey == "" {
		merchantKey = binding.Key
	}
	return binding.Key, ChannelSelection{
		Provider:    binding.Provider,
		Channel:     binding.Channel,
		MerchantKey: merchantKey,
	}, nil
}
