//go:build sdkit_payment_airwallex

package airwallex

import (
	"context"
	"fmt"

	"github.com/huwenlong92/sdkit/core/payment"
)

type Adapter struct {
	client            Client
	loader            ClientLoader
	notifyMerchantKey string
	currencies        []string
}

var _ payment.ProviderAdapter = (*Adapter)(nil)

func NewAdapter(cfg Config) (*Adapter, error) {
	mode := cfg.ClientMode
	if mode == "" {
		mode = ClientModeDynamic
	}
	a := &Adapter{notifyMerchantKey: cfg.NotifyMerchantKey, currencies: append([]string(nil), cfg.SupportedCurrencies...)}
	switch mode {
	case ClientModeDynamic:
		if cfg.ClientLoader == nil {
			return nil, fmt.Errorf("%w: airwallex client loader required", payment.ErrAdapterNotFound)
		}
		a.loader = cfg.ClientLoader
	case ClientModeStatic:
		if cfg.Client == nil || cfg.ClientLoader != nil {
			return nil, fmt.Errorf("%w: static mode requires only a client", payment.ErrInvalidRequest)
		}
		a.client = cfg.Client
	default:
		return nil, fmt.Errorf("%w: airwallex client mode", payment.ErrInvalidRequest)
	}
	return a, nil
}

func (a *Adapter) Name() payment.Provider { return payment.ProviderAirwallex }

func (a *Adapter) Capabilities() payment.Capabilities {
	return payment.Capabilities{
		Provider: a.Name(), Channels: []payment.Channel{payment.ChannelAirwallexHPP},
		SupportedCurrencies: append([]string(nil), a.currencies...), SupportedActions: []payment.ActionType{payment.ActionSDKParams, payment.ActionNone},
		SupportsMultiMerchant: a.loader != nil, SupportsQuery: true, SupportsClose: true,
		SupportsRefund: true, SupportsPartialRefund: true, SupportsQueryRefund: true, SupportsNotify: true,
	}
}

func (a *Adapter) clientFor(ctx context.Context, provider payment.Provider, channel payment.Channel, key string) (Client, ClientCleanup, error) {
	if ctx == nil {
		return nil, nil, payment.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if provider != "" && provider != a.Name() {
		return nil, nil, payment.ErrUnsupportedProvider
	}
	if channel != payment.ChannelAirwallexHPP && channel != payment.ChannelAirwallexTransfer {
		return nil, nil, payment.ErrUnsupportedChannel
	}
	if a.loader == nil {
		return a.client, nil, nil
	}
	if key == "" {
		return nil, nil, fmt.Errorf("%w: airwallex merchant key required", payment.ErrInvalidRequest)
	}
	client, cleanup, err := a.loader.LoadPaymentClient(ctx, key)
	if err != nil {
		cleanupClient(cleanup)
		return nil, nil, err
	}
	if client == nil {
		cleanupClient(cleanup)
		return nil, nil, fmt.Errorf("%w: airwallex loader returned nil", payment.ErrAdapterNotFound)
	}
	return client, cleanup, nil
}

func cleanupClient(cleanup ClientCleanup) {
	if cleanup != nil {
		_ = cleanup()
	}
}

func (a *Adapter) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	result, err := client.CreatePayment(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: nil airwallex response", payment.ErrInvalidRequest)
	}
	return result, nil
}

func (a *Adapter) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	result, err := client.QueryPayment(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: nil airwallex response", payment.ErrInvalidRequest)
	}
	return result, nil
}

func (a *Adapter) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return err
	}
	defer cleanupClient(cleanup)
	return client.ClosePayment(ctx, req)
}

func (a *Adapter) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	result, err := client.Refund(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: nil airwallex response", payment.ErrInvalidRequest)
	}
	return result, nil
}

func (a *Adapter) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	result, err := client.QueryRefund(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: nil airwallex response", payment.ErrInvalidRequest)
	}
	return result, nil
}

func (a *Adapter) ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
	key := req.MerchantKey
	if key == "" {
		key = a.notifyMerchantKey
	}
	client, cleanup, err := a.clientFor(ctx, req.Provider, req.Channel, key)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	result, err := client.ParseNotify(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: nil airwallex response", payment.ErrInvalidRequest)
	}
	if result.Event != nil {
		result.Event.MerchantKey = key
	}
	return result, nil
}
