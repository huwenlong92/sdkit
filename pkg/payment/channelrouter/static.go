package channelrouter

import (
	"context"
	"fmt"

	"github.com/huwenlong92/sdkit/core/payment"
)

type StaticResolver struct {
	routes   map[routeKey]ResolvedRoute
	defaults map[defaultKey]ResolvedRoute
}

type routeKey struct {
	provider    payment.Provider
	channel     payment.Channel
	merchantKey string
}

type defaultKey struct {
	provider payment.Provider
	channel  payment.Channel
}

func NewStaticResolver(routes []Route) (*StaticResolver, error) {
	resolver := &StaticResolver{
		routes:   make(map[routeKey]ResolvedRoute),
		defaults: make(map[defaultKey]ResolvedRoute),
	}
	for _, route := range routes {
		if err := resolver.Add(route); err != nil {
			return nil, err
		}
	}
	return resolver, nil
}

func (r *StaticResolver) Add(route Route) error {
	if route.Provider == "" {
		return fmt.Errorf("%w: route provider is required", payment.ErrUnsupportedProvider)
	}
	if route.MerchantKey == "" {
		return fmt.Errorf("%w: route merchant key is required", payment.ErrInvalidRequest)
	}
	if route.Adapter == nil {
		return fmt.Errorf("%w: route adapter is required", payment.ErrAdapterNotFound)
	}
	if len(route.Channels) == 0 {
		return fmt.Errorf("%w: route channels are required", payment.ErrUnsupportedChannel)
	}
	for _, channel := range route.Channels {
		if channel == "" {
			return fmt.Errorf("%w: route channel is required", payment.ErrUnsupportedChannel)
		}
		resolved := ResolvedRoute{
			Provider:    route.Provider,
			Channel:     channel,
			MerchantKey: route.MerchantKey,
			Adapter:     route.Adapter,
		}
		key := routeKey{provider: route.Provider, channel: channel, merchantKey: route.MerchantKey}
		if _, exists := r.routes[key]; exists {
			return fmt.Errorf("%w: duplicate route %s/%s/%s", payment.ErrAdapterAlreadyExists, route.Provider, channel, route.MerchantKey)
		}
		r.routes[key] = resolved
		if route.Default {
			defaultKey := defaultKey{provider: route.Provider, channel: channel}
			if _, exists := r.defaults[defaultKey]; exists {
				return fmt.Errorf("%w: duplicate default route %s/%s", payment.ErrAdapterAlreadyExists, route.Provider, channel)
			}
			r.defaults[defaultKey] = resolved
		}
	}
	return nil
}

func (r *StaticResolver) ResolvePaymentChannel(ctx context.Context, req ResolveRequest) (*ResolvedRoute, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Provider == "" {
		return nil, fmt.Errorf("%w: provider is required", payment.ErrUnsupportedProvider)
	}
	if req.Channel == "" {
		return nil, fmt.Errorf("%w: channel is required", payment.ErrUnsupportedChannel)
	}
	if req.MerchantKey != "" {
		key := routeKey{provider: req.Provider, channel: req.Channel, merchantKey: req.MerchantKey}
		if route, ok := r.routes[key]; ok {
			return &route, nil
		}
		return nil, fmt.Errorf("%w: payment channel route %s/%s/%s", payment.ErrAdapterNotFound, req.Provider, req.Channel, req.MerchantKey)
	}
	key := defaultKey{provider: req.Provider, channel: req.Channel}
	if route, ok := r.defaults[key]; ok {
		return &route, nil
	}
	return nil, fmt.Errorf("%w: default payment channel route %s/%s", payment.ErrAdapterNotFound, req.Provider, req.Channel)
}
