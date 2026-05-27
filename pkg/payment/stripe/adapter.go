//go:build sdkit_payment_stripe

package stripe

import (
	"context"
	"fmt"

	"github.com/huwenlong92/sdkit/core/payment"
)

type Adapter struct {
	client       Client
	clientLoader ClientLoader
	capabilities payment.Capabilities
}

func NewAdapter(cfg Config) (*Adapter, error) {
	mode := cfg.ClientMode
	if mode == "" {
		mode = ClientModeDynamic
	}
	switch mode {
	case ClientModeDynamic:
		if cfg.ClientLoader == nil {
			return nil, fmt.Errorf("%w: stripe client loader is required", payment.ErrAdapterNotFound)
		}
	case ClientModeStatic:
		if cfg.Client == nil {
			return nil, fmt.Errorf("%w: stripe client is required", payment.ErrAdapterNotFound)
		}
	default:
		return nil, fmt.Errorf("%w: stripe client mode %s", payment.ErrInvalidRequest, mode)
	}
	return &Adapter{
		client:       cfg.Client,
		clientLoader: cfg.ClientLoader,
		capabilities: capabilities(cfg),
	}, nil
}

func (a *Adapter) Name() payment.Provider {
	return payment.ProviderStripe
}

func (a *Adapter) Capabilities() payment.Capabilities {
	return a.capabilities
}

func (a *Adapter) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	client, cleanup, err := a.clientFor(ctx, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	resp, err := client.CreatePayment(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: nil stripe create payment response", payment.ErrPaymentActionRequired)
	}
	if err := validateAction(req.Channel, resp.Action.Type); err != nil {
		return nil, err
	}
	return resp, nil
}

func (a *Adapter) QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	base, cleanup, err := a.clientFor(ctx, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	client, ok := base.(QueryClient)
	if !ok {
		return nil, fmt.Errorf("%w: stripe query payment", payment.ErrUnsupportedCapability)
	}
	return client.QueryPayment(ctx, req)
}

func (a *Adapter) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	if ctx == nil {
		return payment.ErrNilContext
	}
	base, cleanup, err := a.clientFor(ctx, req.MerchantKey)
	if err != nil {
		return err
	}
	defer cleanupClient(cleanup)
	client, ok := base.(CloseClient)
	if !ok {
		return fmt.Errorf("%w: stripe close payment", payment.ErrUnsupportedCapability)
	}
	return client.ClosePayment(ctx, req)
}

func (a *Adapter) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	base, cleanup, err := a.clientFor(ctx, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	client, ok := base.(RefundClient)
	if !ok {
		return nil, fmt.Errorf("%w: stripe refund", payment.ErrUnsupportedCapability)
	}
	return client.Refund(ctx, req)
}

func (a *Adapter) QueryRefund(ctx context.Context, req payment.QueryRefundRequest) (*payment.QueryRefundResponse, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	base, cleanup, err := a.clientFor(ctx, req.MerchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	client, ok := base.(QueryRefundClient)
	if !ok {
		return nil, fmt.Errorf("%w: stripe query refund", payment.ErrUnsupportedCapability)
	}
	return client.QueryRefund(ctx, req)
}

func (a *Adapter) ParseNotify(ctx context.Context, req payment.NotifyRequest) (*payment.NotifyResult, error) {
	if ctx == nil {
		return nil, payment.ErrNilContext
	}
	merchantKey := ""
	if req.Query != nil {
		values := req.Query["merchant_key"]
		if len(values) > 0 {
			merchantKey = values[0]
		}
	}
	base, cleanup, err := a.clientFor(ctx, merchantKey)
	if err != nil {
		return nil, err
	}
	defer cleanupClient(cleanup)
	client, ok := base.(NotifyClient)
	if !ok {
		return nil, fmt.Errorf("%w: stripe notify", payment.ErrUnsupportedCapability)
	}
	return client.ParseNotify(ctx, req)
}

func (a *Adapter) clientFor(ctx context.Context, merchantKey string) (Client, ClientCleanup, error) {
	if a.clientLoader != nil {
		client, cleanup, err := a.clientLoader.LoadPaymentClient(ctx, merchantKey)
		if err != nil {
			cleanupClient(cleanup)
			return nil, nil, err
		}
		if client == nil {
			cleanupClient(cleanup)
			return nil, cleanup, fmt.Errorf("%w: stripe client loader returned nil client", payment.ErrAdapterNotFound)
		}
		return client, cleanup, nil
	}
	if a.client == nil {
		return nil, nil, fmt.Errorf("%w: stripe client is required", payment.ErrAdapterNotFound)
	}
	return a.client, nil, nil
}

func cleanupClient(cleanup ClientCleanup) {
	if cleanup != nil {
		_ = cleanup()
	}
}

func capabilities(cfg Config) payment.Capabilities {
	return payment.Capabilities{
		Provider: payment.ProviderStripe,
		Channels: []payment.Channel{
			payment.ChannelStripeCheckout,
			payment.ChannelStripeIntent,
		},
		SupportedCurrencies:   append([]string(nil), cfg.SupportedCurrencies...),
		SupportedActions:      []payment.ActionType{payment.ActionRedirectURL, payment.ActionClientToken},
		SupportsMultiMerchant: cfg.SupportsMultiMerchant,
		SupportsExpireAt:      cfg.SupportsExpireAt,
		SupportsClose:         cfg.SupportsClose || supportsClose(cfg.Client),
		SupportsRefund:        cfg.SupportsRefund || supportsRefund(cfg.Client),
		SupportsQueryRefund:   cfg.SupportsQueryRefund || supportsQueryRefund(cfg.Client),
		SupportsPartialRefund: cfg.SupportsRefund || supportsRefund(cfg.Client),
		SupportsQuery:         cfg.SupportsQuery || supportsQuery(cfg.Client),
		SupportsNotify:        cfg.SupportsNotify || supportsNotify(cfg.Client),
	}
}

func validateAction(channel payment.Channel, action payment.ActionType) error {
	switch channel {
	case payment.ChannelStripeCheckout:
		if action == payment.ActionRedirectURL {
			return nil
		}
	case payment.ChannelStripeIntent:
		if action == payment.ActionClientToken {
			return nil
		}
	default:
		return fmt.Errorf("%w: stripe channel %s", payment.ErrUnsupportedChannel, channel)
	}
	return fmt.Errorf("%w: stripe channel %s action %s", payment.ErrInvalidAction, channel, action)
}

func supportsQuery(client Client) bool {
	_, ok := client.(QueryClient)
	return ok
}

func supportsClose(client Client) bool {
	_, ok := client.(CloseClient)
	return ok
}

func supportsRefund(client Client) bool {
	_, ok := client.(RefundClient)
	return ok
}

func supportsQueryRefund(client Client) bool {
	_, ok := client.(QueryRefundClient)
	return ok
}

func supportsNotify(client Client) bool {
	_, ok := client.(NotifyClient)
	return ok
}
