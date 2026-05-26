package paypal

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
			return nil, fmt.Errorf("%w: paypal client loader is required", payment.ErrAdapterNotFound)
		}
	case ClientModeStatic:
		if cfg.Client == nil {
			return nil, fmt.Errorf("%w: paypal client is required", payment.ErrAdapterNotFound)
		}
	default:
		return nil, fmt.Errorf("%w: paypal client mode %s", payment.ErrInvalidRequest, mode)
	}
	return &Adapter{
		client:       cfg.Client,
		clientLoader: cfg.ClientLoader,
		capabilities: capabilities(cfg),
	}, nil
}

func (a *Adapter) Name() payment.Provider {
	return payment.ProviderPayPal
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
		return nil, fmt.Errorf("%w: nil paypal create payment response", payment.ErrPaymentActionRequired)
	}
	if req.Channel != payment.ChannelPayPalOrder {
		return nil, fmt.Errorf("%w: paypal channel %s", payment.ErrUnsupportedChannel, req.Channel)
	}
	if resp.Action.Type != payment.ActionRedirectURL {
		return nil, fmt.Errorf("%w: paypal channel %s action %s", payment.ErrInvalidAction, req.Channel, resp.Action.Type)
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
		return nil, fmt.Errorf("%w: paypal query payment", payment.ErrUnsupportedCapability)
	}
	return client.QueryPayment(ctx, req)
}

func (a *Adapter) ClosePayment(ctx context.Context, req payment.ClosePaymentRequest) error {
	if ctx == nil {
		return payment.ErrNilContext
	}
	return fmt.Errorf("%w: paypal close payment", payment.ErrUnsupportedCapability)
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
		return nil, fmt.Errorf("%w: paypal refund", payment.ErrUnsupportedCapability)
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
		return nil, fmt.Errorf("%w: paypal query refund", payment.ErrUnsupportedCapability)
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
		return nil, fmt.Errorf("%w: paypal notify", payment.ErrUnsupportedCapability)
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
			return nil, cleanup, fmt.Errorf("%w: paypal client loader returned nil client", payment.ErrAdapterNotFound)
		}
		return client, cleanup, nil
	}
	if a.client == nil {
		return nil, nil, fmt.Errorf("%w: paypal client is required", payment.ErrAdapterNotFound)
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
		Provider:              payment.ProviderPayPal,
		Channels:              []payment.Channel{payment.ChannelPayPalOrder},
		SupportedCurrencies:   append([]string(nil), cfg.SupportedCurrencies...),
		SupportedActions:      []payment.ActionType{payment.ActionRedirectURL},
		SupportsMultiMerchant: cfg.SupportsMultiMerchant,
		SupportsRefund:        cfg.SupportsRefund || supportsRefund(cfg.Client),
		SupportsQueryRefund:   cfg.SupportsQueryRefund || supportsQueryRefund(cfg.Client),
		SupportsPartialRefund: cfg.SupportsRefund || supportsRefund(cfg.Client),
		SupportsQuery:         cfg.SupportsQuery || supportsQuery(cfg.Client),
		SupportsNotify:        cfg.SupportsNotify || supportsNotify(cfg.Client),
	}
}

func supportsQuery(client Client) bool {
	_, ok := client.(QueryClient)
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
